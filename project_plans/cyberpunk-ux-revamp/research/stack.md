# Stack Research: Cyberpunk UX Revamp

## 1. vanilla-extract `createTheme` Multi-Theme Patterns

### Current State
The project already uses `createThemeContract` + `createTheme` with two themes (`lightTheme`, `darkTheme`) in `web-app/src/styles/theme.css.ts`. The contract (`theme-contract.css.ts`) defines 100+ typed tokens across color, statusBadge, font, space, radii, fontSize, fontWeight, and shadow groups.

The existing `ThemeProvider.tsx` applies the theme as a class on `document.documentElement` by toggling between `lightTheme` and `darkTheme` CSS class names. It currently follows `prefers-color-scheme` only — no localStorage persistence.

### Pattern for 4 Themes

**Step 1: Create 2 additional theme objects.** Each calls `createTheme(vars, { ... })` against the same `vars` contract. This produces a new CSS class that re-assigns all contract tokens. The class name is the only thing you switch at runtime.

```ts
// theme.css.ts
export const matrixTheme  = createTheme(vars, { color: { background: "#001100", ... }, ... });
export const cyberpunk77Theme = createTheme(vars, { ... });
export const wh40kTheme   = createTheme(vars, { ... });
export const cleanTheme   = createTheme(vars, { ... }); // existing lightTheme renamed
```

**Step 2: localStorage persistence.** Store the theme name (e.g. `"matrix"`) and resolve to the corresponding CSS class at runtime.

```ts
type ThemeName = "matrix" | "cyberpunk77" | "wh40k" | "clean";
const THEME_CLASSES: Record<ThemeName, string> = {
  matrix: matrixTheme,
  cyberpunk77: cyberpunk77Theme,
  wh40k: wh40kTheme,
  clean: cleanTheme,
};
const DEFAULT_THEME: ThemeName = "matrix";

const stored = (localStorage.getItem("theme") as ThemeName) ?? DEFAULT_THEME;
document.documentElement.classList.add(THEME_CLASSES[stored]);
```

**Step 3: Persisted switching.** Theme switch logic in `ThemeProvider.tsx`:

```ts
function applyTheme(name: ThemeName) {
  const html = document.documentElement;
  Object.values(THEME_CLASSES).forEach(cls => html.classList.remove(cls));
  html.classList.add(THEME_CLASSES[name]);
  localStorage.setItem("theme", name);
}
```

**Step 4: SSR / FOUC prevention.** Apply theme before hydration using a `<script>` tag injected into `<head>` in `layout.tsx` via `dangerouslySetInnerHTML`. This runs synchronously before React hydrates, preventing flash (see Pitfalls doc for full strategy). The html element in `layout.tsx` currently has `suppressHydrationWarning` already set — extend this.

### assignInlineVars for Runtime-Dynamic Tokens

For tokens that can't be known at build time (e.g., per-session accent color), use `assignInlineVars` from `@vanilla-extract/dynamic`. This is already installed as part of `@vanilla-extract/recipes`.

```ts
import { assignInlineVars } from "@vanilla-extract/dynamic";
<div style={assignInlineVars({ [vars.color.primary]: customAccentColor })} />
```

### Theme Contract Extensions Needed for Cyberpunk Themes

The existing contract covers all needed semantic tokens. Extensions to add:
- `vars.color.glowPrimary` — for glow box-shadow colors (neon green in Matrix, cyan in Cyberpunk)
- `vars.color.glowSecondary` — secondary glow accent
- `vars.color.scanlineColor` — tint for scanline overlay
- `vars.color.terminalCursor` — theme-specific cursor color
- `vars.font.display` — optional display/title font (may differ per theme)

Add these to `theme-contract.css.ts` before implementing themes.

---

## 2. CSS Effects in vanilla-extract

### What is possible at build time

All CSS that doesn't depend on runtime values can be expressed in vanilla-extract. This includes:

- **Keyframe animations** via `keyframes()`: `from`/`to` and percentage stops fully supported.
- **box-shadow glow**: Static glow values work perfectly. `box-shadow: 0 0 8px 2px #00ff41` is a build-time constant. Pulsing glow = keyframe that animates box-shadow opacity via color alpha.
- **Scanlines overlay**: A `::before` or `::after` pseudo-element with `repeating-linear-gradient` and `background-size` set to a few pixels, positioned `fixed` over the viewport, `pointer-events: none`. Fully build-time in vanilla-extract using `globalStyle`.
- **Glitch animation**: Keyframe animation shifting `transform: translateX()` and `clip-path` at random percentage stops — fully expressible.

### What requires runtime values

- **Per-theme glow color in animations**: Keyframes in vanilla-extract don't interpolate CSS custom properties in `box-shadow` on all browsers (Safari has partial support). The workaround is to use `var(--glow-color)` inside `keyframes()` — this works because vanilla-extract emits real CSS, and CSS custom properties resolve at paint time. Confirmed working pattern:

```ts
const pulseGlow = keyframes({
  "0%": { boxShadow: `0 0 4px 1px var(--glow-color)` },
  "50%": { boxShadow: `0 0 12px 3px var(--glow-color)` },
  "100%": { boxShadow: `0 0 4px 1px var(--glow-color)` },
});
```

- **Scanlines intensity/color**: Use a CSS custom property (`vars.color.scanlineColor`) as the gradient color. The gradient itself is built at build time; only the color token changes per theme.

### `prefers-reduced-motion`

Wrap all animations in a media query check in vanilla-extract:

```ts
export const glowingElement = style({
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animation: `${pulseGlow} 2s ease-in-out infinite`,
    },
  },
});
```

---

## 3. View Transitions API

### Browser Support (as of May 2026)

- **Same-document transitions**: Chrome 111+, Edge 111+, Firefox 133+, Safari 18+. Reached Baseline Newly Available in October 2025.
- **Cross-document transitions**: Chrome 126+, Edge 126+ only (no Firefox/Safari).

For a localhost dev tool, same-document transitions are sufficient and broadly supported.

### Next.js App Router Integration

Next.js 15 has native viewTransition support. Enable in `next.config.js`:

```js
module.exports = { experimental: { viewTransition: true } };
```

React 19.2 added `<ViewTransition>` component. Route transitions trigger automatically when `viewTransition: true` is set. React uses feature detection on `document.startViewTransition` — when unavailable, navigation happens immediately without animation (graceful degradation).

For the scanline omnibar open effect specifically: wrap the omnibar open/close in `document.startViewTransition()` manually for the micro-interaction. The Omnibar.css.ts already has `slideDown` and `fadeIn` keyframes — these can be enhanced to use view-transition-name for seamless morphing.

### `view-transition-name` gotcha

Elements with the same `view-transition-name` cannot be duplicated in the DOM during a transition. Session cards in a list must have unique names (e.g. `view-transition-name: session-card-${id}`).

### No polyfill needed

Do not add a polyfill — React's implementation handles degradation internally.

---

## 4. JetBrains Mono via next/font

### Setup

JetBrains Mono is available as a variable font via Fontsource (`@fontsource-variable/jetbrains-mono`) or via `next/font/google`.

**Recommended: `next/font/google`** (zero additional npm package, automatic font optimization):

```ts
// app/fonts.ts
import { JetBrains_Mono } from "next/font/google";

export const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-jetbrains-mono",
  display: "swap",
  axes: ["ital"],
});
```

Apply in `layout.tsx`:

```tsx
<html className={`${jetbrainsMono.variable} ${themeClass}`}>
```

### Integration with vanilla-extract

Update `theme.css.ts` `sharedTokens.font.mono` to use the CSS variable:

```ts
font: {
  mono: "var(--font-jetbrains-mono), 'Monaco', 'Menlo', monospace",
}
```

Then `vars.font.mono` automatically uses JetBrains Mono in all components that reference it.

**Alternative: `@fontsource-variable/jetbrains-mono`**

```ts
import "@fontsource-variable/jetbrains-mono";
// Then in theme.css.ts:
font: { mono: "'JetBrains Mono Variable', monospace" }
```

The `next/font/google` approach is preferred because it handles subsetting, preloading, and CORS automatically.

---

## 5. Storybook 8 with vanilla-extract

### Configuration

Storybook 8 with Next.js framework uses the `@storybook/nextjs` framework package which includes webpack/RSPack. The `@vanilla-extract/next-plugin` must also be added to the Storybook webpack config.

`.storybook/main.ts`:

```ts
import type { StorybookConfig } from "@storybook/nextjs";
const config: StorybookConfig = {
  framework: "@storybook/nextjs",
  addons: ["@storybook/addon-themes", "@chromatic-com/storybook"],
};
export default config;
```

### addon-themes for Multi-Theme Stories

`@storybook/addon-themes` provides `withThemeByClassName` decorator:

```ts
// .storybook/preview.ts
import { withThemeByClassName } from "@storybook/addon-themes";
import { matrixTheme, cyberpunk77Theme, wh40kTheme, cleanTheme } from "../web-app/src/styles/theme.css";

export const decorators = [
  withThemeByClassName({
    themes: { matrix: matrixTheme, "cyberpunk-77": cyberpunk77Theme, "wh40k": wh40kTheme, clean: cleanTheme },
    defaultTheme: "matrix",
  }),
];
```

This applies the theme class to the story root element and adds a toolbar dropdown.

### Known HMR Issues

- With Vite-based Storybook: the vite plugin can load CSS files multiple times when themes are active, potentially freezing Storybook. Webpack-based Storybook (the Next.js default) is generally more stable.
- Style changes during HMR may require a full page reload.
- Decorator changes during HMR may not re-render stories; a page reload is needed.

### Chromatic Integration

Install `@chromatic-com/storybook`. Chromatic captures every story in every theme variant, catching regressions automatically. Enable with:

```bash
npx chromatic --project-token=<token> --auto-accept-changes
```

In Storybook stories, add `parameters.themes.themeOverride` per story to test specific themes.

---

## 6. Playwright Visual Regression

### `toHaveScreenshot()` Basics

```ts
await expect(page).toHaveScreenshot("session-list-matrix.png", {
  maxDiffPixelRatio: 0.02,
  animations: "disabled",
});
```

### Multi-Theme Setup

Use Playwright projects to run snapshot tests for each theme:

```ts
// playwright.config.ts
export default defineConfig({
  projects: [
    { name: "matrix-theme",     use: { storageState: "tests/fixtures/matrix-theme.json" } },
    { name: "cyberpunk77-theme",use: { storageState: "tests/fixtures/cyberpunk77-theme.json" } },
    { name: "wh40k-theme",      use: { storageState: "tests/fixtures/wh40k-theme.json" } },
    { name: "clean-theme",      use: { storageState: "tests/fixtures/clean-theme.json" } },
  ],
});
```

Each `storageState` fixture sets `localStorage.theme` to the desired theme name. The snapshot files are organized as `[snapshotDir]/[testName]/[projectName]/[snapshotName]`.

### Snapshot path template

Set in `playwright.config.ts`:

```ts
snapshotPathTemplate: "tests/snapshots/{projectName}/{testFilePath}/{arg}{ext}",
```

### Threshold Configuration

For cyberpunk themes with glow animations, disable animations in visual tests:

```ts
use: { actionTimeout: 5000 },
// Per-test:
await page.emulateMedia({ reducedMotion: "reduce" });
```

Set `maxDiffPixelRatio: 0.01` for strict theme tests, `0.03` for components with subtle gradients.

### Updating Baselines

```bash
npx playwright test --update-snapshots --project=matrix-theme
```

Run baseline updates only intentionally; CI should fail on unexpected diffs.

---

## Current Dependencies Assessment

The project already has:
- `@vanilla-extract/css: ^1.20.1` — core styling
- `@vanilla-extract/recipes: ^0.5.7` — recipe/variant system
- `@vanilla-extract/next-plugin: ^2.5.1` — Next.js integration
- `@playwright/test: ^1.57.0` — E2E + visual regression
- `next: 15.3.2`, `react: ^19.0.0` — supports View Transitions

Missing (to install):
- `@storybook/nextjs` + `@storybook/addon-themes` — Storybook setup (not yet installed)
- `@chromatic-com/storybook` — Chromatic integration
- `@vanilla-extract/dynamic` — for `assignInlineVars` runtime token overrides
- `@fontsource-variable/jetbrains-mono` OR rely on `next/font/google` (preferred, no install needed)
