package wakes

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// crashBeforeMark simulates a process kill between the pane write and
// MarkWakeDelivered: the write and the attempt record happen, the row stays claimed.
type crashBeforeMark struct {
	*session.EntRepository
	crash atomic.Bool
}

func (c *crashBeforeMark) MarkWakeDelivered(ctx context.Context, id uuid.UUID, now, next time.Time) (bool, error) {
	if c.crash.Load() {
		return false, errBoom
	}
	return c.EntRepository.MarkWakeDelivered(ctx, id, now, next)
}

func wakeIDOf(t *testing.T, text string) string {
	t.Helper()
	const marker = `<wake id="`
	require.True(t, strings.HasPrefix(text, marker))
	return text[len(marker) : len(marker)+36]
}

func TestSteerDelivery_should_CountDuplicate_When_RestartResendsAfterCrashBeforeMarkDelivered(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	crashing := &crashBeforeMark{EntRepository: r.repo}
	crashing.crash.Store(true)
	first := r.newDispatcher(func(c *Config) { c.Store = crashing })
	row := newRow(t, r.repo, rowOpts{})

	first.timerClaim(context.Background(), row)
	require.Len(t, r.steerer.pane.written(), 1, "written, but the process died before MarkWakeDelivered")
	require.Equal(t, domain.WakeStatusClaimed, r.status(row.ID))

	r.clock.Advance(3 * time.Minute)
	restarted := r.newDispatcher()
	restarted.reconcileStartup(context.Background())

	writes := r.steerer.pane.written()
	require.Len(t, writes, 2)
	assert.Equal(t, wakeIDOf(t, writes[0]), wakeIDOf(t, writes[1]), "the second payload carries the same id")
	assert.EqualValues(t, 1, restarted.Metrics().Value(MetricDuplicate))
	assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	assert.Contains(t, writes[1], "If you have already acted on wake id "+row.ID+", do nothing.")
}

func TestSteerDelivery_should_NotCountDuplicate_When_FirstWriteSucceeds(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.d.timerClaim(context.Background(), newRow(t, r.repo, rowOpts{}))
	assert.Zero(t, r.d.Metrics().Value(MetricDuplicate))
}

func TestSteerDelivery_should_MarkMissedOwnerBusyTimeout_When_HeldPastBusyHold(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.steerer.setReady("owner-1", false)
	row := newRow(t, r.repo, rowOpts{})
	r.d.timerClaim(context.Background(), row)
	require.True(t, r.d.Delivery().Holds(row.ID))

	r.clock.Advance(29 * time.Minute)
	r.d.Delivery().AttemptAll(context.Background())
	assert.Equal(t, domain.WakeStatusClaimed, r.status(row.ID), "still inside the 30 minute hold")

	r.clock.Advance(2 * time.Minute) // 31 minutes
	r.d.Delivery().AttemptAll(context.Background())

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusMissed, got.Status)
	assert.Equal(t, domain.WakeReasonOwnerBusyTimeout, got.EndReason)
	notes := r.notifier.all()
	require.Len(t, notes, 1)
	assert.Equal(t, NotifyFailure, notes[0].Kind)
	assert.Contains(t, notes[0].Message, string(domain.WakeReasonOwnerBusyTimeout))
}

func TestSteerDelivery_should_DeliverDespiteHold_When_OwnerReadyAfterBusyHold(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.steerer.setReady("owner-1", false)
	row := newRow(t, r.repo, rowOpts{})
	r.d.timerClaim(context.Background(), row)

	r.clock.Advance(45 * time.Minute)
	r.steerer.setReady("owner-1", true)
	r.d.Delivery().AttemptAll(context.Background())

	assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID), "the hold bounds waiting for a busy owner, not delivery to a ready one")
}

func TestSteerDelivery_should_PreferExpiry_When_ExpiresBeforeBusyHold(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	r.steerer.setReady("owner-1", false)
	exp := t0.Add(10 * time.Minute)
	row := newRow(t, r.repo, rowOpts{expiresAt: &exp})
	r.d.timerClaim(context.Background(), row)

	r.clock.Advance(31 * time.Minute)
	r.d.Delivery().AttemptAll(context.Background())

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeReasonExpired, got.EndReason, "never held past expires_at")
}

// Pinned: Admit counts the owner's own in_progress item, so its revive is
// refused for as long as the item stays in progress. The row must not sit
// silently: it stays needs_revive, an alert fires after an hour, and
// expires_at then ends it as missed.
func TestReviveWaiting_should_AlertAtOneHourThenMiss_When_AdmissionKeepsRefusing(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t) // admission refuses by default
	target := &spyTarget{}
	r.owners.set("owner-1", OwnerFacts{Exists: true, Revive: target})
	exp := t0.Add(24 * time.Hour)
	row := newRow(t, r.repo, rowOpts{allowRevive: true, expiresAt: &exp})
	r.start(r.d)
	r.d.timerClaim(context.Background(), row)
	r.d.WaitIdle()
	require.Equal(t, domain.WakeStatusNeedsRevive, r.status(row.ID))

	r.clock.Advance(61 * time.Minute)
	r.d.Reconcile(context.Background())
	r.d.WaitIdle()
	r.d.Reconcile(context.Background())
	r.d.WaitIdle()

	assert.Equal(t, domain.WakeStatusNeedsRevive, r.status(row.ID))
	assert.Greater(t, r.d.Metrics().Gauge(GaugeNeedsReviveAge), 3600.0)
	notes := r.notifier.all()
	require.Len(t, notes, 1, "one alert, however many passes see the stale row")
	assert.Equal(t, NotifyFailure, notes[0].Kind)
	recovered, started := target.calls()
	assert.Zero(t, recovered+started)

	r.clock.Advance(24 * time.Hour)
	r.d.Reconcile(context.Background())
	r.d.WaitIdle()
	assert.Equal(t, domain.WakeStatusMissed, r.status(row.ID))
}
