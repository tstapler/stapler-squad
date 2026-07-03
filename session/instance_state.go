package session

// instance_state.go contains Instance status/state machine methods.
// Note: InstanceStatusManager and InstanceStatusInfo are in instance_status.go.
// This file contains the instance-level status transitions and related methods.

import (
	"context"
	"fmt"
	"strings"
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
	def, ok := transitionIndex[transitionKey{i.Status, to}]
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
	def, ok := transitionIndex[transitionKey{i.Status, to}]
	if !ok {
		return ErrInvalidTransition{From: i.Status, To: to}
	}
	if def.Guard != nil {
		if err := def.Guard(ctx, i); err != nil {
			return fmt.Errorf("transition %s → %s blocked: %w", i.Status, to, err)
		}
	}
	from := i.Status
	i.Status = to
	i.snapshot.Store(buildSnapshot(i))
	// Trigger side-effects inline instead of via def.After so the actor goroutine
	// doesn't block waiting for the hook to complete.
	switch (transitionKey{from, to}) {
	case transitionKey{Active, Hibernated}:
		hibernateProcessLocked(s, ctx)
	case transitionKey{Hibernated, Active}:
		resumeFromHibernationLocked(s, ctx)
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
func (i *Instance) IsCreating() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Creating
}

// IsActive returns true if the instance has a live AI process.
func (i *Instance) IsActive() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Active
}

// IsPaused returns true if the instance is paused (worktree removed, branch preserved).
func (i *Instance) IsPaused() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Paused
}

// IsStopped returns true if the instance is in the terminal Stopped state.
func (i *Instance) IsStopped() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Stopped
}

// IsHibernated returns true if the instance has been hibernated (checkpoint written, tmux killed).
func (i *Instance) IsHibernated() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Hibernated
}

// GetLifecycleStatus returns the current lifecycle status as a typed Status value.
func (i *Instance) GetLifecycleStatus() Status {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status
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
	i.LastAcknowledged = time.Now()
	i.snapshot.Store(buildSnapshot(i))
}

// MarkNeedsApproval is a no-op: NeedsApproval is no longer a lifecycle state.
// Approval state is now tracked as sub-status via the detection layer.
// Deprecated: do not call from new code.
func (i *Instance) MarkNeedsApproval() error {
	return nil
}

// LastMeaningfulOutputTime returns the time of the last meaningful terminal output.
func (i *Instance) LastMeaningfulOutputTime() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.LastMeaningfulOutput
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
		i.mu.RLock()
		s := i.Status
		i.mu.RUnlock()
		return s
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive || statusInfo.ClaudeStatus == 0 { // 0 = StatusUnknown
		i.mu.RLock()
		s := i.Status
		i.mu.RUnlock()
		return s
	}
	return StatusFromDetected(statusInfo.ClaudeStatus)
}

// GetStatus returns the current lifecycle status of this instance as an int.
// This is intentionally returns int to implement the SessionAccessor interface.
func (i *Instance) GetStatus() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return int(i.Status)
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
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Paused
}

// Hibernated returns true if the instance is hibernated.
func (i *Instance) Hibernated() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == Hibernated
}

// Started returns true if the instance has been started.
func (i *Instance) Started() bool {
	return i.started
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
		i.started = false
		i.snapshot.Store(buildSnapshot(i))
	}
}

// ForceStatus sets the instance status directly without state machine validation.
// Only call from error recovery paths where the normal transition would itself fail
// (e.g. the async-creation goroutine cannot cleanly call Stop() because the session
// was never fully started). Callers must hold no locks.
func (i *Instance) ForceStatus(s Status) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.loadStatus(s)
	i.snapshot.Store(buildSnapshot(i))
}
