// Package qos tracks which terminal sessions a human is typing into right now, so background
// work (pollers, repaints of idle panes) can yield to them. Typing latency is the product's
// top priority; nothing here may ever block or fail a keystroke.
package qos

import (
	"sync"
	"time"
)

// InteractiveWindow is how long after its last input a session still counts as interactive.
const InteractiveWindow = 5 * time.Second

// BackgroundMaxDeferral bounds how long a poller yields to typing before it runs once anyway.
const BackgroundMaxDeferral = 30 * time.Second

// Tracker is a process-wide record of recently typed-into sessions. Use NewTracker or Default.
type Tracker struct {
	now    func() time.Time
	window time.Duration

	mu       sync.Mutex
	lastNote map[string]time.Time
	// lastEver outlives the interactive window: it ranks sessions by recency of use (for example
	// which terminal to attach first after a reload). One entry per session ever typed into.
	lastEver map[string]time.Time
	// firstDeferred is when a named background job was first deferred in the current streak;
	// it bounds starvation (see Defer).
	firstDeferred map[string]time.Time
	watchers      map[int]func()
	nextWatcher   int
}

// Activity is a session's priority class, mirroring tymuxd's PaneActivity hint.
type Activity int

const (
	// ActivityNormal is the default: no one is typing anywhere.
	ActivityNormal Activity = iota
	// ActivityInteractive means a human typed into this session within InteractiveWindow.
	ActivityInteractive
	// ActivityBackground means another session is interactive and this one is not.
	ActivityBackground
)

func (a Activity) String() string {
	switch a {
	case ActivityInteractive:
		return "interactive"
	case ActivityBackground:
		return "background"
	case ActivityNormal:
		return "normal"
	default:
		return "normal"
	}
}

// Default is the tracker the hub and the background pollers share.
var Default = NewTracker(time.Now, InteractiveWindow) //nolint:gochecknoglobals // one process-wide tracker shared by the hub and the pollers

// NewTracker builds a tracker with an injectable clock for deterministic tests.
func NewTracker(now func() time.Time, window time.Duration) *Tracker {
	return &Tracker{
		now: now, window: window,
		lastNote:      make(map[string]time.Time),
		lastEver:      make(map[string]time.Time),
		firstDeferred: make(map[string]time.Time),
		watchers:      make(map[int]func()),
	}
}

// Watch registers fn to run (synchronously, so it must be cheap and non-blocking) whenever a
// session starts or stops being interactive. The returned func unregisters it.
func (t *Tracker) Watch(fn func()) (cancel func()) {
	t.mu.Lock()
	id := t.nextWatcher
	t.nextWatcher++
	t.watchers[id] = fn
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		delete(t.watchers, id)
		t.mu.Unlock()
	}
}

func (t *Tracker) notify() {
	t.mu.Lock()
	fns := make([]func(), 0, len(t.watchers))
	for _, fn := range t.watchers {
		fns = append(fns, fn)
	}
	t.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// ActivityOf classifies session key: interactive if it was typed into, background if only
// another session was, normal otherwise.
func (t *Tracker) ActivityOf(key string) Activity {
	switch {
	case t.Interactive(key):
		return ActivityInteractive
	case t.OtherInteractive(key):
		return ActivityBackground
	default:
		return ActivityNormal
	}
}

// Expire retires key if its window has elapsed, notifying watchers, and otherwise reports how
// long remains. It lets an owner with its own timer end an interactive spell promptly.
func (t *Tracker) Expire(key string) (remaining time.Duration) {
	now := t.now()
	t.mu.Lock()
	at, ok := t.lastNote[key]
	if !ok {
		t.mu.Unlock()
		return 0
	}
	if left := t.window - now.Sub(at); left > 0 {
		t.mu.Unlock()
		return left
	}
	delete(t.lastNote, key)
	t.mu.Unlock()
	t.notify()
	return 0
}

// Note records input for session key. It is O(1) and safe on the keystroke path.
func (t *Tracker) Note(key string) {
	now := t.now()
	t.mu.Lock()
	at, had := t.lastNote[key]
	wasLive := had && now.Sub(at) <= t.window
	t.lastNote[key] = now
	t.lastEver[key] = now
	t.mu.Unlock()
	if !wasLive {
		t.notify()
	}
}

// LastInteraction is when session key last received input, or the zero time if never.
func (t *Tracker) LastInteraction(key string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastEver[key]
}

// Forget drops a session's state when its hub goes away.
func (t *Tracker) Forget(key string) {
	t.mu.Lock()
	_, had := t.lastNote[key]
	delete(t.lastNote, key)
	t.mu.Unlock()
	if had {
		t.notify()
	}
}

// Interactive reports whether session key received input within the window.
func (t *Tracker) Interactive(key string) bool {
	return t.anyLive(func(k string) bool { return k == key })
}

// AnyInteractive reports whether any session received input within the window.
func (t *Tracker) AnyInteractive() bool { return t.anyLive(func(string) bool { return true }) }

// OtherInteractive reports whether a session other than key is interactive.
func (t *Tracker) OtherInteractive(key string) bool {
	return t.anyLive(func(k string) bool { return k != key })
}

// anyLive expires stale entries, then reports whether any live session satisfies match.
func (t *Tracker) anyLive(match func(key string) bool) bool {
	now := t.now()
	t.mu.Lock()
	found, pruned := false, false
	for k, at := range t.lastNote {
		if now.Sub(at) > t.window {
			delete(t.lastNote, k)
			pruned = true
			continue
		}
		if match(k) {
			found = true
		}
	}
	t.mu.Unlock()
	// Pruning ends an interactive spell like Expire does; without telling the watchers, a hub
	// whose own expiry timer then finds nothing to retire would stay boosted or demoted.
	if pruned {
		t.notify()
	}
	return found
}

// Defer reports whether background job name should skip this run because a session is
// interactive. It is bounded: once a job has been deferred for maxDeferral it runs once and
// the streak restarts, so a user who types for an hour cannot starve the pollers forever.
func (t *Tracker) Defer(name string, maxDeferral time.Duration) bool {
	interactive := t.AnyInteractive()
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if !interactive {
		delete(t.firstDeferred, name)
		return false
	}
	first, deferring := t.firstDeferred[name]
	if !deferring {
		t.firstDeferred[name] = now
		return true
	}
	if now.Sub(first) >= maxDeferral {
		delete(t.firstDeferred, name)
		return false
	}
	return true
}
