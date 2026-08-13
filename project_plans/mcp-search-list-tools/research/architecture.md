# Architecture Research: MCP search/list/filter tool exposure

No prior `server/mcp/` hotspot/architecture research exists elsewhere in `project_plans/`
(grepped for "server/mcp" across all `project_plans/*/research/*.md` — the only hit besides
this project is `project_plans/mcp-search-list-tools/requirements.md` itself, and unrelated
docs referencing "MCP" generically like `stapler-squad-mcp-server`, `agent-protocol-architecture`).
This doc is derived fresh from reading `server/mcp/*.go` and `server/services/*.go` directly.

## 1. Call path: MCP tool handler → ??? → ConnectRPC / storage

There are **two distinct patterns already in production** in `server/mcp/`, not one:

**Pattern A — bypass ConnectRPC entirely, call storage/domain layer directly.**
`search_sessions` (`server/mcp/tools_discovery.go:183`, registered at `server/mcp/server.go:151`)
does **not** call any ConnectRPC service method. `discoveryHandlers` (`server/mcp/tools_discovery.go:15`)
holds only `store session.InstanceStore`. The handler calls `d.store.LoadInstances()` directly
and reimplements filtering/matching itself (`matchesSearch`, `server/mcp/tools_discovery.go:240`) —
there is no `SessionService.SearchSessions` ConnectRPC method at all; the web UI's session
search is client-side over the same `LoadInstances()`/`SessionSummary` data, so there was
nothing to share. `get_backlog_item` (`server/mcp/tools_backlog.go:194`) follows the same
pattern: `h.storage.GetBacklogItem(ctx, itemID)` called directly, with the MCP handler doing
its own JSON marshaling — not reusing the ConnectRPC `BacklogService.GetBacklogItem`
proto-conversion path.

**Pattern B — call the ConnectRPC service method in-process, as a plain Go function call.**
Several MCP tools already hold a live `*services.SessionService` or `*services.BacklogService`
and call its exported RPC methods directly, constructing a `connect.Request[T]` and unwrapping
`connect.Response[R].Msg`. Evidence: `backlogHandlers` (`server/mcp/tools_backlog.go:118`) holds
`backlogSvc *services.BacklogService` specifically so `createBacklogItem`/`importGitHubIssue`
can call `h.backlogSvc.MaybeTriggerTriage(...)` — a plain (non-RPC) method on the same service
struct that the RPC handler `BacklogService.CreateBacklogItem` also calls, so the MCP path and
the HTTP path share triage-trigger behavior (this was BUG-061: before the fix, the MCP tool
called `h.storage.CreateBacklogItem` directly and *skipped* triage, diverging from the RPC
handler — the comment at `server/mcp/tools_backlog.go:126-134` documents this explicitly).
`workflowHandlers`/`rulesHandlers` (`server/mcp/tools_workflow.go:19`, `server/mcp/tools_rules.go:17`)
hold `svc *services.SessionService` and call its RPC methods (`ListWorkflows`, `RunWorkflow`,
etc.) directly — no HTTP, no separate interface, just an in-process Go method call against the
same struct the ConnectRPC HTTP handler is registered with in `server/server.go:429`
(`sessionv1connect.NewBacklogServiceHandler(deps.BacklogService, ...)`).

**Conclusion for this task:** there is no separate "thin shared internal function" layer
distinct from the RPC handler — `*services.SessionService` and `*services.BacklogService` are
themselves plain Go structs with exported methods matching the ConnectRPC signature
(`func(ctx, *connect.Request[T]) (*connect.Response[R], error)`); MCP calls that method
in-process, HTTP calls it via the generated ConnectRPC transport, same code. For the three
target RPCs, follow **Pattern B** (call the service method directly, plain Go call, wrap/unwrap
`connect.Request`/`connect.Response`) rather than Pattern A — see rationale in §2.

## 2. Per-RPC mapping

| RPC | Implementing file | Struct | Accessible from MCP via | Extra ctx/scoping needed |
|---|---|---|---|---|
| `GetNotificationHistory` | `server/services/notification_service.go:137` (impl); forwarded by `server/services/session_service.go:3280` | `*NotificationService` (impl), `*SessionService` (forwarder) | `svc.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{...}))` — `svc` already threaded into `NewCore` | None. Params (`limit`, `offset`, `type_filter`, `session_id`, `unread_only`) are all explicit request fields, not derived from context. |
| `ListBacklogItems` | `server/services/backlog_service_query.go:107` | `*BacklogService` | `backlogSvc.ListBacklogItems(ctx, connect.NewRequest(&sessionv1.ListBacklogItemsRequest{...}))` — `backlogSvc` already threaded into `backlogHandlers` (`server/mcp/tools_backlog.go:118`) | None beyond the existing `h.enabledCheck` feature-flag guard every other backlog tool already applies via `featureDisabledResult(h.enabledCheck)` (`server/mcp/tools_backlog.go:69`). |
| `SearchClaudeHistory` | `server/services/search_service.go:459` (impl); forwarded by `server/services/session_service.go:3038` | `*SearchService` (impl), `*SessionService` (forwarder) | `svc.SearchClaudeHistory(ctx, connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{...}))` | None. Fields: `query` (required), `project`, `model`, `start_time`/`end_time` (proto), `limit`/`offset`. |

`ListBacklogItems`' RPC handler does non-trivial work beyond the raw storage call —
`filter := session.BacklogItemFilter{...}` construction from proto fields, then
`s.storage.ListBacklogItemSummaries(ctx, filter)`, then `backlogItemSummaryToProto(&summaries[i], costFor)`
where `costFor := s.buildCostLookup()` is an **unexported** `*BacklogService` method. This is a
concrete reason Pattern A (reimplement in MCP against `h.storage` directly) is *worse* here
than for `get_backlog_item`: the cost-lookup logic isn't reachable from the MCP package without
either exporting `buildCostLookup` (adds surface area for one caller) or duplicating cost
computation — calling `h.backlogSvc.ListBacklogItems` in-process avoids both.

`SearchClaudeHistory` and `GetNotificationHistory` are both accessible as `SessionService`
methods since `SessionService` already forwards to `searchSvc`/`notificationSvc` — no new
field is needed if the new MCP handler(s) hold `svc *services.SessionService`, matching
`workflowHandlers`/`rulesHandlers`' existing field.

## 3. Data flow / consistency

Both `SearchClaudeHistory` and `GetNotificationHistory` are **safe to call from any goroutine
with just a `context.Context`** — neither depends on HTTP-request-scoped state:

- `SearchClaudeHistory` → `ss.getOrRefreshHistoryCache(ctx)` (`server/services/search_service.go:172`)
  uses an atomic-pointer cache (`ss.historySnap.Load()`) with TTL, refreshed via
  `singleflight.Group.Do("refresh", ...)` to coalesce concurrent refreshes — this is exactly the
  double-checked-locking pattern this repo's own
  `.claude/rules/go-double-checked-locking.md` documents (returns the locally-computed value on
  the coalesce race, not a stale re-read). The cache loads from disk
  (`session.NewClaudeSessionHistoryFromClaudeDir()`), not from any per-request state. `ctx` is
  used only for OpenTelemetry span propagation (`trace.SpanFromContext(ctx)`,
  `telemetry.StartSpan(ctx, ...)`), which degrades gracefully to a no-op span if the context
  carries none (i.e. calling from an MCP goroutine's plain `context.Background()`-derived ctx
  works fine, no span parent required).
- `GetNotificationHistory` reads from `ns.notificationStore.List(opts)`, a persistent
  `*notifications.NotificationHistoryStore` — a nil-guarded struct field, not populated per-HTTP-request.
- `ListBacklogItems` reads from `s.storage.ListBacklogItemSummaries(ctx, filter)` — the same
  `*session.Storage` every other backlog MCP tool already reads from directly.

No consistency concern was found (no in-memory cache requiring an active HTTP request to
populate, no auth-context requirement, no workspace-scoping requirement). The one existing
guard that *does* need threading through is the `backlogEnabled`/`enabledCheck` feature flag —
`ListBacklogItems` should apply `featureDisabledResult(h.enabledCheck)` like every other backlog
tool; `GetNotificationHistory`/`SearchClaudeHistory` have no equivalent flag today (they're
unconditionally registered whenever `svc != nil`, matching `registerWorkflowTools`/`registerRulesTools`).

## 4. Interface-pollution / primitive-obsession risk check

Against `.claude/rules/interface-pollution-checklist.md` and
`.claude/rules/primitive-obsession-checklist.md`:

- **No new interface needed.** `search_sessions`'s pattern already establishes: MCP handler
  structs hold the *concrete* service type (`*services.SessionService`, `*services.BacklogService`,
  `session.InstanceStore` — the last one is a pre-existing interface, not new). Do **not**
  introduce a speculative `SearchService`/`ListService`/`HistoryService` MCP-facing interface
  with a single implementation — that would be exactly smell #1 (speculative interface) and #2
  (interface defined next to nothing, since there's no second implementation on the horizon).
  Reuse the existing `svc *services.SessionService` / `backlogSvc *services.BacklogService`
  fields (or add them to whichever handler struct ends up owning the new tools) as plain
  concrete-type fields, exactly like `workflowHandlers`/`rulesHandlers`/`backlogHandlers` do.
- **No forwarding-only wrapper type risk.** The three target RPC calls are single Go
  method calls (`svc.GetNotificationHistory(ctx, req)`, etc.) inline in the tool handler
  function — there's no reason to wrap them in a `NotificationManager`/`SearchHandler` type.
  Follow `discoveryHandlers.searchSessions`'s shape: a plain method on the existing handler
  struct, argument extraction → request construction → call → result marshaling, no
  intermediate object.
- **Watch primitive-obsession on request construction, not signatures.** These MCP tool
  handlers don't define new function signatures with multiple same-typed params — they extract
  `args["x"].(string)`/`.(float64)` from `req.GetArguments()` (a `map[string]any`), matching every
  existing tool. This is idiomatic for the `mcpgo` handler shape used throughout the file and
  isn't a checklist violation (the target type, `sessionv1.SearchClaudeHistoryRequest` etc., is
  already a proper named struct on the proto side — not a pile of interchangeable primitive
  params in a Go function signature).
- One thing to actively avoid when writing the new handler(s): don't add a per-tool
  bespoke "SessionUUID + Query + Limit + Offset" struct as a new named MCP-only type when
  the target already has one (the generated `sessionv1.SearchClaudeHistoryRequest` proto struct)
  — construct that struct directly via `connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{...})`
  rather than inventing a parallel plain-Go args struct first and mapping it over.

## 5. Resource-scoped tools vs. unified `search` tool — recommendation

**Recommendation: resource-scoped tools, one per RPC** (`list_backlog_items`,
`get_notification_history`, `search_claude_history`), matching the `search_sessions` precedent.
Rationale, grounded in what's actually in the 55×-tool (39 literal `mcpgo.NewTool(` call sites
across `server/mcp/*.go`, per `grep -c`; some of that gap from the 55-count in requirements.md
vs. 39 counted here is likely tools registered via loop/table rather than a literal call site,
not re-derived here) registration set:

1. **Every existing tool in this file is resource-scoped already**, with distinct, differently-shaped
   parameter sets per tool (`list_sessions` vs `search_sessions` vs `get_session`; `get_backlog_item`
   vs the not-yet-existing `list_backlog_items`). A unified `search` tool spanning three RPCs with
   non-overlapping filter vocabularies (`status`/`priority`/`sort_by` for backlog;
   `type_filter`/`session_id`/`unread_only` for notifications; `project`/`model`/`start_time`/`end_time`
   for Claude history) would need either a large discriminated-union-shaped input schema (one
   tool description covering three unrelated filter sets — worse tool-selection accuracy for the
   calling LLM, which is the opposite of MCP tool design goals) or a resource-type dispatch
   parameter that just re-implements what three separate tool names already give the LLM for free
   via tool selection.
2. **The proto request shapes for these three RPCs share almost no fields** — `query` (string,
   full-text) only applies to `SearchClaudeHistory`; `status`/`priority` only to
   `ListBacklogItems`; `type_filter`/`unread_only` only to `GetNotificationHistory`. There is no
   natural "one struct fits all three" the way `list_sessions`/`search_sessions`/`get_session`
   at least share a `SessionSummary` output shape. Unifying would produce a request struct that
   is mostly-optional/mutually-irrelevant fields per resource — a primitive-obsession-adjacent
   smell in the request shape itself.
3. **`server/mcp/tools_backlog.go:204`'s existing hint text already names `list_backlog_items`**
   as the expected tool name (dangling reference target from acceptance criterion 4) — the
   resource-scoped name is already the de facto contract other tools' error messages point to.
4. Precedent tool-naming convention in this file is uniformly `<verb>_<resource>`
   (`list_sessions`, `get_session`, `search_sessions`, `get_backlog_item`,
   `create_backlog_item`, `list_workflows`, `run_workflow`) — never a single verb-only tool
   spanning multiple resource types. Introducing the first exception here has no
   requirements-driven justification (the "Out of scope" section explicitly defers
   `SearchFiles`/`SearchGitHubRepos` prioritization, i.e. this is scoped as three independent
   wiring tasks, not one cross-resource search feature).

Suggested tool names, following the `<verb>_<resource>` convention and the "default limit 10"
convention `list_sessions`/`search_sessions` already establish:
- `list_backlog_items` (mirrors `ListBacklogItemsRequest`; default limit convention n/a — the
  RPC itself has no limit/pagination field, returns full filtered set)
- `get_notification_history` (mirrors `GetNotificationHistoryRequest`; already has
  `limit`/`offset` — apply the same default-10-if-unset guard `list_sessions` uses at
  `server/mcp/tools_discovery.go:93-96`, since the proto default is `50` per
  `server/services/notification_service.go:182`, which is above the LLM-context-safe convention
  this project has already chosen for `list_sessions`)
- `search_claude_history` (mirrors `SearchClaudeHistoryRequest`; proto-side default is `20`
  per `server/services/search_service.go:498-500` — same "apply the repo's chosen default,
  not the proto's" consideration applies)
