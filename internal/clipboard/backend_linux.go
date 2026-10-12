//go:build linux && !android

package clipboard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
const (
	wlCopy  = "wl-copy"
	wlPaste = "wl-paste"
	xclip   = "xclip"
	xsel    = "xsel"
)

// clipboardHelper is one external program that can reach the session
// clipboard. Writing helpers consume the text on stdin; reading helpers print
// the clipboard on stdout.
type clipboardHelper struct {
	bin  string
	args []string
}

// writeHelpers lists the copy candidates in the order they are tried.
func writeHelpers() []clipboardHelper {
	return []clipboardHelper{
		{bin: wlCopy},
		{bin: xclip, args: []string{"-in", "-selection", "clipboard"}},
		{bin: xsel, args: []string{"--input", "--clipboard"}},
	}
}

// readHelpers lists the paste candidates in the order they are tried.
func readHelpers() []clipboardHelper {
	return []clipboardHelper{
		{bin: wlPaste, args: []string{"--no-newline"}},
		{bin: xclip, args: []string{"-out", "-selection", "clipboard"}},
		{bin: xsel, args: []string{"--output", "--clipboard"}},
	}
}

// writeAll hands text to the first clipboard helper that accepts it, returning
// why every installed candidate failed when none does.
func writeAll(text string) error {
	var failures []string
	for _, helper := range writeHelpers() {
		if !helperInstalled(helper.bin) {
			continue
		}

		diagnostic, err := writeViaHelper(helper, text)
		if err != nil {
			failures = append(failures, helperFailure(helper.bin, err, diagnostic))
			continue
		}
		return nil
	}
	return clipboardUnavailableError(failures)
}

// readAll returns the clipboard text from the first helper that can reach the
// session, returning why every installed candidate failed when none can.
func readAll() (string, error) {
	var failures []string
	for _, helper := range readHelpers() {
		if !helperInstalled(helper.bin) {
			continue
		}

		out, diagnostic, err := readViaHelper(helper)
		if err != nil {
			failures = append(failures, helperFailure(helper.bin, err, diagnostic))
			continue
		}
		return out, nil
	}
	return "", clipboardUnavailableError(failures)
}

// writeViaHelper pipes text into a writing helper, returning the diagnostic it
// printed. stdout is not captured: only stderr explains a failure, and staying
// clear of it keeps the call free of a pipe the helper's daemon could hold open.
func writeViaHelper(helper clipboardHelper, text string) (string, error) {
	cmd := exec.Command(helper.bin, helper.args...)
	cmd.Stdin = strings.NewReader(text)

	stderr := newStderrCapture(cmd)
	defer stderr.close()

	err := cmd.Run()
	return stderr.text(), err
}

// readViaHelper runs a reading helper and returns the clipboard it printed
// together with the diagnostic it emitted.
func readViaHelper(helper clipboardHelper) (string, string, error) {
	cmd := exec.Command(helper.bin, helper.args...)

	stderr := newStderrCapture(cmd)
	defer stderr.close()

	out, err := cmd.Output()
	return string(out), stderr.text(), err
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

// helperFailure renders a helper's exit error together with the diagnostic it
// printed, which is what names the missing display for the user.
func helperFailure(bin string, err error, diagnostic string) string {
	detail := strings.Join(strings.Fields(diagnostic), " ")
	if detail == "" {
		return fmt.Sprintf("%s: %v", bin, err)
	}
	return fmt.Sprintf("%s: %v (%s)", bin, err, truncateRunes(detail, 96))
}

// clipboardUnavailableError explains why nothing could reach the clipboard
// instead of surfacing a helper's own opaque "exit status 1".
func clipboardUnavailableError(failures []string) error {
	if len(failures) == 0 {
		return errors.New("clipboard unavailable: install wl-clipboard (Wayland) or xclip/xsel (X11)")
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
