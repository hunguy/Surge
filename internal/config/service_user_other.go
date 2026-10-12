//go:build !darwin

package config

// detectHomeFromConsole has no meaning off macOS: Linux exposes the desktop
// session through /run/user/<uid> and /etc/passwd, and Windows has no console
// device to inspect, so primary-user detection stays platform-specific there.
func detectHomeFromConsole() string { return "" }
