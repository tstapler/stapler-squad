# Research: Stack — Drag-and-Drop Library for Kanban Board View

Agent: Agent 1 (Stack) — SDD research phase, project `board-kanban-view`.

## Current stack facts (verified from repo)

- `react`: `^19.0.0`, `react-dom`: `^19.0.0` (`web-app/package.json`)
- `next`: `15.3.2`, `typescript`: `^5.9.3`
- Package manager: `pnpm@10.27.0`
- CSS: vanilla-extract (`@vanilla-extract/recipes ^0.5.7`) per `.claude/rules/css-architecture.md` — any new component must ship a colocated `.css.ts`, use `vars.*` tokens from the theme contract, never `var(--undefined)` strings.
- State: `@reduxjs/toolkit ^2.11.2` + `react-redux ^9.2.0` are present but session-list local UI state (sort, view mode, column widths) is handled with **component-local state + `useState` initializer reading `localStorage`**, not Redux — see `web-app/src/components/sessions/SessionList.tsx:240-265` (`loadFromStorage`/`saveToStorage` helpers, wrapped in try/catch, keyed via a `storageKeyPrefix` prop so multiple list instances — e.g. split view — don't collide). A new "board vs list, per workspace" persistence toggle should follow this exact pattern, not introduce a new mechanism or a Redux slice.
- **Naming collision to avoid**: `SessionList` already has a prop called `viewMode?: "card" | "row"` (display density), unrelated to the new List/Board *page-level* view toggle from the requirements. Do not reuse the name `viewMode` for the new toggle — use something like `sessionViewMode: "list" | "board"` or similar to avoid confusion with the existing prop.
- No existing drag-and-drop library. `grep -rliE 'drag|dnd' src` turns up only terminal/gesture/resize-handle code (`useTerminalGestures.ts`, `useMobileTerminalGestures.ts`, `ResizeHandle.tsx`, `useResizablePanel.ts`) — pointer-drag-for-resize, not a DnD list/kanban library. None of it is reusable for card-to-column drag.
- `react-arborist ^3.4.3` is present (used in `FileTree.tsx` for the file-tree view) and has its own internal drag-and-drop for tree reordering, but it's tree-specific (single-column, parent/child reparenting) and not applicable to multi-column kanban — do not try to repurpose it.

## SessionStatus enum (backend-authoritative, relevant to column mapping)

`web-app/src/gen/session/v1/types_pb.ts:3414-3494` (generated from proto, mirrors `session.Status` in Go):

```
UNSPECIFIED = 0
ACTIVE = 1        (replaces legacy RUNNING=1, READY=2)
PAUSED = 4
CREATING = 6
STOPPED = 7        (terminal)
HIBERNATED = 8
RESTORING = 9      (transient, never persisted)
```
`NEEDS_APPROVAL = 5` is explicitly deprecated — the comment states "NeedsApproval is now a sub-status," meaning "Needs Review" for the board is very likely a **derived** condition (active + pending approval), not a raw enum value — confirms the Open Question in requirements.md. This is a planning-phase concern, flagged here only because it affects what a drag-triggered mutation needs to call (a status-changing RPC vs. an approval-decision RPC) — not resolved by this research task.

## Drag-and-drop library landscape (verified via WebSearch, August 2026)

**react-beautiful-dnd is dead.** The GitHub repo was archived April 30, 2025 (read-only), and it's deprecated on npm. Do not add it as a new dependency under any circumstance.

Three live candidates:

| Library | Status (2026) | Fit for this task |
|---|---|---|
| **`@dnd-kit/core` + `@dnd-kit/sortable`** | Actively maintained, community-standard default for React DnD in 2026 | **Recommended** |
| `@hello-pangea/dnd` | Maintained fork of react-beautiful-dnd, keeps its API surface | Viable but the API is a snapshot of the now-frozen rbd design; less flexible for swimlanes/grouping-strategy-as-axis (Goal 4 in requirements.md) than dnd-kit's lower-level primitives |
| `@atlaskit/pragmatic-drag-and-drop` | Atlassian's newer, lower-level, framework-agnostic library — the one Atlassian itself now points to for new work | Better raw performance at Jira/Trello scale (1000+ cards) and has first-class file-drop support, but ships **without built-in accessibility** — keyboard nav/ARIA announcements must be hand-built via separate composable utilities. Given this project's existing accessibility bar (WCAG AA Axe Core gate in CI per root `CLAUDE.md`'s E2E section) and modest realistic card counts (tens to low hundreds of sessions per workspace, not thousands), the extra accessibility work isn't justified by a performance need this app doesn't have. |

### Recommendation: `@dnd-kit/core` + `@dnd-kit/sortable`

Reasons:
1. **Accessibility out of the box** — `KeyboardSensor` gives Space-to-lift/arrow-keys-to-move/Space-to-drop/Escape-to-cancel with ARIA live-region announcements with zero custom code. This directly satisfies requirements.md's acceptance criterion 10 ("touch targets and drag interaction must have a viable non-drag fallback") more cheaply than Pragmatic DnD, and dovetails with this repo's CI-enforced Axe Core WCAG AA gate on `web-app/src/` PRs.
2. **React 19 support confirmed** — `@dnd-kit/core`'s published peer dependency is `"react": ">=16.8.0", "react-dom": ">=16.8.0"` (verified via `npm view @dnd-kit/core peerDependencies`, no upper bound), and React 19 was explicitly added to the supported peer-dep list upstream; StrictMode lockup regressions have been fixed in the current 6.x line. No known open React-19-specific blocker.
3. **Framework-agnostic core with a small footprint** — `@dnd-kit/core` is ~18.9 kB gzipped (Bundlephobia); `@dnd-kit/sortable` and `@dnd-kit/utilities` are thin layers on top. This is well inside the project's `size-limit` budgets already configured in `web-app/package.json` (`"Total JS bundle"` 5 MB, `"Main app chunk"` 400 KB, both ungzipped) — a kanban board is a lazy-loaded/code-split route-level addition, not global framework weight.
4. **Composability for the swimlane requirement** — dnd-kit's primitives (`DndContext`, `useDraggable`, `useDroppable`, `SortableContext`) are unopinionated about layout, which is what's needed to satisfy Goal 4 (status columns *or* `GroupingStrategy`-based columns as swimlanes) without forking a rbd-shaped, list-specific API.
5. Do **not** reach for the newer `@dnd-kit/react` package (a from-scratch v2 rewrite living at `dndkit.com/react`, currently `0.5.0` per npm) — it is pre-1.0 and still stabilizing; the stable, production-proven line for this stack today is `@dnd-kit/core@^6.3.1` + `@dnd-kit/sortable@^10.0.0` + `@dnd-kit/utilities@^3.2.2` (current published versions, verified via `npm view`).

### New dependencies to add (planning-phase decision, surfaced here for the plan)

```json
"@dnd-kit/core": "^6.3.1",
"@dnd-kit/sortable": "^10.0.0",
"@dnd-kit/utilities": "^3.2.2"
```

This crosses "ladder rung 4/6" (per requirements.md's Open Questions) — i.e. it is a new runtime dependency, not a hand-rolled HTML5 DnD implementation. Given react-beautiful-dnd's death and the accessibility/keyboard requirement already baked into acceptance criterion 10, hand-rolling native HTML5 drag events would mean re-implementing keyboard-drag accessibility from scratch (HTML5 DnD has no native keyboard equivalent) — not recommended as a way to avoid the dependency.

## Non-drag mobile fallback (acceptance criterion 10)

dnd-kit's `PointerSensor`/`TouchSensor` still requires either careful `activationConstraint` tuning or, more reliably per this project's own standing mobile/desktop UX requirement (`feedback_mobile_desktop_ux` memory), a **non-drag fallback control** — e.g. a per-card "Move to…" menu (could reuse the existing `SessionActionsOverflow.tsx` overflow-menu pattern already used elsewhere in `web-app/src/components/sessions/`) that calls the same status-mutation path as a completed drag. This is a planning-phase UI decision; flagged here because it changes the dnd-kit integration shape (the drop handler and the menu action must converge on one shared "attempt column move" function rather than duplicating mutation logic).

## Summary for planning phase

- Add `@dnd-kit/core`, `@dnd-kit/sortable`, `@dnd-kit/utilities` as new `dependencies` in `web-app/package.json`.
- New board components should follow existing conventions: colocated `ComponentName.css.ts` (vanilla-extract, tokens from theme contract), and reuse the `localStorage`-with-try/catch persistence pattern from `SessionList.tsx` for the list/board toggle — do not add a Redux slice for this.
- Avoid the prop name `viewMode` for the new list/board toggle (already used for card/row density in `SessionList.tsx`).
- Do not consider `react-beautiful-dnd` (archived), and prefer `@dnd-kit/*` over `@atlaskit/pragmatic-drag-and-drop` unless a future performance need at 1000+ card scale materializes (not expected for this app's session counts).
