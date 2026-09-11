# BUG-094: flow-control-stress "Mixed Content Stress" test times out under full `npx jest` run [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-26
**Impact**: Occasional false-red CI/local `npx jest` runs; no production impact.

## Problem Description

`Flow Control Stress Tests > Mixed Content Stress > handles alternating text and control codes`
in `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts:251` has a fixed 15000ms
timeout (line 291). It exceeds that timeout when run as part of the full `npx jest` suite
(4449 other tests running concurrently across workers), but passes reliably in isolation.

## Reproduction Steps

1. `cd web-app && npx jest --no-coverage` (full suite)
2. `cd web-app && npx jest --no-coverage --testPathPatterns="flow-control-stress"` (isolated)
3. Expected: both pass.
4. Actual: full suite run intermittently times out this one test; isolated run passes.

## Root Cause

The test is CPU-bound (encodes/parses a large mixed text+control-code stream) and asserts
against a fixed wall-clock timeout rather than a work-scaled or mocked-time budget. Under
full-suite jest-worker parallelism, CPU contention from sibling workers pushes actual
wall-clock past 15s even though the same work completes well under that in isolation.

## Files Likely Affected

- `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts:251-291` — the flaky test and its timeout

## Fix Approach

Either raise the timeout to give headroom for full-suite contention, or make the workload
size/timeout adaptive (scale expected duration to a calibration pass) instead of a fixed
wall-clock threshold.

## Verification

Run `npx jest --no-coverage` (full suite) several times in a loop; the test should pass
consistently without needing isolation.

## Related Tasks

Found while completing backlog item `0ddc4edb-ae2e-4d85-b9cf-067af72be323`
(useFocusTrap trigger-focus-return) — unrelated to that change.
