package session

import (
	"context"
	"encoding/json"
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

	// pipelineModeRepo backs the automated_review config-error check
	// (ADR-006, Part B): resolves a gate's configured pipeline_mode slug to
	// confirm it still exists. May be nil (e.g. in tests) — the check
	// degrades to treating any non-empty pipeline_mode as unresolvable rather
	// than panicking.
	pipelineModeRepo PipelineModeRepository
}

var _ WorkflowEngine = (*ConfiguredWorkflowEngine)(nil)

// StageConfigSnapshot is a frozen, per-item snapshot of the stage config a
// BacklogItem's current stage carried when captured, used as a fallback
// answer for CanTransition/AllowedTransitions when that stage has since been
// deleted from the live stageConfigCache (Epic 2.5, Story 2.5.2). Mirrors
// AcSnapshot's "history/behavior survives later config edits" discipline —
// see plan.md's Domain Glossary entry for StageConfigSnapshot. Building and
// persisting this snapshot per item is a follow-up concern; this type only
// defines the shape CanTransition/AllowedTransitions consume when a caller
// already has one in hand.
type StageConfigSnapshot struct {
	// StageName is the stage's human-readable name at snapshot time — the
	// same value Story 2.5.1 freezes onto BacklogStatusEvent.StageNameSnapshot.
	StageName string
	// AllowedTransitions is the set of destination stages that were legal
	// from this stage at snapshot time.
	AllowedTransitions []BacklogStatus
}

// NewConfiguredWorkflowEngine constructs a ConfiguredWorkflowEngine backed by
// repo, doing one synchronous cache.Load at construction time. Mirrors
// NewPipelineEngine's non-fatal-boot-failure posture (session/pipeline_engine.go):
// a cache.Load failure here never aborts construction, only logs at Warn and
// leaves the engine backed by an empty cache — CanTransition/
// AllowedTransitions/PendingGates all degrade to "no configured graph"
// rather than panicking. gateSatisfactionRepo and pipelineModeRepo may both
// be nil (e.g. in tests that don't exercise those paths).
func NewConfiguredWorkflowEngine(repo StageConfigRepository, gateSatisfactionRepo GateSatisfactionRepository, pipelineModeRepo PipelineModeRepository) (*ConfiguredWorkflowEngine, error) {
	e := &ConfiguredWorkflowEngine{repo: repo, cache: &stageConfigCache{}, gateSatisfactionRepo: gateSatisfactionRepo, pipelineModeRepo: pipelineModeRepo}
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

// CanTransition implements WorkflowEngine. When from is not present at all in
// the live stageConfigCache — the from-stage was since deleted — and a
// non-nil fallback StageConfigSnapshot is supplied, it falls back to
// checking to against the snapshot's own frozen AllowedTransitions (Story
// 2.5.2) rather than unconditionally reporting false, logging a Warn that
// the live config is stale. A from-stage still present in the live cache
// (even with a different edge set than the snapshot) never consults the
// fallback — the live graph is always authoritative when it has an opinion.
func (e *ConfiguredWorkflowEngine) CanTransition(from, to BacklogStatus, fallback *StageConfigSnapshot) bool {
	if _, ok := e.cache.Get(from, to); ok {
		return true
	}
	if e.cache.HasStage(from) {
		return false
	}
	if fallback == nil {
		return false
	}
	for _, allowed := range fallback.AllowedTransitions {
		if allowed == to {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] stage %q not found in live cache (likely deleted); allowing %s->%s from item's captured StageConfigSnapshot", from, from, to)
			return true
		}
	}
	return false
}

// AllowedTransitions implements WorkflowEngine. When from is not present at
// all in the live stageConfigCache — the from-stage was since deleted — and a
// non-nil fallback StageConfigSnapshot is supplied, it returns the
// snapshot's own frozen AllowedTransitions (Story 2.5.2) rather than an empty
// slice, logging a Warn that the live config is stale. A from-stage still
// present in the live cache (even with zero live edges, e.g. a legitimate
// dead-end) never consults the fallback.
func (e *ConfiguredWorkflowEngine) AllowedTransitions(from BacklogStatus, fallback *StageConfigSnapshot) []BacklogStatus {
	result := e.cache.AllowedTransitions(from)
	if !e.cache.HasStage(from) {
		if fallback != nil {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] stage %q not found in live cache (likely deleted); falling back to item's captured StageConfigSnapshot with %d transition(s)", from, len(fallback.AllowedTransitions))
			result = append([]BacklogStatus(nil), fallback.AllowedTransitions...)
		}
	}
	if result == nil {
		result = []BacklogStatus{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// stageConfigUnresolvableGateID identifies the single synthetic blocking gate
// PendingGates returns when the live stageConfigCache has no edge for
// (item.Status, to) but a supplied fallback confirms the edge legally existed
// when the item entered its current stage (ADR-004, Decision 4 — the
// adversarial review's fail-closed-for-gates fix).
const stageConfigUnresolvableGateID = "stage-config-unresolvable"

// PendingGates implements WorkflowEngine. It looks up the configured edge for
// (item.Status, to) and evaluates each of its gates in order_index order. An
// edge with no configured gates (including an edge that doesn't exist in the
// loaded graph at all — CanTransition governs legality separately) returns a
// nil slice, never an error.
//
// fallback is ADR-004's cache-miss fail-closed fix: when the live cache has
// no edge for (item.Status, to) at all — the from-stage was deleted or
// disabled — silently returning nil, nil would let ValidateGates (a thin
// wrapper over this) report zero pending gates, inverting this project's
// fail-closed-for-gates rule. If a supplied fallback's AllowedTransitions
// confirms to was once a legal destination from this stage, this returns a
// single synthetic blocking GateStatus instead. When the fallback doesn't
// contain to either (never a legal edge), today's nil, nil is preserved —
// CanTransition already governs legality separately.
func (e *ConfiguredWorkflowEngine) PendingGates(item BacklogItemTransitionInput, to BacklogStatus, fallback *StageConfigSnapshot) ([]GateStatus, error) {
	edge, ok := e.cache.Get(item.Status, to)
	if !ok {
		if status, blocked := stageConfigUnresolvableGateStatus(to, fallback); blocked {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] stage %q not found in live cache (likely deleted); blocking %s->%s pending manual override (edge once existed per item's captured StageConfigSnapshot)", item.Status, item.Status, to)
			return []GateStatus{status}, nil
		}
		return nil, nil
	}
	if len(edge.Gates) == 0 {
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

// stageConfigUnresolvableGateStatus returns ADR-004's synthetic blocking
// GateStatus (ok=true) when snap confirms to was once a legal destination
// from the item's current stage, or ok=false when snap is nil or doesn't
// contain to — preserving PendingGates' "not a legal edge" nil, nil case for
// a transition that never existed even in the frozen snapshot.
func stageConfigUnresolvableGateStatus(to BacklogStatus, snap *StageConfigSnapshot) (status GateStatus, ok bool) {
	if snap == nil {
		return GateStatus{}, false
	}
	for _, allowed := range snap.AllowedTransitions {
		if allowed != to {
			continue
		}
		return GateStatus{
			GateID:      stageConfigUnresolvableGateID,
			Kind:        GateKindStructural,
			Satisfied:   false,
			Description: fmt.Sprintf("This item's current stage configuration is no longer available (the stage may have been deleted or disabled) — the transition to %s cannot be automatically evaluated.", to),
			ActionHint:  "An operator must restore or reconfigure the stage in Stages settings, or force this transition via Manual Override.",
		}, true
	}
	return GateStatus{}, false
}

// ValidateGates implements WorkflowEngine as a thin wrapper over PendingGates
// (ADR-002's Decision 2): nil exactly when every gate PendingGates reports
// for this transition is satisfied.
func (e *ConfiguredWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus, fallback *StageConfigSnapshot) error {
	statuses, err := e.PendingGates(item, to, fallback)
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
// GateKindHumanApproval, GateKindAutomatedReview, and GateKindCustom are all
// stateful, one-shot gates resolved against a persisted
// GateSatisfactionRecord (Epic 2.4, Stories 2.4.1/2.4.3/2.4.4) — the latter
// two written by ReviewGateRunner.Run and InvokeCustomGateCheck respectively
// once this Epic's follow-up wiring (backlog_lifecycle.go's
// resolveReviewGateContext/resolveCustomCheckGateContext call sites) actually
// invokes them.
func (e *ConfiguredWorkflowEngine) evaluateGate(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	switch g.Kind {
	case GateKindStructural:
		return evaluateStructuralGate(g, item)
	case GateKindHumanApproval:
		return e.evaluateHumanApprovalGate(g, item)
	case GateKindAutomatedReview:
		if status, hasError := e.checkAutomatedReviewGateConfig(g); hasError {
			return status
		}
		return e.evaluateRecordedGate(g, item, "automated review")
	case GateKindCustom:
		if status, hasError := e.checkCustomCheckGateConfig(g); hasError {
			return status
		}
		return e.evaluateRecordedGate(g, item, "custom check")
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

// gateConfigErrorStatus builds ADR-006 Part B's blocking GateStatus for a
// gate whose configuration no longer resolves — ConfigError and Description
// both carry reason so the frontend's plain-satisfaction rendering path (which
// only reads Description) still shows something sensible even before it's
// updated to specifically branch on ConfigError.
func gateConfigErrorStatus(g resolvedGate, reason string) GateStatus {
	return GateStatus{
		GateID:      g.ID.String(),
		Kind:        g.Kind,
		Satisfied:   false,
		ConfigError: reason,
		Description: reason,
	}
}

// checkCustomCheckGateConfig runs ADR-006 Part B's config-error resolution
// check for a GateKindCustom gate, ahead of any GateSatisfactionRepository
// lookup: g.Config must still parse and its skill must still be present in
// registeredCustomCheckSkills (session/gate_config.go) — ParseGateConfig
// already enforces exactly that allowlist membership, so any parse failure
// here means the skill that was valid at save time is no longer registered.
// hasError=false means evaluateGate should fall through to
// evaluateRecordedGate as usual.
func (e *ConfiguredWorkflowEngine) checkCustomCheckGateConfig(g resolvedGate) (status GateStatus, hasError bool) {
	if _, err := parseResolvedGateConfig(g); err != nil {
		skillID, _ := g.Config["skill"].(string)
		return gateConfigErrorStatus(g, fmt.Sprintf("the custom check skill %q is no longer registered", skillID)), true
	}
	return GateStatus{}, false
}

// checkAutomatedReviewGateConfig runs ADR-006 Part B's config-error
// resolution check for a GateKindAutomatedReview gate, ahead of any
// GateSatisfactionRepository lookup: an empty pipeline_mode ("use the item's
// own PipelineMode") never errors here. A non-empty pipeline_mode must
// resolve via pipelineModeRepo.GetBySlug — a nil pipelineModeRepo (not
// wired) or any lookup error (not found) both surface as the same blocking
// config error. hasError=false means evaluateGate should fall through to
// evaluateRecordedGate as usual.
func (e *ConfiguredWorkflowEngine) checkAutomatedReviewGateConfig(g resolvedGate) (status GateStatus, hasError bool) {
	parsed, err := parseResolvedGateConfig(g)
	if err != nil {
		return gateConfigErrorStatus(g, fmt.Sprintf("this gate's automated-review configuration is invalid: %v", err)), true
	}
	cfg, _ := parsed.(AutomatedReviewConfig)
	if cfg.PipelineMode == "" {
		return GateStatus{}, false
	}
	if e.pipelineModeRepo == nil {
		return gateConfigErrorStatus(g, fmt.Sprintf("the pipeline mode %q cannot be resolved (pipeline mode storage not available)", cfg.PipelineMode)), true
	}
	if _, err := e.pipelineModeRepo.GetBySlug(context.Background(), cfg.PipelineMode); err != nil {
		return gateConfigErrorStatus(g, fmt.Sprintf("the pipeline mode %q no longer exists", cfg.PipelineMode)), true
	}
	return GateStatus{}, false
}

// evaluateRecordedGate resolves a GateKindAutomatedReview or GateKindCustom
// gate against its persisted GateSatisfactionRecord (Epic 2.4 follow-up —
// evaluateGate's sibling case for GateKindHumanApproval above already did
// this; this generalizes the same lookup to the two gate kinds that were
// previously hardcoded to Satisfied: false regardless of what actually
// happened). kindLabel is used only for the human-readable description/hint
// text. Falls back to the "not yet actionable" placeholder — unchanged from
// before this follow-up — only when gateSatisfactionRepo is nil, item.ItemID
// doesn't parse, or no record exists yet (the gate hasn't fired for this item
// at all).
func (e *ConfiguredWorkflowEngine) evaluateRecordedGate(g resolvedGate, item BacklogItemTransitionInput, kindLabel string) GateStatus {
	if e.gateSatisfactionRepo != nil && item.ItemID != "" {
		if itemUUID, parseErr := uuid.Parse(item.ItemID); parseErr == nil {
			if record, lookupErr := e.gateSatisfactionRepo.GetByItemAndGate(context.Background(), itemUUID, g.ID); lookupErr == nil {
				actionHint := ""
				if !record.Satisfied {
					actionHint = fmt.Sprintf("re-run the %s check for this item", kindLabel)
				}
				return GateStatus{
					GateID:      g.ID.String(),
					Kind:        g.Kind,
					Satisfied:   record.Satisfied,
					Description: describeGateSatisfactionRecord(record, kindLabel),
					ActionHint:  actionHint,
				}
			}
		}
	}
	return GateStatus{
		GateID:      g.ID.String(),
		Kind:        g.Kind,
		Satisfied:   false,
		Description: fmt.Sprintf("%s gate requires a recorded action; nothing has run yet", kindLabel),
		ActionHint:  "not yet actionable — this transition has not been attempted",
	}
}

// describeGateSatisfactionRecord renders a human-readable description for an
// already-recorded GateSatisfactionRecord, preferring the "detail" key both
// ReviewGateRunner.recordGateSatisfaction and recordCustomCheckTerminalOutcome
// write into OutcomeDetail (session/review_gate.go, session/gate_custom_check.go)
// and falling back to a generic satisfied/not-satisfied phrase for a record
// shape that carries none (e.g. a legacy or hand-written row).
func describeGateSatisfactionRecord(record *GateSatisfactionData, kindLabel string) string {
	if detail, ok := record.OutcomeDetail["detail"].(string); ok && detail != "" {
		return detail
	}
	if record.Satisfied {
		return fmt.Sprintf("%s: satisfied", kindLabel)
	}
	return fmt.Sprintf("%s: not satisfied", kindLabel)
}

// ResolveAutomatedReviewGateContext returns the GateID and parsed
// AutomatedReviewConfig for the first GateKindAutomatedReview gate configured
// on the (from,to) edge, or ok=false when the edge doesn't exist in the
// loaded graph or carries no such gate (Epic 2.4, Story 2.4.3's follow-up).
func (e *ConfiguredWorkflowEngine) ResolveAutomatedReviewGateContext(from, to BacklogStatus) (gateID string, cfg AutomatedReviewConfig, ok bool) {
	edge, found := e.cache.Get(from, to)
	if !found {
		return "", AutomatedReviewConfig{}, false
	}
	for _, g := range edge.Gates {
		if g.Kind != GateKindAutomatedReview {
			continue
		}
		parsed, err := parseResolvedGateConfig(g)
		if err != nil {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] gate %s: %v", g.ID, err)
			continue
		}
		arCfg, _ := parsed.(AutomatedReviewConfig)
		return g.ID.String(), arCfg, true
	}
	return "", AutomatedReviewConfig{}, false
}

// ResolveCustomCheckGateContext returns the GateID and parsed
// CustomCheckConfig for the first GateKindCustom gate configured on the
// (from,to) edge, or ok=false when the edge doesn't exist in the loaded graph
// or carries no such gate. Sibling to ResolveAutomatedReviewGateContext above
// (Epic 2.4, Story 2.4.4's follow-up).
func (e *ConfiguredWorkflowEngine) ResolveCustomCheckGateContext(from, to BacklogStatus) (gateID uuid.UUID, cfg CustomCheckConfig, ok bool) {
	edge, found := e.cache.Get(from, to)
	if !found {
		return uuid.Nil, CustomCheckConfig{}, false
	}
	for _, g := range edge.Gates {
		if g.Kind != GateKindCustom {
			continue
		}
		parsed, err := parseResolvedGateConfig(g)
		if err != nil {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] gate %s: %v", g.ID, err)
			continue
		}
		ccCfg, _ := parsed.(CustomCheckConfig)
		return g.ID, ccCfg, true
	}
	return uuid.Nil, CustomCheckConfig{}, false
}

// parseResolvedGateConfig re-marshals g.Config (the deep-copied
// map[string]interface{} snapshot held in the cache, session/stage_config_cache.go)
// back to JSON and decodes it via ParseGateConfig (session/gate_config.go) —
// reusing the same save-time validator rather than a second ad hoc decode
// path for the read side.
func parseResolvedGateConfig(g resolvedGate) (GateConfig, error) {
	raw, err := json.Marshal(g.Config)
	if err != nil {
		return nil, fmt.Errorf("marshal config for gate %s: %w", g.ID, err)
	}
	return ParseGateConfig(g.Kind, raw)
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
