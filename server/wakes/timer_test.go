package wakes

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var t0 = time.Date(2026, 10, 18, 10, 2, 0, 0, time.UTC)

type claimRecorder struct {
	mu   sync.Mutex
	rows []*session.WakeTriggerData
	ch   chan *session.WakeTriggerData
}

func newClaimRecorder() *claimRecorder {
	return &claimRecorder{ch: make(chan *session.WakeTriggerData, 16)}
}

func (r *claimRecorder) onClaimed(_ context.Context, row *session.WakeTriggerData) {
	r.mu.Lock()
	r.rows = append(r.rows, row)
	r.mu.Unlock()
	r.ch <- row
}

func (r *claimRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

func (r *claimRecorder) next(t *testing.T) *session.WakeTriggerData {
	t.Helper()
	select {
	case row := <-r.ch:
		return row
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a claimed wake")
		return nil
	}
}

func createAt(t *testing.T, repo *session.EntRepository, owner string, at time.Time) uuid.UUID {
	t.Helper()
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "wake up "+at.String())
	require.NoError(t, err)
	d, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: owner, Spec: domain.AtSpec{At: at}, Action: action, NextFireAt: &at,
	})
	require.NoError(t, err)
	return uuid.MustParse(d.ID)
}

func startTimekeeper(t *testing.T, repo *session.EntRepository, clock Clock, claimed ClaimedFunc, opts ...TimekeeperOption) *Timekeeper {
	t.Helper()
	tk := NewTimekeeper(repo, clock, claimed, opts...)
	repo.SetWakeChangeHook(tk.Rearm)
	require.NoError(t, tk.Start(context.Background()))
	t.Cleanup(func() { _ = tk.Stop(context.Background()) })
	return tk
}

// Wakes must work with no workflow scheduler or workflow repo constructed:
// nothing in this package imports server/workflows.
func TestTimekeeper_should_ClaimOnce_When_ClockReachesNextFireAt(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	clock := NewFakeClock(t0)
	rec := newClaimRecorder()
	id := createAt(t, repo, "owner", t0.Add(2*time.Second))
	startTimekeeper(t, repo, clock, rec.onClaimed)

	clock.BlockUntilTimerAt(t0.Add(2 * time.Second))
	clock.Advance(2 * time.Second)

	got := rec.next(t)
	assert.Equal(t, id.String(), got.ID)
	assert.Equal(t, domain.WakeStatusClaimed, got.Status)
	stored, err := repo.GetWakeTrigger(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusClaimed, stored.Status)
	assert.Equal(t, 1, rec.count())
}

func TestTimekeeper_should_RearmToEarliest_When_TriggerCreatedOrCancelled(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	clock := NewFakeClock(t0)
	createAt(t, repo, "owner", t0.Add(2*time.Hour))
	startTimekeeper(t, repo, clock, newClaimRecorder().onClaimed)
	clock.BlockUntilTimerAt(t0.Add(2 * time.Hour))

	soon := createAt(t, repo, "owner", t0.Add(time.Hour))
	clock.BlockUntilTimerAt(t0.Add(time.Hour))
	assert.Equal(t, []time.Time{t0.Add(time.Hour)}, clock.Deadlines(), "old timer is replaced, not kept")

	applied, err := repo.CancelWakeTrigger(context.Background(), soon, t0)
	require.NoError(t, err)
	require.True(t, applied)
	clock.BlockUntilTimerAt(t0.Add(2 * time.Hour))
	assert.Equal(t, []time.Time{t0.Add(2 * time.Hour)}, clock.Deadlines())
}

func TestTimekeeper_should_MarkMissedNotClaim_When_RowExpiredBeforeFiring(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	clock := NewFakeClock(t0)
	rec := newClaimRecorder()
	missed := make(chan *session.WakeTriggerData, 1)

	at := t0.Add(time.Minute)
	exp := t0.Add(30 * time.Second)
	action, _ := domain.NewWakeAction(domain.WakeActionSteerText, "late")
	d, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: "owner", Spec: domain.AtSpec{At: at}, Action: action, NextFireAt: &at, ExpiresAt: &exp,
	})
	require.NoError(t, err)
	startTimekeeper(t, repo, clock, rec.onClaimed, WithMissedFunc(func(_ context.Context, row *session.WakeTriggerData) { missed <- row }))

	clock.BlockUntilTimerAt(at)
	clock.Advance(time.Minute)
	select {
	case row := <-missed:
		assert.Equal(t, d.ID, row.ID)
	case <-time.After(10 * time.Second):
		t.Fatal("expected a missed callback")
	}
	stored, err := repo.GetWakeTrigger(context.Background(), uuid.MustParse(d.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusMissed, stored.Status)
	assert.Equal(t, domain.WakeReasonExpired, stored.EndReason)
	assert.Equal(t, 0, rec.count())
}

func createCron(t *testing.T, repo *session.EntRepository, expr string, next time.Time) uuid.UUID {
	t.Helper()
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "tick")
	require.NoError(t, err)
	d, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: "owner", Spec: domain.CronSpec{Expr: expr}, Action: action, NextFireAt: &next, MaxFires: 10,
	})
	require.NoError(t, err)
	return uuid.MustParse(d.ID)
}

// deliverAndRearm stands in for the Story 4.3 delivery step: confirm the write
// and re-arm the cron row at its next occurrence.
func deliverAndRearm(t *testing.T, repo *session.EntRepository, sched CronSchedule, clock *FakeClock) ClaimedFunc {
	return func(ctx context.Context, row *session.WakeTriggerData) {
		id := uuid.MustParse(row.ID)
		next := sched.NextAfterFire(*row.NextFireAt, clock.Now())
		applied, err := repo.MarkWakeDelivered(ctx, id, clock.Now(), next)
		assert.NoError(t, err)
		assert.True(t, applied)
	}
}

func TestTimekeeper_should_FireCronEveryFiveMinutes_When_Rearmed(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	clock := NewFakeClock(t0)
	sched := mustParseCron(t, "*/5 * * * *")
	first := sched.Next(t0)
	require.Equal(t, utc(2026, 10, 18, 10, 5), first)
	id := createCron(t, repo, "*/5 * * * *", first)

	fired := make(chan time.Time, 4)
	deliver := deliverAndRearm(t, repo, sched, clock)
	startTimekeeper(t, repo, clock, func(ctx context.Context, row *session.WakeTriggerData) {
		fired <- *row.NextFireAt
		deliver(ctx, row)
	})

	for _, want := range []time.Time{utc(2026, 10, 18, 10, 5), utc(2026, 10, 18, 10, 10)} {
		clock.BlockUntilTimerAt(want)
		clock.Set(want)
		assert.Equal(t, want, <-fired)
	}
	clock.BlockUntilTimerAt(utc(2026, 10, 18, 10, 15))
	stored, err := repo.GetWakeTrigger(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, 2, stored.FireCount)
	assert.Equal(t, domain.WakeStatusPending, stored.Status)
}

func TestTimekeeper_should_NotRefireOccurrence_When_ClockJumpsBackAfterFire(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	clock := NewFakeClock(t0)
	sched := mustParseCron(t, "*/5 * * * *")
	first := sched.Next(t0)
	createCron(t, repo, "*/5 * * * *", first)

	fired := make(chan time.Time, 4)
	deliver := deliverAndRearm(t, repo, sched, clock)
	startTimekeeper(t, repo, clock, func(ctx context.Context, row *session.WakeTriggerData) {
		fired <- *row.NextFireAt
		clock.Set(first.Add(-time.Hour)) // the clock steps back before delivery is confirmed
		deliver(ctx, row)
	})

	clock.BlockUntilTimerAt(first)
	clock.Set(first)
	assert.Equal(t, first, <-fired)
	// Re-armed for 10:10 even though "now" is 09:05; 09:10 and 10:05 do not fire again.
	clock.BlockUntilTimerAt(first.Add(5 * time.Minute))
	assert.Empty(t, fired)
}

func TestTimekeeper_Stop_should_JoinLoopGoroutine(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	clock := NewFakeClock(t0)
	createAt(t, repo, "owner", t0.Add(time.Hour))
	tk := NewTimekeeper(repo, clock, nil)
	require.NoError(t, tk.Start(context.Background()))
	clock.BlockUntilTimers(1)

	require.NoError(t, tk.Stop(context.Background()))
	assert.NoError(t, tk.Stop(context.Background()), "second Stop is a no-op")
	assert.Zero(t, clock.ActiveTimers(), "timer released")
}

func TestTimekeeper_Start_should_RejectSecondStart_When_AlreadyRunning(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	tk := startTimekeeper(t, repo, NewFakeClock(t0), nil)
	assert.Error(t, tk.Start(context.Background()))
}

func TestTimekeeper_Start_should_ReturnNotLeader_When_AnotherHolderOwnsLock(t *testing.T) {
	t.Parallel()
	lockPath := filepath.Join(t.TempDir(), "wakes.leader.lock")
	repo := session.NewTestEntRepository(t)

	a := NewTimekeeper(repo, NewFakeClock(t0), nil, WithLeaderGuard(FileLeader{Path: lockPath}))
	b := NewTimekeeper(repo, NewFakeClock(t0), nil, WithLeaderGuard(FileLeader{Path: lockPath}))
	require.NoError(t, a.Start(context.Background()))
	require.ErrorIs(t, b.Start(context.Background()), ErrNotLeader)

	require.NoError(t, a.Stop(context.Background()))
	require.NoError(t, b.Start(context.Background()), "leadership is released on Stop")
	require.NoError(t, b.Stop(context.Background()))
}

func TestTimekeeper_Stop_should_ReturnError_When_ContextExpiresBeforeLoopExits(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	block := make(chan struct{})
	clock := NewFakeClock(t0)
	createAt(t, repo, "owner", t0)
	entered := make(chan struct{}, 1)
	tk := NewTimekeeper(repo, clock, func(context.Context, *session.WakeTriggerData) {
		entered <- struct{}{}
		<-block
	})
	require.NoError(t, tk.Start(context.Background()))
	t.Cleanup(func() { close(block); _ = tk.Stop(context.Background()) })

	// The loop is stuck inside the claimed callback; Stop must honor its own deadline.
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	assert.Error(t, tk.Stop(ctx))
}
