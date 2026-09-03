package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

// waitingForAgentStuckThreshold bounds how long the no-controller path trusts
// StatusWaitingForAgent as unconditional evidence of real background activity, before
// treating it as a possibly-stuck/orphaned background shell and falling through to the
// normal time-based idle re-add check instead. It's deliberately much larger than
// basicIdleThreshold (below): the auto-mode footer count alone can't distinguish a
// healthy long-running background task from one that's stalled, so this is a coarse
// proxy — "background work has plausibly stalled" — not a precise stuck-detector.
const waitingForAgentStuckThreshold = 30 * time.Minute

// DetectionAction represents what the poller should do after status determination.
type DetectionAction int

const (
	DetectionActionSkip   DetectionAction = iota // No change to queue
	DetectionActionAdd                           // Add/update item in queue
	DetectionActionRemove                        // Remove item from queue
)

// DetectionResult is the output of status determination — pure data, no side effects.
type DetectionResult struct {
	Action       DetectionAction
	Reason       AttentionReason
	Priority     Priority
	Context      string
	ClaudeStatus detection.DetectedStatus
	// CleanWorktree is true when the worktree was inspected and found clean.
	// checkSession uses this to remove a queued UncommittedChanges entry immediately.
	CleanWorktree bool
}

// IsHighPriority returns true when the result warrants bypassing grace-period suppression.
func (r DetectionResult) IsHighPriority() bool {
	return r.Priority <= PriorityHigh
}

// StatusDeterminer evaluates whether a session should be added to, removed from,
// or left unchanged in the review queue. It is a pure function — no queue operations.
type StatusDeterminer interface {
	Determine(
		inst *Instance,
		content string,
		statusInfo InstanceStatusInfo,
		detector detection.TerminalDetector,
	) DetectionResult
}

// DefaultStatusDeterminer implements StatusDeterminer with the standard detection logic.
type DefaultStatusDeterminer struct {
	config ReviewQueuePollerConfig
}

// NewDefaultStatusDeterminer creates a DefaultStatusDeterminer with the given config.
func NewDefaultStatusDeterminer(config ReviewQueuePollerConfig) *DefaultStatusDeterminer {
	return &DefaultStatusDeterminer{config: config}
}

// effectiveCtx returns provided when non-empty, otherwise fallback.
func effectiveCtx(provided, fallback string) string {
	if provided != "" {
		return provided
	}
	return fallback
}

// applyWorktreeCheck inspects the git worktree of inst and potentially overrides the
// current add/priority state with an UncommittedChanges reason, or sets CleanWorktree.
// Only called when the session has a git worktree attached.
func (d *DefaultStatusDeterminer) applyWorktreeCheck(inst *Instance, shouldAdd bool, priority Priority) (newShouldAdd bool, newPriority Priority, newReason AttentionReason, newCtx string, cleanWorktree bool) {
	worktree, err := inst.GetGitWorktree()
	if err != nil {
		log.Warn("failed to get git worktree", "session", inst.Title, "err", err)
		return shouldAdd, priority, "", "", false
	}
	if worktree == nil {
		return shouldAdd, priority, "", "", false
	}
	isDirty, err := worktree.IsDirty()
	if err != nil {
		log.Warn("failed to check git status", "session", inst.Title, "err", err)
		return shouldAdd, priority, "", "", false
	}
	if isDirty {
		if !shouldAdd || priority == PriorityLow {
			log.Debug("uncommitted changes detected", "session", inst.Title)
			return true, PriorityLow, ReasonUncommittedChanges, "Uncommitted changes ready to commit", false
		}
		return shouldAdd, priority, "", "", false
	}
	// Worktree is clean — signal caller to remove any UncommittedChanges entry.
	return shouldAdd, priority, "", "", true
}

// Determine evaluates a session's state and returns a DetectionResult.
// It is pure: no queue mutations, no storage calls, no side effects.
func (d *DefaultStatusDeterminer) Determine(
	inst *Instance,
	content string,
	statusInfo InstanceStatusInfo,
	detector detection.TerminalDetector,
) DetectionResult {
	// claudeStatus captures the raw DetectedStatus from whichever detection path ran.
	// For controller sessions this is statusInfo.ClaudeStatus; for no-controller sessions
	// it is set inside the else block when content is available.
	claudeStatus := statusInfo.ClaudeStatus

	var reason AttentionReason
	var priority Priority
	var shouldAdd bool
	var ctx string
	cleanWorktree := false

	if statusInfo.IsControllerActive {
		// Use statusInfo.IdleState.State — already populated by GetStatus() via controller.GetIdleStateInfo().
		// This avoids a redundant GetController()+GetIdleState() call.
		idleState := statusInfo.IdleState.State

		// IMPORTANT: Check Claude status FIRST before idle state handling.
		// Status-based conditions (approval, input required, error) take priority over
		// idle state because they represent explicit user prompts that need attention,
		// even if terminal activity makes the session appear "active".

		// Map controller-reported Claude status to queue action.
		// PendingApprovals is checked alongside StatusNeedsApproval because the controller
		// may set the count before it advances the status string.
		switch {
		case statusInfo.ClaudeStatus == detection.StatusNeedsApproval || statusInfo.PendingApprovals > 0:
			reason = ReasonApprovalPending
			priority = PriorityHigh
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Waiting for approval to proceed")
		case statusInfo.ClaudeStatus == detection.StatusInputRequired:
			reason = ReasonInputRequired
			priority = PriorityMedium
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Waiting for explicit user input")
		case statusInfo.ClaudeStatus == detection.StatusError:
			reason = ReasonErrorState
			priority = PriorityUrgent
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Error state detected")
		case statusInfo.ClaudeStatus == detection.StatusTestsFailing:
			reason = ReasonTestsFailing
			priority = PriorityHigh
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Tests are failing")
			log.Debug("tests failing", "session", inst.Title, "ctx", ctx)
		case statusInfo.ClaudeStatus == detection.StatusSuccess:
			reason = ReasonTaskComplete
			priority = PriorityLow
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Task completed successfully")
			log.Debug("task complete", "session", inst.Title, "ctx", ctx)
		}

		// Now handle idle state - but only if no status-based condition was detected above.
		// This ensures user prompts aren't hidden just because terminal is "active".
		if !shouldAdd {
			switch idleState {
			case detection.IdleStateActive:
				// Actively working, remove from queue (but only if no prompt detected above)
				return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}

			case detection.IdleStateWaiting:
				// Normal idle state (e.g., INSERT mode) - don't add by default
				shouldAdd = false

			case detection.IdleStateTimeout:
				// Definite timeout - been idle too long
				reason = ReasonIdle
				priority = PriorityLow
				shouldAdd = true
				ctx = "Session idle - ready for next task"
			}
		}

		// Check for uncommitted changes (informational - user may want to review and commit)
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			var newReason AttentionReason
			shouldAdd, priority, newReason, ctx, cleanWorktree = d.applyWorktreeCheck(inst, shouldAdd, priority)
			if newReason != "" {
				reason = newReason
			}
		}
	} else {
		// No active controller (either none wired or not yet started) — detect status
		// from terminal content.
		if content != "" {
			// Use line-by-line detection (bottom-up scan) so recent terminal state wins
			// over stale scrollback content. DetectWithContext (full-block) fires error
			// patterns on old "Error: Exit code 1" output before reaching a current
			// approval dialog at the bottom of the pane.
			lines := strings.Split(content, "\n")
			detectedStatus, statusContext := detector.DetectWithContextFromLines(lines)
			claudeStatus = detectedStatus

			// Map terminal-detected status to queue action.
			switch detectedStatus {
			case detection.StatusNeedsApproval:
				reason = ReasonApprovalPending
				priority = PriorityHigh
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Waiting for approval to proceed")
				log.Debug("approval needed (no controller)", "session", inst.Title)
			case detection.StatusInputRequired:
				reason = ReasonInputRequired
				priority = PriorityMedium
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Waiting for explicit user input")
				log.Debug("input required (no controller)", "session", inst.Title)
			case detection.StatusError:
				reason = ReasonErrorState
				priority = PriorityUrgent
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Error state detected")
				log.Debug("error detected (no controller)", "session", inst.Title)
			case detection.StatusTestsFailing:
				reason = ReasonTestsFailing
				priority = PriorityHigh
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Tests are failing")
				log.Debug("tests failing (no controller)", "session", inst.Title)
			case detection.StatusSuccess:
				reason = ReasonTaskComplete
				priority = PriorityLow
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Task completed successfully")
				log.Debug("task complete (no controller)", "session", inst.Title)
			case detection.StatusExecuting, detection.StatusProcessing, detection.StatusCompacting:
				return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
			case detection.StatusWaitingForAgent:
				// Unlike Executing/Processing/Compacting, WaitingForAgent is now reachable
				// via the always-present auto-mode footer override (detection/detector.go's
				// applyFooterIdleOverride) rather than only a genuinely-active spinner line —
				// so it can no longer be trusted as unconditional evidence of real activity.
				// A stuck/orphaned background shell that never decrements would otherwise
				// exclude an actually-idle session from the review queue indefinitely. Only
				// suppress it while recently updated; once stale, fall through to the normal
				// time-based re-add check below.
				if time.Since(inst.UpdatedAt) < waitingForAgentStuckThreshold {
					return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
				}
			}
		}

		// If no status-based condition was detected, fall back to time-based checks
		if !shouldAdd {
			// Check if session has been idle for a long time based on UpdatedAt
			const basicIdleThreshold = 5 * time.Second
			if time.Since(inst.UpdatedAt) > basicIdleThreshold {
				reason = ReasonIdle
				priority = PriorityLow
				shouldAdd = true
				ctx = "Session idle - ready for next task"
			}
		}

		// Check for uncommitted changes (informational - user may want to review and commit)
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			var newReason AttentionReason
			shouldAdd, priority, newReason, ctx, cleanWorktree = d.applyWorktreeCheck(inst, shouldAdd, priority)
			if newReason != "" {
				reason = newReason
			}
		}
	}

	// Check for terminal staleness (no meaningful output for configured threshold)
	// IMPORTANT: Respect acknowledgment - don't flag as stale if user already acknowledged
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
	alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()

	if timeSinceOutput > d.config.StalenessThreshold {
		if alreadyAcknowledged {
			if log.IsDebugEnabled() {
				log.DebugLog().Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
					inst.Title)
			}
		} else {
			// Only override if we don't already have a higher-priority reason.
			// Only set stale if not already flagged with Medium priority or higher.
			if !shouldAdd || priority.IsLowerThan(PriorityMedium) {
				reason = ReasonStale
				priority = PriorityLow
				shouldAdd = true
				ctx = fmt.Sprintf("No activity for %s - session may be stuck or waiting",
					detection.FormatDuration(timeSinceOutput))
				if log.IsDebugEnabled() {
					log.DebugLog().Printf("[ReviewQueue] Session '%s': STALENESS DETECTED - flagged as stale, %s since last meaningful output",
						inst.Title, detection.FormatDuration(timeSinceOutput))
				}
			} else if log.IsDebugEnabled() {
				log.DebugLog().Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
					inst.Title, reason.String())
			}
		}
	}

	action := DetectionActionSkip
	if shouldAdd {
		action = DetectionActionAdd
	}

	// Hidden (system/background) instances must never surface TaskComplete/Idle/Stale
	// notifications — those reasons are routine, low-signal states that a hidden session
	// shouldn't interrupt the user for. ReasonErrorState and ReasonTestsFailing are
	// deliberately excluded from this narrowing: no other durable detector watches a
	// still-alive, stuck-in-error Hidden review session, so those must still surface even
	// when Hidden. This gate is reason-scoped and lives here (not as an early return) so it
	// applies uniformly regardless of which caller (ReviewQueuePoller.checkSession,
	// StartupScanner.Scan) invokes Determine.
	if inst.Hidden && action == DetectionActionAdd &&
		(reason == ReasonTaskComplete || reason == ReasonIdle || reason == ReasonStale) {
		action = DetectionActionSkip
	}

	return DetectionResult{
		Action:        action,
		Reason:        reason,
		Priority:      priority,
		Context:       ctx,
		ClaudeStatus:  claudeStatus,
		CleanWorktree: cleanWorktree,
	}
}
