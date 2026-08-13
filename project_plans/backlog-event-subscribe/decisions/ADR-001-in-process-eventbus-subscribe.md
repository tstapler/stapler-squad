# ADR-001: In-process `EventBus.Subscribe` instead of a ConnectRPC client, for `wait_for_backlog_event`

**Status**: Accepted
**Date**: 2026-08-11
**Project**: `backlog-event-subscribe`

## Context

`WatchBacklogItems` (`server/services/backlog_service_events.go`) is an already-shipped
ConnectRPC server-streaming RPC that emits `BacklogItemEvent`s for every backlog item mutation.
The web UI's `useWatchBacklogItems` hook is its only consumer today. This project adds a second
consumer: a new MCP tool, `wait_for_backlog_event`, that lets a Claude Code session block on
that same event stream for one item instead of polling via `ScheduleWakeup`+`get_backlog_item`.

A reasonable first instinct — and the one a contributor skimming `WatchBacklogItems`'s existence
might reach for — is to have the new MCP tool act as a ConnectRPC *client* of
`WatchBacklogItems`, the same way the web UI does: open a streaming call, read events off it,
apply a timeout. This ADR records why that instinct is wrong for this specific consumer, and
what was built instead.

## Decision

`wait_for_backlog_event`'s handler (`server/mcp/tools_backlog.go`) calls
`h.eventBus.Subscribe(ctx)` directly on the same `*pkg/events.EventBus` instance
`BacklogService` itself subscribes to internally — bypassing `WatchBacklogItems`, ConnectRPC,
and any proto (de)serialization entirely.

This works because the MCP server and `BacklogService` run in the **same Go process** (verified:
`server/mcp/server.go:64`'s `registerBacklogTools` call already wires the identical
`*events.EventBus` pointer that `BacklogService.SetEventBus` receives — confirmed via
`server/dependencies.go`'s single event-bus construction site). There is no network boundary
between the MCP tool handler and the event bus to cross.

## Alternatives Considered

1. **Adapt the ConnectRPC `WatchBacklogItems` streaming client in-process** (mirror what the web
   UI does). Rejected: this would mean the server calling itself over HTTP/2, with full proto
   marshal/unmarshal, on top of a **second, redundant subscription** to the exact same event bus
   `watchBacklogItems`'s own implementation already subscribes to internally. Concretely: three
   layers of indirection (build a `WatchBacklogItemsRequest`, drive a `ServerStream[T]` or a fake
   sender since `connect.ServerStream[Res]` has no exported constructor, convert the resulting
   proto `BacklogItemEvent` back into Go values) to reach data that's one `Subscribe` call away
   as a plain Go struct. No benefit; meaningful added latency, complexity, and failure surface.

2. **Export `watchBacklogItems`'s unexported core logic** so the MCP tool could call it directly
   as a Go function, reusing its snapshot/replay/filter logic wholesale. Rejected: that function
   sends through the narrow `backlogItemEventSender` interface (`Send(*sessionv1.BacklogItemEvent) error`)
   specifically so RPC-layer tests can fake a `connect.ServerStream[T]` — its entire contract is
   proto-message-in/proto-message-out and "run until `ctx.Done()`," neither of which matches a
   bounded, single-event, Go-struct-returning tool call. Exporting it would mean bending an
   RPC-shaped function to a materially different (bounded, Go-native) caller shape, or building
   an adapter layer just as indirect as option 1.

## Consequences

- **Positive**: Zero new dependency on `connectrpc.com/connect`'s streaming client machinery for
  this tool. Zero proto involvement. The event's full Go-native fields (`BacklogItemEventPayload`
  — `Kind`, `Item`, `OldStatus`/`NewStatus`, `Verdict`, `UpdatedFields`, `ArchivedAt`,
  `RemovedReason`) are available directly, with no unmarshal step, letting the tool's result
  carry richer structured data (verdict outcome/summary, updated fields) than a bare
  proto-round-trip would motivate building out.
- **Positive**: Reuses the exact `Subscribe`/`Unsubscribe`/context-cancellation cleanup lifecycle
  `watchBacklogItems` already relies on and that is already covered by `pkg/events/bus_test.go`'s
  `TestEventBusContextCancellation` — no new subscription-lifecycle primitive to design or trust.
- **Negative / accepted risk**: This bypass only works because the MCP tool and `BacklogService`
  are guaranteed to be co-located in the same process **in the common case**. The MCP stdio
  fallback path (`main.go`'s `buildMCPDeps()` branch, used when the daemon's `/mcp` HTTP endpoint
  is unreachable) constructs a fresh, independent dependency stack with `eventBus` hardcoded to
  `nil` — there is no daemon connection to fall back to for events on that path (unlike
  point-in-time reads, which still work via the shared on-disk SQLite storage). This is a
  structural consequence of the chosen approach, not a bug: an in-process subscription has no
  network fallback by definition. It is mitigated, not avoided, by `wait_for_backlog_event`'s
  mandatory `eventBus == nil` guard (returns `EVENT_STREAM_UNAVAILABLE`, a distinct error code
  from `WAIT_TIMEOUT`) rather than silently blocking to a false timeout or panicking — see
  `plan.md` Task 1.2.1a and pitfalls research finding 5.
- **Neutral**: If a future consumer of backlog events genuinely needs to run in a *different*
  process than `BacklogService` (e.g. a separate worker binary), that consumer would need the
  ConnectRPC `WatchBacklogItems` client path this ADR rejects for the MCP tool specifically — the
  rejection here is scoped to "same-process MCP tool," not a blanket statement that the RPC
  client approach is wrong everywhere.
