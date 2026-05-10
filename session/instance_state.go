package session

// instance_state.go contains Instance status/state machine methods.
// Note: InstanceStatusManager and InstanceStatusInfo are in instance_status.go.
// This file contains the instance-level status transitions and related methods.

import (
	"fmt"
	"strings"
	"time"
)

// setStatus sets the instance status without locking.
// Must be called with i.stateMutex held.
func (i *Instance) setStatus(status Status) {
	i.Status = status
}

// transitionTo validates and executes a state transition using the state machine.
// Must be called with i.stateMutex held.
func (i *Instance) transitionTo(s Status) error {
	if !CanTransition(i.Status, s) {
		return ErrInvalidTransition{From: i.Status, To: s}
	}
	i.setStatus(s)
	return nil
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

// MarkNeedsApproval transitions a Running instance to NeedsApproval.
// Called by the review queue poller when terminal output indicates the session is awaiting approval.
func (i *Instance) MarkNeedsApproval() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	return i.transitionTo(NeedsApproval)
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

// Approve transitions the instance from NeedsApproval to Running.
// Returns an error if the current state does not allow this transition.
func (i *Instance) Approve() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(Running); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	return nil
}

// Deny transitions the instance from NeedsApproval to Paused.
// Returns an error if the current state does not allow this transition.
func (i *Instance) Deny() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(Paused); err != nil {
		return fmt.Errorf("deny: %w", err)
	}
	return nil
}

// Paused returns true if the instance is paused.
func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// Started returns true if the instance has been started.
func (i *Instance) Started() bool {
	return i.started
}

// RecoverFromStopped resets a stale Stopped status to Ready so the instance can be
// hot-restored via Start(false). Only call this during startup reconciliation when
// the tmux session is confirmed alive; it bypasses the state machine intentionally.
func (i *Instance) RecoverFromStopped() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if i.Status == Stopped {
		i.setStatus(Ready)
		i.started = false
	}
}
