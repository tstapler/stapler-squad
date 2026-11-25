package triggerlimit

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

func TestTriggerRateLimiter_Allow_RefillsAfterWindow(t *testing.T) {
	t.Parallel()
	lim := &TriggerRateLimiter{limit: rate.Every(5 * time.Millisecond), burst: 1}
	id := uuid.New()

	if !lim.Allow(id) {
		t.Fatal("expected the first fire to be permitted")
	}
	if lim.Allow(id) {
		t.Fatal("expected an immediate second fire to be throttled (burst 1)")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !lim.Allow(id) {
		if time.Now().After(deadline) {
			t.Fatal("bucket never refilled")
		}
		runtime.Gosched()
	}
}

func TestTriggerRateLimiter_Allow_ExactlyBurstUnderContention(t *testing.T) {
	t.Parallel()
	lim := NewTriggerRateLimiter()
	id := uuid.New()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow(id) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != defaultTriggerRateBurst {
		t.Fatalf("allowed = %d, want exactly the burst (%d)", got, defaultTriggerRateBurst)
	}
}

// TestTriggerRateLimiter_Allow_ZeroValue_DoesNotPanic proves the regression guard: a
// TriggerRateLimiter constructed via its zero value (var t TriggerRateLimiter), not
// NewTriggerRateLimiter, must not panic on first Allow() call. Before the fix, the
// nil limiters map panicked on the assignment t.limiters[workflowID] = lim.
func TestTriggerRateLimiter_Allow_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()
	var lim TriggerRateLimiter

	if !lim.Allow(uuid.New()) {
		t.Fatal("expected first Allow() call on a fresh bucket to be permitted")
	}
}

func TestTriggerRateLimiter_Allow_Constructed_StillWorksPerWorkflow(t *testing.T) {
	t.Parallel()
	lim := NewTriggerRateLimiter()
	a, b := uuid.New(), uuid.New()

	if !lim.Allow(a) {
		t.Fatal("expected first Allow() for workflow a to be permitted")
	}
	if !lim.Allow(b) {
		t.Fatal("expected first Allow() for workflow b to be permitted — independent bucket")
	}
}
