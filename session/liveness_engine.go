package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
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
func (e *DefaultLivenessEngine) LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error) {
	def := e.resolve(stage)
	log.InfoLog().Printf("[LivenessEngine] resolved liveness for stage=%s mode=%s kind=%s", stage, ResolvedModeLabel(string(mode)), def.Kind)
	return def, nil
}

// resolve is LivenessFor's unexported, non-logging core: the built-in-table
// lookup itself. Split out so CachingLivenessEngine's fallback path (below)
// can reuse it without triggering this type's own Info log line a second
// time — CachingLivenessEngine.LivenessFor logs its own single Info line for
// the resolution it ultimately returns, whichever engine supplied it.
func (e *DefaultLivenessEngine) resolve(stage BacklogStatus) LivenessDefinition {
	if def, ok := e.table[stage]; ok {
		return def
	}
	return NoTimeoutLiveness
}

// CachingLivenessEngine is the DB-backed LivenessEngine implementation
// (Epic 1.3) backed by a LivenessRepository + livenessCache. It resolves a
// (stage, mode) pair through a two-level fallback:
//
//  1. An exact (stage, mode) row, if configured.
//  2. A mode-less (stage, nil) override row, if configured — resolved by
//     livenessCache.Get, the single owner of this fallback (see that type's
//     doc comment). Not a failure: no Warn is logged.
//  3. embeddedDefault (a DefaultLivenessEngine), when neither row exists —
//     this IS the failure case this project's fail-closed posture requires
//     logging: exactly one Warn line is emitted here, naming the
//     unresolved stage and mode, before delegating.
type CachingLivenessEngine struct {
	repo            LivenessRepository
	cache           *livenessCache
	embeddedDefault *DefaultLivenessEngine
}

var _ LivenessEngine = (*CachingLivenessEngine)(nil)

// NewCachingLivenessEngine constructs a CachingLivenessEngine backed by repo,
// doing one synchronous cache.Load at construction time.
//
// Mirrors NewPipelineEngine's non-fatal-startup posture (session/pipeline_engine.go):
// a cache.Load failure here never aborts construction — it is logged at Warn
// and NewCachingLivenessEngine returns a valid, usable engine backed by an
// empty cache, which resolves every (stage, mode) pair via
// DefaultLivenessEngine until the next successful Load/Invalidate. The
// signature still returns an error for future-proofing, but this
// implementation never returns a non-nil error for a cache.Load failure
// specifically.
func NewCachingLivenessEngine(repo LivenessRepository) (*CachingLivenessEngine, error) {
	e := &CachingLivenessEngine{
		repo:            repo,
		cache:           &livenessCache{},
		embeddedDefault: NewDefaultLivenessEngine(),
	}
	if err := e.cache.Load(context.Background(), repo); err != nil {
		log.WarningLog().Printf("[LivenessEngine] cache.Load failed at startup, continuing with an empty cache: %v", err)
	}
	return e, nil
}

// InvalidateCache re-fetches liveness definitions from the repository and
// swaps the cache wholesale. Called by the CRUD RPC write handlers (Story
// 1.3.2) after every successful Create/Update/Delete.
func (e *CachingLivenessEngine) InvalidateCache(ctx context.Context) error {
	return e.cache.Invalidate(ctx, e.repo)
}

// LivenessFor implements LivenessEngine, applying the two-level fallback
// described in this type's doc comment.
func (e *CachingLivenessEngine) LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error) {
	modeLabel := ResolvedModeLabel(string(mode))

	if rd, ok := e.cache.Get(string(stage), mode); ok {
		log.InfoLog().Printf("[LivenessEngine] resolved liveness for stage=%s mode=%s kind=%s", stage, modeLabel, rd.Kind)
		return rd.LivenessDefinition, nil
	}

	// e.embeddedDefault.resolve (not .LivenessFor) is called deliberately: it
	// skips DefaultLivenessEngine's own Info log line so exactly one Info line
	// is emitted per LivenessFor call, not two, whichever branch resolves it.
	def := e.embeddedDefault.resolve(stage)
	log.WarningLog().Printf("[LivenessEngine] stage=%s mode=%s liveness config unresolved, falling back to default (%s)", stage, modeLabel, def.Kind)
	log.InfoLog().Printf("[LivenessEngine] resolved liveness for stage=%s mode=%s kind=%s", stage, modeLabel, def.Kind)
	return def, nil
}
