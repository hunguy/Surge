//go:build linux && !android

package clipboard

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// standInHelper writes an executable stand-in for a clipboard helper into dir.
// The body must stick to shell builtins so the tests do not depend on PATH,
// which they point at dir alone.
func standInHelper(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stand-in %s: %v", name, err)
	}
}

// recordingHelperBody records the first line it receives on stdin as a copy
// command, ignoring any arguments (xclip and xsel are called with flags).
func recordingHelperBody(dest string) string {
	return "read -r line\nprintf '%s' \"$line\" > " + dest + "\n"
}

// failingHelperBody exits non-zero after printing what the real helper prints
// when it cannot reach the session.
func failingHelperBody(message string) string {
	return "printf '%s\\n' \"" + message + "\" >&2\nexit 1\n"
}

// onlyOnPath restricts PATH to dir so only the stand-ins can be resolved.
func onlyOnPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir)
}

// socketFixture publishes a unix socket named name in dir, the way a display
// server announces itself, and keeps it alive for the test.
func socketFixture(t *testing.T, dir, name string) {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("listen on %s: %v", name, err)
	}
	t.Cleanup(func() { _ = listener.Close() })
}

// withX11SocketDir points X display discovery at dir for the test.
func withX11SocketDir(t *testing.T, dir string) {
	t.Helper()
	original := x11SocketDir
	x11SocketDir = dir
	t.Cleanup(func() { x11SocketDir = original })
}

// withoutDisplayEnv makes the process look like one started with no session
// environment at all, which is where discovery has to earn its keep.
func withoutDisplayEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
}

func TestWriteAll_FallsBackToXclipWhenWaylandHelperFails(t *testing.T) {
	dir := t.TempDir()
	copied := filepath.Join(dir, "copied.txt")
	standInHelper(t, dir, wlCopy, failingHelperBody("Failed to connect to a Wayland server"))
	standInHelper(t, dir, xclip, recordingHelperBody(copied))
	onlyOnPath(t, dir)

	const url = "https://example.com/file.zip"
	if err := writeAll(url); err != nil {
		t.Fatalf("writeAll() = %v, want nil after falling back to xclip", err)
	}

	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("xclip stand-in did not run: %v", err)
	}
	if string(got) != url {
		t.Fatalf("xclip received %q, want %q", got, url)
	}
}

func TestWriteAll_PrefersWaylandHelperWithoutWaylandDisplayEnv(t *testing.T) {
	dir := t.TempDir()
	copied := filepath.Join(dir, "copied.txt")
	standInHelper(t, dir, wlCopy, recordingHelperBody(copied))
	standInHelper(t, dir, xclip, failingHelperBody("Error: Can't open display: (null)"))
	onlyOnPath(t, dir)
	// The remaining drop-down of a session-less process: no Wayland display in
	// the environment, but wl-copy can still reach $XDG_RUNTIME_DIR/wayland-0.
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	const url = "https://example.com/wayland.zip"
	if err := writeAll(url); err != nil {
		t.Fatalf("writeAll() = %v, want nil via wl-copy", err)
	}

	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("wl-copy stand-in did not run: %v", err)
	}
	if string(got) != url {
		t.Fatalf("wl-copy received %q, want %q", got, url)
	}
}

func TestWriteAll_ReportsHelperDiagnosticsWhenEveryHelperFails(t *testing.T) {
	dir := t.TempDir()
	standInHelper(t, dir, wlCopy, failingHelperBody("Failed to connect to a Wayland server"))
	standInHelper(t, dir, xclip, failingHelperBody("Error: Can't open display: (null)"))
	onlyOnPath(t, dir)

	err := writeAll("https://example.com/file.zip")
	if err == nil {
		t.Fatal("writeAll() = nil, want an error when no helper can reach the session")
	}

	msg := err.Error()
	for _, want := range []string{wlCopy, xclip, "Can't open display"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not mention %q", msg, want)
		}
	}
}

func TestWriteAll_HintsMissingHelper(t *testing.T) {
	onlyOnPath(t, t.TempDir())

	err := writeAll("https://example.com/file.zip")
	if err == nil {
		t.Fatal("writeAll() = nil, want an error when no helper is installed")
	}
	if !strings.Contains(err.Error(), "wl-clipboard") {
		t.Fatalf("error %q does not tell the user which package to install", err)
	}
}

func TestReadAll_FallsBackToXclipWhenWaylandHelperFails(t *testing.T) {
	dir := t.TempDir()
	standInHelper(t, dir, wlPaste, failingHelperBody("Nothing is copied"))
	standInHelper(t, dir, xclip, "printf '%s' 'https://example.com/from-xclip.zip'\n")
	onlyOnPath(t, dir)

	got, err := readAll()
	if err != nil {
		t.Fatalf("readAll() = %v, want nil after falling back to xclip", err)
	}
	if want := "https://example.com/from-xclip.zip"; got != want {
		t.Fatalf("readAll() = %q, want %q", got, want)
	}
}

func TestReadAll_HintsMissingHelper(t *testing.T) {
	onlyOnPath(t, t.TempDir())

	if _, err := readAll(); err == nil {
		t.Fatal("readAll() = nil, want an error when no helper is installed")
	} else if !strings.Contains(err.Error(), "xclip/xsel") {
		t.Fatalf("error %q does not tell the user which package to install", err)
	}
}

// TestWriteAll_DoesNotBlockOnHelperDaemon guards the deadlock the first
// implementation hit: a successful wl-copy forks a daemon that inherits stderr
// and outlives the call, so collecting diagnostics in an in-process pipe made
// cmd.Wait block forever.
func TestWriteAll_DoesNotBlockOnHelperDaemon(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("/bin/sleep is unavailable; cannot fork a stand-in daemon")
	}

	dir := t.TempDir()
	copied := filepath.Join(dir, "copied.txt")
	// The background sleep outlives the helper while inheriting its stderr, the
	// way a successful wl-copy leaves a daemon behind.
	standInHelper(t, dir, wlCopy,
		"read -r line\nprintf '%s' \"$line\" > "+copied+"\n/bin/sleep 3 &\n")
	onlyOnPath(t, dir)

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- writeAll("https://example.com/daemon.zip") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writeAll() = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writeAll() blocked on a helper that forked a daemon holding stderr")
	}

	// Waiting on the daemon to release stderr means the diagnostics were read
	// through a pipe again, which hangs forever against a real wl-copy daemon.
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("writeAll() took %v, so it waited for the helper's daemon", elapsed)
	}

	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("wl-copy stand-in did not run: %v", err)
	}
	if string(got) != "https://example.com/daemon.zip" {
		t.Fatalf("wl-copy received %q", got)
	}
}

// TestWriteAll_RetriesDiscoveredX11Display covers the machine whose session is
// not reachable from the inherited environment: no WAYLAND_DISPLAY (so wl-copy
// looks for a wayland-0 that does not exist, as gamescope publishes
// gamescope-0) and no DISPLAY (so xclip cannot open a display either), while an
// X server listening on :7 does hold the session clipboard.
func TestWriteAll_RetriesDiscoveredX11Display(t *testing.T) {
	dir := t.TempDir()
	x11Dir := t.TempDir()
	socketFixture(t, x11Dir, "X7")

	copied := filepath.Join(dir, "copied.txt")
	standInHelper(t, dir, wlCopy, failingHelperBody("Failed to connect to a Wayland server"))
	// The stand-in fails without a display, the way xclip does, and records the
	// display discovery sent it.
	xclipBody := "if [ -z \"$DISPLAY\" ]; then\n" +
		"printf '%s\\n' \"Error: Can't open display: (null)\" >&2\n" +
		"exit 1\n" +
		"fi\n" +
		"read -r line\n" +
		"printf '%s %s' \"$line\" \"$DISPLAY\" > " + copied + "\n"
	standInHelper(t, dir, xclip, xclipBody)
	onlyOnPath(t, dir)

	withoutDisplayEnv(t)
	// An empty runtime directory keeps Wayland discovery out of the way.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	withX11SocketDir(t, x11Dir)

	const url = "https://example.com/x11.zip"
	if err := writeAll(url); err != nil {
		t.Fatalf("writeAll() = %v, want nil via the discovered X display", err)
	}

	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("xclip stand-in did not run: %v", err)
	}
	if want := url + " :7"; string(got) != want {
		t.Fatalf("xclip received %q, want %q", got, want)
	}
}

// TestWriteAll_RetriesDiscoveredWaylandDisplay covers the other half of
// discovery: a wayland-N socket the helper would never guess, because wl-copy
// only falls back to wayland-0.
func TestWriteAll_RetriesDiscoveredWaylandDisplay(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := t.TempDir()
	socketFixture(t, runtimeDir, "wayland-3")

	copied := filepath.Join(dir, "copied.txt")
	// The stand-in fails until discovery names the socket to connect to, the way
	// wl-copy fails when it can only reach wayland-0.
	body := "if [ -z \"$WAYLAND_DISPLAY\" ]; then\n" +
		"printf '%s\\n' 'Failed to connect to a Wayland server' >&2\n" +
		"exit 1\n" +
		"fi\n" +
		"read -r line\n" +
		"printf '%s %s' \"$line\" \"$WAYLAND_DISPLAY\" > " + copied + "\n"
	standInHelper(t, dir, wlCopy, body)
	onlyOnPath(t, dir)

	withoutDisplayEnv(t)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	withX11SocketDir(t, t.TempDir())

	const url = "https://example.com/wayland.zip"
	if err := writeAll(url); err != nil {
		t.Fatalf("writeAll() = %v, want nil via the discovered Wayland display", err)
	}

	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("wl-copy stand-in did not run: %v", err)
	}
	if want := url + " wayland-3"; string(got) != want {
		t.Fatalf("wl-copy received %q, want %q", got, want)
	}
}

// TestWriteAttempts_SkipsDisplayCoveredByTheEnvironment keeps discovery from
// repeating a call that already failed: wl-copy falls back to
// $XDG_RUNTIME_DIR/wayland-0 on its own, so that socket is only worth a second
// look when XDG_RUNTIME_DIR itself was missing.
func TestWriteAttempts_SkipsDisplayCoveredByTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	standInHelper(t, dir, wlCopy, "exit 0\n")
	onlyOnPath(t, dir)

	runtimeDir := t.TempDir()
	socketFixture(t, runtimeDir, "wayland-0")
	socketFixture(t, runtimeDir, "wayland-3")

	withoutDisplayEnv(t)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	withX11SocketDir(t, t.TempDir())

	var labels []string
	for _, attempt := range writeAttempts() {
		if attempt.label != "" {
			labels = append(labels, attempt.label)
		}
	}

	want := []string{"WAYLAND_DISPLAY=wayland-3"}
	if !slices.Equal(labels, want) {
		t.Fatalf("discovery labels = %v, want %v", labels, want)
	}
}

// TestWriteAll_TimesOutHelperThatNeverReturns guards the TUI keypress path: a
// compositor that accepts the connection without serving the clipboard leaves
// wl-copy waiting forever, which used to hang the update loop.
func TestWriteAll_TimesOutHelperThatNeverReturns(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("/bin/sleep is unavailable; cannot hold a helper open")
	}

	dir := t.TempDir()
	standInHelper(t, dir, wlCopy, "printf '%s\\n' 'no display' >&2\n/bin/sleep 10\n")
	onlyOnPath(t, dir)

	withoutDisplayEnv(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	withX11SocketDir(t, t.TempDir())

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- writeAll("https://example.com/hang.zip") }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("writeAll() = nil, want an error when the helper never returns")
		}
		msg := err.Error()
		if !strings.Contains(msg, "timed out") || !strings.Contains(msg, wlCopy) {
			t.Fatalf("writeAll() = %q, want a %s timeout", msg, wlCopy)
		}
	case <-time.After(2 * helperTimeout):
		t.Fatal("writeAll() blocked on a helper that never returned")
	}

	if elapsed := time.Since(start); elapsed > 2*helperTimeout {
		t.Fatalf("writeAll() took %v, want it bounded by the %s timeout", elapsed, helperTimeout)
	}
}

func TestClipboardHelpers_TryWaylandBeforeX11(t *testing.T) {
	if got := writeHelpers()[0].bin; got != wlCopy {
		t.Fatalf("first copy helper = %q, want %q", got, wlCopy)
	}
	if got := readHelpers()[0].bin; got != wlPaste {
		t.Fatalf("first paste helper = %q, want %q", got, wlPaste)
	}
}
