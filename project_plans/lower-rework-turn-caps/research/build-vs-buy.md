# Build vs Buy: lower-rework-turn-caps

**Date**: 2026-10-07

## Summary

Strictly follow the existing `max_auto_rework_iterations` pattern. No external library
or new pattern is warranted. All config-layer work is already done.

---

## 1. Existing Pattern Reuse

The `max_auto_rework_iterations` field is fully wired across all five touchpoints. The
`autonomous_max_turns` config layer is **already complete** — only the settings API and
UI wire-up remain:

| Touchpoint | `max_auto_rework_iterations` state | `autonomous_max_turns` state |
|---|---|---|
| `config/config.go` field + `OrDefault()` | ✅ done | ✅ already done |
| `proto/session/v1/session.proto` `SessionDefaultsConfig` | ✅ field 10 | ❌ missing (next: field 15) |
| `proto/session/v1/session.proto` `UpdateGlobalDefaultsRequest` | ✅ field 8 | ❌ missing (next: field 13) |
| `server/services/defaults_service.go` read + write | ✅ done | ❌ missing |
| `web-app/src/components/settings/GlobalDefaultsForm.tsx` | ✅ done | ❌ missing |

**Verdict: copy the pattern exactly.** Deviating — e.g., grouping autonomous-driver
settings into a sub-message, or adding a separate RPC — would introduce new proto
message shapes for a one-field change that has an identical precedent. The existing
pattern handles the "0 means use server default" semantics cleanly via `OrDefault()`,
and the comment-based contract between proto and Go constant is already established.

**Proto field numbers to use** (verified from the message bodies at the time of
research):

- `SessionDefaultsConfig`: fields 1–14 are occupied (1–13 visible, `retry_policy` is
  field 14 as a message); `autonomous_max_turns` → **field 15**.
- `UpdateGlobalDefaultsRequest`: fields 1–12 are occupied (`retry_policy` is field 12);
  `autonomous_max_turns` → **field 13**.

Confirm with a grep before committing — another concurrent PR could claim a number.

---

## 2. OSS Library Assessment

**No external library adds value here.** The config layer is plain JSON (no Viper, no
Kong, no envconfig). The pattern is:

1. Add a field to a Go struct with a JSON tag.
2. Write an `OrDefault()` method with a named constant for the default and a ceiling.
3. Expose it via a proto field in a settings RPC.

This is already the simplest possible approach. Introducing a config library (Viper,
Kong, envconfig) to save this 15-line pattern would add a new import, new behavior
around env-var override precedence, and test complexity — all for a single int field.
The codebase deliberately uses plain JSON + `OrDefault()` helpers; this is the second
instance of that helper, not the first where a library could have prevented boilerplate.

---

## 3. LLM-Generated Proto Field Additions

**Risk: field number collision.** Protobuf field numbers are permanent — a collision
silently corrupts stored/transported data. LLM generators often infer field numbers by
incrementing the highest visible number in a context window that may be truncated,
missing reserved ranges, or missing fields from other messages in the same file.

**Mitigation already in place:**

- The requirements doc (`requirements.md`) explicitly lists field number constraints.
- The proto file must be re-read in full around each target message before assigning
  numbers.
- `make proto-gen` (buf) catches syntax errors and some schema issues at generation
  time; it does not catch semantic conflicts with already-serialized data.

**For this specific change the risk is low:** `SessionDefaultsConfig` and
`UpdateGlobalDefaultsRequest` are settings-RPC messages (request/response, not stored
on disk or in the database), so a field number error would surface immediately as a
compile or runtime decode error rather than silent data corruption. Still, verify
manually before running `make proto-gen`.

**Stale comment fix risk: none.** Updating a proto comment from "(3)" to "(5)" has no
wire-format impact and no generated-code impact. It is pure documentation; any text
editor or LLM can do it correctly.

---

## 4. Decision

**Build, following the existing pattern strictly.**

- Touchpoints: proto (2 messages) → `sessionDefaultsToProto` → `UpdateGlobalDefaults`
  handler → `GlobalDefaultsForm.tsx` state + render + save → tests.
- Config layer (`AutonomousMaxTurnsOrDefault`, constants, `defaults_test.go`) is
  already done and must not be touched.
- UI bounds: `min={1} max={200}` matching `autonomousMaxTurnsHardCeiling`.
- Comment fix: update "server default (3)" → "(5)" at `session.proto:2202` and
  `session.proto:2282` (both `max_auto_rework_iterations` comment lines).
- No new external dependencies. No new patterns.
