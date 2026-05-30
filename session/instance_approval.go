package session

// instance_approval.go contains review queue integration, approval state methods,
// and terminal timestamp coordination for Instance.

import (
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// GetReviewQueue returns the review queue for this instance.
func (i *Instance) GetReviewQueue() *ReviewQueue {
	return i.reviewQueue
}

// SetReviewQueue sets the review queue for this instance.
func (i *Instance) SetReviewQueue(queue *ReviewQueue) {
	i.reviewQueue = queue
}

// NeedsReview returns true if this session is in the review queue.
func (i *Instance) NeedsReview() bool {
	if i.reviewQueue == nil {
		return false
	}
	return i.reviewQueue.Has(i.Title)
}

// GetReviewItem returns the review item for this instance if it exists.
func (i *Instance) GetReviewItem() (*ReviewItem, bool) {
	if i.reviewQueue == nil {
		return nil, false
	}
	return i.reviewQueue.Get(i.Title)
}

// SetStatusManager sets the status manager for idle detection.
func (i *Instance) SetStatusManager(manager *InstanceStatusManager) {
	i.controllerManager.SetStatusManager(manager)
}

// GetStatusManager returns the status manager.
func (i *Instance) GetStatusManager() *InstanceStatusManager {
	return i.controllerManager.GetStatusManager()
}

// UpdateTerminalTimestamps is a coordinator method that bridges ProcessManager (I/O)
// with ReviewState (timestamp recording). It:
//  1. Calls processManager.FilterBanners/HasMeaningfulContent (no lock needed, read-only ops)
//  2. Acquires stateMutex
//  3. Delegates to ReviewState.UpdateTimestamps
//
// This method intentionally stays on Instance because it coordinates two sub-managers.
// The forceUpdate parameter bypasses meaningful content checking for user-initiated interactions.
func (i *Instance) UpdateTerminalTimestamps(content string, forceUpdate bool) {
	filteredContent := content
	shouldUpdateMeaningful := false

	if i.pm().HasSession() {
		if forceUpdate {
			shouldUpdateMeaningful = true
			filteredContent, _ = i.pm().FilterBanners(content)
		} else {
			hasMeaningful := i.pm().HasMeaningfulContent(content)
			log.ForSession(i.Title).Debug("HasMeaningfulContent check", "hasMeaningful", hasMeaningful, "bytes", len(content))
			if hasMeaningful {
				shouldUpdateMeaningful = true
				filteredContent, _ = i.pm().FilterBanners(content)
			}
		}
	}

	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.UpdateTimestamps(content, filteredContent, shouldUpdateMeaningful, i.Title)
}

// GetTimeSinceLastMeaningfulOutput returns how long ago meaningful output was recorded.
// Fast path: reads the atomic shadow (no lock) once initialised via SyncAtomicTimestamps
// or UpdateTimestamps. Fallback: acquires stateMutex.RLock when the atomic is zero
// (before first write, or in tests that set LastMeaningfulOutput directly).
func (i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration {
	ns := i.loadLastMeaningfulOutputNs()
	if ns != 0 {
		return time.Since(time.Unix(0, ns))
	}
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.TimeSinceLastMeaningfulOutput(i.CreatedAt)
}

// GetTimeSinceLastTerminalUpdate delegates to ReviewState.TimeSinceLastTerminalUpdate.
// Falls back to time since creation if no terminal output has been recorded.
func (i *Instance) GetTimeSinceLastTerminalUpdate() time.Duration {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.TimeSinceLastTerminalUpdate(i.CreatedAt)
}

// detectAndTrackPrompt detects if current state is a new prompt and tracks it.
// Delegates to ReviewState.DetectAndTrackPrompt — caller must hold stateMutex.
func (i *Instance) detectAndTrackPrompt(content string, statusInfo InstanceStatusInfo) bool {
	return i.DetectAndTrackPrompt(content, statusInfo, i.Title)
}
