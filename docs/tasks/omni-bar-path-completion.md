# Omni Bar Path Completion — Implementation Plan

## Epic Overview

### User Value Statement

When creating sessions via the Omnibar (Cmd+K), users type local paths blind — no validation, no completion, no feedback until the server rejects the path post-submission. This feature adds real-time directory completion and inline path validation so users can navigate the filesystem interactively and never submit an invalid path.

### Success Criteria

1. Zero silent failures for invalid local paths — user sees inline validation (green check / red X) before submission
2. Users reach a valid directory in fewer keystrokes via filterable directory dropdown
3. Path input feels native — keyboard-navigable (arrows, Tab for LCP, Enter to accept, Escape to dismiss), consistent with existing Omnibar UX

### Scope

- **In Scope**: Backend ListPathCompletions RPC, frontend usePathCompletions hook, PathCompletionDropdown component, Omnibar integration, path existence indicator, keyboard navigation, tilde expansion, debouncing, caching
- **Out of Scope**: Session search from Omnibar, remote paths, recently-used history, path completion in SessionWizard/Working Directory/Existing Worktree fields (follow-up)

### Architecture Decisions

- **ADR-001**: Unary RPC over server-streaming — see `project_plans/omni-bar-path-completion/decisions/ADR-001-path-completion-api-design.md`
- **ADR-002**: Dedicated PathCompletionDropdown over reusing AutocompleteInput — see `project_plans/omni-bar-path-completion/decisions/ADR-002-completion-dropdown-architecture.md`
- **ADR-003**: Three-layer client protection (debounce + AbortController + LRU cache) — see `project_plans/omni-bar-path-completion/decisions/ADR-003-client-side-caching-and-debounce-strategy.md`

---

## User Stories

### Story 1: Path Existence Feedback

**As a** user typing a local path in the Omnibar
**I want to** see whether the path exists before I submit
**So that** I never waste time creating a session with a bad path

**Acceptance Criteria**:
- Given I type `/Users/tyler/projects` and it exists, a green checkmark appears at the right edge of the input
- Given I type `/Users/tyler/nonexistent` and it does not exist, a red X appears
- Given I am still typing (within debounce window), a subtle spinner appears
- Given I type a GitHub URL, no path indicator appears (only for LocalPath/PathWithBranch types)
- The indicator updates within 300ms of stopping typing

### Story 2: Directory Completion Dropdown

**As a** user navigating to a deep directory
**I want** a dropdown of matching subdirectories as I type
**So that** I can select the right directory without memorizing the full path

**Acceptance Criteria**:
- Given I type `/Users/tyler/p`, a dropdown shows directories starting with `p` (e.g., `projects/`, `personal/`)
- Directories display a trailing `/` and a folder icon; non-directory entries show a file icon
- Dropdown appears below the input, max 8 visible items with scroll
- Matched characters are visually highlighted in each entry name
- If more than 50 results, a "N more..." indicator appears at the bottom
- Hidden files (starting with `.`) are excluded unless the typed prefix starts with `.`

### Story 3: Keyboard Navigation

**As a** keyboard-driven user
**I want to** navigate and select completions without touching the mouse
**So that** path entry is as fast as a terminal

**Acceptance Criteria**:
- Arrow Down/Up navigates the dropdown; highlighted item scrolls into view
- Tab inserts the longest common prefix of all visible completions (shell-style). If only one match, Tab completes the full name + `/`
- Enter accepts the highlighted item: inserts the full path and triggers a new completion for that directory
- Escape dismisses the dropdown without changing the input
- When dropdown is not visible, Tab/Enter/Arrows behave as normal form navigation

### Story 4: Tilde Expansion

**As a** user
**I want to** type `~/projects` and have it resolve correctly
**So that** I don't have to type my full home directory path

**Acceptance Criteria**:
- Given I type `~/pro`, completions show directories inside my home directory matching `pro`
- The server expands `~` to the actual home directory (`os.UserHomeDir()`)
- The displayed path in completions uses `~/` prefix for paths under home directory

---

## Technical Design

### Data Flow

```
User types in Omnibar input
        |
        v
detector.ts classifies as LocalPath or PathWithBranch?
        |
    [yes]  [no] --> no completion, no indicator
        |
        v
usePathCompletions hook:
  1. Debounce 150ms
  2. Check LRU cache (key = pathPrefix)
  3. Cache miss --> ConnectRPC call: ListPathCompletions
  4. AbortController cancels previous in-flight request
  5. Generation counter discards stale responses
  6. Returns: { entries[], isLoading, error, isValidPath }
        |
        v
PathCompletionDropdown renders entries below input
PathExistenceIndicator renders at right edge of input
Omnibar delegates keyboard events to dropdown when visible
```

### Protobuf Schema (additions to `proto/session/v1/session.proto`)

```protobuf
// In service SessionService:
rpc ListPathCompletions(ListPathCompletionsRequest) returns (ListPathCompletionsResponse) {}

message ListPathCompletionsRequest {
  // The path prefix to complete. Split at last '/' to get base_dir + filter_prefix.
  // Supports ~ expansion (server-side).
  string path_prefix = 1;

  // Maximum entries to return (default 50, server cap 500).
  int32 max_results = 2;

  // If true, only return directories (not regular files).
  bool directories_only = 3;
}

message ListPathCompletionsResponse {
  // Matching directory entries.
  repeated PathEntry entries = 1;

  // The resolved base directory that was listed.
  string base_dir = 2;

  // True if results were truncated to max_results.
  bool truncated = 3;

  // True if base_dir exists and is a directory.
  bool base_dir_exists = 4;

  // True if the exact path_prefix (including partial filename) exists.
  bool path_exists = 5;
}

message PathEntry {
  // Full absolute path to the entry.
  string path = 1;

  // Just the filename component.
  string name = 2;

  // True if entry is a directory (follows symlinks).
  bool is_directory = 3;
}
```

### Go Handler: `server/services/path_completion_service.go`

New file. Separate service following the decomposition pattern (like `ConfigService`, `WorkspaceService`).

Key implementation details:
- Split `path_prefix` at last `/` separator to get `baseDir` and `filterPrefix`
- Expand `~` via `os.UserHomeDir()` before any filesystem access
- `filepath.Clean()` to normalize paths (prevent `../` traversal above intended scope)
- `os.ReadDir(baseDir)` to list entries — returns `[]os.DirEntry`
- Filter: entries whose name has `filterPrefix` as case-insensitive prefix. Hidden files (`.` prefix) excluded unless `filterPrefix` starts with `.`
- Symlink handling: `os.DirEntry.Type()` returns `fs.ModeSymlink` for symlinks. Must `os.Stat()` the symlink target to determine if it resolves to a directory
- Cap entries at `min(max_results, 500)` — set `truncated = true` if exceeded
- Permission errors: catch per-entry and skip (do not abort entire listing)
- Timeout: `context.WithTimeout(ctx, 2*time.Second)` safety net. Check `ctx.Done()` inside the entry loop
- Slow filesystem safety: run `os.ReadDir` in a goroutine; select on result channel vs `time.After(2s)`

### Frontend Hook: `web-app/src/lib/hooks/usePathCompletions.ts`

New file. Pattern follows `useRepositorySuggestions.ts` + `useBranchSuggestions.ts` but with debounce, cancellation, and caching.

```typescript
interface PathEntry {
  path: string;
  name: string;
  isDirectory: boolean;
}

interface UsePathCompletionsResult {
  entries: PathEntry[];
  isLoading: boolean;
  error: string | null;
  baseDirExists: boolean;
  pathExists: boolean;
  truncated: boolean;
}

function usePathCompletions(
  pathPrefix: string,
  options?: { enabled?: boolean; debounceMs?: number; directoriesOnly?: boolean }
): UsePathCompletionsResult
```

Implementation:
- `enabled` defaults to `true`; caller sets to `false` when input is not a LocalPath/PathWithBranch
- Debounce via `setTimeout` in `useEffect` cleanup (not using the existing `useDebounce` hook — need tighter control for AbortController coordination)
- LRU cache: module-level `Map<string, { entries, timestamp }>`, 100-entry max, 30s TTL
- Generation counter: `useRef<number>(0)`, incremented on each new request
- AbortController: `useRef<AbortController | null>(null)`, aborted on cleanup and new request
- ConnectRPC client created per-call (matches existing hook pattern)

### Frontend Component: `web-app/src/components/sessions/PathCompletionDropdown.tsx`

New file + CSS module.

Props:
```typescript
interface PathCompletionDropdownProps {
  entries: PathEntry[];
  isLoading: boolean;
  isVisible: boolean;
  highlightedIndex: number;
  filterPrefix: string;  // for match highlighting
  truncated: boolean;
  onSelect: (entry: PathEntry) => void;
  onHighlightChange: (index: number) => void;
}
```

Visual design:
- Positioned absolutely below the Omnibar input container
- Max 8 visible items, scrollable
- Each entry: `[folder/file icon] [name with match highlight] [/]`
- Directory prefix (base path) displayed muted at top of dropdown
- Highlighted item has accent background
- Truncation indicator: "50 of 127 shown" at bottom when `truncated`
- Matches current Omnibar dark theme (uses existing CSS variables)

### Omnibar Integration (`web-app/src/components/sessions/Omnibar.tsx`)

Changes to existing file:
1. Import and call `usePathCompletions` hook, enabled when `detection?.type` is `LocalPath` or `PathWithBranch`
2. Add `PathCompletionDropdown` render below the input container
3. Add `PathExistenceIndicator` (inline span) at right edge of input
4. Modify `handleKeyDown` to delegate to dropdown: when dropdown is visible, ArrowDown/Up/Tab/Enter/Escape are handled by dropdown logic instead of default form behavior
5. Add state: `completionHighlightIndex`, `isDropdownVisible`
6. Tab handler: compute LCP of visible entries, insert into input
7. Enter handler (on dropdown item): insert full path, dismiss dropdown, trigger new completion for directory
8. Escape handler: dismiss dropdown, re-focus input

---

## Known Issues

### BUG-01: Stale completion results from concurrent requests [SEVERITY: Medium]

**Description**: Fast typing produces multiple overlapping API calls. Without cancellation, an earlier slow response can overwrite a later fast response, showing completions for a stale prefix.

**Mitigation**: Three-layer protection (ADR-003): debounce + AbortController + generation counter. All three must be implemented together.

**Files Affected**: `web-app/src/lib/hooks/usePathCompletions.ts`

**Prevention**: Unit test with artificial delay to verify generation counter discards stale responses.

### BUG-02: Symlinks misclassified as files [SEVERITY: Medium]

**Description**: `os.ReadDir` uses `lstat` (does not follow symlinks). A symlink to a directory will have `Type() & fs.ModeSymlink != 0` but `Type() & fs.ModeDir == 0`. Without explicit `os.Stat()` on symlinks, they appear as files in the dropdown.

**Mitigation**: For any entry where `Type() & fs.ModeSymlink != 0`, call `os.Stat(fullPath)` to resolve the target and check `IsDir()`. Handle `os.Stat` errors gracefully (broken symlink = skip or show as file).

**Files Affected**: `server/services/path_completion_service.go`

**Prevention**: Test with a temp directory containing a symlink to a directory.

### BUG-03: Permission denied on ReadDir crashes handler [SEVERITY: High]

**Description**: If the user types a path to a directory they cannot read (e.g., `/private/var`), `os.ReadDir` returns an error. Without handling, this surfaces as an opaque ConnectRPC internal error.

**Mitigation**: Catch `os.ReadDir` errors. If `os.IsPermission(err)`, return a structured ConnectRPC error with code `PermissionDenied` and a human-readable message. Set `base_dir_exists = true` but return empty entries.

**Files Affected**: `server/services/path_completion_service.go`

**Prevention**: Test with a temp directory with 000 permissions.

### BUG-04: Tilde expansion missing on server [SEVERITY: High]

**Description**: Go has no built-in `~` expansion. If the client sends `~/projects`, `os.ReadDir("~/projects")` fails because that literal path does not exist.

**Mitigation**: Server-side: if `path_prefix` starts with `~/`, replace `~` with `os.UserHomeDir()`. If `path_prefix` is exactly `~`, expand to home directory. Do NOT expand `~user` syntax (out of scope).

**Files Affected**: `server/services/path_completion_service.go`

**Prevention**: Test with `~/` prefix input.

### BUG-05: Tab key conflicts with form navigation [SEVERITY: Medium]

**Description**: Tab normally moves focus to the next form field. When the completion dropdown is visible, Tab should insert the LCP instead. If not handled carefully, Tab both inserts the completion AND moves focus away.

**Mitigation**: Call `e.preventDefault()` on Tab keydown ONLY when the dropdown is visible and there are completions. When dropdown is hidden, let Tab behave normally.

**Files Affected**: `web-app/src/components/sessions/Omnibar.tsx`

**Prevention**: Manual test: Tab with dropdown open inserts completion; Tab with dropdown closed moves focus.

### BUG-06: Slow filesystem blocks handler [SEVERITY: Low]

**Description**: `os.ReadDir` is a blocking syscall. On NFS or network-mounted filesystems, it can hang for seconds. Go's `context.Context` does not cancel blocking syscalls.

**Mitigation**: Run `os.ReadDir` in a goroutine. Select on the result channel vs `time.After(2*time.Second)`. If timeout fires, return empty entries with a structured error indicating timeout.

**Files Affected**: `server/services/path_completion_service.go`

**Prevention**: Difficult to unit test without mocking filesystem. Add structured logging for read duration.

---

## Implementation Tasks

### Phase 1: Backend API (vertical slice — server can be tested independently)

#### Task 1.1: Protobuf Schema — Add ListPathCompletions RPC

**Objective**: Define the wire protocol for path completion.

**Files to modify**:
- `proto/session/v1/session.proto` — add RPC + messages

**Implementation details**:
1. Add `ListPathCompletions` RPC to the `SessionService` service block (after `ForkSession`, line ~195)
2. Add `ListPathCompletionsRequest`, `ListPathCompletionsResponse`, and `PathEntry` messages (after the Checkpoint messages section, ~line 1142)
3. Run `make proto-gen` to regenerate Go and TypeScript code

**Protobuf additions** (exact text):
```protobuf
// In service SessionService, add:
  // ListPathCompletions returns filesystem directory entries matching a path prefix.
  // Used by the Omnibar for real-time path completion and validation.
  rpc ListPathCompletions(ListPathCompletionsRequest) returns (ListPathCompletionsResponse) {}

// After existing messages, add:

// ============================================================================
// Path Completion Messages
// ============================================================================

message ListPathCompletionsRequest {
  // Path prefix to complete. The server splits at the last '/' to determine
  // the base directory and filter prefix. Supports ~ expansion.
  string path_prefix = 1;

  // Maximum entries to return. Default: 50, server cap: 500.
  // Use 0 for server default.
  int32 max_results = 2;

  // If true, only return directory entries (not regular files).
  bool directories_only = 3;
}

message ListPathCompletionsResponse {
  // Matching filesystem entries, sorted alphabetically.
  repeated PathEntry entries = 1;

  // The resolved base directory that was listed (after ~ expansion, filepath.Clean).
  string base_dir = 2;

  // True if results were capped at max_results.
  bool truncated = 3;

  // True if base_dir exists and is a directory.
  bool base_dir_exists = 4;

  // True if the full path_prefix (including partial filename) exists on disk.
  bool path_exists = 5;
}

message PathEntry {
  // Full absolute path to the entry.
  string path = 1;

  // Filename component only.
  string name = 2;

  // True if this entry is a directory (symlinks resolved).
  bool is_directory = 3;
}
```

**Verification**: `make proto-gen` succeeds; `make proto-lint` passes; generated files appear in `gen/proto/go/` and `web-app/src/gen/`.

---

#### Task 1.2: Go Handler — PathCompletionService

**Objective**: Implement the server-side directory listing logic.

**Files to create**:
- `server/services/path_completion_service.go`

**Files to modify**:
- `server/services/session_service.go` — add delegation method + wire new service

**Implementation details**:

Create `PathCompletionService` struct with a `ListPathCompletions` method:

```go
// path_completion_service.go
package services

import (
    "context"
    "fmt"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "time"

    sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
    "github.com/tstapler/stapler-squad/log"
    "connectrpc.com/connect"
)

type PathCompletionService struct{}

func NewPathCompletionService() *PathCompletionService {
    return &PathCompletionService{}
}

func (s *PathCompletionService) ListPathCompletions(
    ctx context.Context,
    req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
    // Implementation:
    // 1. Expand tilde
    // 2. Split pathPrefix at last '/' -> baseDir + filterPrefix
    // 3. filepath.Clean(baseDir)
    // 4. Validate baseDir exists (os.Stat)
    // 5. os.ReadDir(baseDir) in goroutine with 2s timeout
    // 6. Filter entries by filterPrefix (case-insensitive prefix match)
    // 7. Skip hidden files unless filterPrefix starts with '.'
    // 8. Resolve symlinks via os.Stat for is_directory
    // 9. Cap at maxResults (default 50, cap 500)
    // 10. Return response with base_dir_exists, path_exists, truncated
}
```

Key behaviors:
- `expandTilde(path string) (string, error)` — helper using `os.UserHomeDir()`
- `splitPathPrefix(pathPrefix string) (baseDir, filterPrefix string)` — split at last `/`
- Permission errors on `os.ReadDir` -> `connect.CodePermissionDenied`
- Individual entry `os.Stat` errors (broken symlinks) -> skip entry, log warning
- `context.WithTimeout(ctx, 2*time.Second)` wrapping the ReadDir goroutine

Wire into SessionService:
```go
// In session_service.go, add field:
pathCompletionSvc *PathCompletionService

// In NewSessionService(), add:
pathCompletionSvc: NewPathCompletionService(),

// Add delegation method:
func (s *SessionService) ListPathCompletions(
    ctx context.Context,
    req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
    return s.pathCompletionSvc.ListPathCompletions(ctx, req)
}
```

**Verification**: `go build .` compiles. `go test ./server/services/ -run TestListPathCompletions` passes (write test in Task 1.3).

---

#### Task 1.3: Go Tests — PathCompletionService

**Objective**: Unit tests covering core logic and edge cases.

**Files to create**:
- `server/services/path_completion_service_test.go`

**Test cases**:
1. `TestListPathCompletions_BasicDirectory` — create temp dir with 3 subdirs + 2 files, verify correct entries returned
2. `TestListPathCompletions_FilterPrefix` — verify only entries matching prefix are returned
3. `TestListPathCompletions_DirectoriesOnly` — verify files excluded when `directories_only = true`
4. `TestListPathCompletions_HiddenFiles` — verify `.hidden` excluded unless prefix starts with `.`
5. `TestListPathCompletions_Symlinks` — create symlink to directory, verify `is_directory = true`
6. `TestListPathCompletions_BrokenSymlink` — create dangling symlink, verify it is skipped gracefully
7. `TestListPathCompletions_PermissionDenied` — create dir with 000 perms, verify structured error
8. `TestListPathCompletions_NonexistentPath` — verify `base_dir_exists = false`
9. `TestListPathCompletions_TildeExpansion` — verify `~/` expands to home dir (integration-style)
10. `TestListPathCompletions_Truncation` — create 60 entries, request max 50, verify `truncated = true`
11. `TestListPathCompletions_PathExists` — verify `path_exists` is true when full prefix matches an existing entry
12. `TestExpandTilde` — unit test for the helper
13. `TestSplitPathPrefix` — unit test for splitting logic

**Verification**: `go test ./server/services/ -run TestListPathCompletions -v` all pass.

---

### Phase 2: Frontend Hook (can be developed after proto-gen, testable with mock)

#### Task 2.1: usePathCompletions Hook

**Objective**: Encapsulate the debounce + cache + API call logic in a reusable hook.

**Files to create**:
- `web-app/src/lib/hooks/usePathCompletions.ts`

**Implementation details**:

```typescript
// Module-level LRU cache
const cache = new Map<string, { entries: PathEntry[]; baseDirExists: boolean; pathExists: boolean; truncated: boolean; timestamp: number }>();
const CACHE_TTL = 30_000; // 30 seconds
const CACHE_MAX = 100;

export function usePathCompletions(
  pathPrefix: string,
  options: { enabled?: boolean; debounceMs?: number; directoriesOnly?: boolean } = {}
): UsePathCompletionsResult {
  const { enabled = true, debounceMs = 150, directoriesOnly = true } = options;
  // State: entries, isLoading, error, baseDirExists, pathExists, truncated
  // Refs: generationRef, abortControllerRef, timerRef

  useEffect(() => {
    if (!enabled || !pathPrefix.trim()) {
      // Reset state
      return;
    }

    // Check cache
    const cached = cache.get(pathPrefix);
    if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
      // Use cached result, skip API call
      return;
    }

    // Debounce
    const generation = ++generationRef.current;
    const timer = setTimeout(async () => {
      // Abort previous
      abortControllerRef.current?.abort();
      const controller = new AbortController();
      abortControllerRef.current = controller;

      try {
        setIsLoading(true);
        const transport = createConnectTransport({ baseUrl, ...(controller.signal && { signal: controller.signal }) });
        const client = createClient(SessionService, transport);
        const response = await client.listPathCompletions({
          pathPrefix,
          maxResults: 50,
          directoriesOnly,
        });

        // Generation check
        if (generation !== generationRef.current) return;

        // Update cache
        const result = { entries: response.entries.map(...), baseDirExists: response.baseDirExists, ... };
        cache.set(pathPrefix, { ...result, timestamp: Date.now() });
        evictCache();

        // Update state
      } catch (err) {
        if (err.name === 'AbortError' || generation !== generationRef.current) return;
        setError(err.message);
      } finally {
        if (generation === generationRef.current) setIsLoading(false);
      }
    }, debounceMs);

    return () => clearTimeout(timer);
  }, [pathPrefix, enabled, debounceMs, directoriesOnly]);
}
```

**Verification**: Write a small test component or Storybook story. Verify debounce behavior manually.

---

### Phase 3: Frontend Dropdown Component

#### Task 3.1: PathCompletionDropdown Component

**Objective**: Render the completion dropdown with match highlighting, icons, and scroll.

**Files to create**:
- `web-app/src/components/sessions/PathCompletionDropdown.tsx`
- `web-app/src/components/sessions/PathCompletionDropdown.module.css`

**Implementation details**:

Component structure:
```tsx
<div className={styles.dropdown} role="listbox" aria-label="Path completions">
  {/* Base directory label */}
  <div className={styles.baseDir}>{baseDir}</div>

  {/* Entry list */}
  <ul className={styles.entryList} ref={listRef}>
    {entries.map((entry, i) => (
      <li
        key={entry.path}
        className={`${styles.entry} ${i === highlightedIndex ? styles.highlighted : ''}`}
        role="option"
        aria-selected={i === highlightedIndex}
        onClick={() => onSelect(entry)}
        onMouseEnter={() => onHighlightChange(i)}
      >
        <span className={styles.icon}>{entry.isDirectory ? '📁' : '📄'}</span>
        <span className={styles.name}>
          {highlightMatches(entry.name, filterPrefix)}
        </span>
        {entry.isDirectory && <span className={styles.dirSlash}>/</span>}
      </li>
    ))}
  </ul>

  {/* Truncation indicator */}
  {truncated && <div className={styles.truncated}>More results available...</div>}

  {/* Loading indicator */}
  {isLoading && <div className={styles.loading}>Loading...</div>}
</div>
```

`highlightMatches(name, prefix)` — returns array of `<span>` elements with matched characters wrapped in `<mark>`.

CSS: positioned absolutely, max-height 280px (8 items * 35px), overflow-y auto, z-index 1001 (above modal), dark theme using Omnibar CSS variables.

**Verification**: Renders correctly in browser. Keyboard navigation scrolls highlighted item into view.

---

### Phase 4: Omnibar Integration (the wiring)

#### Task 4.1: Wire Hook + Dropdown + Indicator into Omnibar

**Objective**: Connect all pieces in Omnibar.tsx — the main integration point.

**Files to modify**:
- `web-app/src/components/sessions/Omnibar.tsx`
- `web-app/src/components/sessions/Omnibar.module.css`

**Implementation details**:

1. **Import and call hook**:
```tsx
const isPathInput = detection?.type === InputType.LocalPath || detection?.type === InputType.PathWithBranch;
const pathForCompletion = isPathInput ? (detection?.localPath || input) : '';

const {
  entries: completionEntries,
  isLoading: completionsLoading,
  baseDirExists,
  pathExists,
  truncated,
} = usePathCompletions(pathForCompletion, {
  enabled: isPathInput && input.trim().length > 0,
  directoriesOnly: true,
});
```

2. **State for dropdown**:
```tsx
const [completionHighlight, setCompletionHighlight] = useState(-1);
const [isDropdownVisible, setIsDropdownVisible] = useState(false);

// Show dropdown when entries available
useEffect(() => {
  setIsDropdownVisible(completionEntries.length > 0 && isPathInput);
  setCompletionHighlight(-1);
}, [completionEntries, isPathInput]);
```

3. **Path existence indicator** (inside `.inputContainer`, after the `<input>`):
```tsx
{isPathInput && input.trim() && (
  <span className={styles.pathIndicator} aria-label={pathExists ? "Path exists" : "Path not found"}>
    {completionsLoading ? '⏳' : pathExists || baseDirExists ? '✓' : '✗'}
  </span>
)}
```

4. **Dropdown render** (inside `.modal`, after `.inputContainer`):
```tsx
{isDropdownVisible && (
  <PathCompletionDropdown
    entries={completionEntries}
    isLoading={completionsLoading}
    isVisible={isDropdownVisible}
    highlightedIndex={completionHighlight}
    filterPrefix={extractFilterPrefix(pathForCompletion)}
    truncated={truncated}
    onSelect={handleCompletionSelect}
    onHighlightChange={setCompletionHighlight}
  />
)}
```

5. **Keyboard handling** — modify `handleKeyDown` in Omnibar:
```tsx
const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
  // Dropdown keyboard handling takes priority
  if (isDropdownVisible && completionEntries.length > 0) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setCompletionHighlight(prev =>
          prev < completionEntries.length - 1 ? prev + 1 : prev
        );
        return;
      case 'ArrowUp':
        e.preventDefault();
        setCompletionHighlight(prev => prev > 0 ? prev - 1 : -1);
        return;
      case 'Tab':
        e.preventDefault();
        handleTabCompletion();
        return;
      case 'Enter':
        if (completionHighlight >= 0) {
          e.preventDefault();
          handleCompletionSelect(completionEntries[completionHighlight]);
          return;
        }
        break;
      case 'Escape':
        e.preventDefault();
        setIsDropdownVisible(false);
        return;
    }
  }

  // Default Omnibar handling
  if (e.key === 'Escape') { onClose(); }
  else if (e.key === 'Enter' && e.metaKey) { handleSubmit(); }
}, [isDropdownVisible, completionEntries, completionHighlight, onClose]);
```

6. **Tab completion (LCP)**:
```tsx
function handleTabCompletion() {
  if (completionEntries.length === 0) return;

  if (completionEntries.length === 1) {
    // Single match: complete full name + /
    const entry = completionEntries[0];
    const newPath = entry.path + (entry.isDirectory ? '/' : '');
    setInput(newPath);
    return;
  }

  // Multiple matches: compute longest common prefix
  const names = completionEntries.map(e => e.name);
  const lcp = longestCommonPrefix(names);
  if (lcp.length > 0) {
    const baseDir = pathForCompletion.substring(0, pathForCompletion.lastIndexOf('/') + 1);
    setInput(baseDir + lcp);
  }
}

function longestCommonPrefix(strings: string[]): string {
  if (strings.length === 0) return '';
  let prefix = strings[0];
  for (let i = 1; i < strings.length; i++) {
    while (strings[i].indexOf(prefix) !== 0) {
      prefix = prefix.substring(0, prefix.length - 1);
      if (prefix === '') return '';
    }
  }
  return prefix;
}
```

7. **Selection handler**:
```tsx
function handleCompletionSelect(entry: PathEntry) {
  if (entry.isDirectory) {
    setInput(entry.path + '/');
  } else {
    setInput(entry.path);
  }
  setIsDropdownVisible(false);
  setCompletionHighlight(-1);
  inputRef.current?.focus();
}
```

8. **CSS additions** to `Omnibar.module.css`:
```css
.pathIndicator {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  font-size: 14px;
  flex-shrink: 0;
  color: var(--text-muted, #666);
}

.pathIndicator.valid {
  color: var(--success-color, #22c55e);
}

.pathIndicator.invalid {
  color: var(--error-color, #ef4444);
}
```

**Verification**: `make restart-web`. Open Omnibar (Cmd+K), type `/`, verify dropdown appears. Test keyboard navigation. Test Tab completion. Test path existence indicator.

---

### Phase 5: Polish and Edge Cases

#### Task 5.1: Edge Case Hardening

**Objective**: Handle all edge cases identified in research.

**Files to modify**:
- `server/services/path_completion_service.go` — path traversal guard, empty input
- `web-app/src/lib/hooks/usePathCompletions.ts` — empty input guard, cleanup on unmount
- `web-app/src/components/sessions/Omnibar.tsx` — dropdown dismiss on input clear

**Edge cases to handle**:
1. Empty string input -> no API call, no dropdown
2. Input is just `/` -> list root directory (may be slow, but valid)
3. Input is `~` (no slash) -> expand to home dir and list it
4. Input contains `..` -> `filepath.Clean` normalizes; no special handling needed
5. Very long path (>4096 chars) -> server rejects with InvalidArgument
6. Path with spaces -> works naturally (no URL encoding needed for ConnectRPC)
7. Trailing slash -> list that directory (filterPrefix is empty, show all children)
8. Dropdown dismissed on input blur -> close after 150ms delay (allow click on dropdown item)
9. Rapid open/close of Omnibar -> AbortController cancels pending, cache cleared on close

**Verification**: Manual testing of each edge case. Add unit tests for server-side edge cases.

---

## Dependency Graph

```
Task 1.1 (Proto Schema)
    |
    v
Task 1.2 (Go Handler) ----> Task 1.3 (Go Tests)
    |
    v
Task 2.1 (Frontend Hook)
    |
    v
Task 3.1 (Dropdown Component)
    |
    v
Task 4.1 (Omnibar Integration)
    |
    v
Task 5.1 (Edge Case Hardening)
```

Tasks 1.2 and 1.3 can be done in parallel (write handler and tests together). Task 3.1 can be started in parallel with 2.1 (mock data). All other dependencies are sequential.

## Testing Strategy

| Layer | What | How |
|-------|------|-----|
| Unit (Go) | PathCompletionService logic | `go test ./server/services/ -run TestListPathCompletions` — temp dirs, symlinks, perms |
| Unit (Go) | Helper functions | `TestExpandTilde`, `TestSplitPathPrefix` |
| Integration (Go) | RPC handler end-to-end | ConnectRPC test client hitting handler with temp filesystem |
| Manual (Frontend) | Hook debounce/cache behavior | Browser DevTools network tab: verify single request per debounce window, cache hits |
| Manual (Frontend) | Dropdown UX | Type path, verify dropdown renders, keyboard nav works, Tab completes LCP |
| Manual (Frontend) | Edge cases | Empty input, permissions, tilde, broken symlinks, rapid typing |
| Build | Proto generation | `make proto-gen` succeeds, `make proto-lint` passes |
| Build | Compile | `make build && make test` pass |

## Files Created/Modified Summary

### New Files
| File | Purpose |
|------|---------|
| `server/services/path_completion_service.go` | Go handler for ListPathCompletions |
| `server/services/path_completion_service_test.go` | Unit tests |
| `web-app/src/lib/hooks/usePathCompletions.ts` | Frontend hook with debounce + cache |
| `web-app/src/components/sessions/PathCompletionDropdown.tsx` | Dropdown component |
| `web-app/src/components/sessions/PathCompletionDropdown.module.css` | Dropdown styles |

### Modified Files
| File | Change |
|------|--------|
| `proto/session/v1/session.proto` | Add RPC + 3 messages |
| `server/services/session_service.go` | Add pathCompletionSvc field + delegation method (~10 lines) |
| `web-app/src/components/sessions/Omnibar.tsx` | Hook call, dropdown render, keyboard delegation, indicator (~80 lines) |
| `web-app/src/components/sessions/Omnibar.module.css` | Path indicator styles (~15 lines) |

### Generated Files (via `make proto-gen`)
| File | Auto-generated |
|------|---------------|
| `gen/proto/go/session/v1/*.go` | Go protobuf + ConnectRPC stubs |
| `web-app/src/gen/session/v1/*.ts` | TypeScript protobuf client |
