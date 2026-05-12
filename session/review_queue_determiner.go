package session

import (
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

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
		detector *detection.StatusDetector,
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

// Determine evaluates a session's state and returns a DetectionResult.
// It is pure: no queue mutations, no storage calls, no side effects.
func (d *DefaultStatusDeterminer) Determine(
	inst *Instance,
	content string,
	statusInfo InstanceStatusInfo,
	detector *detection.StatusDetector,
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
			log.InfoLog.Printf("[ReviewQueue] Session '%s': Tests failing - %s", inst.Title, ctx)
		case statusInfo.ClaudeStatus == detection.StatusSuccess:
			reason = ReasonTaskComplete
			priority = PriorityLow
			shouldAdd = true
			ctx = effectiveCtx(statusInfo.StatusContext, "Task completed successfully")
			log.InfoLog.Printf("[ReviewQueue] Session '%s': Task completion - %s", inst.Title, ctx)
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
		// Only check if we don't already have a higher-priority reason
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
			} else if worktree != nil {
				isDirty, err := worktree.IsDirty()
				if err != nil {
					log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to check git status: %v", inst.Title, err)
					log.LogForSession(inst.Title, "warning", "Failed to check git status: %v", err)
				} else if isDirty {
					if !shouldAdd || priority == PriorityLow {
						reason = ReasonUncommittedChanges
						priority = PriorityLow
						shouldAdd = true
						ctx = "Uncommitted changes ready to commit"
						log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
					}
				} else {
					// Worktree is clean — signal caller to remove any UncommittedChanges entry.
					cleanWorktree = true
				}
			}
		}
	} else {
		// No active controller (either none wired or not yet started) — detect status
		// from terminal content.
		if content != "" {
			// Detect status from terminal content using the shared status detector
			detectedStatus, statusContext := detector.DetectWithContext([]byte(content))
			claudeStatus = detectedStatus

			// Map terminal-detected status to queue action.
			switch detectedStatus {
			case detection.StatusNeedsApproval:
				reason = ReasonApprovalPending
				priority = PriorityHigh
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Waiting for approval to proceed")
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Approval needed (no controller) - %s", inst.Title, ctx)
			case detection.StatusInputRequired:
				reason = ReasonInputRequired
				priority = PriorityMedium
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Waiting for explicit user input")
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Input required (no controller) - %s", inst.Title, ctx)
			case detection.StatusError:
				reason = ReasonErrorState
				priority = PriorityUrgent
				shouldAdd = true
				ctx = effectiveCtx(statusContext, "Error state detected")
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Error detected (no controller) - %s", inst.Title, ctx)
			case detection.StatusActive, detection.StatusProcessing:
				return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
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
		// Only check if we don't already have a higher-priority reason
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
			} else if worktree != nil {
				isDirty, err := worktree.IsDirty()
				if err != nil {
					log.Warn("failed to check git status", "session", inst.Title, "err", err)
				} else if isDirty {
					if !shouldAdd || priority == PriorityLow {
						reason = ReasonUncommittedChanges
						priority = PriorityLow
						shouldAdd = true
						ctx = "Uncommitted changes ready to commit"
						log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
					}
				} else {
					// Worktree is clean — signal caller to remove any UncommittedChanges entry.
					cleanWorktree = true
				}
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
				log.DebugLog.Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
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
					log.DebugLog.Printf("[ReviewQueue] Session '%s': STALENESS DETECTED - flagged as stale, %s since last meaningful output",
						inst.Title, detection.FormatDuration(timeSinceOutput))
				}
			} else if log.IsDebugEnabled() {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
					inst.Title, reason.String())
			}
		}
	} else if log.IsDebugEnabled() {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': NOT STALE - %s since last meaningful output (threshold: %s)",
			inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(d.config.StalenessThreshold))
	}

	action := DetectionActionSkip
	if shouldAdd {
		action = DetectionActionAdd
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
