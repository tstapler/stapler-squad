# Requirements: notimesleeptest analyzer (ADR-003 enforcement)

Source: backlog item 338bce11 — "enforce deterministic tests without raw sleeps".

## Problem
ADR-003 bans static `time.Sleep` in tests, but only the grep-based `make lint-no-sleep-tests` enforces it; it is not in `make lint`/CI and exempts `testutil/`.

## Scope
- New `go/analysis` pass `notimesleeptest` (type-resolved, handles aliased imports) rejecting `time.Sleep` in `*_test.go`.
- Exempt only a named real-time area (e.g. `tests/realtime/`) and `//nolint:notimesleeptest <reason>`.
- Register in `tools/lint/cmd/linter` (runs via `make lint-custom` / `lint` / `ci`).
- Add `make lint-custom` to GitHub Actions lint job.
- Migrate existing corpus before the rule becomes blocking.

## Acceptance criteria
1. Direct or aliased `time.Sleep` in a normal `*_test.go` fails lint.
2. Non-test files and unrelated `Sleep` methods do not fail.
3. Only the real-time area and documented nolint exceptions are exempt.
4. Analyzer tests cover accepted and rejected cases.
5. GitHub Actions enforces the analyzer.
6. Existing corpus migrated (0 unexempted violations) before enforcement lands.

## Verification
`make lint-custom`; analyzer unit tests (`go -C tools/lint test ./notimesleeptest/...`); GH Actions lint workflow.
