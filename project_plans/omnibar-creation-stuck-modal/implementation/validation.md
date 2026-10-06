# Validation Plan: omnibar-creation-stuck-modal

**Date**: 2026-08-06

## Happy Path Scenario
Given a user submits an alias invocation (`@pw retirement`) or a `>shell` command in the
Omnibar, when `onCreateSession` resolves successfully and `onClose()` correctly unmounts the
modal, then the modal closes cleanly with no stuck "Creating…" state and no error shown.

## Requirement → Test Mapping

All tests are React Testing Library component tests against the real `Omnibar` component
(no data store or network call involved — "unit" in this codebase means RTL-rendered
component test, not a pure-function test). File: `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx`
unless noted (per plan.md Task 1.2.2a, a SpawnShell-specific test may instead live in a new
sibling `Omnibar.spawnShell.test.tsx` if one already exists at implementation time — check
first).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1 (Alias-invocation branch resets `isSubmitting` even when `onClose` fails to dismiss) | `Omnibar.alias.test.tsx`, `describe("Omnibar alias submit resets isSubmitting even when onClose is a no-op")` | `"re-enables the Create button after a successful @alias submission even when onClose does not unmount the modal"` | Component (RTL), happy | Type `@ssq retirement`, submit via Ctrl+Enter with `onClose: jest.fn()` (no-op) and `onCreateSession` resolving; assert the Create button is not disabled and its text is not `"Creating…"` after the promise settles. |
| AC2 (SpawnShell branch — same guarantee) | `Omnibar.alias.test.tsx` (or `Omnibar.spawnShell.test.tsx`), `describe("Omnibar spawn-shell submit resets isSubmitting even when onClose is a no-op")` | `"re-enables the Create button after a successful >shell submission even when onClose does not unmount the modal"` | Component (RTL), happy | Type `>shell ~/project`, submit, `onClose` no-op, `onCreateSession` resolves; assert button re-enabled, not stuck "Creating…". |
| AC3 (happy path unchanged — alias branch, `onClose` works) | same describe as AC1 | `"closes cleanly with no error banner shown when onClose successfully dismisses after a successful @alias submission"` | Component (RTL), happy | Submit `@ssq retirement` with a working `onClose` mock (asserted called); assert no error text is rendered at any point (`screen.queryByText` for the error region returns null). |
| AC3 (happy path unchanged — spawn-shell branch, additional/nice-to-have; requirement text is branch-agnostic so include for parity even though the plan's ACs only spell it out for the alias branch) | same describe as AC2 | `"closes cleanly with no error banner shown when onClose successfully dismisses after a successful >shell submission"` | Component (RTL), happy | Same as above, `>shell` input. |
| AC4 (failure path unchanged — alias branch) | `describe("Omnibar alias submit failure path")` | `"shows the error message and re-enables the Create button when onCreateSession rejects for an @alias submission"` | Component (RTL), error | `onCreateSession: jest.fn().mockRejectedValue(new Error("boom"))`; submit `@ssq retirement`; assert error text `"boom"` renders, button re-enabled, modal stays open (`onClose` not called). |
| AC4 (failure path unchanged — spawn-shell branch) | `describe("Omnibar spawn-shell submit failure path")` | `"shows the error message and re-enables the Create button when onCreateSession rejects for a >shell submission"` | Component (RTL), error | Same as above, `>shell ~/project` input. |
| AC5 (regression test: alias success path resets `isSubmitting` with `onClose` as no-op) | same as AC1's test | *is* the AC1 test above (Task 1.2.1a names this test itself as the deliverable, not a separate one) | Component (RTL), regression | Identical to the AC1 row. Recommended verification per plan.md Task 1.2.1a: temporarily revert the `finally` fix locally and confirm this test fails on the pre-fix code, then re-apply the fix and confirm it passes. |
| AC6 (root cause of `onClose()` non-dismissal fixed or filed as tracked follow-up) | N/A — process/documentation AC, not a unit-testable behavior | — | Manual verification | Two mechanisms per plan.md's Unresolved Questions: **mechanism 1** (state leak — `isSubmitting` never reset on close) is code-fixed by Task 1.1.3a and covered by the bonus "reset-on-close effect" test below; **mechanism 2** (`position: fixed` overlay without `createPortal`, `Omnibar.css.ts:21-22`) is unresolved by this plan and must be confirmed filed as a real backlog item (Story 3.1.2/Task 3.1.2a) before this AC can be marked satisfied — check for the item's existence rather than assuming it was created. |
| Defense-in-depth (Story 1.1.3, not independently numbered in requirements.md but required by the plan and by AC6/AC9) | `describe("Omnibar reset-on-close effect")` | `"resets isSubmitting to false when the modal transitions from open to closed, even if a submission was left in flight"` | Component (RTL), regression | Start a submission with `onCreateSession` returning a never-resolving promise (simulating a stuck/in-flight state) so `isSubmitting` is `true`; rerender with `isOpen={false}` then `isOpen={true}`; assert the Create button is enabled and reads `"Create Session"`, not `"Creating…"`. This is also the automated proxy for UX AC-9 below. |
| Escape/overlay-click never gated on `isSubmitting` (Story 1.1.4 — plan.md scopes this as grep-verify only, no test required, but a regression test is cheap and prevents a future accidental regression) | `describe("Omnibar dismiss affordances stay live during submission")` (optional, recommended) | `"calls onClose when Escape is pressed while a submission is in flight"` / `"calls onClose when the overlay backdrop is clicked while a submission is in flight"` | Component (RTL), regression, optional | Start a submission with a never-resolving `onCreateSession` promise; fire `Escape` keydown / click the overlay; assert `onClose` was called. Not required by any numbered AC but directly backs UX AC-5/AC-6 with an automated check instead of relying on grep alone. |

## UX Acceptance Tests

Per `.claude/rules/e2e-test-conventions.md` / this codebase's actual convention for this
component: Jest + React Testing Library, colocated in `__tests__/`, not Playwright — this is
pure client-side component state, no server round-trip to drive through a real browser.
AC-7's screen-reader claim is only partially automatable (RTL can assert ARIA attributes,
not that a real AT actually vocalizes them) so it's listed as RTL-automatable-proxy +
recommended manual pass.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-1 (alias branch stuck-modal recovery) | `Omnibar.alias.test.tsx` | `"never leaves the Create button stuck on Creating… after a successful @alias submission, even if the modal fails to visually close"` | Jest/RTL | Same scenario as unit AC1 test above — type `@ssq retirement`, submit, `onClose` no-op, `onCreateSession` resolves; assert button is not disabled and label is not `"Creating…"` within one render after the promise settles. |
| AC-2 (SpawnShell branch stuck-modal recovery) | `Omnibar.alias.test.tsx` / `Omnibar.spawnShell.test.tsx` | `"never leaves the Create button stuck on Creating… after a successful >shell submission, even if the modal fails to visually close"` | Jest/RTL | Same as AC-1, `>shell ~/project` input. |
| AC-3 (happy path unchanged — no error flash, no lingering confirmation) | `Omnibar.alias.test.tsx` | `"shows no error banner and no lingering confirmation state when onClose correctly dismisses after a successful submission"` | Jest/RTL | Submit with a working `onClose` mock; assert `onClose` called exactly once, no error text ever rendered (`queryByText` for error region stays null across the whole `act` block, not just at the end). |
| AC-4 (failure path unchanged) | `Omnibar.alias.test.tsx` | `"shows the error banner and re-enables Create Session, with the modal staying open, when submission fails"` | Jest/RTL | Same as unit AC4 tests — assert error text present, button re-enabled, `onClose` NOT called (modal stays open). |
| AC-5 (no dead ends — Escape) | `Omnibar.alias.test.tsx` | `"calls onClose when Escape is pressed while isSubmitting is true (submission in flight or stuck)"` | Jest/RTL | Start submission with a never-resolving `onCreateSession` promise so `isSubmitting` stays `true`; fire `keydown` `Escape`; assert `onClose` was called. Covers both "in-flight" and "stuck" cases since both present as `isSubmitting === true`. |
| AC-6 (no dead ends — overlay click) | `Omnibar.alias.test.tsx` | `"calls onClose when the overlay backdrop is clicked while isSubmitting is true"` | Jest/RTL | Same setup as AC-5; click the overlay element (outside the modal content — confirm test targets the actual overlay node at `Omnibar.tsx:1216`, not the inner modal div which stops propagation); assert `onClose` called. |
| AC-7 (screen-reader announcement) | `Omnibar.alias.test.tsx` (automated proxy) + manual pass | `"sets aria-busy=true on the submit button and exposes an aria-live=polite region announcing Creating… while a submission is in flight"` | Jest/RTL (attribute assertions) + manual VoiceOver/NVDA pass | RTL: assert `button.getAttribute("aria-busy") === "true"` while submitting and `"false"` (or attribute absent) once settled; assert an `aria-live="polite"` element's text content transitions from idle → `"Creating…"` → success/error text. **Gap**: this criterion depends on plan.md's Task 2.1.1a (Phase 2, explicitly optional/droppable) — if Phase 2 is dropped, this test cannot pass and AC-7 is unmet; flag this dependency to whoever runs implementation. Manual pass: run a real screen reader through the same flow once, since RTL cannot confirm actual vocalization. |
| AC-8 (duplicate-resubmission guard — "Created ✓" beat) | `Omnibar.alias.test.tsx` (would be new) | `"shows a distinguishable submitted/confirmed state after a successful creation that the modal does not auto-close, so the form cannot be mistaken for untouched and resubmitted"` | Jest/RTL (once implemented) + manual pass | Submit successfully with `onClose` a no-op; assert the button/form is in some visibly-non-idle "submitted" state (e.g. text `"Created ✓"`, or inputs `disabled`/inert) rather than reverting straight to an enabled, blank `"Create Session"` state. **Gap — no implementing task exists**: ux.md recommends this state (Surface A extended) but plan.md's task list (Phase 1 core fix + Phase 2 aria polish) does not include a task that adds a "Created ✓" or equivalent confirmed-state UI; Phase 2 only adds `aria-busy`/`aria-live` to the existing idle/loading states. This test will fail against the plan as currently scoped — either add a task for it before implementation, or explicitly downgrade AC-8 to "not implemented, tracked as follow-up" the same way AC6/mechanism-2 was handled. |
| AC-9 (cross-session state isolation on reopen) | `Omnibar.alias.test.tsx` | `"starts in a clean idle state (no leftover Creating… or error) when reopened after an earlier submission on the same long-lived instance"` | Jest/RTL | Submit successfully (or with a stuck in-flight promise) so `isSubmitting`/`error` are non-default; rerender the same component instance with `isOpen={false}` then `isOpen={true}` (mirrors the long-lived-instance behavior documented in plan.md's Domain Glossary — `OmnibarContext.tsx` never unmounts `Omnibar`); assert the Create button reads `"Create Session"`, is enabled, and no error text is present. This is the same underlying mechanism as the "reset-on-close effect" unit test above — AC-9 is its UX-facing restatement. |

## Test Stack
- **Unit / Component**: Jest + React Testing Library, colocated in `web-app/src/components/sessions/__tests__/`, following the existing mocking pattern in `Omnibar.alias.test.tsx` (mocked hooks: `usePathCompletions`, `usePathHistory`, `useAliases`, `useAliasSuggestions`, `useAtCommandSuggestions`, `useAvailablePrograms`, `useSlashCommands`, `useSlashCommandSuggestions`, `next/navigation`, `ThemeContext`, `@/lib/config`, `@/lib/store*`, `OmnibarResultList`, `@/lib/api/transport`). Uses `jest.useFakeTimers()` for the 150ms detection debounce and `act()`-wrapped submissions.
- **Integration**: N/A — no data store or external call in this fix. `onCreateSession`/`onClose` are injected as mocked callback props; the real ConnectRPC/session-service layer is never exercised by these tests.
- **E2E / UX**: No new Playwright spec required — this is a pure client-side state bug, not a cross-service flow, and ux.md's own "Follow-Up Suggestions" explicitly defers a mobile-viewport Playwright reproduction given the high spin-up cost for what is a unit-testable state bug. AC-7's screen-reader vocalization and AC-8's visual "Created ✓" polish should still get one manual human pass each before shipping, per the table above.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}' --testPathPatterns="Omnibar"` | ≥80% line (repo-wide default; this fix only touches ~10 lines across 2 branches + 1 effect, so the realistic bar is "every new/changed line hit by a test," not the percentage itself) |

- All public service methods: N/A — no service layer touched, this is component-internal state (`isSubmitting`, `error`) inside `handleSubmit`.
- All external integrations: N/A.
- UX acceptance criteria: 7 of 9 (AC-1–AC-6, AC-9) have a direct implementing task in plan.md and a corresponding test above. AC-7 depends on plan.md's optional Phase 2 task (Task 2.1.1a) — test exists but will only pass if that task ships. AC-8 has no implementing task in plan.md at all — the test is designed per ux.md but cannot pass against the plan as currently scoped; this gap should be resolved (add a task, or explicitly re-scope AC-8 to a follow-up) before claiming UX validation complete.

## Migration Plan
N/A — this is a pure frontend bug fix (React component state only). No schema, database, or API/proto changes are made by this plan (confirmed against plan.md: no proto, ent, or migration tasks appear in any phase).

## Root-Cause Coverage Caveat (pre-mortem.md P1 #2)
**The RTL/jsdom regression suite above (Tasks 1.2.1a/1.2.2a and the reset-on-close-effect
test) verifies mechanism 1 (the `isSubmitting` state leak) only.** jsdom does not perform
real CSS layout, so no test in this plan can detect or rule out mechanism 2 (the
`position: fixed`-without-`createPortal` hypothesis, `Omnibar.css.ts:21-22`). A fully green
CI run on this PR proves the state-leak mechanism is fixed; it must **not** be read as proof
that the originally reported mobile symptom is fully root-caused, since mechanism 2 remains
only narrowed (see plan.md's Unresolved Questions — static-CSS ancestor check done, dynamic
mobile-viewport-transform case still unverified), not eliminated. Treat "tests pass" and
"root cause confirmed" as two separate claims when reviewing/merging this PR.

## Summary of Gaps Found During Validation Design
- **AC6** requires confirming, not assuming, that Story 3.1.2's backlog item for the `position:fixed`/no-`createPortal` mechanism actually gets filed — it's a process AC, not something a Jest test can enforce.
- **AC-7** (screen-reader parity) is only satisfied if the optional Phase 2 task ships; it's explicitly droppable per plan.md, which would leave AC-7 unmet.
- **AC-8** (duplicate-resubmission "Created ✓" guard) has no corresponding implementation task anywhere in plan.md's three phases — this is the one UX acceptance criterion this plan cannot currently satisfy, and it should be flagged back to planning rather than silently dropped.
