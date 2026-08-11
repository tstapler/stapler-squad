# Pitfalls & Edge Cases: Better File Tree UX

**Date**: 2026-05-15
**Researcher**: PITFALLS agent
**Source files reviewed**: `FilesTab.tsx`, `FileTree.tsx`, `FilesTab.css.ts`, `FileTree.css.ts`

---

## P1 — Resize drag: pointer leaves document (mouse-up lost)

**Failure mode**: If the user drags the resize handle and moves the pointer outside the browser window (e.g., to another monitor, or the OS taskbar), the `mouseup` event fires on the OS, not the document. The drag handler never receives it. Result: the pane remains stuck in resize-drag mode indefinitely — every subsequent `mousemove` continues resizing even after the user has released the button.

**Root cause**: `mousemove` / `mouseup` listeners added to `window` or `document` still miss OS-level releases when the pointer leaves the browser's hit-testing region.

**Fix**: Use the Pointer Events API, which has built-in capture semantics:
1. On `pointerdown` on the drag handle, call `handle.setPointerCapture(e.pointerId)`.
2. Listen for `pointermove` and `pointerup` on the *handle element itself* (not `document`).
3. Pointer capture guarantees that `pointerup` fires on the capturing element even when the pointer leaves the window.
4. In the `pointerup` handler, call `handle.releasePointerCapture(e.pointerId)` and clear all drag state.

**Fallback guard** (belt-and-suspenders): Also handle `pointercancel` (fires when the browser cancels the gesture, e.g., on touch, or when the window loses focus mid-drag) — treat it identically to `pointerup`.

**State to clear on drag end**: `isDragging` flag, any `document`-level `pointermove` listeners, and the CSS `cursor: col-resize` override on `<body>` (set it on drag start so the cursor doesn't flicker as the pointer moves off the handle).

---

## P2 — Middle truncation: CSS cannot do it; JS cost on every node

**Failure mode**: `text-overflow: ellipsis` (currently in `FileTree.css.ts` `.name`) truncates only from the right end. For file names like `very-long-component-name.tsx`, the extension `.tsx` is hidden. The requirement (R3 / AC-4) demands `very-long-comp…name.tsx` — i.e., the middle is removed so both the leading characters and the extension are visible.

**CSS cannot solve this**: There is no CSS property for mid-string truncation. All CSS solutions (direction RTL tricks, two-element overlays) break with file extensions or require fixed-width fonts.

**JS approach**: A `truncateMiddle(name: string, maxChars: number): string` utility computes the final display string. This must be called at render time per node.

**Performance concern**: The tree renders 28 px rows via react-arborist's virtualised list (react-window). Only visible rows render at any time (typically 20–50 rows in a 600 px container). The JS truncation function runs only for visible rows, so the cost is O(visible rows), not O(total files). This is negligible.

**However**, the `maxChars` input must come from the available pixel width of the name span, not a hardcoded constant. The pane is now resizable (R1), so the available width changes. Two approaches:

- **Approach A (CSS custom property bridge)**: Measure the pane width via `ResizeObserver` (already present in `FileTree` as `dims.w`). Pass it as a prop or derive `maxChars = Math.floor((dims.w - indentPx - iconPx - badgesPx) / charWidth)`. Run `truncateMiddle` in the node renderer using this computed value. Pro: clean. Con: `charWidth` varies by font; the monospace `vars.font.mono` makes this reliable.

- **Approach B (measure DOM node)**: Use a hidden `<span>` with `visibility:hidden` to measure actual character width once on mount. Store in a ref. Use that for all truncation calculations.

**Recommendation**: Approach A using `dims.w` (already tracked) with a fixed char-width estimate (~7.5 px at 13 px monospace). Do not re-measure on every node render — compute `maxChars` once per `dims` change via `useMemo`, then pass it down to `NodeRenderer` as a stable prop.

**Tooltip**: The `title` attribute on the name span (or the node div) provides the full path with zero additional cost.

---

## P3 — Auto-reveal with lazy tree: race condition during sequential async loads

**Failure mode**: Auto-reveal (R5 / AC-6) must expand ancestor directories in order. If `src/components/sessions/FilesTab.tsx` is the target and only the root is loaded, the sequence is:
1. Load `src/` → fetch `src/`
2. Load `src/components/` → fetch `src/components/`
3. Load `src/components/sessions/` → fetch
4. Scroll `FilesTab.tsx` into view

Each step is an async RPC (`fetchDirectoryFiles`). If the user clicks a *different* file while step 2 is in-flight, two scenarios collide:
- The new file's reveal sequence starts (potentially branching at a different ancestor)
- The old reveal sequence completes, scrolling the wrong file into view

**Additional race**: The current `loadDirectory` guard (`if (loadingPaths.has(dirPath)) return`) prevents duplicate in-flight requests, but a second reveal call that shares a prefix will skip loading already-in-flight ancestors and get confused about which nodes are ready.

**Fix — cancellation token pattern**:
- Store a `revealAbortRef = useRef<AbortController | null>(null)` in `FilesTab`.
- On every new reveal trigger (new `selectedPath` from VCS cross-link or Quick Open), abort the previous controller and create a new one.
- The reveal async function checks `controller.signal.aborted` after each `await fetchDirectoryFiles(...)` and early-exits if cancelled.
- This ensures only the most recent reveal completes to the scroll step.

**Additional edge case — path not in worktree**: If the target path is outside the worktree root or does not exist, the sequential load will eventually 404. The reveal function must gracefully handle fetch errors at each ancestor level (log + give up, do not surface an error to the user since the file state may simply be stale).

**Already-open ancestor directories**: Before fetching, check `dirContents.has(ancestorPath)`. If the dir is already loaded, skip the fetch and just call `treeRef.current?.open(ancestorPath)`. This keeps the happy path (shallow paths) synchronous after the initial load.

---

## P4 — Recently-opened list: deduplication is required

**Failure mode**: If the same file is opened twice (e.g., user revisits `README.md`), a naive push creates two entries. With 8 slots this wastes capacity and looks wrong (same file listed twice).

**Correct behavior**: Deduplicate — remove any existing entry for the same path, then prepend the new entry. This is the "move-to-top" (MRU) semantic used by VS Code, JetBrains, and every other editor.

**Implementation**:
```ts
function addToRecent(prev: string[], path: string): string[] {
  const deduped = prev.filter((p) => p !== path);
  return [path, ...deduped].slice(0, 8);
}
```

**Memory**: 8 × ~100-char strings ≈ 800 bytes. Trivially cheap. No concern.

**In-memory scope**: The requirement says "not persisted". Store in `useState` in `FilesTab`. Do not use `localStorage` or `sessionStorage`. If the user reloads, the list resets — this matches the requirement.

**Display format**: Each entry needs basename + parent dir name. Derive these from the path string directly (`path.split("/").at(-1)` for basename; `path.split("/").at(-2) ?? "."` for parent). No extra data structure needed.

**Empty state**: Hide the entire "Recent" section when the array is empty (requirement: "hidden when no files have been opened yet"). Conditional render on `recentPaths.length > 0`.

---

## P5 — Quick-open + search: debounce, RPC limit, and state conflicts

### 5a. RPC latency and the 500-result cap

`SearchFiles` returns up to 500 results (already reflected in the existing `searchTruncated` UI). For keystroke-by-keystroke filtering this is the server-side cap — the client receives the full 500 and renders them. The existing 300 ms debounce in `FileTree` is adequate for inline search. Quick-open (Ctrl+P) is a separate palette with its own input; it should apply the same 300 ms debounce to avoid flooding the backend.

**Key risk**: If the backend is slow (large repo, cold cache), a 300 ms debounce still queues multiple RPCs. The existing `searchRequestIdRef` pattern (increment on each request, discard responses whose ID doesn't match current) already handles this correctly — reuse it in the quick-open palette.

### 5b. Interaction between quick-open and the tree's own search state

The existing `searchTerm` state lives in `FilesTab` and is fed into `FileTree`. The quick-open palette is a separate overlay — it must NOT share `searchTerm` with the inline search. If it did:
- Opening Ctrl+P would clear any in-progress inline search.
- Closing the palette without selecting would leave the tree in search mode.

**Fix**: Keep quick-open state entirely local to the `QuickOpenPalette` component. It maintains its own query string, its own debounced RPC call, and its own results list. It has no effect on `FilesTab.searchTerm`.

### 5c. Closing the palette without selecting

If the user presses Escape or clicks outside, the palette closes and the previously selected file (if any) remains open. The tree scroll position must not change. Since the palette is an overlay, the tree underneath is untouched — this is naturally correct as long as the palette does not mutate tree state on mount/unmount.

### 5d. Keyboard focus trap

While the palette is open, Tab / Shift+Tab must not escape to the underlying tree. Implement a simple focus trap: listen for `keydown` on the palette container and prevent Tab from blurring the input. Arrow keys navigate the results list; Enter selects; Escape closes.

---

## P6 — Mobile layout: orientation change snap-back

**Failure mode**: User is on a 375 px viewport (mobile portrait). They select a file → content view slides in. They rotate to landscape (e.g., 667 px wide — above the 768 px breakpoint? No, 667 < 768 but some tablets at landscape are 1024 px). The question is: should the layout snap back to split-pane?

**Breakpoint analysis**: R2 says "viewports < 768 px" use single-pane. If the user rotates to a width ≥ 768 px, the layout should switch to split-pane. If they are in "content only" (file open, tree hidden), the tree should re-appear — the constraint is that on desktop the tree is always visible.

**Implementation risk**: React state for `mobileContentVisible` (true = show content) persists across re-renders. If the viewport becomes ≥ 768 px while `mobileContentVisible = true`, the tree would still be hidden (wrong). The fix: in the desktop layout branch, always render both panes regardless of `mobileContentVisible`. A media query in CSS or a `useMediaQuery(768)` hook drives which layout branch renders.

**Recommended pattern**:
```tsx
const isMobile = useMediaQuery("(max-width: 767px)");
if (!isMobile) {
  // Render split layout; mobileContentVisible is irrelevant
} else {
  // Render single-pane; respect mobileContentVisible
}
```

**Orientation change event**: `window.matchMedia` listeners fire on resize, so `useMediaQuery` built on `matchMedia` reacts to orientation changes automatically. No need for `orientationchange` event.

**Back button on desktop**: If the user was in content-only mode on mobile, then resizes to desktop, the back button in the content header must not appear. Conditionally render the back button only when `isMobile` is true.

**Snap behaviour on file select (mobile)**: When a file is selected on mobile, set `mobileContentVisible = true`. On desktop this state change is a no-op (the desktop branch ignores it). This is safe.

---

## P7 — Scroll position preservation: ref pattern for tree wrapper

**Failure mode**: React re-renders triggered by `selectedPath` changing cause react-arborist (which uses react-window internally) to re-render the virtualised list. If the scroll position is not explicitly preserved, react-window may reset to the top or jump unpredictably.

**Current state**: `FileTree` passes `width={dims.w}` and `height={dims.h}` to `<Tree>`. react-arborist/react-window manages its own internal `scrollTop`. On data change, react-window does NOT reset scroll by default — but on `width` or `height` prop changes it may re-measure and reset.

**Key risk**: `dims` updates from the `ResizeObserver`. If the user drags the resize handle, `dims.w` changes frequently. Each change causes react-window to reset internal scroll state. This means dragging the pane width currently resets the tree scroll position.

**Fix for resize-induced scroll reset**:
- Capture `scrollTop` before `dims` changes via a ref attached to the scrollable element inside react-arborist.
- react-arborist exposes `treeRef.current.scrollTo(offset)` — call it after the width update settles (inside the ResizeObserver's `requestAnimationFrame` callback, which already exists in the code).

**Fix for file-open-induced scroll reset**:
- The tree scroll is managed by react-window, not the `treeWrapper` div. Do not try to save/restore the wrapper's `scrollTop` directly.
- Instead, react-arborist's virtualised list should preserve scroll across `selectedPath` changes automatically because `selectedPath` is not a prop of `<Tree>` — it's only used in `NodeRenderer` for styling. The `Tree` component itself does not re-mount.
- **R4 requirement (AC-5)**: "scroll only into view when the selected row is not already visible". Use `treeRef.current?.scrollTo` with the `scrollIntoView`-equivalent API only during auto-reveal (P3), not on every `selectedPath` change. When the user clicks a node in the tree, the node is already visible by definition — no scroll needed.

**Implementing "scroll into view only if not visible"**:
react-arborist does not have a built-in "scroll only if not visible" API. The workaround:
1. After updating `selectedPath`, check if the node's rendered DOM element is within the viewport of the tree container (compare `node.getBoundingClientRect()` with `container.getBoundingClientRect()`).
2. Only call `treeRef.current?.scrollTo(nodeIndex * rowHeight)` if the node is outside the visible range.
3. Alternatively, use `node.element?.scrollIntoView({ block: "nearest" })` — `block: "nearest"` is a native behavior that no-ops when the element is already in view.

---

## Cross-cutting pitfall: `loadDirectory` captured deps include `loadingPaths` (Set)

**Existing bug risk** (not new, but impacts R5 auto-reveal): `loadDirectory` in `FileTree.tsx` lists `loadingPaths` as a dependency (line 420). `loadingPaths` is a `Set` stored in state. Every state update creates a new `Set` reference, which invalidates the `useCallback` memoisation and creates a new `loadDirectory` function. This cascades: `handleToggle` depends on `loadDirectory`, so it also rememoises. For the auto-reveal sequential-load chain (P3), this means the reveal function may hold a stale closure over `loadDirectory`. Ensure the reveal function either (a) calls `loadDirectory` via a ref, or (b) uses a stable version that reads `loadingPaths` from a ref rather than closed-over state.

---

## Summary table

| Pitfall | Severity | Blocking for shipping? |
|---------|----------|----------------------|
| P1: Drag stuck on pointer leave window | High | Yes — drag is core to R1 |
| P2: Middle truncation requires JS, not CSS | Medium | Yes — requirement is explicit |
| P3: Auto-reveal race condition | High | Yes — R5/AC-6 requires correct reveal |
| P4: Recent list deduplication | Low | Behavioural correctness |
| P5: Quick-open state isolation | Medium | Yes — conflict with tree search |
| P6: Mobile orientation snap-back | Medium | Required for R2 correctness |
| P7: Scroll reset on resize | Medium | Required for R4/AC-5 |
| Cross-cutting: `loadingPaths` dep | Medium | Risk to P3 and P1 |
