# Stack Research: perf-mutex-hotspots-2026-07

Date: 2026-07-01

---

## 1. `golang.org/x/sync/singleflight` — API & Go 1.25 Compatibility

**Version in use:** `golang.org/x/sync v0.20.0` (confirmed in `go.mod`). Fully compatible with Go 1.25.

### Correct API

```go
import "golang.org/x/sync/singleflight"

var sfGroup singleflight.Group

// Do blocks duplicates; v is the shared return value, shared=true if deduplicated
v, err, shared := sfGroup.Do(key, func() (interface{}, error) {
    // cache-miss computation here
    return result, err
})
result := v.(MyType)
```

- `Do(key string, fn func() (any, error)) (v any, err error, shared bool)` — synchronous; all callers for the same key block until `fn` returns, then all receive the same `(v, err)`.
- `DoChan(key, fn)` returns a `<-chan Result` — useful only when you need non-blocking behaviour or context cancellation. **Not needed here** since callers already accept a synchronous API.
- `Forget(key)` — removes the in-flight record immediately, causing the next `Do` for that key to start a new call even if one is in flight. Use for cache invalidation scenarios (e.g. after `InvalidateDirtyCache()`). **Not needed for the gogit reader** because TTL-based caches already expire naturally.
- Return value semantics: `v` is the value returned by whichever goroutine ran `fn`; **all waiters receive the same pointer**. If `fn` returns a mutable struct, callers must not mutate it. In this codebase the cache values (`aheadBehindEntry`, `diffStatEntry`, `DiffStat`) are stored by value, so sharing is safe.

### Key design for gogit hotspots

- `AheadBehind`: key = `worktreePath + "\x00" + base` (matches existing `aheadBehindCache` key).
- `DiffShortstat`: key = `worktreePath`.
- `HasUncommitted`: key = `worktreePath`.

### Placement in the call flow

The singleflight `Do` call should wrap the **cache-miss slow path only**, after the fast-path TTL check. Do not hold the per-repo `entry.mu` before calling `sfGroup.Do` — the goroutine that wins the race will acquire it internally; waiters must not enter holding the lock.

---

## 2. `sync.RWMutex` — CircularBuffer Upgrade Status

**The CircularBuffer (`session/circular_buffer.go`) already uses `sync.RWMutex`.**

Inspection of the struct definition (line 20) and all call sites confirmed:

| Method | Lock used |
|---|---|
| `Write` | `mu.Lock()` / `mu.Unlock()` |
| `GetRecent` | `mu.RLock()` / `mu.RUnlock()` |
| `GetAll` | `mu.RLock()` / `mu.RUnlock()` |
| `ReadFrom` | `mu.Lock()` / `mu.Unlock()` |
| `WriteTo` | `mu.Lock()` / `mu.Unlock()` |
| `Len` | `mu.RLock()` / `mu.RUnlock()` |
| `TotalBytesWritten` | `mu.RLock()` / `mu.RUnlock()` |

The upgrade from `sync.Mutex` to `sync.RWMutex` has already been applied. **This hotspot item requires no code change.**

`go vet` checks for `sync.RWMutex`: vet catches `Lock`/`Unlock` mismatches and copying of a locked mutex (via `go vet -copylocks`). Since `CircularBuffer` is always used via pointer, copy-lock is not a risk.

---

## 3. Per-Entry TTL Cache for `IsDirty` / `IsDirtyWithHint`

**`GitWorktree.IsDirty` already has a 15 s TTL cache (`IsDirtyCacheTTL = 15 * time.Second`).**

Inspection of `session/git/worktree.go` and `session/git/worktree_git.go` confirmed the full implementation:

- Struct fields: `isDirtyCacheMu sync.RWMutex`, `isDirtyCache bool`, `isDirtyCacheTime time.Time`.
- Fast path: `RLock` → check TTL → return cached value.
- Slow path: run `git status --porcelain` outside any lock → `Lock` → conditional store (double-checked) → `Unlock`.
- `InvalidateDirtyCache()` zeroes `isDirtyCacheTime` under `Lock`.
- `PrimeDirtyCacheAt(t)` staggers initial expiry to prevent thundering herd on startup.

The pattern already follows the CLAUDE.md double-checked-locking rule (returns locally-computed `dirty`, not the cache slot). **This hotspot item requires no code change** — the cache is already in place with correct TTL and the right locking discipline.

The requirements doc's note about "add 5s TTL result cache per worktree path" appears to be superseded by the 15s TTL already present. The scope item should be validated against profiling data before any change.

---

## 4. go-git v5.14.0 Packfile Reader Safety — Singleflight Design Implications

**go-git's packfile reader is NOT safe for concurrent use from multiple goroutines sharing a single `*git.Repository`.**

This is explicitly documented in comments throughout `gogit_vcs_reader.go`:

> "go-git's packfile reader is not goroutine-safe; the scanner runs 4 concurrent workers sharing a single queue, so the same repo can be processed by two workers simultaneously. The lock must therefore cover every `repo.CommitObject` call, not just the snapshot phase." (line ~439)

The existing design uses a per-repo `entry.mu sync.Mutex` (in `cachedRepo`) that must be held for the entire duration of any go-git call sequence. This is the serialisation point singleflight is meant to reduce.

### Singleflight + per-repo mutex interaction

The correct pattern for wrapping with singleflight:

```go
// Fast path: TTL cache hit (no lock needed)
if v, ok := g.aheadBehindCache.Load(cacheKey); ok {
    if e := v.(aheadBehindEntry); time.Now().Before(e.expiry) {
        return e.ahead, e.behind, nil
    }
}

// Deduplicate concurrent cache-miss computations for the same key
val, err, _ := g.sfGroup.Do(cacheKey, func() (any, error) {
    entry, err := g.openRepoEntry(worktreePath)
    if err != nil { return nil, err }
    entry.mu.Lock()
    // ... go-git calls ...
    entry.mu.Unlock()
    g.aheadBehindCache.Store(cacheKey, ...)
    return result, nil
})
```

This means: goroutines that would have all acquired `entry.mu` in sequence are now collapsed into one. The go-git packfile reader still sees only one goroutine at a time (serialised by `entry.mu`), satisfying its safety requirement. Singleflight reduces the number of goroutines that ever reach `entry.mu.Lock()` for the same key.

**`Forget` is not required** here because the TTL cache is the source of truth for freshness; singleflight is only used to deduplicate concurrent cache-miss computations, not as a persistent cache.

A separate `singleflight.Group` per method (or a shared one keyed with a method prefix) both work. A shared group with prefixed keys (e.g. `"ab:" + cacheKey`, `"ds:" + worktreePath`) is simplest.

---

## Summary

| Hotspot | Status | Action Required |
|---|---|---|
| GoGitVCSReader thundering herd (`AheadBehind`, `DiffShortstat`, `HasUncommitted`) | NOT YET addressed | Add `singleflight.Group` field to `GoGitVCSReader`; wrap cache-miss slow paths with `sfGroup.Do` |
| CircularBuffer `sync.Mutex` → `sync.RWMutex` | **ALREADY DONE** | No change needed |
| `IsDirty`/`IsDirtyWithHint` TTL cache | **ALREADY DONE** (15s TTL) | No change needed; confirm 5s vs 15s intent with requirements owner |
