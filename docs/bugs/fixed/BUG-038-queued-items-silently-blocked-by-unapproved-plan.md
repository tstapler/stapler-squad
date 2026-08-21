# BUG-038: Queued Items Blocked by DequeueNextQueuedItems' Planning Gate Retry Silently Forever — No Durable Signal, No UI Path to Unblock [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22, partial — visibility only; see Not Fixed)
**Discovered**: 2026-07-22 — while investigating a user report that the kanban board showed items "queued for a while" with no visibility into why (led directly to BUG-037). After fixing the board's rendering gap, three previously-invisible queued items appeared; one (`e99d3f4a-1d7e-431b-bd28-a46f617dd055`, "Omnibar creation hangs and gets stuck so I can't create a new session") had sat queued for 4 days with the item detail page showing "No items are currently in progress" (0/2 WIP slots used) — i.e. plenty of capacity, yet nothing dequeued it.
**Fixed**: 2026-07-22 — `session/domain/backlog.go`, `session/backlog_lifecycle.go`, `server/services/backlog_service_stuck.go`, `proto/session/v1/backlog.proto`, `web-app/src/components/backlog-stuck/stuckReason.{ts,css.ts}`
**Impact**: `DequeueNextQueuedItems` (`server/services/backlog_service_triage.go`) refuses to claim a queued item unless `SkipPlanning=true` or `PlanApproved=true` — a deliberate defense-in-depth guard per its own doc comment. But for an item that never goes through a distinct "approve plan" step (this item's pipeline is `default`, and there is no "Approve Plan" button anywhere reachable in the UI for it), the guard blocks forever: every 60-second reconcile tick re-attempts the claim, fails identically, and logs one `WARNING` — with no stuck-state row, no board badge, no notification. Live-confirmed: three items (`e99d3f4a...`, `e1fb6825...`, `12981e9d...`) were stuck this way simultaneously, one for 4+ days.

## Live Symptoms

```
[DequeueNextQueuedItems] claim blocked by planning gate item=e99d3f4a-1d7e-431b-bd28-a46f617dd055: plan must be approved or skip_planning must be true before spawning work session — leaving queued
```
— repeating identically every 60 seconds in `staplersquad.log`, for the same 3 items, indefinitely.

- The item had already completed two full work+review cycles (real plan content visible in its "Planning" section, a real prior work session, a real review verdict) — the planning gate's own code comment anticipated this exact shape ("a pre-existing queued row from before that ordering fix") but implemented no remediation for it.
- `ApprovePlan` (the RPC that would set `PlanApproved=true`) exists and would succeed for this item (its `PlanArtifactsPath` is set and the file exists), but no UI button calls it — confirmed via `mcp__claude-in-chrome__find` querying the entire item-detail page for an "Approve Plan" button: none found.
- Before BUG-037's board fix, these items were also invisible on `/backlog/board` entirely, compounding the lack of visibility.

## Root Cause

`DequeueNextQueuedItems`' planning-gate refusal (`server/services/backlog_service_triage.go:546-553`) is correct as a *safety* check — it prevents an item without an approved plan from silently spawning unapproved work. But refusing the claim is a **terminal** dead end for the item: nothing else in the codebase ever re-attempts the claim differently, sets `PlanApproved=true` on its behalf, or surfaces the block to a human. Combined with the "default" pipeline mode apparently never routing through an explicit plan-approval UI step, an item can reach this state with no path forward at all — not a race, not a timing issue, a structural gap.

## Fix Applied (visibility only — see Not Fixed)

Added a new durable stuck-reason, `StuckReasonPlanNotApproved` (`plan_not_approved`), following this codebase's established stuck-detector pattern (mirrors `reconcileOrphanedTriageItems`):

1. **`session/domain/backlog.go`**: new `StuckReasonPlanNotApproved` constant, added to `AllStuckReasons`/`IsValid`.
2. **`session/backlog_lifecycle.go`**: new `reconcilePlanNotApprovedItems` detector, wired into `ReconcileStuck`'s 60s sweep. Flags a queued item whose `SkipPlanning=false && PlanApproved=false` and whose `QueuedAt` is older than `planApprovalStaleness` (5 minutes — short, since unlike a running session there is no legitimate "still working" explanation for this condition). Writes a durable `BacklogStuckState` row via `MarkStuck`, notifies once (DB-backed dedup, matching every other detector). Also added an explicit `case domain.StuckReasonPlanNotApproved` to `selfHealStuck`'s status-anchored resolve switch, so the row clears automatically once the item leaves `queued` (approved-and-dequeued, manually reopened, etc.) — this switch is NOT a generic lookup, so a new reason must be added here explicitly or it silently never resolves (caught by a failing test during this fix).
3. **`proto/session/v1/backlog.proto`** + regenerated bindings: new `STUCK_REASON_PLAN_NOT_APPROVED = 10` enum value.
4. **`server/services/backlog_service_stuck.go`**: added the new reason to both `toProtoStuckReason`/`fromProtoStuckReason` conversion switches.
5. **`web-app/src/components/backlog-stuck/stuckReason.{ts,css.ts}`**: label ("Waiting on plan approval"), icon (🟡), and CSS chip class — `Record<StuckReason, T>` maps make omitting a new enum value a TypeScript compile error, not a silent blank chip.

This item is now visible on the "Stuck Backlog Items" surface and (via BUG-037's board fix) on the kanban board itself, with a clear reason instead of silence.

## Files Affected

- `session/domain/backlog.go` — new `StuckReasonPlanNotApproved` constant
- `session/backlog_lifecycle.go` — `reconcilePlanNotApprovedItems` detector, `selfHealStuck` resolve case, sweep wiring
- `session/backlog_lifecycle_stuck_test.go` — new regression tests
- `proto/session/v1/backlog.proto` + `gen/proto/go/session/v1/backlog.pb.go` + `web-app/src/gen/session/v1/backlog_pb.ts` — new enum value
- `server/services/backlog_service_stuck.go` — proto↔domain conversion
- `web-app/src/components/backlog-stuck/stuckReason.ts`, `stuckReason.css.ts` — label/icon/class

## Verification

- `TestReconcilePlanNotApprovedItems_should_writeDurableRowNotifyOnce_When_QueuedItemStale` — a queued, unapproved, stale item gets a durable row and exactly one notification across repeated ticks.
- `TestReconcilePlanNotApprovedItems_should_notFlag_When_QueuedRecently` — the 5-minute staleness buffer isn't over-eager.
- `TestReconcilePlanNotApprovedItems_should_notFlag_When_SkipPlanningTrue` — items legitimately bypassing planning are never flagged.
- `TestSelfHealSweep_should_resolvePlanNotApprovedRow_When_ItemLeavesQueued` — **caught a real bug during this fix's own development**: my first pass added the detector but forgot the `selfHealStuck` switch case, so the row never auto-resolved; the test failed exactly as expected against that intermediate state, confirming the test is load-bearing.
- **Verified to fail against pre-fix code**: `git stash` on the three backend files reproduces a build failure (`undefined: domain.StuckReasonPlanNotApproved`, `reconcilePlanNotApprovedItems undefined`) in the new test file.
- All pre-existing `TestReconcileOrphanedTriageItems_*` and `TestSelfHealSweep_*` tests pass unmodified.
- Frontend: `jest --testPathPatterns="stuckReason|StuckItem"` — 72/72 pass. `npx tsc --noEmit` clean (the `Record<StuckReason, T>` maps would have failed to compile if any were left unwired).
- `go build ./...`, `golangci-lint run ./session/... ./server/services/...` — clean.

## Not Fixed (scoped out — a product/architecture decision, not a bug fix)

This fix makes the block **visible**; it does not remove it. The underlying question — should the "default" pipeline mode require an explicit plan-approval step at all, and if so, where does the UI action to approve it live? — is a real design decision this fix deliberately did not make unilaterally. Candidate directions, none implemented here:
1. Build the missing "Approve Plan" UI action for pipeline modes that reach this state (the RPC already exists and works).
2. Auto-set `PlanApproved=true` for items with a prior completed work session (arguably, planning has already functionally happened by the time an item cycles back to `queued` for a second round).
3. Exempt the "default" pipeline mode from the gate entirely if it was never meant to have a distinct planning phase.

Any of these changes real gating behavior for the WIP-cap queue — worth a properly-scoped follow-up (likely `sdd:quick` or `sdd:full` depending on which direction, per `.claude/skills/backlog-feature-improvement/SKILL.md`'s own routing table for "non-configurable pipeline steps"), not a `sdd:fix-bug`-shaped change.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — `DequeueNextQueuedItems`' own doc comment explicitly anticipated this exact failure mode ("a pre-existing queued row from before that ordering fix") as a possible outcome of its own safety check, but the check's designer stopped at "refuse the claim" without also asking "then what happens to this item forever after." The safety property (never spawn unapproved work) was implemented; the corresponding liveness property (every item eventually either progresses or is visibly flagged) was not.

**Earliest achievable enforcement**: A regression test is the practical level for the detector logic itself. A structural note for the future: this is (at least) the 8th stuck-reason detector added to this file, each following an identical shape (query condition → MarkStuck → notify-once → wire into `selfHealStuck`'s switch). The `selfHealStuck` switch specifically is a repeat trap — a new detector's row silently never resolves unless its author remembers this second, separate wiring point (exactly what happened during this fix's own first pass, caught only by its own test). A worthwhile follow-up: refactor `MarkStuck`'s call sites to also accept the resolve condition as a parameter (e.g. `MarkStuckWithResolveCondition(reason, expectedStatus, ...)`), collapsing detector-registration and resolve-registration into one call so a new reason can't add one without the other.

**Recurring shape**: A safety check with no corresponding liveness/visibility guarantee — a variant of this session's broader recurring theme ("an action can silently fail to make progress"), but distinct from the teardown-notification gaps (BUG-027/034/035) in that nothing here is torn down or exited; the item is simply, permanently retried-and-refused. Distinct also from BUG-036/032's "review verdict trusted incorrectly" shape. Reinforces (for the 8th time today) the case for a dedicated architecture-review pass specifically auditing "for every state a backlog item can enter, is there a guaranteed path either forward or to a human-visible stuck signal."

## Related

- BUG-037 (`docs/bugs/fixed/BUG-037-backlog-board-hides-queued-pr-pending-refining-items.md`) — the board-visibility fix that surfaced these items in the first place.
- Discovered on the same live investigation as BUG-027/031/032/036, all touching backlog item `9264efe7-b4c2-455a-9e2a-ab0196a63ecd` and its sibling items earlier the same day.
