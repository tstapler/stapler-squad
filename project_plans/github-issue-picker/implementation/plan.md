# Feature: GitHub Issue Picker — In-App Repo/Issue Browser

Date: 2026-07-03
Status: Ready for implementation
ADRs: ADR-001-local-repos-from-sessions-slice.md, ADR-002-frontend-localStorage-cache.md

---

## Ubiquitous Language

Every term below will appear as a type name, method name, or variable name in code.

| Term | Meaning |
|------|---------|
| `GitHubIssuePicker` | The new React component replacing the raw URL `<input>` in the GitHub import modal |
| `RepoSelector` | First-phase sub-view: renders repo list + search input |
| `IssueList` | Second-phase sub-view: renders issues for the selected repo |
| `IssueFilterBar` | Open/closed state toggle + label substring filter input |
| `IssueRow` | Single issue line item: number, title, state dot, label chips |
| `RepoChip` | Dismissible breadcrumb chip showing the selected repo ("owner/repo ×") |
| `GitHubRepoEntry` | Proto message and domain type: `{ owner, repo, isLocal, localPath, description }` |
| `GitHubIssueEntry` | Proto message and domain type: `{ number, title, state, url, labels[] }` |
| `useGitHubIssuePicker` | Custom React hook encapsulating all async state and caching |
| `issuePickerCache` | localStorage TTL cache utility module |
| `SearchGitHubRepos` | New BacklogService RPC: fetches GitHub repos via GitHub REST API (`/user/repos`, `/search/repositories`) using native Go HTTP client |
| `ListGitHubIssues` | New BacklogService RPC: fetches issues via GitHub REST API (`/repos/{owner}/{repo}/issues`, `/search/issues`) using native Go HTTP client |
| `PickerPhase` | Discriminated union: `"repo-selection" \| "issue-search"` |
| `generationRef` | `useRef<number>` counter guarding against stale async responses (pattern from `usePathCompletions`) |
| `REPOS_CACHE_KEY` | `"ssq:{origin}:gh-repos:v1"` — origin-namespaced localStorage key for repo list |
| `ISSUES_CACHE_KEY` | `"ssq:{origin}:gh-issues:{owner}/{repo}:{state}"` — per-repo issues cache key |
| `LAST_REPO_KEY` | `"ssq:{origin}:gh-last-repo"` — localStorage key for last-used repo pre-population |
| `REPOS_TTL_MS` | `14_400_000` (4 hours) — TTL for GitHub repo list cache |
| `ISSUES_TTL_MS` | `300_000` (5 minutes) — TTL for issue list cache |
| `ownerRepoPattern` | Validation regex for owner/repo strings: `^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` |
| `isLocal` | Boolean flag on `GitHubRepoEntry`: `true` = derived from sessions/worktrees (Redux store), `false` = from GitHub REST API |
| `lastUsedRepo` | The `{ owner, repo }` object restored from localStorage on picker open |
| `issueStateFilter` | `"open" \| "closed" \| "all"` — drives the `state` query param in GitHub REST API call |
| `labelFilter` | Client-side substring filter applied to already-fetched issues; no new RPC |
| `ghRepoJSON` | Private Go struct for GitHub REST API `/user/repos` response fields |
| `ghIssueListEntry` | Private Go struct for GitHub REST API `/repos/{owner}/{repo}/issues` response fields |

---

## Observability

**Logs**: Use the existing `log.WarningLog.Printf` / `log.InfoLog.Printf` pattern already in `backlog_service.go`. Log keys:
- `[SearchGitHubRepos] GitHub API request failed: %v`
- `[ListGitHubIssues] GitHub API request failed for %s/%s: %v`
- `[ListGitHubIssues] GitHub token unavailable; returning CodeUnavailable`

**Metrics**: None required (personal dev tool, no Prometheus).

**Alerts**: None required.

**Feature flag**: None. The component replaces the URL input directly; the existing modal is the rollout gate (if disabled, revert the `page.tsx` change).

**Rollback procedure**: Revert `web-app/src/app/backlog/page.tsx` to restore the raw `<input type="url">`. No database migrations; no data to roll back.

**Staged rollout**: Not applicable (personal dev tool, single user).

---

## Open Questions

None. All open questions from the requirements document were resolved in research:
- Inline within existing modal (not expanded panel) — research/ux.md §7
- Debounced (150ms) for issue title search — research/pitfalls.md §4.3
- Cache TTL: repos 4h, issues 5min — research/architecture.md §2.2

---

## Task Dependency Diagram

```
EPIC 1: Backend (SEQUENTIAL — 1.2.1 must complete before 1.3.1; both depend on github/repos.go)
  1.1.1 (proto messages)
    └─► 1.1.2 (make generate-proto)
          └─► 1.2.1 (github/repos.go + SearchGitHubRepos handler)
                ├─► 1.2.2 (SearchGitHubRepos tests)
                └─► 1.3.1 (ListGitHubIssues handler — uses github.ListRepoIssues from 1.2.1)
                      └─► 1.3.2 (ListGitHubIssues tests)

EPIC 2: Frontend Hook + Cache
  [after 1.1.2]
  2.1.1 (issuePickerCache utility)    }
  2.2.1 (useBacklogService extensions) } parallel
                    │
                    └─► 2.3.1 (useGitHubIssuePicker — repo phase)
                          └─► 2.3.2 (useGitHubIssuePicker — issue phase)

EPIC 3: Component + Integration
  3.1.1 (GitHubIssuePicker.css.ts) ──────────────────┐
  [after 2.3.2]                                       │
  3.2.1 (RepoSelector + keyboard nav) ◄───────────────┘
    └─► 3.2.2 (IssueList + IssueFilterBar + IssueRow)
          └─► 3.2.3 (URL-paste + two-level Escape + auth error + RepoChip)
                └─► 3.3.1 (integration into backlog/page.tsx)
                      ├─► 3.4.1 (RTL unit tests)
                      ├─► 3.4.2 (Playwright e2e)
                      └─► 3.4.3 (feature registry updates)
```

---

## Creative Approach Selection

Three high-level approaches were evaluated before committing to architecture:

| Approach | Key Strength | Key Weakness |
|----------|-------------|--------------|
| **A: Two-stage combobox (repo → issue)** | Clear search space; matches existing PathCompletionDropdown template; all research aligns | More component code than B |
| B: URL-first with "Browse" button | Zero risk to existing flow; ~20 LOC | Doesn't satisfy "no leaving the app" requirement; worse UX |
| C: Unified search (repos + issues simultaneously) | Single interaction surface; feels modern | Ambiguous mixed results (repo title vs issue title); over-engineered prop API for one user |

**Chosen: Approach A.** The 2D search space (repo × issue) makes a unified list confusing; Approach B doesn't meet the core requirement. All existing patterns (PathCompletionDropdown, usePathCompletions) align with Approach A.

---

## Pattern Decisions

| Decision | Pattern Chosen | Rejected Pattern |
|----------|---------------|-----------------|
| Local repo source | Read from Redux `sessionsSlice` (Session.githubOwner/githubRepo) — no RPC | SearchGitHubRepos with local_only flag (extra round-trip) |
| Repo caching | localStorage TTL cache in frontend | Backend sync.Map (no multi-user benefit; complicates BacklogService) |
| Dropdown item interaction | `onMouseDown + e.preventDefault()` | `onClick` (has onBlur-before-onClick race bug in AutocompleteInput) |
| Debounce impl | Inline `useRef<NodeJS.Timeout>` + clearTimeout | Shared `useDebounce` hook (doesn't exist in codebase) |
| CSS | vanilla-extract `.css.ts` colocated | CSS modules (prohibited for new components by ADR-009) |
| GitHub API | Native Go HTTP client via `github/http_client.go` (`newGHRequest` + `ghHTTPClient`) | `gh` CLI subprocess or direct frontend REST call (exposes token) |

---

## Epic 1 — Backend: New RPCs for GitHub Data

**Goal**: Expose `SearchGitHubRepos` and `ListGitHubIssues` as ConnectRPC methods on `BacklogService`, using the native Go HTTP client from `github/http_client.go` (the `newGHRequest` + `ghHTTPClient` pattern, same as `GetPRForBranch`).

**As a** backend subsystem, **I want** two new RPCs for browsing GitHub repos and issues, **so that** the frontend picker can fetch data via the authenticated GitHub REST API without exposing credentials to the browser.

---

### Story 1.1 — Proto Definitions

**Acceptance Criteria**:
- `SearchGitHubReposRequest`, `SearchGitHubReposResponse`, `GitHubRepoEntry`, `ListGitHubIssuesRequest`, `ListGitHubIssuesResponse`, `GitHubIssueEntry` messages are defined in `backlog.proto` with field numbers continuing from the current max.
- `SearchGitHubRepos` and `ListGitHubIssues` RPCs are declared on `BacklogService`.
- `make generate-proto` runs without error and produces updated Go + TypeScript bindings.
- Given the current last field number in `BacklogService` is 18 RPCs defined and `CancelTriageResponse.cancelled = 1`, When the proto is compiled, Then no field number collision errors occur and generated Go types include `SearchGitHubReposRequest`.

**Files**:
- `proto/session/v1/backlog.proto`

#### Task 1.1.1 — Add messages and RPCs to backlog.proto

**Steps**:
1. After the existing `CancelTriageResponse` message, append:
   - `GitHubRepoEntry` (owner, repo, is_local, local_path, description — fields 1–5)
   - `GitHubIssueEntry` (number, title, state, url, repeated labels, body — fields 1–6)
   - `SearchGitHubReposRequest` (query, limit — fields 1–2)
   - `SearchGitHubReposResponse` (repeated repos — field 1)
   - `ListGitHubIssuesRequest` (owner, repo, state, search, limit — fields 1–5)
   - `ListGitHubIssuesResponse` (repeated issues — field 1)
2. Add `SearchGitHubRepos` and `ListGitHubIssues` to the `BacklogService` service block after the existing `GetSyncHistory` RPC.
3. Do not touch any existing message field numbers.

**Files**: `proto/session/v1/backlog.proto`

#### Task 1.1.2 — Regenerate bindings and verify

**Steps**:
1. Run `make generate-proto`.
2. Confirm `session/gen/proto/go/session/v1/backlog_pb.go` contains `SearchGitHubReposRequest` and `ListGitHubIssuesRequest`.
3. Confirm `web-app/src/gen/session/v1/backlog_pb.ts` exports `SearchGitHubReposRequest` and `GitHubIssueEntry`.
4. Run `go build ./...` to verify the Go build is not broken.

**Files**: `session/gen/proto/go/session/v1/backlog_pb.go`, `web-app/src/gen/session/v1/backlog_pb.ts`

---

### Story 1.2 — SearchGitHubRepos Handler

**Acceptance Criteria**:
- The handler calls `GET https://api.github.com/user/repos?per_page={limit}&sort=pushed` via `github.newGHRequest(ctx, ...)` + `github.ghHTTPClient.Do(req)`. When `query` is non-empty, it calls `GET /search/repositories?q={query}&per_page={limit}` instead.
- Given `getGHToken(ctx)` returns `""` (no token configured), When `SearchGitHubRepos` is called, Then the handler returns `connect.CodeUnavailable` (not `CodeInternal`).
- Given a valid GitHub token with 3 repos accessible, When `SearchGitHubRepos({query: "", limit: 30})` is called, Then the response contains 3 `GitHubRepoEntry` records with `is_local: false`.
- The `owner` and `repo` fields in returned entries are validated against `ownerRepoPattern` before being included in the response (entries with malformed names are dropped, not errors).

**Files**:
- `server/services/backlog_service.go`
- `server/services/backlog_service_test.go`

#### Task 1.2.1 — Implement SearchGitHubRepos handler body

**Steps**:
1. Add private `ghRepoJSON` struct matching GitHub REST API `/user/repos` response: `{ FullName string \`json:"full_name"\`; Name string \`json:"name"\`; Owner struct{ Login string \`json:"login"\` } \`json:"owner"\`; Description string \`json:"description"\`; Private bool \`json:"private"\`; PushedAt string \`json:"pushed_at"\` }`.
2. Implement `SearchGitHubRepos` handler: call `github.SearchUserRepos(ctx, query, limit)` (domain function — see step 3); convert returned slice to `[]*sessionv1.GitHubRepoEntry` with `IsLocal: false`; annotate with `// +api: backlog:search-github-repos`.
   - Guard: `if s.storage == nil { return nil, connect.NewError(connect.CodeUnavailable, ...) }`
   - Validate `req.Msg.Limit`: default to 30, cap at 100.
   - `github.ErrNotAuthenticated` → return `CodeUnavailable`; other errors → `CodeInternal`.
3. **Add `github/repos.go`** with domain functions. Keep HTTP internals unexported. Architecture review finding: do NOT export `newGHRequest` or `ghHTTPClient` — add domain functions that wrap them:
   ```go
   // RepoResult and IssueResult are the domain return types (no proto dependency)
   type RepoResult struct { Owner, Repo, Description string; Private bool }
   type IssueResult struct { Number int; Title, State, URL string; Labels []string }
   var ErrNotAuthenticated = errors.New("github token not configured")

   // For testability: package-level base URL var overrideable in tests
   var GhBaseURL = "https://api.github.com/"  // add to http_client.go

   func SearchUserRepos(ctx context.Context, query string, limit int) ([]RepoResult, error) { ... }
   func ListRepoIssues(ctx context.Context, owner, repo, state, search string, limit int) ([]IssueResult, error) { ... }
   ```
   Both functions: check `getGHToken(ctx) == ""` → return `ErrNotAuthenticated`; use `newGHRequest` + `ghHTTPClient.Do`; return typed results.

**Files**: `server/services/backlog_service.go`, `github/repos.go`, `github/http_client.go` (add `var ghBaseURL`)

#### Task 1.2.2 — Unit tests for SearchGitHubRepos

**Steps**:
1. Add `TestSearchGitHubRepos_NilStorage` — expects `CodeUnavailable`.
2. Add `TestSearchGitHubRepos_DefaultLimit` — passes `Limit: 0`, expects handler uses limit=30.
3. Add `TestSearchGitHubRepos_NoToken` — set `github.ghBaseURL` to a test server that returns 401; expects `CodeUnavailable`.
4. **Testability pattern**: `ghBaseURL` is a package-level `var` in `github/http_client.go` (initialized to `"https://api.github.com/"`). Tests override it: `github.GhBaseURL = ts.URL + "/"` then `defer func() { github.GhBaseURL = "https://api.github.com/" }()`. The `newGHRequest` function uses `ghBaseURL` instead of the hardcoded string — this is required for `httptest.Server` to intercept requests.

**Files**: `server/services/backlog_service_test.go`, `github/repos_test.go`

---

### Story 1.3 — ListGitHubIssues Handler

**Acceptance Criteria**:
- The handler calls `github.ListRepoIssues(ctx, owner, repo, state, search, limit)` (domain function in `github/repos.go`). When `search` is empty it hits `GET /repos/{owner}/{repo}/issues?state={state}&per_page={limit}`; when non-empty it hits `GET /search/issues?q={search}+in:title+...`. The HTTP internals are encapsulated inside `github/repos.go` — the handler never calls `newGHRequest` or `ghHTTPClient` directly.
- Given no GitHub token is configured, When `ListGitHubIssues` is called, Then `connect.CodeUnavailable` is returned.
- Given `owner = ""` or `repo = ""`, When `ListGitHubIssues` is called, Then `connect.CodeInvalidArgument` is returned.
- Given `owner` contains a shell metacharacter (e.g., `"a b"`), When `ListGitHubIssues` is called, Then `connect.CodeInvalidArgument` is returned (validated against `ownerRepoPattern` before constructing the URL — defense in depth even though we're not using a shell).
- Given `owner/repo` has 5 open issues matching `search="login"`, When `ListGitHubIssues({owner: "tstapler", repo: "stapler-squad", state: "open", search: "login", limit: 30})` is called, Then the response contains up to 5 `GitHubIssueEntry` records with `state: "OPEN"` and titles containing "login".

**Files**:
- `server/services/backlog_service.go`
- `server/services/backlog_service_test.go`

#### Task 1.3.1 — Implement ListGitHubIssues handler body

**Steps**:
1. Add private `ghIssueListJSON` struct matching GitHub REST API response: `{ Number int \`json:"number"\`; Title string \`json:"title"\`; State string \`json:"state"\`; HTMLURL string \`json:"html_url"\`; Labels []struct{ Name string \`json:"name"\` } \`json:"labels"\` }` — distinct from the existing `ghIssueJSON` which includes `Body`.
2. Add private `ghIssueSearchResponse` struct: `{ Items []ghIssueListJSON \`json:"items"\` }` (for the search API endpoint).
3. Implement `ListGitHubIssues` handler: call `github.ListRepoIssues(ctx, owner, repo, state, search, limit)` (domain function in `github/repos.go` — see Task 1.2.1 step 3); convert results to `[]*sessionv1.GitHubIssueEntry`; annotate with `// +api: backlog:list-github-issues`.
   - Guard: storage nil → `CodeUnavailable`.
   - Validate `owner` and `repo` against `ownerRepoPattern` regex; empty → `CodeInvalidArgument`.
   - Default `state` to `"open"` if empty; accept `"open"`, `"closed"`, `"all"`.
   - `github.ErrNotAuthenticated` → `CodeUnavailable`; other errors → `CodeInternal`.
   - State uppercased in proto conversion (`"open"` → `"OPEN"`) to match proto convention.

**Files**: `server/services/backlog_service.go`

#### Task 1.3.2 — Unit tests for ListGitHubIssues

**Steps**:
1. `TestListGitHubIssues_NilStorage` → `CodeUnavailable`.
2. `TestListGitHubIssues_EmptyOwner` → `CodeInvalidArgument`.
3. `TestListGitHubIssues_EmptyRepo` → `CodeInvalidArgument`.
4. `TestListGitHubIssues_InvalidOwnerChars` (owner = `"a b"`) → `CodeInvalidArgument`.
5. `TestListGitHubIssues_DefaultState` — state="" defaults to "open" (assert via `httptest.Server` using the `ghBaseURL` override pattern from Task 1.2.2).
6. `TestListGitHubIssues_SearchUsesSearchAPI` — search non-empty → assert request URL contains `/search/issues?q=`.

**Files**: `server/services/backlog_service_test.go`

---

## Epic 2 — Frontend Hook and Cache Layer

**Goal**: Provide a typed hook and localStorage cache utility that encapsulate all async state (repo fetch, issue fetch, loading, error) and implement debounce + AbortController + generation counter for race-free data fetching.

**As a** React component, **I want** a hook that manages repo/issue loading state and caching, **so that** the `GitHubIssuePicker` component can remain pure presentation logic.

---

### Story 2.1 — issuePickerCache Utility

**Acceptance Criteria**:
- `readCache<T>(key, ttlMs)` returns `T` if valid, `null` if missing or expired.
- `writeCache<T>(key, data)` stores data with a `fetchedAt` timestamp; silently ignores `QuotaExceededError` and `SecurityError`.
- All get/set operations are wrapped in `try/catch`.
- Cache keys include `window.location.origin` prefix to prevent collision between `localhost:8543` (prod) and `localhost:8544` (e2e).
- The `REPOS_CACHE_KEY` helper function takes no args and returns `"ssq:{origin}:gh-repos:v1"`.
- The `ISSUES_CACHE_KEY` helper takes `(owner, repo, state)` and returns the namespaced key.
- Given: a call to `writeCache(REPOS_CACHE_KEY(), repos)` followed by a wait of `REPOS_TTL_MS + 1`, When `readCache(REPOS_CACHE_KEY(), REPOS_TTL_MS)` is called, Then it returns `null` (expired).

**Files**: `web-app/src/lib/utils/issuePickerCache.ts`

#### Task 2.1.1 — Create issuePickerCache.ts

**Steps**:
1. Define `CacheEntry<T> = { data: T; fetchedAt: number }`.
2. Implement `readCache<T>(key: string, ttlMs: number): T | null` with try/catch and TTL check.
3. Implement `writeCache<T>(key: string, data: T): void` with try/catch (log warning on error, never throw).
4. Export key functions. **CRITICAL: `typeof window` guard must be the first line**, not a prose suggestion:
   ```ts
   export function reposCacheKey(): string {
     if (typeof window === "undefined") return ""; // SSR guard — first line
     return "ssq:" + window.location.origin + ":gh-repos:v1";
   }
   export function issuesCacheKey(owner: string, repo: string, state: string): string {
     if (typeof window === "undefined") return "";
     return `ssq:${window.location.origin}:gh-issues:${owner}/${repo}:${state}`;
   }
   export function lastRepoCacheKey(): string {
     if (typeof window === "undefined") return "";
     return "ssq:" + window.location.origin + ":gh-last-repo";
   }
   ```
   `readCache` and `writeCache` must also start with `if (typeof window === "undefined") return null;` / `return;` respectively — Next.js SSR build will crash without this.
5. Export constants `REPOS_TTL_MS = 14_400_000` and `ISSUES_TTL_MS = 300_000`.

**Files**: `web-app/src/lib/utils/issuePickerCache.ts`

---

### Story 2.2 — useBacklogService Extensions

**Acceptance Criteria**:
- `useBacklogService` exposes `searchGitHubRepos(query, limit)` and `listGitHubIssues(owner, repo, state, search, limit)` methods.
- Domain types `GitHubRepo` and `GitHubIssue` are exported from `useBacklogService.ts`.
- `searchGitHubRepos` catches `ConnectError` with `code === Code.Unavailable` and re-throws as a typed `GitHubAuthError` (a plain `Error` subclass with `isAuthError: true`) so the picker can show the auth banner.
- Given the ConnectRPC client returns a response with 3 repos, When `searchGitHubRepos("", 30)` resolves, Then it returns `GitHubRepo[]` with `isLocal: false` for all three.

**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

#### Task 2.2.1 — Add domain types + hook methods

**Steps**:
1. Add exported interfaces:
   ```ts
   export interface GitHubRepo { owner: string; repo: string; isLocal: boolean; localPath?: string; description?: string; }
   export interface GitHubIssue { number: number; title: string; state: string; url: string; labels: string[]; }
   export class GitHubAuthError extends Error { readonly isAuthError = true; }
   ```
2. Add mapping functions `mapGitHubRepo` and `mapGitHubIssue` from proto types.
3. Add `searchGitHubRepos(query: string, limit?: number): Promise<GitHubRepo[]>` to `UseBacklogServiceReturn` interface and implement with `useCallback`.
4. Add `listGitHubIssues(owner, repo, state, search?, limit?): Promise<GitHubIssue[]>` similarly.
5. In both methods, catch `ConnectError` where `.code === Code.Unavailable` and throw `new GitHubAuthError("GitHub CLI not authenticated")`.
6. Add both to the `useMemo` return object.

**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

---

### Story 2.3 — useGitHubIssuePicker Hook

**Acceptance Criteria**:
- On mount, the hook reads `lastUsedRepo` from localStorage and, if present, pre-populates `selectedRepo` and immediately triggers an issue fetch.
- Repo fetch from Redux `sessionsSlice` is synchronous and returns within one render cycle (no loading state for local repos).
- GitHub repo fetch (RPC) shows `isReposLoading: true` and completes within 2s on fast networks; results are written to `issuePickerCache`.
- Issue fetch is debounced at 150ms and uses AbortController + `generationRef` to prevent stale results (same pattern as `usePathCompletions.ts`).
- On repo change (switching from repo A to repo B mid-flight), the in-flight issue request for repo A is cancelled via AbortController.
- On unmount, all pending timers and AbortControllers are cleaned up.
- Given `selectedRepo = { owner: "tstapler", repo: "stapler-squad" }` and `issueStateFilter = "open"`, When `loadIssues()` is called, Then `listGitHubIssues("tstapler", "stapler-squad", "open", "", 30)` is invoked after the debounce; `issues` state updates to the returned list.
- Given issues were cached 3 minutes ago for `tstapler/stapler-squad:open`, When `loadIssues()` is called, Then no RPC is made and the cached issues are returned.

**Files**: `web-app/src/lib/hooks/useGitHubIssuePicker.ts`

#### Task 2.3.1 — Repo phase: local repos + RPC + last-used restore

**Steps**:
1. Define hook return type `UseGitHubIssuePickerReturn` (localRepos, githubRepos, isReposLoading, repoError, selectedRepo, setSelectedRepo, phase, setPhase, lastUsedRepo, clearLastUsedRepo).
2. Read local repos from `useAppSelector(selectAllSessions)` inside the hook; derive `localRepos: GitHubRepo[]` via `useMemo` filtering sessions where `s.githubOwner` is non-empty, deduplicated by `owner/repo`.
3. Restore `lastUsedRepo` from `readCache(lastRepoCacheKey(), Infinity)` in a `useEffect` on mount (Infinity TTL — last-used repo never expires, user clears manually).
4. Implement `loadGitHubRepos()` as a `useCallback`: check `readCache(reposCacheKey(), REPOS_TTL_MS)`; if hit → set `githubRepos` without RPC; if miss → call `searchGitHubRepos("", 30)`, write cache, set state.
5. Call `loadGitHubRepos()` from a `useEffect([])` on mount (deferred GitHub tier).

**Files**: `web-app/src/lib/hooks/useGitHubIssuePicker.ts`

#### Task 2.3.2 — Issue phase: debounce + generation counter + cache

**Steps**:
1. Add state: `issues`, `isIssuesLoading`, `issueError`, `issueSearch`, `issueStateFilter`.
2. Implement issue fetch `useEffect` watching `[selectedRepo, issueSearch, issueStateFilter]`. **CRITICAL: gen must be captured BEFORE setTimeout, not inside it.** Use this exact skeleton:
   ```ts
   useEffect(() => {
     if (!selectedRepo) { setIssues([]); return; }
     const gen = ++generationRef.current; // captured HERE, not inside callback
     const cached = readCache(issuesCacheKey(owner, repo, state), ISSUES_TTL_MS);
     if (cached) { setIssues(cached); return; }
     setIsIssuesLoading(true);
     const controller = new AbortController(); // created HERE so cleanup can abort it
     const timer = setTimeout(async () => {
       try {
         const result = await listGitHubIssues(owner, repo, state, issueSearch, 30);
         if (gen !== generationRef.current) return; // stale — discard
         writeCache(issuesCacheKey(owner, repo, state), result);
         setIssues(result);
       } catch (err) { if (gen === generationRef.current) setIssueError(err); }
       finally { if (gen === generationRef.current) setIsIssuesLoading(false); }
     }, 150);
     return () => { clearTimeout(timer); controller.abort(); };
   }, [selectedRepo, issueSearch, issueStateFilter]);
   ```
   (Pre-mortem P1: placing `const gen` inside the setTimeout callback means gen captures the already-incremented value and the race guard never fires — stale issues silently land.)
3. Save `selectedRepo` to localStorage on selection: `writeCache(lastRepoCacheKey(), selectedRepo)` (no TTL — persist indefinitely).
4. Export `labelFilter` + `setLabelFilter` state: client-side `filteredIssues` derived via `useMemo` filtering `issues` where any label includes `labelFilter` (case-insensitive substring).

**Files**: `web-app/src/lib/hooks/useGitHubIssuePicker.ts`

---

## Epic 3 — Component and Integration

**Goal**: Build the `GitHubIssuePicker` React component (two-phase combobox with keyboard nav, ARIA attrs, URL paste detection, two-level Escape, auth error state) and wire it into the backlog modal.

**As a** solo developer, **I want** to browse and select a GitHub issue from within the backlog page, **so that** I can import it without leaving the app to look up the issue URL.

---

### Story 3.1 — Vanilla-Extract CSS

**Acceptance Criteria**:
- All styles reference `vars.*` tokens from `@/styles/theme.css` — no hardcoded hex values.
- Component has a fixed-height `repoList` and `issueList` containers with `overflowY: "auto"` (max-height ~240px each).
- `stateOpen` dot is `#2da44e`, `stateClosed` is `#8250df` — defined as CSS custom properties on the picker container, not hardcoded in the `style` calls.
- Skeleton rows use `@keyframes` pulse animation for loading state.
- No `.module.css` files created for this component.

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.css.ts`

#### Task 3.1.1 — Create GitHubIssuePicker.css.ts

**Steps**:
1. Import `{ style, keyframes, globalStyle }` from `@vanilla-extract/css` and `{ vars }` from `@/styles/theme.css`.
2. Define: `container`, `repoSearchInput`, `repoList`, `repoOption`, `repoOptionSelected`, `repoOptionLocal`, `repoChip`, `repoChipClose`, `issueSearchInput`, `issueList`, `issueRow`, `issueRowSelected`, `issueNumber`, `issueTitle`, `issueStateDot`, `issueStateOpen`, `issueStateClosed`, `labelChip`, `filterBar`, `stateToggle`, `stateToggleActive`, `authBanner`, `emptyState`, `skeletonRow`, `loadingPulse`.
3. All `repoOption`/`issueRow` items use `selectors: { "&:hover": { background: vars.color.hoverBackground } }`.
4. `skeletonRow` uses the `loadingPulse` keyframe (`opacity 0.4 → 0.9` at 50%`).
5. GitHub state colors: define `vars` fallbacks in `container` using vanilla-extract `@layer` or use inline CSS custom property pattern documented in ADR-009 (`style={{ '--github-open': '#2da44e' }}`).

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.css.ts`

---

### Story 3.2 — GitHubIssuePicker Component

**Acceptance Criteria**:
- Repo selector input has `role="combobox"`, `aria-expanded`, `aria-controls="gh-repo-listbox"`, `aria-activedescendant`.
- Issue list input has `role="combobox"`, `aria-expanded`, `aria-controls="gh-issue-listbox"`, `aria-activedescendant`.
- All list items use `onMouseDown={(e) => { e.preventDefault(); onSelect(item); }}` — NOT `onClick`.
- ArrowDown/ArrowUp/Enter/Escape key handlers are on the `onKeyDown` prop of each input (not global listeners).
- First Escape in `"issue-search"` phase: if `picker.issueSearch !== "" || picker.selectedRepo !== null`, sets `phase = "repo-selection"`, clears `selectedRepo`, does NOT close the modal. If BOTH `issueSearch === ""` AND `selectedRepo === null`, calls `onClose()`. (Adversarial review fix: "or when issueSearch is empty" in the original was ambiguous — this spec is unambiguous.)
- First Escape in `"repo-selection"` phase always calls `onClose()`.
- When the search input value matches `issueURLPattern` or `issueShorthandPattern`, a chip "Import #N from owner/repo directly →" appears below the input; clicking/Enter on it calls `onImport(url)` directly. **URL detection applies to BOTH the repo search input (phase 1) and the issue search input (phase 2)** — extract `detectIssueUrl(value)` as a shared helper called from both `onChange` handlers.
- Given user types "https://github.com/tstapler/stapler-squad/issues/42" into the repo input OR issue search input, When the value is detected as a GitHub issue URL, Then a "Direct import" affordance appears and pressing Enter calls `onImport("https://github.com/tstapler/stapler-squad/issues/42")` without going through repo/issue selection.
- Given `repoError.isAuthError === true`, the picker shows an auth banner "GitHub not authenticated. Run `gh auth login` or set a GITHUB_TOKEN environment variable." with a "Try again" button that calls `reloadRepos()`. (Note: do NOT say "GitHub CLI not authenticated" — the backend uses the Go HTTP client, not gh CLI directly, though gh auth login remains the setup mechanism.)
- `selectedRepo` renders as `RepoChip` (`owner/repo ×`) above the issue search input when in `"issue-search"` phase; clicking `×` sets `phase = "repo-selection"` and clears `selectedRepo`.
- Given: `issuelist` has 5 items, `selectedIndex = 2`, When user presses `ArrowDown`, Then `selectedIndex` becomes 3.

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`

#### Task 3.2.1 — RepoSelector with keyboard navigation

**Steps**:
1. Create `GitHubIssuePicker.tsx` with imports: the hook, CSS, `useAppSelector`, `selectAllSessions`.
2. Implement `RepoSelector` sub-component accepting `{ localRepos, githubRepos, isLoading, error, onSelect, onClose }`.
3. Input: `role="combobox"`, `aria-expanded={showList}`, `aria-controls="gh-repo-listbox"`, `aria-autocomplete="list"`, `aria-activedescendant={selectedIndex >= 0 ? \`gh-repo-listbox-option-${selectedIndex}\` : undefined}`, `placeholder="Search repos or paste a GitHub issue URL…"`.
4. List: `<ul id="gh-repo-listbox" role="listbox">`.
5. Each item: `<li id={"gh-repo-listbox-option-" + i} role="option" aria-selected={i === selectedIndex} onMouseDown={(e) => { e.preventDefault(); onSelect(repo); }}>`.
6. `onKeyDown` on the input: ArrowDown → increment selectedIndex (clamped); ArrowUp → decrement (stop at -1, returns focus to input); Enter → select highlighted repo; Escape → call `onClose()`.
7. Client-side filter: `displayedRepos = useMemo(() => [...localRepos, ...githubRepos].filter(r => `${r.owner}/${r.repo}`.toLowerCase().includes(query.toLowerCase())), [localRepos, githubRepos, query])`.
8. Show divider `<li role="presentation">` between local and GitHub tiers if both present.
9. Show `SkeletonRows` (3 skeleton `<li>`) while `isLoading && githubRepos.length === 0`.
10. URL paste detection: in the input `onChange`, test value against `issueURLPattern` regex; if match, set `detectedIssueUrl` state — render a "Direct import" affordance.

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`

#### Task 3.2.2 — IssueList + IssueFilterBar + IssueRow

**Steps**:
1. Implement `IssueFilterBar` sub-component: three buttons (Open / Closed / All) mapped to `issueStateFilter` value; a text input for `labelFilter`.
2. Implement `IssueRow` sub-component: `issueNumber` span, `issueTitle` span, `issueStateDot` span (green if state=`"OPEN"`, purple if `"CLOSED"`), up to 3 `labelChip` spans.
3. Implement `IssueList` sub-component:
   - Input: `role="combobox"`, `aria-expanded`, `aria-controls="gh-issue-listbox"`, `aria-autocomplete="list"`, `aria-activedescendant`. `onChange` calls `detectIssueUrl(value)` shared helper (same pattern as RepoSelector, enables URL paste shortcut in issue phase too).
   - List: `<ul id="gh-issue-listbox" role="listbox">`.
   - Each item: `role="option"`, `aria-selected={i === selectedIndex}`, `onMouseDown + e.preventDefault()`.
   - Keyboard nav: ArrowDown/ArrowUp/Enter/Escape (same as RepoSelector).
4. Show `SkeletonRows` (5 rows) while `isIssuesLoading`.
5. Empty states — implement all three variants:
   - `filteredIssues.length === 0 && issueStateFilter === "open" && !isIssuesLoading` → `"No open issues. [Show all]"` (button sets `issueStateFilter = "all"`).
   - `filteredIssues.length === 0 && (issueStateFilter === "closed" || issueStateFilter === "all") && !isIssuesLoading` → `"No issues found."` (no action button).
   - `labelFilter !== "" && filteredIssues.length === 0 && !isIssuesLoading` → `"No issues matching label '{labelFilter}'. [Clear]"` (button clears `labelFilter`). Shown instead of the state-based empty state when label filter is active.

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`

#### Task 3.2.3 — RepoChip, two-level Escape, auth error, top-level composition

**Steps**:
1. Implement `RepoChip`: `<span className={styles.repoChip}>{owner}/{repo} <button aria-label="Clear repo selection" onMouseDown={(e) => { e.preventDefault(); onClear(); }}>×</button></span>`.
2. Compose `GitHubIssuePicker` as the default export:
   ```tsx
   export function GitHubIssuePicker({ onImport, onClose }: { onImport: (url: string) => void; onClose: () => void }) {
     const picker = useGitHubIssuePicker();
     // phase switch, keyboard Escape logic, auth banner
   }
   ```
3. Two-level Escape on issue input `onKeyDown`:
   ```ts
   if (e.key === "Escape") {
     if (picker.phase === "issue-search" && (picker.issueSearch || picker.selectedRepo)) {
       e.stopPropagation(); // prevent modal-level Escape from firing
       picker.setPhase("repo-selection");
       picker.setSelectedRepo(null);
       picker.setIssueSearch(""); // clear search text so re-entering repo phase is clean
     } else {
       onClose();
     }
   }
   ```
4. Auth error banner: `if (picker.repoError?.isAuthError) { return <div data-testid="github-auth-banner" className={styles.authBanner}>GitHub not authenticated. Run <code>gh auth login</code> or set a <code>GITHUB_TOKEN</code> environment variable. <button onClick={picker.reloadRepos}>Try again</button></div>; }`.
5. On issue selection: call `onImport(issue.url)` directly (no confirm step).
6. Pre-populate last-used repo: in `useEffect([])` on mount, if `picker.lastUsedRepo` exists, `picker.setSelectedRepo(picker.lastUsedRepo)` and `picker.setPhase("issue-search")`.

**Files**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`

---

### Story 3.3 — Integration into Backlog Page

**Acceptance Criteria**:
- The raw `<input type="url">` block in `backlog/page.tsx` (`formMode === "github"` branch, lines ~573–637) is replaced by `<GitHubIssuePicker onImport={handlePickerImport} onClose={() => setShowForm(false)} />`.
- The `handlePickerImport` callback calls `importGitHubIssue(issueUrl)` (existing hook method), sets `githubImporting`, and on success closes the modal and refreshes the list.
- The Cancel button and import status/error display remain unchanged in the modal shell.
- The existing `githubIssueUrl` state and `handleImportGitHubIssue` form handler remain for backward compatibility until this change is validated.
- Given: User selects an issue from the picker and the picker calls `onImport("https://github.com/tstapler/stapler-squad/issues/42")`, When `handlePickerImport` is invoked, Then `importGitHubIssue("https://github.com/tstapler/stapler-squad/issues/42")` is called, the modal closes on success, and the backlog list refreshes.

**Files**: `web-app/src/app/backlog/page.tsx`

#### Task 3.3.1 — Replace URL input with GitHubIssuePicker in backlog/page.tsx

**Steps**:
1. Import `GitHubIssuePicker` from `@/components/backlog/GitHubIssuePicker`.
2. Add `handlePickerImport` callback:
   ```ts
   const handlePickerImport = useCallback(async (issueUrl: string) => {
     setGithubImporting(true);
     setGithubImportError(null);
     const result = await importGitHubIssue(issueUrl, { skipPlanning: false });
     setGithubImporting(false);
     if (result) { setShowForm(false); setGithubIssueUrl(""); await reloadItems(); }
     else { setGithubImportError("Import failed. Check the URL and try again."); }
   }, [importGitHubIssue, reloadItems]);
   ```
3. In the `formMode === "github"` branch, replace the `<form onSubmit={handleImportGitHubIssue}>` block with:
   ```tsx
   {githubImporting && <p style={{ ... }}>Importing…</p>}
   {githubImportError && <p style={{ color: "var(--error)" }}>{githubImportError}</p>}
   <GitHubIssuePicker onImport={handlePickerImport} onClose={() => { setShowForm(false); setGithubImportError(null); }} />
   ```
4. Remove the unused `<input type="url">` and its surrounding `<form>` wrapper from this branch.
5. Keep `githubIssueUrl` and `handleImportGitHubIssue` in place until the e2e test passes (they can be cleaned up in a follow-up).

**Files**: `web-app/src/app/backlog/page.tsx`

---

### Story 3.4 — Tests and Registry

**Acceptance Criteria**:
- RTL tests cover: onMouseDown prevents blur, two-level Escape, URL paste detection, auth error banner, keyboard nav ArrowDown/Enter.
- Playwright e2e covers: open picker, select first local repo, issue list appears, click first issue, modal closes.
- `docs/registry/backend-features.json` has entries for `backlog:search-github-repos` and `backlog:list-github-issues`.
- `docs/registry/frontend-features.json` has entry for `github-issue-picker` component.

**Files**:
- `web-app/src/components/backlog/__tests__/GitHubIssuePicker.test.tsx`
- `tests/e2e/github-issue-picker.spec.ts`
- `docs/registry/backend-features.json`
- `docs/registry/frontend-features.json`

#### Task 3.4.1 — RTL unit tests for GitHubIssuePicker

**Steps**:
1. Create `web-app/src/components/backlog/__tests__/GitHubIssuePicker.test.tsx`.
2. Mock `useGitHubIssuePicker` and `useBacklogService` via `jest.mock`.
3. Test `onMouseDown_prevents_blur_race`: render picker with 2 repo options; simulate focus on input, then `fireEvent.mouseDown` on first option; assert `onImport` is NOT called (repo selection, not import) and the option selection fires without the dropdown closing.
4. Test `two_level_escape_returns_to_repo_phase`: render in `"issue-search"` phase; press Escape; assert phase is now `"repo-selection"` and `onClose` was NOT called.
5. Test `escape_in_repo_phase_calls_onClose`: render in `"repo-selection"` phase with no selectedRepo; press Escape; assert `onClose` called once.
6. Test `url_paste_detection`: type `"https://github.com/tstapler/stapler-squad/issues/99"` into repo input; assert "Direct import" affordance renders.
7. Test `auth_error_shows_banner`: set `repoError = new GitHubAuthError(...)` in mock; assert `data-testid="github-auth-banner"` is rendered.
8. Test `arrow_down_moves_selection`: repo list has 3 items; press ArrowDown twice; assert `aria-activedescendant` is `"gh-repo-listbox-option-1"`.

**Files**: `web-app/src/components/backlog/__tests__/GitHubIssuePicker.test.tsx`

#### Task 3.4.2 — Playwright e2e

**Steps**:
1. Create `tests/e2e/github-issue-picker.spec.ts` with header `// @feature backlog:import-github-issue, github-issue-picker`.
2. `test.describe("github-issue-picker", () => { ... })`.
3. Test `shows_local_repos_immediately`: navigate to backlog, click "New Backlog Item", click "Import from GitHub Issue" tab; assert `data-testid="github-issue-picker"` renders; assert a repo option with `data-testid="repo-option"` is visible within 100ms.
4. Test `selects_repo_and_shows_issue_list`: click first repo option; assert `data-testid="issue-list"` appears; assert loading skeleton or issue rows render within 2000ms.
5. Test `keyboard_nav_select_issue`: arrow down to first issue, press Enter; assert modal closes (or import state appears).
6. All locators use `data-testid` or ARIA roles per e2e conventions.
7. Server URL: `http://localhost:8544` (e2e test server).

**Files**: `tests/e2e/github-issue-picker.spec.ts`

#### Task 3.4.3 — Feature registry updates

**Steps**:
1. Open `docs/registry/backend-features.json`; add two entries:
   - `{ "id": "backlog:search-github-repos", "type": "backend", "rpc": "BacklogService/SearchGitHubRepos", "markerFound": true, "tested": true, "testIds": ["TestSearchGitHubRepos_NilStorage", "TestSearchGitHubRepos_DefaultLimit"], "lastModified": "2026-07-03T00:00:00Z" }`
   - `{ "id": "backlog:list-github-issues", "type": "backend", "rpc": "BacklogService/ListGitHubIssues", "markerFound": true, "tested": true, "testIds": ["TestListGitHubIssues_NilStorage", "TestListGitHubIssues_EmptyOwner", "TestListGitHubIssues_EmptyRepo", "TestListGitHubIssues_InvalidOwnerChars", "TestListGitHubIssues_DefaultState"], "lastModified": "2026-07-03T00:00:00Z" }`
2. Open `docs/registry/frontend-features.json`; add entry:
   - `{ "id": "github-issue-picker", "type": "frontend", "component": "GitHubIssuePicker", "filePath": "web-app/src/components/backlog/GitHubIssuePicker.tsx", "tested": true, "testIds": ["github-issue-picker > shows_local_repos_immediately", "github-issue-picker > selects_repo_and_shows_issue_list"], "lastModified": "2026-07-03T00:00:00Z" }`

**Files**: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`

---

## Verification Checklist

Before the PR is considered complete, verify each item:

- [ ] `make quick-check` passes (build + test + lint)
- [ ] `make lint` passes — specifically the `norawexec` rule (no `exec.CommandContext` without safeexec; the new RPCs use `github.GHHTTPClient` directly, which is fine)
- [ ] `go test ./server/services -run TestSearchGitHubRepos -v` passes
- [ ] `go test ./server/services -run TestListGitHubIssues -v` passes
- [ ] `cd web-app && npx jest --no-coverage --testPathPatterns="GitHubIssuePicker"` passes
- [ ] `make install-service` builds and runs without error
- [ ] Manual smoke: open backlog page → "Import from GitHub Issue" → picker renders → select a repo → issues load → select issue → modal closes and new item appears in backlog
- [ ] `docs/registry/backend-features.json` updated with new entries
- [ ] `docs/registry/frontend-features.json` updated with new entry
- [ ] `coverage-gaps.json` does not grow (net zero new untested features)
