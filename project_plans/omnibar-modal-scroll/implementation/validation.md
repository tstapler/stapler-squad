# Validation Plan: omnibar-modal-scroll

**Date**: 2026-07-28

## Happy Path Scenario

Given the New Session Omnibar open on a 1024x700 viewport with Advanced Options
expanded, when the fix is applied, then the Create Session button is enabled and
within the viewport without any page scroll.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| FR1 (AC1) | none (manual/Playwright per plan Task 1.1.1d step 4) | n/a — devtools/Playwright inspection of computed style | Manual verification | Confirm `.modal`'s computed `max-height` is `80vh`, matching `WorkspaceSwitchModal.css.ts`'s `.modal`; no automated test — this is a static property equal to the source, so an assertion on it would just restate the code (see Note below) |
| FR2 (AC2, supports) | `tests/e2e/omnibar-modal-scroll.spec.ts` | `omnibar modal scroll > Create Session button stays reachable on a short viewport with Advanced Options expanded` | E2E (Playwright) | 1024x700 viewport, Advanced Options expanded, asserts `createButton.toBeEnabled()` and `toBeInViewport()` — passing this requires `.modal` to actually be a flex column so `.body`/`.footer` stack and `.footer` is pushed to the bottom of the capped box rather than off-page |
| FR3 (AC2, AC3) | `tests/e2e/omnibar-modal-scroll.spec.ts` (same test as above) | same as above | E2E (Playwright) | Same scenario — `toBeInViewport()` on the Create button only passes if `.body` scrolled internally (via `overflowY:"auto"`, `flex:1`, `minHeight:0`) instead of pushing `.footer` past the viewport bottom; plan Task 1.1.1d step 3 additionally covers manual confirmation of `.body`'s own scrollbar and absence of page-level scroll on `document.documentElement` |
| FR4 (AC3) | `tests/e2e/omnibar-modal-scroll.spec.ts` (same test as above) | same as above | E2E (Playwright) | `createButton.toBeEnabled()` + `toBeInViewport()` directly proves the footer/Create button stays reachable and clickable at the exact viewport/Advanced-Options combination that reproduced the original bug report |
| FR5 (AC4) | none automated; `tests/e2e/visual-regression.spec.ts` `"omnibar open"` (existing spec, not new) | `omnibar open` | Manual re-run + existing visual regression | Plan Task 1.1.1d steps 4-5: resize to ≥1200px tall viewport, confirm natural (unstretched) modal height and no visual shift vs. current behavior; separately re-run (or note for CI) the existing `visual-regression.spec.ts` `"omnibar open"` case across its 4 theme projects to catch any pixel-diff from the flex-column change on the default just-opened (unscrolled) state |

**Note on unit tests**: No Jest/RTL unit test is added. `research/pitfalls.md`
(referenced in plan.md) already established that jsdom does not perform real CSS
layout, so a test asserting `style.modal`'s object shape (e.g. `expect(modal).toContain("maxHeight")`)
would only re-assert the same three lines the fix adds to the source file — it
cannot observe overflow, scroll, or reachability, which is exactly the behavior
this bug is about. This repo also has no precedent for testing vanilla-extract
output directly (confirmed: no `*.css.test.ts` or similar file exists anywhere in
the tree). No narrow unit-style test is added for this reason — the acceptance
criteria are behavioral and are covered by the Playwright layer below instead.

## UX Acceptance Tests

Mapped from plan.md Story 1.1.1's five Given-When-Then scenarios (AC1-AC5), which
serve as this project's UX acceptance criteria in place of a `design/ux.md`.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1 — Bounded max-height, matches WorkspaceSwitchModal | none (manual) | n/a | Playwright MCP / browser devtools | Open Omnibar, inspect `.modal`'s computed `max-height` = `80vh` (plan Task 1.1.1d step 1-3) |
| AC2 — `.body` scrolls internally, footer/input bar stay reachable, no page-level scroll | `tests/e2e/omnibar-modal-scroll.spec.ts` | `omnibar modal scroll > Create Session button stays reachable on a short viewport with Advanced Options expanded` | Playwright (automated) | Set viewport 1024x700, open Omnibar (`Control+Shift+K`), select Directory, fill `/tmp`, expand Advanced Options, assert Create button `toBeEnabled()` and `toBeInViewport()`; manually cross-check `.body` scrollHeight > clientHeight and `document.documentElement` has no vertical scrollbar (plan Task 1.1.1d step 3) |
| AC3 — Create button always clickable regardless of viewport/Advanced Options | `tests/e2e/omnibar-modal-scroll.spec.ts` (same test) | same as above | Playwright (automated) | Same steps as AC2; `toBeInViewport()` plus the prior `click()`-free assertions prove the button is clickable without a page scroll first (plan Task 1.1.1e step 4) |
| AC4 — No visual regression on tall viewports, `.overlay` untouched | `tests/e2e/visual-regression.spec.ts` (`"omnibar open"`, existing) + manual | `omnibar open` | Playwright visual regression + manual devtools resize | Resize to ≥1200px tall, confirm natural (unstretched) modal height and positioning match pre-fix behavior (plan Task 1.1.1d step 4); re-run/note the existing 4-theme snapshot spec for pixel diffs (plan Task 1.1.1d step 5); `git diff` confirms zero changes to the `overlay` export |
| AC5 — CSS-only, scoped to `.modal`/`.body` in the Omnibar stylesheet | none (code review, not a runtime test) | n/a | `git diff` inspection | Confirm the only file changed is `Omnibar.css.ts` and the only exports touched are `modal` and `body` — no `.tsx` files, no other modal's `.css.ts`, no other exports in `Omnibar.css.ts` |

## Test Stack

- **E2E**: Playwright, this repo's existing `tests/e2e/` suite and conventions —
  `tests/e2e/omnibar-modal-scroll.spec.ts` (new, per plan Task 1.1.1e) plus the
  existing `tests/e2e/visual-regression.spec.ts` `"omnibar open"` case (unchanged,
  re-run for regression per plan Task 1.1.1d step 5).
- **Manual**: browser devtools viewport resize or Playwright MCP
  (`mcp__playwright__browser_resize`), per plan.md Task 1.1.1d, for AC1 and the
  tall-viewport half of AC4 that aren't cheaply expressible as a hard assertion.

## Coverage Targets and How to Measure

This is a Complexity-1 CSS-only fix with no new Go/TS domain logic, so there is no
Jest coverage-percentage target and no Go/Kotlin/Java/Rust test tier. Coverage here
means acceptance-criteria coverage, not statement/branch coverage:

- Run `cd web-app && npx playwright test omnibar-modal-scroll.spec.ts` against the
  test server (`STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server`)
  — must pass, covering AC2/AC3.
- Run `cd web-app && npx playwright test visual-regression.spec.ts -g "omnibar open"`
  across its 4 theme projects — must show no unexpected diff, covering AC4 (per
  plan Task 1.1.1d step 5; an expected `--update-snapshots` bump is acceptable per
  `research/pitfalls.md`, not a functional regression).
- `cd web-app && npx tsc --noEmit` and `cd web-app && npm run lint` — must pass
  (plan Task 1.1.1c); note `npm run lint:css` does not cover `.css.ts` files and
  verifies nothing for this change.
- All 5 acceptance criteria have a corresponding test or documented manual
  verification step (see mapping table above).
