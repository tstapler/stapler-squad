# ADR-001: CSS Variable Bridge via ViewportProvider

**Status:** Accepted
**Date:** 2026-04-08
**Context:** Mobile UX Improvements — keyboard-aware layout

## Context

The web UI needs to respond to the iOS virtual keyboard opening and closing. Three approaches were evaluated:

1. **CSS-only (`dvh`)** — `100dvh` tracks address-bar chrome but does NOT update when the virtual keyboard opens (by CSS spec). Insufficient for this use case.
2. **ViewportProvider writing CSS vars** — A side-effect-only React component that listens to `visualViewport` resize/scroll events and writes `--keyboard-height` and `--viewport-height` as CSS custom properties on `:root`. Zero React re-renders; all layout adaptation happens in CSS.
3. **React Context (`KeyboardProvider`)** — Wraps `visualViewport` detection in a context exposing `{ isOpen, keyboardHeight, viewportHeight }`. Every state update re-renders all consumers, including the xterm.js canvas parent, causing thrashing during keyboard animation.

## Decision

Use **ViewportProvider (CSS variable bridge)**. Mount it once in `app/layout.tsx`. It renders `null` and writes two CSS custom properties:

- `--keyboard-height` — computed as `max(0, window.innerHeight - vv.height - vv.offsetTop)`
- `--viewport-height` — `visualViewport.height`

All layout containers use `var(--viewport-height, 100dvh)` for height and `var(--keyboard-height, 0px)` for bottom offsets.

React Context is not needed because:
- Only CSS layout needs keyboard dimensions (toolbar positioning, terminal container height)
- The xterm.js `ResizeObserver` already handles `fitAddon.fit()` when the container's computed size changes — it does not need a React state update
- No sibling components at different tree depths need `isOpen` programmatically

## Consequences

**Positive:**
- Zero React re-renders during keyboard animation (60fps layout changes via CSS)
- Single event listener pair (`resize` + `scroll` on `visualViewport`)
- All components opt in via CSS vars — no prop drilling or context subscription
- XtermTerminal's existing `ResizeObserver` automatically fires `fit()` when `--viewport-height` changes the container's computed height

**Negative:**
- CSS vars are not directly testable in unit tests (would need integration/E2E)
- `window.visualViewport` is undefined during SSR — guarded with `if (!vv) return`
- If a future feature needs `isOpen` as a React boolean (e.g., conditional rendering), a hook or context will need to be added

**Risks:**
- `visualViewport` resize fires 3-5 times with transitional values on iOS 15 — mitigated by wrapping updates in `requestAnimationFrame`
- Must listen to both `resize` AND `scroll` events on iOS — omitting `scroll` causes missed updates

## References

- findings-architecture.md Section 2 (ViewportProvider pattern)
- findings-stack.md Section 2 (visualViewport API)
- findings-pitfalls.md Section 2 (resize event reliability)
