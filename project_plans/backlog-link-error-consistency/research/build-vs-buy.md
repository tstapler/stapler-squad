# Build vs. Buy: backlog-link-error-consistency

## 1. Existing OSS library/framework (ent idiom)

Checked `session/ent/itemsession_query.go` and `session/ent/backlogitem_query.go`.

- `ItemSessionQuery.QueryBacklogItem()` (`session/ent/itemsession_query.go:69`) exists and chains
  onto the `backlog_item` edge, but it starts *from* an already-resolved `ItemSession` — no help
  when the join itself is what's failing (that's the exact case we're in).
- `BacklogItemQuery` exposes `Only`, `OnlyID`, `Exist`/`ExistX` (`session/ent/backlogitem_query.go:256,367`)
  — standard ent generated helpers, nothing repo-specific.
- No ent-contrib or generated-code helper distinguishes "edge predicate matched nothing because
  the parent doesn't exist" from "...because the edge itself doesn't exist" in a single call.
  `itemsession.HasBacklogItemWith(backlogitem.ID(...))` (`session/ent/itemsession/where.go:1171`)
  compiles to a single SQL `EXISTS` subquery joined against the predicate — by construction it
  can't tell you *why* zero rows came back, only that zero rows came back. This is inherent to
  how ent (and SQL `EXISTS`/join semantics generally) compiles edge predicates, not a gap specific
  to this codebase.
- The correct ent idiom for "distinguish missing-parent from missing-edge" is exactly two
  sequential queries: `BacklogItem.Query().Where(ID(...)).Exist(ctx)` to check the item, then the
  existing `HasBacklogItemWith` link query only if the item exists. This is standard practice, not
  a workaround — ent's docs/generated code offer no single-query alternative for this shape of
  check.

**Verdict: Not recommended (no single-query ent shortcut exists) / the two-query approach IS the
correct idiom — see §4.**

## 2. SaaS/managed API

N/A — this is internal Go server logic (an MCP tool handler + ent-backed storage layer with no
external dependency). No SaaS/managed API applies.

## 3. LLM-generated implementation vs. battle-tested library

`errResult` (`server/mcp/tools_discovery.go:73`) is a small local helper that builds an
`mcpgo.CallToolResult` from a `(code, message, remediation)` tuple. The MCP tool error codes
(`ErrPermissionDenied`, `ErrItemNotFound`, `ErrInvalidArgument`, `ErrInternalError`, defined at
`server/mcp/tools_backlog.go:57-58` and neighbors) are plain string constants — there is no RFC
7807 problem+json library or generic error-handling framework in play anywhere in `server/mcp/`.
Every one of the ~15+ call sites in `tools_backlog.go` and `tools_goal.go` hand-maps a specific Go
error condition to one of these string codes inline (e.g. `errors.Is(err, session.ErrNotFound)` →
`errResult(ErrItemNotFound, ...)`).

The fix is: in each mutating handler, when `GetItemSessionBySessionAndItem` returns
`session.ErrNotFound`, do one more existence check (`storage.GetBacklogItem` — already imported
and used elsewhere in the same file) before deciding between `ErrItemNotFound` and
`ErrPermissionDenied`. That's an ~8-15 line change per call site (5 mutating tools), well within
"small deterministic fix" territory. No third-party error/problem-details library would reduce
this — the repo's existing string-constant + `errResult` convention is intentionally minimal, and
introducing RFC 7807 tooling here would be a scope increase this ticket doesn't call for
(non-goal: "Redesigning the MCP error envelope/schema wholesale").

**Verdict: Recommended (custom fix, no library).**

## 4. Fork or adapt an existing in-repo helper

Two candidates checked:

- **`report_pr_created`** (`server/mcp/tools_backlog.go:660-679`) already calls *both*
  `GetItemSessionBySessionAndItem` (link check) and `GetBacklogItem` (existence check) — but in
  the wrong order for our purposes: it checks the link first and returns `PERMISSION_DENIED`
  immediately on any `ErrNotFound` from the join query, then only calls `GetBacklogItem`
  afterward, for an unrelated idempotency check (comparing `item.Status`/`PrNumber`). Because the
  link check runs first and unconditionally maps `ErrNotFound` to `PERMISSION_DENIED`, this
  handler has the *same* bug as the other four — it just happens to also contain a `GetBacklogItem`
  call that could be reordered/reused. This is the closest "near-identical" in-repo code, and it's
  useful as a template for correct ordering (existence check before/on link-check failure), but it
  is not currently correct and can't be adapted as-is without also fixing it.
- **`server/services/file_service.go:150-157`** distinguishes `CodeNotFound` vs
  `CodePermissionDenied` via `os.IsNotExist` / `os.IsPermission` — a real precedent for "same code
  shape, different error source" in this repo, but it operates on filesystem errno semantics, not
  an ent join query, so there's no code to lift, only the general pattern (check specific failure
  condition, map deliberately) to follow.

No generalized "resource+auth check" helper exists anywhere in `server/services/*.go` to
generalize; every ConnectRPC handler that returns both `CodeNotFound` and `CodePermissionDenied`
does its own inline mapping, matching the same one-off-per-call-site style already used in
`tools_backlog.go`.

**Verdict: Viable as a template, not as a drop-in fork.** Fix all 5 mutating handlers with the
same shape: on `ErrNotFound` from the link query, call `GetBacklogItem` to disambiguate, return
`ITEM_NOT_FOUND` if the item is gone and `PERMISSION_DENIED` (with the enriched
session-uuid/item-id detail required by AC2) only if it still exists.

## Summary

This is a build, as expected — a small, localized Go fix using existing ent query methods
(`BacklogItem.Query().Where(backlogitem.ID(...)).Exist(ctx)` or `GetBacklogItem`, already used
elsewhere in the file) sequenced correctly around the existing `GetItemSessionBySessionAndItem`
link check. No OSS library, ent-contrib helper, SaaS API, or in-repo generalized helper applies or
would reduce the amount of code needed. The `report_pr_created` handler is the best in-repo
reference for the two-query shape but needs the same reordering fix as the other four handlers,
not just reuse.
