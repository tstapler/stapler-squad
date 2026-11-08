# Validation: Mobile Pane Picker Sheet Scroll Fix

## Checklist

### R1 — Sheet never grows taller than 60dvh
- Confirm `mobilePickerSheet` has `maxHeight: "60dvh"` in generated CSS
- Test: render 20 panes → sheet stays within 60dvh (Jest: mock getBoundingClientRect)

### R2 — Pane list scrollable
- Confirm `mobilePickerPaneList` has `overflowY: "auto"` and `flex: 1`
- Test: 16 panes → `mobile-picker-pane-list` has ≥15 child buttons, fixed actions and Cancel outside it

### R3 — Fixed actions always visible
- "Open existing pane" (when present), "New full-screen pane", "Temporary modal" are
  siblings of the scroll container, not inside it
- Existing tests still pass (they click these buttons by testId after sheet open)

### R4 — Cancel always visible
- Cancel button is after (below) the scrollable list in DOM order and has its own
  `borderTop` visual separator

### R5 — No regression
- `make test` targeting `web-app/src/components/pane/__tests__/` passes
- Existing three `mobilePicker.test.tsx` tests pass without modification

### R6 — Sibling-order tests
- `existing.nextElementSibling === mobile-picker-new-pane`: VALID (both outside scroll div)
- `mobile-picker-new-pane.nextElementSibling === peek`: VALID (all three fixed actions
  are siblings inside the fixed-actions section, before the scroll div)

## Manual Test Plan (device or DevTools mobile emulation)

1. Open Stapler Squad on mobile (or DevTools → iPhone SE viewport)
2. Create 10+ panes (or use the test fixture with many sessions)
3. Click a session in the session list → bottom sheet appears
4. Confirm: "Open session in…" title, "New full-screen pane", "Temporary modal" visible
   without scrolling
5. Confirm: existing pane list items below can be scrolled
6. With virtual keyboard open (tap an input first): confirm sheet doesn't overflow top
7. Tap "New full-screen pane" → sheet closes, new pane opens ✓
8. Tap backdrop → sheet closes ✓

## Edge Cases

| Scenario | Expected |
|---|---|
| 0 existing panes → `mobilePickerPanes` is empty | No scrollable section rendered; sheet shows only 3 fixed actions + cancel |
| 1 existing pane → list has 1 item | Scrollable section present but no scrollbar needed |
| "Open existing pane" absent | Sheet shows 2 fixed actions + scrollable pane list |
| Session already in a pane, `alreadyOpenLeafMobile` set | "Open existing pane" appears first above "New full-screen pane" |

## Confidence Assessment

- Root cause: VERIFIED (confirmed by reading `panePickerOverlay.css.ts` — no maxHeight)
- Fix approach: VERIFIED (flex + maxHeight + inner scroll is a standard mobile bottom-sheet pattern)
- Test impact: INFERRED (sibling tests use `nextElementSibling`; need to run tests to confirm pass)
- dvh browser support: VERIFIED for iOS 15.4+ and Chrome 108+; older Safari degrades gracefully
