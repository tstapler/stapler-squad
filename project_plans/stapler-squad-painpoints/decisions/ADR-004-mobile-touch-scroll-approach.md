# ADR-004: Mobile Touch Scroll Approach

**Status**: Accepted
**Date**: 2026-04-16
**Deciders**: Tyler Stapler

---

## Context

xterm.js has no native touch scroll support. GitHub issue #5377 (July 2025, confirmed open) documents that ballistic/momentum scroll is unsupported and that touch events send arrow keys to the PTY rather than scrolling the viewport. Users on iOS and Android cannot scroll through terminal output.

The xterm.js terminal in Stapler Squad is configured with `scrollback: 0` (tmux owns history). The terminal wrapper `div` uses `overflow-y: hidden` in CSS. The `xterm-viewport` element is an internal xterm.js div that handles scroll position.

Two approaches were evaluated:

- **Option A (CSS-only: `overscroll-behavior: contain`)**: Add `overscroll-behavior: contain` to the terminal container CSS. Prevents scroll chaining to parent.
- **Option B (JS touch interception)**: Add `touchstart`/`touchmove` event listeners to the xterm container. Compute `deltaY` from touch position delta. Call `terminal.scrollLines(delta)` on each move event.

`overscroll-behavior: contain` was verified as added in Safari 16 (not 15). iOS 15 users get no benefit from CSS-only. Furthermore, `overscroll-behavior` prevents scroll *chaining* but does not implement xterm viewport scrolling — the xterm viewport has `overflow: hidden`, so there is nothing to contain.

---

## Decision

**Option B: Application-level JS touch interception, with CSS `overscroll-behavior: contain` as a belt-and-suspenders layer for iOS 16+.**

Implementation in `XtermTerminal.tsx` (or a new `useTouchScroll` hook):

```typescript
// Attach to the xterm container div ref
containerRef.current.addEventListener('touchstart', handleTouchStart, { passive: false });
containerRef.current.addEventListener('touchmove', handleTouchMove, { passive: false });
containerRef.current.addEventListener('touchend', handleTouchEnd, { passive: false });
```

Logic:
- On `touchstart`: record `touchStartY`, set `isScrolling = true`
- On `touchmove`: compute `deltaY = touchStartY - currentY`. Call `terminal.scrollLines(Math.round(deltaY / lineHeightPx))`. Update `touchStartY`. Call `event.preventDefault()` to prevent page scroll.
- On `touchend`: reset state. Optionally implement momentum: apply a decaying velocity over ~300ms using `requestAnimationFrame`.

Line height is derived from `terminal.options.fontSize * 1.2` (approximate — more precise value from `(terminal as any)._core._renderService.dimensions.css.cell.height` if available).

`event.preventDefault()` on `touchmove` is called only when `isScrolling` is true and the gesture is primarily vertical (|deltaY| > |deltaX| by a threshold of 10px). This preserves horizontal swipe gestures for navigation.

The `{ passive: false }` option is required for `preventDefault()` to work on `touchmove`.

CSS additions to `XtermTerminal.module.css`:
```css
.terminal {
  overscroll-behavior: contain;      /* belt-and-suspenders for iOS 16+ */
  touch-action: pan-x pan-y;        /* NOT 'none' — preserves text selection long-press */
}
```

Note: `touch-action: none` is explicitly NOT used because it disables long-press text selection on mobile.

---

## Rationale

CSS alone (Option A) does not work: `overscroll-behavior` is not supported on iOS 15, and even on iOS 16+ it only prevents scroll chaining to a parent — it does not make xterm's `overflow: hidden` viewport scrollable. xterm.js issue #5377 confirms no CSS-only fix exists.

The `terminal.scrollLines(n)` call is the public xterm.js API for programmatic viewport scrolling. It is not a private API call. This is safe to use.

The concern about using `_core.viewport.scrollLines()` (mentioned in research) is only relevant for Phase 2 lazy scrollback (where the private viewport API might be needed for prepend operations). Touch scroll does not require any private APIs.

---

## Consequences

**Positive:**
- Works on iOS 15 and Android Chrome (no CSS feature dependency).
- Uses public `terminal.scrollLines()` API — no version pinning required for this feature.
- Preserves text selection (long-press) by not using `touch-action: none`.
- `overscroll-behavior: contain` provides additional scroll isolation on iOS 16+.

**Negative / Accepted costs:**
- `{ passive: false }` on `touchmove` disables browser scroll optimizations for that element. Performance impact is acceptable for a terminal that doesn't need smooth page scroll.
- Momentum scroll requires ~100 lines of `requestAnimationFrame` animation. This is deferred to Phase 2 of this feature (Phase 1 ships non-momentum scroll — functional but not silky).
- Must be removed or guarded if xterm.js adds native touch support in a future release.

---

## Alternatives Not Chosen

**CSS-only (`overscroll-behavior: contain`)**: Rejected. Does not work on iOS 15. Does not implement scrolling — only prevents scroll chaining. The underlying xterm viewport is `overflow: hidden`.

**CSS `touch-action: none`**: Rejected as a standalone solution. Breaks long-press text selection on mobile, which is the primary way mobile users copy terminal output.
