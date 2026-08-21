# Agent 1: Stack Analysis

## Key File Paths

| Concern | Path |
|---|---|
| Cockpit CSS (grid layout) | `web-app/src/styles/sessionCockpit.css.ts` |
| Main cockpit page | `web-app/src/app/page.tsx` |
| Session detail entry point | `web-app/src/components/sessions/SessionDetail.tsx` |
| Shortcut registry | `web-app/src/lib/shortcuts/shortcutRegistry.ts` |
| useShortcut hook | `web-app/src/lib/shortcuts/useShortcut.ts` |
| Theme contract (token names) | `web-app/src/styles/theme-contract.css.ts` |
| Theme implementation | `web-app/src/styles/theme.css.ts` |
| xterm.js wrapper | `web-app/src/components/sessions/XtermTerminal.tsx` |
| Terminal output (session stream) | `web-app/src/components/sessions/TerminalOutput.tsx` |

## Current Cockpit CSS Grid

`sessionCockpit.css.ts` exports four `recipe()` functions:

### `cockpitGrid`
- `display: grid; height: 100%; overflow: hidden`
- `contextPanelOpen: false` → `gridTemplateColumns: "280px 1fr"` (narrows to `240px 1fr` at ≤900px inner breakpoint)
- `contextPanelOpen: true` → `gridTemplateColumns: "280px 1fr 320px"`
- Mobile ≤768px: `gridTemplateColumns: "1fr"; gridTemplateRows: "1fr"` (collapses to single column)

### `sessionListColumn`
- `overflowY: auto; overflowX: hidden; borderRight: 1px solid vars.color.borderColor; display: flex; flex-direction: column`
- Mobile + session selected: `maxHeight: 0; overflow: hidden` (hides the session list)
- Mobile + no session selected: `borderBottom: 1px solid ...; flex: 1; minHeight: 0; overflow: auto`

### `detailColumn`
- `display: flex; flex-direction: column; overflow: hidden; minWidth: 0`
- Mobile + no session: `display: none`
- Mobile + session selected: `flex: 1; minHeight: 0`

### `contextPanel`
- Fixed 320px, slides in from right with CSS `transform: translateX`
- Mobile: `position: fixed; bottom: 0; height: 50vh` (sheet from bottom)

## How the Grid Is Used in page.tsx

The cockpit grid is a plain `<div className={cockpitGrid({ contextPanelOpen: false })}>` containing two child divs:
- Column 1: `<div className={sessionListColumn({ sessionSelected: !!selectedSession })}>` — contains `<SessionList />`
- Column 2: `<div className={detailColumn({ sessionSelected: !!selectedSession })}>` — contains `<SessionDetailBar />` + `<SessionDetail session={detailSession} />`

The detail column div uses `style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}` on the inner region wrapping `SessionDetail`. **This is the tiling root insertion point** — the pane tree engine will replace the single `<SessionDetail>` call inside the detail column.

There is currently no `tabIndex` or `data-context` on the detail column at the cockpit level; the inner session detail div has `tabIndex={-1}` and `role="region"`.

## ShortcutRegistry: Existing Contexts and Capabilities

Defined in `shortcutRegistry.ts`, exported singleton `registry`:

```typescript
export type ShortcutContext = "global" | "session-list" | "approval" | "terminal";
```

A new `"cockpit"` context will need to be added for pane navigation shortcuts.

**IME guard**: `if (event.isComposing) return;` — single-character shortcuts are IME-safe already.

**Input guard**: shortcuts without modifiers are skipped when focus is in `<input>/<textarea>/<select>/contenteditable`. Ctrl/Meta/Alt shortcuts bypass this guard — they will fire even inside input elements, so tiling shortcuts that use Ctrl must handle the case where the xterm terminal has focus (the xterm terminal is `data-context="terminal"` or similar).

**Terminal context rule**: shortcuts with `context: "terminal"` only fire when active context is terminal; shortcuts with non-terminal, non-global context are skipped when terminal is active. Tiling shortcuts should use context `"global"` or a new `"cockpit"` context registered above terminal in the hierarchy, then call `event.preventDefault()` before the event reaches xterm.

**Conflict detection**: `console.warn` on duplicate ID, last registration wins (overwrite). Registration returns a cleanup `() => void`.

**`useShortcut` hook**: registers on mount, deregisters on unmount; re-registers when `shortcut.action` identity changes — callers must `useCallback` their action.

## Theme Token Names (vars.*)

From `theme-contract.css.ts` (`vars = createThemeContract({...})`):

**Colors used in borders/focus:**
- `vars.color.borderColor` — standard border
- `vars.color.borderSubtle`, `vars.color.borderStrong`, `vars.color.borderHover`
- `vars.color.primary` — primary action color (good for focused pane border highlight)
- `vars.color.primaryHover`, `vars.color.primaryActive`
- `vars.color.cardBackground`, `vars.color.background`, `vars.color.hoverBackground`
- `vars.color.surfaceSubtle`, `vars.color.surfaceMuted`

**Spacing tokens:** `vars.space["1"]`…`vars.space["16"]` (4px, 8px, 12px, 16px, 24px, 32px, 48px, 64px)

**Radii:** `vars.radii.sm` (4px), `vars.radii.md` (6px), `vars.radii.lg` (12px)

**Font sizes:** `vars.fontSize.xs` (12px), `vars.fontSize.sm` (14px), `vars.fontSize.base` (14px)

**Breakpoints (constants, not tokens):** `breakpoints.md = "768px"`, `breakpoints.inner = "900px"`, `breakpoints.sm = "640px"`

**zIndex constants:** `zIndex.base = 0`, `zIndex.raised = 10`, `zIndex.header = 100`, `zIndex.dropdown = 500`, `zIndex.modal = 1000`

## Existing Drag/Resize Code

Files that contain `drag`, `resize`, `mousedown`, `touchstart`, or `pointerdown`:

- `web-app/src/lib/hooks/useTouchScroll.ts` — touchstart/touchmove for terminal scroll
- `web-app/src/lib/hooks/useMobileTerminalGestures.ts` — touch gestures (long press → selection)
- `web-app/src/components/sessions/XtermTerminal.tsx` — ResizeObserver for terminal fit, touch handling
- `web-app/src/components/sessions/TerminalOutput.tsx` — likely via XtermTerminal
- Various other files (modal click-outside, scroll controls, etc.)

**None of the existing drag code uses the Pointer Events API** (`onPointerDown`/`setPointerCapture`). The touch hooks use the `TouchEvent` API directly (`touchstart`, `touchmove`, `touchend`). The resize handle for tiling will be the first use of `PointerEvent` in the codebase — see Agent 3 for the recommended approach.

## SessionDetail Entry Point

`SessionDetail` is a large component (700+ lines) that:
- Takes `session: Session` prop
- Manages tab state (`"terminal" | "diff" | "vcs" | "logs" | "info" | "files"`)
- Uses a "pool" pattern: up to 8 terminal instances live simultaneously (hidden via `visibility: hidden + pointerEvents: none`), identified by `key={poolId}` where poolId = session ID
- The pool is keyed `key={poolId}` on the container div, not `key={poolId}` on `<TerminalOutput>` directly — but the pool div is keyed, so React will create/destroy TerminalOutput as sessions enter/leave the pool
- The terminal area is inside `<div style={{ position: 'relative', flex: 1, minHeight: 0 }}>` with absolutely positioned pool items

For tiling: when two panes show the same session simultaneously, you need `key={paneId + "-" + sessionId}` to force separate React instances — see Agent 4 for details.
