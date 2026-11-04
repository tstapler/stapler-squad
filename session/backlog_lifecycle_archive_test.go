package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessDisableGates stands in for an operator satisfying (or removing) every
// configured gate: the next decision finds none.
func sessDisableGates(t *testing.T, h *gateHarness) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.repo.client.TransitionGate.Update().SetEnabled(false).Exec(ctx))
	require.NoError(t, h.cfg.InvalidateCache(ctx))
}

// Story 1.5.7a: under Enforce an operator gate on done->archived holds the item
// at done, the gate reports the denial for the registered caller, the sweep
// raises no notification of its own, and the next tick after the gate is gone
// archives it.
func TestArchiveStaleDoneItems_Enforce_OperatorGateHoldsItemThenArchivesWhenSatisfied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeEnforce)
	addGateToSeededEdge(t, h.repo.client, h.cfg, BacklogStatusDone, BacklogStatusArchived, GateKindHumanApproval, nil)
	item := newDoneTestItem(t, h.storage, h.repo, maxDoneAge+1)

	notifier := &fakeNotifier{}
	listener := NewBacklogLifecycleListener(h.storage)
	listener.SetNotifier(notifier)

	for tick := 1; tick <= 3; tick++ {
		listener.archiveStaleDoneItems(ctx)
		require.Equal(t, string(BacklogStatusDone), h.status(t, item.ID), "tick %d", tick)
	}
	reports := h.rep.snapshot()
	require.Len(t, reports, 3, "the gate reports each denied attempt; the reporter implementation dedupes")
	for _, r := range reports {
		assert.Equal(t, CallerArchiveStaleDone, r.caller)
		assert.Equal(t, DeniedOperatorGate, r.reason)
		assert.Equal(t, BacklogStatusArchived, r.to)
	}
	assert.Empty(t, notifier.calls, "the sweep does not notify on top of the gate's report")

	sessDisableGates(t, h)
	listener.archiveStaleDoneItems(ctx)
	assert.Equal(t, string(BacklogStatusArchived), h.status(t, item.ID))
}

func TestArchiveStaleDoneItems_Shadow_ArchivesDespiteOperatorGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, ModeShadow)
	addGateToSeededEdge(t, h.repo.client, h.cfg, BacklogStatusDone, BacklogStatusArchived, GateKindHumanApproval, nil)
	item := newDoneTestItem(t, h.storage, h.repo, maxDoneAge+1)

	NewBacklogLifecycleListener(h.storage).archiveStaleDoneItems(ctx)

	assert.Equal(t, string(BacklogStatusArchived), h.status(t, item.ID))
	assert.Empty(t, h.rep.snapshot())
}
