# Validation Plan: e2e-baseline-fail-fixes
**Date**: 2026-11-05

## Happy Path Scenario

Given 34 baseline rows classified as `"fail"` and two as `removedFromRun` candidates, when `keyring.MockInit()` is added to `--test-mode` startup, four AC-04 locators are corrected in `alias-settings.spec.ts`, and the symlink navigation assertion is resolved, then `npx playwright test jules-dispatch.spec.ts alias-settings.spec.ts local-file-browser.spec.ts route-focus-and-announce.spec.ts --project=chromium` exits 0 and `next-e2e-baseline.json` contains no entries for those tests with `class: "fail"`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC-1: All 10 jules-dispatch baseline rows transition to pass | `tests/e2e/jules-dispatch.spec.ts` | `jules dispatch gating > Jules disabled: dispatch trigger is absent from the DOM` | Playwright e2e | Happy: flag off → trigger absent |
| AC-1: All 10 jules-dispatch baseline rows transition to pass | `tests/e2e/jules-dispatch.spec.ts` | `jules dispatch gating > Jules enabled with stubbed key: Dispatch stays disabled until egress checkbox checked` | Playwright e2e | Happy: flag on + KeychainStub in place → dialog opens, Dispatch disabled without checkbox |
| AC-1: All 10 jules-dispatch baseline rows transition to pass (error path) | — | (manual/CI) Run `jules-dispatch.spec.ts` on a build **without** `keyring.MockInit()` on headless Linux — expect D-Bus timeout; confirms the stub is required | Integration | Error: no KeychainStub → D-Bus hang |
| AC-2: e2e test-mode server uses in-memory keyring (not OS D-Bus keychain) | `github/keychain_test.go` | Existing: `TestGetToken_ReturnsStoredToken`, `TestSetToken_Roundtrip` | Unit (Go) | Happy: `keyring.MockInit()` in-process → `Set`/`Get` roundtrip via memory map, no D-Bus call |
| AC-2: KeychainStub active in test-mode binary | `main.go` integration smoke | (run `go test ./... -run TestMainTestModeKeyring` — new test; see note below) | Unit (Go) | Happy: `--test-mode` flag path calls `keyring.Set()`; returns without error on headless Linux |
| AC-2: KeychainStub not active without test-mode flag (error path guard) | Same test | Negative: without `--test-mode`, `keyring.MockInit()` is NOT called; `keyring.Get()` returns `ErrNotFound` or a D-Bus error (not a panic) | Unit (Go) | Error: production path unmodified |
| AC-3: Both failing local-file-browser rows transition to pass | `tests/e2e/local-file-browser.spec.ts` | `local-file-browser > navigates into a symlinked directory` | Playwright e2e | Happy: click symlinked dir → path bar shows resolved real path `<subDir>`, `inside.txt` visible |
| AC-3: local-file-browser symlink error path | `tests/e2e/local-file-browser.spec.ts` | `local-file-browser > navigates into a symlinked directory` | Playwright e2e | Error: if path bar shows symlink path instead of real path → assertion fails (confirms root cause diagnosis) |
| AC-3: open-terminal row transitions to pass | `tests/e2e/local-file-browser.spec.ts` | `local-file-browser > open terminal here creates a directory-mode session` | Playwright e2e | Happy: click open-terminal → URL contains `?session=` within 15 s |
| AC-4: AC-02 edit alias description passes | `tests/e2e/alias-settings.spec.ts` | `alias-settings > AC-02 edit alias description` | Playwright e2e | Happy: create "e2e-edit" → edit → `alias-row-e2e-edit` contains "Updated E2E" |
| AC-4: AC-04 locator fixes all resolve | `tests/e2e/alias-settings.spec.ts` | `alias-settings > AC-04 add environment variable via Advanced section` | Playwright e2e | Happy: `getByLabel("Advanced")` checked → `Add variable` button clicked → key/value filled → save → `alias-row-e2e-envvar` visible |
| AC-4: AC-04 stale locator error path | `tests/e2e/alias-settings.spec.ts` | `alias-settings > AC-04 add environment variable via Advanced section` | Playwright e2e | Error: if old locator strings (`"Advanced options"`, `"Add Variable"`, `"Environment variable 1 key"`) are used → `getByLabel` times out (confirms mismatch) |
| AC-4: AC-05 add tag via Enter passes | `tests/e2e/alias-settings.spec.ts` | `alias-settings > AC-05 add tag via tag input Enter key` | Playwright e2e | Happy: fill "e2e-tag" + press Enter → `getByText("e2e-tag")` resolves visible element (substring match on chip) |
| AC-4: AC-05 tag chip not visible without Enter (error path) | — | (manual trace) fill tag name but do NOT press Enter → `getByText("e2e-tag")` not visible | Playwright e2e | Error: tag not committed without Enter |
| AC-5: 2 route-focus skip-link rows removed from baseline | `tests/e2e/baseline/next-e2e-baseline.json` | (baseline compare script) — no matching entries remain with title containing `"skip link: Tab reaches"` | Integration | Happy: compare exits 0 with no `removedFromRun` report for those two rows |
| AC-5: route-focus skip-link rows previously reported as removedFromRun | — | Baseline snapshot before removal: running compare against an unmodified baseline with those two entries prints `removedFromRun` severity `"report"` | Integration | Error: entries present in baseline for nonexistent tests → compare output carries noise |
| AC-6: dev-mode-hmr rows absent from standard baseline | `tests/e2e/baseline/next-e2e-baseline.json` | (grep audit) `jq '[.entries[] | select(.file | contains("dev-mode-hmr"))] | length' next-e2e-baseline.json` returns 0 | Integration | Happy: dev-mode-hmr spec is excluded |
| AC-7: visual-regression rows not touched | `tests/e2e/baseline/next-e2e-baseline.json` | (grep audit) all entries with `file` containing `visual-regression` retain their original `class` values after the JSON edit | Integration | Happy: visual-regression rows unchanged |
| AC-8: No new failures — `make quick-check` exits 0 | Makefile / CI | Run `make quick-check` after all changes | Integration | Happy: build + unit tests + lint all green |
| AC-8: No new failures — `bun run lint:duplicates` exits 0 | `web-app/.jscpd.json` | Run `cd web-app && bun run lint:duplicates` after alias-settings spec change | Integration | Happy: 4 changed lines stay below 0.14% threshold |
| AC-9: `next-e2e-baseline.json` updated — 20 fail→pass | `tests/e2e/baseline/next-e2e-baseline.json` | (baseline compare script) Previously-failing rows for jules-dispatch (10), alias-settings (6), local-file-browser (4) now have `class: "pass"` | Integration | Happy: compare output reports 0 unexpected failures for target specs |
| AC-9: `next-e2e-baseline.json` — no unintended changes | `tests/e2e/baseline/next-e2e-baseline.json` | (diff audit) `git diff --stat` on the file shows only the expected 20 `fail→pass` mutations and 2 entry removals | Integration | Happy: exactly 22 rows affected, no other rows touched |

## Notes on New Go Unit Test (AC-2)

The plan confirms `keyring.MockInit()` is verified by existing tests in `github/keychain_test.go`. A new integration-level `TestMainTestModeKeyring` is **not strictly required** because:

1. The `main.go` change is mechanical (one import + one call in a guarded block).
2. `github/keychain_test.go` already exercises the full MockInit→Set→Get roundtrip.
3. The e2e run for jules-dispatch serves as the functional integration smoke.

If the implementation team judges the main.go code path warrants a direct unit assertion (e.g. to prevent regression removal of the call), it can be added to `main_test.go` as a startup-integration test using `exec.Command` with `--test-mode` and a synthetic keyring write. This is optional for this AC.

## UX Acceptance Tests

N/A — no user-facing surface; this is a test infrastructure fix.

## Test Stack

- **Unit**: Go testing + testify (`go test ./...`)
- **Integration**: Playwright e2e (`npx playwright test <spec> --project=chromium`)
- **Baseline audit**: `jq` queries against `tests/e2e/baseline/next-e2e-baseline.json`

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out` | ≥80% line (existing; `main.go` testModeFlag block is exercised by e2e global-setup) |
| Playwright (target specs) | `cd tests/e2e && npx playwright test jules-dispatch.spec.ts alias-settings.spec.ts local-file-browser.spec.ts route-focus-and-announce.spec.ts --project=chromium` | 100% spec pass |
| Baseline JSON integrity | `jq '[.entries[] \| select(.class == "fail" and (.file \| test("jules-dispatch\|alias-settings\|local-file-browser\|route-focus")))] \| length' tests/e2e/baseline/next-e2e-baseline.json` | Returns `0` after update |
| Duplication gate | `cd web-app && bun run lint:duplicates` | Exits 0 |
| Full CI | `make quick-check` | Exits 0 |

## Verification Sequence

Run these in order after all code changes are applied:

1. `go build .` — confirms `keyring.MockInit()` import compiles
2. `cd tests/e2e && npx playwright test jules-dispatch.spec.ts --project=chromium` — AC-1 / AC-2
3. `cd tests/e2e && npx playwright test local-file-browser.spec.ts --project=chromium` — AC-3
4. `cd tests/e2e && npx playwright test alias-settings.spec.ts --project=chromium` — AC-4
5. `cd tests/e2e && npx playwright test route-focus-and-announce.spec.ts --project=chromium` — AC-5 (should still pass; skip-link tests do not exist)
6. `jq '[.entries[] | select(.class == "fail" and (.file | test("jules-dispatch|alias-settings|local-file-browser")))] | length' tests/e2e/baseline/next-e2e-baseline.json` — AC-9 (expect `0`)
7. `jq '[.entries[] | select(.title | test("skip link: Tab reaches"))] | length' tests/e2e/baseline/next-e2e-baseline.json` — AC-5 removal (expect `0`)
8. `jq '[.entries[] | select(.file | contains("visual-regression"))] | map(select(.class != "fail")) | length' tests/e2e/baseline/next-e2e-baseline.json` — AC-7 guard (expect same count as before the edit)
9. `cd web-app && bun run lint:duplicates` — AC-8
10. `make quick-check` — AC-8 final gate
