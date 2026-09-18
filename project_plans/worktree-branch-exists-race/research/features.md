# Research: Feature Landscape & Edge Cases — worktree-branch-exists-race

## 1. Call graph: who calls `Setup()` / `setupNewWorktree()`, and what retries exist

```
GitWorktree.Setup()                                    session/git/worktree_ops.go:18
├── branch-check goroutine: repo.Reference(branchRef, false)   :43
│     err != nil (ANY reason) → branchExists = false, errChan <- nil   ← bug: err is discarded, not just miscategorized
├── branchExists → setupFromExistingBranch()             (safe, idempotent, reuse-in-place)
└── !branchExists → setupNewWorktree()                   :217
      ├── repo.Reference(branchRef, false) AGAIN          :235   ← same anti-pattern, independent bug instance
      ├── err == nil (branch exists) → setupFromExistingBranch()
      └── err != nil (ANY reason, folded to "doesn't exist") → cleanupExistingBranch(repo)  :242
            → repo.Storer.RemoveReference(branchRef)      session/git/worktree_branch.go:15  ← UNCONDITIONAL, DESTRUCTIVE
            → git worktree add -b <branch> ...             :262  (fails loudly if branch truly exists — git's own check)
```

Direct callers of `Setup()`:
- `session/instance_worktree.go:123` — `CreateBacklogWorktree(repoPath, branchSuffix)`, error wrapped and propagated (`"CreateBacklogWorktree setup: %w"`), no retry at this layer.
- `session/git_worktree_manager.go:126` — thin passthrough (`return wt.Setup()`).
- `session/instance.go:966,1180,1431,1571` — various instance (re)start paths; propagate error up to the caller (session start/resume flow), no local retry.
- `server/services/backlog_service_triage.go:2417` — triage isolation worktree (`triage-<itemID>`, not the backlog work branch); **on error it silently falls back to running triage directly in `itemRepoPath`** (`log.WarningLog`, no error surfaced, no MarkStuck). Out of scope for this fix's branch-naming discussion (uses `triage-` prefix, not `BacklogBranchPrefix`), but same code (`wt.Setup()`), so this fix's behavior change applies here too.

The AC-relevant path is `CreateBacklogWorktree` → `resolveSessionPath` (`server/services/backlog_service_triage.go:1426`):
```go
func resolveSessionPath(repoPath, slug string) (worktreePath string, useWorktree bool, err error) {
    wt, wtErr := session.CreateBacklogWorktree(repoPath, slug)
    if wtErr == nil { return wt, true, nil }
    ...
    if git.IsGitRepo(resolvedRepo) {
        return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create git worktree: %w", wtErr))
    }
    // falls back to plain directory ONLY when repoPath is not git-managed at all
    ...
}
```
This is the **BUG-057** decision cited in the requirements' Non-Goals: a git-managed repo's worktree-creation failure is a hard `CodeInternal` error, never silently downgraded to a directory session. Confirmed still in place — do not touch.

## 2. What happens when `Setup()`'s error propagates: existing retry/backoff, and does surfacing more errors introduce a new failure mode?

**Yes, a generic backoff/retry mechanism already exists and will absorb this without new plumbing.** `session/backlog_remediation.go`'s `remediationBackoffSchedule` (30m → 2h → 8h → 24h → 72h, `MaxRemediationAttempts = 5`, ~4.5 days total) gates every automated stuck-item remediation action, keyed by `(itemID, StuckReason)`.

Concretely, for the "Not converging 20h" symptom the requirements reference:
- `server/services/autonomous_orchestration_service.go:364-390` gates `AutoRespawnAutonomousWork` behind `RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)`.
- `AutoRespawnAutonomousWork` (`backlog_service_triage.go:1654`) → `SpawnSessionFromItem` → `resolveSessionPath` → `CreateBacklogWorktree` → `wt.Setup()`.
- On failure, `notifyAutonomousRespawnAttemptFailed` (`autonomous_orchestration_service.go:607`) fires a non-terminal WARNING notification ("will retry automatically per the standard backoff schedule"); after 5 exhausted attempts, `justParked` fires a terminal HIGH notification telling the operator to use "Reset."

**So surfacing the error (AC1/AC2) does not introduce a new stuck-forever failure mode** — it routes into a mechanism that already exists and is already exercised by every other kind of respawn failure. The item was *already* failing on every attempt before this fix (that's the bug report itself: "permanently stuck" because the branch legitimately exists on every subsequent attempt too, once it's created once). The fix does not make anything less converging; if anything it makes the *first* attempt fail cleanly with an accurate error message (see §4) instead of masking a real "branch already exists" as a fabricated "doesn't exist" that then hits `cleanupExistingBranch`'s destructive path (see §5 below — this is the more important finding).

**Where the backoff does NOT reach:** `TriggerTriage`'s auto-spawn-after-triage path (`backlog_service_triage.go:2554-2563`) calls `SpawnSessionFromItem` directly and, on error, only `log.WarningLog.Printf`s — no `MarkStuck`, no notification, no retry. An item with `AutoSpawnSession=true` whose first-ever worktree creation hits this race would sit at `BacklogStatusReady` with no session and no operator-visible signal, invisible to `RemediationDue`'s per-reason gate entirely (no `StuckReason` is ever recorded for it). This is a **pre-existing gap unrelated to the swallowed-ref-error bug** (it exists regardless of why `Setup()` fails) and out of this fix's scope, but worth flagging as its own follow-up — it's the one path where "surface the error" doesn't reach an existing safety net.

## 3. Same swallowed-error anti-pattern elsewhere in `session/git/*.go`

Full `repo.Reference(...)` inventory:

| Site | Pattern | Verdict |
|---|---|---|
| `worktree_ops.go:43` (`Setup`) | `err == nil` branch-exists check, err discarded on any failure | **In scope (AC1)** |
| `worktree_ops.go:235` (`setupNewWorktree`) | Same pattern, independent instance | **In scope (AC2)** |
| `ops.go:76` (`isAncestorOfRef`) | `err != nil` → wraps and **returns the error** | Correct — no fix needed |
| `ops.go:109` (`BranchAheadBehind`) | `err != nil` → `return BranchStatus{BranchExists: false}, nil` (swallows ANY error, not just `ErrReferenceNotFound`) | **Same anti-pattern, but explicitly documented as intentional**: the doc comment states "a branch already deleted locally reports BranchExists=false rather than an error, since that's the expected state for a shipped, cleaned-up item, not a failure." A transient ref-read error here would be silently reported as "branch shipped/gone" rather than surfaced — same false-negative risk as the Setup() bug, just in a read-only status path (PR-fix-branch sync status, `backlog_service_ship_status.go:120`; `worktree_git.go:443`) rather than a destructive one. **Recommend flagging as a follow-up, not folding into this fix** — different blast radius (status display vs. branch deletion + fatal worktree-add failure), and changing it would require re-litigating the documented "not a failure" rationale rather than a mechanical one-line change like AC1/AC2. |
| `ops.go:113,166` (`BranchAheadBehind`, `BehindOriginMain`) | `err != nil` → wraps and returns | Correct |
| `util.go:236` | `err != nil` handled via caller's own fallback logic (HEAD detection) | Not a branch-exists check, out of scope |
| `session/unfinished/gogit_vcs_reader.go:715,729,1793` | `err == nil` used as a "does this ref/branch candidate exist" probe when iterating a list of candidates | Different shape — err==nil is genuinely the right test when trying several candidate names in a loop (first successful one wins), not a binary "exists vs corrupt" decision. Also lives under `session/unfinished/` (package name suggests WIP/dead code — verify usage before touching). Not in scope.

**Verdict for collateral-debt fixing (per `.claude/rules/fix-flaky-tests-dont-defer.md`'s sibling standing instruction to fix collateral debt found while working):** `ops.go:109`'s `BranchAheadBehind` is the one other candidate, but it has a *documented* rationale that would need to be revisited (not just mechanically copied), and its blast radius is a status badge, not data loss. Recommend noting it in the PR description as a known, deliberately out-of-scope sibling rather than silently fixing it alongside AC1/AC2 — the `.claude/rules/fix-collateral-debt.md` standing instruction favors fixing collateral debt found while working, but "found while working" should not extend to redesigning a *different* function's already-documented tradeoff without its own review.

## 4. What "surface the error" concretely means — return vs. bounded retry

**Recommendation: return the error (AC1's literal reading), no retry inside the ref-lookup itself.** Reasoning:

- The generic backoff/retry mechanism (§2) already exists one layer up, at the *stuck-item* level (30m–72h schedule) — it is deliberately coarse-grained and shared across every kind of remediation failure, not per-operation. Adding a second, finer-grained retry loop *inside* `Setup()`'s ref check would be a second, independently-tuned backoff competing with the first, for no clear benefit: a ref-read failure caused by packed-refs lock contention from a concurrent writer is contended for milliseconds to low seconds, not the 30-minute-plus scale the outer schedule already operates at. An inner retry loop optimizes for "self-heal within the same call," which is a real but narrow win (avoids burning one of the 5 outer attempts on a blip) — see caveat below.
- go-git's `repo.Reference()` against a lock-contended `packed-refs` file is a plain file read (`os.Open`/`os.ReadFile` under the hood) — go-git does not expose a "the ref file is locked, try again" typed error distinct from a generic I/O error; there's no clean signal to retry on specifically vs. e.g. a genuinely corrupt ref. A bounded retry would have to be a blind "retry N times with M ms backoff on *any* non-`ErrReferenceNotFound` error," which risks masking a real, non-transient error (permissions, disk full, actual corruption) behind a few hundred ms of retries that also delay every legitimate `Setup()` call by that amount on the (presumably rare) contended case.
- Simplicity matches the ACs literally (AC1/AC2 just say "not folded into `branchExists = false` — propagated or surfaced instead") and keeps the fix minimal and reviewable, consistent with this repo's stated preference for the smallest change that closes the gap (see `.claude/rules/fix-flaky-tests-dont-defer.md`'s "most flakes are a missing synchronization point... fix it" — here the "fix" is removing a bug, not adding new retry infrastructure).

**Caveat worth deciding explicitly in the plan phase (not resolved by this research alone):** if a *transient* (self-healing-within-seconds) ref-read error is common enough in practice to matter, a **tiny, bounded retry (e.g. 2-3 attempts with a short fixed delay, ~50-100ms) directly around the single `repo.Reference()` call** would let those cases succeed within the same `Setup()` invocation instead of spending one of the outer schedule's 5 precious attempts (and up to 30 minutes of wall-clock wait before the next respawn) on a blip that would have cleared itself in under a second. This repo has no evidence in-hand (no log grep performed here) of how often that actually happens — the backlog item's own description frames the failure as the *first* symptom of a bigger structural problem ("swallowed ref-lookup errors"), not as "we've observed N transient blips causing wasted 30-minute backoff waits." Recommend: ship the plain propagate-only fix per AC1-4 now (it is strictly correct and required regardless), and treat "should the ref-lookup itself retry" as a separate, evidence-gated follow-up if transient failures are later observed empirically (e.g. via a log grep for the new propagated error's message across `~/.stapler-squad/logs/`).

## 5. The bigger finding: `cleanupExistingBranch`'s destructive `RemoveReference`

This is the most severe consequence of the bug, more so than the literal "branch already exists" fatal error in the backlog item title. `setupNewWorktree()`'s ref check (`worktree_ops.go:235`) is the ONLY gate before calling `cleanupExistingBranch()` (`worktree_branch.go:11`), which unconditionally does:
```go
if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
    return fmt.Errorf(...)
}
```
i.e. it **deletes the branch ref** whenever the (buggy) caller believes the branch doesn't exist. If a transient/racy `repo.Reference()` failure (sustained packed-refs contention, not just a one-off blip) causes `setupNewWorktree` to wrongly conclude "branch doesn't exist" on a branch that legitimately does — and does hold commits — `cleanupExistingBranch` will delete that ref outright, with no "is it merged / does it hold unique commits" check. This is functionally the same bug class as commit `5087a282e` ("fix(git): stop deleting worktree branches on cleanup/reset"), which fixed the exact same destructive-`RemoveReference`-on-a-possibly-unique-branch problem in `Cleanup()` — but that fix did not touch `cleanupExistingBranch` in `worktree_branch.go`, which still has the identical unconditional `RemoveReference` call, just gated by a different (buggy) precondition.

**Implication for the plan/implementation phases:** AC2's fix (propagate non-`ErrReferenceNotFound` errors out of `setupNewWorktree`'s check) is what closes this data-loss path — it's not just about avoiding a confusing error message, it's the guard that prevents `cleanupExistingBranch` from ever running against a branch whose existence couldn't actually be verified. This should be called out explicitly in the regression test (AC5): a test asserting that a non-`ErrReferenceNotFound` ref-lookup error in `setupNewWorktree` returns before reaching `cleanupExistingBranch`/`RemoveReference` (e.g. by asserting the branch ref still exists after a forced-error `Setup()` call), not just that an error is returned.

## 6. Precedent search

- `5087a282e` ("fix(git): stop deleting worktree branches on cleanup/reset", 2026-07-14) — same destructive-branch-deletion class, different code path (`Cleanup()`, not `cleanupExistingBranch`/`setupNewWorktree`). Established the "a leftover local branch ref costs nothing; a lost commit is not recoverable through this code path" principle now enshrined in `Cleanup()`'s doc comment and echoed in `.claude/rules/`. Its regression test (`TestCleanup_PreservesBranchWithCommits`, `session/git/worktree_creation_test.go`) is the closest existing template for this fix's AC5 test: create a worktree, commit to it, force the failure path, assert the branch+commit survive.
- No prior commit specifically touches `ErrReferenceNotFound` handling or a "swallowed ref-lookup error" pattern — `git log -S ErrReferenceNotFound` only shows the current SDD planning commits plus unrelated hits. This is a genuinely new class of fix, not a repeat of a previously-fixed bug.
- `BranchAheadBehind`'s "branch missing = shipped, not a failure" comment (`ops.go:97-102`) was written deliberately (not swallowed by accident) — cross-referenced in §3 as the one other `repo.Reference()` call with the same *shape* but a different, documented intent.

## Summary of edge cases the implementation must handle correctly

1. **Success** — branch doesn't exist, `ErrReferenceNotFound` → unchanged, creates new branch (AC3).
2. **Branch exists** — `err == nil` → unchanged, routes to `setupFromExistingBranch()` (AC4).
3. **Other error in `Setup()`'s goroutine** — must propagate out of `Setup()` entirely, not just skip `setupNewWorktree()` (AC1). Currently `errChan <- nil` is sent even on error — the error value itself must reach the channel.
4. **Other error in `setupNewWorktree()`'s check** — must return before `cleanupExistingBranch()` is ever called (AC2 + §5 — this is the data-loss-prevention case, the highest-value part of the fix).
5. **Regression test (AC5)** must cover all three ref-lookup outcomes (`nil`/success, `ErrReferenceNotFound`, other error) for *both* `Setup()` and `setupNewWorktree()` independently, since they currently have two separate, independently-buggy checks — fixing one without the other leaves the bug reachable via the second (e.g. if `Setup()`'s check is fixed but transient contention clears between the two calls, `setupNewWorktree()`'s own still-buggy check remains exploitable).
6. **No change to branch-preservation/naming (AC6)** — confirmed: this fix touches only the two `repo.Reference()` error-handling branches, not `BacklogBranchPrefix`, `cleanupExistingBranch`'s happy-path behavior, or `Cleanup()`.
