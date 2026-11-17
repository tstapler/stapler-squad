package backlogdto

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
)

// NewBacklogItemTransitionInput builds a BacklogItemTransitionInput from item,
// always setting ItemID alongside the fields every transition-guard call site
// needs. ItemID must be set for ConfiguredWorkflowEngine's evaluateRecordedGate
// to look up a persisted GateSatisfactionRecord for automated_review/custom
// gates (session/configured_workflow_engine.go) — a hand-built literal that
// forgets it silently and permanently blocks those gate kinds. status is the
// item's current ("from") status; callers needing OverallOutcome,
// OverrideReason, HasUnshippedCode, or HasUnresolvedBlockers set those on the
// returned value afterward.
func NewBacklogItemTransitionInput(item *BacklogItemData, status domain.BacklogStatus) domain.BacklogItemTransitionInput {
	return domain.BacklogItemTransitionInput{
		ItemID:            item.ID,
		Status:            status,
		AcCriteria:        item.AcceptanceCriteria,
		PlanApproved:      item.PlanApproved,
		SkipPlanning:      item.SkipPlanning,
		PlanArtifactsPath: item.PlanArtifactsPath,
	}
}

// BuildStageConfigSnapshotFallback reconstructs the StageConfigSnapshot
// (session/configured_workflow_engine.go) an item's current stage carried at
// the moment it entered that stage, from item.StatusEvents — already
// eager-loaded by GetBacklogItem, no new query (ADR-004). It scans for the
// most recent event whose ToStatus matches item.Status (by CreatedAt, not
// slice position — callers order this slice both ascending and descending)
// and builds the snapshot from that event's stageNameSnapshot/
// allowedTransitionsSnapshot pair. Returns nil when no such event exists —
// e.g. an item still sitting in its entry stage, which was never the
// destination of a recorded transition — which is already
// ConfiguredWorkflowEngine's "no fallback" case, so callers degrade to
// today's live-cache-only behavior rather than crashing.
func BuildStageConfigSnapshotFallback(item *BacklogItemData) *domain.StageConfigSnapshot {
	var latest *BacklogStatusEventData
	for i := range item.StatusEvents {
		ev := &item.StatusEvents[i]
		if ev.ToStatus != item.Status {
			continue
		}
		if latest == nil || ev.CreatedAt.After(latest.CreatedAt) {
			latest = ev
		}
	}
	if latest == nil {
		return nil
	}

	stageName := ""
	if latest.StageNameSnapshot != nil {
		stageName = *latest.StageNameSnapshot
	}
	allowed := make([]domain.BacklogStatus, 0, len(latest.AllowedTransitionsSnapshot))
	for _, s := range latest.AllowedTransitionsSnapshot {
		allowed = append(allowed, domain.BacklogStatus(s))
	}
	return &domain.StageConfigSnapshot{
		StageName:          stageName,
		AllowedTransitions: allowed,
	}
}

// Labels of the exempt entry points on workflow_engine_bypass_total{caller}.
// They are not registry entries: a registry entry names a path the engine decides.
const (
	CallerRollback domain.TransitionCaller = "rollback"
	CallerDebug    domain.TransitionCaller = "debug"
	// CallerDebugCreate labels Storage.CreateBacklogItemForDebug: creation has no
	// "from" status, so it is exempt but counted.
	CallerDebugCreate domain.TransitionCaller = "debug_create"
)

// Denial reasons recorded on a transition_denied row (denied_reason).
const (
	DeniedConfigGraph        = "config_graph"
	DeniedDefaultGuard       = "default_guard"
	DeniedOperatorGate       = "operator_gate"
	DeniedTargetStageRemoved = "target_stage_removed"
)

var (
	// ErrUnregisteredTransitionCaller is returned for a Caller absent from the registry.
	ErrUnregisteredTransitionCaller = errors.New("transition caller is not registered")
	// ErrInvalidTransitionRequest is returned for a malformed request (a programmer error).
	ErrInvalidTransitionRequest = errors.New("invalid transition request")
)

// TransitionDeniedReporter makes an agent or system denial visible (stuck row,
// comment, notification). Implementations must be idempotent per
// (item, caller, from, to, reason): the gate calls it once per denial and the
// implementation, not the gate, suppresses repeats while a row stays open.
type TransitionDeniedReporter interface {
	Report(ctx context.Context, item *BacklogItemData, caller domain.TransitionCaller, from, to domain.BacklogStatus, reason string, denyErr error)
}

// TransitionRequest is one status change asked of GatedTransition.
type TransitionRequest struct {
	// Caller must be a registry entry.
	Caller domain.TransitionCaller
	// ItemID names the item; Observed may replace it.
	ItemID string
	// Observed is the caller's own read of the item, which saves the gate a
	// re-read. Its Status is the decided "from" and must carry StatusEvents when
	// stage-snapshot fallback matters (GetBacklogItem loads them).
	Observed *BacklogItemData
	To       domain.BacklogStatus
	// Precondition's three fields reach the raw writer unchanged; ExpectedStatus
	// defaults to the observed status.
	Precondition *BacklogItemPrecondition
	TriggeredBy  string
	// OverrideReason is forwarded in the guard input; the class policy decides
	// what it frees (enforceAnswer).
	OverrideReason string
	// Guard is guard input the caller already assembled (verdict, unshipped
	// code, blockers); the gate fills ItemID and Status when empty.
	Guard *domain.BacklogItemTransitionInput
	// Unconditional re-reads and re-decides once if the write loses its CAS
	// (at most two attempts). It cannot carry ExpectedUpdatedAt.
	Unconditional bool
	// BeforeCommit runs after the engine allows and before the CAS; its error
	// aborts the write.
	BeforeCommit func(ctx context.Context) error
}

// gateVerdict is the pure outcome of the engine decision.
type GateVerdict struct {
	Err    error
	Reason string
}

// decideTransition asks the engine whether spec may move item to req.To. It does
// no I/O beyond what a Configured operator gate itself reads.
func DecideTransition(view domain.WorkflowEngine, item *BacklogItemData, req TransitionRequest) GateVerdict {
	from := domain.BacklogStatus(item.Status)
	snap := BuildStageConfigSnapshotFallback(item)
	in := GuardInputFor(req, item, from)
	if !graphAllows(view, from, req.To, snap, in.OverrideReason) {
		return GateVerdict{
			Err:    fmt.Errorf("invalid transition from %q to %q: %w", from, req.To, domain.ErrTransitionNotAllowed),
			Reason: DeniedConfigGraph,
		}
	}
	if err := view.ValidateGates(in, req.To, snap); err != nil {
		return GateVerdict{Err: err, Reason: classifyGateDenial(err)}
	}
	return GateVerdict{}
}

// graphAllows is the graph check. It evaluates quietly and counts a denial only
// when no operator override frees it, so an overridden transition is not
// recorded as a denied one.
func graphAllows(view domain.WorkflowEngine, from, to domain.BacklogStatus, snap *domain.StageConfigSnapshot, overrideReason string) bool {
	q, quietable := view.(interface{ Quiet() domain.WorkflowEngine })
	if !quietable {
		return view.CanTransition(from, to, snap) || operatorGraphOverride(view, overrideReason, to)
	}
	if q.Quiet().CanTransition(from, to, snap) || operatorGraphOverride(view, overrideReason, to) {
		return true
	}
	if n, ok := view.(interface {
		NoteGraphDenied(from, to domain.BacklogStatus)
	}); ok {
		n.NoteGraphDenied(from, to)
	}
	return false
}

// graphOverrider is implemented by a class-aware view that lets an operator's
// override_reason bypass the graph (ADR-001, ADR-007 D3).
type graphOverrider interface {
	OverridesGraph(overrideReason string, to domain.BacklogStatus) bool
}

func operatorGraphOverride(view domain.WorkflowEngine, overrideReason string, to domain.BacklogStatus) bool {
	o, ok := view.(graphOverrider)
	return ok && o.OverridesGraph(overrideReason, to)
}

func GuardInputFor(req TransitionRequest, item *BacklogItemData, from domain.BacklogStatus) domain.BacklogItemTransitionInput {
	var in domain.BacklogItemTransitionInput
	if req.Guard != nil {
		in = *req.Guard
		if in.ItemID == "" {
			in.ItemID = item.ID
		}
		if in.Status == "" {
			in.Status = from
		}
	} else {
		in = NewBacklogItemTransitionInput(item, from)
	}
	if req.OverrideReason != "" {
		in.OverrideReason = req.OverrideReason
	}
	return in
}

// classifyGateDenial names the layer behind a ValidateGates error. The
// stage-removed gate is matched by its id because it is a synthetic
// operator-gate status, not a distinct error type.
func classifyGateDenial(err error) string {
	switch {
	case errors.Is(err, domain.ErrGateNotSatisfied):
		if strings.Contains(err.Error(), `"`+domain.StageConfigUnresolvableGateID+`"`) {
			return DeniedTargetStageRemoved
		}
		return DeniedOperatorGate
	default:
		return DeniedDefaultGuard
	}
}

// PRAssociation is the PR a GatedSetPRAndTransition call records on the item.
type PRAssociation struct {
	URL     string
	Number  int
	Summary string
	// Guard is required to reassign an item already in pr_pending; see
	// RawSetBacklogItemPRAndTransition.
	Guard *PRReassignmentGuard
	// Source says which writer recorded the PR (the item-direct link's attach source);
	// empty means manual.
	Source domain.AttachSource
	// Additive adds URL as a follow-up PR of an item already in pr_pending instead of
	// reassigning its tracked PR: no guard, status unchanged, the item mirror follows the
	// derived active PR. Only the report_pr_created handler sets it, and only with the
	// multi-PR backlog scope on.
	Additive bool
}

// pinTarget sets the target status a Gated* entry point owns, rejecting a
// request that names a different one.
func PinTarget(req *TransitionRequest, to domain.BacklogStatus) error {
	if req.To != "" && req.To != to {
		return fmt.Errorf("%w: this entry point only moves to %q, not %q", ErrInvalidTransitionRequest, to, req.To)
	}
	req.To = to
	return nil
}

func NormalizeRequest(req *TransitionRequest) error {
	if req.ItemID == "" && req.Observed != nil {
		req.ItemID = req.Observed.ID
	}
	switch {
	case req.ItemID == "":
		return fmt.Errorf("%w: no item id", ErrInvalidTransitionRequest)
	case req.Observed != nil && req.Observed.ID != req.ItemID:
		return fmt.Errorf("%w: observed item %s is not %s", ErrInvalidTransitionRequest, req.Observed.ID, req.ItemID)
	case req.To == "":
		return fmt.Errorf("%w: no target status", ErrInvalidTransitionRequest)
	case req.Unconditional && req.Precondition != nil && req.Precondition.ExpectedUpdatedAt != nil:
		return fmt.Errorf("%w: an unconditional request cannot carry ExpectedUpdatedAt", ErrInvalidTransitionRequest)
	}
	return nil
}

// gatePrecondition derives the raw writer's compare-and-swap from the request
// and the decided "from".
func GatePrecondition(req TransitionRequest, spec domain.TransitionCallerSpec, item *BacklogItemData) BacklogItemPrecondition {
	pre := BacklogItemPrecondition{}
	if req.Precondition != nil {
		pre = *req.Precondition
	}
	if pre.ExpectedStatus == "" {
		pre.ExpectedStatus = item.Status
	}
	// Guard input read from item fields goes stale if an unrelated field is
	// edited between read and write; pin updated_at only then, so GuardsNone
	// callers keep a status-only CAS.
	if pre.ExpectedUpdatedAt == nil && req.Guard != nil && req.Observed != nil && spec.Guards == domain.GuardsBuiltIn {
		observedAt := item.UpdatedAt
		pre.ExpectedUpdatedAt = &observedAt
	}
	return pre
}
