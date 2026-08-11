# Requirements: Front-End Refactor

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-16

## Problem Statement

The web front end has accumulated several compounding issues that slow feature delivery and produce a poor user experience — especially on mobile (primary test device: Pixel 9 Pro Fold):

- **Split CSS standards**: ~70 CSS Module files coexist with a partially-adopted vanilla-extract system. New components feel disconnected; dark/light mode coverage is inconsistent.
- **No shared component primitives**: Buttons, modals, inputs, and layout patterns are re-implemented per feature rather than composed from a library. Adding a new screen means reinventing the wheel.
- **Mobile is broken in practice**: Interactive components don't work correctly on the Pixel 9 Pro Fold; the fold form factor (large inner screen + small outer screen) is not handled. Touch targets are undersized.
- **Weak quality story**: UI jank is hard to diagnose. There is no systematic approach to component testing, visual regression, or accessibility.
- **State management is ad-hoc**: Redux slices are feature-siloed with no clear data-fetching layer; it's unclear what belongs in global vs. local state.
- **Slow iteration velocity**: The absence of shared primitives and clear patterns means each new feature requires disproportionate effort.

The longer-term goal is to ship a React Native mobile app that shares business logic with the web app, making the current refactor a foundation investment.

## Success Criteria

By the end of the 3–6 month refactor window:

1. **All components use vanilla-extract** — zero new CSS Module files; existing `.module.css` files fully migrated.
2. **Shared component primitive library** — a set of typed, composable UI primitives (Button, Modal, Card, Input, etc.) used consistently across all screens.
3. **App is fully usable on Pixel 9 Pro Fold** — inner and outer screen both work; all interactive targets meet touch minimum (44×44 dp); virtual keyboard doesn't obscure inputs.
4. **React Native code-sharing foundation** — business logic (hooks, data-fetching, state slices) is decoupled from web rendering so a React Native app can consume it.
5. **Quality baseline** — each new primitive ships with unit + visual tests; no regressions in existing Playwright e2e suite.
6. **Faster feature iteration** — new screens built from primitives, not bespoke CSS.

## Scope

### Must Have (MoSCoW)
- Full vanilla-extract migration (all CSS Modules replaced)
- Shared UI primitive library (design system layer)
- Consistent design token contract (`createTheme` — replacing the current hand-rolled `vars` wrapper)
- Mobile-responsive layout shell (header, navigation, sidebar)
- Mobile-usable session list + cards
- Mobile-usable terminal view (xterm.js touch input, virtual keyboard integration)
- Mobile-usable diff/file viewer (CodeMirror/Monaco or mobile-appropriate alternative)
- Mobile-usable review queue (user's highest-priority workflow)
- Code-sharing architecture: logic layer (hooks, RTK slices, ConnectRPC clients) separated from rendering layer
- Testing infrastructure for UI components

### Should Have
- Storybook or equivalent component catalogue for primitives
- Accessibility audit (WCAG AA) for all new primitives
- RTK Query or equivalent for server-state (replacing ad-hoc ConnectRPC call patterns)

### Out of Scope
- Shipping a React Native app (code-sharing foundation is in scope; the RN app itself is not)
- Backend API changes (ConnectRPC / protobuf contract is fixed)
- Replacing Next.js, React, or Redux Toolkit
- Feature additions beyond what's needed to validate the new system

## Constraints

- **Tech stack (locked)**: Next.js 15, React 19, Redux Toolkit, ConnectRPC + protobuf, TypeScript
- **CSS target**: vanilla-extract (ADR-009 already adopted; migration must complete it)
- **Timeline**: 3–6 months
- **Team size**: Small (effectively 1 developer + AI assistance)
- **Device target**: Pixel 9 Pro Fold (Android, foldable — inner ~7.6" and outer ~6.1" screens)
- **Dependencies**: Server API is Go + ConnectRPC; no backend changes planned

## Context

### Existing Work

- **vanilla-extract partially adopted** (ADR-009): CSS architecture rules in `.claude/rules/css-architecture.md`; `theme.css.ts` exists but is a thin wrapper over CSS custom properties, not a full `createTheme` contract. Only `VcsStatusDisplay.css.ts` exists as a `.css.ts` file.
- **70+ CSS Module files** need migration.
- **mobile-ux-improvements** project plan exists (`project_plans/mobile-ux-improvements/`) with 4 ADRs already decided:
  - ADR-001: CSS variable bridge via ViewportProvider
  - ADR-002: ResizeObserver for xterm fit
  - ADR-003: Sticky flex layout for toolbar/keyboard avoidance
  - ADR-004: Mobile keyboard toggle + localStorage state
- **history-page-revamp** plan has ADRs for TanStack Virtual, cursor pagination — relevant if we redesign list views.
- **terminal-jank** plan has ADRs for xterm terminal pool and cold-start — relevant for terminal view quality.
- **`ViewportProvider`** already exists (`components/providers/ViewportProvider.tsx`) — foundation for responsive breakpoints.

### Stakeholders

- **Primary user / developer**: Tyler Stapler (sole developer; daily user on desktop and Pixel 9 Pro Fold)
- **End users**: Developers using stapler-squad to manage Claude Code sessions

## Research Dimensions Needed

- [ ] **Stack** — Evaluate component library options (Radix UI primitives, shadcn/ui, etc.); assess vanilla-extract `createTheme` vs. current approach; evaluate code-sharing strategies for web + React Native (Expo + Next.js monorepo, Nx, Turborepo)
- [ ] **Features** — Survey mobile-first terminal and code-review apps; touch-friendly diff viewers; existing mobile-usable xterm.js patterns; review queue UX patterns
- [ ] **Architecture** — Design system architecture (token contract, primitive layer, feature layer); RTK Query vs current ConnectRPC call patterns; monorepo structure for web + RN code sharing; state colocation strategy
- [ ] **Pitfalls** — vanilla-extract migration gotchas; xterm.js on mobile (touch, virtual keyboard, WebGL); Monaco/CodeMirror on mobile; React Native + Next.js code-sharing limitations; foldable-screen CSS media queries
