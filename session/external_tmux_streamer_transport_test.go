package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// newTestExternalTmuxStreamer returns an *ExternalTmuxStreamer that was never
// Start()ed. ExternalTmuxStreamerTransport only ever touches the consumer
// registry/notifyConsumers path (AddConsumer, RemoveConsumer,
// notifyConsumers), never the tmux control-mode/capture-pane machinery Start
// spins up, so an un-started streamer is a real (not faked)
// *ExternalTmuxStreamer for these tests -- mirroring
// newTestExternalStreamer's rationale in external_streamer_transport_test.go.
func newTestExternalTmuxStreamer() *ExternalTmuxStreamer {
	return NewExternalTmuxStreamer("unused-for-this-test")
}

func TestExternalTmuxStreamerTransport_should_ImplementStreamhubTransport(t *testing.T) {
	var _ streamhub.Transport = (*ExternalTmuxStreamerTransport)(nil)
}

func TestExternalTmuxStreamerTransport_should_DeliverSendBytesAsContentToConsumers_When_HubSendsFrame(t *testing.T) {
	streamer := newTestExternalTmuxStreamer()

	var mu sync.Mutex
	var received []string
	done := make(chan struct{}, 1)
	streamer.AddConsumer(func(content string) {
		mu.Lock()
		received = append(received, content)
		mu.Unlock()
		done <- struct{}{}
	})

	transport := NewExternalTmuxStreamerTransport(streamer)

	require.NoError(t, transport.Send([]byte("hello from hub")))
	<-done // notifyConsumers fans out via goroutines, mirroring ExternalStreamer.broadcast

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"hello from hub"}, received)
}

func TestExternalTmuxStreamerTransport_should_DeliverToEveryRegisteredConsumer_When_MultipleConsumersAttached(t *testing.T) {
	streamer := newTestExternalTmuxStreamer()

	var mu sync.Mutex
	count := 0
	done := make(chan struct{}, 2)
	recordAndSignal := func(string) {
		mu.Lock()
		count++
		mu.Unlock()
		done <- struct{}{}
	}
	streamer.AddConsumer(recordAndSignal)
	streamer.AddConsumer(recordAndSignal)

	transport := NewExternalTmuxStreamerTransport(streamer)

	require.NoError(t, transport.Send([]byte("frame")))
	<-done
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, count)
}

func TestExternalTmuxStreamerTransport_should_RegisterPresenceConsumer_When_Constructed(t *testing.T) {
	streamer := newTestExternalTmuxStreamer()

	before := streamer.ConsumerCount()
	transport := NewExternalTmuxStreamerTransport(streamer)
	require.Equal(t, before+1, streamer.ConsumerCount())

	require.NoError(t, transport.Close())
	require.Equal(t, before, streamer.ConsumerCount())
}

func TestExternalTmuxStreamerTransport_should_BeIdempotent_When_CloseCalledTwice(t *testing.T) {
	streamer := newTestExternalTmuxStreamer()
	transport := NewExternalTmuxStreamerTransport(streamer)

	before := streamer.ConsumerCount()
	require.NoError(t, transport.Close())
	require.Equal(t, before-1, streamer.ConsumerCount())

	// Second Close must not attempt to remove an already-removed consumer
	// key and must not error or panic.
	require.NoError(t, transport.Close())
	require.Equal(t, before-1, streamer.ConsumerCount())
}

func TestExternalTmuxStreamerTransport_should_ReturnNilError_When_SendCalledWithNoConsumers(t *testing.T) {
	streamer := newTestExternalTmuxStreamer()
	// Attach and immediately close so the presence consumer is gone too --
	// Send must still be safe (and a no-op observably) with zero consumers.
	transport := NewExternalTmuxStreamerTransport(streamer)
	require.NoError(t, transport.Close())

	require.NoError(t, transport.Send([]byte("nobody listening")))
}

// TestAttachExternalTmuxStreamerTransportToHub_should_DeliverHubBroadcastToConsumers_When_Attached
// proves the wiring end-to-end: a hub.Broadcast call reaches a consumer
// registered on the underlying *ExternalTmuxStreamer via the
// ExternalTmuxStreamerTransport attached by
// AttachExternalTmuxStreamerTransportToHub, exactly like a real ssq-mux
// client's own AddConsumer callback would.
func TestAttachExternalTmuxStreamerTransportToHub_should_DeliverHubBroadcastToConsumers_When_Attached(t *testing.T) {
	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	streamer := newTestExternalTmuxStreamer()

	var mu sync.Mutex
	var received []string
	done := make(chan struct{}, 1)
	streamer.AddConsumer(func(content string) {
		mu.Lock()
		received = append(received, content)
		mu.Unlock()
		done <- struct{}{}
	})

	AttachExternalTmuxStreamerTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount())

	hub.Broadcast([]byte("hub says hello"))
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"hub says hello"}, received)
}

// TestAttachExternalTmuxStreamerTransportToHub_should_AttachOnlyOnce_When_CalledForMultipleBrowserConnections
// proves the idempotency guarantee: two browser connections attaching to the
// same hub (each independently calling
// AttachExternalTmuxStreamerTransportToHub) must not register the session's
// ExternalTmuxStreamer as a second transport subscriber.
func TestAttachExternalTmuxStreamerTransportToHub_should_AttachOnlyOnce_When_CalledForMultipleBrowserConnections(t *testing.T) {
	hub := streamhub.NewStreamHub("test-session", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	streamer := newTestExternalTmuxStreamer()

	before := streamer.ConsumerCount()

	// Simulate a first browser connection attaching...
	AttachExternalTmuxStreamerTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount())
	require.Equal(t, before+1, streamer.ConsumerCount(),
		"first attach must register exactly one presence consumer on the ExternalTmuxStreamer")

	// ...and a second browser connection attaching to the same hub.
	AttachExternalTmuxStreamerTransportToHub(hub, streamer)
	require.Equal(t, 1, hub.SubscriberCount(),
		"a second Attach call for the same hub must not add a second subscriber")
	require.Equal(t, before+1, streamer.ConsumerCount(),
		"a second Attach call must not register a second presence consumer")
}
