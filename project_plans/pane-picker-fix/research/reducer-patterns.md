# Reducer Patterns — Move-and-Clear Atomicity in paneReducer

## Current ASSIGN_SESSION Duplicate Guard (the bug)

```ts
// paneReducer.ts:99–104
const allLeaves = getAllLeaves(state.root);
const isDuplicate = allLeaves.some(
  (l) => l.id !== paneId && l.sessionId === sessionId
);
if (isDuplicate) return state;  // ← silent no-op
```

This is a **single-check guard** — it detects a collision but discards the entire
operation instead of resolving it. In Redux/useReducer best practice, the correct
pattern for a "move" is to resolve conflicts within the same reducer case as an
atomic operation.

## Correct Pattern: Atomic Move-and-Clear

The reducer has all the state it needs to handle moves atomically. The fix replaces
the no-op guard with two sequential `replaceNode` calls in a single reducer case:

```ts
case "ASSIGN_SESSION": {
  const { paneId, sessionId } = action;
  const allLeaves = getAllLeaves(state.root);

  // Find and clear the pane that currently holds this session (if any)
  const sourcePaneId = allLeaves.find(
    (l) => l.id !== paneId && l.sessionId === sessionId
  )?.id ?? null;

  const target = findLeaf(state.root, paneId);
  if (!target) return state;

  // Step 1: clear the source pane if there is one
  let newRoot = state.root;
  if (sourcePaneId) {
    const sourceLeaf = findLeaf(newRoot, sourcePaneId)!;
    const cleared: LeafPane = { ...sourceLeaf, sessionId: null };
    newRoot = replaceNode(newRoot, sourcePaneId, cleared);
  }

  // Step 2: assign to the target pane
  if (target.viewKind === "session-list" && !wouldExceedMaxDepth(newRoot, paneId)) {
    // ... existing auto-split logic unchanged ...
  }
  const updated: LeafPane = { ...target, sessionId, activeTab: "terminal" };
  newRoot = replaceNode(newRoot, paneId, updated);
  return { ...state, root: newRoot };
}
```

**Why this is safe in a pure reducer:**
- `replaceNode` is a recursive structural-sharing function that returns new nodes
  only along the path to the changed node (line 119: `if (newFirst === root.first && newSecond === root.second) return root`).
- Calling it twice in sequence is O(depth) each, and the tree is capped at `MAX_DEPTH = 8`,
  so there is no performance concern.
- Both mutations happen before the return, so the state update is fully atomic from
  React's perspective — a single `dispatch` call produces one new state object.

## SPLIT_AND_ASSIGN_SESSION Duplicate Guard (Bug 3)

```ts
// paneReducer.ts:205–208
const allLeaves = getAllLeaves(state.root);
const isDuplicate = allLeaves.some((l) => l.sessionId === sessionId);
if (isDuplicate) return state;
```

Same pattern, same fix. The corrected version:

```ts
// Find the source pane (may be any leaf, including the target being split)
const sourceLeaf = allLeaves.find((l) => l.sessionId === sessionId) ?? null;
const sourcePaneId = sourceLeaf?.id ?? null;

// Clear the source before splitting
let newRoot = state.root;
if (sourcePaneId && sourcePaneId !== paneId) {
  const src = findLeaf(newRoot, sourcePaneId)!;
  newRoot = replaceNode(newRoot, sourcePaneId, { ...src, sessionId: null });
}
// ... then proceed with split creation using newRoot instead of state.root ...
```

Edge case: if `sourcePaneId === paneId` (the session is already in the pane being
split), clearing first and then splitting is fine — the split node's `first` child is
the original target, but its `sessionId` was just cleared. In that case the new leaf
(second) gets the session, and the original pane becomes empty. This is the correct
"open in new split" behavior.

## replaceNode — Structural Sharing Guarantees

```ts
// paneUtils.ts:114–121
export function replaceNode(root: PaneNode, targetId: PaneId, replacement: PaneNode): PaneNode {
  if (root.id === targetId) return replacement;
  if (root.type === "leaf") return root;
  const newFirst = replaceNode(root.first, targetId, replacement);
  const newSecond = replaceNode(root.second, targetId, replacement);
  if (newFirst === root.first && newSecond === root.second) return root;
  return { ...root, first: newFirst, second: newSecond };
}
```

Key properties:
- **Identity-preserving**: returns the same object reference if nothing changed
  (the `if (newFirst === root.first && newSecond === root.second) return root` guard).
- **Pure**: no mutation; returns new objects only for nodes on the path from root to
  the changed node.
- **Safe for chaining**: calling `replaceNode(result, id2, node2)` on the output of
  a first call is correct and efficient.

## getAllLeaves — Snapshot Before Mutation

```ts
// paneUtils.ts:63–66
export function getAllLeaves(root: PaneNode): LeafPane[] {
  if (root.type === "leaf") return [root];
  return [...getAllLeaves(root.first), ...getAllLeaves(root.second)];
}
```

The snapshot is taken from `state.root` before any mutations. After the first
`replaceNode`, the snapshot is stale (still shows the old source leaf). This is fine
because the source lookup only needs to happen once, before mutations begin.

## State Invariant — focusedPaneId Must Remain Valid

The existing state invariant test (`paneReducer_should_neverProduceStateWithMissingFocusedPaneId_When_$type`)
covers this. For ASSIGN_SESSION and SPLIT_AND_ASSIGN_SESSION, the move-and-clear
change does not alter which panes exist in the tree (only their `sessionId` fields),
so `focusedPaneId` remains valid. The split path in `SPLIT_AND_ASSIGN_SESSION` already
correctly sets `focusedPaneId` to the new leaf.

## Existing Patterns in the Codebase

The `swapPanes` function in `paneUtils.ts` (lines 43–53) is the closest existing
example of a multi-step atomic tree mutation:

```ts
export function swapPanes(root: PaneNode, paneId: PaneId, targetPaneId: PaneId): PaneNode {
  const leaf1 = findLeaf(root, paneId);
  const leaf2 = findLeaf(root, targetPaneId);
  // ...
  let result = replaceNode(root, paneId, { ...leaf1, ...c2 });
  result = replaceNode(result, targetPaneId, { ...leaf2, ...c1 });
  return result;
}
```

This is exactly the pattern to follow for move-and-clear: `replaceNode` called twice
on a `let result` variable, threading the output of one call as the input of the next.
