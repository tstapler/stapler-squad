package session

import "github.com/tstapler/stapler-squad/session/tmux"

// TmuxSocketQuerier abstracts read-only tmux server-socket queries so that
// callers needing to know "which sessions are alive on this socket" or "is
// this socket's server down" can be exercised in tests without a real tmux
// server. Production code backs this with the real tmux package (via
// realTmuxSocketQuerier); tests substitute a fake.
//
// Both ReviewQueuePoller.reconcileSessions and SessionHealthChecker.CheckAllSessions
// need this: instances can be spread across multiple tmux server sockets (the
// default socket for ordinary sessions, isolated sockets for some worktree/test
// scenarios), so every query must be scoped to a specific socket rather than
// assumed to be shared across all instances.
type TmuxSocketQuerier interface {
	// ListSessions returns the set of live tmux session names on serverSocket.
	ListSessions(serverSocket string) (map[string]bool, error)
	// IsServerDown reports whether the tmux server on serverSocket is unreachable.
	IsServerDown(serverSocket string) bool
}

// NewRealTmuxSocketQuerier returns the production TmuxSocketQuerier backed by
// the real tmux package, for callers outside this package (e.g.
// server/services) that need to query live tmux state without depending on
// the unexported realTmuxSocketQuerier type directly.
func NewRealTmuxSocketQuerier() TmuxSocketQuerier {
	return realTmuxSocketQuerier{}
}

// realTmuxSocketQuerier is the production TmuxSocketQuerier, backed by the real
// tmux package (which shells out to the tmux binary).
type realTmuxSocketQuerier struct{}

func (realTmuxSocketQuerier) ListSessions(serverSocket string) (map[string]bool, error) {
	return tmux.ListAllSessions(serverSocket)
}

func (realTmuxSocketQuerier) IsServerDown(serverSocket string) bool {
	return tmux.IsServerDown(serverSocket)
}

// groupInstancesBySocket partitions instances by their own TmuxServerSocket.
// Callers must query each socket independently rather than assuming all
// instances share one socket -- that assumption previously caused instances on
// a non-default socket to falsely appear dead (or alive) based on whichever
// socket happened to be picked, flapping their status on every reconcile pass.
func groupInstancesBySocket(instances []*Instance) map[string][]*Instance {
	bySocket := make(map[string][]*Instance, 1)
	for _, inst := range instances {
		bySocket[inst.TmuxServerSocket] = append(bySocket[inst.TmuxServerSocket], inst)
	}
	return bySocket
}
