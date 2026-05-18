package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// funcLifecycleListener is a test-only LifecycleListener that invokes a function.
type funcLifecycleListener struct {
	fn func(LifecycleEvent, string)
}

func (f *funcLifecycleListener) OnLifecycleEvent(event LifecycleEvent, reason string) {
	f.fn(event, reason)
}

// TestLifecycleCallbackConcurrency verifies that fireLifecycleEvent is data-race free
// when called from multiple goroutines concurrently and delivers every event.
func TestLifecycleCallbackConcurrency(t *testing.T) {
	inst := &Instance{Title: "concurrency-test"}

	var counter int64
	inst.RegisterLifecycleListener(&funcLifecycleListener{
		fn: func(_ LifecycleEvent, _ string) {
			atomic.AddInt64(&counter, 1)
		},
	})

	const goroutines = 20
	var wg sync.WaitGroup
	panicked := make(chan interface{}, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicked <- r
				}
			}()
			inst.fireLifecycleEvent(EventStarted, "concurrent-test")
		}()
	}

	wg.Wait()
	close(panicked)

	for p := range panicked {
		t.Errorf("panic in fireLifecycleEvent: %v", p)
	}

	if got := atomic.LoadInt64(&counter); got != goroutines {
		t.Errorf("expected counter=%d, got=%d", goroutines, got)
	}

	// Verify listener list is not corrupted.
	inst.lifecycleListenersMu.Lock()
	listenerCount := len(inst.lifecycleListeners)
	inst.lifecycleListenersMu.Unlock()
	if listenerCount != 1 {
		t.Errorf("expected 1 listener after concurrent fires, got %d", listenerCount)
	}
}

// TestTransitionToErrorInCallback verifies that transitionTo returns an error for
// an invalid state transition rather than panicking or silently succeeding.
// This validates the fix replacing _ = i.transitionTo(Stopped) with error logging.
func TestTransitionToErrorInCallback(t *testing.T) {
	// Stopped is a terminal state — Stopped→Stopped must return an error.
	inst := &Instance{Title: "transition-test", Status: Stopped}

	err := inst.transitionTo(context.Background(), Stopped)
	if err == nil {
		t.Error("expected ErrInvalidTransition for Stopped→Stopped, got nil")
	}

	// Confirm the instance is still in Stopped status after the failed self-transition.
	if inst.Status != Stopped {
		t.Errorf("test setup: instance should be Stopped, got %s", inst.Status)
	}
}
