# Implementation Plan: Mobile UX Improvements

**Source:** `project_plans/mobile-ux-improvements/`
**Status:** Partially Implemented — Stories 1-3 complete; Task 3.4 (iOS auto-zoom fix) pending
**Date:** 2026-04-08
**Updated:** 2026-05-02

---

## Epic Overview

### User Value

iPhone users can manage and interact with AI agent sessions without content being hidden behind the virtual keyboard, toolbar buttons clipping off-screen, or the home indicator obscuring fixed bottom UI. The supplemental mobile keyboard (Esc, Tab, Ctrl+C, arrows) becomes toggleable instead of permanently visible.

### Success Metrics

- All 8 toolbar buttons reachable on iPhone SE-width screens (375px) via horizontal scroll
- Mobile keyboard toggle persists across sessions (localStorage)
- Terminal content and toolbar remain visible when iOS virtual keyboard opens
- No content obscured by notch or home indicator (safe-area insets applied)
- xterm.js re-fits correctly after keyboard open/close (row/col count matches visible area)

### Scope

**In scope:** Viewport meta, safe-area insets, 100dvh migration, ViewportProvider, toolbar overflow, developer button hiding, touch targets, keyboard toggle, xterm.js resize on keyboard, iOS auto-zoom prevention.

**Out of scope:** Android-specific testing, PWA manifest, push notifications, TUI changes, performance optimization, full mobile redesign.

### Constraints

- iOS Safari is the primary target (iOS 15.4+ for dvh, iOS 13+ for visualViewport)
- CSS Modules only (no Tailwind, no CSS-in-JS)
- xterm.js FitAddon already has ResizeObserver — leverage, don't duplicate
- Must not break desktop layout

### Worktree Setup

```bash
git worktree add ../claude-squad-mobile-ux main -b feat/mobile-ux-improvements
```

---

## Architecture Decisions

| ADR | Title | Decision |
|-----|-------|----------|
| [ADR-001](../../project_plans/mobile-ux-improvements/decisions/ADR-001-css-variable-bridge-via-viewport-provider.md) | CSS Variable Bridge via ViewportProvider | Mount a side-effect-only component in root layout that writes `--keyboard-height` and `--viewport-height` CSS vars from `visualViewport` API. Zero React re-renders. |
| [ADR-002](../../project_plans/mobile-ux-improvements/decisions/ADR-002-resizeobserver-for-xterm-fit.md) | ResizeObserver for xterm.js fit() | Leverage existing ResizeObserver in XtermTerminal.tsx. No new visualViewport listener in terminal component. CSS var change cascades through flex layout to trigger resize. |
| [ADR-003](../../project_plans/mobile-ux-improvements/decisions/ADR-003-sticky-flex-layout-for-toolbar-keyboard-avoidance.md) | Sticky Flex Layout for Toolbar | Use flex column with `--viewport-height` height instead of `position: fixed` + keyboard offset. Sidesteps the iOS fixed-positioning keyboard bug. |
| [ADR-004](../../project_plans/mobile-ux-improvements/decisions/ADR-004-mobile-keyboard-toggle-localstorage-state.md) | Mobile Keyboard Toggle — localStorage | Persist toggle state in localStorage with key `stapler-squad-mobile-keyboard-visible`. Default: `true` (backward compatible). |

---

## Story 1: Foundation — Viewport Meta + Safe-Area Insets + 100dvh Migration

**Goal:** Establish the CSS foundation so all subsequent work can reference `--viewport-height`, `--keyboard-height`, `env(safe-area-inset-*)`, and `100dvh`.

**Acceptance Criteria:**
- `viewport-fit=cover` is set in the Next.js viewport export
- `env(safe-area-inset-bottom)` and `env(safe-area-inset-top)` are available as CSS vars in `:root`
- All layout containers use `100dvh` instead of `100vh` (with `100vh` fallback for older browsers)
- ViewportProvider component is mounted in root layout, writing `--keyboard-height` and `--viewport-height`
- Desktop layout is visually unchanged

### Task 1.1: Add `viewportFit: 'cover'` to layout.tsx viewport export

**Files:** `web-app/src/app/layout.tsx`

Add `viewportFit: 'cover'` to the existing `viewport` export object. This is the prerequisite for all `env(safe-area-inset-*)` values to be non-zero.

```typescript
export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
  viewportFit: "cover",  // <-- add this
};
```

**Verification:** View page source, confirm `<meta name="viewport" content="...viewport-fit=cover">`.

### Task 1.2: Add safe-area CSS variables to globals.css

**Files:** `web-app/src/app/globals.css`

Add safe-area-inset variables to the `:root` block (after the existing `--min-touch-target` line, around line 62):

```css
/* Safe area insets (requires viewport-fit=cover in layout.tsx) */
--safe-area-top: env(safe-area-inset-top, 0px);
--safe-area-bottom: env(safe-area-inset-bottom, 0px);
--safe-area-left: env(safe-area-inset-left, 0px);
--safe-area-right: env(safe-area-inset-right, 0px);
```

Add bottom padding to the mobile modal bottom sheet (around line 231):

```css
@media (max-width: 768px) {
  .modal {
    max-height: calc(100dvh - 5rem);  /* also 100vh -> 100dvh */
    /* ... existing rules ... */
    padding-bottom: var(--safe-area-bottom);
  }
}
```

**Verification:** On iPhone with notch, bottom sheet content is not obscured by home indicator.

### Task 1.3: Replace 100vh with 100dvh across layout containers

**Files:**
- `web-app/src/app/page.module.css` (lines 2, 67-68, 157-158, 163-164)
- `web-app/src/app/globals.css` (line 231)
- `web-app/src/app/sessions/new/page.module.css` (line 2)
- `web-app/src/app/review-queue/page.module.css` (lines 4, 41-42, 54-55, 72-73)
- `web-app/src/app/rules/page.module.css` (line 4)
- `web-app/src/app/login/login.module.css` (line 2)
- `web-app/src/app/history/history.module.css` (line 9)
- `web-app/src/components/history/HistoryGroupView.module.css` (line 62)
- `web-app/src/components/history/HistoryDetailPanel.module.css` (line 9)
- `web-app/src/components/ui/NotificationToast.module.css` (line 6)

**Skip (test/debug pages, not user-facing):**
- `web-app/src/app/test/escape-codes/page.module.css`
- `web-app/src/app/debug/escape-codes/page.module.css`
- `web-app/src/app/test/layout-overlap/page.tsx` (inline styles in test harness)
- `web-app/src/app/test/terminal-stress/page.tsx`

Pattern: add `100vh` as fallback, then `100dvh` as override:
```css
/* Before */
min-height: 100vh;

/* After */
min-height: 100vh;   /* fallback for older browsers */
min-height: 100dvh;
```

For `calc()` expressions:
```css
/* Before */
height: calc(100vh - var(--header-height));

/* After */
height: calc(100vh - var(--header-height));   /* fallback */
height: calc(100dvh - var(--header-height));
```

For modal content on mobile (the keyboard-aware containers), use `--viewport-height` instead:
```css
/* page.module.css mobile modal — keyboard-aware */
height: calc(var(--viewport-height, 100dvh) - var(--header-height));
```

**Verification:** On desktop, no visible change. On iOS Safari, address bar retract/expand causes layout to track smoothly.

### Task 1.4: Create and mount ViewportProvider component

**Files:**
- `web-app/src/components/providers/ViewportProvider.tsx` (new file)
- `web-app/src/app/layout.tsx` (import and mount)

ViewportProvider is a client component that renders `null`. It writes two CSS custom properties on `document.documentElement`:

```typescript
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

Mount in `layout.tsx` inside `<body>`, before other content:

```tsx
<body>
  <ViewportProvider />
  <ErrorBoundary>
    ...
  </ErrorBoundary>
</body>
```

**Verification:** Open DevTools on iOS, inspect `:root` style — `--keyboard-height` and `--viewport-height` should be present. Open virtual keyboard — values should update.

### Story 1 Integration Checkpoint

- [x] `viewportFit: 'cover'` in viewport meta tag (layout.tsx)
- [x] `--safe-area-*` CSS vars defined in globals.css
- [x] All user-facing 100vh replaced with 100dvh (with fallback)
- [x] Modal content on mobile uses `var(--viewport-height, 100dvh)`
- [x] ViewportProvider mounted, writes CSS vars on keyboard open/close
- [ ] Desktop layout visually unchanged (screenshot comparison)
- [ ] iOS Safari: address bar changes tracked by dvh
- [ ] iOS Safari: virtual keyboard open/close updates --viewport-height

---

## Story 2: Terminal Toolbar — Overflow Scroll + Developer-Mode Button Hiding

**Goal:** All toolbar buttons are reachable on mobile. Developer-only buttons (Debug, Record, streaming mode) are hidden behind width-conditional rendering or an overflow menu.

**Acceptance Criteria:**
- Toolbar `.actions` scrolls horizontally on mobile (all buttons reachable)
- Debug, Record, and streaming mode selector are hidden on mobile (<=768px)
- Remaining buttons (Resize, Clear, Bottom, Copy) are accessible and meet 44px touch target minimum
- Scrollbar is hidden but scroll is functional (momentum scrolling on iOS)

### Task 2.1: Add horizontal overflow scroll to .actions on mobile

**Files:** `web-app/src/components/sessions/TerminalOutput.module.css`

Add to the existing `@media (max-width: 768px)` block (around line 291):

```css
.actions {
  gap: 0.25rem;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  white-space: nowrap;
  scrollbar-width: none;           /* Firefox */
  -ms-overflow-style: none;        /* IE/Edge */
}

.actions::-webkit-scrollbar {
  display: none;                   /* Chrome/Safari */
}
```

This replicates the existing session tab overflow pattern documented in findings-features.md Section 2.

**Verification:** On 375px viewport, swipe horizontally on toolbar to reach all buttons.

### Task 2.2: Hide developer-only buttons on mobile

**Files:**
- `web-app/src/components/sessions/TerminalOutput.tsx` (lines 553-591)
- `web-app/src/components/sessions/TerminalOutput.module.css`

Add a CSS class `.devOnly` to the Debug button, Record button, and streaming mode `<select>`:

```tsx
<button className={`${styles.toolbarButton} ${styles.devOnly} ${debugMode ? styles.debugActive : ''}`} ...>
<button className={`${styles.toolbarButton} ${styles.devOnly}`} ...>
<select className={`${styles.toolbarButton} ${styles.devOnly}`} ...>
```

CSS:
```css
@media (max-width: 768px) {
  .devOnly {
    display: none;
  }
}
```

This hides Debug, Record, and streaming mode selector on mobile. If a developer needs these on mobile in the future, they can be moved behind a "More" overflow menu — but that is out of scope for this story.

**Verification:** On mobile viewport, only Reconnect (conditional), Resize, Clear, Bottom, Copy are visible.

### Task 2.3: Apply 44px touch target to toolbar buttons on mobile

**Files:** `web-app/src/components/sessions/TerminalOutput.module.css`

In the `@media (max-width: 768px)` block, update `.toolbarButton`:

```css
.toolbarButton {
  padding: 0.4rem 0.6rem;
  font-size: 0.8rem;
  min-height: var(--min-touch-target, 44px);
  min-width: var(--min-touch-target, 44px);
}
```

This uses the globally-defined `--min-touch-target` CSS var (globals.css line 61) that was previously unused by toolbar buttons.

**Verification:** Inspect toolbar buttons on mobile — each has at least 44x44px hit area.

### Story 2 Integration Checkpoint

- [x] Toolbar scrolls horizontally on 375px viewport (overflow menu implemented via ··· button, ef342b6)
- [x] Debug, Record, streaming mode hidden on mobile
- [x] Remaining buttons have 44px minimum touch target
- [ ] Scrollbar is hidden but functional
- [x] Desktop toolbar unchanged (all buttons visible, no scroll)

---

## Story 3: Mobile Keyboard — Toggle + xterm.js Keyboard-Aware Resize

**Goal:** The supplemental mobile keyboard (Esc, Tab, Ctrl+C, Ctrl+D, arrows) has a toggle button. The xterm.js terminal re-fits correctly when the iOS virtual keyboard opens/closes.

**Acceptance Criteria:**
- Toggle button visible on mobile, positioned in toolbar area
- Toggle state persists in localStorage (key: `stapler-squad-mobile-keyboard-visible`)
- Default state: visible (backward compatible with current always-on behavior)
- Toggle button has `aria-expanded`, `aria-label`, 44px touch target
- xterm.js `fit()` fires correctly after keyboard open/close (via existing ResizeObserver)
- iOS auto-zoom prevented on xterm textarea (font-size: 16px)

### Task 3.1: Add isKeyboardVisible state and toggle button

**Files:** `web-app/src/components/sessions/TerminalOutput.tsx`

Add state with localStorage persistence (near other state declarations, around line 94):

```typescript
const [isKeyboardVisible, setIsKeyboardVisible] = useState(() => {
  if (typeof window === 'undefined') return true;
  try {
    const stored = localStorage.getItem('stapler-squad-mobile-keyboard-visible');
    return stored === null ? true : stored === 'true';
  } catch {
    return true;
  }
});

const toggleMobileKeyboard = useCallback(() => {
  setIsKeyboardVisible(prev => {
    const next = !prev;
    try {
      localStorage.setItem('stapler-squad-mobile-keyboard-visible', String(next));
    } catch {
      // localStorage full or disabled — continue without persistence
    }
    return next;
  });
}, []);
```

Add toggle button in the toolbar `.actions` div (before the mobile keyboard div, visible only on mobile):

```tsx
<button
  className={`${styles.toolbarButton} ${styles.mobileKeyboardToggle}`}
  onClick={toggleMobileKeyboard}
  aria-label={isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard"}
  aria-expanded={isKeyboardVisible}
  title={isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard"}
>
  ⌨️ {isKeyboardVisible ? 'Hide Keys' : 'Show Keys'}
</button>
```

Conditionally render the mobile keyboard div:

```tsx
{isKeyboardVisible && (
  <div className={styles.mobileKeyboard}>
    {/* existing key rows unchanged */}
  </div>
)}
```

**Verification:** On mobile, toggle button shows/hides keyboard overlay. Refresh page — preference persists.

### Task 3.2: Style the toggle button and conditional keyboard display

**Files:** `web-app/src/components/sessions/TerminalOutput.module.css`

```css
.mobileKeyboardToggle {
  display: none;  /* hidden on desktop */
}

@media (max-width: 768px) {
  .mobileKeyboardToggle {
    display: inline-flex;
    align-items: center;
    min-height: var(--min-touch-target, 44px);
    min-width: var(--min-touch-target, 44px);
  }
}
```

Update the existing `.mobileKeyboard` media query rule. Instead of unconditionally showing on mobile, the visibility is now controlled by React state (conditional rendering in TSX). Remove the media query `display: flex` override since React handles it:

```css
/* Before: */
@media (max-width: 768px) {
  .mobileKeyboard {
    display: flex;
  }
}

/* After: */
@media (max-width: 768px) {
  .mobileKeyboard {
    display: flex;
    padding-bottom: max(var(--safe-area-bottom, 0px), 0.4rem);
  }
}
```

The `.mobileKeyboard` base style stays `display: none` for desktop. On mobile, when React renders the element (controlled by `isKeyboardVisible`), the media query sets it to `display: flex`. When `isKeyboardVisible` is false, the element is not in the DOM at all.

**Verification:** Toggle button is 44px tall. Keyboard overlay has safe-area bottom padding on notched iPhones.

### Task 3.3: Add double-rAF to XtermTerminal ResizeObserver for iOS reliability

**Files:** `web-app/src/components/sessions/XtermTerminal.tsx` (lines 288-293)

The existing ResizeObserver debounce uses `setTimeout`. For the initial resize events (which include keyboard open/close transitions), wrap `fit()` in double `requestAnimationFrame` to ensure DOM reflow is complete before measuring:

```typescript
// Current (line 289-291):
resizeTimeout = setTimeout(() => {
  fitAddonRef.current?.fit();
}, debounceDelay);

// Updated:
resizeTimeout = setTimeout(() => {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      fitAddonRef.current?.fit();
    });
  });
}, debounceDelay);
```

This addresses the iOS pitfall where `fitAddon.fit()` measures wrong dimensions because the DOM hasn't reflowed yet (findings-pitfalls.md Section 4C).

**Verification:** Open iOS keyboard in terminal view — terminal re-fits to correct row/col count without misalignment.

### Task 3.4: Prevent iOS auto-zoom on xterm textarea

**Files:** `web-app/src/components/sessions/TerminalOutput.module.css` (or `XtermTerminal.module.css`)

Add a global rule targeting xterm's hidden input textarea:

```css
/* Prevent iOS auto-zoom when xterm's hidden textarea gains focus */
:global(.xterm-helper-textarea) {
  font-size: 16px !important;
}
```

iOS auto-zooms when any input with `font-size < 16px` is focused. xterm's hidden textarea inherits the terminal's font size (typically 13-14px), triggering this behavior.

**Verification:** Tap terminal on iOS — page does not zoom in.

### Story 3 Integration Checkpoint

- [x] Toggle button visible on mobile, hidden on desktop
- [x] Toggle persists in localStorage after page refresh
- [x] Default state is visible (matches current behavior)
- [x] Toggle has aria-expanded and aria-label
- [x] xterm.js re-fits after keyboard open/close (double rAF)
- [ ] No iOS auto-zoom when tapping terminal (Task 3.4 pending — xterm-helper-textarea font-size:16px fix)
- [x] Desktop terminal behavior unchanged

---

## Known Issues — Proactive Bug Identification

### BUG-001: dvh Does NOT Update on Keyboard Open [SEVERITY: High]

**Description:** `100dvh` only responds to browser chrome (address bar) changes, not the virtual keyboard. Code that uses `100dvh` alone for keyboard-aware layout will fail — the container will not shrink when the keyboard opens.

**Mitigation:** All keyboard-aware containers (modal content on mobile, terminal container) must use `var(--viewport-height, 100dvh)` instead of bare `100dvh`. The `100dvh` fallback only handles the address-bar case; `--viewport-height` (from ViewportProvider) handles the keyboard case.

**Files Affected:**
- `web-app/src/app/page.module.css` (mobile modal height)
- Any container that needs to shrink when keyboard opens

**Prevention:** Grep for bare `100dvh` in mobile media queries during code review. If the container holds interactive content (inputs, terminal), it must use `--viewport-height`.

### BUG-002: Must Listen to BOTH resize AND scroll on visualViewport [SEVERITY: High]

**Description:** On iOS Safari, the `visualViewport` `scroll` event fires during keyboard transitions alongside `resize`. Listening only to `resize` causes missed updates — the toolbar may not reposition, and `--viewport-height` may be stale.

**Mitigation:** ViewportProvider must register both event listeners. This is already specified in Task 1.4.

**Files Affected:**
- `web-app/src/components/providers/ViewportProvider.tsx`

**Prevention:** Comment in ViewportProvider explaining why both listeners are required.

### BUG-003: fitAddon.fit() Needs Double requestAnimationFrame on iOS [SEVERITY: High]

**Description:** Calling `fitAddon.fit()` in a `visualViewport` resize handler or `ResizeObserver` callback gives wrong dimensions because the DOM hasn't reflowed yet. A single `requestAnimationFrame` is insufficient on iOS — the browser may batch the rAF with the resize, so the DOM reflow hasn't completed. Double rAF ensures one full frame passes.

**Mitigation:** Task 3.3 wraps the existing `fit()` call in double rAF.

**Files Affected:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (ResizeObserver callback)

**Prevention:** Add comment explaining the double-rAF pattern and linking to xterm.js issue #3895.

### BUG-004: position:fixed Toolbar Goes Under Keyboard [SEVERITY: Medium]

**Description:** If any future change adds `position: fixed; bottom: 0` to the toolbar or mobile keyboard overlay, it will be obscured by the iOS virtual keyboard. iOS's layout viewport does not shrink when the keyboard opens, so fixed elements anchor behind the keyboard.

**Mitigation:** ADR-003 mandates sticky flex layout instead of `position: fixed`. No fixed positioning is used for bottom-anchored elements in the terminal view.

**Prevention:** Code review rule: no `position: fixed; bottom: *` in terminal-related CSS on mobile.

### BUG-005: viewportFit: 'cover' Required Before Safe-Area Insets Work [SEVERITY: Medium]

**Description:** `env(safe-area-inset-*)` values are all `0px` unless the viewport meta tag includes `viewport-fit=cover`. If Task 1.1 is skipped or reverted, all safe-area padding silently fails.

**Mitigation:** Task 1.1 is the first task and a dependency for all safe-area work.

**Prevention:** Integration checkpoint verifies `viewport-fit=cover` is in the rendered meta tag.

### BUG-006: iOS Auto-Zoom on xterm Textarea Focus [SEVERITY: Medium]

**Description:** xterm.js creates a hidden `<textarea>` for keyboard input. Its font-size inherits from the terminal (typically 13-14px). iOS Safari auto-zooms the page when any input with font-size < 16px is focused. This displaces the terminal canvas and breaks the layout.

**Mitigation:** Task 3.4 sets `font-size: 16px !important` on `.xterm-helper-textarea`.

**Prevention:** The CSS rule uses `:global()` to target the xterm class regardless of CSS Modules scoping.

### BUG-007: localStorage Hydration Mismatch [SEVERITY: Low]

**Description:** `isKeyboardVisible` reads localStorage on the client but defaults to `true` during SSR. If the stored value is `false`, the server-rendered HTML includes the keyboard overlay but the client removes it on hydration, causing a flash.

**Mitigation:** Acceptable because the mobile keyboard div is inside a `@media (max-width: 768px)` block that is `display: none` on desktop. Server render targets desktop; mobile users get the correct state on hydration. The flash (if any) is invisible because the element is CSS-hidden during SSR.

**Prevention:** If this becomes visible, wrap the keyboard overlay in a `useEffect`-gated client-only render.

---

## Dependency Visualization

```
Task 1.1 (viewportFit: cover)
  │
  ├── Task 1.2 (safe-area CSS vars)     ← depends on 1.1
  │     │
  │     └── Task 3.2 (keyboard padding) ← uses --safe-area-bottom
  │
  └── Task 1.4 (ViewportProvider)        ← independent of 1.2
        │
        ├── Task 1.3 (100dvh migration)  ← uses --viewport-height for mobile modal
        │     │
        │     └── Story 3 (all tasks)    ← terminal container must use --viewport-height
        │
        └── Story 2 (all tasks)          ← independent, can run in parallel with 1.3

Task 2.1 (toolbar scroll)    ─┐
Task 2.2 (hide dev buttons)  ─┤── independent of each other
Task 2.3 (touch targets)     ─┘

Task 3.1 (toggle state)      ─┐
Task 3.2 (toggle CSS)        ─┤── 3.2 depends on 3.1
Task 3.3 (double rAF)        ─┤── independent
Task 3.4 (auto-zoom fix)     ─┘── independent
```

**Critical path:** 1.1 → 1.4 → 1.3 → 3.1 → 3.2

**Parallelizable:** Story 2 (all tasks) can run in parallel with Tasks 1.3 and 1.4 after Task 1.1 is complete.

---

## Integration Checkpoints

### After Story 1 (Foundation)

1. Open app on iPhone — no content behind notch or home indicator
2. Scroll page — address bar retract/expand causes layout to track (dvh working)
3. Open virtual keyboard — `--viewport-height` updates (check DevTools)
4. Desktop browser — no visual change from before

### After Story 2 (Toolbar)

1. Open terminal on 375px viewport — swipe toolbar to see all buttons
2. Debug, Record, streaming mode are not visible on mobile
3. Resize, Clear, Bottom, Copy buttons are 44px tall
4. Desktop — all buttons visible, no scroll

### After Story 3 (Keyboard)

1. Open terminal on iPhone — toggle button visible in toolbar
2. Tap toggle — keyboard overlay hides; tap again — it shows
3. Refresh page — toggle state persists
4. Tap terminal — no auto-zoom
5. Open iOS keyboard — terminal re-fits to smaller area, toolbar stays visible
6. Close iOS keyboard — terminal re-fits to full area

---

## Context Preparation Guide

### Files to Read Before Implementation

**Must read (understand before touching):**
- `web-app/src/components/sessions/TerminalOutput.tsx` — toolbar + mobile keyboard JSX
- `web-app/src/components/sessions/TerminalOutput.module.css` — all styles for above
- `web-app/src/components/sessions/XtermTerminal.tsx` — ResizeObserver + fitAddon (lines 254-310)
- `web-app/src/app/layout.tsx` — viewport export + component tree
- `web-app/src/app/globals.css` — CSS vars, mobile modal styles
- `web-app/src/app/page.module.css` — modal height calculations

**Reference (patterns to replicate):**
- `web-app/src/components/layout/Header.tsx` — hamburger toggle pattern (lines 48-58)
- `web-app/src/components/layout/Header.module.css` — 44px touch target, aria-expanded (lines 234-272)

**ADRs (decisions already made):**
- `project_plans/mobile-ux-improvements/decisions/ADR-001-css-variable-bridge-via-viewport-provider.md`
- `project_plans/mobile-ux-improvements/decisions/ADR-002-resizeobserver-for-xterm-fit.md`
- `project_plans/mobile-ux-improvements/decisions/ADR-003-sticky-flex-layout-for-toolbar-keyboard-avoidance.md`
- `project_plans/mobile-ux-improvements/decisions/ADR-004-mobile-keyboard-toggle-localstorage-state.md`

### Research Files

- `project_plans/mobile-ux-improvements/research/findings-stack.md` — dvh/svh, visualViewport, safe-area-inset, 100vh history
- `project_plans/mobile-ux-improvements/research/findings-features.md` — existing codebase patterns
- `project_plans/mobile-ux-improvements/research/findings-architecture.md` — ViewportProvider vs Context vs CSS-only
- `project_plans/mobile-ux-improvements/research/findings-pitfalls.md` — iOS Safari bugs and workarounds

### Key iOS Safari Facts for Implementer

1. `dvh` does NOT update on keyboard open — only address bar. Use `--viewport-height` from ViewportProvider.
2. Must listen to BOTH `resize` AND `scroll` on `visualViewport` on iOS.
3. `fitAddon.fit()` needs double `requestAnimationFrame` — single rAF insufficient on iOS.
4. `position: fixed` toolbar goes under keyboard unless using flex layout with `--viewport-height`.
5. `viewportFit: 'cover'` is required before any `env(safe-area-inset-*)` values are non-zero.
6. xterm.js hidden textarea needs `font-size: 16px` or iOS auto-zooms.
