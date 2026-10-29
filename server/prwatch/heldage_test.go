package prwatch

import (
	"testing"
	"time"
)

func newHA(notice, ceiling time.Duration) (*HeldAge, *fakeClock, *fakeNotifier) {
	clk := newFakeClock()
	n := &fakeNotifier{}
	s := Settings{HeldNoticeAfter: notice, HeldAgeCeiling: ceiling}
	return NewHeldAge(clk, s, NewNotifications(n, clk, s, nil)), clk, n
}

func heldEvent(created time.Time) HeldEvent {
	return HeldEvent{SessionUUID: "s1", Repo: "o/r", PR: 7, CreatedAt: created, Reason: HoldBusy}
}

func countKind(n *fakeNotifier, k NotificationKind) int {
	c := 0
	for _, x := range n.Sent() {
		if x.Kind == k {
			c++
		}
	}
	return c
}

func TestHeldAge_should_AckDropAndNotifyOnce_When_HeldItemOlderThanCeiling(t *testing.T) {
	h, clk, n := newHA(60*time.Minute, 24*time.Hour)
	it := heldEvent(clk.Now().Add(-25 * time.Hour))
	for i := 0; i < 3; i++ {
		if got := h.Check(it); got != HeldDrop {
			t.Fatalf("verdict = %v", got)
		}
	}
	if c := countKind(n, NotifyHeldDropped); c != 1 {
		t.Fatalf("HeldDropped = %d, want 1", c)
	}
}

func TestHeldAge_should_NotifyHeldLongOncePerSessionAndPR_When_ItemHeldPastNoticeThreshold(t *testing.T) {
	h, clk, n := newHA(60*time.Minute, 24*time.Hour)
	it := heldEvent(clk.Now())
	clk.Advance(59 * time.Minute)
	if h.Check(it) != HeldKeep || countKind(n, NotifyHeldLong) != 0 {
		t.Fatal("notice before threshold")
	}
	clk.Advance(time.Minute)
	h.Check(it)
	h.Check(it)
	if c := countKind(n, NotifyHeldLong); c != 1 {
		t.Fatalf("HeldLong = %d, want 1", c)
	}
	msg := n.Sent()[0].Message
	if want := "the session is busy"; !contains(msg, want) {
		t.Fatalf("notice lacks hold reason %q: %s", want, msg)
	}
	// A delivered steer clears the key; a later hold notifies again.
	h.Delivered("s1", "o/r", 7)
	h.Check(it)
	if c := countKind(n, NotifyHeldLong); c != 2 {
		t.Fatalf("HeldLong after clear = %d, want 2", c)
	}
	// The ceiling still raises HeldDropped.
	clk.Advance(24 * time.Hour)
	if h.Check(it) != HeldDrop || countKind(n, NotifyHeldDropped) != 1 {
		t.Fatal("ceiling drop not raised")
	}
}

func TestHeldAge_should_HonorConfiguredNoticeThreshold_When_ConfigOverridesDefault(t *testing.T) {
	h, clk, n := newHA(10*time.Minute, 24*time.Hour)
	it := heldEvent(clk.Now().Add(-9 * time.Minute))
	h.Check(it)
	if countKind(n, NotifyHeldLong) != 0 {
		t.Fatal("notice at 9m")
	}
	clk.Advance(time.Minute)
	h.Check(it)
	if countKind(n, NotifyHeldLong) != 1 {
		t.Fatal("no notice at 10m")
	}
}

func TestHeldAge_should_UseConfiguredCeiling_When_ConfigOverridesDefault(t *testing.T) {
	h, clk, _ := newHA(60*time.Minute, 2*time.Hour)
	if got := h.Check(heldEvent(clk.Now().Add(-(1*time.Hour + 59*time.Minute)))); got != HeldKeep {
		t.Fatalf("1h59m = %v", got)
	}
	if got := h.Check(heldEvent(clk.Now().Add(-2 * time.Hour))); got != HeldDrop {
		t.Fatalf("2h = %v", got)
	}
}

func TestHeldAge_should_DropGoneAndHoldOrCoverSuspended_When_SessionNotLive(t *testing.T) {
	for _, k := range []Kind{KindCI, KindComment, KindReview} {
		if v, _ := SessionGate(StateGone, k, false, ModeAll); v != SessionDrop {
			t.Fatalf("gone %s = %v", k, v)
		}
	}
	if v, _ := SessionGate(StateLive, KindComment, false, ModeAll); v != SessionDeliver {
		t.Fatal("live must deliver")
	}
	cases := []struct {
		name string
		kind Kind
		cr   bool
		mode HumanFeedbackMode
		want SessionVerdict
	}{
		{"ci", KindCI, false, ModeAll, SessionDrop},
		{"changes requested, all", KindReview, true, ModeAll, SessionDrop},
		{"changes requested, allowlist", KindReview, true, ModeAllowlist, SessionHold},
		{"other review state", KindReview, false, ModeAll, SessionHold},
		{"comment", KindComment, false, ModeAll, SessionHold},
		{"inline comment", KindReviewComment, false, ModeAll, SessionHold},
	}
	for _, c := range cases {
		v, r := SessionGate(StateSuspended, c.kind, c.cr, c.mode)
		if v != c.want {
			t.Fatalf("%s: %v", c.name, v)
		}
		if v == SessionHold && r != HoldSessionSuspended {
			t.Fatalf("%s: reason %q", c.name, r)
		}
	}
}

func TestHeldAge_should_HoldNotTrackedUnlessGone_When_SessionsPortReportsState(t *testing.T) {
	if v, r := NotTrackedGate(StateLive); v != SessionHold || r != HoldNotTracked {
		t.Fatalf("live: %v %q", v, r)
	}
	if v, _ := NotTrackedGate(StateGone); v != SessionDrop {
		t.Fatalf("gone: %v", v)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
