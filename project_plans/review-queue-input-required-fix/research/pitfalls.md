# Pitfalls: Enabling `attention_ack_bypass_fix` by Default

## What the flag does

`ackSnooze` in `session/review_queue_suppression.go` removes items the user already
acknowledged. Without the flag, the bypass only triggers for `PriorityHigh` (≤2). An
`InputRequired` prompt is `PriorityMedium` (3), so an ack newer than the last output
silently kills a live blocking prompt — the ":901 gap" named in the chain test.

With the flag on:

```go
bypass := c.shouldAdd() && (c.priority <= queuepkg.PriorityHigh ||
    (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
```

`isNeedsHumanReason` is true for `ReasonApprovalPending`, `ReasonInputRequired`, and
`ReasonErrorState`. When the bypass is active, `ackSnooze` does **not** suppress the
item and it reaches the queue — so a live blocking prompt stays visible even after
the user clicked "acknowledge."

## Key risk: will this cause notification spam for legitimately-idle sessions?

**Low risk, with one narrow caveat.**

The bypass only fires when `c.shouldAdd()` is true (the determiner returned
`DetectionActionAdd` this cycle), _and_ the reason is in `needsHumanReasons`. A session
that has already returned to Idle or Stopped will produce `DetectionActionRemove` or
`DetectionActionSkip`, so `ackSnooze` never triggers and the bypass path is never
reached. The spam concern is moot for sessions that are no longer blocked.

The narrow caveat: if the agent is still in `InputRequired` / `ApprovalPending` /
`ErrorState` and the user has acked it (clicked it away), the item will now reappear on
every 2-second poll cycle rather than staying suppressed. The `readdLimit` rule
(`minReAddInterval = 2 min`) does **not** catch this because it only suppresses when the
item is _already in the queue_ from a prior poll. This means the item can reappear
immediately after the user dismisses it, as long as the session is still in a blocking
state. That is precisely the bug being fixed — the session is _genuinely_ waiting for
the user — so this is correct behavior, not spam.

The one scenario where it could feel spammy: a session that is stuck in `InputRequired`
but the user has intentionally chosen to leave it alone for now. With the flag on, that
session stays permanently visible in the NEEDS A DECISION section. Without the flag, the
user could make it disappear by acknowledging it. After the fix, there is no "snooze this
blocking session" path: acknowledge only removes items for non-blocking reasons
(`ReasonWaitingForUser`, `ReasonIdle`, etc.). This is a deliberate semantic change, not
a defect, but it changes user expectations and may surface complaints from users who
relied on ack as a "dismiss temporarily" for input-required sessions.

## What tests currently cover this logic

### Unit tests — `session/review_queue_suppression_test.go`

- `TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput`: verifies
  all three `needsHumanReasons` at `PriorityMedium` are **not** snoozed when the flag is
  on; also verifies `ReasonWaitingForUser` **is** snoozed (not in the bypass set).
- `TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff`: pins the pre-fix
  behavior so a regression restores it.
- `TestAckSnooze_Should_KeepApprovalPending_When_BypassFixOnOrOff`: verifies that
  `PriorityHigh`/`PriorityUrgent` items were always kept regardless of flag value.
- `TestNeedsHumanReasons_Should_ListBlockingReasons_When_Queried`: locks the exact
  membership of `needsHumanReasons`.

### Chain tests — `session/review_queue_poller_chain_test.go`

- `medium_add_ack_after_output_is_snoozed`: confirms the bug without the flag (kept for
  regression detection).
- `medium_add_ack_after_output_is_kept_when_bypass_fix_on`: confirms the fix.
- `medium_non_human_add_ack_after_output_is_snoozed_when_bypass_fix_on`: confirms
  `ReasonWaitingForUser` is not in the bypass set even with the flag on.
- `high_add_ack_after_output_bypasses_snooze_when_bypass_fix_on`: no regression for
  already-working High path when flag is on.

### Flag matrix test — `session/review_queue_flag_matrix_test.go`

- `alone_ackBypassFix`: confirms InputRequired survives ack with only this flag on.
- `order1_idleClock_then_detection_then_ackBypass`, `order2_ackBypass_before_detection`:
  multi-stage flag-flip harness confirming the fix holds across flag combination orders.
- `TestFlagMatrix_Should_MatchAllOffOutcome_When_InertFlagsOnOverCorpus`: corpus sweep
  confirming inert flags don't change outcomes when AckBypassFix is off.

### Wiring tests — `server/attention_options_test.go`, `server/flag_registry_wiring_test.go`

- Verify `AckBypassFix` flows correctly from the flag registry through
  `attentionFlagsCache` → `RuleOptions` → `suppressionPolicy`.

Coverage is comprehensive for the happy and regression paths. No test currently exercises
the scenario of a user acknowledging an InputRequired session **repeatedly** to test for
reappearance rate — that edge is covered implicitly by `readdLimit` (2 min cooldown when
already queued), but the spam behavior when the item was just removed then re-added in the
same cycle is not explicitly exercised.

## Edge cases to verify manually after the change

1. **Ack-then-immediate-reappear UX**: with the flag on by default, clicking the
   "acknowledge" button on an `InputRequired` session should cause the item to reappear
   within 2 seconds (next poll cycle), since `ackSnooze` no longer hides it. Confirm
   this is the intended UX and that the UI clearly communicates the session is waiting
   for the user.

2. **`readdLimit` interaction**: if the item is already in the queue and the user acks
   it, `ackSnooze` bypasses → `readdLimit` runs next and suppresses re-add for 2 minutes
   because `LastAddedToQueue` is recent. Verify the item stays visible in this case
   (ackSnooze bypass prevents removal, so the queue keeps the existing item unchanged,
   and `readdLimit` suppresses a re-add — net effect is the item stays). Check
   `suppressionKind: suppressionSuppress` vs. `suppressionRemove` semantics to confirm
   "suppress" means "do not change the queue," not "remove."

3. **`ErrorState` sessions**: `ReasonErrorState` is in `needsHumanReasons`. A session
   that errored and was acked by the user will now stay in NEEDS A DECISION. Verify this
   is correct — an errored session that the user dismissed and that is still in
   `ErrorState` should indeed stay visible.

4. **Flag flip mid-soak**: because the flag is read once per poll cycle and cached for
   5 seconds, flipping from default-on to off (via config) will take effect within one
   TTL. Verify that sessions that were visible under the new default correctly disappear
   after a flip to off if the conditions for ackSnooze apply.

5. **Existing installs with sessions already acked**: on upgrade, all sessions currently
   in `IsAcknowledgedAfterOutput() == true && reason in needsHumanReasons` will
   reappear in NEEDS A DECISION on the first poll after the flag becomes default-on. This
   is not a bug (they were hidden due to the bug), but it may surprise users who see
   sessions they thought they'd dismissed return to the queue. No migration is needed,
   but it's worth calling out in the release notes.
