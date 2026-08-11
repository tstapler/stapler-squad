# Architecture Research: perf-mutex-hotspots-2026-07

Date: 2026-07-01

## Q1: singleflight.Group placement and cache key design

### Finding

`GoGitVCSReader` already has **separate `sync.Map` caches per operation type**:
- `diffStatCache` — keyed by `worktreePath`
- `aheadBehindCache` — keyed by `worktreePath + "\x00" + base`
- `commitMessagesCache` — keyed by `worktreePath + "\x00" + base + "\x00" + max`
- `reachableSetCache` — keyed by `plumbing.Hash` (base commit hash)

Each operation has a distinct key schema. A single shared `singleflight.Group` would use the same key namespace for all operations, causing false coalescing: an in-flight `AheadBehind("repo", "origin/main")` would coalesce with a hypothetical `DiffShortstat("repo", "origin/main")` if both used the same cache key. This must not happen — the operations return different types.

### Recommendation

Use **separate `singleflight.Group` per operation type**, stored as fields on `GoGitVCSReader`:

```go
type GoGitVCSReader struct {
    // existing fields...
    diffStatSF       singleflight.Group
    aheadBehindSF    singleflight.Group
    commitMessagesSF singleflight.Group
    reachableSetSF   singleflight.Group
}
```

Each group mirrors its corresponding `sync.Map` cache. Keys within each group must match the cache key exactly so that the coalesced call populates the cache and all waiters benefit.

### Panic propagation

`singleflight.Do` propagates panics to all callers sharing the flight. Because go-git pack-file reads can trigger internal panics (nil dereference in malformed packs), wrap the `Do` body in a `recover`:

```go
v, err, _ := g.aheadBehindSF.Do(cacheKey, func() (interface{}, error) {
    defer func() {
        if r := recover(); r != nil {
            // log and return error — singleflight converts this to an error return
        }
    }()
    // ... actual work ...
})
```

`entry.mu` is still required **inside** the `Do` body: `singleflight` coalesces callers at the Go level but does not make the underlying go-git packfile reader goroutine-safe. Only one goroutine executes `Do`'s body at a time per key, but other worktrees may call `entry.mu.Lock()` concurrently on the same `cachedRepo`.

---

## Q2: IsDirty cache placement — per-instance GitWorktree vs package-level sync.Map

### Finding: GitWorktree lifecycle

`GitWorktree` is a **long-lived, per-session struct**. It is constructed once (via `NewGitWorktree*`) when a session is created or loaded from storage, stored as `GitWorktreeManager.worktree`, and lives until the session is paused, deleted, or the process restarts. It is not created per-call.

Evidence:
- `NewGitWorktreeFromStorage` is called during session restore; the struct persists in `Instance.gitManager`
- `GitWorktreeManager` is a field on `Instance` (not created per-request)
- `IsDirtyCacheTTL = 15s` and `isDirtyCacheTime` are instance fields already protected by `isDirtyCacheMu sync.RWMutex`

### Recommendation

**Keep the cache on `GitWorktree` (per-instance)** — do not use a package-level `sync.Map`. Rationale:

1. The cache is already structured correctly: per-instance `isDirtyCache bool` + `isDirtyCacheTime time.Time` + `isDirtyCacheMu sync.RWMutex`.
2. A package-level `sync.Map` keyed by `worktreePath` would require GC or TTL eviction to avoid leaking entries for deleted sessions. The per-instance approach is automatically cleaned up when the `Instance` is garbage-collected.
3. The thundering-herd problem (the actual issue) is that **two concurrent callers on the same `GitWorktree`** see a stale cache simultaneously and both spawn `git status`. This is fixed by adding a `singleflight.Group` field to `GitWorktree` (key = constant `"is_dirty"`, since the worktree path is implicit in the instance), not by moving the cache to a global map.

The existing `IsDirtyWithHint(claudeActive bool)` slow path already follows the correct double-checked-locking return convention (returns locally-computed `dirty`, not the re-read cache slot).

---

## Q3: IsDirty cache invalidation hooks in ReviewQueuePoller / checkSession

### Finding: where state changes happen

`checkSession` in `review_queue_poller.go` does **not** directly observe commit/checkout events. It calls `rqp.statusDeterminer.Determine(...)` which internally calls `i.gitManager.IsDirty()` via `Instance.GetDiffStats()` (line ~755 in `review_queue_poller.go`).

State transitions that should invalidate the dirty cache already call `InvalidateDirtyCache()` directly on the `GitWorktree`:

- `CommitChanges()` in `worktree_git.go` line 121 — calls `g.InvalidateDirtyCache()` after a successful commit
- `PushChanges()` in `worktree_git.go` line 60 — calls `g.InvalidateDirtyCache()` after a successful commit

The `transitionTo` calls in `instance.go` (lines 742, 904, 1037, 1147, 1282) do **not** call `InvalidateDirtyCache()`. This is the gap for the "session state transition" invalidation requirement.

### Recommendation

The cleanest bust hook is to call `g.gitManager.InvalidateDirtyCache()` in `Instance.transitionTo()` for the transitions that involve a git checkout or worktree state change:
- `Paused → Active` (Resume calls `git worktree add` which restores files)
- `Active → Stopped` (session ended — next resume will recheck)

The `ReviewQueuePoller` itself does not need a new invalidation path: it is a polling consumer. The invalidation should happen at the state-transition call site (`instance.go`), not in the poller.

For the 5s TTL IsDirty cache mentioned in the requirements: the current TTL is already 15s (`IsDirtyCacheTTL`). If the goal is to reduce it to 5s, that is a one-line constant change. The singleflight fix is more impactful than the TTL reduction.

---

## Q4: CircularBuffer concurrent access contract and RWMutex design

### Finding

`CircularBuffer` already uses `sync.RWMutex` (field `mu sync.RWMutex` — confirmed in `session/circular_buffer.go` line 20). **This hotspot is already fixed** in the current codebase.

The implementation already uses:
- `cb.mu.Lock()` for `Write`, `Clear`, `EnableDiskFallback`, `DisableDiskFallback`, `Close`
- `cb.mu.RLock()` for `GetRecent`, `GetAll`, `Len`, `TotalBytesWritten`, `WriteTo`

Concurrent access contract: multiple goroutines may call `GetRecent`/`GetAll`/`Len`/`WriteTo` concurrently without blocking each other. `Write` is exclusive. This matches the typical PTY usage pattern: one writer goroutine (PTY read loop) and multiple reader goroutines (WebSocket streamers, status detectors).

The `diskFile *os.File` field is only accessed under the full write lock (`mu.Lock()`), so there is no additional synchronization needed for the disk fallback path (currently a placeholder).

### Implication for the implementation plan

The CircularBuffer `sync.Mutex → sync.RWMutex` upgrade described in the requirements is **already done**. The implementation task should verify this is deployed (i.e., not in a stale worktree) and skip or repurpose that work item.

---

## Summary of Key Decisions

| Question | Decision |
|---|---|
| singleflight placement | One `singleflight.Group` per operation type on `GoGitVCSReader`; keys mirror the existing `sync.Map` cache keys |
| IsDirty cache location | Per-instance on `GitWorktree` (already correct); add `singleflight.Group` field to `GitWorktree` to coalesce thundering herd on same instance |
| IsDirty invalidation hook | Call `InvalidateDirtyCache()` in `Instance.transitionTo()` for worktree-affecting transitions (Resume, Stop); existing commit/push paths already invalidate correctly |
| CircularBuffer RWMutex | Already uses `sync.RWMutex` — no work needed |
