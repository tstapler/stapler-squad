# Build vs. Buy: `wait_for_backlog_event` MCP Tool

Agent 6, SDD Phase 2 research, project `backlog-event-subscribe`.

## 1. Existing OSS library/framework for "MCP long-poll/subscribe tool"

**Verdict: Not recommended (none applies) — this is in-house glue code.**

- The MCP Go SDK used by this repo is `github.com/mark3labs/mcp-go` (`v0.48.0`,
  imported as `mcpgo "github.com/mark3labs/mcp-go/mcp"` — see
  `server/mcp/tools_terminal.go:117` and every other `tools_*.go` file). It provides
  tool registration (`mcpgo.NewTool`, `mcpgo.With*`) and a request/response tool-call
  model. There is no generic "blocking/long-poll tool" helper in the SDK — every
  blocking tool in this codebase (`wait_for_output`) hand-rolls its own wait loop.
- The MCP *spec* does have a native subscription primitive, but it's scoped to
  **Resources**, not Tools: `resources/subscribe` +
  `notifications/resources/updated`, which `mark3labs/mcp-go` implements
  server-side (`server.capabilities.resources.subscribe`,
  `/home/tstapler/go/pkg/mod/github.com/mark3labs/mcp-go@v0.48.0/server/server.go:906-909`).
  This doesn't fit here: it requires modeling each backlog item as an MCP
  *Resource* (a new resource surface, `resources/read`, URI scheme, etc.) and,
  more fundamentally, it delivers an async *notification* to the client
  connection rather than resolving a synchronous tool call — the client (a
  Claude Code session) would need its own notification-listening loop, which is
  exactly the kind of new harness-side plumbing the requirements doc's Out of
  Scope section explicitly rejects ("webhook-style wakeup push"). A blocking
  tool call that returns is a much better fit for how a session currently
  drives tool use.
- No third-party Go pub/sub, event-bus, or "future/promise" library is
  warranted or present in `go.mod` for this — the event bus already exists
  in-house (`pkg/events`, aliased via `server/events/forward.go`).

**Conclusion**: build. There is no library gap to fill; the tool is a thin
Go-side adapter between two already-in-house pieces (`pkg/events.EventBus` and
`mark3labs/mcp-go`'s tool-call model).

## 2. SaaS/managed API

**Verdict: Not applicable — confirmed, no further exploration warranted.**

The event source is `pkg/events.EventBus`, an in-process, per-workspace Go
struct (`pkg/events/bus.go:25`) fed by internal reconcilers and RPC handlers
within the same server binary. There is no external system to subscribe to,
no cross-network delivery requirement, and no case for outsourcing any part of
this to a managed service (e.g. a hosted pub/sub broker) — that would add a
network hop and an external dependency for a need that's fully satisfied
in-process today, matching the requirements doc's Baseline section describing
this infrastructure as already fully built and shipped.

## 3. LLM-generated implementation vs. battle-tested library — concurrency pattern

**Verdict: Recommended — plain stdlib `context` + `select` over a channel; no
dependency needed.**

- The requirement ("block until a matching event arrives on a channel, or
  `timeout_seconds` elapses, whichever first, with clean unsubscribe on either
  path") is the textbook stdlib pattern: `context.WithTimeout(parentCtx, d)`
  wrapping a `for { select { case <-ctx.Done(): ...; case evt, ok := <-eventCh:
  ... } } }` loop, with `defer eventBus.Unsubscribe(subID)` for cleanup. This is
  a few dozen lines, not a "future/promise" abstraction — pulling in a generic
  promise library (e.g. something like `conc` or a hand-rolled `Future[T]`
  generic) would be unjustified generic machinery per
  `.claude/rules/interface-pollution-checklist.md`'s smell #5, for a
  single-call-site need stdlib already expresses cleanly.
- **No existing internal helper already generalizes this exact bounded-wait
  pattern.** Grepped every non-generated, non-test `.go` file using both
  `context.WithTimeout` and a `select`-over-channel body. The only genuine
  hand-rolled wait loop in the codebase is `wait_for_output`'s implementation
  (`server/mcp/tools_terminal.go:390-`), but it polls a *scrollback buffer* on
  a `time.Ticker` (1s interval) rather than blocking on a channel receive — a
  materially different (and strictly worse-latency, poll-based) shape than
  what's needed here.
- Every other `eventBus.Subscribe(ctx)` call site in the codebase
  (`server/services/backlog_service_events.go:83`,
  `server/services/session_service.go:2365`,
  `server/services/unfinished_work_service.go:154`, and the analogous
  `s.cache.Subscribe`/`s.store.Subscribe` bridges in
  `server/services/github_user_service.go:98-101` and
  `server/services/insights_service.go:529-531`) is an **unbounded** loop that
  runs until `ctx.Done()` fires from client disconnect — i.e. "stream forever,"
  not "wait for at most N seconds for the next matching event." None of them
  is a drop-in helper for a bounded single-event wait; the `context.WithTimeout`
  wrapper is the one genuinely new piece of code this project needs, matching
  the requirements doc's own Feasibility Risks note. It should still closely
  mirror the `select { case <-ctx.Done(): ...; case evt, ok := <-eventCh: ...}`
  shape used in every one of those call sites, just with the outer context
  time-bounded instead of the request context.

## 4. Fork or adapt: closest existing implementation to template from

**Recommended: adapt `watchBacklogItems`'s subscribe/filter/select-loop core
(`server/services/backlog_service_events.go:82-181`), not `wait_for_output`'s
polling loop and not the frontend ConnectRPC client.**

| Candidate | Pros | Cons | Verdict |
|---|---|---|---|
| **Adapt `wait_for_output`'s polling-loop structure** (`server/mcp/tools_terminal.go:390-`) | Same MCP-tool file/package (`server/mcp/tools_backlog.go` sibling), same `timeout_seconds` arg convention and result shape (`matched=false` on timeout) the requirements doc explicitly asks to mirror | Its actual *mechanism* is a 1s `time.Ticker` re-poll of a buffer, not a channel wait — adopting that mechanism here would reintroduce polling latency (up to 1s per tick) for no reason, when a genuine push channel (`eventBus.Subscribe`) is available. Only the **tool-shape/argument/timeout conventions** are worth reusing, not the wait mechanism itself. | Viable (for shape/conventions only) |
| **Adapt the ConnectRPC streaming client pattern `useWatchBacklogItems` (web UI) talks to** | Reuses the exact same RPC (`WatchBacklogItems`) and wire event type (`BacklogItemEvent`) end-to-end | The MCP tool runs *in the same process* as `BacklogService` — `backlogHandlers` (`server/mcp/tools_backlog.go:118-121`) is already constructed with a direct `eventBus *events.EventBus` field (wired in `server/mcp/server.go:64`). Going through a ConnectRPC client would mean the server calling itself over HTTP/2 in-process: an unnecessary network round-trip, serialization, and a second, redundant subscription layer on top of the one `WatchBacklogItems` itself already uses internally. The web UI needs ConnectRPC because it's a separate browser process; the MCP tool has no such boundary to cross. | Not recommended |
| **Fresh minimal in-process `EventBus` subscriber** modeled on `watchBacklogItems`'s core (subscribe → filter by `item_id`/event kind → `select` loop), but bounded by `context.WithTimeout` instead of running until disconnect, and returning one event instead of streaming | Reuses the already-proven `eventBus.Subscribe(ctx)`/`Unsubscribe(subID)` lifecycle and the exact `events.EventBacklogItemChanged` + `BacklogItemEventPayload` filtering logic already validated by `backlogItemMatchesFilters` (`server/services/backlog_service_events.go:187-197`, extend/reuse rather than reimplement for `item_id` matching); avoids both the polling-latency problem above and the redundant-RPC problem above; `backlogHandlers` already has direct `eventBus` access so no new plumbing is needed to reach it | This is the one genuinely new piece of code in the project (a bounded, single-shot wait replacing an unbounded stream loop) — needs its own test coverage for timeout/cancellation/cleanup (the Rabbit Holes the requirements doc calls out) since no existing test in the codebase exercises this exact bounded-wait shape yet | **Recommended** |

**Concrete recommendation**: build `wait_for_backlog_event` as a new handler
in `server/mcp/tools_backlog.go` that:
1. Calls `h.eventBus.Subscribe(ctx)` directly (same call `watchBacklogItems`
   makes), where `ctx` is wrapped in `context.WithTimeout(parentCtx,
   timeoutSeconds)`.
2. Reuses (or thinly extends) `backlogItemMatchesFilters`-style filtering,
   narrowed to a single `item_id` instead of the RPC's `status_filter`/
   `category_filter`.
3. Uses the existing `after_seq`/`EventsSince` replay mechanism
   (`pkg/events/bus.go:115`, already used identically in
   `backlog_service_events.go:98-118`) to close the missed-event race the
   requirements doc's Rabbit Holes section flags — subscribe *before* checking
   current state, exactly like `watchBacklogItems` already does, so no event
   can land in the gap.
4. Mirrors `wait_for_output`'s tool-call *shape* only: `timeout_seconds` param
   (`mcpgo.WithNumber(..., DefaultNumber(...), Min(1), Max(...))`), and a
   `matched=false`/timeout result on expiry rather than an error.
5. `defer h.eventBus.Unsubscribe(subID)` for cleanup on every exit path
   (timeout, match, caller cancellation) — the same pattern every other
   `Subscribe` call site in the codebase already uses.

No fork of the ConnectRPC client or the scrollback-polling ticker is needed;
the two existing patterns contribute *conventions* (tool shape from
`wait_for_output`, subscribe/filter/replay logic from `watchBacklogItems`),
and the actual bounded-wait glue is new but small (context.WithTimeout +
select, per §3).
