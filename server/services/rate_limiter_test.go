package services

import (
	"testing"

	"github.com/tstapler/stapler-squad/testutil"
)

func TestNotificationRateLimiter_Allow(t *testing.T) {
	// Create limiter with 10/sec, burst of 20
	rl := NewNotificationRateLimiter(10, 20)

	sessionID := "test-session"

	// Should allow burst of 20 notifications
	for i := 0; i < 20; i++ {
		if !rl.Allow(sessionID) {
			t.Errorf("Expected Allow() to return true for notification %d within burst", i)
		}
	}

	// 21st should be rate limited
	if rl.Allow(sessionID) {
		t.Error("Expected Allow() to return false after burst exceeded")
	}
}

func TestNotificationRateLimiter_MultipleSessions(t *testing.T) {
	rl := NewNotificationRateLimiter(10, 5)

	session1 := "session-1"
	session2 := "session-2"

	// Exhaust session1's burst
	for i := 0; i < 5; i++ {
		rl.Allow(session1)
	}

	// Session1 should be rate limited
	if rl.Allow(session1) {
		t.Error("Session1 should be rate limited")
	}

	// Session2 should still have its own burst
	if !rl.Allow(session2) {
		t.Error("Session2 should not be rate limited")
	}
}

func TestNotificationRateLimiter_Recovery(t *testing.T) {
	// Create limiter with high rate for faster test
	rl := NewNotificationRateLimiter(100, 1)

	sessionID := "test-session"

	// Use up burst
	rl.Allow(sessionID)

	// Should be rate limited
	if rl.Allow(sessionID) {
		t.Error("Expected rate limiting after burst")
	}

	// Poll until the rate limiter recovers and allows a request.
	if err := testutil.WaitForCondition(func() bool {
		return rl.Allow(sessionID)
	}, testutil.FastWaitConfig()); err != nil {
		t.Error("Expected Allow() after rate limit recovery")
	}
}

func TestNotificationRateLimiter_Cleanup(t *testing.T) {
	rl := NewNotificationRateLimiter(10, 20)

	// Create limiters for multiple sessions
	rl.Allow("session-1")
	rl.Allow("session-2")
	rl.Allow("session-3")

	if rl.Count() != 3 {
		t.Errorf("Expected 3 limiters, got %d", rl.Count())
	}

	// Cleanup keeping only session-2
	rl.Cleanup([]string{"session-2"})

	if rl.Count() != 1 {
		t.Errorf("Expected 1 limiter after cleanup, got %d", rl.Count())
	}
}

func TestNotificationRateLimiter_Reset(t *testing.T) {
	rl := NewNotificationRateLimiter(10, 2)

	sessionID := "test-session"

	// Use up burst
	rl.Allow(sessionID)
	rl.Allow(sessionID)

	// Should be rate limited
	if rl.Allow(sessionID) {
		t.Error("Expected rate limiting after burst")
	}

	// Reset the session
	rl.Reset(sessionID)

	// Should have fresh burst
	if !rl.Allow(sessionID) {
		t.Error("Expected Allow() after Reset()")
	}
}
