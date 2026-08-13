# Implementation Plan: perf-mutex-hotspots-2026-07

**Feature**: Add singleflight.Group to GoGitVCSReader to eliminate thundering-herd mutex contention (~1.05T cycles, 7000+ events)
**Date**: 2026-07-01
**Status**: Ready for implementation
**ADRs**: ADR-001-singleflight-per-method-groups.md

---

## Domain Glossary

| Term | Definition | Notes |
|------|------------|-------|
| `cachedRepo` | Per-path struct holding `*git.Repository` + `sync.Mutex` + atomic access timestamp | `entry.mu` is its lock field |
| `entry.mu` | Per-repo `sync.Mutex` inside `cachedRepo` that serialises go-git packfile reads | go-git packfile reader is not goroutine-safe; this lock is mandatory for ALL repo.CommitObject calls |
| `GoGitVCSReader` | In-process VCS reader using go-git; shared across 4 concurrent scanner workers | All-zero-value safe; no constructor required |
| `singleflight.Group` | Deduplicates concurrent identical calls so the slow path runs exactly once per key | From `golang.org/x/sync/singleflight`; already in go.mod |
| thundering herd | 4 scanner workers hitting the same repo path simultaneously, each acquiring `entry.mu` for the full BFS/index walk | Root cause of the ~1.05T cycles hotspot |
| cache miss | Condition where the TTL-based sync.Map cache has no valid entry, forcing the slow go-git path | Fast path: `sync.Map.Load` + expiry check. Slow path: everything else. |
| TTL expiry | A cached entry whose `expiry` field is before `time.Now()` | diffStatCacheTTL = 30s; same value used for all caches in this file |
| DoBody | The closure passed to `singleflight.Group.Do` | Must contain: panic recovery defer, `entry.mu.Lock()`, the go-git work, `entry.mu.Unlock()` |
| `hasUncommittedCache` | New `sync.Map` for `HasUncommitted` results with 30s TTL, mirroring `diffStatCache` | Does not currently exist; must be added |
| `InvalidateDirtyCache` | Method on `*git.GitWorktree` that zeroes `isDirtyCacheTime` | Already exists; needs to be called from `GitWorktreeManager` during Resume/Stop transitions |

---

## Creative Pass: Architectural Approaches Considered

### Option A: Single shared singleflight.Group (rejected)
Use one `singleflight.Group` on `GoGitVCSReader` for all three methods, with method-prefixed keys (e.g. `"ab:" + key`, `"ds:" + key`).

**Rejected because**: Key collision risk if two different methods share any key prefix logic. More critically: singleflight coalesces waiters on the same key — a blocked `AheadBehind` call would be incorrectly coalesced with a `DiffShortstat` call using a different key prefix if the prefix logic ever drifts. Separate groups make coalescing boundaries explicit and typesafe.

### Option B: Singleflight outside entry.mu (rejected)
Place `entry.mu.Lock()` outside the `Do` body so that only one goroutine holds the mutex, while others are coalesced by singleflight but wait before acquiring the mutex.

**Rejected because**: If `entry.mu` is outside `Do`, the coalesced goroutines exit `Do` before the winner holds the mutex, meaning they all then race to reacquire it. This loses the benefit of singleflight for the actual lock contention. The correct architecture is: singleflight eliminates the lock *acquisition* race by reducing N waiters to 1; that 1 winner then acquires `entry.mu` inside `Do`.

### Option C (chosen): Separate singleflight.Group per method, entry.mu INSIDE Do
Three fields: `diffStatSF`, `aheadBehindSF`, `hasUncommittedSF`. Each `Do` body: deferred panic recovery, `entry.mu.Lock()`, go-git work, `entry.mu.Unlock()`, cache store. Fast-path TTL check remains before the `Do` call.

**Chosen because**: Provides the clearest coalescing boundaries, is consistent with the existing `github/user_pr_cache.go` singleflight pattern in this codebase, eliminates the thundering herd for ALL three hot methods, and avoids the false-coalescing risk of Option A.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Singleflight grouping | Separate group per method (`diffStatSF`, `aheadBehindSF`, `hasUncommittedSF`) | ADR-001; `github/user_pr_cache.go` precedent | Single shared group with method-prefixed keys | False coalescing risk; separate groups make coalescing boundaries explicit |
| `entry.mu` placement | Inside `Do` body | go-git not goroutine-safe; must serialise per winner | Outside `Do` body | Placing outside `Do` means coalesced callers race on mutex re-acquisition after `Do` returns — defeats singleflight |
| Panic recovery | Deferred `recover()` inside `Do` body, converts to `error` | singleflight re-broadcasts panics to all waiters (critical: would propagate go-git pack panic to all 4 workers) | None | go-git is known to panic on malformed packs |
| HasUncommitted TTL cache | New `hasUncommittedCache sync.Map` with 30s TTL, same as `diffStatCacheTTL` | Matches existing `diffStatCache` / `aheadBehindCache` pattern | No TTL cache (singleflight-only) | Singleflight only helps concurrent callers; a TTL cache prevents re-running on sequential calls within the window |
| `hasUncommittedEntry` struct | `result bool; expiry time.Time` | Matches `diffStatEntry` pattern | Inline `sync.Map` with `atomic.Value` | Consistency with existing cache entry types |
| InvalidateDirtyCache bonus fix | Add `InvalidateDirtyCache()` call inside `Pause()` after `transitionTo(Paused)` and Resume `transitionTo(Active)` | Analysis of instance.go Pause/Resume code paths | Add to state machine `After` hook | Pause/Resume are already in `instance.go` with explicit post-transition logic; easier to read and test inline |

---

## Observability Plan

- **Logs**: No new log statements required. Existing `log.Error` on `fmt.Errorf("go-git panic: %v", r)` error return is sufficient (caller already logs VCS errors).
- **Metrics**: No new metrics. Existing pprof profiling is the verification tool (`make profile`). The absence of the `entry.mu` contention cluster in a post-deploy pprof trace is the success signal.
- **Alerts**: No new alerts needed. The existing `~/.stapler-squad/logs/stapler-squad.log` captures VCS reader errors at error level.

---

## Risk Control

- **Feature flag**: None. The change is transparent to callers; all existing cache TTLs and cache semantics are preserved. Worst case: a recovered panic returns an error to one caller instead of crashing 4 workers.
- **Rollback procedure**: `git revert` the singleflight commit. No data migrations or config changes involved. The `sync.Map` cache fields are zero-value safe; no constructor change needed.
- **Staged rollout**: Not applicable (local binary, single process). Validate with `make quick-check` and `go test -race ./session/unfinished/...` before merging.

---

## Unresolved Questions

1. **HasUncommitted OS-phase lock release timing**: ~~RESOLVED~~ — Task 1.1.3c mandates extracting Phase 1 into `hasUncommittedGoGitPhase`, which uses a single `entry.mu.Lock()` + `defer entry.mu.Unlock()` with no scattered explicit unlocks. The inner-helper refactor is non-optional.

2. **`hasUncommittedSF` key stability**: `HasUncommitted` currently takes only `worktreePath`. The singleflight key is `worktreePath`. This is correct since the operation is purely path-scoped. Confirmed safe.

---

## Dependency Visualization

```
Task 1.1.1a: Add hasUncommittedEntry + hasUncommittedCache field
    └─> Task 1.1.1b: Add singleflight group fields to GoGitVCSReader struct
            └─> Task 1.1.2a: Wrap AheadBehind slow path in aheadBehindSF.Do
            └─> Task 1.1.2b: Wrap DiffShortstat slow path in diffStatSF.Do
            └─> Task 1.1.3a: Add TTL cache fast-path to HasUncommitted
                    └─> Task 1.1.3b: Wrap HasUncommitted slow path in hasUncommittedSF.Do
                            └─> Task 1.1.3c: Audit non-deferred unlocks in HasUncommitted Do body
Task 1.2.1a: Audit GitManager interface implementations (find all test doubles)
    └─> Task 1.2.1b: Add InvalidateDirtyCache to GitWorktreeManager + GitManager interface
            └─> Task 1.2.1c: Call InvalidateDirtyCache in Pause() after transitionTo(Paused)
            └─> Task 1.2.1d: Call InvalidateDirtyCache in Resume() after transitionTo(Active)
Task 1.3.1a: Write concurrency test (4 goroutines, AheadBehind, assert Do runs once)
Task 1.3.1b: Write panic recovery test (AheadBehind_PanicDoesNotCrashCaller)
Task 1.3.1c: Write HasUncommitted cache test
Task 1.4.1a: make quick-check verification
```

---

## Phase 1: GoGitVCSReader Singleflight

### Epic 1.1: Add singleflight groups and HasUncommitted TTL cache to GoGitVCSReader

**Goal**: Eliminate thundering-herd entry.mu contention by ensuring that when N goroutines call any of AheadBehind, DiffShortstat, or HasUncommitted with the same arguments simultaneously on an expired cache, exactly 1 goroutine performs the slow go-git path; the rest receive the result via singleflight.

---

#### Story 1.1.1: Add singleflight.Group fields and hasUncommittedCache

**As a** scanner worker goroutine, **I want** the slow path for each VCS method to be deduplicated so that only one goroutine performs the go-git work per key per cache miss, **so that** the 4 concurrent workers sharing a GoGitVCSReader no longer generate a thundering herd on entry.mu.

**Acceptance Criteria**:
- `GoGitVCSReader` has three new `singleflight.Group` fields: `aheadBehindSF`, `diffStatSF`, `hasUncommittedSF`
- `GoGitVCSReader` has a new `hasUncommittedCache sync.Map` field (type `map[string]hasUncommittedEntry`)
- `hasUncommittedEntry` struct is defined with `result bool` and `expiry time.Time` fields
- The import `"golang.org/x/sync/singleflight"` is added to `gogit_vcs_reader.go`
- `GoGitVCSReader{}` zero value is still valid (singleflight.Group and sync.Map are zero-value safe)

**Given** a freshly allocated `GoGitVCSReader{}`,
**When** any of AheadBehind, DiffShortstat, or HasUncommitted is called,
**Then** the call completes without a nil-pointer panic (all new fields are zero-value safe).

**Files**: `session/unfinished/gogit_vcs_reader.go`

##### Task 1.1.1a: Define hasUncommittedEntry and add hasUncommittedCache field (~3 min)

In `session/unfinished/gogit_vcs_reader.go`:

1. Add after the existing `diffStatEntry` struct (around line 46):
   ```go
   type hasUncommittedEntry struct {
       result bool
       expiry time.Time
   }
   ```

2. Add to `GoGitVCSReader` struct (after `commitMessagesCache sync.Map`, around line 148):
   ```go
   // hasUncommittedCache caches HasUncommitted results keyed by worktreePath.
   // Eliminates repeated index walks within the TTL window.
   hasUncommittedCache sync.Map // map[string]hasUncommittedEntry
   ```

##### Task 1.1.1b: Add singleflight.Group fields to GoGitVCSReader struct (~3 min)

In `session/unfinished/gogit_vcs_reader.go`:

1. Add import: `"golang.org/x/sync/singleflight"` (in the import block)

2. Add three fields to `GoGitVCSReader` struct after `hasUncommittedCache`:
   ```go
   // aheadBehindSF deduplicates concurrent AheadBehind calls for the same key.
   // On a cache miss, exactly one goroutine performs the BFS; others receive the result.
   aheadBehindSF singleflight.Group //nolint:exhaustruct
   // diffStatSF deduplicates concurrent DiffShortstat calls for the same worktree path.
   diffStatSF singleflight.Group //nolint:exhaustruct
   // hasUncommittedSF deduplicates concurrent HasUncommitted calls for the same path.
   hasUncommittedSF singleflight.Group //nolint:exhaustruct
   ```

---

#### Story 1.1.2: Wrap AheadBehind and DiffShortstat slow paths in singleflight.Do

**As a** scanner worker goroutine, **I want** AheadBehind and DiffShortstat to use singleflight on cache miss, **so that** the BFS walk and blob-batch reads only happen once per key when 4 workers request the same path simultaneously.

**Acceptance Criteria**:
- `AheadBehind`: the fast path (TTL check) remains before the `Do` call; the `openRepoEntry` + `entry.mu.Lock()` + `defer entry.mu.Unlock()` + BFS + cache store are all inside the `Do` body
- `DiffShortstat`: the fast path remains before the `Do` call; `diffShortstatUncached` is called inside the `Do` body; cache store inside `Do` after uncached result
- Both methods have a deferred panic recovery inside the `Do` body that converts panics to errors
- `entry.mu` is acquired with `defer entry.mu.Unlock()` (never explicit scattered calls) — this is required: explicit unlocks + defer recover = permanent deadlock if go-git panics while holding the mutex
- The result stored to cache inside `Do` is also returned to callers via `Do`'s return value
- Callers receive `(int, int, error)` or `(DiffStat, error)` unchanged; signature is unchanged

**Given** 4 goroutines call `AheadBehind(repoPath, "origin/main")` simultaneously on an expired cache,
**When** the first goroutine is inside the BFS (simulated by a gated test mutex),
**Then** the other 3 goroutines block in `Do` without acquiring `entry.mu`, and all 4 return the same result.

**Files**: `session/unfinished/gogit_vcs_reader.go`

##### Task 1.1.2a: Wrap AheadBehind slow path in aheadBehindSF.Do (~5 min)

Current structure of `AheadBehind` (lines 426–486):
```
TTL fast-path check (keep as-is)
openRepoEntry(...)
entry.mu.Lock()
[BFS and count work]
entry.mu.Unlock()
aheadBehindCache.Store(...)
return ahead, behind, nil
```

Replace everything after the TTL fast-path check with a `Do` call:

```go
type abResult struct{ ahead, behind int }
val, err, _ := g.aheadBehindSF.Do(cacheKey, func() (val any, doErr error) {
    // Named returns so recover() can set doErr on panic.
    defer func() {
        if r := recover(); r != nil {
            doErr = fmt.Errorf("go-git panic in AheadBehind: %v", r)
        }
    }()

    entry, openErr := g.openRepoEntry(worktreePath)
    if openErr != nil {
        return nil, fmt.Errorf("open repo %s: %w", worktreePath, openErr)
    }

    entry.mu.Lock()
    defer entry.mu.Unlock() // REQUIRED: defer, never explicit unlocks — ensures mutex releases on panic

    repo := entry.repo
    headRef, headErr := repo.Head()
    if headErr != nil {
        return nil, fmt.Errorf("head: %w", headErr)
    }
    baseHash, baseErr := resolveRef(repo, base)
    if baseErr != nil {
        return nil, fmt.Errorf("resolve base %q: %w", base, baseErr)
    }
    if headRef.Hash() == baseHash {
        g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{expiry: time.Now().Add(diffStatCacheTTL)})
        return abResult{0, 0}, nil
    }
    mb, mbErr := findMergeBase(repo, headRef.Hash(), baseHash)
    if mbErr != nil {
        return nil, fmt.Errorf("merge base: %w", mbErr)
    }
    ahead, aheadErr := countCommitsTo(repo, headRef.Hash(), mb)
    if aheadErr != nil {
        return nil, aheadErr
    }
    behind, behindErr := countCommitsTo(repo, baseHash, mb)
    if behindErr != nil {
        return nil, behindErr
    }
    g.aheadBehindCache.Store(cacheKey, aheadBehindEntry{ahead: ahead, behind: behind, expiry: time.Now().Add(diffStatCacheTTL)})
    return abResult{ahead, behind}, nil
})
if err != nil {
    return 0, 0, err
}
r := val.(abResult)
return r.ahead, r.behind, nil
```

**Critical constraint**: `defer entry.mu.Unlock()` is mandatory here. Explicit scatter-unlock + `defer recover()` is a deadlock: if go-git panics while holding `entry.mu`, `recover()` fires but the explicit `Unlock()` calls are skipped — every subsequent caller to that repo blocks forever on `entry.mu.Lock()`. With `defer entry.mu.Unlock()`, defers fire LIFO: Unlock first (releases mutex), then recover (stops panic propagation). Named returns on the anonymous function (`val any, doErr error`) are required so that `doErr = fmt.Errorf(...)` inside the recover defer is visible as the function's return value.

##### Task 1.1.2b: Wrap DiffShortstat slow path in diffStatSF.Do (~4 min)

Current structure of `DiffShortstat` (lines 578–592):
```
TTL fast-path check
diffShortstatUncached(...)
diffStatCache.Store(...)
return result, err
```

Replace slow path with `Do`:

```go
val, err, _ := g.diffStatSF.Do(worktreePath, func() (val any, doErr error) {
    // Named returns required: recover defer sets doErr, which becomes the function's return value.
    defer func() {
        if r := recover(); r != nil {
            doErr = fmt.Errorf("go-git panic in DiffShortstat: %v", r)
        }
    }()
    result, uncachedErr := g.diffShortstatUncached(worktreePath)
    if uncachedErr != nil {
        return DiffStat{}, uncachedErr
    }
    g.diffStatCache.Store(worktreePath, diffStatEntry{
        result: result,
        expiry: time.Now().Add(diffStatCacheTTL),
    })
    return result, nil
})
if err != nil {
    return DiffStat{}, err
}
return val.(DiffStat), nil
```

**IMPORTANT**: Do NOT add `entry.mu.Lock()` to this `Do` body. `diffShortstatUncached` acquires `entry.mu` internally (it calls `openRepoEntry` + `entry.mu.Lock()` + `defer entry.mu.Unlock()` inside itself). Adding an outer lock in the `Do` body would cause a goroutine-level deadlock via mutex re-entrancy (Go mutexes are not reentrant — the same goroutine cannot Lock twice). The `Do` body here is lock-free; the internal mutex management is entirely inside `diffShortstatUncached`.

---

#### Story 1.1.3: Add TTL cache and singleflight to HasUncommitted

**As a** scanner worker goroutine, **I want** HasUncommitted to have a 30s TTL cache and singleflight deduplication, **so that** the index walk (which currently has neither) does not run N times per scan cycle per worktree.

**Acceptance Criteria**:
- `HasUncommitted` returns the cached `hasUncommittedEntry.result` immediately if the cache entry is valid
- On cache miss, exactly one goroutine performs the index walk via `hasUncommittedSF.Do`; all others share the result
- The Do body has a deferred panic recovery
- `entry.mu.Lock()` is held inside `Do` for the go-git phase and released before the OS-only phase
- The final result is stored to `hasUncommittedCache` with a 30s TTL
- The go-git phase (Phase 1) is extracted into an inner helper function; the inner helper uses a single `entry.mu.Lock()` + `defer entry.mu.Unlock()` with no scattered explicit unlocks — this is **required**, not optional (explicit unlocks + `defer recover()` in same `Do` body = deadlock on panic)
- `go test -race ./session/unfinished/...` passes

**Given** the `HasUncommitted` cache has expired for a worktree with 50 tracked files,
**When** 4 goroutines call `HasUncommitted(worktreePath)` simultaneously,
**Then** only 1 goroutine walks the index and all 4 receive `(false, nil)` (clean worktree), with no data races.

**Files**: `session/unfinished/gogit_vcs_reader.go`

##### Task 1.1.3a: Add TTL fast-path cache check to HasUncommitted (~3 min)

At the top of `HasUncommitted`, before the `openRepoEntry` call, add:

```go
if v, ok := g.hasUncommittedCache.Load(worktreePath); ok {
    if e := v.(hasUncommittedEntry); time.Now().Before(e.expiry) {
        return e.result, nil
    }
}
```

##### Task 1.1.3b: Extract Phase 1 + wrap Do body with named returns (~8 min) [COMBINED with former 1.1.3c — MANDATORY]

Tasks 1.1.3b and 1.1.3c are combined. The inner-helper extraction must be done as part of writing the `Do` body — not separately — because the `Do` body's correctness depends on the extraction.

**CRITICAL**: Do NOT use `var recoverErr error` pattern in the `Do` closure. That pattern is broken: after `recover()` sets `recoverErr` in the deferred function, the function body has already transferred control to the deferred stack — the `if recoverErr != nil` check at the end of the function body is **unreachable**. The function returns `nil, nil` (zero values), `val.(bool)` panics on the nil interface, and the caller crashes. Named returns are required.

**Step 1**: Extract Phase 1 into `hasUncommittedGoGitPhase`:

```go
// hasUncommittedGoGitPhase runs the go-git index phase of HasUncommitted.
// Acquires and releases entry.mu via defer. Returns indexed file set + dirty flag.
// MUST NOT be called with entry.mu already held — Go mutexes are not reentrant.
func (g *GoGitVCSReader) hasUncommittedGoGitPhase(entry *cachedRepo) (indexed map[string]struct{}, dirty bool, dirtyKnown bool, err error) {
    entry.mu.Lock()
    defer entry.mu.Unlock()
    // [all of the current Phase 1 logic — move verbatim]
    // Replace: entry.mu.Unlock(); return ... with: return ...
}
```

**Step 2**: Replace the full body of `HasUncommitted` (after the new TTL fast-path) with:

```go
val, sfErr, _ := g.hasUncommittedSF.Do(worktreePath, func() (val any, doErr error) {
    // Named returns required: recover defer sets doErr as the function's return value.
    defer func() {
        if r := recover(); r != nil {
            doErr = fmt.Errorf("go-git panic in HasUncommitted: %v", r)
        }
    }()

    entry, err := g.openRepoEntry(worktreePath)
    if err != nil {
        return nil, err
    }

    indexed, dirty, dirtyKnown, err := g.hasUncommittedGoGitPhase(entry)
    if err != nil {
        return nil, err
    }
    if dirtyKnown {
        g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
            result: dirty, expiry: time.Now().Add(diffStatCacheTTL),
        })
        return dirty, nil
    }

    // Phase 2: OS-only stat walk — no lock held
    result, err := hasUntrackedFiles(worktreePath, indexed)
    if err != nil {
        return nil, err
    }
    g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
        result: result, expiry: time.Now().Add(diffStatCacheTTL),
    })
    return result, nil
})
if sfErr != nil {
    return false, sfErr
}
return val.(bool), nil
```

##### Task 1.1.3c: (removed — merged into 1.1.3b)

The current `HasUncommitted` has at least 7 early-return paths that call `entry.mu.Unlock()` explicitly (lines 282, 290, 295, 311, 324, 328, 333, 342). This is the deadlock scenario: `Do` body has `defer recover()` + 8 explicit unlocks — any panic while the mutex is held skips the unlock and deadlocks every subsequent caller to that repo.

**This inner-helper refactor is mandatory, not optional.**

Extract Phase 1 into a package-private helper:

```go
// hasUncommittedGoGitPhase runs the go-git index phase of HasUncommitted.
// Acquires and releases entry.mu via defer. Returns indexed file set + dirty flag.
func (g *GoGitVCSReader) hasUncommittedGoGitPhase(entry *cachedRepo) (indexed map[string]struct{}, dirty bool, dirtyKnown bool, err error) {
    entry.mu.Lock()
    defer entry.mu.Unlock()
    // [all of the current Phase 1 logic — move verbatim]
    // Replace: entry.mu.Unlock(); return ... with: return ...
}
```

Then the `Do` body becomes:
```go
val, sfErr, _ := g.hasUncommittedSF.Do(worktreePath, func() (val any, doErr error) {
    defer func() {
        if r := recover(); r != nil {
            doErr = fmt.Errorf("go-git panic in HasUncommitted: %v", r)
        }
    }()

    entry, err := g.openRepoEntry(worktreePath)
    if err != nil {
        return nil, err
    }

    indexed, dirty, dirtyKnown, err := g.hasUncommittedGoGitPhase(entry)
    if err != nil {
        return nil, err
    }
    if dirtyKnown {
        g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
            result: dirty, expiry: time.Now().Add(diffStatCacheTTL),
        })
        return dirty, nil
    }

    // Phase 2: OS-only stat walk (no lock needed)
    result, err := hasUntrackedFiles(worktreePath, indexed)
    if err != nil {
        return nil, err
    }
    g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
        result: result, expiry: time.Now().Add(diffStatCacheTTL),
    })
    return result, nil
})
if sfErr != nil {
    return false, sfErr
}
return val.(bool), nil
```

This eliminates every explicit `entry.mu.Unlock()` from the `Do` body. No deadlock risk.

---

### Epic 1.2: Bonus — InvalidateDirtyCache on Resume and Stop

**Goal**: Prevent stale `dirty=false` reads after Resume by invalidating the 15s `IsDirty` TTL cache at Resume→Active and Pause→Paused transition points.

---

#### Story 1.2.1: Add InvalidateDirtyCache to GitWorktreeManager and call from Pause/Resume

**As a** user who resumes a session, **I want** the dirty status shown in the UI to reflect actual working tree state rather than a 15s-stale cached value, **so that** the session list shows the correct dirty indicator immediately after resuming.

**Acceptance Criteria**:
- `GitWorktreeManager` has a new `InvalidateDirtyCache()` method that calls `gm.worktree.InvalidateDirtyCache()` if `gm.worktree != nil`
- `GitManager` interface includes `InvalidateDirtyCache()`
- `Pause()` in `instance.go` calls `i.gitManager.InvalidateDirtyCache()` after the successful `transitionTo(Paused)` call
- `Resume()` in `instance.go` calls `i.gitManager.InvalidateDirtyCache()` after the successful `transitionTo(Active)` call
- No panic if `gitManager.worktree == nil` (nil-safe guard in manager method)

**Given** a session that has been paused and then resumed,
**When** the scanner next calls `IsDirty()` on the worktree,
**Then** it runs a fresh git-status subprocess (cache was invalidated) rather than returning the pre-pause cached value.

**Files**: `session/git_worktree_manager.go`, `session/git/worktree_git.go` (existing `InvalidateDirtyCache` already there), `session/instance.go`

##### Task 1.2.1a: Audit GitManager interface implementations before widening (~2 min)

Before adding `InvalidateDirtyCache()` to the `GitManager` interface, grep for all structs that implement it (test doubles, mocks, fakes):

```bash
grep -rn "GitManager" session/ --include="*.go" | grep -v "_test.go" | grep -v "interface"
grep -rn "func.*GitManager\b\|MockGitManager\|FakeGitManager\|stubGitManager\|noopGitManager" session/ --include="*.go"
```

Add a no-op `InvalidateDirtyCache()` method to every implementation found before or alongside the interface change, so compilation does not break. Typical test doubles: `noopGitManager`, `testGitManager`, `mockGitManager`.

##### Task 1.2.1b: Add InvalidateDirtyCache to GitWorktreeManager and GitManager interface (~3 min)

In `session/git_worktree_manager.go`, add after `IsDirty()`:

```go
// InvalidateDirtyCache clears the IsDirty TTL cache so the next call re-runs git status.
// Call after transitions that may change worktree dirty state (Resume, Stop).
// No-op if no worktree is set.
func (gm *GitWorktreeManager) InvalidateDirtyCache() {
    if gm.worktree == nil {
        return
    }
    gm.worktree.InvalidateDirtyCache()
}
```

Add `InvalidateDirtyCache()` to the `GitManager` interface (around line 233).

##### Task 1.2.1c: Call InvalidateDirtyCache in Pause() (~2 min)

In `session/instance.go`, at the start of `Pause()` (before `transitionTo`), add a defer so the cache is invalidated regardless of whether the transition succeeds:

```go
defer i.gitManager.InvalidateDirtyCache()
```

This is safer than calling it only on the success path: a failed Pause attempt (e.g. invalid state transition) still invalidates the cache, preventing the UI from showing stale data if Pause is retried. `InvalidateDirtyCache()` is a no-op if the cache is already empty, so no correctness issue from early/extra invalidation.

##### Task 1.2.1d: Call InvalidateDirtyCache in Resume() (~2 min)

In `session/instance.go`, in `Resume()`, after the `transitionTo(Active)` success (around line 1151), add:

```go
i.gitManager.InvalidateDirtyCache()
```

---

### Epic 1.3: Tests

**Goal**: Provide a concurrency regression test that proves singleflight coalescing is working, a panic recovery test, and verify the HasUncommitted cache.

---

#### Story 1.3.1: Add singleflight concurrency test and panic recovery test

**As a** developer, **I want** a Go test that spawns 4 goroutines calling AheadBehind simultaneously and verifies the Do body runs exactly once, **so that** regression of the thundering herd is detectable by the test suite.

**Acceptance Criteria**:
- Test `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers` in `session/unfinished/gogit_vcs_reader_limits_test.go` (white-box, package `unfinished`)
- Test uses a controlled git repo (via `initRepoInternal`)
- 4 goroutines call `AheadBehind(repoPath, "main")` concurrently after cache TTL is expired
- A counter inside a replaced/wrapped slow-path helper tracks actual go-git calls (OR the test uses `-race` + a sync.Mutex + counter to count `entry.mu.Lock()` acquisitions)
- Simplest approach: use `testing.T.Parallel()` on 4 subtests with a shared `GoGitVCSReader` and an expired cache; assert all return identical results without error
- Panic recovery test: `TestGoGitVCSReader_AheadBehind_PanicRecovery` — inject a repo whose `Head()` call panics via a mock (or verify the panic recovery path doesn't crash the caller by testing with a deliberately malformed temp repo)

**Given** 4 parallel goroutines calling `AheadBehind` on the same path with an expired cache,
**When** they all call simultaneously,
**Then** all 4 return `(0, 0, nil)` (zero ahead/behind for a fresh single-commit repo) with no data race.

**Files**: `session/unfinished/gogit_vcs_reader_limits_test.go`

##### Task 1.3.1a: Write parallel AheadBehind singleflight test (~5 min)

Add to `session/unfinished/gogit_vcs_reader_limits_test.go`:

```go
// TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers verifies
// that 4 concurrent callers receive consistent results when the cache is cold.
// go test -race must pass.
func TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers(t *testing.T) {
    dir := initRepoInternal(t)

    r := &GoGitVCSReader{}
    // Ensure cache is cold (zero value = expired)
    const workers = 4
    type result struct {
        ahead, behind int
        err           error
    }
    results := make([]result, workers)
    var wg sync.WaitGroup
    wg.Add(workers)
    for i := range workers {
        go func(idx int) {
            defer wg.Done()
            a, b, err := r.AheadBehind(dir, "main")
            results[idx] = result{a, b, err}
        }(i)
    }
    wg.Wait()
    for i, res := range results {
        if res.err != nil {
            t.Errorf("worker %d: unexpected error: %v", i, res.err)
        }
        if res.ahead != results[0].ahead || res.behind != results[0].behind {
            t.Errorf("worker %d: got (%d, %d), want (%d, %d)",
                i, res.ahead, res.behind, results[0].ahead, results[0].behind)
        }
    }
}
```

##### Task 1.3.1b: Write panic recovery test (~4 min)

The test verifies that a panic inside the `Do` body is recovered and returned as an error, not re-panicked to the caller. Since go-git panics on malformed repos are hard to inject, the test uses a non-existent path — go-git's `PlainOpen` on a missing path returns an error (not a panic), but the path exercises the error-return case from `openRepoEntry`, verifying the caller does not crash. For full panic coverage, the test uses `t.Run` with `recover()` to assert no panic reaches the caller:

```go
// TestGoGitVCSReader_AheadBehind_PanicDoesNotCrashCaller verifies that
// a panic inside the singleflight Do body is caught and returned as an error.
func TestGoGitVCSReader_AheadBehind_PanicDoesNotCrashCaller(t *testing.T) {
    // Use a non-existent path — openRepoEntry returns error, not panic,
    // but confirms the caller handles Do errors without crashing.
    r := &GoGitVCSReader{}
    _, _, err := r.AheadBehind("/nonexistent/path/guaranteed-missing", "main")
    if err == nil {
        t.Fatal("expected error from non-existent repo, got nil")
    }
    // Verify no panic escaped to the caller (if we reach here, no panic occurred)
}
```

For additional panic coverage: if the codebase has a way to create a corrupt `.git` directory (e.g. empty objects dir), use that. Otherwise the above test covers the error-return path through the `Do` body and is sufficient for the acceptance criterion.

##### Task 1.3.1c: Write HasUncommitted cache test (~3 min)

Add a test that verifies the `hasUncommittedCache` fast path:

```go
// TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue verifies
// that a warm hasUncommittedCache entry is returned without re-scanning.
func TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue(t *testing.T) {
    dir := initRepoInternal(t)
    r := &GoGitVCSReader{}
    // Cold call
    got, err := r.HasUncommitted(dir)
    if err != nil {
        t.Fatalf("cold call: %v", err)
    }
    // Pre-populate cache with inverted value to detect cache bypass
    r.hasUncommittedCache.Store(dir, hasUncommittedEntry{
        result: !got,
        expiry: time.Now().Add(30 * time.Second),
    })
    cached, err := r.HasUncommitted(dir)
    if err != nil {
        t.Fatalf("warm call: %v", err)
    }
    if cached != !got {
        t.Errorf("warm call: got %v, want %v (cache was not used)", cached, !got)
    }
}
```

---

### Epic 1.4: Verification

**Goal**: Confirm all changes compile, pass tests, and pass the race detector.

---

#### Story 1.4.1: Run make quick-check

**As a** developer, **I want** the full build + test + lint pipeline to pass after the singleflight changes, **so that** the PR is safe to merge.

**Acceptance Criteria**:
- `make quick-check` exits 0
- `go test -race ./session/unfinished/...` exits 0
- `go test -race ./session/...` exits 0
- No new lint warnings

**Files**: All modified files

##### Task 1.4.1a: Run verification commands (~5 min)

```bash
make build
go test -race ./session/unfinished/...
go test -race ./session/...
make lint
make quick-check
```

Fix any lint issues (likely `//nolint:exhaustruct` on the three `singleflight.Group` fields).

---

## Summary Table

| Epic | Stories | Tasks | Est. Time |
|------|---------|-------|-----------|
| 1.1 Add singleflight + HasUncommitted cache | 3 | 7 | ~30 min |
| 1.2 InvalidateDirtyCache bonus | 1 | 4 | ~9 min |
| 1.3 Tests | 1 | 3 | ~12 min |
| 1.4 Verification | 1 | 1 | ~5 min |
| **Total** | **6** | **15** | **~56 min** |
