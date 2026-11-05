package svctest

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// waitClockPast blocks until the wall clock is at least d past t0, so rows
// stamped from the real clock afterwards sort strictly later than anything
// stamped before t0 (storage exposes no clock injection point).
func WaitClockPast(t testing.TB, t0 time.Time, d time.Duration) {
	t.Helper()
	wait.RequireEventually(t, func() bool { return time.Since(t0) >= d }, time.Second+d, time.Millisecond, "wall clock did not advance")
}
