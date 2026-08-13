# Adversarial Review — Rules Page Robustness Implementation Plan

**Verdict: CONCERNS**

The plan is directionally correct and the core bug fixes are accurate. However, several issues need to be addressed before implementation begins — none are fatal blockers, but two would produce incorrect runtime behavior if left uncorrected.

---

## Issues Found

### CONCERN 1 — Task 1 fix is incomplete: `isBashCat` logic is wrong for the actual bug

**Severity: Medium (logical error in stated fix rationale)**

The plan states that Task 1 fixes Bug B: "After this fix, `isBashCat` will correctly fire for seed rules whose `classifier.Rule.ToolCategory` is `"builtin-agent"` or `"mcp-read"` — causing them to be skipped by the `!isBashCat` check."

This is incorrect. `isBashCat` checks for `strings.EqualFold(spec.ToolCategory, "bash")`. `"builtin-agent"` and `"mcp-read"` are NOT `"bash"`. So even after copying `ToolCategory`, `isBashCat` will remain `false` for those seed rules. The `!isBashCat` arm does NOT fire.

**What actually happens after Task 1:** The `ToolCategory` is now copied. For `seed-allow-agent-tools` (`ToolCategory="builtin-agent"`, `ToolName=""`, `ToolPattern=nil → ""`):
- `isBashTool = false` (ToolName is "")
- `isBashCat = false` (ToolCategory "builtin-agent" ≠ "bash")
- `spec.ToolName == ""` → the `spec.ToolName != ""` guard is false → rule is NOT skipped

This means Task 1's fix alone does NOT eliminate false-positive coverage from `seed-allow-agent-tools` and `seed-allow-mcp-read`. These rules still pass through the filter even after Task 1. They're only handled correctly once Task 2 is applied: because those seed rules have `ToolPattern = nil → ""`, they fall to the third case (R1.3: `ToolName=""`, `ToolPattern=""`), meaning they ARE included in coverage analysis. 

**Root question:** Should `seed-allow-agent-tools` (ToolCategory="builtin-agent") be included or excluded from coverage analysis for Bash subcommands?

Looking at the seed rule: `seed-allow-agent-tools` has `ToolCategory = "builtin-agent"`, `ToolName = ""`, `ToolPattern = nil`, `CommandPattern = nil`. Per the classifier logic, this rule matches tools categorized as "builtin-agent" (e.g., `TodoWrite`, `TaskCreate`). It does NOT match Bash. But in `coveredSubcommands`, after Tasks 1+2, it will reach the `spec.ToolPattern == ""` → R1.3 path and be included (since `ToolName=""` and `ToolPattern=""`), potentially setting `covered[""] = true` and all known subcommands as covered!

**This is a NEW false-positive introduced by Tasks 1+2 together.** The ToolCategory-only seed rules (`seed-allow-agent-tools`, `seed-allow-mcp-read`) have no ToolPattern and no ToolName — they will slip through the Task 2 fix and produce incorrect coverage for ALL programs.

**Mitigation:** The fix for `coveredSubcommands` needs an additional guard:
```go
// After the ToolPattern check, also check ToolCategory
if spec.ToolCategory != "" && spec.ToolCategory != "bash" && !isBashTool {
    continue // rule targets a non-Bash tool category
}
```
This needs to be documented in the plan explicitly.

---

### CONCERN 2 — Task 4 test isolation: `allRuleSpecs()` always appends `classifier.SeedRules()` directly

**Severity: High (tests will not be isolated)**

The plan's "Important implementation note" acknowledges this but the proposed mitigation ("use `c.ReplaceRules(nil)` to clear the classifier") will NOT work. `allRuleSpecs()` at line 373 calls `classifier.SeedRules()` directly:

```go
for _, r := range classifier.SeedRules() {
    all = append(all, ruleToSpec(r))
}
```

Calling `c.ReplaceRules(nil)` on the `RuleBasedClassifier` only affects the classifier's internal rule list used by `Classify()`. It does NOT affect the `allRuleSpecs()` seed enumeration. There is NO way to suppress seed rules from `allRuleSpecs()` without modifying the production code or using a dependency injection approach.

**Consequence:** The test cases expecting `{}` (empty coverage) when a seed rule exists will fail, because seed rules like `seed-allow-read-tools` (ToolPattern=`Read|Glob|Grep`) currently pass through the buggy filter and will produce non-empty results. After the fix, they'll be correctly filtered — but the test author must understand why they're filtered (ToolPattern doesn't match "Bash"), not rely on an empty seed list.

**The plan's fallback suggestion is also wrong:** "construct `RuleSpec` entries that would dominate over seed rules by priority" — priority doesn't suppress seed rules, it only determines which rule fires first. The coverage map is cumulative (OR logic), so a high-priority user rule doesn't suppress seed rule contributions.

**Correct approach:** The tests should be written knowing seed rules will be present, and the test assertions must account for seed rule behavior after the fix. This makes the tests more realistic but harder to write. Alternative: add a package-level test helper that constructs a `RulesService` with a modified `allRuleSpecs()` via interface extraction — this requires minor production code refactoring.

The safest approach without production changes: for the "ToolPattern_ReadGlob_skipped" test case, the expected result should be `{}` ONLY IF no seed rule leaks through. After Tasks 1+2+concern-1-fix are applied, seed rules with non-Bash patterns should be filtered. The test must verify this end-to-end (with seed rules present) rather than in isolation.

---

### CONCERN 3 — R4.2 is not addressed by the plan

**Severity: Medium (missing requirement)**

Requirement R4.2 states: "The 'Uncovered Programs' row in `ApprovalAnalyticsPanel` shows the count of STILL-uncovered subcommands (not just total unmatched events)."

The plan has no task addressing `ApprovalAnalyticsPanel`. Looking at the requirements, R4.2 is explicitly in scope. The plan only implements R4.3 (the three-state column in `ProgramDetailPanel`). R4.1 is partially covered (gap subcommands are highlighted differently via the "✗ gap" state), but R4.2 is completely absent.

If R4.2 is intentionally deferred (e.g., needs backend changes to compute a per-subcommand uncovered count), the plan should explicitly call this out as deferred with justification. Currently it's silently omitted.

---

### CONCERN 4 — Task 3's fix interacts incorrectly with criteria-based rules

**Severity: Low (edge case, correct behavior preserved)**

The plan's Task 3 fix adds a loop over `knownSubcmds` inside `if spec.CommandPattern == ""`. However, this block is only reached when `len(spec.CriteriaPrograms) == 0` (line 332: `continue` after the criteria block). So criteria-based rules already mark all known subcommands in the criteria path. The fix only affects pattern-based (CommandPattern="") rules. This is correct behavior, but the plan could be clearer that the two code paths are mutually exclusive.

One edge case: if a rule has BOTH `CommandPattern = ""` AND `CriteriaPrograms` set, the code takes the `CriteriaPrograms` branch and `continue`s — the `CommandPattern == ""` block is never reached. This is existing behavior and unchanged by the fix. No issue, but worth verifying the test matrix covers this.

---

### CONCERN 5 — Task 2's regex compilation in hot path is not caching-safe

**Severity: Low (performance, not correctness)**

The plan notes this in "Adversarial Risks #2". The `regexp.Compile` call inside `coveredSubcommands` runs once per rule per call. `coveredSubcommands` is called from `GetProgramAnalytics` which may be invoked frequently. The typical rule count is small (~20-30), so this is not a critical issue, but a `regexp.MustCompile` cached in a sync.Map or pre-compiled during `allRuleSpecs()` would be cleaner.

This is a WARNING, not a CONCERN — it's acceptable to ship and optimize later.

---

### CONCERN 6 — Task 7 test: `getByText("⚠ partial")` may have encoding issues

**Severity: Low (test fragility)**

The `⚠` character is a Unicode warning sign (U+26A0). The plan calls for asserting `getByText("⚠ partial")`. Depending on the test environment's encoding and whether the span text includes `&nbsp;` or similar, this assertion may be fragile. Using `getByText(/partial/i)` or a `data-testid="coverage-partial"` attribute would be more robust.

---

### CONCERN 7 — R1.4 is not fully satisfied: the plan only lists 3 of 4 required R1 cases in Task 4

**Severity: Low**

R1.4 requires: "Unit tests cover all four cases above" (R1.1 through R1.3 plus the fourth case from R1.4). Looking at R1, the four cases are:
- R1.1: ToolPattern NOT matching Bash → skipped
- R1.2: ToolPattern matching Bash → included  
- R1.3: ToolName="", ToolPattern="" → included
- (implied 4th) ToolName="Bash" (exact) → included

The plan's test table has all four covered under different names (`ToolPattern_ReadGlob_skipped`, `ToolPattern_Bash_included`, `AllToolRule_ToolNameEmpty_ToolPatternEmpty`, `ToolName_Bash_allSubcmds`). This is fine — just confirming coverage is complete.

---

## Summary of Action Items

| # | Issue | Action Required |
|---|---|---|
| 1 | Task 1 fix rationale is wrong; ToolCategory-only seed rules slip through Task 2 | Add ToolCategory guard to `coveredSubcommands` filter; correct plan documentation |
| 2 | Task 4 seed-rule isolation is not achievable via `ReplaceRules(nil)` | Rewrite test approach to account for seed rules being present; or extract `allRuleSpecs` via interface |
| 3 | R4.2 (`ApprovalAnalyticsPanel` subcommand count) not addressed | Either add a Task 8 for `ApprovalAnalyticsPanel` or explicitly defer with justification |
| 4 | Task 3 / criteria interaction is subtly correct but undocumented | Add a comment in plan noting the mutual exclusivity; add a test case for ToolName="" + CriteriaPrograms="" |
| 5 | Regex compile in hot path | Acceptable as-is; optional perf optimization |
| 6 | ⚠ character in test assertions | Use `data-testid` or regex match |
| 7 | R1.4 coverage count | Already satisfied; confirmed |

---

## Final Verdict

**CONCERNS** — The plan is implementable with mitigations. The highest-priority fix needed before coding starts is Concern 1: the ToolCategory-only seed rules (`seed-allow-agent-tools`, `seed-allow-mcp-read`) will produce false-positive coverage after Tasks 1+2 because they have empty `ToolName` and empty `ToolPattern` but are NOT Bash-applicable. The plan's existing "Adversarial Risk #3" touches this but doesn't provide the fix. Add the ToolCategory guard to the filter logic before executing.

Concern 2 (test isolation) is important but not a blocker — the tests will still be valid if written with awareness that seed rules are always present after the fix.

Concern 3 (R4.2 missing) needs a decision: defer explicitly or add a task.
