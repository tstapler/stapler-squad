# ADR-002: ResizeObserver vs visualViewport Event for xterm.js fit() Trigger

**Status:** Accepted
**Date:** 2026-04-08
**Context:** Mobile UX Improvements — xterm.js keyboard-aware resize

## Context

When the virtual keyboard opens or closes, the xterm.js terminal must re-fit its row/col count to the new container dimensions. Two approaches were evaluated:

1. **Direct `visualViewport` resize listener calling `fitAddon.fit()`** — Listens to `visualViewport.resize`, calls `fit()` inside `requestAnimationFrame`. Problem: calling `fit()` directly in the visualViewport handler gives wrong dimensions because the DOM hasn't reflowed yet. Requires double `requestAnimationFrame` to work on iOS.

2. **ResizeObserver on the xterm container** — The container's computed height changes when the CSS var `--viewport-height` updates (via ViewportProvider from ADR-001). The existing `ResizeObserver` in `XtermTerminal.tsx` (line 259) already detects this and calls `fitAddon.fit()` with debouncing.

## Decision

Use the **existing ResizeObserver** in `XtermTerminal.tsx`. No new `visualViewport` listener is needed in the terminal component.

The chain of causation:
1. Virtual keyboard opens → `visualViewport.height` shrinks
2. ViewportProvider updates `--viewport-height` CSS var (via `requestAnimationFrame`)
3. CSS recalculates container height: `height: calc(var(--viewport-height, 100dvh) - ...)`
4. Container's computed height changes → `ResizeObserver` fires
5. Existing debounced `fit()` call executes

One modification needed: the current `ResizeObserver` uses `setTimeout` for debouncing (lines 281-293). For iOS keyboard transitions, the initial debounce should use `requestAnimationFrame` instead of a 10ms timeout to ensure the DOM has reflowed before `fit()` measures dimensions. This is the "double rAF" pattern identified in pitfalls research.

## Consequences

**Positive:**
- No new event listeners in the terminal component
- Single source of truth for resize handling (the existing ResizeObserver)
- Self-healing: works for any reason the container resizes (keyboard, orientation, window resize, tab switch)
- Avoids the "wrong dimensions" bug documented in findings-pitfalls.md Section 4C

**Negative:**
- Indirect coupling: depends on ViewportProvider updating CSS vars promptly
- Two-hop latency: visualViewport event → CSS var update → ResizeObserver → fit()
- The debounce in the existing ResizeObserver adds ~10-250ms delay on top of the ViewportProvider's rAF delay

**Risks:**
- If ViewportProvider's CSS var update is delayed (e.g., by a long-running main thread task), the ResizeObserver won't fire until the container actually changes size. Mitigated by the rAF wrapper in ViewportProvider.
- The existing 250ms debounce (after first 3 resizes) may feel sluggish during keyboard animation. May need to reset the resize counter or use a shorter debounce for height-only changes.

## References

- XtermTerminal.tsx lines 254-297 (existing ResizeObserver implementation)
- findings-architecture.md Section 4 Pattern A (ResizeObserver recommendation)
- findings-pitfalls.md Section 4C (fitAddon.fit() race condition, double rAF)
