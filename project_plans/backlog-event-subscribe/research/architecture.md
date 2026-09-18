# Architecture Research: backlog-event-subscribe

This is Phase 2 architecture research for a single new MCP tool
(`wait_for_backlog_event`) that lets a session block on an already-shipped
event stream for one backlog item, instead of polling. It builds on, and
does not re-derive, `project_plans/backlog-event-driven-updates/research/architecture.md`
(read in full) — that project designed and shipped `WatchBacklogItems`, the
event bus wiring, and confirmed workspace/multi-instance isolation
correctness. Citations below reference it by file:line rather than
re-verifying those conclusions.

**Skipped per task instructions**: Event-Command-Policy / EventStorming
table. This is not a new business-domain feature — it's a thin new
consumer of an existing event stream — so that grammar doesn't add value
here. (The prior project's architecture.md §4 already has the full
Event-Command-Policy table for the underlying backlog domain if needed.)

---

## 1. What the prior project already established (not re-derived)

- **Transport pattern**: `WatchBacklogItems` mirrors `WatchSessions` —
  subscribe to `pkg/events.EventBus` *before* building the initial
  snapshot/replay batch, to avoid a lost-event race
  (`backlog-event-driven-updates/research/architecture.md` §1, "Recommendation").
- **Replay mechanism**: `EventBus.EventsSince(afterSeq)` binary-searches a
  1-hour/10k-entry ring buffer keyed by a monotonic `Seq` assigned in
  `Publish` (architecture.md §1, citing `pkg/events/bus.go:30,68,115`).
- **Workspace/multi-instance isolation**: confirmed correct "for free" —
  one `*EventBus` instance per process, one process per workspace, zero
  scoping logic in the bus itself (architecture.md §3, their Epic 7.1).
  Nothing in this project changes that; a new in-process subscriber is
  scoped by the same property.
- **Six independent publish call sites** (status transition, non-status
  edit, archive, delete, verdict recorded, session attach) all funnel
  through repository-layer choke points and publish
  `events.EventBacklogItemChanged` events carrying a
  `BacklogItemEventPayload` (architecture.md §2). This project consumes
  that payload as-is; no new event kinds are needed.

---

## 2. `WatchBacklogItems` handler internals (verified, `server/services/backlog_service_events.go`, 388 lines)

Read in full. Key structural facts:

- **Two-layer split, deliberately**: the registered RPC method
  `WatchBacklogItems` (line 59) is a 3-line wrapper that just calls
  the unexported `watchBacklogItems(ctx, msg, sender)` (line 70), where
  `sender` is the narrow `backlogItemEventSender` interface (`Send(*sessionv1.BacklogItemEvent) error`,
  line 39-41) — not a concrete `*connect.ServerStream[T]`. The doc comment
  (line 9-19) explains this exists purely so tests can drive it with a fake
  sender, since `connect.ServerStream[Res]` has no exported constructor.
  **This is not an in-process Go API usable by the MCP tool** — it still
  requires building a `*sessionv1.WatchBacklogItemsRequest` proto message
  and produces wire-shaped `*sessionv1.BacklogItemEvent` protos via
  `convertEventToBacklogItemEvent` (line 228). Calling this in-process
  would mean constructing a throwaway proto request/fake sender just to
  immediately unmarshal the proto back into Go values — pure overhead with
  no benefit over going one level lower (see §3).
- **Subscribe-before-snapshot ordering** (line 79-88): `s.eventBus.Subscribe(ctx)`
  is called first, `defer s.eventBus.Unsubscribe(subID)` immediately after —
  this is the exact cleanup pattern the new tool must replicate.
- **Context-cancellation-driven exit** (line 163-181): the live fan-out loop
  is a `select` between `<-ctx.Done()` (returns nil — clean exit) and
  `<-eventCh` (channel close also returns nil). There is **no separate
  timeout mechanism inside this handler** — `WatchBacklogItems` relies
  entirely on the caller's `ctx` (or ConnectRPC client disconnect) to end
  the stream. `EventBus.Subscribe`'s own goroutine (`pkg/events/bus.go:59-63`)
  also unsubscribes on `ctx.Done()`, so the `defer s.eventBus.Unsubscribe(subID)`
  in the handler is technically redundant with that goroutine but kept for
  determinism/immediacy (avoids a race where the bus's own cancellation
  goroutine hasn't run yet).
- **No `item_id` filter exists on the RPC today.** `WatchBacklogItemsRequest`
  (`proto/session/v1/backlog.proto:752-762`) has exactly three fields:
  `status_filter`, `category_filter`, `after_seq` — no `item_id`. Filtering
  to one item is done by the *caller* inspecting
  `evt.BacklogItemPayload.Item.ID` after receiving each event; there is no
  server-side per-item filter to reuse. This directly answers Feasibility
  Risk item and Rabbit Hole "missed-event race": the new tool must do its
  own `payload.Item.ID == item_id` (or `payload.ItemID` for
  `BacklogChangeItemRemoved`, which has no `Item`) check per received event —
  a few lines of new filtering logic, not a proto change.

---

## 3. `pkg/events/bus.go` Subscribe/Publish API — direct in-process use is the right shape

Read in full (175 lines). The relevant API surface:

```go
func (eb *EventBus) Subscribe(ctx context.Context) (<-chan *Event, string)
func (eb *EventBus) Unsubscribe(id string)
func (eb *EventBus) EventsSince(afterSeq uint64) []*Event
```

`Subscribe` is a **plain, general-purpose, already-thread-safe in-process
API** — it takes a `context.Context` (cleanup wired via an internal
goroutine at line 59-63, no external plumbing needed), returns a receive-only
`chan *events.Event`, and requires no proto/ConnectRPC involvement at all.
`*events.Event` (aliased via `server/events/forward.go` from `pkg/events/types.go:90`)
is a plain Go struct with `Type`, `Seq`, `Timestamp`, and (for this project's
purpose) `BacklogItemPayload *events.BacklogItemEventPayload`
(`pkg/events/types.go:59-79`, fields: `Kind`, `Item *session.BacklogItemData`,
`OldStatus`/`NewStatus`, `SessionID`, `Verdict`, etc.) — every field the tool
needs (status, verdict, item snapshot) directly as Go values, no
unmarshaling.

**Conclusion, answering the task's central architectural question directly:
the new MCP tool should call `EventBus.Subscribe`/`Unsubscribe` directly,
in-process, bypassing `WatchBacklogItems`/ConnectRPC/HTTP entirely.**
Going through the RPC handler would mean: building a fake
`backlogItemEventSender` to capture proto messages, constructing a
`sessionv1.WatchBacklogItemsRequest`, and converting proto back to Go after
receipt — three layers of indirection to reach data that's already
available as a plain Go struct one call away. This is exactly the
"in-process pub/sub subscribe, not a network client" case the task called
out; `EventBus` was already designed as a plain Go API for in-process
subscribers (that's what `WatchBacklogItems`, `WatchSessions`, and
`ReactiveQueueManager`'s upstream generic-event consumption all already do
— none of them talk to the bus over the wire either).

---

## 4. Integration point: `backlogHandlers` already holds an `*events.EventBus` reference

Checked `server/mcp/tools_backlog.go`'s `backlogHandlers` struct
(line 118-158+) and its production wiring:

- **`eventBus *events.EventBus` is already a field** (line 121: `// optional;
  nil means notifications are disabled`), and it's **already populated in
  production** at `server/mcp/server.go:64`:
  ```go
  registerBacklogTools(s, &backlogHandlers{storage: storage, store: store,
      eventBus: eventBus, reviewStopper: svc, reviewTrigger: svc,
      enabledCheck: backlogEnabled, autoReopener: autoReopener, backlogSvc: backlogSvc})
  ```
  That `eventBus` traces back to the single process-wide `*events.EventBus`
  created in `server/dependencies.go` (the same instance passed to
  `BacklogService.SetEventBus`, `ReactiveQueueManager`, `WorkflowScheduler`,
  etc. — confirmed via `grep eventBus server/dependencies.go`, ~10 call
  sites all sharing one instance).
- **It's already used for `Publish`**, not `Subscribe`, today:
  `submit_triage_result`'s handler (tools_backlog.go:1688-1704) does
  `if h.eventBus != nil { ... h.eventBus.Publish(event) }` to fire an
  operator-facing notification. No existing MCP tool calls `Subscribe`.

**Conclusion: no new field, no new wiring, no new constructor plumbing is
needed.** `wait_for_backlog_event`'s handler is a new method on the
existing `*backlogHandlers` receiver (same pattern as every other tool in
this file — `getBacklogItem`, `reportProgress`, etc.), reusing
`h.eventBus` and `h.storage` (for an optional pre-check read, see §5). The
existing nil-guard convention (`if h.eventBus != nil`) should gate this
tool the same way `submit_triage_result` gates its publish — if
`h.eventBus` is nil (e.g. some test construction, though not production),
return a clear "event stream not available" tool error rather than a nil
pointer panic, mirroring `WatchBacklogItems`'s own
`connect.CodeUnimplemented` guard for the same nil case
(`backlog_service_events.go:75-77`).

Registration: one new `s.AddTool(mcpgo.NewTool("wait_for_backlog_event", ...), h.waitForBacklogEvent)` call
in `registerBacklogTools` (tools_backlog.go:1716+), following the exact
`mcpgo.NewTool(...) / mcpgo.WithString(...) / mcpgo.WithNumber(..., mcpgo.DefaultNumber, mcpgo.Min, mcpgo.Max)`
schema style already used by `wait_for_output` (`tools_terminal.go:117-135`)
for its `timeout_seconds` parameter.

### Feature registry (resolves requirements.md's open question)

Grepped `tools_backlog.go` and `tools_terminal.go` for `+api:`/`+feature:`
markers: **zero hits**. Those markers
(`.claude/rules/feature-registry.md`) are only used on ConnectRPC handlers
(e.g. `backlog_service_events.go:58`'s `// +api: backlog:watch` on
`WatchBacklogItems` itself) and React components — MCP tool handlers in
this codebase have never carried one. **No existing precedent requires a
registry entry for a new MCP tool**; this project does not need a new
`docs/registry/features/backend/*.json` file. (Flag this explicitly in
planning as a confirmed research finding, not an assumption, in case a
human reviewer wants to extend the convention — but nothing in the current
registry tooling scans `server/mcp/*.go` for markers, so skipping it is
consistent with every other MCP tool already in this file.)

---

## 5. Call sequence for `wait_for_backlog_event(item_id, timeout_seconds)`

1. **Validate args** — `item_id` required (existing `errResult(ErrInvalidArgument, ...)`
   convention, matches every other backlog tool); `timeout_seconds` optional,
   default/min/max via `mcpgo.WithNumber` schema (mirror `wait_for_output`'s
   `default 30, min 1, max 60` — this tool's SLO target is ≤2s p95 per
   requirements.md, so 60s is a generous ceiling, not the expected case).
2. **Guard**: if `h.eventBus == nil`, return a clear tool error immediately
   (no subscribe attempt) — matches `WatchBacklogItems`'s own nil guard.
3. **Subscribe first**: `eventCh, subID := h.eventBus.Subscribe(ctx)`, then
   `defer h.eventBus.Unsubscribe(subID)` immediately — before any state
   read, to close the missed-event race window named in requirements.md's
   Rabbit Holes. This is the same ordering `WatchBacklogItems` already uses
   (§2) and is the reason it's safe to skip `after_seq`/`EventsSince`
   entirely for this tool's default mode: subscribing before the state
   check means no event can land in the gap.
4. **Optional current-state check** (addresses the "read-then-listen race"
   Rabbit Hole directly): after subscribing, call
   `h.storage.GetBacklogItem(ctx, item_id)` once. If the caller only cares
   about "has this item's status/verdict already changed since I last
   looked" — which is the `/backlog/review`-style use case named in
   requirements.md — planning should decide whether this pre-check is
   in scope for v1, or deferred (the open question about exposing
   `after_seq` as a tool parameter is the more general version of this same
   concern — see below). Either way, subscribing before this read (step 3)
   is what makes it safe: any event published between "subscribe" and
   "read current state" is already sitting in `eventCh`, so it's never
   silently missed even if the tool doesn't act on the pre-check result
   itself.
5. **Bounded select loop**:
   ```go
   deadline := time.NewTimer(timeout)
   defer deadline.Stop()
   for {
       select {
       case <-ctx.Done():
           return <clean early-exit result>, nil   // caller cancelled / session disconnected
       case <-deadline.C:
           return <timeout result, matched=false>, nil   // mirrors wait_for_output's WAIT_TIMEOUT shape
       case evt, ok := <-eventCh:
           if !ok { return <event stream closed> }       // bus shutdown (server.Close())
           if evt.Type != events.EventBacklogItemChanged || evt.BacklogItemPayload == nil {
               continue
           }
           if !matchesItemID(evt.BacklogItemPayload, item_id) {  // new: no server-side item_id filter exists (§2)
               continue
           }
           return <matched result: event kind, old/new status, verdict summary, item snapshot>, nil
       }
   }
   ```
   `matchesItemID` needs a small helper because `BacklogChangeItemRemoved`
   payloads have `Item == nil` and carry the ID differently than the other
   six kinds (`BacklogItemEventPayload` has no top-level `ItemID` field —
   confirm exact shape in planning; likely `payload.Item.ID` for all kinds
   except removal, which may need its own field check, since
   `convertEventToBacklogItemEvent`'s `events.BacklogChangeItemRemoved`
   case (`backlog_service_events.go:307-315`) only sets `itemID` from
   `payload.Item.ID` too — so removal payloads may already carry `Item` for
   ID purposes even though downstream consumers ignore other item fields
   for that kind. Verify in planning/implementation, not assumed here.)
6. **Cleanup**: the `defer h.eventBus.Unsubscribe(subID)` from step 3 fires
   on every return path (match, timeout, ctx-cancel, channel-closed) —
   this is what prevents the goroutine/subscription leak named in
   requirements.md's Rabbit Holes and Non-functional Requirements. No
   manual cleanup needed beyond that one `defer`, since `Unsubscribe` is
   documented idempotent (`bus.go:145-146`) and the bus's own
   ctx.Done()-triggered goroutine (`bus.go:59-63`) makes a second/racing
   call to `Unsubscribe` harmless.

### On concurrent waiters (Rabbit Hole: "multiple concurrent waiters on the
same item")

Each `wait_for_backlog_event` call gets its **own** subscriber channel from
`EventBus.Subscribe` (`bus.go:51-66` allocates a fresh `chan *Event` and a
fresh `generateSubscriberID()` per call) — `Publish` fans out to every
subscriber's channel independently (`bus.go:86-93`). Two concurrent waiters
on the same or different `item_id` are therefore already safe with zero new
code: each has an independent channel, filters independently, and
unsubscribes independently. The only shared state is the bus's
`subscribers map` guarded by `eb.mu` — already thread-safe. No new locking
or coordination is needed in the MCP tool layer.

### On context propagation (does `ctx.Done()` actually fire on disconnect?)

`server/mcp/server.go:114` runs `mcpserver.NewStdioServer(...)` (stdio
transport) alongside an HTTP handler path (`NewHTTPHandler`, line 73). The
mcp-go library's `ToolHandlerFunc` signature is
`func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)`
(`mark3labs/mcp-go@v0.48.0/server/server.go:59`) — `ctx` is threaded from
the transport's own request-scoped context in both cases (HTTP request
context cancels on client disconnect; stdio ties to the read-loop context).
This is a materially different situation from `wait_for_output`
(`tools_terminal.go:390`, `func (th *terminalHandlers) waitForOutput(_ context.Context, ...)`)
which **discards its ctx parameter entirely** and busy-polls via
`time.Ticker` against a locally-computed `deadline` — it never checks
`ctx.Done()` at all, so a cancelled/disconnected caller doesn't actually
stop that loop early (it just keeps running to its own timeout, silently
discarding the result). **This is a precedent to improve on, not copy
verbatim**: `wait_for_backlog_event` has a real reason to honor `ctx.Done()`
(an open channel subscription to clean up, unlike `wait_for_output`'s
stateless scrollback polling), so its `select` should include `<-ctx.Done()`
as a first-class exit case, not silently ignore the context parameter the
way `wait_for_output` does.

---

## 6. Summary of concrete recommendations for planning

1. Implement `(h *backlogHandlers) waitForBacklogEvent(ctx, req) (*mcpgo.CallToolResult, error)`
   in `server/mcp/tools_backlog.go`, calling `h.eventBus.Subscribe(ctx)` /
   `h.eventBus.Unsubscribe(subID)` directly — no ConnectRPC/proto
   involvement, no new field on `backlogHandlers` (it already has
   `eventBus`).
2. No `item_id` filter exists on `WatchBacklogItemsRequest`/the bus — filter
   client-side (tool-side) on `evt.BacklogItemPayload.Item.ID == item_id`
   per received event; this is net-new logic, everything else is reuse.
3. Subscribe-before-any-read ordering (mirroring `WatchBacklogItems`) is
   what closes the missed-event race — no `after_seq`/`EventsSince` needed
   for v1 unless planning decides a resumable-wait parameter is in scope
   (open question in requirements.md; §5 above gives the case for deferring
   it since subscribe-first already covers the stated use case).
4. `select` on `ctx.Done()` / timer / `eventCh` with a `defer Unsubscribe`
   — do **not** copy `wait_for_output`'s pattern of ignoring `ctx` entirely;
   that pattern is safe there only because it has no resource to leak.
5. No feature-registry entry needed (no existing MCP tool has one; the
   marker convention doesn't scan `server/mcp/*.go`).
6. Concurrent waiters on the same/different items are already safe by
   construction (`EventBus.Subscribe` gives each caller an independent
   channel) — no new locking required.
