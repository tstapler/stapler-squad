package wait

import (
	"fmt"
	"testing"
	"time"
)

// RequireEventually polls condition until it returns true, failing t via
// t.Fatalf if it never does within baseTimeout scaled by ScaleTimeout.
// Drop-in-shaped replacement for testify's require.Eventually(t, condition,
// timeout, tick, msgAndArgs...), differing only in that timeout is a base
// value stretched to current machine load rather than a literal bound.
//
// Unlike require.Eventually, a timeout here logs via t.Logf partway through
// (at roughly the halfway point) so a long-running CI/local log shows a
// "still waiting" line distinguishing an in-progress, load-slowed wait from
// a silently hung one before the final failure -- see LoadFactor's doc
// comment for the underlying motivation (BUG-103).
func RequireEventually(t testing.TB, condition func() bool, baseTimeout, tick time.Duration, msgAndArgs ...any) {
	t.Helper()
	if ok, tr := pollEventually(condition, baseTimeout, tick, loggingProgress(t)); !ok {
		t.Fatalf("%s: %s", tr.err(), formatMsgAndArgs(msgAndArgs))
	}
}

// pollEventually is RequireEventually's testing.TB-free core, so it can be
// unit tested without a real *testing.T.
func pollEventually(condition func() bool, baseTimeout, tick time.Duration, onHalfway func(elapsed, remaining time.Duration)) (bool, timeoutReport) {
	scaled := ScaleTimeout(baseTimeout)
	tr := timeoutReport{description: "condition", baseTimeout: baseTimeout, scaledTimeout: scaled, pollInterval: tick, start: time.Now()}
	deadline := tr.start.Add(scaled)
	halfwayLogged := false

	if condition() {
		return true, tr
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		now := <-ticker.C
		tr.polls++
		if condition() {
			return true, tr
		}
		if now.After(deadline) {
			return false, tr
		}
		if !halfwayLogged && onHalfway != nil && now.Sub(tr.start) >= scaled/2 {
			halfwayLogged = true
			onHalfway(now.Sub(tr.start), deadline.Sub(now))
		}
	}
}

func loggingProgress(t testing.TB) func(elapsed, remaining time.Duration) {
	return func(elapsed, remaining time.Duration) {
		t.Logf("wait: still waiting after %v (up to %v remaining, load factor %.1fx) -- if this repeats, the machine is likely contended, not the condition stuck",
			elapsed.Round(time.Millisecond), remaining.Round(time.Millisecond), LoadFactor())
	}
}

func formatMsgAndArgs(msgAndArgs []any) string {
	if len(msgAndArgs) == 0 {
		return "condition never satisfied"
	}
	if s, ok := msgAndArgs[0].(string); ok {
		return fmt.Sprintf(s, msgAndArgs[1:]...)
	}
	return fmt.Sprint(msgAndArgs...)
}
