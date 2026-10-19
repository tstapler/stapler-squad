# Validation Plan: omnibar-backlog-button

**Date**: 2026-10-17
**Complexity**: 1 (one test per requirement/AC; the unit + error + integration triad is skipped)

Paths: `UT` = `web-app/src/components/sessions/`; `E2E` = `tests/e2e/omnibar-backlog-compose.spec.ts`. "Exists" means the test is already in HEAD `dfc17acf5` (verified by listing test names); "New" means a plan task adds it.

## Happy Path Scenario
Given the omnibar open in creation mode with text typed in the main input (baseline: HEAD already renders `omnibar-create-backlog-button` in the footer), when the user clicks the footer "Create Backlog Item" button, then the backlog compose widget replaces the card with its textarea seeded from the typed text and focused, and Cancel returns to the creation form with the draft and form fields intact.

## Requirement -> Test Mapping

Requirements come from `requirements.md` (AC1-AC4). Every AC maps to a concrete test or an explicit running-app step.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: top-level button in the bottom action row, same row as the other actions | `UT OmnibarCreationPanel.test.tsx` | `omnibar-create-backlog-button (footer action) > renders in the footer next to Create Session and fires onCreateBacklog` (Exists, `:56`) | Unit | Same-parent assertion (button is a sibling of Cancel / Create Session) |
| AC1: no clipping or overlap at phone width | `E2E` | `footer buttons do not clip or overlap at V2_PORTRAIT` (New, Task 4) | E2E | Creation mode at `V2_PORTRAIT`: each of the three footer buttons has `x + width <= viewport width`; pairwise `boundingBox()` rectangles do not intersect |
| AC1: discovery has no footer | Running app, step R3 below | n/a | Manual | Carve-out: the discovery list row is the entry point (decision c); owner flag, not tested as a footer |
| AC2: visible and selectable in every card state, no nested control (creation: mouse) | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar footer backlog button > opens compose seeded with the typed text and focuses the textarea when clicked` (New, Task 3) | Unit | Click via the `Create` mode badge then the footer button |
| AC2: Enter on the focused button selects it and does not also submit | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar footer backlog button > opens compose and does not call onCreateSession when Enter is pressed on the focused button` (New, Task 3) | Unit | `canSubmit = true`; `button.focus()` then `await userEvent.keyboard("{Enter}")`. **Must use `userEvent.keyboard`, not bare `fireEvent.keyDown`**: `fireEvent.keyDown` dispatches only the keydown event and never the browser's Enter-to-click activation, so the test would pass or fail without exercising the button's click and could not prove "opens compose". Import `@testing-library/user-event` (installed, `^14.5.2`; this test file does not yet import it) and `setup()` one user instance |
| AC2: Enter isolation does not swallow Escape | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar footer backlog button > lets Escape keydown on the button reach the modal handler` (New, Task 3) | Unit | `fireEvent.keyDown(button, { key: "Escape" })` is acceptable here: only propagation is asserted, no activation |
| AC2: input behavior unchanged (Tab cycles type, Enter submits) | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar footer backlog button > leaves Tab session-type cycling and Enter submit on the main input unchanged` (New, Task 3) | Unit | Regression guard for the key-isolation change |
| AC2: Cmd/Ctrl+Enter on the focused backlog button still submits the session | none (accepted, pre-existing) | n/a | Accepted | The isolation deliberately stops only plain Enter/Space (`!metaKey && !ctrlKey`); Cmd/Ctrl+Enter keeps propagating to the modal-wide `handleKeyDown`, which is the existing submit shortcut. Review minor folded in: accepted as pre-existing behavior, no test, documented so a reviewer does not read it as a missed case |
| AC2: `@` state has an entry point | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar @-command fallback > shows the backlog button only when an @ command hides the list row and opens compose seeded with @deploy on click` (New, Task 3) | Unit | Override `useAtCommandSuggestions` to `isAtCommand: true` per test (reset after) |
| AC2: no duplicate button when the list row is visible | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar @-command fallback > does not render the backlog button in discovery mode with plain input` (New, Task 3) | Unit | Input `fix login`: assert only that `omnibar-create-backlog-button` is absent (the file stubs `OmnibarResultList`, so a list-row assertion would be vacuous) |
| AC2: real discovery list row present | `E2E` | `discovery shows the Create Backlog Item (AI) row and no footer backlog button` (New, Task 4) | E2E | Discovery, input `fix login`: real row visible, `omnibar-create-backlog-button` absent |
| AC2: Tab-reachability | none | n/a | Not delivered (Owner decision K) | Pre-existing limitation shared with Cancel / Create Session; no test claims it |
| AC2 / decision e: disabled only while submitting | `UT OmnibarCreationPanel.test.tsx` | `omnibar-create-backlog-button (footer action) > is disabled while isSubmitting and enabled when canSubmit is false` (New, Task 1) | Unit | `isSubmitting = true` -> disabled; `isSubmitting = false, canSubmit = false` -> enabled. Reuse `buildProps`; no new `jest.mock` block |
| AC3: click triggers the same behavior as before (footer) | `UT __tests__/Omnibar.backlogCompose.test.tsx` | same test as the AC2 mouse row: `opens compose seeded with the typed text and focuses the textarea when clicked` (New, Task 3) | Unit | Same `openBacklogCompose` seeding as the list row (existing seeding tests: `pre-fills the compose textarea with text already typed`, `strips a leading backlog: prefix when seeding the compose textarea`, Exist) |
| AC3: cancel preserves the creation form and refocuses the input | `UT __tests__/Omnibar.backlogCompose.test.tsx` | `Omnibar footer backlog button > returns to the creation panel with firstPrompt intact and the input focused on cancel` (New, Task 3) | Unit | `firstPrompt = "run the tests"`; footer click, then `backlog-compose-cancel` |
| AC3: end to end in a real browser | `E2E` | `footer button opens compose with a focused textarea` (New, Task 4) | E2E | `Omnibar mode` group `Create` button, click the footer button, textarea visible and `toBeFocused()` |
| AC3: Enter on the focused button in a real browser | `E2E` | `Enter on focused footer button opens compose` (New, Task 4) | E2E | `.focus()` then `page.keyboard.press("Enter")`; textarea visible and focused. Named "Enter on focused button", not "keyboard reachable". "No session created" is owned by the unit test |
| AC4: existing omnibar tests pass | `UT` whole `components/sessions` suite | `cd web-app && npx jest --no-coverage --testPathPatterns="components/sessions"` | Regression | Baseline 137 suites / 1631 tests all pass (VERIFIED in plan.md); must stay green with the new cases added |
| AC4: new placement covered by test or running app | rows above (unit + E2E) plus running-app steps R1-R4 | n/a | Mixed | See next section |

## Running-App Verification (explicit "verified in running app" steps)

Use the manual-instance recipe from `CLAUDE.md` (build to `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, `PORT=62871`, `STAPLER_SQUAD_INSTANCE=claude-manual-test`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, `--tmux-keep-server`). Never `make install-service` (restarts the live service). These are human-verifiable and cover what jsdom cannot.

| Step | AC | Action | Expected |
|---|---|---|---|
| R1 | AC1 | Open the omnibar, switch to creation mode via the `Create` mode badge | "Create Backlog Item" sits in the bottom footer row with Cancel and Create Session, left-aligned, Create Session right-most |
| R2 | AC1 | Resize the browser to ~375px wide in creation mode | Footer wraps; all three buttons fully visible, none clipped or overlapping (corroborates the E2E overlap test; also covers the INFERRED ~400px clip estimate in `research/pitfalls.md`) |
| R3 | AC1, AC2 | Open discovery mode, type `fix login` | The list row "Create Backlog Item (AI)" is the entry point; no footer button duplicates it |
| R4 | AC2 | In discovery type `@deploy` | The shortcuts bar shows "Create Backlog Item"; clicking it opens compose seeded with `@deploy` |
| R5 | AC3 | Compare behavior: open compose from the discovery row and from the creation footer | Same compose widget, same seeding, Cancel restores the draft in both |
| R6 | AC2 | Creation mode with a valid path and title, click into the footer button then press Enter | Compose opens; no session is created (check the session list is unchanged) |
| R7 | AC2 (accepted gap) | Press Cmd/Ctrl+Enter with the footer button focused | Session submit still fires (accepted, pre-existing; record the observation, not a defect) |

## UX Acceptance Tests
No `design/ux.md` exists for this project. UX-relevant criteria are covered by the AC rows above (touch targets are not changed and not asserted: decision T, UNVERIFIED beyond the existing e2e `meets touch-target size` check at `E2E:32` which covers the compose widget's controls, not the footer button).

## Test Stack
- **Unit**: Jest + React Testing Library + `@testing-library/user-event` (`^14.5.2`) for any test that depends on key activation; fixtures from `__tests__/omnibarTestFixtures.tsx`.
- **Integration**: none (no data store or external call; frontend-only change, same `openBacklogCompose` handler).
- **E2E / UX**: Playwright in `tests/e2e/` (isolated server auto-started by `global-setup.ts`); manual checklist R1-R7 above.

## Test-Authoring Constraints (from plan.md and review)
- Add all Omnibar-level cases to the existing `__tests__/Omnibar.backlogCompose.test.tsx`; do not create a new test file (a copied `jest.mock` block would push jscpd over its 0.14% ceiling; baseline has zero headroom).
- Enter test uses `userEvent.keyboard("{Enter}")` after `.focus()`, never bare `fireEvent.keyDown` (review minor).
- `@` cases override `useAtCommandSuggestions` per test and reset it so other cases keep `isAtCommand: false`.
- E2E: no `waitForTimeout`, `data-testid`/ARIA locators only, header `// @feature` already present.
- Run `pnpm run lint:duplicates` after Task 1 and Task 3, not only at Task 5.

## Coverage Targets and How to Measure

| Stack | Command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --no-coverage --testPathPatterns="components/sessions"` | 0 failures; every AC row above has a passing test |
| Types | `cd web-app && npx tsc --noEmit -p .` | exit 0 |
| Duplication | `cd web-app && pnpm run lint:duplicates` | exit 0 (<= 0.14%) |
| E2E | `cd tests/e2e && npx playwright test omnibar-backlog-compose.spec.ts` | all pass |
| Registry | `make registry-diff` | no unexpected change |

- Coverage is requirement-based (each AC has a mapped test or running-app step), not a line-percentage gate; a global 80% line threshold is not meaningful for a one-component UI change.

## AC Coverage Summary

| AC | Mapped to |
|---|---|
| AC1 | `OmnibarCreationPanel.test.tsx` same-parent test (Exists); E2E overflow/overlap test; running-app R1, R2, R3 |
| AC2 | Omnibar click test, Enter test, Escape test, `@` fallback test, no-duplicate test, panel disabled test (all New); E2E list-row and Enter tests; running-app R3, R4, R6; Tab-reachability explicitly not delivered; Cmd/Ctrl+Enter accepted (R7) |
| AC3 | Omnibar click + cancel tests (New); existing seeding tests; E2E footer click test; running-app R5 |
| AC4 | Full `components/sessions` jest run, tsc, jscpd, E2E spec; running-app R1-R7 |
