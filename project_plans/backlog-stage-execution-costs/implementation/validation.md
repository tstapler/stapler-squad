# Validation Plan: backlog-stage-execution-costs

**Date**: 2026-09-17

## Happy Path Scenario

Given a backlog item on pipeline mode `cheap-triage` whose `StageExecutors`
map configures `StageRoleTriage → {Model: "claude-haiku-4-5"}` (set via the
`/settings/pipeline-modes` stage-executor table), when the item's triage
stage runs, then `TriggerTriage` resolves that override through
`PipelineEngine.ExecutorFor` and calls `headless.CallOptions{Model:
"claude-haiku-4-5"}`, the resulting `ItemSession` persists
`resolved_model`/`executor_snapshot_hash`/`cost_priced`, and that cost
appears as its own `"triage"` entry in `StageCostChart` on `/insights`,
summing exactly into the page's existing global total cost.

## Naming conventions used

- **Go**: `TestType_should_ExpectedBehavior_When_Condition`, matching this
  repo's dominant convention (verified against
  `session/pipeline_engine_test.go`, `session/ent_repository_backlog_test.go`
  — e.g. `TestPipelineModeCache_Get_should_ReturnFalse_When_SlugNotPresent`,
  `TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields`).
- **Jest**: `it("ComponentName_should_expectedBehavior_When_condition", ...)`,
  matching `web-app/src/app/insights/ModelBreakdownChart.test.tsx`.
- **Playwright e2e**: descriptive `test()` names inside a `test.describe()`
  block, `// @feature` header, `data-testid`/ARIA locators only, no
  `waitForTimeout`, page helpers under `tests/e2e/pages/` — per
  `e2e-test-conventions` skill and existing specs (e.g.
  `tests/e2e/backlog-board-live-updates.spec.ts`).

---

## Requirement → Test Mapping

Requirements are `requirements.md`'s `## Scope → In Scope` bullets (7 total).

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: Extend `PipelineMode` with per-stage `PipelineStageExecutor` overrides | `session/pipeline_mode_executor_test.go` | `TestSerializeStageExecutors_should_RoundTripThroughJSON_When_MapHasAllThreeRoles` | Unit | Happy path — `StageExecutors{StageRoleTriage:{Model:"claude-haiku-4-5"}}` marshals to `{"triage":{"program":"","model":"claude-haiku-4-5"}}` and back |
| REQ-1 | `session/pipeline_mode_executor_test.go` | `TestParseStageExecutors_should_ReturnErrorNotPanic_When_JSONIsMalformed` | Unit | Error path — malformed `stage_executors_json` returns a non-nil error, not a zero-value map masquerading as "no override" |
| REQ-1 | `session/ent_pipeline_mode_repository_test.go` | `TestEntPipelineModeRepository_should_RoundTripStageExecutorsJSON_When_CreatedWithTriageOverride` | Integration (ent/sqlite) | `PipelineModeCreateInput{StageExecutors: map[StageRole]PipelineStageExecutor{StageRoleTriage:{Model:"claude-haiku-4-5"}}}` persists and `ParseStageExecutors(row.StageExecutorsJSON)` reads the same value back |
| REQ-2: Thread per-stage program/model override into headless + work-stage execution | `session/pipeline_engine_test.go` | `TestCachingPipelineEngine_ExecutorFor_should_ReturnConfiguredModel_When_RoleHasOverride` | Unit | Happy path — resolved mode with a `"triage"` entry returns `("", "claude-haiku-4-5")` for `StageRoleTriage` |
| REQ-2 | `session/pipeline_engine_test.go` | `TestCachingPipelineEngine_ExecutorFor_should_ReturnEmptyAndWarnLog_When_PipelineModeSlugUnresolved` | Unit | Error/fallback path — unresolved slug returns `("", "")` and emits the `[PipelineEngine] unresolved pipeline_mode=...` Warn log |
| REQ-2 | `server/services/backlog_service_triage_test.go` | `TestSpawnSessionFromItem_should_SetInstanceProgramViaInstanceOptions_When_WorkStageOverrideConfigured` | Integration (fake `SessionCreator`/session spawn path) | `stage_executors["work"] = {model:"family:sonnet"}` resolves to `Instance.Program == "claude --model claude-sonnet-4-6"` via `InstanceOptions`, never via a post-hoc `SwitchProgram`/`Restart`, and the item's kickoff prompt is still delivered exactly once |
| REQ-3: Headless adapters for feasible non-Claude programs (Gemini); fail-closed fallback for infeasible ones (Aider) | `session/headless/gemini_caller_test.go` | `TestGeminiCaller_CallBlocking_should_InvokeSinkWithPricedTrue_When_ResponseHasKnownModelTokens` | Unit | Happy path — stubbed `gemini -p ... --output-format json` response with `gemini-2.5-pro` token usage computes cost and calls `sink(cost, true)` |
| REQ-3 | `session/headless/gemini_caller_test.go` | `TestGeminiCaller_CallBlocking_should_ReturnNonNilError_When_ResponseContainsErrorObject` | Unit | Error path — a response with an `"error"` object returns a non-nil error naming the message, never a silently-successful empty result |
| REQ-3 | `server/services/backlog_service_test.go` | `TestResolveHeadlessCaller_should_FallBackToClaudeWithFallbackReason_When_ProgramAvailableAtWireTimeButGoneAtCallTime` | Integration (registry + `Available()` re-probe) | Gemini wired at startup but uninstalled before a later call — `resolveHeadlessCaller` re-probes via `GeminiCaller.Available()`, falls back to Claude, and returns `configuredProgram="gemini"`, `fallbackReason="gemini_unavailable"` for persistence onto the `ItemSession` row |
| REQ-4: Extend `/settings/pipeline-modes` UI to configure program+model per stage | `web-app/src/app/settings/pipeline-modes/PipelineModeForm.test.tsx` | `PipelineModeForm_should_SubmitStageExecutorsMap_When_TriageModelFieldSetAndProgramLeftBlank` | Unit (Jest/RTL) | Happy path — setting only the Triage Model field submits `stageExecutors: {triage: {program:"", model:"claude-haiku-4-5"}}` |
| REQ-4 | `web-app/src/app/settings/pipeline-modes/PipelineModeForm.test.tsx` | `PipelineModeForm_should_ShowInlineErrorNamingProgramAndStage_When_ServerRejectsAiderForTriage` | Unit (Jest/RTL) | Error path — a `CodeInvalidArgument` response naming `"aider"`/`"triage"` renders next to the Triage row, not a generic toast, and other field values remain intact |
| REQ-4 | `server/services/backlog_service_pipeline_mode_test.go` | `TestCreatePipelineMode_should_PersistStageExecutorsAndReturnInResponse_When_ValidTriageOverrideProvided` | Integration (RPC + ent) | `CreatePipelineModeRequest{StageExecutors:{"triage":{model:"claude-haiku-4-5"}}}` round-trips through the full Create RPC → repository → response path |
| REQ-5: Cost aggregation treats `SessionRole` as a first-class breakdown dimension | `server/services/insights_service_test.go` | `TestSessionMetaForSessions_should_RetainItemIDAndItemTitle_When_BuildingMapFromBacklogEntries` | Unit | Happy path — the renamed/extended lookup carries `ItemID`/`ItemTitle` alongside `Role`, sourced from data `GetAllItemSessionsWithBacklogInfo` already returns |
| REQ-5 | `server/services/insights_service_test.go` | `TestGetInsightsSummary_RoleBreakdown_should_ExcludeUnpricedSessionFromCostSum_When_SessionSummaryReportsUnpricedModel` | Unit | Error/edge path — a session with `len(summary.UnpricedModels) > 0` increments that role's `unpriced_session_count` and is excluded from `estimated_cost_usd`/`total_cost_usd`, not silently folded in as `$0` |
| REQ-5 | `server/services/insights_service_test.go` | `TestGetInsightsSummary_should_SumRoleBreakdownToTotalCost_When_MultipleRolesPlusUnattributedSessionPresent` | Integration (fixture-backed token store) | 3 attributed sessions (triage/review/work) plus 1 unattributed ad hoc session — `sum(role_breakdown[].estimated_cost_usd) == total_cost_usd`, unattributed session buckets under `session_role: ""` |
| REQ-6: New Insights view — bar chart by stage/role, drillable by item, consistent totals | `web-app/src/app/insights/StageCostChart.test.tsx` | `StageCostChart_should_RenderBarsSortedDescendingByCost_When_ThreeRoleBreakdownEntriesProvided` | Unit (Jest/RTL) | Happy path — `[triage $0.50, review $1.20, work $14.30]` renders bars in order work, review, triage |
| REQ-6 | `web-app/src/app/insights/StageCostChart.test.tsx` | `StageCostChart_should_ShowNoDataCard_When_RoleBreakdownIsEmpty` | Unit (Jest/RTL) | Error/edge path — an all-empty `role_breakdown` shows the same `"No data"` `chartCard` state as `ModelBreakdownChart`, never a broken/blank chart |
| REQ-6 | `server/services/insights_service_test.go` | `TestGetInsightsSummary_should_IncludeGeminiPricedSessionWithNoTranscript_When_ItemSessionHasNoMatchingParseResult` | Integration | An `ItemSession` row for a Gemini-executed triage call with no matching Claude transcript still contributes to `total_cost_usd` and `role_breakdown`/`items[]` (Story 4.1.4's transcript-less fold-in), and a Claude session with a matching transcript is not double-counted |
| REQ-7: Soft budget warning — configurable per-item threshold, visible when crossed | `session/budget_warning_test.go` | `TestEvaluateBudgetThreshold_should_ReturnWarnTrue_When_CumulativeSpendMeetsOrExceedsThreshold` | Unit | Happy path — `cumulativeSpentUSD >= *thresholdUSD` returns `warn=true` |
| REQ-7 | `session/budget_warning_test.go` | `TestEvaluateBudgetThreshold_should_ReturnWarnFalse_When_ThresholdIsNil` | Unit | Error/edge path — no threshold configured never fires a warning, regardless of spend |
| REQ-7 | `server/services/backlog_service_triage_test.go` | `TestTriggerTriage_should_EmitBudgetWarningLogLine_When_CostCrossesItemThreshold` | Integration (`CostSink` + repository cumulative-cost lookup) | Item with `cost_budget_threshold_usd: 5.00` and prior spend `$4.90`; a `$0.15` triage call crosses to `$5.05` and emits `[BudgetWarning] item=... stage=triage threshold=5.00 spent=5.05` without affecting the call's own success/failure status |

**Coverage: 7/7 requirements (100%)** — each has 1 happy-path unit test, 1 error-path unit test, and 1 integration test (all 7 requirements involve ent/`PipelineMode`, headless subprocess calls, or Insights aggregation, per the task brief).

---

## UX Acceptance Tests

18 criteria per `design/ux.md`'s "UX Acceptance Criteria" section (4 task-completion + 6 error/edge + 6 accessibility + 2 consistency). All are Playwright e2e specs per this repo's `ui-playwright`/`e2e-test-conventions` model: `// @feature` header, `data-testid`/ARIA locators, no `waitForTimeout`, page helpers in `tests/e2e/pages/`.

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| 1 | Configure per-stage program+model for all 3 stages in one form submission, 0 extra navigations | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `configures triage, review, and work stage executors in a single form submission` | Playwright | Open `/settings/pipeline-modes`, edit existing mode (no nav away); set Program/Model for all 3 rows; Save; assert `updatePipelineMode` request body contains all 3 role keys and no page navigation occurred |
| 2 | View stage/role cost breakdown in ≤1 page load, no extra click | `tests/e2e/insights-stage-cost-chart.spec.ts` | `renders stage cost chart alongside model breakdown on initial insights page load` | Playwright | Navigate to `/insights` once; assert `StageCostChart`'s `data-testid` is visible with no prior interaction |
| 3 | Drill from stage → sessions in exactly 1 click or 1 keyboard activation | `tests/e2e/insights-stage-cost-chart.spec.ts` | `clicking the work bar filters the sessions table to work-role sessions in one click` (+ keyboard variant `activating the work legend entry via Tab and Enter filters sessions identically to a click`) | Playwright | Click "work" bar (or Tab to legend entry + Enter); assert `SessionsTable` rows all show `sessionRole === "work"` with no intermediate modal/page |
| 4 | Opt into per-item budget warning in ≤1 form field edit, no separate enable toggle | `tests/e2e/item-budget-warning.spec.ts` | `setting the budget threshold input alone opts into the per-item warning with no separate toggle` | Playwright | Open item detail, set threshold input, save; assert no other control was required and the warning banner slot exists in the DOM (visible once threshold is crossed) |
| 5 | Aider for Triage/Review names both the rejected program and stage, with a corrective action | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `rejecting aider for the triage stage names both "aider" and the stage in the error message` | Playwright | Set Triage Program = `aider`, Save; assert error banner text contains `"aider"` and `"triage"` and a corrective action (clear field) is available |
| 6 | Aider for Work stage succeeds with no error | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `accepting aider for the work stage program succeeds with no error banner` | Playwright | Set Work Program = `aider`, Save; assert success (form closes/returns to list) and no error banner rendered |
| 7 | Unrecognized model string rejected, naming the exact value and stage | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `rejecting an unrecognized model id names the exact value and the affected stage` | Playwright | Set Review Model = `claude-opus-9000`, Save; assert error text contains `"claude-opus-9000"` and `"Review"` |
| 8 | Every error state has a visible exit path — no reload, no lost edits | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `recovering from a rejected stage-executor save preserves all other in-progress field edits` | Playwright | Fill Name + Description + a valid Work row, trigger a Triage/aider rejection, correct only the Triage field, re-save; assert Name/Description/Work values are unchanged and no full-page reload occurred (`page.url()` unchanged, no `load` event refire) |
| 9 | Zero-session role omitted from chart — never a zero-height/"$0" bar | `tests/e2e/insights-stage-cost-chart.spec.ts` | `omits a zero-session role from the stage cost chart entirely rather than showing a zero bar` | Playwright | Seed data with no `"review"` sessions in range; assert chart renders only bars with `data-testid` matching roles present, no `review` bar element exists at all |
| 10 | Item with no configured budget threshold shows no warning UI anywhere | `tests/e2e/item-budget-warning.spec.ts` | `shows no budget warning UI when no threshold is configured for the item` | Playwright | Open detail view for an item with `costBudgetThresholdUsd` unset; assert the `ItemBudgetWarning` banner element is entirely absent (not present-but-hidden) |
| 11 | Every stage-executor table input has a unique, descriptive `aria-label`/`<label>` | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `each stage executor input exposes a unique aria-label distinguishing stage and field type` | Playwright | Query all 6 inputs by role; assert each accessible name is unique and matches `"<Stage> stage <program|model>"` |
| 12 | Stage-executor table uses semantic `<table>`/`<th scope="col">`, not styled `div`s | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | `stage executor table uses semantic table and th scope=col markup` | Playwright | Assert the DOM contains a `<table>` ancestor with `<th scope="col">` for Stage/Program/Model headers |
| 13 | `StageCostChart` wrapper has `role="img"` and `aria-label` matching rendered bar values | `tests/e2e/insights-stage-cost-chart.spec.ts` | `stage cost chart wrapper role=img aria-label matches the visually rendered bar values` | Playwright | Read the chart wrapper's `aria-label`; assert it contains each visible role/cost pair exactly as rendered, generated from the same sorted data array |
| 14 | Every clickable legend entry reachable via Tab, activatable via Enter and Space | `tests/e2e/insights-stage-cost-chart.spec.ts` | `legend entries are reachable via Tab and activatable via both Enter and Space` | Playwright | Tab to each legend entry; press Enter on one, Space on another; assert both fire the identical `onRoleClick` cross-filter as a mouse click |
| 15 | `SessionsTable`'s existing role/search breakdown remains fully usable without ever touching the chart | `tests/e2e/insights-stage-cost-chart.spec.ts` | `sessions table role and text search remain fully usable without any interaction with the chart` | Playwright | Skip clicking the chart entirely; use `SessionsTable`'s own search box to find a role's sessions; assert results match, proving the chart is a convenience path, not the only path |
| 16 | New/changed text meets ≥4.5:1 contrast, no new colors | `tests/e2e/accessibility.spec.ts` | `stage cost chart and item budget warning introduce no new axe color-contrast violations` | Playwright + axe-core (existing suite) | Run the existing Axe Core scan against `/insights` and a backlog item detail page with `StageCostChart`/`ItemBudgetWarning` rendered; assert zero new violations beyond the pre-existing baseline |
| 17 | Bar values sum to exactly the existing global total for the same time range | `tests/e2e/insights-stage-cost-chart.spec.ts` | `stage cost chart bar values sum to the total cost card for the same time range` | Playwright | Read each bar's displayed dollar value and the page's total-cost card value for the same range; assert the sum matches exactly (mirrors the backend `sum(role_breakdown[].estimated_cost_usd) == total_cost_usd` invariant, verified visually here) |
| 18 | Global (`ProjectedCostCard`) and per-item (`ItemBudgetWarning`) warnings share visual language but stay distinctly labeled | `tests/e2e/item-budget-warning.spec.ts` | `global and per-item budget warnings share consistent warning styling while remaining distinctly labeled` | Playwright | Trigger both warnings (global monthly + per-item); assert both use the same warning color/icon token (computed style comparison) while their text distinguishes "total spend" vs. "this item's spend" |

---

## Migration Test

Per plan.md's Migration Plan: this repo has no versioned SQL migration
files — ent applies schema changes via `client.Schema.Create(ctx)` at
process startup (`session/ent_repository.go:187`). "Migration up" here means
"the new `.Optional().Default(...)` columns appear on restart and
pre-existing rows read back with safe defaults"; "rollback" means "a
reverted binary tolerates the now-unused extra columns." This mirrors the
existing precedent test `TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields`
(`session/ent_repository_backlog_test.go`).

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `session/ent_repository_backlog_test.go` | `TestMigrationShouldBeReversible_WhenPipelineModeAndItemSessionGainStageExecutorFields` | Integration (ent/sqlite, schema-create round trip) | (1) **Up**: create a `PipelineMode` row and an `ItemSession` row against the pre-project schema shape (omit `stage_executors_json`/`resolved_program`/`resolved_model`/`executor_snapshot_hash`/`configured_program`/`executor_fallback_reason`/`cost_priced`); run `client.Schema.Create(ctx)` with the new schema; assert the existing rows now read `stage_executors_json == "{}"` (parses to an empty map via `ParseStageExecutors`, no error), `cost_priced == true` (existing Claude-only cost data is retroactively priced), and the other new string fields default to `""`. (2) **Behavior parity**: assert `PipelineEngine.ExecutorFor` on that pre-existing row returns `("", "")` for every role — identical to today's un-migrated behavior, no `PipelineMode` treated as if it had overrides it never configured. (3) **Rollback safety**: simulate a reverted binary by round-tripping the row through the pre-project `PipelineModeCreateInput`/`ItemSession` accessors that don't reference the new fields at all; assert no error and no data loss on the fields those accessors do use — confirming the additive-only column set is safe for a binary that doesn't know about it |

**Migration test: Yes.**

---

## Test Stack

- **Unit**: Go `testing` + `testify` (existing convention, e.g.
  `session/pipeline_engine_test.go`); Jest + React Testing Library for
  frontend components (`web-app/src/app/**/*.test.tsx`), run via `cd
  web-app && npx jest --no-coverage`.
- **Integration**: Go tests against an in-memory/sqlite `ent` client
  (existing `session/ent_repository_backlog_test.go` pattern) and
  RPC-level `server/services/*_test.go` tests using fake `SessionCreator`/
  `headless.PoolClient` doubles, run via `go test ./server/services
  ./session/... -timeout=20m` (per this repo's `gotestsum`-wrapped `make
  test`/`test-integration`).
- **E2E / UX**: Playwright specs in `tests/e2e/`, run via `cd tests/e2e &&
  npm test` or targeted `npx playwright test <spec>.spec.ts`; axe-core scan
  extended in the existing `accessibility.spec.ts`.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this project (`ExecutorFor`,
  `resolveHeadlessCaller`, `GeminiCaller.CallBlocking`,
  `EvaluateBudgetThreshold`, `GetInsightsSummary`'s new accumulators):
  happy path + error paths covered per the Requirement → Test Mapping above.
- All external integrations (Gemini subprocess call, ent schema
  read/write): unit mocked (`session/headless/gemini_caller_test.go`'s
  stubbed subprocess) **and** at least one integration test
  (`server/services/backlog_service_test.go`'s registry test;
  `session/ent_pipeline_mode_repository_test.go`'s round-trip).
- UX acceptance criteria: all 18 criteria in `design/ux.md` have a
  corresponding Playwright test above — no manual-only steps remain.
- `dupl`/`jscpd` duplication gates: `StageCostChart.tsx` reuses
  `ModelBreakdownChart.css.ts`'s classes rather than duplicating them (per
  plan Task 5.2.1a) — verify with `make ready-duplication-gate-web` before
  shipping Epic 5.2.
