# Requirements: Terminal Resize/Fit Feedback Loop Convergence

Status: Draft | Phase: 1 - Ideation complete (derived from backlog item, no interview — unattended run)
Created: 2026-07-27
Backlog item: fc63d55b-d6cf-4e11-af02-c76c86637c5e

## Problem Statement

In multi-terminal usage (multiple sessions open, one per browser tab per current architecture —
the original report described a same-page "tiled" layout that does not exist in this codebase;
the equivalent reproducible condition is multiple concurrent terminal tabs/sessions plus a
window resize or tab background/resume cycle), `XtermTerminal`'s `ResizeObserver` → `fit()` →
`terminal.onResize` → `useTerminalFlowControl.resize()` chain can fail to converge:

- The `ResizeObserver` handler (`XtermTerminal.tsx`, around line 453) gates on a **raw pixel**
  delta (`Math.abs(width - lastContainerSize.width) > 1`), not on whether `FitAddon` actually
  proposes different integer `cols`/`rows`. A sub-cell pixel wobble (e.g. from a WebGL glyph
  metric that doesn't exactly equal the CSS cell width — see `Actual pixels per column: 8.45px`
  vs `Expected pixels per column: 8.33px` in the console evidence) can keep re-triggering `fit()`.
- `useTerminalFlowControl.resize()` (around line 403) throttles by **time** (200ms) but not by
  **value** — if resize is invoked repeatedly with the same `cols`/`rows` after the throttle
  window elapses, it still sends a redundant `TerminalResize` RPC and a follow-up
  `currentPaneRequest` re-fetch.
- Each server round trip and each `fit()` can perturb container layout (scrollbar
  appearance/disappearance, sub-pixel reflow) enough to fire the `ResizeObserver` again, so the
  loop can in principle continue indefinitely, pegging CPU, flooding the server with resize RPCs,
  and (per the corroborating "Duplicate shortcut id" churn in the original report) coinciding
  with excessive re-renders elsewhere in the terminal component tree.

## Success Criteria

1. Any resize-triggering event (window resize, tab background/resume, container reflow) settles
   to zero further `fit()` calls and zero further resize RPCs within a bounded number of debounce
   cycles, per terminal instance, independent of how many terminal sessions/tabs are open
   concurrently.
2. `fit()` is only invoked when `FitAddon.proposeDimensions()` reports an integer `cols`/`rows`
   pair that differs from the terminal's currently-applied `cols`/`rows` — sub-cell pixel deltas
   are a no-op regardless of the existing 1px raw-pixel prefilter.
3. `useTerminalFlowControl.resize()` skips sending the `TerminalResize` RPC and the follow-up
   `currentPaneRequest` re-fetch when the incoming `(cols, rows)` equals the last value actually
   sent — this dedup is independent of the existing 200ms time throttle (both apply).
4. The WebGL actual-vs-expected pixels-per-column discrepancy cannot cause unbounded churn even
   if it isn't fully eliminated: an oscillation detector recognizes a resize burst (the same
   `cols`/`rows` pair recurring ≥3 times within a rolling 2000ms window) and falls back to
   **xterm.js's default (non-WebGL) DOM renderer** by disposing the WebGL addon — *amended during
   Phase 3 planning*: the original wording here said "canvas renderer," but `@xterm/addon-canvas`
   is deprecated and incompatible with this repo's pinned `@xterm/xterm ^6.0.0`; see
   `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md` for the full correction and
   rationale. The functional intent (eliminate WebGL as the loop's amplifier) is unchanged. A
   `console.error` backstop fires when there is no WebGL addon instance to dispose because it
   genuinely never loaded — a second oscillation burst *after* an earlier successful fallback this
   session is a distinct, already-handled state and does not repeat that error (logged instead at
   `console.log`, per the ADR). The renderer-fallback decision is documented in an ADR.
5. Automated regression coverage (unit tests for the extracted pure decision functions, plus a
   component test exercising the real `ResizeObserver` wiring) simulates a sequence of sub-cell
   container resizes and asserts `fit()` and the resize RPC are each invoked at most once when
   proposed `cols`/`rows` never actually change.
6. A real `cols`/`rows` change (e.g. resizing the browser window enough to cross a cell boundary)
   still triggers exactly one `fit()` and one resize RPC — the fix must not cause legitimate
   resizes to be silently dropped.
7. The original manual repro (3 concurrent terminal sessions, one per browser tab; background/
   resume the tab or resize the window once) no longer pegs CPU or freezes input, verified
   manually after the fix lands.

## Scope

### Must Have (MoSCoW)
- Integer-`cols`/`rows` gating in `XtermTerminal`'s `ResizeObserver` handler before scheduling
  `fit()` (AC2)
- Value-based dedup in `useTerminalFlowControl.resize()`, independent of the time throttle (AC3)
- Oscillation/refit-burst detector (implemented as `shouldAbandonWebgl()` — renamed from this
  bullet's original working name `shouldFallbackToCanvas()` during Phase 3 planning, since "canvas"
  is not what's actually being fallen back to; see plan.md's Domain Glossary and AC4 above) that
  triggers a WebGL → default-renderer fallback, with a `console.error` backstop when there is no
  WebGL addon instance to dispose because it never loaded (AC4)
- ADR documenting the renderer-fallback decision and rationale (AC4)
- Unit tests for the new pure functions (dimension-gate comparison, resize dedup, oscillation
  detector) (AC5)
- A new/updated `XtermTerminal` component test simulating a sub-cell resize sequence against the
  real `ResizeObserver` wiring (AC5)
- Manual verification pass reproducing the original ticket scenario post-fix (AC6/AC7)

### Should Have
- Nothing beyond Must Have — this is a targeted convergence bug fix, not a resize-system rewrite.

### Out of Scope
- Building an in-page tiled/split-pane terminal layout (does not exist in this codebase today;
  AC1's repro is reframed to the existing one-session-per-tab architecture, per the AC1 text
  itself)
- Root-causing xterm.js's/WebGL's exact sub-pixel glyph-metric math — the fix bounds and
  mitigates the symptom (unbounded churn), it does not attempt to make WebGL glyph width exactly
  equal to CSS cell width upstream
- Fixing the `shortcutRegistry.ts` "Duplicate shortcut id" churn as a separate defect — it is
  treated as a downstream symptom of the resize/re-render loop; if the convergence fix removes
  the continuous re-render storm, this stops firing as a side effect. No dedicated shortcut
  registry idempotency fix is in scope unless verification shows it still fires after the
  convergence fix lands.
- Server-side (Go) resize RPC handling changes — the fix is client-side dedup before the RPC is
  ever sent

## Constraints

- Tech stack: React/TypeScript web-app, xterm.js (`@xterm/xterm`, `@xterm/addon-fit`,
  `@xterm/addon-webgl`), ConnectRPC/WebSocket streaming to the Go backend.
- Must follow `.claude/rules/css-architecture.md` for any styling touched (none expected — this
  is behavioral/logic only).
- Must not regress legitimate resize behavior (AC6) — the fix must be a precision gate, not a
  blanket suppression.
- New ADR required per AC4 — follows existing `docs/adr/` numbering convention (see
  `docs/adr/009-vanilla-extract-type-safe-css.md` for format reference).

## Context

### Current Architecture
```
Window resize / tab resume / container reflow
  → ResizeObserver fires (XtermTerminal.tsx ~line 453)
  → raw-pixel prefilter (>1px width/height delta) — NOT integer cols/rows aware
  → 150ms debounce + double-rAF
  → fitAddon.fit()
  → terminal.onResize({cols, rows}) — already deduped against lastSizeRef (cols/rows equality)
  → onResize prop → ... → useTerminalFlowControl.resize(cols, rows)
  → 200ms time throttle only (no value dedup)
  → TerminalResize RPC sent + 100ms-delayed currentPaneRequest re-fetch
  → server-side resize can perturb layout (scrollbar, content reflow)
  → ResizeObserver may fire again
```

Key files:
- `web-app/src/components/sessions/XtermTerminal.tsx` — ResizeObserver + FitAddon wiring
  (observer setup ~line 449-508), WebGL addon load (~line 264-281)
- `web-app/src/lib/hooks/useTerminalFlowControl.ts` — `resize()` (~line 403-455), time-throttle
  logic
- `web-app/src/lib/shortcuts/shortcutRegistry.ts` — corroborating churn symptom, out of scope
  per above
- `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx`,
  `XtermTerminalBug.test.tsx` — existing test coverage to extend
- `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` — existing test coverage to
  extend
- `docs/adr/` — ADR location/format for AC4's required decision doc

### Existing Work
No prior investigation in this repo specific to this bug. Related but distinct prior plans:
`project_plans/terminal-jank/` (session-switch loading/scroll jank, not resize convergence) and
`project_plans/terminal-robustness/` (general terminal robustness, not this specific loop).

### Stakeholders
All Stapler Squad users — any multi-session terminal usage is affected; this is a P3-priority
correctness/performance bug (CPU pegging, UI freeze) rather than a feature request.

## Research Dimensions

- [ ] Stack — xterm.js `FitAddon.proposeDimensions()` API contract, `@xterm/addon-webgl`
  disposal/fallback pattern, `ResizeObserver` best practices for avoiding feedback loops
- [ ] Features — how `terminal.onResize` cols/rows dedup already works (`lastSizeRef`) vs. where
  the gap is (pixel-level prefilter, not integer-level)
- [ ] Architecture — where the pure decision functions (dimension-gate, resize-value-dedup,
  oscillation-detector) should live for testability (extracted vs. inline)
- [ ] Pitfalls — known xterm.js/WebGL sub-pixel glyph metric issues, ResizeObserver loop
  detection prior art, risks of over-suppressing legitimate resizes (AC6 regression risk)
- [ ] Testing — how to simulate `ResizeObserver` entries in JSDOM/component tests for AC5's
  component-level coverage
