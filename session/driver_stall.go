package session

import (
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// workingStallMultiplier stretches the interactive stall budget while the
// detector reports the agent mid-turn: a long tool call or "thinking" spinner
// emits no meaningful output but is not a hang.
const workingStallMultiplier = 2

// stallVerdict is decideStall's outcome for one inactivity tick.
type stallVerdict int

const (
	// stallWithinBudget: nothing to do yet.
	stallWithinBudget stallVerdict = iota
	// stallRestart: an autonomous (backlog) session went quiet; the existing
	// stall -> retry -> PermanentlyFailed policy applies.
	stallRestart
	// stallNeedsAttention: an interactive session's live pane went quiet; flag
	// it for the operator, never kill or fail it.
	stallNeedsAttention
	// stallPaneDead: an interactive session's pane is gone, a genuine failure.
	stallPaneDead
)

func (v stallVerdict) String() string {
	switch v {
	case stallRestart:
		return "restart"
	case stallNeedsAttention:
		return "needs_attention"
	case stallPaneDead:
		return "pane_dead"
	default:
		return "within_budget"
	}
}

// stallEvidence is everything the driver knows at one inactivity tick.
type stallEvidence struct {
	Autonomous       bool
	Status           detection.DetectedStatus
	ControllerActive bool
	PaneAlive        bool
	Idle             time.Duration
	// Budget is the quiet period the verdict is measured against: for an
	// autonomous session the (possibly nudge-shortened) restart grace, for an
	// interactive one the attention budget before the working-status stretch.
	Budget time.Duration
}

func isWorkingStatus(s detection.DetectedStatus) bool {
	switch s {
	case detection.StatusProcessing, detection.StatusExecuting,
		detection.StatusCompacting, detection.StatusWaitingForAgent:
		return true
	default:
		return false
	}
}

// decideStall is the pure stall policy. Autonomous sessions keep the original
// "quiet past the grace -> restart" rule. Interactive sessions are never
// restarted on quiet alone: a dead pane is a failure; a live pane is only
// flagged, after a longer budget while the detector says it is working.
func decideStall(e stallEvidence) stallVerdict {
	if e.Autonomous {
		if e.Idle > e.Budget {
			return stallRestart
		}
		return stallWithinBudget
	}
	if !e.PaneAlive && e.Idle > driverInactivityTimeout {
		return stallPaneDead
	}
	budget := e.Budget
	if isWorkingStatus(e.Status) {
		budget *= workingStallMultiplier
	}
	if e.Idle > budget {
		return stallNeedsAttention
	}
	return stallWithinBudget
}

// isAutonomousSession reports whether the backlog pipeline owns inst, so the
// automated stall -> retry policy (not operator attention) applies to it.
func isAutonomousSession(inst *instance.Instance) bool {
	return inst.IsBacklogOriginatedSession() || inst.HasTag(instance.TagBacklogRevision) || instance.IsOneShot(inst)
}

// interactiveStallBudget is policy's attention budget, defaulting when unset
// (a zero RetryPolicy, as in tests and one-off callers).
func interactiveStallBudget(policy instance.RetryPolicy) time.Duration {
	if policy.InteractiveStallTimeout > 0 {
		return policy.InteractiveStallTimeout
	}
	return config.RetryPolicyConfig{}.InteractiveStallTimeoutOrDefault()
}

// gatherStallEvidence reads the live signals. Only call once idle has passed
// the cheapest threshold: PaneAlive costs a fresh tmux liveness check.
func gatherStallEvidence(inst *instance.Instance, idle, budget time.Duration) stallEvidence {
	status, controllerActive := inst.GetDetectedStatusInfo()
	return stallEvidence{
		Autonomous:       isAutonomousSession(inst),
		Status:           status,
		ControllerActive: controllerActive,
		PaneAlive:        inst.HasLivePaneNoCache(),
		Idle:             idle,
		Budget:           budget,
	}
}

// logStallDecision records every input to a non-trivial stall verdict so the
// next occurrence is explainable from the log alone.
func logStallDecision(inst *instance.Instance, e stallEvidence, v stallVerdict, policy instance.RetryPolicy, now time.Time) {
	rs := inst.RetrySnapshot()
	log.Warn("SessionDriver: stall decision",
		"session", inst.Title,
		"verdict", v.String(),
		"autonomous", e.Autonomous,
		"detected_status", e.Status.String(),
		"controller_active", e.ControllerActive,
		"pane_alive", e.PaneAlive,
		"idle", e.Idle.Round(time.Second),
		"last_output_age", now.Sub(inst.LastMeaningfulOutputTime()).Round(time.Second),
		"budget", e.Budget,
		"retry_attempt", rs.RetryAttempt,
		"retry_max_attempts", policy.MaxAttempts,
	)
}

// noteInteractiveStall logs a quiet interactive session once per quiet episode.
// It deliberately raises no review-queue entry or notification: the review
// queue's ReasonStale badge (acknowledgement-aware) and StaleSessionNotifier
// already surface quiet active sessions and clear when output resumes, so a
// third path here would duplicate them and ignore acknowledgement.
func noteInteractiveStall(inst *instance.Instance, e stallEvidence, policy instance.RetryPolicy, now time.Time) {
	logStallDecision(inst, e, stallNeedsAttention, policy, now)
}

// deadPaneConfirmWindow is how long an interactive pane must probe dead,
// without an alive probe in between, before the session is failed.
const deadPaneConfirmWindow = 30 * time.Second

// interactiveDriverShouldEnd reports lifecycle states in which an interactive
// driver has nothing left to watch. Ending lets a later start (e.g. after a
// resume) launch a fresh driver instead of no-opping on a stale one.
func interactiveDriverShouldEnd(inst *instance.Instance) bool {
	if inst.IsArchived() {
		return true
	}
	switch inst.Snapshot().Status {
	case domain.Hibernated, domain.Crashed:
		return true
	default:
		return false
	}
}

// driverTotalDeadline bounds a driver run. Only autonomous sessions get the
// safety-net deadline: an interactive driver must outlive its stall budget
// (30m, 60m while working) or the quiet flag could never fire. Interactive
// drivers still end on stop, destroy, pause, archive, hibernate or crash (see
// interactiveDriverShouldEnd). Zero means unbounded.
func driverTotalDeadline(inst *instance.Instance, start time.Time) time.Time {
	if isAutonomousSession(inst) {
		return start.Add(currentDriverTiming().total())
	}
	return time.Time{}
}
