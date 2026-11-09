package tymux

import (
	"testing"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// Per-frame cost of Broadcast to one timed subscriber, including the depth sample and the
// enqueue stamp; the consumer drains concurrently so the queue never fills.
func BenchmarkClientFanoutBroadcastTimed(b *testing.B) {
	f := newClientFanoutRecording(termlat.New(nil))
	id, ch := f.SubscribeTimed()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
			f.Dequeued(id)
		}
	}()
	frame := []byte("x")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Broadcast(frame)
	}
	b.StopTimer()
	f.Unsubscribe(id)
	<-done
}
