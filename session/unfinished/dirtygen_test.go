package unfinished

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

// armedAfterFirstWalk returns a DirtyGen that is armed and has completed its first full walk.
func armedAfterFirstWalk(t *testing.T) (*DirtyGen, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	d := NewDirtyGen(clk.Now)
	d.Arm()
	dec := d.Decide()
	require.True(t, dec.FullWalk)
	d.CompleteFullWalk(dec)
	return d, clk
}

func TestDirtyGenPolicy_ShouldRunStatPass_WhenTierAArmedAndHealthy(t *testing.T) {
	d, _ := armedAfterFirstWalk(t)
	dec := d.Decide()
	assert.False(t, dec.FullWalk)
	assert.False(t, dec.DirtyHint, "a quiet watcher must not imply clean")
	assert.True(t, d.ShouldRunStatPass())
}

func TestDirtyGenPolicy_ShouldFullWalk_WhenWatcherNotArmedOrFirstScanAfterArming(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	d := NewDirtyGen(clk.Now)
	dec := d.Decide()
	assert.True(t, dec.FullWalk)
	assert.Equal(t, ReasonUnarmed, dec.Reason)

	d.Arm()
	dec = d.Decide()
	assert.Equal(t, ReasonUnarmed, dec.Reason, "first scan after arming")
	d.CompleteFullWalk(dec)
	assert.False(t, d.Decide().FullWalk)

	d.Disarm()
	assert.Equal(t, ReasonUnarmed, d.Decide().Reason)
}

func TestDirtyGenPolicy_ShouldFullWalk_When5MinElapsedOnInjectedClock(t *testing.T) {
	d, clk := armedAfterFirstWalk(t)
	clk.t = clk.t.Add(fullWalkMaxAge - time.Nanosecond)
	assert.False(t, d.Decide().FullWalk)
	clk.t = clk.t.Add(time.Nanosecond)
	dec := d.Decide()
	assert.True(t, dec.FullWalk)
	assert.Equal(t, ReasonAge, dec.Reason)
}

func TestDirtyGenPolicy_ShouldFullWalkAndCountReason_WhenOverflowErrorOrDirEvent(t *testing.T) {
	cases := []struct {
		kind   EventKind
		reason FullWalkReason
	}{
		{EventOverflow, ReasonOverflow},
		{EventError, ReasonError},
		{EventDirChange, ReasonDirEvent},
	}
	for _, c := range cases {
		t.Run(c.reason.String(), func(t *testing.T) {
			d, _ := armedAfterFirstWalk(t)
			before := DirtyFullWalkTotal(c.reason)
			g := d.Gen()
			d.Note(c.kind)
			assert.Greater(t, d.Gen(), g)

			dec := d.Decide()
			assert.True(t, dec.FullWalk)
			assert.Equal(t, c.reason, dec.Reason)
			assert.Equal(t, before+1, DirtyFullWalkTotal(c.reason))

			d.CompleteFullWalk(dec)
			assert.False(t, d.Decide().FullWalk, "walk clears the observed reason")
		})
	}
}

func TestDirtyGenPolicy_ShouldReturnDirtyWithoutFullWalk_WhenEventOnTrackedPath(t *testing.T) {
	d, _ := armedAfterFirstWalk(t)
	d.Note(EventTrackedPath)
	dec := d.Decide()
	assert.False(t, dec.FullWalk)
	assert.True(t, dec.DirtyHint)
}

func TestDirtyGenPolicy_ShouldKeepEventsArrivingDuringWalk(t *testing.T) {
	d, _ := armedAfterFirstWalk(t)
	d.Note(EventOverflow)
	dec := d.Decide()
	d.Note(EventDirChange) // lands mid-walk
	d.CompleteFullWalk(dec)
	assert.Equal(t, ReasonDirEvent, d.Decide().Reason)
}

func TestFullWalkReason_ShouldPrioritiseOverflowOverOthers(t *testing.T) {
	r := FullWalkFallback{MaxAge: time.Minute}.Evaluate(false, time.Time{}, time.Unix(100, 0), pendOverflow|pendError|pendDirEvent)
	assert.Equal(t, ReasonOverflow, r)
}

func TestDirtyGenPolicy_ShouldServeFromCache_WhenScansSpacedAtScanInterval(t *testing.T) {
	d, clk := armedAfterFirstWalk(t)
	for i := 1; i <= 2; i++ { // at least two scheduled scans per full walk
		clk.t = clk.t.Add(defaultScanTickInterval)
		dec := d.Decide()
		assert.False(t, dec.FullWalk, "scan %d ticks after the last full walk is still inside the staleness bound", i)
	}
	clk.t = clk.t.Add(fullWalkMaxAge)
	assert.Equal(t, ReasonAge, d.Decide().Reason, "the staleness bound still holds")
}

func TestDirtyGenPolicy_ShouldNotRefireUnarmed_WhenArmedAgainAfterFirstWalk(t *testing.T) {
	d, _ := armedAfterFirstWalk(t)
	gen := d.Gen()
	d.Arm() // watchRepo re-registers on every AddRepo and ref event
	assert.False(t, d.Decide().FullWalk, "an unbroken watch is not a new arming")
	assert.Equal(t, gen, d.Gen())

	d.Disarm()
	d.Arm()
	assert.Equal(t, ReasonUnarmed, d.Decide().Reason, "a real disarm/arm cycle still needs a fresh walk")
}

func TestDirtyGenPolicy_CompleteIndexWalk_ShouldClearAgeButKeepEventReasons(t *testing.T) {
	d, clk := armedAfterFirstWalk(t)
	clk.t = clk.t.Add(fullWalkMaxAge)
	dec := d.Decide()
	require.Equal(t, ReasonAge, dec.Reason)
	d.CompleteIndexWalk(dec)
	assert.False(t, d.Decide().FullWalk, "an index-only walk restarts the age clock")

	d.Note(EventDirChange)
	dec = d.Decide()
	require.Equal(t, ReasonDirEvent, dec.Reason)
	d.CompleteIndexWalk(dec)
	assert.Equal(t, ReasonDirEvent, d.Decide().Reason, "a directory event stays pending until the untracked set was walked")
}
