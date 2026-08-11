package tmux

import (
	"context"
	"sync"
)

// FakeTmuxRegistry is a test double for TmuxStatePort. It holds an in-memory
// session map and lets tests control session state, health, and pane-exit events
// without spawning any real tmux processes.
type FakeTmuxRegistry struct {
	mu       sync.Mutex
	sessions map[string]bool
	healthy  bool

	subsMu      sync.Mutex
	subscribers map[string][]chan struct{}
}

// Compile-time check: FakeTmuxRegistry must implement TmuxStatePort.
var _ TmuxStatePort = (*FakeTmuxRegistry)(nil)

// NewFakeTmuxRegistry creates an empty, healthy registry for use in tests.
func NewFakeTmuxRegistry() *FakeTmuxRegistry {
	return &FakeTmuxRegistry{
		sessions:    make(map[string]bool),
		healthy:     true,
		subscribers: make(map[string][]chan struct{}),
	}
}

// SetSessions replaces the in-memory session map with the provided names.
func (f *FakeTmuxRegistry) SetSessions(names []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = make(map[string]bool, len(names))
	for _, name := range names {
		f.sessions[name] = true
	}
}

// SetHealthy controls the value returned by IsHealthy.
func (f *FakeTmuxRegistry) SetHealthy(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.healthy = v
}

// SessionExists implements SessionExistenceChecker.
func (f *FakeTmuxRegistry) SessionExists(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[name]
}

// IsHealthy implements SessionExistenceChecker and SessionLister.
func (f *FakeTmuxRegistry) IsHealthy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.healthy
}

// ListSessions implements SessionLister. Returns a copy of the sessions map.
func (f *FakeTmuxRegistry) ListSessions() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]bool, len(f.sessions))
	for k, v := range f.sessions {
		out[k] = v
	}
	return out
}

// SubscribePaneExit implements PaneExitSubscriber. Returns a channel that is
// closed either when FirePaneExit is called for sessionName or when ctx is
// cancelled.
func (f *FakeTmuxRegistry) SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{} {
	ch := make(chan struct{}, 1)

	f.subsMu.Lock()
	f.subscribers[sessionName] = append(f.subscribers[sessionName], ch)
	f.subsMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			// Remove this subscription and close the channel.
			f.subsMu.Lock()
			existing := f.subscribers[sessionName]
			filtered := existing[:0]
			for _, c := range existing {
				if c != ch {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) == 0 {
				delete(f.subscribers, sessionName)
			} else {
				f.subscribers[sessionName] = filtered
			}
			f.subsMu.Unlock()
			close(ch)
		case <-ch:
			// Already closed by FirePaneExit; nothing to do.
		}
	}()

	return ch
}

// FirePaneExit simulates a pane-exit event: closes all subscriber channels
// registered for sessionName. This is the test helper for verifying subscribers.
func (f *FakeTmuxRegistry) FirePaneExit(sessionName string) {
	f.subsMu.Lock()
	chs := f.subscribers[sessionName]
	delete(f.subscribers, sessionName)
	f.subsMu.Unlock()

	// Close outside the lock to prevent deadlock.
	for _, ch := range chs {
		close(ch)
	}
}
