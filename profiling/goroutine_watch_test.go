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
	// Measured steady state: 615 goroutines with ~30 sessions.
	v := w.Observe(t0, 615, 30)
	assert.Equal(t, GoroutinesOK, v.State)
	assert.False(t, v.Degraded())
}

func TestGoroutineWatch_should_FlagBug034ScaleLeak_When_FewSessionsHaveHundredsOfGoroutines(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	// BUG-034 leaked to 310-320 goroutines with a handful of sessions; the old 2000 threshold missed it.
	assert.Equal(t, GoroutinesElevated, w.Observe(t0, 315, 3).State)
	assert.Equal(t, GoroutinesElevated, w.Observe(t0.Add(29*time.Minute), 320, 3).State, "not yet sustained")
	v := w.Observe(t0.Add(30*time.Minute), 330, 3)
	assert.Equal(t, GoroutinesSustained, v.State)
	assert.True(t, v.Degraded())
	assert.Equal(t, 30*time.Minute, v.ElevatedFor)
}

func TestGoroutineWatch_should_NotDegrade_When_ASpikeSettlesBeforeTheSustainWindow(t *testing.T) {
	t.Parallel()
	w := &GoroutineWatch{}
	require.Equal(t, GoroutinesElevated, w.Observe(t0, 900, 3).State)
	require.Equal(t, GoroutinesOK, w.Observe(t0.Add(20*time.Minute), 150, 3).State)

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
		runGoroutineWatch(ctx, w, ticks, func() time.Time { return clock }, func() int { return int(count.Load()) }, func() int { return 3 })
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
