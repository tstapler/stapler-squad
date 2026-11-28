//go:build !windows

package services

// Ordering and teardown properties of the stream RPCs that need a hook inside
// the server-side stream.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/session/domain"
)

// sendHook runs around every server-side Send of a streaming handler, and after
// the handler returns while its request context is still live.
type sendHook struct {
	send     func(next func(any) error, msg any) error
	returned func()
}

func (sendHook) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }
func (sendHook) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (h sendHook) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, hookedConn{StreamingHandlerConn: conn, hook: h.send})
		if h.returned != nil {
			h.returned()
		}
		return err
	}
}

type hookedConn struct {
	connect.StreamingHandlerConn
	hook func(next func(any) error, msg any) error
}

func (c hookedConn) Send(msg any) error {
	if c.hook == nil {
		return c.StreamingHandlerConn.Send(msg)
	}
	return c.hook(c.StreamingHandlerConn.Send, msg)
}

// serveHooked points e's client at a second server over the same service.
func (e *charEnv) serveHooked(t *testing.T, hook sendHook) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(e.svc, connect.WithInterceptors(e.exit, hook))
	mux.Handle(path, handler)
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	t.Cleanup(srv.CloseClientConnections)
	e.disconnect = srv.CloseClientConnections
	e.client = sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
}

// publishDuringFirstSend publishes ev from inside the first Send, which is
// after the replay or snapshot list was taken and before the live loop runs.
func publishDuringFirstSend(bus *events.EventBus, ev *events.Event) sendHook {
	var once sync.Once
	return sendHook{send: func(next func(any) error, msg any) error {
		once.Do(func() { bus.Publish(ev) })
		return next(msg)
	}}
}

// The subscription has to exist before the replay is read, or an event
// published while it is being sent reaches neither.
func TestWatchSessions_ShouldDeliverEventsPublishedDuringReplayOrSnapshot_When_ReconnectingOrFresh(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"replay", "snapshot"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			e := newCharEnv(t, true)
			a := watchInstance("a", "", domain.Active, false)
			addAll(e, a)
			first := events.NewSessionUpdatedEvent(a, []string{"first"})
			e.bus.Publish(first)
			e.bus.Publish(events.NewSessionUpdatedEvent(a, []string{"missed"}))
			e.serveHooked(t, publishDuringFirstSend(e.bus, events.NewSessionUpdatedEvent(a, []string{"during"})))

			req, opening := &sessionv1.WatchSessionsRequest{}, "created a"
			if mode == "replay" {
				req, opening = &sessionv1.WatchSessionsRequest{AfterSeq: first.Seq}, "updated a missed"
			}
			w := e.watch(t, req)
			assert.Equal(t, []string{opening, "updated a during"}, w.take(2))
		})
	}
}

// A failed Send returns from the handler while the request context is still
// live, so only the handler's own defers can have released the subscriptions.
func TestWatchSessions_ShouldReleaseBothSubscriptionsBeforeReturning_When_SendFails(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	addAll(e, watchInstance("a", "", domain.Active, false))
	mirror := federation.NewStaticMirror("host_01ARZ3NDEKTSV4RRFFQ69G5FAV", []*sessionv1.Session{{Id: "p", Title: "peer"}})
	e.svc.SetFederation(mirror)
	// The aggregator exposes no count; a renamed field panics here rather than passing.
	peerSubs := func() int { return reflect.ValueOf(mirror.Watch()).Elem().FieldByName("subs").Len() }

	before := e.bus.SubscriberCount()
	var sends, busAtReturn, peersAtReturn atomic.Int64
	e.serveHooked(t, sendHook{
		send: func(next func(any) error, msg any) error {
			// Let the local snapshot through; fail on the peer snapshot, after both subscriptions.
			if sends.Add(1) == 2 {
				return errors.New("send refused")
			}
			return next(msg)
		},
		returned: func() {
			busAtReturn.Store(int64(e.bus.SubscriberCount()))
			peersAtReturn.Store(int64(peerSubs()))
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), charWait)
	defer cancel()
	stream, err := e.client.WatchSessions(ctx, connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
	}
	require.ErrorContains(t, e.exit.wait(t), "failed to send peer snapshot")
	assert.EqualValues(t, before, busAtReturn.Load())
	assert.Zero(t, peersAtReturn.Load())
}

// refusingResizePM signals each resize attempt and fails it.
type refusingResizePM struct {
	*charPM
	attempted chan struct{}
}

func (p refusingResizePM) SetWindowSize(int, int) error {
	p.attempted <- struct{}{}
	return errors.New("boom")
}

// Connect forbids concurrent Send on one stream. The output goroutine is held
// inside Send while the input goroutine has an error reply to send.
func TestStreamTerminal_ShouldNotCallSendConcurrently_When_AnErrorReplyRacesOutput(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	pm := refusingResizePM{charPM: p.pm, attempted: make(chan struct{}, 1)}
	p.inst.SetProcessManagerForTest(pm)
	e.poller.AddInstance(p.inst)

	held, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	var inSend atomic.Int64
	var first sync.Once
	e.serveHooked(t, sendHook{send: func(next func(any) error, msg any) error {
		if inSend.Add(1) > 1 {
			second <- struct{}{}
		}
		defer inSend.Add(-1)
		first.Do(func() {
			close(held)
			<-release
		})
		return next(msg)
	}})
	// Runs before the server cleanups, so a failure never leaves Send blocked.
	var open sync.Once
	releaseSend := func() { open.Do(func() { close(release) }) }
	t.Cleanup(releaseSend)

	id := p.inst.Title
	s := e.open(t, id)
	p.emit(t, "output")
	select {
	case <-held:
	case <-time.After(charWait):
		t.Fatal("output never reached Send")
	}
	s.send(resizeFrame(id, 10, 10))
	select {
	case <-pm.attempted:
	case <-time.After(charWait):
		t.Fatal("resize never attempted")
	}
	// Correct code stays blocked on the lock however long this waits; the window
	// only has to outlast the input goroutine's walk to an unguarded Send.
	select {
	case <-second:
		t.Fatal("a second Send ran while the first was still inside Send")
	case <-time.After(500 * time.Millisecond):
	}
	releaseSend()
	assert.Equal(t, "output", string(s.next().GetOutput().GetData()))
	assert.Equal(t, "RESIZE_ERROR", s.next().GetError().GetCode())
}
