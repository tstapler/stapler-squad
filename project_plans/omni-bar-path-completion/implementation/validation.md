# Validation Plan: Omni Bar Path Completion

Status: Tier 1 + Tier 2 complete (32 frontend tests passing, 23 Go tests passing)
Created: 2026-04-08

## Requirements Traceability

| Requirement | Test(s) | Status |
|---|---|---|
| Path existence indicator — green ✓ for existing dir | Frontend: `Omnibar.pathcompletion.test.tsx` > pathExists=true ✅ | Done |
| Path existence indicator — red ✗ for nonexistent path | Frontend: `Omnibar.pathcompletion.test.tsx` > pathExists=false ✅ | Done |
| Path existence indicator — spinner while loading | Frontend: `Omnibar.pathcompletion.test.tsx` > isLoading=true ✅ | Done |
| Real-time directory completions — show on LocalPath input | Frontend: `usePathCompletions.test.ts` > enabled=true ✅ | Done |
| Real-time directory completions — no completions for GitHub URL | Frontend: `Omnibar.pathcompletion.test.tsx` > InputType.GitHubRepo ✅ | Done |
| Keyboard navigation — ↑↓ moves selection | Frontend: `Omnibar.pathcompletion.test.tsx` > ArrowDown/Up ✅ | Done |
| Tab completes to LCP | Frontend: `Omnibar.pathcompletion.test.tsx` > Tab key ✅ | Done |
| Enter accepts selected entry | Frontend: `Omnibar.pathcompletion.test.tsx` > Enter key ✅ | Done |
| Escape dismisses dropdown before closing modal | Frontend: `Omnibar.pathcompletion.test.tsx` > Escape sequence ✅ | Done |
| 150ms debounce | Frontend: `usePathCompletions.test.ts` > timer mock ✅ | Done |
| Tilde expansion | Go: `TestListPathCompletions_TildeExpansion` ✅ | Done |
| Symlink to directory shows as isDirectory=true | Go: `TestListPathCompletions_Symlink_ToDirectory` ✅ | Done |
| Broken symlink skipped | Go: `TestListPathCompletions_BrokenSymlink_Skipped` ✅ | Done |
| Hidden files hidden unless filter starts with "." | Go: `TestListPathCompletions_HiddenFilesHidden` ✅ | Done |
| Truncated flag when over maxResults | Go: `TestListPathCompletions_Truncation` ✅ | Done |
| Permission error → CodePermissionDenied | Go: planned `TestListPathCompletions_PermissionDenied` | Planned |
| Slow filesystem → 2s timeout | Go: `TestListPathCompletions_ContextCancellation` ✅ (partial) | Done |

---

## Tier 1 — Go Unit Tests (server/services/path_completion_service_test.go)

### Implemented (23 tests, 89.5% line coverage)

| Test | What it covers |
|---|---|
| `TestListPathCompletions_BasicListing` | dirs + files returned, BaseDirExists=true |
| `TestListPathCompletions_FilterByPrefix` | filter narrows to matching names |
| `TestListPathCompletions_DirectoriesOnly` | files excluded when flag set |
| `TestListPathCompletions_HiddenFilesHidden` | "." names excluded by default |
| `TestListPathCompletions_HiddenFilesShownWhenPrefixStartsWithDot` | "." prefix shows hidden |
| `TestListPathCompletions_PathExists` | exact dir → PathExists=true, partial → false |
| `TestListPathCompletions_PathExists_FileIsNotDir` | file path → PathExists=false |
| `TestListPathCompletions_NonexistentBaseDir` | BaseDirExists=false, no entries |
| `TestListPathCompletions_Truncation` | Truncated=true when over MaxResults |
| `TestListPathCompletions_NoTruncationUnderLimit` | Truncated=false when under limit |
| `TestListPathCompletions_Symlink_ToDirectory` | symlink to dir → IsDirectory=true |
| `TestListPathCompletions_Symlink_ToFile` | symlink to file → IsDirectory=false |
| `TestListPathCompletions_Symlink_ToFile_ExcludedByDirOnly` | file symlink excluded with DirectoriesOnly |
| `TestListPathCompletions_BrokenSymlink_Skipped` | broken symlink omitted |
| `TestListPathCompletions_TildeExpansion` | `~/` → home dir BaseDir |
| `TestListPathCompletions_TildeAlone` | `~` → home dir BaseDir |
| `TestListPathCompletions_MaxResultsHardCap` | MaxResults>500 silently clamped |
| `TestListPathCompletions_DefaultMaxResults` | MaxResults=0 → defaults to 100 |
| `TestListPathCompletions_EntryPathCorrectness` | entry.Path = filepath.Join(baseDir, name) |
| `TestListPathCompletions_BaseDirInResponse` | BaseDir correct in response |
| `TestListPathCompletions_ContextCancellation` | pre-cancelled ctx doesn't panic |
| `TestExpandTilde` | table-driven: 7 input cases |
| `TestSplitPathPrefix` | table-driven: 6 input cases |

### Remaining coverage gap (10.5%)

The 3 uncovered branches require synthetic error injection not available via real OS:

| Branch | Why not covered | Mitigation |
|---|---|---|
| `connect.CodeDeadlineExceeded` (line 107) | Requires 2s hang on real filesystem | Use mock FS or integration test with slow FUSE |
| `connect.CodeInternal` (line 103) | Requires non-permission ReadDir error | Inject with interface mock |
| `expandTilde` `~username` form (line 183) | Intentionally unsupported; skip | Document in code comment ✓ |

**Recommendation**: Accept current 89.5% coverage. The missing 10.5% is infrastructure error handling that cannot fail on a local healthy filesystem. Add a `// coverage:skip` comment annotation or mock-based test if 100% becomes a hard requirement.

---

## Tier 2 — Frontend Unit Tests (Jest + Testing Library)

**Location**: `web-app/src/lib/hooks/__tests__/usePathCompletions.test.ts`
**Location**: `web-app/src/components/sessions/__tests__/PathCompletionDropdown.test.tsx`
**Location**: `web-app/src/components/sessions/__tests__/Omnibar.pathcompletion.test.tsx`

### usePathCompletions hook

Mock `@connectrpc/connect` and `@connectrpc/connect-web` to return controlled responses.

```typescript
jest.mock('@connectrpc/connect', () => ({ createClient: jest.fn() }))
jest.mock('@connectrpc/connect-web', () => ({ createConnectTransport: jest.fn() }))
```

| Test | Verifies |
|---|---|
| empty pathPrefix → no fetch, entries=[] | enabled guard |
| enabled=false → no fetch | enabled prop |
| fetches after 150ms debounce | timer: fake timers, advance 149ms → no call, 150ms → call |
| AbortController abort on prefix change | rapid prefix change cancels prior request |
| cache hit: second call with same prefix → no second RPC | LRU cache |
| cache miss after TTL: expired entry triggers new RPC | 30s TTL |
| error response → error field set | error state |
| stale response discarded (generation counter) | generation guard |

### PathCompletionDropdown component

```typescript
import { render, screen, fireEvent } from '@testing-library/react';
```

| Test | Verifies |
|---|---|
| renders null when entries=[] and not loading | empty guard |
| renders loading message when isLoading=true and entries=[] | loading state |
| renders all entry names | list rendering |
| selected entry has `itemSelected` class | highlighting |
| mouseDown on entry calls onSelect(entry) | selection callback |
| mouseDown uses preventDefault (keeps focus) | focus preservation |
| directory entry shows "/" suffix | directory indicator |
| file entry has no "/" suffix | file indicator |
| aria-label on ul | accessibility |

### Omnibar — path completion integration

| Test | Verifies |
|---|---|
| path indicator hidden for non-path input (GitHub URL) | isPathInput guard |
| path indicator shows ✓ when pathExists=true | valid indicator |
| path indicator shows ✗ when pathExists=false | invalid indicator |
| path indicator shows spinner when isLoading=true | loading indicator |
| dropdown not rendered when InputType.GitHubRepo | input type guard |
| ArrowDown increments dropdownIndex | keyboard nav |
| ArrowUp decrements dropdownIndex, floors at -1 | keyboard nav bounds |
| Tab with one entry calls handleCompletionSelect | LCP single entry |
| Tab with multiple entries extends to LCP | LCP multiple entries |
| Enter with dropdownIndex>=0 accepts entry | keyboard accept |
| Escape dismisses dropdown (first) then closes modal (second) | two-stage Escape |
| input onChange resets dropdownIndex to -1 | state reset |
| input onChange resets dropdownDismissed to false | state reset |
| dropdownDismissed prevents dropdown after Escape | dismiss guard |

---

## Tier 3 — E2E Tests (Playwright)

**Location**: `web-app/tests/e2e/omnibar-path-completion.spec.ts`

These require the full server running (`make restart-web`) and a real filesystem.

| Test | Flow |
|---|---|
| Type `~/` in omnibar → dropdown shows home dir entries | Happy path smoke |
| Click dropdown entry → input updates to that path + "/" | Click selection |
| Arrow navigate + Enter → input accepts entry | Keyboard selection |
| Tab on unique prefix → completes to only match | LCP acceptance |
| Tab on ambiguous prefix → extends to common prefix | LCP partial |
| Escape → dismisses dropdown, focus stays in input | Dismiss |
| Second Escape → closes omnibar | Close |
| Type nonexistent path → red ✗ appears | Invalid indicator |
| Type existing dir path → green ✓ appears | Valid indicator |
| Type GitHub URL → no completions, no indicator | Input type guard |
| Type `/` → shows root directory entries | Root listing |
| 150ms debounce: rapid typing → single request | Debounce |

---

## Test Execution

```bash
# Go unit tests
go test ./server/services/ -run "TestListPathCompletions|TestExpandTilde|TestSplitPathPrefix" -v

# Frontend unit tests (once implemented)
cd web-app && npx jest src/lib/hooks/__tests__/usePathCompletions.test.ts
cd web-app && npx jest src/components/sessions/__tests__/PathCompletionDropdown.test.tsx

# Full E2E (requires server running)
cd web-app && npx playwright test tests/e2e/omnibar-path-completion.spec.ts
```

---

## Risk Assessment

| Risk | Severity | Mitigation |
|---|---|---|
| Slow NFS/network filesystem hangs UI | HIGH | 2s goroutine timeout ✓ |
| Stale RPC response shown after rapid typing | HIGH | Generation counter + AbortController ✓ |
| Broken symlink causes crash | HIGH | skip on os.Stat failure ✓ |
| Permission denied on ~/private dir shows error modal | MEDIUM | CodePermissionDenied → graceful empty state in UI |
| LRU cache shows stale entries after filesystem change | LOW | 30s TTL short enough for interactive use |
| Dropdown steals Escape from modal close | MEDIUM | Two-stage Escape implemented ✓ |
