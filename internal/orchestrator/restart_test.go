package orchestrator

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

// TestLifecycleManager_RestartWipesProgressAndRequeues covers the contract the
// TUI relies on: the old download is gone from the store, its partial file is
// wiped, a fresh download owns the same destination, and clients are told to
// drop the stale row.
func TestLifecycleManager_RestartWipesProgressAndRequeues(t *testing.T) {
	store.Configure(filepath.Join(t.TempDir(), "downloads.db"))
	t.Cleanup(store.CloseDB)

	release := make(chan struct{})
	var releaseOnce sync.Once
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "1048576")
			w.WriteHeader(http.StatusOK)
			return
		}
		// The probe sends a one-byte range request; answer it immediately so the
		// test measures restart behaviour rather than the probe timeout.
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 0-0/1048576")
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{0})
			return
		}
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		<-release
		_, _ = w.Write(make([]byte, 1<<20))
	}))
	defer ts.Close()

	dest := t.TempDir()
	const filename = "restart.bin"
	destPath := filepath.Join(dest, filename)
	surgePath := destPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("already-downloaded-bytes"), 0o644); err != nil {
		t.Fatalf("seed partial file: %v", err)
	}

	const oldID = "old-download-id"
	if err := store.AddToMasterList(types.DownloadRecord{
		ID:         oldID,
		URL:        ts.URL + "/" + filename,
		Filename:   filename,
		DestPath:   destPath,
		Status:     "error",
		Downloaded: 23,
		TotalSize:  1048576,
		Workers:    2,
	}); err != nil {
		t.Fatalf("seed store record: %v", err)
	}

	pool := scheduler.New(make(chan types.DownloadEvent, 128), 1)
	eb := NewEventBus()
	mgr := NewLifecycleManager(pool, eb, nil)
	defer mgr.Shutdown()

	events, cleanup := eb.Subscribe()
	defer cleanup()

	newID, err := mgr.Restart(oldID)
	if err != nil {
		t.Fatalf("Restart failed: %v", err)
	}
	defer releaseOnce.Do(func() { close(release) })

	if newID == "" || newID == oldID {
		t.Fatalf("new id = %q, want a fresh id distinct from %q", newID, oldID)
	}

	if entry, _ := store.GetDownload(oldID); entry != nil {
		t.Fatalf("old record survived restart: %+v", entry)
	}

	entry, err := store.GetDownload(newID)
	if err != nil {
		t.Fatalf("load new record: %v", err)
	}
	if entry == nil {
		t.Fatal("new record missing from the store")
	}
	if entry.Downloaded != 0 {
		t.Fatalf("new download starts at %d bytes, want 0", entry.Downloaded)
	}
	if entry.DestPath != destPath {
		t.Fatalf("new destination = %q, want %q", entry.DestPath, destPath)
	}
	if entry.Status == "completed" {
		t.Fatalf("new download status = %q", entry.Status)
	}

	// The restarted download preallocates its working file, so the size alone is
	// not evidence either way; the bytes written by the old download must be gone.
	data, err := os.ReadFile(surgePath)
	if err != nil {
		t.Fatalf("working file was not re-reserved: %v", err)
	}
	if bytes.Contains(data, []byte("already-downloaded-bytes")) {
		t.Fatal("restart kept the old download's bytes in the working file")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type == types.EventRemoved && ev.DownloadID == oldID {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the removal event that drops the old row")
		}
	}
}

func TestLifecycleManager_RestartRejectsCompletedDownload(t *testing.T) {
	store.Configure(filepath.Join(t.TempDir(), "downloads.db"))
	t.Cleanup(store.CloseDB)

	pool := scheduler.New(make(chan types.DownloadEvent, 8), 1)
	mgr := NewLifecycleManager(pool, nil, nil)
	defer mgr.Shutdown()

	const id = "completed-download"
	if err := store.AddToMasterList(types.DownloadRecord{
		ID:       id,
		URL:      "https://example.com/file.bin",
		Filename: "file.bin",
		DestPath: "/tmp/file.bin",
		Status:   "completed",
	}); err != nil {
		t.Fatalf("seed store record: %v", err)
	}

	if _, err := mgr.Restart(id); !errors.Is(err, types.ErrCompleted) {
		t.Fatalf("Restart(completed) error = %v, want ErrCompleted", err)
	}

	if entry, _ := store.GetDownload(id); entry == nil {
		t.Fatal("completed download must not be deleted by a rejected restart")
	}
}

func TestLifecycleManager_RestartUnknownDownload(t *testing.T) {
	store.Configure(filepath.Join(t.TempDir(), "downloads.db"))
	t.Cleanup(store.CloseDB)

	pool := scheduler.New(make(chan types.DownloadEvent, 8), 1)
	mgr := NewLifecycleManager(pool, nil, nil)
	defer mgr.Shutdown()

	if _, err := mgr.Restart("missing-id"); !errors.Is(err, types.ErrNotFound) {
		t.Fatalf("Restart(unknown) error = %v, want ErrNotFound", err)
	}
}
