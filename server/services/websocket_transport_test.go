package services

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// fakeSessionController is a minimal streamhub.SessionController test double
// used by this file's integration-style tests — it never needs a real tmux
// process since these tests only exercise WebSocketTransport's Send/Close
// contract, not the hub's resize/quiescence pipeline.
type fakeSessionController struct {
	stopControlModeCalls int

	// subscribeCalls counts SubscribeControlModeUpdates invocations — used
	// by Story 3.2.1's test asserting the hub subscribes to the underlying
	// TmuxSession's control-mode output exactly once regardless of how many
	// Subscribers are attached to the hub itself.
	subscribeCalls int32
}

func (f *fakeSessionController) SetWindowSize(int, int) error        { return nil }
func (f *fakeSessionController) ResizePTY(int, int) error            { return nil }
func (f *fakeSessionController) CapturePaneContent() (string, error) { return "", nil }
func (f *fakeSessionController) StopControlMode() error {
	f.stopControlModeCalls++
	return nil
}
func (f *fakeSessionController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	atomic.AddInt32(&f.subscribeCalls, 1)
	ch := make(chan []byte)
	close(ch)
	return "fake-sub", ch
}
func (f *fakeSessionController) UnsubscribeControlModeUpdates(string) {}

// TestWebSocketTransport_should_WriteViaMutexGuardedWriteMessage_When_SendCalledOnLiveConnection
// is REQ-2 from validation.md: Send must go through connectWebSocketStream's
// existing mutex-guarded WriteMessage, reusing that concurrency-safety fix
// rather than duplicating it (Story 2.2.1's AC).
func TestWebSocketTransport_should_WriteViaMutexGuardedWriteMessage_When_SendCalledOnLiveConnection(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	transport := NewWebSocketTransport(serverStream)

	payload := []byte("hello from the hub")
	require.NoError(t, transport.Send(payload))

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := clientConn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

// TestWebSocketTransport_should_DetachSubscriberExactlyOnce_When_CloseCalledAfterAttach
// is Story 2.2.1's second AC: Close triggers exactly one DetachSubscriber
// call on the bound hub, with no double-detach even though streamhub's own
// subscriber.close calls back into Transport.Close as part of teardown.
func TestWebSocketTransport_should_DetachSubscriberExactlyOnce_When_CloseCalledAfterAttach(t *testing.T) {
	serverStream, _, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	controller := &fakeSessionController{}
	hub := streamhub.NewStreamHub("ws-transport-test", controller, streamhub.WithTeardownGrace(0))

	transport := NewWebSocketTransport(serverStream)
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true, CanWrite: true})
	transport.BindSubscriber(hub, id)

	require.Equal(t, 1, hub.SubscriberCount())

	require.NoError(t, transport.Close())
	require.NoError(t, transport.Close()) // second call must be a no-op, not a deadlock or double-detach

	require.Eventually(t, func() bool {
		return hub.SubscriberCount() == 0
	}, time.Second, 5*time.Millisecond, "subscriber should be detached exactly once")
}

// TestWebSocketTransport_should_NotDeadlock_When_HubDetachesFirst covers the
// other direction: the hub evicting the subscriber (e.g. via
// DetachSubscriber, which internally calls subscriber.close -> Transport.Close)
// must not deadlock or panic, since Transport.Close's own bookkeeping is
// CompareAndSwap-guarded rather than sync.Once-guarded specifically to allow
// this reentrant call.
func TestWebSocketTransport_should_NotDeadlock_When_HubDetachesFirst(t *testing.T) {
	serverStream, _, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	controller := &fakeSessionController{}
	hub := streamhub.NewStreamHub("ws-transport-test-2", controller, streamhub.WithTeardownGrace(0))

	transport := NewWebSocketTransport(serverStream)
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true, CanWrite: true})
	transport.BindSubscriber(hub, id)

	done := make(chan struct{})
	go func() {
		hub.DetachSubscriber(id)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("hub.DetachSubscriber deadlocked calling back into Transport.Close")
	}

	// Explicit Close from the connection side afterward must still be safe.
	require.NoError(t, transport.Close())
}

// TestWebSocketTransport_should_ReturnSendError_When_ConnectionIsClosed
// exercises the error path Transport.Send is documented to surface —
// StreamHub treats a non-nil Send error identically to a slow-subscriber
// eviction (session/streamhub/transport.go's doc comment).
func TestWebSocketTransport_should_ReturnSendError_When_ConnectionIsClosed(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	transport := NewWebSocketTransport(serverStream)

	clientConn.Close()
	serverStream.conn.Close()

	err := transport.Send([]byte("should fail"))
	require.Error(t, err)
}
