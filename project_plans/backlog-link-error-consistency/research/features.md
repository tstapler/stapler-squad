# Research: Feature/Pattern Landscape — backlog-link-error-consistency

## 1. What can make a BacklogItem row or ItemSession row disappear mid-session

Grepped `session/backlog_lifecycle.go`, `session/backlog_remediation.go`,
`session/backlog_sync.go`, `session/backlog_triage.go`, and
`session/ent_repository_backlog.go` for delete/archive logic.

### Hard delete (the actual trigger for this bug)

`EntRepository.DeleteBacklogItem` (`session/ent_repository_backlog.go:783-840`) is the
**only** code path that removes a `BacklogItem` row. It:
1. Fetches the item (404s via `ErrNotFound` if already gone).
2. Resolves all `ItemSession` IDs joined to it.
3. Hard-deletes their `ReviewVerdict` rows, then hard-deletes the `ItemSession` rows
   themselves (`itemsession.IDIn(itemSessionIDs...)`).
4. Hard-deletes the `BacklogItem` row.

This is wired to `BacklogService.DeleteBacklogItem`
(`server/services/backlog_service_lifecycle.go:375-391`), which is called from the web UI
operator action in `web-app/src/components/backlog/BacklogItemDetail.tsx:542-543`:
```tsx
if (!confirm("Permanently delete this item and all its history? This cannot be undone.")) return;
await deleteBacklogItem(item.id);
```
**There is no guard against deleting an item that has an active work/review session.**
An operator (or anyone driving the UI) can click Delete while a spawned session is mid-flight.
This is the most plausible real-world trigger for the reported incident: the item row and its
`ItemSession` link row vanish in the same transaction, so from that point on
`GetBacklogItem` correctly 404s (`ErrNotFound`) while `GetItemSessionBySessionAndItem`'s join
also 404s (`ErrNotFound`, indistinguishable cause) — exactly the split the bug report describes.

A second, test-only hard-delete path exists (`server/services/backlog_debug_mutate_handler.go`,
`handleDelete`) but is gated to `STAPLER_SQUAD_INSTANCE=e2e-local` and not reachable in a normal
deploy — not a production trigger, but useful for writing the regression test (it lets a test
directly force item deletion without fighting `TransitionBacklogItemStatus` gates).

### Soft delete / archival — NOT a trigger

`ArchiveBacklogItem` (`session/ent_repository_backlog.go:741-`) only sets the `archived_at`
timestamp column; the row is untouched otherwise. `archiveStaleDoneItems`
(`session/backlog_lifecycle.go:1334`, the `auto_archive_done` periodic detector) transitions
the item's *status* to `"archived"` via `TransitionBacklogItemStatus` — also a plain status
update, no row deletion. `GetBacklogItem`'s ent query
(`session/ent_repository_backlog.go:310-333`) does not filter on `archived_at` or status at
all, so an archived item is still found by both `GetBacklogItem` and the
`GetItemSessionBySessionAndItem` join. **Archival cannot reproduce this bug** — only the hard
`DeleteBacklogItem` path can. Worth stating explicitly in the plan so the fix doesn't
over-scope to also special-case archived items.

### Cascade-delete on ItemSession specifically

No standalone "delete just the ItemSession, keep the BacklogItem" path exists in the grepped
files — `ItemSession` rows are only removed as part of `DeleteBacklogItem`'s cascade. So in
practice "item exists but link row alone is gone" (as opposed to "both gone") is not something
today's code does to a previously-linked session; the two failure modes collapse to the same
trigger (full item deletion) rather than being independently reachable. (A link that was
*never created* — see §2 — is the other, structurally different way to reach "no link".)

### Status transitions

`TransitionBacklogItemStatus` (referenced throughout `backlog_lifecycle.go`) only ever changes
the `status` column and appends a `BacklogStatusEvent` audit row — never deletes `ItemSession`
rows. Re-triage/respawn flows (`SpawnSessionFromItem`, `AutoReopenAfterFailedReview`, etc.)
*create new* `ItemSession` rows for the new session; they don't delete the old session's link
row (old sessions' `ItemSession` rows persist until the item itself is deleted or the periodic
`reconcileTerminalItemSessions` sweep archives — not deletes — their *tmux* sessions once the
item reaches done/archived).

## 2. Can a live session legitimately have no ItemSession link yet? Yes — a startup race.

Traced `SpawnSessionFromItem` (`server/services/backlog_service_triage.go:360-`) step by step:

- Step ~10: `s.sessionCreator.CreateWorktreeSession(...)` /
  `CreateDirectorySession(...)` (line 739/742) actually creates and **starts** the tmux
  session — this is what sets `STAPLER_SESSION_UUID` in the tmux environment
  (`session/instance.go:1442`, `session/instance_tmux.go:274`) and auto-sends the initial
  prompt, i.e. the point at which the spawned Claude session starts running and could call
  any MCP tool.
- Step 12 (line 798): `s.storage.CreateItemSession(ctx, ...)` — this is what actually commits
  the `ItemSession` link row — happens **after** the session is already live. The comment at
  line 789 even says the ordering is deliberate ("Create ItemSession with the real session
  UUID (avoids `<pending>` orphan records on failure)").

So there is a real, if narrow, window between "session started, agent can call MCP tools" and
"link row committed" during which `GetItemSessionBySessionAndItem` legitimately returns
`ErrNotFound` for a perfectly healthy, actively-being-worked item — this is not an error state,
just a startup race. **Implication for the fix**: `PERMISSION_DENIED` for "no link" cannot be
treated as unconditionally meaning "stale/abandoned session" — the message/remediation (AC #2)
should account for "possibly still starting up, retry once" as well as "genuinely never linked
or link removed," since both produce the identical `ErrNotFound` from the join query today.

## 3. Existing templates for existence-vs-authorization separation

### Good template already in this repo: `server/services/file_service.go:150-159`
```go
entries, readErr = os.ReadDir(fullPath)
if readErr != nil {
    if os.IsNotExist(readErr) {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("directory not found: %s", requestedPath))
    }
    if os.IsPermission(readErr) {
        return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied: %s", requestedPath))
    }
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read directory: %w", readErr))
}
```
Same shape recurs at `file_service.go:312-315` (`GetFileContent`) and `:342`. The key property:
**a single syscall's error is inspectable enough to distinguish "doesn't exist" from
"exists but denied"** (`os.IsNotExist` vs `os.IsPermission` are different `errno`s under one
`*PathError`). The backlog MCP tools' problem is structurally different: the join query
(`itemsession.HasBacklogItemWith(...)`) collapses two distinguishable underlying conditions
(item missing vs link missing) into one identical `ent.IsNotFound`/`ErrNotFound`, discarding
the information `os.IsNotExist`/`os.IsPermission` preserve. The fix has to reintroduce that
distinction by adding a second query (existence check), not by inspecting the join error more
closely — the join error carries no signal to inspect.

### The backlog MCP tools already contain a (broken) attempt at this pattern
`reportPRCreated` (`server/mcp/tools_backlog.go:623-679`) is the **only** mutating backlog tool
that calls both `GetItemSessionBySessionAndItem` (line 662) *and* `GetBacklogItem` (line 673),
i.e. it already has the two calls the fix needs. But the order is backwards: the link check
runs first and returns early with `PERMISSION_DENIED` (line 664-665) before the
`GetBacklogItem` existence check ever runs. If the item has been deleted, the join's
`ErrNotFound` fires first and the `ErrItemNotFound` branch at line 675-676 is **unreachable
dead code** for that scenario. This is direct evidence the same inconsistency already exists
*within a single handler*, not just across handlers — and it's the clearest template to
correct and then replicate into `reportProgress`, `requestReview`, `submitReviewVerdict`, and
`submitTriageResult` (none of which call `GetBacklogItem` at all today — see the grep below).

```
server/mcp/tools_backlog.go:301  reportProgress      — link check only, no existence check
server/mcp/tools_backlog.go:373  requestReview       — link check only, no existence check
server/mcp/tools_backlog.go:502  submitReviewVerdict — link check only, no existence check
server/mcp/tools_backlog.go:662  reportPRCreated     — link check FIRST, existence check SECOND (unreachable on delete)
server/mcp/tools_backlog.go:755  submitTriageResult  — link check only, no existence check
```

### Correct-order template already established elsewhere in this file
`SpawnSessionFromItem` (`server/services/backlog_service_triage.go:375-380`) does a clean,
single-purpose existence check with no join involved:
```go
item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
if err != nil {
    if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
    }
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
}
```
This is the shape every mutating backlog MCP tool needs to run **first**, before the link
check — check existence via plain `GetBacklogItem`, map absence to `ITEM_NOT_FOUND`; only once
existence is confirmed, run `GetItemSessionBySessionAndItem` and map its `ErrNotFound` to
`PERMISSION_DENIED`.

## 4. Edge cases

### TOCTOU race between the two checks
Even with existence-check-then-link-check ordering, there is an unavoidable window between the
two DB reads where a concurrent `DeleteBacklogItem` call could remove the item and its link
after the existence check passes but before the link check runs. Given `DeleteBacklogItem` has
no active-session guard (§1), this is not just theoretical. Two reasonable options for the plan
phase to weigh:
- Accept the (rare, narrow) race as-is — worst case, a mutating call briefly reports
  `PERMISSION_DENIED` instead of `ITEM_NOT_FOUND` for a delete that happened mid-call; still
  strictly better than today's *unconditional* mismatch. Document this as a known, accepted
  race rather than silently leaving it unhandled.
- Wrap both reads in a single transaction/consistent snapshot if the storage layer supports it
  cheaply (SQLite is capped to 1 connection per `session/ent_repository_backlog.go:43`'s
  comment on `recordStatusEvent`, so a single-connection read pair is about as consistent as
  this codebase gets without an explicit `tx`). Check whether `EntRepository` already exposes a
  transaction helper other backlog mutations use (e.g. `ReconcileStuckItems` at
  `session/storage_backlog.go:589` runs inside a tx for a similar reason) as a precedent to
  reuse rather than inventing new transaction plumbing.

### Soft-deleted/archived vs hard-deleted
As established in §1, only hard delete matters for this bug — archived items remain fully
visible to both `GetBacklogItem` and the link join. No special-casing needed for `archived_at`
or `status="archived"` in the fix.

### `report_blocked` — does not exist in this codebase
Grepped `server/mcp/*.go`, `.claude/commands/backlog/` equivalents
(`session/backlog_commands.go`'s `buildDefaultSlashCommandSet`), and the currently-available
skill list (`backlog:done-N`, `backlog:fail-N`, `backlog:review`, `backlog:ship`,
`backlog:status`, `backlog:help` — no `backlog:blocked`). **There is no `report_blocked` MCP
tool, slash command, or skill anywhere in this repo.** `reportProgress`'s `status` argument
only accepts `pass`, `fail`, `in_progress` (`server/mcp/tools_backlog.go:291-296`) — no
`blocked` value. This means the requirements.md's framing ("report_blocked is itself gated by
the same link check") describes a tool that isn't implemented at all, not one that exists but
shares the bug. That actually *strengthens* the acceptance criteria: with no
`report_blocked`/blocked-status path at all, a session that hits this exact bug genuinely has
**zero** working channel to self-report once every mutating tool returns `PERMISSION_DENIED` —
`get_backlog_item` is read-only and can't record anything, and there is no fallback status to
set. Flag this gap explicitly for the plan phase: either (a) the fix's improved
`PERMISSION_DENIED` message (AC #2) is the entire mitigation — make sure it's actionable enough
for a human/operator reading session logs to self-diagnose without a report_blocked tool ever
existing — or (b) file a follow-up to add a real `report_blocked`/blocked-status escape hatch,
which is explicitly out of scope for this fix per the non-goals section but worth naming as a
recommended follow-up.

## Summary of concrete file/line references for the plan phase

| Concern | File:Line |
|---|---|
| Hard delete cascade (the trigger) | `session/ent_repository_backlog.go:783-840` |
| DeleteBacklogItem RPC handler | `server/services/backlog_service_lifecycle.go:375-391` |
| UI delete button, no active-session guard | `web-app/src/components/backlog/BacklogItemDetail.tsx:542-543` |
| Archive (non-trigger, for contrast) | `session/ent_repository_backlog.go:741-` |
| get_backlog_item existence check (correct, unambiguous) | `server/mcp/tools_backlog.go:127-133` |
| Join query collapsing two conditions into one ErrNotFound | `session/storage_backlog.go:201-223` |
| reportProgress — link check only | `server/mcp/tools_backlog.go:301-307` |
| requestReview — link check only | `server/mcp/tools_backlog.go:373-376` (approx.) |
| submitReviewVerdict — link check only | `server/mcp/tools_backlog.go:502-505` |
| reportPRCreated — has both checks, wrong order (existence check dead code on delete) | `server/mcp/tools_backlog.go:662-679` |
| submitTriageResult — link check only | `server/mcp/tools_backlog.go:755-758` |
| Correct existence-check template (no join) | `server/services/backlog_service_triage.go:375-380` |
| Startup race: session live before ItemSession row committed | `server/services/backlog_service_triage.go:739-742` (session created) vs `:798` (link committed) |
| os.IsNotExist/os.IsPermission template | `server/services/file_service.go:150-159` |
| report_blocked does not exist | grep of `server/mcp/*.go`, `session/backlog_commands.go` — no matches outside requirements.md |
