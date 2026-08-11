# Requirements: perf-mutex-hotspots-2026-07

**Date**: 2026-07-01
**Type**: performance refactor (existing codebase)
**Complexity**: 2 — focused feature / targeted refactors across 3 files

## Problem Statement

Live pprof profiling on 2026-07-01 revealed three new mutex hotspots that together
account for ~1.07 trillion CPU cycles of mutex wait time:

1. **GoGitVCSReader thundering herd** — `session/unfinished/gogit_vcs_reader.go`.
   The 30s scan TTL equals the 30s scan interval. When a large repo takes >30s to scan,
   cycle N+1 starts before cycle N finishes. Both cycles see a stale cache entry for
   the same worktree path and both compete for `entry.mu` while executing expensive
   go-git graph walks (`AheadBehind` BFS, `diffShortstatUncached` blob reads,
   `HasUncommitted` index walk). Combined: ~1.05T cycles across 7,000+ contention events.

2. **CircularBuffer sync.Mutex blocks writers** — `session/circular_buffer.go`.
   `CircularBuffer` uses `sync.Mutex` for both reads (`GetRecent`) and writes (`Write`).
   The HTTP snapshot handler reads the full scrollback while the `streamLoop` goroutine
   writes incoming tmux output. Writers block for the duration of every read. Signal:
   14.4B cycles, 7,872 contention events.

3. **GitWorktree.IsDirty subprocess per check** — `session/git/worktree_git.go`.
   `ReviewQueuePoller.checkSession` calls `IsDirtyWithHint` which forks a `git status`
   subprocess on every poll for every session. No result caching. Signal: ~14.4B cycles
   combined, ~1,900 events.

## Baseline

- `make test` and `make ci` pass clean
- Mutex profile shows GoGitVCSReader operations dominating (677B cycles at rank 1)
- CircularBuffer shows 14.4B cycles / 7,872 events in stream path
- IsDirty shows 14.4B cycles / ~1,900 events in review queue poller
- Previous hotspots (log.Printf in hot loops, per-frame WebSocket debug log,
  ORM read-before-write) are fully absent — prior fixes held

## Users / Consumers

- Developers running stapler-squad with active Claude Code sessions
- All users are affected by scan latency (unfinished work tab refresh) and
  terminal output jank (CircularBuffer write latency)

## Success Metrics

- Mutex profile for `GoGitVCSReader` operations drops from ~1.05T cycles to near zero
  on cache-miss storms (singleflight coalesces concurrent misses into one computation)
- CircularBuffer write contention drops to zero when only one goroutine writes
  (streamLoop is already single-writer; RWMutex eliminates reader interference)
- IsDirty subprocess calls drop by ~5× within a 5s window per worktree
- `make test` continues to pass; `go test -race ./...` passes clean

## Appetite

Small (1–2 days). Three targeted, mechanical changes to existing files.
No new packages, no API changes, no migrations.

## Constraints

- Must not break existing tests
- Must pass `go test -race ./...` — no new data races
- `golang.org/x/sync` is already in go.mod; no new dependencies permitted
- Go 1.25.0 (project standard)

## Non-functional Requirements

- **Performance SLO**: mutex wait for GoGitVCSReader cache misses approaches 0 per scan cycle after fix
- **Scalability**: fix must work correctly under 4 concurrent scanner workers (current config)
- **Security classification**: internal
- **Data residency**: no special requirements

## Scope

### In Scope

1. `GoGitVCSReader.AheadBehind`, `DiffShortstat`, `HasUncommitted` — wrap cache-miss
   computation with `singleflight.Group` to coalesce concurrent misses
2. `CircularBuffer` — upgrade `sync.Mutex` to `sync.RWMutex`; read methods use RLock
3. `GitWorktree.IsDirty` / `IsDirtyWithHint` — add 5s TTL result cache per worktree path

### Out of Scope

- Browser / React performance (Phase 5b of profiling — separate effort)
- `streamViaControlMode.func3` block profile (125K events is expected for active sessions)
- Changing scanner concurrency (numWorkers = 4 is adequate)
- Any ORM or SQL changes
- `xsync.MapOf` adoption (sync.Map is sufficient for these access patterns on Go 1.25)

## Rabbit Holes

- **singleflight panic propagation**: if `AheadBehind` panics inside `Do`, the panic is
  broadcast to all waiting goroutines. Must ensure the inner function cannot panic, or
  wrap with recover.
- **go-git goroutine safety**: the reason `entry.mu` exists is go-git's packfile reader is
  not goroutine-safe. singleflight coalesces callers but does NOT remove the per-entry
  mutex — the mutex is still needed inside the `Do` body.
- **CircularBuffer zero-alloc guarantee**: after the RWMutex change, confirm the
  `testing.AllocsPerRun` baseline (if any test exists) still holds.
- **IsDirty cache invalidation**: the cache must be keyed by worktree path AND invalidated
  on session state transitions (e.g., when a session moves to "review" state). A stale
  dirty=false result that persists through a commit would be wrong.

## Alternatives Considered

- **Increase scan TTL** to 60s to reduce overlap: reduces fix-1 contention but doesn't
  eliminate it for very large repos; doesn't fix fix-2 or fix-3.
- **Reduce numWorkers to 1**: eliminates cross-worker contention entirely but degrades
  scan throughput for users with many repos.
- **Per-repo scan serialization lock in Scanner**: prevents concurrent scans of the same
  repo but doesn't help when two different repos have the same `entry.mu` contention
  pattern; singleflight is more targeted.

## Feasibility Risks

- singleflight `Do` key must match the cache key exactly — any mismatch causes concurrent
  misses to not coalesce. Need careful key construction testing.
- `sync.RWMutex.RLock` has higher overhead than `sync.Mutex.Lock` when there is no
  read/write contention. For `CircularBuffer` instances with only one reader, this is a
  slight regression. Acceptable given 7,872 contention events observed.

## Open Questions

- Does `IsDirtyWithHint` have a test that exercises the subprocess path directly?
  If so, the cache needs to be injectable (or the test needs to set a short TTL).
- Is there a benchmark for `CircularBuffer.Write` that asserts allocs/op? If not, add one.
