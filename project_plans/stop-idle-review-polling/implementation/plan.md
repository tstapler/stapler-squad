# Implementation Plan: stop-idle-review-polling

Architecture: two targeted patches to `server/mcp/tools_backlog.go`, each
independently testable. No new subsystems; no schema changes.

## Task 1 — request_review response: add idle instruction (AC-0, AC-3)

**File:** `server/mcp/tools_backlog.go`, function `requestReview`, around
line 1494.

Replace:
```go
return mcpgo.NewToolResultText(fmt.Sprintf(
    "Review requested for item %s. The item has been moved to review status.",
    itemID,
)), nil
```

With:
```go
return mcpgo.NewToolResultText(fmt.Sprintf(
    "Review requested for item %s. The item has been moved to review status.\n\n"+
        "End your turn now and stay idle — do NOT call wait_for_backlog_event or use "+
        "ScheduleWakeup or /loop to poll. Every wake re-reads your full context; "+
        "the app sends a steering message to this session as soon as the verdict is recorded.",
    itemID,
)), nil
```

**Test (AC-3):** add to `tools_backlog_test.go` (or a dedicated file):
```go
// Pins: request_review response must include stop/idle instruction.
func TestRequestReview_ResponseIncludesIdleInstruction(t *testing.T) {
    // ... call requestReview, check response contains "End your turn" or
    // "end your turn" and does not contain "ScheduleWakeup" as a recommendation
}
```

## Task 2 — wait_for_backlog_event: immediate cap when item is in review (AC-1, AC-2)

**File:** `server/mcp/tools_backlog.go`, function `waitForBacklogEvent`, after
the existing `currentStateWaitResult` precheck (around line 910).

Add a review-status short-circuit before the `waitCapReached` check:

```go
// Post-request_review sessions must not poll — StartVerdictSteering delivers
// the verdict via steer_session. If the item is already in review, return
// the cap result immediately so the session ends its turn.
if item.Status == string(session.BacklogStatusReview) {
    return okResult(WaitForBacklogEventResult{
        MCPResult: MCPResult{Success: true, Error: &MCPError{
            Code: "WAIT_CAP_REACHED",
            Message: fmt.Sprintf(
                "item %s is in review — the app steers this session when the verdict "+
                "lands; do not poll with wait_for_backlog_event, ScheduleWakeup, or /loop. "+
                "End your turn and stay idle.",
                itemID,
            ),
        }},
        EventReceived: false,
        ItemID:        itemID,
    }), nil
}
```

Place this _after_ `currentStateWaitResult` (which handles the "verdict
already exists" case, and for review items with a fresh verdict that check
already fires). Place it _before_ `waitCapReached` to skip the cap counter
entirely — no point incrementing when item is statically in review.

**Test (AC-2):** add to `tools_backlog_wait_cap_test.go`:
```go
func TestWaitForBacklogEvent_ItemInReview_ImmediateCap(t *testing.T) {
    // Create item in review status, call wait_for_backlog_event,
    // assert returns immediately with WAIT_CAP_REACHED without blocking.
    start := time.Now()
    result, err := handler.waitForBacklogEvent(ctx, makeToolReq(...))
    require.NoError(t, err)
    require.Equal(t, "WAIT_CAP_REACHED", decodeWaitResult(t, result).Error.Code)
    require.Less(t, time.Since(start), 200*time.Millisecond)
}
```

## Task 3 — audit other prompts for ScheduleWakeup recommendations (AC-4)

Run:
```bash
grep -rn "ScheduleWakeup\|schedule_wakeup\|/loop" server/mcp/ session/ \
  --include="*.go" | grep -v "_test.go"
```

Verify every result is a prohibition ("do not use", "Never pair") or is
inside a comment/constant name, not a recommendation. Document findings.
No code changes expected based on research.

## Task 4 — regression run (AC-5)

```bash
go test ./server/mcp/... -run "TestWaitForBacklog|TestRequestReview" -timeout=2m
make test  # full suite
```

## Adversarial review

**Risk: `wait_for_backlog_event` blocked for reviewer sessions** — A reviewer
session should never call `wait_for_backlog_event` to wait for a verdict (its
role is to submit one). If it did, the review-status guard would fire. This is
correct behavior, not a regression — the reviewer's role prompt already says
"End your session immediately after calling submit_review_verdict."

**Risk: item transitions from review mid-wait** — If a verdict lands and
transitions the item _during_ an existing `waitForBacklogEvent` call (before
our guard runs), `currentStateWaitResult` fires first (line 905) and returns
the verdict directly. Our guard is only reached when `currentStateWaitResult`
returns nil (no current-state match), meaning item is in review with no
verdict yet. Correct and consistent.

**Risk: stale item read** — `GetBacklogItem` at line 894 fetches the item
freshly; the `item.Status` check uses that fresh value. No stale-read concern.

**Risk: cap counter reset on request_review (line 1322)** — We skip the cap
counter entirely for the review-status path (guard fires before counter
increment). If an item is forced back from review to in_progress (recovery
path), `item.Status` will no longer be `review` so the guard won't fire for
the subsequent in_progress wait. Correct.

**Risk: message text becomes stale if stored in context** — The message
includes `itemID` but no timestamps. Sessions won't cache tool results across
turns. Low risk.
