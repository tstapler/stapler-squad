# Research: Feature Landscape — backlog-event-driven-updates

Agent 2 (Features). Scope: prior art for live-update patterns, complete mutation-surface
enumeration, edge cases/failure modes from existing similar features, and unstated needs.

## 1. Complete Enumeration of Backlog Item Mutation Call Paths

This is the concrete "wide surface of call sites" the requirements flag as a risk. All
line numbers current as of 2026-07-21 on `main`. Test files (`*_test.go`) are excluded.

### 1a. `TransitionBacklogItemStatus` (status transitions)

Repository chain: `session/ent_repository_backlog.go:674` (impl) →
`session/storage.go:721` (`Storage.TransitionBacklogItemStatus`, thin passthrough) →
called from:

| Caller | File:Line | Context |
|---|---|---|
| RPC handler | `server/services/backlog_service_lifecycle.go:527` | Manual `TransitionBacklogItemStatus` RPC — the human-initiated path |
| Triage claim/rollback | `server/services/backlog_service_triage.go:515` | rollback to `queued` on claim failure |
| Triage → in_progress | `server/services/backlog_service_triage.go:717,746,1022,1269` | multiple entry points moving item to `in_progress` |
| Triage rollback → review | `server/services/backlog_service_triage.go:1063` | rollback path |
| Triage rollback → pr_pending | `server/services/backlog_service_triage.go:1322` | rollback path |
| Triage misc | `server/services/backlog_service_triage.go:1508,1667` | |
| Triage → done | `server/services/backlog_service_triage.go:2041` | autonomous completion |
| Autonomous orchestration | `server/services/autonomous_orchestration_service.go:412` | autonomous pipeline mode transitions, explicitly bypasses the manual RPC |
| Sync reconciler | `server/services/backlog_service_sync.go:145` | GitHub sync moving item to `in_progress` |
| MCP tool | `server/mcp/tools_backlog.go:382` | MCP-driven transition to `review` |
| Lifecycle reconciler (generic) | `session/backlog_lifecycle.go:736` | shared helper used by multiple reconcilers |
| Archival on terminal status | `session/backlog_lifecycle.go:1164` | → `archived` |
| PR-pending → done (x2 near-duplicate) | `session/backlog_lifecycle.go:2154,2174` | `ReconcilePRPending` |
| Done via merged-PR detection | `session/backlog_lifecycle.go:2480` | |
| → pr_pending | `session/backlog_lifecycle.go:2634,2681` | |
| Bouncing sweep → done | `session/backlog_lifecycle.go:3152` | `reconcileBouncingItems` (recently touched by commit `5d6bcfb1`) |

**Takeaway:** roughly 20 call sites across 6 files. The RPC handler
(`backlog_service_lifecycle.go:527`) is only ONE of them — most transitions originate in
background reconcilers (`session/backlog_lifecycle.go`, `backlog_service_triage.go`,
`autonomous_orchestration_service.go`, `backlog_service_sync.go`) that call
`storage.TransitionBacklogItemStatus` directly, bypassing the RPC handler entirely. **This
confirms the requirement's core premise**: event publication must live in
`EntRepository.TransitionBacklogItemStatus` itself (or `Storage.TransitionBacklogItemStatus`
as a thin wrapper), not in the RPC handler, or the majority of real-world transitions will
never emit an event.

### 1b. Other status-adjacent repository mutations (`session/repository.go:157-179`)

| Method | Impl | Callers (non-test) |
|---|---|---|
| `UpdateBacklogItem` | `ent_repository_backlog.go:479` | `backlog_service_lifecycle.go:275,553,604`; `backlog_service_triage.go:413,1302,1314,1660`; `backlog_debug_seed_handler.go:169`; `mcp/tools_backlog.go:699`; `session/backlog_sync.go:285`; `session/backlog_lifecycle.go:1017,2449,2589,2963,3070,3181` — general field updates (description, plan, AC snapshot, pipeline mode, etc.), NOT status |
| `ArchiveBacklogItem` | `ent_repository_backlog.go:589` | `backlog_service_lifecycle.go:308` (only caller — manual archive RPC) |
| `DeleteBacklogItem` | `ent_repository_backlog.go:612` | `backlog_service_lifecycle.go:338` (only caller — manual delete RPC) |
| `AppendProgressNote` | `ent_repository_backlog.go:1056` | `mcp/tools_backlog.go:312` (only caller — MCP progress-note tool) |
| `UpdateAcCriterionStatus` | `ent_repository_backlog.go:795` (via `storage.go:981`) | `mcp/tools_backlog.go:304` (only caller) |
| `MarkStuck` | `ent_repository_backlog.go:788` | `autonomous_orchestration_service.go:294`; `backlog_service_triage.go:112,150`; `session/backlog_lifecycle.go:1126,1603,1813,1992,2202,2510,3258` — 6+ distinct stuck-reason reconcilers |
| `ResolveStuck` | `ent_repository_backlog.go:859` | `autonomous_orchestration_service.go:487`; `backlog_service_lifecycle.go:52`; `backlog_service_triage.go:1030,1033,1277,2019,2105`; `session/backlog_lifecycle.go:1716` |

### 1c. Review verdict recording

| Function | Impl | Callers |
|---|---|---|
| `SaveReviewVerdict` | `session/storage_backlog.go:314` (via `storage.go:972`) | `backlog_service_lifecycle.go:755` (RPC path); `mcp/tools_backlog.go:505` (MCP path) |
| `CreateItemSessionWithVerdict` | `session/storage_backlog.go:381` (via `storage.go:1016`) | `backlog_service_lifecycle.go:858`; `backlog_service_triage.go:1996` (autonomous headless review); `session/backlog_review.go:577` (`recordTerminalReviewVerdict` / `RecordDegradedReviewVerdict` — the headless-review degrade path) |

### 1d. Session attach / lifecycle (item ↔ tmux session linkage)

| Function | Impl | Callers |
|---|---|---|
| `UpdateItemSessionSessionUUID` | `storage_backlog.go:207` (via `storage.go:1039`) | attach point when a spawned session gets its tmux UUID |
| `UpdateItemSessionStarted` | `storage_backlog.go:191` (via `storage.go:904`) | `session/backlog_lifecycle.go:673` |
| `UpdateItemSessionEnded` | `storage_backlog.go:223` (via `storage.go:922`) | **13 call sites** across `backlog_service_query.go`, `backlog_service_lifecycle.go`, `backlog_service_triage.go` (9 of the 13), `backlog_review.go`, `backlog_lifecycle.go` — session end/cleanup on nearly every terminal/cleanup path |
| `UpdateItemSessionGitActivity`, `UpdateItemSessionFileTouch`, `UpdateItemSessionTriageResult`, `UpdateItemSessionVerificationNotes` | `storage_backlog.go` | progress-tracking mutations during an active session; not yet grepped exhaustively — flag for Phase 3 to re-verify before implementation since these may also warrant events (e.g. live git-activity ticks) even though the requirement's "routine status transition" language may exclude them |

### 1e. Summary for Phase 3

The **minimum** surface that must publish `BacklogItemEvent` to satisfy the requirement's
"status transition, verdict recording, or session attach" success metric is:
`TransitionBacklogItemStatus`, `SaveReviewVerdict`, `CreateItemSessionWithVerdict`,
`UpdateItemSessionSessionUUID`. The **stuck-state** methods (`MarkStuck`/`ResolveStuck`)
already have a separate notification path (`Notifier`, see §3) and are the concrete case
the ADR needs to resolve (fold into `BacklogItemEvent` or keep separate). `UpdateBacklogItem`
also changes item state (fields visible in the UI, e.g. plan text, AC snapshot) — Phase 3
should decide whether generic field updates warrant their own event variant or ride along
under a generic "item updated" event type, mirroring `ReviewQueueItemUpdatedEvent`'s
`updated_fields` list.

---

## 2. Prior Art: `WatchReviewQueue` / `ReviewQueueEvent` (the mirror target)

This is the closest and most directly reusable precedent, and the requirements name it
explicitly. Full backend implementation: `server/review_queue_manager.go`
(`ReactiveQueueManager`, ~800 lines).

**Architecture:**
- `ReactiveQueueManager` holds `eventBus *events.EventBus` (the same `pkg/events.EventBus`
  used for `WatchSessions`) AND its own internal per-client fan-out
  (`AddStreamClient`/`RemoveStreamClient`/`publishToClients`, `server/review_queue_manager.go:520,571,584`).
  It both **subscribes to** the generic bus (`Start()`, line 149, for session-state-driven
  queue changes) and **maintains a separate list of `ReviewQueueEvent` stream clients** that
  it pushes directly to (bypassing the generic bus for the queue-specific protobuf event
  type). `eventMatchesFilters` (line 603) applies per-connection filters before dispatch.
- Domain-specific proto: `ReviewQueueEvent` (`proto/session/v1/events.proto:317`) — a oneof
  of `ItemAdded`/`ItemRemoved`/`ItemUpdated`/`Statistics`, each carrying a `trigger` string
  and (for `ItemAdded`) an `is_snapshot bool` so the frontend can suppress notification
  toasts for replay/snapshot items vs. genuinely new items.
- RPC: `rpc WatchReviewQueue(WatchReviewQueueRequest) returns (stream ReviewQueueEvent)`
  (`proto/session/v1/session.proto:754`) — filters: `priority_filter`, `reason_filter`,
  `include_statistics`, `initial_snapshot bool`, `session_ids` (scope to specific items).

**Frontend hook**: `web-app/src/lib/hooks/useReviewQueue.ts` (~480 lines) is the fully
worked reference implementation for `useWatchBacklogItems`. Key patterns worth copying
directly:
- **Hybrid push+pull**: WebSocket stream is primary; a 30s fallback poll (`fallbackPollInterval`)
  provides eventual consistency and also acts as the trigger to attempt WS reconnect once
  the stream has exhausted retries (`streamDeadRef`).
- **Exponential backoff with cap**: `Math.min(1000 * 2^retries, 30000)`, `MAX_RETRIES = 5`
  (lines 289, 318-319).
- **Abort-signal race guard**: the reconnect `setTimeout` re-checks `signal.aborted` right
  before calling `connect()` again — without this, an old effect's stale reconnect timer can
  race a new effect's `connect()` on filter/unmount changes (explicit comment "F5" at line 322).
- **Always do an immediate REST fetch on mount** even in push mode, because the WS stream's
  `initialSnapshot` may take a moment to arrive and would otherwise leave the UI empty
  (lines 363-370).
- **Ref-wrapped event handler** (`handleReviewQueueEventRef`) so the WS effect's dependency
  array doesn't include the handler and doesn't cause stream reconnects on every render.
- Redux slice `reviewQueueSlice.ts` holds normalized state; `addItem`/`updateItem`/`removeItem`
  reducers are the "apply this event to my list" logic Agent 2 flags as needed for
  `/backlog` and `/backlog/board` (today those pages hold local `useState` item arrays, not
  Redux — see §4).

## 3. Prior Art: `WatchSessions` / `SessionEvent` (the older, still-active precedent)

Implementation: `server/services/session_service.go:1991` (`SessionService.WatchSessions`).

**Reconnect/replay pattern (the more mature of the two)**: `WatchSessionsRequest.after_seq`
(`proto/session/v1/session.proto:646-649`) — the client remembers the highest `seq` it has
received; on reconnect it passes `after_seq` and the server replays buffered events via
`eventBus.EventsSince(afterSeq)` (`pkg/events/bus.go:115`) before going live, with an explicit
comment that "events are retained for up to one hour." This is a **stronger reconnect
guarantee than `ReviewQueueEvent`'s "just take a fresh snapshot"** approach — it avoids a
gap where events published exactly during the reconnect handshake window are lost. **This
is the correct pattern to adopt for `WatchBacklogItems`**, not the review-queue's rougher
snapshot-based approach, since backlog items can be mutated by a burst of reconciler
activity in the seconds a reconnecting tab is offline.

**Ordering-safety comment worth copying verbatim into the new handler**: `session_service.go:1996-1997`
— "Subscribe before building the snapshot so no events are lost between the two phases
(snapshot races are resolved by client-side upsert semantics)." I.e. subscribe to the bus
FIRST, then build/send the initial snapshot; the client's reducer must treat both snapshot
and live events as idempotent upserts so a duplicate (item arrives in both snapshot and an
in-flight live event) is harmless.

**Event type**: `SessionEvent` (`proto/session/v1/events.proto:10`) carries a monotonic
`seq` field explicitly for this purpose (line 29-33 doc comment).

## 4. Current Frontend State: No Push, No Shared Hook

- `/backlog` list page (`web-app/src/app/backlog/page.tsx:238-240`): `useEffect(() => { void load(); }, [load])` —
  fetches once on mount via `useBacklogService().listBacklogItems`, local `useState<BacklogItem[]>`
  (line 169). No polling, no push, no refetch on window focus.
- `/backlog/board` (`BacklogBoard.tsx`): does not call `useBacklogService` itself — items are
  passed down as props from a parent, so it inherits whatever staleness the parent has (need
  to confirm which page hosts `/backlog/board` and whether it independently fetches; Phase 3
  should verify this to avoid assuming the list page's `load()` also drives the board).
- `BacklogItemDetail.tsx` `shouldPoll` (line 245): `(triageStatus === "running" || (status === "review" && (!gateVerdict || gateVerdict === "PENDING")) || status === "pr_pending") && !editMode` →
  5s interval calling `load()`. Explicitly does **not** poll during `in_progress` — the
  single most common state for a long-running autonomous item — which is exactly the gap
  the requirements call out. Also explicitly suspends while `editMode` is true "so a
  background refresh can't clobber unsaved edits" (line 243 comment) — **this exact
  suppression behavior must be preserved** when replacing polling with push: an incoming
  `BacklogItemEvent` must not clobber an open edit form either.

## 5. Existing Alert/Notification Path — `Notifier` / `EventBusNotifier`

`session.Notifier` interface (`session/backlog_lifecycle.go:28`) has exactly one production
adapter, `EventBusNotifier` (`server/services/backlog_notifier.go`), which wraps
`pkg/events.NewNotificationEvent` and publishes to the **same** `pkg/events.EventBus` that
`WatchSessions` reads from — i.e. alert-worthy backlog conditions already ride the generic
session event bus as `NotificationEvent`s, not a backlog-specific channel. Call sites:
`session/backlog_lifecycle.go:538`, `session/review_gate.go:161,199,237` — all "stuck",
"bouncing", "abandoned review" style conditions, not routine transitions. The
`EventBusNotifier.Notify` doc comment (lines 21-28) already documents a real bug that was
fixed: threading `itemID` as the event's `sessionID` (not just into metadata) so the
notification-history coalescing key (`sessionID:notificationType`,
`server/notifications/subscriber.go`) doesn't collide across different backlog items within
the same coalescing window. **This is directly relevant prior art for a bug class the new
`BacklogItemEvent` design must avoid**: any coalescing/deduplication key design must include
the item ID, not assume a global or session-only key space.

**ADR input**: the requirement asks whether to fold `Notifier`'s alert-condition events into
the new `BacklogItemEvent` model. Given `Notifier` already publishes on the *same* bus
type (`pkg/events.EventBus`) that `WatchSessions` uses, and the new `BacklogItemEvent`
stream will presumably use a **separate** bus/manager (mirroring `ReviewQueueEvent`'s
`ReactiveQueueManager` pattern, not reusing `pkg/events.Event`), folding them would require
either (a) dual-publishing stuck/bounce conditions to both buses, or (b) migrating
`Notifier`'s conditions onto the new `BacklogItemEvent` stream and having toast-worthy
conditions in the frontend key off specific `BacklogItemEvent` variants instead of
`NotificationEvent`. Recommend Phase 3's ADR treat this as the single highest-leverage
design decision, since it determines whether the frontend needs to listen to one stream or
two for full backlog awareness.

## 6. Edge Cases Surfaced by Existing Features (apply directly to the new stream)

| Edge case | How `WatchSessions`/`WatchReviewQueue` solved it | Applicability to `BacklogItemEvent` |
|---|---|---|
| Reconnect after network blip | `after_seq` replay from a 1-hour retained buffer (`WatchSessions`); review queue uses fresh snapshot + fallback poll instead | Adopt `after_seq`-style replay — backlog items can bounce rapidly (see `reconcileBouncingItems`, commit `5d6bcfb1`) during exactly the kind of few-second gap a replay buffer covers |
| Out-of-order / duplicate events | Client reducers treat snapshot + live events as idempotent **upserts** keyed by ID; explicit design note in `session_service.go:1996-1997` | `BacklogItemEvent` payload should carry the full updated item (not a delta) so an out-of-order or duplicate event is a no-op overwrite, not a corrupting partial merge |
| Event for an item not in the client's current list | `ReviewQueueItemAddedEvent` has explicit `trigger` + `is_snapshot` fields so the frontend knows whether to animate/toast; `addItem` reducer handles "wasn't there before" by inserting | For `/backlog` with active filters, an event for an item outside the current filter should NOT force-insert it into a filtered view — mirrors the "should it disappear/appear live" question in the task brief; recommend: update the underlying normalized store always, let the *filtered selector* decide visibility (same pattern `reviewQueueSlice` uses for priority/reason filters) |
| Rapid status bounce (multiple events for the same item within the render cycle) | Redux batches action dispatch; `ItemUpdated` events are last-write-wins on the normalized slice | No special debouncing found in prior art — each event is dispatched as it arrives and React/Redux batching absorbs the churn. Recommend the same "no debounce, rely on last-write-wins" approach rather than adding complexity, unless perf testing shows otherwise |
| Multiple tabs on the same item | Not explicitly tested in existing review-queue/session code — the generic pub/sub design means every connected `WatchSessions`/`WatchReviewQueue` client (each browser tab opens its own stream + subscription) receives the same broadcast; no per-tab coordination needed since the EventBus fans out to N subscribers already | Same fan-out model applies directly; the only novel work is verifying `BacklogItemEvent`'s manager does independent per-client subscriptions like `ReactiveQueueManager.AddStreamClient`, not a single shared subscription that would only deliver to the first tab |
| Workspace / multi-instance scoping | Each `STAPLER_SQUAD_INSTANCE` is a fully separate OS process with its own in-memory `EventBus` (`.claude/docs/state-isolation.md`); no cross-instance leak by construction. The *within-process* "workspace switcher" (`WorkspaceService.SwitchDatabase`, `server/services/session_service.go:3033`) explicitly **restarts the server process** on switch (per its doc comment), which also resets the EventBus — so no live cross-workspace leak was found for that path either | Positive verification for Phase 3: confirm `MergeDatabase` (`session_service.go:3041`, copies sessions from a source workspace into the current DB) does NOT emit live `BacklogItemEvent`s for the merged-in items during a bulk copy — that's a plausible unverified leak path distinct from `SwitchDatabase`'s restart-based isolation |

## 7. Unstated Needs

- **Visual "just changed" affordance is precedented and expected.** `ReviewQueueItemAddedEvent.is_snapshot`
  exists specifically so the frontend can distinguish "this was already there" from "this
  just happened" and avoid firing a toast for the former (comment: "Frontend should NOT fire
  notifications for snapshot items to prevent duplicates," `events.proto:339`). The new
  `BacklogItemEvent` needs the equivalent flag if any flash/highlight or toast is desired on
  arrival — a bare "item changed" push with no snapshot/live distinction would flash every
  item on every page load/reconnect, which is almost certainly not wanted.
- **The existing toast system (`NotificationContext`/`NotificationToast`) already covers
  "something happened" awareness for alert-worthy conditions** (stuck, bouncing) via
  `Notifier`. For *routine* status transitions (e.g. `ready → in_progress`), a toast for
  every transition would almost certainly be too noisy — the more likely correct UX is a
  silent/subtle in-place update (row re-renders with new status badge, maybe a brief
  background-color flash) with toasts reserved for the alert-worthy subset `Notifier`
  already owns. This argues for **not** collapsing all `BacklogItemEvent`s into the
  toast-worthy `NotificationEvent` model, informing the ADR in §5 toward keeping them as two
  purposes even if they share transport.
  - Confirm at Phase 3/ADR time: does the operator want a flash *specifically for the item
    they currently have open in `BacklogItemDetail`* (higher signal, they're watching it) vs.
    just an in-place update in list/board views where the item is one of many rows? Prior art
    doesn't have a direct analogue for "detail view flash" — `useReviewQueue` has no
    single-item detail view, so this is a genuinely new interaction, not a copy from existing
    code.
- **Edit-mode suppression must extend to the new mechanism.** `BacklogItemDetail.tsx`'s
  current polling explicitly disables refresh while `editMode` is true. Replacing polling
  with a push subscription must preserve this: an incoming live-updated item while the user
  is mid-edit should not silently overwrite the form. `useReviewQueue`'s reducers always
  apply updates unconditionally (there's no analogous "user is editing this row" concept in
  the review queue, since it has no inline edit mode) — so this specific guard has no
  existing precedent to copy and needs new design in Phase 3, likely: buffer/withhold the
  live update and show a "this item changed elsewhere" banner rather than either silently
  discarding it or clobbering the form.
