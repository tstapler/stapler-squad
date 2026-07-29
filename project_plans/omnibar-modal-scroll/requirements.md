# Requirements: Omnibar modal scroll fix

Source: backlog item `35f0f7b1-c348-4dd7-8d28-cf31d3ea4266` (interactive ideate skipped — no user present).

Complexity: 1 (quick task — 2-property CSS change reusing a pattern already proven
elsewhere in this codebase, no new architecture, no user-facing behavior change).

## Problem

`web-app/src/components/sessions/Omnibar.css.ts` — the New Session creation modal
(`OmnibarCreationPanel`) — has no height cap and no internal scroll region. On short
viewports (or with Advanced Options expanded), the modal grows past the bottom of the
screen and the footer's Create Session button becomes unreachable. `overflow: "hidden"`
on `.modal` clips content instead of scrolling it.

Two sibling modals in the app already cap height and scroll an inner body region
this way: `ResumeSessionModal.css.ts` (`.modal` maxHeight:"80vh" + flex column,
`.body` flex:1 + overflowY:"auto") and `WorkspaceSwitchModal.css.ts` (same `.modal`
shell, per-section scroll regions instead of one `.body`). `ui/Modal.css.ts` also
caps height (`maxHeight:"85vh"`) but uses a materially different single-scroll-box
shape (no pinned-outside-scroll footer split), so it's a looser precedent, not an
identical one. `ImportRulesModal.css.ts` does not exist in this repo — dropped from
the precedent list; it was carried over from the original bug report without
verification and does not hold up (caught during the plan's adversarial review).
`Omnibar.css.ts` is the outlier lacking any height cap or internal scroll.

## Scope note / discrepancy resolution

The backlog item's AC5 references `Omnibar.module.css` (CSS Modules), but the actual
file in this repo is `Omnibar.css.ts` (vanilla-extract, per ADR-009 / `.claude/rules/css-architecture.md`).
The item's own "Root cause" section (which cites exact line numbers) correctly references
`Omnibar.css.ts`. Treating `.module.css` as a stale/incorrect paraphrase; the real,
existing file — `Omnibar.css.ts` — is in scope. This is the only file this fix touches.

## Functional requirements

1. **FR1 — Bounded modal height.** `.modal` in `Omnibar.css.ts` gets `maxHeight: "80vh"`,
   matching the `WorkspaceSwitchModal` convention, so the modal never exceeds 80% of
   viewport height regardless of content (maps to AC1).
2. **FR2 — Modal becomes a flex column.** `.modal` gets `display: "flex"` and
   `flexDirection: "column"` so `.body` and `.footer` (its siblings, per
   `OmnibarCreationPanel.tsx`) stack vertically and `.body` can be sized as the
   flexible/scrollable region (supports AC2).
3. **FR3 — Scrollable body.** `.body` gets `overflowY: "auto"`, `flex: 1`, and
   `minHeight: 0` so the field region (including expanded Advanced Options) scrolls
   internally instead of pushing the footer off-screen. `minHeight: 0` is required for a
   flex child to shrink below its content size and actually scroll (maps to AC2, AC3).
4. **FR4 — Footer/Create button always reachable.** As a consequence of FR1–FR3, the
   footer (containing the Create Session button) stays outside the scroll region and
   within the capped-height modal, so it remains visible/clickable at any viewport
   height and any Advanced Options state (maps to AC3).
5. **FR5 — No regression on tall viewports.** `.overlay` is left untouched (no change
   to `alignItems`, `paddingTop`, positioning, or animation). When content already fits
   within 80vh, `.modal`'s existing width, border-radius, box-shadow, and entrance
   animation are all unchanged — flex column layout with unconstrained children renders
   identically to block layout when there's no overflow (maps to AC4).

## Non-functional / constraints

- **NFR1 — CSS-only change to production/application code.** No edits to `Omnibar.tsx`,
  `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx`, or any other modal's CSS. Within
  `web-app/src/`, only `Omnibar.css.ts` changes, and only its `.modal` and `.body`
  exports (maps to AC5, adjusted for the real file name). This scope governs
  production/application source under `web-app/src/` — it does not prohibit adding a
  regression test under `tests/e2e/`, since a test file is not application code and a
  bug-fix task without a durable regression check is incomplete. (Clarified during
  Phase 4 cross-artifact consistency review, which correctly caught that the original
  unqualified "only file changed" wording read as literally excluding test files —
  see `implementation/adversarial-review.md` and the consistency check finding for
  context.)
- **NFR2 — Follow existing convention.** Reuse the exact pattern already proven in
  `ResumeSessionModal.css.ts` (`maxHeight: "80vh"` + flex column + `overflowY: "auto"`
  body) rather than inventing a new approach.
- **NFR3 — vanilla-extract, no hardcoded tokens.** `80vh` is a dimensional value (not a
  design token) — consistent with how `WorkspaceSwitchModal.css.ts:31` and
  `ResumeSessionModal.css.ts:34` already inline `"80vh"` directly. No new theme token
  needed.

## Acceptance criteria mapping

| AC | Requirement(s) |
|---|---|
| 1. Bounded max-height, matches WorkspaceSwitchModal | FR1 |
| 2. `.body` scrolls internally, footer/input bar stay reachable, no page-level scroll | FR2, FR3 |
| 3. Create button always clickable regardless of viewport/Advanced Options | FR3, FR4 |
| 4. No visual regression on tall viewports, `.overlay` untouched | FR5 |
| 5. CSS-only, scoped to `.modal`/`.body` in the Omnibar stylesheet | NFR1 (file name corrected to `.css.ts`) |

## Out of scope

- Any change to `.overlay`, `.footer`, `.inputContainer`, or other Omnibar style exports.
- Any change to `OmnibarCreationPanel.css.ts` (Advanced Options' own `maxHeight: 600px`
  collapse animation is untouched — it already lives inside the now-scrollable `.body`).
- Any `.tsx` changes — the footer is already a sibling of `.body`, so no markup
  restructuring is needed.
