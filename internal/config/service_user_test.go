package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestSystemServiceLoadsAllPrimaryUserSettings(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })
	t.Setenv(systemServiceSettingsEnv, "1")

	userPath := filepath.Join(home, ".config", "surge", "settings.toml")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatalf("mkdir user settings dir: %v", err)
	}
	body := `[network]
max_concurrent_downloads = 1
max_concurrent_probes = 1
max_connections_per_host = 4

[general]
default_download_dir = '/home/tester/Downloads'

[performance]
max_task_retries = 7
`
	if err := os.WriteFile(userPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write user settings: %v", err)
	}

	if got := GetSettingsPath(); got != userPath {
		t.Fatalf("GetSettingsPath() = %q, want user settings %q", got, userPath)
	}

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if got := Resolve[int](settings.Network.MaxConcurrentDownloads); got != 1 {
		t.Errorf("max_concurrent_downloads = %d, want 1", got)
	}
	if got := Resolve[int](settings.Network.MaxConcurrentProbes); got != 1 {
		t.Errorf("max_concurrent_probes = %d, want 1", got)
	}
	if got := Resolve[int](settings.Network.MaxConnectionsPerDownload); got != 4 {
		t.Errorf("max_connections_per_host = %d, want 4", got)
	}
	if got := Resolve[int](settings.Performance.MaxTaskRetries); got != 7 {
		t.Errorf("max_task_retries = %d, want 7", got)
	}
}

func TestSystemServiceSavesToPrimaryUserSettings(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })
	t.Setenv(systemServiceSettingsEnv, "1")

	userPath := filepath.Join(home, ".config", "surge", "settings.toml")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatalf("mkdir user settings dir: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("[network]\nmax_concurrent_downloads = 1\n"), 0o640); err != nil {
		t.Fatalf("write user settings: %v", err)
	}

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	settings.Network.MaxConcurrentDownloads.Value = 2
	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings() error: %v", err)
	}

	if info, err := os.Stat(userPath); err == nil && runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o640 {
			t.Errorf("settings mode = %o, want 640 after root service save", got)
		}
	}

	// Ensure the root service writes the user's file, not root's own config.
	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("read user settings after save: %v", err)
	}
	if !strings.Contains(string(data), "max_concurrent_downloads = 2") {
		t.Fatalf("user settings did not receive the save:\n%s", data)
	}
}

func TestRootWithoutServiceModeKeepsProcessSettingsPath(t *testing.T) {
	forceRootEuid(t)
	home := t.TempDir()
	orig := primaryUserHomeFn
	primaryUserHomeFn = func() string { return home }
	t.Cleanup(func() { primaryUserHomeFn = orig })
	t.Setenv(systemServiceSettingsEnv, "")

	if got, want := GetSettingsPath(), filepath.Join(GetSurgeDir(), "settings.toml"); got != want {
		t.Fatalf("GetSettingsPath() = %q, want process settings %q", got, want)
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
