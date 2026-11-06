# Validation: stop-idle-review-polling

## AC-0 coverage

Task 1 modifies `requestReview` response text to include explicit idle
instructions. The pin test (Task 1 test) checks the response contains the
stop phrasing. Pass criterion: `strings.Contains(response, "End your turn")`.

## AC-1 / AC-2 coverage

Task 2 adds a guard in `waitForBacklogEvent` that fires immediately when
`item.Status == review`. The new test asserts:
- Return code is `WAIT_CAP_REACHED`
- Elapsed time < 200ms (no blocking)

Edge case: item with a verdict already present (status "done" or a verdict
on a still-review item) — `currentStateWaitResult` fires first and returns the
verdict before reaching the guard. The new test should use an item explicitly
in "review" with no verdict.

## AC-3 coverage

Pin test on request_review response text. Mirrors the existing
`TestWaitForBacklogEvent_TimeoutMessage_NoPollingAdvice` pattern.

## AC-4 coverage

Manual grep audit (Task 3). Document any hits. Expected: all hits are
prohibitions, not recommendations.

## AC-5 coverage

Full `make test` run. The `server/mcp/` package has good coverage of
`waitForBacklogEvent`. The only expected failures would be regressions
introduced by the guard placement — the adversarial review addressed the
most likely ones.

## Out-of-scope validation

- `StartVerdictSteering` e2e path: covered by existing tests in
  `backlog_service_verdict_steer_test.go`. No changes to that path.
- Prompt-cache TTL behavior: not directly testable in unit tests; the
  fix prevents wakes, which prevents the TTL from being relevant.
