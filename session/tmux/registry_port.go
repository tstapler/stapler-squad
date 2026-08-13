package tmux

import "context"

// SessionExistenceChecker answers "is session X alive right now?"
// Used by TmuxSession.DoesSessionExist to avoid exec.Command forks.
type SessionExistenceChecker interface {
	SessionExists(name string) bool
	IsHealthy() bool
}

// SessionLister returns a snapshot of all live session names.
// Used by PTYDiscovery and reconciliation loops.
type SessionLister interface {
	ListSessions() map[string]bool
	IsHealthy() bool
}

// PaneExitSubscriber delivers a channel that is closed when the named pane
// exits. Caller selects on the returned channel alongside ctx.Done().
// Cancelling ctx unregisters the subscription; channel is closed immediately.
type PaneExitSubscriber interface {
	SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}
}

// TmuxStatePort is the full registry interface.
type TmuxStatePort interface {
	SessionExistenceChecker
	SessionLister
	PaneExitSubscriber
}
