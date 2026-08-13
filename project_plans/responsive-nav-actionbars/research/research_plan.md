# Research Plan: Responsive Nav & Action Bars

**Date:** 2026-04-13
**Input:** requirements.md
**Stack research:** Reuse `project_plans/mobile-ux-improvements/research/findings-stack.md` — viewport units, visualViewport, safe-area insets all documented.

## Subtopics

### 1. Features — Toolbar/Action Bar Audit
**Goal:** Map every toolbar, filter bar, and action bar in the app. Record which ones use `ActionBar`, which are ad-hoc flex rows, and whether they have overflow/wrap/scroll behavior.
**Method:** Read codebase — Header, WorkspaceSwitcher, SessionList, HistoryFilterBar, logs/ components, ActionBar.tsx.
**Cap:** Codebase reads only, no web search needed.
**Output:** `findings-features.md`

### 2. Architecture — CSS Approach Decision
**Goal:** Evaluate 3 approaches: (a) extend ActionBar with mid-breakpoints, (b) container queries per component, (c) CSS-only per-component breakpoints. Determine which fits the project stack (CSS Modules + vanilla-extract, no new deps).
**Method:** 2–3 web searches on container queries + CSS Modules compatibility; read existing ActionBar impl.
**Cap:** 3 searches max.
**Output:** `findings-architecture.md`

### 3. Pitfalls — Overflow & Width Issues
**Goal:** Identify specific technical landmines: overflow:auto vs hidden conflicts in the header, WorkspaceSwitcher min-width at narrow widths, sticky header height changes with breakpoints, intermediate-width gap (~800–1100px) in Header.module.css.
**Method:** Read Header.module.css, WorkspaceSwitcher.module.css, globals.css; cross-reference with stack research.
**Cap:** Codebase reads only; 1 web search if needed.
**Output:** `findings-pitfalls.md`
