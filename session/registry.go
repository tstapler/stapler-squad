package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tstapler/stapler-squad/log"
)

// ReleaseFunc is the refcount-gated teardown closure returned by Acquire and Register.
// It is idempotent (safe to call more than once via an internal sync.Once) and must be
// called exactly once per successful Acquire/Register to avoid refcount leaks. Distinct
// from ForceReleaseFunc so that future callers storing it generically cannot silently
// conflate refcount-gated and unconditional teardown (type-driven-audit finding B).
type ReleaseFunc func()

// ForceReleaseFunc marks an unconditional-teardown closure: evicts regardless of refcount.
// No wrapper of ForceRelease exists in this plan — ForceRelease is always called directly
// with a sessionID. This type exists so that if a future caller wraps ForceRelease into a
// closure, the return type says so explicitly instead of degrading to a bare func().
type ForceReleaseFunc func()

// ErrSessionNotFound is returned by Acquire when the sessionID is not known to Storage.
var ErrSessionNotFound = errors.New("session: not found in storage")

// ErrSessionAlreadyRegistered is returned by Register when a LiveInstance for the given
// ID is already present in the registry (duplicate-ID collision guard).
var ErrSessionAlreadyRegistered = errors.New("session: already registered")

type registryEntry struct {
	instance *LiveInstance
	refcount int
}

// Registry owns the sessionID → live-actor mapping plus refcounts.
// Its mutex guards map membership only — not per-field Instance state.
// Construct with NewRegistry; the zero value is not usable.
type Registry struct {
	storage *Storage
	mu      sync.Mutex
	entries map[string]*registryEntry
	// onConstruct, if non-nil, is invoked exactly once per genuinely-new LiveInstance —
	// i.e. only on the branch that installs a fresh entry into r.entries.
	// NEVER invoked on a refcount++ of an already-live entry, NEVER on the losing side
	// of a construction race (that branch discards via stopActor() instead).
	// Register does NOT invoke onConstruct — see Register's doc comment for why.
	onConstruct func(*LiveInstance)
}

// NewRegistry constructs a Registry. onConstruct may be nil (e.g. daemon.go's own
// Registry, which has no SessionService to wire callbacks for); Acquire nil-checks
// before calling it.
func NewRegistry(storage *Storage, onConstruct func(*LiveInstance)) *Registry {
	return &Registry{
		storage:     storage,
		entries:     make(map[string]*registryEntry),
		onConstruct: onConstruct,
	}
}

// Acquire returns the live handle for sessionID, constructing its actor on first access.
// On success, the caller MUST call the returned ReleaseFunc exactly once — prefer
// WithInstance for synchronous single-call-stack callers to avoid forgetting it.
//
// Three outcomes:
//  1. Not in storage → ErrSessionNotFound
//  2. Not in map, construction succeeds → new entry, refcount=1
//  3. Already in map (or races with concurrent Acquire) → refcount++
func (r *Registry) Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error) {
	r.mu.Lock()
	if e, ok := r.entries[sessionID]; ok {
		e.refcount++
		r.mu.Unlock()
		return e.instance, r.makeRelease(sessionID), nil
	}
	r.mu.Unlock() // release before storage I/O — construction must not block unrelated Acquires

	data, err := r.storage.FindInstanceDataByID(sessionID)
	if errors.Is(err, ErrInstanceDataNotFound) {
		return nil, nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("registry: acquire %q: %w", sessionID, err)
	}
	live, err := newLiveInstance(*data, r.storage)
	if err != nil {
		return nil, nil, fmt.Errorf("registry: construct %q: %w", sessionID, err)
	}

	// Re-check under lock: a concurrent Acquire may have won the race and installed
	// an entry already. Never let two live actors exist for one ID — adopt the winner's
	// entry, discard ours (stopActor is a no-op pre-Epic-3).
	r.mu.Lock()
	if e, ok := r.entries[sessionID]; ok {
		live.stopActor()
		e.refcount++
		r.mu.Unlock()
		return e.instance, r.makeRelease(sessionID), nil
	}
	r.entries[sessionID] = &registryEntry{instance: live, refcount: 1}
	r.mu.Unlock() // unlock before onConstruct — wiring may do non-trivial work
	if r.onConstruct != nil {
		r.onConstruct(live) // exactly once: this is the sole genuine-construction branch
	}
	return live, r.makeRelease(sessionID), nil
}

// Register is the construction-time counterpart to Acquire (R2.18a). CreateSession
// builds a brand-new *LiveInstance via NewInstance (no persisted row exists yet for
// Acquire to look up) and hands it to Register before calling storage.AddInstance.
//
// Register deliberately does NOT invoke onConstruct: CreateSession already performs its
// own explicit post-construction wiring, so routing Register through onConstruct would
// wire the same callbacks twice. onConstruct exists solely to backfill wiring for the
// Acquire-from-storage path (sessions loaded on server restart), which has no other
// caller positioned to do it.
//
// No double-checked locking here (unlike Acquire): Register has no storage I/O to
// release the lock around, so the whole check-then-insert runs under one lock acquisition.
func (r *Registry) Register(instance *LiveInstance) (ReleaseFunc, error) {
	sessionID := instance.GetStableID()
	r.mu.Lock()
	if _, ok := r.entries[sessionID]; ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("registry: register %q: %w", sessionID, ErrSessionAlreadyRegistered)
	}
	r.entries[sessionID] = &registryEntry{instance: instance, refcount: 1}
	r.mu.Unlock()
	return r.makeRelease(sessionID), nil
}

// makeRelease returns an idempotent ReleaseFunc for one Acquire/Register call.
// A double-call on this closure is a no-op (sync.Once); two independent Acquires
// each get their own independently-firing closure.
func (r *Registry) makeRelease(sessionID string) ReleaseFunc {
	var once sync.Once
	return func() { once.Do(func() { r.release(sessionID) }) }
}

// release decrements the refcount for sessionID. When it reaches zero, the entry is
// removed from the map and the actor is stopped. decrement+zero-check+map-delete share
// one critical section with Acquire's check-then-increment (pitfalls-registry.md §3
// Design A) so mutual exclusion is automatic: a concurrent Acquire either sees the
// entry and increments (this release's zero-check then skips teardown) or this release
// removes it first (the concurrent Acquire finds nothing and builds a fresh one).
// stopActor runs outside the lock per the "no I/O under the lock" discipline.
func (r *Registry) release(sessionID string) {
	r.mu.Lock()
	e, ok := r.entries[sessionID]
	if !ok {
		r.mu.Unlock()
		log.Warn("registry: release for unknown sessionID", "id", sessionID)
		return
	}
	e.refcount--
	if e.refcount > 0 {
		r.mu.Unlock()
		return
	}
	delete(r.entries, sessionID)
	r.mu.Unlock()
	e.instance.stopActor()
}

// ForceRelease tears down sessionID's actor and map entry immediately, regardless of
// refcount (R2.18 — DeleteSession's force-invalidate; also used by CreateSession to
// abort a Register()'d entry when the immediately-following storage.AddInstance fails).
//
// Other holders' *LiveInstance pointers stay valid Go values; their next command must
// return a typed error (Story 2.5.9c's contract, implemented in Epic 3), never hang.
//
// For CreateSession's abort path: use ForceRelease (not the release() closure Register
// returned) because a concurrent Acquire racing between Register and the abort would
// bump refcount to 2, making plain release() decrement 2→1 and leave the phantom entry
// alive. ForceRelease deletes unconditionally, regardless of current refcount.
func (r *Registry) ForceRelease(sessionID string) {
	r.mu.Lock()
	e, ok := r.entries[sessionID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.entries, sessionID)
	r.mu.Unlock()
	e.instance.stopActor()
}

// AcquireAll acquires every session known to Storage in one call; returns one release
// closing over all of them. Sugar for sweep-style callers (health.go, hibernation_sweeper.go).
// Sessions that fail to Acquire are logged and skipped, not returned.
func (r *Registry) AcquireAll() ([]*LiveInstance, ReleaseFunc, error) {
	ids, err := r.storage.ListInstanceIDs()
	if err != nil {
		return nil, nil, fmt.Errorf("registry: AcquireAll: list IDs: %w", err)
	}
	var live []*LiveInstance
	var releases []ReleaseFunc
	for _, id := range ids {
		l, release, acquireErr := r.Acquire(id)
		if acquireErr != nil {
			log.Warn("registry: AcquireAll: skipping session", "id", id, "err", acquireErr)
			continue
		}
		live = append(live, l)
		releases = append(releases, release)
	}
	releaseAll := ReleaseFunc(func() {
		for _, rel := range releases {
			rel()
		}
	})
	return live, releaseAll, nil
}

// Shutdown force-stops every actor regardless of refcount. Register this as a
// shutdownHooks entry (Story 2.5.5d) so it fires on server shutdown.
func (r *Registry) Shutdown() {
	r.mu.Lock()
	entries := r.entries
	r.entries = make(map[string]*registryEntry)
	r.mu.Unlock()
	for _, e := range entries {
		e.instance.stopActor()
	}
}

// Storage returns the Registry's backing storage. Used by callers (e.g. daemon.go)
// that need access to storage through a Registry reference.
func (r *Registry) Storage() *Storage { return r.storage }

// WithInstance is the preferred entry point for synchronous, single-call-stack callers
// (RPC handlers, one-shot lookups) where forgetting release() is the common failure mode.
// Reserve raw Acquire/release() for genuinely long-lived holders (WebSocket streams,
// poller caches, background goroutines).
func (r *Registry) WithInstance(ctx context.Context, sessionID string, fn func(*LiveInstance) error) error {
	inst, release, err := r.Acquire(sessionID)
	if err != nil {
		return err
	}
	defer release()
	return fn(inst)
}

// List returns a snapshot of all currently-live instances, holding the lock only for
// the copy.
func (r *Registry) List() []*LiveInstance {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*LiveInstance, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.instance)
	}
	return out
}

// Count returns the number of live entries.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// InstanceAcquirer is the narrowest interface for callers that only ever call Acquire.
// WorkspaceService, MCP tool handlers, and most RPC handlers should be typed against
// this rather than *Registry (Interface Segregation — matches WorkspaceService's existing
// LiveInstanceFinder convention).
type InstanceAcquirer interface {
	Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error)
}

// RegistryInspector is the narrowest interface for callers that only enumerate live
// instances without acquiring individual handles.
type RegistryInspector interface {
	List() []*LiveInstance
	Count() int
}

// Compile-time assertions: *Registry satisfies both interfaces.
var _ InstanceAcquirer = (*Registry)(nil)
var _ RegistryInspector = (*Registry)(nil)
