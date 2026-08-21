# ADR-002: `list_backlog_items` requires a non-nil `backlogSvc`, no degraded fallback

**Status**: Accepted
**Date**: 2026-08-13
**Context**: `project_plans/mcp-search-list-tools`

## Context

*(Corrected 2026-08-13 after adversarial review found the original justification below didn't match
the response shape this plan actually chose — `BacklogItemSummaryResult` (Task 1.1.1b) has no cost
field, so cost-annotated conversion isn't actually why `listBacklogItems` needs `BacklogService`.
The real reason, verified by reading `BacklogService.ListBacklogItems`'s full body
(`server/services/backlog_service_query.go:107-142`), is below.)*

`BacklogService.ListBacklogItems` does two things before returning data: (1) builds a
`session.BacklogItemFilter{SortBy, ExcludeDone, ExcludeArchived, Statuses, Priorities}` from the
request, including a non-trivial business rule — an explicit `status` filter overrides the default
`ExcludeDone`/`ExcludeArchived` exclusion (`backlog_service_query.go:114-122`, "explicit status
filter overrides default exclusion") — and (2) calls the unexported `buildCostLookup`/
`backlogItemSummaryToProto` to attach cost data to each item's proto representation. Only (1) is
relevant to `listBacklogItems`'s actual output: cost data is computed by the RPC but simply unused
by the trimmed `BacklogItemSummaryResult` mapping (a minor wasted-computation cost, tracked
separately, not a functional blocker). The filter-translation business rule in (1), however, has no
equivalent in `session.Storage.ListBacklogItemSummaries`, which takes an already-built
`BacklogItemFilter` and does no interpretation of it — calling `BacklogService.ListBacklogItems`
directly reuses that rule instead of re-deriving it in `server/mcp`, which is the actual reason
Pattern A is chosen over Pattern B here.

`backlogHandlers` (`server/mcp/tools_backlog.go:138`) already carries an optional/nilable
`backlogSvc *services.BacklogService` field, used today for `create_backlog_item`'s post-create
auto-triage trigger. Per `server/mcp/server.go:96-103`, the stdio fallback path
(`buildMCPDeps` in `main.go`, a reduced Phase-1-only dependency set) constructs `NewCore`/`RunServer`
with `backlogSvc: nil`. Existing nil-`backlogSvc` call sites degrade gracefully today (e.g.
`create_backlog_item` just skips the auto-triage trigger silently — the item is still created).

`list_backlog_items` has no equivalent "still mostly works" degraded mode: without `backlogSvc` it
cannot get cost-annotated results, or any results, from `BacklogService.ListBacklogItems`.

## Decision

`listBacklogItems` requires `h.backlogSvc != nil`. If it is nil, the handler returns
`errResult(ErrInternalError, "backlog service unavailable on this server configuration", "")`
rather than falling back to a direct, cost-less `storage.ListBacklogItemSummaries` call.

## Consequences

- On the Phase-1 stdio path (where `backlogSvc` is nil), `list_backlog_items` is unavailable and
  returns an internal error rather than a degraded (cost-less) result.
- This matches the existing pattern elsewhere in `tools_backlog.go`, where some capability quietly
  narrows on the Phase-1 path rather than growing a second, less-capable code path for a field the
  rest of the file already treats as service-owned.
- No new "cost-less proto conversion" code path is introduced in `server/mcp`, avoiding a second,
  divergent implementation of `backlogItemSummaryToProto`'s logic that would need to be kept in
  sync with the real one in `server/services`.
- If the Phase-1 stdio path is later found to need `list_backlog_items`, revisit by either (a)
  exporting `buildCostLookup`/`backlogItemSummaryToProto` for direct reuse, or (b) wiring
  `backlogSvc` into the Phase-1 dependency set instead of leaving it nil — not by re-deriving the
  conversion logic in `server/mcp`.

## Alternatives Considered

1. **Fall back to `h.storage.ListBacklogItemSummaries` + a locally-built `BacklogItemFilter` when
   `backlogSvc` is nil.** Rejected — would require re-deriving `BacklogService.ListBacklogItems`'s
   status-override-exclusion business rule (`ExcludeDone`/`ExcludeArchived` semantics, see Context)
   a second time in `server/mcp`, which is exactly the kind of duplicate-logic drift
   `.claude/rules/primitive-obsession-checklist.md` and this repo's own `RepoRef` duplication
   incident warn against, applied here to a filter-construction rule instead of a type. Cost
   annotation is not part of this rejection's rationale — `BacklogItemSummaryResult` doesn't use it
   either way.
