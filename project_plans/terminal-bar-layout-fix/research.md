# Research: Terminal Bar Layout Fix

**Date:** 2026-10-07

## Current Toolbar Structure (as-coded)

### Files

| File | Role |
|---|---|
| `web-app/src/components/sessions/TerminalOutput.tsx` | Main component; toolbar JSX ~lines 2302–2600 |
| `web-app/src/components/sessions/TerminalOutput.css.ts` | Vanilla-extract styles for toolbar |
| `web-app/src/components/sessions/ScrollingPanel.tsx` | `ScrollModeChip` used in toolbar |

### Toolbar DOM hierarchy (mobile, toolbar expanded)

```
div.toolbar                       ← flexbox row, space-between
  div.status                      ← left: status dot + text
  div.actions                     ← right: flexbox row, gap 0.25rem (mobile)
    button.toolbarToggle          ← always: ⋯ / ✕  (44×44 min)
    button.mobileKeyboardToggle   ← always: ⌨️       (44×44 min)
    button (Redraw)               ← always: ↔️ Redraw  (44×44 min + padding)
    ScrollModeChip                ← always: chip (variable width)
    div.toolbarActions            ← conditional on toolbarExpanded
      div.secondaryGroup          ← hidden on mobile (display:none @≤768px)
        Copy / Paste / Bottom / Clear / Scrolling / Mouse
      button (Gallery)            ← visible but scrolled off on small screens
      button (Files)              ← visible but scrolled off on small screens
      button.mobileOnlyUpload    ← 📷 camera (hidden on fine-pointer)
      button (Dev tools toggle)  ← devOnly, hidden on mobile
      ...dev panel...
      button.mobileMoreButton    ← More ▾  (only visible @≤768px)
```

### Root Cause

The `div.actions` is a single-row flexbox. On mobile it has `gap: 0.25rem` and the
`toolbarActions` child is also a flex row with `overflowX: auto`. Because `actions`
itself has `justifyContent: space-between` at the outer level and `toolbarActions` is
appended inline, the whole right side is a single horizontal strip.

At 375 px screen width, the always-visible strip already consumes approximately:
- `toolbarToggle`: 44 px
- `mobileKeyboardToggle`: ~50 px (44 px min + padding)
- Redraw button text label: ~98 px (icon + " Redraw" + padding)
- `ScrollModeChip`: ~70 px (variable)
- Total right side: ~262 px

With `status` taking ~90 px, total consumed ≈ 352 px — leaving ~23 px for the expanded
`toolbarActions`, which holds Gallery + Files + More. These buttons each need ≥44 px
touch targets, so they overflow the container and the fade mask hides them.

### Existing Mobile Patterns

- **`mobileMoreButton`** already exists: a "More ▾" button that opens `mobileOverflowRow`
  below the toolbar. This row uses `flex-direction: row-reverse` (right-handed mode) and
  is a separate `flexShrink: 0` div that never competes for toolbar width.
- **`secondaryGroup`** is already hidden on mobile (`display: none @≤768px`) and its
  actions (Copy, Paste, etc.) appear in the overflow row via `mobileOverflowRow`.
- The pattern is established: primary toolbar = status + always-critical controls;
  overflow row = everything else.

### Why Gallery/Files Are Not in the Overflow Row Today

Gallery and Files are inside `toolbarActions` (expanded section) but NOT inside the
`mobileOverflowRow` conditional (`mobileOverflowOpen && toolbarExpanded`). The
`mobileOverflowRow` only re-renders `secondaryActions.map(...)` — it does not include
Gallery, Files, or Camera. This is the gap: the upload buttons were added to the
expanded section without a corresponding overflow-row entry.

### Redraw Button Width

`TerminalOutput.tsx:2375–2385`:
```jsx
<button className={styles.toolbarButton} ...>
  ↔️ Redraw
</button>
```

On mobile, `toolbarButton` gets:
```ts
padding: "0.4rem 0.6rem",
fontSize: "0.8rem",
minHeight: "var(--min-touch-target, 44px)",
minWidth: "var(--min-touch-target, 44px)",
```

"↔️ Redraw" at 0.8rem ≈ 12.8 px with icon is ~70 px content + 2×9.6 px padding ≈ 89 px.
Making it icon-only (↔️) saves ~55 px on narrow screens. A `title` attribute still
provides a tooltip on long-press / hover.

### CSS Architecture

The project uses **vanilla-extract** (`@vanilla-extract/css`). All style changes go in
`TerminalOutput.css.ts`. Media query breakpoints in use: `max-width: 768px` (mobile) and
`pointer: fine` (mouse/trackpad).

A new breakpoint at `max-width: 480px` (small phones) is appropriate for icon-only
Redraw, since tablets ≥481 px can accommodate the label.

### Test Coverage

- `web-app/src/components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx` —
  tests keyboard key dispatch (no layout assertions).
- `web-app/src/components/sessions/__tests__/TerminalOutput.toolbar-analytics.test.tsx` —
  tests that `track()` is called with the right button labels; does not assert text
  content of buttons (uses `aria-label` / `data-testid`).
- No existing test asserts button text labels, so making Redraw icon-only on mobile
  (JSX change) does not break tests as long as `aria-label` is preserved.

### Related Components

- `ScrollModeChip` (`ScrollingPanel.tsx`) — also always-visible and variable-width.
  Its width depends on the current scroll mode label. Out of scope for this item.
- `mobileOverflowRow` — the established pattern for secondary mobile actions.

## Key Findings

1. **Fix A (quick, high-impact):** Make the Redraw button icon-only at ≤480 px by adding
   a `@media` breakpoint to a new CSS class, or by conditional JSX using `isMobile`.
   Saves ~55 px; Gallery and Files become reachable.

2. **Fix B (structural, addresses root cause):** Move Gallery, Files, and Camera into the
   `mobileOverflowRow` so they follow the established "primary bar is narrow" pattern.
   This ensures they're always reachable regardless of how many always-visible buttons
   exist.

3. **Fix C (polish):** Remove dead-space on the left of Redraw (the gap between
   ScrollModeChip and Redraw) — this follows naturally from Fix A because the Redraw
   button shrinks and the flexbox gap stays consistent.

Both A + B together constitute the full fix. C is implicit. Neither requires backend
changes or proto regeneration.
