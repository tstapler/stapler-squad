# BUG-040: `pr_pending` Item Can Lose Its PR Reference and Become Permanently Unreconcilable [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22/23, `backlog-feature-improvement` audit — live item `9264efe7-b4c2-455a-9e2a-ab0196a63ecd` ("Backlog History feature Broken", the same item named the sole real stuck item in this doc's 2026-07-22 update, PR #173)
**Fixed**: 2026-07-22 — `session/backlog_lifecycle.go`, `session/domain/backlog.go`, `server/services/backlog_service_stuck.go`, proto + generated bindings
**Impact**: An item can end up with `status = pr_pending` but `pr_url = ""` and `pr_number = 0`. `ReconcilePRPending` (`session/backlog_lifecycle.go:3207+`) only acts on items in `pr_pending` status, and every action it takes (`GetPRStatus`, `AutoReopenForPRFix`'s fix context, `EnablePRAutoMerge`) requires a real `PrNumber`. Once the fields are empty, the item has nothing left to poll and sits in `pr_pending` forever — the terminal dead end this whole subsystem's self-heal machinery (`docs/tasks/backlog-feature-improvement.md`) was built to avoid. `ListStuckBacklogItems` did flag the item, but only via the unrelated, stale `STUCK_REASON_AUTONOMOUS_STUCK` — nothing in the detector set described "pr_pending with no PR."

## Live Evidence (at time of filing)

```
$ sqlite3 sessions.db "SELECT status, pr_url, pr_number FROM backlog_items WHERE id='9264efe7-...'"
pr_pending|| 0
```

PR #173 itself: `state: CLOSED`, `mergedAt: null`, `mergeStateStatus: DIRTY`, `mergeable: CONFLICTING`, closed `2026-07-22T17:29:31Z` — closed without merging, not by `closeIfSupersededByMain` (that path always transitions straight to `done`, never leaves the item in `pr_pending`).

Status-event history for the item's last relevant transitions (all timestamps below are as recorded; the `review→pr_pending` note timestamp is local (`-07:00`), the GitHub PR-closed timestamp is UTC — both refer to `2026-07-22`):
```
in_progress → review     2026-07-22T17:12:13-07:00  (no note)
   [PR #173 closed on GitHub while item was "review": 2026-07-22T17:29:31Z UTC]
review      → pr_pending 2026-07-22T16:01:25-07:00  (no note)   <- pushAndCreatePR, reusing cached PR #173
   [no further status events — item never left pr_pending again]
```

## Root Cause (confirmed via DB trace — logs for the exact window had already rotated out, as anticipated when this was filed)

Two structural gaps in `session/backlog_lifecycle.go` were found; **the live incident traces to root cause #2**, but both were real and independently fixed.

**#1. `pushAndCreatePR`'s field-persist was best-effort and non-blocking** (the `else` branch, taken only when creating a *brand-new* PR — no cached `PrNumber`/`PrURL` to reuse). It called `l.storage.UpdateBacklogItem(...)` to cache the freshly created `PrURL`/`PrNumber` — but a failure there was only `log.WarningLog`'d, never checked. The function proceeded unconditionally to `resolveToPRPending(ctx, item.ID, "", "pushAndCreatePR")` regardless of whether the persist actually succeeded. A transient storage error at exactly that point would silently produce this bug's exact shape.

  **Ruled out for the live incident**: by the time of the final `review → pr_pending` transition, `item.PrNumber` was already `173` (from the original PR creation, well before this incident window), so `pushAndCreatePR` took the *reuse* branch (`item.PrNumber > 0 && item.PrURL != ""`), which never calls `UpdateBacklogItem` at all — root cause #1 was structurally not exercised on this call.

**#2. `ReconcilePRPending`'s closed-PR branch cleared fields before confirming the reopen succeeded** — **this is what actually fired**. Sequence reconstructed from the DB trace:
1. PR #173 already existed and was cached on the item from an earlier work cycle.
2. The item entered "review" at `17:12:13-07:00`; PR #173 was closed without merging on GitHub 17 minutes later (`17:29:31Z`) — while the item was "review," not "pr_pending", so `ReconcilePRPending`'s closed-PR handling (which only scans `pr_pending` items) never saw it at this point.
3. `pushAndCreatePR` ran again at `16:01:25-07:00` (review → pr_pending): since `item.PrNumber` (173) was still cached, it took the reuse branch — re-entering `pr_pending` referencing the now-stale, already-closed PR #173, with no new persist call.
4. On its next tick, `ReconcilePRPending` found the item `pr_pending` with `PrNumber=173>0` (passing `FindPRPendingItems`' filter), called `GetPRStatus(173)`, saw `IsClosed=true`, and — since `closeIfSupersededByMain` didn't apply — **unconditionally cleared `PrURL`/`PrNumber` to empty** *before* calling `fixSpawner.AutoReopenForPRFix` to reopen the item.
5. `AutoReopenForPRFix` (`server/services/backlog_service_triage.go`) has several no-op/error return paths (an active work session already running, the rework cap, or any of its own storage-call errors) that leave the item exactly where step 4 left it: `pr_pending`, fields already cleared, never actually reopened. The live evidence's "16 historical work sessions, none currently unended" and "rework cap 20, not hit" rule out the two documented no-op guards specifically — the precise internal failure (most likely a storage-call error inside `AutoReopenForPRFix`, e.g. `ListItemSessions` or the transition's own precondition) could not be pinned down further: the log window covering this tick had already rotated out by the time of investigation, exactly as anticipated when this bug was filed. What's conclusively established from the DB alone is the *write-ordering* defect: nothing after step 4 ever succeeded, and step 4's clear was irreversible without a working reopen — a definitional dead end.
6. No status event was ever recorded after the `review → pr_pending` transition — consistent with `AutoReopenForPRFix` returning early (nil or error) before its own `TransitionBacklogItemStatus` call ever landed.

Both paths converge on the same failure shape: **a `pr_pending` item with no PR reference is invisible to every downstream reconciler**, since every one of them is keyed on `PrNumber`/`PrURL` being present.

## Fix Applied

**Root cause #1** (`pushAndCreatePR`, `session/backlog_lifecycle.go`): a failure persisting the newly created PR's `PrURL`/`PrNumber` now calls `stayInReviewAndNotify(...)` and returns, exactly like a push/PR-creation failure — the item stays in `review` (code committed, PR creatable/retriable) instead of silently entering `pr_pending` with nothing to look the PR back up by.

**Root cause #2** (`ReconcilePRPending`'s closed-PR branch, `session/backlog_lifecycle.go`): restructured so the stale `PrURL`/`PrNumber` are cleared **only after** confirming `AutoReopenForPRFix` actually transitioned the item off `pr_pending` (re-fetches the item and checks its status after the call). If `AutoReopenForPRFix` errors, or if it returns `nil` without transitioning anything (its legitimate no-op guards), the fields are left intact so the item remains visible and retryable on the next tick instead of becoming an untraceable dead end. The `pr_ready_unmerged` stuck-row resolve (which is about the *old* PR reference, not the field clear) still happens unconditionally, as before.

**Phase D structural backstop**: a new `domain.StuckReason` — `StuckReasonPRPendingNoPR` (`"pr_pending_no_pr"`) — plus a new periodic detector, `reconcilePRPendingWithoutPRItems`, wired into `ReconcileStuck`'s detector list. It flags any item in `pr_pending` status with `pr_number == 0`, writes a durable, notify-once stuck row (mirroring `reconcilePlanNotApprovedItems`' shape), and is picked up by `ListStuckBacklogItems`/the `/unfinished` UI via the existing `StuckBacklogItem` proto surface (`STUCK_REASON_PR_PENDING_NO_PR = 11` added to `proto/session/v1/backlog.proto`, `make proto-gen` run, both `toProtoStuckReason`/`fromProtoStuckReason` mappings and the frontend `stuckReason.ts` `Record<StuckReason, T>` maps updated — the latter is TypeScript-compile-checked exhaustive, so a future new proto value can't silently render blank). `selfHealStuck` resolves the row once the item leaves `pr_pending` (successfully reopened, or manually recovered) — same status-anchored pattern used by every other reason with a clear non-terminal anchor. This closes the class even if a *future* write-ordering mistake reproduces the same shape.

## Files Affected

- `session/backlog_lifecycle.go` — `pushAndCreatePR` (fix #1), `ReconcilePRPending`'s closed-PR branch (fix #2), new `reconcilePRPendingWithoutPRItems` detector, `ReconcileStuck` wiring, `selfHealStuck` case
- `session/domain/backlog.go` — `StuckReasonPRPendingNoPR` constant, `AllStuckReasons`, `IsValid`
- `server/services/backlog_service_stuck.go` — `toProtoStuckReason`/`fromProtoStuckReason` mapping
- `proto/session/v1/backlog.proto` + regenerated Go/TS bindings (`make proto-gen`)
- `web-app/src/components/backlog-stuck/stuckReason.ts` + `stuckReason.css.ts` — label/icon/class entries for the new reason
- `session/backlog_lifecycle_test.go` — updated `fakePRFixSpawner` (now supports simulating success/no-op/error), 2 new regression tests for fix #2, 1 new regression test for fix #1
- `session/backlog_lifecycle_stuck_test.go` — 3 new regression tests for the new detector
- `server/services/backlog_stuck_rpc_test.go` — new proto-mapping round-trip case

## Verification

- `TestReconcilePRPending_ClosedWithoutMerge_ClearsPRFieldsAndReopens` — updated: the fake spawner now performs the real `pr_pending → in_progress` transition on success, and the test asserts fields are cleared *and* the item actually left `pr_pending`.
- `TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenNoOps` — new BUG-040 regression: a no-op `AutoReopenForPRFix` (nil error, no transition — the real contract of its active-work-session/rework-cap guards) must leave `PrNumber`/`PrURL` untouched.
- `TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenErrors` — new BUG-040 regression: same invariant when `AutoReopenForPRFix` returns a genuine error.
- `TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies` — new BUG-040 regression for fix #1: forces the PR-fields persist to fail (by closing the real on-disk SQLite connection right after `CreatePR` succeeds, via a `dbClosingPRCreator` test double — chosen over an interface-wrapping fake because several `Storage` methods on this codebase's hot path, e.g. `GetWorktreeDataBySessionUUID`, shortcut straight to the concrete `*EntRepository` via a type assertion, which a wrapper type would silently break) — then re-opens a fresh connection to the same file to confirm the item stayed in `review` with `PrNumber=0`/`PrURL=""`, never silently reaching `pr_pending`.
- `TestReconcilePRPendingWithoutPRItems_should_writeDurableRowNotifyOnce_When_PrNumberZero`, `_should_notFlag_When_PrNumberSet`, `TestSelfHealSweep_should_resolvePRPendingNoPRRow_When_ItemLeavesPRPending` — new detector coverage, mirroring `reconcilePlanNotApprovedItems`' existing test shape.
- `TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString` — extended with the new reason's round-trip case.
- `go build ./session/... ./server/services/...`, `go vet ./session/... ./server/services/...`, `golangci-lint run ./session/... ./server/services/...` — clean.
- `go test ./session/...` (full package, 42s) and `go test ./server/services/...` (full package, 61s) — all passing, no regressions.
- `cd web-app && npx tsc --noEmit` — clean (confirms the `Record<StuckReason, T>` maps in `stuckReason.ts` are exhaustive for the new proto enum value).
- `cd web-app && npx jest --testPathPatterns=stuckReason` — 14/14 passing, including the existing "non-empty label/class/icon for every StuckReason enum value" exhaustiveness test.

## Live Data Recovery

The live repro item (`9264efe7-b4c2-455a-9e2a-ab0196a63ecd`) was still `pr_pending`/`pr_number=0` at verification time. This fix was developed and tested in an isolated worktree and has **not** been deployed to the running service as part of this fix (deploying via `make install-service` restarts the service and kills every live tmux session, including any active work session, per `.claude/rules/tmux-keep-server-on-restart.md` — not something to do unprompted from a background bug-fix run). Recommended follow-up once this fix is merged and deployed: call `AutoReopenForPRFix` (or the equivalent `TriggerRemediationNow`/`/unfinished` UI action, once wired for this reason) against `9264efe7` through the running application — not a direct DB edit — to push it through a fresh PR attempt.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Semantic/Intent gap paired with an Integration Gap. Root cause #1 is "a failure was logged but not treated as a failure" (the write's result was never checked against the invariant it protects). Root cause #2 is "two writes that must be atomic from the caller's perspective (clear the stale reference, confirm the replacement is in flight) were split across a call boundary with no rollback on the far side's failure" — an integration gap between `ReconcilePRPending` and `AutoReopenForPRFix`, whose no-op contract (`return nil` for "nothing to do yet, retry later") is correct in isolation but dangerous when a caller treats `nil` as "succeeded."

**Earliest achievable enforcement**: For root cause #1, the regression test is the practical level — the invariant ("every write into `pr_pending` must have carried real PR fields") is a runtime contract across two calls, not something a type system or linter could enforce here without much heavier machinery (e.g. a smart constructor for the transition that requires proof the fields were persisted — arguably over-engineered for a single call site). For root cause #2, the fix itself *is* the earliest achievable enforcement: re-fetching and checking status before clearing is exactly the "prove the precondition holds before the destructive write" pattern; a lint rule couldn't distinguish this from any other two-call sequence. The `pr_pending_no_pr` detector is the actual durable defense — a runtime backstop that exists specifically because call-site enforcement can be missed again.

**Recurring shape confirmed**: this is another instance of the pattern this project's `docs/tasks/backlog-feature-improvement.md` has repeatedly named — "a write silently doesn't happen or happens out of order, and nothing detects the resulting dead end" (see BUG-027, BUG-030, BUG-036, BUG-038's own write-ordering/silent-gap shapes). The `pr_pending_no_pr` detector generalizes the defense the same way `StuckReasonSpawnFailed`/`StuckReasonPlanNotApproved` did for their own instances of this shape: a durable, status-anchored, notify-once row that survives even if a *future* variant of the same write-ordering mistake gets introduced somewhere else in this subsystem.

## Related

- BUG-036 (`docs/bugs/fixed/BUG-036-reconcile-pr-closed-branch-missing-superseded-check.md`) — same closed-PR branch this fix touches; that fix added the `closeIfSupersededByMain` check this fix's field-clear now sits immediately after.
- BUG-030 (`docs/bugs/fixed/BUG-030-autoreopen-spawn-silent-stall.md`) — an earlier instance of "AutoReopen* silently stalling" in the adjacent `AutoReopenAfterFailedReview` path; this bug is the `AutoReopenForPRFix`/`pr_pending` sibling of that same family.
- `docs/tasks/backlog-feature-improvement.md` — ongoing audit that discovered this bug; see its BUG-040 entry for the original filing context.
