# Requirements: Mobile Gutter UX

**Item:** We should have more robust usage of the gutter on mobile  
**Project:** `mobile-gutter-ux`  
**Date:** 2026-11-23

---

## Problem Statement

The stapler-squad web app exposes gutter areas in multiple views (log viewer, files/code editor, diff viewer). On mobile, these gutters are either:
- Too narrow for reliable touch targets (CodeMirror default gutter ~30px wide)
- Passive decorations only (diff +/- color strips, line numbers) with no mobile-specific interactions
- Missing mobile-specific affordances that the desktop mouse experience takes for granted (hover to reveal fold controls, right-click context, etc.)

The reference screenshot shows a Logseq-style mobile UI where the gutter contains actionable drag handles and hierarchical bullet markers — interactive elements, not dead space. Stapler-squad should make equivalent use of its gutter space on mobile.

---

## Scope

Three gutter surfaces are in scope:

1. **Log viewer** (`LogRow` / `LogRow.css.ts`) — gutter has line number + log-level badge. Currently 88px desktop / 44px narrow. The badge area is a touch target but tapping it has no mobile-specific effect (no level-filter action is wired to the badge tap).

2. **Files viewer — CodeMirror gutter** (`CmEngine.ts`, `cmTheme.ts`) — hosts:
   - Line numbers (`.cm-lineNumbers`, ~30px wide)  
   - Diff change gutter (2–4px color strip via `changeGutter` in `CmEngine.ts`)
   On mobile: gutter elements are not enlarged, no tap-to-navigate-diff-hunk interaction, fold gutter (if present) has no coarse-pointer sizing adjustment.

3. **Files viewer — mobile layout** (`MobileFilesLayout.tsx`, `MobileBars.tsx`) — top/bottom bars are present, but the editor gutter inside the viewport is not specifically configured for touch.

Out of scope: session-list swipe-to-archive (separate feature), terminal gutter (ghostty renders its own chrome).

---

## Acceptance Criteria

### AC-1 — Log viewer level-badge tap filters the log
Tapping the level badge in a `LogRow` gutter on a touch device opens a quick-filter that restricts log display to that level (or toggles it if already active). This gives the gutter an action on mobile equivalent to what a right-click filter would provide on desktop.

### AC-2 — CodeMirror line-number gutter meets 44px touch target
On coarse-pointer devices, the CM line-number gutter column is at least 44px wide so that tapping a line number selects the line without precision tapping. The diff gutter strip widens proportionally.

### AC-3 — Diff gutter markers are tappable on mobile
On coarse-pointer devices, tapping a diff gutter marker (add/delete/modify strip) navigates to the next change hunk (wrapping). This surfaces the diff-navigation action that desktop users access via keyboard or gutter click.

### AC-4 — Gutter touch targets do not exceed the viewport width budget
Wider gutters must not overflow or push code content off-screen on devices narrower than 390px. The gutter total width on narrow mobile must not exceed 56px (allowing ≥334px for code content).

### AC-5 — Fold gutter (when active) has coarse-pointer sizing
If the CM fold gutter extension is loaded, its fold/unfold click targets must be at least 44px tall on coarse-pointer devices.

### AC-6 — No regression on desktop mouse experience
All gutter changes must be coarse-pointer-conditional. `useCoarsePointer()` / `pointer: coarse` media query must gate any size expansion; desktop layout (fine pointer) must be unchanged.

### AC-7 — Existing LogRow gutter visual hierarchy is preserved
The narrow-screen breakpoint (≤380px) that hides the line number and shows only the badge must continue to work after any changes to badge tap behavior.

---

## Non-Goals

- Swipe-to-reveal row actions (separate feature)
- Reordering items via drag handle in the gutter (backlog reorder is a separate item)
- Terminal (ghostty) gutter improvements (separate sub-system)
- Full context-menu equivalent on mobile (long-press menus are a larger project)
