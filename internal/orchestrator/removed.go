package orchestrator

import "time"

// removedIDRetention bounds how long a removed download's tombstone is kept.
// Late lifecycle events arrive within seconds of a cancel; an hour is
// comfortably generous while keeping the map bounded on long-lived daemons.
const removedIDRetention = time.Hour

// markRemoved tombstones a download ID. Called synchronously from Cancel before
// the removal event is published, so any event the worker processes afterwards
// can tell the download was intentionally deleted rather than resume it.
func (mgr *LifecycleManager) markRemoved(id string) {
	if mgr == nil || id == "" {
		return
	}

	now := time.Now()
	mgr.removedIDsMu.Lock()
	if mgr.removedIDs == nil {
		mgr.removedIDs = make(map[string]time.Time)
	}
	mgr.removedIDs[id] = now
	for existing, at := range mgr.removedIDs {
		if now.Sub(at) > removedIDRetention {
			delete(mgr.removedIDs, existing)
		}
	}
	mgr.removedIDsMu.Unlock()
}

// clearRemoved drops a tombstone so a freshly enqueued download that reuses the
// same ID is not ignored by the event worker.
func (mgr *LifecycleManager) clearRemoved(id string) {
	if mgr == nil || id == "" {
		return
	}
	mgr.removedIDsMu.Lock()
	delete(mgr.removedIDs, id)
	mgr.removedIDsMu.Unlock()
}

// isRemoved reports whether the download was removed and should ignore
// lifecycle events.
func (mgr *LifecycleManager) isRemoved(id string) bool {
	if mgr == nil || id == "" {
		return false
	}
	mgr.removedIDsMu.Lock()
	defer mgr.removedIDsMu.Unlock()
	_, ok := mgr.removedIDs[id]
	return ok
}
