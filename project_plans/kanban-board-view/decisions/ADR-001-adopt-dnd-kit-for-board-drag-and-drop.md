# ADR-001: Adopt `@dnd-kit` for board drag-and-drop

**Date**: 2026-08-06
**Status**: Accepted
**Project**: kanban-board-view
**Deciders**: SDD Phase 3 planning

## Context

The kanban board view (`project_plans/kanban-board-view/requirements.md`) requires dragging a
session card between columns to trigger a state-change RPC. No drag-and-drop library is
installed today — verified in `web-app/package.json` (Phase 2 `research/build-vs-buy.md` and
`research/stack.md` both grepped for `dnd|drag|kanban|sortable` and found no hits; the one
near-miss, `react-arborist@3.4.3`, is a tree component with tree-node reordering, not a
general DnD primitive).

The stack is React `^19.0.0` / Next `15.3.2`, with `react-virtuoso@^4.18.7` and
`@tanstack/react-virtual@^3.13.25` already in use for list virtualization
(`web-app/src/components/sessions/SessionList.tsx:5-6`).

Hard filters derived from the requirements and `.claude/rules/css-architecture.md`:

1. React 19 peer-dependency compatibility (non-negotiable — the app is on React 19).
2. Built-in keyboard sensor / screen-reader support (WCAG 2.1 SC 2.1.1; the board must not be
   a keyboard regression versus today's fully-operable per-session action buttons).
3. A portal-based drag overlay, or the ability to portal one, because
   `.claude/rules/css-architecture.md` forbids non-portaled `position: fixed`/`absolute`
   overlays (an ancestor `transform`/`filter`/`will-change` silently breaks them).
4. Permissive license, no runtime CSS-in-JS dependency (the CSS rule explicitly bans
   `styled-components`/`emotion`).

## Decision

Adopt **`@dnd-kit/core@^6.3.1` + `@dnd-kit/sortable@^10.0.0` + `@dnd-kit/utilities@^3.2.2`**
(MIT) as `dependencies` in `web-app/package.json`.

`@dnd-kit/modifiers` is **not** adopted — the board does not need axis-locked or
boundary-restricted drag in v1. Add it only if a concrete constraint appears.

`@dnd-kit/sortable` is included for `SortableContext`'s per-column collision/keyboard-ordering
helpers. If, at Story 3.1.2, cross-column drag is implemented entirely with `useDraggable` /
`useDroppable` and `SortableContext` proves unused, **drop the `@dnd-kit/sortable` dependency
in the same PR** rather than shipping an unused package.

## Alternatives considered

| Option | License | React 19 | Verdict | Reason rejected |
|---|---|---|---|---|
| `react-beautiful-dnd@13.1.1` | Apache-2.0 | No (`^18` peer cap) | Rejected | `npm view react-beautiful-dnd deprecated` returns Atlassian's formal deprecation notice; peer range excludes React 19. Fails filter 1. |
| `@hello-pangea/dnd@18.0.1` | Apache-2.0 | Yes | Fallback only | Maintained rbd fork, but inherits rbd's "all draggables must be mounted" architecture — the exact virtualization hostility `requirements.md:55` flags as a risk. Keep as a contingency if `@dnd-kit` integration stalls. |
| Native HTML5 drag events | n/a | n/a | Rejected | No keyboard/screen-reader support and no touch support at all — fails filter 2 outright and forecloses the touch decision rather than deferring it. Also collides with the pane-swap native drag already on `PaneLeafComponent` (`web-app/src/components/pane/PaneSplitRenderer.tsx:243-246`). |
| `react-trello@2.2.11` | MIT | Untested | Rejected | Unpublished since 2022-06-26; pulls in `redux`, `react-redux`, `redux-logger`, and `styled-components` — none present in this codebase, and `styled-components` is explicitly banned by `.claude/rules/css-architecture.md`. Fails filter 4. |
| SaaS / managed API | — | — | Not applicable | Drag-and-drop is client-side interaction state; no hosted service performs it. |

## Consequences

**Positive**
- `KeyboardSensor` and built-in ARIA live announcements mean the accessible move path has a
  library-supported implementation to lean on rather than being hand-rolled.
- `<DragOverlay>` renders through a portal by default, satisfying
  `.claude/rules/css-architecture.md`'s portal requirement for the dragged card without custom
  wiring (this must still be **verified at runtime** in Task 3.1.3b, not assumed).
- Composable hooks (`useDraggable`, `useDroppable`, `DndContext`) rather than a monolithic
  board component — consistent with `.claude/rules/interface-pollution-checklist.md`'s
  preference for small focused pieces over an all-in-one abstraction.
- ~10–15 KB gzipped for core + sortable (community-reported; npm's ~1.07 MB figure is unpacked
  source, not shipped bundle). **UNVERIFIED** — measure against the Next build output in Task
  3.1.1c rather than repeating the community number.

**Negative / accepted risk**
- Last publish 2024-12-05 (`npm view @dnd-kit/core time.modified`). The repo
  (`clauderic/dnd-kit`) is not archived and the API surface is mature, but this is a
  maintenance-cadence risk to re-check at the next dependency audit.
- Column layout and card markup are still hand-built; `@dnd-kit` supplies drag mechanics only.
  Mitigated by adapting `web-app/src/components/backlog/BacklogBoard.tsx`'s existing column
  shell and enter/exit transition pattern rather than starting from a blank file.
- A newer `@dnd-kit/react@0.5.x` rewrite exists but has low adoption; deliberately **not**
  chosen. Revisit only if the v1 API is deprecated upstream.

## Verification

- `web-app/package.json` gains exactly the three packages above (or two, if `sortable` is
  dropped per the Decision section).
- `cd web-app && pnpm install && pnpm run build` succeeds with no peer-dependency warning
  mentioning `react@19`.
- `cd web-app && npx jest --no-coverage --testPathPatterns="SessionBoard"` passes with the
  keyboard-move tests from Story 2.3.2 green.
