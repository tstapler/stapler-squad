package session

import (
	"testing"
	"time"
)

func TestConflictRetryLedger_TakeThrottlesPerItem(t *testing.T) {
	var g conflictRetryLedger
	t0 := time.Unix(1_000_000, 0)

	if !g.take("a", t0, time.Hour) {
		t.Fatal("first take must be granted")
	}
	if g.take("a", t0.Add(59*time.Minute), time.Hour) {
		t.Fatal("take inside the interval must be refused")
	}
	if !g.take("b", t0.Add(time.Minute), time.Hour) {
		t.Fatal("other items are throttled independently")
	}
	if !g.take("a", t0.Add(time.Hour), time.Hour) {
		t.Fatal("take at the interval boundary must be granted")
	}
	g.clear("a")
	if !g.take("a", t0.Add(time.Hour+time.Second), time.Hour) {
		t.Fatal("clear must reset the throttle")
	}
}
