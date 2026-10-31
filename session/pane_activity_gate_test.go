package session

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paneTestClock is a manually advanced clock; no real time passes in these tests.
type paneTestClock struct{ t time.Time }

func (c *paneTestClock) now() time.Time          { return c.t }
func (c *paneTestClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakePane models a tmux pane: output bumps pane_last_activity to the current whole second.
type fakePane struct {
	clock    *paneTestClock
	content  string
	activity time.Time
	captures int
}

func (p *fakePane) write(s string) {
	p.content += s
	p.activity = time.Unix(p.clock.now().Unix(), 0)
}

func (p *fakePane) capture(context.Context) (string, error) {
	p.captures++
	return p.content, nil
}

func newTestPane() (*fakePane, *paneTestClock) {
	c := &paneTestClock{t: time.Unix(1_000_000, 0)}
	p := &fakePane{clock: c}
	p.write("boot\n")
	return p, c
}

func gateFor(p *fakePane, snap func() map[string]time.Time) *paneActivityGate {
	return newPaneActivityGate(snap, p.clock.now, func() bool { return true })
}

func freshSnapshot(p *fakePane, name string) func() map[string]time.Time {
	return func() map[string]time.Time { return map[string]time.Time{name: p.activity} }
}

// Parity: with a fresh snapshot the gate returns exactly what an always-capture driver sees, for
// a random schedule of output bursts (including several writes inside one second).
func TestPaneActivityGate_ParityWithAlwaysCapture(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := gateFor(p, freshSnapshot(p, "s"))
	rng := rand.New(rand.NewSource(7))

	ticks := 0
	for step := 0; step < 4000; step++ {
		clock.advance(100 * time.Millisecond)
		if rng.Intn(25) == 0 {
			p.write(fmt.Sprintf("out%d\n", step))
		}
		if step%20 == 0 { // 2 s driver tick
			ticks++
			got, err := g.preview(context.Background(), "s", p.capture)
			require.NoError(t, err)
			require.Equal(t, p.content, got, "step %d", step)
		}
	}
	assert.Less(t, p.captures, ticks, "an idle pane must not be captured every tick")
}

// ttlPane is a fakePane whose capture is served from a capturePaneCacheTTL-old cache, like
// TmuxProcessManager.CapturePaneContentContext.
type ttlPane struct {
	*fakePane
	cached   string
	cachedAt time.Time
	hasCache bool
}

func (p *ttlPane) capture(ctx context.Context) (string, error) {
	if !p.hasCache || p.clock.now().Sub(p.cachedAt) >= capturePaneCacheTTL {
		p.cached, p.cachedAt, p.hasCache = p.content, p.clock.now(), true
		p.captures++
	}
	return p.cached, nil
}

// Parity against an always-capture driver on an identical pane, over every combination of output
// offset, other-consumer capture offset and tick phase (100 ms steps). The other consumer warms the
// capture cache just before the output, so a capture can show content older than its start time.
func TestPaneActivityGate_ParityUnderCaptureCacheStaleness(t *testing.T) {
	t.Parallel()
	const step = 100 * time.Millisecond
	for write := 0; write < 20; write++ {
		for warm := 0; warm <= write; warm++ {
			for phase := 0; phase < 10; phase++ {
				gated, plain := newTTLPane(), newTTLPane()
				g := gateFor(gated.fakePane, freshSnapshot(gated.fakePane, "s"))
				for i := 0; i < 80; i++ { // 8 s
					for _, p := range []*ttlPane{gated, plain} {
						p.clock.advance(step)
					}
					if i == warm {
						gated.capture(context.Background())
						plain.capture(context.Background())
					}
					if i == write {
						gated.write("dialog\n")
						plain.write("dialog\n")
					}
					if i >= phase && (i-phase)%20 == 0 {
						got, _ := g.preview(context.Background(), "s", gated.capture)
						want, _ := plain.capture(context.Background())
						require.Equal(t, want, got, "write=%d warm=%d phase=%d tick@%d", write, warm, phase, i)
					}
				}
			}
		}
	}
}

func newTTLPane() *ttlPane {
	p, _ := newTestPane()
	return &ttlPane{fakePane: p}
}

func TestPaneActivityGate_EmptyContentIsNotCached(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := gateFor(p, freshSnapshot(p, "s"))
	clock.advance(3 * time.Second)
	empty := func(context.Context) (string, error) { p.captures++; return "", nil }

	_, _ = g.preview(context.Background(), "s", empty) // e.g. previewBlocked on a paused instance
	got, _ := g.preview(context.Background(), "s", p.capture)
	assert.Equal(t, p.content, got)
	assert.Equal(t, 2, p.captures)
}

func TestDriverPaneSnapshot_RemoteInstanceGetsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, driverPaneSnapshot(true))
}

func TestCachedFlag_ReadsOncePerTTL(t *testing.T) {
	t.Parallel()
	clock := &paneTestClock{t: time.Unix(1_000_000, 0)}
	reads, value := 0, true
	f := &cachedFlag{read: func() bool { reads++; return value }, ttl: 5 * time.Second, now: clock.now}

	assert.True(t, f.get())
	value = false
	clock.advance(4 * time.Second)
	assert.True(t, f.get(), "still inside the TTL")
	clock.advance(time.Second)
	assert.False(t, f.get(), "TTL elapsed, re-read")
	assert.Equal(t, 2, reads)
}

// Concurrent callers coalesce into one fetch (run under -race).
func TestPaneActivityCache_ConcurrentCallersCoalesce(t *testing.T) {
	t.Parallel()
	clock := &paneTestClock{t: time.Unix(1_000_000, 0)}
	var fetches atomic.Int32
	cache := newPaneActivityCache(func() map[string]time.Time { fetches.Add(1); return nil }, clock.now)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); cache.get(paneSnapshotMaxAgeDriver) }()
	}
	wg.Wait()
	assert.EqualValues(t, 1, fetches.Load())
}

func TestPaneActivityGate_IdlePaneCapturesOnceUntilMaxAge(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := gateFor(p, freshSnapshot(p, "s"))
	clock.advance(3 * time.Second) // capture lands in a later second than the last output

	for i := 0; i < 10; i++ {
		_, err := g.preview(context.Background(), "s", p.capture)
		require.NoError(t, err)
		clock.advance(2 * time.Second)
	}
	assert.Equal(t, 1, p.captures)

	clock.advance(paneGateMaxCaptureAge)
	_, err := g.preview(context.Background(), "s", p.capture)
	require.NoError(t, err)
	assert.Equal(t, 2, p.captures, "forced recapture after max age")
}

// A capture taken in the same second as the last output cannot prove later output will move the
// activity, so the next tick must capture again.
func TestPaneActivityGate_SameSecondCaptureIsNotTrusted(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := gateFor(p, freshSnapshot(p, "s"))

	_, _ = g.preview(context.Background(), "s", p.capture)
	p.write("late-in-same-second\n") // same whole second, same pane_last_activity
	got, _ := g.preview(context.Background(), "s", p.capture)
	assert.Equal(t, p.content, got)
	assert.Equal(t, 2, p.captures)

	clock.advance(2 * time.Second)
	_, _ = g.preview(context.Background(), "s", p.capture)
	_, _ = g.preview(context.Background(), "s", p.capture)
	assert.Equal(t, 3, p.captures, "one capture in the later second, then cache hits")
}

func TestPaneActivityGate_FallsBackToCaptureWhenSessionNotInSnapshot(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	for name, snap := range map[string]func() map[string]time.Time{
		"nil snapshot":   func() map[string]time.Time { return nil },
		"absent session": func() map[string]time.Time { return map[string]time.Time{"other": p.activity} },
	} {
		g := gateFor(p, snap)
		before := p.captures
		clock.advance(3 * time.Second)
		for i := 0; i < 3; i++ {
			_, err := g.preview(context.Background(), "s", p.capture)
			require.NoError(t, err)
		}
		assert.Equal(t, before+3, p.captures, name)
	}
}

func TestPaneActivityGate_CaptureErrorIsReturnedAndNotCached(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := gateFor(p, freshSnapshot(p, "s"))
	clock.advance(3 * time.Second)
	boom := errors.New("session not found")

	_, err := g.preview(context.Background(), "s", func(context.Context) (string, error) { return "", boom })
	assert.ErrorIs(t, err, boom)
	_, err = g.preview(context.Background(), "s", p.capture)
	require.NoError(t, err)
	assert.Equal(t, 1, p.captures, "the failure was not cached as content")
}

func TestPaneActivityGate_DisabledAlwaysCaptures(t *testing.T) {
	t.Parallel()
	p, clock := newTestPane()
	g := newPaneActivityGate(freshSnapshot(p, "s"), p.clock.now, func() bool { return false })
	clock.advance(3 * time.Second)
	for i := 0; i < 4; i++ {
		_, _ = g.preview(context.Background(), "s", p.capture)
	}
	assert.Equal(t, 4, p.captures)
}

// N sessions sharing one cache cost one fetch per window, not one per session per tick.
func TestPaneActivityCache_OneFetchPerWindowAcrossSessions(t *testing.T) {
	t.Parallel()
	clock := &paneTestClock{t: time.Unix(1_000_000, 0)}
	fetches := 0
	cache := newPaneActivityCache(func() map[string]time.Time {
		fetches++
		return map[string]time.Time{"a": clock.now()}
	}, clock.now)

	const sessions = 20
	for tick := 0; tick < 5; tick++ {
		for i := 0; i < sessions; i++ {
			cache.get(paneSnapshotMaxAgeDriver)
		}
		clock.advance(2 * time.Second)
	}
	assert.Equal(t, 5, fetches, "20 sessions x 5 ticks -> 1 spawn per tick")
}

func TestParseWindowActivity(t *testing.T) {
	t.Parallel()
	out := []byte("a 10 100\na 11 200\nb 00 300\nb 01 400\nc 11 abc\nd 11\ne 11 0\n")
	got := parseWindowActivity(out)
	assert.Equal(t, map[string]time.Time{
		"a": time.Unix(200, 0), // the active pane's line wins over an earlier inactive one
		"b": time.Unix(300, 0), // no active pane: first line kept
	}, got)
}
