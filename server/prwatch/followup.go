package prwatch

import (
	"sync"
	"time"
)

// followupTracker implements FollowupObserver (D17). It measures that the
// agent took a turn after a steer (the session left idle), not that the turn
// was useful.
type followupTracker struct {
	sessions Sessions
	clock    Clock
	log      Logger
	window   time.Duration

	mu   sync.Mutex
	open []*followupWindow
}

type followupWindow struct {
	session  string
	repo     string
	pr       int
	ends     time.Time
	observed bool
}

var _ FollowupObserver = (*followupTracker)(nil)

// NewFollowupObserver builds the observer; window is Settings.FollowupObserve.
func NewFollowupObserver(sessions Sessions, clock Clock, log Logger, window time.Duration) FollowupObserver {
	return &followupTracker{sessions: sessions, clock: clock, log: log, window: window}
}

// Track opens an independent observation window for one delivered steer.
func (f *followupTracker) Track(sessionUUID, repo string, pr int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open = append(f.open, &followupWindow{
		session: sessionUUID, repo: repo, pr: pr, ends: f.clock.Now().Add(f.window),
	})
}

// Tick checks each open window and logs the ones that have ended.
func (f *followupTracker) Tick() {
	now := f.clock.Now()
	f.mu.Lock()
	open := append([]*followupWindow(nil), f.open...)
	f.mu.Unlock()

	var done []*followupWindow
	for _, w := range open {
		if !w.observed && f.sessions.IsBusy(w.session) {
			w.observed = true
		}
		if !now.Before(w.ends) {
			done = append(done, w)
		}
	}
	if len(done) == 0 {
		return
	}
	f.mu.Lock()
	kept := f.open[:0]
	for _, w := range f.open {
		if !containsWindow(done, w) {
			kept = append(kept, w)
		}
	}
	f.open = kept
	f.mu.Unlock()
	for _, w := range done {
		f.log.Info("prwatch.steer_followup", "session", w.session, "repo", w.repo, "pr", w.pr,
			"agent_turn_observed", w.observed)
	}
}

func containsWindow(ws []*followupWindow, w *followupWindow) bool {
	for _, x := range ws {
		if x == w {
			return true
		}
	}
	return false
}
