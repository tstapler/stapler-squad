# Validation Plan: session-completion-summary

**Date**: 2026-08-03

## Happy Path Scenario

Given a running session `sess-123` with a dirty worktree (files changed, a mix of
auto-approved/manually-approved decisions, several minutes of activity), when the
user stops it via the explicit stop/delete path (`DeleteSession` → `Instance.Destroy()`
firing `EventStopped`, reason `"operator-destroy"`) or the process exits naturally
(`EventExited`, reason ≠ `"reconcile-session-missing"`), then within the 2s poll
window the session's Summary tab becomes enabled and renders a `READY`
`SessionSummary` document — narrative, Changes (diff stats + link), Decisions
breakdown, Timeline, and Token Usage — which the user copies to the clipboard as
valid GFM markdown in one click, and which remains retrievable via
`/sessions/sess-123/summary` even after the `Session` row is deleted and the
server restarts.

## Requirement → Test Mapping

Per the task brief, FR-2 is decomposed into its 5 named sub-sections (narrative,
changes/diff, decisions, timeline, token usage) since each has a distinct
Domain Glossary builder function (`BuildDiffSnapshot`, `BuildDecisionsSnapshot`,
`BuildCostSnapshot`, `GenerateSessionCompletionNarrative`) and a distinct Story in
plan.md (1.3.1/1.3.2/1.3.3/1.4.1). Rows marked **[plan]** cite a test case already
committed to in plan.md's Tasks (1.1.2b, 1.3.4, 1.5.1b, 1.5.2d, 2.2.2b, 3.1.1b,
3.1.2c, 3.2.1d, 3.3.1b, 3.3.2b, 4.2.1a) — do not re-derive a parallel test with a
different name for these; implement exactly what the task already specifies. Rows
marked **[new]** are gaps this validation pass adds because the plan's task list
doesn't already cover that happy/error/integration angle.

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| FR-1: Trigger scope (AC-1) | `session/session_summary_listener_test.go` | `OnLifecycleEvent_should_DispatchGenerateAndPersist_When_EventExitedOrEventStoppedFireWithNormalReason` | Unit | Happy path — **[plan, Task 1.1.2b]** |
| FR-1: Trigger scope (AC-1 exclusion) | `session/session_summary_listener_test.go` | `OnLifecycleEvent_should_NotDispatch_When_ReasonIsReconcileSessionMissing` | Unit | Error/exclusion path — **[plan, Task 1.1.2b]** |
| FR-1: Trigger scope (AC-1 partial, diff-capture ordering) | `session/instance_test.go` | `Destroy_should_CaptureDiffStatsBeforeCleanupWorktree_When_UpdateDiffStatsRunsFirst` | Integration | `Destroy()` on a real `*Instance` with a dirty worktree; assert `i.GetDiffStats()` returns non-nil `{Added,Removed}` after `CleanupWorktree()` has deleted the worktree dir — **[new]**, closes the gap that Task 1.1.1a (`session/instance.go`) has no dedicated test task in plan.md; Story 1.1.1's own AC is exactly this ordering claim |
| FR-2 (narrative): "What Was Done" (AC-2 partial) | `session/headless/features_test.go` | `GenerateSessionCompletionNarrative_should_ReturnProseAndCallPoolWithFeatureKey_When_TitleGoalDiffAndDecisionsProvided` | Unit | Happy path — **[plan, Story 1.4.1 AC]**, updated per pre-mortem P1 #1's `sessionTitle`/`sessionGoal` grounding-input widening |
| FR-2 (narrative): empty goal doesn't break the call (AC-2 partial) | `session/headless/features_test.go` | `GenerateSessionCompletionNarrative_should_OmitGoalLine_When_SessionGoalIsEmptyString` | Unit | Edge path — **[plan, Story 1.4.1 AC "empty goal"]**, closes pre-mortem P1 #1's new grounding input for the common case of a session that never had a goal set |
| FR-2 (narrative): LLM failure fallback (AC-5) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_UseFallbackNarrative_When_LLMCallFails` | Unit | Error path (fake `headless.PoolClient` returns error) — **[plan, Task 1.5.2d "LLM failure" case]** |
| FR-2 (narrative): timeout enforcement | `session/session_summary_service_test.go` | `GenerateAndPersist_should_TimeoutNarrativeCall_When_PoolBlocksLongerThanLlmNarrativeTimeout` | Integration | Ent in-memory client + fake pool that blocks past `llmNarrativeTimeout` (60s, use a short test override or a context-cancel probe); asserts fallback used, pipeline still reaches `READY` — **[new]**, exercises the `context.WithTimeout` wrapper from Task 1.4.2a end-to-end |
| FR-2 (changes/diff): populated (AC-2 partial) | `session/session_summary_snapshot_test.go` | `BuildDiffSnapshot_should_ReturnPopulatedSnapshot_When_DiffStatsProvided` | Unit | Happy path — **[plan, Task 1.3.4]** |
| FR-2 (changes/diff): nil-safe (AC-6 partial) | `session/session_summary_snapshot_test.go` | `BuildDiffSnapshot_should_ReturnEmptySnapshot_When_DiffStatsIsNil` | Unit | Error/edge path — **[plan, Task 1.3.4, Story 1.3.1 AC]** |
| FR-2 (changes/diff): persisted round-trip | `session/session_summary_service_test.go` | `GenerateAndPersist_should_PersistDiffFieldsFromCapturedGitDiffStats_When_HappyPath` | Integration | Ent in-memory client; asserts `diff_files_changed`/`diff_added`/`diff_removed` land correctly and the rendered markdown's `[View full diff](...)` link is present — **[plan, Task 1.5.2d "happy path"]** |
| FR-2 (decisions): mixed bucketing (AC-2 partial) | `session/session_summary_snapshot_test.go` | `BuildDecisionsSnapshot_should_ReturnCorrectBucketCounts_When_MixedNotificationTypes` | Unit | Happy path — **[plan, Task 1.3.4]** |
| FR-2 (decisions): review-queue timeout | `session/session_summary_snapshot_test.go` | `BuildDecisionsSnapshot_should_ReturnError_When_ReviewQueueLookupExceedsTimeout` | Unit | Error path — fake `ReviewQueueLookup` that blocks past `reviewQueueLookupTimeout` (2s); the only real error source per Task 1.3.2a — **[new]**, closes gap: Task 1.3.4's list covers zero/mixed counts but not this function's one documented error source |
| FR-2 (decisions): real store query | `session/session_summary_snapshot_test.go` (or a dedicated `_integration_test.go` if the package separates build tags) | `BuildDecisionsSnapshot_should_QueryNotificationHistoryStore_When_SessionHasFiveAutoApprovedRecords` | Integration | Real `notifications.NotificationHistoryStore` (`server/notifications/store.go`) seeded with 5 auto-approved + 1 manual-approval record for `session_id: "sess-123"`; asserts `DecisionsSnapshot{AutoApproved:5, ManuallyApproved:1}` — **[plan, Story 1.3.2 AC-2 partial example]** |
| FR-2 (timeline): duration computation (AC-2 partial) | `session/session_summary_snapshot_test.go` | `BuildTimelineSnapshot_should_ComputeDuration_When_CreatedAtAndStoppedAtProvided` | Unit | Happy path — **[plan, Task 1.3.4 scope, Story 1.3.1]** |
| FR-2 (timeline): sub-second render | `session/session_summary_markdown_test.go` | `RenderSessionSummaryMarkdown_should_ShowSubSecondDuration_When_DurationRoundsToZero` | Unit | Edge path ("Duration: <1s") — **[plan, Task 1.5.1b]** |
| FR-2 (timeline): persisted round-trip | `session/session_summary_service_test.go` | `GenerateAndPersist_should_PersistTimelineFromInstanceCreatedAtAndDispatchTime_When_HappyPath` | Integration | Ent in-memory client; asserts `session_started_at`/`session_stopped_at`/`duration_ms` persisted — **[plan, Task 1.5.2d "happy path"]** |
| FR-2 (token usage/cost): populated (AC-2 partial) | `session/session_summary_snapshot_test.go` | `BuildCostSnapshot_should_ReturnPopulatedSnapshot_When_TokenStoreReturnsParseResult` | Unit | Happy path — **[plan, Task 1.3.4]** |
| FR-2 (token usage/cost): unavailable vs. zero (AC-6 partial) | `session/session_summary_snapshot_test.go` | `BuildCostSnapshot_should_ReturnDataUnavailable_When_TokenStoreReturnsNil` | Unit | Error/edge path — **[plan, Task 1.3.4]** |
| FR-2 (token usage/cost): real transcript parse | `session/session_summary_snapshot_test.go` | `BuildCostSnapshot_should_ReturnTotalTokensAndCost_When_RealTranscriptParsedByTokenStore` | Integration | Real `tokens.TokenStore` against a fixture JSONL transcript (reuse an existing `session/tokens` test fixture) — **[new]**, closes gap: Task 1.3.4 only specifies nil/zero `ParseResult` fakes, not a real-store integration path |
| FR-3: Independent persistence (AC-3) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_UpsertBySessionIDNotEdge_When_HappyPath` | Unit | Happy path — asserts the ent schema write path never references a `Session` edge — **[new]**, direct test of Story 1.2.1's "no `Edges()` method" design decision |
| FR-3: Independent persistence, no row yet | `server/services/session_summary_service_test.go` | `GetSessionSummary_should_ReturnNilSummary_When_NoRowExistsYet` | Unit | Error/edge path — **[plan, Task 2.2.2b]** |
| FR-3: Survives Session-row deletion (AC-3) | `server/services/session_summary_service_test.go` | `GetSessionSummary_should_ReturnRow_When_SessionEntRowDeleted` | Integration | Ent in-memory client; delete the `Session` row, assert `GetSessionSummary` still returns the `SessionSummaryProto` and never calls `s.storage`/`SessionService.findInstance` — **[plan, Task 2.2.2b, Story 2.2.1 AC]** |
| FR-4: Export (AC-4) | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_CallCopyToClipboardWithMarkdown_When_CopyButtonClicked` | Unit (Jest+RTL) | Happy path — **[plan, Task 3.1.2c]** |
| FR-4: Export failure (AC-4) | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_AnnounceCopyFailureViaAriaLive_When_ClipboardWriteRejects` | Unit (Jest+RTL) | Error path — **[plan, Task 3.1.2c "announces success/failure via aria-live"]** |
| FR-4: Export delegation | `web-app/src/lib/hooks/useSessionSummary.test.ts` | `useSessionSummary_copy_should_DelegateToClipboardHelperAndReturnBooleanResult_When_Called` | Integration | Exercises the real `copyToClipboard` helper (`web-app/src/lib/clipboard.ts`) against a mocked `navigator.clipboard` — **[plan, Task 3.1.1b]** |
| FR-5: Async/non-blocking (AC-5) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_ReachReadyWithFallbackNarrative_When_TrivialSession` | Unit | Happy path — **[plan, Task 1.5.2d]** |
| FR-5: Deterministic-stage failure → ERROR (AC-5) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_SetStatusErrorWithDecisionsStage_When_DecisionsBuilderFails` | Unit | Error path — asserts already-computed diff/timeline persisted, prior `markdown`/`narrative` untouched — **[plan, Task 1.5.2d]** |
| FR-5: Teardown non-blocking | `session/instance_test.go` | `Destroy_should_ReturnBeforeGenerateAndPersistCompletes_When_SessionSummaryListenerWired` | Integration | Wire a slow fake `summaryGenerator` (blocks on a channel); assert `Destroy()` returns before the fake unblocks — **[new]**, direct test of the NFR "No teardown latency impact" / FR-5's "never blocks or delays session teardown" |
| FR-6: Minimal-activity sessions (AC-6) | `session/session_summary_markdown_test.go` | `RenderSessionSummaryMarkdown_should_RenderEmptyStateText_When_AllSnapshotsZero` | Unit | Happy path — **[plan, Task 1.5.1b]** |
| FR-6: Trivial-threshold boundary (AC-6) | `session/session_summary_snapshot_test.go` | `isTrivialSession_should_ReturnFalse_When_DurationEqualsExactly30Seconds` | Unit | Error/boundary path — **[new]**, boundary-condition test of `trivialSessionMaxDuration` from Task 1.4.2a not explicitly enumerated in Task 1.3.4's list |
| FR-6: Minimal-activity end-to-end persist | `session/session_summary_service_test.go` | `GenerateAndPersist_should_ProduceReadyRowWithFallbackNarrative_When_TrivialSessionEndsImmediately` | Integration | Ent in-memory client; asserts no LLM call recorded on fake pool — **[plan, Task 1.5.2d "trivial session" case]** |
| FR-7: Idempotency/concurrency (AC-8) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_RejectSecondCall_When_FirstCallHoldsInFlightGuard` | Unit | Happy path (dedup succeeds) — **[plan, Task 1.5.2d / 1.5.3a]** |
| FR-7: Sequential-duplicate short-circuit, true duplicate (AC-8) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_SkipRegeneration_When_ReadyRowAndDiffCountsUnchanged` | Unit | Happy path (short-circuit fires) — **[plan, Task 1.5.2d]** |
| FR-7: Sequential-duplicate short-circuit bypassed for resumed sessions (AC-8) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_ProceedWithRegeneration_When_ReadyRowButDiffCountsDiffer` | Unit | Edge path — resume-then-reexit with a genuinely larger diff proceeds with regeneration despite the row already being `READY` — **[new, closes pre-mortem P1 #5, plan.md Task 1.5.2b step 1b / Task 1.5.2d]** |
| FR-7: Panic safety (AC-8) | `session/session_summary_service_test.go` | `GenerateAndPersist_should_RecoverFromPanicAndReleaseGuard_When_BuilderPanics` | Unit | Error path — **[plan, Task 1.5.2d "panic recovery"]** |
| FR-7: Regenerate dedup at RPC layer (AC-8) | `server/services/session_summary_service_test.go` | `RegenerateSessionSummary_should_NotTriggerSecondPoolCallBlocking_When_RowAlreadyGenerating` | Integration | Ent in-memory client + fake pool; asserts call count stays at 1 — **[plan, Task 2.2.2b]** |
| Migration: additive-only, reversible | `session/ent/session_summary_migration_test.go` | `migration_should_be_reversible` | **Migration** | See dedicated section below |

## Migration Test Detail

**File**: `session/ent/session_summary_migration_test.go`
**Test**: `migration_should_be_reversible`
**Type**: Migration (integration-adjacent, file-backed SQLite — mirrors the existing
pattern in `session/backlog_stuck_migration_test.go` rather than `enttest`'s
in-memory shared-cache DSN, since this test needs to inspect `sqlite_master`
directly after a manual `DROP TABLE`, which is easier against a real file than a
`:memory:` connection that could be torn down between steps).

Asserts, in order:
1. **Create**: open a fresh SQLite file, run `entClient.Schema.Create(ctx)` (the
   same auto-migrate call path referenced in plan.md's Migration Plan section);
   query `sqlite_master` and assert a `session_summaries` table now exists.
2. **Write + read back**: insert one `SessionSummary` row (`session_id: "sess-migration-test"`, `status: "ready"`); read it back by `session_id` and assert
   field equality — proves the generated ent code round-trips correctly, not just
   that the table exists.
3. **No FK to `sessions`**: query `PRAGMA foreign_key_list(session_summaries)` (or
   inspect `sqlite_master`'s `sql` column for the `CREATE TABLE` statement) and
   assert the result set is empty / contains no reference to a `sessions` table —
   this is the explicit "assert no FK exists" check called for in Step 5, proving
   the Pattern Decisions table's "plain (non-edge) unique `session_id` string
   field" design was actually implemented, not just documented.
4. **Rollback**: `DROP TABLE session_summaries`; re-query `sqlite_master` and
   assert (a) the table is gone, and (b) the `sessions` table (and any other
   pre-existing table) is unaffected — row counts/schema of `sessions` unchanged
   before vs. after the drop, proving the "additive-only, no cascading impact"
   claim in the Migration Plan section.

## UX Acceptance Tests

Maps 1:1 to the 14 numbered criteria in `project_plans/session-completion-summary/design/ux.md`'s "UX Acceptance Criteria (human-testable)" section. Where plan.md's Task 4.2.1a
(`tests/e2e/session-completion-summary.spec.ts`) already commits to covering a
criterion at the e2e layer, that test is used as-is (do not rename). Criteria that
Task 4.2.1a doesn't reach — because e2e test-mode sessions aren't deleted
mid-suite (AC-7's post-deletion case, per plan.md's own note), or because forcing
a genuine backend `ERROR` state end-to-end is impractical, or because the
assertion is about a CSS media-query / axe-audited property rather than a click
flow — are covered at the component (Jest+RTL) layer instead, per plan.md's own
Task 3.1.2c test list, or by the existing repo-wide Axe Core CI gate. Tool column
follows the `ui-playwright` skill's model for browser-automation specs (Playwright
via `tests/e2e/`, feature-annotation header, `data-testid`/ARIA locators, no
`waitForTimeout`).

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| 1 | Open Summary tab in 1 click → READY or GENERATING visible | `tests/e2e/session-completion-summary.spec.ts` | `summaryTab_should_ShowReadyOrGeneratingDocument_When_ClickedOnce` | Playwright | **[plan, Task 4.2.1a]** Create a trivial one-off session via the UI, stop it, click the now-enabled `Summary` tab (`getByRole('tab', {name: 'Summary'})`), assert either the skeleton (`data-testid="summary-skeleton-block"`) or the READY heading is visible with no extra navigation |
| 2 | Copy markdown in 1 click, no dialog | `tests/e2e/session-completion-summary.spec.ts` | `copyButton_should_CopyMarkdownToClipboard_When_ClickedOnce` | Playwright | **[plan, Task 4.2.1a]** Wait for READY via `expect(locator).toBeEnabled()` on the Copy button (no `waitForTimeout`); click `getByRole('button', {name: 'Copy summary as Markdown'})`; assert the `aria-live` success announcement text equals `"Summary copied to clipboard."` |
| 3 | Regenerate in 1 click from ERROR, no confirmation dialog | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_TriggerRegenerateWithNoConfirmationDialog_When_RegenerateClickedFromErrorState` | Jest+RTL | Forcing a genuine backend `ERROR` is impractical in the isolated e2e server; mock `useSessionSummary` to return `status: ERROR`, click `getByRole('button', {name: /Regenerate/})`, assert `regenerate()` was called exactly once and no `window.confirm`/modal was invoked |
| 4 | Reach summary after deletion in 1 navigation | `web-app/src/app/sessions/[sessionId]/summary/__tests__/page.test.tsx` | `SummaryRoutePage_should_RenderPanelFromRouteParamAlone_When_NoLiveSessionOrReduxState` | Jest+RTL | **[plan, Task 3.3.1b]** — mocks `next/navigation`'s `useParams`; asserts the page renders `SessionSummaryPanel` with `sessionId` from the URL and never calls `useAppSelector(selectAllSessions)`; per Task 4.2.1a's own note, this is the AC-7 post-deletion path and is *not* independently re-verified at the e2e layer (isolated test-mode sessions aren't deleted mid-suite) |
| 5 | ERROR (no stale doc) shows plain-language sentence + sole Regenerate action | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_RenderBareErrorCardWithRegenerate_When_StatusErrorAndMarkdownEmpty` | Jest+RTL | **[plan, Task 3.1.2c]** Mock `status: ERROR`, `error_stage: "decisions"`, `markdown: ""`; assert rendered text is `"Failed while computing approval decisions."` (not the raw enum), Regenerate is the only actionable control, raw `error_message` is only inside the collapsed `<details>` |
| 6 | Copy failure shows exact fallback text + text remains selectable | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_ShowSelectManualCopyMessage_When_ClipboardWriteRejects` | Jest+RTL | **[plan, Task 3.1.2c]** Mock `copy()` to reject; assert inline text `"Copy failed — select the text below and copy manually."`, `aria-live` text `"Copy failed. Select the text and copy manually."`, and the rendered markdown body remains present/selectable in the DOM |
| 7 | No dead ends across every error/edge state | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_RenderActionableExitPath_When_EachErrorOrEdgeStateRenders` | Jest+RTL | Table test over 4 states (ERROR-no-doc, ERROR-with-stale-doc, copy-failure, standalone-route-empty-result); asserts each renders ≥1 focusable actionable element (Regenerate / Try again / Back link / manual-selection affordance) |
| 8 | Trivial session's 5 empty-state strings, same slot, never de-emphasized | `tests/e2e/session-completion-summary.spec.ts` | `summaryDocument_should_ShowEmptyStateStringsInNormalSlots_When_TrivialSessionEnds` | Playwright | **[plan, Task 4.2.1a]** Trivial one-off session created and stopped via the UI; assert the READY document shows the "no work recorded" narrative fallback and each of the 5 section headings is present (not omitted) |
| 9 | Regenerate disables immediately; second click while disabled is inert | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_DisableRegenerateAndIgnoreSecondClick_When_RegenerationInFlight` | Jest+RTL | **[plan, Task 3.1.2c]** Click Regenerate, assert immediate `disabled` + `"Regenerating…"` label before the mocked RPC resolves; fire a second click while disabled, assert `regenerate()` was called exactly once |
| 10 | Every interactive control keyboard-operable, no mouse-only affordance | `tests/e2e/session-completion-summary.spec.ts` | `summaryPanelControls_should_BeOperableViaKeyboardAlone_When_TabbingThroughPanel` | Playwright | **[new]** — not in Task 4.2.1a's list; `Tab` through the panel (`page.keyboard.press('Tab')`) reaching Copy/Regenerate/Details/Back, activate each via `Enter`/`Space`, assert the same effect as a click (e.g. `aria-live` announcement fires) |
| 11 | Single shared `aria-live` region, exact strings per event | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_AnnounceExactAriaLiveStrings_When_StateTransitionsOccur` | Jest+RTL | **[plan, Task 3.1.2c]** Table test asserting the one `aria-live="polite"` node's text content for each of: generating start, ready, copy success/fail, regenerate start/success/fail — exact strings from `design/ux.md`'s table, and that only one such node exists in the DOM |
| 12 | Color paired with text/icon label, never color alone | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `DecisionsAtAGlanceCard_should_PairIconWithTextLabel_When_RenderingEachDecisionCategory` | Jest+RTL | **[new]** For each of the 5 decision categories, assert the rendered node contains both an icon (✓/✕/◔/●) and a text label (e.g. "5 auto-approved"), not a color-only swatch |
| 13 | 4.5:1 contrast, light + dark theme | Repo-wide Axe Core CI gate (per `CLAUDE.md` "UX analysis CI... Axe Core (blocks on WCAG AA violations)") | N/A — automated audit, not a hand-written test | Axe Core (CI) | No new test needed: this criterion is already enforced by the existing CI gate on any PR touching `web-app/src/` (which this feature's PR will), since `SessionSummaryPanel.tsx`/`.css.ts` introduce no new hardcoded colors (per `.claude/rules/css-architecture.md`, all values sourced from `theme.css.ts` tokens) |
| 14 | Skeleton shimmer respects `prefers-reduced-motion: reduce` | `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` | `SessionSummaryPanel_should_RenderStaticSkeletonBlocks_When_PrefersReducedMotionEnabled` | Jest+RTL | **[new]** Mock `window.matchMedia('(prefers-reduced-motion: reduce)').matches = true`; assert skeleton blocks render without the shimmer animation class while GENERATING |

## Test Stack

- **Unit**: Go `testing` + table-driven tests — confirmed this repo's convention via
  `session/backlog_lifecycle_test.go` (e.g. `TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToReview`, table-driven subtests). This validation plan's Go test names use the `Type_Method_should_ExpectedBehavior_When_Condition` shape (e.g. `OnLifecycleEvent_should_DispatchGenerateAndPersist_When_...`), matching the existing `TestXxx_should_Yyy_When_Zzz` idiom used across this package. Jest + React Testing Library for frontend (`SessionSummaryPanel.test.tsx`, `useSessionSummary.test.ts`), matching `web-app/src/lib/hooks/*.test.ts` conventions already in the repo.
- **Integration**: Go, using ent's SQLite test client. Plan.md Task 1.5.2d specifies
  "uses ent's in-memory SQLite test client, matching existing ent test setup
  convention elsewhere in this package" — the generated helper for this is
  `session/ent/enttest/enttest.go` (`enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")`), which exists in the ent-generated tree today but has no
  current callers (confirmed via `grep -rln "ent/enttest" session server` returning
  no results) — this feature's tests are the first consumer. The Migration test
  uses a file-backed SQLite DB instead (mirrors `session/backlog_stuck_migration_test.go`), since it needs to inspect `sqlite_master`/`PRAGMA foreign_key_list`
  after a manual `DROP TABLE`, which a shared in-memory connection makes harder to
  sequence reliably across steps.
- **E2E / UX**: Playwright, per `tests/e2e/` conventions
  (`.claude/rules/e2e-test-conventions.md` — feature annotation header
  `// @feature session-summary-tab, session-summary-standalone-route`, no
  `waitForTimeout`, `data-testid`/ARIA locators only). Modeled on the `ui-playwright`
  skill's browser-automation approach: locate via role/testid, wait via
  `expect(locator).toBeEnabled()`/`toHaveText()` rather than fixed sleeps.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods (`GenerateAndPersist`, `BuildDiffSnapshot`,
  `BuildDecisionsSnapshot`, `BuildCostSnapshot`, `RenderSessionSummaryMarkdown`,
  `GetSessionSummary`, `RegenerateSessionSummary`, `useSessionSummary`): happy
  path + error paths covered per the Requirement → Test Mapping table above.
- All external integrations (ent/SQLite persistence, `headless.Pool.CallBlocking`,
  `notifications.NotificationHistoryStore`, `tokens.TokenStore`, the
  `ReviewQueueLookup` DB call, `navigator.clipboard`): unit mocked + at least one
  integration test each, per the table above.
- UX acceptance criteria: all 14 criteria in `design/ux.md` have a corresponding
  test (11 new/component/e2e tests + 1 route test + 1 already-planned e2e test +
  1 covered by the existing repo-wide Axe Core CI gate rather than a new
  hand-written test, per row 13 above).
- Migration: `migration_should_be_reversible` covers create → write/read-back →
  no-FK assertion → rollback, per Step 5's requirement.
