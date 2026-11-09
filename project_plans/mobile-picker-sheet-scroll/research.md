# Research: Mobile Pane Picker Sheet Scroll

## Code Archaeology

### Mobile sheet rendering — PaneTilingContainer.tsx:324–412

The mobile picker sheet is rendered via `createPortal` into `document.body`:

```tsx
{mobilePickerSession && createPortal(
  <>
    <div className={mobilePickerBackdrop} onClick={cancelPicker} aria-hidden="true" />
    <div ref={mobilePickerSheetRef} className={mobilePickerSheet} role="dialog" ...>
      <div className={mobilePickerSheetTitle}>Open session in…</div>
      {alreadyOpenLeafMobile && <button data-testid="mobile-picker-open-existing">Open existing pane</button>}
      <button data-testid="mobile-picker-new-pane">New full-screen pane</button>
      <button data-testid="mobile-picker-peek-modal">Temporary modal</button>
      {mobilePickerPanes.map((p) => (
        <button key={p.id}>...</button>   ← arbitrary length
      ))}
      <button className={mobilePickerCancelButton}>Cancel</button>
    </div>
  </>,
  document.body
)}
```

Order in DOM (top → bottom inside sheet):
1. Title "Open session in…"
2. "Open existing pane" (optional, present when session already in a pane)
3. "New full-screen pane"
4. "Temporary modal"
5. N × existing pane buttons (the variable-length part that grows with pane count)
6. "Cancel"

The screenshot shows items 2–N overflowing the top of the visible area when N is large,
confirming the analysis: the fixed-action buttons (items 2–4) need to be pinned, and
only items 5–N need to be in a scrollable container.

### CSS — panePickerOverlay.css.ts:94–105

```ts
export const mobilePickerSheet = style({
  position: "fixed",
  left: 0, right: 0,
  bottom: "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px))",
  zIndex: zIndex.mobilePickerSheet,
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
  padding: `${vars.space["4"]} 0`,
  animation: `${slideUp} 180ms ease`,
});
```

Missing: `maxHeight`, `overflowY`. The sheet has no height bound.

### Keyboard inset on mobile Safari / Android Chrome

`100dvh` on modern mobile browsers uses the layout viewport (shrinks when the keyboard
is shown), so `max-height: 60dvh` will correctly exclude the keyboard area. On Safari
<15.4 `dvh` falls back to `vh`; in that case the sheet may still clip, but the gap is
minor (acceptable degradation). `env(keyboard-inset-bottom)` via Visual Viewport API
is not needed for this fix.

### Existing tests (PaneTilingContainer.mobilePicker.test.tsx)

Three tests:
- `offers a new full-screen pane that splits the focused pane` — clicks `mobile-picker-new-pane`
- `lists Open existing pane first when the session is already in a pane` — checks sibling order
  (`existing.nextElementSibling === screen.getByTestId("mobile-picker-new-pane")`)
- `offers a Temporary modal between New full-screen pane and existing panes` — checks sibling order
  (`mobile-picker-new-pane.nextElementSibling === peek`)

**Critical:** tests 2 and 3 assert DOM sibling order via `nextElementSibling`. If the DOM
is restructured (e.g., fixed-actions in a wrapper `<div>` and pane list in a separate
`<div>`), the sibling chain will break. The tests will need updating to reflect
the new structure — or the assertions should be switched to `getAllByRole / within()` queries.

### CSS architecture (vanilla-extract)

New style tokens needed:
- `mobilePickerSheetInner` — the scrollable list container (`overflow-y: auto`, `flex: 1`)
- No new theme tokens required (using existing `vars.*`)

### Comparable pattern in codebase

`scrolling/` project plan exists — suggests the team has dealt with mobile scroll issues before.
No existing `max-height` pattern on bottom sheets found, but the `mobilePickerBackdrop`
and `mobilePickerSheet` combo is the only bottom-sheet pattern in the codebase.

## Related Plans

- `pane-picker-fix/` — fixed a different bug (picker never appearing with 2+ panes, duplicate
  guard in reducer). Already shipped. No overlap with this issue.
- `mobile-session-list-scroll-restore/` — scroll-position preservation for session list on
  mobile pane switch. Different component, different mechanism.
- `mobile-ux-improvements/` — may contain related mobile UX context.

## Risk Assessment

| Risk | Likelihood | Mitigation |
|---|---|---|
| `nextElementSibling` assertions break when DOM is restructured | High | Update tests to use `within()` scoped queries |
| `dvh` not supported on old Safari | Low | Acceptable degradation; sheet just grows taller |
| Sheet feels cramped at 60dvh | Medium | Use 70dvh; adjust once real device-tested |
| Cancel button buried when pane list is long | Low | Pin Cancel outside the scroll container |

## Conclusion

**Root cause confirmed:** `mobilePickerSheet` has no height bound. With N existing panes,
the sheet grows to `~(title-height + 3×action-height + N×pane-height + cancel-height)`,
overflowing the viewport and clipping the top items.

**Fix:** 
1. Add `maxHeight: 60dvh`, `display: flex`, `flexDirection: column` to `mobilePickerSheet`
2. Wrap the per-pane list (`mobilePickerPanes.map(...)`) in a new `<div>` with `overflow-y: auto` and `flex: 1`
3. Keep fixed-action buttons (Open existing / New full-screen / Temporary modal) and Cancel
   outside the scroll container so they're always visible
4. Update the two sibling-order test assertions to use scoped queries
