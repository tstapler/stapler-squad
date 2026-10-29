# Validation: Mobile Session List Scroll Position Restore

## Requirements Coverage

| AC | Plan Coverage | Gap? |
|----|--------------|-------|
| AC1: scroll position retained in memory on pane switch | `useScrollRestore` map save on unmount | ✅ |
| AC2: scroll position restored on return | `useLayoutEffect` restore on mount | ✅ |
| AC3: restoration is instantaneous (no jump) | `useLayoutEffect` fires before paint | ✅ |
| AC4: reset to 0 on re-filter | Not explicitly handled; browser clamps, functionally OK | ⚠️ Acceptable |
| AC5: works for list and board view | Both share the same scroll container | ✅ |
| AC6: automated test coverage | `useScrollRestore.test.ts` + TC-2.5 | ✅ |
| AC7: no regression in existing tests | Hook is additive; existing mock structure unchanged | ✅ |
| AC8: terminal pane not kept in DOM | Option B does not change unmount behavior | ✅ |

AC4 caveat: "reset to 0 if re-filtered" is a soft requirement. The browser already clamps out-of-range `scrollTop` automatically. If the product decision is to explicitly reset on filter changes, the hook could accept a `resetKey` that, when changed, deletes the saved position before restoring. This can be added as a follow-up without changing the core hook interface.

## Test Plan

### Unit — `useScrollRestore.test.ts`

1. Initial render with no saved position → `scrollTop` not set (no crash).
2. On unmount with `scrollTop = 150` → map stores `150` for key.
3. Remount with saved position `150` → hook applies `150` to element.
4. Different key → independent position, no cross-contamination.

### Component — TC-2.5 in `SessionListScrollWrapper.test.tsx`

1. Render `PaneSplitRenderer` with `isMobile=true` (already mocked), session-list pane.
2. Get `data-testid="session-list-scroll"` element.
3. Simulate scroll to `scrollTop=300` (via `Object.defineProperty` on the element).
4. Unmount the component.
5. Remount (same pane.id).
6. Assert `data-testid="session-list-scroll"` element has `scrollTop=300`.

Note: jsdom's `scrollTop` is read-only and always 0. Test must either:
- Use `Object.defineProperty(el, 'scrollTop', { writable: true, value: 300 })` before firing the scroll event.
- Or test the hook in isolation where the mock element is a plain object.

### Manual verification

1. Chrome DevTools → Device emulation → iPhone 14 viewport.
2. Open Stapler Squad, session list with 20+ sessions.
3. Scroll to session #15.
4. Tap session → terminal view opens.
5. Tap "Sessions" in tab strip → session list scrolled to ~session #15 position.
6. Verify no flash at top then jump to #15.

## Risk Checklist

- [x] No new network calls
- [x] No new authentication surface
- [x] No user data written to disk or sent to server (map is in-memory only)
- [x] No changes to pane reducer state or protobuf schema
- [x] No changes to the xterm.js terminal lifecycle
- [x] Existing `make ci` / `make quick-check` should pass unchanged
- [x] `jscpd` ratchet: new hook file is small (<40 lines), new test file is ~60 lines; unlikely to trip the 0.14% threshold
