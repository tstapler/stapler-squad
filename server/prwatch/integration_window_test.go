package prwatch

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

func windowLoop(t *testing.T, mut func(*httpLoop)) *httpLoop {
	t.Helper()
	return newHTTPLoop(t, func(l *httpLoop) {
		l.realCoalescer = true
		l.settings.MinSteerInterval = hWindowClose
		if mut != nil {
			mut(l)
		}
	})
}

func heldLongSent(l *httpLoop) int {
	n := 0
	for _, s := range l.notifier.Sent() {
		if s.Kind == NotifyHeldLong {
			n++
		}
	}
	return n
}

func TestPollerOverHTTPInbox_should_DeliverOneSteerAtWindowClose_When_SecondCommentArrivesInsideWindow(t *testing.T) {
	l := windowLoop(t, nil)
	l.push(httpComment("A"))
	l.tick()
	require.Equal(t, 1, l.steers())
	require.Equal(t, []string{"A"}, l.fake.Acked())

	l.advanceTick(hStepPoll)
	l.push(httpComment("B"))
	for elapsed := 2 * hStepPoll; elapsed < hWindowClose; elapsed += hStepPoll {
		l.advanceTick(hStepPoll)
		require.Equal(t, []string{"B"}, l.fake.Pending(), "B stays pending at +%s", elapsed)
	}
	assert.Equal(t, 1, l.steers(), "nothing steered inside the window")

	l.advanceTick(hStepPoll) // window closes
	assert.Equal(t, 2, l.steers())
	assert.Empty(t, l.fake.Pending())
	assert.Zero(t, heldLongSent(l), "window holds are far shorter than the HeldLong threshold")
}

func TestPollerOverHTTPInbox_should_BoundWindowHoldsByCapAndInterval_When_FourKeysFloodAtOnce(t *testing.T) {
	const floodKeys, perKey = 4, 100
	l := windowLoop(t, nil)
	refs := make([]SessionRef, 0, floodKeys+1)
	for k := range floodKeys + 1 {
		uuid := fmt.Sprintf("flood-%d", k)
		l.sessions.programs[uuid] = "claude"
		refs = append(refs, SessionRef{UUID: uuid, Repo: dRepo, PR: 20 + k, State: StateLive})
	}
	l.sessions.Set(refs...)
	floodItem := func(id string, k int) inbox.Item {
		it := httpComment(id)
		it.Event.PullRequest = 20 + k
		return it
	}

	// One steer per flooded PR opens its window.
	for k := range floodKeys {
		l.push(floodItem(fmt.Sprintf("open-%d", k), k))
	}
	l.tick()
	require.Equal(t, floodKeys, l.steers())

	for k := range floodKeys {
		for i := range perKey {
			l.push(floodItem(fmt.Sprintf("f%d-%03d", k, i), k))
		}
	}
	newer := l.push(floodItem("fifth", floodKeys))

	l.advanceTick(hStepPoll)
	// The oldest 200 pending items are all that one tick can see: three keys
	// are fully processed (50 held, 50 folded over the cap) and the fourth
	// shows its first 50.
	folded := l.metricLabelled(MetricDropped, "drop_reason", string(DropFoldedOverCap))
	assert.Equal(t, 3*(perKey-MaxHeldPerKey), folded)
	assert.Contains(t, l.fake.Pending(), newer.ID, "the newer PR is hidden behind the held page")
	assert.Equal(t, floodKeys, l.steers(), "no steer while windows are open")

	l.advanceTick(hWindowClose)
	assert.NotContains(t, l.fake.Pending(), newer.ID, "the newer PR is steered once the windows close")
	steered := map[string]bool{}
	for _, r := range l.steerer.Requests() {
		steered[r.SessionUUID] = true
	}
	assert.True(t, steered[fmt.Sprintf("flood-%d", floodKeys)])
	assert.Equal(t, 0, heldLongSent(l))
}

func TestPollerOverHTTPInbox_should_DeliverHeldWindowItemsOnFirstTick_When_ProcessRestartsInsideWindow(t *testing.T) {
	l := windowLoop(t, nil)
	l.push(httpComment("A"))
	l.tick()
	l.advanceTick(hStepPoll)
	l.push(httpComment("B"))
	l.advanceTick(hStepPoll)
	require.Equal(t, 1, l.steers())
	require.Equal(t, []string{"B"}, l.fake.Pending())

	l.build() // restart inside the window: window state is lost, B is still in the inbox
	l.advanceTick(hStepPoll)
	assert.Equal(t, 2, l.steers(), "exactly one folded steer on the first tick")
	assert.Empty(t, l.fake.Pending())
	l.advanceTick(hStepPoll)
	assert.Equal(t, 2, l.steers())
}

func TestPollerOverHTTPInbox_should_HoldLiveEventUntilWindowCloses_When_CatchUpSteerJustDelivered(t *testing.T) {
	var snaps *fakePRSnapshot
	l := windowLoop(t, func(l *httpLoop) {
		snaps = &fakePRSnapshot{snapshots: map[string]PRSnapshot{
			dSession: {ChangesRequested: 1, CheckedAt: l.clock.Now().Add(time.Second)},
		}}
		l.snapshots = snaps
	})
	// The snapshot is newer than "never polled", so the first subscription's
	// catch-up steers on the subscribing tick.
	l.push(httpComment("live"))
	l.tick()
	require.Equal(t, 1, l.steers(), "the catch-up steered and opened the human-feedback window")
	assert.Equal(t, 1, l.metrics.Count(MetricCatchupSteers))
	assert.Equal(t, []string{"live"}, l.fake.Pending(), "the live comment is held, not acked as covered")

	l.advanceTick(hWindowClose - hStepPoll)
	assert.Equal(t, 1, l.steers())
	l.advanceTick(hStepPoll)
	assert.Equal(t, 2, l.steers(), "one steer announces the held comment at window close")
	assert.Empty(t, l.fake.Pending())
}
