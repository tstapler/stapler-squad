package streamhub

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/tstapler/stapler-squad/log"
)

// ErrOwnershipResolvedToOtherPath is returned by ResolveExpecting when the
// session's sticky StreamPath resolution does not match the caller's own
// intended path — i.e. the other side of the race won. Callers must treat
// this as an explicit signal to join the winning path (attach as a
// subscriber to its existing hub, or proceed as a legacy per-connection
// stream) rather than silently reinterpreting their own attempt as having
// succeeded (Task 3.1.2b).
var ErrOwnershipResolvedToOtherPath = errors.New("streamhub: session ownership already resolved to a different StreamPath")

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

// ResolveExpecting resolves the lock exactly like Resolve, but additionally
// asserts that the resolved StreamPath matches want — the caller's own
// intended role (hub creation expects PathHubOwned; legacy StartControlMode
// expects PathLegacyPerConnection). If another caller's resolution already
// won and it disagrees with want, ResolveExpecting returns the actual
// resolved path plus ErrOwnershipResolvedToOtherPath instead of proceeding,
// so HubRegistry.GetOrCreate and the legacy per-connection entry point (both
// in package server/services, which cannot itself hold this lock's internal
// state) can refuse to create a competing owner and instead join whichever
// path actually won (Story 3.1.2 / Task 3.1.2b).
func (l *StreamOwnershipLock) ResolveExpecting(flagValue bool, want StreamPath) (StreamPath, error) {
	got := l.Resolve(flagValue)
	if got != want {
		return got, fmt.Errorf("%w: resolved %v, wanted %v", ErrOwnershipResolvedToOtherPath, got, want)
	}
	return got, nil
}

// overlapInvariantHook lets a test observe an OverlapInvariant violation
// synchronously (e.g. to t.Fatal on first occurrence, per plan.md's
// OverlapInvariant Domain Glossary entry) instead of grepping captured log
// output. nil in production. Set via SetOverlapInvariantHookForTest.
var overlapInvariantHook atomic.Pointer[func(sessionName string, ownerCount int)]

// SetOverlapInvariantHookForTest installs hook to run, in addition to
// OverlapInvariant's normal slog.Error+metric behavior, whenever a
// violation is detected. Pass nil to clear it. Tests should always clear it
// via t.Cleanup so a hook installed by one test can't leak into another
// running in the same package binary.
func SetOverlapInvariantHookForTest(hook func(sessionName string, ownerCount int)) {
	if hook == nil {
		overlapInvariantHook.Store(nil)
		return
	}
	overlapInvariantHook.Store(&hook)
}

// OverlapInvariant is the production-reachable, load-bearing regression
// check named in plan.md's Domain Glossary: no two owners (legacy
// connection or hub) should ever hold resize/capture authority for one tmux
// session concurrently. StreamOwnershipLock.Resolve's mutex-guarded
// resolve-once-and-cache behavior already makes this structurally
// impossible under correct usage — this function is the defense-in-depth
// check that would surface a future regression in that guarantee
// immediately (e.g. HubRegistry.GetOrCreate calls it on every hub
// creation/lookup, Task 3.2's real production call site) rather than
// relying solely on code review to keep Resolve/GetOrCreate correct
// forever. It always emits slog.Error with full context and increments
// streamhub_overlap_invariant_violations_total — it never panics, since a
// panic in this single-operator daily-driver process would be worse than
// the bug it would catch. This is the one real implementation: earlier
// phases' overlapInvariantViolated/assertOverlapInvariant test-local
// helpers (Epic 1.4) have been retired in favor of calling this directly,
// via SetOverlapInvariantHookForTest for the t.Fatal-on-first-occurrence
// behavior every -race test in this plan requires.
func OverlapInvariant(sessionName string, ownerCount int) {
	if ownerCount <= 1 {
		return
	}
	recordOverlapInvariantViolation()
	log.Error("streamhub: OverlapInvariant violated — more than one owner resolved for a tmux session",
		"tmux_session", sessionName, "owner_count", ownerCount)
	if hook := overlapInvariantHook.Load(); hook != nil {
		(*hook)(sessionName, ownerCount)
	}
}
