# Pluggable, memory-efficient go-git storage backend

Status: **prototype / design draft** — not wired into production. Companion code lives in
`session/unfinished/gogitstore/` (new package, does not touch `gogit_vcs_reader.go`,
`scanner.go`, `watcher.go`, or `server/dependencies.go`).

## 1. Problem recap

`session/unfinished/gogit_vcs_reader.go` opens one `*git.Repository` per worktree via
`git.PlainOpenWithOptions` and caches it in `GoGitVCSReader.repoCache` (keyed by worktree
path). Two structurally different costs are paid **once per worktree**, even though every
worktree of one repository shares the same underlying object database on disk:

1. **Decoded-object LRU cache** — `cache.DefaultMaxSize` = 96MB
   (`plumbing/cache/common.go:14`), allocated fresh inside `filesystem.NewStorage(fs,
   cache.NewObjectLRUDefault())` on every `PlainOpenWithOptions` call
   (`repository.go:329`). A parallel, concurrently-running fix in this same file shrinks
   this to ~12MB and shares one `*cache.ObjectLRU` per commondir (keyed the same way this
   design keys `SharedObjectStore`) via `filesystem.NewStorage(fs, sharedCache)` +
   `git.Open(storer, worktreeFs)`.

2. **Parsed pack-index** — `storage/filesystem/object.go`'s `ObjectStorage.index
   map[plumbing.Hash]idxfile.Index`, built lazily by `requireIndex()`/`loadIdxFile()` on
   first packfile access, and **never bounded or evicted** for the life of that
   `ObjectStorage`. Per a live heap profile, `idxfile.decoder.go`'s `readObjectNames` and
   `MemoryIndex.genOffsetHash` were the top allocators — confirmed by reading
   `decoder.go` (§4 below): every non-empty fanout bucket gets its own `make([]byte,
   ...)` + `io.ReadFull` copy for `Names`, `Offset32`, and `CRC32`, so total heap bytes
   allocated for one parse are proportional to the full uncompressed `.idx` file size
   (roughly 28 bytes/object plus the 1024-byte fanout table). This field
   (`ObjectStorage.index`) and the method that builds it (`loadIdxFile`) are **unexported**
   — nothing outside the `filesystem` package can share, inspect, or bound it. This is
   the gap the cache-size fix cannot close, and what this design solves.

The two costs are independent: shrinking (1) does nothing for (2), and (2) was the
dominant cost in the profile that caused the OOM this work responds to.

## 2. What "shared" actually means in git's own on-disk layout

`storage/filesystem/dotgit/repository_filesystem.go`'s `RepositoryFilesystem` already
encodes the real split, at the raw-filesystem level, between what a linked worktree keeps
private and what it shares via `$GIT_COMMON_DIR`:

```go
// mapToRepositoryFsByPath — routes to commonDotGitFs for:
//   objects/, refs/, packed-refs, config, branches/, hooks/, info/, remotes/, logs/, shallow, worktrees/
// EXCEPT (routed back to the per-worktree dotGitFs even though they'd otherwise match):
//   logs/HEAD, refs/bisect, refs/rewritten, refs/worktree
// everything else (HEAD, index, ORIG_HEAD, ...) → dotGitFs (per-worktree)
```

This design applies the **same split**, one layer up, at the `storage.Storer` /
`storer.EncodedObjectStorer` level instead of the filesystem level: object storage
(the expensive part) is shared per-commondir; reference/index/shallow/config/module
storage stays genuinely per-worktree, because — unlike an object's content — a ref value,
`HEAD`, and the index **do** legitimately differ per worktree, and git's own on-disk
layout already agrees (`logs/HEAD`, `refs/worktree/*` are explicit per-worktree
exceptions even though most of `logs/` and `refs/` are shared).

## 3. Interfaces actually implemented

`git.Open(s storage.Storer, worktree billy.Filesystem)` requires (`storage/storer.go`):

```go
type Storer interface {
    storer.EncodedObjectStorer
    storer.ReferenceStorer
    storer.ShallowStorer
    storer.IndexStorer
    config.ConfigStorer
    ModuleStorer // storage.Storer's own Module(name) (Storer, error)
}
```

`storer.EncodedObjectStorer` (`plumbing/storer/object.go`), the one that matters for this
design, is exactly 7 methods:

```go
NewEncodedObject() plumbing.EncodedObject
SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
EncodedObject(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error)
IterEncodedObjects(plumbing.ObjectType) (EncodedObjectIter, error)
HasEncodedObject(plumbing.Hash) error
EncodedObjectSize(plumbing.Hash) (int64, error)
AddAlternate(remote string) error
```

`session/unfinished/gogitstore.WorktreeStorer` implements all 7 directly (`storer.go`),
delegating to a shared `SharedObjectStore` for everything except `NewEncodedObject`
(a bare allocation). Per this task's explicit scope, **read-only**: `SetEncodedObject`,
`IterEncodedObjects`, and `AddAlternate` return an explicit "not implemented" error rather
than silently degrading — none of `HasUncommitted`/`DiffShortstat`/`AheadBehind`
(`session/unfinished/vcsreader.go`) call them; a full walk (fsck/GC-style tooling) would
need `IterEncodedObjects` implemented for real, mirroring
`filesystem.ObjectStorage.IterEncodedObjects`'s loose-object-scan + per-pack-iterator
approach.

For `ReferenceStorer`/`ShallowStorer`/`IndexStorer`/`ConfigStorer`/`ModuleStorer`,
`WorktreeStorer` embeds a full `*filesystem.Storage` (built via
`filesystem.NewStorageWithOptions(repositoryFs, throwawayCache, Options{})`, where
`repositoryFs = dotgit.NewRepositoryFilesystem(dotFs, commonFs)` — the exact type from §2).
This is deliberate, not laziness: `filesystem.ReferenceStorage`/`IndexStorage`/
`ShallowStorage`/`ConfigStorage`/`ModuleStorage` all have an **unexported** `dir
*dotgit.DotGit` field and no exported constructor, so there is no way to build a
correctly-wired standalone instance of any of them from outside the `filesystem` package —
the only way to get one is to let `NewStorageWithOptions` build a whole `Storage` and keep
the parts wanted. `filesystem.Storage` also embeds its own `ObjectStorage`, which
independently satisfies `storer.EncodedObjectStorer` — but Go's method-promotion rules
resolve to the **shallowest** embedding depth, and `WorktreeStorer` declares all 7
`EncodedObjectStorer` methods directly (depth 0), so the embedded `ObjectStorage`'s
versions (depth 2) are always shadowed. Verified empirically, not just by reading the
spec: `gogitstore_test.go`'s `TestWorktreeStorer_UsesSharedStore` asserts the shared
store's `IndexBuildCount`/`IndexEntryCount` counters — the only place that can advance —
actually advance as a result of calling `WorktreeStorer` methods.

## 4. Architecture

```
Registry (one per process / one per "pool of repos this component cares about")
  └── map[commonDirAbs]*SharedObjectStore     (keyed by resolved, filepath.Clean'd commondir)
        SharedObjectStore  (§4.1 — the shared, expensive half)
          - dir  *dotgit.DotGit          (rooted at the COMMONDIR filesystem)
          - objectCache cache.Object     (shared ObjectLRU — already internally mutex-safe)
          - mu sync.Mutex + index map[plumbing.Hash]*lockedIndex   (§4.2 — the unsafe half, now guarded)

For each worktree:
  WorktreeStorer            (§3 — the per-worktree, cheap half)
    - *filesystem.Storage   (built from THIS worktree's dotFs+commonFs; gives correct
                              per-worktree Reference/Index/Shallow/Config/Module for free)
    - shared *SharedObjectStore   (pointer to the one entry for this repo's commondir)
```

### 4.1 SharedObjectStore — the shared half

Read-path methods (`EncodedObject`/`HasEncodedObject`/`EncodedObjectSize`) mirror
`filesystem/object.go`'s `getFromUnpacked`/`getFromPackfile`/`findObjectInPackfile`
logic, adapted to a commondir-rooted `dotgit.DotGit` instead of a per-worktree one.
One deliberate prototype simplification: `getFromPackfile` opens a **fresh**
`billy.File` handle to the pack file on every single call (via `s.dir.ObjectPack(pack)`)
instead of caching one open `*packfile.Packfile` per store. This is not an oversight —
see §5's discussion of why a packfile's `Scanner` (a seek cursor over one file handle)
cannot safely be shared across concurrent worktree reads, and why per-call opens
(cheap OS-level opens backed by the same shared page cache) sidestep that without needing
another lock. A production version should cache one open pack handle **per worktree per
pack** (mirroring `filesystem.Options.KeepDescriptors`/`MaxOpenDescriptors`) to cut
per-object-read syscall overhead — flagged as a stage-2 follow-up, not a correctness gap.

### 4.2 The concurrency-safety finding that drives the whole design

**`idxfile.MemoryIndex` is not safe for concurrent access even for pure lookups.**
`FindOffset` and `FindHash` (`plumbing/format/idxfile/idxfile.go:116,172`) lazily populate
an internal `offsetHash map[int64]plumbing.Hash` as a side effect of what looks like a
read:

```go
func (idx *MemoryIndex) FindOffset(h plumbing.Hash) (int64, error) {
    ...
    if !idx.offsetHashIsFull {
        if idx.offsetHash == nil {
            idx.offsetHash = make(map[int64]plumbing.Hash)
        }
        idx.offsetHash[int64(offset)] = h   // ← unguarded map write, on every "lookup"
    }
    return int64(offset), nil
}
```

This is exactly go-git issue #1121 ("concurrent map read and map write", triggered by
concurrent `repo.Log()`/`CommitObject` on one repo) — and it is exactly the situation
this design deliberately creates on purpose: one `MemoryIndex` per pack, shared by every
worktree of a repository. **This was not taken on faith** — `index.go`'s `lockedIndex`
wrapper serializes every `idxfile.Index` method behind one `sync.Mutex` per
`SharedObjectStore`, and `gogitstore_test.go`'s
`TestConcurrentReadsAcrossWorktrees_NoDataRace` proves both directions: with the lock in
place, `go test -race` is clean across N worktrees × M goroutines each walking full commit
history concurrently; with the lock in `lockedIndex.FindOffset` temporarily removed (a
throwaway experiment, reverted immediately after), the exact same test reliably reproduces
the crash —

```
fatal error: concurrent map read and map write
...idxfile.(*MemoryIndex).FindOffset(...)
    plumbing/format/idxfile/idxfile.go:134
...gogitstore.(*lockedIndex).FindOffset(...)
...gogitstore.(*SharedObjectStore).findObjectInPackfile(...)
```

confirming both that the bug is real and reachable through this exact code path, and that
the fix actually prevents it.

**Concurrency trade-off, stated plainly:** today's `cachedRepo.mu` in
`gogit_vcs_reader.go` is one mutex **per worktree**, so N worktrees of one repo can have
their packfile reads proceed in parallel across N scanner workers. `SharedObjectStore` uses
one mutex **per commondir** for all `idxfile.Index` calls, so index *lookups* (not the
actual decompression I/O, which stays per-call/per-handle — see §4.1) across every
worktree of one repository now serialize on that single mutex. Given how cheap one lookup
is (a fanout-table indexed binary search over an in-memory byte slice — microseconds), this
is very unlikely to be the new bottleneck for this scanner's workload (4 workers, 30s+
poll interval), but it should be watched in canary rollout, and §6 lists sharding the lock
per-pack as a documented, deferred follow-up if profiling says otherwise.

`Entries()`/`EntriesByOffset()` are locked only for the construction call, not for
draining the returned iterator — their `Next()` methods read immutable post-decode fields
(`Names`/`Fanout`/etc.) and never touch `offsetHash`, so holding the lock across the whole
iteration would be unnecessarily conservative.

### 4.3 A second, independent race — `*dotgit.DotGit` itself

The `-race` proof in §4.2 was written expecting to catch exactly one bug (`MemoryIndex`).
It caught a **second, unrelated one** on the very next full run: `storage/filesystem/
dotgit/dotgit.go`'s `DotGit` struct carries several lazily-populated caches with no
locking of their own —

```go
type DotGit struct {
    ...
    incomingChecked bool             // written by hasIncomingObjects(), called from Object()
    incomingDirName string
    objectList []plumbing.Hash       // written by genObjectList()
    objectMap  map[plumbing.Hash]struct{}
    packList   []plumbing.Hash       // written by genPackList()
    packMap    map[plumbing.Hash]struct{}
}
```

`hasIncomingObjects()` (`dotgit.go:590`) is a textbook unguarded check-then-set: `if
!d.incomingChecked { ...; d.incomingChecked = true }`, called from `Object()` on **every**
loose-object lookup. Because `SharedObjectStore` deliberately shares one `*dotgit.DotGit`
across every worktree (for the same reason it shares the parsed index — the commondir is
one on-disk object database), concurrent `EncodedObject`/`HasEncodedObject` calls from
different worktrees raced on this exact field, caught by `go test -race` as:

```
WARNING: DATA RACE
Write at 0x... by goroutine N:
  dotgit.(*DotGit).hasIncomingObjects()
      storage/filesystem/dotgit/dotgit.go:604
  dotgit.(*DotGit).Object()
      storage/filesystem/dotgit/dotgit.go:618
  gogitstore.(*SharedObjectStore).getFromUnpacked()
```

**Fix:** `store.go`'s `dirObject`/`dirObjectPack` wrap `s.dir.Object(h)` /
`s.dir.ObjectPack(pack)` in the same `SharedObjectStore.mu` used for the index — so that
mutex now guards two independent pieces of unsynchronized upstream state (the parsed
index, and `*dotgit.DotGit`'s lazy caches), not just one. The lock is held only around the
call that resolves a path and opens a `billy.File`; once a handle is returned, reading
from it never touches `d`'s shared state again, so this does not extend the lock over
packfile decompression. Re-run twice clean plus a 5x-repeated targeted run of
`TestConcurrentReadsAcrossWorktrees_NoDataRace`, all under `-race`, after the fix (see
`gogitstore_test.go`).

This finding matters beyond just "one more bug to fix": it means the concurrency-safety
problem this design has to solve is **not limited to `idxfile.MemoryIndex`** — it is a
general property of "any go-git internal type this design decides to share across
worktrees may have unguarded lazy state," discovered empirically rather than assumed.
Any future extension of `SharedObjectStore` (e.g. the mmap work in §5, or caching open
`*packfile.Packfile` handles per §4.1's stage-1 follow-up) needs the same scrutiny — audit
every field of every shared go-git type for unguarded lazy mutation before assuming a
"read" is actually read-only, rather than trusting the method name.

## 5. mmap for the index — designed, not built

The task's stretch goal: can `idxfile.Index` be satisfied by a value backed by an mmap'd
`.idx` file instead of `decoder.go`'s full-copy `io.ReadFull` calls? **Yes, and the shape
of the win is better than "implement a new type" — it doesn't require a new `idxfile.Index`
implementation at all.**

### 5.1 Why `idxfile.MemoryIndex` itself can be reused unmodified

Looking at `decoder.go`'s `readObjectNames`/`readCRC32`/`readOffsets`: on disk, the Names
block, the CRC32 block, and the Offset32 block are each **one contiguous run of bytes**
across the whole file (sorted, fanout-indexed). The decoder only slices them
bucket-by-bucket in memory because it needs per-bucket-sized `[]byte` values for
`MemoryIndex.Names[k]`/`Offset32[k]`/`CRC32[k]` — not because the on-disk bytes are
non-contiguous. That means a from-scratch loader can compute each bucket's `[start:end]`
byte range from the (tiny, 1024-byte) fanout table, then set
`idx.Names[k] = mapped[namesBase+start : namesBase+end]` as a **zero-copy subslice of the
mmap'd region**, instead of `make([]byte, n)` + `io.ReadFull`. Every field
`MemoryIndex.{Names,Offset32,CRC32,Offset64}` is already `[][]byte`/`[]byte` and already
exported — nothing about `MemoryIndex` itself needs to change; only the *loader* does.
`Fanout [256]uint32` and `FanoutMapping [256]int` stay small fixed-size decodes (1024
bytes total) since they're not worth mmap'ing.

### 5.2 Library choice

`golang.org/x/exp/mmap` exposes a `ReaderAt`-style API (`.At(i)`/`ReadAt`) — using it would
still require a `copy()` per subslice, defeating the point. `github.com/edsrzf/mmap-go`
defines `type MMap []byte` — a real Go byte slice backed by the mapped pages — which is
exactly what the zero-copy subslicing in §5.1 needs. This was not verified against a pinned
version in this sandbox (no network access here); confirm the exact API surface against
`go.sum`'s resolved version before implementing.

### 5.3 Lifetime safety — the actual hard part

**When to munmap:** tie the mapping's lifetime to its `SharedObjectStore` entry, not to Go
GC finalizers alone. A `Registry` eviction path (§6, stage 2 — not built in this prototype;
`Registry` currently never evicts, see `registry.go`'s doc comment) must call an explicit
`Close()`/`munmap()` when a store is evicted, because relying on a finalizer to eventually
run defeats the whole point of being *responsive* to memory pressure — the exact
requirement this OOM-response work exists for.

**What happens if a repack changes the underlying pack while mapped:** git's own repack/gc
never mutates an existing `.idx`/`.pack` file in place — it always writes new,
content-hash-named files, then unlinks the old ones. On POSIX (this project's actual
targets per `CLAUDE.md` — macOS + Linux systemd; no Windows service), an mmap'd file that
gets unlinked from its directory stays fully valid and readable through the existing
mapping: the kernel keeps the backing pages alive until the last reference — open fd *or*
active mapping — drops. So a stale mapping of a since-repacked-away `.idx` is a
**staleness** problem (it won't see newly packed objects), not a **corruption** problem.

**Detecting staleness:** mirror `ObjectStorage.Reindex()`'s existing pattern, triggered by
the same fsnotify-on-`.git`-dir mechanism `scanner.go`'s `fsnotifyLoop` already uses for
scan invalidation — on a write/create event under `objects/pack/`, diff the current
`ObjectPacks()` hash set against what's currently mapped, build entries only for genuinely
new pack hashes, and mark entries for hashes no longer present as "retiring."

**The genuinely hard part, and why this is staged out rather than built now:** a "retiring"
mapping must not be `munmap`'d while any in-flight `EncodedObject` call could still hold a
slice pointing into it — unlike a stale-but-harmless read (returns old data, no crash), an
actual use-after-`munmap` dereference is undefined behavior (SIGBUS at best). The correct
fix is a generation-counted or refcounted handle per pack (an in-flight read pins the
generation it started with; a retiring generation is only actually unmapped once its
refcount reaches zero), which is a meaningfully bigger unit of work than the
lock-serialization fix in §4.2 and was not attempted in the time available for this
prototype. **This is the one piece of the design explicitly NOT de-risked by a passing
test** — flagged per this task's instructions rather than glossed over.

**Windows caveat:** Windows does not allow unlink-while-mapped the way POSIX does. Not
relevant to this project's documented deployment targets, but worth a one-line flag if this
code is ever reused elsewhere.

## 6. Staged rollout plan

| Stage | Scope | Status |
|---|---|---|
| **0** | Shared idx parse + shared decoded-object cache, locked index access, read-only `EncodedObjectStorer`, `WorktreeStorer` wrapping `*filesystem.Storage` for Reference/Index/Shallow/Config/Module. | **Built and tested** in `session/unfinished/gogitstore/` (see §7 for numbers). |
| **1** | Production hardening: per-worktree cached pack file handles (avoid a fresh `os.Open` per object read — §4.1); `Registry` eviction/refcounting mirroring `gogit_vcs_reader.go`'s existing `pruneRepoCache`/`effectiveCacheBudgetBytes` TTL+budget logic, keyed by commondir + refcounted by live `WorktreeStorer`s; wire a `gogitstore`-backed `VCSReader` implementation behind `NewScannerWithReader` for canary rollout (not the default — see `session/unfinished/scanner.go:186` `NewScanner` vs `NewScannerWithReader`). | Not built. |
| **2** | mmap-backed index loading per §5, with the generation/refcount scheme needed for safe invalidation on repack. | Designed only (§5); explicitly the highest-risk, least de-risked piece. |
| **3** | (Only if canary profiling shows it's needed) shard the `SharedObjectStore` lock per-pack instead of per-commondir, to recover cross-worktree parallelism on index lookups. | Not needed unless stage 1 canary data says so. |

**Explicit non-goals kept out of this design and prototype entirely:** the write path
(`SetEncodedObject`/`PackfileWriter`) — this scanner is read-only by construction
(`session/unfinished/vcsreader.go`'s `VCSReader` interface has no write methods);
`IterEncodedObjects`; the `Alternates()` (`git clone --shared`) fallback
`filesystem.ObjectStorage.EncodedObject` has; `DeltaObjectStorer` (delta-aware reads,
needed for pack/repack tooling, not for this scanner); and
`filesystem.ObjectStorage.EncodedObjectSize`'s header-only-peek optimization (this
prototype's `EncodedObjectSize` fully materializes the object — acceptable because none of
`HasUncommitted`/`DiffShortstat`/`AheadBehind` call it).

## 7. What was actually proven, with real numbers

`gogitstore_test.go`'s `TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst` builds a
synthetic-but-packed fixture (250 commits, forced through `git gc --aggressive` so the
objects land in a real packfile — 1,250 index entries), opens 5 worktrees through one
shared `Registry`, and measures `runtime.MemStats.HeapAlloc` deltas around each
open-and-read (`Head()` → `CommitObject()` → `Tree()`), then repeats the same fixture and
measurement against stock `git.PlainOpenWithOptions` as a control:

```
shared:  worktree #0 (pays idx-parse cost) = 123,592–145,880 bytes (varies run to run)
shared:  worktrees #1..#4 average          =     904–2,306 bytes
control: worktree #0                        = 123,184–128,656 bytes
control: worktrees #1..#4 average           = 123,076–123,220 bytes   (no sharing — every worktree re-parses)
```

(Range shown across several `-race` runs — `go test`'s own memory instrumentation adds
noise under `-race`; the qualitative gap is stable across every run.)

`SharedObjectStore.IndexBuildCount` stays at exactly **1** across all 5 opens (asserted in
the test), confirming the parse genuinely only happens once. The later-worktree average
with sharing (well under 1% of the control's later-worktree average of ~123KB — worktrees
#1-4 pay essentially only per-call object materialization, no re-parsing) is one to two
orders of magnitude smaller than stock go-git's later-worktree average, which stays flat
at ~123KB per worktree because every one of them independently re-parses the same on-disk
data. `TestConcurrentReadsAcrossWorktrees_NoDataRace` passes clean under `go test -race`
(verified across 7 total runs, including a 5x-repeat of just that test) with 4 worktrees ×
4 goroutines each walking full commit history concurrently, and — per §4.2 and §4.3 — was
verified to actually catch **two** independent known-real concurrency bugs (the
`idxfile.MemoryIndex` offset-hash mutation, and `*dotgit.DotGit`'s unguarded
`incomingChecked` lazy cache) when their respective locks were temporarily removed.

This fixture is deliberately small (a few hundred KB, not a multi-GB monorepo — this
project's own `.git` is ~17GB and does not fit in the tmpfs-backed test temp directory
available in this sandbox) — the *ratio* is the load-bearing claim, and it should only
improve on a larger, more realistic repository, since the parse cost this design amortizes
scales with object count while the per-worktree marginal cost does not.

## 8. What remains designed-but-not-built (honest accounting)

- Stage 1 production hardening (per-worktree pack handle caching, `Registry`
  eviction/refcounting, the actual `VCSReader`-implementing wrapper wired through
  `NewScannerWithReader`) — none of this was built. The prototype proves the storage-layer
  mechanism; it does not yet provide a production-ready cache lifecycle.
- Stage 2 mmap — fully designed (§5) including the specific library, the zero-copy
  subslicing trick, and the repack-invalidation story, but explicitly **not implemented**,
  and the generation/refcount piece needed to make it safe is flagged as the least de-risked
  part of this whole design.
- Alternates fallback, `IterEncodedObjects`, `DeltaObjectStorer` — not implemented, not
  needed by this scanner's three read operations, documented as gaps rather than silently
  dropped.

## 9. Showstoppers found — none that change the recommendation

No finding in this investigation invalidates the shared-storer approach itself. The two
genuine new risks surfaced (not fully anticipated going in) are the `idxfile.MemoryIndex`
mutates-on-read behavior (§4.2) and the `*dotgit.DotGit` unguarded lazy-cache behavior
(§4.3) — without both fixes, this design would have shipped **guaranteed** crashes under
concurrent scanner load (both reproduced directly, not just theorized), not merely a
theoretical race. Both risks are now understood, deliberately designed around, and proven
fixed by tests that also prove they would have caught the bugs. The fact that a second,
independent race turned up on the very next full test run after the first was believed
fixed is itself the strongest argument in this document for §4.3's closing point: treat
"go-git internal type shared across worktrees" as guilty of unguarded lazy mutation until
proven innocent by a targeted `-race` test, not by reading the method name. The mmap piece
is real extra engineering effort (§5.3) but not infeasible — it is appropriately staged out
rather than either skipped silently or used to block the (already independently valuable)
Stage 0 sharing win described in §7.

## 10. Production-hardening pass — addendum (post-Stage-2)

A later pass addressed this document's two explicitly-flagged honesty gaps and pushed further
production-readiness work. Full detail lives in that pass's own report and in the code/tests it
added (`mmap_realrepo_test.go`, `mmap_adversarial_test.go`, `mmap_truncation_test.go`,
`soak_test.go`); summarized here for anyone reading this design doc without that report:

- **§1's "17 total -race passes" gap**: `TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack`
  was re-run clean 200+ times total across this pass (a `-count=200` run and a follow-up
  `-count=60` run, both `-race`, both 100% clean) — but the `-count=200` run's own repeated
  execution surfaced a REAL bug: the test itself leaked a `packWatchLoop` goroutine + fsnotify
  watcher every iteration (never called `stopPackWatch`), and the accumulation across ~150+
  iterations measurably slowed later iterations (3.4s → 13s+) before the whole binary hit `go
  test`'s default 10-minute timeout. Root-caused and fixed (every mmap-engaging test now cleans
  up); re-verified clean and flat-timing afterward. This was a test-hygiene bug, not a
  production one — `Registry.Prune`'s eviction path already calls `stopPackWatch` correctly —
  but it is the exact mechanism that WOULD leak in production if `Prune` ever stopped running.
- **§7's "18% synthetic fixture, should be more on a real repo" gap**: measured directly
  against this repository's own real `.git/objects/pack/*.idx` (~1.6MB, real production scale)
  using the same `TotalAlloc`-delta technique as §7's synthetic-fixture test. Result: the mmap
  loader allocates **~92–96% less heap** than the copy-based loader on this real repo's index
  (~1.78MB copy-based vs. ~72–139KB mmap-based) — confirming and substantially exceeding the
  synthetic fixture's 18% figure, as this document's own §7 caveat predicted.
- **New finding, fixed**: `loadMemoryIndexMmap` panicked (slice-bounds-out-of-range) on a
  corrupted/adversarial `.idx` with a non-monotonic fanout table, instead of returning a clean
  decode error the way the copy-based loader degrades. Fixed by validating fanout monotonicity
  before any bucket-range arithmetic; proven via a table-driven adversarial-input test plus a
  Go native fuzz target (`FuzzLoadMemoryIndexMmap`, 13M+ executions / 90s, zero crashes
  post-fix).
- **New finding, documented not fixed**: a `.idx` file truncated in place while mapped (NOT
  git's own repack behavior, which is safe per §5.3's unlink analysis — this requires external
  corruption or a misbehaving third-party tool) causes a hard process crash (SIGBUS), proven in
  a subprocess-isolated test. `runtime/debug.SetPanicOnFault` is a proven, available mitigation
  (also demonstrated) but was deliberately not wired into production code — see the activation
  runbook (`mmap-activation-runbook.md`) for the cost/benefit reasoning.
- **New finding, fixed**: a soak test (many repos/worktrees, sustained concurrent
  open/close/repack/prune load) found that BOTH modes — not mmap-specific — can surface
  `dotgit.ErrPackfileNotFound` (an upstream go-git sentinel distinct from
  `plumbing.ErrObjectNotFound`) when a lookup races a concurrent repack; existing
  `errors.Is(err, plumbing.ErrObjectNotFound)`-based tolerance elsewhere in this codebase did
  not catch it. Fixed by translating it in `SharedObjectStore`'s pack-open path. Also
  quantified: under an intentionally adversarial repack cadence, copy-based mode hit this on
  ~98% of operations (no automatic staleness recovery) vs. mmap mode's ~19% (sub-second
  fsnotify recovery) — i.e. mmap mode is measurably MORE resilient to concurrent repacks than
  the copy-based mode, not less.
- **No goroutine/fd leak, confirmed**: the soak test's forced-full-eviction assertions show
  goroutine count and open-fd count both return exactly to baseline after eviction, and heap
  shows only a small (~200KB) residual — eviction genuinely reclaims resources.
- **fsnotify watcher consolidation (§ "one extra watcher per commondir" trade-off)**:
  investigated sharing `scanner.go`'s existing fsnotify infrastructure instead of
  `mmapwatch.go`'s private per-commondir watcher. Feasible in principle (a shared
  `*fsnotify.Watcher` instance registering both `.git`-dir and `objects/pack` paths, with a
  consumer-owned — likely `gogit_vcs_reader.go` — event-dispatch loop routing pack-dir events
  into a new exported `Registry` entry point), but is a real cross-package refactor, not a
  quick win, for a savings that is small and bounded at this project's actual operating scale
  (one extra fd + one goroutine per currently-live mmap-enabled commondir, capped by
  `registryMaxEntries=100`). Recommendation: leave as documented technical debt unless
  profiling ever shows real pressure from it specifically — not attempted, per this task's own
  instruction not to force an architecturally awkward change under time pressure.
