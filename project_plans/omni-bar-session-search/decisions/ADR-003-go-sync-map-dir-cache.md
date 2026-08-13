# ADR-003: Go DirCache with sync.RWMutex, mtime Invalidation, and 60s TTL

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

`PathCompletionService` in `server/services/path_completion_service.go` is stateless. Every `ListPathCompletions` RPC call performs a fresh `os.ReadDir(baseDir)` with a 2-second timeout goroutine. The client has a 30-second TTL LRU cache in `usePathCompletions.ts`, but this is lost on page reload and not shared across browser sessions.

The requirement is: path completion results must appear in <100ms on repeated opens (cache hit) and <500ms cold (first open after server restart).

Four caching patterns were evaluated:

| Pattern | Pros | Cons |
|---|---|---|
| In-memory map + mtime check | Correct invalidation, no background goroutine | One `os.Stat` per Get call (~1µs) |
| TTL-only expiry | Simpler code | May show stale results up to TTL after directory change |
| `sync.Map` (built-in) | Optimized for read-heavy | Less explicit control; no capacity limit built-in |
| Disk-backed JSON | Survives server restart | High complexity; cold start budget <500ms is already met without it |

Two thread-safety mechanisms were evaluated for the in-memory map:
- `sync.Map`: built-in, optimized for append-once workloads (many readers, rare writes). No LRU/capacity limit built-in.
- `sync.RWMutex` + `map[string]*dirCacheEntry`: explicit, allows capacity check and O(n) eviction on write.

## Decision

Implement `DirCache` in `server/services/dir_cache.go` using:
- `sync.RWMutex` + `map[string]*dirCacheEntry` (not `sync.Map`)
- Primary invalidation: mtime comparison via `os.Stat(baseDir)` on each `Get`
- Fallback invalidation: 60-second hard TTL (handles network filesystems where mtime is unreliable)
- Max capacity: 256 entries with O(n) oldest-cachedAt eviction
- No disk persistence

`PathCompletionService` becomes stateful with a `cache *DirCache` field. The `NewPathCompletionService()` constructor signature is unchanged.

## Rationale

1. **mtime-based invalidation is correct for single-level reads.** On macOS APFS/HFS+ and Linux ext4, adding or removing a direct child of a directory updates that directory's mtime. Since `os.ReadDir` operates on one directory level (not recursive), mtime change in `baseDir` means the cache entry is stale. The `os.Stat` overhead is ~1µs per Get call — negligible against the <100ms target.

2. **60s TTL is the right backstop.** Network filesystems (NFS, SMB) may not update mtime synchronously. A 60-second backstop ensures cache entries never go stale for more than 1 minute in the worst case. The pitfalls research confirmed that for single-level caching (the only case here), mtime is reliable on local filesystems; the TTL handles the edge case.

3. **`sync.RWMutex` over `sync.Map`.** The cache profile is read-heavy (many path completion calls for the same prefix burst) with rare writes (cache misses). Both provide safe concurrent reads. `sync.RWMutex` was chosen because it allows explicit capacity management (check `len(c.entries)` under write lock, evict before inserting) and produces clearer code for the Get/Put/evict pattern. `sync.Map` would also work but lacks built-in LRU capacity enforcement.

4. **256 entries capacity.** A typical home directory traversal with 2-3 levels covers 20-50 unique `baseDir` values. 256 provides 5-10x headroom. O(n) eviction at n=256 is a scan of 256 pointers — sub-microsecond.

5. **No disk persistence.** The cold-start budget is <500ms. The client's existing 30s cache covers most realistic page reloads. Disk persistence would add startup I/O, a serialization layer, and stale-on-restart risk (cached mtime may not match actual mtime after unmount/remount). The complexity is not justified until profiling shows the cold-start budget is violated.

6. **Filtering runs per-request.** The cache stores raw `[]os.DirEntry` for the full base directory. Filtering by `filterPrefix`, `directoriesOnly`, and `showHidden` runs on every request. These are cheap CPU operations that depend on per-request parameters the cache cannot key on without significantly more complexity.

## Consequences

- New file: `server/services/dir_cache.go` implementing `DirCache` struct with `Get`, `Put`, `evictOldest`.
- New file: `server/services/dir_cache_test.go` with tests for hit, mtime-miss, TTL-miss, eviction, concurrent safety.
- `PathCompletionService.cache *DirCache` field added; zero callers need updating (constructor unchanged).
- The existing `os.Stat(baseDir)` call at line 67 in `ListPathCompletions` now serves dual purpose: directory existence check + mtime capture for cache invalidation.
- The goroutine+channel ReadDir block (lines 78-112) is wrapped with a cache lookup before and a cache store after on success.
- Known limitation: mtime invalidation is unreliable on network filesystems. The 60s TTL is the backstop. Document as expected behavior.
