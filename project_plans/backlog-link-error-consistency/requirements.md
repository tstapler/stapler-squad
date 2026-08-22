# Requirements: backlog-link-error-consistency

## Source

Backlog item `e7b47f03-9581-45df-81bd-8693a548e3d1`: "Backlog MCP tools: session lost item
linkage — ITEM_NOT_FOUND vs PERMISSION_DENIED inconsistency for the same item id."

## Problem

A work session lost its backlog-item linkage mid-session. `get_backlog_item` returned
`ITEM_NOT_FOUND` for item `b608ab1e-b86e-4130-8879-7328cd363063`, while every mutating tool
(`report_progress`, `request_review`, and — by the same code path — `submit_review_verdict`,
`report_pr_created`, `submit_triage_result`) returned `PERMISSION_DENIED: this session is not
linked to the specified backlog item` for the *same* item id. The two errors imply
contradictory backend states (item doesn't exist vs. item exists but this session isn't
authorized), so the session couldn't self-diagnose or recover, and had no working channel
(`report_blocked` is itself gated by the same link check) to report the problem.

## Root cause (confirmed by reading code, not yet root-caused end-to-end — first research task)

- `get_backlog_item` (`server/mcp/tools_backlog.go:127`) calls `storage.GetBacklogItem(ctx,
  itemID)` directly — a straight lookup by item ID — and returns `ITEM_NOT_FOUND` only if the
  item row itself doesn't exist.
- Every mutating tool instead calls `storage.GetItemSessionBySessionAndItem(ctx, callerUUID,
  itemID)` (`session/storage_backlog.go:201`), which queries the `ItemSession` join table with
  `itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))`. This join predicate returns
  `ErrNotFound` both when the session→item link row is missing **and** when the backlog item
  itself no longer exists (the edge has nothing to join to) — the query cannot distinguish the
  two cases. Every mutating handler maps that single `ErrNotFound` to `PERMISSION_DENIED`
  unconditionally, without ever checking whether the item itself exists.
- Net effect: if the backlog item was deleted/archived out from under an in-flight session (or
  never had a link created), `get_backlog_item` correctly reports `ITEM_NOT_FOUND`, while every
  mutating tool reports the misleading `PERMISSION_DENIED` for the exact same underlying cause.
  This is a **plausible, code-confirmed hypothesis** — research must confirm what actually
  removed the item/link in the reporter's case (deletion, archival, GC, or a genuine link that
  was simply never created) and why it persisted after "reconnection."
- A secondary, distinct symptom in the same report: `list_sessions`, `get_session`, and
  `get_session_goal` all timed out repeatedly for the same session, which blocked
  self-diagnosis independent of the error-code issue above. Research should determine whether
  this shares a root cause (e.g. the same session/item rows in a bad state stalling those
  lookups) or is an unrelated reliability issue.

## Acceptance Criteria

1. `get_backlog_item` and every mutating backlog MCP tool (`report_progress`,
   `request_review`, `submit_review_verdict`, `report_pr_created`, `submit_triage_result`)
   return the *same* error code for the *same* underlying condition on a given item id: if the
   backlog item itself does not exist, all tools return `ITEM_NOT_FOUND`; `PERMISSION_DENIED`
   is returned only when the item exists but this session genuinely has no link to it.
2. The `PERMISSION_DENIED` error message/remediation for a genuinely-unlinked-but-existing item
   includes enough detail for a session to self-report the problem to an operator without
   manual reconstruction (e.g. the session UUID it resolved and the item id it checked).
3. A regression test exists covering: (a) item exists, no link → `PERMISSION_DENIED`; (b) item
   does not exist → `ITEM_NOT_FOUND` from both `get_backlog_item` and at least one mutating
   tool; (c) existing "item exists, link exists" happy path is unaffected.
4. Root cause of the `list_sessions` / `get_session` / `get_session_goal` timeouts observed in
   the same session is investigated and documented (even if the fix is out of scope for this
   item, the finding must be written up with a recommendation: fix now, or file a follow-up
   item).
5. No behavior change to the happy path (linked session, existing item) for any of the affected
   tools — verified by existing test suite (`make test`) passing unmodified plus the new
   regression tests from AC3.

## Non-goals

- Rebuilding or auto-recovering a dropped session→item link (that's a distinct reconciliation
  feature, tracked separately if surfaced — see `backlog-event-driven-updates` and
  `backlog-stuck-item-visibility` in `project_plans/` for related prior work).
- Redesigning the MCP error envelope/schema wholesale — only the specific inconsistency and
  actionability of these two codes for this class of tools.
