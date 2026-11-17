package domain

import (
	"sync/atomic"
	"time"
)

// ReviewState holds all timestamps and state related to the review queue and terminal activity
// tracking for a session. It is embedded in Instance so all field accesses remain unchanged.
//
// Fields are NOT protected by Instance.mu. Mutation is serialized through the actor's
// send()/sendSyncErr() closures instead (see UpdateTerminalTimestamps in
// instance_approval.go, which routes through i.send() rather than i.mu.Lock()) - the
// same "no locking, serialize via the actor's own command queue" discipline used by
// transitionToLocked et al. Methods on ReviewState are intentionally non-locking -
// callers must be running inside an actor command closure (or otherwise be the sole
// writer) if concurrent access is possible.
//
// Direct field access via Go embedding promotion (inst.LastMeaningfulOutput etc.) is used by:
//   - session/review_queue_poller.go: reads LastMeaningfulOutput, LastAcknowledged,
//     LastAddedToQueue, ProcessingGraceUntil, LastPromptDetected, LastPromptSignature,
//     LastUserResponse, LastViewed, LastTerminalUpdate, LastOutputSignature
//   - server/dependencies.go: reads LastMeaningfulOutput, LastTerminalUpdate,
//     LastAddedToQueue, LastAcknowledged
//   - server/adapters/instance_adapter.go: reads LastTerminalUpdate, LastMeaningfulOutput
//   - server/review_queue_manager.go: writes LastUserResponse directly
//
// All access is either within the session package (via the actor's serialized
// closures) or through Instance methods that route through i.send()/sendSyncErr().
//
// TODO: Migrate cross-package field accesses (server/) to accessor methods to enable
// future encapsulation of ReviewState as a composed (non-embedded) field.
type ReviewState struct {
	// LastAcknowledged tracks when the user last acknowledged this session in the review queue.
	// Sessions acknowledged after their last update won't appear in the queue until they update again.
	LastAcknowledged time.Time

	// LastAddedToQueue tracks when this session was last added to the review queue.
	// Used to prevent notification spam by enforcing a minimum re-add interval.
	LastAddedToQueue time.Time

	// LastTerminalUpdate is the timestamp of the last output received from the terminal (any output).
	LastTerminalUpdate time.Time

	// LastMeaningfulOutput is the timestamp of the last meaningful output (excludes tmux status banners).
	// Used by the review queue to determine session staleness.
	LastMeaningfulOutput time.Time

	// LastOutputSignature is a hash of the terminal content, used to detect actual changes
	// vs app restarts with unchanged content (prevents false "new activity" notifications).
	LastOutputSignature string

	// LastViewed tracks when the user last interacted with this session
	// (viewing the terminal, attaching via tmux, or viewing session details).
	// Used for smarter review queue notifications (don't notify if just viewed).
	LastViewed time.Time

	// LastPromptDetected is the timestamp when we last detected a prompt requiring user input.
	// Used to distinguish new prompts from the same prompt re-appearing.
	LastPromptDetected time.Time

	// LastPromptSignature is a hash of the prompt content (last 10 lines before cursor).
	// Used to determine if this is the same prompt or a new one.
	LastPromptSignature string

	// LastUserResponse is the timestamp when the user last provided input/interaction.
	// Used to determine if user responded AFTER a prompt was detected.
	LastUserResponse time.Time

	// ProcessingGraceUntil is the deadline for waiting for the session to respond after
	// user interaction. If the session shows no activity by this time, it may be re-added
	// to the review queue.
	ProcessingGraceUntil time.Time

	// lastMeaningfulOutputNs is a lock-free shadow of LastMeaningfulOutput stored as
	// UnixNano. Written under mu (write); read without any lock via atomic ops.
	// Zero means "not yet recorded". Use SyncAtomicTimestamps() after construction.
	lastMeaningfulOutputNs int64

	// lastAcknowledgedNs is a lock-free shadow of LastAcknowledged stored as UnixNano,
	// following the exact same pattern as lastMeaningfulOutputNs. Written under mu
	// (MarkAcknowledged, instance_state.go); read without any lock via atomic ops.
	// Zero means "not yet recorded". Use SyncAtomicTimestamps() after construction.
	lastAcknowledgedNs int64
}

// SyncAtomicTimestamps initialises atomic shadow fields from their time.Time counterparts.
// Must be called once after constructing ReviewState from persisted or restored data so that
// lock-free readers see the correct initial value immediately.
func (rs *ReviewState) SyncAtomicTimestamps() {
	if !rs.LastMeaningfulOutput.IsZero() {
		atomic.StoreInt64(&rs.lastMeaningfulOutputNs, rs.LastMeaningfulOutput.UnixNano())
	}
	if !rs.LastAcknowledged.IsZero() {
		atomic.StoreInt64(&rs.lastAcknowledgedNs, rs.LastAcknowledged.UnixNano())
	}
}

// LoadLastMeaningfulOutputNs returns the nanosecond timestamp without acquiring any lock.
// Returns 0 when no meaningful output has been recorded yet.
func (rs *ReviewState) LoadLastMeaningfulOutputNs() int64 {
	return atomic.LoadInt64(&rs.lastMeaningfulOutputNs)
}

// StoreLastMeaningfulOutputNs publishes the lock-free shadow of LastMeaningfulOutput.
func (rs *ReviewState) StoreLastMeaningfulOutputNs(ns int64) {
	atomic.StoreInt64(&rs.lastMeaningfulOutputNs, ns)
}

// LoadLastAcknowledgedNs returns the nanosecond timestamp without acquiring any lock.
// Returns 0 when the session has not yet been acknowledged.
func (rs *ReviewState) LoadLastAcknowledgedNs() int64 {
	return atomic.LoadInt64(&rs.lastAcknowledgedNs)
}

// StoreLastAcknowledgedNs publishes the lock-free shadow of LastAcknowledged.
func (rs *ReviewState) StoreLastAcknowledgedNs(ns int64) {
	atomic.StoreInt64(&rs.lastAcknowledgedNs, ns)
}

// TimeSinceLastMeaningfulOutput returns how long ago meaningful terminal output was received.
// If no meaningful output has been recorded yet, returns the duration since the given
// createdAt time.
//
// Lock-free: reads the atomic shadow (lastMeaningfulOutputNs) via loadLastMeaningfulOutputNs()
// rather than the plain LastMeaningfulOutput field, so this method is safe to call from any
// goroutine — including outside the actor's serialized command closures (e.g.
// HibernationSweeper.sweep(), which runs on its own independent background goroutine).
func (rs *ReviewState) TimeSinceLastMeaningfulOutput(createdAt time.Time) time.Duration {
	ns := rs.LoadLastMeaningfulOutputNs()
	if ns == 0 {
		return time.Since(createdAt)
	}
	return time.Since(time.Unix(0, ns))
}

// TimeSinceLastTerminalUpdate returns how long ago any terminal output was received.
// If LastTerminalUpdate is zero, returns the duration since the given createdAt time.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) TimeSinceLastTerminalUpdate(createdAt time.Time) time.Duration {
	if rs.LastTerminalUpdate.IsZero() {
		return time.Since(createdAt)
	}
	return time.Since(rs.LastTerminalUpdate)
}

// IsAcknowledgedAfterOutput returns true if the user acknowledged this session more recently
// than the last meaningful terminal output — meaning no new output has occurred since the
// user last dismissed the session from the review queue.
// Returns false when no meaningful output has been recorded yet: the acknowledgment cannot
// logically be "after" output that never happened, so the session is not snoozed.
//
// Lock-free: reads both atomic shadows (lastMeaningfulOutputNs, lastAcknowledgedNs) rather
// than the plain LastMeaningfulOutput/LastAcknowledged fields, so this method is safe to call
// from any goroutine — including outside the actor's serialized command closures (e.g.
// review_queue_determiner.go's Determine(), called directly on the live *Instance from
// ReviewQueuePoller's own independent background goroutine).
func (rs *ReviewState) IsAcknowledgedAfterOutput() bool {
	outputNs := rs.LoadLastMeaningfulOutputNs()
	if outputNs == 0 {
		return false
	}
	ackNs := rs.LoadLastAcknowledgedNs()
	return ackNs != 0 && ackNs > outputNs
}

// IsInProcessingGracePeriod returns true if the session is within its processing grace window.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) IsInProcessingGracePeriod() bool {
	return !rs.ProcessingGraceUntil.IsZero() && time.Now().Before(rs.ProcessingGraceUntil)
}

// UserRespondedAfterPrompt returns true if the user responded (LastUserResponse) after
// a prompt was detected (LastPromptDetected), indicating the session is no longer waiting.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) UserRespondedAfterPrompt() bool {
	return !rs.LastUserResponse.IsZero() &&
		!rs.LastPromptDetected.IsZero() &&
		rs.LastUserResponse.After(rs.LastPromptDetected)
}
