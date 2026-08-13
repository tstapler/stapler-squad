# Architecture Research: backlog-event-driven-updates

This extends `project_plans/review-queue-event-driven/research/architecture.md` (read in
full; not re-derived here). That document traced PTY-output → status-cache →
`ReactiveQueueManager` wiring for **polling elimination on a single Instance's detected
status**. This document covers the structurally different problem of **CRUD-style state
transitions on a `BacklogItem` row, triggered by ~20+ independent call sites** (RPC handler
+ many reconcilers), not a single PTY event source.

---

## 1. Exact trace: `ReviewQueueEvent` / `WatchReviewQueue` today

### Publish path

`ReactiveQueueManager` (`server/review_queue_manager.go`) is **not** built on the generic
`pkg/events.EventBus` pub/sub for its own domain event (`sessionv1.ReviewQueueEvent`). It
uses the bus only as an **input** — subscribing to generic `events.Event` (interaction,
acknowledgment, approval-response, session-deleted; `handleEvent`, line 182) to decide when
to re-evaluate the queue — and maintains its **own separate, bespoke fan-out registry** for
output:

```
OnItemAdded/OnItemRemoved/OnQueueUpdated (called by ReviewQueuePoller)
  → builds a *sessionv1.ReviewQueueEvent (line 320, 468, 496)
  → rqm.publishToClients(event)          (line 584)
      → iterates rqm.streamClients map (in-process, NOT the EventBus)
      → per-client eventMatchesFilters() check (line 603)
      → non-blocking send to client.eventCh, buffer 100, DROPS on full (line 584-600)
```

`AddStreamClient` (line 520) / `RemoveStreamClient` (line 571) manage a
`map[string]*reviewQueueStreamClient` guarded by `streamClientsMu` — a hand-rolled
subscriber registry, structurally parallel to but **entirely independent of**
`pkg/events.EventBus`'s `subscribers map[string]chan *Event`.

### RPC handler

`ReviewQueueService.WatchReviewQueue` (`server/services/review_queue_service.go:211`) calls
`reactiveQueueMgr.AddStreamClient(ctx, filters)`, then loops forwarding `eventCh` to the
Connect server-stream until the channel closes or ctx is done. `SessionService.WatchReviewQueue`
(`server/services/session_service.go:2599`) is a thin delegation to this.

### Critical finding: no replay support

`WatchReviewQueueRequest` (`proto/session/v1/session.proto:754-769`) has **no `after_seq`
field**. There is no ring buffer, no `Seq` numbering, no `EventsSince` equivalent for
`ReviewQueueEvent` — unlike `pkg/events.EventBus`, which has all three
(`pkg/events/bus.go:30,68,115`). A client that reconnects to `WatchReviewQueue` gets only
`initial_snapshot` (a full re-fetch), never a gap-filling replay. If the requirement is
genuinely "after_seq-based reconnect/replay mirroring `WatchSessions`" (requirements.md,
Scope), **`BacklogItemEvent`/`WatchBacklogItems` must NOT copy `ReviewQueueEvent`'s
transport pattern** — it must copy `WatchSessions`'s, which is built directly on
`pkg/events.EventBus`.

### `WatchSessions`'s pattern (the one to mirror for transport)

`SessionService.WatchSessions` (`server/services/session_service.go:1991`):
```go
eventCh, subID := s.eventBus.Subscribe(ctx)   // pkg/events.EventBus.Subscribe
defer s.eventBus.Unsubscribe(subID)
if req.Msg.AfterSeq > 0 {
    for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {  // ring-buffer replay
        stream.Send(convertEventToProto(event))
    }
} else {
    // fresh connection: send in-memory snapshot, then live-subscribe
}
```
`pkg/events.EventBus.Publish` (`pkg/events/bus.go:72`) assigns `event.Seq` atomically, appends
to a 1-hour/10k-entry ring buffer (`pruneBuffer`, line 96), then fans out non-blocking to all
subscriber channels. `EventsSince(afterSeq)` (line 115) binary-searches the buffer.

**Recommendation**: `BacklogItemEvent` should be a **new `events.EventType`** (e.g.
`EventBacklogItemChanged`) carried on the *existing* generic `pkg/events.Event` struct
(`pkg/events/types.go:31`) — reusing the same `EventBus`, same `Seq`/`EventsSince`
infrastructure `WatchSessions` already has — rather than inventing a second bespoke
stream-client registry like `ReactiveQueueManager`'s. `WatchBacklogItems` becomes a thin RPC
handler nearly identical to `WatchSessions`, subscribing to the *same* `eventBus` and
filtering for `EventBacklogItemChanged` (analogous to how `convertEventToProto` already
switches on `event.Type`). This also means `BacklogItemEvent`'s payload should be attached to
`Event` the same way `Session *session.Instance` is today — e.g. a new
`BacklogItem *session.BacklogItemData` field plus `UpdatedFields []string` (already generic
enough to reuse as-is, see `pkg/events/types.go:42`).

---

## 2. Call-site enumeration: is there one choke point?

**No single choke point covers the whole backlog item lifecycle — but status transitions
specifically DO have exactly one**, and that is the highest-value, highest-volume case named
in the Feasibility Risk.

### Status transitions: ONE choke point (confirmed)

Every call site that changes `BacklogItem.Status` — regardless of caller — funnels through
exactly one repository method:

```
~20 call sites across:
  server/services/backlog_service_triage.go       (7 call sites: dequeue claim, rollback,
                                                     in-progress transitions, done transition)
  server/services/backlog_service_lifecycle.go    (RPC handler TransitionBacklogItemStatus,
                                                     line 441; rework rollback, line 891)
  server/services/backlog_service_sync.go:145
  server/services/autonomous_orchestration_service.go:412
  server/mcp/tools_backlog.go:382                 (request_review MCP tool)
  session/backlog_lifecycle.go                    (~10 call sites: archival hook,
                                                     ReconcilePRPending done-transitions,
                                                     PR-pending transitions, bounce/reopen)
      ↓ ALL call
  session/storage.go:721  Storage.TransitionBacklogItemStatus(...)  — PURE PASSTHROUGH:
      func (s *Storage) TransitionBacklogItemStatus(...) (*BacklogItemData, error) {
          return s.repo.TransitionBacklogItemStatus(ctx, id, toStatus, precondition)
      }
      ↓
  session/ent_repository_backlog.go:674  EntRepository.TransitionBacklogItemStatus(...)
      — the ONE place the SQL UPDATE actually happens (CAS via Where() clause,
      see the doc comment at line 652-673 explaining the TOCTOU fix from
      BUG-026 / PR #199 — this method already had to become the single hardened
      choke point for correctness reasons unrelated to this project).
```

A single publish hook inside `EntRepository.TransitionBacklogItemStatus` (or one layer up,
in `Storage.TransitionBacklogItemStatus`, which is a name-for-name passthrough so either
location is equivalent) catches **every** status transition from every caller, with zero risk
of a silently-missed call site — because there is structurally nowhere else to write
`BacklogItem.Status` outside this method (it is the only method in `EntRepository` that calls
`.SetStatus(...)`).

### But status is not the whole lifecycle: 5 more independent mutation surfaces

The requirements also name "verdict recording" and "session attach" as events that must
become visible — neither goes through `TransitionBacklogItemStatus`:

| Mutation | Method | File:line | Note |
|---|---|---|---|
| Non-status field edit (title, description, pipeline mode, AC list) | `UpdateBacklogItem` | `session/ent_repository_backlog.go:479` | Separate SQL UPDATE, no `.SetStatus()` |
| Archive | `ArchiveBacklogItem` | `session/ent_repository_backlog.go:589` | Sets `archived_at`, not `status`; called by the done-item retention sweep (`FindDoneItemsOlderThan`) |
| Delete | `DeleteBacklogItem` | `session/ent_repository_backlog.go:612` | Row removal — needs an `ItemRemoved` event, not `ItemUpdated` |
| Review verdict recorded | `SaveReviewVerdict` / `CreateItemSessionWithVerdict` | `session/storage_backlog.go:314`, `:381` | Writes to the `ReviewVerdict`/`ItemSession` tables, **not** the `BacklogItem` row — but the UI eagerly joins `ReviewVerdict` onto the item (`WithReviewVerdict()`, `ent_repository_backlog.go:459`), so a verdict landing must still trigger a push for `BacklogItemDetail` to update |
| Work session attach | `CreateItemSession` / `UpdateItemSessionSessionUUID` | `session/storage_backlog.go:54`, `:207` | Also an `ItemSession`-table write, independent of `BacklogItem` |

**Concrete answer to the Open Question**: there are **6 independent low-level repository
methods** that need a publish hook, not 1 — but within the single largest and most
reconciler-heavy category (status transitions, the one called out by name in the Feasibility
Risk about missing a reconciler call site) there genuinely is exactly one choke point already
hardened for an unrelated correctness reason (BUG-026). The other 5 hooks are each a single,
easily-enumerated line-item (one line of code per method above), not a fan-out risk — because
each is *itself* already a repository-layer choke point for its own category of change (e.g.
all title/description edits already funnel through the one `UpdateBacklogItem`, all verdict
writes already funnel through the one `SaveReviewVerdict`/`CreateItemSessionWithVerdict`
pair). The risk named in the Feasibility Risk section ("touches a wide surface of call
sites... risk of missing a call site") is real at the *RPC-handler-and-reconciler* layer (20+
sites) but does not exist at the *repository* layer (6 sites) — which is exactly why the hook
belongs in the repository layer, not scattered across each of the 20+ callers.

---

## 3. Workspace / multi-instance scoping: concrete answer

**`pkg/events.EventBus` has zero workspace-scoping logic today** — verified by reading
`pkg/events/bus.go` in full. `Subscribe`, `Publish`, and `EventsSince` operate on a single
flat `subscribers map[string]chan *Event` and a single flat ring buffer; there is no
workspace ID, tenant ID, or any filtering field anywhere in the `Event` struct
(`pkg/events/types.go:31-69`) or the bus implementation.

**Scoping is achieved entirely by process isolation, not by the bus.** Per
`.claude/docs/state-isolation.md`, each workspace (by default, SHA256-of-cwd) runs as its own
OS process with its own state directory. `events.NewEventBus(100)` is called exactly twice in
the whole codebase (`server/services/session_service.go:442` and `:456` — two different
bootstrap paths, e.g. production vs. test harness), each producing one `*EventBus` instance
that lives for the lifetime of that one process. There is no cross-process fan-out, no shared
memory, no message broker — a subscriber only ever sees events published by `Publish` calls
made in its own process, which is exactly the events generated by that one workspace's own
`Storage`/`EntRepository`.

**Conclusion**: `WatchBacklogItems` inherits workspace isolation **for free**, identically to
how `WatchSessions` already does — by virtue of subscribing to the same
per-process-singleton `eventBus` that only this workspace's reconcilers ever publish to. No
new scoping code, filtering, or hardening is required. The Feasibility Risk item ("may impose
event-scoping requirements... not yet confirmed") can be closed as **not applicable** — this
should still be captured as an explicit test (per the requirements' "positive verification"
scope item), but the test is confirming an existing property of the shared bus (one bus per
process, one process per workspace), not building new isolation logic.

---

## 4. Event-Command-Policy table (EventStorming grammar)

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| `ItemCreated` (idea) | New backlog item captured | `CreateBacklogItem` | Human operator, or `ItemSource` sync (`backlog_service_sync.go`) |
| `ItemReadied` (idea→ready) | Triage marks item groomed/ready | `TransitionBacklogItemStatus(ready)` | Human operator / triage flow |
| `ItemClaimed` (queued/ready→in_progress) | FIFO dequeue sweep claims next item, or manual "Start" | `TransitionBacklogItemStatus(in_progress, precondition)` | `backlog_service_triage.go` dequeue sweep / human |
| `WorkSessionAttached` | Work session spawned for the claimed item | `CreateItemSession` | `session/backlog_lifecycle.go` spawner |
| `ReviewRequested` (in_progress→review) | Agent calls `request_review`, or completion auto-detected | `TransitionBacklogItemStatus(review)` | Headless work agent |
| `ReviewVerdictRecorded` | Headless reviewer submits PASS/FAIL/PARTIAL | `SaveReviewVerdict` / `CreateItemSessionWithVerdict` | Headless review agent |
| `ItemReopenedForRework` (review→in_progress) | FAIL/PARTIAL verdict → auto-reopen policy fires | `AutoReopenAfterFailedReview` → `TransitionBacklogItemStatus(in_progress)` + new session spawn | `AutoReopenSpawner` (system reconciler) |
| `PRPendingEntered` (review→pr_pending) | PASS verdict + `AutoCreatePR` policy, or manual "Ship PR" | `maybeAutoCreatePR` → `TransitionBacklogItemStatus(pr_pending)` | `ReactiveQueueManager.maybeAutoCreatePR` / human |
| `ItemBounceDetected` (stuck flag, no status change) | Reconciler detects repeated review↔rework cycling | `MarkStuck` | `reconcileBouncingItems` (`session/backlog_lifecycle.go`) |
| `PRMergedItemDone` (pr_pending→done) | Reconciler detects the linked PR merged to main | `TransitionBacklogItemStatus(done)` | `ReconcilePRPending` reconciler |
| `ItemArchived` (done→archived) | Retention sweep, N days after done | `ArchiveBacklogItem` | `FindDoneItemsOlderThan` reconciler |
| `ItemDeleted` | Manual removal | `DeleteBacklogItem` | Human operator |

Every row left of `WorkSessionAttached`/`ReviewVerdictRecorded` needs a `BacklogItemEvent`
push from a *repository*-layer hook (§2); `ItemBounceDetected` is the one row that already
has a push mechanism today (`Notifier`/`EventBusNotifier`, see §5) and should keep using it.

---

## 5. Recommendation: keep `Notifier` and `BacklogItemEvent` as two separate channels

**Do not unify.** Fire both, independently, from the same low-level call sites where both
apply (e.g. `MarkStuck` already can, and should continue to, call `Notifier.Notify` for the
toast; a status-transition hook in `TransitionBacklogItemStatus` separately fires
`BacklogItemEvent` for UI upsert). Rationale:

1. **Different consumers, different guarantees.** `Notifier`/`EventBusNotifier`
   (`server/services/backlog_notifier.go`) feeds `events.EventNotification` →
   `server/notifications/subscriber.go`'s toast/history store, which **coalesces** same-type
   notifications for the same item within a 500ms window (see the comment at
   `backlog_notifier.go:21-28` about the `sessionID:notificationType` coalescing key). That
   collapsing behavior is correct for alerts (you don't want 10 "still bouncing" toasts) but
   would be a bug for `BacklogItemEvent` — a UI must never have a status transition coalesced
   away, or `/backlog` silently shows a stale status forever with no toast to compensate.

2. **Different payload shapes.** `Notifier.Notify(itemID, title, message, notificationType,
   priority int32)` (`session/backlog_lifecycle.go:28-30`) is a flat, human-readable alert
   payload. `BacklogItemEvent` needs `item id + updated fields + full current item` for
   client-side upsert (per requirements.md Scope, mirroring `ReviewQueueEvent`'s shape).
   Cramming both into one method signature means ~90% of calls (routine transitions) carry
   unused title/message fields, and the ~10% (bounce/stuck alerts) carry an unused full-item
   payload nobody asked for.

3. **This repo already has the correct precedent for exactly this situation and it should be
   repeated, not replaced.** `session.Notifier` is a narrow, single-method interface defined
   in the *consumer* package (`session/backlog_lifecycle.go`, not next to
   `EventBusNotifier`'s implementation in `server/services/`) — the exact pattern
   `.claude/rules/interface-pollution-checklist.md` prescribes. Follow the identical shape for
   the new hook: define a narrow `session.ItemChangePublisher` (or similar name) interface
   next to `Notifier` in `session/` with one method, e.g.
   `PublishItemChanged(item *BacklogItemData, updatedFields []string)`; implement an adapter
   (`server/services/backlog_item_event_publisher.go`, parallel to `EventBusNotifier`) that
   converts to `events.NewBacklogItemChangedEvent(...)` and calls `bus.Publish`; wire it via a
   `SetItemChangePublisher` setter from `server/dependencies.go`, right next to the existing
   `backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})` call at
   line 525 — same file, same wiring style, new setter.

4. **This is a 1-ADR decision, cheaply reversible either way** — if in practice most consumers
   turn out to want both streams merged client-side, that's a frontend-only concern (the
   `useWatchBacklogItems` hook can internally subscribe to both `WatchBacklogItems` and read
   existing notification state) without forcing a backend event-type merger.

**Bottom line**: add a second interface + adapter + wiring line, following the `Notifier`
precedent exactly; do not widen `Notifier` or introduce a discriminated "mega-event" that
serves both jobs.
