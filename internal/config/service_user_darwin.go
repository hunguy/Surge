//go:build darwin

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// darwinFirstRegularUID is the lowest uid macOS assigns to a real account.
// Lower ids are system accounts (root is 0, _mbsetupuser, loginwindow, ...)
// whose home directories must never be used as a download destination.
const darwinFirstRegularUID = 501

// darwinConsoleDevice is the device owned by the user with the active GUI
// session. It is a variable so tests can point it at a fixture.
var darwinConsoleDevice = "/dev/console"

// detectHomeFromConsole returns the home directory of the macOS console user.
//
// Surge's system service is a LaunchDaemon: it has no session of its own and
// inherits no XDG environment, so /run/user/<uid> and /etc/passwd carry no
// interactive account on macOS. The owner of /dev/console is the logged-in
// desktop user, which is exactly whose settings and Downloads folder the
// service is supposed to honor. It returns "" when nobody is logged in at the
// console, so the caller keeps its existing fallbacks.
func detectHomeFromConsole() string {
	info, err := os.Stat(darwinConsoleDevice)
	if err != nil {
		return ""
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid < darwinFirstRegularUID {
		return ""
	}

	u, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return ""
	}

	home := strings.TrimSpace(u.HomeDir)
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return home
}
