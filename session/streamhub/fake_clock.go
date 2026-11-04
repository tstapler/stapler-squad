package streamhub

import (
	"sort"
	"sync"
	"time"
)

// FakeClock is a manually driven Clock for tests. Time moves only through
// Advance, which runs every due callback synchronously on the caller's
// goroutine, so a test sequences hub timers without sleeping.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

// NewFakeClock returns a FakeClock starting at start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

type fakeTimer struct {
	c        *FakeClock
	deadline time.Time
	f        func()
	done     bool
}

// Now returns the fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc arms f to run once the fake time reaches now+d.
func (c *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, deadline: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

// Stop reports whether the timer was still pending.
func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	wasPending := !t.done
	t.done = true
	return wasPending
}

// Pending returns the number of armed timers that have neither fired nor been stopped.
func (c *FakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.done {
			n++
		}
	}
	return n
}

// Advance moves time forward by d, running due callbacks in deadline order.
// Callbacks run without the clock's lock held, so they may arm or stop timers.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var due []*fakeTimer
		live := c.timers[:0]
		for _, t := range c.timers {
			switch {
			case t.done:
			case !t.deadline.After(target):
				due = append(due, t)
			default:
				live = append(live, t)
			}
		}
		c.timers = live
		if len(due) == 0 {
			c.now = target
			c.mu.Unlock()
			return
		}
		sort.SliceStable(due, func(i, j int) bool { return due[i].deadline.Before(due[j].deadline) })
		t := due[0]
		t.done = true
		if t.deadline.After(c.now) {
			c.now = t.deadline
		}
		// Timers beyond the first stay armed for the next loop pass.
		c.timers = append(c.timers, due[1:]...)
		c.mu.Unlock()
		t.f()
	}
}
