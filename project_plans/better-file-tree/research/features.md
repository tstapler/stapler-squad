# Features Research: Better File Tree UX

**Date**: 2026-05-15
**Scope**: UX patterns for R3 (middle truncation), R4 (scroll preservation), R5 (auto-reveal), R6 (recently opened), R7 (quick-open palette)

---

## 1. Middle Truncation of File Names (R3)

### How Best-in-Class Tools Handle It

**VS Code (Explorer panel)** truncates file names with a trailing ellipsis at the right edge — it does _not_ do middle truncation by default. Long names in the file tree are clipped via `text-overflow: ellipsis; overflow: hidden; white-space: nowrap`. The full path appears in a tooltip. The absence of middle truncation is a known pain point with multiple open GitHub issues (e.g., [microsoft/vscode#248503](https://github.com/microsoft/vscode/issues/248503) requesting horizontal scroll; [microsoft/vscode#111411](https://github.com/microsoft/vscode/issues/111411) requesting name wrapping).

**GitHub file browser** also uses trailing ellipsis (pure CSS) — it prioritizes the beginning of the name and loses the extension.

**JetBrains IDEs** offer horizontal scroll in the file tree rather than truncation, preserving full names at the cost of requiring scroll.

### Middle Truncation: The Right Approach for R3

Pure CSS cannot do middle truncation; the spec has a draft for `text-overflow: [start] [end]` but it is not yet standard (W3C issue [csswg-drafts#3937](https://github.com/w3c/csswg-drafts/issues/3937)).

**Two viable JS approaches:**

**Option A — Fixed character counts (simple, our choice):**
```tsx
function middleTruncate(name: string, maxLen = 28): string {
  if (name.length <= maxLen) return name;
  const ext = name.lastIndexOf(".");
  const suffix = ext > 0 ? name.slice(ext) : ""; // ".tsx", ".go", etc.
  const keep = maxLen - suffix.length - 1; // 1 for "…"
  const head = Math.ceil(keep * 0.6);
  const tail = keep - head;
  return name.slice(0, head) + "…" + name.slice(name.length - tail - suffix.length, name.length - suffix.length) + suffix;
}
// "very-long-component-name.tsx" → "very-long-comp…name.tsx"
```
This matches AC-4 exactly. The `maxLen` should be driven by measured container width — or picked as a fixed conservative value since the tree pane is a fixed min/max range.

**Option B — Canvas measurement (accurate, heavier):**
The `react-middle-truncate` npm package ([matt-d-rat/react-middle-truncate](https://github.com/matt-d-rat/react-middle-truncate)) renders candidate strings to a Canvas context to measure pixel width against the container. It uses ResizeObserver to re-measure on container resize. Accurate but adds ~3 KB and per-render Canvas operations.

**Recommendation for this project:** Use Option A (JS string manipulation in the `NodeRenderer`). Since the tree pane has a bounded width (160–50% viewport), a `maxLen` of ~30 characters with the extension-preserving formula gives the right result. The full path goes in `title={data.id}` (full relative path). No external library needed. The existing `nameClass` CSS style already has `overflow: hidden; text-overflow: ellipsis; white-space: nowrap` — keep it as the fallback for edge cases but the text content itself will be pre-truncated via the JS helper.

**The `name` span currently renders `data.name` (just the basename) with trailing-ellipsis CSS.** Replace the span content with `middleTruncate(data.name)` and add `title={data.id}` (full path) to the outer `div` or the `nameClass` span. This satisfies AC-4.

---

## 2. Recently Opened Files Panel (R6)

### How VS Code and JetBrains Handle It

**VS Code `Ctrl+Tab`** (Editor Group MRU): In-session in-memory list. Default tracks up to 100 recent files (`files.recentFiles.maxItems`). The persisted "File > Open Recent" list is workspace-scoped and survives restarts. Session-scoped cycling (Ctrl+Tab) holds all files opened since app launch.

**JetBrains `Ctrl+E`** (Recent Files popup): Session-scoped list stored per-project; persists when the IDE closes. The popup shows files in MRU order, allows typing to filter within the popup, and supports pressing `Ctrl+E` again to filter to only edited files. No explicit max count documented in public docs — empirically around 50 items.

### Design for R6

**In-memory session list (not persisted):** Correct per requirements. Use a `useRef`-based or `useState`-based array in `FilesTab`. On each `handleFileSelect(path)` call:
1. Remove the path from the list if it exists (dedup).
2. Prepend it to the front (most-recent first).
3. Trim to 8 entries (per R6).

```ts
// In FilesTab — example hook
function useRecentFiles(maxCount = 8) {
  const [recent, setRecent] = useState<string[]>([]);
  const add = useCallback((path: string) => {
    setRecent(prev => [path, ...prev.filter(p => p !== path)].slice(0, maxCount));
  }, [maxCount]);
  return { recent, add };
}
```

**Display in `RecentFilesPanel`:** Show above the directory tree inside `treeWrapper`. Each entry shows:
- File icon (reuse `getFileIcon(basename)`)
- Basename (`path.split("/").pop()`)
- Parent dir name (`path.split("/").at(-2) ?? ""`)
- Full path in `title` attribute for tooltip

**Click handler:** Calls `onFileSelect(path)` — same as tree activation. The panel is hidden when `recent.length === 0`.

**Max count = 8** aligns with typical IDE "Recent Files" conventions (VS Code default Ctrl+Tab list length visible at once is ~8-10; JetBrains shows a similar window before scroll).

---

## 3. Quick-Open Palette, Ctrl+P (R7)

### VS Code's Quick Open Behavior (Reference)

1. `Ctrl+P` opens an overlay input (modal-like, not inline). Focus immediately goes to the input.
2. **Empty state:** shows recently opened files (MRU order).
3. **As user types:** switches to scored results. VS Code uses a fuzzy/subsequence scorer — characters must appear in order but need not be consecutive. Ranking: recency is the tiebreaker when two results score equally.
4. **Keyboard nav:** `↑`/`↓` moves highlight; `Enter` opens; `Escape` closes and returns focus to prior element. `→` (right arrow) opens the file in background and keeps the palette open for multi-select.
5. **Path shown:** each result shows the basename prominently and the directory path dimmer/smaller below or to the right.
6. **No "loading" state** — VS Code pre-indexes the file list. For our case, we make a backend `SearchFiles` RPC call.

### Interaction Pattern for R7

The quick-open palette reuses the existing `SearchFiles` RPC already wired into `FileTree`. The palette is a floating overlay, not embedded in the tree.

**Component structure:**
```
QuickOpenPalette (modal overlay, Radix Dialog or custom)
  ├── <input autofocus placeholder="Search files…" />
  ├── Empty state: show recent[] list (from R6)
  └── Results list (when query.length >= 1)
       └── ResultItem × N
             basename (bold)
             parent/dir/path (muted, smaller)
```

**Ranking strategy:** The project already has Fuse.js (`fuse.js@7.3.0`) installed. Use it for client-side filtering when results come back from `SearchFiles`. Fuse.js config for file names:
```ts
const fuse = useMemo(() => new Fuse(files, {
  keys: [{ name: "name", weight: 2 }, { name: "id", weight: 1 }],
  threshold: 0.4,        // 0 = exact, 1 = match anything; 0.4 is VS Code-like
  includeScore: true,
  includeMatches: true,  // for highlight rendering
  ignoreLocation: true,  // don't penalize matches far from string start
}), [files]);
```
The `ignoreLocation: true` flag is important for file names — "store" should match `src/lib/stores/sessionStore.ts`.

**Keyboard navigation:** Mirror the existing `OmnibarResultList` pattern (already implemented in this codebase):
- `highlightedIndex` state in the palette component
- `↑`/`↓` keyDown handlers on the input
- `el.scrollIntoView({ block: "nearest" })` on the highlighted item (see `OmnibarResultList.tsx` line 80)
- `Enter` → `onFileSelect(result.id)` → close palette

**Trigger:** `Ctrl+P` / `Cmd+P` global keydown handler on `window` in `FilesTab`, guarded by `offsetParent !== null` (same pattern as the existing `Ctrl+F` handler in `FilesTab.tsx` lines 94–105).

**Escape:** closes palette, returns focus to previously focused element (store `document.activeElement` before opening).

---

## 4. Scroll-to-Selected Without Jumping (R4)

### scrollIntoViewIfNeeded vs. scrollIntoView

**`element.scrollIntoViewIfNeeded()`** — Chrome/Safari only (non-standard). Does nothing if the element is fully visible; scrolls minimally if partially/not visible.

**`element.scrollIntoView({ block: "nearest" })`** — Standard. The `block: "nearest"` option scrolls to the nearest edge. However, it _always_ scrolls if the element is not 100% flush-aligned — which can cause a small jump even for visible elements.

**The correct standard pattern** is `scrollMode: "if-needed"` from the `scroll-into-view-if-needed` npm package ([scroll-into-view/scroll-into-view-if-needed](https://github.com/scroll-into-view/scroll-into-view-if-needed)):
```ts
import scrollIntoView from "scroll-into-view-if-needed";
scrollIntoView(el, { scrollMode: "if-needed", block: "nearest", inline: "nearest" });
```
This only scrolls if the element is not fully visible. It is the ponyfill for `scrollIntoView({ scrollMode: "if-needed" })` which is in the CSS spec but not yet universally implemented.

### react-arborist's Built-in Behavior

`TreeApi.scrollTo(id)` (react-arborist v3.6.1 source, `tree-api.js` line 553):
1. Calls `this.openParents(id)` — expands all ancestors.
2. Calls `utils.waitFor(() => id in this.idToIndex)` — polls every 10 ms (up to 100 tries = 1 second) until the node appears in the virtual list.
3. Calls `this.list.current?.scrollToItem(index, align)` — uses react-window's `scrollToItem` with `align = "smart"` by default.

**react-window's `align: "smart"`** scrolls only if the item is not visible; if it's already visible, it does nothing. This is exactly the "if-needed" behavior we want.

**For R4:** Use `treeRef.current?.scrollTo(selectedPath)` with the default `"smart"` alignment whenever `selectedPath` changes. Do not use custom `scrollIntoView` logic — react-arborist already handles this correctly. The key constraint: `scrollTo` only works if the node ID is already in `idToIndex` (i.e., the node is rendered in the virtual list). For the lazy-load case (R5), this is handled by `openParents` + `waitFor`.

---

## 5. Auto-Expand Ancestors / Reveal Path (R5)

### The "Reveal Path" Algorithm

Used by every mature file tree (VS Code "Reveal in Explorer", JetBrains "Select in Project View", etc.). The algorithm:

1. **Split the target path** into ancestor segments: `"src/lib/hooks/useVcsStatus.ts"` → `["src", "src/lib", "src/lib/hooks"]` + the file itself.
2. **Ensure each ancestor directory is loaded** (fetch from backend if not yet in `dirContents`). This is the lazy-load challenge.
3. **Open each ancestor** via `treeRef.current?.open(dirPath)`.
4. **Scroll the file row into view** via `treeRef.current?.scrollTo(filePath)`.

### react-arborist's `openParents` + `scrollTo`

`TreeApi.openParents(id)` (source line 396–407):
```js
openParents(identity) {
  const node = utils.dfs(this.root, id); // depth-first search in loaded tree
  let parent = node?.parent;
  while (parent) {
    this.open(parent.id);
    parent = parent.parent;
  }
}
```

**Critical limitation for lazy loading:** `dfs` searches the in-memory tree. If ancestor directories have not been loaded from the backend yet, `dfs` returns `null` and `openParents` is a no-op. `scrollTo` then polls for the node in `idToIndex` for up to 1 second — but the node will never appear if its parent directory data was never fetched.

### Required Custom "Reveal" Logic for Our Lazy Tree

Since our tree is lazily loaded per directory, we need a custom reveal sequence:

```ts
async function revealPath(path: string) {
  const segments = path.split("/");
  // Load each ancestor directory sequentially if not yet loaded.
  for (let i = 1; i < segments.length; i++) {
    const dirPath = segments.slice(0, i).join("/");
    if (!dirContents.has(dirPath)) {
      await loadDirectory(dirPath);
      // loadDirectory sets dirContents; wait for React state update
      await new Promise(r => setTimeout(r, 0));
    }
    treeRef.current?.open(dirPath);
  }
  // Now the node should be in the virtual list; scroll to it.
  treeRef.current?.scrollTo(path); // uses "smart" align — no jump if visible
}
```

**Key design decisions:**
- Load ancestor dirs sequentially (not in parallel) because each level's existence depends on the parent being in the tree state.
- After each `loadDirectory` call, a `setTimeout(r, 0)` yields to React to re-render before the next `open` call; otherwise `open` fires before the new children are in the virtual list.
- `treeRef.current?.open(dirPath)` triggers `handleToggle` which calls `loadDirectory` again — but `loadDirectory` guards against duplicate in-flight loads via `loadingPaths.has(dirPath)`. The reveal function calls `loadDirectory` directly to await completion before opening.
- After all ancestors are open, `scrollTo(path)` with its internal `waitFor` polling handles the timing of the file row appearing in the virtual list.

**Trigger:** The reveal function should be called whenever `selectedPath` changes to a path whose ancestors are not currently open — specifically when `selectedPath` is set from an external source (e.g., via `initialSelectedPath` prop from the VCS cross-link, or from the quick-open palette). When the user clicks directly in the tree, no reveal is needed.

---

## Summary

| Requirement | Best Practice | Implementation for This Project |
|---|---|---|
| **R3 Middle truncation** | JS string manipulation (extension-preserving); Canvas measurement for pixel-perfect | `middleTruncate(name, 28)` helper in `NodeRenderer`; `title={data.id}` for tooltip |
| **R4 Scroll preservation** | `scrollMode: "if-needed"` / react-window `align: "smart"` | Use `treeRef.current?.scrollTo(path)` — already uses "smart" align; no custom scroll logic needed |
| **R5 Auto-reveal** | "Reveal path" algorithm: load ancestors → open → scrollTo | Custom `revealPath()` async function that sequentially loads + opens each ancestor before calling `scrollTo` |
| **R6 Recently opened** | In-memory MRU array, max 8–10, shown at top of tree pane | `useRecentFiles(8)` hook in `FilesTab`; `RecentFilesPanel` component above `<FileTree>` |
| **R7 Quick-open palette** | Fuzzy search (Fuse.js), keyboard nav, MRU empty state, overlay modal | New `QuickOpenPalette` component using `@radix-ui/react-dialog` + existing `SearchFiles` RPC + Fuse.js (already installed) |
