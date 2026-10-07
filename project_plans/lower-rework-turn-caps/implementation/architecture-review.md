# Architecture Review: lower-rework-turn-caps / expose `AutonomousMaxTurns`

**Reviewer**: Architecture Review Subagent
**Date**: 2026-10-07
**Plan**: `project_plans/lower-rework-turn-caps/implementation/plan.md`
**Verdict**: APPROVED — 0 blockers, 3 concerns

---

## ADR-000 check

`docs/adr/ADR-000-architecture-constitution.md` does not exist. No hard-constraint baseline
applies beyond the project's general architecture rules.

---

## Lens 1 — Structural integrity

**All SOLID checks pass.**

- **SRP**: `sessionDefaultsToProto` stays the single read-side mapper; `UpdateGlobalDefaults`
  stays the single write-side mutator. One line added to each. No new abstraction introduced.

- **OCP**: proto messages are extended by append (field 15 / field 13). Wire-format stability
  is preserved; existing clients do not break.

- **DIP**: plan correctly keeps `AutonomousMaxTurns` out of `sharedBacklogCfg` propagation.
  The distinction is architecture-load-bearing: `MaxAutoReworkIterations` and
  `MaxConcurrentBacklogWorkItems` must be pushed live because `BacklogService` reads them from a
  shared pointer that never re-calls `config.LoadConfig()`; `AutonomousMaxTurns` is consumed at
  driver-start time via a fresh `config.LoadConfig()`, so propagation is unnecessary and
  correctly omitted (ADR-001 §Consequences, verified against
  `server/services/defaults_service.go:186-190`).

- **Layer coupling**: proto → generated Go/TS → `defaults_service.go` → `GlobalDefaultsForm.tsx`.
  No layer-crossing shortcuts.

- **Testability**: `newIsolatedDefaultsService` in `defaults_service_test.go` gives each test
  its own config directory; plan correctly reuses it for the two new test functions.

---

## Lens 2 — Type-level design

### Concern C1 (low): Unexported default constant creates magic literal debt

`autonomousMaxTurnsDefault = 30` (`config/config.go:1077`) is unexported, while the analogous
`DefaultMaxAutoReworkIterations = 5` (`config/config.go:1059`) is exported. The plan
acknowledges this and falls back to `int32(30)` in the test body and `|| 30` in the React load
path. This produces two magic literals for the same value:

- `defaults_service_test.go` (proposed): `assert.Equal(t, int32(30), resp.Msg.Defaults.AutonomousMaxTurns)`
- `GlobalDefaultsForm.tsx` (proposed): `setAutonomousMaxTurns(defaults.autonomousMaxTurns || 30)`

Neither reference `config.AutonomousMaxTurnsOrDefault()` or a named constant. If the default
changes from 30, a failing test is guaranteed, but the test message will be cryptic (`want 30,
got 40`) with no self-documentation of why 30 was the expected value.

**Recommended fix** (small, contained): export the constant before the test is written:
```go
// DefaultAutonomousMaxTurns is the per-session turn cap applied when unset.
const DefaultAutonomousMaxTurns = 30
```
Then tests can use `int32(config.DefaultAutonomousMaxTurns)` — matching the `DefaultMaxAutoReworkIterations` precedent at `defaults_service_test.go:218`.

The existing test for rework iterations
(`TestGetSessionDefaults_ServesRuntimeReworkCapDefault`) uses this form; the new test
should too, not a magic literal.

**Not a blocker**: the plan's fallback `int32(30)` is correct and will pass CI. This is
accepted technical debt if the constant remains unexported.

### Concern C2 (low): `parseInt` vs `Number` inconsistency in React `onChange`

The plan proposes `parseInt(e.target.value, 10) || 1` for the new field (justified in
pitfalls.md: rejects decimal input). The existing `maxAutoReworkIterations` field in the same
form uses `Number(e.target.value) || 1` (`GlobalDefaultsForm.tsx:323`). The plan creates an
inconsistency within the same component without proposing to harmonize the old field.

This is not a bug — `parseInt` is strictly better for integer fields. However, if the old
field is not updated in the same PR, readers will see two different patterns in the same
`onChange` block for structurally identical fields.

**Recommended fix** (optional, 1 line): update `maxAutoReworkIterations`'s `onChange` to
also use `parseInt(..., 10)` in the same commit. Not required; acceptable to defer.

### `loadDefaults` fallback consistency

`setAutonomousMaxTurns(defaults.autonomousMaxTurns || 30)` uses a hardcoded fallback.
The analogous `setMaxAutoReworkIterations(defaults.maxAutoReworkIterations)` at line 62
has no fallback. However, `defaults.maxConcurrentBacklogWorkItems || 2` at line 63 does.
The inconsistency pre-exists; the plan matches the `maxConcurrentBacklogWorkItems` pattern
rather than the `maxAutoReworkIterations` pattern. Both are safe because the server always
returns a resolved, non-zero value from `AutonomousMaxTurnsOrDefault()`. No action required.

---

## Lens 3 — Pattern selection

**Pattern choice is sound.** "Mirror `max_auto_rework_iterations` end-to-end" is the correct
call for this size of change. Alternative B (generic mapper) and Alternative C (separate
endpoint) were correctly rejected.

### Concern C3 (cosmetic): Struct literal insertion order mismatches proto field order

**Task 2.1.1a** instructs insertion "at line 553 (after `StaleSessionThresholdMinutes`)".
At that location the struct literal is:

```go
StaleSessionThresholdMinutes: int32(cfg.StaleSession.ThresholdMinutesOrDefault()),  // line 553
StaleSessionNotifyEnabled:    cfg.StaleSession.NotifyEnabledOrDefault(),             // line 554
RetryPolicy: &sessionv1.RetryPolicyConfig{...},                                     // lines 555–566
```

The plan's insertion point puts `AutonomousMaxTurns` *before* `StaleSessionNotifyEnabled` and
`RetryPolicy` in the struct literal, even though its proto field number (15) comes *after*
`retry_policy = 14`. Go struct literal field order has no semantic effect, but reviewers who
cross-reference proto field numbers against struct literal order will see a mismatch.

**Preferred insertion point**: after the closing `},` of the `RetryPolicy` block (after line
566), matching proto field order.

**Not a blocker**: Go compiles and behaves identically either way.

---

## Lens 4 — Tech debt trajectory

| Item | Plan's disposition | Assessment |
|------|-------------------|------------|
| `defaults_service.go` | Extend as-is | Correct. Two mechanical lines; well within cyclomatic gate. |
| Stale `"server default (3)"` proto comments (lines 2201, 2282) | Fix in same PR | Correct. Verified: both comments say "(3)" while `DefaultMaxAutoReworkIterations = 5`. |
| `backlog_service.go:950` stale `"default 3"` comment | Fix in same PR | Correct. Verified the comment at line 950. |
| Unexported `autonomousMaxTurnsDefault` | Not addressed (fallback to literal) | New debt created. See C1 above. Low risk; easy fix. |

Debt scope is honest and sized correctly. No latent architectural risk is introduced.

---

## Summary

| # | Lens | Severity | Description |
|---|------|----------|-------------|
| C1 | Type-level | Low | `autonomousMaxTurnsDefault` is unexported; plan falls back to magic literal `int32(30)` in tests and React load path. Export the constant to match `DefaultMaxAutoReworkIterations` precedent. |
| C2 | Pattern | Low | New field uses `parseInt(..., 10)` in `onChange`; existing adjacent field uses `Number(...)`. Not a bug; consider harmonizing the old field in the same PR. |
| C3 | Structural | Cosmetic | `sessionDefaultsToProto` insertion point (after `StaleSessionThresholdMinutes`) places the new Go field before `RetryPolicy` in the struct literal, opposite to proto field ordering. Insert after the `RetryPolicy` block instead. |

**No blockers.** The plan is architecturally sound and safe to implement as written; the three
concerns are improvements, not gates.
