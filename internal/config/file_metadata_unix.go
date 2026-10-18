//go:build unix

package config

import (
	"os"
	"syscall"
)

// preserveFileMetadata copies ownership and permissions from an existing
// settings file onto its replacement temp file. A root system service writes to
// the desktop user's settings.toml, and an atomic rename would otherwise leave
// that file owned by root.
func preserveFileMetadata(tempPath, targetPath string) {
	info, err := os.Stat(targetPath)
	if err != nil {
		return
	}
	_ = os.Chmod(tempPath, info.Mode().Perm())
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(tempPath, int(stat.Uid), int(stat.Gid))
	}
}
