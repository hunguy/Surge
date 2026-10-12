package orchestrator

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

// A download can be removed (x in the TUI) while lifecycle events for it are
// still in flight: the scheduler's retry path emits EventQueued, a start races
// the cancel, an error snapshot lands late, etc. Those events must not
// resurrect the deleted record, or the item reappears on the next
// list/reconnect.
func TestEventWorker_RemovedDownloadNotResurrectedByLateEvent(t *testing.T) {
	cases := []struct {
		name  string
		event types.DownloadEvent
	}{
		{name: "started", event: types.DownloadEvent{Type: types.EventStarted}},
		{name: "queued retry", event: types.DownloadEvent{Type: types.EventQueued}},
		{name: "error", event: types.DownloadEvent{Type: types.EventError, Err: errors.New("late failure")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := setupEventStateDB(t)

			const id = "resurrect-victim"
			url := "http://example.com/victim.bin"
			dest := filepath.Join(tmpDir, "victim.bin")
			if err := store.AddToMasterList(types.DownloadRecord{
				ID:       id,
				URL:      url,
				URLHash:  store.URLHash(url),
				DestPath: dest,
				Filename: "victim.bin",
				Status:   "queued",
			}); err != nil {
				t.Fatalf("seed AddToMasterList: %v", err)
			}

			progressCh := make(chan types.DownloadEvent, 16)
			pool := scheduler.New(progressCh, 1)
			eb := NewEventBus()
			mgr := NewLifecycleManager(pool, eb, config.DefaultSettings())
			t.Cleanup(mgr.Shutdown)
			t.Cleanup(eb.Shutdown)

			ch, cleanup := eb.Subscribe()
			t.Cleanup(cleanup)
			go mgr.StartEventWorker(ch)

			if err := mgr.Cancel(id); err != nil {
				t.Fatalf("Cancel: %v", err)
			}

			deadline := time.Now().Add(2 * time.Second)
			for {
				entry, err := store.GetDownload(id)
				if err != nil {
					t.Fatalf("GetDownload: %v", err)
				}
				if entry == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("download %s was not removed", id)
				}
				time.Sleep(10 * time.Millisecond)
			}

			event := tc.event
			event.DownloadID = id
			event.URL = url
			event.DestPath = dest
			event.Filename = "victim.bin"
			if err := eb.Publish(event); err != nil {
				t.Fatalf("publish late %s: %v", tc.name, err)
			}

			time.Sleep(200 * time.Millisecond)

			if entry, err := store.GetDownload(id); err != nil {
				t.Fatalf("GetDownload after late event: %v", err)
			} else if entry != nil {
				t.Fatalf("late %s event resurrected a removed download: %+v", tc.name, entry)
			}
		})
	}
}

// The tombstone helpers back the resurrect guard: markRemoved flags an ID and
// clearRemoved (called on a fresh enqueue that reuses the ID) unflags it.
func TestRemovedTombstone_MarkAndClear(t *testing.T) {
	setupEventStateDB(t)

	pool := scheduler.New(make(chan types.DownloadEvent, 16), 1)
	mgr := NewLifecycleManager(pool, nil, config.DefaultSettings())
	t.Cleanup(mgr.Shutdown)

	const id = "reused-id"
	mgr.markRemoved(id)
	if !mgr.isRemoved(id) {
		t.Fatal("expected ID to be tombstoned")
	}
	mgr.clearRemoved(id)
	if mgr.isRemoved(id) {
		t.Fatal("expected retained tombstone to be cleared")
	}
}
