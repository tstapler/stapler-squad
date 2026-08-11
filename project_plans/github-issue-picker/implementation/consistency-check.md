# Cross-Artifact Consistency Check: GitHub Issue Picker

Date: 2026-07-03
Result: INCONSISTENCIES FOUND

---

## Critical Inconsistencies

### C-1: Debounce timing — requirements.md ↔ plan.md

Severity: critical

Artifact A says (requirements.md Non-functional Requirements):
> "GitHub search debounced at 300ms"

Artifact B says (plan.md Open Questions resolution + Story 2.3 AC):
> "Issue fetch is debounced at 150ms" (Ubiquitous Language, Story 2.3 AC, Task 2.3.2 Step 2 comment `setTimeout(..., 150)`)

Resolution: The plan explicitly resolved this in research/pitfalls.md §4.3 (not yet reviewed here). The plan's 150ms is intentional. requirements.md must be updated to reflect 150ms. The plan is correct; the requirements are stale on this point.

---

### C-2: Story 1.3 AC uses exported HTTP symbols that are declared unexported — plan.md internal

Severity: critical

Artifact A says (plan.md Task 1.2.1 Step 3 — architecture decision):
> "Keep HTTP internals unexported. Architecture review finding: do NOT export `newGHRequest` or `ghHTTPClient`."
> `var ghBaseURL = "https://api.github.com/"` (lowercase, unexported)

Artifact B says (plan.md Story 1.3 Acceptance Criteria):
> "the handler calls `GET https://api.github.com/repos/{owner}/{repo}/issues` via `github.NewGHRequest` + `github.GHHTTPClient.Do`"

And (plan.md Task 1.2.2 Step 4 — test pattern):
> "Tests override it: `github.GhBaseURL = ts.URL + "/"` then defer..."

The Story 1.3 AC references `github.NewGHRequest` and `github.GHHTTPClient` (exported, capital letters), directly contradicting the architecture decision in Task 1.2.1 to keep these unexported. Additionally, tests (Task 1.2.2 Step 4) reference `github.GhBaseURL` (exported) while Task 1.2.1 defines `var ghBaseURL` (unexported) — the test would fail to compile.

Resolution: Story 1.3 AC must use the domain function call (`github.ListRepoIssues(...)`, not raw HTTP symbols), matching how Story 1.2 AC describes `SearchGitHubRepos` calling `github.SearchUserRepos(...)`. The `ghBaseURL` variable must be exported as `GhBaseURL` if tests need to set it, or the test overrides must use an internal test hook. Fix both: update Story 1.3 AC to match the domain-function pattern; export `GhBaseURL`.

---

### C-3: Escape handler missing `setIssueSearch("")` — plan.md ↔ design/ux.md

Severity: critical

Artifact A says (design/ux.md Keyboard Map):
> Escape in issue-search when `issueSearch` set or `selectedRepo` set: "`e.stopPropagation()`; **clear `issueSearch`**; set phase = repo-selection"

Artifact B says (plan.md Task 3.2.3 Step 3 code):
```ts
picker.setPhase("repo-selection");
picker.setSelectedRepo(null);
// ← no setIssueSearch("") call
```

The plan's Escape handler clears `selectedRepo` and changes phase but does not clear `issueSearch`. If the user typed a search query in issue phase and presses Escape, they return to repo selection with the stale `issueSearch` value still set. On the next repo selection, that stale query would pre-populate the issue search, which is surprising behavior. design/ux.md is explicit that issueSearch must be cleared.

Resolution: Add `picker.setIssueSearch("")` to the Escape handler in Task 3.2.3 Step 3. This is also consistent with how research/ux.md §3 describes the behavior: "Return to repo selection step; clear issue results."

---

## Naming Inconsistencies

### N-1: Ubiquitous Language constant names vs. implemented function names — plan.md internal

Severity: minor (but causes confusion during implementation)

Artifact A says (plan.md Ubiquitous Language table):
- `REPOS_CACHE_KEY` — `"ssq:{origin}:gh-repos:v1"` — localStorage key constant
- `ISSUES_CACHE_KEY` — per-repo issues cache key constant
- `LAST_REPO_KEY` — localStorage key for last-used repo constant

Artifact B says (plan.md Task 2.1.1 Steps 4–6):
- `reposCacheKey(): string` — exported function
- `issuesCacheKey(owner, repo, state): string` — exported function
- `lastRepoCacheKey(): string` — exported function

And Story 2.1 AC calls `REPOS_CACHE_KEY` "a helper function" (not a constant).

Resolution: The UL incorrectly classifies these as constants when they are factory functions (because they embed `window.location.origin` dynamically). The function names in Task 2.1.1 are correct. Update the Ubiquitous Language table to use the function signatures `reposCacheKey()`, `issuesCacheKey()`, `lastRepoCacheKey()` and describe them as "helper functions" not "constants."

---

### N-2: `ghIssueListEntry` (Ubiquitous Language) vs. `ghIssueListJSON` (Task 1.3.1) — plan.md internal

Severity: minor

Artifact A says (plan.md Ubiquitous Language):
> `ghIssueListEntry` — "Private Go struct for GitHub REST API `/repos/{owner}/{repo}/issues` response fields"

Artifact B says (plan.md Task 1.3.1 Step 1):
> "Add private `ghIssueListJSON` struct..."

The Ubiquitous Language defines the name as `ghIssueListEntry`; the task that implements it uses `ghIssueListJSON`.

Resolution: The naming convention for private Go JSON structs in this codebase uses the `JSON` suffix (see `ghRepoJSON` in Task 1.2.1 Step 1). `ghIssueListJSON` is the correct name. Update the Ubiquitous Language entry from `ghIssueListEntry` to `ghIssueListJSON`.

---

### N-3: `GitHubAuthError` message text contradicts banner text — plan.md internal

Severity: minor

Artifact A says (plan.md Task 2.2.1 Step 5):
> `throw new GitHubAuthError("GitHub CLI not authenticated")`

Artifact B says (plan.md Story 3.2 AC note):
> "Note: do NOT say 'GitHub CLI not authenticated' — the backend uses the Go HTTP client, not gh CLI directly"

And Task 3.2.3 Step 4 banner text:
> "GitHub not authenticated. Run `gh auth login` or set a `GITHUB_TOKEN` environment variable."

The `GitHubAuthError` constructor string uses the prohibited phrasing "GitHub CLI not authenticated" while the rendered banner text is correctly phrased. The error object message leaks into console/debug output.

Resolution: Change Task 2.2.1 Step 5 to `throw new GitHubAuthError("GitHub not authenticated — no token configured")`. The banner text in Task 3.2.3 is already correct.

---

## Medium Inconsistencies

### M-1: ADR-002 context describes "gh CLI calls" but plan uses native HTTP client — ADR-002 ↔ plan.md

Severity: minor (context section only; decision itself is correct)

Artifact A says (ADR-002 Context section):
> "The `GitHubIssuePicker` makes two expensive `gh` CLI calls: `gh repo list` (~1–3 seconds, GraphQL paged call), `gh issue list --repo owner/repo`"

Artifact B says (plan.md Pattern Decisions table + requirements.md Constraints):
> "GitHub API: Native Go HTTP client via `github/http_client.go` (`newGHRequest` + `ghHTTPClient`)"
> "Must use the native Go HTTP client in the `github/` package — not `safeexec.CommandContext` subprocess calls"

ADR-002 was written to justify the caching approach; its context section describes the problem (expensive data fetching) using the legacy subprocess model. The actual implementation uses the native Go HTTP client. This makes the ADR context misleading for future readers.

Resolution: Update ADR-002 Context to read "two expensive GitHub REST API calls via the native Go HTTP client: repo listing (~1–2s on cold start) and issue listing (~0.3–1s per query)."

---

### M-2: Backend limit cap inconsistency — requirements.md ↔ plan.md

Severity: minor

Artifact A says (requirements.md Scalability NFR):
> "issue search results capped at 30 per query"

Artifact B says (plan.md Task 1.2.1 Step 2):
> "Validate `req.Msg.Limit`: default to 30, **cap at 100**."

The backend will accept requests for up to 100 results, while the requirement states a hard cap of 30. The frontend always sends limit=30, so this is not observable from the existing callers. However, a future caller could request 100 and the backend would comply, violating the stated constraint.

Resolution: Either (a) change the backend cap to 30 to enforce the constraint at the API boundary, or (b) update requirements.md to read "issue search results default to 30 per query; backend accepts up to 100." Option (a) is safer. Update Task 1.2.1 Step 2 to `cap at 30`.

---

### M-3: Empty states for "closed" and "all" filters not differentiated — plan.md ↔ design/ux.md

Severity: minor

Artifact A says (design/ux.md Strings table, §4):
- `issueStateFilter = "all"` + zero issues → requires distinct string "No issues in {owner}/{repo}." (no CTA)
- `issueStateFilter = "closed"` + zero issues → requires distinct string "No closed issues." with [Show open] CTA

Artifact B says (plan.md Task 3.2.2 Step 5):
> `filteredIssues.length === 0 && (issueStateFilter === "closed" || issueStateFilter === "all") && !isIssuesLoading` → `"No issues found."` (no action button)

The plan lumps "closed" and "all" together with a generic message and no CTA. design/ux.md identifies this as a trust failure (emotional JTBD §6): the user cannot distinguish "filtered out" from "genuinely empty."

Resolution: Split the combined condition in Task 3.2.2 Step 5 into two cases matching the design/ux.md spec. Update Task 3.2.2 Step 5 (c) and add a Step 5 (d):
- `issueStateFilter === "closed"` → "No closed issues." + [Show open] button
- `issueStateFilter === "all"` → "No issues in {owner}/{repo}." (no CTA)

---

### M-4: Skeleton loading threshold — research/ux.md ↔ plan.md/design/ux.md

Severity: minor

Artifact A says (research/ux.md §4 Loading States):
> "For debounced issue search (user is typing): show skeleton after 300ms if results haven't arrived. This avoids skeleton flash on fast network responses."

Artifact B says (plan.md Story 2.3 AC + design/ux.md §Phase 2 description):
> "`isIssuesLoading: true` is set before the 150ms debounce timer fires, so five skeleton rows appear immediately when the repo is first selected — the user never sees a blank list while the initial fetch is in-flight."

The research recommended a 300ms delay before showing the skeleton (to avoid flash). The plan shows the skeleton immediately (before the 150ms debounce even fires). This is a deliberate trade-off in the plan for the initial repo-selection case (not a debounced-search case). The research caveat applies to mid-search typing; the plan's "immediately on repo selection" is the first-load scenario.

Resolution: These are addressing different scenarios. The inconsistency is in scope, not in conflict. Add a clarifying comment to Task 2.3.2 Step 1: "Set `isIssuesLoading: true` immediately on repo selection (first load) — the 300ms skeleton-delay from research §4 applies only to mid-search typing latency, not to the initial repo-load trigger. For initial load, immediate skeleton is correct; for search-while-typing, the 150ms debounce provides implicit delay."

---

## Consistent Decisions (verified across all six artifacts)

- Two-step picker flow (repo then issue) with implicit visual state via RepoChip breadcrumb — no Step 1/Step 2 labels
- Local repos sourced from Redux `sessionsSlice` synchronously, not via RPC (ADR-001; confirmed in plan Task 2.3.1, design/ux.md Phase 1 description)
- localStorage caching: repos 4h TTL (`14_400_000 ms`), issues 5min TTL (`300_000 ms`), last-used repo no TTL (ADR-002; confirmed in plan Ubiquitous Language, issuePickerCache task, design/ux.md)
- Cache key format `"ssq:{origin}:gh-repos:v1"` / `"ssq:{origin}:gh-issues:{owner}/{repo}:{state}"` (ADR-002; plan Ubiquitous Language; issuePickerCache Task 2.1.1)
- Native Go HTTP client only — no `safeexec.CommandContext` subprocess for GitHub data (requirements; plan Pattern Decisions; ADR-002 decision section)
- No new npm packages (requirements; plan never introduces any)
- vanilla-extract `.css.ts` only for new components (requirements; plan Task 3.1; CSS architecture rule)
- `onMouseDown + e.preventDefault()` on all dropdown options to prevent onBlur-before-onClick race (plan Pattern Decisions; Task 3.2.1 Step 5; Task 3.2.3 Step 1; design/ux.md RepoChip section)
- Two-level Escape: first Escape in issue-search returns to repo-selection; Escape in repo-selection calls `onClose()` (all four non-ADR artifacts confirm this)
- No confirm step after issue selection — `onImport(issue.url)` called directly (research §2; plan Story 3.2 Step 5; design/ux.md Phase 2 description)
- GitHub color palette: open `#2da44e`, closed `#8250df` (research §4; plan Task 3.2.2 Step 2; plan Story 3.1 AC)
- ARIA pattern: `role="combobox"` on inputs, `role="listbox"` on `<ul>`, `role="option"` + `aria-selected` on `<li>` (research §3; plan Story 3.2 AC; design/ux.md ARIA table)
- `aria-autocomplete="list"` on the repo input (confirmed in plan Task 3.2.1 Step 3 and design/ux.md ARIA table); MISSING from issue input (see C-3 resolution in design/ux.md §5)
- 30-item default page size sent by frontend callers (plan Task 2.3.2, 2.3.1; confirmed in all RPC call sites)
