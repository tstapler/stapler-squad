# Validation: e2e Pre-existing Failures

## Acceptance Criteria Checklist

| # | Criterion | How to verify |
|---|-----------|---------------|
| 1 | `ShowAggregateSummary` passes | `cd tests/e2e && bun run test -- --testPathPattern=escape-analytics-global-view` |
| 2 | `ShowErrorBanner` passes | Same run as above |
| 3 | `jules-status-badge` tests 2 & 3 pass | `bun run test -- --testPathPattern=jules-status-badge` |
| 4 | `multi-window renaming...` passes | `bun run test -- --testPathPattern=multi-window` |
| 5 | No other specs regress | `bun run test` (full suite) |
| 6 | Passes on Bun build | Set `WEB_BUILD=bun make install-service` then re-run e2e suite |

## Edge Cases to Check

- **Bug 1**: If fixture sets `correlationOutcomes` to a *different* value than `totalSequences`, the test assertion needs to be updated to match `correlationOutcomes`, not `totalSequences`. Verify the test asserts the right value after the fix.
- **Bug 2**: After the fix, the `ShowErrorBanner` test must trigger the actual error banner (not just confirm 0 matches). Verify the page-route mock rejection path actually fires (intercepted via `page.route()`).
- **Bug 3**: After removing the `deps.Storage != nil` guard, confirm the seed handler doesn't panic at nil-dereference inside `handleSeedJulesWorkSession`. If it does, add a nil check inside the handler itself.
- **Bug 4**: Verify that when a toast IS shown, `nonToastAlerts` still returns 0 (the toast exclusion still works alongside the new announcer exclusion).

## Risks

- **Low**: All four fixes are test/page-object changes with no production code impact (bugs 1/2/4 are pure test file changes; bug 3 is a server-startup-only guard change).
- **Medium (Bug 3)**: If `deps.Storage` is nil because the backlog storage module isn't initialized for some other reason at e2e test startup, removing the guard alone won't fix the HTML response. The seed handler's own nil-check must return a proper JSON error, not panic.

## Definition of Done

All four failing tests green on the CI run (`make ci`) against both Next and Bun web builds, with no new failures introduced.
