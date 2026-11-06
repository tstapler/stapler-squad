# Research Findings: stop-idle-review-polling

## Existing mechanisms (already implemented)

### 1. StartVerdictSteering (server/services/backlog_service_verdict_steer.go)

`BacklogService.StartVerdictSteering()` is wired at startup
(`server/dependencies.go:1428`). It subscribes to backlog events via the
in-process EventBus and, when a `BacklogChangeVerdictRecorded` event fires,
calls `steerWorkSessionWithVerdict` which:
- Finds the live work session for the item
- Polls `IsReadyForSteer` at 5s intervals (up to 5 min) waiting for idle
- Calls `SteerActiveSession` to push the verdict as a steering message
- Falls back to an operator notification if the session never goes idle

This is the "server steers the session" path. It is correctly wired and has
passing unit tests (`server/services/backlog_service_verdict_steer_test.go`).

### 2. Wait cap (server/mcp/tools_backlog.go, lines 771–831)

`maxWaitTimeoutsPerSession = 3`. After 3 consecutive timeouts on the same
session+item, `waitForBacklogEvent` returns `WAIT_CAP_REACHED` immediately
without blocking. The cap message and the WAIT_TIMEOUT message both already
say "do not use ScheduleWakeup or /loop to wait". Operator notification fires
when cap is reached. Tests: `tools_backlog_wait_cap_test.go`.

### 3. Work role prompt (tools_backlog.go:510)

"After request_review, end your turn and stay idle (do not exit). Do NOT poll,
and do NOT use ScheduleWakeup or /loop to wait — every wake re-reads your
whole context; the app sends you a message with the verdict as soon as it is
recorded."

### 4. review.md command (session/backlog_commands.go:233)

"Then end your turn and stay idle (do not exit). Do NOT poll, and do NOT use
ScheduleWakeup or /loop to wait: every wake re-reads your whole context."

### 5. wait_for_backlog_event description (tools_backlog.go:3101)

"Not for waiting on a review verdict after request_review: end your turn
instead and the app steers the session with the verdict. Never pair with
ScheduleWakeup or /loop."

## Identified gaps

### Gap 1: request_review success response (CRITICAL)

`requestReview` at line 1494 returns:

```go
return mcpgo.NewToolResultText(fmt.Sprintf(
    "Review requested for item %s. The item has been moved to review status.",
    itemID,
)), nil
```

This contains **zero idle instruction**. A session that calls `request_review`
and hasn't loaded its role prompt will immediately start polling because the
response doesn't say "stop". Even sessions that _have_ loaded the role prompt
face a race: a ScheduleWakeup scheduled before parsing the response fires
regardless of what the response says.

Fix: append "End your turn now and stay idle — do NOT call
wait_for_backlog_event or use ScheduleWakeup or /loop. The app sends a message
to this session as soon as the verdict is recorded." to the success response.

### Gap 2: wait_for_backlog_event not blocked post-review

When a work session calls `request_review` on an item and then immediately
calls `wait_for_backlog_event`, the server:
1. Allows it (no review-status guard)
2. Blocks for up to 60s
3. Returns WAIT_TIMEOUT with instructions not to poll
4. Resets wait timeout counter (line 1322 resets on request_review, so the
   session gets 3 fresh timeouts = up to 180s + 3 context re-reads before
   the cap fires)

Fix: in `waitForBacklogEvent`, after the current-state precheck, if the item
is in `review` status AND the caller is a work session for this item,
immediately return `WAIT_CAP_REACHED`. `StartVerdictSteering` handles delivery;
no polling is needed.

### Gap 3: ScheduleWakeup recommendations in other prompts (minor)

Audit found no other MCP tool descriptions or prompt strings recommending
ScheduleWakeup in a polling context. The references in lines 510, 816, 827,
3101 are all _prohibitions_ ("do not use"), not recommendations. No additional
removals needed.

## Related plans

- `fix-idle-wait-cost-guardrails` — per-role token budgets and wall-clock
  bounds for idle loops (different scope; capacity monitor layer)
- `review-queue-event-driven` — PTY status event path for review queue
  (different scope; reviewer discovery latency)

## Risk assessment

- **False positive on review-status guard (Gap 2)**: if `wait_for_backlog_event`
  is called by a non-work session on an item in review, the guard would
  incorrectly fire. Fix: gate the guard on whether the caller's session is
  linked to the item as a work session (use `resolveItemLink` + role check).
  Simpler alternative: add an optional field to the request like
  `skip_if_in_review=true`, but that shifts the burden to the caller.
  Simplest: check item.Status == review at the guard point; the reviewer
  session calling `wait_for_backlog_event` on its own item is not a realistic
  use case (reviewer uses `submit_review_verdict`, not wait).
- **Race on StartVerdictSteering**: verdict arrives before `waitForSteerReady`
  fires the first tick (5s). The `dispatchVerdictSteer` dedup map ensures only
  one delivery per verdict key; no data loss.
- **Steering message too long**: `maxVerdictSummaryBytes = 2000` truncates the
  verdict summary. Sufficient for the action instruction.
