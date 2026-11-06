# Implementation Plan: plan-feedback

**Feature**: Inline "send back for re-planning" feedback box on backlog items
at `in_progress`/`review`/`pr_pending`/`done`, sending them to `ready` with
the operator's feedback recorded and retriage started, in one submit.
**Date**: 2026-09-14
**Status**: Ready for implementation
**ADRs**: [ADR-001-send-back-target-status-ready](../decisions/ADR-001-send-back-target-status-ready.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| Send-back-with-feedback | The combined operator action: type free-text feedback, submit once, item moves to `ready` and a new triage run starts carrying that feedback. | The feature's name throughout code/tests — do not call it "reject and regenerate" (that's the distinct, unchanged ADR-002 flow at `ready`/`pending_review`). |
| `SendBackFeedbackBox` | New React component (`web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx`) rendering the toggle button + inline form for send-back-with-feedback. | Forked from `PlanVerdictBox.tsx`'s toggle/form pattern (build-vs-buy.md §4). |
| `handleSendBackWithFeedback` | New `useCallback` in `BacklogItemDetail.tsx` that sequences the three RPC calls behind one submit. | Mirrors `handleRejectPlan`'s try/catch/`actionLoading`/toast shape (`BacklogItemDetail.tsx:990-1007`). |
| `stopLiveWorkAndReviewSessions` | New shared Go helper on `*BacklogService` (`server/services/backlog_service_triage.go`) that stops every unended work/review `ItemSession` for an item (tmux teardown + `EndedAt` marked). | Extracted from `forceResetItem`'s existing inline loop (`backlog_service_triage.go:1122-1144`) — not new logic, a refactor to make it callable from a second site. |
| `PlanRejectionReason` | Existing persisted field (`session.BacklogItemData`) reused, unchanged, to hold the send-back feedback text. | No new DB field — see ADR-001 and architecture.md §2. |
| `overrideReason` | Existing `TransitionBacklogItemStatusRequest` field. The feedback text is passed as this transition's `overrideReason`, satisfying `ErrVerdictClearRequiredForReady`'s guard when sending back from `review`/`pr_pending` with a recorded PASS verdict, and producing an audit-trail progress note for every send-back. | `session/domain/backlog.go:644-657`; `backlog_service_lifecycle.go:791-800`. |
| `activeWorkSessionCount` | Existing derived value (`BacklogItemDetail.tsx:249`) counting unended `role: "work"` linked sessions. | Reused as-is to drive the new `InlineNotice`; not recomputed. |
| `send_back_ready` (action id) | Existing `BacklogActionId` (`itemActions.ts:41`), already gated to `in_progress`/`review`/`pr_pending`/`done` via `CAN_SEND_BACK_READY` (`itemActions.ts:122-127`). | Unchanged — only what renders for it changes (from a bare button to `SendBackFeedbackBox`). |
| `send_back_refining` | Dead action id/case referenced in `BacklogItemDetail.tsx`'s switch (`:793-795`) and toast dictionary (`:92`) but never added to any `itemActions.ts` eligibility set. | Removed by this plan, not wired up — target status is `ready`, not `refining` (ADR-001). |
| Changes-requested (plan review status) | Existing derived UI state (`derivePlanReviewStatus`, `planReviewStatus.ts:24-32`) — non-empty `planRejectionReason` renders `PlanVerdictBox`'s "Revisions requested" card. | This plan's `ready`-targeted send-back lands the item in exactly this state for free (architecture.md §1) — the failure-recovery path for a failed `triggerTriage` step, *except* the P1 #1 case below, where it doesn't. |
| `SendBackError` | New small `Error` subclass thrown by `handleSendBackWithFeedback` (Task 2.2.1a), carrying `failedAt: "transition" \| "reject" \| "triage"` and `itemStatusAfterFailure: string` (the item's real status re-fetched after the failure, since `triggerTriage`'s own internal `ready→idea` CAS — `backlog_service_trigger_triage.go:249-260` — can commit before a *later* step in that same call fails, per pre-mortem P1 #1). Lets `SendBackFeedbackBox` (Task 2.1.1b) render distinct recovery copy per ux.md Surface 7's table instead of one generic message for every failure. | Resolves BLOCKER 5.2 and pre-mortem P1 #1. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Feedback-capture UI | Inline expand-to-form (fork `PlanVerdictBox`'s toggle/form) | Existing in-repo convention (`PlanVerdictBox.tsx`, `TriageReviewPanel.tsx`) | Radix `Dialog` modal (already a dependency) | Every feedback-capture surface in this codebase is an inline toggle, never a modal (ux.md §1); a modal would need its own focus-trap code for no behavioral gain and would miscalibrate weight (implies something more disruptive than it is). |
| Combined submit sequencing | Transaction Script — one ordered sequence of `await` calls with no branching business logic of its own | PoEAA (Fowler), Transaction Script | New merged backend RPC (e.g. `SendBackWithFeedback`) | ADR-002 already rejected duplicating `TriggerTriage`'s precondition/guard sequence (in-flight map, semaphore, orphan sweep) into a second handler; a merged RPC would either re-duplicate that machinery or just call `TriggerTriage` internally — no functional difference from calling it from the frontend (architecture.md §3). |
| Live-session teardown reuse | Extract Method — pull `forceResetItem`'s inline loop into `stopLiveWorkAndReviewSessions` | Refactoring (Fowler), Extract Method | Write a second, separate copy of the loop inside `TransitionBacklogItemStatus` | Two independently-maintained copies of "stop live work/review sessions" is exactly the drift risk ADR-002 flagged for guard duplication — one small extraction avoids it for a 13-line loop. |
| Send-back target status | Reuse `ready` as the landing state — no new intermediate status | Type-driven design (valid-states-only) | Target `refining` (matches the dead code's literal name) | `refining` is rejected by `TriggerTriage`'s status guard outright and requires a new guard change (requirements.md's Feasibility Risk); `ready` needs zero guard changes and is already `RejectPlan`'s exact post-condition (architecture.md §1). See ADR-001. |
| Feedback persistence | Reuse existing `PlanRejectionReason` value, no new field | Type-driven design (avoid redundant state) | New `SendBackFeedback` column/proto field | Once target status is fixed at `ready`, `PlanRejectionReason`'s actual contract ("outstanding feedback on the plan currently at ready") already matches this feature exactly — adding a parallel field would model the same concept twice (architecture.md §2). |
| Session-teardown scope (GoF) | No new creational/structural/behavioral pattern — a bounded, non-recurring loop doesn't warrant Strategy/Observer/Factory | GoF (applicability check) | — | The problem (stop N live sessions) occurs at exactly two call sites after this plan (`forceResetItem`, the new `TransitionBacklogItemStatus` block); Extract Method is sufficient. Revisit only if a third distinct caller with different stop semantics appears. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/backlog_service_trigger_triage.go` (`TriggerTriage`) | Partially remediated hotspot (`docs/reference/hotspot-ranking.md` row 4) | **Extend as-is** | This feature adds zero lines to this file — its status guard already accepts `ready`, its `feedback` param already flows into the prompt builders unchanged (architecture.md §6). |
| `server/services/backlog_service_lifecycle.go` (`TransitionBacklogItemStatus`, `RejectPlan`) | Not flagged by `docs/reference/hotspot-ranking.md`'s churn×complexity ranking — but a separate, independent check disagrees: `kibitzer run server/services --trigger batch` flags `TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:689`) as `[long-function] body spans 141 lines (over 40)`. Both are accurate on their own terms; this row previously asserted "not a flagged hotspot" unqualified, which only held for the churn-based ranking. | **Extend as-is** | The file already has several status-conditional blocks (`hasUnshippedCode`, `hasUnresolvedBlockers`, the idea/refining reset); one more conditional block for backward-to-`ready` teardown (Task 1.2.1a: a ~7-line `if` calling the already-extracted `stopLiveWorkAndReviewSessions`, not an inlined loop) follows the file's existing shape and doesn't meaningfully worsen the kibitzer long-function flag — a single-call addition through a helper that already lives elsewhere doesn't compound the violation the way inlining a new loop would. Splitting `TransitionBacklogItemStatus` itself is out of scope for this feature; it's a pre-existing condition this plan doesn't make meaningfully worse. |
| `server/services/backlog_service_triage.go`'s `forceResetItem` inline teardown loop | Not a hotspot, but about to gain a second call site | **Isolate via seam** (Extract Method into `stopLiveWorkAndReviewSessions`) | A second, independently-typed copy of "stop live work/review sessions for an item" is the same duplication-drift risk ADR-002 already reasoned about for `TriggerTriage`'s guard sequence — a one-time, low-cost extraction avoids it. |
| `web-app/src/components/backlog/BacklogItemDetail.tsx` (growing per-feature `useState`/handler pile) | Not flagged as a hotspot, but visibly accumulating (architecture.md §6) | **Extend as-is** (seam candidate for later) | One more handler (`handleSendBackWithFeedback`) plus two prop pass-throughs to `ActionsSection` follows the file's existing convention (mirrors `handleRejectPlan`). If a 4th/5th similar feedback-box variant appears later, extracting a shared `useFeedbackSubmit`-style hook is the seam — not warranted by this one addition. |

None of the areas this feature touches are flagged hotspots (churn×complexity) requiring a refactor-first pass. `TransitionBacklogItemStatus` is separately flagged by kibitzer's per-function long-function check (see row above) — that's a pre-existing condition this feature's small addition doesn't meaningfully worsen, not a reason to block or refactor-first here.

---

## Migration Plan

None. No schema, proto, or data changes — every RPC and persisted field this feature uses (`TransitionBacklogItemStatusRequest`, `RejectPlanRequest`, `TriggerTriageRequest`, `PlanRejectionReason`) already exists and is already exercised by other flows (stack.md, architecture.md §2-3).

## Observability Plan

- **Logs**: `stopLiveWorkAndReviewSessions`' best-effort failures (if `sessionStopper.StopSessionByUUID`/`storage.UpdateItemSessionEnded` error) are non-fatal and logged via `log.WarningLog()`, matching the existing idea/refining reset block's error-handling convention (`backlog_service_lifecycle.go:823`) — the transition itself is not rolled back on a teardown failure, since a leftover pane is a known, already-recoverable condition (`hasActiveWorkSession` blocks the next spawn until it's cleared, per pitfalls.md §1).
- **Teardown-failure UX (pre-mortem P1 #2 — explicit, justified choice, not an
  oversight):** the operator still sees the unqualified `"Feedback sent —
  retriage started."` success toast even when `stopLiveWorkAndReviewSessions`'s teardown
  fails; the failure is logged server-side only (`log.WarningLog()`), never
  surfaced in the toast or the RPC response. This is a deliberate best-effort
  silent-continue, for three reasons: (1) blocking or degrading the *send-back
  itself* — the operator's actual intent — on a teardown failure would be
  strictly worse UX than the rare leftover-session case it's guarding against;
  (2) `TransitionBacklogItemStatusResponse` (`proto/session/v1/backlog.proto:659-661`)
  carries only the updated `BacklogItem`, no warning/diagnostics field — adding
  one would be a proto change, which this feature's Migration Plan explicitly
  rules out ("no schema, proto, or data changes"); (3) the leftover session is
  already self-limiting and discoverable, not silently unbounded:
  `hasActiveWorkSession` blocks the next work-session spawn until the stale
  session is cleared (pitfalls.md §1), so an operator who tries to act on the
  item again gets a concrete, actionable signal at that point rather than never.
  If teardown failures turn out to be common enough in practice to warrant
  surfacing them proactively (not just observed here), the fix is a follow-up
  proto field on `TransitionBacklogItemStatusResponse`, not a workaround in this
  feature.
- **Metrics**: None added. This is a low-frequency, operator-initiated action (requirements.md NFR: "not applicable" for performance/scale) — no new counter/histogram is warranted.
- **Alerts**: None. Existing alerting (if any) around `TriggerTriage` failures and orphaned sessions already covers this path since it reuses those exact RPCs.

## Risk Control

- **Feature flag**: None. Matches ADR-002's precedent — the structurally identical `RejectPlan`/regenerate flow shipped without a flag, reusing already-tested `TriggerTriage`/`TransitionBacklogItemStatus` guards. The natural kill-switch already exists: removing an entry from `CAN_SEND_BACK_READY` (`itemActions.ts:122-127`) hides the new UI entirely without touching backend code.
- **Rollback procedure**: Revert the PR. No data migration to undo — the feature only writes through already-existing fields (`PlanRejectionReason`, item `status`, `ItemSession.EndedAt`), so a revert leaves no orphaned schema or half-migrated data.
- **Staged rollout**: Not applicable — single-operator, localhost-bound tool (requirements.md NFR: "internal, single-operator tool").

## Unresolved Questions

- [ ] Should the pre-existing `send_back_idea` ("↩ Return to Triage") backward transition get the same live-session teardown fix, since it has the identical gap (pitfalls.md §1 notes today's bare `send_back_ready` already lacked it, and `send_back_idea` shares the same code path shape)? — Out of scope for this plan (requirements.md only asks for the `ready`-targeted flow); flagged as a follow-up backlog item, not a blocker here. Owner: whoever files the follow-up item.
- Pre-mortem P2 items #3, #4, #5 are considered and deliberately deferred, not
  missed: **#3** (the same feedback string doing three different jobs —
  `overrideReason`, `PlanRejectionReason`, and the `TriggerTriage` prompt — and
  "Revisions requested" framing being nonsensical for an already-`done`/shipped
  item) is left as-is because `CAN_SEND_BACK_READY` already includes `done` by
  design (requirements.md) and narrowing it is a product decision this plan
  doesn't have standing to make unilaterally. **#4** (`activeWorkSessionCount`
  being a load-time snapshot, not re-derived at submit time, so it can drift
  stale in either direction) is left as-is because `stopLiveWorkAndReviewSessions`
  itself always re-queries live DB state at submit time regardless of what the
  notice displayed — the drift only affects the *notice's* accuracy, not
  correctness of the teardown. **#5** (retrying after a partial failure can
  re-apply `rejectPlan` or hit `triggerTriage`'s in-flight/status guards) is
  left as-is because Story 2.2.1's re-fetch-before-error-toast behavior
  (Task 2.2.1a) already surfaces the item's true post-failure state before any
  retry, which bounds the blast radius to a clear, actionable error rather than
  silent corruption. All three remain P2/non-blocking; none gets a new task in
  this plan.
- **BUG-078 cross-reference** (`docs/bugs/open/BUG-078-triage-in-flight-guard-races-item-ready-status-transition.md`):
  checked and it does not apply to this plan's call pattern. BUG-078's race is
  between a *prior* `TriggerTriage` call's own cleanup goroutine (status→`ready`,
  then `UpdateItemSessionEnded`, then `triageInFlight.Delete`, in that order) and
  a *new* `TriggerTriage` call for the same item arriving in the gap between
  those last two steps. This plan's `handleSendBackWithFeedback` sequence
  reaches `ready` via `TransitionBacklogItemStatus` — a different code path that
  never touches `triageInFlight` — not via a prior triage's cleanup, and its one
  `triggerTriage` call fires from an item at `in_progress`/`review`/`pr_pending`/
  `done`, statuses under which `triageInFlight` for that item should already be
  clear (none of those are a mid-triage state). So there's no concurrent/prior
  triage whose cleanup this call could race against under normal operation —
  adjacent risk, not applicable here.

## Dependency Visualization

**Sizing note**: most tasks below run 2-5 min, but 5 of the 14 (Task 1.2.1c,
2.1.1b, 2.1.1c, 2.2.1a, 2.2.1d) run 6-10 min — these are the tasks with the
largest inline code samples or table-driven test lists. That's intentional: the
level of detail already specified in-task (full code, exact assertions) makes
those tasks slower to type/verify, not a sizing miss. Task 2.2.1d is new as of
the iteration 3 plan-repair pass (CONCERN fix: an integration-level test for
`actionError`'s survival across `load()`'s async re-render, using real Promise
timing rather than pre-set static state).

```
Phase 1 (backend, independent of Phase 2)      Phase 2 (frontend, independent of Phase 1)
┌─────────────────────────────────┐            ┌──────────────────────────────────┐
│ Epic 1.1                         │            │ Epic 2.1                          │
│ Extract stopLiveWorkAndReviewSessions     │            │ New SendBackFeedbackBox component │
│ (Task 1.1.1a)                    │            │ (Tasks 2.1.1a → 2.1.1b → 2.1.1c)  │
└────────────────┬──────────────────┘            └─────────────────┬──────────────────┘
                 │                                                 │
                 v                                                 v
┌─────────────────────────────────┐            ┌──────────────────────────────────┐
│ Epic 1.2                         │            │ Epic 2.2                          │
│ Wire teardown into                │            │ Wire combined handler,            │
│ TransitionBacklogItemStatus       │            │ remove dead send_back_refining    │
│ (Tasks 1.2.1a → 1.2.1b → 1.2.1c) │            │ (Tasks 2.2.1a → 2.2.1b → 2.2.1c)  │
└────────────────┬──────────────────┘            └─────────────────┬──────────────────┘
                 │                                                 │
                 └───────────────────┬─────────────────────────────┘
                                     v
                    ┌──────────────────────────────────────┐
                    │ Phase 3: e2e coverage + feature registry │
                    │ Epic 3.1 (Tasks 3.1.1a → 3.1.1b)         │
                    │ Epic 3.2 (Task 3.2.1a)                   │
                    └──────────────────────────────────────┘
```

Phase 1 and Phase 2 have no code dependency on each other and can be implemented in
either order or in parallel — Phase 2's `handleSendBackWithFeedback` calls the
*existing* `transitionStatus` RPC signature regardless of whether Phase 1's teardown
block has landed yet; Phase 1 just makes that same call also stop the live session.
Phase 3 depends on both being complete (the e2e spec exercises the full stack).

---

## Phase 1: Backend — Live Session Teardown on Send-Back

### Epic 1.1: Extract shared session-teardown helper

**Goal**: Make `forceResetItem`'s "stop every live work/review session" logic callable
from a second site without duplicating it.

#### Story 1.1.1: Extract `stopLiveWorkAndReviewSessions`
**As a** backend maintainer, **I want** `forceResetItem`'s live-session teardown loop
extracted into a standalone method, **so that** `TransitionBacklogItemStatus` (Epic
1.2) can reuse it instead of duplicating the loop.

**Acceptance Criteria**:
- `forceResetItem`'s behavior is unchanged after the extraction.
  - *Given* a `BacklogItemData` at status `review` with one unended work-role
    `ItemSession` (`SessionUUID: "work-1"`), *When* `forceResetItem` is called (as it
    already is today from `SpawnSessionFromItem`'s `Force=true` path), *Then*
    `sessionStopper.StopSessionByUUID(ctx, "work-1")` is still called and the item's
    status still transitions to `in_progress`, exactly as before the extraction.
- The new helper is independently callable with just an item ID.
  - *Given* an item ID `"item-42"` with two unended sessions (`"work-1"` role
    `work`, `"review-1"` role `review`) and one already-ended session
    (`"work-0"`), *When* `stopLiveWorkAndReviewSessions(ctx, "item-42")` is called directly,
    *Then* `StopSessionByUUID` is called for `"work-1"` and `"review-1"` only (not
    `"work-0"`), and both their `ItemSession.EndedAt` fields become non-nil.

**Files**: `server/services/backlog_service_triage.go`

##### Task 1.1.1a: Extract the loop (~4 min)
- In `server/services/backlog_service_triage.go`, add a new method directly above
  `forceResetItem` (currently at line 1122):
  ```go
  // stopLiveWorkAndReviewSessions stops every unended work- or review-role ItemSession for
  // itemID (tmux teardown via sessionStopper, then marks the row ended) without
  // touching the item's own status. Extracted from forceResetItem so
  // TransitionBacklogItemStatus's backward-to-ready path (Epic 1.2) can reuse the
  // same teardown instead of a second, independently-maintained copy of this loop.
  // Best-effort: failures are logged, not returned — a leftover live session is a
  // known, already-recoverable condition (hasActiveWorkSession blocks the next
  // spawn until it's cleared), not a reason to fail the caller's transition.
  func (s *BacklogService) stopLiveWorkAndReviewSessions(ctx context.Context, itemID string) {
      sessions, err := s.storage.ListItemSessions(ctx, itemID)
      if err != nil {
          log.WarningLog().Printf("[stopLiveWorkAndReviewSessions] failed to list sessions for item %s: %v", itemID, err)
          return
      }
      for _, ps := range sessions {
          if ps.EndedAt != nil {
              continue
          }
          if ps.Role != string(session.SessionRoleWork) && ps.Role != string(session.SessionRoleReview) {
              continue
          }
          if s.sessionStopper != nil {
              if stopErr := s.sessionStopper.StopSessionByUUID(ctx, ps.SessionUUID); stopErr != nil {
                  log.WarningLog().Printf("[stopLiveWorkAndReviewSessions] failed to stop session %s for item %s: %v", ps.SessionUUID, itemID, stopErr)
              }
          }
          if endErr := s.storage.UpdateItemSessionEnded(ctx, ps.ID, time.Now()); endErr != nil {
              log.WarningLog().Printf("[stopLiveWorkAndReviewSessions] failed to mark session %s ended for item %s: %v", ps.SessionUUID, itemID, endErr)
          }
      }
  }
  ```
  (`log` is already imported in this file — `"github.com/tstapler/stapler-squad/log"`.)
- Replace `forceResetItem`'s existing inline loop (lines 1123-1135) with a single
  call: `s.stopLiveWorkAndReviewSessions(ctx, item.ID)`.
- Run `go build ./...` to confirm it compiles, then `go test ./server/services/... -run TestSpawnSessionFromItem -timeout=5m` to confirm `forceResetItem`'s existing tests still pass.
- Files: `server/services/backlog_service_triage.go`

---

### Epic 1.2: Teardown fires on backward-to-`ready` transition from a live status

**Goal**: Close the gap pitfalls.md §1 identified — nothing today stops a live
work/review session when an item is sent backward to `ready`.

#### Story 1.2.1: Stop live sessions on `in_progress`/`review`/`pr_pending` → `ready`
**As an** operator, **when I** send an item back to `ready` from `in_progress`,
`review`, or `pr_pending`, **I want** its live work/review tmux session stopped
automatically, **so that** it doesn't keep running against a now-superseded plan and
silently block my next session spawn (`hasActiveWorkSession`, pitfalls.md §1).

**Acceptance Criteria**:
- A backward transition from each of the three live statuses to `ready` stops the
  item's live session.
  - *Given* a `BacklogItem` at status `review` (and, in the same table-driven test,
    `in_progress` and `pr_pending`) with one unended work-role `ItemSession`
    (`SessionUUID: "work-live-1"`), *When* `TransitionBacklogItemStatus` is called
    with `TargetStatus: "ready"` and `OverrideReason` set to a feedback string,
    *Then* `stopper.stoppedUUIDs` contains `"work-live-1"`, that `ItemSession`'s
    `EndedAt` is non-nil, and the item's status is `ready` after the call returns.
- The teardown fires correctly even when the real `ErrVerdictClearRequiredForReady`
  guard is in play, not just in a case engineered to avoid it.
  - *Given* a `BacklogItem` at status `review` with a recorded PASS review verdict
    (`CreateItemSessionWithVerdict` with `ReviewVerdictData{OverallOutcome:
    session.ReviewVerdictPass}`) — the exact precondition that makes
    `ErrVerdictClearRequiredForReady` fire per `session/domain/backlog.go:637-657`
    — and one unended work-role `ItemSession`, *When* `TransitionBacklogItemStatus`
    is called with `TargetStatus: "ready"` and `OverrideReason` set to the send-back
    feedback text, *Then* the call succeeds (the override reason satisfies the
    guard) **and** the live session is still torn down (`stopper.stoppedUUIDs`
    contains its UUID) — proving the guard-bypass path and the teardown path
    compose correctly, which is the actual shape `handleSendBackWithFeedback`
    produces in production (feedback text doubles as `overrideReason`).
  - The complementary "fails without an override reason" half of this guard is
    already covered by the pre-existing
    `TestTransitionBacklogItemStatus_should_ReturnFailedPrecondition_When_ReviewToReadyBlockedByPassVerdict`
    (`backlog_service_lifecycle_test.go:449`) and the pre-existing
    `TestTransitionBacklogItemStatus_should_Allow_When_ReviewToReadyHasOverrideReason`
    (`:489`) already covers the bare succeed-with-override case at the RPC layer —
    this story's new test's job is only to additionally prove the teardown
    composes with that guard, not to re-prove the guard itself.
- A forward or unrelated transition never triggers the teardown.
  - *Given* a `BacklogItem` at status `ready` with one unended work-role
    `ItemSession` (`SessionUUID: "work-forward-1"`), *When*
    `TransitionBacklogItemStatus` is called with `TargetStatus: "in_progress"`,
    *Then* `stopper.stoppedUUIDs` does **not** contain `"work-forward-1"` (this is
    the identical case `TestTransitionBacklogItemStatus_should_NotArchiveWorkSessions_When_TransitionIsNotTerminal`
    already exercises for terminal-cleanup, mirrored here for the new teardown
    block).
- **(Pre-mortem P1 #2 — previously an untested CONCERN, now closed.)** A teardown
  failure (`sessionStopper.StopSessionByUUID` erroring) never blocks the
  transition, and the session row is still marked ended so it doesn't stay stuck
  "live" in the DB.
  - *Given* a `BacklogItem` at status `review` with one unended work-role
    `ItemSession` (`SessionUUID: "work-live-1"`) and a `mockSessionStopper` whose
    `stopperErr` is set to a non-nil error, *When* `TransitionBacklogItemStatus`
    is called with `TargetStatus: "ready"` and a non-empty `OverrideReason`,
    *Then* the call still succeeds and the item's status is `ready`
    (best-effort: see the Observability Plan's explicit justification for why a
    silent, logged-only continue is the deliberate choice here, not an
    oversight), `stopper.stoppedUUIDs` still contains `"work-live-1"` (the stop
    was attempted), and the `ItemSession`'s `EndedAt` is still marked non-nil.

**Files**: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_test.go`, `server/services/backlog_service_lifecycle_test.go`

##### Task 1.2.1a: Add the teardown block (~4 min)
- In `server/services/backlog_service_lifecycle.go`'s `TransitionBacklogItemStatus`,
  immediately after the existing idea/refining reset block (ends at line 827, before
  the final `return connect.NewResponse(...)` at line 829), add:
  ```go
  // Backward from a live status to ready: stop any live work/review session so
  // it doesn't keep running against a now-superseded plan (mirrors
  // forceResetItem's teardown for "Restart Session" — see pitfalls.md §1: the
  // 2026-07-29 OOM leak shape this closes, and hasActiveWorkSession's later
  // spawn-block this prevents).
  if to == session.BacklogStatusReady &&
      (from == session.BacklogStatusInProgress || from == session.BacklogStatusReview || from == session.BacklogStatusPRPending) {
      s.stopLiveWorkAndReviewSessions(ctx, req.Msg.ItemId)
  }
  ```
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 1.2.1b: Add `stoppedUUIDs` tracking and error injection to the test mock (~5 min)
- In `server/services/backlog_service_test.go`, add a `stoppedUUIDs []string` field
  and a `stopperErr error` field (pre-mortem P1 #2 — lets a test simulate
  `StopSessionByUUID` failing, which today's mock can never do since it always
  returns nil) to the `mockSessionStopper` struct (next to `archivedUUIDs`,
  ~line 203).
- Update `StopSessionByUUID` (line 250) from `func (m *mockSessionStopper) StopSessionByUUID(_ context.Context, _ string) error { return nil }`
  to append the UUID and return the injected error:
  ```go
  func (m *mockSessionStopper) StopSessionByUUID(_ context.Context, uuid string) error {
      m.stoppedUUIDs = append(m.stoppedUUIDs, uuid)
      return m.stopperErr
  }
  ```
  (`stopperErr` defaults to `nil` — every existing test that doesn't set it keeps
  today's always-succeeds behavior unchanged.)
- Files: `server/services/backlog_service_test.go`

##### Task 1.2.1c: Add the Go tests (~10 min)
- In `server/services/backlog_service_lifecycle_test.go`, add three tests near
  `TestTransitionBacklogItemStatus_should_NotArchiveWorkSessions_When_TransitionIsNotTerminal`
  (line 671), following that test's exact `createTestStorage`/`svc.SetSessionStopper`/
  `storage.CreateItemSession` setup shape:
  - `TestTransitionBacklogItemStatus_should_StopLiveWorkSession_When_SentBackToReady`
    (table-driven over all three live source statuses, so a regression that narrows
    the `if` condition in Task 1.2.1a — e.g. to just `review` — is caught): for each
    `from` in `[]session.BacklogStatus{session.BacklogStatusInProgress,
    session.BacklogStatusReview, session.BacklogStatusPRPending}`, in a `t.Run(string(from), ...)`
    subtest, create an item at that status, create an
    `ItemSessionData{SessionRole: session.SessionRoleWork, SessionUUID: "work-live-1"}`,
    call `TransitionBacklogItemStatus` with `TargetStatus: "ready"` and
    `OverrideReason: "missed the mobile layout, redo with touch targets"` (a
    realistic feedback string, not a placeholder — this doubles as proof the same
    string that satisfies the verdict guard is exactly what
    `handleSendBackWithFeedback` passes), then assert the call succeeds,
    `stopper.stoppedUUIDs` contains `"work-live-1"`, and the item's status is now
    `ready`.
  - `TestTransitionBacklogItemStatus_should_StopLiveWorkSession_When_PassVerdictPresentAndOverrideReasonSet`:
    create an item at `session.BacklogStatusReview`, call
    `storage.CreateItemSessionWithVerdict` with
    `session.ItemSessionData{SessionRole: session.SessionRoleReview, SessionUUID:
    "review-session-pass-verdict"}` and `session.ReviewVerdictData{OverallOutcome:
    session.ReviewVerdictPass}` (the exact precondition that makes
    `ErrVerdictClearRequiredForReady` fire, per `session/domain/backlog.go:637-657`
    — do **not** omit this setup, it's the scenario the plan actually needs proven),
    also create an `ItemSessionData{SessionRole: session.SessionRoleWork,
    SessionUUID: "work-live-1"}`, call `TransitionBacklogItemStatus` with
    `TargetStatus: "ready"` and `OverrideReason: "missed the mobile layout, redo
    with touch targets"`, then `require.NoError(t, err, "override_reason must let a
    PASS-verdict item proceed to ready")` and `assert.Contains(t,
    stopper.stoppedUUIDs, "work-live-1")` — proving the guard-bypass and the
    teardown compose. (The complementary "fails without an override reason" case is
    already covered by the pre-existing
    `TestTransitionBacklogItemStatus_should_ReturnFailedPrecondition_When_ReviewToReadyBlockedByPassVerdict`,
    `backlog_service_lifecycle_test.go:449` — no need to duplicate it here.)
  - `TestTransitionBacklogItemStatus_should_NotStopLiveWorkSession_When_TransitionIsNotBackwardToReady`:
    create item at `session.BacklogStatusReady`, create an `ItemSessionData{SessionRole: session.SessionRoleWork, SessionUUID: "work-forward-1"}`,
    transition `TargetStatus: "in_progress"`, then `assert.NotContains(t, stopper.stoppedUUIDs, "work-forward-1")`.
  - `TestTransitionBacklogItemStatus_should_StillTransition_When_LiveSessionTeardownFails`
    (pre-mortem P1 #2 — the previously-open CONCERN that teardown-failure had zero
    test coverage; `stopperErr` from Task 1.2.1b makes this possible for the first
    time): create an item at `session.BacklogStatusReview` with an unended
    `ItemSessionData{SessionRole: session.SessionRoleWork, SessionUUID:
    "work-live-1"}`, set `stopper.stopperErr = errors.New("tmux teardown failed")`,
    call `TransitionBacklogItemStatus` with `TargetStatus: "ready"` and
    `OverrideReason: "missed the mobile layout, redo with touch targets"`, then
    assert: (1) `require.NoError(t, err)` — the transition itself still succeeds
    despite the teardown error (best-effort semantics, see the Observability Plan's
    explicit justification below — blocking the send-back on a teardown failure
    would be strictly worse for the operator than a best-effort continue); (2) the
    item's status is `ready`; (3) `stopper.stoppedUUIDs` still contains
    `"work-live-1"` (the stop was attempted); (4) the `ItemSession`'s `EndedAt` is
    still non-nil (`stopLiveWorkAndReviewSessions`, Task 1.1.1a, marks the row ended
    unconditionally after attempting the stop, regardless of whether the stop
    itself errored — so the row doesn't stay stuck "live" in the DB even though the
    tmux pane may not have actually been killed).
- Run `go test ./server/services/... -run TestTransitionBacklogItemStatus -timeout=5m` and confirm all pass.
- Files: `server/services/backlog_service_lifecycle_test.go`

---

## Phase 2: Frontend — Combined Send-Back-With-Feedback Action

### Epic 2.1: New `SendBackFeedbackBox` component

**Goal**: A self-contained, `PlanVerdictBox`-shaped toggle/form component for the
`in_progress`/`review`/`pr_pending`/`done` send-back affordance (build-vs-buy.md §4).

#### Story 2.1.1: Inline toggle + feedback form
**As an** operator viewing an item at `in_progress`/`review`/`pr_pending`/`done`,
**I want** an inline "Send back for re-planning" toggle that reveals a required
feedback textarea, **so that** I can describe what should change before submitting.

**Acceptance Criteria**:
- The toggle reveals a validated, accessible form.
  - *Given* `visible={true}` (the item's `status` is `review`, so
    `actions.has("send_back_ready")` is true), *When* the operator clicks the
    toggle button (`data-testid="backlog-action-send-back-feedback"`), *Then* a
    `<div role="form" aria-label="Send back for re-planning">` appears containing a
    `<label htmlFor="send-back-feedback">What should change? (required)</label>` and
    a `rows={3}` `<textarea id="send-back-feedback" data-testid="send-back-feedback-textarea">`,
    and the Submit button (`data-testid="backlog-action-send-back-feedback-submit"`)
    is disabled.
- Submit enables only once text is typed.
  - *Given* the form is open, *When* the operator types `"missed the mobile layout,
    redo with touch targets"` into the textarea, *Then* the Submit button becomes
    enabled (`aria-disabled="false"`).
- Escape cancels and returns focus.
  - *Given* the form is open with typed text `"redo the auth approach"`, *When* the
    operator presses `Escape` inside the textarea, *Then* the form closes, the
    textarea's value resets to empty, and focus moves to the toggle button.
- An active session gets a non-blocking notice, not a blocking confirmation —
  and its copy accurately reflects that submitting stops the session (Epic 1.2's
  `stopLiveWorkAndReviewSessions` runs synchronously inside the same `transitionStatus`
  call this submit makes; the notice must not claim otherwise).
  - *Given* `activeWorkSessionCount={1}`, *When* the form is open, *Then* an
    `InlineNotice` reading `"This item has an active session — submitting this
    will stop it, and its work may be superseded once re-planning completes."` is
    rendered above the textarea, and the Submit button is not disabled by its
    presence.
- A failed submit preserves the typed text.
  - *Given* the operator typed `"redo the auth approach"` and clicked Submit, *When*
    the `onSubmit` promise rejects, *Then* the form stays open, the textarea still
    contains `"redo the auth approach"`, and an `InlineError` with headline
    `"Failed to send back"` is shown (dismiss-only, no `onRetry` — mirrors
    `PlanVerdictBox`'s no-misleading-retry discipline, ux.md §4).
- Cancel and Escape are inert while a submit is in flight (BLOCKER — visibility
  of system status / user control and freedom: without this, clicking Cancel or
  pressing Escape mid-submit visually collapses the form as if the action were
  aborted while the in-flight `transitionStatus`→`rejectPlan`→`triggerTriage`
  chain keeps running unseen, silently dropping its eventual success toast or
  `InlineError`).
  - *Given* the form is open, submit text typed, and `isPending` is `true`
    (Submit already shows `"Sending…"` and is disabled), *When* the operator
    clicks the Cancel button, *Then* nothing happens — the form stays open, the
    typed text is unchanged, and focus does not move.
  - *Given* the same in-flight state, *When* the operator presses `Escape`
    inside the textarea, *Then* nothing happens, for the identical reason —
    `handleCancel` (which both Cancel's `onClick` and the Escape key handler
    call) early-returns while `isPending` is `true`. The Cancel button is also
    rendered `disabled`/`aria-disabled={isPending}`, matching Submit's existing
    gating.
- The component survives losing eligibility (`visible` going `false`) as long
  as there's an unresolved error to show (BLOCKER found by a fresh UX
  triad-lens review, iteration 2 repair pass: `handleSendBackWithFeedback`'s
  catch block, Task 2.2.1a, calls `load()` before re-throwing, which can flip
  `visible` to `false` — the item's post-failure status is `ready` or `idea`,
  both outside `CAN_SEND_BACK_READY` — before the error copy below could ever
  render under the old `if (!visible) return null;` guard).
  - *Given* the component renders with `visible={false}` and `actionError`
    already set, *When* it re-renders, *Then* it still shows the error UI
    (headline, message, Dismiss) rather than returning `null`.
  - *Given* the same state, *When* the operator dismisses the error (the
    `InlineError`'s Dismiss button, `setActionError(null)`), *Then* the next
    render — `visible={false}`, `actionError === null` — returns `null`
    (clean unmount, not stuck rendering forever).
- **The idea-landing failure offers a real Retry, not a dead-end pointer**
  (BLOCKER found by a fresh UX triad-lens review, iteration 3 repair pass:
  the item genuinely lands at `idea` — not `ready` — when `triggerTriage`'s
  own internal CAS commits before a later step in that call fails; neither
  `PlanVerdictBox` card renders at `idea`, and the old copy's instruction to
  "re-run Send back for re-planning from there" pointed at a toggle that
  cannot render, since `SendBackFeedbackBox`'s own `visible` prop excludes
  `idea`).
  - *Given* `onSubmit` rejects with a tag indicating the item is now at
    `idea` (Task 2.1.1b's `SendBackError` with `itemStatusAfterFailure:
    "idea"`), *When* the error renders, *Then* it includes a Retry action
    (`InlineError`'s `onRetry`) alongside Dismiss, and the typed feedback
    text remains in the textarea, unchanged.
  - *Given* that state, *When* the operator clicks Retry, *Then* `onSubmit`
    is called again with the exact same feedback text — no re-typing
    required — and the prior error is cleared immediately, before the retry
    resolves.
  - **The component stays visible for the duration of the retry's own
    pending phase, not just before/after it** (BLOCKER found by a fresh UX
    triad-lens review, iteration 4 repair pass: clicking Retry sets
    `localPending` true and clears `actionError` to `null` in the same call,
    and `visible` is still `false` throughout the retry — the item's status
    doesn't leave `idea` until the retry's own chain resolves — so the
    plain `if (!visible && !actionError) return null;` guard evaluated
    `true` for the whole pending window and unmounted the form, Submit
    button, `"Sending…"` label, and `aria-busy` indicator, reappearing only
    once the promise settled).
    - *Given* the operator has just clicked Retry and the retried `onSubmit`
      promise has not yet resolved, *When* the component re-renders (with
      `visible={false}`, `actionError === null`, `localPending === true`),
      *Then* it still renders the form with the textarea's feedback text
      intact and the Submit button showing `"Sending…"` with
      `aria-busy="true"` and `disabled` — it does not return `null`. This is
      fixed by adding an `isPending` escape hatch to the guard:
      `if (!visible && !actionError && !isPending) return null;` (Task
      2.1.1b), reusing the `isPending` value already computed for Submit's
      own pending gating rather than introducing a second pending flag.
  - *Given* the retried `onSubmit` call succeeds, *When* it resolves, *Then*
    the form collapses and the textarea clears, identically to a first-try
    success (Surface 5) — this works because `idea→ready` is itself a valid
    backend transition (`session/domain/backlog.go`'s `validTransitions`),
    so replaying `transitionStatus`→`rejectPlan`→`triggerTriage` from `idea`
    is not materially different from the original attempt from
    `in_progress`/`review`/`pr_pending`/`done`.

**Files**: `web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx` (new),
`web-app/src/components/backlog/detail/SendBackFeedbackBox.css.ts` (new),
`web-app/src/components/backlog/detail/SendBackError.ts` (new)

##### Task 2.1.1a: Create the vanilla-extract styles (~5 min)
- Create `web-app/src/components/backlog/detail/SendBackFeedbackBox.css.ts`,
  forking only the classes this component needs from
  `web-app/src/components/backlog/PlanVerdictBox.css.ts` (no status-card classes are
  needed — this component has no persistent card, only the toggle/form):
  `section` (flex column, `gap: vars.space["2"]`), `toggleButton` (reuse
  `secondaryButton`'s exact style block), `submitButton` (reuse `primaryButton`'s
  exact style block), `form`, `formLabel`, `formTextarea`, `formActions` (copy these
  four verbatim from `PlanVerdictBox.css.ts:171-203`).
- Files: `web-app/src/components/backlog/detail/SendBackFeedbackBox.css.ts`

##### Task 2.1.1b: Create the component (~7 min)
- First, create `web-app/src/components/backlog/detail/SendBackError.ts` — a small
  shared module (not inlined into either consumer, since both
  `SendBackFeedbackBox.tsx` here and `BacklogItemDetail.tsx`'s
  `handleSendBackWithFeedback`, Task 2.2.1a, need the same class — resolves
  BLOCKER 5.2 and pre-mortem P1 #1):
  ```ts
  export type SendBackFailedAt = "transition" | "reject" | "triage";

  /**
   * Thrown by handleSendBackWithFeedback (Task 2.2.1a) so SendBackFeedbackBox
   * can render distinct recovery copy per which of the 3 chained calls failed
   * (ux.md Surface 7) instead of one generic message for every failure.
   * itemStatusAfterFailure is the item's real status re-fetched after the
   * failure — needed because triggerTriage's own internal ready→idea CAS
   * (backlog_service_trigger_triage.go:249-260) can commit before a LATER
   * step in that same call fails, leaving the item at "idea" rather than
   * "ready" (pre-mortem P1 #1).
   */
  export class SendBackError extends Error {
    constructor(
      public readonly failedAt: SendBackFailedAt,
      public readonly itemStatusAfterFailure: string,
      public readonly cause: unknown
    ) {
      super(cause instanceof Error ? cause.message : String(cause));
      this.name = "SendBackError";
    }
  }
  ```
- Then create `web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx`, forking
  `PlanVerdictBox.tsx`'s toggle/form structure (lines 80-127, 193-243):
  ```tsx
  "use client";
  // +feature: backlog:send-back-feedback

  import { useEffect, useRef, useState } from "react";
  import * as styles from "./SendBackFeedbackBox.css";
  import { InlineError } from "../InlineError";
  import { InlineNotice } from "@/components/common/InlineNotice";
  import { getErrorMessage } from "@/lib/utils/connectError"; // matches BacklogItemDetail.tsx:33's existing import
  import { SendBackError } from "./SendBackError"; // Task 2.2.1a — carries failedAt + itemStatusAfterFailure

  export interface SendBackFeedbackBoxProps {
    visible: boolean;
    activeWorkSessionCount: number;
    /** True while another action is in flight elsewhere in ActionsSection — disables the toggle. */
    disabled?: boolean;
    /** True while THIS box's own submit is in flight (drives aria-busy). */
    actionPending?: boolean;
    onSubmit: (feedback: string) => Promise<void>;
  }

  // Renders null only when the action is ineligible (visible=false) AND
  // there's no pending actionError AND no submit (incl. a Retry) is in
  // flight — so a partial-failure error set just before a parent re-render
  // (Task 2.2.1a's load()) still gets shown even after `visible` flips
  // false, and a Retry launched from a `visible=false`/no-error state stays
  // rendered for the duration of its own pending phase instead of blanking.
  // See the early-return guard below.
  export function SendBackFeedbackBox({
    visible,
    activeWorkSessionCount,
    disabled = false,
    actionPending = false,
    onSubmit,
  }: SendBackFeedbackBoxProps) {
    const [showForm, setShowForm] = useState(false);
    const [feedback, setFeedback] = useState("");
    const [localPending, setLocalPending] = useState(false);
    // headline/message pair, not a bare string: BLOCKER 5.2's fix needs distinct
    // copy for "transitionStatus itself failed" vs. "it succeeded but a later
    // call failed" (ux.md Surface 7), so a single string can no longer carry both
    // the headline and the body. `retryable` (iteration 3 repair pass, BLOCKER B)
    // is true only for the "landed at idea" case (Surface 7 row 4) — idea→ready
    // is itself a valid transition (session/domain/backlog.go's validTransitions),
    // so unlike the row 2/3 cases (where resubmitting would replay
    // transitionStatus against an already-"ready" item and fail), retrying from
    // "idea" is a genuine, correct recovery path, not a misleading affordance.
    const [actionError, setActionError] = useState<{ headline: string; message: string; retryable?: boolean } | null>(
      null
    );

    const isPending = localPending || actionPending;
    const canSubmit = feedback.trim().length > 0 && !isPending;

    const toggleRef = useRef<HTMLButtonElement>(null);
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
      if (showForm) textareaRef.current?.focus();
    }, [showForm]);

    // Stay mounted even when `visible` goes false, as long as there's an
    // unresolved actionError to show: handleSendBackWithFeedback's catch
    // block (Task 2.2.1a) calls load() before re-throwing, which re-fetches
    // the item and can flip `visible` to false (its new status — "ready" or
    // "idea" — is outside CAN_SEND_BACK_READY) before Surface 7's error copy
    // ever has a chance to render. Because actionError is local useState,
    // it survives that parent re-render as long as this component instance
    // stays mounted — which is exactly what this guard preserves (fixes a
    // BLOCKER a fresh UX triad-lens review found in the iteration 2
    // plan-repair pass; see ux.md Surface 7's mechanism note).
    //
    // Also stay mounted while `isPending` is true (iteration 4 repair pass,
    // BLOCKER): the row-4 Retry (handleRetry -> handleSubmit) sets
    // localPending true AND clears actionError to null in the same call,
    // before onSubmit resolves. At that instant `visible` is still false
    // (the item's status is still "idea", outside CAN_SEND_BACK_READY, until
    // the retry's own transitionStatus->rejectPlan->triggerTriage chain
    // settles) and actionError is now null too — so without the isPending
    // check, this guard would return null for the whole retry, unmounting
    // the toggle, form, and "Sending..."/aria-busy UI and re-mounting only
    // once the promise settles. isPending is already computed above (line
    // 601), so this adds no new state, and once pending finishes with the
    // error cleared and visible still false, the guard again correctly
    // returns null (no stuck-mounted-forever regression).
    if (!visible && !actionError && !isPending) return null;

    function handleCancel() {
      // Gated on isPending, matching Submit's existing gating: without this,
      // a click or Escape mid-submit visually aborts the form while the
      // in-flight transitionStatus->rejectPlan->triggerTriage chain keeps
      // running unseen, dropping its eventual success toast or error.
      if (isPending) return;
      setShowForm(false);
      setFeedback("");
      setActionError(null); // lets the component unmount on its next render once !visible (see the guard above)
      toggleRef.current?.focus();
    }

    function handleTextareaKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
      if (e.key === "Escape") handleCancel();
    }

    // BLOCKER B fix (iteration 3 repair pass): re-invokes the exact same
    // submit path — handleSubmit already reads the CURRENT `feedback` state
    // (untouched by this failure) and delegates to `onSubmit`
    // (handleSendBackWithFeedback in the parent), which itself reads the
    // parent's current `item.status`/`item.updatedAtRaw` — "idea" by this
    // point, not the originally-assumed source status — so no separate retry
    // codepath is needed; only a rendering hook to reach it. See Task 2.2.1a's
    // note on why the parent's closure is already current.
    function handleRetry() {
      void handleSubmit();
    }

    async function handleSubmit() {
      if (!canSubmit) return;
      setLocalPending(true);
      // Clear any stale error from a prior attempt before this one starts —
      // load-bearing for Retry (below), which calls handleSubmit directly:
      // without this, a successful retry would leave the old actionError
      // sitting in state (harmless today since showForm/visible both gate its
      // rendering, but not future-proof) and a *second* failed retry would
      // briefly show the previous attempt's copy before the new branch runs.
      setActionError(null);
      try {
        await onSubmit(feedback);
        setShowForm(false);
        setFeedback("");
      } catch (err) {
        // BLOCKER 5.2 fix: distinguish "transitionStatus itself failed" (nothing
        // changed server-side — ux.md Surface 6) from "it succeeded but a later
        // call failed" (ux.md Surface 7) using the SendBackError tag
        // handleSendBackWithFeedback (Task 2.2.1a) throws. A plain rejection with
        // no SendBackError wrapper (e.g. a unit test calling onSubmit directly)
        // falls back to the generic Surface 6 copy.
        if (err instanceof SendBackError && err.failedAt !== "transition") {
          // Pre-mortem P1 #1: triggerTriage's own internal ready→idea CAS
          // (backlog_service_trigger_triage.go:249-260) can commit before a
          // later step in that same call fails, so the item may now be at
          // "idea" rather than "ready" — in which case neither PlanVerdictBox
          // card (ready-only) is the right pointer.
          if (err.itemStatusAfterFailure === "idea") {
            // BLOCKER B fix (iteration 3 repair pass): the old copy here told
            // the operator to "close this form and re-run Send back for
            // re-planning from there" — but SendBackFeedbackBox's own
            // `visible` prop can never be true at "idea" (CAN_SEND_BACK_READY
            // excludes it), so that pointed at an affordance that cannot
            // render. It also claimed "resubmitting this box now will fail,"
            // which is wrong: idea→ready is itself a valid transition
            // (session/domain/backlog.go's validTransitions), so replaying
            // this exact chain from "idea" works. The feedback text is still
            // sitting in this component's own `feedback` state (never
            // cleared on this failure path) — offer a real Retry instead of
            // a dead pointer.
            setActionError({
              headline: "Sent back, but retriage didn't start",
              message:
                'The item moved back to "idea" before retriage could start. ' +
                "Your feedback is still in this box — click Retry to send it " +
                'back to "ready" and start retriage again.',
              retryable: true,
            });
          } else if (err.failedAt === "reject") {
            setActionError({
              headline: "Sent back, but retriage didn't start",
              message:
                'The item already moved to "ready." Close this form and use ' +
                'the "Request Changes" button below to record your feedback ' +
                "and retry — do not resubmit this box.",
            });
          } else {
            // BLOCKER A fix (iteration 3 repair pass): the old copy pointed at
            // the plain "Trigger Triage" button. That button
            // (ActionsSection.tsx) calls triggerTriage(item.id) with NO
            // feedback argument, and the backend only ever reads feedback from
            // the RPC request — never from the stored PlanRejectionReason
            // field rejectPlan (call 2) already persisted — so following the
            // old instruction silently discarded the operator's feedback. At
            // this point rejectPlan succeeded, so PlanVerdictBox is showing
            // its changes_requested card with a "Regenerate Plan with This
            // Feedback" button (PlanVerdictBox.tsx) that explicitly passes
            // item.planRejectionReason to triggerTriage — that's the correct,
            // feedback-preserving affordance to name here.
            setActionError({
              headline: "Sent back, but retriage didn't start",
              message:
                'The item already moved to "ready" and your feedback was ' +
                'recorded. Close this form and use the "Regenerate Plan ' +
                'with This Feedback" button below to retry — do not ' +
                "resubmit this box.",
            });
          }
        } else {
          setActionError({
            headline: "Failed to send back",
            message: getErrorMessage(err, "Failed to send back."),
          });
        }
        console.error(err);
      } finally {
        setLocalPending(false);
      }
    }

    return (
      <div className={styles.section}>
        {visible && (
          <button
            ref={toggleRef}
            className={styles.toggleButton}
            aria-expanded={showForm}
            disabled={disabled}
            onClick={() => setShowForm((prev) => !prev)}
            data-testid="backlog-action-send-back-feedback"
          >
            ↩ Send back for re-planning
          </button>
        )}

        {showForm && (
          <div role="form" aria-label="Send back for re-planning" className={styles.form}>
            {activeWorkSessionCount > 0 && (
              <InlineNotice
                message="This item has an active session — submitting this will stop it, and its work may be superseded once re-planning completes."
                data-testid="send-back-active-session-notice"
              />
            )}
            <label htmlFor="send-back-feedback" className={styles.formLabel}>
              What should change? (required)
            </label>
            <textarea
              id="send-back-feedback"
              ref={textareaRef}
              data-testid="send-back-feedback-textarea"
              rows={3}
              placeholder="e.g. missed the mobile case, re-check the auth approach"
              value={feedback}
              onChange={(e) => setFeedback(e.target.value)}
              onKeyDown={handleTextareaKeyDown}
              className={styles.formTextarea}
            />
            {actionError && (
              <InlineError
                type="transient"
                headline={actionError.headline}
                onDismiss={() => setActionError(null)}
                customMessage={actionError.message}
                // BLOCKER B fix: reuses InlineError's existing onRetry
                // affordance (already built for exactly this "retry is real,
                // not misleading" case, per its own doc comment) — wired only
                // for the retryable=true (idea-landing) case. The row 2/3
                // branches above never set retryable, so they keep getting no
                // onRetry, preserving ux.md's existing "no misleading retry"
                // discipline for those.
                onRetry={actionError.retryable ? handleRetry : undefined}
                retryAriaLabel="Retry send-back with this feedback"
              />
            )}
            <div className={styles.formActions}>
              <button
                className={styles.toggleButton}
                onClick={handleCancel}
                disabled={isPending}
                aria-disabled={isPending}
              >
                Cancel
              </button>
              <button
                className={styles.submitButton}
                aria-disabled={!canSubmit}
                disabled={!canSubmit}
                aria-busy={isPending}
                onClick={() => void handleSubmit()}
                data-testid="backlog-action-send-back-feedback-submit"
              >
                {isPending ? "Sending…" : "Submit"}
              </button>
            </div>
          </div>
        )}
      </div>
    );
  }
  ```
- Files: `web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx`,
  `web-app/src/components/backlog/detail/SendBackError.ts`

##### Task 2.1.1c: Unit tests (~8 min)
- Create `web-app/src/components/backlog/detail/SendBackFeedbackBox.test.tsx`,
  mirroring `PlanVerdictBox.test.tsx`'s structure: test toggle opens/closes the form,
  Submit is disabled until text is typed, Escape cancels and returns focus to the
  toggle, a resolved `onSubmit` clears the form, and the `InlineNotice` renders only
  when `activeWorkSessionCount > 0`. Also cover the distinct error paths BLOCKER 5.2
  and pre-mortem P1 #1 added:
  - a plain rejected `onSubmit` (no `SendBackError` wrapper) preserves the typed
    text and shows headline `"Failed to send back"`;
  - `onSubmit` rejecting with `new SendBackError("transition", "review", err)`
    shows the same generic `"Failed to send back"` headline (transition itself
    failed — Surface 6);
  - `onSubmit` rejecting with `new SendBackError("triage", "ready", err)` shows
    headline `"Sent back, but retriage didn't start"` and body text mentioning
    `"Regenerate Plan with This Feedback"`, not `"Trigger Triage"` (Surface 7,
    table row 3 — BLOCKER A fix, iteration 3 repair pass: the plain "Trigger
    Triage" button takes no feedback argument and would silently discard the
    operator's already-recorded feedback);
  - `onSubmit` rejecting with `new SendBackError("reject", "ready", err)` shows
    the same headline but body text mentioning `"Request Changes"` instead
    (Surface 7, table row 2);
  - `onSubmit` rejecting with `new SendBackError("triage", "idea", err)` shows
    the same headline but body text mentioning `"idea"` and no longer claims
    resubmitting will fail (Surface 7, table row 4 — pre-mortem P1 #1), **and**
    renders a Retry button (`InlineError`'s `onRetry`, `aria-label="Retry
    send-back with this feedback"`) — BLOCKER B fix, iteration 3 repair pass.
  - **Retry behavior (BLOCKER B fix):** with the row-4 error showing and the
    textarea still containing the originally-typed feedback, clicking Retry
    (a) calls `onSubmit` a second time with that exact same feedback string
    (proving the text was never cleared on this failure path), (b) clears the
    prior `actionError` immediately (no stale copy visible during the retry's
    own pending state), and (c) on that second call resolving, behaves
    identically to a fresh successful submit — form collapses, textarea
    clears, no error is shown. Also cover: clicking Retry while the textarea
    has been manually cleared first is a no-op (mirrors `canSubmit`'s existing
    empty-text gating — `handleSubmit`, which `handleRetry` delegates to,
    already early-returns on `!canSubmit`).
  - **Backend coverage note (no new backend test needed):** the retry's
    `transitionStatus(item.id, "ready", { expectedStatus: "idea", ... })` call
    is a plain `idea→ready` transition, already exercised end-to-end (with a
    populated `AcCriteria`, the same precondition `TransitionGuard` enforces
    for this edge per `session/domain/backlog.go:617-621`) by several existing
    tests, e.g. `server/services/backlog_service_test.go`'s "idea → ready →
    in_progress → review → done" walk (line ~816) and its SDD-mode triage
    persistence test (line ~2018). This story's own frontend test above is
    what's new — it proves `SendBackFeedbackBox` calls that same, already-
    trusted RPC path correctly on Retry, not that the RPC path itself works.
  - BLOCKER fix: with a pending `onSubmit` promise (one that hasn't resolved
    yet, so `isPending` is `true` and Submit shows `"Sending…"`), clicking
    Cancel is a no-op (form stays open, textarea value unchanged, focus
    unmoved), and pressing `Escape` in the textarea is equally a no-op, for the
    same reason.
  - **Mount-persistence BLOCKER fix (iteration 2 repair pass):** rendering
    with `visible={false}` and an `actionError` already set (simulating the
    post-`load()` re-render after a partial-failure catch, Task 2.2.1a)
    still shows the error UI (headline, message, Dismiss) — the component
    does not return `null`. Dismissing that error (clicking the
    `InlineError`'s Dismiss, which calls `setActionError(null)`) then makes
    the next render — still `visible={false}`, now with `actionError ===
    null` — return `null` (unmounts cleanly). Also cover: `visible={false}`
    with no `actionError` renders `null` immediately (today's behavior,
    unchanged).
  - **Retry-pending mount-persistence BLOCKER fix (iteration 4 repair
    pass):** starting from `visible={false}` with the row-4 `idea`-landing
    `actionError` already showing (its Retry button rendered), clicking
    Retry and — while its `onSubmit` promise is still unresolved — asserting
    on every intermediate render that the component is **never** absent (no
    frame with neither toggle, form, nor error/pending UI present): the
    form stays rendered, the Submit button reads `"Sending…"` with
    `aria-busy="true"`, and the textarea still shows the original feedback
    text, for the whole pending window — not just before Retry is clicked or
    after the promise settles. This is the case where `handleSubmit` has
    already called `setLocalPending(true)` and `setActionError(null)`, so
    `visible` is still `false` and `actionError` is `null` at the same time
    — the exact combination the plain `if (!visible && !actionError) return
    null;` guard mishandled by unmounting the component mid-retry (a fresh
    UX triad-lens review, code-verified). Then, once the promise resolves
    (cover both outcomes): on success, the form collapses and the textarea
    clears, identically to Task 2.1.1b's existing "retried `onSubmit` call
    succeeds" AC; on a second rejection, the appropriate `actionError` is
    shown again and the component remains mounted (covered by the
    mount-persistence test above).
- Run `cd web-app && npx jest SendBackFeedbackBox --no-coverage` and confirm all pass.
- Files: `web-app/src/components/backlog/detail/SendBackFeedbackBox.test.tsx`

---

### Epic 2.2: Wire the combined handler; remove dead `send_back_refining` code

**Goal**: One submit sequences `transitionStatus` → `rejectPlan` → `triggerTriage`,
and the dead `send_back_refining` code path is removed (requirements.md's success
metric #2).

#### Story 2.2.1: Combined submit handler
**As an** operator, **when I** submit feedback via the new box, **I want** the item
moved to `ready`, my feedback recorded, and retriage started — in one click.

**Acceptance Criteria**:
- A successful submit chains all three calls in order with the right arguments.
  - *Given* item `"item-42"` at status `"review"` with `updatedAtRaw` `T1`, *When*
    the operator submits feedback `"missed the mobile layout, redo with touch
    targets"`, *Then* `transitionStatus("item-42", "ready", { expectedStatus:
    "review", expectedUpdatedAt: T1, overrideReason: "missed the mobile layout, redo
    with touch targets" })` is called, then `rejectPlan("item-42", "missed the mobile
    layout, redo with touch targets")`, then `triggerTriage("item-42", "missed the
    mobile layout, redo with touch targets")`, and on success a toast reading
    `"Feedback sent — retriage started."` appears and `load()` re-fetches the item.
- A failure in the first call leaves nothing changed.
  - *Given* the same item, *When* `transitionStatus` rejects, *Then* `rejectPlan` and
    `triggerTriage` are never called, and `SendBackFeedbackBox`'s own error path
    (Story 2.1.1) shows the failure with the typed text intact.
- A failure after the transition always re-fetches before the error is shown,
  whichever of the two remaining calls failed, and the error carries which call
  failed plus the item's real post-failure status so the box can show distinct
  recovery copy (resolves BLOCKER 5.2 — the plan previously had no logic
  distinguishing these cases; ux.md Surface 7).
  - *Given* `transitionStatus` succeeds but `triggerTriage` (call 3) rejects
    *and* its internal `ready→idea` CAS did **not** get a chance to run first
    (i.e. the failure is a normal downstream `triggerTriage` error, not the
    P1 #1 case below), *When* the catch block runs, *Then* `load()` is called
    before the error toast is shown, the thrown error is a `SendBackError`
    with `failedAt: "triage"` and `itemStatusAfterFailure: "ready"`, the item
    is now at `status: "ready"` with `planRejectionReason` set to the typed
    feedback (from the successful `rejectPlan` call), `PlanVerdictBox` renders
    its `changes_requested` "Revisions requested" card with the "Regenerate
    Plan with This Feedback" button as the recoverable retry path, and
    `SendBackFeedbackBox` shows headline **"Sent back, but retriage didn't
    start"** with body text pointing at that same "Regenerate Plan with This
    Feedback" button — **not** "Trigger Triage" (BLOCKER A, iteration 3 repair
    pass: the plain "Trigger Triage" button in `ActionsSection.tsx` calls
    `triggerTriage(item.id)` with no feedback argument, and the backend only
    ever reads feedback from the RPC request, never from the stored
    `PlanRejectionReason` field — pointing there would silently discard the
    operator's already-recorded feedback; ux.md Surface 7, table row 3) — no
    new bespoke retry UI is needed beyond that copy correction.
  - *Given* `transitionStatus` succeeds but `rejectPlan` (call 2) rejects — so
    `triggerTriage` (call 3) never runs and `planRejectionReason` is never set —
    *When* the catch block runs, *Then* `load()` is still called before the error
    toast is shown, the thrown error is a `SendBackError` with `failedAt:
    "reject"` and `itemStatusAfterFailure: "ready"`, the item detail view
    reflects the true server state (`status: "ready"`, no rejection reason
    yet) instead of the stale pre-submit `item.status`, and
    `SendBackFeedbackBox` shows the same "Sent back, but retriage didn't
    start" headline but with body text pointing at "Request Changes" instead
    (ux.md Surface 7, table row 2). This also means a subsequent retry's
    `transitionStatus` call reads the freshly-loaded `item.status`/`item.updatedAtRaw`
    for its `expectedStatus`/`expectedUpdatedAt` CAS precondition, rather than
    replaying the original (now-stale) values from before the first submit —
    which would otherwise fail a second time with an unrelated
    `ErrPreconditionFailed`/`CodeAborted` against the item's real,
    already-advanced state.
  - **Pre-mortem P1 #1** — *Given* `transitionStatus` succeeds, `rejectPlan`
    succeeds, and `triggerTriage` (call 3) rejects *after* its own internal
    `ready→idea` CAS already committed (`backlog_service_trigger_triage.go`'s
    step 3b, lines 249-260 — this fires before the later artifact-dir-creation
    (step 5) and headless-pool-nil (step 6) checks that can still fail, so a
    failure at either of those lands here, not in the row above), *When* the
    catch block runs, *Then* the re-fetch (via a direct `getBacklogItem(item.id)`
    call, not just `load()`'s side effect, so the fresh status is available
    synchronously to the catch block) observes `status: "idea"`, the thrown
    error is a `SendBackError` with `failedAt: "triage"` and
    `itemStatusAfterFailure: "idea"`, and `SendBackFeedbackBox` shows the same
    "Sent back, but retriage didn't start" headline, with body text naming
    the item's actual `"idea"` status and a working **Retry** action
    (`InlineError`'s `onRetry`) — **not** a pointer at `PlanVerdictBox`'s
    "Trigger Triage"/"Request Changes" affordances (neither card renders at
    `idea`) and **not** the old copy's claim that resubmitting will fail
    (BLOCKER B, iteration 3 repair pass: `idea→ready` is itself a valid
    transition per `session/domain/backlog.go`'s `validTransitions`, so
    retrying the same `transitionStatus`→`rejectPlan`→`triggerTriage` chain
    from `idea` works; ux.md Surface 7, table row 4). Clicking Retry re-calls
    `onSubmit` with the same feedback text still present in the textarea
    (Task 2.1.1b) — `handleSendBackWithFeedback` needs no new logic for this,
    since it already reads `item.status`/`item.updatedAtRaw` from its
    `useCallback` closure, which is regenerated (via the `[item, ...]` dep
    array) every time `item` changes, including the `load()` this same catch
    block just triggered — so by the time the operator clicks Retry, the
    closure already reflects `expectedStatus: "idea"`, not the original
    `in_progress`/`review`/`pr_pending`/`done` source status.
  - **Discovered constraint, documented per this repair pass's instructions
    (not a blocker to the fix):** `idea→ready` is structurally valid, but
    `TransitionGuard` (`session/domain/backlog.go:617-621`) additionally
    requires the item's `AcCriteria` to be non-empty for that specific edge,
    returning `ErrACRequired`/`FailedPrecondition` otherwise. This does not
    block the Retry fix: every item eligible for send-back-with-feedback in
    the first place (`in_progress`/`review`/`pr_pending`/`done`) can only
    have reached those statuses by first passing through `ready`, and the
    only two edges into `ready` from an idea-shaped item
    (`idea→ready`/`refining→ready`) already require non-empty `AcCriteria` —
    so by construction, an item that got far enough to trigger this failure
    mode already carries the `AcCriteria` the retry's `idea→ready` call
    needs. No code path in this feature clears `AcCriteria` after that point.
    If Retry ever does hit `ErrACRequired` in practice (e.g. AC criteria
    edited down to empty out-of-band between the original attempt and the
    retry), it surfaces as an ordinary `SendBackError("transition", "idea",
    e)` through the ordinary call-1-failure path (Surface 6) — no special
    copy is needed for this edge case, since Surface 6's generic "Failed to
    send back" framing (with the real server error text) is accurate there.
- The toast shown across all three partial-failure branches above (`reject`,
  `triage`-after-`ready`, and `triage`-after-`idea`) reads **"Send-back needs
  attention — see details below."**, not the generic
  `getErrorMessage(e, "Failed to send back.")` copy — the item *did* move
  server-side in every one of those branches, so framing the toast as total
  failure would contradict `SendBackFeedbackBox`'s own, more accurate in-form
  message for the same failure (ux.md Surface 7; CONCERN fix, iteration 2
  repair pass). Only the call-1-fails branch (nothing changed server-side)
  keeps the original `getErrorMessage(e, "Failed to send back.")` framing,
  since it's accurate there.
- The dead code path no longer exists.
  - *Given* `BacklogItemDetail.tsx`'s current build, *When* the codebase is grepped
    for `send_back_refining`, *Then* zero matches remain in
    `BacklogItemDetail.tsx` (no `case`, no `ACTION_SUCCESS_MESSAGES` entry).

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/detail/ActionsSection.tsx`,
`web-app/src/components/backlog/detail/SendBackError.ts`,
`web-app/src/components/backlog/BacklogItemDetail.sendBackFeedbackPersistence.test.tsx` (new, Task 2.2.1d — CONCERN fix)

##### Task 2.2.1a: Add `handleSendBackWithFeedback`, remove dead code (~9 min)
- In `web-app/src/components/backlog/BacklogItemDetail.tsx`:
  - Add `import { SendBackError } from "./detail/SendBackError";` (Task 2.1.1b).
  - Remove the `send_back_refining: "Sent back to refining."` entry from
    `ACTION_SUCCESS_MESSAGES` (line 92).
  - Remove `case "send_back_refining": await transitionStatus(item.id, "refining"); break;`
    (lines 793-795) from the `handleAction` switch.
  - Remove `case "send_back_ready": await transitionStatus(item.id, "ready"); break;`
    (lines 796-798) from the same switch — this action is now dispatched via the
    dedicated handler below, not the generic `onAction` string dispatch.
  - Add a new `useCallback`, placed near `handleRejectPlan` (after line 1007),
    mirroring its exact try/catch/`actionLoading`/toast shape. Each of the three
    RPC calls is now individually wrapped so the catch block below knows which
    one failed (resolves BLOCKER 5.2), and the catch block re-fetches the item
    directly (not just via `load()`'s side effect) so it knows the item's real
    post-failure status — needed because `triggerTriage`'s own internal
    `ready→idea` CAS (`backlog_service_trigger_triage.go:249-260`) can commit
    before a later step in that same call fails, leaving the item at `"idea"`
    rather than `"ready"` (pre-mortem P1 #1):
    ```tsx
    const handleSendBackWithFeedback = useCallback(
      async (feedback: string) => {
        if (!item) return;
        const toastKey = `${item.id}:send_back_ready`;
        setActionLoading("send_back_ready");
        try {
          try {
            await transitionStatus(item.id, "ready", {
              expectedStatus: item.status,
              expectedUpdatedAt: item.updatedAtRaw,
              overrideReason: feedback,
            });
          } catch (e) {
            // Nothing has changed server-side — item.status is still accurate.
            throw new SendBackError("transition", item.status, e);
          }
          try {
            await rejectPlan(item.id, feedback);
          } catch (e) {
            // transitionStatus committed; the item is "ready" even though this
            // call failed (rejectPlan doesn't change status).
            throw new SendBackError("reject", "ready", e);
          }
          try {
            await triggerTriage(item.id, feedback);
          } catch (e) {
            // Can't assume "ready" here (pre-mortem P1 #1): triggerTriage's own
            // internal CAS may have already moved the item to "idea" before
            // this failure. Re-fetch directly (not via load(), whose result
            // isn't returned to this scope) to learn the real status.
            const fresh = await getBacklogItem(item.id);
            throw new SendBackError("triage", fresh?.status ?? "ready", e);
          }
          showActionToast("Feedback sent — retriage started.", "success", toastKey);
          await load();
        } catch (e) {
          // Always re-fetch the displayed item here, regardless of which of the
          // three calls above failed. Once transitionStatus (call 1) succeeds,
          // the server has already committed a status change — and Epic 1.2's
          // teardown has already stopped any live session — even if rejectPlan
          // (call 2) or triggerTriage (call 3) is what actually failed. Without
          // this, the local `item` stays stale and a retry would replay
          // transitionStatus with the now-stale expectedStatus/expectedUpdatedAt
          // CAS precondition against the item's real, already-advanced state,
          // failing a second time with a confusing, unrelated
          // ErrPreconditionFailed. Re-fetching unconditionally is a cheap no-op
          // on the rarer branch where transitionStatus itself is what failed.
          await load();
          // Iteration 2 repair pass CONCERN fix: a partial failure (call
          // 2/3 failed after call 1 already committed the status change)
          // gets a neutral toast that doesn't contradict the more accurate
          // in-form message SendBackFeedbackBox now shows for that same
          // case (ux.md Surface 7) — only a true call-1 failure (nothing
          // changed server-side) keeps the "Failed to send back." framing.
          const toastMessage =
            e instanceof SendBackError && e.failedAt !== "transition"
              ? "Send-back needs attention — see details below."
              : getErrorMessage(e, "Failed to send back.");
          showActionToast(toastMessage, "error", toastKey);
          throw e; // still a SendBackError (or the original error) — SendBackFeedbackBox's catch reads it
        } finally {
          if (mountedRef.current) setActionLoading(null);
        }
      },
      [item, transitionStatus, rejectPlan, triggerTriage, getBacklogItem, load, showActionToast]
    );
    ```
    (`item.status` is already typed as `BacklogItemStatus` on the `BacklogItem`
    interface, `useBacklogService.ts:129` — no cast needed. `transitionStatus`'s
    `options.expectedStatus`/`expectedUpdatedAt` shape is already used at
    `handleApplyTriageSuggestions`, line 1030, and documented at
    `useBacklogService.ts:665-680`. `getBacklogItem` is already in scope — it's
    what `load()` itself calls, line 506.)
- **Why `actionError` survives the `load()` call above (iteration 2
  plan-repair pass, closing the fresh UX triad-lens BLOCKER):**
  `actionError` is `SendBackFeedbackBox`'s own local `useState`
  (Task 2.1.1b), not anything passed down from `BacklogItemDetail`. React
  only discards a component's local state when that component is omitted
  from its parent's render output (unmounted). `await load()` above
  re-fetches the item and re-renders `ActionsSection` → `SendBackFeedbackBox`
  with a new `visible` value — but it never touches `SendBackFeedbackBox`'s
  own state directly. Before this repair pass, `SendBackFeedbackBox`'s
  `if (!visible) return null;` guard meant that re-render itself unmounted
  the component whenever the item's post-failure status (`ready` or `idea`)
  fell outside `CAN_SEND_BACK_READY`, discarding the `actionError` that
  `SendBackFeedbackBox.handleSubmit` had just set — before the operator ever
  saw it. Task 2.1.1b's guard (as of the iteration 4 repair pass: `if
  (!visible && !actionError && !isPending) return null;`) keeps the
  component instance mounted whenever `actionError` is set, so its state —
  including `actionError` — persists across this `load()`-triggered
  re-render exactly as any other local `useState` would across any other
  parent re-render. (The `!isPending` clause was added in iteration 4 to
  cover a separate gap — a Retry launched from `visible=false` blanking the
  component during its own pending phase, since it clears `actionError`
  before `onSubmit` resolves — see the guard's own comment in Task 2.1.1b
  and Story 2.1.1's dedicated AC.)
- **Why the Retry button (Task 2.1.1b) needs no separate handler here
  (BLOCKER B, iteration 3 repair pass):** `handleSendBackWithFeedback`'s
  `useCallback` dependency array already includes `item`
  (`[item, transitionStatus, rejectPlan, triggerTriage, getBacklogItem, load,
  showActionToast]`), so React regenerates the closure every time `item`
  changes — including the `await load()` this function's own catch block
  runs before re-throwing. That means `expectedStatus: item.status` and
  `expectedUpdatedAt: item.updatedAtRaw` inside the function body are never
  stale relative to the *previous* render's `item` — they're read fresh on
  whichever render produced the closure that actually gets invoked. When the
  operator clicks Retry, `SendBackFeedbackBox` calls the `onSubmit` prop it
  was most recently re-rendered with, which — since `ActionsSection`/
  `BacklogItemDetail` already re-rendered after `load()` updated `item` to
  `status: "idea"` — is the closure carrying `expectedStatus: "idea"`, not
  the original source status. No code change to this function was needed to
  satisfy this; this note exists so a future edit doesn't accidentally
  memoize `item.status`/`item.updatedAtRaw` into a ref or a narrower dep
  array and silently reintroduce staleness here.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
  `web-app/src/components/backlog/detail/SendBackError.ts`

##### Task 2.2.1b: Thread new props into `ActionsSection` (~4 min)
- In `web-app/src/components/backlog/BacklogItemDetail.tsx`'s `<ActionsSection>`
  usage (currently lines 1643-1661), add two props:
  ```tsx
  activeWorkSessionCount={activeWorkSessionCount}
  onSendBackWithFeedback={handleSendBackWithFeedback}
  ```
  (`activeWorkSessionCount` is already computed at line 249 — no new computation.)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.1c: Render `SendBackFeedbackBox` from `ActionsSection` (~5 min)
- In `web-app/src/components/backlog/detail/ActionsSection.tsx`:
  - Add `import { SendBackFeedbackBox } from "./SendBackFeedbackBox";`.
  - Add to `ActionsSectionProps` (after `onDispatchToJulesClick`, line 46):
    ```tsx
    activeWorkSessionCount: number;
    onSendBackWithFeedback: (feedback: string) => Promise<void>;
    ```
  - Add both to the destructured function parameters (line 60-75).
  - Replace the bare `send_back_ready` button block (lines 440-451) with:
    ```tsx
    <SendBackFeedbackBox
      visible={actions.has("send_back_ready")}
      activeWorkSessionCount={activeWorkSessionCount}
      disabled={actionLoading !== null && actionLoading !== "send_back_ready"}
      actionPending={actionLoading === "send_back_ready"}
      onSubmit={onSendBackWithFeedback}
    />
    ```
    (Leave the `send_back_idea` block at lines 428-439 untouched — out of scope,
    requirements.md's Out of Scope section.)
- Files: `web-app/src/components/backlog/detail/ActionsSection.tsx`

##### Task 2.2.1d: Integration test for `actionError` persisting across `load()`'s async re-render (CONCERN fix, ~6 min)
- **Why this task exists:** iteration 2's plan-repair pass added the
  `if (!visible && !actionError) return null;` guard (Task 2.1.1b) and
  justified it by reasoning that `actionError` is local `useState` and
  "survives" a parent-triggered re-render. A fresh review round flagged that
  reasoning as unverified: `handleSendBackWithFeedback`'s `setActionError(...)`
  (inside `SendBackFeedbackBox`, triggered by its own `onSubmit` promise
  rejecting) and the parent's `await load()` (triggered by the *same*
  rejection, but awaited separately in `BacklogItemDetail`'s catch block) are
  two independently-scheduled async state updates, not two `setState` calls
  in one synchronous batch — Task 2.1.1c's existing unit test only ever
  mounts `SendBackFeedbackBox` with `actionError` pre-set as static initial
  state, so it has never actually exercised the real timing: does the
  component ever render, even for one frame, with `actionError` cleared/unset
  and `visible` already `false` (which would return `null` and flash-unmount)
  before `actionError` gets set?
- In `web-app/src/components/backlog/BacklogItemDetail.sendBackFeedbackPersistence.test.tsx`
  (new — mirrors the existing `BacklogItemDetail.<scenario>.test.tsx`
  convention, e.g. `BacklogItemDetail.loadGuard.test.tsx`), mount
  `BacklogItemDetail` (not just `SendBackFeedbackBox` in isolation) with the
  real `ActionsSection`, mocking only the RPC layer (`transitionStatus`,
  `rejectPlan`, `triggerTriage`, `getBacklogItem`) — each mock must return a
  **real, independently-resolving `Promise`** (e.g.
  `new Promise((resolve) => setTimeout(resolve, N))`, or manually
  resolved/rejected promises with `await act(async () => { ... })` between
  each, never a synchronously-resolved mock), so the two state updates above
  genuinely interleave across separate microtask/macrotask turns the way they
  would in production, not collapse into one React batch by construction.
  - Given an item at `review`, submit send-back feedback where
    `transitionStatus` resolves but `triggerTriage` rejects *after* its own
    simulated `ready→idea` status change (mock `getBacklogItem` to return
    `status: "idea"` only after the rejection settles), assert via
    `@testing-library/react`'s `screen` queries polled across every
    intermediate render (e.g. a `MutationObserver`-backed helper, or
    `rerender`-triggering polling — whatever this repo's existing async test
    helpers provide, matching `BacklogItemDetail.loadGuard.test.tsx`'s own
    async-assertion style) that the send-back form/toggle region is **never**
    observed fully absent (no toggle AND no error AND no form) at any point
    between the rejection firing and the "Sent back, but retriage didn't
    start" error finally appearing — i.e. no flash-unmount.
  - Assert the same for the ordinary (non-idea-landing) partial-failure case
    (`triggerTriage` rejects with `status` still `ready`), since that
    branch's `visible` also flips `false` on the same `load()`.
- Run `cd web-app && npx jest BacklogItemDetail.sendBackFeedbackPersistence --no-coverage`
  and confirm it passes; if it *fails* against the Task 2.1.1b guard as
  currently specified, that's the actual finding this task exists to surface —
  fix the guard (not the test) and update this task's note accordingly rather
  than loosening the assertion.
- Files: `web-app/src/components/backlog/BacklogItemDetail.sendBackFeedbackPersistence.test.tsx` (new)

---

## Phase 3: Test Coverage & Feature Registry

### Epic 3.1: e2e coverage

**Goal**: Cover the send-back-with-feedback flow end to end (pitfalls.md §5: zero
existing e2e coverage of `send_back_ready`/`send_back_refining`, so this is
purely additive).

#### Story 3.1.1: e2e spec for send-back-with-feedback
**As a** maintainer, **I want** e2e coverage of the full send-back-with-feedback
flow, **so that** a regression is caught in CI rather than discovered manually.

**Acceptance Criteria**:
- The full happy path is covered end to end.
  - *Given* a backlog item seeded at status `review` with `planArtifactsPath` set
    (so `RejectPlan`'s precondition passes) is open in the detail pane, *When* the
    test opens the send-back form, types feedback text, and clicks Submit, *Then*
    the item's status badge shows `"ready"` and the Plan Review card shows
    `"Revisions requested"` with the typed feedback text visible — located via
    `data-testid`/ARIA only, no `waitForTimeout` (per `e2e-test-conventions`
    skill).

**Files**: `tests/e2e/pages/BacklogItemDetailPage.ts`,
`tests/e2e/backlog-send-back-feedback.spec.ts` (new)

##### Task 3.1.1a: Add page-object locators (~4 min)
- In `tests/e2e/pages/BacklogItemDetailPage.ts`, add locators and two helper methods
  following the file's existing `getByTestId` convention (see `lifecycleSummary`,
  `pipelineBadge`, lines 22-23):
  ```ts
  readonly sendBackToggle: Locator;
  readonly sendBackTextarea: Locator;
  readonly sendBackSubmit: Locator;
  // in constructor:
  this.sendBackToggle = page.getByTestId("backlog-action-send-back-feedback");
  this.sendBackTextarea = page.getByTestId("send-back-feedback-textarea");
  this.sendBackSubmit = page.getByTestId("backlog-action-send-back-feedback-submit");

  async submitSendBackFeedback(feedback: string) {
    await this.sendBackToggle.click();
    await this.sendBackTextarea.fill(feedback);
    await this.sendBackSubmit.click();
  }
  ```
- Files: `tests/e2e/pages/BacklogItemDetailPage.ts`

##### Task 3.1.1b: New spec (~5 min)
- Create `tests/e2e/backlog-send-back-feedback.spec.ts`, starting with the required
  feature-annotation header comment (`e2e-test-conventions` skill; mirror
  `backlog-manual-override.spec.ts`'s structure for seeding an item at `review` via
  the existing test-mutation helpers in `tests/e2e/pages/BacklogMutations.ts`):
  ```ts
  // @feature backlog:send-back-feedback
  ```
  Cover: (1) the toggle reveals the form and Submit starts disabled; (2) submitting
  non-empty feedback transitions the item to `ready` and shows the
  `changes_requested` card with the typed text (`expect(locator).toHaveValue(...)`/
  `toContainText(...)`, never `waitForTimeout`); (3) pressing Escape closes the form
  and clears the textarea.
- Run `cd tests/e2e && npx playwright test backlog-send-back-feedback.spec.ts` and
  confirm it passes.
- Files: `tests/e2e/backlog-send-back-feedback.spec.ts`

---

### Epic 3.2: Feature registry

**Goal**: Keep `docs/registry/features/` in sync per `docs/reference/feature-registry.md`.

#### Story 3.2.1: Register the new component
**As a** maintainer, **I want** the new component's feature marker picked up by the
registry generator, **so that** `docs/registry/features/` stays accurate.

**Acceptance Criteria**:
- The registry reflects the new component.
  - *Given* `SendBackFeedbackBox.tsx` has `// +feature: backlog:send-back-feedback`
    in its first 10 lines (already added in Task 2.1.1b), *When*
    `make registry-generate` is run, *Then* a corresponding entry appears under
    `docs/registry/features/` and `git status` shows it as a new/modified file ready
    to commit alongside the code change.

**Files**: `web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx`,
`docs/registry/features/*.json`

##### Task 3.2.1a: Run the generator and commit the diff (~3 min)
- Confirm `SendBackFeedbackBox.tsx`'s `// +feature: backlog:send-back-feedback`
  marker is present (added in Task 2.1.1b's file content — no separate edit needed if
  that task was done as written).
- Run `make registry-generate` from the repo root.
- Review the resulting diff under `docs/registry/features/` with `git status`/
  `git diff`, and stage it alongside this feature's other changed files.
- Files: `docs/registry/features/*.json` (generated, exact filename determined by
  the generator's existing naming convention)
