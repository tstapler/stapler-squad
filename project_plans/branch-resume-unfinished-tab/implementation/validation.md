# Validation Plan: branch-resume-unfinished-tab

**Date**: 2026-05-31

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: ListLocalBranches returns branches ahead of main | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_returnBranchesAheadOfMain_When_repoHasLocalCommits` | Integration (real tmp repo) | Repo with 2 branches, 1 ahead by 3 commits; expect both in result with correct CommitsAhead |
| REQ-1: ListLocalBranches excludes already-checked-out branches | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_excludeCheckedOutBranch_When_worktreeChecksItOut` | Integration (real tmp repo) | Add linked worktree on branch-x; ListLocalBranches should not include branch-x |
| REQ-1: ListLocalBranches caps at max param | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_capResults_When_maxExceeded` | Integration (real tmp repo) | Repo with 60 branches ahead; call with max=10; expect len==10 |
| REQ-1: ListLocalBranches returns empty when no branches ahead | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_returnEmpty_When_allBranchesAtMain` | Integration (real tmp repo) | All branches merged; expect empty slice |
| REQ-1: ListLocalBranches on non-repo path returns error | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_returnError_When_pathIsNotGitRepo` | Unit | Pass non-existent path; expect non-nil error |
| REQ-1: GoGitVCSReader implements ListLocalBranches | `session/unfinished/vcsreader_test.go` | `TestVCSReaderContractGoGit_ListLocalBranches` | Integration | Contract suite covers GoGitVCSReader |
| REQ-1: GitVCSReader implements ListLocalBranches | `session/unfinished/vcsreader_test.go` | `TestVCSReaderContractGit_ListLocalBranches` | Integration | Contract suite covers GitVCSReader |
| REQ-1: JJVCSReader ListLocalBranches returns nil,nil | `session/unfinished/vcsreader_test.go` | `JJVCSReader_ListLocalBranches_should_returnNilNil_Always` | Unit | JJVCSReader stub; expect nil result and nil error |
| REQ-1: BranchInfo carries correct fields | `session/unfinished/vcsreader_test.go` | `ListLocalBranches_should_populateAllBranchInfoFields_When_branchHasCommits` | Integration (real tmp repo) | Verify BranchName, CommitsAhead, RecentMessages (≤5), LastCommitTime all set |
| REQ-2: scanRepoBranches returns only ahead-of-main branches | `session/unfinished/scanner_test.go` | `scanRepoBranches_should_returnOnlyAheadBranches_When_repoHasMixedBranches` | Unit (fake VCSReader) | Fake reader with 2 branches: 1 ahead, 1 even; expect 1 BranchScanResult |
| REQ-2: scanRepoBranches skips branches checked out in worktrees | `session/unfinished/scanner_test.go` | `scanRepoBranches_should_skipCheckedOutBranches_When_worktreeExistsForBranch` | Unit (fake VCSReader) | ListWorktrees returns branch-x; ListLocalBranches also returns branch-x; expect exclusion |
| REQ-2: scanRepoBranches uses branchWorkers not resultStore | `session/unfinished/scanner_test.go` | `scanRepoBranches_should_notReadResultStore_When_determiningSkip` | Unit (fake VCSReader) | Pre-seed resultStore with a branch; ensure it still appears in branch scan (C-2 fix) |
| REQ-2: branchWorker processes branchScanQueue tasks | `session/unfinished/scanner_test.go` | `branchWorker_should_processBranchScanQueue_When_taskEnqueued` | Unit | Enqueue a branch task; confirm branchWorker processes and stores result |
| REQ-2: Refresh button trigger wires into branchScanQueue | `session/unfinished/scanner_test.go` | `TriggerScan_should_enqueueBranchScan_When_called` | Unit | Call TriggerScan(); verify branchScanQueue receives a task (C-4 fix) |
| REQ-2: Separate branchCircuitBreaker for branch timeouts | `session/unfinished/scanner_test.go` | `branchCircuitBreaker_should_backoffAfterThreeTimeouts_When_branchScanTimesOut` | Unit | Call recordBranchTimeout three times; expect shouldScanBranches returns false |
| REQ-2: Branch circuit breaker is independent of worktree breaker | `session/unfinished/scanner_test.go` | `branchCircuitBreaker_should_notAffectWorktreeBreaker_When_branchTimesOut` | Unit | Trigger branch backoff; worktree shouldScan must still return true |
| REQ-2: publishBranchResults emits EventDormantBranchUpdated | `session/unfinished/scanner_test.go` | `publishBranchResults_should_emitUpdatedEvent_When_newBranchFound` | Unit | Subscribe to event bus; publish branch result; assert EventDormantBranchUpdated received |
| REQ-2: publishBranchResults emits EventDormantBranchRemoved | `session/unfinished/scanner_test.go` | `publishBranchResults_should_emitRemovedEvent_When_branchNoLongerAhead` | Unit | Store branch in branchResultStore; run scan that finds it gone; assert EventDormantBranchRemoved |
| REQ-2: publishBranchResults suppresses dismissed branches | `session/unfinished/scanner_test.go` | `publishBranchResults_should_skipDismissed_When_branchIsDismissed` | Unit | Dismiss a branch in stateStore; run scan; assert no event emitted |
| REQ-2: branchTickInterval defaults to 5 min | `session/unfinished/scanner_test.go` | `Scanner_should_haveFiveMinuteBranchTickInterval_When_constructed` | Unit | NewScanner(); assert branchTickInterval == 5*time.Minute |
| REQ-3: BranchInfo struct matches proto DormantBranch fields | `server/services/unfinished_work_test.go` | `dormantBranchToProto_should_mapAllFields_When_BranchInfoIsPopulated` | Unit | Build BranchInfo with all fields; call helper; assert proto fields match |
| REQ-3: ListUnfinishedWork populates DormantBranches | `server/services/unfinished_work_test.go` | `ListUnfinishedWork_should_includeDormantBranches_When_scannerHasBranchResults` | Integration | Inject fake scanner with branches; call ListUnfinishedWork; assert dormant_branches len > 0 |
| REQ-3: WatchUnfinishedWork streams branch_updated events | `server/services/unfinished_work_test.go` | `WatchUnfinishedWork_should_streamBranchUpdated_When_scannerEmitsBranchEvent` | Integration | Start WatchUnfinishedWork; publish EventDormantBranchUpdated; assert branch_updated payload received |
| REQ-3: WatchUnfinishedWork streams branch_removed events | `server/services/unfinished_work_test.go` | `WatchUnfinishedWork_should_streamBranchRemoved_When_scannerEmitsBranchRemovedEvent` | Integration | Publish EventDormantBranchRemoved; assert branch_removed payload with matching key |
| REQ-3: DismissDormantBranch RPC happy path | `server/services/unfinished_work_test.go` | `DismissDormantBranch_should_dismissBranchAndEmitRemovedEvent_When_validRequest` | Integration | Call DismissDormantBranch; assert stateStore records dismiss; bus gets Removed event |
| REQ-3: DismissDormantBranch missing repoPath returns InvalidArgument | `server/services/unfinished_work_test.go` | `DismissDormantBranch_should_returnInvalidArgument_When_repoPathEmpty` | Unit | Call with empty repoPath; assert connect.CodeInvalidArgument |
| REQ-3: DismissDormantBranch stateStore error returns Internal | `server/services/unfinished_work_test.go` | `DismissDormantBranch_should_returnInternal_When_stateStoreFails` | Unit | Inject failing stateStore; assert connect.CodeInternal |
| REQ-4: useUnfinishedWork exposes dormantBranches sorted by lastCommitTime | `web-app/src/lib/hooks/useUnfinishedWork.test.ts` | `useUnfinishedWork_should_sortDormantBranchesByLastCommitTimeDesc_When_branchesReceived` | Unit (Jest) | Simulate branchUpdated events with different timestamps; assert sorted desc |
| REQ-4: useUnfinishedWork handles branchUpdated event | `web-app/src/lib/hooks/useUnfinishedWork.test.ts` | `useUnfinishedWork_should_addBranch_When_branchUpdatedEventReceived` | Unit (Jest) | Mock stream emitting branch_updated; assert dormantBranches contains the new entry |
| REQ-4: useUnfinishedWork handles branchRemoved event | `web-app/src/lib/hooks/useUnfinishedWork.test.ts` | `useUnfinishedWork_should_removeBranch_When_branchRemovedEventReceived` | Unit (Jest) | Seed branch in state; emit branch_removed event with matching key; assert removed from dormantBranches |
| REQ-4: useUnfinishedWork key format is repoPath+pipe+branchName | `web-app/src/lib/hooks/useUnfinishedWork.test.ts` | `useUnfinishedWork_should_useRepoPathPipeBranchNameAsKey_When_deduplicatingBranches` | Unit (Jest) | Send two updates for same key; assert map has exactly 1 entry |
| REQ-5: DormantBranchCard renders branch name | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_renderBranchName_When_mounted` | Unit (Jest/RTL) | Render with branchName="feat/login"; assert visible text |
| REQ-5: DormantBranchCard renders repo name | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_renderRepoName_When_mounted` | Unit (Jest/RTL) | Assert repoName displayed |
| REQ-5: DormantBranchCard renders commitsAhead chip | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_renderAheadChip_When_commitsAheadGreaterThanZero` | Unit (Jest/RTL) | commitsAhead=3; assert "↑3" chip visible |
| REQ-5: DormantBranchCard renders last commit date | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_renderLastCommitDate_When_lastCommitTimeProvided` | Unit (Jest/RTL) | Provide lastCommitTime; assert formatted date visible |
| REQ-5: DormantBranchCard expands to show commit messages | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_showCommitMessages_When_expanded` | Unit (Jest/RTL) | Click card; assert recentMessages list rendered |
| REQ-5: DormantBranchCard collapses commit list on second click | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_hideCommitMessages_When_collapsed` | Unit (Jest/RTL) | Click twice; assert list hidden |
| REQ-5: DormantBranchCard shows no more than 5 commit messages | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_showAtMostFiveCommitMessages_When_expanded` | Unit (Jest/RTL) | Provide 7 messages; expand; assert list length == 5 |
| REQ-5: DormantBranchCard Resume button is present | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_renderResumeButton_When_mounted` | Unit (Jest/RTL) | Assert button with "Resume" label visible |
| REQ-5: DormantBranchCard Resume button calls onResume with correct args | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_callOnResume_When_ResumeButtonClicked` | Unit (Jest/RTL) | Click Resume; assert onResume called with repoPath and branchName |
| REQ-5: DormantBranchCard Resume opens omnibar pre-filled | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_openOmnibarPrefilledWithRepoBranch_When_ResumeClicked` | Unit (Jest/RTL) | Mock router; click Resume; assert navigation to `/?path=repoPath@branchName` (C-6: uses routes.newSessionFromWorktree) |
| REQ-6: DormantBranchesSection renders collapsible header | `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | `DormantBranchesSection_should_renderCollapsibleHeader_When_mounted` | Unit (Jest/RTL) | Assert "Dormant Branches" heading with aria-expanded |
| REQ-6: DormantBranchesSection collapses by default when count > 5 | `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | `DormantBranchesSection_should_beCollapsedByDefault_When_moreThanFiveBranches` | Unit (Jest/RTL) | Pass 6 branches; assert aria-expanded=="false" |
| REQ-6: DormantBranchesSection expands on click | `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | `DormantBranchesSection_should_expandOnClick_When_collapsed` | Unit (Jest/RTL) | Click header; assert cards visible |
| REQ-6: DormantBranchesSection groups by repoName | `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | `DormantBranchesSection_should_groupBranchesByRepoName_When_branchesFromMultipleRepos` | Unit (Jest/RTL) | 3 branches from 2 repos; assert 2 repo group headings |
| REQ-6: DormantBranchesSection shows empty state when no branches | `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | `DormantBranchesSection_should_showEmptyState_When_noBranches` | Unit (Jest/RTL) | Pass empty array; assert "No dormant branches" text |
| REQ-6: UnfinishedTab renders DormantBranchesSection below active worktrees | `web-app/src/app/unfinished/UnfinishedTab.test.tsx` | `UnfinishedTab_should_renderDormantBranchesSectionBelowWorktrees_When_dormantBranchesExist` | Unit (Jest/RTL) | Mock hook returning worktrees and dormantBranches; assert section present and positioned after worktree groups |
| REQ-6: UnfinishedTab shows no DormantBranchesSection when no branches | `web-app/src/app/unfinished/UnfinishedTab.test.tsx` | `UnfinishedTab_should_notRenderDormantBranchesSection_When_noDormantBranches` | Unit (Jest/RTL) | Mock hook returning empty dormantBranches; assert section absent |
| REQ-E2E: Dormant branches visible without running git commands | `tests/e2e/dormant-branches.spec.ts` | `dormant-branches > shows dormant branch card for local branch ahead of main` | E2E (Playwright) | Create real repo with branch ahead by 2 commits; pin repo via API; navigate to /unfinished; assert DormantBranchCard with branch name visible |
| REQ-E2E: Dormant branch card shows commits ahead chip | `tests/e2e/dormant-branches.spec.ts` | `dormant-branches > shows commits-ahead chip matching actual commit count` | E2E (Playwright) | Same setup; assert "↑2" chip visible in card |
| REQ-E2E: Resume button opens omnibar pre-filled | `tests/e2e/dormant-branches.spec.ts` | `dormant-branches > Resume button navigates to omnibar with repoPath@branchName prefill` | E2E (Playwright) | Click Resume; assert URL contains `path=<repoPath>@<branchName>` param |
| REQ-E2E: Dormant section below active worktrees | `tests/e2e/dormant-branches.spec.ts` | `dormant-branches > Dormant Branches section renders below active worktrees section` | E2E (Playwright) | Page has both sections; assert DOM order: active worktrees precede dormant branches |
| REQ-E2E: Zero branches after checking out | `tests/e2e/dormant-branches.spec.ts` | `dormant-branches > branch disappears from Dormant section when checked out in a worktree` | E2E (Playwright) | Add worktree for branch; trigger rescan; assert branch no longer in Dormant section |

## Adversarial / Pitfall Guards

| Risk | Test File | Test Name | Type | Scenario |
|------|-----------|-----------|------|----------|
| C-2: Cross-contamination — resultStore skip must not silence branches | `session/unfinished/scanner_test.go` | `scanRepoBranches_should_notUseResultStore_When_filteringCheckedOutBranches` | Unit | Worktree scanner has a result for branch-x; branch scan should still skip based on ListWorktrees output only |
| C-4: Refresh does not skip branch scan | `session/unfinished/scanner_test.go` | `TriggerScan_should_enqueueBranchTask_When_refreshTriggered` | Unit | TriggerScan(); assert branchScanQueue gets a task alongside normal scanQueue |
| C-6: Resume uses existing route helper | `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | `DormantBranchCard_should_useNewSessionFromWorktreeRoute_When_resumeClicked` | Unit (Jest/RTL) | Spy on routes.newSessionFromWorktree; click Resume; assert spy called with correct args |

## Test Stack

- **Unit (Go)**: `testing` + `github.com/stretchr/testify` — same package (`package unfinished`) for white-box tests, `package unfinished_test` for black-box contract tests
- **Unit (TS)**: Jest + React Testing Library — mirrors existing pattern in `web-app/src/components/backlog/*.test.tsx`
- **Integration (Go)**: Go test with real tmp git repo — same helper pattern as `vcsreader_bench_test.go` (`initRepo`, `addCommit`, `filepath.EvalSymlinks` for macOS symlink compat)
- **E2E**: Playwright in `tests/e2e/dormant-branches.spec.ts` — mirrors `unfinished-work.spec.ts` structure with `test.beforeAll` creating a real git repo via `execSync`, pinning via API, and using `data-testid` / ARIA locators only

## File Locations

| Test File | New or Extend Existing |
|-----------|----------------------|
| `session/unfinished/vcsreader_test.go` | Extend existing — add `ListLocalBranches` subtests to `testVCSReaderContract` |
| `session/unfinished/scanner_test.go` | Extend existing — add `scanRepoBranches`, branch circuit breaker, `publishBranchResults` tests |
| `server/services/unfinished_work_test.go` | Extend existing — add `DismissDormantBranch`, `ListUnfinishedWork` branch population, `WatchUnfinishedWork` branch event tests |
| `web-app/src/lib/hooks/useUnfinishedWork.test.ts` | New — hook does not yet have a test file |
| `web-app/src/components/unfinished/DormantBranchCard.test.tsx` | New — component does not yet exist |
| `web-app/src/components/unfinished/DormantBranchesSection.test.tsx` | New — component does not yet exist |
| `web-app/src/app/unfinished/UnfinishedTab.test.tsx` | New — tab does not yet have a test file |
| `tests/e2e/dormant-branches.spec.ts` | New — feature-specific spec |

## Coverage Targets

- Unit test coverage: ≥80% (line) on new Go packages/files (`session/unfinished/vcsreader.go` extension, `scanner.go` branch paths)
- All public VCSReader methods: happy path + error paths (GoGit + Git readers)
- All new RPC handlers: happy path + at least 1 error path (`DismissDormantBranch`)
- All new React components: render + interaction (expand/collapse, Resume click)
- `useUnfinishedWork` hook: branchUpdated, branchRemoved, sort order

## Test Case Count Summary

| Type | Count |
|------|-------|
| Go unit (package-internal, fake VCSReader) | 12 |
| Go integration (real tmp git repo) | 9 |
| Go service layer (extend unfinished_work_test.go) | 7 |
| Jest unit (hooks) | 4 |
| Jest unit (React components) | 16 |
| Playwright E2E | 5 |
| **Total** | **53** |

## Requirements Coverage

| Requirement | Tests Covering It | Covered? |
|-------------|-------------------|----------|
| Scan dormant local branches (ahead of main, no active session) | vcsreader ListLocalBranches (×7) + scanner scanRepoBranches (×3) | Yes |
| Rich cards: branch name, commits ahead, ≤5 messages, last commit date | DormantBranchCard render tests (×7) | Yes |
| Resume button opens omnibar pre-filled with repoPath@branchName | DormantBranchCard Resume tests (×2) + E2E (×1) | Yes |
| Separate collapsible Dormant Branches section below active worktrees | DormantBranchesSection (×5) + UnfinishedTab (×2) + E2E (×1) | Yes |
| Branch circuit breaker separate from worktree breaker | Scanner branch breaker tests (×2) | Yes |
| DismissDormantBranch RPC | Service layer tests (×3) | Yes |
| Real-time streaming (branch_updated / branch_removed events) | Service streaming tests (×2) + useUnfinishedWork hook tests (×3) | Yes |
| E2E: zero git commands needed by user | E2E dormant-branches.spec.ts (×5) | Yes |

**Requirements covered**: 8 / 8 (100%)
