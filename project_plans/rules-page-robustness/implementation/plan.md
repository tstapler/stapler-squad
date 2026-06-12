# Rules Page Robustness — Implementation Plan

## Overview

7 tasks fix two Go backend bugs, add missing test coverage, and improve a frontend UX detail. Tasks 1–3 are backend bug fixes; Tasks 4–5 are Go unit tests (verify the fixes are correct); Task 6 is a frontend UX enhancement; Task 7 is a frontend unit test.

**Dependencies:**
- Task 4 depends on Tasks 2 and 3 being complete (tests validate the fixed code)
- Task 5 is independent (covers existing `ReclassifyGaps`/`ComputeSummary` functions)
- Task 6 and 7 are independent of each other and of the Go tasks
- Task 7 depends on Task 6 (tests the enhanced UI)

---

## Task 1 — Fix `ruleToSpec`: copy `ToolCategory` from classifier.Rule

**File:** `server/services/rules_service.go`  
**Lines:** 427–453 (`ruleToSpec` function)  
**Bug B from root cause analysis.**

`ruleToSpec` converts a `classifier.Rule` to a `RuleSpec` but never copies `r.ToolCategory`. Seed rules with `ToolCategory = "builtin-agent"` or `ToolCategory = "mcp-read"` always produce `spec.ToolCategory = ""`, so the `isBashCat` check in `coveredSubcommands` never fires for them.

**Change:** Add one field assignment inside the `ruleToSpec` struct literal:

```go
// existing fields:
spec := RuleSpec{
    ID:          r.ID,
    Name:        r.Name,
    ToolName:    r.ToolName,
    // ADD THIS:
    ToolCategory: r.ToolCategory,
    ...
}
```

Exact location: after `ToolName: r.ToolName,` at line ~431.

**Acceptance:** After this fix, `isBashCat` will correctly fire for seed rules whose `classifier.Rule.ToolCategory` is `"builtin-agent"` or `"mcp-read"` — causing them to be skipped by the `!isBashCat` check.

---

## Task 2 — Fix `coveredSubcommands`: check `ToolPattern` for Bash relevance

**File:** `server/services/rules_service.go`  
**Lines:** 300–309 (the tool-filter block inside `coveredSubcommands`)  
**Bug A from root cause analysis.**

Current filter (lines 305–309):
```go
isBashTool := strings.EqualFold(spec.ToolName, "Bash")
isBashCat := strings.EqualFold(spec.ToolCategory, "bash")
if !isBashTool && !isBashCat && spec.ToolName != "" {
    continue
}
```

Rules with `ToolName = ""` and `ToolPattern = "Read|Glob|Grep|..."` pass through this filter because `spec.ToolName == ""` satisfies `spec.ToolName != ""` as false. They then set `covered[""] = true`, producing false-positive coverage.

**Replace the filter block** (lines 305–309) with:

```go
isBashTool := strings.EqualFold(spec.ToolName, "Bash")
isBashCat := strings.EqualFold(spec.ToolCategory, "bash")
// If a ToolPattern is set (and ToolName is empty), check whether the pattern
// actually matches "Bash". A pattern like "Read|Glob|Grep" does not match Bash;
// a pattern like "Bash" or ".*" does.
if spec.ToolPattern != "" && !isBashTool {
    re, err := regexp.Compile(spec.ToolPattern)
    if err != nil || !re.MatchString("Bash") {
        continue // pattern excludes Bash → skip
    }
    isBashTool = true // treat as bash-applicable
}
if !isBashTool && !isBashCat && spec.ToolName != "" {
    continue
}
```

**Note:** `regexp` is already imported in this file. No new imports needed.

**Acceptance criteria (R1.1–R1.3):**
- R1.1: `ToolPattern="Read|Glob|Grep"` → skipped (does not match "Bash")
- R1.2: `ToolPattern="Bash"` or `ToolPattern=".*"` → included
- R1.3: `ToolName=""`, `ToolPattern=""` → falls through to existing logic (included)

---

## Task 3 — Fix `coveredSubcommands`: `CommandPattern==""` marks all known subcommands

**File:** `server/services/rules_service.go`  
**Lines:** 335–339 (`if spec.CommandPattern == ""` block inside `coveredSubcommands`)  
**Bug C from root cause analysis.**

Current code (lines 335–339):
```go
if spec.CommandPattern == "" {
    covered[""] = true
    continue
}
```

A rule that covers all Bash (e.g., `ToolName="Bash"`, `CommandPattern=""`) only marks the bare-program key. Specific subcommands like `"push"`, `"commit"` remain uncovered, so the drill-down table shows them as gaps even though a broad "allow all Bash" rule exists.

**Replace** with:
```go
if spec.CommandPattern == "" {
    covered[""] = true
    for _, sub := range knownSubcmds {
        covered[sub] = true
    }
    continue
}
```

**Acceptance criteria (R2.1–R2.2):**
- R2.1: A rule with `ToolName="Bash"`, `CommandPattern=""`, no `CriteriaPrograms` marks all `knownSubcmds` as covered.
- R2.2: The `""` key (bare program call) also remains covered.

---

## Task 4 — Go unit tests: `coveredSubcommands`

**File:** `server/services/rules_service_test.go`  
**Depends on:** Tasks 1, 2, 3  
**No existing tests for `coveredSubcommands` — add new test function.**

Add `TestCoveredSubcommands` as a table-driven test after the existing helpers. The test must call `coveredSubcommands` directly; since it is a method on `*RulesService`, construct a minimal service via `newRulesServiceWithAI` (or a direct constructor) with the relevant rules in its store.

Because `coveredSubcommands` reads from `allRuleSpecs()` (which merges user rules + seed rules), the simplest approach is to seed a `RulesStore` with controlled `RuleSpec` entries.

**Test cases (all table rows in a single `TestCoveredSubcommands` function):**

| Test name | Input rule spec | `program` | `knownSubcmds` | Expected `covered` |
|---|---|---|---|---|
| `ToolPattern_ReadGlob_skipped` | `ToolPattern="Read\|Glob"`, `Enabled=true` | `"git"` | `["push","commit"]` | `{}` (empty — rule skipped) |
| `ToolPattern_Bash_included` | `ToolPattern="Bash"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"":true,"push":true,"commit":true}` |
| `ToolPattern_wildcard_included` | `ToolPattern=".*"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push"]` | `{"":true,"push":true}` |
| `ToolName_Bash_allSubcmds` | `ToolName="Bash"`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push","commit","status"]` | `{"":true,"push":true,"commit":true,"status":true}` |
| `AllToolRule_ToolNameEmpty_ToolPatternEmpty` | `ToolName=""`, `ToolPattern=""`, `CommandPattern=""`, `Enabled=true` | `"git"` | `["push"]` | `{"":true,"push":true}` |
| `CriteriaPrograms_match_noSubcmds` | `CriteriaPrograms=["git"]`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"":true,"push":true,"commit":true}` |
| `CriteriaPrograms_match_withSubcmds` | `CriteriaPrograms=["git"]`, `CriteriaSubcommands=["push"]`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"push":true}` |
| `CriteriaPrograms_noMatch` | `CriteriaPrograms=["npm"]`, `Enabled=true` | `"git"` | `["push"]` | `{}` |
| `CommandPattern_specificSubcmd` | `ToolName="Bash"`, `CommandPattern="^git push"`, `Enabled=true` | `"git"` | `["push","commit"]` | `{"push":true}` |
| `DisabledRule_notCovered` | `ToolName="Bash"`, `CommandPattern=""`, `Enabled=false` | `"git"` | `["push"]` | `{}` |

**Important implementation note:** `coveredSubcommands` calls `rs.allRuleSpecs()`, which appends seed rules from `classifier.SeedRules()`. To get clean isolation, the test should construct a `RulesService` with an empty store and with a `RuleBasedClassifier` that has been cleared of seed rules (using `c.ReplaceRules([]classifier.Rule{})` after construction), then call `rulesStore.Upsert(spec)` + `rs.rebuildClassifier()` to populate only the desired rule.

Alternatively: since `coveredSubcommands` is not exported, use a thin exported helper `testCoveredSubcommands` placed under a `_test.go` build tag, or test via the exported `GetProgramAnalytics` RPC. The simplest approach is to make the function accessible to the `_test.go` file in the same package (it already is, since both are `package services`).

**Test helper pattern:** Follow the existing `newRulesServiceWithAI` pattern. Add a `newRulesServiceForCoverage(t, specs []RuleSpec)` helper that:
1. Creates test storage
2. Upserts each spec into the store
3. Creates a `RuleBasedClassifier` with NO seed rules (`c.ReplaceRules(nil)`)
4. Returns the `*RulesService`

---

## Task 5 — Go unit tests: `ReclassifyGaps` and `ComputeSummary`

**File:** `server/services/analytics_store_test.go`  
**Independent of Tasks 1–3.**

The current `analytics_store_test.go` contains only one test for `DailyBucket.AutoApproveRate()`. Add:

### TestReclassifyGaps

```
TestReclassifyGaps_should_reclassifyEntry_When_ruleNowCoversCommand
TestReclassifyGaps_should_skipEntry_When_alreadyDecided
TestReclassifyGaps_should_skipEntry_When_hasRuleID
TestReclassifyGaps_should_notMutateOriginalSlice
TestReclassifyGaps_should_handleCommandUnder200Chars (R3.3)
```

Key scenario for R3.3: create a `RuleBasedClassifier` with a rule matching `"git push"` (under 200 chars), create an `AnalyticsEntry` with `Decision="escalate"`, `RuleID=""`, `CommandPreview="git push origin main"`, then verify `ReclassifyGaps` returns the entry with `Decision="auto_allow"`.

### TestComputeSummary

```
TestComputeSummary_should_countCoverageGaps_When_escalateNoRuleID
TestComputeSummary_should_notCountGap_When_escalateWithRuleID
TestComputeSummary_should_computeCorrectRates
TestComputeSummary_should_returnZeroSummary_When_empty
```

These tests verify:
- `CoverageGapCount` is incremented only for `escalate` + `RuleID == ""`
- `CoverageGapRate` is calculated as `(gapCount / total) * 100`
- After `ReclassifyGaps` re-classifies entries, `ComputeSummary` on the result shows fewer gaps

---

## Task 6 — Frontend UX: three-state coverage column in `ProgramDetailPanel`

**Files:**
- `web-app/src/components/sessions/ProgramDetailPanel.tsx` (lines 191–204, the Coverage `<td>`)
- `web-app/src/components/sessions/ProgramDetailPanel.css.ts` (add `coveragePartial` style)

### Change to `ProgramDetailPanel.css.ts`

Add a `coveragePartial` style after `coverageYes` (line 100–106):

```ts
export const coveragePartial = style([
  coverageBadge,
  {
    background: vars.color.warningBg,
    color: vars.color.warning,
  },
]);
```

### Change to `ProgramDetailPanel.tsx` — `SubcommandRow` component (lines 191–204)

Replace the binary `hasRuleCoverage` check with three-state logic:

```tsx
// Determine coverage status: covered / partial / gap
const hasEscalated = row.escalate > 0;
let coverageCell: React.ReactNode;
if (row.hasRuleCoverage && !hasEscalated) {
  coverageCell = <span className={styles.coverageYes}>✓ covered</span>;
} else if (row.hasRuleCoverage && hasEscalated) {
  coverageCell = <span className={styles.coveragePartial}>⚠ partial</span>;
} else {
  coverageCell = <span className={styles.coverageNo}>✗ gap</span>;
}
```

And update the Coverage `<td>` to render `{coverageCell}`.

Also update the "Add rule →" link condition — only show it when `!row.hasRuleCoverage` (gap state). Partial coverage may still benefit from a more specific rule, but don't change that logic per requirements (out of scope).

**Data source:** `SubcommandBreakdownProto` already has `escalate: number` (from `row.escalate` used in the `manual` calculation at line 170). No proto changes required.

**Acceptance criteria (R4.3):**
- `hasRuleCoverage=true`, `escalate=0` → "✓ covered" (green)
- `hasRuleCoverage=true`, `escalate>0` → "⚠ partial" (warning/amber)
- `hasRuleCoverage=false` → "✗ gap" (red)

---

## Task 7 — Frontend unit tests: `ProgramDetailPanel`

**File:** `web-app/src/components/sessions/ProgramDetailPanel.test.tsx` (new file)  
**Depends on:** Task 6  
**Pattern:** Follow `ApprovalAnalyticsPanel.test.tsx`

### Mock setup

Mock `@/lib/hooks/useProgramAnalytics` to return controlled `SubcommandBreakdownProto` data.

### Test cases

```
ProgramDetailPanel_should_showCoveredBadge_When_hasRuleCoverageAndNoEscalate
ProgramDetailPanel_should_showPartialBadge_When_hasRuleCoverageAndEscalateGt0
ProgramDetailPanel_should_showGapBadge_When_noRuleCoverage
ProgramDetailPanel_should_showAddRuleLink_When_gap
ProgramDetailPanel_should_notShowAddRuleLink_When_covered
```

Each test renders `<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />` with a mocked hook returning one subcommand row with the relevant `hasRuleCoverage`/`escalate` values. Assert `getByText("✓ covered")`, `getByText("⚠ partial")`, or `getByText("✗ gap")` respectively.

---

## Adversarial Risks

1. **Seed rules cleared in tests**: Task 4 requires disabling seed rules. If `RuleBasedClassifier.ReplaceRules(nil)` is called but `allRuleSpecs()` still appends `classifier.SeedRules()` directly (it does — see line 373), the test isolation strategy above (clearing the classifier but not the seed-rule call in `allRuleSpecs`) will fail. The correct approach is to use only user-rules-store specs and verify that the test-constructed specs produce the expected coverage output when the seed rules are present. OR: refactor `allRuleSpecs` to accept an option to skip seeds (invasive). The safest approach: construct `RuleSpec` entries that would dominate over seed rules by priority and test the output. Document this constraint.

2. **`ToolPattern` already compiled in `ruleToSpec`**: Task 2 introduces a `regexp.Compile(spec.ToolPattern)` in the hot path. If `spec.ToolPattern` is already a pre-compiled regex source string (it is — `ruleToSpec` stores `r.ToolPattern.String()`), this adds one compile per rule per call to `coveredSubcommands`. For the typical dozen rules this is fine, but worth noting.

3. **`isBashCat` checks `"bash"` (lowercase)**: The `isBashCat` check `strings.EqualFold(spec.ToolCategory, "bash")` will never fire for seed rules using `ToolCategoryBuiltinAgent = "builtin-agent"` — these are NOT Bash tools and should be skipped. Task 1's fix (copying `ToolCategory`) means these rules now have `spec.ToolCategory = "builtin-agent"`, which is not `"bash"`. Combined with `ToolName = ""` and `ToolPattern` potentially empty, they will NOT be filtered by the existing `ToolName != ""` check. Verify that seed rules with `ToolCategory="builtin-agent"` also have `ToolPattern` set (or accept that they fall through to `CommandPattern` matching, which is harmless since their `CommandPattern` won't match bash commands).

4. **`covered[""]` semantics**: The `""` key represents "bare program name with no subcommand" (e.g., just `git` with no args). Task 3's fix sets this key AND all known subcommands. If a program is called without args (`git` alone) and a subcommand-specific rule exists, the existing logic correctly handles this. The new code doesn't change that. However: if `knownSubcmds` is empty (no data in the window), both the old and new code only set `covered[""] = true`. This is correct — there are no subcommands to mark.

5. **Frontend `vars.color.warningBg` / `vars.color.warning` existence**: Task 6 uses `vars.color.warningBg` and `vars.color.warning`. Verify these tokens exist in `web-app/src/styles/theme-contract.css.ts` before writing code. The CSS architecture rules require using `vars.xxx` references — if these tokens don't exist, they must be added to the theme contract first.

6. **`SubcommandBreakdownProto.escalate` type**: Task 6 reads `row.escalate` which is used at line 170 of `ProgramDetailPanel.tsx` (`const manual = row.escalate + row.manualAllow + row.manualDeny`). This field already exists and is numeric. No issue.

---

## Execution Order

```
Task 1 (ruleToSpec fix)     ─┐
Task 2 (ToolPattern filter)  ├─► Task 4 (Go tests for coveredSubcommands)
Task 3 (CommandPattern fix) ─┘

Task 5 (ReclassifyGaps/ComputeSummary tests)   ← independent

Task 6 (ProgramDetailPanel UX) ─► Task 7 (ProgramDetailPanel tests)
```

Tasks 1+2+3 can be implemented in a single commit since they are all in `rules_service.go`. Tasks 4+5 can be done in a follow-up commit or alongside the fixes. Tasks 6+7 are a separate frontend commit.

---

## Files Changed Summary

| File | Task(s) | Change type |
|---|---|---|
| `server/services/rules_service.go` | 1, 2, 3 | Edit (3 targeted changes) |
| `server/services/rules_service_test.go` | 4 | Edit (add ~120 lines) |
| `server/services/analytics_store_test.go` | 5 | Edit (add ~80 lines) |
| `web-app/src/components/sessions/ProgramDetailPanel.tsx` | 6 | Edit (SubcommandRow coverage cell) |
| `web-app/src/components/sessions/ProgramDetailPanel.css.ts` | 6 | Edit (add coveragePartial style) |
| `web-app/src/components/sessions/ProgramDetailPanel.test.tsx` | 7 | Create (new file, ~80 lines) |
