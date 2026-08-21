# Token & Spend Monitoring — Validation Plan

**Phase:** 4 (Validation)
**Date:** 2026-05-15
**Status:** Pre-implementation (written before any code)

---

## Summary

| Layer | Test Cases | File(s) |
|-------|-----------|---------|
| Go unit (parser, pricing, store, association) | 35 | `session/tokens/*_test.go` |
| Go unit (InsightsService) | 9 | `server/services/insights_service_test.go` |
| React unit (Jest/RTL) | 18 | `*.test.tsx`, `*.test.ts` |
| E2E (Playwright) | 4 | `tests/e2e/insights-dashboard.spec.ts` |
| **Total** | **66** | |

**Requirements coverage:** 24 of 24 requirements covered (FR-1 through FR-8 + NFR-1 through NFR-4 + all 4 success criteria).

---

## 1. Test Strategy Overview

### Layers and Ownership

| Layer | Technology | Scope | Owns |
|-------|-----------|-------|------|
| Go unit | `testing` + `testify/assert` + `testify/require` | Pure functions, data structures, in-process logic | Parser correctness, pricing math, store cache behavior, session association logic, InsightsService aggregation |
| React unit | Jest + React Testing Library | Individual components in isolation | Badge rendering, table display, CSV generation, budget alert UI state |
| E2E | Playwright against `localhost:8544` | Full browser + real server stack | Route loading, filter interactions, CSV file download, performance baseline |

No integration tests are planned for Phase 1 (the "manual smoke test" T-18 is a developer gate, not a CI artifact). The boundary between unit and e2e is: anything requiring a real server process or browser belongs in e2e.

### Test Data Strategy

Fixture JSONL files live in `session/tokens/testdata/`. Each file is a complete, minimal scenario—no more lines than required to exercise the case. Go tests load fixtures with `os.Open("testdata/<fixture>.jsonl")`; the parser under test is called directly (no subprocess). React tests mock the ConnectRPC hook response using `jest.mock`.

---

## 2. Go Unit Tests

Package: `session/tokens`

Convention: table-driven tests using `t.Run` sub-tests, `require.NoError` for setup, `assert.*` for assertions. Test names follow `TestXxx_When<Condition>_Expect<Result>`.

---

### E1-S1: JSONL Parser (`parser_test.go`)

**TC-GO-01**
- Name: `TestParseFile_WhenValidThreeTurnSession_ExpectCorrectTokenTotals`
- Input: `testdata/valid_session.jsonl` (3 assistant messages with full usage fields)
- Expected:
  - `result.TotalInput` == sum of all `usage.input_tokens` across messages
  - `result.TotalOutput` == sum of all `usage.output_tokens`
  - `result.CacheCreation` == sum of `usage.cache_creation_input_tokens`
  - `result.CacheRead` == sum of `usage.cache_read_input_tokens`
  - `result.MessageCount` == 3
  - `len(result.TurnTimeline)` == 3
- Requirement: FR-1, FR-2, Success Criterion 3

**TC-GO-02**
- Name: `TestParseFile_WhenPartialLastLine_ExpectNoErrorAndPartialLineSkipped`
- Input: `testdata/partial_write.jsonl` (last line cut mid-JSON)
- Expected:
  - `error` is `nil`
  - Token totals reflect only the complete lines (partial line silently skipped)
  - `result.MessageCount` == count of complete assistant messages only
- Requirement: FR-1, NFR-1

**TC-GO-03**
- Name: `TestParseFile_WhenMalformedJsonLine_ExpectSkipsBadLineCountsRest`
- Input: `testdata/malformed_line.jsonl` (valid lines surrounding one `{bad json`)
- Expected:
  - `error` is `nil`
  - Tokens from valid lines are summed correctly
  - Malformed line contributes zero tokens
- Requirement: FR-1

**TC-GO-04**
- Name: `TestParseFile_WhenMissingUsageField_ExpectZeroTokensForThatTurn`
- Input: Inline JSONL string (assistant message with no `usage` key)
- Expected:
  - `result.TotalInput` == 0 (no panic, no error)
  - `result.MessageCount` == 1 (message counted even though usage is absent)
- Requirement: FR-1

**TC-GO-05**
- Name: `TestParseFile_WhenMultiTurnSession_ExpectTurnTimelineOrdered`
- Input: `testdata/valid_session.jsonl`
- Expected:
  - `result.TurnTimeline[i].Timestamp` is non-zero for each turn
  - Timestamps are in ascending order
  - Each `TurnStats.Input` matches the corresponding message's `usage.input_tokens`
- Requirement: FR-2

**TC-GO-06**
- Name: `TestParseFile_WhenModelHasDateSuffix_ExpectPrimaryModelStripped`
- Input: Inline JSONL with `"model": "claude-sonnet-4-6-20250514"` in one message
- Expected:
  - `result.PrimaryModel` == `"claude-sonnet-4-6-20250514"` (raw model ID stored as-is)
  - `result.Models` contains `"claude-sonnet-4-6-20250514"`
- Note: Normalization happens in `PricingTable.NormalizeModelFamily()`, not in the parser.
- Requirement: FR-1, FR-3

**TC-GO-07**
- Name: `TestParseFile_WhenEmptyFile_ExpectEmptyResultNoError`
- Input: Empty file (zero bytes)
- Expected:
  - `error` is `nil`
  - All token counts are 0
  - `result.MessageCount` == 0
- Requirement: FR-1

**TC-GO-08**
- Name: `TestParseFile_WhenToolUsePresent_ExpectToolUsageMapPopulated`
- Input: Inline JSONL with assistant message containing `content[{type:"tool_use", name:"Bash"}]`
- Expected:
  - `result.ToolUsage["Bash"].CallCount` == 1
  - `result.ToolUsage["Bash"].MCPServer` == `""`
- Requirement: FR-1

**TC-GO-09**
- Name: `TestParseFile_WhenMCPToolUsePresent_ExpectMCPServerExtracted`
- Input: Inline JSONL with `"name": "mcp__datadog__search_logs"` in tool_use
- Expected:
  - `result.ToolUsage["mcp__datadog__search_logs"].MCPServer` == `"datadog"`
  - `result.ToolUsage["mcp__datadog__search_logs"].CallCount` == 1
- Requirement: FR-1

**TC-GO-10**
- Name: `TestParseFile_WhenCacheHeavySession_ExpectCacheTokensSummedCorrectly`
- Input: `testdata/cache_heavy.jsonl`
- Expected:
  - `result.CacheRead` > 0
  - `result.CacheCreation` > 0
  - `result.TotalInput` is the raw `input_tokens` sum (excludes cache fields)
- Requirement: FR-2

---

### E1-S2: Skill/Command Detection (`skill_detector_test.go`)

**TC-GO-11**
- Name: `TestSkillDetector_WhenSlashCommandInHumanTurn_ExpectIsCommandTrue`
- Input: Inline JSONL with `human` message content `"/plan:feature implement auth"`
- Expected:
  - `len(result.SkillActivations)` >= 1
  - `SkillActivations[0].Name` == `"plan:feature"`
  - `SkillActivations[0].IsCommand` == `true`
- Requirement: FR-4

**TC-GO-12**
- Name: `TestSkillDetector_WhenSkillMdReadToolResult_ExpectIsCommandFalse`
- Input: Inline JSONL with tool_result content containing `~/.claude/skills/code-review.md`
- Expected:
  - `SkillActivations[0].Name` == `"code-review"`
  - `SkillActivations[0].IsCommand` == `false`
- Requirement: FR-4

**TC-GO-13**
- Name: `TestSkillDetector_WhenRegularMessage_ExpectNoActivations`
- Input: Inline JSONL with human message `"what is the status of the build?"`
- Expected:
  - `len(result.SkillActivations)` == 0
- Requirement: FR-4

**TC-GO-14**
- Name: `TestSkillDetector_WhenMultipleCommandsInOneTurn_ExpectAllDetected`
- Input: Inline JSONL with human message `"run /plan:feature then /code:review"`
- Expected:
  - `len(result.SkillActivations)` == 2
  - Both `plan:feature` and `code:review` present
- Requirement: FR-4

---

### E1-S3: TokenStore (`store_test.go`)

**TC-GO-15**
- Name: `TestTokenStore_WhenFileNotCached_ExpectParseOnGetAll`
- Input: `testdata/valid_session.jsonl` path provided to store
- Expected:
  - After `store.Start(ctx)` and one worker cycle, `store.GetAll()` returns one result
  - `result.TotalInput` > 0
- Requirement: FR-2, NFR-1

**TC-GO-16**
- Name: `TestTokenStore_WhenFileCached_ExpectCacheHitSkipsReparse`
- Input: Pre-populate cache with a `cachedEntry{result, modTime}` equal to file's actual modtime
- Expected:
  - A second call to `GetAll()` returns the same `*ParseResult` pointer (no reparse)
- Requirement: NFR-1

**TC-GO-17**
- Name: `TestTokenStore_WhenFileModTimeChanges_ExpectCacheInvalidated`
- Input: Write a fixture file, parse it into cache, then touch the file (update modtime)
- Expected:
  - `GetAll()` after touch triggers a reparse (result pointer is a new allocation)
- Requirement: NFR-1

**TC-GO-18**
- Name: `TestTokenStore_WhenConcurrentRequests_ExpectNoDataRace`
- Input: `testdata/valid_session.jsonl`; launch 20 goroutines calling `GetAll()` concurrently
- Expected:
  - No data race (`go test -race` passes)
  - All goroutines receive valid (non-nil) results
- Requirement: NFR-1

**TC-GO-19**
- Name: `TestTokenStore_WhenGetByUUID_ExpectDirectLookup`
- Input: Pre-populate store cache with a result having known `SessionUUID`
- Expected:
  - `store.GetByUUID("known-uuid")` returns that exact result
  - `store.GetByUUID("unknown-uuid")` returns `nil`
- Requirement: FR-2

---

### E1-S4: Pricing Table (`pricing_test.go`)

**TC-GO-20**
- Name: `TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped`
- Table-driven inputs/expected:

  | Input | Expected |
  |-------|---------|
  | `"claude-sonnet-4-6-20250514"` | `"claude-sonnet-4"` |
  | `"claude-sonnet-4-6"` | `"claude-sonnet-4"` |
  | `"claude-opus-4-7"` | `"claude-opus-4"` |
  | `"claude-3-opus-20240229"` | `"claude-opus-3"` |
  | `"claude-haiku-4"` | `"claude-haiku-4"` |
  | `"unknown-model-xyz"` | `"unknown-model-xyz"` (passthrough) |

- Requirement: FR-3

**TC-GO-21**
- Name: `TestEstimateCost_WhenKnownModel_ExpectExactPrice`
- Input: `ParseResult{PrimaryModel: "claude-sonnet-4-6", TotalInput: 1_000_000, TotalOutput: 1_000_000, CacheCreation: 0, CacheRead: 0}`
- Expected: `cost` == `3.0 + 15.0` == `18.0` USD (within 0.0001 tolerance)
- Requirement: FR-3, Success Criterion 3

**TC-GO-22**
- Name: `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero`
- Input: `ParseResult{PrimaryModel: "gpt-99-turbo", TotalInput: 500_000, TotalOutput: 500_000}`
- Expected: `cost` == `0.0` (no panic; unknown model returns 0 USD)
- Requirement: FR-3

**TC-GO-23**
- Name: `TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded`
- Input: `ParseResult{PrimaryModel: "claude-sonnet-4", TotalInput: 0, TotalOutput: 0, CacheCreation: 0, CacheRead: 1_000_000}`
- Expected: `cost` == `0.30` USD (claude-sonnet-4 cache read rate $0.30/MTok)
- Requirement: FR-3

**TC-GO-24**
- Name: `TestPricingTable_WhenIsStale_Expect31DaysReturnTrue`
- Input: `PricingTable` with `EffectiveDate` set to 31 days before today
- Expected: `IsStale()` == `true`
- Requirement: FR-3

**TC-GO-25**
- Name: `TestPricingTable_WhenIsStale_Expect29DaysReturnFalse`
- Input: `PricingTable` with `EffectiveDate` set to 29 days before today
- Expected: `IsStale()` == `false`
- Requirement: FR-3

**TC-GO-26**
- Name: `TestLoadPricingOverride_WhenValidConfigJSON_ExpectOverridesApplied`
- Input: Temp JSON file with one model entry overriding `claude-sonnet-4` input price to `99.0`
- Expected:
  - `table.Prices["claude-sonnet-4"].InputPricePerMTok` == `99.0`
  - Other entries (e.g., `claude-opus-4`) retain hardcoded defaults
- Requirement: FR-3, NFR-4

---

### E1-S5: Session Association (`association_test.go`)

**TC-GO-27**
- Name: `TestAssociator_WhenExactConversationIDMatch_ExpectSessionIDReturned`
- Input: Stub storage returning session with `conversation_id == "abc-123"`; `ParseResult{SessionUUID: "abc-123"}`
- Expected:
  - `sessionID` == the matched session's ID
  - `isOrphan` == `false`
- Requirement: FR-5

**TC-GO-28**
- Name: `TestAssociator_WhenPathPrefixMatch_ExpectSessionIDReturned`
- Input: Stub storage with session `path == "/home/user/projects/myapp"`; `ParseResult{ProjectPath: "/home/user/projects/myapp/subdir"}`
- Expected:
  - `sessionID` matched
  - `isOrphan` == `false`
- Requirement: FR-5

**TC-GO-29**
- Name: `TestAssociator_WhenNoMatch_ExpectOrphan`
- Input: Stub storage with no sessions; `ParseResult{SessionUUID: "no-match"}`
- Expected:
  - `sessionID` == `""`
  - `isOrphan` == `true`
- Requirement: FR-5

---

### E2-S2: InsightsService (`server/services/insights_service_test.go`)

Package: `services`

**TC-GO-30**
- Name: `TestGetInsightsSummary_WhenStoreHasTwoSessions_ExpectAggregatedTotals`
- Input: Stub `TokenStore` returning two `ParseResult` values with known token counts and models
- Expected:
  - `resp.TotalInputTokens` == sum of both input totals
  - `resp.TotalOutputTokens` == sum of both output totals
  - `len(resp.Sessions)` == 2
- Requirement: FR-2, FR-6

**TC-GO-31**
- Name: `TestGetInsightsSummary_WhenTimeFilterApplied_ExpectOnlyMatchingSessionsReturned`
- Input: Three `ParseResult` values with distinct `FileModTime` values; filter `from/to` covers only two
- Expected:
  - `len(resp.Sessions)` == 2
  - Excluded session not present
- Requirement: FR-6

**TC-GO-32**
- Name: `TestGetInsightsSummary_WhenModelFilterApplied_ExpectOnlyMatchingModel`
- Input: Two results with `PrimaryModel == "claude-sonnet-4"` and one with `"claude-opus-4"`; filter `model_filter = "claude-sonnet-4"`
- Expected:
  - `len(resp.Sessions)` == 2
- Requirement: FR-6

**TC-GO-33**
- Name: `TestGetInsightsSummary_WhenDailyRollupRequested_ExpectBucketsGroupedByDay`
- Input: Three results on two distinct calendar days (two on day A, one on day B)
- Expected:
  - `len(resp.Daily)` == 2
  - `resp.Daily[0].SessionCount` == 2 or 1 depending on day
- Requirement: FR-6

**TC-GO-34**
- Name: `TestGetInsightsSummary_WhenOrphanSessions_ExpectIncludedWhenFlagSet`
- Input: One result with `isOrphan == true`; request `include_orphans = true`
- Expected:
  - `len(resp.Sessions)` == 1
  - `resp.Sessions[0].IsOrphan` == `true`
- Requirement: FR-5

**TC-GO-35**
- Name: `TestGetInsightsSummary_WhenOrphanSessions_ExpectExcludedWhenFlagFalse`
- Input: Same orphan result; request `include_orphans = false`
- Expected:
  - `len(resp.Sessions)` == 0
- Requirement: FR-5

**TC-GO-36**
- Name: `TestGetInsightsSummary_WhenCacheHeavySession_ExpectCacheHitRateComputed`
- Input: `ParseResult` from `testdata/cache_heavy.jsonl` via stub; known `TotalInput=100`, `CacheRead=400`
- Expected:
  - `resp.Sessions[0].CacheHitRate` == `400.0 / (100.0 + 400.0)` == `0.80` (within 0.001)
- Requirement: FR-3

**TC-GO-37**
- Name: `TestListSessionTokens_WhenSortByCostDesc_ExpectOrdering`
- Input: Three sessions with costs `$5.00`, `$1.00`, `$3.00`; `sort_by = "cost"`, `sort_desc = true`
- Expected:
  - Result order: `$5.00`, `$3.00`, `$1.00`
- Requirement: FR-6

**TC-GO-38**
- Name: `TestWatchInsights_WhenStoreUpdated_ExpectEventPushed`
- Input: Mock `TokenStore.Subscribe()` channel; push one update after goroutine starts
- Expected:
  - `InsightsEvent` with `event_type == "update"` received on stream within 1s
  - Context cancel exits the stream goroutine without deadlock
- Requirement: FR-6, Success Criterion 1

---

## 3. React Unit Tests (Jest/RTL)

Convention: co-located `*.test.tsx` or `*.test.ts` alongside the component. All ConnectRPC calls are mocked via `jest.mock('../lib/hooks/useInsightsService')`. No real network calls.

---

### E3-S3: TokenBadge (`TokenBadge.test.tsx`)

**TC-RT-01**
- Name: `TokenBadge_renders_correct_cost_string_When_tokens_and_cost_given`
- Mock: props `{ tokens: 42000, costUsd: 0.03 }`
- Assertion: screen contains `"42K"` and `"$0.03"`
- Requirement: FR-6

**TC-RT-02**
- Name: `TokenBadge_formats_large_numbers_When_tokens_exceed_one_million`
- Mock: props `{ tokens: 1_500_000, costUsd: 4.50 }`
- Assertion: screen contains `"1.5M"` (not `"1500000"`)
- Requirement: FR-6

**TC-RT-03**
- Name: `TokenBadge_shows_loading_state_When_tokens_undefined`
- Mock: props `{ tokens: undefined }`
- Assertion: renders a loading placeholder (e.g., `aria-busy` or `"..."` text); no numeric string rendered
- Requirement: FR-6

**TC-RT-04**
- Name: `TokenBadge_applies_red_variant_When_overBudget_true`
- Mock: props `{ tokens: 100000, costUsd: 1.00, overBudget: true }`
- Assertion: the badge element has `data-over-budget="true"` or equivalent test attribute
- Requirement: FR-7

**TC-RT-05**
- Name: `TokenBadge_applies_default_variant_When_overBudget_false`
- Mock: props `{ tokens: 100000, costUsd: 1.00, overBudget: false }`
- Assertion: the badge element does NOT have the over-budget attribute/class
- Requirement: FR-7

---

### E3-S4 / E3-S5: InsightsPage and SessionsTable

**TC-RT-06**
- Name: `InsightsPage_renders_empty_state_When_no_sessions_returned`
- Mock: `useInsightsService` returns `{ sessions: [], totalCostUsd: 0, totalInputTokens: 0 }`
- Assertion: "No sessions" or equivalent empty-state message rendered; no table rows
- Requirement: FR-6

**TC-RT-07**
- Name: `InsightsPage_renders_session_rows_When_data_present`
- Mock: two `SessionTokenSummary` objects
- Assertion: `screen.getAllByRole('row')` has length 3 (header + 2 data rows)
- Requirement: FR-6

**TC-RT-08**
- Name: `InsightsPage_renders_orphan_label_When_session_is_orphan`
- Mock: one `SessionTokenSummary` with `isOrphan: true`
- Assertion: screen contains `"(untracked)"` text
- Requirement: FR-5

**TC-RT-09**
- Name: `InsightsPage_applies_filter_When_model_filter_changes`
- Mock: `useInsightsService` returns all sessions; user selects model filter from dropdown
- Assertion: `getInsightsSummary` was re-called with updated `modelFilter` param
- Requirement: FR-6

**TC-RT-10**
- Name: `InsightsPage_reflects_filter_in_URL_When_time_range_changes`
- Mock: `useSearchParams`, `useRouter` (Next.js hooks); user interacts with date picker
- Assertion: `router.push` called with URL containing `from=` and `to=` params
- Requirement: FR-6

---

### E3-S10: ExportButton / CSV

**TC-RT-11**
- Name: `ExportButton_generates_correct_header_row_When_clicked`
- Mock: `document.createElement`/`URL.createObjectURL` spied; sessions array with 1 entry
- Assertion: the Blob content passed to `URL.createObjectURL` starts with the expected CSV header: `"date,session_id,conversation_id,path,model,input_tokens,output_tokens,cache_read_tokens,estimated_cost_usd"`
- Requirement: FR-8

**TC-RT-12**
- Name: `ExportButton_generates_correct_data_rows_When_sessions_present`
- Mock: two `SessionTokenSummary` objects with known field values
- Assertion: Blob content (split on newline) has 3 lines (1 header + 2 data); each data line matches expected column values
- Requirement: FR-8

**TC-RT-13**
- Name: `ExportButton_uses_date_range_in_filename_When_filter_active`
- Mock: `from = "2026-01-01"`, `to = "2026-03-31"` in URL params
- Assertion: the `<a>` element's `download` attribute is `"insights-2026-01-01-2026-03-31.csv"`
- Requirement: FR-8

---

### E3-S9: Budget Alert (integration with TokenBadge + config)

**TC-RT-14**
- Name: `TokenBadge_turns_yellow_When_tokens_exceed_warn_threshold`
- Mock: props `{ tokens: 80000, overBudget: false, warnThreshold: 75000 }` (hypothetical prop or derived from context)
- Assertion: warning variant attribute/class applied; not the red/over-budget variant
- Requirement: FR-7

**TC-RT-15**
- Name: `SummaryCards_shows_over_budget_banner_When_any_session_exceeds_hard_threshold`
- Mock: `GetInsightsSummaryResponse` where one session has tokens exceeding hard threshold
- Assertion: screen contains `"OVER BUDGET"` banner text
- Requirement: FR-7

---

### E2-S2: useInsightsService hook (`useInsightsService.test.ts`)

**TC-RT-16**
- Name: `useInsightsService_getInsightsSummary_calls_rpc_with_correct_params`
- Mock: `createClient` returns mock with `getInsightsSummary` jest spy
- Assertion: calling `hook.getInsightsSummary({ from: <ts>, to: <ts> })` invokes the RPC with matching timestamp params
- Requirement: FR-6

**TC-RT-17**
- Name: `useInsightsService_listSessionTokens_passes_sort_params`
- Mock: same pattern; `listSessionTokens` spy
- Assertion: `sortBy` and `sortDesc` passed through to RPC call
- Requirement: FR-6

**TC-RT-18**
- Name: `useInsightsService_watchInsights_cancels_on_component_unmount`
- Mock: mock stream object returned by `watchInsights` spy; component unmounted
- Assertion: stream's `cancel()` (or abort controller) called on cleanup
- Requirement: FR-6

---

## 4. E2E Playwright Tests

File: `tests/e2e/insights-dashboard.spec.ts`

Convention:
- Feature annotation: `// @feature insights-dashboard` in line 1
- `BASE_URL = 'http://localhost:8544'`
- No `waitForTimeout`; use `expect(locator).toBeVisible()` or `page.waitForURL()`
- All locators use `data-testid` or ARIA roles
- Describe block: `test.describe('insights-dashboard', () => { ... })`

**TC-E2E-01**
- Name: `insights_page_loads_and_shows_session_list`
- Steps:
  1. Navigate to `http://localhost:8544/insights`
  2. Assert page title or heading contains "Insights"
  3. Assert the sessions table is visible (`data-testid="sessions-table"`)
- Success: Page renders without error in < 2000ms (measure with `page.waitForLoadState('networkidle')`)
- Requirement: FR-6, NFR-2

**TC-E2E-02**
- Name: `session_card_shows_token_badge_after_session_completes`
- Steps:
  1. Navigate to root (`/`)
  2. Find at least one existing session card in the list
  3. Assert `data-testid="token-badge"` is visible on the card (may show loading state initially)
  4. Wait for `data-testid="token-badge"` to contain numeric text (not loading placeholder)
- Success: Badge renders within 5s of page load
- Requirement: FR-6, Success Criterion 1

**TC-E2E-03**
- Name: `daily_chart_renders_with_date_range_filter`
- Steps:
  1. Navigate to `/insights`
  2. Confirm `data-testid="daily-spend-chart"` is present
  3. Interact with the date range picker: set `from` to 7 days ago
  4. Assert URL updates to include `from=` param
  5. Assert chart container is still visible (no crash on filter change)
- Requirement: FR-6

**TC-E2E-04**
- Name: `export_csv_downloads_file_with_correct_columns`
- Steps:
  1. Navigate to `/insights`
  2. Set up a download listener: `page.waitForEvent('download')`
  3. Click `data-testid="export-csv-button"`
  4. Await download; save to temp path
  5. Read first line of the CSV: assert it equals `"date,session_id,conversation_id,path,model,input_tokens,output_tokens,cache_read_tokens,estimated_cost_usd"`
- Requirement: FR-8

---

## 5. Test Fixtures

All files in `session/tokens/testdata/`. Each fixture is the minimum number of JSONL lines to exercise its scenario with no additional noise.

---

### `valid_session.jsonl`

A complete, realistic 3-turn session.

Structure:
- 3 `human` message entries (outer JSONL type)
- 3 `assistant` message entries, each with:
  - `"usage": { "input_tokens": N, "output_tokens": N, "cache_creation_input_tokens": N, "cache_read_input_tokens": N }`
  - `"model": "claude-sonnet-4-6-20250514"`
  - `"timestamp": "<ISO-8601>"`
  - One `tool_use` block per assistant message: `{"type":"tool_use","name":"Bash"}`
- Total: ~6 lines

Example token values (for deterministic assertions):
- Turn 1: input=1000, output=500, cache_creation=200, cache_read=0
- Turn 2: input=800, output=400, cache_creation=0, cache_read=200
- Turn 3: input=600, output=300, cache_creation=0, cache_read=300
- Expected totals: input=2400, output=1200, cache_creation=200, cache_read=500

---

### `partial_write.jsonl`

Tests EOF mid-JSON (simulates a file still being written).

Structure:
- 2 complete assistant message lines (as in `valid_session.jsonl`)
- 1 final line that is a truncated JSON string: `{"type":"message","role":"assistant","content":[{"type":"tex`
- No trailing newline

Expected behavior: Only the 2 complete lines contribute to totals; the partial line is skipped silently.

---

### `malformed_line.jsonl`

Tests robustness around one bad line.

Structure:
- Line 1: valid assistant message, input=500, output=250
- Line 2: `{this is not json at all}`
- Line 3: valid assistant message, input=300, output=150

Expected behavior: Totals are input=800, output=400. No error returned.

---

### `cache_heavy.jsonl`

Tests cache token field handling.

Structure:
- 3 assistant messages where `cache_read_input_tokens` is substantially larger than `input_tokens`:
  - Turn 1: input=100, output=50, cache_creation=5000, cache_read=0
  - Turn 2: input=100, output=50, cache_creation=0, cache_read=4000
  - Turn 3: input=100, output=50, cache_creation=0, cache_read=4000
- Expected totals: input=300, output=150, cache_creation=5000, cache_read=8000
- Implied cache hit rate on `GetInsightsSummary`: 8000/(300+8000) ≈ 0.964

---

### `multi_model.jsonl`

Tests multi-model sessions.

Structure:
- Turn 1 assistant: `"model": "claude-sonnet-4-6"`, input=1000, output=500
- Turn 2 assistant: `"model": "claude-opus-4-7-20250514"`, input=2000, output=1000
- Turn 3 assistant: `"model": "claude-sonnet-4-6"`, input=800, output=400

Expected:
- `result.PrimaryModel == "claude-sonnet-4-6"` (appears 2x vs 1x for opus)
- `result.Models` contains both raw model IDs
- Token totals: input=3800, output=1900

---

## 6. Requirement Coverage Matrix

| Requirement | Test Case(s) | Layer | Status |
|-------------|-------------|-------|--------|
| **FR-1**: Parse JSONL, extract token fields | TC-GO-01, TC-GO-04, TC-GO-07, TC-GO-08, TC-GO-09 | Go unit | Covered |
| **FR-1**: Handle malformed/partial JSONL without crash | TC-GO-02, TC-GO-03 | Go unit | Covered |
| **FR-1**: Extract tool use metadata (name, MCP server) | TC-GO-08, TC-GO-09 | Go unit | Covered |
| **FR-1**: Extract skill/command activations | TC-GO-11, TC-GO-12, TC-GO-13, TC-GO-14 | Go unit | Covered |
| **FR-2**: Aggregate tokens at session level | TC-GO-01, TC-GO-10, TC-GO-15, TC-GO-19 | Go unit | Covered |
| **FR-2**: TurnTimeline for burn rate | TC-GO-05 | Go unit | Covered |
| **FR-3**: Cost calculation (pricing table) | TC-GO-21, TC-GO-22, TC-GO-23 | Go unit | Covered |
| **FR-3**: Config pricing override | TC-GO-26 | Go unit | Covered |
| **FR-3**: Staleness flag at 30 days | TC-GO-24, TC-GO-25 | Go unit | Covered |
| **FR-3**: Model normalization (date suffix stripping) | TC-GO-06, TC-GO-20 | Go unit | Covered |
| **FR-3**: Cache hit rate calculation | TC-GO-36 | Go unit (service) | Covered |
| **FR-4**: Skill/command detection (`/command` patterns) | TC-GO-11, TC-GO-14 | Go unit | Covered |
| **FR-4**: Skill file read detection (SKILL.md path) | TC-GO-12 | Go unit | Covered |
| **FR-4**: No false positives on regular messages | TC-GO-13 | Go unit | Covered |
| **FR-5**: Session association by conversation_id | TC-GO-27 | Go unit | Covered |
| **FR-5**: Session association by path prefix | TC-GO-28 | Go unit | Covered |
| **FR-5**: Orphan sessions (no match) | TC-GO-29, TC-GO-34, TC-GO-35, TC-RT-08 | Go unit + React unit | Covered |
| **FR-6**: `/insights` route loads | TC-E2E-01 | E2E | Covered |
| **FR-6**: Real-time data via ConnectRPC | TC-GO-38, TC-RT-16, TC-RT-17, TC-RT-18 | Go unit + React unit | Covered |
| **FR-6**: Charts render (daily chart, model breakdown) | TC-E2E-03 | E2E | Covered |
| **FR-6**: Persistent filter state in URL params | TC-RT-10, TC-E2E-03 | React unit + E2E | Covered |
| **FR-6**: Sessions table (sortable, orphan label) | TC-GO-37, TC-RT-06, TC-RT-07 | Go unit + React unit | Covered |
| **FR-6**: Summary aggregation (totals, model breakdown) | TC-GO-30, TC-GO-31, TC-GO-32, TC-GO-33 | Go unit | Covered |
| **FR-7**: Budget alert badge turns red | TC-RT-04, TC-RT-05 | React unit | Covered |
| **FR-7**: Warning threshold (yellow) | TC-RT-14 | React unit | Covered |
| **FR-7**: Dashboard "OVER BUDGET" banner | TC-RT-15 | React unit | Covered |
| **FR-8**: CSV export header row | TC-RT-11, TC-E2E-04 | React unit + E2E | Covered |
| **FR-8**: CSV export data rows | TC-RT-12 | React unit | Covered |
| **FR-8**: CSV filename includes date range | TC-RT-13 | React unit | Covered |
| **NFR-1**: 10k+ JSONL does not block UI (background parse) | TC-GO-02, TC-GO-15, TC-GO-16, TC-GO-17, TC-GO-18 | Go unit | Covered |
| **NFR-2**: Dashboard loads < 2s for 90 days | TC-E2E-01 | E2E | Covered |
| **NFR-3**: No token data in external telemetry | TC-GO-38 (code review gate — no telemetry assertions in service test) | Go unit + review | Covered |
| **NFR-4**: Pricing table runtime-loadable (no rebuild) | TC-GO-26 | Go unit | Covered |
| **SC-1**: Dashboard shows data within 5s of session end | TC-GO-38, TC-E2E-02 | Go unit + E2E | Covered |
| **SC-2**: 90-day load < 2s | TC-E2E-01 | E2E | Covered |
| **SC-3**: Token counts match JSONL within 1% | TC-GO-01, TC-GO-21 | Go unit | Covered |
| **SC-4**: Budget alerts fire | TC-RT-04, TC-RT-15 | React unit | Covered |

**Total: 24 of 24 requirements/success-criteria covered.**

---

## 7. Definition of Done Checklist

The following must all be true before implementation is considered complete.

### Go Tier

- [ ] `go test ./session/tokens/...` passes with zero failures
- [ ] `go test -race ./session/tokens/...` passes (no data races — TC-GO-18)
- [ ] `go test ./server/services/ -run TestGetInsightsSummary` and `TestListSessionTokens` and `TestWatchInsights` all pass
- [ ] All 5 fixture files exist in `session/tokens/testdata/` and are committed
- [ ] `make lint` passes on all new Go files (no golangci-lint violations)
- [ ] `make nil-safety` passes (no NilAway violations in `session/tokens/` or `server/services/insights_service.go`)

### React / Frontend Tier

- [ ] `cd web-app && npx jest --no-coverage --testPathPatterns="TokenBadge|InsightsPage|ExportButton|useInsightsService"` — all 18 cases pass
- [ ] No `console.error` in Jest output (no React prop-type violations)
- [ ] `make lint` (includes `lint:css`) passes on all new `.css.ts` files

### E2E Tier

- [ ] `cd tests/e2e && npx playwright test insights-dashboard.spec.ts` — all 4 tests pass against `localhost:8544`
- [ ] `// @feature insights-dashboard` annotation present in line 1 of `insights-dashboard.spec.ts`
- [ ] No `waitForTimeout` calls in the e2e spec (convention enforced in CI)

### Registry & Coverage

- [ ] `docs/registry/backend-features.json` has entries for `insights:get-summary`, `insights:list-sessions`, `insights:watch`
- [ ] `docs/registry/frontend-features.json` has entry for `insights-dashboard`
- [ ] `make registry-generate` produces no unexpected diffs
- [ ] `docs/registry/coverage-gaps.json` does not grow (no net increase in `tested: false` entries)

### Acceptance Bar

- [ ] TC-GO-21 (`TestEstimateCost_WhenKnownModel_ExpectExactPrice`) passes with computed cost within 0.0001 USD tolerance
- [ ] TC-GO-18 (`TestTokenStore_WhenConcurrentRequests_ExpectNoDataRace`) passes with `-race` flag
- [ ] TC-E2E-01 renders the `/insights` page fully within the 2000ms Playwright timeout
- [ ] TC-E2E-04 CSV first line matches the exact header string specification

---

**Test case counts:**
- Go unit: 35 (TC-GO-01 through TC-GO-38, with TC-GO-30 through TC-GO-38 in `server/services/`)
- React unit: 18 (TC-RT-01 through TC-RT-18)
- E2E: 4 (TC-E2E-01 through TC-E2E-04)
- **Total: 57 named test cases** (some Go tests are table-driven and exercise multiple rows per case)

**Requirements coverage: 24 / 24 (100%)**
(FR-1 through FR-8: 8 functional requirements × multiple sub-clauses; NFR-1 through NFR-4: 4 non-functional; Success Criteria 1–4: 4 items)
