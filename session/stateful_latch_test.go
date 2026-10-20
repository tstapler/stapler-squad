package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	latchACDone    = `[{"index":0,"text":"c","status":"done"}]`
	latchACPending = `[{"index":0,"text":"c","status":"pending"}]`
)

func TestEffectiveGateStateful_FieldOrKind(t *testing.T) {
	t.Parallel()
	require.False(t, EffectiveGateStateful(GateKindStructural, false))
	require.True(t, EffectiveGateStateful(GateKindStructural, true))
	for _, k := range []GateKind{GateKindHumanApproval, GateKindAutomatedReview, GateKindCustom} {
		require.True(t, EffectiveGateStateful(k, false), "%s is always stateful", k)
	}
}

func TestShadow_SkipsStatefulStructuralGateWithoutIo_WhenFieldSet(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stateful    bool
		wantSkipped int
	}{{true, 1}, {false, 0}} {
		repo := NewTestEntRepository(t)
		require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), repo.client))
		gateRepo := &failIfCalledGateRepo{t: t}
		cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), gateRepo, nil)
		require.NoError(t, err)
		from, to := newCustomTransitionWithGates(t, repo.client, cfg, []GateKind{GateKindStructural}, []bool{tc.stateful})
		h := newSelectorHarness(t, ModeShadow, cfg)

		_ = h.sel.ValidateGates(BacklogItemTransitionInput{Status: from, ItemID: uuid.NewString(), AcCriteria: latchACDone}, to, nil)

		require.Equal(t, tc.wantSkipped, h.recorder.notCompared[NotComparedStatefulGate], "stateful=%t", tc.stateful)
		require.Zero(t, gateRepo.calls.Load())
	}
}

func TestLatch_StatefulStructuralGateStaysSatisfied_AfterRegression(t *testing.T) {
	t.Parallel()
	for _, stateful := range []bool{true, false} {
		repo := NewTestEntRepository(t)
		require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), repo.client))
		cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), NewEntGateSatisfactionRepository(repo.client), nil)
		require.NoError(t, err)
		from, to := newCustomTransitionWithGates(t, repo.client, cfg, []GateKind{GateKindStructural}, []bool{stateful})
		itemID := uuid.NewString()
		satisfied := BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACDone}
		regressed := BacklogItemTransitionInput{Status: from, ItemID: itemID, AcCriteria: latchACPending}

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
