package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// --- fixtures --------------------------------------------------------------

func lastEvent(t *testing.T, h *gateHarness, id string) backlogdto.BacklogStatusEventData {
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

	item := h.newItem(t, domain.BacklogStatusReady)
	got, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerQueueBacklogItem, ItemID: item.ID, To: domain.BacklogStatusQueued, TriggeredBy: instance.TriggeredBySystem})
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusQueued), got.Status)
	require.Equal(t, string(domain.BacklogStatusReady), lastEvent(t, h, item.ID).FromStatus, "from is the observed status")

	// The caller observed ready; the item moved to queued before the write.
	stale := h.newItem(t, domain.BacklogStatusReady)
	observed, err := h.storage.GetBacklogItem(ctx, stale.ID)
	require.NoError(t, err)
	h.moveRaw(t, stale.ID, domain.BacklogStatusQueued)

	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerQueueBacklogItem, Observed: observed, To: domain.BacklogStatusIdea, TriggeredBy: instance.TriggeredBySystem})
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	require.NotErrorIs(t, err, ErrTransitionNotAllowed)
	require.Equal(t, string(domain.BacklogStatusQueued), h.status(t, stale.ID), "nothing is written")
	require.Empty(t, h.rep.snapshot(), "a lost CAS is not a denial")
}

func TestGatedTransition_CallerExpectedStatusMismatch_FailsWithoutEngineCall(t *testing.T) {
	// Not parallel: GateCallsTotal is process-wide, so a parallel test using the same caller would bump it between the two reads.
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusReady)

	before := GateCallsTotal(domain.ClassSystem, domain.CallerQueueBacklogItem, ModeEnforce)
	_, err := h.storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{
		Caller: domain.CallerQueueBacklogItem, ItemID: item.ID, To: domain.BacklogStatusQueued,
		Precondition: &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusIdea)},
	})
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	require.Equal(t, before, GateCallsTotal(domain.ClassSystem, domain.CallerQueueBacklogItem, ModeEnforce), "the engine was not consulted")
	require.Equal(t, string(domain.BacklogStatusReady), h.status(t, item.ID))
}

func TestGatedTransition_PassesWholePreconditionThrough_AndRecordsNoteAndTriggeredBy(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, domain.BacklogStatusReady)

	stale := item.UpdatedAt.Add(-time.Hour)
	_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
		Caller: domain.CallerQueueBacklogItem, ItemID: item.ID, To: domain.BacklogStatusQueued,
		Precondition: &backlogdto.BacklogItemPrecondition{ExpectedUpdatedAt: &stale},
	})
	require.ErrorIs(t, err, domain.ErrPreconditionFailed, "ExpectedUpdatedAt is never dropped")
	require.Equal(t, string(domain.BacklogStatusReady), h.status(t, item.ID))

	fresh, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
		Caller: domain.CallerQueueBacklogItem, ItemID: item.ID, To: domain.BacklogStatusQueued, TriggeredBy: instance.TriggeredBySystem,
		Precondition: &backlogdto.BacklogItemPrecondition{ExpectedUpdatedAt: &fresh.UpdatedAt, Note: "WIP cap hit"},
	})
	require.NoError(t, err)
	ev := lastEvent(t, h, item.ID)
	require.NotNil(t, ev.Note)
	require.Equal(t, "WIP cap hit", *ev.Note)
	require.Equal(t, instance.TriggeredBySystem, ev.TriggeredBy)
}

func TestGatedTransition_PinsUpdatedAtOnlyWhenGuardInputAndObservedSupplied(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()

	run := func(t *testing.T, caller domain.TransitionCaller, withGuard bool) error {
		t.Helper()
		item := h.newItem(t, domain.BacklogStatusIdea)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		// An unrelated field edit bumps updated_at after the caller's read.
		title := "edited"
		_, err = h.storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{Title: &title}, nil)
		require.NoError(t, err)
		req := backlogdto.TransitionRequest{Caller: caller, Observed: observed, To: domain.BacklogStatusRefining}
		if withGuard {
			g := backlogdto.NewBacklogItemTransitionInput(observed, domain.BacklogStatusIdea)
			req.Guard = &g
		}
		_, err = h.storage.GatedTransition(ctx, req)
		return err
	}

	require.ErrorIs(t, run(t, domain.CallerOperatorTransitionRPC, true), domain.ErrPreconditionFailed, "built-in guards plus prebuilt Guard pin updated_at")
	require.NoError(t, run(t, domain.CallerOperatorTransitionRPC, false), "no Guard input: status-only CAS")
	require.NoError(t, run(t, domain.CallerRespawnTriage, true), "guards=None: status-only CAS even with Guard input")
}

// --- Unconditional retry ---------------------------------------------------

func TestGatedTransition_Unconditional_RedecidesOnceAndNeverWritesAStaleFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("second attempt decides on the fresh status and succeeds", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, domain.BacklogStatusQueued)

		_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorForceReset, Observed: observed, To: domain.BacklogStatusIdea, Unconditional: true})
		require.NoError(t, err)
		require.Equal(t, string(domain.BacklogStatusIdea), h.status(t, item.ID))
		require.Equal(t, string(domain.BacklogStatusQueued), lastEvent(t, h, item.ID).FromStatus, "the written from is the re-read status")
	})

	t.Run("at most two attempts", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, domain.BacklogStatusQueued)

		commits := 0
		movers := []domain.BacklogStatus{domain.BacklogStatusRefining, domain.BacklogStatusQueued}
		_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorForceReset, Observed: observed, To: domain.BacklogStatusIdea, Unconditional: true,
			BeforeCommit: func(context.Context) error { // a mover wins every race
				commits++
				h.moveRaw(t, item.ID, movers[commits-1])
				return nil
			},
		})
		require.ErrorIs(t, err, domain.ErrPreconditionFailed)
		require.Equal(t, 2, commits)
	})

	t.Run("without Unconditional there is one attempt", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusReady)
		observed, err := h.storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		h.moveRaw(t, item.ID, domain.BacklogStatusQueued)

		commits := 0
		_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorForceReset, Observed: observed, To: domain.BacklogStatusIdea,
			BeforeCommit: func(context.Context) error { commits++; return nil },
		})
		require.ErrorIs(t, err, domain.ErrPreconditionFailed)
		require.Equal(t, 1, commits)
	})

	t.Run("Unconditional cannot carry ExpectedUpdatedAt", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusReady)
		now := time.Now()
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorForceReset, ItemID: item.ID, To: domain.BacklogStatusIdea, Unconditional: true,
			Precondition: &backlogdto.BacklogItemPrecondition{ExpectedUpdatedAt: &now},
		})
		require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
	})
}

// --- BeforeCommit ----------------------------------------------------------

func TestGatedTransition_BeforeCommit_RunsAfterDecisionAndBeforeCAS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("runs between the engine and the write", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusIdea)
		var statusInHook string
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusRefining,
			BeforeCommit: func(ctx context.Context) error { statusInHook = h.status(t, item.ID); return nil },
		})
		require.NoError(t, err)
		require.Equal(t, string(domain.BacklogStatusIdea), statusInHook, "not yet written")
		require.Equal(t, string(domain.BacklogStatusRefining), h.status(t, item.ID))
	})

	t.Run("an error aborts the write", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusIdea)
		boom := errors.New("push failed")
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusRefining,
			BeforeCommit: func(context.Context) error { return boom },
		})
		require.ErrorIs(t, err, boom)
		require.Equal(t, string(domain.BacklogStatusIdea), h.status(t, item.ID))
	})

	t.Run("does not run for a denial", func(t *testing.T) {
		t.Parallel()
		h := newGateHarness(t, ModeEnforce)
		item := h.newItem(t, domain.BacklogStatusIdea)
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusDone,
			BeforeCommit: func(context.Context) error { t.Error("BeforeCommit ran for a denied request"); return nil },
		})
		require.ErrorIs(t, err, ErrTransitionNotAllowed)
	})
}

// --- denials and the reporter ----------------------------------------------

// denialRig is one denial source: how to build the item and gate, and the edge asked.
type denialRig struct {
	source string
	setup  func(t *testing.T, h *gateHarness) (item *backlogdto.BacklogItemData, to domain.BacklogStatus)
}

func denialRigs() []denialRig {
	return []denialRig{
		{backlogdto.DeniedConfigGraph, func(t *testing.T, h *gateHarness) (*backlogdto.BacklogItemData, domain.BacklogStatus) {
			return h.newItem(t, domain.BacklogStatusIdea), domain.BacklogStatusDone // no such edge
		}},
		{backlogdto.DeniedDefaultGuard, func(t *testing.T, h *gateHarness) (*backlogdto.BacklogItemData, domain.BacklogStatus) {
			return h.newItem(t, domain.BacklogStatusReady), domain.BacklogStatusInProgress // no approved plan
		}},
		{backlogdto.DeniedOperatorGate, func(t *testing.T, h *gateHarness) (*backlogdto.BacklogItemData, domain.BacklogStatus) {
			addGateToSeededEdge(t, h.repo.Client, h.cfg, domain.BacklogStatusIdea, domain.BacklogStatusRefining, domain.GateKindHumanApproval, nil)
			return h.newItem(t, domain.BacklogStatusIdea), domain.BacklogStatusRefining
		}},
		{backlogdto.DeniedTargetStageRemoved, func(t *testing.T, h *gateHarness) (*backlogdto.BacklogItemData, domain.BacklogStatus) {
			ctx := context.Background()
			from, err := h.repo.Client.BacklogStage.Create().SetSlug("gone-from").SetName("Gone From").SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			to, err := h.repo.Client.BacklogStage.Create().SetSlug("gone-to").SetName("Gone To").SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			_, err = h.repo.Client.StageTransition.Create().SetFromStageID(from.ID).SetToStageID(to.ID).SetEnabled(true).Save(ctx)
			require.NoError(t, err)
			require.NoError(t, h.cfg.InvalidateCache(ctx))

			item := h.newItem(t, domain.BacklogStatusIdea)
			h.moveRaw(t, item.ID, "gone-from")
			require.NoError(t, h.repo.Client.BacklogStage.DeleteOneID(from.ID).Exec(ctx))
			require.NoError(t, h.cfg.InvalidateCache(ctx))
			return item, "gone-to"
		}},
	}
}

// denialCallers are one fixed-edge caller per class: the operator RPC, an agent
// tool and a system path, each with Default built-in guards where it has any.
var denialCallers = []domain.TransitionCaller{domain.CallerOperatorTransitionRPC, domain.CallerMCPResumeWork, domain.CallerAttachSessionToItem}

func TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, rig := range denialRigs() {
		for _, caller := range denialCallers {
			spec, ok := domain.LookupTransitionCaller(caller)
			require.True(t, ok)
			t.Run(rig.source+"/"+string(spec.Class), func(t *testing.T) {
				t.Parallel()
				h := newGateHarness(t, ModeEnforce)
				item, to := rig.setup(t, h)
				fromStatus := h.status(t, item.ID)

				_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: caller, ItemID: item.ID, To: to})
				require.Error(t, err)
				require.Equal(t, fromStatus, h.status(t, item.ID), "nothing is written")

				calls := h.rep.snapshot()
				if spec.Class == domain.ClassOperator {
					require.Empty(t, calls, "an operator denial returns the error only")
					return
				}
				require.Len(t, calls, 1, "exactly one Report per denial")
				got := calls[0]
				require.Equal(t, rig.source, got.reason)
				require.Equal(t, caller, got.caller)
				require.Equal(t, domain.BacklogStatus(fromStatus), got.from)
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
	item := h.newItem(t, domain.BacklogStatusIdea)
	// Two system callers share the idea->ready edge; an operator gate denies both.
	for _, caller := range []domain.TransitionCaller{domain.CallerAutonomousComplete, domain.CallerMaybeTriggerTriage} {
		spec, _ := domain.LookupTransitionCaller(caller)
		require.Equal(t, domain.GuardsNone, spec.Guards)
	}
	addGateToSeededEdge(t, h.repo.Client, h.cfg, domain.BacklogStatusIdea, domain.BacklogStatusReady, domain.GateKindHumanApproval, nil)
	for _, caller := range []domain.TransitionCaller{domain.CallerAutonomousComplete, domain.CallerMaybeTriggerTriage, domain.CallerAutonomousComplete} {
		_, err := h.storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{Caller: caller, ItemID: item.ID, To: domain.BacklogStatusReady})
		require.ErrorIs(t, err, domain.ErrGateNotSatisfied)
	}
	calls := h.rep.snapshot()
	require.Len(t, calls, 3, "the gate reports every denial; the reporter suppresses repeats while a row is open")
	require.Equal(t, domain.CallerAutonomousComplete, calls[0].caller)
	require.Equal(t, domain.CallerMaybeTriggerTriage, calls[1].caller)
}

func TestGatedTransition_ReporterPanicNeverChangesTheDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	h.rep.panics = true
	item := h.newItem(t, domain.BacklogStatusIdea)
	_, err := h.storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{Caller: domain.CallerMCPResumeWork, ItemID: item.ID, To: domain.BacklogStatusDone})
	require.ErrorIs(t, err, ErrTransitionNotAllowed)
	require.Len(t, h.rep.snapshot(), 1)
}

// --- override policy through the gate --------------------------------------

func TestGatedTransition_OverrideReason_BypassesOperatorGatesForOperatorsOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := func(t *testing.T, caller domain.TransitionCaller, reason string) error {
		h := newGateHarness(t, ModeEnforce)
		addGateToSeededEdge(t, h.repo.Client, h.cfg, domain.BacklogStatusIdea, domain.BacklogStatusRefining, domain.GateKindHumanApproval, nil)
		item := h.newItem(t, domain.BacklogStatusIdea)
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: caller, ItemID: item.ID, To: domain.BacklogStatusRefining, OverrideReason: reason})
		return err
	}
	require.ErrorIs(t, run(t, domain.CallerOperatorTransitionRPC, ""), domain.ErrGateNotSatisfied)
	require.NoError(t, run(t, domain.CallerOperatorTransitionRPC, "owner says so"))
	require.ErrorIs(t, run(t, domain.CallerMCPResumeWork, "agent insists"), domain.ErrGateNotSatisfied, "an agent never bypasses a Configured gate")
	require.ErrorIs(t, run(t, domain.CallerAttachSessionToItem, "system has none"), domain.ErrGateNotSatisfied)
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
		item := h.newItem(t, domain.BacklogStatusIdea)
		// mcp_resume_work is an Unchecked agent caller: today a bare storage write.
		_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPResumeWork, ItemID: item.ID, To: domain.BacklogStatusDone})
		if tc.allowed {
			require.NoErrorf(t, err, "mode %s", modeLabel(tc.mode))
			require.Equal(t, string(domain.BacklogStatusDone), h.status(t, item.ID))
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

	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "unwired", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)

	before := GateUnwiredTotal()
	// Unchecked caller, edge the graph lacks: the legacy answer is allow.
	got, err := storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{Caller: domain.CallerMCPResumeWork, ItemID: item.ID, To: domain.BacklogStatusDone})
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusDone), got.Status)
	require.Greater(t, GateUnwiredTotal(), before)

	// An engine-legacy caller still gets Default's answer.
	other, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "unwired 2", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	_, err = storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{Caller: domain.CallerQueueBacklogItem, ItemID: other.ID, To: domain.BacklogStatusDone})
	require.ErrorIs(t, err, ErrTransitionNotAllowed)
}

func TestSetTransitionGate_RequiresEngineAndReporter(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeOff)
	require.ErrorIs(t, h.storage.SetTransitionGate(nil, h.rep), backlogdto.ErrInvalidTransitionRequest)
	require.ErrorIs(t, h.storage.SetTransitionGate(h.sel, nil), backlogdto.ErrInvalidTransitionRequest)
	require.True(t, h.storage.TransitionGateWired(), "a rejected injection leaves the gate as it was")
	require.Same(t, h.sel, h.storage.TransitionGateEngine())
}

func TestGatedTransition_RejectsUnregisteredCallerAndMalformedRequests(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusIdea)
	ctx := context.Background()

	_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: "nobody", ItemID: item.ID, To: domain.BacklogStatusReady})
	require.ErrorIs(t, err, backlogdto.ErrUnregisteredTransitionCaller)
	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, To: domain.BacklogStatusReady})
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID})
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: "other", Observed: item, To: domain.BacklogStatusReady})
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
}

func TestGatedTransition_RecordsGateCallsPerClassCallerAndMode(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeShadow)
	item := h.newItem(t, domain.BacklogStatusIdea)
	before := GateCallsTotal(domain.ClassOperator, domain.CallerOperatorTransitionRPC, ModeShadow)
	_, err := h.storage.GatedTransition(context.Background(), backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusRefining})
	require.NoError(t, err)
	require.Equal(t, before+1, GateCallsTotal(domain.ClassOperator, domain.CallerOperatorTransitionRPC, ModeShadow))
}

// --- exempt entry points ---------------------------------------------------

func TestRollback_RequiresExpectedStatus_AndCountsBypass(t *testing.T) {
	// Not parallel: EngineBypassCount is process-wide, so a parallel test using the same caller would bump it between the two reads.
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, domain.BacklogStatusQueued)

	_, err := h.storage.Rollback(ctx, item.ID, domain.BacklogStatusReady, nil, instance.TriggeredBySystem)
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
	_, err = h.storage.Rollback(ctx, item.ID, domain.BacklogStatusReady, &backlogdto.BacklogItemPrecondition{}, instance.TriggeredBySystem)
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)

	before := EngineBypassCount(backlogdto.CallerRollback)
	_, err = h.storage.Rollback(ctx, item.ID, domain.BacklogStatusReady, &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReady)}, instance.TriggeredBySystem)
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	require.Equal(t, before, EngineBypassCount(backlogdto.CallerRollback), "a lost CAS is not a bypass write")

	_, err = h.storage.Rollback(ctx, item.ID, domain.BacklogStatusReady, &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusQueued)}, instance.TriggeredBySystem)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReady), h.status(t, item.ID))
	require.Equal(t, before+1, EngineBypassCount(backlogdto.CallerRollback))
}

func TestDebugForce_WritesAnyStatusUnchecked_AndCountsBypass(t *testing.T) {
	// Not parallel: EngineBypassCount is process-wide, so a parallel test using the same caller would bump it between the two reads.
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, domain.BacklogStatusIdea)

	before := EngineBypassCount(backlogdto.CallerDebug)
	_, err := h.storage.DebugForce(ctx, item.ID, domain.BacklogStatusDone, instance.TriggeredByUser) // idea->done has no edge
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusDone), h.status(t, item.ID))

	_, err = h.storage.DebugForce(ctx, item.ID, domain.BacklogStatusArchived, instance.TriggeredByUser)
	require.NoError(t, err)
	archived, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt, "archive goes through the archive writer")
	require.Equal(t, before+2, EngineBypassCount(backlogdto.CallerDebug))
}

// --- GatedArchive, GatedUnarchive, GatedSetPRAndTransition -------------------

func TestGatedArchive_ArchivesFromAnyStatus_AndBeforeCommitRunsFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, mode := range []EngineMode{ModeOff, ModeShadow, ModeEnforce} {
		h := newGateHarness(t, mode)
		item := h.newItem(t, domain.BacklogStatusInProgress)
		var statusInHook string
		got, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID, TriggeredBy: instance.TriggeredByUser,
			Precondition: &backlogdto.BacklogItemPrecondition{Note: "operator archived"},
			BeforeCommit: func(context.Context) error { statusInHook = h.status(t, item.ID); return nil },
		})
		require.NoError(t, err, "mode %d", mode)
		require.Equal(t, string(domain.BacklogStatusInProgress), statusInHook, "the hook runs before the write")
		require.Equal(t, string(domain.BacklogStatusArchived), got.Status)
		require.NotNil(t, got.ArchivedAt)
		ev := lastEvent(t, h, item.ID)
		require.Equal(t, string(domain.BacklogStatusInProgress), ev.FromStatus)
		require.NotNil(t, ev.Note)
		require.Equal(t, "operator archived", *ev.Note)
	}
}

func TestGatedArchive_AlreadyArchived_DecidesOnSystemRequiredEdgeAndRewrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, mode := range []EngineMode{ModeOff, ModeShadow, ModeEnforce} {
		h := newGateHarness(t, mode)
		item := h.newItem(t, domain.BacklogStatusReady)
		_, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID, TriggeredBy: instance.TriggeredByUser})
		require.NoError(t, err)

		again, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID, TriggeredBy: instance.TriggeredByUser})
		require.NoError(t, err, "re-archive is allowed in mode %d", mode)
		require.Equal(t, string(domain.BacklogStatusArchived), again.Status)
		require.Equal(t, string(domain.BacklogStatusArchived), lastEvent(t, h, item.ID).FromStatus, "the second write is archived->archived")
	}
}

func TestGatedArchive_BeforeCommitErrorAbortsAndWrongTargetIsRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusReady)
	boom := errors.New("push failed")
	_, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{
		Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID,
		BeforeCommit: func(context.Context) error { return boom },
	})
	require.ErrorIs(t, err, boom)
	require.Equal(t, string(domain.BacklogStatusReady), h.status(t, item.ID), "an aborted archive writes nothing")

	_, err = h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID, To: domain.BacklogStatusDone})
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
}

func TestGatedUnarchive_RestoresToIdea_AndRejectsRequestsTheRawWriterCannotHonor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusReady)
	_, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID})
	require.NoError(t, err)

	at := time.Now()
	_, err = h.storage.GatedUnarchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorUnarchiveRPC, ItemID: item.ID, Precondition: &backlogdto.BacklogItemPrecondition{ExpectedUpdatedAt: &at}})
	require.ErrorIs(t, err, backlogdto.ErrInvalidTransitionRequest)
	require.Equal(t, string(domain.BacklogStatusArchived), h.status(t, item.ID))

	got, err := h.storage.GatedUnarchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorUnarchiveRPC, ItemID: item.ID})
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusIdea), got.Status)
	require.Nil(t, got.ArchivedAt)
}

func TestGatedSetPRAndTransition_RecordsPRInOneWrite_IsIdempotent_AndGuardsReassignment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusReview)
	observed, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	pr := backlogdto.PRAssociation{URL: "https://example.test/o/r/pull/7", Number: 7, Summary: "opened"}
	req := backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: observed}
	require.NoError(t, h.storage.GatedSetPRAndTransition(ctx, req, pr))

	got, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusPRPending), got.Status)
	require.Equal(t, 7, got.PrNumber)
	require.Equal(t, pr.URL, got.PrURL)

	// A retry of the same PR is a no-op success.
	require.NoError(t, h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: got}, pr))

	// Reassigning to another PR is a field write that still needs the guard.
	other := backlogdto.PRAssociation{URL: "https://example.test/o/r/pull/8", Number: 8, Summary: "moved"}
	err = h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: got}, other)
	require.ErrorIs(t, err, backlogdto.ErrPRReassignmentNotAllowed)
	other.Guard = &backlogdto.PRReassignmentGuard{OverrideReason: "polluted branch", NewPRAuthorVerified: true}
	require.NoError(t, h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: got}, other))
	got, err = h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, 8, got.PrNumber)
}

func TestGatedSetPRAndTransition_DeniedByOperatorGate_WritesNothingAndReportsAgentCaller(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	addGateToSeededEdge(t, h.repo.Client, h.cfg, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.GateKindHumanApproval, nil)
	item := h.newItem(t, domain.BacklogStatusReview)
	observed, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	err = h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: observed},
		backlogdto.PRAssociation{URL: "https://example.test/o/r/pull/9", Number: 9, Summary: "opened"})
	require.ErrorIs(t, err, domain.ErrGateNotSatisfied)

	got, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), got.Status)
	require.Zero(t, got.PrNumber, "the PR fields are not written on a denial")
	calls := h.rep.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, domain.CallerMCPReportPRCreated, calls[0].caller)
	require.Equal(t, backlogdto.DeniedOperatorGate, calls[0].reason)
}

func TestGatedSetPRAndTransition_RejectsMissingObservedAndPrecondition(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, domain.BacklogStatusReview)
	pr := backlogdto.PRAssociation{URL: "u", Number: 1}
	require.ErrorIs(t, h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, ItemID: item.ID}, pr), backlogdto.ErrInvalidTransitionRequest)
	observed, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.ErrorIs(t, h.storage.GatedSetPRAndTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPReportPRCreated, Observed: observed, Precondition: &backlogdto.BacklogItemPrecondition{}}, pr), backlogdto.ErrInvalidTransitionRequest)
}

// --- GatedReconcileStuck -----------------------------------------------------

// countingPublisher records ChangeStatusTransition publishes per item.
type countingPublisher struct {
	mu          sync.Mutex
	transitions map[string][]BacklogItemChange
}

func (p *countingPublisher) PublishItemChanged(item *backlogdto.BacklogItemData, change BacklogItemChange) {
	if change.Kind != ChangeStatusTransition {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.transitions == nil {
		p.transitions = map[string][]BacklogItemChange{}
	}
	p.transitions[item.ID] = append(p.transitions[item.ID], change)
}

func (p *countingPublisher) forItem(id string) []BacklogItemChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]BacklogItemChange(nil), p.transitions[id]...)
}

func TestGatedReconcileStuck_PerItemAtomicity_GatedItemStaysAndIsReported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	pub := &countingPublisher{}
	h.storage.SetItemChangePublisher(pub)

	addGateToSeededEdge(t, h.repo.Client, h.cfg, domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.GateKindHumanApproval, nil)
	approved := h.newStuckInProgressItem(t)
	gated := h.newStuckInProgressItem(t)

	edge, ok := h.cfg.cache.Get(domain.BacklogStatusInProgress, domain.BacklogStatusReview)
	require.True(t, ok)
	require.Len(t, edge.Gates, 1)
	_, err := RecordGateApproval(ctx, NewEntGateSatisfactionRepository(h.repo.Client), uuid.MustParse(approved.ID), edge.Gates[0].ID, "operator", true)
	require.NoError(t, err)

	moved, err := h.storage.GatedReconcileStuck(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, moved, "only the item whose gate is satisfied moves")

	got, err := h.storage.GetBacklogItem(ctx, approved.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), got.Status)
	require.Equal(t, "existing notes\n[auto] Transitioned to review: all work sessions ended.", got.Notes)
	ev := lastEvent(t, h, approved.ID)
	require.Equal(t, instance.TriggeredBySystem, ev.TriggeredBy)
	require.Equal(t, string(domain.BacklogStatusInProgress), ev.FromStatus)
	require.Equal(t, string(domain.BacklogStatusReview), ev.ToStatus)
	require.Len(t, got.StatusEvents, 1, "one status event for the moved item")
	require.Equal(t, []BacklogItemChange{{Kind: ChangeStatusTransition, OldStatus: string(domain.BacklogStatusInProgress), NewStatus: string(domain.BacklogStatusReview)}}, pub.forItem(approved.ID),
		"exactly one ChangeStatusTransition publish")

	require.Equal(t, string(domain.BacklogStatusInProgress), h.status(t, gated.ID), "the gated item is not moved")
	require.Empty(t, pub.forItem(gated.ID))
	calls := h.rep.snapshot()
	require.Len(t, calls, 1, "the denial reaches the reporter, which owns the transition_denied stuck row")
	require.Equal(t, gated.ID, calls[0].itemID)
	require.Equal(t, domain.CallerReconcileStuck, calls[0].caller)
	require.Equal(t, backlogdto.DeniedOperatorGate, calls[0].reason)
}

func TestGatedReconcileStuck_SkipsItemsWithARunningSessionAndCountsNothingWhenIdle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)

	moved, err := h.storage.GatedReconcileStuck(ctx)
	require.NoError(t, err)
	require.Zero(t, moved)

	item, err := h.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "running", Status: string(domain.BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = h.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: uuid.NewString(), SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)
	moved, err = h.storage.GatedReconcileStuck(ctx)
	require.NoError(t, err)
	require.Zero(t, moved)
	require.Equal(t, string(domain.BacklogStatusInProgress), h.status(t, item.ID))
}

func TestCreateBacklogItemForDebug_CreatesAtAnyStatus_AndCountsDebugCreate(t *testing.T) {
	// Not parallel: EngineBypassCount is process-wide, so a parallel test using the same caller would bump it between the two reads.
	h := newGateHarness(t, ModeEnforce)
	before := EngineBypassCount(backlogdto.CallerDebugCreate)

	item, err := h.storage.CreateBacklogItemForDebug(context.Background(), backlogdto.BacklogItemData{Title: "seeded", Status: string(domain.BacklogStatusReview)})
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), h.status(t, item.ID))
	require.Equal(t, before+1, EngineBypassCount(backlogdto.CallerDebugCreate))
}

func TestGatedUnarchive_StaleObservedLosesCAS_AndNeverArchivedIsRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeOff)
	item := h.newItem(t, domain.BacklogStatusReady)
	_, err := h.storage.GatedArchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorArchiveRPC, ItemID: item.ID})
	require.NoError(t, err)
	observed, err := h.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	h.moveRaw(t, item.ID, domain.BacklogStatusInProgress)
	_, err = h.storage.GatedUnarchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorUnarchiveRPC, ItemID: item.ID, Observed: observed})
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	require.Equal(t, string(domain.BacklogStatusInProgress), h.status(t, item.ID), "a stale unarchive must not reset a live item")

	_, err = h.storage.GatedUnarchive(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorUnarchiveRPC, ItemID: item.ID})
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	require.Equal(t, string(domain.BacklogStatusInProgress), h.status(t, item.ID))
}
