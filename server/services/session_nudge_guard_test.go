package services

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGuardClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeGuardClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeGuardClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestGuard() (*sessionNudgeGuard, *fakeGuardClock) {
	clk := &fakeGuardClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return &sessionNudgeGuard{now: clk.Now}, clk
}

func TestTryBegin_should_ReturnBusyThenOKAfterRelease_When_SecondAttemptWhileInFlight(t *testing.T) {
	g, _ := newTestGuard()

	release, out := g.TryBegin("uuid-1", "sigA")
	require.Equal(t, GuardOK, out)

	_, out = g.TryBegin("uuid-1", "sigA")
	assert.Equal(t, GuardBusy, out)

	release(false)
	_, out = g.TryBegin("uuid-2", "sigA")
	assert.Equal(t, GuardOK, out, "other sessions are independent")

	// Failure cooldown elapsed is covered separately; a different signature is not blocked by it.
	_, out = g.TryBegin("uuid-1", "sigB")
	assert.Equal(t, GuardOK, out)
}

func TestTryBegin_should_ReturnDuplicateWithin60sAndOKAfter61sOrDifferentSignature_When_FakeClockAdvances(t *testing.T) {
	g, clk := newTestGuard()
	const sig = "FAILING_CHECKS|UNRESOLVED_THREADS"

	release, out := g.TryBegin("uuid-1", sig)
	require.Equal(t, GuardOK, out)
	release(true)

	clk.Advance(30 * time.Second)
	_, out = g.TryBegin("uuid-1", sig)
	assert.Equal(t, GuardDuplicate, out)

	r2, out := g.TryBegin("uuid-1", "MERGE_CONFLICT")
	assert.Equal(t, GuardOK, out, "a different signature is not suppressed")
	r2(false)

	clk.Advance(31 * time.Second)
	_, out = g.TryBegin("uuid-1", sig)
	assert.Equal(t, GuardOK, out, "61s after the delivery of sig")
}

func TestTryBegin_should_AllowExactlyOne_When_20GoroutinesRaceUnderRace(t *testing.T) {
	g, _ := newTestGuard()
	var ok, rejected atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, out := g.TryBegin("uuid-1", "sigA")
			if out == GuardOK {
				ok.Add(1)
			} else {
				rejected.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, ok.Load())
	assert.EqualValues(t, 19, rejected.Load())
}

func TestRelease_should_Record10sCooldownNot60s_When_WriteFailed(t *testing.T) {
	g, clk := newTestGuard()

	release, _ := g.TryBegin("uuid-1", "sigA")
	release(false)

	clk.Advance(5 * time.Second)
	_, out := g.TryBegin("uuid-1", "sigA")
	assert.Equal(t, GuardDuplicate, out, "inside the 10s cooldown")

	clk.Advance(6 * time.Second)
	_, out = g.TryBegin("uuid-1", "sigA")
	assert.Equal(t, GuardOK, out, "11s after the failed write, well inside 60s")
}

func TestRelease_should_BeIdempotent_When_CalledTwice(t *testing.T) {
	g, _ := newTestGuard()
	release, _ := g.TryBegin("uuid-1", "sigA")
	release(true)
	_, out := g.TryBegin("uuid-1", "sigB")
	require.Equal(t, GuardOK, out) // sigB now in flight

	release(true) // stale second release must not clear sigB's in-flight claim
	_, out = g.TryBegin("uuid-1", "sigC")
	assert.Equal(t, GuardBusy, out)
}

func TestTryBegin_should_CapRecordsAt1024_When_2000DistinctSessionsRecorded(t *testing.T) {
	g, clk := newTestGuard()
	for i := 0; i < 2000; i++ {
		release, out := g.TryBegin(fmt.Sprintf("uuid-%d", i), "sigA")
		require.Equal(t, GuardOK, out)
		release(true)
		clk.Advance(time.Millisecond)
	}

	_, out := g.TryBegin("uuid-next", "sigA")
	require.Equal(t, GuardOK, out)

	g.mu.Lock()
	defer g.mu.Unlock()
	assert.LessOrEqual(t, len(g.last), nudgeMaxRecords)
	_, oldestKept := g.last["uuid-0"]
	assert.False(t, oldestKept, "oldest record is the one dropped")
	_, newestKept := g.last["uuid-1999"]
	assert.True(t, newestKept)
}

func TestTryBegin_should_SweepRecordOlderThan10Minutes_When_NextTryBegin(t *testing.T) {
	g, clk := newTestGuard()
	release, _ := g.TryBegin("uuid-1", "sigA")
	release(true)

	clk.Advance(11 * time.Minute)
	_, out := g.TryBegin("uuid-2", "sigA")
	require.Equal(t, GuardOK, out)

	g.mu.Lock()
	defer g.mu.Unlock()
	_, stillThere := g.last["uuid-1"]
	assert.False(t, stillThere)
}
