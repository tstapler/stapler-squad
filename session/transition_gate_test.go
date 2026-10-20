package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- fixtures --------------------------------------------------------------

type reportedDenial struct {
	itemID       string
	caller       TransitionCaller
	from, to     BacklogStatus
	reason       string
	err          error
	statusAtCall string
}

// recordingReporter records every Report and, to prove "before the write path
// returns", the item's stored status at the instant of the call.
type recordingReporter struct {
	mu      sync.Mutex
	calls   []reportedDenial
	storage *Storage
	panics  bool
}

func (r *recordingReporter) Report(ctx context.Context, item *BacklogItemData, caller TransitionCaller, from, to BacklogStatus, reason string, denyErr error) {
	cur, _ := r.storage.GetBacklogItem(ctx, item.ID)
	r.mu.Lock()
	r.calls = append(r.calls, reportedDenial{item.ID, caller, from, to, reason, denyErr, cur.Status})
	r.mu.Unlock()
	if r.panics {
		panic("reporter boom")
	}
}

func (r *recordingReporter) snapshot() []reportedDenial {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reportedDenial(nil), r.calls...)
}

type gateHarness struct {
	storage *Storage
	repo    *EntRepository
	cfg     *ConfiguredWorkflowEngine
	sel     *SwitchableWorkflowEngine
	rep     *recordingReporter
}

func newGateHarness(t *testing.T, mode EngineMode) *gateHarness {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, repo.client))
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), NewEntGateSatisfactionRepository(repo.client), nil)
	require.NoError(t, err)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	sel := NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, allClasses(mode),
		WithSelectorLoggers(func(string, ...any) {}, func(string, ...any) {}))
	rep := &recordingReporter{storage: storage}
	require.NoError(t, storage.SetTransitionGate(sel, rep))
	return &gateHarness{storage: storage, repo: repo, cfg: cfg, sel: sel, rep: rep}
}

func (h *gateHarness) newItem(t *testing.T, status BacklogStatus) *BacklogItemData {
	t.Helper()
	item, err := h.storage.CreateBacklogItem(context.Background(), BacklogItemData{Title: "gate item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func (h *gateHarness) status(t *testing.T, id string) string {
	t.Helper()
	item, err := h.storage.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	return item.Status
}

// moveRaw changes the stored status behind the gate's back (a concurrent mover).
func (h *gateHarness) moveRaw(t *testing.T, id string, to BacklogStatus) {
	t.Helper()
	_, err := h.repo.TransitionBacklogItemStatus(context.Background(), id, to, nil, TriggeredBySystem)
	require.NoError(t, err)
}

func lastEvent(t *testing.T, h *gateHarness, id string) BacklogStatusEventData {
	t.Helper()
	item, err := h.storage.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	require.NotEmpty(t, item.StatusEvents)
	last := item.StatusEvents[0]
	for _, ev := range item.StatusEvents {
		if ev.CreatedAt.After(last.CreatedAt) {
			last = ev
		}
	}
	return last
}

// --- CAS derivation --------------------------------------------------------

func TestGatedTransition_DerivesExpectedStatus_AndConcurrentMoverIsPreconditionFailedNotDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()

	item := h.newItem(t, BacklogStatusReady)
	got, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerQueueBacklogItem, ItemID: item.ID, To: BacklogStatusQueued, TriggeredBy: TriggeredBySystem})
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusQueued), got.Status)
	require.Equal(t, string(BacklogStatusReady), lastEvent(t, h, item.ID).FromStatus, "from is the observed status")

	// The caller observed ready; the item moved to queued before the write.
	stale := h.newItem(t, BacklogStatusReady)
	observed, err := h.storage.GetBacklogItem(ctx, stale.ID)
	require.NoError(t, err)
	h.moveRaw(t, stale.ID, BacklogStatusQueued)

	_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerQueueBacklogItem, Observed: observed, To: BacklogStatusIdea, TriggeredBy: TriggeredBySystem})
	require.ErrorIs(t, err, ErrPreconditionFailed)
	require.NotErrorIs(t, err, ErrTransitionNotAllowed)
	require.Equal(t, string(BacklogStatusQueued), h.status(t, stale.ID), "nothing is written")
	require.Empty(t, h.rep.snapshot(), "a lost CAS is not a denial")
}

func TestGatedTransition_CallerExpectedStatusMismatch_FailsWithoutEngineCall(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, BacklogStatusReady)

	before := GateCallsTotal(ClassSystem, CallerQueueBacklogItem, ModeEnforce)
	_, err := h.storage.GatedTransition(context.Background(), TransitionRequest{
		Caller: CallerQueueBacklogItem, ItemID: item.ID, To: BacklogStatusQueued,
		Precondition: &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusIdea)},
	})
	require.ErrorIs(t, err, ErrPreconditionFailed)
	require.Equal(t, before, GateCallsTotal(ClassSystem, CallerQueueBacklogItem, ModeEnforce), "the engine was not consulted")
	require.Equal(t, string(BacklogStatusReady), h.status(t, item.ID))
}

func TestGatedTransition_PassesWholePreconditionThrough_AndRecordsNoteAndTriggeredBy(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusReady)

	stale := item.UpdatedAt.Add(-time.Hour)
	_, err := h.storage.GatedTransition(ctx, TransitionRequest{
		Caller: CallerQueueBacklogItem, ItemID: item.ID, To: BacklogStatusQueued,
		Precondition: &BacklogItemPrecondition{ExpectedUpdatedAt: &stale},
	})
	require.ErrorIs(t, err, ErrPreconditionFailed, "ExpectedUpdatedAt is never dropped")
	require.Equal(t, string(BacklogStatusReady), h.status(t, item.ID))

	fresh, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	_, err = h.storage.GatedTransition(ctx, TransitionRequest{
		Caller: CallerQueueBacklogItem, ItemID: item.ID, To: BacklogStatusQueued, TriggeredBy: TriggeredBySystem,
		Precondition: &BacklogItemPrecondition{ExpectedUpdatedAt: &fresh.UpdatedAt, Note: "WIP cap hit"},
	})
	require.NoError(t, err)
	ev := lastEvent(t, h, item.ID)
	require.NotNil(t, ev.Note)
	require.Equal(t, "WIP cap hit", *ev.Note)
	require.Equal(t, TriggeredBySystem, ev.TriggeredBy)
}

func TestGatedTransition_PinsUpdatedAtOnlyWhenGuardInputAndObservedSupplied(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()

	run := func(t *testing.T, caller TransitionCaller, withGuard bool) error {
		t.Helper()
		item := h.newItem(t, BacklogStatusIdea)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		// An unrelated field edit bumps updated_at after the caller's read.
		title := "edited"
		_, err = h.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{Title: &title}, nil)
		require.NoError(t, err)
		req := TransitionRequest{Caller: caller, Observed: observed, To: BacklogStatusRefining}
		if withGuard {
			g := NewBacklogItemTransitionInput(observed, BacklogStatusIdea)
			req.Guard = &g
		}
		_, err = h.storage.GatedTransition(ctx, req)
		return err
	}

	require.ErrorIs(t, run(t, CallerOperatorTransitionRPC, true), ErrPreconditionFailed, "built-in guards plus prebuilt Guard pin updated_at")
	require.NoError(t, run(t, CallerOperatorTransitionRPC, false), "no Guard input: status-only CAS")
	require.NoError(t, run(t, CallerRespawnTriage, true), "guards=None: status-only CAS even with Guard input")
}

// --- Unconditional retry ---------------------------------------------------

func TestGatedTransition_Unconditional_RedecidesOnceAndNeverWritesAStaleFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("second attempt decides on the fresh status and succeeds", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, BacklogStatusQueued)

		_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerOperatorForceReset, Observed: observed, To: BacklogStatusIdea, Unconditional: true})
		require.NoError(t, err)
		require.Equal(t, string(BacklogStatusIdea), h.status(t, item.ID))
		require.Equal(t, string(BacklogStatusQueued), lastEvent(t, h, item.ID).FromStatus, "the written from is the re-read status")
	})

	t.Run("at most two attempts", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, BacklogStatusQueued)

		commits := 0
		movers := []BacklogStatus{BacklogStatusRefining, BacklogStatusQueued}
		_, err = h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorForceReset, Observed: observed, To: BacklogStatusIdea, Unconditional: true,
			BeforeCommit: func(context.Context) error { // a mover wins every race
				commits++
				h.moveRaw(t, item.ID, movers[commits-1])
				return nil
			},
		})
		require.ErrorIs(t, err, ErrPreconditionFailed)
		require.Equal(t, 2, commits)
	})

	t.Run("without Unconditional there is one attempt", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, BacklogStatusQueued)

		commits := 0
		_, err = h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorForceReset, Observed: observed, To: BacklogStatusIdea,
			BeforeCommit: func(context.Context) error { commits++; return nil },
		})
		require.ErrorIs(t, err, ErrPreconditionFailed)
		require.Equal(t, 1, commits)
	})

	t.Run("Unconditional cannot carry ExpectedUpdatedAt", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusReady)
		now := time.Now()
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorForceReset, ItemID: item.ID, To: BacklogStatusIdea, Unconditional: true,
			Precondition: &BacklogItemPrecondition{ExpectedUpdatedAt: &now},
		})
		require.ErrorIs(t, err, ErrInvalidTransitionRequest)
	})
}

// --- BeforeCommit ----------------------------------------------------------

func TestGatedTransition_BeforeCommit_RunsAfterDecisionAndBeforeCAS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("runs between the engine and the write", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusIdea)
		var statusInHook string
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusRefining,
			BeforeCommit: func(ctx context.Context) error { statusInHook = h.status(t, item.ID); return nil },
		})
		require.NoError(t, err)
		require.Equal(t, string(BacklogStatusIdea), statusInHook, "not yet written")
		require.Equal(t, string(BacklogStatusRefining), h.status(t, item.ID))
	})

	t.Run("an error aborts the write", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusIdea)
		boom := errors.New("push failed")
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusRefining,
			BeforeCommit: func(context.Context) error { return boom },
		})
		require.ErrorIs(t, err, boom)
		require.Equal(t, string(BacklogStatusIdea), h.status(t, item.ID))
	})

	t.Run("does not run for a denial", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, BacklogStatusIdea)
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusDone,
			BeforeCommit: func(context.Context) error { t.Error("BeforeCommit ran for a denied request"); return nil },
		})
		require.ErrorIs(t, err, ErrTransitionNotAllowed)
	})
}

// --- denials and the reporter ----------------------------------------------

// denialRig is one denial source: how to build the item and gate, and the edge asked.
type denialRig struct {
	source string
	setup  func(t *testing.T, h *gateHarness) (item *BacklogItemData, to BacklogStatus)
}

func denialRigs() []denialRig {
	return []denialRig{
		{DeniedConfigGraph, func(t *testing.T, h *gateHarness) (*BacklogItemData, BacklogStatus) {
			return h.newItem(t, BacklogStatusIdea), BacklogStatusDone // no such edge
		}},
		{DeniedDefaultGuard, func(t *testing.T, h *gateHarness) (*BacklogItemData, BacklogStatus) {
			return h.newItem(t, BacklogStatusReady), BacklogStatusInProgress // no approved plan
		}},
		{DeniedOperatorGate, func(t *testing.T, h *gateHarness) (*BacklogItemData, BacklogStatus) {
			addGateToSeededEdge(t, h.repo.client, h.cfg, BacklogStatusIdea, BacklogStatusRefining, GateKindHumanApproval, nil)
			return h.newItem(t, BacklogStatusIdea), BacklogStatusRefining
		}},
		{DeniedTargetStageRemoved, func(t *testing.T, h *gateHarness) (*BacklogItemData, BacklogStatus) {
			ctx := context.Background()
			from, err := h.repo.client.BacklogStage.Create().SetSlug("gone-from").SetName("Gone From").SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			to, err := h.repo.client.BacklogStage.Create().SetSlug("gone-to").SetName("Gone To").SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			_, err = h.repo.client.StageTransition.Create().SetFromStageID(from.ID).SetToStageID(to.ID).SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			require.NoError(t, h.cfg.InvalidateCache(ctx))

			item := h.newItem(t, BacklogStatusIdea)
			h.moveRaw(t, item.ID, "gone-from")
			require.NoError(t, h.repo.client.BacklogStage.DeleteOneID(from.ID).Exec(ctx))
			require.NoError(t, h.cfg.InvalidateCache(ctx))
			return item, "gone-to"
		}},
	}
}

// denialCallers are one fixed-edge caller per class: the operator RPC, an agent
// tool and a system path, each with Default built-in guards where it has any.
var denialCallers = []TransitionCaller{CallerOperatorTransitionRPC, CallerMCPResumeWork, CallerAttachSessionToItem}

func TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, rig := range denialRigs() {
		for _, caller := range denialCallers {
			spec, ok := LookupTransitionCaller(caller)
			require.True(t, ok)
			t.Run(rig.source+"/"+string(spec.Class), func(t *testing.T) {
				t.Parallel()
				h := newGateHarness(t, ModeEnforce)
				item, to := rig.setup(t, h)
				fromStatus := h.status(t, item.ID)

				_, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: caller, ItemID: item.ID, To: to})
				require.Error(t, err)
				require.Equal(t, fromStatus, h.status(t, item.ID), "nothing is written")

				calls := h.rep.snapshot()
				if spec.Class == ClassOperator {
					require.Empty(t, calls, "an operator denial returns the error only")
					return
				}
				require.Len(t, calls, 1, "exactly one Report per denial")
				got := calls[0]
				require.Equal(t, rig.source, got.reason)
				require.Equal(t, caller, got.caller)
				require.Equal(t, BacklogStatus(fromStatus), got.from)
				require.Equal(t, to, got.to)
				require.Equal(t, item.ID, got.itemID)
				require.Equal(t, fromStatus, got.statusAtCall, "reported before anything could be written")
				require.ErrorIs(t, got.err, err)
			})
		}
	}
}

func TestGatedTransition_SameEdgeDeniedForDifferentCallers_ReportsSeparately(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, BacklogStatusIdea)
	// Two system callers share the idea->ready edge; an operator gate denies both.
	for _, caller := range []TransitionCaller{CallerAutonomousComplete, CallerMaybeTriggerTriage} {
		spec, _ := LookupTransitionCaller(caller)
		require.Equal(t, GuardsNone, spec.Guards)
	}
	addGateToSeededEdge(t, h.repo.client, h.cfg, BacklogStatusIdea, BacklogStatusReady, GateKindHumanApproval, nil)
	for _, caller := range []TransitionCaller{CallerAutonomousComplete, CallerMaybeTriggerTriage, CallerAutonomousComplete} {
		_, err := h.storage.GatedTransition(context.Background(), TransitionRequest{Caller: caller, ItemID: item.ID, To: BacklogStatusReady})
		require.ErrorIs(t, err, ErrGateNotSatisfied)
	}
	calls := h.rep.snapshot()
	require.Len(t, calls, 3, "the gate reports every denial; the reporter suppresses repeats while a row is open")
	require.Equal(t, CallerAutonomousComplete, calls[0].caller)
	require.Equal(t, CallerMaybeTriggerTriage, calls[1].caller)
}

func TestGatedTransition_ReporterPanicNeverChangesTheDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	h.rep.panics = true
	item := h.newItem(t, BacklogStatusIdea)
	_, err := h.storage.GatedTransition(context.Background(), TransitionRequest{Caller: CallerMCPResumeWork, ItemID: item.ID, To: BacklogStatusDone})
	require.ErrorIs(t, err, ErrTransitionNotAllowed)
	require.Len(t, h.rep.snapshot(), 1)
}

// --- override policy through the gate --------------------------------------

func TestGatedTransition_OverrideReason_BypassesOperatorGatesForOperatorsOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := func(t *testing.T, caller TransitionCaller, reason string) error {
		h := newGateHarness(t, ModeEnforce)
		addGateToSeededEdge(t, h.repo.client, h.cfg, BacklogStatusIdea, BacklogStatusRefining, GateKindHumanApproval, nil)
		item := h.newItem(t, BacklogStatusIdea)
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: caller, ItemID: item.ID, To: BacklogStatusRefining, OverrideReason: reason})
		return err
	}
	require.ErrorIs(t, run(t, CallerOperatorTransitionRPC, ""), ErrGateNotSatisfied)
	require.NoError(t, run(t, CallerOperatorTransitionRPC, "owner says so"))
	require.ErrorIs(t, run(t, CallerMCPResumeWork, "agent insists"), ErrGateNotSatisfied, "an agent never bypasses a Configured gate")
	require.ErrorIs(t, run(t, CallerAttachSessionToItem, "system has none"), ErrGateNotSatisfied)
}

// --- modes, unwired, registry ----------------------------------------------

func TestGatedTransition_UncheckedCallerIsAllowedInOffAndShadow_DeniedInEnforce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		mode    EngineMode
		allowed bool
	}{{ModeOff, true}, {ModeShadow, true}, {ModeEnforce, false}} {
		h := newGateHarness(t, tc.mode)
		item := h.newItem(t, BacklogStatusIdea)
		// mcp_resume_work is an Unchecked agent caller: today a bare storage write.
		_, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerMCPResumeWork, ItemID: item.ID, To: BacklogStatusDone})
		if tc.allowed {
			require.NoErrorf(t, err, "mode %s", modeLabel(tc.mode))
			require.Equal(t, string(BacklogStatusDone), h.status(t, item.ID))
		} else {
			require.ErrorIsf(t, err, ErrTransitionNotAllowed, "mode %s", modeLabel(tc.mode))
		}
	}
}

func TestGatedTransition_Unwired_BehavesAsOff_AndCountsUnwired(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	require.False(t, storage.TransitionGateWired())
	require.Nil(t, storage.TransitionGateEngine())

	item, err := storage.CreateBacklogItem(context.Background(), BacklogItemData{Title: "unwired", Status: string(BacklogStatusIdea)})
	require.NoError(t, err)

	before := GateUnwiredTotal()
	// Unchecked caller, edge the graph lacks: the legacy answer is allow.
	got, err := storage.GatedTransition(context.Background(), TransitionRequest{Caller: CallerMCPResumeWork, ItemID: item.ID, To: BacklogStatusDone})
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), got.Status)
	require.Greater(t, GateUnwiredTotal(), before)

	// An engine-legacy caller still gets Default's answer.
	other, err := storage.CreateBacklogItem(context.Background(), BacklogItemData{Title: "unwired 2", Status: string(BacklogStatusIdea)})
	require.NoError(t, err)
	_, err = storage.GatedTransition(context.Background(), TransitionRequest{Caller: CallerQueueBacklogItem, ItemID: other.ID, To: BacklogStatusDone})
	require.ErrorIs(t, err, ErrTransitionNotAllowed)
}

func TestSetTransitionGate_RequiresEngineAndReporter(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeOff)
	require.ErrorIs(t, h.storage.SetTransitionGate(nil, h.rep), ErrInvalidTransitionRequest)
	require.ErrorIs(t, h.storage.SetTransitionGate(h.sel, nil), ErrInvalidTransitionRequest)
	require.True(t, h.storage.TransitionGateWired(), "a rejected injection leaves the gate as it was")
	require.Same(t, h.sel, h.storage.TransitionGateEngine())
}

func TestGatedTransition_RejectsUnregisteredCallerAndMalformedRequests(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, BacklogStatusIdea)
	ctx := context.Background()

	_, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: "nobody", ItemID: item.ID, To: BacklogStatusReady})
	require.ErrorIs(t, err, ErrUnregisteredTransitionCaller)
	_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerOperatorTransitionRPC, To: BacklogStatusReady})
	require.ErrorIs(t, err, ErrInvalidTransitionRequest)
	_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerOperatorTransitionRPC, ItemID: item.ID})
	require.ErrorIs(t, err, ErrInvalidTransitionRequest)
	_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerOperatorTransitionRPC, ItemID: "other", Observed: item, To: BacklogStatusReady})
	require.ErrorIs(t, err, ErrInvalidTransitionRequest)
}

func TestGatedTransition_RecordsGateCallsPerClassCallerAndMode(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeShadow)
	item := h.newItem(t, BacklogStatusIdea)
	before := GateCallsTotal(ClassOperator, CallerOperatorTransitionRPC, ModeShadow)
	_, err := h.storage.GatedTransition(context.Background(), TransitionRequest{Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusRefining})
	require.NoError(t, err)
	require.Equal(t, before+1, GateCallsTotal(ClassOperator, CallerOperatorTransitionRPC, ModeShadow))
}

// --- exempt entry points ---------------------------------------------------

func TestRollback_RequiresExpectedStatus_AndCountsBypass(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusQueued)

	_, err := h.storage.Rollback(ctx, item.ID, BacklogStatusReady, nil, TriggeredBySystem)
	require.ErrorIs(t, err, ErrInvalidTransitionRequest)
	_, err = h.storage.Rollback(ctx, item.ID, BacklogStatusReady, &BacklogItemPrecondition{}, TriggeredBySystem)
	require.ErrorIs(t, err, ErrInvalidTransitionRequest)

	before := EngineBypassCount(CallerRollback)
	_, err = h.storage.Rollback(ctx, item.ID, BacklogStatusReady, &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReady)}, TriggeredBySystem)
	require.ErrorIs(t, err, ErrPreconditionFailed)
	require.Equal(t, before, EngineBypassCount(CallerRollback), "a lost CAS is not a bypass write")

	_, err = h.storage.Rollback(ctx, item.ID, BacklogStatusReady, &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusQueued)}, TriggeredBySystem)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), h.status(t, item.ID))
	require.Equal(t, before+1, EngineBypassCount(CallerRollback))
}

func TestDebugForce_WritesAnyStatusUnchecked_AndCountsBypass(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)

	before := EngineBypassCount(CallerDebug)
	_, err := h.storage.DebugForce(ctx, item.ID, BacklogStatusDone, TriggeredByUser) // idea->done has no edge
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), h.status(t, item.ID))

	_, err = h.storage.DebugForce(ctx, item.ID, BacklogStatusArchived, TriggeredByUser)
	require.NoError(t, err)
	archived, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt, "archive goes through the archive writer")
	require.Equal(t, before+2, EngineBypassCount(CallerDebug))
}
