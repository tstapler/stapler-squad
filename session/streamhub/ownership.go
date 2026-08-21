package streamhub

import (
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
)

// StreamOwnershipLock resolves STAPLER_SQUAD_USE_STREAM_HUB (plus any future
// per-session override, Story 3.3.1) into a StreamPath exactly once per
// tmux session, at the first connection's attach time, and sticks that
// resolution for the rest of the session's lifetime — see ADR-003. A
// per-connection re-read (Epic 2.2's useStreamHub placeholder) would let a
// flag flip mid-rollout split one session across two owners; this type
// closes that window by construction rather than by convention, the same
// safety property session/tmux.TmuxSession.controlModeStartMu gives the
// legacy 0->1 refcount transition.
//
// Story 3.1.2 widens this into the shared mutual-exclusion primitive both
// legacy StartControlMode and hub creation acquire; today it only owns the
// resolve-once-and-cache behavior (Story 3.1.1).
type StreamOwnershipLock struct {
	mu       sync.Mutex
	resolved bool
	path     StreamPath
}

// ownershipLocks is the package-level, per-tmux-session-name registry of
// StreamOwnershipLocks. Keyed by tmux session name (not a global value) so
// two different sessions can independently resolve to different
// StreamPaths. session/streamhub never imports package session — callers in
// package session reach this via AcquireOwnershipLock, a one-way dependency
// (see ADR-003 / plan.md's SessionController Pattern Decisions entry).
var ownershipLocks = xsync.NewMap[string, *StreamOwnershipLock]()

// AcquireOwnershipLock returns the StreamOwnershipLock for sessionName,
// creating it if this is the first call for that name. Every caller passing
// the same sessionName gets the same *StreamOwnershipLock instance, which is
// what makes the mutual exclusion in Story 3.1.2 possible.
func AcquireOwnershipLock(sessionName string) *StreamOwnershipLock {
	lock, _ := ownershipLocks.LoadOrCompute(sessionName, func() (*StreamOwnershipLock, bool) {
		return &StreamOwnershipLock{}, false
	})
	return lock
}

// Resolve turns flagValue into a StreamPath the first time it is called on
// this lock, caches that result, and returns the cached StreamPath on every
// subsequent call — regardless of what flagValue later callers pass. This is
// what makes a flag flip mid-rollout safe: the first connection's resolution
// sticks for the session's lifetime, so a second connection arriving after
// the flag changed in the environment still observes the original decision.
func (l *StreamOwnershipLock) Resolve(flagValue bool) StreamPath {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.resolved {
		if flagValue {
			l.path = PathHubOwned
		} else {
			l.path = PathLegacyPerConnection
		}
		l.resolved = true
	}
	return l.path
}
