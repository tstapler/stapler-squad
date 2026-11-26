# Adversarial Review: Review Queue Input-Required Fix

**Date**: 2026-11-21
**Verdict**: CONCERNS

## Blockers

None.

## Concerns

- [ ] **Plan lists only one test file but three more will fail on CI** — The plan identifies
  `server/services/feature_flags_attention_test.go` as the only file needing updates, but
  three additional test files will break as written once `Default: true` lands:

  1. `server/services/feature_flags_reader_injected_registry_test.go:33` — asserts
     `assert.Equal(t, AttentionFlags{}, LoadAttentionFlags(testFlagRegistry()), "default registry: all six default off")`.
     After the flip, `testFlagRegistry()` will return `AckBypassFix: true`, so the assertion
     comparing against the zero value fails.

  2. `server/attention_options_injected_test.go:23` — asserts
     `assert.False(t, r.AckBypassFix)` on a call that uses `services.FlagRegistry()` (the
     global registry) with no persisted override. After the flip that registry defaults
     `AckBypassFix` to `true`, so the assertion is wrong.

  3. `server/flag_registry_wiring_test.go:30-35` — calls
     `flipFlagDefault("attention_ack_bypass_fix")` (currently flips `false` → `true`) then
     asserts `assert.True(t, rules.AckBypassFix, ...)`. After the fix the flag defaults to
     `true`, so `flipFlagDefault` produces `false`, and `assert.True` fails. The test body
     would need to be restructured — either invert the assertion, or test a different flag
     with a known-false default and separately add an `AckBypassFix`-specific proof.

  **Recommendation**: Add explicit update tasks for all three files. The existing Task 1.1.2b
  ("run tests and confirm green") would catch these at runtime, but the plan as written would
  leave an implementer surprised by red CI on files they weren't told to touch.

## Minors

- **Semantic UX change (no "snooze blocking session" path) has no release-note task** —
  The pitfalls.md correctly documents that after the fix, an `InputRequired` session
  acknowledged by the user will immediately reappear on the next 2-second poll cycle.
  This breaks the previously-working "ack to temporarily dismiss" flow. The plan has no
  corresponding PR-description or release-note task to surface this to users on upgrade.
  Low impact (the fix is deliberate, not a defect), but users who relied on the ack-dismiss
  behaviour will be surprised.

- **`readdLimit` edge-case verification missing from task list** — pitfalls.md edge case 2
  calls for a manual check of the `ackSnooze bypass + readdLimit suppress` interaction to
  confirm "suppress" means "do not change the queue" (not "remove"). No plan task captures
  this. Low risk (the semantics are tested elsewhere), but the pitfalls author considered it
  worth verifying manually, and that intent was not carried forward.
