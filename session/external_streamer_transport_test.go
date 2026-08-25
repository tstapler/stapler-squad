package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// newTestExternalStreamer returns an *ExternalStreamer with no real mux
// socket behind it. MuxTransport only ever touches the consumer
// registry/broadcast path (AddConsumer, RemoveConsumer, broadcast), never
// the socket-connection machinery, so a streamer that was never Start()ed
// is a perfectly real (not faked) *ExternalStreamer for these tests —
// matching validation.md's requirement that MuxTransport be tested "against
// a real ExternalStreamer", not a hand-rolled fake.
func newTestExternalStreamer() *ExternalStreamer {
	return NewExternalStreamer("/tmp/unused-for-this-test.sock", 0)
}

func TestMuxTransport_should_ImplementStreamhubTransport(t *testing.T) {
	var _ streamhub.Transport = (*MuxTransport)(nil)
}

func TestMuxTransport_should_DeliverBroadcastBytesToConsumers_When_HubSendsFrame(t *testing.T) {
	streamer := newTestExternalStreamer()

	var mu sync.Mutex
	var received [][]byte
	done := make(chan struct{}, 1)
	streamer.AddConsumer(func(data []byte) {
		mu.Lock()
		received = append(received, append([]byte(nil), data...))
		mu.Unlock()
		done <- struct{}{}
	}, false)

	transport := NewMuxTransport(streamer)

	require.NoError(t, transport.Send([]byte("hello from hub")))
	<-done // ExternalStreamer.broadcast fans out via goroutines (Task 4.1.1a note)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, [][]byte{[]byte("hello from hub")}, received)
}

func TestMuxTransport_should_DeliverToEveryRegisteredConsumer_When_MultipleConsumersAttached(t *testing.T) {
	streamer := newTestExternalStreamer()

	var mu sync.Mutex
	count := 0
	done := make(chan struct{}, 2)
	recordAndSignal := func([]byte) {
		mu.Lock()
		count++
		mu.Unlock()
		done <- struct{}{}
	}
	streamer.AddConsumer(recordAndSignal, false)
	streamer.AddConsumer(recordAndSignal, false)

	transport := NewMuxTransport(streamer)

	require.NoError(t, transport.Send([]byte("frame")))
	<-done
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, count)
}

func TestMuxTransport_should_RegisterPresenceConsumer_When_Constructed(t *testing.T) {
	streamer := newTestExternalStreamer()

	before := streamer.ConsumerCount()
	transport := NewMuxTransport(streamer)
	require.Equal(t, before+1, streamer.ConsumerCount())

	require.NoError(t, transport.Close())
	require.Equal(t, before, streamer.ConsumerCount())
}

func TestMuxTransport_should_BeIdempotent_When_CloseCalledTwice(t *testing.T) {
	streamer := newTestExternalStreamer()
	transport := NewMuxTransport(streamer)

	before := streamer.ConsumerCount()
	require.NoError(t, transport.Close())
	require.Equal(t, before-1, streamer.ConsumerCount())

	// Second Close must not attempt to remove an already-removed consumer
	// key (which would be a silent no-op via RemoveConsumer's map delete
	// anyway, but the CompareAndSwap guard means it is never even
	// attempted) and must not error or panic.
	require.NoError(t, transport.Close())
	require.Equal(t, before-1, streamer.ConsumerCount())
}

func TestMuxTransport_should_ReturnNilError_When_SendCalledWithNoConsumers(t *testing.T) {
	streamer := newTestExternalStreamer()
	// Attach and immediately close so the presence consumer is gone too —
	// Send must still be safe (and a no-op observably) with zero consumers.
	transport := NewMuxTransport(streamer)
	require.NoError(t, transport.Close())

	require.NoError(t, transport.Send([]byte("nobody listening")))
}

// TestAttachMuxTransportToHub_should_DeliverHubBroadcastToExternalStreamerConsumers_When_Attached
// proves the Story 4.1.2 wiring end-to-end: a hub.Broadcast call reaches a
// consumer registered on the underlying *ExternalStreamer via the
// MuxTransport attached by AttachMuxTransportToHub, exactly like a real
// ssq-mux client's own AddConsumer callback would.
func TestAttachMuxTransportToHub_should_DeliverHubBroadcastToExternalStreamerConsumers_When_Attached(t *testing.T) {
	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	streamer := newTestExternalStreamer()

	var mu sync.Mutex
	var received [][]byte
	done := make(chan struct{}, 1)
	streamer.AddConsumer(func(data []byte) {
		mu.Lock()
		received = append(received, append([]byte(nil), data...))
		mu.Unlock()
		done <- struct{}{}
	}, false)

	AttachMuxTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount())

	hub.Broadcast([]byte("hub says hello"))
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, [][]byte{[]byte("hub says hello")}, received)
}

// TestAttachMuxTransportToHub_should_AttachOnlyOnce_When_CalledForMultipleBrowserConnections
// proves the idempotency guarantee Story 4.1.2 requires: two browser
// connections attaching to the same hub (each independently calling
// AttachMuxTransportToHub, mirroring streamViaHub's per-connection attach
// path) must not register the session's ExternalStreamer as a second
// MuxTransport subscriber.
func TestAttachMuxTransportToHub_should_AttachOnlyOnce_When_CalledForMultipleBrowserConnections(t *testing.T) {
	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	streamer := newTestExternalStreamer()

	before := streamer.ConsumerCount()

	// Simulate a first browser connection attaching...
	AttachMuxTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount())
	require.Equal(t, before+1, streamer.ConsumerCount(),
		"first attach must register exactly one presence consumer on the ExternalStreamer")

	// ...and a second browser connection attaching to the same hub.
	AttachMuxTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount(),
		"a second AttachMuxTransportToHub call for the same hub must not add a second subscriber")
	require.Equal(t, before+1, streamer.ConsumerCount(),
		"a second AttachMuxTransportToHub call must not register a second presence consumer")
}
