package matview

import (
	"sync"
	"time"
)

// Clock is the view's time source. Every timer and timestamp goes through it so
// tests advance time deterministically instead of sleeping.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f in its own goroutine after d, like time.AfterFunc.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the stoppable handle AfterFunc returns.
type Timer interface {
	Stop() bool
}

// RealClock is the production Clock.
type RealClock struct{}

// Now returns the wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// AfterFunc wraps time.AfterFunc.
func (RealClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// FakeClock is a manually driven Clock. Advance runs due callbacks
// synchronously on the caller's goroutine, in deadline order, so a test sees
// every effect of a timer before Advance returns.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c      *FakeClock
	due    time.Time
	f      func()
	active bool
}

// NewFakeClock returns a clock frozen at start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

// Now returns the fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc schedules f at now+d.
func (c *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, due: c.now.Add(d), f: f, active: true}
	c.timers = append(c.timers, t)
	return t
}

// Stop cancels the timer and reports whether it was still pending.
func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

// Pending reports how many timers are armed.
func (c *FakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if t.active {
			n++
		}
	}
	return n
}

// Advance moves time forward by d, firing every timer that comes due, earliest
// first. A callback may arm new timers; those due within the window also fire.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		t := c.popDue(target)
		if t == nil {
			break
		}
		t.f()
	}
	c.mu.Lock()
	c.now = target
	c.mu.Unlock()
}

func (c *FakeClock) popDue(target time.Time) *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	var best *fakeTimer
	live := c.timers[:0]
	for _, t := range c.timers {
		if !t.active {
			continue
		}
		live = append(live, t)
		if !t.due.After(target) && (best == nil || t.due.Before(best.due)) {
			best = t
		}
	}
	c.timers = live
	if best == nil {
		return nil
	}
	best.active = false
	if best.due.After(c.now) {
		c.now = best.due
	}
	return best
}
