package prwatch

import (
	"context"
	"sync"
	"time"
)

// Leadership holds the process-lifetime leader lock (D9). The poll loop calls
// Held once per tick: a holder keeps the lock until the context passed to
// Watch is cancelled, and a non-holder retries TryAcquire at most once per
// retry interval, so there is no busy loop and no retry timer of its own.
type Leadership struct {
	guard LeaderGuard
	clock Clock
	log   Logger
	retry time.Duration

	mu       sync.Mutex
	release  func()
	nextTry  time.Time
	released bool
}

// NewLeadership builds a Leadership. A retry of zero or less retries on every
// call, so callers pass Settings.LeaderRetry.
func NewLeadership(guard LeaderGuard, clock Clock, log Logger, retry time.Duration) *Leadership {
	return &Leadership{guard: guard, clock: clock, log: log, retry: retry}
}

// Held reports whether this process leads, trying to acquire when it does not
// and the retry interval has elapsed. A TryAcquire error counts as not held
// and is retried on the same interval.
func (l *Leadership) Held() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.release != nil {
		return true
	}
	if l.released {
		return false
	}
	now := l.clock.Now()
	if now.Before(l.nextTry) {
		return false
	}
	l.nextTry = now.Add(l.retry)
	release, ok, err := l.guard.TryAcquire()
	if err != nil {
		l.log.Warn("prwatch leader acquire failed", "error", err.Error())
		return false
	}
	if !ok {
		return false
	}
	if release == nil {
		release = func() {}
	}
	l.release = release
	l.log.Info("prwatch leadership acquired")
	return true
}

// State is the leader's contribution to the inbox state: leader_not_held
// until the lock is taken, ok afterwards.
func (l *Leadership) State() InboxState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.release != nil {
		return InboxStateOK
	}
	return InboxStateLeaderNotHeld
}

// Watch releases the lock once ctx is cancelled and blocks until then. Run it
// under the server's background WaitGroup so shutdown joins the release.
func (l *Leadership) Watch(ctx context.Context) {
	<-ctx.Done()
	l.Release()
}

// Release gives up the lock and bars re-acquisition. It is safe to call more
// than once; only the first call releases.
func (l *Leadership) Release() {
	l.mu.Lock()
	release := l.release
	l.release = nil
	l.released = true
	l.mu.Unlock()
	if release != nil {
		release()
	}
}
