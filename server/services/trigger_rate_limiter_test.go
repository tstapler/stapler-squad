package services

import (
	"testing"

	"github.com/google/uuid"
)

// TestTriggerRateLimiter_Allow_ZeroValue_DoesNotPanic proves the regression guard: a
// TriggerRateLimiter constructed via its zero value (var t TriggerRateLimiter), not
// NewTriggerRateLimiter, must not panic on first Allow() call. Before the fix, the
// nil limiters map panicked on the assignment t.limiters[workflowID] = lim.
func TestTriggerRateLimiter_Allow_ZeroValue_DoesNotPanic(t *testing.T) {
	var lim TriggerRateLimiter

	if !lim.Allow(uuid.New()) {
		t.Fatal("expected first Allow() call on a fresh bucket to be permitted")
	}
}

func TestTriggerRateLimiter_Allow_Constructed_StillWorksPerWorkflow(t *testing.T) {
	lim := NewTriggerRateLimiter()
	a, b := uuid.New(), uuid.New()

	if !lim.Allow(a) {
		t.Fatal("expected first Allow() for workflow a to be permitted")
	}
	if !lim.Allow(b) {
		t.Fatal("expected first Allow() for workflow b to be permitted — independent bucket")
	}
}
