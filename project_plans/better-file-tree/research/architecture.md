# Architecture Research: Better File Tree

**Date**: 2026-05-15
**Scope**: Component and state architecture changes for R1–R7

---

## 1. Where Does Resize State Live?

**Decision: `useResizablePanel` custom hook, owned by `FilesTab`.**

The existing pane split system (`ResizeHandle.tsx`, `paneSplit.css.ts`, `PaneSplitRenderer.tsx`) manages resize as a ratio (`--split-ratio` CSS custom property) for a two-pane grid. That system is general-purpose and stores ratio in a central pane store. For the file tree we need pixel-width semantics (min 160 px, max 50% viewport) with localStorage persistence — a ratio-only approach breaks when the viewport changes.

A dedicated `useResizablePanel` hook in `web-app/src/lib/hooks/useResizablePanel.ts` keeps the concern self-contained and reusable:

```ts
interface ResizablePanelOptions {
  storageKey: string;          // e.g. "filestab.treeWidth"
  defaultWidth: number;        // 260
  minWidth: number;            // 160
  maxWidthFraction: number;    // 0.5 (50% of viewport)
}

interface ResizablePanelResult {
  width: number;               // current pixel width
  collapsed: boolean;
  containerRef: RefObject<HTMLDivElement>;
  handleProps: {               // spread onto the drag handle <div>
    onPointerDown: (e: React.PointerEvent) => void;
    onPointerMove: (e: React.PointerEvent) => void;
    onPointerUp: (e: React.PointerEvent) => void;
    onPointerCancel: (e: React.PointerEvent) => void;
    style: React.CSSProperties;
  };
  collapse: () => void;
  expand: () => void;
}
```

**State shape inside the hook:**
- `widthRef` (ref, not state) — tracks pixel width during drag to avoid render-per-pixel (same pattern as existing `ResizeHandle.tsx`)
- `displayWidth` (state, `number`) — committed width after drag ends or on collapse/expand; triggers re-render
- `collapsed` (state, `boolean`) — drives the CSS class that sets `width: 0` / `overflow: hidden`
- Persistence: `useEffect` writes `{ width, collapsed }` to `localStorage[storageKey]` when either changes; initial value read synchronously in `useState(() => JSON.parse(localStorage.getItem(storageKey) ?? '{}'))` to avoid a flash

**Why not a ratio?** The requirement says "min 160 px, max 50% viewport". A pixel approach is simpler and the existing `ResizeHandle.tsx` already uses a ratio+clamping pattern that we can adapt directly. The clamping logic lives inside the hook:

```ts
function clampWidth(raw: number, containerWidth: number): number {
  return Math.max(MIN_WIDTH, Math.min(Math.floor(containerWidth * maxWidthFraction), raw));
}
```

**Why not in a context?** Only `FilesTab` owns the split. A context adds indirection with no benefit.

**FilesTab changes:**
- Add `containerRef` to the outer `<div className={container}>`
- Apply `width: panel.width, overflow: 'hidden'` as inline style (CSS custom property bridge) to `<div className={treePane}>`
- Render a `<TreeResizeHandle handleProps={panel.handleProps} />` between tree and content panes
- Render collapse/expand buttons in the toolbar

---

## 2. Recently-Opened State

**Decision: `useRef` list (array ref) inside `FilesTab`, surfaced to `FileTree` via a new prop.**

**Rationale:** The requirements explicitly say "in-memory, not persisted" (R6). A `useRef` keeps the array stable across renders without triggering extra re-renders when files are opened. A React context would be over-engineered for a single tab's internal navigation history. A custom hook (`useRecentFiles`) is a reasonable middle-ground but only pays off if other components need the list — they don't here.

**State shape in FilesTab:**

```ts
const recentFilesRef = useRef<RecentFileEntry[]>([]);

interface RecentFileEntry {
  path: string;   // full relative path (matches TreeNode.id)
  name: string;   // basename
  dir: string;    // parent directory name (last segment before the file)
}
```

Update logic in `handleFileSelect`:

```ts
const handleFileSelect = useCallback((path: string) => {
  setSelectedPath(path);
  onSelectedPathChange?.(path);
  // prepend, deduplicate, cap at 8
  const entry: RecentFileEntry = {
    path,
    name: path.split('/').pop() ?? path,
    dir: path.split('/').slice(-2, -1)[0] ?? '',
  };
  recentFilesRef.current = [
    entry,
    ...recentFilesRef.current.filter(e => e.path !== path),
  ].slice(0, 8);
  setRecentFilesTick(t => t + 1); // force re-render so FileTree sees new list
}, [onSelectedPathChange]);

const [recentFilesTick, setRecentFilesTick] = useState(0);
const recentFiles = recentFilesRef.current; // stable reference, updated above
```

**Props change to FileTree:**

```ts
interface FileTreeProps {
  // existing ...
  recentFiles?: RecentFileEntry[];   // NEW — array, hidden when empty
  onRecentFileClick?: (path: string) => void; // NEW — same as onFileSelect but from recent list
}
```

`FileTree` renders a `RecentFilesSection` above the `<Tree>` component when `recentFiles.length > 0`. This section is a plain scrollable list of rows styled identically to tree nodes — no react-arborist involvement needed for this flat list.

**RecentFilesSection component** (local to FileTree.tsx or extracted to `RecentFilesSection.tsx`):

```tsx
interface RecentFilesSectionProps {
  files: RecentFileEntry[];
  selectedPath: string | null | undefined;
  onSelect: (path: string) => void;
}
```

Each entry shows: file icon (reuse `getFileIcon`), basename, parent dir name as a muted suffix. Clicking fires `onSelect(path)` which in `FilesTab` calls `handleFileSelect` (opens file + scrolls tree).

---

## 3. Quick-Open Palette

**Decision: New `QuickOpenPalette.tsx` standalone component rendered as a React portal.**

**Rationale:**
- The palette needs to overlay the entire FilesTab (and possibly the session cockpit). Portaling to `document.body` is the standard approach for overlays and avoids z-index stacking conflicts with the tree's `overflow: hidden` container.
- Rendering inline in FilesTab would require removing `overflow: hidden` from the container or setting a high z-index on an absolutely-positioned child — both are fragile.
- A standalone component is independently testable.

**Location:** `web-app/src/components/sessions/QuickOpenPalette.tsx` + `QuickOpenPalette.css.ts`

**How it receives `sessionId` and `baseUrl`:**

FilesTab holds `isQuickOpenOpen` state and passes them as props:

```tsx
// In FilesTab render:
{isQuickOpenOpen && (
  <QuickOpenPalette
    sessionId={sessionId}
    baseUrl={baseUrl}
    onSelect={(path) => {
      setIsQuickOpenOpen(false);
      handleFileSelect(path);
    }}
    onClose={() => setIsQuickOpenOpen(false)}
  />
)}
```

`QuickOpenPalette` uses `createPortal(..., document.body)` internally.

**Keyboard wiring in FilesTab** (new keydown listener alongside the existing Cmd+F one):

```ts
if ((e.metaKey || e.ctrlKey) && e.key === 'p') {
  if (!searchInputRef.current) return;
  if (searchInputRef.current.offsetParent === null) return; // tab hidden
  e.preventDefault();
  setIsQuickOpenOpen(true);
}
```

**QuickOpenPalette props:**

```ts
interface QuickOpenPaletteProps {
  sessionId: string;
  baseUrl: string;
  onSelect: (path: string) => void;
  onClose: () => void;
}
```

**Internal state:** `query` (string), `results` (FileNode[]), `loading` (boolean), `activeIndex` (number). Uses `searchFiles` from `useFileService.ts` (already exported, no new RPC needed). Debounce: 200 ms. Arrow keys navigate `activeIndex`; Enter fires `onSelect`; Escape fires `onClose`.

**CSS:** `position: fixed; inset: 0; z-index: zIndex.modal` backdrop + centered card. Import `zIndex` from `@/styles/theme.css` (already exported from the contract).

---

## 4. Auto-Reveal Selected Path

**Decision: Imperative `ref` API — expose `revealPath(path: string)` via `useImperativeHandle`.**

**Why a ref, not a prop change?**

A prop change (`selectedPath`) already exists and handles visual highlighting. But auto-expanding ancestor directories and scrolling the highlighted row into view requires imperative calls to `treeRef.current` (react-arborist `TreeApi`). These calls need to happen _after_ the new data has rendered, requiring a timing guarantee that a `useEffect` on `selectedPath` inside `FileTree` can provide — but that effect fires on _every_ `selectedPath` change, including user clicks inside the tree where scroll is already correct (AC-5: "tree scroll position stays at bottom when user opens a file in the middle"). So the effect must distinguish "external reveal" from "internal selection".

The cleanest solution: a separate `revealPath` imperative handle that FilesTab calls _only_ when selection arrives externally (from `initialSelectedPath` changes or from recent/quick-open file selection that the tree didn't initiate):

```ts
// FileTree.tsx
export interface FileTreeHandle {
  revealPath: (path: string) => void;
  collapseAll: () => void;  // replaces the existing onCollapseAllRef callback pattern
}

// Usage in FilesTab:
const fileTreeRef = useRef<FileTreeHandle>(null);

// When external cross-link arrives:
useEffect(() => {
  if (initialSelectedPath) {
    setSelectedPath(initialSelectedPath);
    fileTreeRef.current?.revealPath(initialSelectedPath);
  }
}, [initialSelectedPath]);
```

`revealPath` implementation inside `FileTree`:

```ts
useImperativeHandle(ref, () => ({
  revealPath: async (path: string) => {
    // 1. Ensure all ancestor directories are loaded and opened.
    const segments = path.split('/');
    for (let i = 1; i < segments.length; i++) {
      const dirPath = segments.slice(0, i).join('/');
      if (!dirContents.has(dirPath)) {
        await loadDirectory(dirPath);  // async load
      }
      treeRef.current?.open(dirPath);
    }
    // 2. After state settles, scroll the node into view.
    requestAnimationFrame(() => {
      treeRef.current?.scrollTo(path, { align: 'auto' }); // react-arborist API
    });
  },
  collapseAll: () => treeRef.current?.closeAll(),
}));
```

Note: `loadDirectory` must be stable (useMemo with correct deps) for this to work reliably. The existing implementation has a stale closure issue (`loadingPaths` in deps) — this should be fixed as part of R5 work.

**Prop cleanup:** The existing `onCollapseAllRef` callback prop is replaced by `collapseAll()` on the forwarded ref, which is cleaner and type-safe.

**FileTree signature change:**

```ts
// Before:
export function FileTree({ ..., onCollapseAllRef, ... }: FileTreeProps)

// After:
export const FileTree = forwardRef<FileTreeHandle, FileTreeProps>(
  function FileTree({ ..., /* onCollapseAllRef removed */ ... }, ref) { ... }
)
```

---

## 5. Mobile Layout

**Decision: React state `mobilePane: 'tree' | 'content'` in `FilesTab` + CSS `@media` query in `FilesTab.css.ts` to switch from flex-row to single-pane display.**

**CSS changes to `FilesTab.css.ts`:**

The current `container` is `display: flex` with fixed `treePane` width (30%, hardcoded). For mobile, we need a single-pane layout controlled by a data attribute:

```ts
// FilesTab.css.ts additions

export const container = style({
  display: "flex",
  height: "100%",
  overflow: "hidden",
  background: vars.color.terminalBackground,
  // No @media here — mobile switching is data-attribute driven (see below)
});

export const treePaneCollapsed = style({
  // Applied when panel.collapsed = true (desktop collapse)
  width: "0 !important",
  overflow: "hidden",
  borderRight: "none",
});

// New: mobile single-pane CSS
// On mobile, both panes are full-width; visibility is toggled via display
export const mobilePaneHidden = style({
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      display: "none !important",
    },
  },
});

export const mobilePaneVisible = style({
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      display: "flex !important",
      flex: "1 !important",
      width: "100% !important",
      maxWidth: "none !important",
    },
  },
});

export const mobileBackButton = style({
  display: "none",
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      display: "flex",
      alignItems: "center",
      gap: 4,
      padding: "6px 12px",
      fontSize: vars.fontSize.sm,
      background: "transparent",
      border: "none",
      color: vars.color.primary,
      cursor: "pointer",
      flexShrink: 0,
    },
  },
});
```

**React state in FilesTab:**

```ts
const [mobilePane, setMobilePane] = useState<'tree' | 'content'>('tree');

// Switch to content pane when a file is selected (mobile)
const handleFileSelect = useCallback((path: string) => {
  setSelectedPath(path);
  onSelectedPathChange?.(path);
  // ... update recentFiles ...
  setMobilePane('content'); // auto-switch on mobile; no-op on desktop (CSS handles display)
}, [onSelectedPathChange]);
```

**Render changes in FilesTab:**

```tsx
<div className={container}>
  {/* Tree pane */}
  <div
    className={`${treePane} ${panel.collapsed ? treePaneCollapsed : ''} ${
      mobilePane === 'content' ? mobilePaneHidden : mobilePaneVisible
    }`}
    style={{ width: panel.collapsed ? 0 : panel.width }}
  >
    {/* toolbar + FileTree */}
  </div>

  {/* Resize handle — hidden on mobile */}
  <TreeResizeHandle {...panel.handleProps} />

  {/* Content pane */}
  <div className={`${contentPane} ${mobilePane === 'tree' ? mobilePaneHidden : mobilePaneVisible}`}>
    {/* Mobile back button — only visible via CSS @media */}
    <button
      className={mobileBackButton}
      onClick={() => setMobilePane('tree')}
    >
      ← Files
    </button>
    <FileContentViewer sessionId={sessionId} filePath={selectedPath} baseUrl={baseUrl} />
  </div>
</div>
```

**Why CSS classes + `mobilePane` state, not just media query alone?**

A pure CSS media query can hide/show panes, but it cannot control which pane is "active" after a file selection — that's user-triggered state. The combination of React state (which pane) and CSS (how it looks at each breakpoint) is the standard approach used elsewhere in this codebase (e.g., `mobilePaneTabStrip.css.ts`).

**Why not `display: none` on the resize handle on mobile?**

The handle between the panes has `display: none` on mobile automatically because its parent grid collapses. On mobile the tree pane takes `width: 100%` and the content pane is hidden (or vice versa), so the handle is sandwiched and has zero size anyway. Explicitly hiding it is cleaner but not required for correctness.

---

## Summary of Component Interface Changes

### `FilesTab` — additions

| Addition | Purpose |
|---|---|
| `useResizablePanel` hook | R1 resize + collapse/expand + localStorage |
| `mobilePane` state (`'tree' \| 'content'`) | R2 mobile pane switching |
| `recentFilesRef` + `recentFilesTick` | R6 recently-opened list |
| `isQuickOpenOpen` state | R7 Ctrl+P palette toggle |
| `fileTreeRef` (forwarded ref) | R5 imperative revealPath |
| `<TreeResizeHandle>` between panes | R1 drag handle |
| `<QuickOpenPalette>` portal | R7 quick-open |

### `FileTree` — prop and API changes

| Change | From | To |
|---|---|---|
| Export style | `function FileTree(...)` | `const FileTree = forwardRef<FileTreeHandle, FileTreeProps>(...)` |
| `onCollapseAllRef` prop | callback ref pattern | removed; use `fileTreeRef.current.collapseAll()` |
| `recentFiles` prop | (none) | `RecentFileEntry[]` (optional, default `[]`) |
| `onRecentFileClick` prop | (none) | `(path: string) => void` (optional) |
| `selectedPath` prop | unchanged | unchanged |
| `revealPath` in handle | (none) | opens ancestor dirs + scrolls to node |

### New files

| File | Contents |
|---|---|
| `web-app/src/lib/hooks/useResizablePanel.ts` | R1 resize/collapse/localStorage hook |
| `web-app/src/components/sessions/QuickOpenPalette.tsx` | R7 Ctrl+P overlay |
| `web-app/src/components/sessions/QuickOpenPalette.css.ts` | Palette styles |
| `web-app/src/components/sessions/RecentFilesSection.tsx` | R6 recent list (optional extraction) |

### Modified files

| File | Changes |
|---|---|
| `FilesTab.tsx` | All 5 features wired here |
| `FilesTab.css.ts` | Mobile pane classes, resize handle slot, collapse styles |
| `FileTree.tsx` | `forwardRef`, `FileTreeHandle`, `recentFiles` prop, `revealPath` impl |
| `FileTree.css.ts` | R3 middle-truncation: `name` style uses `direction: rtl` trick or JS formatting |

---

## R3 File Name Truncation: Implementation Note

CSS-only middle truncation is not reliably supported (`text-overflow: ellipsis` always clips at the end). Two approaches:

1. **JS formatting** (recommended): In `NodeRenderer`, call a helper `truncateMiddle(name, maxChars)` that computes `foo…bar.tsx` based on a character budget derived from the measured pane width. Pass the full path as `title` attribute on the `<span className={nameClass}>`.

2. **CSS `direction: rtl` trick**: Set `direction: rtl; text-overflow: ellipsis` on the name span — this clips the _left_ side (middle of a long filename) but treats the extension as the end. This works for most file names but breaks on names without extensions. Not recommended.

The JS approach is more reliable. The character budget can be derived from `Math.floor(panel.width / 7.5)` (approximate char width in mono 13px) and recalculated when `panel.width` changes. The function is pure and easily unit-tested.

---

## Key Architectural Constraints

1. **No new RPCs needed.** `searchFiles` already exists and is used by the quick-open palette. `listFiles` handles directory loading. `getFileContent` handles file content. All R1–R7 features work with the existing proto surface.

2. **`useResizablePanel` must use pointer events, not mouse events**, to support touch drag on mobile (matching the existing `ResizeHandle.tsx` pattern with `setPointerCapture`).

3. **`revealPath` is async** because ancestor directories may not be loaded yet. FilesTab callers should fire-and-forget (`fileTreeRef.current?.revealPath(path)`) — the loading state is managed inside `FileTree`.

4. **The `recentFiles` list update must force a re-render.** Since the list lives in a `useRef`, FilesTab needs a lightweight `setRecentFilesTick(t => t+1)` state trigger whenever the list changes (one extra render per file open — acceptable).
