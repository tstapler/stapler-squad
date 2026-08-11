# BUG-024: SearchService branchCache and historyCache Use sync.RWMutex; Should Use singleflight + atomic.Value [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-06-24
**Impact**: `server/services/search_service.go` uses `branchCacheMu sync.RWMutex` and
`historyCacheMu sync.RWMutex` to protect map caches. The lock is not held across I/O
(correct pattern), but concurrent cache misses for the same key cause redundant git
subprocess calls. Replacing with `singleflight + atomic.Value` eliminates the duplicate
calls and the mutex.

## Problem Description

`search_service.go` lines ~111–145:

```go
func (ss *SearchService) cachedBranch(projectPath string) (string, error) {
    ss.branchCacheMu.RLock()
    entry, ok := ss.branchCache[projectPath]
    ss.branchCacheMu.RUnlock()
    if ok && time.Now().Before(entry.expiry) {
        return entry.branch, nil
    }

    // ← Lock NOT held during this git call (good!) but...
    branch, err := ss.resolveDefaultBranch(projectPath)
    if err != nil {
        return "", err
    }

    ss.branchCacheMu.Lock()
    ss.branchCache[projectPath] = historyBranchEntry{branch: branch, expiry: ...}
    ss.branchCacheMu.Unlock()
    return branch, nil
}
```

Two problems:

1. **Thundering herd on cache miss**: If 10 goroutines call `cachedBranch` simultaneously
   for the same `projectPath` at cache-miss time, all 10 will see `ok == false` (or a stale
   entry), all 10 will call `resolveDefaultBranch`, and all 10 will write the same value
   back to the map. The git calls are idempotent but wasteful.

2. **Mutex still required for the write path**: The `sync.RWMutex` still introduces
   contention — even if reads are lock-free at the OS level, the semaphore mechanism
   under RLock has overhead. An `atomic.Value` snapshot eliminates the lock entirely for
   readers; writers only serialize among themselves through `singleflight`.

## Root Cause

Standard Go cache-miss pattern: read-lock, miss, release, compute, write-lock. Correct for
correctness, suboptimal for concurrent same-key misses. Written before codebase preference for
atomic/singleflight patterns was established.

## Files Affected

- `server/services/search_service.go` — `cachedBranch()`, `cachedHistory()` (similar pattern)

## Fix Approach

Replace `map + sync.RWMutex` with `singleflight.Group` for miss deduplication and
`atomic.Value` for the cached snapshot:

```go
import "golang.org/x/sync/singleflight"

type SearchService struct {
    branchCache   atomic.Value         // stores map[string]historyBranchEntry (immutable)
    branchGroup   singleflight.Group   // coalesces concurrent cache-miss fetches
    // remove: branchCacheMu sync.RWMutex, branchCache map[...]
}

func (ss *SearchService) cachedBranch(projectPath string) (string, error) {
    // Load snapshot — lock-free
    if m, ok := ss.branchCache.Load().(map[string]historyBranchEntry); ok {
        if entry, hit := m[projectPath]; hit && time.Now().Before(entry.expiry) {
            return entry.branch, nil
        }
    }

    // Coalesce concurrent misses for the same key
    v, err, _ := ss.branchGroup.Do(projectPath, func() (any, error) {
        branch, err := ss.resolveDefaultBranch(projectPath)
        if err != nil {
            return "", err
        }
        // Write new snapshot atomically — COW map update
        for {
            old := ss.branchCache.Load()
            var oldMap map[string]historyBranchEntry
            if old != nil {
                oldMap = old.(map[string]historyBranchEntry)
            }
            newMap := make(map[string]historyBranchEntry, len(oldMap)+1)
            for k, v := range oldMap {
                newMap[k] = v
            }
            newMap[projectPath] = historyBranchEntry{branch: branch, expiry: time.Now().Add(branchCacheTTL)}
            if ss.branchCache.CompareAndSwap(old, newMap) {
                break
            }
            // Another goroutine updated concurrently — retry CAS
        }
        return branch, nil
    })
    if err != nil {
        return "", err
    }
    return v.(string), nil
}
```

Note: `atomic.Value.CompareAndSwap` requires Go 1.17+. The singleflight ensures only one
goroutine performs the git call per key per miss event; the CAS loop handles the rare race
between two singleflight groups for different keys updating the snapshot simultaneously.

If COW map CAS feels overly complex for this cache size, an acceptable middle ground is
`singleflight + sync.Mutex` (mutex held only by the singleflight winner, not by all readers).

## Verification

After fix: `go test ./server/services/... -race -count=3` must pass without DATA RACE.
Add `BenchmarkSearchService_CachedBranch_ConcurrentReads` to confirm zero contention
on cache-hit path (`AllocsPerRun` ≈ 1 for the `atomic.Value.Load` + map lookup).

## Related

- BUG-022: ETagCache RWMutex map
- BUG-023: PRStatusPoller mutex churn
- `server/services/connectrpc_websocket.go:170` — `snapshotCacheMu sync.RWMutex` has the same
  pattern and should be similarly migrated (tracked under BUG-025 when opened)
