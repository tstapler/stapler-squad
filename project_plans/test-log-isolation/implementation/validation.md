# Validation Plan: test-log-isolation

**Date**: 2026-08-29

## Happy Path Scenario
Given the confirmed race between `TestAnthropicAIClient_Complete_CancelsOnCtxDone` and `TestSlackNotifier_NeverLogsWebhookURL` (requirements.md's Baseline, instance #2), when the Story 5 stress-repro command (`go test -race -run 'TestAnthropicAIClient_Complete_CancelsOnCtxDone|TestSlackNotifier_NeverLogsWebhookURL' -count=20 ./server/services/...`) runs after Stories 1-4 land, then no `-race` data-race report fires and both tests' buffer reads/writes remain correct across every iteration.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Scenario |
|-------------|-----------|-----------|----------|
| Success Metric 1 — `TestAnthropicAIClient_Complete_CancelsOnCtxDone` no longer races any `captureLogs`-style test | `server/services/anthropic_client_test.go`, `server/services/slack_notifier_test.go` | `TestAnthropicAIClient_Complete_CancelsOnCtxDone` + `TestSlackNotifier_NeverLogsWebhookURL` via the stress command `go test -race -run 'TestAnthropicAIClient_Complete_CancelsOnCtxDone\|TestSlackNotifier_NeverLogsWebhookURL' -count=20 ./server/services/...` (plan.md Task 5.1; plan.md recommends `-count=50` post-fix for higher confidence) | Given the post-fix code (Stories 1-4 applied), when the stress command runs, then it passes cleanly with no `WARNING: DATA RACE` report across all iterations (plan.md Task 5.1 Given/When/Then) |
| Success Metric 2 — no regression in instance #1's `ForceTeardown`-per-iteration pattern or the `slogDefaultMu`-cooperating tests | `server/services/connectrpc_websocket_test.go`; `server/services/autonomous_orchestration_service_test.go` (and the other 3 Story 2 migration sites) | `TestHubRegistryAndStreamOwnershipLock_should_NeverProduceTwoOwners_When_RacedConcurrently` (existing, untouched by this fix) plus the full regression pass `go test -race ./server/services/... -count=2` and `make build && make test && make lint` (plan.md Task 5.2) | Given Stories 1-4 applied, when the full regression pass runs, then instance #1's test and every `slogDefaultMu`-cooperating test from Story 2's 4 migrated call sites continue to pass under `-race` with zero new `make lint` violations (plan.md Task 5.2 Given/When/Then) |
| Success Metric 3 — the fix generalizes: a future test starting an `httptest.Server`/background goroutine without knowing about `slogDefaultMu` can't reintroduce this race class | `server/services/autonomous_orchestration_service_test.go` | `syncLogBuffer` concurrency safety, verified via `go test -race -run TestAutonomousOrchestrationService -count=20 ./server/services/...` (plan.md Task 3.1 Given/When/Then) | Given `captureLogs`'s buffer is now the mutex-guarded `*syncLogBuffer` and, after Story 1+2's structural seam, no `server/services` test ever calls the real `slog.SetDefault()`, when two goroutines concurrently `Write`/`String` on the buffer, then `-race` reports no data race — proving the mechanism is safe by construction (the precondition for the race is removed package-wide), not by a convention a new test author could forget to opt into |

## UX Acceptance Tests
N/A — pure test-infrastructure fix, no user-facing surface.

## Test Stack
- **Unit/Race**: go test -race (stdlib testing + testify per existing repo convention)

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All 3 requirements/success-metrics above have a named, runnable verification command.
