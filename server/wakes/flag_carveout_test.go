package wakes

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func scopeRig(t *testing.T, on *atomic.Bool) *dispatcherRig {
	t.Helper()
	return newDispatcherRig(t, func(c *Config) {
		c.TypeEnabled = func(domain.WakeTriggerType) bool { return on.Load() }
	})
}

// ADR-008: a pending row whose type scope is false is neither fired nor marked
// missed, except that expires_at always wins.
func TestScheduleWake_ShouldMarkPendingAtRowMissedAtExpiry_WhenTriggerScopeFalse(t *testing.T) {
	t.Parallel()
	var on atomic.Bool
	r := scopeRig(t, &on)
	ctx := context.Background()
	expires := t0.Add(time.Hour)
	row := newRow(t, r.repo, rowOpts{expiresAt: &expires})

	r.clock.Advance(time.Minute) // the row is due, its scope is false
	assert.False(t, r.d.timer.FireDue(ctx), "a held row makes the loop back off instead of spinning")
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID), "neither fired nor missed while the scope is false")
	assert.Empty(t, r.steerer.pane.written())

	r.clock.Advance(2 * time.Hour) // past expires_at, scope still false
	r.d.timer.FireDue(ctx)
	assert.Equal(t, domain.WakeStatusMissed, r.status(row.ID), "expires_at wins over the false scope")
	assert.Empty(t, r.steerer.pane.written())
	assert.Len(t, r.notifier.all(), 1, "the operator hears it was missed")
}

func TestScheduleWake_ShouldFireHeldAtRow_When_ScopeTurnsTrueBeforeExpiry(t *testing.T) {
	t.Parallel()
	var on atomic.Bool
	r := scopeRig(t, &on)
	ctx := context.Background()
	expires := t0.Add(time.Hour)
	row := newRow(t, r.repo, rowOpts{expiresAt: &expires})

	r.clock.Advance(time.Minute)
	r.d.timer.FireDue(ctx)
	require.Equal(t, domain.WakeStatusPending, r.status(row.ID))

	on.Store(true)
	assert.True(t, r.d.timer.FireDue(ctx))
	r.eventually(func() bool { return r.status(row.ID) == domain.WakeStatusDelivered })
	assert.Len(t, r.steerer.pane.written(), 1, "fired exactly once after the scope came back")
}

func TestScheduleWake_ShouldMarkPendingEventRowMissedAtExpiry_WhenTriggerScopeFalse(t *testing.T) {
	t.Parallel()
	var on atomic.Bool
	r := scopeRig(t, &on)
	ctx := context.Background()
	expires := t0.Add(time.Hour)
	r.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}
	row := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{expiresAt: &expires})

	r.d.reindex(ctx)
	r.d.fireHeldConditions(ctx) // the condition holds, the scope is false
	r.d.fireCell(ctx, EventMatch{Type: domain.WakeTriggerBacklogEvent, Key: itemX, Kinds: []string{"verdict_recorded"}}, "")
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID))
	assert.Empty(t, r.steerer.pane.written())

	r.clock.Advance(2 * time.Hour)
	r.d.fireHeldConditions(ctx) // the periodic reconcile
	assert.Equal(t, domain.WakeStatusMissed, r.status(row.ID))
	assert.Empty(t, r.steerer.pane.written())
}

func TestScheduleWake_ShouldNotDebounceAwayTheFire_When_EventSeenWhileScopeFalse(t *testing.T) {
	t.Parallel()
	var on atomic.Bool
	r := scopeRig(t, &on)
	ctx := context.Background()
	r.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}
	row := backlogRow(t, r, itemX, "verdict_recorded", rowOpts{})

	r.d.fireHeldConditions(ctx)
	on.Store(true)
	r.d.fireHeldConditions(ctx)

	r.eventually(func() bool { return r.status(row.ID) == domain.WakeStatusDelivered })
}
