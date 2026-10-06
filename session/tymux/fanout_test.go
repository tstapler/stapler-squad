package tymux

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Story 2.3.2 acceptance: "Given two subscribers registered via
// SubscribeToControlModeUpdates(), When one Output event arrives on the
// standing stream, Then both subscriber channels receive the same bytes."
// This test exercises ClientFanout directly (the unit under Story 2.3.2);
// stream_test.go covers the same contract wired through a real
// tymuxGRPCSession + standing stream.
func TestClientFanout_Broadcast_DeliversToEverySubscriber(t *testing.T) {
	f := NewClientFanout()
	id1, ch1 := f.Subscribe()
	id2, ch2 := f.Subscribe()
	require.NotEqual(t, id1, id2, "each subscriber must get a distinct id")

	f.Broadcast([]byte("hello"))

	select {
	case got := <-ch1:
		assert.Equal(t, []byte("hello"), got)
	case <-time.After(time.Second):
		t.Fatal("subscriber 1 never received the broadcast")
	}
	select {
	case got := <-ch2:
		assert.Equal(t, []byte("hello"), got)
	case <-time.After(time.Second):
		t.Fatal("subscriber 2 never received the broadcast")
	}
}

func TestClientFanout_Unsubscribe_ClosesChannelAndStopsDelivery(t *testing.T) {
	f := NewClientFanout()
	id, ch := f.Subscribe()

	f.Unsubscribe(id)

	_, open := <-ch
	assert.False(t, open, "Unsubscribe must close the subscriber's channel")

	// Broadcasting after Unsubscribe must not panic or resurrect the entry.
	f.Broadcast([]byte("ignored"))
	assert.Equal(t, 0, f.count())
}

func TestClientFanout_Broadcast_NonBlockingWhenSubscriberFull(t *testing.T) {
	f := NewClientFanout()
	_, ch := f.Subscribe()

	// Fill the subscriber's buffer, then send one more — Broadcast must
	// drop it rather than block the caller (the reader goroutine in a real
	// standing stream).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < subscriberBufferSize+10; i++ {
			f.Broadcast([]byte{byte(i)})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked instead of dropping frames for a full subscriber")
	}

	// Drain whatever made it through — just confirming no deadlock/panic.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestClientFanout_Subscribe_ConcurrentUseIsRace_Free(t *testing.T) {
	f := NewClientFanout()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, ch := f.Subscribe()
			f.Broadcast([]byte("x"))
			<-ch
			f.Unsubscribe(id)
		}()
	}
	wg.Wait()
}
