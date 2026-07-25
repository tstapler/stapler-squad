# Architecture Review: perf-mutex-hotspots-2026-07

**Reviewer**: Architecture Review Subagent
**Date**: 2026-07-01
**Plan**: `project_plans/perf-mutex-hotspots-2026-07/implementation/plan.md`
**Verdict**: CONDITIONAL APPROVAL — 0 blockers, 4 concerns, 3 nitpicks

---

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist. No hard-constraint violations to report.

Applicable existing ADRs reviewed:
- **ADR-011**: Prefer lock-free concurrency — the plan's use of `singleflight.Group` and `sync.Map` is consistent with ADR-011's guidance on high-contention scenarios.
- **ADR-001** (project-local): `decisions/ADR-001-singleflight-per-method-groups.md` is referenced by the plan and exists; the chosen Option C directly implements its decision.

---

## Lens 1 — Structural Integrity

### Finding 1.1 — CONCERN: `err` variable shadowing in `Do` body panic recovery (Story 1.1.2, Task 1.1.2a)

The plan's code snippet for `AheadBehind` illustrates the panic recovery pattern, but the `err` in the deferred closure conflicts with the outer `err` variable declared by `aheadBehindSF.Do(...)`:

```go
val, err, _ := g.aheadBehindSF.Do(cacheKey, func() (any, error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("go-git panic in AheadBehind: %v", r)  // ← assigns to outer 'err', but Do already returned
        }
    }()
    ...
})
```

The plan itself acknowledges this in the note at the end of Task 1.1.2a: "Use a named return or reassign." However, the note is easy to miss during implementation, and the `github/client.go` precedent (the existing singleflight pattern) does **not** use panic recovery at all, so there is no in-codebase model for the correct pattern.

The safe pattern — acknowledged but not shown in the plan — is:

```go
val, err, _ := g.aheadBehindSF.Do(cacheKey, func() (any, error) {
    var recoverErr error
    defer func() {
        if r := recover(); r != nil {
            recoverErr = fmt.Errorf("go-git panic in AheadBehind: %v", r)
        }
    }()
    // ... all go-git work ...
    if recoverErr != nil {
        return nil, recoverErr
    }
    return abResult{ahead, behind}, nil
})
```

The deferred recover only sets `recoverErr`; it cannot return from the `Do` closure directly. If the deferred closer assigns to the outer `err` (returned by `Do`), the assignment happens *after* `Do` has already unwound the stack from the panic — it has no effect on the value `Do` returns. The result would be a silently swallowed panic and a nil `val`, causing a nil-pointer panic on `val.(abResult)` at the call site.

**Remediation**: The plan's Task 1.1.2a note must be promoted to a concrete code pattern. Show the `var recoverErr error` + check-after-work pattern explicitly. The same applies to Task 1.1.2b (DiffShortstat) and Task 1.1.3b (HasUncommitted). Task 1.1.3b already uses `var recoverErr error` correctly — the inconsistency between 1.1.2a/b and 1.1.3b suggests 1.1.2a/b were drafted before the pattern was finalized.

---

### Finding 1.2 — CONCERN: `hasUntrackedFiles` runs outside the `Do` body's panic recovery scope (Story 1.1.3, Task 1.1.3b)

The plan wraps `HasUncommitted`'s entire body inside a `Do` closure, including Phase 2 (the OS-only `hasUntrackedFiles` directory walk). The panic recovery defer at the top of the `Do` body protects against go-git panics in Phase 1. But `hasUntrackedFiles` is a recursive `os.ReadDir` walk — it cannot panic from go-git but *can* from a stack overflow on a deeply nested directory tree (>= thousands of levels). More importantly, because the entire function is now inside `Do`, any future contributor adding a go-git call to Phase 2 would not realize it is unprotected.

The plan's note (Task 1.1.3b) states: "The `Do` body must wrap both phases." This is correct for cache-store atomicity (the cache store must happen inside `Do` so all waiters receive the final value). However, it creates a subtle scoping implication: the deferred recover at the top of the closure protects only the code that executes *before* it panics. Since `hasUntrackedFiles` is pure OS work and does not touch go-git, this is not a runtime risk in the current code — it is a cognitive load / maintenance risk.

**Remediation**: Add a comment at the top of the Phase 2 block inside the `Do` closure explicitly noting: "entry.mu released; no go-git access below; panic recovery above covers only Phase 1." This costs one line and prevents future confusion.

---

### Finding 1.3 — CONCERN: `InvalidateDirtyCache` on `GitManager` interface widens a stable contract (Story 1.2.1, Task 1.2.1a)

`GitManager` is a package-level interface with 19 existing methods, satisfied by `*GitWorktreeManager` and implemented by test doubles. Adding `InvalidateDirtyCache()` to the interface means every test double (mock or stub) that implements `GitManager` must add the method or fail to compile. The plan does not mention this downstream impact.

The actual callers in `instance.go` (Pause/Resume) always use the concrete `*GitWorktreeManager` indirectly via the interface — but if test files in `session/` or elsewhere define a `MockGitManager` struct that explicitly lists `GitManager` satisfactions, those will break.

**Remediation**: Before adding to the interface, search for all `GitManager` implementations or `var _ GitManager` compile-time assertions outside of `git_worktree_manager.go`. If test doubles exist, they need a no-op `InvalidateDirtyCache()` method added. The plan's checklist should include: "grep for `GitManager` implementors outside `git_worktree_manager.go`."

---

### Finding 1.4 — NITPICK: `DiffShortstat`'s `Do` body calls `diffShortstatUncached` which itself acquires `entry.mu` internally (Story 1.1.2, Task 1.1.2b)

Looking at the actual `diffShortstatUncached` implementation (lines 607–748 in `gogit_vcs_reader.go`), it calls `g.openRepoEntry(worktreePath)` and then `entry.mu.Lock()` inside its own body. The plan wraps `diffShortstatUncached` in `diffStatSF.Do` as a black-box call. This is correct — `entry.mu` is properly scoped inside `diffShortstatUncached` — but the plan's description of "entry.mu is acquired and released inside Do" is slightly misleading for this case: the lock acquisition happens *inside a callee*, not directly in the `Do` closure.

This is not a bug. However, the plan's acceptance criteria for Story 1.1.2 says "`entry.mu.Lock()` is held inside `Do` for DiffShortstat" — this is technically true (the call is inside `Do`) but readers may expect to see it at the closure's top level. The code will work correctly.

**Remediation**: Clarify the acceptance criteria for 1.1.2 to say "`entry.mu` is acquired transitively via `diffShortstatUncached` inside the `Do` closure" to avoid implementor confusion.

---

## Lens 2 — Type-Level Design

### Finding 2.1 — NITPICK: `hasUncommittedEntry` mirrors `diffStatEntry` exactly except for the result field type (Lens 2 — DRY)

The plan adds a third cache entry struct (`hasUncommittedEntry{result bool, expiry time.Time}`) alongside `diffStatEntry` and `aheadBehindEntry`. All three share the `expiry time.Time` field and the same 30s TTL constant. This is intentional (Go does not allow generic cache entry structs without generics) and acceptable, but worth noting: if the TTL logic ever changes (e.g., to per-entry TTL policies), three structs must be updated. The existing code already accepted this tradeoff for `diffStatEntry`/`aheadBehindEntry`, so the plan is consistent.

**Remediation**: No action required. Acknowledged as accepted tradeoff consistent with codebase style.

---

### Finding 2.2 — CONCERN: `val.(bool)` type assertion in HasUncommitted `Do` return path is fragile if the closure ever returns a non-bool (Story 1.1.3, Task 1.1.3b)

The plan's `Do` closure for `HasUncommitted` returns `result, nil` where `result` comes from `hasUntrackedFiles(worktreePath, indexed)`. The `hasUntrackedFiles` return type is `(bool, error)`, so the closure returns `(any, error)` with the `any` holding a `bool`. At the call site, `val.(bool)` asserts this.

The risk: if any early-return path inside the closure accidentally returns `(nil, nil)` — which is valid for `(any, error)` — the assertion `val.(bool)` will panic with "interface conversion: interface is nil, not bool". This is particularly likely for the early-return paths inside Phase 1 (e.g., "index has a conflict stage → return `true, nil`"). In those paths, the current plan returns `result, nil` — but `result` is `true` (a `bool`), which is fine. However, one early-return path reads: "return result, nil" where result has not yet been assigned (it is only assigned by `hasUntrackedFiles` at the end). If Phase 1 returns early with a named `result bool` that is zero-valued, the returned `val` will be `false` (not `nil`), so the assertion is safe. This is a subtle correctness property that depends on how the implementor writes the early returns.

**Remediation**: Add a note to Task 1.1.3b: "All `return` statements inside the `Do` closure must return a typed `bool` value (not `nil`) as the first argument. The pattern `return true, nil` is correct; `return nil, nil` would cause a type-assertion panic at the call site."

---

### Finding 2.3 — NITPICK: No parse-at-boundary validation for `worktreePath` — pre-existing, not introduced by plan

`GoGitVCSReader` accepts `worktreePath` as a raw `string` across all methods. There is no `WorktreePath` value object. This is a pre-existing primitive obsession that the plan does not introduce and is not required to fix given the "performance refactor" scope classification. Mentioned for completeness only.

**Remediation**: Out of scope for this plan. Acceptable as-is.

---

## Lens 3 — Pattern Selection

### Finding 3.1 — Pattern fit: PoEAA — the plan correctly applies the Cache pattern (Lazy Load variant)

The TTL cache + singleflight combination in the plan is a well-formed Lazy Load (Fowler, PoEAA §12). The fast path checks the cache (the "ghost" or "value holder" variant); on miss, the singleflight group ensures the underlying load runs once. This is consistent with how `diffStatCache` and `aheadBehindCache` already work in the file. The plan adds `hasUncommittedCache` as a direct structural analog — no pattern mismatch.

---

### Finding 3.2 — GoF pattern: the plan correctly uses the Null Object pattern for `InvalidateDirtyCache` nil-guard

The nil guard in `GitWorktreeManager.InvalidateDirtyCache()` (`if gm.worktree == nil { return }`) follows the same nil-safe defensive pattern used by every other method in `git_worktree_manager.go` (see `IsDirty`, `PrimeDirtyCacheJitter`, etc.). This is consistent and correct.

---

### Finding 3.3 — NITPICK: The concurrency test (Task 1.3.1a) does not assert that the `Do` body ran exactly once — it only asserts result consistency

The plan's acceptance criteria states "a counter... tracks actual go-git calls" but the actual test code does not include such a counter. The plan acknowledges this with "Simplest approach: use `testing.T.Parallel()`... and assert all return identical results." Result consistency is a necessary but not sufficient proof of singleflight coalescing: all 4 workers could have each run the full `Do` body (no coalescing) and still return identical results for a clean repo.

For a regression test to be meaningful, it should verify *behavioral* coalescing, not just result consistency. The plan's simpler version is still useful (it catches data races via `-race`) but does not fulfill the "assert Do runs once" acceptance criterion.

**Remediation**: Either update the acceptance criteria to drop the "runs once" assertion (accepting `-race` + result consistency as sufficient), or inject a `sync/atomic` counter into a test-only hook. The simplest approach: add an `atomic.Int64` call counter to `GoGitVCSReader` behind a `testing.TB`-gated field, or accept that `-race` + consistent results is the actual bar. The plan should be explicit about which it chose. This does not affect correctness of the production code.

---

## Summary Table

| # | Finding | Story | Classification |
|---|---------|-------|----------------|
| 1.1 | Panic recovery `err` shadowing — plan note must become explicit code pattern | 1.1.2 | CONCERN |
| 1.2 | Phase 2 of HasUncommitted inside `Do` needs inline comment about recovery scope | 1.1.3 | CONCERN |
| 1.3 | `GitManager` interface widening — downstream test doubles not audited in plan | 1.2.1 | CONCERN |
| 2.2 | `val.(bool)` assertion safety — early-return nil risk in `Do` closure | 1.1.3 | CONCERN |
| 1.4 | `diffShortstatUncached` acquires `entry.mu` transitively — acceptance criteria wording | 1.1.2 | NITPICK |
| 2.1 | Three parallel cache entry structs — accepted DRY tradeoff | 1.1.1 | NITPICK |
| 3.3 | Concurrency test does not actually assert `Do` ran once | 1.3.1 | NITPICK |

**No blockers.** None of the findings prevent the plan from being implemented correctly, but the 4 concerns are implementation traps that will cause bugs or build failures if overlooked.
