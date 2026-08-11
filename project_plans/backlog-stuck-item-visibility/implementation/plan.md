# Implementation Plan: backlog-stuck-item-visibility

**Feature**: Durable, queryable, restart-surviving visibility of backlog items that have stopped progressing toward merge — with an explicit reason per stuck item and a browsable UI view — replacing today's ephemeral, easy-to-miss toast-only signals.
**Date**: 2026-07-14
**Status**: Ready for implementation
**ADRs**: ADR-001 (Durable Stuck-State Storage Model)

---

## System Type

A **reconciliation / observability feature** bridging two Go service packages
(`session` — the `BacklogLifecycleListener` reconciler; and `server/services` — the
`BacklogService` ConnectRPC handler + triage rework-cap logic), a **small additive ent
schema entity** (one new table, ent auto-migration), **one new ConnectRPC read surface**
(unary `ListStuckBacklogItems` + a small `SnoozeStuckItem` on the existing all-unary
`BacklogService`), and a **frontend view** (a new section on the existing `/unfinished`
page reusing its list/detail/badge/snooze patterns). No new external dependency, no new
scheduler — the existing 60s reconcile ticker is the only integration point.

---

## Domain Glossary
| Term | Definition | Notes |
|------|-----------|-------|
| **StuckReason** | Validated string-backed enum (house style, `IsValid()`-guarded — a validated primitive, not a truly unrepresentable sum type) of the classes an item can be stuck for: `pr_ready_unmerged`, `rework_cap`, `abandoned_review`, `stale_work`, `bouncing`, `push_failed`. | Go string-backed type + matching proto `StuckReason` enum. Validated at the boundary; `MarkStuck` only ever receives compile-time `StuckReason*` constants so no unvalidated string reaches the DB. |
| **BacklogStuckState** | Durable ent entity: **exactly one row per `(item_id, reason)`** stuck condition (resolve-in-place, not episode history); the notify-once store AND the UI source of truth. | New table `backlog_stuck_states`; mirrors `BacklogStatusEvent` shape. Plain 2-column unique index `(item_id, reason)`. |
| **first_detected_at** | Persisted timestamp the condition was first observed by the reconciler. Source of the "stuck for N" duration shown in the UI. On **resolve-in-place reopen** (a resolved row re-detected as stuck), this is reset to the new onset time. | Survives restarts — replaces process-uptime-relative duration. Distinct from `status_events` history, which records status *transitions*, not stuck-condition onset. |
| **last_checked_at** | Timestamp of the most recent reconcile tick that re-confirmed the condition still holds. | Drives the UX "checked recently, trust this" signal (features.md unstated-need #6). |
| **notified_at** | Timestamp the operator notification fired for this stuck instance; `NULL` = detected but not yet notified. Cleared back to `NULL` on reopen so a genuinely-recurred condition can re-notify once. | The durable notify-once dedup key that replaces in-memory `staleWorkNotified` / `stuckReviewNotified`. |
| **resolved_at** | Timestamp the reconciler (or a status-transition call site) observed the condition clear; `NULL` = currently stuck (row is "open"), non-`NULL` = resolved. A resolved row is **reused in place** if the same condition recurs (clear `resolved_at`/`notified_at`, reset `first_detected_at`) — no second row is created. | Set idempotently (only `WHERE resolved_at IS NULL`) at every un-stick call site and by the self-heal sweep, not only on the next tick. |
| **snoozed_until** | Timestamp until which a stuck row is suppressed from the active view and from re-notification. `NULL` = not snoozed. | "Visibility control, not remediation" (features.md unstated-need #1). |
| **context** | Human-readable "why" string persisted alongside the row (e.g. `last verdict: FAIL`, `PR #148 green & mergeable 3d`). | Persists the string `reconcileStuckReviewItems` already builds for the notification, as structured data (features.md #4). |
| **pr_ready_unmerged** | Reason: a `pr_pending` item's PR is judged **merge-ready-for-a-solo-operator** — not draft, no CHANGES_REQUESTED review (`ChangesRequestedCount == 0`), CI green (`CheckConclusion ∈ {"success",""}`, i.e. `!CIFailing`), mergeable/no-conflicts (`Mergeable == "MERGEABLE"`), and not closed/merged — held past the threshold and still unmerged. **Intentionally NOT the same predicate as `github.DerivePRPriority(info) == github.PRPriorityReady`.** `PRPriorityReady` additionally requires `ApprovedCount > 0` (`github/priority.go:50`), which is *impossible* on this single-user repo — Tyler cannot self-approve his own PR, so `ApprovedCount` is always 0 and the canonical helper resolves to `PRPriorityPending` forever. Keying on it would make this detector permanently blind and structurally exclude PR #148, the feature's own motivating case. So `pr_ready_unmerged` applies the **same blocking-exclusions** `DerivePRPriority` uses (draft, changes-requested, CI-failure all still disqualify — so a genuine "changes requested" block is NOT missed) but **drops the approval gate**. | Root cause #1. Reuses the existing `GetPRStatus`/`PRInfo` result — no new poller, no new GitHub call. Readiness is computed by the co-located pure predicate `prReadyToMergeSolo(info)` (Story 2.1.0), NOT re-derived ad-hoc at the call site — see ADR-001 "Single-user readiness" note so this decision isn't silently re-diverged back to `PRPriorityReady`. |
| **rework_cap** | Reason: the auto-rework loop hit `maxAutoReworkIterations` (=3) and parked the item for manual action. | Root cause #2. |
| **abandoned_review** | Reason: a `review`-status item has a review verdict on record but nothing active in flight. | Root cause (existing `FindStuckReviewItems`). |
| **stale_work** | Reason: an `in_progress` item's active work session reported no progress for > `maxWorkSessionStaleness` (2h). | Existing `reconcileStaleWorkSessions`. |
| **bouncing** | Reason: an item crossed `in_progress ↔ review` ≥ `bounceThreshold` times within `bounceLookback` with no PASS verdict. | Root cause #4. Non-converging cycle. |
| **push_failed** | Reason: `pushAndCreatePR` failed (push rejected / `gh pr create` errored) leaving a post-review item with **no `pr_number`** — invisible to `FindPRPendingItems`' `PrNumberGT(0)` filter. Event-shaped: written at the failure site, not poll-derived. | Root cause: requirements In-Scope class "push/PR-creation failure". Already fires an ephemeral `NOTIFICATION_TYPE_ERROR` via `stayInReviewAndNotify`; this durable row supersedes that toast for restart-surviving visibility (Story 2.1.6). |
| **pr_status_unknown** | Derived **UI-only** state: the reconciler could not fetch PR status (GitHub API error / rate limit). NOT a stored `StuckReason`. Rendered when `last_checked_at` is older than the **staleness threshold = 5 min** (~5 missed 60s ticks). | Rendered as a distinct chip with a stale "last checked" timestamp so a failed poll isn't read as "healthy" (ux.md §4, features.md edge case #6). Threshold justified: on `GetPRStatus` error the reconciler `continue`s (`backlog_lifecycle.go:841-843`) so `last_checked_at` freezes; 5 min = enough missed ticks to distinguish a transient blip from a sustained fetch outage. |
| **ReconcileStuck** | The existing 60s-tick pipeline (`session/backlog_lifecycle.go:519`) where new detectors hook in. | The sole scheduled entry point; no second ticker. |
| **MarkStuck** | Repository **resolve-in-place upsert**: a single `INSERT ... ON CONFLICT(item_id, reason)` statement that opens a new row, refreshes an already-open row (`last_checked_at` only), or reopens a resolved row (clear `resolved_at`/`notified_at`, reset `first_detected_at`) — atomically, no read-then-write for row dedup. The item-status precondition is applied best-effort before the write; a row written after a racing transition is corrected by the self-heal sweep next tick (Story 2.1.5d). | `OnConflictColumns(item_id, reason)` on the plain 2-column unique index. |
| **ResolveStuck** | Repository **atomic idempotent** conditional update: `UPDATE ... SET resolved_at=now WHERE item_id=? AND reason=? AND resolved_at IS NULL`, returning affected-row-count. No-op (not error) if already resolved. | Wired into every un-stick call site AND the self-heal sweep. |
| **ListStuckBacklogItems** | New unary `BacklogService` RPC returning open (unresolved, un-snoozed) stuck rows joined to item + PR context. | `+api: backlog:list-stuck`. |
| **SnoozeStuckItem** | New unary `BacklogService` RPC that sets `snoozed_until` on a stuck row. | `+api: backlog:snooze-stuck`. Reuses the `/unfinished` snooze shape. |
| **backfill (seeding)** | One-time insert of currently-stuck items into the table at deploy, with `notified_at` pre-set, to suppress the first-tick notification storm. | Runs once before the ticker starts (pitfalls.md §3). |

---

## Pattern Decisions
| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Durable stuck-state storage | New entity via **Repository / Data Mapper**, **resolve-in-place upsert** by `(item_id, reason)` — exactly one row per pair, reopened not duplicated | PoEAA Repository + Data Mapper; in-repo `BacklogStatusEvent` | (A) fields on `BacklogItem`; (C) fully derived, no schema; (D) append-only episode-history rows | A can't hold >1 concurrent reason and races status writes on the hot item row; C must still persist a notify marker AND re-polls GitHub on every read; D breaks the notify-once unique-index invariant on SQLite (NULLs distinct) — episode history explicitly dropped as unneeded scope. See ADR-001. |
| Stuck-reason detection | **Transaction Script** — concrete reconciler methods on `BacklogLifecycleListener` | PoEAA Transaction Script; existing `reconcileStaleWorkSessions`/`reconcileStuckReviewItems` | GoF **Strategy** via a `StuckItemDetector` interface | interface-pollution-checklist smell #1 (speculative interface, one impl) + #4 (forwarding wrapper); existing `ReconcileStuck` calls concrete methods directly, no interface. |
| `StuckReason` modeling | **Validated string-backed enum** — Go const type (`IsValid()`) + proto enum, matching the house `BacklogStatus`/`ReviewOutcome` style | type-driven-design (validate-at-boundary); Parse-Don't-Validate | Raw un-validated `string` field | Validated at the boundary and forces an exhaustive `switch` in handler + UI reason-chip map. (Not a truly-unrepresentable sum type — `StuckReason("banana")` compiles but fails `IsValid()`; only compile-time constants ever reach `MarkStuck`.) |
| Notify-once dedup | **Durable nullable marker** (`notified_at`) replacing in-memory map | ADR-001 | `map[string]bool` on the listener (status quo) | Resets on every restart = root cause #3, the core bug. |
| Green-PR-unmerged signal | **Extra `else`-branch on the already-fetched result, keyed on a co-located pure predicate `prReadyToMergeSolo(info)`** = `DerivePRPriority`'s blocking-exclusions (not draft, no CHANGES_REQUESTED, CI not failing) + mergeable, but **without** the `ApprovedCount > 0` gate — no new poller, one documented readiness definition | pitfalls.md §4; pre-mortem F1; `github/priority.go:34,39,44,50` | (a) New `prMergeReadyPoller` / second GitHub client; (b) reuse `github.PRPriorityReady` verbatim | (a) doubles GitHub API volume, trips secondary rate limits; (b) **`PRPriorityReady` requires `ApprovedCount > 0` (`github/priority.go:50`), which is unreachable on a single-user repo** — Tyler cannot self-approve, so it resolves to `PRPriorityPending` forever and PR #148 (the motivating case) is never flagged (pre-mortem F1). The solo predicate keeps every real block `DerivePRPriority` enforces (a changes-requested PR is still excluded — no false-positive) and only removes the impossible approval requirement. Predicate lives in one place + a unit test that an **unapproved** green PR flags stuck, so it can't silently re-diverge back to the canonical helper. |
| Concurrency guard on stuck writes | **Atomic conditional writes + self-heal sweep**: `ResolveStuck` is a single `UPDATE ... WHERE resolved_at IS NULL` (atomic, idempotent, affected-row-count); `MarkStuck` is a single `INSERT ... ON CONFLICT` (atomic row dedup) with a best-effort `ExpectedStatus` pre-filter; the reconcile self-heal pass (Story 2.1.5d) resolves any open row whose item status no longer matches its reason | pitfalls.md §1 (atomic `UPDATE...WHERE`, not check-then-act); existing house pattern (`ent_repository_backlog.go:516`) | Read-then-unconditional-write (TOCTOU) | A read→compare-in-Go→write leaves a window where a legitimate transition lands mid-write and a stale "stuck" row commits with **no un-stick event to clear it** → phantom false-positive leak. The atomic writes plus the self-heal sweep make stale writes self-correct within one tick (pitfalls.md §1 anticipated this). |
| Read surface | **Unary request/response** `ListStuckBacklogItems` (Service Layer / Remote Facade) | PoEAA Service Layer; existing all-unary `BacklogService` | Streaming `WatchStuckBacklogItems` | Unary matches `BacklogService`'s style; polling is fine at single-user / ~10-item scale; avoids the stream reconnect/backoff/phantom-leak pitfalls of `useUnfinishedWork` (pitfalls.md §5). |
| Frontend state | **Polled unary hook** `useStuckBacklogItems` (fetch on mount + interval) | in-repo `useUnfinishedWork`, simplified | Streaming hook w/ `AbortController` + 3s-fixed reconnect | No delta-leak, no backoff, no stale-closure client — a full authoritative list every poll self-heals (pitfalls.md §5). |

---

## Migration Plan
- **Migration file**: none authored by hand. ent auto-migration (`client.Schema.Create(ctx)` at `session/ent_repository.go:86`) applies the new `backlog_stuck_states` table additively on process start. Regenerate the ent client with the pinned command **`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`** (via `go generate ./session/ent/...`) — the `sql/upsert` flag is required because `MarkStuck` uses `OnConflictColumns`. A **plain 2-column unique index on `(item_id, reason)`** (NOT a 3-column or partial index — resolve-in-place keeps exactly one row per pair) MUST be defined in the schema *before* first generation so `OnConflictColumns` has a valid target. Note: this is only viable because episode-history was dropped (see ADR-001) — an append-only/multi-episode design could not be a plain-unique `OnConflict` target on SQLite (NULLs distinct).
- **Reversibility**: additive-only (new table, no column changes to `BacklogItem`). A revert of the code leaves an unused empty table — harmless. No destructive DDL.
- **Zero-downtime strategy**: single-user self-hosted tool, restart-based deploy; auto-migration runs before request serving. No online-migration concern.
- **Rollback procedure**: `git revert` the feature. The orphaned `backlog_stuck_states` table can be left in place (ignored) or manually dropped; existing `c99f6595` in-memory safety nets remain as the fallback signal.
- **Backfill step (one-time, notification-suppressing)**: at startup, *before* the 60s ticker goroutine begins, run a seeding pass that inserts an open `BacklogStuckState` row for every currently-stuck item with `notified_at = now` and `first_detected_at = now`, so the first genuine tick does not re-notify the 6+ pre-existing parked items (pitfalls.md §3). Idempotent: guarded by the `(item_id, reason)` unique constraint. **Seed only DB-derivable reasons** (`rework_cap`, `abandoned_review`, `stale_work`, `bouncing`, `push_failed`) — **exclude `pr_ready_unmerged`**, whose detection needs `IsPRMerged`+`GetPRStatus` per `pr_pending` item. Backfilling it would fire an unbounded GitHub API burst on every one of the 15+ daily boots, ungated by `github.DefaultRateLimiter` (pitfalls.md §4). The first genuine tick handles the green-PR case with its own `notified_at IS NULL` + 30-min gate, rate-limited by the existing poller path — no storm, one-tick delay only.

## Observability Plan
- **Logs**: reuse existing structured `log.InfoLog`/`log.WarningLog`/`log.DebugLog` patterns. Emit one INFO on each new stuck-row open (`item marked stuck reason=X`), one INFO on resolve (`stuck reason=X cleared for item`), and one DEBUG per tick summarizing counts (`N open stuck rows across M items`). Match the existing `[BacklogLifecycle]` log prefix.
- **Silently-dead-detector defense (pre-mortem F5/P3)**: because there are no metrics or alerts on this single-user tool, a detector that panics or nil-derefs every tick would otherwise leave its reason's rows unwritten and present as the reassuring "Nothing stuck" empty state indefinitely — indistinguishable from a genuinely healthy empty state. Two log surfaces make that distinguishable over time:
  1. **WARNING on every per-detector panic-recovery** (Task 2.1.5e): each detector's own `recover()` logs at WARNING with the detector name + recovered value, so a repeatedly-panicking detector produces a steady WARNING stream rather than silence.
  2. **Per-tick "detectors completed" self-check line**: after the tick's detectors run, emit one summary line naming which detectors ran successfully vs. panicked this tick, e.g. `[BacklogLifecycle] stuck sweep tick: detectors ok=[pr_ready,rework_cap,abandoned_review,stale_work,bouncing,push_failed] panicked=[] openRows=N`. A detector that stops appearing in `ok=[...]` (or shows up in `panicked=[...]`) is then visible in a log grep, so "detector ran and found nothing" is distinguishable from "detector never ran." This is the log-level analog of the `pr_status_unknown` staleness signal applied to the whole sweep. (The UI-side "last successful full sweep at" timestamp from pre-mortem P3(c) remains an optional future enhancement, not required for this ship.)
- **Metrics**: none new — single-user internal tool, no metrics infra (per requirements NFRs). The UI view's count *is* the metric surface.
- **Alerts**: none — no oncall rotation. The in-app nav badge + notification is the only alert path (reusing `EventBusNotifier`/`Notifier`).
- **Multi-reason fanout (deliberate decision, pitfalls §3)**: an item stuck for 2+ reasons at once (e.g. `bouncing` AND `stale_work`) produces one row per `(item_id, reason)` → one notification per reason and appears in 2+ UI reason-groups. This is intentional: each reason is a distinct actionable condition and the reason-group is the actionable unit. Accepted mild notification-fatigue tradeoff for a self-hosted single user; not deduped to a single per-item notification.

## Risk Control
- **Feature flag**: gate the new detectors behind the listener's existing `enabled atomic.Bool` (already checked at the top of `ReconcileStuck`); no new flag needed. The RPC + UI section render empty if no rows exist, so shipping the backend first is safe.
- **Rollback procedure**: `git revert`; table left orphaned (see Migration Plan). Existing reconcilers/notifications are unchanged in behavior for their notification content — only their *dedup source* moves from memory to DB, so a revert restores the in-memory maps cleanly.
- **Staged rollout**: N/A (single instance). Ship backend (schema + detection + backfill) and verify via the live-data queries before wiring the UI section on.

## Unresolved Questions
None. The three open questions from requirements.md are resolved as explicit decisions:

1. **Stuck-time thresholds per reason** (requirements chose "leave to planning — pick defensible defaults"):
   - `pr_ready_unmerged`: **green-and-mergeable for > 30 minutes**. Justification: the 60s poll means ~30 confirming ticks; 30 min is long enough that "the user is actively merging right now" has passed, short enough that a forgotten PR (like #148, which sat 3 days) surfaces within the same working session. Measured from `first_detected_at` of the green-and-mergeable condition, not from PR creation.
   - `rework_cap`: **threshold 0 (immediate)**. The cap hit is a discrete, definitive "parked for manual action" event (`workCount >= maxAutoReworkIterations`); no elapsed-time gate is meaningful — mark stuck the moment the cap is hit.
   - `abandoned_review`: **> 15 minutes** since the most recent `to_status = "review"` `BacklogStatusEvent`. Justification: gives the 60s reconcile one or more ticks to re-spawn a review gate before flagging, avoiding a false positive on an item that just entered review.
   - `stale_work`: **reuse the existing `maxWorkSessionStaleness = 2h`** — do not introduce a new number; the codebase already made this product decision.
   - `bouncing`: **≥ 3 `in_progress → review` round trips within a 24h lookback with no PASS verdict**. Justification: reuses `maxAutoReworkIterations = 3` as the established "we've tried enough" threshold; the 24h window bounds it to *active* thrashing (df0d5872 bounced over 4 days, so a rolling-24h ≥3 catches it while it's hot; a since-gone-quiet item falls to `abandoned_review`/`stale_work` instead).
   - `push_failed`: **threshold 0 (immediate)**. Event-shaped like `rework_cap` — the push/`gh pr create` failure is a discrete definitive event; mark stuck the moment `pushAndCreatePR` reports failure.
   - `pr_status_unknown` is a **derived UI state, not a stored reason**, shown when the last poll failed. Concrete staleness threshold: **`last_checked_at` older than 5 minutes** (~5 missed 60s ticks) renders the "couldn't check" chip.
2. **Surface the repo's `allow_auto_merge` setting read-only?** **Yes — in the detail expansion only** (not the list glance), fetched best-effort. It directly explains root cause #1's mechanism (the PR isn't auto-merging because `allow_auto_merge: false`). Read-only single line; changing it stays out of scope. Degrades gracefully to "unknown" if the fetch fails. **This is a single per-repo, TTL-cached fetch gated on `github.DefaultRateLimiter` — NOT a per-item or per-detail-expand call** (a repo has one `allow_auto_merge` value; fetching it per stuck item would be unbounded API surface). Sized as one small story (4.1.4).
3. **UI location — extend `/unfinished` vs. new view?** **Extend `/unfinished` with a new "Stuck Backlog Items" section**, but back it with a **new RPC on `BacklogService`** (NOT `UnfinishedWorkService`). Rationale: architecture.md — the RPC domain is backlog (item IDs, verdicts, PR mergeability); `UnfinishedWorkService`'s scanner has no concept of backlog item IDs. ux.md — reuse `/unfinished`'s list/detail/badge/snooze visual patterns regardless of which page hosts it. Extending the page reuses the snooze plumbing and repo-grouping convention without duplicating them.

## Dependency Visualization
Node labels use the real `Epic.Story` numbering used throughout this document
(Phase N → Epic N.1 → Stories N.1.x), so the graph and the story headings below
stay in sync.
```
Phase 1: Durable stuck-state foundation  (Epic 1.1)
  1.1.1 ent schema BacklogStuckState     ──┐
  1.1.2 StuckReason validated enum         │
  1.1.3 repo MarkStuck/ResolveStuck/Find   ├───────────────┐
  1.1.4 backfill seeding at startup      ──┘               │
        │                                                  │
        ▼                                                  ▼
Phase 2: Detection & reconciliation (Epic 2.1)     Phase 3: Read RPC surface (Epic 3.1)
  2.1.0 pure decision fns (unit-tested)              3.1.1 proto: StuckReason enum + 2 RPCs
  2.1.1 pr_ready_unmerged (prReadyToMergeSolo)       3.1.2 ListStuck/Snooze handlers + registry
  2.1.2 rework_cap durable write (triage)                  │
  2.1.3 abandoned_review + stale_work → durable            │
        └─ Task 2.1.3d zombie-session review detect        │
  2.1.4 bouncing detector                                  │
  2.1.5 ResolveStuck + self-heal sweep +                   │
        per-detector panic recovery                        │
        └─ Task 2.1.5d per-reason expected-status sweep    │
  2.1.6 push_failed durable write (event)                  ▼
        └────────────────────────────────────────►  Phase 4: Frontend (Epic 4.1)
                                                      4.1.1 useStuckBacklogItems hook
                                                      4.1.2 reason chips + StuckItem card + detail
                                                      4.1.3 nav badge + section + empty state
                                                      4.1.4 read-only allow_auto_merge context
                                                      4.1.5 e2e spec
                                                           │
                                                           ▼
                                                    Phase 5: Snooze wiring (Epic 5.1)
                                                      5.1.1 snooze button → RPC → hook refresh
```
Phase 1 blocks 2 and 3. Phase 3 blocks 4. Within Phase 4, Story **4.1.4**
(read-only `allow_auto_merge`) depends on 3.1.1 (the RPC must carry the setting
field) **and** 4.1.2 (it renders in the `StuckItemDetail` component from 4.1.2),
so it sequences after both. Phase 5 depends on 3.1.1 (`SnoozeStuckItem` RPC) +
4.1.2/4.1.3 (the card + section it hangs the snooze control on).

**Diagram-vs-prose clarification — Phase 4 does NOT depend on Phase 2's detectors.**
The arrow drawn from Phase 2 (`2.1.6 push_failed`) into Phase 4 in the graph above
overstates the true dependency: it exists only because the two phases were laid out
vertically, not because Phase 4 needs Phase 2's detectors to run. **Phase 4 (the UI
section, hook, badge, and e2e spec) depends only on the Phase 1 storage layer
(`BacklogStuckState` + `MarkStuck`) and the Phase 3 read RPC (`ListStuckBacklogItems`)** —
it renders whatever open rows exist. The frontend e2e tests (Story 4.1.5, per
validation.md) seed stuck rows **directly through the backlog store** (`MarkStuck` on the
`BacklogStuckState` table), **bypassing the detectors entirely**, so Phase 4 can be
built, run, and tested end-to-end against directly-seeded rows while Phase 2's detectors
(2.1.1–2.1.6) are still incomplete or in flight. The real hard edges are therefore
**1 → 3 → 4** (storage → RPC → UI); Phase 2 → Phase 4 is a *soft* ordering (both consume
the same table independently), not a blocking prerequisite. Practically: Phase 2 and
Phase 4 can proceed in parallel once Phases 1 and 3 land.

---

## Phase 1: Durable Stuck-State Foundation

### Epic 1.1: Persistent stuck-state storage
**Goal**: Add the `BacklogStuckState` ent entity and the repository methods that open, refresh, resolve, and query stuck rows, with the same precondition safety `TransitionBacklogItemStatus` uses — so stuck bookkeeping survives restarts (fixes root cause #3).

#### Story 1.1.1: BacklogStuckState ent schema
**As a** maintainer, **I want** a durable table recording each open stuck condition per item, **so that** "which items are stuck, why, and since when" survives the 15+ daily restarts.
**Acceptance Criteria**:
- A new `backlog_stuck_states` table exists with a unique constraint on `(item_id, reason)` and cascade-delete from `BacklogItem`.
  - *Given* the schema file `session/ent/schema/backlog_stuck_state.go` defines fields `item_id`, `reason`, `first_detected_at`, `last_checked_at`, `notified_at` (nillable), `resolved_at` (nillable), `snoozed_until` (nillable), `context`, with `index.Fields("item_id","reason").Unique()` and `edge.From("item", BacklogItem.Type).Ref("stuck_states")` plus `OnDelete(Cascade)`, *When* `go generate ./session/ent/...` runs with `--feature sql/upsert` and `go build ./...` follows, *Then* the generated `backlogstuckstate` package compiles and `OnConflictColumns(...)` methods exist.
- Deleting a backlog item cascade-deletes its stuck rows.
  - *Given* item `f9fcef32-c27e-434d-b23f-c873c18afa92` has one open `pr_ready_unmerged` row, *When* the item is deleted via `DeleteBacklogItem`, *Then* no `backlog_stuck_states` row referencing that `item_id` remains (mirrors `status_events` cascade).
**Files**: `session/ent/schema/backlog_stuck_state.go` (new), `session/ent/schema/backlog_item.go` (add `edge.To("stuck_states", ...)` with cascade annotation), `session/ent/generate.go` (unchanged — run it).

##### Task 1.1.1a: Author the schema file (~4 min)
- Copy `backlog_status_event.go` structure; set fields per AC; add `StorageKey`/**plain 2-column unique index `index.Fields("item_id","reason").Unique()`** (NOT 3-column with `resolved_at`, NOT partial — resolve-in-place keeps exactly one row per pair so a plain unique index is both the `OnConflict` target and the correctness guard); add `reason` as `field.String("reason")` (validated in Go by `StuckReason.IsValid()`, not an ent enum, to match the string-status house style). Add a schema comment documenting the resolve-in-place model (row reopened by clearing `resolved_at`/`notified_at`, never duplicated) and that episode history was intentionally dropped (ADR-001).
- Files: `session/ent/schema/backlog_stuck_state.go`

##### Task 1.1.1b: Add the parent edge on BacklogItem (~2 min)
- Add `edge.To("stuck_states", BacklogStuckState.Type).Annotations(entsql.OnDelete(entsql.Cascade))` to `BacklogItem.Edges()`.
- Files: `session/ent/schema/backlog_item.go`

##### Task 1.1.1c: Regenerate ent + build (~3 min)
- Run `go generate ./session/ent/...` (NOT hand-typed `ent generate`), then `go build ./...`. Commit all generated `session/ent/` changes together.
- Files: generated `session/ent/**` (do not hand-edit)

#### Story 1.1.2: StuckReason validated enum
**As a** developer, **I want** an `IsValid()`-guarded `StuckReason` type, **so that** only known reasons reach the DB and every consumer must switch exhaustively (validated at the boundary, matching the house `BacklogStatus` style — not a truly-unrepresentable sum type).
**Acceptance Criteria**:
- `StuckReason` is a defined string type with the **six** constants and an `IsValid()` guard.
  - *Given* `StuckReason("rework_cap").IsValid()` and `StuckReason("banana").IsValid()`, *When* evaluated, *Then* the first returns `true` and the second returns `false`.
**Files**: `session/domain/backlog.go` (add `StuckReason` type + constants next to the existing `BacklogStatus` type).

##### Task 1.1.2a: Define StuckReason (~3 min)
- Add `type StuckReason string`, constants `StuckReasonPRReadyUnmerged`, `StuckReasonReworkCap`, `StuckReasonAbandonedReview`, `StuckReasonStaleWork`, `StuckReasonBouncing`, `StuckReasonPushFailed`, an `AllStuckReasons` slice, and `func (r StuckReason) IsValid() bool`.
- Files: `session/domain/backlog.go`

#### Story 1.1.3: Repository MarkStuck / ResolveStuck / FindOpenStuckStates (atomic, resolve-in-place)
**As a** reconciler, **I want** atomic, idempotent, resolve-in-place upsert/resolve/query methods, **so that** a tick records stuck state without duplicating rows, without a TOCTOU window that leaks phantom rows, and without ever clobbering a concurrent legitimate status transition.
**Acceptance Criteria**:
- `MarkStuck` is a single atomic `INSERT ... ON CONFLICT(item_id, reason)` (resolve-in-place) and applies its item-status precondition best-effort.
  - *Given* item `96cc9eaa` is queried as rework-cap-stuck while in `review`, *When* `MarkStuck(ctx, "96cc9eaa", StuckReasonReworkCap, expectedStatus="review", ...)` runs and the precondition read still shows `review`, *Then* one row is upserted; *When* the item concurrently transitioned to `in_progress` before the write, *Then* either the pre-filter skips the write, OR a stale open row is written and the **self-heal sweep (Story 2.1.5d)** resolves it on the next tick — never a permanently-leaked phantom, never an error.
- `MarkStuck` twice for the same still-open condition does not duplicate the row or reset `first_detected_at`.
  - *Given* item `f9fcef32-...`'s `pr_ready_unmerged` row was opened at T0 with `first_detected_at=T0`, *When* `MarkStuck` runs again at T0+60s, *Then* the same single row is updated (`last_checked_at=T0+60s`) with `first_detected_at` unchanged at T0 (the plain 2-column unique index guarantees no duplicate), so the UI's "stuck for 3d" duration keeps counting from T0.
- `MarkStuck` on a `(item_id, reason)` whose only row is **resolved** reopens that row in place.
  - *Given* item `df0d5872`'s `bouncing` row is resolved (`resolved_at` set, `notified_at` set), *When* the condition recurs and `MarkStuck` runs, *Then* the SAME row is reopened (`resolved_at` cleared, `notified_at` cleared, `first_detected_at` reset to now) — no second row is created, and it can notify once again.
- `ResolveStuck` is atomic and idempotent — sets `resolved_at` only on a currently-open row.
  - *Given* item `f9fcef32-...` has an open `pr_ready_unmerged` row, *When* `ResolveStuck(ctx, "f9fcef32-...", StuckReasonPRReadyUnmerged)` runs, *Then* a single `UPDATE ... WHERE resolved_at IS NULL` sets `resolved_at` and it no longer appears in `FindOpenStuckStates`; *When* `ResolveStuck` runs again, *Then* it affects 0 rows and does NOT overwrite the existing `resolved_at` (no error).
- `FindOpenStuckStates` returns only unresolved, un-snoozed rows, parsed into a proven open/un-snoozed projection at the repository boundary (parse-don't-validate) so callers never re-check nullable timestamps.
  - *Given* the 6 review-parked items each have an open row and one is snoozed until tomorrow, *When* `FindOpenStuckStates(ctx)` runs, *Then* it returns 5 projected rows, each carrying the item title, status, and `pr_number`/`pr_url` for rendering.
**Files**: `session/storage_backlog.go` (new `Find*` query + open-projection type), `session/ent_repository_backlog.go` (new `MarkStuck`/`ResolveStuck` — `MarkStuck` via `OnConflictColumns`, `ResolveStuck` via atomic conditional `Update`), `session/repository.go` (extend `BacklogItemPrecondition` usage if needed).

##### Task 1.1.3a: MarkStuck resolve-in-place upsert with best-effort precondition (~10-15 min)
<!-- Estimate widened from ~5 min: this is a three-way conditional upsert (fresh insert vs.
     refresh-open-row vs. reopen-resolved-row) over a single atomic ON CONFLICT statement with
     a non-atomic best-effort status pre-filter — inherently denser than a plain insert; the
     estimate reflects that, it is not split further. -->


- Implement `MarkStuck(ctx, itemID, reason, expectedStatus, context)` on `EntRepository`: read item, if `current.Status != expectedStatus` return `(applied=false, nil)` (best-effort pre-filter); else a single `client.BacklogStuckState.Create(...).OnConflictColumns(item_id, reason).Update(func(u){ u.SetLastCheckedAt(now); u.SetContext(...); /* reopen-in-place */ u.SetFirstDetectedAt(...ONLY when the existing row is resolved) ... }).Exec(ctx)`. On a fresh insert set `first_detected_at`/`last_checked_at`/`context`, leave `notified_at` null. On conflict with a **resolved** row, clear `resolved_at`+`notified_at` and reset `first_detected_at` (reopen); on conflict with an **open** row, update `last_checked_at`/`context` only. The upsert is one atomic statement — no read-then-write for row dedup. Note explicitly: the status pre-filter is NOT atomic across the item table; the self-heal sweep (Task 2.1.5d) is the correctness backstop for stale writes.
- Files: `session/ent_repository_backlog.go`

##### Task 1.1.3b: ResolveStuck (atomic, idempotent) + FindOpenStuckStates (~5 min)
- `ResolveStuck(ctx, itemID, reason)`: a single atomic `client.BacklogStuckState.Update().Where(itemID, reason, resolved_at IS NULL).SetResolvedAt(now).Save(ctx)` returning affected-row-count — idempotent, never overwrites an existing `resolved_at`. `FindOpenStuckStates(ctx)`: predicate `resolved_at IS NULL AND (snoozed_until IS NULL OR snoozed_until < now)`, `WithItem()` eager load, parsed into the open-projection type.
- Files: `session/ent_repository_backlog.go`, `session/storage_backlog.go`

##### Task 1.1.3c: MarkNotified helper (~2 min)
- `MarkStuckNotified(ctx, itemID, reason)`: set `notified_at=now` where null — the durable notify-once write called after a successful `notify()`.
- Files: `session/ent_repository_backlog.go`

#### Story 1.1.4: Startup backfill (storm suppression)
**As a** maintainer, **I want** pre-existing stuck items seeded silently at deploy, **so that** the first tick after shipping does not fire 6+ simultaneous notifications.
**Acceptance Criteria**:
- A one-time backfill inserts open rows with `notified_at` pre-set for all currently-stuck **DB-derivable** items, before the ticker starts, without any GitHub API calls.
  - *Given* at deploy time 6 items are parked in `review`, *When* the backfill runs once at startup, *Then* each DB-derivable stuck condition (`rework_cap`/`abandoned_review`/`stale_work`/`bouncing`/`push_failed`) gets an open row with `notified_at=now` and `first_detected_at=now`, and the first 60s tick issues **zero** new notifications for them.
  - *Given* PR #148 (item `f9fcef32-...`) is green-and-mergeable-unmerged, *When* backfill runs, *Then* it does **NOT** call `GetPRStatus`/`IsPRMerged` and does not seed a `pr_ready_unmerged` row — the first genuine tick surfaces it via its own `notified_at IS NULL` + 30-min gate (one-tick delay, no startup API burst).
**Files**: `server/dependencies.go` (call backfill before the ticker goroutine at line ~822, and only when the backlog feature is enabled), `session/backlog_lifecycle.go` (new `BackfillStuckStates(ctx)` method) or `session/ent_repository_backlog.go`.

##### Task 1.1.4a: BackfillStuckStates method (~4 min)
- Run only the **DB-derivable** detection queries once (`rework_cap`/`abandoned_review`/`stale_work`/`bouncing`/`push_failed`) — explicitly EXCLUDE `pr_ready_unmerged` (needs `GetPRStatus`/`IsPRMerged`, which would burst the GitHub API ungated on every one of the 15+ daily boots, blocking startup — pitfalls.md §4). `MarkStuck` each, then `MarkStuckNotified` each in the same pass (so `notified_at` is non-null). Idempotent via the unique constraint.
- Files: `session/backlog_lifecycle.go`

##### Task 1.1.4b: Wire backfill at startup (~2 min)
- Call `backlogLifecycleListener.BackfillStuckStates(context.Background())` immediately before the `go func(){ ticker ... }()` at `server/dependencies.go:822`, gated so it only runs when the backlog feature is enabled (avoid seeding rows a disabled ticker would never maintain — adversarial-review minor).
- Files: `server/dependencies.go`

---

## Phase 2: Detection & Reconciliation

### Epic 2.1: Surface every stuck-reason class durably
**Goal**: Make every detector write durable `BacklogStuckState` rows (replacing the in-memory maps) and add the three missing detectors (green-PR-unmerged, bouncing, push-failed), all inside the existing `ReconcileStuck` pipeline — no second ticker. The fuzzy decision arithmetic is extracted as pure, DB-independent, unit-tested functions; a self-heal sweep and per-detector panic recovery keep the signal trustworthy.

#### Story 2.1.0: Pure decision functions (DB-independent, unit-tested)
**As a** maintainer, **I want** the stuck/threshold/cycle decisions as pure functions, **so that** the fuzziest logic (root cause #4, requirements' flagged rabbit hole) has exhaustive table-driven unit tests with no DB, where the false-positive risk actually lives.
**Acceptance Criteria**:
- The decision arithmetic lives in pure functions taking plain values (times, counts, bools) and returning a bool, testable with no `*EntRepository`.
  - *Given* `stuckPRReady(firstDetected, now)`, `abandonedReview(lastReviewAt, now)`, and `isBouncing(cycleCount int, hasPass bool)`, *When* each is called with boundary inputs (e.g. exactly 30m, exactly 3 cycles with/without PASS), *Then* the returned bool matches the documented threshold — verified by table-driven tests with zero DB access.
- The **solo readiness predicate** `prReadyToMergeSolo(info *github.PRInfo) bool` is a pure function that returns readiness **without** requiring approval, so an unapproved-but-green self-authored PR qualifies (single-user repos cannot self-approve — pre-mortem F1).
  - *Given* `prReadyToMergeSolo` fed a PR that is not draft, has `ChangesRequestedCount == 0`, `CheckConclusion ∈ {"success",""}`, `Mergeable == "MERGEABLE"`, and state open with `ApprovedCount == 0`, *When* called, *Then* it returns `true`; *Given* the same PR with `ChangesRequestedCount > 0` OR `CheckConclusion == "failure"` OR `IsDraft` OR `Mergeable == "CONFLICTING"`, *Then* it returns `false` — i.e. it keeps every block `DerivePRPriority` enforces except the `ApprovedCount > 0` gate.
- The reconcilers call these functions after fetching data; the functions do not import ent.
  - *Given* the reconciler bodies, *When* reviewed, *Then* each threshold/cycle comparison is delegated to a pure function (not inlined `time.Since(...) > 30m` entangled with a DB read).
**Files**: `session/backlog_lifecycle.go` (or a new `session/stuck_decisions.go`) for the pure functions + constants, `session/stuck_decisions_test.go` (table-driven unit tests).

##### Task 2.1.0a: Extract + test the pure decision functions (~5 min)
- Define `stuckPRReady`, `abandonedReview`, `isBouncing`, and `prReadyToMergeSolo(info *github.PRInfo) bool` (and any other threshold predicate) beside the `prReadyThreshold`/`bounceThreshold`/`bounceLookback` constants; write table-driven tests covering each boundary. `prReadyToMergeSolo` is `!info.IsDraft && info.ChangesRequestedCount == 0 && (info.CheckConclusion == "success" || info.CheckConclusion == "") && strings.EqualFold(info.Mergeable, "MERGEABLE") && state ∉ {merged,closed}` — the same disqualifiers `DerivePRPriority` applies MINUS the `ApprovedCount > 0` gate (do NOT call `DerivePRPriority`/`PRPriorityReady`, which is a permanent false-negative on a self-authored PR — pre-mortem F1). Reconcilers in 2.1.1/2.1.3/2.1.4 call these instead of inlining the arithmetic.
- Files: `session/stuck_decisions.go` (new) or `session/backlog_lifecycle.go`, `session/stuck_decisions_test.go` (new)

#### Story 2.1.1: PR green-and-mergeable-but-unmerged signal
**As** Tyler, **I want** a durable flag + notification when a PR is green, mergeable, and unmerged past the threshold, **so that** a ready PR never sits for days unseen (fixes root cause #1).
**Acceptance Criteria**:
- The dead-end healthy-PR branch in `ReconcilePRPending` keys readiness on the **solo predicate `prReadyToMergeSolo(info)`** (Story 2.1.0) — **NOT** `github.DerivePRPriority(info) == github.PRPriorityReady`, which requires `ApprovedCount > 0` (`github/priority.go:50`) and is a permanent false-negative on a self-authored single-user PR (pre-mortem F1) — and marks stuck + notifies once past 30 min.
  - *Given* item `f9fcef32-c27e-434d-b23f-c873c18afa92`'s PR #148 is green, mergeable, no changes requested, not draft, **unapproved** (`ApprovedCount == 0`, because Tyler cannot self-approve) and not merged, *When* that condition has held since `first_detected_at` for > 30 min across ticks (`prReadyToMergeSolo(info)` true AND `stuckPRReady(...)` true), *Then* an open `pr_ready_unmerged` row exists and exactly one notification ("PR #148 is ready to merge") has fired (`notified_at` set), and no second notification fires on subsequent ticks while it stays ready. **The lack of an approval MUST NOT prevent the flag** — this is the flagship case.
  - *Given* a PR that has CHANGES_REQUESTED (`ChangesRequestedCount > 0`), failing CI, is draft, or has conflicts (`Mergeable != "MERGEABLE"`), *When* the branch runs, *Then* it is **not** flagged `pr_ready_unmerged` — `prReadyToMergeSolo` keeps every genuine block, dropping only the approval requirement (no false-positive on a truly-blocked PR).
- **Poll-shaped resolve (else-branch, mark-OR-resolve every tick — pre-mortem F2):** when the branch re-evaluates and finds readiness no longer holds while the item is still `pr_pending` (e.g. a new commit re-runs CI or introduces a conflict), it calls `ResolveStuck` directly at that point.
  - *Given* an item with an open `pr_ready_unmerged` row whose PR then gets a new commit (now CI-running or conflicting) while the item stays `pr_pending`, *When* the tick runs and `prReadyToMergeSolo(info)` returns false, *Then* the detector calls `ResolveStuck(..., StuckReasonPRReadyUnmerged)` in the same branch — it does NOT wait for the status-anchored self-heal sweep, which structurally cannot see a same-status clear (row status is still `pr_pending`).
- No second GitHub API call is added, and readiness is computed by the one co-located predicate.
  - *Given* the existing `GetPRStatus(item.PrNumber)` / `PRInfo` result already in scope at `session/backlog_lifecycle.go:874`, *When* the new branch runs, *Then* it consumes that same result via `prReadyToMergeSolo(info)` — grep confirms no new `GetPRStatus`/`gh` call and no ad-hoc inline re-derivation (the predicate is the single definition).
**Files**: `session/backlog_lifecycle.go` (replace the `continue` at line 874-876 with `prReadyToMergeSolo`-keyed mark-OR-resolve + notify-once logic). If `PRInfo` is not in scope at line 874, thread it through rather than re-implementing the predicate.

##### Task 2.1.1a: Replace the healthy-PR no-op branch (~5 min)
- At line 874: replace the raw-flag check with `prReadyToMergeSolo(info)` (thread `PRInfo` through if needed) — do NOT gate on `DerivePRPriority`/`PRPriorityReady` (permanent false-negative on an unapproved self-authored PR, pre-mortem F1). **Mark-OR-resolve every tick (pre-mortem F2):** if `prReadyToMergeSolo(info)` is true → `MarkStuck(..., StuckReasonPRReadyUnmerged, expectedStatus="pr_pending", ...)`, read back the row, and if `notified_at` is null AND `stuckPRReady(firstDetected, now)` (Story 2.1.0), call `notify(...)` then `MarkStuckNotified`; **else** (`prReadyToMergeSolo(info)` false while still `pr_pending`) → `ResolveStuck(..., StuckReasonPRReadyUnmerged)` so a cleared-without-status-change condition doesn't linger (the sweep can't catch it). Add `const prReadyThreshold = 30 * time.Minute` (used by the pure `stuckPRReady`).
- Files: `session/backlog_lifecycle.go`

#### Story 2.1.2: Rework-cap durable write
**As** Tyler, **I want** the rework-cap hit recorded durably, **so that** a missed toast doesn't make item `96cc9eaa` invisible again (fixes root cause #2).
**Acceptance Criteria**:
- `notifyReworkCapHit` also writes a durable `rework_cap` stuck row.
  - *Given* item `96cc9eaa` reaches its 3rd work session and `AutoReopenAfterFailedReview` hits the cap, *When* `notifyReworkCapHit` fires, *Then* an open `rework_cap` stuck row exists for `96cc9eaa` with `context` describing the cap hit, and it survives a service restart (re-querying `FindOpenStuckStates` after restart still returns it).
**Files**: `server/services/backlog_service_triage.go` (extend `notifyReworkCapHit` at line 29 to call the storage `MarkStuck` + `MarkStuckNotified`).

##### Task 2.1.2a: Persist on cap hit (~4 min)
- In `notifyReworkCapHit`, after the `eventBus.Publish`, call `s.storage.MarkStuck(ctx, itemID, StuckReasonReworkCap, expectedStatus=<current>, context)` then `MarkStuckNotified`. Thread `ctx` in if the signature lacks it (call sites at lines 421-423, 482-484 pass it). Decision per architecture.md §5: the write lives here (BacklogService already knows the exact cap-hit moment) rather than re-deriving in the listener.
- Files: `server/services/backlog_service_triage.go`

#### Story 2.1.3: Route abandoned_review and stale_work through durable state
**As** Tyler, **I want** the two existing in-memory notify-once reconcilers backed by the durable table, **so that** their "since when" and dedup survive restarts (fixes root cause #3).
**Acceptance Criteria**:
- `reconcileStuckReviewItems` writes a durable `abandoned_review` row and reads `notified_at` for dedup instead of the in-memory map, past the 15-min grace.
  - *Given* one of the 6 review-parked items has had its most recent `to_status="review"` event > 15 min ago with nothing in flight, *When* the tick runs, *Then* an open `abandoned_review` row is written, notified once, and after a restart the same tick does NOT re-notify (because `notified_at` is read from the DB, not a fresh empty map).
- **Poll-shaped resolve (else-branch, mark-OR-resolve every tick — pre-mortem F2):** `reconcileStuckReviewItems` calls `ResolveStuck(StuckReasonAbandonedReview)` when it re-evaluates an item and the abandoned condition no longer holds while the item is still `review`.
  - *Given* an item with an open `abandoned_review` row whose review gate comes back in flight (a review/work session becomes active again) while the item stays `review`, *When* the tick runs, *Then* the detector's else-branch calls `ResolveStuck(..., StuckReasonAbandonedReview)` immediately — the status-anchored self-heal sweep cannot see this same-status clear.
- `reconcileStaleWorkSessions` writes a durable `stale_work` row using the existing 2h threshold.
  - *Given* an `in_progress` item's active work session has `LastProgressAt` > 2h ago, *When* the tick runs, *Then* an open `stale_work` row exists and notify-once is DB-backed.
- **Poll-shaped resolve (else-branch, mark-OR-resolve every tick — pre-mortem F2):** `reconcileStaleWorkSessions` calls `ResolveStuck(StuckReasonStaleWork)` when the active session resumes reporting progress while the item stays `in_progress`.
  - *Given* an item with an open `stale_work` row whose active work session then reports fresh progress (`LastProgressAt` now within 2h) while the item stays `in_progress`, *When* the tick runs, *Then* the detector's else-branch calls `ResolveStuck(..., StuckReasonStaleWork)` directly — not relying on the self-heal sweep, which structurally cannot see a same-status clear.
- The in-memory `staleWorkNotified` / `stuckReviewNotified` maps are removed.
  - *Given* the struct fields at `session/backlog_lifecycle.go:119-133`, *When* the change lands, *Then* those fields and their mutexes are deleted and no code references them.
- **Zombie-session review items are covered (pre-mortem F3):** `abandoned_review` is broadened so a `review`-status item whose only "active" (`EndedAt IS NULL`) session's underlying tmux/CLI process is confirmed dead is treated as abandoned — closing the gap where `FindStuckReviewItems` *excludes* such items (`storage_backlog.go:519-527` requires NO session with `EndedAt IS NULL`), leaving zombie-session items stuck-forever after ship (the exact failure this feature exists to prevent).
  - *Given* a `review`-status item whose only session with `EndedAt IS NULL` has an underlying tmux/CLI session that no longer exists (a zombie: the row looks active in the DB but the process is gone), *When* the tick runs, *Then* the item IS flagged with an open `abandoned_review` row and notified once — it is not silently skipped by the `EndedAt IS NULL` exclusion. The liveness check REUSES the existing session-liveness helper the codebase already relies on (`Instance.TmuxSessionExists()` / `TmuxProcessManager.DoesSessionExist()`) rather than inventing a new one.
**Files**: `session/backlog_lifecycle.go` (edit both reconcilers; add the zombie-session liveness path; delete the map fields + mutexes at lines 119-133, 609-614, 678-683), `session/storage_backlog.go` (a `FindZombieReviewItems`/broadened query that includes `review` items whose only un-ended session is dead, complementing `FindStuckReviewItems`).

##### Task 2.1.3a: Convert reconcileStuckReviewItems to durable dedup + else-branch resolve (~5 min)
- Replace the `stuckReviewNotifiedMu` block with `MarkStuck(StuckReasonAbandonedReview, expectedStatus="review")` + a 15-min grace check on the most-recent review status-event time; notify only if `notified_at` null; then `MarkStuckNotified`. Persist the existing `outcomeDesc` as `context`. **Else-branch (pre-mortem F2):** for a `review` item that no longer meets the abandoned condition (review gate back in flight) but still has an open row, call `ResolveStuck(StuckReasonAbandonedReview)` on the same tick.
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.3b: Convert reconcileStaleWorkSessions to durable dedup + else-branch resolve (~5 min)
- Replace the `staleWorkNotifiedMu` block with `MarkStuck(StuckReasonStaleWork, expectedStatus="in_progress")` + `notified_at`-null check + `MarkStuckNotified`. **Else-branch (pre-mortem F2):** when the active session's `LastProgressAt` is back within `maxWorkSessionStaleness` while the item stays `in_progress` and an open `stale_work` row exists, call `ResolveStuck(StuckReasonStaleWork)` on the same tick.
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.3c: Delete the in-memory maps (~2 min)
- Remove `staleWorkNotifiedMu`/`staleWorkNotified`/`stuckReviewNotifiedMu`/`stuckReviewNotified` fields and their initialization in `newListenerBase`.
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.3d: Zombie-session review detection (pre-mortem F3) (~10-15 min)
<!-- Estimate widened from ~5 min: adds a new/broadened store query PLUS a per-item process
     liveness check threaded through the listener (tmux/CLI session existence), a genuinely
     denser task than a single query edit. Reflected in the estimate, not split further. -->


- `FindStuckReviewItems` (`storage_backlog.go:519-527`) excludes any `review` item that still has a session with `EndedAt IS NULL` — but a **zombie** session (DB row un-ended, underlying tmux/CLI process actually dead) leaves such an item invisible to *every* detector (not `stale_work` — it's `review` not `in_progress`; not `abandoned_review` — a session "exists"; maybe not `bouncing`/`rework_cap`). At least some of the original 6 observed stuck-in-review items are this class. Add a `FindZombieReviewItems` query (or broaden the reconciler) that surfaces `review` items whose only `EndedAt IS NULL` session is confirmed dead, verifying liveness with the **existing** helper (`Instance.TmuxSessionExists()` / `TmuxProcessManager.DoesSessionExist()` — thread a liveness accessor into the listener if not already reachable; do NOT invent a new liveness mechanism). Flag those via `MarkStuck(StuckReasonAbandonedReview, expectedStatus="review", context="review session process is gone (zombie)")` + notify-once, and include them in the F2 else-branch resolve when a live session reappears.
- Files: `session/backlog_lifecycle.go`, `session/storage_backlog.go`

#### Story 2.1.4: Bouncing (non-converging cycle) detector
**As** Tyler, **I want** items that thrash `in_progress ↔ review` flagged, **so that** a churning item like `df0d5872` is visible even when it never hits the cap (fixes root cause #4).
**Acceptance Criteria**:
- A new reconciler queries `BacklogStatusEvent` for ≥3 `in_progress→review` round trips in 24h with no PASS and marks `bouncing`.
  - *Given* item `df0d5872` has bounced `in_progress ↔ review` across 6 triage + 3 review + 2 work sessions with its status events showing ≥3 `in_progress→review` transitions inside the last 24h and no PASS verdict, *When* the tick runs, *Then* an open `bouncing` row is written with `context` naming the cycle count, and notified once.
- An item with <3 cycles or a PASS verdict is not flagged.
  - *Given* an item with exactly 2 `in_progress→review` transitions in 24h, *When* the tick runs, *Then* no `bouncing` row is created.
**Files**: `session/backlog_lifecycle.go` (new `reconcileBouncingItems`), `session/storage_backlog.go` (new `CountReviewCyclesSince` query over `BacklogStatusEvent`), and call it from `ReconcileStuck` at line 594.

##### Task 2.1.4a: Cycle-count query (~5 min)
- `CountReviewCyclesSince(ctx, itemID, since)` counts `to_status="review" AND from_status="in_progress"` `BacklogStatusEvent` rows ordered by `created_at` within the window. Add `const bounceThreshold = 3`, `const bounceLookback = 24 * time.Hour`.
- Files: `session/storage_backlog.go`

##### Task 2.1.4b: reconcileBouncingItems + wire into pipeline (~5 min)
- Iterate items with recent activity, call `CountReviewCyclesSince`, then delegate the flag decision to the pure `isBouncing(cycleCount, hasPass)` (Story 2.1.0) — do NOT inline the `>= 3 && !hasPass` arithmetic; `MarkStuck(StuckReasonBouncing, ...)` + notify-once when it returns true. Wire `l.reconcileBouncingItems(ctx, er)` into `ReconcileStuck` **before** the merge-detection (`ReconcilePRPending`) step, or wrapped in its own `recover()` (Task 2.1.5e), so a bouncing-detector panic can't skip merge auto-transition-to-done.
- Files: `session/backlog_lifecycle.go`

#### Story 2.1.5: ResolveStuck wiring, self-heal sweep, and per-detector panic recovery
**As** Tyler, **I want** stuck rows cleared the moment an item makes progress AND a self-heal pass plus per-detector isolation, **so that** the UI badge never shows a phantom-stuck item, a stale write self-corrects within one tick, and one detector panicking can't disable the others.
**Acceptance Criteria**:
- Every status-transition-away-from-stuck path calls `ResolveStuck`.
  - *Given* item `f9fcef32-...`'s PR #148 is merged, *When* `ReconcilePRPending` transitions it to `done`, *Then* `ResolveStuck(..., StuckReasonPRReadyUnmerged)` runs in the same path and the row is resolved immediately (not waiting for a later tick).
  - *Given* rework-capped item `96cc9eaa` is manually re-reviewed via `TriggerReReview` and moves to `in_progress`, *When* that transition completes, *Then* its `rework_cap` and `abandoned_review` rows are resolved.
- A resolved condition that recurs is reopened **in place** (same row, resolve-in-place), NOT a second row.
  - *Given* item `df0d5872` was `bouncing`, resolved, then bounces again, *When* re-detected, *Then* the SAME `bouncing` row is reopened (`resolved_at`/`notified_at` cleared, `first_detected_at` reset) — there is exactly one row per `(item_id, reason)` at all times, and it can notify once again.
- A self-heal sweep resolves any open row whose current item status is inconsistent with its reason's expected status.
  - *Given* a phantom open `stale_work` row exists for an item that has since moved to `done` (e.g. a write raced a transition, or an un-stick call site was missed such as `onSessionExited`), *When* the next tick's self-heal pass runs, *Then* the row is resolved (its expected-status no longer matches the item's current status), so no false-positive stuck signal leaks beyond one tick.
- Each new detector is panic-isolated.
  - *Given* `reconcileBouncingItems` panics on one tick, *When* the tick runs, *Then* the existing merge detection (`ReconcilePRPending`) and the other reconcilers still complete (the panic is caught by the detector's own `recover()`, not the whole-tick `recover()` at `dependencies.go:828-832`).
**Files**: `session/backlog_lifecycle.go` (merge→done path ~line 594/ReconcilePRPending, `pushAndCreatePR` pr_pending transition, `onSessionExited`, new self-heal pass + per-detector `recover()`), `server/services/backlog_service_triage.go` (`AutoReopenAfterFailedReview`/`AutoReopenForPRFix`), `server/services/backlog_service.go` (`TriggerReReview`, manual `TransitionBacklogItemStatus`).

##### Task 2.1.5a: Resolve on merge + pr_pending transitions + onSessionExited (~4 min)
- Wire `ResolveStuck` into the merge→done path, the `pushAndCreatePR` pr_pending transition, and `onSessionExited` (named in pitfalls §1 as a status-mutating path).
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.5b: Resolve on auto-reopen + manual re-review/transition (~4 min)
- Files: `server/services/backlog_service_triage.go`, `server/services/backlog_service.go`

##### Task 2.1.5c: Resolve-in-place reopen semantics on MarkStuck (~3 min)
- Confirm `MarkStuck` (Task 1.1.3a) reopens a resolved `(item_id, reason)` row **in place** — clear `resolved_at`+`notified_at`, reset `first_detected_at`, keep exactly one row via the plain 2-column `OnConflictColumns(item_id, reason)` upsert. Episode history is explicitly NOT retained (ADR-001); recurrence, if ever needed, would be a separate append-only table. No 3-column or partial index. Document the resolve-in-place model in the schema comment.
- Files: `session/ent/schema/backlog_stuck_state.go`, `session/ent_repository_backlog.go`

##### Task 2.1.5d: Self-heal sweep in the reconcile tick (~10-15 min)
<!-- Estimate widened from ~4 min: implementing the 6-branch per-reason expected-status table
     below (3 status-anchored reasons, the two-status `bouncing` span, and 2 event-shaped
     reasons excluded from the sweep) plus its matching C1 regression tests is denser than a
     simple iterate-and-resolve loop. Reflected in the estimate, not split further. -->


- Add a pass in `ReconcileStuck` (or a `reconcileSelfHealStuck`) that iterates `FindOpenStuckStates` (already eager-loads the item) and, for each row whose reason's expected status no longer matches the item's current status, calls the atomic idempotent `ResolveStuck`. This backstops racing `MarkStuck` writes AND any missed un-stick call site. Nearly free (no extra query).
- **Per-reason expected-status map (adversarial concern C1 — the sweep MUST key off this exact table, not a single "expected status" scalar):**

  | Reason | Anchor set (item status the open row is consistent with) | Sweep behavior |
  |---|---|---|
  | `pr_ready_unmerged` | `{pr_pending}` | Resolve when item status ∉ anchor set (e.g. moved to `done`). Same-status clears are NOT handled here — the detector's F2 else-branch (Task 2.1.1a) handles them. |
  | `abandoned_review` | `{review}` | Resolve when item status ∉ anchor set. Same-status clears handled by the detector's F2 else-branch (Task 2.1.3a). |
  | `stale_work` | `{in_progress}` | Resolve when item status ∉ anchor set. Same-status clears handled by the detector's F2 else-branch (Task 2.1.3b). |
  | `bouncing` | `{in_progress, review}` | `bouncing` legitimately **spans both** halves of the cycle, so the sweep does **NOT** resolve while the item sits in `in_progress` OR `review` (either is a healthy half-cycle — resolving there would kill a valid signal). Resolve **only** when the item reaches a terminal/converged status: `done` (or a recorded PASS verdict). |
  | `rework_cap` | — (event-shaped, `expectedStatus=<current>` at write, no fixed anchor) | **Excluded from the status sweep entirely.** These rows are resolved ONLY by their explicit event-site `ResolveStuck` call-sites (Task 2.1.5b for `rework_cap`; Task 2.1.6a for `push_failed`), never by status-anchor matching. |
  | `push_failed` | — (event-shaped, `expectedStatus=<current>` at write, no fixed anchor) | **Excluded from the status sweep entirely** (same as `rework_cap`). |

  Implementation: the sweep switches on `row.Reason`; the three anchored reasons resolve on `status ∉ anchorSet`; `bouncing` resolves only on `done`/PASS (never on `in_progress`/`review`); `rework_cap`/`push_failed` are skipped by the sweep (`continue`). This matches the C1 regression tests in validation.md (`TestSelfHealSweep_should_resolveAnchoredRow_*`, `*_should_notResolveBouncingRow_When_ItemInInProgressHealthyHalfCycle`, `*_should_resolveBouncingRow_When_ItemReachesDoneOrPass`, `*_should_notResolveEventShapedRows_When_StatusVaries`).
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.5e: Per-detector panic recovery + loud-failure logging (pre-mortem P3) (~5 min)
- Wrap each new detector (`reconcileBouncingItems`, the durable-write reconcilers, self-heal sweep) in its own `func(){ defer recover(); ... }()` so a panic in one does not skip the others or merge detection. The existing whole-tick `recover()` (`dependencies.go:828-832`) stays as the outer net.
- **The per-detector `recover()` MUST log at WARNING** (`log.WarningLog`, `[BacklogLifecycle]` prefix) naming the detector that panicked and the recovered value — a swallowed panic must be *loud*, never silent, because there are no metrics/alerts on this single-user tool (pre-mortem F5/P3). A silently-dead detector must be distinguishable from "nothing stuck" in the logs, not indistinguishable from a healthy empty state.
- **Add a per-tick detector self-check**: after all detectors run, emit one INFO/DEBUG "self-check" line summarizing which detectors completed successfully this tick (e.g. `[BacklogLifecycle] stuck sweep tick: detectors ok=[pr_ready,rework_cap,abandoned_review,stale_work,bouncing,push_failed] panicked=[] openRows=N`), so a detector that stops running (or starts panicking every tick) is visible over time in the logs rather than presenting as a permanent calm empty state. See the Observability Plan for the log-line shape.
- Files: `session/backlog_lifecycle.go`

#### Story 2.1.6: push_failed durable write (event-shaped)
**As** Tyler, **I want** a push/PR-creation failure recorded durably, **so that** an item left with no PR at all (invisible to `FindPRPendingItems`' `PrNumberGT(0)` filter) is not reduced to a single ephemeral ERROR toast (requirements In-Scope class "push/PR-creation failure").
**Decision (concern #8, resolved explicitly)**: "push/PR-creation failure" is a **literal** push/`gh pr create` failure, distinct from `pr_ready_unmerged` (which is the healthy-green-unmerged case). It already fires a `NOTIFICATION_TYPE_ERROR` via `stayInReviewAndNotify`, but that ephemeral toast is exactly what this feature exists to replace — so it ALSO gets a durable `push_failed` row. Event-shaped like `rework_cap`: written at the failure site, not poll-derived (threshold 0 / immediate).
**Acceptance Criteria**:
- The `pushAndCreatePR` failure path writes a durable `push_failed` stuck row alongside the existing ERROR notification.
  - *Given* `pushAndCreatePR` fails (push rejected or `gh pr create` errors) for an item, *When* `stayInReviewAndNotify` handles it, *Then* an open `push_failed` row exists for that item with `context` describing the failure, `notified_at` set (dedup), and it survives a restart (`FindOpenStuckStates` still returns it).
- The row resolves when the item next successfully pushes/creates a PR or transitions off the failed state.
  - *Given* an item with an open `push_failed` row, *When* a subsequent push/PR-creation succeeds (or the self-heal sweep sees the status moved on), *Then* the row is resolved.
**Files**: `session/backlog_lifecycle.go` (or wherever `pushAndCreatePR`/`stayInReviewAndNotify` lives — write `MarkStuck(StuckReasonPushFailed, ...)` + `MarkStuckNotified` at the failure site; `ResolveStuck` on subsequent success).

##### Task 2.1.6a: Persist on push/PR-creation failure (~4 min)
- At the `pushAndCreatePR` failure / `stayInReviewAndNotify` site, after the existing ERROR notification, call `MarkStuck(ctx, itemID, StuckReasonPushFailed, expectedStatus=<current>, context)` then `MarkStuckNotified`. Add `ResolveStuck(StuckReasonPushFailed)` to the success path.
- Files: `session/backlog_lifecycle.go`

---

## Phase 3: Read RPC Surface

### Epic 3.1: BacklogService stuck-item RPCs
**Goal**: Expose open stuck items (and a snooze) over the existing all-unary `BacklogService`, with a proto `StuckReason` enum and a feature-registry entry.

#### Story 3.1.1: Proto StuckReason enum + ListStuckBacklogItems + SnoozeStuckItem
**As a** frontend, **I want** a typed RPC returning open stuck items with reason, since-when, and PR context, **so that** the UI renders a browsable list.
**Acceptance Criteria**:
- `ListStuckBacklogItems` returns `repeated StuckBacklogItem` (item_id, title, status, `StuckReason reason`, first_detected_at, last_checked_at, pr_number, pr_url, context, snoozed_until).
  - *Given* the 5 un-snoozed open stuck rows, *When* the UI calls `ListStuckBacklogItems`, *Then* it receives 5 `StuckBacklogItem` messages, item `f9fcef32-...` carrying `reason=STUCK_REASON_PR_READY_UNMERGED`, `pr_number=148`, and a `first_detected_at` ~3 days old.
- `StuckReason` is a proto enum with `STUCK_REASON_UNSPECIFIED=0` + the **six** values (`PR_READY_UNMERGED`, `REWORK_CAP`, `ABANDONED_REVIEW`, `STALE_WORK`, `BOUNCING`, `PUSH_FAILED`).
  - *Given* an unknown reason string somehow in the DB, *When* mapped to proto, *Then* it maps to `STUCK_REASON_UNSPECIFIED` (never panics).
- `SnoozeStuckItem(item_id, reason, until)` sets `snoozed_until`.
  - *Given* item `96cc9eaa`'s `rework_cap` row, *When* `SnoozeStuckItem` with `until=tomorrow` is called, *Then* the next `ListStuckBacklogItems` omits it.
**Files**: `proto/session/v1/backlog.proto` (enum + 4 messages + 2 rpcs), `proto/session/v1/types.proto` (if enum belongs there — follow existing enum placement), then `make proto-gen`.

##### Task 3.1.1a: Proto edits (~5 min)
- Add `enum StuckReason`, `message StuckBacklogItem`, request/response messages, and the two `rpc`s to `service BacklogService` (after line 404).
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.1b: Regenerate protos (~2 min)
- `make proto-gen`; commit both `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` (tracked despite gitignore).
- Files: generated (do not hand-edit)

#### Story 3.1.2: Handlers + registry
**As a** frontend, **I want** the handlers implemented and registered, **so that** the RPCs work and pass the registry CI gate.
**Acceptance Criteria**:
- Handlers implemented with `+api:` markers and a backend registry entry.
  - *Given* `ListStuckBacklogItems` calls `storage.FindOpenStuckStates` and maps to proto, *When* invoked in an integration test seeded with item `f9fcef32-...`'s green-PR row, *Then* it returns that item with the correct reason enum; and `docs/registry/features/backend/backlog-list-stuck.json` exists with `markerFound: true`.
**Files**: `server/services/backlog_service.go` (two handlers + `// +api: backlog:list-stuck` / `// +api: backlog:snooze-stuck`), `server/server.go` (already registers `BacklogService` — no change unless new service), `docs/registry/features/backend/backlog-list-stuck.json` (new), then `make registry-generate`.

##### Task 3.1.2a: Implement the two handlers (~5 min)
- Files: `server/services/backlog_service.go`

##### Task 3.1.2b: Registry entry + regenerate (~3 min)
- Create the backend JSON per `.claude/rules/feature-registry.md`; run `make registry-generate`; verify `coverage-gaps.json` count does not grow.
- Files: `docs/registry/features/backend/backlog-list-stuck.json`

---

## Phase 4: Frontend — Stuck Backlog Items section on /unfinished

### Epic 4.1: Browsable stuck-items view
**Goal**: A polled hook, reason-chipped list + expand-on-click detail, a nav badge count, and a new section on `/unfinished` — reusing the existing `unfinished/*` component and `BacklogItemBadge` patterns, styled with vanilla-extract.

#### Story 4.1.1: useStuckBacklogItems hook
**As a** view, **I want** a polled hook exposing the stuck list + a connection/loading state, **so that** the list self-heals each poll and never shows false confidence when disconnected.
**Acceptance Criteria**:
- The hook fetches on mount, polls on an interval, and exposes `{ items, isLoading, error, lastFetched, refetch }`.
  - *Given* the RPC returns 5 stuck items, *When* the component mounts, *Then* `items` has length 5 within one fetch and `lastFetched` is set; *When* the RPC later errors, *Then* `error` is populated (not silently swallowed) so the UI can show "may be out of date".
- Transport/client are memoized (not rebuilt every render).
  - *Given* the hook body, *When* reviewed, *Then* `createConnectTransport`/`createClient` are inside `useMemo` (not the raw-every-render anti-pattern from `useUnfinishedWork`, pitfalls.md §5).
**Files**: `web-app/src/lib/hooks/useStuckBacklogItems.ts` (new).

##### Task 4.1.1a: Implement polled hook (~5 min)
- Model on `useUnfinishedWork.ts` but unary+interval; `useMemo` the transport/client; expose error/loading.
- Files: `web-app/src/lib/hooks/useStuckBacklogItems.ts`

#### Story 4.1.2: Reason chips + StuckItem card + detail
**As** Tyler, **I want** each stuck item shown as a card with a text+color reason chip and duration, expandable to the "why" + PR link, **so that** I can triage in one glance and drill in when needed.
**Acceptance Criteria**:
- A `STUCK_REASON_CLASS` map + `getStuckReasonLabel` pair color and text (no color-only signaling).
  - *Given* `reason=STUCK_REASON_PR_READY_UNMERGED`, *When* rendered, *Then* the chip shows text like "PR ready to merge" with its color class, and carries an `aria-label` with the full phrase.
- The card shows duration-since-stuck at the glance level and expands (Enter/Space, `aria-expanded`) to detail with a direct PR link.
  - *Given* item `f9fcef32-...` (PR #148, stuck ~3 days), *When* the list renders, *Then* "stuck 3d" is visible without expanding; *When* expanded, *Then* the `context`, `last_checked_at`, and a link to PR #148 (`pr_url`) are shown.
- A `pr_status_unknown` / stale-check state renders distinctly, using the concrete **5-minute** staleness threshold.
  - *Given* an item whose `last_checked_at` is older than 5 minutes (poll failed / froze), *When* rendered, *Then* a distinct "couldn't check" chip shows rather than implying healthy.
**Files**: `web-app/src/components/backlog-stuck/StuckItem.tsx` + `.css.ts`, `StuckItemDetail.tsx` + `.css.ts`, `stuckReason.ts` (label/class maps) — mirror `web-app/src/components/unfinished/UnfinishedItem.tsx`, `UnfinishedItemDetail.tsx`, and `BacklogItemBadge.tsx`.

##### Task 4.1.2a: Reason label/class maps (~3 min)
- The `STUCK_REASON_CLASS` + `getStuckReasonLabel` maps must cover **all six** reasons (including `push_failed`, label e.g. "Push/PR-create failed") plus the derived `pr_status_unknown` chip — exhaustive over the proto enum so a new reason is a compile/lint miss, not a silent blank chip.
- Files: `web-app/src/components/backlog-stuck/stuckReason.ts`, `stuckReason.css.ts`

##### Task 4.1.2b: StuckItem card (glance) (~5 min)
- Copy `UnfinishedItem.tsx` expand/keyboard/`aria-expanded` verbatim; show reason chip + duration.
- Files: `web-app/src/components/backlog-stuck/StuckItem.tsx`, `StuckItem.css.ts`

##### Task 4.1.2c: StuckItemDetail (~4 min)
- `context`, `first_detected_at`/`last_checked_at`, PR link, and the read-only `allow_auto_merge` line (Story 4.1.4).
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`, `StuckItemDetail.css.ts`

#### Story 4.1.3: Nav badge + section on /unfinished + empty state
**As** Tyler, **I want** an "N items stuck" badge and a grouped-by-reason section, **so that** I can see at a glance whether anything is stuck and never a bare blank when nothing is.
**Acceptance Criteria**:
- A `StuckNavBadge` shows the open count with a full-phrase `aria-label`, hidden at zero.
  - *Given* 5 open stuck items, *When* nav renders, *Then* the badge shows "5" with `aria-label="5 items stuck"`; *Given* 0, *Then* the badge is not rendered.
- The `/unfinished` page renders a "Stuck Backlog Items" section grouped by reason with a reassuring empty state distinct from a filtered-empty state.
  - *Given* 0 stuck items, *When* the section renders, *Then* it shows "Nothing stuck — all backlog items are progressing" (not a blank area), matching the existing `UnfinishedTab` empty-state convention.
  - *Given* the 5 items across reasons, *When* rendered, *Then* they are grouped by reason class (the actionable unit), sorted stuck-longest-first, with the count region using `aria-live="polite"`.
**Files**: `web-app/src/components/backlog-stuck/StuckNavBadge.tsx` + `.css.ts` (mirror `UnfinishedNavBadge.tsx`), `web-app/src/app/unfinished/UnfinishedTab.tsx` (add the section), `web-app/src/components/backlog-stuck/StuckItemsSection.tsx` + `.css.ts`.

##### Task 4.1.3a: Nav badge (~3 min)
- Files: `web-app/src/components/backlog-stuck/StuckNavBadge.tsx`, `.css.ts`

##### Task 4.1.3b: Grouped section + empty state (~5 min)
- Files: `web-app/src/components/backlog-stuck/StuckItemsSection.tsx`, `.css.ts`

##### Task 4.1.3c: Mount the section on /unfinished + frontend registry (~4 min)
- Add `<StuckItemsSection />` to `UnfinishedTab.tsx`; add `// +feature: backlog-stuck-items` marker; create `docs/registry/features/frontend/backlog-stuck-items.json`; `make registry-generate`.
- Files: `web-app/src/app/unfinished/UnfinishedTab.tsx`, `docs/registry/features/frontend/backlog-stuck-items.json`

#### Story 4.1.4: Read-only allow_auto_merge context (open question #2)
**As** Tyler, **I want** to see the repo's `allow_auto_merge` setting in a stuck PR's detail, **so that** I understand *why* PR #148 isn't auto-merging.
**Acceptance Criteria**:
- The detail view shows `allow_auto_merge: false` read-only, best-effort, backed by a **single per-repo TTL-cached fetch gated on `github.DefaultRateLimiter`** — never a per-item or per-detail-expand call.
  - *Given* item `f9fcef32-...` on a repo with `allow_auto_merge: false`, *When* the `pr_ready_unmerged` detail expands, *Then* a read-only line "Repo auto-merge: off" is shown; *Given* the setting fetch fails, *Then* it shows "unknown" and does not block the rest of the detail.
  - *Given* N stuck items on the same repo, *When* `ListStuckBacklogItems` populates the setting, *Then* the repo's `allow_auto_merge` is fetched **at most once per repo per TTL window** (cached), not once per item.
**Files**: `server/services/backlog_service.go` (include the setting in the RPC response, fetched via the existing `github/` package best-effort, cached per-repo with a TTL and gated on `DefaultRateLimiter`), `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`.

##### Task 4.1.4a: Populate + render setting (~4 min)
- Add a per-repo TTL cache (reuse the existing rate-limit/backoff pattern from `session/pr_status_poller.go` / `session/worktree_pr_poller.go`; do not add an ungated GitHub call) keyed by repo; populate the RPC field from the cache; render read-only in the detail.
- Files: `server/services/backlog_service.go`, `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`

#### Story 4.1.5: e2e coverage
**As a** maintainer, **I want** a Playwright spec, **so that** the view meets the e2e/registry gates.
**Acceptance Criteria**:
- A spec asserts the badge + list render with seeded stuck data using `data-testid`/ARIA locators only, feature-annotated.
  - *Given* a seeded stuck item, *When* the spec loads `/unfinished`, *Then* it finds the stuck section, the reason chip, and the duration via `getByTestId`/`getByRole` (no CSS selectors, no `waitForTimeout`), and the file starts with `// @feature backlog:list-stuck`.
**Files**: `tests/e2e/backlog-stuck-items.spec.ts` (new), `tests/e2e/pages/` (helper if navigation is reused).

##### Task 4.1.5a: Write the spec (~5 min)
- Files: `tests/e2e/backlog-stuck-items.spec.ts`

---

## Phase 5: Snooze wiring (secondary)

### Epic 5.1: Snooze without remediation
**Goal**: Let Tyler say "I know, stop reminding me" per stuck item — visibility control, not a remediation action — reusing the `/unfinished` snooze shape.

#### Story 5.1.1: Snooze button → SnoozeStuckItem → refresh
**As** Tyler, **I want** a snooze control on each stuck card, **so that** an intentionally-parked item stops nagging without me merging/retrying it.
**Acceptance Criteria**:
- A hover-revealed snooze control calls `SnoozeStuckItem` and the item leaves the active list.
  - *Given* rework-capped item `96cc9eaa`, *When* Tyler clicks "Snooze 1 day", *Then* `SnoozeStuckItem(item_id, reason, until=+24h)` is called, the hook refetches, and the item no longer appears (until the snooze expires), mirroring `UnfinishedItem`'s snooze affordance.
**Files**: `web-app/src/components/backlog-stuck/StuckItem.tsx` (snooze button), `web-app/src/lib/hooks/useStuckBacklogItems.ts` (imperative `snooze()` action).

##### Task 5.1.1a: Snooze action + button (~4 min)
- Files: `web-app/src/lib/hooks/useStuckBacklogItems.ts`, `web-app/src/components/backlog-stuck/StuckItem.tsx`

---

## Explicit Non-Goals (do NOT create stories for these)
- Changing the GitHub repo's `allow_auto_merge` or any branch-protection setting (surfaced read-only only; never written).
- Changing `maxAutoReworkIterations` value or policy (reused as the bouncing threshold; not modified).
- One-click remediation (retry rework / re-trigger review / retry push / merge now) from the view. Snooze is visibility control, not remediation.
- Any change to review-verdict logic, the `c99f6595` diff auto-repair mechanism, or the reasons individual reviews FAIL.
- **Episode / flapping history** (retaining resolved rows to count how many times an item has recurred). Explicitly dropped: `requirements.md` asks only for *current* stuck-state visibility, not historical episode tracking, and the multi-episode model is incompatible with the plain-unique `OnConflict` upsert on SQLite (NULLs distinct). The table uses **resolve-in-place** (one row per `(item_id, reason)`, reopened not duplicated). If recurrence counting is ever wanted, it belongs in a separate append-only table — not this live-state table. Rejected on the strength of both the architecture and adversarial reviews (see ADR-001).
