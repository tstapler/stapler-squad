package workflowengine

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
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

func TestClearLatches_RemovesStatefulLatch_SoRegressedGateFailsAgain(t *testing.T) {
	t.Parallel()
	entClient := entdbtest.NewClient(t)
	require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), entClient))
	cfg, err := NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(entClient), stagecfg.NewEntGateSatisfactionRepository(entClient), nil)
	require.NoError(t, err)
	from, to := newCustomTransitionWithGates(t, entClient, cfg, []domain.GateKind{domain.GateKindStructural}, []bool{true})
	itemID := uuid.NewString()
	satisfied := domain.BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACDone}
	regressed := domain.BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACPending}

	cfg.RecordLatchesAfterCommit(satisfied, to)
	require.NoError(t, cfg.ValidateGates(regressed, to, nil), "latched")

	cfg.ClearLatches(itemID, from, to)

	require.Error(t, cfg.ValidateGates(regressed, to, nil), "cleared latch no longer answers satisfied")
	require.NoError(t, cfg.ValidateGates(satisfied, to, nil), "the live check still passes")
}
