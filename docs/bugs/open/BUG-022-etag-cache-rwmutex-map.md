# BUG-022: ETagCache Uses sync.RWMutex Over Map Instead of sync.Map [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-06-24
**Impact**: ETagCache is a read-mostly cache where entries are written once and read on every poll
tick. `sync.RWMutex` over `map[string]etagEntry` forces readers to acquire a shared lock;
replacing with `sync.Map` (or `xsync.MapOf`) gives lock-free reads in the steady state.

## Problem Description

`github/etag_cache.go` declares:

```go
type ETagCache struct {
    mu    sync.RWMutex
    store map[string]etagEntry
}
```

`GetPRInfoConditional` correctly releases the RLock before the HTTP call and re-acquires a
write lock after — so the bug is not "lock held across I/O". The problem is architectural:

1. **Contention on every read**: In a 20-session poller, 20 goroutines can call `Get()` on the
   same tick. With `sync.RWMutex`, all readers serialize on the semaphore acquisition path
   even when no writer is active. `sync.Map.Load` has no lock at all in the common read-hit path.

2. **Invisible lock scope risk**: The exported `RLock/RUnlock` pattern is easy to misuse.
   A future caller could forget to release the lock before an outbound HTTP call, causing the
   lock-across-I/O bug that BUG-020/021 already document. `sync.Map` removes this surface —
   callers cannot hold its internal lock across an I/O call because there is no exported lock.

3. **Inconsistency with codebase style**: The new preference (established by the user 2026-06-24)
   is `sync.Map` or `xsync.Map` for concurrent maps, not `map + mutex`.

## Reproduction Steps

1. Profile the PR status poller under concurrent load with `go tool pprof -mutex`.
2. Look for `ETagCache.RLock` or `ETagCache.RUnlock` in the top contention nodes.
3. With 20 sessions polling at 60s intervals, expect ~20 concurrent readers per tick.

## Root Cause

`ETagCache` was written before the codebase-wide preference for lock-free structures was
established. `sync.RWMutex` is technically correct but is the low-value option for a map
whose entries are written once (on first PR discovery) and read on every subsequent tick.

## Files Affected

- `github/etag_cache.go` — `ETagCache` struct and all its methods
- `github/pr_status_poller.go` — caller of `ETagCache.GetPRInfoConditional`

## Fix Approach

**Option A (minimal)**: Replace `map[string]etagEntry + sync.RWMutex` with `sync.Map`:

```go
type ETagCache struct {
    store sync.Map // key: string, value: etagEntry
}

func (c *ETagCache) Get(key string) (etagEntry, bool) {
    v, ok := c.store.Load(key)
    if !ok {
        return etagEntry{}, false
    }
    return v.(etagEntry), true
}

func (c *ETagCache) Set(key string, e etagEntry) {
    c.store.Store(key, e)
}
```

**Option B (preferred for typed safety)**: Replace with `xsync.MapOf[string, etagEntry]`
from `puzpuzpuz/xsync/v4`. Provides CLHT-based lock-free map with generic type safety and
better throughput than `sync.Map` under write contention.

```go
import "github.com/puzpuzpuz/xsync/v4"

type ETagCache struct {
    store *xsync.MapOf[string, etagEntry]
}

func NewETagCache() *ETagCache {
    return &ETagCache{store: xsync.NewMapOf[string, etagEntry]()}
}
```

## Verification

After fix: `go test ./github/... -race -count=3` must pass without DATA RACE.
Benchmark: add `BenchmarkETagCache_ConcurrentReads` with `b.RunParallel` and verify
`ns/op` is lower than the `sync.RWMutex` baseline.

## Related

- BUG-020: GetVCSStatus mutex contention (same class of bug)
- BUG-021: CheckGHAuth mutex contention
- BUG-023: PRStatusPoller mutex churn
