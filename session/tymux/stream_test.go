package tymux

import (
	"context"
	"io"
	"sync"
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

func TestStandingStream_Attach_ReturnsChannelClosedWhenStreamEnds(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	done, err := sess.Attach()
	require.NoError(t, err)

	select {
	case <-done:
		t.Fatal("Attach's channel closed before the stream ended")
	default:
	}

	close(stream.events) // simulate the server ending the stream (io.EOF)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Attach's channel never closed after the stream ended")
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
