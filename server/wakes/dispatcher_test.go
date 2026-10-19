package wakes

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type fakeBacklog struct {
	items map[string]ItemState
}

func (b *fakeBacklog) ItemState(_ context.Context, id string) (ItemState, error) {
	return b.items[id], nil
}

type fixedAdmission struct {
	allow atomic.Bool
	calls atomic.Int32
}

func (a *fixedAdmission) Admit(context.Context) (bool, error) {
	a.calls.Add(1)
	return a.allow.Load(), nil
}

type dispatcherRig struct {
	t         *testing.T
	repo      *session.EntRepository
	clock     *FakeClock
	steerer   *fakeSteerer
	owners    *fakeOwners
	notifier  *recordingNotifier
	bus       *events.EventBus
	backlog   *fakeBacklog
	admission *fixedAdmission
	d         *Dispatcher
}

func newDispatcherRig(t *testing.T, mutate ...func(*Config)) *dispatcherRig {
	t.Helper()
	r := &dispatcherRig{
		t: t, repo: session.NewTestEntRepository(t), clock: NewFakeClock(t0),
		steerer: newFakeSteerer(), owners: newFakeOwners(), notifier: &recordingNotifier{},
		bus: events.NewEventBus(64), backlog: &fakeBacklog{items: map[string]ItemState{}},
		admission: &fixedAdmission{},
	}
	r.steerer.setReady("owner-1", true)
	r.d = r.newDispatcher(mutate...)
	return r
}

// newDispatcher builds another dispatcher over the same database, as a restart does.
func (r *dispatcherRig) newDispatcher(mutate ...func(*Config)) *Dispatcher {
	cfg := Config{
		Store: r.repo, Clock: r.clock, Steerer: r.steerer, Admission: r.admission, Owners: r.owners,
		Notifier: r.notifier, Events: r.bus, Backlog: r.backlog,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	d := NewDispatcher(cfg)
	r.repo.SetWakeChangeHook(d.ChangeHook())
	return d
}

func (r *dispatcherRig) start(d *Dispatcher) {
	r.t.Helper()
	require.NoError(r.t, d.Start(context.Background()))
	r.t.Cleanup(func() { _ = d.Stop(context.Background()) })
}

func (r *dispatcherRig) status(id string) domain.WakeStatus { return statusOf(r.t, r.repo, id) }

func (r *dispatcherRig) eventually(cond func() bool) {
	r.t.Helper()
	require.Eventually(r.t, cond, 5*time.Second, 5*time.Millisecond)
}

func backlogRow(t *testing.T, r *dispatcherRig, item, kind string, o rowOpts) *session.WakeTriggerData {
	t.Helper()
	o.spec = domain.BacklogEventSpec{ItemID: item, Kind: kind}
	o.nextFire = nil
	return newRow(t, r.repo, o)
}

func verdictEvent(item string) *events.Event {
	return &events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeVerdictRecorded, Item: &session.BacklogItemData{ID: item},
		},
	}
}

const itemX = "b608ab1e-0000-4000-8000-000000000001"

func TestDispatcher_should_DeliverOnce_When_EventPublishedBetweenSubscribeAndFirstRead(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	row := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{})
	r.d.testBeforeRead = func() { r.bus.Publish(verdictEvent(itemX)) }

	r.start(r.d)

	r.eventually(func() bool { return r.status(row.ID) == domain.WakeStatusDelivered })
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestDispatcher_should_DeliverOnce_When_EventAndReconcilePredicateBothSeeTheTrigger(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}
	row := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{})
	r.d.testBeforeRead = func() { r.bus.Publish(verdictEvent(itemX)) }

	r.start(r.d)

	r.eventually(func() bool { return r.status(row.ID) == domain.WakeStatusDelivered })
	r.d.WaitIdle()
	assert.Len(t, r.steerer.pane.written(), 1, "the guarded claim lets exactly one path win")
}

func TestDispatcher_should_ReportReadyOnlyAfterStartupReconcile(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	select {
	case <-r.d.Ready():
		t.Fatal("ready before Start")
	default:
	}
	r.start(r.d)
	select {
	case <-r.d.Ready():
	default:
		t.Fatal("not ready after Start returned")
	}
}

func TestDispatcher_should_CoalesceTenEvents_When_SameKeyWithinWindow(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	row := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{maxFires: 5})
	r.start(r.d)

	for range 10 {
		r.bus.Publish(verdictEvent(itemX))
	}
	r.eventually(func() bool { return len(r.steerer.pane.written()) >= 1 })
	r.eventually(func() bool { return r.d.Delivery().Len() == 0 })

	assert.Len(t, r.steerer.pane.written(), 1)
	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
	require.NoError(t, err)
	assert.Equal(t, 1, got.FireCount)
}

func TestDispatcher_should_ReconcileFourRows_When_RestartedOverSameDatabase(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	ctx := context.Background()
	past := t0.Add(-30 * time.Minute)
	expPast := t0.Add(-10 * time.Minute)
	expFuture := t0.Add(time.Hour)

	overdue := newRow(t, r.repo, rowOpts{owner: "owner-1", payload: "overdue", spec: domain.AtSpec{At: past}, nextFire: &past, expiresAt: &expFuture})
	expired := newRow(t, r.repo, rowOpts{owner: "owner-2", payload: "expired", spec: domain.AtSpec{At: past}, nextFire: &past, expiresAt: &expPast})
	event := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{owner: "owner-1", payload: "event"})
	r.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}
	stale := claimedRow(t, r.repo, t0.Add(-3*time.Minute), rowOpts{owner: "owner-1", payload: "stale claim"})
	r.steerer.setReady("owner-2", true)

	restarted := r.newDispatcher()
	restarted.reconcileStartup(ctx)

	m := restarted.Metrics()
	assert.EqualValues(t, 3, m.Value(MetricTrigger, "at", "delivered")+m.Value(MetricTrigger, "backlog_event", "delivered"))
	assert.EqualValues(t, 1, m.Value(MetricTrigger, "at", "missed"))
	assert.EqualValues(t, 1, m.Value(MetricReconcile, ReconcileRequeuedClaimed))
	for _, id := range []string{overdue.ID, event.ID, stale.ID} {
		assert.Equal(t, domain.WakeStatusDelivered, r.status(id))
	}
	assert.Equal(t, domain.WakeStatusMissed, r.status(expired.ID))
	assert.Len(t, r.steerer.pane.written(), 3)
	assert.Len(t, r.notifier.all(), 1, "the missed row is reported once, as a digest")

	before := len(r.steerer.pane.written())
	restarted.reconcileStartup(ctx)
	assert.Len(t, r.steerer.pane.written(), before, "a second reconcile changes nothing")
	assert.EqualValues(t, 1, m.Value(MetricReconcile, ReconcileRequeuedClaimed))
}

func TestDispatcher_should_NotRequeueHeldClaim_When_PeriodicReconcileRunsButDoRequeueAfterRestart(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.steerer.setReady("owner-1", false) // owner busy
	r.start(r.d)
	row := claimedRow(t, r.repo, t0, rowOpts{})
	r.d.dispatchClaimed(context.Background(), row)
	require.True(t, r.d.Delivery().Holds(row.ID))

	r.clock.Advance(3 * time.Minute) // past the 2 minute lease
	r.d.Reconcile(context.Background())

	assert.EqualValues(t, 0, r.d.Metrics().Value(MetricReconcile, ReconcileRequeuedClaimed))
	assert.True(t, r.d.Delivery().Holds(row.ID))
	assert.Empty(t, r.steerer.pane.written())

	r.steerer.setReady("owner-1", true)
	r.d.Delivery().AttemptAll(context.Background())
	assert.Len(t, r.steerer.pane.written(), 1, "exactly one delivery once the owner idles")

	// A row left claimed by a previous process is re-queued by the next one.
	r2 := newDispatcherRig(t)
	r2.steerer.setReady("owner-1", true)
	left := claimedRow(t, r2.repo, t0, rowOpts{})
	r2.clock.Advance(3 * time.Minute)
	restarted := r2.newDispatcher()
	restarted.reconcileStartup(context.Background())
	assert.EqualValues(t, 1, restarted.Metrics().Value(MetricReconcile, ReconcileRequeuedClaimed))
	assert.Equal(t, domain.WakeStatusDelivered, r2.status(left.ID))
}

func TestDispatcher_should_ReportStaleClaimWithoutResending_When_ClaimedThisRunAndNotHeld(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.start(r.d)
	row := claimedRow(t, r.repo, r.clock.Now(), rowOpts{}) // claimed by this process, not in the cache
	r.clock.Advance(5 * time.Minute)

	r.d.Reconcile(context.Background())

	assert.Equal(t, domain.WakeStatusClaimed, r.status(row.ID))
	assert.Empty(t, r.steerer.pane.written())
}

func TestDispatcher_should_DeliverReminder_When_NotifyOperatorOwnerIsArchivedOrDeleted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		facts OwnerFacts
		text  string
	}{
		{"archived", OwnerFacts{Exists: true, Archived: true}, "check review"},
		{"one-shot", OwnerFacts{Exists: true, Live: true, OneShot: true}, "check review"},
		{"terminal item", OwnerFacts{Exists: true, Live: true, ItemTerminal: true}, "check review"},
		{"stopped", OwnerFacts{Exists: true}, "check review"},
		{"deleted", OwnerFacts{}, "Session no longer exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newDispatcherRig(t)
			target := &spyTarget{}
			tt.facts.Revive = target
			r.owners.set("owner-1", tt.facts)
			at := t0
			row := newRow(t, r.repo, rowOpts{action: domain.WakeActionNotifyOperator, payload: "check review", spec: domain.AtSpec{At: at}, nextFire: &at})

			r.d.timerClaim(context.Background(), row)

			assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
			notes := r.notifier.all()
			require.Len(t, notes, 1)
			assert.Equal(t, NotifyReminder, notes[0].Kind)
			assert.Contains(t, notes[0].Message, tt.text)
			recovered, started := target.calls()
			assert.Zero(t, recovered+started, "no revive call")
			assert.Empty(t, r.steerer.pane.written())
		})
	}
}

func TestDispatcher_should_SkipWithFailureNotification_When_ArchivedOrStoppedWithoutAllowRevive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		facts  OwnerFacts
		reason domain.WakeReason
	}{
		{"archived", OwnerFacts{Exists: true, Live: true, Archived: true}, domain.WakeReasonArchived},
		{"stopped", OwnerFacts{Exists: true}, domain.WakeReasonOwnerStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newDispatcherRig(t)
			target := &spyTarget{}
			tt.facts.Revive = target
			r.owners.set("owner-1", tt.facts)
			row := newRow(t, r.repo, rowOpts{})

			r.d.timerClaim(context.Background(), row)

			got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
			require.NoError(t, err)
			assert.Equal(t, domain.WakeStatusSkipped, got.Status)
			assert.Equal(t, tt.reason, got.EndReason)
			notes := r.notifier.all()
			require.Len(t, notes, 1)
			assert.Equal(t, NotifyFailure, notes[0].Kind)
			recovered, started := target.calls()
			assert.Zero(t, recovered+started)
		})
	}
}

func TestDispatcher_should_ReviveOnce_When_AllowReviveAndSlotFreesOnNextCycle(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	target := &spyTarget{onStart: func() {
		r.owners.set("owner-1", OwnerFacts{Exists: true, Live: true})
	}}
	r.owners.set("owner-1", OwnerFacts{Exists: true, Revive: target})
	row := newRow(t, r.repo, rowOpts{allowRevive: true})
	r.start(r.d)

	r.d.timerClaim(context.Background(), row)
	r.d.WaitIdle()
	assert.Equal(t, domain.WakeStatusNeedsRevive, r.status(row.ID), "WIP cap full: nothing revived")
	recovered, started := target.calls()
	assert.Zero(t, recovered+started)

	r.admission.allow.Store(true) // a slot frees
	r.d.Reconcile(context.Background())
	r.d.WaitIdle()

	assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	recovered, started = target.calls()
	assert.Equal(t, 1, recovered)
	assert.Equal(t, 1, started)
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestDispatcher_should_MarkMissed_When_NeedsReviveRowExpires(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.owners.set("owner-1", OwnerFacts{Exists: true, Revive: &spyTarget{}})
	exp := t0.Add(2 * time.Hour)
	row := newRow(t, r.repo, rowOpts{allowRevive: true, expiresAt: &exp})
	r.start(r.d)
	r.d.timerClaim(context.Background(), row)
	r.d.WaitIdle()
	require.Equal(t, domain.WakeStatusNeedsRevive, r.status(row.ID))

	r.clock.Advance(3 * time.Hour)
	r.d.Reconcile(context.Background())
	r.d.WaitIdle()

	assert.Equal(t, domain.WakeStatusMissed, r.status(row.ID))
}

func TestDispatcher_should_RaiseOneDigest_When_ManyRowsEndDuringReconcile(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	past := t0.Add(-time.Hour)
	exp := t0.Add(-30 * time.Minute)
	for i := range 12 {
		newRow(t, r.repo, rowOpts{owner: fmt.Sprintf("owner-%d", i%4+2), payload: fmt.Sprintf("p%d", i), spec: domain.AtSpec{At: past}, nextFire: &past, expiresAt: &exp})
	}
	for i := range 3 {
		owner := fmt.Sprintf("archived-%d", i)
		r.owners.set(owner, OwnerFacts{Exists: true, Live: true, Archived: true})
		r.steerer.setReady(owner, true)
		newRow(t, r.repo, rowOpts{owner: owner, payload: "x", spec: domain.AtSpec{At: past}, nextFire: &past})
	}

	r.newDispatcher().reconcileStartup(context.Background())

	notes := r.notifier.all()
	require.Len(t, notes, 1, "one digest, no per-row notification")
	assert.Contains(t, notes[0].Message, "15 scheduled prompts")
}

func TestDispatcher_should_ReturnErrNotLeader_When_AnotherProcessLeads(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Leader = denyLeader{} })
	assert.ErrorIs(t, r.d.Start(context.Background()), ErrNotLeader)
}

type denyLeader struct{}

func (denyLeader) TryAcquire() (func(), bool, error) { return nil, false, nil }
