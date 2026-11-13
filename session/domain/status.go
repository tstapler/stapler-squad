package domain

import (
	"fmt"
)

// Status is the lifecycle state of a session instance.
type Status int

const (
	// Creating is the status when the instance is being initialized.
	Creating Status = 0
	// Active is the status when the instance has a live AI process (running or ready).
	Active Status = 1
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused Status = 2
	// Stopped is a terminal state: the instance has been shut down and cannot transition further.
	Stopped Status = 3
	// Hibernated is the status when the instance has been checkpointed and the tmux session killed.
	Hibernated Status = 4
	// Restoring is the transient startup state when a hibernated session is being restored.
	// Never persisted to the database — transitions to Active or Creating on completion.
	Restoring Status = 5
	// Crashed is a terminal state distinct from Stopped: the wrapped program exited
	// abnormally (non-zero exit code or signal) and tmux's remain-on-exit left a dead
	// pane placeholder, detected by SessionHealthChecker's polling (see session/health.go).
	// Unlike Stopped, a Crashed session is not auto-recovered by the health checker —
	// it surfaces to the user/automation for an explicit resume (see ExitReason).
	Crashed Status = 6
	// PermanentlyFailed is a terminal state reached when the configurable retry
	// policy's automated crash/stall recovery (session/retry_state.go) exhausts
	// MaxAttempts, or a failure reason isn't in RetryOn at all. Unlike Stopped, it
	// is not auto-revived by reconcileSessions even if tmux is alive — it's a
	// deliberate terminal state pending human action via "Retry now"
	// (Instance.RetryNow), not an incidental stop. See ADR-001.
	PermanentlyFailed Status = 7
	// Failed is a non-terminal state: the async creation pipeline (Background
	// Resolution Pipeline, Epic 2.2) failed before the session ever reached
	// Active. Distinct from Crashed (a previously-Active session whose process
	// later exited abnormally): Failed→Creating is a legal transition, used by
	// the retry path (Epic 1.2's TryStartRetry), while Crashed only recovers to
	// Active. See ADR-001 and SESSION_STATUS_FAILED in
	// proto/session/v1/types.proto.
	Failed Status = 8
)

// String returns a human-readable name for the status.
func (s Status) String() string {
	switch s {
	case Creating:
		return "Creating"
	case Active:
		return "Active"
	case Paused:
		return "Paused"
	case Stopped:
		return "Stopped"
	case Hibernated:
		return "Hibernated"
	case Restoring:
		return "Restoring"
	case Crashed:
		return "Crashed"
	case PermanentlyFailed:
		return "PermanentlyFailed"
	case Failed:
		return "Failed"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// IsSuspended reports whether a session in this status is not actively running
// and so must not be treated as a live target for background work that assumes
// a running process or tmux session (health checks, PR/CI status polling, etc.):
// Paused and Hibernated sessions have no tmux session at all; Stopped, Crashed,
// and PermanentlyFailed are terminal states awaiting an explicit resume/retry,
// not incidental gaps that background pollers should paper over. See
// healthCheckSkipReason (session/health.go) for the health-checker's per-status
// skip messages, and pr_status_poller.go's checkAllSessions for another consumer.
func (s Status) IsSuspended() bool {
	switch s {
	case Paused, Hibernated, Stopped, Crashed, PermanentlyFailed:
		return true
	default:
		return false
	}
}
