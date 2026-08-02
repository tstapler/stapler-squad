# Validation Plan: repo-path-picker-parity

**Date**: 2026-08-01

## Happy Path Scenario
Given a user has a prior session whose path is recency-ranked at the top of their session
history, when they open the Omnibar in New Project mode, focus the Parent Directory field,
and click that history row, then the field's value is set to the selected path in exactly
two interactions (focus + click) with no manual retyping and no dropdown-driven override of
subsequent edits. *(Story 2.1.1, UX-AC-1)*

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R1: Replace plain-text inputs with `RepoPathInput` (AC1) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-001`, `T-E2E-RPP-002` | E2E | Both fields resolve as `RepoPathInput` instances via existing `getByLabel('Parent Directory *')` / `getByLabel('Existing Worktree Path *')` locators |
| R1: No regression to pre-existing labeled locators (AC1/AC7 regression guard) | `tests/e2e/session-create-new-project.spec.ts` | All 7 existing `T-E2E-NP-*` tests (verification only, Task 3.1.1g) | E2E | `page.getByLabel('Parent Directory *').fill('~/Projects')` still resolves post-swap |
| R1: No regression to Existing Worktree Path fallback locator + `canSubmit` gating (AC1/AC7 regression guard) | `tests/e2e/session-create-existing-worktree.spec.ts` | Existing suite (verification only, Task 3.1.1h) | E2E | `page.getByLabel('Existing Worktree Path').fill(...)` and empty-path `canSubmit` gating still pass |
| R2: History suggestions surfaced — Parent Directory (AC2) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-001` (Task 3.1.1b) | E2E | Seeded session path appears as a `role="listbox"` option on focus |
| R2: History suggestions surfaced — Existing Worktree Path fallback (AC2) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-002` (Task 3.1.1c) | E2E | Seeded worktree path appears as a `role="listbox"` option on focus, empty-discovery state |
| R3: `useSessionRepoPaths` sources from `selectActiveSessionsSortedByUpdatedAt` with a total, deterministic tiebreak (AC3) | `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` | `describe("selectActiveSessionsSortedByUpdatedAt — tiebreak", ...)` — 4 `it` cases: primary `updatedAt` order, `createdAt` tiebreak, `id` ascending final tiebreak, `UNSPECIFIED`-status filter regression guard (Task 1.2.1b) | Unit | `updatedAt` desc → `createdAt` desc → `id` asc comparator produces a total order; `UNSPECIFIED` sessions excluded regardless of recency |
| R3: `useSessionRepoPaths` propagates recency order + dedup (AC3) | `web-app/src/lib/hooks/useSessionRepoPaths.test.ts` (new — **addition**, see note below) | `describe("useSessionRepoPaths — recency-ordered paths", ...)` `it("returns deduplicated paths in selectActiveSessionsSortedByUpdatedAt order, excluding UNSPECIFIED sessions", ...)` | Unit | 3-session store (`s1` most recent, `s2` older, `s3` `UNSPECIFIED`) → hook returns `["/repo/a", "/repo/b"]` per Story 1.2.2's Given/When/Then |
| R4: Manual free-text entry not overridden — both fields (AC4) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-003`, `T-E2E-RPP-004` (Task 3.1.1d) | E2E | Typing a path absent from history/fs results in `expect(locator).toHaveValue(typedPath)` immediately, no dropdown overwrite |
| R5: Dropdown rendering — desktop visibility, no overflow (AC5) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-001`, `T-E2E-RPP-002` (listbox visibility assertions, Tasks 3.1.1b/c) | E2E | Dropdown renders as a visible `listbox` at standard desktop viewport with expected option rows |
| R5: Dropdown rendering — 390×844, no horizontal overflow, no vertical clip — Parent Directory (AC5) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-007` (Task 3.1.1f, contingency: Task 3.1.1f-contingency open-upward fallback if triggered) | E2E | `getBoundingClientRect()`-based check: `dropdown.right <= 390` and `dropdown.bottom <= modal.bottom` |
| R5: Dropdown rendering — 390×844 spot-check — Existing Worktree Path fallback (AC5) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-008` (**addition** — see note below) | E2E | Same `getBoundingClientRect()` check applied to the fallback field, per `ux.md` Surface 10's "should get at minimum a spot-check" |
| R6: Escape stops propagation when dropdown open, isolated (AC6) | `web-app/src/components/ui/RepoPathInput.test.tsx` | `describe("RepoPathInput — Escape key handling", ...)` `it` case 1 (Task 1.1.1b) | Unit | Dropdown open → Escape → listbox removed from document |
| R6: Escape does not bubble to parent `onKeyDown` when dropdown open (AC6) | `web-app/src/components/ui/RepoPathInput.test.tsx` | `describe("RepoPathInput — Escape key handling", ...)` `it` case 2 (Task 1.1.1c, nested-parent spy) | Unit | Dropdown open, parent `onKeyDown` spy → Escape → spy NOT called |
| R6: Escape bubbles normally when dropdown closed / never opened (AC6, no-regression half) | `web-app/src/components/ui/RepoPathInput.test.tsx` | `describe("RepoPathInput — Escape key handling", ...)` `it` case 3 (Task 1.1.1c, second sub-case) | Unit | Dropdown never opened, parent `onKeyDown` spy → Escape → spy IS called |
| R6: Escape bubbles normally when `open===true` but dropdown empty (`showDropdown===false`) — the case an `open`-only gate would get wrong (AC6) | `web-app/src/components/ui/RepoPathInput.test.tsx` | `describe("RepoPathInput — Escape key handling", ...)` `it` case 4 (Task 1.1.1c, third case) | Unit | Empty history + no fs matches, focused → Escape → `stopImmediatePropagation` not triggered, parent `onKeyDown` spy IS called |
| R6: Escape closes only dropdown in the live Omnibar; second Escape resets/closes panel as before (AC6, integration) | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-005`, `T-E2E-RPP-006` (Task 3.1.1e) | E2E | 1st Escape: listbox hidden, New Project radio still `aria-checked="true"`; 2nd Escape: pre-existing reset-to-discovery/close fires |
| R7: New e2e spec follows `.claude/rules/e2e-test-conventions.md` (AC7) | `tests/e2e/repo-path-picker-parity.spec.ts` | File-level `// @feature session:create, repo-path-picker-parity` header + shared setup (Task 3.1.1a) | E2E | Feature annotation present; no `waitForTimeout`; `data-testid`/ARIA locators only |
| R7: Feature registered under `docs/registry/features/frontend/` (AC7) | `docs/registry/features/frontend/ui/repo-path-picker-parity.json` | N/A (registry entry, not a test) — `"tested": true`, `testIds` referencing the specs above (Task 3.2.1a) | Registry | Entry documents component, path, and test coverage |
| R7: `make registry-generate` produces no net growth in `docs/registry/coverage-gaps.json` (AC7) | `docs/registry/coverage-gaps.json` (generated) | N/A — verification step (Task 3.2.1b) | Registry | Untested-feature count after `<=` count before |

**Note on additions**: `T-E2E-RPP-008` and `useSessionRepoPaths.test.ts`'s new `describe` block are
not explicitly enumerated as separate tasks in `plan.md`. They are flagged here as genuine
coverage gaps:
- `useSessionRepoPaths.test.ts` — Story 1.2.2 (`plan.md` line ~335) lists only
  `web-app/src/lib/hooks/useSessionRepoPaths.ts` under **Files**, with no companion test file,
  even though its own Acceptance Criteria section states a full Given/When/Then for the hook's
  output. Task 1.2.1b's `sessionsSlice.test.ts` tests only prove the *selector* is correct, not
  that the hook correctly consumes and dedupes its output — a distinct, narrow unit test closes
  this gap.
- `T-E2E-RPP-008` — `design/ux.md`'s Surface 10 narrative explicitly says the Existing Worktree
  Path fallback field "should get at minimum a spot-check, ideally the same automated assertion"
  for the 390×844 clipping check, but `plan.md`'s Task 3.1.1f only names the Parent Directory
  field as the committed target. Adding the mirrored assertion for the second field costs one
  additional `page.evaluate()` call inside the same spec file.

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-AC-1 — Parent Directory: reuse a known path in ≤2 actions | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-001` (Task 3.1.1b) | Playwright | Focus field → click matching history `option` → assert field value equals selected path |
| UX-AC-2 — Existing Worktree Path fallback: reuse a known path in ≤2 actions | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-002` (Task 3.1.1c) | Playwright | Focus field → click matching history `option` → assert field value equals selected path |
| UX-AC-3 — Typing a brand-new path is never intercepted | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-003`, `T-E2E-RPP-004` (Task 3.1.1d) | Playwright | Type a guaranteed-no-match path into each field → `toHaveValue(typedPath)` immediately |
| UX-AC-4 — No dropdown / no error text when zero results; manual entry still works | `tests/e2e/repo-path-picker-parity.spec.ts` | Covered implicitly by `T-E2E-RPP-003`/`004`'s value assertion; **manual/visual check** for "no dropdown, no error banner" per `ux.md` Surfaces 1/3/4/6 (not independently asserted in plan.md) | Playwright + manual | Assert `page.getByRole('listbox')` is not visible after typing a no-match string, alongside the value assertion above |
| UX-AC-5 — Selecting a history entry doesn't block further editing | *(not in plan.md — manual/visual check)* | — | Manual | After clicking a history row, type an additional character and confirm the input remains focused/editable |
| UX-AC-6 — Invalid Existing Worktree Path submit doesn't silently fail | Out of scope for this project (pre-existing `CreateSession` RPC error handling, unchanged) | — | Regression guard only | No new test — confirmed unchanged behavior per `plan.md` Pattern Decisions table |
| UX-AC-7 — One Escape press closes only the dropdown, panel/mode/values unchanged | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-005` (Task 3.1.1e) | Playwright | Dropdown open → Escape → listbox hidden, New Project radio still `aria-checked="true"` |
| UX-AC-8 — Escape with dropdown closed reproduces pre-existing panel reset/close | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-006` (Task 3.1.1e) | Playwright | Dropdown already closed → Escape → existing reset-to-discovery/close behavior fires |
| UX-AC-9 — Full exit from any state in ≤2 Escape presses | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-005` + `T-E2E-RPP-006` sequence (Task 3.1.1e) | Playwright | Same two-press sequence as UX-AC-7/8, asserted as one flow |
| UX-AC-10 — No horizontal overflow at 390×844 | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-007` (Task 3.1.1f) | Playwright | `dropdown.right <= 390` via `getBoundingClientRect()` |
| UX-AC-11 — No vertical clipping under keyboard occlusion at 390×844 | `tests/e2e/repo-path-picker-parity.spec.ts` | `T-E2E-RPP-007` + contingency (Tasks 3.1.1f / 3.1.1f-contingency) | Playwright | `dropdown.bottom <= modal.bottom`; if clipped, open-upward fallback implemented and re-verified |
| UX-AC-12 — Dropdown rows ≥24×24 CSS px tap target | *(not in plan.md — manual/visual check; `ux.md` itself notes this is "not a known gap today")* | — | Manual | Inspect row `getBoundingClientRect()` height (~30–32px, already compliant); no automated regression test committed in plan.md |
| UX-AC-13 — Full keyboard operability (Tab/type/Arrow/Enter/Escape, no mouse) | `web-app/src/components/ui/RepoPathInput.test.tsx` (pre-existing keyboard-nav tests, unchanged by this plan) + `tests/e2e/repo-path-picker-parity.spec.ts` `T-E2E-RPP-005`/`006` (Escape only) | Existing `RepoPathInput.test.tsx` describe blocks (keyboard nav) + Task 3.1.1e | Unit + Playwright | Arrow/Enter selection pre-existing and unchanged; Escape scoping newly covered by `T-E2E-RPP-005`/`006` |
| UX-AC-14 — Combobox ARIA triad present, `aria-expanded` reflects live state | `web-app/src/components/ui/RepoPathInput.test.tsx` | `describe("RepoPathInput — combobox a11y", ...)` — 2 `it` cases (Task 1.1.2b) | Unit + manual screen-reader spot-check | Closed: `role="combobox"`, `aria-haspopup="listbox"`, `aria-expanded="false"`; open: `aria-expanded="true"` |
| UX-AC-15 — Dropdown rows announced by visible text alone (icons `aria-hidden`) | *(not in plan.md — manual/visual check, flagged in `ux.md` as a verification item, no defect found)* | — | Manual | Screen reader pass confirming tilde-abbreviated path text alone is unambiguous |
| UX-AC-16 — Text contrast ≥4.5:1 (input, hint, dropdown row incl. muted history style) | *(not in plan.md — manual/visual check against existing token contract)* | — | Manual | Verify `web-app/src/app/globals.css` tokens used by `RepoPathInput.css.ts` meet contrast in both themes |
| UX-AC-17 — Labels remain correctly associated after swap | `tests/e2e/session-create-new-project.spec.ts`, `tests/e2e/session-create-existing-worktree.spec.ts` | Existing suites (Tasks 3.1.1g, 3.1.1h) | Playwright | `page.getByLabel('Parent Directory *')` / `page.getByLabel('Existing Worktree Path')` continue to resolve |
| UX-AC-18 — Every surface has a confirmed exit path (Escape/click-outside/Tab/continue typing) | `tests/e2e/repo-path-picker-parity.spec.ts` (Escape only) + pre-existing `RepoPathInput.test.tsx` click-outside coverage (unchanged) | `T-E2E-RPP-005`/`006` + existing click-outside `it` block | Playwright + Unit | Composite check — no single new test, summary property covered by the union of existing + new tests above |

---

## Test Stack
- **Unit**: Jest + React Testing Library (TypeScript, `web-app/`)
- **Integration**: N/A — no backend/data-store changes in this feature
- **E2E / UX**: Playwright (`tests/e2e/`)

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="RepoPathInput\|sessionsSlice\|useSessionRepoPaths"` | existing repo convention — check `web-app`'s jest config for an enforced threshold; otherwise no hard numeric gate, note it |
| Playwright | `cd tests/e2e && npx playwright test repo-path-picker-parity.spec.ts session-create-new-project.spec.ts session-create-existing-worktree.spec.ts` | all listed specs pass; no `T-E2E-NP-*` or existing-worktree-spec regressions |

- All public component behavior: happy path (R2, R4, UX-AC-1/2/3) + error/no-match paths (UX-AC-3/4) covered.
- All external integrations: N/A (no new RPC/external call — `usePathCompletions` untouched).
- UX acceptance criteria: each of the 18 UX-AC criteria in `design/ux.md` has a corresponding
  automated test or an explicitly named manual/visual verification step (see table above) — 12 of
  18 are automated (unit or e2e), 6 are manual/visual (UX-AC-5, 6, 12, 15, 16, and the
  screen-reader half of UX-AC-14), consistent with `plan.md`'s own scope decisions (no new
  validation UI, no automated contrast/tap-target regression harness in this repo today).
- Registry: `docs/registry/features/frontend/ui/repo-path-picker-parity.json` created (Task
  3.2.1a); `make registry-generate` run and `coverage-gaps.json` net growth confirmed `<= 0`
  (Task 3.2.1b) — no migration test applicable (no schema/backend change).
