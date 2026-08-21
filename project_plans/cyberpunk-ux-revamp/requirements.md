# Requirements: Cyberpunk UX Revamp

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-05-02

## Builds On

This plan extends and does NOT supersede:
- `project_plans/ux-overhaul/` — mobile keyboard/layout fixes, session actions from session view, terminal toolbar
- `project_plans/squad-ux-polish/` — existing polish items

New theme, interaction, and tooling decisions go in `cyberpunk-ux-revamp/decisions/`.

---

## Vision

Transform stapler-squad from a functional-but-flat developer tool into an immersive, cyberpunk-aesthetic session management cockpit that feels like **being a cyborg jacked into the matrix**. The Matrix green-on-black theme is the default experience. Users can switch to three other themed skins. The UI should demonstrate what a polished AI developer tool cockpit looks like — suitable for demos to broader teams and stakeholders while remaining a power-user keyboard-first tool.

---

## User Profile

**Primary**: Solo developer and small teams (2–5 devs) using stapler-squad daily to manage Claude/Aider sessions.
**Secondary**: Broader team members and stakeholders seeing demos — needs to look impressive and professional.

---

## Problem Statement

1. **Visual flatness** — Every element looks the same weight. No hierarchy, no drama. Running sessions don't feel alive.
2. **Keyboard-blind** — Power actions require mouse. Keyboard shortcuts exist but have no visual cues or discoverability.
3. **Session list cognitive overload** — Long lists with no visual differentiation between running/paused/complete make scanning painful.
4. **Review queue flow is disconnected** — Approving tool calls, receiving notifications, and acting on them are three separate workflows with no cohesive flow.
5. **Layout wastes space** — No side-by-side layout option; the terminal panel and session list compete for vertical space.
6. **No theme personality** — The current light/dark toggle is purely functional. There is no joy or identity in the visual design.
7. **No visual quality gate** — Design regressions sneak in because there is no visual regression testing, no accessibility CI gate, and no component catalog.

---

## Requirements

### R1 — Theme System (4 themes)

**R1.1 Matrix (default)**
- Background: `#000000` / surfaces: `#0a0a0a`, `#0d0d0d`
- Primary text: `#00ff41` (Matrix green), muted: `#004d18`
- Accent glow: `0 0 8px #00ff41`, `0 0 16px rgba(0,255,65,0.4)`
- Border: `#003300` — subtle grid lines
- Font: JetBrains Mono for all UI text (not just terminal)
- FX: CSS scanlines overlay (::before pseudo on body), glitch hover effect on interactive elements
- Status indicators: running = pulsing green glow, approval-needed = amber pulse, paused = dim green

**R1.2 Cyberpunk 2077**
- Background: `#0d0d1a` / surfaces: `#12122a`, `#1a1a35`
- Primary text: `#fcee09` (yellow), accent: `#ff2d78` (hot pink), secondary accent: `#00d4ff`
- Glow: `0 0 12px #ff2d78` on active elements
- Border: `#1a1a3e` with diagonal angular cuts via `clip-path` on cards
- Font: Rajdhani (headings) + JetBrains Mono (data/code)
- FX: Glitch text animation on page load, animated gradient border on focused inputs

**R1.3 Warhammer 40K — Grimdark**
- Background: `#0c0a08` / surfaces: `#1a1510`, `#221e18`
- Primary text: `#c8b89a` (bone/parchment), accent: `#8b1a1a` (blood red), trim: `#c0a020` (gold)
- Border: `#3d3020` with double-border effect (outer gold, inner dark)
- Font: Cinzel (headings) + JetBrains Mono (data)
- FX: Vignette on body edges, parchment texture via SVG noise filter, skull/cog subtle watermark

**R1.4 Clean Default — Modern Dark**
- Background: `#0f0f11` / surfaces: `#1a1a1f`, `#22222a`
- Primary text: `#f0f0f0`, muted: `#888`, accent: `#7c3aed` (purple)
- Border: `#2a2a35`
- Font: Inter / system-ui
- FX: None — clean, demo-safe, professional
- Includes light mode variant (system preference / manual toggle)

**R1.5 Theme Contract**
- All tokens defined in `web-app/src/styles/theme.css.ts` via `createTheme` (vanilla-extract)
- Each theme exports a `themeClass` applied to `<body>`
- Theme persisted in `localStorage['stapler-theme']`
- Theme switchable via: settings page picker + Cmd+K omnibar command `theme: <name>`

---

### R2 — Layout: Side-by-Side Cockpit

**R2.1 Collapsible Drawer Navigation**
- Left nav drawer: 240px expanded, 56px icon-only collapsed
- Collapse triggered by: button in header, keyboard shortcut `[`, auto-collapse below 1024px viewport
- Nav items: Sessions (with count badge), Review Queue (with badge), History, Rules, Config, Logs
- Active item highlighted with theme accent color + left border indicator

**R2.2 Three-Column Session View**
- Column 1 (session list): 280px, collapsible, groups/tags/status at a glance
- Column 2 (session detail + terminal): fills remaining space
- Column 3 (context panel): 320px, slides in for diff/files/approval — replaces full-page overlays
- Responsive: ≤768px collapses to single-column with bottom sheet panels

**R2.3 Terminal Space Optimization**
- Terminal fills full available height in column 2
- Action bar above terminal: compact, fixed height, keyboard shortcut hints visible
- No vertical scrolling in the session detail header area — all key info (branch, status, path) fits in a single compact bar above the terminal

---

### R3 — Session List Redesign

**R3.1 Visual Status Differentiation**
- Running sessions: animated left border pulse in theme accent color
- Awaiting approval: amber/warning left border + badge, audible option
- Paused: dimmed card (50% opacity), grey left border
- Complete/error: muted appearance with result badge

**R3.2 Keyboard Navigation**
- `j`/`k` or arrow keys navigate session list
- `Enter` opens selected session
- `p` pauses, `r` resumes, `d` deletes (with confirmation), `a` opens in terminal
- `?` overlay shows all shortcuts with theme-styled keyboard shortcut visualization

**R3.3 Hierarchy and Grouping**
- Group headers rendered as collapsible section headers (not just visual separators)
- Expand/collapse all groups with keyboard shortcut
- Sticky group headers as list scrolls
- Session count shown per group

---

### R4 — Review Queue Redesign

**R4.1 Unified Notification + Approval Flow**
- Approval needed sessions surface immediately in a persistent top-of-page banner (not just nav badge)
- Clicking the banner opens a slide-in panel (not a full page navigation) showing pending approvals
- Approval actions: Approve (`y`), Deny (`n`), Approve All (`shift+y`), keyboard-navigable list
- After approving/denying, auto-advance to next pending item

**R4.2 Notification Integration**
- In-app notification toast appears when a new approval is needed (theme-styled)
- Toast includes: session name, the tool call being requested, one-click Approve/Deny
- Notification persists as a badge on the nav item until cleared

**R4.3 Approval Card Design**
- Card shows: session name, tool name, arguments (syntax-highlighted), risk level badge
- Risk levels: LOW (green glow), MEDIUM (amber glow), HIGH (red glow, requires explicit confirm)
- Animated entry — card slides in from right

---

### R5 — Keyboard-First Interaction

**R5.1 Keyboard Shortcut System**
- All primary actions have keyboard shortcuts
- Shortcut hints rendered as styled `<kbd>` elements throughout UI (not just tooltips)
- `?` key opens a searchable shortcut reference overlay in theme style
- New shortcuts: `[` toggle nav drawer, `t` open terminal focus, `\`` cycle through open sessions, `` ` `` open omnibar

**R5.2 Omnibar Enhancements**
- Theme command: `theme matrix`, `theme cyberpunk`, `theme 40k`, `theme clean`
- Navigation commands: `go sessions`, `go review`, `go history`
- Session actions from omnibar: `pause <name>`, `resume <name>`, `approve all`
- Opening animation: scanline wipe effect (theme-dependent)

**R5.3 Visual Keyboard Cue System**
- Action bars show shortcut key hints inline: `[p] Pause  [r] Resume  [d] Delete`
- Hover on any actionable element shows its shortcut in a theme-styled tooltip
- Keyboard shortcuts use the theme's accent color for the key badge

---

### R6 — Micro-Interactions and Animations

**R6.1 Status Glow / Pulse**
- Running sessions: `box-shadow` keyframe animation pulsing theme accent
- CSS: `@keyframes pulse-glow` with 2s ease-in-out infinite
- Respect `prefers-reduced-motion` — disable all animations when set

**R6.2 Omnibar Opening Effect**
- Matrix theme: scanline sweep down (CSS animation, 120ms)
- Cyberpunk: glitch flash (3-frame position jitter, 80ms)
- 40K: parchment unfurl (scale + opacity, 150ms)
- Clean: fade-in (100ms)

**R6.3 Page Transitions**
- View transitions API (`document.startViewTransition`) for page navigation
- Fallback: simple opacity fade for browsers without support
- Duration: 150ms, easing: `ease-out`

**R6.4 Hover and Focus States**
- Cards: theme-color left border slides in on hover (`transition: border-left-color 100ms`)
- Buttons: glow `box-shadow` appears on hover (`transition: box-shadow 150ms`)
- Focus rings: 2px offset, theme accent color (replaces browser default)

---

### R7 — Quality Tooling (Process & CI)

**R7.1 Visual Regression Testing**
- Playwright visual snapshot tests for all 4 themes × key page states
- Snapshots stored in `tests/e2e/screenshots/`
- CI step: `playwright test --update-snapshots=none` — fails on diff > threshold
- Threshold: 0.1% pixel difference (allows antialiasing variance)

**R7.2 Accessibility Auditing**
- Axe Core already partially in place — extend to all 4 theme variants
- Block PRs on WCAG 2.1 AA violations (critical + serious)
- Warn on all 4 themes simultaneously: Matrix green needs contrast ratio check
- Report uploaded to Allure

**R7.3 Design Token Linting**
- `lint:css` CI step already in place — enforce no hardcoded hex values in `.css.ts` files
- Add ESLint rule `no-restricted-syntax` to catch inline `style={{ color: '#...' }}` in `.tsx` files
- Token coverage report: what % of color usages are tokenized

**R7.4 Component Storybook**
- Storybook 8 configured in `web-app/`
- Stories required for: all theme variants of Button, Badge, Card, SessionRow, ApprovalCard, Omnibar
- Chromatic integration for PR visual diff review
- Stories co-located with components: `Button.stories.tsx` next to `Button.tsx`

**R7.5 Automated Theme Contrast Check**
- Custom script `scripts/check-theme-contrast.ts` — reads theme token values and validates WCAG AA contrast ratios
- Run in CI and locally with `npm run check-contrast`
- Reports: token name, foreground, background, ratio, PASS/FAIL

---

## Non-Goals

- Native app or Electron wrapper
- Animation beyond CSS (no Three.js, WebGL, or canvas)
- Theme customizer (user-created themes) — too complex for this phase
- Monetization or multi-tenancy
- Full Warhammer IP artwork (use abstract aesthetic only)

---

## Success Criteria

1. **All 4 themes render without any hardcoded values** — `lint:css` passes
2. **WCAG 2.1 AA contrast** — all themes pass `check-contrast` script, Axe Core in CI
3. **Visual regression baseline established** — Playwright snapshots captured for all themes × 3 pages
4. **Storybook deployed** — all key components have stories in all 4 theme variants
5. **Keyboard completeness** — every primary session action accessible without a mouse
6. **Review queue flow** — approval/deny possible without leaving the current page
7. **Performance** — Lighthouse performance score ≥ 70, no regression from current baseline
8. **Side-by-side layout** — terminal and session list visible simultaneously on ≥ 1280px viewports

---

## Constraints

- Must use vanilla-extract for all new CSS (ADR-009)
- No new CSS-in-JS runtimes (styled-components, emotion)
- Existing theme contract in `theme-contract.css.ts` must be extended, not replaced
- Must not break existing E2E tests
- Must respect `prefers-reduced-motion` for all animations
- JetBrains Mono must be loaded via `next/font` (no external CDN)
