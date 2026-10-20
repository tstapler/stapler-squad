# Mouse Input Gating — Architecture Research

## 1. Where Should Gating Happen?

### Source 1: `handleTap()` in `useTerminalGestures.ts`

`handleTap()` (lines 628–645) already branches on `isMouseTracking()`. The X10 sequences are sent in the `else if (t)` branch — specifically when `isMouseTracking()` is true. The correct gate is a second check inside that branch.

The cleanest approach: add `mouseModeEnabled?: () => boolean` to `GestureOptions`. The hook already stores all options in `optionsRef` (line 117–119) specifically to avoid stale closures, and `isInputBusy` follows the same getter pattern. Gate becomes:

```ts
} else if (t && (optionsRef.current.mouseModeEnabled?.() ?? true)) {
  // send X10 press/release
}
```

No listener re-registration, no ref juggling — just one option read at call time.

### Source 2: xterm.js native SGR mouse events

xterm generates mouse sequences from native browser mouse/touch events and emits them via its `onData` callback (outgoing, user-input direction) when `terminal.modes.mouseTrackingMode !== "none"`. That callback flows up through `XtermTerminal.tsx`'s `onData` prop to `TerminalOutput.tsx`, and from there to the PTY.

There is no public `ITerminalOptions` field in xterm.js 6 to suppress native mouse forwarding (comment at `XtermTerminal.tsx:294–295` confirms this). The cleanest gate is a filter wrapper applied to the `onData` callback passed to xterm inside `XtermTerminal.tsx`: when a `mouseMode: 'none' | 'any'` prop is provided and is `'none'`, drop bytes that match the SGR mouse sequence prefix (`\x1b[<`). This keeps the filter co-located with the terminal creation and does not require changes to xterm's internal state.

## 2. How Should `mouseMode` Be Threaded?

Current state: `mouseMode` (`'none' | 'any'`) is owned by `TerminalOutput.tsx` (line 391) and is **not** in `scrollGestureProps` (lines 1099–1112). It reaches neither `XtermTerminal.tsx` nor `useTerminalGestures`.

Threading path:

```
TerminalOutput.tsx: mouseMode state
  → scrollGestureProps (useMemo, line 1099):
      add: mouseModeEnabled: () => mouseMode === 'any'
  → XtermTerminalProps.scrollGesture (ScrollGestureProps interface, XtermTerminal.tsx:153)
      add field: mouseModeEnabled?: () => boolean
  → useTerminalGestures({ ..., mouseModeEnabled: scrollGesture?.mouseModeEnabled })
      consumed in handleTap()
```

For xterm's own `onData` filtering: `mouseMode` can travel through `XtermTerminalProps` directly (not via `ScrollGestureProps`, since it applies to the terminal's native event handling, not gesture routing). Use a ref inside `XtermTerminal.tsx` so the filter closure stays stable.

Three touchpoints total:
1. `ScrollGestureProps` in `XtermTerminal.tsx` — add `mouseModeEnabled?: () => boolean`
2. `useTerminalGestures` call in `XtermTerminal.tsx` — thread the option through
3. `scrollGestureProps` useMemo in `TerminalOutput.tsx` — supply the getter

If native xterm SGR filtering is also needed: one additional touchpoint in `XtermTerminalProps` + a ref-backed filter wrapper in `XtermTerminal.tsx`'s terminal `onData` setup.

## 3. PTY-Tracking-Based vs User-Preference-Based vs Both

The correct gate is **both**: `isMouseTracking(terminal) && mouseMode === 'any'`.

- PTY-only (`isMouseTracking()` alone): the user's "Mouse OFF" toggle would have no effect — the toolbar button would be cosmetic-only, as it is today.
- User-preference only (`mouseMode === 'any'` alone): X10 sequences would be sent to programs that never requested mouse tracking, corrupting their input streams.
- Both: semantically correct — "the PTY asked for mouse, and the user hasn't opted out." Matches the toolbar's documented intent ("enable for vim/tmux").

## 4. `scrollMode.mouseTrackingMode` vs `mouseMode` Relationship

These are different axes:

| | Source | Updated by | Represents |
|---|---|---|---|
| `scrollMode.mouseTrackingMode` | `readScrollMode(terminal)` via `onScrollModeChange` | PTY escape sequences | What the foreground program requested |
| `mouseMode` | `useState` in `TerminalOutput.tsx` | User toolbar tap | User's forwarding preference |

The design `isMouseTracking(terminal) && mouseMode === 'any'` maps these correctly: `isMouseTracking` reads the live xterm modes object (not the derived `scrollMode` state, which is one render behind), so the check is synchronous at gesture time. `mouseMode` is the async user override.

Note: `scrollMode` is the upward-reported snapshot used for scroll routing decisions. For the gate inside `handleTap`, read `isMouseTracking(terminalRef.current)` directly (as the existing code already does) rather than going through `scrollMode` — `scrollMode` is a React state value that could lag by one render.

## 5. Tech Debt Disposition: Extend-as-is

No refactor needed. The hook's options-ref pattern (`optionsRef.current`, line 117–119) was designed exactly for this: a live option that updates without re-registering touch listeners. Adding `mouseModeEnabled?: () => boolean` follows the `isInputBusy` precedent identically.

The `ScrollGestureProps` interface is the established seam for threading gesture-related settings from `TerminalOutput.tsx` through `XtermTerminal.tsx` to the hook. Adding one field there is additive.

The xterm native SGR filter, if pursued, is two lines in the terminal init effect inside `XtermTerminal.tsx`: a ref for the current mode and a wrapper around the `onData` callback. No architecture changes.

**Verdict: Extend-as-is.** The seam (`GestureOptions`, `ScrollGestureProps`) already exists and accepts this extension cleanly.
