package session

import (
	"testing"
	"time"
)

func TestConflictRetryLedger_TakeThrottlesPerItem(t *testing.T) {
	var g ConflictRetryLedger
	t0 := time.Unix(1_000_000, 0)

	if g.Take("a", t0, time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("first take must be granted")
	}
	if g.Take("a", t0.Add(59*time.Minute), time.Hour, 0) != ConflictRetryThrottled {
		t.Fatal("take inside the interval must be throttled")
	}
	if g.Take("b", t0.Add(time.Minute), time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("other items are throttled independently")
	}
	if g.Take("a", t0.Add(time.Hour), time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("take at the interval boundary must be granted")
	}
	g.Clear("a")
	if g.Take("a", t0.Add(time.Hour+time.Second), time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("clear must reset the throttle")
	}
}

func TestConflictRetryLedger_ExhaustsAfterMaxGrants_AndNotifiesOnce(t *testing.T) {
	var g ConflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < ConflictRetryMaxAttempts; i++ {
		if g.Take("a", now, time.Hour, 0) != ConflictRetryGranted {
			t.Fatalf("grant %d must succeed", i+1)
		}
		now = now.Add(time.Hour)
	}
	if g.Take("a", now, time.Hour, 0) != ConflictRetryExhausted {
		t.Fatal("grant past the max must report exhaustion")
	}
	if !g.MarkExhaustedNotified("a") {
		t.Fatal("first exhaustion notice must be allowed")
	}
	if g.MarkExhaustedNotified("a") {
		t.Fatal("the exhaustion notice must fire once")
	}
	g.Clear("a")
	if g.Take("a", now, time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("clear must re-arm an exhausted item")
	}
}

func TestConflictRetryLedger_RefundKeepsThrottleButReturnsTheGrant(t *testing.T) {
	var g ConflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < ConflictRetryMaxAttempts; i++ {
		if g.Take("a", now, time.Hour, 0) != ConflictRetryGranted {
			t.Fatalf("grant %d must succeed", i+1)
		}
		now = now.Add(time.Hour)
	}
	g.Refund("a")
	if g.Take("a", now, time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("a refunded grant must be available again")
	}
	g.Refund("a")
	if g.Take("a", now.Add(time.Minute), time.Hour, 0) != ConflictRetryThrottled {
		t.Fatal("a refund must not lift the hourly throttle")
	}
	g.Refund("missing") // no entry: no-op
}

func TestConflictRetryLedger_RowAttemptsDroppingReArmsAnExhaustedItem(t *testing.T) {
	var g ConflictRetryLedger
	now := time.Unix(1_000_000, 0)
	for i := 0; i < ConflictRetryMaxAttempts; i++ {
		g.Take("a", now, time.Hour, 5) // the row is parked at 5 attempts
		now = now.Add(time.Hour)
	}
	if g.Take("a", now, time.Hour, 5) != ConflictRetryExhausted {
		t.Fatal("setup: the item must be exhausted")
	}
	if g.Take("a", now, time.Hour, 0) != ConflictRetryGranted {
		t.Fatal("a Reset (attempt counter back to 0) must re-arm the override")
	}
}
