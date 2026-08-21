# ADR-002: GitHub Repo and Issue Lists Cached in Frontend localStorage

**Date**: 2026-07-03
**Status**: Accepted
**Feature**: github-issue-picker

---

## Context

The `GitHubIssuePicker` makes two expensive `gh` CLI calls:
- `gh repo list` (~1–3 seconds, GraphQL paged call)
- `gh issue list --repo owner/repo` (~0.5–1.5 seconds per call)

Both calls should be cached to meet the SLO of "second open of the picker shows cached results without a network call" and avoid unnecessary `gh` CLI subprocess overhead.

Two caching locations were evaluated:

**Option A: Backend `sync.Map` cache in `BacklogService`**
A `sync.Map[string, cachedRepoList]` in `BacklogService` holds the last repo list in memory. Issue lists keyed by `owner/repo` are stored similarly.

**Option B: Frontend `localStorage` with TTL (chosen)**
A utility module `issuePickerCache.ts` wraps `localStorage.getItem`/`setItem` with TTL checks and `try/catch`. Cache keys include `window.location.origin` to prevent collision between the `localhost:8543` production instance and the `localhost:8544` e2e test instance.

---

## Decision

Use **Option B**: cache in frontend `localStorage` with a TTL.

Cache TTLs:
- Repo list: 4 hours (`REPOS_TTL_MS = 14_400_000`)
- Issue list per `(owner/repo, state)` combo: 5 minutes (`ISSUES_TTL_MS = 300_000`)

Cache key format:
- `"ssq:{window.location.origin}:gh-repos:v1"` → `{ data: GitHubRepo[], fetchedAt: number }`
- `"ssq:{window.location.origin}:gh-issues:{owner}/{repo}:{state}"` → `{ data: GitHubIssue[], fetchedAt: number }`
- `"ssq:{window.location.origin}:gh-last-repo"` → `{ owner: string, repo: string }` (no TTL — persists until cleared)

---

## Rationale

1. **Single-user tool**: There are no other users whose actions could invalidate a shared backend cache. The cache is semantically equivalent between frontend and backend for this use case.

2. **Survives re-renders without re-fetching**: A backend `sync.Map` is cleared on server restart. `localStorage` survives page refreshes and modal open/close cycles. This is the correct granularity for the picker's UX goal.

3. **Established codebase pattern**: `usePathHistory.ts`, `notificationStorage.ts`, `ThemeContext.tsx`, and `TerminalDimensionCache.ts` all use `localStorage` with try/catch. The `issuePickerCache.ts` module follows the exact same pattern and requires no new engineering primitives.

4. **Avoids backend coupling**: The `BacklogService` already manages multiple `sync.Map` caches (branch cache in `session_service.go` line 117; worktree detection in `repo_path.go` line 25). Adding repo/issue caches there would grow the server's memory footprint with UI-specific data that is meaningless to other callers.

5. **Origin-namespaced keys prevent e2e test pollution**: The e2e test suite runs on `localhost:8544`. Without origin namespacing, a cached repo list from a prior `localhost:8543` session could appear in e2e tests, creating flaky assertions. The `window.location.origin` prefix is the minimal fix.

---

## Consequences

- **localStorage quota**: GitHub repos (~50 entries × ~200 bytes each = ~10 KB) and issue lists (~30 entries × ~300 bytes each = ~9 KB) are well within the 5 MB localStorage limit. All writes are wrapped in `try/catch` to silently handle `QuotaExceededError`.
- **Private browsing**: Firefox private mode returns `null` from `localStorage.getItem` silently; Safari throws a `SecurityError`. The `try/catch` in `issuePickerCache.ts` handles both — a cache miss on every call, degrading gracefully to always-fresh data.
- **Cache staleness**: A deleted GitHub repo can appear in the picker for up to 4 hours. Selecting a stale repo shows an empty issue list (or a `gh` error), which the UI handles with the "No issues found" empty state. This is acceptable for a personal dev tool.
- **SSR compatibility**: `window.location.origin` is guarded with `typeof window !== "undefined"` to prevent Next.js prerender failures.

---

## Rejected Alternative: Backend `sync.Map` Cache

**Key weakness**: Backend in-memory caches are reset on server restart. The `make install-service` command (used daily) restarts the server, clearing all caches on every deployment. A frontend `localStorage` cache survives this. Additionally, backend caching couples the UI's concept of "staleness" to the server process lifecycle, which is harder to reason about.
