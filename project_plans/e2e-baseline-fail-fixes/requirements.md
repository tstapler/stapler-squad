# Requirements: e2e-baseline-fail-fixes

**Item ID:** b21fb6da-77cb-4877-bf8b-3d82e7514ba5  
**Title:** e2e: 34 baseline-fail rows depend on the host or on removed behaviour

## Background

After PR #1166, the recorded Next baseline (`tests/e2e/baseline/next-e2e-baseline.json`) contains specs with class `fail` that are caused by environmental constraints, removed/renamed UI behaviour, or test-runner misconfigurations — not real regressions in the production path.  Fixing them is prerequisite to making the baseline a reliable signal.

## Root Causes (per spec file)

| Spec | Rows | Root cause |
|---|---|---|
| `alias-settings.spec.ts` | 6 | Label mismatches: test uses `getByLabel("Advanced options")` but component renders `"Advanced"`; env-var aria-label is `"Environment variable key N"` but test expects `"Environment variable N key"`. |
| `dev-mode-hmr.spec.ts` | 6 | HMR tests run only under `playwright.dev-mode.config.ts`; already excluded from standard production projects in the triage PR. No baseline rows remain after exclusion. |
| `jules-dispatch.spec.ts` | 10 | `updateJulesConfigDirect` → `UpdateJulesConfig` RPC → `SetJulesAPIKey` → OS keychain → 500 "The name org.freedesktop.secrets was not provided" on headless Linux (no D-Bus Secret Service in CI). Circuit breaker then blocks all subsequent Jules keychain reads for 5 min. |
| `local-file-browser.spec.ts` | 4 | (a) "navigates into symlinked directory" — symlink navigation may not update URL path as the test expects; (b) "open terminal here" — test asserts `toHaveURL(/[?&]session=/)` but the page navigates to `/files/?path=…` without a `session=` param after session creation. |
| `route-focus-and-announce.spec.ts` | 2 | Skip-link test (`"skip link: Tab reaches it, Enter activates it"`) is in the "Next baseline" describe block and asserts `mainFocused === false`, which is a Next-only fact (Bun build may focus main on skip-link activation). |
| `visual-regression.spec.ts` | 11 | Visual snapshot differences across builds tracked separately (not yet rebased snapshots). |

## Acceptance Criteria

1. **AC-1 jules-dispatch**: All 10 `jules-dispatch.spec.ts` baseline-fail rows transition to `pass` or `skip` with a documented skip reason. No test may throw 500 from a keychain error when run against the e2e test-mode server on headless Linux.

2. **AC-2 jules-dispatch (keychain stub)**: The e2e test-mode server (`--test-mode`/`--test-dir`) uses an in-memory keyring backed by `jules/keychain.go`'s existing `keyringGet`/`keyringSet`/`keyringDelete` package-level seams, NOT the real OS D-Bus keychain.

3. **AC-3 local-file-browser**: Both failing `local-file-browser.spec.ts` rows transition to `pass`. Either (a) the "open terminal here" assertion is updated to match the actual post-creation URL, or (b) the production behaviour is fixed so the URL contains `session=` as originally intended.

4. **AC-4 alias-settings**: All 6 `alias-settings.spec.ts` baseline-fail rows (AC-02, AC-04, AC-05 × 2 projects) transition to `pass`. Locators in the test and/or the component match: "Advanced options" → actual label text, env-var aria-label word order, description field label.

5. **AC-5 route-focus-and-announce**: The 2 failing skip-link rows either (a) transition to `pass` by correcting the assertion for Bun build behaviour, or (b) the test is tagged as Next-only and transitions to `skip` with an explicit `test.skip(isNextBuild, "...")` guard.

6. **AC-6 dev-mode-hmr**: The 6 `dev-mode-hmr.spec.ts` rows do not appear in the standard baseline at all (already excluded; verify exclusion is committed and held by the triage PR or its successor).

7. **AC-7 visual-regression**: The 11 (4 in baseline) `visual-regression.spec.ts` rows are addressed in a separate tracking item and are excluded from this fix's scope, documented as such in the plan.

8. **AC-8 no new failures**: After this fix, `make ci` passes, `bun run lint:duplicates` in `web-app/` passes, and no previously-passing baseline row transitions to `fail`.

9. **AC-9 baseline update**: `tests/e2e/baseline/next-e2e-baseline.json` is updated via the baseline capture script to reflect the corrected spec outcomes.

## Out of Scope

- Visual-regression snapshot regeneration (tracked separately).
- Other `fail`-class specs in the Next baseline unrelated to the five spec files above.
- Enabling a full D-Bus Secret Service in CI infrastructure.
