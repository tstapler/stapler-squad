# Stack Research: backlog-event-subscribe

Agent 1 (Stack researcher), SDD Phase 2.

## 1. Precedent: `wait_for_output` (server/mcp/tools_terminal.go:117-496)

Registration (`tools_terminal.go:116-135`):

```go
s.AddTool(
    mcpgo.NewTool("wait_for_output",
        mcpgo.WithDescription("..."),
        mcpgo.WithString("session_id", mcpgo.Description("..."), mcpgo.Required()),
        mcpgo.WithString("pattern", mcpgo.Description("..."), mcpgo.Required()),
        mcpgo.WithNumber("timeout_seconds",
            mcpgo.Description("How long to wait in seconds (default 30, max 60)"),
            mcpgo.DefaultNumber(30),
            mcpgo.Min(1),
            mcpgo.Max(60),
        ),
    ),
    th.waitForOutput,
)
```

Handler shape (`waitForOutput`, `tools_terminal.go:390-496`):
- Reads `timeout_seconds` via `args["timeout_seconds"].(float64)`, clamps to a max (60s here).
- Computes `deadline := time.Now().Add(...)`.
- **Polls, does not block on a channel**: `time.NewTicker(time.Second)` + `for { ...check...; if time.Now().After(deadline) { return timeout result }; <-ticker.C }`. This is polling scrollback bytes every second, not event-driven — appropriate for terminal output (no event bus for PTY bytes) but not the pattern to copy for our case, since a real event channel exists.
- On timeout it returns a **success result with an embedded error** (`MCPResult{Success: true, Error: &MCPError{Code: "WAIT_TIMEOUT", ...}}`) rather than a tool-call error — i.e. timeout is a normal, structured outcome, not a Go `error` return. `wait_for_backlog_event` should follow the same convention (matches requirements.md's "returns the event or a timeout result").
- Handler signature: `func (th *terminalHandlers) waitForOutput(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)` — note it discards the incoming `ctx` entirely (uses `time.Now()`/`time.NewTicker` instead of `context.WithTimeout`). Other handlers in the same file (`writeToSession`, `sendControl`) **do** use the incoming `ctx` via `context.WithTimeout(ctx, ...)` + `select` on an error channel — that `select`-based pattern (not the polling one) is the closer template for adapting a channel-based wait, and it already propagates caller cancellation.

## 2. WatchBacklogItems / event bus internals (server/services/backlog_service_events.go)

- `WatchBacklogItems` (exported, the ConnectRPC RPC) is a thin wrapper: `return s.watchBacklogItems(ctx, req.Msg, stream)`. The real logic lives in unexported `(s *BacklogService) watchBacklogItems(ctx, msg, sender backlogItemEventSender) error`, deliberately kept unexported and sending through a narrow interface so it's unit-testable without a real RPC round-trip (see file's header comment). Both `watchBacklogItems` and `backlogItemEventSender` are unexported — **not callable from `server/mcp` package**, so this exact function cannot be reused directly across the package boundary.
- The actual primitive underneath is `*events.EventBus` (aliased in `server/events/forward.go` as `type EventBus = pkgevents.EventBus`, so `server/events.EventBus` and `pkg/events.EventBus` are the literal same type — no adapter needed regardless of which import path is used).
- `EventBus` API (`pkg/events/bus.go`):
  ```go
  func NewEventBus(bufferSize int) *EventBus
  func (eb *EventBus) Subscribe(ctx context.Context) (<-chan *Event, string)
  func (eb *EventBus) Publish(event *Event)
  func (eb *EventBus) EventsSince(afterSeq uint64) []*Event
  func (eb *EventBus) Unsubscribe(id string)
  func (eb *EventBus) SubscriberCount() int
  func (eb *EventBus) Close()
  ```
  `Subscribe` returns a receive-only `<-chan *Event`; caller must `defer eb.Unsubscribe(subID)` — this is the exact cleanup call needed to satisfy the "must clean up the stream subscription on timeout/cancellation/disconnect" scope requirement.
- Relevant exported types (`pkg/events/types.go`), all directly usable from the `mcp` package:
  - `EventType` const `EventBacklogItemChanged`
  - `BacklogChangeKind` consts: `BacklogChangeStatusTransition`, `BacklogChangeVerdictRecorded`, `BacklogChangeSessionAttached`, `BacklogChangeItemUpdated`, `BacklogChangeItemArchived`, `BacklogChangeItemRemoved`, `BacklogChangeTriageProgressUpdated`
  - `BacklogItemEventPayload{Kind, Item, OldStatus, NewStatus, UpdatedFields, SessionID, ArchivedAt, RemovedReason, Verdict, ...}` — carries `Item *session.BacklogItemData` (has `.ID`), so filtering an incoming `*events.Event` down to "does this concern item_id X" is a one-line check: `evt.BacklogItemPayload.Item.ID == itemID` (plus the `evt.Type == events.EventBacklogItemChanged` guard already shown in `watchBacklogItems`'s own live-fan-out loop, `backlog_service_events.go:163-181`).

### Same-process confirmation (VERIFIED)

`server/mcp/tools_backlog.go`'s `backlogHandlers` struct (`tools_backlog.go:118-190`) already holds **both**:
- `eventBus *events.EventBus` (field, `tools_backlog.go:121`)
- `backlogSvc *services.BacklogService` (field, `tools_backlog.go:138`, comment: "Held as the concrete type ... to match this package's existing pattern")

and wiring in `server/mcp/server.go:64` passes the **same live `*events.EventBus` and `*services.BacklogService` instances** used by the ConnectRPC `BacklogService` into `backlogHandlers` at server construction:
```go
registerBacklogTools(s, &backlogHandlers{storage: storage, store: store, eventBus: eventBus, ..., backlogSvc: backlogSvc})
```
The MCP server and `BacklogService` run in the **same Go binary/process** — there is no separate MCP server process. `backlogHandlers` already calls exported `*services.BacklogService` methods in-process today (`h.backlogSvc.MaybeTriggerTriage(...)`, `tools_backlog.go:1195,1263`), confirming the in-process-call pattern is established, not novel.

**Implication for design**: `wait_for_backlog_event` does not need a ConnectRPC client, a stream adapter, or any new dependency on `connectrpc.com/connect`'s streaming client machinery. It can call `h.eventBus.Subscribe(ctx)` directly (identical to what `watchBacklogItems` does internally) and filter the resulting `<-chan *events.Event` for `Item.ID == item_id` (+ optionally `Kind == BacklogChangeVerdictRecorded` if the tool should only wake on verdicts specifically — requirements.md's "e.g. VerdictRecorded, StatusChanged" phrasing leaves this open as a design decision for Phase 3, not a stack question). This sidesteps the unexported `watchBacklogItems`/`backlogItemEventSender` entirely — no export needed, no cross-package API change to `server/services`.

## 3. mcpgo (`github.com/mark3labs/mcp-go`) tool-schema surface

Module: `github.com/mark3labs/mcp-go v0.48.0` (go.mod:140).

Confirmed schema helpers in use across `server/mcp/*.go` (from `wait_for_output`'s registration, section 1 above):
- `mcpgo.NewTool(name string, opts ...)`
- `mcpgo.WithDescription(string)`
- `mcpgo.WithString(name string, opts ...)` / `mcpgo.WithNumber(name string, opts ...)` — parameter declarations
- `mcpgo.Description(string)`, `mcpgo.Required()`, `mcpgo.Enum(values ...string)` (string params)
- `mcpgo.DefaultNumber(float64)`, `mcpgo.Min(float64)`, `mcpgo.Max(float64)` (number params) — this is the exact combination needed for `timeout_seconds`, already proven on `wait_for_output` and `run_command`.

Handler side: `req.GetArguments()` returns `map[string]any`; numeric args arrive as `float64` (JSON number decoding), read via `args["timeout_seconds"].(float64)` type assertion (seen in both `waitForOutput` and `runCommand`). `req mcpgo.CallToolRequest` carries the incoming context implicitly via the handler's own `ctx context.Context` first parameter — `mcp-go`'s server dispatch passes request-scoped `context.Context` through to the registered handler function signature `func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`; cancellation/timeout of that `ctx` (e.g. client disconnect) propagates automatically to any `ctx`-aware code inside the handler — this is exactly the same `ctx` `writeToSession`/`sendControl` use with `context.WithTimeout(ctx, ...)` (section 1). No extra cancellation plumbing needed beyond using the handler's own `ctx` (not discarding it, unlike `waitForOutput`'s current `_ context.Context`).

Registration pattern for new backlog tools: follow `registerBacklogTools`'s existing `s.AddTool(mcpgo.NewTool(...), h.<method>)` calls in `server/mcp/tools_backlog.go` (same file the new tool belongs in per requirements.md's scope).

## 4. Go version / stdlib fit

- Go version: **1.26.3** (go.mod:3).
- This is a stdlib-solvable "adapt a stream into a bounded blocking call" problem — no new dependency required:
  ```go
  ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
  defer cancel()
  eventCh, subID := h.eventBus.Subscribe(ctx)
  defer h.eventBus.Unsubscribe(subID)
  for {
      select {
      case <-ctx.Done():
          return timeoutResult(...), nil // covers both explicit timeout and caller cancellation/disconnect
      case evt, ok := <-eventCh:
          if !ok {
              return timeoutResult(...), nil // bus closed
          }
          if matches(evt, itemID) {
              return matchedResult(evt), nil
          }
      }
  }
  ```
  `context.WithTimeout` + `select` over the subscription channel and `ctx.Done()` is the same primitive combination `writeToSession`/`sendControl` already use (section 1) and is sufficient on its own — `eb.Subscribe(ctx)` already ties the subscription's internal goroutine lifecycle to the passed context per its own signature, and `defer h.eventBus.Unsubscribe(subID)` guarantees cleanup on every exit path (timeout, match, or caller disconnect canceling `ctx`), directly satisfying the "no leak" scope requirement.
- No third-party timer/context/streaming library is warranted; `context` + `time` + a buffered/unbuffered channel `select` fully covers this.

## 5. Dependency versions (go.mod)

| Dependency | Version | go.mod line |
|---|---|---|
| Go toolchain | 1.26.3 | 3 |
| `connectrpc.com/connect` | v1.19.0 | 6 |
| `github.com/mark3labs/mcp-go` | v0.48.0 | 140 |

`connectrpc.com/connect` is relevant only as context (it's what `WatchBacklogItems` the RPC uses) — per section 2, the new MCP tool does **not** need to touch it at all, since it subscribes to the same underlying `*events.EventBus` directly rather than adapting a `connect.ServerStream`.

## Summary for Phase 3 (plan)

- No new Go dependency needed.
- New tool goes in `server/mcp/tools_backlog.go`, registered via `registerBacklogTools`, added to `backlogHandlers` (which already has both `eventBus` and `backlogSvc` fields wired in-process).
- Implementation is `h.eventBus.Subscribe(ctx)` + `context.WithTimeout` + `select` loop filtering `*events.Event` by `evt.BacklogItemPayload.Item.ID == item_id`, `defer h.eventBus.Unsubscribe(subID)` for cleanup — no ConnectRPC client, no stream adapter, no ServerStream involvement.
- Timeout/param schema: mirror `wait_for_output`'s `mcpgo.WithNumber("timeout_seconds", mcpgo.DefaultNumber(30), mcpgo.Min(1), mcpgo.Max(60))` pattern.
- Timeout should be returned as a structured `MCPResult{Success: true, Error: &MCPError{Code: "WAIT_TIMEOUT", ...}}` result (matching `wait_for_output`'s convention), not a Go `error`.
- Open design question deferred to Phase 3 (not a stack question): whether to filter to *any* event kind for the item, or only specific kinds (e.g. `BacklogChangeVerdictRecorded`, `BacklogChangeStatusTransition`) per requirements.md's "e.g." phrasing.
