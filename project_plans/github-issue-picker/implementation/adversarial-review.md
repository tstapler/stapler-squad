# Adversarial Review: GitHub Issue Picker Implementation Plan

Date: 2026-07-03
Verdict: CONCERNS

The plan is structurally sound for the happy path but has three meaningful gaps: the test mocking approach for the Go backend is unimplementable as described (requires URL injection not specified), the Escape key acceptance criteria text directly contradicts the code sample in the same task (one of them will produce wrong behavior), and URL detection is scoped to only the repo input despite the UX research explicitly requiring EITHER input. None of these breaks the core feature, but at least two will require the implementer to discover and resolve the contradiction on their own.

---

## Finding 1 — onBlur-before-onClick Race

Verdict: CLEAN

Scenario: User in repo-selection phase clicks a repo option in the list while the search input is focused.
Expected: onMouseDown fires before onBlur, preventing the dropdown from closing before the click registers.
Actual: The plan correctly specifies `onMouseDown + e.preventDefault()` for RepoSelector items (Task 3.2.1 step 5), IssueList items (Task 3.2.2 step 3 "onMouseDown + e.preventDefault() on each row"), and the RepoChip × button (Task 3.2.3 step 1). The auth banner "Try again" button uses onClick, which is correct for a non-dropdown element where the blur race does not apply.
Fix: No fix required.

---

## Finding 2 — Two-level Escape Spec Contradiction (HIGH)

Verdict: CONCERN

Scenario: User is in `"issue-search"` phase, has selected a repo (selectedRepo is non-null), has typed nothing into the issue search (issueSearch is empty string), and presses Escape.
Expected: Escape returns to repo-selection — the user still has a selected repo context to go back to, so closing the modal entirely would be wrong.
Actual: AMBIGUOUS. The Story 3.2 acceptance criteria text says "First Escape in `'repo-selection'` phase (or when issueSearch is empty) calls the modal's onClose prop." This phrase "(or when issueSearch is empty)" is ambiguous but can be read as: whenever issueSearch is empty, regardless of phase, Escape closes the modal. An implementer following the AC text literally would write a condition like `issueSearch === ""` → call onClose(), which would fire even with a selectedRepo set.

The code sample in Task 3.2.3 step 3 correctly writes: `if (picker.phase === "issue-search" && (picker.issueSearch || picker.selectedRepo))` — this evaluates to true when selectedRepo is non-null even with empty issueSearch, going back to repo-selection. The code is correct. The AC text is wrong.

There is now a direct contradiction in the plan between the acceptance criteria for Story 3.2 and the code in Task 3.2.3. One of these will be the implementer's reference.

Fix: Rewrite the Story 3.2 AC bullet to: "First Escape in `'issue-search'` phase calls `onClose()` only when BOTH `issueSearch` is empty AND `selectedRepo` is null. When either is set, Escape goes back to repo-selection." Delete the ambiguous "(or when issueSearch is empty)" parenthetical.

---

## Finding 3 — AbortController Scope

Verdict: LOW (not the ReferenceError stated in the brief)

Scenario: Issue search useEffect fires, debounce timer is set, then the component unmounts before the 150ms fires.
Expected: Cleanup cancels the timer and the in-flight request.
Actual: The plan's step ordering (Task 2.3.2 step 2) creates the AbortController BEFORE `const debounceTimer = setTimeout(...)`. The controller is captured in the cleanup closure correctly. No ReferenceError occurs.

However, there is a different gap: the `listGitHubIssues` function signature exposed by `useBacklogService` is `(owner, repo, state, search?, limit?): Promise<GitHubIssue[]>` with no signal parameter. The plan never threads the AbortController's signal into the ConnectRPC call. Calling `controller.abort()` in cleanup will not cancel the in-flight network request — only the generation counter prevents stale state from being set. The network request continues to completion and consumes bandwidth and GitHub API quota even after the user has dismissed the picker.

Fix: Add a signal parameter to `listGitHubIssues` in `useBacklogService.ts` and thread it through to the ConnectRPC client call. ConnectRPC's generated TypeScript clients accept an AbortSignal via the request options object.

---

## Finding 4 — httptest.Server Mocking Approach is Unimplementable as Described (HIGH)

Verdict: CONCERN

Scenario: Implementer follows Tasks 1.2.2 and 1.3.2 and writes `httptest.NewServer(handler)` to mock GitHub API responses.
Expected: The handler under test sends HTTP requests to the test server, and the test server intercepts them.
Actual: `newGHRequest` (and its exported wrapper `NewGHRequest` as proposed) hardcodes `"https://api.github.com/"` as the base URL (confirmed in `github/http_client.go` line 86). The plan proposes `var GHHTTPClient = ghHTTPClient` as an exported variable, but even if the test replaces this with a custom `*http.Client`, the request URL still points to `api.github.com`, not the test server's `127.0.0.1:{port}`. The test server will never receive the request.

To intercept the request, the test would need a custom `http.RoundTripper` that rewrites the URL before making the connection. This requires non-trivial scaffolding the plan does not describe. Without it, the test either hits the real GitHub API (fails in CI with no token) or fails silently.

Note: `newGHRequestWithToken` (already in `github/http_client.go`) separates URL construction from token injection, but still hardcodes the base URL. A working test pattern would require either:
- A package-level `var ghBaseURL = "https://api.github.com/"` that tests override, OR
- A custom transport approach that rewrites the request URL at RoundTrip time, OR
- Mocking at the exported function level (mock `NewGHRequest` itself) rather than at the HTTP client level

Fix: Add a step to Task 1.2.1: "Add `var ghBaseURL = 'https://api.github.com/'` to `github/http_client.go` and update `newGHRequest` to use `ghBaseURL` as the base. In tests, set `github.GhBaseURL = testServer.URL + '/'` in a `TestMain` setup or per-test." Update Tasks 1.2.2 and 1.3.2 to specify `github.GhBaseURL = testServer.URL + "/"` in the test setup before calling the handler.

---

## Finding 5 — Rate Limit Handling Not Propagated to New Handlers

Verdict: LOW

Scenario: User types rapidly in the issue search box, firing requests against `/search/issues`. GitHub's search API rate limit is 30 requests/minute for authenticated users (distinct from the standard 5000/hr limit). Limit is exceeded.
Expected: The handler returns a rate-limit-specific error that the frontend can display helpfully.
Actual: `checkRateLimitHeaders` already exists in `github/http_client.go` (lines 43-77) and is called by `etag_cache.go` and `user_pr_cache.go` after every response. It logs warnings and returns a sleep duration for 429/403. The plan's new handlers (Tasks 1.2.1 and 1.3.1) do not mention calling `checkRateLimitHeaders`. A 429 response would fall through to the "other error → `CodeInternal`" path, returning a generic error with no guidance. Additionally, `state:"all"` in the `/search/issues` query string (generated when issueStateFilter is "all") is an unknown qualifier — GitHub ignores it and returns all states, which is the correct behavior. This is not a bug.

Fix (LOW, may defer): In the new handlers, after `ghHTTPClient.Do(req)`, call `checkRateLimitHeaders(resp)` and honor the returned backoff duration. On HTTP 429, return `connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("GitHub API rate limit exceeded; try again in %s", backoff))` so the frontend can display a meaningful message.

---

## Finding 6 — Proto Field Numbers

Verdict: CLEAN

Scenario: Implementer adds 6 new message types to `backlog.proto`.
Expected: No field number collisions.
Actual: All 6 new types (`GitHubRepoEntry`, `GitHubIssueEntry`, `SearchGitHubReposRequest`, `SearchGitHubReposResponse`, `ListGitHubIssuesRequest`, `ListGitHubIssuesResponse`) are entirely new top-level messages. Each starts field numbering from 1. Proto field collisions only occur when fields are added to an EXISTING message — not when new messages are appended. The concern in the brief does not apply. The two new service methods (`SearchGitHubRepos`, `ListGitHubIssues`) are appended after the existing `GetSyncHistory` RPC and require no field numbers.
Fix: No fix required.

---

## Finding 7 — URL Detection Missing from Issue Search Phase (MEDIUM)

Verdict: CONCERN

Scenario: User navigates to issue-search phase (repo already selected), copies a GitHub issue URL from their browser, and pastes it into the issue search input.
Expected: The URL is detected and a "Direct import" affordance appears, allowing immediate import without requiring the user to go back to the repo search input.
Actual: Task 3.2.1 implements URL detection only in the RepoSelector's `onChange` handler. Task 3.2.2 (IssueList + IssueFilterBar) describes no URL detection for the issue search input. The `issueSearch` string is passed as a literal search term to `/search/issues?q=...`. A full GitHub issue URL as a search query will return zero results from GitHub's API (not a valid text search). The user is stuck with no results and no affordance, and may not realize they need to go back to phase 1 to use the direct-import path.

This directly contradicts the UX research (section 5, "User Pastes a GitHub URL into the Search Input"): "If the user types/pastes the URL into either the repo search or issue search input, detect it immediately and import directly."

Fix: In Task 3.2.2, add to the IssueList sub-component's issue search `onChange` handler: "Test value against `issueURLPattern`. If match, set `detectedIssueUrl` state and render the same 'Direct import' affordance as in RepoSelector." The detection and render logic is already implemented for the repo input and can be extracted into a shared `useUrlDetection(value, onDetected)` helper.

---

## Summary

Verdict: CONCERNS

Most critical failure mode: The `httptest.NewServer` test approach in Tasks 1.2.2 and 1.3.2 is unimplementable without a URL injection mechanism — `newGHRequest` hardcodes `api.github.com` and no test scaffolding is specified to redirect requests to the test server.

Required fix: Add `var ghBaseURL = "https://api.github.com/"` to `github/http_client.go`, update `newGHRequest` to use it, and specify `github.GhBaseURL = testServer.URL + "/"` in the test setup steps.

Secondary issue: The Story 3.2 acceptance criteria text directly contradicts the Task 3.2.3 code sample for the Escape-in-issue-search-with-selectedRepo edge case. Fix the AC text before implementation begins.
