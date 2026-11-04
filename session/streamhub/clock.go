package streamhub

import "time"

// Clock is the hub's time source. Every hub timer and timestamp goes through
// it so tests can advance time deterministically instead of sleeping.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f in its own goroutine after d, like time.AfterFunc.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the stoppable handle AfterFunc returns.
type Timer interface {
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// WithClock replaces the hub's real clock, e.g. with a fake in tests.
func WithClock(c Clock) HubOption {
	return func(h *StreamHub) { h.clock = c }
}
