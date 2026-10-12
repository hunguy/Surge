//go:build darwin

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func withConsoleDevice(t *testing.T, path string) {
	t.Helper()
	orig := darwinConsoleDevice
	darwinConsoleDevice = path
	t.Cleanup(func() { darwinConsoleDevice = orig })
}

func TestDetectHomeFromConsole_OwnedByRegularUser(t *testing.T) {
	// A file owned by the test user stands in for /dev/console: the console
	// owner is what the LaunchDaemon must read settings from.
	device := filepath.Join(t.TempDir(), "console")
	if err := os.WriteFile(device, nil, 0o644); err != nil {
		t.Fatalf("create console fixture: %v", err)
	}
	withConsoleDevice(t, device)

	uid, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	if id, err := strconv.Atoi(uid.Uid); err != nil || id < darwinFirstRegularUID {
		t.Skipf("test account %s is a system account", uid.Uid)
	}

	got := detectHomeFromConsole()
	if got != uid.HomeDir {
		t.Fatalf("detectHomeFromConsole() = %q, want the console owner's home %q", got, uid.HomeDir)
	}
}

func TestDetectHomeFromConsole_SystemOwnerIsIgnored(t *testing.T) {
	// /dev/null is root-owned on macOS, which is what /dev/console looks like
	// when nobody has an active GUI session. Returning root's home there would
	// put downloads back in /var/root.
	info, err := os.Stat("/dev/null")
	if err != nil {
		t.Skipf("cannot stat /dev/null: %v", err)
	}
	if info.Sys() == nil {
		t.Skip("no ownership information for /dev/null")
	}
	withConsoleDevice(t, "/dev/null")

	if got := detectHomeFromConsole(); got != "" {
		t.Fatalf("detectHomeFromConsole() = %q, want empty for a system-owned console", got)
	}
}

func TestDetectHomeFromConsole_MissingDevice(t *testing.T) {
	withConsoleDevice(t, filepath.Join(t.TempDir(), "does-not-exist"))

	if got := detectHomeFromConsole(); got != "" {
		t.Fatalf("detectHomeFromConsole() = %q, want empty when the device is missing", got)
	}
}
