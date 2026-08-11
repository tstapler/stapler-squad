# Agent 4: Pitfalls and Gotchas

## 1. xterm.js FitAddon and ResizeObserver (CRITICAL)

`XtermTerminal.tsx` uses a `ResizeObserver` (line ~286) that calls `fitAddon.fit()` whenever the container element changes size. This is the automatic resize path. The pattern already exists and works correctly.

**What this means for tiling:** When the user drags a pane resize handle and the terminal's container changes width/height, the `ResizeObserver` inside `XtermTerminal` will detect the change and call `fit()` automatically. No additional wiring is needed in the tiling engine — as long as the terminal container div is a flex/grid child that actually changes dimensions during resize, the existing `ResizeObserver` handles it.

**Gotcha:** The `ResizeObserver` inside `XtermTerminal` has a debounce (10ms for first 3 resizes, 250ms thereafter) and uses double `requestAnimationFrame` for iOS Safari compatibility. This means terminal columns/rows update with a slight delay after drag. This is acceptable and expected behavior. Do not try to call `fit()` manually from the tiling engine — the existing observer is sufficient.

**Additional risk:** The pool pattern in `SessionDetail` wraps terminals in `visibility: hidden` divs. Hidden elements report a zero bounding box in some browsers, which can cause `proposeDimensions()` to return null or `0x0`. This is already guarded in the existing code (`if (width === 0 || height === 0) return`). No new risk from tiling as long as `visibility: visible` divs have real dimensions.

## 2. Duplicate Session in Two Panes — React Key Conflict (CRITICAL)

`SessionDetail` currently uses an internal terminal pool keyed by `poolId = session.id`. If two panes display the same `sessionId`, and both instantiate `<SessionDetail session={...} />` with the same `session.id`, React will see two separate component trees (different DOM nodes), so there is no React key conflict at the `SessionDetail` level. **However:**

- Each `SessionDetail` instance will create its own terminal pool, which means two xterm.js instances will both subscribe to the same session's terminal stream. This leads to duplicated output, doubled WebSocket connections, and potentially conflicting resize signals.

**Fix:** The pane leaf must include its own unique `paneId` as part of the React `key` on the `SessionDetail` wrapper, and each pane needs its own isolated terminal pool. Since terminal streaming in `TerminalOutput` uses the session ID as a WebSocket identifier, two simultaneous connections to the same session may not be supported by the backend.

**Recommended approach for MVP:** Prevent assigning the same session to two panes simultaneously. Display an error or warning in the pane header ("Session already open in another pane") and refuse the ASSIGN_SESSION action if `sessionId` already appears in another leaf. This avoids the duplicate stream problem entirely. Post-MVP, a read-only replay mode or shared-state terminals could be considered.

If same-session in two panes must be supported, use `key={paneId + "-" + sessionId}` on the outer div wrapping `SessionDetail`, so each pane gets a completely independent React subtree with its own pool.

## 3. Ctrl+W Conflicts with Browser Tab Close

`Ctrl+W` is the browser's "close tab" shortcut in Chrome, Firefox, and Edge on desktop. Registering `Ctrl+W` via `ShortcutRegistry` calls `event.preventDefault()` when the shortcut fires (line 106 of `shortcutRegistry.ts`). However, there is a caveat:

**The `beforeunload` dialog:** In most browsers, if the page hasn't been interacted with in a meaningful way, or if the pane close action is the only handler, `Ctrl+W` may close the browser tab before the JavaScript handler fires, depending on browser focus state. Specifically:
- Chrome ≥ 91: `Ctrl+W` on a page with unsaved state triggers `beforeunload`, which the page can intercept. But if there's no `beforeunload` handler and the shortcut fires in a non-focused context, the tab closes.
- **The ShortcutRegistry guard helps:** The registry dispatches `event.preventDefault()` before calling the action (line 106). This blocks the browser's built-in Ctrl+W behavior when the page's `keydown` handler fires first.
- **Risk area:** When the xterm terminal has focus inside the tiling pane, xterm.js may intercept `Ctrl+W` before the ShortcutRegistry sees it (xterm processes keydown events on its canvas element). The terminal context would need special handling.

**Recommended mitigation:** Register the `CLOSE_PANE` shortcut with `context: "global"` and use a `Ctrl+W` binding. Test explicitly in Chrome/Firefox to verify `preventDefault()` fires before the browser tab-close. If unreliable, offer an alternative binding (`Ctrl+Shift+W` or a close button in the pane header). The pane header close button (✕) is non-controversial and should be the primary close mechanism.

## 4. Ctrl+- Conflicts with Browser Zoom

`Ctrl+-` (Ctrl+Minus) is the browser's zoom-out shortcut universally in Chrome, Firefox, Edge, Safari. The ShortcutRegistry's `event.preventDefault()` in the dispatch loop (line 106) should block the browser zoom, but there is a known issue:

**Browser zoom shortcuts may be handled at a lower level** than `keydown` events in some browsers (particularly Safari on macOS). In Chrome and Firefox on Windows/Linux, `preventDefault()` on `keydown` for `Ctrl+-` reliably blocks zoom. On macOS Chrome/Safari, `Cmd+-` is zoom, so `Ctrl+-` is typically safe. On Windows, `Ctrl+-` zoom is blocked by `preventDefault()` in testing.

**Key name gotcha:** `KeyboardEvent.key` for the minus key is `"-"` (a literal hyphen). The shortcut registration must use `key: "-"` with `modifiers: { ctrl: true }`. Test this in the browser's devtools to confirm the key string.

**Recommended mitigation:** In the `ShortcutRegistry`, ensure `Ctrl+-` is registered as `{ key: "-", modifiers: { ctrl: true } }`. Add an e2e/manual test note to verify zoom is not triggered. If it proves unreliable on Safari, fallback to `Ctrl+Shift+H` (H for "horizontal split") as an alternative.

## 5. ShortcutRegistry IME Guard — Already Handled

The `ShortcutRegistry.dispatch()` method already has:
```typescript
if (event.isComposing) return;
```
This correctly skips all shortcuts during IME composition (Chinese, Japanese, Korean, etc.), preventing accidental pane splits while typing in Asian input methods. No additional work needed.

## 6. Cockpit Grid Architecture — Detail Column Is the Tiling Root

From `sessionCockpit.css.ts` and `page.tsx`, the current structure is:
```
cockpitGrid (CSS Grid: "280px 1fr")
  ├── sessionListColumn (flex column)
  └── detailColumn (flex column, minWidth: 0)
       ├── SessionDetailBar (32px header)
       └── [inner wrapper div, flex: 1, minHeight: 0]
            └── SessionDetail (single session)
```

The tiling engine replaces the `[inner wrapper div]` + `<SessionDetail>` with the pane tree renderer. The `detailColumn` div itself becomes the outer boundary. Its existing styles (`display: flex; flex-direction: column; overflow: hidden; minWidth: 0`) are correct for the tiling root — no changes needed to `sessionCockpit.css.ts`.

**The `sessionListColumn` resize handle (US-1)** is separate from the pane tree. It resizes the session list column (`gridTemplateColumns: "280px 1fr"` → variable). This requires modifying `cockpitGrid` to use a CSS custom property: `--list-col-width: 280px` → `gridTemplateColumns: "var(--list-col-width) 1fr"`. The list column drag handle sits between `sessionListColumn` and `detailColumn` at the cockpit grid level.

## 7. Mobile: Vertical Splits Stack on <768px

From `sessionCockpit.css.ts`, the mobile breakpoint is `breakpoints.md = "768px"`. On mobile:
- The cockpit grid collapses to a single column: `gridTemplateColumns: "1fr"; gridTemplateRows: "1fr"`
- `sessionListColumn` with `sessionSelected: true` collapses to `maxHeight: 0` (hidden)
- `detailColumn` with `sessionSelected: true` gets `flex: 1; minHeight: 0`

**Tiling interaction:** The requirements say vertical splits (side-by-side) should collapse to single-pane on `<768px` with a tab row to switch. Horizontal splits (top/bottom) remain.

The pane tree renderer needs to detect the mobile breakpoint and:
1. Filter out `direction: "vertical"` splits in the render path — render only the focused child
2. Add a tab strip at the bottom showing pane titles for switching

This can be done with a `useMediaQuery("(max-width: 768px)")` hook or by reading a `isMobile` prop from `ViewportProvider` (which already exists at `web-app/src/components/providers/ViewportProvider.tsx`).

**Check `ViewportProvider`:** This file was in the drag/resize grep results — it likely provides a `useViewport()` hook. Use it rather than adding a new media query hook.

## 8. ShortcutContext — "cockpit" Context Needs to be Added

The current `ShortcutContext` union is:
```typescript
export type ShortcutContext = "global" | "session-list" | "approval" | "terminal";
```

The tiling shortcuts (`Ctrl+\`, `Ctrl+-`, `Ctrl+W`, etc.) need a new `"cockpit"` context so they:
1. Only fire when the cockpit area is focused (not in a modal or other page)
2. Appear in the `?` overlay under a "Cockpit / Panes" section

Adding `"cockpit"` requires modifying `shortcutRegistry.ts` to expand the `ShortcutContext` type and the `getAll()` result object. The `getAll()` return type `Record<ShortcutContext, Shortcut[]>` is exhaustively typed, so TypeScript will catch any missing cases.

Also update the `?` overlay component (wherever it renders the grouped shortcuts) to add a section for `"cockpit"`.

## 9. Session List Click → Focused Pane (US-3 Integration)

Currently `handleSessionClick` in `page.tsx` calls `setSelectedSession(session)` which affects a single global session state. With tiling, clicking a session in the list must route to the focused pane via `dispatch({ type: "ASSIGN_SESSION", paneId: focusedPaneId, sessionId: session.id })`.

This requires lifting pane state to the `HomeContent` component level (or a context), and passing `dispatch` + `focusedPaneId` to `SessionList`. The existing `handleSessionClick` prop chain will need updating.

## 10. localStorage Key Collision — Session List Width vs. Pane Layout

The requirements specify two separate localStorage keys:
- `cockpit.listColumnWidth` (US-1: session list resize)
- `cockpit.paneLayout` (US-6: full pane tree)

These are distinct and won't conflict. However, on first load, if `cockpit.paneLayout` exists in localStorage but references a session that no longer exists, the `validateAndRepair` function (Agent 2) must be called before first render to avoid a flash of wrong session content. Load order: restore layout → validate against current session list → render.

Since sessions are loaded asynchronously, the layout should be restored optimistically on mount and re-validated once `sessions` are available (not null/loading). Show empty pane slots (not the old stale session) during the initial load window.
