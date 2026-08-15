package unfinished

import (
	"bytes"
	"cmp"
	"container/list"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
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

// trackedFile holds the index metadata for a single tracked file needed by the
// OS stat phase of HasUncommitted.
type trackedFile struct {
	name       string
	size       uint32
	modifiedAt time.Time
}

type aheadBehindEntry struct {
	ahead  int
	behind int
	expiry time.Time
}

type commitMessagesEntry struct {
	msgs   []string
	expiry time.Time
}

type reachableSetEntry struct {
	set    map[plumbing.Hash]bool
	expiry time.Time
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
// own. 1.5 GB covers roughly 16 simultaneously "hot" repos at the ~96 MB
// estimate, which comfortably covers realistic concurrent-viewing workloads.
const repoCacheMemoryBudgetBytes = 1536 * 1024 * 1024

// highMemoryPressureThreshold/severeMemoryPressureThreshold gate
// effectiveCacheBudgetBytes' tiered response. Measured against the Go
// runtime's own HeapInuse (this process's heap, not host-wide memory) so the
// signal is specific to this process's contribution to memory pressure.
const (
	highMemoryPressureThreshold   = 3 * 1024 * 1024 * 1024 // 3 GB heap in-use: halve the budget
	severeMemoryPressureThreshold = 6 * 1024 * 1024 * 1024 // 6 GB heap in-use: floor to a handful of repos
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
	// Rebuilt only when HEAD moves (resolveHeadTreeHashes clears
	// untrackedMatcherBuilt on a HEAD change) — the same invalidation signal
	// used for headTreeCache/blobCache above. This means a .gitignore edit
	// made between HEAD moves can be stale for up to one poll cycle, an
	// acceptable trade-off matching this file's other TTL-based staleness
	// tolerances (e.g. diffStatCacheTTL). Guarded by mu, same as the fields above.
	untrackedMatcherBuilt bool
	untrackedMatcher      gitignore.Matcher
}

// gogitstoreCloser is implemented by storage.Storer values that hold a
// reference into a gogitstore.Registry and must release it when the
// cachedRepo wrapping them is evicted from repoCache — concretely,
// *gogitstore.WorktreeStorer (see gogitstore/storer.go's Close method).
// Declared here, in the consumer package, scoped to exactly the one method
// this file needs — see .claude/rules/interface-pollution-checklist.md
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

	// reachableSetCache caches reachableSet results keyed by
	// worktreePath+"\x00"+baseHash.String(), matching the pattern used by
	// aheadBehindCache/commitMessagesCache — scoping the cache per-worktree
	// prevents a hash collision across two different repos/worktrees that
	// happen to share a base commit (e.g. a shallow clone and its full-clone
	// origin) from returning one worktree's reachable set to the other.
	// The reachable set for a given base ref is expensive (O(N) commit walk) and
	// changes rarely; a 30s TTL matches diffStatCacheTTL and eliminates the
	// mutex-held walk that was the #1 hotspot (47.4B cycles, 38 events).
	reachableSetCache sync.Map // map[string]reachableSetEntry

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
		gitdirData, err := os.ReadFile(filepath.Join(base, "gitdir"))
		if err != nil {
			continue
		}
		// Strip the trailing "/.git" to get the worktree path.
		wtPath := filepath.Dir(strings.TrimSpace(string(gitdirData)))

		wt := WorktreeInfo{Path: wtPath}

		// Read HEAD: either "ref: refs/heads/<branch>" or a bare SHA.
		headData, err := os.ReadFile(filepath.Join(base, "HEAD"))
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

// resolveHeadTreeHashes returns the name->blob-hash map for repo's current HEAD
// tree, using entry.headTreeCache when it's still valid for the current HEAD.
// Both hasUncommittedGoGitPhase and diffShortstatUncached need the identical
// map for the same HEAD commit, and HEAD moves far less often than either is
// called, so the two callers share this cache instead of each walking the
// HEAD tree on every invocation.
//
// Callers must already hold entry.mu. Returns (nil, nil) if there is no HEAD
// yet (unborn branch / empty repo) — callers should treat that the same as
// an empty tree, not as an error.
func resolveHeadTreeHashes(g *GoGitVCSReader, entry *cachedRepo, repo *git.Repository) (map[string]plumbing.Hash, error) {
	headRef, headErr := repo.Head()
	if headErr != nil {
		if errors.Is(headErr, plumbing.ErrReferenceNotFound) {
			return nil, nil //nolint:nilnil // documented sentinel: no HEAD yet (unborn branch) is a valid, non-error "empty tree" result — see the doc comment above.
		}
		return nil, fmt.Errorf("head: %w", headErr)
	}
	headHash := headRef.Hash()
	if entry.headTreeHash == headHash && entry.headTreeCache != nil {
		return entry.headTreeCache, nil
	}
	headCommit, cerr := repo.CommitObject(headHash)
	if cerr != nil {
		return nil, fmt.Errorf("head commit: %w", cerr)
	}
	headTree, terr := headCommit.Tree()
	if terr != nil {
		return nil, fmt.Errorf("head tree: %w", terr)
	}

	// Walk the tree without loading blob content — only tree-entry hashes are needed.
	// object.FileIter.Next() calls GetBlob() per entry (loads full content); TreeWalker
	// loads only tree objects, which are orders of magnitude smaller.
	headHashes := make(map[string]plumbing.Hash, len(headTree.Entries))
	tw := object.NewTreeWalker(headTree, true, nil)
	defer tw.Close()
	for {
		name, te, twErr := tw.Next()
		if errors.Is(twErr, io.EOF) {
			break
		}
		if twErr != nil {
			return nil, fmt.Errorf("walk head tree: %w", twErr)
		}
		if te.Mode == filemode.Dir {
			continue
		}
		headHashes[name] = te.Hash
	}
	if entry.headTreeHash != headHash {
		// HEAD moved (or this is the first population): the name->hash
		// mapping above is stale, but blobCache is keyed by content hash, not
		// by name/HEAD, so it stays valid and is intentionally left alone
		// (see the blobCache field comment — PerfFix-1).
		entry.untrackedMatcherBuilt = false // F6: force a gitignore matcher rebuild too.
		if entry.headTreeCache != nil {
			// Only count an invalidation once the matcher has actually been
			// built at least once (headTreeCache != nil implies a prior
			// populate) — the first-ever build below isn't "invalidated by
			// a HEAD move," it's just startup.
			atomic.AddInt64(&g.gitignoreMatcherInvalidations, 1)
		}
	}
	entry.headTreeHash = headHash
	entry.headTreeCache = headHashes
	return headHashes, nil
}

// getOrBuildUntrackedMatcher returns the cached gitignore matcher for entry,
// rebuilding it if resolveHeadTreeHashes invalidated it since the last build.
// MUST be called with entry.mu already held (mirrors the caching contract of
// entry.headTreeCache above — both are entry-scoped fields protected by entry.mu).
func (g *GoGitVCSReader) getOrBuildUntrackedMatcher(entry *cachedRepo, worktreePath string) gitignore.Matcher {
	if entry.untrackedMatcherBuilt {
		return entry.untrackedMatcher
	}

	// Check the decoupled cache before paying for a rebuild: entry may be a
	// freshly (re)opened cachedRepo whose predecessor was evicted from
	// repoCache purely for memory pressure, with HEAD never having moved.
	if cached, ok := g.matcherCache.Load(worktreePath); ok {
		mc, _ := cached.(*matcherCacheEntry)
		if mc != nil && mc.headTreeHash == entry.headTreeHash {
			atomic.StoreInt64(&mc.accessedAtNs, time.Now().UnixNano())
			entry.untrackedMatcher = mc.matcher
			entry.untrackedMatcherBuilt = true
			return entry.untrackedMatcher
		}
	}

	atomic.AddInt64(&g.gitignoreMatcherRebuilds, 1)
	var m gitignore.Matcher
	if patterns, ignErr := gitignore.ReadPatterns(osfs.New(worktreePath), nil); ignErr == nil {
		m = gitignore.NewMatcher(patterns)
	}
	entry.untrackedMatcher = m
	entry.untrackedMatcherBuilt = true
	g.matcherCache.Store(worktreePath, &matcherCacheEntry{
		matcher:      m,
		headTreeHash: entry.headTreeHash,
		accessedAtNs: time.Now().UnixNano(),
	})
	return m
}

// hasUncommittedGoGitPhase runs the go-git index phase of HasUncommitted.
// Acquires and releases entry.mu via defer. Returns tracked file slice + dirty flag.
// MUST NOT be called with entry.mu already held — Go mutexes are not reentrant.
//
// Returns:
//   - tracked: slice of index entries needed for the OS stat phase (lock released before caller uses these)
//   - dirty: true if dirty was determined from go-git alone
//   - dirtyKnown: true if dirty result is definitive (caller should skip OS phase)
//   - matcher: cached gitignore matcher, built under entry.mu here since Phase 3
//     (the untracked-files walk) runs without the lock held
//   - err: any error encountered
func (g *GoGitVCSReader) hasUncommittedGoGitPhase(entry *cachedRepo, worktreePath string) (tracked []trackedFile, dirty bool, dirtyKnown bool, matcher gitignore.Matcher, err error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	repo := entry.repo

	idx, idxErr := repo.Storer.Index()
	if idxErr != nil {
		return nil, false, false, nil, fmt.Errorf("read index: %w", idxErr)
	}

	// --- staged changes: index vs HEAD (go-git, needs lock) ---
	headHashes, headErr := resolveHeadTreeHashes(g, entry, repo)
	if headErr != nil {
		return nil, false, false, nil, headErr
	}
	matcher = g.getOrBuildUntrackedMatcher(entry, worktreePath)
	if headHashes != nil { // nil means no HEAD yet (unborn branch) — nothing staged to compare
		indexNames := make(map[string]bool, len(idx.Entries))
		for _, idxEntry := range idx.Entries {
			if idxEntry.Stage != 0 { // merge conflict stage → dirty
				return nil, true, true, matcher, nil
			}
			indexNames[idxEntry.Name] = true
			if h, ok := headHashes[idxEntry.Name]; !ok || h != idxEntry.Hash {
				return nil, true, true, matcher, nil // new or modified staged file
			}
		}
		for name := range headHashes {
			if !indexNames[name] {
				return nil, true, true, matcher, nil // staged deletion
			}
		}
	}

	// Capture index entries needed for the OS phase as plain value types.
	// entry.mu is released by defer when this function returns — OS stat work
	// must happen in the caller after this function returns.
	result := make([]trackedFile, len(idx.Entries))
	for i, idxEntry := range idx.Entries {
		result[i] = trackedFile{idxEntry.Name, idxEntry.Size, idxEntry.ModifiedAt}
	}
	return result, false, false, matcher, nil
}

// HasUncommitted reports whether the worktree has any staged or unstaged changes.
//
// Strategy (no subprocess, low allocations):
//  1. Staged changes: compare index entry hashes against HEAD tree hashes — O(n)
//     hash comparisons, zero file I/O.
//  2. Working-tree changes: stat each tracked file and compare mtime/size against
//     the index record — O(n) stat calls, no file reads.
//
// This avoids the 1.85 GB allocation caused by wt.Status(), which hashes every
// modified file in full.

func (g *GoGitVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	if v, ok := g.hasUncommittedCache.Load(worktreePath); ok {
		if e := v.(hasUncommittedEntry); time.Now().Before(e.expiry) {
			return e.result, nil
		}
	}

	result, sfErr := sfDo(&g.hasUncommittedSF, worktreePath, func() (bool, error) {
		entry, err := g.openRepoEntry(worktreePath)
		if err != nil {
			return false, fmt.Errorf("open repo %s: %w", worktreePath, err)
		}

		// Phase 1: go-git index phase — entry.mu is held only inside this call.
		// The gitignore matcher is also built/cached here (under entry.mu) so
		// Phase 3 below can consume it without needing the lock itself.
		tracked, dirty, dirtyKnown, matcher, err := g.hasUncommittedGoGitPhase(entry, worktreePath)
		if err != nil {
			return false, err
		}
		if dirtyKnown {
			g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
				result: dirty, expiry: time.Now().Add(diffStatCacheTTL),
			})
			return dirty, nil
		}

		// Phase 2: OS stat walk — entry.mu is NOT held here (released by hasUncommittedGoGitPhase).
		indexedMap := make(map[string]struct{}, len(tracked))
		for _, tf := range tracked {
			indexedMap[tf.name] = struct{}{}
			info, serr := os.Lstat(filepath.Join(worktreePath, tf.name))
			if serr != nil {
				if os.IsNotExist(serr) {
					r := true // tracked file deleted
					g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
						result: r, expiry: time.Now().Add(diffStatCacheTTL),
					})
					return r, nil
				}
				continue
			}
			if info.Size() != int64(tf.size) ||
				!info.ModTime().Truncate(time.Second).Equal(tf.modifiedAt.Truncate(time.Second)) {
				r := true
				g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
					result: r, expiry: time.Now().Add(diffStatCacheTTL),
				})
				return r, nil
			}
		}

		// Phase 3: untracked files walk — no lock held. matcher was built under
		// entry.mu in Phase 1 above and is safe to read here (a plain local copy).
		r, err := hasUntrackedFiles(worktreePath, indexedMap, matcher)
		if err != nil {
			return false, err
		}
		g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
			result: r, expiry: time.Now().Add(diffStatCacheTTL),
		})
		return r, nil
	})
	if sfErr != nil {
		return false, sfErr
	}
	return result, nil
}

// hasUntrackedFiles reports whether any file under root is absent from the indexed set.
// It skips the .git directory and, when matcher is non-nil, skips gitignored files and
// whole subtrees (matcher is built from the worktree's .gitignore files by
// getOrBuildUntrackedMatcher — pass nil to disable gitignore filtering).
// For the mtime-stat approach this is a best-effort check sufficient for typical use.
func hasUntrackedFiles(root string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	return hasUntrackedFilesRec(root, root, indexed, matcher)
}

func hasUntrackedFilesRec(root, dir string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if matcher != nil && matcher.Match(strings.Split(rel, "/"), de.IsDir()) {
			continue // gitignored — for a dir this skips the whole subtree
		}
		if de.IsDir() {
			found, err := hasUntrackedFilesRec(root, full, indexed, matcher)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		} else if _, tracked := indexed[rel]; !tracked {
			return true, nil // untracked file
		}
	}
	return false, nil
}

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
			g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{expiry: time.Now().Add(diffStatCacheTTL)})
			return abResult{0, 0}, nil
		}
		mb, mbErr := findMergeBase(repo, headRef.Hash(), baseHash)
		if mbErr != nil {
			return abResult{}, fmt.Errorf("merge base: %w", mbErr)
		}
		ahead, aheadErr := countCommitsTo(repo, headRef.Hash(), mb)
		if aheadErr != nil {
			return abResult{}, aheadErr
		}
		behind, behindErr := countCommitsTo(repo, baseHash, mb)
		if behindErr != nil {
			return abResult{}, behindErr
		}
		g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{ahead: ahead, behind: behind, expiry: time.Now().Add(diffStatCacheTTL)})
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
// using reachableSetCache to avoid repeating the O(N) walk within the TTL window.
// The cache key is worktreePath+"\x00"+baseHash.String() (matching
// aheadBehindCache/commitMessagesCache) rather than baseHash alone, so two
// different worktrees that happen to share a base commit hash (e.g. a shallow
// clone and its full-clone origin) never share one another's reachable set.
// On a cache miss it acquires entry.mu, runs the walk, then releases the lock.
// The returned map must not be mutated by callers.
func (g *GoGitVCSReader) cachedReachableSet(entry *cachedRepo, worktreePath string, baseHash plumbing.Hash) (map[plumbing.Hash]bool, error) {
	cacheKey := worktreePath + "\x00" + baseHash.String()
	if v, ok := g.reachableSetCache.Load(cacheKey); ok {
		if e := v.(reachableSetEntry); time.Now().Before(e.expiry) {
			return e.set, nil
		}
	}

	entry.mu.Lock()
	set, err := reachableSet(entry.repo, baseHash)
	entry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	g.reachableSetCache.Store(cacheKey, reachableSetEntry{set: set, expiry: time.Now().Add(diffStatCacheTTL)})
	return set, nil
}

// DiffShortstat returns changed-file and line counts for the given worktree.
// Results are cached for diffStatCacheTTL (30s) to avoid repeated wt.Status()
// calls from concurrent scanner workers, which was the top mutex hotspot (537M
// cycles, 13,941 events in profiling).
func (g *GoGitVCSReader) DiffShortstat(worktreePath string) (DiffStat, error) {
	if v, ok := g.diffStatCache.Load(worktreePath); ok {
		if e := v.(diffStatEntry); time.Now().Before(e.expiry) {
			return e.result, nil
		}
	}
	result, err := sfDo(&g.diffStatSF, worktreePath, func() (DiffStat, error) {
		// diffShortstatUncached acquires entry.mu internally — do NOT add an outer lock here.
		// It does not call DiffShortstat, so there is no recursive sfDo path for this key.
		stat, uncachedErr := g.diffShortstatUncached(worktreePath)
		if uncachedErr != nil {
			return DiffStat{}, uncachedErr
		}
		g.diffStatCache.Store(worktreePath, diffStatEntry{
			result: stat,
			expiry: time.Now().Add(diffStatCacheTTL),
		})
		return stat, nil
	})
	if err != nil {
		return DiffStat{}, err
	}
	return result, nil
}

// diffShortstatUncached computes a DiffStat for worktreePath using index metadata
// to detect changed files, then loads HEAD blobs only for those files.
//
// This replaces the previous wt.Status() approach, which loaded ALL blob content
// into Go heap (MemoryObject) to hash every tracked file — allocating ~30 GB/hour
// cumulatively across active repos. The new approach:
//
//  1. Identifies staged changes by comparing index hashes with HEAD tree hashes
//     (TreeWalker, no blobs loaded).
//  2. Identifies unstaged changes by comparing index mtime/size with disk stat
//     (O(n) lstat calls, zero blob reads for unchanged files).
//  3. Loads HEAD blobs only for the M actually-changed files (M << N typical).
//  4. Computes line diffs on []byte directly, avoiding string conversion overhead.
func (g *GoGitVCSReader) diffShortstatUncached(worktreePath string) (DiffStat, error) {
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return DiffStat{}, err
	}

	// ── Phase 1: go-git metadata reads under lock (no blob loading) ──────────
	entry.mu.Lock()
	repo := entry.repo

	idx, err := repo.Storer.Index()
	if err != nil {
		entry.mu.Unlock()
		return DiffStat{}, err
	}

	// Build HEAD tree hash map — reused across polls via entry.headTreeCache
	// (shared with hasUncommittedGoGitPhase) as long as HEAD hasn't moved,
	// since HEAD only changes on commit/checkout, far less often than the
	// poll cycle. resolveHeadTreeHashes returns (nil, nil) for an unborn
	// branch (no commits yet), which we treat as an empty HEAD tree below.
	//
	// NOTE: this now propagates HEAD-commit/HEAD-tree lookup errors as fatal,
	// whereas the pre-extraction code silently swallowed them and proceeded
	// with an empty headHashes map. Unifying to "always propagate" matches
	// hasUncommittedGoGitPhase's existing behavior and is intentional — no
	// test in this package or server/services relies on the old swallow
	// behavior (verified via `go test ./session/unfinished/... ./server/services/...`).
	headHashes, headErr := resolveHeadTreeHashes(g, entry, repo)
	if headErr != nil {
		entry.mu.Unlock()
		return DiffStat{}, headErr
	}
	if headHashes == nil {
		headHashes = make(map[string]plumbing.Hash, len(idx.Entries))
	}

	// F6: reuse the compiled gitignore matcher across polls; resolveHeadTreeHashes
	// clears untrackedMatcherBuilt whenever HEAD moves, so a stale matcher can
	// only persist for at most one HEAD move's worth of polls — an acceptable
	// trade-off for skipping a redundant .gitignore walk every 30s poll.
	untrackedMatcher := g.getOrBuildUntrackedMatcher(entry, worktreePath)

	// Classify index entries: staged-changed (index hash ≠ HEAD hash) vs stable.
	type indexMeta struct {
		name       string
		indexHash  plumbing.Hash
		headHash   plumbing.Hash // zero if not in HEAD
		size       uint32
		modifiedAt time.Time
		staged     bool
	}
	metas := make([]indexMeta, 0, len(idx.Entries))
	indexedNames := make(map[string]struct{}, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.Stage != 0 {
			continue
		}
		indexedNames[e.Name] = struct{}{}
		headHash := headHashes[e.Name]
		metas = append(metas, indexMeta{
			name:       e.Name,
			indexHash:  e.Hash,
			headHash:   headHash,
			size:       e.Size,
			modifiedAt: e.ModifiedAt,
			staged:     e.Hash != headHash,
		})
	}

	// Staged deletions: files in HEAD but absent from index.
	type stagedDel struct {
		name     string
		headHash plumbing.Hash
	}
	var stagedDels []stagedDel
	for name, headHash := range headHashes {
		if _, inIdx := indexedNames[name]; !inIdx {
			stagedDels = append(stagedDels, stagedDel{name, headHash})
		}
	}

	entry.mu.Unlock() // release before OS operations

	// indexMtime anchors the racy-git window check below: a file is only
	// "racy" (stat can't prove it's unchanged) if its own mtime is not clearly
	// before the index was last written — the same test git itself uses,
	// rather than re-hashing every stat-clean file on every single poll.
	var indexMtime time.Time
	if fi, statErr := os.Stat(filepath.Join(worktreeGitDir(worktreePath), "index")); statErr == nil {
		indexMtime = fi.ModTime()
	}

	// ── Phase 2: build change target list (no lock, no blob reads) ───────────
	type changeTarget struct {
		name      string
		headHash  plumbing.Hash // zero = new file not in HEAD
		isDeleted bool          // no working-tree file exists
	}

	stagedTargets := make([]changeTarget, 0, len(stagedDels))
	for _, m := range metas {
		if m.staged {
			stagedTargets = append(stagedTargets, changeTarget{m.name, m.headHash, false})
		}
	}
	for _, sd := range stagedDels {
		stagedTargets = append(stagedTargets, changeTarget{sd.name, sd.headHash, true})
	}

	unstagedTargets := make([]changeTarget, 0)
	for _, m := range metas {
		if m.staged {
			continue
		}
		wtPath := filepath.Join(worktreePath, m.name)
		info, serr := os.Lstat(wtPath)
		if serr != nil {
			if os.IsNotExist(serr) {
				unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, true})
			}
			continue
		}
		sizeDiffers := info.Size() != int64(m.size)
		mtimeDiffers := !info.ModTime().Truncate(time.Second).Equal(m.modifiedAt.Truncate(time.Second))
		if sizeDiffers || mtimeDiffers {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}

		// Racy-clean case: size matches the index entry AND the working-tree
		// mtime (truncated to second) equals the index entry's recorded mtime.
		// stat alone cannot prove the file is unchanged in the racy window —
		// a file rewritten with identical size within the same wall-clock
		// second as the index write looks clean by stat but may differ in
		// content (the classic "racy git" problem). Outside that window
		// (file mtime clearly predates the index write), stat alone is
		// sufficient proof of no change, exactly as real git treats it — so
		// only pay for a content hash when the file could actually be racy.
		if !fileNeedsContentCheck(info.ModTime(), indexMtime) {
			continue // stat-clean and outside the racy window: confirmed unchanged
		}
		if info.Size() > maxUntrackedFileSize {
			// Too large to hash within the cap; conservatively treat as changed
			// rather than reading it into memory, consistent with how large
			// modified tracked files are handled elsewhere (applyDiff skips the
			// read but still counts the file via readFileIfSmall returning false).
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}
		wtData, ok := readFileIfSmall(wtPath)
		if !ok {
			// Unreadable or raced past the cap; treat as changed conservatively.
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}
		if plumbing.ComputeHash(plumbing.BlobObject, wtData) != m.indexHash {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
		}
	}

	// ── Phase 3: batch-read all needed blobs under a single lock hold ────────
	// Collecting all hashes first and reading them in one critical section
	// eliminates the N lock-acquire/release cycles (one per changed file) that
	// was the #2 mutex hotspot: 9.87B cycles, 1641 events in pprof profiling.
	// Peak memory is bounded by the sum of changed-file blobs (M << N typical),
	// which is acceptable since blobs > maxUntrackedFileSize are still skipped.
	allTargets := append(stagedTargets, unstagedTargets...) //nolint:gocritic // intentional ephemeral append
	blobMap := make(map[plumbing.Hash][]byte, len(allTargets))
	func() {
		// Deferred unlock (rather than an explicit entry.mu.Unlock() at the
		// end of this block): a panic partway through the loop below would
		// otherwise leave entry.mu permanently held, deadlocking every future
		// operation on this repo entry.
		entry.mu.Lock()
		defer entry.mu.Unlock()
		for _, t := range allTargets {
			if t.headHash == (plumbing.Hash{}) {
				continue
			}
			if _, already := blobMap[t.headHash]; already {
				continue
			}
			// Blob content is keyed by its content-addressed hash, so a cache hit
			// is always correct — no invalidation needed, ever. This skips
			// re-decompressing the same HEAD blob from the packfile on every poll
			// for a file that stays modified across several consecutive polls.
			if cached, ok := entry.blobCache.get(t.headHash); ok {
				blobMap[t.headHash] = cached
				atomic.AddInt64(&g.blobCacheHits, 1)
				continue
			}
			missStart := time.Now()
			blob, berr := entry.repo.BlobObject(t.headHash)
			if berr != nil {
				continue
			}
			// Skip blobs larger than maxUntrackedFileSize to avoid reading large
			// binaries or auto-generated files (e.g. package-lock.json, JARs).
			if blob.Size > maxUntrackedFileSize {
				continue
			}
			r, rerr := blob.Reader()
			if rerr != nil {
				continue
			}
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			_ = r.Close()
			data := bytes.Clone(buf.Bytes())
			blobMap[t.headHash] = data
			atomic.AddInt64(&g.blobCacheMisses, 1)
			atomic.AddInt64(&g.blobCacheMissNanos, int64(time.Since(missStart)))
			entry.blobCache.put(t.headHash, data, g.effectiveBlobCacheMaxBytes())
		}
	}()

	var d DiffStat

	applyDiff := func(t changeTarget) {
		d.Files++
		headData := blobMap[t.headHash] // nil for zero hash or skipped blobs
		if t.isDeleted {
			d.Deletions += countBytesLines(headData)
			return
		}
		wtData, ok := readFileIfSmall(filepath.Join(worktreePath, t.name))
		if !ok {
			d.Deletions += countBytesLines(headData)
			return
		}
		// F7: surface when the LCS cap forces a "fully replaced" approximation
		// instead of an exact diff, so this is discoverable without reading code.
		if oldLines, newLines := countBytesLines(headData), countBytesLines(wtData); oldLines > maxLCSLines || newLines > maxLCSLines {
			log.DebugLog.Printf("[GoGitVCSReader] LCS cap hit for %s (old=%d new=%d lines, cap=%d): reporting a fully-replaced approximation instead of an exact line diff", t.name, oldLines, newLines, maxLCSLines)
		}
		ins, del := linesDiffBytes(headData, wtData)
		d.Insertions += ins
		d.Deletions += del
	}

	for _, t := range stagedTargets {
		applyDiff(t)
	}
	for _, t := range unstagedTargets {
		applyDiff(t)
	}

	// Untracked files: count lines as pure insertions (no HEAD version).
	// Limited to maxUntrackedFiles to avoid OOM on repos with large build-artifact trees.
	// Files over maxUntrackedFileSize are counted but not read for line counting.
	// gitignore-matched paths (node_modules/, build output, etc.) are skipped
	// entirely rather than walked and read — this was previously the single
	// largest allocator in the app (~163GB cum in one profiling session),
	// since ignored build trees were read just like any other untracked file.
	// untrackedMatcher was resolved in Phase 1 above (cached on entry, F6).
	_ = walkUntracked(worktreePath, indexedNames, untrackedMatcher, func(absPath string) {
		d.Files++
		if data, ok := readFileIfSmall(absPath); ok {
			d.Insertions += countBytesLines(data)
		}
	})

	return d, nil
}

// fileNeedsContentCheck reports whether a stat-clean file (size and mtime both
// matching the index entry) still requires a content-hash fallback to rule
// out the racy-git case. A file is only racy if its own mtime is not clearly
// before the index was last written — the same window real git uses — so a
// file whose mtime predates the index write is safe to trust from stat alone.
// An unknown indexMtime (zero value) is treated conservatively as racy.
func fileNeedsContentCheck(fileMtime, indexMtime time.Time) bool {
	if indexMtime.IsZero() {
		return true
	}
	return !fileMtime.Truncate(time.Second).Before(indexMtime.Truncate(time.Second))
}

// readFileIfSmall reads the file at path only if its size is ≤ maxUntrackedFileSize.
// Returns (data, true) on success; (nil, false) when the file is too large or unreadable.
// This is the single safe wrapper for all content reads in diffShortstatUncached —
// callers should use this instead of os.ReadFile to prevent OOM on large build artifacts.
func readFileIfSmall(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.Size() > maxUntrackedFileSize {
		return nil, false
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, false
	}
	return data, true
}

// LinesDiff returns inserted and deleted line counts between old and new using LCS.
// Exported so tests can exercise the algorithm directly.
func LinesDiff(old, newContent string) (insertions, deletions int) {
	oldLines := splitLines(old)
	newLines := splitLines(newContent)
	lcs := lcsLength(oldLines, newLines)
	return len(newLines) - lcs, len(oldLines) - lcs
}

// maxLCSLines caps the input size to the O(n*m) LCS DP below. A file well
// under maxUntrackedFileSize in bytes can still contain hundreds of
// thousands of lines (e.g. mostly-blank or single-character lines), and
// n×m cells at that scale turns a shortstat poll into minutes of CPU on one
// goroutine. Beyond this cap, lcsLength/lcsLengthBytes return 0, which the
// existing len(new)-lcs / len(old)-lcs formulas turn into a "fully replaced"
// approximation — bounded and cheap instead of exact and unbounded.
// Cost at the cap: 20,000 × 20,000 = 4×10^8 DP-cell comparisons — on the
// order of a few seconds of CPU per file at this size, even though the
// rolling two-row implementation keeps memory at O(min(n,m)) (~160 KB of
// ints), not O(n*m); it's the CPU time, not memory, that this cap bounds.
// Declared as var (not const) so tests can lower it without generating huge fixtures.
var maxLCSLines = 20_000

// lcsLength computes the length of the longest common subsequence of two line slices.
// Uses O(n*m) DP — acceptable for typical source files, capped by maxLCSLines.
func lcsLength(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > maxLCSLines || len(b) > maxLCSLines {
		return 0
	}
	// Use two rows to keep memory O(min(n,m)).
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return prev[len(b)]
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Drop the empty string that results from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// countBytesLines counts the number of lines in data by counting '\n' bytes,
// treating a trailing newline as not creating an extra empty line.
func countBytesLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++ // last line without terminating newline
	}
	return n
}

// linesDiffBytes returns LCS-based insertion and deletion counts between old and new,
// operating directly on []byte to avoid string conversion overhead.
func linesDiffBytes(old, newData []byte) (insertions, deletions int) {
	a := splitLinesBytes(old)
	b := splitLinesBytes(newData)
	lcs := lcsLengthBytes(a, b)
	return len(b) - lcs, len(a) - lcs
}

// splitLinesBytes splits data into lines as subslices of data (no allocation per line).
func splitLinesBytes(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lcsLengthBytes computes the LCS length of two byte-slice-of-lines sequences.
// Uses O(min(n,m)) space rolling DP, same algorithm as lcsLength, capped by maxLCSLines.
func lcsLengthBytes(a, b [][]byte) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > maxLCSLines || len(b) > maxLCSLines {
		return 0
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if bytes.Equal(a[i-1], b[j-1]) {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return prev[len(b)]
}

// walkUntracked calls fn for every file under root that is not in indexed and
// not matched by matcher (pass nil to skip gitignore filtering entirely).
// Skips the .git directory. Stops after maxUntrackedFiles files to avoid
// unbounded walks in large build trees.
func walkUntracked(root string, indexed map[string]struct{}, matcher gitignore.Matcher, fn func(absPath string)) error {
	count := 0
	err := walkUntrackedRec(root, indexed, matcher, nil, &count, fn)
	if errors.Is(err, errStopWalk) {
		return nil
	}
	return err
}

// walkUntrackedRec recurses depth-first, threading the path components seen
// so far through `parts` instead of re-deriving and re-splitting a relative
// path string on every visited entry (the original approach allocated a new
// []string via strings.Split(filepath.Rel(...), "/") per file/dir, in this
// same hot walk the PR is optimizing). Appending to `parts` and letting
// sibling calls overwrite the same backing-array slot is safe here because
// the walk is single-goroutine and depth-first: a sibling's entire subtree
// (including every fn callback in it) completes before the next sibling's
// append can reuse that slot.
func walkUntrackedRec(dir string, indexed map[string]struct{}, matcher gitignore.Matcher, parts []string, count *int, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		childParts := append(parts, name)
		if matcher != nil && matcher.Match(childParts, de.IsDir()) {
			continue // gitignored — for a dir this skips the whole subtree
		}
		if de.IsDir() {
			if err := walkUntrackedRec(full, indexed, matcher, childParts, count, fn); err != nil {
				return err
			}
		} else {
			rel := strings.Join(childParts, "/")
			if _, tracked := indexed[rel]; !tracked {
				*count++
				if *count > maxUntrackedFiles {
					return errStopWalk
				}
				fn(full)
			}
		}
	}
	return nil
}

// worktreeGitDir returns the path to this worktree's own gitdir — the main
// .git directory, or a linked worktree's per-worktree gitdir under
// .git/worktrees/<name>. Unlike gitCommonDir, this is NOT resolved through
// commondir: files that are per-worktree rather than shared (notably the
// index) always live here, never in the shared commondir.
func worktreeGitDir(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	data, err := os.ReadFile(gitPath)
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
	data, err := os.ReadFile(gitPath)
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
	// Each per-worktree gitdir contains a "commondir" file pointing to the main .git.
	if cdData, err := os.ReadFile(filepath.Join(wtGitDir, "commondir")); err == nil {
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

// maxReachableSetCommits caps the number of commits reachableSet will walk —
// named to match this file's other max* caps (maxUntrackedFiles, maxLCSLines)
// rather than the previous reachableSetLimit. The same kind of bound
// findMergeBase already applies via mergeBaseBFSLimit.
// The only caller (CommitMessages, via cachedReachableSet) uses the result
// solely to detect "already merged into base" and stop printing — a partial
// set beyond this depth means a handful of already-merged commits could
// appear in the output on a repo with a base branch this deep, a cosmetic
// imperfection preferable to an unbounded walk over the full history.
// Cost at the cap: 50,000 plumbing.Hash keys (20 bytes each) plus Go map
// bucket overhead is a few MB and a few tens of milliseconds of commit-log
// walking — cheap and bounded, versus an unbounded walk that on a
// million-commit repo would cost proportionally more of both.
// Declared as var (not const) so tests can lower it without creating
// thousands of commits.
var maxReachableSetCommits = 50_000

// reachableSet returns the set of commits reachable from start, up to
// maxReachableSetCommits commits.
func reachableSet(repo *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool, 64)
	iter, err := repo.Log(&git.LogOptions{From: start})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if err := iter.ForEach(func(c *object.Commit) error {
		if len(seen) >= maxReachableSetCommits {
			return storer.ErrStop
		}
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return nil, err
	}
	return seen, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return strings.TrimSpace(s)
}

// mergeBaseBFSLimit caps the number of commits visited per side in findMergeBase.
// For a 10K-commit repo the unbounded walk allocates ~640 KB; bounding at 2000
// limits that to ~128 KB while still covering typical branch divergences.
const mergeBaseBFSLimit = 2000

// findMergeBase returns the most-recent common ancestor of h1 and h2 using BFS.
// It first marks ancestors of h1 (up to mergeBaseBFSLimit), then walks ancestors
// of h2 until it finds one that is already marked. If no merge base is found
// within the depth limit, a sentinel error is returned.
func findMergeBase(repo *git.Repository, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}

	// Mark ancestors of h1 (bounded).
	anc := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q := []plumbing.Hash{h1}
	visited := 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if anc[h] {
			continue
		}
		anc[h] = true
		visited++
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue // object missing from shallow clone or pack — skip
		}
		q = append(q, c.ParentHashes...)
	}

	// Walk from h2 breadth-first; first ancestor in anc is the nearest merge base.
	seen := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q = []plumbing.Hash{h2}
	visited = 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[0]
		q = q[1:]
		if seen[h] {
			continue
		}
		seen[h] = true
		visited++
		if anc[h] {
			return h, nil
		}
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue
		}
		q = append(q, c.ParentHashes...)
	}
	return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", mergeBaseBFSLimit)
}

// countCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive).
func countCommitsTo(repo *git.Repository, start, stop plumbing.Hash) (int, error) {
	seen := make(map[plumbing.Hash]bool, 32)
	q := []plumbing.Hash{start}
	n := 0
	for len(q) > 0 {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if seen[h] || h == stop {
			continue
		}
		seen[h] = true
		n++
		c, err := repo.CommitObject(h)
		if err != nil {
			return 0, err
		}
		for _, p := range c.ParentHashes {
			if !seen[p] && p != stop {
				q = append(q, p)
			}
		}
	}
	return n, nil
}
