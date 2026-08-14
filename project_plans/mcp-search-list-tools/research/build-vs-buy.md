# Research: Build vs. Buy — MCP exposure for ListBacklogItems / GetNotificationHistory / SearchClaudeHistory

Agent 6 output for the `mcp-search-list-tools` SDD planning cycle. Research-only — no source
changes made.

## Question

Should the 3 target MCP tools be hand-written following the existing `server/mcp/*.go`
`mcpgo.NewTool` pattern, or is there an existing solution (library, generator, or closer-to-done
in-repo code) to adopt instead?

## 1. Existing OSS library/framework — generic ConnectRPC/gRPC → MCP bridge

**Verdict: Not recommended (none fits; none exists in-repo).**

`go.mod` has exactly one MCP-related dependency:

```
github.com/mark3labs/mcp-go v0.48.0
```

This is the same low-level MCP protocol library already used for every hand-rolled tool in
`server/mcp/*.go` (`mcpgo.NewTool`, `mcpgo.WithString`, etc.) — it is not a generator, it's the
SDK the hand-written tools are built on. There is no second MCP-related dependency (no
reflection-based bridge, no OpenAPI-to-MCP tool, no gRPC-to-MCP codegen plugin).

`grpc-ecosystem/grpc-gateway/v2` appears in `go.mod` but only as an `// indirect` transitive
dependency (pulled in by something else in the graph) — the codebase's actual REST/RPC surface is
served via Connect (`bufbuild/connect-go`), not grpc-gateway, and nothing wires grpc-gateway's
reflection/codegen into MCP tool generation.

Pros of a generic bridge (hypothetical): would auto-generate 3 tools instantly, stays in sync as
RPC schemas evolve.

Cons (why it doesn't apply here): no such library exists in the current dependency graph; adding
one for 3 RPCs is disproportionate; every other MCP tool in this repo (55+) is hand-rolled with
per-tool argument validation, feature-flag gating (`featureDisabledResult`), and custom
`okResult`/`errResult` response shaping (e.g. Markdown-formatted text output, `SanitizeForAgentContext`
truncation) that a generic reflection-based bridge would not reproduce — the repo has deliberately
chosen hand-tuned MCP-shaped output over a 1:1 RPC mirror (see `list_sessions`'s "Default limit is
10 to avoid filling LLM context" convention, `server/mcp/server.go:121`, which is UX policy no
generator would infer). Introducing a new dependency for 3 mechanical wrappers also violates the
repo's existing precedent of zero MCP-generation tooling.

## 2. SaaS/managed API

**Not applicable.** This feature wraps in-process ConnectRPC handlers (`server/services/*.go`)
that already run inside the same Go binary as the MCP server (`server/mcp/server.go`'s
`RunServer`/`NewCore`). There is no external network boundary, no third-party service, and no
hosted API to buy — the "build" is entirely local Go code calling other local Go code via
`connect.NewRequest`/`connect.Response`, in-process (see workflow tools below).

## 3. LLM-generated implementation vs. battle-tested library — is there a nontrivial algorithm here?

**Verdict: No — the hard part (full-text search) is already a battle-tested in-repo library, and
this feature does not touch it.**

Confirmed: `session/search/` exists and contains a real inverted-index/BM25 search engine —
`engine.go`, `bm25.go`, `inverted_index.go`, `tokenizer.go`, `document_store.go`,
`index_store.go`, `snippet.go`, each with a matching `_test.go`. `SearchClaudeHistory` is
implemented at `server/services/search_service.go:459` (`func (ss *SearchService)
SearchClaudeHistory`) and is exercised by tests in
`server/services/history_service_test.go`. The MCP tool for this RPC is a pure wrapper: it
constructs a `SearchClaudeHistoryRequest`, calls the existing service method, and maps the
response — it does not reimplement tokenization, ranking, or indexing. Same for
`ListBacklogItems` (`server/services/backlog_service_query.go:107`, pagination/filter logic
already exists) and `GetNotificationHistory` (`server/services/notification_service.go:137`,
called through `server/services/session_service.go:3280`'s thin passthrough).

There is no case here for "should we use a library instead of hand-writing this" — the
nontrivial logic (search ranking, backlog filter semantics, notification pagination) is already a
library, already used by the web UI, and this feature's only job is to expose it through another
transport (MCP tool call → ConnectRPC in-process call), which is inherently thin, mechanical
per-RPC glue code, not an algorithm.

## 4. Fork or adapt — closest existing in-repo template per target RPC

**Verdict: Recommended.** The repo already has two flavors of MCP tool, and the right template
differs depending on whether the target RPC is reached through an in-process ConnectRPC handler
call (the common case here) or through local field-matching (the `search_sessions` case, which
does *not* apply to these 3 targets since all 3 already have real backend RPCs, unlike
`search_sessions` which has no corresponding `SessionService.SearchSessions` RPC at all — it
operates directly on `session.Instance` loaded from `d.store.LoadInstances()`,
`server/mcp/tools_discovery.go:210`).

The actual best template is **`tools_workflow.go`'s `listWorkflows`** handler
(`server/mcp/tools_workflow.go:291-306`), because it is the only existing MCP tool that does
exactly what all 3 new tools need to do: call an existing ConnectRPC service method in-process via
`connect.NewRequest`, then map the typed proto response into a small MCP result struct:

```go
func (h *workflowHandlers) listWorkflows(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	resp, err := h.svc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	out := make([]WorkflowResult, 0, len(resp.Msg.GetWorkflows()))
	for _, w := range resp.Msg.GetWorkflows() {
		out = append(out, workflowToResult(w))
	}
	return okResult(ListWorkflowsResult{MCPResult: MCPResult{Success: true}, Workflows: out}), nil
}
```

Per-target mapping:

| Target RPC | Best template | Why |
|---|---|---|
| `ListBacklogItems` | `tools_workflow.go`'s `listWorkflows` (call shape) + `tools_backlog.go`'s existing `backlogHandlers` struct (already holds `storage`, `backlogSvc *services.BacklogService`, `enabledCheck`) for feature-flag gating and error-result conventions. `ListBacklogItems` itself lives at `server/services/backlog_service_query.go:107`. | Same package/struct as `get_backlog_item` et al. — the new tool's handler is a peer method on `backlogHandlers`, not a new struct. Also resolves the dangling `list_backlog_items` reference at `server/mcp/tools_backlog.go:204`. |
| `GetNotificationHistory` | `tools_workflow.go`'s `listWorkflows` (call shape) — reached via `SessionService.GetNotificationHistory` (`server/services/session_service.go:3280`, thin passthrough to `NotificationService.GetNotificationHistory`, `server/services/notification_service.go:137`). | `SessionService` is already held as `svc *services.SessionService` on `workflowHandlers`/`lifecycleHandlers`/etc. — same in-process call pattern, same field name convention. |
| `SearchClaudeHistory` | `tools_workflow.go`'s `listWorkflows` (call shape, for the RPC call itself) + `list_sessions`'s tool-definition conventions (`server/mcp/server.go:120-135`, default-limit-10 pagination doc string) for the MCP argument schema/UX. Backing RPC at `server/services/search_service.go:459`. | Needs a `*services.SearchService` reference on whatever handler struct hosts it (new or existing) — same `h.svc.Method(ctx, connect.NewRequest(...))` shape as `listWorkflows`, but the *tool description and default-limit UX* should copy `list_sessions`'s explicit "avoid filling LLM context" convention since this is the one target RPC that returns full-text search results (highest token-blowup risk of the three). |

Every one of the 3 new tools additionally must follow the shared conventions already
demonstrated across `server/mcp/*.go`: `featureDisabledResult` gate, `errResult`/`okResult`
helpers, argument extraction via `req.GetArguments()` with manual type assertions (no reflection),
and a colocated `*_test.go` (every existing `tools_*.go` file has one, per acceptance criterion 5
in `requirements.md`).

## Summary table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| 1. Generic OSS gRPC/ConnectRPC → MCP bridge | Instant generation, stays in sync with schema | Doesn't exist in this dependency graph; would bypass the repo's deliberate hand-tuned MCP UX (context-safe limits, feature gating, formatted text output); new dependency for 3 mechanical wrappers | **Not recommended** |
| 2. SaaS/managed API | — | Not applicable — everything is in-process Go | **N/A** |
| 3. Reimplement search/filter logic vs. use library | — | Not needed — `session/search/` (BM25 inverted index) and the backlog/notification filter logic already exist, tested, and used by the web UI; these tools only add a transport-layer wrapper | **N/A (already using the library)** |
| 4. Fork/adapt closest in-repo template | Mechanical, ~30-60 lines/tool, matches 55+ existing tools' conventions exactly, no new dependency, satisfies acceptance criteria 4-6 by construction | Still 3x hand-written handler + test files (no shortcut) | **Recommended** |

## Confirmed conclusion

The requirements doc's suggested entry point ("each tool is a mechanical RPC→MCP-tool wrap
following the `search_sessions` template") is **directionally correct but the more precise
template is `tools_workflow.go`'s `listWorkflows`**, not `search_sessions` itself —
`search_sessions` is the odd one out among existing tools because it has no backing RPC and
matches locally over `session.Instance`, whereas all 3 new targets (`ListBacklogItems`,
`GetNotificationHistory`, `SearchClaudeHistory`) already have real ConnectRPC handlers to call
in-process, making `listWorkflows`'s `h.svc.Method(ctx, connect.NewRequest(...))` → map-response
shape the closer structural match. `search_sessions`/`list_sessions` remain the right reference
for MCP-facing *UX conventions* (default limit, pagination doc strings) but not for the Go-level
call pattern. Overall verdict: **hand-write all 3 tools following the in-repo
`listWorkflows`-style RPC-wrapping pattern, no new dependency.**
