# Validation Plan: vcs-tab-redesign

**Date**: 2026-08-27

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario | Plan.md Task |
|---|---|---|---|---|---|
| Scope-1: Commit list | `session/git/ops_test.go` | `TestListShippedCommits_should_ReturnNewestFirst_When_MultipleCommitsShipped` (pre-existing) | Unit | Happy path | n/a (pre-existing; re-verified by Task 5.1.2a's delegation change) |
| Scope-1: Commit list | `session/git/ops_test.go` | `TestListShippedCommits_should_ReportTruncatedTrue_When_CommitCountExceedsCap` | Unit | Error/edge (cap hit) | Task 5.1.2a |
| Scope-1: Commit list | `server/services/workspace_service_test.go` | `TestGetVCSStatus_should_PopulateCommitsAndAggregateDiffStat_When_DirectorySessionHasBaseSHA` (name per Task 5.1.2c's description) | Integration | Directory-mode session, real fixture repo | Task 5.1.2c |
| Scope-1: Commit list | `.../vcs-widget/VcsWidgetCommitList.test.tsx` | `VcsWidgetCommitList_should_CapAtTwentyWithShowAllButton_When_FullModeWithTwentyFivCommits` | Unit (component) | Happy path | Task 4.2.1b |
| Scope-1: Commit list | `.../vcs-widget/VcsWidgetCommitList.test.tsx` | `VcsWidgetCommitList_should_RenderUnavailableNotice_When_CommitsEmptyAndUnavailableTrue` | Unit (component) | Error path (fetch failed) | Task 4.2.2c |
| Scope-2: Aggregate diff-stat | `session/git/ops_test.go` | `TestDiffStatBetween` (table-driven: additions-only/deletions-only/mixed/zero-diff) | Unit | Happy path | Task 5.1.2d |
| Scope-2: Aggregate diff-stat | `session/git/ops_test.go` | `TestDiffStatBetween` (invalid-SHA subtest) | Unit | Error path (`FileStatsBetween` passthrough) | Task 5.1.2d |
| Scope-2: Aggregate diff-stat | `server/services/workspace_service_test.go` | (same test as Scope-1's integration row — asserts `AggregateDiffStat` too) | Integration | Directory-mode session | Task 5.1.2c |
| Scope-2: Aggregate diff-stat | `.../shared/VcsWidget.test.tsx` | `VcsWidget_should_RenderAggregateStatLine_When_FullModeWithAggregateStatsPresent` | Unit (component) | Happy path | Task 4.1.1b |
| Scope-3: Itemized CI checks | *(none — see Gap 1)* | — | Unit | Happy path (`GetPRInfoCtx` parses `Checks`) | **GAP** |
| Scope-3: Itemized CI checks | *(none — see Gap 1)* | — | Unit | Error path (empty `StatusCheckRollup`) | **GAP** |
| Scope-3: Itemized CI checks | `.../vcs-widget/VcsWidgetCheckList.test.tsx` | (unnamed in plan — "renders N rows for N checks with correct status glyph per conclusion") | Unit (component) | Happy path | Task 4.3.1d |
| Scope-3: Itemized CI checks | `.../vcs-widget/VcsWidgetCheckList.test.tsx` | (unnamed in plan — "renders null for empty checks") | Unit (component) | Error/empty path | Task 4.3.1d |
| Scope-4: Why-blocked rollup | `web-app/src/lib/vcs/mergeability.test.ts` | `deriveBlockingReasons_should_ReturnAllThreeReasons_When_DraftAndChangesRequestedAndCiFailingCoOccur` | Unit | Happy path (multi-reason) | Task 3.1.1b |
| Scope-4: Why-blocked rollup | `web-app/src/lib/vcs/mergeability.test.ts` | (unnamed in plan — `shipped: true` short-circuit) | Unit | Error/edge (empty result) | Task 3.1.1b |
| Scope-4: Why-blocked rollup | `.../vcs-widget/VcsWidgetBlockingReasons.test.tsx` | `VcsWidgetBlockingReasons_should_RenderStaleNotice_When_LastCheckedAtExceedsThreshold` | Unit (component) | Edge case (stale data) | Task 4.5.1c |
| Scope-5: Reviewer body text | *(none — see Gap 2)* | — | Unit | Happy path (`GetPRInfoCtx`/`parseReviewCounts` capture `Body`) | **GAP** |
| Scope-5: Reviewer body text | `.../vcs-widget/VcsWidgetReviewFeedback.test.tsx` | (unnamed — "renders review body text for a CHANGES_REQUESTED entry") | Unit (component) | Happy path | Task 4.4.1c |
| Scope-5: Reviewer body text | `.../vcs-widget/VcsWidgetReviewFeedback.test.tsx` | `VcsWidgetReviewFeedback_should_RenderRawHtmlAsLiteralText_When_ReviewBodyContainsHtmlTags` | Unit (component) | Error/security path (XSS) | Task 4.4.1c |
| Scope-6: Staleness timestamp | `server/services/workspace_service_test.go` | (same integration test as Scope-1 — asserts `StatusAsOf` set on fresh-compute) | Integration | Happy path (fresh compute) | Task 5.1.2c |
| Scope-6: Staleness timestamp | *(none — see Gap 3)* | — | Unit | Edge path (cache-hit branch sets `StatusAsOf` from `cachedAt`, distinct from fresh-compute) | **GAP** |
| Scope-6: Staleness timestamp | `.../shared/VcsWidget.test.tsx` | (unnamed — "both labels render when both timestamps present... omitting one doesn't hide the other") | Unit (component) | Happy + edge (partial data) | Task 4.5.2b |
| Scope-7: Ahead/behind-vs-base | `.../vcs-widget/VcsWidgetHeader.test.tsx` | pre-existing (`aheadOfMain: 3, behindMain: 1`; and a `0/0` case at line 54) | Unit (component) | Happy + edge path | **Pre-existing, not a plan.md task — see note below** |
| OQ1: PR comments | `.../vcs-widget/VcsWidgetComments.test.tsx` | (unnamed — "expanding triggers exactly one `GetPRComments` call... re-expanding does not re-fetch") | Unit (component) | Happy path (lazy fetch + cache) | Task 4.6.2c |
| OQ1: PR comments | `.../vcs-widget/VcsWidgetComments.test.tsx` | (unnamed — "section starts collapsed and makes no RPC call") | Unit (component) | Error/edge (no premature fetch) | Task 4.6.2c |
| OQ2: Itemized-checks data source (poller, zero new API calls) | *(none — see Gap 1)* | — | Unit | `applyPRUpdate` threads `Checks`/`Reviews`/`Mergeable` into `PRStatusUpdate` | **GAP** |
| OQ3: Two staleness labels, not one blended | `.../shared/VcsWidget.test.tsx` | (unnamed, same as Scope-6 component row) | Unit (component) | Happy + edge | Task 4.5.2b |
| OQ3: Two staleness labels, not one blended | `.../vcs-widget/VcsWidgetBlockingReasons.test.tsx` | `VcsWidgetBlockingReasons_should_RenderStaleNotice_When_LastCheckedAtExceedsThreshold` | Unit (component) | Edge (3x-poller-interval threshold) | Task 4.5.1c |
| OQ4: Local-only (no PR) sessions | `session/instance_terminal_test.go` | (unnamed — "returns recorded SHA for a **directory** session... and `\"\"` when neither is set") | Unit | Happy + error path | Task 5.1.2b |
| OQ4: Local-only (no PR) sessions | `server/services/workspace_service_test.go` | (unnamed — "no recorded base SHA asserts `Commits`/`AggregateDiffStat` stay unset without erroring") | Integration | Error/edge path (no base SHA) | Task 5.1.2c |
| OQ5: Mobile layout (default-closed sections) | `.../vcs-widget/VcsWidgetCheckList.test.tsx` | (unnamed — "section starts collapsed (`aria-expanded=\"false\"`)") | Unit (component, standalone) | Happy path | Task 4.3.1d |
| OQ5: Mobile layout (default-closed sections) | *(none — see Gap 4)* | — | Integration (component) | `VcsWidget.tsx`'s actual `CollapsibleGroup` composition renders default-closed, not just the standalone subcomponent | **GAP** |

## Test Stack

- **Backend (Go)**: stdlib `testing` + `github.com/stretchr/testify` (`require`/`assert`) — confirmed via `go.mod:45` and existing usage in `session/git/ops_test.go`. Fixture-repo pattern (real `git init` + commits in `t.TempDir()`, via a shared helper) is the established convention for `session/git` tests; `server/services` tests use a similar fixture-repo + in-memory service wiring (`workspace_service_test.go`).
- **Frontend (TS)**: Jest 30 + `@testing-library/react` 16 + `@testing-library/jest-dom`/`user-event` (confirmed via `web-app/package.json`). Component tests render via RTL, assert DOM output/`aria-*` attributes, and mock ConnectRPC clients for RPC-triggering components (`VcsWidgetComments`).
- **Integration**: fixture git repos (real `git init`/commit sequences in a temp dir), following the precedent in `session/git/ops_test.go` and extended in this plan's Task 5.1.2c to exercise `WorkspaceService.GetVCSStatus` end-to-end (fixture repo → provider → cache → proto mapping) rather than mocking `vc.GitProvider`.

## Coverage Gaps Found — all 4 closed directly in plan.md

Four real gaps were found — requirements with no corresponding **backend or composition-level**
test task in plan.md, distinct from mere naming reorganization. All four were patched directly
into `plan.md` immediately after this validation pass (cheap at the planning stage; each gap
below states which new/edited task closed it):

1. **`Checks`/`Reviews` parsing and poller-threading has zero backend test coverage** (covers Scope-3, Scope-5, and OQ2). Three specific untested functions, each in a file that already has an established test pattern to extend:
   - `GetPRInfoCtx`'s new `Checks []CheckItem`/`Reviews []ReviewItem` population from `resp.StatusCheckRollup`/`resp.Reviews` (Task 1.2.1b, `github/client.go`) — `github/client_pr_by_number_test.go` already exercises `GetPRInfoCtx` via mocked `gh` responses (`TestGetPRByNumber_should_ReturnPRInfo_When_PRExists` et al.) but no task adds a check/review-itemization assertion there.
   - `checksToProto`/`reviewFeedbackToProto` (Task 2.2.2a, `server/adapters/instance_adapter.go`) — `server/adapters/instance_adapter_test.go` already exists with the exact `TestInstanceToProto_*` naming convention these would extend, but no task adds one.
   - `applyPRUpdate`'s `PRStatusUpdate` construction now setting `Checks`/`Reviews`/`Mergeable` (Task 1.2.2c, `session/pr_status_poller.go`) — `session/pr_status_poller_test.go` already exists and already tests `applyPRUpdate`'s change-detection behavior (`TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange`), but no task extends it to assert the 3 new fields survive the round trip.

   **CLOSED**: inserted `Story 5.1.4: Backend test coverage for itemized checks/review feedback`
   after Story 5.1.2 in plan.md, before Story 5.1.3's `make quick-check` gate, with three tasks:
   - Task 5.1.4a — `TestGetPRInfoCtx_should_PopulateChecksAndReviews_When_StatusCheckRollupAndReviewsPresent` in `github/client_pr_by_number_test.go`.
   - Task 5.1.4b — `TestInstanceToProto_should_MapChecksAndReviewFeedback_When_Populated` in `server/adapters/instance_adapter_test.go`.
   - Task 5.1.4c — `TestApplyPRUpdate_should_ThreadChecksReviewsMergeable_When_PRInfoPopulated` in `session/pr_status_poller_test.go`.

2. **Scope-3/Scope-5's frontend component tests only exercise the isolated `CheckItem`/`ReviewItem` types built by hand in the test file** — this is expected and fine for component-level unit tests, but it means the *only* place the backend-to-frontend field mapping (`github.CheckItem` → proto `GithubCheckItem` → TS `CheckItemSummary`) is exercised end-to-end is nowhere, once Gap 1 is accounted for. Gap 1's fix closes this.

3. **`GetVCSStatus`'s cache-hit branch setting `StatusAsOf` from `entry.cachedAt` is untested** (Task 1.1.3c, `server/services/workspace_service.go`). Task 5.1.2c's integration test only exercises the fresh-compute path (a single call against a fixture repo); there's no assertion that a second `GetVCSStatus` call within the 15s cache window returns `StatusAsOf` reflecting the *original* `cachedAt` rather than the second call's time — the exact distinction Open Question 3's "PR status last confirmed" vs. "checks last updated" copy rationale depends on getting right for the local-staleness label too. Lower severity than Gap 1 (a one-line code path, not unexercised parsing logic), but the two-branch nature of Task 1.1.3c's acceptance criteria ("both cache-hit and fresh-compute response branches") is only half-tested.

   **CLOSED**: added the exact assertion to Task 5.1.2c — a fifth sub-assertion calling
   `GetVCSStatus` twice within the cache window and asserting the second response's
   `StatusAsOf` equals the first's exact value, not `time.Now()` at the second call.

4. **No test verifies `VcsWidget.tsx`'s actual composed `CollapsibleGroup` renders default-closed** (Open Question 5's resolution). Task 4.3.1d's test asserts `VcsWidgetCheckList` starts collapsed *in isolation*, but Task 4.3.1c's own doc comment states that once wrapped in `CollapsibleGroup`, the standalone `defaultExpanded` prop "becomes a documentation-only no-op" — the group's `defaultValue`/`value` is what actually drives initial state once composed. No task specifies what `defaultValue` `VcsWidget.tsx` passes to `<CollapsibleGroup>`, and no test renders the full `VcsWidget` in full mode with checks/reviews/comments populated to assert all three sections start closed. This is the one gap that risks shipping the opposite of OQ5's stated resolution without any test catching it.

   **CLOSED**: added Task 4.6.2d — `VcsWidget_should_RenderAllDisclosureSectionsCollapsed_When_FullModeWithChecksReviewsAndCommentsPresent`
   in `web-app/src/components/shared/VcsWidget.test.tsx`, placed right after Task 4.6.2b (the
   task that actually assembles the real `CollapsibleGroup` composition), rendering with all
   three sections populated and asserting every section's `aria-expanded` is `"false"` on
   initial render.

**Not a gap** — Scope-7 (ahead/behind-vs-base display): `VCSStatus.AheadBy`/`BehindBy` (`session/vc/types.go:84-85`) are already computed for live sessions by the pre-existing `git_provider.go:92-93` (`git rev-list --count`), already flow through `fromSessionVcs` (`adapters.ts:89-90`), and are already rendered in full mode by `VcsWidgetHeader.tsx:59-62` with existing test coverage (`VcsWidgetHeader.test.tsx`, both a populated case and a `0/0` case). This requirement predates the redesign and needed no new plan.md work — confirmed by `grep -rn "AheadBehind|ahead_by" plan.md` returning only one prose mention, not an omission.

## Coverage Targets

- Unit test coverage: ≥80% (line) for new/changed Go and TS code in this feature.
- All public service methods touched by this feature (`ListShippedCommits`, `DiffStatBetween`, `Instance.GetBaseCommitSHA`, `GetVCSStatus`, `GetPRInfoCtx`, `Instance.UpdatePRStatus`): happy path + error paths.
- All external integrations (GitHub API via `gh`/HTTP, go-git): unit mocked/fixture-based (`github/client_pr_by_number_test.go`'s response-fixture pattern; `session/git/ops_test.go`'s real-repo fixture pattern) + at least one integration-level test (`server/services/workspace_service_test.go`'s fixture-repo-through-provider-through-cache test, Task 5.1.2c).
