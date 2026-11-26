# Plan: Fix Review Queue "Needs a Decision" Empty

**Item ID:** b41b6253-386b-4998-b5a9-5a19dcc5d1ca  
**Project:** fix-review-queue-empty

## Chosen Approach: Flip `AckBypassFix` default to ON

The fix is already implemented; the only change needed is enabling the
`attention_ack_bypass_fix` feature flag globally by default. No new logic,
no behavior change for other priorities, no restart needed.

## Implementation Steps

### Step 1 — Enable the flag default in `feature_flags_attention.go`

The attention flags are registered in `server/services/feature_flags_attention.go` via
the `AttentionFlags` struct, backed by `feature_flag_specs.go`.

Change the registration so `attention_ack_bypass_fix` is on by default. The pattern used
by other graduated flags (`probe_program`, `changelog`, `restart_with_summary`) is to
update the description in `feature_flag_specs.go` to say "Default: on" and then flip the
backing default in the registration.

Concretely:

**`server/services/feature_flags_attention.go`** — no code change needed; the flag is
already wired via `on("attention_ack_bypass_fix")` which reads from the registry.

**`server/services/feature_flag_specs.go`** — no entry for attention flags (they live in
`feature_flags_attention.go`). The attention flags have their own registration loop.

The simplest path: in `feature_flags_attention.go`, change the default from `off` to `on`
using the `defaultOn` helper (or equivalent) so a missing config key reads as `true`.
Verify the helper exists: search for `defaultOn` or `onWithDefault` in the services package.

If no `defaultOn` helper exists, add one mirroring the existing `on()`:
```go
func defaultOn(name string) bool {
    flags, err := registryflags.ReadFlags()
    if err != nil { return true } // fail-open to the default
    v, ok := flags[name]
    if !ok { return true }        // key absent → default ON
    return v
}
```
Then change line 40:
```go
// Before:
AckBypassFix: on(attentionAckBypassFixFlagName),
// After:
AckBypassFix: defaultOn(attentionAckBypassFixFlagName),
```

### Step 2 — Update the test that documents the broken behavior

`session/review_queue_suppression_test.go:270`:
`TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff`

This test explicitly tests `AckBypassFix=false` → item removed. With the new default ON,
this test becomes a flag-off regression guard, which is still valid. Rename it for clarity:
`TestAckSnooze_Should_RemoveInputRequired_When_BypassFixExplicitlyOff`

No behavior change to the test body — it passes `AckBypassFix: false` explicitly, so it
still documents (and guards) the opt-out behavior.

### Step 3 — Add a default-behavior integration test (optional but recommended)

Add a test that uses `RuleOptions{}` (zero value, all flags false) to verify that
`InputRequired` with an ack is still suppressed at the zero value — this guards against
accidentally changing the struct default — and a test using the new live-path default
(flag key absent from config) verifying it reads as ON.

### Step 4 — Update the flag description

In the attention-flags registration (or wherever the description lives), update:
> "Default: off."  →  "Default: on."

## Files to Change

| File | Change |
|---|---|
| `server/services/feature_flags_attention.go` | Use `defaultOn` helper for `AckBypassFix` |
| `session/review_queue_suppression_test.go` | Rename `TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff` for clarity |
| Flag description string | Update "Default: off" → "Default: on" |

## Files NOT to Change

| File | Why |
|---|---|
| `session/review_queue_suppression.go` | Logic is correct with flag ON; no change needed |
| `session/review_queue_reason_table.go` | Priority assignment is intentional; InputRequired staying at Medium is fine once bypass is on |
| `web-app/` | Frontend behavior unchanged; "Needs a Decision" tier already correct |

## Risk Assessment

**Low risk.** The fix:
- Was already designed, implemented, and tested in `bdb0d38d0`
- Requires no restart (flag read at decision time)
- Has no effect on `PriorityLow` items (ackSnooze never fires for removes, only adds)
- Has no effect on `ApprovalPending`/`ErrorState` (they already bypass via `priority <= PriorityHigh`)
- Does not change the grace or readdLimit rules
- Can be rolled back by setting `attention_ack_bypass_fix: false` in config.json without deploying

**One edge case to verify:** A user who intentionally dismissed an `InputRequired` prompt
(to defer it) will now see it re-appear if the prompt is still live. This is the _desired_
behavior — the bug was that live blocking prompts were being hidden. The ackSnooze intent
is to suppress idle notifications after acknowledgment, not blocking human-required actions.

## Adversarial Review

**Concern: Does flipping the default break the notification suppression design?**

No. `ackSnooze` was designed to suppress low-priority noise after acknowledgment
(the user has seen it and moved on). Its comment at line 126-129 explicitly states:
> "Active high-priority adds bypass it: content-signature dedup leaves LastMeaningfulOutput
> unchanged when an identical prompt reappears, so the ack would otherwise hide a live
> blocking prompt."

`InputRequired` IS a live blocking prompt — the design intent always was to bypass
ackSnooze for it. The bug is that `PriorityMedium=3` fell below the `<= PriorityHigh=2`
threshold despite being a needs-human reason.

**Concern: Will this cause notification spam for sessions that keep looping?**

The `readdLimit` rule (2-minute minimum re-add interval) still applies and prevents spam.
`ackSnooze` bypass does not bypass `readdLimit`. A session that keeps emitting the same
prompt will re-appear at most once per 2 minutes.

**Concern: Are there other needsHumanReasons that should stay suppressable?**

`ReasonWaitingForUser` (prose questions) is intentionally NOT in `needsHumanReasons` yet —
it arrives in PR B with the prose-question detector. The plan does not change this.

## Definition of Done

1. `attention_ack_bypass_fix` is on by default without manual config override
2. `TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput` passes (already does)
3. `TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff` (renamed) still passes with explicit `AckBypassFix: false`
4. `make test` passes; `make lint` passes
5. Live verification: acknowledge an `InputRequired` session in the review queue → it re-appears when the prompt is still live
