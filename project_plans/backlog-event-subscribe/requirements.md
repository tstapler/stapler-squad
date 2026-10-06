# Requirements: backlog-event-subscribe

**Date**: 2026-08-11
**Type**: feature addition
**Complexity**: 2 — moderate, single new MCP tool backed by fully-existing infrastructure
**Backlog item**: `9c31a206-5821-4292-9b1f-24354463e57b` / GitHub issue [tstapler/stapler-squad#428](https://github.com/tstapler/stapler-squad/issues/428)

## Problem Statement

Claude Code sessions waiting on a backlog outcome (most commonly an automated review
verdict) have no way to be woken by the event itself. They fall back to a polling loop
implemented via `ScheduleWakeup`/`/loop`: reschedule a wakeup a few minutes out, call
`get_backlog_item` or run `/backlog/status`, see nothing new, reschedule again. One
session was observed doing this across several wakeup cycles ("Still no verdict...",
"Still waiting... checked twice now with no result...") and even attempted a blocking
`sleep 120` to bridge the gap, which the harness correctly blocked (long sleeps are
disallowed specifically to prevent polling loops) — but a scheduled-wakeup polling loop
is the *only* alternative the session had.

This wastes wakeup cycles/tokens on empty checks and adds latency between an event
happening and the session reacting to it, bounded only by the polling interval the
session guessed at.

## Baseline — what already exists (VERIFIED against this checkout)

The server-side event infrastructure this item asks for is **already fully built and
shipped** — it was implemented by the separate, already-completed
`project_plans/backlog-event-driven-updates/` project (commits `fd8bc1f69` through
`556e61347`, confirmed via `git log --oneline --all | grep -i watch-backlog`):

- `BacklogService.WatchBacklogItems` — a ConnectRPC server-streaming RPC
  (`server/services/backlog_service_events.go:59`) that emits `BacklogItemEvent`s
  (`VerdictRecorded`, `StatusChanged`, and others) for every backlog item mutation,
  including ones made by internal reconcilers, not just RPC-handler-driven changes.
- A frontend hook (`useWatchBacklogItems`) consumes this stream for the web UI
  (`/backlog`, `/backlog/board`, `BacklogItemDetail`, `BacklogItemPanel`).
- Multi-instance/workspace event scoping is already confirmed correct (one
  `*events.EventBus` per workspace process — see that project's
  `research/architecture.md` §3 and its Epic 7.1 isolation test).

**What is missing** is any consumer of this stream on the session/MCP side.
`server/mcp/tools_backlog.go` (`registerBacklogTools`, tools like `get_backlog_item`,
`report_progress`, `submit_review_verdict`) exposes only point-in-time reads — no tool
blocks on or subscribes to `WatchBacklogItems`. The closest existing pattern is
`wait_for_output` (`server/mcp/tools_terminal.go:117`), a blocking tool with a
`timeout_seconds` parameter that "combines write + wait + read in one call" for
terminal output — the same shape this item needs, but for backlog events instead of
PTY output.

This project is scoped **narrowly** to closing that one gap: give a session-facing MCP
tool a way to block on/subscribe to `WatchBacklogItems` for a specific item, instead of
polling. It does **not** rebuild or duplicate the already-shipped backend stream, web
UI wiring, or event model — those are complete and out of scope for changes here except
as a consumed dependency.

## Users / Consumers

- Claude Code sessions (via the `stapler-squad` MCP tool surface) running
  `/backlog/review`-style flows that need to react the moment a verdict or status
  change lands on a specific backlog item they're waiting on.
- Indirectly, the `/loop`/`ScheduleWakeup` harness path — this removes the *need* for
  those flows to use wakeup-based polling as a verdict-detection mechanism, though
  `ScheduleWakeup` itself is unrelated infrastructure and stays as-is for actual
  interval-based work.

## Success Metrics

- A session waiting on a backlog item's verdict/status can get that update via a single
  blocking MCP tool call instead of a `ScheduleWakeup` + `get_backlog_item` polling
  loop.
- End-to-end latency from server-side event publish to the tool call returning is
  bounded by the existing `WatchBacklogItems` stream latency (sub-second to a few
  seconds per the prior project's SLO), not by a polling interval.
- Existing `get_backlog_item` point-in-time reads and `/backlog/status` continue to work
  unchanged — this is an additive tool, not a replacement of existing reads.

## Appetite

Small (days, not weeks) — the hard part (event bus, stream RPC, event model, isolation
correctness) is already done; this is "add one MCP tool that's a thin client of an
existing RPC," analogous in shape/effort to `wait_for_output`.

## Constraints

- Must reuse `WatchBacklogItems` as-is; no new event bus, new event types, or backend
  event-model changes unless research finds a genuine gap (e.g. a missing filter
  parameter) — if so, keep any such addition minimal and additive.
- Must follow this repo's MCP tool conventions (`server/mcp/tools_backlog.go`
  registration pattern, `mcpgo.NewTool` schema style, `timeout_seconds`-bounded blocking
  per `wait_for_output`'s precedent) rather than inventing a new tool-calling shape.
- Must not regress `.claude/rules/session-creation-registry.md`-style registries — check
  whether a new MCP tool requires a feature-registry entry per
  `.claude/rules/feature-registry.md` (new backend "feature" marker/registry file).

## Non-functional Requirements

- **Latency**: tool call should return within roughly the same latency envelope
  `WatchBacklogItems` already delivers to the web UI (target: matching that project's
  documented ≤2s p95 publish-to-visible SLO), not a new, slower path.
- **Bounded blocking**: the tool must accept a `timeout_seconds`-style parameter and
  return a clear "timed out, no new event" result rather than blocking indefinitely —
  mirroring `wait_for_output`'s existing contract so callers get a predictable, callable
  loop-friendly shape.
- **Concurrency safety**: multiple sessions/tool calls subscribing to the same or
  different backlog items concurrently must not interfere with each other or leak
  stream subscriptions/goroutines on timeout, disconnect, or caller cancellation.

## Scope

### In Scope
- One new MCP tool (working name: `wait_for_backlog_event`, exact name TBD in planning)
  that opens (or reuses) a `WatchBacklogItems` subscription filtered to a specific
  `item_id`, blocks until a matching event arrives or `timeout_seconds` elapses, and
  returns the event (or a timeout result) to the calling session.
- Registration of the new tool in `server/mcp/tools_backlog.go` following the existing
  `registerBacklogTools` pattern.
- Correct cleanup of the underlying stream subscription on timeout, tool-call
  cancellation, or session disconnect (no goroutine/subscription leak).
- Feature-registry entry per `.claude/rules/feature-registry.md` if the registry
  convention requires one for new MCP tools (research phase confirms this).
- Minimal doc/example update showing a session how to replace a `ScheduleWakeup`
  polling loop with this tool (e.g. in the tool's own MCP description, mirroring how
  `wait_for_output`'s description explains its own usage).

### Out of Scope
- Any change to `WatchBacklogItems`, `BacklogItemEvent`, the event bus, or the web UI
  consumers — all already shipped and working; this project only adds a new consumer.
- A webhook-style "push a wakeup to the waiting session" mechanism (the second
  alternative floated in the original item description) — the blocking-tool approach
  fully satisfies the stated need with far less new surface area (no new
  wakeup-injection plumbing into `ScheduleWakeup`/the harness) and mirrors the
  `wait_for_output` precedent already in the codebase; revisit only if research finds
  the blocking-tool shape is unworkable for some session-lifecycle reason.
- Changing `ScheduleWakeup`/`/loop` semantics or deprecating scheduled wakeups as a
  general mechanism — they remain the right tool for genuinely interval-based work;
  this project only removes the *need* to misuse them for event-waiting.
- Any backlog pipeline/reconciliation logic changes.

## Rabbit Holes

- **Subscription lifecycle correctness**: an MCP tool call is not a long-lived stream
  connection the way `WatchSessions`'/`WatchBacklogItems`' ConnectRPC streaming clients
  are — the new tool must adapt "subscribe to a stream, block synchronously, and return
  once" without leaking the underlying stream goroutine if the tool call times out or
  the session disconnects mid-wait. Get this exactly right in planning; it's the one
  genuinely new piece of code in this project.
- **Multiple concurrent waiters on the same item**: if two tool calls (or a
  retry-after-timeout) subscribe to the same `item_id` concurrently, confirm this is
  safe and doesn't double-fire or miss events for either caller.
- **Missed-event race**: a naive "subscribe, then block" has a race window between
  "read current state" and "start listening" where an event could land in between and
  be missed. `WatchBacklogItems`' existing `after_seq` replay mechanism (used by the web
  UI) is the likely fix — confirm in research whether/how the MCP tool should use it.

## Alternatives Considered

- **Webhook-style wakeup push into `ScheduleWakeup`**: rejected for this iteration (see
  Out of Scope) — bigger surface area, new coupling between the backlog event system
  and the wakeup/harness scheduling system, for the same net benefit the blocking-tool
  approach already delivers.
- **Extend `get_backlog_item` with a `wait` flag** instead of a new tool: rejected in
  favor of a distinct tool — keeps the existing point-in-time-read tool's contract
  simple/fast and matches the `wait_for_output` precedent of a separate blocking tool
  rather than an overloaded flag on a fast-read tool.

## Feasibility Risks

- Adapting a ConnectRPC server-streaming client into a bounded-blocking MCP tool call is
  a shape this codebase hasn't done before for `WatchBacklogItems` specifically (though
  `wait_for_output`'s polling-with-timeout pattern is a close analog for *terminal*
  output, not a gRPC/ConnectRPC stream) — Phase 2 research must confirm the concrete
  Go pattern (e.g. context-with-timeout wrapping a stream `Receive()` loop) compiles
  cleanly against the existing `WatchBacklogItems` server implementation before planning
  commits to an approach.

## Observability Requirements

- Log subscribe/timeout/deliver/cleanup for the new tool at the same level of detail
  `WatchBacklogItems`' server-side handler already logs connect/disconnect, so a stuck
  or leaking subscription is diagnosable the same way.
- No new oncall alert — internal dev-tool feature, matching the parent project's
  precedent.

## Risk Control

Direct ship, no feature flag — additive MCP tool, existing polling-based flows are
unaffected and remain a safe fallback if the new tool has issues. Revert is a normal
`git revert` if needed.

## Open Questions

- Exact tool name and parameter shape (`item_id` + `timeout_seconds` + optionally an
  event-type filter like `verdict_recorded`) — resolve in planning.
- Whether `after_seq`-based replay (already used by the web UI hook) should be exposed
  as an optional tool parameter so a session can resume-from-last-seen-event across
  multiple tool calls, or whether "always wait for the next new event from call time"
  is sufficient for the `/backlog/review`-style use case — resolve in research/planning.
- Whether this needs its own feature-registry file per
  `.claude/rules/feature-registry.md`, given MCP tools aren't obviously "backend RPC" or
  "frontend UI" in that rule's current two categories — resolve in research.
