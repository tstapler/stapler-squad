# Validation Plan: escalation-reasoning

**Date**: 2026-08-02

## Happy Path Scenario

Given a session with a pending tool-use request whose Bash command matches no auto-approval rule,
when the classifier escalates it (`Decision: Escalate`, `RuleID: ""`) and the reviewer opens
`/review-queue`, then the card shows `❓ No matching rule; escalated for manual review.` above the
command preview, a `create-rule-<sessionId>` button with `intent="secondary"` is visible, and the
same category is counted under "No auto-approval rule matched" in the Approval Analytics
"Escalation Reasons" table for the current window.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1 (no-match/explicit-rule/domain-age categorization) | `pkg/classifier/escalation_test.go` | `TestCategorizeEscalationRuleID` (table-driven, 6 cases: `""`, `seed-escalate-git-branch-safe-delete`, `new-domain-check`, `secret-scan`, `shell-expansion-program`, unknown fallback) | Unit — happy path | All 5 known `RuleID` shapes map to their category |
| AC1 (reason text fallback) | `pkg/classifier/escalation_test.go` | `TestEscalationReasonText` (2 cases: non-empty `Reason` passthrough, zero-value `ClassificationResult{}` fallback) | Unit — error/edge path | Zero-valued `escalation` var (neither domain-checker nor classifier configured) still yields a sane fallback sentence, not `""` |
| AC1 (end-to-end capture at source, all 3 paths) | `server/services/approval_handler_test.go` | `TestHandlePermissionRequest_EscalationReason_NoMatch`, `TestHandlePermissionRequest_EscalationReason_ExplicitRule`, `TestHandlePermissionRequest_EscalationReason_DomainAge` (Story 5.1.1b) | Integration — real `ApprovalStore`, real `HandlePermissionRequest` | Each of the 3 escalation code paths (classifier no-match, classifier explicit-rule match, domain-age `goto`) populates `PendingApproval.EscalationReason`/`.EscalationCategory` correctly via `store.Get(approvalID)` |
| AC1 (poller enrichment → `ReviewItem.Metadata`) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_should_SetEscalationMetadata_When_ApprovalPending` (new — not named in plan.md; follows Story 2.2.5's AC) | Unit — happy path | `ApprovalMetadata{EscalationReason, EscalationCategory}` copies into `item.Metadata["escalation_reason"]` / `["escalation_reason_category"]` |
| AC1 (poller enrichment, absent fields) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_should_OmitEscalationMetadata_When_FieldsEmpty` (new) | Unit — error/edge path | `EscalationReason == ""` (pre-feature orphaned approval) → metadata keys not set at all (the `if a.EscalationReason != ""` guard), matching the `omitempty` contract downstream |
| AC2 (struct fields compile + JSON marshal) | `server/services/approval_store_test.go` | `TestPersistedApproval_should_OmitEscalationFields_When_Empty` / `TestPersistedApproval_should_IncludeEscalationFields_When_Set` (new — codifies Story 2.2.1's 2 ACs) | Unit — happy + edge path (both cases in one table-driven test is acceptable) | `json.Marshal` includes `"escalation_reason":"x"` when set, omits the key entirely when `""` (`omitempty`) |
| AC2 (persist round-trip through disk) | `server/services/approval_service_test.go` | `TestApprovalStore_LoadFromDisk_PreservesEscalationReason` (Story 5.1.1c) | Integration — real `ApprovalStore` against a real temp-file `pending_approvals.json`, no mocking | Simulated restart: fixture JSON with `escalation_reason`/`escalation_category` populated reloads into `PendingApproval`/`ApprovalMetadata` with both fields intact and `Orphaned == true` |
| AC3 (Create Rule gated to no-match, opens existing modal) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `describe("escalation reason") > "renders create-rule button with intent=secondary when category is no-match"` (Task 3.2.2a) | Unit (Jest/RTL) — happy path | No-match card renders `create-rule-<id>` with `intent="secondary"`; click fires `generateRule({source: COMMAND_SAMPLE, commandSample, toolNameFilter})` unchanged |
| AC3 (button absent for non-no-match) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `describe("escalation reason") > "omits create-rule button when category is domain-age"` (Task 3.2.2a) | Unit (Jest/RTL) — error/edge path | `queryByTestId("create-rule-<id>")` returns `null` even though `tool_input_command` is present |
| AC4 (analytics aggregation, all 5 buckets) | `server/services/analytics_store_test.go` | `TestComputeSummary_EscalationReasonCounts` (Task 4.1.2e) | Unit — happy path | Fixture of 6 `AnalyticsEntry` values buckets into `{"no-match":2,"domain-age":1,"secret-scan":1,"unclassifiable":1,"explicit-rule":1}` |
| AC4 (non-escalation entries excluded) | `server/services/analytics_store_test.go` | `TestComputeSummary_EscalationReasonCounts` (same test, second table case per Story 4.1.2's 2nd AC) | Unit — error/edge path | `{Decision: "auto_allow", RuleID: "some-rule"}` contributes to no key in `EscalationReasonCounts` |
| AC4 (frontend rendering — explicitly required, backend test alone insufficient) | `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx` | Task 4.2.1c's new test (name TBD at implementation, e.g. `"renders Escalation Reasons table with mapped labels and counts"`) | Unit (Jest/RTL, mocked `useApprovalAnalytics` hook) | Fixture `escalationReasonCounts` renders one row per non-zero category via `ESCALATION_CATEGORY_LABELS`, e.g. `"No auto-approval rule matched" / 12` |
| AC4 (analytics summary end-to-end through the real store) | `server/services/approval_service_test.go` | `TestGetApprovalAnalytics_IncludesEscalationReasonCounts` (new — sibling to the existing `TestGetApprovalAnalytics_ReturnsEmptySummaryWhenNoData`/`_CustomWindowDays`) | Integration — real `AnalyticsStore`/`ApprovalService`, no mocking | Record several real `AnalyticsEntry` values via the service's recording path, call `GetApprovalAnalytics`, assert the returned `AnalyticsSummaryProto.EscalationReasonCounts` map matches — proves the full `ComputeSummary` → `summaryToProto` chain, not just the pure function in isolation |
| AC5 (existing suites unmodified) | `server/services/approval_handler_test.go`, `session/review_queue_determiner_test.go` | Full existing suites, run via `go test ./server/services -run TestHandlePermissionRequest` and `go test ./session -run TestReviewQueueDeterminer` (Tasks 5.1.1a, 5.2.1a) | Integration — regression baseline, zero assertion diffs | Pre- and post-implementation runs produce identical pass/fail results with no edits to existing test function bodies |
| AC5 (new escalation-reason fields don't break auto-allow/deny/timeout paths) | `server/services/approval_handler_test.go` | Existing auto-allow/auto-deny/timeout test functions (unmodified) | Regression — happy path | Suite green with zero code changes to existing assertions after Phase 1-4 land |
| AC6 (plain-text render via `itemContext`, category-correct copy verbatim) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `describe("escalation reason") > "renders reason paragraph verbatim with category emoji prefix"` (Task 3.2.2a) | Unit (Jest/RTL) — happy path | `<p id="escalation-reason-<sessionId>">` text starts with the category's emoji and contains the backend `Reason` string verbatim, no re-wrapping |
| AC6 (orphaned/fallback copy, never blank) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `describe("escalation reason") > "renders fallback copy when escalation_reason is absent"` (Task 3.2.2a, 3rd case) | Unit (Jest/RTL) — error/edge path | `pending_approval_id` present, `escalation_reason` key absent → fallback sentence renders, not a blank `<p>` |
| AC7 (Create Rule `intent="secondary"`, never `"primary"`) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | Same test as AC3's happy-path case (Task 3.2.2a) — `intent="secondary"` asserted directly | Unit (Jest/RTL) — happy path | Rendered `create-rule-<id>` has `intent="secondary"` attribute/class, never `"primary"` |
| AC7 (Approve stays `intent="primary"`, regression) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | New assertion appended to an existing Approve-button test, or a new one-line case in the escalation-reason `describe` block | Unit (Jest/RTL) — error/edge path (regression guard) | `approve-<id>` button's `intent="primary"` is unchanged by this feature — guards against the two buttons' intents being accidentally swapped |
| AC8 (real end-to-end no-match escalation) | `tests/e2e/escalation-reasoning.spec.ts` | `test("shows escalation reason for a real no-match hook escalation")` (Story 6.1.1 / Tasks 6.1.1a-c) | E2E (Playwright) — this test *is* the integration test for AC8; no separate unit split | Real POST to `/api/hooks/permission-request` (unawaited) → `waitForReviewQueue(1)` → navigate to `/review-queue` → reason text visible via `getByTestId(\`review-item-${sessionId}\`)` → cleanup resolves the approval in `afterEach`/`finally` regardless of pass/fail |
| AC8 (hook doesn't hang test / cleanup discipline) | `tests/e2e/escalation-reasoning.spec.ts` | Same spec, `afterEach` cleanup path (Task 6.1.1c) | E2E (Playwright) — error/edge path | Approval is resolved via `approve-${sessionId}`/`deny-${sessionId}` even on assertion failure; the backgrounded POST promise is awaited afterward to confirm the hook itself returns, preventing cross-spec queue pollution |

---

## UX Acceptance Tests

21 criteria from `design/ux.md`. "Tool" column: **Playwright** (automatable via `ui-playwright` skill
model, `data-testid`/ARIA locators, no `waitForTimeout`, feature-annotation header per
`.claude/rules/e2e-test-conventions.md`), **Jest** (cross-referenced to a test already in the
Requirement → Test Mapping table above — no duplicate test, just confirms which existing test
covers it), or **Manual** (genuinely requires human judgment: glyph rendering, contrast measurement,
screen-reader announcement, color-blindness simulation).

### Surface A — Review-queue card

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| A1 | Task completion in ≤2 clicks | `tests/e2e/escalation-reasoning.spec.ts` | `test("reviewer reads reason and opens Create Rule within 2 clicks")` | Playwright | Land on `/review-queue` with a no-match fixture (0 clicks: assert reason `<p>` visible via `getByText`); click `create-rule-<id>` (1 click: assert modal `role="dialog"` visible) |
| A2 | No dead ends — every `pending_approval_id` card shows a non-empty reason | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `"renders fallback copy when escalation_reason is absent"` (same as AC6 edge-path row above) | Jest | Cross-ref: covered by the orphaned-fixture Jest case already in the AC mapping table |
| A3 | Category-correct copy for all 4 categories (no-match/explicit-rule/domain-age/unclassifiable) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `describe("escalation reason") > "renders correct emoji+text for each category"` — extend Task 3.2.2a's table beyond the plan's 2 named cases to cover all 4 emoji entries | Jest | Table-driven RTL test, one row per `ESCALATION_REASON_EMOJI` key, asserting exact emoji + verbatim backend text |
| A4 | Create Rule absent for 3 of 4 non-no-match categories | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` + spot check | `"omits create-rule button when category is domain-age"` (AC3 edge-path row) | Jest + Manual | Jest covers domain-age programmatically; manually spot-check one live domain-age escalation in the running app per ux.md's explicit ask |
| A5 | No misclick risk — Approve (solid) vs Create Rule (bordered) visually distinct | `tests/e2e/escalation-reasoning.spec.ts` (screenshot capture) | Manual review of `test("visual: approve vs create-rule button styling")` screenshot | Manual (Playwright-assisted) | Playwright navigates + takes a screenshot of the action row; a human confirms Approve is solid-filled and Create Rule is bordered/non-solid — pure class-name equality doesn't prove visual distinction |
| A6 | Reading order — reason line above command preview | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `"renders reason paragraph before command preview in DOM order"` (new) | Jest | RTL `container.querySelectorAll` (or `compareDocumentPosition`) confirms the reason `<p>` precedes the `commandPreview` `<pre>` in DOM order |
| A7 | Screen reader — `aria-describedby` announces reason without extra tab stop | Manual | — | Manual | Run VoiceOver/NVDA (or `browser_snapshot`'s accessibility tree as a proxy), tab to a review-queue card, confirm the accessible name includes the reason text via `aria-describedby` and no extra tab stop is introduced |
| A8 | Keyboard-only — Tab sequence unaffected by new reason paragraph | `tests/e2e/escalation-reasoning.spec.ts` | `test("keyboard tab order skips non-interactive reason paragraph")` | Playwright | `page.keyboard.press("Tab")` repeatedly from a known start point; assert focus lands on Approve → Deny → Create Rule in sequence with no extra stop on the reason text |
| A9 | Contrast ≥4.5:1 for `itemContext` text, light + dark theme | Manual | — | Manual | Screenshot the rendered card in both themes; run a contrast-checker tool (e.g. axe or a color-contrast utility) against the actual rendered pixel colors, not the token name |
| A10 | Emoji glyph legibility (❓🛑🌐⚙️ render as recognizable glyphs, not tofu) | Manual | — | Manual | Screenshot each of the 4 category cards in the app's actual font stack/browser; visually confirm no missing-character boxes |

### Surface B — SuggestedRuleCard modal

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| B11 | No dead ends — every terminal modal state has a working exit | `tests/e2e/escalation-reasoning.spec.ts` | `test("modal closes via [x] after a generateRule failure")` | Playwright | Mock/throttle the `GenerateSuggestedRule` RPC to fail (`page.route` intercept), open modal, confirm error text renders, click `[✕]`, assert modal unmounts |
| B12 | Trigger-gating correctness — modal only reachable from no-match Create Rule | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | Cross-ref: same as A4/AC3 edge-path (`queryByTestId` null on non-no-match cards) — absence of the button is the trigger-gating proof | Jest | No separate test needed; button absence already proves no code path reaches `generateRule(...)` from those cards |
| B13 | Focus management — dialog receives focus on open, sensible return on close | `tests/e2e/escalation-reasoning.spec.ts` | `test("focus moves into dialog on open and returns to trigger on close")` | Playwright | After clicking `create-rule-<id>`, assert `document.activeElement` is inside the `role="dialog"`; after closing via `[✕]`, assert focus returns to `create-rule-<id>` (not `<body>`) |

### Surface C — Escalation Reasons analytics table

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| C14 | Task completion in 0 clicks — dominant category is the top row | `tests/e2e/approval-analytics-escalation-reasons.spec.ts` (new) | `test("dominant escalation category is the top row with zero extra clicks")` | Playwright | Navigate directly to the Approval Analytics tab; assert the first `<tr>` in the Escalation Reasons table matches the highest-count fixture category, no interaction beyond page load |
| C15 | Error state — "Failed to load analytics" + Retry | `tests/e2e/approval-analytics-escalation-reasons.spec.ts` | `test("shows error banner and retry on analytics fetch failure")` | Playwright | `page.route` to fail the analytics RPC; assert error banner text + `getByRole("button", {name: "Retry"})` visible |
| C16 | No dead ends — Retry always re-attempts | `tests/e2e/approval-analytics-escalation-reasons.spec.ts` | `test("retry re-fetches and clears the error state")` | Playwright | After C15's failure, unblock the route, click Retry, assert the error banner is gone and the table renders |
| C17 | Zero-escalation window shows explicit empty message, not a blank table | `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx` | `"renders empty-state message when all escalation categories are zero"` (extends Task 4.2.1c per the empty-state branch Task 4.2.1b adds) | Jest | Mock `escalationReasonCounts` as all-zero or `{}`; assert `empty`-styled "No escalations in this window" text renders instead of a headers-only table |
| C18 | Window-switch consistency — no stale-data flash | `tests/e2e/approval-analytics-escalation-reasons.spec.ts` | `test("switching window updates counts without stale flash")` | Playwright | Load with 7d fixture, assert count X; click 30d, assert count changes to Y and no intermediate render still shows X under the "30d" label (poll via `expect(locator).toHaveText(...)`, never `waitForTimeout`) |
| C19 | Long-tail category (`secret-scan`) still visible when non-zero | `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx` | Extend Task 4.2.1c's fixture to include a non-zero `secret-scan` count and assert its row renders with label `"Plaintext secret detected"` | Jest | Table-driven fixture includes `secret-scan: 1`; assert the row is present (not silently dropped as the "smallest" bucket) |

### Cross-surface

| # | UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|---|
| 20 | Copy never leaks internals (raw `RuleID`, `"Escalate"`, sentinel strings) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` + `ApprovalAnalyticsPanel.test.tsx` | `"never renders raw RuleID or sentinel strings"` (new, both files) | Jest | Assert rendered text does not match `/new-domain-check|shell-expansion-program|secret-scan|Escalate/` outside of the mapped human labels |
| 21 | Color is never the sole differentiator | Manual | — | Manual | View both surfaces through a grayscale/color-blindness simulation filter (e.g. browser devtools vision-deficiency emulation); confirm categories remain distinguishable via emoji (Surface A) or text label (Surface C) |

**Counts**: 10 Surface A + 3 Surface B + 6 Surface C + 2 cross-surface = 21. Tool breakdown: 11
Playwright, 5 Jest-cross-ref/extension, 5 Manual (A5 is Playwright-assisted + Manual judgment,
counted under Manual).

---

## Test Stack

- **Unit**: Go `testing` + table-driven tests (`pkg/classifier/escalation_test.go`,
  `server/services/analytics_store_test.go`); Jest + React Testing Library
  (`ReviewQueuePanel.test.tsx`, `ApprovalAnalyticsPanel.test.tsx`).
- **Integration**: Go `testing` against a **real** `ApprovalStore` (`NewApprovalStore("")` for
  in-memory, or a real temp-file path for disk round-trip tests) and real `ComputeSummary` — this
  codebase does not mock its store layer. Confirmed by reading
  `server/services/approval_service_test.go` (`TestResolveApproval_PublishesEventBusEvent`,
  `TestListPendingApprovals_ReturnsAllPending`, etc.) — every test constructs a real
  `NewApprovalStore("")`/`NewApprovalService(...)` and asserts against it directly; no
  `mock`/`Mock` types exist in that file. `TestApprovalStore_LoadFromDisk_PreservesEscalationReason`
  (AC2) follows the same pattern with a real temp file instead of `""`.
- **E2E / UX**: Playwright per `.claude/rules/e2e-test-conventions.md` — feature annotation header,
  no `waitForTimeout`, `data-testid`/ARIA role locators only. New spec files:
  `tests/e2e/escalation-reasoning.spec.ts` (AC8 + Surface A/B UX checks that need a real hook-driven
  escalation) and `tests/e2e/approval-analytics-escalation-reasons.spec.ts` (Surface C UX checks,
  mocked analytics fetch — no real hook call needed since the analytics summary is independently
  mockable via `page.route`). No new `tests/e2e/pages/` helper is required for AC8 itself per
  requirements.md's explicit note (uses existing `data-testid` locators directly); the two new spec
  files above are cohesive enough as-is that a dedicated `ReviewQueuePage.ts`/`AnalyticsPage.ts`
  page object is optional polish, not required for AC8/UX coverage — revisit only if a 3rd spec
  needs the same locators.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% line coverage on the new `pkg/classifier/escalation.go` (it's two pure functions with fully enumerated branches — no excuse for a gap) |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this feature (`CategorizeEscalationRuleID`,
  `EscalationReasonText`, `ComputeSummary`'s new branch, `GetApprovalMetadataBySession`): happy path
  + error/edge paths covered per the Requirement → Test Mapping table above.
- External integrations: none new — `DomainAgeChecker.IsNewlyRegistered` is pre-existing and
  unchanged; the only "external call" in this feature's scope is the real HTTP hook POST in AC8's
  e2e spec, which is the integration test itself (no separate unit-mocked version needed since the
  handler-level integration tests in `approval_handler_test.go` already exercise
  `HandlePermissionRequest` directly in-process).
- UX acceptance criteria: all 21 criteria in `design/ux.md` have a corresponding automated test or
  an explicit Manual verification step in the table above — none are silently unaddressed.
- Regression guard (AC5): `make test` run after Phase 1-4 lands must show zero diffs to existing
  `approval_handler_test.go`/`review_queue_determiner_test.go` assertions — this is itself a test
  step (Tasks 5.1.1a, 5.2.1a), not just a hope.
