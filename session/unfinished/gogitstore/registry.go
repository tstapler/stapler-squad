package gogitstore

import (
	"cmp"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/tstapler/stapler-squad/log"
)

// registryStoreTTL bounds how long a SharedObjectStore with a zero
// reference count (no live WorktreeStorer using it) stays cached before
// Prune's TTL pass evicts it. Mirrors gogit_vcs_reader.go's repoCacheTTL —
// 30 minutes comfortably covers the scanner's poll cycle while bounding
// memory for repos that have gone genuinely cold.
const registryStoreTTL = 30 * time.Minute

// registryMaxEntries is the absolute hard ceiling on SharedObjectStores held
// by one Registry, mirroring gogit_vcs_reader.go's repoCacheMaxEntries — a
// backstop so a misconfigured/miscalculated budget can never regress past
// this number. The everyday cap is effectiveMaxEntries(), normally far
// smaller.
const registryMaxEntries = 100

// registryDefaultMemoryBudgetBytes is the default byte budget for a
// Registry under normal (non-pressured) conditions, mirroring
// gogit_vcs_reader.go's repoCacheMemoryBudgetBytes. Deliberately far below
// registryMaxEntries*a-store's-cache-size — this process runs alongside many
// other memory-hungry tools on a single host.
const registryDefaultMemoryBudgetBytes = 1536 * 1024 * 1024

// registryHighMemoryPressureThreshold/registrySevereMemoryPressureThreshold
// gate effectiveBudgetBytes' tiered response, mirroring
// gogit_vcs_reader.go's highMemoryPressureThreshold/
// severeMemoryPressureThreshold. Measured against this process's own
// runtime.MemStats.HeapInuse (via readHeapInUse below), not host-wide
// memory.
const (
	registryHighMemoryPressureThreshold   = 3 * 1024 * 1024 * 1024
	registrySevereMemoryPressureThreshold = 6 * 1024 * 1024 * 1024
)

// readHeapInUse returns the process's current in-use heap bytes. This is a
// deliberate small duplicate of gogit_vcs_reader.go's own readHeapInUse var
// (same name, different package — no import cycle risk) rather than a
// cross-package dependency: gogitstore is meant to be usable independently
// of session/unfinished's GoGitVCSReader, and this is a two-line function,
// not a shared abstraction worth coupling two packages over. Declared as a
// var (not a plain function) so tests can override it to simulate memory
// pressure deterministically without needing to actually allocate gigabytes
// — see gogit_vcs_reader_memory_test.go's withHeapInUse for the pattern this
// mirrors.
var readHeapInUse = func() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

// Registry hands out one *SharedObjectStore per commondir, so every
// worktree of the same repository resolves to the same shared index +
// object cache. Callers should construct one Registry per process (or per
// logical "pool of repos this component cares about") and reuse it across
// every Open call — a fresh Registry per call defeats the entire point.
//
// Eviction: Registry evicts idle SharedObjectStores via TTL and a
// memory-derived budget, mirroring session/unfinished/gogit_vcs_reader.go's
// GoGitVCSReader.repoCache (pruneRepoCache/effectiveCacheBudgetBytes) — see
// Prune's doc comment. A store is NEVER evicted while its reference count is
// nonzero (i.e. at least one live WorktreeStorer still points at it) — see
// SharedObjectStore.refCount and acquire/release below. Callers are
// responsible for calling Prune() periodically (proactively, e.g. on a
// timer) and MAY rely on acquire's own reactive prune-on-overflow as a
// backstop, mirroring openRepoEntry's reactive pruneRepoCache call in
// gogit_vcs_reader.go.
type Registry struct {
	// CacheMaxSize is the per-commondir decoded-object LRU cache size in
	// bytes, passed to cache.NewObjectLRU for each new SharedObjectStore.
	// Zero-value Registry uses cache.DefaultMaxSize (go-git's own 96MB
	// default) — callers migrating off the OOM-prone default should set
	// this explicitly (e.g. 12MB, matching the parallel cache-sharing fix
	// in gogit_vcs_reader.go).
	CacheMaxSize int64

	// LargeObjectThreshold is forwarded to packfile.NewPackfileWithCache;
	// see storage/filesystem.Options.LargeObjectThreshold upstream. Zero
	// means "no limit" (matches upstream's zero-value behavior).
	LargeObjectThreshold int64

	// UseMmapIndex enables stage 2's mmap-backed .idx loader (mmapindex.go,
	// design doc §5) for every SharedObjectStore this Registry creates from
	// this point forward — existing stores keep whatever mode they were
	// created with; flipping this field does not retroactively convert an
	// already-open store.
	//
	// Default false. Safe to set directly via a struct literal (&Registry{
	// UseMmapIndex: true}) only before the Registry is shared across
	// goroutines (e.g. in a test, before Open is ever called). Once a
	// Registry may be read concurrently by acquire() — true for any
	// production Registry — use SetUseMmapIndex instead, which holds the
	// same mutex acquire() reads this field under; a bare field write from
	// another goroutine after that point is a data race.
	//
	// session/unfinished/gogit_vcs_reader.go's gogitstoreRegistry() — the
	// constructor for the Registry actually used by the live production
	// scanner — leaves this at its zero value and instead calls
	// SetUseMmapIndex on every openRepoEntry call with the live value of
	// the "unfinished:mmap-index" feature flag, so toggling that flag
	// (config.SetFeatureFlag, e.g. via the Settings UI) takes effect for
	// the next repo opened or re-opened after eviction — no process
	// restart required — without needing this field to itself be an
	// atomic type, since every write funnels through this one
	// mutex-guarded setter.
	UseMmapIndex bool

	mu     sync.Mutex
	stores map[string]*SharedObjectStore // key: filepath.Clean'd absolute commondir
}

// SetUseMmapIndex sets UseMmapIndex under the same mutex acquire() reads it
// under — the safe way to change this field on a Registry that may already
// be in concurrent use (i.e. any production Registry). Only affects
// SharedObjectStores created after this call; see UseMmapIndex's doc
// comment.
func (r *Registry) SetUseMmapIndex(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.UseMmapIndex = v
}

// NewRegistry returns a Registry using go-git's own default object-cache
// size (96MB) per commondir. Use &Registry{CacheMaxSize: N} directly to
// override it.
func NewRegistry() *Registry {
	return &Registry{}
}

// acquire returns the SharedObjectStore for commonDirAbs, creating it on
// first use, and bumps its reference count — see
// SharedObjectStore.acquireRefLocked's doc comment for why the increment
// happens HERE, still under r.mu, rather than as a separate call made by the
// caller after acquire returns (that gap is exactly the TOCTOU window that
// would let Prune evict a store the instant before its ref count is bumped).
// Every successful acquire call MUST be matched by exactly one
// SharedObjectStore.release() call (via WorktreeStorer.Close(), see
// storer.go) once the caller is done with the returned store — open.go's
// Open is the only current caller, and gogit_vcs_reader.go's
// GoGitVCSReader is responsible for calling Close() when it evicts the
// cachedRepo wrapping the resulting WorktreeStorer.
//
// commonDirAbs must already be an absolute, filepath.Clean'd path — callers
// (open.go) are responsible for normalizing it so that two
// different-looking-but-equal paths (e.g. a trailing slash) don't
// accidentally create two stores for one commondir.
func (r *Registry) acquire(commonDirAbs string, commonFs billy.Filesystem) *SharedObjectStore {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stores == nil {
		r.stores = make(map[string]*SharedObjectStore)
	}
	if s, ok := r.stores[commonDirAbs]; ok {
		s.acquireRefLocked()
		return s
	}

	// Reactive prune-on-overflow, mirroring gogit_vcs_reader.go's
	// openRepoEntry: if the registry is already at or past its
	// memory-derived budget, try to make room before growing it further.
	// This can only free zero-refcount stores (see pruneLocked) — if every
	// existing store is still in active use, this is a no-op and the
	// registry is allowed to grow past the budget rather than fail the
	// caller's Open, matching effectiveCacheMaxEntries' "cap is a target,
	// not a hard failure" behavior in gogit_vcs_reader.go.
	if int64(len(r.stores)) >= r.effectiveMaxEntries() {
		r.pruneLocked()
	}

	maxSize := r.CacheMaxSize
	if maxSize <= 0 {
		maxSize = int64(cache.DefaultMaxSize)
	}
	s := newSharedObjectStore(commonDirAbs, commonFs, cache.NewObjectLRU(cache.FileSize(maxSize)), r.LargeObjectThreshold, r.UseMmapIndex)
	s.acquireRefLocked()
	r.stores[commonDirAbs] = s
	return s
}

// approxBytesPerStore estimates the memory cost of one SharedObjectStore,
// dominated by its decoded-object LRU cache (CacheMaxSize) plus a smaller,
// unmeasured contribution from the parsed pack index. Mirrors
// gogit_vcs_reader.go's approxBytesPerCachedRepo, but derived from this
// Registry's own configured CacheMaxSize rather than a flat constant, since
// that is the one real, caller-controlled knob affecting a store's size.
func (r *Registry) approxBytesPerStore() int64 {
	if r.CacheMaxSize > 0 {
		return r.CacheMaxSize
	}
	return int64(cache.DefaultMaxSize)
}

// effectiveBudgetBytes returns the current byte budget for this Registry,
// shrinking under real process memory pressure — the gogitstore-package twin
// of gogit_vcs_reader.go's effectiveCacheBudgetBytes.
func (r *Registry) effectiveBudgetBytes() int64 {
	heapInUse := readHeapInUse()
	switch {
	case heapInUse >= registrySevereMemoryPressureThreshold:
		return r.approxBytesPerStore() * 5 // floor: keep only ~5 stores hot
	case heapInUse >= registryHighMemoryPressureThreshold:
		return registryDefaultMemoryBudgetBytes / 2
	default:
		return registryDefaultMemoryBudgetBytes
	}
}

// effectiveMaxEntries derives the current entry-count cap from the memory
// budget, clamped to registryMaxEntries as an absolute ceiling and to 1 as a
// floor — the gogitstore-package twin of gogit_vcs_reader.go's
// effectiveCacheMaxEntries.
func (r *Registry) effectiveMaxEntries() int64 {
	n := r.effectiveBudgetBytes() / r.approxBytesPerStore()
	if n > registryMaxEntries {
		n = registryMaxEntries
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Prune evicts zero-refcount SharedObjectStore entries idle past
// registryStoreTTL, then LRU-trims any remaining zero-refcount entries
// (oldest-idle first) if the registry is still over its memory-derived
// budget (effectiveMaxEntries) — the gogitstore-package twin of
// gogit_vcs_reader.go's pruneRepoCache. Intended to be called proactively on
// a timer (mirroring Scanner.Start's 1-minute PruneToMemoryBudget ticker —
// see gogit_vcs_reader.go/scanner.go) and is also called reactively from
// acquire on overflow, matching openRepoEntry's own reactive prune call.
//
// A store with a nonzero reference count (at least one live WorktreeStorer
// still points at it — see SharedObjectStore.refCount) is NEVER evicted,
// regardless of TTL or budget pressure — skipped and logged at DEBUG. This
// is the core safety property the design doc's stage-1 rollout requires:
// evicting a store still backing a live worktree would silently break the
// cross-worktree sharing invariant (a NEW worktree opened afterward for the
// same commondir would build a second, independent SharedObjectStore rather
// than reusing the existing one — doubling memory for that repo rather than
// crashing, but defeating the whole point of this package).
//
// Concurrency note: Prune holds r.mu for its entire body, the same lock
// acquire holds while bumping a store's refcount (see acquire's doc
// comment) — so a store's refcount can never transition 0→1 via a
// concurrent acquire while Prune is deciding whether that same store is
// evictable; both are strictly serialized. A concurrent release() (called
// from a DIFFERENT WorktreeStorer's Close, decrementing 1→0) is NOT
// serialized against Prune the same way — release only touches the store's
// own atomic fields, not r.mu — but this is benign: if Prune observes
// refCount==0 because a release() happened moments ago, evicting the store
// is correct (nothing is using it), and if a store's refcount transitions
// to zero DURING Prune's iteration, at worst that store survives one more
// Prune cycle before becoming eligible (never the reverse: Prune can never
// evict a store while something is actively using it).
func (r *Registry) Prune() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
}

// pruneLocked is Prune's implementation. Callers MUST already hold r.mu —
// see acquire's reactive-prune-on-overflow call for the other caller besides
// the exported, lock-acquiring Prune.
func (r *Registry) pruneLocked() {
	if len(r.stores) == 0 {
		return
	}

	cutoff := time.Now().Add(-registryStoreTTL).UnixNano()
	type candidate struct {
		key           string
		unusedSinceNs int64
	}
	var candidates []candidate
	for k, s := range r.stores {
		if s.RefCount() != 0 {
			log.DebugLog.Printf("[gogitstore.Registry] skipping eviction of %s: refCount=%d (still in use by at least one worktree)", k, s.RefCount())
			continue
		}
		unusedSince := atomic.LoadInt64(&s.unusedSinceNs)
		if unusedSince != 0 && unusedSince < cutoff {
			delete(r.stores, k)
			s.stopPackWatch() // no-op for non-mmap stores; see mmapwatch.go
			continue
		}
		candidates = append(candidates, candidate{k, unusedSince})
	}

	maxEntries := r.effectiveMaxEntries()
	if int64(len(r.stores)) <= maxEntries || len(candidates) == 0 {
		return
	}

	// LRU trim: evict the longest-idle zero-refcount stores until back
	// within budget, or until no more zero-refcount candidates remain
	// (stores still in use are never touched — see the doc comment above).
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(a.unusedSinceNs, b.unusedSinceNs)
	})
	overBudget := int64(len(r.stores)) - maxEntries
	for i := 0; i < len(candidates) && int64(i) < overBudget; i++ {
		key := candidates[i].key
		if s, ok := r.stores[key]; ok {
			s.stopPackWatch() // no-op for non-mmap stores; see mmapwatch.go
		}
		delete(r.stores, key)
	}
}

// RefreshIndexes re-diffs the SharedObjectStore currently registered for
// commonDirAbs (if any) against the on-disk pack set — see
// SharedObjectStore.refreshIndexes. A no-op (not an error) if no store is
// currently registered for commonDirAbs, e.g. before the first Open of that
// repo, or after it has been evicted. Exposed mainly for tests and for any
// caller that wants to force a staleness recheck outside of
// mmapwatch.go's own fsnotify-driven debounce loop.
func (r *Registry) RefreshIndexes(commonDirAbs string) {
	r.mu.Lock()
	s, ok := r.stores[commonDirAbs]
	r.mu.Unlock()
	if !ok {
		return
	}
	if err := s.refreshIndexes(); err != nil {
		log.Warn("gogitstore: RefreshIndexes failed", "commonDir", commonDirAbs, "err", err)
	}
}

// Stats returns a snapshot of (commondir -> parsed index-entry count) for
// every SharedObjectStore currently registered (i.e. not yet evicted by
// Prune) — prototype introspection for tests/manual verification, not meant
// as a stable API.
func (r *Registry) Stats() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.stores))
	for k, s := range r.stores {
		out[k] = s.IndexEntryCount
	}
	return out
}

// RefCounts returns a snapshot of (commondir -> current reference count) for
// every SharedObjectStore currently registered — prototype introspection for
// tests/manual verification of the refcount-safety property Prune depends
// on, not meant as a stable API.
func (r *Registry) RefCounts() map[string]int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int32, len(r.stores))
	for k, s := range r.stores {
		out[k] = s.RefCount()
	}
	return out
}
