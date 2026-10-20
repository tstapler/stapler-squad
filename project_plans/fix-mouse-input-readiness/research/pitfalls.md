# Mouse Event Forwarding Fix — Pitfalls & Risks

Codebase: `web-app/src/`
Files analysed: `useTerminalGestures.ts`, `TerminalOutput.tsx`, `XtermTerminal.tsx`,
`lib/terminal/mouseTracking.ts`, `lib/terminal/scrollRouting.ts`

---

## 1. Stale closure — `mouseMode` must flow through `optionsRef`, not a direct capture (CRITICAL)

**Location:** `useTerminalGestures.ts`, `useEffect` at line 133; deps at line 793.

The `useEffect` that builds the whole gesture machine runs only when
`[containerRef, terminalRef, gestureScrollEnabled]` change. All other
runtime values are read through `optionsRef.current` — that is the
explicit contract (line 793: `// options other than these are read via optionsRef`).

`mouseMode` lives in `TerminalOutput` as plain React state. It is currently
NOT passed to `useTerminalGestures` at all (absent from `scrollGestureProps`
at lines 1099-1111 and absent from the `useTerminalGestures({...})` call at
line 353 in `XtermTerminal.tsx`).

If the fix passes `mouseMode` into `GestureOptions` but `handleTap()` reads
it as a captured closure variable (as all the existing vars in the effect body
do), it will permanently read the value from the first render and never see
the user toggling it. The fix MUST:

1. Add `mouseMode?: 'none' | 'any'` to `GestureOptions`.
2. Thread it through `scrollGestureProps` in `TerminalOutput`.
3. Thread it through the `useTerminalGestures({...})` call in `XtermTerminal`.
4. Inside `handleTap()`, read it as `optionsRef.current.mouseMode ?? 'any'` —
   the same pattern used by `onSendData` at line 138.

A direct closure capture compiles and works until the user first toggles the
button, then silently stops working. This is the highest-probability
implementation mistake.

---

## 2. X10 encoding hardcoded; SGR encoding path undetectable via public xterm API (CRITICAL)

**Location:** `useTerminalGestures.ts` lines 637–643; `lib/terminal/mouseTracking.ts`;
`lib/terminal/scrollRouting.ts` lines 115–121.

`handleTap()` always sends X10-encoded events:
```
\x1b[M + char(32) + char(col+32) + char(row+32)   // press
\x1b[M + char(35) + char(col+32) + char(row+32)   // release
```

The scroll wheel path in `scrollRouting.ts` already uses SGR encoding
(`\x1b[<button;col;rowM`), explicitly calling `encodeWheelX10` a
"spike fallback" that is "deliberately not selectable". Terminal apps
(neovim, tmux, lazygit) that enable `?1006h` (SGR mouse encoding) alongside
`?1003h` (any-event tracking) will receive malformed tap events even when
the gate is open and working correctly.

The deeper problem is detection. xterm.js's `Terminal.modes.mouseTrackingMode`
(the only public API) reflects the tracking *mode* (none/x10/vt200/drag/any)
but NOT the encoding extension: `?1006h` (SGR) and `?1005h` (UTF-8 extended)
are separate DEC private mode flags that xterm.js tracks in internal state
not exposed on the public `modes` surface. `isMouseTrackingUtil()` in
`mouseTracking.ts` therefore cannot tell the implementer which encoding to use.

Options in priority order:
- **Best:** read `(terminal as any)._core._inputHandler._mouseService?.isSGREnabled`
  or a similar internal path (fragile, must be guarded with `?.`).
- **Good fallback:** always send SGR (`\x1b[<0;col;rowM` / `\x1b[<0;col;rowm`) when
  `mouseTrackingMode !== 'x10'` — modern apps enable SGR; legacy X10-only apps only
  set `?9h` which xterm reports as `x10`.
- **Minimum:** document in code that X10 is wrong for SGR-mode apps and file a
  follow-up; gate on `mouseMode` now, fix encoding separately.

Without the encoding fix, the gate opens correctly but sends a protocol-level
garbage event to any modern mouse-tracking app.

---

## 3. Reconnect does not reset `terminal.modes.mouseTrackingMode`; stale tracking persists (HIGH)

**Location:** `useTerminalGestures.ts` lines 124–131; `lib/terminal/mouseTracking.ts`.

`connectionEpoch` is bumped on reconnect/full-snapshot and cancels the
in-flight gesture (lines 127–129). It does NOT reset xterm.js's internal
terminal mode state. `isMouseTracking()` reads `terminal.modes.mouseTrackingMode`
live from the xterm instance, which persists across reconnect unless the PTY
sends an explicit disable sequence (`?1003l` / `?1000l`) or a full terminal
reset (`\x1bc` RIS).

Scenario: a Claude Code session in vim (mouse=all, `?1003h` enabled) crashes.
The session reconnects; the PTY replay omits the disable sequence. xterm's
`mouseTrackingMode` stays `'any'`. The gate `isMouseTracking() && mouseMode === 'any'`
remains open. The user never sees the new session's prompt cursor respond to taps
because mouse events are still being forwarded to a vim that is no longer running.

The existing `connectionEpoch` hook is the right place to add a mode reset.
After bumping the epoch, write `\x1b[?1003l\x1b[?1002l\x1b[?1001l\x1b[?1000l`
to the terminal (or `terminal.reset()` if clearing scroll history is acceptable)
so that `isMouseTracking()` returns false until the new PTY re-enables it.

---

## 4. `mouseMode` initial value: brief window on mobile where gate is incorrectly `'any'` (LOW)

**Location:** `TerminalOutput.tsx` lines 391–395.

```ts
const [mouseMode, setMouseMode] = useState<'none' | 'any'>('any');

useEffect(() => {
  setMouseMode(isMobile ? 'none' : 'any');
}, [isMobile]);
```

`useState` initialises synchronously to `'any'` before React runs effects.
The correction to `'none'` for mobile runs after the first commit. During
this one-render window, if `isMouseTracking()` happens to be true (a resumed
session that had mouse tracking on), a tap could pass the gate and send an
X10 event when the user expected selection behaviour.

In practice the risk is tiny — the terminal isn't usable in that sub-frame
window — but it could be eliminated by initialising from `isMobile` directly:
```ts
const [mouseMode, setMouseMode] = useState<'none' | 'any'>(() => isMobile ? 'none' : 'any');
```

---

## 5. Selection behaviour is isolated from the `mouseMode` gate — no regression risk (INFORMATIONAL)

**Location:** `useTerminalGestures.ts` lines 322–339.

`enterSelecting()` branches on `isMouseTracking()` directly, not on `mouseMode`.
The proposed plan only gates `handleTap()`, not the long-press selection path.
With `mouseMode === 'none'` and `isMouseTracking() === true` (user suppressed
mouse mode but PTY still owns tracking):
- `handleTap()` is blocked (correct).
- `enterSelecting()` still calls `beginDirectSelectDrag()` using xterm's
  `t.select()` API (also correct — this is the right path when the PTY owns the
  mouse).

No regression here; the two paths are already cleanly separated.

---

## Summary: Three Most Critical Risks

1. **Stale closure (will silently break the toggle)** — `mouseMode` must be read as
   `optionsRef.current.mouseMode` inside `handleTap()`, not captured directly; the
   gesture effect deps do not include it, so a captured value freezes at mount.

2. **X10 encoding sent when PTY expects SGR** — `handleTap()` always emits
   `\x1b[M…` (X10); modern apps enable `?1006h` (SGR) and expect `\x1b[<0;col;rowM`.
   xterm.js does not expose the encoding flag on its public `modes` surface, so
   detection requires either an internal accessor or a heuristic (always use SGR
   unless `mouseTrackingMode === 'x10'`). Without this fix, the gate opens but
   delivers garbled events to neovim, tmux, and lazygit.

3. **Stale tracking mode after reconnect** — `connectionEpoch` cancels in-flight
   gestures but does not reset xterm's `mouseTrackingMode`; a reconnected session
   that skips the disable sequence stays in tracking mode indefinitely, keeping
   the gate open and forwarding taps when the user expects focus/selection.
