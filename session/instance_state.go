package session

// instance_state.go contains Instance status/state machine methods.
// Note: InstanceStatusManager and InstanceStatusInfo are in instance_status.go.
// This file contains the instance-level status transitions and related methods.

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// loadStatus sets Status directly without state machine validation.
// Call ONLY from FromInstanceData() deserialization or test setup.
// Never call from operational code paths.
// Must be called with i.mu held (or before the instance is shared).
func (i *Instance) loadStatus(status Status) {
	i.Status = status
}

// setStatus is a backward-compatibility alias for loadStatus.
// Deprecated: use loadStatus for deserialization; use transitionTo for operational transitions.
func (i *Instance) setStatus(status Status) {
	i.loadStatus(status)
}

// transitionTo validates and executes a state transition using the TransitionDef table.
// Must be called with i.mu held.
func (i *Instance) transitionTo(ctx context.Context, to Status) error {
	def, ok := lookupTransition(i.Status, to)
	if !ok {
		return ErrInvalidTransition{From: i.Status, To: to}
	}
	if def.Guard != nil {
		if err := def.Guard(ctx, i); err != nil {
			return fmt.Errorf("transition %s → %s blocked: %w", i.Status, to, err)
		}
	}
	i.Status = to
	// Store before calling After: After hooks may spawn goroutines that race with
	// a post-After snapshot read of the same fields.
	// Caller already holds i.mu (see doc comment above), so buildSnapshot's
	// "must be called while i.mu is held" contract is satisfied here.
	i.snapshot.Store(buildSnapshot(i))
	if def.After != nil {
		def.After(ctx, i)
	}
	return nil
}

// transitionToLocked executes a state transition from within an actor command.
// Does NOT invoke def.After hooks; hibernate/resume side-effects are handled
// inline by hibernateProcessLocked/resumeFromHibernationLocked so that heavy
// I/O is dispatched to goroutines without blocking the actor.
// Must only be called from within sendSyncErr/send/sendCtx closures.
func transitionToLocked(s *instanceState, ctx context.Context, to Status) error {
	i := s.inst
	// i.mu.RLock() around this initial read: RecoverFromStopped bypasses the
	// actor entirely (a direct i.mu.Lock()-protected write, not routed through
	// sendCtx/the mailbox), so it can genuinely run concurrently with an
	// in-flight actor command. Reading i.Status without the lock raced with
	// RecoverFromStopped's protected write under -race. See runActor's doc
	// comment in actor.go for the full explanation.
	i.mu.RLock()
	status := i.Status
	i.mu.RUnlock()
	def, ok := lookupTransition(status, to)
	if !ok {
		return ErrInvalidTransition{From: status, To: to}
	}
	if def.Guard != nil {
		if err := def.Guard(ctx, i); err != nil {
			return fmt.Errorf("transition %s → %s blocked: %w", status, to, err)
		}
	}
	// The write to i.Status and the buildSnapshot read are done under the same
	// i.mu.Lock()/Unlock() critical section, not just the read alone: legacy
	// setters (MarkViewed, MarkUserResponded, MarkAcknowledged,
	// SetLastMeaningfulOutput, RecoverFromStopped) bypass the actor and mutate
	// fields directly from arbitrary caller goroutines while holding
	// i.mu.Lock(), and their own buildSnapshot() call reads i.Status as part of
	// the full-struct copy. An unlocked write here raced with those i.mu-locked
	// readers under -race even though the write itself is otherwise safely
	// confined to the actor goroutine relative to OTHER actor commands. See
	// runActor's doc comment in actor.go for the full explanation.
	i.mu.Lock()
	from := i.Status
	i.Status = to
	snap := buildSnapshot(i)
	i.mu.Unlock()
	i.snapshot.Store(snap)
	// Trigger side-effects inline instead of via def.After so the actor goroutine
	// doesn't block waiting for the hook to complete.
	switch (transitionKey{from, to}) {
	case transitionKey{Active, Hibernated}:
		hibernateProcessLocked(s, ctx)
	case transitionKey{Hibernated, Active}:
		resumeFromHibernationLocked(s, ctx)
	case transitionKey{Crashed, Active}:
		resumeFromCrashLocked(s, ctx)
	}
	return nil
}

// approveLocked transitions the instance to Active from within an actor command.
func approveLocked(s *instanceState) error {
	if err := transitionToLocked(s, context.Background(), Active); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	return nil
}

// denyLocked transitions the instance to Paused from within an actor command.
func denyLocked(s *instanceState) error {
	if err := transitionToLocked(s, context.Background(), Paused); err != nil {
		return fmt.Errorf("deny: %w", err)
	}
	return nil
}

// IsCreating returns true if the instance is in the Creating state.
//
// Reads via Snapshot(), not i.mu.RLock() — see GetStatus's doc comment for why
// an RLock here doesn't actually synchronize with the actor's status writes.
func (i *Instance) IsCreating() bool {
	return i.Snapshot().Status == Creating
}

// IsActive returns true if the instance has a live AI process.
func (i *Instance) IsActive() bool {
	return i.Snapshot().Status == Active
}

// IsPaused returns true if the instance is paused (worktree removed, branch preserved).
func (i *Instance) IsPaused() bool {
	return i.Snapshot().Status == Paused
}

// IsStopped returns true if the instance is in the terminal Stopped state.
func (i *Instance) IsStopped() bool {
	return i.Snapshot().Status == Stopped
}

// IsHibernated returns true if the instance has been hibernated (checkpoint written, tmux killed).
func (i *Instance) IsHibernated() bool {
	return i.Snapshot().Status == Hibernated
}

// GetLifecycleStatus returns the current lifecycle status as a typed Status value.
func (i *Instance) GetLifecycleStatus() Status {
	return i.Snapshot().Status
}

// GetCategoryPath returns the category path as a slice of strings for nested category support
// Supports "Work/Frontend" syntax by splitting on "/" delimiter
func (i *Instance) GetCategoryPath() []string {
	if i.Category == "" {
		return []string{"Uncategorized"}
	}
	// Split category by "/" for nested support (e.g., "Work/Frontend" -> ["Work", "Frontend"])
	// Limit to max 2 levels deep for simplicity
	parts := strings.Split(i.Category, "/")
	if len(parts) > 2 {
		// Truncate to first 2 levels if more than 2 levels are provided
		parts = parts[:2]
	}
	return parts
}

// MarkViewed records that the user has viewed this session.
func (i *Instance) MarkViewed() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.LastViewed = time.Now()
	i.snapshot.Store(buildSnapshot(i))
}

// MarkUserResponded records that the user has responded to this session.
// Returns the timestamp that was set so callers can persist it without a second lock acquisition.
func (i *Instance) MarkUserResponded() time.Time {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.LastUserResponse = time.Now()
	i.snapshot.Store(buildSnapshot(i))
	return i.LastUserResponse
}

// MarkAcknowledged records that the user has acknowledged (dismissed) this session from the review queue.
func (i *Instance) MarkAcknowledged() {
	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now()
	i.LastAcknowledged = now
	// Keep the lock-free shadow in sync so IsAcknowledgedAfterOutput() can be called safely
	// from goroutines outside the actor pattern (e.g. ReviewQueuePoller's own goroutine via
	// review_queue_determiner.go's Determine()) — same discipline as UpdateTimestamps()
	// storing lastMeaningfulOutputNs alongside LastMeaningfulOutput.
	atomic.StoreInt64(&i.lastAcknowledgedNs, now.UnixNano())
	i.snapshot.Store(buildSnapshot(i))
}

// MarkNeedsApproval is a no-op: NeedsApproval is no longer a lifecycle state.
// Approval state is now tracked as sub-status via the detection layer.
// Deprecated: do not call from new code.
func (i *Instance) MarkNeedsApproval() error {
	return nil
}

// LastMeaningfulOutputTime returns the time of the last meaningful terminal output.
//
// Fast path: the atomic shadow (no lock), same as GetTimeSinceLastMeaningfulOutput.
// Fallback: Snapshot(), not a fresh i.mu-guarded read — i.mu doesn't synchronize
// with actor commands' direct field writes (see GetStatus's doc comment).
func (i *Instance) LastMeaningfulOutputTime() time.Time {
	if ns := i.loadLastMeaningfulOutputNs(); ns != 0 {
		return time.Unix(0, ns)
	}
	return i.Snapshot().LastMeaningfulOutput
}

// SetLastMeaningfulOutput sets the time of the last meaningful terminal output.
func (i *Instance) SetLastMeaningfulOutput(t time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.LastMeaningfulOutput = t
	i.snapshot.Store(buildSnapshot(i))
}

// GetEffectiveStatus returns the most accurate status for this instance,
// combining the lifecycle status with real-time terminal detection when available.
// Unlike Status (which only reflects lifecycle transitions), this consults the
// ClaudeController's detected terminal state to surface NeedsApproval, Idle, etc.
func (i *Instance) GetEffectiveStatus() Status {
	mgr := i.GetStatusManager()
	if mgr == nil {
		return i.Snapshot().Status
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive || statusInfo.ClaudeStatus == 0 { // 0 = StatusUnknown
		return i.Snapshot().Status
	}
	return StatusFromDetected(statusInfo.ClaudeStatus)
}

// GetStatus returns the current lifecycle status of this instance as an int.
// This is intentionally returns int to implement the SessionAccessor interface.
//
// Reads via Snapshot(), not i.mu.RLock(): actor commands (transitionToLocked
// and friends) write i.Status directly while running inside the actor's own
// serialization, not under i.mu, and only publish the change by atomically
// storing a fresh snapshot. An RLock here doesn't synchronize with that write
// at all — caught by -race via a concurrent GetStatus() poll during Start().
// Do not call this from within a sendSyncErr/send/sendCtx closure (see
// Snapshot's reentrancy note).
func (i *Instance) GetStatus() int {
	return int(i.Snapshot().Status)
}

// GetDetectedStatus returns the raw DetectedStatus from the terminal detection layer.
// Returns detection.StatusUnknown when no controller is active or no status has been detected.
// Use this for sub-status display; do not use for lifecycle decisions.
func (i *Instance) GetDetectedStatus() detection.DetectedStatus {
	mgr := i.GetStatusManager()
	if mgr == nil {
		return detection.StatusUnknown
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive {
		return detection.StatusUnknown
	}
	return statusInfo.ClaudeStatus
}

// GetDetectedContext returns the human-readable context string from the terminal detection layer.
// Returns an empty string when no controller is active or no context is available.
func (i *Instance) GetDetectedContext() string {
	mgr := i.GetStatusManager()
	if mgr == nil {
		return ""
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive {
		return ""
	}
	return statusInfo.StatusContext
}

// Approve transitions the instance to Active (approval granted).
// Returns an error if the current state does not allow this transition.
func (i *Instance) Approve() error {
	return i.sendSyncErr(func(s *instanceState) error { return approveLocked(s) })
}

// Deny transitions the instance to Paused (approval denied).
// Returns an error if the current state does not allow this transition.
func (i *Instance) Deny() error {
	return i.sendSyncErr(func(s *instanceState) error { return denyLocked(s) })
}

// Paused returns true if the instance is paused.
func (i *Instance) Paused() bool {
	return i.Snapshot().Status == Paused
}

// Hibernated returns true if the instance is hibernated.
func (i *Instance) Hibernated() bool {
	return i.Snapshot().Status == Hibernated
}

// Started returns true if the instance has been started.
func (i *Instance) Started() bool {
	return i.started.Load()
}

// RecoverFromStopped resets a stale Stopped status to Creating so the instance can be
// hot-restored via Start(false). Only call this during startup reconciliation when
// the tmux session is confirmed alive; it bypasses the state machine intentionally.
// Deprecated: prefer transitionTo(ctx, Active) on the Stopped→Active path.
func (i *Instance) RecoverFromStopped() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Status == Stopped {
		i.loadStatus(Creating)
		i.started.Store(false)
		i.snapshot.Store(buildSnapshot(i))
	}
}

// ForceStatus sets the instance status directly without state machine validation.
// Only call from error recovery paths where the normal transition would itself fail
// (e.g. the async-creation goroutine cannot cleanly call Stop() because the session
// was never fully started). Callers must hold no locks.
//
// Routes through the actor mailbox (sendCtx) rather than taking i.mu directly:
// ForceStatus is invoked from ad hoc goroutines outside the actor (e.g. the async
// CreateSession goroutine in SessionService), not from inside an actor command.
// Funneling through sendCtx serializes this write with the actor's command loop
// when the instance is actor-owned (LiveInstance), and falls back to running
// synchronously in-place when it isn't (e.g. tests constructing a bare *Instance).
//
// The write (loadStatus) and the buildSnapshot read are done under the SAME
// i.mu.Lock()/Unlock() critical section (not lock-write-then-unlock-then-read,
// which is what this used to do). buildSnapshot reads every mutable field,
// including ones mutated directly under i.mu by legacy setters (MarkViewed,
// SetLastMeaningfulOutput, MarkUserResponded, MarkAcknowledged,
// RecoverFromStopped) that bypass the actor entirely and run on arbitrary
// caller goroutines. Calling buildSnapshot() after releasing the lock left a
// window where one of those setters could mutate fields concurrently with
// this unguarded read — caught by -race via a concurrent MarkViewed()/
// ForceStatus() pairing during CreateSession. See runActor's doc comment in
// actor.go for the matching fix on the read side.
func (i *Instance) ForceStatus(s Status) {
	_ = i.sendCtx(context.Background(), func(_ *instanceState) {
		i.mu.Lock()
		i.loadStatus(s)
		snap := buildSnapshot(i)
		i.mu.Unlock()
		i.snapshot.Store(snap)
	})
}
