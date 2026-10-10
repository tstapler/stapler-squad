# Adversarial Review: lower-rework-turn-caps

**Reviewer**: adversarial architecture agent  
**Date**: 2026-10-07  
**Verdict**: PROCEED WITH FIXES — 0 blockers, 4 concerns, 3 minors

---

## Summary

The plan is technically sound and follows an established end-to-end pattern. The proto field numbers are correct (field 15 and 13), the call-site survey confirms all three `NewAutonomousDriver` call sites already pass `config.LoadConfig().AutonomousMaxTurnsOrDefault()`, and the backend wiring is a two-line mechanical change. The stale comment fixes are accurate. No blockers were found. Four concerns and three minors are documented below.

---

## Findings

### CONCERN 1 — No ceiling-clamping test on the write path

**Category**: missing coverage  
**Location**: `server/services/defaults_service_test.go` — plan Task 2.1.3b

**Issue**: The plan adds two sub-tests (`ZeroResetsToDefault` and `ExplicitValueRoundTrips`). Neither tests the design's ceiling contract. Sending `autonomous_max_turns: 201` stores `201` in config, but `AutonomousMaxTurnsOrDefault()` returns `200` because of the hard ceiling. The response from `UpdateGlobalDefaults` would therefore return `200` for a `201` input — a meaningful invariant that has no test.

**Risk**: If a future change accidentally moves the ceiling enforcement from `AutonomousMaxTurnsOrDefault` to the write path (or removes it), no test catches the breakage. A `201 → 200` round-trip test would pin this contract.

**Resolution**: Add a third sub-test:
```go
t.Run("AboveCeilingClampedAt200", func(t *testing.T) {
    svc := newIsolatedDefaultsService(t)
    resp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
        AutonomousMaxTurns: 201,
    }))
    require.NoError(t, err)
    assert.Equal(t, int32(200), resp.Msg.Defaults.AutonomousMaxTurns)
})
```

---

### CONCERN 2 — Phase 4 runs `make build && make test`, not `make ci`

**Category**: missing coverage  
**Location**: Phase 4, Story 4.1.1, Task 4.1.1a

**Issue**: `make ci` (the CLAUDE.md-designated "definitive pre-push check") runs: `test-race`, `lint-css-tokens`, `fmt-check`, `registry-generate`, `actor-field-guard`, `ptmx-field-guard`, and `otel-auto-isolation-guard` in addition to `build` and `test`. The plan only calls `make build && make test`. These checks could surface issues:

- `gofmt` formatting drift (a proto-edit plus a Go file touch can produce formatting changes).
- `make registry-generate` is run by CI and then `git diff --exit-code docs/registry/features/` is checked. The registry scanner processes proto files for RPCs. Adding *fields* to existing RPCs does not add new RPC feature files, so this should be a no-op — but it is unverified in the plan.
- `actor-field-guard` and `ptmx-field-guard` scan for raw actor-field reads; the plan doesn't touch those code paths, but running the checks confirms it.

**Resolution**: Replace `make build && make test` in Task 4.1.1a with `make ci` (or at minimum add `make lint fmt-check registry-generate`). If `make ci` is too slow for the task, add an explicit note that `make registry-generate` was verified to produce no diff.

---

### CONCERN 3 — `parseInt` vs `Number` inconsistency in the UI onChange handler

**Category**: architecture risk (style consistency / review friction)  
**Location**: `web-app/src/components/settings/GlobalDefaultsForm.tsx` — plan Task 3.1.2a

**Issue**: The plan's Pattern Decisions table explicitly chooses `parseInt(e.target.value, 10)` over `Number(e.target.value)`, citing decimal rejection as the reason. However, every existing bounded number field in `GlobalDefaultsForm.tsx` uses `Number()`:

- `maxConcurrentBacklogWorkItems`: `Math.min(10, Math.max(1, Number(e.target.value) || 1))`
- `maxAutoReworkIterations`: `Math.max(1, Number(e.target.value) || 1)`
- `staleSessionThresholdMinutes`: (similar pattern)

`Number("3.5")` returns `3.5`, but the React number input with `type="number"` already prevents non-integer input via the native browser control. The difference is invisible in practice. A reviewer seeing the single `parseInt` departure will likely request alignment. The plan's `parseInt` reasoning is correct but creates an asymmetry that will slow review.

**Resolution**: Either use `Number()` to match all existing fields (the pattern already works in production), or upgrade all numeric-field handlers to `parseInt` in the same PR to eliminate the inconsistency. Leaving one field different creates a class of "which one is right?" questions for every future contributor.

---

### CONCERN 4 — Frontend test covers only rendering, not the submit path

**Category**: missing coverage  
**Location**: `web-app/src/components/settings/GlobalDefaultsForm.test.tsx` — plan Task 3.1.3b

**Issue**: The plan's frontend test verifies that the field renders with a server-provided value (45). It does not verify that `autonomousMaxTurns` is included in the `updateGlobalDefaults` RPC call when the user saves. If the submit wiring (Task 3.1.1c) is accidentally omitted or the field name is misspelled in the request object, the field renders correctly but silently never persists.

The existing test file (`GlobalDefaultsForm.test.tsx`) likely has a submit test for other fields. If it does, this gap is more prominent — a reviewer will notice the new field has no save-path coverage.

**Resolution**: Add a test that simulates the user changing the value and submitting the form, then asserts that the mocked `updateGlobalDefaults` was called with `autonomousMaxTurns` equal to the user-entered value.

---

### MINOR 1 — `autonomousMaxTurnsDefault` is unexported; test uses a magic literal

**Category**: simplification  
**Location**: `server/services/defaults_service_test.go` — plan Task 2.1.3a

**Issue**: The plan acknowledges that `config.autonomousMaxTurnsDefault` is unexported (lowercase) and falls back to the literal `int32(30)`. This is workable but less self-documenting than a named constant. If the default ever changes, the literal in the test won't cause a compile error, only a test failure.

**Resolution**: Export the constant as `config.AutonomousMaxTurnsDefault` in the same PR (a one-line change in `config/config.go`), consistent with `config.DefaultMaxAutoReworkIterations` which *is* exported. The plan's test can then reference `int32(config.AutonomousMaxTurnsDefault)` with a compile-time guarantee. Low effort, eliminates the magic number.

---

### MINOR 2 — `NewAutonomousDriver`'s internal zero-fallback is 20, not 30

**Category**: correctness (pre-existing, not introduced by this plan)  
**Location**: `session/autonomous_driver.go:126`

**Issue**: `NewAutonomousDriver` has `if maxTurns <= 0 { maxTurns = 20 }`. All production call sites now pass `config.LoadConfig().AutonomousMaxTurnsOrDefault()` (returns 30 when unset), so this internal fallback is never reached in production. However, the driver's self-documenting comment at line 123 says `maxTurns ≤ 0 defaults to 20`, which disagrees with the config default (30). Test code and future callers could be surprised.

This is pre-existing debt, not introduced by this plan. It does not need to be fixed here but is worth noting for a follow-up. If it's fixed here, update the `NewAutonomousDriver` fallback from `20` to `autonomousMaxTurnsDefault` (or just remove the fallback entirely since callers are expected to pass a resolved value).

---

### MINOR 3 — `|| 30` UI load fallback has inconsistent precedent

**Category**: simplification  
**Location**: `web-app/src/components/settings/GlobalDefaultsForm.tsx` — plan Task 3.1.1b

**Issue**: The plan adds `setAutonomousMaxTurns(defaults.autonomousMaxTurns || 30)`. The form already has similar patterns:
- `maxConcurrentBacklogWorkItems`: `defaults.maxConcurrentBacklogWorkItems || 2` (has fallback)
- `staleSessionThresholdMinutes`: `defaults.staleSessionThresholdMinutes || 30` (has fallback)
- `maxAutoReworkIterations`: `defaults.maxAutoReworkIterations` (no fallback)

The `|| 30` fallback is consistent with the majority of numeric fields in the form. However, since the read path always returns the resolved default (never 0) via `AutonomousMaxTurnsOrDefault()`, the fallback is unreachable. It's harmless and fits the dominant pattern — noting it only to flag it is deliberate noise, not an oversight.

---

## Verification Notes

The following were confirmed against live code before writing this review:

- `SessionDefaultsConfig` ends at `retry_policy = 14`; field 15 is free. (`proto/session/v1/session.proto:2213`)
- `UpdateGlobalDefaultsRequest` ends at `retry_policy = 12`; field 13 is free. (`proto/session/v1/session.proto:2292`)
- Three production `NewAutonomousDriver` call sites all pass `config.LoadConfig().AutonomousMaxTurnsOrDefault()`, not `0`. (`server/services/autonomous_orchestration_service.go:217,235`; `server/services/session_creation_pipeline.go:346`)
- `config.DefaultMaxAutoReworkIterations = 5` (not 3); stale comments confirmed at `session.proto:2201,2282` and `backlog_service.go:950`. (config/config.go:1059)
- `autonomousMaxTurnsDefault = 30` is unexported (lowercase) at `config/config.go:1077`.
