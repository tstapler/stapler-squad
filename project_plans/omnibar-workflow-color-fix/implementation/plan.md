# Implementation Plan: omnibar-workflow-color-fix

**Feature**: Fix workflow-suggestion `@slug` text color in the omnibar's `@`-mention dropdown so it renders as solid accent text instead of a faded background-overlay color
**Date**: 2026-09-11
**Status**: Ready for implementation
**ADRs**: None

---

## Domain Glossary
N/A — complexity 1, no new domain types.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `web-app/src/components/ui/AtCommandDropdown.css.ts` `slug` style | Direct token swap: `color: vars.color.accentHover` → `color: vars.color.accentText` | Existing precedent in `BacklogItemDetail.css.ts`, `JulesStatusBadge.css.ts`, `PlanVerdictBox.css.ts`, `InlineNotice.css.ts` (all use `accentText` for foreground text) | 1. Introduce a new dedicated token (e.g. `accentSlugText`) | Rejected — no other consumer needs a distinct value from `accentText`; a new token duplicates an existing solid, already-contrast-checked-in-2/7-themes color for no behavioral gain, adding a token to maintain across all 7 theme variants for zero benefit. |
| | | | 2. Hardcode a literal color (e.g. `#2d9cdb`) in the `.css.ts` file | Rejected — bypasses the theme contract entirely; the value wouldn't adapt across the other 6 themes (matrix, cyberpunk77, wh40k, clean, light), reintroducing exactly the kind of per-theme drift the `vars` contract exists to prevent. |
| | | | 3. Direct token swap (chosen) | Matches `research/build-vs-buy.md`'s verdict and existing sibling usage; zero new surface area, one-property change, self-documenting via existing precedent. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| None identified in scope of this fix (no architecture.md was produced for this complexity-1 item; `research/pitfalls.md` found no collateral debt in sibling dropdown files — `OmnibarResultList.css.ts`, `OmnibarSessionResult.css.ts`, `OmnibarPresetList.css.ts` — that misuse `accentHover`/`accentBg` as text color). | N/A | N/A |

**Follow-up note (non-blocking):** `research/pitfalls.md` #2 — 4 of `theme.css.ts`'s 6 themes' `accentText` (matrix, cyberpunk77, wh40k, clean; light/dark have documented ratios) has no recorded contrast check against `cardBackground` specifically (only against `accentBg` in the themes that do document it). This fix inherits that pre-existing gap rather than introducing it; worth a manual visual pass or an Axe check across themes as separate follow-up work, not a blocker here. Also worth a follow-up: no lint rule or test currently guards against a background/overlay token (`accentHover`, `accentBg`) being reused as a foreground `color` in this or sibling dropdown files, so this exact misuse class could recur undetected.

---

## Migration Plan
N/A — complexity 1.

## Observability Plan
N/A — complexity 1.

## Risk Control
N/A — complexity 1.

## Unresolved Questions
- [ ] None.

## Dependency Visualization
N/A — single task, no dependency graph needed.

---

## Phase 1: Fix workflow slug text color

### Epic 1.1: Correct `AtCommandDropdown` slug text token
**Goal**: The `@slug` text in workflow-suggestion rows of the omnibar's `@`-mention dropdown renders as solid, WCAG-AA-contrast accent text — the same `accentText` token used as solid foreground text elsewhere in the app — with no other visual change to the row.

#### Story 1.1.1: Swap `slug` style's text color token from `accentHover` to `accentText`
**As a** user typing `@` in the omnibar to reference a workflow, **I want** the workflow's slug text to render in a clear, solid accent color, **so that** I can read it without it looking faded or washed out.

**Acceptance Criteria**:
- AC0: `AtCommandDropdown.css.ts`'s `slug` style uses `vars.color.accentText` instead of `vars.color.accentHover`.
  - *Given* the dark theme is active (`accentText: "#2d9cdb"`, `accentHover: "rgba(45, 156, 219, 0.2)"` at `web-app/src/styles/theme.css.ts:219,222`), *When* `AtCommandDropdown.css.ts:42`'s `slug` style is inspected, *Then* its `color` property resolves to `vars.color.accentText` (`#2d9cdb` solid) and no longer to `vars.color.accentHover` (`rgba(45, 156, 219, 0.2)`).
- AC1: The workflow suggestion row's `@slug` text renders as a solid, non-translucent accent color — replacing the low-alpha `accentHover` overlay — and measures WCAG-AA contrast (≥4.5:1) against the dropdown's `cardBackground` in the themes with documented hex values. This is a same-token-family reuse of `accentText` as it renders elsewhere as solid foreground text (e.g. `JulesStatusBadge.css.ts:21`), not a claim that the workflow row and repo-suggestion rows appear together or share a token family: `AtCommandDropdown` (workflow rows) and `OmnibarResultList`/`OmnibarRepoResult` (repo rows, `@ssq`/`@pw`) are mutually exclusive render branches in `Omnibar.tsx` (`isAtDropdownVisible` gates the former, `isDiscoveryMode && !isAtDropdownVisible` gates the latter — they can never both be visible), and `OmnibarRepoResult`'s text uses `vars.color.textPrimary` + bold weight, a different token entirely.
  - *Given* the workflow suggestion row for slug `knowledge-maintenance` (`AtCommandDropdown.tsx:67`, `<span className={styles.slug}>@{wf.slug}</span>`) rendered against the dropdown's `cardBackground`, *When* the row is rendered in a browser, *Then* the text `@knowledge-maintenance` displays as solid `accentText` (not the faded/translucent `accentHover` overlay). Computed contrast ratios of `accentText` against `cardBackground` (WCAG relative-luminance formula, verified via a `python3` script run against the hex values in `theme.css.ts`): light theme `#003d99` on `#f9f9f9` = **9.38:1**; dark theme `#2d9cdb` on `#1a1a1a` = **5.71:1** — both exceed the 4.5:1 AA threshold for normal text. `theme.css.ts` defines 6 themes total (light, dark, matrix, cyberpunk77, wh40k, clean); the remaining 4 (matrix, cyberpunk77, wh40k, clean) have no recorded contrast check against `cardBackground` specifically — this is the same pre-existing gap the plan's Follow-up note already tracks as non-blocking, not a new claim introduced by this fix.
- AC2: No other visual property of the row changes.
  - *Given* the `slug` style block at `AtCommandDropdown.css.ts:39-45` before the fix, *When* the fix is applied, *Then* `fontFamily`, `fontSize`, `flexShrink`, and `minWidth` remain byte-identical to their pre-fix values — only the `color` property's value changes. Verified by inspecting `git diff` for the file and confirming it shows exactly one changed line.
- AC3: `AtCommandDropdown.css.ts` compiles; `make lint` / `cd web-app && npx jest --no-coverage` pass with no new failures.
  - *Given* the one-line edit is applied, *When* `cd web-app && npx tsc --noEmit`, `make lint`, and `cd web-app && npx jest --no-coverage` are run, *Then* all three complete with no new errors/failures introduced by this change (no existing test file covers `AtCommandDropdown`, per `research/stack.md`, so no test file needs updating).
- AC4: The fix is visually confirmed in a running instance of the app, not just via static checks.
  - *Given* a locally built/run instance of `stapler-squad` (see Task 1.1.1c), *When* the omnibar is opened and `@` is typed to trigger a workflow suggestion, *Then* the `@slug` text is observed rendering as solid accent color rather than the faded overlay it showed pre-fix.

**Files**: `web-app/src/components/ui/AtCommandDropdown.css.ts`

##### Task 1.1.1a: Swap the `slug` style's color token (~2 min)
- In `web-app/src/components/ui/AtCommandDropdown.css.ts`, change line 42 from `color: vars.color.accentHover,` to `color: vars.color.accentText,` inside the `slug` style block (lines 39-45). No other lines in the block change.
- Files: `web-app/src/components/ui/AtCommandDropdown.css.ts`

##### Task 1.1.1b: Verify compile and test gates (~3 min)
- Run `cd web-app && npx tsc --noEmit` to confirm the `.css.ts` file compiles cleanly.
- Run `make lint` from the repo root to confirm no new lint failures.
- Run `cd web-app && npx jest --no-coverage` to confirm no new test failures (no existing `AtCommandDropdown` test file to update).
- Run `git diff -- web-app/src/components/ui/AtCommandDropdown.css.ts` and confirm it shows exactly one changed line (the `color` value) — satisfies AC2.
- Files: none (verification only)

##### Task 1.1.1c: Visually verify the rendered fix (~5 min)
None of Task 1.1.1b's checks render anything — `tsc`, lint, and jest are all static and no jest test covers `AtCommandDropdown` — so this step is the only verification that the color actually changes on screen. This is a manual visual check, not new test infrastructure (unwarranted for a complexity-1 fix).
- Fastest path: `cd web-app && pnpm dev` (pure frontend CSS change, no backend behavior involved) and open the app in a browser.
  - Alternative if the frontend needs a live backend: build and run a manual instance per this repo's CLAUDE.md "Manual/interactive testing without touching the live deployed instance" — `go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .` then `PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &`.
- Open the omnibar (Meta+K) and type `@` followed by a workflow name (e.g. `@knowledge-maintenance` or any configured workflow slug) to trigger `AtCommandDropdown`.
- Confirm the `@slug` text renders as solid accent color (`accentText`), not the faded/translucent overlay (`accentHover`) it showed pre-fix. A before/after comparison (stash the change, look, restore it) is the clearest way to see the difference if the pre-fix look isn't already memorized.
- Stop the manual instance / dev server when done. Satisfies AC1 and AC4.
- Files: none (verification only)
