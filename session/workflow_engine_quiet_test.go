package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

func w21bTotalDenied(h *gateHarness) int64 {
	var n int64
	for _, src := range []workflowengine.DenialSource{workflowengine.DenialConfigGraph, workflowengine.DenialOperatorGate, workflowengine.DenialDefaultGuard} {
		n += h.sel.EnforceDeniedTotal(src)
	}
	return n
}

func TestRedriveAndHolds_should_NotInflateEnforceDeniedCounts_OverManyTicks(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, workflowengine.ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, domain.BacklogStatusIdea, domain.BacklogStatusReady)
	item := h.newItem(t, domain.BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, domain.CallerAutonomousComplete, domain.BacklogStatusIdea, domain.BacklogStatusReady, backlogdto.DeniedOperatorGate)

	for i := 0; i < 5; i++ {
		h.storage.RedriveTransitionDenials(ctx)
		require.True(t, h.storage.TransitionDenialHolds(ctx, item))
	}

	require.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), 1)
	require.Zero(t, w21bTotalDenied(h), "re-decides and probes of an open denial are not new denials")
}

func TestGatedTransition_RealGraphDenialCountsOnce_OperatorOverrideCountsNone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, workflowengine.ModeEnforce)
	item := h.newItem(t, domain.BacklogStatusIdea)

	_, err := h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerMCPResumeWork, ItemID: item.ID, To: domain.BacklogStatusDone})
	require.ErrorIs(t, err, domain.ErrTransitionNotAllowed)
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(workflowengine.DenialConfigGraph))

	_, err = h.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusDone, OverrideReason: "owner says so"})
	require.NoError(t, err)
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(workflowengine.DenialConfigGraph), "an override that frees the graph is not a denial")
}
