# Build vs. Buy: backlog-event-driven-updates

**Agent**: 6 (Build vs. Buy)
**Date**: 2026-07-21

## Question

Should backlog item change notifications to the frontend be built from scratch, sourced from an
external library/service, or copied/adapted from this codebase's existing `ReviewQueueEvent` /
`WatchReviewQueue` pattern?

## Evidence gathered

- `pkg/events/bus.go` — `EventBus` is a real, complete implementation: thread-safe pub/sub over Go
  channels, per-subscriber non-blocking fan-out (slow subscribers get events dropped, not blocked),
  a ring buffer (`eventBufTTL` = 1hr, `eventBufMaxLen` = 10,000) with monotonic `Seq` numbers and
  binary-search `EventsSince(afterSeq)` for reconnect replay. This is not a stub — it has TTL
  pruning, a hard size cap, and idempotent `Unsubscribe`/`Close`.
- `server/services/session_service.go`'s `WatchSessions` (line ~1991) is a working ConnectRPC
  server-streaming handler: subscribes to the bus before building the initial snapshot (avoids a
  lost-event race), replays missed events via `AfterSeq`/`EventsSince` on reconnect, otherwise
  sends a fresh snapshot from the in-memory poller cache, then streams live events with
  request-scoped filters. Genuinely shipped, not aspirational.
- `server/review_queue_manager.go`'s `ReactiveQueueManager` is the domain-specific layer built on
  top of the same bus: it subscribes once (`Start`), reacts to interaction events
  (`handleUserInteraction`, `handleApprovalResponse`, etc.), and separately implements
  `session.ReviewQueueObserver` (`OnItemAdded`, `OnItemRemoved`, `OnQueueUpdated`) to convert
  domain-level queue mutations into `sessionv1.ReviewQueueEvent` proto messages and fan them out to
  per-client `streamClients` (a `map[string]*reviewQueueStreamClient`, each with its own filtered
  buffered channel) — a second, smaller pub/sub layer scoped to this one RPC, independent of the
  generic bus's subscriber list.
- `server/services/review_queue_service.go`'s `WatchReviewQueue` handler (line ~211) is ~30 lines:
  build filters from the request, call `reactiveQueueMgr.AddStreamClient(ctx, filters)`, loop
  `stream.Send` until the channel closes or context is done.
- Proto shape (`proto/session/v1/events.proto`): `ReviewQueueEvent` is a `oneof` of
  `ItemAdded`/`ItemRemoved`/`ItemUpdated`/`Statistics` sub-messages, each carrying a domain item
  (`ReviewItem`) or a session/reason ID — a template that maps near 1:1 onto a `BacklogItemEvent`
  carrying `BacklogItem`/item ID instead.
- Frontend precedent: `web-app/src/lib/hooks/useReviewQueue.ts` and
  `web-app/src/lib/store/reviewQueueSlice.ts` already wrap `WatchReviewQueue` in a hook + Redux
  slice pattern that `/backlog`, `/backlog/board`, and `BacklogItemDetail` could reuse the shape of
  for a new `useBacklogEvents`-style hook.
- Storage-layer publish requirement: today `EventBus.Publish` calls originate almost entirely from
  the **service** layer (`server/services/*.go`, `server/review_queue_manager.go`,
  `server/mcp/tools_backlog.go`) — `session/storage.go`'s `Storage` struct currently holds only a
  `repo Repository` field, no `EventBus` reference, and `session/storage_backlog.go` /
  `session/backlog_lifecycle.go` publish nothing today. The requirement's "publish from storage,
  not just the RPC handler" is a real gap the existing session/review-queue path doesn't fully
  close either — this will need a `Storage`-layer publish hook (or a thin observer the storage
  layer calls into) regardless of which option is chosen below; it is new wiring, not something to
  source externally.

## Option 1: External OSS library/framework for server push (SSE, NATS, Redis pub/sub, GraphQL subscriptions, hosted realtime)

**Pros**
- SSE is simpler than WebSockets for one-directional server→client push and has broad browser
  support without a client library.
- NATS/Redis pub/sub scale to multi-process, multi-host fan-out.
- GraphQL subscriptions give typed, declarative client query shape.

**Cons**
- Stapler Squad is a **single-process, self-hosted Go binary** (per requirements.md constraints —
  "Internal, single-operator tool — NOT a multi-tenant SaaS"). NATS/Redis pub/sub solve
  cross-process/cross-host fan-out, a problem this app does not have — introducing them would mean
  running and operating an extra broker process for a single-operator desktop-adjacent tool.
  SSE would still require building the same server-side event routing (subscribe/publish/filter)
  this app already has via `pkg/events.EventBus` — SSE is a wire-protocol swap, not a replacement
  for the fan-out logic itself, and ConnectRPC server-streaming (already used everywhere else in
  this app — `WatchSessions`, `WatchReviewQueue`, `StreamTerminal`) already gives duplex-capable,
  typed, proto-schema'd streaming over HTTP/2 that the frontend already knows how to consume.
  GraphQL subscriptions would require introducing an entire GraphQL layer next to the existing
  ConnectRPC API for one feature — pure architectural inconsistency for no functional gain.
- Every one of these options solves "push events across process/host boundaries reliably," which
  is not the problem here: the event source (backlog state change) and the event sink (ConnectRPC
  stream handler) already live in the same Go process and share `pkg/events.EventBus` today.

**Verdict: Not recommended.** The actual mechanism problem (fan out an in-process state change to
N open browser tabs) is already solved in this exact binary, twice.

## Option 2: SaaS / managed realtime service (Pusher, Ably, Supabase Realtime)

**Pros**
- Offloads WebSocket/SSE connection scaling and reconnect logic to a vendor.
- Fast to prototype if starting from zero infrastructure.

**Cons**
- Requires network egress from the self-hosted binary to a third-party service for what is
  currently a fully local, offline-capable feature — every other realtime feature in this app
  (sessions, review queue, terminal streaming) works with zero external network dependency.
  Introducing one now for backlog events alone would be an inconsistent, single-feature exception.
  and a new operational dependency and possible cost for a tool with "no external
  compliance/regulatory constraint" and "single-operator" scope per the requirements — there's no
  multi-tenant or multi-region delivery problem to justify it.
- Adds an auth/API-key management surface and a new failure mode (vendor outage silently breaks
  the one feature routed through it) to a codebase that otherwise has no such dependency.

**Verdict: Not recommended.** No aspect of this problem (self-hosted, single-operator, single
process, LAN/localhost UI) matches what a hosted realtime service is for.

## Option 3: LLM-generated implementation vs. adapting the existing `ReviewQueueEvent`/`WatchReviewQueue` pattern

Assessed by tracing what would actually differ if `WatchBacklogItems` were built by copying
`ReviewQueueEvent`/`WatchReviewQueue`/`ReactiveQueueManager` mechanics:

| Piece | ReviewQueue version | Backlog version | Change needed |
|---|---|---|---|
| Proto event `oneof` | `ItemAdded`/`ItemRemoved`/`ItemUpdated`/`Statistics`, each wrapping `ReviewItem` | Same shape, wrapping `BacklogItem` (already defined in `session/domain/backlog.go` / ent schema) | Mechanical: new message names, same oneof structure |
| RPC handler (`WatchReviewQueue`, ~30 lines) | Delegates entirely to a manager's `AddStreamClient`/`RemoveStreamClient` | Same delegation shape | Mechanical: swap manager type, request/response proto types |
| Manager (`ReactiveQueueManager`) fan-out to `streamClients` map with per-client buffered channel + filter | Filters on priority/reason/session ID | Filters would likely be on status/tag/category instead | Mechanical structure identical; only the filter *fields* differ — same `map[string]*xClient` + `publishToClients` + `eventMatchesFilters` shape |
| Publish trigger | `session.ReviewQueueObserver` interface (`OnItemAdded`/`OnItemRemoved`/`OnQueueUpdated`) called by `session.ReviewQueue`'s own mutation methods | New observer interface called from backlog storage mutation methods (`session/storage_backlog.go`, `session/backlog_lifecycle.go`) | **This is the one genuinely new piece** — no backlog-side observer/publish hook exists yet (see storage-layer gap noted above). Requires adding an observer call analogous to `ReviewQueue`'s, not adapting an existing one. |
| Frontend hook (`useReviewQueue.ts` + `reviewQueueSlice.ts`) | Redux slice + hook wrapping the ConnectRPC stream | New `useBacklogEvents`-shaped hook/slice, shared across `/backlog`, `/backlog/board`, `BacklogItemDetail` | Mechanical: same hook shape, different action/event types; "shared across 3 call sites" is new *usage*, not new *mechanism* |

Everything except the storage-layer publish hook is a close-to-mechanical rename-and-retype of
proven code already exercised in production by `WatchSessions` and `WatchReviewQueue`. The backlog
domain doesn't need a structurally different mechanism — `BacklogItem` is a data shape of
comparable complexity to `ReviewItem`, and the same "mutation method calls an observer, observer
converts to proto, fan out to a filtered per-client channel map" pattern applies directly.

An LLM asked to build this "from scratch" with no awareness of the existing pattern would very
likely reinvent a weaker version of `pkg/events.EventBus` (no ring buffer/replay, no TTL pruning,
naive blocking fan-out) or under-scope the reconnect story (`AfterSeq`/`EventsSince` replay is easy
to omit entirely and only surfaces as a bug on client reconnect/network blip, not in casual
testing).

**Verdict: Recommended — adapt the existing pattern, not a fresh LLM implementation.** The pattern
is proven within this codebase (two shipped call sites), the adaptation is mechanical for ~80% of
the surface area, and the only genuinely new code (the storage-layer publish hook) is scoped and
small regardless of approach chosen.

## Option 4: Fork/adapt recommendation

This is not "fork an external project" — it's "copy this repo's own internal pattern file-for-file
and adapt names/types, vs. write fresh with the pattern in mind."

**Recommendation: Copy-and-adapt, concretely:**

1. **Proto** (`proto/session/v1/events.proto`): add `BacklogItemEvent` message mirroring
   `ReviewQueueEvent`'s oneof shape (`BacklogItemAddedEvent`/`BacklogItemUpdatedEvent`/
   `BacklogItemRemovedEvent`, each wrapping the existing `BacklogItem` proto message) plus a
   `WatchBacklogItems` streaming RPC in `session.proto`, run `make proto-gen`.
2. **Storage-layer publish hook**: add an observer interface (e.g. `BacklogQueueObserver` or reuse
   the existing `ReviewQueueObserver`-style naming convention) that `session/storage_backlog.go`'s
   and `session/backlog_lifecycle.go`'s mutation methods call on create/update/status-transition —
   this is the one piece with no existing analog to copy verbatim, since `Storage` doesn't hold an
   `EventBus` reference today. Size this as its own task in the implementation plan.
3. **Manager**: new type (e.g. `BacklogEventManager`) copying `ReactiveQueueManager`'s
   `streamClients map[string]*xClient` + `AddStreamClient`/`RemoveStreamClient`/
   `publishToClients`/`eventMatchesFilters` structure, filtering on whatever backlog-relevant
   fields matter (status, tag, category) instead of priority/reason.
4. **RPC handler**: new `WatchBacklogItems` in a backlog service file, same ~30-line delegate-to-
   manager shape as `WatchReviewQueue`.
5. **Frontend**: new shared hook (e.g. `useBacklogEvents.ts`) + Redux slice modeled directly on
   `useReviewQueue.ts`/`reviewQueueSlice.ts`, consumed by `/backlog`, `/backlog/board`, and
   `BacklogItemDetail.tsx` per the requirement's explicit scope.

Do **not** reach for any external library, broker, or hosted service for the transport mechanism —
ConnectRPC server-streaming over the in-process `pkg/events.EventBus` (or a sibling in-process
manager built the same way) is already the established, working, single-process-appropriate
pattern in this codebase, proven by `WatchSessions` and `WatchReviewQueue` running in production
today.

## Summary Table

| Option | Verdict |
|---|---|
| External OSS mechanism (SSE/NATS/Redis pub-sub/GraphQL subscriptions) | Not recommended |
| Hosted/SaaS realtime (Pusher/Ably/Supabase Realtime) | Not recommended |
| Fresh LLM-generated implementation, no pattern awareness | Not recommended |
| Copy-and-adapt existing `ReviewQueueEvent`/`WatchReviewQueue`/`ReactiveQueueManager` pattern | **Recommended** |
