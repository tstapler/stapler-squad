# Validation Plan: GitHub Work Continuity

**Status**: Phase 4 complete — ready for implementation
**Date**: 2026-06-24
**Plan ref**: `implementation/plan.md`
**Requirements ref**: `requirements.md`
**Adversarial review ref**: `implementation/adversarial-review.md`

---

## 1. Unit Tests — Go

### 1.1 WorktreePRPoller (`session/worktree_pr_poller_test.go`) — NEW FILE

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| U-01 | `TestWorktreePRPoller_StartStop` | Start() launches background goroutine; Stop() cancels context and Wait() returns without hanging. Verify via WaitGroup + timeout. | Plan Story 2.1 AC: clean shutdown |
| U-02 | `TestWorktreePRPoller_ScanDoneIntegration` | Feed a mock scanner that fires `ScanDone()` channel; verify poller calls `GetAllResults()` and populates `data` map with PR info from stub `GetPRForBranch`. | Plan Story 2.1 AC: subscribes to ScanDone |
| U-03 | `TestWorktreePRPoller_AuthFailureGraceful` | When `CheckGHAuth()` returns false, polling loop does not call `GetPRForBranch` and stores no data. Verify no panic and subsequent auth recovery resumes polling. | SC-4 (auth detection), Plan Story 2.1 AC: auth checked once per 5-minute interval |
| U-04 | `TestWorktreePRPoller_NoDoublePolling` | Construct with a mock `PRStatusPoller` that owns worktree path `/repo/A`. Verify `WorktreePRPoller.pollWorktrees()` skips `/repo/A` and only polls `/repo/B`. | Plan Story 2.1 AC: session-owned worktrees skipped |
| U-05 | `TestWorktreePRPoller_SessionOwnedWorktreesSkipped` | PRStatusPoller owns 3 worktrees; all 3 are in `GetAllResults()`; verify zero `GetPRForBranch` calls. | Plan Story 2.1 AC: "skip any ScanResult.WorktreePath that appears in PRStatusPoller's instance list" |
| U-06 | `TestWorktreePRPoller_GetPRDataCacheHit` | After a poll tick populates data for `repoPath|branch`, `GetPRData()` returns cached `PRInfo` without a new HTTP call. | Plan Story 2.3 AC: enrichment from GetPRData |
| U-07 | `TestWorktreePRPoller_OnUpdatedCallbackFires` | Stub PR transitions from state "open" to "merged"; verify `onUpdated` callback is invoked with updated `PRInfo`. | Plan Story 6.4 AC: onUpdated fires on state change |
| U-08 | `TestWorktreePRPoller_ConcurrentAccess` | 100 goroutines call `GetPRData()` while 10 goroutines call `pollWorktrees()`. Run with `-race`; expect no race detector output. | Plan Story 2.1 AC: thread-safe |

**Total U-01 to U-08 = 8 unit tests**

---

### 1.2 UserPRCache (`github/user_pr_cache_test.go`) — NEW FILE

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| U-09 | `TestUserPRCache_GraphQLFetch` | Mock `gh api graphql` subprocess returning valid JSON fixture; verify parsed `[]*UserPR` slice has correct field mappings (Owner, Repo, Number, State, Priority). | Plan Story 3.1 AC: fetches via gh api graphql |
| U-10 | `TestUserPRCache_TTLRefresh` | Prime cache at T=0; mock clock at T=4m59s → `GetOpenPRs()` returns stale; at T=5m01s background loop has refreshed → new data returned. | Plan Story 3.1 AC: 5-minute refresh loop |
| U-11 | `TestUserPRCache_ConcurrentAccess` | `GetOpenPRs()` and `GetByBranch()` called from 50 goroutines while `refresh()` runs. Run with `-race`. | Plan Story 3.1 AC: thread-safe |
| U-12 | `TestUserPRCache_AuthErrorSurface` | When `gh api graphql` exits non-zero with "authentication required" message, cache sets error state; `GetOpenPRs()` returns empty slice; no infinite retry loop. | Plan Story 3.1 AC: auth failure stops refreshing, stores error |
| U-13 | `TestUserPRCache_DerivePriorityApplied` | GraphQL fixture returns PR with 0 approvals, 1 change_requested; verify returned `UserPR.Priority == PRPriorityBlocking`. | Plan Story 3.1 AC: calls DerivePRPriority on each result |
| U-14 | `TestUserPRCache_GetByBranch` | Cache contains PRs across 3 repos; lookup by specific owner/repo/branch returns correct PR; miss returns nil. | Plan Story 3.2 AC: GetByBranch for local worktree linking |
| U-15 | `TestUserPRCache_AnnotateWithSessions` | Call `Annotate()` with mock sessions; verify `UserPR.SessionIDs` populated for matching branch+owner pairs. | Plan Story 3.2 AC: Annotate sets SessionIDs |

**Total U-09 to U-15 = 7 unit tests**

---

### 1.3 UnfinishedWorkService.ListUnfinishedWork (`server/services/unfinished_work_service_test.go` — extend existing file or new sub-test)

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| U-16 | `TestListUnfinishedWork_EnrichmentJoin_PRPriorityAdded` | ScanResult for `/repo/A` on branch `feat/X`; WorktreePRPoller stub returns `PRInfo{State:"open", Priority: ready}`; verify returned `UnfinishedWorktree.GithubPrPriority == "ready"`. | SC-3 (PR enriches worktree items), Plan Story 2.3 AC |
| U-17 | `TestListUnfinishedWork_CompletedWorktreeSection` | ScanResult for `/repo/B` on branch `feat/Y`; PRInfo.State = "merged"; verify item appears in `completed_worktrees` response field, not in `worktrees`. | SC-3 (merged PR → deprioritized), Plan Story 4.2 AC |
| U-18 | `TestListUnfinishedWork_NoGHAuthPath` | WorktreePRPoller returns auth error state; verify `ListUnfinishedWork` returns clean response (all worktrees in `worktrees` field, GitHub fields zero/empty, no crash). | SC-4 (auth detection), Plan Story 2.3 AC: "no crash if no PR data" |
| U-19 | `TestListUnfinishedWork_SessionPRIndexTakesPrecedence` | Worktree has active session with `GitHubPrPriority = "blocking"` AND WorktreePRPoller has stale `ready`; verify session data wins (Story 2.3 enrichment priority). | Plan Story 2.3 AC: "For worktrees with active session: use Instance.GitHubPR* fields" |
| U-20 | `TestListUnfinishedWork_NoPRDataReturnsClean` | WorktreePRPoller returns nil for all lookups; verify proto fields are zero/empty (not panic, not garbage). | Plan Story 2.3 AC: "If no PR data is available, GitHub fields are zero/empty" |

**Total U-16 to U-20 = 5 unit tests**

---

### 1.4 DerivePRPriority — Verify Existing Tests (`github/priority_test.go`)

**Finding**: `github/priority_test.go` does NOT currently exist. Only `github/url_parser_test.go` is present. `DerivePRPriority` in `github/priority.go` has no test coverage.

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| U-21 | `TestDerivePRPriority_NilInfo` | `DerivePRPriority(nil)` returns `PRPriorityNoPR`. | Plan Story 3.1 (called on every UserPR) |
| U-22 | `TestDerivePRPriority_MergedState` | `State="merged"` → `PRPriorityComplete`. | SC-3 (merged PR deprioritized) |
| U-23 | `TestDerivePRPriority_ClosedState` | `State="closed"` → `PRPriorityComplete`. | SC-3 |
| U-24 | `TestDerivePRPriority_Draft` | `IsDraft=true`, state="open" → `PRPriorityDraft`. | SC-1 (priority badge variants) |
| U-25 | `TestDerivePRPriority_ChangesRequested` | `ChangesRequestedCount=1` → `PRPriorityBlocking`. | SC-1 |
| U-26 | `TestDerivePRPriority_CIFailure` | `CheckConclusion="failure"` → `PRPriorityBlocking`. | SC-1 |
| U-27 | `TestDerivePRPriority_ApprovedCIPassing` | `ApprovedCount=1`, `CheckConclusion="success"` → `PRPriorityReady`. | SC-1 |
| U-28 | `TestDerivePRPriority_Pending` | No approvals, `CheckConclusion="pending"` → `PRPriorityPending`. | SC-1 |

**Note for implementation gate**: `github/priority_test.go` must be created as part of Epic 2 work. These 8 tests cover the function already in place.

**Total U-21 to U-28 = 8 unit tests**

---

**Unit test subtotal: 28 tests across 4 subjects**

---

## 2. Integration Tests — Go

### 2.1 PRStatusPoller + WorktreePRPoller Combined (no double-polling)

**File**: `session/worktree_pr_poller_integration_test.go` (new file, build tag `//go:build integration`)

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| I-01 | `TestIntegration_NoDoublePolling_ActiveSessions` | Construct a real `PRStatusPoller` owning 2 instances. Construct `WorktreePRPoller` referencing the same poller. Feed 4 scan results, 2 matching session worktrees, 2 new. Verify `GetPRForBranch` called exactly 2 times (not 4). | Plan Story 2.1 AC: "no double-polling" |
| I-02 | `TestIntegration_PollerHandoff_SessionEnds` | Session ends (removed from PRStatusPoller); verify WorktreePRPoller picks up that worktree on next poll tick. | Plan Story 2.3 end-to-end continuity |

**Total I-01 to I-02 = 2 integration tests**

---

### 2.2 GitHubUserService.ListUserPRs RPC End-to-End

**File**: `server/services/github_user_service_test.go` (new file)

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| I-03 | `TestGitHubUserService_ListUserPRs_MockGH` | Inject a mock `UserPRCache` pre-populated from a captured `gh api graphql` fixture. Call `ListUserPRs` RPC. Verify response contains correct count of open PRs + 5 recent closed. Verify priority sort order: blocking first. | SC-2, Plan Story 3.4 AC |
| I-04 | `TestGitHubUserService_ListUserPRs_Unauthenticated` | UserPRCache in auth-error state. Verify RPC returns empty PR list with `auth_state.available = false` (not gRPC error). | SC-4, Plan Story 3.4 AC: "Unauthenticated state returns empty list, not error" |
| I-05 | `TestGitHubUserService_GetGitHubAuthState_Available` | Mock `gh api user` subprocess returns valid user JSON. Verify `GetGitHubAuthState` returns `available=true` with correct username. | SC-4 |

**Total I-03 to I-05 = 3 integration tests**

---

**Integration test subtotal: 5 tests across 2 subjects**

---

## 3. Frontend Tests — TypeScript/React

### 3.1 GitHubBadge Priority Variant Rendering

**Finding**: `web-app/src/components/sessions/GitHubBadge.tsx` exists (179 lines, Epic 5 complete). No test file `GitHubBadge.test.tsx` exists in `web-app/src/components/sessions/` or `__tests__/`. Tests must be created.

**File**: `web-app/src/components/sessions/GitHubBadge.test.tsx` (new file)

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| F-01 | `renders blocking variant with correct color and label` | Pass `priority="blocking"` prop; assert badge has correct CSS class/color token and label text "Blocking". | SC-1 (color-coded badge) |
| F-02 | `renders ready variant` | `priority="ready"` → green badge. | SC-1 |
| F-03 | `renders pending variant` | `priority="pending"` → yellow badge. | SC-1 |
| F-04 | `renders draft variant` | `priority="draft"` → gray badge. | SC-1 |
| F-05 | `renders complete variant` | `priority="complete"` → muted badge. | SC-1 |
| F-06 | `renders no_pr variant` | `priority="no_pr"` → no badge rendered (or branch-only label). | SC-1 |
| F-07 | `tooltip shows review counts` | `approvedCount=2, changesReqCount=1` → tooltip text contains "2 approvals, 1 change requested". | SC-2 (review counts visible) |
| F-08 | `has accessible aria-label` | Badge element has `aria-label` or `title` attribute present (a11y). | Plan Epic 5 GitHubBadge a11y spec |

**Total F-01 to F-08 = 8 frontend tests**

---

### 3.2 UnfinishedWorkTab — GitHub PRs Section

**File**: `web-app/src/components/unfinished/GitHubPRsSection.test.tsx` (new file)

| # | Test Name | Description | Requirement Traced |
|---|-----------|-------------|-------------------|
| F-09 | `renders PR list from WatchUserPRs stream data` | Mock `useWatchUserPRs` hook returns 3 open PRs; assert 3 PR card elements rendered with correct PR number and title. | SC-2 (GitHub PRs section lists open PRs) |
| F-10 | `shows auth banner when auth_state.available is false` | Mock hook returns `authAvailable=false`; assert dismissible banner element rendered with text matching "GitHub not connected" pattern. | SC-4 (auth banner) |
| F-11 | `renders empty state when no open PRs` | Mock hook returns 0 open PRs; assert empty state element is rendered (not error, not crash). | Plan Story 3.5 AC: loading state / empty state |
| F-12 | `shows loading state on first load` | Mock hook returns `isLoading=true`; assert loading indicator visible, no PR cards rendered. | Plan Story 3.5 AC |
| F-13 | `renders Recent section with closed PRs` | Mock hook returns 0 open, 3 closed; assert "Recent" section rendered with 3 compact rows. | SC-2 ("Recent" subsection) |
| F-14 | `PR card links to session when session_ids non-empty` | Mock PR with `sessionIds=["sess-123"]`; assert rendered card contains link/button for that session. | SC-2 (links to session if one exists) |
| F-15 | `PR card shows Start Session when no session` | Mock PR with `sessionIds=[]`; assert rendered card contains "Start session" element. | SC-2 |
| F-16 | `section collapse/expand persists to localStorage` | Click collapse toggle; assert localStorage item updated; re-render; assert section collapsed. | Plan Story 3.5 AC: localStorage persistence |

**Total F-09 to F-16 = 8 frontend tests**

---

**Frontend test subtotal: 16 tests across 2 subjects**

---

## 4. Manual Verification Checklist

These checks cannot be automated via unit or integration tests and require a running dev environment.

### M-01: Session Card Badge Updates Within 60 Seconds

**Steps**:
1. Start a session on a branch with an open GitHub PR that has no review activity (priority: pending).
2. In GitHub UI, request changes on the PR.
3. Wait up to 60 seconds.

**Expected**: Session card badge transitions from "Pending" to "Blocking" without page reload.

**Traces to**: SC-1 ("within 60 seconds of state change")

---

### M-02: Unfinished Tab GitHub PRs Section Loads on Page Open

**Steps**:
1. Ensure `gh auth status` shows authenticated user.
2. Open Stapler Squad. Navigate to Unfinished Work tab.

**Expected**: "GitHub PRs" section appears at top of tab. At least one open PR listed within 10 seconds.

**Traces to**: SC-2 (GitHub PRs section in Unfinished tab)

---

### M-03: Merged PR Causes Worktree Item Move to Completed Section

**Steps**:
1. Open a session on branch `feat/X` with an open PR.
2. In GitHub UI, merge the PR.
3. Wait up to 60 seconds for poll tick.

**Expected**: In the Unfinished tab, the worktree card for `feat/X` moves from the active section to the "Completed" section (visually de-emphasized / grayed out). No page reload required.

**Traces to**: SC-3 ("merged/closed PR → shown at bottom, grayed out"), Plan Story 6.4

---

### M-04: Auth Banner Shows When `gh auth status` Fails

**Steps**:
1. Log out of GitHub CLI: `gh auth logout`.
2. Restart Stapler Squad server.
3. Navigate to Unfinished Work tab.

**Expected**: GitHub PRs section shows dismissible banner containing "GitHub not connected" (or equivalent message). No crash. Worktree items still render (GitHub fields just empty).

**Traces to**: SC-4 ("surface auth state"), requirements.md "GitHub account detection"

---

### M-05: Commit/Push Diff Preview Modal Appears Before Execute

**Steps**:
1. In Unfinished tab, locate a worktree card with uncommitted changes.
2. Click "Commit & Push" action.

**Expected**: A modal appears showing: (a) list of changed files with +/- counts, (b) collapsible diff per file, (c) commit message input, (d) "Confirm & Push" and "Cancel" buttons. Push does NOT execute until "Confirm & Push" is clicked.

**Traces to**: G5 gap fix, Plan Story 6.1, requirements.md "Fix G5"

---

## 5. Requirement-to-Test Traceability Matrix

| Success Criterion | Unit Tests | Integration Tests | Frontend Tests | Manual |
|---|---|---|---|---|
| SC-1: Session card badge within 60s | U-21..U-28 (priority logic) | I-01 (no double-poll) | F-01..F-08 (badge rendering) | M-01 |
| SC-2: GitHub PRs section in Unfinished tab | U-09..U-15 (UserPRCache) | I-03..I-05 (RPC e2e) | F-09..F-16 (section rendering) | M-02 |
| SC-3: PR enriches worktree items; merged → completed | U-16..U-20 (ListUnfinishedWork) | I-01 (integration) | F-09 (data rendered) | M-03 |
| SC-4: Auto-detected GitHub account, auth banner | U-03, U-12 (auth failure paths) | I-04, I-05 (RPC auth) | F-10 (banner render) | M-04 |
| SC-5: Zero new browser tabs needed | — (emergent from SC-1..4) | — | F-14, F-15 (session links) | M-01..M-03 |
| G5 gap: Commit/push diff preview | — | — | — | M-05 |

**Coverage**: 5 of 5 success criteria have at least one automated test. G5 gap fix covered by M-05 manual check.

---

## 6. Test File Summary

| File | Status | Tests |
|---|---|---|
| `session/worktree_pr_poller_test.go` | NEW — must create | U-01..U-08 (8) |
| `github/user_pr_cache_test.go` | NEW — must create | U-09..U-15 (7) |
| `server/services/unfinished_work_service_test.go` | EXTEND or new sub-file | U-16..U-20 (5) |
| `github/priority_test.go` | NEW — must create (function exists, no tests) | U-21..U-28 (8) |
| `session/worktree_pr_poller_integration_test.go` | NEW — build tag `integration` | I-01..I-02 (2) |
| `server/services/github_user_service_test.go` | NEW | I-03..I-05 (3) |
| `web-app/src/components/sessions/GitHubBadge.test.tsx` | NEW — component exists, no test | F-01..F-08 (8) |
| `web-app/src/components/unfinished/GitHubPRsSection.test.tsx` | NEW — component is new | F-09..F-16 (8) |
| Manual checklist | — | M-01..M-05 (5 scenarios) |

**Total automated test cases: 49 (28 unit + 5 integration + 16 frontend)**
**Total manual scenarios: 5**

---

## 7. Adversarial Review Patch Confirmation

The adversarial review verdict was BLOCKED on 4 issues. All have been patched in plan.md before this validation was written:

| Issue | Adversarial Severity | Status in plan.md |
|---|---|---|
| `github.CachedAuthState` type fabrication | BLOCKER | PATCHED — Story 2.1 struct now uses inline `authOK bool / authCheckedAt / authMu` fields |
| `scanFeed <-chan []unfinished.ScanResult` channel does not exist | BLOCKER | PATCHED — struct uses `*unfinished.Scanner` + `ScanDone()` + `GetAllResults()` pattern |
| New `session` → `session/unfinished` import not acknowledged | BLOCKER | PATCHED — Story 2.1 implementation note explicitly calls out new import direction and architecture constraint |
| `completed_worktrees = 2` proto field number conflict with `last_scan = 2` | BLOCKER | PATCHED — Story 4.2 now uses field number 3 |
| `make generate-proto` wrong target | CONCERN | PATCHED — all occurrences replaced with `make proto-gen` |
| EventBus event name "WorktreeUpdated" incorrect | CONCERN | PATCHED — Stories 2.2 and 6.4 now reference `EventUnfinishedWorkUpdated` |
| Raw `ScanResult.SessionIDs` empty warning missing | CONCERN | PATCHED — Story 2.1 note added |
| `GitHubUserService` mux registration thin spec | CONCERN | PATCHED — Story 3.4 now has explicit checklist for `ServiceDeps` and registration block |

**Post-patch adversarial verdict: CONCERNS CLEAN** (no remaining blockers; concern #9 PR boundaries is documentation-only and does not block implementation).

---

## 8. Implementation Readiness Gate

### Criterion 1: requirements.md complete

- Problem statement: PASS (solo-dev context, concrete pain points documented)
- Success criteria: PASS (5 numbered, measurable criteria)
- Scope: PASS (Must Have / Should Have / Could Have / Out of Scope all defined)
- Constraints: PASS (tech stack, rate limits, package boundaries, incremental delivery all specified)

**Verdict: PASS**

---

### Criterion 2: plan.md epics → stories → tasks with file paths and acceptance criteria

- Epic 1–6: PASS — each story has named files, acceptance criteria, implementation notes
- Story 2.1 patches applied: PASS (struct corrected, import direction noted, ScanDone API used)
- Story 4.2 proto field conflict corrected: PASS (field 3 not 2)
- All make targets corrected to `proto-gen`: PASS
- EventBus names corrected to `EventUnfinishedWorkUpdated`: PASS

**Verdict: PASS**

---

### Criterion 3: validation.md maps tests to requirements (traceability matrix)

- Section 5 traceability matrix covers all 5 success criteria: PASS
- Each criterion has at least 1 automated test: PASS
- G5 gap (commit/push modal) covered by M-05: PASS

**Verdict: PASS**

---

### Criterion 4: adversarial-review.md verdict not BLOCKED

- Original verdict: BLOCKED (4 blockers, 5 concerns)
- Post-patch status: all 4 blockers patched in plan.md (Patches 1–6 applied)
- Remaining: 0 blockers, concern #9 (PR boundary docs) is non-blocking
- Current verdict: **CONCERNS CLEAN**

**Verdict: PASS**

---

## Readiness Gate Final Verdict: **PASS**

All 4 gate criteria pass. The plan is clear to enter implementation (Epic 1 first).

**Test case counts**:
- Unit tests: 28
- Integration tests: 5
- Frontend tests: 16
- Manual scenarios: 5
- **Total automated: 49**

**Requirements coverage**: 5/5 success criteria have at least one automated test (100%).
