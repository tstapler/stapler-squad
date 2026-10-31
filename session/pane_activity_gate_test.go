package session

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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
