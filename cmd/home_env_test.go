//go:build !android

package cmd

import (
	"os"
	"os/user"
	"runtime"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
)

// A systemd system service starts without HOME; ensureUserHomeEnv must restore
// it from the account database and refresh the XDG paths so config/state do not
// resolve under "/" (read-only on image-based distros).
func TestEnsureUserHomeEnvSetsHomeWhenMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix home semantics")
	}

	origHome, hadHome := os.LookupEnv("HOME")
	origConfig, hadConfig := os.LookupEnv("XDG_CONFIG_HOME")
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", origHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if hadConfig {
			_ = os.Setenv("XDG_CONFIG_HOME", origConfig)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		xdg.Reload()
	})

	u, err := user.Current()
	if err != nil || strings.TrimSpace(u.HomeDir) == "" {
		t.Skip("no account home directory for current user")
	}

	_ = os.Unsetenv("HOME")
	_ = os.Unsetenv("XDG_CONFIG_HOME")

	ensureUserHomeEnv()

	assert.Equal(t, u.HomeDir, os.Getenv("HOME"))
	assert.True(t, strings.HasPrefix(xdg.ConfigHome, u.HomeDir),
		"xdg.ConfigHome = %q, want it under the account home %q", xdg.ConfigHome, u.HomeDir)
}

func TestEnsureUserHomeEnvKeepsExistingHome(t *testing.T) {
	origHome, hadHome := os.LookupEnv("HOME")
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", origHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	want := t.TempDir()
	_ = os.Setenv("HOME", want)

	ensureUserHomeEnv()

	assert.Equal(t, want, os.Getenv("HOME"), "an existing absolute HOME must be left untouched")
}
