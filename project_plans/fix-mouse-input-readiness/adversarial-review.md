# Adversarial Review: fix-mouse-input-readiness Plan

Reviewed against the requirements and the actual source code in:
- `web-app/src/lib/hooks/useTerminalGestures.ts`
- `web-app/src/components/sessions/XtermTerminal.tsx`
- `web-app/src/components/sessions/TerminalOutput.tsx`
- `web-app/src/lib/terminal/mouseTracking.ts`
- `web-app/src/components/providers/ViewportProvider.tsx`

---

## BLOCKER — Epic 2: SGR heuristic conflates tracking mode with wire encoding

**Story 2.1** says: "if `mouseTrackingMode === 'x10'` use X10 encoding; else use SGR encoding."

`mouseTrackingMode` describes *which mouse events* the terminal reports (x10/vt200/drag/any — set by DECSET ?9/?1000/?1002/?1003). The wire *encoding format* (X10 vs SGR vs UTF-8) is a separate, orthogonal control:
- SGR encoding is enabled by a distinct DECSET: `?1006h`.
- `vt200`, `drag`, and `any` modes all default to **X10 encoding** unless the program also sends `?1006h`.

The plan unconditionally sends SGR bytes for any non-x10 `mouseTrackingMode`. A program that sets `?1000h` (vt200) without `?1006h` — which is common in older terminal applications — expects X10-format bytes `\x1b[Mbxy` but would receive `\x1b[<0;col;rowM`. Most programs would silently misparse or ignore it; some could corrupt their internal state.

**What's needed:** check a second mode flag for the SGR extension. xterm.js does not expose `?1006` state in its public `terminal.modes` API, so the correct fallback is: use X10 encoding for `x10` mode; use X10 encoding for all other modes *unless* there is a reliable way to confirm `?1006h` is also active. The plan needs to acknowledge this limitation and document the chosen heuristic explicitly. Sending X10 for all modes is safe (overly conservative but never harmful); sending SGR for `vt200` without confirmation of `?1006` is not.

---

## BLOCKER — Epic 4: write mechanism is unspecified and the wrong channel will break things

Story 4.1 says "write to terminal: `'\x1b[?1003l...'`" with no specification of which API performs the write.

There are two entirely different channels:
1. `terminal.write(seq)` — processes `seq` as PTY *output* (xterm's VT parser receives it, updates internal mode flags). This is the correct path for DEC private mode resets.
2. `onSendData(seq)` — sends `seq` to the PTY as *input* (keyboard data). A developer reading "write to terminal" inside the context of `useTerminalGestures` (which only has `onSendData`) would naturally reach for `onSendData`. That would forward `\x1b[?1003l...` to the running program as raw keyboard bytes — garbage input that could corrupt vim, less, or any interactive program.

Additionally, `useTerminalGestures` does not have a `terminal.write()` path at all. The write must live in `TerminalOutput.tsx` where `xtermRef` (the `XtermTerminalHandle`) is available:

```typescript
useEffect(() => {
  xtermRef.current?.write('\x1b[?1003l\x1b[?1002l\x1b[?1001l\x1b[?1000l\x1b[?9l');
}, [connectionEpoch]); // xtermRef is stable; omit from deps
```

The plan must be explicit: this write goes in `TerminalOutput`, calls `xtermRef.current?.write()`, and must NOT go through `onSendData`. As written, the story leaves the wrong channel as the most obvious implementation choice.

---

## CONCERN — Epic 3: lazy initializer introduces a desktop flash due to `ViewportProvider`'s default

`ViewportProvider` initializes its context state with `isMobile: true` (line 29 of `ViewportProvider.tsx`). This is the defensive SSR default used before `window.innerWidth` can be measured.

The plan's fix:
```typescript
useState<'none' | 'any'>(() => isMobile ? 'none' : 'any')
```

Because `isMobile` starts as `true`, this lazy initializer always evaluates to `'none'` on first render — for both mobile and desktop. For desktop users, `mouseMode` is then `'none'` until the viewport `useEffect` fires (within one RAF) and sets it to `'any'`. 

The current code (`useState('any')` + a correction useEffect) is actually better for desktop: it starts correct for desktop and only has a brief wrong state for mobile. The lazy initializer inverts this: it starts correct for mobile but briefly wrong for desktop.

Since the plan says "keep the useEffect for reactivity," the useEffect still corrects desktop to `'any'` quickly. The net effect is that the plan's Epic 3 provides no improvement over the status quo for the initial-state flash on either platform — both approaches have a one-frame wrong initial value for one device class. The fix should either accept this or restructure: pass `isMobile` as a server-side hint, or initialize based on `window.innerWidth` directly inside the lazy initializer (with an SSR guard).

This is a concern, not a blocker: the flash is sub-frame and the behavior converges quickly. But the plan's stated benefit ("lazy initializer solves the initial-value problem") is not achieved given how `ViewportProvider` works.

---

## CONCERN — Epic 4 scope: the reset sends to xterm locally but the server PTY still has tracking enabled

After `terminal.write(resetSeq)`, xterm's internal `mouseTrackingMode` becomes `'none'`. `isMouseTracking()` returns `false`. The gesture hook's next tap would fire `t.focus()` instead of encoding a click — correct.

But the PTY server side still has mouse tracking enabled until the reconnected session's capture-pane snapshot arrives and re-enables it via the stream. During this window (before the snapshot), mobile users get the correct behavior (no bytes forwarded) but any desktop user or re-connected session will have a stale xterm mode that disagrees with the server. This is probably acceptable for the reconnect use case, but should be explicitly acknowledged: the reset is a client-only transition pending server re-synchronization. A comment in the code would prevent future confusion.

---

## CONCERN — AC5 "No regression on desktop" has no dedicated test path

The plan's test list for Story 1.3 covers:
- `mouseModeOverride:'none'` + `isMouseTracking: true` → only focus, no `onSendData`
- `mouseModeOverride:'any'` + `isMouseTracking: true` → `onSendData` called
- default (`undefined`) → backward compatible
- toggle propagates immediately

None of these test desktop behavior end-to-end through the TerminalOutput → XtermTerminal → `useTerminalGestures` prop chain. The unit tests in isolation are fine, but a missing-prop regression (e.g., `scrollGestureProps` useMemo forgetting to include `mouseModeOverride`) would not be caught. Adding an integration-level test (or checking the existing `TerminalOutput.test.tsx` wiring) for the desktop case would close this gap.

---

## MINOR — Story 1.1/1.2: `mouseModeOverride` type should be documented as optional with explicit semantics

The plan adds `mouseModeOverride?: 'none' | 'any'` with "default (undefined) → backward compatible ('any')". The implementation reads `optionsRef.current.mouseModeOverride !== 'none'`, which correctly treats `undefined` as "forward clicks." This is fine, but the GestureOptions JSDoc should state the default explicitly so future callers don't have to read the implementation to understand that `undefined` means clicks are forwarded.

---

## MINOR — Story 2.1: X10 column/row clamping not needed for SGR encoding

X10 encoding requires coordinates clamped to 1–223 because `charCode(col + 32)` must stay in valid range. SGR format `\x1b[<0;col;rowM` encodes coordinates as decimal integers with no such constraint. Removing the clamp in the SGR branch is not mentioned, which could leave dead clamping code in place. Minor, but worth calling out for code clarity.

---

---

## Resolution Status (updated post-Phase 3 plan revision)

**BLOCKER #1 (Epic 2 SGR heuristic)** → RESOLVED. plan.md revised: Epic 2 keeps X10 encoding as-is for all modes. Reasoning: `?1006h` SGR extension is not exposed on xterm.js public `modes` surface, so the correct behavior is X10 for all modes (safe, never harmful), with a code comment documenting the limitation. SGR detection deferred to a follow-up backlog item.

**BLOCKER #2 (Epic 4 write mechanism)** → RESOLVED. plan.md revised: Story 4.1 now explicitly specifies `xtermRef.current?.terminal?.write(MOUSE_TRACKING_DISABLE)` in `TerminalOutput.tsx`, with a critical API note distinguishing `write()` (feeds xterm VT parser as PTY output) from `onSendData()` (routes to PTY stdin — would corrupt running programs).

**CONCERN (Epic 3 lazy initializer)** → RESOLVED. Story 3.1 accepted as no-op: lazy initializer would cause desktop regression due to `ViewportProvider` defaulting `isMobile: true`. The existing two-step init is kept as-is.

**CONCERN (AC5 desktop test path)** → Noted in validation.md: integration test AC6-a added for desktop mouseMode default; prop-threading test AC7-b covers the TerminalOutput→XtermTerminal→useTerminalGestures chain.

VERDICT: CLEAN (all blockers resolved in plan.md)
