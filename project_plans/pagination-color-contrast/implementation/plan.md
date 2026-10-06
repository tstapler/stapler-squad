# Implementation Plan: pagination-color-contrast

**Feature**: Fix WCAG AA color-contrast failure on `clean` theme's `primary`/`primaryText`
token pair (axe-core `color-contrast`, `accessibility.spec.ts` IT-5.1)
**Date**: 2026-08-16
**Status**: Ready for implementation
**ADRs**: None (see Unresolved Questions for why a short note was chosen over a full ADR)

---

## Domain Glossary

N/A — complexity 1, no new domain types introduced.

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `clean.primary` replacement hex | `#5457ef` (5.268:1 vs `#ffffff`) | research/stack.md candidate list | `#5a5df0` (4.931:1, minimal perceptual shift) | Passes but with the smallest margin of the three real candidates; a value just over 4.5:1 risks tipping back under from a future minor tweak (research/pitfalls.md finding 5) |
| `clean.primary` replacement hex | `#5457ef` (5.268:1 vs `#ffffff`) | research/stack.md candidate list | `#4f46e5` (reuse `primaryActive`, 6.288:1) | Reusing `primaryActive`'s exact value would make `primary` and `primaryActive` visually identical, collapsing a meaningful resting/pressed state distinction (explicitly not recommended in research/stack.md) |
| Contrast verification tooling | Throwaway `node -e` one-liner using the same WCAG relative-luminance formula `check-theme-contrast.ts` implements (hexToRgb → linearize → luminance → contrast) | research/build-vs-buy.md verdict (rated an acceptable alternative) | Fix stale `clean` snapshot in `web-app/scripts/check-theme-contrast.ts:44-52` (correct all 6 stale fields to match `theme.css.ts:609-640`) | **Revised in adversarial-review iteration 1**: the originally-planned single-field fix (`primary` only) was found to leave 5 other already-stale `clean` fields (`textPrimary`, `textSecondary`, `textMuted`, `background`, `cardBackground`) untouched, producing a half-synced, more-misleading tool than before. Fully resyncing all 6 fields for a script not wired into CI is scope creep beyond this ticket's complexity-1 "token-only edit" boundary, so the plan now uses the disposable one-liner and leaves `check-theme-contrast.ts` untouched. This also doubles as the documented fallback verification path for Tasks 1.1.2b/1.1.3a if the Playwright toolchain is unavailable (see those tasks) |
| Contrast verification tooling | (as above) | research/build-vs-buy.md verdict | New npm contrast-checking library | Redundant — research explicitly rated this "Not recommended" (the node -e manual formula already covers it, no new dependency justified for a 1-line check) |

## Migration Plan

N/A — complexity 1.

## Observability Plan

N/A — complexity 1.

## Risk Control

N/A — complexity 1.

## Unresolved Questions

None requiring a full ADR. One scope decision is worth recording inline rather than as a
separate ADR file, given the complexity-1 size of this change: the `primaryHover`
hover-state contrast gap on the same "Next" button (`ModalTour.css.ts:157-159`, background
swaps to `primaryHover` `#818cf8` without swapping text off white, ~2.98:1 — worse than the
bug this ticket fixes) is real and pre-existing, but is explicitly out of scope per
acceptance criteria (which only cover the resting `primary` state) and is NOT fixed by this
plan. It is flagged as a follow-up suggestion in Task 1.1.3 and must be called out in the PR
description rather than silently fixed, because fixing it would be a behavior change across
~70+ `Button.css.ts`/`interactiveBase.css.ts` primary-variant call sites — well beyond a
single-token edit. A full ADR was judged unnecessary because this is a documented deferral
of a known, separately-scoped issue, not an irreversible or contested architectural choice;
a plan note plus PR-description callout gives it enough durability for a complexity-1 change.

## Dependency Visualization

```
1.1.1a (edit token + comment)
      │
      ├──> 1.1.1b (verify via node -e WCAG-luminance one-liner)
      │
      ├──> 1.1.2a (make build && make lint — full frontend+backend rebuild + lint gate)
      │        │
      │        ├──> 1.1.2b (run accessibility.spec.ts, confirm 0 failed)
      │        │
      │        ├──> 1.1.3a (update visual-regression snapshots)
      │        │
      │        └──> 1.1.3b (visual sanity check: Button.css.ts CTA + ModalTour Next button)
      │
      ├──> 1.1.2c (grep other themes' primary/primaryText pairs, confirm untouched)
      │
      └──> 1.1.4a (flag primaryHover hover-state gap in PR description — no code change)
```
1.1.1b and 1.1.2c depend only on the token edit (1.1.1a) landing — neither touches the
built binary. **1.1.2a (`make build` + `make lint`) is a hard prerequisite for 1.1.2b,
1.1.3a, and 1.1.3b** — each of those three verification tasks needs a fresh build that
actually embeds the `theme.css.ts` change into `server/web/dist`, not a possibly-stale
cached binary (`tests/e2e/helpers/test-server.ts`'s `ensureBinary()` reuses any binary
under 1 hour old without rebuilding the frontend at all). 1.1.2b, 1.1.3a, and 1.1.3b can
run in any order relative to each other once 1.1.2a completes. 1.1.4a (PR description)
has no code dependency and can be written any time after 1.1.1a.

---

## Phase 1: Fix `clean` theme contrast token

### Epic 1.1: Bring `clean.primary`/`primaryText` to WCAG AA compliance
**Goal**: The `clean` theme's default `primary` background clears the 4.5:1 AA contrast
threshold against white `primaryText`, matching the `retro` theme's existing fix pattern,
with no regression to other themes and no silent scope creep into the adjacent
`primaryHover` issue.

#### Story 1.1.1: Replace the `clean.primary` hex value with a compliant shade
**As a** user relying on WCAG AA contrast, **I want** the "Next" button (and every other
`vars.color.primary`-consumer) in the default `clean` theme to have compliant white-on-primary
text contrast, **so that** the app passes automated accessibility checks and is usable for
low-vision users.

**Acceptance Criteria**:
- `clean` theme's `primary` token is `#5457ef` with an inline comment recording the old/new
  ratio, matching the `retro` theme's comment convention.
  - *Given* `web-app/src/styles/theme.css.ts:636` currently reads `primary: "#6366f1",`
    with no comment, *When* the token is changed to
    `primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.47:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */`,
    *Then* `git diff web-app/src/styles/theme.css.ts` shows exactly one changed line in the
    `clean` theme block (line 636) and no other lines in the file differ.
- The corrected value is independently verifiable via a WCAG relative-luminance
  calculation, without touching `check-theme-contrast.ts` (see Pattern Decisions:
  iteration-1 adversarial review found that script has 5 other already-stale `clean`
  fields beyond `primary`, so a single-field fix would leave it half-synced rather than
  actually fixed — this plan uses a disposable calculation instead).
  - *Given* the same relative-luminance formula `check-theme-contrast.ts` implements
    (hexToRgb → linearize → luminance → contrast ratio), *When* a throwaway `node -e`
    one-liner (see Task 1.1.1b) computes the contrast of `#ffffff` against the new
    `#5457ef` token, *Then* it prints a ratio of ~5.27:1, confirming ≥4.5:1 WCAG AA
    compliance without editing `check-theme-contrast.ts`.

**Files**:
- `web-app/src/styles/theme.css.ts` (line 636)

##### Task 1.1.1a: Edit the `clean.primary` token (~2 min)
- In `web-app/src/styles/theme.css.ts`, change line 636 from:
  ```ts
  primary: "#6366f1",
  ```
  to:
  ```ts
  primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.47:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */
  ```
- Leave `primaryHover` (`#818cf8`), `primaryActive` (`#4f46e5`), `primaryDark` (`#3730a3`),
  and `primaryText` (`#ffffff`) on lines 637-640 untouched.
- Files: `web-app/src/styles/theme.css.ts`

##### Task 1.1.1b: Verify the new contrast ratio via a throwaway calculation (~2 min)
- Do NOT edit `web-app/scripts/check-theme-contrast.ts` — iteration-1 adversarial review
  found it has 5 other stale `clean`-theme fields beyond `primary` (see Pattern
  Decisions), so a single-field fix would leave it half-synced and more misleading than
  before. Use the disposable `node -e` one-liner instead (same WCAG relative-luminance
  formula the script implements):
  ```bash
  node -e '
  function lum(hex){const c=hex.match(/\w\w/g).map(x=>{x=parseInt(x,16)/255;return x<=0.03928?x/12.92:((x+0.055)/1.055)**2.4});return 0.2126*c[0]+0.7152*c[1]+0.0722*c[2]}
  const l1=lum("ffffff"), l2=lum("5457ef");
  console.log(((Math.max(l1,l2)+0.05)/(Math.min(l1,l2)+0.05)).toFixed(3));
  '
  ```
- Confirm the printed ratio is ≥4.5 (expect ~5.27).
- This same one-liner (swap in the relevant hex pair) is the documented fallback
  verification path for Tasks 1.1.2b and 1.1.3a if `npx playwright` can't run in the
  executing environment (browser install, tmux/port availability) — see those tasks.
- Files: none (verification only, no source file edited)

#### Story 1.1.2: Confirm the fix closes the reported failure with no regressions
**As a** maintainer, **I want** the e2e accessibility suite green and every other theme's
contrast pairs untouched, **so that** the fix resolves the reported bug without introducing
new ones.

**Acceptance Criteria**:
- `accessibility.spec.ts` IT-5.1 passes for both `[chromium]` and `[chromium-dom]` projects,
  run against a binary built by Task 1.1.2a's fresh `make build` — not a cached binary
  that `tests/e2e/helpers/test-server.ts`'s `ensureBinary()` would otherwise reuse if
  under 1 hour old (`test-server.ts:144-151`), and not a bare `go build`
  (`test-server.ts:155`) that embeds a stale or absent `server/web/dist`.
  - *Given* Task 1.1.2a's `make build` has just run, embedding the `theme.css.ts` change
    into `server/web/dist`, *When* `cd tests/e2e && npx playwright test
    accessibility.spec.ts --reporter=line` is run, *Then* output shows `0 failed`
    (previously `4 failed, 16 passed` per requirements.md's reproduction), with both
    "Main page has no critical or serious accessibility violations" and "Secondary
    routes are accessible" passing on `[chromium]` and `[chromium-dom]`.
  - *Fallback*: if Playwright cannot run in the executing environment (browser install,
    tmux/port availability), fall back to Task 1.1.1b's `node -e` contrast calculation as
    a lower-confidence substitute confirming the token value itself is compliant, and
    note in the PR description that the full e2e suite was not run.
- No other theme's `primary`/`primaryText` pair is modified.
  - *Given* `retro` (`theme.css.ts:412`, `#cc245f`/`#ffffff`) and `wh40k`
    (`theme.css.ts:524,528`, `#c0a020`/`#0c0a08`) are the only other themes with `primary`/
    `primaryText` pairs in this file, *When* `grep -n 'primary:\|primaryText:' web-app/src/styles/theme.css.ts`
    is run after the edit, *Then* the `retro` and `wh40k` lines are byte-identical to their
    pre-change values (only the `clean` theme's `primary` line at 636 differs).

**Files**: `web-app/src/styles/theme.css.ts` (read-only verification, no further edits)

##### Task 1.1.2a: Rebuild the full app and run the lint gate (~8 min — first `next build` can take longer)
- **Resolves adversarial-review Blocker + Concern 4.** From the repo root, run `make
  build` — this runs the full `next build` → `server/web/dist` → `go build` chain (per
  `Makefile:132,162`), so the resulting `stapler-squad` binary's embedded
  `server/web/dist` (`//go:embed all:dist` in `server/web/embed.go`) actually contains
  the `theme.css.ts` change from Task 1.1.1a. A bare `go build`, or reusing an existing
  binary under 1 hour old (what `tests/e2e/helpers/test-server.ts`'s `ensureBinary()`
  does by default, `test-server.ts:144-155`), does NOT regenerate the frontend and can
  silently validate stale or absent CSS — this is the exact failure mode the adversarial
  review flagged as a Blocker.
- Run `make lint` (this repo's documented required gate — CLAUDE.md calls
  `make quick-check`/`make lint` required and `make build` already fails if lint fails;
  run it explicitly here as a cheap, explicit close of the "no build/lint gate anywhere
  in this plan" gap, given the only source edit is a plain string literal in
  `theme.css.ts`, a file already exempted from `lint-css-tokens`).
- Confirm both commands exit 0.
- Files: none (build/lint verification only)

##### Task 1.1.2b: Run the accessibility e2e suite (~3 min)
- Prerequisite: Task 1.1.2a's `make build` must have just run — this task depends on
  that fresh build, not a possibly-stale cached binary.
- `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`
- Confirm `0 failed` and that both IT-5.1 tests pass on `[chromium]` and `[chromium-dom]`.
- Fallback: if Playwright cannot run in this environment, fall back to Task 1.1.1b's
  `node -e` contrast calculation and note the gap in the PR description.
- Files: none (verification only)

##### Task 1.1.2c: Grep-confirm no other theme regressed (~2 min)
- `grep -n "primary:\|primaryText:" web-app/src/styles/theme.css.ts`
- Confirm the `retro` block (line 412, `primary: "#cc245f",` and its existing `primaryText`)
  and `wh40k` block (lines 524/528, `primary: "#c0a020",` / `primaryText: "#0c0a08",`) are
  unchanged from their pre-edit values; only the `clean` block's `primary` line (636) differs.
- Files: none (verification only)

#### Story 1.1.3: Update visual baselines and confirm the shade looks on-brand
**As a** reviewer, **I want** committed visual-regression snapshots and a quick visual
sanity check, **so that** a developer running `npm test` locally after this change
doesn't hit a spurious visual diff, and the new shade doesn't look like a jarring hue
shift. (**Correction, adversarial-review iteration 1**: this is local dev-run hygiene,
not a CI gate — `visual-regression.spec.ts` is not wired into any GitHub Actions
workflow. `e2e-video.yml`'s `FEATURE_SPECS` allowlist excludes it, and `ux-analysis.yml`
only runs `accessibility.spec.ts`. The plan previously stated CI would fail on this;
that claim was wrong and is corrected here.)

**Acceptance Criteria**:
- `visual-clean` Playwright snapshots are regenerated and committed alongside the token
  change.
  - *Given* `tests/e2e/visual-regression.spec.ts`'s `visual-clean` project has committed PNG
    baselines with a 1% max-pixel-diff threshold, and `vars.color.primary` appears in 142
    `.css.ts` files including Header/Omnibar surfaces captured in those baselines (per
    research/pitfalls.md finding 4), *When*
    `npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean`
    is run from `tests/e2e/` after the token change, *Then* the regenerated PNGs under
    `tests/e2e/tests/snapshots/visual-clean/` are staged and committed in the same change as
    the token edit.
- The new shade reads as a plausible darkening of indigo on at least two surfaces beyond
  the pagination/tour button.
  - *Given* `Button.css.ts:35` (`intent: "primary"` variant, `background: vars.color.primary`)
    and `ModalTour.css.ts:151` (`primaryButton` recipe) both consume `vars.color.primary`,
    *When* the app is run via a manual dev instance (per this repo's CLAUDE.md convention —
    `go build -o /tmp/ssq-manual-test . && PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`,
    never `make install-service`) and the `clean` theme's primary CTA button and the
    onboarding tour's "Next" button are viewed side by side, *Then* both render a
    slightly-darker indigo (`#5457ef` vs the prior `#6366f1`) with no jarring hue shift (still
    reads as "indigo," not shifted toward blue/purple/magenta).

**Files**:
- `tests/e2e/tests/snapshots/visual-clean/*.png`
- `web-app/src/components/ui/Button.css.ts` (read-only, visual check)
- `web-app/src/components/ui/ModalTour.css.ts` (read-only, visual check)

##### Task 1.1.3a: Regenerate and commit visual-regression snapshots (~5 min)
- Prerequisite: Task 1.1.2a's `make build` must have just run — this task depends on
  that fresh build, not a possibly-stale cached binary.
- Rationale: local dev-run hygiene, not a CI gate — `visual-regression.spec.ts` is not
  invoked by any GitHub Actions workflow (`e2e-video.yml`'s `FEATURE_SPECS` allowlist
  excludes it; `ux-analysis.yml` only runs `accessibility.spec.ts`). Regenerating keeps
  `npm test` clean for the next local run; it does not prevent a CI failure.
- `cd tests/e2e && npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean`
- Review the diff of regenerated PNGs (expect widespread but subtle changes across baselines
  containing `primary`-colored surfaces) and stage them alongside the token edit.
- Fallback: if Playwright/snapshot generation is unavailable in this environment, skip
  this task and rely on Task 1.1.3b's manual visual check instead; note in the PR that
  snapshots are stale pending a follow-up run.
- Files: `tests/e2e/tests/snapshots/visual-clean/*.png`

##### Task 1.1.3b: Manual visual sanity check (~3 min)
- Prerequisite: Task 1.1.2a's `make build` must have just run (or build fresh here with
  the command below) — this task depends on a build containing the token change, not a
  possibly-stale cached binary.
- Build and run a manual instance per CLAUDE.md's "Manual/interactive testing" section:
  `go build -o /tmp/ssq-manual-test . && PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`
- With the `clean` theme active (app default), glance at: the primary CTA button styled by
  `Button.css.ts`'s `intent: "primary"` variant, and the onboarding tour's "Next" button
  (`ModalTour.css.ts`'s `primaryButton`). Confirm both read as a plausible, on-brand
  darkening of indigo rather than a hue shift.
- `kill %1` when done.
- Files: none (manual verification only)

#### Story 1.1.4: Flag the known `primaryHover` gap without fixing it
**As a** future maintainer, **I want** the pre-existing `primaryHover` contrast gap
documented, **so that** it's tracked as a known issue instead of being silently left
undiscovered or accidentally conflated with this fix.

**Acceptance Criteria**:
- The PR description explicitly names the `primaryHover` hover-state gap as a known,
  pre-existing, out-of-scope issue, with no code change accompanying it.
  - *Given* `ModalTour.css.ts:157-159` swaps the "Next" button's background to
    `vars.color.primaryHover` (`#818cf8`) on `:hover` without swapping `color` off
    `primaryText` (white), producing a ~2.98:1 ratio — worse than the bug this ticket fixes,
    but invisible to IT-5.1 because the spec never calls `.hover()` (per
    research/pitfalls.md finding 2) — *When* the PR for this change is opened, *Then* its
    description includes a "Known follow-up (not fixed here)" note naming this file:line,
    the ratio, and pointing to `StuckItemsSection.css.ts:59-74`'s `chipActive` recipe as the
    existing prior-art pattern (swap to `vars.color.textInverse` on hover instead of
    `primaryText`) for whoever picks up the follow-up.
  - *Given* the follow-up would touch `Button.css.ts`/`interactiveBase.css.ts`'s primary
    hover variant, affecting ~70+ call sites, *When* this ticket's diff is reviewed, *Then*
    it contains zero changes to any hover-state selector or `primaryHover` consumer — the
    diff is strictly the `theme.css.ts:636` token line and the regenerated visual
    snapshots (`check-theme-contrast.ts` is untouched — see Task 1.1.1b).

**Files**: PR description only — no source files touched by this story.

##### Task 1.1.4a: Write the PR description follow-up note (~3 min)
- Draft a "Known follow-up (not fixed here)" section for the PR body naming
  `ModalTour.css.ts:157-159`, the ~2.98:1 `primaryHover`/`primaryText` ratio, why it's
  invisible to IT-5.1, why it's out of scope (blast radius: ~70+ call sites via
  `Button.css.ts`/`interactiveBase.css.ts`), and the `StuckItemsSection.css.ts:59-74`
  `chipActive` pattern as prior art for the eventual fix.
- Files: none (PR description text, written at ship time — no repo file changes)
