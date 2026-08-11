# Implementation Plan: Cyberpunk UX Revamp

**Status**: Draft | **Phase**: 3 — Planning complete
**Created**: 2026-05-02
**Epics**: 6 | **Stories**: 28 | **Tasks**: 97

---

## Technology Decisions

### TD-1: FOUC Prevention — Blocking Inline Script
Apply theme class synchronously in `<head>` via `dangerouslySetInnerHTML` script before React hydration. `suppressHydrationWarning` already present on `<html>`. No `next-themes` (class-based VE theming is incompatible with its default `data-theme` attribute model).

### TD-2: Keyboard Shortcut System — Custom Centralized Registry
Build `web-app/src/lib/shortcuts/shortcutRegistry.ts` as a singleton class with a single `document.addEventListener("keydown")`. No third-party library. Rationale: existing `useKeyboard.ts` is a solid foundation; a custom registry enables the `?` overlay by exposing `getAll()` without adding a dependency. Context sensitivity via `data-context` attribute and `document.activeElement.closest()`.

### TD-3: Drawer State — React Context + localStorage
`NavigationContext.tsx` — consistent with all other app contexts. Persist `nav-drawer-open` in localStorage. Default: open (≥1024px), closed (<1024px).

### TD-4: Notification Wiring — Extend Existing NotificationContext
No new context. Add `addApprovalResolvedNotification(toolName, decision)` to existing `NotificationContext`. `ApprovalCard.tsx` calls this from its `onApprove`/`onDeny` handlers after the ConnectRPC call resolves.

### TD-5: `@vanilla-extract/dynamic` — Add the Package
Install `@vanilla-extract/dynamic` for `assignInlineVars`. Required for per-session accent color and runtime-dynamic tokens (e.g., applying a session-specific glow color to a card via inline CSS variables). The package is lightweight and zero-runtime by design.

### TD-6: Storybook — Webpack-Based (`@storybook/nextjs`) Before Visual Regression
Set up Storybook first (Phase C, Epic 6, Story 6.1) before adding multi-theme Playwright snapshot projects (Story 6.2). Rationale: Storybook component stories double as the baseline visual reference; visual regression snapshots should be taken against known-good story states. Vite-based Storybook is explicitly prohibited (CSS file multiplication issue).

### TD-7: `globals.css` Migration — Scope First, Delete Later
Do NOT delete the `@media (prefers-color-scheme: dark)` block in one step. Sequence:
1. Audit which components still use `globals.css` legacy vars (`--background`, `--primary`, etc.)
2. Migrate those components to `vars.*` references
3. Once zero consumers remain, remove the `@media` block
This is Story 1.5 (migration gate) and must complete before Epic 2 (layout) ships.

### TD-8: View Transitions — Audit `flushSync` First
Before enabling `experimental.viewTransition: true` in `next.config.js`, audit all `flushSync` call sites. Wrap omnibar open/close in `startTransition` to activate the View Transition animation.

### TD-9: JetBrains Mono — `next/font/google`
No new npm package. Use `next/font/google` in `web-app/src/app/fonts.ts`. Update `sharedTokens.font.mono` in `theme.css.ts` to use the CSS variable `var(--font-jetbrains-mono, 'Monaco', monospace)`. Matrix and Cyberpunk themes also set `vars.font.display` to the same variable; WH40K sets it to the Cinzel variable; Clean sets it to Inter.

---

## Phasing Overview

| Phase | Label | Epics | Prerequisite |
|-------|-------|-------|-------------|
| A | Foundation | 1, 2, 3 | None — must complete before B |
| B | Visual Polish | 4, 5 | Phase A complete |
| C | Tooling & QA | 6 | Phase A complete (can run in parallel with B) |

Phase A establishes the token system, layout skeleton, and keyboard registry. Phase B adds the cyberpunk visual effects and review queue redesign. Phase C adds Storybook, visual regression, and contrast tooling — can start once Phase A delivers stable components.

---

## Epic 1: Theme System Foundation (Phase A)

**Goal**: Four fully working vanilla-extract themes selectable at runtime without FOUC. All new CSS tokens defined and linted. JetBrains Mono loaded. `globals.css` conflict neutralized.

---

### Story 1.1 — Extend Theme Contract with Cyberpunk Tokens

**As a** developer, **I want** all cyberpunk-specific design tokens in the type-safe contract **so that** no `.css.ts` file ever hardcodes a hex value.

**Acceptance criteria**:
- `theme-contract.css.ts` exports `vars.color.glowPrimary`, `vars.color.glowSecondary`, `vars.color.scanlineColor`, `vars.color.terminalCursor`, and `vars.font.display`
- TypeScript compile passes with all four `createTheme` calls satisfying the full contract
- `lint:css` CI step passes

**Tasks**:

1.1.1. **Add 5 new tokens to `web-app/src/styles/theme-contract.css.ts`**
  - Add to `color` group: `glowPrimary: null`, `glowSecondary: null`, `scanlineColor: null`, `terminalCursor: null`
  - Add to `font` group: `display: null`
  - Approach: edit the `createThemeContract` call directly; TypeScript will immediately report all `createTheme` call sites as errors, surfacing every place that needs updating
  - Dependencies: none

1.1.2. **Update `lightTheme` and `darkTheme` in `web-app/src/styles/theme.css.ts` to satisfy new tokens**
  - `lightTheme`: `glowPrimary: "rgba(0,112,243,0.4)"`, `glowSecondary: "rgba(0,112,243,0.2)"`, `scanlineColor: "transparent"`, `terminalCursor: "#0070f3"`, `font.display: "system-ui, sans-serif"`
  - `darkTheme`: `glowPrimary: "rgba(45,156,219,0.4)"`, `glowSecondary: "rgba(45,156,219,0.2)"`, `scanlineColor: "transparent"`, `terminalCursor: "#2d9cdb"`, `font.display: "system-ui, sans-serif"`
  - Dependencies: 1.1.1

---

### Story 1.2 — Implement Matrix, Cyberpunk77, WH40K, Clean Themes

**As a** user, **I want** four distinct visual themes **so that** I can choose a visual identity for my cockpit.

**Acceptance criteria**:
- Four exported theme classes: `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`
- Every token in the contract is populated (no TypeScript errors)
- Matrix is the new default; `cleanTheme` supersedes `darkTheme` for the "safe" mode
- `lightTheme` and `darkTheme` remain for backward compatibility but are no longer the default

**Tasks**:

1.2.1. **Add `matrixTheme` to `web-app/src/styles/theme.css.ts`**
  - Background: `#000000`, cardBackground: `#0a0a0a`, hoverBackground: `#0d1a00`
  - textPrimary: `#00ff41`, textSecondary: `#00cc33`, textMuted: `#004d18`, textDisabled: `#002b0e`
  - primary: `#00ff41`, primaryHover: `#33ff66`, primaryActive: `#00cc33`, primaryDark: `#004d18`, primaryText: `#000000`
  - glowPrimary: `rgba(0,255,65,0.5)`, glowSecondary: `rgba(0,255,65,0.25)`, scanlineColor: `rgba(0,255,65,0.03)`, terminalCursor: `#00ff41`
  - borderColor: `#003300`, borderSubtle: `#002200`, borderStrong: `#005500`
  - font.display: `"var(--font-jetbrains-mono,'Monaco',monospace)"`, font.mono: `"var(--font-jetbrains-mono,'Monaco',monospace)"`
  - success: `#00ff41`, warning: `#ffaa00`, error: `#ff0040`
  - All statusBadge tokens: use dark green backgrounds with bright green foregrounds
  - Dependencies: 1.1.1, 1.1.2

1.2.2. **Add `cyberpunk77Theme` to `web-app/src/styles/theme.css.ts`**
  - Background: `#0d0d1a`, cardBackground: `#12122a`, hoverBackground: `#1a1a35`
  - textPrimary: `#fcee09`, textSecondary: `#c8be08`, textMuted: `#7a7405`, textDisabled: `#4a4603`
  - primary: `#ff2d78`, primaryHover: `#ff5090`, primaryActive: `#cc2460`, primaryText: `#ffffff`
  - glowPrimary: `rgba(255,45,120,0.5)`, glowSecondary: `rgba(0,212,255,0.4)`, scanlineColor: `rgba(255,45,120,0.02)`, terminalCursor: `#00d4ff`
  - borderColor: `#1a1a3e`, borderStrong: `#2d2d6e`
  - font.display: `"var(--font-rajdhani,'Rajdhani',system-ui,sans-serif)"`, font.mono: `"var(--font-jetbrains-mono,'Monaco',monospace)"`
  - success: `#00ff9f`, warning: `#fcee09`, error: `#ff2d78`
  - Dependencies: 1.1.1, 1.1.2

1.2.3. **Add `wh40kTheme` to `web-app/src/styles/theme.css.ts`**
  - Background: `#0c0a08`, cardBackground: `#1a1510`, hoverBackground: `#221e18`
  - textPrimary: `#c8b89a`, textSecondary: `#a89878`, textMuted: `#786858`, textDisabled: `#484038`
  - primary: `#c0a020`, primaryHover: `#d4b424`, primaryActive: `#a08818`, primaryText: `#0c0a08`
  - glowPrimary: `rgba(192,160,32,0.4)`, glowSecondary: `rgba(139,26,26,0.4)`, scanlineColor: `transparent`, terminalCursor: `#c0a020`
  - borderColor: `#3d3020`, borderStrong: `#c0a020`
  - font.display: `"var(--font-cinzel,'Cinzel',serif)"`, font.mono: `"var(--font-jetbrains-mono,'Monaco',monospace)"`
  - success: `#4a7c3f`, warning: `#c0a020`, error: `#8b1a1a`
  - Dependencies: 1.1.1, 1.1.2

1.2.4. **Add `cleanTheme` to `web-app/src/styles/theme.css.ts`**
  - Clone values from `darkTheme` but use purple accent: primary: `#7c3aed`, primaryHover: `#8b5cf6`
  - Background: `#0f0f11`, cardBackground: `#1a1a1f`, hoverBackground: `#22222a`
  - glowPrimary: `rgba(124,58,237,0.4)`, glowSecondary: `rgba(124,58,237,0.2)`, scanlineColor: `transparent`
  - font.display: `"Inter,system-ui,sans-serif"`
  - Light mode variant: expose `cleanLightTheme` using existing `lightTheme` values with purple primary
  - Dependencies: 1.1.1, 1.1.2

---

### Story 1.3 — JetBrains Mono + Display Font Loading

**As a** user, **I want** JetBrains Mono loaded for all UI text in Matrix/Cyberpunk themes and display fonts for WH40K **so that** typography reinforces the aesthetic.

**Acceptance criteria**:
- JetBrains Mono loaded via `next/font/google`, no CDN calls
- Rajdhani loaded via `next/font/google`
- Cinzel loaded via `next/font/google`
- Inter loaded via `next/font/google` (for clean theme)
- All four fonts inject their CSS variable into `<html>` className
- `vars.font.mono` and `vars.font.display` resolve to the correct font per theme

**Tasks**:

1.3.1. **Create `web-app/src/app/fonts.ts`**
  - Export `jetbrainsMono` (`JetBrains_Mono`, variable `--font-jetbrains-mono`, subsets: `["latin"]`, axes: `["ital"]`)
  - Export `rajdhani` (`Rajdhani`, variable `--font-rajdhani`, subsets: `["latin"]`, weights: `["400","500","600","700"]`)
  - Export `cinzel` (`Cinzel`, variable `--font-cinzel`, subsets: `["latin"]`, weights: `["400","700"]`)
  - Export `inter` (`Inter`, variable `--font-inter`, subsets: `["latin"]`)
  - Dependencies: none

1.3.2. **Update `web-app/src/app/layout.tsx` to apply font variables**
  - Import all four font objects from `fonts.ts`
  - Change `<html className={...}>` to also include all four font variable class names
  - Example: `className={`${jetbrainsMono.variable} ${rajdhani.variable} ${cinzel.variable} ${inter.variable} ${matrixTheme}`}`
  - Dependencies: 1.3.1, 1.2.1

---

### Story 1.4 — Theme Context, Switching, and FOUC Prevention

**As a** user, **I want** to switch themes at runtime and have my choice persisted across page loads without a flash **so that** the experience feels seamless.

**Acceptance criteria**:
- `ThemeContext` exposes `theme`, `setTheme`, `availableThemes`
- Switching theme updates `<html>` class and `localStorage['stapler-theme']` atomically
- No FOUC: refreshing with a non-default theme stored shows the correct theme from the first paint
- Omnibar accepts commands `theme matrix`, `theme cyberpunk77`, `theme wh40k`, `theme clean`

**Tasks**:

1.4.1. **Create `web-app/src/lib/contexts/ThemeContext.tsx`**
  - Replace existing bare `ThemeProvider.tsx`
  - Export `ThemeContext`, `ThemeProvider`, `useTheme` hook
  - `ThemeProvider` on mount: reads `localStorage.getItem('stapler-theme')`, falls back to `"matrix"`
  - `setTheme(name)`: removes all theme classes from `document.documentElement`, adds new class, calls `localStorage.setItem('stapler-theme', name)`
  - Export `THEME_CLASSES: Record<ThemeName, string>` mapping names to vanilla-extract class names
  - Dependencies: 1.2.1–1.2.4

1.4.2. **Inject FOUC-prevention script in `web-app/src/app/layout.tsx`**
  - Add `<script dangerouslySetInnerHTML>` as the very first child of `<head>`
  - Script body: read `localStorage['stapler-theme']`, look up class in inline `themeMap` object that embeds the hashed class names via template literal, add class to `document.documentElement.className`
  - The hashed class name map must be interpolated at build time using the imported theme class name constants
  - Keep `suppressHydrationWarning` on `<html>` (already present)
  - Dependencies: 1.4.1

1.4.3. **Register theme commands in omnibar `CommandDetector`**
  - Create `web-app/src/lib/omnibar/detectors/CommandDetector.ts`
  - Detects inputs starting with `>` (VS Code-style command prefix), priority 5
  - Recognizes: `>theme matrix`, `>theme cyberpunk77`, `>theme wh40k`, `>theme clean`, `>go sessions`, `>go review`, `>go history`
  - Returns `DetectionResult` with type `InputType.Command`
  - Register in `createDefaultRegistry()` in `web-app/src/lib/omnibar/detector.ts` at priority 5
  - Dependencies: 1.4.1

1.4.4. **Add `theme` action to omnibar dispatch**
  - In `web-app/src/lib/omnibar/actions/types.ts`: add `{ type: "set_theme"; themeName: ThemeName }` to `OmnibarAction` union
  - In `web-app/src/lib/omnibar/actions/dispatch.ts`: add `case "set_theme"`: call `deps.setTheme(action.themeName)`
  - `ActionDeps` interface: add `setTheme: (name: ThemeName) => void`
  - In `web-app/src/components/sessions/Omnibar.tsx`: wire `deps.setTheme` from `useTheme().setTheme`
  - Dependencies: 1.4.3, 1.4.1

1.4.5. **Add theme picker to settings page**
  - In `web-app/src/app/settings/page.tsx` (create if not exists, or add section to existing): render four theme cards with preview swatch and name
  - Clicking a card calls `setTheme(name)` from `useTheme()`
  - Active theme card has a border highlight using `vars.color.primary`
  - Dependencies: 1.4.1

---

### Story 1.5 — Neutralize `globals.css` Dark Mode Conflict

**As a** developer, **I want** `globals.css` legacy CSS variables to not fight the vanilla-extract theme classes **so that** all components display correctly in all four themes.

**Acceptance criteria**:
- Zero components read from `globals.css` legacy vars (`--background`, `--primary`, etc.) after migration
- The `@media (prefers-color-scheme: dark)` block in `globals.css` is removed
- `lint:css-vars` CI step still passes
- All existing E2E tests still pass

**Tasks**:

1.5.1. **Audit `globals.css` consumers**
  - Run: `grep -r "var(--background\|var(--primary\|var(--text-primary\|var(--border-color\|var(--card-background" web-app/src --include="*.css" --include="*.css.ts" --include="*.tsx" -l`
  - Output list of files still using legacy vars
  - Produce a migration checklist comment in `globals.css` (one line per file)
  - Dependencies: none

1.5.2. **Migrate remaining `.module.css` consumers to `vars.*` references**
  - For each file identified in 1.5.1 that uses `var(--background)` etc.: convert to use `vars.color.background` (import `vars` from `theme.css.ts`)
  - Do this file-by-file; run `make restart-web` and visually verify after each batch
  - Dependencies: 1.5.1

1.5.3. **Remove `@media (prefers-color-scheme: dark)` block from `globals.css`**
  - After all consumers migrated: delete the entire `@media (prefers-color-scheme: dark) { :root { ... } }` block
  - Verify with `npm run lint` in `web-app/`
  - Dependencies: 1.5.2

---

## Epic 2: Layout Redesign — Cockpit Shell (Phase A)

**Goal**: Replace the flat single-column layout with a collapsible left drawer nav + 3-column session view grid. Terminal fills full height. Mobile falls back gracefully.

---

### Story 2.1 — Collapsible Left Drawer Navigation

**As a** user, **I want** a collapsible navigation drawer **so that** I can access all sections while maximizing screen space for the terminal.

**Acceptance criteria**:
- Drawer expands to 240px, collapses to 56px (icon-only)
- Toggle via: button in drawer, keyboard shortcut `[`
- Auto-collapses below 1024px viewport width
- Drawer state persists in `localStorage['nav-drawer-open']`
- Nav items: Sessions (with count badge), Review Queue (badge), History, Rules, Config, Logs
- Active item highlighted with `vars.color.primary` left border

**Tasks**:

2.1.1. **Create `web-app/src/lib/contexts/NavigationContext.tsx`**
  - `NavigationContextValue`: `isDrawerOpen`, `toggleDrawer`, `closeDrawer`, `openDrawer`
  - On mount: read `localStorage['nav-drawer-open']`; if viewport < 1024px, override to closed
  - `toggleDrawer`: flips state, writes to localStorage
  - Add `ResizeObserver` on `window` to auto-close below 1024px
  - Dependencies: none

2.1.2. **Create `web-app/src/components/layout/DrawerNav.tsx` and `DrawerNav.css.ts`**
  - Structure: `<nav>` with `data-testid="drawer-nav"`, `aria-label="Main navigation"`
  - Two modes driven by `isDrawerOpen`: expanded (shows text + icons) vs collapsed (icons only)
  - Nav items rendered as `<Link>` with active state from `usePathname()`
  - Count badges on Sessions and Review Queue from existing badge hooks
  - Collapse toggle button at bottom of drawer with `aria-label="Toggle navigation"`
  - CSS: use `vars` tokens throughout; drawer uses `transform: translateX` for animation; `transition: transform 200ms ease`, disable under `prefers-reduced-motion`
  - Dependencies: 2.1.1

2.1.3. **Create cockpit layout styles in `web-app/src/styles/layout.css.ts`**
  - `cockpitRoot`: `display: grid`, `gridTemplateColumns: "var(--drawer-width) 1fr"`, `height: "100dvh"`, `overflow: hidden`
  - `drawerColumn`: `width: 240px`, `transition: width 200ms ease`, with collapsed variant at `56px`
  - `mainContent`: `overflow: hidden`, `display: flex`, `flexDirection: column`
  - `@media (prefers-reduced-motion: reduce)`: `transition: none`
  - Dependencies: none

2.1.4. **Refactor `web-app/src/app/layout.tsx` to use cockpit layout**
  - Replace existing layout body structure with: `NavigationProvider > cockpitRoot div > DrawerNav + main content area`
  - Remove `ConditionalHeader` from top-level (repurpose header content into drawer or compact top bar)
  - Wrap in `NavigationProvider` in the providers chain
  - Keep `NotificationPanel` and `ApprovalDrawer` as portals (they float over layout)
  - Dependencies: 2.1.1, 2.1.2, 2.1.3

2.1.5. **Register `[` keyboard shortcut to toggle drawer**
  - In `web-app/src/lib/shortcuts/shortcutRegistry.ts` (Epic 3 creates this file; coordinate dependency)
  - Register: id `"nav:toggle-drawer"`, key `"["`, context `"global"`, label `"Toggle navigation drawer"`, action: `navigationContext.toggleDrawer()`
  - This task depends on Story 3.1 creating the registry; if sequencing is tight, register via `useKeyboard` hook temporarily
  - Dependencies: 2.1.1, (3.1.1 — soft dependency)

---

### Story 2.2 — Three-Column Session View

**As a** user, **I want** session list, session detail/terminal, and context panel side-by-side **so that** I can see my work without navigating away.

**Acceptance criteria**:
- Column 1 (session list): 280px fixed, theme-styled
- Column 2 (terminal/detail): fills remaining space, terminal fills full height
- Column 3 (context panel): 320px, slides in when diff/approval selected, not a full navigation
- On ≤768px: single column, panels become bottom sheets
- `<main>` receives `data-testid="session-cockpit"` for E2E targeting

**Tasks**:

2.2.1. **Create session cockpit layout styles in `web-app/src/styles/sessionCockpit.css.ts`**
  - `cockpitGrid`: `display: grid`, `gridTemplateColumns: "280px 1fr"`, transitions to `"280px 1fr 320px"` when context panel open
  - `sessionListColumn`: `overflow-y: auto`, `borderRight: "1px solid ${vars.color.borderColor}"`
  - `detailColumn`: `display: flex`, `flexDirection: column`, `overflow: hidden`
  - `contextPanel`: `width: 320px`, `transform: translateX(100%)` when closed, `translateX(0)` when open, `transition: transform 200ms ease`
  - Media query `(max-width: 768px)`: grid becomes single column; context panel becomes `position: fixed; bottom: 0; height: 50vh`
  - Dependencies: none

2.2.2. **Refactor `web-app/src/app/page.tsx` to use three-column cockpit layout**
  - Replace ad-hoc flex/grid with `cockpitGrid` class
  - `SessionList` goes in column 1
  - `SessionDetail` (containing terminal) goes in column 2
  - Diff/approval panel (currently full-page overlay) mounts in column 3 as sliding context panel
  - Add `data-testid="session-cockpit"` to root element
  - Dependencies: 2.2.1

2.2.3. **Ensure terminal fills full column 2 height**
  - In `web-app/src/components/sessions/terminal/Terminal.tsx` (or equivalent): set `height: 100%`, `flex: 1`
  - Verify xterm.js `fitAddon` resizes correctly when column 2 resizes due to context panel sliding in
  - Trigger `fitAddon.fit()` on a `ResizeObserver` on the terminal container element
  - Dependencies: 2.2.2

2.2.4. **Compact session detail header bar (single row above terminal)**
  - Create `web-app/src/components/sessions/SessionDetailBar.tsx` and `SessionDetailBar.css.ts`
  - Single-row bar showing: branch name, status badge, path (truncated), keyboard shortcut hints `[t] terminal  [p] pause  [r] resume`
  - Replaces the current multi-row session detail header
  - Height: 40px fixed; uses `vars.font.mono` for all text; `vars.color.borderColor` bottom border
  - Dependencies: 2.2.2

---

### Story 2.3 — Responsive Mobile Layout

**As a** mobile user, **I want** the cockpit to collapse gracefully to single-column **so that** sessions remain usable on a phone.

**Acceptance criteria**:
- On ≤768px: single column layout; drawer becomes bottom sheet slide-up
- Session list → detail navigation works via back button / swipe
- Existing `BottomNav` preserved for mobile
- No horizontal scroll on any screen width ≥320px

**Tasks**:

2.3.1. **Add mobile breakpoint overrides to `sessionCockpit.css.ts`**
  - At `(max-width: ${breakpoints.md})`: `cockpitGrid` becomes `gridTemplateColumns: "1fr"`, all columns stack vertically
  - DrawerNav at mobile: `position: fixed; bottom: 0; width: 100%` (repurpose as slide-up bottom nav or hide and show `BottomNav`)
  - Dependencies: 2.2.1

2.3.2. **Add back-navigation for mobile session detail**
  - When `≤768px` and `selectedSession` is set, column 1 hidden, column 2 shown full-width
  - Back button in `SessionDetailBar` calls `clearSelectedSession()` on mobile only
  - Dependencies: 2.2.2, 2.2.4

---

## Epic 3: Keyboard-First Interaction (Phase A)

**Goal**: All primary session actions reachable without a mouse. Centralized shortcut registry. `?` help overlay. Shortcut hints rendered inline in UI.

---

### Story 3.1 — Centralized Shortcut Registry

**As a** developer, **I want** a single source of truth for all keyboard shortcuts **so that** the `?` overlay can enumerate them and we can detect conflicts at registration time.

**Acceptance criteria**:
- Single `document.addEventListener("keydown")` at registry level
- Components register/deregister via `registry.register(id, config)` returning a cleanup function
- Context-sensitive dispatch: terminal-focused keyboard events skip non-global shortcuts
- IME composition events (`event.isComposing`) skipped for single-character shortcuts
- `registry.getAll()` returns all registered shortcuts grouped by context

**Tasks**:

3.1.1. **Create `web-app/src/lib/shortcuts/shortcutRegistry.ts`**
  - `ShortcutContext = "global" | "session-list" | "approval" | "terminal"`
  - `Shortcut` interface: `key`, `modifiers?`, `label`, `context`, `action: () => void`
  - `ShortcutRegistry` class with `Map<string, Shortcut>` storage
  - `register(id, shortcut): () => void` — returns cleanup function
  - `dispatch(event: KeyboardEvent)`: detect active context via `document.activeElement.closest('[data-context]')`, skip if `event.isComposing`, match key + modifiers, call action
  - `getAll(): Record<ShortcutContext, Shortcut[]>` — for `?` overlay
  - Single `document.addEventListener("keydown", dispatch)` in constructor; no per-component listeners
  - Export singleton: `export const registry = new ShortcutRegistry()`
  - Dependencies: none

3.1.2. **Create `web-app/src/lib/shortcuts/useShortcut.ts` hook**
  - `useShortcut(id: string, shortcut: Omit<Shortcut, 'action'> & { action: () => void })` — registers on mount, deregisters on unmount via returned cleanup
  - Wraps `registry.register(id, shortcut)` in `useEffect` with proper dep array
  - Dependencies: 3.1.1

3.1.3. **Migrate existing shortcuts to registry**
  - `OmnibarContext.tsx`: replace `document.addEventListener` in `useEffect` with `useShortcut` calls for `Cmd+K`, `Cmd+Shift+K`, `n`
  - `Header.tsx`: migrate `Escape` for mobile menu close
  - `ApprovalDrawer.tsx`: migrate `Escape` for drawer close
  - In all cases: remove the raw `document.addEventListener` + `removeEventListener` pattern
  - Dependencies: 3.1.1, 3.1.2

3.1.4. **Add terminal focus guard to existing keyboard areas**
  - In `web-app/src/components/sessions/terminal/Terminal.tsx` (or terminal container): add `data-context="terminal"` to the outermost div
  - In `web-app/src/app/page.tsx`: add `data-context="session-list"` to the session list column
  - In `web-app/src/components/review/ApprovalCard.tsx`: add `data-context="approval"` to the card
  - Dependencies: 3.1.1

---

### Story 3.2 — Session List Keyboard Navigation

**As a** power user, **I want** `j`/`k`/`Enter`/`p`/`r`/`d` to control sessions **so that** I never need the mouse for session management.

**Acceptance criteria**:
- `j` / ArrowDown: moves focus to next session in list
- `k` / ArrowUp: moves focus to previous session in list
- `Enter`: opens selected session detail
- `p`: pauses selected running session
- `r`: resumes selected paused session
- `d`: deletes with inline confirmation (not a modal — inline "Press d again to confirm" message in the row)
- `a`: attaches/focuses terminal for selected session
- All shortcuts fire only when `data-context="session-list"` is active (not when terminal or approval is focused)

**Tasks**:

3.2.1. **Register session list shortcuts in `web-app/src/app/page.tsx`**
  - Using `useShortcut` from 3.1.2: register `j` (context: `session-list`, label: `"Next session"`, action: `navigateDown`), `k` (`"Previous session"`, `navigateUp`), `Enter` (`"Open session"`, `openSelected`), `p` (`"Pause session"`, `pauseSelected`), `r` (`"Resume session"`, `resumeSelected`), `d` (`"Delete session"`, `deleteSelected`)
  - Dependencies: 3.1.2

3.2.2. **Implement inline delete confirmation in `web-app/src/components/sessions/SessionRow.tsx`**
  - When `d` pressed on a session row: add `data-confirming-delete` attribute to the row
  - Show inline text "Press d again to confirm delete" in the row (styled with `vars.color.warning`)
  - If `d` pressed again within 3 seconds: fire delete
  - If any other key pressed or 3 seconds elapsed: cancel, remove attribute
  - Dependencies: 3.2.1

3.2.3. **Add `a` shortcut to focus terminal**
  - When `a` pressed in `session-list` context: call `terminalRef.current?.focus()` on the terminal instance in column 2
  - If no session selected, `a` opens selected session's terminal
  - Dependencies: 3.2.1, 2.2.3

---

### Story 3.3 — Keyboard Shortcut Help Overlay (`?`)

**As a** user, **I want** a searchable keyboard shortcut reference overlay **so that** I can discover all available shortcuts without leaving the app.

**Acceptance criteria**:
- `?` key (when not in an input) opens the overlay
- Overlay shows all shortcuts grouped by context: Global, Session List, Approval, Terminal
- Shortcuts rendered as styled `<kbd>` elements in theme accent color
- Search input filters shortcuts by label
- Escape closes overlay
- Overlay has `role="dialog"`, `aria-label="Keyboard shortcuts"`, focus trap

**Tasks**:

3.3.1. **Create `web-app/src/components/ui/KeyboardShortcutOverlay.tsx` and `.css.ts`**
  - Reads `registry.getAll()` to get all registered shortcuts
  - Groups by `context`
  - Renders `<kbd>` elements styled with `vars.color.primary` background, `vars.color.primaryText` foreground, `vars.font.mono`
  - Search input at top filters via `label.toLowerCase().includes(query)`
  - `useFocusTrap` applied when open
  - Escape: close (registered as `{ context: "global", key: "Escape" }` on overlay mount; deregistered on close)
  - Dependencies: 3.1.1

3.3.2. **Register `?` shortcut globally**
  - In `web-app/src/lib/shortcuts/shortcutRegistry.ts` or in a top-level component: register `?` as `context: "global"`, action: `openShortcutOverlay()`
  - Wire `openShortcutOverlay` via React state in `web-app/src/app/layout.tsx`
  - Dependencies: 3.1.1, 3.3.1

3.3.3. **Add styled `<kbd>` component**
  - Create `web-app/src/components/ui/Kbd.tsx` and `Kbd.css.ts`
  - Props: `children` (key name), optional `size: "sm" | "md"`
  - CSS: `background: vars.color.primary`, `color: vars.color.primaryText`, `fontFamily: vars.font.mono`, border with `vars.color.borderStrong`, border-radius `vars.radii.sm`, padding `2px 6px`
  - Used throughout UI for inline shortcut hints
  - Dependencies: none (pure component)

---

## Epic 4: Session List Visual Redesign (Phase B)

**Goal**: Session cards communicate status at a glance through visual hierarchy: glow/pulse for running, amber for approval-needed, dimmed for paused. Sticky collapsible group headers.

---

### Story 4.1 — Scanlines Overlay and Global Effects

**As a** user in Matrix or Cyberpunk theme, **I want** ambient visual effects (scanlines) **so that** the cyberpunk aesthetic is reinforced at the global level.

**Acceptance criteria**:
- Scanlines overlay: fixed, full-viewport, `pointer-events: none`, `z-index: 9999`, invisible in WH40K and Clean themes
- Scanlines respect `prefers-reduced-motion: reduce` (disabled entirely)
- Single overlay element — not per-card
- Body/root element has `will-change: opacity` only on the pseudo-element (not the whole body)

**Tasks**:

4.1.1. **Create `web-app/src/styles/globalEffects.css.ts`**
  - `globalStyle('body', { position: 'relative' })`
  - `globalStyle('body::before', { content, position: 'fixed', inset: 0, backgroundImage: repeating-linear-gradient with `vars.color.scanlineColor`, pointerEvents: 'none', zIndex: 9999 })`
  - Wrap in `@media (prefers-reduced-motion: no-preference)` — no scanlines if reduced motion preferred
  - Dependencies: 1.1.1 (needs `scanlineColor` token)

4.1.2. **Import `globalEffects.css.ts` in `web-app/src/app/layout.tsx`**
  - Side-effect import (no named exports needed): `import '../styles/globalEffects.css'`
  - vanilla-extract processes this at build time
  - Dependencies: 4.1.1

---

### Story 4.2 — Session Card Status Glow/Pulse

**As a** user, **I want** running sessions to visually pulse and approval-needed sessions to visually demand attention **so that** status is immediately legible at a glance.

**Acceptance criteria**:
- Running session card: pulsing left border + optional glow pseudo-element using `vars.color.glowPrimary`
- Approval-needed card: amber/warning color left border, amber pulse, `vars.color.warning` glow
- Paused card: 60% opacity, grey left border
- Complete/error: muted card with status badge
- All animations disabled under `prefers-reduced-motion: reduce`
- Status NOT indicated by glow alone: status text badge always visible

**Tasks**:

4.2.1. **Create `web-app/src/styles/animations.css.ts`**
  - `pulseGlow` keyframe: `0%, 100%`: `opacity: 0.3`, `50%`: `opacity: 1` — animates a `::after` pseudo-element with `boxShadow: "0 0 12px 4px var(--glow-color)"`
  - `pulseGlowAmber` keyframe: same structure with amber `var(--glow-color-warning)` using `vars.color.warning`
  - All keyframes wrapped in `@media (prefers-reduced-motion: no-preference)`
  - Export `glowingRunning`, `glowingApproval`, `glowingPaused` style objects using these keyframes
  - Dependencies: 1.1.1

4.2.2. **Update `web-app/src/components/sessions/SessionRow.tsx` and `SessionRow.css.ts`**
  - Import `glowingRunning`, `glowingApproval`, `glowingPaused` from `animations.css.ts`
  - Apply `glowingRunning` class when `session.status === "running"`
  - Apply `glowingApproval` class when session has pending approvals
  - Apply `glowingPaused` class (opacity 0.6, static grey left border) when `session.status === "paused"`
  - Add `position: relative` to card (required for `::after` pseudo-element positioning)
  - Add `data-testid="session-row-{status}"` for visual regression targeting
  - Dependencies: 4.2.1

4.2.3. **Add risk level glow to `web-app/src/components/review/ApprovalCard.tsx`**
  - LOW risk: `vars.color.success` glow (same `glowingRunning` style)
  - MEDIUM risk: `vars.color.warning` glow (amber pulse)
  - HIGH risk: `vars.color.error` glow (red pulse, no continuous animation — static red border on load, then static)
  - HIGH risk approval also renders a text warning: "High risk action — confirm explicitly"
  - Dependencies: 4.2.1

---

### Story 4.3 — Sticky Collapsible Group Headers

**As a** user with many sessions, **I want** group headers to stick while scrolling and be collapsible **so that** I can navigate long lists without losing context.

**Acceptance criteria**:
- Group headers use `position: sticky; top: 0` within the scrolling session list column
- Clicking a group header collapses/expands that group's sessions
- Expand/collapse state per group persisted in component state (not localStorage)
- Group session count shown in header
- Keyboard: `e` in `session-list` context expands/collapses the focused group

**Tasks**:

4.3.1. **Update `web-app/src/components/sessions/GroupHeader.tsx` and `GroupHeader.css.ts`**
  - Props: `groupName`, `count`, `isCollapsed`, `onToggle`
  - CSS: `position: sticky; top: 0; zIndex: ${zIndex.raised}`; `background: vars.color.cardBackground`; left border accent `vars.color.primary`
  - Chevron icon rotates on collapse (CSS `transform: rotate(-90deg)` transition)
  - Dependencies: none (extends existing component or creates new one)

4.3.2. **Wire collapse state in session list grouping logic**
  - In `web-app/src/components/sessions/SessionList.tsx`: maintain `collapsedGroups: Set<string>` state
  - When `isCollapsed` for a group: render only the `GroupHeader`, not the session rows
  - Dependencies: 4.3.1

---

## Epic 5: Review Queue + Notification Flow Redesign (Phase B)

**Goal**: Approvals are surfaced persistently, keyboard-navigable, and wired to the notification system. Approval/deny triggers in-app toasts.

---

### Story 5.1 — Persistent Approval Banner

**As a** user with pending approvals, **I want** a persistent banner at the top of the page **so that** I never miss a session waiting for my input.

**Acceptance criteria**:
- Banner visible whenever `approvals.length > 0`, hidden otherwise (not `return null` — use CSS `visibility: hidden` or zero height so layout doesn't shift)
- Banner shows count, most-urgent session name, and "Review Now" button
- Clicking banner opens the approval panel in column 3 (context panel)
- Banner uses `vars.color.warning` background, `vars.color.warningText` foreground
- Banner has `role="alert"` and `aria-live="assertive"`

**Tasks**:

5.1.1. **Create `web-app/src/components/review/ApprovalBanner.tsx` and `ApprovalBanner.css.ts`**
  - Consumes approval count from existing approval context/hook
  - `visibility: hidden; height: 0` when no approvals vs `visibility: visible; height: 48px` (CSS transition for height may cause jank — use `transform: translateY(-100%)` approach instead)
  - "Review Now" button calls `openContextPanel("approvals")` on the context panel state
  - Dependencies: 2.2.1 (context panel exists)

5.1.2. **Mount `ApprovalBanner` in `web-app/src/app/layout.tsx` above main content**
  - Position: between drawer and main content, full width of main content area
  - Adjust `cockpitRoot` grid row template to accommodate banner height: `"auto 1fr"` rows
  - Dependencies: 5.1.1, 2.1.4

---

### Story 5.2 — Keyboard Approve/Deny

**As a** user reviewing approvals, **I want** `y` to approve and `n` to deny the focused approval card **so that** I can process approvals without using the mouse.

**Acceptance criteria**:
- `y` approves the currently focused `ApprovalCard`
- `n` denies the currently focused `ApprovalCard`
- `Shift+Y` approves all pending approvals in the current session
- After approve/deny, focus automatically advances to the next pending card
- Shortcuts only active when `data-context="approval"` is the active context

**Tasks**:

5.2.1. **Register `y`, `n`, `Shift+Y` shortcuts in `web-app/src/components/review/ApprovalPanel.tsx`**
  - `y`: context `"approval"`, action: call `approveCard(focusedCardId)`
  - `n`: context `"approval"`, action: call `denyCard(focusedCardId)`
  - `Shift+Y`: context `"approval"`, action: call `approveAll()`
  - After each action: `setFocusedCardId(nextCardId)` (advance to next)
  - Dependencies: 3.1.2

5.2.2. **Add `data-context="approval"` and focus management to `ApprovalPanel.tsx`**
  - Panel root: `data-context="approval"`
  - Track `focusedCardId` in `useState`
  - First card auto-focused when panel opens
  - When a card is resolved, move focus to next card; when last card resolved, move focus to "close panel" button
  - Dependencies: 5.2.1

---

### Story 5.3 — Approval Resolution Toasts

**As a** user, **I want** an in-app toast notification when an approval is resolved **so that** I get feedback on my actions and session owners are informed.

**Acceptance criteria**:
- Approving a tool call: toast with green glow, "Approved: `<tool-name>` for `<session-name>`"
- Denying: toast with red glow, "Denied: `<tool-name>` for `<session-name>`"
- Toast styled in active theme (mono font, neon border)
- Toast auto-dismisses after 4 seconds
- Toast position: top-right, does not overlap drawer nav

**Tasks**:

5.3.1. **Add `addApprovalResolvedNotification` to `NotificationContext.tsx`**
  - New function: `addApprovalResolvedNotification(toolName: string, sessionName: string, decision: "approved" | "denied") => void`
  - Calls existing `addNotification` with `type: decision === "approved" ? "success" : "error"`, `title`, `message`
  - Dependencies: none (extends existing context)

5.3.2. **Wire `onApprove`/`onDeny` callbacks in `ApprovalCard.tsx`**
  - After the ConnectRPC approve/deny call resolves: call `addApprovalResolvedNotification(toolName, sessionName, "approved")`
  - Import `useNotification` hook, call in the approve/deny handler callbacks
  - Dependencies: 5.3.1

5.3.3. **Style `NotificationToast.tsx` with active theme tokens**
  - Update `NotificationToast.css.ts`: use `vars.color.cardBackground` bg, `vars.color.borderColor` border, `vars.font.mono` font, `vars.color.glowPrimary`-based box-shadow on the toast container
  - Slide-in animation: `transform: translateX(100%)` → `translateX(0)`, 200ms ease-out
  - Position container: `top: ${vars.space[4]}; right: ${vars.space[4]}` in a `position: fixed` portal
  - Success variant: `vars.color.success` left border; error variant: `vars.color.error` left border
  - Dependencies: 1.1.1 (glow tokens)

---

### Story 5.4 — Approval Card Visual Redesign

**As a** user reviewing tool calls, **I want** approval cards styled with the active theme **so that** risk is immediately legible and the UX feels cohesive.

**Acceptance criteria**:
- Card slide-in animation: `transform: translateX(100%)` → `translateX(0)`, 150ms ease-out
- Syntax-highlighted tool arguments using `shiki` (already in dependencies)
- Risk level badge: LOW/MEDIUM/HIGH with appropriate `statusBadge` colors and glow
- HIGH risk card has red pulsing border
- Respects `prefers-reduced-motion`

**Tasks**:

5.4.1. **Update `web-app/src/components/review/ApprovalCard.css.ts`**
  - Add `slideIn` keyframe: `from { transform: translateX(32px); opacity: 0 }`, `to { transform: translateX(0); opacity: 1 }`
  - Card entrance animation: `animation: ${slideIn} 150ms ease-out`
  - `prefers-reduced-motion`: `animation: none`
  - Risk glow styles using `vars.color.glowPrimary` (LOW), `vars.color.warning` (MEDIUM), `vars.color.error` (HIGH)
  - Dependencies: 1.1.1

5.4.2. **Add syntax highlighting to tool arguments in `ApprovalCard.tsx`**
  - Use `shiki` (already in deps): highlight JSON/shell arguments in the card's argument preview
  - Wrap in a `<pre>` with `vars.color.terminalBackground` bg
  - Dependencies: none

---

## Epic 6: Micro-Interactions and Omnibar Effects (Phase B)

**Goal**: Omnibar opening effects per theme. View transitions for page navigation. Hover/focus states that reinforce the theme personality. All effects respect `prefers-reduced-motion`.

---

### Story 6.1 — Omnibar Theme-Specific Opening Animations

**As a** user, **I want** the omnibar to open with a theme-specific visual effect **so that** the aesthetic is reinforced even in micro-interactions.

**Acceptance criteria**:
- Matrix: scanline sweep down (CSS animation, 120ms)
- Cyberpunk77: glitch flash (3-frame translateX jitter, 80ms)
- WH40K: scale + opacity fade-in (150ms)
- Clean: simple fade-in (100ms)
- All: opens via `startTransition` to activate View Transitions
- `prefers-reduced-motion`: all themes use simple fade-in

**Tasks**:

6.1.1. **Add theme-specific open animations to `web-app/src/components/sessions/Omnibar.css.ts`**
  - `scanlineSweep` keyframe: `from { clipPath: "inset(0 0 100% 0)" }`, `to { clipPath: "inset(0 0 0% 0)" }` — creates a top-to-bottom wipe
  - `glitchFlash` keyframe: three stops — `0%: translateX(-2px)`, `33%: translateX(2px)`, `66%: translateX(-1px)`, `100%: translateX(0)` 
  - `parchmentUnfurl` keyframe: `from { transform: scaleY(0.8); opacity: 0 }`, `to { transform: scaleY(1); opacity: 1 }`
  - Theme-specific classes use CSS variable `vars.color.glowPrimary` in `boxShadow` for the modal container
  - Wrap all in `@media (prefers-reduced-motion: no-preference)` — fallback to existing `fadeIn`
  - Dependencies: 1.1.1

6.1.2. **Wire `startTransition` for omnibar open in `web-app/src/lib/contexts/OmnibarContext.tsx`**
  - Replace `setIsOmnibarOpen(true)` with `startTransition(() => setIsOmnibarOpen(true))`
  - Enable `experimental.viewTransition: true` in `web-app/next.config.js` (after auditing for `flushSync` calls — task 6.1.3)
  - Dependencies: none

6.1.3. **Audit for `flushSync` calls before enabling viewTransition**
  - Run: `grep -r "flushSync" web-app/src --include="*.ts" --include="*.tsx"`
  - Document any findings; if found, replace with `startTransition` alternatives before enabling the flag
  - Dependencies: none (prerequisite for 6.1.2)

---

### Story 6.2 — Hover and Focus State System

**As a** user, **I want** interactive elements to respond to hover and focus with theme-appropriate effects **so that** interactivity is clear and visually satisfying.

**Acceptance criteria**:
- Session cards: theme-color left border slides in on hover (`transition: border-left-color 100ms`)
- Buttons: `vars.color.glowPrimary` box-shadow appears on hover (`transition: box-shadow 150ms`)
- Focus rings: 2px `outline`, `vars.color.primary` color, 2px offset — replaces browser default on all interactive elements
- Inputs: animated gradient border on focus in Cyberpunk77 theme (uses `vars.color.glowSecondary`)
- All transitions disabled under `prefers-reduced-motion: reduce`

**Tasks**:

6.2.1. **Create `web-app/src/styles/interactiveBase.css.ts`**
  - `globalStyle('button:focus-visible, a:focus-visible, [tabIndex]:focus-visible', { outline: '2px solid var(...)' ...})` using `vars.color.primary`
  - `globalStyle('button:hover', { boxShadow: `0 0 8px 2px ${vars.color.glowPrimary}`, transition: 'box-shadow 150ms ease' })`
  - Wrap hover effects in `@media (prefers-reduced-motion: no-preference)`
  - Dependencies: 1.1.1

6.2.2. **Update `SessionRow.css.ts` with hover left-border transition**
  - Add `borderLeft: '3px solid transparent'`, `transition: 'border-left-color 100ms ease'`
  - On `:hover`: `borderLeftColor: vars.color.primary`
  - On `.selected`: `borderLeftColor: vars.color.primary` (static, no transition)
  - Dependencies: 1.1.1

6.2.3. **Import `interactiveBase.css.ts` as a side-effect in `layout.tsx`**
  - `import '../styles/interactiveBase.css'` (side-effect import — no named exports needed)
  - Dependencies: 6.2.1

---

## Epic 7: Quality Tooling and CI (Phase C)

**Goal**: Storybook with multi-theme stories. Visual regression snapshots for all 4 themes. Contrast check script. ESLint rule for hardcoded colors.

---

### Story 7.1 — Storybook Setup

**As a** developer, **I want** Storybook with all four themes **so that** I can develop and review components in isolation.

**Acceptance criteria**:
- `npm run storybook` starts Storybook in `web-app/`
- All four themes selectable via toolbar dropdown
- Stories exist for: `Button`, `Badge`, `SessionRow`, `ApprovalCard`, `Omnibar`, `KeyboardShortcutOverlay`, `NotificationToast`
- Chromatic integration configured (token stored in CI secret `CHROMATIC_PROJECT_TOKEN`)

**Tasks**:

7.1.1. **Install Storybook dependencies**
  - Run: `cd web-app && npx storybook@8 init --type nextjs --no-dev` to scaffold
  - Install: `@storybook/addon-themes`, `@chromatic-com/storybook`
  - Confirm `@storybook/nextjs` selected (not Vite)
  - Dependencies: none

7.1.2. **Configure `.storybook/main.ts`**
  - Framework: `@storybook/nextjs`
  - Addons: `["@storybook/addon-themes", "@chromatic-com/storybook", "@storybook/addon-a11y"]`
  - Apply vanilla-extract HMR workaround in `webpackFinal` if needed (filter `.vanilla.css` rule)
  - Dependencies: 7.1.1

7.1.3. **Configure `.storybook/preview.tsx`**
  - Import `withThemeByClassName` from `@storybook/addon-themes`
  - Import `THEME_CLASSES` from `web-app/src/lib/contexts/ThemeContext.tsx`
  - Decorator: `withThemeByClassName({ themes: THEME_CLASSES, defaultTheme: "matrix" })`
  - Import `globals.css` and `interactiveBase.css.ts` as side effects
  - Set `parameters.layout = "centered"` globally
  - Dependencies: 7.1.2, 1.4.1

7.1.4. **Write component stories**
  - `web-app/src/components/ui/Kbd.stories.tsx`: all sizes, all theme overrides
  - `web-app/src/components/sessions/SessionRow.stories.tsx`: running/paused/approval/complete states × 4 themes
  - `web-app/src/components/review/ApprovalCard.stories.tsx`: LOW/MEDIUM/HIGH risk × 4 themes
  - `web-app/src/components/sessions/Omnibar.stories.tsx`: open state × 4 themes
  - `web-app/src/components/ui/NotificationToast.stories.tsx`: success/error × 4 themes
  - Each story file: `// +feature: <feature-id>` marker in first 5 lines
  - Dependencies: 7.1.3

7.1.5. **Add `storybook` and `chromatic` scripts to `web-app/package.json`**
  - `"storybook": "storybook dev -p 6006"`
  - `"build-storybook": "storybook build"`
  - `"chromatic": "npx chromatic --project-token=$CHROMATIC_PROJECT_TOKEN"`
  - Dependencies: 7.1.2

---

### Story 7.2 — Multi-Theme Visual Regression with Playwright

**As a** QA engineer, **I want** Playwright snapshot tests for all four themes × key page states **so that** visual regressions are caught automatically.

**Acceptance criteria**:
- 4 Playwright projects: `matrix-theme`, `cyberpunk77-theme`, `wh40k-theme`, `clean-theme`
- Each project has `storageState` fixture setting `localStorage['stapler-theme']` appropriately
- Snapshot tests cover: session list (empty, with sessions, running session glow), approval drawer, omnibar open
- All tests set `reducedMotion: 'reduce'` to stabilize animations
- CI fails on diff ratio > 0.01

**Tasks**:

7.2.1. **Create 4 localStorage fixture files**
  - `tests/e2e/fixtures/matrix-theme.json`: `{ "origins": [{ "origin": "http://localhost:8544", "localStorage": [{ "name": "stapler-theme", "value": "matrix" }] }] }`
  - Repeat for `cyberpunk77-theme.json`, `wh40k-theme.json`, `clean-theme.json`
  - Dependencies: 1.4.1

7.2.2. **Update `tests/e2e/playwright.config.ts` to add theme projects**
  - Add 4 named projects with `use.storageState` pointing to fixtures from 7.2.1
  - Set `use.viewport: { width: 1280, height: 800 }` in each project
  - Set `snapshotPathTemplate: "tests/snapshots/{projectName}/{testFilePath}/{arg}{ext}"`
  - Dependencies: 7.2.1

7.2.3. **Create `tests/e2e/visual-regression.spec.ts`**
  - `// @feature session:list, review-queue:list`
  - Test: session list empty state — `expect(page).toHaveScreenshot("session-list-empty.png", { maxDiffPixelRatio: 0.01, animations: "disabled" })`
  - Test: session list with running session — navigate to app with seeded data, screenshot
  - Test: approval drawer open — trigger drawer, screenshot
  - Test: omnibar open — press Cmd+K, screenshot
  - All tests call `page.emulateMedia({ reducedMotion: "reduce" })` in `beforeEach`
  - Dependencies: 7.2.2

7.2.4. **Capture initial baselines**
  - Run: `cd web-app && npx playwright test tests/e2e/visual-regression.spec.ts --update-snapshots --project=matrix-theme` (and each other project)
  - Commit baseline screenshots to `tests/snapshots/`
  - Dependencies: 7.2.3

---

### Story 7.3 — Theme Contrast Check Script

**As a** developer, **I want** a CI-enforced contrast ratio checker for all theme tokens **so that** WCAG AA compliance is guaranteed for new themes.

**Acceptance criteria**:
- `npm run check-contrast` in `web-app/` reports each text/background token pair with ratio and PASS/FAIL
- CI step fails if any pair fails WCAG AA (4.5:1 for normal text, 3:1 for large text)
- Matrix green (#00ff41 on #000000) must explicitly pass
- Cyberpunk yellow (#fcee09 on #0d0d1a) must explicitly pass

**Tasks**:

7.3.1. **Create `web-app/scripts/check-theme-contrast.ts`**
  - Import all four theme objects by extracting color token values using `getComputedStyle`-style resolution (resolve the CSS variable values from JS objects directly — no browser needed)
  - Implement WCAG relative luminance formula: `L = 0.2126*R + 0.7152*G + 0.0722*B` with gamma correction
  - Contrast ratio: `(L1 + 0.05) / (L2 + 0.05)` where L1 > L2
  - Check pairs: `textPrimary vs background`, `textSecondary vs background`, `textMuted vs cardBackground`, `primaryText vs primary`
  - Output: table with token names, hex values, ratio, PASS/FAIL
  - Exit with code 1 if any FAIL
  - Dependencies: 1.2.1–1.2.4

7.3.2. **Add script to `web-app/package.json`**
  - `"check-contrast": "ts-node --project tsconfig.scripts.json scripts/check-theme-contrast.ts"`
  - Add `tsconfig.scripts.json` if not exists (CommonJS target for scripts)
  - Dependencies: 7.3.1

7.3.3. **Add contrast check to CI**
  - In `.github/workflows/ux-analysis.yml` (already exists per requirements): add step `npm run check-contrast` after `npm run lint`
  - Failure mode: blocks PR merge (not advisory)
  - Dependencies: 7.3.2

---

### Story 7.4 — ESLint Rule for Hardcoded Colors

**As a** developer, **I want** ESLint to catch inline hardcoded hex colors in `.tsx` files **so that** all color usage is tokenized.

**Acceptance criteria**:
- `npm run lint` fails if any `.tsx` file contains `style={{ color: '#...` or `style={{ backgroundColor: '#...`
- Existing violations: zero (audit before enabling as error; warn if violations found)
- Rule covers both `#rrggbb` and `#rgb` formats

**Tasks**:

7.4.1. **Add `no-restricted-syntax` rule to `web-app/eslint.config.mjs`**
  - Selector: `JSXAttribute[name.name="style"] > JSXExpressionContainer > ObjectExpression > Property[key.name=/color|background/i] > Literal[value=/^#[0-9a-fA-F]{3,8}$/]`
  - Message: "Use vars.color.* tokens from theme.css.ts instead of hardcoded hex values"
  - Start as `"warn"` during audit, upgrade to `"error"` after all existing violations resolved
  - Dependencies: none

7.4.2. **Audit and fix existing hardcoded hex violations**
  - Run: `npm run lint` with the new warn rule; collect all warnings
  - Fix each violation by replacing with the appropriate `vars.color.*` token
  - Once zero warnings: change rule severity to `"error"` in `eslint.config.mjs`
  - Dependencies: 7.4.1

---

## Migration Strategy: `globals.css` Dark Mode Conflict

This is the most critical pitfall. Execute in strict sequence:

**Step 1 (Story 1.5.1 — before any Epic 2 work)**: Audit — find every file using legacy vars.

**Step 2 (Story 1.5.2 — before layout changes)**: Migrate all `.module.css` consumers to `vars.*`. Do NOT proceed to Epic 2 until this is complete. The new cockpit layout will render broken if components still read from `globals.css` legacy vars and those vars respond to OS dark mode instead of the selected vanilla-extract theme.

**Step 3 (Story 1.5.3 — gate for Epic 2 and beyond)**: Only after all consumers migrated, delete the `@media (prefers-color-scheme: dark)` block. Run `make restart-web` and verify all 4 themes in the browser.

**Step 4 (ongoing)**: `lint:css-vars` CI step (already exists) prevents regression. The ESLint `no-restricted-syntax` rule (Story 7.4) prevents inline hex regression.

---

## Risk Register

| # | Risk | Probability | Impact | Mitigation |
|---|------|------------|--------|-----------|
| R1 | `globals.css` dark mode conflict causes visual breakage in new themes | High | High | Mandatory migration gate (Story 1.5) before Epic 2 starts; `lint:css-vars` CI enforces no new legacy vars |
| R2 | Terminal xterm.js keyboard capture causes `j`/`k` to type into terminal | High | Medium | `data-context="terminal"` attribute + registry context check (Story 3.1.4); confirmed working pattern in architecture research |
| R3 | View Transitions `flushSync` conflict breaks omnibar animation | Medium | Low | Audit task 6.1.3 before enabling `viewTransition: true`; graceful degradation (no animation, not a crash) |
| R4 | Storybook HMR instability with vanilla-extract slows development | Medium | Low | Use `@storybook/nextjs` (webpack, not Vite); document reload requirement; Chromatic handles CI visual comparison without requiring live Storybook |
| R5 | WCAG AA failure on Matrix/Cyberpunk secondary text colors | Medium | High | Contrast check script (Story 7.3) runs in CI and blocks PRs; muted text colors verified against backgrounds before merging themes |

---

## Tooling Setup Order Rationale

**Storybook first, then visual regression snapshots.**

Rationale: Playwright snapshots should be taken against known-good component states. If snapshots are taken before components are styled correctly, the baseline captures bugs. Storybook forces component-level correctness first (Chromatic catches per-component regressions), then Playwright captures full-page regression across all four themes simultaneously. This two-layer approach (component + page) catches different classes of problems without duplication.

The order:
1. Story 7.1 (Storybook) — establish component baselines
2. Story 7.2 (Playwright visual regression) — establish page-level baselines against stable components
3. Story 7.3 (contrast check) — automated token validation
4. Story 7.4 (ESLint rule) — prevent future regressions

---

## Epic / Story / Task Summary

| Epic | Title | Stories | Tasks | Phase |
|------|-------|---------|-------|-------|
| 1 | Theme System Foundation | 5 | 18 | A |
| 2 | Layout Redesign — Cockpit Shell | 3 | 12 | A |
| 3 | Keyboard-First Interaction | 3 | 10 | A |
| 4 | Session List Visual Redesign | 3 | 7 | B |
| 5 | Review Queue + Notification Flow | 4 | 12 | B |
| 6 | Micro-Interactions + Omnibar Effects | 2 | 8 | B |
| 7 | Quality Tooling and CI | 4 | 14 | C |
| **Total** | | **24** | **81** | |

> Note: Tasks counted per plan body above (some stories have sub-numbered tasks; totals reflect distinct implementation steps).

---

## Files Created / Modified: Quick Reference

**New files to create:**
- `web-app/src/app/fonts.ts`
- `web-app/src/lib/contexts/ThemeContext.tsx`
- `web-app/src/lib/contexts/NavigationContext.tsx`
- `web-app/src/lib/shortcuts/shortcutRegistry.ts`
- `web-app/src/lib/shortcuts/useShortcut.ts`
- `web-app/src/lib/omnibar/detectors/CommandDetector.ts`
- `web-app/src/styles/layout.css.ts`
- `web-app/src/styles/sessionCockpit.css.ts`
- `web-app/src/styles/globalEffects.css.ts`
- `web-app/src/styles/animations.css.ts`
- `web-app/src/styles/interactiveBase.css.ts`
- `web-app/src/components/layout/DrawerNav.tsx` + `DrawerNav.css.ts`
- `web-app/src/components/sessions/SessionDetailBar.tsx` + `SessionDetailBar.css.ts`
- `web-app/src/components/review/ApprovalBanner.tsx` + `ApprovalBanner.css.ts`
- `web-app/src/components/ui/Kbd.tsx` + `Kbd.css.ts`
- `web-app/src/components/ui/KeyboardShortcutOverlay.tsx` + `KeyboardShortcutOverlay.css.ts`
- `web-app/scripts/check-theme-contrast.ts`
- `tests/e2e/visual-regression.spec.ts`
- `tests/e2e/fixtures/matrix-theme.json` (+ 3 others)
- `.storybook/main.ts`, `.storybook/preview.tsx`
- Multiple `*.stories.tsx` files colocated with components

**Existing files with significant changes:**
- `web-app/src/styles/theme-contract.css.ts` — 5 new tokens
- `web-app/src/styles/theme.css.ts` — 4 new theme objects + update `lightTheme`/`darkTheme`
- `web-app/src/app/layout.tsx` — FOUC script, font variables, cockpit layout, drawer nav
- `web-app/src/app/page.tsx` — 3-column cockpit grid
- `web-app/src/components/sessions/SessionRow.tsx` + `.css.ts` — status glow, hover border
- `web-app/src/components/review/ApprovalCard.tsx` + `.css.ts` — slide-in, y/n shortcuts, risk glow
- `web-app/src/components/review/ApprovalPanel.tsx` — keyboard shortcuts, focus management
- `web-app/src/lib/contexts/OmnibarContext.tsx` — `startTransition`, theme command
- `web-app/src/lib/contexts/NotificationContext.tsx` — `addApprovalResolvedNotification`
- `web-app/src/lib/omnibar/actions/types.ts` — `set_theme` action
- `web-app/src/lib/omnibar/actions/dispatch.ts` — `set_theme` case
- `web-app/src/lib/omnibar/detector.ts` — `CommandDetector` registration
- `web-app/src/components/sessions/Omnibar.css.ts` — theme-specific open animations
- `web-app/next.config.js` — `experimental.viewTransition: true`
- `web-app/package.json` — new dev dependencies, scripts
- `web-app/eslint.config.mjs` — `no-restricted-syntax` rule
- `tests/e2e/playwright.config.ts` — 4 theme projects
