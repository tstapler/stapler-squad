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

// suppressedByAck reports whether inst was acknowledged (e.g. via the review queue's
// "skip" action) more recently than its last meaningful output — meaning nothing new has
// happened since the user dismissed it. Both Idle detection sites and the Stale check
// share this single helper so the suppression rule lives in exactly one place.
func (d *DefaultStatusDeterminer) suppressedByAck(inst *Instance) bool {
	return inst.IsAcknowledgedAfterOutput()
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

// verdict accumulates the queue decision while Determine walks its detection paths.
type verdict struct {
	reason        AttentionReason
	priority      Priority
	shouldAdd     bool
	ctx           string
	cleanWorktree bool
}

func (v *verdict) apply(dec statusDecision) {
	v.reason, v.priority, v.ctx, v.shouldAdd = dec.Reason, dec.Priority, dec.Context, true
}

func (v *verdict) markIdle() {
	v.reason, v.priority, v.ctx, v.shouldAdd = ReasonIdle, PriorityLow, "Session idle - ready for next task", true
}

// checkUncommitted flags a dirty worktree when nothing more urgent is queued
// (informational - user may want to review and commit).
func (d *DefaultStatusDeterminer) checkUncommitted(inst *Instance, v *verdict) {
	if (v.shouldAdd && v.priority != PriorityLow) || !inst.HasGitWorktree() {
		return
	}
	var newReason AttentionReason
	v.shouldAdd, v.priority, newReason, v.ctx, v.cleanWorktree = d.applyWorktreeCheck(inst, v.shouldAdd, v.priority)
	if newReason != "" {
		v.reason = newReason
	}
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
	// it is set when content is available.
	claudeStatus := statusInfo.ClaudeStatus
	var v verdict

	if statusInfo.IsControllerActive {
		if d.determineFromController(inst, statusInfo, &v) {
			return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
		}
	} else {
		var remove bool
		claudeStatus, remove = d.determineFromContent(inst, content, claudeStatus, detector, &v)
		if remove {
			return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
		}
	}

	d.applyStaleness(inst, &v)

	action := DetectionActionSkip
	if v.shouldAdd {
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
		(v.reason == ReasonTaskComplete || v.reason == ReasonIdle || v.reason == ReasonStale) {
		action = DetectionActionSkip
	}

	return DetectionResult{
		Action:        action,
		Reason:        v.reason,
		Priority:      v.priority,
		Context:       v.ctx,
		ClaudeStatus:  claudeStatus,
		CleanWorktree: v.cleanWorktree,
	}
}

// determineFromController handles sessions with an active controller. It reports true when
// the session should be removed from the queue.
func (d *DefaultStatusDeterminer) determineFromController(inst *Instance, statusInfo InstanceStatusInfo, v *verdict) (remove bool) {
	// Status-based conditions (approval, input required, error) take priority over idle
	// state: they are explicit user prompts, even if terminal activity looks "active".
	// PendingApprovals counts alongside StatusNeedsApproval because the controller may
	// set the count before it advances the status string.
	status := statusInfo.ClaudeStatus
	if statusInfo.PendingApprovals > 0 {
		status = detection.StatusNeedsApproval
	}
	if dec, ok := reasonForStatus(status, statusInfo.StatusContext); ok {
		v.apply(dec)
	} else if status == detection.StatusWaitingForAgent &&
		time.Since(inst.Snapshot().UpdatedAt) < waitingForAgentStuckThreshold {
		// Mirrors the no-controller grace-period logic: trust WaitingForAgent as real
		// background activity only while recently updated. A stale one falls through
		// to idle-state handling.
		return true
	}

	// Idle state only matters when no status-based condition was detected above,
	// so user prompts aren't hidden just because the terminal is "active".
	if !v.shouldAdd {
		switch statusInfo.IdleState.State {
		case detection.IdleStateActive:
			return true
		case detection.IdleStateTimeout:
			// Idle too long, unless the user already acknowledged this session and
			// nothing new has happened since.
			if !d.suppressedByAck(inst) {
				v.markIdle()
			}
		}
	}

	d.checkUncommitted(inst, v)
	return false
}

// determineFromContent handles sessions with no active controller by detecting status from
// terminal content. It returns the detected status and whether to remove the session.
func (d *DefaultStatusDeterminer) determineFromContent(inst *Instance, content string, claudeStatus detection.DetectedStatus, detector detection.TerminalDetector, v *verdict) (detection.DetectedStatus, bool) {
	if content != "" {
		// Line-by-line detection (bottom-up scan) so recent terminal state wins over stale
		// scrollback. DetectWithContext (full-block) fires error patterns on old
		// "Error: Exit code 1" output before reaching a current approval dialog at the bottom.
		detected, statusContext := detector.DetectWithContextFromLines(strings.Split(content, "\n"))
		claudeStatus = detected

		if dec, ok := reasonForStatus(detected, statusContext); ok {
			v.apply(dec)
			log.Debug("status detected (no controller)", "session", inst.Title, "reason", dec.Reason)
		} else if isActiveStatus(detected) {
			return claudeStatus, true
		} else if detected == detection.StatusWaitingForAgent && time.Since(inst.Snapshot().UpdatedAt) < waitingForAgentStuckThreshold {
			// Reachable via the always-present auto-mode footer override
			// (detection/detector.go's applyFooterIdleOverride), so it is not unconditional
			// evidence of activity: a stuck background shell would otherwise exclude an
			// idle session forever. Suppress only while recently updated.
			return claudeStatus, true
		}
	}

	// No status-based condition: fall back to time-based idle, unless the user already
	// acknowledged this session and nothing new has happened since.
	const basicIdleThreshold = 5 * time.Second
	if !v.shouldAdd && time.Since(inst.Snapshot().UpdatedAt) > basicIdleThreshold && !d.suppressedByAck(inst) {
		v.markIdle()
	}

	d.checkUncommitted(inst, v)
	return claudeStatus, false
}

func isActiveStatus(s detection.DetectedStatus) bool {
	return s == detection.StatusExecuting || s == detection.StatusProcessing || s == detection.StatusCompacting
}

// applyStaleness flags sessions with no meaningful output for the configured threshold.
// An acknowledged session is never flagged, and a Medium-or-higher reason is not overridden.
func (d *DefaultStatusDeterminer) applyStaleness(inst *Instance, v *verdict) {
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
	if timeSinceOutput <= d.config.StalenessThreshold || d.suppressedByAck(inst) {
		return
	}
	if v.shouldAdd && !v.priority.IsLowerThan(PriorityMedium) {
		return
	}
	v.reason = ReasonStale
	v.priority = PriorityLow
	v.shouldAdd = true
	v.ctx = fmt.Sprintf("No activity for %s - session may be stuck or waiting",
		detection.FormatDuration(timeSinceOutput))
}
