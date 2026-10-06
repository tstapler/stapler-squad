# Architecture Review: mcp-search-list-tools
**Date**: 2026-08-13
**Verdict**: CONCERNS

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repo — no constitution check applies.

## Constitution Violations
N/A — no constitution file present.

## Blockers
None.

The three specifically-flagged risk areas were re-verified against live source and are all
correctly handled in the current plan revision:

- **Pagination on `list_backlog_items`**: `ListBacklogItemsRequest` (proto/session/v1/backlog.proto:336-350)
  has no `limit`/`offset` fields at the wire level — confirmed by reading the proto directly.
  Task 1.1.1d's schema now includes `mcpgo.WithNumber("offset", mcpgo.DefaultNumber(0), mcpgo.Min(0))`
  alongside `limit`, and Task 1.1.1c's handler explicitly "slice[s] the response by `limit`/`offset`,
  compute[s] `TotalCount`/`HasMore`" after calling `ListBacklogItems`. This matches the Pattern
  Decisions table's stated rationale (no wire-level offset to pass through, so MCP-layer
  post-fetch slicing is correct, and `BacklogItemFilter`'s backing query is DB-capped at 1000 per
  `defaultSafetyLimit`, so the full-fetch-then-slice approach is bounded). Fixed from any earlier
  limit-only version.

- **`validateBacklogPriority` wiring**: Task 1.1.1c's handler description reads "parse
  `status`/`priority`/.../ from `req.GetArguments()`; validate via **the Task 1.1.1a helpers**"
  (plural) — Task 1.1.1a introduces both `validateBacklogStatus` and `validateBacklogPriority`.
  Story 1.1.2's test list also includes `TestListBacklogItems_ValidatesPriorityRange` (Task 1.1.2c)
  as a required test, confirming both validators are intended to be exercised from the handler, not
  just declared. (Minor: Story 1.1.1's formal Given/When/Then acceptance criteria only cover the
  status-typo case, not the priority-range case — see Nitpicks.)

- **`SessionService` facade methods actually exist**: verified `SessionService.SearchClaudeHistory`
  (`server/services/session_service.go:3039`) and `SessionService.GetNotificationHistory`
  (`server/services/session_service.go:3280`) are real forwarding methods on `*SessionService`
  (delegating to the embedded `searchSvc`/`notificationSvc` sub-services), so `historyHandlers`'s
  and `notificationHandlers`'s planned `svc *services.SessionService` field genuinely has the
  methods the plan calls — matches the existing `workflowHandlers.svc` convention exactly.

## Concerns
- [ ] **Tasks 1.1.1b / 1.2.1b / 1.3.1b (Domain Glossary, result-type naming)** — `SearchClaudeHistoryResult.TotalMatches`
  is still inconsistently named vs. `ListBacklogItemsResult.TotalCount` and
  `GetNotificationHistoryResult.TotalCount` — same "how many things matched before pagination"
  concept, three tools, two different JSON field names. This partially inherits the backend's own
  wire-naming split (`SearchClaudeHistoryResponse.total_matches` vs.
  `GetNotificationHistoryResponse.total_count`, proto/session/v1/session.proto:991 vs. :1377), but
  `ListBacklogItemsResponse` has **no** wire-level total-count field at all (proto/session/v1/backlog.proto:353-355
  only returns `items`) — its `TotalCount` is synthesized entirely at the MCP layer, so the plan had
  a free choice there and picked `TotalCount`, matching the existing `ListSessionsResult.TotalCount`
  MCP-layer precedent (`server/mcp/types.go:43`). That leaves `search_claude_history` as the one
  outlier for no MCP-layer reason, only an inherited wire-naming accident. Since this is a
  client-facing JSON contract being introduced for the first time (cheapest point to fix), recommend
  renaming `SearchClaudeHistoryResult.TotalMatches` → `TotalCount` for consistency across all three
  new envelopes and with the existing `list_sessions` precedent — or, if the divergence is kept
  deliberately (e.g. to mirror the RPC's own field name), add a one-line Domain Glossary note saying
  so, so a future reader sees a decision rather than an oversight.

## Nitpicks
- **ADR-002's Context section** describes `buildCostLookup` as one of "two unexported package-level
  functions in `server/services`" alongside `backlogItemSummaryToProto`. Verified:
  `backlogItemSummaryToProto` (`server/services/backlog_service.go:590`) is indeed an unexported
  package-level function, but `buildCostLookup` (`server/services/backlog_service.go:431`) is
  actually an unexported **method** on `*BacklogService` (called as `s.buildCostLookup()`). Doesn't
  change the ADR's conclusion (an MCP handler still can't call either without exporting them or
  duplicating logic), but the citation should say "an unexported method and an unexported
  package-level function" for precision.
- **Task 1.1.1a's prose** lists the status constant as `PrPending` (lowercase r); the real Go
  constant is `BacklogStatusPRPending` (capital R, `session/backlog.go:23`, re-exported from
  `session/domain/backlog.go`). Harmless as descriptive plan text, but worth fixing before an
  implementer copy-pastes the wrong casing into an identifier.
- **Validation-helper shape asymmetry** (Domain Glossary): `validateBacklogStatus`/`validateBacklogPriority`
  are validate-only (`(...) error`, pass the original primitive through unchanged), while
  `parseNotificationTypeFilter` is parse-into-a-type (`(sessionv1.NotificationType, error)`). The
  Domain Glossary calls the notification helper "a proto-enum-backed sibling of
  `validateBacklogStatus` — same trap, different mechanism," which is accurate but slightly
  undersells the structural difference. Not a bug — the task-level signatures are already spelled
  out correctly in 1.1.1a vs. 1.2.1b, and the asymmetry is justified (`BacklogStatus` is genuinely
  wire-typed as a raw `string` on `ListBacklogItemsRequest`, so there's nothing to parse into; the
  proto's own `NotificationType` is a real enum, so parsing is the correct move there — see
  `type-driven-design`'s "parse, don't validate" only applies where a proven type is available
  downstream). Worth a one-line Domain Glossary clarification so a future contributor doesn't try to
  force one shape onto the other.
- **Story 1.1.1's acceptance criteria** cover the `status` typo-validation case (AC2) but have no
  matching Given/When/Then for the `priority` out-of-range case, even though Task 1.1.2c
  (`TestListBacklogItems_ValidatesPriorityRange`) is planned and required. Low severity — the test
  task exists and will catch a regression either way — but the story's AC section is the part a
  reviewer reads first, and it under-documents behavior the plan otherwise clearly intends to ship.
