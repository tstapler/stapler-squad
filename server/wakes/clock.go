// Package wakes is the durable wake-trigger dispatcher: one timer over the
// wake_triggers table, owned here rather than by workflows.Scheduler
// (ADR-006 rule 3). It defines the ports it needs; server wiring supplies
// adapters.
package wakes

import (
	"sync"
	"time"
)

// Timer is the subset of *time.Timer the dispatcher uses.
type Timer interface {
	C() <-chan time.Time
	// Stop prevents the timer from firing; it reports whether it was active.
	Stop() bool
}

// Clock is the time source the dispatcher is injected with. robfig/cron has no
// clock seam, so every time decision in this package goes through it.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// RealClock is the production Clock.
type RealClock struct{}

// Now returns the wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer wraps time.NewTimer.
func (RealClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time { return r.t.C }
func (r realTimer) Stop() bool          { return r.t.Stop() }

// FakeClock is a manually driven Clock for tests. Advance and Set fire the
// timers whose deadline has been reached; BlockUntilTimers lets a test wait
// for the code under test to arm its timer before moving time.
type FakeClock struct {
	mu     sync.Mutex
	cond   *sync.Cond
	now    time.Time
	timers map[*fakeTimer]struct{}
}

// NewFakeClock returns a FakeClock starting at start.
func NewFakeClock(start time.Time) *FakeClock {
	c := &FakeClock{now: start, timers: map[*fakeTimer]struct{}{}}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Now returns the fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer arms a timer for d from the fake now. A non-positive d fires on creation.
func (c *FakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, deadline: c.now.Add(d), ch: make(chan time.Time, 1)}
	if d <= 0 {
		t.ch <- c.now
	} else {
		c.timers[t] = struct{}{}
	}
	c.cond.Broadcast()
	return t
}

// Advance moves time forward by d and fires every timer now due.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(c.now.Add(d))
}

// Set moves time to t, forward or backward (a clock jump), firing due timers.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(t)
}

func (c *FakeClock) setLocked(t time.Time) {
	c.now = t
	for tm := range c.timers {
		if !tm.deadline.After(t) {
			tm.ch <- t
			delete(c.timers, tm)
		}
	}
	c.cond.Broadcast()
}

// ActiveTimers is the number of armed, unfired timers.
func (c *FakeClock) ActiveTimers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// BlockUntilTimers blocks until at least n timers are armed.
func (c *FakeClock) BlockUntilTimers(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.timers) < n {
		c.cond.Wait()
	}
}

// Deadlines returns the deadlines of the armed timers (for assertions).
func (c *FakeClock) Deadlines() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Time, 0, len(c.timers))
	for tm := range c.timers {
		out = append(out, tm.deadline)
	}
	return out
}

type fakeTimer struct {
	clock    *FakeClock
	deadline time.Time
	ch       chan time.Time
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	_, active := t.clock.timers[t]
	delete(t.clock.timers, t)
	t.clock.cond.Broadcast()
	return active
}

// BlockUntilTimerAt blocks until a timer with exactly this deadline is armed.
func (c *FakeClock) BlockUntilTimerAt(deadline time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for !c.hasDeadlineLocked(deadline) {
		c.cond.Wait()
	}
}

func (c *FakeClock) hasDeadlineLocked(deadline time.Time) bool {
	for tm := range c.timers {
		if tm.deadline.Equal(deadline) {
			return true
		}
	}
	return false
}
