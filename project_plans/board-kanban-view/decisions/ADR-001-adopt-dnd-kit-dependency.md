# ADR-001: Adopt `@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities` as a New Frontend Dependency

**Status**: Accepted
**Date**: 2026-08-06
**Project**: board-kanban-view

## Context

The board/kanban view (requirements.md, ACs 4/5/10) needs a drag-and-drop mechanic for moving
session cards between status columns, with:
- A client-side rejection path for illegal transitions (AC5).
- A non-drag fallback for touch devices, which WCAG 2.1 SC 2.5.7 (Dragging Movements, Level
  AA) makes a binding requirement, not optional polish, given this repo's CI-enforced Axe
  Core WCAG AA gate on `web-app/src/` PRs.
- Keyboard operability (no ARIA APG pattern exists for drag-and-drop at all — confirmed by
  direct search of the W3C ARIA APG index during research; this is a genuinely under-specified
  interaction with no canonical markup to copy).

No drag-and-drop library exists in `web-app/package.json` today (confirmed via grep — zero
matches for `dnd`/`drag`/`sortable` outside of unrelated resize-handle/terminal-gesture code
and `react-arborist`'s tree-internal DnD, which is not reusable for a multi-column board).

## Decision

Add three new runtime dependencies to `web-app/package.json`:
```json
"@dnd-kit/core": "^6.3.1",
"@dnd-kit/sortable": "^10.0.0",
"@dnd-kit/utilities": "^3.2.2"
```

## Alternatives Considered

| Option | Why rejected |
|---|---|
| `react-beautiful-dnd` | Archived by Atlassian Aug 2025 (read-only repo), deprecated on npm, no React 19 support. Adopting a dead dependency for new code is a maintenance liability from day one. |
| `@hello-pangea/dnd` | Community fork of react-beautiful-dnd with explicit React 19 peer support (cleaner on paper than dnd-kit's ambiguous-but-working peer range), but WebSearch flagged critically low contributor activity (1 active contributor/quarter) — a single-maintainer bus-factor risk on top of inheriting react-beautiful-dnd's now-superseded "physicality" design, which Atlassian itself moved away from. |
| `@atlaskit/pragmatic-drag-and-drop` | Actively developed, smallest bundle (<4KB core), production-proven at Jira/Trello/Confluence scale. Rejected for v1 because it is a lower-level primitives toolkit with no built-in keyboard-drag sensor — it deliberately favors action-menu alternatives over directional keyboard dragging, meaning more hand-assembly of standard kanban behavior (column reflow, autoscroll, keyboard path) for the same scope dnd-kit provides pre-built. Worth revisiting if a future performance need at 1000+ cards/board materializes; not expected for this app's realistic session counts (tens to low hundreds per workspace). |
| Hand-rolled native HTML5 DnD | The codebase already has one working example (`PaneSplitRenderer.tsx`'s 2-pane swap via `draggable`/`dragstart`/`drop`), but that only works for a fixed 1:1 swap. HTML5 native DnD has **no keyboard equivalent at all** — a keyboard-only user could not move a card between columns without a fully separate hand-built interaction path — and no touch event support (touch requires an entirely separate gesture system, not an extension of the mouse path). Given AC10 already mandates a non-drag fallback regardless of library choice, and that same fallback control can also serve as the keyboard path, hand-rolling the *drag* mechanic itself would still leave every failure mode (touch, autoscroll, drop-rejection animation, Safari/Firefox `dataTransfer` inconsistencies) to reimplement for no accessibility benefit over adopting a library. |

## Consequences

- **Bundle size**: `@dnd-kit/core` is 14.2KB gzip, `@dnd-kit/utilities` +1.6KB gzip (measured
  via Bundlephobia, 2026-08-06); well inside the project's `size-limit` budget (5MB total JS,
  ungzipped, per `web-app/package.json`'s `size-limit` config).
- **React 19 risk (accepted)**: `@dnd-kit/core`'s last publish was ~20 months before this
  decision (npm registry metadata, Dec 2024), and an open upstream issue
  ([clauderic/dnd-kit#1511](https://github.com/clauderic/dnd-kit/issues/1511)) tracks
  "officially support React 19" as still unresolved. The published peer dependency
  (`react: ">=16.8.0"`, no upper bound) installs and runs on React 19 without an override —
  the gap is "not yet advertised as tested," not a confirmed incompatibility. If a genuine
  React-19-specific defect surfaces during implementation, `@hello-pangea/dnd` is the
  documented fallback (Apache-2.0, explicit `"react": "^18.0.0 || ^19.0.0"` peer range) — see
  the Pattern Decisions table in `implementation/plan.md`.
- **A parallel `@dnd-kit/react` package exists** (v0.5.0, a from-scratch rewrite) — explicitly
  not adopted; it is pre-1.0 and still stabilizing. The stable `@dnd-kit/core` +
  `@dnd-kit/sortable` + `@dnd-kit/utilities` line is the production-proven choice for this
  stack today.
- Accessibility is materially cheaper to deliver correctly: `KeyboardSensor` (Space/Enter to
  pick up, arrow keys to move, Space/Enter to drop, Escape to cancel) plus a live-region
  announcements hook ship out of the box, satisfying part of the WCAG posture this decision
  was made under — though this plan still builds a single shared `MoveToMenu` component for
  both touch and keyboard (see the Pattern Decisions table, "Non-drag fallback vs.
  keyboard-drag sensor" row) rather than relying on dnd-kit's own arrow-key mode, to avoid
  maintaining two independent accessible paths for the same capability.
