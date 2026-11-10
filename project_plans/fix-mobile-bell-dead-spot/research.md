# Research: Fix Mobile Bell Dead Spot

## Component Map

### TrayHandle (`web-app/src/components/ui/TrayHandle.tsx`)
- Uses `useDraggableBell` hook; renders as a `<button>` with `trayHandle` class
- Conditional render: on phone, only renders if `floatingBellEnabled && !isVirtualKeyboardOpen`; on desktop (`isInnerScreen` = true), always renders
- Has `touchAction: none`, `userSelect: none` inline styles — pointer events are captured

### TrayHandle CSS (`NotificationPanel.css.ts`, line 1260)
```
position: fixed
top: 35%
right: 0
width: 44px
minHeight: 88px
zIndex: zIndex.slideOver
```
When the panel is open: `right: min(400px, 40vw)`.

### Session list scroll container (`paneSplit.css.ts`, line 110)
```js
export const sessionListScroll = style({
  flex: 1,
  overflowY: "auto",
  minHeight: 0,
});
```
No right padding. This is the wrapper rendered by `PaneSplitRenderer.tsx:325` around `<SessionList>`.

### Backlog list container (`backlog.css.ts`, line 81)
```js
export const listPane = style({
  flex: 1,
  overflow: "auto",
  padding: vars.space["4"],
});
```
No explicit right padding that accounts for the tray handle.

## Existing Pattern: `--bottom-nav-height`
`BottomNav.tsx` measures its own height and publishes it as `--bottom-nav-height` on `document.documentElement`. Other components (bell bounds, layout) consume it via `var(--bottom-nav-height)`. The same pattern should be used for the tray handle width.

## Existing Custom Property: `--tray-handle-inset`
Used in `NotificationToast.css.ts` (lines 369, 379) with a fallback of `60px`:
```
right: "calc(var(--tray-handle-inset, 60px) + var(--terminal-scrollbar-width, 0px))"
```
This property is used but never explicitly set — it always falls back to `60px`. This is a related but separate concern (toast positioning). We should publish the actual tray width separately for layout purposes.

## Proposed CSS Variable: `--tray-handle-width`
- Set to `44px` on `<html>` when TrayHandle is visible
- Set to `0px` (or `inherit`) when TrayHandle is not rendered  
- Consumed by `sessionListScroll` and `listPane` via `paddingRight: "var(--tray-handle-width, 0px)"`

## Alternative: Static Media Query Padding
Simpler but less precise: add `paddingRight: "44px"` to scroll containers at all widths (or above 900px). At wide desktop the session list is in a fixed-width 280px pane column so the bell may not overlap it, making the padding unnecessary waste. The CSS variable approach is more correct.

## Alternative: `pointer-events: none` on the handle when closed
Would let clicks pass through the bell to underlying content when the tray is closed. Breaks the ability to click the bell to open the tray — not viable.

## Affected List Views
Based on the codebase:
1. Session list (`PaneSplitRenderer.tsx` → `sessionListScroll`)  
2. Backlog list (`backlog.css.ts` → `listPane`)  
3. Any other full-width page-level lists at the affected viewport widths

## Viewport Widths Where Overlap Occurs
- 900–1099px: narrow desktop. Session list may be full-width in a single-pane layout. Bell is at right edge.
- ≥1100px: session list is typically in a fixed left pane (~280px); bell at viewport right edge. Overlap is less likely but still theoretically possible in full-screen single-pane mode.
- <900px: header hidden, BottomNav present. TrayHandle only renders if `floatingBellEnabled` (opt-in). Most users won't see the overlap here.

## Test Coverage Gaps
- `SessionListScrollWrapper.test.tsx` tests that the scroll wrapper exists and contains the session list, but does NOT test for right-side padding.
- No test asserts the layout accounts for the tray handle width.
