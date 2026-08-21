# Validation Plan: Omni Bar Session Search

Status: Draft | Phase: 4 - Validation
Created: 2026-04-15

---

## Requirements Traceability Matrix

| Success Criterion (requirements.md) | Test ID(s) | Layer | Status |
|---|---|---|---|
| SC-1: Reach any session in ≤3 keystrokes after Cmd+K | T-E2E-001 | E2E | Pending |
| SC-2: Start new session on any previously-used repo in ≤5 keystrokes | T-E2E-002 | E2E | Pending |
| SC-3: Path completion <100ms on cache hit | T-E2E-003, T-UNIT-GO-004 | E2E + Unit | Pending |
| SC-3: Path completion <500ms cold start | T-E2E-004 | E2E | Pending |
| SC-4: "myfeat" matches "my-feature-branch" | T-UNIT-TS-004 | Unit | Pending |
| SC-4: "squad" matches "stapler-squad" | T-UNIT-TS-003 | Unit | Pending |
| Must Have: Session fuzzy search in Omnibar | T-UNIT-TS-001–007, T-INT-001 | Unit + Integration | Pending |
| Must Have: Recent repos quick-pick empty state | T-UNIT-TS-012–015, T-INT-002 | Unit + Integration | Pending |
| Must Have: Proper fuzzy algorithm (not naive substring) | T-UNIT-TS-003, T-UNIT-TS-004, T-UNIT-TS-016 | Unit | Pending |
| Must Have: Directory tree cache (mtime + TTL) | T-UNIT-GO-001–007, T-INT-GO-001–002 | Unit + Integration | Pending |
| Constraint: Zero new RPC endpoints for session search | T-ARCH-001 | Architecture | Pending |
| Constraint: Zero proto changes | T-ARCH-002 | Architecture | Pending |
| Constraint: Session search results update <16ms per keypress | T-PERF-001 | Performance | Pending |

---

## Story Acceptance Criteria Traceability

| Story | Acceptance Criterion | Test ID(s) |
|---|---|---|
| Story 1: DirCache | Cache hit returns entries without ReadDir | T-UNIT-GO-001, T-INT-GO-001 |
| Story 1: DirCache | Miss on changed mtime | T-UNIT-GO-002 |
| Story 1: DirCache | Miss on TTL expiry | T-UNIT-GO-003 |
| Story 1: DirCache | Eviction at maxSize | T-UNIT-GO-005, T-UNIT-GO-006 |
| Story 1: DirCache | No data race under concurrency | T-UNIT-GO-007 |
| Story 2: Detector | Bare text routes to SessionSearch, not Unknown | T-UNIT-TS-008 |
| Story 2: Detector | Path input still routes to LocalPath | T-UNIT-TS-010 |
| Story 2: Detector | GitHub shorthand still routes correctly | T-UNIT-TS-011 |
| Story 2: Detector | Empty input does not match SessionSearch | T-UNIT-TS-009 |
| Story 2: usePathHistory | getAll returns top N by score | T-UNIT-TS-012 |
| Story 2: usePathHistory | getAll ordering: recent beats stale | T-UNIT-TS-015 |
| Story 2: dropdownDismissed | Session results appear after Escape + backspace | T-UNIT-TS-020, T-INT-005 |
| Story 3: useSessionSearch | Empty query returns [] | T-UNIT-TS-001 |
| Story 3: useSessionSearch | Title weight beats path weight | T-UNIT-TS-002 |
| Story 3: useSessionSearch | STOPPED sessions excluded | T-UNIT-TS-005 |
| Story 3: useSessionSearch | Results capped at 8 | T-UNIT-TS-006 |
| Story 3: useSessionSearch | Fuzzy non-prefix match | T-UNIT-TS-003, T-UNIT-TS-004 |
| Story 4: OmnibarResultList | Session section hidden when results empty | T-UNIT-TS-017 |
| Story 4: OmnibarResultList | Repo section hidden when entries empty | T-UNIT-TS-018 |
| Story 4: OmnibarResultList | Create-new always visible | T-UNIT-TS-019 |
| Story 4: OmnibarResultList | Arrow down skips section headers | T-UNIT-TS-022 |
| Story 4: OmnibarResultList | Enter on session calls onSessionSelect | T-UNIT-TS-023 |
| Story 4: OmnibarResultList | Enter on repo calls onRepoSelect | T-UNIT-TS-024 |
| Story 4: OmnibarResultList | ARIA activedescendant tracks highlight | T-UNIT-TS-021 |
| Story 5: Integration | Discovery mode on open | T-INT-001 |
| Story 5: Integration | Creation mode on path input | T-INT-003 |
| Story 5: Integration | Session navigate + close | T-INT-004 |
| Story 5: Integration | Repo select pre-fills path | T-INT-006 |
| Story 5: Integration | Escape clears highlight; second Escape closes | T-INT-007, T-INT-008 |
| Story 5: Integration | canSubmit false for SessionSearch type | T-UNIT-TS-025 |

---

## Test Suite Overview

### Layer Distribution

| Layer | Count |
|---|---|
| Go unit tests | 9 |
| Go integration tests | 2 |
| TypeScript unit tests | 25 |
| TypeScript integration tests | 8 |
| E2E tests | 4 |
| Architecture/constraint tests | 2 |
| Performance benchmarks | 4 |
| **Total** | **54** |

Target pyramid: ~60% unit, ~25% integration, ~7% E2E, ~8% performance/architecture.

---

## Go Unit Tests

### DirCache (`server/services/dir_cache_test.go`)

#### T-UNIT-GO-001: Cache hit returns stored entries without re-reading

```
File: server/services/dir_cache_test.go
Function: TestDirCache_Hit
```

Given: A `DirCache` with `maxSize=10, ttl=60s`. A temporary directory has been created with 3 files. `Put` has been called with the directory path, the 3 `os.DirEntry` items, and the directory's current `ModTime()`.

When: `Get` is called for the same path immediately after.

Then: `(entries, true)` is returned. `len(entries) == 3`. The returned slice is the same object (or equal) to what was passed to `Put`.

Pass criteria: `ok == true`, `len(entries) == 3`, no call to `os.ReadDir` (verified by inserting a counter or mock that wraps `os.ReadDir` — alternatively, verify via timing that no syscall happens).

---

#### T-UNIT-GO-002: Cache miss when directory mtime has changed

```
File: server/services/dir_cache_test.go
Function: TestDirCache_MissOnMtimeChange
```

Given: A `DirCache`. A temporary directory has been created. `Put` called with current mtime. A new file is then created inside that directory, advancing the directory's mtime.

When: `Get` is called for the same path after the mtime change.

Then: `(nil, false)` is returned — the cache detects the directory changed and returns a miss.

Pass criteria: `ok == false`, `entries == nil`.

Implementation note: On macOS/Linux, creating a file inside a directory advances the directory's mtime. The test must call `os.Stat(path).ModTime()` after the file creation to confirm mtime advanced before asserting the miss.

---

#### T-UNIT-GO-003: Cache miss after TTL expiry

```
File: server/services/dir_cache_test.go
Function: TestDirCache_MissOnTTLExpiry
```

Given: A `DirCache` with `ttl=1ms`. `Put` called with current mtime.

When: `time.Sleep(5 * time.Millisecond)` passes, then `Get` is called.

Then: `(nil, false)` returned — TTL has expired.

Pass criteria: `ok == false`. The directory mtime has NOT changed (only TTL triggered the miss).

---

#### T-UNIT-GO-004: Cache hit performance — Get completes in <100ms

```
File: server/services/dir_cache_test.go
Function: TestDirCache_HitPerformance
```

Given: A `DirCache` populated with 256 entries (maxSize) covering a typical home directory listing.

When: `Get` is called for one of the cached paths and the call is timed with `time.Since`.

Then: Elapsed time is less than 100ms.

Pass criteria: `elapsed < 100*time.Millisecond`. This test enforces the <100ms cache-hit requirement from SC-3.

---

#### T-UNIT-GO-005: No eviction below maxSize

```
File: server/services/dir_cache_test.go
Function: TestDirCache_NoEvictionBelowMax
```

Given: A `DirCache` with `maxSize=5`. `Put` has been called 4 times with 4 different paths, each with a different `cachedAt` time spaced 1ms apart.

When: The internal `entries` map is inspected.

Then: All 4 paths are still present. No eviction occurred.

Pass criteria: `len(cache.entries) == 4`.

---

#### T-UNIT-GO-006: Evicts oldest entry at maxSize

```
File: server/services/dir_cache_test.go
Function: TestDirCache_EvictsOldestAtMax
```

Given: A `DirCache` with `maxSize=3`. `Put` called 3 times with paths `"/a"`, `"/b"`, `"/c"`, with `cachedAt` at t=0, t=1ms, t=2ms respectively (controlled via a fake clock or by spacing real sleeps).

When: `Put` is called for a fourth path `"/d"`.

Then: Total entries in the map is 3. The entry for `"/a"` (oldest `cachedAt`) is gone. Entries for `"/b"`, `"/c"`, and `"/d"` are present.

Pass criteria: `len(cache.entries) == 3`, `cache.entries["/a"] == nil`, `cache.entries["/d"] != nil`.

---

#### T-UNIT-GO-007: Concurrent reads produce no data race

```
File: server/services/dir_cache_test.go
Function: TestDirCache_ConcurrentReads
Run with: go test -race ./server/services/...
```

Given: A `DirCache` with one entry populated via `Put`.

When: 50 goroutines each call `Get` simultaneously using `sync.WaitGroup`.

Then: No data race is reported by the race detector. All 50 goroutines receive the same entries.

Pass criteria: Test passes with `go test -race` — zero race conditions reported. `len(results) == 50` with all entries matching.

---

### PathCompletionService Integration (`server/services/path_completion_service_test.go`)

#### T-INT-GO-001: Second call returns cached entries (ReadDir not called twice)

```
File: server/services/path_completion_service_test.go
Function: TestListPathCompletions_CacheHit
```

Given: A `PathCompletionService` with an instrumented `DirCache`. A real temporary directory with 5 subdirectories. `ListPathCompletions` called once, which populates the cache.

When: `ListPathCompletions` is called again with the exact same path prefix before any directory changes.

Then: The same 5 completions are returned. `os.ReadDir` was called exactly once total (verified via a call-count wrapper or by asserting the ReadDir counter incremented only on the first call).

Pass criteria: Both calls return identical results. `readDirCallCount == 1`.

---

#### T-INT-GO-002: Cache miss after directory mutation returns fresh entries

```
File: server/services/path_completion_service_test.go
Function: TestListPathCompletions_CacheMissAfterMtimeChange
```

Given: A `PathCompletionService`. A real temporary directory with 2 subdirectories. `ListPathCompletions` called once — cache populated.

When: A new subdirectory is created inside the temporary directory (advancing its mtime), then `ListPathCompletions` is called again.

Then: 3 completions are returned (the original 2 plus the new one). `readDirCallCount == 2` — the cache invalidated and re-read.

Pass criteria: Second call returns 3 results. `readDirCallCount == 2`.

---

## TypeScript Unit Tests

### useSessionSearch hook (`web-app/src/lib/hooks/useSessionSearch.test.ts`)

#### T-UNIT-TS-001: Empty query returns empty array

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: describe("useSessionSearch") > "returns [] for empty query"
```

Given: Redux store with 5 active sessions. `useSessionSearch` rendered with `renderHook`.

When: `query = ""`

Then: Hook returns `[]`.

Pass criteria: `result.current.length === 0`.

---

#### T-UNIT-TS-002: Title match ranks above path-only match

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "title match ranks above path match for same query"
```

Given: Redux store with two sessions:
- Session A: `{ title: "auth-service", path: "/home/user/other", branch: "main" }`
- Session B: `{ title: "other-service", path: "/home/user/auth", branch: "main" }`

When: `query = "auth"`

Then: Session A appears before Session B in results (`results[0].session.title === "auth-service"`).

Pass criteria: `results[0].session.title === "auth-service"`, `results.length >= 2`.

---

#### T-UNIT-TS-003: Fuzzy non-prefix match — "squad" matches "stapler-squad"

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "matches tail substring via ignoreLocation"
```

Given: Redux store with one session: `{ title: "stapler-squad", path: "/home/user/stapler-squad" }`.

When: `query = "squad"`

Then: The session is returned in results.

Pass criteria: `results.length === 1`, `results[0].session.title === "stapler-squad"`.

---

#### T-UNIT-TS-004: Character-sequence fuzzy — "myfeat" matches "my-feature-branch"

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "matches hyphenated branch via character sequence"
```

Given: Redux store with one session: `{ title: "auth", branch: "my-feature-branch", path: "/x" }`.

When: `query = "myfeat"`

Then: The session is returned in results (Fuse.js `ignoreLocation: true` finds the match across the hyphen).

Pass criteria: `results.length === 1`, `results[0].session.branch === "my-feature-branch"`.

---

#### T-UNIT-TS-005: STOPPED sessions are excluded from results

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "excludes STOPPED sessions"
```

Given: Redux store with two sessions:
- Session A: `{ title: "active-session", status: SessionStatus.RUNNING }`
- Session B: `{ title: "stopped-session", status: SessionStatus.STOPPED }`

When: `query = "session"` (matches both titles)

Then: Only Session A is returned. Session B is absent.

Pass criteria: `results.length === 1`, `results[0].session.title === "active-session"`.

---

#### T-UNIT-TS-006: Results capped at 8

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "caps results at 8"
```

Given: Redux store with 20 active sessions, all with titles matching "proj" (e.g., "proj-1" through "proj-20").

When: `query = "proj"`

Then: At most 8 results are returned.

Pass criteria: `results.length <= 8`.

---

#### T-UNIT-TS-007: Multi-field match scores higher than single-field match

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "multi-field match ranks above single-field match"
```

Given: Redux store with two sessions:
- Session A: `{ title: "auth-service", branch: "auth-feature", path: "/other" }` (matches "auth" in title AND branch)
- Session B: `{ title: "unrelated", branch: "unrelated", path: "/projects/auth-module" }` (matches "auth" only in path)

When: `query = "auth"`

Then: Session A has a lower score value (better match in Fuse.js convention) and appears first.

Pass criteria: `results[0].session.title === "auth-service"`, `results[0].score < results[1].score`.

---

### Detector (`web-app/src/lib/omnibar/detector.test.ts`)

#### T-UNIT-TS-008: Bare text resolves to InputType.SessionSearch

```
File: web-app/src/lib/omnibar/detector.test.ts
Function: "SessionSearchDetector" > "resolves bare word to SessionSearch"
```

Given: The default detector registry (includes SessionSearchDetector at priority 200).

When: `detect("squad")` is called.

Then: Result has `type === InputType.SessionSearch`.

Pass criteria: `result.type === InputType.SessionSearch`, `result.parsedValue === "squad"`.

---

#### T-UNIT-TS-009: Empty input does not resolve to SessionSearch

```
File: web-app/src/lib/omnibar/detector.test.ts
Function: "SessionSearchDetector" > "returns null for empty string"
```

Given: The default detector registry.

When: `detect("")` is called.

Then: Result does NOT have `type === InputType.SessionSearch`. The registry default (`InputType.Unknown`) applies.

Pass criteria: `result.type !== InputType.SessionSearch`. (The SessionSearchDetector's internal `null` return causes fallthrough to Unknown.)

---

#### T-UNIT-TS-010: Path input still resolves to LocalPath

```
File: web-app/src/lib/omnibar/detector.test.ts
Function: "LocalPath not displaced by SessionSearch"
```

Given: The default detector registry.

When: `detect("~/projects")` is called.

Then: Result has `type === InputType.LocalPath`.

Pass criteria: `result.type === InputType.LocalPath`.

---

#### T-UNIT-TS-011: GitHub shorthand still resolves correctly

```
File: web-app/src/lib/omnibar/detector.test.ts
Function: "GitHubShorthand not displaced by SessionSearch"
```

Given: The default detector registry.

When: `detect("org/repo")` is called.

Then: Result has `type === InputType.GitHubShorthand`.

Pass criteria: `result.type === InputType.GitHubShorthand`.

---

### usePathHistory (`web-app/src/lib/hooks/usePathHistory.test.ts`)

#### T-UNIT-TS-012: getAll returns top N entries by score when more than N exist

```
File: web-app/src/lib/hooks/usePathHistory.test.ts
Function: "getAll" > "returns top N by score"
```

Given: `usePathHistory` with 10 stored entries. The entry at index 3 has the highest score (most recent + most frequent).

When: `getAll(5)` is called.

Then: An array of exactly 5 entries is returned. The highest-scoring entry is at index 0.

Pass criteria: `result.length === 5`, `result[0]` corresponds to the highest-scored entry.

---

#### T-UNIT-TS-013: getAll returns all entries when count < limit

```
File: web-app/src/lib/hooks/usePathHistory.test.ts
Function: "getAll" > "returns all when count below limit"
```

Given: `usePathHistory` with 3 stored entries. Limit is 10.

When: `getAll(10)` is called.

Then: All 3 entries are returned, sorted by score descending.

Pass criteria: `result.length === 3`.

---

#### T-UNIT-TS-014: save then getAll includes the newly saved path

```
File: web-app/src/lib/hooks/usePathHistory.test.ts
Function: "getAll" > "includes newly saved path"
```

Given: `usePathHistory` with 2 stored entries.

When: `save("/home/user/new-repo")` is called, then `getAll(10)`.

Then: `/home/user/new-repo` appears in the returned entries.

Pass criteria: `result.some(e => e.path === "/home/user/new-repo") === true`.

---

#### T-UNIT-TS-015: Score ordering favors recently used over old

```
File: web-app/src/lib/hooks/usePathHistory.test.ts
Function: "getAll" > "score ordering: recent beats stale"
```

Given: Two entries in storage:
- Entry A: path `/recent`, last accessed 2 hours ago, accessed 1 time
- Entry B: path `/stale`, last accessed 3 weeks ago, accessed 5 times

When: `getAll(2)` is called.

Then: Entry A ranks higher (recency dominates over raw frequency for old entries).

Pass criteria: `result[0].path === "/recent"`. (Validates that `entryScore` function weights recency sufficiently.)

---

### OmnibarResultList component (`web-app/src/components/sessions/OmnibarResultList.test.tsx`)

#### T-UNIT-TS-016: Repo fuzzy search via Fuse.js matches non-contiguous characters

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
  OR: web-app/src/components/sessions/Omnibar.test.tsx
Function: "repo fuzzy search" > "matches non-contiguous characters in path"
```

Given: A Fuse instance configured with `{ keys: [{ name: "path", weight: 1.0 }], threshold: 0.4, ignoreLocation: true }`. History entries: `[{ path: "/home/user/stapler-squad" }]`.

When: `repoFuse.search("squad")` is called.

Then: The entry is returned.

Pass criteria: `results.length > 0`, `results[0].item.path` contains `"stapler-squad"`.

---

#### T-UNIT-TS-017: Session section hidden when sessionResults is empty

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "OmnibarResultList" > "hides SESSIONS header when sessionResults is empty"
```

Given: `OmnibarResultList` rendered with `sessionResults=[]` and `repoEntries=[{path: "/a"}]`.

When: Component renders.

Then: No element with text "SESSIONS" is present in the DOM.

Pass criteria: `queryByText("SESSIONS") === null`.

---

#### T-UNIT-TS-018: Repo section hidden when repoEntries is empty

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "OmnibarResultList" > "hides REPOS header when repoEntries is empty"
```

Given: `OmnibarResultList` rendered with `repoEntries=[]` and `sessionResults=[{session: mockSession, score: 0, matchedFields: []}]`.

When: Component renders.

Then: No element with text "REPOS" is present in the DOM.

Pass criteria: `queryByText("REPOS") === null`.

---

#### T-UNIT-TS-019: "+ New Session" item always renders

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "OmnibarResultList" > "always renders create-new item"
```

Given: `OmnibarResultList` rendered with both `sessionResults=[]` and `repoEntries=[]`.

When: Component renders.

Then: An element containing text matching "New Session" is present.

Pass criteria: `getByText(/New Session/i)` does not throw.

---

#### T-UNIT-TS-020: dropdownDismissed resets on mode transition

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "dropdownDismissed" > "resets on InputType change"
```

Given: `Omnibar` rendered. User has typed `"~/projects"` (triggers `LocalPath`). User pressed Escape (`dropdownDismissed = true`).

When: User backspaces fully to `"auth"` and waits >150ms (debounce fires, detection changes to `SessionSearch`).

Then: `dropdownDismissed` is reset to `false`. Session results section is visible in the DOM.

Pass criteria: `queryByText("SESSIONS")` is not null (or `queryAllByRole("option")` includes session results).

---

#### T-UNIT-TS-021: aria-activedescendant tracks highlighted item

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "accessibility" > "aria-activedescendant matches highlighted item id"
```

Given: `OmnibarResultList` with 2 session results and `highlightedIndex=1`.

When: Component renders.

Then: The input element's `aria-activedescendant` attribute equals the id of the second session result item (`omnibar-result-session-{session.id}`).

Pass criteria: `input.getAttribute("aria-activedescendant") === "omnibar-result-session-" + sessions[1].id`.

---

#### T-UNIT-TS-022: Arrow down moves highlight from last session to first repo (skips header)

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "keyboard navigation" > "arrow down skips section header from sessions to repos"
```

Given: `OmnibarResultList` with 2 session results and 2 repo entries. Parent has `highlightedIndex` in state. The last session item (index 1) is currently highlighted.

When: `ArrowDown` key event is fired on the input.

Then: `highlightedIndex` advances to 2 (first repo item), not to the REPOS section header.

Pass criteria: After arrow-down, the highlighted element has `id === "omnibar-result-repo-" + encodeURIComponent(repoEntries[0].path)`.

---

#### T-UNIT-TS-023: Enter on highlighted session calls onSessionSelect with correct session

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "keyboard navigation" > "Enter on session result triggers onSessionSelect"
```

Given: `OmnibarResultList` with 2 session results. `highlightedIndex=0` (first session). `onSessionSelect` is a jest mock.

When: `Enter` key event is fired (without metaKey).

Then: `onSessionSelect` is called once with `sessions[0]` as the argument.

Pass criteria: `onSessionSelect.mock.calls.length === 1`, `onSessionSelect.mock.calls[0][0].id === sessions[0].id`.

---

#### T-UNIT-TS-024: Enter on highlighted repo calls onRepoSelect with correct path

```
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "keyboard navigation" > "Enter on repo result triggers onRepoSelect"
```

Given: `OmnibarResultList` with 0 session results and 1 repo entry `{ path: "/home/user/myrepo" }`. `highlightedIndex=0`. `onRepoSelect` is a jest mock.

When: `Enter` key event is fired.

Then: `onRepoSelect` is called once with `"/home/user/myrepo"`.

Pass criteria: `onRepoSelect.mock.calls[0][0] === "/home/user/myrepo"`.

---

#### T-UNIT-TS-025: canSubmit is false when detection type is SessionSearch

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "canSubmit" > "false when InputType.SessionSearch"
```

Given: `Omnibar` rendered. Input value is `"squad"` (triggers `InputType.SessionSearch` after debounce).

When: The submit button (or Cmd+Enter handler) is interrogated.

Then: The form submit button is disabled OR `Cmd+Enter` does not trigger `onCreateSession`.

Pass criteria: Submit button has `disabled` attribute, OR `onCreateSession` mock is not called after `Cmd+Enter` with `SessionSearch` detection active.

---

## TypeScript Integration Tests

### Omnibar mode state machine (`web-app/src/components/sessions/Omnibar.test.tsx`)

#### T-INT-001: Omnibar opens in discovery mode showing recent sessions and repos

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "opens in discovery mode"
```

Given: `Omnibar` rendered with `isOpen=true`. Redux store has 3 active sessions. `usePathHistory` has 3 stored paths.

When: Component mounts (no input typed).

Then: Discovery mode UI is active. Session results section shows recent sessions. Repo results section shows recent repos. Form body (branch/program fields) is NOT visible.

Pass criteria: Session items visible (`getAllByRole("option").length > 0`). No element with label for branch input visible (`queryByLabelText(/branch/i) === null`).

---

#### T-INT-002: Empty state shows top-5 recent sessions from Redux by updatedAt

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "empty state shows 5 most recent sessions"
```

Given: Redux store with 8 active sessions with different `updatedAt` timestamps.

When: Omnibar opens with empty input.

Then: At most 5 session results are shown in discovery mode. They correspond to the 5 sessions with the most recent `updatedAt`.

Pass criteria: `getAllByRole("option", { name: /session/ }).length <= 5`. The displayed sessions match the 5 with highest `updatedAt` seconds.

---

#### T-INT-003: Typing a path prefix transitions to creation mode

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "path input triggers creation mode"
```

Given: `Omnibar` in discovery mode.

When: User types `"~/projects"` into the input and waits 200ms (debounce fires with `InputType.LocalPath`).

Then: Mode transitions to `"creation"`. Form body (branch and program fields) becomes visible. `OmnibarResultList` is hidden. `PathCompletionDropdown` is visible (or its aria-controls is present).

Pass criteria: Form body visible (`getByLabelText(/branch/i)` does not throw). Session result list absent (`queryByRole("listbox", { name: /results/ }) === null`).

---

#### T-INT-004: Selecting a session result calls onNavigateToSession and closes omnibar

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "session result selection navigates and closes"
```

Given: `Omnibar` in discovery mode with session results visible. `onNavigateToSession` and `onClose` are jest mocks.

When: User clicks (or presses Enter on) the first session result.

Then: `onNavigateToSession` is called with the correct session ID. `onClose` is called once.

Pass criteria: `onNavigateToSession.mock.calls[0][0] === sessions[0].id`, `onClose.mock.calls.length === 1`.

---

#### T-INT-005: Session results visible after Escape + backspace sequence

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "dropdownDismissed resets on mode transition"
```

Given: `Omnibar` open. User types `"~/proj"` (triggers LocalPath, path completion dropdown appears). User presses Escape (sets `dropdownDismissed=true`). User backspaces until input is `"auth"` and waits 200ms (triggers SessionSearch).

When: The debounce fires and mode becomes discovery with query "auth".

Then: Session results list is visible. Path completion dropdown is not visible. The "SESSIONS" section header is present.

Pass criteria: `queryByText("SESSIONS")` is not null. `queryByRole("listbox", { name: /completions/ }) === null`.

---

#### T-INT-006: Repo result selection pre-fills input and transitions to creation mode

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "repo select pre-fills path and enters creation mode"
```

Given: `Omnibar` in discovery mode with one repo entry `{ path: "/home/user/myrepo" }` visible.

When: User clicks the repo result item.

Then: Input value becomes `"/home/user/myrepo/"`. After debounce fires (200ms), mode becomes `"creation"`. Path completion dropdown appears. Form body is visible.

Pass criteria: `input.value === "/home/user/myrepo/"`. After 200ms: form body visible (`getByLabelText(/branch/i)` does not throw).

---

#### T-INT-007: First Escape clears result highlight without closing omnibar

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "first Escape clears highlight"
```

Given: `Omnibar` in discovery mode. User pressed ArrowDown (highlightedIndex = 0). `onClose` is a jest mock.

When: Escape is pressed once.

Then: `highlightedIndex` becomes -1 (no highlight). Omnibar is still open. `onClose` not called.

Pass criteria: No item has `aria-selected="true"`. `onClose.mock.calls.length === 0`.

---

#### T-INT-008: Second Escape closes omnibar

```
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "integration" > "second Escape closes omnibar"
```

Given: `Omnibar` open with no item highlighted (highlightedIndex = -1). `onClose` is a jest mock.

When: Escape is pressed.

Then: `onClose` is called once.

Pass criteria: `onClose.mock.calls.length === 1`.

---

## E2E Tests

E2E tests use the running application. Assume a test harness that can open a browser, interact with the UI, and assert on DOM state. Timing assertions use wall-clock measurements within the browser.

### T-E2E-001: Reach existing session in ≤3 keystrokes

```
Requirement: SC-1
```

Precondition: Application is running with at least 3 active sessions. One session has the title "auth-service".

Steps:
1. Press Cmd+K — opens Omnibar (keystroke 1).
2. Type "a" — filters session results to include "auth-service" (keystroke 2).
3. Press Enter — selects the highlighted/first result (keystroke 3).

Then: Omnibar is closed. The application has navigated to the "auth-service" session (e.g., session is visible/focused in the session list or terminal is active).

Pass criteria:
- Total keyboard events before session is active: exactly 3.
- Omnibar `isOpen` is false after Enter.
- Active session ID in app state matches "auth-service" session ID.
- Wall-clock time from step 1 to session active: <2 seconds total.

Failure modes to check: Omnibar stays open, wrong session selected, more than 3 key events required.

---

### T-E2E-002: Create session from recent repo in ≤5 keystrokes

```
Requirement: SC-2
```

Precondition: Application is running. `usePathHistory` has `"/home/user/myrepo"` stored as a recent path.

Steps:
1. Press Cmd+K — opens Omnibar (keystroke 1).
2. Type "my" — filters repo results to include "myrepo" (keystroke 2).
3. Press ArrowDown — highlights the first repo result (keystroke 3).
4. Press Enter — selects the repo, pre-fills input with `"/home/user/myrepo/"` (keystroke 4).
5. Press Cmd+Enter — creates the session (keystroke 5).

Then: A new session creation is initiated with path `/home/user/myrepo/`. `onCreateSession` is called (or equivalent API call fires).

Pass criteria:
- Total keyboard events: exactly 5.
- Session creation callback is triggered with path containing `"myrepo"`.
- Wall-clock time from step 1 to session creation initiated: <3 seconds.

---

### T-E2E-003: Path completion cache hit completes in <100ms

```
Requirement: SC-3 (cache hit)
```

Precondition: Application running. The directory `~/projects` exists with at least 5 subdirectories.

Steps:
1. Open Omnibar (Cmd+K).
2. Type `"~/projects/"` — triggers path completion. Wait for results (this is the cold call).
3. Close Omnibar (Escape).
4. Reopen Omnibar (Cmd+K).
5. Type `"~/projects/"` again — triggers path completion from cache.
6. Measure time from input change to results appearing.

Then: Results appear in the second call within 100ms.

Pass criteria: `responseTime < 100` (milliseconds, measured from the moment the input value is set to when result items appear in DOM). Verified by browser-side `performance.now()` instrumentation.

---

### T-E2E-004: Path completion cold start completes in <500ms

```
Requirement: SC-3 (cold start)
```

Precondition: Server restarted (cache empty). Directory `~/projects` exists.

Steps:
1. Open Omnibar (Cmd+K).
2. Type `"~/projects/"` — triggers first-ever path completion call.
3. Measure time from input change to results appearing.

Then: Results appear within 500ms.

Pass criteria: `responseTime < 500ms`. Any latency above 500ms is a regression on SC-3.

---

## Pitfall-Specific Tests

These tests exist to prevent the specific pitfalls identified in pitfalls.md from shipping undetected.

### P1-A: Detector does not block bare text (pitfalls.md § "Detector mode conflict")

#### T-PITFALL-001: Bare-text query must NOT resolve to InputType.Unknown

```
Pitfall: P1 — Detector mode conflict
File: web-app/src/lib/omnibar/detector.test.ts
Function: "pitfall" > "bare text does not resolve to Unknown"
```

Given: Default detector registry (with SessionSearchDetector registered).

When: `detect("squad")` is called.

Then: `result.type !== InputType.Unknown`.

Pass criteria: `result.type === InputType.SessionSearch`. If this test fails, session results will never appear for bare-word queries.

---

#### T-PITFALL-002: Bare-text query with hyphen resolves to SessionSearch

```
Pitfall: P1 — Detector mode conflict
File: web-app/src/lib/omnibar/detector.test.ts
Function: "pitfall" > "hyphenated bare text resolves to SessionSearch"
```

Given: Default detector registry.

When: `detect("my-feature")` is called.

Then: `result.type === InputType.SessionSearch` (not `InputType.Unknown`).

Pass criteria: `result.type === InputType.SessionSearch`.

---

### P1-B: Enter key routing is unambiguous (pitfalls.md § "Enter key ambiguity")

#### T-PITFALL-003: Enter on session result does not trigger session creation

```
Pitfall: P1 — Enter key ambiguity
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "pitfall" > "Enter on session result navigates, not creates"
```

Given: `Omnibar` in discovery mode with session results. `onCreateSession` and `onNavigateToSession` are jest mocks. First session result is highlighted (index 0).

When: `Enter` is pressed (without Cmd).

Then: `onNavigateToSession` is called. `onCreateSession` is NOT called.

Pass criteria: `onNavigateToSession.mock.calls.length === 1`, `onCreateSession.mock.calls.length === 0`.

---

#### T-PITFALL-004: Cmd+Enter in creation mode still creates session (routing not broken)

```
Pitfall: P1 — Enter key ambiguity (regression check)
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "pitfall" > "Cmd+Enter in creation mode calls onCreateSession"
```

Given: `Omnibar` in creation mode (path typed, `InputType.LocalPath` detected). Form is valid. `onCreateSession` is a jest mock.

When: `Cmd+Enter` is pressed.

Then: `onCreateSession` is called once. `onNavigateToSession` is NOT called.

Pass criteria: `onCreateSession.mock.calls.length === 1`, `onNavigateToSession.mock.calls.length === 0`.

---

### P1-C: PathCompletionService gets server-side cache (pitfalls.md § "stateless and uncached")

#### T-PITFALL-005: DirCache is wired into PathCompletionService (not a no-op)

```
Pitfall: P1 — PathCompletionService stateless
File: server/services/path_completion_service_test.go
Function: TestListPathCompletions_CacheHit (same as T-INT-GO-001)
```

Given: `PathCompletionService` (real implementation, not a mock).

When: `ListPathCompletions` is called twice for the same directory without any directory changes.

Then: `os.ReadDir` is called exactly once (cache serves the second call).

Pass criteria: `readDirCallCount == 1`. This test is identical to T-INT-GO-001 and dual-tagged as a pitfall guard.

---

### P2-A: BM25 tokenizer is not used for session search (pitfalls.md § "Ranking instability")

#### T-PITFALL-006: Session search does not invoke the BM25 SearchEngine

```
Pitfall: P2 — BM25 tokenizer bypassed
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "pitfall" > "does not import or call BM25 SearchEngine"
```

Given: Source of `useSessionSearch.ts` (inspected statically or via import graph).

When: The module's imports are examined.

Then: No import from `session/search/` or equivalent BM25 module is present. The search is performed exclusively via `Fuse` from `fuse.js`.

Pass criteria (static analysis): `grep -r "SearchEngine\|bm25\|tokenizer" web-app/src/lib/hooks/useSessionSearch.ts` returns empty. Alternatively, a Jest test that spies on the BM25 SearchEngine constructor confirms it is never called during `useSessionSearch` invocations.

---

### P2-B: dropdownDismissed stickiness is fixed (pitfalls.md § "dropdownDismissed resets on mode transition")

#### T-PITFALL-007: Session results are not suppressed after Escape + backspace

```
Pitfall: P2 — dropdownDismissed stickiness
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: Same as T-INT-005 — dual-tagged as a pitfall guard
```

Given: `Omnibar` open. User typed path (Escape dismissed path completion dropdown). User backspaced to session query.

When: Debounce fires with `InputType.SessionSearch`.

Then: Session result list is visible.

Pass criteria: `queryByText("SESSIONS")` is not null. This test is identical to T-INT-005 and dual-tagged.

---

### P2-C: Escape on result list has two-press contract (pitfalls.md § "Escape key")

#### T-PITFALL-008: First Escape on result list does not close omnibar

```
Pitfall: P2 — Escape key contract
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: Same as T-INT-007 — dual-tagged as a pitfall guard
```

Given: `Omnibar` open with a result highlighted.

When: Escape pressed once.

Then: Omnibar remains open. Highlight is cleared. `onClose` not called.

Pass criteria: `onClose.mock.calls.length === 0`. This test is identical to T-INT-007 and dual-tagged.

---

### P2-D: Separate ranked sections prevent repo paths from burying session results (pitfalls.md § "Mixed result-type ranking")

#### T-PITFALL-009: Sessions always render above repos in the result list

```
Pitfall: P2 — Mixed result-type ranking
File: web-app/src/components/sessions/OmnibarResultList.test.tsx
Function: "pitfall" > "sessions section renders above repos section in DOM order"
```

Given: `OmnibarResultList` with both session results and repo entries.

When: Component renders.

Then: In the DOM, all session `<li>` items appear before any repo `<li>` items (inspected by DOM order of elements with roles "option").

Pass criteria: `const options = getAllByRole("option"); const firstRepoIndex = options.findIndex(el => el.id.startsWith("omnibar-result-repo")); const lastSessionIndex = options.reduce((max, el, i) => el.id.startsWith("omnibar-result-session") ? i : max, -1); lastSessionIndex < firstRepoIndex`. (All sessions precede all repos in tab/navigation order.)

---

### P3-A: getMatching prefix-only limitation is avoided for recent repos (pitfalls.md § "getMatching does prefix matching only")

#### T-PITFALL-010: Repo path fuzzy search matches non-prefix fragment

```
Pitfall: P3 — usePathHistory.getMatching prefix-only
File: web-app/src/components/sessions/Omnibar.test.tsx
Function: "pitfall" > "repo search matches non-prefix fragment"
```

Given: `Omnibar` rendered. `usePathHistory` contains `{ path: "/home/user/auth-service" }`.

When: User types `"auth"` (which is not a prefix of the full path `/home/user/auth-service`).

Then: The repo entry for `/home/user/auth-service` appears in the displayed repo results section.

Pass criteria: An element with text `auth-service` is visible in the repo section of the result list. (This verifies the Fuse.js `repoFuse.search` path is used instead of `getMatching`.)

---

## Performance Benchmarks

These are measured and recorded as pass/fail against concrete thresholds. They must be executed as part of CI or as a named `make` target.

| Benchmark | Threshold | Measurement Method | Test ID |
|---|---|---|---|
| `useSessionSearch` with 500 sessions, single query | <16ms | Jest `performance.now()` timer around hook render | T-PERF-001 |
| Fuse repo search over 50 history entries | <5ms | Jest `performance.now()` timer | T-PERF-002 |
| DirCache Get (cache hit) | <100ms | Go benchmark `BenchmarkDirCache_Get` | T-PERF-003 |
| Full path completion round-trip (cold) | <500ms | E2E wall-clock (T-E2E-004) | T-PERF-004 |

#### T-PERF-001: useSessionSearch 500 sessions completes in <16ms

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts
Function: "performance" > "search completes in <16ms for 500 sessions"
```

Given: Redux store populated with 500 active sessions with varied titles, branches, and paths.

When: `useSessionSearch("auth")` is invoked and the result is read. The call is timed with `performance.now()`.

Then: Elapsed time is <16ms (one frame at 60fps — imperceptible to the user).

Pass criteria: `elapsed < 16`.

---

#### T-PERF-002: Fuse repo search 50 entries completes in <5ms

```
File: web-app/src/lib/hooks/useSessionSearch.test.ts (or Omnibar.test.tsx)
Function: "performance" > "repo Fuse search <5ms for 50 entries"
```

Given: A Fuse instance built from 50 `PathHistoryEntry` objects.

When: `fuse.search("squad")` is called and timed.

Then: Elapsed time <5ms.

Pass criteria: `elapsed < 5`.

---

#### T-PERF-003: DirCache Get (cache hit) completes in <100ms

```
File: server/services/dir_cache_test.go
Function: BenchmarkDirCache_Get
Run with: go test -bench=BenchmarkDirCache_Get -benchmem ./server/services/...
```

Given: A `DirCache` with 256 entries (maxSize). One entry is populated.

When: `Get` is called in a tight benchmark loop.

Then: Per-operation time is well below 100ms (expected: nanoseconds to microseconds given it is a map lookup under RLock).

Pass criteria: `BenchmarkDirCache_Get` reports <1ms/op. If it reports >100ms/op, the implementation is pathologically slow.

---

## Architecture / Constraint Tests

These are verified via static analysis or targeted test assertions, not runtime behavior.

#### T-ARCH-001: Zero new RPC endpoints for session search

```
Method: Static inspection
```

Given: The `server/` directory and `proto/session/v1/session.proto`.

When: The diff of `server/server.go`, `server/services/`, and `proto/session/v1/session.proto` is inspected.

Then: No new RPC handler registrations for session-search functionality. No new protobuf `rpc` method definitions. Session search results are computed client-side in `useSessionSearch.ts` from the Redux store.

Pass criteria: `git diff main -- server/server.go proto/` shows no new `Connect` handler registrations for session search. Zero new `rpc` definitions in `.proto` files.

---

#### T-ARCH-002: Zero proto changes

```
Method: Static inspection / CI enforcement
```

Given: The proto files in `proto/session/v1/`.

When: The diff from `main` is examined.

Then: No `.proto` files are modified.

Pass criteria: `git diff main -- proto/` is empty (or contains only Story 1/2/3 implementation additions to `.go` / `.ts` files — no `.proto` changes).

---

## Definition of Done

All items in this checklist must be green before the implementation is considered complete.

### Go Tests

- [ ] `go test -race ./server/services/... -run TestDirCache` — all 7 DirCache unit tests pass with no race conditions
- [ ] `go test ./server/services/... -run TestListPathCompletions` — both PathCompletionService integration tests pass
- [ ] `go test -bench=BenchmarkDirCache_Get -benchmem ./server/services/...` — benchmark reports <1ms/op

### TypeScript Tests

- [ ] `npm test -- --testPathPattern="useSessionSearch"` — all 7 hook unit tests pass (T-UNIT-TS-001 through T-UNIT-TS-007)
- [ ] `npm test -- --testPathPattern="detector"` — all 4 detector tests pass (T-UNIT-TS-008 through T-UNIT-TS-011), including pitfall guards T-PITFALL-001 and T-PITFALL-002
- [ ] `npm test -- --testPathPattern="usePathHistory"` — all 4 path history tests pass (T-UNIT-TS-012 through T-UNIT-TS-015)
- [ ] `npm test -- --testPathPattern="OmnibarResultList"` — all 9 result list tests pass (T-UNIT-TS-016 through T-UNIT-TS-024)
- [ ] `npm test -- --testPathPattern="Omnibar"` — integration tests T-INT-001 through T-INT-008 pass, pitfall tests T-PITFALL-003 through T-PITFALL-010 pass
- [ ] Performance benchmarks T-PERF-001 and T-PERF-002 pass (<16ms and <5ms respectively)

### E2E Tests

- [ ] T-E2E-001: Reach session in ≤3 keystrokes passes
- [ ] T-E2E-002: Create from recent repo in ≤5 keystrokes passes
- [ ] T-E2E-003: Path completion cache hit <100ms passes
- [ ] T-E2E-004: Path completion cold start <500ms passes

### Architecture Constraints

- [ ] T-ARCH-001: No new RPC endpoints (verified via `git diff main -- proto/ server/server.go`)
- [ ] T-ARCH-002: No proto changes (verified via `git diff main -- proto/`)

### Build Health

- [ ] `make lint` passes — no new lint errors introduced
- [ ] `make build` passes — no compilation errors in Go or TypeScript
- [ ] `go test -race ./server/services/...` passes — no data races

### Known Acceptable Gaps (not blocking DoD)

- Bug 1 (GitHubShorthand fires on `feature/auth-overhaul` style session names) — documented workaround: search for branch suffix. Tracked as future fix.
- Bug 2 (deleted paths in recent repos list) — mitigated by `✓`/`✗` path indicator. Tracked as future fix.
- Bug 3 (ConnectRPC transport created per-render) — predates this feature. Tracked as future fix.
- Bug 5 (DirCache mtime unreliable on NFS) — documented limitation. Acceptable for solo-practitioner local-filesystem use.
