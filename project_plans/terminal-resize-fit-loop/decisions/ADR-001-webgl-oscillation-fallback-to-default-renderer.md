# ADR-001: WebGL Oscillation Fallback Falls Back to xterm.js's Default Renderer, Not "Canvas"

**Status**: Accepted
**Date**: 2026-07-27
**Real ADR**: `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`

## Summary

AC4 requires an oscillation/burst detector that, when the same `(cols, rows)` pair recurs ≥3 times
within a rolling 2000ms window (a WebGL sub-cell glyph-metric wobble), disposes the WebGL addon
to stop the churn. AC4's text calls this a fallback to "the canvas renderer" — that is not
achievable as literally worded on this repo's pinned `@xterm/xterm ^6.0.0` (`@xterm/addon-canvas`
is deprecated and incompatible with v6, and is not a dependency today). Disposing the WebGL addon
(reusing the exact mechanism the existing `onContextLoss` handler already uses) actually falls
back to xterm.js's **default DOM renderer**. This ADR is the authoritative record of that
terminology correction and of the decision to keep the fallback minimal (dispose-only, no new
dependency, session-scoped, silent/console-only), not to chase the literal "canvas" wording.

See the real ADR at `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md` for full
context, the decision, and consequences.
