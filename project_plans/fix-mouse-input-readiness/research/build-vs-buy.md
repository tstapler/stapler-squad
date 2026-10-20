# Build vs Buy: Mouse Event Forwarding Gate

**Date:** 2026-10-19  
**xterm.js version:** `@xterm/xterm ^6.0.0`

---

## 1. xterm.js 6.x API for Mouse Event Interception

**Verdict: no public API exists for this.**

xterm.js 6.x provides:
- `attachCustomKeyEventHandler(handler)` — keyboard input only; no mouse equivalent.
- `onData` — fires for user keyboard input AND for mouse sequences the terminal encodes (when mouse tracking is active and a real pointer event triggers xterm's internal `CoreMouseService`). However, this callback receives all terminal input indiscriminately — there is no filter, interceptor, or cancellation hook.
- `terminal.modes.mouseTrackingMode` — read-only property reflecting what the PTY enabled via escape sequence. There is no setter; you cannot disable mouse tracking at the xterm.js level without sending PTY escape sequences (`\x1b[?1000l` etc.) that would confuse the running program.
- No `customEventDecoder`, `interceptData`, or `InputDecoder` hook exists in any xterm.js 6.x release. Searching `web-app/src/` found zero references to any such API.

**Confirmed:** the codebase already uses `terminal.modes.mouseTrackingMode` via `readMouseTrackingMode()` in `web-app/src/lib/terminal/mouseTracking.ts:8`. This is the only supported read path.

---

## 2. Custom Event Decoder / onData Filter Search

`grep` over `web-app/src/` for `customEvent`, `onData.*filter`, `interceptData`, `InputDecoder` returned no results — neither the codebase nor the xterm.js library surface any such hook. The approach does not exist.

Wrapping `onData` to filter out mouse escape sequences (e.g. regex matching `\x1b[M...`, `\x1b[<...`, etc.) is technically possible but brittle: the regex must cover all five xterm mouse encoding variants (X10, UTF-8, SGR, URXVT, SGR-Pixels), must handle partial sequences buffered across writes, and would silently swallow data the PTY intended. Not recommended.

---

## 3. Existing OSS Patterns (ttyd, wetty, GoTTY)

Known patterns from other web terminals:
- **ttyd/wetty/GoTTY** handle this the same way: they do not intercept at the xterm.js level. Instead, they gate mouse forwarding in their own touch/pointer event wrappers before data reaches xterm. The pattern is universally: check user preference first, then check `terminal.modes.mouseTrackingMode`, and only encode + send if both say "forward."
- None of them attempt to wrap or replace xterm's internal mouse encoder. They all own the touch-event path and skip the xterm native path on mobile.

---

## 4. The Simple Approach — Is It Correct and Complete?

**Yes, for mobile. Here's the exact gap and the two-line fix:**

### The gap

`mouseMode` is live state in `TerminalOutput.tsx` (line 391):

```tsx
const [mouseMode, setMouseMode] = useState<'none' | 'any'>('any');
// ... defaults to 'none' on mobile (line 394)
```

It is **never passed to `useTerminalGestures`**. The `GestureOptions` interface (`useTerminalGestures.ts:68`) has no `mouseModeOverride` field. The `ScrollGestureProps` interface forwarded through `XtermTerminal` (`XtermTerminal.tsx:154`) has no such field either.

`handleTap()` at line 630 only checks `isMouseTracking()` (xterm's internal mode). When `mouseMode === 'none'` but the PTY previously set `mouseTrackingMode !== 'none'`, taps still encode and send X10 mouse bytes.

### The forwarding paths

On mobile, touch events are **exclusively owned** by `useTerminalGestures` — the hook calls `preventDefault()` on touchstart, so xterm's own DOM pointer handlers never fire. The only forwarding path for touch-generated mouse events is `handleTap()` → `onSendData(press + release)` at line 643.

On desktop, `mouseMode` defaults to `'any'`, so gating it would change nothing.

This means: for the mobile stale-mouse-mode problem, **gating `handleTap()` alone is complete** — there is no second path to guard.

### The fix

1. Add `mouseModeOverride?: 'none' | 'any'` to `GestureOptions` (line 68, `useTerminalGestures.ts`)
2. In `handleTap()` at line 630, change:
   ```ts
   } else if (t) {
     if (!isMouseTracking()) {
       t.focus();
     } else if (t.element) {
   ```
   to:
   ```ts
   } else if (t) {
     const forwardMouse = isMouseTracking() && opts.mouseModeOverride !== 'none';
     if (!forwardMouse) {
       t.focus();
     } else if (t.element) {
   ```
3. Thread `mouseModeOverride: mouseMode` through `ScrollGestureProps` in `XtermTerminal.tsx` and the call site in `TerminalOutput.tsx` (line ~1108).

No xterm.js internals are touched. No fragile escape-sequence filtering. No second forwarding path exists on mobile.

---

## 5. onData Wrapping Feasibility (XtermTerminal.tsx line 891)

```tsx
const dataDisposable = terminal.onData((data) => {
  onDataRef.current?.(data);
});
```

**Not needed for the mobile fix.** On mobile, xterm's own native pointer handlers never fire (gesture hook owns `preventDefault`), so no mouse sequences arrive through `onData` from xterm's internal encoder. A wrapper on `onData` would only be relevant if someone used a real pointing device on a mobile device — unlikely — and even then, the `mouseMode` check at the xterm-native level would require parsing escape sequences.

The `onProgrammaticData` / `onData` split at `XtermTerminal.tsx:169–172` already correctly separates gesture-generated bytes from user keystrokes. The proposed fix writes through `onSendData` → `onProgrammaticData`, so filtering at `onData` would have no effect on the gesture path anyway.

---

## Summary

- **xterm.js 6.x has no API** to intercept, cancel, or override mouse event forwarding — no `customEventDecoder`, no `onData` filter, no mouse equivalent of `attachCustomKeyEventHandler`.
- **The gap is a missing wire**: `mouseMode` state in `TerminalOutput.tsx` is never passed to `useTerminalGestures`; the fix is adding `mouseModeOverride` to `GestureOptions` and checking it in `handleTap()` before sending X10 bytes.
- **The fix is complete for mobile**: on mobile, `useTerminalGestures`'s `preventDefault()` ownership of touch events means `handleTap()` is the only forwarding path — no second path, no `onData` wrapping needed.
