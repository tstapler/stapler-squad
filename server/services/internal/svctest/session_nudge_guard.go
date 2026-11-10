package svctest

import (
	"sync"
	"time"
)

type FakeGuardClock struct {
	mu sync.Mutex
	T  time.Time
}

func (c *FakeGuardClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.T
}

func (c *FakeGuardClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.T = c.T.Add(d)
}
