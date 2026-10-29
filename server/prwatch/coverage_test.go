package prwatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

const covConsumer = "squad:devbox"

func newCovFixture(recheck time.Duration) (*Coverage, *fakeInbox, *fakeNotifier, *fakeClock, *fakeLog) {
	in := &fakeInbox{errs: map[string]error{}}
	n := &fakeNotifier{}
	clk := newFakeClock()
	lg := &fakeLog{}
	c := NewCoverage(in, n, clk, lg, Settings{ConsumerID: covConsumer, CoverageRecheck: recheck, StatusURL: "http://localhost:8543/actuator/inbox-pr-watch"})
	return c, in, n, clk, lg
}

func countCalls(in *fakeInbox, prefix string) int {
	n := 0
	for _, c := range in.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func TestCoverage_should_FallBackToSubscriptionsAndLogUnknownCoverage_When_WatchReturns404(t *testing.T) {
	c, in, n, _, lg := newCovFixture(0)
	in.errs["watch"] = inbox.ErrWatchUnsupported

	for pr := 1; pr <= 2; pr++ {
		sub, st, err := c.Watch(context.Background(), "o/r", pr)
		if err != nil || st != CoverageStateUnknown || sub.PullRequest != pr {
			t.Fatalf("pr %d: sub=%+v st=%q err=%v", pr, sub, st, err)
		}
	}
	if got := countCalls(in, "subscribe:"); got != 2 {
		t.Fatalf("subscribe calls = %d, want 2", got)
	}
	if len(n.Sent()) != 0 {
		t.Fatalf("unexpected notifications: %+v", n.Sent())
	}
	infos := 0
	for _, l := range lg.Lines() {
		if l.Msg == "prwatch.coverage_unknown" {
			infos++
		}
	}
	if infos != 1 {
		t.Fatalf("unknown-coverage log lines = %d, want 1", infos)
	}
	in.errs["watch"] = errors.New("x")
	c.Recheck(context.Background())
	if countCalls(in, "watch:") != 2 {
		t.Fatalf("unknown repos must not be re-checked: %v", in.Calls())
	}
}

func TestCoverage_should_RaiseCoverageGapOncePerRepoWithReason_When_WatchUnavailable(t *testing.T) {
	c, in, n, _, _ := newCovFixture(0)
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "no admin access to the repository"}

	for pr := 42; pr <= 43; pr++ {
		if _, st, err := c.Watch(context.Background(), "other/repo", pr); err != nil || st != CoverageStateUnavailable {
			t.Fatalf("st=%q err=%v", st, err)
		}
	}
	sent := n.Sent()
	if len(sent) != 1 {
		t.Fatalf("notifications = %d, want 1", len(sent))
	}
	g := sent[0]
	if g.Kind != NotifyCoverageGap || g.DedupeKey != "prwatch-coverage:other/repo" ||
		g.Title != "PR watch: no webhook coverage for other/repo" {
		t.Fatalf("bad notification: %+v", g)
	}
	for _, want := range []string{"PR #42", "gh-signal said: no admin access to the repository.", "ignore_repos", "repository admin", "Status: http://localhost:8543"} {
		if !strings.Contains(g.Message, want) {
			t.Errorf("message missing %q: %s", want, g.Message)
		}
	}
	// Different reason, different next step.
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "ingress is down"}
	c.Watch(context.Background(), "a/b", 7)
	if m := n.Sent()[1].Message; !strings.Contains(m, "Next step: run gh-signal inbox watch a/b 7 --consumer squad:devbox.") {
		t.Errorf("generic next step missing: %s", m)
	}
}

func TestCoverage_should_RecheckUncoveredReposEveryTenMinutesAndClearGap_When_HookIsFixed(t *testing.T) {
	c, in, n, clk, _ := newCovFixture(10 * time.Minute)
	ctx := context.Background()
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "r"}
	c.Watch(ctx, "bad/repo", 5)
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageExisting}
	c.Watch(ctx, "good/repo", 6)
	in.errs["watch"] = inbox.ErrWatchUnsupported
	c.Watch(ctx, "old/repo", 8)
	in.errs["watch"] = nil
	base := countCalls(in, "watch:")

	clk.Advance(9*time.Minute + 59*time.Second)
	c.Recheck(ctx)
	if countCalls(in, "watch:") != base {
		t.Fatal("re-checked before 10 minutes")
	}

	// Still unavailable at 10 minutes: re-called for that repo only, no repeat notice.
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "r"}
	clk.Advance(time.Second)
	c.Recheck(ctx)
	if countCalls(in, "watch:") != base+1 || countCalls(in, "watch:"+covConsumer+":bad/repo#5") != 2 {
		t.Fatalf("calls: %v", in.Calls())
	}
	if len(n.Sent()) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.Sent()))
	}

	// Hook fixed: next due check clears the gap.
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageRegistered}
	clk.Advance(10 * time.Minute)
	c.Recheck(ctx)
	if st, _ := c.State("bad/repo"); st != CoverageStateCovered {
		t.Fatalf("state = %q", st)
	}
	clk.Advance(time.Hour)
	before := countCalls(in, "watch:")
	c.Recheck(ctx)
	if countCalls(in, "watch:") != before {
		t.Fatal("covered repo re-checked")
	}

	// A later loss raises the notification again (key forgotten).
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "lost"}
	c.Watch(ctx, "bad/repo", 5)
	if len(n.Sent()) != 2 {
		t.Fatalf("notifications = %d, want 2", len(n.Sent()))
	}
}

func TestCoverage_should_HonorConfiguredRecheckInterval_When_SettingDiffers(t *testing.T) {
	c, in, _, clk, _ := newCovFixture(time.Minute)
	in.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable}
	c.Watch(context.Background(), "o/r", 1)
	clk.Advance(time.Minute)
	c.Recheck(context.Background())
	if countCalls(in, "watch:") != 2 {
		t.Fatalf("calls: %v", in.Calls())
	}
}
