# ADR-013: Implement Resize Splitter from Scratch Using Pointer Events

## Status
Accepted

## Context

R1 (Resizable tree panel) requires a drag handle between the file tree and content panes that persists width to localStorage (`filestab.treeWidth`). Two approaches were evaluated:

**Option A — Third-party library** (`react-resizable-panels` or `allotment`):
- `react-resizable-panels` adds ~12 KB gzipped to the bundle
- `allotment` is heavier (~30 KB gzipped) and pulls in a Monaco dependency
- Both libraries impose their own layout model (flex-based panels with percentage sizing), which would require wrapping or replacing the existing `FilesTab` split layout
- The collapse/expand behavior (R1) and mobile single-pane switch (R2) would need custom overrides or escape hatches that fight the library's internal state

**Option B — Extend the existing bespoke splitter** (`ResizeHandle.tsx` + `useListColumnWidth.ts`):
- The project already ships `ResizeHandle.tsx` (79 lines): pointer capture via `setPointerCapture`, rAF-throttled move callbacks, clamped ratio math
- `useListColumnWidth.ts` (55 lines) provides the localStorage persistence pattern with clamping and SSR safety
- Together they implement the full pointer-events drag pattern that `react-resizable-panels` also uses internally
- The file-tree variant needs pixel widths (not ratios) and a collapse state, but these are straightforward extensions of the existing hook

The project philosophy, established in ADR-010 (frontend modularity) and observed throughout the codebase, strongly prefers small focused utilities over heavy dependencies. There is no functionality gap: pointer capture, rAF throttling, localStorage persistence, and min/max clamping are all already present in the codebase.

## Decision

Implement the file-tree resize splitter by extending the existing bespoke components:

1. Extract a new hook `useTreePaneWidth` modeled after `useListColumnWidth`, using the `filestab.treeWidth` localStorage key, a 160 px minimum, a 50% viewport maximum, and a separate `filestab.treeCollapsed` boolean key for the collapse state.
2. Reuse `ResizeHandle.tsx` as-is (or with a minor prop addition for pixel-based callbacks instead of ratio-based) rather than adding a new component.
3. Do not add `react-resizable-panels`, `allotment`, or any other resize library.

## Consequences

- **Bundle size**: no change — zero new dependencies.
- **Collapse/expand** (R1) and **mobile single-pane** (R2) behaviors are implemented directly in `FilesTab` state, keeping them co-located with the layout logic rather than scattered across library adapters.
- **Maintenance**: the splitter surface area (~130 lines total across hook + handle) is owned by the project and trivially auditable.
- **Constraint**: the implementation must handle the `direction="vertical"` case of `ResizeHandle` in pixel mode. The rAF throttle and pointer-capture patterns must be replicated or the handle must emit absolute `clientX` coordinates and let the hook do the pixel arithmetic. Either approach is acceptable; the implementer chooses based on which produces fewer changes to `ResizeHandle.tsx`.
- **Testing**: the new hook is unit-testable with `localStorage` mocks; the handle behavior is covered by existing Playwright pointer-event helpers in `tests/e2e/`.
