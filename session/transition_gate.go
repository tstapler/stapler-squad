package session

// transition_gate.go is the status-write choke point (ADR-007 D1, D3, D5, D6):
// GatedTransition decides with the class-aware workflow engine and only then
// calls the raw status writer, so no caller reaches the database without the
// engine. Rollback and DebugForce are the counted, never-checked exempt entry
// points. GatedArchive, GatedUnarchive, GatedSetPRAndTransition and
// GatedReconcileStuck share GatedTransition's decision function.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Labels of the exempt entry points on workflow_engine_bypass_total{caller}.
// They are not registry entries: a registry entry names a path the engine decides.
const (
	CallerRollback TransitionCaller = "rollback"
	CallerDebug    TransitionCaller = "debug"
	// CallerDebugCreate labels Storage.CreateBacklogItemForDebug: creation has no
	// "from" status, so it is exempt but counted.
	CallerDebugCreate TransitionCaller = "debug_create"
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
	Report(ctx context.Context, item *BacklogItemData, caller TransitionCaller, from, to BacklogStatus, reason string, denyErr error)
}

// TransitionRequest is one status change asked of GatedTransition.
type TransitionRequest struct {
	// Caller must be a registry entry.
	Caller TransitionCaller
	// ItemID names the item; Observed may replace it.
	ItemID string
	// Observed is the caller's own read of the item, which saves the gate a
	// re-read. Its Status is the decided "from" and must carry StatusEvents when
	// stage-snapshot fallback matters (GetBacklogItem loads them).
	Observed *BacklogItemData
	To       BacklogStatus
	// Precondition's three fields reach the raw writer unchanged; ExpectedStatus
	// defaults to the observed status.
	Precondition *BacklogItemPrecondition
	TriggeredBy  string
	// OverrideReason is forwarded in the guard input; the class policy decides
	// what it frees (enforceAnswer).
	OverrideReason string
	// Guard is guard input the caller already assembled (verdict, unshipped
	// code, blockers); the gate fills ItemID and Status when empty.
	Guard *BacklogItemTransitionInput
	// Unconditional re-reads and re-decides once if the write loses its CAS
	// (at most two attempts). It cannot carry ExpectedUpdatedAt.
	Unconditional bool
	// BeforeCommit runs after the engine allows and before the CAS; its error
	// aborts the write.
	BeforeCommit func(ctx context.Context) error
}

type transitionGate struct {
	engine   *SwitchableWorkflowEngine
	reporter TransitionDeniedReporter
}

// SetTransitionGate injects the selector and the denial reporter together, so
// the gate can never exist without its reporter (ADR-007 D8).
func (s *Storage) SetTransitionGate(engine *SwitchableWorkflowEngine, reporter TransitionDeniedReporter) error {
	if engine == nil || reporter == nil {
		return fmt.Errorf("%w: the transition gate needs an engine and a reporter", ErrInvalidTransitionRequest)
	}
	s.gate.Store(&transitionGate{engine: engine, reporter: reporter})
	return nil
}

// TransitionGateEngine returns the injected selector, or nil while unwired.
func (s *Storage) TransitionGateEngine() *SwitchableWorkflowEngine {
	if g := s.gate.Load(); g != nil {
		return g.engine
	}
	return nil
}

// TransitionGateWired reports whether a gate is injected.
func (s *Storage) TransitionGateWired() bool { return s.gate.Load() != nil }

// unwiredEngine answers every call in Off mode (the legacy answer) when no gate
// is injected.
var unwiredEngine = NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), nil, nil)

// gateVerdict is the pure outcome of the engine decision.
type gateVerdict struct {
	Err    error
	Reason string
}

// decideTransition asks the engine whether spec may move item to req.To. It does
// no I/O beyond what a Configured operator gate itself reads.
func decideTransition(view WorkflowEngine, item *BacklogItemData, req TransitionRequest) gateVerdict {
	from := BacklogStatus(item.Status)
	snap := BuildStageConfigSnapshotFallback(item)
	in := guardInputFor(req, item, from)
	if !graphAllows(view, from, req.To, snap, in.OverrideReason) {
		return gateVerdict{
			Err:    fmt.Errorf("invalid transition from %q to %q: %w", from, req.To, ErrTransitionNotAllowed),
			Reason: DeniedConfigGraph,
		}
	}
	if err := view.ValidateGates(in, req.To, snap); err != nil {
		return gateVerdict{Err: err, Reason: classifyGateDenial(err)}
	}
	return gateVerdict{}
}

// graphAllows is the graph check. It evaluates quietly and counts a denial only
// when no operator override frees it, so an overridden transition is not
// recorded as a denied one.
func graphAllows(view WorkflowEngine, from, to BacklogStatus, snap *StageConfigSnapshot, overrideReason string) bool {
	q, quietable := view.(interface{ Quiet() WorkflowEngine })
	if !quietable {
		return view.CanTransition(from, to, snap) || operatorGraphOverride(view, overrideReason, to)
	}
	if q.Quiet().CanTransition(from, to, snap) || operatorGraphOverride(view, overrideReason, to) {
		return true
	}
	if n, ok := view.(interface{ NoteGraphDenied(from, to BacklogStatus) }); ok {
		n.NoteGraphDenied(from, to)
	}
	return false
}

// graphOverrider is implemented by a class-aware view that lets an operator's
// override_reason bypass the graph (ADR-001, ADR-007 D3).
type graphOverrider interface {
	OverridesGraph(overrideReason string, to BacklogStatus) bool
}

func operatorGraphOverride(view WorkflowEngine, overrideReason string, to BacklogStatus) bool {
	o, ok := view.(graphOverrider)
	return ok && o.OverridesGraph(overrideReason, to)
}

func guardInputFor(req TransitionRequest, item *BacklogItemData, from BacklogStatus) BacklogItemTransitionInput {
	var in BacklogItemTransitionInput
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
	case errors.Is(err, ErrGateNotSatisfied):
		if strings.Contains(err.Error(), `"`+stageConfigUnresolvableGateID+`"`) {
			return DeniedTargetStageRemoved
		}
		return DeniedOperatorGate
	default:
		return DeniedDefaultGuard
	}
}

// GatedTransition decides req with the engine and, when allowed, writes the new
// status with a compare-and-swap derived from the observed "from". A denial for
// an agent or system caller reaches the reporter before the error returns; an
// operator denial returns the error only.
func (s *Storage) GatedTransition(ctx context.Context, req TransitionRequest) (*BacklogItemData, error) {
	return s.gated(ctx, req, s.writeGated)
}

// PRAssociation is the PR a GatedSetPRAndTransition call records on the item.
type PRAssociation struct {
	URL     string
	Number  int
	Summary string
	// Guard is required to reassign an item already in pr_pending; see
	// RawSetBacklogItemPRAndTransition.
	Guard *PRReassignmentGuard
}

// pinTarget sets the target status a Gated* entry point owns, rejecting a
// request that names a different one.
func pinTarget(req *TransitionRequest, to BacklogStatus) error {
	if req.To != "" && req.To != to {
		return fmt.Errorf("%w: this entry point only moves to %q, not %q", ErrInvalidTransitionRequest, to, req.To)
	}
	req.To = to
	return nil
}

// GatedArchive decides an archive (to=archived, from any status) with the same
// decision function as GatedTransition. An already archived item decides on the
// system-required archived->archived edge and, in Off and Shadow, is re-archived
// as before. BeforeCommit runs after the allow and before the write; worktree
// cleanup and link-session ending stay with the caller, after a successful
// return.
func (s *Storage) GatedArchive(ctx context.Context, req TransitionRequest) (*BacklogItemData, error) {
	if err := pinTarget(&req, BacklogStatusArchived); err != nil {
		return nil, err
	}
	return s.gated(ctx, req, s.writeGatedArchive)
}

func (s *Storage) writeGatedArchive(ctx context.Context, req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error) {
	pre := gatePrecondition(req, spec, item)
	return s.repo.rawArchiveBacklogItem(ctx, req.ItemID, &pre, req.TriggeredBy, pre.Note)
}

// GatedUnarchive decides archived->idea. The write is a compare-and-swap on
// status only (default: the observed status, which must be archived), so the
// request cannot carry ExpectedUpdatedAt or a Note.
func (s *Storage) GatedUnarchive(ctx context.Context, req TransitionRequest) (*BacklogItemData, error) {
	if err := pinTarget(&req, BacklogStatusIdea); err != nil {
		return nil, err
	}
	if p := req.Precondition; p != nil && (p.ExpectedUpdatedAt != nil || p.Note != "") {
		return nil, fmt.Errorf("%w: unarchive carries only an ExpectedStatus", ErrInvalidTransitionRequest)
	}
	return s.gated(ctx, req, s.writeGatedUnarchive)
}

func (s *Storage) writeGatedUnarchive(ctx context.Context, req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error) {
	pre := gatePrecondition(req, spec, item)
	if pre.ExpectedStatus != string(BacklogStatusArchived) {
		return nil, fmt.Errorf("%w: item is %q, not archived", ErrPreconditionFailed, pre.ExpectedStatus)
	}
	return s.repo.rawUnarchiveBacklogItem(ctx, req.ItemID, pre.ExpectedStatus)
}

// GatedSetPRAndTransition records pr on the item and moves it review->pr_pending
// in one UPDATE after the engine allowed it. req.Observed is required and is the
// only snapshot the write is pinned to; req.Precondition must be nil because the
// write derives its own. A repeat of the recorded PR is a no-op, and a
// reassignment (observed pr_pending) is a status-preserving field write that
// PRReassignmentGuard, not the engine, decides (ADR-007 Decision 7).
func (s *Storage) GatedSetPRAndTransition(ctx context.Context, req TransitionRequest, pr PRAssociation) error {
	if _, ok := LookupTransitionCaller(req.Caller); !ok {
		return fmt.Errorf("%w: %q", ErrUnregisteredTransitionCaller, req.Caller)
	}
	if err := pinTarget(&req, BacklogStatusPRPending); err != nil {
		return err
	}
	observed := req.Observed
	if observed == nil || req.Precondition != nil {
		return fmt.Errorf("%w: a PR transition needs Observed and no Precondition", ErrInvalidTransitionRequest)
	}
	if observed.Status == string(BacklogStatusPRPending) {
		return s.setPRAndTransition(ctx, observed, pr)
	}
	_, err := s.gated(ctx, req, func(ctx context.Context, _ TransitionRequest, _ TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error) {
		return nil, s.setPRAndTransition(ctx, item, pr)
	})
	return err
}

// reconcileStuckNote is appended to the item's Notes by GatedReconcileStuck.
const reconcileStuckNote = "[auto] Transitioned to review: all work sessions ended."

// GatedReconcileStuck moves every in_progress item whose work sessions have all
// ended to review, each through the gate as CallerReconcileStuck. Atomicity is
// per item: a denied or raced item is skipped (the gate reports the denial) and
// never rolls back its siblings. It returns the number moved.
func (s *Storage) GatedReconcileStuck(ctx context.Context) (int, error) {
	candidates, err := s.repo.FindStuckInProgressCandidates(ctx)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, c := range candidates {
		_, err := s.gated(ctx, TransitionRequest{
			Caller:       CallerReconcileStuck,
			ItemID:       c.ID,
			To:           BacklogStatusReview,
			Precondition: &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusInProgress), Note: "all work sessions ended"},
			TriggeredBy:  TriggeredBySystem,
		}, s.writeGatedReconcile)
		switch {
		case err == nil:
			moved++
		case errors.Is(err, ErrPreconditionFailed):
			log.Debug("[GatedReconcileStuck] item moved concurrently, skipped", "item", c.ID)
		default:
			log.Warn("[GatedReconcileStuck] item not moved to review", "item", c.ID, "error", err)
		}
	}
	return moved, nil
}

func (s *Storage) writeGatedReconcile(ctx context.Context, req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error) {
	pre := gatePrecondition(req, spec, item)
	return s.repo.rawTransitionBacklogItemStatusWithNote(ctx, req.ItemID, req.To, &pre, req.TriggeredBy, reconcileStuckNote)
}

// gateWriter performs the raw write once the engine allowed req.
type gateWriter func(ctx context.Context, req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error)

// gated is the one decision function behind every Gated* entry point: they
// differ only in the target status they pin and the raw writer they call.
func (s *Storage) gated(ctx context.Context, req TransitionRequest, write gateWriter) (*BacklogItemData, error) {
	spec, ok := LookupTransitionCaller(req.Caller)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnregisteredTransitionCaller, req.Caller)
	}
	if err := normalizeRequest(&req); err != nil {
		return nil, err
	}
	call := &gateCall{spec: spec, engine: unwiredEngine, write: write}
	if g := s.gate.Load(); g != nil {
		call.engine, call.reporter = g.engine, g.reporter
	} else {
		noteGateUnwired()
	}

	for attempt := 1; ; attempt++ {
		call.req = req
		item, err := s.observeForGate(ctx, req)
		if err != nil {
			return nil, err
		}
		updated, casLost, err := s.gatedAttempt(ctx, call, item)
		if casLost && req.Unconditional && attempt < 2 {
			req.Observed, req.Guard = nil, nil
			continue
		}
		return updated, err
	}
}

// gateCall is what one GatedTransition invocation decides with.
type gateCall struct {
	req      TransitionRequest
	spec     TransitionCallerSpec
	engine   *SwitchableWorkflowEngine
	reporter TransitionDeniedReporter
	write    gateWriter
}

func normalizeRequest(req *TransitionRequest) error {
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

func (s *Storage) observeForGate(ctx context.Context, req TransitionRequest) (*BacklogItemData, error) {
	if req.Observed != nil {
		return req.Observed, nil
	}
	return s.GetBacklogItem(ctx, req.ItemID)
}

// gatedAttempt makes one decide-then-write pass. casLost is true only when the
// write itself lost its compare-and-swap, the one outcome Unconditional retries.
func (s *Storage) gatedAttempt(ctx context.Context, c *gateCall, item *BacklogItemData) (updated *BacklogItemData, casLost bool, err error) {
	req := c.req
	if req.Precondition != nil && req.Precondition.ExpectedStatus != "" && req.Precondition.ExpectedStatus != item.Status {
		return nil, false, fmt.Errorf("%w: expected status %q, observed %q", ErrPreconditionFailed, req.Precondition.ExpectedStatus, item.Status)
	}

	noteGateCall(c.spec.Class, c.spec.Name, c.engine.ModeFor(c.spec.Class))
	view := c.engine.ForCaller(c.spec)
	if v := decideTransition(view, item, req); v.Err != nil {
		c.reportDenial(ctx, item, v)
		return nil, false, v.Err
	}
	if req.BeforeCommit != nil {
		if err := req.BeforeCommit(ctx); err != nil {
			return nil, false, err
		}
	}
	updated, err = c.write(ctx, req, c.spec, item)
	if err == nil {
		notifyTransitionCommitted(view, guardInputFor(req, item, BacklogStatus(item.Status)), req.To)
	}
	return updated, errors.Is(err, ErrPreconditionFailed), err
}

func (c *gateCall) reportDenial(ctx context.Context, item *BacklogItemData, v gateVerdict) {
	if c.reporter == nil || c.spec.Class == ClassOperator {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[GatedTransition] denial reporter panicked for item %s caller %s: %v", item.ID, c.spec.Name, r)
		}
	}()
	c.reporter.Report(ctx, item, c.spec.Name, BacklogStatus(item.Status), c.req.To, v.Reason, v.Err)
}

// writeGated derives the CAS from the decided "from" and calls the raw writer.
func (s *Storage) writeGated(ctx context.Context, req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) (*BacklogItemData, error) {
	pre := gatePrecondition(req, spec, item)
	return s.repo.rawTransitionBacklogItemStatus(ctx, req.ItemID, req.To, &pre, req.TriggeredBy)
}

// gatePrecondition derives the raw writer's compare-and-swap from the request
// and the decided "from".
func gatePrecondition(req TransitionRequest, spec TransitionCallerSpec, item *BacklogItemData) BacklogItemPrecondition {
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
	if pre.ExpectedUpdatedAt == nil && req.Guard != nil && req.Observed != nil && spec.Guards == GuardsBuiltIn {
		observedAt := item.UpdatedAt
		pre.ExpectedUpdatedAt = &observedAt
	}
	return pre
}

// Rollback restores the status a failed multi-step operation moved the item
// away from. It is never engine-checked, only counted, and it requires an
// ExpectedStatus so it cannot overwrite a concurrent mover. It also clears the
// stateful gate latches of the edge it undoes (to -> ExpectedStatus), since the
// transition that latched them did not stick.
func (s *Storage) Rollback(ctx context.Context, itemID string, to BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	if precondition == nil || precondition.ExpectedStatus == "" {
		return nil, fmt.Errorf("%w: a rollback requires an ExpectedStatus", ErrInvalidTransitionRequest)
	}
	updated, err := s.repo.rawTransitionBacklogItemStatus(ctx, itemID, to, precondition, triggeredBy)
	if err == nil {
		RecordEngineBypass(CallerRollback)
		if g := s.gate.Load(); g != nil {
			g.engine.ClearLatches(itemID, to, BacklogStatus(precondition.ExpectedStatus))
		}
	}
	return updated, err
}

// DebugForce writes any status for the operator-only debug endpoints. It is
// never engine-checked, only counted.
func (s *Storage) DebugForce(ctx context.Context, itemID string, to BacklogStatus, triggeredBy string) (*BacklogItemData, error) {
	var (
		updated *BacklogItemData
		err     error
	)
	if to == BacklogStatusArchived {
		updated, err = s.repo.rawArchiveBacklogItem(ctx, itemID, nil, triggeredBy, "")
	} else {
		updated, err = s.repo.rawTransitionBacklogItemStatus(ctx, itemID, to, nil, triggeredBy)
	}
	if err == nil {
		RecordEngineBypass(CallerDebug)
	}
	return updated, err
}

// --- metrics ---------------------------------------------------------------

type gateCallKey struct {
	Class  CallerClass
	Caller TransitionCaller
	Mode   string
}

var (
	gateMu         sync.Mutex
	gateCallCounts = map[gateCallKey]int64{}

	gateUnwiredCount atomic.Int64
	gateUnwiredWarn  atomic.Int64

	gateCallsTotal   = mustGateCounter("workflow_engine_gate_calls_total", `Status transitions decided by Storage.GatedTransition, labeled by "class", "caller" and "mode"`)
	gateUnwiredTotal = mustGateCounter("workflow_engine_gate_unwired_total", "GatedTransition calls made while no gate was injected (answered as Off)")
)

func mustGateCounter(name, desc string) metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		// A malformed instrument name is a build-time programmer error.
		panic(err)
	}
	return c
}

func modeLabel(m EngineMode) string {
	switch m {
	case ModeShadow:
		return "shadow"
	case ModeEnforce:
		return "enforce"
	default:
		return "off"
	}
}

func noteGateCall(class CallerClass, caller TransitionCaller, mode EngineMode) {
	label := modeLabel(mode)
	gateMu.Lock()
	gateCallCounts[gateCallKey{class, caller, label}]++
	gateMu.Unlock()
	gateCallsTotal.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("class", string(class)), attribute.String("caller", string(caller)), attribute.String("mode", label)))
}

// GateCallsTotal is the in-process workflow_engine_gate_calls_total{class,caller,mode}.
func GateCallsTotal(class CallerClass, caller TransitionCaller, mode EngineMode) int64 {
	gateMu.Lock()
	defer gateMu.Unlock()
	return gateCallCounts[gateCallKey{class, caller, modeLabel(mode)}]
}

// GateUnwiredTotal is the in-process workflow_engine_gate_unwired_total.
func GateUnwiredTotal() int64 { return gateUnwiredCount.Load() }

// noteGateUnwired counts an unwired call and WARNs at most once a minute.
func noteGateUnwired() {
	gateUnwiredCount.Add(1)
	gateUnwiredTotal.Add(context.Background(), 1)
	now := time.Now().UnixNano()
	last := gateUnwiredWarn.Load()
	if last != 0 && now-last < int64(time.Minute) {
		return
	}
	if gateUnwiredWarn.CompareAndSwap(last, now) {
		log.WarningLog().Printf("[GatedTransition] no transition gate is injected; answering as Off (workflow_engine_gate_unwired_total)")
	}
}
