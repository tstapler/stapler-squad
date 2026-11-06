package wakes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestEffectiveCronFloor_should_EqualMaxOfFiveAndCeilSixtyOverLimit(t *testing.T) {
	t.Parallel()
	for limit := 1; limit <= 12; limit++ {
		want := max(5, (60+limit-1)/limit)
		assert.Equal(t, time.Duration(want)*time.Minute, EffectiveCronFloor(limit), "limit %d", limit)
	}
}

func TestResolveLimits_should_ClampToCeilingsAndDeriveCronFloor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		settings  LimitSettings
		wantFloor time.Duration
		wantOwner int
	}{
		{"defaults", LimitSettings{}, 10 * time.Minute, 6},
		{"floor lowered but owner limit still 6 stays 10 min", LimitSettings{MinCronIntervalMinutes: 5}, 10 * time.Minute, 6},
		{"floor 5 with owner limit 12 is 5 min", LimitSettings{MinCronIntervalMinutes: 5, FiresPerOwnerHour: 12}, 5 * time.Minute, 12},
		{"floor 4 is clamped to 5", LimitSettings{MinCronIntervalMinutes: 4, FiresPerOwnerHour: 12}, 5 * time.Minute, 12},
		{"owner limit above ceiling is clamped to 12", LimitSettings{FiresPerOwnerHour: 99}, 5 * time.Minute, 12},
		{"a larger configured floor is kept", LimitSettings{MinCronIntervalMinutes: 30}, 30 * time.Minute, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := ResolveLimits(tt.settings)
			assert.Equal(t, tt.wantFloor, l.MinCronInterval)
			assert.Equal(t, tt.wantOwner, l.FiresPerOwnerHour)
		})
	}
	big := ResolveLimits(LimitSettings{PendingPerOwner: 99, MaxFiresCron: 99, ExpiresHorizonHours: 9999, Generation: 99, FiresGlobalHour: 999, PeerOpenPerAsker: 99, PayloadChars: 99999, DebounceSeconds: 999, BusyHoldMinutes: 9999, ReviveLockSeconds: 999})
	assert.Equal(t, 10, big.PendingPerOwner)
	assert.Equal(t, 48, big.MaxFiresCron)
	assert.Equal(t, 7*24*time.Hour, big.ExpiresHorizon)
	assert.Equal(t, 5, big.Generation)
	assert.Equal(t, 120, big.FiresGlobalHour)
	assert.Equal(t, 5, big.PeerOpenPerAsker)
	assert.Equal(t, 4000, big.PayloadChars)
	assert.Equal(t, time.Minute, big.Debounce)
	assert.Equal(t, 2*time.Hour, big.BusyHold)
	assert.Equal(t, time.Minute, big.ReviveLockHold)
}

func scheduleAt(owner string, at time.Time, payload string) ScheduleRequest {
	return ScheduleRequest{
		Owner: owner, Spec: domain.AtSpec{At: at},
		Action: domain.WakeAction{Kind: domain.WakeActionSteerText, Payload: payload},
	}
}

func scheduleCron(owner, expr string) ScheduleRequest {
	return ScheduleRequest{
		Owner: owner, Spec: domain.CronSpec{Expr: expr},
		Action: domain.WakeAction{Kind: domain.WakeActionSteerText, Payload: "tick"},
	}
}

func assertCap(t *testing.T, err error, limit string) {
	t.Helper()
	var ce *CapExceededError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, limit, ce.Limit)
	assert.ErrorIs(t, err, ErrCapExceeded)
}

func TestSchedule_should_RefuseCron_When_IntervalBelowEffectiveFloor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings LimitSettings
		expr     string
		wantErr  bool
	}{
		{"every minute at default", LimitSettings{}, "* * * * *", true},
		{"every 5 at default", LimitSettings{}, "*/5 * * * *", true},
		{"every 10 at default", LimitSettings{}, "*/10 * * * *", false},
		{"irregular gap below floor", LimitSettings{}, "0,3 * * * *", true},
		{"floor lowered only: effective floor is still 10", LimitSettings{MinCronIntervalMinutes: 5}, "*/5 * * * *", true},
		{"floor 5 and owner limit 12 accepts every 5", LimitSettings{MinCronIntervalMinutes: 5, FiresPerOwnerHour: 12}, "*/5 * * * *", false},
		{"floor 4 is clamped to 5 so every 4 is refused", LimitSettings{MinCronIntervalMinutes: 4, FiresPerOwnerHour: 12}, "*/4 * * * *", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newDispatcherRig(t, func(c *Config) { c.Limits = tt.settings })
			row, _, err := r.d.Schedule(context.Background(), scheduleCron("owner-1", tt.expr))
			if tt.wantErr {
				assertCap(t, err, LimitMinInterval)
				assert.Nil(t, row)
				live, cerr := r.repo.CountLiveWakeTriggersByOwner(context.Background(), "owner-1")
				require.NoError(t, cerr)
				assert.Zero(t, live, "no row is created")
				assert.EqualValues(t, 1, r.d.Metrics().Value(MetricLimitHit, LimitMinInterval, PhaseCreate))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 12, row.MaxFires)
		})
	}
}

func TestSchedule_should_RefuseOutOfRangeRequests_When_PastCeilings(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	ctx := context.Background()
	in := t0.Add(time.Hour)

	req := scheduleAt("owner-1", in, "p")
	req.MaxFires = 49
	_, _, err := r.d.Schedule(ctx, req)
	assertCap(t, err, LimitMaxFires)

	req = scheduleAt("owner-1", in, "p")
	eightDays := t0.Add(8 * 24 * time.Hour)
	req.ExpiresAt = &eightDays
	_, _, err = r.d.Schedule(ctx, req)
	assertCap(t, err, LimitExpiresHorizon)

	_, _, err = r.d.Schedule(ctx, scheduleAt("owner-1", t0.Add(30*time.Hour), "beyond the 24h default horizon"))
	assertCap(t, err, LimitExpiresHorizon)

	req = scheduleAt("owner-1", in, "p")
	req.Generation = 4
	_, _, err = r.d.Schedule(ctx, req)
	assertCap(t, err, LimitGeneration)

	_, _, err = r.d.Schedule(ctx, scheduleAt("owner-1", in, strings.Repeat("x", 2001)))
	assert.ErrorIs(t, err, ErrInvalid)
	assert.EqualValues(t, 1, r.d.Metrics().Value(MetricLimitHit, LimitPayloadSize, PhaseCreate))

	req = scheduleAt("owner-1", in, strings.Repeat("x", 2000))
	req.MaxFires, req.Generation = 48, 3
	_, created, err := r.d.Schedule(ctx, req)
	require.NoError(t, err)
	assert.True(t, created, "the boundary values themselves are accepted")
}

func TestSchedule_should_ReturnEventStreamUnavailable_When_NoBus(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Events = nil })
	req := ScheduleRequest{
		Owner: "owner-1", Spec: domain.BacklogEventSpec{ItemID: itemX, Kind: "verdict_recorded"},
		Action: domain.WakeAction{Kind: domain.WakeActionSteerText, Payload: "p"},
	}
	_, _, err := r.d.Schedule(context.Background(), req)
	assert.ErrorIs(t, err, ErrEventStreamUnavailable)
}

func TestSchedule_should_AcceptExactlyCap_When_TwentyConcurrentCreates(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, capped := 0, 0
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := r.d.Schedule(context.Background(), scheduleAt("owner-1", t0.Add(time.Hour), fmt.Sprintf("p%d", i)))
			mu.Lock()
			defer mu.Unlock()
			var ce *CapExceededError
			switch {
			case err == nil:
				ok++
			case errors.As(err, &ce) && ce.Limit == LimitPendingPerOwner:
				capped++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 5, ok)
	assert.Equal(t, 15, capped)
	assert.EqualValues(t, 15, r.d.Metrics().Value(MetricLimitHit, LimitPendingPerOwner, PhaseCreate))
}

func TestSchedule_should_NotWaitOnReviveLock_When_RevivalIsBlocked(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.admission.allow.Store(true)
	release := make(chan struct{})
	target := &spyTarget{onStart: func() { <-release }}
	r.owners.set("stopped", OwnerFacts{Exists: true, Revive: target})
	go func() { _, _ = r.d.reviver.TryRevive(context.Background(), "stopped") }()
	require.Eventually(t, func() bool { _, started := target.calls(); return started == 1 }, 5*time.Second, time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, _, err := r.d.Schedule(context.Background(), scheduleAt("owner-1", t0.Add(time.Hour), "while the spawn hangs"))
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("create-time cap check waited on the revive lock")
	}
	close(release)
}

func dueAtRows(t *testing.T, r *dispatcherRig, owner string, n int) []*session.WakeTriggerData {
	t.Helper()
	rows := make([]*session.WakeTriggerData, n)
	for i := range n {
		rows[i] = newRow(t, r.repo, rowOpts{owner: owner, payload: fmt.Sprintf("due %d", i)})
	}
	return rows
}

func TestRateGate_should_DeferSeventhWake_When_OwnerLimitIsSix(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	rows := dueAtRows(t, r, "owner-1", 7)

	for _, row := range rows {
		r.d.timerClaim(context.Background(), row)
		r.clock.Advance(time.Minute)
	}

	delivered := 0
	for _, row := range rows[:6] {
		if r.status(row.ID) == domain.WakeStatusDelivered {
			delivered++
		}
	}
	assert.Equal(t, 6, delivered)
	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(rows[6].ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusPending, got.Status)
	require.NotNil(t, got.NextFireAt)
	assert.True(t, got.NextFireAt.Equal(t0.Add(time.Hour)), "next fire is when the window frees a slot: %s", got.NextFireAt)
	assert.Equal(t, 1, got.RateLimitDeferrals)
	assert.EqualValues(t, 1, r.d.Metrics().Value(MetricLimitHit, LimitFiresPerOwnerHour, PhaseFire))
	notes := r.notifier.all()
	require.Len(t, notes, 1, "exactly one failure-class digest for the owner")
	assert.Equal(t, NotifyFailure, notes[0].Kind)
	assert.Equal(t, 6, r.d.DeliveredPerOwnerHour("owner-1"))

	// Another deferral inside the hour must not raise a second digest.
	r.d.timerClaim(context.Background(), got)
	assert.Len(t, r.notifier.all(), 1)
}

func TestRateGate_should_DeferAcrossOwners_When_GlobalLimitReached(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Limits = LimitSettings{FiresGlobalHour: 3} })
	rows := make([]*session.WakeTriggerData, 0, 4)
	for i := range 4 {
		owner := fmt.Sprintf("owner-g%d", i)
		r.steerer.setReady(owner, true)
		rows = append(rows, newRow(t, r.repo, rowOpts{owner: owner}))
	}

	for _, row := range rows {
		r.d.timerClaim(context.Background(), row)
	}

	for _, row := range rows[:3] {
		assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	}
	assert.Equal(t, domain.WakeStatusPending, r.status(rows[3].ID))
	assert.EqualValues(t, 1, r.d.Metrics().Value(MetricLimitHit, LimitFiresGlobalHour, PhaseFire))
}

func TestRateGate_should_SkipOneShot_When_ThirdConsecutiveDeferral(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Limits = LimitSettings{FiresPerOwnerHour: 1} })
	first := newRow(t, r.repo, rowOpts{payload: "uses the only slot"})
	r.d.timerClaim(context.Background(), first)
	victim := newRow(t, r.repo, rowOpts{payload: "starved"})

	for range 3 {
		got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(victim.ID))
		require.NoError(t, err)
		r.d.timerClaim(context.Background(), got)
	}

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(victim.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusSkipped, got.Status)
	assert.Equal(t, domain.WakeReasonRateLimited, got.EndReason)
	assert.Len(t, r.notifier.all(), 2, "one deferral digest plus the skip notification")
}

func TestRateGate_should_SkipOnlyTheOccurrence_When_CronRowIsRateLimited(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Limits = LimitSettings{FiresPerOwnerHour: 1} })
	r.d.timerClaim(context.Background(), newRow(t, r.repo, rowOpts{payload: "uses the only slot"}))
	due := t0
	cronRow := newRow(t, r.repo, rowOpts{spec: domain.CronSpec{Expr: "*/10 * * * *"}, nextFire: &due, maxFires: 5})

	r.d.timerClaim(context.Background(), cronRow)

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(cronRow.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusPending, got.Status)
	assert.Equal(t, 0, got.FireCount)
	assert.Equal(t, 0, got.RateLimitDeferrals)
	require.NotNil(t, got.NextFireAt)
	assert.True(t, got.NextFireAt.After(t0))
	assert.Zero(t, got.NextFireAt.Minute()%10)
}

func TestRateGate_should_NotCountNotifyOperatorWakes(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Limits = LimitSettings{FiresPerOwnerHour: 1} })
	for i := range 4 {
		row := newRow(t, r.repo, rowOpts{action: domain.WakeActionNotifyOperator, payload: fmt.Sprintf("remind %d", i)})
		r.d.timerClaim(context.Background(), row)
		assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	}
	assert.Zero(t, r.d.DeliveredPerOwnerHour("owner-1"))
}

func TestRateGate_should_ReleaseSlot_When_ClaimEndsSkipped(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t, func(c *Config) { c.Limits = LimitSettings{FiresPerOwnerHour: 1} })
	r.owners.set("owner-1", OwnerFacts{Exists: true, Live: true, Archived: true})
	skipped := newRow(t, r.repo, rowOpts{payload: "archived owner"})
	r.d.timerClaim(context.Background(), skipped)
	require.Equal(t, domain.WakeStatusSkipped, r.status(skipped.ID))
	assert.Zero(t, r.d.DeliveredPerOwnerHour("owner-1"), "a skipped claim gives its slot back")
}

func TestMetrics_should_ObserveFireLag_When_ATimeTriggerIsClaimedLate(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	due := t0.Add(-90 * time.Second)
	row := newRow(t, r.repo, rowOpts{owner: "owner-1", payload: "late", spec: domain.AtSpec{At: due}, nextFire: &due})

	r.d.timerClaim(context.Background(), row)

	assert.Equal(t, []float64{90}, r.d.Metrics().FireLags("at"), "wake_fire_lag_seconds{type=at}")
}

// A flag that was off while rows came due re-enables through the same rate gate
// as any other fire (plan 4.3, pre-mortem #6): no bypass, one digest.
func TestReconcile_should_DeliverAtMostTheOwnerLimitAndDigestTheRest_When_ThirtyOverdueRowsAreReEnabled(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	rows := make([]*session.WakeTriggerData, 30)
	for i := range rows {
		due := t0.Add(-time.Duration(60-i) * time.Minute)
		rows[i] = newRow(t, r.repo, rowOpts{owner: "owner-1", payload: fmt.Sprintf("overdue %d", i), spec: domain.AtSpec{At: due}, nextFire: &due})
	}

	r.d.reconcileStartup(context.Background())

	delivered, pending := 0, 0
	for _, row := range rows {
		switch r.status(row.ID) {
		case domain.WakeStatusDelivered:
			delivered++
		case domain.WakeStatusPending:
			pending++
		}
	}
	assert.Equal(t, 6, delivered, "the per-owner hourly limit")
	assert.Equal(t, 24, pending, "the rest are deferred, not lost and not fired")
	assert.Equal(t, 6, r.d.DeliveredPerOwnerHour("owner-1"))
	assert.Len(t, r.steerer.pane.written(), 6)
	notes := r.notifier.all()
	require.Len(t, notes, 1, "exactly one failure-class digest for the owner")
	assert.Equal(t, NotifyFailure, notes[0].Kind)
}
