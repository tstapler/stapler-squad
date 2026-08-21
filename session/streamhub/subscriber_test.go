package streamhub_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// memoryTransport is an in-memory Transport test double. By default Send
// succeeds and records every frame; withErrorSend and withBlockingSend
// reconfigure it to exercise the error-eviction and slow-subscriber-eviction
// paths respectively.
type memoryTransport struct {
	mu       sync.Mutex
	received [][]byte
	closed   bool
	closeCh  chan struct{}

	errSend   error
	blockSend bool
}

func newMemoryTransport(opts ...func(*memoryTransport)) *memoryTransport {
	m := &memoryTransport{closeCh: make(chan struct{})}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// withErrorSend makes every Send call return err instead of succeeding.
func withErrorSend(err error) func(*memoryTransport) {
	return func(m *memoryTransport) { m.errSend = err }
}

// withBlockingSend makes Send block until Close is called, simulating a
// stalled writer (e.g. a dead network connection whose write never returns).
func withBlockingSend() func(*memoryTransport) {
	return func(m *memoryTransport) { m.blockSend = true }
}

func (m *memoryTransport) Send(data []byte) error {
	m.mu.Lock()
	if m.errSend != nil {
		err := m.errSend
		m.mu.Unlock()
		return err
	}
	block := m.blockSend
	m.mu.Unlock()

	if block {
		<-m.closeCh
		return errors.New("memoryTransport: closed while send was blocked")
	}

	m.mu.Lock()
	m.received = append(m.received, append([]byte(nil), data...))
	m.mu.Unlock()
	return nil
}

func (m *memoryTransport) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()
	close(m.closeCh)
	return nil
}

func (m *memoryTransport) receivedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.received)
}

func (m *memoryTransport) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

func TestAttachSubscriber_should_IncrementSubscriberCount_When_CalledOnActiveHub(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	transport := newMemoryTransport()

	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true})

	if id == "" {
		t.Fatal("expected a non-empty SubscriberID")
	}
	if got := hub.SubscriberCount(); got != 1 {
		t.Fatalf("expected SubscriberCount() == 1, got %d", got)
	}

	hub.DetachSubscriber(id)
	if !waitFor(t, time.Second, func() bool { return transport.isClosed() }) {
		t.Fatal("expected transport to be closed after DetachSubscriber")
	}
}

func TestDetachSubscriber_should_RemoveSubscriberAndStopWriterGoroutine_When_SubscriberIsAttached(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	transport := newMemoryTransport()

	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{})
	if got := hub.SubscriberCount(); got != 1 {
		t.Fatalf("expected SubscriberCount() == 1 after attach, got %d", got)
	}

	hub.DetachSubscriber(id)

	if got := hub.SubscriberCount(); got != 0 {
		t.Fatalf("expected SubscriberCount() == 0 after detach, got %d", got)
	}
	if !waitFor(t, time.Second, func() bool { return transport.isClosed() }) {
		t.Fatal("expected transport.Close() to have been called on detach")
	}
	// goleak.VerifyNone (deferred above) proves the writer goroutine exited.
}

func TestDetachSubscriber_should_BeNoOp_When_SubscriberIDIsUnknown(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(10*time.Millisecond))

	hub.DetachSubscriber(streamhub.NewSubscriberID())

	if got := hub.SubscriberCount(); got != 0 {
		t.Fatalf("expected SubscriberCount() == 0, got %d", got)
	}
}

func TestStreamHub_should_NotBlockOtherSubscribers_When_OneSubscriberChannelStaysFull(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub(
		"test-session",
		nil,
		streamhub.WithSubscriberBufferSize(4),
		streamhub.WithSlowSubscriberGrace(20*time.Millisecond),
		streamhub.WithTeardownGrace(time.Hour),
	)

	fast := newMemoryTransport()
	slow := newMemoryTransport(withBlockingSend())

	fastID := hub.AttachSubscriber(fast, streamhub.SubscriberCapability{})
	slowID := hub.AttachSubscriber(slow, streamhub.SubscriberCapability{})

	// Broadcast far more frames than the outbound buffer holds. slow's
	// writer never drains (Send blocks forever), so its queue fills and
	// stays full; fast's writer drains essentially instantly, so waiting
	// for it to catch up between sends proves each Broadcast call itself
	// never blocks on slow, regardless of scheduling jitter under -race.
	const frameCount = 20
	start := time.Now()
	for i := 1; i <= frameCount; i++ {
		hub.Broadcast([]byte("frame"))
		if !waitFor(t, 500*time.Millisecond, func() bool { return fast.receivedCount() == i }) {
			t.Fatalf("expected fast subscriber to have received %d frames by broadcast %d, got %d", i, i, fast.receivedCount())
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("expected the broadcast loop to complete quickly despite a stalled subscriber, took %s", elapsed)
	}

	if !waitFor(t, time.Second, func() bool { return hub.SubscriberCount() == 1 }) {
		t.Fatalf("expected slow subscriber to be evicted, SubscriberCount() == %d", hub.SubscriberCount())
	}
	if !waitFor(t, time.Second, func() bool { return slow.isClosed() }) {
		t.Fatal("expected slow subscriber's transport to be closed on eviction")
	}

	hub.DetachSubscriber(fastID)
	_ = slowID
}

func TestStreamHub_should_EvictSubscriberExactlyOnce_When_TransportSendReturnsError(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	sendErr := errors.New("boom")

	other := newMemoryTransport()
	failing := newMemoryTransport(withErrorSend(sendErr))

	otherID := hub.AttachSubscriber(other, streamhub.SubscriberCapability{})
	failingID := hub.AttachSubscriber(failing, streamhub.SubscriberCapability{})

	if got := hub.SubscriberCount(); got != 2 {
		t.Fatalf("expected SubscriberCount() == 2 after both attach, got %d", got)
	}

	hub.Broadcast([]byte("frame"))

	if !waitFor(t, time.Second, func() bool { return hub.SubscriberCount() == 1 }) {
		t.Fatalf("expected failing subscriber to be evicted, SubscriberCount() == %d", hub.SubscriberCount())
	}
	if !waitFor(t, time.Second, func() bool { return other.receivedCount() == 1 }) {
		t.Fatalf("expected the other subscriber to still receive the broadcast, got %d frames", other.receivedCount())
	}

	// A second broadcast must not attempt to re-evict the already-removed
	// subscriber or otherwise misbehave.
	hub.Broadcast([]byte("frame-2"))
	if got := hub.SubscriberCount(); got != 1 {
		t.Fatalf("expected SubscriberCount() to remain 1, got %d", got)
	}

	hub.DetachSubscriber(otherID)
	_ = failingID
}

func TestAttachSubscriber_should_ReturnDistinctIDs_When_CalledMultipleTimes(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))

	firstID := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})
	secondID := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	if firstID == secondID {
		t.Fatalf("expected distinct SubscriberIDs, both were %q", firstID)
	}
	if got := hub.SubscriberCount(); got != 2 {
		t.Fatalf("expected SubscriberCount() == 2, got %d", got)
	}

	hub.DetachSubscriber(firstID)
	hub.DetachSubscriber(secondID)
}
