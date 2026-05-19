# Validation: AI-Assisted Rule Generation
> Phase 4 — SDD Validation Gate
> Date: 2026-05-18
> Source: requirements.md, plan.md, adversarial-review.md

---

## Part 1: Test Suite (Requirement-to-Test Traceability)

### FR-1: GenerateSuggestedRule RPC — proto shape, source enum, plural response

#### T-UNIT-GO-001 — Happy path: valid analytics_gaps request returns plural suggestions
- **Test name:** `TestGenerateSuggestedRule_AnalyticsGaps_ReturnsSuggestions`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test (mock `AIClient` returning 2-element JSON array, mock `analyticsStore`)
- **Covers:** FR-1 (plural `repeated SuggestedRuleProto suggestions`), FR-7 (context assembly)
- **Setup:** Inject `MockAIClient` returning `[{...}, {...}]`; call `GenerateSuggestedRule` with `source: ANALYTICS_GAPS, window_days: 7`.
- **Assert:** `len(resp.Msg.Suggestions) == 2`; each suggestion has non-empty `Name`, `CommandPattern`, `Confidence > 0`.

#### T-UNIT-GO-002 — Failure mode: unspecified source returns CodeInvalidArgument
- **Test name:** `TestGenerateSuggestedRule_UnspecifiedSource_ReturnsError`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test
- **Covers:** FR-1 (source validation guard)
- **Assert:** `connect.CodeOf(err) == connect.CodeInvalidArgument`; error message contains "source is required".

#### T-UNIT-GO-003 — Failure mode: nil AI client returns CodeUnimplemented
- **Test name:** `TestGenerateSuggestedRule_NilAIClient_ReturnsUnimplemented`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test
- **Covers:** FR-1 (graceful degradation when `ANTHROPIC_API_KEY` not set)
- **Assert:** `connect.CodeOf(err) == connect.CodeUnimplemented`; error mentions "ANTHROPIC_API_KEY".

---

### FR-2: Suggestion Review UI — SuggestedRuleCard fields editable, Accept & Save, Discard

#### T-UNIT-TS-001 — Happy path: renders all editable fields from suggestion proto
- **Test name:** `SuggestedRuleCard_should_renderEditableFields_When_SuggestionProvided`
- **File:** `web-app/src/components/sessions/SuggestedRuleCard.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-2 (editable inline fields, confidence badge, explanation, source commands)
- **Assert:** `commandPattern` input has value from fixture; confidence badge text matches `Math.round(0.9 * 100) + "%"`; explanation text is visible.

#### T-UNIT-TS-002 — Happy path: Accept & Save calls upsertRule with edited values
- **Test name:** `SuggestedRuleCard_should_callUpsertRule_When_AcceptClicked`
- **File:** `web-app/src/components/sessions/SuggestedRuleCard.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-2 (Accept & Save triggers UpsertApprovalRule, not auto-save)
- **Setup:** Mock `useApprovalRules` hook; edit `commandPattern` field; click "Accept & Save".
- **Assert:** `upsertRule` called once with the edited `commandPattern` value; `onAccept` callback invoked.

#### T-UNIT-TS-003 — Happy path: Discard calls onDiscard without saving
- **Test name:** `SuggestedRuleCard_should_callOnDiscard_When_DiscardClicked`
- **File:** `web-app/src/components/sessions/SuggestedRuleCard.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-2 (Discard dismisses card; FR-8 no auto-save)
- **Assert:** `onDiscard` called; `upsertRule` NOT called.

#### T-UNIT-TS-004 — Failure mode: conflict warning rendered when shadowedByRuleIds non-empty
- **Test name:** `SuggestedRuleCard_should_showConflictWarning_When_ShadowedByRulesPresent`
- **File:** `web-app/src/components/sessions/SuggestedRuleCard.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-2 (conflict detection banner from `shadowed_by_rule_ids`)
- **Assert:** Conflict banner visible; banner text includes "may overlap with" (heuristic language — not definitive per adversarial review Issue 5).

---

### FR-3: Rules Page — Generate Suggestions Panel

#### T-E2E-001 — Happy path: loading state shown while RPC in flight
- **Test name:** `rules-generate-suggestions > should show loading state when Generate Suggestions clicked`
- **File:** `tests/e2e/rules-generate-suggestions.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-3 (loading state while agent runs, latency NFR)
- **Assert:** "Generating…" button text visible immediately after click; button is disabled.

#### T-E2E-002 — Happy path: SuggestedRuleCard list rendered after RPC returns
- **Test name:** `rules-generate-suggestions > should show SuggestedRuleCard list when suggestions returned`
- **File:** `tests/e2e/rules-generate-suggestions.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-3 (renders list of SuggestedRuleCard; plural per Issue 1 resolution)
- **Assert:** At least one `[data-testid="suggested-rule-card"]` visible; card contains fixture rule name.

#### T-E2E-003 — Failure mode: cancel hides loading state
- **Test name:** `rules-generate-suggestions > should hide loading state when Cancel clicked`
- **File:** `tests/e2e/rules-generate-suggestions.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-3 (cancellation — latency NFR)
- **Assert:** After "Cancel" clicked, loading state disappears; button reverts to "Generate Suggestions".

#### T-E2E-004 — Happy path: Discard removes card
- **Test name:** `rules-generate-suggestions > should hide card when Discard clicked`
- **File:** `tests/e2e/rules-generate-suggestions.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-3, FR-2 (Discard clears panel)
- **Assert:** After Discard, `[data-testid="suggested-rule-card"]` not present.

---

### FR-4: Review Queue — Create Rule From This

#### T-E2E-005 — Happy path: "Create Rule from This" opens modal with SuggestedRuleCard
- **Test name:** `review-queue-create-rule > should open modal with SuggestedRuleCard when button clicked`
- **File:** `tests/e2e/review-queue-create-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-4 (button on pending-approval item, modal with card)
- **Assert:** Modal visible; `[data-testid="suggested-rule-card"]` inside modal; fixture rule name shown.

#### T-E2E-006 — Failure mode: button absent on non-pending items
- **Test name:** `review-queue-create-rule > should show Create Rule button only on pending-approval items`
- **File:** `tests/e2e/review-queue-create-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-4 (button scoped to pending items only)
- **Assert:** "Create Rule from This" button not present on auto-approved/denied items in queue.

#### T-E2E-007 — Failure mode: Discard closes modal without saving
- **Test name:** `review-queue-create-rule > should close modal when Discard clicked`
- **File:** `tests/e2e/review-queue-create-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-4, FR-8
- **Assert:** Modal not present after Discard; no rule added to rules list.

---

### FR-5: Analytics Gap Item — Suggest Rule

#### T-E2E-008 — Happy path: "Suggest Rule" button appears on coverage-gap rows
- **Test name:** `analytics-suggest-rule > should show Suggest Rule button when coverage gaps exist`
- **File:** `tests/e2e/analytics-suggest-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-5 (icon button on each gap row)
- **Assert:** `[data-testid^="suggest-rule-tool-"]` visible in uncovered-tools table.

#### T-E2E-009 — Happy path: SuggestedRuleCard shown inline after click
- **Test name:** `analytics-suggest-rule > should show SuggestedRuleCard inline when suggestion returned`
- **File:** `tests/e2e/analytics-suggest-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-5 (inline/popover card scoped to clicked row)
- **Assert:** Card appears below the clicked row; card tool name matches the row's tool name.

#### T-E2E-010 — Failure mode: clicking a different row cancels the previous in-flight request
- **Test name:** `analytics-suggest-rule > should cancel previous suggestion when different row clicked`
- **File:** `tests/e2e/analytics-suggest-rule.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-5 (AbortController behavior; latency NFR)
- **Assert:** After clicking row B while row A is loading, only row B's loading indicator is shown; row A's loading indicator is gone.

---

### FR-6: Command Sample Input

#### T-UNIT-TS-005 — Happy path: "Generate" pre-fills commandPattern from fixture suggestion
- **Test name:** `ApprovalRulesPanel_should_prefillFormFields_When_CommandSampleSuggestionReturns`
- **File:** `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-6 (paste command → suggestion → pre-fill form)
- **Assert:** `commandPattern` input has fixture value; "AI-generated — review before saving" badge visible.

#### T-UNIT-TS-006 — Failure mode: user-edited fields not overwritten by subsequent suggestion
- **Test name:** `ApprovalRulesPanel_should_notOverwriteTouchedFields_When_SuggestionReturns`
- **File:** `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx`
- **Type:** Jest unit test (RTL)
- **Covers:** FR-6 (touched fields protection via `touchedFields` ref)
- **Setup:** User types into `commandPattern`; then suggestion returns with different value.
- **Assert:** `commandPattern` retains the user-typed value.

#### T-E2E-011 — Happy path: pasting command and clicking Generate pre-fills form
- **Test name:** `rule-from-command-sample > should prefill form fields when Generate clicked with command`
- **File:** `tests/e2e/rule-from-command-sample.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-6 (end-to-end command sample flow)
- **Assert:** After generate, `commandPattern` input contains value from mock response.

#### T-E2E-012 — Failure mode: AI-generated badge displayed after pre-fill
- **Test name:** `rule-from-command-sample > should show AI-generated badge after prefill`
- **File:** `tests/e2e/rule-from-command-sample.spec.ts`
- **Type:** Playwright e2e test
- **Covers:** FR-6 (user review signal)
- **Assert:** "AI-generated" notice visible in form after pre-fill.

---

### FR-7: Agent Context Assembly

#### T-UNIT-GO-004 — Happy path: buildPromptContext includes existing rules and analytics gaps
- **Test name:** `TestBuildPromptContext_IncludesRulesAndGaps`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test
- **Covers:** FR-7 (all existing rules via `AllRules`, analytics data via `GetAnalyticsSummary`)
- **Setup:** Fixture `analyticsStore` with 3 escalated entries; fixture `rulesStore` with 2 rules.
- **Assert:** `RulePromptContext.ExistingRules` has length 2; `AnalyticsGaps` has length ≥ 1 with `Count == 3`.

#### T-UNIT-GO-005 — Happy path: BuildSystemPrompt includes JSON schema block and seed examples
- **Test name:** `TestDefaultRulePromptBuilder_BuildSystemPrompt_ContainsSchemaAndSeeds`
- **File:** `server/services/rule_prompt_builder_test.go`
- **Type:** Go unit test
- **Covers:** FR-7 (seed rule examples, JSON schema in system prompt)
- **Assert:** `BuildSystemPrompt(ctx)` result contains substrings "JSON schema", "existing rules", "priority tiers".

#### T-UNIT-GO-006 — Happy path: BuildUserPrompt formats analytics gap with tool name and count
- **Test name:** `TestDefaultRulePromptBuilder_BuildUserPrompt_FormatsGap`
- **File:** `server/services/rule_prompt_builder_test.go`
- **Type:** Go unit test
- **Covers:** FR-7 (analytics item as focal point in prompt)
- **Setup:** `RulePromptContext` with one gap: `ToolName: "Bash", Count: 42`.
- **Assert:** User prompt string contains "Bash" and "42".

#### T-INTEG-001 — Integration: full handler pipeline with mock AIClient returns valid suggestions
- **Test name:** `TestGenerateSuggestedRule_Integration_MockAI`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go integration test (in-process; mock AIClient only, real prompt builder)
- **Covers:** FR-7 (full context assembly → prompt build → mock AI → parse → response)
- **Assert:** With a mock `AIClient` returning a 2-item JSON array, `GenerateSuggestedRule` returns `len(Suggestions) == 2`; each `Confidence` is in [0, 1].

---

### FR-8: No Auto-Save

#### T-UNIT-GO-007 — Security invariant: GenerateSuggestedRule never calls rulesStore.Upsert
- **Test name:** `TestGenerateSuggestedRule_NeverCallsUpsert`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test (spy `rulesStore`)
- **Covers:** FR-8 (architectural no-auto-save guarantee)
- **Setup:** Call `GenerateSuggestedRule` successfully. `rulesStore` is a spy that records all method calls.
- **Assert:** `rulesStore.Upsert` call count is 0 after handler returns.

#### T-UNIT-TS-003 (shared above — Discard does not call upsertRule)
- Already specified under FR-2. Cross-covers FR-8.

#### T-UNIT-TS-002 (shared above — Accept calls upsertRule explicitly)
- Already specified under FR-2. Demonstrates persistence requires explicit user action.

---

### Security: Secret Redaction (Pitfalls §5a + adversarial-review Issue 4)

#### T-UNIT-GO-008 — SECRET REDACTION — command containing ghp_ is not persisted to analytics
- **Test name:** `TestApprovalHandler_SecretNotPersistedToAnalytics`
- **File:** `server/services/approval_handler_test.go`
- **Type:** Go unit test (mock `analyticsStore` spy)
- **Covers:** Story 1.4 fix; adversarial-review Issue 4 (primary redaction path)
- **Setup:** Fire approval with command `curl -H "Authorization: Bearer ghp_secret123"`. Spy `analyticsStore.RecordFromResult`.
- **Assert:** `RecordFromResult` called with payload where `Command == "[REDACTED: secret detected]"`; `"ghp_secret123"` does not appear in any recorded payload.

#### T-UNIT-GO-009 — SECRET REDACTION — BuildUserPrompt redacts command previews with ghp_ token
- **Test name:** `TestBuildUserPrompt_RedactsSecretCommandPreviews`
- **File:** `server/services/rule_prompt_builder_test.go`
- **Type:** Go unit test
- **Covers:** Story 1.2, Task 1.2.2 second-pass redaction; adversarial-review Issue 4 (defense-in-depth path)
- **Setup:** `RulePromptContext.AnalyticsGaps[0].RepresentativeCmds = ["curl -H 'Auth: ghp_xxx'"]`.
- **Assert:** `BuildUserPrompt(ctx)` output contains `[REDACTED]`; does not contain `ghp_xxx`.

#### T-INTEG-002 — Integration: analytics query after approval with secret command contains no secret
- **Test name:** `TestApprovalHandler_LoadWindow_ContainsNoSecret`
- **File:** `server/services/approval_handler_test.go`
- **Type:** Go integration test (in-process; real analyticsStore in temp DB)
- **Covers:** Story 1.4, Task 1.4.2 — end-to-end redaction + query path
- **Setup:** Call approval handler with command `ANTHROPIC_API_KEY=sk-ant-test123 curl ...`; then `LoadWindow`.
- **Assert:** No returned entry's `CommandPreview` contains `sk-ant-`.

---

### Handler-Level Cancellation (adversarial-review Issue 8)

#### T-UNIT-GO-010 — Handler returns promptly when context cancelled mid-AI-call
- **Test name:** `TestGenerateSuggestedRule_ReturnsOnCtxCancellation`
- **File:** `server/services/rules_service_test.go`
- **Type:** Go unit test (mock `AIClient` blocks on `ctx.Done()`)
- **Covers:** Adversarial Issue 8 (goroutine leak prevention); latency NFR
- **Setup:** Inject mock `AIClient` that blocks until context is cancelled. Call handler; cancel context after 100ms.
- **Assert:** Handler returns within 200ms; error is context-related (not a timeout from the HTTP layer).

---

### AnthropicAIClient Cancellation (Story 1.2, Task 1.2.3)

#### T-UNIT-GO-011 — AnthropicAIClient.Complete cancels outbound HTTP request on ctx.Done
- **Test name:** `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
- **File:** `server/services/anthropic_client_test.go`
- **Type:** Go unit test (`httptest.Server` that blocks until context cancelled)
- **Covers:** Story 1.2 acceptance criterion; goroutine leak prevention
- **Assert:** `Complete` returns within 200ms of context cancel; `httptest.Server` receives a connection abort (not a full response cycle).

---

### useGenerateRule hook (Story 2.1)

#### T-UNIT-TS-007 — Loading state is true during generate, false after
- **Test name:** `useGenerateRule_should_setLoading_During_Fetch`
- **File:** `web-app/src/lib/hooks/useGenerateRule.test.ts`
- **Type:** Jest unit test (`renderHook` + injected mock client)
- **Covers:** FR-3, FR-4, FR-5, FR-6 loading state; latency NFR
- **Assert:** `loading === true` while `generate` promise is pending; `false` after resolution.

#### T-UNIT-TS-008 — Suggestions array populated on success
- **Test name:** `useGenerateRule_should_setSuggestions_When_RPCSucceeds`
- **File:** `web-app/src/lib/hooks/useGenerateRule.test.ts`
- **Type:** Jest unit test
- **Covers:** FR-1 (plural suggestions in hook state)
- **Assert:** After `generate` resolves, `suggestions.length === 2` (fixture mock returns 2).

#### T-UNIT-TS-009 — Error state set on RPC failure; cleared on next generate call
- **Test name:** `useGenerateRule_should_setError_When_RPCFails`
- **File:** `web-app/src/lib/hooks/useGenerateRule.test.ts`
- **Type:** Jest unit test
- **Covers:** FR-3 error banner; latency NFR (error display)
- **Assert:** After failed generate, `error !== null`; after second `generate` call starts, `error === null`.

#### T-UNIT-TS-010 — AbortError does not set error state
- **Test name:** `useGenerateRule_should_notSetError_When_Cancelled`
- **File:** `web-app/src/lib/hooks/useGenerateRule.test.ts`
- **Type:** Jest unit test
- **Covers:** FR-3 (cancel does not show error banner)
- **Assert:** After `cancel()` called, `error === null`; `loading === false`.

---

## Part 2: Test Count Summary

| Type | Count |
|------|-------|
| Go unit tests | 11 (T-UNIT-GO-001 through T-UNIT-GO-011) |
| Go integration tests | 2 (T-INTEG-001, T-INTEG-002) |
| Jest unit tests (TypeScript) | 10 (T-UNIT-TS-001 through T-UNIT-TS-010) |
| Playwright e2e tests | 12 (T-E2E-001 through T-E2E-012) |
| **Total** | **35** |

---

## Part 3: Requirements Coverage

| FR | Test IDs | Covered? |
|----|----------|----------|
| FR-1: GenerateSuggestedRule RPC | T-UNIT-GO-001, T-UNIT-GO-002, T-UNIT-GO-003, T-UNIT-TS-008, T-INTEG-001 | YES |
| FR-2: Suggestion Review UI | T-UNIT-TS-001, T-UNIT-TS-002, T-UNIT-TS-003, T-UNIT-TS-004 | YES |
| FR-3: Rules Page panel | T-E2E-001, T-E2E-002, T-E2E-003, T-E2E-004, T-UNIT-TS-007, T-UNIT-TS-009 | YES |
| FR-4: Review Queue action | T-E2E-005, T-E2E-006, T-E2E-007 | YES |
| FR-5: Analytics gap button | T-E2E-008, T-E2E-009, T-E2E-010 | YES |
| FR-6: Command sample input | T-UNIT-TS-005, T-UNIT-TS-006, T-E2E-011, T-E2E-012 | YES |
| FR-7: Agent context assembly | T-UNIT-GO-004, T-UNIT-GO-005, T-UNIT-GO-006, T-INTEG-001 | YES |
| FR-8: No auto-save | T-UNIT-GO-007, T-UNIT-TS-002, T-UNIT-TS-003 | YES |

**Requirements coverage: 8/8 FRs covered.**

---

## Part 4: Readiness Gate

### Gate 1: Requirements coverage — PASS

All 8 functional requirements have at least one happy-path and one failure-mode test case mapped to them. See coverage table above.

---

### Gate 2: Security coverage — PASS

**Secret-redaction bug (pitfalls §5a):**
- T-UNIT-GO-008 (`TestApprovalHandler_SecretNotPersistedToAnalytics`) — primary redaction path (Story 1.4, Task 1.4.1). Validates that `approval_handler.go` shallow-copies the payload and replaces `Command` with `[REDACTED: secret detected]` before calling `RecordFromResult`.
- T-UNIT-GO-009 (`TestBuildUserPrompt_RedactsSecretCommandPreviews`) — defense-in-depth path (Story 1.2, Task 1.2.2). Validates that `DefaultRulePromptBuilder.BuildUserPrompt` runs a second `ScanForSecrets` pass on `RepresentativeCmds` and substitutes `[REDACTED]`.
- T-INTEG-002 (`TestApprovalHandler_LoadWindow_ContainsNoSecret`) — end-to-end validation that no secret survives through to `LoadWindow` output.

**FR-8 no-auto-save invariant:**
- T-UNIT-GO-007 (`TestGenerateSuggestedRule_NeverCallsUpsert`) — spy-based test that asserts `rulesStore.Upsert` call count is zero after a successful `GenerateSuggestedRule` call.

Both security-sensitive cases have dedicated tests.

---

### Gate 3: Plan completeness — PASS WITH NOTES

Every story in plan.md has acceptance criteria. Evaluation by story:

| Story | Acceptance criteria present? | Vague? | Notes |
|-------|------------------------------|--------|-------|
| 1.1 (proto definition) | Yes | No | "make generate-proto succeeds; SuggestedRuleProto appears in generated files" — concrete and verifiable. |
| 1.2.1 (ai_interfaces.go) | Yes | No | "compiles with no external dependencies; both interfaces in package services" — binary pass/fail. |
| 1.2.2 (DefaultRulePromptBuilder) | Yes | No | Three named unit tests with specific assertions. |
| 1.2.3 (AnthropicAIClient) | Yes | No | Named cancellation test with httptest.Server specified. |
| 1.2.4 (config field) | Yes | No | "starts without panic; AnthropicAPIKey non-empty when set via env" — verifiable. |
| 1.2.5 (wiring) | Yes | No | "nil,nil compiles; existing tests still pass" — binary. |
| 1.3.1 (buildPromptContext) | Yes | No | Unit test with fixture analytics store; named assertion. |
| 1.3.2 (GenerateSuggestedRule method) | Yes | No | "Compiles. Integration test with mock AIClient returning 2-element array produces len(Suggestions)==2". |
| 1.3.3 (parseSuggestions rename) | Implicit (folded into 1.3.4) | **Marginal** | No standalone acceptance criterion; success is verified indirectly by 1.3.4's table-driven tests. Recommend adding: "parseSuggestions with a 6-element array returns exactly 5 items (cap enforced)." |
| 1.3.4 (parseSuggestion helper) | Yes | No | Four-case table test specified with exact assertions. |
| 1.3.5 (attachConflictInfo) | Yes | No | Named unit test with fixture rule set and specific assertion on ShadowedByRuleIds. |
| 1.3.6 (SessionService pass-through) | Yes | No | "make build succeeds; make test passes" — binary. |
| 1.4.1 (redaction fix) | Yes | No | Named test with mock analyticsStore spy and exact assertion. |
| 1.4.2 (integration redaction) | Yes | No | End-to-end query assertion; deliberate-revert verification step specified. |
| 2.1.1 (useGenerateRule hook) | Yes | No | Named test with loading-state assertion. |
| 2.1.2 (hook tests) | Yes | No | Five named tests; command to verify. |
| 2.2.1 (SuggestedRuleCard component) | Yes | **Marginal** | References Storybook stories as criteria — Storybook is not in CI. The five Jest tests in 2.2.3 are the enforceable criteria; recommend removing the Storybook criterion or adding a snapshot test. |
| 2.2.2 (CSS) | Yes | No | "make lint passes; npx tsc --noEmit passes" — CI-enforced. |
| 2.2.3 (SuggestedRuleCard tests) | Yes | No | Five named tests; command to verify. |
| 3.1.1 (ApprovalRulesPanel changes) | Yes | No | References e2e test by filename and scenario. |
| 3.1.2 (e2e rules-generate-suggestions) | Yes | No | Four named test cases with convention checklist. |
| 4.1.1 (ReviewQueuePanel changes) | Yes | No | Modal + loading + mock response assertion. |
| 4.1.2 (e2e review-queue-create-rule) | Yes | No | Three named test cases. |
| 5.1.1–5.1.3 (Analytics panel) | Yes | No | Row-level loading, per-row card, cancel-on-new-row assertions. |
| 6.1.1 (command sample form) | Yes | No | Specific prefill + badge assertion named. |
| 6.1.2 (ApprovalRulesPanel.test.tsx) | Yes | No | Named test with 4 assertion steps. |
| 6.1.3 (e2e rule-from-command-sample) | Yes | No | Three named test cases. |

**Two marginal items identified:**
1. Story 1.3.3 has no standalone acceptance criterion for the cap-at-5 behavior. Add: "unit test asserts that a 6-element JSON array from the AI is truncated to 5 suggestions in the response."
2. Story 2.2.1 references Storybook stories as acceptance criteria. Storybook is not run in CI. Replace with or supplement by: "SuggestedRuleCard renders without TypeScript errors (npx tsc --noEmit passes); the Jest tests in 2.2.3 provide behavioral coverage."

Neither gap is blocking — both can be tightened during implementation without replanning.

---

### Gate 4: Adversarial blockers resolved — PASS

**BLOCKING Issue 1 — Singular response vs. plural UX (proto shape):**

The plan resolves this: Story 1.1 Task 1.1.2 defines `GenerateSuggestedRuleResponse` with `repeated SuggestedRuleProto suggestions = 1` (plural). Story 1.3.2 specifies that `parseSuggestions` handles a JSON array and the response carries `Suggestions`. Story 1.3.3 renames `parseSuggestion` → `parseSuggestions` returning `[]*sessionv1.SuggestedRuleProto`. The `useGenerateRule` hook (Story 2.1) exposes `suggestions: SuggestedRuleProto[]`. The plan adopts Option B (multi-suggestion response) consistently. **Resolved.**

**BLOCKING Issue 2 — Plan implements rejected Option A interface; ADR chose Option B:**

The plan resolves this: Story 1.2 is titled "Separated RulePromptBuilder + AIClient interfaces (ADR-001 Option B)". Task 1.2.1 creates `server/services/ai_interfaces.go` defining both `RulePromptBuilder` (pure, no I/O) and `AIClient` (transport) as separate interfaces. Task 1.2.2 implements `DefaultRulePromptBuilder` in its own file with pure prompt-building logic. Task 1.2.3 implements `AnthropicAIClient` separately. Task 1.2.5 wires both into `RulesService` via constructor injection. The original `RuleAIProvider` single-interface from the rejected Option A does not appear in the plan. **Resolved.**

Both BLOCKING items from the adversarial review are reflected correctly in plan.md.

---

## Overall Verdict: PASS WITH NOTES

The plan is ready to enter Phase 5 (implementation) subject to two non-blocking tightening items:

1. **Story 1.3.3** — add an explicit acceptance criterion for the array-cap-at-5 behavior (1 line in plan.md or in the unit test file stub).
2. **Story 2.2.1** — replace the Storybook acceptance criterion with a TypeScript-compile criterion that is CI-enforceable.

Neither item requires a replanning cycle. Both can be addressed in the implementation PR description or by the implementing developer inline.

The two BLOCKING issues from the adversarial review are resolved in the current plan. Security coverage (secret redaction + no-auto-save invariant) has dedicated tests. All 8 FRs have mapped happy-path and failure-mode tests.
