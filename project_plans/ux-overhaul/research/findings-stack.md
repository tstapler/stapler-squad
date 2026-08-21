# Stack Research: UX Overhaul

## 1. `dvh` + `visualViewport` API — Browser Support

### `100dvh` vs `dvh` on iOS
- `100dvh` (dynamic viewport height) is supported in iOS Safari **15.4+** (released March 2022).
  - Below 15.4, `100dvh` is treated like `100svh` (small viewport, excludes browser chrome) or behaves unpredictably.
  - On iOS Safari 15.4+, `100dvh` correctly reflects the height available *after* the browser toolbar shrinks/disappears during scroll.
  - **Project target (iOS 15.4+) is fully covered.**
- The `100svh` (small, always-stable) value is safe on all modern browsers for content that must never be cut off.
- `100lvh` (large viewport, including browser chrome) is generally too large and causes bottom content to be hidden.
- **Recommendation**: Use `100dvh` as the primary layout height token via the existing `--viewport-height: 100dvh` CSS custom property in `globals.css`. Supplement with `visualViewport` overrides from `ViewportProvider` for keyboard-aware adjustment.

### `visualViewport` API
- Available in Chrome 61+, Firefox 63+, Safari 13+. **Fully supported on iOS 15.4+ and Android Chrome.**
- The project already uses `visualViewport` in `ViewportProvider.tsx`:
  - Listens to both `resize` and `scroll` events (correct — iOS Safari fires `scroll` during keyboard transition).
  - Computes `--keyboard-height` as `window.innerHeight - vv.height - vv.offsetTop`.
  - Sets `--viewport-height` to `vv.height`.
- The `requestAnimationFrame` wrapping in the current implementation is correct to avoid sync layout.
- **Gap found**: `ViewportProvider` does not use `dvh` as a fallback — the root `--viewport-height: 100dvh` in `globals.css` line 87 is correct for SSR, but the JS override to `vv.height` (in px, not dvh) means the terminal layout should use `var(--viewport-height)` rather than `100dvh` directly everywhere.

## 2. `env(safe-area-inset-*)` in Next.js App Router

### Setup Status (Current)
- `layout.tsx` already has `viewportFit: "cover"` in the `Viewport` export — this is the required prerequisite for `env(safe-area-inset-*)` to have non-zero values on iOS notch devices.
- `globals.css` already defines CSS custom properties:
  ```css
  --safe-area-top: env(safe-area-inset-top, 0px);
  --safe-area-bottom: env(safe-area-inset-bottom, 0px);
  --safe-area-left: env(safe-area-inset-left, 0px);
  --safe-area-right: env(safe-area-inset-right, 0px);
  ```
- **Gap**: These variables are *defined* but not consistently *used* in layout components. `BottomNav.css.ts` must consume `var(--safe-area-bottom)` for its padding. The terminal container must apply `var(--safe-area-left)` and `var(--safe-area-right)`.

### Next.js App Router specifics
- `env()` safe area insets work in vanilla CSS, CSS Modules, and vanilla-extract `style()` alike — no special App Router treatment needed.
- In vanilla-extract `.css.ts` files, write: `paddingBottom: 'env(safe-area-inset-bottom, 0px)'` directly. vanilla-extract emits this as a static string (it does not resolve `env()` at build time — the browser resolves it at runtime, which is correct).
- **Do not** use `vars.safeAreaBottom` referencing a vanilla-extract variable — `env()` functions cannot be represented as JS values. The pattern is to use string literals for `env()` calls inside vanilla-extract.

## 3. Bottom Sheet Pattern Options

### Option A: Radix Dialog (ADR-010 already decided)
- `@radix-ui/react-dialog` version ~1.1.x (current latest).
- Does **not** natively slide up from the bottom — it renders centered by default. Requires CSS override to position bottom-aligned on mobile (`position: fixed; bottom: 0; top: auto; border-radius: 16px 16px 0 0`).
- The existing `globals.css:229-240` already has a `@media (max-width: 768px)` override that transforms Modal into a bottom sheet. This is the correct approach — no additional library needed for Milestone 1.
- Radix Dialog handles: focus trap, `aria-modal`, `role="dialog"`, keyboard dismiss (Escape), scroll lock, Portal rendering.
- **For session actions**: use the existing Modal component (which already has bottom-sheet mobile behavior) rather than introducing a new library.

### Option B: Vaul (by emilkowalski)
- `vaul` v0.9.x — purpose-built drawer/bottom-sheet primitive for React, built on top of Radix Dialog.
- Adds: swipe-to-dismiss gesture, velocity-based close, snap points, drag handle affordance.
- **Adds ~7KB gzipped.** ADR-010 calls for no new packages in Milestone 1 unless unavoidable.
- **Verdict for Milestone 1**: Do not add. The Radix Dialog bottom-sheet CSS approach is sufficient.
- **Verdict for Milestone 2**: Consider Vaul if swipe-to-dismiss gestures are required.

### Option C: Custom CSS bottom sheet
- A `position: fixed; bottom: 0; left: 0; right: 0` sheet with `transform: translateY(100%); transition: transform` and vanilla-extract `.css.ts` styles.
- Simpler but lacks accessibility (focus trap, aria-modal) unless built manually.
- **Not recommended** — Radix Dialog already provides what's needed.

## 4. Radix UI + vanilla-extract Integration

### Compatibility
- Radix UI components render with `data-*` attributes for state (`data-state="open|closed"`, `data-side`, etc.).
- vanilla-extract `style()` can target these via `selectors`:
  ```ts
  export const overlay = style({
    selectors: {
      '&[data-state="open"]': { opacity: 1 },
      '&[data-state="closed"]': { opacity: 0 },
    },
  });
  ```
- The `recipe()` API works too, but `data-*` selectors require `selectors` not `variants` (variants map to class names, not data attributes).
- **No CSS Modules needed for Radix primitives** — vanilla-extract handles all styling.

### CSS Variable Bridge
- Radix Portal renders outside the component tree; it will inherit CSS custom properties defined on `:root` or `html`.
- The `vars.*` token references in vanilla-extract resolve to CSS custom property var() calls at build time, so they work correctly inside Portals.

### Current Package Versions (web-app/package.json — to verify)
- Radix UI packages to add for Milestone 2: `@radix-ui/react-dialog`, `@radix-ui/react-dropdown-menu`, `@radix-ui/react-popover`, `@radix-ui/react-tooltip`.
- vanilla-extract packages already present: `@vanilla-extract/css`, `@vanilla-extract/recipes`, webpack/next plugin.

## 5. xterm.js Mobile/Touch Behavior

### Touch Event Handling
- xterm.js v5 (current): touch scroll works via `touchstart`/`touchmove`/`touchend` when `mouseTracking` is `'none'`.
- When `mouseTracking` is `'any'` (the default for vim/tmux), xterm forwards mouse events as escape sequences and the browser's native scroll/selection is suppressed.
- **The project correctly handles this**: `TerminalOutput.tsx` sets `mouseMode: 'none'` on mobile (detected via `max-width: 768px` or `ontouchstart`).

### iOS Safari Specifics
- `position: fixed` on the xterm container prevents rubber-band scrolling interference, but can interact poorly with `100vh`.
- xterm.js v5 uses a Canvas renderer by default. On iOS Safari 15+, Canvas 2D rendering is accelerated — no known issues at this version.
- **FitAddon + ResizeObserver**: The `FitAddon.fit()` call must be called *after* the container has a non-zero size. The project uses `ResizeObserver` via `XtermTerminal.tsx` to trigger fit — this is correct.
- **visualViewport + xterm**: The `TerminalOutput.tsx` already has a `visualViewport` resize listener (lines 572-584) that calls `fit()` after 300ms. The 300ms debounce is adequate for iOS keyboard transitions.

### Mobile Keyboard Rows
- The project has a `mobileKeyboard` section in `TerminalOutput.tsx` with two rows of extra keys (Esc, /, -, arrows, Tab, Ctrl, Alt, etc.). This is a Termux-compatible layout.
- **Touch target concern**: the `mobileKey` buttons need a `min-height: 44px` / `min-width: 44px` check. Currently enforced only via `--min-touch-target` variable which is defined in `globals.css` but not applied to `styles.mobileKey`.

## Summary

- **`dvh` support**: Project target (iOS 15.4+) is fully covered; `--viewport-height` should be the single source of truth for layout heights throughout the component tree.
- **`env(safe-area-inset-*)`**: Infrastructure is in place (`viewportFit=cover` + CSS vars) but not consistently applied in layout components — needs to be wired to `BottomNav`, `Header`, and terminal container.
- **Bottom sheet**: Use the existing Radix Dialog bottom-sheet CSS override (already present in globals.css); do not introduce Vaul in Milestone 1.
- **Radix + vanilla-extract**: Compatible; use `selectors` for `data-state` targeting; avoid CSS Modules for new Radix components.
- **xterm.js mobile**: Core setup is correct; main gap is `min-height: 44px` on mobile keyboard buttons and ensuring `--viewport-height` (not a hardcoded `100dvh`) drives the terminal container height.
