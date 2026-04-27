package mcp

import (
	"sync"
	"time"
)

// tokenBucket is a simple per-key token-bucket rate limiter.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   map[string]*bucket
	rate     float64 // tokens added per second
	capacity float64 // max tokens
	now      func() time.Time
}

type bucket struct {
	tokens    float64
	lastRefil time.Time
}

func newTokenBucket(rate, capacity float64) *tokenBucket {
	return &tokenBucket{
		tokens:   make(map[string]*bucket),
		rate:     rate,
		capacity: capacity,
		now:      time.Now,
	}
}

// allow returns true if the key has tokens available and consumes one.
func (tb *tokenBucket) allow(key string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.tokens[key]
	if !ok {
		b = &bucket{tokens: tb.capacity, lastRefil: tb.now()}
		tb.tokens[key] = b
	}

	now := tb.now()
	elapsed := now.Sub(b.lastRefil).Seconds()
	added := elapsed * tb.rate
	if b.tokens+added > tb.capacity {
		b.tokens = tb.capacity
	} else {
		b.tokens += added
	}
	b.lastRefil = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// createSessionLimiter limits create_session to 3 per minute globally.
// Using "global" as the key since create_session has no per-session identity yet.
var createSessionLimiter = newTokenBucket(3.0/60.0, 3)
