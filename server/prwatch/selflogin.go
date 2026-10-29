package prwatch

import (
	"context"
	"sync"
	"time"
)

const (
	selfLoginInitialBackoff = 5 * time.Second
	selfLoginMaxBackoff     = 5 * time.Minute
	selfLoginNotifyAfter    = 10 * time.Minute
)

// SelfLoginResolver resolves the operator's login in the background (not on
// the event path), caches a non-empty result for the process lifetime, and
// raises one LoginUnresolved notification after 10 unresolved minutes.
// Failures and empty logins are the same: unresolved, retry with backoff.
type SelfLoginResolver struct {
	src      SelfLogin
	clock    Clock
	notifier Notifier
	log      Logger
	// OnResolved, if set, runs once when the login resolves. The login must
	// not be logged or counted by the callback.
	OnResolved func(login string)

	mu       sync.Mutex
	login    string
	started  time.Time
	nextTry  time.Time
	backoff  time.Duration
	notified bool
}

// NewSelfLoginResolver builds a resolver; the first Tick tries immediately.
func NewSelfLoginResolver(src SelfLogin, clock Clock, notifier Notifier, log Logger) *SelfLoginResolver {
	return &SelfLoginResolver{src: src, clock: clock, notifier: notifier, log: log}
}

// Cached returns the resolved login; ok is false until one resolves.
func (r *SelfLoginResolver) Cached() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.login, r.login != ""
}

// Tick makes at most one lookup attempt if one is due, and raises the
// 10-minute notification when due. Safe to call every poll tick; a no-op
// once resolved.
func (r *SelfLoginResolver) Tick(ctx context.Context) {
	now := r.clock.Now()
	r.mu.Lock()
	if r.login != "" {
		r.mu.Unlock()
		return
	}
	if r.started.IsZero() {
		r.started = now
	}
	due := !now.Before(r.nextTry)
	r.mu.Unlock()

	if due {
		login, err := r.src.Login(ctx)
		r.record(login, err, r.clock.Now())
	}
	r.maybeNotify()
}

func (r *SelfLoginResolver) record(login string, err error, now time.Time) {
	r.mu.Lock()
	if err == nil && login != "" {
		r.login = login
		wasNotified := r.notified
		r.notified = false
		cb := r.OnResolved
		r.mu.Unlock()
		r.log.Info("prwatch.self_login_resolved")
		if wasNotified {
			_ = r.notifier.Notify(r.notification(TitleRecovered, "GitHub login resolved; comment and review steering has resumed."))
		}
		if cb != nil {
			cb(login)
		}
		return
	}
	switch {
	case r.backoff == 0:
		r.backoff = selfLoginInitialBackoff
	case r.backoff < selfLoginMaxBackoff:
		r.backoff = min(r.backoff*2, selfLoginMaxBackoff)
	}
	r.nextTry = now.Add(r.backoff)
	next := r.backoff
	r.mu.Unlock()
	r.log.Warn("prwatch.self_login_unresolved", "error", err != nil, "retry_in", next.String())
}

func (r *SelfLoginResolver) maybeNotify() {
	r.mu.Lock()
	if r.login != "" || r.notified || r.clock.Now().Sub(r.started) < selfLoginNotifyAfter {
		r.mu.Unlock()
		return
	}
	r.notified = true
	r.mu.Unlock()
	_ = r.notifier.Notify(r.notification(TitleLoginUnresolved,
		"Squad could not determine your GitHub login for 10 minutes, so comment and review events are held. CI failures still steer. Run `gh auth status` and fix the login."))
}

func (r *SelfLoginResolver) notification(title, msg string) Notification {
	return Notification{Kind: NotifyLoginUnresolved, DedupeKey: "prwatch:login_unresolved", Title: title, Message: msg}
}

// Run drives Tick until the login resolves or ctx ends. It sleeps in real
// time between attempts, up to the max backoff; tests call Tick directly.
func (r *SelfLoginResolver) Run(ctx context.Context) {
	for {
		r.Tick(ctx)
		if _, ok := r.Cached(); ok {
			return
		}
		r.mu.Lock()
		wait := r.nextTry.Sub(r.clock.Now())
		r.mu.Unlock()
		wait = max(wait, time.Second)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
