# Requirements: Fix Review Queue "Needs a Decision" Empty

**Item ID:** b41b6253-386b-4998-b5a9-5a19dcc5d1ca  
**Project:** fix-review-queue-empty

## Problem Statement

The Review Queue's "Needs a Decision" section appears empty even when sessions are actively waiting for user input (e.g., `InputRequired`, `ApprovalPending`). The screenshot shows 39 total items, all classified as "Informational" (Priority.LOW = Stale/Idle/Complete), with 0 items surfaced in "Needs a Decision."

## Root Cause (Identified)

The `ackSnooze` suppression rule in `session/review_queue_suppression.go` applies a priority-based bypass:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
```

- `InputRequired` is assigned `PriorityMedium = 3` (in `review_queue_reason_table.go`)
- `PriorityHigh = 2`, so `PriorityMedium <= PriorityHigh` → `3 <= 2` → FALSE
- Once a user acknowledges a session, `IsAcknowledgedAfterOutput() = true`
- When content-dedup means `LastMeaningfulOutput` is unchanged (identical prompt reappears), `bypass = false` and `ackedAfterOutput = true` → the item is **removed from the queue** via `removeDecision("ackSnooze")`

**The fix already exists** as the `AckBypassFix` feature flag (`attention_ack_bypass_fix`) in `server/services/feature_flags_attention.go`, but it defaults to OFF. With it ON, all `needsHumanReasons` (`ApprovalPending`, `InputRequired`, `ErrorState`) bypass the ack snooze regardless of priority.

## Scope

- Fix `InputRequired` items disappearing from the review queue after a prior acknowledgment
- Ensure `ApprovalPending` and `ErrorState` are similarly protected (both are in `needsHumanReasons`)
- Optionally promote `InputRequired` from `PriorityMedium` to `PriorityHigh` so it always bypasses without the flag
- Optionally add `ReasonWaitingForUser` (prose questions) to `needsHumanReasons` once the prose-question detector is on

## Out of Scope

- The 38 "Informational" items (Stale/Idle) in the screenshot — these are working as designed; sessions the user has seen or are genuinely idle show as Informational
- The separate prose-question detector feature (tracked elsewhere)
- UI changes to the "Informational" tier display

## Acceptance Criteria

1. After acknowledging an `InputRequired` session, if that session presents a new (or repeated) input prompt, it re-appears in the "Needs a Decision" section — not silently suppressed by `ackSnooze`.
2. After acknowledging an `ApprovalPending` session, if a new approval dialog appears, it re-appears in "Needs a Decision" — not suppressed.
3. The `ackSnooze` suppression rule remains effective for Idle/Stale/Complete sessions (no regression in notification spam reduction).
4. The fix applies without requiring the user to manually toggle a feature flag.
5. Existing test coverage passes; new regression tests cover the previously-broken `InputRequired` + `ackSnooze` interaction.
