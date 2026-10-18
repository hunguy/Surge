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

// systemServiceSettingsEnv marks a root system-service process. Unlike a
// manually launched root server, the service has no interactive session of its
// own and must resolve settings from the desktop user who configured Surge.
const systemServiceSettingsEnv = "SURGE_SYSTEM_SERVICE_SETTINGS"

// EnableSystemServiceSettings makes a root system service resolve settings from
// the primary desktop user's settings.toml instead of root's own file. The
// service entry point calls this before cobra loads settings.
func EnableSystemServiceSettings() {
	_ = os.Setenv(systemServiceSettingsEnv, "1")
}

// primaryUserSettingsPath returns the primary desktop user's settings.toml when
// this process is a root system service and the user has one. It returns "" so
// callers keep the process-scoped settings path for other modes.
func primaryUserSettingsPath() string {
	if strings.TrimSpace(os.Getenv(systemServiceSettingsEnv)) != "1" {
		return ""
	}
	home := PrimaryUserHome()
	if home == "" {
		return ""
	}
	for _, path := range userSettingsTomlCandidates(home) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// PrimaryUserHome returns the home directory of the primary interactive desktop
// user when this process is the root system service, or "" otherwise.
//
// The Surge system service runs as root and would otherwise resolve settings
// and download paths from root's account, ignoring the configuration the
// desktop user actually sees. Detection is best-effort: an explicit
// SURGE_SERVICE_USER override (username or absolute home path) wins, then the
// owner of the macOS console, then the owner of a /run/user/<uid> session
// directory, then the first regular account in /etc/passwd that owns a real home.
func PrimaryUserHome() string {
	if geteuidFn() != 0 {
		return ""
	}
	return primaryUserHomeFn()
}

// ServiceUserDownloadDir returns the primary desktop user's configured default
// download directory, falling back to their ~/Downloads or home directory. It
// returns "" when no desktop user can be determined. This lets the root system
// service honor the user's settings.toml instead of writing to its own home: on
// Linux to "/" (read-only on image-based distros) and on macOS to /var/root.
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
// Surge settings.toml. It returns "" when no candidate file is readable or none
// carries an absolute directory.
func readUserDefaultDownloadDir(home string) string {
	for _, path := range userSettingsTomlCandidates(home) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if dir := parseUserDefaultDownloadDir(data); dir != "" {
			return dir
		}
	}
	return ""
}

func parseUserDefaultDownloadDir(data []byte) string {
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

// userSettingsTomlCandidates lists the settings.toml locations Surge writes for a
// user home, most authoritative first: the XDG config home (honored on every
// platform when the user sets XDG_CONFIG_HOME, which is why it leads), then the
// macOS application-support directory, then the Windows roaming profile.
// Cross-platform paths that do not exist are simply skipped, so a macOS service
// does not need to know how the user launched their own Surge process.
func userSettingsTomlCandidates(home string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".config", "surge", "settings.toml"),
		filepath.Join(home, "Library", "Application Support", "surge", "settings.toml"),
		filepath.Join(home, "AppData", "Roaming", "surge", "settings.toml"),
	}
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

	if home := detectHomeFromConsole(); home != "" {
		return home
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
