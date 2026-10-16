package unfinished

import (
	"cmp"
	"container/list"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/unfinished/gogitstore"
	"golang.org/x/sync/singleflight"
)

// diffStatCacheTTL is how long a DiffShortstat result is reused before re-scanning.
// The scanner runs every 30s per repo; a 30s TTL avoids redundant wt.Status() calls
// when multiple goroutines or scan cycles hit the same worktree path.
const diffStatCacheTTL = 30 * time.Second

// maxUntrackedFiles is the hard cap on untracked files walked per repo. Repos with
// large build-artifact trees (e.g. Android/Bazel projects with 200K+ .class/.dex files)
// would otherwise trigger an OOM by calling os.ReadFile on every file.
// Declared as var (not const) so tests can lower it without creating 10K files.
var maxUntrackedFiles = 10_000

// maxUntrackedFileSize is the per-file read limit for untracked line-counting.
// Files larger than this (binaries, JARs, APKs, etc.) have their file count
// incremented but their line count skipped. Also used for blob and working-tree
// file reads in applyDiff.
// Declared as var (not const) so tests can use smaller thresholds.
var maxUntrackedFileSize = int64(512 * 1024) // 512 KB

// errStopWalk is returned by walkUntrackedRec to signal early termination when
// the maxUntrackedFiles cap has been reached.
var errStopWalk = errors.New("stop untracked walk")

// blobCacheTotalTargetFraction is the share of the CURRENT repoCache budget
// (effectiveCacheBudgetBytes, which already shrinks under memory pressure)
// reserved for ALL blobCaches combined, split evenly across however many
// repos are actually cached right now (see effectiveBlobCacheMaxBytes). This
// rides on the existing pressure-aware budget instead of being a second,
// disconnected budget: as effectiveCacheBudgetBytes shrinks under pressure,
// the blob-cache total shrinks with it automatically.
// Declared as var (not const) so tests can tune it without needing to spin
// up blobCacheTotalTargetFraction-many fake repos.
var blobCacheTotalTargetFraction int64 = 8 // 1/8th of the repoCache budget

// blobCacheMaxBytesFloor/Ceiling bound a single repo's per-put cap regardless
// of how many repos are currently cached — the floor keeps a lone quiet repo
// from being starved to uselessness when many repos are hot (n large), the
// ceiling keeps a single repo from hogging an outsized share when few repos
// are hot (n small). Ceiling is deliberately a small fraction of
// approxBytesPerCachedRepo, for the same reason the old flat cap was: this
// cache lives inside one cachedRepo, whose total cost that constant already
// budgets for.
const (
	blobCacheMaxBytesFloor   = 2 * 1024 * 1024              // 2MB: room for a handful of typical changed files
	blobCacheMaxBytesCeiling = approxBytesPerCachedRepo / 4 // 24MB
)

// effectiveBlobCacheMaxBytes returns the current per-repo blobCache cap,
// scaled so that (repos currently cached) * (this cap) stays within
// roughly effectiveCacheBudgetBytes()/blobCacheTotalTargetFraction — more
// budget per repo when few repos are hot, less when many are, clamped to
// [blobCacheMaxBytesFloor, blobCacheMaxBytesCeiling]. Reuses repoCacheSize
// (already maintained atomically for repoCache's own eviction trigger)
// rather than a fresh Range scan.
func (g *GoGitVCSReader) effectiveBlobCacheMaxBytes() int {
	n := atomic.LoadInt64(&g.repoCacheSize)
	if n < 1 {
		n = 1
	}
	perRepo := effectiveCacheBudgetBytes() / blobCacheTotalTargetFraction / n
	switch {
	case perRepo > blobCacheMaxBytesCeiling:
		return blobCacheMaxBytesCeiling
	case perRepo < blobCacheMaxBytesFloor:
		return blobCacheMaxBytesFloor
	default:
		return int(perRepo)
	}
}

// blobCacheLRU is a byte-size-bounded LRU cache of blob content keyed by
// plumbing.Hash. It exists because blobCache (see cachedRepo) now persists
// across HEAD moves instead of being wiped every commit, so it needs its own
// bound rather than relying on commit frequency to cap it. Recently-touched
// blobs are the ones most likely to be re-requested on the next poll cycle
// (the scanner re-diffs the same worktree every 30s), so LRU eviction — not
// LIFO — is the right policy: it keeps what was just used and drops what
// hasn't been touched in a while.
//
// Not safe for concurrent use on its own — every call site in this file
// holds entry.mu for the duration of its use, so it needs no internal lock.
type blobCacheLRU struct {
	order *list.List // front = most recently used
	elems map[plumbing.Hash]*list.Element
	size  int
}

type blobCacheNode struct {
	hash plumbing.Hash
	data []byte
}

// forEach visits every cached entry in most-recently-used order, stopping
// early if fn returns false. Test-only helper (mirrors sync.Map.Range's
// shape, which is what this cache replaced).
func (c *blobCacheLRU) forEach(fn func(hash plumbing.Hash, data []byte) bool) {
	if c.order == nil {
		return
	}
	for el := c.order.Front(); el != nil; el = el.Next() {
		node := el.Value.(blobCacheNode)
		if !fn(node.hash, node.data) {
			return
		}
	}
}

func (c *blobCacheLRU) get(hash plumbing.Hash) ([]byte, bool) {
	el, ok := c.elems[hash]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(blobCacheNode).data, true
}

// put inserts data under hash, evicting least-recently-used entries until
// the cache is at or under maxBytes. maxBytes is passed in per-call (see
// effectiveBlobCacheMaxBytes) rather than read from a fixed field, since the
// right cap depends on how many other repos are currently cached.
func (c *blobCacheLRU) put(hash plumbing.Hash, data []byte, maxBytes int) {
	if c.order == nil {
		c.order = list.New()
		c.elems = make(map[plumbing.Hash]*list.Element)
	}
	if el, ok := c.elems[hash]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.elems[hash] = c.order.PushFront(blobCacheNode{hash, data})
	c.size += len(data)
	for c.size > maxBytes {
		back := c.order.Back()
		if back == nil {
			c.size = 0
			break
		}
		evicted := back.Value.(blobCacheNode)
		c.order.Remove(back)
		delete(c.elems, evicted.hash)
		c.size -= len(evicted.data)
	}
}

type diffStatEntry struct {
	result DiffStat
	expiry time.Time
}

type hasUncommittedEntry struct {
	result bool
	expiry time.Time
}

type aheadBehindEntry struct {
	ahead    int
	behind   int
	headHash plumbing.Hash
	baseHash plumbing.Hash
	expiry   time.Time
}

type commitMessagesEntry struct {
	msgs   []string
	expiry time.Time
}

// reachableSetEntry has no expiry: a commit's ancestry is fixed by its hash, so
// an entry is valid for as long as baseHash is the worktree's current base.
type reachableSetEntry struct {
	baseHash plumbing.Hash
	set      map[plumbing.Hash]bool
}

// repoCacheMaxEntries is the ABSOLUTE hard ceiling on repositories held in
// repoCache, regardless of the memory budget below. It exists so a
// misconfigured or miscalculated budget can never regress past this number —
// the real day-to-day cap is effectiveCacheBudgetBytes()/approxBytesPerCachedRepo,
// which is normally far smaller than this.
const repoCacheMaxEntries = 100

// repoCacheTTL is the duration after which an unaccessed repo entry is evicted.
// 30 minutes covers the scanner's poll cycle comfortably while bounding memory.
const repoCacheTTL = 30 * time.Minute

// approxBytesPerCachedRepo estimates the memory cost of one open *cachedRepo,
// dominated by go-git's internal packfile index + object LRU cache. Measured
// empirically (see ClearCache's doc comment / prior profiling sessions) at
// ~96 MB per repo. This is an estimate, not a measurement of any single repo —
// it drives a budget-based eviction policy, not exact accounting.
const approxBytesPerCachedRepo = 96 * 1024 * 1024

// repoCacheMemoryBudgetBytes is the default byte budget for repoCache under
// normal (non-pressured) conditions. Deliberately far below the
// repoCacheMaxEntries*approxBytesPerCachedRepo ceiling (~9.6 GB): this process
// runs alongside many other memory-hungry tools on a single host, and a
// scanner cache has no business claiming multiple GB of that budget on its
// own.
//
// 3 GB covers roughly 32 simultaneously "hot" repos at the ~96 MB estimate --
// doubled from an original 1.5 GB/16-repo budget after live /debug/blob-cache
// stats on a host running dozens of concurrent sessions showed a 0.07% hit
// rate (77 hits / 117330 misses): with more actively-scanned repos than the
// cache had room for, every poll cycle evicted and reopened a cold repo,
// wiping its per-repo blobCache before the next poll could ever hit it. This
// is a size trade-off, not a fix for unbounded growth -- the
// high/severeMemoryPressureThreshold tiers below still shrink this budget
// under real process memory pressure, so a host that can't afford the extra
// headroom degrades gracefully rather than OOMing.
const repoCacheMemoryBudgetBytes = 3072 * 1024 * 1024

// highMemoryPressureThreshold/severeMemoryPressureThreshold gate
// effectiveCacheBudgetBytes' tiered response, measured against the Go
// runtime's own HeapInuse (this process's heap, not host-wide memory).
//
// Both are derived from repoCacheMemoryBudgetBytes rather than independent
// constants, specifically so normal cache growth toward its own budget can
// never itself cross into a pressure tier: an earlier version set
// highMemoryPressureThreshold to a flat 3 GB, identical to the budget it
// gates, so filling the cache to its own full budget triggered the
// high-pressure branch, which halved the budget and forced eviction --
// heap then dropped back below 3 GB, the budget rose back to 3 GB, and the
// cache refilled toward 3 GB again, an oscillating grow/evict loop that
// re-created the exact cache-thrashing symptom repoCacheMemoryBudgetBytes'
// own doc comment describes fixing.
const (
	highMemoryPressureThreshold   = repoCacheMemoryBudgetBytes + repoCacheMemoryBudgetBytes/2 // 1.5x budget: 4.5 GB
	severeMemoryPressureThreshold = repoCacheMemoryBudgetBytes * 2                            // 2x budget: 6 GB
)

// readHeapInUse returns the process's current in-use heap bytes. Declared as
// a var (not a plain function) so tests can override it to simulate memory
// pressure deterministically without needing to actually allocate gigabytes.
var readHeapInUse = func() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

// effectiveCacheBudgetBytes returns the current byte budget for repoCache,
// shrinking under real process memory pressure so the cache degrades
// gracefully under load instead of contributing to an OOM. This is the
// memory-based replacement for a flat entry-count cap: pruneRepoCache derives
// its effective max-entries from this budget, not from a fixed constant.
func effectiveCacheBudgetBytes() int64 {
	heapInUse := readHeapInUse()
	switch {
	case heapInUse >= severeMemoryPressureThreshold:
		return approxBytesPerCachedRepo * 5 // floor: keep only ~5 repos hot
	case heapInUse >= highMemoryPressureThreshold:
		return repoCacheMemoryBudgetBytes / 2
	default:
		return repoCacheMemoryBudgetBytes
	}
}

// effectiveCacheMaxEntries derives the current entry-count cap from the
// memory budget, clamped to repoCacheMaxEntries as an absolute ceiling and to
// 1 as a floor (an empty cache would defeat the whole point of caching).
func effectiveCacheMaxEntries() int64 {
	n := effectiveCacheBudgetBytes() / approxBytesPerCachedRepo
	if n > repoCacheMaxEntries {
		n = repoCacheMaxEntries
	}
	if n < 1 {
		n = 1
	}
	return n
}

// UnderSeverePressure reports whether this process's heap is currently at or
// above severeMemoryPressureThreshold. Scanner callers use this to skip a
// scan cycle for a repo (staying on its last-known-good cached result)
// instead of piling on more allocation while already under pressure.
func (g *GoGitVCSReader) UnderSeverePressure() bool {
	return readHeapInUse() >= severeMemoryPressureThreshold
}

// cachedRepo holds an open go-git Repository and a per-repo mutex that
// serialises concurrent VCS reads. go-git's packfile reader is not safe for
// concurrent use; holding this mutex for the duration of a VCS operation
// eliminates the internal packfile-reader contention that was the #1 profiling
// hotspot (3.8B cycles, 36K events).
type cachedRepo struct {
	repo         *git.Repository
	mu           sync.Mutex
	accessedAtNs int64 // atomic UnixNano; updated on every cache hit

	// headTreeHash/headTreeCache cache the name->blob-hash map for the HEAD tree,
	// valid only while headTreeHash still matches the repo's current HEAD commit.
	// Both fields are guarded by mu (always held while diffShortstatUncached reads
	// or repopulates them), so plain reads/writes are safe without atomics.
	headTreeHash  plumbing.Hash
	headTreeCache map[string]plumbing.Hash

	// blobCache holds blob content keyed by its (content-addressed, thus
	// immutable) hash. A hit is always correct regardless of where HEAD points
	// — the same blob hash decodes to the same bytes forever — so entries are
	// NOT cleared when HEAD moves (unlike headTreeCache/untrackedMatcher
	// above, which cache HEAD-relative *mappings* and do need that
	// invalidation). Since it now persists for the cachedRepo's whole
	// lifetime rather than being wiped every commit, it is capped via LRU
	// eviction (blobCacheLRU) instead of relying on HEAD-move churn to bound
	// it — see GoGitVCSReader.effectiveBlobCacheMaxBytes for how the cap is
	// sized. Guarded by mu, same as the fields above.
	blobCache blobCacheLRU

	// untrackedMatcherBuilt/untrackedMatcher cache the compiled gitignore
	// matcher used by the untracked-file walk in diffShortstatUncached.
	// Rebuilt only when HEAD moves AND a .gitignore file actually changed
	// (resolveHeadTreeHashes clears untrackedMatcherBuilt in that case,
	// checked via gitignoreEntriesChanged) — most commits don't touch
	// .gitignore, so this avoids paying gitignore.ReadPatterns' recursive
	// filesystem walk on every commit. A .gitignore edit made between HEAD
	// moves can still be stale for up to one poll cycle, an acceptable
	// trade-off matching this file's other TTL-based staleness tolerances
	// (e.g. diffStatCacheTTL). Guarded by mu, same as the fields above.
	untrackedMatcherBuilt bool
	untrackedMatcher      gitignore.Matcher
}

// gogitstoreCloser is implemented by storage.Storer values that hold a
// reference into a gogitstore.Registry and must release it when the
// cachedRepo wrapping them is evicted from repoCache — concretely,
// *gogitstore.WorktreeStorer (see gogitstore/storer.go's Close method).
// Declared here, in the consumer package, scoped to exactly the one method
// this file needs — see the `interface-pollution-checklist` skill
// (interfaces belong next to their consumer, not their implementer).
type gogitstoreCloser interface {
	Close() error
}

// releaseGogitstoreRef releases entry's claim on its gogitstore Registry
// entry (a SharedObjectStore reference count decrement), if entry.repo.Storer
// implements the Close hook. In production this is always true — entry.repo
// comes from gogitstore.Open via openRepoEntry — but must stay nil-safe:
// gogit_vcs_reader_memory_test.go's seedFakeCacheEntries populates repoCache
// directly with *cachedRepo values that have a nil repo, and this function
// is called from every repoCache eviction path (pruneRepoCache, ClearCache,
// and the discarded-duplicate-open branch of openRepoEntry), so it must
// tolerate those synthetic entries without panicking.
func releaseGogitstoreRef(entry *cachedRepo) {
	if entry == nil || entry.repo == nil {
		return
	}
	if c, ok := entry.repo.Storer.(gogitstoreCloser); ok {
		_ = c.Close()
	}
}

// pruneRepoCache evicts entries not accessed within repoCacheTTL, then trims
// the oldest entries if the cache still exceeds the current memory-derived
// budget (effectiveCacheMaxEntries) — NOT a flat constant, so the effective
// cap shrinks automatically under real process memory pressure. Called both
// reactively (openRepoEntry on overflow) and proactively (Scanner's periodic
// prune ticker), so it must stay cheap: a single Range scan plus a sort of
// the surviving entries. Every eviction here also releases the evicted
// entry's gogitstore SharedObjectStore reference (releaseGogitstoreRef) so
// that store becomes eligible for its own eviction by
// GoGitVCSReader.gogitstoreRegistry().Prune() once nothing in repoCache
// references it anymore.
func (g *GoGitVCSReader) pruneRepoCache() {
	cutoff := time.Now().Add(-repoCacheTTL).UnixNano()
	type liveEntry struct {
		key          string
		accessedAtNs int64
		entry        *cachedRepo
	}
	var live []liveEntry
	g.repoCache.Range(func(k, v any) bool {
		entry := v.(*cachedRepo)
		ts := atomic.LoadInt64(&entry.accessedAtNs)
		if ts < cutoff {
			g.repoCache.Delete(k)
			atomic.AddInt64(&g.repoCacheSize, -1)
			atomic.AddInt64(&g.repoCacheEvictions, 1)
			releaseGogitstoreRef(entry)
		} else {
			live = append(live, liveEntry{k.(string), ts, entry})
		}
		return true
	})

	// LRU trim: if still over the memory-derived budget after the TTL pass,
	// evict the coldest entries down to that budget.
	maxEntries := effectiveCacheMaxEntries()
	if int64(len(live)) > maxEntries {
		slices.SortFunc(live, func(a, b liveEntry) int {
			return cmp.Compare(a.accessedAtNs, b.accessedAtNs)
		})
		for _, e := range live[:int64(len(live))-maxEntries] {
			g.repoCache.Delete(e.key)
			atomic.AddInt64(&g.repoCacheSize, -1)
			atomic.AddInt64(&g.repoCacheEvictions, 1)
			releaseGogitstoreRef(e.entry)
		}
	}

	// matcherCache uses its own, longer TTL (see matcherCacheTTL's doc
	// comment) — deliberately not tied to repoCache's TTL/LRU pass above,
	// since decoupling those two lifetimes is the whole point of this cache.
	matcherCutoff := time.Now().Add(-matcherCacheTTL).UnixNano()
	g.matcherCache.Range(func(k, v any) bool {
		if mc, ok := v.(*matcherCacheEntry); ok && atomic.LoadInt64(&mc.accessedAtNs) < matcherCutoff {
			g.matcherCache.Delete(k)
		}
		return true
	})
}

// PruneToMemoryBudget runs a gentle, budget-respecting prune pass: evicts
// idle-past-TTL entries, then LRU-trims any remaining excess down to the
// current memory-derived budget. Intended to run proactively on a short
// ticker (Scanner does this every minute) rather than only reactively on
// overflow, so cache pressure never has a chance to build up between polls.
// Unlike ClearCache, entries that are still hot (recently accessed, within
// budget) are left alone — this is the normal-operation degradation path;
// ClearCache remains available as a rarely-used emergency escape valve.
func (g *GoGitVCSReader) PruneToMemoryBudget() {
	g.pruneRepoCache()
}

// ClearCache evicts ALL cached *git.Repository entries, allowing their
// internal go-git object LRU caches (~96 MB per repo, approxBytesPerCachedRepo)
// to be garbage-collected. Callers already holding a *cachedRepo reference
// (mid-scan) are unaffected — their reference keeps the object alive until
// the operation finishes. The next operation for an evicted repo re-opens it
// from disk (fast: only the pack index is read, not all objects).
//
// This is intentionally NOT called on a routine ticker — PruneToMemoryBudget
// is the gentle, budget-respecting path for normal operation. ClearCache is
// reserved as an emergency escape valve for severe memory pressure (see
// Scanner.Start's prune ticker), since it evicts hot repos along with cold
// ones and forces needless re-opens for anything still actively in use.
func (g *GoGitVCSReader) ClearCache() {
	g.repoCache.Range(func(k, v any) bool {
		g.repoCache.Delete(k)
		atomic.AddInt64(&g.repoCacheSize, -1)
		atomic.AddInt64(&g.repoCacheEvictions, 1)
		releaseGogitstoreRef(v.(*cachedRepo))
		return true
	})
}

// GoGitVCSReader implements VCSReader using the go-git library.
// No subprocesses are spawned; all operations run in-process.
// Prefer this in environments where spawning git subprocesses is undesirable
// or where index.lock contention is a concern.
//
// All fields are zero-value safe — GoGitVCSReader{} is valid without a constructor.
type GoGitVCSReader struct {
	// repoCache caches open go-git Repository handles keyed by absolute path.
	// Values are *cachedRepo; LoadOrStore ensures only one entry per path wins.
	// Entries are evicted after repoCacheTTL of inactivity (pruneRepoCache).
	repoCache sync.Map // map[string]*cachedRepo

	// repoCacheSize tracks the approximate entry count atomically so eviction
	// can be triggered without a full Range scan on every cache miss.
	repoCacheSize int64

	// repoCacheColdOpens/repoCacheEvictions back RepoCacheStats: instrumentation
	// added to root-cause the blob-cache's near-zero hit rate observed in
	// production (see /debug/blob-cache) before deciding whether to decouple
	// blobCache from cachedRepo's lifetime the way matcherCache already is.
	// blobCache lives inside *cachedRepo (gogit_vcs_reader.go's cachedRepo
	// struct), so every eviction here also discards that repo's blob cache —
	// if repoCacheEvictions tracks repoCacheColdOpens closely, evictions (not a
	// genuinely low same-blob-revisit workload) are the reason blobCache never
	// warms up, which would justify that decoupling; if evictions are rare
	// relative to opens, the low hit rate reflects the workload instead.
	repoCacheColdOpens int64
	repoCacheEvictions int64

	// diffStatCache caches DiffShortstat results keyed by absolute worktreePath.
	// Values are diffStatEntry (stored by value; no mutation after Store).
	// Races on a cache miss are benign: last writer wins; both compute the same value.
	diffStatCache sync.Map // map[string]diffStatEntry

	// aheadBehindCache caches AheadBehind results keyed by worktreePath+"\x00"+base.
	// Eliminates packfile-reader lock contention on repeated calls within TTL.
	aheadBehindCache sync.Map // map[string]aheadBehindEntry

	// commitMessagesCache caches CommitMessages results keyed by worktreePath+"\x00"+base.
	// Eliminates packfile-reader lock contention on repeated commit log walks.
	commitMessagesCache sync.Map // map[string]commitMessagesEntry

	// reachableSetCache caches reachableSet results, one entry per worktreePath,
	// validated by the entry's baseHash. Per-worktree scoping prevents a shallow
	// clone and its full-clone origin (same base hash, different reachable set)
	// from sharing one set. The set is an expensive O(N) walk, but immutable for
	// a given base hash, so there is no TTL; a moved base replaces the entry,
	// which also bounds the cache to one set per worktree.
	reachableSetCache sync.Map // map[string]reachableSetEntry

	// invalidationGen counts InvalidateWorktree calls per worktreePath
	// (*atomic.Uint64), so a computation that straddles an invalidation can
	// re-evict the pre-change result it just cached. See guardInvalidation.
	invalidationGen sync.Map

	// hasUncommittedCache caches HasUncommitted results keyed by worktreePath.
	// Eliminates repeated index walks within the TTL window.
	hasUncommittedCache sync.Map // map[string]hasUncommittedEntry

	// aheadBehindSF deduplicates concurrent AheadBehind calls for the same key.
	// On a cache miss, exactly one goroutine performs the BFS; others receive the result.
	aheadBehindSF singleflight.Group //nolint:exhaustruct
	// diffStatSF deduplicates concurrent DiffShortstat calls for the same worktree path.
	diffStatSF singleflight.Group //nolint:exhaustruct
	// hasUncommittedSF deduplicates concurrent HasUncommitted calls for the same path.
	hasUncommittedSF singleflight.Group //nolint:exhaustruct
	// commitMessagesSF deduplicates concurrent CommitMessages calls for the same key.
	commitMessagesSF singleflight.Group //nolint:exhaustruct

	// gogitstoreReg shares both the decoded-object cache AND the parsed
	// pack-index across every worktree of the same repo (keyed by resolved
	// common .git dir, not worktree path) — see
	// session/unfinished/gogitstore and session/unfinished/design/
	// pluggable-gitstore.md. Supersedes an earlier version of this field
	// that only shared the object cache; the pack-index parse was the
	// dominant cost per the original heap profile and is unexported inside
	// go-git's own filesystem.ObjectStorage, so gogitstore reimplements
	// storer.EncodedObjectStorer directly to reach it. Lazily initialized
	// via gogitstoreRegistry() so GoGitVCSReader{} stays zero-value safe.
	gogitstoreReg     *gogitstore.Registry
	gogitstoreRegOnce sync.Once

	// blobCacheHits/blobCacheMisses/blobCacheMissNanos back BlobCacheStats:
	// a hit/miss count and cumulative decompress+read time spent on misses,
	// letting callers judge whether blobCache is earning its keep (see
	// BlobCacheStats' doc comment). Deliberately plain atomics rather than a
	// metrics-library integration — this package has no existing metrics/
	// OTel-meter plumbing to hook into (telemetry/ only wires up tracing),
	// and a dashboard can be layered on top of these later if wanted.
	blobCacheHits      int64
	blobCacheMisses    int64
	blobCacheMissNanos int64

	// gitignoreMatcherInvalidations/gitignoreMatcherRebuilds back
	// GitignoreMatcherStats: PerfFix-6's instrumentation step, added to
	// measure how often HEAD moves force a gitignore matcher rebuild before
	// deciding whether a debounce is actually warranted (see that fix's
	// doc comment on entry.untrackedMatcherBuilt for the invalidation
	// trigger). Invalidations increment in resolveHeadTreeHashes whenever
	// HEAD changes; rebuilds increment in getOrBuildUntrackedMatcher
	// whenever the matcher is actually recompiled from disk. If rebuilds
	// tracks invalidations 1:1, every HEAD move is paying the full
	// ReadPatterns cost even when nothing untracked-related changed —
	// that's the signal a debounce would need to justify itself.
	gitignoreMatcherInvalidations int64
	gitignoreMatcherRebuilds      int64

	// matcherCache decouples the compiled gitignore matcher's lifetime from
	// cachedRepo's lifetime. pruneRepoCache evicts *cachedRepo entries purely
	// for memory pressure (TTL/LRU), independent of whether HEAD or
	// .gitignore actually moved; without this cache, every re-open after such
	// an eviction paid gitignore.ReadPatterns' full recursive directory walk
	// again even though the repo's ignore rules hadn't changed — confirmed as
	// 14.03% of all allocations app-wide. Keyed by worktreePath; validated
	// against the entry's headTreeHash so a real HEAD move (which can change
	// .gitignore) still forces a rebuild. Values are *matcherCacheEntry.
	matcherCache sync.Map

	// statFn, when non-nil, is called with the index-relative path before every
	// per-file lstat of the tracked-file stat passes (HasUncommitted and
	// DiffShortstat). Observer-only test seam for StatCount; nil in production.
	statFn func(rel string)
}

// matcherCacheEntry is the value type stored in GoGitVCSReader.matcherCache.
type matcherCacheEntry struct {
	matcher      gitignore.Matcher
	headTreeHash plumbing.Hash
	accessedAtNs int64
}

// matcherCacheTTL controls how long an unused matcherCache entry survives.
// Set well above repoCacheTTL: a compiled gitignore.Matcher is orders of
// magnitude cheaper to hold in memory than a full cachedRepo (the object this
// cache is deliberately decoupled from), so there is no memory-pressure
// reason to evict it as aggressively.
const matcherCacheTTL = 2 * time.Hour

// GitignoreMatcherStats reports how often the cached gitignore matcher (see
// getOrBuildUntrackedMatcher) is invalidated by a HEAD move versus actually
// rebuilt from disk. Instrumentation for PerfFix-6 — added to measure
// invalidation frequency before implementing any debounce/coalescing, per
// "no fix without root cause."
type GitignoreMatcherStats struct {
	Invalidations int64
	Rebuilds      int64
}

func (g *GoGitVCSReader) GitignoreMatcherStats() GitignoreMatcherStats {
	return GitignoreMatcherStats{
		Invalidations: atomic.LoadInt64(&g.gitignoreMatcherInvalidations),
		Rebuilds:      atomic.LoadInt64(&g.gitignoreMatcherRebuilds),
	}
}

// BlobCacheStats reports blobCache effectiveness across every repo this
// reader has touched: hit/miss counts and an estimated amount of wall-clock
// packfile decompression time avoided by cache hits (hits * the average
// observed miss duration). A low hit rate relative to misses suggests
// effectiveBlobCacheMaxBytes is sized too small for this workload (or that
// HEAD/blobs are churning too fast for caching to help at all); a high hit
// rate with a large EstimatedTimeSaved means the cache is earning its keep.
type BlobCacheStats struct {
	Hits               int64
	Misses             int64
	EstimatedTimeSaved time.Duration
}

func (g *GoGitVCSReader) BlobCacheStats() BlobCacheStats {
	hits := atomic.LoadInt64(&g.blobCacheHits)
	misses := atomic.LoadInt64(&g.blobCacheMisses)
	var avgMiss time.Duration
	if misses > 0 {
		avgMiss = time.Duration(atomic.LoadInt64(&g.blobCacheMissNanos) / misses)
	}
	return BlobCacheStats{
		Hits:               hits,
		Misses:             misses,
		EstimatedTimeSaved: avgMiss * time.Duration(hits),
	}
}

// currentReader holds the process's live GoGitVCSReader for debug
// introspection (see BlobCacheStatsSnapshot). There is normally exactly one
// per process — the scanner's own reader, constructed once in
// server/dependencies.go — registered here by NewScannerWithReader. This
// lets profiling.StartProfiling's debug HTTP server (which starts before
// the scanner exists — see main.go) reach it later without threading a
// reference through that early setup: the registered pointer is only
// dereferenced when a debug request actually arrives.
var currentReader atomic.Pointer[GoGitVCSReader]

// BlobCacheStatsSnapshot returns BlobCacheStats for the process's registered
// reader (see currentReader), or a zero value if none has been registered
// yet — e.g. queried before the scanner starts, or in tests that construct
// a *GoGitVCSReader directly without going through NewScanner/
// NewScannerWithReader.
func BlobCacheStatsSnapshot() BlobCacheStats {
	r := currentReader.Load()
	if r == nil {
		return BlobCacheStats{}
	}
	return r.BlobCacheStats()
}

// RepoCacheStats reports repoCache churn: how many *cachedRepo entries were
// ever opened cold (ColdOpens) versus evicted (Evictions, via pruneRepoCache's
// TTL/LRU passes or ClearCache). Since blobCache lives inside *cachedRepo
// (see that struct's doc comment), every eviction discards the evicted repo's
// blob cache along with it. Evictions tracking ColdOpens closely means most
// opens are actually re-opens of a previously-evicted path — the likely
// explanation for a low BlobCacheStats hit rate; Evictions staying small
// relative to ColdOpens means most opens are genuinely first-time, and the low
// hit rate instead reflects the workload rarely revisiting the same blob.
type RepoCacheStats struct {
	CurrentSize int64
	ColdOpens   int64
	Evictions   int64
}

func (g *GoGitVCSReader) RepoCacheStats() RepoCacheStats {
	return RepoCacheStats{
		CurrentSize: atomic.LoadInt64(&g.repoCacheSize),
		ColdOpens:   atomic.LoadInt64(&g.repoCacheColdOpens),
		Evictions:   atomic.LoadInt64(&g.repoCacheEvictions),
	}
}

// RepoCacheStatsSnapshot mirrors BlobCacheStatsSnapshot for RepoCacheStats.
func RepoCacheStatsSnapshot() RepoCacheStats {
	r := currentReader.Load()
	if r == nil {
		return RepoCacheStats{}
	}
	return r.RepoCacheStats()
}

// perRepoObjectCacheSize replaces go-git's PlainOpenWithOptions default of
// cache.DefaultMaxSize (96MB, plumbing/cache/common.go) with a much smaller
// budget. 96MB was sized for an interactive single-repo CLI tool holding one
// repo open for a whole session; this scanner holds many repos "hot"
// concurrently, so a smaller per-repo cache trades a bit more re-decoding on
// a cache miss for a much lower memory ceiling per resident repo.
const perRepoObjectCacheSize = 12 * cache.MiByte

// mmapIndexFeatureFlag is the feature-flag name gating gogitstore's
// mmap-backed .idx loader — see server/services/feature_flag_service.go's
// knownFeatureFlags and session/unfinished/design/mmap-activation-runbook.md.
const mmapIndexFeatureFlag = "unfinished:mmap-index"

// gogitstoreRegistry returns the shared gogitstore.Registry, creating it on
// first use. One Registry per GoGitVCSReader — a fresh Registry per call
// would defeat the whole point of cross-worktree sharing (see
// gogitstore.Registry's own doc comment). Does NOT set UseMmapIndex here —
// that is refreshed live on every openRepoEntry call instead (see
// syncMmapIndexFlag), so toggling the feature flag takes effect for the
// next repo opened without a process restart.
func (g *GoGitVCSReader) gogitstoreRegistry() *gogitstore.Registry {
	g.gogitstoreRegOnce.Do(func() {
		g.gogitstoreReg = &gogitstore.Registry{CacheMaxSize: int64(perRepoObjectCacheSize)}
	})
	return g.gogitstoreReg
}

// syncMmapIndexFlag reads the live, persisted state of mmapIndexFeatureFlag
// and applies it to reg via the mutex-guarded SetUseMmapIndex (never writes
// the field directly — see Registry.UseMmapIndex's doc comment on why a
// bare write would race acquire()'s read). config.LoadConfig() re-reads the
// config file from disk on every call (no in-memory caching), so this
// always reflects the latest value — including one just changed via the
// Settings UI's feature-flag toggle — at the cost of one small disk read
// per call. Called from openRepoEntry, which only runs on a repoCache
// miss (bounded by distinct-repo count and TTL eviction, not per
// git-operation), so that cost is negligible in practice.
func syncMmapIndexFlag(reg *gogitstore.Registry) {
	reg.SetUseMmapIndex(config.LoadConfig().GetFeatureFlag(mmapIndexFeatureFlag))
}

var _ VCSReader = (*GoGitVCSReader)(nil)

// sfDo calls sf.Do(key, fn) and catches any panic fn produces, returning it
// as an error. The result is type-asserted to T; callers must ensure fn only
// returns a non-nil value of type T when err is nil.
func sfDo[T any](sf *singleflight.Group, key string, fn func() (T, error)) (T, error) {
	val, err, _ := sf.Do(key, func() (v any, doErr error) {
		defer func() {
			if r := recover(); r != nil {
				doErr = fmt.Errorf("go-git panic: %v", r)
			}
		}()
		result, fnErr := fn()
		if fnErr != nil {
			return result, fnErr
		}
		return result, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return val.(T), nil
}

func (g *GoGitVCSReader) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	repo, err := g.openWorktree(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo %s: %w", repoPath, err)
	}

	// Main worktree.
	main := WorktreeInfo{Path: repoPath}
	if head, err := repo.Head(); err == nil {
		main.HEAD = head.Hash().String()
		if head.Name().IsBranch() {
			main.Branch = head.Name().Short()
		} else {
			main.IsDetached = true
		}
	}
	worktrees := []WorktreeInfo{main}

	// Linked worktrees live in $GIT_COMMON_DIR/worktrees/<name>/.
	// Use gitCommonDir to handle the case where repoPath is itself a linked worktree.
	worktreesDir := filepath.Join(gitCommonDir(repoPath), "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return worktrees, nil // no linked worktrees — not an error
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(worktreesDir, entry.Name())

		// gitdir file contains the absolute path to the worktree's .git file.
		// base is built from entry.Name(), enumerated by os.ReadDir(worktreesDir)
		// just above -- a trusted filesystem listing, not user input.
		gitdirData, err := os.ReadFile(filepath.Join(base, "gitdir")) // #nosec G304 -- base comes from a trusted os.ReadDir listing, not user input
		if err != nil {
			continue
		}
		// Strip the trailing "/.git" to get the worktree path.
		wtPath := filepath.Dir(strings.TrimSpace(string(gitdirData)))

		wt := WorktreeInfo{Path: wtPath}

		// Read HEAD: either "ref: refs/heads/<branch>" or a bare SHA.
		headData, err := os.ReadFile(filepath.Join(base, "HEAD")) // #nosec G304 -- base comes from a trusted os.ReadDir listing, not user input
		if err == nil {
			headStr := strings.TrimSpace(string(headData))
			const refPrefix = "ref: refs/heads/"
			if strings.HasPrefix(headStr, refPrefix) {
				wt.Branch = strings.TrimPrefix(headStr, refPrefix)
			} else {
				wt.IsDetached = true
				wt.HEAD = headStr
			}
		}

		if _, err := os.Stat(filepath.Join(base, "locked")); err == nil {
			wt.IsLocked = true
		}
		if _, err := os.Stat(filepath.Join(base, "gitdir")); err == nil {
			// Check prune flag.
			if _, err := os.Stat(wtPath); os.IsNotExist(err) {
				wt.IsPrunable = true
			}
		}

		worktrees = append(worktrees, wt)
	}
	return worktrees, nil
}

func (g *GoGitVCSReader) ResolveDefaultBranch(repoPath string) string {
	repo, err := g.openWorktree(repoPath)
	if err != nil {
		return ""
	}

	// Try refs/remotes/origin/HEAD first.
	if ref, err := repo.Reference("refs/remotes/origin/HEAD", true); err == nil {
		name := ref.Name().Short() // e.g. "origin/main"
		if name != "" {
			return name
		}
	}

	// Fall back to well-known remote tracking refs, then local.
	for _, candidate := range []string{
		"refs/remotes/origin/main", "refs/remotes/origin/master",
		"refs/remotes/origin/develop", "refs/remotes/origin/trunk",
		"refs/heads/main", "refs/heads/master",
		"refs/heads/develop", "refs/heads/trunk",
	} {
		if _, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			// Return the short name callers expect (e.g. "origin/main").
			short := plumbing.ReferenceName(candidate).Short()
			return short
		}
	}
	return ""
}

// gitignoreEntriesChanged reports whether any ".gitignore" blob (at any
// directory depth) was added, removed, or modified between old and new
// head-tree snapshots. Both maps come from resolveHeadTreeHashes's tree walk,
// so this is an O(tracked-file-count) in-memory map comparison, not a
// filesystem walk — still orders of magnitude cheaper than
// gitignore.ReadPatterns' recursive I/O.
func gitignoreEntriesChanged(old, newer map[string]plumbing.Hash) bool {
	isGitignore := func(name string) bool {
		return name == ".gitignore" || strings.HasSuffix(name, "/.gitignore")
	}
	for name, hash := range newer {
		if !isGitignore(name) {
			continue
		}
		if oldHash, ok := old[name]; !ok || oldHash != hash {
			return true
		}
	}
	for name := range old {
		if isGitignore(name) {
			if _, ok := newer[name]; !ok {
				return true // a .gitignore file was removed
			}
		}
	}
	return false
}

// HasUncommitted reports whether the worktree has any staged or unstaged changes.
//
// Strategy (no subprocess, low allocations):
//  1. Staged changes: compare index entry hashes against HEAD tree hashes — O(n)
//     hash comparisons, zero file I/O.
//  2. Working-tree changes: stat each tracked file and compare full-precision
//     mtime/size against the index record — O(n) stat calls, no file reads.
//
// This avoids the 1.85 GB allocation caused by wt.Status(), which hashes every
// modified file in full.

// AheadBehind returns the number of commits by which worktreePath's HEAD is
// ahead of and behind the given base ref.
//
// Strategy (no subprocess): find the merge base with a BFS over each side,
// then count commits between each tip and the merge base. This bounds the
// walk to the diverged portion of history rather than the full reachable set.
func (g *GoGitVCSReader) AheadBehind(worktreePath, base string) (int, int, error) {
	cacheKey := worktreePath + "\x00" + base
	if v, ok := g.aheadBehindCache.Load(cacheKey); ok {
		if e := v.(aheadBehindEntry); time.Now().Before(e.expiry) {
			return e.ahead, e.behind, nil
		}
	}

	type abResult struct{ ahead, behind int }
	// After the cache check: the guard allocates, and a hit must stay alloc-free.
	defer g.guardInvalidation(worktreePath)()
	result, err := sfDo(&g.aheadBehindSF, cacheKey, func() (abResult, error) {
		entry, openErr := g.openRepoEntry(worktreePath)
		if openErr != nil {
			return abResult{}, fmt.Errorf("open repo %s: %w", worktreePath, openErr)
		}

		entry.mu.Lock()
		defer entry.mu.Unlock() // REQUIRED: defer, never explicit unlocks — ensures mutex releases on panic

		repo := entry.repo
		headRef, headErr := repo.Head()
		if headErr != nil {
			return abResult{}, fmt.Errorf("head: %w", headErr)
		}
		baseHash, baseErr := resolveRef(repo, base)
		if baseErr != nil {
			return abResult{}, fmt.Errorf("resolve base %q: %w", base, baseErr)
		}
		if headRef.Hash() == baseHash {
			g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{headHash: headRef.Hash(), baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
			return abResult{0, 0}, nil
		}
		// A TTL-expired entry whose head/base hashes still match the current
		// ones means neither ref moved since the last computation — the BFS
		// result can't have changed, so reuse it instead of re-running
		// findMergeBase + two countCommitsTo walks (the actual expensive work;
		// resolving HEAD/base above is cheap by comparison).
		if v, ok := g.aheadBehindCache.Load(cacheKey); ok {
			if e := v.(aheadBehindEntry); e.headHash == headRef.Hash() && e.baseHash == baseHash {
				e.expiry = time.Now().Add(diffStatCacheTTL)
				g.aheadBehindCache.Store(cacheKey, e)
				return abResult{e.ahead, e.behind}, nil
			}
		}
		pm := newParentMemo(repo)
		mb, mbErr := findMergeBase(pm, headRef.Hash(), baseHash)
		if mbErr != nil {
			return abResult{}, fmt.Errorf("merge base: %w", mbErr)
		}
		ahead, aheadErr := countCommitsTo(pm, headRef.Hash(), mb)
		if aheadErr != nil {
			return abResult{}, aheadErr
		}
		behind, behindErr := countCommitsTo(pm, baseHash, mb)
		if behindErr != nil {
			return abResult{}, behindErr
		}
		g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{ahead: ahead, behind: behind, headHash: headRef.Hash(), baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
		return abResult{ahead, behind}, nil
	})
	if err != nil {
		return 0, 0, err
	}
	return result.ahead, result.behind, nil
}

func (g *GoGitVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	cacheKey := fmt.Sprintf("%s\x00%s\x00%d", worktreePath, base, max)
	if v, ok := g.commitMessagesCache.Load(cacheKey); ok {
		if e := v.(commitMessagesEntry); time.Now().Before(e.expiry) {
			return e.msgs, nil
		}
	}

	// After the cache check: the guard allocates, and a hit must stay alloc-free.
	defer g.guardInvalidation(worktreePath)()
	msgs, err := sfDo(&g.commitMessagesSF, cacheKey, func() ([]string, error) {
		entry, openErr := g.openRepoEntry(worktreePath)
		if openErr != nil {
			return nil, openErr
		}

		// Phase 1: snapshot HEAD and base as plain hash values under a brief lock.
		// These are cheap reference-store reads; holding the lock only here avoids
		// keeping it across the far more expensive graph walks below.
		var headHash plumbing.Hash
		var baseHash plumbing.Hash
		if err := func() error {
			entry.mu.Lock()
			defer entry.mu.Unlock()
			headRef, headErr := entry.repo.Head()
			if headErr != nil {
				return headErr
			}
			headHash = headRef.Hash() // plumbing.Hash is a plain [20]byte value type
			var baseErr error
			baseHash, baseErr = resolveRef(entry.repo, base)
			return baseErr
		}(); err != nil {
			return nil, err
		}

		// Phase 2: reachable set for base. cachedReachableSet acquires entry.mu
		// only on a cache miss, so repeated calls within the TTL window skip the
		// O(N) walk entirely and return the cached map without touching the mutex.
		baseReachable, reachErr := g.cachedReachableSet(entry, worktreePath, baseHash)
		if reachErr != nil {
			return nil, reachErr
		}

		// Phase 3: log walk from HEAD. go-git's packfile reader requires the lock.
		entry.mu.Lock()
		defer entry.mu.Unlock()
		iter, iterErr := entry.repo.Log(&git.LogOptions{From: headHash})
		if iterErr != nil {
			return nil, iterErr
		}
		defer iter.Close()

		var result []string
		walkErr := iter.ForEach(func(c *object.Commit) error {
			if baseReachable[c.Hash] {
				return storer.ErrStop
			}
			if len(result) < max {
				// Mimic `git log --oneline`: short hash + first line of message.
				result = append(result, c.Hash.String()[:7]+" "+firstLine(c.Message))
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
		g.commitMessagesCache.Store(cacheKey, commitMessagesEntry{msgs: result, expiry: time.Now().Add(diffStatCacheTTL)})
		return result, nil
	})
	return msgs, err
}

// cachedReachableSet returns the set of all commits reachable from baseHash,
// reusing the worktree's cached set while its base hash is unchanged.
// Known limit: a shallow clone deepened in place keeps its truncated set until
// the base moves or the process restarts (cosmetic, see maxReachableSetCommits).
// On a cache miss it acquires entry.mu, runs the walk, then releases the lock.
// The returned map must not be mutated by callers.
func (g *GoGitVCSReader) cachedReachableSet(entry *cachedRepo, worktreePath string, baseHash plumbing.Hash) (map[plumbing.Hash]bool, error) {
	if v, ok := g.reachableSetCache.Load(worktreePath); ok {
		if e := v.(reachableSetEntry); e.baseHash == baseHash {
			return e.set, nil
		}
	}

	entry.mu.Lock()
	set, err := reachableSet(entry.repo, baseHash)
	entry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	g.reachableSetCache.Store(worktreePath, reachableSetEntry{baseHash: baseHash, set: set})
	return set, nil
}

// InvalidateWorktree evicts worktreePath's TTL-cached results so the next call
// re-reads git state; Scanner calls it when a .git change event arrives.
// Leaves reachableSetCache alone: its entries are validated by base hash, and
// evicting on every commit would repeat the 50k-commit walk it exists to avoid.
func (g *GoGitVCSReader) InvalidateWorktree(worktreePath string) {
	g.pathInvalidationGen(worktreePath).Add(1)
	g.evictWorktreeResults(worktreePath)
}

func (g *GoGitVCSReader) evictWorktreeResults(worktreePath string) {
	g.diffStatCache.Delete(worktreePath)
	g.hasUncommittedCache.Delete(worktreePath)
	prefix := worktreePath + "\x00"
	for _, m := range []*sync.Map{&g.aheadBehindCache, &g.commitMessagesCache} {
		m.Range(func(k, _ any) bool {
			if key, _ := k.(string); strings.HasPrefix(key, prefix) {
				m.Delete(k)
			}
			return true
		})
	}
}

func (g *GoGitVCSReader) pathInvalidationGen(worktreePath string) *atomic.Uint64 {
	v, _ := g.invalidationGen.LoadOrStore(worktreePath, new(atomic.Uint64))
	return v.(*atomic.Uint64)
}

// guardInvalidation is deferred at the top of each cached public method:
// `defer g.guardInvalidation(path)()`. If InvalidateWorktree ran while the
// method was computing, the result it just cached predates the change, so
// evict it again and let the next call re-populate from the new state.
func (g *GoGitVCSReader) guardInvalidation(worktreePath string) func() {
	gen := g.pathInvalidationGen(worktreePath)
	start := gen.Load()
	return func() {
		if gen.Load() != start {
			g.evictWorktreeResults(worktreePath)
		}
	}
}

// ForgetWorktree is InvalidateWorktree plus dropping the worktree's reachable
// set, for a worktree that is gone (the set is MBs and nothing else evicts it).
func (g *GoGitVCSReader) ForgetWorktree(worktreePath string) {
	g.InvalidateWorktree(worktreePath)
	g.reachableSetCache.Delete(worktreePath)
	g.invalidationGen.Delete(worktreePath)
}

// worktreeGitDir returns the path to this worktree's own gitdir — the main
// .git directory, or a linked worktree's per-worktree gitdir under
// .git/worktrees/<name>. Unlike gitCommonDir, this is NOT resolved through
// commondir: files that are per-worktree rather than shared (notably the
// index) always live here, never in the shared commondir.
func worktreeGitDir(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	// repoPath is the local repo/worktree directory this server itself
	// manages for the session (Instance.Path), validated to exist at session
	// creation -- not raw untrusted network/RPC input.
	data, err := os.ReadFile(gitPath) // #nosec G304 -- repoPath is the session's own worktree directory, not user-supplied network input
	if err != nil {
		// .git is a directory (or missing).
		return gitPath
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return gitPath
	}
	wtGitDir := strings.TrimPrefix(line, prefix)
	if !filepath.IsAbs(wtGitDir) {
		// Defensive: git worktree add always writes an absolute path in
		// practice, but resolve relative to repoPath if it ever isn't,
		// matching how the commondir line below already handles this.
		wtGitDir = filepath.Join(repoPath, wtGitDir)
	}
	return filepath.Clean(wtGitDir)
}

// gitCommonDir returns the path to the common git directory (the main .git dir),
// resolving through the .git file in linked worktrees.
func gitCommonDir(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	// repoPath is the session's own worktree directory (see worktreeGitDir above).
	data, err := os.ReadFile(gitPath) // #nosec G304 -- repoPath is the session's own worktree directory, not user-supplied network input
	if err != nil {
		// .git is a directory (or missing).
		return gitPath
	}
	// .git is a file: "gitdir: /abs/path/to/.git/worktrees/<name>\n"
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return gitPath
	}
	wtGitDir := strings.TrimPrefix(line, prefix)
	// wtGitDir is parsed from gitPath above, which is itself rooted at the
	// trusted repoPath -- not user input.
	if cdData, err := os.ReadFile(filepath.Join(wtGitDir, "commondir")); err == nil { // #nosec G304 -- wtGitDir derives from repoPath's own .git file, not user input
		commondir := strings.TrimSpace(string(cdData))
		if !filepath.IsAbs(commondir) {
			commondir = filepath.Join(wtGitDir, commondir)
		}
		return commondir
	}
	return filepath.Dir(wtGitDir)
}

// openRepoEntry returns the cached *cachedRepo for path, opening it if needed.
// Uses LoadOrStore so that only one *cachedRepo is ever stored per path even
// when multiple goroutines race on the first access.
// Access timestamps are updated atomically on every hit so pruneRepoCache can
// evict cold entries without interrupting concurrent readers.
func (g *GoGitVCSReader) openRepoEntry(path string) (*cachedRepo, error) {
	now := time.Now().UnixNano()
	if v, ok := g.repoCache.Load(path); ok {
		entry := v.(*cachedRepo)
		atomic.StoreInt64(&entry.accessedAtNs, now)
		return entry, nil
	}

	// Trigger eviction before adding a new entry if the cache is at or past
	// the current memory-derived budget (not the flat repoCacheMaxEntries
	// ceiling — effectiveCacheMaxEntries() is normally far smaller and
	// shrinks further under real memory pressure).
	if atomic.LoadInt64(&g.repoCacheSize) >= effectiveCacheMaxEntries() {
		g.pruneRepoCache()
		// Re-check after eviction — another goroutine may have stored this path.
		if v, ok := g.repoCache.Load(path); ok {
			entry := v.(*cachedRepo)
			atomic.StoreInt64(&entry.accessedAtNs, now)
			return entry, nil
		}
	}

	// Open via gogitstore instead of PlainOpenWithOptions: shares both the
	// decoded-object cache and the parsed pack-index across every worktree
	// of the same repo (keyed by resolved common .git dir), instead of
	// PlainOpenWithOptions's per-worktree 96MB cache plus a fresh index
	// parse on every open. All downstream logic below is unchanged — it
	// operates on the returned *git.Repository exactly as it did with
	// PlainOpenWithOptions.
	reg := g.gogitstoreRegistry()
	syncMmapIndexFlag(reg)
	repo, err := gogitstore.Open(path, reg)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	entry := &cachedRepo{repo: repo, accessedAtNs: now}
	actual, loaded := g.repoCache.LoadOrStore(path, entry)
	if !loaded {
		atomic.AddInt64(&g.repoCacheSize, 1)
		atomic.AddInt64(&g.repoCacheColdOpens, 1)
	} else {
		// Another goroutine won the race to store the canonical entry for
		// this path first. The repo we just opened here — and its
		// WorktreeStorer's claim on the shared gogitstore Registry entry —
		// is discarded; release that reference immediately so it doesn't
		// keep a SharedObjectStore's refcount artificially nonzero forever
		// (an orphaned reference that would block Prune from ever evicting
		// it, even though nothing in repoCache actually points at this
		// particular *cachedRepo).
		releaseGogitstoreRef(entry)
	}
	return actual.(*cachedRepo), nil
}

// openWorktree opens a git repo that may be a linked worktree (has a .git file
// rather than a .git directory). It returns the cached *git.Repository.
// Callers that perform heavy VCS work should use openRepoEntry directly so
// they can hold the per-repo mutex for the duration of their operation.
func (g *GoGitVCSReader) openWorktree(path string) (*git.Repository, error) {
	entry, err := g.openRepoEntry(path)
	if err != nil {
		return nil, err
	}
	return entry.repo, nil
}

// resolveRef resolves a short ref name (e.g. "origin/main") to a commit hash.
func resolveRef(repo *git.Repository, name string) (plumbing.Hash, error) {
	// Try as a full or short reference name.
	for _, candidate := range []string{
		name,
		"refs/remotes/" + name,
		"refs/heads/" + name,
	} {
		if ref, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			return ref.Hash(), nil
		}
	}
	// Try as a literal hash.
	h := plumbing.NewHash(name)
	if !h.IsZero() {
		return h, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("cannot resolve ref %q", name)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return strings.TrimSpace(s)
}
