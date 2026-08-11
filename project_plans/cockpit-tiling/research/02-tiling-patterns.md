# Agent 2: React Tiling Patterns

## Existing Dependencies

No split/tile/pane/panel/mosaic library is present in `web-app/package.json`. The project has no `react-resizable`, `allotment`, `re-resizable`, `react-mosaic`, or `split.js`. A zero-dependency implementation is fully warranted and aligns with the requirements constraint.

## Binary Split Tree Data Model

The entire pane layout is represented as a recursive binary tree:

```typescript
type PaneId = string; // nanoid(8) or crypto.randomUUID()

interface LeafPane {
  type: "leaf";
  id: PaneId;
  sessionId: string | null;  // null = empty slot
  activeTab: SessionDetailTab;
}

interface SplitPane {
  type: "split";
  id: PaneId;
  direction: "horizontal" | "vertical";
  // "horizontal" = split creates top/bottom children
  // "vertical"   = split creates left/right children
  ratio: number;  // [0.0, 1.0] — fraction of space given to `first`
  first: PaneTree;
  second: PaneTree;
}

type PaneTree = LeafPane | SplitPane;

interface PaneState {
  root: PaneTree;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;  // Ctrl+Z zoom: null = normal
}
```

## useReducer Actions

```typescript
type PaneAction =
  | { type: "SPLIT_PANE"; paneId: PaneId; direction: "horizontal" | "vertical" }
  | { type: "CLOSE_PANE"; paneId: PaneId }
  | { type: "RESIZE_PANE"; paneId: PaneId; ratio: number }
  | { type: "FOCUS_PANE"; paneId: PaneId }
  | { type: "ASSIGN_SESSION"; paneId: PaneId; sessionId: string; tab?: SessionDetailTab }
  | { type: "ASSIGN_TAB"; paneId: PaneId; tab: SessionDetailTab }
  | { type: "ZOOM_PANE"; paneId: PaneId | null }
  | { type: "RESET_LAYOUT" }
  | { type: "RESTORE_LAYOUT"; state: PaneState }
  | { type: "NUDGE_RESIZE"; paneId: PaneId; direction: "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown"; amountPx: number; containerSizePx: number };
```

### Reducer logic outline (~150 lines)

`SPLIT_PANE`: Find the target leaf by `paneId`, replace it with a `SplitPane` whose `first` is the original leaf and `second` is a new empty `LeafPane`. Focus the new pane.

`CLOSE_PANE`: Find the `SplitPane` whose child matches `paneId`. Replace the `SplitPane` with its other child. If closing the root leaf, reset to initial state.

`RESIZE_PANE`: Find the `SplitPane` whose `id === paneId` (the split node, not a leaf), update `ratio` clamped to `[minRatio, 1-minRatio]`.

`FOCUS_PANE`: Update `focusedPaneId`.

`ASSIGN_SESSION`: Find leaf by `paneId`, set `sessionId` and `activeTab`.

`ZOOM_PANE`: Toggle `zoomedPaneId`. When set, the renderer shows only that leaf; other panes hide.

`NUDGE_RESIZE`: Find the nearest ancestor `SplitPane` that contains `focusedPaneId`. Compute `deltaPx / containerSizePx` and add/subtract to `ratio` (direction matters: ArrowRight/ArrowDown increases ratio if focused pane is `first`).

`RESET_LAYOUT`: Return initial state (single LeafPane, no session, focus on root).

`RESTORE_LAYOUT`: Deserialize from localStorage; validate sessionIds against live sessions (nullify missing ones).

## Rendering the Tree

```typescript
// PaneSplit.tsx — recursive renderer
function PaneNode({ node, totalW, totalH }: { node: PaneTree; totalW: number; totalH: number }) {
  if (node.type === "leaf") {
    return <PaneLeaf pane={node} />;
  }
  
  // SplitPane: use CSS grid with fractional columns or rows
  const isVertical = node.direction === "vertical"; // left|right
  return (
    <div
      className={splitContainer({ direction: node.direction })}
      style={{
        "--split-ratio": node.ratio,
      } as React.CSSProperties}
    >
      <PaneNode node={node.first} ... />
      <ResizeHandle splitId={node.id} direction={node.direction} />
      <PaneNode node={node.second} ... />
    </div>
  );
}
```

In the vanilla-extract `.css.ts` (since `node.ratio` is runtime):

```typescript
// paneSplit.css.ts
export const splitContainer = recipe({
  base: {
    display: "grid",
    width: "100%",
    height: "100%",
    overflow: "hidden",
  },
  variants: {
    direction: {
      vertical: {
        // left | handle | right
        // gridTemplateColumns driven by CSS custom property set inline
        gridTemplateColumns: "calc(var(--split-ratio) * 100%) 6px 1fr",
        gridTemplateRows: "100%",
      },
      horizontal: {
        // top | handle | bottom
        gridTemplateColumns: "100%",
        gridTemplateRows: "calc(var(--split-ratio) * 100%) 6px 1fr",
      },
    },
  },
});
```

This uses the CSS custom property bridge: `--split-ratio` is set as an inline style at runtime, then referenced in the build-time vanilla-extract class. This is the documented vanilla-extract pattern for dynamic values.

## localStorage Serialization Format

Key: `cockpit.paneLayout`

```typescript
interface PersistedPaneLayout {
  version: 1;
  root: PaneTree;          // full tree (recursively serializable — no functions)
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;
}
```

`PaneTree` is a plain JSON-serializable object (no DOM refs, no functions). `SessionDetailTab` is a string union. Serialization is `JSON.stringify(state)` and deserialization is `JSON.parse(stored)` with a validation pass:

```typescript
function validateAndRepair(tree: PaneTree, validSessionIds: Set<string>): PaneTree {
  if (tree.type === "leaf") {
    return {
      ...tree,
      sessionId: tree.sessionId && validSessionIds.has(tree.sessionId)
        ? tree.sessionId
        : null,
    };
  }
  return {
    ...tree,
    first: validateAndRepair(tree.first, validSessionIds),
    second: validateAndRepair(tree.second, validSessionIds),
  };
}
```

On load, call with the set of currently-known session IDs. Any stale session IDs become `null` (empty pane slot).

## Summary: What Is Needed vs. External Libraries

The full implementation requires approximately:
- `usePaneReducer.ts` — reducer + initial state + action creators (~150 lines)
- `PaneSplitRenderer.tsx` — recursive renderer + `PaneLeaf` + `PaneHeader` (~80 lines)
- `ResizeHandle.tsx` — pointer events handle (~60 lines, see Agent 3)
- `paneSplit.css.ts` — vanilla-extract styles (~60 lines)
- `usePaneLayout.ts` — localStorage persistence hook (~30 lines)

Total: ~380 lines. No external library needed.
