package mcp

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketAllow(t *testing.T) {
	tb := newTokenBucket(1, 3)
	for i := 0; i < 3; i++ {
		if !tb.allow("key") {
			t.Fatalf("call %d: expected allow to return true, got false", i+1)
		}
	}
	if tb.allow("key") {
		t.Error("4th call: expected allow to return false (bucket empty), got true")
	}
}

func TestTokenBucketRefill(t *testing.T) {
	fakeNow := time.Now()
	tb := newTokenBucket(1, 3)
	tb.now = func() time.Time { return fakeNow }

	// Drain the bucket.
	for i := 0; i < 3; i++ {
		tb.allow("key")
	}
	if tb.allow("key") {
		t.Fatal("bucket should be empty after draining")
	}

	// Advance fake clock by 1.1s so more than 1 token refills.
	fakeNow = fakeNow.Add(1100 * time.Millisecond)

	if !tb.allow("key") {
		t.Error("expected allow to return true after refill, got false")
	}
}

func TestCreateSessionRateLimit(t *testing.T) {
	// Reset the global limiter to a fresh state by using a local instance
	// that mirrors its parameters: 3/min rate, capacity 3.
	limiter := newTokenBucket(3.0/60.0, 3)
	for i := 0; i < 3; i++ {
		if !limiter.allow("global") {
			t.Fatalf("call %d: expected true, got false", i+1)
		}
	}
	if limiter.allow("global") {
		t.Error("4th call: expected false (rate limit exhausted), got true")
	}
}

func TestWriteRateLimitPerSession(t *testing.T) {
	tb := newTokenBucket(1, 1)

	// Drain S1.
	if !tb.allow("S1") {
		t.Fatal("first S1 allow: expected true")
	}
	if tb.allow("S1") {
		t.Error("second S1 allow: expected false (bucket empty)")
	}

	// S2 should have its own fresh bucket.
	if !tb.allow("S2") {
		t.Error("first S2 allow: expected true (independent bucket)")
	}
}

func TestTokenBucketRaceSafety(t *testing.T) {
	tb := newTokenBucket(100, 100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tb.allow("key")
		}()
	}
	wg.Wait()
}
