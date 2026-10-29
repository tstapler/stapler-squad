package prwatch

import (
	"testing"
	"time"
)

const (
	cwSession = "s1"
	cwRepo    = "o/r"
	cwPR      = 7
)

func cwSettings(iv time.Duration) Settings { return Settings{MinSteerInterval: iv} }

func newCW(iv time.Duration) (*WindowCoalescer, *fakeClock, *fakeMetrics) {
	clk := newFakeClock()
	m := &fakeMetrics{}
	return NewWindowCoalescer(clk, cwSettings(iv), m), clk, m
}

func hfKey() CoalesceKey {
	return CoalesceKey{SessionUUID: cwSession, Repo: cwRepo, PR: cwPR, Class: ClassHumanFeedback}
}

func ciKey() CoalesceKey {
	return CoalesceKey{SessionUUID: cwSession, Repo: cwRepo, PR: cwPR, Class: ClassCI}
}

func TestCoalesce_should_SteerFirstEventImmediatelyAndHoldLaterCommentsUntilWindowCloses_When_WithinMinInterval(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("first = %s", got)
	}
	c.RecordSteer(hfKey(), "")
	clk.Advance(60 * time.Second)
	for i := 0; i < 4; i++ {
		if got := c.Decide(hfKey(), "", i); got != CoalesceHold {
			t.Fatalf("tick %d = %s", i, got)
		}
		clk.Advance(60 * time.Second)
	}
	// t0+300s: window closed, the held items fold into one steer.
	if got := c.Decide(hfKey(), "", 2); got != CoalesceSteer {
		t.Fatalf("at close = %s", got)
	}
}

func TestCoalesce_should_NotLoseSecondHumanComment_When_ItArrivesInsideTheWindow(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	clk.Advance(60 * time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceHold {
		t.Fatalf("B inside window = %s, want hold (never acked early)", got)
	}
	clk.Advance(240 * time.Second)
	if got := c.Decide(hfKey(), "", 1); got != CoalesceSteer {
		t.Fatalf("B at close = %s, want steer", got)
	}
}

func TestCoalesce_should_SteerImmediately_When_EventArrivesAfterWindowAndNothingIsHeld(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	clk.Advance(301 * time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("got %s", got)
	}
	c.RecordSteer(hfKey(), "")
	clk.Advance(time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceHold {
		t.Fatalf("new window not started: %s", got)
	}
}

func TestCoalesce_should_MergeKindsIntoOneSteerAndAckAll_When_SeveralKindsInSameTick(t *testing.T) {
	// The coalescer's part: a due review and a CI failure for a new SHA both
	// decide steer in one tick, so the engine merges them into one steer.
	c, _, _ := newCW(300 * time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("review = %s", got)
	}
	if got := c.Decide(ciKey(), "sha1", 0); got != CoalesceSteer {
		t.Fatalf("ci = %s", got)
	}
	got := mergeKinds([]Kind{KindReview, KindCI})
	if got != "review, CI failure" {
		t.Fatalf("merged = %q", got)
	}
}

func mergeKinds(ks []Kind) string {
	s := ""
	for i, k := range ks {
		if i > 0 {
			s += ", "
		}
		s += k.Rendered()
	}
	return s
}

func TestCoalesce_should_NotCoalesce_When_MinSteerIntervalIsZero(t *testing.T) {
	c, clk, _ := newCW(0)
	for i := 0; i < 3; i++ {
		if got := c.Decide(hfKey(), "", 0); got != CoalesceSteer {
			t.Fatalf("hf %d = %s", i, got)
		}
		c.RecordSteer(hfKey(), "")
		if got := c.Decide(ciKey(), "same", 0); got != CoalesceSteer {
			t.Fatalf("ci %d = %s", i, got)
		}
		c.RecordSteer(ciKey(), "same")
		clk.Advance(time.Second)
	}
	if _, ok := c.NextEligible(hfKey()); ok {
		t.Fatal("NextEligible must report now when disabled")
	}
}

func TestCoalesce_should_KeepWindowPerSessionPRAndClass_When_TwoPRsTwoSessionsOrTwoClasses(t *testing.T) {
	c, _, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	otherPR := hfKey()
	otherPR.PR = 8
	otherSession := hfKey()
	otherSession.SessionUUID = "s2"
	for name, k := range map[string]CoalesceKey{"pr": otherPR, "session": otherSession} {
		if got := c.Decide(k, "", 0); got != CoalesceSteer {
			t.Fatalf("%s suppressed: %s", name, got)
		}
	}
	// Other class: a SHA-less CI failure is not held by the feedback window.
	if got := c.Decide(ciKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("ci class suppressed: %s", got)
	}
}

func TestCoalesce_should_CollapseCommentSpamToOneSteer_When_100CommentsInWindow(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	clk.Advance(30 * time.Second)
	held, folded := 0, 0
	for i := 0; i < 100; i++ {
		switch c.Decide(hfKey(), "", held) {
		case CoalesceHold:
			held++
		case CoalesceFoldOverCap:
			folded++
		default:
			t.Fatalf("item %d: unexpected action", i)
		}
	}
	if held != 50 || folded != 50 {
		t.Fatalf("held=%d folded=%d", held, folded)
	}
	clk.Advance(300 * time.Second)
	steers := 0
	for i := 0; i < held; i++ {
		if c.Decide(hfKey(), "", i) == CoalesceSteer {
			steers++
		}
	}
	if steers != held {
		t.Fatalf("all held items must be released with the steer, got %d", steers)
	}
	c.RecordSteer(hfKey(), "")
	if got := c.Decide(hfKey(), "", 0); got != CoalesceHold {
		t.Fatalf("no second steer expected, got %s", got)
	}
}

func TestCoalesce_should_SteerCIFailure_When_CommentSteerWasWithinWindowAndHeadSHAIsNew(t *testing.T) {
	c, clk, m := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	clk.Advance(90 * time.Second)
	if got := c.Decide(ciKey(), "sha-new", 0); got != CoalesceSteer {
		t.Fatalf("ci = %s", got)
	}
	// A human comment in the same window stays held.
	if got := c.Decide(hfKey(), "", 0); got != CoalesceHold {
		t.Fatalf("comment = %s", got)
	}
	c.RecordSteer(ciKey(), "sha-new")
	if n := m.Count(MetricCISteerInsideFeedbackWindow); n != 1 {
		t.Fatalf("metric = %d", n)
	}
	// Feedback window is untouched by the CI steer.
	at, ok := c.NextEligible(hfKey())
	if !ok || !at.Equal(clk.Now().Add(210*time.Second)) {
		t.Fatalf("feedback window moved: %v %v", at, ok)
	}
}

func TestCoalesce_should_CollapseCIFanOutToOneSteer_When_AllEventsShareOneHeadSHA(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	if got := c.Decide(ciKey(), "A", 0); got != CoalesceSteer {
		t.Fatalf("first = %s", got)
	}
	c.RecordSteer(ciKey(), "A")
	for i := 0; i < 3; i++ {
		if got := c.Decide(ciKey(), "A", 0); got != CoalesceCoveredSameSHA {
			t.Fatalf("inside window %d = %s", i, got)
		}
	}
	clk.Advance(10 * time.Minute)
	if got := c.Decide(ciKey(), "A", 0); got != CoalesceCoveredSameSHA {
		t.Fatalf("outside window = %s", got)
	}
}

func TestCoalesce_should_SteerAgain_When_SecondCIFailureHasNewHeadSHAInsideCIWindow(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	c.RecordSteer(ciKey(), "A")
	clk.Advance(30 * time.Second)
	if got := c.Decide(ciKey(), "B", 0); got != CoalesceSteer {
		t.Fatalf("got %s", got)
	}
}

func TestCoalesce_should_FallBackToClassWindowAndFold_When_CIEventHasNoHeadSHA(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	if got := c.Decide(ciKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("first = %s", got)
	}
	c.RecordSteer(ciKey(), "")
	clk.Advance(60 * time.Second)
	if got := c.Decide(ciKey(), "", 0); got != CoalesceHold {
		t.Fatalf("second = %s", got)
	}
	clk.Advance(240 * time.Second)
	if got := c.Decide(ciKey(), "", 1); got != CoalesceSteer {
		t.Fatalf("at close = %s", got)
	}
}

func TestCoalesce_should_RestartWindowAfterClosingSteer_When_FoldedSteerIsDelivered(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	clk.Advance(300 * time.Second)
	if got := c.Decide(hfKey(), "", 1); got != CoalesceSteer {
		t.Fatalf("close = %s", got)
	}
	c.RecordSteer(hfKey(), "")
	clk.Advance(10 * time.Second)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceHold {
		t.Fatalf("after close = %s", got)
	}
	// An interval above 30 minutes is clamped.
	cl, clk2, _ := newCW(2 * time.Hour)
	cl.RecordSteer(hfKey(), "")
	clk2.Advance(31 * time.Minute)
	if got := cl.Decide(hfKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("clamp = %s", got)
	}
}

func TestCoalesce_should_ReportNextEligibleTime_When_WindowOpenOrClosed(t *testing.T) {
	c, clk, _ := newCW(300 * time.Second)
	if _, ok := c.NextEligible(hfKey()); ok {
		t.Fatal("no window yet")
	}
	t0 := clk.Now()
	c.RecordSteer(hfKey(), "")
	clk.Advance(100 * time.Second)
	at, ok := c.NextEligible(hfKey())
	if !ok || !at.Equal(t0.Add(300*time.Second)) {
		t.Fatalf("open: %v %v", at, ok)
	}
	// Read-only: repeated reads and a Decide change nothing.
	c.Decide(hfKey(), "", 0)
	if at2, _ := c.NextEligible(hfKey()); !at2.Equal(at) {
		t.Fatal("NextEligible mutated state")
	}
	clk.Advance(200 * time.Second)
	if _, ok := c.NextEligible(hfKey()); ok {
		t.Fatal("closed window must report now")
	}
}

func TestCoalesce_should_EvictLastSteerState_When_SessionEndsPRTerminalOrSubscriptionRemoved(t *testing.T) {
	c, _, _ := newCW(300 * time.Second)
	c.RecordSteer(hfKey(), "")
	c.RecordSteer(ciKey(), "A")
	c.Evict(cwSession)
	if got := c.Decide(hfKey(), "", 0); got != CoalesceSteer {
		t.Fatalf("feedback state kept: %s", got)
	}
	if got := c.Decide(ciKey(), "A", 0); got != CoalesceSteer {
		t.Fatalf("ci sha kept: %s", got)
	}
	for i := 0; i < 10000; i++ {
		k := CoalesceKey{SessionUUID: "e" + string(rune('a'+i%26)) + time.Duration(i).String(), Repo: cwRepo, PR: i, Class: ClassHumanFeedback}
		c.RecordSteer(k, "")
		c.Evict(k.SessionUUID)
	}
	if n := c.size(); n != 0 {
		t.Fatalf("size = %d after evicting everything", n)
	}
}
