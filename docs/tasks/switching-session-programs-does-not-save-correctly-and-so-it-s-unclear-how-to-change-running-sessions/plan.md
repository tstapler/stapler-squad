# Plan: Switching Session Programs Does Not Save Correctly

## Executive Summary

The primary bug is a single backend guard (`*req.Msg.Program != ""` in `session_service.go:1430`) that silently drops any request that sets a session's program to "System default" (empty string `""`), returning HTTP 200 with stale data and no error. A compound UX gap makes the issue worse: the only editable program field is buried in the "Info" tab of `SessionDetailView` with no overflow menu entry, making it nearly impossible for users to discover. The minimal fix is one backend condition removal; discoverability and UX polish are additive follow-on tasks.

## Root Causes

### RC1 — Backend guard silently drops "System default" saves (required fix)

`server/services/session_service.go:1430`:
```go
// BEFORE (broken): silently no-ops when user selects "System default" (value="")
if req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program {

// AFTER (fix): proto optional already distinguishes nil (not sent) from "" (explicitly cleared)
if req.Msg.Program != nil && instance.Program != *req.Msg.Program {
```

`programs.ts` defines `{ label: "System default", value: "" }`. The frontend sends this correctly as an explicit empty string via the proto `optional string` field. The `!= ""` guard is wrong and discards valid requests.

**Blocker caveat:** The ent schema has `field.String("program").NotEmpty()` — empty string may fail DB persistence. Resolve by storing the config-resolved default name instead of empty string on save.

### RC2 — Stale React state after WatchSessions updates (secondary)

`SessionDetailView.tsx` initializes `programValue` via `useState(session.program || "")` with no `useEffect` to re-sync when `session.program` changes. After a save or external update, local state drifts from server value.

### RC3 — Discoverability gap (UX)

The program edit UI exists only in the "Info" tab of `SessionDetailView`. No overflow menu entry, no session-list affordance, no hint in `ResumeSessionModal`. Users have no clear path to change a session's program.

### RC4 — Persist-after-restart race (footgun)

`Restart()` fires at line 1436 before `SaveInstances` at line 1533. If `instance.started == false` (fallback load path), `Restart()` returns `ErrCannotRestart` and early-returns — `SaveInstances` is never called. Program is neither restarted NOR saved to DB.

## Implementation Approach

**Phase 1 — Backend core fix:** Remove `&& *req.Msg.Program != ""`. Resolve the `NotEmpty()` constraint (store resolved default rather than empty). Move `SaveInstances` before `Restart()` to fix RC4.

**Phase 2 — React state hardening:** Add `useEffect` syncing `programValue` ← `session.program` when `!isEditingProgram`.

**Phase 3 — Discoverability:** Add "Change Program" entry to `SessionActionsOverflow.tsx` (reuse steer/checkpoint inline dialog pattern).

**Phase 4 — Safety UX:** Confirmation dialog before killing Active session. "Pending on resume" indicator for non-Active sessions.

**Phase 5 — Tests:** Cover zero-tested `UpdateSession` program paths in Go + Jest.

## Task Breakdown

| # | Task | Estimate | Category |
|---|------|----------|----------|
| 1 | Remove `&& *req.Msg.Program != ""` from `session_service.go:1430`; resolve empty-string via config default on save | 1h | backend |
| 2 | Move `SaveInstances` call before `Restart()` in `UpdateSession` to fix RC4 ordering | 0.5h | backend |
| 3 | Clear `claudeSession.ConversationUUID` when switching program away from claude family | 1h | backend |
| 4 | Add `useEffect` to re-sync `programValue` from `session.program` when not editing in `SessionDetailView.tsx` | 0.5h | frontend |
| 5 | Add "Change Program" item to `SessionActionsOverflow.tsx` with inline select dialog | 2h | frontend |
| 6 | Confirmation dialog before killing Active session on program change | 1h | frontend |
| 7 | "Pending on next resume" indicator for Stopped/Paused sessions after program change | 1h | frontend |
| 8 | Go tests: `UpdateSession` program — Active (restart occurs), Stopped (save only), empty-string clear, restart error path | 2h | test |
| 9 | Jest test: program inline select — save, cancel, stale-state re-sync from WatchSessions | 1h | test |

## Dependencies and Blockers

- **`NotEmpty()` ent constraint** — must decide persistence strategy (store resolved default name) before RC1 fix lands
- **Poller vs fallback path** — confirm test environment uses live poller so `started == true`; otherwise RC4 is not testable without mocking
- **No proto changes needed** — `optional string program = 5` is already correct; `optional` wrapper already distinguishes nil from `""`
- **No ORM schema changes needed if resolving empty to default** — `SetProgram(resolvedDefault)` works with existing `NotEmpty()` constraint
