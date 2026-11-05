package prwatch

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testLeaderRetry = 30 * time.Second

func newTestLeadership(g *fakeLeader) (*Leadership, *fakeClock) {
	clock := newFakeClock()
	return NewLeadership(g, clock, &fakeLog{}, testLeaderRetry), clock
}

func TestLeader_should_RetryTryAcquireOnInterval_And_TakeOver_When_HolderReleases(t *testing.T) {
	g := &fakeLeader{}
	l, clock := newTestLeadership(g)

	if l.Held() || l.State() != InboxStateLeaderNotHeld {
		t.Fatal("refused guard must not be held and must report leader_not_held")
	}
	for i := 0; i < 100; i++ { // many ticks inside the interval: no extra attempts
		clock.Advance(100 * time.Millisecond)
		if l.Held() {
			t.Fatal("held while refused")
		}
	}
	if g.attempts != 1 {
		t.Fatalf("attempts inside interval = %d, want 1", g.attempts)
	}
	clock.Advance(testLeaderRetry)
	l.Held()
	if g.attempts != 2 {
		t.Fatalf("attempts after interval = %d, want 2", g.attempts)
	}

	g.mu.Lock()
	g.grant = true
	g.mu.Unlock()
	clock.Advance(testLeaderRetry)
	if !l.Held() || l.State() != InboxStateOK {
		t.Fatal("must take over on the first retry after the holder releases")
	}
}

func TestLeader_should_HoldLockForProcessLifetime_And_ReleaseOnlyAtShutdown_When_ManyTicksPass(t *testing.T) {
	g := &fakeLeader{grant: true}
	l, clock := newTestLeadership(g)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Watch(ctx); close(done) }()

	for i := 0; i < 1000; i++ {
		clock.Advance(5 * time.Second)
		if !l.Held() {
			t.Fatalf("tick %d: not held", i)
		}
	}
	if g.attempts != 1 || g.released != 0 {
		t.Fatalf("attempts=%d released=%d, want 1 and 0", g.attempts, g.released)
	}
	cancel()
	<-done
	l.Release() // a second release must not release twice
	if g.released != 1 {
		t.Fatalf("released = %d, want exactly 1", g.released)
	}
	clock.Advance(time.Hour)
	if l.Held() || g.attempts != 1 {
		t.Fatal("must not re-acquire after shutdown")
	}
}

func TestLeader_should_RetryOnInterval_And_ReportNotHeld_When_TryAcquireErrors(t *testing.T) {
	g := &fakeLeader{err: errors.New("lock dir unreadable")}
	l, clock := newTestLeadership(g)
	if l.Held() {
		t.Fatal("error must mean not held")
	}
	l.Held()
	if g.attempts != 1 {
		t.Fatalf("attempts = %d, want 1 inside interval", g.attempts)
	}
	clock.Advance(testLeaderRetry)
	l.Held()
	if g.attempts != 2 || l.State() != InboxStateLeaderNotHeld {
		t.Fatalf("attempts=%d state=%s", g.attempts, l.State())
	}
}
