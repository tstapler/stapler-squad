package session

import (
	"sync"
	"testing"

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
