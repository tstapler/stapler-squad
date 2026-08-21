# Pitfalls Research: UX Overhaul

## 1. iOS Safari `dvh` Rounding Bugs

### The Problem
On iOS Safari 15.x, `dvh` values can exhibit sub-pixel rounding errors:
- A container set to `100dvh` may compute to `667.4px` on an iPhone 8 instead of an integer, causing a 0.4px gap at the bottom.
- This gap exposes the browser chrome or creates a visual artifact.
- The rounding issue is most visible on non-retina screens and when combining `dvh` with `calc()`.

### Mitigation
- Use `100dvh` for the container height and add `overflow: hidden` to prevent the sub-pixel gap from being visible.
- When using `calc(100dvh - var(--header-height))`, add `1px` safety margin: `calc(100dvh - var(--header-height) + 1px)` and `overflow: hidden` on the parent.
- **Do not** use `100dvh` on elements that contain scrollable content — it can cause content to overflow by the rounding delta. Use `var(--viewport-height)` (from `ViewportProvider`) instead, since that value comes from `visualViewport.height` (integer pixel value from the browser, no rounding).

### Project-Specific Risk
The project uses `var(--viewport-height)` as the primary height token, set by `ViewportProvider` to `vv.height` (integer pixels). This sidesteps the `dvh` rounding issue entirely for the terminal container. The risk is low if layout heights consistently use `var(--viewport-height)` instead of `100dvh`.

## 2. `env(safe-area-inset-bottom)` on Android

### The Problem
- On Android Chrome, `env(safe-area-inset-bottom)` is typically `0px` unless the device has a gesture navigation bar AND the app is in immersive/edge-to-edge mode.
- On Android 10+ with gesture navigation (swipe home, no buttons), the value is `24px–34px` depending on device.
- On Android with 3-button navigation bar (older pattern), the value is `0px` because the nav bar is not overlapping the app.
- On the Pixel 9 Pro Fold (project target): inner screen uses gesture navigation → `env(safe-area-inset-bottom)` ≈ `24px`. Outer screen with gesture nav ≈ `24px`.

### Mitigation
- Always provide a fallback: `padding-bottom: max(var(--safe-area-bottom), 16px)` — this gives at least 16px padding on Android with 3-button nav where the inset is 0.
- Test on both gesture-nav and 3-button-nav Android configurations.
- The `viewportFit: "cover"` in `layout.tsx` is required on Android too (not just iOS) for safe area insets to have non-zero values on devices with gesture navigation.

### Bottom Nav Specific Risk
`BottomNav` must use:
```css
padding-bottom: max(env(safe-area-inset-bottom, 0px), 8px);
```
Without the `max()`, the nav bar would have 0px bottom padding on 3-button Android devices, leaving the last nav item's tap area too close to the screen edge.

## 3. Bottom Sheet + Keyboard Interaction

### The Problem: "Sheet Pushes Up" vs "Sheet Stays Fixed"
When a bottom sheet contains a text input (e.g., rename field, tag input) and the user taps the input:
- **iOS Safari**: The virtual keyboard opens and `visualViewport.height` shrinks. If the sheet uses `position: fixed; bottom: 0`, it stays at the *page* bottom, which is now behind the keyboard. The input is hidden.
- **Android Chrome**: Same problem — the keyboard pushes the viewport up but `position: fixed` elements anchor to the initial viewport.

### Solutions
**Option A: `position: fixed; bottom: env(safe-area-inset-bottom)` + `margin-bottom: var(--keyboard-height)`**
- When the keyboard opens, `--keyboard-height` (from `ViewportProvider`) becomes non-zero.
- Setting `margin-bottom: var(--keyboard-height)` on the sheet pushes it above the keyboard.
- **Works but causes visual jump** — the sheet slides up when the keyboard appears.

**Option B: Avoid text inputs in bottom sheet**  
- For rename: open a separate modal centered in the viewport (not anchored to bottom), which floats above the keyboard naturally via `position: fixed; top: 50%; transform: translateY(-50%)`.
- **Recommended for Milestone 1**: Keep the bottom sheet for actions that don't require text input (Delete, Pause, Resume). Put Rename and Tag editing in separate focused modals.

**Option C: `transform: translateY(calc(-1 * var(--keyboard-height)))` on sheet**
- Animate the sheet upward as the keyboard opens.
- Requires listening to `--keyboard-height` changes with a CSS transition.
- More complex but provides the smoothest UX.

### Project-Specific Recommendation
For Milestone 1: Use Option B. The bottom sheet shows non-input actions (Delete, Pause/Resume, Switch Workspace). Rename and Tag editing open their existing modal components. This avoids the keyboard + sheet interaction problem entirely.

## 4. xterm.js + visualViewport Resize Loop

### The Problem
A known issue pattern with xterm.js + mobile:
1. User taps the terminal (focuses it).
2. iOS shows the software keyboard.
3. `visualViewport` resize event fires → `TerminalOutput` calls `fit()` after 300ms.
4. `fit()` resizes xterm → `FitAddon` emits a `resize` event → sends resize to backend.
5. Backend redraws the terminal at new dimensions → sends repaint.
6. Repaint causes the browser to recalculate layout → `visualViewport` may fire again.
7. Loop: steps 4-6 can repeat 2-4 times, causing flickering or layout instability.

### The Project's Current Handling
`TerminalOutput.tsx` lines 572-584:
```tsx
const onVpResize = () => {
  setTimeout(() => xtermRef.current?.fit(), 300);
};
vp.addEventListener('resize', onVpResize);
```
The 300ms debounce is the mitigation. However, if `fit()` itself triggers a `visualViewport` resize (rare but possible), there is no loop prevention.

### Mitigation
- Add a `isFittingRef = useRef(false)` guard: set to true before `fit()`, clear it after in a `requestAnimationFrame`. Skip `fit()` if already fitting.
- Increase the debounce to 400ms on iOS (use `isMobile && 400 || 300`).
- Use `ResizeObserver` on the *container* (not `visualViewport`) for the fit trigger — `ResizeObserver` fires only when the container itself changes size, which is more precise and doesn't fire during xterm internal repaints.

### Current Code Risk
The project's terminal resize loop from `XtermTerminal.tsx` uses `ResizeObserver` on the container element for the primary fit trigger — this is correct. The `visualViewport` listener is an *additional* mechanism for keyboard show/hide events. The risk of a loop from the `visualViewport` listener alone is low, but not zero.

## 5. Radix UI Portal Z-index Conflicts with xterm Canvas

### The Problem
- xterm.js renders its terminal using a `<canvas>` element (Canvas renderer) or multiple `<div>` elements (DOM renderer).
- The xterm canvas can have its own stacking context due to `transform`, `will-change`, or `position` on parent elements.
- Radix UI Portals render into a separate `div` appended to `document.body` with a z-index (typically `z-index: 50+` for Dialog/Dropdown).
- **If the xterm canvas container has a `transform` or `will-change: transform` applied** (e.g., for GPU acceleration hints or animation), it creates a new stacking context, making it impossible for Portals to render above it via z-index alone.

### Project-Specific Risk Assessment
Looking at `XtermTerminal.tsx` and `TerminalOutput.css.ts`:
- The terminal container does not currently apply `transform` or `will-change`.
- However, the `SessionDetail.css.ts` may apply animation/transition classes to the container (needs verification).
- The `styles.loadingOverlay` in `TerminalOutput` uses `position: absolute` which is within the terminal's stacking context.

### Mitigation
1. Never apply `transform`, `will-change: transform`, or `filter` to the xterm container or its ancestors unless in a `@media (prefers-reduced-motion: no-preference)` block with explicit `z-index` override on the Portal.
2. For the action bottom sheet: render it via the existing Modal component's Radix Portal mechanism, which already handles z-index correctly.
3. If animation flicker occurs: add `z-index: 1` to the `#terminal-container` and ensure the Radix Portal uses `z-index: 9999`.

## 6. `100vh` Issues on iOS Safari (Historical Context)

### The Classic Bug
`100vh` on iOS Safari has historically been the full viewport height **before** Safari's browser UI appears. When the browser shows its toolbar at the bottom, content at `100vh` is hidden behind it.

### Current Status
- iOS Safari 15.4+ introduced `dvh`/`svh`/`lvh` to fix this.
- The project already targets iOS 15.4+, so `100dvh` via `var(--viewport-height)` is correct.
- **Remaining risk**: Any hardcoded `100vh` in the codebase (not using the custom property) will exhibit the old bug on iOS. An audit of all `.css.ts` and `.module.css` files for `100vh` is needed.

### Current Occurrences to Check
In `globals.css` and component files, search for `100vh`. The `page.module.css` is specifically mentioned in requirements as having `100vh` issues — this should be migrated to `var(--viewport-height)` or `100dvh`.

## 7. Additional iOS Safari Layout Pitfalls

### Fixed Positioned Elements During Keyboard Transitions
- `position: fixed` elements on iOS Safari can "jump" during keyboard open/close transitions because the browser repaints fixed elements at the end of the scroll animation.
- The `BottomNav` is `position: fixed; bottom: 0` — this may cause a visible jump when the keyboard closes after typing in a session input field.
- **Mitigation**: Use `position: sticky` instead of `position: fixed` for the bottom nav if wrapped in an appropriate parent structure. Alternatively, accept the iOS behavior (all major apps have this issue) and focus on ensuring content is not hidden.

### Input Zoom on iOS
- iOS Safari zooms in when focusing an `<input>` with `font-size < 16px`. This zoom is not affected by `user-scalable=no`.
- The terminal's mobile keyboard buttons use `onPointerDown` (correct — prevents the iOS zoom and focus behavior).
- Text inputs in modals (rename, tag input) must have `font-size: 16px` minimum to prevent zoom.

## Summary

- **Top risk**: Bottom sheet + keyboard interaction — avoid text input fields in the bottom sheet; use separate modals for Rename and Tag editing to sidestep the keyboard-pushes-sheet problem entirely.
- **Second risk**: xterm.js visualViewport resize loop — add a `isFitting` guard ref and ensure `ResizeObserver` is the primary fit trigger (not `visualViewport` alone).
- **Third risk**: Radix Portal z-index conflicts — do not apply `transform` or `will-change` to the xterm container or its ancestors; verify `SessionDetail.css.ts` does not animate the container with transforms.
- **Mitigation checklist**: Audit for hardcoded `100vh` (replace with `var(--viewport-height)`); ensure `max()` fallback on `env(safe-area-inset-bottom)` for Android 3-button nav; ensure all touch inputs in session modals have `font-size: 16px` to prevent iOS zoom.
