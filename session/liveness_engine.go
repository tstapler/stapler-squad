package session

import (
	"fmt"
	"time"
)

// LivenessEngine resolves the LivenessDefinition (session/liveness_definition.go,
// Epic 1.1) that governs how long a given stage may go without progress before
// a reconcile* sweep treats an item as stuck. Deliberately a sibling interface
// to WorkflowEngine/PipelineEngine, not an extension of either — its consumers
// (the periodic reconcile* sweeps in session/backlog_lifecycle*.go) are
// disjoint from WorkflowEngine's (the synchronous transition path) and
// PipelineEngine's (prompt-content resolution), per
// project_plans/backlog-custom-workflow-stages/research/architecture.md §3a.
type LivenessEngine interface {
	// LivenessFor resolves the liveness definition for stage, given the item's
	// current pipeline mode (PipelineModeDefault = no override). Implementations
	// must never return an error for a resolvable-to-a-safe-default case (an
	// unconfigured stage/mode resolves to NoTimeoutLiveness or a built-in
	// default, never a zero/infinite threshold) — error is reserved for
	// implementations backed by a real repository (Epic 1.3) whose lookup
	// itself can fail (e.g. a DB error), not for "no override configured."
	LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error)
}

// NoTimeoutLiveness is the sentinel LivenessDefinition value for a stage with
// no timeout concept at all — e.g. plan_not_approved and blocked_by_dependency
// (DequeueNextQueuedItems' planning/dependency gates, both keyed to
// BacklogStatusQueued below), which are gated on human/external action, not
// elapsed time. It is deliberately the zero value of LivenessDefinition
// (Kind == ""), never one of the three real LivenessKind values from
// session/liveness_definition.go, so it can never be mistaken for a real
// Shape A/B/C definition by a switch that forgets a default case. It is
// constructed as a raw struct literal (bypassing NewLivenessDefinition)
// because LivenessDefinition.validate() rejects Kind == "" as an "unknown
// LivenessKind" — that rejection is correct for anything reaching
// NewLivenessDefinition, since NoTimeoutLiveness is a resolution-time
// sentinel, not a constructible, storable definition. Callers must check
// IsNoTimeout() rather than calling StalenessThreshold()/
// MaxNoProgressDuration/CycleThreshold on this value.
var NoTimeoutLiveness = LivenessDefinition{}

// IsNoTimeout reports whether d is the NoTimeoutLiveness sentinel rather than
// a real Shape A/B/C definition.
func (d LivenessDefinition) IsNoTimeout() bool {
	return d.Kind == ""
}

// defaultTriageExpectedDuration mirrors server/services.triageCallBudget
// (30m) — the headless triage call's own timeout. That constant is
// unexported in a different package (server/services), so its value is
// duplicated here as a literal rather than imported; StalenessMargin below is
// derived by subtraction from maxHeadlessTriageSessionStaleness (this
// package's own constant) rather than a second hardcoded literal, so the two
// numbers can never drift apart from the 35m total BUG-055 requires.
const defaultTriageExpectedDuration = 30 * time.Minute

// DefaultLivenessEngine is the in-memory LivenessEngine implementation that
// reproduces every hardcoded StuckReason threshold surveyed in
// project_plans/backlog-custom-workflow-stages/research/architecture.md §1,
// verbatim, with no configuration source of its own. It is the zero-migration
// baseline every other LivenessEngine implementation (Epic 1.3's
// CachingLivenessEngine) falls back to when no override is configured or
// resolvable — see that same doc's §6 "Migration safety" fail-closed
// requirement.
type DefaultLivenessEngine struct {
	table map[BacklogStatus]LivenessDefinition
}

// NewDefaultLivenessEngine builds the built-in stage table. Each entry is
// constructed via NewLivenessDefinition (rather than a raw struct literal) so
// a typo mismatching a Kind with its fields is caught here, at construction,
// instead of silently producing a malformed definition — panics are
// appropriate because a failure here is exclusively a programmer error in
// this file's own table, never a runtime/configuration condition.
func NewDefaultLivenessEngine() *DefaultLivenessEngine {
	// Shape A: idea stage, headless triage call. ExpectedDuration (30m) +
	// StalenessMargin (35m - 30m = 5m) reproduces
	// maxHeadlessTriageSessionStaleness (35m) exactly — see that constant's
	// doc comment (session/backlog_lifecycle_triage.go) for why it must stay
	// strictly greater than the call budget, with real margin.
	orphanedTriage, err := NewLivenessDefinition(LivenessKindDurationBudget,
		WithExpectedDuration(defaultTriageExpectedDuration),
		WithStalenessMargin(maxHeadlessTriageSessionStaleness-defaultTriageExpectedDuration),
	)
	if err != nil {
		panic(fmt.Sprintf("DefaultLivenessEngine: invalid built-in idea-stage definition: %v", err))
	}

	// Shape B: in_progress stage, live tmux-backed work session. Reproduces
	// maxWorkSessionStaleness (2h, session/backlog_lifecycle_stale.go) exactly.
	staleWorkDef, err := NewLivenessDefinition(LivenessKindHeartbeat,
		WithMaxNoProgressDuration(maxWorkSessionStaleness),
	)
	if err != nil {
		panic(fmt.Sprintf("DefaultLivenessEngine: invalid built-in in_progress-stage definition: %v", err))
	}

	// Shape C: bouncing (in_progress<->review cycle count). Reproduces
	// bounceThreshold/bounceLookback (3 cycles / 24h, session/stuck_decisions.go)
	// exactly.
	//
	// Judgment call: reconcileBouncingItems (session/backlog_lifecycle.go)
	// scans both BacklogStatusInProgress and BacklogStatusReview, so bouncing
	// has no single natural "home" stage the way Shape A/B reasons do.
	// architecture.md §1/§7 never resolves this to one literal BacklogStatus.
	// This table keys it to BacklogStatusReview rather than
	// BacklogStatusInProgress (already occupied by stale_work above) because
	// the "is this converging" question bouncing answers is fundamentally
	// about review verdicts (isBouncing's own signature takes hasPass, derived
	// from the item's most recent *review* verdict) — not because any survey
	// document states this explicitly. A future epic reconciling the full
	// stage/reason key space (including rework_blocked_stale, also
	// review-anchored) may need to revisit this.
	bouncingDef, err := NewLivenessDefinition(LivenessKindCycleFrequency,
		WithCycleThreshold(bounceThreshold),
		WithCycleLookback(bounceLookback),
	)
	if err != nil {
		panic(fmt.Sprintf("DefaultLivenessEngine: invalid built-in review-stage definition: %v", err))
	}

	return &DefaultLivenessEngine{
		table: map[BacklogStatus]LivenessDefinition{
			BacklogStatusIdea:       *orphanedTriage,
			BacklogStatusInProgress: *staleWorkDef,
			BacklogStatusReview:     *bouncingDef,
		},
	}
}

// LivenessFor resolves stage's built-in definition. mode is accepted (per the
// LivenessEngine interface) but ignored: DefaultLivenessEngine has no
// per-pipeline-mode overrides of its own (that is Epic 1.3's
// CachingLivenessEngine, backed by stage_liveness_definitions rows). A stage
// with no table entry — including plan_not_approved/blocked_by_dependency's
// BacklogStatusQueued and any other unmapped stage — resolves to
// NoTimeoutLiveness, never an error: an unresolvable stage must fail closed to
// "no timeout enforced," not to a zero/infinite threshold or a hard error that
// would abort a reconcile* sweep tick.
func (e *DefaultLivenessEngine) LivenessFor(stage BacklogStatus, _ PipelineMode) (LivenessDefinition, error) {
	if def, ok := e.table[stage]; ok {
		return def, nil
	}
	return NoTimeoutLiveness, nil
}
