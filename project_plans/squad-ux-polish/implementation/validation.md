# Validation Plan: Squad UX Polish

**Phase**: 4 — Validation
**Date**: 2026-04-17
**Input artifacts**: `requirements.md`, `project_plans/squad-ux-polish/implementation/plan.md`, `docs/tasks/squad-ux-polish.md`
**Output**: This file — test coverage specification before any implementation begins

---

## Requirements Traceability Matrix

| Req # | Requirement (Must Have) | Story | Test Type | Test Name(s) | Priority |
|-------|-------------------------|-------|-----------|--------------|----------|
| R1 | Prompt input at session creation time | S1 | Unit | `TestPromptStore_RecordUsage_Idempotent`, `TestCLAUDEmdInjection_WritesPromptFile` | P0 |
| R1 | Prompt input at session creation time | S1 | Integration | `TestCreateSession_WithInitialPrompt_CLAUDE_md_Written` | P0 |
| R1 | Prompt input at session creation time | S1 | UI | `SessionWizard_PromptTextarea_Visible`, `SessionWizard_PromptTextarea_WiredToRequest` | P0 |
| R2 | Prompt library / recents | S1 | Unit | `TestPromptStore_List_SortedByLastUsedDesc`, `TestPromptStore_Eviction_At500Entries` | P1 |
| R2 | Prompt library / recents | S1 | Integration | `TestListPromptHistory_ReturnsEntriesSortedByRecency` | P1 |
| R2 | Prompt library / recents | S1 | UI | `SessionWizard_RecentPromptsDropdown_PopulatesTextarea` | P1 |
| R3 | Batch / multi-session creation | S2 | Unit | `TestBatchCreateSessions_TitleDedup`, `TestBatchCreateSessions_MaxBatchEnforced`, `TestBatchCreateSessions_PartialFailureDoesNotCancelRest` | P0 |
| R3 | Batch / multi-session creation | S2 | Integration | `TestBatchCreateSessions_SameRepo_NoIndexLock`, `TestBatchCreateSessions_DifferentRepos_RunConcurrently` | P0 |
| R3 | Batch / multi-session creation | S2 | UI | `BatchTab_TaskCountValidation`, `BatchTab_PreviewUpdatesOnInput`, `BatchTab_ResultsListShowsPerItemStatus` | P0 |
| R4 | Review queue / merge flow | S3 | Unit | `TestRunOneShot_ParsesPRURL_LastLine`, `TestRunOneShot_ParsesPRURL_TrailingNewline`, `TestRunOneShot_TimeoutEnforced`, `TestCheckBranchDivergence_DivergesToTrue` | P0 |
| R4 | Review queue / merge flow | S3 | Integration | `TestRunOneShot_UpdatesSessionPRURL` | P0 |
| R4 | Review queue / merge flow | S3 | UI | `ReviewQueuePanel_CreatePRButton_AppearsForTaskComplete`, `ReviewQueuePanel_DivergenceWarningBadge`, `ReviewQueuePanel_Modal_SpinnerOnSubmit` | P0 |
| R5 | Project concept | S4 | Unit | `TestCreateProject_DuplicateNameReturnsAlreadyExists`, `TestDeleteProject_NullsSessionFK_DoesNotDeleteSessions`, `TestListProjects_AggregateCounts` | P1 |
| R5 | Project concept | S4 | Integration | `TestProjectMigration_AddsProjectsTableNoDropColumns`, `TestListSessions_FilteredByProjectID` | P1 |
| R5 | Project concept | S4 | UI | `SessionWizard_ProjectPicker_ShowsAllProjects`, `ProjectGrouping_UngroupedCatchall`, `ProjectPanel_RenameOptimisticUpdate` | P1 |
| R6 | IDE open integration | Deferred | — | — | Out of scope (v2) |

---

## Test Pyramid Summary

| Layer | Count | Packages / Directories |
|-------|-------|------------------------|
| Unit | 28 | `session/prompts/`, `server/services/`, `session/` |
| Integration | 8 | `server/services/` (SQLite ent + real filesystem) |
| UI component | 17 | `web-app/src/components/sessions/__tests__/`, `web-app/src/lib/grouping/` |
| **Total** | **53** | |

Ratio: ~53% unit / ~15% integration / ~32% UI. Aim to hold this shape; do not add integration tests for logic that is exercisable in unit tests.

---

## Risk Register and Test Coverage

| # | Risk | Severity | Mitigating Test(s) | Story |
|---|------|----------|--------------------|-------|
| R-01 | `git worktree add` concurrent stale `.git/index.lock` on batch failure | High | `TestBatchCreateSessions_SameRepo_NoIndexLock` — creates 5 sessions on the same repo path concurrently with a simulated lock delay; verifies no goroutine returns an index.lock error | S2 |
| R-02 | Partial batch failure corrupts result array or silently drops items | High | `TestBatchCreateSessions_PartialFailureDoesNotCancelRest` — injects failure at position 3 of a 5-session batch; asserts sessions 1–2 and 4–5 have `success=true`, index 3 has `success=false` with an error string, and the response `succeeded=4, failed=1` | S2 |
| R-03 | CLAUDE.md injection with `$VAR` shell metacharacters in prompt | Medium | `TestCLAUDEmdInjection_ShellMetacharactersWrittenLiterally` — sets `InitialPrompt` to `"run $MY_SECRET and $(whoami)"`, calls `start()`, reads `session-prompt.md` from disk, asserts the file contains the string verbatim without expansion | S1 |
| R-04 | Diverged branch at PR creation triggers wrong PR URL stored | Medium | `TestCheckBranchDivergence_DivergesToTrue` — creates a worktree on a branch 3 commits behind `origin/main`; asserts `checkBranchDivergence()` returns `true`; `TestRunOneShot_DivergenceFieldPopulatedInResponse` asserts the divergence flag is forwarded to the proto response | S3 |
| R-05 | `gh pr create` stdout URL parse: false positive or missing trailing newline | Low | `TestRunOneShot_ParsesPRURL_TrailingNewline` and `TestRunOneShot_ParsesPRURL_NoTrailingNewline` — mock subprocess stdout with `"https://github.com/owner/repo/pull/42\n"` and without; assert `pr_url == "https://github.com/owner/repo/pull/42"` in both cases; `TestRunOneShot_ParsesPRURL_RejectsNonPRURL` — output contains a non-PR GitHub URL such as `"https://github.com/owner/repo/issues/7"`, asserts `pr_url == ""` | S3 |

---

## Story 1: Prompt at Session Creation

### Unit Tests

**Package**: `session/prompts/`
**File**: `session/prompts/store_test.go`
**Runner**: `go test ./session/prompts/`

---

**T-S1-U-01** `TestPromptStore_Load_EmptyFile` (P0)
- Setup: PromptStore pointing at a temp dir with no `prompts.json`
- Assert: `Load()` returns empty slice, no error
- Validates: R1 baseline; "Load returns empty slice on missing file"

**T-S1-U-02** `TestPromptStore_RecordUsage_Idempotent` (P0)
- Call `RecordUsage("fix the tests")` twice
- Assert: `UsedCount == 2`, `LastUsed` updated after second call, only 1 entry in store
- Validates: R1 deduplification requirement

**T-S1-U-03** `TestPromptStore_Eviction_At500Entries` (P0 — boundary value)
- Populate store with 499 entries (unique text, ascending `LastUsed` timestamps spaced 1s apart)
- Record one more unique entry (newest)
- Assert: `len(entries) == 500`, oldest entry by `LastUsed` is no longer present
- Add entry 501: assert `len(entries) == 500`, second-oldest is evicted
- Validates: R2 ring-buffer cap at exactly 500

**T-S1-U-04** `TestPromptStore_List_SortedByLastUsedDesc` (P1)
- Insert 3 entries with varying `LastUsed` values in non-chronological order
- `List(10)` must return them sorted newest first
- Validates: R2 recency ordering

**T-S1-U-05** `TestPromptStore_List_HonorsLimit` (P1)
- Insert 5 entries; call `List(3)`
- Assert: only 3 returned (the 3 most recent)

**T-S1-U-06** `TestPromptStore_Delete_RemovesById` (P2)
- Insert 2 entries; delete by ID of first; load; assert 1 entry remains with correct ID

**T-S1-U-07** `TestPromptStore_ConcurrentRecordUsage` (P1 — concurrency)
- Launch 20 goroutines each calling `RecordUsage("concurrent text")`
- Assert: no panic, `UsedCount == 20` (or close, depending on atomicity guarantee), file is valid JSON
- Validates: ADR-001 "Concurrent Save calls serialize via file lock"

**T-S1-U-08** `TestPromptStore_AtomicWrite_NoCorruption` (P1)
- Simulate a partial-write by calling `Save()` while another goroutine is mid-save
- Assert: on a subsequent `Load()`, JSON is valid (no partial-write corruption)
- Implementation note: use `os.Rename` atomicity; this test verifies the file is never partially written

---

**Package**: `session/`
**File**: `session/instance_prompt_test.go`

**T-S1-U-09** `TestCLAUDEmdInjection_WritesPromptFile` (P0)
- Create an `Instance` with `InitialPrompt = "do X"`, using stub `TmuxManager` and stub `GitManager`
- Call `start()` with mocked worktree dir pointing at a real temp directory
- Assert: `<worktreePath>/.claude/session-prompt.md` exists with content "do X"
- Assert: `<worktreePath>/CLAUDE.md` contains the line `@.claude/session-prompt.md`
- Validates: R1 core injection requirement

**T-S1-U-10** `TestCLAUDEmdInjection_NoPrompt_NoFileCreated` (P0 — regression)
- Instance with `InitialPrompt = ""`
- Call `start()`, assert `.claude/session-prompt.md` does NOT exist
- Validates: backward compatibility for sessions without prompts

**T-S1-U-11** `TestCLAUDEmdInjection_ShellMetacharactersWrittenLiterally` (P0 — Risk R-03)
- `InitialPrompt = "run $MY_SECRET and $(whoami) and \`date\`"`
- After `start()`, read `session-prompt.md` from disk
- Assert exact file content matches the input string without expansion
- Validates: Risk R-03 (shell metacharacters must not be interpreted)

**T-S1-U-12** `TestCLAUDEmdInjection_Idempotent_NoDoubleAppend` (P1 — bug mitigation)
- Start session with `InitialPrompt`; CLAUDE.md gets the import line appended
- Simulate session restart: call the injection path again for same worktree
- Assert: CLAUDE.md contains `@.claude/session-prompt.md` exactly once
- Validates: S1 known issue "CLAUDE.md append idempotency"

**T-S1-U-13** `TestCLAUDEmdInjection_NoCLAUDEmd_CreatesFile` (P1 — bug mitigation)
- Use a worktree with no existing CLAUDE.md
- After injection, assert CLAUDE.md exists and contains only the import line
- Validates: S1 known issue "Worktree without CLAUDE.md"

**T-S1-U-14** `TestCLAUDEmdInjection_PromptAddedToGitignore` (P1)
- After `start()`, read `<worktreePath>/.gitignore`
- Assert: `.claude/session-prompt.md` appears in the file
- Validates: plan requirement that `session-prompt.md` is gitignored

**T-S1-U-15** `TestCLAUDEmdInjection_LargePrompt_NoTruncation` (P1 — input partitioning)
- `InitialPrompt` is a 15KB string (beyond old 255-byte send-keys limit and beyond a typical CLAUDE.md annotation)
- Assert: `session-prompt.md` contains the full 15KB without truncation
- Validates: ADR-001 rationale (CLAUDE.md injection has no size limit)

**T-S1-U-16** `TestCLAUDEmdInjection_EmptyPromptInput_Partitions` (P2 — input partitioning)
- Whitespace-only `InitialPrompt = "   \t\n"`: verify treated as empty (no file written)
- Validates: edge case from input space analysis

### Integration Tests

**Package**: `server/services/`
**File**: `server/services/prompt_integration_test.go`
**Setup**: uses `createTestStorage(t)` with real SQLite ent; real temp filesystem for worktrees

**T-S1-I-01** `TestCreateSession_WithInitialPrompt_RecordsInHistory` (P0)
- Call `CreateSession` RPC with `initial_prompt = "implement feature X"`
- Assert: `ListPromptHistory` returns one entry with `text == "implement feature X"` and `used_count == 1`
- Call `CreateSession` again with same `initial_prompt`
- Assert: `ListPromptHistory` returns one entry with `used_count == 2`
- Validates: R2 RecordUsage called on session creation

**T-S1-I-02** `TestListPromptHistory_ReturnsEntriesSortedByRecency` (P1)
- Create 3 sessions with different prompts at staggered times
- `ListPromptHistory(limit=10)` — assert ordering: newest used first
- Validates: R2 recency sort in the handler layer

**T-S1-I-03** `TestDeletePromptHistory_RemovesEntry` (P1)
- Create session with prompt; confirm entry in history; call `DeletePromptHistory(id)`; confirm entry gone
- Validates: R2 delete capability

### UI Component Tests

**Directory**: `web-app/src/components/sessions/__tests__/`
**File**: `SessionWizard.prompt.test.tsx`
**Framework**: Vitest + React Testing Library (RTL), with `jest.fn()` / `vi.fn()` for RPC mocks

**T-S1-UI-01** `SessionWizard_PromptTextarea_Visible` (P0)
- Render `SessionWizard` (in single-session mode)
- Assert: a `<textarea>` with accessible label matching "Initial prompt" (or equivalent aria-label) is present in the DOM
- Validates: R1 UI requirement "textarea visible on the form"

**T-S1-UI-02** `SessionWizard_PromptTextarea_WiredToRequest` (P0)
- Type "fix the login bug" into the prompt textarea
- Submit the form
- Assert: the mocked `createSession` RPC was called with `initial_prompt == "fix the login bug"`
- Validates: R1 field is wired to the request

**T-S1-UI-03** `SessionWizard_EmptyPrompt_OmittedFromRequest` (P0)
- Leave prompt textarea empty; submit
- Assert: `createSession` called with `initial_prompt == ""` OR field omitted (not a whitespace string)
- Validates: plan "Submitting with an empty InitialPrompt omits the field"

**T-S1-UI-04** `SessionWizard_RecentPromptsDropdown_PopulatesTextarea` (P1)
- Mock `ListPromptHistory` to return 2 entries
- Focus prompt textarea; assert dropdown appears; click first entry
- Assert: textarea value equals the text of the first entry
- Validates: R2 recent-prompts dropdown

**T-S1-UI-05** `SessionWizard_FilePickerReadsIntoTextarea` (P2)
- Simulate file selection via the "File" button with a `.txt` file containing "hello from file"
- Assert: textarea value is "hello from file"
- Validates: plan "File picker reads file content into textarea"

---

## Story 2: Batch Session Creation

### Unit Tests

**Package**: `server/services/`
**File**: `server/services/batch_session_test.go`

**T-S2-U-01** `TestBatchCreateSessions_MaxBatchEnforced` (P0 — boundary value)
- Call handler with 20 sessions → success
- Call handler with 21 sessions → returns `connect.CodeInvalidArgument`
- Validates: plan requirement "Returns CodeInvalidArgument for batch > 20"

**T-S2-U-02** `TestBatchCreateSessions_TitleDedup_WithinBatch` (P0)
- Input: `["Fix auth", "Fix auth", "Fix auth"]`
- Assert output titles: `["Fix auth -01", "Fix auth -02", "Fix auth -03"]`
- Validates: plan dedup requirement

**T-S2-U-03** `TestBatchCreateSessions_TitleDedup_DoesNotAffectUniqueNames` (P1)
- Input: `["Fix auth", "Add logging", "Fix auth"]`
- Assert: `"Add logging"` remains unchanged; only the duplicates are renamed

**T-S2-U-04** `TestBatchCreateSessions_PartialFailureDoesNotCancelRest` (P0 — Risk R-02)
- Provide 5 batch items; mock `createSessionInternal` to return an error on call 3
- Assert: results array has length 5
- results[0], results[1], results[3], results[4]: `success = true`
- results[2]: `success = false`, `error` non-empty
- Response: `succeeded == 4`, `failed == 1`
- Validates: Risk R-02 partial failure behavior

**T-S2-U-05** `TestBatchCreateSessions_ZeroSessions_InvalidArgument` (P1 — boundary value)
- Input: empty sessions list
- Assert: `connect.CodeInvalidArgument`

**T-S2-U-06** `TestBatchCreateSessions_MissingPath_ReturnsError` (P1 — input partitioning)
- One batch item has `path == ""`
- Assert: that item's result is `success = false`; others succeed

**T-S2-U-07** `TestBatchCreateSessions_SameRepo_SequentialWorktreeCreation` (P0 — Risk R-01)
- Use a real temp git repo; call `BatchCreateSessions` with 3 sessions all pointing to the same `path`
- Assert: no `.git/index.lock` errors; all 3 worktrees exist after the call
- Implementation note: introduce a deliberate 10ms delay inside `git worktree add` mock to expose races; run with `-race`
- Validates: Risk R-01 per-repo mutex requirement

**T-S2-U-08** `TestBatchCreateSessions_DifferentRepos_RunConcurrently` (P1)
- 3 sessions on 3 distinct repo paths (each a separate temp git repo)
- Assert: all 3 complete without sequencing delays (timing-based: total time < 1.5× single creation time)
- Validates: concurrency is not serialized across different repos

### Integration Tests

**File**: `server/services/batch_integration_test.go`

**T-S2-I-01** `TestBatchCreateSessions_SameRepo_NoIndexLock` (P0 — Risk R-01)
- Real git repo in `t.TempDir()`, initialized with `git init`
- `BatchCreateSessions` with 5 sessions on the same repo path
- Assert: all 5 sessions appear in storage, all 5 have valid `WorktreePath` values
- Assert: no test error from `exec.Command("git", "worktree", "list")` returning lock errors
- Run test with `go test -race`

**T-S2-I-02** `TestBatchCreateSessions_InitialPromptPropagatedToAll` (P1)
- Batch of 3 sessions with `initial_prompt = "build the widget"`
- Assert: all 3 sessions have a `session-prompt.md` in their respective worktrees

### UI Component Tests

**File**: `web-app/src/components/sessions/__tests__/SessionWizard.batch.test.tsx`

**T-S2-UI-01** `BatchTab_CannotSubmitWithZeroTasks` (P0 — boundary value)
- Render `SessionWizard`; switch to batch tab; leave textarea empty
- Assert: submit button is disabled

**T-S2-UI-02** `BatchTab_CannotSubmitWithMoreThan20Tasks` (P0 — boundary value)
- Enter 21 newline-separated lines in the textarea
- Assert: submit button is disabled OR an inline error shows "max 20 tasks"

**T-S2-UI-03** `BatchTab_SubmitEnabledAt20Tasks` (P0 — boundary value at max)
- Enter exactly 20 lines
- Assert: submit button is enabled

**T-S2-UI-04** `BatchTab_PreviewRendersNPills` (P1)
- Enter 3 tasks; assert 3 title pills are visible in the preview section (debounced — use `act()` with fake timers)

**T-S2-UI-05** `BatchTab_ResultsListShowsSuccessAndFailure` (P0 — Risk R-02 UI)
- Mock `BatchCreateSessions` to return results[0].success=true, results[1].success=false with error message "title exists"
- Submit; assert: green checkmark visible for result 0; red X plus "title exists" visible for result 1

**T-S2-UI-06** `BatchTab_CleanupButtonAppearsOnFailure` (P1)
- Any failed item in results → "Cleanup failed" button visible; clicking it calls `DeleteSession` for each failed item

---

## Story 3: Review Queue + One-Shot PR Creation

### Unit Tests

**Package**: `server/services/`
**File**: `server/services/oneshot_test.go`

**T-S3-U-01** `TestRunOneShot_ParsesPRURL_LastLine` (P0 — Risk R-05)
- Mock subprocess stdout: `"Updating branch...\nhttps://github.com/owner/repo/pull/42\n"`
- Assert: `pr_url == "https://github.com/owner/repo/pull/42"`
- Validates: URL parsed from the last non-empty line

**T-S3-U-02** `TestRunOneShot_ParsesPRURL_NoTrailingNewline` (P0 — Risk R-05)
- Mock stdout: `"Done\nhttps://github.com/owner/repo/pull/42"` (no trailing `\n`)
- Assert: `pr_url == "https://github.com/owner/repo/pull/42"`

**T-S3-U-03** `TestRunOneShot_ParsesPRURL_RejectsNonPRURL` (P0 — Risk R-05)
- Mock stdout: `"See https://github.com/owner/repo/issues/7\nDone"`
- Assert: `pr_url == ""` (issues URL is not a PR URL)

**T-S3-U-04** `TestRunOneShot_ParsesPRURL_MultipleURLs_TakesLast` (P1)
- Mock stdout has two valid PR URLs on separate lines (e.g., existing PR referenced in output, new PR on last line)
- Assert: `pr_url` is the last one

**T-S3-U-05** `TestRunOneShot_TimeoutEnforced` (P0)
- Mock subprocess that sleeps for 200s; set `timeout_seconds = 1`
- Assert: handler returns within ~3 seconds with a timeout error in `error` field and `exit_code != 0`
- Validates: "Returns within timeout_seconds + 5s"

**T-S3-U-06** `TestRunOneShot_NonZeroExitCode_ErrorFieldPopulated` (P0)
- Mock subprocess exits with code 1 and stderr "permission denied"
- Assert: `error` field contains "permission denied"; `exit_code == 1`

**T-S3-U-07** `TestCheckBranchDivergence_DivergesToTrue` (P0 — Risk R-04)
- Create a temp git repo; make a commit on `main`; create branch `feature/x`; add 2 more commits to `main` after the branch point
- Assert: `checkBranchDivergence(worktreeDir, "main")` returns `true`
- Validates: Risk R-04 divergence detection

**T-S3-U-08** `TestCheckBranchDivergence_UpToDateReturnsFalse` (P1 — Risk R-04)
- Branch created from the tip of `main`; no new commits on `main`
- Assert: `checkBranchDivergence()` returns `false`

**T-S3-U-09** `TestRunOneShot_CLAUDEBinaryNotOnPath_FallsBackToGH` (P1 — Risk: "claude binary not on PATH")
- Stub `exec.LookPath("claude")` to return an error
- Assert: handler uses the `gh pr create` fallback path without returning `CodeInternal`

### Integration Tests

**File**: `server/services/oneshot_integration_test.go`

**T-S3-I-01** `TestRunOneShot_UpdatesSessionPRURL` (P0)
- Create a real session in SQLite storage
- Stub the subprocess to emit a mock PR URL on stdout
- Call `RunOneShot(session_id, prompt)`
- Reload session from storage; assert `GitHubPRURL` equals the parsed URL

**T-S3-I-02** `TestRunOneShot_DivergenceFieldPopulatedInResponse` (P1 — Risk R-04)
- Set up a diverged worktree (branch behind `origin/main`)
- Assert: `RunOneShotResponse.branch_diverged_from_base == true`

### UI Component Tests

**File**: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.pr.test.tsx`

**T-S3-UI-01** `ReviewQueuePanel_CreatePRButton_AppearsForTaskComplete` (P0)
- Render `ReviewQueuePanel` with a session: `reason == REASON_TASK_COMPLETE`, `github_pr_url == ""`
- Assert: "Create PR" button is visible

**T-S3-UI-02** `ReviewQueuePanel_CreatePRButton_HiddenWhenPRExists` (P0)
- Same session but `github_pr_url = "https://github.com/owner/repo/pull/5"`
- Assert: "Create PR" button is NOT rendered

**T-S3-UI-03** `ReviewQueuePanel_CreatePRButton_HiddenForNonTaskComplete` (P0)
- Session with `reason == REASON_IDLE`
- Assert: "Create PR" button absent

**T-S3-UI-04** `ReviewQueuePanel_DivergenceWarningBadge` (P0 — Risk R-04)
- Session with `reason == REASON_TASK_COMPLETE`, `branch_diverged_from_base == true`
- Assert: warning badge with text "Diverged from main" (or equivalent) is visible

**T-S3-UI-05** `ReviewQueuePanel_Modal_OpensOnClick` (P0)
- Click "Create PR" button
- Assert: confirmation modal appears with a pre-filled textarea

**T-S3-UI-06** `ReviewQueuePanel_Modal_SpinnerOnSubmit` (P0)
- Open modal; click "Run"
- Assert: spinner element visible; "Run" button disabled during the pending RPC call

**T-S3-UI-07** `ReviewQueuePanel_Modal_ShowsPRURLOnSuccess` (P1)
- Mock `RunOneShot` to resolve with `pr_url = "https://github.com/owner/repo/pull/99"`
- Assert: modal shows a clickable link to the PR URL; modal closes (or transition to success state)

**T-S3-UI-08** `ReviewQueuePanel_Modal_ShowsErrorOnFailure` (P1)
- Mock `RunOneShot` to reject with an error
- Assert: error message visible in modal; "Retry" affordance present

---

## Story 4: Project Concept

### Unit Tests

**Package**: `session/ent/` + `server/services/`
**File**: `server/services/project_test.go`

**T-S4-U-01** `TestCreateProject_UniqueNameEnforced` (P1)
- Create project "Alpha"; attempt to create another project "Alpha"
- Assert: second call returns `connect.CodeAlreadyExists`
- Validates: plan "Returns CodeAlreadyExists if name is taken"

**T-S4-U-02** `TestDeleteProject_NullsSessionFK_DoesNotDeleteSessions` (P1 — Risk: DeleteProject race)
- Create project; assign 3 sessions to it
- Delete project; reload sessions
- Assert: all 3 sessions still exist in storage with `project_id == nil`
- Validates: plan "sessions become ungrouped, not deleted"

**T-S4-U-03** `TestDeleteProject_Transactional` (P1 — Risk: DeleteProject race)
- Verify the "null FK on sessions + delete project" operation is wrapped in a single ent transaction
- Implementation note: if ent client doesn't expose transaction visibility, this is verified by a race test — run two goroutines: one deleting the project, one updating a session's status in the same project; assert no data corruption

**T-S4-U-04** `TestListProjects_AggregateCounts` (P1)
- Create project "Beta" with 2 Running sessions, 1 Complete session
- `ListProjects()` — assert the project entry has `running_count == 2`, `complete_count == 1`
- Validates: plan "ListProjects aggregate stats are correct"

**T-S4-U-05** `TestListSessions_FilteredByProjectID` (P1)
- 2 projects; sessions in each; `ListSessions(project_id = project1.id)`
- Assert: only project1's sessions returned
- Validates: plan `ListSessions` with project_id filter

**T-S4-U-06** `TestGroupByProject_UngroupedCatchall` (P1)
- Sessions list: 2 with `project_id = project1`, 1 with `project_id = nil`
- `GroupByProject` strategy applied
- Assert: 2 groups exist: "Alpha" group with 2 sessions, "Ungrouped" group with 1 session
- Validates: plan "sessions with project_id = NULL go into Ungrouped"

**T-S4-U-07** `TestGroupByProject_SessionCountsInHeader` (P2)
- Assert: group header shows correct count matching actual sessions

**Package**: `session/ent/schema/`
**File**: `session/ent/schema/project_migration_test.go`

**T-S4-U-08** `TestProjectMigration_NoDangerousOptions` (P0 — Risk: ent migration drop risk)
- Open the server startup code; verify `ent.Schema.Create(ctx)` is called without `migrate.WithDropColumn` or `migrate.WithDropIndex`
- Implementation note: this is a code-review-style test; the actual check is a grep assertion in the test:
  ```go
  content, _ := os.ReadFile("../../main.go") // or wherever server startup is
  assert.NotContains(t, string(content), "WithDropColumn")
  assert.NotContains(t, string(content), "WithDropIndex")
  ```
- Validates: Risk "ent migration drop risk"

### Integration Tests

**File**: `server/services/project_integration_test.go`

**T-S4-I-01** `TestProjectMigration_AddsProjectsTableNoDropColumns` (P0 — Risk: ent migration drop risk)
- Create a pre-migration SQLite database with a known schema (sessions table with existing columns)
- Run `ent.Schema.Create(ctx)` against it (the actual migration)
- Assert: `projects` table exists; existing `sessions` table columns are all still present (no column was dropped)
- Validates: Risk "ent migration drop risk" at runtime

**T-S4-I-02** `TestProjectCRUD_EndToEnd` (P1)
- `CreateProject` → `ListProjects` (see it) → `UpdateProject` (rename) → `ListProjects` (see new name) → `DeleteProject` → `ListProjects` (gone)
- Full round-trip against real ent/SQLite

**T-S4-I-03** `TestAssignSessionsToProject_BatchUpdate` (P1)
- 3 sessions in storage (no project); call `AssignSessionsToProject(project_id, [s1_id, s2_id, s3_id])`
- Assert: all 3 sessions have `project_id` set; `ListProjects` shows count 3

### UI Component Tests

**File**: `web-app/src/components/sessions/__tests__/SessionWizard.project.test.tsx`

**T-S4-UI-01** `SessionWizard_ProjectPicker_ShowsAllProjects` (P1)
- Mock `ListProjects` returning ["Alpha", "Beta"]
- Open `SessionWizard`; assert project picker dropdown shows "Alpha" and "Beta" plus "(No project)"

**T-S4-UI-02** `SessionWizard_ProjectPicker_DefaultIsNoProject` (P1)
- Without selecting a project, submit
- Assert: `CreateSessionRequest.project_id` is empty/null

**T-S4-UI-03** `SessionWizard_ProjectPicker_InlineCreate` (P2)
- Type "Gamma" in the inline "New project" field; press Enter
- Assert: `CreateProject` RPC was called with name "Gamma"; "Gamma" now appears selected in the dropdown

**File**: `web-app/src/lib/grouping/strategies.test.ts`

**T-S4-UI-04** `GroupSessions_ByProject_UngroupedCatchall` (P1)
- Add `GroupingStrategy.Project` to the existing `strategies.test.ts` describe block (consistent with existing tag/category tests)
- Sessions: 2 with `project_id = "proj-1"`, 1 with no `project_id`
- Assert: 2 groups — "Alpha" (2 sessions) and "Ungrouped" (1 session)

**T-S4-UI-05** `GroupSessions_ByProject_AggregateCounts` (P2)
- Group returns correct per-group running/complete counts

**File**: `web-app/src/components/sessions/__tests__/ProjectPanel.test.tsx`

**T-S4-UI-06** `ProjectPanel_RenameOptimisticUpdate` (P1)
- Mock `UpdateProject`; trigger inline rename; before RPC resolves, assert new name already visible (optimistic update)

**T-S4-UI-07** `ProjectPanel_DeleteConfirmationShowsSessionCount` (P1)
- Delete button for project with 4 sessions; assert confirmation dialog text contains "4 sessions will become ungrouped"

---

## Test Infrastructure Requirements

### Go Mocks and Stubs

| Interface / Component | Mock Location | Usage |
|-----------------------|--------------|-------|
| `TmuxManager` | `session/tmux/mock_tmux_manager.go` (already exists from recent refactor) | S1 injection tests — prevent real tmux calls |
| `GitManager` | `session/git/mock_git_manager.go` (already exists from recent refactor) | S1 injection tests — control worktree path |
| `exec.Cmd` subprocess | Use `testexec` helper or replace `exec.Command` with an injectable `CommandRunner` interface | S3 `RunOneShot` tests |
| ent/SQLite client | `session.NewEntRepository(session.WithDatabasePath(":memory:"))` — existing pattern from `session_service_test.go` | All integration tests |
| `PromptStore` | Concrete with `t.TempDir()` for filesystem path | S1 integration tests |

**New infrastructure needed**:

1. **`testexec.CommandStub`** — a small helper that allows tests to inject a fake `exec.Cmd` by swapping a `var execCommand = exec.Command` package-level variable (standard Go pattern). Required for S3 `RunOneShot` unit tests.

   ```go
   // session/testutil/fake_command.go (build-tagged _test)
   var ExecCommand = exec.Command

   func MakeFakeCommand(stdout, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
       return func(name string, args ...string) *exec.Cmd {
           cmd := exec.Command("echo", stdout)
           // ... configure to exit with exitCode
           return cmd
       }
   }
   ```

2. **`createTestPromptStore(t)`** helper in `session/prompts/store_test.go`:
   ```go
   func createTestPromptStore(t *testing.T) *PromptStore {
       dir := t.TempDir()
       return NewPromptStore(filepath.Join(dir, "prompts.json"))
   }
   ```

3. **`seedPromptEntries(store, n, baseTime)`** helper — creates N entries with LastUsed values spaced 1 second apart from `baseTime`, for eviction boundary tests.

4. **`createBareGitRepo(t)`** helper in `server/services/` or `session/git/` — creates a `git init` repo in `t.TempDir()`, adds an initial commit, for batch and divergence integration tests.

### Frontend Test Infrastructure

| Need | Implementation |
|------|----------------|
| Mock `createSession` RPC | `vi.fn()` / `jest.fn()` on the ConnectRPC client; existing pattern in `Omnibar.discovery.test.tsx` |
| Mock `listPromptHistory` RPC | Same pattern |
| Mock `batchCreateSessions` RPC | Same pattern |
| Mock `runOneShot` RPC | Same pattern; return a delayed `Promise` to test spinner state |
| Fake timers for debounce | `vi.useFakeTimers()` + `vi.runAllTimers()` for the 300ms batch preview debounce |
| `act()` wrappers | All state-updating interactions must be wrapped in RTL `act()` |

---

## Coverage Targets

| Package | Target Line Coverage | Rationale |
|---------|----------------------|-----------|
| `session/prompts/` (new) | 90% | Pure logic, no system calls; high coverage is cheap |
| `session/` (injection additions) | 80% | New functions in `instance.go`; existing package has broad coverage |
| `server/services/` (new handlers) | 75% | RPC handlers have error paths that are expensive to exercise; target covers happy paths + top risks |
| `session/ent/schema/` (new) | 70% | Generated code mostly; focus on migration test |
| `web-app/src/components/sessions/` (new/modified) | 70% | Component tests cover the wiring; exact visual polish is not test-driven |
| `web-app/src/lib/grouping/` (GroupByProject) | 85% | Pure function; mirrors existing tag/category coverage level |

---

## Definition of Done (Testing)

Before implementation is considered complete, all of the following must be true:

- [ ] All P0 tests pass: `make test` exits 0 across all Go packages
- [ ] All P0 UI tests pass: `cd web-app && npm test -- --run` exits 0
- [ ] `make lint` passes (linting blocks build; no new golangci-lint warnings)
- [ ] `make nil-safety` passes (NilAway + go vet -nilness) on all new code
- [ ] `go test -race ./session/prompts/ ./server/services/` shows no race conditions
- [ ] `TestProjectMigration_AddsProjectsTableNoDropColumns` (T-S4-I-01) passes against the actual production-path `schema.Create()` call
- [ ] `TestBatchCreateSessions_SameRepo_NoIndexLock` (T-S2-I-01) passes with `-race` flag
- [ ] `TestCLAUDEmdInjection_ShellMetacharactersWrittenLiterally` (T-S1-U-11) passes — Risk R-03 is mitigated
- [ ] All 5 Risk Register mitigating tests (R-01 through R-05) pass
- [ ] No regression in existing tests: `make test` passes on the full test suite (not just new packages)
- [ ] Coverage targets met (verified via `make test-coverage`)
- [ ] Pre-commit hook passes: `make pre-commit`

---

## Appendix: Test Execution Commands

```bash
# Run all new Story 1 (PromptStore) unit tests
go test ./session/prompts/ -v -run TestPromptStore

# Run all Story 1 injection unit tests
go test ./session/ -v -run TestCLAUDEmdInjection

# Run batch creation with race detection
go test -race ./server/services/ -run TestBatchCreateSessions -timeout 60s

# Run all Story 3 one-shot tests
go test ./server/services/ -v -run TestRunOneShot

# Run all Story 4 project tests
go test ./server/services/ -v -run TestProject
go test ./server/services/ -v -run TestListSessions_FilteredByProject

# Run all integration tests (requires build first)
make build && go test ./server/services/ -v -run '.*Integration.*' -timeout 120s

# Run frontend tests
cd web-app && npm test -- --run

# Run frontend tests with coverage
cd web-app && npm test -- --coverage

# Full validation pass
make build && make test && make lint && make nil-safety
```
