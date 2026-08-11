# Architecture: Keyboard-Aware Layout — Research Findings

**Date:** 2026-04-07
**Stack:** React (Next.js App Router), CSS Modules, xterm.js
**Target problems:**
- Terminal toolbar overflows / gets obscured when virtual keyboard appears
- Mobile keyboard toggle button placement
- Virtual keyboard compresses or covers terminal content

---

## 1. CSS-Only Approach: `dvh`

### How it works

`100dvh` tracks the actual visible viewport height in real time as the address bar animates. With a flex-column shell, children adapt automatically:

```css
/* CSS Module — app shell */
.appShell {
  height: 100dvh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.terminalArea {
  flex: 1;
  min-height: 0;   /* critical: allows flex child to shrink below content size */
  overflow: hidden;
}

.toolbar {
  flex-shrink: 0;
}
```

### When it works

- Static layouts where only the shell needs to resize
- Preventing the page from being taller than the visible area
- Keeping a fixed footer/toolbar in flow (flex pushes it to the bottom)
- Browser support: Safari 15.4+ (~95%+ of active iOS installs)

### When it falls short

1. **xterm.js does not respond to CSS changes.** xterm renders into a `<canvas>` and computes row/col count from `offsetWidth`/`offsetHeight` at `FitAddon.fit()` time. A CSS height change from `dvh` recalculation does NOT trigger xterm resize.
2. **`dvh` does not update on keyboard open.** On iOS, `dvh` only tracks browser chrome (address bar), not the virtual keyboard. So `100dvh` does NOT shrink when the keyboard opens — it's not the right tool for keyboard-aware layout.
3. **No programmatic access to keyboard height.** Can't position overlays, tooltips, or calculate offsets from CSS alone.
4. **`height: 100%` bug in Safari flex.** Use `flex: 1` + `min-height: 0` instead.

### Pros / Cons

| Pros | Cons |
|------|------|
| Zero JavaScript | Does not update on keyboard open (critical limitation) |
| No React re-renders | xterm.js requires explicit `fit()` call |
| Eliminates `100vh` address-bar bug | Slight animation lag during address-bar slide |

---

## 2. `visualViewport` Hook — CSS Variable Pattern

### How it works

`window.visualViewport.height` shrinks when the virtual keyboard opens. Write this as CSS custom properties on `:root` from a side-effect-only component — no React re-renders, CSS-driven layout:

```tsx
// ViewportProvider.tsx — mount once in root layout, renders nothing
'use client'
import { useEffect } from 'react'

export function ViewportProvider() {
  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return

    const update = () => {
      requestAnimationFrame(() => {
        const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
        document.documentElement.style.setProperty('--keyboard-height', `${kb}px`)
        document.documentElement.style.setProperty('--viewport-height', `${vv.height}px`)
      })
    }

    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update) // iOS needs both
    update()
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [])

  return null
}
```

```css
/* CSS Module — use the variables */
.appShell {
  height: var(--viewport-height, 100dvh);
}

.toolbar {
  position: fixed;
  bottom: var(--keyboard-height, env(safe-area-inset-bottom, 0px));
  transition: bottom 0.15s ease-out;
}

.terminalContainer {
  height: calc(var(--viewport-height, 100dvh) - 48px);
}
```

### Pros / Cons

| Pros | Cons |
|------|------|
| Exact keyboard height available to CSS and JS | Fires on every animation frame during keyboard slide — needs `requestAnimationFrame` debounce |
| Zero React re-renders (CSS variable bridge) | `window.visualViewport` is undefined in SSR — requires guards in Next.js |
| One `useEffect`, no context/providers | Derived `keyboardHeight` can be non-zero during scroll (use `> 100` threshold) |
| Works on iOS Safari 13+ | On Android, `window.innerHeight` changes instead — needs separate handling |
| Drives xterm.js `fit()` on same resize event | — |

---

## 3. React Context Approach

### How it works

`KeyboardContext` wraps `visualViewport` detection and exposes `{ isOpen, keyboardHeight, viewportHeight }`:

```tsx
// context/KeyboardContext.tsx
'use client'
import { createContext, useContext, useEffect, useState, ReactNode } from 'react'

interface KeyboardState {
  isOpen: boolean
  keyboardHeight: number
  viewportHeight: number
}

const KeyboardContext = createContext<KeyboardState>({
  isOpen: false, keyboardHeight: 0, viewportHeight: 0,
})

export function KeyboardProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<KeyboardState>({
    isOpen: false, keyboardHeight: 0, viewportHeight: 0,
  })

  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return
    const update = () => {
      const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
      setState({ isOpen: kb > 100, keyboardHeight: kb, viewportHeight: vv.height })
    }
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    update()
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [])

  return <KeyboardContext.Provider value={state}>{children}</KeyboardContext.Provider>
}

export const useKeyboard = () => useContext(KeyboardContext)
```

### When is Context overkill vs. the right call?

**Overkill when:**
- Only one or two components need keyboard state
- The reaction is purely visual (use CSS variables instead — zero re-renders)
- You only need to call `FitAddon.fit()` — put `visualViewport` listener directly in the terminal component

**Right call when:**
- Multiple sibling components at different tree depths need `isOpen` with different responses
- You need atomic coordination: toolbar hides AND terminal resizes AND overlay repositions at the same time
- You want to unit-test keyboard-aware behavior (context can be mocked)

### Pros / Cons

| Pros | Cons |
|------|------|
| Clean `useKeyboard()` API anywhere in tree | Every state update re-renders all consumers — thrashes xterm canvas parent during animation |
| Single source of truth, no duplicate listeners | Must pair with `React.memo` or component splitting |
| Testable via context mocking | Provider boilerplate in layout |
| Easy to extend with `animating`, `keyboardType` etc. | Overkill for simple cases |

---

## 4. xterm.js + Virtual Keyboard

### The core problem

xterm.js computes row/col count by measuring `element.offsetHeight`/`element.offsetWidth` at `FitAddon.fit()` time. It does not observe CSS changes automatically (unless a `ResizeObserver` on the container detects a computed height change).

What breaks on iOS when keyboard opens:
- Canvas is sized for the old viewport height — content misaligns
- xterm's hidden `<textarea>` may not trigger iOS keyboard (iOS requires visible, focusable inputs)
- Scroll-into-view: iOS auto-scrolls page to bring the focused textarea into view, displacing the canvas

### Patterns that work

**Pattern A — ResizeObserver on the container (recommended)**

```tsx
useEffect(() => {
  const container = containerRef.current
  if (!container || !fitAddon.current) return

  const observer = new ResizeObserver(() => {
    requestAnimationFrame(() => {
      fitAddon.current?.fit()
    })
  })
  observer.observe(container)
  return () => observer.disconnect()
}, [])
```

Self-healing: fires whenever the container's computed size changes (keyboard, orientation, window resize). Combine with `dvh`/`--viewport-height` CSS on the container and it becomes automatic.

**Pattern B — `visualViewport` resize calling `fit()`**

```tsx
useEffect(() => {
  const vv = window.visualViewport
  if (!vv || !fitAddon.current) return

  const handleResize = () => {
    requestAnimationFrame(() => {
      fitAddon.current?.fit()
    })
  }

  vv.addEventListener('resize', handleResize)
  return () => vv.removeEventListener('resize', handleResize)
}, [])
```

**Pattern C — Prevent iOS scroll-into-view displacing terminal**

```css
/* Terminal wrapper uses position: fixed to remove from scroll flow */
.terminalWrapper {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: var(--keyboard-height, 0px);
  transition: bottom 0.15s ease-out;
}
```

**Pattern D — Prevent iOS textarea auto-zoom**

xterm.js creates a hidden `<textarea>` for keyboard input. If its `font-size` is under 16px, iOS auto-zooms on focus:

```css
/* globals.css or TerminalOutput.module.css */
.xterm-helper-textarea {
  font-size: 16px !important;
}
```

Or via JS after `terminal.open()`:
```tsx
const textarea = containerRef.current?.querySelector('textarea')
if (textarea) (textarea as HTMLElement).style.fontSize = '16px'
```

---

## Recommendation Matrix

| Problem | Best Approach | Why |
|---------|--------------|-----|
| Terminal toolbar obscured by keyboard | `ViewportProvider` writing `--keyboard-height` CSS var + `position: fixed` toolbar | Exact offset, CSS-driven, zero re-renders |
| xterm.js wrong size after keyboard | `ResizeObserver` on container → `fitAddon.fit()` | Self-healing, no keyboard detection needed |
| Mobile keyboard toggle button | `useState` toggle + `localStorage` (hamburger pattern) | Already established pattern in codebase |
| Keyboard toggle button placement | CSS `position: fixed; bottom: var(--keyboard-height)` | Stays above keyboard automatically |
| Preventing iOS scroll-into-view | `position: fixed` container with `--keyboard-height` bottom | Removes terminal from scroll flow |
| iOS textarea auto-zoom | CSS `font-size: 16px` on `.xterm-helper-textarea` | CSS-only, no JS needed |
| Multi-component coordination | React Context (only if tree is split) | Single listener, testable |

---

## Overall Architecture Recommendation

**Two-layer approach:**

**Layer 1 — CSS (layout, zero re-renders):**
- Mount `ViewportProvider` in root `layout.tsx` (renders nothing, writes CSS vars)
- `height: 100dvh` on app shell (address bar; keyboard handled by `--viewport-height`)
- All layout adjustments via `--keyboard-height` and `--viewport-height` CSS vars
- `env(safe-area-inset-bottom)` on bottom-fixed elements (after adding `viewportFit: 'cover'`)

**Layer 2 — JS (xterm.js only):**
- `ResizeObserver` on xterm container calling `fitAddon.fit()` inside `requestAnimationFrame`
- No React Context needed for this project's stated problems

**Skip React Context for now.** Add it only if a third/fourth component in a separate subtree needs `isOpen` programmatically.

### Implementation order

1. Add `viewportFit: 'cover'` to `app/layout.tsx` viewport export
2. Mount `ViewportProvider` in root layout — writes CSS vars, renders nothing
3. Set `height: 100dvh` on app shell; replace all `100vh` in layout containers
4. Add `env(safe-area-inset-bottom)` padding to `globals.css` bottom-fixed elements
5. Toolbar: add `overflow-x: auto` to `.actions` on mobile; add `position: fixed` + `--keyboard-height` bottom offset
6. Mobile keyboard toggle: add `isKeyboardVisible` state, toggle button, `localStorage` persistence
7. xterm.js: add `ResizeObserver` in terminal component calling `fitAddon.fit()`; set textarea `font-size: 16px`

---

## Sources

- Bram.us: Large, Small, and Dynamic Viewports — https://www.bram.us/2021/09/13/the-large-small-and-dynamic-viewports/
- CSS-Tricks: Large, Small, and Dynamic Viewport Units — https://css-tricks.com/the-large-small-and-dynamic-viewport-units/
- MDN: Visual Viewport API — https://developer.mozilla.org/en-US/docs/Web/API/Visual_Viewport_API
- xterm.js GitHub: iOS keyboard issues #2941, #3895 — https://github.com/xtermjs/xterm.js/issues/2941
