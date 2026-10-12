//go:build linux && !android

package clipboard

import (
	"os"
	"path/filepath"
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

func TestClipboardHelpers_TryWaylandBeforeX11(t *testing.T) {
	if got := writeHelpers()[0].bin; got != wlCopy {
		t.Fatalf("first copy helper = %q, want %q", got, wlCopy)
	}
	if got := readHelpers()[0].bin; got != wlPaste {
		t.Fatalf("first paste helper = %q, want %q", got, wlPaste)
	}
}
