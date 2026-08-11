# Feature Research: perf-mutex-hotspots-2026-07
Date: 2026-07-01

---

## 1. GoGitVCSReader — AheadBehind / DiffShortstat / HasUncommitted

### Call path and frequency

All three methods are called exclusively from `Scanner.scanWorktree` in
`session/unfinished/scanner.go` (~line 340–410). The scanner has:

- 1 coordinator goroutine ticking every 30 s
- **4 concurrent worker goroutines** pulling from a shared channel
- A per-worktree TTL cache inside the scanner (separate from the per-method caches already on GoGitVCSReader)

Execution order within a single `scanWorktree` call:

1. `HasUncommitted(wt.Path)` — always called
2. `AheadBehind(wt.Path, defaultBranch)` — called if `defaultBranch != ""`
3. `CommitMessages(wt.Path, defaultBranch, 5)` — called only if `AheadCount > 0`
4. `DiffShortstat(wt.Path)` — always called

The two callers of `AheadBehind` are both from the scanner worker path (no other
callers in the main codebase). `DiffShortstat` and `HasUncommitted` are also
scanner-only.

### Singleflight key design

The existing per-method caches already key on `worktreePath` for `DiffShortstat`/`HasUncommitted` and `worktreePath+"\x00"+base` for `AheadBehind`. A singleflight group should use the same keys:

- `DiffShortstat` / `HasUncommitted`: key = `worktreePath` (no extra caller params)
- `AheadBehind`: key = `worktreePath + "\x00" + base`

The `base` parameter comes from `ResolveDefaultBranch` output (e.g. `"origin/main"`), not from individual callers, so it is stable within a scan cycle.

### go-git goroutine safety constraint

`cachedRepo.mu` (per-repo `sync.Mutex`) must still be held **inside** the singleflight `Do` body for all `repo.*` calls. The singleflight group only collapses concurrent callers waiting for the same result; it does not replace the packfile-reader lock.

### Current caching status (already in place)

- `DiffShortstat`: 30 s TTL cache on `GoGitVCSReader.diffStatCache` (sync.Map)
- `AheadBehind`: 30 s TTL cache on `GoGitVCSReader.aheadBehindCache` (sync.Map)
- `HasUncommitted`: **no TTL cache** — runs the full index/stat walk on every miss
- `CommitMessages`: 30 s TTL cache on `GoGitVCSReader.commitMessagesCache`

The thundering-herd problem is: 4 workers can simultaneously miss the cache for the same worktree path within a single tick and each start an expensive graph walk while holding `entry.mu`. Singleflight collapses all 4 into a single execution.

---

## 2. CircularBuffer — sync.Mutex → sync.RWMutex

### Current state

`session/circular_buffer.go` already declares `mu sync.RWMutex` (line 22) and
already uses `cb.mu.RLock()` / `cb.mu.RUnlock()` on **all read paths**:

- `GetRecent` — uses `RLock` ✅
- `GetAll` — uses `RLock` ✅
- `Len` — uses `RLock` ✅
- `TotalBytesWritten` — uses `RLock` ✅
- `WriteTo` — uses `RLock` ✅

Write paths (`Write`, `Clear`, `EnableDiskFallback`, `DisableDiskFallback`,
`Close`) use `Lock` ✅

**The RWMutex upgrade is already complete in the codebase.** The field was `sync.RWMutex` at line 22 and all read-path methods already use `RLock`. This hotspot may already be fixed, or the profiling data predates the conversion.

### Callers

The CircularBuffer in the session package is used via `PTYAccess` (`session/pty_access.go`). The main write path is:

- `session/response_stream.go` line 284: `rs.ptyAccess.buffer.Write(chunk.Data)` — stream writer goroutine
- `session/external_streamer.go` line 418: `s.buffer.Write(msg.Data)`
- `session/claude_controller.go` line 768: `h.Write([]byte(s))`

The main concurrent read paths (which benefit from RLock):

- `session/claude_controller.go` line 559/561: `h.GetAll()` / `h.GetRecent(limit)` — used on every status detection poll
- `session/detection/idle.go` line 118: `GetRecentOutput(...)` — idle detector
- `session/detection/ratelimit/integration.go` lines 152/157: `GetRecentOutput(4096)` — rate-limit detector
- `session/search/engine.go` lines 93, 505, 617: `history.GetAll()` — search engine

These reads happen from multiple goroutines concurrently (stream writer + detector + search), so the RLock separation is valuable. If profiling still shows contention, the `write` path in `response_stream.go` is the dominant writer (~PTY data rate).

---

## 3. GitWorktree.IsDirty / IsDirtyWithHint — 5 s TTL cache

### Current state

`session/git/worktree_git.go` already has a cache: `IsDirtyCacheTTL = 15 s` (defined in `session/git/worktree.go` line 26). The implementation uses a **`sync.RWMutex`** with fast-path `RLock` and slow-path `Lock`, with the subprocess running outside any lock. Double-checked locking pattern is correctly implemented (returns locally-computed `dirty`, not the cache slot).

The requirements document says "Add 5s TTL result cache" but a 15 s TTL cache already exists. Scope for this hotspot is likely: reduce TTL from 15 s to 5 s, or verify the existing cache is sufficient.

### Callers and state transition sites

All callers in the main codebase (excluding worktrees/):

| File | Caller | Context |
|---|---|---|
| `session/git/worktree_git.go:43` | `PushChanges` | Before commit — followed by `InvalidateDirtyCache()` after commit |
| `session/git/worktree_git.go:104` | `CommitChanges` | Before commit — followed by `InvalidateDirtyCache()` after commit |
| `session/git_worktree_manager.go:132` | `GitWorktreeManager.IsDirty()` | Wrapper, delegates to `worktree.IsDirty()` |
| `session/instance.go:987` | session poller | Checks dirty state on each poll tick |
| `session/review_queue_determiner.go:79` | review queue | Determines if session needs review |

Cache invalidation is already explicit at the two state-transition points that matter: after `git commit` in both `PushChanges` and `CommitChanges` via `InvalidateDirtyCache()`. No additional invalidation points are needed for a TTL change.

`PrimeDirtyCacheAt` is called from `session/git_worktree_manager.go:51` with a random jitter in `[0, IsDirtyCacheTTL)` to stagger initial scan expiry across sessions. If TTL is reduced to 5 s, the jitter range auto-adjusts since it reads `git.IsDirtyCacheTTL` dynamically.

`IsDirtyWithHint(claudeActive=true)` is not called from any external caller in the current codebase — only `IsDirty()` → `IsDirtyWithHint(false)`. The `claudeActive` path is implemented but unused externally.

---

## 4. Test Coverage Map

### GoGitVCSReader tests

| Test file | Functions covering hotspot methods |
|---|---|
| `session/unfinished/vcsreader_test.go` | `TestVCSReaderContractGoGit` (contract suite), `TestGoGitVCSReader_AheadBehind_BehindCount`, `TestGoGitVCSReader_HasUncommitted_StagedChange`, `TestGoGitVCSReader_HasUncommitted_StagedDeletion`, `TestGoGitVCSReader_HasUncommitted_MergeConflict`, `TestDiffShortstat_MultiBlobWorktree` |
| `session/unfinished/gogit_vcs_reader_limits_test.go` | `TestGoGitVCSReader_DiffShortstat_largeUntrackedFile_countedButNotRead`, `TestGoGitVCSReader_DiffShortstat_smallUntrackedFile_linesAreCounted`, `TestGoGitVCSReader_DiffShortstat_untrackedFilesCapStopsWalk`, `TestGoGitVCSReader_DiffShortstat_largeModifiedTrackedFile_countedButNotDiffed` |
| `session/unfinished/vcsreader_bench_test.go` | `BenchmarkHasUncommitted`, `BenchmarkAheadBehind`, `BenchmarkDiffShortstat`, `BenchmarkDiffShortstatCached`, `BenchmarkFullScanCycle` |

Singleflight changes will need a new concurrency test (e.g. `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`) to assert only one compute happens when N goroutines race on a cache miss.

### CircularBuffer tests

File: `session/circular_buffer_test.go`

Key tests: `TestCircularBuffer_ConcurrentWrites`, `TestCircularBuffer_ConcurrentReads`, `TestCircularBuffer_ConcurrentReadWrite`. Benchmarks: `BenchmarkCircularBufferWrite_4KB`, `BenchmarkCircularBufferGetRecent_4KB`, `BenchmarkCircularBufferGetAll`. Since the RWMutex upgrade appears already done, no test changes needed unless re-verifying.

### IsDirty tests

File: `session/git/worktree_git_test.go`

Existing test: `TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine` covers the double-checked locking invariant. A TTL change from 15 s → 5 s requires updating the constant in `session/git/worktree.go` and verifying no tests hard-code `15 * time.Second`.

---

## 5. Key Risks and Constraints

### Singleflight panic propagation

`singleflight.Do` propagates panics to all waiters. go-git methods can panic on
malformed pack data. Mitigation: recover in the `Do` body and return an error
instead, or use `DoChan` and check for panics in the channel result.

### go-git goroutine safety inside Do

`entry.mu` must still be held inside the `Do` body for all `repo.*` calls. The
singleflight group reduces the number of goroutines competing for `entry.mu` to
at most 1 per key, which is the intended fix.

### IsDirty and the 5 s TTL

The current `IsDirtyCacheTTL = 15 s` was already a deliberate decision. Reducing
to 5 s triples subprocess frequency at idle. Check if `instance.go:987` poll
interval is shorter than 15 s before deciding — if the poller already runs every
10-30 s, 5 s adds no value.
