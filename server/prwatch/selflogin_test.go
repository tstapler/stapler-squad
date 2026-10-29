package prwatch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newResolver(login *fakeSelfLogin) (*SelfLoginResolver, *fakeClock, *fakeNotifier) {
	clk := newFakeClock()
	n := &fakeNotifier{}
	return NewSelfLoginResolver(login, clk, n, &fakeLog{}), clk, n
}

func TestSelfLogin_should_HoldCommentAndReviewButSteerCI_When_LoginUnresolved(t *testing.T) {
	for _, fl := range []*fakeSelfLogin{{login: ""}, {err: errors.New("rate limited")}} {
		r, _, _ := newResolver(fl)
		r.Tick(context.Background())
		f := NewActorFilter(settings(ModeAll, nil, nil, true), r)
		for _, kind := range []Kind{KindComment, KindReviewComment, KindReview} {
			res := f.Evaluate(kind, "stranger")
			if res.Verdict != VerdictHold || res.Hold != HoldLoginUnresolved {
				t.Fatalf("%s: %+v", kind, res)
			}
		}
		if res := f.Evaluate(KindCI, "stranger"); res.Verdict != VerdictSteer {
			t.Fatalf("ci: %+v", res)
		}
	}
}

func TestSelfLogin_should_RetryWithBackoff_When_LookupErrorsOrReturnsEmpty(t *testing.T) {
	fl := &fakeSelfLogin{err: errors.New("boom")}
	r, clk, _ := newResolver(fl)
	ctx := context.Background()
	r.Tick(ctx) // attempt 1 at t0
	r.Tick(ctx)
	if fl.calls != 1 {
		t.Fatalf("calls %d, want 1 (not due)", fl.calls)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 160 * time.Second, 300 * time.Second, 300 * time.Second}
	calls := 1
	for _, d := range want {
		clk.Advance(d - time.Second)
		r.Tick(ctx)
		if fl.calls != calls {
			t.Fatalf("retried %v early", d)
		}
		clk.Advance(time.Second)
		r.Tick(ctx)
		calls++
		if fl.calls != calls {
			t.Fatalf("no retry after %v: calls %d", d, fl.calls)
		}
	}
	fl.mu.Lock()
	fl.err, fl.login = nil, "Me"
	fl.mu.Unlock()
	clk.Advance(5 * time.Minute)
	r.Tick(ctx)
	if got, ok := r.Cached(); !ok || got != "Me" {
		t.Fatalf("cached %q %v", got, ok)
	}
	before := fl.calls
	clk.Advance(time.Hour)
	r.Tick(ctx)
	if fl.calls != before {
		t.Fatal("resolved login must be cached for the process lifetime")
	}
}

func TestSelfLogin_should_NotifyOnceAfter10Minutes_When_StillUnresolved(t *testing.T) {
	fl := &fakeSelfLogin{}
	r, clk, n := newResolver(fl)
	ctx := context.Background()
	r.Tick(ctx)
	clk.Advance(9*time.Minute + 59*time.Second)
	r.Tick(ctx)
	if len(n.Sent()) != 0 {
		t.Fatalf("notified before 10 min: %v", n.Sent())
	}
	clk.Advance(time.Second)
	r.Tick(ctx)
	r.Tick(ctx)
	clk.Advance(30 * time.Minute)
	r.Tick(ctx)
	sent := n.Sent()
	if len(sent) != 1 || sent[0].Kind != NotifyLoginUnresolved || sent[0].Title != TitleLoginUnresolved {
		t.Fatalf("sent %+v", sent)
	}
	fl.mu.Lock()
	fl.login = "me"
	fl.mu.Unlock()
	clk.Advance(5 * time.Minute)
	r.Tick(ctx)
	sent = n.Sent()
	if len(sent) != 2 || sent[1].Title != TitleRecovered || sent[1].DedupeKey != sent[0].DedupeKey {
		t.Fatalf("expected recovery overwrite, got %+v", sent)
	}
}

func TestSelfLogin_should_ResumeSteeringHeldEvents_When_LoginResolves(t *testing.T) {
	fl := &fakeSelfLogin{}
	r, clk, _ := newResolver(fl)
	f := NewActorFilter(settings(ModeAll, nil, nil, true), r)
	ctx := context.Background()
	r.Tick(ctx)
	if res := f.Evaluate(KindComment, "me"); res.Verdict != VerdictHold {
		t.Fatalf("%+v", res)
	}
	fl.mu.Lock()
	fl.login = "me"
	fl.mu.Unlock()
	clk.Advance(5 * time.Second)
	r.Tick(ctx)
	if res := f.Evaluate(KindComment, "me"); res.Drop != DropSelfActor {
		t.Fatalf("self after resolve: %+v", res)
	}
	if res := f.Evaluate(KindComment, "other"); res.Verdict != VerdictSteer {
		t.Fatalf("other after resolve: %+v", res)
	}
}
