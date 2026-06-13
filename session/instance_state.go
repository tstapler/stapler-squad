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
// Must be called with i.stateMutex held (or before the instance is shared).
func (i *Instance) loadStatus(status Status) {
	i.Status = status
}

// setStatus is a backward-compatibility alias for loadStatus.
// Deprecated: use loadStatus for deserialization; use transitionTo for operational transitions.
func (i *Instance) setStatus(status Status) {
	i.loadStatus(status)
}

// transitionTo validates and executes a state transition using the TransitionDef table.
// Must be called with i.stateMutex held.
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
	if def.After != nil {
		def.After(ctx, i)
	}
	return nil
}

// IsCreating returns true if the instance is in the Creating state.
func (i *Instance) IsCreating() bool { return i.Status == Creating }

// IsActive returns true if the instance has a live AI process.
func (i *Instance) IsActive() bool { return i.Status == Active }

// IsPaused returns true if the instance is paused (worktree removed, branch preserved).
func (i *Instance) IsPaused() bool { return i.Status == Paused }

// IsStopped returns true if the instance is in the terminal Stopped state.
func (i *Instance) IsStopped() bool { return i.Status == Stopped }

// IsHibernated returns true if the instance has been hibernated (checkpoint written, tmux killed).
func (i *Instance) IsHibernated() bool { return i.Status == Hibernated }

// GetLifecycleStatus returns the current lifecycle status as a typed Status value.
func (i *Instance) GetLifecycleStatus() Status { return i.Status }

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
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastViewed = time.Now()
}

// MarkUserResponded records that the user has responded to this session.
// Returns the timestamp that was set so callers can persist it without a second lock acquisition.
func (i *Instance) MarkUserResponded() time.Time {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastUserResponse = time.Now()
	return i.LastUserResponse
}

// MarkAcknowledged records that the user has acknowledged (dismissed) this session from the review queue.
func (i *Instance) MarkAcknowledged() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastAcknowledged = time.Now()
}

// MarkNeedsApproval is a no-op: NeedsApproval is no longer a lifecycle state.
// Approval state is now tracked as sub-status via the detection layer.
// Deprecated: do not call from new code.
func (i *Instance) MarkNeedsApproval() error {
	return nil
}

// LastMeaningfulOutputTime returns the time of the last meaningful terminal output.
func (i *Instance) LastMeaningfulOutputTime() time.Time {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.LastMeaningfulOutput
}

// SetLastMeaningfulOutput sets the time of the last meaningful terminal output.
func (i *Instance) SetLastMeaningfulOutput(t time.Time) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastMeaningfulOutput = t
}

// GetEffectiveStatus returns the most accurate status for this instance,
// combining the lifecycle status with real-time terminal detection when available.
// Unlike Status (which only reflects lifecycle transitions), this consults the
// ClaudeController's detected terminal state to surface NeedsApproval, Idle, etc.
func (i *Instance) GetEffectiveStatus() Status {
	mgr := i.GetStatusManager()
	if mgr == nil {
		return i.Status
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive || statusInfo.ClaudeStatus == 0 { // 0 = StatusUnknown
		return i.Status
	}
	return StatusFromDetected(statusInfo.ClaudeStatus)
}

// GetStatus returns the current lifecycle status of this instance as an int.
// This is intentionally returns int to implement the SessionAccessor interface.
func (i *Instance) GetStatus() int {
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

// Approve transitions the instance to Active (approval granted).
// Returns an error if the current state does not allow this transition.
func (i *Instance) Approve() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(context.Background(), Active); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	return nil
}

// Deny transitions the instance to Paused (approval denied).
// Returns an error if the current state does not allow this transition.
func (i *Instance) Deny() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(context.Background(), Paused); err != nil {
		return fmt.Errorf("deny: %w", err)
	}
	return nil
}

// Paused returns true if the instance is paused.
func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// Hibernated returns true if the instance is hibernated.
func (i *Instance) Hibernated() bool {
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
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if i.Status == Stopped {
		i.loadStatus(Creating)
		i.started = false
	}
}

// ForceStatus sets the instance status directly without state machine validation.
// Only call from error recovery paths where the normal transition would itself fail
// (e.g. the async-creation goroutine cannot cleanly call Stop() because the session
// was never fully started). Callers must hold no locks.
func (i *Instance) ForceStatus(s Status) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.loadStatus(s)
}

// SetArchivedAtIfNil sets ArchivedAt to t only if it is currently nil.
// Returns true if the value was set (CAS semantics). Thread-safe via stateMutex.
func (i *Instance) SetArchivedAtIfNil(t time.Time) bool {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if i.ArchivedAt != nil {
		return false
	}
	i.ArchivedAt = &t
	return true
}
