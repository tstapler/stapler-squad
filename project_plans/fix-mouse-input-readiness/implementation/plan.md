# Implementation Plan: fix-mouse-input-readiness

**Complexity:** 2 (focused frontend fix, 3 files, no new modules)  
**Disposition:** Extend-as-is — the `GestureOptions`/`ScrollGestureProps` seam was built for exactly this extension.

---

## Problem Summary

On mobile, tapping a terminal that has a stale `mouseTrackingMode !== "none"` (left by a previous program) sends raw X10 mouse escape sequences to the current PTY program, which can't handle them and prints garbled output.

Three sub-bugs:
1. `handleTap()` in `useTerminalGestures.ts` sends X10 mouse bytes whenever `isMouseTracking()` is true, regardless of user's `mouseMode` preference
2. `mouseMode` state in `TerminalOutput.tsx` is never threaded to the gesture hook — the "Mouse" toggle is cosmetic only
3. X10 encoding is always used; PTY programs expecting SGR encoding (`?1006h`) receive malformed events

Secondary issue: stale `mouseTrackingMode` persists after reconnect, keeping the gate incorrectly open.

---

## Epic 1 — Wire `mouseMode` Toggle to Gate Mouse Click Forwarding

### Story 1.1 — Add `mouseModeOverride` to `GestureOptions`

**File:** `web-app/src/lib/hooks/useTerminalGestures.ts`  
**Line range:** ~68–106 (`GestureOptions` interface)

Add one field to `GestureOptions`:
```ts
/**
 * When 'none', handleTap() will not send mouse click bytes to the PTY even if
 * isMouseTracking() is true. Defaults to 'any' (forward clicks when tracking is on).
 * Read via optionsRef.current to avoid stale closures.
 */
mouseModeOverride?: 'none' | 'any';
```

**No tests needed** for this task (interface-only change; covered by Story 1.3's tests).

### Story 1.2 — Thread `mouseModeOverride` through the prop chain

**Three touchpoints, all additive:**

**Task 1.2a — `ScrollGestureProps` in `XtermTerminal.tsx`** (~line 153):
```ts
// In ScrollGestureProps interface:
mouseModeOverride?: 'none' | 'any';
```

**Task 1.2b — `useTerminalGestures` call in `XtermTerminal.tsx`** (~line 353–368):
```ts
useTerminalGestures({
  // ... existing fields ...
  mouseModeOverride: scrollGesture?.mouseModeOverride,
});
```

**Task 1.2c — `scrollGestureProps` useMemo in `TerminalOutput.tsx`** (~line 1099–1112):
```ts
const scrollGestureProps = useMemo<ScrollGestureProps>(
  () => ({
    // ... existing fields ...
    mouseModeOverride: mouseMode,  // ADD THIS
  }),
  [/* ... existing deps ... */, mouseMode],  // ADD mouseMode to deps
);
```

### Story 1.3 — Gate `handleTap()` behind `mouseModeOverride`

**File:** `web-app/src/lib/hooks/useTerminalGestures.ts`  
**Location:** `handleTap()` function (~line 628–645)

**Current code:**
```ts
} else if (t) {
  if (!isMouseTracking()) {
    t.focus();
  } else if (t.element) {
    // ... send X10 press/release
    onSendData(press + release);
    t.focus();
  }
}
```

**New code:**
```ts
} else if (t) {
  // Gate: only forward mouse clicks when BOTH the PTY has enabled tracking AND
  // the user has not opted out via the "Mouse" toolbar toggle.
  const forwardMouseClicks = isMouseTracking() && optionsRef.current.mouseModeOverride !== 'none';
  if (!forwardMouseClicks) {
    t.focus();
  } else if (t.element) {
    // ... send mouse bytes (see Story 1.4 for encoding fix)
    onSendData(press + release);
    t.focus();
  }
}
```

**Critical implementation note:** The value is read as `optionsRef.current.mouseModeOverride` — NOT captured from the enclosing effect closure. The gesture `useEffect` deps include only `[containerRef, terminalRef, gestureScrollEnabled]`; `mouseModeOverride` is not a dep. If captured directly from the closure, it freezes at mount value and the toggle silently breaks. This is the same pattern used by `onSendData` (line 138) and `isInputBusy`.

**Tests to add** in `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` (or existing test file):
- When `mouseModeOverride: 'none'` and `isMouseTracking()` is true, a tap calls `t.focus()` only — no `onSendData` call
- When `mouseModeOverride: 'any'` and `isMouseTracking()` is true, a tap calls `onSendData` with mouse bytes
- When `mouseModeOverride` is undefined (default), behavior is backward-compatible (`'any'`)
- Toggle from `'none'` → `'any'` mid-session propagates immediately (not stale)

---

## Epic 2 — Encoding: Keep X10 As-Is (Defer SGR Fix)

### Story 2.1 — Document encoding limitation; no code change in this PR

**Background from adversarial review:**  
`mouseTrackingMode` (x10/vt200/drag/any) and the wire encoding (X10 vs SGR) are two independent axes. The SGR encoding extension (`?1006h`) is a separate DECSET flag that xterm.js does not expose on its public `modes` surface. The naive heuristic "use SGR when mode != x10" is **incorrect** — `vt200`/`drag`/`any` modes frequently default to X10 encoding without `?1006h` active (many older terminal apps).

**Decision:** Keep X10 encoding as-is in `handleTap()` for this PR. The current X10 behavior is:
- Correct for apps using `?9h` (x10 mode)  
- Correct for apps using `?1000h`/`?1002h`/`?1003h` WITHOUT `?1006h` (X10 encoding default)
- Incorrect only for apps using `?1006h` (SGR extension) — this was pre-existing before this fix

**Action:** Add a code comment at the X10 send site in `handleTap()` noting the known limitation:
```ts
// Encoding: X10 format (\x1b[M...). This is correct for ?9h, ?1000h, ?1002h, ?1003h
// without the ?1006h SGR extension. Apps that set ?1006h receive X10 format instead
// of SGR — a pre-existing limitation. Fixing requires detecting ?1006h from xterm
// internals (not on the public modes surface); tracked as a separate follow-up.
```

**Follow-up:** File a separate backlog item for SGR encoding detection using xterm.js internals (`(terminal as any)._core._inputHandler._mouseService?.isSGREnabled`).

**No tests needed** for this story (no code change).

---

## Epic 3 — Fix Mobile `mouseMode` Initial Value

### Story 3.1 — No change needed (adversarial review finding)

**Adversarial review finding:** `ViewportProvider` defaults `isMobile: true` before measuring `window.innerWidth`. A lazy initializer `() => isMobile ? 'none' : 'any'` would **always** start as `'none'` — including on desktop — because `isMobile` is `true` at first render. This would be a desktop regression.

**Decision:** Leave the initial value as `useState<'none' | 'any'>('any')`. The existing `useEffect` that corrects it to `'none'` on mobile runs synchronously after the first commit and before any user interaction is possible — the one-render window is not exploitable in practice (the terminal is not interactable until layout completes). No code change.

**Epic 3 is a no-op.** Remove from task list.

---

## Epic 4 — Reset Stale Mouse Tracking on Reconnect

### Story 4.1 — Disable mouse tracking sequences on epoch bump

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`  
**Location:** The `useEffect` that watches `connectionEpoch` (~line 1153–1158)

When `connectionEpoch` changes (reconnect or full snapshot), write mouse-tracking-disable sequences **to the terminal via xterm's `write()` API** so `isMouseTracking()` returns false until the new PTY session re-enables it:

```ts
// In TerminalOutput.tsx, in the useEffect that watches connectionEpoch:
useEffect(() => {
  if (lastEpochRef.current === connectionEpoch) return;
  lastEpochRef.current = connectionEpoch;
  netPagesUp.invalidate("reconnect");
  // Reset stale mouse tracking mode: these are written to xterm's VT parser,
  // NOT sent to the PTY as input. xterm updates terminal.modes.mouseTrackingMode
  // to "none" which makes isMouseTracking() return false until the new PTY session
  // re-enables mouse tracking via its own escape sequences.
  const MOUSE_TRACKING_DISABLE = '\x1b[?1003l\x1b[?1002l\x1b[?1001l\x1b[?1000l\x1b[?9l';
  xtermRef.current?.terminal?.write(MOUSE_TRACKING_DISABLE);
}, [connectionEpoch, netPagesUp]);
```

**CRITICAL API note (from adversarial review):** Use `xtermRef.current?.terminal?.write()`, NOT `onSendData()` or `deliverTerminalData()`. The `write()` API feeds bytes to xterm's VT parser as if they came from the PTY — xterm interprets them and updates `terminal.modes`. Sending via `onSendData` would route them to PTY *input* (stdin of the running program), which would corrupt vim, less, etc.

**How to verify the path:**
- `xtermRef` is the `XtermTerminalHandle` ref in `TerminalOutput.tsx`
- `xtermRef.current.terminal` is the raw `xterm.Terminal` instance (exposed via `XtermTerminalHandle`)
- `terminal.write()` is the standard xterm.js API for writing PTY output bytes

The sequences:
- `?1003l` — disable any-event tracking
- `?1002l` — disable button+drag tracking
- `?1001l` — disable highlight tracking
- `?1000l` — disable basic (X10-compat) tracking
- `?9l` — disable X10 mouse reporting

**Tests:**
- After `connectionEpoch` bumps, `isMouseTracking(terminal)` returns false even if it was true before the epoch change

---

## Epic 5 — Suppress xterm.js Native Mouse Events When mouseMode is 'none' (AC#5)

### Story 5.1 — Suppress native xterm pointer events on desktop when mouseMode='none'

**Context (pre-mortem P1 #1 resolution):**  
On mobile, xterm.js's native pointer handlers never fire because the gesture hook calls `preventDefault()` on every touchstart — so AC#5 is already satisfied for mobile. On desktop, xterm.js does handle native pointer events, and `mouseMode` defaults to `'any'` so suppression is only needed when the user explicitly toggles "Mouse OFF" (an intentional desktop action).

**Approach:** Set `terminal.options.mouseEnabled = false` when `mouseMode === 'none'` and `true` when `mouseMode === 'any'`. xterm.js 6.x exposes runtime-settable `options` on the `Terminal` instance; setting `mouseEnabled` to `false` prevents xterm from encoding pointer events into `terminal.onData`.

**Note on API availability:** If `terminal.options.mouseEnabled` is not present in xterm.js v6, fall back to inserting a guard in the `onData` disposable in `XtermTerminal.tsx` that drops bytes matching `/^\x1b\[</` (SGR mouse) or `/^\x1b\[M/` (X10 mouse) when `mouseMode === 'none'`. The `onData` disposable is at `XtermTerminal.tsx:891`; use a `mouseEnabledRef` (updated via `useEffect` watching the `mouseMode` prop) to avoid stale closure.

**File:** `web-app/src/components/sessions/TerminalOutput.tsx` and/or `web-app/src/components/sessions/XtermTerminal.tsx`

**Tests:**
- Desktop: `terminal.options.mouseEnabled` is `false` when `mouseModeOverride === 'none'` and `true` when `'any'`
- Desktop: `onData` emits no mouse bytes when mouseMode='none' (regardless of mouseTrackingMode)

### Story 5.2 — Clarify mobile default: mouseMode auto-set to 'none' on mount

**Context (pre-mortem P1 #2 resolution):**  
`TerminalOutput.tsx` already contains:
```ts
const [mouseMode, setMouseMode] = useState<'none' | 'any'>('any');
useEffect(() => { setMouseMode(isMobile ? 'none' : 'any'); }, [isMobile]);
```
The initial value `'any'` is corrected to `'none'` on mobile synchronously after mount (before any user interaction is possible — the terminal is not interactable during layout). **No code change is needed.** The "user never touches the toolbar" scenario IS protected on mobile from the first render cycle.

**Action:** Add a test in `TerminalOutput.toolbarKeys.test.tsx` that renders `TerminalOutput` with `isMobile=true` without clicking the Mouse button, and verifies `scrollGesture.mouseModeOverride === 'none'` in the first rendered `scrollGestureProps` object passed to the XtermTerminal mock — confirming the auto-default is wired through.

---

## Non-Goals / Explicit Exclusions

- No changes to the PTY/server-side (Go) code — this is frontend-only
- Not sending PTY input escape sequences to force-disable mouse tracking in the running program
- Not filtering generic terminal data (only the specific mouse event sequences when mouseMode='none', per Story 5.1)
- Not fixing the `enterSelecting()` / long-press selection path — pitfalls research confirmed it has no regression risk from this fix

---

## Tech Debt Disposition

**Extend-as-is** — The `GestureOptions`/`ScrollGestureProps` seam was designed for live-option threading (the `optionsRef` pattern, `isInputBusy` precedent). Adding `mouseModeOverride` is a clean extension with no architectural debt introduced or deepened.

The X10 encoding fix (Epic 2) pays down pre-existing debt: `handleTap()` has always sent the wrong encoding for SGR-mode apps; this was never caught because the overall gate was broken. Fixing both together is correct.

---

## Unresolved Questions

- Should `mouseMode` persist to `localStorage` across sessions (like `scrollOverride` does)? Not in scope for this fix — but worth filing as a follow-up.
- The stale-tracking reset (Epic 4) uses `terminal.write()` from `TerminalOutput.tsx`. Confirm that `xtermRef.current?.write()` is available at the point where `connectionEpoch` changes are detected.

---

## Risk Controls

| Risk | Mitigation |
|---|---|
| Stale closure freezes toggle | Read `optionsRef.current.mouseModeOverride` in `handleTap()`; never capture directly |
| X10 sent when PTY expects SGR | Epic 2 keeps X10 as-is (SGR extension `?1006h` not on public xterm API); code comment documents limitation; follow-up filed for SGR detection |
| Desktop regression | `mouseMode` defaults to `'any'` on desktop; the gate `!== 'none'` passes, no change |
| Reconnect stale mode | Epic 4 writes disable sequences on epoch bump |
| Mobile initial value window | Existing `useEffect` auto-sets to `'none'` on mobile; one-render window not exploitable (terminal not interactable during layout); no code change needed (Epic 3 no-op) |

---

## Implementation Deviations (recorded at sdd:6-verify)

- **Epic 2 reversed:** SGR encoding implemented. `?1006h/l` is tracked through the public `terminal.parser.registerCsiHandler` API (not `_core`); the flag resets on RIS, DECSTR and new-Terminal creation. Taps send SGR when set, X10 otherwise.
- **Story 5.1:** native suppression uses the planned `onData` fallback filter (xterm 6 has no `mouseEnabled`); whole-chunk match of one or more SGR/X10 reports.
- **Epic 4 skipped:** the post-snapshot disable write could wipe tracking a running vim/tmux legitimately re-enabled. Stale tracking with Mouse='any' remains; follow-up.
- **Story 5.2 test not added.** Open: the toggle only gates the tap path; double-tap/long-press/scroll still use `isMouseTracking()`.
