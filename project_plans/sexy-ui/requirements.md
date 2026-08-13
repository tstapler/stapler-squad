# Requirements: stapler-squad UI Redesign ("sexy-ui")

## Project Context

stapler-squad is a Go + React web app (localhost:8543) that manages AI coding agent sessions
(Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees. It currently uses a
matrix-green terminal aesthetic. This redesign pivots to a modern, engineering-grade dark UI
modeled after Linear/Vercel — tight spacing, slate palette, sharp typography, no visual chrome.

## Stakeholders

- Primary: solo developer / small team power users who manage many parallel AI sessions
- Secondary: new users who need onboarding to understand the worktree + session model

## Aesthetic Direction

**Model**: Linear / Vercel
- Dark slate background (#0a0a0b / #111113 range), not pure black
- Neutral text (#e2e8f0 primary, #94a3b8 secondary, #475569 muted)
- Single accent color: indigo/violet (#6366f1) for interactive states, not green
- Subtle borders (#1e293b) — no heavy dividers
- Inter or system-ui sans-serif for UI labels; JetBrains Mono / Fira Code for terminal/paths
- Micro-animations: hover state transitions (100-150ms ease), no heavy spring physics
- Status indicators: colored dots (green=running, amber=paused, slate=idle) — no text badges

## Must-Have Requirements

### REQ-1: Dark mode theme overhaul
**Goal**: Replace matrix-green palette with Linear/Vercel dark slate throughout the entire UI.

**Acceptance criteria**:
- No `#00ff00`, `#00cc00`, or any "matrix green" color remaining anywhere in the app
- Global CSS variables in `globals.css` updated to slate dark palette
- vanilla-extract theme contract (`theme.css.ts`) updated to match
- All existing components pick up the new theme without individual component changes where possible
- Terminal foreground uses neutral white/light-gray, not green
- Accent color (buttons, focus rings, active states) is indigo/violet
- Dark background: `#0f1117` or similar slate-dark; card surfaces: `#161b22` or similar

### REQ-2: Session list — compact row density
**Goal**: Replace padded cards with compact single-line rows so users can see many more sessions without scrolling.

**Acceptance criteria**:
- Each session row is 36–40px tall maximum
- Single line per row: `[status dot] [branch/name] [agent icon] [path (truncated)] [elapsed time]`
- No multi-line cards — all metadata fits on one line or is accessible via hover/expand
- Running count: should fit 15+ sessions in a standard 1080p window without scrolling
- Hover state reveals an inline action row (pause/resume/delete icons) without layout shift
- Grouping headers (by category/tag/branch) are slim: 24px, muted label, no heavy dividers
- Session row click navigates to session; no wasted click targets

### REQ-3: Merge Config + Settings pages
**Goal**: Consolidate all configuration surfaces into a single Settings page.

**Acceptance criteria**:
- Single `/settings` route replaces any separate `/config` or `/preferences` routes
- Settings page uses a tab or sidebar-section layout: General | Sessions | Appearance | Keyboard Shortcuts
- All fields currently split across multiple pages are accessible from this one destination
- Navigation items pointing to old routes are removed or redirected
- No duplicate settings exposed in multiple places

### REQ-4: First-run onboarding flow
**Goal**: New users see a structured introduction on first open that teaches the core model
(sessions, worktrees, the omnibar) and key keyboard shortcuts.

**Acceptance criteria**:
- Triggered once on first visit (localStorage flag `stapler-squad:onboarded`)
- Modal/overlay with 4-5 steps: (1) What is stapler-squad, (2) Sessions + worktrees concept,
  (3) The omnibar (⌘K), (4) Key shortcuts reference, (5) "You're ready" CTA
- Each step has a brief headline, 1-2 sentence body, and optional illustration/ASCII diagram
- Skip button on every step; "Don't show again" checkbox on last step
- Re-triggerable from Settings > Help or via keyboard shortcut `?` / `⌘?`

### REQ-5: Always-accessible keyboard shortcut cheatsheet
**Goal**: Users can pull up a reference of all keyboard shortcuts at any time.

**Acceptance criteria**:
- Keyboard shortcut `?` (or `⌘?` / `Shift+?`) opens a cheatsheet panel
- Panel lists all registered shortcuts grouped by context (Global, Session List, Terminal, Omnibar)
- Panel is dismissible with Escape
- Cheatsheet is also accessible from Settings > Keyboard Shortcuts tab
- Shortcut list is defined in a single source-of-truth file (not duplicated between UI and docs)

### REQ-6: In-app documentation hub
**Goal**: Users have access to searchable help docs without leaving the app.

**Acceptance criteria**:
- `/help` route with a searchable documentation index
- At minimum covers: session types, omnibar usage, keyboard shortcuts, configuration options,
  worktree management, tmux integration
- Search filters docs in real-time (client-side, no server round-trip)
- Accessible from the Settings page and from the onboarding flow's "Learn more" links
- Docs content is stored as markdown files and rendered in-app

## Nice-to-Have Requirements

### NTH-1: Smooth micro-animations
- Hover/focus transitions: 100-150ms ease
- Session status dot pulse animation for "running" state
- Omnibar open/close: fade + scale (not slide, which feels slow)

### NTH-2: Command palette improvements
- Show keyboard shortcut annotations next to each omnibar result
- Recent/pinned sessions section at top when omnibar is opened with no input

### NTH-3: Responsive layout
- Sidebar collapses to icon-only below 1024px
- Session list adapts to two-column grid on very wide displays (>1800px)

## Technical Constraints

- CSS architecture: vanilla-extract for new components; CSS Modules for edits to existing ones
- Token contract lives in `web-app/src/styles/theme.css.ts`; no hardcoded hex in `.css.ts` files
- All new CSS variables must be added to `globals.css` before referencing them
- `make lint:css` must pass — no undefined CSS variable references
- No new heavy animation libraries (Framer Motion is acceptable if already a dependency; otherwise use CSS transitions)
- No breaking changes to the ConnectRPC API surface
- Settings consolidation must not break existing config persistence (JSON file in `~/.stapler-squad/`)

## Out of Scope

- Mobile/phone layout
- Light mode
- Theming system (user-selectable themes) — one well-designed dark theme is the goal
- Real-time collaboration features
- Backend API changes (except as required for new help/docs endpoint if needed)
