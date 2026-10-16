# Requirements: stop-idle-review-polling

Item 1e693954 — 2026-10-04 quota audit found work sessions burning tokens by
repeatedly waking up after `request_review`, each wake re-reading a growing
context that can exceed the 1-hour prompt-cache TTL (paying a full cache
rewrite) or exceed the 5-minute TTL on credit-draw plans.

## Problem statement

After `request_review`, a work session is supposed to end its turn and stay
idle while the server-side `StartVerdictSteering` loop waits for the verdict
and then steers the session. In practice sessions are still polling because:

1. **`request_review` success response** contains no "end your turn" instruction
   — it only says "Review requested for item %s. The item has been moved to
   review status." Sessions end the turn correctly only when their role prompt
   loaded that instruction; a ScheduleWakeup call made _before_ the tool
   response is parsed skips that window entirely.
2. **`wait_for_backlog_event` is callable after review starts** — nothing
   prevents a session in "review" status from calling it; it blocks up to 60s,
   times out, returns WAIT_TIMEOUT (which says "don't use ScheduleWakeup"),
   and the session calls ScheduleWakeup anyway. The cap of 3 timeouts adds up
   to 180s of idle turns plus 3 full context re-reads before parking fires.
3. **Other session prompts** may still recommend ScheduleWakeup or /loop in
   contexts where they should not.

## Scope

In scope:
- `server/mcp/tools_backlog.go` — `requestReview` success response, and
  optionally short-circuit `waitForBacklogEvent` when item is in review
- `session/backlog_commands.go` — `review.md` (already fixed; verify)
- `server/mcp/tools_backlog.go` tool descriptions (already fixed; verify)
- Audit all other MCP tool descriptions + prompt strings for remaining
  `ScheduleWakeup`/`/loop` recommendations

Out of scope:
- Removing `StartVerdictSteering` (it works; keep it)
- Removing `wait_for_backlog_event` tool entirely
- Changes to `fix-idle-wait-cost-guardrails` (per-role token budgets; tracked
  separately)
- `review-queue-event-driven` (PTY-to-queue event path; tracked separately)

## Acceptance criteria

AC-0: `request_review` success response includes an explicit "end your turn
and stay idle — the app steers you when the verdict lands" instruction so a
session that reads the response and then ends its turn has no need to poll.

AC-1: When `wait_for_backlog_event` is called for an item that is currently
in "review" status (i.e., the item is waiting for a reviewer verdict), the
call returns `WAIT_CAP_REACHED` immediately instead of blocking for up to 60s.
The message directs the session to end its turn.

AC-2: A unit test covers AC-1: calling `wait_for_backlog_event` on an item
in `review` status returns `WAIT_CAP_REACHED` without blocking.

AC-3: A unit test asserts that the `request_review` success response text
contains "end your turn" (or equivalent stop phrasing), paralleling the
existing pin test in `tools_backlog_wait_cap_test.go`.

AC-4: No MCP tool description or session prompt string in the codebase
recommends calling `ScheduleWakeup` or `/loop` in a wait/poll context. (A
compile-time grep-as-test or a simple audit comment suffices if every instance
has been reviewed and suppressed or removed.)

AC-5: Existing tests in `server/mcp/tools_backlog_wait_cap_test.go` and
`server/mcp/tools_backlog_test.go` continue to pass (`make test`).
