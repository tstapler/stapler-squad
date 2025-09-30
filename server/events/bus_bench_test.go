package events

import (
	"claude-squad/session"
	"context"
	"testing"
)

// BenchmarkEventBusPublish benchmarks event publishing throughput.
func BenchmarkEventBusPublish(b *testing.B) {
	bus := NewEventBus(100)
	defer bus.Close()

	testEvent := NewSessionCreatedEvent(&session.Instance{Title: "bench-test"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.Publish(testEvent)
	}
}

// BenchmarkEventBusSubscribe benchmarks subscription creation.
func BenchmarkEventBusSubscribe(b *testing.B) {
	bus := NewEventBus(100)
	defer bus.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, subID := bus.Subscribe(ctx)
		bus.Unsubscribe(subID)
		cancel()
	}
}

// BenchmarkEventBusPublishWithSubscribers benchmarks publishing with active subscribers.
func BenchmarkEventBusPublishWithSubscribers(b *testing.B) {
	sizes := []int{1, 10, 100, 1000}

	for _, numSubs := range sizes {
		b.Run(string(rune(numSubs))+"_subscribers", func(b *testing.B) {
			bus := NewEventBus(100)
			defer bus.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create subscribers
			for i := 0; i < numSubs; i++ {
				events, _ := bus.Subscribe(ctx)
				// Drain events in background to prevent blocking
				go func() {
					for range events {
					}
				}()
			}

			testEvent := NewSessionCreatedEvent(&session.Instance{Title: "bench-test"})

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				bus.Publish(testEvent)
			}
		})
	}
}

// BenchmarkEventBusConcurrentPublish benchmarks concurrent publishing from multiple goroutines.
func BenchmarkEventBusConcurrentPublish(b *testing.B) {
	bus := NewEventBus(100)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create subscriber and drain events
	events, _ := bus.Subscribe(ctx)
	go func() {
		for range events {
		}
	}()

	testEvent := NewSessionCreatedEvent(&session.Instance{Title: "bench-test"})

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bus.Publish(testEvent)
		}
	})
}

// BenchmarkEventBusSubscriberCount benchmarks SubscriberCount method.
func BenchmarkEventBusSubscriberCount(b *testing.B) {
	bus := NewEventBus(100)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create some subscribers
	for i := 0; i < 10; i++ {
		bus.Subscribe(ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bus.SubscriberCount()
	}
}

// BenchmarkEventCreation benchmarks event constructor performance.
func BenchmarkEventCreation(b *testing.B) {
	testSession := &session.Instance{Title: "test"}

	b.Run("SessionCreated", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewSessionCreatedEvent(testSession)
		}
	})

	b.Run("SessionUpdated", func(b *testing.B) {
		fields := []string{"title", "category"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewSessionUpdatedEvent(testSession, fields)
		}
	})

	b.Run("SessionDeleted", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewSessionDeletedEvent("test-id")
		}
	})

	b.Run("SessionStatusChanged", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewSessionStatusChangedEvent(testSession, session.Running, session.Paused)
		}
	})
}

// BenchmarkEventBusEndToEnd benchmarks complete pub/sub cycle.
func BenchmarkEventBusEndToEnd(b *testing.B) {
	bus := NewEventBus(1000)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, _ := bus.Subscribe(ctx)

	// Count received events
	received := 0
	go func() {
		for range events {
			received++
		}
	}()

	testEvent := NewSessionCreatedEvent(&session.Instance{Title: "bench-test"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.Publish(testEvent)
	}

	// Note: We don't wait for all events to be received to avoid benchmarking goroutine scheduling
}
