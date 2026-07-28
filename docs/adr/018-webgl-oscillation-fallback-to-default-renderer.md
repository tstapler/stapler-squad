# ADR-018: WebGL Oscillation Fallback Falls Back to xterm.js's Default Renderer, Not "Canvas"

**Status**: Accepted
**Date**: 2026-07-27
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/terminal-resize-fit-loop/` (Terminal Resize/Fit Feedback Loop
Convergence), Acceptance Criterion 4

## Context

`XtermTerminal.tsx`'s `ResizeObserver` → `FitAddon.fit()` → `terminal.onResize` →
`useTerminalFlowControl.resize()` chain can fail to fully converge when a WebGL glyph-metric
mismatch causes `FitAddon.proposeDimensions()` to disagree with the terminal's currently-applied
grid by a sub-cell amount — observed in production console output as:

```
[XtermTerminal] Actual pixels per column: 8.45px
[XtermTerminal] Expected pixels per column: 8.33px
[XtermTerminal] ⚠️ SIZING MISMATCH! Container width doesn't match cell width calculation
```

Two other fixes in this project (AC2's integer-cols/rows gate, AC3's value-dedup) close the
straightforward re-triggering paths, but per `project_plans/terminal-resize-fit-loop/research/pitfalls.md`
§3, a WebGL glyph-metric mismatch can in principle cause `proposeDimensions()` to genuinely
*ping-pong* between two adjacent integer values (e.g. 84↔85 cols) as sub-pixel rounding flips back
and forth — a real oscillation, not merely redundant identical calls, and therefore not fully
closed by AC2/AC3 alone. The requirement (AC4) calls for a circuit breaker: detect ≥3 recurrences
of the same `(cols, rows)` pair within a rolling 2000ms window and "fall back to the canvas
renderer, disposing the WebGL addon."

### The "canvas renderer" wording is not achievable as literally stated

- This repo pins `"@xterm/xterm": "^6.0.0"` (`web-app/package.json`), already the newest release.
- `@xterm/addon-canvas` — the actual package providing a canvas-based renderer — is **deprecated
  and does not support `@xterm/xterm` v6**. The canvas renderer was moved out of xterm.js core into
  this now-deprecated addon; upstream direction (corroborated by `cockpit-project/cockpit` issue
  #22509, "`@xterm/addon-canvas` deprecated and will be removed in `@xterm/xterm` v6") confirms it
  blocks consumers from upgrading to xterm.js 6.0.0 if they depend on it.
- `web-app/package.json` has no `@xterm/addon-canvas` dependency today.
- What `webglAddon.dispose()` actually does (confirmed from `@xterm/addon-webgl@0.19.0` source,
  `WebglAddon.ts`'s registered teardown callback): it calls
  `renderService.setRenderer(core._createRenderer())` — re-installing whatever renderer
  `_createRenderer()` returns by default when no accelerated addon is loaded. Since xterm.js 5.x,
  that default is the **DOM renderer** (per-cell `fillText`-based), not a canvas renderer.
- This is exactly what the codebase's *existing* `onContextLoss` handler
  (`XtermTerminal.tsx:269-272`) already does today for genuine browser-driven WebGL context loss —
  it calls `webglAddon.dispose()` with nothing else loaded, and has always fallen back to the DOM
  renderer, not canvas. AC4's oscillation-triggered fallback is meant to reuse this exact,
  already-proven mechanism.

Two options were considered:

1. **Correct the terminology**: keep the existing minimal `dispose()`-only fallback (matching
   `onContextLoss`), and document that the resulting renderer is xterm.js's default DOM renderer.
2. **Chase the literal wording**: add `@xterm/addon-canvas` as a new dependency despite its
   deprecation and incompatibility with the pinned `@xterm/xterm ^6.0.0`, likely requiring a pin to
   an older, unmaintained addon version and accepting known breakage risk, just to technically
   produce a canvas-backed renderer.

## Decision

**Option 1.** The oscillation-burst detector (`shouldAbandonWebgl` in
`web-app/src/lib/terminal/resizeConvergence.ts`) calls `webglAddonRef.current.dispose()` — the
same call the existing `onContextLoss` handler already makes — with no new renderer addon loaded.
The resulting renderer is xterm.js's **default (DOM) renderer**, and all code comments, log
messages, and this project's plan/glossary refer to it as such, not as "canvas."

Trigger condition: the most recent `(cols, rows)` value observed via `terminal.onResize` (the
common funnel every resize source — `ResizeObserver`, font-size/font-family effects, imperative
`ref.fit()`, `StateApplicator.applyDimensionChange()` — routes through) recurs `>= 3` times within
a rolling 2000ms window. The detector is fed only "post-dedup" resize applications (xterm.js's own
`Terminal.resize()` no-ops when unchanged, and `XtermTerminal.tsx`'s `lastSizeRef` guard already
filters `terminal.onResize` to genuine value changes before the detector ever sees an entry) —
this is what prevents a legitimate, monotonically-changing window/pane-divider drag from
false-triggering the fallback (see `research/pitfalls.md` §3 for the full false-positive analysis).

The fallback is **session-scoped, not persisted**: the WebGL addon is only ever loaded once, in
the component's mount-time effect; there is no code path that reloads it mid-session, so once
disposed (by either `onContextLoss` or this oscillation trip) the terminal instance stays on the
default renderer for the rest of its mount. Nothing is written to `localStorage` or module-level
state to remember the fallback across remounts/reconnects/other sessions — this is a deliberate
choice, not an oversight: WebGL sub-pixel glyph metrics are font/zoom/monitor-DPI dependent per
session, not a stable global property of the browser or machine, so a permanent/global
disablement would be over-broad. Root-causing the underlying pixel-vs-cell-width mismatch is
explicitly out of scope for this decision and for the parent project.

If the detector trips but no `WebglAddon` instance is currently loaded (`webglAddonRef.current`
is `null` — WebGL never successfully loaded, e.g. no `WebGL2RenderingContext` on the platform, or
it was already disposed by `onContextLoss`), the fallback branch logs via `console.error` instead
of throwing, since canvas/DOM is presumably already active and "falling back" is a no-op that
should still be visible for debugging (the oscillation in that case has some other root cause
worth flagging distinctly).

The fallback is deliberately **silent to the user** — `console.warn`/`console.error` only, no
toast or status indicator — matching every other renderer-level infrastructure event already
logged in this file (WebGL load failure, WebGL unavailable, context loss). Per
`project_plans/terminal-resize-fit-loop/research/ux.md`: the renderer swap is not visually
perceptible (xterm.js redraws from its in-memory buffer, not from stale pixels), the trigger
condition is a stuck/frozen UI where the user cannot act on the information, and every comparable
event in this codebase already follows the silent pattern.

## Consequences

### Positive
- Zero new dependencies; reuses code (`webglAddon.dispose()`) already proven correct by the
  existing `onContextLoss` handler.
- No risk of a `@xterm/addon-canvas`/`@xterm/xterm ^6.0.0` incompatibility ever entering the
  dependency tree.
- Terminology in code, tests, logs, and this project's plan is now internally consistent and
  accurate — future maintainers reading "falls back to the default renderer" won't go looking for
  a canvas addon that was never installed.
- The oscillation detector's history buffer, hooked at `terminal.onResize`, catches oscillations
  from *any* fit()-triggering source (not just the `ResizeObserver`), including font-size changes
  and `StateApplicator`-driven resyncs.

### Negative
- The AC4 acceptance criterion's literal text ("falls back to the canvas renderer") remains
  technically inaccurate versus the shipped behavior; this ADR is the authoritative correction, and
  `requirements.md`/the plan reference it rather than silently diverging from the written AC.
- The DOM renderer is measurably slower than WebGL for large-scrollback repaints. This is accepted
  because the fallback only engages after a live oscillation is already pegging CPU — the DOM
  renderer is strictly better than the spin loop it replaces, and it only affects the one terminal
  instance that tripped the detector (session-scoped), not all tabs/sessions.
- Font rendering may differ subtly (WebGL texture-atlas glyphs vs. DOM `fillText` glyphs) — an
  existing, already-live risk from the `onContextLoss` path, not a new risk introduced here.

### Neutral
- No mechanism exists (or is added) to re-enable WebGL later in the same mount, or to remember the
  fallback across a remount/reconnect/new session. A future project could add a
  `TerminalDimensionCache`-style persisted "skip WebGL for this session" flag if repeated
  oscillation across reconnects proves common in practice — deliberately not built now, per Scope.
