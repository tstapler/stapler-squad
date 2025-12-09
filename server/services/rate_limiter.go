package services

import (
	"sync"

	"golang.org/x/time/rate"
)

// NotificationRateLimiter provides per-session rate limiting for notifications.
// Prevents notification flooding from individual sessions while allowing
// legitimate notification volumes across all sessions.
type NotificationRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewNotificationRateLimiter creates a rate limiter.
// rate: notifications per second (e.g., 10)
// burst: max burst size (e.g., 20)
func NewNotificationRateLimiter(r float64, b int) *NotificationRateLimiter {
	return &NotificationRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(r),
		burst:    b,
	}
}

// Allow checks if a notification is allowed for the given session.
// Returns true if the notification should be processed, false if rate limited.
func (rl *NotificationRateLimiter) Allow(sessionID string) bool {
	rl.mu.Lock()
	limiter, exists := rl.limiters[sessionID]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[sessionID] = limiter
	}
	rl.mu.Unlock()

	return limiter.Allow()
}

// Cleanup removes rate limiters for sessions that are no longer active.
// Should be called periodically to prevent memory leaks.
func (rl *NotificationRateLimiter) Cleanup(activeSessions []string) {
	active := make(map[string]bool)
	for _, id := range activeSessions {
		active[id] = true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for sessionID := range rl.limiters {
		if !active[sessionID] {
			delete(rl.limiters, sessionID)
		}
	}
}

// Reset removes the rate limiter for a specific session.
// Useful for testing or when a session is recreated.
func (rl *NotificationRateLimiter) Reset(sessionID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limiters, sessionID)
}

// Count returns the number of active rate limiters (for monitoring).
func (rl *NotificationRateLimiter) Count() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.limiters)
}
