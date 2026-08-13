# Stack Research: MCP search/list/filter tool exposure

## 1. MCP SDK

`go.mod:140`: `github.com/mark3labs/mcp-go v0.48.0`.

Imported in `server/mcp/server.go` under two aliases:
- `mcpgo "github.com/mark3labs/mcp-go/mcp"` — tool/schema definition types (`NewTool`,
  `WithString`, `WithNumber`, `WithArray`, `Description`, `Required`, `Enum`, `DefaultNumber`,
  `Min`/`Max`, `CallToolRequest`, `CallToolResult`).
- `mcpserver "github.com/mark3labs/mcp-go/server"` — `MCPServer`, `NewMCPServer`,
  `NewStdioServer`, `NewStreamableHTTPServer`, `WithToolCapabilities`, `WithStateLess`.

No `go:generate` directives exist under `server/mcp/` — adding a tool is a pure hand-written
Go change, no codegen step analogous to `make proto-gen` for the *tool* wiring itself. (Proto
regeneration is only needed if you touch a `.proto` file, which is not required here — see §4.)

## 2. Established tool-definition pattern

Server construction lives in `server/mcp/server.go`'s `NewCore` (`server/mcp/server.go:34`),
which wires up one `register*Tools(s, &xHandlers{...})` call per tool family and conditionally
skips registration when an optional dependency is `nil` (e.g. backlog/goal tools only register
`if storage != nil && (backlogEnabled == nil || backlogEnabled())`, GitHub tools only `if
prCache != nil`). `NewHTTPHandler` and `RunServer` both funnel through `NewCore`, so a tool
registered there is automatically available on both the stdio and HTTP/Streamable transports.

**Per-family file convention**: `tools_<family>.go` (`tools_backlog.go`, `tools_discovery.go`,
`tools_workflow.go`, `tools_rules.go`, `tools_github.go`, `tools_terminal.go`, `tools_vcs.go`,
`tools_goal.go`, `tools_lifecycle.go`) each define:
- a `type <family>Handlers struct { ... }` holding the dependencies the family's tools need
  (often `svc *services.SessionService`, sometimes `storage *session.Storage`, `store
  session.InstanceStore`, plus optional fields with nil-safety doc comments explaining the
  fallback behavior),
- a `func register<Family>Tools(s *mcpserver.MCPServer, h *<family>Handlers)` that calls
  `s.AddTool(mcpgo.NewTool(name, ...schema options...), h.handlerMethod)` once per tool,
- one `func (h *<family>Handlers) <toolName>(ctx context.Context, req mcpgo.CallToolRequest)
  (*mcpgo.CallToolResult, error)` per tool.

**Input schema definition** (see `server/mcp/server.go:120-168`, `search_sessions`):
```go
s.AddTool(
    mcpgo.NewTool("search_sessions",
        mcpgo.WithDescription("Search sessions by text query or tags. ..."),
        mcpgo.WithString("query",
            mcpgo.Description("Search query matched against title, path, branch, and tags"),
            mcpgo.Required(),
        ),
        mcpgo.WithArray("tag_filter",
            mcpgo.Description("Filter to sessions that have all of these tags"),
        ),
        mcpgo.WithNumber("limit",
            mcpgo.Description("Max results (default 10, max 50)"),
            mcpgo.DefaultNumber(10),
            mcpgo.Min(1),
            mcpgo.Max(50),
        ),
    ),
    d.searchSessions,
)
```

**Handler body pattern** (`server/mcp/tools_discovery.go:183` `searchSessions`, and
`server/mcp/tools_backlog.go:190` `getBacklogItem`):
1. `args := req.GetArguments()` then manual type-assert each field out of the
   `map[string]interface{}` (e.g. `query, ok := args["query"].(string)`; numbers arrive as
   `float64` — `limitF, _ := args["limit"].(float64)`; arrays as `[]interface{}` requiring a
   per-element type assertion loop — see `tools_discovery.go:198-208` for the `tag_filter`
   extraction idiom).
2. Validate required fields inline, return `errResult(ErrInvalidArgument, "...", "remediation
   hint")` (no Go `error` return — the second return value `nil` is *always* nil for
   well-formed tool errors; `errResult` builds an `*mcpgo.CallToolResult` carrying the error
   payload). `errResult` is defined once in `server/mcp/tools_discovery.go:73` and reused by
   every tool file.
3. Backlog tools additionally guard with `if r := featureDisabledResult(h.enabledCheck); r !=
   nil { return r, nil }` as their first line when the family is flag-gated
   (`server/mcp/tools_backlog.go:69`).
4. Clamp/normalize (limit bounds, lowercasing, etc.) match the bounds already declared in the
   schema (`Min`/`Max`/`DefaultNumber`) — the schema is documentation/hinting only; mcp-go does
   not appear to enforce it server-side, so handlers re-clamp defensively.
5. Call into the backend — **two call shapes coexist**, both established:
   - **Direct storage/service-layer call** (bypasses ConnectRPC entirely): e.g.
     `getBacklogItem` calls `h.storage.GetBacklogItem(ctx, itemID)` directly
     (`session.Storage`, not a proto-typed RPC) and `search_sessions` calls
     `d.store.LoadInstances()` + an in-process filter (`session.InstanceStore`). Used when the
     MCP tool's shape doesn't need to match the RPC's request/response proto 1:1, or where
     going through connect would just be unwrapping overhead.
   - **ConnectRPC-handler-via-connect.NewRequest call** (wraps the same handler the web UI
     hits): every `tools_workflow.go` and `tools_rules.go` mutation goes through
     `h.svc.<Method>(ctx, connect.NewRequest(&sessionv1.<Method>Request{...}))` and unwraps
     `resp.Msg` — e.g. `server/mcp/tools_workflow.go:215` (`CreateWorkflow`),
     `:257` (`UpdateWorkflow`), `:277` (`DeleteWorkflow`), `:292` (`ListWorkflows`), `:322`
     (`RunWorkflow`); `server/mcp/tools_rules.go:142/201/218` (`ListApprovalRules`,
     `UpsertApprovalRule`, `DeleteApprovalRule`). **This is the closer precedent for the three
     target RPCs** (`ListBacklogItems`, `GetNotificationHistory`, `SearchClaudeHistory`) since
     all three already exist as full `*services.SessionService` methods with populated proto
     request/response types — no proto-to-storage translation needs to be hand-rolled the way
     `getBacklogItem`/`search_sessions` do.
6. Build a human-readable text response (many backlog handlers hand-build a `strings.Builder`
   Markdown-ish summary — see `getBacklogItem`, `server/mcp/tools_backlog.go:216-260`) and
   return `mcpgo.NewToolResultText(...)` (pattern present elsewhere in the same files, not
   shown in the excerpt above but consistent across all handlers).

## 3. Target RPC signatures (all already implemented — no new backend logic needed)

All three are ConnectRPC handlers taking `*connect.Request[XRequest]` and returning
`*connect.Response[XResponse], error`, reachable via `*services.SessionService` (a facade —
each method one-line-delegates to a sub-service, so calling `svc.<Method>` is equivalent to
calling the sub-service directly and matches the existing `h.svc.*` call convention used by
workflow/rules tools):

- **`ListBacklogItems`** — `server/services/backlog_service_query.go:107`
  (`*BacklogService`, delegated from `SessionService`). Request `sessionv1.
  ListBacklogItemsRequest` (`proto/session/v1/backlog.proto:336`): `status []string`,
  `priority []int32`, `sort_by string`, `include_terminal bool`, `include_archived bool`.
  Response: `Items []*sessionv1.BacklogItem`. Internally calls
  `s.storage.ListBacklogItemSummaries(ctx, session.BacklogItemFilter{...})` — note this is a
  *different* storage method than the one `get_backlog_item`'s MCP handler already calls
  (`h.storage.GetBacklogItem`), so a new `list_backlog_items` tool should go through
  `*services.BacklogService`/`SessionService`, not reimplement filter-building against
  `session.Storage` directly, to stay in sync with the web UI's filtering semantics (default
  exclusion of done/archived unless requested, etc.). Marked `// +api: backlog:list-items`.
  `server/mcp/tools_backlog.go:204`'s existing `list_backlog_items`-referencing error-hint
  string (in `get_backlog_item`'s UUID-validation remediation text) is currently a dangling
  reference to a tool that doesn't exist — confirms this gap directly from within the MCP
  package itself.

- **`GetNotificationHistory`** — `server/services/notification_service.go:137`
  (`*NotificationService`, delegated from `SessionService` at
  `server/services/session_service.go:3280`). Request `sessionv1.
  GetNotificationHistoryRequest` (`proto/session/v1/session.proto:1367`): all fields optional
  pointers — `limit *int32`, `offset *int32`, `type_filter *NotificationType`, `session_id
  *string`, `unread_only *bool`. Response: `Notifications
  []*NotificationHistoryRecord`, `TotalCount int32`, `UnreadCount int32`, `HasMore bool`.
  Internally calls `ns.notificationStore.List(notifications.ListOptions{...})`.

- **`SearchClaudeHistory`** — `server/services/search_service.go:459` (`*SearchService`,
  delegated from `SessionService` at `server/services/session_service.go:3039`, marked
  `// +api: history:search`). Request `sessionv1.SearchClaudeHistoryRequest`
  (`proto/session/v1/session.proto:972`): `query string` (required), `project *string`,
  `model *string`, `start_time/end_time *timestamppb.Timestamp`, `limit int32` (default 20,
  max 100 — clamped server-side same as the schema's declared bounds should be), `offset
  int32`. Response includes search results plus pagination info (not fully read past line
  520, but follows the same total-count/offset/limit shape as the others). This is backed by
  the real inverted-index engine (`ss.searchEngine`, `session/search/` per the requirements
  doc) with an `IncrementalSync` step run on every call — i.e. it's not a cheap in-memory
  filter like `search_sessions`, so no additional caching/sync logic is needed in the MCP
  handler, just pass through.

## 4. Versioning / codegen concerns

- **No proto changes required.** All three RPCs and their request/response messages already
  exist in `proto/session/v1/{backlog,session}.proto` and are already used by the web UI —
  this is a pure MCP-surface wiring gap, not a backend gap (confirms the requirements doc's
  framing). `make proto-gen` is **not** needed unless a future iteration wants to add
  MCP-specific fields not already on these request messages.
  - Corollary: an MCP tool schema does not have to expose every proto field 1:1 — e.g. it's
    reasonable to omit `SearchClaudeHistoryRequest.start_time`/`end_time` from the initial
    tool schema if timestamp-typed MCP tool inputs are awkward, since `mcpgo.WithString`/
    `WithNumber` don't have a native timestamp type; would need string parsing into
    `timestamppb.Timestamp` in the handler, matching the "manual type-assert from
    `map[string]interface{}`" convention above.
- **No mcp-go codegen.** The library is used purely as an imperative builder API
  (`mcpgo.NewTool(...)` chains); there's no schema-file/IDL step to regenerate for mcp-go
  itself, unlike proto. Adding a tool is `s.AddTool(...)` + a handler method + wiring the new
  `register*Tools` call (or adding tools to an existing family file/registration point) into
  `NewCore`.
- **Feature registry** (separate from MCP, but likely relevant to the eventual PR per
  `.claude/rules/feature-registry.md`): these are pre-existing RPCs already registered in
  `docs/registry/features/backend/`; adding an MCP tool wrapping them does not require a new
  registry entry (registry entries are per-RPC, not per-transport), but `make
  registry-generate` should still be re-run to be safe since MCP tool registration functions
  match the `// +api:` marker scanning the registry tooling does elsewhere in this codebase —
  worth double-checking `docs/registry/README.md` during planning, not confirmed further here
  (out of scope for the Stack research question).
- **Dependency version**: `mark3labs/mcp-go v0.48.0` is a single pinned version with no
  multi-version concern found (only one entry in `go.sum`); no upgrade needed to add tools —
  all the schema-builder functions used above (`WithString`, `WithNumber`, `WithArray`,
  `Enum`, `DefaultNumber`, `Min`/`Max`, `Required`) are already exercised elsewhere in the
  package at this version.
