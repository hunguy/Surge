//go:build !unix

package config

import "os"

// preserveFileMetadata copies permissions where the platform supports them.
// Ownership is not preserved on non-Unix platforms because the service does not
// resolve settings from another user there.
func preserveFileMetadata(tempPath, targetPath string) {
	if info, err := os.Stat(targetPath); err == nil {
		_ = os.Chmod(tempPath, info.Mode().Perm())
	}
}
