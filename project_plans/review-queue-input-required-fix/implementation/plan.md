# Implementation Plan: Review Queue Input-Required Fix

**Feature**: Flip `attention_ack_bypass_fix` flag default to ON so InputRequired sessions remain visible in "NEEDS A DECISION" after acknowledgment
**Date**: 2026-11-21
**Status**: Ready for implementation
**ADRs**: None

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Fix delivery | Flag default flip (`Spec.Default: true`) | Existing feature-flag infrastructure (`server/services/featureflags/registry.go`) | Raise `PriorityMedium` → `PriorityHigh` for InputRequired | Would not fix the bypass gap for the medium-priority case; changes queue-ordering semantics for all InputRequired sessions everywhere, not just the ackSnooze path; broader blast radius. |
| Fix delivery | Flag default flip | As above | Direct code edit — remove the priority guard from `ackSnooze` and always bypass for `isNeedsHumanReason` | Removes rollback path; equivalent to the flag already doing when ON; the flag buys a one-line revert if the spam concern from pitfalls.md proves real for some users. |

---

## Tech Debt Disposition

None identified.

---

## Migration Plan

N/A — complexity 1

## Observability Plan

N/A — complexity 1

## Risk Control

N/A — complexity 1

The flag's own mechanics provide the rollback: set `attention_ack_bypass_fix` to `false` via
`UpdateFeatureFlag` to restore the previous suppression behaviour with no restart needed.

## Unresolved Questions

None — root cause and fix are fully diagnosed. The one semantic change (acknowledge no longer
"snoozes" a genuinely-blocking session) is intentional and documented in pitfalls.md.

## Dependency Visualization

```
Task A (flag spec)  ──► Task B (test update)  ──► verify: make test
```

Tasks A and B touch different files; they may be done in either order, but both must be
complete before the test suite is run.

---

## Phase 1: Enable flag by default and update tests

### Epic 1.1: Flip `attention_ack_bypass_fix` to on-by-default

**Goal**: Make `InputRequired`, `ApprovalPending`, and `ErrorState` sessions bypass ackSnooze
after acknowledgment, so the Review Queue's "NEEDS A DECISION" section shows them again on the
next poll cycle rather than permanently suppressing them.

#### Story 1.1.1: Update flag spec

**As a** user with sessions waiting for input, **I want** those sessions to stay visible in the
Review Queue even after I have previously acknowledged them, **so that** I never miss a
blocking prompt.

**Acceptance Criteria**:
- `attention_ack_bypass_fix` resolves to `true` for every installation that has never
  explicitly overridden it.
  - *Given* a fresh stapler-squad instance with no persisted feature-flag config, *When* the
    Review Queue evaluates an `InputRequired` session that was previously acknowledged, *Then*
    the session appears in "NEEDS A DECISION" rather than being suppressed.
- The flag description says "Default: on."
  - *Given* `GET /api/feature-flags`, *When* the response lists `attention_ack_bypass_fix`,
    *Then* the description contains the text "Default: on."

**Files**: `server/services/feature_flag_specs.go`

##### Task 1.1.1a: Add `Default: true` and update description in feature_flag_specs.go

Lines 320–323 of `server/services/feature_flag_specs.go`:

- Change the struct literal for `attentionAckBypassFixFlagName` to add `Default: true`.
- Update the `Description` string from `"Default: off."` to `"Default: on."`.

Before:
```go
{
    Name:        attentionAckBypassFixFlagName,
    Description: "Attention ack-bypass fix: an acknowledged item no longer reappears or hides a newer question because of unrelated activity. Applies at once; global only. Default: off.",
},
```

After:
```go
{
    Name:        attentionAckBypassFixFlagName,
    Description: "Attention ack-bypass fix: an acknowledged item no longer reappears or hides a newer question because of unrelated activity. Applies at once; global only. Default: on.",
    Default:     true,
},
```

Files: `server/services/feature_flag_specs.go`

---

#### Story 1.1.2: Fix tests to reflect the new default

**As a** developer, **I want** the test suite to correctly assert the new on-by-default
behaviour, **so that** CI stays green and the tests serve as a live specification.

**Acceptance Criteria**:
- `TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfSixFlags`
  no longer asserts that `attentionAckBypassFixFlagName` defaults to false.
  - *Given* the test file is updated and `go test ./server/services/...` is run, *When* the
    test for `attention_ack_bypass_fix` executes, *Then* it asserts `Default: true` and
    `testFlagRegistry().Default(name) == true`.
- The baseline `AttentionFlags{}` check at line 37 is updated to reflect that `AckBypassFix`
  is now true.
  - *Given* a fresh registry with no config, *When* `LoadAttentionFlags(testFlagRegistry())`
    is called, *Then* it returns `AttentionFlags{AckBypassFix: true}`.
- The remaining five attention flags still assert `Default: false` and the test name reflects
  that only five flags remain off by default.

**Files**: `server/services/feature_flags_attention_test.go`

##### Task 1.1.2a: Split the loop and fix the AttentionFlags baseline assertion

In `server/services/feature_flags_attention_test.go`:

1. Remove `attentionAckBypassFixFlagName` from the `attentionFlagNames` slice (lines 12–19),
   or introduce a separate slice for the five still-off flags and keep `attentionFlagNames` as
   the full list for other uses. The simplest approach: define a new `attentionFlagsOffByDefault`
   slice containing the five non-AckBypassFix names, and use that slice in the existing loop.

2. Rename `TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfSixFlags`
   to `TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfFiveFlags`
   (or a similarly accurate name, since only five flags are now off by default).

3. Add a new sub-test (or standalone test) for `attentionAckBypassFixFlagName` asserting
   `require.True(t, f.Default)` and `require.True(t, testFlagRegistry().Default(name))`.

4. Fix line 37's `AttentionFlags{}` baseline:
   ```go
   // Before
   require.Equal(t, AttentionFlags{}, LoadAttentionFlags(testFlagRegistry()))
   // After
   require.Equal(t, AttentionFlags{AckBypassFix: true}, LoadAttentionFlags(testFlagRegistry()))
   ```

Files: `server/services/feature_flags_attention_test.go`

##### Task 1.1.2b: Fix three additional test files that assert the old default

Adversarial review identified three more files that assert `AckBypassFix` is false by default
and will fail CI after the flag flip:

1. **`server/services/feature_flags_reader_injected_registry_test.go:33`** — asserts
   `AttentionFlags{}` for the default registry (all six flags off). Update to
   `AttentionFlags{AckBypassFix: true}`.

2. **`server/attention_options_injected_test.go:23`** — `assert.False(t, r.AckBypassFix)` on
   `services.FlagRegistry()` with no override. Change to `assert.True(t, r.AckBypassFix)`.

3. **`server/flag_registry_wiring_test.go:30–35`** — calls `flipFlagDefault("attention_ack_bypass_fix")`
   which currently flips false→true, then asserts `true`. After the flag is on by default,
   `flipFlagDefault` will flip true→false; update the assertion to `false`, or update
   `flipFlagDefault` to toggle from whatever the current default is.

Files: `server/services/feature_flags_reader_injected_registry_test.go`,
`server/attention_options_injected_test.go`, `server/flag_registry_wiring_test.go`

##### Task 1.1.2c: Run tests and confirm green

```bash
go test ./server/services/... -run TestGetFeatureFlag -v
go test ./server/... -v 2>&1 | tail -30
```

Both must pass with no failures before marking the work done.

Files: (read-only verification step)
