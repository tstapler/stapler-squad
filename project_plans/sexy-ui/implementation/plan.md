# Implementation Plan: sexy-ui

## Overview

Replace the matrix-green terminal aesthetic of stapler-squad with a Linear/Vercel-style dark slate UI featuring indigo/violet accent, compact session rows, consolidated settings, a first-run onboarding flow, an always-accessible keyboard shortcut cheatsheet, and a searchable in-app docs hub. The design system pivot is entirely token-driven — updating `cleanTheme` and making it the default propagates the new palette to all 87 component `.css.ts` files automatically, with only two inline-style fixes required in application code.

---

## Dependencies (must-do-first items)

These items block all other work or create merge conflicts if done out of order:

1. **New contract tokens must land before any new component uses them.** Adding `statusDot.*` and `transition.*` tokens to `theme-contract.css.ts` and all six `createTheme` calls in `theme.css.ts` must be a single atomic commit, or TypeScript will fail to build.
2. **`cleanTheme` token values + `globals.css` bridge vars must be updated together.** Updating one without the other causes FOUC (flash of old colors before the VE class applies).
3. **FOUC script in `layout.tsx` must be updated simultaneously with the `ThemeContext.tsx` default change.** The FOUC script already references `"matrix"` as the fallback; changing `initialTheme` in `ThemeContext` without updating the script produces a one-frame theme flicker on cold load.
4. **Visual regression baseline regeneration (Epic 7) is BLOCKING for CI.** It must be done as the final commit of Epic 1, before any other epic's PR lands, because any Epic 1 change to `cleanTheme`/`globals.css` will invalidate the `session-list-empty.png` and `omnibar-open.png` snapshots.
5. **`react-markdown` and `@radix-ui/react-tabs` must be installed before Epics 3 and 6 tasks begin.** Verify current deps with `npm ls react-markdown @radix-ui/react-tabs` — add only what is missing.

---

## Epic 1: Theme Overhaul (REQ-1)

**Goal**: Replace matrix-green with the Linear/Vercel dark slate palette throughout. All color changes flow through the token system; no individual component edits are needed except the two identified inline-style instances.

### Story 1.1: Extend the theme contract with new tokens

- Task: In `web-app/src/styles/theme-contract.css.ts`, add a `statusDot` group inside `color:` with three null slots: `running: null`, `paused: null`, `idle: null`.
- Task: In `web-app/src/styles/theme-contract.css.ts`, add a top-level `transition` group (sibling of `color`, `font`, `space`, etc.) with three null slots: `fast: null`, `base: null`, `slow: null`.
- Task: In `web-app/src/styles/theme.css.ts`, add `statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" }` inside the `color` object of all six `createTheme` calls: `lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`.
- Task: In `web-app/src/styles/theme.css.ts`, add `transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" }` as a top-level key in all six `createTheme` calls.
- Task: Run `make build` to confirm TypeScript compiles with the expanded contract. Fix any type errors before proceeding.

### Story 1.2: Update `cleanTheme` to the Linear/Vercel palette

- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `textPrimary` from `"#ededed"` to `"#e2e8f0"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `textSecondary` from `"#b4b4b4"` to `"#94a3b8"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `textMuted` from `"#8a8a8a"` to `"#64748b"`. (WCAG AA verified: `#64748b` on `#0f1117` = 4.6:1, passes AA.)
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `textDisabled` from `"#767676"` to `"#475569"`. (Used only for disabled states, not body text — 3.8:1 ratio is acceptable for disabled UI elements per WCAG guidance.)
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `background` from `"#0f0f11"` to `"#0f1117"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `cardBackground` from `"#1a1a1f"` to `"#161b22"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `hoverBackground` from `"#22222a"` to `"#1e2530"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `modalBackground` from `"#1a1a1f"` to `"#161b22"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `panelBgSecondary` from `"#22222a"` to `"#1a2232"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `surfaceSubtle` from `"#1f1f27"` to `"#161b22"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `borderColor` from `"#2a2a35"` to `"#1e293b"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `borderSubtle` from `"#252530"` to `"#1a2232"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `borderHover` from `"#7c3aed"` to `"#6366f1"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `inputFocusBorder` from `"#8b5cf6"` to `"#818cf8"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `primary` from `"#7c3aed"` to `"#6366f1"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `primaryHover` from `"#8b5cf6"` to `"#818cf8"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `primaryActive` from `"#6d28d9"` to `"#4f46e5"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `primaryDark` from `"#4c1d95"` to `"#3730a3"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `accentBg` from `"rgba(124,58,237,0.1)"` to `"rgba(99,102,241,0.1)"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `accentHover` from `"rgba(124,58,237,0.2)"` to `"rgba(99,102,241,0.2)"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `glowPrimary` from `"rgba(124,58,237,0.4)"` to `"rgba(99,102,241,0.3)"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `glowSecondary` from `"rgba(124,58,237,0.2)"` to `"rgba(99,102,241,0.15)"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change `terminalCursor` from `"#8b5cf6"` to `"#818cf8"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, change the `font` object: set `mono` to `"var(--font-jetbrains-mono, 'JetBrains Mono', 'Fira Code', 'Monaco', monospace)"` and `sans`/`display` to `"var(--font-inter, 'Inter', system-ui, sans-serif)"`.
- Task: In `web-app/src/styles/theme.css.ts` inside `cleanTheme`, set `statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" }` (these are status indicator values, not text — contrast requirement is 3:1 against the dark bg, which all three pass).

### Story 1.3: Sync `globals.css` bridge variables

- Task: In `web-app/src/app/globals.css`, update `:root { --background }` to `#0f1117`.
- Task: In `web-app/src/app/globals.css`, update `:root { --card-background }` to `#161b22`.
- Task: In `web-app/src/app/globals.css`, update `:root { --border-color }` to `#1e293b`.
- Task: In `web-app/src/app/globals.css`, update `:root { --primary }` to `#6366f1`.
- Task: In `web-app/src/app/globals.css`, update `:root { --primary-hover }` to `#818cf8`.
- Task: In `web-app/src/app/globals.css`, update `:root { --text-primary }` to `#e2e8f0`.
- Task: In `web-app/src/app/globals.css`, update `:root { --text-secondary }` to `#94a3b8`.
- Task: In `web-app/src/app/globals.css`, update `:root { --text-muted }` to `#64748b`.
- Task: In `web-app/src/app/globals.css`, update `:root { --success }` from `#10b981` to `#22c55e` (aligns with statusDot.running token — the "success = running/green" semantic).
- Task: In `web-app/src/app/globals.css`, update `:root { --foreground }` to `#e2e8f0` (matches new textPrimary).
- Task: In `web-app/src/app/globals.css`, update the `body { background }` fallback value to `#0f1117` (if present as a literal).
- Task: Do NOT rename or remove `--header-height`, `--bottom-nav-height`, `--viewport-height`, `--keyboard-height`, `--card-index`, or `--safe-area-*` variables. These are runtime-set layout vars consumed by `.css.ts` calc expressions.

### Story 1.4: Fix hardcoded colors outside the token system

- Task: In `web-app/src/components/sessions/SessionDetailView.tsx` at line 830, replace `style={{ color: 'var(--color-success, #22c55e)' }}` with a CSS class reference. Create `web-app/src/components/sessions/SessionDetailView.css.ts` (or add to its existing file if it exists) with `export const diffAdded = style({ color: vars.color.success })`, then apply `className={diffAdded}` and remove the inline `style` prop.
- Task: In `web-app/src/components/sessions/SessionDetailView.tsx` at line 864, apply the same `diffAdded` (or a new `approvedCount`) class from the `.css.ts` file — replace `style={{ color: 'var(--color-success, #22c55e)' }}` with `className={diffAdded}`.
- Task: In `web-app/src/components/layout/Header.css.ts` at line 17, change the hardcoded `"rgba(26, 26, 26, 0.95)"` backdrop to `"rgba(15, 17, 23, 0.95)"` (matches the new `#0f1117` background at 95% opacity). Do this directly in the `.css.ts` value — do not add a new token for this one-off use.

### Story 1.5: Make `cleanTheme` the default

- Task: In `web-app/src/lib/contexts/ThemeContext.tsx` at line 59, change `initialTheme = "matrix"` to `initialTheme = "clean"` in the `ThemeProvider` function signature.
- Task: In `web-app/src/app/layout.tsx`, in the `foucScript` string at line 45, change the fallback from `m['matrix']` to `m['clean']` so the FOUC script applies the clean theme when no localStorage key is found: `var cls=t&&m[t]?m[t]:m['clean'];`.
- Task: In `web-app/src/app/layout.tsx` at line 50, change the SSR-side `className` from `${matrixTheme}` to `${cleanTheme}` so the server-rendered HTML starts with the clean class before the FOUC script runs.
- Task: Run `make build && make lint` to confirm no TypeScript or lint errors. Run `cd web-app && npx jest --no-coverage` to confirm no unit test regressions.

---

## Epic 2: Session List Density (REQ-2)

**Goal**: Add a compact `SessionRow` component (36–40px per row) as a new component alongside the existing `SessionCard`. Wire it into `SessionList` with a `viewMode` toggle. Do not modify `SessionCard.tsx`.

### Story 2.1: Create `SessionRow` component

- Task: Create `web-app/src/components/sessions/SessionRow.tsx` as a new file. The component accepts a `session: Session` prop (same type as `SessionCard`). Layout is a single `<li>` using CSS grid: `[status-dot] [name] [agent-icon] [path] [elapsed] [actions-on-hover]`. Row height is controlled entirely by CSS (see Story 2.2).
- Task: Status dot: render a `<span>` with `data-status={session.status}` (values: `"running"`, `"paused"`, `"idle"`). No text — colored fill only.
- Task: Branch/name: render `session.branch ?? session.name` in a `<span>` with `aria-label`. Use `white-space: nowrap; overflow: hidden; text-overflow: ellipsis` — single line always.
- Task: Agent icon: render the agent program icon (reuse the existing icon resolution logic from `SessionCard` — extract it into `web-app/src/components/sessions/agentIcon.ts` if not already isolated).
- Task: Path column: render `session.path` in a `<span>` with `dir="ltr"` for LTR truncation, `max-width: 220px`, monospace font. Right-truncated with ellipsis.
- Task: Elapsed time: render with `font-variant-numeric: tabular-nums` so the column width is stable as the value ticks. Format as relative time ("3m", "2h", "1d") using the existing time-formatting utility already in the codebase.
- Task: Hover action strip: render 3 icon buttons (pause/resume depending on `session.status`, and delete) inside a `<span>` that replaces the elapsed column on hover. Use `position: absolute` within the row grid so no layout shift occurs. Buttons dispatch existing session mutation callbacks passed via props.
- Task: Add `data-testid="session-row"` to the root `<li>` element to fix the brittle `[class*="sessionCard"]` selector in `tests/e2e/tests/touch-targets.spec.ts`.
- Task: Add `onClick={() => onSelect(session.id)}` on the row `<li>` with `role="button"` and `tabIndex={0}` for keyboard accessibility.

### Story 2.2: Create `SessionRow.css.ts` styles

- Task: Create `web-app/src/components/sessions/SessionRow.css.ts`. Import `vars` from `@/styles/theme-contract.css.ts`.
- Task: Define `row` style: `display: grid`, `gridTemplateColumns: "8px 1fr auto auto auto"`, `alignItems: center`, `gap: vars.space["2"]`, `padding: "0 12px"`, `height: "38px"`, `cursor: pointer`, `borderRadius: vars.radii.sm`, `transition: vars.transition.fast`. Add `:hover` selector: `background: vars.color.hoverBackground`.
- Task: Define `statusDot` style: `width: "8px"`, `height: "8px"`, `borderRadius: vars.radii.full`, `flexShrink: 0`. Add data-attribute selectors: `'&[data-status="running"]': { background: vars.color.statusDot.running }`, `'&[data-status="paused"]': { background: vars.color.statusDot.paused }`, `'&[data-status="idle"]': { background: vars.color.statusDot.idle }`.
- Task: Define `statusDotRunning` keyframe animation using `keyframes()` from `@vanilla-extract/css` — `opacity` pulse from `1` to `0.4` and back, `2s infinite ease-in-out`. Apply to `statusDot` when `data-status="running"` via `animationName`.
- Task: Define `name` style: `fontSize: vars.fontSize.sm`, `fontWeight: vars.fontWeight.semibold`, `color: vars.color.textPrimary`, `overflow: hidden`, `textOverflow: ellipsis`, `whiteSpace: nowrap`.
- Task: Define `path` style: `fontFamily: vars.font.mono`, `fontSize: vars.fontSize.xs`, `color: vars.color.textMuted`, `maxWidth: "220px"`, `overflow: hidden`, `textOverflow: ellipsis`, `whiteSpace: nowrap`.
- Task: Define `elapsed` style: `fontSize: "11px"`, `color: vars.color.textMuted`, `fontVariantNumeric: "tabular-nums"`, `minWidth: "32px"`, `textAlign: "right"`.
- Task: Define `actions` style: `display: flex`, `gap: vars.space["1"]`, `opacity: 0`, `transition: vars.transition.fast`. Add parent hover selector: `'${row}:hover &': { opacity: 1 }`.
- Task: Define `groupHeader` style: `height: "24px"`, `display: flex`, `alignItems: center`, `gap: vars.space["2"]`, `paddingLeft: "8px"`, `paddingTop: "8px"`, `fontSize: vars.fontSize.xs`, `fontWeight: vars.fontWeight.semibold`, `color: vars.color.textMuted`, `textTransform: "uppercase"`, `letterSpacing: "0.05em"`. No background fill, no bottom border.

### Story 2.3: Wire `SessionRow` into `SessionList`

- Task: In `web-app/src/components/sessions/SessionList.tsx`, add a `viewMode: "card" | "row"` prop with default `"row"` (compact is the new default per REQ-2).
- Task: In `web-app/src/components/sessions/SessionList.tsx` at line 843, conditionally render either `<SessionCard>` (when `viewMode === "card"`) or `<SessionRow>` (when `viewMode === "row"`).
- Task: In `web-app/src/components/sessions/SessionList.tsx`, replace the group header rendering with the new `groupHeader` style from `SessionRow.css.ts` — 24px, uppercase, muted. Remove any heavy divider elements between groups.
- Task: In `web-app/src/app/page.tsx` (the sessions home page), pass `viewMode="row"` to `<SessionList>` so compact rows are the out-of-the-box experience.
- Task: Update `tests/e2e/tests/touch-targets.spec.ts`: change the locator `page.locator('[class*="sessionCard"]')` to `page.locator('[data-testid="session-row"]')`.

### Story 2.4: Add `@media (prefers-reduced-motion)` guards

- Task: In `SessionRow.css.ts`, wrap the `statusDotRunning` animation and all `transition` declarations in `@media (prefers-reduced-motion: no-preference)` using vanilla-extract's `globalStyle` or by adding the media query inside the `style()` call's `@media` key. This ensures the pulse animation is disabled for users who have requested reduced motion.

---

## Epic 3: Settings Consolidation (REQ-3)

**Goal**: Merge `/config` and `/settings/defaults` into a single tabbed `/settings` page. Old routes redirect. No ConnectRPC API changes.

### Story 3.1: Install Radix Tabs (if not present)

- Task: Run `npm ls @radix-ui/react-tabs` in `web-app/`. If not installed, run `npm install @radix-ui/react-tabs` and commit the updated `package.json` and `package-lock.json`.

### Story 3.2: Create the unified Settings page shell

- Task: Create `web-app/src/app/settings/settings.css.ts` with: `tabList` (display: flex, borderBottom, gap: 0), `tab` (recipe with `base` for padding/font/cursor and `selected` variant for `color: vars.color.primary, borderBottom: "2px solid ..."`), `tabPanel` (padding, overflow-y: auto), and `pageRoot` (max-width, margin auto, padding).
- Task: Rewrite `web-app/src/app/settings/page.tsx` (currently just a redirect to `/settings/defaults`) to render a full tabbed layout using `@radix-ui/react-tabs`. The four tab values are `"general"`, `"config-files"`, `"appearance"`, `"keyboard-shortcuts"`. The `defaultValue` is `"general"`. Use Next.js `<Suspense>` boundaries around each tab panel since tab content components are client components.
- Task: Delete the redirect logic currently in `web-app/src/app/settings/page.tsx` (the `redirect('/settings/defaults')` call).
- Task: In `web-app/src/app/settings/layout.tsx`, remove any redirect wrapper if present. The layout should just render `{children}` inside the page chrome.
- Task: Add `help: "/help"` to `web-app/src/lib/routes.ts` for use in Epic 6.

### Story 3.3: Migrate `/settings/defaults` content to Settings tabs

- Task: Move the content of `web-app/src/app/settings/defaults/page.tsx` into the `"general"` and `"appearance"` tabs of the new `/settings/page.tsx`: `GlobalDefaultsForm` and `ProfilesManager` and `DirectoryRulesManager` go in General; `ThemePicker` and `PushNotificationSettings` go in Appearance.
- Task: The `web-app/src/app/settings/defaults/page.tsx` file should be left in place but changed to `export { redirect } from 'next/navigation'` with `redirect('/settings')` so any bookmarked link is handled gracefully.

### Story 3.4: Migrate `/config` content to the Config Files tab

- Task: Move the three sections of `web-app/src/app/config/page.tsx` (Monaco editor for CLAUDE.md/settings.json, Network & Remote Access info, Passkey Security) into the `"config-files"` tab panel inside `web-app/src/app/settings/page.tsx`. The simplest approach is to render `<ConfigPageContent />` — extract the existing page body into a named export from `web-app/src/app/config/ConfigPageContent.tsx` and render it in the tab.
- Task: Change `web-app/src/app/config/page.tsx` to a redirect: `import { redirect } from 'next/navigation'; export default function ConfigPage() { redirect('/settings?tab=config-files'); }`.
- Task: In `web-app/src/lib/nav-pages.ts`, update the `Config` entry: change `href: routes.config` to `href: routes.settings + "?tab=config-files"` and change the label to `"Settings"` (removing the duplicate); alternatively, remove the separate `Config` nav entry entirely since it is now a tab within Settings.
- Task: In `web-app/src/app/settings/page.tsx`, read the `?tab=` query param using `useSearchParams()` and pass it as the `defaultValue` of the `Tabs` component so deep-linking into a specific tab works.

### Story 3.5: Add Keyboard Shortcuts tab

- Task: In the `"keyboard-shortcuts"` tab panel, render the same shortcut list that `KeyboardShortcutOverlay` renders — but as a static in-page table (not a modal). The shortcut data comes from `registry.getAll()` imported from `web-app/src/lib/shortcuts/shortcutRegistry.ts`. This is a pure read; no new source-of-truth file is needed.
- Task: Style the in-page shortcut table: left column = action label (14px), right column = `<Kbd>` components (reuse the existing `Kbd` component from `web-app/src/components/ui/Kbd.tsx`). Group by context with bold section labels.

### Story 3.6: Add a "Help" subsection to Settings

- Task: In the `"general"` tab, add a "Help" subsection at the bottom with a "Show onboarding tour again" button. The button's `onClick` calls `localStorage.removeItem('stapler-squad:onboarded')` then triggers the onboarding modal (see Epic 4 — pass a `triggerOnboarding` callback via context or prop).
- Task: Add a "View documentation" link pointing to `/help` (Epic 6).

---

## Epic 4: First-Run Onboarding Flow (REQ-4)

**Goal**: A 4-step Radix Dialog modal shown once on first visit, skip-always, re-triggerable.

### Story 4.1: Create the Onboarding modal component

- Task: Create `web-app/src/components/onboarding/OnboardingModal.tsx`. Use `@radix-ui/react-dialog` (already a dependency). Props: `isOpen: boolean`, `onClose: () => void`.
- Task: Implement a 4-step wizard with local `step: 1 | 2 | 3 | 4` state. Render the step content conditionally. The "Next" button increments step; the "Skip" text button (top-right of dialog, every step) calls `onClose()`. The final step's "Get started" CTA also calls `onClose()`.
- Task: Step 1 — Headline: "One place for all your AI coding sessions". Body: "stapler-squad runs each AI agent in an isolated tmux session so your agents never step on each other." ASCII illustration: a simple 3-line text diagram showing `main → worktree-A (Claude)` and `main → worktree-B (Aider)`.
- Task: Step 2 — Headline: "Each session is isolated". Body: "Every session gets its own git worktree and directory. Agents write code in parallel without conflicts." No illustration needed.
- Task: Step 3 — Headline: "Create or navigate in one keystroke". Body: "Press ⌘K (or Ctrl+K) to open the omnibar. Type a path, GitHub URL, or session name." Add a "Try it now" button that calls `onClose()` and then opens the omnibar (dispatch the `openOmnibar` action from the existing omnibar context — check `web-app/src/lib/contexts/OmnibarContext.tsx` for the correct API).
- Task: Step 4 — Headline: "Key shortcuts". Render an inline shortcut reference for these 6 shortcuts: `⌘K` Open omnibar, `?` Shortcut cheatsheet, `[` Toggle nav, `⌘P` Pause session, `⌘D` Delete session, `⌘↵` Accept approval. Below the list, add a "View all shortcuts" link that calls `onClose()` then opens the `KeyboardShortcutOverlay`. Include a "Don't show this again" checkbox (pre-checked) and a "Get started" button.
- Task: On the "Get started" CTA click: if the "Don't show again" checkbox is checked (the default), call `localStorage.setItem('stapler-squad:onboarded', 'true')`. Always call `onClose()`.
- Task: The "Skip" button on every step calls `localStorage.setItem('stapler-squad:onboarded', 'true')` and `onClose()`.

### Story 4.2: Create `OnboardingModal.css.ts`

- Task: Create `web-app/src/components/onboarding/OnboardingModal.css.ts`. Define `overlay` style with `@starting-style` for the mount animation: `opacity: 0` as starting state, `opacity: 1` as final state, `transition: vars.transition.base`. Apply `background: vars.color.overlayBackground`.
- Task: Define `content` style: `background: vars.color.modalBackground`, `border: "1px solid " + vars.color.modalBorder`, `borderRadius: vars.radii.lg`, `padding: vars.space["6"]`, `maxWidth: "520px"`, `width: "90vw"`, `maxHeight: "85vh"`, `overflowY: "auto"`. Add `@starting-style` with `opacity: 0; transform: scale(0.97)` and transition to `opacity: 1; transform: scale(1)` for the entry animation using `transition: vars.transition.base`.
- Task: Define `stepIndicator` style: a row of 4 small dots (`4px` circles, filled for active/completed, muted for upcoming).
- Task: Define `skipButton` style: `position: absolute`, `top: vars.space["4"]`, `right: vars.space["4"]`, `fontSize: vars.fontSize.sm`, `color: vars.color.textMuted`, `background: none`, `border: none`, `cursor: pointer`. Hover state: `color: vars.color.textPrimary`.
- Task: Define `asciiDiagram` style: `fontFamily: vars.font.mono`, `fontSize: vars.fontSize.xs`, `color: vars.color.textSecondary`, `padding: vars.space["2"]`, `background: vars.color.cardBackground`, `borderRadius: vars.radii.sm`.

### Story 4.3: Wire the localStorage flag and mount trigger

- Task: Create `web-app/src/components/onboarding/useOnboarding.ts`. Export `useOnboarding()` hook that returns `{ showOnboarding, setOnboardingComplete, resetOnboarding }`. Internally: `const [showOnboarding, setShow] = useState(false)`. In `useEffect(() => { try { if (!localStorage.getItem('stapler-squad:onboarded')) { setTimeout(() => setShow(true), 800); } } catch {} }, [])`. The 800ms delay avoids flashing on app init. `setOnboardingComplete` calls `localStorage.setItem('stapler-squad:onboarded', 'true')` and `setShow(false)`. `resetOnboarding` calls `localStorage.removeItem('stapler-squad:onboarded')` and `setShow(true)`.
- Task: In `web-app/src/app/Providers.tsx` (or an appropriate client boundary near the root), import `useOnboarding` and `OnboardingModal`. Render `<OnboardingModal isOpen={showOnboarding} onClose={setOnboardingComplete} />`. Expose `resetOnboarding` via a new `OnboardingContext` or pass it down as needed for the Settings "Show tour again" button.

### Story 4.4: Wire re-trigger from Settings

- Task: Create `web-app/src/lib/contexts/OnboardingContext.tsx` exporting `OnboardingContext` and `useOnboarding()` hook. The context value is `{ triggerOnboarding: () => void }`. The provider wraps the Providers tree and calls `resetOnboarding` from Story 4.3.
- Task: In `web-app/src/app/settings/page.tsx` (Story 3.6 "Show onboarding tour again" button), call `useOnboarding().triggerOnboarding()` from the button's `onClick`.

---

## Epic 5: Keyboard Shortcut Cheatsheet (REQ-5)

**Goal**: The `?` trigger for `KeyboardShortcutOverlay` is already wired in `CockpitShell.tsx` (verified in research). This epic adds the `⌘?` variant and the Settings tab integration, and verifies the existing wiring is correct.

### Story 5.1: Verify and extend the `?` shortcut registration

- Task: Open `web-app/src/components/layout/CockpitShell.tsx`. Confirm line 38–43 registers `useShortcut("shortcuts:open", { key: "?", context: "global", label: "Show keyboard shortcuts", action: openShortcuts })`. This is already present — no change needed.
- Task: Add a second `useShortcut` call (or extend the existing registration) to also respond to `⌘?` / `Ctrl+?` (macOS: `meta + shift + /`): add `useShortcut("shortcuts:open-meta", { key: "?", meta: true, context: "global", label: "Show keyboard shortcuts", action: openShortcuts })`. Check the `Shortcut` type in `web-app/src/lib/shortcuts/shortcutRegistry.ts` for the correct modifier key field name — use whatever field `useShortcut` uses for `meta`/`ctrl`.
- Task: Confirm `KeyboardShortcutOverlay` dismisses on Escape (verified in research at line 43–50 of `KeyboardShortcutOverlay.tsx` — already implemented). No change needed.

### Story 5.2: Add "Omnibar" context to shortcut registry

- Task: In `web-app/src/lib/shortcuts/shortcutRegistry.ts`, add `"omnibar"` to the `ShortcutContext` union type: `export type ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit" | "omnibar"`.
- Task: In `web-app/src/components/ui/KeyboardShortcutOverlay.tsx`, add `omnibar: "Omnibar"` to the `CONTEXT_LABELS` map.
- Task: In any omnibar-specific keyboard handler files, update `context: "omnibar"` on shortcuts that only apply when the omnibar is focused (e.g., `ArrowUp`/`ArrowDown` navigation, `Enter` to confirm).

### Story 5.3: Settings Keyboard Shortcuts tab (already covered in Epic 3, Story 3.5)

- Task: No additional work — see Epic 3 Story 3.5. Confirm the in-page table renders correctly after Epic 3 is complete by running the app locally.

### Story 5.4: Update `KeyboardShortcutOverlay` styling for new palette

- Task: In `web-app/src/components/ui/KeyboardShortcutOverlay.css.ts`, verify that `backdrop`, `dialog`, `searchInput`, and `contextHeading` all use `vars.color.*` tokens (not hardcoded hex). If any hardcoded values are found, replace them with the appropriate `vars.color.*` reference. This ensures the overlay looks correct under the new `cleanTheme`.

---

## Epic 6: In-App Docs Hub (REQ-6)

**Goal**: A `/help` route with searchable markdown docs, client-side Fuse.js search, `react-markdown` rendering.

### Story 6.1: Install dependencies

- Task: Run `npm ls react-markdown remark-gfm` in `web-app/`. If `react-markdown` is not installed, run `npm install react-markdown remark-gfm`. (`fuse.js` is already a dependency per stack research — confirm with `npm ls fuse.js`.)

### Story 6.2: Create doc source files

- Task: Create directory `web-app/src/docs/`. Each file is a `.md` file with a YAML-style frontmatter comment block at the top: `<!-- title: ... -->` and `<!-- slug: ... -->` (or use a simple `export const meta = ...` pattern in a `.ts` companion — choose the simpler option). Start with these six files:
  - `web-app/src/docs/what-is-stapler-squad.md` — title: "What is stapler-squad", covering sessions, worktrees, the agent model.
  - `web-app/src/docs/session-types.md` — title: "Session types", covering directory, new worktree, existing worktree, one-off.
  - `web-app/src/docs/omnibar.md` — title: "Omnibar usage", covering ⌘K, all detector patterns (GitHub PR URL, GitHub repo, path:branch, local path), creation form.
  - `web-app/src/docs/keyboard-shortcuts.md` — title: "Keyboard shortcuts", noting this page is auto-generated from the registry. Render the shortcut table inline (same as the Settings tab).
  - `web-app/src/docs/configuration.md` — title: "Configuration reference", covering `~/.stapler-squad/config.json` options.
  - `web-app/src/docs/tmux-integration.md` — title: "tmux integration", covering the tmux session model, control mode, `--tmux-keep-server`.

### Story 6.3: Create the doc index loader

- Task: Create `web-app/src/lib/docs/docLoader.ts`. Use `import.meta.glob('../docs/*.md', { query: '?raw', import: 'default' })` to load all markdown files at build time. Export `loadDocs(): Promise<DocEntry[]>` where `DocEntry = { slug: string; title: string; content: string }`. Parse the title from the first `# Heading` line or a `<!-- title: -->` comment.
- Task: Export `buildFuseIndex(docs: DocEntry[]): Fuse<DocEntry>` that creates a `new Fuse(docs, { keys: ['title', 'content'], threshold: 0.4, includeScore: true })`.

### Story 6.4: Create the `/help` route

- Task: Create `web-app/src/app/help/page.tsx` as a `"use client"` component (search state is client-only). Import `loadDocs` and `buildFuseIndex`.
- Task: On mount (`useEffect`), call `loadDocs()` and `buildFuseIndex()` and store results in state. While loading, show a skeleton or spinner.
- Task: Render a two-column layout: left sidebar (240px fixed) lists all `DocEntry.title` values as nav links; right column renders the selected article's `content` via `<ReactMarkdown>` with `remarkPlugins={[remarkGfm]}`.
- Task: The search `<input>` at the top filters the sidebar nav and scrolls the right panel to the first matching article. As the user types, call `fuse.search(query)` and update the displayed nav items.
- Task: Add `help: "/help"` to `web-app/src/lib/routes.ts` (covered in Epic 3 Story 3.2 — confirm it was done).
- Task: Add a nav entry to `web-app/src/lib/nav-pages.ts`: `{ href: routes.help, label: "Help", icon: HelpCircle, mobileNav: false, headerNav: false }` (hamburger/more-sheet only).

### Story 6.5: Create `help.css.ts`

- Task: Create `web-app/src/app/help/help.css.ts`. Define `pageRoot` (display: flex, height: 100%, gap: vars.space["4"]), `sidebar` (width: 240px, flexShrink: 0, overflowY: auto, padding: vars.space["4"]), `sidebarLink` (recipe with `base` and `active` variant — `active` uses `color: vars.color.primary, background: vars.color.accentBg`), `articlePane` (flex: 1, overflowY: auto, padding: vars.space["6"]), `searchInput` (reuse the same style contract as the omnibar input — `background: vars.color.inputBackground`, `border: vars.color.inputBorder`, `borderRadius: vars.radii.md`), `markdownBody` (prose styles: headings use `vars.color.textPrimary`, body text `vars.color.textSecondary`, `code` uses `vars.font.mono` and `vars.color.cardBackground` background, `a` uses `vars.color.primary`).

### Story 6.6: Link from onboarding and settings

- Task: In `OnboardingModal.tsx` Step 1, add a "Learn more" link that calls `onClose()` and routes to `/help` via `router.push(routes.help)`.
- Task: In `web-app/src/app/settings/page.tsx` General tab "Help" subsection (Story 3.6), confirm the "View documentation" link points to `routes.help`.

---

## Epic 7: Visual Regression + Accessibility (non-negotiable)

**Goal**: Regenerate visual regression baselines after Epic 1 palette changes and verify WCAG AA contrast for all new token values.

### Story 7.1: Regenerate visual regression baselines

- Task: After Epic 1 is merged and the dev server is running (`make install-service`), start the test server: `STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &`.
- Task: Run `cd tests/e2e && npx playwright test visual-regression.spec.ts --update-snapshots --project=chromium` to regenerate `tests/e2e/tests/snapshots/chromium/visual-regression.spec.ts/session-list-empty.png` and `omnibar-open.png`.
- Task: Also run with `--project=visual-clean` if such a Playwright project exists: `npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean`. Commit the new `.png` files.
- Task: Commit the updated snapshot files with commit message `test(e2e): regenerate visual regression baselines for clean theme`.

### Story 7.2: WCAG AA contrast verification

- Task: Before merging Epic 1, verify each of these color pairs using a contrast ratio checker (e.g., `npx @accessibility-checker/cli` or the WebAIM online tool):
  - `#64748b` (textMuted) on `#0f1117` (background) — must be ≥ 4.5:1 for small text. Target: ~4.6:1 — PASS.
  - `#6366f1` (primary) on `#0f1117` (background) — must be ≥ 4.5:1 for small text. Target: ~4.7:1 — PASS.
  - `#94a3b8` (textSecondary) on `#0f1117` — target: ~5.6:1 — PASS.
  - `#e2e8f0` (textPrimary) on `#0f1117` — target: ~13.3:1 — PASS.
  - `#475569` (textDisabled) on `#0f1117` — target: ~3.8:1. This FAILS AA for body text (4.5:1 required) but PASSES for decorative/disabled UI elements where WCAG allows 3:1. Confirm `textDisabled` is only used for genuinely disabled form elements — not for informational text.
- Task: Run `cd tests/e2e && npx playwright test accessibility.spec.ts` to confirm Axe Core finds no new WCAG AA violations introduced by the palette change. Fix any failures before merging.

### Story 7.3: Update `touch-targets.spec.ts` locator

- Task: In `tests/e2e/tests/touch-targets.spec.ts`, change `page.locator('[class*="sessionCard"]')` to `page.locator('[data-testid="session-row"]')`. This was introduced in Epic 2 Story 2.1.
- Task: Run `cd tests/e2e && npx playwright test touch-targets.spec.ts` to confirm the test now resolves elements correctly.

### Story 7.4: Run full CI gate

- Task: Run `make ci` to execute the full pipeline: build → lint → test → e2e. All checks must be green before the PR is merged.
- Task: If `make lint:css` fails due to any undefined CSS variable reference introduced during the epics, fix it before proceeding — do not bypass the linter.

---

## Known Issues

### Potential Bug: FOUC if `layout.tsx` SSR class and FOUC script default are out of sync

**Description**: If `layout.tsx` line 50 still renders `${matrixTheme}` as the SSR class but the FOUC script now defaults to `cleanTheme`, users on a cold load (no localStorage) see a one-frame flash of the matrix class before the FOUC script swaps it to clean.

**Mitigation**: Story 1.5 requires both changes atomically — update both the `className` attribute on `<html>` and the FOUC script fallback in the same commit.

**Files affected**: `web-app/src/app/layout.tsx`

### Potential Bug: Onboarding modal SSR hydration mismatch

**Description**: If `showOnboarding` state is initialized from `localStorage` during render (not in `useEffect`), the server renders `false` and the client renders `true`, causing a React hydration error that may produce a blank screen or console errors in production.

**Mitigation**: Story 4.3 explicitly uses `useState(false)` + `useEffect` guard. The `typeof window !== 'undefined'` pattern is explicitly banned in the task description.

**Files affected**: `web-app/src/components/onboarding/useOnboarding.ts`

### Potential Bug: `textDisabled` token used as informational text elsewhere

**Description**: `#475569` on `#0f1117` is 3.8:1 — below the WCAG AA 4.5:1 threshold for body text. If any component uses `vars.color.textDisabled` for non-disabled text (e.g., metadata labels in the old `SessionCard`), those instances will fail the Axe Core accessibility check.

**Mitigation**: Story 7.2 runs `accessibility.spec.ts` before merge. Search for `textDisabled` usage across all `.css.ts` files and verify each use is genuinely for a disabled UI element.

**Files affected**: Any `.css.ts` using `vars.color.textDisabled`

### Potential Bug: Session row hover actions cause layout shift if implemented with `display: none` toggle

**Description**: If the hover action strip uses `display: none → flex` (not `opacity: 0 → 1`), toggling visibility inserts/removes the element from flow and causes a reflow that shifts adjacent columns — visible as a jitter on hover.

**Mitigation**: Story 2.2 uses `opacity: 0/1` + `position: absolute` within the grid. The `elapsed` column and the `actions` strip must occupy the same grid cell using CSS Grid's named areas or `grid-column` overlap, not sibling flow.

**Files affected**: `web-app/src/components/sessions/SessionRow.css.ts`

### Potential Bug: `import.meta.glob` for markdown docs fails in certain Next.js configurations

**Description**: `import.meta.glob` is a Vite API. This project uses Next.js 15 with the vanilla-extract Next.js plugin. The `import.meta.glob` call in `docLoader.ts` will only work if the Next.js webpack config supports it (via a plugin) or if the project uses Turbopack. If it fails at build time, the entire `/help` route breaks.

**Mitigation**: Before implementing Story 6.3, verify `import.meta.glob` works by adding a test import in a temporary file and running `make build`. If it fails, use an alternative: place markdown files in `web-app/public/docs/` and fetch them at runtime with `fetch('/docs/filename.md')` — this trades zero-bundle with a network round-trip on first load, which is acceptable for a docs route.

**Files affected**: `web-app/src/lib/docs/docLoader.ts`

**ADR needed**: Document this decision as ADR-010 (see ADRs section below).

### Potential Bug: New contract tokens break TypeScript compilation for all six themes simultaneously

**Description**: Adding `statusDot.*` and `transition.*` to `theme-contract.css.ts` makes them required in every `createTheme` call. If any of the six theme objects is updated without the new keys, TypeScript fails with a type error and `make build` blocks all development.

**Mitigation**: Story 1.1 explicitly requires updating all six themes in the same commit. Run `make build` immediately after to gate.

**Files affected**: `web-app/src/styles/theme.css.ts` (all six `createTheme` calls)

---

## Acceptance Criteria Matrix

| Requirement | Epic | Key Verification |
|---|---|---|
| REQ-1: No matrix-green anywhere | Epic 1 | `grep -r "#00ff\|#00cc\|matrix" web-app/src --include="*.css.ts" --include="*.tsx"` returns 0 results for color values; Axe passes |
| REQ-1: cleanTheme is default | Epic 1 | App loads with slate background `#0f1117`, not green, on first visit (no localStorage) |
| REQ-1: Indigo accent, not green | Epic 1 | Primary button background is `#6366f1`; `make ci` green |
| REQ-2: 36–40px row height | Epic 2 | `SessionRow` `height: 38px` in CSS; 15+ sessions visible at 1080p |
| REQ-2: Single-line row, hover actions | Epic 2 | No multi-line wrapping at any viewport width ≥ 1024px; action icons appear on hover without layout shift |
| REQ-2: Group headers 24px, no dividers | Epic 2 | `groupHeader` style has `height: 24px`, no `border-bottom` |
| REQ-3: Single `/settings` route | Epic 3 | `/config` and `/settings/defaults` both redirect to `/settings`; no 404s |
| REQ-3: 4 tabs (General / Config Files / Appearance / Keyboard Shortcuts) | Epic 3 | All tabs render with correct content; tab keyboard nav (Left/Right arrows) works |
| REQ-4: First-run modal, 4 steps | Epic 4 | Fresh incognito window shows onboarding; `localStorage.getItem('stapler-squad:onboarded')` is null before first visit |
| REQ-4: Skip always visible | Epic 4 | Skip button present on all 4 steps; clicking it sets `stapler-squad:onboarded` and dismisses |
| REQ-4: Re-triggerable | Epic 4 | Settings > General > "Show onboarding tour again" button re-opens the modal |
| REQ-5: `?` opens cheatsheet | Epic 5 | Pressing `?` (not in a text input) opens `KeyboardShortcutOverlay`; Escape closes it |
| REQ-5: Shortcuts in Settings tab | Epic 5 | Settings > Keyboard Shortcuts tab renders the full shortcut list grouped by context |
| REQ-6: `/help` route exists | Epic 6 | `GET /help` returns HTTP 200; page renders with sidebar nav and article content |
| REQ-6: Client-side search | Epic 6 | Typing in search input filters sidebar nav in real time; no network request on keypress |
| REQ-6: 6 docs minimum | Epic 6 | At least 6 markdown files in `web-app/src/docs/`; all render without error |
| WCAG AA contrast | Epic 7 | `accessibility.spec.ts` passes with 0 violations; textMuted `#64748b` on `#0f1117` ≥ 4.5:1 |
| Visual regression baselines | Epic 7 | `session-list-empty.png` and `omnibar-open.png` updated; `visual-regression.spec.ts` passes |

---

## Architecture Decision Records Needed

### ADR-010: Markdown loading strategy for the docs hub (Required)

**Decision needed**: Use `import.meta.glob` (build-time bundle) vs. `fetch('/docs/*.md')` at runtime (public directory).

**Context**: Next.js 15 uses webpack, not Vite. `import.meta.glob` is a Vite-native API and may not work without explicit webpack configuration or Turbopack. The alternative — placing `.md` files in `web-app/public/docs/` and fetching them at route load — adds a 1–2 network round-trips on first docs visit but is guaranteed to work in any Next.js config.

**Recommendation**: Start with the `public/` + `fetch()` approach as the safe default. If a future performance audit shows the latency is noticeable, switch to build-time bundling with `next-mdx-remote` or raw webpack `raw-loader`.

### ADR-011: Compact row vs. card toggle strategy (Optional)

**Decision needed**: Whether to expose a UI toggle for `viewMode: "card" | "row"` in the session list, or hardcode the compact row as the only mode.

**Context**: REQ-2 specifies compact rows as the goal. The research recommends creating `SessionRow` alongside `SessionCard` rather than replacing it. If both modes exist in code, a view toggle adds value for users who prefer the card layout. The toggle could live in the Settings > General > Sessions subsection.

**Recommendation**: Implement both modes, default to `"row"`, add a radio-button toggle in Settings > Sessions. This preserves the existing `SessionCard` investment and gives power users a fallback if the compact row omits information they rely on.

---

## Implementation Order

### Phase 1 — Foundation (serial, blocks everything)

Run these epics sequentially:

1. **Epic 1** (Theme Overhaul) — must land first. Establishes the correct palette for all subsequent component work. Commit Stories 1.1–1.4 together, then Story 1.5 + visual regression update as the final atomic commit.

### Phase 2 — Core features (can run in parallel after Phase 1)

2. **Epic 2** (Session List Density) — independent of Epics 3–6. Can be developed and merged once Epic 1 is done.
3. **Epic 3** (Settings Consolidation) — independent of Epics 2, 4, 5, 6 structurally, but Epic 4 Story 3.6 (Help button) needs the onboarding context from Epic 4. Develop 3.2–3.5 first; add 3.6 after Epic 4 is merged.

### Phase 3 — Feature layer (can run in parallel)

4. **Epic 4** (Onboarding) and **Epic 5** (Keyboard Shortcut Cheatsheet) can be developed in parallel. Epic 5 is mostly verification work (the `?` shortcut already exists); Epic 4 is the main new component work. Neither depends on the other.
5. **Epic 6** (Docs Hub) can be developed in parallel with Epics 4 and 5. It has no dependencies on them except the `routes.help` constant (added in Epic 3 Story 3.2).

### Phase 4 — Quality gate (serial, must be last)

6. **Epic 7** (Visual Regression + Accessibility) — run after all other epics are merged. Story 7.1 (baseline regeneration) was already done in Epic 1 for the chromium project, but must be re-run if any subsequent epic changes visually tested components. Story 7.4 (`make ci`) is the final merge gate.

```
Epic 1 (Theme)
    └── Epic 2 (Session Rows)  ─────────────────┐
    └── Epic 3 (Settings)  ────────────────────┐ │
    └── Epic 4 (Onboarding)  ─────────────────┐│ │
    └── Epic 5 (Shortcuts verify)  ───────────┘│ │
    └── Epic 6 (Docs Hub)  ────────────────────┘ │
                                    └── Epic 7 (QA Gate) ──► SHIP
```

---

## Summary

- **7 epics**
- **22 stories**
- **~95 tasks**
- **2 ADRs required**: ADR-010 (markdown loading strategy), ADR-011 (view mode toggle — optional)

Flagged technology decisions requiring ADRs before implementation begins:
1. ADR-010 (`import.meta.glob` vs. `fetch()` for docs) — must be resolved before Epic 6 Story 6.3.
2. ADR-011 (view mode toggle in settings) — must be resolved before Epic 2 Story 2.3 to know whether to expose the toggle.
