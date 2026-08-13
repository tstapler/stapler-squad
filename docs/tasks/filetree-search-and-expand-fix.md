# FileTree: Fix Directory Expand and Nested File Search

## Status: COMPLETE

**Branch**: claude-squad-fix-filesystem-nested
**Stories**: 3 / 3 complete
**Tasks**: 10 / 10 complete
**Key commits**:
- `c8fbb64` - docs: Add feature plan
- `cc65029` - feat(filetree): add backend SearchFiles RPC and frontend search mode with auto-expand (Stories 1 + 2 + Tasks 3.1-3.2)
- `efa27c0` - fix(filetree): propagate ctx to search walk and restore browse state on search exit (Task 3.3 + ctx fix)

**Go tests**: 218 passing (server/services); SearchFiles tests: 8 passing
**Remaining work**: End-to-end smoke test, then open PR to main

---

## Epic Overview

### User Value Statement

A developer using the Files tab in Stapler Squad's web UI cannot search for files nested inside collapsed directories. The search only matches files that have already been loaded via directory expansion. Additionally, when search results exist inside collapsed directories, those directories do not auto-expand to reveal the matches. This makes the search bar effectively useless for discovering files in a project -- the user must manually expand every directory first, defeating the purpose of search.

### Success Criteria

- Searching for a filename returns matches from the entire worktree, not just loaded directories
- When a search term matches files inside collapsed directories, their ancestor directories auto-expand to reveal them
- Directory click to expand/collapse continues to work with lazy loading for non-search mode
- Performance remains acceptable for repositories with up to 10,000 files
- No new backend dependencies; the existing `ListFiles` RPC is reused

### Scope

- **In Scope**: Backend recursive search RPC, frontend search integration with auto-expand, directory expand/collapse correctness
- **Out of Scope**: Fuzzy search, regex search, file content search (grep), restructuring the lazy-load architecture for non-search browsing

### Constraints

- react-arborist v3.4.3 is the tree component; its `searchTerm`/`searchMatch` props only filter nodes already present in the data tree
- The backend `ListFiles` RPC returns only immediate children of a directory (non-recursive by design)
- The proto schema (`session.proto`, `types.proto`) is shared across Go and TypeScript via codegen -- changes require `make generate-proto`

---

## Root Cause Analysis

### Problem 1: Search cannot find files in unloaded directories

`FileTree.tsx` uses react-arborist's `searchTerm` and `searchMatch` props (lines 448-455). react-arborist filters only nodes present in its `data` prop. Directories that have never been expanded have `children: []` (set by `buildTreeData` line 82), meaning their real children don't exist in the tree data. The search therefore has zero visibility into unexpanded directories.

### Problem 2: No auto-expand for search matches

react-arborist's `searchMatch` controls which nodes are visible in the filtered view, but it does not programmatically open parent nodes. Even if a parent directory happened to be loaded (present in `dirContents`), a matching child inside a collapsed directory would not cause the directory to open.

### Problem 3: onToggle lazy-load is correct but insufficient for search

The `handleToggle` callback (line 360-381) correctly calls `loadDirectory` when a directory is expanded for the first time. This pattern works well for manual browsing but is architecturally incompatible with search -- search needs all data upfront or a separate search pathway.

---

## Architecture Decisions

### ADR: Search via backend recursive endpoint vs. frontend full-tree preload

**Context**: The current frontend search operates on the client-side tree data. To search nested files, we need either (a) a backend search/find RPC that returns matching paths, or (b) eagerly loading the entire tree on the frontend.

**Decision**: Add a `SearchFiles` RPC to the backend that performs recursive file discovery with name matching, returning a flat list of matching `FileNode` entries with their full relative paths. The frontend uses these results to populate the tree and auto-expand ancestor directories.

**Rationale**:
- Eagerly loading the full tree on the frontend would require N recursive `ListFiles` calls (one per directory) or a new "recursive list all" RPC that could return tens of thousands of nodes. For large repos, this is prohibitively slow and memory-intensive.
- A backend search RPC with a name filter keeps the response small (only matching files + their ancestor paths), leverages Go's filesystem performance (`filepath.WalkDir`), and respects the existing gitignore/hardSkip filtering.
- The search RPC can be debounced on the frontend (300ms) to avoid excessive calls during typing.

**Consequences**:
- One new RPC method and two new protobuf messages
- The frontend must merge search results into the lazy-loaded tree structure
- Two distinct modes in the FileTree: browse mode (lazy-load per directory) and search mode (flat results from backend)

---

## Dependency Visualization

```
Story 1 (Backend: SearchFiles RPC)
  |
  +---> Story 2 (Frontend: Search integration + auto-expand)
          |
          +---> Story 3 (Polish: debounce, result count, clear behavior)
```

Story 2 depends on Story 1's RPC endpoint. Story 3 depends on Story 2's wiring.

---

## Story 1: Backend -- SearchFiles RPC [COMPLETE]

### Goal

Add a `SearchFiles` RPC that recursively walks a session's worktree and returns files matching a name substring filter, along with their ancestor directory paths so the frontend can build an expanded tree.

### User Stories

**US-1.1**: As a web UI client, I can call `SearchFiles(session_id, query)` and receive a flat list of `FileNode` entries whose names or paths contain the query substring.

Acceptance Criteria:
- Given a query "main", When the worktree contains `src/main.go` and `cmd/main_test.go`, Then both are returned with their full relative paths
- Given a query that matches 0 files, Then an empty list is returned (not an error)
- Given a query with fewer than 2 characters, Then an empty list is returned (guard against overly broad searches)
- Given a worktree with `node_modules/`, `.git/`, `vendor/`, Then those directories are never traversed
- Given `include_ignored=false`, Then gitignored files are excluded from results
- Given a result set exceeding 500 entries, Then the response is capped with `truncated=true`

### Tasks

**Task 1.1: Proto schema additions** [COMPLETE - commit cc65029]

Add new messages and RPC to the proto files.

Files touched (2):
- `proto/session/v1/session.proto` -- add `rpc SearchFiles` method, add `SearchFilesRequest` and `SearchFilesResponse` messages
- `proto/session/v1/types.proto` -- no changes needed (reuses existing `FileNode`)

Proto additions:
```protobuf
// In session.proto, inside service SessionService:
rpc SearchFiles(SearchFilesRequest) returns (SearchFilesResponse) {}

// New messages (in session.proto alongside other file viewer messages):
message SearchFilesRequest {
  string session_id = 1;
  string query = 2;           // substring match against file/dir name and path
  bool include_ignored = 3;
  int32 max_results = 4;      // 0 = use server default (500)
}

message SearchFilesResponse {
  repeated FileNode files = 1;   // matching files with full relative paths
  bool truncated = 2;            // true if results were capped
  int32 total_matches = 3;       // total before cap
}
```

Run `make generate-proto` after changes.

**Task 1.2: SearchFiles handler implementation** [COMPLETE - commit cc65029]

Add the `SearchFiles` method to `FileService` in the existing file service file.

Key implementation details:
- Use `filepath.WalkDir` for recursive traversal (efficient, does not follow symlinks by default)
- Reuse `hardSkipDirs` map to skip `.git`, `node_modules`, etc. (return `fs.SkipDir` in the walk callback)
- Reuse `loadGitignorePatterns` for gitignore filtering (load patterns at each directory level during walk)
- Case-insensitive substring match: `strings.Contains(strings.ToLower(name), strings.ToLower(query))`
- Also match against the full relative path for queries like "src/main"
- Cap results at 500 (configurable via `max_results`, server default 500)
- Return both files and matching directories (directories are useful for path-based queries)
- Path traversal check: reuse `resolveAndValidatePath`
- Minimum query length: 2 characters (return empty for shorter queries)
- Collect ancestor directory paths for each match (split the relative path and include each prefix) -- this enables the frontend to build the tree structure

Files touched (1):
- `server/services/file_service.go`

**Task 1.3: Wire up and register handler** [COMPLETE - commit cc65029]

The FileService is already instantiated and wired into SessionService. The new `SearchFiles` method on `FileService` needs a delegation method on `SessionService`.

Files touched (1):
- `server/services/session_service.go` -- add `SearchFiles` delegation method

**Task 1.4: Unit tests** [COMPLETE - commit cc65029; 8 tests passing]

Files touched (1):
- `server/services/file_service_test.go` -- add tests following the existing pattern (`testFileService` with `listFiles`/`getFileContent` helpers)

Test cases:
- Search matches files in nested directories (create `a/b/c/target.go`, search "target")
- Search respects `hardSkipDirs` (create `node_modules/foo.js`, search "foo", expect no match)
- Search respects gitignore (create `.gitignore` with `*.tmp`, create `x.tmp`, search "x.tmp", expect no match)
- Search returns empty for query shorter than 2 characters
- Search caps results at max_results
- Search with no matches returns empty list, not error
- Path traversal in session_id is rejected

### Integration Checkpoint 1

```bash
make generate-proto
make build && go test ./server/services -run TestSearchFiles -v
```

Verify: `SearchFiles` RPC responds correctly via manual curl or grpcurl.

---

## Story 2: Frontend -- Search integration with auto-expand [COMPLETE]

### Goal

Replace the current client-side-only search filtering with a search mode that calls the new `SearchFiles` RPC and displays results in the tree with ancestor directories auto-expanded.

### User Stories

**US-2.1**: As a user, when I type a search term (2+ chars) in the filter input, the tree shows matching files from the entire worktree with their ancestor directories expanded.

Acceptance Criteria:
- Given I type "main" and the worktree has `src/cmd/main.go`, Then the tree shows `src/` > `cmd/` > `main.go` with both directories expanded
- Given search results span multiple directories, Then all ancestor paths are expanded
- Given I clear the search field, Then the tree returns to its previous browse state (collapsed directories restored)
- Given fewer than 2 characters in the search field, Then no search RPC is fired and browse mode is active

**US-2.2**: As a user, matched file names are highlighted in the search results.

Acceptance Criteria:
- The existing `highlightMatch` function continues to work with search results
- Both filename and path matches show the highlighted substring

### Tasks

**Task 2.1: Add `searchFiles` fetch function** [COMPLETE - commit cc65029]

Add a standalone async function to the file service hook file, following the pattern of `fetchDirectoryFiles`.

Files touched (1):
- `web-app/src/lib/hooks/useFileService.ts` -- add `searchFiles(sessionId, query, includeIgnored, baseUrl)` function that calls the `SearchFiles` RPC

**Task 2.2: Implement search mode in FileTree component** [COMPLETE - commit cc65029]

This is the core change. The FileTree component needs two modes:
1. **Browse mode** (current behavior): lazy-load directories on expand, react-arborist `searchTerm` prop used for local filtering
2. **Search mode** (new): when `searchTerm` length >= 2, fire `SearchFiles` RPC (debounced 300ms), build a filtered tree from the results with all ancestor directories open

Key implementation details:
- Add a `searchResults` state: `TreeNode[] | null` (null = browse mode, non-null = search mode)
- Add a `searchLoading` state for the search spinner
- When `searchTerm` changes and length >= 2:
  - Debounce 300ms
  - Call `searchFiles` RPC
  - Convert response `FileNode[]` to a nested `TreeNode[]` tree structure by grouping by path segments
  - Set `searchResults` to the built tree
  - Call `treeRef.current?.openAll()` to expand all nodes
- When `searchTerm` is cleared or length < 2:
  - Set `searchResults` to null (return to browse mode)
- Pass `searchResults ?? treeData` to the `<Tree data={...}>` prop
- When in search mode, disable the `onToggle` lazy-load behavior (all data is already present)
- Continue using `searchMatch` for highlighting but not for filtering (the backend already filtered)

Building the nested tree from flat search results:
```typescript
function buildSearchTree(files: FileNode[]): TreeNode[] {
  // 1. Collect all unique directory paths from file paths
  // 2. Create TreeNode for each directory and file
  // 3. Nest them according to path segments
  // 4. Sort: directories first, then alphabetical
}
```

Files touched (1):
- `web-app/src/components/sessions/FileTree.tsx`

**Task 2.3: Wire search result count in FilesTab** [COMPLETE - commit cc65029]

Add a search result count indicator to the toolbar.

Files touched (2):
- `web-app/src/components/sessions/FilesTab.tsx` -- add optional result count display next to the search input (e.g., "12 matches")
- `web-app/src/components/sessions/FilesTab.module.css` -- add `.searchCount` style for the result count badge

### Integration Checkpoint 2

```bash
make restart-web
# Open browser to localhost:8543
# Navigate to a session's Files tab
# Type a filename that exists in a nested directory
# Verify: matching files appear with ancestor directories expanded
# Verify: clearing the search returns to browse mode
# Verify: directory expand/collapse still works in browse mode
```

---

## Story 3: Polish -- Debounce, UX, and edge cases [COMPLETE]

### Goal

Refine the search experience with proper debounce, loading states, empty states, and cleanup on unmount.

### Tasks

**Task 3.1: Debounce and cancellation** [COMPLETE - commits cc65029, efa27c0]

Ensure the search debounce properly cancels in-flight requests when the user types more characters or clears the field.

Key implementation details:
- Use `useRef` to track the current request ID (incrementing counter)
- On each debounced search call, increment the counter and compare on response
- If the counter has changed by the time the response arrives, discard the result
- On unmount, set a cleanup flag to prevent state updates

Files touched (1):
- `web-app/src/components/sessions/FileTree.tsx` -- add request cancellation logic

**Task 3.2: Search loading and empty states** [COMPLETE - commit cc65029]

- While the search RPC is in flight, show a small spinner next to the search input (or replace the tree with a loading indicator)
- When the search returns 0 results, show "No files match '[query]'"
- When results are truncated (> 500 matches), show "Showing first 500 of N matches"

Files touched (2):
- `web-app/src/components/sessions/FileTree.tsx` -- add loading/empty/truncated states
- `web-app/src/components/sessions/FileTree.module.css` -- add `.searchEmpty`, `.searchTruncated` styles

**Task 3.3: Restore browse state on search clear** [COMPLETE - commit efa27c0]

When the user clears the search, the tree should return to its previous browse state. The `dirContents` map (which tracks lazily loaded directories) should be preserved during search mode so the user doesn't lose their expand state.

Key implementation details:
- Do NOT clear `dirContents` when entering search mode
- When exiting search mode (searchResults set to null), the tree rebuilds from `dirContents` which still has all previously loaded directories
- react-arborist's internal open/close state may need to be restored; call `treeRef.current?.closeAll()` then selectively re-open previously-open nodes, OR simply accept that nodes return to their default closed state (simpler, acceptable UX)

Files touched (1):
- `web-app/src/components/sessions/FileTree.tsx` -- ensure dirContents is preserved

### Integration Checkpoint 3

```bash
make restart-web
# Test rapid typing in search (debounce should prevent excessive RPC calls)
# Test clearing search mid-flight (should not show stale results)
# Test search with no matches (empty state message)
# Test expanding directories, then searching, then clearing (browse state preserved)
```

---

## Known Issues

### Bug Risk: Race condition between search response and mode switch [SEVERITY: Medium]

**Description**: User types a search query, then quickly clears the input. The search RPC may still be in flight and return results after the component has switched back to browse mode, causing stale search results to flash.

**Mitigation**:
- Use request ID pattern (incrementing counter in useRef) to discard stale responses
- Check current searchTerm length before applying results

**Files Likely Affected**:
- `web-app/src/components/sessions/FileTree.tsx`

**Prevention Strategy**:
- Task 3.1 explicitly addresses this with cancellation logic
- Test case: type "main", immediately backspace to empty, verify no flash

### Bug Risk: Search results include ancestor directories that also match the query [SEVERITY: Low]

**Description**: If the search query matches both a directory name and files within it, the directory node itself will appear in results AND as an ancestor container for nested matches. This could cause visual duplication.

**Mitigation**:
- The `buildSearchTree` function should deduplicate: if a path appears both as a direct match and as an ancestor, it appears once with its children
- Mark matched nodes distinctly from ancestor-only nodes (e.g., only highlight matched nodes)

**Files Likely Affected**:
- `web-app/src/components/sessions/FileTree.tsx` (buildSearchTree function)

### Bug Risk: filepath.WalkDir follows symlinked directories on some platforms [SEVERITY: Medium]

**Description**: `filepath.WalkDir` does not follow symlinks by default, but the behavior can vary. If a symlinked directory is encountered and somehow followed, it could cause infinite loops or path traversal.

**Mitigation**:
- The existing ListFiles handler already reports symlinks as non-directories; apply the same check in the WalkDir callback: if `d.Type()&os.ModeSymlink != 0`, skip
- Add a max depth guard (e.g., 20 levels) to prevent runaway traversal

**Files Likely Affected**:
- `server/services/file_service.go`

**Prevention Strategy**:
- WalkDir callback checks `d.Type()&os.ModeSymlink` and returns `filepath.SkipDir`
- Unit test with symlinked directory verifying it is not traversed

### Bug Risk: Gitignore pattern loading during recursive walk is expensive [SEVERITY: Medium]

**Description**: The existing `loadGitignorePatterns` function reads `.gitignore` files from root to the target directory. During a recursive walk, calling this for every directory visited could cause redundant file reads (re-reading the root `.gitignore` for every subdirectory).

**Mitigation**:
- Build an incremental gitignore matcher: start with the root `.gitignore`, and as the walk descends into each directory, check if a `.gitignore` exists there and append its patterns
- Alternatively, use a single `go-git/v5/plumbing/format/gitignore.NewMatcher` built from all `.gitignore` files found during the walk

**Files Likely Affected**:
- `server/services/file_service.go`

**Prevention Strategy**:
- For the initial implementation, accept the simpler approach of building a flat matcher from all `.gitignore` files discovered during the walk
- Profile with `make benchmark-tier1` if performance is a concern

### Bug Risk: Large search results overwhelm the frontend [SEVERITY: Low]

**Description**: A search query matching 500 files in deeply nested directories could produce a tree with thousands of total nodes (files + ancestor directories), potentially causing react-arborist to lag.

**Mitigation**:
- The 500-result cap on the backend limits the worst case
- react-arborist virtualizes rendering (only visible rows are rendered), so DOM performance should be fine
- The tree building is O(N * D) where N = matches and D = average depth -- for 500 matches at depth 10, this is 5000 operations, negligible

**Files Likely Affected**:
- `web-app/src/components/sessions/FileTree.tsx`

---

## Context Preparation Guide

### For Story 1 (Backend)

Read these files before starting:
1. `server/services/file_service.go` -- existing ListFiles/GetFileContent pattern to follow
2. `server/services/file_service_test.go` -- existing test pattern with `testFileService`
3. `proto/session/v1/session.proto` (lines 1163-1191) -- existing `ListFilesRequest/Response` messages
4. `server/services/session_service.go` -- find the delegation pattern for `ListFiles`/`GetFileContent`

Key patterns to follow:
- `resolveAndValidatePath` for path safety
- `hardSkipDirs` map for directory exclusion
- `loadGitignorePatterns` for gitignore matching
- `connect.NewError` with appropriate codes for error handling
- `testFileService` struct with `findInstance` override for testing

### For Story 2 (Frontend)

Read these files before starting:
1. `web-app/src/components/sessions/FileTree.tsx` -- the entire file, understand both data model and rendering
2. `web-app/src/lib/hooks/useFileService.ts` -- the RPC client pattern
3. `web-app/src/components/sessions/FilesTab.tsx` -- the parent component that manages searchTerm state
4. react-arborist v3 docs: `Tree` props (data, searchTerm, searchMatch, ref, openByDefault)

Key patterns to follow:
- `fileNodeToTreeNode` for proto-to-UI conversion
- `buildTreeData` for nested tree construction from flat maps
- `handleToggle` + `loadDirectory` for the lazy-load pattern
- `treeRef.current?.openAll()` / `closeAll()` for programmatic expand/collapse

### For Story 3 (Polish)

Read these files before starting:
1. `web-app/src/components/sessions/FileTree.tsx` -- the search mode implementation from Story 2
2. `web-app/src/components/sessions/FileTree.module.css` -- existing style patterns

Key patterns to follow:
- `requestIdRef` pattern in `useGetFileContent` (useFileService.ts lines 69-100) for stale response prevention
- Loading/error/empty state rendering pattern in FileTree.tsx (lines 383-428)
