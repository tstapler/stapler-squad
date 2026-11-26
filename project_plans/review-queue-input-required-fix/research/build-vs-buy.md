# Build vs Buy: Review Queue Ack-Bypass Fix

## Context

The `ackSnooze` rule at `session/review_queue_suppression.go:131` suppresses `InputRequired`
sessions from the "NEEDS A DECISION" section after acknowledgment. The condition is:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
```

`InputRequired` maps to `PriorityMedium` (3) in `session/review_queue_reason_table.go:18`.
`PriorityHigh` is 2. Since `PriorityMedium > PriorityHigh` numerically, the left side of the
`||` is false, and the right side only fires when `AckBypassFix` is true. With the flag off
(the current default), an acknowledged `InputRequired` session is suppressed even if it
still needs user attention.

## Approaches Evaluated

### Option 1: Flip `attention_ack_bypass_fix` default to true (flag default flip)

The flag already exists. Its `Spec` in `server/services/feature_flag_specs.go:321` has
`Default: false`. Changing it to `Default: true` is a one-line edit:

```go
{
    Name:        attentionAckBypassFixFlagName,
    Default:     true,     // was false
    Description: "...",
},
```

The `featureflags.Registry.Enabled` method falls back to `Spec.Default` when the config has no
explicit value (`registry.go:76`), so the flip takes effect on the next config read without a
restart. Users who already explicitly set the flag to `false` in their config retain that
override.

**Risk:** Low. `AckBypassFix: true` is exercised by `review_queue_flag_matrix_test.go` (the
`bypass` and `bypassDetect` matrix variants) and `review_queue_suppression_test.go`. The flag's
test coverage path is fully separate from the `<= PriorityHigh` path, so no existing test breaks.
The flag description ("an acknowledged item no longer reappears or hides a newer question") already
describes the desired steady-state behavior.

### Option 2: Change the priority condition (`<= PriorityHigh` → `<= PriorityMedium`)

Replace the numeric boundary:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityMedium || ...)
```

This would let Medium-priority sessions bypass ackSnooze unconditionally — including future
reasons at PriorityMedium that are not needs-human. The `statusRules` table assigns
`PriorityMedium` only to `InputRequired` today, but that coupling is implicit. The change is
broader than the intent (fix InputRequired bypass) and cannot be rolled back without a code
deploy. It also eliminates the reason-based discrimination that `isNeedsHumanReason` exists
to provide.

**Risk:** Medium. Semantically correct for now, but fragile: a future `PriorityMedium` reason
that is not human-blocking would inherit the bypass silently.

### Option 3: Promote `InputRequired` to `PriorityHigh`

Change `review_queue_reason_table.go:18`:

```go
detection.StatusInputRequired: {queue.ReasonInputRequired, queue.PriorityHigh, "..."},
```

The `statusRules` comment at `queue.go:56–57` documents the design intent:
`PriorityHigh = Approval dialogs`, `PriorityMedium = Input requests`. Promoting InputRequired
to High conflates these two distinct signals and changes how the item sorts in the queue UI
relative to test failures (`PriorityHigh`) and approvals (also `PriorityHigh`). The golden tests
in `review_queue_determiner_golden_test.go` would need updating. And unlike the flag flip, the
priority change affects the queue ordering for all users immediately, with no opt-out.

There is also a semantic argument against it: an approval dialog blocks the agent entirely;
an input request is lighter (the agent might be waiting for a name or a path). Flattening
them to the same priority discards that distinction permanently.

**Risk:** Medium. Correct fix for the ackSnooze bug but causes collateral sorting changes and
breaks the priority taxonomy the queue was designed around.

### Option 4: Remove ackSnooze for needs-human reasons entirely

Delete the ackSnooze bypass condition and instead exempt `isNeedsHumanReason` from ackSnooze
altogether:

```go
func ackSnooze(c ruleCtx) (SuppressionDecision, bool) {
    if isNeedsHumanReason(c.reason) {
        return SuppressionDecision{}, false   // never snooze needs-human items
    }
    bypass := c.shouldAdd() && c.priority <= queuepkg.PriorityHigh
    ...
}
```

This is semantically cleaner: the ack mechanism exists to suppress stale items, but a
needs-human item is never stale until the human actually responds. However, this is a larger
behavioral change than the flag flip — it removes suppression entirely rather than gating it
on a reason-based bypass. The existing `AckBypassFix` path already does this gating, just
under a flag. Removing the flag and always applying the bypass is equivalent to the flag flip
(Option 1) but without the ability to roll back via config.

**Risk:** Low-to-medium as a behavior change, but higher operational risk than Option 1 because
it is not reversible without a code deploy.

### Option 5: OSS notification/snooze management library

No OSS library addresses this problem. The ack-snooze pattern here is tightly coupled to the
review queue's data model (detected status → priority → suppression rule chain), session-level
acknowledgment state (`IsAcknowledgedAfterOutput`), and the queue's episode/escalation semantics.
A generic snooze library (e.g., a job scheduler) would solve a different problem. There is nothing
to buy.

## Recommendation

**Flip the `attention_ack_bypass_fix` flag default to `true`** (Option 1).

Rationale:
- The fix is already correctly implemented and tested behind the flag; the flag was clearly
  intended as a soak gate, not a permanent off-by-default toggle (its description says
  "acknowledged item no longer reappears or hides a newer question", which is the desired
  steady-state).
- The flag infrastructure provides a clean rollback path: a user who hits an unexpected
  regression can set `attention_ack_bypass_fix: false` in their config without waiting for a
  code deploy.
- The change is a one-line edit to `Spec.Default`. No logic changes, no test updates, no
  priority taxonomy disruption.
- The flag is already exercised by the `bypass` and `bypassDetect` matrix variants in
  `review_queue_flag_matrix_test.go`, so the behavior under `true` is well-covered.

## Concerns about the flag infrastructure itself

The flag infrastructure is sound. `featureflags.Registry` is immutable and panic-tested on
duplicate/empty names at startup (`registry.go:33–44`). The `Enabled` fallback to `Spec.Default`
is a single line with no branching. The `attention_ack_bypass_fix` flag is registered in the
`coreFlagSpecTable` alongside five peers, all sharing the same read path. No concerns.

One minor note: the flag description in `feature_flag_specs.go:322` says "Default: off" and
should be updated to "Default: on" as part of the flip to avoid misleading anyone who reads it.
