# Triage: Review Queue Empty for InputRequired Sessions

## Problem

The Review Queue "Needs a Decision" section is empty even when sessions with `InputRequired` status exist. The root cause is in `session/review_queue_suppression.go`'s `ackSnooze` rule:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
```

`InputRequired` sessions have `PriorityMedium` (3), which fails the `priority <= PriorityHigh` (2) check. After any prior acknowledgment, `ackedAfterOutput` is true and `bypass` is false, so `ackSnooze` returns `removeDecision("ackSnooze")` — silently filtering the session out of the queue.

## Fix

The `attention_ack_bypass_fix` feature flag already implements the correct logic: when enabled, `isNeedsHumanReason(c.reason)` causes `InputRequired`, `ApprovalPending`, and `ErrorState` sessions to bypass the ackSnooze filter regardless of priority. The flag defaults to OFF.

**One-line change:** Set `Default: true` on the `attention_ack_bypass_fix` spec in `server/services/feature_flag_specs.go` (line 320-323).

## Key Files

- `session/review_queue_suppression.go:131` — ackSnooze rule with the priority check
- `server/services/feature_flag_specs.go:320-323` — flag spec (add `Default: true`)
- `server/services/featureflags/registry.go:19` — `Spec.Default bool` field controls default
- `server/services/feature_flags_attention.go:40` — `AckBypassFix: on(attentionAckBypassFixFlagName)`

## Tasks

1. Add `Default: true` to the `attention_ack_bypass_fix` Spec in `feature_flag_specs.go`
2. Update the Description string to say "Default: on"
3. Update `config_surface_proof_test.go` snapshot if it records the default value
4. Run `go test ./session/... ./server/...` to verify no regressions
5. Manual smoke test: create an InputRequired session, acknowledge it, verify it stays in the Review Queue
