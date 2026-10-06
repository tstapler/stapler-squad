# Research: Stack — kanban-board-view

Agent 1 (Stack). Scope: what's already installed, drag-and-drop library choice, styling
convention, and the exact dependency/version delta needed.

## Current stack (verified from `web-app/package.json`)

| Package | Version |
|---|---|
| `react` / `react-dom` | `^19.0.0` |
| `next` | `15.3.2` |
| `typescript` | `^5.9.3` |
| `react-virtuoso` | `^4.18.7` |
| `@tanstack/react-virtual` | (present, version not pinned in the grep excerpt — imported directly) |

No drag-and-drop library is installed. `grep -riE "dnd|drag"` across `web-app/src` only
matches unrelated code: terminal text-selection (`XtermTerminal*`), pane-resize handles
(`ResizeHandle.tsx`, `PaneSplitRenderer.tsx`, `useResizablePanel.ts`), and mobile terminal
touch gestures (`useMobileTerminalGestures.ts`, `useTerminalGestures.ts`). None of these are
a generic drag-and-drop primitive reusable for kanban cards — this confirms the requirements
doc's Feasibility Risk that a new dependency decision is required.

`SessionList.tsx` (`web-app/src/components/sessions/SessionList.tsx:5-6`) already imports
both virtualization libraries in active use:
```ts
import { useVirtualizer } from "@tanstack/react-virtual";
import { GroupedVirtuoso } from "react-virtuoso";
```
`GroupedVirtuoso` specifically drives the existing card-mode grouped rendering
(`SessionList.tsx:1417`) — this is the component whose windowing model the board's per-column
lists would need to coexist with, or deliberately not use.

`groupSessions()` / `GroupingStrategy` live in `web-app/src/lib/grouping/strategies.ts` and are
already consumed by `SessionList.tsx`, `ReviewQueuePanel.tsx`, and the history hooks
(`useHistoryGrouping.ts`, `useHistoryFilters.ts`) — confirms this logic is reusable as-is for
the board's swimlane axis without modification.

## Drag-and-drop library comparison (2026)

**Recommendation: `@dnd-kit/core` (+ `@dnd-kit/sortable`, `@dnd-kit/utilities`, optionally
`@dnd-kit/modifiers`).**

| Library | Status (2026) | Verdict |
|---|---|---|
| `react-beautiful-dnd` (Atlassian) | Formally deprecated by Atlassian; superseded by their own Pragmatic drag-and-drop. React 19 support is unconfirmed/unlikely (project unmaintained). | **Do not use** — explicitly named as unmaintained in the requirements doc's own risk list; confirmed via search. |
| Native HTML5 Drag Events (`draggable`, `ondragstart`/`ondrop`) | No dependency, but: no built-in accessibility (keyboard drag, screen-reader announcements), inconsistent drag-image/ghost rendering across browsers, and manual reimplementation of collision detection, auto-scroll, and touch fallback — exactly the "rabbit hole" the requirements doc flags for touch. | Only worth it for a deliberately minimal read-only interaction; not recommended given DnD is in-scope. |
| Atlassian Pragmatic drag-and-drop | Actively maintained, good for very large lists (1000s of items) and external/file drag sources. Ships fewer batteries-included (animations, drag handles, drop indicators are DIY). | Reasonable alternative if @dnd-kit's virtualization friction (below) proves worse than expected, but heavier to build against for a first cut. |
| **`@dnd-kit/core`** | Community standard, ~500K weekly downloads on `@dnd-kit/core` alone (the widely-adopted "v1" API — there is also a newer, less-adopted `@dnd-kit/react` v0.5.0 rewrite, but it's early/low-adoption and not the recommended entry point yet). 6KB core, modular (`sortable`, `utilities`, `modifiers` as separate installs), first-class TypeScript, built-in keyboard/screen-reader accessibility, framework-agnostic sensors. | **Recommended.** Verified peer dependency range is open-ended (`"react": ">=16.8.0"`, `"react-dom": ">=16.8.0"` — see below), so it is compatible with React 19 even though the last publish predates React 19's release; no known breakage reported. |

### Verified peer dependencies (`npm registry`, `@dnd-kit/core@6.3.1`)
```json
"peerDependencies": { "react": ">=16.8.0", "react-dom": ">=16.8.0" }
```
Open-ended lower bound, no upper cap — satisfies this repo's `react: ^19.0.0`. Latest published
versions of the companion packages as of this research: `@dnd-kit/sortable@10.0.0`,
`@dnd-kit/utilities@3.2.2`, `@dnd-kit/modifiers@9.0.0`.

## Versions to add to `web-app/package.json`

```json
"@dnd-kit/core": "^6.3.1",
"@dnd-kit/sortable": "^10.0.0",
"@dnd-kit/utilities": "^3.2.2"
```
`@dnd-kit/modifiers` (`^9.0.0`) is optional — only needed if columns want axis-locked or
boundary-restricted drag (e.g. constrain drag to the board's horizontal scroll container).

## Known compatibility issue: virtualization + drag-and-drop

This is the one real technical risk, and it's already flagged in the requirements doc's Rabbit
Holes / Feasibility Risks. Findings:

- `@dnd-kit/sortable` is documented as usable together with virtualized lists
  (`@tanstack/react-virtual` / `react-window`), and real-world examples exist combining
  `@tanstack/react-virtual` with `dnd-kit`.
- The specific friction reported by the `dnd-kit` and `TanStack/virtual` communities: both
  virtualization positioning and drag-drop animation use CSS `transform`, and naively
  combining them causes visual conflicts (dropped/disappearing rows, transform fights) unless
  one is careful about which layer owns the transform.
- No verified reports of `react-virtuoso` (`GroupedVirtuoso` specifically, which is what
  `SessionList.tsx` uses today) combined with `dnd-kit` — this pairing is comparatively
  untested in the wild, more so than `@tanstack/react-virtual` + `dnd-kit`.

**Practical implication for Phase 3 planning:** the safest v1 approach is to *not* virtualize
inside board columns at all — swimlane sub-lists are naturally bounded subsets of the full
session set (a session lands in exactly one of 4 columns), so per-column counts should be far
below whatever threshold made `GroupedVirtuoso` necessary for the flat/grouped list view. Only
add windowing to a column if a specific column is empirically shown to grow large (e.g. a
long-lived "Complete" column with no archival), and if so, prefer `@tanstack/react-virtual`
over `react-virtuoso` for that column since it has more precedent paired with `dnd-kit`. This
should be an explicit sizing decision in Phase 3, not assumed away.

## Styling convention (must follow — `.claude/rules/css-architecture.md`)

- New components use vanilla-extract: colocate `SessionBoard.css.ts` next to
  `SessionBoard.tsx` (per the requirements doc's suggestion of a sibling component rather than
  inlining into the 1601-line `SessionList.tsx`). No new CSS Modules.
- All values come from `vars.*` in `web-app/src/styles/theme.css.ts` /
  `theme-contract.css.ts` (space, radii, fontSize, color tokens) — no hardcoded hex or raw
  `var(--token)` strings in `.css.ts` files.
- Runtime-dynamic values (e.g. a drag-active highlight color, per-card accent) must go through
  the CSS-custom-property bridge pattern (`style={{ "--card-accent": ... }}` +
  `var(--card-accent, ${vars.color.x})` fallback in the `.css.ts` file), not inline layout
  styles — inline `style` for anything cascade-relevant (e.g. flex direction changes when a
  column collapses) is explicitly disallowed; use a `data-*` attribute + `selectors` instead.
- No hardcoded `zIndex` — any new stacking context (e.g. the dragged card's overlay, which
  `@dnd-kit`'s `DragOverlay` renders via portal) must get a named slot added to `zIndex` in
  `theme-contract.css.ts`.
- `@dnd-kit`'s `DragOverlay` is portal-based by default, which satisfies this repo's rule
  against non-portaled `position: fixed`/`absolute` overlays — no custom portal wiring needed
  for the dragged-card overlay specifically.

## Summary of dependency delta

| Action | File |
|---|---|
| Add `@dnd-kit/core`, `@dnd-kit/sortable`, `@dnd-kit/utilities` (and optionally `@dnd-kit/modifiers`) | `web-app/package.json` |
| No changes needed | `react`, `react-dom`, `next`, `react-virtuoso`, `@tanstack/react-virtual` — all already satisfy `@dnd-kit`'s peer range |
| New file (not edit) | `web-app/src/components/sessions/SessionBoard.tsx` + colocated `SessionBoard.css.ts` |
