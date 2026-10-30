package session

import (
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
