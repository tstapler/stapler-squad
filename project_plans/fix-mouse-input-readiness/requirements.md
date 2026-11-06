# Requirements: fix-mouse-input-readiness

## Problem Statement

When a mobile user taps the terminal while the "Mouse" button is ON (or when the PTY's
mouse tracking mode is non-"none" due to a previous program not cleaning up), raw X10 and/or
SGR mouse escape sequences are forwarded to the running PTY program even when that program
has not requested mouse tracking and cannot handle those bytes. The result is garbled output:
the program treats the mouse escape sequences as literal keyboard input.

The screenshot (backlog attachment) shows `gh auth login` receiving:
- Cursor-position report responses (`^[[5;1R`) and OSC color responses (`^[]11;rgb:1e1e1e/...`)
- X10 mouse press/release pairs (`[MR5[M#R5`)
- SGR mouse events (`[<35;51;21M[<0;51;21M[<0;51;21m`)

All appearing on the `execute:` prompt line as if the user had typed them.

## Root Cause (Diagnosed)

**Location 1 — `web-app/src/lib/hooks/useTerminalGestures.ts`, `handleTap()` (~line 609)**

When `isMouseTracking()` is `true` (xterm's `terminal.modes.mouseTrackingMode !== "none"`),
`handleTap()` sends a hard-coded X10 click+release pair to `onSendData`:

```ts
const press   = `\x1b[M${String.fromCharCode(32, clampedCol + 32, clampedRow + 32)}`;
const release = `\x1b[M${String.fromCharCode(35, clampedCol + 32, clampedRow + 32)}`;
onSendData(press + release);
```

`isMouseTracking()` is stale — it reflects whether any program in the session's
history requested mouse tracking, not whether the *current foreground program* did.
When a program like `vim` exits without sending `\e[?1000l` (mouse tracking off), the
next foreground program (`gh auth login`, a shell prompt, etc.) inherits the "tracking on"
state and receives click events it didn't request and can't handle.

**Location 2 — xterm.js native mouse-event generation**

When `terminal.modes.mouseTrackingMode !== "none"`, xterm.js natively captures pointer
events on `.xterm-screen` and emits them via `terminal.onData`, independently of the
gesture hook. This generates additional SGR mouse events (`[<...M`) on top of the X10
events from `handleTap()`.

**Location 3 — `mouseMode` toggle is wired to nothing**

`TerminalOutput.tsx` has `const [mouseMode, setMouseMode] = useState<'none' | 'any'>('any')`
and a toolbar "Mouse" button that toggles it. This state is **never passed to
`useTerminalGestures`** or to xterm.js, so the button changes label text only — it does not
actually suppress or enable mouse event forwarding.

## Acceptance Criteria

1. A tap on a mobile terminal whose PTY-reported `mouseTrackingMode` is `"none"` MUST NOT
   send any mouse escape sequences to the PTY. Only `terminal.focus()` should be called.

2. A tap on a mobile terminal whose PTY-reported `mouseTrackingMode` is non-`"none"` and
   where the user's `mouseMode` toolbar toggle is `'any'` (Mouse ON) MUST send the
   appropriate mouse click sequence in the encoding the PTY requested (X10 for `x10` mode,
   SGR for `vt200`/`any`/`drag` modes), not a hard-coded X10 sequence unconditionally.

3. When the user's `mouseMode` toolbar toggle is `'none'` (Mouse OFF), a tap MUST NOT send
   any mouse escape sequences to the PTY, even if `terminal.modes.mouseTrackingMode` is
   currently non-`"none"`. Only `terminal.focus()` should be called.

4. The "Mouse" toolbar button in `TerminalOutput.tsx` MUST be wired to actually gate mouse
   event forwarding: toggling it off stops mouse events; toggling it on restores them.

5. xterm.js's native mouse-event generation (SGR/X10 via `onData`) MUST also be suppressed
   when `mouseMode === 'none'`, so both forwarding paths (gesture hook + xterm native) are
   blocked consistently.

6. No regression in desktop mouse tracking behavior: on desktop, mouse tracking continues
   to work as before (xterm.js natively handles it; the gesture hook does not interfere).

7. Existing tests in `__tests__/XtermTerminal.test.tsx` and `useTerminalGestures` tests
   pass; new unit tests cover the gated-tap behavior.

## Out of Scope

- Automatic detection of stale mouse-tracking state (a program exiting without
  sending mouse-off sequences is a known terminal hygiene problem; an explicit
  user toggle is the correct affordance for now).
- Sending PTY escape sequences to forcibly enable/disable mouse tracking in the PTY
  (the PTY owns its own mode; we only control what we forward).
- Any server-side (Go) changes — this is a frontend-only fix.
