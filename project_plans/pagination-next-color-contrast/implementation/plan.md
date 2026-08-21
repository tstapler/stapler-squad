# Implementation Plan: pagination-next-color-contrast

**Feature**: Fix WCAG AA color-contrast failure on the clean theme's `primary` token (`#6366f1` → `#5457ef`), which drives the tour/wizard "Next" button flagged by axe-core in `tests/e2e/accessibility.spec.ts` IT-5.1.
**Date**: 2026-08-21
**Status**: Ready for implementation
**ADRs**: None

---

## Domain Glossary

N/A — no new domain types, methods, or variables are introduced. This is a single hex-literal value change to an existing design token (`primary`) inside an existing vanilla-extract theme (`cleanTheme`). The token's name, contract slot, and consumers are all pre-existing and unchanged.

---

## Pattern Decisions

### Step 0.5 — Alternatives considered

1. **Edit `primary` in place (chosen)** — Change only the clean theme's `primary` hex value at theme.css.ts:636 to the pre-computed `#5457ef`, matching the existing inline-comment convention.
   - Strength: Single-line diff, no consumer file is edited (`git diff --stat` touches only `theme.css.ts`), exactly matches the acceptance criteria's pinned target value, and every consumer that reads `vars.color.primary` inherits the fix automatically because the token is defined in exactly one place (theme.css.ts:636) and read, never duplicated, everywhere else. **Correction (post-review):** the token is *not* narrowly scoped to two consumers — `grep -rn "vars\.color\.primary\b" web-app/src --include='*.css.ts' | grep -v theme.css.ts` returns **412 matches across 142 files** (re-verified live; see research/stack.md §2). `Button.css.ts` and `ModalTour.css.ts` remain the two files relevant to this story's specific visual-sanity checks (the failing ModalTour element and the Button CTA named in requirement 5), but they are not the sole consumers of the token. Requirement 2's "don't touch a consumer file" constraint is satisfied regardless of whether there are 2 or 142 consumers, since this alternative edits zero of them either way — see Story 1.1.2 for the analysis of whether the value change itself (not the file-touch count) has any unintended effect across that wider consumer set.
   - Weakness: `borderHover` and `accentText` (theme.css.ts:631, 657) stay hardcoded to the old `#6366f1` and now visibly diverge in hue from `primary` for the first time — a cosmetic side effect, not a contrast regression (see Pitfall §1). Separately, ~100+ of the 142 consumer files use the token as *foreground* `color` (or, for `ReviewQueuePanel.css.ts`, `border`) against dark clean-theme backgrounds, where a darker value can only reduce contrast further — see Story 1.1.2: most of these pairs don't cross a new WCAG threshold (pre-existing debt), but `ReviewQueuePanel.css.ts`'s `accentBg`-composited border usage does cross a new threshold (WCAG 1.4.11, 3.50:1 → 2.97:1) — a genuine regression, named explicitly and scoped out of this fix.

2. **Edit `primaryText` instead of `primary`** — Keep `#6366f1` and change `primaryText` (currently `#ffffff`) to a darker foreground color that clears 4.5:1 against `#6366f1`.
   - Strength: Would also fix the axe violation without touching `borderHover`/`accentText`'s shared hue.
   - Rejected because: `primaryText` is also the foreground on `primaryHover`/`primaryActive` button states (theme-contract.css.ts) and the button focus outline currently reuses `primary` at full saturation elsewhere in the UI; darkening the text color (rather than the background) is a bigger visual departure from the existing indigo brand color than darkening `primary` slightly, and the acceptance criteria explicitly pin the fix to `primary` at theme.css.ts:636 with a specific target hex — this alternative doesn't match the given requirement.

3. **Introduce a new contract token (e.g. `primaryAccessible`) used only by the Next button** — Add a new slot to `theme-contract.css.ts` and swap `ModalTour.css.ts`'s `primaryButton` to reference it, leaving `Button.css.ts`'s CTA on the old `primary`.
   - Strength: Would isolate the fix to only the failing element, leaving other `primary` consumers untouched.
   - Rejected because: Requirement 2 explicitly forbids touching any consuming `.css.ts` file, and this repo's `Button.css.ts` CTA has the identical contrast problem (same `#6366f1`/`#ffffff` pair) — leaving it unfixed would just relocate the WCAG failure rather than resolve it, and adds an unjustified new abstraction (extra contract slot, extra per-theme value) for what is a single-value fix. Violates the "N/A — token value change, no new abstraction" framing given in the task.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `cleanTheme.primary` value | In-place hex edit at theme.css.ts:636 | requirements.md #1, research/stack.md §7 | Edit `primaryText` instead | Bigger visual departure; doesn't match the pinned target value/line in acceptance criteria |
| `cleanTheme.primary` value | In-place hex edit at theme.css.ts:636 | requirements.md #1, research/stack.md §7 | New `primaryAccessible` contract token scoped to ModalTour only | Requirement 2 forbids touching consumer `.css.ts` files; would leave Button.css.ts's identical contrast failure unfixed; unjustified new abstraction for a single-value fix |

---

## Migration Plan

N/A — single CSS token value change, no schema/service/rollout implications.

## Observability Plan

N/A — single CSS token value change, no schema/service/rollout implications.

## Risk Control

N/A — single CSS token value change, no schema/service/rollout implications.

## Unresolved Questions

None.

## Dependency Visualization

```
1.1.1a (edit theme.css.ts:636)
        │
        ▼
1.1.1b (diff-check retro/wh40k lines byte-identical; git diff --stat) ──┐
        │                                                               │
        ▼                                                               │
1.1.2a (check foreground-contrast on named files; expect the            │
        accentBg-composite exception, not a blanket reconfirm) ─┐       │
        │                                                       │       │
        ▼                                                       │       │
1.1.2b (PR-callout: foreground-usage finding incl. the genuine  │       │
        accentBg-composite border-contrast regression, feeds 1.5.1a)    │
        │                                                       │       │
        ▼                                                       ▼       │
1.2.1a (make build)                                                     │
        │                                                               │
        ▼                                                               │
1.2.1b (run accessibility.spec.ts, both projects — cross-check          │
        failing selector against Story 1.1.2 if not ModalTour)          │
        │                                                               │
        ▼                                                               │
1.3.1a (regenerate visual-clean snapshots)                              │
        │                                                               │
        ▼                                                               │
1.3.1b (confirm tour modal in-frame + inspect diff; also eyeball        │
        SessionList active-filter-toggle + ReviewQueuePanel tag/banner) │
        │                                                               │
        ▼                                                               │
1.3.1c (git add + verify snapshot files committed)                      │
        │                                                               │
        ▼                                                               │
1.4.1a (make lint) ◄─────────────────────────────────────────────────────┘
        │
        ▼
1.5.1a (PR-description callout: borderHover/accentText, globals.css legacy
        --primary, foreground-usage finding from 1.1.2 (incl. the genuine
        accentBg-composite regression), and make lint's Go-only/procedural
        scope)
```

---

## Phase 1: Fix the clean-theme contrast token

### Epic 1.1: Change the token value and verify no cross-theme regression
**Goal**: Change `cleanTheme.primary` to the pre-computed AA-passing hex value, add the standard inline ratio comment, confirm no other theme's primary/primaryText lines moved, and document/verify the token's wider (412-reference, 142-file) foreground-usage blast radius — finding that most usages don't cross a new WCAG threshold, but one (`ReviewQueuePanel.css.ts`'s `accentBg`-composited border usage) does, and documenting that genuine regression explicitly (Story 1.1.2).

#### Story 1.1.1: Update `cleanTheme.primary` to `#5457ef` with ratio comment
**As a** user relying on the default (clean) theme, **I want** the tour/wizard "Next" button's background/foreground pair to meet WCAG AA contrast, **so that** the button text is legible and axe-core's `color-contrast` check passes.

**Acceptance Criteria**:
- Requirement 1 — clean theme `primary` changes from `#6366f1` to `#5457ef` with an inline ratio comment matching the existing convention.
  - *Given* `web-app/src/styles/theme.css.ts` line 636 currently reads `primary: "#6366f1",`, *When* the edit is applied, *Then* line 636 reads exactly `primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.46:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */` (matching the comment style at theme.css.ts:412, 389, 475, 477, 501).
- Requirement 2 — token-only edit, no consumer files touched.
  - *Given* `vars.color.primary` has 412 references across 142 `.css.ts` files (confirmed via `grep -rn "vars\.color\.primary\b" web-app/src --include='*.css.ts' | grep -v theme.css.ts`; see research/stack.md §2 for the correction of an earlier, false "exactly two consumers" claim), the requirement is satisfied by touching *zero* of them, not by there being only a small number to avoid, *When* the diff for this story is reviewed, *Then* `git diff --stat` shows exactly one file changed: `web-app/src/styles/theme.css.ts`.
- Requirement 4 — no regression to retro (cyberpunk77) or wh40k primary/primaryText pairs.
  - *Given* theme.css.ts:412 currently reads `primary: "#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */` and theme.css.ts:524/528 read `primary: "#c0a020",` / `primaryText: "#0c0a08",`, *When* the story's diff is applied, *Then* `git diff web-app/src/styles/theme.css.ts` shows zero changes to lines 412, 524, and 528 (byte-identical).

**Files**: `web-app/src/styles/theme.css.ts`

##### Task 1.1.1a: Edit line 636 (~2 min)
- Open `web-app/src/styles/theme.css.ts`, locate line 636 inside `cleanTheme`'s `color` block (between `primaryText: ... ` no — between `borderHover: "#6366f1",` at 631 and `primaryHover: "#818cf8",` at 637).
- Replace `primary: "#6366f1",` with `primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.46:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */`.
- Files: `web-app/src/styles/theme.css.ts`

##### Task 1.1.1b: Diff-check retro/wh40k lines stay byte-identical, and confirm no consumer file was touched (~2 min)
- Run `git diff web-app/src/styles/theme.css.ts` and confirm the only changed line is 636 — no other line in the hunk touches lines 412, 524, or 528.
- **Correction (post-review):** the token has 412 references across 142 files (`web-app/src --include='*.css.ts'`), not the "exactly two files" narrow list an earlier draft of this task asserted — that narrow-file-list check would fail immediately if run as originally written. Use one of these two checks instead, both of which are actually true and are the load-bearing evidence for requirement 2 (a token-only edit, not a claim about consumer count):
  - **Preferred:** run `git diff --stat` and confirm it lists exactly one file, `web-app/src/styles/theme.css.ts` — this directly proves no consumer `.css.ts` file was edited, regardless of how many consumers exist.
  - **Optional cross-check:** run `grep -c 'vars\.color\.primary\b' -r web-app/src --include='*.css.ts' | grep -v theme.css.ts` before and after the edit and confirm the per-file counts are identical — this proves the edit changed the *value* only, not any reference site.
- Files: none changed (verification only)

---

#### Story 1.1.2: Document and verify foreground-usage contrast debt (one new threshold crossing found)
**As a** reviewer of this PR, **I want** to know whether darkening `primary` (`#6366f1` → `#5457ef`) regresses any of its ~100+ *foreground*-text usages against dark clean-theme backgrounds — not just its background-under-white-text role fixed by Story 1.1.1 — **so that** a real cross-consumer contrast regression isn't silently shipped alongside the axe fix.

**Background (finding, not re-derived — table independently verified by relative-luminance calculation)**:

| Background | old `#6366f1` as text | new `#5457ef` as text |
|---|---|---|
| `background` (`#0f1117`) | 4.22:1 | 3.58:1 |
| `cardBackground` (`#161b22`) | 3.87:1 | 3.28:1 |
| `panelBgSecondary` (`#1a2232`) | 3.56:1 | 3.02:1 |
| `surfaceMuted` (`#374151`) | 2.31:1 | 1.96:1 |

**Conclusion (corrected)**: For the four *flat*-background pairs above, all were already below the 4.5:1 normal-text AA threshold *before* this change — pre-existing, undetected debt (the team has hit this exact failure mode once before with a different color; see the pre-existing comment at `web-app/src/components/layout/Header.css.ts:163-164`). Checking the 3:1 UI-component/large-text floor instead: `background` 4.22→3.58, `cardBackground` 3.87→3.28, and `panelBgSecondary` 3.56→3.02 all stay ≥3:1 on both sides (no new crossing, though `panelBgSecondary` is now close to the floor); `surfaceMuted` 2.31→1.96 was already below 3:1 and stays below it. **This change does not push any of these four flat-background pairs across a WCAG threshold (4.5:1 or 3:1) they weren't already across before the change** — it makes pre-existing, already-failing debt marginally worse in ratio, but it does not create a new axe-detectable violation on these four pairs. This part is real, pre-existing, out-of-scope accessibility debt — not introduced by this fix, not fixed by it either.

**However, this "no new crossing" finding does NOT hold universally — there is one genuine, newly-introduced regression.** `web-app/src/components/sessions/ReviewQueuePanel.css.ts`'s `tag`, `newItemsBanner`, and `filterToggleActive` styles (lines 287/429/493) don't render against a flat background token — they render against `vars.color.accentBg`, which is `rgba(99,102,241,0.1)` (theme.css.ts:654), a **hardcoded rgba baked from the OLD `primary` value** that does NOT derive from `vars.color.primary` and therefore does not move when `primary` changes. Composited on top of `cardBackground` (`#161b22`), `accentBg` yields an effective background of ≈`rgb(30, 35, 55)`. Against that composite:
- Old `primary` (`#6366f1`) as the `border: 1px solid vars.color.primary` color: **3.50:1** — passes the WCAG 1.4.11 non-text (UI-component) contrast floor of 3:1.
- New `primary` (`#5457ef`) as the same border color: **2.97:1** — fails the 3:1 floor.

This is a genuine new crossing, not pre-existing debt: the border usage flips from passing to failing as a direct result of this fix, because `accentBg` is independently hardcoded and doesn't track `primary`. **This is specifically a border/non-text-contrast (WCAG 1.4.11, 3:1) regression, not a text-contrast one.** The same components' `color: vars.color.primary` *text* usage (WCAG normal-text, 4.5:1 threshold) was already failing before this change (3.50:1 < 4.5:1) and remains failing after (2.97:1 < 4.5:1) — no new crossing on the text-contrast requirement, only on the border. Note also that axe-core's `color-contrast` rule (the check `accessibility.spec.ts` runs for Requirement 3/IT-5.1) evaluates text contrast, not border/non-text contrast — so this regression is not expected to be caught by this PR's own automated gate, which is exactly why it must be caught and named here in planning rather than shipped silently.

**Scoping call (out of this fix's authorized scope):** Requirements 1 and 2 pin this fix to editing only `theme.css.ts:636`'s `primary` value; `accentBg` at `theme.css.ts:654` is a separate token this story is not authorized to touch, and the regression is not caught by the AC's own verification gate (axe/IT-5.1). The plan therefore proceeds with the AC-pinned `primary`-only fix as specified, documents this newly-found border-contrast regression prominently in the PR body (Task 1.5.1a), and recommends — without performing — a follow-up backlog item to either derive `accentBg` from `primary` or choose a different fix for `ReviewQueuePanel.css.ts`'s border contrast. This story's Files: list and task set are not expanded to cover an `accentBg` edit.

Known affected consumers reachable from pages IT-5.1 scans: `web-app/src/components/sessions/SessionList.css.ts:107` (`filterToggleActive`, main page — a flat-background usage, part of the "no new crossing" group above) and `web-app/src/components/sessions/ReviewQueuePanel.css.ts:287/429/493` (`tag`, `newItemsBanner`, `filterToggleActive`, on `/review-queue` — the `accentBg`-composited group with the new border-contrast regression above). Both files are state-conditional (active-filter / conditional-banner states) and may not render in axe's default-page-state scan, either before or after this change — so even the genuine `ReviewQueuePanel.css.ts` regression is not expected to newly fail `accessibility.spec.ts` as a direct, automated result of Story 1.1.1's edit; it is caught only because it's named explicitly here.

**Acceptance Criteria**:
- The foreground-usage finding above (including the `ReviewQueuePanel.css.ts` accentBg-composite border-contrast regression) is present in the plan (this story) and echoed in the PR description (Task 1.5.1a's callout bullet, drafted in Task 1.1.2b).
- `SessionList.css.ts:107` and `ReviewQueuePanel.css.ts:287/429/493` are spot-checked against the new `#5457ef` value using the same relative-luminance method as the table above, *When* checked, *Then* the result matches the corrected finding above: `SessionList.css.ts:107` renders against a flat clean-theme background token and reconfirms the four-row table (no new crossing), while `ReviewQueuePanel.css.ts:287/429/493` renders against the `accentBg`-composited background and is documented as a genuine new WCAG 1.4.11 border-contrast crossing (3.50:1 → 2.97:1), not a reconfirmation of "no crossing."

**Files**: none (verification/documentation-only story; no source files change — `vars.color.primary`'s foreground usages are read-only consumers and are explicitly not edited, per requirement 2)

##### Task 1.1.2a: Check the two named consumer files; expect and document the accentBg-composite exception (~4 min)
- Read `web-app/src/components/sessions/SessionList.css.ts:107` and `web-app/src/components/sessions/ReviewQueuePanel.css.ts:287,429,493` and confirm which clean-theme background token each renders against (e.g. `background`, `cardBackground`, `panelBgSecondary`, `surfaceMuted`, `accentBg`, or another).
- **Do not expect this task to reconfirm "no new crossing" for both files.** `SessionList.css.ts:107` renders against a flat background token and is expected to reconfirm the four-row table. `ReviewQueuePanel.css.ts:287/429/493` renders against `vars.color.accentBg` (theme.css.ts:654, a hardcoded rgba of the *old* primary that doesn't track the token) composited on `cardBackground` — expect and document this as the accentBg-composite exception described in Story 1.1.2: a genuine new WCAG 1.4.11 border-contrast crossing (3.50:1 → 2.97:1), not a reconfirmation.
- Record the per-file result inline in this task's notes for the PR body (feeds Task 1.5.1a's callout bullet).
- Files: none (read-only verification)

##### Task 1.1.2b: Add the corrected foreground-usage finding as a PR-description callout (~2 min)
- Add a bullet to Task 1.5.1a's PR description draft (see Story 1.5.1) stating the corrected finding, not the disproven blanket claim: "`primary` is also used as foreground `color`/border in ~100+ places against dark clean-theme backgrounds. For flat-background usages (e.g. `SessionList.css.ts:107`), contrast was already below WCAG AA before this change and remains marginally further below it after — pre-existing debt, not introduced or fixed by this PR. However, `ReviewQueuePanel.css.ts:287/429/493` (`tag`/`newItemsBanner`/`filterToggleActive`) render against `accentBg` (theme.css.ts:654), a hardcoded rgba that doesn't track `primary`; their `border: 1px solid vars.color.primary` usage newly crosses the WCAG 1.4.11 3:1 non-text-contrast floor (3.50:1 → 2.97:1 — a genuine regression, not pre-existing debt). This is out of this fix's authorized scope (requirements 1/2 pin the edit to `theme.css.ts:636` only) and is not caught by axe-core's `color-contrast` check (which targets text, not border, contrast) — flagged here for reviewer visibility and recommended as a follow-up backlog item."
- Files: none (PR body, not a repo file)

---

## Phase 2: Build and verify against the accessibility test suite

### Epic 1.2: Rebuild the binary and run IT-5.1
**Goal**: Ensure the Playwright accessibility spec runs against a binary that actually contains the frontend fix (not a stale cached one), and confirm it passes.

#### Story 1.2.1: Rebuild and pass `accessibility.spec.ts`
**As a** reviewer verifying this fix, **I want** the e2e accessibility spec to run against a freshly built binary, **so that** a pass is real evidence the CSS change reached the served app, not a false pass from `ensureBinary()`'s 1-hour cache or its non-frontend-aware fallback rebuild.

**Acceptance Criteria**:
- Requirement 3 (build freshness half) — the tested binary is not stale.
  - *Given* `tests/e2e/helpers/test-server.ts`'s `ensureBinary()` reuses `./stapler-squad` if its mtime is under 1 hour old, and its own fallback path runs bare `go build` (which does NOT regenerate `web-app/out` / `server/web/dist`, per pitfalls.md §5), *When* `make build` is run at the repo root immediately before the Playwright invocation, *Then* `server/web/dist` and the repo-root `./stapler-squad` binary are regenerated from the current `web-app/src` (including the theme.css.ts edit), verified by `ls -la ./stapler-squad` showing an mtime newer than the `git commit`/edit timestamp of `theme.css.ts`.
- Requirement 3 (test-pass half) — `accessibility.spec.ts` passes fully.
  - *Given* the freshly built binary is running via the e2e harness's own `global-setup.ts`, *When* `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line` is run, *Then* the output reports `0 failed` across both the `[chromium]` and `[chromium-dom]` projects, with IT-5.1 ("Main page has no critical or serious accessibility violations") passing in both.

**Files**: none (verification-only story; no source files change)

##### Task 1.2.1a: Rebuild the binary (~3 min)
- From the repo root, run `make build`.
- Confirm it completes without error and regenerates `server/web/dist` (build output should reference the Next.js static export step, not just `go build`).
- Files: none (build artifacts only, not committed)

##### Task 1.2.1b: Run the accessibility spec (~3 min)
- Run `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`.
- Confirm the summary line reports `0 failed` and that IT-5.1 passes under both `[chromium]` and `[chromium-dom]` project tags in the output.
- If any failure remains, first check *which* node/selector failed:
  - If the failing node/selector is the ModalTour Next button (the element this fix targets), re-check that `make build` (task 1.2.1a) actually ran before this task — do not rely on `ensureBinary()`'s own fallback (pitfalls.md §5); this is almost certainly a stale-binary false failure.
  - If the failing node/selector is anything *other than* the ModalTour Next button, do not assume staleness — per Story 1.1.2, `vars.color.primary` has 412 consumers across 142 files, so a different failing element is a plausible cross-consumer contrast regression (e.g. one of the foreground-`color` usages against a dark background). Investigate which element/selector failed and cross-reference it against Story 1.1.2's table before re-running `make build`.
- Files: none

---

## Phase 3: Visual snapshot regeneration

### Epic 1.3: Regenerate and commit `visual-clean` snapshots
**Goal**: Confirm the new shade reads as a plausible darker indigo (not a hue shift) on the two consumers, and update the visual-regression baseline.

#### Story 1.3.1: Regenerate `visual-clean` snapshots with the tour modal confirmed in-frame
**As a** reviewer of this PR, **I want** the visual-regression baseline to reflect the new `primary` shade, and want confirmation that the screenshot actually exercises the fixed button, **so that** a near-zero pixel diff is real evidence, not evidence from a screenshot that never showed the button (per pitfalls.md §3).

**Acceptance Criteria**:
- Requirement 5 — new shade is a plausible darker indigo, not a hue shift, on both `Button.css.ts` CTA and `ModalTour.css.ts` Next button; snapshots regenerated and committed.
  - *Given* `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/` currently contains exactly `session-list-empty.png` and `omnibar-open.png` (2 files, no existing snapshot of the tour modal), *When* `cd tests/e2e && npx playwright test visual-regression.spec.ts --project=visual-clean --update-snapshots` is run against the `make build`-fresh binary from Story 1.2.1, *Then* both PNGs are rewritten in place (same two filenames — no new files added, since neither existing scenario name references a tour/onboarding flow) and `git status` shows both as modified.
  - *Given* neither `session-list-empty.png` nor `omnibar-open.png`'s scenario name suggests it opens `BacklogTourModal`, *When* the regenerated PNGs are visually inspected (or the modal's on-page trigger condition in `BacklogTourModal.tsx` is checked against what state `visual-regression.spec.ts`'s `visual-clean` project puts the page in), *Then* the plan explicitly records whether the tour modal (and therefore the fixed button) is actually visible in either captured screenshot — if not, a manual check via the CLAUDE.md "Manual/interactive testing" workflow (a second local instance on a non-8543 port) is performed instead, and its result is noted in the PR body.
- Requirement 5 (color-shift sanity) — visual check that `#5457ef` is a plausible darker indigo, not a hue shift.
  - *Given* `#6366f1` (old) and `#5457ef` (new) are both indigo-family hues (blue-violet, HSL hue ~243° for both), *When* the two hex values are compared side by side (e.g. via a swatch or the regenerated screenshot), *Then* the new color reads as a slightly darker/more saturated version of the same indigo, not a shift toward a different hue family (e.g. not toward teal or magenta).

**Files**: `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/session-list-empty.png`, `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/omnibar-open.png`

##### Task 1.3.1a: Regenerate visual-clean snapshots (~3 min)
- Ensure the binary from task 1.2.1a is still fresh (rerun `make build` if more than a few minutes have passed and further edits occurred — none are expected here).
- Run `cd tests/e2e && npx playwright test visual-regression.spec.ts --project=visual-clean --update-snapshots`.
- Confirm exactly the two existing files (`session-list-empty.png`, `omnibar-open.png`) were rewritten — no new snapshot files appeared.
- Files: `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/session-list-empty.png`, `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/omnibar-open.png`

##### Task 1.3.1b: Confirm the tour modal is in-frame; fall back to manual check if not (~4 min)
- Read the two scenario definitions in `tests/e2e/visual-regression.spec.ts` (or wherever `session-list-empty` / `omnibar-open` are defined) to check whether either navigates to a state where `BacklogTourModal` auto-renders (per its "not-yet-onboarded user" trigger condition in `BacklogTourModal.tsx`).
- If neither scenario shows the modal: start a manual second instance per CLAUDE.md's "Manual/interactive testing without touching the live deployed instance" section (`go build -o /tmp/ssq-manual-test .` then `PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`), trigger the tour modal's Next button and the Button.css.ts CTA in a browser, visually confirm the new indigo shade, then `kill %1`.
- **Note (post-review):** the two existing baseline snapshots (`session-list-empty.png`, `omnibar-open.png`) only cover `Button.css.ts` and `ModalTour.css.ts` — a weak proxy for "no unintended UI shift" given the token's true 142-file blast radius (Story 1.1.2). This is not worth a new automated test for this fix, but while already in the manual browser session above, also do a quick visual look at `SessionList.css.ts`'s active-filter-toggle state and `ReviewQueuePanel.css.ts`'s tag/banner states (the same components flagged in Story 1.1.2) to confirm nothing looks obviously broken — a recommendation, not a blocking check.
- Record the finding (which path was used, and the visual sanity result) as a one-line note for the PR body task (1.5.1a).
- Files: none

##### Task 1.3.1c: Stage and verify the snapshot commit (~2 min)
- Run `git add tests/e2e/tests/snapshots/visual-clean/` (scoped add — not `git add -A`).
- Run `git status` and confirm only the two intended PNGs are staged, nothing else.
- Files: `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/session-list-empty.png`, `tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/omnibar-open.png`

---

## Phase 4: Lint and PR documentation

### Epic 1.4: Lint gate
**Goal**: Confirm `make lint` passes with the change applied (procedural, per research/pitfalls.md §4 — root `make lint` is Go-only and won't inspect `theme.css.ts`, but requirement 6 still calls for it as a gate).

#### Story 1.4.1: Run `make lint`
**As a** contributor opening this PR, **I want** `make lint` to pass, **so that** the repo's standard pre-PR gate is satisfied per requirement 6.

**Acceptance Criteria**:
- Requirement 6 — `make lint` passes with the change applied.
  - *Given* no `.go` files are modified by this change (only `web-app/src/styles/theme.css.ts` and two PNG snapshots), *When* `make lint` is run from the repo root, *Then* it exits 0, since `golangci-lint` and this repo's custom Go linters (Makefile:601, `lint-custom` at Makefile:612) have no Go-file diff to flag.

**Files**: none

##### Task 1.4.1a: Run `make lint` (~2 min)
- From the repo root, run `make lint`.
- Confirm exit code 0.
- Files: none

### Epic 1.5: PR documentation
**Goal**: Call out the known, intentionally-out-of-scope color-mismatch side effects (borderHover/accentText hue divergence, globals.css legacy `--primary`, and the Story 1.1.2 foreground-usage finding — including the genuine `ReviewQueuePanel.css.ts` accentBg-composite border-contrast regression, not just the pre-existing-debt cases) and clarify `make lint`'s procedural scope, so reviewers aren't surprised by any of them.

#### Story 1.5.1: Note out-of-scope color-mismatch findings in the PR description
**As a** reviewer of this PR, **I want** the known cosmetic side effects and pre-existing debt called out explicitly, **so that** I don't mistake them for unintended regressions or missed scope.

**Acceptance Criteria**:
- PR body documents the pitfalls.md §1 and §2 findings, the Story 1.1.2 foreground-usage finding (pre-existing debt plus the genuine accentBg-composite regression), and `make lint`'s procedural (non-CSS) scope.
  - *Given* `cleanTheme.borderHover` (theme.css.ts:631) and `cleanTheme.accentText` (theme.css.ts:657) remain hardcoded to `#6366f1` independently of `vars.color.primary` (per research/stack.md §1/pitfalls.md §1), *When* the PR description is written, *Then* it includes a line noting that `primary` now diverges in hue from `borderHover`/`accentText` for the first time in the clean theme, and that this is a pre-existing independent-hardcoding pattern (not a new bug), out of scope for this fix.
  - *Given* `web-app/src/app/globals.css:46` defines a legacy `--primary: #6366f1;` CSS custom property still consumed by ~9 components via the legacy `.btn-primary` class (pitfalls.md §2), *When* the PR description is written, *Then* it includes a line noting these ~9 components will keep rendering the old, lower-contrast `#6366f1` after this fix, creating a visible two-tone indigo inconsistency across the app, and that `globals.css` is a separate, independently-defined value (not a consumer of `vars.color.primary`), so out of scope per requirement 2.
  - *Given* Story 1.1.2's corrected finding — that most foreground/border usages of `vars.color.primary` against dark clean-theme backgrounds are pre-existing debt with no new crossing, but `ReviewQueuePanel.css.ts:287/429/493`'s `accentBg`-composited border usage is a genuine NEW WCAG 1.4.11 (3:1) crossing (3.50:1 → 2.97:1) caused by `accentBg` (theme.css.ts:654) being an independently-hardcoded value that doesn't track `primary` — *When* the PR description is written, *Then* it includes the bullet drafted in Task 1.1.2b, verbatim or near-verbatim, and does not state or imply the disproven blanket claim that no new threshold was crossed anywhere.
  - *Given* research/stack.md §5's finding that root `make lint` is Go-only and never inspects `.css.ts` files, *When* the PR description is written, *Then* it includes a line noting that a passing `make lint` (Story 1.4.1) is procedural evidence only — it does not constitute CSS-safety verification for this change — so reviewers should rely on the accessibility spec (Story 1.2.1) and visual snapshots (Story 1.3.1) for that, not the lint gate.

**Files**: PR description only (no repo files)

##### Task 1.5.1a: Draft PR-description callout (~3 min)
- Write four short bullet points (see acceptance criteria above — borderHover/accentText, globals.css legacy `--primary`, the Story 1.1.2 foreground-usage finding (drafted in Task 1.1.2b — including the genuine `ReviewQueuePanel.css.ts` accentBg-composite border-contrast regression, not just the pre-existing-debt cases), and `make lint`'s procedural scope) into the PR description alongside the standard summary/test-plan sections.
- Cross-reference `web-app/src/styles/theme.css.ts:631,657`, `web-app/src/app/globals.css:46`, `web-app/src/components/sessions/SessionList.css.ts:107`, and `web-app/src/components/sessions/ReviewQueuePanel.css.ts:287,429,493` by path:line.
- Files: none (PR body, not a repo file)
