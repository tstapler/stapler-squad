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
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/telemetry"
)

type transitionGate struct {
	engine   *SwitchableWorkflowEngine
	reporter backlogdto.TransitionDeniedReporter
}

// SetTransitionGate injects the selector and the denial reporter together, so
// the gate can never exist without its reporter (ADR-007 D8).
func (s *Storage) SetTransitionGate(engine *SwitchableWorkflowEngine, reporter backlogdto.TransitionDeniedReporter) error {
	if engine == nil || reporter == nil {
		return fmt.Errorf("%w: the transition gate needs an engine and a reporter", backlogdto.ErrInvalidTransitionRequest)
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

// GatedTransition decides req with the engine and, when allowed, writes the new
// status with a compare-and-swap derived from the observed "from". A denial for
// an agent or system caller reaches the reporter before the error returns; an
// operator denial returns the error only.
func (s *Storage) GatedTransition(ctx context.Context, req backlogdto.TransitionRequest) (*backlogdto.BacklogItemData, error) {
	return s.gated(ctx, req, s.writeGated)
}

// GatedArchive decides an archive (to=archived, from any status) with the same
// decision function as GatedTransition. An already archived item decides on the
// system-required archived->archived edge and, in Off and Shadow, is re-archived
// as before. BeforeCommit runs after the allow and before the write; worktree
// cleanup and link-session ending stay with the caller, after a successful
// return.
func (s *Storage) GatedArchive(ctx context.Context, req backlogdto.TransitionRequest) (*backlogdto.BacklogItemData, error) {
	if err := backlogdto.PinTarget(&req, domain.BacklogStatusArchived); err != nil {
		return nil, err
	}
	return s.gated(ctx, req, s.writeGatedArchive)
}

func (s *Storage) writeGatedArchive(ctx context.Context, req backlogdto.TransitionRequest, spec domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	pre := backlogdto.GatePrecondition(req, spec, item)
	return s.Repo.rawArchiveBacklogItem(ctx, req.ItemID, &pre, req.TriggeredBy, pre.Note)
}

// GatedUnarchive decides archived->idea. The write is a compare-and-swap on
// status only (default: the observed status, which must be archived), so the
// request cannot carry ExpectedUpdatedAt or a Note.
func (s *Storage) GatedUnarchive(ctx context.Context, req backlogdto.TransitionRequest) (*backlogdto.BacklogItemData, error) {
	if err := backlogdto.PinTarget(&req, domain.BacklogStatusIdea); err != nil {
		return nil, err
	}
	if p := req.Precondition; p != nil && (p.ExpectedUpdatedAt != nil || p.Note != "") {
		return nil, fmt.Errorf("%w: unarchive carries only an ExpectedStatus", backlogdto.ErrInvalidTransitionRequest)
	}
	return s.gated(ctx, req, s.writeGatedUnarchive)
}

func (s *Storage) writeGatedUnarchive(ctx context.Context, req backlogdto.TransitionRequest, spec domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	pre := backlogdto.GatePrecondition(req, spec, item)
	if pre.ExpectedStatus != string(domain.BacklogStatusArchived) {
		return nil, fmt.Errorf("%w: item is %q, not archived", domain.ErrPreconditionFailed, pre.ExpectedStatus)
	}
	return s.Repo.rawUnarchiveBacklogItem(ctx, req.ItemID, pre.ExpectedStatus)
}

// GatedSetPRAndTransition records pr on the item and moves it review->pr_pending
// in one UPDATE after the engine allowed it. req.Observed is required and is the
// only snapshot the write is pinned to; req.Precondition must be nil because the
// write derives its own. A repeat of the recorded PR is a no-op, and a
// reassignment (observed pr_pending) is a status-preserving field write that
// PRReassignmentGuard, not the engine, decides (ADR-007 Decision 7).
func (s *Storage) GatedSetPRAndTransition(ctx context.Context, req backlogdto.TransitionRequest, pr backlogdto.PRAssociation) error {
	if _, ok := domain.LookupTransitionCaller(req.Caller); !ok {
		return fmt.Errorf("%w: %q", backlogdto.ErrUnregisteredTransitionCaller, req.Caller)
	}
	if err := backlogdto.PinTarget(&req, domain.BacklogStatusPRPending); err != nil {
		return err
	}
	observed := req.Observed
	if observed == nil || req.Precondition != nil {
		return fmt.Errorf("%w: a PR transition needs Observed and no Precondition", backlogdto.ErrInvalidTransitionRequest)
	}
	if observed.Status == string(domain.BacklogStatusPRPending) {
		return s.setPRAndTransition(ctx, observed, pr)
	}
	_, err := s.gated(ctx, req, func(ctx context.Context, _ backlogdto.TransitionRequest, _ domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
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
	candidates, err := s.Repo.FindStuckInProgressCandidates(ctx)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, c := range candidates {
		_, err := s.gated(ctx, backlogdto.TransitionRequest{
			Caller:       domain.CallerReconcileStuck,
			ItemID:       c.ID,
			To:           domain.BacklogStatusReview,
			Precondition: &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusInProgress), Note: "all work sessions ended"},
			TriggeredBy:  instance.TriggeredBySystem,
		}, s.writeGatedReconcile)
		switch {
		case err == nil:
			moved++
		case errors.Is(err, domain.ErrPreconditionFailed):
			log.Debug("[GatedReconcileStuck] item moved concurrently, skipped", "item", c.ID)
		default:
			log.Warn("[GatedReconcileStuck] item not moved to review", "item", c.ID, "error", err)
		}
	}
	return moved, nil
}

func (s *Storage) writeGatedReconcile(ctx context.Context, req backlogdto.TransitionRequest, spec domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	pre := backlogdto.GatePrecondition(req, spec, item)
	return s.Repo.rawTransitionBacklogItemStatusWithNote(ctx, req.ItemID, req.To, &pre, req.TriggeredBy, reconcileStuckNote)
}

// gateWriter performs the raw write once the engine allowed req.
type gateWriter func(ctx context.Context, req backlogdto.TransitionRequest, spec domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error)

// gated is the one decision function behind every Gated* entry point: they
// differ only in the target status they pin and the raw writer they call.
func (s *Storage) gated(ctx context.Context, req backlogdto.TransitionRequest, write gateWriter) (*backlogdto.BacklogItemData, error) {
	spec, ok := domain.LookupTransitionCaller(req.Caller)
	if !ok {
		return nil, fmt.Errorf("%w: %q", backlogdto.ErrUnregisteredTransitionCaller, req.Caller)
	}
	if err := backlogdto.NormalizeRequest(&req); err != nil {
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
	req      backlogdto.TransitionRequest
	spec     domain.TransitionCallerSpec
	engine   *SwitchableWorkflowEngine
	reporter backlogdto.TransitionDeniedReporter
	write    gateWriter
}

func (s *Storage) observeForGate(ctx context.Context, req backlogdto.TransitionRequest) (*backlogdto.BacklogItemData, error) {
	if req.Observed != nil {
		return req.Observed, nil
	}
	return s.GetBacklogItem(ctx, req.ItemID)
}

// gatedAttempt makes one decide-then-write pass. casLost is true only when the
// write itself lost its compare-and-swap, the one outcome Unconditional retries.
func (s *Storage) gatedAttempt(ctx context.Context, c *gateCall, item *backlogdto.BacklogItemData) (updated *backlogdto.BacklogItemData, casLost bool, err error) {
	req := c.req
	if req.Precondition != nil && req.Precondition.ExpectedStatus != "" && req.Precondition.ExpectedStatus != item.Status {
		return nil, false, fmt.Errorf("%w: expected status %q, observed %q", domain.ErrPreconditionFailed, req.Precondition.ExpectedStatus, item.Status)
	}

	noteGateCall(c.spec.Class, c.spec.Name, c.engine.ModeFor(c.spec.Class))
	view := c.engine.ForCaller(c.spec)
	if v := backlogdto.DecideTransition(view, item, req); v.Err != nil {
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
		notifyTransitionCommitted(view, backlogdto.GuardInputFor(req, item, domain.BacklogStatus(item.Status)), req.To)
	}
	return updated, errors.Is(err, domain.ErrPreconditionFailed), err
}

func (c *gateCall) reportDenial(ctx context.Context, item *backlogdto.BacklogItemData, v backlogdto.GateVerdict) {
	if c.reporter == nil || c.spec.Class == domain.ClassOperator {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[GatedTransition] denial reporter panicked for item %s caller %s: %v", item.ID, c.spec.Name, r)
		}
	}()
	c.reporter.Report(ctx, item, c.spec.Name, domain.BacklogStatus(item.Status), c.req.To, v.Reason, v.Err)
}

// writeGated derives the CAS from the decided "from" and calls the raw writer.
func (s *Storage) writeGated(ctx context.Context, req backlogdto.TransitionRequest, spec domain.TransitionCallerSpec, item *backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	pre := backlogdto.GatePrecondition(req, spec, item)
	return s.Repo.RawTransitionBacklogItemStatus(ctx, req.ItemID, req.To, &pre, req.TriggeredBy)
}

// Rollback restores the status a failed multi-step operation moved the item
// away from. It is never engine-checked, only counted, and it requires an
// ExpectedStatus so it cannot overwrite a concurrent mover. It also clears the
// stateful gate latches of the edge it undoes (to -> ExpectedStatus), since the
// transition that latched them did not stick.
func (s *Storage) Rollback(ctx context.Context, itemID string, to domain.BacklogStatus, precondition *backlogdto.BacklogItemPrecondition, triggeredBy string) (*backlogdto.BacklogItemData, error) {
	if precondition == nil || precondition.ExpectedStatus == "" {
		return nil, fmt.Errorf("%w: a rollback requires an ExpectedStatus", backlogdto.ErrInvalidTransitionRequest)
	}
	updated, err := s.Repo.RawTransitionBacklogItemStatus(ctx, itemID, to, precondition, triggeredBy)
	if err == nil {
		RecordEngineBypass(backlogdto.CallerRollback)
		if g := s.gate.Load(); g != nil {
			g.engine.ClearLatches(itemID, to, domain.BacklogStatus(precondition.ExpectedStatus))
		}
	}
	return updated, err
}

// DebugForce writes any status for the operator-only debug endpoints. It is
// never engine-checked, only counted.
func (s *Storage) DebugForce(ctx context.Context, itemID string, to domain.BacklogStatus, triggeredBy string) (*backlogdto.BacklogItemData, error) {
	var (
		updated *backlogdto.BacklogItemData
		err     error
	)
	if to == domain.BacklogStatusArchived {
		updated, err = s.Repo.rawArchiveBacklogItem(ctx, itemID, nil, triggeredBy, "")
	} else {
		updated, err = s.Repo.RawTransitionBacklogItemStatus(ctx, itemID, to, nil, triggeredBy)
	}
	if err == nil {
		RecordEngineBypass(backlogdto.CallerDebug)
	}
	return updated, err
}

// --- metrics ---------------------------------------------------------------

type gateCallKey struct {
	Class  domain.CallerClass
	Caller domain.TransitionCaller
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

func noteGateCall(class domain.CallerClass, caller domain.TransitionCaller, mode EngineMode) {
	label := modeLabel(mode)
	gateMu.Lock()
	gateCallCounts[gateCallKey{class, caller, label}]++
	gateMu.Unlock()
	gateCallsTotal.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("class", string(class)), attribute.String("caller", string(caller)), attribute.String("mode", label)))
}

// GateCallsTotal is the in-process workflow_engine_gate_calls_total{class,caller,mode}.
func GateCallsTotal(class domain.CallerClass, caller domain.TransitionCaller, mode EngineMode) int64 {
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
