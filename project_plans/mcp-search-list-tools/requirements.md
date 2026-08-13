# Requirements: MCP search/list/filter tool exposure

## Problem

`server/mcp/` registers 55 `mcpgo.NewTool(...)` calls (39+ per the backlog item's original
count, now 55 per current `grep`), but the RPCs that back full-text/filtered search and list
operations for notifications, backlog items, and Claude/session history are not among them.
An LLM operating stapler-squad through MCP can search session metadata narrowly
(`search_sessions`) but cannot:

- List/filter backlog items (`ListBacklogItems` — `status`, `priority`, `sort_by`,
  `include_terminal`, `include_archived`)
- Query notification history (`GetNotificationHistory` — `limit`, `offset`, `type_filter`,
  `session_id`, `unread_only`)
- Full-text search Claude/session history (`SearchClaudeHistory` — `project`, `model`,
  `start_time`/`end_time`, pagination, backed by the real inverted-index engine in
  `session/search/`)
- (Lower priority, may be out of scope for first pass) `SearchFiles`, `SearchGitHubRepos`

This is a wiring gap: the backend logic exists and is exercised by the web UI; only the MCP
surface is missing. `server/mcp/tools_backlog.go:204`'s own error-hint text references a
`list_backlog_items` tool that doesn't exist yet.

## Desired outcome

MCP tool coverage extends to existing search/list/filter RPCs so an LLM client can look up
notifications, backlog items, and Claude/session history the way a human can in the web UI,
without new backend capability being built.

## Acceptance criteria (from backlog item)

1. An LLM client can list/filter backlog items via MCP (by `status`, `priority`, etc.)
   without a raw RPC/curl call.
2. An LLM client can query notification history via MCP with the same filters
   `GetNotificationHistory` already supports (`type_filter`, `session_id`, `unread_only`,
   pagination).
3. An LLM client can run a full-text Claude/session-history search via MCP (wrapping
   `SearchClaudeHistory`), with results respecting an LLM-context-safe default limit,
   following `list_sessions`' existing "default limit 10" convention.
4. `server/mcp/tools_backlog.go:204`'s dangling reference to `list_backlog_items` is
   resolved (tool now exists, or hint text corrected).

## Out of scope

- New backend search capability — FTS engine, notification filters, backlog filters all
  already exist server-side.
- Deciding to expose every candidate RPC (`SearchFiles`, `SearchGitHubRepos`) in this pass —
  prioritization is a planning-phase call.
- Redesigning web UI search UX.

## Open questions carried into research/planning

- Priority order among `SearchClaudeHistory` / `GetNotificationHistory` /
  `ListBacklogItems` / `SearchFiles` / `SearchGitHubRepos`.
- Resource-scoped tools (precedent: `search_sessions`) vs. one unified cross-resource
  `search` tool.
- Context-size/truncation handling for full-text history search results.

## Suggested entry point

`/sdd:quick` — mechanical RPC→MCP-tool wraps following the `search_sessions` template
(`server/mcp/server.go:151`). A short planning pass only if a unified-tool design is favored.
