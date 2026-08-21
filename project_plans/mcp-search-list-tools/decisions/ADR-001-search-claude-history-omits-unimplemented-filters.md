# ADR-001: `search_claude_history` MCP tool omits `project`/`model`/`start_time`/`end_time`

**Status**: Accepted
**Date**: 2026-08-13
**Context**: `project_plans/mcp-search-list-tools`

## Context

`requirements.md`'s "Open questions" section describes `SearchClaudeHistory` as supporting
"`project`, `model`, `start_time`/`end_time` filters and pagination," implying the new MCP tool
should expose all of them.

`research/features.md` §2 (`SearchClaudeHistory` section) reports reading the full handler body
at `server/services/search_service.go:459-591` and confirms: `project`, `model`, `start_time`, and
`end_time` are declared on `SearchClaudeHistoryRequest`'s wire format but **never read** anywhere
in the handler. `session/search/engine.go`'s `SearchOptions` struct (lines 22-29) only has
`Limit`/`Offset`/`SessionID` fields — no project/model/date filtering exists in the underlying
search engine at all. (A different RPC, `GetClaudeHistoryMessages`, does use `Project` — that is a
separate code path, not `SearchClaudeHistory`.)

## Decision

The `search_claude_history` MCP tool schema exposes only `query`, `limit`, and `offset`. It does
**not** expose `project`, `model`, `start_time`, or `end_time` as tool arguments.

## Consequences

- A caller cannot filter Claude-history search by project/model/date range through this tool today
  — the same limitation the underlying RPC already has.
- The tool surface is honest: no argument silently does nothing. The rejected alternative
  (exposing all four fields as passthrough arguments, matching the RPC's wire shape) would look
  like a working filter but silently return unfiltered results, which is worse than not offering
  the filter — a caller who thinks they filtered by project has no signal that they didn't.
- If project/model/date filtering is later implemented in `SearchClaudeHistory` or
  `session/search/engine.go`'s `SearchOptions`, this MCP tool's schema should be extended to match
  at that time — this decision is scoped to "wrap the RPC as it exists today," not a permanent
  design constraint.
- This is explicitly **not** new backend search capability (out of scope per `requirements.md`) —
  implementing the filters server-side was considered and rejected as out of scope for this pass,
  not silently deferred.

## Alternatives Considered

1. **Expose all 4 fields as passthrough arguments.** Rejected — creates a misleading "looks
   filtered, isn't" tool surface (see Context).
2. **Expose the fields but implement client-side post-filtering in the MCP handler** (over-fetch
   from the engine, filter on `entry.Model`/`entry.Project`/`entry.CreatedAt` already available per
   result, per `search_service.go:566-570`). Rejected for this pass — technically stays within "no
   new backend RPC capability" scope (the filtering happens after the existing RPC returns), but it
   changes `total_matches`/`has_more` semantics subtly (a truncated post-filtered page no longer
   matches the RPC's own accounting) and adds real complexity for a Complexity-2 feature. Worth
   reconsidering as a follow-up if project/model/date filtering turns out to be commonly needed by
   MCP callers.
