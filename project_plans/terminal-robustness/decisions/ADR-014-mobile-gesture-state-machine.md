# ADR-014: Mobile Gesture State Machine Design

## Status: Proposed

## Context

Two React hooks — `useTouchScroll.ts` and `useMobileTerminalGestures.ts` — currently coexist in
`XtermTerminal.tsx` (registered at lines 115 and 136 respectively), both attaching non-passive
`touchmove` listeners to the same container element. This creates several conflicts:

- Both hooks call `terminal.scrollLines()` from `touchmove` events using incompatible delta
  strategies: `useTouchScroll` updates `touchStartY` cumulatively each move (relative delta);
  `useMobileTerminalGestures` measures total displacement from `touchState.startY` to decide scroll
  vs. select, then uses `lastY` delta for the scroll amount. When both handlers fire on the same
  event, the terminal double-scrolls.
- When `useMobileTerminalGestures` enters the long-press selecting state (`isSelecting = true`),
  `useTouchScroll` has no visibility into this flag and continues calling `terminal.scrollLines()`
  on every vertical `touchmove` — scrolling the terminal while the user is trying to extend a
  text selection.
- Both register `{ passive: false }` on `touchmove`. `useTouchScroll` fires first (registration
  order). Even if `useMobileTerminalGestures` calls `event.stopImmediatePropagation()`, the damage
  from the first handler's `scrollLines()` call is already done.
- The private cell-height API (`terminal._core._renderService.dimensions.css.cell.height`) used in
  `useMobileTerminalGestures.ts` line 114 is undefined until after the first render frame in
  xterm.js 6.x (pitfall #6). This silently breaks all pixel-to-cell coordinate math at startup.
- `getMouseTracking()` in `useMobileTerminalGestures.ts` reads from a component prop, not from
  `terminal.modes.mouseTrackingMode` (the runtime state set by PTY escape sequences). When Claude
  Code enables `vt200` tracking, the gesture hook's guard reads stale prop data and may behave
  incorrectly.

Three architectural options were evaluated:

**Option A — Single unified `useTerminalGestures` hook with explicit state machine**: Remove both
existing hooks and replace with one hook implementing a five-state machine:
`IDLE → PENDING → SCROLLING | SELECTING | TAPPING`. All touch event listeners are registered
once. The state machine's current state is the single source of truth for which behavior
`touchmove` should exhibit. `SCROLLING` and `SELECTING` are mutually exclusive states — the
conflict is structurally impossible.

**Option B — Coordinator/arbitrator**: Keep both hooks but add a shared ref (or context value)
that each hook checks before acting. When `useMobileTerminalGestures` enters `isSelecting = true`,
it sets the shared ref; `useTouchScroll` checks the ref and skips its handler. The hooks remain
independent files.

**Option C — Remove `useTouchScroll` entirely, extend `useMobileTerminalGestures` to cover
scrolling**: `useTouchScroll` duplicates scroll logic already present in `useMobileTerminalGestures`.
Delete `useTouchScroll.ts` and move its delta calculation into the existing gesture hook's scroll
handling branch, keeping the gesture hook's existing file structure.

Key constraints from research:

- `useTouchScroll` and `useMobileTerminalGestures` are both non-passive and both call
  `event.preventDefault()`. Any architecture that keeps both handlers on the same element requires
  coordination; without it the double-scroll bug is guaranteed on every vertical swipe.
- The five-state machine (IDLE/PENDING/SCROLLING/SELECTING/TAPPING) is the architecture
  recommended in both the architecture research and the features research. It maps directly to the
  four requirements: R4.1 (tap-to-cursor), R4.2 (tap-to-focus), R3.3 (long-press selection),
  R4.3 (deconflict scroll and select).
- The scroll delta strategy in the unified hook must use `lastY` incremental delta per `touchmove`
  event (not cumulative from `touchstart`), matching the behavior users expect from native scroll.
  Cell height is computed via `terminal.element.clientHeight / terminal.rows` (public API).
- iOS Safari's `pointercancel` fires when a scroll gesture is detected after `pointerdown`. Using
  `TouchEvent` rather than `PointerEvent` for the state machine avoids unexpected cancellation of
  the long-press timer during fast vertical swipes; the `touchcancel` event must transition any
  non-IDLE state back to IDLE.
- Multi-touch (second finger down) must cancel gesture recognition and return to IDLE.
- `terminal.modes.mouseTrackingMode` (xterm.js 6 public API) must be the runtime source of truth
  for distinguishing tap-to-focus (mode `'none'`) from tap-to-cursor (mode `'vt200'` or higher).

## Decision

Adopt **Option A**: a single unified `useTerminalGestures` hook with an explicit five-state
machine, replacing both `useTouchScroll.ts` and `useMobileTerminalGestures.ts`.

The hook implements:

```
States: IDLE | PENDING | SCROLLING | SELECTING | TAPPING

Transitions:
  IDLE        → PENDING    on touchstart (exactly 1 finger); start 400 ms long-press timer
  PENDING     → SCROLLING  on touchmove with |dy| > 8 px; cancel timer
  PENDING     → SELECTING  on long-press timer fires (400 ms, no scroll)
  PENDING     → TAPPING    on touchend with |dy| < 8 px within 400 ms; cancel timer
  SCROLLING   → IDLE       on touchend or touchcancel
  SELECTING   → IDLE       on touchend
  TAPPING     → IDLE       immediately after tap action is dispatched
  Any         → IDLE       on touchcancel or second finger down (multi-touch)

Actions per state:
  SCROLLING (touchmove):
    dy = currentY - lastY (update lastY); lastY set on each move event
    cellH = terminal.element.clientHeight / terminal.rows  (public API)
    lines = Math.round(-dy / cellH)
    if lines !== 0: terminal.scrollLines(lines)
    event.preventDefault()

  SELECTING (on enter):
    startCol = Math.floor((touchX - canvasLeft) / cellW)
    startRow = Math.floor((touchY - canvasTop) / cellH)
    terminal.select(startCol, startRow, 0)  -- works regardless of mouseTrackingMode

  SELECTING (touchmove):
    currentCol/Row from touch pixel coordinates
    length = (currentRow - startRow) * terminal.cols + (currentCol - startCol)
    terminal.select(startCol, startRow, Math.max(0, length))
    event.preventDefault()

  TAPPING:
    if terminal.modes.mouseTrackingMode === 'none':
      terminal.focus()  -- show on-screen keyboard (R4.2)
    else:
      col = Math.floor((tapX - canvasLeft) / cellW)
      row = Math.floor((tapY - canvasTop) / cellH)
      terminal.input('\x1b[M' + String.fromCharCode(32, col + 32, row + 32), false)  -- R4.1
```

File placement: `web-app/src/lib/hooks/useTerminalGestures.ts` (new file).
Both `useTouchScroll.ts` and `useMobileTerminalGestures.ts` are deleted.
`XtermTerminal.tsx` replaces the two hook calls at lines 115 and 136 with a single
`useTerminalGestures(containerRef, terminalRef)` call.

Cell dimension calculation throughout uses:
```ts
const cellH = terminal.element.clientHeight / terminal.rows;
const cellW = terminal.element.clientWidth / terminal.cols;
```
This replaces all uses of `terminal._core._renderService.dimensions.css.cell.height` (R3.4, R4.4).

Mouse tracking mode check uses `terminal.modes.mouseTrackingMode` directly (not the prop).

## Consequences

**Positive:**
- The double-scroll conflict is structurally eliminated: `SCROLLING` and `SELECTING` are exclusive
  states; only one `touchmove` handler runs at any time.
- All four gesture behaviors (scroll, long-press select, tap-to-focus, tap-to-cursor) are
  implemented and tested in a single unit with a clear state transition diagram.
- The private `_core._renderService` API is fully removed from the gesture path; the hook uses
  only public xterm.js 6 APIs.
- `terminal.modes.mouseTrackingMode` provides the correct runtime tracking state; the stale-prop
  bug that caused silent selection failure in Claude Code sessions is fixed.
- Single registration point for all touch listeners eliminates listener ordering issues and reduces
  the risk of `{ passive: true }` refactoring accidentally breaking `preventDefault()`.

**Negative / trade-offs:**
- Deleting `useTouchScroll.ts` and `useMobileTerminalGestures.ts` is a breaking change for any
  consumers outside `XtermTerminal.tsx`. A search of the codebase confirms no other consumers
  exist, so this is safe.
- The tap-to-cursor ANSI escape sequence (`\x1b[M<btn><col+32><row+32>`) is X10 mouse encoding.
  It is correct for `vt200` mode but does not cover `drag` or `any` extended tracking modes (which
  use different encodings). For the current implementation, X10 encoding is sufficient; extended
  mode support can be added as a follow-up.
- The `useTerminalGestures` hook requires the terminal to be open (i.e., `terminal.element` must
  be non-null) for cell dimension calculations. The hook must guard against the case where
  `terminal.element` is null (before `open()` is called) and skip gesture handling in that state.
- The 400 ms long-press threshold matches iOS system behavior and is not user-configurable. This
  is intentional; exposing it as a config option adds complexity without clear user benefit.

## Alternatives Considered

**Option B (coordinator/arbitrator)** was rejected because it preserves the fundamental problem: two
independent hooks with incompatible delta-tracking logic operating on the same events. The
arbitrator ref would need to be checked on every `touchmove` event in `useTouchScroll`, adding a
cross-hook dependency that is fragile and hard to test. The architecture research explicitly
identifies a single state machine as the correct solution.

**Option C (remove `useTouchScroll`, extend `useMobileTerminalGestures`)** was rejected because
`useMobileTerminalGestures` already has structural problems: it reads mouse tracking mode from a
stale prop, uses private API for cell height, and does not implement tap-to-cursor. Extending it
to absorb scrolling while fixing all these issues produces a hook that is effectively a rewrite
in place — but without the clarity of an explicit named state machine. The five-state machine
(Option A) makes the allowed state transitions explicit and testable as a unit, which is the
pattern established by ADR-007 (enum-based state transitions) for this codebase.
