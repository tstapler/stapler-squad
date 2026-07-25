Date: 2026-07-03
Verdict: CONCERNS

The overall architecture fits cleanly into the existing codebase. ConnectRPC handler placement in `BacklogService`, vanilla-extract CSS, Redux-sourced local repos, and localStorage TTL caching are all sound decisions that align with established patterns. Two structural issues need resolution before implementation starts: how the `github/` package is exported to `backlog_service.go`, and which auth-detection path to use.

---

## Issue 1 — github/ package export design

Risk: medium
Detail: The plan proposes adding exported wrappers `NewGHRequest` and `GHHTTPClient` (or a variable export of the HTTP client) to `github/client.go` so `backlog_service.go` can call them directly. This breaks the package's established encapsulation pattern. Every existing exported symbol in `github/client.go` is a domain function (`CheckGHAuth`, `GetPRForBranch`, `GetCurrentUserLogin`, `GetCurrentUserLoginWithToken`). The unexported primitives `newGHRequest` and `ghHTTPClient` are intentionally internal — exporting them leaks a transport-layer detail into service code, creates a coupling point that prevents future changes to the HTTP layer (retries, tracing, pooling), and creates an inconsistency where some callers get full domain encapsulation and others get raw HTTP primitives.
Recommendation: Add two domain functions to a new `github/repos.go` file: `SearchUserRepos(ctx context.Context, query string, limit int) ([]RepoResult, error)` and `ListRepoIssues(ctx context.Context, owner, repo, state, search string, limit int) ([]IssueResult, error)`. These functions own the HTTP call, JSON parsing, and rate-limit header checking internally. `BacklogService` calls the domain functions; the HTTP primitives stay unexported. This mirrors how `GetPRForBranch` wraps `newGHRequest` + `ghHTTPClient.Do` internally and exposes only the structured result.

---

## Issue 2 — Auth detection pattern

Risk: low
Detail: The plan guards both handlers with `if github.GetGHToken(ctx) == ""` (requiring a new exported wrapper around the unexported `getGHToken`). This checks only whether a token string is present, not whether it is valid. A configured-but-expired or revoked token passes the guard, the API call returns HTTP 401, and the handler maps that to `CodeUnavailable` inline. The behavior is correct, but the existing pattern is split: `GetPRInfoCtx` calls `CheckGHAuth()` upfront (which validates via `GET /user` with a 5-minute cache and singleflight deduplication), while `GetPRForBranch` skips `CheckGHAuth()` and handles 401 inline. Both work, but they diverge on which path is authoritative for auth failures. If the domain functions in `github/repos.go` are implemented as recommended in Issue 1, auth detection belongs inside those functions. The domain function can call `CheckGHAuth()` or handle 401 inline — both are acceptable — and the `BacklogService` handler maps domain errors to connect error codes without needing to know token internals.
Recommendation: Implement auth detection inside the domain functions in `github/repos.go`. Map an auth failure (missing token OR API 401/403) to a typed error (e.g., `github.ErrNotAuthenticated`) that the handler translates to `connect.CodeUnavailable`. Do not export `GetGHToken` or introduce a new exported token-check function; keep token access internal to the `github/` package.

---

## Issue 3 — Research doc vs. plan implementation approach

Risk: low
Detail: `research/architecture.md` sections 1.4 and 2.1 document `safeexec.CommandContext` with `gh issue list` / `gh repo list` as the implementation approach. The plan overrides this with the native HTTP client (matching the `requirements.md` constraint). The research is now stale and contradicts the plan. This is not an architectural problem but will cause confusion during implementation if a developer follows the research doc.
Recommendation: Note in the implementation kick-off that the research doc's CLI subprocess sections (1.4, 2.1) are superseded by the plan. No code change needed.

---

## Issue 4 — Search API rate limits

Risk: low
Detail: `/search/issues?q=...+in:title` is subject to the GitHub Search API secondary rate limit of 30 requests per minute for authenticated users (distinct from the 5000/hr core API limit). With 150ms debounce and a 5-minute localStorage cache TTL per `owner/repo/state` combination, a single user would need to trigger approximately 30 unique uncached searches within 60 seconds to hit this limit. That is implausible for a personal tool.
Recommendation: No change required. Document the rate limit distinction in a code comment on the search path so future developers understand why 30/min applies here versus the 5000/hr they might expect.

---

## Items confirmed clean

**API endpoint split**: `/user/repos` for the unfiltered list and `/search/repositories` for queries is the correct GitHub REST API split. For a single-user personal tool, `/user/repos` covers all repos the authenticated user owns or has access to. No `GET /orgs/{org}/repos` supplementation is needed.

**localStorage cache key scoping**: `window.location.origin` returns the full origin including port (`http://localhost:8543` vs `http://localhost:8544`), so the `ssq:{origin}:gh-repos:v1` key correctly isolates prod and e2e caches without collision.

**Redux local repos source**: `Session.github_owner` (field 27) and `Session.github_repo` (field 28) are confirmed present in `proto/session/v1/types.proto`. Reading from Redux `sessionsSlice` client-side for the local tier avoids a round-trip and renders in one cycle. The design decision to drop the `local_only` flag (from the research doc) in favor of pure client-side Redux reads is sound.

**Proto placement**: New messages and RPCs appended to `backlog.proto` after `CancelTriageResponse` with fresh field-number sequences (each new message starts at 1) will not collide with any existing field numbers.

**Component architecture**: The two-phase combobox (repo selector → issue list), `onMouseDown + e.preventDefault()` for blur-race prevention, generation counter + AbortController for stale-response cancellation, and vanilla-extract `.css.ts` colocated styles all fit the existing codebase patterns cleanly.
