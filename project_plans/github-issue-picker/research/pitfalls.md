# GitHubIssuePicker — Pitfalls & Risks

Date: 2026-07-03
Feature: Replace raw URL input in GitHub issue import modal with smart picker
Scope: New `GitHubIssuePicker` React component + two backend RPCs (`SearchGitHubRepos`, `ListGitHubIssues`)

---

## 1. React Autocomplete / Combobox Pitfalls

### 1.1 Race Conditions on Rapid Typing

The existing `usePathCompletions` hook uses three layers to prevent stale results:
- 150ms debounce via `setTimeout`
- `AbortController` to cancel in-flight requests
- A generation counter (`generationRef`) to discard responses from superseded requests

The `GitHubIssuePicker` must replicate all three layers. A single `AbortController` without a generation counter is not sufficient: if the in-flight request resolves before being aborted (possible with cached or fast backend responses), a stale result can still land and overwrite a newer result.

**Pattern to follow:** `usePathCompletions` in `web-app/src/lib/hooks/usePathCompletions.ts` — the `generation !== generationRef.current` guard at the top of the `try` block is the critical piece.

### 1.2 Memory Leak: AbortController Not Cleaned Up

The `useBranchSuggestions` hook demonstrates the safe pattern: the `useEffect` cleanup function calls `controller.abort()`. The `GitHubIssuePicker` hook must return a cleanup function that aborts the controller and clears any pending debounce timer. Failing to clear the debounce timer is a latent leak — the setTimeout callback closes over React state setters and can fire after unmount.

### 1.3 Dropdown Closes Unexpectedly on Click (The `onBlur`-before-`onClick` Race)

`AutocompleteInput.tsx` does **not** use `onMouseDown + preventDefault` on its list items. This is a known bug pattern: when the user clicks a dropdown option, the browser fires `onBlur` on the input before `onClick` on the option, closing the dropdown and making the click handler unreachable.

The existing dropdowns that work correctly (`PathCompletionDropdown`, `SlashCommandDropdown`, `AtCommandDropdown`) all use `onMouseDown={(e) => { e.preventDefault(); onSelect(entry); }}` on list items. `preventDefault()` suppresses the blur event that would otherwise close the dropdown. The new picker must use this pattern.

### 1.4 Screen Reader / Accessibility Issues

The existing `AutocompleteInput` component is missing `role="combobox"` on the input and `aria-owns` / `aria-controls` pointing to the listbox. The `RepoPathInput` is more complete: it uses `aria-autocomplete="list"`, `aria-controls`, and `aria-activedescendant`. The new `GitHubIssuePicker` should follow the `RepoPathInput` pattern (not `AutocompleteInput`) to be WCAG-compliant. Key required attributes:
- Input: `role="combobox"`, `aria-expanded`, `aria-autocomplete="list"`, `aria-controls="{listboxId}"`, `aria-activedescendant="{optionId}"` when highlighted
- List: `role="listbox"` with a stable `id`
- Items: `role="option"`, `id="{listboxId}-option-{index}"`, `aria-selected`

Missing `aria-activedescendant` is the most common screen reader failure for comboboxes — focus stays on the input but screen readers announce nothing when arrow keys move the highlight.

### 1.5 Stale Closure Bugs in Debounced useCallback

If `useCallback` is used for the debounced fetch and the dependency array is incomplete, the callback will close over stale values of the search query or abort controller. The safest pattern (as demonstrated in `usePathCompletions`) is to put the entire fetch logic inside `useEffect` (not `useCallback`), using the query as a dependency. This avoids stale closure issues entirely because `useEffect` always runs with fresh values.

---

## 2. gh CLI Pitfalls

### 2.1 Unauthenticated State Not Detected Before Search

The existing `ImportGitHubIssue` handler in `backlog_service.go` (line 1724) runs `gh issue view` directly without first checking `gh auth status`. If `gh` is not authenticated, the CLI exits with code 1 and an error on stderr, but `cmd.Output()` only captures stdout. The error returned wraps the generic `exec.ExitError` without the human-readable stderr message ("You are not logged into any GitHub hosts").

**Fix for new RPCs:** Use `cmd.CombinedOutput()` instead of `cmd.Output()` so the stderr message is available, then check for exit code 1 and surface a specific `connect.CodeUnauthenticated` error (not `CodeInternal`). This lets the frontend distinguish "not authenticated" from generic backend failure.

The `GitHubUserService` handles authentication state via a background cache (`GetCachedLogins()`), but the new RPCs are in a different service and cannot assume the cache is available. They must handle the unauthenticated case independently.

### 2.2 `gh issue list` Silently Returns Empty on Private Repos

`gh issue list --json` returns an empty array `[]` (not an error) when the authenticated user does not have issue-read access to a private repo. This is indistinguishable from a repo with no issues. The backend must document this behavior and the frontend must show a neutral "No issues found" message rather than implying authentication failure.

### 2.3 `gh repo list` Is Slow (2–3 Seconds)

`gh repo list` performs a GraphQL call per page and can take 2–4 seconds. The 4-hour cache TTL proposed in the design is appropriate, but the **cold-start experience** (first load, no cache) will block the repo dropdown from being usable. Mitigations:
- Show a loading skeleton immediately while `gh repo list` runs in the background
- Load session/worktree-derived repos synchronously first (fast, from existing sessions RPC), then augment with GitHub repos once available
- Apply a 10-second timeout to `gh repo list` via `context.WithTimeout` to prevent the RPC from hanging indefinitely

### 2.4 Shell Injection Risk

`owner` and `repo` are user-provided or derived from `gh` output. Before passing them to subsequent `gh issue list --repo owner/repo` calls, they must be validated against a safe pattern. The existing `issueShorthandPattern` regex (`^([a-zA-Z0-9_-]+)/([a-zA-Z0-9_.-]+)#(\d+)$`) is a good model. For the new RPCs, owner and repo must be validated against a similar pattern (no spaces, no shell metacharacters) before being concatenated into the `--repo` flag.

`safeexec.CommandContext` passes args as a slice (not a shell string), so there is no shell expansion — but an attacker-controlled repo name like `"--format=..."` could still cause flag injection if not sanitized.

### 2.5 Context Cancellation / Request Abandonment

If the frontend user closes the picker before the `SearchGitHubRepos` RPC completes, the ConnectRPC request context will be cancelled. The `safeexec.CommandContext` properly propagates this cancellation to kill the `gh` subprocess. The `WaitDelay` of 2 seconds (see `executor/safeexec/safeexec.go`) ensures pipes are closed even if a grandchild process holds them open. No additional handling is needed here, but the RPC handler must not use `context.Background()` for the `gh` subprocess — it must use the request context (or a `WithTimeout` derived from it).

### 2.6 JSON Shape Variance

`gh issue list --json` field availability depends on the `gh` version. Fields like `assignees`, `milestone`, and `projectCards` may be absent on older versions. The struct decoder must use `json:",omitempty"` on optional fields and not fail if unknown fields are present. Use `json.Decoder` with `DisallowUnknownFields` set to `false` (the default) to be forward-compatible.

---

## 3. localStorage Caching Pitfalls

### 3.1 localStorage Is Synchronous and Can Block Render

For repos (payload potentially 10–50 KB), calling `JSON.parse(localStorage.getItem(...))` synchronously in a React effect or render path will block the main thread. The existing `TerminalDimensionCache` stores small payloads per-session; the repo cache will be much larger if `gh repo list` returns 50+ repos with metadata.

**Fix:** Wrap the `localStorage.getItem` call in a `try/catch` and call it inside a `useEffect` (never inside render). For the issue list cache (30 issues × ~500 bytes each = ~15 KB), this is fine. For repos, consider capping the stored payload to the top 50 results.

### 3.2 localStorage Quota Exceeded (5 MB)

If another feature stores large payloads and the repo cache pushes total localStorage past 5 MB, `setItem` throws a `QuotaExceededError` (a `DOMException`). The existing `TerminalDimensionCache` already handles this with a `try/catch` and `console.warn` (see `TerminalDimensionCache.ts` lines 87–90). The new cache must use the same pattern — never let a quota error propagate to the UI as an uncaught exception.

### 3.3 localStorage Unavailable in Private Browsing

In Firefox private mode, `localStorage.getItem` returns `null` silently. In hardened Safari, it throws a `SecurityError`. The pattern already used in `ErrorBoundary.tsx` (catching the `SecurityError`) must be applied here. The simplest approach is a `safeLocalStorage` helper that wraps get/set in a try/catch and falls back to an in-memory Map when storage is unavailable.

### 3.4 Stale Cache After Repo Deletion or Issue Close

A 4-hour TTL for repos means a deleted repo can appear in the picker for up to 4 hours. When the user selects a stale repo and fetches issues, the `ListGitHubIssues` RPC will succeed but return no issues (repo not found → empty list). The frontend must not interpret an empty issue list as "this repo has no issues" without adding a hint like "This repository may no longer exist or you may not have access." Consider reducing repo TTL to 30 minutes or providing an explicit "Refresh" button.

### 3.5 Cache Key Collision Between Stapler Squad Instances

The proposed cache keys (e.g., `github-repos-cache`) are not namespaced to the stapler-squad instance. If two instances run on the same machine (e.g., `localhost:8543` for prod and `localhost:8544` for e2e tests), they share `localStorage` because they share the same origin. Cache keys must include the instance base URL or port as part of the key, e.g., `github-repos-{baseUrl}`.

---

## 4. Performance Pitfalls

### 4.1 gh Repo List Blocks UI Cold Start

As noted in §2.3, cold-start repo fetch can take 2–4 seconds. The repo picker must render immediately with the session/worktree-sourced repos (which come from an existing RPC and are fast) and show a loading indicator while GitHub repos load. A two-phase render (local repos first, then augmented with remote repos) avoids the perception of a blocked UI.

### 4.2 Issue List Fetch Latency

`gh issue list --json` for 30 issues takes approximately 0.5–1.5 seconds on typical connections. The picker must show a skeleton/spinner for the issue list panel while fetching. Avoid the pattern of setting `isLoading: false` before results arrive — the existing `usePathCompletions` sets `isLoading: true` immediately before the debounce fires and only clears it in the `try` block after a successful response.

### 4.3 Rapid Keystroke Re-renders

Typing in the issue search box will update query state on every keystroke. Without debouncing, each keystroke triggers a new RPC. The 150ms debounce used in `usePathCompletions` is appropriate here. However, client-side filtering (label filter, open/closed filter) should be applied synchronously to the already-fetched 30-issue list without triggering a new RPC — these are UI state updates only.

---

## 5. UX Pitfalls

### 5.1 Empty State When GitHub Not Authenticated

The existing import modal shows a generic error when `gh issue view` fails due to auth. The new picker has two distinct empty states that must be designed explicitly:
1. **Not authenticated:** Show a message like "GitHub CLI not authenticated. Run `gh auth login` in your terminal." with no search functionality.
2. **No repos found:** Show "No repositories found. You can still type a GitHub URL directly."

The `GitHubUserService.resolveAuthState()` pattern (returning `Available: false` with an error message) should be reused for the `SearchGitHubRepos` RPC to signal the unauthenticated case distinctly from the empty-results case.

### 5.2 URL Input Fallback

Users who already know the issue URL (and have been using the old modal) will try to paste a URL into the new combobox. If the combobox only accepts picker selections, this breaks the existing workflow. The design should either:
- Accept typed URLs and resolve them server-side (backward compatible), or
- Include a "Paste URL directly" escape hatch below the combobox

This is a workflow continuity risk — removing URL input without a fallback will frustrate power users.

### 5.3 Confusing Empty Issue List

If a repo has no open issues, the issue list panel will be empty. This is indistinguishable (to the user) from a fetch failure or a loading state. The picker must explicitly distinguish:
- Still loading (spinner)
- No issues found (empty state with "No open issues" message)
- Fetch failed (error message with retry option)

---

## 6. Existing Patterns to Follow

### Backend: gh CLI Error Handling

From `backlog_service.go` (ImportGitHubIssue):
- Use `safeexec.CommandContext` (never `exec.CommandContext` directly — enforced by the `norawexec` lint rule)
- Use `context.WithTimeout(ctx, 30*time.Second)` for each `gh` call
- Use `cmd.Output()` for stdout; for new RPCs, consider `cmd.CombinedOutput()` to capture stderr for better error messages
- Return `connect.NewError(connect.CodeInternal, ...)` for unexpected CLI failures
- Return `connect.NewError(connect.CodeUnavailable, ...)` for `gh` auth failures (not `CodeInternal`)

**Gap in existing code:** `ImportGitHubIssue` uses `CodeInternal` for all `gh` failures, including auth failures. The new RPCs should use `CodeUnauthenticated` when the exit code or stderr indicates "not logged in."

### Frontend: ConnectRPC Error Handling

From `web-app/src/lib/config.ts`:
- `Code.Unauthenticated` (HTTP 401) triggers automatic redirect to login — do not use this code for GitHub auth failures unless you intend that behavior
- ConnectError instances expose `.code` and `.message` for display
- The `catch` clause in hooks should distinguish `controller.signal.aborted` (ignore) from real errors (surface to UI)

### Frontend: localStorage Safety

From `TerminalDimensionCache.ts` (the established pattern):
```ts
try {
  localStorage.setItem(key, JSON.stringify(payload));
} catch (err) {
  console.warn('[Cache] Failed to save:', err);
}
```
Both get and set must be wrapped. The check `if (typeof window === 'undefined') return null;` is required for SSR compatibility (even though this app is client-rendered, Next.js may prerender pages).

---

## Summary: Top 3 Critical Risks

1. **The `onBlur`-before-`onClick` race causes dropdown selections to be dropped.** `AutocompleteInput` has this bug; it must not be used as a template. The new picker must use `onMouseDown + e.preventDefault()` on option items, as `PathCompletionDropdown`, `SlashCommandDropdown`, and `AtCommandDropdown` all do.

2. **`gh issue list` silently returns empty on private repos without access, and `gh` auth failures return `CodeInternal` instead of a user-actionable code.** The backend must check stderr from `CombinedOutput()` and return a specific `CodeUnavailable` or `CodePermissionDenied` error so the frontend can display a meaningful "not authenticated" or "no access" state instead of a generic error.

3. **localStorage cache keys are not instance-scoped, and no `QuotaExceededError` handling is planned.** Keys must include the base URL to prevent collision between the production and e2e-test instances, and all get/set calls must be wrapped in try/catch — the `TerminalDimensionCache` pattern is the established template.
