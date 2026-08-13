# Existing Codebase Mobile Patterns — Feature Findings

**Date:** 2026-04-07
**Scope:** `web-app/src/` — mobile-friendly patterns already in the codebase
**Purpose:** Identify what already exists so new work builds on established patterns

---

## Summary

The codebase implements 6 established mobile-friendly patterns across multiple components. Primary breakpoint is `max-width: 768px`. The `--min-touch-target: 44px` CSS var is defined globally but inconsistently applied.

**No usage found of:** `dvh`, `visualViewport`, `safe-area-inset-*`, or dynamic viewport height anywhere in the codebase.

Current `100vh`-based layout approach:
```css
/* All height calculations use this pattern — does not account for mobile keyboard */
max-height: calc(100vh - var(--header-height));
height: calc(100vh - var(--header-height));
```

---

## 1. Terminal Toolbar (Current State — No Overflow Handling)

**File:** `web-app/src/components/sessions/TerminalOutput.module.css`

The `.actions` div has no overflow handling on mobile:

```css
/* Lines 12–20 */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  background: #2d2d30;
  border-bottom: 1px solid #3e3e42;
  flex-shrink: 0;
}

/* Lines 82–85 */
.actions {
  display: flex;
  gap: 0.5rem;
  /* ← NO overflow-x: auto, NO flex-wrap, buttons will clip */
}
```

Mobile responsive styles reduce size but don't fix clipping:

```css
/* Lines 268–294 */
@media (max-width: 768px) {
  .toolbar {
    padding: 0.5rem 0.75rem;
  }
  .toolbarButton {
    padding: 0.4rem 0.6rem;
    font-size: 0.8rem;
  }
  .actions {
    gap: 0.25rem;
  }
}
```

**Gap to fix:** Add `overflow-x: auto` to `.actions` on mobile, or hide dev-only buttons behind a "More" menu.

---

## 2. Session Tab Overflow Pattern (Good Reference)

**File:** `web-app/src/components/sessions/SessionDetail` (or similar tab container)

The session detail tabs already use `overflow-x: auto` on mobile — this is the exact pattern to replicate for the terminal toolbar.

**Pattern to replicate:**
```css
@media (max-width: 768px) {
  .tabList {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none; /* hide scrollbar on mobile */
  }
  .tabList::-webkit-scrollbar {
    display: none;
  }
}
```

---

## 3. Hamburger Menu Pattern (Header.tsx)

**File:** `web-app/src/components/layout/Header.tsx` (Lines 48–58)
**CSS:** `web-app/src/components/layout/Header.module.css` (Lines 234–272, 299–325)

Full implementation with accessible markup and smooth animations:

```tsx
<button
  className={styles.hamburger}
  aria-label={isMobileMenuOpen ? "Close navigation menu" : "Open navigation menu"}
  aria-expanded={isMobileMenuOpen}
  aria-controls="mobile-nav"
  onClick={() => setIsMobileMenuOpen((prev) => !prev)}
>
  <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen1 : ""}`} />
  <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen2 : ""}`} />
  <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen3 : ""}`} />
</button>
```

CSS:
```css
.hamburger {
  display: none;
  min-width: 44px;
  min-height: 44px;
  padding: 10px;
  background: transparent;
  border: 1px solid var(--border-color, #333);
  border-radius: 0.375rem;
  cursor: pointer;
}

.hamburgerLineOpen1 { transform: translateY(7px) rotate(45deg); }
.hamburgerLineOpen2 { opacity: 0; }
.hamburgerLineOpen3 { transform: translateY(-7px) rotate(-45deg); }

@media (max-width: 768px) {
  .hamburger { display: flex; }
}
```

**Relevant to:** The mobile keyboard toggle button (show/hide `.mobileKeyboard`) should use this same toggle pattern — `useState` boolean + `aria-expanded` + click handler. Store state in `localStorage` for persistence.

---

## 4. Bottom Sheet Modal Pattern

**File:** `web-app/src/app/globals.css` (Lines 229–240)
**File:** `web-app/src/app/page.module.css` (Lines 146–182)

```css
/* globals.css */
@media (max-width: 768px) {
  .modal {
    max-height: calc(100vh - 5rem);
    border-radius: 12px 12px 0 0;
    width: 100%;
    max-width: 100%;
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
  }
}

/* page.module.css */
@media (max-width: 768px) {
  .modal {
    padding: 0;
    padding-top: var(--header-height);
  }
  .modalContent {
    max-height: calc(100vh - var(--header-height));
    height: calc(100vh - var(--header-height));
    border-radius: 0;
  }
}
```

**Note:** Both use `100vh` — will be affected by the `100dvh` migration and `safe-area-inset-bottom` additions.

---

## 5. Touch Target Sizing (44px)

**File:** `web-app/src/app/globals.css` (Line 61)

```css
--min-touch-target: 44px;  /* defined globally, inconsistently used */
```

**Used in:**

| Component | File | Implementation |
|-----------|------|----------------|
| Hamburger | `Header.module.css:235` | `min-width/height: 44px` — hardcoded, not using var |
| Nav links | `Header.module.css:320` | `min-height: 44px` on mobile |
| Session actions | `SessionCard.module.css:509` | `min-height: 44px` on mobile |
| Action buttons | `NotificationPanel.module.css:530` | `min-height: 44px` on mobile |

**Gap:** `--min-touch-target` var is defined but never referenced. Components use hardcoded `44px`. The terminal toolbar buttons do NOT apply the touch target minimum.

---

## 6. Mobile Keyboard Overlay (Current State)

**File:** `web-app/src/components/sessions/TerminalOutput.tsx` (Lines 652–666)
**CSS:** `web-app/src/components/sessions/TerminalOutput.module.css` (Lines 225–293)

Current implementation:

```tsx
<div className={styles.mobileKeyboard}>
  <div className={styles.mobileKeyRow}>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b'); }}>Esc</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\t'); }}>Tab</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x03'); }}>Ctrl+C</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x04'); }}>Ctrl+D</button>
  </div>
  <div className={styles.mobileKeyRow}>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[D'); }}>←</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[A'); }}>↑</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[B'); }}>↓</button>
    <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[C'); }}>→</button>
  </div>
</div>
```

CSS:
```css
.mobileKeyboard {
  display: none;  /* hidden on desktop */
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.4rem 0.5rem;
  background: #252526;
  border-top: 1px solid #3e3e42;
  flex-shrink: 0;
}

.mobileKey {
  flex: 1;
  padding: 0.55rem 0.4rem;
  background: #3c3c3c;
  touch-action: manipulation;
  -webkit-user-select: none;
  /* visual 3D button effect */
  border-bottom: 3px solid #333;
}

@media (max-width: 768px) {
  .mobileKeyboard {
    display: flex;
  }
}
```

**Always-visible via media query — no toggle.** This is the thing to fix: add `isKeyboardVisible` state, toggle button, and `localStorage` persistence.

Good patterns already present:
- `onPointerDown` + `e.preventDefault()` (correct for mobile)
- `touch-action: manipulation` (disables double-tap zoom)
- `-webkit-user-select: none` (prevents text selection on tap)

---

## 7. Media Query Breakpoints

Defined in `globals.css`:
```css
--breakpoint-sm: 640px;
--breakpoint-md: 768px;
--breakpoint-lg: 1024px;
--breakpoint-xl: 1280px;
```

**Primary responsive breakpoint:** `max-width: 768px` — used in:
- `globals.css` (modal, layout overrides)
- `Header.module.css` (hamburger, nav dropdown)
- `TerminalOutput.module.css` (toolbar, mobile keyboard)
- `SessionCard.module.css` (action grid, touch targets)
- `page.module.css` (modal fullscreen)
- `NotificationPanel.module.css` (approval buttons)
- 20+ additional CSS module files

**Secondary:** `prefers-color-scheme: dark/light` and `prefers-reduced-motion: reduce` — both applied globally and per-component.

---

## 8. Confirmed: No dvh / visualViewport / safe-area Usage

Comprehensive search across all of `web-app/src/`:

| Pattern | Files found |
|---------|-------------|
| `dvh` | **0** |
| `visualViewport` | **0** |
| `safe-area-inset` | **0** |
| `viewport-fit` | **0** |
| `env(` | **0** |

All height calculations currently use `100vh`:
```css
/* All height-constrained containers follow this pattern */
height: calc(100vh - var(--header-height));
max-height: calc(100vh - var(--header-height));
```

This is the root cause of the virtual keyboard issue — none of these containers shrink when the keyboard opens.

---

## Key Takeaways for Implementation

1. **Toolbar overflow:** `.actions` needs `overflow-x: auto; white-space: nowrap` on mobile — replicate the session tab pattern
2. **Keyboard toggle:** Follow the hamburger menu pattern (useState boolean, aria-expanded, localStorage); CSS class toggle on `.mobileKeyboard`
3. **Viewport height:** All `calc(100vh - ...)` expressions need updating to `calc(100dvh - ...)` or `calc(var(--viewport-height, 100dvh) - ...)`
4. **Safe area:** `globals.css` needs `env(safe-area-inset-bottom)` vars added; bottom-fixed elements need padding
5. **Touch targets:** Terminal toolbar buttons need `min-height: var(--min-touch-target, 44px)` — the var exists, use it
