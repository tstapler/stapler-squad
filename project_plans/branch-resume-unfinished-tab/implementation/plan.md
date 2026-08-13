# Implementation Plan: branch-resume-unfinished-tab

**Feature**: Surface dormant local branches in the Unfinished Work tab with one-click Resume
**Date**: 2026-05-31
**Status**: Ready for implementation
**ADRs**: None — all changes use established in-repo patterns

---

## Dependency Visualization

```
Phase 1: VCSReader interface
    └── Phase 2: Scanner extension (BranchScanResult + branchResultStore)
            └── Phase 3: Proto (DormantBranch message + UnfinishedWorkEvent oneof)
                    └── Phase 4: Service layer (GetAllBranchResults + WatchUnfinishedWork extension)
                            └── Phase 5: Frontend hook (useUnfinishedWork branchMap)
                                    └── Phase 6: Frontend card (DormantBranchCard + DormantBranchesSection)
                                            └── Phase 7: Tests (Go unit + Jest + e2e)
                                                    └── Phase 8: Post-implementation review
                                                            ├── Epic 8.1: Go + TS idioms pass
                                                            └── Epic 8.2: Architecture + refactor candidates
                                                                    └── 🔁 REFACTOR → back to Phase 5 if issues found
                                                                        ✅ PASS → proceed to /sdd:7-ship
```

---

## Phase 1: Backend — VCSReader Interface Extension

### Epic 1.1: Add ListLocalBranches to VCSReader

#### Story 1.1.1: Define BranchInfo type and extend VCSReader interface

##### Task 1.1.1a: Add BranchInfo struct and ListLocalBranches to VCSReader (~3 min)
- **File**: `session/unfinished/vcsreader.go`
- Add `BranchInfo` struct: `Name string`, `CommitsAhead int`, `LastCommitTime time.Time`, `RecentMessages []string`
- Add `ListLocalBranches(repoPath, defaultBranch string, max int) ([]BranchInfo, error)` to `VCSReader` interface
- `max` caps the scan at 50 branches per repo (guards against wide repos)

##### Task 1.1.1b: Implement ListLocalBranches on GoGitVCSReader (~5 min)
- **File**: `session/unfinished/gogit_vcs_reader.go`
- Use `repo.Branches()` iterator to enumerate local refs under `refs/heads/`
- For each branch, compute `AheadBehind` against resolved default branch using existing `countCommitsTo`/`findMergeBase` helpers
- Skip branches with 0 commits ahead (already merged)
- Collect up to 5 recent commit messages via existing `CommitMessages` logic
- Read last commit timestamp from tip commit object
- Cap total branches processed at `max` (50) — stop iterator early once limit hit

##### Task 1.1.1c: Implement ListLocalBranches on GitVCSReader (~3 min)
- **File**: `session/unfinished/git_vcs_reader.go`
- Run `git for-each-ref refs/heads/ --format=%(refname:short) --sort=-committerdate` (3s timeout)
- For each branch (capped at `max`): call `AheadBehind` and `CommitMessages` (reuse existing methods)
- Return `[]BranchInfo`

##### Task 1.1.1d: Add stub to JJVCSReader (~2 min)
- **File**: `session/unfinished/jj_vcs_reader.go`
- Add `ListLocalBranches` returning `nil, nil` (jj has no local-branch concept equivalent; graceful no-op)

---

## Phase 2: Backend — Scanner Extension

### Epic 2.1: BranchScanResult type and separate store

#### Story 2.1.1: Define BranchScanResult and add branchResultStore to Scanner

##### Task 2.1.1a: Define BranchScanResult struct (~2 min)
- **File**: `session/unfinished/scanner.go`
- Add `BranchScanResult` struct: `RepoPath`, `BranchName`, `RepoName`, `CommitsAhead int`, `LastCommitTime time.Time`, `RecentMessages []string`, `ScanTime time.Time`, `DefaultBranch string`
- Add `branchResultStore sync.Map` field to `Scanner` (key = `repoPath+"|"+branchName`)
- Add `branchTickInterval time.Duration` field to `Scanner` (default 5 minutes, not 30s)

##### Task 2.1.1b: Add branchScanQueue and branch worker to Scanner.Start (~3 min)
- **File**: `session/unfinished/scanner.go`
- Add `branchScanQueue chan scanTask` field (buffer 20 — smaller than worktree queue)
- In `NewScannerWithReader`: initialize `branchScanQueue`, set `branchTickInterval = 5 * time.Minute`
- In `Start`: launch 2 branch worker goroutines (`branchWorker`) and update `coordinator` to also tick the branch queue on the slower cadence
- Add `SetBranchTickInterval(d time.Duration)` for test override

### Epic 2.2: Branch scan logic

#### Story 2.2.1: scanRepoBranches and circuit-breaker separation

##### Task 2.2.1a: Implement scanRepoBranches (~5 min)
- **File**: `session/unfinished/scanner.go`
- New method `scanRepoBranches(repoPath string) []BranchScanResult`
- Call `s.reader.ListLocalBranches(repoPath, defaultBranch, 50)`
- For each `BranchInfo`: create `BranchScanResult`; skip branch if its name equals the current worktree's checked-out branch (already shown in worktree section)
- Cross-reference `s.resultStore` by key `repoPath+"|"+branchName` — if a result already exists there (branch is a checked-out worktree), skip it
- Return only branches with `CommitsAhead > 0`

##### Task 2.2.1b: Add separate branch timeout counter to circuit breaker (~3 min)
- **File**: `session/unfinished/scanner.go`
- Extend `circuitBreaker` struct: add `branchTimeouts int` and `branchBackoffUntil time.Time`
- Add `recordBranchTimeout(repoPath string)` and `shouldScanBranches(repoPath string)` methods
- These track timeouts separately so branch scan timeouts never trip the worktree circuit breaker
- Cap: 3 branch timeouts → 10-minute backoff (longer than worktree backoff, since branch scans are slower)

##### Task 2.2.1c: Implement publishBranchResults and event emission (~3 min)
- **File**: `session/unfinished/scanner.go`
- New method `publishBranchResults(results []BranchScanResult)`
- For each result: check dismiss state via `s.stateStore.IsDismissed(repoPath, branchName)`
- Compare against stored `BranchScanResult` — only publish if `CommitsAhead` changed or new
- Store in `branchResultStore`; emit `EventDormantBranchUpdated` event
- After batch: detect keys in `branchResultStore` that no longer appear in new results → emit `EventDormantBranchRemoved` and delete from store
- Add `GetAllBranchResults() []BranchScanResult` and `GetBranchResultByKey(repoPath, branch string) (BranchScanResult, bool)` accessors

##### Task 2.2.1d: Wire branchWorker into coordinator (~2 min)
- **File**: `session/unfinished/scanner.go`
- Add `branchWorker(ctx context.Context)` goroutine consuming `branchScanQueue`
- Update `enqueueAll` to also send to `branchScanQueue` (guarded by `shouldScanBranches`)
- Update `coordinator` to tick `branchScanQueue` on `branchTickInterval` (separate ticker from worktree ticker)

### Epic 2.3: Event types for dormant branches

#### Story 2.3.1: Add EventDormantBranch* constants

##### Task 2.3.1a: Add event constants and constructors (~2 min)
- **File**: `session/unfinished/events.go`
- Add `EventDormantBranchUpdated pkgevents.EventType = "unfinished.dormant_branch_updated"`
- Add `EventDormantBranchRemoved pkgevents.EventType = "unfinished.dormant_branch_removed"`
- Add `newDormantBranchUpdatedEvent(r BranchScanResult) *pkgevents.Event` (Context = `repoPath+"|"+branchName`)
- Add `newDormantBranchRemovedEvent(repoPath, branch string) *pkgevents.Event`

---

## Phase 3: Proto — DormantBranch Message

### Epic 3.1: Extend unfinished.proto and types.proto

#### Story 3.1.1: Add DormantBranch proto message and extend UnfinishedWorkEvent

##### Task 3.1.1a: Add DormantBranch message to types.proto (~3 min)
- **File**: `proto/session/v1/types.proto`
- Add after `UnfinishedWorkConfig`:
  ```protobuf
  // DormantBranch represents a local branch with commits ahead of the default
  // branch that is NOT checked out in any active session worktree.
  message DormantBranch {
    string repo_path        = 1;
    string branch_name      = 2;
    string repo_name        = 3;
    int32  commits_ahead    = 4;
    string default_branch   = 5;
    repeated string recent_commit_messages = 6; // up to 5
    google.protobuf.Timestamp last_commit_time = 7;
    google.protobuf.Timestamp scan_time        = 8;
    bool is_dismissed = 9;
  }
  ```

##### Task 3.1.1b: Extend UnfinishedWorkEvent oneof and add list response field (~3 min)
- **File**: `proto/session/v1/unfinished.proto`
- Extend `UnfinishedWorkEvent.oneof payload` with:
  ```protobuf
  DormantBranch branch_updated = 4;
  DormantBranch branch_removed = 5;
  ```
- Extend `ListUnfinishedWorkResponse`:
  ```protobuf
  repeated DormantBranch dormant_branches = 3;
  ```
- Run `make generate-proto` to regenerate Go and TypeScript bindings

---

## Phase 4: Service Layer

### Epic 4.1: Extend UnfinishedWorkService

#### Story 4.1.1: Plumb branch results through existing RPCs

##### Task 4.1.1a: Add dormantBranchToProto helper (~2 min)
- **File**: `server/services/unfinished_work_service.go`
- Add `dormantBranchToProto(r unfinished.BranchScanResult) *sessionv1.DormantBranch` conversion function
- Maps all fields from `BranchScanResult` to `DormantBranch` proto

##### Task 4.1.1b: Extend ListUnfinishedWork to include dormant branches (~2 min)
- **File**: `server/services/unfinished_work_service.go`
- In `ListUnfinishedWork`: call `s.scanner.GetAllBranchResults()` and convert to `[]*sessionv1.DormantBranch`
- Populate `ListUnfinishedWorkResponse.DormantBranches`

##### Task 4.1.1c: Extend WatchUnfinishedWork initial snapshot and event forwarding (~3 min)
- **File**: `server/services/unfinished_work_service.go`
- In `WatchUnfinishedWork` initial snapshot: also send `branch_updated` events for all branch results
- In `convertUnfinishedEvent`: add cases for `EventDormantBranchUpdated` and `EventDormantBranchRemoved`
  - `EventDormantBranchUpdated`: look up via `s.scanner.GetBranchResultByKey`, convert, return `branch_updated` payload
  - `EventDormantBranchRemoved`: parse Context, return `branch_removed` payload with minimal proto

---

## Phase 5: Frontend Hook

### Epic 5.1: Extend useUnfinishedWork with branchMap

#### Story 5.1.1: Add DormantBranch state to the hook

##### Task 5.1.1a: Extend useUnfinishedWork hook (~4 min)
- **File**: `web-app/src/lib/hooks/useUnfinishedWork.ts`
- Add `branchMap: Map<string, DormantBranch>` state (key = `repoPath|branchName`)
- Import `DormantBranch` from generated `types_pb`
- In the stream `for await` loop, add handlers for `event.payload.case === "branchUpdated"` and `"branchRemoved"`:
  - `branchUpdated`: upsert into `branchMap`
  - `branchRemoved`: delete from `branchMap`
- Derive `dormantBranches: DormantBranch[]` sorted by `lastCommitTime` descending (secondary sort: `repoPath+branchName`)
- Extend `UseUnfinishedWorkReturn` type to add `dormantBranches: DormantBranch[]`

---

## Phase 6: Frontend Card and Tab Integration

### Epic 6.1: DormantBranchCard component

#### Story 6.1.1: Build DormantBranchCard

##### Task 6.1.1a: Create DormantBranchCard component (~5 min)
- **File**: `web-app/src/components/unfinished/DormantBranchCard.tsx` (new)
- Props: `branch: DormantBranch`, `isExpanded: boolean`, `onToggleExpand: () => void`
- Collapsed row: branch name, repo name, commits-ahead chip (`↑N`), last commit date
- Expanded section: `recent_commit_messages` list (same `commitList`/`commitItem` styles as `UnfinishedItemDetail`)
- Resume button: calls `router.push(routes.sessions())` with omnibar pre-filled via URL query `?omnibar=<repoPath>@<branchName>` — uses the existing `PathWithBranchDetector` pattern
- Keyboard handling: Enter/Space toggles expand; matches `UnfinishedItem` behavior
- `data-testid="dormant-branch-card"`

##### Task 6.1.1b: Create DormantBranchCard.css.ts styles (~3 min)
- **File**: `web-app/src/components/unfinished/DormantBranchCard.css.ts` (new)
- Use vanilla-extract `style()` with `vars` from theme contract
- Define: `card`, `cardExpanded`, `header`, `branchName`, `repoLabel`, `chipAhead`, `commitList`, `commitItem`, `resumeBtn`, `actions`
- Visually match existing `UnfinishedItem.css.ts` sizing and spacing; reuse `vars.color.actionPrimary` for the Resume button

### Epic 6.2: DormantBranchesSection collapsible

#### Story 6.2.1: Build the collapsible section component

##### Task 6.2.1a: Create DormantBranchesSection component (~3 min)
- **File**: `web-app/src/components/unfinished/DormantBranchesSection.tsx` (new)
- Props: `branches: DormantBranch[]`
- Collapsible section header "Dormant Branches (N)" — collapsed by default if N > 5, open otherwise
- Maps over `branches` grouped by `repoName` using the same grouping pattern as `UnfinishedTab`
- Renders `DormantBranchCard` for each branch; manages `expandedKey` local state
- `data-testid="dormant-branches-section"`

### Epic 6.3: Wire into UnfinishedTab

#### Story 6.3.1: Add DormantBranchesSection below active worktrees

##### Task 6.3.1a: Extend UnfinishedTab to render dormant branches (~3 min)
- **File**: `web-app/src/app/unfinished/UnfinishedTab.tsx`
- Destructure `dormantBranches` from `useUnfinishedWork()`
- Below the existing `<div className={styles.repoList}>`, render:
  ```tsx
  {dormantBranches.length > 0 && (
    <DormantBranchesSection branches={dormantBranches} />
  )}
  ```
- Update empty-state message to account for `worktrees.length === 0 && dormantBranches.length === 0`

### Epic 6.4: Resume button — omnibar pre-fill

#### Story 6.4.1: Verify omnibar pre-fill route and add helper

##### Task 6.4.1a: Confirm and document route for omnibar pre-fill (~2 min)
- **File**: `web-app/src/lib/routes.ts` (read-only audit)
- Verify `routes.sessions()` accepts a `?omnibar=` query param or that the sessions page reads it
- If the route helper does not support an omnibar param, add `routes.sessionsWithOmnibar(input: string): string` returning `/sessions?omnibar=<encoded>`
- The `PathWithBranchDetector` already handles `repoPath@branchName` input — no omnibar changes needed

---

## Phase 7: Tests

### Epic 7.1: Go unit tests

#### Story 7.1.1: VCSReader and Scanner branch tests

##### Task 7.1.1a: Unit tests for ListLocalBranches on GoGitVCSReader (~4 min)
- **File**: `session/unfinished/vcsreader_test.go` (extend existing)
- Test: `ListLocalBranches_shouldReturnBranchesAheadOfDefault` — use existing `initTestRepo` helper, create a local branch with 2 commits, verify `CommitsAhead = 2`
- Test: `ListLocalBranches_shouldSkipBranchesNotAhead` — branch at same SHA as default → not returned
- Test: `ListLocalBranches_shouldCapAt50` — inject 60 branch names, verify max 50 returned

##### Task 7.1.1b: Unit tests for scanRepoBranches and circuit breaker separation (~4 min)
- **File**: `session/unfinished/scanner_test.go` (extend existing)
- Test: `scanRepoBranches_shouldSkipCheckedOutWorktreeBranches` — seed `resultStore` with key matching branch, verify branch not in `scanRepoBranches` output
- Test: `recordBranchTimeout_shouldNotTripWorktreeBreaker` — call `recordBranchTimeout` 3× on a repo, then assert `shouldScan` still returns `true` (worktree breaker unaffected)
- Test: `branchCircuitBreaker_shouldBackoffAfterThreeBranchTimeouts` — call `recordBranchTimeout` 3×, assert `shouldScanBranches` returns `false`

##### Task 7.1.1c: Unit tests for publishBranchResults removal detection (~3 min)
- **File**: `session/unfinished/scanner_test.go` (extend existing)
- Test: `publishBranchResults_shouldEmitRemovedEventForStaleBranch` — seed `branchResultStore` with a key, call `publishBranchResults` with a result set that omits it, verify `EventDormantBranchRemoved` published on event bus

### Epic 7.2: Frontend Jest tests

#### Story 7.2.1: Hook and component tests

##### Task 7.2.1a: useUnfinishedWork hook tests for branch events (~3 min)
- **File**: `web-app/src/lib/hooks/useUnfinishedWork.test.ts` (new or extend)
- Test: hook handles `branchUpdated` event → `dormantBranches` array grows by 1
- Test: hook handles `branchRemoved` event → branch removed from `dormantBranches`
- Use existing mock pattern for ConnectRPC streams (AsyncIterator mock)

##### Task 7.2.1b: DormantBranchCard render and Resume button tests (~3 min)
- **File**: `web-app/src/components/unfinished/DormantBranchCard.test.tsx` (new)
- Test: renders branch name, commits-ahead chip, repo name
- Test: Resume button calls `router.push` with `?omnibar=repoPath@branchName`
- Test: expand/collapse on click toggles commit list visibility

### Epic 7.3: E2E test

#### Story 7.3.1: Playwright spec for dormant branch display

##### Task 7.3.1a: Create e2e spec for dormant branch flow (~5 min)
- **File**: `tests/e2e/dormant-branches.spec.ts` (new)
- `// @feature session:resume, unfinished:dormant-branches`
- `describe("dormant-branches")`:
  - `it("dormant-branches_should_showDormantSection_When_branchesExist")` — mock WatchUnfinishedWork stream to emit `branch_updated` event; assert `data-testid="dormant-branches-section"` visible
  - `it("dormant-branches_should_expandCard_When_clicked")` — click a `data-testid="dormant-branch-card"`, assert commit list appears
  - `it("dormant-branches_should_openOmnibar_When_resumeClicked")` — click Resume button; assert URL contains `?omnibar=`

---

## Phase 8: Post-Implementation Review

### Epic 8.1: Language idioms review

#### Story 8.1.1: Go idioms pass on all new/changed Go files

##### Task 8.1.1a: Go idiom review (~5 min)
- Run `sdd:6-verify` Layer 1 Go agent on the diff covering Phases 1–4 files
- Focus areas: error wrapping in `ListLocalBranches` implementations, goroutine lifecycle in `branchWorker`, context cancellation propagation, receiver name consistency, interface sizing on `VCSReader` extension
- **Files**: `session/unfinished/vcsreader.go`, `gogit_vcs_reader.go`, `git_vcs_reader.go`, `scanner.go`, `server/services/unfinished_work_service.go`
- Fix any MUST FIX findings inline; note SUGGEST findings as follow-up tasks

##### Task 8.1.1b: TypeScript/React idioms pass on all new/changed frontend files (~5 min)
- Run `sdd:6-verify` Layer 1 TS/React agent on the diff covering Phases 5–6 files
- Focus areas: `useUnfinishedWork` hook dependency arrays, `DormantBranchCard` memo/callback usage, discriminated union type safety for the `branchUpdated`/`branchRemoved` event payload, vanilla-extract `vars` usage (no hardcoded hex)
- **Files**: `useUnfinishedWork.ts`, `DormantBranchCard.tsx`, `DormantBranchCard.css.ts`, `DormantBranchesSection.tsx`, `UnfinishedTab.tsx`
- Fix any MUST FIX findings inline

### Epic 8.2: Architecture review

#### Story 8.2.1: Architecture & design quality pass

##### Task 8.2.1a: Architecture review against plan (~5 min)
- Run `sdd:6-verify` Layer 2 architecture agent on the full diff
- Specific checks for this feature:
  - Does `BranchScanResult` duplicate fields already on `ScanResult`? Could they share a common type?
  - Is `branchResultStore` appropriately decoupled from `resultStore`, or is there hidden coupling?
  - Does `DormantBranchCard` share enough logic with `UnfinishedItem` to warrant a shared base component, or are they correctly independent?
  - Is the `branchMap` in `useUnfinishedWork` correctly isolated from `worktreeMap` (no accidental cross-contamination)?
- Verdict: REFACTOR → back to Phase 5 if structural issues found; PASS → proceed to Phase 7 tests

##### Task 8.2.1b: Refactor candidates scan (~3 min)
- Run `sdd:6-verify` Layer 2 refactor agent on the diff
- Focus: any repeated key-building logic (`repoPath+"|"+branchName` appears in multiple places — candidate for a shared helper), any `scanRepoBranches` function doing more than one thing, any component prop interfaces that have grown beyond 5 fields
- Apply refactors if total estimated effort < 20 min; otherwise note as follow-up

---

## Safeguard Summary

| Safeguard | Where enforced |
|---|---|
| Cap at 50 branches per repo | `ListLocalBranches` `max` param; call site passes `50` |
| 5-minute branch scan cadence (not 30s) | `branchTickInterval = 5 * time.Minute` in `NewScannerWithReader` |
| Separate branch timeout counter | `branchTimeouts` / `branchBackoffUntil` in `circuitBreaker`; `recordBranchTimeout` / `shouldScanBranches` methods |
| No LCS diffstat for branches | `BranchScanResult` has no `LinesAdded`/`LinesRemoved` fields; `scanRepoBranches` never calls `DiffShortstat` |
| Skip checked-out worktree branches | `scanRepoBranches` cross-references `resultStore` before including a branch |
| Validate branch exists before Resume | Frontend reads `branch_name` from the streamed proto; if branch card is absent (removed event received), card is gone before user can click |

---

## File Change Index

| Phase | Files Changed |
|---|---|
| 1 | `session/unfinished/vcsreader.go`, `gogit_vcs_reader.go`, `git_vcs_reader.go`, `jj_vcs_reader.go` |
| 2 | `session/unfinished/scanner.go`, `events.go` |
| 3 | `proto/session/v1/types.proto`, `proto/session/v1/unfinished.proto` → `make generate-proto` |
| 4 | `server/services/unfinished_work_service.go` |
| 5 | `web-app/src/lib/hooks/useUnfinishedWork.ts` |
| 6 | `web-app/src/app/unfinished/UnfinishedTab.tsx`, `web-app/src/components/unfinished/DormantBranchCard.tsx` (new), `DormantBranchCard.css.ts` (new), `DormantBranchesSection.tsx` (new), `web-app/src/lib/routes.ts` (minor) |
| 7 | `session/unfinished/vcsreader_test.go`, `scanner_test.go`, `web-app/src/lib/hooks/useUnfinishedWork.test.ts` (new), `web-app/src/components/unfinished/DormantBranchCard.test.tsx` (new), `tests/e2e/dormant-branches.spec.ts` (new) |
| 8 | No new files — review pass only; fixes applied inline to Phase 1–6 files |
