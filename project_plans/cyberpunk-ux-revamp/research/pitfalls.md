# Pitfalls & Anti-Patterns: Cyberpunk UX Revamp

## 1. vanilla-extract Theme Switching Pitfalls

### FOUC on SSR (Critical)

**The problem**: `layout.tsx` currently renders `className={lightTheme}` on `<html>` at SSR time. When a user has `localStorage.theme = "matrix"`, the server-rendered HTML shows `lightTheme` class, then JavaScript runs and swaps to `matrixTheme`. This causes a visible flash.

**Magnitude**: On fast connections this is a single-frame flash. On slow connections or with large CSS files, it can be visible for hundreds of milliseconds and is jarring.

**Solution**: Inject a blocking `<script>` in `<head>` that reads localStorage and sets the class synchronously before the browser paints anything (see Architecture doc). `suppressHydrationWarning` is already on `<html>` — this handles the React hydration mismatch.

**Anti-pattern to avoid**: Conditional rendering (`if (!mounted) return null`) hides FOUC by showing nothing until JavaScript runs. This causes layout shift and is visible as a blank page flash. Do NOT use this pattern for the theme class.

**Anti-pattern to avoid**: Using `next-themes` without understanding its class vs data-attribute model. `next-themes` defaults to `data-theme` attributes, but vanilla-extract themes are CSS classes. You would need `attribute: "class"` option and exact class name matching — fragile since vanilla-extract class names are hashed. Build a lightweight custom solution instead.

### Class Name Conflicts

**The problem**: `globals.css` has `@media (prefers-color-scheme: dark)` that overrides `--background`, `--primary`, etc. When a user sets theme to "matrix" in localStorage, the vanilla-extract class correctly overrides `vars.*` tokens, but the `globals.css` legacy tokens (`--background`, `--primary`) continue to respond to OS dark mode. Components still using legacy CSS vars will diverge from the vanilla-extract theme.

**Solution**: 
1. Remove the `@media (prefers-color-scheme: dark)` block from `globals.css` once the vanilla-extract theme class fully drives all tokens.
2. Until then: scope the legacy CSS vars under the theme classes:
   ```css
   .matrixTheme { --background: #001100; --primary: #00ff41; ... }
   ```
   But this requires manual duplication — fragile.
3. Best path: migrate all remaining `globals.css` consumers to use `vars.*` references first, then delete the media query block.

**Do NOT** add new tokens to `globals.css`. All new tokens go in `theme-contract.css.ts`.

### Token Drift Between Themes

**The problem**: TypeScript will catch missing tokens (the `createTheme` call fails at build time if the contract isn't fully satisfied). But semantic misuse is silent — e.g., using `vars.color.primary` (blue) for a glow effect that should use `vars.color.glowPrimary` (neon green in Matrix theme). The wrong token is type-safe but semantically wrong.

**Solution**: Establish clear naming conventions:
- `glowPrimary` / `glowSecondary` — for neon glow effects only
- `scanlineColor` — for scanline overlay tint only
- `terminalCursor` — for terminal cursor color only
- Never repurpose existing tokens (e.g., don't use `primary` as glow color)
- Add a `/* theme: cyberpunk-only */` comment on tokens that have no meaning in the clean theme (so `cleanTheme` can set them to `transparent` / `none`)

### Build-Time vs Runtime Limits

**What vanilla-extract CANNOT do**:
- Generate CSS dynamically at runtime (it's zero-runtime by design)
- Use JavaScript expressions in keyframes (no `${Math.random()}px` in animations)
- Conditionally include CSS rules based on props (use `recipe()` variants instead)
- Reference CSS variables in `@keyframes` on all browsers equally (see Glow section below)

**What vanilla-extract CAN do that might surprise you**:
- CSS custom properties (`var(--token)`) inside `style()` resolve at paint time even though the CSS is built at build time — this is correct and expected
- `keyframes()` with `var()` references work in modern browsers (Chrome/Firefox/Safari 18+)

---

## 2. CSS Animation Performance Pitfalls

### Properties That Cause Reflow — Never Animate These

The following properties trigger layout recalculation on every frame, causing jank:

| Avoid | Use Instead |
|-------|-------------|
| `width`, `height` | `transform: scale()` |
| `top`, `left`, `right`, `bottom` | `transform: translate()` |
| `margin`, `padding` | `transform` |
| `border-width` | `outline` or `box-shadow` |
| `font-size` | `transform: scale()` |

**The session glow/pulse effect**: Use `box-shadow` with `opacity` changes — box-shadow animates on the compositor layer in modern browsers (Chrome 108+, Firefox 110+). It does NOT trigger layout. However, changing `box-shadow` spread radius does trigger a repaint (not reflow). The optimal glow animation:

```ts
// Animate opacity of a pseudo-element instead of box-shadow directly
const glowPulse = keyframes({
  "0%, 100%": { opacity: 0.4 },
  "50%": { opacity: 1 },
});

export const statusGlow = style({
  "::after": {
    content: '""',
    position: "absolute",
    inset: "-2px",
    borderRadius: "inherit",
    boxShadow: `0 0 12px 4px var(--glow-color)`,
    animation: `${glowPulse} 2s ease-in-out infinite`,
    pointerEvents: "none",
  },
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      "::after": { animation: "none", opacity: 0.6 },
    },
  },
});
```

### Scanlines Overlay Performance

The scanlines effect (CRT-style horizontal lines) is commonly implemented as:

```ts
export const scanlines = style({
  "::before": {
    content: '""',
    position: "fixed",
    top: 0, left: 0, right: 0, bottom: 0,
    backgroundImage: `repeating-linear-gradient(
      0deg,
      transparent,
      transparent 2px,
      rgba(0, 0, 0, 0.15) 2px,
      rgba(0, 0, 0, 0.15) 4px
    )`,
    pointerEvents: "none",
    zIndex: 9999,
  },
});
```

**Performance concern**: A `fixed` pseudo-element covering the entire viewport creates a new composite layer — this is GOOD for performance (it won't trigger repaints of other content). The repeating-linear-gradient is static — no animation, no reflow, minimal GPU cost.

**Anti-pattern**: Animating the scanline position (scrolling effect) with `background-position` on every frame is expensive. If you want a subtle scanline flicker, animate `opacity` only (0.1 → 0.15 → 0.1) using `will-change: opacity`.

**Anti-pattern**: Adding scanlines to every session card individually — use a single fixed overlay on `body` or the root layout element.

### `will-change` Misuse

`will-change: transform` is useful on elements that will animate imminently, but:
- DO NOT put it on all animated elements permanently — the browser allocates GPU resources upfront
- DO NOT use `will-change: *` — promotes the entire element to its own layer for no reason
- USE it only on elements that are about to animate (e.g., add/remove class before/after animation)
- The glow pseudo-element (`::after`) can have `will-change: opacity` since it pulses continuously

---

## 3. Accessibility Pitfalls with Custom Cyberpunk Themes

### Matrix Green on Black — Contrast Analysis

Matrix green (`#00ff41`) on black (`#000000`):
- Contrast ratio: ~10.3:1 — passes WCAG AA (4.5:1) and AAA (7:1)
- Good for body text

But dark green variants often used for "secondary" text (e.g., `#00aa2a` on `#000000`):
- `#00aa2a` on `#000000`: ~4.8:1 — barely passes AA, fails AAA
- `#008000` on `#000000`: ~2.6:1 — FAILS AA

**Rule**: Every text color token in each theme must be contrast-checked against its background token. Add a contrast checker script (`scripts/check-theme-contrast.ts`) to the CI pipeline as part of the token linting step.

### Cyberpunk 2077 Colors — Known Problem Colors

Cyberpunk 2077 palette often uses:
- Neon cyan (`#00ffff`) on white or light backgrounds — bright on bright, fails easily
- Hot pink (`#ff006e`) — works on dark but needs checking at small sizes
- Yellow-green on dark — often good contrast

**Anti-pattern**: Using `#ffff00` (pure yellow) on `#ffffff` (white) = 1.07:1. Catastrophically low contrast. Never use light neon on light backgrounds.

### Glow as Sole State Indicator

Box-shadow glow CANNOT be the only indicator of session status (running vs. paused vs. error). Users with `prefers-contrast: more` or color vision deficiencies may not perceive glow. Always pair glow with:
- A status badge (text label)
- An icon
- A shape difference (not just color)

The existing `statusBadge` token group already provides text-based status colors — keep these in all themes.

### Keyboard Trap in Drawer Navigation

The `ApprovalDrawer.tsx` already has focus management (`closeButtonRef.current?.focus()` on open). When implementing the collapsible nav drawer:

- When drawer opens: move focus to the first interactive element inside it
- When drawer closes: return focus to the trigger element that opened it
- Escape must close the drawer from anywhere inside it
- Tab key must cycle within the drawer when it's open as a modal (on mobile)
- On desktop (non-modal drawer): Tab should move freely through the entire page

Use `useFocusTrap.ts` only for modal drawers, not for persistent side navigation.

### Focus Management After Panel Open/Close

The existing pattern in `page.tsx` (using `useRef` + `useEffect` to track trigger element) is correct. Do not use `document.activeElement` saved in state — it's stale if the DOM changes. The `triggerRef` pattern is the right approach.

**Anti-pattern**: Auto-focusing the first element in a newly opened panel using `setTimeout(..., 0)` — timing-dependent, breaks under React concurrent features. Use `useEffect` with a ref instead.

---

## 4. View Transitions API Gotchas

### `flushSync` Incompatibility

If any code path calls `flushSync` during a navigation that triggers a View Transition, React will skip the transition animation entirely (it relies on completing asynchronously). This is not a bug — it's intentional behavior. Audit the codebase for `flushSync` calls before enabling View Transitions.

### Duplicate `view-transition-name`

If two elements have the same `view-transition-name` in the DOM simultaneously during a transition, the browser throws. Session cards will need unique names:

```ts
// Dynamic view-transition-name via inline style (not vanilla-extract, since IDs are runtime values)
<div style={{ viewTransitionName: `session-card-${session.id}` }} />
```

Do NOT use vanilla-extract for transition names that include runtime IDs — it's a build-time system.

### Back/Forward Navigation Skip

As noted in React docs: animations are skipped for `popstate`-triggered navigations (back button) unless you use the Navigation API router. Next.js App Router's native navigation uses the Navigation API — so this should be fine with `experimental.viewTransition: true`. But custom `router.push()` calls triggered from keyboard shortcuts (j/k navigation) will work correctly.

### Firefox/Safari Cross-Document

Cross-document view transitions (between pages) still require Chrome/Edge only. The project uses same-document transitions (single-page app via Next.js App Router) — so this is not a concern.

### Concurrent Mode (React 19)

`ViewTransition` animations only activate when state changes occur within a React transition (`startTransition`). Regular `useState` setters do NOT trigger View Transitions. When wiring up the omnibar open/close scanline effect:

```tsx
// Correct: use startTransition to trigger the view transition
import { startTransition } from "react";
startTransition(() => setIsOmnibarOpen(true));

// Incorrect: plain state update won't trigger view transition animation
setIsOmnibarOpen(true);
```

---

## 5. Keyboard Shortcut Pitfalls

### Browser Shortcut Conflicts

Reserved browser shortcuts that CANNOT be overridden with `preventDefault()`:
- `Cmd+N` (new window) — macOS cannot be prevented from page JS
- `Cmd+T` (new tab) — cannot be prevented
- `Cmd+W` (close tab) — cannot be prevented
- `Cmd+R` / `F5` (reload) — cannot be prevented
- `Cmd+L` (address bar focus) — cannot be prevented

Shortcuts to AVOID in the global registry:
- `Ctrl+S` — browser save (on Windows/Linux, cannot prevent)
- `Ctrl+P` — print
- `F1`-`F12` — mixed browser behaviors

Safe shortcuts to USE:
- `Cmd+K` / `Ctrl+K` (already used for omnibar — no browser conflict)
- `j`, `k` (vim nav — no browser conflict when not in input)
- `?` (help overlay — no browser conflict)
- `[`, `]` (drawer toggle — no conflict)
- `y`, `n` (approval — no conflict when not in input)
- `Escape` (modal close — already used)

### Terminal Input Conflict (Critical)

xterm.js (the terminal component) has its own key event handling. When the xterm terminal canvas has focus:
- xterm intercepts key events BEFORE they bubble to `document`
- `j` and `k` type into the terminal instead of navigating sessions

**HOWEVER**: there's a subtle edge case. If the user clicks outside the terminal but the terminal still has DOM focus via `document.activeElement`, document-level key listeners WILL still fire. The `isInputElement` check in `OmnibarContext.tsx` checks `tagName`, but the xterm canvas is not a standard input element — it's a `<canvas>` or `<div>` with `tabIndex`.

**Solution**:
1. Add a `data-context="terminal"` attribute to the terminal container div
2. In the shortcut registry dispatcher, check `document.activeElement.closest('[data-context="terminal"]')` before dispatching j/k shortcuts
3. Also check `contentEditable` elements (Monaco editor if present)

### IME Input Issues

Input Method Editors (IME) for CJK languages fire `keydown` events with `event.key === "Process"` during composition. Shortcuts registered for single characters must check `event.isComposing || event.keyCode === 229` and skip if true.

The existing `useKeyboard.ts` does NOT check for IME composition. Add:

```ts
if (event.isComposing || event.keyCode === 229) return;
```

before dispatching to handlers.

### Scattered Listener Memory Leaks

The current architecture registers `document.addEventListener("keydown", ...)` in multiple `useEffect` hooks across multiple components. If a component unmounts without calling `removeEventListener` (e.g., due to an error boundary catching a render error), the listener persists.

The existing code has correct cleanup in all `useEffect` returns — this is good. But as more shortcuts are added to more components, the risk of a missed cleanup grows.

**Solution**: The centralized registry (see Architecture doc) has a single listener at the registry level. Components register/deregister handler functions, but never touch `document.addEventListener` directly.

---

## 6. Storybook + vanilla-extract Known Issues

### HMR with Webpack in Storybook

Known issue ([GitHub #905](https://github.com/vanilla-extract-css/vanilla-extract/issues/905)): When HMR kicks in after changing vanilla-extract styles, Storybook can lose track of the component module reference, requiring a full page reload.

**Workaround**: Accept that HMR for vanilla-extract style changes in Storybook requires a reload. This is a known limitation, not a bug you can easily fix. Document this in the Storybook README so developers know.

### Vite-Based Storybook CSS File Multiplication

If the project is ever migrated to Vite-based Storybook, the `@vanilla-extract/vite-plugin` loads a new CSS file for every file that imports a theme, which can cause thousands of requests and freeze Storybook with large theme files. The `@storybook/nextjs` webpack-based framework avoids this. **Do not use Vite for Storybook in this project.**

### Decorator Ordering

In Storybook, decorator execution order is innermost-first (bottom of array to top). The `withThemeByClassName` decorator must be the OUTERMOST decorator (last in the array, or first — check `@storybook/addon-themes` documentation) to ensure the theme class is applied to the story wrapper before any child components render.

If the theme decorator wraps inside a font decorator, the theme class applies to a node that already has a rendered child — this may cause brief style flashes in Storybook previews.

### CSS Extraction in Jest

Jest does NOT extract CSS from `.css.ts` files — it mocks them (via `identity-obj-proxy` in this project's config). This means CSS-based logic (e.g., checking if a className contains a theme) CANNOT be tested in Jest. Visual correctness must be verified in Storybook/Chromatic or Playwright, not Jest.

**Anti-pattern**: `expect(element).toHaveClass(matrixTheme)` in Jest — this will always fail because `matrixTheme` is a hashed class name that `identity-obj-proxy` can't resolve. Use `data-testid` or `aria-*` attributes for Jest assertions instead.

---

## Summary: Top 10 Pitfalls to Avoid

1. **FOUC**: Use a blocking `<script>` in `<head>` to set the theme class before paint — never rely on `useEffect` for initial theme application
2. **globals.css conflict**: The `@media (prefers-color-scheme: dark)` block will fight vanilla-extract theme classes — plan its removal before shipping
3. **Glow as sole status indicator**: Always pair visual effects with text/icon indicators for accessibility
4. **Animate box-shadow spread radius**: Stick to opacity-only animations on glow pseudo-elements
5. **`will-change` overuse**: Only add `will-change: opacity` to elements that continuously animate (glow pseudo-elements); remove it from everything else
6. **Terminal j/k conflict**: Add `data-context="terminal"` and check in the shortcut dispatcher
7. **IME key events**: Add `event.isComposing` check before dispatching single-character shortcuts
8. **flushSync + View Transitions**: Audit for `flushSync` calls before enabling `experimental.viewTransition`
9. **Duplicate `view-transition-name`**: Use inline styles with runtime IDs for session card transitions, not vanilla-extract
10. **Jest class name assertions**: Use `data-testid` attributes, not vanilla-extract class names, in unit tests
