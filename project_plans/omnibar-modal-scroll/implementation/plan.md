# Implementation Plan: omnibar-modal-scroll

**Feature**: Cap the New Session Omnibar modal's height and make its body scroll internally so the footer/Create Session button stay reachable on short viewports
**Date**: 2026-07-28
**Status**: Ready for implementation
**ADRs**: None

---

## Domain Glossary

No new domain terms — this is a CSS layout fix with no new types/concepts. The existing terms (`.modal`, `.body`, `.overlay`) are pre-existing vanilla-extract style exports in `web-app/src/components/sessions/Omnibar.css.ts`.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `Omnibar.css.ts` `.modal` / `.body` | Copy the proven "capped-height flex column + scrollable body" shell: `.modal` gets `maxHeight: "80vh"`, `display: "flex"`, `flexDirection: "column"`; `.body` gets `overflowY: "auto"`, `flex: 1`, `minHeight: 0` | `ResumeSessionModal.css.ts` (`.modal` lines 28-39, `.body` lines 50-54) and `WorkspaceSwitchModal.css.ts` (`.modal` lines 25-36) — same shell already shipped in production. Note (caught during adversarial review): neither sibling file actually sets `minHeight: 0` on its scrollable body; this plan adds it anyway as the more defensive/correct form per `research/stack.md`, so this is a close match, not a byte-for-byte copy | Migrate `Omnibar.tsx` to the shared `ui/Modal.tsx` primitive that already implements this pattern once | Out of scope structural refactor per NFR1 and `build-vs-buy.md` — `Omnibar.tsx` hand-rolls its own `role="dialog"` markup independently of `ui/Modal.tsx`, and `ui/Modal.css.ts` uses a different single-scroll-box shape anyway (confirmed during architecture review), so this wouldn't even be a drop-in swap; migrating it is a separate consolidation effort, not a CSS bug fix |

---

## Technology Validation

No new technology. vanilla-extract 1.20.1 `style()` API, already in use in this exact file. `maxHeight`, `display`, `flexDirection`, `overflowY`, `flex`, and `minHeight` are all plain CSS properties already exercised by the identical pattern in `ResumeSessionModal.css.ts` and `WorkspaceSwitchModal.css.ts` — no compatibility risk, no build config change.

---

## Migration Plan
N/A — no schema or data changes.

## Observability Plan
N/A — pure CSS layout fix, no new service boundaries, logs, metrics, or alerts needed.

## Risk Control
- Feature flag: not gated — trivial CSS fix matching an already-shipped pattern used by 3 other modals (`ResumeSessionModal`, `WorkspaceSwitchModal`, `ui/Modal`) in production.
- Rollback procedure: standard revert via PR close + revert commit.
- Staged rollout: full rollout on merge.

## Unresolved Questions
None.

## Known Accepted Limitation

(Raised in `research/pitfalls.md` and confirmed still open by adversarial review.)
Scoping the scroll fix to `.body` alone (per NFR1) means the *other* siblings of
`.modal` — `inputContainer`, `detectionInfo`, `pathDisplay`/`createRepoNotice`
(conditional), `error` (conditional), `footer`, `shortcuts` — stay outside the
scroll region at their natural height. On an extremely short viewport where one of
those conditional notices is visible *and* Advanced Options is expanded, that fixed
chrome could itself approach 80vh, leaving `.body` little room to scroll into. This
is not a regression introduced by this fix — `ResumeSessionModal` has the identical
limitation (its `header` also sits outside its scrollable `.body`) — and is accepted
as a known limitation of the "cap + scroll the field region only" pattern rather
than a blocker for this task. A future consolidation onto the shared `ui/Modal.tsx`
primitive (see Pattern Decisions "Alternative Rejected") would be the place to
revisit whether the whole content area, not just `.body`, should scroll.

## Dependency Visualization

```
Task 1.1.1a (add maxHeight/display/flexDirection to .modal)
        │
        ▼
Task 1.1.1b (add overflowY/flex/minHeight to .body)
        │
        ▼
Task 1.1.1c (build + lint)
        │
        ▼
Task 1.1.1d (manual/Playwright verification: AC1-AC4)
        │
        ▼
Task 1.1.1e (permanent Playwright regression test: AC2/AC3)
```

---

## Phase 1: CSS scroll-clamp fix

### Epic 1.1: Bound the Omnibar modal's height and scroll its body

**Goal**: `Omnibar.css.ts`'s `.modal` and `.body` exports adopt the same capped-height, flex-column, scrollable-body shell already proven in `ResumeSessionModal.css.ts` and `WorkspaceSwitchModal.css.ts`, so the New Session creation modal never grows past the viewport and the footer's Create Session button is always reachable.

#### Story 1.1.1: Modal stays within the viewport and the Create button stays reachable
**As a** user opening the New Session Omnibar on a short viewport (or with Advanced Options expanded), **I want** the modal to cap its height and scroll its body internally, **so that** the footer and Create Session button never get pushed off-screen.

**Acceptance Criteria** (mapped 1:1 to requirements.md's AC1-AC5):

- AC1 — Bounded max-height, matches WorkspaceSwitchModal
  - *Given* the Omnibar `.modal` style in `Omnibar.css.ts`, *When* the CSS is built, *Then* `.modal` has `maxHeight: "80vh"`, identical in value and property name to `WorkspaceSwitchModal.css.ts`'s `.modal` (line 25-36 region).

- AC2 — `.body` scrolls internally, footer/input bar stay reachable, no page-level scroll
  - *Given* a browser viewport 700px tall with Advanced Options expanded in the New Session Omnibar, *When* the modal is open, *Then* `.modal`'s computed height is ≤ 80vh (≤ 560px at 700px viewport height), `.body` shows its own vertical scrollbar (its content's scrollHeight exceeds its clientHeight), and `document.documentElement` shows no page-level vertical scrollbar.

- AC3 — Create button always clickable regardless of viewport/Advanced Options
  - *Given* a browser viewport 700px tall with Advanced Options expanded, *When* the Omnibar modal is open, *Then* `.modal`'s computed height is ≤ 80vh (≤ 560px) and the Create Session button's bounding rect (`getBoundingClientRect()`) is fully within the viewport (top ≥ 0 and bottom ≤ window.innerHeight), and a Playwright `click()` on it succeeds without needing to scroll the page.

- AC4 — No visual regression on tall viewports, `.overlay` untouched
  - *Given* a browser viewport 1200px tall (Advanced Options collapsed, the modal's natural content height is well under 80vh i.e. under 960px), *When* the Omnibar modal is open, *Then* the modal renders at its natural content height (no forced stretch to 80vh), positioned exactly as before the fix (`.overlay`'s `paddingTop: "10vh"` and centering unchanged), and a diff of `Omnibar.css.ts` shows zero changes to the `overlay` export.

- AC5 — CSS-only, scoped to `.modal`/`.body` in the Omnibar stylesheet
  - *Given* the git diff for this fix, *When* reviewed, *Then* within `web-app/src/` the only file changed is `web-app/src/components/sessions/Omnibar.css.ts`, and the only exports modified are `modal` and `body` — no `.tsx` files, no other modal's `.css.ts`, and no other exports in `Omnibar.css.ts` (`overlay`, `inputContainer`, `typeIndicator`, `input`, `detectionInfo`, `detectionBadge`, `unknown`, `field`, `label`, etc.) are touched. (Per requirements.md NFR1, this "only file changed" scope governs production/application source under `web-app/src/`; it does not prohibit `tests/e2e/omnibar-modal-scroll.spec.ts` from Task 1.1.1e — see NFR1's note, added after Phase 4 cross-artifact consistency review flagged the original unqualified wording as a literal contradiction with Task 1.1.1e.)

**Files**: `web-app/src/components/sessions/Omnibar.css.ts`

##### Task 1.1.1a: Add height cap and flex-column layout to `.modal` (~2 min)
- Open `web-app/src/components/sessions/Omnibar.css.ts`.
- In the `export const modal = style({...})` block (currently lines 42-58), add three properties as top-level siblings of the existing `background`, `borderRadius`, `width`, `maxWidth`, `boxShadow`, `overflow`, `position` properties (leave the nested `"@media": {"(prefers-reduced-motion: no-preference)": {...}}` block untouched):
  - `maxHeight: "80vh"`
  - `display: "flex"`
  - `flexDirection: "column"`
- Leave `overflow: "hidden"` as-is (it still correctly clips the rounded corners / box-shadow container; the inner `.body` scrollbar is what handles overflow content).
- Files: `web-app/src/components/sessions/Omnibar.css.ts`

##### Task 1.1.1b: Make `.body` a scrollable flex child (~2 min)
- In the same file, in `export const body = style({...})` (currently lines 116-121), which already has `padding: 16`, `display: "flex"`, `flexDirection: "column"`, `gap: 16`, add three properties:
  - `overflowY: "auto"`
  - `flex: 1`
  - `minHeight: 0`
- Files: `web-app/src/components/sessions/Omnibar.css.ts`

##### Task 1.1.1c: Build, lint, and verify AC5 scope (~3 min)
- Run `cd web-app && npx tsc --noEmit` (or the project's standard frontend build step) to confirm the vanilla-extract file still compiles.
- Run `cd web-app && npm run lint` (`next lint`) to type-check/lint the `.ts` change. Note: `npm run lint:css` (`stylelint 'src/**/*.css'`) does NOT cover `.css.ts` files — it globs plain `.css` only — so it verifies nothing for this change; don't rely on it (caught during adversarial review).
- **AC5 verification** (gap caught during Phase 4 cross-artifact consistency review — AC5 previously had no dedicated check): run `git diff --stat` and confirm within `web-app/src/` the only changed file is `web-app/src/components/sessions/Omnibar.css.ts`, and within that file only the `modal` and `body` `style({...})` blocks differ (`git diff web-app/src/components/sessions/Omnibar.css.ts` — no other export touched). `tests/e2e/omnibar-modal-scroll.spec.ts` (Task 1.1.1e) is expected and allowed per NFR1's test-file carve-out; any other file appearing in the diff is a scope violation.
- Files: none (verification only)

##### Task 1.1.1d: Manual/Playwright verification of AC1-AC4 (~5 min)
- No new Jest test is added — jsdom does not perform real CSS layout, so dimension assertions there would not actually verify anything (per `research/pitfalls.md`).
- Instead, verify manually via Playwright MCP or browser devtools:
  1. Start the dev server / test server, open the app, trigger the New Session Omnibar.
  2. Resize the viewport to 700px tall (e.g. `mcp__playwright__browser_resize` or devtools responsive mode), expand Advanced Options.
  3. Confirm: the modal's computed height stays ≤ 80vh, `.body` has an internal scrollbar, the footer/Create Session button's bounding box is fully within the viewport, and clicking Create Session succeeds without prior page scroll (AC1-AC3).
  4. Resize to a tall viewport (≥1200px), confirm the modal renders at natural height with no stretching or visual shift versus current behavior (AC4).
  5. Re-run (or note for CI) the existing Playwright visual-regression spec `tests/e2e/visual-regression.spec.ts` `"omnibar open"` case across its 4 theme projects — expect no diff in the default just-opened snapshot state. If CI trips on the 1% pixel-diff threshold, visually inspect the diff image before running `--update-snapshots` (per triad UX-lens review — don't blind-accept a snapshot update without eyeballing it, even though `research/pitfalls.md` predicts no visible diff in this state).
- Files: none (verification only; optionally `tests/e2e/visual-regression.spec.ts` snapshots if CI requires `--update-snapshots`, but no new spec file is created since this is out of scope per NFR1)

##### Task 1.1.1e: Add a permanent Playwright regression test for AC2/AC3 (~5 min)
- Raised by adversarial review: this is a real, previously user-reported bug (Create
  Session button unreachable) — a one-time manual check leaves it regressable.
  NFR1 governs the *production* code diff (CSS-only, `.modal`/`.body` in
  `Omnibar.css.ts`), not the test suite, so adding a test file does not violate it.
- Add `tests/e2e/omnibar-modal-scroll.spec.ts` following this repo's e2e
  conventions (`// @feature session:create` marker reusing the existing feature id
  — no new registry entry needed, this is a bug fix to an existing feature, not a
  new one; ARIA-role locators only, no CSS class selectors; no `waitForTimeout`
  polling where an `expect(...).toBeEnabled()`/`toBeInViewport()` suffices):
  1. Set viewport to 1024×700 (short height, reproduces the bug).
  2. Open the Omnibar in creation mode (`Control+Shift+K`), select the Directory
     radio, fill `/tmp` into the session-source input to enable submit.
  3. Click "Advanced Options" to expand it, then wait for the expansion to settle
     (e.g. `await expect(page.locator(...advancedSectionOpen...)).toBeVisible()` or
     an equivalent explicit wait — not a raw `waitForTimeout`) before asserting
     viewport position, since the section animates open (`maxHeight: 0 → 600px`
     transition in `OmnibarCreationPanel.css.ts`) and asserting mid-transition would
     be flaky (flagged by triad Engineering-lens review).
  4. Assert `getByRole('button', { name: 'Create Session' })` is enabled and
     `toBeInViewport()` — this only passes without an explicit `scrollIntoView`,
     so it directly proves AC2 ("footer reachable without page-level scroll") and
     AC3 ("Create button always clickable") on the exact viewport/state combination
     that was broken.
- Files: `tests/e2e/omnibar-modal-scroll.spec.ts` (new)
