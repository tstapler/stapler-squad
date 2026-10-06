# Research: Stack — MCP tools for search/list/filter RPCs

## Bottom line

No new dependency. This is a pure wrapping exercise on top of the MCP framework and
ConnectRPC handlers already vendored in this repo. The three new tools
(`list_backlog_items`, `get_notification_history`, `search_claude_history`) should each
be a `mcpgo.NewTool(...)` + handler method added to an existing `server/mcp/tools_*.go`
file, following patterns already established there — no `go.mod` change, no proto
change (the RPCs and their request/response messages already exist).

## MCP framework in use

- **Library**: `github.com/mark3labs/mcp-go`, pinned at **v0.48.0** in
  `go.mod:140` — this is the only MCP-related dependency in the module.
- Imported under two aliases throughout `server/mcp/*.go`:
  - `mcpgo "github.com/mark3labs/mcp-go/mcp"` — tool/schema builders
    (`NewTool`, `WithString`, `WithNumber`, `WithArray`, `WithDescription`,
    `Required()`, `Enum()`, `DefaultNumber()`, `DefaultString()`, `Min()`/`Max()`,
    `Items()`, `NewToolResultText`, `CallToolRequest`, `CallToolResult`).
  - `mcpserver "github.com/mark3labs/mcp-go/server"` — `MCPServer`,
    `NewStdioServer`, `NewStreamableHTTPServer`, `WithStateLess`.
- No indication a newer `mark3labs/mcp-go` release is needed for this feature —
  all the schema primitives the new tools require (string/number/array params,
  enums, defaults, min/max, pagination cursors) are already used elsewhere in this
  codebase on v0.48.0. Confirming the exact latest upstream release was out of
  scope for this pass (no `go list -m -u` / network check run); the recommendation
  is to **not** bump the dependency as part of this feature — do that separately if
  ever needed, since it's orthogonal to wiring these three tools.

## Registration pattern (established, to be followed exactly)

Each tool-file (`tools_discovery.go`, `tools_backlog.go`, `tools_workflow.go`, etc.)
follows the same three-part shape:

1. A `*Handlers` struct holding whatever dependency the tools need
   (`store session.InstanceStore`, `svc *services.SessionService`,
   `storage *session.Storage`, etc.) — see `discoveryHandlers` in
   `server/mcp/tools_discovery.go:15` and `backlogHandlers` in `tools_backlog.go`.
2. A `register<X>Tools(s *mcpserver.MCPServer, h *xHandlers)` function that calls
   `s.AddTool(mcpgo.NewTool(...), h.methodName)` once per tool — e.g.
   `registerDiscoveryTools` (`server/mcp/server.go:118-169`, three tools:
   `list_sessions`, `get_session`, `search_sessions`) and `registerBacklogTools`
   (`server/mcp/tools_backlog.go:1716`, ~10 tools).
3. Wiring into `NewCore` (`server/mcp/server.go:34-70`), which both
   `RunServer` (stdio) and `NewHTTPHandler` (Streamable HTTP `/mcp`) share — so a
   tool registered here is automatically exposed on both transports. Backlog
   tools are gated: `if storage != nil && (backlogEnabled == nil || backlogEnabled())`
   (`server.go:63`) — the new `list_backlog_items` tool must go inside/alongside
   that same gate since it depends on the same feature flag and `storage`.

## Two data-access sub-patterns already in play — pick per-tool

- **Direct storage/repository call** (used by all existing backlog tools): handler
  calls `h.storage.GetBacklogItem(ctx, itemID)` etc. directly against
  `session.Storage` (backed by `session.EntRepository`), not through the
  ConnectRPC service layer. `session.Storage.ListBacklogItems(ctx context.Context,
  filter session.BacklogItemFilter) ([]session.BacklogItemData, error)`
  (`session/storage.go:727`) already exists with this exact signature and is what
  `list_backlog_items` should call — it's the same method
  `server/services/session_service.go`'s `ListBacklogItems` RPC handler itself
  calls, so wrapping it directly avoids an extra ConnectRPC round-trip layer for
  no benefit (consistent with how `getBacklogItem`/other backlog tools are already
  written).
- **ConnectRPC service call** (used by workflow/rules tools): handler calls
  `h.svc.SomeRPC(ctx, connect.NewRequest(&sessionv1.SomeRequest{...}))` directly
  against `*services.SessionService`, then unwraps `.Msg`. See
  `server/mcp/tools_workflow.go:18-19` (`workflowHandlers{svc *services.SessionService}`).
  This is the natural fit for `GetNotificationHistory` and `SearchClaudeHistory`,
  since those are implemented as `SessionService` methods that delegate to
  `NotificationService`/`SearchService` internally:
  - `(*services.SessionService).GetNotificationHistory` —
    `server/services/session_service.go:3280`, delegating to
    `(*services.NotificationService).GetNotificationHistory` —
    `server/services/notification_service.go:137`.
  - `(*services.SessionService).SearchClaudeHistory` —
    `server/services/session_service.go:3039`, delegating to
    `(*services.SearchService).SearchClaudeHistory` —
    `server/services/search_service.go:459`.
  - Both are exposed as ConnectRPC handlers already (`sessionv1connect` bindings
    in `gen/proto/go/session/v1/`), so `discoveryHandlers` (which already holds
    `store session.InstanceStore` — would need `svc *services.SessionService`
    added, mirroring `workflowHandlers`/`rulesHandlers`) or a small new handler
    struct is the natural home. `svc` is already threaded into `NewCore`
    unconditionally (not behind a nil-guard for discovery tools the way backlog
    is), so no new plumbing is required to reach it from a discovery-adjacent
    tool file.

## Argument parsing and error conventions (established, must reuse — do not reinvent)

- `req.GetArguments()` returns `map[string]any`; each field is manually
  type-asserted, e.g. `itemID, ok := args["item_id"].(string)`
  (`tools_backlog.go:199`). No struct-tag/reflection-based binding is used
  anywhere in this package — stay consistent, don't introduce one.
- Errors are returned via `errResult(code, message, hint string) *mcpgo.CallToolResult`
  (`server/mcp/types.go`) with `nil` as the Go `error` return — MCP tool errors are
  encoded in the result payload, not as Go errors, e.g.
  `return errResult(ErrInvalidArgument, "item_id is required", ""), nil`.
- Error code constants already defined in `server/mcp/types.go`:
  `ErrInvalidArgument`, `ErrInternalError`, plus `ErrItemNotFound`,
  `ErrPermissionDenied`, `ErrFeatureDisabled` defined in `tools_backlog.go:60-64`.
  `search_claude_history`/`get_notification_history` will mostly need
  `ErrInvalidArgument` (bad filter/date args) and `ErrInternalError` (backend
  failure) — no new error code should be needed.
- `featureDisabledResult(h.enabledCheck)` gate at the top of every backlog handler
  (`tools_backlog.go:69-74`) — `list_backlog_items` must include this since it's
  registered under the same `backlogEnabled` flag. `get_notification_history` and
  `search_claude_history` are not backlog-gated in the requirements, so they
  likely don't need this specific gate (confirm in planning phase whether they
  need any gate at all, e.g. a search-index-availability check for
  `SearchClaudeHistory`, since `session/search/` is described as file-backed).
- Success results: `mcpgo.NewToolResultText(...)`, typically a hand-built
  human-readable string plus/or a JSON envelope (see `getBacklogItem`'s
  `envelope` construction around `tools_backlog.go:220-317`).

## Pagination pattern to reuse for `search_claude_history` (context-safe default limit)

`list_sessions`' cursor pattern (`server/mcp/tools_discovery.go:19-153`) is the
established precedent for "default limit to avoid filling LLM context" mentioned
in the requirements (`server.go:121`: *"Default limit is 10 to avoid filling LLM
context"*):

- `paginationCursor{LastTitle, CreatedAt}` struct, `encodeCursor`/`decodeCursor`
  helpers (base64/JSON — check exact encoding in `tools_discovery.go:19-45`).
- Tool schema: `mcpgo.WithNumber("limit", mcpgo.DefaultNumber(10), mcpgo.Min(1),
  mcpgo.Max(...))` + `mcpgo.WithString("cursor", ...)`.
- `SearchClaudeHistory`'s own request already supports pagination server-side
  (per requirements: "with `project`, `model`, `start_time`/`end_time` filters and
  pagination") — the MCP tool likely just needs to surface the RPC's own
  limit/offset or cursor fields with an LLM-safe default (e.g. `DefaultNumber(10)`
  matching `list_sessions`' convention) rather than reimplementing pagination from
  scratch; confirm the exact `SearchClaudeHistoryRequest` pagination shape
  (cursor vs offset) in the request/proto reading during planning
  (`proto/session/v1/session.proto:972`).

## Testing convention

Every `tools_*.go` file has a matching `tools_*_test.go` (see
`tools_backlog_test.go`, `tools_discovery.go` has no separate test file listed —
check `server_test.go`/`server_integration_test.go` for discovery-tool coverage).
New tools need matching Go tests in whichever file houses their handler struct,
per acceptance criterion 5. `feature_flag_test.go` is the existing pattern for
`featureDisabledResult` gating tests if `list_backlog_items` needs one.

## Summary of concrete file targets

| Tool | Likely home file | Data-access pattern | Registration site |
|---|---|---|---|
| `list_backlog_items` | `server/mcp/tools_backlog.go` (new handler method + `AddTool` call inside `registerBacklogTools`) | `h.storage.ListBacklogItems(ctx, session.BacklogItemFilter{...})` direct | Already gated by `backlogEnabled` in `NewCore` (`server.go:63`) — no new gating code needed, just add inside `registerBacklogTools` |
| `get_notification_history` | `server/mcp/tools_discovery.go` or new `tools_notifications.go` | `h.svc.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{...}))` | Needs `svc *services.SessionService` on the handler struct; register unconditionally (svc is always non-nil per `NewCore`'s existing unconditional `registerDiscoveryTools`/`registerTerminalTools` calls) or guarded by `if svc != nil` like `registerWorkflowTools` |
| `search_claude_history` | same file as above, or its own `tools_search.go` | `h.svc.SearchClaudeHistory(ctx, connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{...}))` | same as above |

No proto changes needed (all three request/response messages already exist and
are used by the web UI); no `make proto-gen` step required for this feature.
