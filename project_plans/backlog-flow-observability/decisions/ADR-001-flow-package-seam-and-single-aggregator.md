# ADR-001: Isolate flow analytics behind a `session/flow` seam with one aggregator and two thin transports

**Status**: Accepted (plan phase) **Date**: 2026-11-05

## Context
The integration points are hotspots: `server/services/backlog_service_triage.go` (4018 lines, 29 commits/90d), `server/mcp/tools_backlog.go` (3255 lines), `proto/session/v1/backlog.proto` (1806 lines), and the `BacklogService` god object (`research/architecture.md` section 1). Two consumers (web UI, Claude via MCP) must see identical numbers.

## Options
1. Extend `BacklogService` plus `backlog.proto` plus `tools_backlog.go` in place. Strength: least new wiring. Weakness: adds churn to the three worst files and cannot be unit-tested without SQLite.
2. Refactor-first (extract `session/backlog`). Strength: clean base. Weakness: unrelated multi-PR project that blocks the feature.
3. **Isolate via seam (chosen).** New pure package `session/flow` (`Aggregator`, four role interfaces `EventSource`/`VerdictSource`/`StuckSource`/`ItemSource`, plain structs, no ent import; the import boundary is enforced by a `depguard` strict allow-list `isolate_flow_core`, not by prose); SQL reader in `session/ent_repository_flow.go`; own `proto/session/v1/backlog_flow.proto` with `BacklogFlowService`; `server/mcp/tools_backlog_flow.go`. Both transports reach the one `Aggregator` through `BacklogFlowService`: the RPC handler directly, and the MCP tools in-process (`flowSvc.GetFlowSummary(ctx, connect.NewRequest(...))`, marshalled with `protojson`), the way `server/mcp/tools_backlog.go:1076` calls `backlogSvc.ListBacklogItems`. There is one `FlowSummary` mapper, so parity holds by construction.

## Decision
Option 3. `BacklogService`, `backlog.proto` and `tools_backlog.go` are not edited. The only edits to existing hot files are single wiring lines (`server/server.go`, `server/dependencies.go`, `server/mcp/server.go`) and the `NewCore` parameter.

## Consequences
- RPC and MCP parity is enforceable by one test that compares JSON for the same `FlowQuery`.
- **Wiring (revised after architecture review).** No positional parameter is added to `NewCore` (10 params on `origin/main`). A variadic `CoreOption` (`WithFlowService(*services.BacklogFlowService)`) is appended to `NewCore`, `NewHTTPHandler` (`server/mcp/server.go:120`) and `RunServer` (`:156`, the stdio entry point), which forward `opts` to `NewCore`; the 7 existing test call sites are untouched. Absent option returns `ErrUnavailable`, matching `link_session_to_item`.
- **Construction site**: `server/dependencies.go` builds one `session.NewEntFlowReader(storage.GetEntClient())`, one `flow.Aggregator` and one `BacklogFlowService`; `server/server.go` registers the handler (next to Insights, `:600`) and passes `servermcp.WithFlowService(deps.BacklogFlowService)` at the `NewHTTPHandler` call (`:1178`), so the RPC and HTTP-MCP paths share one cache.
- **Stdio fallback decision**: `main.go:172` passes `WithFlowService(services.NewFlowServiceFromStorage(storage))`, which builds a private aggregator from `storage` (the only dependency that path has). It has its own cache per process; `as_of` makes that visible. With `storage == nil` the flow tools are not registered. Tested in Story 3.2.3.
- **Hotspot dispositions** (`session/ent_repository_backlog.go`, `server/dependencies.go`, `server/server.go`, `session/storage.go`) are in the plan's Tech Debt Disposition table: minimal one-line seams, episode logic in a new file, `storage.go` unedited.
- New MCP tool names use the `flow` infix and do not collide with `get_backlog_stuck_report` (verified on `origin/main`: `server/mcp/tools_stuck_report.go`, registered via `registerStuckReportTools` at `server/mcp/server.go:93`).
