package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func w21bTotalDenied(h *gateHarness) int64 {
	var n int64
	for _, src := range []DenialSource{DenialConfigGraph, DenialOperatorGate, DenialDefaultGuard} {
		n += h.sel.EnforceDeniedTotal(src)
	}
	return n
}

func TestRedriveAndHolds_should_NotInflateEnforceDeniedCounts_OverManyTicks(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

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
	h := newGateHarness(t, ModeEnforce)
	item := h.newItem(t, BacklogStatusIdea)

	_, err := h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerMCPResumeWork, ItemID: item.ID, To: BacklogStatusDone})
	require.ErrorIs(t, err, ErrTransitionNotAllowed)
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(DenialConfigGraph))

	_, err = h.storage.GatedTransition(ctx, TransitionRequest{Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusDone, OverrideReason: "owner says so"})
	require.NoError(t, err)
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(DenialConfigGraph), "an override that frees the graph is not a denial")
}
