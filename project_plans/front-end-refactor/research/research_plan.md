# Research Plan: Front-End Refactor

**Date**: 2026-04-16
**Input**: `project_plans/front-end-refactor/requirements.md`

## Subtopics

### 1. Stack (`findings-stack.md`)
What component library and design-system primitives to adopt; how to complete the vanilla-extract migration; what web+RN code-sharing strategy to use.

**Search strategy**:
- shadcn/ui vs Radix UI primitives vs headless UI — adoption, bundle, a11y
- vanilla-extract `createTheme` vs CSS custom properties bridge — migration patterns
- Expo Router + Next.js monorepo — shared hooks/state, 2025 state of the art
- Turborepo vs Nx for web+RN workspace

**Search cap**: 4 searches
**Trade-off axes**: Bundle size, Accessibility coverage, TypeScript ergonomics, RN compatibility, Migration effort from current state

---

### 2. Features (`findings-features.md`)
What UX patterns work for mobile terminal apps, touch-friendly code/diff viewers, foldable screens, and review queue interfaces.

**Search strategy**:
- xterm.js mobile touch input and virtual keyboard — known solutions
- CodeMirror 6 vs Monaco on mobile — which works better on Android
- Foldable Android CSS media queries — `fold` env(), `screen-fold-angle`
- Review queue / approval workflow mobile UX patterns

**Search cap**: 4 searches
**Trade-off axes**: Touch usability, Foldable support, Pixel Fold compatibility, Existing integration cost

---

### 3. Architecture (`findings-architecture.md`)
How to structure a design system with tokens → primitives → features; how to manage server state with ConnectRPC; monorepo layout for shared logic.

**Search strategy**:
- Design system architecture layering — Brad Frost atomic design in practice
- RTK Query + ConnectRPC integration pattern — 2024/2025
- Monorepo `packages/ui-web` + `packages/ui-native` + `packages/core` pattern
- State colocation best practices React 19 with RTK

**Search cap**: 4 searches
**Trade-off axes**: Separation of concerns, Code reuse %, Tooling complexity, Incremental adoptability

---

### 4. Pitfalls (`findings-pitfalls.md`)
Known failure modes for each major technology change in this refactor.

**Search strategy**:
- xterm.js WebGL addon + mobile Safari/Chrome Android — known bugs 2024
- vanilla-extract migration from CSS Modules — pitfalls in large codebases
- React Native + Next.js shared logic — what actually can and can't be shared
- Pixel 9 Pro Fold web browser quirks — viewport, fold state API

**Search cap**: 4 searches
**Trade-off axes**: Severity, Likelihood, Mitigation availability, Blocking vs. non-blocking

---

## Output Files

| File | Owner | Status |
|------|-------|--------|
| `research_plan.md` | parent | ✅ done |
| `findings-stack.md` | subagent-1 | pending |
| `findings-features.md` | subagent-2 | pending |
| `findings-architecture.md` | subagent-3 | pending |
| `findings-pitfalls.md` | subagent-4 | pending |
| `synthesis.md` | parent | pending |

## Already-Decided (do not re-litigate)

From existing ADRs in this repo:
- **ADR-001** (mobile-ux-improvements): CSS variable bridge via ViewportProvider
- **ADR-002** (mobile-ux-improvements): ResizeObserver for xterm fit
- **ADR-003** (mobile-ux-improvements): Sticky flex layout for toolbar/keyboard avoidance
- **ADR-004** (mobile-ux-improvements): Mobile keyboard toggle + localStorage state
- **ADR-001** (terminal-jank): Terminal instance pool
- **ADR-002** (terminal-jank): xterm upgrade to 6.0
- **ADR-001** (history-page-revamp): TanStack Virtual for virtualization
