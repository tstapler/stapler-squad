package session

import (
	"fmt"
	"strings"
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

// controllerStatusDetection is the outcome of mapping a controller-reported Claude status
// to a queue decision. shouldAdd is false and reason/priority/ctx are zero values when no
// status-based condition applies (the caller should then check idle state).
type controllerStatusDetection struct {
	reason    AttentionReason
	priority  Priority
	shouldAdd bool
	ctx       string
}

// detectFromControllerStatus maps statusInfo.ClaudeStatus (as reported by an active
// controller) to a queue decision, delegating the reason/priority/default-context portion
// to AttentionReasonFromDetected. PendingApprovals is checked alongside StatusNeedsApproval
// because the controller may set the count before it advances the status string.
//
// IMPORTANT: Check Claude status FIRST before idle state handling (see detectFromIdleState).
// Status-based conditions (approval, input required, error) take priority over idle state
// because they represent explicit user prompts that need attention, even if terminal
// activity makes the session appear "active".
func detectFromControllerStatus(statusInfo InstanceStatusInfo, title string) controllerStatusDetection {
	switch {
	case statusInfo.ClaudeStatus == detection.StatusNeedsApproval || statusInfo.PendingApprovals > 0:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detection.StatusNeedsApproval)
		return controllerStatusDetection{
			reason:    reason,
			priority:  priority,
			shouldAdd: true,
			ctx:       effectiveCtx(statusInfo.StatusContext, defaultCtx),
		}
	case statusInfo.ClaudeStatus == detection.StatusInputRequired:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detection.StatusInputRequired)
		return controllerStatusDetection{
			reason:    reason,
			priority:  priority,
			shouldAdd: true,
			ctx:       effectiveCtx(statusInfo.StatusContext, defaultCtx),
		}
	case statusInfo.ClaudeStatus == detection.StatusError:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detection.StatusError)
		return controllerStatusDetection{
			reason:    reason,
			priority:  priority,
			shouldAdd: true,
			ctx:       effectiveCtx(statusInfo.StatusContext, defaultCtx),
		}
	case statusInfo.ClaudeStatus == detection.StatusTestsFailing:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detection.StatusTestsFailing)
		ctx := effectiveCtx(statusInfo.StatusContext, defaultCtx)
		log.Debug("tests failing", "session", title, "ctx", ctx)
		return controllerStatusDetection{reason: reason, priority: priority, shouldAdd: true, ctx: ctx}
	case statusInfo.ClaudeStatus == detection.StatusSuccess:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detection.StatusSuccess)
		ctx := effectiveCtx(statusInfo.StatusContext, defaultCtx)
		log.Debug("task complete", "session", title, "ctx", ctx)
		return controllerStatusDetection{reason: reason, priority: priority, shouldAdd: true, ctx: ctx}
	default:
		return controllerStatusDetection{}
	}
}

// idleStateDetection is the outcome of evaluating a controller's idle-state signal.
// remove=true means the caller should immediately return DetectionActionRemove.
type idleStateDetection struct {
	remove    bool
	shouldAdd bool
	reason    AttentionReason
	priority  Priority
	ctx       string
}

// detectFromIdleState evaluates statusInfo.IdleState.State. Only called when no
// status-based condition was already found by detectFromControllerStatus.
func detectFromIdleState(idleState detection.IdleState) idleStateDetection {
	switch idleState {
	case detection.IdleStateActive:
		// Actively working, remove from queue (but only if no prompt detected above).
		return idleStateDetection{remove: true}
	case detection.IdleStateTimeout:
		// Definite timeout - been idle too long.
		reason, priority, ctx := AttentionReasonFromDetected(detection.StatusIdle)
		return idleStateDetection{shouldAdd: true, reason: reason, priority: priority, ctx: ctx}
	default:
		// IdleStateWaiting (normal idle state, e.g. INSERT mode) or IdleStateUnknown -
		// don't add by default.
		return idleStateDetection{}
	}
}

// contentDetection is the outcome of scanning terminal content for a no-controller
// session. remove=true means the caller should immediately return DetectionActionRemove.
type contentDetection struct {
	claudeStatus detection.DetectedStatus
	remove       bool
	shouldAdd    bool
	reason       AttentionReason
	priority     Priority
	ctx          string
}

// detectFromContentLines scans terminal content line-by-line (bottom-up scan, via
// DetectWithContextFromLines) so recent terminal state wins over stale scrollback content,
// then maps the detected status to a queue decision. Only called when there is no active
// controller (either none wired or not yet started) and content is non-empty.
func detectFromContentLines(content string, detector detection.TerminalDetector, title string) contentDetection {
	lines := strings.Split(content, "\n")
	detectedStatus, statusContext := detector.DetectWithContextFromLines(lines)

	result := contentDetection{claudeStatus: detectedStatus}

	switch detectedStatus {
	case detection.StatusNeedsApproval:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detectedStatus)
		result.ctx = effectiveCtx(statusContext, defaultCtx)
		log.Debug("approval needed (no controller)", "session", title)
		result.shouldAdd, result.reason, result.priority = true, reason, priority
	case detection.StatusInputRequired:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detectedStatus)
		result.ctx = effectiveCtx(statusContext, defaultCtx)
		log.Debug("input required (no controller)", "session", title)
		result.shouldAdd, result.reason, result.priority = true, reason, priority
	case detection.StatusError:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detectedStatus)
		result.ctx = effectiveCtx(statusContext, defaultCtx)
		log.Debug("error detected (no controller)", "session", title)
		result.shouldAdd, result.reason, result.priority = true, reason, priority
	case detection.StatusTestsFailing:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detectedStatus)
		result.ctx = effectiveCtx(statusContext, defaultCtx)
		log.Debug("tests failing (no controller)", "session", title)
		result.shouldAdd, result.reason, result.priority = true, reason, priority
	case detection.StatusSuccess:
		reason, priority, defaultCtx := AttentionReasonFromDetected(detectedStatus)
		result.ctx = effectiveCtx(statusContext, defaultCtx)
		log.Debug("task complete (no controller)", "session", title)
		result.shouldAdd, result.reason, result.priority = true, reason, priority
	case detection.StatusExecuting, detection.StatusProcessing, detection.StatusWaitingForAgent:
		result.remove = true
	}

	return result
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

// applyStalenessOverride checks whether the session has gone quiet for longer than the
// configured staleness threshold and, if so and not already acknowledged, overrides the
// current reason/priority/context with ReasonStale — but only when there isn't already a
// higher-priority (Medium or above) reason set. Common to both the controller-active and
// no-controller paths.
func (d *DefaultStatusDeterminer) applyStalenessOverride(inst *Instance, shouldAdd bool, priority Priority, reason AttentionReason, ctx string) (bool, Priority, AttentionReason, string) {
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
	alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()

	if timeSinceOutput <= d.config.StalenessThreshold {
		if log.IsDebugEnabled() {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': NOT STALE - %s since last meaningful output (threshold: %s)",
				inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(d.config.StalenessThreshold))
		}
		return shouldAdd, priority, reason, ctx
	}

	if alreadyAcknowledged {
		if log.IsDebugEnabled() {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
				inst.Title)
		}
		return shouldAdd, priority, reason, ctx
	}

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

	return shouldAdd, priority, reason, ctx
}

// Determine evaluates a session's state and returns a DetectionResult.
// It is pure: no queue mutations, no storage calls, no side effects.
//
// It orchestrates, in order: (1) mapping the controller-reported Claude status or the
// terminal-content-detected status to a reason/priority via detectFromControllerStatus /
// detectFromContentLines (both of which delegate to AttentionReasonFromDetected for the
// DetectedStatus -> reason/priority/context portion), (2) idle-state / time-based fallback
// handling, (3) an uncommitted-changes worktree check, and (4) a staleness override.
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
		// IMPORTANT: Check Claude status FIRST before idle state handling.
		// Status-based conditions (approval, input required, error) take priority over
		// idle state because they represent explicit user prompts that need attention,
		// even if terminal activity makes the session appear "active".
		controllerResult := detectFromControllerStatus(statusInfo, inst.Title)
		reason, priority, shouldAdd, ctx = controllerResult.reason, controllerResult.priority, controllerResult.shouldAdd, controllerResult.ctx

		// Now handle idle state - but only if no status-based condition was detected above.
		// This ensures user prompts aren't hidden just because terminal is "active".
		// Use statusInfo.IdleState.State — already populated by GetStatus() via
		// controller.GetIdleStateInfo(). This avoids a redundant
		// GetController()+GetIdleState() call.
		if !shouldAdd {
			idleResult := detectFromIdleState(statusInfo.IdleState.State)
			if idleResult.remove {
				return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
			}
			reason, priority, shouldAdd, ctx = idleResult.reason, idleResult.priority, idleResult.shouldAdd, idleResult.ctx
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
			contentResult := detectFromContentLines(content, detector, inst.Title)
			claudeStatus = contentResult.claudeStatus
			if contentResult.remove {
				return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
			}
			reason, priority, shouldAdd, ctx = contentResult.reason, contentResult.priority, contentResult.shouldAdd, contentResult.ctx
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

	// Check for terminal staleness (no meaningful output for configured threshold).
	// IMPORTANT: Respect acknowledgment - don't flag as stale if user already acknowledged.
	shouldAdd, priority, reason, ctx = d.applyStalenessOverride(inst, shouldAdd, priority, reason, ctx)

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
