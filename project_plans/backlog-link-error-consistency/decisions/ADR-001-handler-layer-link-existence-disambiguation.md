# ADR-001: Disambiguate ITEM_NOT_FOUND vs PERMISSION_DENIED at the Handler Layer

**Status**: Accepted
**Date**: 2026-08-16
**Project**: backlog-link-error-consistency

## Context

`get_backlog_item` (`server/mcp/tools_backlog.go:127`) and the 5 mutating backlog MCP tools
(`report_progress`, `request_review`, `submit_review_verdict`, `report_pr_created`,
`submit_triage_result`) return contradictory error codes for the same underlying condition. The
mutating tools call `storage.GetItemSessionBySessionAndItem` (`session/storage_backlog.go:201`),
whose ent join predicate (`itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))`) returns
`session.ErrNotFound` both when the link row is missing *and* when the backlog item itself no
longer exists. Every mutating handler currently maps that single `ErrNotFound` unconditionally to
`PERMISSION_DENIED`, even when the item was deleted (confirmed real trigger:
`EntRepository.DeleteBacklogItem`, `session/ent_repository_backlog.go:783-840`, reachable
unguarded from `web-app/src/components/backlog/BacklogItemDetail.tsx:542-543`).

Two structurally different places could host the fix:

- **(A) Storage layer**: make `GetItemSessionBySessionAndItem` itself return a distinguishable
  error/sentinel (e.g. a new wrapped sentinel or a custom error type) so each of the 5 handlers
  only needs one more `errors.Is` branch.
- **(B) Handler layer**: add a shared helper method on `backlogHandlers`
  (`server/mcp/tools_backlog.go`) that performs the link lookup and, only on the not-found path,
  falls back to `storage.GetBacklogItem` to determine whether the item exists — encapsulating the
  full disambiguation and MCP-error-code mapping in one place the 5 handlers call into.

## Decision

We chose **(B)**: a handler-layer helper, `resolveItemLink`, added to `backlogHandlers` in
`server/mcp/tools_backlog.go`. Each of the 5 mutating handlers replaces its existing
`GetItemSessionBySessionAndItem` + single `errors.Is(linkErr, session.ErrNotFound)` block with a
call to `h.resolveItemLink(ctx, callerUUID, itemID)`, keeping any separate role-mismatch check
(`itemSession.Role != "..."`) unchanged immediately after it.

## Rationale

- **`GetItemSessionBySessionAndItem` has other callers with a different not-found contract.**
  `get_backlog_item`'s own role-lookup (`tools_backlog.go:193`) calls the same method and
  deliberately wants a single "not found → no role guidance" branch; it does not want or need to
  distinguish item-missing from link-missing (it already gets item-missing separately, via its own
  direct `GetBacklogItem` call at line 127). Pushing a new sentinel/error type into the storage
  method's contract would either force that caller to add a branch it has no use for, or produce
  a storage-layer error type whose meaning ("item missing" vs "link missing") is really an
  MCP-response-shaping concern, not a persistence concern.
- **Interface-pollution checklist**: per `.claude/rules/interface-pollution-checklist.md` item 1
  (speculative interface/generalization), there is exactly one real consumer *pattern* today — 5
  handlers doing the identical two-step disambiguation before mapping to an MCP error code. A
  handler-layer helper serves that one pattern directly; a storage-layer sentinel would be
  speculative generalization of the storage contract for a concern (`ErrItemNotFound` /
  `ErrPermissionDenied` string constants, `errResult()` envelope) that already lives entirely in
  the handler layer today (see `server/mcp/tools_backlog.go:57-58`, `tools_discovery.go:73`).
- **Matches the existing convention exactly.** research/stack.md confirmed this file's established
  pattern is "sentinel + `errors.Is`, flat if/else-if, no `errors.As` custom types." A new
  storage-layer error type would introduce a second error-handling idiom into a codebase that
  currently has exactly one; the handler-layer helper needs no new idiom, only a new function.
- **Reuses, rather than duplicates, the fallback lookup.** The helper's fallback call is the
  already-imported, already-used `storage.GetBacklogItem` (used directly in this same file at
  lines 127, 404, 673) rather than a new lean existence-check primitive — see the Pattern
  Decisions table in `plan.md` for why a new `Exists`-only storage method was also rejected
  (checklist items 1 and 5: speculative interface, unjustified new primitive for a single call
  site).

## Consequences

- **Positive**: The diff is small and localized to `server/mcp/tools_backlog.go` (one new method,
  5 call-site edits) plus its test file — no change to `session/storage_backlog.go`,
  `session/repository.go`, or any ent schema/query file.
- **Positive**: Role-mismatch checks (a separate, already-correct condition per
  research/pitfalls.md) are structurally impossible to touch by accident — they live outside
  `resolveItemLink` entirely, in each handler, unchanged.
- **Negative / accepted trade-off**: `GetItemSessionBySessionAndItem`'s ambiguous-`ErrNotFound`
  contract remains ambiguous at the storage layer. Any *future* caller of that method outside
  `server/mcp/tools_backlog.go` would have to re-implement the same disambiguation (or call
  `resolveItemLink` if it happens to be in the same package). This is accepted because no such
  caller exists today, and per the interface-pollution checklist, designing for a hypothetical
  future caller now would be the speculative-generalization smell this decision explicitly avoids.
  If a second real consumer emerges, promoting the disambiguation into the storage layer (Option A)
  becomes the right call at that point — see requirements.md's `research/stack.md` for the original
  framing of that option.
- **TOCTOU** (both queries are non-transactional): accepted as a documented race, not fixed with an
  `ent.Tx` — see the Pattern Decisions table in `plan.md` for the reasoning (SQLite
  `SetMaxOpenConns(1)` already serializes access; the race's failure mode is graceful, not
  data-corrupting).
