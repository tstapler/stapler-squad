# Pre-mortem: e2e-baseline-fail-fixes
**Date**: 2026-11-05

## Failure Modes

| # | Failure | First Symptom | Prevention | Severity |
|---|---------|--------------|------------|----------|
| 1 | `open terminal here` is a URL routing bug, not a keychain bug — still fails after Phase 1; Task 3.2.1 unconditionally marks it `"class": "pass"` → CI fails every run because test fails but baseline says pass | Task 1.1.3 `npx playwright test local-file-browser.spec.ts -g "open terminal here"` exits non-zero; CI baseline compare reports unexpected failure on a "pass" row | Add an explicit branch to Task 3.2.1: **only** mark those 2 rows pass if Task 1.1.3 confirmed a pass; add a fallback investigation task (Story 3.1) covering open-terminal as a URL routing bug (find the navigation handler that should redirect to `?session=` and add the missing query-param) | P1 |
| 2 | Python baseline mutation script silently mismatches entries (incorrect title substring, off-by-one spec name, or wrong JSON key name) → stale `"fail"` rows remain after tests now pass; compare script treats unexpected improvements as gate failures → CI blocked | `make ci` fails with compare output showing one of the target specs as "unexpected pass" (test passed but baseline still says fail); `jq` AC-9 guard returns non-zero | Before writing the mutation script, run `jq '[.specs[] \| select(.spec \| test("jules-dispatch\|alias-settings\|local-file-browser")) \| select(.class == "fail")] \| length'` to get the exact count (expect 20); verify the Python filter logic produces exactly that count before mutating; commit the resulting diff and confirm `git diff --stat` shows exactly 20 `"fail"→"pass"` changes and 2 entry removals | P1 |
| 3 | `alias-settings` AC-02 fails due to success-toast timing: the toast fires and clears before `expect(page.getByTestId("alias-row-e2e-edit")).toContainText("Updated E2E")` runs in slow CI → 2 baseline rows remain `fail`, AC-4 is partially unmet | Task 2.2.1 `npx playwright test alias-settings.spec.ts -g "AC-02"` exits non-zero in CI even though it passes locally; HTML trace shows the row was not yet rendered when the assertion fired | In Task 2.2.1, pre-plan a concrete conditional fix for the timing path: replace the bare `expect(...).toContainText(...)` with `expect.poll(...)` or add an explicit `await expect(page.getByTestId("alias-save-success")).toBeVisible()` wait before asserting the row text; do not leave "diagnose from trace" as the only action | P2 |
| 4 | `route-focus-and-announce.spec.ts` lines 65 and 130 have no `isBunBuild` guard; they use `useFocusOnNavigate` which is a no-op in the Bun build → tests run with new titles that the Bun baseline cannot match, creating new unprotected failures in Bun CI | `make ci` (or the Bun e2e CI job) reports new failures for `the announcer is role=status...` and `/history: duplicate main#main-content...` with no matching baseline entry; compare classifies them as new unknown failures | Add a Story before Phase 3 to audit which route-focus tests rely on `useFocusOnNavigate`; apply `test.skip(isBunBuild, "useFocusOnNavigate is a no-op in Bun build")` to lines 65 and 130; verify that `bun run test` on that spec exits 0 | P2 |
| 5 | `github/keychain.go` serializes via `keychainMu` and `session/sshremote/keystore.go` serializes via its own `keyringMu`; both are separate mutexes guarding calls to the shared `go-keyring` mock whose `mockStore` map is unsynchronized → concurrent calls from both packages during e2e tests produce a data race; `make test-race` fails | `make test-race` exits non-zero with `DATA RACE` on a map read/write in `github.com/zalando/go-keyring`'s mock provider; CI race-detection job (if wired) blocks the PR | Promote the per-package `keyringMu` variables to a single shared `internal/keyring/mu.go` package-level mutex, or add a mutex inside the go-keyring mock (fork or monkey-patch); document the constraint in the interim as a code comment so a future e2e test adding SSH-remote concurrent to Jules-auth knows to expect the race | P2 |

## P1 Items (address before implementation)

- [ ] **Failure #1 — "open terminal here" missing fallback task**: before Task 3.2.1 is written, confirm Task 1.1.3 passes or add a Story 3.1b that investigates the open-terminal URL routing path and fixes the `SessionType.DIRECTORY` creation → `?session=` redirect independently of the keychain fix. Task 3.2.1 must gate on confirmation, not assume all 4 local-file-browser rows pass.

- [ ] **Failure #2 — Python mutation script verification gate**: before writing the edit script, run the `jq` count command to verify the exact number of target entries (should be 20 fail→pass + 2 removals = 22 total changes); include the count verification as the first step of Task 3.2.1 so a discrepancy fails loudly before any mutation.

## Verification Queries Added by This Pre-mortem

```bash
# FM2 guard: verify exact count before mutating
jq '[.specs[] | select(.spec | test("jules-dispatch|alias-settings|local-file-browser")) | select(.class == "fail")] | length' \
  tests/e2e/baseline/next-e2e-baseline.json
# Expected: 20

# FM2 guard: verify skip-link removal targets exist and use the real title
jq '[.specs[] | select(.title | test("skip link: Tab reaches"))] | .[].title' \
  tests/e2e/baseline/next-e2e-baseline.json
# Expected: "route focus and announcements (Next baseline) > skip link: Tab reaches it, Enter keeps the path and sets the hash"
# (NOT "...Enter activates it" as written in plan.md Task 3.2.2)
```
