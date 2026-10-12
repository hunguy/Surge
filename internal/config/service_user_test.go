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

func writeUserSettingsToml(t *testing.T, home, generalBody string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "surge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	content := "[general]\n" + generalBody
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write settings.toml: %v", err)
	}
}
