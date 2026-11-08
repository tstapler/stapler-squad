# Plan: Mobile Pane Picker Sheet Scroll Fix

## Summary

Two-file change: add flex + maxHeight/overflow to the CSS, and wrap the per-pane list
in a scrollable `<div>` in the JSX. Add two tests; the existing sibling-order assertions stay valid. Cost class: XS
(one small worker run).

## Implementation Steps

### Step 1 — CSS changes (`panePickerOverlay.css.ts`)

**`mobilePickerSheet`**: add flex column layout and height bound.

```ts
export const mobilePickerSheet = style({
  position: "fixed",
  left: 0, right: 0,
  bottom: "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px))",
  zIndex: zIndex.mobilePickerSheet,
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
  padding: `${vars.space["4"]} 0 0`,  // remove bottom padding — cancel has its own
  animation: `${slideUp} 180ms ease`,
  // NEW:
  display: "flex",
  flexDirection: "column",
  maxHeight: "60dvh",  // 60 not 65: see Adversarial Review (keyboard-open headroom)
  // dvh falls back to vh on Safari < 15.4; acceptable degradation
});
```

**New export `mobilePickerPaneList`**: the scrollable section wrapping per-pane items only.

```ts
export const mobilePickerPaneList = style({
  overflowY: "auto",
  flex: "1 1 auto",
  minHeight: 0,
  overscrollBehavior: "contain",
  borderTop: `1px solid ${vars.color.borderColor}`,
});
```

### Step 2 — DOM restructure (`PaneTilingContainer.tsx`, mobile sheet render)

Before (flat list inside sheet):
```tsx
<div className={mobilePickerSheet}>
  <div className={mobilePickerSheetTitle}>Open session in…</div>
  {alreadyOpenLeafMobile && <button data-testid="mobile-picker-open-existing">...</button>}
  <button data-testid="mobile-picker-new-pane">New full-screen pane</button>
  <button data-testid="mobile-picker-peek-modal">Temporary modal</button>
  {mobilePickerPanes.map((p) => <button key={p.id}>...</button>)}
  <button className={mobilePickerCancelButton}>Cancel</button>
</div>
```

After (fixed actions + scrollable pane list):
```tsx
<div className={mobilePickerSheet}>
  {/* Always-visible header + fixed actions */}
  <div id="mobile-pane-picker-title" className={mobilePickerSheetTitle}>Open session in…</div>
  {alreadyOpenLeafMobile && (
    <button className={mobilePickerPaneItem} data-testid="mobile-picker-open-existing" ...>
      Open existing pane
    </button>
  )}
  <button className={mobilePickerPaneItem} data-testid="mobile-picker-new-pane" ...>
    New full-screen pane
  </button>
  <button className={mobilePickerPaneItem} data-testid="mobile-picker-peek-modal" ...>
    Temporary modal
  </button>
  {/* Scrollable pane list — only this region scrolls */}
  {mobilePickerPanes.length > 0 && (
    <div className={mobilePickerPaneList} data-testid="mobile-picker-pane-list">
      {mobilePickerPanes.map((p) => (
        <button key={p.id} className={mobilePickerPaneItem} ...>
          <span className={mobilePickerPaneLabel}>{p.letter}</span>
          {p.label}
        </button>
      ))}
    </div>
  )}
  {/* Always-visible cancel at bottom */}
  <button className={mobilePickerCancelButton} onClick={cancelPicker}>Cancel</button>
</div>
```

A thin divider (`borderTop: 1px solid borderColor`) between the fixed actions and the
scrollable list section helps the user understand what scrolls. This can be done by
giving `mobilePickerPaneList` a `borderTop` or by adding a visual separator element.

### Step 3 — Update tests (`PaneTilingContainer.mobilePicker.test.tsx`)

The two sibling-order assertions break because "Open existing pane" and "New full-screen pane"
are now siblings inside the fixed-actions section, but `mobile-picker-peek-modal` is also
in that section — sibling order is preserved. Confirm the structure still holds with
the new code, or switch to `within(dialog).getAllByRole('button')` ordering.

Specifically, test 2: `expect(existing.nextElementSibling).toBe(screen.getByTestId("mobile-picker-new-pane"))`
— still valid after restructure since both remain siblings outside the scroll div.

Test 3: `expect(screen.getByTestId("mobile-picker-new-pane").nextElementSibling).toBe(peek)`
— still valid for the same reason.

Add a new test:
```ts
it("renders a scrollable pane list when many panes are present", () => {
  // 16-pane tree: list has >=15 children; fixed actions and Cancel are outside it
  const paneList = screen.getByTestId("mobile-picker-pane-list");
  expect(paneList.children.length).toBeGreaterThanOrEqual(15);
});
```

## Adversarial Review

**Q: Does `65dvh` leave enough room on a small phone with keyboard open?**
iPhone SE 2 is 667px tall. With keyboard (~260px), available height is ~407px.
65dvh = ~433px — will slightly clip. Consider `60dvh` or clamping differently.
The sheet bottom is offset by `bottom-nav-height (64px)`, so effective max is
`65dvh - 0 = 65% of layout viewport`. On iOS, `dvh` shrinks with keyboard, so
`65dvh` ≈ 65% of (667 - 260) = 264px. That's enough for the 3 fixed actions
(~144px) plus 1–2 scrollable pane rows visible before scrolling.

**Verdict:** 60dvh is safer. Enough to show the 3 fixed actions without scrolling,
and shows the first few pane entries so the user knows there are more below.

**Q: Does removing `padding` bottom from the sheet break the cancel button visual?**
`mobilePickerCancelButton` has its own `padding` and `borderTop`. No issue.

**Q: Can the user accidentally dismiss the sheet by trying to scroll the backdrop?**
`mobilePickerBackdrop` has `onClick={cancelPicker}`. Touch scroll on the backdrop
fires click on touch-end if no scroll occurred — mobile browser behavior. The inner
sheet div stops propagation because it's a separate DOM node. No change here.

**Q: Does `-webkit-overflow-scrolling: touch` still do anything?**
Deprecated in iOS 13+; not harmful to include; remove if linting flags it. The
modern `overscroll-behavior: contain` on the pane list is a better addition:
prevents scroll chaining to the body when the list reaches its top/bottom.

## Final CSS for `mobilePickerPaneList`

```ts
export const mobilePickerPaneList = style({
  overflowY: "auto",
  flex: "1 1 auto",
  minHeight: 0,
  overscrollBehavior: "contain",
  borderTop: `1px solid ${vars.color.borderColor}`,
});
```

(Implemented as written; `minHeight: 0` was added so the flex child can shrink below its content.)

## Files Changed

| File | Change |
|---|---|
| `web-app/src/styles/pane/panePickerOverlay.css.ts` | Add flex/maxHeight to sheet; new `mobilePickerPaneList` export |
| `web-app/src/components/pane/PaneTilingContainer.tsx` | Wrap pane list in scrollable div; import new style |
| `web-app/src/components/pane/__tests__/PaneTilingContainer.mobilePicker.test.tsx` | Add scroll-list test; verify existing sibling tests still pass |
