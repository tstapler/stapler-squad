package prwatch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var catchupT0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type catchupRig struct {
	snaps   *fakePRSnapshot
	steer   *fakeSteerer
	coal    *fakeCoalescer
	metrics *fakeMetrics
	c       *Catchup
}

func newCatchupRig(mode HumanFeedbackMode) *catchupRig {
	r := &catchupRig{
		snaps:   &fakePRSnapshot{snapshots: map[string]PRSnapshot{}},
		steer:   &fakeSteerer{},
		coal:    &fakeCoalescer{},
		metrics: &fakeMetrics{},
	}
	r.c = NewCatchup(CatchupDeps{
		Snapshots: r.snaps, Steerer: r.steer, Coalescer: r.coal, Metrics: r.metrics,
		Log: &fakeLog{}, Settings: Settings{HumanFeedbackMode: mode},
	})
	return r
}

func (r *catchupRig) set(uuid string, s PRSnapshot) {
	r.snaps.mu.Lock()
	defer r.snaps.mu.Unlock()
	r.snaps.snapshots[uuid] = s
}

func liveRef(uuid string) SessionRef {
	return SessionRef{UUID: uuid, Repo: "acme/widget", PR: 5, State: StateLive}
}

func TestCatchUp_should_SteerOnce_When_FirstSubscribeAndSnapshotShowsFailingCI(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	r.c.Tick(context.Background(), []SessionRef{liveRef("a")})

	reqs := r.steer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, []Kind{KindCI}, reqs[0].Kinds)
	want, err := render([]Kind{KindCI}, "acme/widget", 5)
	require.NoError(t, err)
	assert.Equal(t, want, reqs[0].Text)
	require.Len(t, r.coal.steers, 1)
	assert.Equal(t, ClassCI, r.coal.steers[0].Class)
}

func TestCatchUp_should_SteerOnce_When_FirstSubscribeAndChangesRequested(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "success", ChangesRequested: 1, CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))

	reqs := r.steer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, []Kind{KindReview}, reqs[0].Kinds)
}

func TestCatchUp_should_NotSteer_When_SnapshotHealthyOrPRAlreadySubscribed(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "success", CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	assert.Empty(t, r.steer.Requests())

	// A later reconcile of an already-subscribed PR never calls Subscribed;
	// Tick alone must not steer even with a failing snapshot.
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0.Add(time.Minute)})
	r.c.Tick(context.Background(), []SessionRef{liveRef("a")})
	assert.Empty(t, r.steer.Requests())

	// Only failure steers.
	for _, v := range []string{"", "pending", "neutral"} {
		rr := newCatchupRig(ModeAll)
		rr.set("b", PRSnapshot{CheckConclusion: v, CheckedAt: catchupT0})
		rr.c.Subscribed(context.Background(), liveRef("b"))
		assert.Empty(t, rr.steer.Requests(), v)
	}
}

func TestCatchUp_should_WaitForPoll_When_SnapshotNeverPolledAfterRestart(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{}) // not persisted: empty and never polled
	r.c.Subscribed(context.Background(), liveRef("a"))
	assert.Empty(t, r.steer.Requests())

	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.c.Tick(context.Background(), []SessionRef{liveRef("a")})
	assert.Len(t, r.steer.Requests(), 1)
}

func TestCatchUp_should_RunOnceAfterFreshSnapshot_When_SuspendedSessionResumes(t *testing.T) {
	r := newCatchupRig(ModeAll)
	resumed := catchupT0
	ref := liveRef("a")
	ref.ResumedAt = resumed
	// Snapshot predates the resume: not trusted yet.
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: resumed.Add(-time.Minute)})
	r.c.Tick(context.Background(), []SessionRef{ref})
	assert.Empty(t, r.steer.Requests())

	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: resumed.Add(time.Minute)})
	r.c.Tick(context.Background(), []SessionRef{ref})
	r.c.Tick(context.Background(), []SessionRef{ref})
	assert.Len(t, r.steer.Requests(), 1, "once per resume")

	// Healthy after a second resume: re-armed, but no steer.
	ref2 := ref
	ref2.ResumedAt = resumed.Add(time.Hour)
	r.set("a", PRSnapshot{CheckConclusion: "success", CheckedAt: ref2.ResumedAt.Add(time.Minute)})
	r.c.Tick(context.Background(), []SessionRef{ref2})
	assert.Len(t, r.steer.Requests(), 1)

	// A third resume with failing CI steers again.
	ref3 := ref
	ref3.ResumedAt = resumed.Add(2 * time.Hour)
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: ref3.ResumedAt.Add(time.Minute)})
	r.c.Tick(context.Background(), []SessionRef{ref3})
	assert.Len(t, r.steer.Requests(), 2)
}

func TestCatchUp_should_SkipChangesRequestedHalf_When_HumanFeedbackModeIsNotAll(t *testing.T) {
	for _, mode := range []HumanFeedbackMode{ModeAllowlist, ModeOff} {
		r := newCatchupRig(mode)
		r.set("a", PRSnapshot{CheckConclusion: "failure", ChangesRequested: 3, CheckedAt: catchupT0})
		r.c.Subscribed(context.Background(), liveRef("a"))
		reqs := r.steer.Requests()
		require.Len(t, reqs, 1, string(mode))
		assert.Equal(t, []Kind{KindCI}, reqs[0].Kinds, string(mode))

		r2 := newCatchupRig(mode)
		r2.set("b", PRSnapshot{ChangesRequested: 3, CheckedAt: catchupT0})
		r2.c.Subscribed(context.Background(), liveRef("b"))
		assert.Empty(t, r2.steer.Requests(), string(mode))
	}

	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "failure", ChangesRequested: 3, CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	require.Len(t, r.steer.Requests(), 1)
	assert.ElementsMatch(t, []Kind{KindCI, KindReview}, r.steer.Requests()[0].Kinds)
}

func TestCatchUp_should_BeSkipped_When_FirstSubscriptionFollowsAPRPendingExclusion(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.set("b", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	excluded := liveRef("a")
	excluded.SkipReason = SkipBacklogPRPending
	r.c.NoteExcluded([]SessionRef{excluded})

	r.c.Subscribed(context.Background(), liveRef("a"))
	r.c.Subscribed(context.Background(), liveRef("b"))
	reqs := r.steer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "b", reqs[0].SessionUUID)
}

func TestCatchUp_should_NotSteer_When_CoalescerWindowIsOpenOrOutcomeIsNotDelivered(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.coal.actions = map[CoalesceKey]CoalesceAction{
		{SessionUUID: "a", Repo: "acme/widget", PR: 5, Class: ClassCI}: CoalesceHold,
	}
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	assert.Empty(t, r.steer.Requests())

	// Busy: stays armed and retries; no coalescer record until delivered.
	b := newCatchupRig(ModeAll)
	b.steer.outcome = map[string]Outcome{"a": OutcomeBusy}
	b.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	b.c.Subscribed(context.Background(), liveRef("a"))
	assert.Empty(t, b.coal.steers)
	b.steer.mu.Lock()
	b.steer.outcome = nil
	b.steer.mu.Unlock()
	b.c.Tick(context.Background(), []SessionRef{liveRef("a")})
	assert.Len(t, b.steer.Requests(), 2)
	assert.Len(t, b.coal.steers, 1)
}

func TestCatchUp_should_CountCatchUpSteersAgainstFirstSubscriptions_When_Subscribing(t *testing.T) {
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.set("b", PRSnapshot{CheckConclusion: "success", CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	r.c.Subscribed(context.Background(), liveRef("b"))

	assert.Equal(t, CatchupCounts{FirstSubscriptions: 2, FirstSubscriptionSteer: 1}, r.c.Counts())
	assert.Equal(t, 1, r.metrics.Count(MetricCatchupSteers))
}

func TestCatchUp_should_ExportShareAsInformationalOnly_When_CatchUpSteersAreReported(t *testing.T) {
	// The share is exposed only as raw counters; there is no threshold, ratio
	// or alert in the API (requirements A3).
	r := newCatchupRig(ModeAll)
	r.set("a", PRSnapshot{CheckConclusion: "failure", CheckedAt: catchupT0})
	r.c.Subscribed(context.Background(), liveRef("a"))
	counts := r.c.Counts()
	assert.Equal(t, 1, counts.FirstSubscriptions)
	assert.Equal(t, 1, counts.FirstSubscriptionSteer)
	assert.Equal(t, 0, counts.ResumeSteers)
}
