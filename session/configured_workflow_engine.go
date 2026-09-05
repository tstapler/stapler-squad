package session

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
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

	// gateSatisfactionRepo backs PendingGates' human_approval branch (Epic
	// 2.4, Story 2.4.1): a stateful gate's satisfaction is a persisted
	// GateSatisfactionRecord, not something the copy-on-write stageConfigCache
	// can hold (it's per-item, not per-graph). May be nil (e.g. in tests that
	// only exercise CanTransition/AllowedTransitions) — every lookup through
	// it is nil-guarded and degrades to "unsatisfied", never panics.
	gateSatisfactionRepo GateSatisfactionRepository
}

var _ WorkflowEngine = (*ConfiguredWorkflowEngine)(nil)

// NewConfiguredWorkflowEngine constructs a ConfiguredWorkflowEngine backed by
// repo, doing one synchronous cache.Load at construction time. Mirrors
// NewPipelineEngine's non-fatal-boot-failure posture (session/pipeline_engine.go):
// a cache.Load failure here never aborts construction, only logs at Warn and
// leaves the engine backed by an empty cache — CanTransition/
// AllowedTransitions/PendingGates all degrade to "no configured graph"
// rather than panicking. gateSatisfactionRepo may be nil.
func NewConfiguredWorkflowEngine(repo StageConfigRepository, gateSatisfactionRepo GateSatisfactionRepository) (*ConfiguredWorkflowEngine, error) {
	e := &ConfiguredWorkflowEngine{repo: repo, cache: &stageConfigCache{}, gateSatisfactionRepo: gateSatisfactionRepo}
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
		status := e.evaluateGate(g, item)
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
// a previously-satisfied result (Story 2.3.2's acceptance criterion).
// GateKindHumanApproval is a stateful, one-shot gate resolved against a
// persisted GateSatisfactionRecord (Epic 2.4, Story 2.4.1). GateKindAutomatedReview
// and GateKindCustom are also stateful/one-shot — their satisfaction-recording
// mechanisms (the generalized ReviewGateRunner, InvokeCustomGateCheck) are
// built by this same Epic (Stories 2.4.3/2.4.4), but wiring their PendingGates
// dispatch to consult GateSatisfactionRepository is out of this epic's task
// scope (plan.md names only Tasks 2.4.1b/2.4.2b for this file) — left for a
// follow-up, so they still report Satisfied: false unconditionally here.
func (e *ConfiguredWorkflowEngine) evaluateGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	switch g.Kind {
	case GateKindStructural:
		return evaluateStructuralGate(g, item)
	case GateKindHumanApproval:
		return e.evaluateHumanApprovalGate(g, item)
	case GateKindAutomatedReview, GateKindCustom:
		return GateStatus{
			GateID:      g.ID.String(),
			Kind:        g.Kind,
			Satisfied:   false,
			Description: fmt.Sprintf("%s gate requires a recorded action; PendingGates dispatch for this kind is not yet wired (Epic 2.4 follow-up)", g.Kind),
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

// evaluateHumanApprovalGate resolves a human_approval gate against a
// persisted GateSatisfactionRecord: satisfied iff RecordGateApproval
// (session/gate_approval.go) has already recorded one for (item.ItemID,
// g.ID). Fails closed (Satisfied: false) when gateSatisfactionRepo is nil,
// item.ItemID doesn't parse as a UUID, or no record exists yet — the last of
// these is the normal "not yet approved" case, not an error.
//
// Uses context.Background() rather than a caller-supplied context:
// WorkflowEngine.PendingGates' signature (shared with DefaultWorkflowEngine,
// session/workflow_engine.go) carries no context parameter, mirroring
// NewConfiguredWorkflowEngine's identical constructor-time cache.Load call
// just above.
func (e *ConfiguredWorkflowEngine) evaluateHumanApprovalGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	if e.gateSatisfactionRepo != nil && item.ItemID != "" {
		if itemUUID, parseErr := uuid.Parse(item.ItemID); parseErr == nil {
			if record, lookupErr := e.gateSatisfactionRepo.GetByItemAndGate(context.Background(), itemUUID, g.ID); lookupErr == nil && record.Satisfied {
				return GateStatus{
					GateID:      g.ID.String(),
					Kind:        GateKindHumanApproval,
					Satisfied:   true,
					Description: "approved",
				}
			}
		}
	}
	return GateStatus{
		GateID:      g.ID.String(),
		Kind:        GateKindHumanApproval,
		Satisfied:   false,
		Description: "requires explicit human approval",
		ActionHint:  "call RecordGateApproval to approve this gate",
	}
}

// evaluateStructuralGate dispatches to Story 2.4.2's closed set of named
// structural-check evaluators (session/gate_structural.go) based on the
// gate's configured check_id, recomputed fresh on every call.
func evaluateStructuralGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	checkID, _ := g.Config["check_id"].(string)
	result := evaluateStructuralCheck(checkID, item)
	return GateStatus{
		GateID:      g.ID.String(),
		Kind:        GateKindStructural,
		Satisfied:   result.Satisfied,
		Description: result.Description,
		ActionHint:  result.ActionHint,
	}
}
