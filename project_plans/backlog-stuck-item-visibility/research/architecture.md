# Architecture Research: backlog-stuck-item-visibility

## 1. Existing reconciliation architecture

`BacklogLifecycleListener` (session/backlog_lifecycle.go) is the single reconciliation
actor. It is constructed once in `server/dependencies.go` (`NewBacklogLifecycleListenerWithPool`,
~line 483), wired to every `Instance` via `WireToInstance` (event-driven path:
`EventStarted`/`EventExited` → `onSessionStarted`/`onSessionExited`), and separately driven
by a single 60s ticker goroutine (dependencies.go ~line 822-836) that calls
`ReconcileStuck(ctx)` as "the only fallback" for review-gate respawn, staleness detection,
and PR-pending polling. `ReconcileStuck` is a fixed pipeline, in order:

1. `er.ReconcileStuckItems(ctx)` — `in_progress` items whose sessions have all ended → `review`.
2. `er.FindReviewItemsWithoutGate(ctx)` — re-spawn review gate if headless pool/spawner configured.
3. `reconcileStaleWorkSessions(ctx)` — flag `in_progress` items whose active work session has
   had no progress for > `maxWorkSessionStaleness` (2h). In-memory notify-once via
   `staleWorkNotified map[string]bool` guarded by `staleWorkNotifiedMu`.
4. `er.BackfillMissingPRNumbers(ctx)` — self-heal `pr_pending` items with `pr_url` but `pr_number==0`.
5. `reconcileStuckReviewItems(ctx, er)` — `er.FindStuckReviewItems` finds `review`-status items with
   a review verdict on record but nothing in flight. In-memory notify-once via
   `stuckReviewNotified map[string]bool`.
6. `l.ReconcilePRPending(ctx, er)` — polls `pr_pending` items via `prPendingChecker`
   (`IsPRMerged`, `GetPRStatus` — session/pr_status_poller.go, session/worktree_pr_poller.go
   backed interfaces defined as consumer-scoped interfaces right in backlog_lifecycle.go).
   Transitions to `done` on merge; clears cached PR fields + respawns fix session via
   `PRFixSpawner.AutoReopenForPRFix` on close-without-merge, CI failure, blocking review, or
   conflict. **Gap confirmed**: a PR that is open, green, and mergeable (no CI failure, no
   blocking review, no conflict) falls through the `continue` at line 874-876 with **no
   notification at all** — this is root cause #1 from requirements.md, reproduced exactly.

**Root cause #2** (rework-cap silent parking) lives in a different file/actor:
`server/services/backlog_service_triage.go` — `maxAutoReworkIterations = 3` (line 55),
checked in `AutoReopenAfterFailedReview` (line 397+) and `AutoReopenForPRFix` (line 482+).
On cap hit, `notifyReworkCapHit` (line 29) fires one `events.NewNotificationEvent` via
`s.eventBus` directly — bypassing `BacklogLifecycleListener.notify` entirely (this is
`BacklogService`, a ConnectRPC handler, not the lifecycle listener). No durable record is
kept of the cap-hit; if the notification toast is missed, the item is invisible again,
matching root cause #2.

**Root cause #3** confirmed: `staleWorkNotified` and `stuckReviewNotified` are plain
`map[string]bool` fields on `BacklogLifecycleListener`, reset to empty on every process
restart (`newListenerBase`). No schema, no persistence layer backs them at all.

**Root cause #4** (bouncing without converging): no code path detects this today. The only
history available is `BacklogStatusEvent` (append-only audit log, see below) — nothing
queries it for repeated `in_progress ↔ review` cycles.

## 2. Storage abstraction and ent schema

Three layers, thin pass-through (each earns its place — no interface pollution here):
- `session.Storage` (session/storage.go) — public facade used by services; delegates to `repo`.
- `session.EntRepository` (session/ent_repository_backlog.go, session/storage_backlog.go) —
  concrete ent-backed implementation. `BacklogLifecycleListener.ReconcileStuck` type-asserts
  `l.storage.repo.(*EntRepository)` directly to call Ent-specific reconciler methods
  (`ReconcileStuckItems`, `FindReviewItemsWithoutGate`, `FindStuckReviewItems`,
  `FindPRPendingItems`, `BackfillMissingPRNumbers`, `GetMostRecentReviewVerdictForItem`) —
  these are NOT on any interface, they're concrete-type reconciler helpers colocated with the
  repository. New "stuck state" queries should follow this exact pattern: a new
  `EntRepository` method with an ent predicate query, called from `ReconcileStuck`.
- ent schema: `session/ent/schema/backlog_item.go` (BacklogItem — has `status string`,
  `pr_url`, `pr_number`, `notes`, `user_modified_status_at`, `updated_at`,
  index on `(status, updated_at)` already present — useful for a stuck-detection query) and
  `session/ent/schema/backlog_status_event.go` (BacklogStatusEvent — append-only,
  `item_id, from_status, to_status, triggered_by, note, created_at`, indexed on
  `(item_id, created_at)`, `OnDelete(Cascade)` from BacklogItem).

`TransitionBacklogItemStatus` (session/ent_repository_backlog.go:516) is the canonical
optimistic-concurrency pattern: read current row → check `BacklogItemPrecondition`
(`ExpectedStatus`, `ExpectedUpdatedAt`) → update inside implicit single-statement write →
best-effort append to `BacklogStatusEvent` (audit failure is swallowed, non-fatal). Every
new "mark item as stuck" write must reuse this precondition pattern to avoid racing a
concurrent status transition (see §4).

Ent regeneration MUST use `go run -mod=mod entgo.io/ent/cmd/ent generate --feature
sql/upsert ./session/ent/schema` (session/ent/generate.go) — omitting `--feature sql/upsert`
silently breaks Upsert-style methods.

## 3. Should stuck-state be stored, or derived at query time?

Recommend: **hybrid — derive the stuck *reason* and *since-when* at query time from existing
columns/events; persist only the notify-once dedup key durably.**

Evaluated options:

**A. New fields on `BacklogItem`** (e.g. `stuck_reason string`, `stuck_since time.Time`,
`stuck_notified_at *time.Time`). Pros: single-row read, trivial to expose via existing
`ListBacklogItems`/`GetBacklogItem` RPCs, matches existing precondition/audit pattern.
Cons: requires a migration; introduces a second source of truth that must be kept
consistent with `status`/PR polling results — a write to `stuck_reason` racing a legitimate
status transition needs the same precondition guard `TransitionBacklogItemStatus` uses, or
the stuck flag can go stale (item un-sticks itself but the flag isn't cleared until the next
tick, or worse, the flag write clobbers a concurrent legitimate status update if done as a
separate non-transactional write).

**B. New ent entity** (e.g. `BacklogStuckEvent`, mirroring `BacklogStatusEvent`) — append-only
row per detection tick, `item_id, reason, detected_at, resolved_at nullable`. Pros: preserves
full history (useful for root cause #4's "bouncing" pattern — count distinct stuck episodes
per item), cascade-deletes cleanly like `BacklogStatusEvent` already does. Cons: more schema
surface, needs its own reconciler query to compute "currently stuck" (open row with
`resolved_at IS NULL`) — but this is exactly the append-only-log idiom this codebase already
uses for status events, so it's the most idiomatic fit, not a bigger lift than option A.

**C. Fully derived, no new schema** — compute "is stuck" and "why" on every read from
`status`, `pr_url`/`pr_number` + a live `GetPRStatus` call, `BacklogStatusEvent` history
(count `in_progress→review→in_progress` cycles for root cause #4), and `ItemSession`
end-times (already the exact query `FindStaleWorkSessions`-equivalent does). Pros: zero
migration, zero risk of stale/duplicated state, "stuck" becomes a pure function of existing
durable facts — trivially restart-survives because there is no cache to invalidate. Cons:
every list/watch call either re-polls GitHub (expensive, rate-limited — bad for a UI that
should refresh often) or reports PR-mergeable staleness only as of the last poller tick;
notify-once dedup (avoiding duplicate notification spam across the 60s ticker) still needs
*some* persisted marker, or every tick re-notifies.

**Recommended split**: use **B** for the durable, restart-surviving, notify-once and
history-preserving part (a new `BacklogStuckState` or extend `BacklogStatusEvent`'s
`triggered_by`/`note` convention with a dedicated small table keyed by `item_id` +
`reason`, one open row per active stuck condition, closed via `resolved_at` when the
reconciler observes the condition clear) — this directly replaces `staleWorkNotified`,
`stuckReviewNotified`, and adds a durable equivalent for the cap-hit and green-PR cases.
Use **C** (query-time) for "is this *specific* PR green-and-mergeable right now" (must
always reflect the latest poll, per requirements: "must positively confirm PR state via
polling... rather than relying on EnablePRAutoMerge's success") — i.e. don't cache the
GitHub-derived verdict itself in the stuck table, only cache "have we already notified for
this instance of stuckness."

This avoids the full migration-heavy option A while giving root cause #4 (bouncing) a
natural implementation: count rows in the new table per item over time, or query
`BacklogStatusEvent` for repeated `in_progress↔review` transitions within a lookback window
— either works without touching `BacklogItem` itself.

## 4. Race conditions to guard against

1. **Reconciler marks "stuck" while a status transition is in flight.** E.g. ticker calls
   `FindStuckReviewItems`, gets item X as a candidate, but between the query and the
   "mark stuck" write, `AutoReopenAfterFailedReview` (triggered by a review verdict landing
   concurrently) transitions X to `in_progress`. Mitigation: mirror
   `TransitionBacklogItemStatus`'s precondition pattern — the "mark stuck" write must be
   conditioned on `status` still matching what the detection query saw (`ExpectedStatus`) or
   equivalently include the `status=?` predicate directly in the stuck-table UPDATE/INSERT's
   WHERE clause, so a concurrent transition either wins (stuck row references a status that
   no longer applies and is ignored/cleared on next tick) or the stuck-write no-ops. Never
   trust "the item was in status X when I queried it 50ms ago."
2. **Un-sticking without clearing the durable flag.** If a durable `resolved_at`-style close
   is added (option B above), it must be set opportunistically at every point that already
   transitions status away from the stuck condition — i.e. `pushAndCreatePR`'s pr_pending
   transition, `ReconcilePRPending`'s merge-detected done transition, and
   `AutoReopenAfterFailedReview`/`AutoReopenForPRFix`'s in_progress transition — not only
   discovered lazily on the next 60s tick (a UI watching a stream would show a stale "stuck"
   badge for up to 60s otherwise; acceptable per NFRs but worth flagging as a design choice,
   not an oversight).
3. **Duplicate notifications across restarts once state is durable.** Today's in-memory
   `map[string]bool` cannot duplicate-notify a nonexistent problem after restart because the
   maps just start empty (silently *under*-notifying, which is root cause #3). Once durable,
   the failure mode flips to *duplicate* notification risk: reconciler must check "already
   have an open stuck-row + already notified" before calling `notify()` again, using the same
   read-then-conditional-write idiom, not a naive "insert row, always notify."
4. **Concurrent reconciler ticks.** The 60s ticker is a single goroutine (no overlap possible
   since it's a `for range ticker.C` loop, not `go func` per tick), so no intra-reconciler
   concurrency exists today — but `TriggerReviewForSession` and `onSessionExited` run
   concurrently with the ticker (both call into the same storage/EntRepository). Any new
   stuck-detection write must be safe against a concurrent legitimate transition from these
   event-driven paths, which is the same concern as #1.

## 5. Wiring point in server/dependencies.go

`backlogLifecycleListener := session.NewBacklogLifecycleListenerWithPool(storage,
headlessPool)` (line 483) is constructed once; `backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})`
(line 484) wires the (currently ephemeral) notifier. The 60s ticker (line 822-836) is the
single integration point requirement explicitly calls out reusing — new stuck-detection
logic should be a new private method on `BacklogLifecycleListener` (e.g.
`reconcileGreenUnmergedPRs`, `reconcileReworkCapStuck`, `reconcileBouncingItems`) called from
inside `ReconcileStuck` alongside the existing five steps, not a second ticker. The rework-cap
case (root cause #2) currently lives in `BacklogService` (server/services/backlog_service_triage.go),
a different package/actor than `BacklogLifecycleListener` — plan should decide whether to (a)
have `BacklogService.notifyReworkCapHit` also write the durable stuck row directly (needs the
new `EntRepository`/`Storage` method), or (b) have `BacklogLifecycleListener`'s ticker detect
"item has hit work-session-count cap and is stuck in review/pr_pending" independently by
querying `ItemSession` counts — (a) is simpler and avoids duplicating the cap-detection logic
in two places, since `BacklogService` already knows the moment the cap is hit.

## 6. ConnectRPC / frontend exposure

Two existing patterns to choose between:

- **UnfinishedWorkService** (proto/session/v1/unfinished.proto, server/services/unfinished_work_service.go):
  has a `stream` RPC, `WatchUnfinishedWork`, whose handler sends an initial snapshot
  (`s.scanner.GetAllResults()`) then subscribes to the shared `eventBus` (`s.eventBus.Subscribe(ctx)`,
  deferred `Unsubscribe`) and forwards matching events for the stream's lifetime
  (unfinished_work_service.go:131-161). This is the "durable snapshot + live tail" idiom
  already used for git-worktree-based unfinished work, and requirements.md explicitly asks
  whether to "extend `/unfinished`... or new one."
- **BacklogService** (proto/session/v1/backlog.proto): currently **request/response only**,
  no streaming RPC at all (`ListBacklogItems`, `GetBacklogItem`, `TransitionBacklogItemStatus`,
  etc. — all unary). Status is a plain `string`, not a proto enum, mirroring the Go
  `BacklogStatus` string type.

Recommendation: add the new capability to **BacklogService**, not UnfinishedWorkService —
stuck-item state is fundamentally backlog-item domain data (status, PR mergeability, rework
count), not git-worktree-disk-state, and `/unfinished`'s scanner has no concept of backlog
item IDs at all (confirmed: baseline description says exactly this). Add a unary
`ListStuckBacklogItems` (simplest, matches BacklogService's existing all-unary style) for
the browsable view, and optionally a `WatchStuckBacklogItems` streaming RPC only if the UI
needs live push — copy the exact `WatchUnfinishedWork` snapshot+subscribe+forward skeleton if so,
reusing `s.eventBus` (BacklogService already holds `s.eventBus`, confirmed in
`notifyReworkCapHit`). Either way this is a `+api:` marker in the handler and a
`docs/registry/features/backend/*.json` entry per `.claude/rules/feature-registry.md`, plus a
new frontend page/component per `.claude/rules/feature-registry.md`'s frontend-feature
convention and an e2e spec per `.claude/rules/e2e-test-conventions.md`.

## 7. Event-Command-Policy table (EventStorming grammar)

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `WorkSessionExited` (all ItemSessions for item ended) | 60s ticker, `ReconcileStuckItems` | `TransitionBacklogItemStatus(item, review)` | Reconciler (BacklogLifecycleListener) |
| `ReviewVerdictRecorded` (PASS) | `spawnReviewGate` callback | `pushAndCreatePR` → `CreatePR` + `EnablePRAutoMerge` | Reconciler → GitHub |
| `PRAutoMergeEnableFailed` (silent, no event today) | **gap**: none — should trigger | **new**: `MarkStuckReason(item, "auto_merge_failed")` | Reconciler |
| `PRPolled: open, CI green, no blocking review, no conflict, not merged` | `ReconcilePRPending` per-item loop | **gap**: `continue` (no-op) today — should be | **new**: `MarkStuckReason(item, "pr_ready_unmerged")` + `Notify` | Reconciler (PR poller) |
| `PRMerged` | `IsPRMerged` returns true | `TransitionBacklogItemStatus(item, done)` + **new**: `ResolveStuckReason(item, "pr_ready_unmerged")` | Reconciler |
| `ReworkCapHit` (workCount ≥ maxAutoReworkIterations) | `AutoReopenAfterFailedReview` / `AutoReopenForPRFix` | `notifyReworkCapHit` (ephemeral today) → should also **new**: `MarkStuckReason(item, "rework_cap")` | BacklogService (triage) |
| `ReviewItemAbandoned` (verdict exists, nothing in flight) | `FindStuckReviewItems` (existing) | `notify` (ephemeral, in-memory dedup) → should also **new**: `MarkStuckReason(item, "abandoned_review")` | Reconciler |
| `WorkSessionStale` (no progress > 2h) | `reconcileStaleWorkSessions` (existing) | `notify` (ephemeral, in-memory dedup) → should also **new**: `MarkStuckReason(item, "stale_work")` | Reconciler |
| `StatusCycleDetected` (item bounced in_progress↔review N times) | **gap**: no policy exists — should be a **new** query over `BacklogStatusEvent` history each tick | **new**: `MarkStuckReason(item, "bouncing")` | Reconciler |
| `StuckConditionCleared` (any of the above transitions away from the stuck status) | Existing transition call sites (pushAndCreatePR→pr_pending, ReconcilePRPending→done, AutoReopen*→in_progress) | **new**: `ResolveStuckReason(item, reason)` | Reconciler / BacklogService |
| `StuckItemListRequested` | User opens stuck-items UI view | `ListStuckBacklogItems` (unary RPC) | Human via ConnectRPC/UI |

## Key files for implementation planning

- `session/backlog_lifecycle.go` — `BacklogLifecycleListener`, `ReconcileStuck` (line 519),
  `reconcileStuckReviewItems` (602), `reconcileStaleWorkSessions` (646), `ReconcilePRPending`
  (807, gap at 874-876), `pushAndCreatePR` (703, auto-merge best-effort at 789).
- `session/ent_repository_backlog.go` — `TransitionBacklogItemStatus` (516), precondition
  pattern to reuse for new stuck-state writes.
- `session/storage_backlog.go` — `ReconcileStuckItems` (430), `FindStuckReviewItems` (519),
  `FindPRPendingItems` (538), `BackfillMissingPRNumbers` (562) — pattern to follow for any new
  `Find*`/`Reconcile*` query method.
- `session/repository.go` — `BacklogItemPrecondition` (423), `BacklogItemUpdate` (406).
- `session/ent/schema/backlog_item.go`, `session/ent/schema/backlog_status_event.go` — schema
  conventions (index on `(status, updated_at)` already present; append-only log pattern with
  `OnDelete(Cascade)`).
- `server/dependencies.go` — listener construction (483), notifier wiring (484), 60s ticker
  (819-836) — the sole integration point for new reconciler logic.
- `server/services/backlog_service_triage.go` — `maxAutoReworkIterations` (55),
  `notifyReworkCapHit` (29), cap-hit call sites (421-423, 482-484).
- `proto/session/v1/unfinished.proto` + `server/services/unfinished_work_service.go:131-161` —
  streaming snapshot+eventBus-tail pattern to copy if a `Watch*` RPC is added.
- `proto/session/v1/backlog.proto` — existing all-unary `BacklogService` RPC surface; add new
  RPC(s) here per the recommendation in §6.
- `pkg/events/bus.go`, `server/services/backlog_notifier.go` — in-memory-only `EventBus`
  confirming root cause #3 (no persistence backs today's notifications).
