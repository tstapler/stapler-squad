# ADR-003: Sticky Flex Layout for Toolbar Keyboard Avoidance

**Status:** Accepted
**Date:** 2026-04-08
**Context:** Mobile UX Improvements — terminal toolbar visibility when virtual keyboard opens

## Context

When the iOS virtual keyboard opens, `position: fixed` elements are anchored to the **layout viewport** (which does not shrink), causing them to be hidden behind the keyboard. This bug persists on iOS 17 and is architectural to Apple's viewport model — not a fixable browser bug.

Three approaches were evaluated for keeping the terminal toolbar visible when the keyboard is open:

1. **`position: fixed` + `--keyboard-height` CSS var transform** — Move the fixed toolbar up by `--keyboard-height` using `transform: translateY(-var(--keyboard-height))`. Functional but requires JS-driven CSS var AND a transition. Double-listener pattern (resize + scroll) required. Can still lag on iOS 15.

2. **Sticky flex layout with `--viewport-height`** — Make the terminal container a `display: flex; flex-direction: column; height: var(--viewport-height, 100dvh)` shell. The toolbar is `flex-shrink: 0` at the bottom of the flex column. When `--viewport-height` shrinks (written by ViewportProvider on keyboard open), the entire shell shrinks and the flex layout pushes the toolbar up automatically — no separate keyboard-height offset needed.

3. **`position: sticky` within scrollable panel** — Less control than flex layout.

## Decision

Use **sticky flex layout (Option 2)** for the terminal container.

The `TerminalOutput` component renders a flex column with `height: var(--viewport-height, 100dvh)`. Children:
1. `.toolbar` — `flex-shrink: 0`
2. `.terminalContainer` — `flex: 1; min-height: 0; overflow: hidden`
3. `.mobileKeyboard` — `flex-shrink: 0` (toggleable)

When `--viewport-height` decreases on keyboard open, the flex container shrinks, xterm canvas gets smaller (triggering ResizeObserver → `fitAddon.fit()`), and the toolbar remains visible. `position: fixed` is not used for keyboard-interactive elements.

## Consequences

**Positive:**
- Completely avoids the iOS `position: fixed` + virtual keyboard bug
- Toolbar stays visible without any transform or per-element keyboard offset CSS
- xterm canvas resize is self-healing via ResizeObserver (ADR-002)
- Works correctly in iOS PWA mode
- No transition timing to manage for toolbar repositioning

**Negative:**
- Terminal container must be a flex column — descendants needing `height: 100%` need `min-height: 0`
- Modal height calculations using `calc(100vh - ...)` need updating to `var(--viewport-height, 100dvh)`

**Risks:**
- `height: 100%` in Safari flex sometimes resolves incorrectly — mitigated by `flex: 1; min-height: 0` instead
- If `--viewport-height` is unset (SSR, non-iOS), `100dvh` fallback ensures no regression

## References

- findings-pitfalls.md Section 5 (position:fixed + keyboard bug, iOS 17)
- findings-architecture.md Recommendation Matrix
- findings-stack.md Section 1 (dvh fallback)
