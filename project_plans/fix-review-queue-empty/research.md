# Research: Fix Review Queue "Needs a Decision" Empty

**Item ID:** b41b6253-386b-4998-b5a9-5a19dcc5d1ca  
**Project:** fix-review-queue-empty

## Root Cause: Confirmed

The `ackSnooze` suppression rule in `session/review_queue_suppression.go:131` removes
`InputRequired` sessions from the queue when the user has previously acknowledged them:

```go
func ackSnooze(c ruleCtx) (SuppressionDecision, bool) {
    bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
    if !bypass && c.ackedAfterOutput {
        return removeDecision("ackSnooze"), true
    }
    return SuppressionDecision{}, false
}
```

Steps to reproduce the bug:
1. Session presents an `InputRequired` prompt → added to queue with `PriorityMedium=3`
2. User acknowledges/skips the item → `LastAcknowledged` set, `IsAcknowledgedAfterOutput() = true`
3. Session presents the same (or new identical) `InputRequired` prompt → `LastMeaningfulOutput` unchanged (content-signature dedup) → `IsAcknowledgedAfterOutput()` still `true`
4. `ackSnooze` bypass check: `priority <= PriorityHigh` = `3 <= 2` = FALSE; `AckBypassFix` = OFF → `bypass = false`
5. Result: `ackedAfterOutput = true` AND `bypass = false` → `removeDecision("ackSnooze")` → session silently removed

## Priority Mismatch

`session/review_queue_reason_table.go` assigns:
- `StatusInputRequired` → `PriorityMedium = 3`
- `StatusNeedsApproval` → `PriorityHigh = 2`

The bypass threshold is `<= PriorityHigh (2)`, so `ApprovalPending` and `ErrorState` (which uses `PriorityUrgent=1`) both bypass ackSnooze automatically. Only `InputRequired` falls through the gap.

## The Fix Already Exists (Gated Behind a Flag)

`attention_ack_bypass_fix` in `server/services/feature_flags_attention.go` already implements
the correct behavior: when `AckBypassFix=true`, all `needsHumanReasons`
(`ApprovalPending`, `InputRequired`, `ErrorState`) bypass ackSnooze regardless of priority.

The flag chain:
- `feature_flags_attention.go:40` → `AckBypassFix: on("attention_ack_bypass_fix")`
- `server/attention_options.go` → `session.RuleOptions{AckBypassFix: f.AckBypassFix}`
- `review_queue_suppression.go:131` → `c.opts.AckBypassFix && isNeedsHumanReason(c.reason)`

The flag defaults to OFF. No restart required — it's read at decision time.

## Tests

Three existing tests in `session/review_queue_suppression_test.go` cover this behavior:

| Test (line) | Flag | Expected |
|---|---|---|
| `TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput` (258) | ON | InputRequired survives ack |
| `TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff` (270) | OFF | InputRequired removed (documents bug as expected) |
| `TestAckSnooze_Should_KeepApprovalPending_When_BypassFixOnOrOff` (278) | either | ApprovalPending always survives |

The flag matrix test (`review_queue_flag_matrix_test.go:38`) also captures:
> `wantNeedsHumanKept bool // InputRequired Medium survives an ack newer than the output`
This field is `false` in the `all_flags_off` row (the broken path).

## Other Suppression Rules: Not Affected

- **`grace`**: `session/review_queue_suppression.go:139-150` — only fires when `priority == PriorityLow` or `controllerActive == false`. InputRequired with an active controller skips grace entirely. No mis-categorization.
- **`readdLimit`**: Only suppresses re-adds to an already-queued item. Not relevant for the missing-item scenario.

## `ReasonWaitingForUser` Is NOT in `needsHumanReasons`

`ReasonWaitingForUser` exists (`session/queue/queue.go:26`) but is intentionally excluded from
`needsHumanReasons` (see comment at `review_queue_reason_table.go:38`: "distinct from
needsHumanReasons; that set decides ack-bypass, and includes ErrorState but not WaitingForUser").
Prose-question detection (PR B) will add it when the prose-question detector ships.

## Fix Options

### Option A: Flip `AckBypassFix` default to ON (recommended)
- Change: `feature_flags_attention.go` — the `on("attention_ack_bypass_fix")` function uses
  the registry config. Enable the flag globally in `~/.stapler-squad/config.json` or change
  the registration default in `feature_flag_specs.go` for the attention flags.
- Risk: Low. The flag was designed exactly for this scenario, tests pass with it on,
  and no special rollout mechanism is needed (no restart required per the flag description).

### Option B: Promote `InputRequired` to `PriorityHigh`
- Change: `review_queue_reason_table.go:18` — change `queue.PriorityMedium` to `queue.PriorityHigh`
- Risk: Medium. Changes the priority of InputRequired for all purposes (notification urgency,
  "Needs a Decision" tier ordering), not just ackSnooze bypass. Would need to update test expectations
  for the flag matrix test's `wantNeedsHumanKept` column.

### Option C: Remove the flag gate — make bypass unconditional for `needsHumanReasons`
- Change: `review_queue_suppression.go:131` — remove the `c.opts.AckBypassFix &&` guard
- Risk: Low for behavior, but loses the ability to roll back independently if a regression appears.
  The comment at line 115 notes "PR B" (prose-question detector) will add `WaitingForUser` to
  needsHumanReasons, so the set will grow — keeping the behavior gated makes sense until stable.

### Recommended: Option A
Flip the default in the feature flag registry. This is the lowest-risk path: the fix is already
implemented and tested, no logic change needed, and no restart required. The test
`TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff` documents the broken behavior
and should be updated to reflect the new default (or removed as it documents a flag-off edge case
that is no longer the user-visible default).

## Confidence: VERIFIED

All findings sourced from direct code reads. No inference without a file reference.
