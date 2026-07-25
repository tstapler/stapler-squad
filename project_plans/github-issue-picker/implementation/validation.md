# Validation: GitHub Issue Picker Implementation Plan

Date: 2026-07-03
Readiness: READY

All critical concerns from architecture-review.md and adversarial-review.md are patched in the current plan. Four lower-priority gaps from the UX review survive but none block implementation. The plan is implementation-ready with three notes the implementer should act on immediately when starting Phase 5.

---

## Requirement Coverage

| Requirement / Success Metric | Story / Task |
|---|---|
| Import without leaving the app | Story 3.2, Task 3.3.1 |
| Repo list renders < 100ms from local data | Task 2.3.1 (Redux sessionsSlice, synchronous, zero RPC) |
| Issue search debounced, results < 2s | Task 2.3.2 (150ms debounce, AbortController, generationRef) |
| GitHub repos cached; second open skips network | Story 2.1, Task 2.3.1 (4h TTL localStorage cache) |
| Filter by open/closed state and labels | Task 3.2.2 (IssueFilterBar, labelFilter) |
| Must use native Go HTTP client in `github/` package | Task 1.2.1 step 3 (github/repos.go domain functions) |
| No new npm packages | No npm additions anywhere in the plan |
| All new styles use vanilla-extract `.css.ts` | Story 3.1 (GitHubIssuePicker.css.ts), Task 3.2.1 (no .module.css) |
| Existing raw-URL import not broken until replaced | Task 3.3.1 step 5 (keep old handler until e2e passes) |
| Two new backend RPCs: SearchGitHubRepos, ListGitHubIssues | Story 1.2, Story 1.3 |
| Keyboard navigation (arrows, Enter, Escape) | Tasks 3.2.1, 3.2.2, 3.2.3 |

Note: requirements.md states "GitHub search debounced at 300ms" in the non-functional section; the plan resolves the open question to 150ms (citing research/pitfalls.md §4.3). This is a deliberate override — 150ms is stricter. No action needed.

---

## Acceptance Criteria Ambiguity Assessment

All story-level ACs are testable with concrete given/when/then structure. No "should work correctly" language found.

**Escape AC — verified unambiguous.** Story 3.2 AC reads:
> "First Escape in `'issue-search'` phase: if `picker.issueSearch !== "" || picker.selectedRepo !== null`, sets `phase = "repo-selection"`, clears `selectedRepo`, does NOT close the modal. If BOTH `issueSearch === ""` AND `selectedRepo === null`, calls `onClose()`."

This matches Task 3.2.3 step 3 code exactly. Adversarial Finding 2 is resolved.

**Minor internal inconsistency (not blocking):** Task 2.2.1 step 5 constructs `new GitHubAuthError("GitHub CLI not authenticated")` — the error message still says "CLI". The displayed banner text in Task 3.2.3 step 4 correctly says "GitHub not authenticated." Because the banner text is hardcoded in the component rather than derived from the error message, the user-visible string is correct. However, the `GitHubAuthError` constructor message will appear in browser devtools. Recommend updating step 5 to `new GitHubAuthError("GitHub not authenticated")` for consistency.

---

## Task Dependency Ordering Assessment

The dependency diagram has one incorrect parallelism that will cause a build error if followed literally:

```
1.1.2 (make generate-proto)
  ├─► 1.2.1 (SearchGitHubRepos handler)   ← creates github/repos.go
  └─► 1.3.1 (ListGitHubIssues handler)    ← DEPENDS on github/repos.go
```

Task 1.3.1 step 3 says "call `github.ListRepoIssues(ctx, ...)` (domain function in `github/repos.go` — see Task 1.2.1 step 3)". The function `ListRepoIssues` is created during Task 1.2.1. The diagram shows 1.2.1 and 1.3.1 as parallel branches from 1.1.2, but 1.3.1 cannot compile until 1.2.1 creates repos.go.

The correct diagram for Epic 1 is: `1.1.1 → 1.1.2 → 1.2.1 → 1.3.1`, sequential, not parallel. No circular dependencies exist. All other dependency edges are correct.

github/repos.go is created inside Task 1.2.1 (step 3), which is the first handler task. Both handlers consume this file. The Epic 2 dependency "[after 1.1.2]" is correct in intent (TypeScript bindings needed) but should arguably read "[after 1.2.1]" to ensure repos.go compiles before the frontend type references are validated. This is a minor documentation issue, not a blocking one.

---

## Test Coverage Summary

| Layer | Test count | Pattern |
|---|---|---|
| Go backend (github/repos_test.go + backlog_service_test.go) | 9 | httptest.Server + ghBaseURL override |
| Frontend RTL (GitHubIssuePicker.test.tsx) | 6 | jest.mock(useGitHubIssuePicker) |
| Playwright e2e (github-issue-picker.spec.ts) | 3 | data-testid + ARIA roles, localhost:8544 |
| **Total** | **18** | |

The `ghBaseURL` override pattern is fully specified in Task 1.2.2 (Adversarial Finding 4 resolved): package-level `var ghBaseURL = "https://api.github.com/"` in `github/http_client.go`, tests set `github.GhBaseURL = testServer.URL + "/"` with a deferred reset. This makes httptest.Server intercept all requests correctly.

---

## Checklist: Review Concerns

### Architecture Review

| Concern | Status |
|---|---|
| github/ package export: use domain functions not raw HTTP primitives | RESOLVED — Task 1.2.1 step 3 creates `github/repos.go` with `SearchUserRepos` + `ListRepoIssues`; `newGHRequest`/`ghHTTPClient` remain unexported |
| Auth detection: inside domain functions via ErrNotAuthenticated | RESOLVED — domain functions return `ErrNotAuthenticated`; handler maps to `CodeUnavailable` |
| Research doc stale (safeexec sections superseded) | NOTE — no code change needed; note in kick-off |
| Search API rate limits (30/min vs 5000/hr) | LOW — acceptable for personal tool; code comment recommended on search path |

### Adversarial Review

| Concern | Status |
|---|---|
| onBlur-before-onClick race | CLEAN — no change needed |
| Escape AC contradiction | RESOLVED — Story 3.2 AC now unambiguous; matches Task 3.2.3 code |
| AbortController signal not threaded to ConnectRPC | UNRESOLVED — see Failure Mode 1 below |
| httptest.Server URL injection | RESOLVED — ghBaseURL override pattern specified in Task 1.2.2 |
| Rate limit 429 → generic CodeInternal | LOW — deferred; acceptable for personal tool |
| Proto field numbers | CLEAN — new messages each start at field 1 |
| URL detection missing from issue search phase | RESOLVED — Task 3.2.2 step 3 calls `detectIssueUrl(value)` shared helper from issue onChange |

### UX Review

| Concern | Status |
|---|---|
| Placeholder on repo search input | RESOLVED — Task 3.2.1 step 3: `placeholder="Search repos or paste a GitHub issue URL…"` |
| Placeholder on issue search input | UNRESOLVED — Task 3.2.2 has no step for issue input placeholder; HIGH per UX §1 |
| Two-level Escape visual affordance | UNRESOLVED — no status line ("Esc Go back") added to the plan; HIGH per UX §2 |
| Auth error copy ("GitHub CLI" → "GitHub not") | RESOLVED — Task 3.2.3 step 4: "GitHub not authenticated. Run `gh auth login` or set a `GITHUB_TOKEN`..." |
| Missing empty states for closed/all filter states | PARTIAL — Task 3.2.2 step 5 groups "closed" and "all" into one "No issues found." variant; UX recommends distinct text per state with [Show open] CTA for closed. Functional but lower-quality UX. |
| aria-autocomplete="list" on issue input | RESOLVED — Task 3.2.2 step 3 explicitly includes it |
| aria-selected on issue option li elements | RESOLVED — Task 3.2.2 step 3 specifies `aria-selected={i === selectedIndex}` |
| Skeleton row count discrepancy (3 vs 5) | LOW — intentional (issue list is primary surface); recommend a code comment |
| Dropdown open trigger unspecified | LOW — implementer decision; recommend on-focus per UX §7 |
| Tab key behavior unspecified | LOW — default browser behavior likely acceptable |
| "Recently used" indicator on pre-populated repo | LOW — deferred; not in plan |

---

## Three Most Likely Implementation Failures

### 1. Dependency ordering: Task 1.3.1 started before repos.go exists

Likelihood: medium
Impact: high (build breaks immediately on compilation)
Mitigation: Before starting 1.3.1, confirm `github/repos.go` exists and `github.ListRepoIssues` compiles. Treat the Epic 1 execution order as strictly sequential: 1.1.1 → 1.1.2 → 1.2.1 → 1.3.1. Do not parallelize 1.2.1 and 1.3.1 as the diagram implies.

### 2. AbortController signal not wired into ConnectRPC call

Likelihood: high (easy to miss — no task step mentions it)
Impact: low (the generation counter guards stale state; only wasted bandwidth/API quota result)
Mitigation: In Task 2.3.2, add a step: "Pass `{ signal: controller.signal }` as the request options to the ConnectRPC-generated `listGitHubIssues` client call. ConnectRPC TypeScript clients accept `AbortSignal` via the request init options object." This makes cancellation complete — not just stale-result guarded. Absence doesn't break the feature but wastes GitHub API quota on dismissed picker instances.

### 3. Issue search input has no placeholder text

Likelihood: high (no plan step exists for it)
Impact: medium (users in issue-search phase have no hint that they can paste a GitHub URL or search by title)
Mitigation: Add to Task 3.2.2 step 3: `placeholder={"Search issues in " + selectedRepo.owner + "/" + selectedRepo.repo + "..."}`. Dynamic interpolation using `selectedRepo` from hook state. This is the only discoverable hint for URL paste in the issue-search phase.

---

READY — all critical concerns (architecture, adversarial Findings 2 and 4) are resolved in the patched plan. Three implementer-action notes above should be addressed at the start of Phase 5, not deferred: the dependency ordering error will cause an immediate build failure, and the AbortSignal + placeholder gaps are small enough to fix inline during implementation but will be missed if not tracked.
