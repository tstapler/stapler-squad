# Requirements: Review Queue Empty for Sessions Requiring Input

## Background

The Review Queue's "NEEDS A DECISION" section shows "All caught up — Nothing needs your attention
right now" even when 39 sessions are present (34 stale, oldest 15h). All 39 items are classified
as "Informational" (Priority.LOW). The user knows sessions should be waiting for their input.

## Problem Statement

Sessions detected as `InputRequired` (waiting for explicit user input, e.g. AskUserQuestion
responses) or `NeedsApproval` (tool approval dialogs) are silently suppressed from the
"NEEDS A DECISION" tier of the Review Queue after the user has previously acknowledged them.

## Root Cause (diagnosed in triage.md)

The `ackSnooze` suppression rule in `session/review_queue_suppression.go:131`:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
```

`InputRequired` sessions have `PriorityMedium` (3), which fails `priority <= PriorityHigh` (2).
After any prior acknowledgment (`ackedAfterOutput = true`), `bypass` is false → `ackSnooze`
returns `removeDecision("ackSnooze")` → session is filtered from the queue entirely.

The `attention_ack_bypass_fix` feature flag already implements the correct bypass logic but
defaults to OFF: when ON, `isNeedsHumanReason(c.reason)` causes `InputRequired`,
`ApprovalPending`, and `ErrorState` sessions to bypass ackSnooze regardless of priority.

## Acceptance Criteria

1. Sessions with `ReasonInputRequired` (PriorityMedium) remain in "NEEDS A DECISION" after the
   user has previously acknowledged them, as long as new output has arrived since the ack.
2. Sessions with `ReasonApprovalPending` and `ReasonErrorState` behave the same way (they were
   already Medium/High but the fix unifies the bypass logic).
3. The `attention_ack_bypass_fix` flag defaults to ON; existing off-by-default wording in
   the flag description is updated.
4. No regression to the spam-prevention behaviour: a session truly idle with no new output
   since the last ack is still correctly suppressed.

## Scope

- **In scope**: Enable `attention_ack_bypass_fix` flag by default; update description; add/update tests.
- **Out of scope**: Detecting InputRequired when the controller is active but `ClaudeStatus`
  returns Idle (separate upstream detection gap; tracked separately).
- **Out of scope**: Changing the priority of InputRequired from Medium to High.

## Complexity

**1** (quick targeted fix — a single flag default change with description and test updates)

## Open Questions

None — root cause and fix are fully diagnosed.
