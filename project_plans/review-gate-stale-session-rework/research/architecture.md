# Architecture Research: review-gate-stale-session-rework

**Date**: 2026-07-24

## Prior analysis incorporated

- `project_plans/backlog-stuck-item-visibility/research/architecture.md` and `project_plans/backlog-stuck-item-visibility/implementation/plan.md` — the source of the `StuckReason` architecture this plan extends. That project has **shipped**, not just planned (see verification below); build on its committed shape rather than its original design docs where they've since diverged.
- `project_plans/review-queue-state-detection/` — covers the *general* working/idle/stuck detection heuristics for the Review Queue (spinner/tool-call/prompt parsing). Out of scope here; this plan only touches the specific `StalenessThreshold` value it defined and how one specific consumer (`notifyIfActiveWorkSessionStale`) uses it.

## Two structurally distinct "staleness" subsystems exist today — confirmed live in `main`

| | Review Queue "Stale" badge | Durable `StuckReasonStaleWork` |
|---|---|---|
| Entry point | `session/review_queue_determiner.go:262`, `determineAttentionReason` | `session/backlog_lifecycle.go:1917`, `reconcileStaleWorkSessions` (periodic reconcile tick) |
| Threshold | `session.DefaultReviewQueuePollerConfig().StalenessThreshold` = **2 min** (`session/review_queue_poller.go:49`, comment: "reduced from 5min") | `maxWorkSessionStaleness` = **2 hours** (`session/backlog_lifecycle.go:1874`) |
| Signal source | `Instance.GetTimeSinceLastMeaningfulOutput()` (tmux scrollback parsing) | `ItemSession.LastProgressAt` (`report_progress` MCP calls), falling back to `CreatedAt` |
| Item status scope | Any session in the Review Queue, independent of backlog status | `BacklogStatusInProgress` items only (`ListBacklogItems(BacklogItemFilter{Statuses: [in_progress]})`) |
| Durability | None — recomputed every poll tick, not persisted | Durable: `MarkStuck`/`FindOpenStuckStates` DB rows, survive restarts |
| Consumer signal | Ephemeral queue badge/priority (`ReasonStale`, `PriorityLow`) | `StuckReason.STALE_WORK` chip + full detail view in `backlog-stuck` UI, plus automated remediation (`remediateStaleWorkWithBackoffGate` → `StaleWorkRemediator.RemediateStaleWorkSession`, gated by `RemediationDue` 30-min backoff) |

**`notifyIfActiveWorkSessionStale` (`server/services/backlog_service_triage.go:882-967`) is a *third*, parallel mechanism** — it deliberately reuses the Review Queue's 2-minute threshold and computation (per its own doc comment, to avoid "inventing a second definition of stale"), but its output is neither the Review Queue badge nor a `MarkStuck` row: it only calls `eventBus.Publish(events.NewNotificationEvent(...))` directly — a one-shot toast with no persisted trace. This is the actual gap: not "no durable stuck-item infrastructure exists" (it does, and is mature), but "this specific call site was wired to the wrong existing mechanism's threshold and to no persistence mechanism at all."

## Why `StuckReasonStaleWork`'s existing detector cannot already cover this case

`reconcileStaleWorkSessions` filters items to `BacklogStatusInProgress` only. `notifyIfActiveWorkSessionStale` fires from inside `AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:1032`), whose own doc comment says it "transitions the item from review back to in_progress" — meaning at the point `notifyIfActiveWorkSessionStale` runs, **the item's current status is `review`**, not `in_progress`. It has already failed a review verdict and the work session that requested that review is still alive, per the task-protocol's explicit "stay in this session" instruction (`session/backlog_context.go:114`, rule 8). So this is not an overlapping detector accidentally duplicated — it is a genuinely uncovered item-status/reason combination: *review*-status item whose (still-alive) prior work session has stalled, blocking automatic reopen.

The closest existing sibling reason, `StuckReasonAbandonedReview` ("a review-status item has a review verdict on record but nothing active in flight" — `session/domain/backlog.go:45-47`), is explicitly the *opposite* case (no active session). This confirms a new reason (or an existing one's status precondition relaxed) is architecturally the right shape — not scope creep into an already-solved problem.

## Recommended architecture shape (for Phase 3 to validate/refine)

Follow the exact established pattern each of the 11 existing `StuckReason` values already follows (see `session/domain/backlog.go:38-100` for the full catalogue and `session/ent_repository_backlog.go:1035` `MarkStuck`/`:1130` `MarkStuckNotified`):

1. **Backend detection**: from within (or immediately alongside) `notifyIfActiveWorkSessionStale`, call `MarkStuck(ctx, itemID, <reason>, BacklogStatusReview, stuckContext)` instead of (or in addition to) publishing the bare notification event. `MarkStuck`'s `expectedStatus` precondition already exists precisely to handle "the item moved on between read and write" races (see `TestMarkStuck_should_returnAppliedFalse_When_StatusPreconditionMismatch`) — no new concurrency handling needed.
**Terminology note (backfilled after Phase 3 planning, for artifact-trail consistency): Phase 3 resolved this open question by choosing option (b2) — a new, distinct reason, named `StuckReasonReworkBlockedStale` (`"rework_blocked_stale"`), not the `stale_work_blocks_rework` placeholder name used speculatively below. See `implementation/plan.md`'s Domain Glossary and Pattern Decisions for the final name and rationale.**

2. **Reason value**: strong candidate is reusing `StuckReasonStaleWork` itself (same underlying "work session went stale" concept) but this requires either (a) relaxing `reconcileStaleWorkSessions`' hardcoded `BacklogStatusInProgress` filter to also accept `BacklogStatusReview`, or (b) keeping the two call sites separate but writing the same reason value from both (`MarkStuck`'s `expectedStatus` param already supports per-call-site status). Alternatively, a new distinct reason (e.g. `stale_work_blocks_rework`) keeps the two "stale work" stories (a still-in-progress item making no progress, vs. a post-review item whose reopen is blocked) separately labeled/filterable in the UI, at the cost of one more `StuckReason` constant, one more `toProtoStuckReason`/`fromProtoStuckReason` switch arm (`server/services/backlog_service_stuck.go:23-88`), and one more proto enum value + `make proto-gen`. **This is the single most consequential open design decision for Phase 3** — resolve it by checking whether the UI/product intent wants these two shown identically or distinctly (the toast copy for each is already worded differently: "may be hung or working silently" for in_progress vs. "can't reopen for another rework attempt" for review-blocked).
3. **Threshold**: whichever value Phase 3 lands on, it should NOT be `session.DefaultReviewQueuePollerConfig().StalenessThreshold` (2 min) — that value is calibrated for a different consumer (a low-priority "might be worth a look" queue signal) and is the direct cause of the reported 37/41 false positives when reused here as a hard automation gate. The existing `maxWorkSessionStaleness` (2h) is a strong reuse candidate since it's already calibrated for "is a work session genuinely stuck," but Phase 2's pitfalls research below discusses why a shorter value may still be justified for the specifically-blocking-an-automated-decision case.
4. **UI surfacing**: `web-app/src/components/backlog-stuck/{StuckItem,StuckItemDetail,StuckItemsSection,stuckReason}.{ts,tsx}` already render every `StuckReason` generically from the `STUCK_REASON_LABELS`/`_ICONS`/`_CLASS` maps (`stuckReason.ts:15-60`) — adding a new reason (or reusing `STALE_WORK`'s existing entry) requires no new component, only a new map entry (if a new reason) and confirming the click-through/detail view surfaces a path to the item. **The "reopen and confirm" action itself already exists and does not need to be built**: `GateVerdictBox.tsx`'s "Reopen for Revision" button (`web-app/src/components/backlog/GateVerdictBox.tsx:320,331,350,371`, wired via an `onReopen` callback prop) already renders on a review-status item's detail page when it has a FAIL/PARTIAL/UNVERIFIABLE verdict — which is exactly the state a review-blocked item is in. The actual UX gap is *discoverability* (nothing points the user at that item from a durable list) rather than a missing action.

## Event-Command-Policy table

This is not a multi-actor business domain in the EventStorming sense — it's a single automated policy gate with one human escape hatch. A lightweight table still clarifies the flow:

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| `ReviewVerdictFailed` | Whenever a review FAILs and the item's prior work session is still open | `AttemptAutoReopen` | `AutoReopenAfterFailedReview` (backend) |
| `ActiveWorkSessionFound` | Whenever `AttemptAutoReopen` finds `hasActiveWorkSession == true` | `CheckWorkSessionLiveness` | `AutoReopenAfterFailedReview` (backend) |
| `WorkSessionJudgedStale` | Whenever `idle > threshold` (recalibrated, TBD) | `MarkItemStuck` (NEW — currently only `PublishNotification`) | `notifyIfActiveWorkSessionStale` (backend) |
| `ItemMarkedStuck` | Whenever a new open `BacklogStuckState` row is created | `NotifyOperator` (existing) + `SurfaceInStuckItemsList` (NEW, largely free via existing generic UI) | Stuck-items reconcile sweep / UI |
| `OperatorConfirmsStuck` | Whenever the operator clicks "Reopen for Revision" (existing button, existing RPC) | `ReopenForRevision` | Human, via `GateVerdictBox` |
| `WorkSessionResumedProgress` | Whenever a later tick finds the session no longer stale | `ResolveStuckState` (existing `resolveStuckLogged` pattern) | Reconcile sweep |

## Integration points

- `server/services/backlog_service_triage.go` — `notifyIfActiveWorkSessionStale`, `AutoReopenAfterFailedReview` (the call site to change).
- `session/backlog_lifecycle.go` — `reconcileStaleWorkSessions`, `maxWorkSessionStaleness`, `resolveStuckLogged` (the sibling detector/resolver pattern to extend or parallel).
- `session/ent_repository_backlog.go` — `MarkStuck`, `MarkStuckNotified`, `FindOpenStuckStates` (existing, reusable as-is).
- `session/domain/backlog.go` — `StuckReason` enum (extend only if Phase 3 picks a new reason value).
- `server/services/backlog_service_stuck.go` — `toProtoStuckReason`/`fromProtoStuckReason` (extend only if new reason value).
- `proto/session/v1/*.proto` — `StuckReason` enum (extend + `make proto-gen` only if new reason value).
- `web-app/src/components/backlog-stuck/stuckReason.ts` — label/icon/class maps (extend only if new reason value; free if reusing `STALE_WORK`).
- `session/review_queue_poller.go` — `DefaultReviewQueuePollerConfig`, `StalenessThreshold` (candidate for a second, decoupled config field or a second exported constant, if Phase 3 decides the general badge and the rework-gate need genuinely different values).
- `session/backlog_context.go` — `taskProtocolBlock` rule 8 (item D, the polling-cadence wording fix — pure prompt-text change, no code path).

## Data flow / consistency requirements

- `MarkStuck`'s existing `expectedStatus` optimistic-concurrency precondition is sufficient for this feature's write path — no new consistency mechanism needed. Any new call site must pass the item's actual current status at the point of the check (`BacklogStatusReview`), matching the pattern the other 10 reasons already use.
- The existing notify-once semantics (`NotifiedAt` nil vs. set, `MarkStuckNotified`) should be reused for the new/extended detection path — do not reinvent a separate in-memory or ad-hoc dedup mechanism (this repo has explicitly moved away from in-memory `stuckReviewNotified`-style maps per the adjacent `backlog-stuck-item-visibility` project's own stated rationale).
