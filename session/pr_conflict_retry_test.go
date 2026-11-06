package session

import (
	"strings"
	"testing"
	"time"
)

func TestConflictRetryLedger_TakeThrottlesPerItem(t *testing.T) {
	var g conflictRetryLedger
	t0 := time.Unix(1_000_000, 0)

	if g.take("a", t0, time.Hour, 0) != conflictRetryGranted {
		t.Fatal("first take must be granted")
	}
	if g.take("a", t0.Add(59*time.Minute), time.Hour, 0) != conflictRetryThrottled {
		t.Fatal("take inside the interval must be throttled")
	}
	if g.take("b", t0.Add(time.Minute), time.Hour, 0) != conflictRetryGranted {
		t.Fatal("other items are throttled independently")
	}
	if g.take("a", t0.Add(time.Hour), time.Hour, 0) != conflictRetryGranted {
		t.Fatal("take at the interval boundary must be granted")
	}
	g.clear("a")
	if g.take("a", t0.Add(time.Hour+time.Second), time.Hour, 0) != conflictRetryGranted {
		t.Fatal("clear must reset the throttle")
	}
}

func TestConflictRetryLedger_ExhaustsAfterMaxGrants_AndNotifiesOnce(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < conflictRetryMaxAttempts; i++ {
		if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
			t.Fatalf("grant %d must succeed", i+1)
		}
		now = now.Add(time.Hour)
	}
	if g.take("a", now, time.Hour, 0) != conflictRetryExhausted {
		t.Fatal("grant past the max must report exhaustion")
	}
	if !g.markExhaustedNotified("a") {
		t.Fatal("first exhaustion notice must be allowed")
	}
	if g.markExhaustedNotified("a") {
		t.Fatal("the exhaustion notice must fire once")
	}
	g.clear("a")
	if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
		t.Fatal("clear must re-arm an exhausted item")
	}
}

func TestConflictRetryLedger_RefundKeepsThrottleButReturnsTheGrant(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < conflictRetryMaxAttempts; i++ {
		if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
			t.Fatalf("grant %d must succeed", i+1)
		}
		now = now.Add(time.Hour)
	}
	g.refund("a")
	if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
		t.Fatal("a refunded grant must be available again")
	}
	g.refund("a")
	if g.take("a", now.Add(time.Minute), time.Hour, 0) != conflictRetryThrottled {
		t.Fatal("a refund must not lift the hourly throttle")
	}
	g.refund("missing") // no entry: no-op
}

// A fix attempt that fails to start on every tick must stay on the hourly throttle: the
// live loop (one pr_pending<->in_progress flip pair per ~minute) came from a refund of the
// only grant dropping grants to 0, which the throttle treated as "never granted".
func TestConflictRetryLedger_RefundOfOnlyGrantStillThrottles(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	attempts := 0
	for i := 0; i < 120; i++ { // two hours of one-minute ticks
		if g.take("a", now, time.Hour, 0) == conflictRetryGranted {
			attempts++
			g.refund("a") // the spawn failed
		}
		now = now.Add(time.Minute)
	}
	if attempts > 2 {
		t.Fatalf("a failing spawn got %d attempts in 2h; want at most 2 (hourly)", attempts)
	}
}

func TestConflictRetryLedger_RefundsAreBoundedSoFailuresEventuallyExhaust(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	granted := 0
	for i := 0; i < 100; i++ { // hourly ticks, every spawn fails
		if g.take("a", now, time.Hour, 0) == conflictRetryGranted {
			granted++
			g.refund("a")
		}
		now = now.Add(time.Hour)
	}
	if want := conflictRetryMaxAttempts + conflictRetryMaxRefunds; granted != want {
		t.Fatalf("persistently failing spawn got %d attempts; want exactly %d", granted, want)
	}
}

func TestConflictRetryLedger_RowAttemptsDroppingReArmsAnExhaustedItem(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < conflictRetryMaxAttempts; i++ {
		g.take("a", now, time.Hour, 5) // the row is parked at 5 attempts
		now = now.Add(time.Hour)
	}
	if g.take("a", now, time.Hour, 5) != conflictRetryExhausted {
		t.Fatal("setup: the item must be exhausted")
	}
	if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
		t.Fatal("a Reset (attempt counter back to 0) must re-arm the override")
	}
}

// Refunds bound CONSECUTIVE failed starts: a grant that did start a session ends the run, so
// an item that fails to start now and then is not eventually starved of its refunds.
func TestConflictRetryLedger_RefundBudgetResetsAfterSuccessfulStart(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for round := 0; round < 3; round++ {
		for i := 0; i < conflictRetryMaxRefunds; i++ {
			if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
				t.Fatalf("round %d take %d must be granted", round, i)
			}
			g.refund("a")
			now = now.Add(time.Hour)
		}
		if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
			t.Fatalf("round %d: the successful take must be granted", round)
		}
		g.confirmStarted("a")
		now = now.Add(time.Hour)
	}
	// 3 rounds x 1 started session = 3 grants spent; each round's refunds were fully honoured.
	if got := g.sessionsRan("a"); got != 3 {
		t.Fatalf("sessionsRan = %d, want 3", got)
	}
	if g.take("a", now, time.Hour, 0) != conflictRetryGranted {
		t.Fatal("budget left after refunds were reset by successes must still grant")
	}
}

// When the refund cap is hit, failed starts spend grants. The exhaustion message must count
// only grants that really ran a session, and be able to say none did.
func TestConflictRetryLedger_SessionsRanExcludesFailedStarts(t *testing.T) {
	var g conflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for g.take("a", now, time.Hour, 0) == conflictRetryGranted {
		g.refund("a") // every spawn fails to start
		now = now.Add(time.Hour)
	}
	if got := g.sessionsRan("a"); got != 0 {
		t.Fatalf("no fix session ever started, but sessionsRan = %d", got)
	}
	if got, want := g.failedStarts("a"), conflictRetryMaxAttempts+conflictRetryMaxRefunds; got != want {
		t.Fatalf("failedStarts = %d, want %d", got, want)
	}
}

func TestConflictExhaustionMessage_DoesNotClaimSessionsThatNeverRan(t *testing.T) {
	none := conflictExhaustionMessage("Item", 0, 9)
	if strings.Contains(none, "survived") {
		t.Fatalf("message claims sessions ran when none did: %q", none)
	}
	if !strings.Contains(none, "could not be started") {
		t.Fatalf("message should say the fix could not be started: %q", none)
	}
	ran := conflictExhaustionMessage("Item", 6, 0)
	if !strings.Contains(ran, "survived 6 automated fix sessions") {
		t.Fatalf("message lost the normal wording: %q", ran)
	}
	mixed := conflictExhaustionMessage("Item", 4, 2)
	if !strings.Contains(mixed, "4 automated fix sessions") || !strings.Contains(mixed, "2") {
		t.Fatalf("mixed message should report both counts: %q", mixed)
	}
}
