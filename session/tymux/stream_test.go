package tymux

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// fakeAttachStream is a hand-driven attachStream substitute: Receive()
// plays back queued events (send more anytime via push) until either the
// events channel is closed (io.EOF, mimicking a server-ended stream) or
// ctx is cancelled (mimicking DetachSafely()'s full-cancellation contract,
// Task 2.3.4b) — matching how the real
// *connect.BidiStreamForClient[...]'s Receive reacts to context
// cancellation.
type fakeAttachStream struct {
	ctx    context.Context
	events chan *v1.AttachEvent
	sendFn func(*v1.AttachRequest) error

	// recvErr, if set, makes every Receive() call return this error
	// immediately instead of consulting events/ctx — used by Epic 2.5's
	// reconnect tests to simulate a stream that ends immediately with a
	// specific RPC error (e.g. connect.CodeFailedPrecondition, Task
	// 2.5.3a) without a live server.
	recvErr error

	mu   sync.Mutex
	sent []*v1.AttachRequest
}

func newFakeAttachStream(ctx context.Context) *fakeAttachStream {
	return &fakeAttachStream{ctx: ctx, events: make(chan *v1.AttachEvent, 16)}
}

func (f *fakeAttachStream) Send(req *v1.AttachRequest) error {
	f.mu.Lock()
	f.sent = append(f.sent, req)
	f.mu.Unlock()
	if f.sendFn != nil {
		return f.sendFn(req)
	}
	return nil
}

func (f *fakeAttachStream) Receive() (*v1.AttachEvent, error) {
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	select {
	case ev, ok := <-f.events:
		if !ok {
			return nil, io.EOF
		}
		return ev, nil
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	}
}

func (f *fakeAttachStream) CloseRequest() error  { return nil }
func (f *fakeAttachStream) CloseResponse() error { return nil }

func (f *fakeAttachStream) push(ev *v1.AttachEvent) { f.events <- ev }

func (f *fakeAttachStream) sentRequests() []*v1.AttachRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*v1.AttachRequest, len(f.sent))
	copy(out, f.sent)
	return out
}

var _ attachStream = (*fakeAttachStream)(nil)

// startedSessionWithStream builds a started tymuxGRPCSession (via the
// fakeTransport helpers in session_test.go) and returns it alongside the
// fakeAttachStream the standing stream opened against, and the
// fakeTransport itself (for attachCalls assertions).
func startedSessionWithStream(t *testing.T) (TymuxManager, *fakeAttachStream, *fakeTransport) {
	t.Helper()
	dir := t.TempDir()
	var stream *fakeAttachStream
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	transport.attachFn = func(ctx context.Context) attachStream {
		stream = newFakeAttachStream(ctx)
		return stream
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	require.NotNil(t, stream)
	return sess, stream, transport
}

// --- Story 2.3.1 ---

func TestStandingStream_OpensExactlyOnce_NotPerSendKeysCall(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)

	_, err := sess.SendKeys("a")
	require.NoError(t, err)
	_, err = sess.SendKeys("b")
	require.NoError(t, err)
	_, err = sess.SendKeys("c")
	require.NoError(t, err)

	assert.EqualValues(t, 1, transport.attachCalls, "exactly one Attach stream must be opened, not one per SendKeys call")
}

func TestStandingStream_SendsPaneIdAsFirstMessage(t *testing.T) {
	_, stream, _ := startedSessionWithStream(t)

	sent := stream.sentRequests()
	require.Len(t, sent, 1)
	assert.Equal(t, "pane-1", sent[0].GetPaneId())
}

func TestStandingStream_SnapshotEvent_SeedsLiveness(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
		Snapshot: &v1.PaneSnapshot{PaneId: "pane-1", Liveness: v1.Liveness_LIVENESS_DEAD},
	}})

	// IsAlive() always falls back to a fresh CapturePane RPC (Task
	// 2.2.1c) rather than trusting the cache, so assert on the cached
	// field the reader goroutine actually writes instead.
	concrete := sess.(*tymuxGRPCSession)
	require.Eventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.liveness == v1.Liveness_LIVENESS_DEAD
	}, time.Second, time.Millisecond, "snapshot event should seed cached liveness from PaneSnapshot.liveness")
}

// --- Story 2.3.2 ---

func TestStandingStream_OutputEvent_FansOutToAllSubscribers(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	_, ch1 := sess.SubscribeToControlModeUpdates()
	_, ch2 := sess.SubscribeToControlModeUpdates()

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("hi")}})

	for i, ch := range []chan []byte{ch1, ch2} {
		select {
		case got := <-ch:
			assert.Equal(t, []byte("hi"), got, "subscriber %d", i)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d never received the Output event", i)
		}
	}
}

func TestStandingStream_UnsubscribeFromControlModeUpdates_StopsDelivery(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	id, ch := sess.SubscribeToControlModeUpdates()

	sess.UnsubscribeFromControlModeUpdates(id)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("hi")}})

	_, open := <-ch
	assert.False(t, open, "the channel must be closed after Unsubscribe")
}

// --- Story 2.3.3 ---

func TestStandingStream_SendKeys_SendsInputOnExistingStream(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)

	n, err := sess.SendKeys("echo hi\n")

	require.NoError(t, err)
	assert.Equal(t, len("echo hi\n"), n)
	assert.EqualValues(t, 1, transport.attachCalls, "SendKeys must not open a new stream")
	sent := stream.sentRequests()
	require.Len(t, sent, 2) // [0] = pane_id, [1] = this Input
	assert.Equal(t, []byte("echo hi\n"), sent[1].GetInput())
}

func TestStandingStream_TapEnter_SendsCarriageReturn(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.TapEnter())

	sent := stream.sentRequests()
	require.Len(t, sent, 2)
	assert.Equal(t, []byte{0x0D}, sent[1].GetInput())
}

func TestStandingStream_ExitedEvent_UpdatesLivenessAndFiresRegisteredCallback(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	reasons := make(chan string, 1)
	sess.SetOnExitCallback(func(reason string) { reasons <- reason })

	code := int32(42)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	select {
	case reason := <-reasons:
		assert.Equal(t, "exited: code=42", reason)
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired")
	}

	concrete := sess.(*tymuxGRPCSession)
	require.Eventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.liveness == v1.Liveness_LIVENESS_DEAD
	}, time.Second, time.Millisecond, "Exited event must update cached liveness to DEAD")
}

// --- Story 2.3.4 ---

// TestStandingStream_Attach_ReturnsChannelClosedWhenDeliberatelyDetached
// replaces the old (pre-Epic-2.5)
// TestStandingStream_Attach_ReturnsChannelClosedWhenStreamEnds: under
// Story 2.5.1, ending the stream WITHOUT a preceding Close()/
// DetachSafely() is a drop that triggers ReconnectLoop, not an immediate
// "the standing stream ended" signal — see the reconnect-specific tests
// below. Attach()'s channel only closes immediately for a deliberate
// detach, exercised here via DetachSafely() itself rather than directly
// closing the fake stream's events channel.
func TestStandingStream_Attach_ReturnsChannelClosedWhenDeliberatelyDetached(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)

	done, err := sess.Attach()
	require.NoError(t, err)

	select {
	case <-done:
		t.Fatal("Attach's channel closed before DetachSafely was called")
	default:
	}

	require.NoError(t, sess.DetachSafely())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Attach's channel never closed after a deliberate DetachSafely")
	}
}

func TestStandingStream_DetachSafely_CancelsTheAttachContext(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.DetachSafely())

	select {
	case <-stream.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("DetachSafely must fully cancel the Attach call's context")
	}
}

func TestStandingStream_DetachSafely_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.Error(t, sess.DetachSafely())
}

func TestStandingStream_Attach_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	_, err := sess.Attach()
	assert.Error(t, err)
}

// --- Story 2.4.1 ---

func TestStandingStream_SetWindowSize_SendsResizeOnStandingStream(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.SetWindowSize(100, 40))

	sent := stream.sentRequests()
	require.Len(t, sent, 2) // [0] is the initial PaneId, [1] is our Resize
	resize := sent[1].GetResize()
	require.NotNil(t, resize)
	assert.Equal(t, uint32(100), resize.GetCols())
	assert.Equal(t, uint32(40), resize.GetRows())
}

// TestStandingStream_SetDetachedSize_SendsResizeOnStandingStream_EvenWithNoActiveAttachCaller
// asserts the acceptance criterion from plan.md Story 2.4.1 verbatim: a
// SetDetachedSize call still succeeds and sends Resize on the always-open
// standing stream, even though nothing here ever calls sess.Attach() (i.e.
// no active Attach() caller from stapler-squad's own UI layer) —
// the standing-stream design makes detached resize possible where tmux's
// own detached-resize needs a separate mechanism (architecture.md §1 Gap
// #3).
func TestStandingStream_SetDetachedSize_SendsResizeOnStandingStream_EvenWithNoActiveAttachCaller(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.SetDetachedSize(100, 40, "title"))

	sent := stream.sentRequests()
	require.Len(t, sent, 2)
	resize := sent[1].GetResize()
	require.NotNil(t, resize)
	assert.Equal(t, uint32(100), resize.GetCols())
	assert.Equal(t, uint32(40), resize.GetRows())
}

func TestStandingStream_SetWindowSize_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.Error(t, sess.SetWindowSize(80, 24))
}

func TestRefreshClient_AlwaysReturnsNil(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	assert.NoError(t, sess.RefreshClient())
}

// --- Epic 2.5: reconnect loop and resync ---

// setReconnectBackoff overrides a started session's ReconnectLoop backoff
// bounds for fast, deterministic tests — the production defaults
// (session.go's defaultReconnect* consts) would make these tests slow.
func setReconnectBackoff(sess TymuxManager, base, max time.Duration, maxAttempts int) *tymuxGRPCSession {
	c := sess.(*tymuxGRPCSession)
	c.mu.Lock()
	c.reconnectBaseDelay = base
	c.reconnectMaxDelay = max
	c.reconnectMaxAttempts = maxAttempts
	c.mu.Unlock()
	return c
}

// --- Story 2.5.1 ---

func TestReconnectLoop_DoesNotFire_OnDeliberateDetach(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)

	require.NoError(t, sess.DetachSafely())
	time.Sleep(20 * time.Millisecond) // let any (incorrect) reconnect attempt start

	assert.EqualValues(t, 1, transport.attachCalls, "DetachSafely must not trigger ReconnectLoop — no second Attach call")
}

func TestReconnectLoop_Fires_OnTransportErrorNotPrecededByDetach(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 5*time.Millisecond, 3)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
		}})
		return s
	}

	close(stream.events) // drop, not preceded by DetachSafely/Close

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&transport.attachCalls) >= 2
	}, time.Second, time.Millisecond, "a non-deliberate stream end must trigger ReconnectLoop (a second Attach call)")
}

// --- Story 2.5.2 ---

func TestReconnectLoop_TransparentlyReattaches_OnTransientDrop_WithoutClosingAttachChannel(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 5*time.Millisecond, 4)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{
				Liveness: v1.Liveness_LIVENESS_LIVE,
				Grid:     []*v1.Row{row(cell("R", 0, 0, 0), cell("E", 0, 0, 0), cell("D", 0, 0, 0))},
			},
		}})
		return s
	}

	done, err := sess.Attach()
	require.NoError(t, err)
	_, subCh := sess.SubscribeToControlModeUpdates()

	close(stream.events) // drop

	select {
	case <-done:
		t.Fatal("Attach's channel closed on a transient drop that should have reconnected transparently")
	case <-time.After(200 * time.Millisecond):
	}

	select {
	case got := <-subCh:
		assert.Contains(t, string(got), "RED", "expected the post-reconnect resync redraw")
	case <-time.After(time.Second):
		t.Fatal("subscriber never received the post-reconnect resync redraw")
	}

	reconnecting, _, _ := sess.ReconnectState()
	assert.False(t, reconnecting, "ReconnectState must clear once the reconnect succeeds")
}

func TestReconnectLoop_GivesUp_AfterMaxAttempts_ClosesAttachChannel_AndFiresDistinctExitReason(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 2)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.sendFn = func(*v1.AttachRequest) error { return errors.New("boom: unreachable") }
		return s
	}

	reasons := make(chan string, 1)
	sess.SetOnExitCallback(func(reason string) { reasons <- reason })

	done, err := sess.Attach()
	require.NoError(t, err)

	close(stream.events) // drop

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Attach's channel never closed after ReconnectLoop exhausted its attempts")
	}

	select {
	case reason := <-reasons:
		assert.Contains(t, reason, "reconnect failed", "the give-up reason must be distinguishable from an ordinary process exit")
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired after ReconnectLoop exhaustion")
	}
}

// TestReconnectLoop_NoDuplicateRenderedOutput_AcrossReconnectBoundary is
// Task 2.5.2d's regression test: force a drop while output is actively
// streaming, let ReconnectLoop reconnect, and assert ClientFanout's
// subscribers never receive the same content twice across the reconnect
// boundary — closing adversarial-review.md's subscribe-then-snapshot
// Blocker for the client-triggered-reconnect case specifically.
func TestReconnectLoop_NoDuplicateRenderedOutput_AcrossReconnectBoundary(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)
	_, subCh := sess.SubscribeToControlModeUpdates()

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("before-drop")}})
	select {
	case got := <-subCh:
		assert.Equal(t, []byte("before-drop"), got)
	case <-time.After(time.Second):
		t.Fatal("never received pre-drop output")
	}

	var second *fakeAttachStream
	var secondMu sync.Mutex
	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{
				Liveness: v1.Liveness_LIVENESS_LIVE,
				Grid:     []*v1.Row{row(cell("R", 0, 0, 0), cell("W", 0, 0, 0))},
			},
		}})
		secondMu.Lock()
		second = s
		secondMu.Unlock()
		return s
	}

	close(stream.events) // drop mid-stream

	var redraw []byte
	select {
	case redraw = <-subCh:
	case <-time.After(time.Second):
		t.Fatal("never received the post-reconnect resync redraw")
	}
	assert.Contains(t, string(redraw), "RW")

	require.Eventually(t, func() bool {
		secondMu.Lock()
		defer secondMu.Unlock()
		return second != nil
	}, time.Second, time.Millisecond)
	secondMu.Lock()
	second.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("after-reconnect")}})
	secondMu.Unlock()

	select {
	case got := <-subCh:
		assert.Equal(t, []byte("after-reconnect"), got)
	case <-time.After(time.Second):
		t.Fatal("never received post-reconnect live output")
	}

	select {
	case extra := <-subCh:
		t.Fatalf("received an unexpected extra broadcast (possible duplicate across the reconnect boundary): %q", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

// --- Story 2.5.3 ---

func TestReconnectLoop_DetectsDaemonRestart_RevivesSession_AndSurfacesDistinctState(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)

	var revived int32
	transport.reviveSessionFn = func(_ context.Context, _ *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
		atomic.AddInt32(&revived, 1)
		return connect.NewResponse(&v1.ReviveSessionResponse{}), nil
	}

	var calls int32
	transport.attachFn = func(ctx context.Context) attachStream {
		n := atomic.AddInt32(&calls, 1)
		s := newFakeAttachStream(ctx)
		if n == 1 {
			s.recvErr = connect.NewError(connect.CodeFailedPrecondition, errors.New("pane exited"))
		} else {
			s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
				Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
			}})
		}
		return s
	}

	done, err := sess.Attach()
	require.NoError(t, err)

	close(stream.events) // drop

	select {
	case <-done:
		t.Fatal("Attach's channel closed; a daemon-restart reconnect should still succeed transparently")
	case <-time.After(300 * time.Millisecond):
	}

	concrete := sess.(*tymuxGRPCSession)
	require.Eventually(t, func() bool {
		restarted, _ := concrete.BackendRestarted()
		return restarted
	}, time.Second, time.Millisecond, "BackendRestarted should report true after a FailedPrecondition-detected daemon restart")

	assert.EqualValues(t, 1, atomic.LoadInt32(&revived), "ReviveSession must be called exactly once for the daemon-restart case")
	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2), "expected at least 2 Attach dial attempts (dead, then revived)")
}

func TestReconnectLoop_OrdinaryDrop_DoesNotSetBackendRestarted(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
		}})
		return s
	}

	close(stream.events) // ordinary transport drop, pane stays live

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&transport.attachCalls) >= 2
	}, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	concrete := sess.(*tymuxGRPCSession)
	restarted, _ := concrete.BackendRestarted()
	assert.False(t, restarted, "an ordinary transport blip must not be surfaced as a daemon restart")
}
