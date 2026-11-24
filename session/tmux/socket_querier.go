package tmux

// SocketQuerier abstracts read-only tmux server-socket queries so that
// callers needing to know "which sessions are alive on this socket" or "is
// this socket's server down" can be exercised in tests without a real tmux
// server. Production code uses NewRealSocketQuerier; tests substitute a fake.
//
// Both ReviewQueuePoller.reconcileSessions and SessionHealthChecker.CheckAllSessions
// need this: instances can be spread across multiple tmux server sockets (the
// default socket for ordinary sessions, isolated sockets for some worktree/test
// scenarios), so every query must be scoped to a specific socket rather than
// assumed to be shared across all instances.
type SocketQuerier interface {
	// ListSessions returns the set of live tmux session names on serverSocket.
	ListSessions(serverSocket string) (map[string]bool, error)
	// IsServerDown reports whether the tmux server on serverSocket is unreachable.
	IsServerDown(serverSocket string) bool
}

// NewRealSocketQuerier returns the production SocketQuerier, which shells out
// to the tmux binary through ListAllSessions and IsServerDown.
func NewRealSocketQuerier() SocketQuerier {
	return realSocketQuerier{}
}

type realSocketQuerier struct{}

func (realSocketQuerier) ListSessions(serverSocket string) (map[string]bool, error) {
	return ListAllSessions(serverSocket)
}

func (realSocketQuerier) IsServerDown(serverSocket string) bool {
	return IsServerDown(serverSocket)
}
