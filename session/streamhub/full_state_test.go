package streamhub_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// redrawController is a fakeSessionController whose backend also provides a full-state redraw.
type redrawController struct {
	*fakeSessionController
	snap  streamhub.FullStateSnapshot
	ok    bool
	err   error
	calls atomic.Int32
}

func (r *redrawController) CapturePaneRedraw(context.Context) (streamhub.FullStateSnapshot, bool, error) {
	r.calls.Add(1)
	return r.snap, r.ok, r.err
}

func attachAndFrames(t *testing.T, c streamhub.SessionController) [][]byte {
	t.Helper()
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	tr := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{CanResize: true})
	frames := tr.ReceivedFrames()
	hub.DetachSubscriber(id)
	return frames
}

func TestAttachSubscriber_should_SendRedrawVerbatimAndFresh_When_BackendProvidesOne(t *testing.T) {
	defer goleak.VerifyNone(t)
	const redraw = "\x1b[?1049h\x1b]0;t\x07\x1b[2;3Hx"
	c := &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: redraw, AltScreen: true}, ok: true}
	c.captureContent = "legacy text"

	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	first, second := streamhub.NewMemoryTransport(), streamhub.NewMemoryTransport()
	id1 := hub.AttachSubscriber(first, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id1)
	id2 := hub.AttachSubscriber(second, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id2)

	for i, tr := range []*streamhub.MemoryTransport{first, second} {
		frames := tr.ReceivedFrames()
		if len(frames) != 1 || string(frames[0]) != redraw {
			t.Fatalf("subscriber %d: want exactly the redraw verbatim, got %q", i, frames)
		}
	}
	if got := c.captureCalls.Load(); got != 0 {
		t.Fatalf("text capture ran %d times; the redraw must replace it", got)
	}
	if got := c.calls.Load(); got != 2 {
		t.Fatalf("redraw captured %d times for 2 attaches; a redraw must never be served from the cache", got)
	}
}

func TestAttachSubscriber_should_FallBackToTextSnapshot_When_RedrawUnavailable(t *testing.T) {
	cases := map[string]*redrawController{
		"older daemon declines": {ok: false},
		"capture errors":        {ok: true, err: errors.New("daemon gone")},
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			defer goleak.VerifyNone(t)
			c.fakeSessionController = newFakeSessionController()
			c.captureContent = "legacy text"
			frames := attachAndFrames(t, c)
			if len(frames) != 1 || !strings.Contains(string(frames[0]), "legacy text") {
				t.Fatalf("want the legacy text snapshot, got %q", frames)
			}
		})
	}
}
