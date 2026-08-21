package services

import (
	"sync"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// Default per-Workflow trigger fire rate: 10/minute with a burst of 10 (i.e. up to 10
// fires can happen back-to-back before throttling kicks in, refilling at ~1 every 6s
// thereafter). Chosen over a smaller burst so a legitimate short burst of pushes/events
// isn't immediately throttled — see webhook-triggers plan.md Story 2.4.2.
const (
	defaultTriggerRateLimit = rate.Limit(10.0 / 60.0)
	defaultTriggerRateBurst = 10
)

// TriggerRateLimiter enforces a per-Workflow rate limit on trigger fires (webhook-
// triggers Epic 2.4.2), guarding against a noisy or malicious webhook source spawning
// unbounded sessions. Concrete type, not an interface — one implementation, per
// .claude/rules/interface-pollution-checklist.md. server/workflows.Scheduler consumes
// it through its own narrow triggerRateLimiterGate interface (defined in
// scheduler.go), to avoid a server/workflows -> server/services import.
type TriggerRateLimiter struct {
	mu       sync.Mutex
	limiters map[uuid.UUID]*rate.Limiter
	limit    rate.Limit
	burst    int
}

// NewTriggerRateLimiter creates a TriggerRateLimiter using the default rate (10/min,
// burst 10).
func NewTriggerRateLimiter() *TriggerRateLimiter {
	return &TriggerRateLimiter{
		limiters: make(map[uuid.UUID]*rate.Limiter),
		limit:    defaultTriggerRateLimit,
		burst:    defaultTriggerRateBurst,
	}
}

// Allow reports whether a fire for workflowID is permitted right now, consuming one
// token from that workflow's bucket if so. Each Workflow gets its own independent
// token bucket, created lazily on first use.
func (t *TriggerRateLimiter) Allow(workflowID uuid.UUID) bool {
	t.mu.Lock()
	// ponytail: guards the zero-value case (var t TriggerRateLimiter) where limiters is
	// nil — the only real construction path is NewTriggerRateLimiter, which already
	// initializes both fields, so this is a no-op there.
	if t.limiters == nil {
		t.limiters = make(map[uuid.UUID]*rate.Limiter)
	}
	if t.limit == 0 {
		t.limit = defaultTriggerRateLimit
		t.burst = defaultTriggerRateBurst
	}
	lim, ok := t.limiters[workflowID]
	if !ok {
		lim = rate.NewLimiter(t.limit, t.burst)
		t.limiters[workflowID] = lim
	}
	t.mu.Unlock()

	return lim.Allow()
}
