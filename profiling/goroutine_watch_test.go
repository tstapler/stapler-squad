package profiling

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestGoroutineWatch_should_StayOK_When_CountMatchesTheSessionScaledBudget(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	v := w.Observe(t0, 615, 30)
	assert.Equal(t, GoroutinesOK, v.State)
	assert.False(t, v.Degraded())
}

func TestGoroutineWatch_should_StayOK_AtTheMeasuredIdleAndLiveCounts(t *testing.T) {
	t.Parallel()
	// 2026-11-09: a fresh idle instance ran 149-156 goroutines; the live service 575-579 with 19 active sessions.
	assert.Equal(t, GoroutinesOK, (&GoroutineWatch{}).Observe(t0, 156, 0).State)
	assert.Equal(t, GoroutinesOK, (&GoroutineWatch{}).Observe(t0, 579, 19).State)
}

func TestGoroutineWatch_should_FlagASmallFleetLeak_OnceItIsSustained(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	assert.Equal(t, GoroutinesElevated, w.Observe(t0, 500, 3).State)
	assert.Equal(t, GoroutinesElevated, w.Observe(t0.Add(29*time.Minute), 520, 3).State, "not yet sustained")
	v := w.Observe(t0.Add(30*time.Minute), 530, 3)
	assert.Equal(t, GoroutinesSustained, v.State)
	assert.True(t, v.Degraded())
	assert.Equal(t, 30*time.Minute, v.ElevatedFor)
}

func TestGoroutineWatch_should_NotDegrade_When_ASpikeSettlesBeforeTheSustainWindow(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	require.Equal(t, GoroutinesElevated, w.Observe(t0, 900, 3).State)
	require.Equal(t, GoroutinesElevated, w.Observe(t0.Add(20*time.Minute), 150, 3).State, "one low sample does not clear it")
	require.Equal(t, GoroutinesOK, w.Observe(t0.Add(25*time.Minute), 150, 3).State, "settled for the clear window")

	// The clock restarts: another spike starts from zero, not from the first one.
	v := w.Observe(t0.Add(40*time.Minute), 900, 3)
	assert.Equal(t, GoroutinesElevated, v.State)
	assert.Zero(t, v.ElevatedFor)
}

func TestGoroutineWatch_should_ScaleTheBudgetWithSessions_SoALargeFleetIsNotALeak(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	// 2500 goroutines is over the old absolute 2000 but normal for 100 sessions.
	v := w.Observe(t0, 2500, 100)
	assert.Equal(t, GoroutinesOK, v.State)
	v = w.Observe(t0.Add(time.Hour), 2500, 100)
	assert.Equal(t, GoroutinesOK, v.State)
}

func TestGoroutineWatch_should_ReportImmediately_When_OverTheHardCeiling(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	v := w.Observe(t0, HighGoroutineThreshold+1, 0)
	assert.Equal(t, GoroutinesOverCeiling, v.State)
	assert.True(t, v.Degraded())
}

func TestGoroutineWatch_should_RaiseTheCeiling_WithSessionCount(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	expected := ExpectedGoroutines(200)
	v := w.Observe(t0, expected*2, 200)
	assert.NotEqual(t, GoroutinesOverCeiling, v.State, "ceiling is twice the expectation once that exceeds the floor")
	v = w.Observe(t0, expected*2+1, 200)
	assert.Equal(t, GoroutinesOverCeiling, v.State)
}

func TestGoroutineWatch_Last_should_ReturnTheLatestVerdict(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	assert.Equal(t, GoroutineState(""), w.Last().State, "no sample yet")
	w.Observe(t0, 100, 1)
	assert.Equal(t, 100, w.Last().Count)
}

func TestRunGoroutineWatch_should_SampleOnEveryTick_AndStopWithTheContext(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	ticks := make(chan time.Time)
	var count atomic.Int64
	count.Store(100)
	clock := t0
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		runGoroutineWatch(ctx, w, ticks, func() time.Time { return clock }, func() int { return int(count.Load()) }, func() (int, bool) { return 3, true })
	}()

	ticks <- clock
	ticks <- clock // unbuffered: the first sample is complete before this send returns
	assert.Equal(t, GoroutinesOK, w.Last().State)
	count.Store(900)
	ticks <- clock
	ticks <- clock
	assert.Equal(t, GoroutinesElevated, w.Last().State)
	assert.Equal(t, 3, w.Last().Sessions)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not stop with its context")
	}
}

func TestGoroutineWatch_should_StillReachSustained_When_ALeakDipsBelowTheLimitBriefly(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	w.Observe(t0, 900, 3)
	w.Observe(t0.Add(10*time.Minute), 150, 3) // one dip, shorter than the clear window
	w.Observe(t0.Add(12*time.Minute), 900, 3)

	v := w.Observe(t0.Add(30*time.Minute), 900, 3)

	assert.Equal(t, GoroutinesSustained, v.State)
}

func TestExpectedGoroutines_should_TreatNegativeSessionsAsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ExpectedGoroutines(0), ExpectedGoroutines(-4))
}

func TestGoroutineWatch_should_JudgeTheBoundaryExclusively(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	limit := goroutineLimit(ExpectedGoroutines(2))
	assert.Equal(t, GoroutinesOK, w.Observe(t0, limit, 2).State, "at the limit is within budget")
	assert.Equal(t, GoroutinesElevated, w.Observe(t0, limit+1, 2).State)
	assert.Equal(t, GoroutinesElevated, (&GoroutineWatch{}).Observe(t0, HighGoroutineThreshold, 0).State, "at the ceiling is not over it")
}

func TestRunGoroutineWatch_should_SkipTicks_WhileTheSessionCountIsUnknown(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	ticks := make(chan time.Time)
	var known atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runGoroutineWatch(ctx, w, ticks, func() time.Time { return t0 }, func() int { return 5000 },
		func() (int, bool) { return 0, known.Load() })

	ticks <- t0
	ticks <- t0 // the first tick has been fully handled once this send returns
	assert.Equal(t, GoroutineState(""), w.Last().State, "an unpopulated poller must not be judged against an empty budget")

	known.Store(true)
	ticks <- t0
	ticks <- t0
	assert.Equal(t, GoroutinesOverCeiling, w.Last().State)
}

// feed samples the watch every 30s for d, with count(minutes) giving the goroutine count.
func feed(w *GoroutineWatch, d time.Duration, sessions int, count func(min float64) int) GoroutineVerdict {
	var v GoroutineVerdict
	for el := time.Duration(0); el <= d; el += 30 * time.Second {
		v = w.Observe(t0.Add(el), count(el.Minutes()), sessions)
	}
	return v
}

func TestGoroutineWatch_should_FlagADrift_WhenTheCountClimbsSteadilyInsideItsLimit(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	// 150/h from 300 stays under the 1.5x limit (1500 at 20 sessions) for the whole window.
	v := feed(w, 90*time.Minute, 20, func(m float64) int { return 700 + int(m*2.5) })

	assert.Equal(t, GoroutinesDrifting, v.State)
	assert.InDelta(t, 150, v.DriftPerHour, 5)
	assert.True(t, v.Degraded())
}

func TestGoroutineWatch_should_NotFlagADrift_BeforeAnHourOfHistory(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	v := feed(w, 50*time.Minute, 20, func(m float64) int { return 700 + int(m*2.5) })
	assert.Equal(t, GoroutinesOK, v.State)
}

func TestGoroutineWatch_should_NotFlagADrift_ForANoisyPlateau(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	v := feed(w, 2*time.Hour, 20, func(m float64) int { return 700 + int(m)%7*15 })
	assert.Equal(t, GoroutinesOK, v.State)
}

func TestGoroutineWatch_should_NotFlagADrift_ForASlowClimbBelowTheRate(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	v := feed(w, 2*time.Hour, 20, func(m float64) int { return 700 + int(m/6) }) // 10/h
	assert.Equal(t, GoroutinesOK, v.State)
}

func TestGoroutineWatch_should_NotFlagADrift_WhenOnlyTheSessionCountGrows(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	var v GoroutineVerdict
	for el := time.Duration(0); el <= 90*time.Minute; el += 30 * time.Second {
		sessions := 10 + int(el.Minutes()/9) // +1 session per 9 minutes, ~25 goroutines each
		v = w.Observe(t0.Add(el), 400+25*sessions, sessions)
	}
	assert.Equal(t, GoroutinesOK, v.State, "growth explained by sessions is not a leak")
}

func TestGoroutineWatch_should_ForgetOldSamples_SoARecoveredLeakStopsDrifting(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	feed(w, 90*time.Minute, 20, func(m float64) int { return 700 + int(m*2.5) })
	var v GoroutineVerdict
	for el := 90 * time.Minute; el <= 4*time.Hour; el += 30 * time.Second {
		v = w.Observe(t0.Add(el), 925, 20) // leak fixed: flat
	}
	assert.Equal(t, GoroutinesOK, v.State)
}
