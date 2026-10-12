package cmd

import (
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestResolveOutputDir_PrefersExplicitPath(t *testing.T) {
	settings := config.DefaultSettings()

	got := resolveOutputDir("/explicit/dir", false, "", settings)
	if got != "/explicit/dir" {
		t.Fatalf("resolveOutputDir = %q, want the explicit path", got)
	}
}

func TestResolveOutputDir_PrefersOutputFlag(t *testing.T) {
	settings := config.DefaultSettings()

	got := resolveOutputDir("", false, "/from/flag", settings)
	if got != "/from/flag" {
		t.Fatalf("resolveOutputDir = %q, want %q", got, "/from/flag")
	}
}

func TestResolveOutputDir_UsesConfiguredDefault(t *testing.T) {
	settings := &config.Settings{}
	settings.General.DefaultDownloadDir = &config.Setting{Value: "/configured/dir"}

	got := resolveOutputDir("", false, "", settings)
	if got != "/configured/dir" {
		t.Fatalf("resolveOutputDir = %q, want configured default", got)
	}
}

// A systemd service runs with cwd "/", which is read-only on image-based
// distros. resolveOutputDir must never hand back the working directory (".", or
// "/" after EnsureAbsPath) when no destination is configured.
func TestResolveOutputDir_NeverFallsBackToCwd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := resolveOutputDir("", false, "", &config.Settings{})
	if got == "." || got == "/" {
		t.Fatalf("resolveOutputDir = %q, want a real directory, not the working directory", got)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("resolveOutputDir = %q, want an absolute path", got)
	}
}

func TestResolveOutputDir_RelativeJoinsFallbackBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := resolveOutputDir("subdir", true, "", &config.Settings{})
	if got == "subdir" || !filepath.IsAbs(got) {
		t.Fatalf("resolveOutputDir = %q, want an absolute path under a base dir", got)
	}
	if filepath.Base(got) != "subdir" {
		t.Fatalf("resolveOutputDir = %q, want it to end in subdir", got)
	}
}
