# BUG-079: 4 unrelated Jest suites fail only in the full `npx jest` run, pass in isolation [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-16 (during dynamic-rule-reload backlog item's Phase 7 verification gate)
**Impact**: Intermittent CI noise on the full web-app Jest suite — the failures are not reproducible on demand, which erodes trust in red CI (same pattern as BUG-076, applied to the frontend suite instead of `server/services`).

## Problem Description

Running `cd web-app && npx jest --no-coverage` (full suite, 354 test files) produced 12 failed
suites / 15 failed tests, including:

- `src/components/backlog-stuck/StuckItem.test.tsx`
- `src/lib/transport/websocket-transport.test.ts`
- `src/components/sessions/__tests__/TerminalOutput.enter-detection.test.tsx`
- `src/components/backlog/BacklogItemDetail.markdown.test.tsx`

(plus 8 more suites not yet enumerated individually).

Re-running just those 4 suites in isolation immediately after:

```
npx jest --no-coverage --testPathPatterns="StuckItem.test|websocket-transport.test|TerminalOutput.enter-detection.test|BacklogItemDetail.markdown.test"
Test Suites: 4 passed, 4 total
Tests:       53 passed, 53 total
```

passed cleanly. None of these 4 files touch anything related to the diff that surfaced this
(dynamic-rule-reload's `ApprovalRulesPanel.tsx`/`useApprovalRules.ts`/`NotificationContext`
consumer changes) — `ApprovalRulesPanel.test.tsx` itself passed in both the full run and
isolation. This points to cross-suite state leakage or resource contention (shared timers,
jsdom globals, or worker-process contention) under Jest's default parallel-worker full-suite
run, not a logic bug in any of the 4 files — mirrors BUG-076's diagnosis exactly, one layer
up the stack (frontend Jest workers vs. Go `server/services` package-level state).

Confirmed unrelated to the dynamic-rule-reload diff (only touches
`ApprovalRulesPanel.tsx`/`useApprovalRules.ts`/backend Go files) — filed per the blast-radius
exception in `.claude/rules/fix-flaky-tests-dont-defer.md` rather than root-caused here, since
bisecting shared Jest worker/global state across 354 suites is out of scope for a rules-reload
feature.

## Fix Approach

- Re-run the full suite with `--runInBand` (single worker, serial) to check whether the
  failures disappear — if so, it confirms cross-worker/global-state contention rather than a
  per-file bug, narrowing the fix to Jest config (`testEnvironment` reset behavior,
  `maxWorkers`, or a global mock/timer not being torn down between files).
- If failures persist under `--runInBand`, bisect which preceding suite(s) leak state (fake
  timers left running, a `jest.mock` at module scope not reset, a WebSocket/global stub not
  cleaned up) that these 4 depend on implicitly.
- Once identified, add proper `afterEach`/`afterAll` teardown in the leaking suite(s), matching
  the precedent set by BUG-076's fix approach for the equivalent Go-side issue.

## Related Tasks

Found during Phase 7 verification (`make quick-check` + full Jest suite) of the
`dynamic-rule-reload` backlog item (project_plans/dynamic-rule-reload/). Not fixed in that
item's PR — out of scope (unrelated pre-existing frontend test infra vs. a rules-hot-reload
feature).
