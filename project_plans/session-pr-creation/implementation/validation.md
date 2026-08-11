# Validation Plan: session-pr-creation

**Date**: 2026-08-06

## Happy Path Scenario

Given a live session `sess-7f3a` titled "Add rate limit toggle" with an active worktree on branch `feature/rate-limit-toggle` that is 3 commits ahead of `main` and has no existing PR, when the user opens the session card's overflow menu, clicks "Create PR" (which opens `CreatePullRequestModal`, auto-drafts via `DraftPullRequest`, and pre-fills title/body/base-branch), reviews the pre-filled fields, and clicks the "Create PR" submit button, then `CreatePullRequest` pushes the branch, calls `GitWorktree.CreatePR` directly (no agentic `RunOneShot` turn), persists `GitHubPrUrl`/`GitHubPrNumber` on the session, and the modal shows "Created PR #512" with a working link to GitHub.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: modal pre-fill (title/body/base branch) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_PrefillTitleBodyBaseBranch_When_SessionHasCommitsAhead` | Unit | Happy path |
| AC1: modal pre-fill — no live instance | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_ReturnNotFound_When_SessionInstanceMissing` | Unit | Error path |
| AC1: pre-fill fields exist on the wire (proto) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_PrefillTitleBodyBaseBranch_When_SessionHasCommitsAhead` | Integration | Asserts `DraftPullRequestResponse` fields via real RPC handler + real temp git worktree + fake `gh`/executor (per `worktree_git_test.go`'s fake-executor pattern) |
| AC2: title/body/base-branch editable client-side | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_PrefillFields_When_Opened` | Unit | Happy path |
| AC2: editing surfaced correctly (no clamp/reject) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_AllowEditingBaseBranch_When_UserTypes` | Unit | Error/edge path (empty/overwritten field still accepted, no client validation) |
| AC3: mechanical path only, no agent turn | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_CallCreatePRDirectly_NotHeadlessPool` | Unit | Happy path |
| AC3: base branch honored by `gh pr create --base` | `session/git/worktree_git_test.go` | `TestGitWorktree_CreatePR_PassesBaseBranch_When_NonEmpty` | Unit | Happy path |
| AC3: empty base branch preserves default-resolution fallback | `session/git/worktree_git_test.go` | `TestGitWorktree_CreatePR_OmitsBaseFlag_When_Empty` | Unit | Error/regression path |
| AC3: mechanical path called end-to-end via RPC | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_CallCreatePRDirectly_NotHeadlessPool` | Integration | Real worktree + fake `gh` executor, asserts `headlessPool.CallBlocking` never invoked |
| AC4: existing-PR short-circuit (no duplicate) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_ReturnExistingPR_When_SessionAlreadyHasOne` | Unit | Happy path |
| AC4: existing-PR reuse via `CreatePullRequest` fast path | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_SetAlreadyExisted_When_SessionHasCachedPRUrl` | Unit | Error/edge path (avoids a second `gh pr create` call) |
| AC4: existing-PR short-circuit skips diff/draft work | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_ReturnExistingPR_When_SessionAlreadyHasOne` | Integration | Asserts no `GetGitDiff`/`headlessPool.CallBlocking` invocation against a real worktree |
| AC5: PR URL persisted + event published on success | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_PersistAndPublishEvent_When_CreateSucceeds` | Unit | Happy path |
| AC5: persist failure surfaced as structured partial success | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_ReturnPersistedFalse_When_SaveInstancesFails` | Unit | Error path |
| AC5: persistence integration (storage + event bus) | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_PersistAndPublishEvent_When_CreateSucceeds` | Integration | Real `storage.SaveInstances` + real `eventBus.Publish`, asserts `SessionUpdatedEvent` fields |
| AC6: specific error surfaced — gh not authenticated | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_SurfaceSpecificError_When_GHNotAuthenticated` | Unit | Happy path (error is the "expected" outcome under test) |
| AC6: specific error surfaced — commit failure not swallowed | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_SurfaceError_When_CommitFails` | Unit | Error path |
| AC1: `DraftPullRequest` previews working-tree-inclusive diff, stays read-only (pre-mortem #1 fix) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_PreviewWorkingTreeDiff_When_UncommittedChangesPresent` | Unit | Happy path (also asserts zero `wt.CommitChanges` calls) |
| AC6: no-commits-ahead gates the trigger | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_ReportHasCommitsAhead_False_When_BranchIsUpToDate` | Unit | Happy path (false is the expected/correct value) |
| AC6: push-rejected error surfaced verbatim | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_SurfaceError_When_PushFails` | Integration | Fake executor returns non-fast-forward push error; asserts `connect.CodeUnavailable` with literal message |
| AC7: single entry point, old button removed | `web-app/src/components/sessions/SessionActionsOverflow.test.tsx` | `SessionActionsOverflow_should_NotRenderOnRunOneShotButton_When_SessionHasWorktree` | Unit | Happy path |
| AC7: `ReviewQueuePanel` uses shared modal, not inline `prModal` | `web-app/src/components/sessions/ReviewQueuePanel.test.tsx` | `ReviewQueuePanel_should_OpenCreatePullRequestModal_When_CreatePrClicked` | Unit | Happy path |
| AC7: grep regression guard (no `onRunOneShot` anywhere) | n/a (verification script, not a test file) | `grep -rn "onRunOneShot" web-app/src/` returns no matches | Integration | Repo-wide static check, run in Epic 2.5's Task 2.5.1g and CI |
| AC8: Go test coverage on mechanical RPC path | `server/services/create_pull_request_test.go` | `go test ./server/services -run 'TestCreatePullRequest\|TestDraftPullRequest'` | Integration | Full suite run, happy + error paths |
| AC8: Playwright e2e coverage | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > creates a PR from the modal happy path` | E2E | Happy path |
| AC9: backend registry entries updated | `docs/registry/features/backend/session/draft-pull-request.json`, `.../create-pull-request.json` | n/a (registry file presence + `make registry-generate`) | Integration | `git diff --stat docs/registry/coverage-gaps.json` shows no net increase |
| AC9: frontend registry entry updated | `docs/registry/features/frontend/create-pull-request-modal.json` | n/a (registry file presence + `make registry-generate`) | Integration | Same coverage-gap check |
| In-flight guard (concurrent double-click / two-tab), pitfalls.md §3 | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_RejectConcurrentCall_When_AlreadyInFlight` | Unit | Error path |
| `prNumber == 0` after non-error `CreatePR` return (BUG-063(a) analog) | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_ReturnInternalError_When_PRNumberIsZero` | Unit | Error path |
| nil `headlessPool` fallback (Post-Review Revision #1) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_UseFallbackBody_When_HeadlessPoolNil` | Unit | Error/edge path |
| Empty-diff fallback body (Task 1.3.1c) | `server/services/create_pull_request_test.go` | `TestDraftPullRequest_should_UseFallbackBody_When_DiffIsEmpty` | Unit | Error/edge path |
| `RecordPRCreatedOutOfBand` called unconditionally, nil-checked | `server/services/create_pull_request_test.go` | `TestCreatePullRequest_should_CallRecordPRCreatedOutOfBand_When_BacklogListenerPresent` | Unit | Happy path |
| `PRCreationService` extraction wiring (Epic 1.0) | `server/services/pr_creation_service_test.go` | `TestNewPRCreationService_should_ConstructWithNarrowFindInstanceFunc` | Unit | Happy path |
| `SessionService` delegator methods forward correctly (Epic 1.0) | `server/services/session_service_test.go` | `TestSessionService_DraftPullRequest_should_DelegateToPRCreationService` | Unit | Happy path |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Happy path in ≤3 clicks (Surfaces 1→3→4→6→7) | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > creates a PR from the modal happy path` | Playwright | Open overflow menu (`getByTestId("create-pr-trigger-<id>")`), click; wait for `create-pr-title-input` to have non-empty value (`expect(locator).not.toHaveValue("")`, no `waitForTimeout`); click `create-pr-submit`; assert `getByTestId("github-pr-link")` visible |
| 2. Viewing existing PR takes 1 click (Surface 1 State C) | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > shows View PR link and skips the modal when a PR already exists` | Playwright | For a session with `githubPrUrl` set, click trigger; assert `getByTestId("github-pr-link")` is an `<a target=_blank>` and the dialog (`role="dialog"`) never appears |
| 3. Editing requires no extra clicks (in-place edit) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_AllowEditingBaseBranch_When_UserTypes` | Jest/RTL | Render modal in editable-form state, fire change event on base-branch input, assert value updates without any mode-switch action |
| 4. Every error state shows the specific backend message | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > surfaces the exact gh-not-authenticated error message` | Playwright | Induce `gh` auth failure server-side (test fixture), submit, assert `getByTestId("create-pr-error")` text equals the literal backend string, not a generic fallback |
| 5. Persist-failure never displays as a failure (Surface 7 Variant C) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_ShowPersistWarning_When_PersistedFalse` | Jest/RTL | Mock `createPullRequest` resolving `persisted:false`; assert success PR link renders alongside a `role="alert"` warning banner, and the error-state UI (Surface 8) is not rendered |
| 6. No dead ends — every error state offers retry + exit | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_OfferRetryAndClose_When_DraftFetchFails` | Jest/RTL | Mock `draftPullRequest` rejecting; assert both a `Retry` button and a `Close`-equivalent exit are present and enabled |
| 7. Field values survive a submit failure | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_PreserveFieldValues_When_SubmitFails` | Jest/RTL | Edit all three fields, mock `createPullRequest` rejecting, assert inputs still show edited values post-error |
| 8. No duplicate-PR risk from the UI (inputs locked during submit) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_DisableAllFieldsAndButtons_When_Submitting` | Jest/RTL | Trigger submit, assert title/body/base-branch inputs and both Cancel/Create-PR buttons are `disabled`; fire two rapid clicks and assert `createPullRequest` called exactly once |
| 9. Keyboard-only operation (tab order, Escape) | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > supports keyboard-only operation through submit` | Playwright | Open modal via keyboard, `Tab` through title → body → base branch → Cancel → Create PR, verify focus order via `page.evaluate(() => document.activeElement)` or ARIA snapshot; press `Escape` from mid-dialog focus and assert dialog closes |
| 10. Focus management (dialog → title input → trigger on close) | `tests/e2e/create-pull-request.spec.ts` | `create-pull-request > moves focus into dialog then to title input, and returns focus to trigger on close` | Playwright | Assert focus is trapped in dialog while drafting, moves to `create-pr-title-input` once the draft resolves, and returns to the trigger button after `Close`/Escape |
| 11. Screen-reader labeling (`<label htmlFor>`, `aria-labelledby`) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_HaveAccessibleLabelsForAllInputs_When_Rendered` | Jest/RTL + `jest-axe` | Render form state, run `axe()` against the container, assert zero label/name violations; assert `getByLabelText` resolves for title/body/base-branch |
| 12. Color contrast ≥4.5:1 in default and error/warning states | manual / `ui-web-design-guidelines` skill audit | n/a — human-verifiable, not automatable in Jest | Manual (axe DevTools or skill audit) | Load the modal in each of Surfaces 4, 7 (Variant C), and 8; run a contrast checker against the actual theme tokens; confirm ≥4.5:1 |
| 13. No information by color alone (icon + text pairing) | `web-app/src/components/sessions/CreatePullRequestModal.test.tsx` | `CreatePullRequestModal_should_PairIconAndTextWithSuccessAndWarningStates_When_Rendered` | Jest/RTL | Assert success state renders "✅" + "Created"/"Updated" text, warning banner renders "⚠" + explanatory text — not solely a CSS class/color |
| 14. Disabled-state semantics (native `disabled` + `title` tooltip) | `tests/e2e/create-pull-request.spec.ts` (Task 3.2.1c basis) | `create-pull-request > disables the trigger with a tooltip when there are no commits ahead` | Playwright | For a zero-commits-ahead session, assert `getByTestId("create-pr-trigger-<id>")` has the `disabled` attribute and its `title` attribute equals "No commits ahead of main yet" |
| 15. z-index uses token, not magic number | grep-based static check (part of Task 2.1.1a verification) | n/a | Static check | `grep -n "9999" web-app/src/components/sessions/CreatePullRequestModal.css.ts` returns no matches; confirm `vars.zIndex.modal` is referenced instead |
| 16. Single entry point per session card (AC7 regression guard) | grep-based static check (Task 2.5.1's own verification) | n/a | Static check | `grep -rn "onRunOneShot" web-app/src/` returns no matches; manual visual pass over `SessionActionsOverflow.tsx` and `ReviewQueuePanel.tsx` confirms Create-PR-button XOR View-PR-link, never both |

## Migration Test

N/A — Migration Plan is "Omitted — no schema/data changes." Proto changes are additive and regenerated via `make proto-gen`, not a data migration; no `migration_should_be_reversible`-style test applies.

## Test Stack

- **Unit**: Go `testing` package (`server/services/create_pull_request_test.go`, `server/services/pr_creation_service_test.go`, `session/git/worktree_git_test.go`) / Jest + React Testing Library (`web-app/src/components/sessions/*.test.tsx`)
- **Integration**: Real temp-git-repo worktrees constructed the same way `server/services/session_retention_sweeper_test.go` and `session/backlog_lifecycle_test.go` already do (no mocked `GitWorktree`, per architecture.md §5's "no new interface" decision), with `gh`/git shelled-out calls stubbed via `GitWorktree`'s existing `cmdExec` executor-injection seam (`worktree_git.go:301-314`), matching `worktree_git_test.go`'s existing fake-executor pattern (e.g. `raceSimulatorExecutor`, `countingErrExecutor`). Test doubles for `storage`/`eventBus`/`backlogLifecycleListener` follow `backlog_lifecycle_test.go`'s existing fake-type conventions (e.g. `fakeQueueDequeuer`, `fakePRPendingChecker`) — a `fakePRCreationStorage`/`fakeEventBus` recording-call-args style, not a mocking framework.
- **E2E / UX**: Playwright (`tests/e2e/create-pull-request.spec.ts`), following `.claude/rules/e2e-test-conventions.md`: `@feature` header, `data-testid`/ARIA locators only, no `waitForTimeout` (poll via `expect(locator).toHaveValue(...)`/`toBeVisible()` instead).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods (`PRCreationService.DraftPullRequest`, `PRCreationService.CreatePullRequest`, `SessionService`'s two thin delegators): happy path + error paths covered.
- All external integrations (`gh` CLI via `cmdExec`, `headless.DraftPRDescription`, `storage.SaveInstances`, `eventBus.Publish`, `RecordPRCreatedOutOfBand`): unit mocked (fake executor / fake storage / fake event bus) + at least one integration test exercising the real `GitWorktree` + real temp git repo end to end.
- UX acceptance criteria: each of the 16 criteria in `design/ux.md`'s "UX Acceptance Criteria" section has a corresponding automated test above, except Criterion 12 (color contrast), which is explicitly manual/tool-audit per the criterion's own text ("verified via a contrast-checking tool").
