# Pre-Mortem: GitHub Issue Picker

Date: 2026-07-03

---

## Scenario 1

Priority: P1
Trigger: User selects repo-A. While the issue RPC is in-flight (the 150ms debounce has fired and the request is awaiting a response), the user clicks repo-B. The developer placed `const gen = generationRef.current` INSIDE the `setTimeout` callback rather than at the top of the `useEffect` body. During the 150ms wait for repo-A's debounce, React runs repo-B's `useEffect`, which increments `generationRef.current` from 1 to 2. When repo-A's setTimeout fires, it captures `gen = generationRef.current` (= 2). Repo-A's request resolves. The guard `if (gen !== generationRef.current)` evaluates as `2 !== 2 = false` — the stale result passes.
Symptom: The issue list displays repo-A's issues while the RepoChip shows "repo-B". The user reads issue titles from the wrong repository and may import an incorrect issue into the backlog with no indication anything is wrong.
Root cause: Task 2.3.2, step 2 states "guard with `if (gen !== generationRef.current) return`" but does not show WHERE `gen` is captured. The phrase "Inside timeout: ... guard with `if (gen !== generationRef.current) return`" causes an implementor to capture `gen` inside the setTimeout, destroying the guard's correctness. The generation counter only functions as a stale-response guard when `gen` is captured at the `useEffect` scope (before the `setTimeout` call) so it reflects the generation at the moment the effect ran, not the generation at the moment the timeout fires.
Mitigation: Amend Task 2.3.2 to show an explicit code skeleton with the capture point marked: `const gen = ++generationRef.current;` as the first line of the `useEffect` body, before `const timer = setTimeout(...)`. The timeout callback receives `gen` via closure, not by reading `generationRef.current` again at callback entry. Add this as an explicit acceptance criterion ("gen is captured at useEffect scope, not inside the setTimeout callback").

---

## Scenario 2

Priority: P1
Trigger: Developer calls `reposCacheKey()` (or `issuesCacheKey()`) in the body of `useGitHubIssuePicker.ts` outside of a `useEffect` — for example, as the initializer for a `useMemo` or `useState` default value. During `next build`, Next.js server-pre-renders the backlog page. On the server `window` is undefined. Even if the developer added the guard `if (typeof window === 'undefined') return ''`, the subsequent `localStorage.getItem(...)` call (also server-executed, outside a `useEffect`) throws `ReferenceError: localStorage is not defined` because `localStorage` is also a browser global.
Symptom: `make install-service` fails at the web UI build step with `ReferenceError: localStorage is not defined` (or `window is not defined`). The production binary cannot be rebuilt. Every `make install-service` attempt fails until the bug is reverted. The rollback path (reverting `page.tsx`) does not help because the bad code lives in the hook, not the page.
Root cause: Task 2.1.1, step 7 says "Guard all `window.location.origin` accesses with `typeof window !== 'undefined'`" but does not specify that `localStorage` requires the same guard, and does not specify what the helper functions should return when the guard fires (returning `''` causes `localStorage.getItem('')` to execute next, which also throws). The plan relies on callers to only use these functions inside `useEffect`, but does not enforce or document this restriction in the hook or utility file.
Mitigation: The `readCache` and `writeCache` functions must each begin with `if (typeof window === 'undefined') return null / return;` as their first line, before any key construction or `localStorage` access. The key helper functions (`reposCacheKey`, `issuesCacheKey`, `lastRepoCacheKey`) must return `null` (not `''`) when `window` is undefined, and both callers (`readCache`, `writeCache`) must short-circuit on a `null` key. Add a comment at the top of `issuePickerCache.ts`: "All exports are safe to import on the server; functions are no-ops when window is undefined." Add a Jest test that mocks `window` as `undefined` and asserts `readCache` returns `null` without throwing.

---

## Scenario 3

Priority: P2
Trigger: User's browser localStorage is at ~4.95 MB (from other Stapler Squad cache entries). After the picker loads, `writeCache(reposCacheKey(), repos)` is called with 60 repos (approximately 30 KB serialized). The browser throws `QuotaExceededError`. The `writeCache` try/catch catches it and logs a warning — correct behavior. However, on the NEXT page load `readCache` calls `JSON.parse(localStorage.getItem(reposCacheKey()))`. The prior failed `setItem` left the key absent (browsers atomically reject the write on quota error; they do not partially write). `getItem` returns `null`. `JSON.parse(null)` returns `null` in V8 — this is fine. BUT: if the pre-existing entry at that key was written by an older version of the cache schema (e.g., no `fetchedAt` field), `JSON.parse` succeeds but the TTL check `Date.now() - entry.fetchedAt > ttlMs` evaluates as `Date.now() - undefined > ttlMs` which is `NaN > number = false` — the expired entry is treated as valid and served. Or if the developer omits the try/catch on `JSON.parse` itself and the key contains malformed JSON (written by another app sharing the same origin), a `SyntaxError` is thrown from inside the hook, propagates past the hook, and crashes the component with no error boundary.
Symptom: Either (a) stale repo data from an old cache schema is served indefinitely (user sees deleted repos), or (b) a `SyntaxError` propagates to React and the picker renders as a blank area or escalates to the nearest error boundary.
Root cause: Task 2.1.1, step 2 states "implement `readCache` with try/catch and TTL check" but the pseudocode only shows TTL validation. The `JSON.parse` call and the TTL field access (`entry.fetchedAt`) are each independent throw surfaces that the acceptance criteria do not explicitly require to be inside the try/catch. Separately, the cache schema (`CacheEntry<T>`) has no version field; an entry written by an old code version will silently pass the `fetchedAt` check if the field is missing (NaN comparison).
Mitigation: Wrap the entire body of `readCache` — including `getItem`, `JSON.parse`, and `entry.fetchedAt` access — in a single try/catch that returns `null` on any error. Add a `schemaVersion: 1` field to `CacheEntry<T>`; reject entries where `entry.schemaVersion !== CURRENT_SCHEMA_VERSION`. Write a unit test for `readCache` that puts malformed JSON into a mocked localStorage and asserts `null` is returned.

---

## Scenario 4

Priority: P2
Trigger: Developer places `const controller = new AbortController()` INSIDE the `setTimeout` callback (a misread of plan step ordering, where "Create AbortController" appears adjacent to "const debounceTimer = setTimeout"). The user types rapidly in the issue search box. Each keystroke after 150ms fires the debounce and starts an RPC. The `useEffect` cleanup function calls `controller.abort()` on the outer-scope variable — but the outer `controller` was never passed to any fetch call. The actual controller (created inside setTimeout) is never aborted. After sustained typing, 30+ calls to GitHub's `/search/issues` endpoint accumulate within 60 seconds. GitHub returns HTTP 403 with body `{"message":"You have exceeded a secondary rate limit..."}`. The Go handler in `ListGitHubIssues` (Task 1.3.1) has no handling for this response: it checks for 401 (→ `CodeUnavailable`) and maps all other non-200 responses to `CodeInternal`. The frontend receives `CodeInternal`, maps it to the generic `issueError` state, and renders "Failed to load issues." with a "Retry" button that immediately hits the rate limit again.
Symptom: After roughly 30 seconds of active typing in the issue search box, the issue list is replaced by a generic error. Pressing "Retry" makes it worse. The rate limit window is 60 seconds; no countdown or wait indication is shown. The user cannot recover without waiting silently.
Root cause (A — AbortController): Task 2.3.2, step 2 lists "Create `AbortController`" as a bullet in the same indented block as "const debounceTimer = setTimeout(..." without a code skeleton showing nesting. An implementor places the controller inside the setTimeout, making cleanup call abort on a variable that controls nothing. This allows requests to pile up.
Root cause (B — Rate limit): Stories 1.2.1 and 1.3.1 specify handling for 401 (→ `CodeUnavailable`) but not for 403 with a rate-limit body. The GitHub secondary rate limit (30 search requests/minute) is not mentioned in the backend stories and has no corresponding error code or frontend message.
Mitigation: Amend Task 2.3.2 pseudocode to show `const controller = new AbortController()` explicitly placed BEFORE the `setTimeout(...)` call in the `useEffect` body. Add an acceptance criterion to Story 1.3.1: "When the GitHub API returns 403 with `X-RateLimit-Remaining: 0` or a body containing 'rate limit', return `connect.CodeResourceExhausted`." Map `CodeResourceExhausted` in `useBacklogService.ts` to a typed `GitHubRateLimitError` and display "Search rate limited — wait 60 seconds before searching again."

---

## Scenario 5

Priority: P2
Trigger: User is in "issue-search" phase with 8 issues visible. User presses ArrowDown twice; `selectedIndex` is now 1. User presses Escape, expecting to clear the keyboard highlight (standard combobox convention: first Escape clears selection, second Escape dismisses).
Symptom: Instead of resetting `selectedIndex` to -1, the Escape handler in Task 3.2.3, step 3 evaluates `picker.phase === "issue-search" && (picker.issueSearch || picker.selectedRepo)`. Because `selectedRepo` is always non-null in issue-search phase, this condition is always truthy. The handler immediately calls `picker.setPhase("repo-selection")` and `picker.setSelectedRepo(null)`, clearing the repo chip and replacing the issue list with the repo selector. The user must re-select the repo, wait for cache validation, navigate back to the issue list, and re-navigate to the desired item. If the user arrived via the "last used repo" pre-population on mount, phase reversion does not re-trigger that pre-population (it only runs on mount in a `useEffect([], [])`) — the user must re-select manually.
Root cause: Task 3.2.3, step 3 defines Escape in terms of phase and search text but omits `selectedIndex` from the condition. The plan's acceptance criterion for Escape behavior (Story 3.2) says "First Escape in `issue-search` phase: if `picker.issueSearch !== '' || picker.selectedRepo !== null`, sets `phase = 'repo-selection'`..." — this is unambiguous but incorrect for the keyboard navigation case. The condition `selectedRepo !== null` is always true in issue-search phase, so the two-step Escape (clear highlight → navigate back) is never possible under the specified logic.
Mitigation: Add `selectedIndex > -1` as the highest-priority condition in the Escape handler: if `selectedIndex > -1`, reset `selectedIndex` to -1 and `e.stopPropagation()` and return early. Only when `selectedIndex === -1` proceed to evaluate the phase-transition condition. Update the acceptance criterion in Story 3.2 to explicitly describe this three-state Escape sequence: (1) Escape with `selectedIndex > -1` → clear highlight only; (2) Escape with `selectedIndex === -1` and any of `issueSearch`, `selectedRepo` → go to repo-selection; (3) Escape in repo-selection → `onClose()`.

---

## Summary

P1 count: 2  
P2 count: 3

Overall risk: MEDIUM-HIGH. Two build-breaking or data-corrupting failures are possible from plausible implementation misreads of the plan's pseudocode.

Most critical P1: Scenario 1 (stale issue results after rapid repo switching). The generationRef pattern is the sole defence against stale RPC responses landing in the UI, and the plan's task description leaves the capture point ambiguous enough that a developer will likely place it inside the setTimeout callback. This bug causes silent data corruption — the user imports the wrong issue with no error indicator.

Recommended mitigation: Add an explicit, commented code skeleton to Task 2.3.2 showing `const gen = ++generationRef.current;` as the first statement of the `useEffect` body (before `setTimeout`), with `gen` referenced by closure inside the callback. This one addition closes the most dangerous race and makes the intent unambiguous.
