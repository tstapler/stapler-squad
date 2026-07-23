# ADR-001: Durable Stuck-State Storage Model

**Status**: Accepted
**Date**: 2026-07-14
**Feature**: backlog-stuck-item-visibility
**Deciders**: planning phase (SDD Phase 3)

## Context

Backlog items that stop progressing toward merge are invisible except as ephemeral
toasts. Four confirmed root causes (see requirements.md) share one structural gap:
the "already notified / has been stuck since T" bookkeeping lives in in-memory
`map[string]bool` fields (`staleWorkNotified`, `stuckReviewNotified`) on
`BacklogLifecycleListener` (`session/backlog_lifecycle.go:119-133`). These reset on
every process restart — and this dev instance restarts 15+ times/day — so the "since
when" context and notify-once dedup are lost constantly (root cause #3).

We need a durable place to record, per backlog item, that a stuck condition is
currently open, which reason(s) it is stuck for, when it was first detected, whether
the operator has been notified, and when it resolved — queryable for a browsable UI
view and surviving restarts.

## Options Considered

### Option A — New fields directly on `BacklogItem`
Add `stuck_reason string`, `stuck_since time.Time`, `stuck_notified_at *time.Time`
columns to the existing `BacklogItem` entity (`session/ent/schema/backlog_item.go`).

- **Strength**: single-row read; trivially exposed through the existing
  `ListBacklogItems`/`GetBacklogItem` RPCs with no join.
- **Weakness**: a single `stuck_reason` column cannot represent an item that is stuck
  for **more than one** reason at once (edge case #3 in features.md — e.g. rework-cap
  parked AND its last auto-respawned session went stale). Worse, it introduces a second
  mutable source of truth on the *hot* item row: a stuck-flag write racing a legitimate
  `TransitionBacklogItemStatus` write can clobber a concurrent status update unless every
  write carries the precondition guard, and the flag goes stale the instant the item
  un-sticks. Also requires editing the most heavily-used entity in the system.

### Option B — New `BacklogStuckState` entity, resolve-in-place (CHOSEN)
A thin new ent entity keyed by `(item_id, reason)`, **exactly one row per pair**
(resolve-in-place — the row is reopened when a condition recurs, never duplicated),
loosely mirroring the existing `BacklogStatusEvent` shape
(`session/ent/schema/backlog_status_event.go`): `item_id` FK-by-value, `reason`,
`first_detected_at`, `last_checked_at`, `notified_at` (nullable), `resolved_at`
(nullable), `snoozed_until` (nullable), `context` (human-readable "why"), with a **plain
2-column unique index on `(item_id, reason)`** as both the `OnConflictColumns` upsert
target and the correctness guard, and `OnDelete(Cascade)` from `BacklogItem`.

- **Strength**: supports >1 concurrent reason per item (one row each); doubles as BOTH
  the durable notify-once store (replacing the in-memory maps) AND the queryable UI
  source of truth; cascade-deletes cleanly like `status_events` already does. The plain
  2-column unique key makes `MarkStuck`'s upsert both atomic and duplicate-proof at
  single-user scale.
- **Weakness**: more schema surface than A; "currently stuck" requires its own query
  (`resolved_at IS NULL AND (snoozed_until IS NULL OR snoozed_until < now)`), not a
  single column read.

#### Episode / flapping history — considered and explicitly rejected
An earlier draft kept **resolved rows as an append-only episode log** so root cause #4's
"how often does this item bounce" could be answered by counting closed rows per item.
Both the architecture review and the adversarial review independently found this
**incompatible with the notify-once upsert on SQLite** (the driver is `mattn/go-sqlite3`,
`session/ent_repository.go:26,69`; SQLite treats `NULL` as *distinct* in unique indexes,
so a `(item_id, reason, resolved_at)` key permits unlimited concurrent open rows and a
partial `WHERE resolved_at IS NULL` index cannot be an ent `OnConflictColumns` target).
Keeping episode history would therefore either reintroduce the exact double-notification
bug this feature exists to kill or force abandoning the upsert idiom. Since
`requirements.md` asks only for **current** stuck-state visibility — never historical
episode tracking — episode history is **dropped as unnecessary scope**. The table is
resolve-in-place. If recurrence counting is ever genuinely wanted, it belongs in a
**separate append-only table** so the live-state table keeps its clean 2-column unique
key.

### Option C — Fully derived, no new schema
Compute "is stuck + why + since when" on every read from `status`, `pr_url`/`pr_number`
+ a live `GetPRStatus`, `BacklogStatusEvent` history, and `ItemSession` end-times.

- **Strength**: zero migration; no stale/duplicated state; restart-survives trivially
  because there is no cache to invalidate.
- **Weakness**: every list/watch either re-polls GitHub (expensive, rate-limited — bad
  for a UI that should refresh often) or can only report PR mergeability as of the last
  poller tick. And notify-once dedup across the 60s ticker **still** needs *some*
  persisted marker or every tick re-notifies — so C cannot actually eliminate durable
  state, only relocate the hard part.

## Decision

Adopt a **hybrid**: **Option B (resolve-in-place)** for the durable, restart-surviving,
notify-once part (the `BacklogStuckState` entity — one row per `(item_id, reason)`,
reopened not duplicated; no episode history), and **Option C's query-time principle**
for the one fact that must always reflect the latest poll — "is this PR
green-and-mergeable *right now*". Concretely: **do not cache the GitHub-derived PR
verdict in the stuck table** — only persist "have we already notified for this instance
of stuckness" and "when did we first see it". The live PR readiness check remains the
existing `GetPRStatus` result inside `ReconcilePRPending` (pitfalls.md §4: no second
poller, no second GitHub API call). Readiness is judged by a **single co-located
predicate** (`prReadyToMergeSolo`) — see the "Single-user readiness" note below for why
this is deliberately NOT `github.DerivePRPriority(info) == github.PRPriorityReady`.

### Single-user readiness — why `pr_ready_unmerged` is NOT `PRPriorityReady`

`github.PRPriorityReady` requires `info.ApprovedCount > 0` (`github/priority.go:50`).
This repo is a **single-user, self-hosted instance**: Tyler authors every PR and GitHub
forbids self-approval, so `ApprovedCount` is *always* 0 and `DerivePRPriority` resolves to
`PRPriorityPending` **forever**. Keying `pr_ready_unmerged` on `PRPriorityReady` would make
the detector permanently blind and structurally exclude PR #148 — the single flagship case
that motivates this entire feature (pre-mortem F1).

Therefore `pr_ready_unmerged` readiness is computed by a dedicated pure predicate,
`prReadyToMergeSolo(info *github.PRInfo) bool` (Story 2.1.0), defined as:

```
!info.IsDraft
  && info.ChangesRequestedCount == 0                 // no CHANGES_REQUESTED review
  && (info.CheckConclusion == "success" || info.CheckConclusion == "")  // CI green / none
  && strings.EqualFold(info.Mergeable, "MERGEABLE")  // mergeable, no conflicts
  && state ∉ {merged, closed}
```

This intentionally applies **the same blocking-exclusions** `DerivePRPriority` uses
(draft, changes-requested, CI-failure all still disqualify — so this does NOT re-introduce
the risk the architecture review flagged of missing a genuine "changes requested" block)
and **only drops the `ApprovedCount > 0` gate**. It is co-located with the detector and
covered by a unit test asserting an **unapproved** green PR flags `pr_ready_unmerged`
(`TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved`), so the decision
cannot be silently re-diverged back to `PRPriorityReady` in a later refactor. If GitHub's
canonical helper ever needs a no-review-required branch, `prReadyToMergeSolo` is the one
place to migrate it into.

`reason` is modeled as a **validated string-backed enum** — a Go `StuckReason` type with
`IsValid()` and a matching proto `StuckReason` enum on the wire — matching the house
`BacklogStatus`/`ReviewOutcome` style. It is validated at the boundary and forces an
exhaustive switch in handler and UI; it is *not* a truly-unrepresentable sum type
(`StuckReason("banana")` compiles but fails `IsValid()`), so `MarkStuck` only ever
receives compile-time `StuckReason*` constants. The six reasons are `pr_ready_unmerged`,
`rework_cap`, `abandoned_review`, `stale_work`, `bouncing`, and `push_failed` (the last
covering requirements' In-Scope "push/PR-creation failure" class — a literal
push/`gh pr create` failure that leaves an item with no PR, event-shaped like
`rework_cap`; it also fires the pre-existing `stayInReviewAndNotify` ERROR notification,
which the durable row supersedes for restart-surviving visibility).

## Consequences

- One migration: adds the `backlog_stuck_states` table via ent auto-migration
  (`client.Schema.Create` at startup) — additive, no manual migration file. Regenerate
  with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`
  (the `sql/upsert` flag is **required** because `MarkStuck` uses `OnConflictColumns`).
- **Atomic writes + self-heal, not check-then-act.** `MarkStuck` is a single atomic
  `INSERT ... ON CONFLICT(item_id, reason)` (resolve-in-place: reopens a resolved row by
  clearing `resolved_at`/`notified_at` and resetting `first_detected_at`; refreshes an
  open row's `last_checked_at`) — no read-then-write for row dedup. `ResolveStuck` is a
  single atomic idempotent `UPDATE ... SET resolved_at=now WHERE resolved_at IS NULL`
  (never overwrites an existing resolution). The `ExpectedStatus` check is a best-effort
  pre-filter only — it is **not** atomic across the item table, so the plan does **not**
  claim "a tick can never win a race." Instead, a **self-heal sweep** in every reconcile
  tick resolves any open row whose current item status is inconsistent with its reason's
  expected status. This corrects both a stale write that lost a race AND any missed
  un-stick call site (e.g. `onSessionExited`) within one tick — the honest guarantee
  pitfalls.md §1 anticipated (stale writes are possible but self-heal in ≤1 tick).
- **The storage contract is the single seam; detection triggers may be distributed.**
  `MarkStuck` / `ResolveStuck` / `MarkStuckNotified` are the one documented seam both
  `session/backlog_lifecycle.go` (poll-shaped reasons) and
  `server/services/backlog_service_triage.go` (`rework_cap`, event-shaped) call. Detection
  *triggers* are intentionally split by whether the condition is poll- or event-shaped,
  but the storage *contract* stays centralized so a future reason has one obvious home.
- A one-time **backfill** seeds currently-stuck items into the table at deploy with
  `notified_at` pre-set, so the first reconcile tick does not fire a simultaneous
  notification storm for the 6+ pre-existing parked items (pitfalls.md §3). It seeds only
  **DB-derivable** reasons and explicitly **excludes `pr_ready_unmerged`** (which needs
  `GetPRStatus`/`IsPRMerged`) so it makes **zero synchronous GitHub API calls** on the
  15+ daily boots (pitfalls.md §4); the first genuine tick surfaces the green-PR case.
- Un-sticking (`ResolveStuck`) is wired into every path that moves an item off a stuck
  condition, not just the next 60s tick — accepting a ≤60s stale-badge window as a
  documented tradeoff (architecture.md §4.2), with the self-heal sweep as the backstop.
- **Each new detector is panic-isolated** with its own `recover()` so one detector
  panicking (e.g. the new `bouncing` detector) does not skip the others or the existing
  merge auto-transition-to-done; the whole-tick `recover()` remains the outer net.
- **Read-side projection (parse-don't-validate).** `FindOpenStuckStates` parses each row
  into a proven "open, un-snoozed" projection at the repository boundary, so handlers/UI
  receive a validated shape and never re-check the nullable `notified_at`/`resolved_at`/
  `snoozed_until` combinations — the illegal combos are unreachable above persistence even
  though the row stays a flat Data Mapper record.
