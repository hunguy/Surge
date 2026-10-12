package config

import (
	"os"
	"path/filepath"
	"testing"
)

// These tests mutate package-level seams, so they must not run in parallel.

func forceRootEuid(t *testing.T) {
	t.Helper()
	orig := geteuidFn
	geteuidFn = func() int { return 0 }
	t.Cleanup(func() { geteuidFn = orig })
}

func TestPrimaryUserHome_NotRoot(t *testing.T) {
	orig := geteuidFn
	geteuidFn = func() int { return 1000 }
	t.Cleanup(func() { geteuidFn = orig })

	if got := PrimaryUserHome(); got != "" {
		t.Fatalf("PrimaryUserHome() = %q, want empty when not root", got)
	}
}

func TestPrimaryUserHome_EnvOverride(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	t.Setenv("SURGE_SERVICE_USER", home)

	if got := PrimaryUserHome(); got != home {
		t.Fatalf("PrimaryUserHome() = %q, want overridden home %q", got, home)
	}
}

func TestPrimaryUserHome_EnvOverrideUsername(t *testing.T) {
	forceRootEuid(t)
	// "root" always resolves through the account database.
	if _, err := os.Stat("/root"); err != nil {
		t.Skip("no /root account to look up")
	}
	t.Setenv("SURGE_SERVICE_USER", "root")

	if got := PrimaryUserHome(); got == "" {
		t.Fatal("PrimaryUserHome() = empty, want root home from account database")
	}
}

func TestServiceUserDownloadDir_ReadsUserSettings(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })

	// No settings file and no ~/Downloads: fall back to the home directory.
	if got := ServiceUserDownloadDir(); got != home {
		t.Fatalf("ServiceUserDownloadDir() = %q, want home %q", got, home)
	}

	// A configured absolute directory wins over everything else.
	target := filepath.Join(home, "Emulation", "roms", "ps3")
	writeUserSettingsToml(t, home, "default_download_dir = '"+target+"'\n")
	if got := ServiceUserDownloadDir(); got != target {
		t.Fatalf("ServiceUserDownloadDir() = %q, want configured %q", got, target)
	}

	// A relative value is rejected; with no ~/Downloads the home is returned.
	writeUserSettingsToml(t, home, "default_download_dir = 'relative/path'\n")
	if got := ServiceUserDownloadDir(); got != home {
		t.Fatalf("ServiceUserDownloadDir() = %q, want home %q for relative config", got, home)
	}

	// ~/Downloads is the fallback when settings carry no directory.
	if err := os.Remove(filepath.Join(home, ".config", "surge", "settings.toml")); err != nil {
		t.Fatalf("remove settings: %v", err)
	}
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir Downloads: %v", err)
	}
	if got := ServiceUserDownloadDir(); got != downloads {
		t.Fatalf("ServiceUserDownloadDir() = %q, want %q", got, downloads)
	}
}

func TestServiceUserDownloadDir_ReadsMacOSApplicationSupport(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })

	// A LaunchDaemon has no XDG environment, so the canonical macOS location is
	// where the desktop user's own Surge process writes its settings.
	target := filepath.Join(home, "Movies")
	writeUserSettingsTomlAt(t, filepath.Join(home, "Library", "Application Support", "surge"),
		"default_download_dir = '"+target+"'\n")

	if got := ServiceUserDownloadDir(); got != target {
		t.Fatalf("ServiceUserDownloadDir() = %q, want macOS application-support setting %q", got, target)
	}
}

func TestServiceUserDownloadDir_PrefersXDGConfigHome(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })

	// XDG_CONFIG_HOME overrides the platform default for the user's own Surge
	// process, so the daemon must read the same file it does.
	xdgTarget := filepath.Join(home, "Documents")
	macTarget := filepath.Join(home, "Movies")
	writeUserSettingsTomlAt(t, filepath.Join(home, ".config", "surge"),
		"default_download_dir = '"+xdgTarget+"'\n")
	writeUserSettingsTomlAt(t, filepath.Join(home, "Library", "Application Support", "surge"),
		"default_download_dir = '"+macTarget+"'\n")

	if got := ServiceUserDownloadDir(); got != xdgTarget {
		t.Fatalf("ServiceUserDownloadDir() = %q, want XDG config setting %q", got, xdgTarget)
	}
}

func TestReadUserDefaultDownloadDir_IgnoresUnreadableOrRelativeSettings(t *testing.T) {
	home := t.TempDir()

	// Legacy settings.json is no longer honored by Surge, so a stale value in it
	// must not decide where the service writes downloads.
	legacy := filepath.Join(home, "Library", "Application Support", "surge")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	stale := []byte(`{"general":{"default_download_dir":"/Volumes/Stale"}}`)
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), stale, 0o644); err != nil {
		t.Fatalf("write legacy settings.json: %v", err)
	}
	if got := readUserDefaultDownloadDir(home); got != "" {
		t.Fatalf("readUserDefaultDownloadDir() = %q, want empty for legacy settings.json", got)
	}

	// A relative value is rejected rather than resolved against the daemon cwd.
	writeUserSettingsTomlAt(t, filepath.Join(home, ".config", "surge"),
		"default_download_dir = 'Downloads/from-browser'\n")
	if got := readUserDefaultDownloadDir(home); got != "" {
		t.Fatalf("readUserDefaultDownloadDir() = %q, want empty for relative path", got)
	}

	if got := readUserDefaultDownloadDir(""); got != "" {
		t.Fatalf("readUserDefaultDownloadDir(\"\") = %q, want empty", got)
	}
}

func TestUserSettingsTomlCandidates(t *testing.T) {
	home := "/home/tester"
	got := userSettingsTomlCandidates(home)
	want := []string{
		filepath.Join(home, ".config", "surge", "settings.toml"),
		filepath.Join(home, "Library", "Application Support", "surge", "settings.toml"),
		filepath.Join(home, "AppData", "Roaming", "surge", "settings.toml"),
	}
	if len(got) != len(want) {
		t.Fatalf("userSettingsTomlCandidates() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("userSettingsTomlCandidates()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if candidates := userSettingsTomlCandidates("  "); candidates != nil {
		t.Fatalf("userSettingsTomlCandidates(blank) = %v, want nil", candidates)
	}
}

func writeUserSettingsToml(t *testing.T, home, generalBody string) {
	t.Helper()
	writeUserSettingsTomlAt(t, filepath.Join(home, ".config", "surge"), generalBody)
}

func writeUserSettingsTomlAt(t *testing.T, dir, generalBody string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	content := "[general]\n" + generalBody
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write settings.toml: %v", err)
	}
}
