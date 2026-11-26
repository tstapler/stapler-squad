# Validation: Fix Review Queue "Needs a Decision" Empty

**Item ID:** b41b6253-386b-4998-b5a9-5a19dcc5d1ca  
**Project:** fix-review-queue-empty

## Acceptance Criteria Validation

### AC1 — InputRequired re-appears after prior acknowledgment
**Method:** Unit test `TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput`
(`session/review_queue_suppression_test.go:258`) already verifies this with `AckBypassFix=true`.
Once the flag defaults to ON, this is the live behavior.

**Live verification:** In a running instance, trigger a session that asks a question → dismiss it
in the review queue → confirm the same session remains in "Needs a Decision" if the prompt is
still present (not in the terminal, in another browser tab). With the bug, it disappears; with
the fix, it stays.

**Status:** Test exists and passes with flag ON. VERIFIED.

### AC2 — ApprovalPending re-appears after prior acknowledgment
**Method:** `TestAckSnooze_Should_KeepApprovalPending_When_BypassFixOnOrOff` (line 278) already
verifies this for BOTH flag states, since `ApprovalPending` has `PriorityHigh=2 <= PriorityHigh=2`.
No change needed here.

**Status:** VERIFIED — not affected by this change, already correct.

### AC3 — ackSnooze still suppresses Idle/Stale/Complete (no regression)
**Method:** The flag matrix test row `alone_all_flags_off` → `wantNothing` captures the
all-flags-off scenario. With `AckBypassFix=true`, only `needsHumanReasons` escape suppression;
`PriorityLow` items (Idle, Success) still get removed via ackSnooze when
`priority <= PriorityHigh` is false for them too (they are Low=4, so `4 <= 2 = false`), BUT
they also fail `isNeedsHumanReason()`. So they remain suppressable.

Verify: check that the flag matrix test row for `alone_ackBypass` has `wantIdle: false`
(Idle items do not appear even with the flag on).

**Status:** PLAUSIBLE — the matrix test already differentiates the `wantNeedsHumanKept` column
(InputRequired) from other columns. Confirm the matrix row with AckBypassFix=true does not
set `wantIdle=true`.

### AC4 — Fix applies without manual flag toggle
**Method:** Confirm `defaultOn("attention_ack_bypass_fix")` returns `true` when the key is
absent from `~/.stapler-squad/config.json`. Write a unit test for the helper:
```go
func TestDefaultOnFlag_ReturnsTrue_WhenKeyAbsent(t *testing.T) {
    // set up an empty registry config
    // call defaultOn("attention_ack_bypass_fix")
    // assert result == true
}
```

**Status:** Requires the `defaultOn` helper to be implemented. Verify helper exists or add it.

### AC5 — Existing tests pass; new regression test covers broken path
**Method:**
1. `make test` passes after the flag-default change
2. `TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff` (renamed) still passes with
   explicit `AckBypassFix: false` — confirms the opt-out path works
3. The flag matrix test passes without modification

**Status:** Requires running `make test` after the change.

## Rollout Validation

**Before deploying:**
1. `make quick-check` (build + test + lint)
2. Run the flag matrix test in isolation: `go test ./session/... -run TestReviewQueue` 
3. Confirm no test sets `AckBypassFix` implicitly (zero value) and expects `InputRequired`
   to be kept — that would be a test bug, since the zero value is `false`

**After deploying:**
1. Set `attention_ack_bypass_fix: false` in config → verify rollback works (InputRequired
   silently removed again)
2. Remove the override → verify default-on behavior restored

## Risks Not Covered by Tests

1. **Content-signature dedup timing:** `IsAcknowledgedAfterOutput()` reads
   `lastMeaningfulOutputNs` atomically (`session/domain/review_state.go:158`). If the
   dedup runs before the ack is recorded, `ackedAfterOutput` could be `false` briefly,
   causing a double-add. The `readdLimit` rule (2-minute window) absorbs this.

2. **Large number of InputRequired sessions:** With bypass ON, the queue may surface
   more items concurrently than users expect. This is correct behavior — the fix restores
   visibility of blocked sessions. No action needed.

3. **`ReasonWaitingForUser` still suppressable:** Prose questions (not yet detected via
   `attention_prose_question_detection`) will still be suppressed by ackSnooze. This is
   intentional and tracked for PR B.

## Validation Summary

| AC | Verification Method | Status |
|---|---|---|
| AC1: InputRequired re-appears after ack | Existing unit test + live check | VERIFIED (test) / Needs live |
| AC2: ApprovalPending re-appears after ack | Existing unit test (flag-independent) | VERIFIED |
| AC3: Idle/Stale not regressed | Flag matrix test | PLAUSIBLE — confirm wantIdle column |
| AC4: No manual toggle needed | New unit test for defaultOn helper | PENDING |
| AC5: Tests pass, regression test exists | make test + renamed test | PENDING |
