# Frontend Architecture Audit — Requirements

## Project Summary

Produce a comprehensive architectural audit of the `web-app/src/` frontend codebase for the stapler-squad project. The deliverable is a prioritized report of architectural opportunities — no code changes. The goal is to identify pattern consolidation opportunities, improve long-term maintainability, and evaluate tooling (linters, static analysis) that can automate ongoing enforcement.

## Tech Stack Context

- React (Next.js App Router) + TypeScript
- ConnectRPC (protobuf-based) for all API calls
- vanilla-extract for all CSS (0 `.module.css` files, 115 `.css.ts` files)
- Custom context + hooks for state management (no Zustand/Redux)
- 386 source files across `components/`, `lib/contexts/`, `lib/hooks/`, `app/`
- Jest + React Testing Library for unit tests; Playwright for e2e

## Problem Areas (from user)

### 1. State Management
- Multiple React Contexts for overlapping concerns (SessionServiceContext, OmnibarContext, ReviewQueueContext, ApprovalsContext, NavigationContext, etc.)
- Unclear ownership boundaries: where does session state live vs. where is it consumed?
- Prop drilling vs. context split is inconsistent

### 2. Component Coupling
- Large components that do too much (suspected in `Omnibar.tsx`, session cockpit area)
- Hard to change one component without breaking others
- No clear layer separation (data access / business logic / presentation)

### 3. CSS / Styling
- vanilla-extract adopted but consistency of token usage not audited
- Possible hardcoded values remaining in older `.css.ts` files
- Theme contract may have gaps; linting enforces it at build time but no report of coverage

### 4. Data Fetching
- ConnectRPC hooks (`useSessionService`, `useApprovals`, etc.) potentially duplicated across components
- Error/loading states handled inconsistently across data hooks
- No documented standard pattern for data fetching

### 5. Code Duplication / Unification Opportunities
- Hard to discover when similar code exists elsewhere that should be unified
- Need tooling recommendations: linters, AST-based scanners, or custom ESLint rules that flag duplicate logic/patterns

## Success Criteria

The audit must deliver:

1. **Pattern inventory**: catalog of every distinct pattern used for state, data fetching, CSS, and component structure — with file-level examples of each variant
2. **Duplication map**: specific instances of duplicated logic or near-identical components that should be unified, with file paths and line numbers
3. **Consolidation opportunities**: prioritized list of refactors ranked by impact/risk, each with: what to consolidate, why, what the canonical form should be
4. **Tooling recommendations**: ESLint plugins, TypeScript strict settings, or custom rules that would automate ongoing enforcement of the chosen patterns
5. **Architecture boundary diagram**: which layers exist, which are violated, and where

## Scope

- **In scope**: all of `web-app/src/` (components, lib/contexts, lib/hooks, app/, styles/)
- **Out of scope**: generated protobuf files in `gen/`, backend Go code
- **Output**: written to `project_plans/frontend-architecture-audit/implementation/plan.md` and supporting research files

## Non-Goals

- No code changes in this phase
- No new features
- No performance optimization (separate concern)

## Constraints

- The project has ADR-009 (vanilla-extract mandatory for new CSS)
- The project has strict registries for omnibar actions and session creation modes — any future refactoring must respect those 7-touchpoint checklists
- CI runs `lint:css`, TypeScript strict, and e2e tests — recommendations must be CI-enforceable
