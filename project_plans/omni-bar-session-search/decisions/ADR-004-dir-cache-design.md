# ADR-004: Server-Side Directory Tree Cache Design

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

`PathCompletionService` in `server/services/path_completion_service.go` is currently stateless (comment at line 25: "It is stateless; each call performs a fresh directory listing"). Every `ListPathCompletions` RPC call executes a full `os.ReadDir(baseDir)` with a 2-second timeout.

The client already has a 30s TTL LRU cache in `usePathCompletions.ts` (module-level `completionCache` at lines 27-55). This protects against burst typing but not against page reloads or cold server starts.

Requirement 3 specifies: path completion results must appear in <100ms on repeated opens (cache hit) and <500ms cold.

Four caching patterns were evaluated:
1. In-memory map + mtime check (primary invalidation)
2. TTL-only expiry (simpler, slightly less correct)
3. `sync.Map` vs custom `sync.RWMutex` map
4. Disk-backed JSON persistence

## Decision

Implement a server-side in-memory `DirCache` in a new file `server/services/dir_cache.go` with:
- Key: absolute `baseDir` path (after tilde expansion)
- Invalidation: mtime comparison on `os.Stat(baseDir)` — if directory mtime changed, cache entry is invalid
- Fallback TTL: 60 seconds (handles network filesystems where mtime updates are unreliable)
- Max size: 256 entries with LRU eviction (O(n) scan acceptable at this scale)
- Thread safety: `sync.RWMutex` with read-lock for Get, write-lock for Put/eviction
- No disk persistence initially

`PathCompletionService` becomes stateful by adding a `*DirCache` field. The `NewPathCompletionService()` constructor signature is unchanged — it allocates the cache internally.

## Rationale

1. **mtime-based invalidation is correct for single-level reads.** On macOS (APFS/HFS+), adding or removing a direct child of a directory updates that directory's mtime. `os.ReadDir` operates on one directory level. The pitfalls research confirms this is sufficient for the use case.
2. **60s TTL as safety net.** Network filesystems and some edge cases may not update mtime reliably. The TTL backstop catches these without sacrificing correctness in the common case.
3. **256 entries.** A typical home directory traversal with 2-3 levels of drilling covers ~20-50 unique `baseDir` values. 256 provides generous headroom.
4. **`sync.RWMutex` over `sync.Map`.** The cache profile is read-heavy (many path completion calls) with rare writes (cache miss). `sync.RWMutex` allows concurrent reads; `sync.Map` would also work but `RWMutex` gives explicit control and clarity.
5. **No disk persistence.** Cold-start budget is <500ms. The client's existing 30s cache covers most realistic reloads. Disk persistence adds complexity for marginal gain at the current scale.
6. **Filtering still runs per-request.** The cache stores raw `[]os.DirEntry` for the base directory. Filtering by `filterPrefix`, `directoriesOnly`, and `showHidden` continues to run on every request (cheap CPU operations that depend on per-request parameters).

## Consequences

- New file: `server/services/dir_cache.go` with `DirCache` struct.
- `PathCompletionService` struct gains `cache *DirCache` field.
- `NewPathCompletionService()` allocates `NewDirCache(256, 60*time.Second)` internally — no caller changes required.
- The `os.ReadDir` goroutine in `ListPathCompletions` (lines 88-91) is wrapped with a cache check before and a cache store after.
- `os.Stat(baseDir)` at line 67 now serves double duty: existence check + mtime capture for cache invalidation.
- Tests required: cache hit, mtime invalidation, TTL expiry, concurrent read safety.
