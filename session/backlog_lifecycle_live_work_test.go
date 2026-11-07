package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveWorkListener builds a listener with reader:backlog on, an injected clock and
// the given probe, over an item whose only PR (#21) is merged in the rows.
func liveWorkListener(t *testing.T, probe LiveWorkProbe, clock *time.Time, reader bool) (*BacklogLifecycleListener, *itemPRFixture, *fakeNotifier) {
	t.Helper()
	f := newItemPRFixture(t, 21)
	f.link(ItemPROwner(f.itemID), 21, PRStateMerged)
	newTrackedWorkSession(t, f.storage, f.itemID, "/tmp/fake-repo", "backlog/live-work", "")
	l := NewBacklogLifecycleListener(f.storage)
	l.backlogReaderOverride = func() bool { return reader }
	l.liveWork.clock = func() time.Time { return *clock }
	if probe != nil {
		l.SetLiveWorkProbe(probe)
	}
	n := &fakeNotifier{}
	l.SetNotifier(n)
	overridePRPendingChecker(t, l, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(l, "backlog/live-work")
	return l, f, n
}

func statusOf(t *testing.T, f *itemPRFixture) string {
	t.Helper()
	got, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)
	return got.Status
}

func TestReconcilePRPending_ShouldHoldThenReleaseDone_WhenMergedAndWorkSessionLive(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	probe := &stubLiveAll{live: true}
	l, f, notes := liveWorkListener(t, probe, &now, true)
	ctx := context.Background()

	l.ReconcilePRPending(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusPRPending), statusOf(t, f), "merged, session still active: stays pr_pending")
	assert.Empty(t, notes.calls, "no notification while held")

	probe.live = false
	l.ReconcilePRPending(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusDone), statusOf(t, f), "session idle or ended: the next pass completes the item")
}

func TestReconcilePRPending_ShouldHoldOnProbeError_AndProceedToDoneWithOneWarning_WhenPastMaxHold(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	probe := &stubLiveAll{err: assert.AnError}
	l, f, notes := liveWorkListener(t, probe, &now, true)
	ctx := context.Background()
	before := LiveWorkResultCount(LiveWorkResultMaxHold)

	l.ReconcilePRPending(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusPRPending), statusOf(t, f), "a probe error holds")

	now = prTestEpoch.Add(59 * time.Minute)
	l.ReconcilePRPending(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusPRPending), statusOf(t, f), "59 minutes after first observation: still held")

	now = prTestEpoch.Add(61 * time.Minute)
	l.ReconcilePRPending(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusDone), statusOf(t, f), "expiry wins over the probe error")
	require.Len(t, notes.calls, 1, "exactly one max-hold notification")
	assert.Equal(t, int64(1), LiveWorkResultCount(LiveWorkResultMaxHold)-before)
}

func TestReconcilePRPending_ShouldTrustRowsNotTheCheckerObservation_WhenAnotherPROpen(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	l, f, _ := liveWorkListener(t, &stubLiveAll{}, &now, true)
	f.link(ItemPROwner(f.itemID), 22, PRStateOpen)

	l.ReconcilePRPending(context.Background(), f.repo)
	assert.Equal(t, string(BacklogStatusPRPending), statusOf(t, f), "verdict is Open while #22 is open")
}

func TestReconcilePRPending_ShouldRefreshRowBeforeReadingVerdict_WhenRefresherWired(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	f := newItemPRFixture(t, 21)
	f.link(ItemPROwner(f.itemID), 21, PRStateOpen) // row lags GitHub
	newTrackedWorkSession(t, f.storage, f.itemID, "/tmp/fake-repo", "backlog/live-work", "")
	l := NewBacklogLifecycleListener(f.storage)
	l.backlogReaderOverride = func() bool { return true }
	l.liveWork.clock = func() time.Time { return now }
	overridePRPendingChecker(t, l, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(l, "backlog/live-work")
	var refreshed atomic.Int32
	l.SetPRRowRefresher(refresherFunc(func(ctx context.Context, ref PRRef, src PRObservationSource) error {
		refreshed.Add(1)
		assert.Equal(t, PRSourceReconcilerRefresh, src)
		_, err := f.repo.UpdatePullRequestState(ctx, ref, PRObservation{Source: src, Kind: PRObservationFetch, ObservedAt: now, Now: now, State: PRStateMerged, MergedAt: now})
		return err
	}))

	l.ReconcilePRPending(context.Background(), f.repo)
	assert.EqualValues(t, 1, refreshed.Load())
	assert.Equal(t, string(BacklogStatusDone), statusOf(t, f), "the awaited refresh wrote the row before the verdict")
}

func TestReconcilePRPending_ShouldBehaveAsToday_WhenFlagOffOrProbeNil(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	for name, tc := range map[string]struct {
		probe  LiveWorkProbe
		reader bool
	}{
		"reader off, probe wired": {&stubLiveAll{live: true}, false},
		"reader on, probe nil":    {nil, true},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l, f, notes := liveWorkListener(t, tc.probe, &now, tc.reader)
			l.ReconcilePRPending(context.Background(), f.repo)
			assert.Equal(t, string(BacklogStatusDone), statusOf(t, f))
			assert.Empty(t, notes.calls)
		})
	}
}

func TestReconcileBouncingItems_ShouldHoldDoneTransition_WhenMergedAndWorkSessionLive(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	probe := &stubLiveAll{live: true}
	l, f, _ := liveWorkListener(t, probe, &now, true)
	ctx := context.Background()
	_, err := f.repo.client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(BacklogStatusInProgress)).Save(ctx)
	require.NoError(t, err)
	checker := &fakePRPendingChecker{merged: true}
	overridePRPendingChecker(t, l, checker)

	l.reconcileBouncingItems(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusInProgress), statusOf(t, f), "live session: no done transition")
	assert.Zero(t, checker.isMergedCalls, "rows already say Done and a session is live: no GitHub call (C4)")

	probe.live = false
	l.reconcileBouncingItems(ctx, f.repo)
	assert.Equal(t, string(BacklogStatusDone), statusOf(t, f), "idle or ended: it moves to done as today")
}

type stubLiveAll struct {
	live bool
	err  error
}

func (s *stubLiveAll) IsLive(context.Context, string, time.Time) (bool, error) { return s.live, s.err }

type refresherFunc func(context.Context, PRRef, PRObservationSource) error

func (f refresherFunc) Refresh(ctx context.Context, ref PRRef, src PRObservationSource) error {
	return f(ctx, ref, src)
}
