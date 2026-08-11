# Research Plan: Mobile UX Improvements

Created: 2026-04-07

## Subtopics

### 1. Stack — iOS Safari Viewport & Keyboard Behavior
**Goal**: Understand `dvh`, `visualViewport` API, `env(safe-area-inset-*)`, and `100vh` behavior on iOS Safari.
**Search strategy**: Web search (MDN, web.dev, CSS-tricks, iOS Safari release notes)
**Searches**: 3–5
**Output**: `research/findings-stack.md`

### 2. Features — Existing Codebase Patterns
**Goal**: Document existing mobile-friendly patterns in claude-squad web UI (toolbar overflow, hamburger menus, bottom sheets, touch targets).
**Search strategy**: Codebase exploration (Glob, Grep, Read)
**Searches**: N/A — codebase only
**Output**: `research/findings-features.md`

### 3. Architecture — Keyboard-Aware Layout State
**Goal**: Identify best approach for keyboard-aware layout (CSS-only `dvh`, `visualViewport` hook, or React context). Evaluate trade-offs.
**Search strategy**: Web search + codebase context
**Searches**: 3–5
**Output**: `research/findings-architecture.md`

### 4. Pitfalls — iOS Safari Known Bugs
**Goal**: Document known iOS Safari issues: keyboard resize events, `dvh` support matrix, `overscroll-behavior`, xterm.js + virtual keyboard conflicts.
**Search strategy**: Web search (caniuse, Safari release notes, xterm.js issues, Stack Overflow)
**Searches**: 3–5
**Output**: `research/findings-pitfalls.md`

## Scope Constraint
Each agent: max 5 searches. Synthesize only from output files — do not carry subagent context forward.
