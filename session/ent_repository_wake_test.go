package session

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

var wakeBase = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

func mustWakeAction(t *testing.T, payload string) domain.WakeAction {
	t.Helper()
	a, err := domain.NewWakeAction(domain.WakeActionSteerText, payload)
	require.NoError(t, err)
	return a
}

func atInput(t *testing.T, owner string, at time.Time, payload string) CreateWakeTriggerInput {
	t.Helper()
	return CreateWakeTriggerInput{
		OwnerSessionUUID: owner,
		Spec:             domain.AtSpec{At: at},
		Action:           mustWakeAction(t, payload),
		NextFireAt:       &at,
	}
}

func cronInput(t *testing.T, owner string, maxFires int) CreateWakeTriggerInput {
	t.Helper()
	next := wakeBase.Add(5 * time.Minute)
	return CreateWakeTriggerInput{
		OwnerSessionUUID: owner,
		Spec:             domain.CronSpec{Expr: "*/5 * * * *"},
		Action:           mustWakeAction(t, "tick"),
		NextFireAt:       &next,
		MaxFires:         maxFires,
	}
}

func mustCreateWake(t *testing.T, repo *EntRepository, in CreateWakeTriggerInput) (uuid.UUID, *WakeTriggerData) {
	t.Helper()
	d, _, err := repo.CreateWakeTrigger(context.Background(), in)
	require.NoError(t, err)
	id, err := uuid.Parse(d.ID)
	require.NoError(t, err)
	return id, d
}

func wakeStatusOf(t *testing.T, repo *EntRepository, id uuid.UUID) *WakeTriggerData {
	t.Helper()
	d, err := repo.GetWakeTrigger(context.Background(), id)
	require.NoError(t, err)
	return d
}

func TestCreateWakeTrigger_should_ReturnFirstRow_When_SameTriggerCreatedTwice(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	in := atInput(t, "7c1e", wakeBase, "check PR")
	first, created1, err := repo.CreateWakeTrigger(context.Background(), in)
	require.NoError(t, err)
	second, created2, err := repo.CreateWakeTrigger(context.Background(), in)
	require.NoError(t, err)

	assert.True(t, created1)
	assert.False(t, created2)
	assert.Equal(t, first.ID, second.ID)
	n, err := repo.CountLiveWakeTriggersByOwner(context.Background(), "7c1e")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestCreateWakeTrigger_should_Reject_When_InputInvalid(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	noNext := atInput(t, "o", wakeBase, "x")
	noNext.NextFireAt = nil
	noSpec := atInput(t, "o", wakeBase, "x")
	noSpec.Spec = nil
	noOwner := atInput(t, "", wakeBase, "x")
	for name, in := range map[string]CreateWakeTriggerInput{"no next_fire_at": noNext, "no spec": noSpec, "no owner": noOwner} {
		_, _, err := repo.CreateWakeTrigger(ctx, in)
		assert.Error(t, err, name)
	}
}

func TestCreateWakeTrigger_should_EnforceOwnerCap_When_DedupHitIsExempt(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	const capLimit = 2
	mk := func(i int) CreateWakeTriggerInput {
		in := atInput(t, "owner", wakeBase.Add(time.Duration(i)*time.Hour), "p")
		in.PendingCap = capLimit
		return in
	}
	for i := 0; i < capLimit; i++ {
		_, _, err := repo.CreateWakeTrigger(ctx, mk(i))
		require.NoError(t, err)
	}
	_, _, err := repo.CreateWakeTrigger(ctx, mk(capLimit))
	require.ErrorIs(t, err, ErrWakeCapExceeded)

	// Re-creating an existing trigger at the cap resolves to it, not a rejection.
	_, created, err := repo.CreateWakeTrigger(ctx, mk(0))
	require.NoError(t, err)
	assert.False(t, created)
}

func TestClaimWakeTrigger_should_AllowExactlyOneWinner_When_EightGoroutinesRace(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))

	const racers = 8
	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			applied, err := repo.ClaimWakeTrigger(context.Background(), id, wakeBase)
			assert.NoError(t, err)
			if applied {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), wins.Load())
	got := wakeStatusOf(t, repo, id)
	assert.Equal(t, domain.WakeStatusClaimed, got.Status)
	assert.NotNil(t, got.ClaimedAt)
	assert.Nil(t, got.DeliveredAt)
}

func TestClaimWakeTrigger_should_Refuse_When_RowExpired(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	in := atInput(t, "o", wakeBase, "x")
	exp := wakeBase.Add(-time.Minute)
	in.ExpiresAt = &exp
	id, _ := mustCreateWake(t, repo, in)

	applied, err := repo.ClaimWakeTrigger(context.Background(), id, wakeBase)
	require.NoError(t, err)
	assert.False(t, applied)

	applied, err = repo.MarkWakeMissed(context.Background(), id, domain.WakeReasonExpired, wakeBase)
	require.NoError(t, err)
	assert.True(t, applied)
	got := wakeStatusOf(t, repo, id)
	assert.Equal(t, domain.WakeStatusMissed, got.Status)
	assert.Nil(t, got.ClaimedAt)
	assert.Nil(t, got.DeliveredAt)
	assert.Equal(t, domain.WakeReasonExpired, got.EndReason)
}

func TestMarkWakeDelivered_should_BeIdempotent_When_CalledTwice(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))

	applied, err := repo.MarkWakeDelivered(ctx, id, wakeBase, time.Time{})
	require.NoError(t, err)
	assert.False(t, applied, "pending -> delivered is not in the table")

	_, err = repo.ClaimWakeTrigger(ctx, id, wakeBase)
	require.NoError(t, err)
	applied, err = repo.MarkWakeDelivered(ctx, id, wakeBase.Add(time.Second), time.Time{})
	require.NoError(t, err)
	assert.True(t, applied)
	applied, err = repo.MarkWakeDelivered(ctx, id, wakeBase.Add(time.Minute), time.Time{})
	require.NoError(t, err)
	assert.False(t, applied)

	got := wakeStatusOf(t, repo, id)
	assert.Equal(t, domain.WakeStatusDelivered, got.Status)
	assert.Equal(t, 1, got.FireCount)
	require.NotNil(t, got.DeliveredAt)
	assert.True(t, got.DeliveredAt.Equal(wakeBase.Add(time.Second)))
}

func TestWakeTriggerRepository_should_FollowNeedsReviveAndCancelRules(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "a"))
	_, err := repo.ClaimWakeTrigger(ctx, id, wakeBase)
	require.NoError(t, err)
	ok, err := repo.MarkWakeNeedsRevive(ctx, id, wakeBase)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, domain.WakeStatusNeedsRevive, wakeStatusOf(t, repo, id).Status)

	ok, err = repo.ReadmitWakeTrigger(ctx, id, wakeBase)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.MarkWakeSkipped(ctx, id, domain.WakeReasonArchived, wakeBase)
	require.NoError(t, err)
	assert.True(t, ok)
	got := wakeStatusOf(t, repo, id)
	assert.Equal(t, domain.WakeStatusSkipped, got.Status)
	assert.Equal(t, domain.WakeReasonArchived, got.EndReason)

	// delivered -> claimed and delivered -> cancelled are not in the table.
	did, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(time.Hour), "b"))
	_, _ = repo.ClaimWakeTrigger(ctx, did, wakeBase)
	_, _ = repo.MarkWakeDelivered(ctx, did, wakeBase, time.Time{})
	for _, fn := range []func() (bool, error){
		func() (bool, error) { return repo.ClaimWakeTrigger(ctx, did, wakeBase) },
		func() (bool, error) { return repo.ReadmitWakeTrigger(ctx, did, wakeBase) },
		func() (bool, error) { return repo.CancelWakeTrigger(ctx, did, wakeBase) },
	} {
		applied, err := fn()
		require.NoError(t, err)
		assert.False(t, applied)
	}

	// cancel from claimed
	cid, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(2*time.Hour), "c"))
	_, _ = repo.ClaimWakeTrigger(ctx, cid, wakeBase)
	ok, err = repo.CancelWakeTrigger(ctx, cid, wakeBase)
	require.NoError(t, err)
	assert.True(t, ok)
}

// wakePathTo drives a fresh one-shot row into the wanted status.
func wakePathTo(t *testing.T, repo *EntRepository, owner string, status domain.WakeStatus) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id, _ := mustCreateWake(t, repo, atInput(t, owner, wakeBase, string(status)))
	claim := func() { _, err := repo.ClaimWakeTrigger(ctx, id, wakeBase); require.NoError(t, err) }
	var err error
	switch status {
	case domain.WakeStatusPending:
	case domain.WakeStatusClaimed:
		claim()
	case domain.WakeStatusNeedsRevive:
		claim()
		_, err = repo.MarkWakeNeedsRevive(ctx, id, wakeBase)
	case domain.WakeStatusDelivered:
		claim()
		_, err = repo.MarkWakeDelivered(ctx, id, wakeBase, time.Time{})
	case domain.WakeStatusMissed:
		_, err = repo.MarkWakeMissed(ctx, id, domain.WakeReasonExpired, wakeBase)
	case domain.WakeStatusCancelled:
		_, err = repo.CancelWakeTrigger(ctx, id, wakeBase)
	case domain.WakeStatusSkipped:
		_, err = repo.MarkWakeSkipped(ctx, id, domain.WakeReasonRateLimited, wakeBase)
	}
	require.NoError(t, err)
	require.Equal(t, status, wakeStatusOf(t, repo, id).Status)
	return id
}

func TestWakeTriggerRepository_Transitions_should_MatchDomainTable_When_EveryFromToPairTried(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	// One method per reachable target (claimed has two entry points).
	moves := map[domain.WakeStatus][]func(id uuid.UUID) (bool, error){
		domain.WakeStatusClaimed: {
			func(id uuid.UUID) (bool, error) { return repo.ClaimWakeTrigger(ctx, id, wakeBase) },
			func(id uuid.UUID) (bool, error) { return repo.ReadmitWakeTrigger(ctx, id, wakeBase) },
		},
		domain.WakeStatusNeedsRevive: {func(id uuid.UUID) (bool, error) { return repo.MarkWakeNeedsRevive(ctx, id, wakeBase) }},
		domain.WakeStatusDelivered: {func(id uuid.UUID) (bool, error) {
			return repo.MarkWakeDelivered(ctx, id, wakeBase, time.Time{})
		}},
		domain.WakeStatusMissed: {func(id uuid.UUID) (bool, error) {
			return repo.MarkWakeMissed(ctx, id, domain.WakeReasonExpired, wakeBase)
		}},
		domain.WakeStatusSkipped: {func(id uuid.UUID) (bool, error) {
			return repo.MarkWakeSkipped(ctx, id, domain.WakeReasonRateLimited, wakeBase)
		}},
		domain.WakeStatusCancelled: {func(id uuid.UUID) (bool, error) { return repo.CancelWakeTrigger(ctx, id, wakeBase) }},
	}
	for _, from := range domain.AllWakeStatuses() {
		for to, fns := range moves {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				id := wakePathTo(t, repo, "pair-"+string(from)+"-"+string(to), from)
				anyApplied := false
				for _, fn := range fns {
					applied, err := fn(id)
					require.NoError(t, err)
					anyApplied = anyApplied || applied
				}
				assert.Equal(t, from.CanTransitionTo(to), anyApplied)
				want := from
				if anyApplied {
					want = to
				}
				assert.Equal(t, want, wakeStatusOf(t, repo, id).Status)
			})
		}
	}
}

func TestMarkWakeDelivered_should_RearmCronRow_When_FiresRemain(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	id, d := mustCreateWake(t, repo, cronInput(t, "o", 3))
	next := *d.NextFireAt

	for fire := 1; fire <= 3; fire++ {
		applied, err := repo.ClaimWakeTrigger(ctx, id, next)
		require.NoError(t, err)
		require.True(t, applied, "claim %d", fire)
		next = next.Add(5 * time.Minute)
		applied, err = repo.MarkWakeDelivered(ctx, id, next, next)
		require.NoError(t, err)
		require.True(t, applied)
		got := wakeStatusOf(t, repo, id)
		assert.Equal(t, fire, got.FireCount)
		if fire < 3 {
			assert.Equal(t, domain.WakeStatusPending, got.Status)
			assert.Nil(t, got.ClaimedAt)
			assert.True(t, got.NextFireAt.Equal(next))
		} else {
			assert.Equal(t, domain.WakeStatusDelivered, got.Status)
		}
	}
	applied, err := repo.ClaimWakeTrigger(ctx, id, next)
	require.NoError(t, err)
	assert.False(t, applied, "fourth claim rejected")
}

func TestMarkWakeDelivered_should_Finish_When_CronNextFireIsPastExpiry(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	in := cronInput(t, "o", 10)
	exp := wakeBase.Add(7 * time.Minute)
	in.ExpiresAt = &exp
	id, d := mustCreateWake(t, repo, in)
	_, err := repo.ClaimWakeTrigger(ctx, id, *d.NextFireAt)
	require.NoError(t, err)
	_, err = repo.MarkWakeDelivered(ctx, id, *d.NextFireAt, d.NextFireAt.Add(5*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusDelivered, wakeStatusOf(t, repo, id).Status)
}

func TestMarkWakeSkipped_should_RejectPendingCron_When_RateLimited(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	atID, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))
	applied, err := repo.MarkWakeSkipped(ctx, atID, domain.WakeReasonRateLimited, wakeBase)
	require.NoError(t, err)
	assert.True(t, applied)

	cronID, d := mustCreateWake(t, repo, cronInput(t, "o", 5))
	_, err = repo.MarkWakeSkipped(ctx, cronID, domain.WakeReasonRateLimited, wakeBase)
	require.ErrorIs(t, err, ErrWakeTransitionRejected)

	later := d.NextFireAt.Add(5 * time.Minute)
	_, _, err = repo.DeferWakeTrigger(ctx, cronID, d.NextFireAt.Add(time.Minute))
	require.NoError(t, err)
	applied, err = repo.SkipWakeOccurrence(ctx, cronID, later)
	require.NoError(t, err)
	assert.True(t, applied)
	got := wakeStatusOf(t, repo, cronID)
	assert.Equal(t, domain.WakeStatusPending, got.Status)
	assert.Equal(t, 0, got.FireCount)
	assert.Equal(t, 0, got.RateLimitDeferrals)
	assert.True(t, got.NextFireAt.Equal(later))

	_, err = repo.SkipWakeOccurrence(ctx, atID, later)
	require.ErrorIs(t, err, ErrWakeTransitionRejected)
}

func TestDeferWakeTrigger_should_SurviveRestartAndResetOnClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wake-restart.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))
	for want := 1; want <= 2; want++ {
		n, applied, err := repo.DeferWakeTrigger(ctx, id, wakeBase.Add(time.Duration(want)*time.Minute))
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, want, n)
	}
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	assert.Equal(t, 2, wakeStatusOf(t, reopened, id).RateLimitDeferrals)

	n, _, err := reopened.DeferWakeTrigger(ctx, id, wakeBase.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	applied, err := reopened.MarkWakeSkipped(ctx, id, domain.WakeReasonRateLimited, wakeBase)
	require.NoError(t, err)
	assert.True(t, applied)

	// A deferred-then-claimed row starts again at zero.
	id2, _ := mustCreateWake(t, reopened, atInput(t, "o", wakeBase.Add(time.Hour), "y"))
	_, _, err = reopened.DeferWakeTrigger(ctx, id2, wakeBase.Add(time.Minute))
	require.NoError(t, err)
	_, err = reopened.ClaimWakeTrigger(ctx, id2, wakeBase)
	require.NoError(t, err)
	assert.Equal(t, 0, wakeStatusOf(t, reopened, id2).RateLimitDeferrals)
}

func TestWakeTrigger_should_SurviveRestart_When_PendingRowReadBackFromDisk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wake-survive.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	_, created := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "keep me"))
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	pending, err := reopened.ListPendingWakeTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, created.ID, pending[0].ID)
	assert.Equal(t, "keep me", pending[0].Action.Payload)
	assert.Equal(t, domain.AtSpec{At: wakeBase}, pending[0].Spec)
}

func TestWakeTrigger_should_FreeDedupKey_When_RowBecomesTerminal(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	in := atInput(t, "o", wakeBase, "again")
	id, first := mustCreateWake(t, repo, in)

	dup, created, err := repo.CreateWakeTrigger(ctx, in)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, dup.ID)

	_, _ = repo.ClaimWakeTrigger(ctx, id, wakeBase)
	_, _ = repo.MarkWakeDelivered(ctx, id, wakeBase, time.Time{})
	ended := wakeStatusOf(t, repo, id)
	assert.True(t, ended.DedupKey.IsTombstone())

	again, created, err := repo.CreateWakeTrigger(ctx, in)
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, first.ID, again.ID)
}

func TestSweepTerminalWakeTriggers_should_DeleteOnlyRowsOlderThanRetention(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	const retention = 30 * 24 * time.Hour
	now := wakeBase.Add(40 * 24 * time.Hour)

	old, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "old"))
	recent, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(time.Hour), "recent"))
	live, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(2*time.Hour), "live"))
	_, err := repo.CancelWakeTrigger(ctx, old, now.Add(-31*24*time.Hour))
	require.NoError(t, err)
	_, err = repo.CancelWakeTrigger(ctx, recent, now.Add(-29*24*time.Hour))
	require.NoError(t, err)

	n, err := repo.SweepTerminalWakeTriggers(ctx, now, retention)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, err = repo.GetWakeTrigger(ctx, old)
	assert.Error(t, err)
	assert.Equal(t, domain.WakeStatusCancelled, wakeStatusOf(t, repo, recent).Status)
	assert.Equal(t, domain.WakeStatusPending, wakeStatusOf(t, repo, live).Status)
}

func TestWakeTriggerQueries_should_ReturnTimerAndReconcileViews(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	_, ok, err := repo.NextWakeFireAt(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	later, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(2*time.Hour), "later"))
	sooner, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase.Add(time.Hour), "sooner"))
	at, ok, err := repo.NextWakeFireAt(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, at.Equal(wakeBase.Add(time.Hour)))

	due, err := repo.ListDueWakeTriggers(ctx, wakeBase.Add(90*time.Minute))
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, sooner.String(), due[0].ID)

	_, _ = repo.ClaimWakeTrigger(ctx, later, wakeBase)
	stale, err := repo.ListStaleClaimedWakeTriggers(ctx, wakeBase.Add(3*time.Minute))
	require.NoError(t, err)
	require.Len(t, stale, 1)
	fresh, err := repo.ListStaleClaimedWakeTriggers(ctx, wakeBase.Add(-time.Minute))
	require.NoError(t, err)
	assert.Empty(t, fresh)

	_, _ = repo.MarkWakeNeedsRevive(ctx, later, wakeBase)
	nr, err := repo.ListNeedsReviveWakeTriggers(ctx)
	require.NoError(t, err)
	assert.Len(t, nr, 1)

	byOwner, err := repo.ListWakeTriggersByOwner(ctx, "o", domain.WakeStatusPending)
	require.NoError(t, err)
	assert.Len(t, byOwner, 1)
	require.NoError(t, repo.RecordWakeDeliveryAttempt(ctx, later))
}

func TestWakeChangeHook_should_Fire_When_RowsChange(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	var calls atomic.Int32
	repo.SetWakeChangeHook(func() { calls.Add(1) })
	id, _ := mustCreateWake(t, repo, atInput(t, "o", wakeBase, "x"))
	assert.Equal(t, int32(1), calls.Load())
	_, err := repo.CancelWakeTrigger(context.Background(), id, wakeBase)
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	_, err = repo.CancelWakeTrigger(context.Background(), id, wakeBase) // no-op
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}
