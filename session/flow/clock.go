package flow

import "time"

// Clock is the injected time source; window logic never calls time.Now directly.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the wall clock in UTC.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock always returns T; for tests and deterministic callers.
type FixedClock struct{ T time.Time }

// Now implements Clock.
func (c FixedClock) Now() time.Time { return c.T.UTC() }
