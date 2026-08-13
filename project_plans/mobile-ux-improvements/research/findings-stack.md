# iOS Safari Viewport & Keyboard Behavior — Stack Findings

**Date:** 2026-04-07
**Scope:** React/Next.js web apps on iPhone iOS Safari

---

## Summary

- Use `100dvh` instead of `100vh` for full-height containers tracking the visible viewport
- Use `100svh` when you need a stable minimum that never overflows
- Use `visualViewport` API to detect virtual keyboard open/close — the only reliable cross-browser iOS method
- Add `viewport-fit=cover` + `env(safe-area-inset-bottom)` padding on all fixed bottom UI
- Next.js App Router exposes a `viewport` export from `layout.tsx` for all of this

All four features have broad support: Chrome 108+, Safari 15.4+, Firefox 101+ for viewport units; Safari 13+ for visualViewport.

---

## 1. `dvh` / `svh` / `lvh` — Dynamic Viewport Units

### What it is

CSS Values Level 4 introduced three new viewport height unit families:

| Unit | Definition | iOS Safari behavior |
|------|-----------|---------------------|
| `svh` | **Small viewport** — browser UI fully visible | Stable, smallest. Always fits on screen. |
| `lvh` | **Large viewport** — browser UI fully hidden | Largest. May be obscured by address bar. |
| `dvh` | **Dynamic viewport** — updates in real time | Animates with address bar. Can cause reflow. |

`100dvh` is the "live" viewport height — the behavior developers expected from `100vh` but never got on iOS.

### Browser Support

| Browser | Version |
|---------|---------|
| Chrome | 108+ (Nov 2022) |
| Safari (iOS + macOS) | **15.4+** (Mar 2022) |
| Firefox | 101+ (May 2022) |
| Edge | 108+ |

**Global support: ~96%+** — safe to use without fallbacks.

### Code Examples

```css
/* Full-height app shell that tracks address bar */
.app-shell {
  height: 100dvh;
}

/* Full-height content that must always fit on screen */
.hero {
  min-height: 100svh;
}

/* Fallback pattern for older browsers */
.container {
  height: 100vh;   /* fallback */
  height: 100dvh;  /* override in supporting browsers */
}
```

### Gotchas

- `100dvh` causes **reflow during scroll** as the address bar animates — avoid on expensive-paint elements
- On **iOS Safari in standalone PWA mode**, `dvh == lvh == svh` (no address bar)
- `dvh` does NOT update when the virtual keyboard opens — only browser chrome changes trigger it
- Tailwind CSS: `h-dvh`, `min-h-dvh`, `max-h-dvh` added in v3.4

---

## 2. `visualViewport` API — Keyboard Detection

### What it is

`window.visualViewport` represents the visible portion of the document. When the virtual keyboard opens, it shrinks the visual viewport. The layout viewport (`window.innerHeight`) does NOT change on iOS.

| Property | Description |
|----------|-------------|
| `visualViewport.height` | Visible area height — shrinks when keyboard opens |
| `visualViewport.offsetTop` | Offset from layout viewport top |
| `visualViewport.scale` | Current pinch-zoom scale |

Key events: `resize` (fires when keyboard opens/closes) and `scroll`.

### Keyboard Detection Pattern

```typescript
import { useEffect, useState } from 'react'

export function useVirtualKeyboard() {
  const [keyboardHeight, setKeyboardHeight] = useState(0)

  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return

    const handleResize = () => {
      const diff = window.innerHeight - vv.height - vv.offsetTop
      setKeyboardHeight(Math.max(0, diff))
    }

    vv.addEventListener('resize', handleResize)
    vv.addEventListener('scroll', handleResize) // iOS needs both
    return () => {
      vv.removeEventListener('resize', handleResize)
      vv.removeEventListener('scroll', handleResize)
    }
  }, [])

  return { keyboardOpen: keyboardHeight > 100, keyboardHeight }
}
```

### CSS Variable Bridge (zero re-renders)

```tsx
// ViewportProvider.tsx — mount once in layout, renders nothing
'use client'
import { useEffect } from 'react'

export function ViewportProvider() {
  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return

    const update = () => {
      const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
      document.documentElement.style.setProperty('--keyboard-height', `${kb}px`)
      document.documentElement.style.setProperty('--viewport-height', `${vv.height}px`)
    }

    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    update()
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [])

  return null
}
```

### Browser Support

| Browser | Version |
|---------|---------|
| Safari (iOS) | **13+** (iOS 13, Sep 2019) |
| Chrome | 61+ (Sep 2017) |
| Firefox | 91+ (Aug 2021) |

**Global support: ~96%+** — guard with `if (window.visualViewport)`.

### Gotchas

- `window.innerHeight` does NOT change when keyboard opens on iOS — only `visualViewport.height` shrinks
- On Android Chrome, `window.innerHeight` DOES shrink — inconsistency is why `visualViewport` is needed
- The `resize` event fires **frequently** during keyboard animation — use `requestAnimationFrame`
- No explicit "keyboardOpen" boolean anywhere in the API — the height comparison is the standard workaround

---

## 3. `env(safe-area-inset-*)` — Notch / Home Indicator Avoidance

### What it is

`env()` reads environment variables set by the browser/OS. The `safe-area-inset-*` variables expose the physical safe area not obscured by the notch or home indicator.

| Variable | iPhone 12+ portrait |
|----------|---------------------|
| `env(safe-area-inset-top)` | ~47px |
| `env(safe-area-inset-bottom)` | ~34px (home indicator) |
| `env(safe-area-inset-left/right)` | 0px |

**Critical prerequisite:** `viewport-fit=cover` must be in the viewport meta tag. Without it all values are `0px`.

### Next.js App Router (layout.tsx)

```tsx
import type { Viewport } from 'next'

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  viewportFit: 'cover',   // ← enables safe-area-inset-* variables
}
```

### CSS Usage

```css
/* Basic — pad bottom nav above home indicator */
.bottom-bar {
  padding-bottom: env(safe-area-inset-bottom);
}

/* With fallback */
.bottom-bar {
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

/* Take the larger of safe area or a minimum value */
.bottom-bar {
  padding-bottom: max(env(safe-area-inset-bottom), 16px);
}

/* Add to existing padding */
.bottom-sheet {
  padding-bottom: calc(env(safe-area-inset-bottom) + 24px);
}
```

### Browser Support

Safari 11.1+, Chrome 69+, Firefox 65+ — **97%+ global, universally safe.**

### Gotchas

- Nothing works without `viewport-fit=cover` — single most common mistake
- In Next.js App Router, the `viewport` export is the correct and only place to set this
- `env()` values are not accessible in JavaScript directly — use a CSS custom property bridge
- Values are `0px` on Android and notch-less iPhones (SE, iPhone 8 and older)

---

## 4. The `100vh` iOS Safari Bug — History & Current Status

### The Original Bug (iOS < 15, 2017–2021)

Apple's choice: `100vh` = large viewport (address bar hidden). This caused content at the bottom of `100vh` elements to be hidden behind the address bar on page load.

### iOS 15 Change (September 2021)

Apple changed `100vh` to equal the small viewport (address bar visible) — equivalent to `100svh`. This fixed the overflow bug but `100vh` elements no longer fill the screen when the address bar retracts.

**Current status on iOS 15+:** `100vh == 100svh`. Safe to use, but not ideal for "fill the screen" layouts.

### Current Best Practice

```css
/* Old, broken */
.full-screen { height: 100vh; }

/* Correct: dynamic tracking */
.app-shell { height: 100dvh; }

/* Correct: stable minimum */
.page { min-height: 100svh; }
```

| Scenario | Recommended |
|----------|-------------|
| App shell / main layout | `100dvh` |
| "Must fit on screen" content | `100svh` |
| Background fills | `100dvh` |
| Avoid | `100vh` (ambiguous) |

---

## Key Recommendations for This Project

1. **Set `viewportFit: 'cover'` in `app/layout.tsx`** — unlocks safe-area insets; add `maximumScale: 5` to the existing viewport export
2. **Replace `100vh` with `100dvh`** in all layout containers (global search-replace)
3. **Pad fixed bottom UI** with `max(env(safe-area-inset-bottom), 8px)`
4. **Mount `ViewportProvider`** in the root layout — writes `--keyboard-height` and `--viewport-height` CSS vars; zero React re-renders

---

## Sources

- MDN: VisualViewport API — https://developer.mozilla.org/en-US/docs/Web/API/VisualViewport
- MDN: env() CSS function — https://developer.mozilla.org/en-US/docs/Web/CSS/env
- web.dev: New viewport units (Una Kravets, 2022) — https://web.dev/blog/viewport-units
- caniuse: viewport-unit-variants — https://caniuse.com/viewport-unit-variants
- caniuse: visual-viewport — https://caniuse.com/visual-viewport
- Next.js docs: Viewport — https://nextjs.org/docs/app/api-reference/functions/generate-viewport
