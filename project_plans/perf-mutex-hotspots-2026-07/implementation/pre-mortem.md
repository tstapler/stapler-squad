# Pre-Mortem: perf-mutex-hotspots-2026-07

**Date**: 2026-07-01
**Status**: Gate analysis — prior to implementation
**Reviewer role**: Pre-mortem agent

---

## Executive Summary

The plan is architecturally sound for P1 risks in AheadBehind and DiffShortstat. Two implementation-path P1 risks exist in HasUncommitted: (1) an intermediate task (1.1.3b) uses a recover pattern that silently swallows panics and causes a downstream `nil.(bool)` assertion panic if 1.1.3c is not completed, and (2) the DiffShortstat `Do` body plan ambiguity could lead to a double-lock deadlock if a developer adds `entry.mu.Lock()` before calling `diffShortstatUncached`. Both are mitigable by strengthening task sequencing and clarifying the DiffShortstat constraint. One P2 (stale dirty state after failed Pause) is real. All P3s are not real risks.

---

## P1 Failure Modes

### P1-A: `defer entry.mu.Unlock()` + `defer recover()` LIFO order in AheadBehind

**Scenario**: Go panics while `entry.mu` is held inside the `Do` body. Do the defers fire in the correct order?

**Trace**:

In the proposed AheadBehind `Do` body (Task 1.1.2a):

```go
func() (val any, doErr error) {
    defer func() { /* recover sets doErr */ }()  // defer slot 1 — registered first
    
    entry, _ := g.openRepoEntry(...)
    entry.mu.Lock()
    defer entry.mu.Unlock()                       // defer slot 2 — registered second
    
    // ... go-git work that might panic ...
}
```

Go executes defers LIFO (last-in, first-out). Defer slot 2 (`entry.mu.Unlock()`) was registered last, so it fires **first**. Defer slot 1 (`recover()`) fires **second**.

Execution on panic:
1. `entry.mu.Unlock()` fires → mutex is released ✓
2. `recover()` fires → catches the panic, sets `doErr` ✓
3. `Do` returns `(nil, doErr)` to callers ✓

**Verdict**: Not a real risk. The LIFO ordering is exactly correct for this use case. The mutex is unlocked before the recover fires. The plan's explanation (Task 1.1.2a critical constraint section) correctly describes this.

**Mitigation**: None required. The pattern is safe as written.

---

### P1-B: `val.(abResult)` panic if `Do` returns a non-nil error

**Scenario**: `Do` returns `(nil, err)` — can the `val.(abResult)` assertion at the call site panic?

**Proposed code**:
```go
val, err, _ := g.aheadBehindSF.Do(cacheKey, func() (val any, doErr error) { ... })
if err != nil {
    return 0, 0, err       // ← returns before assertion
}
r := val.(abResult)        // ← only reached when err == nil
return r.ahead, r.behind, nil
```

When `Do` returns a non-nil error, the body returned `(nil, someErr)` or singleflight propagated a panic-turned-error. In either case `err != nil` is true and the function returns before reaching `val.(abResult)`.

When `Do` returns `err == nil`, the body must have returned `(abResult{...}, nil)` — a non-nil `any` containing an `abResult` value. The assertion is safe.

**Verdict**: Not a real risk. The `if err != nil` guard is always evaluated before the type assertion. The only way to reach the assertion is `err == nil`, which guarantees `val` is a valid `abResult`.

**Mitigation**: None required.

---

### P1-C: `val.(bool)` panic in HasUncommitted — general analysis

**Scenario**: Can `val.(bool)` at the `Do` call site panic?

The `Do` call site (Task 1.1.3c final form):
```go
if sfErr != nil {
    return false, sfErr
}
return val.(bool), nil
```

Same guard pattern as P1-B. `val.(bool)` is only reached when `sfErr == nil`.

When `sfErr == nil`, the `Do` body must have returned a `bool` as `any`. In Go, `return dirty, nil` where `dirty` is a `bool` packages the bool into an interface value — `val.(bool)` succeeds.

**Verdict**: Not a real risk in the final (1.1.3c) implementation. The guard is always evaluated first.

**Mitigation**: See P1-D below for the intermediate-state risk.

---

### P1-D: Task 1.1.3b's recover pattern silently swallows panics, causing downstream nil assertion panic (REAL RISK)

**Scenario**: A developer implements Task 1.1.3b before Task 1.1.3c, or implements 1.1.3b incorrectly. Task 1.1.3b as written uses an unnamed closure with a non-named-return recover pattern:

```go
val, sfErr, _ := g.hasUncommittedSF.Do(worktreePath, func() (any, error) {
    var recoverErr error
    defer func() {
        if r := recover(); r != nil {
            recoverErr = fmt.Errorf("go-git panic in HasUncommitted: %v", r)
        }
    }()
    
    // ... go-git work ...
    
    // At the bottom:
    if recoverErr != nil {
        return nil, recoverErr   // ← THIS LINE IS NEVER REACHED AFTER A PANIC
    }
    return result, nil
})
```

**Root cause**: When a panic occurs and `recover()` fires inside a deferred function, execution does NOT resume at the bottom of the enclosing function body. The enclosing function returns immediately with its zero-value named returns — or, for unnamed returns, with `nil, nil`. The `if recoverErr != nil` check at the bottom of the body is unreachable after a panic.

**Consequence**: `Do` returns `(nil, nil)`. `sfErr` is nil. The call site reaches `return val.(bool), nil` with `val == nil`. In Go, a type assertion on a nil interface value panics:
```
interface conversion: interface is nil, not bool
```

This panic is NOT inside a `Do` body with a recover — it panics in the caller of `HasUncommitted`, propagates up through the scanner worker goroutine, and crashes it. With 4 workers, this could crash all 4 if they all call HasUncommitted on a malformed repo simultaneously.

**Task 1.1.3c is the correct fix**: Named returns on the closure (`func() (val any, doErr error)`) mean that `doErr = fmt.Errorf(...)` inside the recover defer mutates the actual return value. This is what makes the pattern work correctly. The plan correctly marks 1.1.3c as MANDATORY, but 1.1.3b provides an intermediate scaffold that is subtly broken and could be shipped as-is.

**Verdict**: Real risk. The defect exists in the intermediate Task 1.1.3b scaffold, which is broken and could be mistaken for a complete implementation. If 1.1.3b is shipped without 1.1.3c, a go-git panic in HasUncommitted causes a nil interface assertion panic in the caller, crashing scanner workers.

**Required mitigation**: 
1. Mark Task 1.1.3b as "scaffolding only — DO NOT MERGE without 1.1.3c complete." 
2. Better: collapse 1.1.3b and 1.1.3c into a single task so the broken intermediate form is never committed to main.
3. The PR gate test `go test -race ./session/unfinished/...` will not catch this unless a test deliberately triggers a panic in a HasUncommitted Do body. Add Task 1.3.1b (panic recovery test) coverage to HasUncommitted specifically.

---

### P1-E: DiffShortstat `Do` body ambiguity — potential double-lock deadlock (REAL RISK)

**Scenario**: Task 1.1.2b wraps `diffShortstatUncached` in a `Do` body. The task description says:

> "check whether `entry.mu` is exposed to the `Do` body. If `diffShortstatUncached` acquires and releases `entry.mu` internally, no `defer entry.mu.Unlock()` is needed in this `Do` body. **If it does not**, add the same `entry.mu.Lock()` + `defer entry.mu.Unlock()` pattern."

The actual `diffShortstatUncached` code (lines 614 and 758 of `gogit_vcs_reader.go`) acquires `entry.mu.Lock()` twice, with an explicit `entry.mu.Unlock()` between them (lines 686 and 784). The function fully manages its own mutex lifecycle.

If a developer reads the ambiguous "if it does not" branch and adds `entry.mu.Lock()` to the `Do` body AND calls `diffShortstatUncached`, the goroutine double-locks `entry.mu`:

1. `Do` body: `entry.mu.Lock()` ← first lock acquired
2. `diffShortstatUncached`: `entry.mu.Lock()` ← second lock on same goroutine

Go's `sync.Mutex` is not reentrant. The second `Lock()` call blocks forever waiting for itself to release. The goroutine deadlocks permanently. All subsequent callers to any method on this `cachedRepo` also deadlock.

**Verdict**: Real risk. The ambiguous "check whether" instruction in Task 1.1.2b leaves room for a developer to add the outer lock incorrectly. The actual code clearly shows `diffShortstatUncached` manages `entry.mu` internally.

**Required mitigation**: Replace the ambiguous check-and-decide instruction in Task 1.1.2b with an explicit constraint: "Do NOT add `entry.mu.Lock()` to the DiffShortstat `Do` body. `diffShortstatUncached` acquires and releases `entry.mu` internally (verified: lines 614 and 758 of gogit_vcs_reader.go). Adding an outer lock causes permanent deadlock via mutex re-entrancy." The panic recovery defer is still needed in the Do body (go-git can panic during blob reads); only the explicit lock/unlock must be omitted.

---

## P2 Failure Modes

### P2-A: Singleflight stale cache — window between Do return and cache store

**Scenario**: Could singleflight return a "stale" result past the TTL to a second caller?

Singleflight does not maintain its own cache of results between calls. The `singleflight.Group` map only holds in-flight calls. Once `Do` returns, the key is removed from the group. A subsequent call with the same key after `Do` completes starts a new flight if the TTL cache check fails.

The concern might be: could caller A get a TTL cache hit while caller B (the flight winner) has just computed a fresh value but not yet stored it to the TTL cache? No — the cache store happens inside the `Do` body, before `Do` returns. By the time any caller receives the result from `Do`, the fresh value is already in the TTL cache. Subsequent callers hit the fresh TTL cache entry.

**Verdict**: Not a real risk. Cache store is inside `Do`; singleflight only coalesces concurrent flights; there is no window where singleflight could return a cached result that is staler than the TTL.

**Mitigation**: None required.

---

### P2-B: Stale dirty state after failed or partial Pause (REAL RISK)

**Scenario**: `Pause()` fails midway — for example, `CommitChanges` fails (line 994 of instance.go: `return i.combineErrors(errs)` is called before `transitionTo(Paused)`). The plan places `i.gitManager.InvalidateDirtyCache()` after `transitionTo(Paused)` succeeds (Task 1.2.1c). On a partial Pause, `InvalidateDirtyCache()` is never called.

Looking at the actual Pause code path: if `CommitChanges` fails, the function returns at line 995 before the `transitionTo(Paused)` call at line 1037. The invalidation is never triggered. The IsDirty TTL cache (15s window, separate from the 30s singleflight TTL cache) retains its pre-pause value. If the user retries the pause or the scanner runs, it may show a stale dirty indicator.

Additionally, if `Remove()` or `Prune()` fail (lines 1016-1029), the function also returns early. In these cases, the worktree may be in a partially dismantled state, and the cached dirty value is no longer meaningful.

**Verdict**: Real risk. Stale dirty state persists until the 15s IsDirty TTL expires. Not data loss or deadlock, but users see incorrect dirty indicators in the UI for up to 15 seconds after a failed Pause attempt. Given the typical Pause failure modes (network issues, lock contention), this could be a recurring annoyance.

**Required mitigation**: Call `InvalidateDirtyCache()` at ALL early return paths in `Pause()`, not only after successful `transitionTo(Paused)`. A simple approach: call it unconditionally after `StopController()` at the top of `Pause()` (line 981), where it has no side effects but ensures the cache is always invalidated once Pause begins. Alternatively, use a `defer i.gitManager.InvalidateDirtyCache()` at the top of `Pause()` to guarantee it fires regardless of exit path.

---

### P2-C: `hasUncommittedGoGitPhase` returns early (dirty=true, dirtyKnown=true) — type assertion safety

**Scenario**: When `hasUncommittedGoGitPhase` returns `(nil, true, true, nil)` (dirty detected early, indexed file set not needed for Phase 2), the `Do` body executes:

```go
if dirtyKnown {
    g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
        result: dirty, expiry: time.Now().Add(diffStatCacheTTL),
    })
    return dirty, nil   // dirty is bool → returned as any
}
```

`dirty` is a `bool` value type. When returned as `any`, Go boxes it: the interface is non-nil with dynamic type `bool`. `val.(bool)` at the call site succeeds.

**Verdict**: Not a real risk. The `bool` return is correctly boxed.

**Mitigation**: None required.

---

## P3 Failure Modes

### P3-A: Mutex re-entrancy in `diffShortstatUncached` called from its own `Do` body

**Note**: This was elevated to P1-E above because the ambiguity in Task 1.1.2b could lead to adding an outer `entry.mu.Lock()` to the `Do` body. See P1-E.

If the `Do` body for DiffShortstat correctly omits the outer lock (as the actual code requires), there is no re-entrancy risk. `diffShortstatUncached` acquires and releases `entry.mu` twice, sequentially, non-reentrantly.

**Verdict**: Not a real risk provided Task 1.1.2b's ambiguity is resolved. Mitigation covered under P1-E.

---

### P3-B: cachedReachableSet acquires entry.mu while CommitMessages also holds entry.mu

**Scenario** (observed in existing code, not a new risk): `CommitMessages` (line 504) acquires `entry.mu.Lock()` for Phase 1, unlocks, then calls `cachedReachableSet` which may acquire `entry.mu.Lock()` again on a cache miss (line 564). Since these are sequential (not nested), no re-entrancy issue. Confirmed not a deadlock risk. Not introduced by this plan.

**Verdict**: Not a real risk. Pre-existing sequential lock pattern, outside scope of this plan.

---

## Summary Table

| ID | Severity | Category | Verdict | Mitigated by Plan? |
|----|----------|----------|---------|-------------------|
| P1-A | P1 | LIFO defer order in AheadBehind Do body | Not a real risk — LIFO is correct | N/A |
| P1-B | P1 | `val.(abResult)` nil assertion in AheadBehind | Not a real risk — err guard is always evaluated first | N/A |
| P1-C | P1 | `val.(bool)` nil assertion in HasUncommitted (final form) | Not a real risk — err guard is always evaluated first | N/A |
| **P1-D** | **P1** | **Task 1.1.3b broken recover pattern → nil assertion panic** | **REAL RISK** | **No — 1.1.3b scaffold is broken; requires task consolidation or explicit gate** |
| **P1-E** | **P1** | **DiffShortstat Do body ambiguity → double-lock deadlock** | **REAL RISK** | **No — plan leaves "check whether" instruction ambiguous; requires explicit prohibition** |
| P2-A | P2 | Singleflight stale cache window | Not a real risk — cache store is inside Do | N/A |
| **P2-B** | **P2** | **Stale dirty state after failed/partial Pause** | **REAL RISK** | **No — InvalidateDirtyCache only called on success path; needs defer or early-path calls** |
| P2-C | P2 | bool type assertion when dirtyKnown=true | Not a real risk — bool boxes correctly | N/A |
| P3-A | P3 | Mutex re-entrancy in DiffShortstat Do body | Not a standalone P3 risk — elevated to P1-E | Covered under P1-E |
| P3-B | P3 | cachedReachableSet + CommitMessages re-entrancy | Not a real risk — sequential, not nested | N/A |

---

## Required Plan Amendments

### Amendment 1 (P1-D): Consolidate Tasks 1.1.3b and 1.1.3c

Task 1.1.3b's intermediate scaffold uses a `var recoverErr error` pattern with unnamed returns. This pattern silently swallows go-git panics (the `if recoverErr != nil` check at the bottom of the function body is unreachable after a panic), causing a `nil.(bool)` assertion panic in the caller. The MANDATORY refactor in 1.1.3c corrects this by using named returns.

**Amendment**: Merge 1.1.3b and 1.1.3c into a single task. The task should provide only the final named-returns form (from 1.1.3c) and explicitly prohibit the intermediate `var recoverErr error` pattern. No intermediate broken form should appear in task scaffolding.

### Amendment 2 (P1-E): Prohibit outer lock in DiffShortstat Do body

Task 1.1.2b instructs the developer to "check whether `entry.mu` is exposed" and conditionally add a lock. The actual `diffShortstatUncached` always manages `entry.mu` internally.

**Amendment**: Replace the conditional instruction with: "Do NOT add `entry.mu.Lock()` to this `Do` body. `diffShortstatUncached` acquires and releases `entry.mu` internally (Phase 1 at line 614, Phase 3 at line 758 of the current file). Adding an outer lock to the `Do` body causes a permanent goroutine deadlock via mutex re-entrancy on the first call."

### Amendment 3 (P2-B): Invalidate dirty cache on all Pause exit paths

Task 1.2.1c places `i.gitManager.InvalidateDirtyCache()` only after `transitionTo(Paused)` succeeds. Failed Pause paths (CommitChanges failure, Remove failure) return early without invalidating.

**Amendment**: Add a `defer i.gitManager.InvalidateDirtyCache()` at the top of `Pause()` (immediately after `StopController()`) so invalidation fires unconditionally on all exit paths. This is safe: `InvalidateDirtyCache()` is a nil-safe no-op when no worktree is set. Apply the same pattern to `Resume()` (Task 1.2.1d).
