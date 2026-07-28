# Implementation Plan: review-gate-stale-session-rework

**Feature**: Recalibrate stale-session detection thresholds and give the "rework blocked by a stale-but-alive session" bug a durable, actionable state instead of an ephemeral toast.
**Date**: 2026-07-24
**Status**: Ready for implementation
**ADRs**: ADR-001-staleness-threshold-recalibration

---

## Step 0.5 — Alternatives explored

1. **Reuse `StuckReasonStaleWork` verbatim, widen its detector's status filter to include `review`.** Strength: zero new proto/enum surface, minimal code. Weakness: conflates two UX-distinct situations (a quietly-grinding in_progress item vs. an automation gate actively blocked) under one label and copy, and risks two independent call sites (`reconcileStaleWorkSessions` on its 2h/tick cadence, and the `AutoReopenAfterFailedReview` call site on its own event+backoff cadence) racing to mark/resolve the same reason with different timestamps.
2. **New, distinct `StuckReason` value (`REWORK_BLOCKED_STALE`) wired inline at the existing `notifyIfActiveWorkSessionStale` call site, using its own threshold.** Strength: clean separation, preserves the two toasts' already-distinct urgency framing, no risk of detector races, small and well-precedented amount of new surface (11 prior reasons already established this exact ceremony). Weakness: one more proto enum value, one more Go switch arm, one more TS map entry — all boilerplate, none of it novel.
3. **Auto-escalate past a long grace period by detaching the stale session's bookkeeping and respawning without killing the tmux process (requirements.md's "C").** Strength: would fully unblock automation without any human step. Weakness: explicitly out of scope per requirements.md — double-work/double-ship risk needs its own design; not attempted here.

**Chosen: Option 2.** It is the only option that doesn't either conflate two different signals (Option 1) or exceed this plan's explicit scope (Option 3). See ADR-001 for the threshold-value rationale specifically.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `StuckReason` | Existing validated string-backed enum (`session/domain/backlog.go`) classifying why a backlog item cannot make automated progress. | Not a sum type in the Go type-system sense — validated at the boundary via `IsValid()`, matching this codebase's established `BacklogStatus`/`ReviewOutcome` style. This plan follows the same convention rather than introducing a sealed-interface pattern inconsistent with the other 11 values. |
| `StuckReasonReworkBlockedStale` | **NEW.** `StuckReason` value (`"rework_blocked_stale"`) marking a `review`-status item whose prior (still-alive) work session has produced no output long enough that `AutoReopenAfterFailedReview` cannot safely respawn a fresh rework session. | Distinct from `StuckReasonStaleWork` (which covers `in_progress` items) and `StuckReasonAbandonedReview` (review-status items with *no* active session). |
| `maxReworkBlockStaleness` | **NEW.** `time.Duration` constant in `server/services/backlog_service_triage.go` (co-located with its sole consumers, `notifyIfActiveWorkSessionStale` and `ResolveReworkBlockedStaleIfRecovered` — not in `session/backlog_lifecycle.go`, per the same layering reasoning as `ReworkBlockStaleResolver` below), the threshold both compare `idle` against. Set to 15 minutes — see ADR-001. | Replaces the reused `session.DefaultReviewQueuePollerConfig().StalenessThreshold` (2 min) at this specific call site. |
| `ReviewQueuePollerConfig.StalenessThreshold` | Existing config field (`session/review_queue_poller.go`) driving the general Review Queue "Stale" badge. | Value changes from 2 min → 5 min (ADR-001); remains a *separate* threshold from `maxReworkBlockStaleness` — the two are intentionally decoupled by this plan, not merged. |
| `maxWorkSessionStaleness` | Existing constant (2h) gating `StuckReasonStaleWork`'s `in_progress`-item detector. Untouched by this plan — cited only as calibration precedent in ADR-001. | Not reused directly; `maxReworkBlockStaleness` is a distinct, shorter value for a distinct, more urgent purpose (see ADR-001 §3). |
| `notifyIfActiveWorkSessionStale` | Existing function (`server/services/backlog_service_triage.go`) — extended by this plan to also call `MarkStuck`/manage a resolve pass, not just publish a notification event. | |
| `AutoReopenAfterFailedReview` | Existing function whose `hasActiveWorkSession` guard is the entry point into `notifyIfActiveWorkSessionStale`. Not restructured — only its downstream call is extended. | |
`ReworkBlockStaleResolver` | **NEW.** Narrow interface defined in `session/backlog_lifecycle.go` (mirrors `StaleWorkRemediator`), implemented by `BacklogService`, injected via `SetReworkBlockStaleResolver`. One method: `ResolveReworkBlockedStaleIfRecovered(ctx, itemID) error`. | Exists because `session.BacklogLifecycleListener` has no `sessionStopper`/liveness dependency and shouldn't gain one directly — see Pattern Decisions. |
| `reconcileReworkBlockedStaleResolution` | **NEW.** Small periodic pass in `session/backlog_lifecycle.go` (mirrors `reconcileStaleWorkSessions`' "poll-shaped resolve" half), added to the existing reconcile tick: finds open `StuckReasonReworkBlockedStale` rows via `FindOpenStuckStates` and, for each, delegates the actual "is it still stale" check to `ReworkBlockStaleResolver`. | Orchestration-only — contains no liveness-checking logic itself, only the delegation call. |
| `taskProtocolBlock` rule 8 | Existing prompt text (`session/backlog_context.go`) instructing agents to "stay in this session... wait, then check again" with no concrete cadence. Edited by this plan to add an explicit interval. | Pure string-literal change. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `StuckReasonReworkBlockedStale` | Validated string enum, house style | This codebase's existing `StuckReason`/`BacklogStatus` convention | Sealed interface / sum type (type-driven-design's usual recommendation) | Consistency: all 11 existing `StuckReason` values use this convention, including their storage as a plain string ent column; introducing a different pattern for the 12th value would fragment the codebase's own established idiom for no functional gain. |
| Detection ("is this stale") | Inline check at the existing `notifyIfActiveWorkSessionStale` call site (Transaction Script — PoEAA) | PoEAA (Fowler) | New standalone periodic `reconcileReworkBlockedStale*Detection*` function (Domain Model-ish orchestration layered on top) | `notifyIfActiveWorkSessionStale` already fires with an appropriate cadence via its existing callers (immediate, on verdict-submit via `review_gate.go`; retried via `autoReopenWithBackoffGate`'s 30-min `RemediationDue` gate). A second, independent periodic detector would duplicate this computation and risk disagreeing with it — the exact "two independently-computed staleness signals disagreeing" failure mode `notifyIfActiveWorkSessionStale`'s own doc comment says it was built to avoid. |
| Resolution ("stop being stale") | Small dedicated periodic resolve pass in `session.BacklogLifecycleListener`, delegating the actual liveness check to `BacklogService` via a new narrow interface (`ReworkBlockStaleResolver`) | Mirrors `reconcileStaleWorkSessions`' "poll-shaped resolve" half for *orchestration*, and mirrors `StaleWorkRemediator`'s existing interface-injection shape for *layering* | (a) Rely solely on `selfHealStuck`; (b) give `BacklogLifecycleListener` its own direct liveness-checker dependency | (a) rejected: `selfHealStuck` only clears a row once the item's status changes, but `AutoReopenAfterFailedReview` explicitly does NOT transition status while `hasActiveWorkSession == true` — status can stay `review` the whole time a session is stuck-then-recovers, so `selfHealStuck` never fires for this case. (b) rejected on layering grounds, caught during planning: `session.BacklogLifecycleListener` (the `session` package) has no `sessionStopper`/`TimeSinceLastMeaningfulOutput` dependency and none should be added directly — `server/services.BacklogService` already owns that dependency, and the codebase's own established pattern for exactly this situation (`session`-package orchestration needing a `server/services`-layer, liveness-aware action) is a narrow interface defined in `session`, implemented by `BacklogService`, injected via a `SetX` setter — see `StaleWorkRemediator`/`SetStaleWorkRemediator` (`session/backlog_lifecycle.go:70-77`, wired at `server/dependencies.go:986`) as the exact precedent. `ReworkBlockStaleResolver` follows that precedent instead of crossing the layering boundary. |
| Threshold values | Two decoupled named constants (PoEAA: parameterize by call site) | This plan's own analysis (pitfalls.md #1) | One shared `StalenessThreshold` (status quo) | The status-quo shared value is the root cause under investigation — see ADR-001 for the full rationale on why "might be worth a look" (Review Queue badge) and "block an automated action" (rework gate) are different-enough purposes to warrant different numbers, now that both are named and documented instead of silently shared. |
| Proto/UI wiring for the new reason | Extend the existing generic `Record<StuckReason, T>` maps (`stuckReason.ts`) | Existing house pattern | New bespoke React component for this one reason | The existing `backlog-stuck` components already render every `StuckReason` generically; a new component would be a Forwarding-only wrapper duplicating what the generic renderer already does (see `.claude/rules/interface-pollution-checklist.md` smell #4). |

---

## Migration Plan

No SQL schema migration required. `StuckReason` is stored as a plain string column on the existing `BacklogStuckState` ent entity (confirmed: all 11 existing values are Go-level constants, not a DB-level enum/check-constraint) — adding a 12th valid string value requires no ent schema change, only the Go-level `IsValid()` switch extension (Story 1.2.1) and the proto enum extension + `make proto-gen` (Story 1.2.2).

## Observability Plan

- **Logs**: reuse existing `log.WarningLog`/`log.InfoLog` structured-print patterns at the new `MarkStuck`/resolve call sites, mirroring `reconcileStaleWorkSessions`' existing log lines exactly (component tag `[AutoReopenAfterFailedReview]` / `[BacklogLifecycle]`, item ID, timing values).
- **Metrics**: none new — this repo has no metrics infrastructure beyond structured logs for this subsystem (confirmed in `backlog-stuck-item-visibility/requirements.md`'s own Observability Requirements section: "No new metrics/alerting infrastructure required... single-user internal tool without an oncall rotation"). Consistent with that precedent.
- **Alerts**: none new — the durable UI surfacing (Story 2.2.x) *is* the alerting mechanism for a single-user instance, per the same precedent.

## Risk Control

- **Feature flag**: not gated — this is a bug fix to detection thresholds and a small additive UI/backend wiring change with no user-visible behavior change beyond "the signal is now accurate and durable instead of noisy and ephemeral." A flag would add complexity without a meaningful rollback surface.
- **Rollback procedure**: standard revert via PR close + revert commit. The threshold changes (Story 1.1.x) are the highest-risk single commit to isolate for an easy partial revert if the new values prove wrong in practice — structure the PR/commits so threshold changes can be reverted independently of the StuckReason wiring if needed.
- **Staged rollout**: full rollout on merge (single-user instance, no staged-cohort mechanism exists in this codebase).

## Unresolved Questions

None — all previously-open requirements.md questions were resolved during planning (see ADR-001 for the threshold values, and Pattern Decisions above for the reason-reuse-vs-new-value decision). If live post-deploy monitoring (Story 4.1.1 in validation.md) shows either recalibrated threshold is still miscalibrated, that is an expected, planned-for possibility (ADR-001 explicitly flags both values as best-effort estimates pending live confirmation), not a plan gap.

---

## Dependency Visualization

```
Phase 1: Threshold Recalibration            Phase 2: Durable Stuck-State Wiring
┌─────────────────────────────┐             ┌──────────────────────────────────┐
│ 1.1.1a Add maxReworkBlock-  │             │ 1.2.1a Add StuckReasonRework-     │
│   Staleness const (in        │             │   BlockedStale to domain/backlog │
│   backlog_service_triage.go) │             │                                    │
│ 1.1.1b Use it in             │──┐          │   .go + IsValid + AllStuckReasons│
│   notifyIfActiveWork-        │  │          └───────────────┬────────────────┘
│   SessionStale                │  │                          │
│ 1.1.2a Recalibrate general    │  │          ┌───────────────▼────────────────┐
│   StalenessThreshold 2m→5m    │  │          │ 1.2.2a Add proto enum value +   │
└──────────────┬────────────────┘  │          │   make proto-gen                │
               │                    │          └───────────────┬────────────────┘
               │                    │                          │
               │                    ▼                          ▼
               │        ┌───────────────────────┐  ┌────────────────────────────┐
               │        │ 2.1.1a Wire MarkStuck  │  │ 2.1.2a Add toProto/from-   │
               │        │  into notifyIfActive-  │◄─┤  Proto switch arms         │
               │        │  WorkSessionStale       │  │  (backlog_service_stuck.go)│
               │        └───────────┬─────────────┘  └────────────────────────────┘
               │                    │
               │                    ▼
               │        ┌────────────────────────────┐
               │        │ 2.1.2a Define ReworkBlock- │
               │        │  StaleResolver interface +  │
               │        │  BacklogService impl        │
               │        │ 2.1.2b Add resolve pass to  │
               │        │  BacklogLifecycleListener,  │
               │        │  wire via SetReworkBlock-   │
               │        │  StaleResolver               │
               │        └───────────┬─────────────────┘
               │                    │
               │                    ▼
               │        ┌────────────────────────────┐
               │        │ 2.2.1a Add stuckReason.ts  │
               │        │  label/icon/class entries   │
               │        │  + stuckReason.css.ts chip  │
               │        └───────────┬─────────────────┘
               │                    │
               ▼                    ▼
     ┌───────────────────────────────────────┐
     │ 3.1.1a Task-protocol rule 8 cadence     │
     │   text edit (session/backlog_context.go)│
     │   — independent, can land any time      │
     └───────────────────────────────────────┘
```

---

## Phase 1: Threshold Recalibration

### Epic 1.1: Decouple and recalibrate staleness thresholds
**Goal**: Stop the rework-block gate from reusing the Review Queue's 2-minute badge threshold, and recalibrate the badge threshold itself given the observed 37/41 false-positive rate.

#### Story 1.1.1: Give the rework-block gate its own threshold
**As a** backlog automation operator, **I want** the "is this blocking work session actually stuck" check to use a threshold calibrated for that purpose, **so that** a single slow LLM turn no longer blocks a legitimate rework attempt and floods me with a false "stale-but-alive" toast.

**Acceptance Criteria**:
- `notifyIfActiveWorkSessionStale` compares `idle` against a new `maxReworkBlockStaleness` constant (15 minutes) instead of `session.DefaultReviewQueuePollerConfig().StalenessThreshold`.
  - *Given* a `review`-status item with an active work session whose `TimeSinceLastMeaningfulOutput()` returns `idle=10*time.Minute, live=true`, *When* `AutoReopenAfterFailedReview` is called and finds `hasActiveWorkSession==true`, *Then* `notifyIfActiveWorkSessionStale` does NOT fire a notification or mark the item stuck (10 min < 15 min threshold).
  - *Given* the same setup but `idle=20*time.Minute`, *When* the same call happens, *Then* `notifyIfActiveWorkSessionStale` DOES fire (20 min > 15 min threshold).
**Files**: `server/services/backlog_service_triage.go`

##### Task 1.1.1a: Add `maxReworkBlockStaleness` constant (~2 min)
- Add `const maxReworkBlockStaleness = 15 * time.Minute` in `server/services/backlog_service_triage.go` near `notifyIfActiveWorkSessionStale`, with a doc comment cross-referencing ADR-001 for the rationale, explicitly noting it is intentionally distinct from both `maxWorkSessionStaleness` (2h, `session/backlog_lifecycle.go`) and the Review Queue's `StalenessThreshold` (5 min after Story 1.1.2), and noting it will also be used by `ResolveReworkBlockedStaleIfRecovered` (Story 2.1.2) so the mark and resolve sides of this feature agree on one number.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.1.1b: Use the new constant in `notifyIfActiveWorkSessionStale` (~3 min)
- Replace `threshold := session.DefaultReviewQueuePollerConfig().StalenessThreshold` (line 952) with `threshold := maxReworkBlockStaleness`. Update the function's doc comment (lines 908-920) to remove the now-inaccurate "reused directly rather than inventing a second definition of stale" claim and replace it with a pointer to ADR-001.
- Files: `server/services/backlog_service_triage.go`

#### Story 1.1.2: Recalibrate the general Review Queue "Stale" badge threshold
**As a** user of the mobile/desktop Review Queue, **I want** the "Stale" badge to only flag sessions that are actually likely stuck, **so that** the badge remains a useful signal instead of firing on 37 of 41 items.

**Acceptance Criteria**:
- `DefaultReviewQueuePollerConfig().StalenessThreshold` changes from `2 * time.Minute` to `5 * time.Minute`.
  - *Given* a session instance with `GetTimeSinceLastMeaningfulOutput()` returning `3*time.Minute`, *When* `determineAttentionReason` runs, *Then* it does NOT set `ReasonStale` (3 min < 5 min).
  - *Given* the same instance with `6*time.Minute` since last output, *When* the same check runs, *Then* it DOES set `ReasonStale` with `PriorityLow` (unchanged existing behavior otherwise).
**Files**: `session/review_queue_poller.go`

##### Task 1.1.2a: Change `StalenessThreshold` default (~2 min)
- Change line 49's `StalenessThreshold: 2 * time.Minute` to `5 * time.Minute`. Update the existing inline comment ("reduced from 5min") to document the reversal and cite ADR-001 — do not leave a comment that contradicts the code.
- Files: `session/review_queue_poller.go`

##### Task 1.1.2b: Update/add unit test asserting the new default (~3 min)
- Find and update the existing test asserting `DefaultReviewQueuePollerConfig().StalenessThreshold == 2*time.Minute` (search `session/review_queue_poller_test.go` and `session/review_queue_determiner_test.go`) to assert `5*time.Minute`. Add a new table-driven case to the determiner's staleness test verifying the 3-min/6-min boundary from Story 1.1.2's Given-When-Then.
- Files: `session/review_queue_poller_test.go`, `session/review_queue_determiner_test.go`

---

## Phase 2: Durable Stuck-State Wiring

### Epic 1.2: Register the new `StuckReason` value
**Goal**: Add `StuckReasonReworkBlockedStale` through every existing registration point the other 11 reasons already use, per the established pattern.

#### Story 1.2.1: Domain-level enum registration
**As a** developer extending the stuck-reason system, **I want** the new reason registered in the domain layer exactly like the existing 11, **so that** `IsValid()`, `AllStuckReasons`, and every consumer that iterates the full set picks it up automatically.

**Acceptance Criteria**:
- `domain.StuckReasonReworkBlockedStale = "rework_blocked_stale"` is added to `session/domain/backlog.go`, included in `AllStuckReasons`, and accepted by `IsValid()`.
  - *Given* `r := domain.StuckReasonReworkBlockedStale`, *When* `r.IsValid()` is called, *Then* it returns `true`.
  - *Given* `domain.AllStuckReasons`, *When* the slice is iterated, *Then* it contains exactly 12 entries including this new one.
**Files**: `session/domain/backlog.go`

##### Task 1.2.1a: Add the constant + doc comment (~3 min)
- Add `StuckReasonReworkBlockedStale StuckReason = "rework_blocked_stale"` after `StuckReasonPRPendingNoPR` (line ~99), with a doc comment matching the style of the other 11 (what triggers it, which detector sets it, cross-reference to `notifyIfActiveWorkSessionStale` and `AutoReopenAfterFailedReview`).
- Files: `session/domain/backlog.go`

##### Task 1.2.1b: Register in `AllStuckReasons` and `IsValid()` (~2 min)
- Append to the `AllStuckReasons` slice (line ~103-115) and the `IsValid()` switch (line ~118-127).
- Files: `session/domain/backlog.go`

##### Task 1.2.1c: Add/update domain-level unit test (~3 min)
- Add `TestStuckReasonReworkBlockedStale_should_beValid_When_Checked` (or extend an existing table-driven `IsValid` test) asserting the new constant is valid and `AllStuckReasons` has 12 entries.
- Files: `session/domain/backlog_test.go` (create if it doesn't already exist; check first)

### Epic 1.3: Proto + RPC-layer registration
**Goal**: Extend the proto enum and Go proto-conversion switches so the new reason round-trips through ConnectRPC exactly like the existing 11.

#### Story 1.3.1: Proto enum value
**As a** frontend consuming `StuckBacklogItem.reason`, **I want** the new reason represented in the generated TypeScript types, **so that** the compile-time-exhaustive `Record<StuckReason, T>` maps catch a missing entry instead of silently rendering blank.

**Acceptance Criteria**:
- `STUCK_REASON_REWORK_BLOCKED_STALE = 12;` is added to `proto/session/v1/backlog.proto`'s `StuckReason` enum, and `make proto-gen` regenerates both Go and TS bindings without other diffs.
  - *Given* the regenerated `web-app/src/gen/session/v1/backlog_pb.ts`, *When* it's inspected, *Then* `StuckReason.REWORK_BLOCKED_STALE === 12` is exported.
**Files**: `proto/session/v1/backlog.proto`

##### Task 1.3.1a: Add the enum value (~2 min)
- Add `STUCK_REASON_REWORK_BLOCKED_STALE = 12;` after line 1002, with a comment mirroring the style of the other entries (cross-reference `session/domain/backlog.go`'s doc comment).
- Files: `proto/session/v1/backlog.proto`

##### Task 1.3.1b: Regenerate bindings (~2 min)
- Run `make proto-gen`. Verify only the expected new-value diff appears in `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` — no unrelated regeneration drift.
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

#### Story 1.3.2: Go proto-conversion switch arms
**As a** backend RPC handler, **I want** `toProtoStuckReason`/`fromProtoStuckReason` to round-trip the new reason, **so that** `ListStuckBacklogItems` and `SnoozeStuckItem` work correctly for items stuck on this reason.

**Acceptance Criteria**:
- Both switch functions in `server/services/backlog_service_stuck.go` gain a case for the new value/constant pair.
  - *Given* `domain.StuckReasonReworkBlockedStale`, *When* `toProtoStuckReason` is called, *Then* it returns `sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE`.
  - *Given* `sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE`, *When* `fromProtoStuckReason` is called, *Then* it returns `domain.StuckReasonReworkBlockedStale`.
**Files**: `server/services/backlog_service_stuck.go`

##### Task 1.3.2a: Add both switch arms (~3 min)
- Add the case to `toProtoStuckReason` (line ~28-55) and `fromProtoStuckReason` (line ~61-88).
- Files: `server/services/backlog_service_stuck.go`

##### Task 1.3.2b: Add round-trip unit test (~3 min)
- Add a table-driven case (or extend an existing one) to whatever test file covers `toProtoStuckReason`/`fromProtoStuckReason` today (check `server/services/backlog_service_stuck_test.go`) asserting the round-trip for the new value.
- Files: `server/services/backlog_service_stuck_test.go`

### Epic 2.1: Detection and resolution wiring
**Goal**: Make `notifyIfActiveWorkSessionStale` durably mark the item stuck (not just notify), and add the resolve-only pass that clears the row when the session recovers without the item's status changing.

#### Story 2.1.1: Mark the item stuck when the gate fires
**As a** user relying on the stuck-items view, **I want** a rework-blocked-by-staleness event to leave a durable, queryable trace, **so that** I can find it later even if I missed the toast.

**Acceptance Criteria**:
- `notifyIfActiveWorkSessionStale` calls `MarkStuck(ctx, itemID, domain.StuckReasonReworkBlockedStale, session.BacklogStatusReview, stuckContext)` before/alongside publishing the existing notification, and skips gracefully (matching the existing `applied == false` handling pattern used elsewhere in this file) if the status precondition no longer holds.
  - *Given* a `review`-status item whose active work session has been idle 20 minutes (`live=true`), *When* `notifyIfActiveWorkSessionStale` runs, *Then* a `BacklogStuckState` row is created (or its `LastCheckedAt` updated if already open) with `Reason=StuckReasonReworkBlockedStale`, `ItemStatus=review`.
  - *Given* the item's status has changed to `in_progress` between the read and this call (e.g. a human manually reopened it in the interim), *When* `MarkStuck` is called with `expectedStatus=review`, *Then* it returns `applied=false` and the function logs+skips rather than erroring.
- `s.eventBus.Publish(...)`'s existing notification call is preserved unchanged (no regression to the current toast behavior) — this is additive, not a replacement.
  - *Given* the same 20-minutes-idle scenario, *When* `notifyIfActiveWorkSessionStale` runs, *Then* the existing `"Rework blocked by a stale-but-alive session"` notification event is still published exactly as before.
- **Explicitly no automated remediation action** is added for this reason (unlike `StuckReasonStaleWork`'s `remediateStaleWorkWithBackoffGate` → `RemediateStaleWorkSession`, which ends and respawns the session automatically after repeated staleness). This is intentional, not an oversight: requirements.md scopes "auto-escalation past a grace period that detaches bookkeeping and proceeds without killing the session" as explicitly out of scope (item C) pending its own future design given real double-work/double-ship risk. This reason is notify + durably mark + resolve-when-recovered only; the human "Reopen for Revision" action (Story 2.2.2) remains the only way to act on it.
**Files**: `server/services/backlog_service_triage.go`

##### Task 2.1.1a: Add the `MarkStuck` call with precondition handling (~5 min)
- Insert a `MarkStuck` call into `notifyIfActiveWorkSessionStale` after the existing threshold check (after line 953's early-return, before the existing `log.WarningLog`/`eventBus.Publish` block). Requires access to `s.storage` (confirm the receiver already has it — `BacklogService` almost certainly does, per every other method in this file). Handle both return values per the established best-effort pattern used throughout this file: on `err != nil` (e.g. a transient DB error), log a warning and continue to the notification publish regardless; on `applied == false` (status precondition mismatch — the item moved off `review` between read and write), log at Info level and continue to the notification publish regardless. In both cases, a storage-layer failure or race must never block or skip the existing notification — that publish call is the one behavior this task must not regress.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.1.1b: Update the function's doc comment (~2 min)
- Update the doc comment (lines 893-933) to describe the new `MarkStuck` behavior, remove the now-stale "This function does NOT change the reopen decision" framing if it needs adjustment (it's still accurate — marking stuck doesn't change the reopen decision — but add a sentence noting it now also durably marks the item, cross-referencing `StuckReasonReworkBlockedStale`'s doc comment in `session/domain/backlog.go`).
- Files: `server/services/backlog_service_triage.go`

##### Task 2.1.1c: Unit test — MarkStuck called on threshold breach (~5 min)
- Extend `server/services/backlog_service_triage_test.go`'s existing `notifyIfActiveWorkSessionStale` test coverage (confirmed present) with: `notifyIfActiveWorkSessionStale_should_callMarkStuck_When_ThresholdExceeded`, `notifyIfActiveWorkSessionStale_should_notCallMarkStuck_When_ThresholdNotExceeded`, `notifyIfActiveWorkSessionStale_should_skipGracefully_When_StatusPreconditionMismatched`. Use an injectable/fake storage matching this file's existing test-double conventions (check the existing test file for the pattern already in use before introducing a new one).
- Files: `server/services/backlog_service_triage_test.go`

##### Task 2.1.1d: Unit test — coexistence with other open stuck reasons on the same item (~3 min)
- Backlog items can already carry multiple simultaneous open `StuckReason` rows (established capability — see `server/services/backlog_service_lifecycle.go:47`'s `[]domain.StuckReason{AbandonedReview, StaleWork}` precedent and `FindOpenStuckStates`' multi-row model). Add `notifyIfActiveWorkSessionStale_should_addSecondOpenReason_When_ItemAlreadyHasReworkCapRowOpen` (or similar) confirming `MarkStuck` for `StuckReasonReworkBlockedStale` does not clobber or conflict with a pre-existing open row for a different reason (e.g. `StuckReasonReworkCap`) on the same item — both rows should coexist. This is a coexistence smoke test, not new production logic (the multi-row model already supports this generically) — its purpose is to catch a regression if a future change accidentally assumes one-reason-per-item.
- Files: `server/services/backlog_service_triage_test.go`

#### Story 2.1.2: Resolve the stuck state when the session recovers
**As a** user, **I want** the stuck-items list to stop showing an item once its blocking session is producing output again, **so that** the list stays trustworthy and doesn't accumulate stale-but-resolved entries.

**Acceptance Criteria**:
- A new `ReworkBlockStaleResolver` interface (`session/backlog_lifecycle.go`), implemented by `BacklogService` (which already has `sessionStopper` and `storage` wired), is injected into `BacklogLifecycleListener` via `SetReworkBlockStaleResolver`, mirroring the existing `StaleWorkRemediator`/`SetStaleWorkRemediator` precedent exactly.
- A new `reconcileReworkBlockedStaleResolution` orchestration function in `session/backlog_lifecycle.go`, invoked from the existing periodic reconcile tick, finds open `StuckReasonReworkBlockedStale` rows via `FindOpenStuckStates` and calls `ReworkBlockStaleResolver.ResolveReworkBlockedStaleIfRecovered(ctx, itemID)` for each — it contains no liveness-checking logic itself, only the loop and delegation.
- `BacklogService.ResolveReworkBlockedStaleIfRecovered` (the interface implementation, in `server/services`) re-checks the item's active work session's current staleness via `s.sessionStopper.TimeSinceLastMeaningfulOutput`, and calls `s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonReworkBlockedStale)` if the session is no longer stale (recovered) OR no longer has an active work session OR the item has left `review` status (belt-and-suspenders alongside `selfHealStuck`, matching `reconcileStaleWorkSessions`' own explicit belt-and-suspenders comment for the identical situation). No-op (nil error) otherwise.
  - *Given* an open `StuckReasonReworkBlockedStale` row for item X, and X's active work session now shows `idle=2*time.Minute, live=true` (recovered), *When* the reconcile tick runs, *Then* `ResolveReworkBlockedStaleIfRecovered` is called and the row is resolved via `s.storage.ResolveStuck`.
  - *Given* the same open row, but X's session is still idle 25 minutes, *When* the tick runs, *Then* the row remains open (no `ResolveStuck` call).
- **No debounce/hysteresis at the threshold boundary is added**, and none is required — `reconcileStaleWorkSessions` has no debounce either and it hasn't been a reported problem in production; an item flickering mark/resolve right at the 15-minute edge if output arrives in a trickle is accepted as a known, low-impact edge case rather than something this plan needs to engineer around.
**Files**: `session/backlog_lifecycle.go`, `server/services/backlog_service_triage.go`, `server/dependencies.go`

##### Task 2.1.2a: Define `ReworkBlockStaleResolver` interface + guarded field + setter (~4 min)
- In `session/backlog_lifecycle.go`, add the interface (mirroring `StaleWorkRemediator`, lines 70-77) and a guarded field + `SetReworkBlockStaleResolver` method on `BacklogLifecycleListener` (mirroring `staleWorkRemediatorMu`/`staleWorkRemediator`/`SetStaleWorkRemediator` exactly, including the nil-safe getter pattern the other three `Set*` fields use).
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.2b: Implement `ResolveReworkBlockedStaleIfRecovered` on `BacklogService` (~5 min)
- Add the method to `server/services/backlog_service_triage.go` (co-located with `notifyIfActiveWorkSessionStale`, which it mirrors in reverse). Uses `s.storage.ListItemSessions`/`hasActiveWorkSession`, `s.sessionStopper.TimeSinceLastMeaningfulOutput`, and `s.storage.ResolveStuck` — all already available on the existing receiver, no new fields needed on `BacklogService` itself. On a `ResolveStuck` error, log a warning and return nil (best-effort — matching `resolveStuckLogged`'s established style of never surfacing a resolve failure as a hard error to the reconcile tick, so one item's storage hiccup can't abort the tick for every other open row).
- Files: `server/services/backlog_service_triage.go`

##### Task 2.1.2c: Add `reconcileReworkBlockedStaleResolution` orchestration + wire into the tick (~4 min)
- Add the function to `session/backlog_lifecycle.go`, modeled on `reconcileStaleWorkSessions`' resolve half (lines 2000-2021) for the `FindOpenStuckStates` → filter-by-reason → delegate shape. Add the call alongside the existing `reconcileStaleWorkSessions`/`reconcileBouncingItems`/etc. calls in the function that orchestrates the periodic tick (search for the `reconcileStaleWorkSessions(ctx` call site to find it).
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.2d: Wire `SetReworkBlockStaleResolver(backlogSvc)` in `server/dependencies.go` (~2 min)
- Add the call alongside the existing `backlogLifecycleListener.SetAutoReopener(backlogSvc)` / `SetPRFixSpawner(backlogSvc)` / `SetStaleWorkRemediator(backlogSvc)` calls (`server/dependencies.go:963-986`).
- Files: `server/dependencies.go`

##### Task 2.1.2e: Unit tests — resolver + orchestration (~5 min)
- `BacklogService.ResolveReworkBlockedStaleIfRecovered`: `_should_resolveStuckRow_When_SessionRecovered`, `_should_leaveRowOpen_When_StillStale`, `_should_beNoOp_When_NoActiveWorkSession` (item D of the belt-and-suspenders check). Add to `server/services/backlog_service_triage_test.go`.
- `reconcileReworkBlockedStaleResolution`: `_should_delegateToResolver_When_OpenRowsExist`, `_should_beNoOp_When_NoOpenRows`, following `session/backlog_lifecycle_test.go`'s existing test-double conventions for the `StaleWorkRemediator`-style interfaces.
- Files: `server/services/backlog_service_triage_test.go`, `session/backlog_lifecycle_test.go`

### Epic 2.2: UI surfacing
**Goal**: Render the new reason using the existing generic `backlog-stuck` components — no new component, only new map entries.

#### Story 2.2.1: Label, icon, and chip class for the new reason
**As a** user viewing the stuck-items list, **I want** the new reason to render with a clear label and distinct visual treatment, **so that** I can tell it apart from `StuckReasonStaleWork` at a glance (see pitfalls.md #4 — deliberately kept visually distinguishable, not identical).

**Acceptance Criteria**:
- `stuckReason.ts`'s three `Record<StuckReason, T>` maps each gain a `[StuckReason.REWORK_BLOCKED_STALE]` entry; TypeScript compiles without a missing-key error (this is the existing compile-time enforcement mechanism, not a new one).
  - *Given* `StuckReason.REWORK_BLOCKED_STALE`, *When* `getStuckReasonLabel(reason)` is called, *Then* it returns a label distinct from `"Stale work session"` (e.g. `"Rework blocked — session stalled"`) — exact copy to be finalized during implementation with the existing label style in mind (short, plain-language, matching the other 11).
**Files**: `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 2.2.1a: Add label/icon/class map entries (~3 min)
- Add entries to `STUCK_REASON_LABELS`, `STUCK_REASON_ICONS`, `STUCK_REASON_CLASS` (lines 15-60). Before finalizing copy, re-read all 11 existing `STUCK_REASON_LABELS` entries for length/tone consistency (short, plain-language, action-oriented where relevant) rather than picking wording in isolation — the two existing toast messages for this exact bug already demonstrate the level of care expected. Icon: pick a glyph distinct from `STALE_WORK`'s 🟠 (e.g. 🟥 or ⏸️, matching the "actively blocking" urgency), constrained only by "must not duplicate an existing icon 1:1 with a different meaning."
- Files: `web-app/src/components/backlog-stuck/stuckReason.ts`

##### Task 2.2.1b: Add chip CSS class (~3 min)
- Add `chipReworkBlockedStale` to `stuckReason.css.ts`, following the exact `style([...])` pattern of the 11 existing chip classes (line ~44's `chipStaleWork` is the closest template given the shared underlying concept).
- Files: `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 2.2.1c: Unit test — new map entries (~3 min)
- Extend `stuckReason.test.ts` (confirmed existing) with assertions that `getStuckReasonLabel`/`getStuckReasonIcon`/`getStuckReasonClass` return non-fallback values for the new reason (i.e., don't silently fall through to the `UNSPECIFIED` default). Note: an existing exhaustiveness test, `getStuckReasonLabel_should_returnTextLabelForEveryReason_When_MappedExhaustively` (per `docs/registry/features/frontend/backlog-stuck-items.json`'s `testIds`), likely already iterates all `StuckReason` values and would automatically cover the new one by construction — confirm this before assuming a new test is strictly necessary; a targeted new test is still worth adding for the specific icon/class distinctness assertion (vs. `STALE_WORK`), which the generic exhaustiveness test won't check.
- Files: `web-app/src/components/backlog-stuck/stuckReason.test.ts`

#### Story 2.2.2: Confirm the existing "Reopen for Revision" action remains the resolution path
**As a** user who finds a `rework_blocked_stale` item in the stuck-items list, **I want** a clear path to act on it, **so that** discovering the durable state isn't a dead end.

**Acceptance Criteria**:
- No new action/button is built (per architecture.md/build-vs-buy.md's explicit recommendation) — clicking through to the item detail page surfaces the existing `GateVerdictBox` "Reopen for Revision" button (already present for any `review`-status item with a FAIL/PARTIAL/UNVERIFIABLE verdict, which this item necessarily has by construction).
  - *Given* an item with an open `StuckReasonReworkBlockedStale` row shown in the stuck-items list, *When* the user clicks through to its detail page, *Then* `GateVerdictBox`'s existing "Reopen for Revision" button is visible and functional (no code change needed — this is a verification task, not an implementation task).
**Files**: none (verification only)

##### Task 2.2.2a: Verify the click-through path with a component-level test (~5 min)
- Prefer a fast, deterministic component test over e2e for this: check `StuckItemsSection.test.tsx`/`StuckItemDetail.test.tsx` for an existing pattern asserting navigation/link target per stuck reason, and add a case for `REWORK_BLOCKED_STALE` confirming the card/detail links to the item's detail route. Only reach for a new or extended Playwright e2e spec (per `.claude/rules/e2e-test-conventions.md`) if no component-level navigation assertion pattern exists to extend — record which approach was taken in the task's completion notes so this isn't silently skipped.
- Files: `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` or `StuckItemDetail.test.tsx` (preferred); `tests/e2e/*.spec.ts` only as fallback

### Epic 2.3: Feature registry update
**Goal**: Satisfy `.claude/rules/feature-registry.md` for the modified backend RPC surface and frontend component, per requirements.md's explicit In Scope "Feature registry entries" bullet.

#### Story 2.3.1: Update existing registry entries (no new entries needed)
**As a** maintainer of `docs/registry/`, **I want** the existing entries for the stuck-items RPC and UI to reflect this change, **so that** `make registry-diff` doesn't show unexplained drift and coverage-gaps tracking stays accurate.

**Acceptance Criteria**:
- Both `docs/registry/features/backend/backlog/list-stuck.json` (the existing `ListStuckBacklogItems` RPC entry — modified, not new, since this plan extends an existing enum rather than adding a new RPC) and `docs/registry/features/frontend/backlog-stuck-items.json` (the existing `StuckItemsSection` entry — modified, since this plan adds map entries and tests to already-registered files) have `lastModified` bumped to the implementation date, and any newly-added test names (Task 2.2.1c, 2.2.2a, and the backend tests from Epic 2.1) are appended to their respective `testIds` arrays.
  - *Given* the current `backlog-stuck-items.json` (`testIds` array of 17 entries, `lastModified: "2026-07-20T00:00:00Z"`), *When* this feature's frontend tests are added and `make registry-generate` is run, *Then* the array includes the new test names and `lastModified` reflects the current date.
- `make registry-generate` is run and shows no unexpected drift beyond this feature's own changes (per `make registry-diff`'s dry-run check).
**Files**: `docs/registry/features/backend/backlog/list-stuck.json`, `docs/registry/features/frontend/backlog-stuck-items.json`

##### Task 2.3.1a: Update `lastModified` and `testIds` on both existing entries (~3 min)
- No new registry JSON files are created — both the backend RPC and frontend component already have entries (confirmed on disk: `docs/registry/features/backend/backlog/list-stuck.json`, `docs/registry/features/frontend/backlog-stuck-items.json`). Update `lastModified` on each and append the new test names from Epics 2.1/2.2 to `backlog-stuck-items.json`'s `testIds` (the backend entry's existing `testIds` array should similarly gain any new Go test names from Epic 2.1 if that file tracks them the same way — confirm its schema matches before editing).
- Files: `docs/registry/features/backend/backlog/list-stuck.json`, `docs/registry/features/frontend/backlog-stuck-items.json`

##### Task 2.3.1b: Run `make registry-generate` and verify no unexpected drift (~2 min)
- Run `make registry-generate`, then `make registry-diff` as a dry-run sanity check before committing. Confirm the aggregated `docs/registry/coverage-gaps.json` count does not grow.
- Files: none (generated aggregate files only)

---

## Phase 3: Task-Protocol Cadence Fix (item D)

### Epic 3.1: Give rule 8 a concrete polling cadence
**Goal**: Reduce false "looks idle/stale" appearance from a well-behaved, quietly-waiting agent, without the recommended cadence itself risking tripping the new `maxReworkBlockStaleness` (15 min) threshold.

#### Story 3.1.1: Add an explicit wait interval to rule 8
**As an** agent following the task protocol after requesting review, **I want** a concrete cadence for re-checking status, **so that** my quiet waiting isn't indistinguishable from being stuck.

**Acceptance Criteria**:
- `taskProtocolBlock` rule 8's text is amended to suggest a concrete interval (e.g. "wait about 2-3 minutes") that is comfortably below `maxReworkBlockStaleness` (15 min) — at least 5x headroom, per pitfalls.md #5's recommendation.
  - *Given* the updated `taskProtocolBlock` string, *When* it's rendered into a session's initial prompt, *Then* it contains a concrete numeric interval, not just "wait."
**Files**: `session/backlog_context.go`

##### Task 3.1.1a: Edit rule 8's text (~3 min)
- Amend line 114's rule 8 string to add a concrete interval, e.g.: `"...After `/backlog/review`, stay in this session — do not exit. Wait roughly 2-3 minutes, then run `/backlog/status` again to check for a verdict...."` Keep the rest of the rule's logic (PASS/FAIL branching) unchanged.
- Files: `session/backlog_context.go`

##### Task 3.1.1b: Update/add prompt-text test if one exists (~2 min)
- Check for an existing test asserting `taskProtocolBlock`'s literal content (search for `taskProtocolBlock` in `session/backlog_context_test.go`); if one snapshot-asserts the full string, update it. If none exists, do not add one purely for a text-content assertion (low value — this is prose, not logic).
- Files: `session/backlog_context_test.go` (only if an existing test needs updating)

---

## Decision Reference

See `project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md` for the full rationale behind the two threshold values chosen in Phase 1.
