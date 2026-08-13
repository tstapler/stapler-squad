# File Viewer Feature Plan

## Epic Overview

### User Value Statement

A solo developer reviewing LLM session output in stapler-squad needs to browse the full file tree of a session's worktree and view individual file contents with syntax highlighting, without leaving the browser. Currently, understanding what an LLM session changed requires switching to an IDE. This feature adds a "Files" tab to SessionDetail that provides a read-only file browser with git status annotations, closing the review loop entirely within the web UI.

### Success Criteria

- Browse the full file tree of any session's worktree without opening an IDE
- View any text file with syntax highlighting appropriate to its language
- Git-modified files display colored status badges (M/A/D/?) in the file tree
- File tree supports search/filter by filename or path substring
- Feature integrates as a natural tab inside existing SessionDetail panel
- Binary files show a placeholder; files over 1MB degrade highlighting gracefully; files over 10MB are rejected

### Scope

- **In Scope**: File tree view, file content viewer with syntax highlighting, git status badges, file tree search/filter, gitignore filtering with toggle, binary/large file handling, "Files" tab in SessionDetail
- **Out of Scope**: File editing/writing, commit operations, cross-session file comparison, server-side syntax highlighting, directory diffing, inline diff within the file viewer

## Architecture Decisions

References to full ADR files:
- ADR-001: Syntax highlighting strategy -- `project_plans/claude-squad-file-viewer/decisions/ADR-001-syntax-highlighting.md`
- ADR-002: File tree component selection -- `project_plans/claude-squad-file-viewer/decisions/ADR-002-file-tree-component.md`
- ADR-003: Git status integration pattern -- `project_plans/claude-squad-file-viewer/decisions/ADR-003-git-status-integration.md`

### Summary of Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Syntax highlighting | Shiki (`@shikijs/react`) with CodeMirror 6 fallback for >5k lines | TextMate grammar quality, CSS variable dual-theme, fine-grained imports |
| File tree component | react-arborist | Virtualized, headless render-prop, `disableDrag`/`disableDrop`, native `searchMatch` |
| Git status join | Client-side parallel join (ListFiles + GetVCSStatus fired simultaneously) | Decouples concerns, enables client-side VCS caching across directory expands |
| Directory loading | Lazy per-directory (one RPC per expand) | Scales to any repo size; matches VS Code/GitHub pattern |
| Gitignore library | `go-git/v5/plumbing/format/gitignore` (already in go.mod) | Zero new dependencies |
| Symlinks | Report via `is_symlink`/`symlink_target`, never follow | Prevents path traversal and cycle attacks |
| Binary detection | Extension allowlist + `http.DetectContentType` + null-byte scan | Layered approach, stdlib only |
| Proto schema shape | Flat `FileNode` list per directory request | Wire-efficient, no recursive nesting |

## Dependency Visualization

```
Story 1 (Backend)
  |
  +---> Story 2 (Frontend: File Tree)
  |       |
  |       +---> Story 3 (Frontend: File Viewer + Git Integration)
  |               |
  |               +---> Story 4 (Polish: Search, Gitignore Toggle, Binary Handling)
```

All stories are sequential. Story 2 depends on Story 1's RPC endpoints. Story 3 depends on Story 2's tree component for file selection. Story 4 depends on Story 3's viewer infrastructure for edge-case handling.

---

## Story 1: Backend -- ListFiles and GetFileContent RPCs

### Goal

Define protobuf messages, implement server-side handlers for directory listing and file content retrieval with full defensive handling (gitignore, symlinks, binary detection, size limits, path traversal protection, permission errors).

### Correction from Research

The research documents state that `ListFiles`, `GetFileContent`, and `FileNode` already exist in the proto and server. **This is incorrect.** As of the current codebase, these RPCs and messages do NOT exist in `proto/session/v1/session.proto`, `proto/session/v1/types.proto`, or `server/services/session_service.go`. They must be created from scratch.

### User Stories

**US-1.1**: As a web UI client, I can call `ListFiles(session_id, path)` and receive the immediate children of a directory as flat `FileNode` entries.

Acceptance Criteria:
- Given a valid session_id and path ".", When I call ListFiles, Then I receive a `ListFilesResponse` with `FileNode` entries for each immediate child (files and directories)
- Given a path containing symlinks, Then symlinked entries have `is_symlink=true` and `symlink_target` populated, and symlinked directories are reported as non-directories (no expansion)
- Given a directory with >10,000 entries, Then the response is capped and `truncated=true` is set
- Given a path like `../../etc/passwd`, Then the request is rejected (path traversal check)

**US-1.2**: As a web UI client, I can call `GetFileContent(session_id, path)` and receive the file content with metadata (size, content_type, is_binary).

Acceptance Criteria:
- Given a text file under 1MB, Then I receive the full content as a UTF-8 string with `content_type` set
- Given a binary file (detected by extension, MIME sniffing, or null-byte scan), Then `is_binary=true` is set and content is empty
- Given a file over 10MB, Then the request returns an error with a human-readable message (not HTTP 500)
- Given a file that was deleted between tree listing and content fetch, Then I receive a `NOT_FOUND` error

**US-1.3**: As a web UI client, I can toggle `include_ignored=true` on ListFiles to see gitignored files, which are annotated with `is_ignored=true`.

Acceptance Criteria:
- Given `include_ignored=false` (default), Then gitignored files and directories are excluded from the response
- Given `include_ignored=true`, Then gitignored files appear with `is_ignored=true`
- Given `.git/`, `node_modules/`, `vendor/`, Then these are always excluded regardless of `include_ignored`

### Tasks

**Task 1.1: Proto schema additions**

Add new RPC methods to `proto/session/v1/session.proto`:
```protobuf
rpc ListFiles(ListFilesRequest) returns (ListFilesResponse) {}
rpc GetFileContent(GetFileContentRequest) returns (GetFileContentResponse) {}
```

Add new messages to `proto/session/v1/types.proto`:
```protobuf
message FileNode {
  string name = 1;
  string path = 2;         // relative to session root
  bool is_dir = 3;
  int64 size = 4;
  string git_status = 5;   // populated client-side, left empty by server
  bool is_symlink = 6;
  string symlink_target = 7;
  bool is_ignored = 8;     // true if matched by .gitignore
}

message ListFilesRequest {
  string session_id = 1;
  string path = 2;
  bool include_ignored = 3;
}

message ListFilesResponse {
  repeated FileNode files = 1;
  string base_path = 2;
  bool truncated = 3;
  int32 total_count = 4;
}

message GetFileContentRequest {
  string session_id = 1;
  string path = 2;
}

message GetFileContentResponse {
  string content = 1;
  string encoding = 2;
  bool is_binary = 3;
  int64 size = 4;
  string content_type = 5;
  bool is_truncated = 6;
}
```

Run `make generate-proto` to regenerate Go and TypeScript code.

Files touched:
- `proto/session/v1/session.proto`
- `proto/session/v1/types.proto`

**Task 1.2: ListFiles handler implementation**

Create `server/services/file_service.go` following the sub-service delegation pattern used by `WorkspaceService`, `ConfigService`, etc.

Key implementation details:
- Resolve session path via `findInstance(sessionId)` using the same pattern as other RPCs
- Path traversal check: `filepath.Clean(joined)` must have `filepath.Clean(basePath)` as prefix
- Use `os.ReadDir(fullPath)` (not `os.WalkDir` -- lazy per-directory, not recursive)
- Symlink detection: `entry.Type()&os.ModeSymlink != 0`; populate `is_symlink`/`symlink_target` via `os.Readlink`; report symlinked dirs as `is_dir=false`
- Gitignore filtering: load `.gitignore` files at the requested directory level plus ancestors using `go-git/v5/plumbing/format/gitignore.ReadPatterns`; match each entry
- Hardcoded skip list: `.git`, `node_modules`, `vendor`, `.tox`, `__pycache__`, `target`, `.gradle`, `dist`, `build`
- Node cap: 10,000 entries, set `truncated=true` if exceeded
- Permission errors: `os.IsPermission(err)` -> skip entry gracefully, do not fail request
- Sort: directories first, then alphabetical within each group

Files touched:
- `server/services/file_service.go` (new)

**Task 1.3: GetFileContent handler implementation**

Implement `GetFileContent` in `server/services/file_service.go`.

Key implementation details:
- Path traversal check (same as ListFiles)
- File size check: `os.Stat` first; reject >10MB with `connect.CodeFailedPrecondition`
- Binary detection (3-layer):
  1. Extension allowlist (known text extensions -> skip binary check)
  2. `http.DetectContentType` on first 512 bytes
  3. Null-byte scan on first 8000 bytes
- Text files >1MB: serve first 1MB, set `is_truncated=true`
- `os.IsNotExist` -> `connect.CodeNotFound`
- Never leak `os.PathError` details to client

Files touched:
- `server/services/file_service.go`

**Task 1.4: Wire up and test**

- Add `fileSvc *FileService` field to `SessionService` struct
- Instantiate in `NewSessionService` constructor
- Add delegation methods `ListFiles()` and `GetFileContent()` on `SessionService`
- Write unit tests covering: path traversal, gitignore, symlinks, binary detection, size limits, permissions, node cap

Files touched:
- `server/services/session_service.go`
- `server/services/file_service.go`
- `server/services/file_service_test.go` (new)

### Integration Checkpoint 1

```bash
make build && make test
go test ./server/services -run TestFileService -v
```

Verify with a running server using `grpcurl` or test client: directory listing, gitignore filtering, binary detection, size limits, path traversal rejection.

---

## Story 2: Frontend -- File Tree Component

### Goal

Add a "Files" tab to SessionDetail that renders a virtualized, read-only file tree using react-arborist. Each directory expand triggers a `ListFiles` RPC call.

### User Stories

**US-2.1**: As a user viewing a session, I see a "Files" tab alongside terminal/diff/vcs/logs/info. Clicking it shows the root-level directory listing.

Acceptance Criteria:
- Given I click the "Files" tab, Then I see the root-level files and directories
- Given the ListFiles call is in progress, Then I see a loading indicator
- Given the ListFiles call fails, Then I see an error message with a retry button

**US-2.2**: As a user, I can expand directories to lazily load their children.

Acceptance Criteria:
- Given I click a directory chevron, Then children load via ListFiles RPC
- Given a directory has been expanded before, Then re-collapsing and re-expanding uses cached data
- Given a symlinked directory, Then it shows a symlink icon and cannot be expanded

**US-2.3**: As a user, I can click a file to select it (visual highlight, preparing for content viewing).

### Tasks

**Task 2.1: Install react-arborist dependency**

```bash
cd web-app && npm install react-arborist
```

Files touched:
- `web-app/package.json`
- `web-app/package-lock.json`

**Task 2.2: Create ConnectRPC client hooks**

Create `web-app/src/lib/hooks/useFileService.ts`:
- `useListFiles(sessionId, path, includeIgnored)` -- returns `{ data, loading, error, refetch }`
- `useGetFileContent(sessionId, path)` -- returns `{ data, loading, error }`
- Follow pattern from `useSessionService.ts`

Files touched:
- `web-app/src/lib/hooks/useFileService.ts` (new)

**Task 2.3: Create FileTree component**

Create `web-app/src/components/sessions/FileTree.tsx` and `FileTree.module.css`.

Implementation:
- Props: `sessionId`, `baseUrl`, `onFileSelect: (path: string) => void`
- react-arborist `<Tree>` with `disableDrag={true}`, `disableDrop={true}`
- Custom node renderer: chevron for dirs, file/folder/symlink icons, node name
- State: `Map<string, FileNode[]>` for lazy-loaded children keyed by directory path
- Root: call ListFiles on mount for "."
- Expand: call ListFiles for subdirectory path
- Loading spinner per-directory while RPC in-flight
- Error handling: inline error per directory
- Sorting: server provides (directories first, alphabetical)
- Indent guides: CSS `border-left` at each nesting level

Files touched:
- `web-app/src/components/sessions/FileTree.tsx` (new)
- `web-app/src/components/sessions/FileTree.module.css` (new)

**Task 2.4: Add "Files" tab to SessionDetail**

Modify `SessionDetail.tsx`:
- Add `"files"` to `SessionDetailTab` union type: `"terminal" | "diff" | "vcs" | "logs" | "info" | "files"`
- Add tab entry: `{ id: "files", label: "Files", icon: "📁" }`
- Add `"files"` to fullscreen-eligible tabs check
- Render `<FileTree>` when `activeTab === "files"`

Files touched:
- `web-app/src/components/sessions/SessionDetail.tsx`

### Integration Checkpoint 2

```bash
make restart-web
```
- Open web UI, select a session, click "Files" tab
- Root directory listing appears
- Expand a directory, children load
- Symlinks show icon, not expandable
- Loading spinners visible during RPC calls

---

## Story 3: Frontend -- File Content Viewer + Git Status Badges

### Goal

Display file content with syntax highlighting when a file is selected. Annotate tree nodes with git status badges.

### User Stories

**US-3.1**: As a user, when I click a file in the tree, its content renders with syntax highlighting.

Acceptance Criteria:
- `.go` file renders with Go highlighting; `.md` with Markdown; `.tsx` with TypeScript JSX
- Loading skeleton shown while content fetches
- Files >5,000 lines use CodeMirror 6 read-only mode (virtualized)
- Binary files show placeholder with size and content type
- Truncated files show warning bar

**US-3.2**: As a user, I see colored git status badges on modified files in the tree.

Acceptance Criteria:
- Modified=yellow M, Added=green A, Deleted=red D, Untracked=teal ?
- Badges right-aligned on tree row (VSCode pattern)
- Directory shows status dot if any child is modified
- VCS status cached 5s, manual refresh available

**US-3.3**: As a user, clicking a file path in VcsPanel navigates to it in the Files tab.

### Tasks

**Task 3.1: Install Shiki and CodeMirror 6**

```bash
cd web-app && npm install @shikijs/react shiki
cd web-app && npm install @codemirror/view @codemirror/state @codemirror/lang-javascript @codemirror/lang-python @codemirror/lang-go @codemirror/lang-markdown @codemirror/lang-json @codemirror/lang-html @codemirror/lang-css @codemirror/lang-rust @codemirror/lang-java @codemirror/theme-one-dark
```

Files touched:
- `web-app/package.json`
- `web-app/package-lock.json`

**Task 3.2: Create FileContentViewer component**

Create `web-app/src/components/sessions/FileContentViewer.tsx` and `FileContentViewer.module.css`.

Implementation:
- Props: `sessionId`, `filePath`, `baseUrl`
- Fetch content via `useGetFileContent` hook on mount/path change
- Language detection: file extension -> Shiki language ID mapping
- Shiki rendering:
  - `useEffect` lazy init with `@shikijs/react` pattern
  - Dual theme: `themes: { light: 'github-light', dark: 'github-dark' }` via CSS variables
  - Load only needed languages on first render
- Line numbers (Shiki built-in)
- Large file fallback: >5,000 lines -> CodeMirror 6 with `EditorView.editable.of(false)`
- Binary placeholder: centered "Binary file -- cannot display" with size and content_type
- Truncation notice: warning bar "File truncated to 1MB"
- Breadcrumb: full file path, each segment clickable
- Error: inline with retry button
- Shiki init failure fallback: plain `<pre>` with no highlighting

Files touched:
- `web-app/src/components/sessions/FileContentViewer.tsx` (new)
- `web-app/src/components/sessions/FileContentViewer.module.css` (new)

**Task 3.3: Create FilesTab container with split layout**

Create `web-app/src/components/sessions/FilesTab.tsx` and `FilesTab.module.css`.

Layout:
- Left pane: `FileTree` (~30% width, resizable)
- Right pane: `FileContentViewer` (fills remaining)
- State: `selectedFilePath` managed here
- If no file selected: empty state "Select a file to view its contents"

Update `SessionDetail.tsx` to render `<FilesTab>` instead of bare `<FileTree>`.

Files touched:
- `web-app/src/components/sessions/FilesTab.tsx` (new)
- `web-app/src/components/sessions/FilesTab.module.css` (new)
- `web-app/src/components/sessions/SessionDetail.tsx`

**Task 3.4: Git status badge integration**

In `FilesTab.tsx`:
- Fire `GetVCSStatus(sessionId)` in parallel with initial ListFiles
- Build `Map<string, FileStatus>` from staged/unstaged/untracked/conflict files
- Pass map to FileTree as `gitStatusMap` prop
- In FileTree node renderer: lookup path, render colored badge:
  - M=`#cca700`, A=`#2ea043`, D=`#f85149`, ?=`#3fb950`, R=cyan
- Directory propagation: dot if any descendant has status
- Cache VCS status 5s; add manual refresh button

Files touched:
- `web-app/src/components/sessions/FilesTab.tsx`
- `web-app/src/components/sessions/FileTree.tsx`

**Task 3.5: VcsPanel cross-link to Files tab**

Modify `VcsPanel.tsx`:
- Make file paths clickable
- Add `onNavigateToFile?: (path: string) => void` prop
- In `SessionDetail.tsx`: handle callback by switching to "files" tab and setting `selectedFilePath`

Files touched:
- `web-app/src/components/sessions/VcsPanel.tsx`
- `web-app/src/components/sessions/SessionDetail.tsx`

### Integration Checkpoint 3

```bash
make restart-web
```
- Expand directories, click .go file -> Go syntax highlighting
- Git status badges visible (M yellow, A green, D red, ? teal)
- VCS tab file click -> switches to Files tab showing that file
- Binary file -> placeholder; >5k lines -> CodeMirror fallback; truncated -> warning bar

---

## Story 4: Polish -- Search, Gitignore Toggle, Edge Cases

### Goal

File tree search/filter, gitignore visibility toggle, Collapse All, expand state persistence, edge-case handling.

### User Stories

**US-4.1**: Search box filters tree by filename with ancestor auto-expansion.

**US-4.2**: Toggle "Show ignored files" reveals gitignored files (dimmed).

**US-4.3**: "Collapse All" button collapses tree to root level.

### Tasks

**Task 4.1: File tree search/filter**

- Add `<input type="search">` pinned above tree
- Wire `searchTerm` to `<Tree searchTerm={searchTerm} searchMatch={...}>`
- `searchMatch`: case-insensitive substring on name and path
- react-arborist handles ancestor auto-expansion
- Save pre-search openState; restore on clear
- Highlight matched chars with `<mark>` in node renderer
- Keyboard: Cmd+F / Ctrl+F focuses search input (prevent browser find)
- Debounce 150ms

Files touched: `FileTree.tsx`

**Task 4.2: Gitignore toggle**

- Toggle/checkbox in FileTree toolbar: "Show ignored" (default off)
- Toggle on: refetch expanded directories with `include_ignored=true`
- Toggle off: refetch with `include_ignored=false`
- Ignored files: `opacity: 0.5`, italic
- State: per-session, not persisted

Files touched: `FileTree.tsx`

**Task 4.3: Collapse All button**

- Button in toolbar with collapse icon
- Calls react-arborist `tree.closeAll()` API
- Placed to right of search input

Files touched: `FileTree.tsx`

**Task 4.4: Expand state persistence within session**

- Store expanded node IDs in `Set<string>` state in `FilesTab`
- Preserved across tab switches (away from "files" and back)
- Reset on session change (different session selected)
- Use react-arborist `initialOpenState`

Files touched: `FilesTab.tsx`

**Task 4.5: Edge case polish and loading states**

- Empty directory: "This directory is empty" message
- Permission denied: lock icon + "Access denied" tooltip
- Truncated response: warning banner "10,000+ files -- not all shown"
- Network errors: inline retry buttons per directory and per file
- Loading skeletons: pulsing placeholders
- Keyboard navigation: verify react-arborist ArrowUp/Down/Left/Right

Files touched: `FileTree.tsx`, `FileContentViewer.tsx`, `FilesTab.tsx`

### Integration Checkpoint 4

```bash
make restart-web
```
- Search filters tree, ancestors expand, matches highlighted
- Clear search restores prior state
- Show ignored toggle works (dimmed files appear/disappear)
- Collapse All works
- Tab switch preserves expand state
- Large repo: node_modules skipped
- Cmd+F focuses search, not browser find

---

## Known Issues / Potential Bugs

### BUG-001: Race Condition -- File Deleted Between Tree and Content Fetch [SEVERITY: Medium]

**Description**: A file listed by ListFiles may be deleted by the time GetFileContent is called (active LLM session modifying files concurrently).

**Mitigation**:
- Server: `os.IsNotExist(err)` -> `connect.CodeNotFound` with "File no longer exists"
- Client: catch NOT_FOUND, show "File was deleted since last refresh" with refresh button

**Files Likely Affected**: `server/services/file_service.go`, `FileContentViewer.tsx`

### BUG-002: Path Traversal via Encoded Slashes or Symlinks [SEVERITY: High]

**Description**: Malformed paths (`../../../etc/passwd`, URL-encoded slashes, symlinks outside worktree) could read arbitrary host files.

**Mitigation**:
- `filepath.Clean(joined)` must have worktree root as prefix
- Never follow symlinks into directories
- Reject any path containing `..` segments after cleaning
- `os.Readlink` reports target but content fetch verifies file is within worktree

**Files Likely Affected**: `server/services/file_service.go`

### BUG-003: OOM on Large Binary File Serve [SEVERITY: High]

**Description**: `os.ReadFile` on a multi-GB file exhausts server memory.

**Mitigation**:
- `os.Stat` before read; reject >10MB
- Files 1-10MB: serve first 1MB with `is_truncated=true`
- Binary check on first 512 bytes only

**Files Likely Affected**: `server/services/file_service.go`

### BUG-004: Shiki WASM Initialization Failure [SEVERITY: Medium]

**Description**: Shiki WASM (~1.5MB) may fail on slow connections or restrictive CSP.

**Mitigation**:
- Wrap init in try/catch; fall back to plain `<pre>` rendering
- Loading spinner with 5s timeout
- Cache highlighter in React context to avoid re-init on tab switch

**Files Likely Affected**: `FileContentViewer.tsx`

### BUG-005: Stale VCS Status Cache [SEVERITY: Low]

**Description**: 5-second cache may show stale badges during active LLM sessions.

**Mitigation**:
- Manual refresh button
- Consider 2s TTL for running sessions
- Cosmetic only; no functional impact

**Files Likely Affected**: `FilesTab.tsx`

### BUG-006: react-arborist Node ID Collision [SEVERITY: Medium]

**Description**: If node IDs are not unique, expand/select state collides.

**Mitigation**:
- Use full relative path as node ID (unique within worktree), not filename

**Files Likely Affected**: `FileTree.tsx`

### BUG-007: Gitignore Pattern Performance on Deep Repos [SEVERITY: Low]

**Description**: Many nested `.gitignore` files could slow directory listing.

**Mitigation**:
- Compile patterns once per file; cache compiled matchers
- Lazy per-directory loading bounds cost to one parse per user action
- Hardcoded skip list prevents entering expensive directories

**Files Likely Affected**: `server/services/file_service.go`

---

## New File Summary

| File | Type | Story |
|---|---|---|
| `proto/session/v1/session.proto` | Modified | 1 |
| `proto/session/v1/types.proto` | Modified | 1 |
| `server/services/file_service.go` | New | 1 |
| `server/services/file_service_test.go` | New | 1 |
| `server/services/session_service.go` | Modified | 1 |
| `web-app/src/lib/hooks/useFileService.ts` | New | 2 |
| `web-app/src/components/sessions/FileTree.tsx` | New | 2, 3, 4 |
| `web-app/src/components/sessions/FileTree.module.css` | New | 2, 3, 4 |
| `web-app/src/components/sessions/FileContentViewer.tsx` | New | 3, 4 |
| `web-app/src/components/sessions/FileContentViewer.module.css` | New | 3 |
| `web-app/src/components/sessions/FilesTab.tsx` | New | 3, 4 |
| `web-app/src/components/sessions/FilesTab.module.css` | New | 3 |
| `web-app/src/components/sessions/SessionDetail.tsx` | Modified | 2, 3 |
| `web-app/src/components/sessions/VcsPanel.tsx` | Modified | 3 |

## Testing Strategy

### Unit Tests (server/services/file_service_test.go)

- Path traversal rejection
- Gitignore filtering (matched excluded/included based on flag)
- Hardcoded skip list enforcement
- Symlink handling (detected, not followed for directories)
- Binary detection (extension, MIME, null bytes)
- Size limits (>10MB rejected, >1MB truncated)
- Permission error handling
- Node cap truncation
- Empty directory handling
- Non-existent path returns NOT_FOUND

### Integration Tests (manual via `make restart-web`)

- Full flow: Files tab -> expand -> click file -> content renders
- Git status badges correct
- VCS cross-link navigation
- Search, Collapse All, gitignore toggle
- Large file CodeMirror fallback
- Binary placeholder
- Empty state for sessions without worktree

### Performance Considerations

- Shiki WASM loads once, cached across renders
- react-arborist virtualizes (only visible nodes in DOM)
- Lazy per-directory loading prevents upfront cost
- VCS status cached client-side across expands
- CodeMirror 6 virtualizes for files >5k lines
