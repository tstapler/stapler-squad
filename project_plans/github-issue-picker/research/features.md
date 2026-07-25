# GitHub Issue Picker — Feature Research

_Date: 2026-07-03_

---

## 1. Similar Features in the Codebase

### 1.1 Existing GitHub Detectors (`web-app/src/lib/omnibar/detector.ts`)

Four detector classes already parse GitHub refs via regex:

| Class | Priority | Pattern |
|---|---|---|
| `GitHubPRDetector` | 10 | `https://github.com/owner/repo/pull/N` |
| `GitHubBranchDetector` | 20 | `https://github.com/owner/repo/tree/branch` |
| `GitHubRepoDetector` | 30 | `https://github.com/owner/repo` |
| `GitHubShorthandDetector` | 40 | `owner/repo` or `owner/repo:branch` |

All detectors return a `DetectionResult` with a `GitHubRef` payload (`owner`, `repo`, optional `prNumber`/`branch`). The registry runs detectors in priority order, first match wins. The same regex patterns and `GitHubRef` type should be reused for the issue picker to build normalized `owner/repo` strings for the new `ListGitHubIssues` RPC.

The frontend also has `web-app/src/lib/github/urlParser.ts` (imported by `RepoPathInput`) with `isGitHubRef`/`parseGitHubRef`/`getRepoFullName` helpers — directly reusable for parsing pasted URLs in the picker.

### 1.2 Current ImportGitHubIssue Handler (`server/services/backlog_service.go` ~L1684)

The existing handler:

1. Accepts a raw URL string (`issue_url`) and parses it via `parseIssueRef()` (regex for `https://github.com/owner/repo/issues/N` and `owner/repo#N` shorthand).
2. Calls `safeexec.CommandContext(ghCtx, "gh", "issue", "view", number, "--repo", owner+"/"+repo, "--json", "number,title,body,labels,url,state")` with a 30-second timeout.
3. Stores: title, body, labels (mapped to `notes`), issue URL as `ExternalID`, resolved repo path.
4. Optionally triggers auto-triage if `headlessPool != nil` and `repoPath != ""`.

The new `ListGitHubIssues` RPC should use the same `safeexec.CommandContext` pattern but call `gh issue list --repo owner/repo --json number,title,labels,state,assignees --limit 30`. The existing `ghIssueJSON` struct at L1672 already covers `number`, `title`, `body`, `labels`, and `url` — it needs `state` and `assignees` added for the picker's display.

For `SearchGitHubRepos`, the `gh` command would be `gh repo list [owner] --json nameWithOwner,description --limit 20 --source` (or `gh search repos query --json fullName,description`). No existing implementation; this is net-new.

### 1.3 Auth Detection — Existing `resolveAuthState` Pattern

`GitHubUserService.resolveAuthState()` (`server/services/github_user_service.go` L188) reads cached logins from `UserPRCache.GetCachedLogins()`. Returns `{ Available: false, ErrorMessage: "..." }` when no accounts are known. The new RPCs should call this same cache rather than running `gh auth status` inline — the cache is already populated by the background poller.

The frontend already has a `GitHubAuthBanner` component with Device Flow OAuth in `web-app/src/components/unfinished/GitHubPRsSection.tsx`. The same banner can be surfaced in the picker modal when `authState.Available === false`.

### 1.4 Existing Combobox/Autocomplete Patterns

**`RepoPathInput`** (`web-app/src/components/ui/RepoPathInput.tsx`) is the closest existing analog:
- Combines history paths from `useSessionRepoPaths()` (reads session paths from Redux store) with live `usePathCompletions()` results.
- Uses `PathCompletionDropdown` for the list, with keyboard nav (ArrowUp/Down, Enter, Escape).
- Splits items into "history" (from sessions) vs "live completions" (from FS), with a visual divider.
- ARIA attributes: `aria-autocomplete="list"`, `aria-controls`, `aria-activedescendant` on the input.

This is the direct architectural model for the `GitHubIssuePicker`. The two-phase repo source (sessions first, then search) mirrors the history-then-FS-completions split.

**`QuickOpenPalette`** (`web-app/src/components/sessions/QuickOpenPalette.tsx`) shows the pattern for a full-modal search:
- `requestIdRef` (generation counter) discards stale responses.
- Focus saved/restored on mount/unmount.
- `createPortal` for overlay rendering.
- Debounce: fires async search after `timerRef` delay.

**`usePathCompletions`** hook (`web-app/src/lib/hooks/usePathCompletions.ts`):
- Three-layer stale-prevention: 150ms debounce + `AbortController` + generation counter.
- Module-level LRU cache (100 entries, 30s TTL) with `Map` for LRU eviction.
- Cache key: `${pathPrefix}::${directoriesOnly}::${maxResults}`.

This exact pattern (debounce + abort + generation + LRU module cache) should be replicated for the issue picker's issue list cache.

**`useSessionRepoPaths`** (`web-app/src/lib/hooks/useSessionRepoPaths.ts`):
- Reads all sessions from Redux store via `selectAllSessions`, returns deduplicated `path` values.
- The new picker needs the equivalent but returning `{ owner, repo }` pairs extracted from `session.gitHubOwner` / `session.gitHubRepo` fields (already present in session state per `github_user_service.go` annotation logic).

---

## 2. Edge Cases and Failure Modes

### 2.1 GitHub Auth Not Configured

`resolveAuthState` returns `Available: false` when `GetCachedLogins()` is empty. The new `SearchGitHubRepos` / `ListGitHubIssues` RPCs should check this before running `gh` commands and return `connect.CodeUnauthenticated` with a message like `"GitHub authentication required — connect your account first"`. The frontend should detect this error code and render the Device Flow banner inline in the modal instead of an error message. Auth can change mid-session; the picker should re-check on each open.

### 2.2 Private Repo / Access Revoked

`gh repo list` and `gh issue list` will fail with a non-zero exit code. The handler pattern from `ImportGitHubIssue` is: check `cmd.Output()` error, wrap in `connect.CodeInternal`. The frontend should show a repo-level error below the repo selector (not a modal-level error) so the user can select a different repo without dismissing. The error message from `gh` stderr should be passed through (it includes meaningful text like "Could not resolve to a Repository with the name...").

### 2.3 Zero Results / Empty State

Two distinct zero states:
1. **Repo has no open issues** — show "No open issues in `owner/repo`" with a link to create one.
2. **Filter/search matches nothing** — show "No issues match your filter" with a "Clear filters" action.

Both should be non-error states — do not surface them as API errors.

### 2.4 Network Timeout

The existing `ImportGitHubIssue` uses a 30-second context timeout. For the picker (interactive, blocking UI), 10 seconds is more appropriate. On timeout the RPC returns an error; the frontend should show a retry button. The LRU cache means a successful result is served immediately on retry if the cache is still warm.

### 2.5 Race: Typing Faster Than Debounce

The `usePathCompletions` generation counter pattern directly solves this. Each fetch increments a ref; the response checks if its generation still matches the current ref before calling `setState`. The `AbortController` also cancels the in-flight HTTP/ConnectRPC request, not just ignores the result.

### 2.6 Stale Cache (Repo Archived/Deleted)

TTL-based cache (proposed: 5 minutes for issue lists, 15 minutes for repo lists) means stale data is possible. On mount of the picker modal, if the cache age exceeds 50% of TTL, trigger a background refresh so the result is fresh by the time the user types. Provide a manual refresh button (icon) that clears the cache entry and re-fetches. Distinguish between "archived" (issues still visible, no new ones) and "deleted" (fetch fails outright).

### 2.7 Issue Deleted After Cache

Cached issue was valid when listed but deleted before the user selects it. The `ImportGitHubIssue` RPC will fail with `gh issue view` returning an error. Return a clear error from the RPC; the frontend should show it inline (not dismiss the modal) so the user can select a different issue.

### 2.8 User Switches Repo Mid-Search

When the user selects a new repo, the in-flight issue list request for the old repo must be cancelled. Use the same `AbortController` pattern as `usePathCompletions`: a new repo selection triggers a new `AbortController`, the old controller is aborted, and the generation counter is incremented. The issue list clears immediately on repo change (no stale results from the old repo shown).

---

## 3. Unstated User Needs

### 3.1 What Makes This "Magical" vs Just Functional

The friction the feature spec identifies is "knowing the exact URL." The magical version eliminates that by making the repo step trivial — the user's current session repos are shown immediately (no network call), so a typical workflow is: open picker → see your most-recently-used repo pre-selected (or highlighted) → jump directly to the issue list → type 2-3 chars of the title → Enter. Zero context-switching to a browser.

**Pre-selection of last repo**: If the user imported an issue from `owner/repo` last time, pre-selecting that repo on next open reduces the flow to one step. Store last-used `owner/repo` in `localStorage` with a key like `github-issue-picker-last-repo`.

**Inline issue preview on hover/selection**: Showing the first 100 chars of the issue body in a tooltip or inline expansion removes the need to open the issue in a browser to confirm identity. The `gh issue list --json` already includes a `body` field that can be fetched and trimmed.

**Number prefix search**: Users often know "it was issue #847 something." Supporting a query like `#847` that filters by number prefix, with the title shown alongside, is a common omnibox pattern (VS Code file picker does this with `:`).

### 3.2 Issue Context That Helps Identification

In priority order, what helps users recognize the right issue:
1. **Issue number** (e.g. `#847`) — unambiguous reference; always show.
2. **Title** — first line of the issue title; truncate at ~60 chars.
3. **Labels** — colored chips (bug/feature/etc.) give at-a-glance categorization.
4. **State** (open/closed) — the picker defaults to open, but showing state in the closed-issues view avoids confusion.
5. **Assignee** — useful for repos with many contributors; lower priority for personal repos.

The `gh issue list` JSON fields that cover these: `number`, `title`, `labels`, `state`, `assignees`. The existing `ghIssueJSON` struct only has `number`, `title`, `body`, `labels`, `url`, `state` — needs `assignees` added.

### 3.3 Should the Picker Remember the Last Repo?

Yes, with a two-tier memory:

1. **Within session** (React state): Selected repo persists for the duration of the modal open session — if the user dismisses and reopens the modal, the repo should still be selected.
2. **Across sessions** (`localStorage`): Store `{ owner, repo, ts }` under `github-issue-picker-last-repo`. On modal open, if the stored repo matches one of the session repos (from `useSessionRepoPaths`), pre-select it. TTL of 7 days prevents stale pre-selection for repos the user no longer works on.

The `review-queue-auto-advance` pattern in `web-app/src/app/review-queue/page.tsx` (L41-44) shows the project's `localStorage` preference pattern to follow.

---

## 4. Backend Implementation Notes

### 4.1 `SearchGitHubRepos` RPC

The simplest correct implementation: `gh repo list [owner] --json nameWithOwner,description --source --limit 30`. Adding `--source` excludes forks. Without an `owner` argument, `gh repo list` returns the authenticated user's repos. A separate query like `gh search repos "query" --owner org --json fullName` enables cross-org search.

Alternative path (avoids `gh repo list` permission scope issues): call the GitHub REST API directly via the existing `http_client.go`. The `GET /user/repos` endpoint (no extra scope) returns personal repos; `GET /orgs/{org}/repos` requires `read:org`. The `gh` CLI path is simpler for MVP.

### 4.2 `ListGitHubIssues` RPC

`gh issue list --repo owner/repo --json number,title,labels,state,url,assignees --state open --limit 30`

The `--search` flag supports `gh issue list --search "query in:title"` — pass the user's search string here for server-side filtering. This avoids returning all 30 issues and filtering client-side when the repo has hundreds of issues.

### 4.3 Error Passthrough Pattern

The existing `ImportGitHubIssue` wraps `cmd.Output()` errors as `connect.CodeInternal`. For the picker RPCs, distinguish auth failures (return `CodeUnauthenticated` so the frontend can show the auth banner) from access/permission errors (return `CodePermissionDenied`) from transient failures (return `CodeInternal` with retry suggestion).

---

## 5. CSS Architecture

All new components must use vanilla-extract `.css.ts` files colocated with the component. The picker modal is a fixed overlay — must use `createPortal(..., document.body)` (see CSS rules). The two-column layout (repo list left, issue list right) should use `section[data-type="layout-two-equal"]` or a vanilla-extract `style` with `display: grid`. Import design tokens from `web-app/src/styles/theme.css.ts`.

---

## 6. Files to Create / Modify

| File | Change |
|---|---|
| `proto/session/v1/backlog.proto` | Add `SearchGitHubReposRequest/Response`, `ListGitHubIssuesRequest/Response` messages and RPCs |
| `server/services/backlog_service.go` | Add `SearchGitHubRepos` and `ListGitHubIssues` handlers |
| `web-app/src/lib/hooks/useBacklogService.ts` | Add `searchGitHubRepos` and `listGitHubIssues` methods |
| `web-app/src/components/backlog/GitHubIssuePicker.tsx` | New component (replaces URL text input) |
| `web-app/src/components/backlog/GitHubIssuePicker.css.ts` | Vanilla-extract styles |
| `web-app/src/app/backlog/page.tsx` | Wire in `GitHubIssuePicker`, remove plain URL input |
| `web-app/src/lib/hooks/useGitHubIssuePicker.ts` | Hook for cache management, debounce, abort |

---

## Summary

- **The `RepoPathInput` + `usePathCompletions` pair is the architectural template**: debounce + AbortController + generation counter + LRU module cache (100 entries, 30s TTL). Replicate this exactly for issue list fetching.
- **Auth is already solved**: `GitHubUserService.resolveAuthState()` (reads cached logins, no network call) is the correct auth check; the `GitHubPRsSection` Device Flow banner is the correct auth-failure UI to inline in the picker modal.
- **The tiered repo source** (session paths from Redux first, GitHub API search second) mirrors the existing history-then-FS pattern in `RepoPathInput` — use `useSessionRepoPaths()` as the "instant" tier and `SearchGitHubRepos` RPC as the "search" tier, with a visual separator between them.
