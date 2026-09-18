# Research: Stack (ent ORM + MCP handler conventions)

## 1. ent query APIs to distinguish "item doesn't exist" vs "link doesn't exist"

There is no single-round-trip ent API that returns both pieces of information (item
existence + link existence) atomically from one `ItemSession` query — `HasBacklogItemWith`
compiles to a JOIN/EXISTS predicate on `item_sessions`, so a miss is structurally
indistinguishable between "no row on either side" and "item row is gone."

The idiomatic composition is two cheap, sequential queries, and this pattern is already used
elsewhere in the same file (`session/storage_backlog.go`):

- **Item existence check**: `r.client.BacklogItem.Query().Where(backlogitem.ID(parsedItemID)).Exist(ctx)`
  — generated in `session/ent/backlogitem_query.go:367` (`BacklogItemQuery.Exist`). Internally
  it's `FirstID` + `IsNotFound` mapping, i.e. a `SELECT id ... LIMIT 1`, cheaper than a full
  `Get`/`First` that hydrates all fields. `ItemSessionQuery.Exist` (`session/ent/itemsession_query.go:271`)
  is the same shape if a symmetric check is ever needed on that side.
- **Link check**: the existing `GetItemSessionBySessionAndItem` query (`session/storage_backlog.go:201-223`).

Recommended fix shape (only pay the second round trip on the *error* path, not on every
call, since the link check succeeds on the happy path the overwhelming majority of the time):

```go
is, err := r.client.ItemSession.Query().
    Where(
        itemsession.SessionUUID(sessionUUID),
        itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
    ).
    Order(ent.Desc(itemsession.FieldCreatedAt)).
    First(ctx)
if err != nil {
    if ent.IsNotFound(err) {
        exists, existErr := r.client.BacklogItem.Query().
            Where(backlogitem.ID(parsedItemID)).Exist(ctx)
        if existErr == nil && !exists {
            return ItemSessionSummary{}, fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
        }
        return ItemSessionSummary{}, fmt.Errorf("%w: item session for session=%s item=%s", ErrPermissionDenied-equivalent..., sessionUUID, itemID)
    }
    ...
}
```
(Exact sentinel/wrapping strategy is a plan-phase decision — see §4. The key stack finding is
that `BacklogItemQuery.Exist` is the correct low-cost primitive, not a second full `Get`.)

An eager-load alternative (`BacklogItem.Query().Where(ID).QueryItemSessions()...`) was
considered but rejected: it still requires the item to exist as the query root, so it can't
distinguish "item missing" from "item exists, no matching session" in one shot either — same
two-case ambiguity, just phrased from the other direction. No ent API collapses this to one
round trip without giving up the useful distinction.

## 2. Schema confirmation — `session/ent/schema/`

- **`BacklogItem`** (`session/ent/schema/backlog_item.go`): PK is `field.UUID("id", ...)`. Edge
  `edge.To("item_sessions", ItemSession.Type)` (line 125) — **no `entsql.OnDelete(entsql.Cascade)`
  annotation**, unlike `status_events`/`stuck_states`/`progress_notes` on the same schema, which
  do have cascade. This is load-bearing for the root-cause investigation (Agent 2's job, not
  mine): `DeleteBacklogItem` (`session/ent_repository_backlog.go:783-836`) explicitly queries and
  deletes all `ItemSession` (and their `ReviewVerdict`) rows itself, in code, before deleting the
  `BacklogItem` row — because there's no DB-level cascade to rely on. Deleting a backlog item
  therefore always deletes its links in the same call, confirming that "item gone" and "link
  gone" are not independent events in the deletion path (they can't happen out of sync via
  `DeleteBacklogItem` alone) — though this doesn't yet rule out archival/GC paths as the actual
  cause in the reporter's incident.
- **`ItemSession`** (`session/ent/schema/item_session.go`): PK `field.UUID("id", ...)`. Edge
  `edge.From("backlog_item", BacklogItem.Type).Ref("item_sessions").Unique().Required()` (line
  76-79) — the join used by `itemsession.HasBacklogItemWith(backlogitem.ID(...))`. Also has a
  loose (non-edge) `session_uuid string` field, indexed (`index.Fields("session_uuid")`), used by
  `itemsession.SessionUUID(sessionUUID)` in the link-check predicate.
- Field/edge names to use in the fix: `backlogitem.ID(...)` (generated predicate package
  `session/ent/backlogitem`), `itemsession.HasBacklogItemWith(...)`, `itemsession.SessionUUID(...)`,
  `itemsession.FieldCreatedAt`.

## 3. Existing "does X exist" convention in this repo

**No prior instance of this two-step existence-then-permission pattern exists anywhere in
`session/` or `server/mcp/`** (grepped for "does not exist", `itemExists`, `checkExists`,
`.Exist(ctx)` call sites — none found outside the generated ent code itself). This will be a
new pattern in the codebase, not a refactor of an existing one. The closest precedent is
structural, not behavioral: `requestReview` (`server/mcp/tools_backlog.go:373-407`) already
calls `GetItemSessionBySessionAndItem` for the link check *and separately* calls
`GetBacklogItem` a few lines later (line 404) for an unrelated reason (reading `SkipReviewGate`)
— proving a second round trip to `GetBacklogItem`/an item-exists check in the same handler is
already an accepted cost in this file, not a new anti-pattern being introduced.

All 5 mutating handlers (`reportProgress` line 301, `requestReview` line 373,
`submitReviewVerdict` line 502, `reportPRCreated` line 662, `submitTriageResult` line 755) use
the textually-identical block:
```go
_, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
if linkErr != nil {
    if errors.Is(linkErr, session.ErrNotFound) {
        return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "..."), nil
    }
    return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
}
```
A fix at the `storage.GetItemSessionBySessionAndItem` layer (returning a distinguishable error,
e.g. a new `session.ErrBacklogItemNotFound` sentinel vs. the existing generic `session.ErrNotFound`
used for "link missing") would let all 5 call sites share one small `errors.Is` branch addition
each, rather than duplicating the two-query logic 5 times in `server/mcp/tools_backlog.go`. This
keeps the existence check in one place (`session/storage_backlog.go`) — consistent with how
`GetBacklogItem`'s own `ITEM_NOT_FOUND` mapping already lives entirely in
`server/mcp/tools_backlog.go:127-133` off a single `session.ErrNotFound` from the storage layer.

## 4. Error-wrapping conventions in this file

`session/storage_backlog.go` and `server/mcp/tools_backlog.go` already have a consistent,
established convention — the fix should match it exactly, not introduce a new style:

- **Storage layer** (`session/storage_backlog.go`): sentinel-wrap with `fmt.Errorf("%w: <context>", ErrNotFound, ...)`,
  e.g. line 216: `fmt.Errorf("%w: item session for session=%s item=%s", ErrNotFound, sessionUUID, itemID)`.
  `ErrNotFound` is declared once in `session/repository.go:15` (`var ErrNotFound = errors.New("not found")`)
  and reused as the single sentinel across all "missing row" cases in this package — a new
  `ErrBacklogItemNotFound` sentinel (if the plan phase picks that approach) should follow the
  identical declaration style in the same file/area.
- **Non-not-found storage errors**: plain `%w` wrap with a human message prefix, e.g. line 218:
  `fmt.Errorf("failed to get item session: %w", err)` — no sentinel, just wrapped for
  `errors.Is`/`errors.As`-compatible unwrapping up the stack.
- **Handler layer** (`server/mcp/tools_backlog.go`): every handler unwraps with `errors.Is(err, session.ErrNotFound)`
  (e.g. lines 129, 303, 375) and maps to one of the package-level string error codes
  (`ErrItemNotFound`, `ErrPermissionDenied`, `ErrInternalError`, `ErrInvalidArgument` — declared
  around line 57-58) via the shared `errResult(code, message, remediation string) *mcpgo.CallToolResult`
  helper (`server/mcp/tools_discovery.go:73`). The fix's new branch (distinguishing item-missing
  from link-missing) should add one more `errors.Is` check per handler, in the same
  if/else-if chain shape already used, rather than a type switch or a new helper abstraction —
  consistent with this file's existing flat style.
- No `errors.As` custom error types are used anywhere in either file — everything is sentinel
  + `errors.Is`, so the fix should not introduce a custom error struct.

## Summary for the plan phase

The fix is small and fits existing idioms: add a distinguishing existence check in
`GetItemSessionBySessionAndItem` (paid only on the error path via `BacklogItemQuery.Exist`),
surface it as a new sentinel error (or a typed wrapper distinguishable via `errors.Is`), and
add one more `errors.Is` branch to each of the 5 mutating handlers plus one shared remediation
string builder for AC2's "include session UUID + item id" requirement — all following patterns
already present in `session/storage_backlog.go` and `server/mcp/tools_backlog.go` today.
