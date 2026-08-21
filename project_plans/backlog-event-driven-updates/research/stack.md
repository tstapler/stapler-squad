# Stack Research: backlog-event-driven-updates

## Summary

No new third-party dependencies are needed. This feature is a **mechanical mirror** of two
patterns that already ship in this repo: `WatchSessions` (generic session event stream) and
`ReviewQueueEvent`/`WatchReviewQueue` (a domain-specific event built on the same bus). Both
the Go event bus and the frontend streaming client are hand-rolled on top of existing,
already-current dependencies — `connectrpc.com/connect`, `@connectrpc/connect-web`, and
`@bufbuild/protobuf`. The correct move is to reuse these exact versions and patterns, not to
introduce anything new.

## 1. `pkg/events.EventBus` — hand-rolled, not a pub/sub library

`pkg/events/bus.go` is a **hand-rolled, dependency-free pub/sub bus**. Its only imports are
stdlib: `context`, `sync`, `sync/atomic`, `time`. No NATS, no Redis, no `golang.org/x/sync`
even. Key design:

- `EventBus` holds `subscribers map[string]chan *Event` guarded by `sync.RWMutex`, plus a
  ring buffer (`buf []bufferedEvent`) guarded by a separate `sync.Mutex` (`bufMu`) for replay.
- `Subscribe(ctx)` returns `(<-chan *Event, string)` — a receive-only channel and a
  crypto-random subscriber ID (`pkg/events/subscriber.go` uses `crypto/rand` + `encoding/hex`,
  not `google/uuid`, for the subscriber ID specifically — distinct from the `google/uuid`
  used elsewhere for event/notification IDs).
- `Publish(event *Event)` assigns a monotonic `Seq` (`atomic.Uint64`), appends to the ring
  buffer (TTL `eventBufTTL = time.Hour`, hard cap `eventBufMaxLen = 10_000`), then **fans out
  non-blocking**: each subscriber channel gets a `select { case ch <- event: default: }` — a
  slow subscriber has events dropped for it rather than blocking others.
- `EventsSince(afterSeq)` does a binary search over the ring buffer for reconnect replay.
- Cleanup is context-driven: `Subscribe` spawns a goroutine that calls `Unsubscribe` on
  `<-ctx.Done()`.

**`server/events` is a thin alias package**, not a second implementation:
```go
// server/events/forward.go
type EventBus = pkgevents.EventBus
var NewEventBus = pkgevents.NewEventBus
```
This exists solely to avoid an import cycle (`pkg/events` imports `session`, so `session`
itself cannot import `pkg/events` directly; consumers in `server/services` and
`server/services/backlog_notifier.go` import `server/events` instead, which is the same type).
**Any new backlog event type should follow this same alias pattern** if the storage layer
needs to reference the bus type without creating a cycle — check whether `session/backlog`
(or wherever `TransitionBacklogItemStatus` lives) has the same constraint before wiring in
the publish call.

**Conclusion**: no pub/sub library to add. A `BacklogItemEvent` will reuse the exact same
`*events.EventBus` instance already threaded through `SessionService` (see
`NewSessionService(storage session.InstanceStore, eventBus *events.EventBus)` at
`server/services/session_service.go:223`) — likely the same bus instance, since
`EventBusNotifier` already publishes backlog-adjacent events (bouncing/stuck alerts) through
it today (`server/services/backlog_notifier.go`).

## 2. `WatchSessions` — ConnectRPC server-streaming pattern

`server/services/session_service.go:1991` (`WatchSessions`), signature:
```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error
```
This is a standard **ConnectRPC server-streaming RPC** (`connectrpc.com/connect` `v1.19.0`,
pinned in `go.mod`) — no special version requirement beyond what's already in `go.mod`.
Pattern to mirror for `WatchBacklogItems`:

1. Subscribe to the bus **before** building the initial snapshot (`eventBus.Subscribe(ctx)`,
   deferred `Unsubscribe`) — avoids a race where an event fires between snapshot-build and
   subscribe.
2. Branch on `req.Msg.AfterSeq`: if set (reconnecting client), replay via
   `eventBus.EventsSince(afterSeq)`; otherwise send a fresh snapshot from an in-memory cache
   (here, `s.reviewQueuePoller.GetInstances()`, avoiding a full SQLite scan on every connect —
   the backlog equivalent should look for an analogous in-memory cache rather than hitting the
   DB per-connect).
3. Loop `select { case <-ctx.Done(): return nil; case event, ok := <-eventCh: ... }`, applying
   server-side filters (category/status) before `stream.Send(...)`.
4. Visibility filtering (`Hidden` field) is applied both at snapshot time and at real-time
   fan-out time — the backlog equivalent should apply the analogous workspace/instance-scoping
   filter at both points (this directly matters for the requirement's workspace-isolation
   feasibility risk).

No new proto plugin or connect feature is required — `WatchSessions` already proves
server-streaming works end-to-end with the current toolchain.

## 3. `ReviewQueueEvent`/`WatchReviewQueue` — exact shape to mirror

Proto lives in **`proto/session/v1/events.proto`** (not a separate file — despite being
"review queue", it's defined alongside `SessionEvent` et al. in `events.proto`), lines
312–382. Shape:

```protobuf
message ReviewQueueEvent {
  google.protobuf.Timestamp timestamp = 1;
  oneof event {
    ReviewQueueItemAddedEvent item_added = 2;
    ReviewQueueItemRemovedEvent item_removed = 3;
    ReviewQueueItemUpdatedEvent item_updated = 4;
    ReviewQueueStatisticsEvent statistics = 5;
  }
}
```
with `ReviewQueueItemAddedEvent { ReviewItem item; string trigger; bool is_snapshot; }`,
`ReviewQueueItemRemovedEvent { string session_id; string reason; }`,
`ReviewQueueItemUpdatedEvent { string session_id; ReviewItem item; repeated string
updated_fields; }`, and a `ReviewQueueStatisticsEvent` for aggregate counts.

Notable conventions worth copying verbatim for `BacklogItemEvent`:
- A top-level `timestamp` + a `oneof event` discriminated union (not a flat message with
  optional fields) — matches this repo's existing `SessionEvent` oneof pattern too.
- `is_snapshot: bool` on the "added" variant so the frontend can suppress notification/toast
  firing for items that are part of the initial catch-up snapshot vs. genuinely new
  real-time occurrences — directly relevant since the requirements call for status
  transitions **and** verdict recording **and** session-attach as distinct occurrences a
  `BacklogItemEvent` oneof should likely model as separate variants (e.g.
  `ItemStatusChanged`, `ItemVerdictRecorded`, `ItemSessionAttached`) rather than one generic
  "updated" blob, mirroring how `ReviewQueueItemUpdatedEvent.updated_fields` names what
  changed.
- The request message (`WatchReviewQueueRequest`, `session.proto:754`) carries filter fields
  (`priority_filter`, `reason_filter`, `session_ids`) plus two booleans (`include_statistics`,
  `initial_snapshot`) — a `WatchBacklogItemsRequest` should follow the same
  filter-fields-plus-behavior-flags convention, and should include an `after_seq`-equivalent
  field like `WatchSessionsRequest.AfterSeq` for reconnect replay (`ReviewQueueEvent` itself
  doesn't carry a `Seq` field the way `Event` in `pkg/events` does internally — confirm during
  planning whether reconnect replay is wanted for backlog events the way it's wanted for
  `WatchSessions`, since `ReactiveQueueManager` is a separate manager, not going through
  `pkg/events.EventBus`'s ring buffer at all).

**Important divergence to flag for the planning phase**: `ReviewQueueEvent` is *not*
published through `pkg/events.EventBus` — it's driven by a separate
`ReactiveQueueManager` (`server/review_queue_manager.go`) with its own
`AddStreamClient(ctx, filters) (<-chan *sessionv1.ReviewQueueEvent, string)` and
`publishToClients` fan-out, its own filter struct (`WatchReviewQueueFilters`), completely
parallel to `pkg/events.EventBus`. So there are actually **two** existing precedents in this
codebase, not one:
- `WatchSessions` → generic `pkg/events.EventBus` (ring-buffer replay, `Seq`-based).
- `WatchReviewQueue` → purpose-built `ReactiveQueueManager` (no ring buffer/replay seen in the
  reviewed code; filters baked into the manager, not the shared bus).

The requirements doc explicitly asks the ADR to decide "whether/how to fold the existing
`Notifier` alert-condition events into this new event model" — this stack finding sharpens
that decision: the ADR should also decide whether `WatchBacklogItems` rides `pkg/events.EventBus`
(gets replay/reconnect for free, consistent with `WatchSessions`) or gets its own manager like
`ReactiveQueueManager` (more filter flexibility, but reimplements fan-out/replay from scratch).
Given the requirement explicitly wants `WatchSessions`-equivalent latency and the `EventBus`
already has replay + the `Notifier` already publishes to it, **riding the shared `EventBus`
is the lower-risk default** unless research in the planning phase surfaces a concrete reason
`ReactiveQueueManager`-style per-stream filtering is needed for backlog items specifically.

## 4. Frontend streaming client pattern

**Versions** (`web-app/package.json`):
```
"@connectrpc/connect": "^2.1.1",
"@connectrpc/connect-web": "^2.1.1",
"@bufbuild/protobuf": "^2.11.0",
"@bufbuild/protoc-gen-es": "^2.11.0",   (devDependency, codegen only)
```
These are the current major-version lines for Connect-ES v2 / Protobuf-ES v2 (the v2 line
unified message-type and RPC-client codegen under `protoc-gen-es` — there is no separate
`protoc-gen-connect-es` plugin in this repo's `buf.gen.yaml`; service client typing comes
from the same generated `_pb.ts` file's `ServiceType` descriptor, consumed via
`createClient(ServiceType, transport)`). No version bump is needed for this feature — these
are already the versions every other streaming consumer in the repo uses.

**Client construction pattern** (seen in `FeatureFlagsContext.tsx`, `useUnfinishedWork.ts`,
`useDatabase.ts`, and transitively wherever `SessionServiceContext` builds its client):
```ts
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
const client = createClient(SessionService, transport);
```

**Streaming consumption pattern** — canonical example in
`web-app/src/lib/hooks/useReviewQueue.ts` (lines ~275–344), which is the closest existing
analog to the planned `useWatchBacklogItems` hook:
- `AbortController` stored in a ref; a fresh one is created per-effect-run and aborted on
  cleanup/filter-change.
- `for await (const event of stream)` — the generated client method returns an
  `AsyncIterable`, consumed directly with `for await...of`, no separate "subscribe" callback
  API.
- On stream error: distinguish `AbortError` (intentional, ignore) from real errors. Real
  errors trigger **exponential backoff reconnect**: `Math.min(1000 * 2**retries, 30000)`,
  capped at `MAX_RETRIES = 5`, via `setTimeout` + re-checking `signal.aborted` before
  reconnecting (guards against a stale timer firing after a newer effect run already
  cleaned up — noted inline as bug-class "F5").
- After exhausting retries, the hook falls back to REST polling and exposes a
  `streamReconnectRef` the polling path can invoke once a REST call succeeds, to attempt
  re-establishing the push stream opportunistically.
- Request objects are built with `create(WatchReviewQueueRequestSchema, {...})` (Protobuf-ES
  v2's `create()` factory pattern, not `new WatchReviewQueueRequest()`).

`useWatchBacklogItems` should copy this hook's structure closely: same
AbortController-per-effect lifecycle, same `for await` consumption, same backoff/retry
constants, same `create(...)` request-construction idiom. This also directly satisfies the
"remove `shouldPoll`" requirement — `useReviewQueue.ts` already demonstrates the
"push-primary, REST-fallback-secondary" hybrid this project should replicate rather than
inventing a new reconnect strategy.

## 5. `make proto-gen` — confirmed current and correct

`Makefile:397` target `proto-gen` runs `buf generate proto` (guarded by a timestamp-stamp
file `$(PROTO_STAMP)` that's invalidated when any `.proto` file or `protoc-gen-es` binary is
newer). `buf.gen.yaml` (v2 config) plugins:
```yaml
plugins:
  - remote: buf.build/protocolbuffers/go        # out: gen/proto/go
  - remote: buf.build/connectrpc/go             # out: gen/proto/go
  - local: web-app/node_modules/.bin/protoc-gen-es  # out: web-app/src/gen, target=ts
```
This regenerates both `gen/proto/go/session/v1/*.pb.go` (+ `*.connect.go` service stubs) and
`web-app/src/gen/session/v1/*_pb.ts`. `session/ent/generate.go` is unrelated — that's the ent
ORM schema generator (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
./session/ent/schema`), a completely separate generator from proto-gen; don't confuse the
two when the plan references "regenerate bindings." For this feature: add the new
`BacklogItemEvent`/`WatchBacklogItems`/`WatchBacklogItemsRequest` messages to
`proto/session/v1/events.proto` and `session.proto` respectively, then run `make proto-gen`
(or bare `buf generate proto` if iterating faster than the Makefile's stamp check allows) —
no config changes to `buf.gen.yaml` needed since both Go and TS output targets already exist
and cover any new message/RPC added to the existing `session/v1` package.

Related Go dependency versions already pinned in `go.mod` (no changes needed):
```
connectrpc.com/connect   v1.19.0
google.golang.org/protobuf v1.36.11
entgo.io/ent             v0.14.5   (unrelated to proto-gen, relevant only if
                                     TransitionBacklogItemStatus touches ent-generated code)
go 1.26.3
```

## Recommendations for Phase 3 (Planning)

1. **No new dependencies.** Everything needed (event bus, ConnectRPC streaming, frontend
   AsyncIterable consumption, backoff/reconnect) already exists in-repo at current versions.
2. **Decide bus vs. manager in the ADR** (see §3): default recommendation is to publish
   `BacklogItemEvent` through the existing shared `*pkg/events.EventBus` (via the
   `server/events` alias, same as `EventBusNotifier` does today) rather than building a
   second `ReactiveQueueManager`-style parallel manager — this gets ring-buffer replay
   (`AfterSeq`) for free and keeps one event-plumbing mental model instead of two.
3. **Model the oneof by concern, not generically**: mirror `ReviewQueueItemAddedEvent` /
   `...RemovedEvent` / `...UpdatedEvent`'s per-concern message split — likely
   `BacklogItemStatusChangedEvent`, `BacklogItemVerdictRecordedEvent`,
   `BacklogItemSessionAttachedEvent` as distinct oneof variants, each carrying
   `is_snapshot: bool` for the same replay/notification-suppression reason
   `ReviewQueueItemAddedEvent` does.
4. **Frontend hook**: base `useWatchBacklogItems` directly on `useReviewQueue.ts`'s
   AbortController/backoff/`for await`/REST-fallback structure — it's the most complete
   existing reference (more so than `useNotificationHistory.ts`, which is a simpler
   consumer).
5. **Publish-call placement**: since the requirement explicitly flags "hooking the publish
   call into the storage layer touches a wide surface of call sites" as a feasibility risk,
   and `pkg/events` cannot be imported directly from `session`/storage-layer packages (import
   cycle — same constraint that produced the `server/events` alias), the planning phase needs
   to resolve *where* the publish call can structurally live (e.g. an injected
   notifier/callback interface on the storage type, analogous to how `session.Notifier` /
   `EventBusNotifier` already solve this for the existing bouncing/stuck alerts) rather than
   a direct import.
