# triggerPicker Callers — Full Call Graph

## Function Definitions

Both functions are defined as `useCallback` closures in `PaneTilingContainer.tsx`.

### `triggerPicker` (lines 68–98)

```ts
const triggerPicker = useCallback(
  (session: Session, tab?: string) => {
    // BUG 1 — bypass: if focused pane is session-detail, assign directly regardless of pane count
    const focusedLeaf = findLeaf(state.root, state.focusedPaneId);
    if (focusedLeaf && focusedLeaf.viewKind === "session-detail") {
      dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
      dispatch({ type: "ASSIGN_TAB", paneId: state.focusedPaneId, tab: resolvedTab });
      return;
    }

    const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");
    if (eligiblePanes.length === 0) {
      dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
    } else if (eligiblePanes.length === 1) {
      dispatch({ type: "ASSIGN_SESSION", paneId: eligiblePanes[0].id, sessionId: session.id });
      dispatch({ type: "ASSIGN_TAB", ... });
      dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
    } else {
      setPickerPendingSession(session);
    }
  },
  [state.root, state.focusedPaneId, dispatch],
);
```

### `triggerPickerForceNew` (lines 47–66)

```ts
const triggerPickerForceNew = useCallback(
  (session: Session, tab?: string) => {
    // Chooses split direction based on container width
    const direction = containerWidth / 2 >= 960 ? "vertical" : "horizontal";
    dispatch({
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: state.focusedPaneId,
      sessionId: session.id,
      tab: resolvedTab,
      direction,
    });
  },
  [state.focusedPaneId, dispatch, containerRef],
);
```

Both are exposed through `PaneContext` (type defined in `PaneContext.ts`):
```ts
export interface PaneContextValue {
  triggerPicker: (session: Session, tab?: string) => void;
  triggerPickerForceNew: (session: Session, tab?: string) => void;
  // ...
}
```

## Direct Callers

### 1. `PaneSplitRenderer.tsx` (lines 146, 161–162)

```ts
const { triggerPicker, triggerPickerForceNew } = usePaneContext();
// ...
<SessionList
  onSessionClick={triggerPicker}
  onSessionOpenInNewPane={triggerPickerForceNew}
  ...
/>
```

`SessionList` receives these as `onSessionClick?: (session: Session) => void` and
`onSessionOpenInNewPane?: (session: Session) => void` props (defined in `SessionList.tsx:53–54`).

Inside `SessionList.tsx` (lines 847–848):
```ts
onClick={() => onSessionClick?.(session)}
onOpenInNewPane={onSessionOpenInNewPane ? () => onSessionOpenInNewPane(session) : undefined}
```

`triggerPicker` is called with the full `Session` object and no tab argument (defaults
to `"terminal"` inside the function). `triggerPickerForceNew` likewise.

### 2. `PaneTilingContainer.tsx` — `externalSessionAssign` effect (lines 134–146)

```ts
useEffect(() => {
  if (!externalSessionAssign) return;
  if (externalSessionAssign.version === prevVersionRef.current) return;
  prevVersionRef.current = externalSessionAssign.version;

  const session = sessions.find((s) => s.id === externalSessionAssign.sessionId);
  if (!session) return;
  if (externalSessionAssign.forceNewPane) {
    triggerPickerForceNew(session, externalSessionAssign.tab);
  } else {
    triggerPicker(session, externalSessionAssign.tab);
  }
}, [externalSessionAssign, sessions, triggerPicker, triggerPickerForceNew]);
```

This effect fires when `externalSessionAssign` changes. The prop carries a `version`
counter to distinguish repeated assignments of the same session. The `tab` field is
threaded through.

## Indirect Callers (externalSessionAssign providers)

### 3. `app/page.tsx` (lines 159–165, 475)

Sets `externalSessionAssign` in two cases:

**URL param deep-link** (`?session=<id>&tab=<tab>&newPane=true`):
```ts
setExternalAssignSession({
  sessionId: session.id,
  tab: resolvedTab,
  forceNewPane: newPaneParam === "true",
});
```

`forceNewPane: true` → `triggerPickerForceNew` → `SPLIT_AND_ASSIGN_SESSION`
`forceNewPane: false` (default) → `triggerPicker`

**Omnibar / keyboard navigation** (lines 440–441):
```ts
onSessionClick: handleSessionClick,
```
`handleSessionClick` (not shown in the excerpt, but present in page.tsx) calls
`setExternalAssignSession` to route through the pane system.

The prop is threaded at line 475:
```ts
<PaneTilingContainer
  ...
  externalSessionAssign={externalAssignSession ? {
    ...externalAssignSession,
    version: externalAssignCounter,
  } : null}
/>
```

## Data Flow Summary

```
User click session in SessionList
  → onSessionClick(session)                       [SessionList.tsx:847]
  → triggerPicker(session)                        [PaneSplitRenderer.tsx:161]
  → { dispatch ASSIGN_SESSION } or               [PaneTilingContainer.tsx:76–79]
    { setPickerPendingSession(session) }          [PaneTilingContainer.tsx:94]

User Alt+click / "Open in new pane"
  → onSessionOpenInNewPane(session)               [SessionList.tsx:848]
  → triggerPickerForceNew(session)                [PaneSplitRenderer.tsx:162]
  → dispatch SPLIT_AND_ASSIGN_SESSION             [PaneTilingContainer.tsx:57–63]

URL nav / omnibar select / keyboard
  → setExternalAssignSession({sessionId, tab, forceNewPane})  [page.tsx]
  → externalSessionAssign prop changes
  → useEffect fires                               [PaneTilingContainer.tsx:134]
  → triggerPicker(session, tab) or
    triggerPickerForceNew(session, tab)
```

## Edge Cases from Changing triggerPicker

### R1 Fix: Remove the focused-detail bypass for 2+ panes

The change is:
```ts
// Before (Bug 1):
if (focusedLeaf && focusedLeaf.viewKind === "session-detail") {
  dispatch ASSIGN_SESSION to focusedPaneId; return;
}

// After:
const eligiblePanes = ...;
if (eligiblePanes.length === 1 && focusedLeaf?.viewKind === "session-detail") {
  dispatch ASSIGN_SESSION to focusedPaneId; return;
}
// For 2+ panes: fall through to setPickerPendingSession
```

**Edge cases to verify:**

1. **Single detail pane focused**: should still assign directly. The existing
   `eligiblePanes.length === 1` branch already handles this. No regression.

2. **Session-list pane focused, 1 detail pane**: uses `eligiblePanes[0].id`, not
   `state.focusedPaneId`. Already correct.

3. **Session-list pane focused, 2+ detail panes**: already reaches `setPickerPendingSession`.
   Not affected by the bypass removal.

4. **Detail pane focused, 2+ detail panes (Bug 1 scenario)**: the bypass currently
   fires before the `eligiblePanes.length` check, so the picker never shows. After
   fix, bypassing must be conditional on `eligiblePanes.length === 1`.

5. **`externalSessionAssign` with `tab` specified**: The bypass path dispatches
   `ASSIGN_TAB` separately. The single-pane path also dispatches `ASSIGN_TAB`.
   After fix, both code paths must continue to dispatch `ASSIGN_TAB`. The 2+ pane
   path (`setPickerPendingSession`) defers tab selection to the picker overlay click
   handler (line 272: `dispatch ASSIGN_TAB ... tab: "terminal"`). The keyboard handler
   (line 122) also dispatches `ASSIGN_TAB tab: "terminal"`. The requested `tab` value
   from `externalSessionAssign` is lost in the picker path — this is pre-existing
   behavior and out of scope for the fix.

6. **`ASSIGN_SESSION` dispatched from triggerPicker will now trigger move-and-clear
   (after R2 fix)**: this is the desired interaction. The picker path dispatches
   `ASSIGN_SESSION` to the selected pane id; the reducer move-and-clear logic then
   ensures the source pane is cleared automatically.

7. **Keyboard picker (A–Z) dispatches `ASSIGN_SESSION` directly** (lines 119–122):
   also benefits automatically from the R2 fix in the reducer without any change to
   the keyboard handler.

8. **Picker overlay click dispatches `ASSIGN_SESSION` directly** (line 272):
   same — benefits from R2 reducer fix automatically.

## Key Invariant: cancelPicker Always Fires After Assignment

The requirements (R6) require `cancelPicker()` to fire after every successful assignment.
Current code paths:
- Keyboard handler: `cancelPicker()` called at line 124 after `dispatch`
- Picker overlay click: `cancelPicker()` called at line 274 after `dispatch`
- Direct assignment (bypass path): never calls `cancelPicker()` because `pickerPendingSession`
  is still null (picker was never shown) — correct

After the fix, the same invariant holds: the bypass path (single pane) never shows
the picker, so `cancelPicker()` is not needed. The picker overlay and keyboard handler
still call `cancelPicker()` after dispatch.
