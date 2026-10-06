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
	t.Parallel()
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
	t.Parallel()
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

// TestDestroy_FiresEventStopped_EvenWhenNeverStarted verifies BUG-027's fix:
// Destroy() must notify lifecycle listeners (e.g. BacklogLifecycleListener's
// ItemSession.EndedAt bookkeeping) on every operator-initiated stop, not just
// natural process exits. This covers the not-started early-return path — the
// case a real live instance never hits, but the bare &Instance{} test double
// used elsewhere in this file exercises cheaply without a real tmux/process
// manager, since !i.started.Load() short-circuits before any real teardown.
func TestDestroy_FiresEventStopped_EvenWhenNeverStarted(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "destroy-event-test"}

	var gotEvent LifecycleEvent
	var gotReason string
	fired := false
	inst.RegisterLifecycleListener(&funcLifecycleListener{
		fn: func(event LifecycleEvent, reason string) {
			fired = true
			gotEvent = event
			gotReason = reason
		},
	})

	if err := inst.Destroy(); err != nil {
		t.Fatalf("Destroy() on a never-started instance should not error, got: %v", err)
	}

	if !fired {
		t.Fatal("expected Destroy() to fire a lifecycle event, but none was fired")
	}
	if gotEvent != EventStopped {
		t.Errorf("expected EventStopped, got %v", gotEvent)
	}
	if gotReason == "" {
		t.Error("expected a non-empty reason for the EventStopped fire")
	}
}
