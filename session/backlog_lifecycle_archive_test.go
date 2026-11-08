package session

import (
	"context"
	"testing"
	"time"

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

// ruleLSweepFixture is a done item with one work session, a recording archiver, the
// real settledTracker and an injected sweep clock.
func ruleLSweepFixture(t *testing.T, probe LiveWorkProbe, reader bool) (*BacklogLifecycleListener, *fakeSessionArchiver, *itemPRFixture, *time.Time) {
	t.Helper()
	f := newItemPRFixture(t, 0)
	_, err := f.repo.client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(BacklogStatusDone)).Save(context.Background())
	require.NoError(t, err)
	f.session(SessionRoleWork)
	now := prTestEpoch
	l := NewBacklogLifecycleListener(f.storage)
	l.backlogReaderOverride = func() bool { return reader }
	l.sweepClock = func() time.Time { return now }
	if probe != nil {
		l.SetLiveWorkProbe(probe)
	}
	archiver := &fakeSessionArchiver{}
	l.SetSessionArchiver(archiver)
	return l, archiver, f, &now
}

// Not parallel: it reads the process-wide RULE L gauge, which a parallel sweep would reset.
func TestReconcileTerminalItemSessions_ShouldNotSettleItem_WhenLiveSessionSkipped_AndArchiveOnNextPassNotAfterTTL(t *testing.T) {
	ctx := context.Background()
	probe := &stubLiveAll{live: true}
	l, archiver, f, now := ruleLSweepFixture(t, probe, true)
	before := LiveWorkResultCount(LiveWorkResultRuleLSkip)

	l.reconcileTerminalItemSessions(ctx)
	assert.Empty(t, archiver.archivedUUIDs, "RULE L: a live session is not archived")
	assert.Empty(t, archiver.killedUUIDs, "nor is its pane killed")
	assert.Equal(t, 1, RuleLUnsettledItems(), "the unsettled gauge counts the item")
	assert.Greater(t, LiveWorkResultCount(LiveWorkResultRuleLSkip), before)

	*now = now.Add(time.Minute) // well inside terminalSweepSettleTTL
	probe.live = false
	l.reconcileTerminalItemSessions(ctx)
	assert.Len(t, archiver.archivedUUIDs, 1, "the first pass after idle archives it, not the pass after the TTL")
	assert.Len(t, archiver.killedUUIDs, 1)
	assert.Zero(t, RuleLUnsettledItems())

	l.reconcileTerminalItemSessions(ctx)
	assert.Len(t, archiver.archivedUUIDs, 1, "settled now: the item is skipped")
	_ = f
}

func TestReconcileTerminalItemSessions_ShouldArchiveAsToday_WhenScopeOffOrProbeNil_OrProbeErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		probe     LiveWorkProbe
		reader    bool
		wantEmpty bool
	}{
		"scope off":   {&stubLiveAll{live: true}, false, false},
		"probe nil":   {nil, true, false},
		"probe error": {&stubLiveAll{err: assert.AnError}, true, true},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l, archiver, _, _ := ruleLSweepFixture(t, tc.probe, tc.reader)
			l.reconcileTerminalItemSessions(ctx)
			assert.Equal(t, tc.wantEmpty, len(archiver.archivedUUIDs) == 0)
		})
	}
}

func TestReconcileTerminalItemWorktrees_ShouldSettleOnPRBlock_ButRetryEveryPassOnLiveOnlyBlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		outcome     CleanupOutcome
		wantCalls   int
		secondCalls int
	}{
		"done settles":            {CleanupDoneOutcome(), 1, 1},
		"pr block settles":        {CleanupOutcome{Status: CleanupBlocked, Reason: "open"}, 1, 1},
		"failed settles for TTL":  {CleanupOutcome{Status: CleanupFailed}, 1, 1},
		"live-only never settles": {CleanupOutcome{Status: CleanupBlocked, LiveOnly: true}, 1, 2},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l, _, _, now := ruleLSweepFixture(t, nil, false)
			cleaner := &fakeWorktreeCleaner{outcome: tc.outcome}
			l.SetWorktreeCleaner(cleaner)
			l.reconcileTerminalItemWorktrees(ctx)
			assert.Len(t, cleaner.calls(), tc.wantCalls)
			*now = now.Add(time.Minute)
			l.reconcileTerminalItemWorktrees(ctx)
			assert.Len(t, cleaner.calls(), tc.secondCalls)
		})
	}
}
