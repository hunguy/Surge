package scheduler

import (
	"fmt"
	"os"
	"strings"

	"github.com/SurgeDM/Surge/internal/integrity"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// verifyCompletedDownload validates the finished working file before the
// lifecycle worker promotes it to its final name.
//
// The structural checks (byte count and chunk coverage) always run: a file
// that is short, or that the engine never fully wrote, must never be reported
// as complete. The digest comparison is optional because most origins do not
// advertise one; when a checksum was captured at probe time it proves the
// bytes match what the server described.
func verifyCompletedDownload(cfg *types.DownloadRecord, progState *progress.DownloadProgress, destPath string, expectedSize int64) error {
	if destPath == "" {
		return fmt.Errorf("%w: missing destination path", types.ErrChecksumMismatch)
	}
	workingPath := destPath + types.IncompleteSuffix

	info, err := os.Stat(workingPath)
	if err != nil {
		return fmt.Errorf("%w: cannot stat finished file: %v", types.ErrChecksumMismatch, err)
	}
	if expectedSize <= 0 && cfg != nil {
		expectedSize = cfg.TotalSize
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return fmt.Errorf("%w: size mismatch: expected %d bytes, found %d",
			types.ErrChecksumMismatch, expectedSize, info.Size())
	}
	if progState != nil && expectedSize > 0 && !progState.AllChunksComplete(expectedSize) {
		return fmt.Errorf("%w: chunk coverage incomplete", types.ErrChecksumMismatch)
	}

	if cfg == nil || cfg.SkipChecksumVerify || cfg.Checksum == "" {
		return nil
	}

	expected, ok := integrity.Parse(cfg.Checksum)
	if !ok {
		// The digest was stored by an older build or in a form we cannot
		// compute. Structural verification already passed; do not fail the
		// download over unverifiable metadata.
		utils.Debug("Verification: ignoring unsupported checksum %q for %s", cfg.Checksum, destPath)
		return nil
	}

	actual, err := integrity.HashFile(workingPath, expected.Algorithm)
	if err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}
	if !strings.EqualFold(actual, expected.Value) {
		return fmt.Errorf("%w: expected %s:%s, got %s:%s",
			types.ErrChecksumMismatch, expected.Algorithm, expected.Value, expected.Algorithm, actual)
	}

	utils.Debug("Verification: %s checksum matched for %s", expected.Algorithm, destPath)
	return nil
}
