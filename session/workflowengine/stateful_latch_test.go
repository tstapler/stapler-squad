package workflowengine

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/transitiongate"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

const (
	latchACDone    = `[{"index":0,"text":"c","status":"done"}]`
	latchACPending = `[{"index":0,"text":"c","status":"pending"}]`
)

func TestEffectiveGateStateful_FieldOrKind(t *testing.T) {
	t.Parallel()
	require.False(t, EffectiveGateStateful(domain.GateKindStructural, false))
	require.True(t, EffectiveGateStateful(domain.GateKindStructural, true))
	for _, k := range []domain.GateKind{domain.GateKindHumanApproval, domain.GateKindAutomatedReview, domain.GateKindCustom} {
		require.True(t, EffectiveGateStateful(k, false), "%s is always stateful", k)
	}
}

func TestShadow_SkipsStatefulStructuralGateWithoutIo_WhenFieldSet(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stateful    bool
		wantSkipped int
	}{{true, 1}, {false, 0}} {
		entClient := entdbtest.NewClient(t)
		require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), entClient))
		gateRepo := &failIfCalledGateRepo{t: t}
		cfg, err := NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(entClient), gateRepo, nil)
		require.NoError(t, err)
		from, to := newCustomTransitionWithGates(t, entClient, cfg, []domain.GateKind{domain.GateKindStructural}, []bool{tc.stateful})
		h := newSelectorHarness(t, ModeShadow, cfg)

		_ = h.sel.ValidateGates(domain.BacklogItemTransitionInput{Status: from, ItemID: uuid.NewString(), AcCriteria: latchACDone}, to, nil)

		require.Equal(t, tc.wantSkipped, h.recorder.notCompared[NotComparedStatefulGate], "stateful=%t", tc.stateful)
		require.Zero(t, gateRepo.calls.Load())
	}
}

func TestLatch_StatefulStructuralGateStaysSatisfied_AfterRegression(t *testing.T) {
	t.Parallel()
	for _, stateful := range []bool{true, false} {
		entClient := entdbtest.NewClient(t)
		require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), entClient))
		cfg, err := NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(entClient), stagecfg.NewEntGateSatisfactionRepository(entClient), nil)
		require.NoError(t, err)
		from, to := newCustomTransitionWithGates(t, entClient, cfg, []domain.GateKind{domain.GateKindStructural}, []bool{stateful})
		itemID := uuid.NewString()
		satisfied := domain.BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACDone}
		regressed := domain.BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACPending}

		require.NoError(t, cfg.ValidateGates(satisfied, to, nil))
		require.Error(t, cfg.ValidateGates(regressed, to, nil), "read-only evaluation never latches")

		cfg.RecordLatchesAfterCommit(satisfied, to)
		cfg.RecordLatchesAfterCommit(satisfied, to) // idempotent

		err = cfg.ValidateGates(regressed, to, nil)
		if stateful {
			require.NoError(t, err, "latched gate answers from its record")
		} else {
			require.Error(t, err, "stateless gate recomputes")
		}
	}
}

func TestClearLatches_RemovesOnlyStatefulStructuralLatchesOfThatItem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	entClient := entdbtest.NewClient(t)
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, entClient))
	gateRepo := stagecfg.NewEntGateSatisfactionRepository(entClient)
	cfg, err := NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(entClient), gateRepo, nil)
	require.NoError(t, err)
	from, to := newCustomTransitionWithGates(t, entClient, cfg,
		[]domain.GateKind{domain.GateKindStructural, domain.GateKindStructural, domain.GateKindHumanApproval}, []bool{true, false, true})
	gateID := func(kind domain.GateKind, stateful bool) uuid.UUID {
		g, err := entClient.TransitionGate.Query().Where(transitiongate.Kind(string(kind)), transitiongate.Stateful(stateful)).Only(ctx)
		require.NoError(t, err)
		return g.ID
	}
	latchGate, plainGate, approvalGate := gateID(domain.GateKindStructural, true), gateID(domain.GateKindStructural, false), gateID(domain.GateKindHumanApproval, true)
	has := func(item string, gate uuid.UUID) bool {
		_, err := gateRepo.GetByItemAndGate(ctx, uuid.MustParse(item), gate)
		if err != nil {
			require.ErrorIs(t, err, domain.ErrNotFound)
		}
		return err == nil
	}
	itemA, itemB := uuid.NewString(), uuid.NewString()
	for _, id := range []string{itemA, itemB} {
		cfg.RecordLatchesAfterCommit(domain.BacklogItemTransitionInput{Status: from, ItemID: id, AcCriteria: latchACDone}, to)
	}
	for _, g := range []uuid.UUID{plainGate, approvalGate} {
		_, err := gateRepo.Create(ctx, stagecfg.GateSatisfactionCreateInput{ItemID: uuid.MustParse(itemA), GateID: g, Satisfied: true})
		require.NoError(t, err)
	}
	require.True(t, has(itemA, latchGate), "precondition: item A latched")
	require.True(t, has(itemB, latchGate), "precondition: item B latched")

	cfg.ClearLatches(itemA, from, to)

	require.False(t, has(itemA, latchGate), "the stateful structural latch of the item is cleared")
	require.True(t, has(itemA, plainGate), "a stateless structural gate's record survives")
	require.True(t, has(itemA, approvalGate), "a non-structural gate's record survives")
	require.True(t, has(itemB, latchGate), "another item's latch survives")
}
