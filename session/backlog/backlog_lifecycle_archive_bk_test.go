package backlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// Story 1.5.7a: under Enforce an operator gate on done->archived holds the item
// at done, the gate reports the denial for the registered caller, the sweep
// raises no notification of its own, and the next tick after the gate is gone
// archives it.
func TestArchiveStaleDoneItems_Enforce_OperatorGateHoldsItemThenArchivesWhenSatisfied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, session.ModeEnforce)
	addGateToSeededEdge(t, h.repo.Client, h.cfg, session.BacklogStatusDone, session.BacklogStatusArchived, session.GateKindHumanApproval, nil)
	item := newDoneTestItem(t, h.storage, h.repo, maxDoneAge+1)

	notifier := &sessiontest.FakeNotifier{}
	listener := NewBacklogLifecycleListener(h.storage)
	listener.SetNotifier(notifier)

	for tick := 1; tick <= 3; tick++ {
		listener.archiveStaleDoneItems(ctx)
		require.Equal(t, string(session.BacklogStatusDone), h.status(t, item.ID), "tick %d", tick)
	}
	reports := h.rep.snapshot()
	require.Len(t, reports, 3, "the gate reports each denied attempt; the reporter implementation dedupes")
	for _, r := range reports {
		assert.Equal(t, session.CallerArchiveStaleDone, r.caller)
		assert.Equal(t, session.DeniedOperatorGate, r.reason)
		assert.Equal(t, session.BacklogStatusArchived, r.to)
	}
	assert.Empty(t, notifier.Calls, "the sweep does not notify on top of the gate's report")

	sessDisableGates(t, h)
	listener.archiveStaleDoneItems(ctx)
	assert.Equal(t, string(session.BacklogStatusArchived), h.status(t, item.ID))
}

func TestArchiveStaleDoneItems_Shadow_ArchivesDespiteOperatorGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, session.ModeShadow)
	addGateToSeededEdge(t, h.repo.Client, h.cfg, session.BacklogStatusDone, session.BacklogStatusArchived, session.GateKindHumanApproval, nil)
	item := newDoneTestItem(t, h.storage, h.repo, maxDoneAge+1)

	NewBacklogLifecycleListener(h.storage).archiveStaleDoneItems(ctx)

	assert.Equal(t, string(session.BacklogStatusArchived), h.status(t, item.ID))
	assert.Empty(t, h.rep.snapshot())
}
