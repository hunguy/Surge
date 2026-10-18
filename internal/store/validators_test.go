package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestSaveState_PersistsSourceValidators(t *testing.T) {
	tmpDir := setupTestDB(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer CloseDB()

	url := "https://cdn.example.com/file.bin"
	destPath := filepath.Join(tmpDir, "file.bin")
	state := &types.DownloadRecord{
		ID:           "validators-id",
		URL:          url,
		DestPath:     destPath,
		Filename:     "file.bin",
		Status:       "paused",
		TotalSize:    4096,
		Downloaded:   1024,
		Tasks:        []types.Task{{Offset: 1024, Length: 3072}},
		ETag:         `"v1-strong"`,
		LastModified: "Wed, 21 Oct 2015 07:28:00 GMT",
		Checksum:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	if err := AddToMasterList(*state); err != nil {
		t.Fatalf("AddToMasterList failed: %v", err)
	}
	if err := SaveStateWithOptions(url, destPath, state, SaveStateOptions{SkipFileHash: true}); err != nil {
		t.Fatalf("SaveStateWithOptions failed: %v", err)
	}

	loaded, err := LoadState(url, destPath)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if loaded.ETag != state.ETag || loaded.LastModified != state.LastModified || loaded.Checksum != state.Checksum {
		t.Fatalf("detail state lost validators: %+v", loaded)
	}

	master, err := GetDownload(state.ID)
	if err != nil || master == nil {
		t.Fatalf("GetDownload failed: %v", err)
	}
	if master.ETag != state.ETag || master.LastModified != state.LastModified || master.Checksum != state.Checksum {
		t.Fatalf("master state lost validators: %+v", master)
	}
}
