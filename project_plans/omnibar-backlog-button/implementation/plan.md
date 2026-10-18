# Implementation Plan: omnibar-backlog-button

**Feature**: Finish promoting "Create Backlog Item" to a top-level button in the omnibar's bottom action row (the button itself already landed in `dfc17acf5`; this plan covers the gaps research found).
**Date**: 2026-10-17
**Status**: Ready for implementation
**ADRs**: None (no non-standard technology choices)
**Complexity**: 1 (Domain Glossary omitted)

Paths are relative to `web-app/src/components/sessions/` unless they start with `tests/`, `docs/`, or `web-app/`.

---

## Baseline and AC status (what is verified now)

Baseline run on HEAD `dfc17acf5` in this worktree (working-tree diff is `requirements.md` only):

| Check | Result |
|---|---|
| `cd web-app && npx jest --no-coverage --testPathPatterns="components/sessions"` | VERIFIED: 137 suites, 1631 tests, all pass (includes `__tests__/Omnibar.backlogCompose.test.tsx`) |
| `cd web-app && npx tsc --noEmit -p .` | VERIFIED: exit 0 |
| `cd web-app && pnpm run lint:duplicates` | VERIFIED: exit 0, 15 clones, 0.14% duplicated lines, equal to the `threshold` in `web-app/.jscpd.json`. **No headroom**: new tests must not copy a `jest.mock` block (see Task 3) |
| e2e (`tests/e2e`), full jest, `pnpm run lint` | NOT RUN |

| AC | Status at HEAD | Evidence / gap | Closed by |
|---|---|---|---|
| AC1 top-level button in the bottom action row, same row as the other action buttons | **Verified for creation mode.** Not applicable to discovery (no footer). | `OmnibarCreationPanel.tsx:1022-1043`: the button is a direct child of `<div className={footer}>`, sibling of Cancel / Create Session; unit test `OmnibarCreationPanel.test.tsx:56-64` asserts the same parent. Rendering is unconditional in the panel (`!isDiscoveryMode`, `Omnibar.tsx:2121`). Not verified: narrow widths (Task 1, 4). | Task 1, 4 |
| AC2 visible and selectable in every state the card is shown, no nested control | **Partly met.** "Selectable" is read as: mouse click, and Enter/Space once the button has focus. Mouse-selectable in creation and discovery. Gaps: (i) Enter on the focused button also submits the session form (Finding K), fixed by Task 2a; (ii) discovery + `@` command has no entry point (Finding D). **Not delivered, not built:** Tab-reachability of footer buttons. It is a pre-existing limitation shared with Cancel and Create Session (Tab is swallowed for session-type cycling), see Owner decision K. | Findings below. Backlog compose / review are separate early returns that replace the card (`Omnibar.tsx:1721`, `:1748`), not states of the card: out of scope. | Task 2a, 2b, 3, 4 |
| AC3 click triggers the same behavior as before | **Met by construction, not tested end to end.** | Both call sites pass the same callback: `onCreateBacklog={openBacklogCompose}` at `Omnibar.tsx:2084` (list row) and `:2127` (footer). No Omnibar-level test clicks the footer button. | Task 3, 4 |
| AC4 existing omnibar tests pass; new placement covered by test or running app | **Existing tests: verified (above).** New placement: unit-tested for creation mode only; running-app check not done. | Task 1 to 4 add unit and e2e coverage; Task 5 reruns every gate. | Task 3, 4, 5 |

## Findings the plan relies on (re-verified by reading the code)

- **K, keyboard (decision a).** `handleKeyDown` is attached to the modal wrapper (`Omnibar.tsx:1795-1800`) and has no `e.target` check. In creation mode with no dropdown open, `Tab` is `preventDefault`ed and cycles session type (`Omnibar.tsx:1217-1224`) and focus starts in the input, so **keyboard focus cannot reach the footer buttons**; this is pre-existing and shared with Cancel and Create Session. Fixing it would change Tab behavior for those buttons too, so it is out of scope (Owner decision K). Separately, plain `Enter` on a focused button reaches the fallthrough `handleSubmitRef.current()` (`Omnibar.tsx:1241-1247`) in addition to the button's own click. `handleSubmit` returns early unless `canSubmit` (`Omnibar.tsx:1413`, `if (!canSubmit || isSubmitting ...) return`; VERIFIED by adversarial review), so the double fire only matters when `canSubmit` is true. The same Enter defect affects Cancel and the mode-badge buttons (`OmnibarModeBadge.tsx:13-31`); this plan fixes it for the backlog button only. VERIFIED by reading; NOT reproduced in a browser.
- **L, layout (decision b).** `footer` is `display:flex; justifyContent:flex-end; gap:8; padding:16` with no `flexWrap` (`Omnibar.css.ts:238-245`); `.modal` has `overflowX:hidden` (`Omnibar.css.ts:61`). Three buttons can clip at phone width. The ~400px estimate in `research/pitfalls.md` is INFERRED, not measured.
- **D, discovery (decision c).** Discovery renders `OmnibarResultList` only when `isDiscoveryMode && !isAtDropdownVisible` (`Omnibar.tsx:2066`), where `isAtDropdownVisible = isDiscoveryMode && isAtCommand` (`:616`). The "Create Backlog Item (AI)" row is the last listbox option (`OmnibarResultList.tsx:150-172`; keyboard dispatch `Omnibar.tsx:1047`). In the `@` state neither the row nor any footer exists. VERIFIED by reading.
- **F, focus (decision f).** `BacklogComposeWidget` has `autoFocus` on the textarea (`components/backlog/BacklogComposeWidget.tsx:60`) and handles its own Escape/cancel (`:36-44`); on cancel `Omnibar.tsx:1765-1768` restores the draft into `input`, and the effect at `:1283-1287` refocuses `inputRef`. `formState` is `useState` in `Omnibar` (`:293`), so it should survive the swap; NOT verified that cancel leaves `modeState` in creation (Task 3 tests it).
- **S, seed.** `openBacklogCompose` seeds from `input.trim()` minus `backlog:` (`Omnibar.tsx:1030-1033`). The single `<input>` is shared by both modes, so in creation mode it can hold a path. Cancel round-trips the draft back into `input` (`:1766`), so nothing is lost.
- **Duplicate action in creation mode.** The shortcuts bar also renders a second "Create Session" button in creation mode (`Omnibar.tsx:2213-2222`). Pre-existing; left alone (not an AC).

---

## Pattern Decisions

Step 0.5 alternatives, then the choice:

1. *Literal reading: footer button in every state, including a new discovery footer.* Strength: AC1/AC2 hold literally in all states. Weakness: duplicates the list row two lines above it and adds a second control for one action (UX research).
2. *Keep the existing structure, close the verified gaps (chosen).* Strength: small, reversible, keeps the listbox keyboard path. Weakness: the `@` state needs a small fallback.
3. *Hoist a shared `OmnibarActionBar` rendered in all states.* Strength: one bottom row everywhere. Weakness: re-verifies every dropdown and the shortcuts bar; large regression surface for a one-button AC.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Enter isolation for the backlog button | `onKeyDown` inside `OmnibarBacklogButton` that calls `e.stopPropagation()` only for plain `Enter` (no meta/ctrl) and `Space`, so the modal-wide `handleKeyDown` never sees them | Event delegation hygiene, scoped narrowly | Modal-wide `HTMLButtonElement` target guard in `handleKeyDown` (also changes Enter/Tab for Cancel, the "Open as" radios, mode badges, "More") | Modal-wide guard changes pre-existing behavior of every button (review concern) and was needed only for Tab, which is now out of scope. Tab, Escape and Cmd/Ctrl+Enter are left to propagate untouched. `handleKeyDown` is not edited at all |
| Backlog button | Small presentational component `OmnibarBacklogButton` (props: `onClick`, `disabled?`, `className?`) | Extract Component | Inline JSX at two call sites | Two sites (panel footer, `@` fallback) need identical testid/label/classes and the same key isolation; keeps `Omnibar.tsx` growth to one line |
| Discovery entry point | Keep list row; add the button only where the row is hidden (`@` state) | Progressive disclosure | Always show a discovery button; new discovery footer | Avoids duplicating the action when the row is visible; closes the only no-entry-point state |
| Button disabling | Boolean `disabled={isSubmitting}` | n/a | Gating on `canSubmit` | The action needs no session fields (UX research) |

## Tech Debt Disposition

No `research/architecture.md` exists for this project. Known by inspection: `Omnibar.tsx` is a ~2250-line component and its wrapper-level `handleKeyDown` is target-unaware.

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `Omnibar.tsx` size | Large component; this change must not grow it materially | Isolate via seam | New `OmnibarBacklogButton` component keeps the fallback to one JSX line |
| `handleKeyDown` target-unaware (`Omnibar.tsx:1063-1248`) | Wrapper handler swallows Tab / double-fires Enter for any child button | Defer (only the backlog button is isolated, inside its own component) | A handler-level fix changes behavior for Cancel, Create Session, radios, badges; separate refactor, listed as Owner decision K |

## Migration Plan
N/A: complexity 1, no schema or data change.

## Observability Plan
N/A: complexity 1.

## Risk Control
N/A: complexity 1. Not feature-gated; rollback is a revert commit (frontend only, no proto or Go change). Do not use `make install-service` to try the change (restarts the live service); use the manual-instance recipe in `CLAUDE.md` (port block 62871/62872).

## Decisions (resolved here) and owner flags

| # | Item | Decision | Why | Owner decision? |
|---|---|---|---|---|
| a | Enter on a focused backlog button | `stopPropagation` for plain Enter/Space inside `OmnibarBacklogButton`; tests assert Enter opens compose and does not submit. No Tab change | See Finding K and the Pattern table | No |
| K | Tab-reachability of footer buttons (backlog, Cancel, Create Session) | **Not built.** Pre-existing limitation: Tab cycles session type from the input. Options if wanted: a dedicated shortcut (e.g. Cmd/Ctrl+Shift+B shown in the shortcuts bar), or let Tab leave the input after the last form field | Changing Tab affects Cancel and Create Session too; out of scope for a one-button AC | **Yes (flag):** decide whether to make footer buttons Tab-reachable as a separate change |
| b | Narrow-width clipping | `flexWrap: "wrap"` on `footer`; `marginRight: "auto"` on the backlog button so it sits left, apart from Cancel/Create Session | Primary stays right-most; wrapping beats clipping | No |
| c | Discovery has no footer | Keep the list row as the discovery entry point (AC2 satisfied there: top-level, selectable, not nested). Add the button in the discovery shortcuts bar **only when `isAtDropdownVisible`** (row hidden) | Closes the one state with no entry point without duplicating the action elsewhere | **Yes (flag):** if the owner reads AC1 literally ("footer button in discovery too"), change the render condition in Task 2b from `isAtDropdownVisible` to `isDiscoveryMode`; no other task changes |
| d | Label "Create Backlog Item" vs "Create Backlog Item (AI)" | Keep both as-is. The two never render together (row only in discovery, footer only in creation), so no strict-mode locator clash; the existing e2e locator `getByText('Create Backlog Item (AI)')` (`tests/e2e/omnibar-backlog-compose.spec.ts:22`) is untouched | Shorter footer label helps narrow widths; visible text equals accessible name (no `aria-label`) | **Yes (flag, low stakes):** append "(AI)" to the footer label if the owner wants one name |
| e | Disable while submitting | `disabled={isSubmitting}` only | Avoids switching modes mid-request; stays enabled when the form is invalid | No |
| f | Focus on compose open / back | No code change; add assertions: textarea has focus on open; after cancel the input has focus and creation form fields are intact | `autoFocus` and the refocus effect already exist (Finding F) | No |
| S | Seeding compose from a path in creation mode | Keep (AC3: same behavior as before); cancel restores it | Changing it alters the behavior AC3 says to keep | **Yes (flag, low stakes):** seed empty in creation mode if the owner prefers |
| T | 44px touch targets | Not changed; shared `button` style is 8px/16px (~35px), same as Cancel/Create Session | Not an AC; e2e asserts overflow only. Note as unverified | No |

## Unresolved Questions
- [ ] Tab-reachability of footer buttons (decision K) — pre-existing, shared with Cancel/Create Session, not built; blocks nothing — owner: Tyler
- [ ] AC1 literal reading for discovery (decision c) — blocks nothing; default is built, switch is one condition in Task 2b — owner: Tyler
- [ ] Footer label "(AI)" (decision d) and creation-mode seeding (decision S) — block nothing; defaults chosen — owner: Tyler

## Dependency Visualization

```
Task 1 (button component, footer layout/disabled)
   |
   v
Task 2a (Enter isolation, in button component) --> Task 2b (@ fallback)
                                 |
                  +--------------+--------------+
                  v                             v
        Task 3 (Omnibar unit tests)     Task 4 (e2e)
                  \                             /
                   +-----------+---------------+
                               v
                    Task 5 (docs + gates)
```

---

## Phase 1: Close the gaps

### Epic 1.1: Footer button behaves like a first-class action

**Goal**: The button does not double-fire Enter when focused, does not clip, and has an entry point in every card state. Tab-reachability is explicitly not part of this goal (Owner decision K).

#### Story 1.1.1: Footer layout and submitting state (decisions b, e)
**As a** user on a phone-width screen, **I want** the footer buttons to wrap rather than clip, **so that** Create Backlog Item, Cancel and Create Session are all visible and tappable.
**Acceptance Criteria**:
- Footer wraps and does not overflow horizontally at V2_PORTRAIT.
  - *Given* the omnibar in creation mode at the `V2_PORTRAIT` viewport, *When* the footer renders three buttons, *Then* each of `omnibar-create-backlog-button`, Cancel and `omnibar-create-session-button` has `x + width <= viewport width`, and no two of the three bounding boxes intersect. The backlog button is left-aligned (`marginRight: auto`); when the row wraps it may sit alone on the first line with Cancel/Create Session on the second, which is accepted.
- Button is disabled while a session is being created.
  - *Given* `OmnibarCreationPanel` with `isSubmitting = true` and `onCreateBacklog` set, *When* it renders, *Then* `omnibar-create-backlog-button` is disabled; with `isSubmitting = false` and `canSubmit = false` it is enabled.
**Files**: `OmnibarBacklogButton.tsx` (new), `OmnibarCreationPanel.tsx`, `Omnibar.css.ts`, `OmnibarCreationPanel.test.tsx`

##### Task 1: Extract button component, add wrap and disabled state (cost S)
- Create `OmnibarBacklogButton.tsx`: native `<button type="button" data-testid="omnibar-create-backlog-button">` with `buttonClass + buttonSecondary`, visible text "Create Backlog Item", props `onClick`, `disabled?`, `className?`. No `aria-label` (label-in-name). Include the Enter/Space `onKeyDown` isolation from Task 2a here so both call sites get it.
- Replace the inline button in `OmnibarCreationPanel.tsx:1024-1033` with it; pass `disabled={isSubmitting}` and the new `backlogButton` class.
- `Omnibar.css.ts`: add `flexWrap: "wrap"` to `footer` (l.238); export `backlogButton = style({ marginRight: "auto" })`.
- `OmnibarCreationPanel.test.tsx`: add disabled-while-submitting and enabled-when-`canSubmit`-false cases beside the existing block at `:55-68` (reuse `buildProps`; do not add a new `jest.mock` block).
- Files: the four above. Check `pnpm run lint:css-vars` is unaffected (vanilla-extract file, no raw CSS vars added).
- Run `pnpm run lint:duplicates` after this task (not only at Task 5) so any jscpd regression is attributed to one task.

#### Story 1.1.2: Enter on the focused backlog button does not also submit (decision a)
**As a** user who has focused the backlog button (by mouse click or programmatically), **I want** Enter to run only that button, **so that** the backlog action never also submits a session. Tab-reachability is NOT claimed (Owner decision K).
**Acceptance Criteria**:
- Enter on the focused button opens compose and does not submit.
  - *Given* creation mode with `canSubmit = true` (path `/home/tstapler/code/stapler-squad`, session title `fix-login`) and focus on `omnibar-create-backlog-button` (via `.focus()`), *When* a plain `Enter` keydown fires and the button's click activates, *Then* `backlog-compose-textarea` is visible and `onCreateSession` is not called.
- Existing input behavior is unchanged.
  - *Given* the same state with focus in the main input, *When* `Tab` is pressed, *Then* session type still cycles; *When* `Enter` is pressed, *Then* the form still submits.
- Cmd/Ctrl+Enter and Escape are not swallowed by the button.
  - *Given* focus on the backlog button, *When* `Escape` keydown fires, *Then* it still reaches the modal handler (not stopped).
**Files**: `OmnibarBacklogButton.tsx`

##### Task 2a: Enter/Space isolation inside `OmnibarBacklogButton` (cost XS)
- In `OmnibarBacklogButton`, add `onKeyDown`: if `(e.key === "Enter" && !e.metaKey && !e.ctrlKey) || e.key === " "` then `e.stopPropagation()`. Native button activation (click) still fires. One-line "why" comment: the modal-wide `handleKeyDown` would otherwise also run `handleSubmit` on Enter.
- Do not edit `Omnibar.tsx`'s `handleKeyDown`. Tab, Escape, Cmd/Ctrl+Enter propagate as before. Cancel, Create Session, radios and mode badges keep their current behavior (unchanged; Owner decision K).
- Tests make no keyboard-reachability claim: they focus the button directly only to exercise Enter handling.
- Files: `OmnibarBacklogButton.tsx` (same file as Task 1; do in the same wave)

#### Story 1.1.3: Entry point where the list row is hidden (decision c)
**As a** user typing an `@workflow` command in discovery mode, **I want** a visible Create Backlog Item button, **so that** the action is not missing in any state of the card.
**Acceptance Criteria**:
- Fallback shows only in the `@` state.
  - *Given* discovery mode with input `@deploy` (so `isAtDropdownVisible` is true), *When* the omnibar renders, *Then* `omnibar-create-backlog-button` is present in the bottom shortcuts bar; *Given* discovery mode with input `fix login` (list row visible), *Then* the button is absent and the list row is present.
- Same behavior on click.
  - *Given* discovery mode with input `@deploy`, *When* the fallback button is clicked, *Then* `backlog-compose-textarea` is visible with value `@deploy`.
**Files**: `Omnibar.tsx`

##### Task 2b: Render fallback in the discovery shortcuts bar (cost XS)
- In the `isDiscoveryMode` fragment (`Omnibar.tsx:2223-2232`) render `<OmnibarBacklogButton onClick={openBacklogCompose} className={createButton} />` when `isAtDropdownVisible`. Import the component; reuse existing `Omnibar.css.ts` exports only (`createButton`, already used by the creation-mode "Create Session" button in the same bar at `Omnibar.tsx:2213-2222`, gives it `marginLeft:auto` and the bar's button look). No new CSS. The `@`-state button is a sibling control in the shortcuts bar, which is accepted as slightly off-pattern (the bar is otherwise hints plus that one action) and is covered by the owner flag in decision c.
- Single render condition so decision c's alternative is a one-token change.
- Files: `Omnibar.tsx`

## Phase 2: Coverage, docs, gates

### Epic 2.1: Prove the placement and behavior

**Goal**: Each AC has a test that would fail if the behavior regressed.

#### Story 2.1.1: Omnibar-level unit tests (AC2, AC3)
**As a** maintainer, **I want** Omnibar-level tests of the footer and fallback buttons, **so that** wiring regressions in `Omnibar.tsx` are caught (today only the panel in isolation is tested).
**Acceptance Criteria**:
- Footer click opens compose and focuses the textarea (AC3).
  - *Given* the omnibar in creation mode (click the `Create` mode-badge button) with input `fix the flaky login test`, *When* `omnibar-create-backlog-button` is clicked, *Then* `backlog-compose-textarea` has value `fix the flaky login test` and has focus.
- Cancel preserves the creation form (decision f).
  - *Given* creation mode with `firstPrompt` = `run the tests`, *When* the footer button is clicked then `backlog-compose-cancel`, *Then* the creation panel is shown again, `firstPrompt` still equals `run the tests`, and the main input has focus.
- Story 1.1.2 (Enter) and Story 1.1.3 (`@` fallback) ACs above.
**Files**: `__tests__/Omnibar.backlogCompose.test.tsx`

##### Task 3: Add Omnibar-level tests (cost S)
- Add cases to the existing `__tests__/Omnibar.backlogCompose.test.tsx` (**do not create a new test file**: a copied `jest.mock` block would push jscpd over its 0.14% ceiling). Its `OmnibarResultList` mock only matters in discovery; creation mode renders the real panel.
- Use fixtures from `__tests__/omnibarTestFixtures.tsx`. The shared `useAtCommandSuggestions` mock returns `isAtCommand: false` (`omnibarTestFixtures.tsx:105`). For the `@` cases override it per test with `jest.requireMock("<the module path the fixture mocks>").useAtCommandSuggestions.mockReturnValue({ ...default return shape, isAtCommand: true })` (copy the default shape from the fixture at `:105`), and reset in `afterEach`/`beforeEach` so other cases keep `false`.
- The test file mocks `OmnibarResultList` with its own stub row. Therefore the `@`-state "fallback absent when the list row is visible" check must NOT assert the stub row (vacuous): in the unit test assert only that `omnibar-create-backlog-button` is absent for input `fix login` in discovery mode. The real "list row present" assertion lives in the e2e spec (Task 4).
- Cases: footer click seeds + focuses; cancel preserves form + refocuses input; plain Enter on the focused footer button opens compose and does not call `onCreateSession` (`canSubmit = true`); Escape keydown on the button still reaches the modal handler; input Tab cycles and input Enter submits (unchanged); `@` state shows the fallback and click opens compose seeded with `@deploy`; discovery `fix login` shows no backlog button. No test asserts Tab reachability.
- Run `pnpm run lint:duplicates` after this task as well.
- Files: `__tests__/Omnibar.backlogCompose.test.tsx`

#### Story 2.1.2: Running-app coverage (AC1, AC2, AC4)
**As a** maintainer, **I want** an e2e check in a real browser, **so that** layout and keyboard behavior are verified where jsdom cannot.
**Acceptance Criteria**:
- Footer button works end to end (AC1, AC3).
  - *Given* the omnibar opened and switched to creation mode via the `Omnibar mode` group's `Create` button, *When* `omnibar-create-backlog-button` is clicked, *Then* `backlog-compose-textarea` is visible and focused.
- Enter on the focused button opens compose (AC2, "selectable once focused"). No Tab-reachability claim.
  - *Given* creation mode with `omnibar-create-backlog-button` focused (`.focus()`), *When* `Enter` is pressed, *Then* the compose textarea is visible and focused. ("No session created" is owned by the Task 3 unit test.)
- Discovery list row is present when the `@` fallback is absent (AC2).
  - *Given* discovery mode with input `fix login`, *Then* the real `Create Backlog Item (AI)` row is visible and `omnibar-create-backlog-button` is absent.
- No clipping or overlap at phone width (AC1).
  - *Given* creation mode at `V2_PORTRAIT`, *Then* the three footer buttons have `x + width <= viewport width` and their `boundingBox()` rectangles pairwise do not intersect.
**Files**: `tests/e2e/omnibar-backlog-compose.spec.ts`

##### Task 4: Extend the e2e spec (cost S)
- Add a `test.describe` for the footer button to the existing spec (already has the required `// @feature backlog:compose-widget, ...` header, `V1_DESKTOP`/`V2_PORTRAIT` loop, and the `openCompose` setup pattern). Locators: `getByTestId('omnibar-create-backlog-button')`, `getByRole('group', { name: 'Omnibar mode' }).getByRole('button', { name: 'Create', exact: true })`, `getByRole('button', { name: 'Cancel' })`. No `waitForTimeout`; use `expect(...).toBeFocused()` / `toBeVisible()`.
- Keep the Enter assertion to what is observable without a negative-request check: compose textarea visible and focused (the unit test in Task 3 owns "`onCreateSession` not called"). Do not press Tab to reach the button; use `.focus()` or click and name the test as "Enter on focused button", not "keyboard reachable".
- Overlap check: read `boundingBox()` of the three footer buttons and assert pairwise non-intersection (`a.x + a.width <= b.x || b.x + b.width <= a.x || a.y + a.height <= b.y || b.y + b.height <= a.y`) in addition to the `x + width <= viewport` check.
- Add the real-list-row check from Story 2.1.2 (discovery, input `fix login`).
- Run: `cd tests/e2e && npx playwright test omnibar-backlog-compose.spec.ts` (global-setup starts its own isolated server on a free port; do not start one or touch `:8543`).
- Files: `tests/e2e/omnibar-backlog-compose.spec.ts`

### Epic 2.2: Docs and gates

#### Story 2.2.1: Registry doc and full gate run (AC4)
**As a** maintainer, **I want** the registry doc to match the UI and every gate green, **so that** the PR passes CI.
**Acceptance Criteria**:
- Registry doc names both entry points.
  - *Given* `docs/reference/feature-testing-registry.md:19`, *When* updated, *Then* it says the compose mode is opened by the discovery list row, the creation-mode footer button, and the `@`-state shortcuts-bar button, all via testid `omnibar-create-backlog-button` for the buttons.
- Gates pass.
  - *Given* the finished change, *When* the gates run, *Then*: jest `components/sessions` passes with no failures (baseline 137 suites / 1631 tests, plus new cases); `npx tsc --noEmit -p .` exits 0; `pnpm run lint:duplicates` exits 0 (still <= 0.14%); `make registry-diff` reports no unexpected change.
**Files**: `docs/reference/feature-testing-registry.md`

##### Task 5: Update doc, run gates (cost XS)
- Edit `docs/reference/feature-testing-registry.md` line 19 ("Special case: compose mode"). `session-creation-registry.md` needs no change (no backlog mention; not a session creation mode).
- Run, in `web-app/`: the jest command above, `npx tsc --noEmit -p .` (check the exit code, not just output), `pnpm run lint:duplicates` (check the exit code), `pnpm run lint`. From repo root: `make registry-diff`.
- No `OmnibarAction`/`DetectorRegistry` change (same handler, no new action or detector).
- Files: `docs/reference/feature-testing-registry.md`

---

## Effort estimate

Assumed mix per run: 12% output, 60% cache read, 28% fresh input, so 1 raw token is about 0.94 CU (`0.28 + 0.6x0.1 + 0.12x5`). Classes per `ESTIMATION.md` (bands INFERRED).

| Story | Tasks | Raw tokens | Cost units (CU) |
|---|---|---|---|
| 1.1.1 Layout, submitting state | Task 1 (S) | ~50k | ~47k |
| 1.1.2 Enter isolation | Task 2a (XS) | ~25k | ~24k |
| 1.1.3 `@` fallback | Task 2b (XS) | ~25k | ~24k |
| 2.1.1 Omnibar unit tests | Task 3 (S) | ~60k | ~56k |
| 2.1.2 e2e | Task 4 (S) | ~60k | ~56k |
| 2.2.1 Docs and gates | Task 5 (XS) | ~25k | ~24k |
| **Subtotal** | 6 tasks | ~245k | ~231k |
| **With verification multiplier x1.5 to x2** | | | **~0.35M to ~0.46M CU** (Small band) |

- **Critical path**: 4 agent waves (Task 1, then 2a to 2b, then 3 in parallel with 4, then 5). Tasks 3 and 4 touch different files and can run concurrently.
- **Overrun checkpoint**: if spend on completed stories exceeds the table value by more than 25%, pause and report the delta and cause (informational).
- **Wall-clock blockers**: none required. Owner flags (decisions c, d, S) have defaults built and do not block. The e2e run needs a Chromium-capable environment (see `docs/how-to/fix-playwright-chromium-install-stall.md` if the install stalls).

## Traceability

| AC | Tasks | Test that fails if it regresses |
|---|---|---|
| AC1 | 1, 4 | `OmnibarCreationPanel.test.tsx` same-parent assertion (exists); e2e footer overflow and no-overlap check. Discovery relies on the requirements' own carve-out (list row is the discovery entry point; no footer there) |
| AC2 | 2a, 2b, 3, 4 | Mouse click and Enter-on-focused-button tests, `@`-state fallback tests (unit); e2e real list row. Tab-reachability not delivered (Owner decision K) |
| AC3 | 3, 4 | Omnibar-level footer click seeds compose; e2e click opens compose |
| AC4 | 3, 4, 5 | Full `components/sessions` jest run, tsc, jscpd, e2e spec |
