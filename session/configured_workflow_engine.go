package session

import (
	"context"
	"fmt"
	"sort"

	"github.com/tstapler/stapler-squad/log"
)

// ConfiguredWorkflowEngine is a DB-backed WorkflowEngine implementation
// loading BacklogStage/StageTransition/TransitionGate rows via
// StageConfigRepository, cached copy-on-write in stageConfigCache. Sibling to
// DefaultWorkflowEngine (session/workflow_engine.go) — see Epic 2.3,
// project_plans/backlog-custom-workflow-stages/implementation/plan.md, and
// ADR-002-configured-workflow-engine-and-gates.md.
//
// NOT wired into server/dependencies.go as the live WorkflowEngine by this
// epic. Per plan.md's Migration Plan ("ConfiguredWorkflowEngine is not wired
// into server/dependencies.go until Epic 2.3 is merged") and Risk Control
// ("Staged rollout: full rollout on merge for both phases... Phase 2 must
// not merge until Phase 1's characterization-test gate and the two
// bypass-call-site fixes are both green"), the cutover itself has no
// dedicated task anywhere in this plan — pre-mortem P2 #5 explicitly defers
// shadow-mode validation before that swap. The tables exist and this engine
// is fully usable starting now; the one-line server/dependencies.go change
// remains a deliberate, separate, later step.
type ConfiguredWorkflowEngine struct {
	repo  StageConfigRepository
	cache *stageConfigCache
}

var _ WorkflowEngine = (*ConfiguredWorkflowEngine)(nil)

// NewConfiguredWorkflowEngine constructs a ConfiguredWorkflowEngine backed by
// repo, doing one synchronous cache.Load at construction time. Mirrors
// NewPipelineEngine's non-fatal-boot-failure posture (session/pipeline_engine.go):
// a cache.Load failure here never aborts construction, only logs at Warn and
// leaves the engine backed by an empty cache — CanTransition/
// AllowedTransitions/PendingGates all degrade to "no configured graph"
// rather than panicking.
func NewConfiguredWorkflowEngine(repo StageConfigRepository) (*ConfiguredWorkflowEngine, error) {
	e := &ConfiguredWorkflowEngine{repo: repo, cache: &stageConfigCache{}}
	if err := e.cache.Load(context.Background(), repo); err != nil {
		log.WarningLog().Printf("[ConfiguredWorkflowEngine] cache.Load failed at startup, continuing with an empty cache: %v", err)
	}
	return e, nil
}

// InvalidateCache re-fetches stages/transitions/gates from the repository and
// swaps the cache wholesale. Exported for the Epic 2.7 RPC write handlers
// that must invalidate the cache after every Create/Update/Delete of a
// stage, transition, or gate.
func (e *ConfiguredWorkflowEngine) InvalidateCache(ctx context.Context) error {
	return e.cache.Invalidate(ctx, e.repo)
}

// CanTransition implements WorkflowEngine.
func (e *ConfiguredWorkflowEngine) CanTransition(from, to BacklogStatus) bool {
	_, ok := e.cache.Get(from, to)
	return ok
}

// AllowedTransitions implements WorkflowEngine.
func (e *ConfiguredWorkflowEngine) AllowedTransitions(from BacklogStatus) []BacklogStatus {
	result := e.cache.AllowedTransitions(from)
	if result == nil {
		result = []BacklogStatus{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// PendingGates implements WorkflowEngine. It looks up the configured edge for
// (item.Status, to) and evaluates each of its gates in order_index order. An
// edge with no configured gates (including an edge that doesn't exist in the
// loaded graph at all — CanTransition governs legality separately) returns a
// nil slice, never an error.
func (e *ConfiguredWorkflowEngine) PendingGates(item BacklogItemTransitionInput, to BacklogStatus) ([]GateStatus, error) {
	edge, ok := e.cache.Get(item.Status, to)
	if !ok || len(edge.Gates) == 0 {
		return nil, nil
	}

	statuses := make([]GateStatus, 0, len(edge.Gates))
	unsatisfied := 0
	for _, g := range edge.Gates {
		status := evaluateGate(g, item)
		if !status.Satisfied {
			unsatisfied++
		}
		statuses = append(statuses, status)
	}

	log.InfoLog().Printf("[ConfiguredWorkflowEngine] resolved transition %s->%s, %d gate(s) pending", item.Status, to, unsatisfied)
	return statuses, nil
}

// ValidateGates implements WorkflowEngine as a thin wrapper over PendingGates
// (ADR-002's Decision 2): nil exactly when every gate PendingGates reports
// for this transition is satisfied.
func (e *ConfiguredWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error {
	statuses, err := e.PendingGates(item, to)
	if err != nil {
		return err
	}
	for _, s := range statuses {
		if !s.Satisfied {
			return fmt.Errorf("%w: %s gate %q: %s", ErrGateNotSatisfied, s.Kind, s.GateID, s.Description)
		}
	}
	return nil
}

// evaluateGate dispatches gate g to its evaluation path, per GateKind:
// GateKindStructural always recomputes fresh from item state — never trusts
// a previously-satisfied result (Story 2.3.2's acceptance criterion). Every
// other kind (human_approval, automated_review, custom) is a stateful,
// one-shot gate whose real satisfaction-recording mechanism is wired in Epic
// 2.4 (GateSatisfactionRepository/RecordGateApproval, the generalized
// ReviewGateRunner, InvokeCustomGateCheck) — until one of those lands, such a
// gate has no way to ever become satisfied, so reporting Satisfied: false
// unconditionally is the structurally correct answer for this epic's scope,
// not a stub shortcut.
func evaluateGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	switch g.Kind {
	case GateKindStructural:
		return evaluateStructuralGate(g, item)
	case GateKindHumanApproval, GateKindAutomatedReview, GateKindCustom:
		return GateStatus{
			GateID:      g.ID.String(),
			Kind:        g.Kind,
			Satisfied:   false,
			Description: fmt.Sprintf("%s gate requires a recorded action not yet implemented (Epic 2.4)", g.Kind),
			ActionHint:  "not yet actionable in this build",
		}
	default:
		log.WarningLog().Printf("[ConfiguredWorkflowEngine] gate %s unresolved, blocking transition: unrecognized kind %q", g.ID, g.Kind)
		return GateStatus{
			GateID:      g.ID.String(),
			Kind:        g.Kind,
			Satisfied:   false,
			Description: fmt.Sprintf("unrecognized gate kind %q", g.Kind),
		}
	}
}

// evaluateStructuralGate recomputes "all acceptance criteria done" fresh on
// every call — the only structural check available before Epic 2.4.2 adds
// the closed set of named structural-check identifiers (ac_complete,
// pr_green, no_open_blockers) read from the gate's config.
func evaluateStructuralGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	satisfied := allAcceptanceCriteriaDone(item.AcCriteria)
	hint := ""
	if !satisfied {
		hint = "mark every acceptance criterion done"
	}
	return GateStatus{
		GateID:      g.ID.String(),
		Kind:        GateKindStructural,
		Satisfied:   satisfied,
		Description: "all acceptance criteria must be marked done",
		ActionHint:  hint,
	}
}

// allAcceptanceCriteriaDone reports whether raw parses to a non-empty
// acceptance-criteria list with every entry AcStatusDone. A parse error or an
// empty list is treated as unsatisfied (fail-closed), matching TransitionGuard's
// ErrACRequired guards elsewhere in this package.
func allAcceptanceCriteriaDone(raw AcCriteriaJSON) bool {
	criteria, err := ParseAcCriteria(raw)
	if err != nil || len(criteria) == 0 {
		return false
	}
	for _, c := range criteria {
		if c.Status != AcStatusDone {
			return false
		}
	}
	return true
}
