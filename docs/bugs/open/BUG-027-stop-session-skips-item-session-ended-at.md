# BUG-027: `stop_session` (MCP tool / `StopSession` RPC) Never Updates the Backlog `ItemSession.EndedAt` [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-07-20
**Impact**: A backlog work session stopped via the `stop_session` MCP tool (or the underlying `SessionService.StopSession` RPC) leaves its `ItemSession.EndedAt` permanently `null`, even though the tmux process and worktree are gone. The item's own bookkeeping believes the session is still running. This bypasses the `onSessionExited` reconciliation path (`session/backlog_lifecycle.go`) entirely — none of today's stale-work remediation (BUG fix in PR #196), review-verdict processing, or auto-ship logic ever fires for a session torn down this way, because they all key off `onSessionExited` being called on natural process exit.

## Problem Description

stapler-squad's backlog automation assumes every work/review session eventually calls `onSessionExited` (`session/backlog_lifecycle.go`), which is what drives `handleReviewSessionExited`, the stale-work reconciliation added in PR #196, and the rest of the review→ship pipeline. That hook appears to be wired to the *natural* process-exit path (tmux pane closing, agent process terminating on its own). It is not called — or not called with the same effect — when a session is torn down via the explicit `stop_session` MCP tool / `StopSession` RPC.

Confirmed live: backlog item `c2ad7bf3-91bf-4d47-8654-0f2f20869080` ("Dedent shortcut broken in edit mode") had a zombie work session (`stelekit-fix-shift-tab-dedent-desktop-r9`, session UUID `289f5433-ce30-4a25-b951-cf01bd864ed5`) stuck with `endedAt: null` for 6+ hours after the underlying blocker (a real git merge conflict) had already been resolved out-of-band. `mcp__stapler-squad__stop_session` was called with `confirm: true` to clean it up. `get_session` afterward confirmed the session was fully gone (`SESSION_NOT_FOUND`). But re-querying the backlog item via `GetBacklogItem` showed the same `ItemSession` row still `endedAt: null` — no transition, no reconciliation, nothing.

This means: **manually stopping a stuck backlog session is not equivalent to letting it exit naturally**, and doing so silently strands the backlog item exactly the way a hung session already does — except now there isn't even a process left for a stale-work detector to eventually notice as "no activity," because the item's own session bookkeeping still thinks it's alive.

## Reproduction Steps

1. Have a backlog item in `review` (or `in_progress`) status with a live/tracked work `ItemSession` whose `endedAt` is `null`.
2. Call `mcp__stapler-squad__stop_session` (or `SessionService.StopSession` RPC) with `confirm: true` for that session.
3. Confirm the session is gone: `mcp__stapler-squad__get_session` returns `SESSION_NOT_FOUND`.
4. Query the backlog item: `mcp__stapler-squad__get_backlog_item` / `BacklogService.GetBacklogItem`.
5. Expected: the `ItemSession` entry for the stopped session has a non-null `endedAt`, and `onSessionExited`'s normal handling (status transition / stale-work reconciliation / etc.) has run.
6. Actual: `endedAt` is still `null`. The item's status is unchanged. Nothing downstream fires.

## Root Cause

Not yet fully traced to a specific line — needs investigation into `server/services/session_service.go`'s `StopSession` handler to determine whether it calls the same exit-notification path (`BacklogLifecycleListener.onSessionExited` or whatever the natural-exit hook chain is) as a real process exit, or whether it tears down the tmux/process/worktree directly without invoking it. Likely candidates:
- `StopSession` may kill the tmux session/process directly (`tmux kill-session` or similar) without going through the same callback chain a natural process exit uses (e.g. a tmux pane-death hook, `on-window-close`, or a supervisor goroutine watching for process exit).
- Or `onSessionExited` may be invoked, but with an early-return that skips the backlog-specific `ItemSession` update — e.g. if it checks `is.Role` or some other field first and the confirm-required stop path constructs/reaches it differently than natural exit does.

## Files Likely Affected

- `server/services/session_service.go` — `StopSession` handler (need to find the exact function; the MCP tool's `CONFIRMATION_REQUIRED` error message mentions "removes its tmux process and git worktree")
- `session/backlog_lifecycle.go` — `onSessionExited` and its natural-exit trigger path (compare against whatever `StopSession` actually calls)
- `session/session_driver.go` / `session/mux` — wherever the natural tmux-process-exit → `onSessionExited` wiring actually lives, to compare against `StopSession`'s teardown path

## Fix Approach

Make `StopSession` call the same exit-notification path a natural process exit uses (or explicitly call `onSessionExited`-equivalent logic) before/as part of tearing down the tmux process and worktree, so a manually-stopped backlog session gets the same `ItemSession.EndedAt` update, status-transition, and stale-work-reconciliation treatment as one that exits on its own. If there's a reason manual stops are intentionally exempted from this (e.g. to avoid double-processing a session an operator is intentionally killing mid-work, before it reaches a natural checkpoint), that exemption should be a deliberate, documented decision — not a silent gap discovered by accident.

## Verification

After fix: repeat the reproduction steps above — `stop_session` on a backlog work session should leave the corresponding `ItemSession.EndedAt` non-null, and any related backlog reconciliation (status transition, stale-work handling per PR #196's pattern) should behave the same as it would for a naturally-exited session.

## Related Tasks

- PR #196 (`fix(backlog): auto-remediate stale work sessions instead of notify-only`) — the stale-work remediation this gap bypasses entirely, since a session torn down via `stop_session` never accumulates the "no activity for N hours" signal that detector keys off (the row in `BacklogStuckState` is never even created, because from the item's perspective the session — per `EndedAt: null` — never stopped being "current").
- BUG-026 (`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md`) — same general area of the codebase (backlog session/status lifecycle bookkeeping), found during the same investigation session.
