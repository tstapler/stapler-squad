# Implementation Plan: backlog-event-driven-updates

**Feature**: Push-based, per-item real-time updates for `/backlog`, `/backlog/board`, `BacklogItemDetail`, and `BacklogItemPanel`, replacing fetch-once/narrow-polling with a new `BacklogItemEvent`/`WatchBacklogItems` stream built on the existing `pkg/events.EventBus`.
**Date**: 2026-07-21
**Status**: Ready for implementation
**ADRs**: ADR-001 (`project_plans/backlog-event-driven-updates/decisions/ADR-001-separate-notifier-and-backlog-item-event-channels.md`)

---

## Step 0.5: Creative Pass — Alternatives Considered for Overall Event Shape

1. **One generic `BacklogItemUpdatedEvent` with a flat `updated_fields: repeated string` list** (single oneof arm for every mutation kind).
   - Strength: smallest proto surface — one message, one RPC handler branch, one frontend case.
   - Weakness: `verdict recorded` and `session attached` aren't "field diffs" on `BacklogItem` at all (they live in `ReviewVerdict`/`ItemSession` tables per architecture.md §2) — forcing them through a generic field-list either fabricates fake field names or loses the information the UI needs (e.g. "show verdict badge now") without a type-safe payload.

2. **Per-concern typed oneof variants** (`BacklogItemStatusChangedEvent`, `BacklogItemVerdictRecordedEvent`, `BacklogItemSessionAttachedEvent`, `BacklogItemUpdatedEvent`, `BacklogItemArchivedEvent`, `BacklogItemRemovedEvent`), mirroring `ReviewQueueEvent`'s oneof-of-sub-messages shape (stack.md §3).
   - Strength: each event's payload is exactly what its consumer needs (compile-time exhaustive `switch` on the frontend, same "missing case is a compile error" guard this repo already relies on for `OmnibarAction`/`dispatch.ts`); matches existing proto convention (`SessionEvent`, `ReviewQueueEvent`) exactly.
   - Weakness: more proto messages and more RPC-handler/frontend-switch surface than option 1.

3. **Reuse `SessionEvent`/`WatchSessions` directly, adding a backlog-specific oneof variant to the existing session-shaped proto.**
   - Strength: zero new RPC, zero new frontend subscription hook — one stream already wired everywhere.
   - Weakness: already explicitly rejected in requirements.md's Alternatives Considered ("keeping backlog concerns out of the session-shaped proto") — conflates two unrelated domains in one wire type, and couples every session-stream consumer to backlog-shaped payloads it doesn't need.

**Chosen: Option 2 (per-concern typed oneof variants) on a new `BacklogItemEvent` message**, transported over the *existing* `pkg/events.EventBus` (not a new bespoke manager — see Pattern Decisions and the Synthesis Note below). Options 1 and 3 are recorded as rejected in the Pattern Decisions table.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogItemEvent` | New proto message: a `timestamp` + `oneof event` discriminated union carrying one backlog-domain change per instance. | `proto/session/v1/events.proto`, mirrors `ReviewQueueEvent`'s shape (lines 312–382). |
| `BacklogItemStatusChangedEvent` | Oneof variant: item id, old status, new status, full `BacklogItem`, `is_snapshot`. | Fired from `TransitionBacklogItemStatus`. |
| `BacklogItemVerdictRecordedEvent` | Oneof variant: item id, `ReviewVerdict`, full `BacklogItem`. The `verdict` field is populated explicitly and directly from the just-written `session.ReviewVerdictData` (see `BacklogItemChange.Verdict` / `BacklogItemEventPayload.Verdict` below) — the event is self-contained; the frontend does **not** need to join against `item_sessions` to learn "the new verdict" for this event (that client-side join, per features.md §1c, describes today's pre-feature behavior, which this event supersedes for the verdict-recorded case). | Fired from `SaveReviewVerdict`/`CreateItemSessionWithVerdict`. |
| `BacklogItemSessionAttachedEvent` | Oneof variant: item id, session id/UUID, full `BacklogItem`. | Fired from `CreateItemSession`/`UpdateItemSessionSessionUUID`. |
| `BacklogItemUpdatedEvent` | Oneof variant: item id, `updated_fields: repeated string`, full `BacklogItem`. Generic non-status field edits (title, description, AC list, plan text). | Fired from `UpdateBacklogItem`, **and** from `UpdateItemSessionTriageResult` with `updated_fields: ["triageResultSummary"]` (Story 2.2.5) — triage-progress writes reuse this variant rather than getting a dedicated proto message, since they're a field update on the item's derived state, not a distinct shape. |
| `BacklogItemArchivedEvent` | Oneof variant: item id, `archived_at`. | Fired from `ArchiveBacklogItem`. |
| `BacklogItemRemovedEvent` | Oneof variant: item id, reason. Distinct from "updated" — the client must delete, not upsert. | Fired from `DeleteBacklogItem`. |
| `is_snapshot` | Bool on every oneof variant; true when the event is part of the initial catch-up snapshot on connect/reconnect, false for a genuinely real-time occurrence. | Copied verbatim from `ReviewQueueItemAddedEvent`'s convention (stack.md §3). Drives "should this flash/highlight" on the frontend. |
| `WatchBacklogItemsRequest` | New RPC request message: optional status/category filter fields + `after_seq: uint64` (reconnect replay cursor), mirroring `WatchSessionsRequest.AfterSeq`. | `proto/session/v1/session.proto`. |
| `WatchBacklogItems` | New ConnectRPC server-streaming RPC on `BacklogService`, signature mirrors `SessionService.WatchSessions` (`server/services/session_service.go:1991`). | `server/services/backlog_service_events.go` (new file). |
| `events.EventType` | Existing Go enum (`pkg/events/types.go:31`-adjacent) identifying what kind of generic `Event` a bus message is. | Gets one new constant: `EventBacklogItemChanged`. |
| `events.Event` | Existing generic pub/sub envelope struct carried by `pkg/events.EventBus`; today carries a `Session *session.Instance` field for session events. | Gets a new `BacklogItem *session.BacklogItemData` field + `BacklogEventPayload` (holds which oneof variant + its fields) so the same envelope type serves both domains, per architecture.md's recommendation. |
| `pkg/events.EventBus` | Existing hand-rolled, dependency-free pub/sub bus (`pkg/events/bus.go`) — ring-buffer replay (`Seq`/`EventsSince`), non-blocking fan-out. Reused unchanged; no new bus/manager is built. | Two live instances today (`session_service.go:442`, `:456`), one per workspace process. |
| `session.ItemChangePublisher` | New narrow interface (one method: `PublishItemChanged(item *BacklogItemData, change BacklogItemChange)`), defined in the `session` package next to the existing `Notifier` interface (`session/backlog_lifecycle.go:28`). | Solves the "storage can't import `pkg/events` directly" cycle (stack.md §5) exactly the way `Notifier` already solves it for alerts. |
| `BacklogItemEventPublisher` | New adapter struct in `server/services` implementing `ItemChangePublisher`; converts a repository-layer change into an `events.Event{Type: EventBacklogItemChanged, ...}` and calls `bus.Publish`. | Parallel to the existing `EventBusNotifier` (`server/services/backlog_notifier.go`). |
| `BacklogItemChange` | New small Go struct/enum in `session` package describing which kind of change occurred (status/verdict/session-attach/updated/archived/removed) plus the fields needed to build the corresponding oneof variant. | Consumer-defined shape so `ItemChangePublisher` stays narrow and typed, not stringly-typed. |
| `EntRepository.TransitionBacklogItemStatus` | Existing repo method (`session/ent_repository_backlog.go:674`) — the single hardened choke point (CAS via `Where()`, doc comment lines 652–673) for every status transition from ~20 call sites. | Gets the first and highest-value publish hook. |
| `Storage.TransitionBacklogItemStatus` | Existing thin passthrough (`session/storage.go:721`) wrapping the repo method — holds no CAS logic of its own. | Confirmed via grep: the repo method (`*EntRepository`) is the correct hook location, not `Storage`. `Storage` additionally gets a *forwarding* `SetItemChangePublisher` method (Task 1.3.3a) so `server/dependencies.go` can wire the publisher without depending on the concrete `*EntRepository` type — see `Storage.GetEntClient()` (`session/storage.go:234-239`) for the exact type-assertion-forwarding precedent this follows. |
| `UpdateBacklogItem` / `ArchiveBacklogItem` / `DeleteBacklogItem` | Existing repo methods (`ent_repository_backlog.go:479`, `:589`, `:612`) — each its own independent choke point for its category of change. | Each gets its own publish hook (architecture.md §2). |
| `SaveReviewVerdict` / `CreateItemSessionWithVerdict` | Existing storage methods (`session/storage_backlog.go:314`, `:381`) writing to the `ReviewVerdict`/`ItemSession` tables. | Each gets a publish hook emitting `BacklogItemVerdictRecordedEvent`. |
| `CreateItemSession` / `UpdateItemSessionSessionUUID` | Existing storage methods (`session/storage_backlog.go:54`, `:207`) for work-session attach. | Each gets a publish hook emitting `BacklogItemSessionAttachedEvent`. |
| `UpdateItemSessionTriageResult` | Existing storage method (`session/storage_backlog.go:276`, thin passthrough `Storage.UpdateItemSessionTriageResult` at `session/storage.go:880`) — writes the triage-result JSON payload onto an `ItemSession` mid-triage-run; called from `backlog_service_triage.go:1652` and `mcp/tools_backlog.go:705`. Confirmed via grep (features.md §1d flagged this method as needing re-verification before implementation; pre-mortem.md P1 #5). | Gets its own publish hook, kind `ChangeTriageProgressUpdated` / `BacklogChangeTriageProgressUpdated`, wired onto the *existing* `BacklogItemUpdatedEvent` proto oneof variant (Task 1.1.1b) rather than a new proto message — a triage-result write is a field update on the item's derived state (`BacklogItemData.TriageResultSummary`, `session/repository.go:302`), which `BacklogItemUpdatedEvent`'s `updated_fields` shape already models exactly. See Story 2.2.5. This closes the Success Metrics gap requirements.md flags ("no regression in ... triage progress") that Story 5.3.1's `shouldPoll` removal would otherwise reopen. |
| `Notifier` / `EventBusNotifier` | Existing alert-condition channel (`session/backlog_lifecycle.go:28`, `server/services/backlog_notifier.go`) for bouncing/stuck conditions. Publishes `NotificationEvent`s to the *same* `EventBus`, but is kept as a deliberately separate, narrower channel — not unified with `BacklogItemEvent` (ADR-001). `Notifier` is wired via `BacklogLifecycleListener.SetNotifier` (`session/backlog_lifecycle.go:522`, called from `server/dependencies.go:525`) — a **different struct** from `*EntRepository`/`*Storage`, and `BacklogLifecycleListener` does not own any of the 9 hooked mutation methods. This is a distinct wiring point from `ItemChangePublisher`, not a template for it — see `Storage.GetEntClient()` for `ItemChangePublisher`'s actual precedent. | Unchanged by this project except that both channels now fire from some of the same call sites. |
| `useWatchBacklogItems` | New shared frontend hook (`web-app/src/lib/hooks/useWatchBacklogItems.ts`), structurally mirrors `useReviewQueue.ts` (AbortController, exponential backoff, `for await` consumption, REST fallback) but adds `after_seq` tracking + forward/backward gap detection (pitfalls.md #1, #4) that neither existing hook has. | Consumed identically by all 4 views. |
| `backlogItemsSlice` | New Redux slice (`web-app/src/lib/store/backlogItemsSlice.ts`), normalized `Record<id, BacklogItem>` map + `addItem`/`upsertItem`/`removeItem` reducers, mirroring `sessionsSlice.ts`'s shape. | See Pattern Decisions for why Redux (not local `useState`) is chosen despite today's list/board using local state. |
| `upsertBacklogItem` reducer | The reducer function inside `backlogItemsSlice` that applies an incoming event to the store — includes a **real** `incoming.updatedAt < existing.updatedAt ⇒ drop` staleness guard (or `seq`-based if threaded through), not `sessionsSlice.upsertSession`'s equal-only check (pitfalls.md #2). | Unit-tested directly (Phase 7). |
| `lastSeqRef` / `needsFullResyncRef` | Refs inside `useWatchBacklogItems`, same names/roles as `useSessionService.ts:730-742`'s backwards-jump detector, PLUS new forward-gap detection (`event.seq !== lastSeq + 1 ⇒ resync`) that `useSessionService.ts` does not have today (pitfalls.md #4). | New logic, not a copy. |
| Edit-mode buffering | UX behavior: while `BacklogItemDetail` is in `editMode`, an incoming live event for the open item is held (not applied) and a non-blocking banner is shown instead of overwriting the form. | Replaces the current polling suspension at `BacklogItemDetail.tsx:245`. |
| `InlineNotice` | New (or extended) UI component reusing `InlineError`'s informational styling for the "this item changed elsewhere" banner. | ux.md §4. |
| `ConnectionIndicator` | Small persistent connection-state affordance ("Live"/"Reconnecting…") shown wherever `useWatchBacklogItems` is active. | ux.md §5 recommendation #7; component built in Task 6.2.1b, mounted in Task 6.2.1c. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Transport mechanism | Publish-Subscribe (Observer, GoF) over the existing `pkg/events.EventBus`; new `EventType` + payload field on the existing generic `Event` struct | architecture.md, stack.md, pitfalls.md (independent convergent recommendation); overrides build-vs-buy.md — see Synthesis Note below | Bespoke per-client stream registry copying `ReactiveQueueManager`'s `streamClients` map (build-vs-buy.md's literal recommendation) | `ReactiveQueueManager` has zero `Seq`/`after_seq`/replay capability (confirmed independently by architecture.md, stack.md, and pitfalls.md); requirements.md explicitly wants `WatchSessions`-equivalent reconnect/replay, which only the `EventBus` transport provides |
| Event payload shape | Per-concern typed `oneof` variants (`BacklogItemStatusChangedEvent`, etc.), type-driven design — illegal/ambiguous states (e.g. a "verdict" event with no verdict field) unrepresentable | Step 0.5 Option 2; `ReviewQueueEvent`'s proto shape | Option 1: single generic `BacklogItemUpdatedEvent` with `updated_fields: repeated string` for everything | Verdict-recorded and session-attached aren't field diffs on `BacklogItem` at all (architecture.md §2 table); a generic field list either fabricates field names or drops information the UI needs |
| Event payload shape (2nd alt) | (same as above) | requirements.md Alternatives Considered | Option 3: reuse `SessionEvent`/`WatchSessions` with a backlog oneof variant | Already explicitly rejected in requirements.md: keeps backlog concerns out of the session-shaped proto |
| Storage → bus decoupling | Adapter pattern (GoF): narrow `session.ItemChangePublisher` interface defined in the **consumer** package (`session`), implemented by a `server/services` adapter | architecture.md §5; `.claude/rules/interface-pollution-checklist.md` | Direct import of `pkg/events` from `session`/storage packages | Import cycle: `pkg/events` imports `session` (stack.md §1), so `session` cannot import `pkg/events` back |
| Storage → bus decoupling (naming) | Interface named/shaped exactly like the existing `Notifier` precedent, one method, injected via a `SetItemChangePublisher` setter in `server/dependencies.go` | architecture.md §5 | Widening the existing `Notifier` interface to also carry routine transitions | ADR-001: different consumers (coalescing toast store vs. non-coalesced UI upsert) and different payload shapes argue for two channels, not one wider one |
| Publish hook placement | Repository-layer hooks (7 methods: `TransitionBacklogItemStatus`, `UpdateBacklogItem`, `ArchiveBacklogItem`, `DeleteBacklogItem`, `SaveReviewVerdict`/`CreateItemSessionWithVerdict`, `CreateItemSession`/`UpdateItemSessionSessionUUID`, `UpdateItemSessionTriageResult`) — Guarded/Template-ish "the mutation publishes itself" | pitfalls.md §3, architecture.md §2 | A test that enumerates all ~20+ RPC-handler/reconciler call sites and asserts each publishes | The enumeration test rots the moment a new caller is added without updating the list; collapsing into the repository layer makes missing a call site structurally impossible instead of relying on test discipline |
| Frontend event application | Server-authoritative "last-write-wins by timestamp/version" via a real staleness guard in the `upsertBacklogItem` reducer | pitfalls.md §2, ux.md §4 | Copy `sessionsSlice.upsertSession`'s equal-timestamp-only check verbatim | That check only skips exact-duplicate timestamps; it does not drop a genuinely older/out-of-order event, which `pkg/events.EventBus.Publish`'s non-atomic seq-assignment-then-fanout (pitfalls.md §2) can produce under concurrent publishers |
| Frontend state container | Redux slice (`backlogItemsSlice`), normalized `Record<id, item>` map — Identity Map (PoEAA) | `sessionsSlice.ts`/`reviewQueueSlice.ts` existing precedent | Keep local `useState` array per consumer (today's `/backlog`/`/backlog/board` pattern), with the shared hook only doing in-memory upsert per call site | `BacklogItemDetail`'s side panel is rendered *inside* the same `/backlog` page as the list (requirements.md baseline) — list and detail are mounted simultaneously, so per-component local state would open two independent subscriptions and risk the list card and the open detail panel showing divergent state for the same item after an event; a single shared normalized store (fed by one subscription, mirroring the `sessionsSlice` model) avoids both problems and matches this repo's own established pattern for exactly this kind of live-updated list |
| Reconnect/replay | `after_seq`-based ring-buffer replay (`EventBus.EventsSince`) as a **best-effort optimization only**, paired with a mandatory full-list REST refetch on every reconnect (never trusted alone) | pitfalls.md §1 | Trusting `after_seq` as a correctness guarantee (mirroring `WatchSessions`'s current, weaker client behavior which only detects *backwards* jumps) | `nextSeq`/ring buffer are purely in-process and reset to 0 on every server restart with no signal to the client (pitfalls.md §1); a naive client would show permanent silent staleness after any restart |
| Backpressure | Keep `EventBus`'s existing bounded-channel, non-blocking, drop-oldest fan-out unchanged; add **forward-gap detection** client-side (`event.seq !== lastSeq + 1 ⇒ resync`) as a new self-healing signal | pitfalls.md §4 | Reinventing bus internals (e.g. a larger buffer or blocking send) to avoid drops | Bus internals are shared infrastructure used by `WatchSessions` too; changing them is out of scope and risks the session stream. A client-side gap check turns a silent drop into a self-healing resync without touching the bus |
| Alert vs. routine-change channels | Two separate channels: existing `Notifier`/`EventBusNotifier` (alerts) and new `ItemChangePublisher`/`BacklogItemEvent` (routine state) — see ADR-001 | architecture.md §5 | Unify into one "mega-event" discriminated union covering both alerts and routine transitions | Different consumers (coalescing toast/notification-history store vs. non-coalesced UI upsert), different payload shapes, and this repo's own interface-pollution precedent (`.claude/rules/interface-pollution-checklist.md`) argue against widening `Notifier` or merging event types |

**Synthesis note (required by task instructions):** `build-vs-buy.md`'s literal recommendation (copy `ReactiveQueueManager`'s bespoke per-client registry) is **not followed**. `architecture.md`, `stack.md`, and `pitfalls.md` independently traced `ReactiveQueueManager`/`ReviewQueueEvent` in more depth and all three confirm it has no `Seq`/`after_seq`/replay concept at all — it would need that capability built from scratch, duplicating what `pkg/events.EventBus` already provides today for `WatchSessions`. This plan builds `BacklogItemEvent`'s **transport** on `pkg/events.EventBus` (mirroring `WatchSessions`), and borrows `ReviewQueueEvent`'s **proto message shape** (oneof-of-sub-messages, `is_snapshot`) only as a template for the payload — these are orthogonal design axes, and the plan makes an explicit, different choice on each.

---

## Migration Plan

No database schema changes. `session/ent/schema` is untouched — this feature only adds a publish side-effect to existing repository methods and a new proto/RPC surface; no new tables, columns, or ent generation is required.

## Observability Plan

- **Logs**: `WatchBacklogItems` connect/disconnect and `after_seq` replay-count logged at the same level/format `WatchSessions` uses today (`server/services/session_service.go:1991` area) — grep that handler's existing log lines and match the format exactly (structure: subscriber id, filter summary, replay count, disconnect reason).
- **Metrics**: none new required (requirements.md: "no special scaling design needed beyond following the existing bus's patterns"). If existing `EventBus` subscriber-count metrics exist, `WatchBacklogItems` subscribers should be tagged distinctly from `WatchSessions` subscribers so dashboards don't conflate the two streams.
- **Performance target**: p95 end-to-end latency (server-side event publish via `bus.Publish` to client-visible UI update) ≤ 2 seconds under normal load — see requirements.md's Non-functional Requirements → Performance SLO for the derivation (confirmed against `pkg/events.EventBus`'s actual design: non-blocking buffered-channel `Publish`, no network hop, no serialization delay beyond ConnectRPC's own framing — `pkg/events/bus.go:51-72`). No new automated latency-measurement task exists in this plan (Phase 7's tests cover correctness/branching, not timing); this target should be validated manually during Phase 6 UX polish testing (e.g. observe wall-clock time from a reconciler transition to the flash appearing in an open `/backlog` tab) rather than left as an unmeasured aspiration.
- **Alerts**: none — internal dev-tool feature (requirements.md Observability Requirements).

## Risk Control

- **Feature flag**: not gated — direct ship per requirements.md Risk Control. The stream is additive; existing fetch-on-mount remains the safe fallback if the stream never connects.
- **Rollback procedure**: the new RPC/proto surface and repository publish hooks are additive and side-effect-only (a failed/slow `bus.Publish` call must never block or fail the underlying mutation). This is enforced, not just asserted in prose: Task 1.3.2b wraps `BacklogItemEventPublisher.PublishItemChanged`'s entire body in a `recover()` (mirroring the `runStuckDetector` precedent, `session/backlog_lifecycle.go:1218-1234`), so a panic during publish is logged and contained inside the adapter and never reaches any of the 9 hooked repository methods; Task 2.1.1d proves this end-to-end with a deliberately panicking `ItemChangePublisher` wired into `TransitionBacklogItemStatus`. Rollback = revert the PR(s); no data migration to undo.
- **Staged rollout**: full rollout on merge, per requirements.md.

## Unresolved Questions

None block implementation start. The requirements.md Open Questions section marks all three open questions resolved during Phase 2 research, and this plan's Pattern Decisions table resolves the one remaining implementation-level choice (Redux vs. local state) with explicit justification. Separately, 4 items remain open in `pre-mortem.md` as tracked, accepted P2/P3 risks (not required to resolve before Phase 5):

- **P2 #1** — No idle-staleness backstop timer in `useWatchBacklogItems`, unlike `WatchSessions`'s 30s/15s detector, risking a tab that silently goes stale after a server restart with no organic event to trigger resync. **Resolved during this repair pass** — see Epic 4.2 Story 4.2.3, which ports `useSessionService.ts`'s 30s periodic backstop + 15s visibility/online staleness check.
- **P2 #2** — No structural guard (lint/architecture test/shared helper) stops a future 7th+ mutating repository method from bypassing all publish hooks. Still open — accepted scope for this project; adding a lint/architecture-test guard was an earlier repair-loop decision explicitly deferred, not addressed here.
- **P3 #3** — No explicit requirement for memoized selectors (`reselect`/`createSelector`) over `backlogItemsSlice`, risking unnecessary re-renders on unrelated item updates. Still open — accepted scope for this project, per the earlier repair-loop decision not to add a memoized-selector requirement at this stage.
- **P2 #4** — Task 3.1.1c didn't force `is_snapshot: true` on replayed events, risking double-flash/double-`aria-live`-announce on the `Subscribe()`/`EventsSince(afterSeq)` race window. **Resolved during this repair pass** — see Task 3.1.1c and Story 3.1.1's new acceptance criterion.

`pre-mortem.md`'s own table rows for #1 and #4 have been updated to note they were resolved during Phase 4's triad-review repair loop (marked, not deleted, to preserve the audit trail). #2 and #3 remain open, unresolved P2/P3 risks by deliberate choice.

## Dependency Visualization

```
Phase 1: Backend Event Infrastructure
  Epic 1.1 (proto) ──┐
  Epic 1.2 (EventType/Event struct) ──┤
  Epic 1.3 (ItemChangePublisher + adapter + wiring) ──┘
        │
        ▼
Phase 2: Repository-Layer Publish Hooks (needs 1.2 + 1.3)
  Epic 2.1 (status transition hook)
  Epic 2.2 (remaining mutation hooks: Stories 2.2.1-2.2.5, covering 8 methods across 6 change kinds, incl. triage-progress — Story 2.2.5)
        │
        ▼
Phase 3: WatchBacklogItems RPC Handler (needs Phase 1 proto + Phase 2 hooks to test end-to-end)
  Epic 3.1 (handler) ── Epic 3.2 (handler tests)
        │
        ▼
Phase 4: Frontend Streaming Infra (needs Phase 1 proto + generated TS bindings)
  Epic 4.1 (backlogItemsSlice) ──┐
  Epic 4.2 (useWatchBacklogItems hook) ──┘ (4.2 depends on 4.1's reducer + Phase 3's live RPC)
        │
        ▼
Phase 5: Consumer Wiring (needs Phase 4)
  Epic 5.1 (/backlog list)
  Epic 5.2 (/backlog/board)
  Epic 5.3 (BacklogItemDetail — remove shouldPoll, edit-mode buffering)
  Epic 5.4 (BacklogItemPanel)
        │
        ▼
Phase 6: UX Polish (needs Phase 5 views wired)
  Epic 6.1 (flash/highlight) ── Epic 6.2 (aria-live + Live indicator) ── Epic 6.3 (exit transition) ── Epic 6.4 (edit-mode banner)
        │
        ▼
Phase 7: Testing & Verification (spans all phases; can start as soon as each unit exists)
  Epic 7.1 (workspace isolation test) ─ independent, can run after Phase 3
  Epic 7.2 (MergeDatabase leak test) ─ independent, can run after Phase 3
  Epic 7.3 (reducer staleness tests) ─ independent, can run after Phase 4
  Epic 7.4 (handler branching tests) ─ independent, can run after Phase 3
        │
        ▼
Phase 8: Documentation & Governance
  Epic 8.1 (ADR-001 — actually written in parallel, Step 5) ── Epic 8.2 (feature registry)
```

---

## Phase 1: Backend Event Infrastructure

### Epic 1.1: Proto Schema Changes
**Goal**: Define the wire shape for `BacklogItemEvent` and the `WatchBacklogItems` RPC so Go and TypeScript bindings can be generated before any handler code is written.

#### Story 1.1.1: Define `BacklogItemEvent` oneof in `events.proto`
**As a** backend developer, **I want** a `BacklogItemEvent` message with per-concern oneof variants, **so that** the RPC handler and frontend can exhaustively switch on backlog change kinds with compile-time safety.
**Acceptance Criteria**:
- `BacklogItemEvent` has a `timestamp` field and a `oneof event` with 6 variants: `status_changed`, `verdict_recorded`, `session_attached`, `item_updated`, `item_archived`, `item_removed`.
  - *Given* the compiled `BacklogItemEvent` Go struct, *When* a `BacklogItemStatusChangedEvent{ItemId: "abc123", OldStatus: "in_progress", NewStatus: "review"}` is set on the oneof, *Then* `event.GetStatusChanged().GetNewStatus()` returns `"review"` and `event.GetVerdictRecorded()` returns `nil`.
- Every variant carries `is_snapshot: bool`.
  - *Given* a `BacklogItemStatusChangedEvent` built during initial-snapshot send, *When* `is_snapshot` is set `true`, *Then* the frontend's snapshot-vs-live branch (Story 4.2.2) treats it as non-flash-worthy.
**Files**: `proto/session/v1/events.proto`

##### Task 1.1.1a: Add `BacklogItemEvent` message + 6 oneof sub-messages (~5 min)
- Add after the existing `ReviewQueueEvent` block (proto lines ~312–382): `BacklogItemEvent { google.protobuf.Timestamp timestamp = 1; oneof event { BacklogItemStatusChangedEvent status_changed = 2; BacklogItemVerdictRecordedEvent verdict_recorded = 3; BacklogItemSessionAttachedEvent session_attached = 4; BacklogItemUpdatedEvent item_updated = 5; BacklogItemArchivedEvent item_archived = 6; BacklogItemRemovedEvent item_removed = 7; } }`
- Define `BacklogItemStatusChangedEvent { string item_id = 1; string old_status = 2; string new_status = 3; BacklogItem item = 4; bool is_snapshot = 5; }` (reuse the existing `BacklogItem` message already defined elsewhere in the proto package for RPC responses).
- Define `BacklogItemVerdictRecordedEvent { string item_id = 1; ReviewVerdict verdict = 2; BacklogItem item = 3; bool is_snapshot = 4; }`.
- Define `BacklogItemSessionAttachedEvent { string item_id = 1; string session_id = 2; BacklogItem item = 3; bool is_snapshot = 4; }`.
- Files: `proto/session/v1/events.proto`

##### Task 1.1.1b: Add remaining 3 oneof sub-messages (~4 min)
- Define `BacklogItemUpdatedEvent { string item_id = 1; repeated string updated_fields = 2; BacklogItem item = 3; bool is_snapshot = 4; }`.
- Define `BacklogItemArchivedEvent { string item_id = 1; google.protobuf.Timestamp archived_at = 2; bool is_snapshot = 3; }`.
- Define `BacklogItemRemovedEvent { string item_id = 1; string reason = 2; }` (no `is_snapshot` — a removal is never part of a snapshot by definition).
- Files: `proto/session/v1/events.proto`

#### Story 1.1.2: Define `WatchBacklogItemsRequest` and the `WatchBacklogItems` RPC
**As a** frontend developer, **I want** a `WatchBacklogItems` server-streaming RPC with filter fields and an `after_seq` cursor, **so that** `useWatchBacklogItems` can subscribe with the same reconnect story as `WatchSessions`.
**Acceptance Criteria**:
- `WatchBacklogItemsRequest` has `repeated string status_filter`, `repeated string category_filter`, and `uint64 after_seq`.
  - *Given* a `WatchBacklogItemsRequest{AfterSeq: 42}`, *When* the handler (Story 3.1.1) receives it, *Then* it calls `eventBus.EventsSince(42)` instead of sending a fresh snapshot.
- `BacklogService` gains `rpc WatchBacklogItems(WatchBacklogItemsRequest) returns (stream BacklogItemEvent);`.
  - *Given* the generated `BacklogServiceClient`, *When* `client.watchBacklogItems({})` is called with no filters, *Then* the stream yields snapshot events for every currently-visible `BacklogItem` followed by live events, matching `WatchSessions`'s no-filter behavior.
**Files**: `proto/session/v1/session.proto`

##### Task 1.1.2a: Add `WatchBacklogItemsRequest` message (~3 min)
- Add near `WatchReviewQueueRequest` (proto line ~754): `message WatchBacklogItemsRequest { repeated string status_filter = 1; repeated string category_filter = 2; uint64 after_seq = 3; }`.
- Files: `proto/session/v1/session.proto`

##### Task 1.1.2b: Add `WatchBacklogItems` RPC to `BacklogService` (~3 min)
- Add `rpc WatchBacklogItems(WatchBacklogItemsRequest) returns (stream BacklogItemEvent);` to the existing `BacklogService` service block (same service that defines `TransitionBacklogItemStatus`, `UpdateBacklogItem`, etc.).
- Files: `proto/session/v1/session.proto`

#### Story 1.1.3: Regenerate proto bindings
**As a** developer on either side of the RPC, **I want** `make proto-gen` run and its output committed, **so that** Go and TypeScript code can reference the new types.
**Acceptance Criteria**:
- Running `make proto-gen` regenerates `session/gen/session/v1/events_pb.go`, `session/gen/session/v1/session_pb.go`, and `web-app/src/gen/session/v1/events_pb.ts`/`session_pb.ts` without errors.
  - *Given* a clean working tree after Task 1.1.1a/b and 1.1.2a/b, *When* `make proto-gen` runs, *Then* `git status` shows only the expected generated files changed, and `go build ./...` succeeds.
**Files**: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts` (generated, not hand-edited)

##### Task 1.1.3a: Run `make proto-gen` and verify build (~3 min)
- Run `make proto-gen`.
- Run `go build ./...` and `cd web-app && npx tsc --noEmit` to confirm both sides compile against the new types.
- Files: (generated files only, per above)

---

### Epic 1.2: Event Bus Payload Extension
**Goal**: Extend the existing generic `pkg/events.Event` envelope so a `BacklogItemChanged` event can carry backlog payload, without disturbing `WatchSessions`'s existing session-event handling.

#### Story 1.2.1: Add `EventBacklogItemChanged` type + payload field
**As a** backend developer, **I want** a new `events.EventType` constant and a backlog payload field on `events.Event`, **so that** the same bus/ring-buffer/replay machinery serves both session and backlog events.
**Acceptance Criteria**:
- `pkg/events/types.go` defines `EventBacklogItemChanged EventType = "backlog_item_changed"` alongside existing constants.
  - *Given* `events.EventBacklogItemChanged`, *When* compared against existing constants like `events.EventSessionCreated`, *Then* it is a distinct, non-colliding string value.
- `events.Event` gains a `BacklogItemPayload *BacklogItemEventPayload` field (a new small Go struct holding the oneof-variant-equivalent data: kind, item snapshot, old/new status, verdict, etc.) — analogous to the existing `Session *session.Instance` field.
  - *Given* `bus.Publish(&events.Event{Type: events.EventBacklogItemChanged, BacklogItemPayload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, Item: item, OldStatus: "in_progress", NewStatus: "review"}})`, *When* a subscriber receives it via `eventCh`, *Then* `event.BacklogItemPayload.Kind == events.BacklogChangeStatusTransition` and `event.Seq > 0` (assigned by `Publish`).
**Files**: `pkg/events/types.go`, `pkg/events/bus.go` (no changes needed to `Publish`/`Subscribe`/`EventsSince` logic itself — only the `Event` struct's fields)

##### Task 1.2.1a: Add `EventBacklogItemChanged` constant (~2 min)
- Add `EventBacklogItemChanged EventType = "backlog_item_changed"` to the `EventType` const block in `pkg/events/types.go`.
- Files: `pkg/events/types.go`

##### Task 1.2.1b: Add `BacklogItemEventPayload` struct + field on `Event` (~5 min)
- Define `type BacklogChangeKind string` with constants `BacklogChangeStatusTransition`, `BacklogChangeVerdictRecorded`, `BacklogChangeSessionAttached`, `BacklogChangeItemUpdated`, `BacklogChangeItemArchived`, `BacklogChangeItemRemoved`, `BacklogChangeTriageProgressUpdated` (Story 2.2.5's hook on `UpdateItemSessionTriageResult`; converts to the existing `BacklogItemUpdatedEvent` oneof variant, not a new proto message — see Domain Glossary).
- Define `type BacklogItemEventPayload struct { Kind BacklogChangeKind; Item *session.BacklogItemData; OldStatus, NewStatus string; UpdatedFields []string; SessionID string; ArchivedAt *time.Time; RemovedReason string; Verdict *session.ReviewVerdictData; IsSnapshot bool }`. `Verdict` mirrors `BacklogItemChange.Verdict` (Task 1.3.1a) one-to-one — the adapter (Story 1.3.2) copies it straight through so the verdict reaches the RPC handler as first-class payload data, not something the handler or frontend must derive by joining `item_sessions`.
- Add `BacklogItemPayload *BacklogItemEventPayload` field to the existing `Event` struct.
- Files: `pkg/events/types.go`

##### Task 1.2.1c: Add `NewBacklogItemChangedEvent` constructor helper (~3 min)
- Add a small constructor function (mirroring any existing `NewXEvent` helpers in the package) `NewBacklogItemChangedEvent(payload *BacklogItemEventPayload) *Event` that stamps `Type: EventBacklogItemChanged`, `Timestamp: time.Now()`, and the payload — used by the adapter in Epic 1.3 instead of hand-building the struct at each call site.
- Files: `pkg/events/types.go` (or a new small `pkg/events/backlog_event.go` if the existing file is already large — check file length first)

---

### Epic 1.3: `ItemChangePublisher` Interface, Adapter, and Wiring
**Goal**: Bridge the storage layer (which cannot import `pkg/events` directly) to the event bus, following the exact `Notifier`/`EventBusNotifier` precedent.

#### Story 1.3.1: Define `session.ItemChangePublisher` interface
**As a** storage-layer developer, **I want** a narrow interface defined in the `session` package, **so that** repository methods can publish changes without creating an import cycle.
**Acceptance Criteria**:
- `session.ItemChangePublisher` has exactly one method: `PublishItemChanged(item *BacklogItemData, change BacklogItemChange)`.
  - *Given* a `nil` `ItemChangePublisher` on a `Storage`/`EntRepository` (publisher not wired), *When* `TransitionBacklogItemStatus` runs, *Then* the method nil-checks before calling and the transition still succeeds (publish is best-effort, never blocking).
- `session.BacklogItemChange` is a small struct (`Kind`, `OldStatus`, `NewStatus`, `UpdatedFields`, etc.) defined in `session`, not a stringly-typed map.
  - *Given* `BacklogItemChange{Kind: session.ChangeStatusTransition, OldStatus: "in_progress", NewStatus: "review"}`, *When* the adapter (Story 1.3.2) receives it, *Then* it maps deterministically to `events.BacklogChangeStatusTransition` with matching fields.
**Files**: `session/backlog_lifecycle.go` (next to existing `Notifier` interface, line 28) or a new `session/backlog_item_change.go` if that file is more appropriate given its current size

##### Task 1.3.1a: Define `BacklogItemChange` struct and `ChangeKind` enum (~4 min)
- Add `type BacklogChangeKind string` with constants mirroring Task 1.2.1b's kinds (`ChangeStatusTransition`, `ChangeVerdictRecorded`, `ChangeSessionAttached`, `ChangeItemUpdated`, `ChangeItemArchived`, `ChangeItemRemoved`, `ChangeTriageProgressUpdated`).
- Add `type BacklogItemChange struct { Kind BacklogChangeKind; OldStatus, NewStatus string; UpdatedFields []string; SessionID string; ArchivedAt *time.Time; RemovedReason string; Verdict *ReviewVerdictData }`. `Verdict` is populated only when `Kind == ChangeVerdictRecorded`, set directly from the `ReviewVerdictData` value the caller already has in hand (the actual parameter type of `SaveReviewVerdict`/`CreateItemSessionWithVerdict`, `session/storage_backlog.go:37`) — this carries the verdict through the pipeline as first-class data, not via a client-side join against `item_sessions`.
- Files: `session/backlog_lifecycle.go` (near existing `Notifier`, line 28) — check current file length first; if >1500 lines, create `session/backlog_item_change.go` instead to avoid further bloating an already-large file.

##### Task 1.3.1b: Define `ItemChangePublisher` interface (~2 min)
- Add `type ItemChangePublisher interface { PublishItemChanged(item *BacklogItemData, change BacklogItemChange) }` next to the `Notifier` interface.
- Files: same file as Task 1.3.1a.

#### Story 1.3.2: Implement `BacklogItemEventPublisher` adapter
**As a** wiring-layer developer, **I want** a `server/services` adapter implementing `ItemChangePublisher`, **so that** repository-layer changes reach the event bus.
**Acceptance Criteria**:
- `BacklogItemEventPublisher.PublishItemChanged` converts `BacklogItemChange` → `events.BacklogItemEventPayload` → `events.NewBacklogItemChangedEvent(...)` → `bus.Publish(...)`.
  - *Given* a `BacklogItemEventPublisher{Bus: testBus}` and a call `PublishItemChanged(item, session.BacklogItemChange{Kind: session.ChangeStatusTransition, OldStatus: "review", NewStatus: "done"})`, *When* a test subscriber reads from `testBus.Subscribe(ctx)`, *Then* it receives an `*events.Event` with `Type == events.EventBacklogItemChanged` and `BacklogItemPayload.NewStatus == "done"`.
- `PublishItemChanged`'s entire body is wrapped in its own `recover()`, so a panic anywhere inside it (payload construction, an unmapped `BacklogChangeKind`, `bus.Publish` itself) can never propagate into the repository method that called it — the same "best-effort side channel must not take down the caller" idiom this codebase already uses at `session/backlog_lifecycle.go:1218-1234` (`runStuckDetector`: `defer func() { if r := recover(); r != nil { log.WarningLog.Printf(...) } }()` wrapping a synchronous `fn()` call, logging and continuing rather than propagating) and at `server/services/session_service.go:2216`/`:2300` (goroutine-local `recover()` around PTY forwarding). Because the recover happens inside the adapter itself, `session.ItemChangePublisher.PublishItemChanged` deliberately keeps its no-error-return signature from Story 1.3.1 — there is never an error for a repository caller to log or swallow, because a panic can never reach it in the first place.
  - *Given* a `BacklogItemEventPublisher{Bus: testBus}` whose payload construction panics (e.g. a test double that panics inside `PublishItemChanged` before calling `bus.Publish`), *When* a repository method (e.g. `TransitionBacklogItemStatus`, Task 2.1.1d) calls `PublishItemChanged` as its last step, *Then* the panic is recovered and logged at WARNING/ERROR level inside the adapter, and the repository method still returns its normal success result to its own caller.
**Files**: `server/services/backlog_item_event_publisher.go` (new file, parallel to `server/services/backlog_notifier.go`)

##### Task 1.3.2a: Create `BacklogItemEventPublisher` struct + `PublishItemChanged` method (~5 min)
- New file, struct `type BacklogItemEventPublisher struct { Bus *events.EventBus }` (using the `server/events` alias per stack.md §1, matching `EventBusNotifier`'s pattern in `backlog_notifier.go`).
- Implement `PublishItemChanged` mapping every `BacklogChangeKind` to the matching `events.BacklogChangeKind` and calling `p.Bus.Publish(events.NewBacklogItemChangedEvent(payload))`.
- Files: `server/services/backlog_item_event_publisher.go` (new), reference `server/services/backlog_notifier.go` for the exact adapter style to match.

##### Task 1.3.2b: Wrap `PublishItemChanged`'s body in `recover()`-based panic isolation (~4 min)
- At the top of `PublishItemChanged`, add `defer func() { if r := recover(); r != nil { log.WarningLog.Printf("[BacklogItemEventPublisher] PublishItemChanged panicked (recovered): %v", r) } }()` before any payload construction or `p.Bus.Publish(...)` call, mirroring `runStuckDetector`'s shape (`session/backlog_lifecycle.go:1218-1234`) — a synchronous best-effort call, recovered and logged in place, never propagated.
- This makes the Risk Control section's "a failed/slow `bus.Publish` call must never block or fail the underlying mutation" claim an enforced property of the single adapter method (and therefore all 9 hooked call sites, which all go through it) rather than prose with no backing code.
- Files: `server/services/backlog_item_event_publisher.go`

#### Story 1.3.3: Wire the publisher in `server/dependencies.go`
**As a** server startup path, **I want** the adapter constructed and injected into the repository that owns the 9 hooked methods, **so that** every repository method has a live publisher at runtime.
**Acceptance Criteria**:
- `server/dependencies.go` constructs `&services.BacklogItemEventPublisher{Bus: eventBus}` and calls `storage.SetItemChangePublisher(...)` — a forwarding method on `*Storage` (Task 1.3.3a) that type-asserts down to the concrete `*EntRepository`, the struct that actually owns all 9 hooked methods (`TransitionBacklogItemStatus`, `UpdateBacklogItem`, `ArchiveBacklogItem`, `DeleteBacklogItem` in `session/ent_repository_backlog.go`; `SaveReviewVerdict`, `CreateItemSessionWithVerdict`, `CreateItemSession`, `UpdateItemSessionSessionUUID`, `UpdateItemSessionTriageResult` in `session/storage_backlog.go` — confirmed via grep, all are `func (r *EntRepository) ...`). This call can be placed near the existing `backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})` call (line ~525) for readability, but the two calls wire **two different structs** (`*EntRepository` via `*Storage`'s forwarding method, vs. `*BacklogLifecycleListener` directly) — they are adjacent in the startup file, not the same wiring pattern.
  - *Given* server startup completes, *When* any of the 9 hooked repository methods (Phase 2) runs in production, *Then* the concrete `*EntRepository`'s `itemChangePublisher` field is non-nil and a corresponding `BacklogItemEvent` reaches any connected `WatchBacklogItems` stream.
  - *Given* a test double repository that is not `*EntRepository` (e.g. an in-memory fake used in unit tests), *When* `storage.SetItemChangePublisher(...)` is called against it, *Then* the type assertion fails gracefully (no panic) and the publisher is simply never wired for that fake — matching `Storage.GetEntClient()`'s existing nil-on-mismatch behavior.
**Files**: `server/dependencies.go`, `session/storage.go`, `session/ent_repository_backlog.go`

##### Task 1.3.3a: Add `SetItemChangePublisher` to `*EntRepository`, plus a forwarding setter on `*Storage` (~5 min)
- Add a field `itemChangePublisher session.ItemChangePublisher` (nil-safe — every hooked method nil-checks before calling it) and a method `func (r *EntRepository) SetItemChangePublisher(p session.ItemChangePublisher)` directly to `*EntRepository` in `session/ent_repository_backlog.go`. This is the struct that owns all 9 hooked methods — do **not** add the field/setter to `Storage` itself; `Storage.TransitionBacklogItemStatus` (`session/storage.go:721`) is a thin passthrough with no CAS logic of its own, and the same is true of `Storage`'s wrappers around the other 8 methods (including `Storage.UpdateItemSessionTriageResult`, `session/storage.go:880`).
- Since `server/dependencies.go` only has a `*Storage` value in scope (which holds a `repo Repository` interface field, not a concrete `*EntRepository`), add a forwarding method on `Storage` that follows the **already-established** precedent at `Storage.GetEntClient()` (`session/storage.go:234-239`), which type-asserts `s.repo.(*EntRepository)` to reach ent-specific behavior:
  ```go
  func (s *Storage) SetItemChangePublisher(p session.ItemChangePublisher) {
      if er, ok := s.repo.(*EntRepository); ok {
          er.SetItemChangePublisher(p)
      }
  }
  ```
  This is the correct applicable precedent — not `Notifier`'s wiring, which targets an unrelated struct (`BacklogLifecycleListener`, see Domain Glossary's `Notifier`/`EventBusNotifier` row).
- Files: `session/ent_repository_backlog.go` (field + setter on `*EntRepository`), `session/storage.go` (forwarding setter on `*Storage`, placed next to `GetEntClient`)

##### Task 1.3.3b: Wire in `server/dependencies.go` (~3 min)
- Add `storage.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: eventBus})`. May be placed near the existing `backlogLifecycleListener.SetNotifier(...)` line for readability, but it is wiring a different struct (`*EntRepository`, via `*Storage`'s forwarding method from Task 1.3.3a) — not "mirroring" that call's target.
- Files: `server/dependencies.go`

---

## Phase 2: Repository-Layer Publish Hooks

### Epic 2.1: Status Transition Hook
**Goal**: Wire the single highest-value, highest-volume choke point — every status transition from any of the ~20 call sites becomes visible.

#### Story 2.1.1: Publish `BacklogItemStatusChangedEvent`-equivalent from `TransitionBacklogItemStatus`
**As an** operator watching any backlog view, **I want** every status transition — regardless of whether it came from the RPC handler or an internal reconciler — to push an event, **so that** `reconcileBouncingItems`/`ReconcilePRPending`/auto-reopen changes are visible without a manual refresh.
**Acceptance Criteria**:
- After the CAS `UPDATE` in `EntRepository.TransitionBacklogItemStatus` (`session/ent_repository_backlog.go:674`) succeeds, the method calls `r.itemChangePublisher.PublishItemChanged(updatedItem, session.BacklogItemChange{Kind: session.ChangeStatusTransition, OldStatus: oldStatus, NewStatus: newStatus})` before returning.
  - *Given* an in-memory test `EventBus` subscribed via `bus.Subscribe(ctx)`, and a `BacklogItem` with `Status: "in_progress"`, *When* `session/backlog_lifecycle.go`'s `ReconcilePRPending` calls `storage.TransitionBacklogItemStatus(ctx, itemID, "done", "in_progress")` (simulating a merged-PR reconciler transition, matching the call site at `session/backlog_lifecycle.go:2154`), *Then* the test subscriber receives an `*events.Event{Type: EventBacklogItemChanged}` with `BacklogItemPayload.OldStatus == "in_progress"` and `BacklogItemPayload.NewStatus == "done"` within 100ms, with no RPC handler involved in the call chain.
- If the CAS `UPDATE` affects 0 rows (precondition failed / TOCTOU race lost), no event is published.
  - *Given* two goroutines racing `TransitionBacklogItemStatus` with the same `expectedOldStatus`, *When* the second call's `Where()` clause matches 0 rows, *Then* the method returns its existing "precondition failed" error and the publisher is never called.
**Files**: `session/ent_repository_backlog.go`

##### Task 2.1.1a: Add publish call after successful CAS in `TransitionBacklogItemStatus` (~5 min)
- Locate the success path of `EntRepository.TransitionBacklogItemStatus` (`session/ent_repository_backlog.go:674`, doc comment at 652–673) — after the `UPDATE` confirms `rowsAffected > 0`.
- Call `r.itemChangePublisher.PublishItemChanged(item, session.BacklogItemChange{Kind: session.ChangeStatusTransition, OldStatus: oldStatus, NewStatus: newStatus})`, guarded by a nil-check on `r.itemChangePublisher`.
- Wrap the call so it cannot return an error into the caller's path (log-and-continue if the interface method were ever changed to return an error; currently it does not, per Story 1.3.1).
- Files: `session/ent_repository_backlog.go`

##### Task 2.1.1b: Unit test — status transition publishes with correct old/new status (~5 min)
- New test in `session/ent_repository_backlog_test.go` (or wherever existing `TransitionBacklogItemStatus` tests live): subscribe to a real `*events.EventBus` instance wired into the repository via `SetItemChangePublisher`, call `TransitionBacklogItemStatus`, assert the received event via a timeout `select`.
- Files: `session/ent_repository_backlog_test.go`

##### Task 2.1.1c: Regression test — failed CAS does not publish (~4 min)
- Simulate a lost race (call `TransitionBacklogItemStatus` with a stale `expectedOldStatus`) and assert no event arrives on the subscriber channel within a short timeout.
- Files: `session/ent_repository_backlog_test.go`

##### Task 2.1.1d: Regression test — a panicking `ItemChangePublisher` does not propagate through `TransitionBacklogItemStatus` (~5 min)
- Wire a test double `ItemChangePublisher` (e.g. `panickingPublisher struct{}` whose `PublishItemChanged` does `panic("boom")`) into the repository via `SetItemChangePublisher` (Task 1.3.3a) instead of the real `BacklogItemEventPublisher`.
- Call `TransitionBacklogItemStatus` with a valid transition and assert: (1) it returns its normal success result (no error, no panic reaching the test goroutine), and (2) the underlying row was actually updated (re-fetch and check `Status`) — proving the panic was contained entirely within the publish step and never affected the CAS `UPDATE` or its return path.
- This exercises the same `recover()` guarantee added in Task 1.3.2b from the repository-caller's side, closing the loop the adversarial review flagged: Task 1.3.2b proves the adapter itself recovers; this test proves a real hooked call site is unaffected end-to-end. One shared test at this call site is sufficient — all 9 hooked methods route through the same `PublishItemChanged` implementation, so this test's coverage of the recover boundary generalizes to the other 8 without needing a duplicate test per hook.
- Files: `session/ent_repository_backlog_test.go`

---

### Epic 2.2: Remaining 6 Mutation Hooks
**Goal**: Cover the rest of the lifecycle table from architecture.md §2/§4 so every UI-visible mutation pushes an event — including in-flight triage-progress writes (Story 2.2.5), whose omission would otherwise regress `shouldPoll`'s `triageStatus === "running"` coverage once Story 5.3.1 deletes the poll (pre-mortem.md P1 #5).

#### Story 2.2.1: Publish from `UpdateBacklogItem`
**Acceptance Criteria**:
- `UpdateBacklogItem` (`session/ent_repository_backlog.go:479`) publishes `BacklogItemChange{Kind: ChangeItemUpdated, UpdatedFields: [...]}` listing which fields actually changed (title, description, planText, acceptance criteria list), after a successful `UPDATE`.
  - *Given* a `BacklogItem` with `Title: "Old title"`, *When* `UpdateBacklogItem(ctx, itemID, &UpdateBacklogItemParams{Title: ptr("New title")})` is called (matching the call site at `server/services/backlog_service_lifecycle.go:553`), *Then* the subscriber receives `BacklogItemPayload.Kind == ChangeItemUpdated` and `BacklogItemPayload.UpdatedFields == []string{"title"}`.
**Files**: `session/ent_repository_backlog.go`

##### Task 2.2.1a: Add publish call in `UpdateBacklogItem` with field-diff detection (~5 min)
- After the successful `UPDATE`, compute which non-nil params were actually set in the call (title/description/planText/AC list) and call `PublishItemChanged` with `ChangeItemUpdated` + that field list.
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.1b: Unit test — `UpdateBacklogItem` publishes correct `UpdatedFields` (~4 min)
- Files: `session/ent_repository_backlog_test.go`

#### Story 2.2.2: Publish from `ArchiveBacklogItem` and `DeleteBacklogItem`
**Acceptance Criteria**:
- `ArchiveBacklogItem` (`ent_repository_backlog.go:589`) publishes `ChangeItemArchived` with the `archived_at` timestamp.
  - *Given* a `done` item, *When* `ArchiveBacklogItem(ctx, itemID)` runs (called from `server/services/backlog_service_lifecycle.go:308`, the retention sweep's only caller), *Then* the subscriber receives `BacklogItemPayload.Kind == ChangeItemArchived` with a non-nil `ArchivedAt`.
- `DeleteBacklogItem` (`ent_repository_backlog.go:612`) publishes `ChangeItemRemoved`, mapped to `BacklogItemRemovedEvent` on the frontend (a delete, not an upsert).
  - *Given* an existing item, *When* `DeleteBacklogItem(ctx, itemID)` runs (called from `backlog_service_lifecycle.go:338`), *Then* the subscriber receives `BacklogItemPayload.Kind == ChangeItemRemoved` and the RPC handler (Story 3.1.1) translates this into a `BacklogItemRemovedEvent`, not a status/updated event.
**Files**: `session/ent_repository_backlog.go`

##### Task 2.2.2a: Add publish call in `ArchiveBacklogItem` (~3 min)
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.2b: Add publish call in `DeleteBacklogItem` (~3 min)
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.2c: Unit tests for both (~5 min)
- Files: `session/ent_repository_backlog_test.go`

#### Story 2.2.3: Publish from `SaveReviewVerdict` / `CreateItemSessionWithVerdict`
**Acceptance Criteria**:
- The verdict is carried explicitly **in** the event payload — `BacklogItemChange.Verdict` → `BacklogItemEventPayload.Verdict` → the proto `BacklogItemVerdictRecordedEvent.verdict` field (Task 1.3.1a/1.2.1b) — end to end. The frontend must **not** determine "the new verdict" by joining the event's `item` snapshot against `item_sessions`; the verdict is self-contained on the event itself. This removes the ambiguity flagged in the architecture review.
- `SaveReviewVerdict` (`session/storage_backlog.go:314`) publishes `ChangeVerdictRecorded` with `BacklogItemChange.Verdict` set to the just-written `ReviewVerdictData`, after the verdict row is written.
  - *Given* an item in `review` status with no verdict, *When* `SaveReviewVerdict(ctx, itemSessionID, ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass})` is called (matching `backlog_service_lifecycle.go:755`, the RPC path — note the real signature takes an `itemSessionID string` and a `ReviewVerdictData` value, not a `*ReviewVerdict` pointer), *Then* the subscriber receives `BacklogItemPayload.Kind == ChangeVerdictRecorded` with `BacklogItemPayload.Verdict.OverallOutcome == session.ReviewOutcomePass`, populated directly from the write — not derived by re-reading `item_sessions`.
- `CreateItemSessionWithVerdict` (`storage_backlog.go:381`) publishes the same event kind for its combined create+verdict path (e.g. the MCP `submit_review_verdict` tool path), with `BacklogItemChange.Verdict` set from the same `ReviewVerdictData` argument it was called with.
  - *Given* a headless review agent calling the equivalent of `mcp__stapler-squad__submit_review_verdict`, *When* `CreateItemSessionWithVerdict` runs (matching `session/backlog_review.go:577`), *Then* the same `ChangeVerdictRecorded` event fires with `BacklogItemPayload.Verdict` populated from that call's `ReviewVerdictData` argument — confirming both verdict-recording paths converge on one event kind and both carry the verdict inline, with no client-side join required in either case.
**Files**: `session/storage_backlog.go`

##### Task 2.2.3a: Add publish call in `SaveReviewVerdict` (~4 min)
- Files: `session/storage_backlog.go`

##### Task 2.2.3b: Add publish call in `CreateItemSessionWithVerdict` (~4 min)
- Files: `session/storage_backlog.go`

##### Task 2.2.3c: Unit tests for both verdict-recording paths (~5 min)
- Files: `session/storage_backlog_test.go`

#### Story 2.2.4: Publish from `CreateItemSession` / `UpdateItemSessionSessionUUID`
**Acceptance Criteria**:
- `CreateItemSession` (`storage_backlog.go:54`) and `UpdateItemSessionSessionUUID` (`storage_backlog.go:207`) publish `ChangeSessionAttached` with the session id/UUID.
  - *Given* an item transitioning `ready → in_progress` via the dequeue sweep, *When* `session/backlog_lifecycle.go`'s spawner calls `CreateItemSession(ctx, itemID, sessionID)` followed later by `UpdateItemSessionSessionUUID(ctx, itemSessionID, tmuxUUID)` (matching the attach-point description in architecture.md §4's `WorkSessionAttached` row), *Then* the subscriber receives two `ChangeSessionAttached` events (one per call), each with the correct `SessionID`/UUID, so the UI can show "session attached" as soon as either piece of info lands.
**Files**: `session/storage_backlog.go`

##### Task 2.2.4a: Add publish call in `CreateItemSession` (~4 min)
- Files: `session/storage_backlog.go`

##### Task 2.2.4b: Add publish call in `UpdateItemSessionSessionUUID` (~4 min)
- Files: `session/storage_backlog.go`

##### Task 2.2.4c: Unit tests for both (~5 min)
- Files: `session/storage_backlog_test.go`

#### Story 2.2.5: Publish from `UpdateItemSessionTriageResult`
**As an** operator watching an item's detail panel during an active triage run, **I want** each triage-progress write to push a live event, **so that** removing `shouldPoll`'s `triageStatus === "running"` polling (Story 5.3.1) doesn't leave triage progress invisible — closing the gap features.md §1d flagged for re-verification and pre-mortem.md P1 #5 identified as the plan's highest-severity unresolved risk.

**Real method confirmed via grep (2026-07-21)**: `EntRepository.UpdateItemSessionTriageResult(ctx context.Context, id string, triageResult string) error` (`session/storage_backlog.go:276`), thin passthrough `Storage.UpdateItemSessionTriageResult` (`session/storage.go:880`). Non-test callers: `server/services/backlog_service_triage.go:1652` (headless-triage progress write) and `server/mcp/tools_backlog.go:705` (MCP progress-report path). The Domain Glossary's original candidate name from features.md §1d turned out to be exactly correct — no rename needed, only the missing hook.

**Acceptance Criteria**:
- `UpdateItemSessionTriageResult` publishes `BacklogItemChange{Kind: ChangeTriageProgressUpdated, UpdatedFields: []string{"triageResultSummary"}}` after the successful `Save`, converted by the RPC handler (`convertEventToBacklogItemEvent`, Task 3.1.1d) into the **existing** `BacklogItemUpdatedEvent` proto oneof variant (Task 1.1.1b) — no new proto message is required, since a triage-result write is a field update on the item's derived state (`BacklogItemData.TriageResultSummary`, parsed from the `ItemSession.triage_result` JSON column per `session/repository.go:302`), which `BacklogItemUpdatedEvent`'s `updated_fields` shape already models exactly.
  - *Given* an `ItemSession` mid-triage with `TriageResult: ""`, *When* `UpdateItemSessionTriageResult(ctx, itemSessionID, payloadJSON)` is called (matching the call site at `backlog_service_triage.go:1652`), *Then* the subscriber receives an `*events.Event{Type: EventBacklogItemChanged}` with `BacklogItemPayload.Kind == events.BacklogChangeTriageProgressUpdated` and `BacklogItemPayload.UpdatedFields == []string{"triageResultSummary"}`.
- Because `UpdateItemSessionTriageResult` takes an `ItemSession` id, not a `BacklogItem` id, the hook first resolves the owning `BacklogItemID` via the `ItemSession`'s `backlog_item` edge (the same edge `GetItemSession`'s `.WithBacklogItem()` query already loads, `session/ent_repository_backlog.go`), then calls the existing `r.GetBacklogItem(ctx, itemID)` (`ent_repository_backlog.go:258`) to build the fresh full-item snapshot carried on the event — `CreateItemSession`'s hook (Story 2.2.4) already has the item id in hand directly from `data.ItemID`, but this method must look it up first since it's only ever called with the `ItemSession` id.
  - *Given* an `ItemSession` row whose `backlog_item` edge points to item `"item-42"`, *When* `UpdateItemSessionTriageResult` runs, *Then* the published event's `Item.ID == "item-42"` and `Item.TriageResultSummary` reflects the just-written value, not a stale read.
- If the owning-item lookup fails (e.g. the `ItemSession` row was deleted concurrently — an edge case, not the common path), the publish step is skipped (logged, not fatal) and the underlying `UPDATE` to `ItemSession.triage_result` still succeeds and returns normally — same "publish is best-effort, never blocking" guarantee as every other hook (Story 1.3.1, Task 1.3.2b).
  - *Given* the owning-item lookup returns `ErrNotFound`, *When* `UpdateItemSessionTriageResult` runs, *Then* the method still returns its normal success result for the `triage_result` write, and no panic or error propagates from the skipped publish step.
**Files**: `session/storage_backlog.go`

##### Task 2.2.5a: Add publish call in `UpdateItemSessionTriageResult` with owning-item lookup (~5 min)
- After the successful `SetTriageResult(...).Save(ctx)` call, resolve the `ItemSession`'s owning `BacklogItemID` (e.g. `r.client.ItemSession.Query().Where(itemsession.ID(parsedID)).QueryBacklogItem().OnlyID(ctx)`), then call `r.GetBacklogItem(ctx, itemID.String())` to fetch the current full item, then call `r.itemChangePublisher.PublishItemChanged(item, session.BacklogItemChange{Kind: session.ChangeTriageProgressUpdated, UpdatedFields: []string{"triageResultSummary"}})`, guarded by a nil-check on `r.itemChangePublisher` exactly like Task 2.1.1a. If the owning-item lookup or `GetBacklogItem` fails, log and skip the publish call rather than returning an error from `UpdateItemSessionTriageResult` itself (the `triage_result` write already succeeded).
- Files: `session/storage_backlog.go`

##### Task 2.2.5b: Unit test — `UpdateItemSessionTriageResult` publishes with correct `Kind`/`UpdatedFields` (~4 min)
- New test in `session/storage_backlog_test.go` (mirroring Task 2.2.3c/2.2.4c's style): seed a `BacklogItem` + `ItemSession` linked via the `backlog_item` edge, subscribe to a real `*events.EventBus` wired via `SetItemChangePublisher`, call `UpdateItemSessionTriageResult`, assert the received event's `BacklogItemPayload.Kind == events.BacklogChangeTriageProgressUpdated`, `UpdatedFields == []string{"triageResultSummary"}`, and `Item.ID` matches the seeded item's id.
- Files: `session/storage_backlog_test.go`

---

## Phase 3: `WatchBacklogItems` RPC Handler

### Epic 3.1: Handler Implementation
**Goal**: A ConnectRPC server-streaming handler mirroring `WatchSessions` exactly, including real `after_seq` replay.

#### Story 3.1.1: Implement `WatchBacklogItems` handler
**As a** frontend client, **I want** to connect, optionally pass `after_seq`, and receive a snapshot-then-live (or replay-then-live) stream of `BacklogItemEvent`s, **so that** `useWatchBacklogItems` has a working RPC to call.
**Acceptance Criteria**:
- On a fresh connection (`AfterSeq == 0`), the handler subscribes to the bus **before** building the snapshot (copying the ordering-safety comment from `session_service.go:1996-1997` verbatim), then sends one `BacklogItemEvent` per currently-visible item with `is_snapshot: true`, then streams live events.
  - *Given* 3 existing `BacklogItem`s in storage, *When* a client calls `WatchBacklogItems({})` (no `after_seq`), *Then* the stream yields exactly 3 events with `is_snapshot: true` (one per item, as the appropriate "current state" variant — e.g. `item_updated` — since there's no prior state to diff against) followed by any subsequent live event.
- On a reconnect (`AfterSeq > 0`), the handler calls `eventBus.EventsSince(afterSeq)` and streams those instead of a fresh snapshot.
  - *Given* a client with `lastSeq == 500`, *When* it calls `WatchBacklogItems({AfterSeq: 500})` and the bus has events with `Seq` 501–510 still in the ring buffer, *Then* the stream yields exactly those 10 events (converted to `BacklogItemEvent`), then goes live.
- Filters (`status_filter`, `category_filter`) are applied both at snapshot/replay time and at live fan-out time.
  - *Given* `WatchBacklogItemsRequest{StatusFilter: ["in_progress"]}`, *When* a live event for an item with `Status: "done"` arrives on the bus, *Then* the handler does not call `stream.Send` for it.
- Every event delivered through the `after_seq`/`EventsSince` replay branch (Task 3.1.1c) has `is_snapshot` forced to `true`, even if it was `false` on the originally-published event — preventing double-delivery from manifesting as a double flash or double `aria-live` announcement.
  - *Given* a live event published during the race window between `Subscribe()` and the `EventsSince(afterSeq)` read is delivered via BOTH branches, *When* the client receives it via replay, *Then* `is_snapshot` is `true` on that copy (even if it was `false` when originally published), so the frontend never double-flashes/double-announces it.
**Files**: `server/services/backlog_service_events.go` (new file)

##### Task 3.1.1a: Define `backlogItemEventSender`, scaffold thin RPC wrapper + `watchBacklogItems` core method, subscribe-before-snapshot pattern (~5 min)
- New file. Add `type backlogItemEventSender interface { Send(*sessionv1.BacklogItemEvent) error }` — the narrow local interface that lets the core logic be tested without a real `connect.ServerStream[T]` (see Story 3.2.1 for why a fake `*connect.ServerStream[T]` itself is not constructible).
- The registered RPC method becomes a thin wrapper: `func (s *BacklogService) WatchBacklogItems(ctx context.Context, req *connect.Request[sessionv1.WatchBacklogItemsRequest], stream *connect.ServerStream[sessionv1.BacklogItemEvent]) error { return s.watchBacklogItems(ctx, req.Msg, stream) }`. This compiles with no adapter/conversion step: `*connect.ServerStream[sessionv1.BacklogItemEvent]` already has a `Send(*sessionv1.BacklogItemEvent) error` method, so it satisfies `backlogItemEventSender` structurally — Go interface satisfaction requires no explicit "implements" declaration.
- All actual logic (subscribe-before-snapshot, branch handling, live fan-out — Tasks 3.1.1b-d) lives in `func (s *BacklogService) watchBacklogItems(ctx context.Context, msg *sessionv1.WatchBacklogItemsRequest, sender backlogItemEventSender) error`, which every `stream.Send(...)` call below should be read as `sender.Send(...)` against.
- `eventCh, subID := s.eventBus.Subscribe(ctx); defer s.eventBus.Unsubscribe(subID)` — copy the ordering comment from `session_service.go:1996-1997`.
- Files: `server/services/backlog_service_events.go`

##### Task 3.1.1b: Implement fresh-snapshot branch (~5 min)
- Branch on `msg.AfterSeq == 0`: fetch the current visible item list (reuse whatever in-memory cache/listing method the existing `ListBacklogItems` RPC uses — check for an in-memory cache analogous to `s.reviewQueuePoller.GetInstances()` per stack.md §2; fall back to a DB query only if no cache exists), convert each to a `BacklogItemEvent` with `is_snapshot: true`, `sender.Send` each.
- Files: `server/services/backlog_service_events.go`

##### Task 3.1.1c: Implement `after_seq` replay branch (~4 min)
- Branch on `msg.AfterSeq > 0`: call `s.eventBus.EventsSince(msg.AfterSeq)`, convert each matching `*events.Event` (filter to `Type == EventBacklogItemChanged`) to `*sessionv1.BacklogItemEvent`, `sender.Send` each.
- **Force `is_snapshot: true` on every event sent through this branch, unconditionally, regardless of the original event's flag value.** After `convertEventToBacklogItemEvent` builds the message but *before* calling `sender.Send(...)`, explicitly set the built message's `is_snapshot` field to `true` on whichever oneof variant it populated (e.g. `converted.GetStatusChanged().IsSnapshot = true`). This closes the double-delivery/double-flash/double-`aria-live`-announce race: a live event published during the window between `Subscribe()` and the `EventsSince(afterSeq)` read can be delivered via *both* the live fan-out loop and this replay branch — forcing `is_snapshot: true` here guarantees the replay-branch copy is always treated as non-flash-worthy/non-announce-worthy by the frontend (Epic 6.1/6.2), even if the same event's live-branch copy (correctly) has `is_snapshot: false`.
- Files: `server/services/backlog_service_events.go`

##### Task 3.1.1d: Implement `convertEventToBacklogItemEvent` + live fan-out loop with filters (~5 min)
- Write `convertEventToBacklogItemEvent(event *events.Event) *sessionv1.BacklogItemEvent` switching on `event.BacklogItemPayload.Kind` to build the right oneof variant (mirrors `convertEventToProto`'s switch-on-`event.Type` pattern already used for session events).
- Main loop: `select { case <-ctx.Done(): return nil; case event, ok := <-eventCh: if !ok { return nil }; if event.Type != EventBacklogItemChanged { continue }; if !matchesFilters(event, msg) { continue }; sender.Send(convertEventToBacklogItemEvent(event)) }`.
- Files: `server/services/backlog_service_events.go`

##### Task 3.1.1e: Register `WatchBacklogItems` on the `BacklogService` struct / server routing (~3 min)
- Ensure `BacklogService` has access to `eventBus` (likely already present if `BacklogService` is constructed alongside `SessionService`; otherwise thread it through the constructor, checking `server/server.go`'s registration call).
- Files: `server/services/backlog_service.go` (constructor, if `eventBus` isn't already a field), `server/server.go` (if handler registration needs updating — verify whether ConnectRPC auto-registers new methods on an existing service or needs an explicit mount)

---

### Epic 3.2: Handler Tests
**Goal**: Directly exercise the handler's branching logic, not just "subscribe to the bus and assert" (pitfalls.md §5 — neither existing stream has this today) — via the `backlogItemEventSender` interface extracted in Task 3.1.1a, not a fake `connect.ServerStream[T]` (confirmed infeasible, see below).

#### Story 3.2.1: Direct `watchBacklogItems` core-logic test via a fake `backlogItemEventSender` (not a fake `connect.ServerStream[T]`)
**As a** developer testing the RPC handler's snapshot/replay/filter/fan-out branching, **I want** that logic callable and assertable without a real `connect.ServerStream[T]`, **so that** the handler's behavior is covered by fast, direct unit tests instead of being untestable or requiring a full RPC round-trip.

**Why a fake `connect.ServerStream[T]` doesn't work**: confirmed directly against this repo's vendored `connectrpc.com/connect v1.19.0` (`go.mod` line 6; source at `$(go env GOPATH)/pkg/mod/connectrpc.com/connect@v1.19.0/handler_stream.go:93-95`): `type ServerStream[Res any] struct { conn StreamingHandlerConn }` — a concrete struct with an unexported `conn` field, and the package exports no constructor for it (only internal-use `NewServerStreamHandler`/`NewServerStreamHandlerSimple` factories, which build the value themselves from a live RPC connection). There is no way outside the `connect` package to construct or substitute a value of this type, so Task 3.1.1a's original plan of a hand-rolled "fake `connect.ServerStream[T]`" was never buildable, not just hard to build.

**Why the fix is a narrow interface, not an `httptest.Server` integration test**: this repo already has both patterns in `server/services/*_test.go` — direct in-process calls against service methods (the dominant style: 34+ of 81 test files call handler methods directly with a plain `context.Context` and `*connect.Request[T]`, no HTTP server involved) and `httptest.Server`-backed tests reserved for cases that specifically need a real connection (`session_service_stream_terminal_test.go` needs real HTTP/2 for bidirectional streaming; `notification_service_test.go` needs a real localhost peer address for an origin check). No existing test in this repo drives `WatchSessions` or `WatchReviewQueue` — the two structurally closest streaming RPCs — through either approach, so there is no direct precedent to follow for *this* RPC shape specifically. Given that, and given this repo's general preference for testing logic directly over spinning up HTTP servers, extracting the core logic behind a narrow, consumer-defined interface (`backlogItemEventSender`, Task 3.1.1a) is the lower-risk, more-consistent choice: it keeps the test in-process and synchronous like the majority of this package's tests, and the interface is satisfied structurally by the real `*connect.ServerStream[T]` with zero production-code overhead (see Task 3.1.1a).
**Acceptance Criteria**:
- `backlogItemEventSender` (Task 3.1.1a) has exactly one method, `Send(*sessionv1.BacklogItemEvent) error`, and `*connect.ServerStream[sessionv1.BacklogItemEvent]` satisfies it without any wrapper/adapter code.
  - *Given* the registered `WatchBacklogItems(ctx, req, stream)` wrapper, *When* ConnectRPC invokes it, *Then* it calls `s.watchBacklogItems(ctx, req.Msg, stream)`, passing the concrete `*connect.ServerStream[...]` directly as the `backlogItemEventSender` interface value.
- A hand-rolled fake implementing `backlogItemEventSender`, capturing sent messages in a slice (guarded by a mutex — the live fan-out loop runs in a goroutine in Task 3.2.1d's test), enables calling `watchBacklogItems` directly in tests.
  - *Given* the fake sender and a `BacklogService` with 2 seeded items, *When* `svc.watchBacklogItems(ctx, &sessionv1.WatchBacklogItemsRequest{}, fakeSender)` runs for a bounded duration (context cancelled after a short deadline), *Then* `fakeSender.sent` contains exactly 2 snapshot events, each with `is_snapshot: true`, matching the 2 seeded items' IDs.
- A separate test exercises the `after_seq` branch.
  - *Given* the fake sender and `WatchBacklogItemsRequest{AfterSeq: N}` where the bus has 3 buffered events with `Seq > N`, *When* `watchBacklogItems` runs, *Then* `fakeSender.sent` contains exactly those 3 events, in `Seq` order, and zero snapshot events.
**Files**: `server/services/backlog_service_events.go` (interface + wrapper/core-method split, from Task 3.1.1a), `server/services/backlog_service_events_test.go` (new file)

##### Task 3.2.1a: Write fake `backlogItemEventSender` implementation (~4 min)
- `type fakeBacklogItemEventSender struct { mu sync.Mutex; sent []*sessionv1.BacklogItemEvent }` with `func (f *fakeBacklogItemEventSender) Send(e *sessionv1.BacklogItemEvent) error { f.mu.Lock(); defer f.mu.Unlock(); f.sent = append(f.sent, e); return nil }` — locked because Task 3.2.1d's test publishes concurrently while `watchBacklogItems` is live-looping in a goroutine.
- Files: `server/services/backlog_service_events_test.go`

##### Task 3.2.1b: Test fresh-snapshot branch via fake sender (~4 min)
- Seed 2 `BacklogItem`s in storage, call `svc.watchBacklogItems(ctx, &sessionv1.WatchBacklogItemsRequest{}, fakeSender)` directly (no RPC/HTTP involved) with a context that's cancelled after a short deadline so the live-loop branch returns; assert `fakeSender.sent` has exactly 2 events, `is_snapshot: true`, matching the seeded IDs.
- Files: `server/services/backlog_service_events_test.go`

##### Task 3.2.1c: Test `after_seq` replay branch via fake sender (~4 min)
- Seed 3 buffered bus events with `Seq > N`, call `svc.watchBacklogItems(ctx, &sessionv1.WatchBacklogItemsRequest{AfterSeq: N}, fakeSender)`; assert `fakeSender.sent` contains exactly those 3, in `Seq` order, zero snapshot events.
- Files: `server/services/backlog_service_events_test.go`

##### Task 3.2.1d: Test filter application on live fan-out via fake sender (~4 min)
- Run `svc.watchBacklogItems(ctx, &sessionv1.WatchBacklogItemsRequest{StatusFilter: [...]}, fakeSender)` in a goroutine; publish 2 events to the real bus (one matching `status_filter`, one not) while it is live-looping; assert (with a short polling wait, then cancel the context to stop the goroutine) that only the matching one reaches `fakeSender.sent`.
- Files: `server/services/backlog_service_events_test.go`

---

## Phase 4: Frontend Streaming Infrastructure

### Epic 4.1: `backlogItemsSlice` Redux Slice
**Goal**: A normalized store with a staleness-guarded upsert reducer, shared by all 4 consumers (per Pattern Decisions: Redux chosen over per-component local state).

#### Story 4.1.1: Create `backlogItemsSlice` with `upsertItem`/`removeItem` reducers
**Acceptance Criteria**:
- `upsertItem` drops an incoming item whose `updatedAt` is strictly older than the currently-stored item's `updatedAt` for the same id.
  - *Given* `backlogItemsSlice` state containing `{ "item-1": { id: "item-1", status: "review", updatedAt: "2026-07-21T10:00:05Z" } }`, *When* `upsertItem({ id: "item-1", status: "in_progress", updatedAt: "2026-07-21T10:00:02Z" })` is dispatched (an out-of-order/older event), *Then* the state is unchanged — `state["item-1"].status` remains `"review"`.
- `upsertItem` applies an incoming item whose `updatedAt` is newer or equal-but-different-instance (idempotent overwrite).
  - *Given* the same initial state, *When* `upsertItem({ id: "item-1", status: "done", updatedAt: "2026-07-21T10:00:10Z" })` is dispatched, *Then* `state["item-1"].status === "done"`.
- `removeItem` deletes the id from the map entirely (used for `BacklogItemRemovedEvent`, not upsert).
  - *Given* state containing `"item-1"`, *When* `removeItem("item-1")` is dispatched, *Then* `state["item-1"] === undefined`.
**Files**: `web-app/src/lib/store/backlogItemsSlice.ts` (new file), `web-app/src/lib/store/backlogItemsSlice.test.ts` (new file)

##### Task 4.1.1a: Scaffold slice with normalized `Record<string, BacklogItem>` state (~4 min)
- Mirror `sessionsSlice.ts`'s createSlice structure; state shape `{ items: Record<string, BacklogItem> }`.
- Files: `web-app/src/lib/store/backlogItemsSlice.ts`

##### Task 4.1.1b: Implement `upsertItem` reducer with real staleness guard (~5 min)
- Compare `action.payload.updatedAt` (or `seq` if threaded through the action) against `state.items[id]?.updatedAt`; skip the write if incoming is older. This is the fix for pitfalls.md #2 — do not copy `sessionsSlice.upsertSession`'s equal-only check.
- Files: `web-app/src/lib/store/backlogItemsSlice.ts`

##### Task 4.1.1c: Implement `removeItem` reducer (~2 min)
- Files: `web-app/src/lib/store/backlogItemsSlice.ts`

##### Task 4.1.1d: Unit tests for staleness guard, overwrite, and remove (~5 min)
- Cover: older event dropped, newer event applied, equal-timestamp idempotency, remove deletes key.
- Files: `web-app/src/lib/store/backlogItemsSlice.test.ts`

##### Task 4.1.1e: Register `backlogItemsSlice` in the root store (~3 min)
- Files: `web-app/src/lib/store/store.ts` (or wherever `sessionsSlice` is registered)

---

### Epic 4.2: `useWatchBacklogItems` Hook
**Goal**: The shared subscription hook, structurally mirroring `useReviewQueue.ts` per stack.md/features.md, plus new gap-detection logic pitfalls.md identifies as missing from every existing precedent.

#### Story 4.2.1: Implement stream connection lifecycle (AbortController, backoff, REST fallback)
**Acceptance Criteria**:
- On mount, the hook immediately issues a REST `listBacklogItems` call (dispatching results into `backlogItemsSlice`) before/alongside opening the stream — mirrors `useSessionService.ts`'s `listSessions()`-before-`watchSessions()` sequencing (pitfalls.md #1).
  - *Given* a component calling `useWatchBacklogItems({ statusFilter: ["in_progress"] })`, *When* it mounts, *Then* a `listBacklogItems` REST call fires within the same tick, populating the store even if the stream connection is still pending.
- On stream error (not `AbortError`), the hook retries with exponential backoff capped at 30s, `MAX_RETRIES = 5`, matching `useReviewQueue.ts`'s constants exactly.
  - *Given* the stream throws a non-abort error, *When* retry attempt 3 fires, *Then* the backoff delay is `Math.min(1000 * 2**3, 30000) = 8000`ms.
- After exhausting retries, the hook falls back to REST polling (interval matching the existing `BacklogItemDetail` 5s cadence as a floor) and exposes a way for a successful poll to attempt stream reconnection.
  - *Given* 5 failed retries, *When* the fallback poll's REST call succeeds, *Then* the hook attempts one more stream connection before continuing to poll.
**Files**: `web-app/src/lib/hooks/useWatchBacklogItems.ts` (new file)

##### Task 4.2.1a: Scaffold hook signature + initial REST fetch (~4 min)
- `function useWatchBacklogItems(filters: { statusFilter?: string[]; categoryFilter?: string[] }): { items: BacklogItem[]; connectionState: "connecting" | "live" | "reconnecting" | "polling" }`.
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.1b: Implement `AbortController`-guarded stream connect + `for await` consumption (~5 min)
- Copy `useReviewQueue.ts`'s AbortController-per-effect-run pattern; dispatch each received `BacklogItemEvent` to the appropriate `backlogItemsSlice` action based on its oneof variant (`status_changed`/`verdict_recorded`/`session_attached`/`item_updated` → `upsertItem`; `item_removed` → `removeItem`).
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.1c: Implement exponential backoff reconnect + `signal.aborted` race guard (~4 min)
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.1d: Implement REST-fallback polling after retry exhaustion (~4 min)
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

#### Story 4.2.2: Implement `after_seq` tracking + forward/backward gap detection
**Acceptance Criteria**:
- The hook tracks `lastSeqRef` from each received event's `seq` and passes it as `after_seq` on reconnect.
  - *Given* the hook has processed events up to `seq: 517`, *When* the stream disconnects and reconnects, *Then* the reconnect call passes `WatchBacklogItemsRequest({ afterSeq: 517, ...filters })`.
- Backwards-jump detection: if the first event's `seq` after a fresh connection is less than `lastSeqRef`, treat it as a server restart, reset `lastSeqRef` to 0, and trigger a full `listBacklogItems` refetch — mirroring `useSessionService.ts:730-742` exactly.
  - *Given* `lastSeqRef.current === 800`, *When* a fresh connection's first event arrives with `seq: 12` (server restarted, ring buffer reset), *Then* `needsFullResyncRef.current` is set and a full REST refetch fires before further events are trusted.
- Forward-gap detection (new logic, per pitfalls.md #4): if a live event's `seq` is not exactly `lastSeqRef.current + 1`, trigger the same full-refetch resync path.
  - *Given* `lastSeqRef.current === 100`, *When* the next live event arrives with `seq: 103` (events 101–102 were dropped by the bus's non-blocking, buffer-100 fan-out under backpressure), *Then* the hook triggers a full `listBacklogItems` refetch and updates `lastSeqRef.current` to 103 afterward.
**Files**: `web-app/src/lib/hooks/useWatchBacklogItems.ts`, `web-app/src/lib/hooks/useWatchBacklogItems.test.ts` (new file)

##### Task 4.2.2a: Implement `lastSeqRef` tracking + `after_seq` passthrough on reconnect (~4 min)
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.2b: Implement backwards-jump detection → full resync (~4 min)
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.2c: Implement forward-gap detection → full resync (~4 min)
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.2d: Unit tests for both gap-detection paths (~5 min)
- Mock the stream's async iterable to emit events with a seq gap / a backwards jump; assert `listBacklogItems` (mocked) is called and `connectionState` reflects a brief resync state.
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.test.ts`

#### Story 4.2.3: Idle-staleness backstop — periodic timer forces reconnect + refetch even with zero live events

**Real mechanism confirmed via reading `useSessionService.ts:944-997` (2026-07-21)**: `useSessionService.ts` implements two complementary idle-staleness checks, not one:
1. A **30s periodic backstop timer** (`useSessionService.ts:944-962`, `setInterval(..., 30_000)`): while the tab is enabled, on every tick, if the stream is currently *disconnected* (`!isConnectedRef.current`) **and** `Date.now() - lastEventTimeRef.current > 30_000`, it dispatches `setConnectionState("stale")` and — guarded by a `backstopTriggeredRef` so it only fires once per stale period — calls `watchSessionsRef.current?.(watchOptionsRef.current)` to force a reconnect.
2. A **15s staleness threshold on visibility/online events** (`useSessionService.ts:971-986`): on `visibilitychange`/`online`, after a 200ms debounce, it computes `isStale = lastEventTimeRef.current < Date.now() - 15_000`; if the stream is disconnected or stale, it resets the backoff and reconnects, dispatching `setConnectionState("stale")` first if the trigger was staleness rather than a plain disconnect.

Both checks key off a shared `lastEventTimeRef` (updated whenever any event — live *or* keepalive — is received) rather than any single subscription's open/closed state alone, so a connection that stays technically "open" but has gone silent (e.g. a dead TCP connection with no FIN) is still caught.

**Acceptance Criteria**:
- `useWatchBacklogItems` maintains a `lastEventTimeRef`, updated on every received `BacklogItemEvent` (snapshot or live).
  - *Given* the hook has just processed an event, *When* the next tick of the backstop timer runs, *Then* `Date.now() - lastEventTimeRef.current` is small and no reconnect fires.
- A 30s periodic timer mirrors `useSessionService.ts:944-962` exactly: while enabled, if the stream is currently disconnected and no event has arrived in the last 30s, it forces a reconnect (`watchBacklogItems` re-invocation) and a full `listBacklogItems` refetch, even though zero live `BacklogItemEvent`s have arrived to trigger the existing forward/backward gap-detection logic (Story 4.2.2), which only fires on receipt of an event and therefore cannot self-heal a connection that has gone completely silent.
  - *Given* `useWatchBacklogItems` mounted with `enabled: true`, the stream marked disconnected, and `lastEventTimeRef.current` more than 30s in the past, *When* the 30s backstop interval ticks, *Then* the hook sets `connectionState` to `"stale"`, triggers exactly one reconnect attempt (guarded against duplicate firing, mirroring `backstopTriggeredRef`), and that reconnect's success path issues a full `listBacklogItems` refetch — this happens even if zero `BacklogItemEvent`s were ever received during the whole idle period.
- A 15s staleness check runs on `visibilitychange`/`online`, mirroring `useSessionService.ts:971-986`: if the tab becomes visible (or the network comes back online) and the last event is more than 15s old, the hook forces a reconnect regardless of whether the stream reports itself as still "connected."
  - *Given* the tab was backgrounded and `lastEventTimeRef.current` is 20s in the past when it regains visibility, *When* the `visibilitychange` handler fires (after its debounce), *Then* the hook dispatches `connectionState: "stale"` and reconnects, resetting the backoff counter first.
**Files**: `web-app/src/lib/hooks/useWatchBacklogItems.ts`, `web-app/src/lib/hooks/useWatchBacklogItems.test.ts`

##### Task 4.2.3a: Add `lastEventTimeRef` update on every received event (~2 min)
- Update `lastEventTimeRef.current = Date.now()` in the same dispatch path added in Task 4.2.1b/4.2.2a, for both snapshot and live events.
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.3b: Implement 30s periodic backstop timer (~5 min)
- Port `useSessionService.ts:944-962`'s `setInterval(..., 30_000)` pattern verbatim: while disconnected and `Date.now() - lastEventTimeRef.current > 30_000`, set `connectionState` to `"stale"` and call the hook's reconnect function once (guarded by a `backstopTriggeredRef`-equivalent so it doesn't refire every tick while still stale).
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.3c: Implement 15s visibility/online staleness check (~4 min)
- Port `useSessionService.ts:971-986`'s debounced `visibilitychange`/`online` listener: on becoming visible or coming online, if `lastEventTimeRef.current < Date.now() - 15_000` or the stream is disconnected, reset backoff and reconnect.
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.2.3d: Unit/integration test — simulated silence past the timeout triggers a refetch (~5 min)
- Using fake timers, mount the hook, mark the stream disconnected (or simply let no event arrive), advance time past 30s, and assert: (1) the reconnect function was called exactly once, (2) a full `listBacklogItems` refetch fired, and (3) `connectionState` reflects `"stale"` before the reconnect resolves. Add a second test for the 15s visibility-change path using a mocked `visibilitychange` dispatch after advancing fake time past 15s.
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.test.ts`

---

## Phase 5: Consumer Wiring

### Epic 5.1: `/backlog` List Page
#### Story 5.1.1: Replace fetch-once with `useWatchBacklogItems`
**Acceptance Criteria**:
- `app/backlog/page.tsx` no longer holds a local `useState<BacklogItem[]>` populated by a one-time `listBacklogItems` call; it reads from `backlogItemsSlice` via `useWatchBacklogItems`.
  - *Given* `/backlog` is open and a reconciler transitions an item's status server-side, *When* the corresponding `BacklogItemStatusChangedEvent` arrives, *Then* the rendered list re-renders that item's card with the new status within the hook's live-event latency, with no page reload.
**Files**: `web-app/src/app/backlog/page.tsx`

##### Task 5.1.1a: Replace local state with `useWatchBacklogItems` + Redux selector (~5 min)
- Files: `web-app/src/app/backlog/page.tsx`

##### Task 5.1.1b: Remove now-dead one-time fetch code path (~3 min)
- Files: `web-app/src/app/backlog/page.tsx`

### Epic 5.2: `/backlog/board`
#### Story 5.2.1: Wire `BacklogBoard.tsx` to the same hook
**Acceptance Criteria**:
- `BacklogBoard.tsx` (which today receives items as props from its parent per features.md §4) either calls `useWatchBacklogItems` itself or receives live-updating props from a parent that does — Phase 3 planning confirms which; this plan chooses: **the board calls the hook directly**, matching the list page, so the board is independently live even if later reused outside the current parent.
  - *Given* `/backlog/board` is open, *When* a `BacklogItemSessionAttachedEvent` arrives for an item currently shown in the "in_progress" column, *Then* the card updates in place without moving columns (since session-attach doesn't change status).
**Files**: `web-app/src/components/backlog/BacklogBoard.tsx` (confirm exact path via Glob during implementation — features.md references it without a full path), parent page that currently passes items as props (confirm which page hosts it)

##### Task 5.2.1a: Locate `BacklogBoard.tsx` and its current parent/prop-passing (~3 min)
- Discovery must also resolve ux.md Surface 2's drag-and-drop interaction-precedence question (Open Question #4: "confirm whether `BacklogBoard` supports manual drag today; if so, define explicit precedence between a live event and an in-flight drag gesture"). Confirmed via grep (2026-07-21): `web-app/src/components/backlog/BacklogBoard.tsx` contains zero drag/drop/draggable/DnD-related code today — no manual drag-and-drop exists on this board. Conclusion: no interaction-precedence concern exists for this project; a live event arriving mid-column-membership-change cannot collide with a drag gesture that doesn't exist. If drag-and-drop is added to the board in a future project, that project must revisit this question, not assume it's still moot.
- Files: (discovery task — no edits)

##### Task 5.2.1b: Convert `BacklogBoard.tsx` to call `useWatchBacklogItems` directly (~5 min)
- Files: `web-app/src/components/backlog/BacklogBoard.tsx` (path TBD by 5.2.1a)

##### Task 5.2.1c: Remove now-unused prop-drilling from the parent page (~4 min)
- Files: parent page (path TBD by 5.2.1a)

### Epic 5.3: `BacklogItemDetail` — Remove `shouldPoll`, Add Edit-Mode Buffering
#### Story 5.3.1: Remove polling, subscribe by item id
**Acceptance Criteria**:
- The `shouldPoll` computation and its 5s interval (`BacklogItemDetail.tsx:245`) are deleted.
  - *Given* `BacklogItemDetail` open on an item with `status: "in_progress"` (today's zero-coverage blind spot per ux.md §5), *When* a reconciler transitions it to `"review"`, *Then* the detail panel updates without any polling interval firing — it comes from the live stream.
- The detail panel subscribes by item id regardless of the current list/board filter state (decoupling confirmed per ux.md §4).
  - *Given* `/backlog`'s list filter is set to `status: in_progress` only, *When* the open detail panel's item transitions to `"done"` (now filtered out of the list), *Then* the detail panel still shows the item's current (done) state — it does not disappear or freeze.
- Triage-progress updates — the one condition `shouldPoll` covered that has no status/verdict/session-attach analogue — now arrive via Story 2.2.5's `UpdateItemSessionTriageResult` hook instead of polling, with no regression in update latency versus the deleted 5s poll (requirements.md Success Metrics; pre-mortem.md P1 #5).
  - *Given* an item mid-triage with `triageStatus: running` open in `BacklogItemDetail` (today's `shouldPoll` condition, `BacklogItemDetail.tsx:245`), *When* the triage session reports incremental progress via `UpdateItemSessionTriageResult` (`session/storage_backlog.go:276`, called from `backlog_service_triage.go:1652`), *Then* `BacklogItemDetail` receives a live `BacklogItemUpdatedEvent` with `updated_fields: ["triageResultSummary"]` and re-renders the triage progress UI (`BacklogItemDetail.tsx:772-821`) without `shouldPoll`'s removed polling interval ever firing, at latency matching or beating the deleted 5s poll.
- If a `BacklogItemArchivedEvent` or `BacklogItemRemovedEvent` arrives for the currently-open item, the panel disables/hides its action buttons (Approve/Reopen/Delete/etc.) and shows a terminal-state `InlineNotice` banner instead of continuing to render controls that would now silently fail or 404 on click (ux.md UX AC #13, Surface 3 edge cases).
  - *Given* `BacklogItemDetail` open on item `"item-7"` with its normal action buttons visible, *When* a `BacklogItemArchivedEvent` for `"item-7"` arrives (fired from archiving the item in another tab), *Then* every action button on the panel becomes disabled/hidden and an `InlineNotice`-family banner reading "This item was archived elsewhere" appears in its place — the same outcome applies for a `BacklogItemRemovedEvent`, with copy reading "This item was removed elsewhere".
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.1a: Delete `shouldPoll` computation and its polling `useEffect` (~3 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.1b: Subscribe via `useWatchBacklogItems`/selector scoped to the open item id (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.1c: Handle `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent` for the open item (~5 min)
- Using the same item-id-scoped subscription as Task 5.3.1b, branch on the two terminal oneof variants: on receipt of either, set a local `terminalState: "archived" | "removed" | null` flag, disable/hide the panel's action buttons (Approve/Reopen/Delete/etc.) while that flag is set, and render an `InlineNotice` (Task 5.3.2b's component) with copy "This item was archived elsewhere" / "This item was removed elsewhere" respectively — closes the gap the Product Triad Review UX blocker flagged (stale action buttons that 404 after an archive/removal that happened while the panel was open). The backend already publishes both events unconditionally (Stories 2.2.2), so this is purely a frontend consumption gap.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Story 5.3.2: Edit-mode buffering
**Acceptance Criteria**:
- While `editMode === true`, an incoming live update for the open item is buffered (not applied to the visible form state) and a banner is shown offering to reload.
  - *Given* `BacklogItemDetail` in `editMode` with unsaved title changes, *When* a live `BacklogItemUpdatedEvent` arrives changing the server-side description, *Then* the form's title field is untouched, and an `InlineNotice` banner reading something like "This item changed elsewhere" appears with a "Reload" action.
  - *Given* the user then clicks "Reload" (or exits edit mode), *When* that happens, *Then* the buffered update is applied and the banner clears.
- If the user clicks **Save** while a buffered live update is pending for the open item, the save is not submitted immediately — a warn-before-overwrite confirmation is shown first, per ux.md's own recommendation ("warn, don't silently overwrite," Surface 6 edge cases / Open Question #1).
  - *Given* `BacklogItemDetail` in `editMode` with unsaved changes and a buffered `InlineNotice` already showing ("This item changed elsewhere"), *When* the user clicks **Save**, *Then* a confirm-style variant of the notice appears reading "Saving will overwrite a change made elsewhere — Reload first?" with **Save Anyway** and **Reload** actions, and the underlying save RPC is not called until the user picks one — clicking **Save Anyway** submits the original save (accepting the overwrite), clicking **Reload** discards the in-progress edit and applies the buffered update instead.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.2a: Add buffered-update state + suppress apply during `editMode` (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.2b: Add `InlineNotice` banner with "Reload" action (~5 min)
- Reuse `InlineError`'s informational styling per ux.md §4 — check whether a generic `InlineNotice` variant already exists on that component or needs a new sibling component.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/common/InlineNotice.tsx` (new, if `InlineError` can't be reused via a prop/variant)

##### Task 5.3.2c: Implement warn-before-overwrite confirmation on Save-while-buffered (~5 min)
- When the Save handler fires and a buffered update is currently pending (Task 5.3.2a's state is non-null), intercept the save: instead of calling the save RPC immediately, swap the `InlineNotice` banner to a confirm-style variant ("Saving will overwrite a change made elsewhere — Reload first?") with two explicit actions — **Save Anyway** (proceeds with the original save call, discarding the buffered server-side change) and **Reload** (discards the in-progress edit, applies the buffered update, and returns to view mode without saving). This resolves ux.md's Surface 6 Open Question #1 ("warn, don't silently overwrite") with a concrete implementation rather than leaving it open.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

### Epic 5.4: `BacklogItemPanel`
#### Story 5.4.1: Wire the 4th consumer
**Acceptance Criteria**:
- `BacklogItemPanel.tsx` (shown inside `SessionDetail`/`SessionDetailView` for a session's linked backlog item, per requirements.md's Open Questions resolution) subscribes via the same `useWatchBacklogItems` hook, scoped to its single linked item id.
  - *Given* a `SessionDetail` view showing a session whose linked backlog item transitions to `"review"` with a verdict recorded shortly after, *When* both events arrive, *Then* `BacklogItemPanel` reflects the new status and verdict without the session detail page being reloaded.
- If a `BacklogItemArchivedEvent` or `BacklogItemRemovedEvent` arrives for the panel's linked item, the panel disables/hides its action buttons and shows a terminal-state `InlineNotice` banner, mirroring `BacklogItemDetail`'s Task 5.3.1c handling exactly (ux.md UX AC #13's guarantee extends to this surface too, not just `BacklogItemDetail`).
  - *Given* `BacklogItemPanel` open inside `SessionDetail` showing its linked item, *When* a `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent` arrives for that item (fired from another tab), *Then* the panel's action buttons become disabled/hidden and an `InlineNotice` reading "This item was archived/removed elsewhere" replaces them, rather than continuing to render controls that would now silently fail or 404.
**Files**: `web-app/src/components/backlog/BacklogItemPanel.tsx` (confirmed path — see Task 5.4.1a; not `web-app/src/components/sessions/` as originally assumed)

##### Task 5.4.1a: Locate `BacklogItemPanel.tsx` and its current data-fetching (~3 min)
- Confirmed via Glob: the file lives at `web-app/src/components/backlog/BacklogItemPanel.tsx` (not `web-app/src/components/sessions/` as this plan originally guessed).
- Discovery must also explicitly resolve the connection-indicator reuse-vs-duplicate question (ux.md Open Question #3): check `web-app/src/components/sessions/SessionDetail.tsx`, `SessionDetailView.tsx`, and `SessionDetailBar.tsx` for any existing session-level "Live"/connection affordance the panel could inherit instead of mounting a second, competing `ConnectionIndicator`. Confirmed via grep (2026-07-21): none of those three files contain an existing live/connection-status indicator component (`SessionDetailView.tsx` only has an internal comment noting it re-syncs on `WatchSessions` pushes — no visible badge/dot). Conclusion: `BacklogItemPanel` mounts its own `ConnectionIndicator` (Task 6.2.1c) — there is nothing to reuse today. If a session-level indicator is added later, this decision should be revisited.
- Files: (discovery task — no edits)

##### Task 5.4.1b: Wire to `useWatchBacklogItems` scoped by linked item id (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemPanel.tsx`

##### Task 5.4.1c: Handle `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent` for the linked item (~5 min)
- Same handling as Task 5.3.1c, scoped to this panel's linked item id: on receipt of either event, disable/hide the panel's action buttons and render an `InlineNotice`-family terminal-state banner instead of leaving now-stale controls clickable.
- Files: `web-app/src/components/backlog/BacklogItemPanel.tsx`

---

## Phase 6: UX Polish

### Epic 6.1: Flash/Highlight on `BacklogItemCard`
#### Story 6.1.1: Non-remounting change flash
**Acceptance Criteria**:
- On receiving a live (non-`is_snapshot`) update for a card's item, the card briefly flashes/pulses (100–300ms, Linear/Jira-style per ux.md §1) without remounting (no `key` churn that would lose scroll/focus).
  - *Given* a rendered `BacklogItemCard` for item `"item-1"`, *When* a live status-change event for `"item-1"` is applied to the store, *Then* the card's className toggles a transient `.justChanged` class removed after ~250ms via a `useEffect` timeout, and the DOM node is not remounted (verified by an unchanged `key` prop and unchanged focus if the card was focused).
- The flash respects `prefers-reduced-motion` (falls back to an instant, non-animated but still-resetting background swap).
  - *Given* `prefers-reduced-motion: reduce`, *When* the same event arrives, *Then* the flash keyframe animation is replaced by an instant background-color set-then-clear with no transition.
**Files**: `web-app/src/components/backlog/BacklogItemCard.tsx`, `web-app/src/components/backlog/BacklogItemCard.css.ts`

##### Task 6.1.1a: Add `justChanged` transient state + timeout clear, keyed off `is_snapshot: false` events only (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 6.1.1b: Add `.justChanged` vanilla-extract style with `prefers-reduced-motion` guard (~4 min)
- Follow `.claude/rules/css-architecture.md` — new styles in `.css.ts`, tokens from `theme.css.ts`, no hardcoded colors.
- Files: `web-app/src/components/backlog/BacklogItemCard.css.ts`

### Epic 6.2: `aria-live` Regions + Live/Reconnecting Indicator
#### Story 6.2.1: Extend `GateVerdictBox`'s existing live region, add connection indicator
**Acceptance Criteria**:
- `GateVerdictBox.tsx`'s existing `role="status" aria-live="polite"` root gains `aria-atomic="true"` and is fed live verdict data instead of static props.
  - *Given* `GateVerdictBox` rendered read-only, *When* a `BacklogItemVerdictRecordedEvent` updates the verdict prop it receives, *Then* the live region announces the full new verdict text as one atomic update, not a partial diff.
- A small persistent "Live"/"Reconnecting…" indicator is shown wherever `useWatchBacklogItems`'s `connectionState` is exposed (list, board, detail).
  - *Given* the hook's `connectionState === "reconnecting"`, *When* the indicator renders, *Then* it shows "Reconnecting…" text/icon, not silently nothing.
**Files**: `web-app/src/components/backlog/GateVerdictBox.tsx`, `web-app/src/components/backlog/ConnectionIndicator.tsx` (new small component)

##### Task 6.2.1a: Add `aria-atomic="true"` to `GateVerdictBox`'s live region root (~2 min)
- Files: `web-app/src/components/backlog/GateVerdictBox.tsx`

##### Task 6.2.1b: Create `ConnectionIndicator` component (~5 min)
- Files: `web-app/src/components/backlog/ConnectionIndicator.tsx` (new), `.css.ts` sibling

##### Task 6.2.1c: Mount `ConnectionIndicator` in list, board, detail, and panel views (~5 min)
- Includes `BacklogItemPanel.tsx` alongside the other three — Task 5.4.1a's discovery pass confirmed `SessionDetail`/`SessionDetailView`/`SessionDetailBar` have no existing session-level "Live" indicator to reuse instead, so the panel needs its own mounted `ConnectionIndicator` like every other consumer (ux.md UX AC #19).
- Files: `web-app/src/app/backlog/page.tsx`, `web-app/src/components/backlog/BacklogBoard.tsx`, `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/backlog/BacklogItemPanel.tsx`

### Epic 6.3: Filtered-List Exit Transition
#### Story 6.3.1: Brief exit animation instead of instant removal
**Acceptance Criteria**:
- When an item's fields change such that it no longer matches the list/board's active filter, it briefly fades/slides out (respecting reduced-motion) instead of vanishing instantly.
  - *Given* `/backlog` filtered to `status: in_progress` showing item `"item-2"`, *When* `"item-2"` transitions to `"review"` (now filtered out), *Then* the card plays a ~200ms fade-out before being removed from the DOM, or (under `prefers-reduced-motion`) is removed instantly.
**Files**: `web-app/src/app/backlog/page.tsx` (or a shared list-rendering component it uses), `.css.ts` sibling

##### Task 6.3.1a: Implement exit-transition wrapper (e.g. via a small CSS transition on unmount, or existing animation utility) (~5 min)
- Files: `web-app/src/app/backlog/page.tsx` or shared list component

### Epic 6.4: Edit-Mode Buffering Banner Polish
*(Covered functionally in Story 5.3.2 — this epic is UX-review pass only.)*

#### Story 6.4.1: UX review pass on the buffering banner copy/placement
**Acceptance Criteria**:
- The banner text and "Reload" action are reviewed against ux.md §4's guidance (non-blocking, informational styling, not a modal/toast).
  - *Given* the banner implemented in Task 5.3.2b, *When* reviewed against `InlineError`'s existing informational variant, *Then* it visually matches that family (not styled as an error/danger state) and does not block interaction with the rest of the form.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 6.4.1a: Copy/placement pass on the banner from Task 5.3.2b (~3 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

## Phase 7: Testing & Verification

### Epic 7.1: Workspace Isolation — Positive Verification
#### Story 7.1.1: Test confirming `WatchBacklogItems` inherits per-process isolation
**Acceptance Criteria**:
- A test documents and confirms (per architecture.md §3 — this is verification of an existing property, not new isolation code) that two separate `*events.EventBus` instances never cross-deliver `BacklogItemEvent`s.
  - *Given* two independently constructed `*events.EventBus` instances, `busA` and `busB` (simulating two workspace processes), *When* `PublishItemChanged` is called through an adapter wired to `busA` only, *Then* a subscriber on `busB` never receives the event, confirming isolation is structural (one bus per process) rather than filter-based.
**Files**: `session/ent_repository_backlog_test.go` or `server/services/backlog_item_event_publisher_test.go`

##### Task 7.1.1a: Write the two-bus isolation test (~4 min)
- Files: `server/services/backlog_item_event_publisher_test.go`

### Epic 7.2: `MergeDatabase` Live-Leak Check
**Phase-ordering risk (acknowledged)**: this check runs in Phase 7, three phases after the repository methods it might need to modify (`TransitionBacklogItemStatus`, `UpdateBacklogItem`, and the other hooked methods) are effectively frozen following Phase 2. A full plan reorder to move this discovery earlier is likely too disruptive at this point in the project given how much of Phases 3-6 build on the Phase 2 hook signatures as-is. Contingency: if this discovery finds `MergeDatabase` needs a bypass parameter on the Phase 2 hooks, treat it as a fast-follow patch to those specific methods rather than blocking Phase 7 sign-off — the risk is scoped to one narrow, rare bulk-copy code path (`MergeDatabase`), not the primary hook mechanism used by every other caller.
#### Story 7.2.1: Confirm bulk-copy during `MergeDatabase` does not emit live `BacklogItemEvent`s
**Acceptance Criteria**:
- The flagged plausible leak path from features.md §6 is closed: `MergeDatabase` (`server/services/session_service.go:3041`) copying sessions/items from a source workspace into the current DB must not fire live `BacklogItemEvent`s for the merged-in items (they should appear on next fetch/reconnect, not as a flood of "just changed" flashes).
  - *Given* a `MergeDatabase` call copying 5 backlog items from a source workspace DB, *When* the merge runs against a repository wired with a live `itemChangePublisher`, *Then* zero `BacklogItemEvent`s are published during the merge (the underlying bulk-copy path must bypass or explicitly suppress the per-mutation publish hooks, e.g. via a direct bulk-insert SQL path rather than looping calls to `TransitionBacklogItemStatus`/`UpdateBacklogItem`).
**Files**: `server/services/session_service_test.go` (or wherever `MergeDatabase` tests live), `session/ent_repository_backlog.go` (if the bulk-copy path is found to route through the hooked methods, add a bypass flag/parameter)

##### Task 7.2.1a: Trace `MergeDatabase`'s actual backlog-item copy path (~5 min)
- Confirm whether it calls the 9 hooked repository methods in a loop (leak) or uses a separate bulk-insert path (already safe) — this determines whether a code fix is needed or only a confirming test.
- Files: (discovery — read `server/services/session_service.go` around line 3041 and wherever it copies backlog items)

##### Task 7.2.1b: Add bypass/suppression if the trace finds a leak, else write the confirming test (~5 min)
- If a leak is found: add a parameter (e.g. `skipEventPublish bool`) to the bulk-copy path's repository calls, or route through a raw bulk `INSERT`/`UPDATE` that doesn't go through the hooked methods.
- Files: `session/ent_repository_backlog.go` (if fix needed), `server/services/session_service_test.go` (test either way)

### Epic 7.3: Reducer Staleness Guard Tests
*(Already specified in Task 4.1.1d — cross-referenced here for coverage completeness.)*

#### Story 7.3.1: Confirm coverage maps to pitfalls.md #2's exact scenario
**Acceptance Criteria**:
- The out-of-order concurrent-publish scenario pitfalls.md #2 describes (`in_progress→review→in_progress` within milliseconds from two goroutines) is representable as a reducer test with two `upsertItem` dispatches arriving in the "wrong" order and asserting the later-`updatedAt` one wins regardless of dispatch order.
  - *Given* dispatch order `upsertItem({status: "in_progress", updatedAt: t2})` then `upsertItem({status: "review", updatedAt: t1})` where `t1 < t2`, *When* both have been dispatched, *Then* final state shows `status: "in_progress"` (the newer-`updatedAt` one), even though it was dispatched first — proving the guard is timestamp-based, not arrival-order-based.
**Files**: `web-app/src/lib/store/backlogItemsSlice.test.ts`

##### Task 7.3.1a: Add the out-of-order arrival test case (~3 min)
- Files: `web-app/src/lib/store/backlogItemsSlice.test.ts`

### Epic 7.4: Handler Branching Tests
*(Already specified in Epic 3.2 — cross-referenced here for coverage completeness.)*

---

## Phase 8: Documentation & Governance

### Epic 8.1: ADR-001
**Goal**: Record the Notifier/BacklogItemEvent separation decision per architecture.md §5.

#### Story 8.1.1: Write ADR-001
**Acceptance Criteria**:
- ADR-001 exists at `project_plans/backlog-event-driven-updates/decisions/ADR-001-separate-notifier-and-backlog-item-event-channels.md` in standard Context/Decision/Consequences/Alternatives format.
  - *Given* the ADR file, *When* a future contributor considers unifying the two channels, *Then* the ADR's Alternatives Considered section already documents why that was rejected, preventing re-litigation without new information.
**Files**: `project_plans/backlog-event-driven-updates/decisions/ADR-001-separate-notifier-and-backlog-item-event-channels.md`

##### Task 8.1.1a: Write ADR-001 (~5 min)
- (Written directly as part of this planning pass — see Step 5 deliverable.)
- Files: `project_plans/backlog-event-driven-updates/decisions/ADR-001-separate-notifier-and-backlog-item-event-channels.md`

### Epic 8.2: Feature Registry Updates
**Goal**: Comply with `.claude/rules/feature-registry.md` — every new RPC/UI feature needs a per-feature registry file.

#### Story 8.2.1: Register `WatchBacklogItems` and the 4 updated frontend consumers
**Acceptance Criteria**:
- `docs/registry/features/backend/watch-backlog-items.json` exists with `markerFound` reflecting whether a `// +api: backlog:watch` marker was added to the handler.
  - *Given* the handler in Task 3.1.1a has a `// +api: backlog:watch` comment, *When* `make registry-generate` runs, *Then* the aggregated backend registry includes this feature with `markerFound: true`.
- `make registry-generate` shows no net increase in `docs/registry/coverage-gaps.json` beyond what's justified (new features start `tested: false` until Phase 7 tests are linked via `testIds`).
  - *Given* Phase 3.2 and Phase 4.2.2's tests exist, *When* the registry entries are updated with matching `testIds`, *Then* `tested: true` for `WatchBacklogItems` and `useWatchBacklogItems`.
**Files**: `docs/registry/features/backend/watch-backlog-items.json` (new), `docs/registry/features/frontend/*.json` (new/updated for the 4 consumers)

##### Task 8.2.1a: Add `// +api: backlog:watch` marker to the handler (~2 min)
- Files: `server/services/backlog_service_events.go`

##### Task 8.2.1b: Create backend registry entry (~3 min)
- Files: `docs/registry/features/backend/watch-backlog-items.json`

##### Task 8.2.1c: Create/update frontend registry entries for the 4 consumers (~4 min)
- Files: `docs/registry/features/frontend/backlog-list.json`, `backlog-board.json`, `backlog-item-detail.json`, `backlog-item-panel.json` (create or update per existing entries)

##### Task 8.2.1d: Run `make registry-generate` and verify no unjustified coverage-gap increase (~3 min)
- Files: (generated aggregate files)
