# File Tree Performance, Theme, and File Browser Enhancement

## Epic Overview

### User Value
A solo developer navigating code changes in stapler-squad needs to click around the file tree without perceptible delay, view files correctly in both light and dark mode, and download or preview any file (including images) created by an LLM session — without switching to an IDE.

### Success Metrics
- Directory expand latency: < 100ms perceived (was: ~200–500ms due to cold disk + gitignore parse)
- Search first-result latency: < 300ms (was: ~500ms+ due to full WalkDir per query)
- File viewer readable in light mode (currently broken: Shiki theme CSS not activated)
- Image files render inline; all files downloadable with one click
- Smooth 60fps scroll in trees with 1000+ files (virtual scroll fills actual container height)

### Scope
**In:** Backend caching, frontend memoization, ResizeObserver dynamic height, light/dark mode file viewer, image viewer, file download button
**Out:** Editing files, replacing react-arborist, server-side search ranking, offline mode, drag-and-drop

### Constraints
- Go backend, React/Next.js frontend
- react-arborist must receive numeric `height` and `width` (not `"100%"`)
- No new external dependencies (existing DirCache pattern is sufficient for backend caching)
- Raw file endpoint must reuse existing path traversal validation (`resolveAndValidatePath`)

---

## Architecture Decisions

| ADR | File | Decision |
|---|---|---|
| ADR-001 | `project_plans/file-tree-performance/decisions/ADR-001-backend-caching-strategy.md` | Wire existing DirCache + new GitignoreCache into FileService; path-keyed, no new deps |
| ADR-002 | `project_plans/file-tree-performance/decisions/ADR-002-frontend-memoization-strategy.md` | useMemo chain treeData→displayedData→dirStatusMap; flatten handleToggle to O(1) |
| ADR-003 | `project_plans/file-tree-performance/decisions/ADR-003-file-viewer-theme-and-download.md` | CSS data-theme selectors for Shiki; dynamic CodeMirror theme; raw HTTP endpoint for download/image |

---

## Story Breakdown

### Story 1: Backend File Service Caching [1 week]
*Wire existing DirCache infrastructure into FileService; create GitignoreCache to eliminate cold disk reads.*

**User value:** Directory expand and search feel instant on repeated visits.

**Acceptance criteria:**
- Expanding the same directory twice hits the cache on the second call (no `os.ReadDir`)
- Running the same search query twice hits the gitignore cache (no `WalkDir`)
- `go test ./server/services/...` passes including new cache tests
- `make lint` passes

#### Task 1.1: Wire DirCache into FileService.ListFiles [Small — 2h]

**Objective:** Add `dirCache *DirCache` to `FileService` and wrap the `os.ReadDir` call in `ListFiles` with a Get/Put pair.

**Context boundary:**
- Primary: `server/services/file_service.go` (633 lines)
- Supporting: `server/services/dir_cache.go` (123 lines), `server/services/path_completion_service.go` (reference pattern)
- Total: ~800 lines

**Prerequisites:** Understanding of DirCache API (`Get(path) → entries, bool`; `Put(path, entries, mtime)`)

**Implementation approach:**
1. Add `dirCache *DirCache` field to `FileService` struct (line ~69)
2. Update `NewFileService` to accept and store: `NewDirCache(512, 30*time.Second)`
3. In `ListFiles`, before `os.ReadDir(fullPath)` (line ~121): check `fs.dirCache.Get(fullPath)`
4. On cache miss, call `os.ReadDir`, then `os.Stat(fullPath)` for mtime, then `fs.dirCache.Put`
5. Pass the cached/fresh entries into the existing loop (no logic change below the read)

**Validation:**
- Unit: `TestFileService_ListFiles_DirCacheHit` — second call returns without calling ReadDir
- Unit: `TestFileService_ListFiles_DirCacheMissAfterModify` — stale mtime causes miss
- Integration: existing `TestListFiles_*` tests still pass unchanged
- Success: `go test ./server/services/ -run TestFileService_ListFiles` green

---

#### Task 1.2: Create GitignoreCache [Small — 2h]

**Objective:** New `server/services/gitignore_cache.go` mirroring `dir_cache.go`'s design for `[]gitignore.Pattern`.

**Context boundary:**
- Primary: `server/services/gitignore_cache.go` (new, ~65 lines)
- Supporting: `server/services/dir_cache.go` (structural reference), `server/services/gitignore_cache_test.go` (new)
- Total: ~200 lines

**Prerequisites:** Understanding of DirCache pattern; `gitignore.Pattern` type from `go-git/v5`

**Implementation approach:**
1. Create `gitignore_cache.go` with `gitignoreCacheEntry{patterns, dirMtime, cachedAt}` and `GitignoreCache{mu, entries, maxSize, ttl}`
2. Implement `Get(key string) ([]gitignore.Pattern, bool)` — TTL + mtime check on key's directory
3. Implement `Put(key string, patterns []gitignore.Pattern, mtime time.Time)`
4. Implement `evictOldest()` — identical to DirCache
5. Write `TestGitignoreCache_Hit`, `_MissOnTTLExpiry`, `_MissOnMtimeChange`

**Validation:**
- All three test cases pass
- `go vet ./server/services/...` clean
- Success: `go test ./server/services/ -run TestGitignoreCache` green

---

#### Task 1.3: Wire GitignoreCache into FileService [Small — 2h]

**Objective:** Use GitignoreCache in both `loadGitignorePatterns` and `collectAllGitignorePatterns`.

**Context boundary:**
- Primary: `server/services/file_service.go`
- Supporting: `server/services/gitignore_cache.go` (Task 1.2)
- Total: ~700 lines

**Prerequisites:** Task 1.2 complete

**Implementation approach:**
1. Add `gitignoreCache *GitignoreCache` to `FileService`; init `NewGitignoreCache(256, 5*time.Minute)` in `NewFileService`
2. In `ListFiles` before `loadGitignorePatterns(basePath, fullPath)`:
   - Key: `basePath + ":" + fullPath`
   - On hit: use cached patterns directly
   - On miss: call `loadGitignorePatterns`, cache result with `os.Stat(basePath).ModTime()`
3. In `searchFilesInWorktree` before `collectAllGitignorePatterns(basePath)`:
   - Key: `basePath`
   - Same hit/miss pattern
4. Update `NewFileService` constructor signature (or use `FileServiceOptions` if constructor is called from tests directly)

**Validation:**
- Unit: `TestFileService_SearchFiles_GitignoreCacheHit` — second search doesn't re-walk
- Existing search/listfiles tests unchanged
- `make lint` passes
- Success: `go test ./server/services/` fully green

---

### Story 2: Frontend FileTree Performance [1 week]
*Memoize derived state, flatten handleToggle, fix ResizeObserver height/width.*

**User value:** File tree navigation feels instant; no lag on hover or parent re-renders.

**Acceptance criteria:**
- `handleToggle` no longer calls `buildTreeData` (confirmed by code review)
- Tree fills its container vertically (no 600px clip)
- `width="100%"` replaced with numeric value
- `cd web-app && npx jest --no-coverage --testPathPatterns="FileTree"` passes

#### Task 2.1: Flatten handleToggle and add useMemo chain [Small — 2h]

**Objective:** Eliminate redundant `buildTreeData` in `handleToggle`; memoize `treeData`, `displayedData`, `dirStatusMap`.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileTree.tsx` (669 lines)
- Total: ~669 lines (single file)

**Prerequisites:** Understanding of react-arborist `onToggle` semantics (fires for all directory nodes)

**Implementation approach:**
1. Add module-level `const EMPTY_GIT_STATUS_MAP = new Map<string, string>()`; replace `gitStatusMap = new Map()` default
2. Replace lines 506–510 (treeData/displayedData/dirStatusMap) with `useMemo` versions in order:
   ```tsx
   const treeData = useMemo(() => buildTreeData(dirContents.get(".") ?? [], dirContents), [dirContents]);
   const displayedData = useMemo(() => searchResults ?? treeData, [searchResults, treeData]);
   const dirStatusMap = useMemo(() => { const m = new Map<string,string>(); computeDirStatuses(displayedData, gitStatusMap, m); return m; }, [displayedData, gitStatusMap]);
   ```
3. Replace `handleToggle` (lines 522–547) with simplified version using only `dirContents.has(id)`:
   ```tsx
   const handleToggle = useCallback((id: string) => {
     if (searchResults !== null) return;
     if (!dirContents.has(id) || errorPaths.has(id)) loadDirectory(id);
   }, [dirContents, errorPaths, loadDirectory, searchResults]);
   ```
4. Remove the now-unused `rootNodes` declaration from the render body (it's computed inside the useMemo now)

**Validation:**
- Unit: existing `FileTree.test.tsx` — all pass without change
- Manual: expand directory → no lag; hover over session list while tree is open → no tree re-render
- Success: `npx jest --testPathPatterns="FileTree" --no-coverage` green; React DevTools Profiler shows no unexpected renders

---

#### Task 2.2: ResizeObserver dynamic height and width [Small — 2h]

**Objective:** Replace `height={600} width="100%"` on `<Tree>` with observed pixel dimensions.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileTree.tsx`
- Supporting: `web-app/src/components/sessions/__tests__/FileTree.test.tsx` (add ResizeObserver mock)
- Total: ~700 lines

**Prerequisites:** Task 2.1 (to avoid re-reading the same file twice)

**Implementation approach:**
1. Add `const containerRef = useRef<HTMLDivElement>(null)` and `const [dims, setDims] = useState({ w: 300, h: 600 })`
2. Add `useEffect` with `ResizeObserver`:
   ```tsx
   useEffect(() => {
     const el = containerRef.current;
     if (!el) return;
     const ro = new ResizeObserver(([e]) => {
       requestAnimationFrame(() => {
         const { width, height } = e.contentRect;
         if (width > 0 && height > 0) setDims({ w: Math.floor(width), h: Math.floor(height) });
       });
     });
     ro.observe(el);
     return () => ro.disconnect();
   }, []);
   ```
3. Attach `ref={containerRef}` to the `<div className={container}>` in the non-loading render path (line ~618)
4. Change `height={600} width="100%"` → `height={dims.h} width={dims.w}`
5. In `jest.setup.ts` (or `jest.setup.js`): add `global.ResizeObserver = class { observe(){} unobserve(){} disconnect(){} };` if not already present

**Validation:**
- Manual: open Files tab in a tall window → tree fills height; resize window → tree adjusts
- Manual: open Files tab in a narrow panel → no horizontal clip
- Unit: `npx jest --testPathPatterns="FileTree"` still passes with mock
- Success: no 600px hard-stop visible in any screen size

---

### Story 3: File Viewer Light/Dark Mode [3 days]
*Fix Shiki theme CSS activation; respect app theme in CodeMirror.*

**User value:** Files are readable in light mode. Syntax highlighting matches the app's current theme.

**Acceptance criteria:**
- Switching the app to light mode makes the file viewer use a light background and dark text
- Switching to dark mode uses dark background
- CodeMirror (large files) matches the theme
- No visual regression in dark mode (currently the only working mode)

#### Task 3.1: Fix Shiki theme switching CSS [Micro — 1h]

**Objective:** Add global CSS to `FileContentViewer.css.ts` that activates the correct Shiki dual-theme.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileContentViewer.css.ts`
- Supporting: `web-app/src/styles/theme.css.ts` (to confirm `data-theme` attribute name)
- Total: ~200 lines

**Prerequisites:** Understanding of Shiki dual-theme output format (`.shiki.github-light` / `.shiki.github-dark` CSS classes)

**Implementation approach:**
Shiki with `themes: { light, dark }` generates HTML like:
```html
<pre class="shiki shiki-themes github-light github-dark" style="--shiki-light:...; --shiki-dark:...">
```
The CSS needed to activate one theme at a time:

1. In `FileContentViewer.css.ts`, add after the `shikiOutput` block:
   ```ts
   // Light mode: show light theme, hide dark theme variables
   globalStyle(`[data-theme="light"] ${shikiOutput} .shiki`, {
     background: "var(--shiki-light-bg) !important",
     color: "var(--shiki-light) !important",
   });
   globalStyle(`[data-theme="light"] ${shikiOutput} .shiki span`, {
     color: "var(--shiki-light) !important",
   });
   // Dark mode (explicit)
   globalStyle(`[data-theme="dark"] ${shikiOutput} .shiki`, {
     background: "var(--shiki-dark-bg) !important",
     color: "var(--shiki-dark) !important",
   });
   globalStyle(`[data-theme="dark"] ${shikiOutput} .shiki span`, {
     color: "var(--shiki-dark) !important",
   });
   // System preference fallback
   globalStyle(`@media (prefers-color-scheme: light)`, {}); // handled by data-theme in practice
   ```
2. Verify the app sets `data-theme` on `<html>` (check `ThemeProvider` or layout.tsx)

**Validation:**
- Manual: toggle theme → file viewer syntax colors change immediately
- No regression: dark mode renders same as before
- Success: `make restart-web` then visual check in both modes

---

#### Task 3.2: Fix CodeMirror theme for large files [Small — 2h]

**Objective:** Make `CodeMirrorViewer` use light or dark theme based on the app's current theme setting.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileContentViewer.tsx` (lines 140–235 — CodeMirrorViewer section)
- Total: ~100 lines changed

**Prerequisites:** Task 3.1 (to confirm `data-theme` attribute mechanism works)

**Implementation approach:**
1. Create a `useAppTheme(): "light" | "dark"` hook (or inline) that reads `document.documentElement.dataset.theme` and subscribes to `MutationObserver` for changes
2. Pass `isDark: boolean` as a prop to `CodeMirrorViewer`
3. In the `CodeMirrorViewer` `useEffect`:
   - If `isDark`: import and apply `oneDark` extension (existing behavior)
   - If not `isDark`: do not apply any theme extension (CodeMirror's default is a clean light theme)
4. Re-run the effect when `isDark` changes (add to dep array); destroy and recreate the editor

**Validation:**
- Manual: open a large file (> 5000 lines) → light theme shows white background; dark shows dark
- No re-render loop when theme changes
- Success: visual check in both modes with a file > 5000 lines

---

### Story 4: File Download and Image Viewer [1 week]
*Add raw file HTTP endpoint; wire download button and inline image rendering.*

**User value:** Any file created by an LLM session is accessible — images render inline, all files downloadable with one click.

**Acceptance criteria:**
- Clicking the download button saves the file locally for any file type
- Image files (PNG, JPEG, GIF, SVG, WebP) render inline with correct colors
- Binary non-image files show the existing placeholder plus a download button
- Download respects the same path traversal protection as `GetFileContent`

#### Task 4.1: Backend raw file endpoint [Small — 2h]

**Objective:** Add `GET /api/files/raw` HTTP handler to `FileService` for serving raw file bytes.

**Context boundary:**
- Primary: `server/services/file_service.go`
- Supporting: `server/server.go` (route registration), `server/services/workspace_provider.go` (interface reference)
- Total: ~750 lines

**Prerequisites:** Understanding of existing `resolveAndValidatePath` security function

**Implementation approach:**
1. Add `ServeFileRaw` method to `FileService`:
   ```go
   func (fs *FileService) ServeFileRaw(w http.ResponseWriter, r *http.Request) {
       sessionId := r.URL.Query().Get("sessionId")
       relPath := r.URL.Query().Get("path")
       download := r.URL.Query().Get("download") == "true"
       // Validate sessionId, resolve path (reuse resolveAndValidatePath)
       // Stat file: reject > 50MB
       // Detect content type
       // If SVG: add Content-Security-Policy: sandbox
       // If download: add Content-Disposition: attachment; filename="<basename>"
       // http.ServeContent(w, r, filename, modTime, f)
   }
   ```
2. Register in `server/server.go`: `mux.HandleFunc("/api/files/raw", fileService.ServeFileRaw)`
3. Apply same `hardSkipDirs` check from `resolveAndValidatePath` (path traversal already handled)

**Validation:**
- Unit: `TestFileService_ServeFileRaw_PathTraversal` — `../../../etc/passwd` returns 400
- Unit: `TestFileService_ServeFileRaw_LargeFile` — file > 50MB returns 413
- Unit: `TestFileService_ServeFileRaw_Download` — response has `Content-Disposition: attachment`
- Success: `go test ./server/services/ -run TestFileService_ServeRaw` green; `go build .` clean

---

#### Task 4.2: Frontend download button [Micro — 1h]

**Objective:** Add a download `<a>` link to the `FileContentViewer` breadcrumb for all files.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileContentViewer.tsx` (Breadcrumb + main component sections)
- Supporting: `web-app/src/components/sessions/FileContentViewer.css.ts`
- Total: ~400 lines

**Prerequisites:** Task 4.1 complete (endpoint must exist); knowledge of `baseUrl` prop threading

**Implementation approach:**
1. Add `baseUrl` and `sessionId` props to `FileContentViewer` (or thread from existing props — check if they're already available)
2. In the `Breadcrumb` section or as a separate toolbar row, add:
   ```tsx
   <a
     href={`${baseUrl}/api/files/raw?sessionId=${sessionId}&path=${encodeURIComponent(filePath)}&download=true`}
     download
     className={downloadButton}
     title="Download file"
   >
     ↓ Download
   </a>
   ```
3. Add `downloadButton` style to `FileContentViewer.css.ts` (small pill, consistent with toolbar)
4. Show for all file states where `filePath` is set (text files, binary files, binary-rendered images)

**Validation:**
- Manual: click Download on a text file → browser saves file with correct name
- Manual: click Download on a binary file → browser prompts save dialog
- Unit: snapshot test of breadcrumb renders download link
- Success: download works for `.go`, `.png`, `.pdf` files

---

#### Task 4.3: Frontend image viewer [Small — 2h]

**Objective:** Detect image content types and render `<img>` instead of the binary placeholder.

**Context boundary:**
- Primary: `web-app/src/components/sessions/FileContentViewer.tsx` (binary rendering section, lines ~339–356)
- Supporting: `web-app/src/components/sessions/FileContentViewer.css.ts`
- Total: ~400 lines

**Prerequisites:** Task 4.1 (raw endpoint must exist for the img src); Task 4.2 (download button already added)

**Implementation approach:**
1. Add image content type detection:
   ```tsx
   const IMAGE_TYPES = new Set(["image/png","image/jpeg","image/gif","image/svg+xml","image/webp","image/bmp"]);
   const isImage = data.isBinary && IMAGE_TYPES.has(data.contentType);
   ```
2. In the binary branch, before the existing `binaryPlaceholder` render, check `isImage`:
   ```tsx
   if (isImage) {
     return (
       <div className={container}>
         <Breadcrumb ... />
         <div className={imageViewer}>
           <img
             src={`${baseUrl}/api/files/raw?sessionId=${sessionId}&path=${encodeURIComponent(filePath)}`}
             alt={filePath}
             className={imagePreview}
           />
         </div>
       </div>
     );
   }
   ```
3. Add `imageViewer` (flex center, overflow auto) and `imagePreview` (max-width: 100%, max-height: 100%) styles
4. SVG: same `<img>` tag; browser sandboxes SVG loaded via `<img>` (scripts do not execute)

**Validation:**
- Manual: open a PNG file from an LLM session → renders inline
- Manual: open an SVG → renders inline without executing scripts
- Manual: open a JPEG photo → renders inline, download button present
- Success: visual check with `.png`, `.svg`, `.jpg` files

---

## Known Issues

### Bug 001: `openAll()` fan-out on browse mode toggle
**Severity:** High (potential N concurrent API calls)
**Description:** react-arborist's `openAll()` fires `onToggle` for every internal node. If called in browse mode on an unloaded tree, `handleToggle` would fire `loadDirectory` for every unexpanded directory simultaneously.
**Mitigation:** The `if (searchResults !== null) return` guard in `handleToggle` is load-bearing — preserve it. Never add an "Expand All" button in browse mode without a depth/count guard.
**Files affected:** `web-app/src/components/sessions/FileTree.tsx:handleToggle`
**Prevention:** Add a comment at the guard explaining the invariant; add a Jest test confirming `loadDirectory` is not called when `openAll()` fires from outside search mode.

### Bug 002: gitignore cache stale after nested .gitignore edit
**Severity:** Low
**Description:** `GitignoreCache` keys on `basePath` (root mtime). Editing `src/components/.gitignore` does not advance root mtime. Cache serves stale patterns for up to 5 minutes.
**Mitigation:** Accept. Gitignore files are rarely edited during active sessions. Document TTL as the known staleness window.
**Files affected:** `server/services/file_service.go`, `server/services/gitignore_cache.go`

### Bug 003: `width="100%"` currently passed to react-window
**Severity:** Medium (already present)
**Description:** react-window `FixedSizeList` receives `width="100%"` string and silently misrenders virtual rows (incorrect clipping, wrong cell widths).
**Mitigation:** Fixed in Task 2.2 by observing container width via ResizeObserver.
**Files affected:** `web-app/src/components/sessions/FileTree.tsx:line 624`

### Bug 004: SVG served as image could execute scripts if opened directly
**Severity:** Medium
**Description:** If an SVG is opened directly in a browser tab (not via `<img>`), inline scripts execute.
**Mitigation:** Add `Content-Security-Policy: sandbox` response header on the `/api/files/raw` endpoint for SVG content types. Note: SVG loaded via `<img>` is already sandboxed by browsers.
**Files affected:** `server/services/file_service.go:ServeFileRaw` (Task 4.1)

### Bug 005: Large file CodeMirror editor does not respond to theme change while open
**Severity:** Low
**Description:** `CodeMirrorViewer` creates an EditorView on mount. If the user switches themes while viewing a large file, the editor does not update until the component remounts.
**Mitigation:** The `useEffect` re-runs when `isDark` changes (dep array includes it), which destroys and recreates the editor. This causes a brief flash. Acceptable for a developer tool.
**Files affected:** `web-app/src/components/sessions/FileContentViewer.tsx:CodeMirrorViewer`

---

## Dependency Visualization

```
Story 1 (Backend Caching)          Story 2 (Frontend Perf)
  ├── Task 1.1 DirCache wiring        ├── Task 2.1 useMemo + handleToggle
  ├── Task 1.2 GitignoreCache impl    └── Task 2.2 ResizeObserver [after 2.1]
  └── Task 1.3 Wire GitignoreCache        (same file, read once)
       [after 1.2]

Story 1 ──────────────────── INDEPENDENT ────────────────── Story 2
Both can be done in parallel on separate branches

Story 3 (Theme Fix)                Story 4 (Download + Images)
  ├── Task 3.1 Shiki CSS fix          ├── Task 4.1 Backend raw endpoint
  └── Task 3.2 CodeMirror theme       ├── Task 4.2 Download button [after 4.1]
       [after 3.1]                    └── Task 4.3 Image viewer [after 4.1 + 4.2]

Story 3 ──────────────────── INDEPENDENT ────────────────── Story 4

Story 1 (backend) is prerequisite for full performance validation but not for Stories 3/4.
```

---

## Integration Checkpoints

**After Story 1:** Run `make benchmark-tier1` — verify ListFiles bench shows cache hit path (< 1µs vs baseline). Run `go test ./server/services/` fully green.

**After Story 2:** Open a large repo's Files tab. Expand/collapse 10 directories rapidly — no lag. React DevTools Profiler shows no unexpected renders on hover.

**After Story 3:** Toggle theme while viewing a syntax-highlighted file. Light mode: white background, dark text. Dark mode: unchanged from current.

**After Story 4:** Open an LLM session that created PNG files. Files appear inline. Click Download on a `.go` file — browser saves it correctly. Click Download on a PNG — browser saves binary correctly.

**Final validation:** All four stories merged. Run `make ci`. Open a large repo (stapler-squad itself), expand 20+ directories, search for "handler", toggle theme, download a file. All interactions sub-100ms.

---

## Context Preparation Guide

### Task 1.1 (DirCache wiring)
Load: `server/services/file_service.go`, `server/services/dir_cache.go`, `server/services/path_completion_service.go` (reference for DirCache usage pattern). Concept: Cache-Aside pattern; `os.Stat` for mtime.

### Task 1.2 (GitignoreCache)
Load: `server/services/dir_cache.go` (structural template). Concept: `gitignore.Pattern` type from `go-git/v5/plumbing/format/gitignore`.

### Task 1.3 (Wire GitignoreCache)
Load: `server/services/file_service.go`, `server/services/gitignore_cache.go` (Task 1.2 output). Concept: cache key design for nested path lookups.

### Task 2.1 (useMemo chain)
Load: `web-app/src/components/sessions/FileTree.tsx`. Concept: React `useMemo` referential equality with `Map` deps; declaration order for chained memos.

### Task 2.2 (ResizeObserver)
Load: `web-app/src/components/sessions/FileTree.tsx` (after Task 2.1 edits), `web-app/src/components/terminal/XtermTerminal.tsx` (reference ResizeObserver implementation). Concept: ResizeObserver + `requestAnimationFrame` loop prevention.

### Task 3.1 (Shiki CSS)
Load: `web-app/src/components/sessions/FileContentViewer.css.ts`, `web-app/src/styles/theme.css.ts`. Concept: Shiki dual-theme CSS variables (`--shiki-light`, `--shiki-dark`); vanilla-extract `globalStyle`.

### Task 3.2 (CodeMirror theme)
Load: `web-app/src/components/sessions/FileContentViewer.tsx` (CodeMirrorViewer section). Concept: `MutationObserver` for `data-theme` attribute changes; CodeMirror `EditorView.destroy()`.

### Task 4.1 (Raw endpoint)
Load: `server/services/file_service.go`, `server/server.go`. Concept: `http.ServeContent` for range requests; `resolveAndValidatePath` security primitive; Content-Security-Policy sandbox for SVG.

### Task 4.2 (Download button)
Load: `web-app/src/components/sessions/FileContentViewer.tsx`, `web-app/src/components/sessions/FileContentViewer.css.ts`. Concept: `<a download>` attribute pattern.

### Task 4.3 (Image viewer)
Load: `web-app/src/components/sessions/FileContentViewer.tsx`. Concept: Content-type based rendering dispatch; `<img>` SVG sandboxing behavior.

---

## Success Criteria

- [ ] All 9 atomic tasks completed and green
- [ ] `make ci` passes (proto check, web build, Go build, tests, lint)
- [ ] ListFiles benchmark shows cache hit path
- [ ] File viewer renders correctly in both light and dark mode
- [ ] Images render inline; download button works for all file types
- [ ] `make lint` and `go vet ./...` clean
- [ ] No new dependencies added to go.mod or package.json
