# Build vs. Buy: perf-mutex-hotspots-2026-07

Date: 2026-07-01

---

## Context: What the codebase actually looks like today

Before evaluating options, it is worth noting what has already been implemented:

- **CircularBuffer** (`session/circular_buffer.go`): already uses `sync.RWMutex`. `Write` takes a full write lock; `GetRecent`, `GetAll`, `Len`, `TotalBytesWritten`, and `WriteTo` all take read locks. Fix 2 is **already done**.
- **`scrollback.CircularBuffer`** (`session/scrollback/buffer.go`): also already uses `sync.RWMutex` (`mutex sync.RWMutex`), with `Append` using write lock and reads using `RLock`. Also already done.
- **`GitWorktree.IsDirty`** (`session/git/worktree_git.go`): has a TTL cache with `sync.RWMutex` (`isDirtyCacheMu`), a 15 s TTL (`IsDirtyCacheTTL`), a fast read-lock path, and a slow path that runs the subprocess outside any lock. Fix 3 is **already done**.
- **`GoGitVCSReader`** (`session/unfinished/gogit_vcs_reader.go`): uses `sync.Map` caches with TTL entries for `DiffShortstat`, `AheadBehind`, `CommitMessages`, and `reachableSet`. The thundering herd on cache miss (Fix 1) is the one remaining open hotspot.

---

## Fix 1 — Request coalescing for GoGitVCSReader (thundering herd)

### Problem

`GoGitVCSReader.DiffShortstat` (and its siblings `AheadBehind`, `CommitMessages`, `cachedReachableSet`) use a read-then-store pattern on `sync.Map`. When the TTL expires, all 4 concurrent scanner workers that share the same `GoGitVCSReader` instance can simultaneously observe a cache miss, each independently call the expensive `diffShortstatUncached` path, and then race to store the result. This is the classic thundering herd: N goroutines each pay the full cost of a go-git packfile walk for the same repo within a narrow window.

### Options

#### Option A — `golang.org/x/sync/singleflight` (RECOMMENDED)

- **Already in `go.mod`** at v0.20.0 (explicit dependency, not indirect). Zero new dependencies.
- Already used in this codebase: worktree agents in `.claude/worktrees/*/github/client.go` and `http_client.go` demonstrate the exact pattern (`singleflight.Group`, call `Do(key, fn)`).
- Fits the problem exactly: deduplicate concurrent in-flight calls for the same `worktreePath` key during TTL expiry windows. The scanner's 4 workers share one `GoGitVCSReader` instance, so they share the `singleflight.Group` automatically.
- The group can be embedded as a struct field on `GoGitVCSReader` — one field per method group (one for `diffStatGroup`, one for `aheadBehindGroup`, one for `commitMessagesGroup`).
- Error semantics are transparent: if the inflight call errors, all waiters receive the same error.
- **Limitation**: all waiters receive the same `(result, error)` value via `DoChan` or `Do`. For cache-TTL coalescing this is correct behavior — all callers asking about the same worktree in the same 30 s window want the same answer.

#### Option B — `github.com/tarndt/shardedsingleflight` (NOT RECOMMENDED)

- Not in `go.mod`; adds a new dependency.
- Sharding reduces lock contention when many *different* keys are in flight simultaneously. The scanner operates on at most `numWorkers` (4) paths concurrently; standard `singleflight` already handles this with negligible contention. Sharding buys nothing at this scale.
- Adds complexity for no measurable gain.

#### Option C — Increase TTL only (NOT SUFFICIENT)

- Increasing `diffStatCacheTTL` from 30 s to, say, 120 s reduces the frequency of thundering-herd windows but does not eliminate them. Any TTL-based cache without coalescing can still be stampeded if N goroutines all expire at the same moment (e.g. after a server restart when all caches are cold simultaneously).
- A longer TTL also degrades result freshness, which conflicts with the scanner's 30 s tick purpose.
- TTL alone is not a substitute for singleflight.

### Verdict: Use `golang.org/x/sync/singleflight` (Option A)

Add three `singleflight.Group` fields to `GoGitVCSReader` — one each for `DiffShortstat`, `AheadBehind`/`CommitMessages` (can share, or keep separate), and `cachedReachableSet`. Wrap the slow-path calls in `group.Do(key, func() (any, error) { ... })`. No new dependency; idiomatic; already proven in this codebase.

---

## Fix 2 — CircularBuffer lock upgrade

### Problem (as stated in requirements)

The requirements describe upgrading `CircularBuffer` from `sync.Mutex` to `sync.RWMutex`.

### Actual state

**This fix is already implemented.** Both `session/circular_buffer.go` and `session/scrollback/buffer.go` already declare `sync.RWMutex` and correctly use `RLock`/`RUnlock` for all read methods (`GetRecent`, `GetAll`, `Len`, `TotalBytesWritten`, `WriteTo`, `GetRange`, `GetAll`) and `Lock`/`Unlock` only for write methods (`Write`, `Append`, `Clear`, `Enable/DisableDiskFallback`, `Close`).

### Options (evaluated anyway for completeness)

#### Option A — stdlib `sync.RWMutex` (ALREADY DONE — RECOMMENDED)

- 1-line change from `sync.Mutex` to `sync.RWMutex` plus updating lock call sites. This is exactly what exists today.
- Correct for the access pattern: many concurrent `GetRecent`/`GetAll` readers (terminal streaming) vs. single-writer PTY data writes.
- No new dependency; idiomatic Go.

#### Option B — Replace with external ring-buffer library

- Libraries like `circularbuffer` or similar packages are not in `go.mod` and are not warranted. The existing implementation is purpose-built with disk-fallback, `io.WriterTo`, and `TotalBytesWritten` semantics that a generic package would not provide.
- Adding a dependency to replace working code adds maintenance burden and migration risk with no clear benefit.

#### Option C — Lock-free ring buffer

- Lock-free ring buffers (using `atomic.Uint64` for head/tail pointers) are only safe for single-producer single-consumer (SPSC) scenarios without a separate `count` field. The `CircularBuffer` supports multiple readers and a single writer with a wrap-detection `count` field — making a lock-free variant substantially more complex.
- The `Write` critical section is short (two `copy` calls + pointer arithmetic). Even under high PTY throughput, write contention is not a bottleneck compared to the reader lock traffic that `RWMutex` already handles efficiently.
- Not worth the complexity.

### Verdict: Already done; no action needed

---

## Fix 3 — IsDirty TTL cache

### Problem (as stated in requirements)

Caching the result of `git status --porcelain` subprocess to avoid redundant calls.

### Actual state

**This fix is already implemented.** `GitWorktree.IsDirtyWithHint` in `session/git/worktree_git.go` (lines 155–184) implements exactly the described pattern:

- `isDirtyCacheMu sync.RWMutex`, `isDirtyCache bool`, `isDirtyCacheTime time.Time` fields on `GitWorktree`.
- Fast read-lock path checks `!isDirtyCacheTime.IsZero() && time.Since(isDirtyCacheTime) < IsDirtyCacheTTL` (15 s TTL).
- Slow path runs the subprocess **outside any lock** so concurrent readers are not blocked during the 50–200 ms git subprocess wall time.
- Write lock stores result only if cache is still expired (handles write-race correctly, per the CLAUDE.md double-checked locking pattern).
- `InvalidateDirtyCache()` and `PrimeDirtyCacheAt()` helper methods for explicit invalidation and staggered warm-up.
- `claudeActive bool` hint in `IsDirtyWithHint` skips the subprocess entirely when Claude is actively generating output.

### Options (evaluated anyway for completeness)

#### Option A — Hand-rolled `sync.RWMutex` + `time.Time` TTL (ALREADY DONE — RECOMMENDED)

- Exactly what exists. Simple, zero dependencies, correctly handles the double-checked locking pattern.

#### Option B — `patrickmn/go-cache`

- Not in `go.mod`; adds a dependency.
- Provides TTL, expiration callbacks, and a full generics-based API. Useful when managing many independently-expiring keys. Here there is exactly one cache slot per `GitWorktree` struct — a map-based cache is over-engineering.
- `go-cache` uses a global `sync.RWMutex` over its entire map, which is identical in contention to the hand-rolled field mutex for single-key use.
- Not worth the dependency.

#### Option C — Use go-git directly instead of subprocess

- `go-git` is already in `go.mod` (`github.com/go-git/go-git/v5 v5.14.0`) and is used extensively in `GoGitVCSReader`.
- **Feasibility**: `GoGitVCSReader.HasUncommitted` (lines 268–378) implements exactly this: in-process dirty detection via index comparison + mtime stat walk, no subprocess. It is correct and already tested.
- **However**, `GitWorktree.IsDirty` and `GoGitVCSReader.HasUncommitted` serve different call sites. `IsDirty` is called from session lifecycle code (`PushChanges`, `CommitChanges`) where a single worktree is checked; `HasUncommitted` is called from the scanner. Replacing the `git status` subprocess with `go-git` in `IsDirty` would eliminate the need for the TTL cache entirely but requires ensuring `go-git`'s per-repo mutex is accessible from `GitWorktree` (currently it is only on the `GoGitVCSReader` instance).
- This is a valid future optimization but is a larger refactor than caching. The current TTL cache is sufficient for the stated hotspot fix.

### Verdict: Already done; no action needed

---

## Dependency Audit Summary

Current `go.mod` state relevant to these fixes:

| Package | Version | In go.mod? | Notes |
|---|---|---|---|
| `golang.org/x/sync` | v0.20.0 | Yes (explicit) | `singleflight` subpackage available, used in worktree agents |
| `github.com/go-git/go-git/v5` | v5.14.0 | Yes | Used in GoGitVCSReader; feasible for IsDirty replacement |
| `github.com/puzpuzpuz/xsync/v4` | v4.5.0 | Yes | Used in ShellRegistry; available for lock-free maps if needed |
| `github.com/tarndt/shardedsingleflight` | — | No | Not needed |
| `patrickmn/go-cache` | — | No | Not needed |

No new dependencies are required for any of the three fixes. Fix 1 (singleflight) uses an already-imported subpackage. Fixes 2 and 3 are already complete.

---

## Implementation Priority

| Fix | Status | Effort | Recommendation |
|---|---|---|---|
| Fix 1 — GoGitVCSReader singleflight | Open | Low (add 3 fields, wrap slow paths in `Do`) | Implement now |
| Fix 2 — CircularBuffer RWMutex | Already done | — | Verify, close |
| Fix 3 — IsDirty TTL cache | Already done | — | Verify, close |
