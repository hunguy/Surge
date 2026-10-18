//go:build linux && !android

package clipboard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Linux has no clipboard API: the text lives in a helper process that talks to
// the running Wayland or X11 session. atotto/clipboard resolves that helper
// once, at package init, and keeps it for the whole run, and it only picks
// wl-clipboard when WAYLAND_DISPLAY is set and both wl-copy and wl-paste are on
// PATH. A process without WAYLAND_DISPLAY in its environment (a container, a
// toolbox, ssh, sudo, or a launcher that does not import the session
// environment) therefore locks onto xclip and fails every copy with a bare
// "exit status 1", even though an installed wl-copy could reach the session.
// Resolve the helper per call and try every candidate that might still work.
//
// wl-copy leads the list on every session, not just Wayland ones: libwayland
// falls back to $XDG_RUNTIME_DIR/wayland-0 when WAYLAND_DISPLAY is unset, so it
// succeeds where xclip cannot find an X display. xclip and xsel remain for X11
// sessions, and their diagnostics are what the copy path used to swallow.
//
// A helper still needs a display to talk to, and the clipboard is often reached
// from a process that has none: over ssh, from a systemd unit, or on a machine
// whose session is gamescope, which publishes its compositor socket as
// $XDG_RUNTIME_DIR/gamescope-0 rather than wayland-0. When the inherited
// environment reaches nothing, look for the displays that do exist on the
// machine and retry the helpers against them instead of giving up.
const (
	wlCopy  = "wl-copy"
	wlPaste = "wl-paste"
	xclip   = "xclip"
	xsel    = "xsel"
)

// x11SocketDir holds the local sockets X servers listen on ("X0" for DISPLAY=:0).
// It is a variable so tests can point discovery at a fixture directory.
var x11SocketDir = "/tmp/.X11-unix"

// helperTimeout bounds one helper call. A helper with no usable display fails at
// once, but a compositor that accepts the connection without ever serving the
// clipboard (gamescope does this) leaves wl-copy waiting forever, and the copy
// runs on the TUI's keypress path.
const helperTimeout = 3 * time.Second

// maxDiscoveredDisplays caps how many displays discovery retries, so a copy that
// cannot work does not spawn a helper for every stale socket on the machine.
const maxDiscoveredDisplays = 4

// maxReportedFailures caps the failure list in the error message: discovery can
// leave several attempts behind, and the first few reasons are the useful ones.
const maxReportedFailures = 4

// helperKind is the display protocol a helper speaks, so discovery only retries
// it against displays it can actually use.
type helperKind int

const (
	waylandHelper helperKind = iota
	x11Helper
)

// clipboardHelper is one external program that can reach the session
// clipboard. Writing helpers consume the text on stdin; reading helpers print
// the clipboard on stdout.
type clipboardHelper struct {
	bin  string
	args []string
	kind helperKind
}

// clipboardAttempt is one helper call: the helper, plus the display environment
// it should run with. A nil env inherits the environment as it is.
type clipboardAttempt struct {
	helper clipboardHelper
	env    []string
	// label names the display a discovered attempt targets; it is empty when the
	// attempt reuses the inherited environment.
	label string
}

// displayTarget is a display found on the machine that the inherited
// environment does not point at.
type displayTarget struct {
	kind helperKind
	// name is the value for WAYLAND_DISPLAY or DISPLAY.
	name string
	// runtimeDir accompanies a Wayland target when XDG_RUNTIME_DIR is missing,
	// since libwayland resolves a relative WAYLAND_DISPLAY against it.
	runtimeDir string
}

// writeHelpers lists the copy candidates in the order they are tried.
func writeHelpers() []clipboardHelper {
	return []clipboardHelper{
		{bin: wlCopy, kind: waylandHelper},
		{bin: xclip, args: []string{"-in", "-selection", "clipboard"}, kind: x11Helper},
		{bin: xsel, args: []string{"--input", "--clipboard"}, kind: x11Helper},
	}
}

// readHelpers lists the paste candidates in the order they are tried.
func readHelpers() []clipboardHelper {
	return []clipboardHelper{
		{bin: wlPaste, args: []string{"--no-newline"}, kind: waylandHelper},
		{bin: xclip, args: []string{"-out", "-selection", "clipboard"}, kind: x11Helper},
		{bin: xsel, args: []string{"--output", "--clipboard"}, kind: x11Helper},
	}
}

// writeAll hands text to the first clipboard helper that accepts it, returning
// why every attempt failed when none does.
func writeAll(text string) error {
	var failures []string
	for _, attempt := range writeAttempts() {
		diagnostic, err := writeViaHelper(attempt, text)
		if err != nil {
			failures = append(failures, attemptFailure(attempt, err, diagnostic))
			continue
		}
		return nil
	}
	return clipboardUnavailableError(failures)
}

// readAll returns the clipboard text from the first helper that can reach the
// session, returning why every attempt failed when none can.
func readAll() (string, error) {
	var failures []string
	for _, attempt := range readAttempts() {
		out, diagnostic, err := readViaHelper(attempt)
		if err != nil {
			failures = append(failures, attemptFailure(attempt, err, diagnostic))
			continue
		}
		return out, nil
	}
	return "", clipboardUnavailableError(failures)
}

// writeAttempts lists every copy worth trying, in order: the inherited
// environment first, which is all a normal desktop session needs, then the
// displays discovery found on the machine.
func writeAttempts() []clipboardAttempt {
	return helperAttempts(writeHelpers())
}

// readAttempts lists every paste worth trying, in the same order as a copy.
func readAttempts() []clipboardAttempt {
	return helperAttempts(readHelpers())
}

func helperAttempts(helpers []clipboardHelper) []clipboardAttempt {
	var available []clipboardHelper
	for _, helper := range helpers {
		if helperInstalled(helper.bin) {
			available = append(available, helper)
		}
	}
	if len(available) == 0 {
		return nil // nothing is installed, so no display is worth looking for
	}

	attempts := make([]clipboardAttempt, 0, len(available))
	for _, helper := range available {
		attempts = append(attempts, clipboardAttempt{helper: helper})
	}

	for _, target := range discoveredDisplays() {
		for _, helper := range available {
			if helper.kind != target.kind {
				continue
			}
			attempts = append(attempts, clipboardAttempt{
				helper: helper,
				env:    target.env(),
				label:  target.label(),
			})
		}
	}
	return attempts
}

// env returns the process environment pointed at the target's display, so the
// helper talks to the display discovery found rather than to its own guess.
func (t displayTarget) env() []string {
	if t.kind == waylandHelper {
		env := os.Environ()
		if t.runtimeDir != "" {
			env = withEnv(env, "XDG_RUNTIME_DIR", t.runtimeDir)
		}
		return withEnv(env, "WAYLAND_DISPLAY", t.name)
	}
	return withEnv(os.Environ(), "DISPLAY", t.name)
}

func (t displayTarget) label() string {
	if t.kind == waylandHelper {
		return "WAYLAND_DISPLAY=" + t.name
	}
	return "DISPLAY=" + t.name
}

// discoveredDisplays returns the displays found on the machine that the
// inherited environment does not already cover, Wayland first.
func discoveredDisplays() []displayTarget {
	return append(discoveredWaylandDisplays(), discoveredX11Displays()...)
}

// discoveredWaylandDisplays lists the wayland-* sockets in the user's runtime
// directory. wl-copy already falls back to wayland-0, so that name is skipped
// once the environment names a runtime directory to look in.
func discoveredWaylandDisplays() []displayTarget {
	runtimeDir := runtimeDir()
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return nil
	}

	fallbackCovered := os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("XDG_RUNTIME_DIR") != ""
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !isSocket(entry) || !strings.HasPrefix(name, "wayland-") {
			continue
		}
		if fallbackCovered && name == "wayland-0" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	targets := make([]displayTarget, 0, len(names))
	for _, name := range names[:min(len(names), maxDiscoveredDisplays)] {
		targets = append(targets, displayTarget{kind: waylandHelper, name: name, runtimeDir: runtimeDir})
	}
	return targets
}

// discoveredX11Displays lists the X displays published in x11SocketDir, lowest
// number first. A process without DISPLAY still usually has an X server to
// talk to: Xwayland keeps one on :0 for every Wayland session, including the
// gamescope session Bazzite boots into for Steam.
func discoveredX11Displays() []displayTarget {
	entries, err := os.ReadDir(x11SocketDir)
	if err != nil {
		return nil
	}

	inherited := os.Getenv("DISPLAY")
	var numbers []int
	for _, entry := range entries {
		number, ok := x11DisplayNumber(entry.Name())
		if !ok || !isSocket(entry) {
			continue
		}
		if ":"+strconv.Itoa(number) == inherited {
			continue // already tried with the environment as it is
		}
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)

	targets := make([]displayTarget, 0, len(numbers))
	for _, number := range numbers[:min(len(numbers), maxDiscoveredDisplays)] {
		targets = append(targets, displayTarget{kind: x11Helper, name: ":" + strconv.Itoa(number)})
	}
	return targets
}

// x11DisplayNumber parses the display number out of an X socket name ("X0" is
// DISPLAY=:0).
func x11DisplayNumber(name string) (int, bool) {
	digits, ok := strings.CutPrefix(name, "X")
	if !ok {
		return 0, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return number, true
}

// isSocket reports whether the directory entry is a unix socket. Stale sockets
// still qualify: the helper fails fast on them, and the attempt order decides
// which one is real.
func isSocket(entry os.DirEntry) bool {
	return entry.Type()&fs.ModeSocket != 0
}

// runtimeDir returns the user's runtime directory, falling back to the
// systemd-created /run/user/<uid> when XDG_RUNTIME_DIR is not exported, which is
// what happens over ssh and under sudo while the session sockets stay there.
func runtimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
}

// withEnv returns env with key set to value, replacing an existing entry.
func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

// writeViaHelper pipes text into a writing helper, returning the diagnostic it
// printed. stdout is not captured: only stderr explains a failure, and staying
// clear of it keeps the call free of a pipe the helper's daemon could hold open.
func writeViaHelper(attempt clipboardAttempt, text string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, attempt.helper.bin, attempt.helper.args...)
	cmd.Env = attempt.env
	cmd.Stdin = strings.NewReader(text)

	stderr := newStderrCapture(cmd)
	defer stderr.close()

	err := cmd.Run()
	return stderr.text(), helperRunError(ctx, err)
}

// readViaHelper runs a reading helper and returns the clipboard it printed
// together with the diagnostic it emitted.
func readViaHelper(attempt clipboardAttempt) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, attempt.helper.bin, attempt.helper.args...)
	cmd.Env = attempt.env

	stderr := newStderrCapture(cmd)
	defer stderr.close()

	out, err := cmd.Output()
	return string(out), stderr.text(), helperRunError(ctx, err)
}

// helperRunError reports a helper that ran past helperTimeout as a timeout
// instead of surfacing the signal it was killed with.
func helperRunError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s", helperTimeout)
	}
	return err
}

// stderrCapture collects a helper's diagnostics in a temporary file.
//
// A pipe cannot be used here. A successful wl-copy forks a daemon that inherits
// stderr and outlives the call, so the pipe would never report EOF and cmd.Wait
// would block forever on the copy goroutine. A file descriptor needs no
// goroutine, so the call always returns.
type stderrCapture struct {
	file *os.File
}

func newStderrCapture(cmd *exec.Cmd) stderrCapture {
	file, err := os.CreateTemp("", "surge-clipboard-stderr-")
	if err != nil {
		return stderrCapture{}
	}
	cmd.Stderr = file
	return stderrCapture{file: file}
}

// text returns what the helper wrote. Reading by name sees the bytes the
// already-exited helper flushed even while a forked daemon keeps the file open.
func (c stderrCapture) text() string {
	if c.file == nil {
		return ""
	}
	data, err := os.ReadFile(c.file.Name())
	if err != nil {
		return ""
	}
	return string(data)
}

func (c stderrCapture) close() {
	if c.file == nil {
		return
	}
	name := c.file.Name()
	_ = c.file.Close()
	_ = os.Remove(name)
}

// helperInstalled reports whether the helper can be resolved on PATH.
func helperInstalled(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// attemptFailure renders a failed helper call together with the diagnostic it
// printed and the display it was aimed at, which is what names the missing
// display for the user.
func attemptFailure(attempt clipboardAttempt, err error, diagnostic string) string {
	where := ""
	if attempt.label != "" {
		where = fmt.Sprintf(" (%s)", attempt.label)
	}

	detail := strings.Join(strings.Fields(diagnostic), " ")
	if detail == "" {
		return fmt.Sprintf("%s%s: %v", attempt.helper.bin, where, err)
	}
	return fmt.Sprintf("%s%s: %v (%s)", attempt.helper.bin, where, err, truncateRunes(detail, 96))
}

// clipboardUnavailableError explains why nothing could reach the clipboard
// instead of surfacing a helper's own opaque "exit status 1".
func clipboardUnavailableError(failures []string) error {
	if len(failures) == 0 {
		return errors.New("clipboard unavailable: install wl-clipboard (Wayland) or xclip/xsel (X11)")
	}
	if len(failures) > maxReportedFailures {
		hidden := len(failures) - maxReportedFailures
		failures = append(failures[:maxReportedFailures:maxReportedFailures],
			fmt.Sprintf("and %d more failed display(s)", hidden))
	}
	return fmt.Errorf("clipboard unavailable: %s", strings.Join(failures, "; "))
}

// truncateRunes shortens s to at most max runes without splitting a character.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
