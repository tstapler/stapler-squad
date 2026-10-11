# Validation Plan: lower-rework-turn-caps

**Feature**: Expose `AutonomousMaxTurns` via Settings UI and fix stale proto/service comments
**Date**: 2026-10-07
**Status**: Pre-implementation

---

## Happy Path Scenario

User opens Settings → Global Defaults, sees "Max Autonomous Session Turns" pre-populated with the server-resolved value (30 on a fresh install), changes it to 50, clicks Save, reloads the page, and sees 50.

---

## Requirement → Test Mapping

| Req | Requirement Summary | Unit Test — Happy Path | Unit Test — Error Path | Integration Test |
|-----|---------------------|----------------------|----------------------|-----------------|
| REQ-1 | `SessionDefaultsConfig` has `int32 autonomous_max_turns = 15` with default-30 comment | `TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault` — fresh `DefaultsService` via `newIsolatedDefaultsService(t)` returns `AutonomousMaxTurns == 30` in proto response, confirming generated field is wired | n/a (static; enforced by `make build` compilation gate) | `TestGetSessionDefaults_AutonomousMaxTurns_FieldPresentInResponse` — `newIsolatedDefaultsService` full RPC call; assert field is non-zero |
| REQ-2 | `UpdateGlobalDefaultsRequest` has `int32 autonomous_max_turns = 13` with hint comment | `TestUpdateGlobalDefaults_AutonomousMaxTurns_ExplicitValueRoundTrips` — send `AutonomousMaxTurns: 50`, assert response `Defaults.AutonomousMaxTurns == 50` | n/a (static; enforced by `make build`) | `TestUpdateGlobalDefaults_AutonomousMaxTurns_PersistsToConfig` — after `UpdateGlobalDefaults`, reload config from disk and assert `cfg.AutonomousMaxTurns == 50` |
| REQ-3 | `GetSessionDefaults` returns `AutonomousMaxTurnsOrDefault()`, never 0 | `TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault` — unset config → response value is 30, not 0 | `TestGetSessionDefaults_AutonomousMaxTurns_NeverZeroAfterExplicitZeroSave` — save 0 via `UpdateGlobalDefaults`, call `GetSessionDefaults`, assert response field is 30 (not 0) | (covered by REQ-1 integration test above) |
| REQ-4 | `UpdateGlobalDefaults` persists `autonomous_max_turns`; 0 resets to default | `TestUpdateGlobalDefaults_AutonomousMaxTurns_ExplicitValueRoundTrips` — send 50, response returns 50 | `TestUpdateGlobalDefaults_AutonomousMaxTurns_ZeroResetsToDefault` — send 0, response `AutonomousMaxTurns == 30` | `TestUpdateGlobalDefaults_AutonomousMaxTurns_PersistsToConfig` — isolated config dir; verify JSON on disk after save |
| REQ-5 | `GlobalDefaultsForm` shows numeric input "Max Autonomous Session Turns" bounded \[1, 200\] | `renders "Max Autonomous Session Turns" input with type number and min=1 max=200` — RTL: `screen.getByLabelText` resolves; `getAttribute("min")` == "1", `getAttribute("max")` == "200" | `clamps onChange value 201 to 200` — RTL: fire change event with `{target: {value: "201"}}`, assert input value is `200` | n/a |
| REQ-6 | Saving the form sends `autonomous_max_turns` in the request | `submit includes autonomousMaxTurns in updateGlobalDefaults call` — mock `updateGlobalDefaults`; fill field to 75; click Save; assert mock called with `autonomousMaxTurns: 75` | `submit after clamping still sends bounded value` — fill field to "999" (clamped to 200); click Save; assert mock called with `autonomousMaxTurns: 200` | n/a |
| REQ-7 | Stale "server default (3)" comments fixed to "(5)" | n/a (static; verified by `grep -c "server default (3)" session.proto` == 0 in CI) | n/a | n/a |
| REQ-8 | `GlobalDefaultsForm.test.tsx` has test for turn-cap rendering with server value | `shows the server-resolved Max Autonomous Session Turns default, not a client constant` — mock returns `autonomousMaxTurns: 45`; assert `screen.findByLabelText("Max Autonomous Session Turns")` has value 45 | `shows fallback 30 when server returns autonomousMaxTurns=0` — mock returns `autonomousMaxTurns: 0`; assert field value is 30 | n/a |
| REQ-9 | `defaults_service_test.go` covers both `GetSessionDefaults` and `UpdateGlobalDefaults` | `TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault` (see REQ-3) | `TestUpdateGlobalDefaults_AutonomousMaxTurns_ZeroResetsToDefault` (see REQ-4) | `TestUpdateGlobalDefaults_AutonomousMaxTurns_PersistsToConfig` (see REQ-4) |
| REQ-10 | `make build && make test` pass with no new failures | n/a (CI gate — all unit tests above must be green) | n/a | n/a |

---

## UX Acceptance Tests

All 17 criteria from `design/ux.md` § 6. Test medium for each: Jest + React Testing Library unless noted.

| # | UX Criterion | Test Name | Test Medium | Assertion |
|---|--------------|-----------|-------------|-----------|
| UX-1 | Field appears between "Max Auto-Rework Iterations" and "Max Concurrent Backlog Work Items" | `renders "Max Autonomous Session Turns" between the rework and concurrent fields` | RTL | `screen.getByLabelText("Max Autonomous Session Turns")` is in the DOM; its preceding sibling label contains "Rework" and its following sibling label contains "Concurrent" |
| UX-2 | Field displays 30 when server returns 0 (omitempty / unset) | `shows 30 as fallback when server returns autonomousMaxTurns=0` | RTL | Mock returns `autonomousMaxTurns: 0`; `findByLabelText(...)` resolves with value `30` |
| UX-3 | Field displays saved value (e.g. 50) when server returns it | `shows server-provided value 50 when loaded` | RTL | Mock returns `autonomousMaxTurns: 50`; `findByLabelText(...)` resolves with value `50` |
| UX-4 | Typing 201 results in 200 (upper bound clamped, no error message) | `clamps entered 201 to 200 without showing an error message` | RTL | Fire change `{target: {value: "201"}}`; assert input value `200`; assert no error alert role in document |
| UX-5 | Typing 0 or -1 results in 1 (lower bound clamped, no error message) | `clamps entered 0 to 1 without showing an error message` | RTL | Fire change `{target: {value: "0"}}`; assert value `1`; no error role |
| UX-6 | Clearing the field shows 1; Save button remains enabled | `shows 1 when field is cleared and Save stays enabled` | RTL | Fire change `{target: {value: ""}}`; assert value `1`; assert Save button not disabled |
| UX-7 | Typing 25 and saving completes in ≤ 3 clicks | `typing and saving requires no more than three user interactions` | RTL | `userEvent.type` on input, then `userEvent.click` Save — exactly 2 interactions after focus; assert mock called once with `autonomousMaxTurns: 25` |
| UX-8 | Clicking Save shows "Saving..." then "Global defaults saved." banner; banner auto-dismisses within 4s | `shows Saving... then success banner after save, which auto-dismisses` | RTL | Mock `updateGlobalDefaults` with deferred resolve; click Save; assert "Saving..." visible; resolve mock; assert "Global defaults saved." visible; advance timers 4 s; assert banner gone |
| UX-9 | Reloading the page shows the previously saved value | `reloading form after save shows persisted value` | Integration (Go service + full RTL re-mount) | Save 45 via mock; re-mount `<GlobalDefaultsForm />`; mock `getSessionDefaults` returns 45; assert field value is 45 |
| UX-10 | Save failure shows error banner; Save button re-enables; retry works | `shows error banner when save fails and re-enables Save for retry` | RTL | Mock `updateGlobalDefaults` rejects; click Save; assert "Failed to save defaults" banner; assert Save not disabled; click Save again; assert mock called twice |
| UX-11 | Load failure shows error banner; refreshing retries | `shows error banner when initial load fails` | RTL | Mock `getSessionDefaults` rejects; render; assert "Failed to load defaults" banner visible |
| UX-12 | Keyboard-only: Tab → type value → Tab → Enter saves without mouse | `supports keyboard Tab → type → Tab → Enter navigation` | RTL + `userEvent` | `userEvent.keyboard("{Tab}")` to focus input; type "25"; `userEvent.keyboard("{Tab}{Enter}")`; assert save mock called |
| UX-13 | Screen reader announces label + value on focus | `label is programmatically associated with input for screen reader` | RTL + axe | `getByLabelText("Max Autonomous Session Turns")` resolves (implicit `htmlFor`/`id` pairing); `axe.run` on the rendered node reports no violation for label association |
| UX-14 | Hint text follows input in natural reading order (no skip-link required) | `hint text follows input in DOM order` | RTL | `getByText(/How many turns/)` is a later sibling of the input within the same `<div className={field}>` container |
| UX-15 | WCAG AA contrast (≥ 4.5:1) for label and hint text | `Axe Core CI does not flag contrast violations on new field` | CI — `make e2e-lighthouse` (Axe Core) | Zero new Axe critical/serious violations in the `GlobalDefaultsForm` route |
| UX-16 | Mobile tap raises numeric keypad | `type="number" attribute set on input` | RTL | `getByLabelText(...)` `getAttribute("type")` == `"number"` |
| UX-17 | Touch target comparable to adjacent numeric fields | `input uses shared CSS class identical to adjacent fields` | RTL | `getByLabelText(...).className` includes the same `input` CSS module class as the `maxAutoReworkIterations` input |

---

## Test Stack

| Layer | Tool | Location |
|-------|------|----------|
| Go unit + integration | `testify` (`assert`/`require`), `connect-go` | `server/services/defaults_service_test.go` |
| Go test helpers | `newIsolatedDefaultsService(t)` — isolated config dir per test | `server/services/defaults_service_test.go` (existing helper) |
| React unit + UX acceptance | Jest + `@testing-library/react` + `@testing-library/user-event` | `web-app/src/components/settings/GlobalDefaultsForm.test.tsx` |
| Accessibility | `jest-axe` (`axe.run`) | same test file as React unit tests |
| WCAG contrast / Axe CI | Axe Core via `make e2e-lighthouse` (Playwright-driven) | `tests/e2e/` |
| Build gate | `make build && make test` | CI (`make ci`) |

---

## Coverage Targets

| Category | Count | Notes |
|----------|-------|-------|
| Unit tests — happy path | 9 | 3 Go (GetDefaults default, UpdateDefaults explicit, UpdateDefaults zero-reset) + 6 React (render, clamping high, clamping low, clear→1, submit wires value, submit with clamped value) |
| Unit tests — error path | 6 | 1 Go (GetDefaults never-zero after zero-save) + 5 React (fallback-30 on zero response, error banner on save failure, error banner on load failure, Save re-enables after failure, no error message shown on clamp) |
| Integration tests | 2 | `TestGetSessionDefaults_AutonomousMaxTurns_FieldPresentInResponse` + `TestUpdateGlobalDefaults_AutonomousMaxTurns_PersistsToConfig` |
| UX acceptance tests | 17 | All 17 criteria from `design/ux.md` § 6; UX-15 is CI/Axe, UX-16/17 are low-cost RTL attribute assertions |
| Requirements coverage | 10/10 | Every AC has at least one mapped test; REQ-7 is a static grep gate rather than a runtime test |
| Migration tests | N/A | No schema changes; proto `omitempty` is backward-compatible |
