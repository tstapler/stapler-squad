# Consistency Report: perf-mutex-hotspots-2026-07

**Date**: 2026-07-01  
**Reviewer**: Cross-artifact consistency agent  
**Status**: READY (with annotated concerns)

---

## 1. Scope Consistency

### Requirements vs. Plan

**Requirements stated three fixes:**
1. GoGitVCSReader — singleflight for AheadBehind, DiffShortstat, HasUncommitted
2. CircularBuffer — upgrade sync.Mutex to sync.RWMutex
3. GitWorktree.IsDirty — add 5s TTL cache

**What the plan implements:**
1. Fix 1 (singleflight) — fully implemented in Epics 1.1–1.4 ✅
2. Fix 2 (CircularBuffer RWMutex) — correctly identified by research as already done; plan omits it as no-op ✅
3. Fix 3 (IsDirty TTL cache) — correctly identified by research as already done (15s TTL); plan omits it as no-op ✅

**Scope creep items (in plan but not in requirements):**
- **Epic 1.2: InvalidateDirtyCache on Pause/Resume** — this is a bonus fix not requested in requirements. The requirements document does not mention Pause/Resume invalidation. This is labeled "Bonus" in the plan header, which is appropriate. It is a low-risk additive change that is internally self-consistent.

**Missing items:** None. All in-scope requirements are addressed.

**Verdict**: Scope is consistent. The InvalidateDirtyCache bonus is clearly labeled and does not conflict with requirements. It adds value without violating the "no API changes" constraint (it is an internal method call).

---

## 2. Research-Plan Alignment

| Research Recommendation | Plan Approach | Aligned? |
|---|---|---|
| Separate `singleflight.Group` per method (pitfalls.md §1.3 + build-vs-buy.md §Fix1) | Plan uses three separate fields: `aheadBehindSF`, `diffStatSF`, `hasUncommittedSF` (Option C in plan) | ✅ Yes |
| `entry.mu` must remain inside the `Do` body (features.md §1, pitfalls.md §1.3) | Plan places `entry.mu.Lock()` / `defer entry.mu.Unlock()` inside `Do` body (Option B rejected, Option C chosen) | ✅ Yes |
| Named-return recover pattern (features.md §5, pitfalls.md §1.1) | Plan uses `func() (val any, doErr error)` named-return signature; `doErr = fmt.Errorf(...)` in recover | ✅ Yes — but see CONCERN 1 below |
| Only cache on success, not errors (pitfalls.md §1.2) | Plan only stores to TTL cache inside `Do` before returning non-nil result | ✅ Yes |
| Key = `worktreePath + "\x00" + base` for AheadBehind, `worktreePath` for others (features.md §1) | Plan uses these exact keys | ✅ Yes |
| `HasUncommitted` has no TTL cache — needs one added (features.md §1) | Plan adds `hasUncommittedCache sync.Map` with 30s TTL (Task 1.1.1a) | ✅ Yes |
| CircularBuffer already done (build-vs-buy.md) | Plan omits Fix 2 entirely | ✅ Yes |
| IsDirty cache already done at 15s (build-vs-buy.md) | Plan omits Fix 3 entirely | ✅ Yes |
| singleflight is in go.mod, no new deps (build-vs-buy.md) | Plan adds import but no `go get` | ✅ Yes |

**Verdict**: Research recommendations are consistently reflected in the plan's architectural choices.

---

## 3. Pitfall Mitigation Coverage

| Pitfall (pitfalls.md) | Mitigation in Plan | Status |
|---|---|---|
| §1.1 Panic propagation from Do to all waiters | Named-return recover defer in all three Do bodies; `doErr = fmt.Errorf(...)` | ✅ Addressed — but see CONCERN 1 for ambiguity in Task 1.1.2a code sample |
| §1.2 Negative cache / error sharing | Cache store only on success path inside Do | ✅ Addressed |
| §1.3 Key collision between methods | Separate singleflight.Group per method (Option C) | ✅ Addressed |
| §1.4 Context cancellation via DoChan | Plan uses synchronous `Do` (not DoChan); notes scanner has no per-call contexts | ✅ Addressed — correct decision |
| §1.5 Forget for cache invalidation | Plan acknowledges no invalidation path needed currently; Forget not required | ✅ Addressed (documented as non-issue) |
| §2.1 RWMutex write starvation (CircularBuffer) | Already implemented in codebase; not a new risk | ✅ N/A (pre-existing fix) |
| §2.2 Incorrect RLock/RUnlock pairing | Task 1.1.3c mandates extracting Phase 1 to inner helper with single defer; Task 1.1.2a mandates defer unlock | ✅ Addressed — but see CONCERN 1 |
| §2.3 defer ordering with multiple defers | inner helper pattern (1.1.3c) ensures iter.Close() pattern is preserved | ✅ Addressed |
| §2.4 copylock vet check | No struct copying; plan passes types by pointer; no new mutex fields need special treatment | ✅ Addressed |
| §2.5 RLock overhead under no-contention | Acknowledged as acceptable; no new RWMutex additions | ✅ Addressed |
| §3.1 Thundering herd on expiry | Core problem solved by singleflight; HasUncommitted gets TTL cache | ✅ Addressed |
| §3.2 time.Now() overhead | Plan uses one key variable for both cache check and singleflight key (§3.3 efficiency) | ✅ Addressed |
| §3.3 Cache key heap escape | Single `cacheKey` local variable reused for both sync.Map and singleflight Do | ✅ Addressed |
| §3.4 sync.Map dead-entry leak | Acknowledged as acceptable; no eviction needed | ✅ Addressed |
| §4.1–4.4 Go 1.25 specifics | No behavioral changes in Go 1.25 for singleflight, RWMutex, sync.Map | ✅ N/A |

**Verdict**: All pitfalls are either mitigated in the plan or correctly documented as non-issues. One mitigation (the AheadBehind Do body) has an ambiguity addressed under CONCERN 1 below.

---

## 4. Adversarial Concern Resolution

### CONCERN 1: Deferred panic-recovery variable shadowing in AheadBehind Do body
**Adversarial review**: The code sample in Task 1.1.2a shows explicit unlock chains while the prose below it says to use `defer entry.mu.Unlock()`. An implementer following the code sample will produce a deadlock-on-panic scenario.

**Plan's response**: The plan includes a "Critical constraint" note at the end of Task 1.1.2a that explicitly states: "defer `entry.mu.Unlock()` is mandatory here" and explains why. The plan's stated Acceptance Criteria for Story 1.1.2 also mandate: "`entry.mu` is acquired with `defer entry.mu.Unlock()` (never explicit scattered calls)".

**Assessment**: The Acceptance Criteria and prose are correct. The code sample is internally inconsistent with the prose (it shows `entry.mu.Lock()` without a corresponding `defer entry.mu.Unlock()` in the same Do body). However, the critical-constraint prose is prominent and unambiguous. An implementer reading the full task will follow the prose. The adversarial review's request to "remove the incorrect sample" is valid; the inconsistency should be resolved in the code sample itself before implementation starts. **This does not block implementation but must be fixed in the code sample before the implementer begins Task 1.1.2a.**

**Residual risk**: LOW — the Acceptance Criteria override the code sample.

### CONCERN 2: HasUncommitted inner-helper refactor must be mandatory
**Adversarial review**: Task 1.1.3c was marked optional ("if inner-helper refactor adds > 5 minutes, keep explicit unlocks").

**Plan's response**: The current plan text at Task 1.1.3c header states: `[MANDATORY]` explicitly. The plan already incorporated this fix from the adversarial review. The "keep as-is" escape hatch has been removed. The glossary entry for `DoBody` explicitly states that `entry.mu.Lock()` inside Do is mandatory.

**Assessment**: ✅ Fully resolved in the plan.

### CONCERN 3: Shared-error amplification from singleflight not documented
**Adversarial review**: Transient go-git errors will be broadcast to all N waiters simultaneously, producing N error log entries and a "persistent error" pattern on consecutive cycles.

**Plan's response**: The plan's observability section states "no new log statements required" but does not address the amplification semantics. No comment is added at Do call sites about shared-error behavior.

**Assessment**: NOT addressed in the plan. The adversarial review asks only for a comment (no code change). This is a documentation gap. **The implementer should add a single-line comment at each `.Do(` call site noting the shared-error semantics.** This does not affect correctness.

### CONCERN 4: GitManager interface expansion will break undiscovered test doubles
**Adversarial review**: Adding `InvalidateDirtyCache()` to `GitManager` interface breaks all structs that implement it.

**Plan's response**: Task 1.2.1a explicitly addresses this: "Before adding `InvalidateDirtyCache()` to the `GitManager` interface, grep for all structs that implement it and add a no-op `InvalidateDirtyCache()` method to every implementation found."

**Assessment**: The plan addresses the concern with an explicit audit sub-task. ✅ Resolved.

**Additional finding from repo audit**: The `gitManager` field in `instance.go:316` is declared as `GitWorktreeManager` (the concrete struct), not as `GitManager` (the interface). This means adding `InvalidateDirtyCache()` to the `GitManager` interface does not technically affect instance.go — it calls the method directly on `GitWorktreeManager`. However, there is one compile-time check (`var _ GitManager = (*GitWorktreeManager)(nil)`) that will catch if the implementation misses the new method. The search for other `GitManager` interface users found **no other implementations** outside of `*GitWorktreeManager`. The test file `history_linker_test.go` uses `inst.gitManager.SetWorktree(...)` which calls directly on the concrete type. **Concern 4 from the adversarial review is a non-issue for this specific codebase** — there are no other types implementing `GitManager`. The audit sub-task in 1.2.1a is still worth running to confirm.

---

## 5. File Path Accuracy

All four file paths named in the plan tasks were verified to exist:

| Path | Exists? | Size |
|---|---|---|
| `session/unfinished/gogit_vcs_reader.go` | ✅ Yes | 37.8 KB |
| `session/git_worktree_manager.go` | ✅ Yes | 7.9 KB |
| `session/instance.go` | ✅ Yes | 51.9 KB |
| `session/unfinished/gogit_vcs_reader_limits_test.go` | ✅ Yes | 8.4 KB |

Additional paths referenced in research and plan were also verified:
- `session/circular_buffer.go` ✅ — already uses `sync.RWMutex` at line 20 (Fix 2 confirmed done)
- `session/git/worktree_git.go` ✅ — `InvalidateDirtyCache` exists; `isDirtyCacheMu sync.RWMutex` present (Fix 3 confirmed done)
- `session/git/worktree.go` ✅ — `IsDirtyCacheTTL = 15 * time.Second` at line 26

**Singleflight not yet present**: Confirmed that `singleflight` does not appear in `gogit_vcs_reader.go` — the fix is genuinely not yet implemented, matching research findings.

---

## 6. Additional Consistency Notes

### Requirement says "5s TTL" but codebase has 15s TTL
Requirements §Scope item 3 says "5s TTL result cache per worktree path." The actual `IsDirtyCacheTTL` is 15 seconds (not 5s). Research/features.md §3 notes this discrepancy and advises checking the poller interval before reducing to 5s. The plan does not implement Fix 3 at all (correctly, since it already exists). The 5s vs. 15s discrepancy is a requirements accuracy issue but has no implementation impact — the existing 15s TTL satisfies the spirit of the requirement (reduce subprocess calls).

### CommitMessages / cachedReachableSet thundering herd gap
Adversarial MINOR 2 correctly identifies that `CommitMessages` and `cachedReachableSet` have the same thundering-herd risk. The plan explicitly excludes them from scope (correct for MVP). This should be tracked as follow-on work. No consistency issue.

### Pause/Resume line number accuracy
The plan cites Pause() around line 1041 and Resume() around line 1151. Actual lines are `func (i *Instance) Pause() error` at line 972 and `func (i *Instance) Resume() error` at line 1048. The transitionTo(Paused) call is near line 1037. These approximate references are close enough — the plan says "around line", which is guidance, not a hard constraint.

---

## Summary

| Check | Result |
|---|---|
| Scope consistency | ✅ Pass — bonus Epic 1.2 is clearly additive |
| Research-plan alignment | ✅ Pass — all key recommendations followed |
| Pitfall mitigation | ✅ Pass — all pitfalls addressed |
| CONCERN 1 resolution | ⚠️ Partial — Acceptance Criteria correct; code sample ambiguous (must be clarified before Task 1.1.2a) |
| CONCERN 2 resolution | ✅ Pass — `[MANDATORY]` tag present in Task 1.1.3c |
| CONCERN 3 resolution | ⚠️ Not addressed — no comment at Do call sites about shared-error semantics (low risk, doc-only) |
| CONCERN 4 resolution | ✅ Pass — audit sub-task present; confirmed no other implementors in codebase |
| File path accuracy | ✅ Pass — all 4 primary paths exist; singleflight not yet present (correct) |

**Blocking issues**: None. No artifacts directly contradict each other in a way that would produce incorrect runtime behavior.

**Pre-implementation actions recommended**:
1. Fix the Task 1.1.2a code sample to show `defer entry.mu.Unlock()` inside the Do body (not bare `entry.mu.Lock()` without defer).
2. Add a comment at each `.Do(` call site noting shared-error semantics per Concern 3.
