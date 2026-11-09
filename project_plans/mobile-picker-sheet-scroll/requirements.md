# Requirements: Mobile Pane Picker Sheet — Scrollable with Pinned Actions

## Problem Statement

When the user has many panes open on mobile and taps a session in the session list, the
"Open session in…" bottom sheet can grow so tall that its top items scroll off the top of
the screen. The virtual keyboard compounds this by consuming ~40% of viewport height.
The options the user most commonly wants — "Open existing pane" and "New full-screen pane"
— are anchored at the top of the sheet and become unreachable.

The sheet has no `maxHeight` and no `overflow-y: auto`, so it expands unbounded and
cannot be scrolled by the user.

## Screenshot Evidence

Attached screenshot (`1793918318320-3555616810-10753.png`) shows 15 panes listed in the
sheet. "New full-screen pane" is partly clipped at the top of the visible area and
"Open existing pane" (which appears above it) is entirely off-screen. The virtual keyboard
is visible at the bottom. The cancel button at the bottom of the sheet is also not visible.

## Root Cause

`mobilePickerSheet` in `web-app/src/styles/pane/panePickerOverlay.css.ts` (lines 94–105):

```ts
export const mobilePickerSheet = style({
  position: "fixed",
  left: 0,
  right: 0,
  bottom: "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px))",
  zIndex: zIndex.mobilePickerSheet,
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
  padding: `${vars.space["4"]} 0`,
  animation: `${slideUp} 180ms ease`,
  // ← no maxHeight, no overflow-y: auto
});
```

The sheet grows from the bottom upward without bound. With 15 panes at ~48px each,
the sheet is ~720px tall — taller than the available viewport above the keyboard.

## Affected Files

| File | Change |
|---|---|
| `web-app/src/styles/pane/panePickerOverlay.css.ts` | Add `maxHeight`, `overflowY`, and inner scroll layout |
| `web-app/src/components/pane/PaneTilingContainer.tsx` | Restructure mobile sheet DOM: sticky header (fixed actions) + scrollable pane list body |
| `web-app/src/components/pane/__tests__/PaneTilingContainer.mobilePicker.test.tsx` | Add test: sheet with many panes still renders all items in DOM |

## Acceptance Criteria

1. The mobile picker sheet never grows taller than 60% of the viewport height (or 60dvh),
   regardless of how many panes are listed.
2. When the sheet is taller than the viewport clamp, its pane list is scrollable — the user
   can scroll down to see panes not visible on initial open.
3. "Open existing pane" (when present) and "New full-screen pane" and "Temporary modal"
   are pinned in a sticky/non-scrolling section at the top of the sheet so they are always
   visible regardless of pane count.
4. The Cancel button is pinned or always visible (not buried below the pane list).
5. All items remain in the DOM (no windowing/virtualisation) — existing tests continue to
   work with `.getByTestId("mobile-picker-open-existing")` etc.
6. The fix works when the virtual keyboard is open (the sheet must stay within
   `100dvh - keyboard height`; using `dvh` units or `env(keyboard-inset-bottom)` handles this).
7. No regression in existing `PaneTilingContainer.mobilePicker.test.tsx` tests.
8. No regression in `PaneTilingContainer.test.tsx` tests.

## Out of Scope

- Desktop pane picker overlay (a separate component, unaffected)
- Virtualising the pane list (overkill for this list size)
- Animation changes beyond preserving the existing `slideUp` keyframe
- Keyboard-height detection via VisualViewport API (level of effort exceeds the bug severity)
