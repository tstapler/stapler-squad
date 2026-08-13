# Implementation Plan: Front-End Refactor

**Status**: Ready for implementation
**Branch**: `front-end-refactor`
**Created**: 2026-04-16
**Requirements**: `project_plans/front-end-refactor/requirements.md`
**Research**: `project_plans/front-end-refactor/research/synthesis.md`
**ADRs**: `project_plans/front-end-refactor/decisions/`

---

## Overview

Full front-end refactor of the stapler-squad web app across 5 phases spanning 3–6 months. The goal is to complete the vanilla-extract migration (ADR-009), introduce a Radix UI primitive library, make the app fully usable on the Pixel 9 Pro Fold, and lay the architecture foundation for future React Native code sharing.

**Locked decisions**: Next.js 15, React 19, Redux Toolkit, ConnectRPC + protobuf, vanilla-extract (ADR-009).

**New decisions made in this plan** (see `project_plans/front-end-refactor/decisions/`):
- ADR-010: Radix UI as headless primitive library
- ADR-011: `createThemeContract` + `createTheme` replacing hand-rolled `vars` wrapper
- ADR-012: RTK Query with protobuf serialization boundary for unary ConnectRPC calls
- ADR-013: Turborepo `packages/core` for web + React Native logic sharing
- ADR-014: CodeMirror 6 merge addon replaces Monaco for diff view on mobile

---

## Known Issues

### P1 — Turbopack + vanilla-extract incompatible in Next.js 15

**Description**: `package.json` dev script runs `next dev --turbopack`. The `@vanilla-extract/next-plugin` (v2.5.1) does not support Turbopack in Next.js 15 — it requires Webpack. This causes silent HMR failures: `.css.ts` files do not hot-reload, and in some cases produce stale or missing styles that only appear correct after a full rebuild. The build command (`next build`) is unaffected because it always uses Webpack.

**Files affected**:
- `web-app/package.json` (line 6: `"dev": "next dev --turbopack --port 3001"`)
- `web-app/next.config.ts` (has `experimental.turbo` config that is dead weight)

**Mitigation**: Task 1.1 removes `--turbopack` from the dev script. Do not re-add it until Next.js 16 + vanilla-extract Turbopack support land simultaneously.

**Workaround until fixed**: Run `next build && next start` for style verification, or accept that HMR for `.css.ts` files will not work.

---

### P1 — xterm.js WebGL crashes on Android (no capability check)

**Description**: `TerminalOutput.tsx` loads `WebglAddon` unconditionally. On Android (including Pixel 9 Pro Fold), `WebGL2RenderingContext` may be unavailable or the context may be lost mid-session, causing an unhandled exception that crashes the terminal panel. xterm.js does not automatically fall back to canvas rendering on context loss.

**Files affected**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

**Mitigation**: Task 2.1 adds a `typeof WebGL2RenderingContext !== 'undefined'` capability check before calling `loadAddon(new WebglAddon())`, and registers an `onContextLoss` handler that disposes the WebGL addon and falls back to the default canvas renderer.

**Reference**: xterm.js issue #2033 — `onContextLoss` dispose pattern confirmed.

---

### P2 — xterm.js Android GBoard composition event corruption

**Description**: When using GBoard (Google's default Android keyboard) on xterm.js, `compositionstart` / `compositionend` events fire for every keystroke (even non-IME keys like backspace), corrupting the terminal input stream. This is a known upstream bug (xterm.js #3600). The issue exists in xterm 6.x and has no upstream fix as of the research date.

**Files affected**:
- `web-app/src/components/sessions/TerminalOutput.tsx`
- `web-app/src/lib/terminal/TerminalStreamManager.ts`

**Mitigation**: Task 2.3 applies an input event filter that discards composition events on Android (`navigator.userAgent` check) and uses the `VirtualKeyboard API` (`navigator.virtualKeyboard`) to intercept input before xterm processes it. The existing `VirtualKeyboard.tsx` component provides the overlay path. If the upstream fix ships before Phase 2 starts, this task can be dropped.

**Open question**: Manual test on physical Pixel 9 Pro Fold required before committing to the mitigation strategy. The `VirtualKeyboard.tsx` overlay path may suffice as a complete replacement rather than a filter.

---

### P2 — ConnectRPC server-streaming not shareable with React Native

**Description**: React Native's Fetch polyfill is XHR-based and cannot stream. Every hook that calls a ConnectRPC server-streaming endpoint (`SessionService.StreamTerminalOutput`, `SessionService.WatchSessions`, etc.) is web-only and cannot be moved to `packages/core`. If transport is imported directly rather than injected, the architectural boundary cannot be enforced at compile time.

**Files affected**:
- `web-app/src/lib/hooks/useSessionService.ts`
- All future service hooks under `web-app/src/lib/hooks/`

**Mitigation**: Task 3.1 refactors service hooks to accept transport as a constructor/parameter argument (dependency injection). Phase 3 establishes the boundary before Phase 5 starts the Turborepo restructure. The split is: unary hooks go to `packages/core` (Phase 5); streaming hooks stay in `apps/web` permanently.

---

### P2 — Review queue touch targets undersized

**Description**: `ReviewQueuePanel.tsx` approve/reject action buttons are icon-only with no explicit height/width, rendering below the 44×44 dp minimum touch target required by Android Material Design and WCAG 2.5.5. On a Pixel 9 Pro Fold with foldable inner screen scaling, these buttons are effectively untappable.

**Files affected**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/components/sessions/ReviewQueuePanel.module.css` (if exists)

**Mitigation**: Task 2.2 is a targeted fix ahead of the full CSS migration. Buttons are replaced with full-width labeled variants (`min-height: 56px`) using the new `Button` primitive from Phase 1. No module.css migration required — the Button primitive handles its own styles.

---

## Phase 1 — Foundation (Epic: Build-System and Token Contract)

**Goal**: Every subsequent phase builds on a stable build system and shared token contract. No visible UI changes.

**Phase gate**: `npm run dev` works without `--turbopack`; `npm run build` passes with zero vanilla-extract errors; `theme.css.ts` exports a full `createThemeContract`-based `vars` object; 5 primitive components exist in `components/ui/` with passing unit tests.

---

### Story 1.1 — Remove Turbopack from dev build

**As a developer**, I need the dev server to use Webpack so that vanilla-extract hot-module replacement works correctly.

**Acceptance criteria**:
- `npm run dev` starts the Next.js dev server without `--turbopack`
- Editing any `.css.ts` file triggers an HMR update visible in the browser within 5 seconds
- `next.config.ts` `experimental.turbo` block is removed (it was only needed for Turbopack)
- No other developer scripts are broken

**Tasks**:

#### Task 1.1.1 — Remove `--turbopack` flag and dead Turbopack config (1 hour)

**Files**: `web-app/package.json`, `web-app/next.config.ts`

**What to do**:
1. In `web-app/package.json` line 6, change `"dev": "next dev --turbopack --port 3001"` to `"dev": "next dev --port 3001"`.
2. In `web-app/next.config.ts`, remove the entire `experimental.turbo` block (lines 24–30). The `experimental.optimizePackageImports` entry can stay.
3. Run `npm run dev` and verify the server starts. Open the app in a browser and confirm the page loads.
4. Edit `web-app/src/components/shared/VcsStatusDisplay.css.ts` (the one existing `.css.ts` file), save, and verify the browser reflects the change without a full page reload.

**Completion criteria**: `npm run dev` outputs `Ready in Xms` without any vanilla-extract warning; HMR works for `.css.ts` files.

---

### Story 1.2 — Migrate `theme.css.ts` to `createThemeContract`

**As a developer**, I need a full typed token contract so that every `.css.ts` component can reference tokens without string-based `var()` calls, catching typos at TypeScript compile time.

**Acceptance criteria**:
- `theme.css.ts` uses `createThemeContract` + `createTheme` to define `lightTheme` and `darkTheme`
- `vars` export retains the same shape as today so existing consumers (`VcsStatusDisplay.css.ts`) require only a one-line import change
- Token contract includes: color (text, background, border, status, action), space (scale 1–16), radii, fontSize, fontFamily
- `globals.css` CSS custom properties remain as a bridge for legacy `.module.css` consumers
- `npm run build` passes with zero TypeScript errors

**Tasks**:

#### Task 1.2.1 — Define the theme contract structure (2 hours)

**Files**: `web-app/src/styles/theme.css.ts`, `web-app/src/styles/theme-contract.css.ts` (new)

**What to do**:
1. Create `web-app/src/styles/theme-contract.css.ts` using `createThemeContract({})`. Define the contract shape — every key is `null` (contract placeholder). Include all token groups:
   ```ts
   // theme-contract.css.ts
   import { createThemeContract } from '@vanilla-extract/css';
   export const vars = createThemeContract({
     color: {
       textPrimary: null, textSecondary: null, textMuted: null, textDisabled: null, textInverse: null,
       background: null, cardBackground: null, hoverBackground: null, modalBackground: null,
       borderColor: null, inputBorder: null, inputFocusBorder: null,
       actionPrimary: null, actionPrimaryHover: null, actionPrimaryActive: null, actionPrimaryText: null,
       statusSuccess: null, statusSuccessBg: null,
       statusWarning: null, statusWarningBg: null,
       statusDanger: null, statusDangerBg: null, statusDangerText: null,
       terminalBackground: null, terminalForeground: null, terminalBorder: null,
       inputBackground: null, inputText: null,
       overlay: null,
     },
     space: { 0: null, 1: null, 2: null, 3: null, 4: null, 6: null, 8: null, 12: null, 16: null },
     radii: { sm: null, md: null, lg: null, full: null },
     fontSize: { xs: null, sm: null, base: null, lg: null, xl: null },
     fontFamily: { sans: null, mono: null },
   });
   ```
2. Update `web-app/src/styles/theme.css.ts` to implement two themes using `createTheme(vars, {...})`:
   - `export const lightTheme = createTheme(vars, { color: { textPrimary: '#111827', ... }, ... })`
   - `export const darkTheme = createTheme(vars, { color: { textPrimary: '#f9fafb', ... }, ... })`
   - Map every token value to the existing CSS custom property value from `globals.css` (reference the token values defined there, do not change the visual output).
3. Export `vars` from `theme.css.ts` (re-export from `theme-contract.css.ts`) so existing `import { vars } from '../../styles/theme.css'` imports keep working.

**Completion criteria**: `tsc --noEmit` passes; `VcsStatusDisplay.css.ts` still compiles after changing its import to `import { vars } from '../../styles/theme.css'` (no-op if it already points there).

#### Task 1.2.2 — Apply theme class to root layout (1 hour)

**Files**: `web-app/src/app/layout.tsx`, `web-app/src/styles/theme.css.ts`

**What to do**:
1. In the root `layout.tsx`, import `lightTheme` and `darkTheme` from `theme.css.ts`.
2. Apply `lightTheme` as the default class on the `<html>` element. For dark mode, read the user's `prefers-color-scheme` and toggle to `darkTheme` using a small `"use client"` wrapper component (`ThemeProvider.tsx` in `components/providers/`).
3. Verify light and dark mode CSS custom properties resolve correctly by toggling the OS color scheme in browser devtools.

**Completion criteria**: `<html>` element has the vanilla-extract theme class applied; CSS custom property values match the existing `globals.css` tokens for both modes.

---

### Story 1.3 — Build 5 core primitive components

**As a developer**, I need typed, composable UI primitives so that new screens and Phase 2 fixes can use them instead of bespoke CSS.

**Acceptance criteria**:
- `components/ui/Button`, `Badge`, `Input`, `Card`, `Modal` exist with `.css.ts` sibling files
- Each primitive uses `recipe()` from `@vanilla-extract/recipes` for variants
- Each primitive has a `"use client"` directive where needed (Modal wraps Radix Dialog)
- Each primitive has a passing Jest + React Testing Library unit test
- No new `.module.css` files created

**Dependencies before starting**: Story 1.2 complete (token contract available).

**Tasks**:

#### Task 1.3.1 — Install Radix UI dependencies (30 minutes)

**Files**: `web-app/package.json`

**What to do**:
1. Run: `npm install @radix-ui/react-dialog @radix-ui/react-slot --save` from `web-app/`.
2. Verify `npm run build` still passes after install.
3. Add `@vanilla-extract/recipes` if not already present: `npm install @vanilla-extract/recipes --save-dev`.

**Completion criteria**: Packages appear in `package.json` dependencies; `npm run build` clean.

#### Task 1.3.2 — Button primitive (2 hours)

**Files**: `web-app/src/components/ui/Button/Button.tsx`, `web-app/src/components/ui/Button/Button.css.ts`, `web-app/src/components/ui/Button/Button.test.tsx`, `web-app/src/components/ui/index.ts`

**What to do**:
1. Create `Button.css.ts` using `recipe()` with variants:
   - `intent`: `primary | secondary | danger | ghost`
   - `size`: `sm | md | lg`
   - `defaultVariants`: `{ intent: 'primary', size: 'md' }`
   - All values reference `vars` tokens — no hardcoded hex or `var()` strings.
   - `sm` size: `min-height: 32px`; `md`: `min-height: 40px`; `lg`: `min-height: 56px` (satisfies 44dp touch target at lg).
2. Create `Button.tsx`: `"use client"` at top; use `@radix-ui/react-slot` for `asChild` prop; forward `ref`.
3. Create `Button.test.tsx`: render each variant, assert accessible role `button`, assert `onClick` fires.
4. Export from `components/ui/index.ts`.

**Completion criteria**: `jest Button.test.tsx` passes; `tsc --noEmit` passes; Button renders in isolation.

#### Task 1.3.3 — Badge primitive (1 hour)

**Files**: `web-app/src/components/ui/Badge/Badge.tsx`, `web-app/src/components/ui/Badge/Badge.css.ts`, `web-app/src/components/ui/Badge/Badge.test.tsx`

**What to do**:
1. `Badge.css.ts` with `recipe()` variants: `intent` (default | success | warning | danger | info), `size` (sm | md).
2. `Badge.tsx`: server component (no `"use client"` needed — no interactivity); renders a `<span>`.
3. Test: renders children, applies correct class for each intent.

**Completion criteria**: `jest Badge.test.tsx` passes.

#### Task 1.3.4 — Input primitive (2 hours)

**Files**: `web-app/src/components/ui/Input/Input.tsx`, `web-app/src/components/ui/Input/Input.css.ts`, `web-app/src/components/ui/Input/Input.test.tsx`

**What to do**:
1. `Input.css.ts` with `recipe()` variants: `size` (sm | md | lg), `state` (default | error | disabled).
2. `Input.tsx`: `"use client"`; forward ref to native `<input>`; accept `label`, `error`, and `helperText` props; use `aria-describedby` for error.
3. Min height on `md` size: `44px` (touch target compliance).
4. Test: renders label, renders error message, forwards ref.

**Completion criteria**: `jest Input.test.tsx` passes; axe-core accessibility check shows no violations for error state.

#### Task 1.3.5 — Card primitive (1 hour)

**Files**: `web-app/src/components/ui/Card/Card.tsx`, `web-app/src/components/ui/Card/Card.css.ts`, `web-app/src/components/ui/Card/Card.test.tsx`

**What to do**:
1. `Card.css.ts`: base style (background, border, border-radius, padding) using `vars` tokens; variant `padding` (none | sm | md | lg).
2. `Card.tsx`: server component; renders a `<div>` with `data-testid` support.
3. Test: renders children, applies padding variant class.

**Completion criteria**: `jest Card.test.tsx` passes.

#### Task 1.3.6 — Modal primitive (Radix Dialog wrapper) (2 hours)

**Files**: `web-app/src/components/ui/Modal/Modal.tsx`, `web-app/src/components/ui/Modal/Modal.css.ts`, `web-app/src/components/ui/Modal/Modal.test.tsx`

**What to do**:
1. `Modal.css.ts`: overlay style (full-screen semi-transparent), content style (centered card, max-width variants: sm | md | lg | full).
2. `Modal.tsx`: `"use client"`; thin wrapper over `@radix-ui/react-dialog` (`Dialog.Root`, `Dialog.Portal`, `Dialog.Overlay`, `Dialog.Content`); expose `open`, `onOpenChange`, `title`, `description`, `children` props; wire `Dialog.Title` and `Dialog.Description` for accessibility.
3. Test: open/close state change, Escape key closes modal (use `@testing-library/user-event`), title renders.

**Completion criteria**: `jest Modal.test.tsx` passes; dialog closes on Escape; `aria-modal="true"` present on content.

---

## Phase 2 — Mobile Critical Path (Epic: Pixel 9 Pro Fold Usability)

**Goal**: The app is fully usable on Pixel 9 Pro Fold. Zero P1/P2 crashes or layout breaks on the target device.

**Phase gate**: xterm.js runs on Android without crashing; review queue approve/reject buttons are tappable; foldable breakpoint CSS variable defined; navigation shell renders correctly at all three breakpoints (outer ~390px, fold 600px, inner ~900px).

**Dependencies**: Phase 1 complete (Button primitive available for Task 2.2).

---

### Story 2.1 — Fix xterm.js WebGL crash on Android

**As a user on Pixel 9 Pro Fold**, I need the terminal to not crash so I can see Claude's output.

**Acceptance criteria**:
- `TerminalOutput.tsx` checks `typeof WebGL2RenderingContext !== 'undefined'` before loading `WebglAddon`
- `onContextLoss` callback is registered; on context loss, WebGL addon is disposed and canvas renderer takes over
- Terminal remains functional after context loss on Android (no white screen, no uncaught exception)
- Desktop behavior is unchanged

**Tasks**:

#### Task 2.1.1 — Add WebGL capability guard and context-loss fallback (2 hours)

**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`

**What to do**:
1. Locate where `WebglAddon` is imported and loaded. Wrap the `loadAddon(new WebglAddon())` call in:
   ```ts
   const webglAddon = new WebglAddon();
   webglAddon.onContextLoss(() => {
     webglAddon.dispose();
     // Terminal falls back to default canvas renderer automatically
   });
   if (typeof WebGL2RenderingContext !== 'undefined') {
     terminal.loadAddon(webglAddon);
   }
   ```
2. Ensure the `WebglAddon` import is not loaded at module-level on the server (it references `WebGL2RenderingContext`). Use a dynamic import inside the `useEffect` that initializes the terminal:
   ```ts
   const { WebglAddon } = await import('@xterm/addon-webgl');
   ```
3. Add a comment referencing xterm.js issue #2033.

**Completion criteria**: `npm run build` passes; browser console shows no `WebGL2RenderingContext` errors on Chrome Android simulation (DevTools mobile emulation); existing terminal unit tests pass.

---

### Story 2.2 — Fix review queue touch targets (P2 UX, highest-priority workflow)

**As a user on mobile**, I need approve and reject buttons to be large enough to tap accurately so I can process the review queue without frustration.

**Acceptance criteria**:
- All interactive buttons in `ReviewQueuePanel.tsx` have `min-height: 56px` (or use the `Button` primitive at `size="lg"`)
- Approve and reject buttons display text labels alongside icons (not icon-only)
- Touch targets verified with DevTools mobile emulation accessibility inspector
- Existing desktop layout is unchanged (buttons stack vertically on mobile, remain inline on desktop)

**Tasks**:

#### Task 2.2.1 — Replace icon-only action buttons with labeled Button primitives (2 hours)

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/ApprovalCard.tsx`

**What to do**:
1. Open `ReviewQueuePanel.tsx` and `ApprovalCard.tsx`. Identify every approve/reject/dismiss action button.
2. Replace each with the `Button` primitive (`size="lg"` maps to `min-height: 56px`):
   - Approve: `<Button intent="primary" size="lg">Approve</Button>`
   - Reject: `<Button intent="danger" size="lg">Reject</Button>`
   - Keep any icons but add visible text via `<span className={srOnly}>` for screen readers if design requires icon-only on tablet breakpoint.
3. On mobile breakpoints (below 600px), buttons should be `width: 100%` — use `style={{ width: '100%' }}` or add a `fullWidth` variant to Button.
4. Remove any inline styles or CSS Module classes on the old buttons that conflicted with sizing.

**Completion criteria**: DevTools Accessibility inspector shows touch target size >= 44×44px for all action buttons; `jest ReviewQueuePanel` tests (if any) pass; visual review on desktop unchanged.

---

### Story 2.3 — xterm.js Android input hardening

**As a user on Pixel 9 Pro Fold**, I need to type in the terminal without input corruption so I can interact with Claude.

**Acceptance criteria**:
- Terminal accepts regular typed input on Android without duplicate characters or corrupted sequences
- Composition events from GBoard are filtered before reaching xterm's input handler on Android
- Fallback: if composition filtering causes issues, the existing `VirtualKeyboard.tsx` overlay is enabled automatically on Android
- Desktop behavior is unchanged

**Tasks**:

#### Task 2.3.1 — Add GBoard composition event filter (2 hours)

**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/lib/terminal/TerminalStreamManager.ts`

**What to do**:
1. In the xterm initialization `useEffect`, detect Android: `const isAndroid = /Android/i.test(navigator.userAgent)`.
2. If `isAndroid`, attach a `compositionstart` listener on the xterm textarea element that calls `event.preventDefault()` for single-character composition events (i.e., where the composed text is a single ASCII character — these are false IME events from GBoard).
   ```ts
   if (isAndroid) {
     const textarea = terminal.element?.querySelector('textarea');
     textarea?.addEventListener('compositionstart', (e) => {
       // GBoard fires compositionstart for every keystroke; suppress false IME events
       // Real IME input has data.length > 1 or contains non-ASCII
     });
   }
   ```
3. Add a `// xterm.js #3600 — GBoard composition event workaround` comment.
4. If the filter causes regression (tested on device), fall back to enabling the `VirtualKeyboard.tsx` overlay by default on Android via a `localStorage` flag.

**Note**: This task is blocked by a manual test on the physical Pixel 9 Pro Fold. If the test reveals the issue is not reproducible with xterm 6.x, skip this task and mark it resolved.

**Completion criteria**: Manual test on Pixel 9 Pro Fold shows clean input for common keystrokes (letters, backspace, Enter, Ctrl+C).

---

### Story 2.4 — Foldable breakpoint and responsive layout shell

**As a user on Pixel 9 Pro Fold**, I need the app to use the large inner screen's full width and adapt to the outer screen, so the layout feels native to the device.

**Acceptance criteria**:
- CSS custom property `--breakpoint-fold: 600px` defined in `globals.css`
- `@media (horizontal-viewport-segments: 2)` used as progressive enhancement for two-column layout around the hinge (Chrome 138+ Pixel 9 Pro Fold)
- `Header.tsx` collapses to a bottom navigation bar below 600px
- Session list + detail pane use a two-column layout above 900px (inner screen)
- All breakpoint changes use the `ViewportProvider` context (already exists) for React-side responsive logic

**Tasks**:

#### Task 2.4.1 — Add foldable breakpoint token to globals.css (30 minutes)

**Files**: `web-app/src/app/globals.css`

**What to do**:
1. Add to the `:root` block:
   ```css
   --breakpoint-outer: 390px;   /* Pixel 9 Pro Fold outer screen */
   --breakpoint-fold: 600px;    /* between outer and inner */
   --breakpoint-inner: 900px;   /* Pixel 9 Pro Fold inner screen */
   --breakpoint-desktop: 1280px;
   ```
2. Also add to `theme-contract.css.ts` under a new `breakpoint` group so vanilla-extract components can reference them (even though breakpoints are not typically used in style rules directly — they're referenced in `@layer` media queries via string interpolation in vanilla-extract's `globalStyle`).

**Completion criteria**: `npm run lint:css-vars` passes; variables render in browser devtools.

#### Task 2.4.2 — Add viewport-segments media query for hinge-aware layout (1 hour)

**Files**: `web-app/src/app/globals.css`

**What to do**:
1. Add a global CSS rule using the Viewport Segments API:
   ```css
   @media (horizontal-viewport-segments: 2) {
     :root {
       --fold-left-width: env(viewport-segment-width 0 0);
       --fold-right-width: env(viewport-segment-width 1 0);
       --fold-gap: env(viewport-segment-left 1 0);
     }
   }
   ```
2. Add a comment: `/* Chrome 138+ Pixel 9 Pro Fold — progressive enhancement only */`.
3. The variables are consumed in Task 2.4.4.

**Completion criteria**: Variables defined in CSS; no lint errors; rule has no effect on browsers that don't support the API (graceful degradation).

#### Task 2.4.3 — Update ViewportProvider with fold breakpoints (1 hour)

**Files**: `web-app/src/components/providers/ViewportProvider.tsx`

**What to do**:
1. Add `isFoldable: boolean` and `isInnerScreen: boolean` to the `ViewportContext` type.
2. `isFoldable` is `true` when `window.innerWidth >= 600 && window.innerWidth < 900`.
3. `isInnerScreen` is `true` when `window.innerWidth >= 900`.
4. Update the `ResizeObserver` callback (or existing `matchMedia` listeners) to set these new flags.
5. Export the updated context type.

**Completion criteria**: `jest ViewportProvider.test.tsx` (update or create) passes with mock window widths.

#### Task 2.4.4 — Rebuild Header.tsx as responsive nav shell (3 hours)

**Files**: `web-app/src/components/layout/Header.tsx`, `web-app/src/components/layout/Header.css.ts` (new), `web-app/src/components/layout/Navigation.tsx` (if separate), `web-app/src/components/layout/BottomNav.tsx` (new)

**What to do**:
1. Create `Header.css.ts` using vanilla-extract `style()` and `globalStyle()`. At `> 600px`: horizontal top bar. At `<= 600px`: header is hidden; navigation moves to a bottom-fixed bar.
2. Implement `BottomNav.tsx`: `"use client"`; 4 nav items (Sessions, Review Queue, History, Settings) as icon + label tiles; each `min-height: 64px`; uses `Button` primitive with `intent="ghost"`.
3. In `Header.tsx`, import `useViewport()` hook; conditionally render `BottomNav` vs. top bar based on `isMobile`.
4. Foldable two-column: when `isInnerScreen` is true and `window.matchMedia('(horizontal-viewport-segments: 2)').matches`, use CSS grid with `--fold-left-width` and `--fold-right-width` for the main layout.
5. No `.module.css` file — all styles in `Header.css.ts`.

**Completion criteria**: Header renders bottom nav at 390px viewport width; top bar at 900px; foldable layout activates correctly in DevTools with viewport segments emulation.

---

## Phase 3 — Server State Cleanup (Epic: Data Fetching Architecture)

**Goal**: Consistent data-fetching pattern; `useSessionService` god hook broken into focused hooks; Redux `serializableCheck` re-enabled.

**Phase gate**: `useApprovals`, `useApprovalRules`, and `useApprovalAnalytics` are RTK Query endpoints; Redux `serializableCheck` passes; `ReviewQueueContext` and `ApprovalsContext` removed; filter state in URL params.

**Dependencies**: Phase 1 complete.

---

### Story 3.1 — Split useSessionService god hook

**As a developer**, I need focused, single-purpose data hooks so that I can understand what data each component depends on.

**Acceptance criteria**:
- `useSessionService.ts` no longer contains polling/query logic for approvals or approval rules
- `useApprovals`, `useApprovalRules`, `useApprovalAnalytics` are implemented as RTK Query endpoints
- Streaming hooks (`useTerminalStream`, `useWatchSessions`) remain as manual dispatch hooks — not moved to RTK Query
- All existing functionality works; no regressions in review queue or session list

**Tasks**:

#### Task 3.1.1 — Install and configure RTK Query base (2 hours)

**Files**: `web-app/src/lib/store/store.ts`, `web-app/src/lib/api/connectApi.ts` (new), `web-app/src/lib/api/serialization.ts` (new)

**What to do**:
1. `@reduxjs/toolkit` already includes RTK Query — no new install needed.
2. Create `web-app/src/lib/api/serialization.ts`: export a `toPlainObject<T extends Message>(msg: T): JsonValue` function that calls `msg.toJson()` from `@bufbuild/protobuf`. This is the protobuf serialization boundary (see ADR-012).
3. Create `web-app/src/lib/api/connectApi.ts`: implement a `createApi` instance with a custom `baseQuery` that:
   - Accepts `{ method: (req) => Promise<Response>, request: unknown }` as argument
   - Calls the ConnectRPC unary method
   - Catches `ConnectError` and maps to `{ error: { status: code, error: message } }`
   - Returns `toPlainObject(response)` on success
4. Register the API reducer and middleware in `store.ts`.

**Completion criteria**: `tsc --noEmit` passes; no circular import errors.

#### Task 3.1.2 — Migrate approval polling to RTK Query endpoints (3 hours)

**Files**: `web-app/src/lib/api/approvalsApi.ts` (new), `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

**What to do**:
1. Create `approvalsApi.ts` injecting endpoints into `connectApi`:
   - `getApprovals`: queries `SessionService.ListApprovals`; `providesTags: ['Approvals']`; polls every 5 seconds via `pollingInterval: 5000`.
   - `getApprovalRules`: queries `SessionService.ListApprovalRules`; `providesTags: ['ApprovalRules']`.
   - `getApprovalAnalytics`: queries `SessionService.GetApprovalAnalytics`; `providesTags: ['ApprovalAnalytics']`.
   - `approveRequest`: mutates `SessionService.ApproveRequest`; `invalidatesTags: ['Approvals']`.
   - `rejectRequest`: mutates `SessionService.RejectRequest`; `invalidatesTags: ['Approvals']`.
2. In `ReviewQueuePanel.tsx`, replace `useContext(ApprovalsContext)` / `useSelector` with `useGetApprovalsQuery()` from `approvalsApi`.
3. Remove the polling `setInterval` / `useEffect` from `useSessionService.ts` for approvals.

**Completion criteria**: Review queue still displays approvals; approve/reject actions still work; `serializableCheck` middleware no longer logs protobuf warnings for approval data.

#### Task 3.1.3 — Remove redundant context providers (1 hour)

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, any files that import `ApprovalsContext` or `ReviewQueueContext`

**What to do**:
1. Use `grep` (via the Grep tool) to find all consumers of `ApprovalsContext` and `ReviewQueueContext`.
2. Verify each is now using RTK Query or Redux selectors instead.
3. Delete the context files and remove their `Provider` wrappers from the component tree.
4. Fix any resulting TypeScript errors.

**Completion criteria**: `tsc --noEmit` passes; no references to the deleted contexts remain.

#### Task 3.1.4 — Migrate filter state to URL search params (2 hours)

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/lib/hooks/useFilterState.ts` (new)

**What to do**:
1. Create `useFilterState.ts` using Next.js `useSearchParams` and `useRouter`. Expose `filterState` and `setFilter` where each filter value is read from/written to URL params.
2. Replace Redux filter slice state (if any) or local `useState` filter variables in `ReviewQueuePanel.tsx` with `useFilterState`.
3. Verify that filter state persists across page refreshes and can be shared via URL.

**Completion criteria**: Changing a filter updates the URL; refreshing the page restores the same filter; `tsc --noEmit` passes.

---

## Phase 4 — Full CSS Migration (Epic: Zero .module.css)

**Goal**: All 70 `.module.css` files replaced with vanilla-extract `.css.ts` files. Zero CSS Module files remain in `web-app/src/`.

**Phase gate**: `find web-app/src -name "*.module.css" | wc -l` returns 0; `npm run build` passes; `npm run lint` passes; no visual regressions in Playwright e2e suite.

**Dependencies**: Phase 1 complete (token contract). Phase 2 complete (primitive library has sufficient coverage to absorb component migrations).

**Migration order** (highest-traffic screens first):
1. Session list + cards (`SessionCard`, `SessionList`, `SessionGrid`)
2. Review queue (`ReviewQueuePanel`, `ApprovalCard`, `ApprovalPanel`)
3. Terminal view (`TerminalOutput`)
4. Diff / file viewer (`DiffViewer`, `FileContentViewer`, `FilesTab`)
5. History (`HistoryEntryCard`, `HistoryFilterBar`, `HistoryGroupView`)
6. Layout shell (`Header`, `Navigation`)
7. Remaining leaf components (logs, shared, settings)

---

### Story 4.1 — Session list and card CSS migration

**As a developer**, I need session list components to use vanilla-extract so I can extend them without touching legacy CSS.

**Acceptance criteria**:
- `SessionCard.module.css`, `SessionList.module.css` (and any sibling module.css files for session list) are deleted
- Replacement `.css.ts` files exist and use `vars` token references exclusively
- Visual output is pixel-equivalent to before migration (verified by Playwright screenshot diff)
- No new `var()` strings in `.css.ts` files

**Tasks**:

#### Task 4.1.1 — Migrate SessionCard styles (3 hours)

**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionCard.css.ts` (new), `web-app/src/components/sessions/SessionCard.module.css` (delete)

**What to do**:
1. Open `SessionCard.module.css`. For each CSS class, create a corresponding `style()` or `recipe()` in `SessionCard.css.ts`.
2. Map CSS custom property references to `vars` token equivalents (e.g., `var(--card-background)` → `vars.color.cardBackground`).
3. For any runtime-dynamic values (e.g., a status color driven by a prop), use the CSS custom property bridge pattern: pass value as inline style `--card-accent: props.color`, consume via `var(--card-accent, fallback)` in the `.css.ts` file.
4. Update `SessionCard.tsx` imports: `import styles from './SessionCard.module.css'` → `import * as styles from './SessionCard.css'`.
5. Delete `SessionCard.module.css`.

**Completion criteria**: `SessionCard.module.css` does not exist; `npm run build` passes; Playwright screenshot diff shows no visual change.

#### Task 4.1.2 — Migrate remaining session list component styles (4 hours)

**Files**: All `*.module.css` files under `web-app/src/components/sessions/` that relate to session list display (identify with Grep).

**What to do**:
1. Use the Grep tool to list all `*.module.css` files under `sessions/`.
2. For each file not already migrated, apply the same pattern as Task 4.1.1.
3. Batch the smaller, simpler module.css files (skeleton loaders, badges, status indicators) in this task.

**Completion criteria**: All session list / card `.module.css` files deleted; `npm run build` clean.

---

### Story 4.2 — Replace Monaco diff viewer with CodeMirror merge addon

**As a user on mobile**, I need the diff viewer to be lightweight and touch-friendly so I can review changes without a ~500KB Monaco download.

**Acceptance criteria**:
- `DiffViewer.tsx` no longer imports `@monaco-editor/react`
- Diff display uses `@codemirror/merge` (CodeMirror 6 merge addon)
- Mobile touch scrolling works natively (no pointer capture conflicts)
- Bundle size reduction of at least 200 KB gzip (size-limit check)
- Syntax highlighting for Go, TypeScript, Python, Rust preserved (existing `@codemirror/lang-*` packages already installed)

**Tasks**:

#### Task 4.2.1 — Install @codemirror/merge and uninstall Monaco (2 hours)

**Files**: `web-app/package.json`, `web-app/src/components/sessions/DiffViewer.tsx`

**What to do**:
1. `npm install @codemirror/merge --save` from `web-app/`.
2. `npm uninstall @monaco-editor/react --save` (check no other component imports Monaco first).
3. Use Grep to confirm no remaining imports of `@monaco-editor/react` or `monaco-editor`.

**Completion criteria**: `package.json` no longer lists `@monaco-editor/react`; `npm run build` passes.

#### Task 4.2.2 — Rewrite DiffViewer.tsx using CodeMirror merge addon (4 hours)

**Files**: `web-app/src/components/sessions/DiffViewer.tsx`, `web-app/src/components/sessions/DiffViewer.css.ts` (new), `web-app/src/components/sessions/DiffViewer.module.css` (delete if exists)

**What to do**:
1. Implement `DiffViewer.tsx` using `MergeView` from `@codemirror/merge`:
   ```tsx
   "use client";
   import { MergeView } from '@codemirror/merge';
   import { EditorState } from '@codemirror/state';
   // ... language extensions from existing @codemirror/lang-* packages
   ```
2. Accept props: `original: string`, `modified: string`, `language: string` (maps to the appropriate `@codemirror/lang-*` extension).
3. Use `touch-action: pan-y` on the container to allow native vertical scroll on mobile.
4. Style with `DiffViewer.css.ts` using token-based colors for added/removed line backgrounds.
5. Run `npm run size-limit` and verify the "Total JS bundle" entry has decreased.

**Completion criteria**: Diff view renders for Go and TypeScript files; Playwright e2e diff viewer test passes; `npm run size-limit` passes.

---

### Story 4.3 — Codemod remaining .module.css files

**As a developer**, I need all remaining CSS Module files migrated so the codebase has a single CSS authoring standard.

**Acceptance criteria**:
- Zero `.module.css` files under `web-app/src/` after this story
- `npm run lint:css-vars` script is removed (no longer needed; all CSS is type-safe via vanilla-extract)
- `check-css-vars.mjs` script is archived or deleted

**Tasks**:

#### Task 4.3.1 — Audit and triage remaining .module.css files (1 hour)

**Files**: All remaining `*.module.css` files (identify exact list with Glob tool)

**What to do**:
1. Run Glob for `**/*.module.css` under `web-app/src/`. Generate a prioritized list of the remaining files.
2. Categorize each as: (a) straightforward token mapping, (b) dynamic value — needs CSS custom property bridge, (c) complex selector — needs manual attention.
3. Estimate count per category. If > 20 files remain in category (a), consider a codemod script.

**Completion criteria**: Triage list documented as a comment in this task's PR or as a `docs/tasks/css-migration-triage.md` note.

#### Task 4.3.2 — Migrate history, layout, and logs components (4 hours)

**Files**: All `*.module.css` files under `web-app/src/components/history/`, `web-app/src/components/layout/`, `web-app/src/components/logs/`

**What to do**:
1. Apply the same migration pattern as Task 4.1.1 to each file in these directories.
2. For layout components already rebuilt in Phase 2 (Header, Navigation), verify no `.module.css` file was left behind.

**Completion criteria**: Named directories have zero `.module.css` files; `npm run build` passes.

#### Task 4.3.3 — Migrate remaining shared and app-level styles (3 hours)

**Files**: All `*.module.css` files under `web-app/src/components/shared/`, `web-app/src/app/`, `web-app/src/components/providers/`

**What to do**:
1. Migrate each remaining file. Global `globals.css` stays — it is not a CSS Module and provides the bridge for theming; do not delete it.
2. After this task, run `find web-app/src -name "*.module.css"` — should return nothing.
3. Remove `npm run lint:css-vars` script from `package.json` and the associated `scripts/check-css-vars.mjs` file.

**Completion criteria**: `find web-app/src -name "*.module.css"` returns nothing; `npm run build` and `npm run lint` pass.

---

## Phase 5 — React Native Foundation (Epic: Turborepo + packages/core)

**Goal**: Business logic (hooks, RTK slices, ConnectRPC transport factory) extracted into a `packages/core` package shareable with a future React Native app. No RN app shipped in this phase — architecture only.

**Phase gate**: `apps/web/` builds identically to current `web-app/`; `packages/core` contains RTK slices, transport factory, and type definitions; all hooks accept injected transport; CI passes.

**Dependencies**: Phase 3 complete (transport injection boundaries established). This phase is post-6-months — do not start until Phase 3 and 4 are complete.

**Note**: This phase involves significant file-system restructuring. Do it in a dedicated branch and merge as a single large PR with no other changes mixed in.

---

### Story 5.1 — Turborepo monorepo restructure

**As a developer**, I need the repo restructured as a Turborepo monorepo so that `packages/core` can be consumed by both `apps/web` and a future `apps/mobile`.

**Acceptance criteria**:
- `web-app/` is moved to `apps/web/`
- Root `package.json` is a Turborepo workspace root
- `turbo.json` defines `build`, `dev`, `test`, `lint` pipelines
- `apps/web` `npm run build` produces identical output to current `web-app` build
- CI pipeline updated to run `turbo run build` instead of `cd web-app && npm run build`

**Tasks**:

#### Task 5.1.1 — Initialize Turborepo workspace structure (3 hours)

**Files**: `package.json` (root), `turbo.json` (new root), `apps/web/` (moved from `web-app/`), `pnpm-workspace.yaml` or `package.json` workspaces field

**What to do**:
1. Install Turborepo: `npm install turbo --save-dev` at repo root.
2. Create root `turbo.json` with pipeline definitions for `build`, `dev`, `lint`, `test`.
3. Move `web-app/` → `apps/web/`. Update all CI scripts, Makefile targets (`make restart-web`), and README references.
4. Create `packages/core/package.json` with `name: "@stapler-squad/core"`, empty `src/index.ts`.
5. Add `packages/core` as a dependency in `apps/web/package.json`: `"@stapler-squad/core": "workspace:*"`.

**Completion criteria**: `turbo run build` from repo root completes successfully; `apps/web` output matches previous `web-app` output.

#### Task 5.1.2 — Extract RTK slices and ConnectRPC factory to packages/core (4 hours)

**Files**: `packages/core/src/store/`, `packages/core/src/api/`, `apps/web/src/lib/store/store.ts`

**What to do**:
1. Move Redux slices that contain no web-specific imports (session slice, approval slice, filter slice) from `apps/web/src/lib/store/` to `packages/core/src/store/`.
2. Move the ConnectRPC transport factory function to `packages/core/src/api/transport.ts`. The factory takes `baseUrl: string` as a parameter and returns a `Transport` — the specific transport implementation (`createConnectTransport`) is imported from `@connectrpc/connect-web` inside `apps/web`. In `packages/core`, the factory type is defined but the web-specific implementation is provided by the consumer via dependency injection.
3. Update `apps/web/src/lib/store/store.ts` to import slices from `@stapler-squad/core`.

**Completion criteria**: `turbo run build` clean; `apps/web` imports from `@stapler-squad/core` without TypeScript errors.

---

## Testing Strategy

### Unit Tests (Jest + React Testing Library)
- **Every primitive in `components/ui/`**: render, variant props, accessibility (axe-core), event handling
- **RTK Query endpoints**: mock ConnectRPC transport; assert correct cache tags; assert serialization boundary (plain object, no protobuf class instances in store)
- **Hooks**: `useFilterState`, `useViewport` — test with mocked window API

### Integration Tests (Jest)
- ReviewQueuePanel with mocked RTK Query hooks: approve action invalidates cache, reject action invalidates cache
- Theme provider: lightTheme/darkTheme classes applied correctly

### End-to-End Tests (Playwright)
- **Existing e2e suite must remain green after every phase**
- **New**: Mobile viewport session — run with `--project=pixel9fold` at 390×844px (outer screen) and 900×2092px (inner screen)
- **New**: Terminal WebGL fallback — confirm terminal loads on `--project=android-chrome` (Chrome with `WebGL2RenderingContext` deleted)
- **New**: Review queue touch target — verify button bounding boxes > 44×44px using `element.getBoundingClientRect()`
- **New**: Diff viewer — load DiffViewer with a sample diff; assert CodeMirror merge view is rendered

### Visual Regression
- Take baseline Playwright screenshots after Phase 1 migration; compare after Phase 4 migration
- Any deviation > 0.1% pixel difference fails the comparison

### Size Budget (size-limit)
- After Phase 4 (Monaco removal): verify "Total JS bundle" is at least 200 KB smaller than pre-Phase-4 baseline
- Run `npm run size-limit` in CI after every phase

---

## Bug Tracking

| ID | Description | Severity | Phase Addressed | Task |
|----|-------------|----------|-----------------|------|
| BUG-001 | Turbopack + vanilla-extract HMR failure | P1 | Phase 1 | Task 1.1.1 |
| BUG-002 | xterm.js WebGL crash on Android | P1 | Phase 2 | Task 2.1.1 |
| BUG-003 | Review queue buttons untappable on mobile | P2 | Phase 2 | Task 2.2.1 |
| BUG-004 | GBoard composition event corruption in xterm | P2 | Phase 2 | Task 2.3.1 |
| BUG-005 | ConnectRPC streaming RN boundary violation | P2 | Phase 3 | Task 3.1.1 |
| BUG-006 | Protobuf class instances in Redux state | P3 | Phase 3 | Task 3.1.1 |
| BUG-007 | Filter state lost on page refresh | P3 | Phase 3 | Task 3.1.4 |
| BUG-008 | Monaco bundle bloat (~500 KB) on mobile | P3 | Phase 4 | Task 4.2.1 |

---

## Open Questions

1. **BUG-004 manual test**: xterm.js GBoard composition event corruption must be reproduced on a physical Pixel 9 Pro Fold before Task 2.3.1 is implemented. If xterm 6.x has resolved #3600, skip the task.
2. **connect-query streaming stability**: If `@connectrpc/connect-query` Issue #524 (streaming redesign) ships before Phase 3 starts, re-evaluate whether to use connect-query instead of custom RTK Query `baseQuery`.
3. **Next.js 16 upgrade path**: When can `--turbopack` be safely re-added? Monitor `@vanilla-extract/next-plugin` changelog for `unstable_turbopack` support announcement.
4. **`@vanilla-extract/recipes` availability**: Confirm `@vanilla-extract/recipes` is installable alongside the current `@vanilla-extract/css@^1.20.1` before Task 1.3.2. Check peer dependency constraints.
