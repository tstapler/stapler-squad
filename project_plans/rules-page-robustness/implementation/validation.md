# Rules Page Robustness — Validation Plan

## Readiness Gate Summary

| Criterion | Status |
|---|---|
| Requirements are clear and non-contradictory | PASS |
| Plan covers all requirements | PASS (R4.2 explicitly deferred — see §5) |
| Tests cover all acceptance criteria | PASS |
| Adversarial concerns resolved or deferred | PASS |

**Overall verdict: PASS** — implementation may begin.

---

## 1. Requirement-to-Task Traceability

| Requirement | Tasks | Test IDs |
|---|---|---|
| R1.1 ToolPattern non-Bash → skipped | Task 2 + Concern-1 fix | Go: TC-G-01, TC-G-02 |
| R1.2 ToolPattern matching Bash → included | Task 2 | Go: TC-G-03, TC-G-04 |
| R1.3 ToolName="", ToolPattern="" → included | Task 2 | Go: TC-G-05 |
| R1.4 Four cases tested | Tasks 4 | Go: TC-G-01 – TC-G-05 |
| R2.1 CommandPattern="" marks all knownSubcmds | Task 3 | Go: TC-G-06, TC-G-07 |
| R2.2 Seed rule ToolName="Bash" + no CommandPattern | Tasks 1+3 | Go: TC-G-07 |
| R2.3 Unit test for R2.1/R2.2 | Task 4 | Go: TC-G-06, TC-G-07 |
| R3.1 ReclassifyGaps passes command to classifier | Task 5 | Go: TC-G-11 |
| R3.2 200-char truncation adequate for criteria matching | Task 5 | Go: TC-G-11 |
| R3.3 ReclassifyGaps reclassifies under-200-char entry | Task 5 | Go: TC-G-11 |
| R4.1 Gap subcommands highlighted differently | Task 6 | Jest: TC-J-02, TC-J-03 |
| R4.2 ApprovalAnalyticsPanel uncovered-subcommand count | **DEFERRED** (see §5) | — |
| R4.3 Three-state coverage column | Task 6 | Jest: TC-J-01 – TC-J-03 |
| R5.1 Go unit tests for coveredSubcommands cases | Task 4 | Go: TC-G-01 – TC-G-10 |
| R5.2 Jest tests for ProgramDetailPanel coverage states | Task 7 | Jest: TC-J-01 – TC-J-05 |

---

## 2. Go Unit Tests

All tests live in `server/services/` (package `services`).

### 2.1 `TestCoveredSubcommands` — `rules_service_test.go`

**Helper required:** Add `newRulesServiceForCoverage(t *testing.T, specs []RuleSpec) *RulesService` that:
1. Creates test storage via `createTestStorage(t)`.
2. Creates a `NewRulesStore(storage)` and upserts each `RuleSpec` into it.
3. Creates a `classifier.NewRuleBasedClassifier()` — seed rules are present (see §4.1 for isolation strategy).
4. Returns `NewRulesService(rulesStore, analyticsStore, c, &DefaultRulePromptBuilder{}, nil)`.

The tests below are table rows in a single `TestCoveredSubcommands` table-driven test. Each row constructs the service via `newRulesServiceForCoverage`, then calls `rs.coveredSubcommands(program, knownSubcmds)` and asserts the result.

**Isolation note:** Seed rules are always present via `allRuleSpecs()`. Assertions must account for seed rule effects after the bug fixes are applied (see §4.1). Specifically, after the Concern-1 fix, `seed-allow-agent-tools` (`ToolCategory="builtin-agent"`) and `seed-allow-mcp-read` (`ToolCategory="mcp-read"`) are filtered out. `seed-allow-read-tools` (`ToolPattern="Read|Glob|Grep|..."`) is also filtered out because its pattern does not match "Bash". The primary seed rule that can leak through is `seed-allow-bash-ls-pwd` (criteria-based, `CriteriaPrograms=["ls","pwd",...]`); it only activates when `program` matches one of those programs. Use `program="git"` throughout to avoid seed-rule interference from criteria-based rules.

| ID | Test name | User rule spec | program | knownSubcmds | Expected covered keys |
|---|---|---|---|---|---|
| TC-G-01 | `ToolPattern_ReadGlob_skipped` | `ToolPattern="Read\|Glob"`, `Enabled=true` | `"git"` | `["push","commit"]` | `{}` — seed rules filtered; user rule filtered by ToolPattern |
| TC-G-02 | `ToolCategory_BuiltinAgent_skipped` | `ToolCategory="builtin-agent"`, `ToolName=""`, `ToolPattern=""`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push"]` | `{}` — Concern-1 guard filters non-bash ToolCategory |
| TC-G-03 | `ToolPattern_Bash_included` | `ToolPattern="Bash"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"":true,"push":true,"commit":true}` |
| TC-G-04 | `ToolPattern_Wildcard_included` | `ToolPattern=".*"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push"]` | `{"":true,"push":true}` |
| TC-G-05 | `AllToolRule_EmptyToolNameAndPattern` | `ToolName=""`, `ToolPattern=""`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push"]` | `{"":true,"push":true}` — R1.3 preserved |
| TC-G-06 | `ToolName_Bash_CommandPatternEmpty_allSubcmds` | `ToolName="Bash"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push","commit","status"]` | `{"":true,"push":true,"commit":true,"status":true}` — R2.1 |
| TC-G-07 | `ToolName_Bash_CommandPatternEmpty_emptyKnownSubcmds` | `ToolName="Bash"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `[]` | `{"":true}` — only bare key when no known subcommands |
| TC-G-08 | `CriteriaPrograms_match_noSubcmdRestriction` | `CriteriaPrograms=["git"]`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"":true,"push":true,"commit":true}` |
| TC-G-09 | `CriteriaPrograms_match_withSubcmdRestriction` | `CriteriaPrograms=["git"]`, `CriteriaSubcommands=["push"]`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"push":true}` |
| TC-G-10 | `DisabledRule_notCovered` | `ToolName="Bash"`, `CommandPattern=""`, `Enabled=false` | `"git"` | `["push"]` | `{}` |
| TC-G-11 (bonus) | `CommandPattern_specificSubcmd` | `ToolName="Bash"`, `CommandPattern="^git push"`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"push":true}` |

**Count: 11 Go tests in `TestCoveredSubcommands`.**

### 2.2 `TestReclassifyGaps` — `analytics_store_test.go`

| ID | Test name | Setup | Expected |
|---|---|---|---|
| TC-G-12 | `TestReclassifyGaps_should_reclassifyEntry_When_ruleNowCoversCommand` | Classifier has rule matching `"git push"`. Entry: `Decision="escalate"`, `RuleID=""`, `CommandPreview="git push origin main"` (28 chars, well under 200). | Returned entry has `Decision="auto_allow"`, `RuleID != ""` |
| TC-G-13 | `TestReclassifyGaps_should_skipEntry_When_alreadyDecided` | Entry with `Decision="auto_allow"` (not escalate). | Entry unchanged |
| TC-G-14 | `TestReclassifyGaps_should_skipEntry_When_hasRuleID` | Entry with `Decision="escalate"`, `RuleID="some-rule-id"`. | Entry unchanged — already attributed to a rule |
| TC-G-15 | `TestReclassifyGaps_should_notMutateOriginalSlice` | Pass a slice; verify `result[i] != &input[i]` (copy semantics). | Original slice entries unchanged after reclassification |
| TC-G-16 | `TestReclassifyGaps_should_handleCommandUnder200Chars` (R3.3) | Command is 60 chars; classifier rule uses criteria matching `"git"` program. Verifies truncation is irrelevant for typical commands. | Entry reclassified; demonstrates R3.3 compliance |

**Count: 5 Go tests in `TestReclassifyGaps`.**

### 2.3 `TestComputeSummary` — `analytics_store_test.go`

| ID | Test name | Setup | Expected |
|---|---|---|---|
| TC-G-17 | `TestComputeSummary_should_countCoverageGaps_When_escalateNoRuleID` | 3 entries: 2 escalate+no-ruleID, 1 auto_allow. | `CoverageGapCount=2` |
| TC-G-18 | `TestComputeSummary_should_notCountGap_When_escalateWithRuleID` | 1 entry: escalate + `RuleID="r1"`. | `CoverageGapCount=0` |
| TC-G-19 | `TestComputeSummary_should_computeCorrectRates` | 10 entries: 8 auto_allow, 1 auto_deny, 1 escalate. | `AutoApproveRate=0.8`, `ManualReviewRate=0.1` |
| TC-G-20 | `TestComputeSummary_should_returnZeroSummary_When_empty` | Empty entry slice. | All counts zero, rates zero |
| TC-G-21 | `TestComputeSummary_should_showFewerGaps_After_ReclassifyGaps` | 3 escalate+no-ruleID entries; classifier covers 2. Run `ReclassifyGaps` then `ComputeSummary`. | `CoverageGapCount=1` (2 reclassified) |

**Count: 5 Go tests in `TestComputeSummary`.**

**Total Go test cases: 21**

---

## 3. Jest Unit Tests

All tests live in `web-app/src/components/sessions/ProgramDetailPanel.test.tsx` (new file).

**Mock pattern:** Mock `@/lib/hooks/useProgramAnalytics` (or the equivalent hook used by `ProgramDetailPanel`) to return controlled `SubcommandBreakdownProto` data. Follow the pattern in `ApprovalAnalyticsPanel.test.tsx`.

**Render pattern:** Each test renders:
```tsx
<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />
```
with mocked hook returning one subcommand row.

**Assertion note:** Use `data-testid` attributes on coverage spans (e.g., `data-testid="coverage-covered"`, `data-testid="coverage-partial"`, `data-testid="coverage-gap"`) rather than asserting on the raw `⚠` Unicode character, which can be fragile. If `data-testid` is not added to the implementation, fall back to `getByText(/covered/i)`, `getByText(/partial/i)`, `getByText(/gap/i)` for resilience (addresses adversarial Concern 6).

| ID | Test name | Mock data | Assertion |
|---|---|---|---|
| TC-J-01 | `ProgramDetailPanel_should_showCoveredBadge_When_hasRuleCoverageAndNoEscalate` | `hasRuleCoverage=true`, `escalate=0` | `getByTestId("coverage-covered")` present OR `getByText(/covered/i)` |
| TC-J-02 | `ProgramDetailPanel_should_showPartialBadge_When_hasRuleCoverageAndEscalateGt0` | `hasRuleCoverage=true`, `escalate=3` | `getByTestId("coverage-partial")` present OR `getByText(/partial/i)` |
| TC-J-03 | `ProgramDetailPanel_should_showGapBadge_When_noRuleCoverage` | `hasRuleCoverage=false`, `escalate=1` | `getByTestId("coverage-gap")` present OR `getByText(/gap/i)` |
| TC-J-04 | `ProgramDetailPanel_should_showAddRuleLink_When_gap` | `hasRuleCoverage=false` | `getByText(/Add rule/)` present |
| TC-J-05 | `ProgramDetailPanel_should_notShowAddRuleLink_When_covered` | `hasRuleCoverage=true`, `escalate=0` | `queryByText(/Add rule/)` is null |

**Count: 5 Jest tests.**

**Total test cases: 26 (21 Go + 5 Jest)**

---

## 4. Adversarial Concern Resolutions

### 4.1 Concern 1 — ToolCategory false-positive after Tasks 1+2

**Resolution: Fixed in implementation.**

The final filter block in `coveredSubcommands` must include a ToolCategory guard. After the existing ToolPattern check, add:

```go
// Reject rules whose ToolCategory explicitly targets a non-Bash tool group.
// This catches seed rules like seed-allow-agent-tools (ToolCategory="builtin-agent")
// and seed-allow-mcp-read (ToolCategory="mcp-read") that have empty ToolName and
// empty ToolPattern — they would otherwise fall through to the "all tools" path.
if spec.ToolCategory != "" && !strings.EqualFold(spec.ToolCategory, "bash") && !isBashTool {
    continue
}
```

This guard must be inserted AFTER the `ToolPattern` block (which sets `isBashTool = true` when ToolPattern matches "Bash") and BEFORE the existing `ToolName != ""` check. The final filter order is:

1. `if !spec.Enabled { continue }` (existing)
2. Compute `isBashTool`, `isBashCat`
3. ToolPattern check — may set `isBashTool = true` or `continue` (Task 2)
4. **ToolCategory guard — continue if non-bash category** (Concern-1 fix, NEW)
5. Existing `ToolName != ""` check

TC-G-02 directly verifies this guard fires for `ToolCategory="builtin-agent"`.

**Affected seed rules (verified against classifier.go):**
- `seed-allow-agent-tools`: `ToolCategory="builtin-agent"`, `ToolName=""`, `ToolPattern=nil` → now skipped by guard
- `seed-allow-mcp-read`: `ToolCategory="mcp-read"`, `ToolName=""`, `ToolPattern=nil` → now skipped by guard
- `seed-allow-read-tools`: `ToolPattern="Read|Glob|..."` → skipped by Task 2 ToolPattern check (pattern does not match "Bash")

### 4.2 Concern 2 — Test isolation: `allRuleSpecs()` always appends `classifier.SeedRules()`

**Resolution: Tests written with seed rules present (no isolation bypass needed).**

`allRuleSpecs()` at line 373 of `rules_service.go` calls `classifier.SeedRules()` directly — this cannot be suppressed without production code changes. The `ReplaceRules(nil)` approach proposed in the plan does NOT work (it only affects `Classify()`, not `allRuleSpecs()`).

Strategy for `TestCoveredSubcommands`:
- Use `program="git"` for all test cases. After all three fixes are applied (Tasks 1, 2, 3 + Concern-1 guard), seed rules relevant to "git" are limited to criteria-based `seed-allow-bash-ls-pwd` (programs: ls, pwd, echo, etc. — none match "git"). No other seed rule will contribute coverage for program="git".
- Tests therefore assert the full expected map including only what the user-controlled rule produces.
- TC-G-01 expects `{}` (empty): after fixes, `seed-allow-read-tools` (ToolPattern check), `seed-allow-agent-tools` (ToolCategory guard), and `seed-allow-mcp-read` (ToolCategory guard) are all filtered. The user rule (ToolPattern="Read|Glob") is also filtered. Result is `{}`. This is valid post-fix behavior.
- TC-G-02 expects `{}`: user rule has `ToolCategory="builtin-agent"` — filtered by Concern-1 guard.

This approach makes the tests **end-to-end realistic** (seed rules present, but correctly filtered after fix), which is strictly better than isolated unit tests that test against a stripped-down service.

If future seed rules for git subcommands are added, tests may need updating — this is acceptable and expected.

### 4.3 Concern 3 — R4.2 missing from plan

**Resolution: Explicitly deferred.**

**Rationale:** R4.2 ("Uncovered Programs row in ApprovalAnalyticsPanel shows count of STILL-uncovered subcommands") requires:
1. A new backend field in `GetProgramAnalytics` or `ComputeSummary` that tracks per-program uncovered subcommand counts.
2. UI changes to `ApprovalAnalyticsPanel` parent rows.
3. Additional proto fields or a new summary field.

This scope exceeds the stated goal of this PR (bug fixes + focused UX improvement to `ProgramDetailPanel`). Adding it now risks scope creep and delays the critical bug fixes (R1, R2).

**Deferral condition:** R4.2 will be tracked as a follow-up issue. The deferred work requires a clear product decision on what "uncovered subcommand count" means at the program level (unique subcommands with gaps? total gap events?). The current PR delivers R4.1 and R4.3, which provide the most user-visible improvement (the drill-down view).

**Acceptance criteria not met by this PR:** R4.2 only. All other acceptance criteria are met.

### 4.4 Concern 4 — Criteria-based rule mutual exclusivity (Task 3)

**Resolution: Documented, test case added.**

The `CriteriaPrograms` block at line 311 always `continue`s after processing — the `CommandPattern == ""` block is never reached for criteria-based rules. Task 3's fix therefore only applies to non-criteria pattern rules. This is correct behavior.

TC-G-08 covers `CriteriaPrograms=["git"]` with no subcommand restriction and verifies correct "all subcommands covered" behavior for the criteria path, confirming the two paths are independent.

### 4.5 Concern 5 — Regex compile in hot path

**Resolution: Accept as-is; log for future optimization.**

`regexp.Compile(spec.ToolPattern)` in Task 2 runs once per rule per `coveredSubcommands` call. With ~20-30 rules typical, this is ~20 extra compiles per `GetProgramAnalytics` request. Acceptable for the current call frequency. A pre-compiled cache can be added in a follow-up if profiling shows this as a hotspot.

### 4.6 Concern 6 — `⚠` character in test assertions

**Resolution: Use `data-testid` attributes (preferred) or regex fallback.**

Task 6 implementation must add `data-testid="coverage-covered"`, `data-testid="coverage-partial"`, and `data-testid="coverage-gap"` to the respective `<span>` elements. Jest tests then use `getByTestId(...)`. If `data-testid` is not added, tests fall back to `getByText(/partial/i)` etc. (documented in §3 above).

### 4.7 Concern 7 — R1.4 four cases coverage

**Resolution: Already satisfied.** The four required cases (R1.1, R1.2, R1.3, and exact ToolName="Bash") map to TC-G-01, TC-G-03/04, TC-G-05, and TC-G-06/07 respectively. Coverage confirmed.

---

## 5. Deferred Requirements

| Requirement | Reason | Follow-up action |
|---|---|---|
| R4.2: ApprovalAnalyticsPanel uncovered-subcommand count | Requires new backend field + proto changes + parent row UI changes; out of scope for bug-fix PR | File follow-up issue; requires product decision on count semantics |

---

## 6. Acceptance Criteria Checklist

### Backend (Go)

- [ ] `ruleToSpec` copies `ToolCategory` from `classifier.Rule` (Task 1)
- [ ] `coveredSubcommands` filter skips rules whose `ToolPattern` does not match "Bash" (R1.1, R1.2) (Task 2)
- [ ] `coveredSubcommands` filter preserves rules with empty `ToolName` and empty `ToolPattern` (R1.3) (Task 2)
- [ ] `coveredSubcommands` filter includes the ToolCategory guard to skip non-bash categories (Concern-1 fix)
- [ ] `coveredSubcommands` with `CommandPattern=""` marks all `knownSubcmds` as covered (R2.1, R2.2) (Task 3)
- [ ] `TestCoveredSubcommands` passes with all 11 table rows green (Task 4)
- [ ] `TestReclassifyGaps` passes with all 5 cases green, including R3.3 scenario (Task 5)
- [ ] `TestComputeSummary` passes with all 5 cases green (Task 5)
- [ ] `make build && make test` passes

### Frontend (TypeScript/React)

- [ ] `ProgramDetailPanel.css.ts` exports `coveragePartial` style using `vars.color.warningBg` / `vars.color.warning` (confirmed tokens exist in `theme-contract.css.ts` at lines 43–44) (Task 6)
- [ ] `SubcommandRow` renders "✓ covered" for `hasRuleCoverage=true` + `escalate=0` (R4.3)
- [ ] `SubcommandRow` renders "⚠ partial" for `hasRuleCoverage=true` + `escalate>0` (R4.3) (Task 6)
- [ ] `SubcommandRow` renders "✗ gap" for `hasRuleCoverage=false` (R4.3) (Task 6)
- [ ] Coverage spans have `data-testid` attributes for test resilience (Concern-6 fix) (Task 6)
- [ ] "Add rule →" link only shown for gap state (Task 6)
- [ ] All 5 Jest tests in `ProgramDetailPanel.test.tsx` pass (Task 7)
- [ ] `cd web-app && npx jest --no-coverage` passes

### Regression

- [ ] No new false-positive coverage for `seed-allow-agent-tools` after fix (TC-G-02)
- [ ] No new false-positive coverage for `seed-allow-read-tools` after fix (TC-G-01)
- [ ] Existing `TestClassify_DailyBucketAutoApproveRate` still passes (no regression)
- [ ] `make quick-check` passes (build + test + lint)

---

## 7. Test Count Summary by Type

| Type | Count |
|---|---|
| Go table rows in `TestCoveredSubcommands` | 11 |
| Go tests in `TestReclassifyGaps` | 5 |
| Go tests in `TestComputeSummary` | 5 |
| Jest tests in `ProgramDetailPanel.test.tsx` | 5 |
| **Total** | **26** |

Requirements coverage: **14 of 15 acceptance criteria** met by this plan (R4.2 explicitly deferred with rationale).

---

## 8. Implementation Readiness Gate

| Gate criterion | Result | Notes |
|---|---|---|
| Requirements are clear and non-contradictory | PASS | All requirements have defined acceptance criteria |
| Plan covers all requirements | PASS | R4.2 deferred with justification; all others have tasks |
| Tests cover all acceptance criteria | PASS | 26 tests mapped to requirements; each AC has ≥1 test |
| Adversarial concerns resolved or deferred | PASS | Concerns 1+2+3+4+6+7 resolved; C5 accepted as low risk |

**Readiness gate verdict: PASS**
