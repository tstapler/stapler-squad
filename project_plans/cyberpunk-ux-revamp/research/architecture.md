# Architecture Research: Cyberpunk UX Revamp

## 1. Multi-Theme Architecture with vanilla-extract

### The Core Pattern

vanilla-extract's `createTheme` produces a CSS class that reassigns all contract variables. Theme switching = swapping the class on `<html>`. This is the only runtime operation — all actual CSS is static and pre-built.

```
Build time:  createTheme(vars, {...}) → generates .css3af8d1b (hashed CSS class)
Runtime:     html.className = ".css3af8d1b .css9f2e3a1" → all vars resolve to new values
```

The `theme-contract.css.ts` exports `vars`, which is the typed proxy. Any `.css.ts` file that references `vars.color.primary` will automatically pick up the currently active theme's value with zero JavaScript overhead.

### FOUC Prevention Strategy (SSR / Hydration Safety)

The existing `layout.tsx` hardcodes `className={lightTheme}` on `<html>` as the SSR fallback. This prevents a blank flash but does cause a theme mismatch flash if the user has a different theme saved in localStorage.

**Solution: Blocking inline script in `<head>`**

In `layout.tsx`, inject a script before React hydration:

```tsx
<head>
  <script dangerouslySetInnerHTML={{ __html: `
    (function() {
      var stored = localStorage.getItem('theme') || 'matrix';
      var themeMap = {
        matrix: '${matrixTheme}',
        cyberpunk77: '${cyberpunk77Theme}',
        wh40k: '${wh40kTheme}',
        clean: '${cleanTheme}',
      };
      document.documentElement.className =
        (document.documentElement.className + ' ' + (themeMap[stored] || themeMap['matrix'])).trim();
    })();
  `}} />
</head>
```

This runs synchronously before CSS is applied, before React hydrates. The `suppressHydrationWarning` on `<html>` already handles the class mismatch between server render and client.

**Why not `next-themes`?** The project uses vanilla-extract class-based theming. `next-themes` sets `data-theme` attributes and relies on CSS variable selectors — incompatible with the class-based `createTheme` approach without modification. A custom lightweight solution is preferable.

### Theme Context Architecture

Create `ThemeContext.tsx` to replace the current bare `ThemeProvider.tsx`:

```ts
interface ThemeContextValue {
  theme: ThemeName;
  setTheme: (name: ThemeName) => void;
  availableThemes: ThemeName[];
}
```

- `ThemeProvider` reads from localStorage on mount
- Exposes `setTheme` for the settings UI + omnibar theme command
- Fires `document.documentElement.classList` swap
- Theme names: `"matrix" | "cyberpunk77" | "wh40k" | "clean"`

### Class Name Conflict Prevention

vanilla-extract generates deterministic, content-hashed class names. When the build re-runs, the class name changes if the theme values change. The localStorage value stores the human-readable name (`"matrix"`), not the hashed class — the mapping is resolved at runtime via the theme class map.

**Token Drift Risk**: If a token exists in `matrixTheme` but not in `cleanTheme` (or vice versa), TypeScript will catch it at build time because `createTheme` requires all contract fields to be satisfied. This is a strong guard against token drift.

---

## 2. Collapsible Drawer Navigation

### State Management Approach

**Recommendation: React Context (not Zustand)**

The app already uses Context for OmnibarContext, NotificationContext, SessionServiceContext, etc. The drawer is UI-local state. Zustand would add a dependency for minimal benefit.

```ts
// NavigationContext.tsx
interface NavigationContextValue {
  isDrawerOpen: boolean;
  toggleDrawer: () => void;
  closeDrawer: () => void;
}
```

Persist drawer state in localStorage (`nav-drawer-open`) so it survives page reloads. Default: open on desktop, closed on mobile.

### Layout Architecture

Replace the current top-level layout with a grid:

```
┌──────────────────────────────────────────┐
│              Header (optional)            │
├──────────┬───────────────────────────────┤
│          │                               │
│  Drawer  │       Main Content            │
│  (nav)   │  (SessionList | Detail | etc) │
│          │                               │
└──────────┴───────────────────────────────┘
```

In vanilla-extract:

```ts
export const cockpitLayout = style({
  display: "grid",
  gridTemplateColumns: `var(--drawer-width, 240px) 1fr`,
  gridTemplateRows: "var(--header-height, 0px) 1fr",
  height: "100dvh",
  transition: "grid-template-columns 200ms ease",
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      transition: "none",
    },
  },
});

export const drawerCollapsed = style({
  vars: { "--drawer-width": "48px" }, // icon-only mode
});
```

**Animation approach**: Animating CSS grid column widths does NOT trigger layout reflow on modern browsers (it's handled by the compositor). Alternatively, use `width` + `overflow: hidden` on the drawer itself — but prefer `transform: translateX` for guaranteed compositing.

The cleanest approach for zero-reflow collapse:

```ts
// drawer transforms out of view, main content uses padding-left instead of grid
export const drawerPanel = style({
  width: "240px",
  transform: "translateX(0)",
  transition: "transform 200ms ease, width 200ms ease",
});
export const drawerPanelCollapsed = style({
  transform: "translateX(-240px)",
  width: "240px", // keep width for correct reflow in grid; use translateX to animate
});
```

### Keyboard: Drawer Toggle

Add to the global shortcut registry: `[` or `Cmd+\` to toggle drawer. Store in the centralized registry (see section 3).

---

## 3. Keyboard Shortcut System Architecture

### Current State

Shortcuts are registered in 5+ different places:
- `OmnibarContext.tsx`: Cmd+K, Cmd+Shift+K, `n`
- `Header.tsx`: Escape (mobile menu)
- `ApprovalDrawer.tsx`: Escape
- `useKeyboard.ts`: generic hook called from pages
- `useReviewQueueNavigation.ts`: review-specific navigation

This creates hidden conflict risk and no way to show a unified `?` help overlay.

### Recommended: Centralized Registry Pattern

Do NOT use a third-party library. The `useKeyboard.ts` hook is a solid foundation. Extend it into a registry:

```ts
// lib/shortcuts/shortcutRegistry.ts
type ShortcutContext = "global" | "session-list" | "approval" | "terminal";

interface Shortcut {
  key: string;
  modifiers?: { ctrl?: boolean; meta?: boolean; shift?: boolean; alt?: boolean };
  label: string;
  context: ShortcutContext;
  action: () => void;
}

class ShortcutRegistry {
  private shortcuts = new Map<string, Shortcut>();
  register(id: string, shortcut: Shortcut): () => void { ... }
  dispatch(event: KeyboardEvent, activeContext: ShortcutContext): void { ... }
  getAll(): Shortcut[] { ... }
}

export const registry = new ShortcutRegistry();
```

One `document.addEventListener("keydown")` in `ShortcutRegistry.dispatch` dispatches to all registered handlers. Each hook/component registers shortcuts on mount and deregisters on unmount via the returned cleanup function.

### Context-Sensitive Shortcuts (Terminal Conflict)

The critical requirement: `j`/`k` must NOT navigate sessions when the terminal has focus.

The terminal (xterm.js) captures all keyboard input when focused — keyboard events inside the terminal element do not bubble to `document` when the terminal element has DOM focus. This is the correct behavior.

However, the guard must also exist in software: register `j`/`k` as `context: "session-list"` shortcuts. The registry's dispatch checks whether the active context is `"terminal"` and skips session-list shortcuts if so.

**Active context detection:**

```ts
function getActiveContext(): ShortcutContext {
  const focused = document.activeElement;
  if (focused?.closest("[data-context='terminal']")) return "terminal";
  if (focused?.closest("[data-context='approval']")) return "approval";
  if (focused?.closest("[data-context='session-list']")) return "session-list";
  return "global";
}
```

Components set `data-context` on their root element. The registry reads this before dispatching.

### `?` Overlay

```ts
// components/ui/KeyboardShortcutOverlay.tsx
// Triggered by `?` key (global context, not in input)
// Reads registry.getAll() and renders grouped by context
// Uses styled <kbd> elements
```

### Omnibar Theme Command

Register `>theme` as an omnibar command prefix (like VS Code's `>` commands). When the omnibar input starts with `>`, switch to command mode and offer theme names as completions. This requires extending `DetectorRegistry` with a `CommandDetector` (priority ~5, before all others).

---

## 4. Notification/Toast Architecture

### Current State

`NotificationContext.tsx` already has a comprehensive system:
- `notifications: NotificationData[]` — active toasts
- `notificationHistory` — persistent history
- `addNotification()`, `acknowledgeNotification()`, `showSessionNotification()`
- `NotificationToast` component already exists
- `NotificationPanel` component renders in `layout.tsx`

The system already connects to ConnectRPC streaming events via `useReviewQueueNotifications.ts` (12KB hook) and `useSessionNotifications.ts` (11.5KB hook).

### What to Change for Cyberpunk UX

1. **Toast appearance**: Style `NotificationToast` with cyberpunk aesthetic (terminal-style, mono font, neon border, slide-in from right with translate animation)
2. **Approval toasts**: When `ApprovalCard` approve/deny fires, call `addNotification` with appropriate type — this is NOT currently wired up
3. **Toast position**: Currently unknown — verify it's top-right or bottom-right, not overlapping the drawer nav
4. **Approval-specific notification flow**: 
   - Approval pending → badge in header (already exists via `ApprovalNavBadge`)
   - Approval resolved → toast notification
   - Wire in `ApprovalCard.tsx` `onApprove`/`onDeny` handlers to call `addNotification`

### Architecture Recommendation

No new context needed. Extend existing `NotificationContext` with:
```ts
addApprovalResolvedNotification: (toolName: string, decision: "approved" | "denied") => void;
```

This keeps all toast state in one place.

---

## 5. Visual Regression Testing Architecture

### Organization

```
tests/snapshots/
  matrix-theme/
    session-list/
      empty-state.png
      with-sessions.png
      running-session-glow.png
    approval-drawer/
      open-state.png
    omnibar/
      open.png
  cyberpunk77-theme/
    ...
  wh40k-theme/
    ...
  clean-theme/
    ...
```

### Playwright Project Configuration

4 projects (one per theme) + 1 default non-themed project. Each themed project:
1. Sets `localStorage.theme` via `storageState` fixture
2. Disables animations (`reducedMotion: 'reduce'`)
3. Uses `viewport: { width: 1280, height: 800 }` (desktop cockpit view)

### Baseline Management

- Baselines stored in `tests/snapshots/` in git
- CI fails on diff > threshold; developers run `--update-snapshots` locally to accept intentional changes
- Storybook + Chromatic handles component-level visual regression separately from Playwright (full-page)

### Component vs Page Level

- **Playwright**: Full page screenshots per route × 4 themes — catches layout regressions
- **Chromatic** (via Storybook): Per-component screenshots × 4 themes — catches component regressions in isolation
- Both are needed; they complement each other

---

## 6. Storybook + vanilla-extract Decorator Pattern

### Global Decorator Pattern

```ts
// .storybook/preview.tsx
import { withThemeByClassName } from "@storybook/addon-themes";
import { THEME_CLASSES } from "../web-app/src/styles/themes";

export const decorators = [
  withThemeByClassName({
    themes: THEME_CLASSES,
    defaultTheme: "matrix",
  }),
  // Ensure fonts are loaded in Storybook
  (Story) => (
    <div style={{ fontFamily: "var(--font-jetbrains-mono, monospace)" }}>
      <Story />
    </div>
  ),
];
```

### Per-Story Theme Override

```ts
// ApprovalCard.stories.ts
export default {
  parameters: {
    themes: { themeOverride: "wh40k" }, // always render this story in WH40K theme
  },
};
```

### CSS Import in Storybook

vanilla-extract CSS is extracted at build time. In the Storybook Next.js framework, this happens automatically via `@vanilla-extract/next-plugin`. The `.storybook/main.ts` config needs:

```ts
import VanillaExtractPlugin from "@vanilla-extract/next-plugin";
const config = {
  webpackFinal: async (config) => {
    // VanillaExtractPlugin should already be injected by @storybook/nextjs
    return config;
  },
};
```

### Known HMR Workaround

If Storybook freezes during development due to vite/webpack CSS file multiplication (known vanilla-extract issue), add to Storybook webpack config:

```ts
config.module.rules = config.module.rules.filter(
  (rule) => !rule?.test?.toString().includes("\\.vanilla\\.css")
);
```

Then manually add a corrected rule. This is a known workaround from the vanilla-extract GitHub issues.

---

## Key Architectural Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Theme state | React Context | Consistent with existing app patterns; localStorage backed |
| FOUC prevention | Blocking inline script | Only reliable SSR solution; `suppressHydrationWarning` already on `html` |
| Keyboard registry | Custom centralized registry | No new deps; extends existing `useKeyboard.ts`; enables `?` overlay |
| Terminal shortcut guard | `data-context` attribute + focus detection | xterm.js already captures keyboard; software guard is extra safety |
| Drawer state | React Context + localStorage | UI-local, no global server state |
| Toast architecture | Extend existing `NotificationContext` | System already exists and is connected to streaming events |
| Visual regression | Playwright projects × 4 themes | Existing Playwright infra; project-based approach scales |
| Storybook | `@storybook/nextjs` + `addon-themes` | Best vanilla-extract HMR stability; multi-theme stories |
