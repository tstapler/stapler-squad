# Research: Session Peek Mobile

## Existing Infrastructure

### SessionPeekModal
- **File:** `web-app/src/components/sessions/SessionPeekModal.tsx` (76 lines)
- Hand-rolled portal overlay (backdrop + fixed panel), does NOT use Radix `Modal.tsx`
- Props: `{ session: Session; onClose: () => void }`
- Renders `<SessionDetail>` inside the panel — full terminal/tabs
- Escape key closes (captured at window level), backdrop click closes
- Shows a "Peek" badge and a BacklogOriginBadge
- Focuses the modal div on mount

### Desktop picker wiring (reference)
In `PaneTilingContainer.tsx`:
```tsx
// state
const [peekSession, setPeekSession] = useState<Session | null>(null);
const openPeek = useCallback((session: Session) => setPeekSession(session), []);

// desktop action bar button (line ~290)
<button onClick={() => {
  setPeekSession(pickerPendingSession);
  cancelPicker();
}}>
  Open in Modal
</button>

// render peek modal (line ~312)
{peekSession && (
  <SessionPeekModal
    session={peekSession}
    onClose={() => setPeekSession(null)}
  />
)}
```

### Mobile picker current buttons (PaneTilingContainer.tsx lines 338–392)
1. "→ Open existing pane" — `FOCUS_PANE` + `cancelPicker()`
2. "+ New full-screen pane" — `SPLIT_AND_ASSIGN_SESSION` + `cancelPicker()`
3. `{p.letter} {p.label}` (per existing pane) — `ASSIGN_SESSION` + `ASSIGN_TAB` + `FOCUS_PANE` + `cancelPicker()`
4. "Cancel" — `cancelPicker()`

### CSS
`panePickerOverlay.css.ts` already has `mobilePickerPaneItem` class (same styling as other items). The new button can reuse this class.

### Tests
- `web-app/src/components/pane/__tests__/PaneTilingContainer.mobilePicker.test.tsx` — existing mobile picker tests. The new button test goes here.

## Key Findings
1. `peekSession` state and `SessionPeekModal` render are already present in `PaneTilingContainer`. The mobile picker just needs to call `setPeekSession(mobilePickerSession)` + `cancelPicker()`.
2. `cancelPicker` sets both `pickerPendingSession` and `mobilePickerSession` to null.
3. No new state, no new components, no CSS additions needed — purely a button insertion + test.
4. The change is ~8 lines of JSX in the mobile sheet section.

## Risk
- Low. `SessionPeekModal` is stable and already exercised by the desktop flow.
- The only new surface is the button itself. No pane-tree mutations.
- Mobile tests use `@testing-library/react` with mock `createPortal`; the pattern is established in the existing mobile picker test file.
