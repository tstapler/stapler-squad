# Features Audit: Cyberpunk UX Revamp

## 1. Current Theme Implementation

### Theme Contract (`web-app/src/styles/theme-contract.css.ts`)
- Uses `createThemeContract` with `vars` exported object
- Token groups: `color` (37 tokens), `statusBadge` (18 tokens), `font` (2), `space` (9), `radii` (4), `fontSize` (5), `fontWeight` (4), `shadow` (4)
- No glow, scanline, or cyberpunk-specific tokens defined yet
- `breakpoints` and `zIndex` exported as plain constants (not theme tokens — correct, CSS vars can't be used in media queries)

### Theme Implementations (`web-app/src/styles/theme.css.ts`)
- Two themes: `lightTheme` and `darkTheme` via `createTheme(vars, {...})`
- `sharedTokens` object used for non-color tokens (font, space, radii, fontSize, fontWeight) — identical across both themes
- `terminalTokens` always dark (same in both themes) — intentional design
- Dark theme uses blue-tinted primary (`#2d9cdb`), light uses blue (`#0070f3`)
- Font mono = `'Monaco', 'Menlo', 'Ubuntu Mono', monospace` — JetBrains Mono NOT yet integrated

**What needs to change:**
- Add 4 new theme objects: `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`
- Add glow/scanline/cursor tokens to the contract
- Update `font.mono` to include JetBrains Mono
- Rename `lightTheme`/`darkTheme` or fold them into "clean" variants

### `globals.css`
- Dual CSS custom property system exists alongside vanilla-extract (legacy `--variable` naming)
- Dark mode implemented via `@media (prefers-color-scheme: dark)` on `:root`
- Many duplicate tokens between `globals.css` and `theme.css.ts` — both systems coexist
- Status badge tokens defined in both places
- `--font-mono` uses same Monaco/Menlo stack as vanilla-extract

**What needs to change:**
- `globals.css` is legacy/compatibility layer; new code should not add to it
- Once theme switching is user-controlled (not media query), the `@media (prefers-color-scheme: dark)` block in `globals.css` becomes a conflict — needs to be scoped or removed

### `ThemeProvider.tsx`
- Currently: reads `prefers-color-scheme` and toggles `lightTheme`/`darkTheme` class on `document.documentElement`
- NO localStorage persistence
- NO user-controlled theme selection
- Must be extended to: read from `localStorage`, support 4 themes, expose a `setTheme` function via context

---

## 2. Current Layout

### Root Layout (`web-app/src/app/layout.tsx`)
- `<html lang="en" className={lightTheme} suppressHydrationWarning>` — starts with lightTheme class baked in
- Providers: `ViewportProvider > ErrorBoundary > AuthProvider > Providers`
- Contains `<ConditionalHeader />`, `{children}`, `<NotificationPanel />`
- No sidebar, no drawer nav, no collapsible panel

### Header (`web-app/src/components/layout/Header.tsx`)
- Top horizontal bar with branding ("Stapler Squad", "Session Manager")
- Links to all nav pages, uses `NAV_PAGES` constant
- Contains: hamburger menu (mobile), ReviewQueueNavBadge, ApprovalNavBadge, UnfinishedNavBadge, notification bell, omnibar trigger, debug menu, WorkspaceSwitcher
- `ApprovalDrawer` is mounted within the Header component (slide-in from right)
- Keyboard: only Escape to close mobile menu

### Session List (`web-app/src/app/page.tsx`)
- Home page uses `SessionList`, `SessionDetail`, `SessionWizard`, `ResumeSessionModal`
- Full-page layout — no explicit column grid
- Session selection opens `SessionDetail` as a panel/modal (managed by state `selectedSession`)
- Keyboard shortcuts via `useKeyboard`: existing j/k navigation in review queue; omnibar handles Cmd+K/n
- Focus management: modal refs, trigger refs, useEffect-based focus return on close

### BottomNav (`web-app/src/components/layout/BottomNav.tsx`)
- Mobile bottom navigation bar
- Not relevant for desktop cockpit layout

**What needs to change:**
- Layout needs complete redesign: collapsible left drawer nav, 3-column session view
- Header repurposed or integrated into drawer nav
- Terminal should fill full height in detail pane
- Page transitions between nav sections

---

## 3. Current Review Queue

### `ApprovalPanel.tsx`
- Simple panel showing `ApprovalCard` list with header + count badge
- `onResolved` callback when all approvals drain
- **Renders null when no approvals** — completely invisible
- No persistent banner
- No keyboard shortcuts (y/n) for approve/deny

### `ApprovalCard.tsx`
- Shows: tool name, countdown timer (with urgency color change at 30s/10s), session name, command preview, working directory
- Expandable details section ("Show full details")
- Approve (green) / Deny (red) / Dismiss (expired) buttons
- Countdown classes: `countdownNormal`, `countdownWarning`, `countdownUrgent`
- NO keyboard shortcut support for approve/deny
- NO notification toast on resolution

### `ApprovalDrawer.tsx`
- Non-modal right-side drawer (no backdrop overlay)
- Sorts approvals by time-to-expire (most urgent first)
- Has `aria-live` region for expiry announcements
- Closes on Escape
- Focuses close button on open
- Triggered from `Header.tsx` — header manages `isApprovalDrawerOpen` state

### `ApprovalDrawer.css.ts` (likely slide-in animation)
File exists at 2KB — drawer has some CSS animation but minimal

**What needs to change per R4:**
- Persistent approval banner visible at all times (not `return null` when empty)
- Slide-in panel (extend existing drawer with better animation)
- Keyboard `y` to approve, `n` to deny focused approval
- Notification toasts on approve/deny resolution
- Visual urgency hierarchy needs stronger cyberpunk treatment

---

## 4. Current Keyboard Shortcuts

### `useKeyboard.ts`
- Global `window.addEventListener("keydown")` hook
- Ignores events from INPUT, TEXTAREA, SELECT elements
- `requireModifier` option available
- `useArrowNavigation` for list navigation (ArrowUp/Down/Home/End/Enter)
- Used throughout the app for session list navigation

### `OmnibarContext.tsx` keyboard bindings
- `Cmd+K` / `Ctrl+K` — toggle omnibar (discovery mode)
- `Cmd+Shift+K` — open omnibar in creation mode
- `n` (when not in input) — open omnibar
- All registered via `document.addEventListener("keydown")` in a `useEffect`

### Header keyboard
- `Escape` — close mobile menu (Header.tsx useEffect)

### ApprovalDrawer keyboard
- `Escape` — close drawer (useEffect listener)

### Review Queue Navigation (`useReviewQueueNavigation.ts`)
- Exists! 4KB file — likely has j/k or arrow navigation for review queue items

### `useFocusTrap.ts`
- Focus trap hook for modals — already implemented

**What needs to change per R5:**
- Centralized shortcut registry (currently scattered across 5+ components)
- `?` overlay showing all available shortcuts
- `<kbd>` styled elements in UI hints
- Theme-switching command in omnibar
- `y`/`n` for approval in ApprovalCard (context-sensitive — only when card is focused)
- `j`/`k` for session list navigation (must NOT fire when terminal is focused)

---

## 5. Current Animation/Transition Code

### Omnibar animations (Omnibar.css.ts)
- `fadeIn` keyframe: opacity 0→1
- `slideDown` keyframe: `translateY(-20px)` → `translateY(0)` + opacity
- `spin` keyframe: full rotation (used for loading)
- Omnibar overlay uses `animation: ${fadeIn} 0.15s ease-out`
- Omnibar modal uses `animation: ${slideDown} 0.2s ease-out`

### globals.css animations
- Card stagger animation via `--card-index` CSS variable (inline style)
- Various hover transitions on `.card:hover`

### ApprovalCard.css.ts
- Countdown urgency transitions (color change)
- No pulse/glow animations

### Missing animations needed:
- Scanline overlay (fixed, pointer-events: none, globally applied)
- Status glow/pulse on session cards
- Page transition via View Transitions API
- Drawer slide-in animation (enhance existing ApprovalDrawer)
- Focus glow states on interactive elements

---

## 6. Accessibility — Current State

### Existing Axe setup
- `tests/e2e/accessibility.spec.ts` exists and is the CI gate
- Uses `@axe-core/playwright` (`AxeBuilder`)
- Tests: main page + `/review-queue` route
- Blocks on `critical` and `serious` WCAG 2.1 AA violations
- Terminal `pre` elements excluded from Axe analysis
- Lighthouse CI config exists (`tests/e2e/lighthouse.config.js`)

### Existing accessibility features
- `suppressHydrationWarning` on `html` element
- Focus management in modals (refs + useEffect)
- `useFocusTrap.ts` hook
- `aria-label` on ApprovalDrawer, buttons
- `aria-live` region in ApprovalDrawer for expiry announcements
- Skip link: `<a href="#main-content">Skip to main content</a>` in layout.tsx
- Touch target minimum: `--min-touch-target: 44px` in globals.css

### Accessibility gaps for cyberpunk themes:
- Matrix green (#00ff41) on black (#000000): WCAG AA requires 4.5:1 ratio for normal text. `#00ff41` on `#000000` = ~10:1 — good. But at small sizes, WCAG AAA (7:1) may be needed.
- Cyberpunk neon cyan on dark: depends on specific values chosen
- Glow effects must not be the sole indicator of state (color-blind users)
- Reduced motion must be respected for all animations
- No Storybook exists yet — no component-level accessibility testing

---

## Summary: What Exists vs. What Needs Building

| Area | Exists | Needs Building |
|------|--------|----------------|
| Theme contract | ✅ Full contract | Add glow/scanline tokens |
| Theme impl | ✅ Light + Dark | Add Matrix, Cyberpunk77, WH40K themes |
| Theme switching | ❌ Only prefers-color-scheme | localStorage + user selector + Cmd+K command |
| Layout | Flat single-column | 3-column cockpit, collapsible drawer nav |
| Terminal | ✅ Full height in detail | Verify fills properly in new layout |
| Review queue | Basic panel + drawer | Persistent banner, y/n shortcuts, toasts |
| Keyboard system | Scattered hooks | Centralized registry, ? overlay, kbd elements |
| Animations | Fade/slide in omnibar | Scanlines, status glow/pulse, page transitions |
| Visual regression | ✅ Playwright infra | Multi-theme snapshot projects |
| Accessibility | ✅ Axe CI gate | Contrast audit for new themes, focus management in drawer |
| Storybook | ❌ Not installed | Setup with addon-themes, Chromatic |
