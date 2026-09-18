# Architecture Research: backlog-link-error-consistency

## 1. Uniformity of the link-check error mapping across call sites

Verified exact line numbers (server/mcp/tools_backlog.go):

| Tool | Link-check call | Role check after? |
|---|---|---|
| `get_backlog_item` | `tools_backlog.go:193` (best-effort, `linkErr` swallowed — only used to pick role-guidance text, never surfaced as an error) | n/a |
| `report_progress` | `tools_backlog.go:301` | none |
| `request_review` | `tools_backlog.go:373` | none (role is implicitly "work" by convention, not checked) |
| `submit_review_verdict` | `tools_backlog.go:502` | yes, `itemSession.Role != "review"` at :509 |
| `report_pr_created` | `tools_backlog.go:662` | yes, `itemSession.Role != session.SessionRoleWork` at :669 |
| `submit_triage_result` | `tools_backlog.go:755` | yes, `itemSession.Role != "triage"` at :762 |

The 5 mutating call sites (301/373/502/662/755) share **byte-for-byte identical** error-mapping shape:

```go
itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
if linkErr != nil {
    if errors.Is(linkErr, session.ErrNotFound) {
        return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "..."), nil
    }
    return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
}
```

Only the remediation string (3rd `errResult` arg) varies (`report_progress` has one, the other four pass `""`). The subsequent role check (where present) is a clean, independent `if itemSession.Role != "..."` guard *after* the link check succeeds — it never touches `linkErr` handling and doesn't complicate factoring out the link check itself.

**A single shared helper is straightforward, not just feasible.** Per-tool nuance (role checks) sits entirely downstream of the link check and needs no special-casing inside the helper — the helper only needs to return the resolved `ItemSessionSummary` (or a ready-to-return error result) and let each caller layer its own role check on top, exactly as today.

Precedent for combining two lookups already exists in this exact file: `get_backlog_item` (tools_backlog.go:127 `GetBacklogItem` + :193 `GetItemSessionBySessionAndItem`) and `request_review` (tools_backlog.go:373 link check + :404 a second `GetBacklogItem` call for the `SkipReviewGate` check) both already issue two sequential, non-transactional storage calls in one handler. The fix doesn't introduce a new pattern to this codebase, it extends one already in use.

## 2. Storage-layer seam: which architectural option

Read `session/storage_backlog.go:201` (`EntRepository.GetItemSessionBySessionAndItem`), `session/storage.go:1000` (`Storage.GetItemSessionBySessionAndItem`), `session/storage.go:706` (`Storage.GetBacklogItem`), `session/ent_repository_backlog.go:310` (`EntRepository.GetBacklogItem`).

Key facts:
- `GetItemSessionBySessionAndItem` queries `ItemSession` filtered by `itemsession.SessionUUID(sessionUUID)` AND `itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))` (storage_backlog.go:207-213) — a single join predicate. Ent's `ent.IsNotFound` fires identically whether the *join* finds no row because the link is missing or because the referenced `backlogitem` doesn't exist; the join can't be selectively told to distinguish those (this is the root cause, confirmed).
- `GetBacklogItem` (ent_repository_backlog.go:310) does a **direct, unrelated** point lookup — `r.client.BacklogItem.Query().Where(backlogitem.ID(parsedID)).Only(ctx)` — no join, no dependency on `ItemSession` at all.
- Both wrap `ent.IsNotFound(err)` into the **same sentinel**, `session.ErrNotFound` (storage_backlog.go:216, ent_repository_backlog.go:328), so `errors.Is(err, session.ErrNotFound)` behaves consistently across both.
- `Storage.GetItemSessionBySessionAndItem` (storage.go:1000) type-asserts `s.repo.(*EntRepository)` and returns `ErrNotFound` directly if the backend isn't `EntRepository` — i.e. `h.storage` is a concrete `*session.Storage` (tools_backlog.go:92 `storage *session.Storage`), not an interface. There is no interface to route around; a plain function/method is idiomatic here per `.claude/rules/interface-pollution-checklist.md` (no need to define a `BacklogLinkChecker` interface for a single concrete storage type with one call site pattern).

**Recommendation: option (c) — a small shared helper in `server/mcp/tools_backlog.go`,** not a new combined storage-layer method (option a) and not just inlining the sequencing per-tool (option b, which would duplicate the same 6-line block 5 times — the antithesis of DRY with no offsetting benefit).

Why not (a), a new combined storage method: `GetItemSessionBySessionAndItem`'s join and `GetBacklogItem`'s point lookup are different queries against different root entities (`ItemSession` vs `BacklogItem`) with no shared WHERE-clause fragment to fuse; a "give me both facts in one call" method would just be these two queries glued together inside the storage layer instead of the handler layer — moving the sequencing without simplifying it, and adding a new storage-layer method whose only consumer is this one handler pattern (a speculative addition to the `Storage`/`EntRepository` surface, the sixth interface-pollution smell in `.claude/rules/interface-pollution-checklist.md` — a two-call sequence with no new query logic doesn't earn a new layer).

Why not (b), inlining per call site: identical 6-line block × 5 call sites is exactly what a helper exists to avoid, and would leave 5 places to keep in sync if the error message/remediation text changes again.

Proposed shape (illustrative, not exhaustive — plan phase should finalize):

```go
// resolveItemLink verifies the caller's session is linked to itemID, returning the
// ItemSession on success. On failure it returns a ready-to-return error result:
// ITEM_NOT_FOUND if the backlog item itself doesn't exist, PERMISSION_DENIED if the
// item exists but this session has no link to it.
func (h *backlogHandlers) resolveItemLink(ctx context.Context, callerUUID, itemID string) (session.ItemSessionSummary, *mcpgo.CallToolResult) {
    itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
    if linkErr == nil {
        return itemSession, nil
    }
    if !errors.Is(linkErr, session.ErrNotFound) {
        return session.ItemSessionSummary{}, errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), "")
    }
    // Link row missing — disambiguate: does the item itself exist?
    if _, itemErr := h.storage.GetBacklogItem(ctx, itemID); errors.Is(itemErr, session.ErrNotFound) {
        return session.ItemSessionSummary{}, errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), "")
    }
    // Item exists, no link for this session — genuine permission denial (AC2: include
    // enough detail to self-report without manual reconstruction).
    return session.ItemSessionSummary{}, errResult(ErrPermissionDenied,
        fmt.Sprintf("session %s is not linked to backlog item %s", callerUUID, itemID),
        "This session has no ItemSession row for this item; if you believe it should, report_blocked or escalate to a human — this tool cannot self-recover the link.")
}
```

Each of the 5 call sites replaces its inline block with `itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID); if errRes != nil { return errRes, nil }`, then keeps its own role check unchanged. `report_progress`'s slightly different remediation text and `get_backlog_item`'s "swallow the error, only use for role text" behavior both still fit — `get_backlog_item` doesn't need to call the new helper at all (it isn't gated by the link, it only *reads* role from it), so `get_backlog_item`'s own `ITEM_NOT_FOUND` path (tools_backlog.go:127-133) is unaffected and already correct.

This satisfies AC1 (same code → same error everywhere) and AC2 (message includes both the resolved session UUID and the checked item id) with one new ~15-line function and 5 call-site substitutions — no proto/schema changes, no new storage method, no interface.

## 3. Second symptom: list_sessions / get_session / get_session_goal timeouts

- `list_sessions` (server/mcp/tools_discovery.go:87) and `get_session` (tools_discovery.go:157) both call `d.store.LoadInstances()` where `d.store session.InstanceStore` — this resolves to `Storage.LoadInstances()` (session/storage.go:290).
- `get_session_goal` (server/mcp/tools_goal.go:188) calls `h.findInstanceByID(sessionID)` first (tools_goal.go:371, also backed by an instance list/lookup), then `h.storage.GetSessionGoal(ctx, inst.UUID)`.
- **They are architecturally unrelated to `GetItemSessionBySessionAndItem`/`GetBacklogItem` at the query level** — `LoadInstances` does a bulk `s.repo.List(ctx)` over the `session` ent table plus a bulk `SessionGoal.Query().Where(sessiongoal.SessionUUIDIn(...))` (storage.go:290-340an), not a join against `ItemSession`/`BacklogItem` at all, and shares no table, index, or lock with the backlog link/item queries at the SQL level.
- **They do share the exact same physical resource: one SQLite connection.** `session/ent_repository.go:74-86` opens the DB with `db.SetMaxOpenConns(1)` / `db.SetMaxIdleConns(1)`, explicitly commented "SQLite supports only one writer at a time; serialise all access through a single connection to eliminate 'database is locked' contention." Every ent-backed call in the process — `GetBacklogItem`, `GetItemSessionBySessionAndItem`, `LoadInstances`, `GetSessionGoal`, and every backlog mutation (`UpdateAcCriterionStatus`, `TransitionBacklogItemStatus`, etc.) — funnels through that single `*sql.DB` connection. `_timeout=5000` in the DSN (ent_repository.go:75) sets SQLite's internal *busy timeout* (how long SQLite itself waits for an internal lock once it has the connection), but does **not** bound how long a goroutine blocks in Go's `database/sql` connection-pool checkout waiting for that one connection to free up — that wait is unbounded unless the caller's `ctx` carries its own deadline.

**Hypothesis (INFERRED from code, not confirmed against incident logs/timing for the specific session in the bug report):** `list_sessions`/`get_session`/`get_session_goal` timing out simultaneously for the same session is consistent with *some other query or transaction holding the single SQLite connection* for an extended period (e.g. a slow bulk `LoadInstances` call itself, a stuck/long-running write transaction such as one of the `client.Tx(ctx)` blocks in `ent_repository_backlog.go:1065`/`:1552` or `storage_backlog.go:438`/`:519`/`:608`, or simple contention from many concurrent MCP tool calls all wanting the one connection at once). This would explain simultaneous, unrelated-looking timeouts without requiring any shared table/lock at the SQL level — the bottleneck is Go-level connection-pool serialization, not a row/table lock. This is architecturally plausible and would be the single highest-value follow-up to verify (e.g. instrument connection-checkout wait time, or temporarily raise `SetMaxOpenConns` with WAL mode already enabled to see if timeouts correlate with pool contention) — but confirming it as *the* cause (vs. coincidental timing, an unrelated tmux/session-store issue, or a slow query specific to that one session's data volume) needs log evidence from the actual incident window, which wasn't available for this research pass. Recommend filing this as a documented follow-up per requirements.md's "fix now or file follow-up" (AC4) — it's a plausible-but-unconfirmed shared root cause, not a proven one, and is not required to unblock the ITEM_NOT_FOUND/PERMISSION_DENIED fix (which doesn't touch `LoadInstances`, connection pooling, or the goal/session-list code paths at all).

## 4. TOCTOU / consistency requirements

The proposed `resolveItemLink` helper (section 2) does two sequential, non-transactional storage calls: link check, then (only on link-not-found) an item-existence check. A window exists where the item could be deleted between those two calls, and — given `SetMaxOpenConns(1)` — the single connection *is* released and can be reacquired by a different goroutine's write in between the two calls, so the race is real, not merely theoretical.

**Recommendation: accept this as a documented, acceptable race — do not wrap it in an `ent.Tx`.** Reasoning:
1. **Precedent already accepts this exact shape.** `request_review` (tools_backlog.go:373 link check, :404 second `GetBacklogItem` call) already has this identical race today, unaddressed, with no reported issue — the fix isn't introducing a new risk class to the codebase.
2. **The failure mode of losing the race is graceful, not silent.** If the item is deleted in the gap, the caller gets `PERMISSION_DENIED` (stale-but-plausible) instead of `ITEM_NOT_FOUND` (fresher) for one request; a retry (or the next tool call) will see `ITEM_NOT_FOUND` correctly once the delete has landed. This is a transient inconsistency, not a wrong-forever state — materially different from the bug being fixed, where *every* call was permanently wrong for the same underlying condition.
3. **A transaction wouldn't actually close the gap that matters.** `client.Tx(ctx)` in this repo (e.g. ent_repository_backlog.go:1065) is used for multi-statement *writes* needing atomicity; wrapping two *reads* in a transaction on a single-connection SQLite setup adds transaction-begin/commit overhead and a longer connection hold (worsening contention against the section-3 hypothesis) for a race window measured in the time between two point-SELECTs — sub-millisecond in the common case, and irrelevant to concurrent correctness since nothing here mutates state.
4. Consistent with `.claude/rules/go-double-checked-locking.md`'s spirit (return what you actually observed, don't paper over a race with false certainty): the helper should return the error for the state it actually saw at each check, not attempt to fabricate stronger consistency than the storage layer offers elsewhere in this codebase.

Document this explicitly in a code comment on `resolveItemLink` (a TOCTOU note, not a TODO) so a future reader doesn't "fix" it into a transaction that fights the connection-pool constraint from section 3.

## Summary for the plan phase

- Fix location: new unexported helper method on `backlogHandlers` in `server/mcp/tools_backlog.go`, called from the 5 mutating tool call sites (lines 301, 373, 502, 662, 755); `get_backlog_item`'s existing behavior (line 127, 193) is untouched.
- No proto changes, no new storage/ent methods, no interfaces.
- PERMISSION_DENIED message must interpolate `callerUUID` and `itemID` to satisfy AC2.
- Symptom 2 (list_sessions/get_session/get_session_goal timeouts): document as a plausible-but-unconfirmed shared root cause (single-SQLite-connection contention, `session/ent_repository.go:84` `SetMaxOpenConns(1)`) and file as a follow-up rather than fixing in this project — it's architecturally distinct from the join-query bug and touches a different, higher-risk surface (connection pool sizing / WAL tuning) that needs its own investigation with real incident data.
- TOCTOU between the link check and the item-existence check: accept as a documented race, do not add a transaction — matches existing unguarded two-call sequencing already present in `request_review`, and a transaction would fight the single-connection design instead of helping it.
