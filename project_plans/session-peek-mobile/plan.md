# Plan: Session Peek Mobile

## Summary
Add a "Temporary modal" option to the mobile bottom-sheet pane picker that opens the `SessionPeekModal` (already implemented for desktop). The change is ~8 JSX lines in one component plus a new test case.

## Implementation

### Task 1: Add button to mobile picker (PaneTilingContainer.tsx)

In the mobile bottom-sheet section (around line 371, after the "New full-screen pane" button and before the `mobilePickerPanes.map`):

```tsx
<button
  className={mobilePickerPaneItem}
  data-testid="mobile-picker-peek-modal"
  onClick={() => {
    setPeekSession(mobilePickerSession);
    cancelPicker();
  }}
>
  {/* analytics-exempt */}
  <span className={mobilePickerPaneLabel}><Maximize2 size={14} /></span>
  Temporary modal
</button>
```

No new imports needed — `setPeekSession` and `mobilePickerSession` are already in scope in this closure.

### Task 2: Add test in PaneTilingContainer.mobilePicker.test.tsx

New test case:
```tsx
it("shows a 'Temporary modal' button and opens peek modal when tapped", async () => {
  // render with 2+ panes to trigger mobile picker
  // click a session card to open picker
  // assert data-testid="mobile-picker-peek-modal" is in the document
  // click the button
  // assert picker sheet is gone
  // assert SessionPeekModal is rendered (look for role="dialog" with aria-label matching session title, or peekBadge text)
});
```

### No other changes needed
- `SessionPeekModal` is unchanged
- No new CSS
- No backend changes
- No route-meta update (no new page)

## Adversarial Review

**Q: Does `cancelPicker()` clear `mobilePickerSession` before `setPeekSession` runs?**  
A: No — both are `useState` setters; React batches them. `mobilePickerSession` is captured by closure at the time the click fires, so `setPeekSession(mobilePickerSession)` gets the correct session. `cancelPicker()` then nulls `mobilePickerSession`. Safe.

**Q: What if the user taps "Temporary modal" and then immediately hits Escape?**  
A: `SessionPeekModal` captures Escape at the window level with `{ capture: true }`, closing itself. The picker is already gone. Correct behavior.

**Q: Could two modals stack (peek + another modal)?**  
A: Only one `peekSession` state exists; opening via mobile picker replaces any existing peek session. Same as the desktop path.

**Q: Is the label "Temporary modal" the right UX copy?**  
A: Desktop uses "Open in Modal". Mobile might prefer "Temporary modal" or "Open as overlay" to distinguish from a persistent pane. This is a bikeshed — either works. "Temporary modal" is consistent with the backlog item's title.

**Q: Icon choice — ◻?**  
A: Desktop uses no icon (text only in the action bar). The mobile picker uses span labels like "→", "+", "A"–"Z". A square outline (◻) suggests overlay without implying a split. Alternative: "⬜" or "◈". Can be revised by the implementer.

## Estimate
~1 hour total (8 lines of JSX + 1 test case).
