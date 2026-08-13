# Validation Plan: perf-mutex-hotspots-2026-07

**Date**: 2026-07-01
**Plan**: `project_plans/perf-mutex-hotspots-2026-07/implementation/plan.md`
**Source files reviewed**:
- `session/unfinished/gogit_vcs_reader.go` (implementation target, pre-change)
- `session/unfinished/gogit_vcs_reader_limits_test.go` (existing white-box tests + `initRepoInternal`)
- `session/unfinished/vcsreader_bench_test.go` (benchmarks, package `unfinished_test`)
- `session/unfinished/vcsreader_test.go` (black-box contract tests, package `unfinished_test`)
- `session/git_worktree_manager.go` (GitManager interface and GitWorktreeManager)
- `session/instance.go` (Pause/Resume call sites, `gitManager GitWorktreeManager` field)
- `session/git/worktree_git.go` (existing `InvalidateDirtyCache` implementation)

---

## 1. Requirements-to-Test Coverage Matrix

Each row maps one acceptance criterion to a concrete test, what it verifies, and edge cases that must be covered.

---

### Story 1.1.1 — Add singleflight.Group fields and hasUncommittedCache

**Acceptance Criteria** (from plan §Story 1.1.1):
- `GoGitVCSReader` has `aheadBehindSF`, `diffStatSF`, `hasUncommittedSF` singleflight.Group fields
- `GoGitVCSReader` has `hasUncommittedCache sync.Map` field
- `hasUncommittedEntry` struct defined with `result bool` and `expiry time.Time`
- Import `"golang.org/x/sync/singleflight"` present
- `GoGitVCSReader{}` zero value is valid

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 1a | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers` | Allocates `r := &GoGitVCSReader{}` (zero value, no constructor); calls `AheadBehind` on 4 concurrent goroutines — panics immediately if any singleflight field requires init. White-box package access forces compile failure if `aheadBehindSF` is absent. | Zero-value struct only; no `NewGoGitVCSReader()` call allowed. Must compile against the white-box package (`package unfinished`). |
| 1b | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` | Directly references `r.hasUncommittedCache.Store(dir, hasUncommittedEntry{result: !got, expiry: ...})` — **will not compile** if the field or struct is absent. Compile failure is the test. | Field names and types (`result bool`, `expiry time.Time`) are compile-enforced by the struct literal. |
| 1c | `session/unfinished/vcsreader_test.go` — `TestVCSReaderContractGoGit` (all sub-tests) | Allocates `&unfinished.GoGitVCSReader{}` and calls `AheadBehind`, `DiffShortstat`, `HasUncommitted`; nil-deref on first call if any field requires init. | All three methods exercised; if any singleflight group panics on first use from zero value, the contract suite catches it immediately. |

**Pass condition**: `go test ./session/unfinished/...` compiles and passes. No constructor call is required anywhere in the test suite.

---

### Story 1.1.2 — Wrap AheadBehind and DiffShortstat slow paths in singleflight.Do

**Acceptance Criteria** (from plan §Story 1.1.2):
- Fast path (TTL check) remains before the `Do` call for both methods
- `entry.mu.Lock()` + `defer entry.mu.Unlock()` inside `Do` (never explicit scattered unlocks)
- Deferred panic recovery inside `Do` converts panics to `error`; named returns required
- Result stored to cache inside `Do` is returned to all waiters
- Caller signatures unchanged

#### AC-1.1.2-A: Singleflight deduplication — N callers → consistent results

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 2a | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers` | 4 goroutines call `AheadBehind(dir, "main")` concurrently on a cold cache (zero-value struct); all 4 return identical `(0, 0, nil)`; `go test -race` passes. Single-commit repo → `base` branch at same commit → 0/0 is the correct result. | Cache must be cold at test start (zero-value struct has no entries). Worker count = 4 matches the 4 production scanner workers. Must run with `t.Parallel()` disabled for the goroutine coordination to be deterministic. |
| 2b | `session/unfinished/vcsreader_bench_test.go` — `BenchmarkDiffShortstatCached` | Post-bench assertion: `testing.AllocsPerRun(100, ...)` on warm `DiffShortstat` = 0 allocs/op. If `diffStatSF.Do` is called on every iteration instead of the fast path, allocs spike above 0 and the assertion fails. | `b.ResetTimer` after warm-up call; alloc assertion runs after the benchmark loop. |
| 2c | `session/unfinished/vcsreader_bench_test.go` — `BenchmarkFullScanCycle` | Combined `HasUncommitted` + `AheadBehind` + `DiffShortstat` completes without deadlock or test timeout. A deadlock inside any `Do` body would cause the benchmark to hang indefinitely (caught by Go test timeout). | Default `-timeout 30s`; any deadlock causes the process to be killed and the test to fail. |

#### AC-1.1.2-B: Panic recovery — panic inside Do → error returned, not crash

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 3a | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_AheadBehind_PanicRecovery` | Returns `(0, 0, non-nil error)` from a repo designed to trigger an early failure (non-existent path forces `openRepoEntry` to return an error; the test verifies the process does not crash and an error is returned). For a full panic-path test: create a temp dir with `.git/` but corrupt `HEAD` to point to a non-existent hash — this triggers a go-git panic during `Head()`. | **Named-return correctness**: if `recover()` sets `doErr` but the function uses unnamed returns, the recovered error is silently discarded and the method returns `(0, 0, nil)`. The test must assert `err != nil`. **Test binary crash** = recovery is broken. |
| 3b | Same test, run with `go test -timeout 10s -race` | If `entry.mu.Unlock()` is an explicit call (not deferred) and the panic fires while the mutex is held, the mutex is never released; subsequent `AheadBehind` calls deadlock. The 10s timeout catches the deadlock. | Run the test twice in sequence (two `AheadBehind` calls with the same repo path); the second call must not hang. If it hangs, the mutex was leaked by the first call's panic. |

#### AC-1.1.2-C: entry.mu inside Do, not outside

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 4a | `go test -race -count=3 ./session/unfinished/...` | The race detector reports no data races on `cachedRepo.mu` or any field inside `cachedRepo` across all tests. If `entry.mu` is acquired outside `Do`, the coalesced goroutines race on re-acquisition after `Do` returns. | `count=3` improves scheduling-interleaving probability. Race detector false-negative rate is low but non-zero on a single run. |

---

### Story 1.1.3 — Add TTL cache and singleflight to HasUncommitted

**Acceptance Criteria** (from plan §Story 1.1.3):
- `HasUncommitted` returns cached result immediately on cache hit
- On cache miss, exactly one goroutine performs the index walk; all share the result
- `Do` body has deferred panic recovery
- `entry.mu.Lock()` held inside `Do` for go-git phase, released before OS-only phase
- `hasUncommittedGoGitPhase` inner helper uses single `defer entry.mu.Unlock()` — no explicit unlocks
- Final result stored to `hasUncommittedCache` with 30s TTL

#### AC-1.1.3-A: TTL cache fast path

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 5a | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` (plan §Task 1.3.1b) | Cold call populates cache. Pre-populate `r.hasUncommittedCache` with the *inverted* result and a future expiry. Second call returns the inverted value — proves the cache was consulted, not the index. | Expiry must be future (`time.Now().Add(30 * time.Second)`). If TTL comparison uses `>=` instead of `After`, a same-second entry may not be found; use `time.Now().Add(1 * time.Hour)` to remove timing sensitivity. |
| 5b | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_HasUncommitted_CacheExpiryTriggersRecompute` | Pre-populate cache with an expired entry (`expiry: time.Now().Add(-1 * time.Second)`) and a result that contradicts actual repo state (repo is clean, cache says dirty=true). Verify second call returns `false` (actual state), not `true` (expired cached value). | Tests that expiry boundary (`time.Now().Before(e.expiry)`) behaves correctly at negative TTL. If the implementation silently ignores expiry, this test fails. |

#### AC-1.1.3-B: Singleflight deduplication for HasUncommitted

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 5c | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_HasUncommitted_SingleflightCollapsesParallelCallers` | 4 goroutines call `HasUncommitted(dir)` concurrently on a cold cache; all 4 return identical results with no data race. Pattern mirrors `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`. | Repo must be clean (expected result = `false`); all 4 workers must agree. Run with `go test -race`. |

#### AC-1.1.3-C: inner-helper defer pattern

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 5d | Code inspection: `grep 'entry.mu.Unlock()' session/unfinished/gogit_vcs_reader.go` must return 0 results inside any `hasUncommittedSF.Do` body outside `hasUncommittedGoGitPhase` | Explicit unlocks inside a `Do` body with `defer recover()` = deadlock on panic. Zero occurrences = structural guarantee. | Run as part of CI lint or pre-merge code review checklist. |
| 5e | `session/unfinished/gogit_vcs_reader_limits_test.go` — **new** `TestGoGitVCSReader_HasUncommitted_PanicRecovery` | Same panic-then-no-deadlock guarantee for `HasUncommitted`: corrupt repo triggers error/panic; method returns `(false, non-nil error)`; second call on same path does not hang. | Same approach as test 3a/3b adapted for `HasUncommitted`. |

---

### Story 1.2.1 — InvalidateDirtyCache on Resume and Pause

**Acceptance Criteria** (from plan §Story 1.2.1):
- `GitWorktreeManager.InvalidateDirtyCache()` calls `gm.worktree.InvalidateDirtyCache()` if worktree != nil; no-op if nil
- `GitManager` interface includes `InvalidateDirtyCache()`
- `Pause()` calls `i.gitManager.InvalidateDirtyCache()` after successful `transitionTo(Paused)`
- `Resume()` calls `i.gitManager.InvalidateDirtyCache()` after successful `transitionTo(Active)`

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 6a | Compile-time: `var _ GitManager = (*GitWorktreeManager)(nil)` at line 249 of `session/git_worktree_manager.go` | If `GitWorktreeManager` does not implement `InvalidateDirtyCache()`, the build fails immediately. | Must widen the `GitManager` interface **and** add the method to `GitWorktreeManager` in the same commit to avoid a broken intermediate state. |
| 6b | Any test double found by `grep -rn "GitManager" session/ --include="*.go"`: add no-op `InvalidateDirtyCache()` to each double | All test doubles implementing `GitManager` must be updated or the build fails. Current state: only `*GitWorktreeManager` satisfies `GitManager` (no mocks found in `session/`); `gitManager` in `instance.go` is a concrete `GitWorktreeManager` value, not interface-typed. | If new test doubles are added after this change, they must also implement `InvalidateDirtyCache()`. |
| 6c | `session/review_queue_uncommitted_changes_test.go` lines 103, 360 (existing) | Verifies the underlying `GitWorktree.InvalidateDirtyCache()` method still compiles and works correctly. The new manager wrapper delegates directly to this method; its correctness is pre-verified by these existing tests. | These tests do not need modification. |
| 6d | **New**: `session/git_worktree_manager_test.go` — `TestGitWorktreeManager_InvalidateDirtyCache_NilSafe` | Allocate `GitWorktreeManager{}` with no worktree (`gm.worktree == nil`); call `InvalidateDirtyCache()`; must not panic. | This is the nil-guard test. The guard `if gm.worktree == nil { return }` is the only protection; without it, this test panics. |
| 6e | **New**: `session/instance_lifecycle_test.go` — `TestInstance_Pause_InvalidatesDirtyCache` | Create a session with `gitManager.worktree` set to a real `GitWorktree` pointing at a temp repo; call `IsDirty()` to warm the cache (`isDirtyCacheTime != zero`); call `Pause()`; verify `gitManager.worktree.isDirtyCacheTime == zero` (cache was cleared). | `gitManager` in `Instance` is a concrete `GitWorktreeManager` (not interface), so internal state is inspectable. Session must be `Active` state before `Pause()` is called. |
| 6f | **New**: same file — `TestInstance_Resume_InvalidatesDirtyCache` | Same as 6e for the `Resume()` path: session is `Paused`; call `Resume()`; verify `isDirtyCacheTime == zero` after `transitionTo(Active)`. | Resume requires either a real tmux session or a stub `processManager` that bypasses tmux setup. |

**Pass condition**: `make build` exits 0; `go test ./session/...` passes including tests 6d, 6e, 6f.

---

### Story 1.4.1 — Race detector and make quick-check

**Acceptance Criteria** (from plan §Story 1.4.1):
- `make quick-check` exits 0
- `go test -race ./session/unfinished/...` exits 0
- `go test -race ./session/...` exits 0
- No new lint warnings

| # | File / Test | What it Verifies | Edge Cases |
|---|-------------|-----------------|------------|
| 7a | `go test -race -count=3 ./session/unfinished/...` | No data races on `GoGitVCSReader` fields, `cachedRepo.mu`, `sync.Map` stores/loads, or any goroutine spawned inside `Do`. `count=3` increases scheduling-interleaving probability. | Must include all new tests added in Story 1.3.1. |
| 7b | `go test -race -count=1 ./session/...` | No data races introduced by the `InvalidateDirtyCache` call sites in `Pause`/`Resume` (which execute after `i.stateMutex.Unlock()` in the current structure). | Verify the call does not require additional locking beyond what `Pause`/`Resume` already hold. |
| 7c | `make lint` | `//nolint:exhaustruct` present on the three `singleflight.Group` fields (go-exhaustruct linter rejects unkeyed struct literals for structs with unexported fields). No other new lint violations. | The three `singleflight.Group` fields are zero-value safe; exhaustruct annotation documents this is intentional. |

---

## 2. Test Sufficiency Verdict Per Story

### Story 1.1.1 — Add singleflight.Group fields and hasUncommittedCache
**SUFFICIENT**

Field existence and struct layout are compile-enforced by tests 1a and 1b. Zero-value safety is runtime-enforced by the existing contract suite (1c) and the new concurrency test (1a). No P1 criterion is left uncovered.

### Story 1.1.2 — Wrap AheadBehind and DiffShortstat slow paths in singleflight.Do
**SUFFICIENT with one required addition**

Singleflight coalescing: covered by test 2a. Allocation-free warm path: covered by 2b. Deadlock-on-timeout: covered by 2c. Race cleanliness: covered by 4a.

Required addition during implementation: `TestGoGitVCSReader_AheadBehind_PanicRecovery` (test 3a/3b). This test is specified in the plan but has no implementation body. It must be written during implementation. Its absence after implementation is a P1 gap.

### Story 1.1.3 — Add TTL cache and singleflight to HasUncommitted
**SUFFICIENT with three required additions**

Cache fast path: covered by 5a. Cache expiry boundary: covered by 5b (must be written). Singleflight deduplication: covered by 5c (must be written). Defer-pattern structural guarantee: covered by 5d (code inspection) + 5e (must be written).

Required additions during implementation: `TestGoGitVCSReader_HasUncommitted_CacheExpiryTriggersRecompute` (5b), `TestGoGitVCSReader_HasUncommitted_SingleflightCollapsesParallelCallers` (5c), and `TestGoGitVCSReader_HasUncommitted_PanicRecovery` (5e).

### Story 1.2.1 — Add InvalidateDirtyCache to GitWorktreeManager and call from Pause/Resume
**REQUIRES NEW TESTS**

Compile-time interface check covers method existence (6a). Nil guard requires test 6d. Pause/Resume call sites require tests 6e and 6f. All three must be written during implementation. Until they exist, this story has no runtime test coverage.

### Story 1.4.1 — Verification
**SUFFICIENT**

The verification step (7a, 7b, 7c) is a run-time gate; no new test files are required. Passes when all prior stories are implemented and their tests pass.

---

## 3. Required Test Implementations (Ordered by Priority)

Tests not yet in the repository that must be written during implementation.

| Priority | Test Name | File | Story | Blocks Merge? |
|----------|-----------|------|-------|---------------|
| P1 | `TestGoGitVCSReader_AheadBehind_PanicRecovery` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.1.2 | Yes — panic recovery AC has no coverage without this |
| P1 | `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.3.1 | Yes — singleflight deduplication is the core claim |
| P1 | `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.3.1 | Yes — HasUncommitted cache correctness |
| P2 | `TestGoGitVCSReader_HasUncommitted_SingleflightCollapsesParallelCallers` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.1.3 | Recommended — mirrors AheadBehind parallel test for HasUncommitted |
| P2 | `TestGoGitVCSReader_HasUncommitted_CacheExpiryTriggersRecompute` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.1.3 | Recommended — guards TTL boundary logic |
| P2 | `TestGoGitVCSReader_HasUncommitted_PanicRecovery` | `session/unfinished/gogit_vcs_reader_limits_test.go` | 1.1.3 | Recommended — same safety guarantee as AheadBehind |
| P2 | `TestGitWorktreeManager_InvalidateDirtyCache_NilSafe` | `session/git_worktree_manager_test.go` | 1.2.1 | Recommended — nil-guard for bonus fix |
| P3 | `TestInstance_Pause_InvalidatesDirtyCache` | `session/instance_lifecycle_test.go` | 1.2.1 | Optional at merge, required before ship |
| P3 | `TestInstance_Resume_InvalidatesDirtyCache` | `session/instance_lifecycle_test.go` | 1.2.1 | Optional at merge, required before ship |

---

## 4. Edge Case Register

| Edge Case | Risk if Not Tested | Test Coverage |
|-----------|--------------------|---------------|
| `entry.mu.Unlock()` explicit (not deferred) inside `Do` body + panic while mutex held | Permanent deadlock of all subsequent callers to that repo path | Test 3b: call `AheadBehind` twice on the same path after a panic; second call must not hang |
| Named returns absent from `Do` closure; `doErr = fmt.Errorf(...)` in recover defer is silently discarded | `AheadBehind` returns `(0, 0, nil)` after a go-git panic; caller cannot detect failure | Test 3a: assert `err != nil`; if named returns are broken, test fails because nil error is returned |
| singleflight re-broadcasts panics to all N waiters (recover in wrong scope) | All 4 scanner workers crash on a single malformed pack | Test 3a: single-caller panic; test 3b: second call must succeed (no deadlock, no crash propagation) |
| Cache TTL off-by-one: entry at exactly `time.Now()` treated as valid | Stale results returned at TTL expiry boundary | Test 5b: `expiry: time.Now().Add(-1 * time.Second)` forces past-expiry; second call must return actual repo state |
| `hasUncommittedGoGitPhase` locks `entry.mu` while `Do` body also holds it | Immediate deadlock on first `HasUncommitted` call | Test 5c (parallel callers) hangs on deadlock; race detector flags nested lock |
| `diffShortstatUncached` acquires `entry.mu` internally AND the `Do` wrapper also acquires it | Double-lock deadlock on `DiffShortstat` | Code inspection per plan §Task 1.1.2b note; `BenchmarkFullScanCycle` hangs if double-locked |
| Zero-value `GoGitVCSReader{}` with 4 concurrent callers on first use | Nil-deref or uninitialized map panic | Test 2a: 4 goroutines on zero-value struct with no prior use |
| `gitManager.worktree == nil` when `Pause()` calls `InvalidateDirtyCache` | Nil-pointer dereference in manager wrapper | Test 6d: `GitWorktreeManager{}` with no worktree; call `InvalidateDirtyCache()`; must not panic |
| `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` timing: `expiry` expires between Store and Load | Test flakiness due to system clock | Use `time.Now().Add(1 * time.Hour)` (1-hour TTL in test pre-population), not `30 * time.Second` |
| `AheadBehind` on a repo with no remote named "origin/main" | `resolveRef` error path vs panic | Covered by existing `TestGoGitVCSReader_AheadBehind_BehindCount` which uses a local `base` branch; the singleflight layer must propagate the error, not swallow it |

---

## 5. Race Detector Analysis for New Tests

### `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`

**Runnable under `go test -race`: YES**

- `sync.WaitGroup` usage is correct: `wg.Add(workers)` before goroutine launch, `wg.Wait()` before reading `results`.
- `results[idx]` — each goroutine writes to a distinct index; no concurrent writes to the same slot.
- `results[0]` read after `wg.Wait()` — safe; all goroutines have exited.
- `r := &GoGitVCSReader{}` shared across 4 goroutines — `singleflight.Group`, `sync.Map`, and `atomic` operations are all race-detector-safe. Against pre-change code (no singleflight), the race detector **will** surface data races on `entry.mu` contention — which is the intended regression signal.

### `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue`

**Runnable under `go test -race`: YES**

- Single-goroutine test; no concurrent access; race detector has nothing to flag.
- `r.hasUncommittedCache.Store(...)` — `sync.Map` is race-detector-safe.
- Does not call `t.Parallel()`; runs sequentially relative to other tests; no cross-test sharing.
- Against pre-change code: **compile failure** (field does not exist). Correct sentinel behavior — the test cannot even be compiled until Story 1.1.1 is implemented.

### `TestGoGitVCSReader_HasUncommitted_SingleflightCollapsesParallelCallers`

**Runnable under `go test -race`: YES** — identical concurrency pattern to the AheadBehind parallel test; same analysis applies.

### `TestGoGitVCSReader_AheadBehind_PanicRecovery`

**Runnable under `go test -race`: YES** — single or two-goroutine test; race detector will flag any unsynchronized access inside the panic-and-recover path.

---

## 6. Full Test Execution Checklist

Run in order; each must exit 0 before proceeding.

```bash
# 1. Build — compiles all code, runs proto generation, catches interface widening failures
make build

# 2. Race detector on the hot VCS package — all new + existing tests
go test -race -count=3 ./session/unfinished/...

# 3. Race detector on broader session package — catches InvalidateDirtyCache regressions
go test -race -count=1 ./session/...

# 4. Linter — nolint:exhaustruct on singleflight fields, no other new violations
make lint

# 5. Full CI pipeline gate
make quick-check
```

---

## 7. Readiness Gate

The following P1 criteria must each have a passing test before the PR is considered mergeable.

| ID | Criterion | Test | Status |
|----|-----------|------|--------|
| P1-A | `hasUncommittedCache` field and `hasUncommittedEntry` struct exist and are correctly typed | `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` (compile + runtime) | Must be written |
| P1-B | Singleflight coalescing for `AheadBehind`: N callers → consistent results, no race | `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers` + `go test -race` | Must be written |
| P1-C | Panic recovery: go-git panic → `error` returned, `entry.mu` released, subsequent callers unblocked | `TestGoGitVCSReader_AheadBehind_PanicRecovery` (two-call no-hang + non-nil error assertion) | Must be written |
| P1-D | HasUncommitted TTL cache fast path returns cached value within TTL | `TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue` | Must be written |
| P1-E | Zero-value safety: `GoGitVCSReader{}` works without constructor | Existing `TestVCSReaderContractGoGit` + new concurrency test | Existing coverage |
| P1-F | `go test -race ./session/unfinished/...` passes | All new tests + existing suite | Gate command |
| P1-G | `GitManager` interface widening compiles | `var _ GitManager = (*GitWorktreeManager)(nil)` at `git_worktree_manager.go:249` | Compile-time check |

**GATE: BLOCKED until P1-A through P1-D tests are written and passing.**
