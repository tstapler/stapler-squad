# Adversarial Review: perf-mutex-hotspots-2026-07

**Reviewer**: Adversarial Architecture Review  
**Date**: 2026-07-01  
**Plan**: `implementation/plan.md`  
**Verdict**: CONDITIONAL APPROVE — 0 blockers, 4 concerns, 4 minors

---

## BLOCKER — None

---

## CONCERN 1: Deferred panic-recovery variable shadowing in AheadBehind Do body

**Severity**: CONCERN  
**Location**: Task 1.1.2a, the `Do` body pseudo-code

The plan's code snippet in Task 1.1.2a has a subtle variable shadowing bug in the recover closure:

```go
defer func() {
    if r := recover(); r != nil {
        err = fmt.Errorf("go-git panic in AheadBehind: %v", r)  // ← assigns to WHAT `err`?
    }
}()
```

The `err` variable inside the `Do` closure is the same `err` variable declared with `:=` in the first error-checking `if` statement inside the `Do` body (`entry, err := ...`). A deferred closure captures the variable by reference, so assigning to `err` in `recover()` only works correctly if `err` is in scope at defer time AND the `Do` body returns after the deferred function runs.

The problem: if the panic occurs after `entry.mu.Lock()` and before `entry.mu.Unlock()`, the deferred recover will set `err`, but `entry.mu` is still held when `recover()` runs. The mutex will never be unlocked because the unlock calls are explicit (not deferred), leaving `entry.mu` permanently locked. **This is a deadlock on the next call to any method on this repo.**

The plan does note this in the "Note" paragraph at the end of Task 1.1.2a ("declare `var recoverErr error`... check `if recoverErr != nil { return nil, recoverErr }` after the defer"), but the note contradicts the code snippet shown above it. The implementation instruction is ambiguous — the shown code is wrong, the prose correction is right, but an implementer reading quickly will follow the code.

**Required fix**: The implementation must use `defer entry.mu.Unlock()` inside the `Do` body (not manual unlock chains) when a panic recovery defer is also present. Manual unlock + panic recovery cannot coexist safely. The note at the end of Task 1.1.2a must be promoted to the code sample itself, and the incorrect sample must be removed.

---

## CONCERN 2: HasUncommitted Do body — mutex still not deferred, same deadlock risk

**Severity**: CONCERN  
**Location**: Task 1.1.3b, Task 1.1.3c

`HasUncommitted` has 8+ explicit `entry.mu.Unlock()` calls across multiple early-return paths (counted from the real source: lines 281, 290, 295, 311, 324, 328, 335, 341, 354). The plan wraps all of this in a `Do` closure that also has a deferred panic recovery.

The same mutex-deadlock-on-panic risk from Concern 1 applies here. If a go-git panic fires at any of the many points where the mutex is held but has not yet been explicitly unlocked (e.g., inside the TreeWalker loop at line 305-312), the deferred `recover()` will catch the panic, but `entry.mu` will remain locked forever.

Task 1.1.3c acknowledges this and proposes converting Phase 1 to an inner helper with a single `defer entry.mu.Unlock()`. This is the correct fix, but the plan marks it as optional ("if inner-helper refactor adds > 5 minutes, keep the explicit unlock chains as-is"). The fallback option is unsafe: keeping explicit unlock chains while adding a panic recovery defer is not correct — it is exactly the scenario that produces a permanent mutex lock on panic.

**Required fix**: Task 1.1.3c's inner-helper refactor (or equivalent `defer entry.mu.Unlock()` approach) must be mandatory, not optional. The "keep as-is" escape hatch must be removed from the plan.

---

## CONCERN 3: singleflight error result is shared — stale-error amplification

**Severity**: CONCERN  
**Location**: All three `Do` call sites

`singleflight.Group.Do` returns `(v interface{}, err error, shared bool)`. When `shared == true`, the error returned to waiting goroutines is the exact same `error` value produced by the winning goroutine. This is fine for the normal case, but it has two implications not addressed in the plan:

1. **Transient errors become amplified**: If the winning goroutine hits a transient go-git error (e.g., a pack file read that transiently fails under I/O load), all N waiting goroutines receive that error simultaneously. The callers (scanner workers) will log N error entries. This is not a correctness issue — it is a log-noise concern — but it can make transient failures look catastrophic in monitoring. The plan's observability section says "no new log statements required" without acknowledging this amplification.

2. **Error result is not cached**: On a `Do` error return, the plan stores nothing in the TTL cache (correct). But the next scan cycle's 4 workers will all race into a new `Do` call simultaneously, since the cache is still empty. If the underlying error is persistent (e.g., corrupted pack), this pattern will generate 4 × (scan cycle count) calls to the failing go-git path per minute, bounded only by the scan interval. The singleflight deduplicates each burst to 1 actual call, but the burst repeats every cycle. This is acceptable behavior but should be called out explicitly.

**Required fix**: Add a comment at each `Do` call site noting the shared-error semantics and the absence of negative TTL caching. No code change required; documentation only.

---

## CONCERN 4: GitManager interface expansion breaks all test doubles and mocks

**Severity**: CONCERN  
**Location**: Task 1.2.1a — adding `InvalidateDirtyCache()` to `GitManager` interface

The `GitManager` interface currently has 18 methods. Adding `InvalidateDirtyCache()` as a new required method will break every struct that implements `GitManager` outside of `*GitWorktreeManager`. A search of the codebase shows test helpers and mock implementations of this interface.

The plan correctly adds the method to both `GitWorktreeManager` and the `GitManager` interface, but does not enumerate which other implementations exist. If any test file uses a hand-rolled struct satisfying `GitManager`, it will fail to compile.

**Required fix**: Before implementing Task 1.2.1a, search for all types that implement `GitManager` (`var _ GitManager = ...` lines or structs passed as `GitManager` arguments) and add `InvalidateDirtyCache()` stubs to all of them. The plan should include this as an explicit sub-task.

---

## MINOR 1: Test for singleflight coalescing does not actually verify single execution

**Severity**: MINOR  
**Location**: Task 1.3.1a

The proposed test (`TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`) verifies that 4 goroutines return consistent results. It does NOT verify that the `Do` body ran exactly once. Without a call counter, the test passes whether singleflight is working (1 execution) or broken (4 executions returning the same idempotent result). A regression that removes singleflight but preserves correctness would not be caught.

The plan acknowledges this ("Simplest approach: ...assert all return identical results") but the acknowledged weakness is worth flagging. A proper test should inject a call counter into the slow path (via an interface or package-level var) and assert `callCount == 1` after the WaitGroup completes.

---

## MINOR 2: `CommitMessages` and `cachedReachableSet` are excluded from singleflight despite the same thundering-herd risk

**Severity**: MINOR  
**Location**: Scope definition

`CommitMessages` acquires `entry.mu` twice (Phase 1 for head/base snapshot, Phase 3 for the log walk) and `cachedReachableSet` acquires it once on cache miss. Both are called by the same 4 scanner workers on the same repo path. They have TTL caches, but those caches do not prevent a thundering herd on cache miss — exactly the same problem as `AheadBehind` before this fix.

The plan explicitly limits scope to AheadBehind, DiffShortstat, and HasUncommitted. This is an appropriate MVP scope boundary. However, it means the thundering-herd risk on `CommitMessages` cache misses is not addressed and should be tracked as follow-on work.

---

## MINOR 3: `Stop()` / `Destroy()` transition path not covered by InvalidateDirtyCache bonus

**Severity**: MINOR  
**Location**: Epic 1.2 scope

The plan adds `InvalidateDirtyCache()` calls to `Pause()` (after `transitionTo(Paused)`) and `Resume()` (after `transitionTo(Active)`). It does not add it to `Destroy()` / the `transitionTo(Stopped)` path.

Looking at the actual `Destroy()` code in `instance.go`, the `Stopped` transition happens in an exit callback, not in a method that has easy access to `gitManager`. The IsDirty cache is less relevant after `Destroy()` since the session is gone, but the omission is slightly inconsistent with the stated goal of "invalidate on state transitions that may change dirty state." The plan's stated scope matches what it implements; this is noted for awareness, not as a required change.

---

## MINOR 4: `go.sum` does not contain `golang.org/x/sync v0.20.0` hash (only go.mod)

**Severity**: MINOR  
**Location**: Task 1.1.1b — `go.mod` already has v0.20.0 listed

`go.mod` lists `golang.org/x/sync v0.20.0`, and `go.sum` has two entries for v0.20.0. The existing `github/user_pr_cache.go` already imports `golang.org/x/sync/singleflight`, so the dependency is already active in the build. No `go get` step is required. The plan does not mention this explicitly, which is correct. This is a non-issue — just confirming the dependency claim is accurate.

Actually upon checking: this is a non-finding. Logged only to confirm the plan's dependency claim was verified.

---

## Summary

| # | Finding | Severity | Location |
|---|---------|----------|----------|
| 1 | Deferred recover + explicit unlock = permanent mutex deadlock on panic | CONCERN | Task 1.1.2a code sample |
| 2 | HasUncommitted: optional inner-helper must be mandatory to avoid same deadlock | CONCERN | Task 1.1.3c |
| 3 | Shared-error amplification from singleflight not documented | CONCERN | All Do call sites |
| 4 | GitManager interface expansion will break undiscovered test doubles | CONCERN | Task 1.2.1a |
| 5 | Coalescing test does not assert single execution; regression-blind | MINOR | Task 1.3.1a |
| 6 | CommitMessages / cachedReachableSet have same thundering-herd gap, not in scope | MINOR | Scope |
| 7 | Stop/Destroy path excluded from InvalidateDirtyCache; minor inconsistency | MINOR | Epic 1.2 scope |
| 8 | go.sum dependency claim verified (non-finding) | MINOR | Task 1.1.1b |

**Blockers**: 0  
**Concerns**: 4 (1 and 2 must be resolved before implementation; 3 and 4 are resolve-before-merge)  
**Minors**: 4 (addressable during implementation or tracked as follow-on)

### Mandatory pre-implementation actions
1. Fix the Task 1.1.2a code sample: use `defer entry.mu.Unlock()` inside the `Do` body when a panic recovery defer is present, OR prominently remove the incorrect sample and elevate the prose fix.
2. Make Task 1.1.3c's inner-helper refactor mandatory (remove the "keep as-is if > 5 min" escape hatch).
3. Add a sub-task to 1.2.1a: enumerate and stub `InvalidateDirtyCache()` on all types that implement `GitManager`.
