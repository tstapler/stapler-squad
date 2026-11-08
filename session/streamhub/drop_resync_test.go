package streamhub_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// stalledTransport records frames. Its first free Sends (the attach catch-up) return at once;
// later ones block on gate until it is closed, like a socket whose write buffer is full.
type stalledTransport struct {
	gate chan struct{}
	free atomic.Int32
	mu   sync.Mutex
	got  []string
}

func newStalledTransport(free int32) *stalledTransport {
	t := &stalledTransport{gate: make(chan struct{})}
	t.free.Store(free)
	return t
}

func (g *stalledTransport) Send(data []byte) error {
	if g.free.Add(-1) < 0 {
		<-g.gate
	}
	g.mu.Lock()
	g.got = append(g.got, string(data))
	g.mu.Unlock()
	return nil
}
func (g *stalledTransport) Close() error { return nil }
func (g *stalledTransport) frames() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.got...)
}

func count(frames []string, want string) int {
	n := 0
	for _, f := range frames {
		if f == want {
			n++
		}
	}
	return n
}

func newRepaintHub(clock *streamhub.FakeClock) *streamhub.StreamHub {
	c := &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: "REDRAW"}, ok: true}
	return streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithClock(clock), streamhub.WithSubscriberBufferSize(2),
		streamhub.WithSlowSubscriberGrace(time.Hour))
}

// A frame dropped because a subscriber's queue was full leaves a hole in a byte stream the client
// parses statefully. If the subscriber then recovers within the slow-subscriber grace it is never
// evicted, so the hub has to repaint it or the terminal stays garbled until the app redraws.
func TestHub_should_RepaintSubscriber_When_FrameDroppedAndSubscriberRecovers(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := newRepaintHub(clock)
	tr := newStalledTransport(1)
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{})
	defer hub.DetachSubscriber(id)

	// One frame is held in the blocked Send, two fill the queue, the rest are dropped.
	for _, f := range []string{"f1", "f2", "f3", "f4", "f5", "f6"} {
		hub.Broadcast([]byte(f))
	}
	clock.Advance(time.Second) // runs the debounced repaint, whose capture happens now
	close(tr.gate)

	// REDRAW is also the attach catch-up, so a repaint makes it appear twice.
	require.Eventually(t, func() bool {
		fr := tr.frames()
		return count(fr, "REDRAW") == 2 && fr[len(fr)-1] == "REDRAW"
	}, 5*time.Second, time.Millisecond)
	hub.Broadcast([]byte("after"))
	require.Eventually(t, func() bool {
		fr := tr.frames()
		return fr[len(fr)-1] == "after"
	}, 5*time.Second, time.Millisecond)
}

// A healthy subscriber that never lost a frame must not be repainted because a neighbour did.
func TestHub_should_NotRepaintHealthySubscriber_When_NeighbourDropsFrames(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := newRepaintHub(clock)
	slow := newStalledTransport(1)
	idSlow := hub.AttachSubscriber(slow, streamhub.SubscriberCapability{})
	defer hub.DetachSubscriber(idSlow)
	fast := newStalledTransport(1 << 20)
	idFast := hub.AttachSubscriber(fast, streamhub.SubscriberCapability{})
	defer hub.DetachSubscriber(idFast)

	for _, f := range []string{"f1", "f2", "f3", "f4", "f5"} {
		hub.Broadcast([]byte(f))
		require.Eventually(t, func() bool { fr := fast.frames(); return fr[len(fr)-1] == f }, 5*time.Second, time.Millisecond)
	}
	clock.Advance(time.Second)
	close(slow.gate)
	require.Eventually(t, func() bool {
		fr := slow.frames()
		return count(fr, "REDRAW") == 2 && fr[len(fr)-1] == "REDRAW"
	}, 5*time.Second, time.Millisecond)
	require.Equal(t, []string{"REDRAW", "f1", "f2", "f3", "f4", "f5"}, fast.frames())
}
