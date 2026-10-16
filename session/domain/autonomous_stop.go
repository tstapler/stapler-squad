package domain

import "time"

// StopReason says why an autonomous driver run ended. The empty value is a
// legacy/unspecified outcome and is treated as a stall.
type StopReason string

const (
	StopReasonDone               StopReason = "done"
	StopReasonMaxTurns           StopReason = "max_turns"
	StopReasonStartupTimeout     StopReason = "startup_timeout"
	StopReasonOrchestratorError  StopReason = "orchestrator_error"
	StopReasonSubmitFailed       StopReason = "submit_failed"
	StopReasonLeaseTimeout       StopReason = "lease_timeout"
	StopReasonRateLimitExhausted StopReason = "rate_limit_exhausted"
	StopReasonCancelledExternal  StopReason = "cancelled_external"
	StopReasonCancelledShutdown  StopReason = "cancelled_shutdown"
	StopReasonPanic              StopReason = "panic"
	// StopReasonStaleWork and StopReasonDeadPane are recorded by the stale-work
	// detector and the health checker, which restart a session without a driver stop.
	StopReasonStaleWork StopReason = "stale_work"
	StopReasonDeadPane  StopReason = "dead_pane"
)

// AllStopReasons lists every defined StopReason (not the empty legacy value).
// Tests derive their tables from it; add a constant here and the exhaustive
// tests demand a decision row for it.
var AllStopReasons = []StopReason{
	StopReasonDone,
	StopReasonMaxTurns,
	StopReasonStartupTimeout,
	StopReasonOrchestratorError,
	StopReasonSubmitFailed,
	StopReasonLeaseTimeout,
	StopReasonRateLimitExhausted,
	StopReasonCancelledExternal,
	StopReasonCancelledShutdown,
	StopReasonPanic,
	StopReasonStaleWork,
	StopReasonDeadPane,
}

// OutcomeClass is the three-way split callers branch on, instead of a bare
// Done bool that cannot tell a cancel from a stall.
type OutcomeClass string

const (
	OutcomeClassDone      OutcomeClass = "done"
	OutcomeClassStall     OutcomeClass = "stall"
	OutcomeClassCancelled OutcomeClass = "cancelled"
)

// Class derives the OutcomeClass. Unknown and empty reasons are stalls so a
// legacy outcome is never silently treated as success or as a requested stop.
func (r StopReason) Class() OutcomeClass {
	switch r {
	case StopReasonDone:
		return OutcomeClassDone
	case StopReasonCancelledExternal, StopReasonCancelledShutdown:
		return OutcomeClassCancelled
	default:
		return OutcomeClassStall
	}
}

// IsDone reports whether the run finished its work.
func (r StopReason) IsDone() bool { return r.Class() == OutcomeClassDone }

// IsStall reports that the work did not finish and nobody asked for the stop.
func (r StopReason) IsStall() bool { return r.Class() == OutcomeClassStall }

// IsValid reports whether r is one of AllStopReasons.
func (r StopReason) IsValid() bool {
	for _, v := range AllStopReasons {
		if v == r {
			return true
		}
	}
	return false
}

func (r StopReason) String() string { return string(r) }

// ObservedState is what the pane was doing at stop time, independent of why
// the driver stopped.
type ObservedState string

const (
	ObservedIdlePrompt    ObservedState = "idle_prompt"
	ObservedNeedsApproval ObservedState = "needs_approval"
	ObservedIdle          ObservedState = "idle"
	ObservedWorking       ObservedState = "working"
	ObservedError         ObservedState = "error"
	ObservedExited        ObservedState = "exited"
	ObservedCrashed       ObservedState = "crashed"
	ObservedUnknown       ObservedState = "unknown"
)

// AllObservedStates lists every ObservedState.
var AllObservedStates = []ObservedState{
	ObservedIdlePrompt,
	ObservedNeedsApproval,
	ObservedIdle,
	ObservedWorking,
	ObservedError,
	ObservedExited,
	ObservedCrashed,
	ObservedUnknown,
}

func (s ObservedState) String() string { return string(s) }

// StatusBucket is the detector status folded to the few classes the
// classifier needs; the fold lives in package session, which owns detection.
type StatusBucket string

const (
	BucketSafeIdle   StatusBucket = "safe_idle"
	BucketIdle       StatusBucket = "idle"
	BucketNeedsInput StatusBucket = "needs_input"
	BucketError      StatusBucket = "error"
	BucketActive     StatusBucket = "active"
	BucketUnknown    StatusBucket = "unknown"
)

// AllStatusBuckets lists every StatusBucket.
var AllStatusBuckets = []StatusBucket{
	BucketSafeIdle,
	BucketIdle,
	BucketNeedsInput,
	BucketError,
	BucketActive,
	BucketUnknown,
}

// StopSnapshot is captured once, in the driver, before anything is ended or
// killed. PaneTail is bounded and ANSI-stripped but NOT redacted: it must
// reach a durable sink only through the service-side sanitizer.
type StopSnapshot struct {
	CapturedAt       time.Time
	LastActivityAt   time.Time
	PaneTail         string
	Bucket           StatusBucket
	DetectedStatus   string
	DetectedContext  string
	ControllerActive bool
	TmuxAlive        bool
	PaneDead         bool
	ExitCode         int
	CaptureErr       string
}

// ClassifyObserved maps a snapshot to what the pane was doing: pane death, then an
// unproven-liveness capture failure, then tmux liveness, controller, and bucket.
// A failed capture reads as unknown, not exited.
func ClassifyObserved(s StopSnapshot) ObservedState {
	switch {
	case s.PaneDead:
		if s.ExitCode == 0 {
			return ObservedExited
		}
		return ObservedCrashed
	case s.CaptureErr != "" && (!s.TmuxAlive || !s.ControllerActive):
		return ObservedUnknown
	case !s.TmuxAlive, !s.ControllerActive:
		return ObservedExited
	}
	switch s.Bucket {
	case BucketSafeIdle:
		return ObservedIdlePrompt
	case BucketNeedsInput:
		return ObservedNeedsApproval
	case BucketError:
		return ObservedError
	case BucketActive:
		return ObservedWorking
	case BucketIdle:
		return ObservedIdle
	default:
		return ObservedUnknown
	}
}

// StallAction is the recovery chosen for a stalled work session.
type StallAction string

const (
	StallActionNone        StallAction = "none"
	StallActionNudge       StallAction = "nudge"
	StallActionResupervise StallAction = "resupervise"
	StallActionRespawn     StallAction = "respawn"
)

// AllStallActions lists every StallAction.
var AllStallActions = []StallAction{
	StallActionNone,
	StallActionNudge,
	StallActionResupervise,
	StallActionRespawn,
}

func (a StallAction) String() string { return string(a) }

// StallDecisionInput bundles the inputs of DecideStallAction.
// NudgeEnabled is already the AND of both nudge flags (computed by the caller).
// ItemInWorkStatus is true while the backlog item is still in the status the
// work role works in; after request_review the pane idles by design.
// NudgeAllowed and ResuperviseAllowed are the caller's budget/headroom checks
// (attempt caps, cooldowns) for the nudge and resupervise rows respectively.
type StallDecisionInput struct {
	Reason             StopReason
	State              ObservedState
	NudgeEnabled       bool
	NudgeAllowed       bool
	ResuperviseAllowed bool
	ItemInWorkStatus   bool
	Archived           bool
	RetryPending       bool
}

// DecideStallAction implements the ADR-001 decision table; the first matching
// row wins. It is consulted only for a backlog-linked work-role stall.
func DecideStallAction(in StallDecisionInput) StallAction {
	switch {
	case in.Reason.Class() != OutcomeClassStall: // row 1
		return StallActionNone
	case in.Archived: // row 2
		return StallActionNone
	case in.RetryPending: // row 3
		return StallActionNone
	case in.Reason == StopReasonMaxTurns || in.Reason == StopReasonPanic: // row 4
		return StallActionRespawn
	case in.State == ObservedWorking && in.ItemInWorkStatus && in.ResuperviseAllowed: // row 5
		return StallActionResupervise
	case in.State == ObservedWorking: // row 6
		return StallActionRespawn
	case in.State == ObservedIdlePrompt && in.ItemInWorkStatus && in.NudgeEnabled && in.NudgeAllowed: // row 7
		return StallActionNudge
	default: // row 8
		return StallActionRespawn
	}
}
