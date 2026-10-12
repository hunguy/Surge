package config

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// primaryUserHomeFn and geteuidFn are injectable so tests can simulate the
// desktop user and root privileges without depending on the host session.
var (
	primaryUserHomeFn = detectPrimaryUserHome
	geteuidFn         = os.Geteuid
)

// PrimaryUserHome returns the home directory of the primary interactive desktop
// user when this process is the root system service, or "" otherwise.
//
// The Surge system service runs as root and would otherwise resolve settings
// and download paths from root's account, ignoring the configuration the
// desktop user actually sees. Detection is best-effort: an explicit
// SURGE_SERVICE_USER override (username or absolute home path) wins, then the
// owner of a /run/user/<uid> session directory, then the first regular account
// in /etc/passwd that owns a real home.
func PrimaryUserHome() string {
	if geteuidFn() != 0 {
		return ""
	}
	return primaryUserHomeFn()
}

// ServiceUserDownloadDir returns the primary desktop user's configured default
// download directory, falling back to their ~/Downloads or home directory. It
// returns "" when no desktop user can be determined. This lets the root system
// service honor the user's settings.toml instead of writing to its own home (or
// worse, to "/" when none is configured).
func ServiceUserDownloadDir() string {
	home := PrimaryUserHome()
	if home == "" {
		return ""
	}
	if dir := readUserDefaultDownloadDir(home); dir != "" {
		return dir
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads
	}
	return home
}

// readUserDefaultDownloadDir reads general.default_download_dir from the user's
// Surge settings.toml. It returns "" when the file is missing, unreadable, or
// does not carry an absolute directory.
func readUserDefaultDownloadDir(home string) string {
	path := filepath.Join(home, ".config", "surge", "settings.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var parsed struct {
		General struct {
			DefaultDownloadDir string `toml:"default_download_dir"`
		} `toml:"general"`
	}
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return ""
	}

	dir := strings.TrimSpace(parsed.General.DefaultDownloadDir)
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Clean(dir)
}

func detectPrimaryUserHome() string {
	// Explicit override: a username, or an absolute home directory.
	if override := strings.TrimSpace(os.Getenv("SURGE_SERVICE_USER")); override != "" {
		if filepath.IsAbs(override) {
			return override
		}
		if u, err := user.Lookup(override); err == nil && strings.TrimSpace(u.HomeDir) != "" {
			return u.HomeDir
		}
		return ""
	}

	if home := detectHomeFromRuntimeDir(); home != "" {
		return home
	}
	return detectHomeFromPasswd()
}

// detectHomeFromRuntimeDir picks the lowest regular uid that owns a
// /run/user/<uid> session directory, which pam_systemd creates for the logged-in
// desktop user. This is the most reliable signal on modern Linux systems.
func detectHomeFromRuntimeDir() string {
	if runtime.GOOS == "windows" {
		return ""
	}

	entries, err := os.ReadDir(filepath.Join(string(filepath.Separator), "run", "user"))
	if err != nil {
		return ""
	}

	bestUID := -1
	for _, entry := range entries {
		uid, err := strconv.Atoi(entry.Name())
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}
		if bestUID == -1 || uid < bestUID {
			bestUID = uid
		}
	}
	if bestUID == -1 {
		return ""
	}

	u, err := user.LookupId(strconv.Itoa(bestUID))
	if err != nil || strings.TrimSpace(u.HomeDir) == "" {
		return ""
	}
	return u.HomeDir
}

// detectHomeFromPasswd is the fallback for headless hosts without a session
// directory: the first regular account (uid 1000-65533) that owns a real home.
func detectHomeFromPasswd() string {
	if runtime.GOOS == "windows" {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(string(filepath.Separator), "etc", "passwd"))
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 6 {
			continue
		}
		uid, err := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}
		home := strings.TrimSpace(fields[5])
		if home == "" {
			continue
		}
		if info, err := os.Stat(home); err == nil && info.IsDir() {
			return home
		}
	}
	return ""
}
