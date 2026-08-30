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

// GetNotifier returns the notifier for this instance, or nil if unset.
func (i *Instance) GetNotifier() Notifier {
	return i.notifier
}

// SetNotifier sets the notifier for this instance, used by
// markSessionPermanentlyFailed to push a proactive notification when the
// retry policy exhausts its attempt budget.
func (i *Instance) SetNotifier(notifier Notifier) {
	i.notifier = notifier
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
//  1. Calls processManager.FilterBanners/HasMeaningfulContent (I/O-ish, done before
//     touching the actor — same "no I/O inside the command" discipline as the other
//     *Locked helpers)
//  2. Routes the actual field mutation through the actor's send() — this is called
//     from the PTY-read hot path (server/services/session_service.go's StreamTerminal),
//     exactly the "callback that must not block the caller" case send() exists for
//     (see actor.go's doc comment). Routing through the actor instead of i.mu means
//     this mutation is serialized against every other actor command (transitionToLocked
//     et al.), which don't take i.mu either — caught by -race via a concurrent
//     StreamTerminal + StartController flow.
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

	i.send(func(s *instanceState) {
		// send() only auto-republishes the snapshot when a live actor is running
		// (runActor's loop, after every command); in the actor-less fallback path
		// (li == nil — e.g. plain-struct-literal tests) nothing else does this, so
		// the mutation above would never become visible via Snapshot(). Publish
		// explicitly here, matching transitionToLocked's own discipline — but only
		// when UpdateTimestamps actually changed something, so a terminal frame
		// with no meaningful content doesn't force a snapshot rebuild.
		//
		// UpdateTimestamps and the buildSnapshot read are done under i.mu so this
		// is ordered against legacy direct-lock setters (MarkViewed & co.) that
		// read these same fields via buildSnapshot under i.mu.Lock() from outside
		// the actor. See runActor's doc comment in actor.go.
		s.inst.mu.Lock()
		changed := s.inst.UpdateTimestamps(content, filteredContent, shouldUpdateMeaningful, s.inst.Title)
		var snap *InstanceSnapshot
		if changed {
			snap = buildSnapshot(s.inst)
		}
		s.inst.mu.Unlock()
		if changed {
			s.inst.snapshot.Store(snap)
		}
	})
}

// GetTimeSinceLastMeaningfulOutput returns how long ago meaningful output was recorded.
// Fast path: reads the atomic shadow (no lock) once initialised via SyncAtomicTimestamps
// or UpdateTimestamps. Fallback: Snapshot() when the atomic is zero (before first
// write, or in tests that set LastMeaningfulOutput directly) — not a fresh
// i.mu-guarded read, since i.mu doesn't synchronize with actor commands' direct
// field writes (see GetStatus's doc comment).
func (i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration {
	ns := i.loadLastMeaningfulOutputNs()
	if ns != 0 {
		return time.Since(time.Unix(0, ns))
	}
	snap := i.Snapshot()
	return snap.TimeSinceLastMeaningfulOutput(snap.CreatedAt)
}

// GetTimeSinceLastTerminalUpdate delegates to ReviewState.TimeSinceLastTerminalUpdate.
// Falls back to time since creation if no terminal output has been recorded.
// Reads via Snapshot(), not i.mu.RLock() — see GetTimeSinceLastMeaningfulOutput.
func (i *Instance) GetTimeSinceLastTerminalUpdate() time.Duration {
	snap := i.Snapshot()
	return snap.TimeSinceLastTerminalUpdate(snap.CreatedAt)
}
