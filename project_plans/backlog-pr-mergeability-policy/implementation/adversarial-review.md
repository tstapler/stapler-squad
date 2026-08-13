# Adversarial Review (Round 2): backlog-pr-mergeability-policy

**Date**: 2026-07-17
**Verdict**: CONCERNS
**Round-1 verdict**: BLOCKED (2 blockers + 6 concerns). Both blockers are now genuinely
**RESOLVED** (verified against source, not papered over). The patches introduce **2 new
CONCERNS** and leave the previously-accepted risks reasonably accepted. No blocker remains.

Load-bearing claims re-verified against real source (all TRUE):
- `prReadyToMergeSolo` CI predicate (`stuck_decisions.go:88-93`): `switch CheckConclusion { case
  "success", "": proceed; default: return false }` — any value other than success/"" (including
  `"pending"`) returns false. The tri-state fix works *because* of this exact predicate.
- The real Blocker-1 defect site (`backlog_lifecycle.go:1600-1602`): `if prStatus.CIFailing {
  info.CheckConclusion = "failure" }` — CIPending has no representation, so pending/absent CI
  collapses to `""` = green. Confirmed.
- `EnablePRAutoMerge` is unconditional today at `backlog_lifecycle.go:1475` (in `pushAndCreatePR`).
- `pushAndCreatePR` writes PR fields at `:1460` **before** the `review→pr_pending` transition at
  `:1489` (precondition `ExpectedStatus: review`) — the Class-B orphan is real and recoverable.
- Transition table (`domain/backlog.go:241-262`): `in_progress` reaches only
  review/ready/refining/idea — **`in_progress→pr_pending` INVALID**; `review→pr_pending` VALID.
  The two-step `in_progress→review→pr_pending` routing is therefore sound.
- All three rescue paths miss Class B: `FindPRPendingItems` needs `pr_pending`+`PrNumber>0`
  (`storage_backlog.go:553-554`); `BackfillMissingPRNumbers` needs `pr_pending`+`PrNumber==0`
  (`:577-578`); original detector filtered `PrNumber==0`.
- `prPendingChecker` interface (`backlog_lifecycle.go:55-58`) + its production factory
  `defaultPRPendingCheckerFactory` returns `*git.GitWorktree`, which already implements
  `EnablePRAutoMerge` (same concrete type backs `prCreator`, which lists it at `:67`).
- `FindInstanceDataByID` (`storage.go:392`) reads `ListInstanceData()` directly — no registry
  `Acquire`, no `onConstruct`.
- `FeatureController` interface (`session_service.go:56-60`); `ctrl.IsEnabled()` is the read
  source of truth (`feature_flag_service.go:86-88`); toggled by `UpdateFeatureFlag`
  (`:149-157`); existing `"backlog"` controller wired at `dependencies.go:892,941`.
- `CountReviewCyclesSince` (`storage_backlog.go:816-823`) counts `BacklogStatusEvent` rows with
  `from=in_progress, to=review` in the 24h window; `bounceThreshold=3`; `isBouncing := count>=3
  && !hasPass` (`stuck_decisions.go:61-63`). Shared cap counts **all** lifetime `SessionRoleWork`
  sessions across both reopen paths (`backlog_service_triage.go:515-521, 603-609`).

---

## Blocker resolution (both RESOLVED)

**BLOCKER-1 (CI "green" was "not-failing") — RESOLVED.** The fix makes BOTH green-dependent
gates require positively-passing CI:
- Epic 3.0 adds `CIPending` to `PRStatus` and derives it in the `StatusCheckRollup` loop
  (`worktree_git.go:512-533`) — the loop already parses `check.Status` and `check.Conclusion`, so
  "at least one check non-terminal" is derivable; the fix is implementable as written.
- Task 3.0.1b maps the tri-state (`failure`/`pending`/`""`) into `PRInfo.CheckConclusion`,
  replacing the `CIFailing`-only map at `:1600-1602`. Because `prReadyToMergeSolo` rejects any
  `CheckConclusion` outside `{success, ""}`, `pending` now blocks the **ready-notify**; and
  `ciPassing := !CIFailing && !CIPending` gates the **auto-merge arm**.
- The arm is relocated out of `pushAndCreatePR:1475` into the reconciler healthy branch
  (`policyActive && ciPassing && prReadyToMergeSolo`) — the only place CI truth is polled. Arming
  at PR-create (before CI starts) is eliminated, closing the "`--auto` merges to unprotected
  `main` while checks run" hole. ADR-024 Context + §c document the `--auto`/non-required-checks
  correction honestly.
- Adding `EnablePRAutoMerge` to `prPendingChecker` is **sound** — the production concrete type
  already implements it; no factory change; only test fakes need a stub. Confirmed.

**BLOCKER-2 (Phase-0 detector could not reach the precondition-failure orphan) — RESOLVED.** The
detector is broadened to adopt `review`/`in_progress` items with `PrNumber>0` **OR** `PrURL!=""`
(Class B), not solely `PrNumber==0` (Class A). Verified that Class B was genuinely unreachable by
all three existing paths, and that the broadened filter now catches it. The `in_progress` routing
uses the two valid hops `in_progress→review→pr_pending` (confirmed against the transition table),
each precondition-guarded. The Class-A URL parse can reuse `prNumberFromURLRe`
(`storage_backlog.go:565`), which exists in-package. The idempotency guard (Task 3.2.1b) prevents
the *sequential* duplicate — see Concern 2 for the residual concurrent window.

---

## Concerns (new — introduced by the patches)

- [ ] **E7's two-step `in_progress→review` hop feeds the bouncing detector → a spurious
  `bouncing` + `rework_cap` double-flag for churning `SkipReviewGate` policy items.**
  `CountReviewCyclesSince` counts every `in_progress→review` `BacklogStatusEvent`. E7 (Task
  3.2.1a) routes a `SkipReviewGate` policy item `in_progress→review` on each work-session exit —
  and such items **never receive a PASS verdict** (no review gate), so `hasPass` is permanently
  false, which is exactly `isBouncing`'s trigger condition. Trace: initial work exit (event #1) →
  CI fails → `AutoReopenForPRFix` spawns fix #2, exit (#2) → CI fails → fix #3, exit (#3) →
  `CountReviewCyclesSince==3` with `!hasPass` ⇒ `reconcileBouncingItems` fires a `bouncing` stuck
  row + WARNING notification, at ~the same tick the shared cap (`workCount>=3`) fires `rework_cap`.
  This interaction is **new**: before this feature, `SkipReviewGate` items went straight
  `in_progress→done` and never entered this loop. ADR-024 §b's "escalation notifications cannot
  double-fire" claim covers only `rework_cap` vs `pr_ready_unmerged` — **not** `bouncing` vs
  `rework_cap`. Not a blocker (the shared cap still bounds churn — a terminal state exists; this is
  notification noise, not unbounded churn), but the plan should either (a) exclude E7-originated
  `in_progress→review` hops from the bounce counter, or (b) explicitly accept and document the
  possible `bouncing`+`rework_cap` double-notification and correct ADR-024 §b's double-fire
  wording. — Files to touch on fix: `session/backlog_lifecycle.go` (E7 routing / bounce query) or
  ADR-024 §b + Accepted Risks.

- [ ] **The Task 3.2.1b idempotency guard is check-then-act — it closes the sequential/retry
  duplicate but leaves a residual concurrent TOCTOU window, so it does not fully "prevent the E7
  caller manufacturing a duplicate PR."** The guard reloads the item and returns early on
  `PrNumber>0`, but `CreatePR` runs *before* the `review→pr_pending` transition and the guard has
  no lock/transaction. Two concurrent `onSessionExited` goroutines for the same session (dispatched
  via `go il.parent.onSessionExited(...)`, `backlog_lifecycle.go:370`) could both observe
  `PrNumber==0`, both call `CreatePR`, and open **two** GitHub PRs; only one's `pr_pending`
  transition then wins (the DB precondition serializes the *status* write, not the PR creation).
  The existing reuse check at `:1433` doesn't help — it reads the stale passed-in `item`, not a
  reload. The realistic trigger is a duplicated `EventExited` (the E7 and PASS callers are mutually
  exclusive by `SkipReviewGate`), so the window is narrow, but a duplicate PR is precisely the
  failure the guard exists to stop. Not a blocker if double-delivery of `EventExited` is known
  impossible — but the plan/ADR-025 assert full prevention. — Either confirm `EventExited` is
  deduplicated upstream and note it, or tighten the guard (e.g. stamp an in-flight marker / rely on
  a unique-PR-per-branch constraint) and soften the "prevents a duplicate PR" claim to "prevents
  the sequential-retry duplicate." Files: `session/backlog_lifecycle.go`, ADR-025 Part 2.

## Minors

- **Round-1 concerns confirmed addressed in text (not papered over):**
  - *Kill-switch cheap-AND-live*: Task 2.1.1b wires an `atomic.Bool`-backed `FeatureController`
    (`IsEnabled` is an atomic load, toggled by `UpdateFeatureFlag`), mirroring the existing
    `"backlog"` controller — genuinely cheap and runtime-live. The stale-`cfg`-snapshot and
    per-call `LoadConfig` anti-patterns are both explicitly rejected. Verified the mechanism exists.
  - *Dead-session hydration*: Task 0.1.1a routes the Instance-PR lookup through
    `FindInstanceDataByID` (a direct `ListInstanceData` read) and explicitly forbids
    `registry.WithInstance`/`Acquire`, so no `LiveInstance`/`onConstruct` side effect per sweep.
    Verified `FindInstanceDataByID` is non-hydrating. The ended-session retention dependency is
    called out.
  - *Default-OFF regression*: ADR-024 §a states it is a deliberate, accepted, NOT-additive
    behavior change (non-policy review-passing items stop auto-merging until armed), with the
    ship-ON alternative considered and rejected, and a release-note callout. Reasonable to accept.
- **Accepted risks are reasonable to accept (none is a hidden blocker):**
  - *Shared rework-cap starvation*: the cap bounds **total** autonomous churn per item and parks a
    capped item in `pr_pending` with a durable `rework_cap` row — a clean escalation, not a dead
    end. `notifyReworkCapHit` copy is to note the shared budget. Acceptable.
  - *GitHub-outage no-escalation*: `IsPRMerged`/`GetPRStatus` errors `continue`; deferred with a
    noted follow-up ("N consecutive failed reconciles"). Acceptable for this feature's scope.
  - *No-checks-yet vs no-CI ambiguity*: an empty rollup reads as passing; mitigated because the arm
    is poll-gated ≥1 tick after PR-create (by which point GitHub has created the check runs) and
    this repo always produces CI. Acceptable.
- ADR-024 §b's "escalation vs ready notifications cannot double-fire" should be widened to
  acknowledge the `bouncing`↔`rework_cap` pair (Concern 1), or the E7 hop excluded from the bounce
  counter so the claim stays true.
- Verified sound, no action: the tri-state map keeps the empty-rollup ("no CI") case as passing by
  construction (both bools false); the healthy-branch entry guard (`!CIFailing && !HasBlockingReviews
  && !HasConflicts`) still admits a pending-CI PR, which then correctly fails `prReadyToMergeSolo`
  (no notify) and `ciPassing` (no arm) — pending CI is handled without a new branch. Every new
  status write still routes through `TransitionBacklogItemStatus` (audit trail intact). No
  out-of-scope creep.

---

## Round 1 (resolved — for the record)

Round 1 returned **BLOCKED** on two blockers, both now addressed and re-verified against source:

1. **"CI green" was actually "CI not-failing"** — pending/absent CI (`CheckConclusion==""`) read
   as green, so the ready-notify could fire and (via PR-create-time `--auto` arming to unprotected
   `main`) a merge could land before CI finished. **Resolved** by the CI tri-state (`CIPending` in
   `PRStatus`, mapped to `pending` in `PRInfo`), gating both the ready-notify and a relocated,
   `ciPassing`-gated auto-merge arm on positively-passing CI. ADR-024 corrects its false `--auto`
   premise.
2. **Phase-0 orphan detector could not adopt the precondition-failure (Class-B) orphan** — the
   `PrNumber==0`-only filter missed items left with `PrNumber>0` + status `review`/`in_progress`
   after a lost `review→pr_pending` race, and all three rescue paths require `pr_pending`.
   **Resolved** by broadening the detector to `PrNumber>0 OR PrURL!=""`, routing `in_progress`
   orphans via the valid two-hop `in_progress→review→pr_pending`, plus an entry idempotency guard
   on `pushAndCreatePR` (residual concurrent window noted as Concern 2).
