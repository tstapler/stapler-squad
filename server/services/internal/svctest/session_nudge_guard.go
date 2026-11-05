package services

import (
	"sync"
	"time"
)

type fakeGuardClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeGuardClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeGuardClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
