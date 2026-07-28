# UX Research — terminal-resize-fit-loop

## Scope note

This fix (AC1-3, 5-7) has **no user-facing surface**: it changes internal gating logic
in a `ResizeObserver` callback (`XtermTerminal.tsx` ~L449-506) and a throttled RPC
dispatcher (`useTerminalFlowControl.ts` `resize()`, ~L403-436). Both are pure
size-comparison / debounce / logging code today — no DOM writes, no ARIA, no focus
management. The only user-observable delta anywhere in this project is **AC4: the
WebGL→canvas renderer fallback triggered by the oscillation detector**. This document
focuses there, and states the "N/A" conclusion explicitly for everything else.

## 1. Is a WebGL→canvas renderer swap visually perceptible?

Reasoning from the existing code path (`XtermTerminal.tsx` L263-281), which already
does the WebGL→canvas transition once, on `onContextLoss`:

```ts
webglAddon.onContextLoss(() => {
  console.warn('[XtermTerminal] WebGL context lost, falling back to canvas renderer');
  webglAddon.dispose();
});
```

Calling `dispose()` on the WebGl addon unregisters its render callback and detaches its
canvas layer; xterm.js's core renderer (DOM/canvas fallback) then takes over on the
next render pass, using the terminal's already-populated `Buffer` (the in-memory grid of
cells) as the source of truth — not the pixels that were on screen. Because xterm.js
separates model (buffer) from view (renderer), the content is redrawn from that model,
not decoded from the old canvas. Practically this means:

- **No data loss and no visible "flash to blank."** The next render immediately repaints
  the same rows/cols from the buffer.
- **A one-frame redraw is technically possible** (renderer swap happens between two
  animation frames) but at 60fps this is sub-perceptible for a static/mostly-static
  terminal, which is the actual trigger condition here — AC4 fires when cols/rows have
  been *oscillating without net change* for 3+ repeats in 2s, i.e. precisely when nothing
  is visually changing.
- **Font rendering may differ subtly** — WebGL glyph rendering (texture-atlas based) vs.
  canvas glyph rendering (per-cell `fillText`) can have marginally different subpixel
  antialiasing. This is the kind of difference a user would only notice in an A/B
  screenshot diff, not in normal use, and is already a live risk today via the existing
  `onContextLoss` path (i.e. not a new risk introduced by this fix).
- **Performance change is real but not "perceptible" in the UX sense**: canvas is
  slower for large scrollback repaints, but the fallback only engages after oscillation
  is already pegging CPU — canvas is strictly better than the spin loop it replaces.

No xterm.js community reports were reachable to corroborate beyond the addon's own
documented `dispose()`/`onContextLoss()` contract, so this conclusion is reasoned from
that contract plus the buffer/renderer separation already relied upon by the existing
context-loss handler in this codebase — not from an external report.

## 2. Should the fallback be silent or surfaced (toast/status indicator)?

**Recommendation: silent — console.warn only, matching `onContextLoss` exactly.**

Evidence from grepping `XtermTerminal.tsx` for every `console.warn`/`console.error`/
toast call (18 sites total):

| Pattern | Count | Surfaces to user? |
|---|---|---|
| `console.warn` / `console.error` / `console.log` (SSR guard, WebGL load failure, WebGL context loss, cell-dimension mismatch, terminal init failure, etc.) | 15 sites | **No** — console only |
| `showToast()` / `showToastInHandler()` | 1 mechanism, called from copy/paste success/failure paths only | **Yes** — visible toast, `aria-live="polite"` (L652) |

The toast component (`toastRef`, L109, L137-150) exists **exclusively** for clipboard
copy feedback ("Copied" / "Copy failed") — a direct result of a user-initiated action
(pressing Ctrl+C / right-click copy) where the user needs confirmation the action
succeeded. It is never used for background/infrastructure events. Every other
warning-or-worse condition in this file, including the *existing* WebGL context-loss
fallback at L269-272 (functionally identical to what AC4 proposes — same
`dispose()` call, same renderer downgrade), is console-only.

Adding a toast for the oscillation-triggered fallback would break this precedent and
would be actively worse UX: it fires during a moment the user is fighting with an
unresponsive/janky terminal (mid-resize, tab-switch, or window-drag), which is exactly
when an interrupting toast is least welcome and least actionable. The user cannot do
anything with "renderer switched to canvas" — there's no decision or recovery step for
them to take. Per Nielsen's "visibility of system status" heuristic, status should be
surfaced when it helps the user predict outcomes or take action; here it does neither.

**Action**: log via `console.warn('[XtermTerminal] Oscillation detected (cols/rows
repeated Nx in Nms), falling back to canvas renderer')` from the same call site pattern
as `onContextLoss`, and record the decision + rationale in the AC4 ADR. Do not add a
toast, banner, or status-bar indicator.

(Optional, not required for AC4: if there's ever a debug/diagnostics panel in this app,
the fallback event would be a reasonable thing to expose there — but that's future
scope, not part of this bug fix.)

## 3. Accessibility: confirmed no overlap

Checked the diff surface directly:

- **`ResizeObserver` block** (`XtermTerminal.tsx` L449-518): contains only
  `entry.contentRect` size comparison, `setTimeout`/double-`requestAnimationFrame`
  debounce scheduling, and `fitAddonRef.current?.fit()` calls. No `aria-*`, `role`,
  `tabIndex`, or `.focus()` calls anywhere in this block.
- **`useTerminalFlowControl.ts` `resize()`** (L403-436): pure throttle check +
  `console.warn`/`console.log` + RPC message dispatch (`case: "resize"`). No DOM, no
  ARIA, no focus.
- The file's actual focus/ARIA surface is elsewhere and untouched by this fix:
  - Floating Copy button: `aria-label="Copy selected text"` (L642)
  - Copy toast: `aria-live="polite"` (L652)
  - `terminalRef.current?.focus()` (L612, only in an unrelated visibility/lifecycle path)
  - `TerminalContextMenu` (imported L13, rendered L659-666) — a separate component with
    its own focus/keyboard handling, invoked only from the `contextmenu` DOM listener
    (L291-296), which the resize fix does not touch.

No code path introduced by AC1-4 shares a line, a ref, or a state variable with any of
the above. Confirmed no accessibility regression risk.

## Summary for the ADR (AC4)

State in the ADR: the WebGL→canvas fallback on detected oscillation follows the
project's existing, established pattern (`onContextLoss` handler, same file) of
console-only diagnostics for renderer-level infrastructure events. It is intentionally
silent to the user — no toast, no status indicator — because (a) the renderer swap is
not visually perceptible under the buffer/renderer model xterm.js uses, (b) the trigger
condition is a stuck/frozen UI where the user cannot take any action on the information
anyway, and (c) every comparable event in this codebase (WebGL load failure, WebGL
unavailable, context loss) already follows this silent pattern — introducing a toast
here would be inconsistent and would add exactly the kind of "look at me" interruption
the fix is supposed to eliminate.
