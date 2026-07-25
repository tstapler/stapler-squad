package session

import (
	"sync"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// fakeTmuxSocketQuerier is an in-memory TmuxSocketQuerier for tests. It lets tests
// declare independent live-session sets and down/up states per socket, so
// multi-socket behavior (e.g. reconcileSessions, CheckAllSessions) is exercisable
// without a real tmux server.
type fakeTmuxSocketQuerier struct {
	mu       sync.Mutex
	sessions map[string]map[string]bool // socket -> set of live session names
	down     map[string]bool            // socket -> is server down
	calls    []string                   // sockets passed to ListSessions/IsServerDown, in call order
}

func newFakeTmuxSocketQuerier() *fakeTmuxSocketQuerier {
	return &fakeTmuxSocketQuerier{
		sessions: make(map[string]map[string]bool),
		down:     make(map[string]bool),
	}
}

// setLiveSessions declares that exactly these session names are alive on serverSocket.
func (f *fakeTmuxSocketQuerier) setLiveSessions(serverSocket string, names ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	f.sessions[serverSocket] = set
}

func (f *fakeTmuxSocketQuerier) setDown(serverSocket string, down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down[serverSocket] = down
}

// socketsQueried returns the distinct sockets passed to either method, in first-seen
// order. Tests use this to assert each socket was queried independently rather than
// one socket's result being reused for another.
func (f *fakeTmuxSocketQuerier) socketsQueried() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := make(map[string]bool, len(f.calls))
	out := make([]string, 0, len(f.calls))
	for _, s := range f.calls {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (f *fakeTmuxSocketQuerier) ListSessions(serverSocket string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serverSocket)
	if f.down[serverSocket] {
		return nil, tmux.ErrServerDown
	}
	set := f.sessions[serverSocket]
	result := make(map[string]bool, len(set))
	for k, v := range set {
		result[k] = v
	}
	return result, nil
}

func (f *fakeTmuxSocketQuerier) IsServerDown(serverSocket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serverSocket)
	return f.down[serverSocket]
}

// makeSocketTestInstance builds a minimal managed Instance whose GetTmuxSessionName()
// returns tmuxSessionName (via a mockTmuxManager-backed process manager) without
// touching a real tmux server, for reconcileSessions/health-check tests.
func makeSocketTestInstance(title, tmuxSessionName, socket string, status Status) *Instance {
	inst := &Instance{
		Title:            title,
		Status:           status,
		IsManaged:        true,
		TmuxServerSocket: socket,
	}
	inst.processManager = NewTmuxBackend(&mockTmuxManager{tmuxSessionName: tmuxSessionName})
	return inst
}
