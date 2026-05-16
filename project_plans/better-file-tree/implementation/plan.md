# Implementation Plan: Better File Tree UX

**Date**: 2026-05-15
**Requirements source**: `project_plans/better-file-tree/requirements.md`
**Research sources**: `research/stack.md`, `research/features.md`, `research/architecture.md`, `research/pitfalls.md`

---

## Epics

### Epic 1: Resizable + Collapsible Tree Panel
**Goal**: Let users drag the divider between tree and content panes to set a persistent custom width, and collapse/expand the tree panel with a button. On mobile viewports (< 768 px), switch to a single-pane layout with a back button.
**Requirements covered**: R1, R2
**Pitfalls addressed**: P1 (pointer capture for drag), P6 (orientation snap-back), P7 (scroll reset on resize)

#### Story 1.1: `useResizablePanel` hook
**Tasks**:
- [ ] Task 1.1.1: Create `web-app/src/lib/hooks/useResizablePanel.ts` — implement the hook with interface `ResizablePanelOptions { storageKey, defaultWidth, minWidth, maxWidthFraction }` returning `{ width, collapsed, containerRef, handleProps, collapse, expand }`. Initialize `width` from `localStorage` using the lazy-init pattern from `useListColumnWidth.ts` (try/catch, SSR guard). Store `collapsed` in a separate `localStorage` key (`storageKey + 'Collapsed'`).
- [ ] Task 1.1.2: In `useResizablePanel.ts` — implement drag logic using pointer capture: `onPointerDown` calls `e.currentTarget.setPointerCapture(e.pointerId)` and sets `isDragging = true`; `onPointerMove` computes `newWidth = e.clientX - containerRef.current.getBoundingClientRect().left` then clamps via `Math.max(minWidth, Math.min(containerWidth * maxWidthFraction, raw))`, updates `widthRef.current` inside `requestAnimationFrame` callback and calls `setDisplayWidth`; `onPointerUp` / `onPointerCancel` calls `releasePointerCapture` and clears drag state. This satisfies P1 (pointer leaves window).
- [ ] Task 1.1.3: In `useResizablePanel.ts` — persist `width` and `collapsed` to localStorage via `useEffect` that writes on every change. Persist `width` as stringified number to key `filestab.treeWidth`; persist `collapsed` as `"true"/"false"` to key `filestab.treeCollapsed`. Wrap writes in `try { } catch { }`.
- [ ] Task 1.1.4: In `useResizablePanel.ts` — implement `collapse()`: saves current width to a `lastWidthRef`, sets `collapsed = true`. Implement `expand()`: sets `collapsed = false`, restores width from `lastWidthRef` or `defaultWidth` if ref is empty.

#### Story 1.2: Resize handle component
**Tasks**:
- [ ] Task 1.2.1: Create `web-app/src/components/sessions/TreeResizeHandle.tsx` — a thin wrapper `<div>` that spreads `handleProps` from `useResizablePanel`. Apply `cursor: col-resize` style on the element. Set `touch-action: none` to prevent mobile scroll conflicts. Width of the interactive zone: 8 px visual, 20 px hit target (match the existing `resizeHandle.css.ts` pattern).
- [ ] Task 1.2.2: Create `web-app/src/components/sessions/TreeResizeHandle.css.ts` — vanilla-extract style for the handle. The handle bar is `width: 4px`, the hit target wrapper uses a negative margin trick (`margin: 0 -8px; padding: 0 8px`) to expand the clickable area without affecting layout. Hide on mobile via `@media (max-width: 767px) { display: none }`.

#### Story 1.3: Mobile single-pane layout
**Tasks**:
- [ ] Task 1.3.1: In `web-app/src/components/sessions/FilesTab.css.ts` — remove the hardcoded `width: "30%"`, `minWidth: 200`, `maxWidth: 480` from `treePane`. Replace with a style that has no fixed width (width will be injected via inline style from the hook). Add `export const treePaneCollapsed = style({ width: "0 !important", overflow: "hidden", borderRight: "none" })` for the collapsed state.
- [ ] Task 1.3.2: In `web-app/src/components/sessions/FilesTab.css.ts` — add `export const mobilePaneHidden` and `export const mobilePaneVisible` styles that use `@media (max-width: 767px)` to set `display: none !important` and `display: flex !important; flex: 1; width: 100%; maxWidth: none` respectively. Add `export const mobileBackButton` style that is `display: none` normally, `display: flex` at the mobile breakpoint with appropriate padding, color matching `vars.color.primary`, and `cursor: pointer`.
- [ ] Task 1.3.3: In `web-app/src/components/sessions/FilesTab.tsx` — add `const [mobilePane, setMobilePane] = useState<'tree' | 'content'>('tree')`. In `handleFileSelect`, add `setMobilePane('content')` after `setSelectedPath`. This is a no-op on desktop because the CSS media query controls visibility independently.
- [ ] Task 1.3.4: In `web-app/src/components/sessions/FilesTab.tsx` — wire `useResizablePanel` with `{ storageKey: 'filestab.treeWidth', defaultWidth: 260, minWidth: 160, maxWidthFraction: 0.5 }`. Add `ref={panel.containerRef}` to the outer `<div className={container}>`. Apply `style={{ width: panel.collapsed ? 0 : panel.width }}` as inline style on the `treePane` div (overrides any CSS width). Apply `treePaneCollapsed` class conditionally when `panel.collapsed`. Apply `mobilePaneHidden` / `mobilePaneVisible` classes based on `mobilePane` state.
- [ ] Task 1.3.5: In `web-app/src/components/sessions/FilesTab.tsx` — render `<TreeResizeHandle {...panel.handleProps} />` between the tree pane and content pane divs. Render collapse/expand buttons in the toolbar: a `←` arrow button that calls `panel.collapse()` (hidden when already collapsed), and a `→` arrow button that calls `panel.expand()` (hidden when not collapsed). Apply appropriate `toolbarButton` class and `title` attributes.
- [ ] Task 1.3.6: In `web-app/src/components/sessions/FilesTab.tsx` — add a back button inside the `contentPane` div: `<button className={mobileBackButton} onClick={() => setMobilePane('tree')}>← Files</button>`. This button is invisible on desktop (CSS `display: none`) and visible only at ≤ 767 px.
- [ ] Task 1.3.7: In `web-app/src/components/sessions/FilesTab.tsx` — apply `mobilePaneHidden` / `mobilePaneVisible` classes to the content pane div based on `mobilePane === 'tree'`. When `mobilePane === 'tree'`, the content pane is hidden on mobile. When `mobilePane === 'content'`, the tree pane is hidden on mobile. This satisfies R2 / AC-3.
- [ ] Task 1.3.8: In `web-app/src/components/sessions/FilesTab.tsx` — guard against orientation snap-back (P6): the `mobilePane` state swap happens only as a side effect of user actions; desktop CSS overrides the state via `mobilePaneVisible` which forces both panes visible at > 767 px regardless of `mobilePane` state. Verify this is correct by reviewing that `mobilePaneHidden` only applies `display: none` inside the `@media (max-width: 767px)` block.

---

### Epic 2: File Name Display + Tree UX Polish
**Goal**: Fix truncated file names using middle-truncation that preserves both start and extension; preserve tree scroll position when switching files; auto-reveal and highlight the currently open file when selected from outside the tree.
**Requirements covered**: R3, R4, R5
**Pitfalls addressed**: P2 (JS middle truncation), P3 (auto-reveal race), P7 (scroll position), cross-cutting loadDirectory stale closure

#### Story 2.1: Middle truncation helper and NodeRenderer wiring
**Tasks**:
- [ ] Task 2.1.1: Create `web-app/src/lib/utils/truncateMiddle.ts` — export `function truncateMiddle(name: string, maxLen: number): string`. Algorithm: if `name.length <= maxLen` return `name`; find last `.` index as `ext = name.lastIndexOf(".")`; if `ext > 0` set `suffix = name.slice(ext)` else `suffix = ""`; `keep = maxLen - suffix.length - 1` (1 for "…"); `head = Math.ceil(keep * 0.6)`; `tail = keep - head`; return `name.slice(0, head) + "…" + name.slice(name.length - tail - suffix.length, name.length - suffix.length) + suffix`. Edge cases: ensure `head >= 1` and `tail >= 1` when `maxLen >= 5`.
- [ ] Task 2.1.2: In `web-app/src/components/sessions/FileTree.tsx` — add `maxChars` prop to `NodeRendererProps` interface: `maxChars: number`. In the `FileTree` component, compute `const maxChars = useMemo(() => Math.floor((dims.w - 48) / 7.5), [dims.w])` where 48 accounts for indent + icon + badge pixels (16px indent per level average ~24px + 16px icon + 8px badges). Pass `maxChars` to `NodeRenderer` via the render prop callback.
- [ ] Task 2.1.3: In `web-app/src/components/sessions/FileTree.tsx` — in `NodeRenderer`, import `truncateMiddle`. Replace the `<span className={nameClass}>` content from `{highlightMatch(data.name, searchTerm)}` to: when `searchTerm` is active use `highlightMatch(data.name, searchTerm)` unchanged (search mode highlights the raw name); when not in search mode use `truncateMiddle(data.name, maxChars)`. Add `title={data.id}` on the outer node `<div>` (the full relative path as tooltip). This satisfies R3 / AC-4.
- [ ] Task 2.1.4: In `web-app/src/components/sessions/FileTree.css.ts` — the existing `name` style already has `overflow: hidden; textOverflow: ellipsis; whiteSpace: nowrap` — keep this as a CSS safety fallback for edge cases where JS truncation runs with an unexpectedly large `maxChars`. No CSS-only truncation change needed.

#### Story 2.2: Fix `loadDirectory` stale closure (prerequisite for R5)
**Tasks**:
- [ ] Task 2.2.1: In `web-app/src/components/sessions/FileTree.tsx` — replace `loadingPaths` state with `loadingPathsRef = useRef<Set<string>>(new Set())`. In the `loadDirectory` callback, read/write `loadingPathsRef.current` directly instead of calling `setLoadingPaths`. To trigger re-renders for spinner display only, keep a separate `loadingPaths` state but update it by calling `setLoadingPaths(new Set(loadingPathsRef.current))` after mutating the ref. Remove `loadingPaths` from the `useCallback` deps array. This fixes the stale closure bug (cross-cutting pitfall) that would otherwise break `revealPath` sequential loading.
- [ ] Task 2.2.2: In `web-app/src/components/sessions/FileTree.tsx` — expose `loadDirectory` via a stable ref: `const loadDirectoryRef = useRef(loadDirectory); useEffect(() => { loadDirectoryRef.current = loadDirectory; }, [loadDirectory])`. This allows `revealPath` inside `useImperativeHandle` to always call the latest version of `loadDirectory` without closing over a stale version.

#### Story 2.3: `forwardRef` + `FileTreeHandle` with `revealPath`
**Tasks**:
- [ ] Task 2.3.1: In `web-app/src/components/sessions/FileTree.tsx` — export `interface FileTreeHandle { revealPath: (path: string) => void; collapseAll: () => void; }`. Change the component from `export function FileTree(...)` to `export const FileTree = forwardRef<FileTreeHandle, FileTreeProps>(function FileTree({ ..., /* onCollapseAllRef removed */ }, ref) { ... })`. Add `import { forwardRef, useImperativeHandle } from "react"`.
- [ ] Task 2.3.2: In `web-app/src/components/sessions/FileTree.tsx` — implement `useImperativeHandle(ref, () => ({ collapseAll: () => treeRef.current?.closeAll(), revealPath: async (path: string) => { ... } }), [dirContents])`. The `revealPath` implementation: (1) store a cancel check: capture `AbortController` signal via `revealAbortRef.current` (see Task 2.3.3); (2) split `path` into ancestor segments; (3) for each ancestor from shallowest to deepest: check `signal.aborted`, if `!dirContents.has(ancestorPath)` call `await loadDirectoryRef.current(ancestorPath)` then `await new Promise(r => setTimeout(r, 0))`; call `treeRef.current?.open(ancestorPath)`; (4) after all ancestors, check `signal.aborted`, then call `requestAnimationFrame(() => treeRef.current?.scrollTo(path))`.
- [ ] Task 2.3.3: In `web-app/src/components/sessions/FilesTab.tsx` — add `const fileTreeRef = useRef<FileTreeHandle>(null)` and `const revealAbortRef = useRef<AbortController | null>(null)`. Pass `ref={fileTreeRef}` to `<FileTree>`. Pass `revealAbortRef` down as a prop or pass a `onRevealStart` callback — simpler: keep `revealAbortRef` in `FilesTab` and pass `abortSignal` down, or place it inside `FileTree`'s `useImperativeHandle` scope. **Simplest implementation**: `revealAbortRef` lives inside `FileTree.tsx` as a module-level ref on the component instance — declare it inside the `forwardRef` body: `const revealAbortRef = useRef<AbortController | null>(null)`. Before each `revealPath` call, `revealAbortRef.current?.abort(); revealAbortRef.current = new AbortController()`. This satisfies P3.
- [ ] Task 2.3.4: In `web-app/src/components/sessions/FilesTab.tsx` — update the `useEffect` for `initialSelectedPath` to call `fileTreeRef.current?.revealPath(initialSelectedPath)` after `setSelectedPath`. This triggers auto-expand + scroll when an external path arrives (VCS cross-link, R5 / AC-6). Remove the `fileTreeCollapseRef` ref and the `onCollapseAllRef` prop wiring; replace the collapse toolbar button's `onClick` with `() => fileTreeRef.current?.collapseAll()`.
- [ ] Task 2.3.5: In `web-app/src/components/sessions/FileTree.tsx` — remove `onCollapseAllRef` from `FileTreeProps` interface and from the destructured props. Remove the `useEffect` that called `onCollapseAllRef(...)`. This cleans up the old callback-ref pattern replaced by the imperative handle.

#### Story 2.4: Scroll preservation (R4)
**Tasks**:
- [ ] Task 2.4.1: In `web-app/src/components/sessions/FileTree.tsx` — verify that the existing `ResizeObserver` callback in `FileTree` (lines 361–374) already uses `requestAnimationFrame` before calling `setDims`. This prevents layout thrash but does not explicitly save/restore scroll. After `setDims`, the `<Tree width={dims.w} height={dims.h}>` props change triggers react-window to re-measure. Add a scroll-restore step: capture `treeRef.current?.root?.tree?.list?.current?.scrollOffset` before the dims update by storing it in a `scrollOffsetRef = useRef(0)`, read it in the `ResizeObserver` callback before the RAF, then after `setDims` call `requestAnimationFrame(() => { treeRef.current?.list?.current?.scrollTo(scrollOffsetRef.current) })`. Note: react-window's `FixedSizeList` exposes `scrollTo(offset)` on its ref; react-arborist passes the list ref as `treeRef.current.list`. Verify the API path before final implementation.
- [ ] Task 2.4.2: In `web-app/src/components/sessions/FileTree.tsx` — ensure `selectedPath` prop changes do NOT trigger any scroll. The `selectedPath` prop is only passed through to `NodeRenderer` for styling; it has no direct effect on the `<Tree>` component's scroll position. Confirm no `useEffect` on `selectedPath` calls `scrollTo`. The only scroll calls must come from (a) `revealPath` (external reveal, R5) and (b) user's own keyboard navigation in `handleTreeKeyDown` (already correct).

---

### Epic 3: Recently Opened Files
**Goal**: Show a "Recent" section at the top of the tree pane listing up to 8 most-recently opened files this session. The section is hidden until at least one file has been opened.
**Requirements covered**: R6
**Pitfalls addressed**: P4 (deduplication)

#### Story 3.1: Recent files state in FilesTab
**Tasks**:
- [ ] Task 3.1.1: In `web-app/src/components/sessions/FilesTab.tsx` — add `const [recentPaths, setRecentPaths] = useState<string[]>([])`. In `handleFileSelect`, add the dedup-and-prepend logic: `setRecentPaths(prev => [path, ...prev.filter(p => p !== path)].slice(0, 8))`. This is pure state (no ref needed since `useState` already triggers the re-render) and satisfies P4 deduplication.

#### Story 3.2: RecentFilesSection component
**Tasks**:
- [ ] Task 3.2.1: Create `web-app/src/components/sessions/RecentFilesSection.tsx` — component with props `{ paths: string[], selectedPath: string | null | undefined, onSelect: (path: string) => void }`. Renders a `<div>` wrapping a heading label "Recent" and a list of entries. Each entry `<button>` renders: the file icon via `getFileIcon(basename)`, the basename (bold), and the parent dir name (muted, smaller). Bind `title={path}` on the button for full-path tooltip. Call `onSelect(path)` on click. Hidden entirely when `paths.length === 0` via early return `null`.
- [ ] Task 3.2.2: Create `web-app/src/components/sessions/RecentFilesSection.css.ts` — vanilla-extract styles. The section container has `borderBottom: 1px solid vars.color.borderColor` and `paddingBottom: 4px`. The heading is `fontSize: 11, color: vars.color.textMuted, textTransform: "uppercase", padding: "4px 8px 2px"`. Each row button is styled identically to the tree `node` class (height 28, cursor pointer, hover background). The selected state matches `FileTree.css.ts`'s `selected` style.
- [ ] Task 3.2.3: Move `getFileIcon` from `FileTree.tsx` to `web-app/src/lib/utils/fileIcons.ts` — export it as a named export so both `FileTree.tsx` and `RecentFilesSection.tsx` can import it without circular deps.
- [ ] Task 3.2.4: In `web-app/src/components/sessions/FilesTab.tsx` — render `<RecentFilesSection paths={recentPaths} selectedPath={selectedPath} onSelect={handleFileSelect} />` inside the `<div className={treeWrapper}>`, above `<FileTree>`. This ensures it appears above the directory tree in the left pane.
- [ ] Task 3.2.5: In `web-app/src/components/sessions/FilesTab.tsx` — when a recent file is selected (`onSelect` in `RecentFilesSection`), `handleFileSelect` already handles it (sets `selectedPath`, updates `recentPaths`). Additionally call `fileTreeRef.current?.revealPath(path)` to scroll the tree to the selected file (satisfies the "clicking an entry opens the file and scrolls the tree to it" requirement in R6).

---

### Epic 4: Quick-Open Palette (Ctrl+P)
**Goal**: Open a Ctrl+P overlay palette that fuzzy-searches file names across the entire tree using the existing `SearchFiles` RPC and Fuse.js. Keyboard navigation mirrors the existing `OmnibarResultList` pattern. State is entirely isolated from the tree's inline search.
**Requirements covered**: R7 (Ctrl+P, Escape, Arrow keys, Enter)
**Pitfalls addressed**: P5 (state isolation, debounce, focus trap)

#### Story 4.1: QuickOpenPalette component
**Tasks**:
- [ ] Task 4.1.1: Create `web-app/src/components/sessions/QuickOpenPalette.tsx` — component with props `{ sessionId: string, baseUrl: string, recentPaths: string[], onSelect: (path: string) => void, onClose: () => void }`. Renders via `createPortal(..., document.body)`. The portal content is a full-screen backdrop `<div>` (fixed, inset 0, translucent) with a centered card `<div>` (fixed, top 20%, left 50%, transform translateX(-50%), width min(560px, 90vw), z-index from `vars.zIndex.modal` or equivalent).
- [ ] Task 4.1.2: In `QuickOpenPalette.tsx` — internal state: `const [query, setQuery] = useState("")`, `const [results, setResults] = useState<FileNode[]>([])`, `const [loading, setLoading] = useState(false)`, `const [activeIndex, setActiveIndex] = useState(0)`. Store `prevFocusRef = useRef<Element | null>(null)` set to `document.activeElement` on mount; restore focus to it on unmount.
- [ ] Task 4.1.3: In `QuickOpenPalette.tsx` — use a debounced `SearchFiles` RPC call (300 ms, same debounce pattern as `FileTree.tsx` with `searchTimerRef` and `searchRequestIdRef`). When `query === ""`, show `recentPaths` as the results list (formatted as `{ name: basename, id: path }` objects). When `query.length >= 1`, fetch from RPC and run results through Fuse.js for client-side ranking: `new Fuse(files, { keys: [{ name: "name", weight: 2 }, { name: "id", weight: 1 }], threshold: 0.4, ignoreLocation: true })`. State isolation: do NOT touch `FilesTab.searchTerm`. Satisfies P5b.
- [ ] Task 4.1.4: In `QuickOpenPalette.tsx` — implement keyboard navigation: `onKeyDown` on the `<input>` handles `ArrowDown` (increment `activeIndex` mod results.length), `ArrowUp` (decrement, wrap), `Enter` (call `onSelect(results[activeIndex].id)` then `onClose()`), `Escape` (call `onClose()`). After `activeIndex` changes, call `resultRefs[activeIndex].current?.scrollIntoView({ block: "nearest" })` — mirror the `OmnibarResultList.tsx` pattern. This satisfies R7 arrow key + Enter + Escape requirements.
- [ ] Task 4.1.5: In `QuickOpenPalette.tsx` — implement a simple focus trap (P5d): on the container `<div>` add `onKeyDown` handler that intercepts `Tab` and `Shift+Tab` and calls `e.preventDefault()` when only one focusable element exists (the input). If results are rendered as focusable buttons, let Tab cycle only within the palette by preventing default when the last/first focusable element would lose focus.
- [ ] Task 4.1.6: In `QuickOpenPalette.tsx` — render a `<ul>` of result items. Each `<li>` contains a `<button>` showing: file icon (`getFileIcon(basename)`), basename (`<span>` primary color), path without filename (`<span>` muted, smaller). Apply `activeClass` style when `i === activeIndex`. On click call `onSelect(item.id)` then `onClose()`.
- [ ] Task 4.1.7: Create `web-app/src/components/sessions/QuickOpenPalette.css.ts` — vanilla-extract styles. Backdrop: `position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)", zIndex: 1000`. Card: `position: "fixed", top: "20%", left: "50%", transform: "translateX(-50%)", width: "min(560px, 90vw)", background: vars.color.terminalBackground, border: \`1px solid ${vars.color.borderColor}\`, borderRadius: 8, overflow: "hidden", boxShadow: "0 8px 32px rgba(0,0,0,0.4)"`. Input inside card: full width, no border, padding 12px 16px, fontSize 14px. Results list: max-height 360px, overflow-y auto. Each result row: height 40px flex row gap 8px, padding 0 12px, cursor pointer, hover background. Active state: background matching `selected` in `FileTree.css.ts`.

#### Story 4.2: Keyboard trigger wiring in FilesTab
**Tasks**:
- [ ] Task 4.2.1: In `web-app/src/components/sessions/FilesTab.tsx` — add `const [isQuickOpenOpen, setIsQuickOpenOpen] = useState(false)`. In the existing `useEffect` keydown handler (lines 94–105), add a new branch alongside the Ctrl+F handler: `if ((e.metaKey || e.ctrlKey) && e.key === 'p') { if (!searchInputRef.current) return; if (searchInputRef.current.offsetParent === null) return; e.preventDefault(); setIsQuickOpenOpen(true); }`. This reuses the same `offsetParent === null` guard for tab visibility.
- [ ] Task 4.2.2: In `web-app/src/components/sessions/FilesTab.tsx` — conditionally render `<QuickOpenPalette>` when `isQuickOpenOpen`: `{isQuickOpenOpen && <QuickOpenPalette sessionId={sessionId} baseUrl={baseUrl} recentPaths={recentPaths} onSelect={(path) => { setIsQuickOpenOpen(false); handleFileSelect(path); fileTreeRef.current?.revealPath(path); }} onClose={() => setIsQuickOpenOpen(false)} />}`. The `onSelect` handler both opens the file and scrolls the tree to it. State isolation is complete: `QuickOpenPalette` does not interact with `searchTerm` in `FilesTab`.

#### Story 4.3: Keyboard nav enhancements (R7 — verify existing + Escape fix)
**Tasks**:
- [ ] Task 4.3.1: In `web-app/src/components/sessions/FilesTab.tsx` — verify that `Ctrl+F` / `Cmd+F` successfully focuses the search input even when focus is in the content pane. The existing handler at lines 94–105 already uses a global `window.addEventListener` so it fires regardless of focus location. No change needed if the `offsetParent` guard is correct — test against a non-visible tab to confirm.
- [ ] Task 4.3.2: In `web-app/src/components/sessions/FilesTab.tsx` — add an `onKeyDown` handler on the search input to handle `Escape`: `onKeyDown={(e) => { if (e.key === 'Escape') { setSearchTerm(""); searchInputRef.current?.blur(); } }}`. This satisfies the "Escape clears search and returns focus to tree" requirement in R7. Focus can be returned to the tree container by calling `containerRef.current?.focus()` — the `<Tree>` wrapper `div` already has `tabIndex={0}`.

---

## File Change Summary

### New files
| File | Epic |
|---|---|
| `web-app/src/lib/hooks/useResizablePanel.ts` | Epic 1 |
| `web-app/src/components/sessions/TreeResizeHandle.tsx` | Epic 1 |
| `web-app/src/components/sessions/TreeResizeHandle.css.ts` | Epic 1 |
| `web-app/src/components/sessions/RecentFilesSection.tsx` | Epic 3 |
| `web-app/src/components/sessions/RecentFilesSection.css.ts` | Epic 3 |
| `web-app/src/components/sessions/QuickOpenPalette.tsx` | Epic 4 |
| `web-app/src/components/sessions/QuickOpenPalette.css.ts` | Epic 4 |
| `web-app/src/lib/utils/truncateMiddle.ts` | Epic 2 |
| `web-app/src/lib/utils/fileIcons.ts` | Epic 3 (extracted) |

### Modified files
| File | Changes |
|---|---|
| `web-app/src/components/sessions/FilesTab.tsx` | All epics: hook wiring, mobile state, recent files, quick-open, Escape handler |
| `web-app/src/components/sessions/FilesTab.css.ts` | Remove hardcoded treePane width; add collapsed/mobile styles |
| `web-app/src/components/sessions/FileTree.tsx` | forwardRef + FileTreeHandle; remove onCollapseAllRef; revealPath; fix loadDirectory stale closure; maxChars prop; middle truncation |
| `web-app/src/components/sessions/FileTree.css.ts` | Minor: keep name style as CSS fallback (no structural change needed) |

---

## Acceptance Criteria Cross-Reference

| AC | Story | Key Tasks |
|---|---|---|
| AC-1 (drag to 400 px, reload, still 400 px) | 1.1 | 1.1.1, 1.1.3 |
| AC-2 (collapse/expand) | 1.1, 1.3 | 1.1.4, 1.3.5 |
| AC-3 (mobile 375 px, back button) | 1.3 | 1.3.3, 1.3.4, 1.3.6, 1.3.7 |
| AC-4 (middle truncation of long name) | 2.1 | 2.1.1, 2.1.2, 2.1.3 |
| AC-5 (tree scroll stays put when file opened) | 2.4 | 2.4.1, 2.4.2 |
| AC-6 (auto-expand + scroll via VCS cross-link) | 2.2, 2.3 | 2.2.1, 2.2.2, 2.3.1–2.3.4 |
| AC-7 (open 3 files, Recent shows all 3) | 3.1, 3.2 | 3.1.1, 3.2.1–3.2.4 |
| AC-8 (Ctrl+P opens palette, "store" filters) | 4.1, 4.2 | 4.1.1–4.1.7, 4.2.1–4.2.2 |
