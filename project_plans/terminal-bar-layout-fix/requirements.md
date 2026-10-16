# Requirements: Terminal Bar Layout Fix

**Item ID:** 229df75a-06cc-4aa9-be47-3dbc9d5ff147
**Project name:** terminal-bar-layout-fix
**Date:** 2026-10-07

## Problem Statement

On mobile (the primary target is phone-sized screens ≤480 px, observed at ~375 px), the
terminal toolbar ("bar") has two layout defects:

1. **Oversized Redraw button** — the `↔️ Redraw` button is always rendered with full text
   label and `minWidth/minHeight: 44 px` touch target, consuming most of the right half of
   the toolbar row.
2. **Gallery and Files pushed off-screen** — because the expanded `toolbarActions` div is
   inserted inline inside the same `actions` row (not a separate row), Gallery and Files
   appear to the right of the Redraw button and ScrollModeChip. On a narrow viewport the
   `overflowX: auto` container and its fade-mask (`maskImage` gradient) hide these buttons
   from casual view; users don't discover them.

Secondary observation: dead space appears to the left of the Redraw button (two small
44×44 px squares for the toolbar-toggle `✕` and keyboard-toggle `⌨️` with no visible text
content at that size).

## Source Location

`web-app/src/components/sessions/TerminalOutput.tsx` (toolbar JSX starting ~line 2303)
`web-app/src/components/sessions/TerminalOutput.css.ts` (styles)

## Goals

- All intended controls — including Gallery and Files — are visible without horizontal
  scroll on a 375 px mobile screen.
- The toolbar uses available space efficiently with no excessive dead space.
- The Redraw button remains quickly reachable (it's a critical recovery action) but does
  not dominate the bar.
- Touch targets stay at ≥44 px (WCAG 2.5.5 / Apple HIG).
- No regression on desktop (≥769 px) layout.
- No regression on existing toolbar tests
  (`TerminalOutput.toolbarKeys.test.tsx`, `TerminalOutput.toolbar-analytics.test.tsx`).

## Out of Scope

- Redesigning the mobile keyboard row below the terminal.
- Changing the desktop toolbar layout.
- Changes to `ScrollModeChip` appearance (addressed in a separate backlog item if needed).
- New toolbar features.

## Constraints

- Must not remove the always-visible Redraw button (recovery path for blank screens).
- Gallery and Files upload functionality must remain fully accessible on mobile.
- The `data-testid` attributes used by tests must be preserved.
- This is a `web-app/` change only — no Go backend changes.
- Package manager: always `pnpm`, never `npm` or `yarn` (see `docs/how-to/use-pnpm-in-web-app.md`).

## Acceptance Criteria

1. On a ≤480 px viewport (portrait phone), the Redraw button is icon-only (↔️ or ↩️,
   no text label) so it does not dominate the toolbar row.
2. Gallery and Files buttons are reachable without horizontal scrolling on a 375 px
   viewport when the toolbar is expanded.
3. The toolbar row has no visible dead space wider than ~8 px between any two adjacent
   always-visible controls on a ≤480 px viewport.
4. All existing `TerminalOutput` Jest tests pass without modification to test assertions.
5. On a ≥769 px viewport, the toolbar renders identically to today (no desktop regression).
6. Touch targets for all toolbar buttons remain ≥44×44 px.
7. Axe/Lighthouse CI passes (no new WCAG violations introduced).
