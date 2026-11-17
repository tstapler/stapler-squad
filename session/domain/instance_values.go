package domain

import (
	"time"
)

// LifecycleEvent is a notification type emitted by an Instance when key state
// transitions occur (e.g., the session starts, or the program exits unexpectedly).
type LifecycleEvent int

const (
	// EventStarted fires at the end of start() when the instance has successfully
	// transitioned to Running and the controller is up.
	EventStarted LifecycleEvent = iota
	// EventExited fires when the underlying program exits unexpectedly (not via an
	// operator-initiated Kill/Stop). Callers may use this to drive auto-restart logic.
	EventExited
	// EventStopped fires when Destroy() tears down the instance via an explicit
	// operator-initiated Kill/Stop (e.g. the stop_session MCP tool, DeleteSession
	// RPC, or backlog stale-work remediation). Kept distinct from EventExited so a
	// future auto-restart listener can still ignore deliberate stops, while
	// listeners that only care "is this session now gone" (e.g.
	// BacklogLifecycleListener's ItemSession.EndedAt bookkeeping) can subscribe to
	// both.
	EventStopped
)

// ReasonColdRestoreLostHistory is passed to fireLifecycleEvent(EventStarted, ...)
// when a cold restore was forced fresh after recoverConversationBeforeLaunch's
// recovery attempt found nothing, despite EverHadConversationHistory being true
// (session-revive-uuid-loss AC3).
const ReasonColdRestoreLostHistory = "cold-restore-fresh-lost-history"

// LifecycleListener is implemented by any component that wants to receive Instance
// lifecycle notifications. Implementations must be non-blocking; use a goroutine
// or channel if the handler needs to do significant work.
type LifecycleListener interface {
	OnLifecycleEvent(event LifecycleEvent, reason string)
}

// reasonForReviveOutcome returns the fireLifecycleEvent(EventStarted, ...)
// reason for outcome — non-empty only for ReviveOutcomeFreshLostHistory, so
// the notification listener in server/services/session_service.go can filter
// on the reason string alone (session-revive-uuid-loss AC3).
func ReasonForReviveOutcome(outcome ReviveOutcome) string {
	if outcome == ReviveOutcomeFreshLostHistory {
		return ReasonColdRestoreLostHistory
	}
	return ""
}

// reviveOutcomeForColdRestore derives the ColdRestore branch's ReviveOutcome
// (session-revive-uuid-loss AC3) from its three decision inputs. Pure and
// side-effect-free so startLocked/start share one definition instead of
// duplicating the branch logic.
func ReviveOutcomeForColdRestore(hadUUIDBeforeRecovery, hasClaudeSession, everHadHistory bool) ReviveOutcome {
	if hasClaudeSession {
		if hadUUIDBeforeRecovery {
			return ReviveOutcomeResumeLive
		}
		return ReviveOutcomeResumeRecovered
	}
	if everHadHistory {
		return ReviveOutcomeFreshLostHistory
	}
	return ReviveOutcomeFreshExpected
}

// GitHubResolution carries the fields a deferred GitHub-URL clone resolves to
// (Epic 2.1, async-session-creation): SessionService.CreateSession publishes
// an instance immediately, before the clone runs, using the raw request
// path/branch as placeholders; once the background clone completes, it calls
// SetGitHubResolution with the real values. Grouped into one struct rather
// than a same-typed-string parameter pile (see
// .claude/rules/primitive-obsession-checklist.md).
type GitHubResolution struct {
	Path           string
	Branch         string
	Owner          string
	Repo           string
	SourceRef      string
	ClonedRepoPath string
	PRNumber       int
	PRURL          string
}

// RetryAttemptRecord is one entry in RetryState.RetryHistory.
type RetryAttemptRecord struct {
	Attempt   int       `json:"attempt"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// maxRetryHistoryEntries caps RetryState.RetryHistory so a long-lived,
// repeatedly crash-looping session doesn't grow this slice unboundedly.
const maxRetryHistoryEntries = 10

// RetryState holds the automated crash/stall retry lifecycle for one session:
// attempt count, resolved max, last failure reason, pending-retry timestamp,
// and history. Embedded (by value) into Instance, mirroring ReviewState.
// Protected by inst.mu.
type RetryState struct {
	// RetryAttempt is the number of automated retry attempts consumed so far in
	// the current failure episode. Reset to 0 by RetryNow().
	RetryAttempt int `json:"retry_attempt,omitempty"`
	// RetryMaxAttempts is the policy-resolved cap, snapshotted at driver start
	// (or by RetryNow) so a live config edit mid-cycle doesn't change the cap a
	// session is already partway through.
	RetryMaxAttempts int `json:"retry_max_attempts,omitempty"`
	// LastFailureReason is the most recent classifyFailureReason result:
	// "crashed", "stalled", or "tmux_exited".
	LastFailureReason string `json:"last_failure_reason,omitempty"`
	// NextRetryAt is when the next backed-off restart is due. Zero means no
	// retry is pending.
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
	// RetryHistory records reason + timestamp per attempt, newest-last, capped
	// at maxRetryHistoryEntries. Preserved across RetryNow() episodes (AC5 wants
	// history to survive a manual retry, not just the automated ones).
	RetryHistory []RetryAttemptRecord `json:"retry_history,omitempty"`
}

// RecordAttempt appends a RetryAttemptRecord to RetryHistory, dropping the
// oldest entry once maxRetryHistoryEntries is exceeded. Caller must hold
// inst.mu.
func (rs *RetryState) RecordAttempt(attempt int, reason string, now time.Time) {
	rs.RetryHistory = append(rs.RetryHistory, RetryAttemptRecord{Attempt: attempt, Reason: reason, Timestamp: now})
	if len(rs.RetryHistory) > maxRetryHistoryEntries {
		rs.RetryHistory = rs.RetryHistory[len(rs.RetryHistory)-maxRetryHistoryEntries:]
	}
}

// ResetEpisode clears the attempt/backoff bookkeeping back to a fresh episode, used
// by RetryNow(). RetryHistory is deliberately preserved. Caller must hold
// inst.mu.
func (rs *RetryState) ResetEpisode() {
	rs.RetryAttempt = 0
	rs.LastFailureReason = ""
	rs.NextRetryAt = time.Time{}
}

// GetManagedPermissions returns full permissions for squad-managed instances
func GetManagedPermissions() InstancePermissions {
	return InstancePermissions{
		CanView:              true,
		CanAttach:            true,
		CanSendCommand:       true,
		CanPause:             true,
		CanResume:            true,
		CanDestroy:           true,
		CanModifyGit:         true,
		CanAddToQueue:        true,
		RequiresConfirmation: make(map[string]bool),
	}
}

// GetExternalPermissions returns limited permissions for external instances
// allowAttach controls whether attach operations are permitted (power user mode)
func GetExternalPermissions(allowAttach bool) InstancePermissions {
	perms := InstancePermissions{
		CanView:              true,
		CanAttach:            allowAttach,
		CanSendCommand:       allowAttach,
		CanPause:             false,
		CanResume:            false,
		CanDestroy:           false,
		CanModifyGit:         false,
		CanAddToQueue:        false,
		RequiresConfirmation: make(map[string]bool),
	}

	// If attach is allowed, require confirmation
	if allowAttach {
		perms.RequiresConfirmation["attach"] = true
		perms.RequiresConfirmation["send"] = true
	}

	return perms
}

// Workspace describes where a session is operating. It names the four path
// concepts the domain actually has; picking the wrong one has already shipped a
// bug. Why, and the full story: .claude/rules/instance-lock-free-reads.md.
//
// Which one you want:
//
//	comparing two sessions' locations  -> ActiveDir
//	reading or writing files           -> ExistingDir
//	"same repository?"                 -> RepoRoot, or Instance.WorkspaceKey()
//
// Use Instance.Workspace() to obtain this value; do not construct directly.
type Workspace struct {
	// RepoRoot is the path the session was created against, as recorded on
	// Instance.Path. For a session created from a pre-existing worktree this is
	// that worktree, not the main checkout -- Instance.MainRepoPath holds the
	// main checkout in that case.
	RepoRoot string

	// WorktreeDir is the dedicated git worktree created for this session, or ""
	// when the session works directly in RepoRoot.
	WorktreeDir string

	// ActiveDir is where this session works: WorktreeDir if it has one, else
	// RepoRoot. Deliberately does NOT check whether the directory still exists,
	// so it stays stable across a worktree cleanup -- which makes it the correct
	// key for comparing, correlating, or identifying sessions, and the right
	// default when you are unsure.
	ActiveDir string

	// ExistingDir is ActiveDir when that directory exists on disk, else
	// RepoRoot. Use it only to open files: pause_session deletes a worktree
	// while keeping its branch, and without the fallback filesystem callers
	// surface a bare "directory not found: .".
	//
	// Lossy by construction -- two sessions whose worktrees were both cleaned up
	// share an ExistingDir without being in the same place, so never compare it.
	ExistingDir string
}
