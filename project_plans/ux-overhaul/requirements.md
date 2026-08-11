# Requirements: UX Overhaul

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-26

## Supersedes

This plan is the single source of truth for all UX/frontend work. It absorbs and supersedes:

- `project_plans/mobile-ux-improvements/` — iOS keyboard/safe-area/toolbar overflow fixes
- `project_plans/front-end-refactor/` — design system, vanilla-extract migration, primitives
- `project_plans/responsive-nav-actionbars/` — toolbar/ActionBar consistency at all widths
- `project_plans/stapler-squad-painpoints/` — session actions, terminal quality, rename/retag

All ADRs in those plans remain valid and are referenced here. New decisions go in `ux-overhaul/decisions/`.

---

## Problem Statement

Stapler Squad's UI has grown organically — each feature added its own CSS, layout patterns, and interaction model without a shared foundation. The result is a collection of compounding UX friction points that slow down the daily developer workflow, especially on mobile (primary devices: iPhone iOS Safari and Pixel 9 Pro Fold):

1. **Terminal toolbar state-switching friction** — The terminal view has an expanded and compact toolbar mode. Switching between them is too many taps and the state change is visually jarring. When expanded, the toolbar eats vertical space that should belong to the terminal.

2. **Session name invisible during active sessions** — When inside a session, the session name is not persistently visible. On mobile, when the virtual keyboard opens, context is lost entirely and there is no way to know which session you are in.

3. **Mobile keyboard causes layout collapse** — On iOS Safari, `100vh` does not shrink when the virtual keyboard appears. Terminal and input fields are hidden behind the keyboard. The terminal does not use the full available horizontal space, and users accidentally tap outside the session area because interactive targets are too small or misplaced.

4. **Session actions siloed on the sessions list** — Delete, pause/resume, rename/retag, and worktree/branch actions are only available from the sessions list page. When the user navigates to a session (e.g. from a push notification), there is no way to take those actions without leaving the session view. This is the most common workflow: open notification → review session → delete or pause.

5. **Bottom navbar hides content** — On mobile, the bottom navigation bar overlaps page content when scrolled to the bottom. Pages with dense content (session list, filter bars, terminal) are cut off.

6. **No design system** — Buttons, modals, inputs, cards, and layout patterns are reimplemented per feature. ~70 CSS Module files coexist with a partially-adopted vanilla-extract system. Dark/light mode coverage is inconsistent. There are no shared primitives, so adding new screens requires disproportionate effort and produces UI inconsistency.

7. **Accessibility and affordance gaps** — Touch targets are often below 44px. Interactive elements do not communicate their state clearly. No systematic accessibility or WCAG audit has been done.

---

## Success Criteria

### Milestone 1 — Mobile Friction (ships first)

- Terminal toolbar: a single persistent toggle switches between compact (terminal-maximized) and expanded (toolbar visible) states; toggle is always reachable with one tap; state is remembered in localStorage
- Session name always visible in the session view header, even on mobile with keyboard open; no content shift when keyboard appears
- All session actions (delete, pause/resume, rename/retag, open worktree) accessible from within the session view — no need to return to the sessions list
- Bottom navbar does not occlude any content; page content has correct `padding-bottom` / `env(safe-area-inset-bottom)` to clear the navbar
- All interactive elements in the terminal and session view meet 44px minimum touch target; no accidental out-of-session taps
- iOS virtual keyboard: layout uses `dvh`/`visualViewport` so terminal and input remain visible when keyboard is open
- Safe-area insets applied (`env(safe-area-inset-*)`) so notch and home indicator never clip content

### Milestone 2 — Design System Foundation

- Full vanilla-extract migration: zero new CSS Module files; existing `.module.css` migrated to `.css.ts`
- Shared UI primitive library: Button, Modal, Card, Input, ActionBar, Badge used consistently across all screens
- Complete design token contract via `createTheme` (replacing ad-hoc CSS custom properties)
- Consistent dark/light mode coverage for all primitives
- All new primitives ship with unit + visual regression tests
- App fully usable on Pixel 9 Pro Fold (inner and outer screens; all touch targets ≥44dp)

### Milestone 3 — UX Best Practices Alignment

- Navigation and session management UX reviewed against established mobile app patterns (bottom sheet for actions, swipe-to-dismiss, contextual menus)
- Session list and session view share a consistent action surface — same actions available in both places
- No page has content hidden by fixed UI chrome (navbar, header, bottom bar) without correct inset compensation
- Interaction latency instrumented: click-to-render and RPC durations observable via OpenTelemetry

---

## Scope

### Must Have (Milestone 1 — mobile friction)

- Terminal toolbar compact/expanded toggle — always accessible one-tap toggle, state in localStorage
- Session name persistent in session view header
- Keyboard-aware layout: `dvh`/`visualViewport` API, layout does not collapse when iOS virtual keyboard opens
- `env(safe-area-inset-*)` applied globally in `globals.css` and key layout components
- Bottom navbar `padding-bottom` clearance on all pages
- Session actions in session view: Delete, Pause/Resume, Rename/Retag, Open worktree/branch (bottom sheet or action menu pattern)
- Touch target audit for terminal and session view — all controls ≥44px

### Must Have (Milestone 2 — design system)

- vanilla-extract `createTheme` token contract (`web-app/src/styles/theme.css.ts`)
- Primitive library: Button (with variants/sizes), Modal/BottomSheet, Card, Input, ActionBar, Badge
- Migration of top-10 most-used CSS Module files to `.css.ts`
- Responsive header with intermediate breakpoint (800–1100px) per `responsive-nav-actionbars` plan

### Should Have

- All remaining CSS Module files migrated to vanilla-extract
- Storybook or equivalent component catalogue
- WCAG AA accessibility audit for all primitives
- RTK Query for server-state (replacing ad-hoc ConnectRPC call patterns)
- Pixel 9 Pro Fold foldable breakpoints

### Could Have

- React Native code-sharing foundation (hooks/slices decoupled from rendering)
- Lazy/paginated terminal scrollback (send only last N lines on attach)
- Branch autocomplete and worktree state badges on session cards
- Bulk session actions

### Out of Scope

- Shipping a React Native app
- Backend API changes beyond what Milestone 1 session actions require
- Desktop Electron packaging
- Multi-user / shared session support
- New session types

---

## Constraints

- **Tech stack (locked)**: Next.js 15, React 19, Redux Toolkit, ConnectRPC + protobuf, TypeScript
- **CSS target**: vanilla-extract (ADR-009 already adopted); new styles in `.css.ts`; surgical edits to existing `.module.css` allowed for Milestone 1 fixes
- **No new npm packages for Milestone 1** unless unavoidable; Milestone 2 may introduce Radix UI primitives (ADR-010 already decided in `front-end-refactor/decisions/`)
- **Timeline**: Milestone 1 ships incrementally as small PRs; Milestone 2 is a 3–6 month window
- **Team size**: Solo developer + AI assistance
- **Primary device targets**: iPhone (iOS Safari 15.4+), Pixel 9 Pro Fold (Android Chrome)
- **Desktop**: must not regress (desktop experience is already good)

---

## Context

### Pre-decided ADRs (from absorbed plans)

From `mobile-ux-improvements/decisions/`:
- **ADR-001**: CSS variable bridge via ViewportProvider (`--keyboard-height`, `--viewport-height`)
- **ADR-002**: ResizeObserver for xterm fit (terminal resizes to fill container, not hardcoded)
- **ADR-003**: Sticky flex layout for toolbar/keyboard avoidance
- **ADR-004**: Mobile keyboard toggle + localStorage state

From `front-end-refactor/decisions/`:
- **ADR-010**: Radix UI headless primitives as the base for the shared component library
- **ADR-011**: `createThemeContract` token system (full vanilla-extract token contract)
- **ADR-012**: RTK Query at the protobuf boundary (replaces ad-hoc ConnectRPC calls)

### Existing Code Foundations

- `ViewportProvider.tsx` — writes `--keyboard-height` / `--viewport-height` CSS vars; already live
- `ActionBar.tsx` + `ActionBar.module.css` — scroll + wrap props exist; underused across the app
- Mobile keyboard overlay in `TerminalOutput.tsx:652-666` — exists, needs toggle
- `globals.css` — `--min-touch-target: 44px` defined but unused in most components
- Modal already renders as a bottom sheet on mobile (`globals.css:229-240`)
- `VcsStatusDisplay.css.ts` — only existing `.css.ts` component; reference implementation
- `theme.css.ts` — thin wrapper today, needs full `createTheme` contract

### Affected Components (Milestone 1)

- `web-app/src/components/sessions/TerminalOutput.tsx` + `.module.css` — toolbar toggle, session name header
- `web-app/src/components/sessions/SessionDetail.tsx` — session actions (add delete/pause/rename/worktree)
- `web-app/src/components/layout/BottomNav.tsx` (or equivalent) — safe-area padding
- `web-app/src/app/globals.css` — safe-area insets, dvh, root CSS vars
- `web-app/src/app/layout.tsx` — viewport meta
- `web-app/src/app/page.module.css` — modal height / 100vh issues

### Stakeholders

Tyler Stapler — sole developer and daily user on iPhone and desktop. Wants frictionless one-handed mobile use while reviewing active Claude Code sessions.

---

## Research Dimensions Needed

- [ ] **Stack** — `dvh` + `visualViewport` API browser support (iOS 15.4+ vs `100dvh`); `env(safe-area-inset-*)` in Next.js App Router; bottom sheet pattern options (Radix Dialog, Vaul, custom); Radix UI + vanilla-extract integration
- [ ] **Features** — Audit all session actions currently on the sessions list page; survey bottom-sheet/action-menu patterns in comparable mobile-first tools (Linear, Vercel dashboard, GitHub mobile); existing ActionBar usage across the codebase
- [ ] **Architecture** — Session actions API: does the session view already receive the session object with all needed fields for delete/pause/rename? What ConnectRPC calls are needed? Where should the action menu state live?
- [ ] **Pitfalls** — iOS Safari `dvh` rounding bugs; `env(safe-area-inset-bottom)` on Android; bottom sheet keyboard interaction (keyboard pushes sheet up vs. sheet stays fixed); xterm.js + visualViewport resize loop; Radix UI portal z-index conflicts with xterm canvas
