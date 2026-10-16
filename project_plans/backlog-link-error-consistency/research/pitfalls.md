# Pitfalls Research: backlog-link-error-consistency

## Code map (confirmed)

Five mutating handlers in `server/mcp/tools_backlog.go` share the identical link-check
pattern (`storage.GetItemSessionBySessionAndItem` → `errors.Is(linkErr, session.ErrNotFound)`
→ `PERMISSION_DENIED`):

| Handler | Link-check site | Message |
|---|---|---|
| `reportProgress` | tools_backlog.go:301-306 | tools_backlog.go:304 |
| `requestReview` | tools_backlog.go:373-379 | tools_backlog.go:376 |
| `submitReviewVerdict` | tools_backlog.go:502-508 | tools_backlog.go:505 |
| `reportPRCreated` | tools_backlog.go:662-668 | tools_backlog.go:665 |
| `submitTriageResult` | tools_backlog.go:755-761 | tools_backlog.go:758 |

`getBacklogItem` (tools_backlog.go:127-133) calls `storage.GetBacklogItem` directly and is
the only one of the six that already distinguishes item-not-found correctly.

`GetItemSessionBySessionAndItem` (session/storage_backlog.go:201-223) uses
`itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))` as a query predicate — ent
returns the same `ErrNotFound` whether the join row is missing or the backlog item row
itself is gone, so the handler-level `errors.Is` check can't tell the two apart. This is the
confirmed root cause.

**Important existing precedent**: `reportPRCreated` already does a *second*,
independent `h.storage.GetBacklogItem(ctx, itemID)` call at tools_backlog.go:673-679,
purely to fetch item fields for idempotency/PR-URL checks, and it *already* returns
`ErrItemNotFound` correctly if that call fails. But it's ordered *after* the link check
(line 662), so if the item doesn't exist, the link check fails first and the correct
item-not-found branch below is never reached. This is a concrete illustration of the bug and
a template for the fix (reorder / reuse this pattern), but also a warning: reportPRCreated
is not "clean" of the pattern the fix needs to add — it needs *reordering*, not a bolt-on,
while the other four handlers need a *new* existence check added from scratch.

## 1. TOCTOU risk (check-then-check race)

If the fix does "check item exists" then separately "check link exists," the risk window is:
item deleted/archived between the two ent queries. Given:
- `BacklogItem` deletion/archival is a user-triggered, low-frequency admin action (no code
  path found that deletes items on any hot/frequent trigger — archival sets `archived_at`,
  a soft state field, not a row delete for most flows; confirm at plan time whether "item
  doesn't exist" in practice means hard-deleted row or archived-but-present row, since those
  need different queries).
- Backlog MCP tool calls are invoked by a single work/review/triage session, not concurrent
  high-QPS callers.

**Recommendation**: acceptable to document rather than solve with a transaction. A single
`context`-scoped read using ent's query builder (`Where(itemsession.HasBacklogItemWith(...))`
plus a *separate* `storage.GetBacklogItem` fallback only on the `ErrNotFound` path — not
speculatively on every call) minimizes the window to a single extra round-trip that only
fires on the already-rare not-found path. Do not wrap in a DB transaction; note the residual
race in a code comment on the fix instead of adding transactional complexity for a
lifecycle event this infrequent.

## 2. Information-disclosure risk

**Checked**: `session/ent/schema/backlog_item.go` (full schema read) has no
workspace/tenant/owner/user field of any kind — no `WorkspaceID`, `OwnerID`, `TenantID`, or
similar. `grep -rn "workspace|tenant|owner_id"` across the schema package returns nothing for
BacklogItem. Stapler Squad is a single-user local tool (per project CLAUDE.md: "Manages AI
agent sessions... in isolated tmux sessions with git worktrees" — no multi-user/auth model
anywhere in the architecture). There is exactly one implicit "tenant": the local user running
the app.

**Conclusion**: returning `ITEM_NOT_FOUND` vs `PERMISSION_DENIED` for the same item id does
**not** leak cross-tenant information — there are no other tenants. The caller's session UUID
(`STAPLER_SESSION_UUID`, read from context via `sessionUUIDFromContext` /
`callerSessionUUID`, tools_backlog.go:30-42) is trusted as "a session this same local
Stapler Squad instance spawned," not as an authorization boundary between mutually
distrusting parties. This risk is a non-issue for this fix; no design changes needed on this
axis. (Flag this finding explicitly in the plan so a reviewer doesn't re-litigate it — it's a
one-line "checked, no tenant field exists" fact, not a judgment call.)

## 3. Test-fragility / exhaustiveness risk

Five near-identical call sites (`report_progress`, `request_review`, `submit_review_verdict`,
`report_pr_created`, `submit_triage_result`) is a classic fix-4-of-5 trap. Confirmed via
`server/mcp/tools_backlog_test.go`: each handler already has its own
`Test<Handler>_RejectsWhenSessionNotLinked`-style test (e.g.
`TestReportProgress_RejectsWhenSessionNotLinkedToItem`,
`TestRequestReview_RejectsWhenSessionNotLinked`,
`TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork` — note this last one tests a
*different* PERMISSION_DENIED, the role check, not the link check; see pitfall 3b below) —
but each test lives in isolation, so nothing currently forces all five to be updated together.

**Recommendation for the plan**:
- Add one **table-driven / iterating** regression test that enumerates all 5 mutating tool
  *names* (or handler functions) as a single slice/table and asserts, for a non-existent
  `item_id`, that every one of them returns `ITEM_NOT_FOUND` — not five independent copy-pasted
  test functions that can silently diverge or be forgotten when a 6th mutating tool is added
  later.
- Pair it with a **grep-based structural guard** (or an ast-grep/static check) asserting no
  remaining call site matches the pattern `errors.Is(linkErr, session.ErrNotFound)` immediately
  followed by an unconditional `ErrPermissionDenied` without a preceding item-existence check —
  this catches the "5th call site missed" case even if the hand-written table test itself has a
  gap. At minimum, grep for all 5 known line-number sites in a comment in the test file so a
  reviewer can diff site count against fix count.
- **3b — do not conflate two different PERMISSION_DENIED sources.** `reportPRCreated`
  (tools_backlog.go:670) and `submitTriageResult` (tools_backlog.go:763) *also* return
  `PERMISSION_DENIED` for a **role mismatch** (`itemSession.Role != "work"` /
  `!= "triage"`) — a completely different, already-correct condition (item exists, link
  exists, wrong role). A naive "replace every PERMISSION_DENIED near a not-linked item" sweep
  risks touching these role-check branches too. The fix must scope strictly to the
  link-existence check immediately following `GetItemSessionBySessionAndItem`, not every
  `ErrPermissionDenied` literal in the file.

## 4. Backward-compatibility risk

Searched `.claude/skills`, `.claude/commands`, and `server/mcp/tools_backlog.go`'s own
embedded guidance strings (the role-aware text built in `getBacklogItem`, lines 197-228) for
`"PERMISSION_DENIED"` and `"not linked"` — **zero matches** in skills/commands, and the
embedded guidance text itself never references either the error code or the string
"not linked" (it only describes the workflow steps: report_progress → request_review →
wait/poll, etc.). The project-local backlog skill commands (`backlog:status`,
`backlog:review`, `backlog:done-N`, `backlog:fail-N`, `backlog:ship`, `backlog:help`) invoke
MCP tools directly by name/args, not by pattern-matching error text.

**Conclusion**: no discovered caller hardcodes or parses the current
`PERMISSION_DENIED: "this session is not linked to the specified backlog item"` string. The
only consumer of these error codes is the calling LLM session reading the JSON `code` field
(`MCPError.Code`) to decide what to do next — changing `PERMISSION_DENIED` → `ITEM_NOT_FOUND`
for the item-doesn't-exist case changes what an in-flight session *does* next (per
acceptance criterion 2, ideally toward better self-diagnosis), which is the intended
behavior change, not a break. Still worth a final grep across `session/` and `server/` Go
test files for literal `"not linked to the specified backlog item"` before landing the fix,
in case any test asserts on message text rather than just the code — `tools_backlog_test.go`
tests checked above assert on `errCode`/`errObj["code"]`, not message strings, so this
looks safe, but re-verify at implementation time since message wording may still change
(acceptance criterion 2 asks for more detail in the message).

## 5. Root-cause-vs-symptom risk (explicitly flag in plan)

This item's stated scope (`Root cause` section of requirements.md, and acceptance criteria
1-3) is **the error code, not the underlying disappearance of the item/link**. Fixing only the
error code makes the failure *legible* (a session can now tell "item is gone" from
"I'm not linked") but does **not** prevent a backlog item or its ItemSession link from
vanishing out from under a live session in the first place — the mismatch will recur, just
with a clearer signal next time. The plan should state this explicitly as an accepted
trade-off matching the requirements' own non-goal ("Rebuilding/auto-recovering a dropped
session→item link (separate reconciliation feature)"), not silently imply the class of bug
is closed.

**Secondary distinct symptom** (list_sessions/get_session/get_session_goal timeouts,
acceptance criterion 4): spot-checked `discoveryHandlers.getSession`
(server/mcp/tools_discovery.go:157-181) — it calls `d.store.LoadInstances()` then loops over
in-memory instances; no obvious per-call blocking I/O in the handler itself, but
`LoadInstances()` itself (session/storage layer, not read in this pass) is the likely place a
timeout would originate (e.g. per-instance git/tmux liveness checks). This is a genuinely
**separate code path** from the ItemSession join-query bug — different storage layer, no
shared function — so there's no evidence the two symptoms share a root cause. Recommend the
plan document this as: investigated, no shared cause found with the link-consistency bug,
timeout root cause needs its own follow-up investigation (do not block this fix on solving
it, but do not silently drop acceptance criterion 4 either — write up what was checked and
what wasn't, per the requirement's own "fix now or file follow-up" framing).
