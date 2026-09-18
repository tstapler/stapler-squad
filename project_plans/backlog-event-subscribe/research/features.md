# Research: Feature Landscape — backlog-event-subscribe

Agent 2 (Features). Scope: prior blocking-tool patterns, `tools_backlog.go`'s existing
conventions this new tool must fit, edge cases, unstated needs of `/backlog/review`-style
callers, and cross-check against `backlog-event-driven-updates`'s prior research so we don't
re-derive what that project already found. All line numbers current as of this checkout
(2026-08-11).

## 1. Existing "block until event or timeout" patterns in `server/mcp/`

Grepped `timeout_seconds`/`WithTimeout`/`WithDeadline` across every `server/mcp/tools_*.go`.
Three tools in `server/mcp/tools_terminal.go` (`registerTerminalTools`, lines 59–178) share
the shape; no other `tools_*.go` file has a blocking-with-timeout tool.

| Tool | File:Line | Poll mechanism | Return shape (success) | Return shape (timeout) | Return shape (error) |
|---|---|---|---|---|---|
| `wait_for_output` | `tools_terminal.go:390` (`waitForOutput`) | `time.NewTicker(1s)` loop, deadline = `time.Now().Add(timeoutSecs)`, re-reads scrollback each tick | `WaitForOutputResult{Matched:true, MatchedLine, Output, Truncated}`, `MCPResult.Success:true` | **Same result type**, `Matched:false`, `Output` = last-seen, `MCPResult.Success:true` but `MCPResult.Error = &MCPError{Code:"WAIT_TIMEOUT", ...}` — i.e. timeout is encoded as a *populated but non-nil `Error` field on a `Success:true` envelope*, not a Go `error` return and not a different result type | Go-level `errResult(...)` (`ErrInvalidArgument`, `ErrSessionNotFound`) for bad input/missing session, returned before the wait loop starts |
| `run_command` | `tools_terminal.go:509` (`runCommand`) | `time.NewTicker(1s)` loop, breaks on 2 consecutive stable checksums (`bytesChecksum`) or deadline | `RunCommandResult{Output, Truncated, TimedOut:false, LastSequence}` | **Same result type**, `TimedOut:true` — note the inverse encoding from `wait_for_output`: timeout is a `bool` field on the *same success envelope*, `MCPResult.Error` is NOT set (caller must check `TimedOut`, not `Error`) | `errResult(...)` for bad input, `PTY_WRITE_TIMEOUT` for the initial 5s send-side timeout (separate from the poll-loop timeout) |
| `write_to_session` (partial precedent) | `tools_terminal.go:257` | Not a poll loop — a single `select` between `errCh` (goroutine result) and a `context.WithTimeout(ctx, 5*time.Second)` | `WriteSessionResult{BytesWritten}` | N/A (fire-and-forget after the 5s send-side guard; no "waited, nothing happened" case) | `PTY_WRITE_TIMEOUT` errResult if the PTY write itself doesn't complete in 5s |

**Key inconsistency to resolve in planning, not copy blindly**: `wait_for_output` encodes
"timed out" via `MCPResult.Error` (non-nil) on an otherwise-successful envelope;
`run_command` encodes it via a plain `bool` field with `Error` left nil. Requirements
explicitly cite `wait_for_output` as the precedent ("return a clear 'timed out, no new
event' result... mirroring `wait_for_output`'s existing contract"), so **follow
`wait_for_output`'s shape**: `Success:true` + `Error:{Code:"WAIT_TIMEOUT", Message:...}` +
a `Matched`-equivalent bool (e.g. `EventReceived bool`) — not `run_command`'s bare bool.

No other blocking-tool pattern exists elsewhere in the repo's MCP surface — `tools_vcs.go`,
`tools_workflow.go`, `tools_github.go`, `tools_rules.go`, `tools_discovery.go`,
`tools_goal.go` have zero `timeout_seconds`/`WithTimeout` hits (confirmed via grep,
`server/mcp/tools_backlog.go`'s only `WithTimeout` hit, line 724, is an unrelated 30s
reopen-context guard inside `reportPRCreated`, not a blocking-wait tool).

## 2. `tools_backlog.go` conventions the new tool must fit

Read `registerBacklogTools` (line 1716+) and the four cited tools' handlers.

- **Registration pattern**: one `s.AddTool(mcpgo.NewTool("name", mcpgo.WithDescription(...),
  mcpgo.WithString/WithNumber/WithArray(...)), h.handlerFunc)` block per tool, all inside
  `registerBacklogTools(s *mcpserver.MCPServer, h *backlogHandlers)`
  (`server/mcp/tools_backlog.go:1716`). New tool goes here, following the same block shape
  the terminal tools use for `timeout_seconds` (`mcpgo.WithNumber("timeout_seconds",
  mcpgo.Description(...), mcpgo.DefaultNumber(30), mcpgo.Min(1), mcpgo.Max(60))`, see
  `tools_terminal.go:127-131`).
- **`item_id` parameter convention**: every existing backlog tool takes `item_id` as a
  required string (`mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog
  item"), mcpgo.Required())`) — reuse verbatim, do not invent a different param name.
- **Validation order** (from `getBacklogItem`, `tools_backlog.go:194-213`, and every other
  handler): `featureDisabledResult(h.enabledCheck)` first → extract+type-assert `item_id`
  from `args := req.GetArguments()` → `errResult(ErrInvalidArgument, ...)` if
  missing/empty → `validateUUID(itemID)` (`tools_backlog.go:51`, regex
  `^[0-9a-f-]{36}$`) → `errResult(ErrInvalidArgument, err.Error(), "Provide a valid UUID
  (e.g. from list_backlog_items or get_backlog_item).")` if invalid.
- **Item-not-found convention**: `h.storage.GetBacklogItem(ctx, itemID)` returns
  `session.ErrNotFound`; handlers check `errors.Is(err, session.ErrNotFound)` and return
  `errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), "")`
  (`getBacklogItem`, `tools_backlog.go:207-213`). The new tool's up-front existence check
  (before subscribing) should use this exact pattern.
- **`callerSessionUUID(ctx)`** (`tools_backlog.go:39-46`, actually defined in
  `server/mcp/tools_backlog.go` per the earlier grep — confirmed at line 39): returns the
  session UUID injected via `WithSessionUUID` or a Go `error` (not an MCP errResult) if
  `STAPLER_SESSION_UUID` isn't set. `reportProgress`, `requestReview`, `submitReviewVerdict`,
  etc. all call this immediately after `featureDisabledResult`. `get_backlog_item` does
  *not* require it (it's a read, works even without a caller session UUID; role guidance is
  best-effort via `sessionUUIDFromContext`, tools_backlog.go:272, which silently no-ops if
  absent). **Recommendation for planning**: `wait_for_backlog_event` is a read/wait, not a
  mutation — model it after `get_backlog_item`'s optional-session-UUID pattern rather than
  the mutating tools' required-`callerSessionUUID` pattern, unless there's a reason to
  restrict which sessions can wait on which items (no such restriction exists today for
  `get_backlog_item`).
- **Result envelope**: every non-text-blob tool returns `okResult(SomeResult{MCPResult:
  MCPResult{Success:true}, ...})` on success and `errResult(code, message, hint string) *mcpgo.CallToolResult`
  on failure (both return `(*mcpgo.CallToolResult, nil)` — the handler's own Go `error`
  return is always `nil`; failures are encoded in the result payload, not a Go error, except
  for `callerSessionUUID`'s absent-UUID case which does return a real error before any
  result is built). `errResult`/`okResult`/`MCPResult`/`MCPError` are defined in
  `server/mcp/types.go`.
- **Direct `*events.EventBus` access already exists on `backlogHandlers`** — this is the
  single most important finding for planning. `backlogHandlers` (struct at
  `tools_backlog.go:118-149`) already carries `eventBus *events.EventBus` (field, line 121),
  wired at construction time in `server/mcp/server.go:64`:
  `registerBacklogTools(s, &backlogHandlers{storage: storage, store: store, eventBus:
  eventBus, ...})`. This is the **exact same `*pkg/events.EventBus` instance** the
  ConnectRPC `WatchBacklogItems` handler subscribes to
  (`server/services/backlog_service.go:230`, `s.eventBus *events.EventBus`, set via
  `SetEventBus` at line 311; `server/events` is confirmed to be a pure type-alias/const
  forwarding package over `pkg/events` — `server/events/forward.go:22,32` — not a second
  bus). **This resolves the requirements doc's stated Feasibility Risk** ("Adapting a
  ConnectRPC server-streaming client into a bounded-blocking MCP tool call... Phase 2
  research must confirm the concrete Go pattern"): the new tool does **not** need to adapt a
  ConnectRPC stream client at all. It can call `h.eventBus.Subscribe(ctx)` directly,
  in-process, exactly like `watchBacklogItems` does (`server/services/backlog_service_events.go:83`),
  and filter the resulting `<-chan *events.Event` for `Type ==
  events.EventBacklogItemChanged && BacklogItemPayload.Item.ID == itemID`. Zero new RPC
  client code, zero protobuf stream framing overhead — genuinely simpler than the shape the
  requirements worried about.

## 3. Edge cases

- **Item doesn't exist / bad `item_id`**: handled by the existing `validateUUID` +
  `h.storage.GetBacklogItem` existence check pattern (§2) — return `ErrInvalidArgument` or
  `ErrItemNotFound` immediately, before subscribing to the bus at all. No new pattern needed.
- **Item already in a terminal state before the wait starts**: **unresolved by existing
  code — this is a genuine design decision for planning**, not something `WatchBacklogItems`
  answers by precedent, because the ConnectRPC stream's fresh-connection branch
  (`backlog_service_events.go:125-143`) always sends a snapshot event for the item's
  *current* state as its very first message — a web UI tab that opens `WatchBacklogItems`
  on an already-`done` item immediately receives a snapshot `ItemUpdated` event, not a
  block. If `wait_for_backlog_event` mirrors that behavior exactly, it would return
  instantly for an already-terminal item (arguably the *useful* behavior for
  `/backlog/review`-style polling replacement — a session that calls the tool after the
  verdict already landed shouldn't have to wait a full `timeout_seconds` to find out).
  Recommend planning treat this as: **check current state first (after subscribing, per the
  race-window rule below), and if it already satisfies the caller's implicit interest
  (e.g. a verdict already exists), return immediately** — mirroring the snapshot branch —
  rather than "always wait for the strictly-next event," which would force a caller into an
  unnecessary full timeout on every retry-after-restart.
- **Multiple concurrent waiters on the same item, from different sessions**: `EventBus.Subscribe`
  (`pkg/events/bus.go:51`) creates one independent buffered channel per subscriber
  (`eb.subscribers[id] = ch`, keyed by a generated subscriber ID) and `Publish`
  (`bus.go:72-94`) fans out to every subscriber's channel independently — this is the same
  fan-out model the prior project's research already verified for `WatchSessions`/
  `WatchReviewQueue` (`project_plans/backlog-event-driven-updates/research/features.md` §6,
  "Multiple tabs on the same item" row: "no per-tab coordination needed since the EventBus
  fans out to N subscribers already"). Two concurrent `wait_for_backlog_event` calls for the
  same `item_id` (or a retry-after-timeout re-subscribing) are safe by construction — no new
  code required to handle this.
- **Backlog item deleted/archived while a wait is in progress**: the event model already has
  dedicated variants for both — `events.BacklogChangeItemArchived` and
  `events.BacklogChangeItemRemoved` (`pkg/events/types.go:44,46`, converted to
  `BacklogItemEvent_ItemArchived`/`BacklogItemEvent_ItemRemoved` in
  `convertEventToBacklogItemEvent`, `backlog_service_events.go:297-315`). Planning must
  decide whether the new tool treats these as a *terminal* result (return immediately with
  an "item archived/removed" outcome, since no further verdict/status event will ever come
  for a removed item) or just another matched-event return — returning immediately is almost
  certainly correct, since waiting out the full `timeout_seconds` after a `ItemRemoved` event
  for an item that provably can't emit anything else would be a pointless stall for the
  caller.
- **`WatchBacklogItems`/EventBus itself errors, or no listener registered yet at subscribe
  time (fast-firing-event race)**: `s.eventBus == nil` guard exists in the ConnectRPC handler
  (`backlog_service_events.go:75-77`, returns `CodeUnimplemented`) — the new tool needs the
  equivalent nil-check on `h.eventBus` (mirrors `backlogHandlers`'s existing
  `nil means notifications are disabled` convention for other optional deps, e.g.
  `reviewStopper`, `reviewTrigger`, `tools_backlog.go:122-124`) and should return a
  `FEATURE_DISABLED`-style errResult rather than panicking on a nil dereference. The
  missed-event race itself (subscribe-then-check-state ordering) is already solved
  structurally by `EventBus.Subscribe`: subscription registration
  (`eb.subscribers[id] = ch`, `bus.go:56`) happens synchronously inside `Subscribe`, before
  it returns the channel — so as long as the new tool calls `h.eventBus.Subscribe(ctx)`
  **before** its own `GetBacklogItem` existence/current-state check (mirroring
  `watchBacklogItems`'s explicit ordering comment, `backlog_service_events.go:79-82`:
  "Subscribe before building the snapshot... so no events are lost between the two phases"),
  no event published after `Subscribe()` returns can be missed. This is the `after_seq`
  question from the requirements' Rabbit Holes section — **the fix is subscribe-before-read
  ordering, not `after_seq` replay** (see §5 below for why `after_seq` itself is likely
  unnecessary for this tool's single-call shape).
- **Session's own tool-call context cancelled (session paused/killed) mid-wait**:
  `EventBus.Subscribe(ctx)` already spawns a cleanup goroutine keyed to the passed context
  (`bus.go:59-63`: `go func() { <-ctx.Done(); eb.Unsubscribe(id) }()`) — if the new tool
  derives its wait context from the incoming `ctx` (via `context.WithTimeout(ctx,
  timeoutSecs*time.Second)`, matching `wait_for_output`'s pattern of using the ambient
  request context rather than `context.Background()`), then a session pause/kill that
  cancels the parent MCP request context automatically unsubscribes and frees the channel —
  **no explicit leak-prevention code beyond correctly deriving the context and calling
  `defer cancel()`** (mirroring `watchBacklogItems`'s own `defer
  s.eventBus.Unsubscribe(subID)`, `backlog_service_events.go:84`, which is redundant-but-safe
  alongside the bus's own ctx-based auto-cleanup — belt-and-suspenders, worth copying).
- **Slow-consumer channel drop**: `Publish` is non-blocking per-subscriber — a full
  subscriber buffer (default 100, `bus.go:38-46`) causes silent event drop for that
  subscriber (`bus.go:86-93`, comment: "Client can recover via EventsSince on reconnect").
  For a single-item-filtered, single-call `wait_for_backlog_event` this is very unlikely to
  matter in practice (traffic per item is low, buffer is 100-deep, and the call only needs
  to see ONE matching event to return) — flag for planning but likely not worth mitigating
  beyond what already exists.

## 4. Unstated needs of `/backlog/review`-style flows

- **Event-type filtering is very likely wanted, not just "wake on anything."** The
  `session.SessionType`-analogous evidence: `get_backlog_item`'s own review-workflow
  guidance text (`tools_backlog.go:292`) tells a work-role session to "call `get_backlog_item`
  again — once a verdict lands it appears under 'Latest Review Verdict'" — the thing the
  session is actually waiting for is specifically a **verdict**, not just "the item changed
  somehow." A bare "wake on the next `BacklogItemChanged` event of any kind" would also
  return on `SessionAttached` or unrelated `ItemUpdated` (triage-progress-tick) events that
  carry no new information for a work-role session waiting on review outcome — forcing the
  caller into an immediate re-call loop, which just relocates the polling problem one layer
  down instead of eliminating it. **Recommend an optional `event_type` filter parameter**
  (e.g. `verdict_recorded`, `status_changed`, or unfiltered/`any`) in planning, mirroring
  `WatchBacklogItemsRequest`'s existing `status_filter`/`category_filter` filter-parameter
  precedent (`backlog_service_events.go:190-201`, `backlogItemMatchesFilters`) even though
  those filter by item attributes rather than event kind — the *pattern* of "accept a filter,
  reject non-matching events silently and keep waiting" is the one to reuse, just filtering
  on `BacklogChangeKind` instead of status/category.
- **The caller wants the item's current state once *something relevant* changes, not a raw
  event delta.** Every `BacklogItemEvent` variant already carries the full `Item` (protobuf)
  alongside the delta-specific fields (`OldStatus`/`NewStatus`, `Verdict`, etc.) — see
  `convertEventToBacklogItemEvent`'s switch, `backlog_service_events.go:251-316`, every case
  populates `Item: protoItem`. The MCP tool's result should likewise surface enough of the
  item's post-event state to make a follow-up `get_backlog_item` call optional in the common
  case (matching `run_command`'s "combines write + wait + read in one call" philosophy
  explicitly named in its own description, `tools_terminal.go:154`) — but should probably
  return a **summarized** shape (status, verdict outcome/summary, updated fields) rather than
  the full protobuf `Item`, consistent with `get_backlog_item`'s existing text-envelope
  convention over raw structured dumps of the whole item.

## 5. Cross-check against `backlog-event-driven-updates`'s prior research (cite, don't re-derive)

Read `project_plans/backlog-event-driven-updates/research/features.md` in full. Findings
this project's planning must respect rather than re-litigate:

- **§3, "Subscribe before building the snapshot" ordering rule** — already the load-bearing
  fix for this project's missed-event race edge case (§3 above); the prior project sourced
  it from `session_service.go:1996-1997`'s `WatchSessions` comment and it was carried into
  `watchBacklogItems`'s own implementation (`backlog_service_events.go:79-82`). Same rule
  applies here: subscribe to `h.eventBus` before doing the existence/current-state check.
- **§6 table, "Reconnect after network blip" row** — `after_seq`-style replay was recommended
  *for the streaming RPC* (a long-lived connection that can drop and reconnect). This
  tool is a **single bounded request/response call**, not a long-lived reconnecting stream —
  there is no "reconnect" event for a tool call that already returned. The open question in
  this project's requirements ("Whether `after_seq`-based replay... should be exposed as an
  optional tool parameter so a session can resume-from-last-seen-event across multiple tool
  calls") is a **different, tool-call-specific use case** than what `after_seq` was built
  for (stream reconnect) — it's closer to "give me the next event after seq N" as a
  polling-with-memory primitive. This is a legitimate, separable feature (`EventsSince`,
  `pkg/events/bus.go:115`, already supports arbitrary `afterSeq` lookups and doesn't require
  an active subscription), but planning should treat it as an **additive, optional**
  parameter on top of the baseline "wait for next event" tool, not a required part of the
  MVP — matches the requirements' own "Constraints" section ("keep any such addition minimal
  and additive").
- **§6 table, "Out-of-order / duplicate events" row** — "payload should carry the full
  updated item... so an out-of-order or duplicate event is a no-op overwrite." Directly
  informs §4 above (return summarized-but-complete state, not a bare delta) — a caller that
  gets the tool's result after already having independently called `get_backlog_item` should
  be able to trust the tool's returned state is at-least-as-fresh, never a stale partial.
- **§5, `Notifier`/`EventBusNotifier` coalescing-key bug precedent** — not directly
  applicable to this tool (it doesn't publish, only subscribes), but worth flagging: if
  planning adds any logging/observability keyed by `item_id` for this tool (per the
  requirements' Observability Requirements section), follow the same lesson — key
  diagnostic/log correlation by `item_id`, not just `session_id`, to avoid the same
  cross-item collision class documented in `backlog_notifier.go`'s doc comment.

## 6. Feature-registry applicability (resolves requirements' open question)

Checked `docs/registry/features/backend/` for any MCP-tool-keyed entries: none exist. Every
entry under `docs/registry/features/backend/backlog/*.json` is keyed to a ConnectRPC
`service`+`method` pair (e.g. `{"id": "backlog:approve-plan", "service": "BacklogService",
"method": "ApprovePlan", "protoFile": "proto/session/v1/backlog.proto", "markerFound":
true, ...}`) tied to a `// +api: scope:action` marker on the RPC handler
(`.claude/rules/feature-registry.md`). Grepped every `server/mcp/tools_*.go` file for
`// +api:` markers: **zero hits** — no existing MCP tool (`wait_for_output`,
`get_backlog_item`, `report_progress`, etc.) has ever been given a registry entry or marker.
**Conclusion for planning**: the registry's two-category model ("backend" = RPC method,
"frontend" = UI component) genuinely has no slot for "MCP tool" today, and there is no
established precedent of retrofitting one for an existing tool — `wait_for_backlog_event`
should follow the *established* precedent of not adding a registry entry, consistent with
every other MCP tool in the codebase, rather than being the first one to invent a new
registry category unprompted. If the SDD process wants MCP tools tracked going forward,
that's a separate, repo-wide registry-schema change out of scope for this narrowly-scoped
project (per requirements' Out of Scope: "no new event bus, new event types, or backend
event-model changes unless research finds a genuine gap" — this is the analogous "don't
expand scope to fix a pre-existing gap" call for the registry).
