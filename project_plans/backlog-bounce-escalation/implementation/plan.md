# Implementation Plan: backlog-bounce-escalation

**Feature**: Durable escalation signals for backlog items that accumulate multiple
simultaneous stuck reasons, or that exhaust the remediation-attempt cap while still bouncing —
both surfaced via the existing `backlog_stuck_states` infrastructure, no new storage.
**Date**: 2026-08-11
**Status**: Ready for implementation
**ADRs**: [ADR-001 — synthetic StuckReason values](../decisions/ADR-001-multi-reason-escalation-as-synthetic-stuck-reason.md), [ADR-002 — defer flaky-test review differentiation](../decisions/ADR-002-defer-flaky-test-review-differentiation.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `StuckReasonMultipleReasons` | New `domain.StuckReason` constant (`"multiple_reasons"`) marking an item that has `multiReasonThreshold` or more *other*, non-escalation stuck reasons open simultaneously. | New in this project. |
| `StuckReasonBounceCapExhausted` | New `domain.StuckReason` constant (`"bounce_cap_exhausted"`) marking an item whose `bouncing` remediation gate hit `MaxRemediationAttempts` while `bouncing` itself is still open. | New in this project. |
| `OpenStuckStateData` | Existing per-row projection returned by `FindOpenStuckStates` — one row per open, un-snoozed `(item_id, reason)` pair, carrying `ItemID`, `Reason`, `FirstDetectedAt`, `NotifiedAt`, `RemediationAttempts`, `ItemStatus`. | `session/storage_backlog.go:894`. Reused unchanged as the input to the new count computation. |
| `multiReasonThreshold` | New `const` (value `2`) — minimum count of simultaneously open **non-escalation** stuck reasons on one item before it is escalated. | `session/stuck_decisions.go`. |
| `isMultiReasonEscalated` | New pure predicate `func(openNonEscalationCount int) bool` — `openNonEscalationCount >= multiReasonThreshold`. | `session/stuck_decisions.go`, same style as `isBouncing`. |
| `multiReasonNotifyDwell` | New `const` (`60 * time.Second`, one reconcile-tick width) — how long `StuckReasonMultipleReasons`' row must have been open (`FirstDetectedAt`) before its notification fires, so a single-tick threshold crossing doesn't notify immediately. | `session/stuck_decisions.go`. Independent constant (session package can't import `server/dependencies.go`'s ticker), matching `bounceMainBranch`'s existing precedent for duplicating a cross-package constant. |
| `multiReasonEscalationNotifyReady` | New pure predicate `func(firstDetected, now time.Time) bool` — `now.Sub(firstDetected) >= multiReasonNotifyDwell`. | `session/stuck_decisions.go`, same style as `stuckPRReady`/`abandonedReview`. |
| `reconcileMultiReasonEscalation` | New detector method on `*BacklogLifecycleListener`, registered via `runStuckDetector("multi_reason_escalation", ...)` in `ReconcileStuck`. Groups `FindOpenStuckStates`' result by `ItemID`, counts non-escalation reasons per item, `MarkStuck`/`ResolveStuck`s `StuckReasonMultipleReasons` accordingly, and notifies (once, dwell-gated) on first crossing. | `session/backlog_lifecycle.go`. |
| `justParked` | Existing bool returned by `RemediationDue` — true exactly when this call's attempt count reached `MaxRemediationAttempts`. | `session/backlog_remediation.go:168`. Reused unchanged as the trigger for Signal 2. |
| `autoReopenWithBackoffGate` | Existing method, `session/backlog_lifecycle_review.go:194`. Signature extended to accept `itemStatus BacklogStatus` (previously missing — needed so the new `MarkStuck` call for `bounce_cap_exhausted` has the item's real current status, since `handleReviewSessionExited`'s item is in `review` status at both call sites, not `in_progress`). | Existing bouncing-reason `justParked` notify call site — Signal 2's exact insertion point. |
| `otherReasonsCount` / `otherReasonLabels` | Existing `StuckItem.tsx` props, computed client-side in `StuckItemsSection.tsx` by grouping the flat `ListStuckBacklogItems` result by `itemId`. | Unchanged — the two new reasons flow through this existing computation automatically once they exist as rows. |
| `chipEscalated` | New vanilla-extract class in `stuckReason.css.ts` for the two new reasons' chip — visually distinct (not reusing any existing reason's color), per `research/ux.md`'s "never repurpose existing chip colors for severity" constraint. | New in this project. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Escalation signal storage | Synthetic `domain.StuckReason` values reusing `BacklogStuckState`/`MarkStuck`/`ResolveStuck` | ADR-001; `research/architecture.md` §1-3 | New `severity`/`escalated_at` column(s) on `BacklogStuckState` | Requires an ent migration and a parallel upsert/query path for zero behavioral benefit — the plain-string `reason` column already accepts any validated value with no schema change. |
| Escalation signal storage | (same as above) | ADR-001 | Purely computed at read time, no persisted row | Fails the requirement's own "durable... not a one-time toast" success metric; a value computed only inside a request handler isn't independently queryable or restart-survivable. |
| Multi-reason count computation | Fresh `FindOpenStuckStates` read, grouped by `ItemID` in-process, evaluated once per item after all reason-specific detectors have run this tick | `research/pitfalls.md` §2; `research/architecture.md` §4.1 | Cached/denormalized `open_reason_count` column on `backlog_items`, incremented/decremented per detector write | Reintroduces exactly the check-then-act race class the prior project's atomic `UPDATE ... WHERE` rewrite already closed for single-row writes, at the aggregate level instead. |
| Multi-reason detector ordering | Registered immediately **after** `self_heal` (not before it, and not merely "near" it per architecture.md's looser guidance) | This plan, refining `research/architecture.md` §4.1 | Registered before `self_heal` | If a terminal-status item still has stale non-escalation rows `self_heal` hasn't yet cleared this tick, counting them would over-count; running after `self_heal` guarantees the count reflects the tick's fully-settled, self-healed state. |
| Escalation threshold | Fixed `const multiReasonThreshold = 2` | `requirements.md` Open Questions (own suggested default, "≥2 would have caught both" live cases) | Weighted/tunable scoring rubric | Requirements' own Rabbit Holes section explicitly rejects this ("keep the initial version to a simple count/threshold, not a tunable rubric"). |
| Escalation threshold | (same as above) | This plan | Threshold of 3 | Would miss the live 2-reason case (`ccbfe7a6`: `bouncing` + `abandoned_review`) that is one of this project's two motivating examples. |
| Flap control (escalate) | Escalate (`MarkStuck`) immediately on crossing threshold every tick (idempotent no-op if already open); **debounce only the notification** via `multiReasonEscalationNotifyReady`'s `FirstDetectedAt`-based dwell check | `research/pitfalls.md` §1, §3 (explicit: "Debounce the escalation *notification* (not the underlying open-reason state)") | New in-memory "was elevated last tick" boolean map per item | Adds new mutable listener state that doesn't survive a restart, inconsistent with every other detector in this codebase deriving its decision from persisted timestamps only (`FirstDetectedAt`, `LastCheckedAt`). |
| Flap control (de-escalate) | Resolve (`ResolveStuck`) immediately once count drops below threshold — no symmetric dwell gate on de-escalation in this pass | This plan, see Unresolved Questions | Full hysteresis on de-escalation (require count to stay below threshold across 2+ ticks before resolving) | No existing persisted "count first dropped below threshold at time T" data point to hang a duration predicate on, unlike the notify-dwell case which reuses `FirstDetectedAt` for free; adding one would mean new mutable state (rejected above) or a second timestamp column (out of the "simple count/threshold" mandate). Flagged as a documented follow-up if empirically noisy — see Unresolved Questions. |
| Capped-while-bouncing detection | Extend the existing `justParked` branch inside `autoReopenWithBackoffGate` — no new detector, no new poll | `research/architecture.md` §3 | New standalone detector reading `RemediationAttemptsGTE(MaxRemediationAttempts)` combined with a `bouncing`-open check | `justParked` already computes exactly this condition as a side effect of `RemediationDue`, which every bouncing-reopen call site already invokes; a second detector would duplicate that computation and risk drifting out of sync with the gate's own timing. |
| Notification differentiation | Reuse existing-but-unused proto enum values `NOTIFICATION_TYPE_ERROR` (7) / `NOTIFICATION_PRIORITY_URGENT` (4) for both new signals' first-notify | `research/architecture.md` §5 | New proto fields/enum values for a numeric "severity" | `proto/session/v1/types.proto` already defines both values and nothing in the backlog lifecycle code uses them — visually distinct without any proto surface change. |
| Frontend severity treatment | Reuse the existing generic per-reason card rendering (`otherReasonsCount`, `GROUP_ORDER`, `STUCK_REASON_*` maps) plus one new additive CSS class (`chipEscalated`) for the two new reasons | `research/ux.md` §2-3 (explicit: extend `otherReasonsCount`, don't build new component family; never repurpose existing chip colors for severity) | New dedicated "severity badge" component | `research/ux.md` explicitly names this pattern as the one to extend, not replace, and warns that repurposing existing reason-chip colors for severity contradicts `stuckReason.ts`'s own documented "never color-only, never severity-ranked" design decision. |
| Flaky-test review differentiation (requirement item 3) | Deferred to its own follow-up backlog item; no code in this project | ADR-002; `research/pitfalls.md` §4; `research/build-vs-buy.md` §3b | Ship a title/description keyword heuristic now | Requirements' own Fallback Increment explicitly permits deferral; both research docs independently flag the keyword heuristic as more likely to mis-trigger than help, and the better-fitting behavioral alternative (`IsRepeatedFailure`-style) is itself greenfield detection logic out of this project's appetite. |
| Flaky-test review differentiation (requirement item 3) | (same as above) | ADR-002 | Ship a behavioral (`IsRepeatedFailure`-style) heuristic now | Calibrating a new behavioral signal against live bouncing items is its own complexity-2+ scoped effort, not a task that fits this plan's task-sizing constraints; better scoped as its own follow-up per the Rabbit Holes guidance against ballooning into "a general intent-classification subsystem." |

---

## Observability Plan

- **Logs**: `reconcileMultiReasonEscalation` logs at `log.InfoLog` on every state transition
  (`"escalated item=%s open_reasons=%d"` / `"de-escalated item=%s open_reasons=%d"`), matching
  every existing detector's transition-logging convention (e.g. `reconcileBouncingItems`'s
  `MarkStuck`/notify logging). The extended `autoReopenWithBackoffGate` logs
  `"bounce cap exhausted while still bouncing item=%s"` at `log.WarningLog` alongside its
  existing `justParked` branch.
- **Metrics**: none new — this codebase has no existing metrics/StatsD integration for the
  stuck-state subsystem (confirmed: `research/architecture.md`/`research/stack.md` name no
  metrics layer), so this plan does not introduce one. The durable `BacklogStuckState` rows
  themselves are the queryable signal.
- **Alerts**: no new alerting infrastructure — the durable row + differentiated
  `NOTIFICATION_TYPE_ERROR`/`NOTIFICATION_PRIORITY_URGENT` notification via the existing
  `EventBusNotifier` is the entire alert surface (single-user, self-hosted; per
  `research/build-vs-buy.md` §2, no oncall/paging need exists).

## Risk Control

- **Feature flag**: not gated. Both signals reuse the existing always-on `ReconcileStuck`
  ticker and existing `notifier` wiring (`l.enabled.Load()` already gates the whole listener,
  same as every other detector) — consistent with every other stuck-reason detector in this
  codebase, none of which are individually flagged.
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration to
  reverse — a reverted deploy simply stops writing the two new `reason` string values; any
  already-open rows for them become dead (never re-evaluated, never resolved) but remain
  harmless, non-blocking rows queryable via the existing stuck-items UI/RPC until an operator
  manually resolves/snoozes them (same as any other orphaned-but-inert `BacklogStuckState`
  row).
- **Staged rollout**: full rollout on merge (single-user instance, no cohort concept).

## Unresolved Questions

- [ ] Whether `bouncing` + `abandoned_review` co-occurs commonly enough (structurally, per
  `research/pitfalls.md` §2 "An item that will ALWAYS have 2 reasons 'by design'") that the
  fixed `multiReasonThreshold = 2` fires too often once live for more than a few days — blocks
  nothing in this plan (Story 3.3.1's post-ship verification step should explicitly check this
  against a week of live data), but should gate whether a future refinement (excluding
  structurally-implied reason pairs from the count) is needed — owner: Tyler, re-check after
  ~1 week of live escalation-row history.
- [ ] Whether de-escalation needs its own dwell/hysteresis gate (see Pattern Decisions table)
  — deferred as a documented, not-yet-needed refinement; only becomes a real question if
  Story 3.3.1's verification or subsequent live use shows visible flapping in the UI — owner:
  Tyler, same re-check point as above.

## Dependency Visualization

```
Phase 1: Backend Escalation Signals
  Epic 1.1 (domain+proto plumbing)
    Story 1.1.1 (domain constants) ──┐
    Story 1.1.2 (proto+RPC mapping) ─┼──> Epic 1.2 (Signal 1: multi-reason)
                                      │       Story 1.2.1 (pure predicates)
                                      │       Story 1.2.2 (detector+wiring) ──> Story 1.2.3 (selfHeal)
                                      │       Story 1.2.4 (tests, depends on 1.2.1-1.2.3)
                                      │
                                      └──> Epic 1.3 (Signal 2: capped-while-bouncing)
                                              Story 1.3.1 (gate extension)
                                              Story 1.3.2 (resolve call sites + selfHeal)
                                              Story 1.3.3 (tests, depends on 1.3.1-1.3.2)

Phase 2: Frontend Surfacing (depends on Phase 1's proto regen, Story 1.1.2)
  Epic 2.1
    Story 2.1.1 (labels/icons/GROUP_ORDER) ──> Story 2.1.2 (chipEscalated) ──> Story 2.1.3 (tests)

Phase 3: Registry, Decision Record, Verification (depends on Phases 1-2 complete)
  Epic 3.1  Story 3.1.1 (registry update)
  Epic 3.2  Story 3.2.1 (ADR-002 already written; file follow-up item)
  Epic 3.3  Story 3.3.1 (live verification against Success Metrics)
```

---

## Phase 1: Backend Escalation Signals

### Epic 1.1: Domain & Proto Plumbing for the Two New Stuck Reasons
**Goal**: Make `StuckReasonMultipleReasons` and `StuckReasonBounceCapExhausted` valid,
round-trippable `StuckReason` values end to end (Go domain type → proto enum → RPC mapping)
before any detector logic depends on them.

#### Story 1.1.1: Add the two new `domain.StuckReason` constants
**As a** detector implementer, **I want** two new validated `StuckReason` constants, **so
that** `MarkStuck`/`ResolveStuck` accept them without failing `IsValid()`.
**Acceptance Criteria**:
- `domain.StuckReasonMultipleReasons` and `domain.StuckReasonBounceCapExhausted` exist, are
  listed in `AllStuckReasons`, and `IsValid()` returns true for both.
  - *Given* `domain.StuckReasonMultipleReasons.IsValid()`, *When* called, *Then* it returns
    `true` (currently would return `false`, since the constant doesn't exist yet).
**Files**: `session/domain/backlog.go`

##### Task 1.1.1a: Add the two constants with doc comments (~4 min)
- Add `StuckReasonMultipleReasons StuckReason = "multiple_reasons"` and
  `StuckReasonBounceCapExhausted StuckReason = "bounce_cap_exhausted"` to the const block
  (after `StuckReasonRespawnBlockedActive`, line 150), each with a doc comment following the
  existing style (what triggers it, what resolves it, cross-reference to the detector).
- Files: `session/domain/backlog.go`

##### Task 1.1.1b: Register in `AllStuckReasons` and `IsValid()` (~3 min)
- Append both constants to the `AllStuckReasons` slice (line 153) and to the `switch` in
  `IsValid()` (line 172).
- Files: `session/domain/backlog.go`

---

#### Story 1.1.2: Add proto enum values and RPC mapping cases
**As a** frontend consumer, **I want** the two new reasons available as
`sessionv1.StuckReason` enum values, **so that** `ListStuckBacklogItems` can return them and
generated TS types stay exhaustive.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto`'s `StuckReason` enum has two new values (numbers 15, 16);
  `make proto-gen` regenerates both Go and TS bindings; `toProtoStuckReason`/
  `fromProtoStuckReason` round-trip both new values correctly.
  - *Given* `domain.StuckReasonMultipleReasons`, *When* `toProtoStuckReason` is called,
    *Then* it returns `sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS` (not
    `STUCK_REASON_UNSPECIFIED`).
  - *Given* `sessionv1.StuckReason_STUCK_REASON_BOUNCE_CAP_EXHAUSTED`, *When*
    `fromProtoStuckReason` is called, *Then* it returns `domain.StuckReasonBounceCapExhausted`.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`

##### Task 1.1.2a: Add two enum values to the proto `StuckReason` enum (~3 min)
- Add `STUCK_REASON_MULTIPLE_REASONS = 15;` and `STUCK_REASON_BOUNCE_CAP_EXHAUSTED = 16;`
  after `STUCK_REASON_RESPAWN_BLOCKED_ACTIVE = 14;` (line ~1085), each with a one-line doc
  comment mirroring the existing entries' style.
- Files: `proto/session/v1/backlog.proto`

##### Task 1.1.2b: Regenerate proto bindings (~2 min)
- Run `make proto-gen`. Confirm `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/backlog_pb.ts` both contain the two new enum values.
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/backlog_pb.ts` (generated)

##### Task 1.1.2c: Add mapping cases to `toProtoStuckReason`/`fromProtoStuckReason` (~4 min)
- Add `case domain.StuckReasonMultipleReasons: return sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS`
  and the `BounceCapExhausted` equivalent to `toProtoStuckReason` (line 28); add the two
  reverse cases to `fromProtoStuckReason` (line 67).
- Files: `server/services/backlog_service_stuck.go`

---

### Epic 1.2: Multi-Reason Escalation Detector (Signal 1)
**Goal**: An item with `multiReasonThreshold`+ simultaneously open non-escalation stuck
reasons gets a durable `StuckReasonMultipleReasons` row and a one-time, dwell-gated,
differentiated notification.

#### Story 1.2.1: Pure predicate functions
**As a** detector implementer, **I want** the threshold and dwell logic expressed as
DB-independent pure functions, **so that** they're exhaustively table-driven-testable without
a DB, matching every other detector's house style.
**Acceptance Criteria**:
- `isMultiReasonEscalated(openNonEscalationCount int) bool` returns `true` iff count >= 2.
  - *Given* `openNonEscalationCount = 2`, *When* `isMultiReasonEscalated(2)` is called, *Then*
    it returns `true`.
  - *Given* `openNonEscalationCount = 1`, *When* `isMultiReasonEscalated(1)` is called, *Then*
    it returns `false`.
- `multiReasonEscalationNotifyReady(firstDetected, now time.Time) bool` returns `true` iff
  `now.Sub(firstDetected) >= 60*time.Second`.
  - *Given* `firstDetected = T`, `now = T + 61s`, *When* called, *Then* it returns `true`.
  - *Given* `firstDetected = T`, `now = T + 30s`, *When* called, *Then* it returns `false`.
**Files**: `session/stuck_decisions.go`

##### Task 1.2.1a: Add `multiReasonThreshold` and `multiReasonNotifyDwell` constants (~3 min)
- Add `const multiReasonThreshold = 2` and `const multiReasonNotifyDwell = 60 * time.Second`
  near `bounceThreshold`, with doc comments explaining the threshold's source (requirements'
  own live-data-informed default) and the dwell's independence from
  `server/dependencies.go`'s ticker constant (session package can't import server).
- Files: `session/stuck_decisions.go`

##### Task 1.2.1b: Add `isMultiReasonEscalated` and `multiReasonEscalationNotifyReady` (~4 min)
- Implement both pure functions immediately below the new constants, doc-commented in the
  same style as `isBouncing`/`stuckPRReady`.
- Files: `session/stuck_decisions.go`

##### Task 1.2.1c: Table-driven tests for both predicates (~5 min)
- Add `TestIsMultiReasonEscalated_should_returnTrue_When_CountAtOrAboveThreshold`,
  `TestIsMultiReasonEscalated_should_returnFalse_When_CountBelowThreshold`,
  `TestMultiReasonEscalationNotifyReady_should_returnTrue_When_DwellElapsed`,
  `TestMultiReasonEscalationNotifyReady_should_returnFalse_When_WithinDwell` following the
  existing file's naming/table-driven style.
- Files: `session/stuck_decisions_test.go`

---

#### Story 1.2.2: `reconcileMultiReasonEscalation` detector + wiring
**As** the reconcile ticker, **I want** a detector that groups open stuck reasons by item,
marks/resolves the `multiple_reasons` row, and notifies once (dwell-gated), **so that** the
signal is computed fresh every tick from live data (per `research/pitfalls.md` §2 — never a
cached count) and is visible without a manual DB query.
**Acceptance Criteria**:
- An item with 2 open non-escalation reasons gets a `StuckReasonMultipleReasons` row within
  one reconcile tick; the notification fires only after the row has existed for
  `multiReasonNotifyDwell`, not on the tick that created it.
  - *Given* a `BacklogItemData` with open `OpenStuckStateData` rows for `bouncing` and
    `abandoned_review` (2 non-escalation reasons, item status `in_progress`), *When*
    `reconcileMultiReasonEscalation` runs on tick N, *Then* `MarkStuck(item, "multiple_reasons",
    BacklogStatusInProgress, ...)` is called and `applied=true`, but `l.notify(...)` is **not**
    called yet (row was just created this tick, `FirstDetectedAt == now`).
  - *Given* the same row still open on tick N+1 (>60s later), *When*
    `reconcileMultiReasonEscalation` runs, *Then* `l.notify(...)` fires with
    `NOTIFICATION_TYPE_ERROR`/`NOTIFICATION_PRIORITY_URGENT`, and `MarkStuckNotified` is
    called.
  - *Given* the item's `abandoned_review` reason later resolves (down to 1 open non-escalation
    reason: just `bouncing`), *When* `reconcileMultiReasonEscalation` runs on the next tick,
    *Then* `ResolveStuck(item, "multiple_reasons")` is called.
- Registered via `runStuckDetector("multi_reason_escalation", ...)` immediately **after**
  `self_heal` and before `auto_archive_done` in `ReconcileStuck`.
  - *Given* `ReconcileStuck`'s detector registration order, *When* read top-to-bottom, *Then*
    `self_heal` appears before `multi_reason_escalation`, which appears before
    `auto_archive_done`.
**Files**: `session/backlog_lifecycle.go`

##### Task 1.2.2a: Implement `reconcileMultiReasonEscalation` — grouping + escalate branch (~5 min)
- New method `func (l *BacklogLifecycleListener) reconcileMultiReasonEscalation(ctx
  context.Context, er *EntRepository)`. Call `er.FindOpenStuckStates(ctx)`, group by `ItemID`
  into `map[string][]OpenStuckStateData`, filtering out any row whose `Reason` is
  `StuckReasonMultipleReasons` or `StuckReasonBounceCapExhausted` (the count must exclude the
  escalation reasons themselves — see plan.md Pattern Decisions / ADR-001 consequences). For
  each item where `isMultiReasonEscalated(len(nonEscalationRows))`, call
  `er.MarkStuck(ctx, itemID, domain.StuckReasonMultipleReasons, BacklogStatus(item.ItemStatus),
  contextString)` where `contextString` summarizes the open reasons (e.g. `"bouncing,
  abandoned_review"`, joined from the row set) for the `Context` column shown in the UI detail
  view.
- Files: `session/backlog_lifecycle.go`

##### Task 1.2.2b: Notify branch (dwell-gated, notify-once) (~5 min)
- After `MarkStuck`, re-fetch (or reuse, if `MarkStuck` is extended to return the row —
  simpler: do a second small lookup via the already-fetched `open` slice re-grouped, or track
  `FirstDetectedAt` from the pre-`MarkStuck` open row if one existed, else `now`) whether the
  row's `FirstDetectedAt` satisfies `multiReasonEscalationNotifyReady`. If ready and
  `NotifiedAt == nil`, call `l.notify(itemID, "Multiple stuck reasons open", <message
  listing the reasons>, 7 /* ERROR */, 4 /* URGENT */)` then `er.MarkStuckNotified(ctx, itemID,
  domain.StuckReasonMultipleReasons)`.
- Files: `session/backlog_lifecycle.go`

##### Task 1.2.2c: De-escalate branch (~3 min)
- For every item that currently has an open `StuckReasonMultipleReasons` row (from the same
  `FindOpenStuckStates` result, filtered to that reason) but whose non-escalation open-reason
  count has dropped below `multiReasonThreshold`, call `er.ResolveStuck(ctx, itemID,
  domain.StuckReasonMultipleReasons)`.
- Files: `session/backlog_lifecycle.go`

##### Task 1.2.2d: Wire into `ReconcileStuck` (~2 min)
- Add `l.runStuckDetector("multi_reason_escalation", &okNames, &panickedNames, func() {
  l.reconcileMultiReasonEscalation(ctx, er) })` immediately after the existing `self_heal`
  block (after line ~1067) and before `auto_archive_done`.
- Files: `session/backlog_lifecycle.go`

---

#### Story 1.2.3: `selfHealStuck` — explicit no-case decision
**As** the self-heal sweep, **I want** an explicit, documented decision about whether
`StuckReasonMultipleReasons` needs a switch case, **so that** the omission is intentional, not
an oversight (per ADR-001's consequence list).
**Acceptance Criteria**:
- `selfHealStuck`'s switch has **no** case added for `StuckReasonMultipleReasons` — it falls
  through to the `default: continue` branch — with a comment explaining why (its own detector,
  `reconcileMultiReasonEscalation`, is the sole resolver via the de-escalate branch in Story
  1.2.2c; the blanket terminal-status rule at the top of `selfHealStuck` still catches
  done/archived items regardless).
  - *Given* an item with an open `StuckReasonMultipleReasons` row transitions to `done`,
    *When* `selfHealStuck` runs, *Then* the row is resolved by the existing blanket terminal
    rule (line ~1636), not by a reason-specific case — confirming no new case is needed for
    that path.
**Files**: `session/backlog_lifecycle.go`

##### Task 1.2.3a: Add explanatory comment at the `default` case (~2 min)
- In `selfHealStuck`'s switch (line ~1643), extend the existing `default:` comment to
  explicitly name `multiple_reasons` alongside `autonomous_stuck`, `push_failed`,
  `rework_cap` as "resolved by its own detector or the blanket terminal rule, not a
  status-anchor case here."
- Files: `session/backlog_lifecycle.go`

---

#### Story 1.2.4: Integration tests for Signal 1
**As a** maintainer, **I want** end-to-end tests against a real (test) DB proving the
escalate/notify/de-escalate cycle, **so that** the dwell-gate and threshold behavior is
verified against actual `MarkStuck`/`ResolveStuck` semantics, not just the pure predicates.
**Acceptance Criteria**:
- A table-driven or scenario test seeds 2+ open `OpenStuckStateData`-shaped rows for one item,
  runs `reconcileMultiReasonEscalation`, and asserts a `multiple_reasons` row now exists with
  `NotifiedAt == nil`.
  - *Given* an item with `bouncing` and `abandoned_review` rows freshly `MarkStuck`'d in the
    test DB, *When* `reconcileMultiReasonEscalation(ctx, er)` runs once, *Then*
    `FindOpenStuckStates` returns a `multiple_reasons` row for that item with `NotifiedAt ==
    nil`.
- A second run of the same test, with the row's `FirstDetectedAt` backdated past the dwell
  window, asserts the notification fires and `NotifiedAt` is set.
  - *Given* the `multiple_reasons` row's `FirstDetectedAt` set to `now - 61s`, *When*
    `reconcileMultiReasonEscalation` runs again, *Then* the test's stub `Notifier` records
    exactly one call, and `FindOpenStuckStates` shows `NotifiedAt != nil`.
- A third case resolves one of the two underlying reasons and asserts the `multiple_reasons`
  row gets `ResolveStuck`'d on the next run.
  - *Given* `abandoned_review`'s row is `ResolveStuck`'d (leaving only `bouncing` open), *When*
    `reconcileMultiReasonEscalation` runs, *Then* `FindOpenStuckStates` no longer returns an
    open `multiple_reasons` row for that item.
**Files**: `session/backlog_lifecycle_stuck_test.go`

##### Task 1.2.4a: Escalate + dwell-gated-notify test (~5 min)
- Add `TestReconcileMultiReasonEscalation_should_MarkStuckWithoutNotifying_When_ThresholdFirstCrossed`
  and `TestReconcileMultiReasonEscalation_should_Notify_When_DwellElapsedAndStillOpen`,
  following this file's existing ent-test-DB setup pattern (see the `bouncing` tests around
  line 3447 for the seeding idiom).
- Files: `session/backlog_lifecycle_stuck_test.go`

##### Task 1.2.4b: De-escalate test (~4 min)
- Add `TestReconcileMultiReasonEscalation_should_ResolveStuck_When_CountDropsBelowThreshold`.
- Files: `session/backlog_lifecycle_stuck_test.go`

##### Task 1.2.4c: Exclusion test — escalation reasons don't self-count (~3 min)
- Add `TestReconcileMultiReasonEscalation_should_ExcludeEscalationReasonsFromCount_When_Counting`
  — seed `bouncing` + `multiple_reasons` (2 rows) for one item and assert escalation does NOT
  fire from those 2 rows alone (since only 1 is non-escalation), guarding the exact
  self-reinforcement risk named in ADR-001's consequences.
- Files: `session/backlog_lifecycle_stuck_test.go`

---

### Epic 1.3: Capped-While-Bouncing Escalation Marker (Signal 2)
**Goal**: The instant `bouncing`'s remediation gate parks (`justParked`), a durable
`bounce_cap_exhausted` row is created with a differentiated notification, and it clears
whenever `bouncing` itself resolves.

#### Story 1.3.1: Extend `autoReopenWithBackoffGate`'s `justParked` branch
**As** the bouncing-reason remediation gate, **I want** a durable marker + differentiated copy
when it parks, **so that** "capped while still bouncing" is distinguishable from an ordinary
single-reason park (requirement item 2's exact gap).
**Acceptance Criteria**:
- `autoReopenWithBackoffGate` gains an `itemStatus BacklogStatus` parameter; both call sites
  (`handleReviewSessionExited`, lines 132 and 149) pass `BacklogStatus(item.Status)`.
  - *Given* `handleReviewSessionExited` is processing an item currently in `review` status,
    *When* it calls `autoReopenWithBackoffGate`, *Then* the call passes
    `BacklogStatus(item.Status)` (i.e. `BacklogStatusReview`), not a hardcoded
    `BacklogStatusInProgress` — a hardcoded value would make the subsequent `MarkStuck` call's
    `expectedStatus` precondition silently fail (`applied=false`) every time, since
    `handleReviewSessionExited`'s item is never `in_progress` at this call site.
- When `justParked` is true, in addition to (replacing, for this specific bouncing-cap-out
  case, not the sibling notifications in other files) the existing generic notify, the code
  calls `MarkStuck(itemID, StuckReasonBounceCapExhausted, itemStatus, ...)` and sends a
  message stating the cap was hit *while still bouncing*, using `NOTIFICATION_TYPE_ERROR`/
  `NOTIFICATION_PRIORITY_URGENT` instead of the existing `WARNING`/`HIGH`.
  - *Given* `justParked == true` and `itemStatus == BacklogStatusReview`, *When* the branch
    executes, *Then* `er.MarkStuck(ctx, itemID, domain.StuckReasonBounceCapExhausted,
    BacklogStatusReview, ...)` is called and returns `applied=true`, and `l.notify(...)` is
    called with type `7` and priority `4` (not `8`/`3`).
**Files**: `session/backlog_lifecycle_review.go`

##### Task 1.3.1a: Extend the function signature and both call sites (~4 min)
- Change `func (l *BacklogLifecycleListener) autoReopenWithBackoffGate(ctx context.Context,
  itemID, itemTitle string)` to accept `itemStatus BacklogStatus` as a 4th parameter. Update
  both call sites at lines 132 and 149 to pass `BacklogStatus(item.Status)`.
- Files: `session/backlog_lifecycle_review.go`

##### Task 1.3.1b: Replace the `justParked` branch body (~5 min)
- Replace lines 205-211 (the existing generic `notify` call) with: `MarkStuck(ctx, itemID,
  domain.StuckReasonBounceCapExhausted, itemStatus, "bouncing remediation cap exhausted while
  bouncing reason still open")` (best-effort, log a warning on error, don't return), then
  `l.notify(itemID, "Bounce cap exhausted — retry loop not converging", fmt.Sprintf("%s —
  automated rework hit its retry cap (%d attempts) while still bouncing between in_progress
  and review. This is evidence the retry loop itself isn't converging, not a transient
  failure — a different approach may be needed before using Reset.", itemTitle,
  MaxRemediationAttempts), 7, 4)`, then `MarkStuckNotified` for the new reason (best-effort).
- Files: `session/backlog_lifecycle_review.go`

---

#### Story 1.3.2: Resolve `bounce_cap_exhausted` alongside `bouncing`, plus self-heal backstop
**As** the reconciler, **I want** `bounce_cap_exhausted` to clear whenever `bouncing` itself
resolves, **so that** the escalation marker never outlives the condition it describes.
**Acceptance Criteria**:
- Both `resolveStuckLogged(..., domain.StuckReasonBouncing, ...)` call sites in
  `reconcileBouncingItems` also resolve `StuckReasonBounceCapExhausted`.
  - *Given* an item has open `bouncing` and `bounce_cap_exhausted` rows, and its PR is
    confirmed merged, *When* `reconcileBouncingItems`'s merged-branch resolves `bouncing`
    (line 1456), *Then* it also calls `l.resolveStuckLogged(ctx, er, item.ID,
    domain.StuckReasonBounceCapExhausted, "reconcileBouncingItems/merged")` in the same
    branch.
- `selfHealStuck`'s switch gets a case for `StuckReasonBounceCapExhausted` mirroring
  `StuckReasonBouncing`'s own anchor-status rule (resolve if item status is no longer
  `in_progress` or `review`), as a backstop for any path that transitions status without going
  through the explicit resolve call sites above.
  - *Given* an open `bounce_cap_exhausted` row on an item now in `pr_pending` status, *When*
    `selfHealStuck` runs, *Then* `resolve = true` for that row (matches `bouncing`'s own rule),
    and the row is resolved.
**Files**: `session/backlog_lifecycle.go`

##### Task 1.3.2a: Add the second `resolveStuckLogged` call at both `bouncing` resolve sites (~3 min)
- At lines 1456 and 1479, add a second `l.resolveStuckLogged(ctx, er, item.ID,
  domain.StuckReasonBounceCapExhausted, "<same caller tag>")` immediately after each existing
  `bouncing` resolve call.
- Files: `session/backlog_lifecycle.go`

##### Task 1.3.2b: Add `selfHealStuck` case (~3 min)
- Add `case domain.StuckReasonBounceCapExhausted: resolve = row.ItemStatus !=
  BacklogStatusInProgress && row.ItemStatus != BacklogStatusReview` immediately after the
  existing `StuckReasonBouncing` case (line 1650), with a comment noting it mirrors bouncing's
  own anchor scope since `bounce_cap_exhausted` can only ever coexist with an open `bouncing`
  row.
- Files: `session/backlog_lifecycle.go`

---

#### Story 1.3.3: Tests for Signal 2
**As a** maintainer, **I want** tests proving the differentiated notify fires exactly at
`justParked`, and that the marker clears with `bouncing`, **so that** regressions in the
gate-extension logic are caught.
**Acceptance Criteria**:
- A test drives `RecordRemediationAttempt` up to `MaxRemediationAttempts - 1` (not yet
  parked), calls `autoReopenWithBackoffGate`, and asserts no `bounce_cap_exhausted` row
  exists yet; one more attempt crosses the cap and the row appears with the ERROR/URGENT
  notify.
  - *Given* an item with a `bouncing` row at `remediation_attempts = MaxRemediationAttempts -
    1`, *When* `autoReopenWithBackoffGate` runs and records the next attempt (reaching the
    cap), *Then* `justParked == true`, a `bounce_cap_exhausted` row exists with `NotifiedAt !=
    nil`, and the stub notifier recorded a call with type `7`/priority `4`.
- A test resolves `bouncing` (merged path) and asserts `bounce_cap_exhausted` resolves too.
  - *Given* both `bouncing` and `bounce_cap_exhausted` open for an item, *When*
    `reconcileBouncingItems` resolves it via the merged-PR branch, *Then*
    `FindOpenStuckStates` returns neither row for that item afterward.
**Files**: `session/backlog_lifecycle_stuck_test.go`, `session/backlog_lifecycle_test.go`

##### Task 1.3.3a: `justParked` → `bounce_cap_exhausted` test (~5 min)
- Add `TestAutoReopenWithBackoffGate_should_MarkBounceCapExhausted_When_JustParked`, seeding
  via the existing `RecordRemediationAttempt` idiom (see `backlog_lifecycle_test.go:1176-1179`
  for the seeding pattern).
- Files: `session/backlog_lifecycle_test.go`

##### Task 1.3.3b: Resolve-alongside-bouncing test (~4 min)
- Add `TestReconcileBouncingItems_should_ResolveBounceCapExhausted_When_BouncingResolves`.
- Files: `session/backlog_lifecycle_stuck_test.go`

---

## Phase 2: Frontend Surfacing

### Epic 2.1: Reason Label/Icon/Class + Distinct Escalation Treatment
**Goal**: The two new reasons render with their own label/icon/chip (exhaustive
`Record<StuckReason, T>` maps force this at compile time), appear in `GROUP_ORDER` (which is
*not* compile-checked — the exact omission class of bug this file's own doc comment warns
about), and get a visually distinct — not existing-color-reusing — treatment per
`research/ux.md`.

#### Story 2.1.1: Labels, icons, classes, and `GROUP_ORDER` entries
**As a** user viewing the Stuck Items panel, **I want** the two new reasons to render with
real labels instead of falling back to "Unknown reason", **so that** the escalation is legible,
not a blank/fallback chip.
**Acceptance Criteria**:
- `STUCK_REASON_LABELS`/`STUCK_REASON_ICONS`/`STUCK_REASON_CLASS` all have entries for
  `MULTIPLE_REASONS` and `BOUNCE_CAP_EXHAUSTED`; TypeScript compiles (the `Record<StuckReason,
  T>` type forces this).
  - *Given* `StuckReason.MULTIPLE_REASONS`, *When* `getStuckReasonLabel` is called, *Then* it
    returns `"Multiple reasons stuck"` (not the `"Unknown reason"` fallback).
- `GROUP_ORDER` includes both new values.
  - *Given* an item with an open `multiple_reasons` row, *When* `StuckItemsSection` renders,
    *Then* the row appears under its own group heading (not silently dropped, per the file's
    own documented prior-incident warning about entries missing from `GROUP_ORDER`).
**Files**: `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/StuckItemsSection.tsx`

##### Task 2.1.1a: Add label/icon/class entries (~4 min)
- Add `[StuckReason.MULTIPLE_REASONS]: "Multiple reasons stuck"` and
  `[StuckReason.BOUNCE_CAP_EXHAUSTED]: "Bounce cap exhausted"` to `STUCK_REASON_LABELS`; a
  distinguishing icon (e.g. `"🔺"` for both, or two distinct escalation-flavored glyphs) to
  `STUCK_REASON_ICONS`; `styles.chipEscalated` (new, Story 2.1.2) to `STUCK_REASON_CLASS` for
  both.
- Files: `web-app/src/components/backlog-stuck/stuckReason.ts`

##### Task 2.1.1b: Add both to `GROUP_ORDER` (~2 min)
- Append `StuckReason.MULTIPLE_REASONS` and `StuckReason.BOUNCE_CAP_EXHAUSTED` to the
  `GROUP_ORDER` array, positioned after `StuckReason.BOUNCING` (adjacent to the reason they
  most directly escalate from — consistent with the array's existing "actionability, not
  severity" ordering rationale, not placed first as if ranking danger).
- Files: `web-app/src/components/backlog-stuck/StuckItemsSection.tsx`

---

#### Story 2.1.2: Distinct `chipEscalated` visual treatment
**As a** user, **I want** the two new reasons' chips to look visually distinct from every
existing reason's chip, **so that** escalation reads as "needs different handling," not just
another same-styled reason among fourteen others — without contradicting `stuckReason.ts`'s
existing "never severity-rank via color" convention for the other 14 reasons.
**Acceptance Criteria**:
- `chipEscalated` is a new, distinct style (e.g. a bold border + dedicated color) — not a
  reuse of any existing `chipXxx` style — defined in `stuckReason.css.ts`.
  - *Given* `chipEscalated`'s CSS declaration, *When* compared to every existing `chipXxx`
    export, *Then* it does not share its `style([...])` base array with `chipBouncing` or any
    other existing reason chip (i.e., it is its own independent `style()`, not a variant of one
    reused for a different reason).
**Files**: `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 2.1.2a: Add `chipEscalated` style (~4 min)
- Add `export const chipEscalated = style([chip, { /* distinct border/background/color not
  used by any other chipXxx export */ }]);` following the file's existing `style([chip, {...}])`
  composition pattern (base `chip` + reason-specific override, same as `chipBouncing` etc.).
- Files: `web-app/src/components/backlog-stuck/stuckReason.css.ts`

---

#### Story 2.1.3: Frontend tests
**As a** maintainer, **I want** tests proving both new reasons are exhaustively mapped and
rendered, **so that** the `GROUP_ORDER` omission class of bug (already burned once per this
file's own doc comment) cannot silently recur for these two reasons.
**Acceptance Criteria**:
- `getStuckReasonLabel`/`Icon`/`Class` return non-fallback values for both new reasons.
  - *Given* `StuckReason.BOUNCE_CAP_EXHAUSTED`, *When* `getStuckReasonLabel` is called, *Then*
    it does not equal `STUCK_REASON_LABELS[StuckReason.UNSPECIFIED]`.
- A `StuckItemsSection` render test seeds an item with an open `multiple_reasons` row and
  asserts its card is actually rendered (findable by test id), not silently absent despite
  the section's total count including it.
  - *Given* a seeded `StuckBacklogItem` with `reason = MULTIPLE_REASONS`, *When*
    `StuckItemsSection` renders, *Then* a card for that item is present in the DOM (matching
    the existing `StuckItemsSection_should_showOtherReasonsBadge_When_SameItemInMultipleGroups`
    test's assertion style).
**Files**: `web-app/src/components/backlog-stuck/stuckReason.test.ts`, `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx`

##### Task 2.1.3a: `stuckReason.ts` exhaustiveness tests (~3 min)
- Add `getStuckReasonLabel_should_returnDistinctLabel_When_MultipleReasons` and the
  `BounceCapExhausted` equivalent, following the existing
  `getStuckReasonLabel_should_returnDistinctLabel_When_ReworkBlockedStale` pattern.
- Files: `web-app/src/components/backlog-stuck/stuckReason.test.ts`

##### Task 2.1.3b: `StuckItemsSection` render test for the new `GROUP_ORDER` entries (~4 min)
- Add `StuckItemsSection_should_renderMultipleReasonsGroup_When_ItemEscalated`, mirroring the
  existing group-rendering test setup.
- Files: `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx`

---

## Phase 3: Registry, Decision Record, Verification

### Epic 3.1: Feature Registry Updates
**Goal**: Comply with `.claude/rules/feature-registry.md` — the modified `ListStuckBacklogItems`
surface (new enum values flow through it) and the modified `StuckItemsSection` component both
need their per-feature registry entries touched.

#### Story 3.1.1: Update registry entries and regenerate
**As a** repo maintainer, **I want** the registry to reflect the modified RPC/UI surfaces,
**so that** `make registry-generate` doesn't silently drift from what actually shipped.
**Acceptance Criteria**:
- `docs/registry/features/backend/backlog/list-stuck.json` and
  `docs/registry/features/frontend/backlog-stuck-items.json` both have `lastModified` bumped
  to the ship date, and `frontend/backlog-stuck-items.json`'s `testIds` includes the new test
  names added in Stories 1.2.4, 1.3.3, 2.1.3.
  - *Given* the new test `getStuckReasonLabel_should_returnDistinctLabel_When_MultipleReasons`
    now exists, *When* `docs/registry/features/frontend/backlog-stuck-items.json` is read,
    *Then* that test name appears in its `testIds` array.
- `make registry-generate` produces no new entries in `coverage-gaps.json` attributable to
  this project's changes.
**Files**: `docs/registry/features/backend/backlog/list-stuck.json`, `docs/registry/features/frontend/backlog-stuck-items.json`

##### Task 3.1.1a: Update both per-feature JSON files (~4 min)
- Bump `lastModified` on both files; add the new test names from Stories 1.2.4/1.3.3/2.1.3 to
  `backlog-stuck-items.json`'s `testIds`.
- Files: `docs/registry/features/backend/backlog/list-stuck.json`, `docs/registry/features/frontend/backlog-stuck-items.json`

##### Task 3.1.1b: Run `make registry-generate` and commit regenerated artifacts (~2 min)
- Run `make registry-generate`; diff `docs/registry/coverage-gaps.json` to confirm no
  unattributed new gaps.
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json` (generated)

---

### Epic 3.2: Flaky-Test Review Differentiation — Explicit Deferral
**Goal**: Requirement item 3 is closed out as a deliberate, documented deferral (ADR-002),
not silently dropped.

#### Story 3.2.1: File the follow-up backlog item
**As** the project owner, **I want** ADR-002's deferred scope captured as a real backlog item,
**so that** the behavioral-signal recommendation (not the keyword heuristic) survives to the
next planning pass.
**Acceptance Criteria**:
- A new backlog item exists (via `create_backlog_item` or the UI) titled along the lines of
  "Flaky-test-aware review strategy (behavioral signal)", body summarizing ADR-002's
  reasoning and pointing at `research/pitfalls.md` §4's behavioral-signal recommendation.
  - *Given* ADR-002 is written, *When* the follow-up item is created, *Then* its description
    references ADR-002's path and explicitly rules out the keyword-heuristic approach as
    already-considered-and-rejected (so the next planning pass doesn't re-litigate it from
    scratch).
**Files**: none (backlog item, not a code file)

##### Task 3.2.1a: Create the follow-up backlog item (~3 min)
- Create the item with title/description as above; link `project_plans/backlog-bounce-escalation/decisions/ADR-002-defer-flaky-test-review-differentiation.md`.
- Files: none

---

### Epic 3.3: End-to-End Verification Against Live Data
**Goal**: Confirm the shipped feature actually produces the escalated signals requirements.md's
Success Metrics describe, against whatever items are live at ship time (not hardcoded to the
three now-possibly-resolved example IDs, per requirements' own Feasibility Risk).

#### Story 3.3.1: Live verification pass
**As** the project owner, **I want** to re-run the investigation queries from requirements.md
against the shipped feature, **so that** the Success Metrics are confirmed against reality, not
just unit/integration tests.
**Acceptance Criteria**:
- Querying `backlog_stuck_states` for any item with 2+ open non-escalation reasons shows a
  corresponding open `multiple_reasons` row (or, if none currently qualify, a synthetic
  test item seeded specifically for this check does).
  - *Given* the live `backlog_stuck_states` table at verification time, *When* filtered to
    items with 2+ open non-`multiple_reasons`/`bounce_cap_exhausted` rows sharing an
    `item_id`, *Then* each such item also has an open `multiple_reasons` row.
- Any item at `remediation_attempts >= MaxRemediationAttempts` with an open `bouncing` row
  also has an open `bounce_cap_exhausted` row.
  - *Given* a live item matching that condition, *When* queried, *Then* a `bounce_cap_exhausted`
    row exists and is open for the same item.
**Files**: none (manual/scripted verification, not a code change)

##### Task 3.3.1a: Run the verification queries and record results (~5 min)
- Query the live DB (or via the `ListStuckBacklogItems` RPC) for both conditions above;
  record pass/fail in the PR description per this repo's evidence-with-source convention.
- Files: none
