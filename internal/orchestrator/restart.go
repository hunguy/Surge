package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

// Restart discards a download's progress and queues the same URL, destination
// and authentication again from zero.
//
// The old download keeps no resumable state: its pool entry is cancelled, its
// persisted state (chunk topology, headers, master-list record) is deleted and
// its working .surge file is removed before the fresh enqueue reserves the same
// path. The new download gets a new ID, so lifecycle events that were already
// in flight for the cancelled run cannot land on it. The old ID is tombstoned
// instead, and clients are told to drop its row.
func (mgr *LifecycleManager) Restart(id string) (string, error) {
	if id == "" {
		return "", types.ErrNotFound
	}
	if mgr.pool == nil {
		return "", types.ErrServiceUnavailable
	}

	url, destPath, filename := mgr.restartIdentity(id)
	if url == "" || destPath == "" {
		return "", types.ErrNotFound
	}

	if st := mgr.pool.GetStatus(id); st != nil && st.Status == "completed" {
		return "", types.ErrCompleted
	}

	entry, _ := store.GetDownload(id)
	if entry != nil && entry.Status == "completed" {
		return "", types.ErrCompleted
	}

	headers, mirrors, workers, minChunkSize, rateLimit, rateLimitSet := restartOptions(entry, url, destPath)

	// Stop the running workers first: the files below are still being written
	// until the pool has cancelled them.
	if result := mgr.pool.Cancel(id); result.Found {
		if destPath == "" {
			destPath = result.DestPath
		}
		if filename == "" {
			filename = result.Filename
		}
	}

	if err := store.DeleteState(id); err != nil {
		return "", fmt.Errorf("clear download state: %w", err)
	}
	mgr.markRemoved(id)

	if err := RemoveIncompleteFile(destPath); err != nil {
		return "", fmt.Errorf("remove partial download: %w", err)
	}

	// Drop the old row from every connected client. Completed is set so the
	// event worker does not remove files again — the partial file is gone and a
	// newer download may already own the same working path.
	if mgr.eventBus != nil {
		_ = mgr.eventBus.Publish(types.DownloadEvent{
			Type:       types.EventRemoved,
			DownloadID: id,
			Completed:  true,
		})
	}

	req := &DownloadRequest{
		URL:                url,
		Path:               filepath.Dir(destPath),
		Filename:           filename,
		Mirrors:            mirrors,
		Headers:            headers,
		IsExplicitCategory: true, // keep the destination the user already chose
		Workers:            workers,
		MinChunkSize:       minChunkSize,
	}
	newID, _, err := mgr.Enqueue(context.Background(), req)
	if err != nil {
		return "", err
	}

	if rateLimitSet && rateLimit > 0 {
		if err := store.UpdateRateLimit(newID, rateLimit); err == nil {
			mgr.pool.SetDownloadRateLimit(newID, rateLimit)
		}
	}

	return newID, nil
}

// restartIdentity reads the URL, full destination path and filename of a
// download from the persisted record, falling back to the in-memory pool for a
// download that has not been persisted yet.
func (mgr *LifecycleManager) restartIdentity(id string) (url, destPath, filename string) {
	if entry, err := store.GetDownload(id); err == nil && entry != nil {
		url, destPath, filename = entry.URL, entry.DestPath, entry.Filename
	}
	if st := mgr.pool.GetStatus(id); st != nil {
		if url == "" {
			url = st.URL
		}
		if destPath == "" {
			destPath = st.DestPath
		}
		if filename == "" {
			filename = st.Filename
		}
	}
	return url, destPath, filename
}

// restartOptions carries the per-download configuration that must survive a
// restart: authentication headers, extra mirrors, worker overrides and an
// explicit rate limit. The persisted detail state holds everything except the
// rate limit, which lives on the master-list record.
func restartOptions(entry *types.DownloadRecord, url, destPath string) (headers map[string]string, mirrors []string, workers int, minChunkSize int64, rateLimit int64, rateLimitSet bool) {
	if entry != nil {
		mirrors = append([]string(nil), entry.Mirrors...)
		workers = entry.Workers
		minChunkSize = entry.MinChunkSize
		rateLimit = entry.RateLimit
		rateLimitSet = entry.RateLimitSet
	}

	if saved, err := store.LoadState(url, destPath); err == nil && saved != nil {
		headers = saved.Headers
		if workers == 0 {
			workers = saved.Workers
		}
		if minChunkSize == 0 {
			minChunkSize = saved.MinChunkSize
		}
	}
	return headers, mirrors, workers, minChunkSize, rateLimit, rateLimitSet
}
