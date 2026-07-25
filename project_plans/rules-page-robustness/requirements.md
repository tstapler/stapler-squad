# Rules Page Robustness — Requirements

## Problem Statement

The rules page (Approval Rules + Approval Analytics) has several correctness and usability issues:

1. **False-positive coverage display (BUG)**: Items appear in the "Rule Coverage Gaps → Uncovered Programs" section but their drill-down in `ProgramDetailPanel` shows "✓ covered" for subcommands that are NOT actually covered by any Bash rule.

2. **Complex rules not handled**: Rules with `ToolPattern` (e.g., seed rules with `ToolPattern = "Read|Glob|Grep"`) leak into `coveredSubcommands()` because the function only checks `ToolName` and `ToolCategory`, not `ToolPattern`. This causes false-positive coverage assessments for non-Bash rules.

3. **ReclassifyGaps robustness**: `ReclassifyGaps` uses only `CommandPreview` (truncated to 200 chars) and `ToolName`. Long commands may fail to reclassify. Cwd context is also absent, which matters for path-based rules.

4. **UX: unclear why a program is in the uncovered section**: Users see a program in "Uncovered Programs" then drill down and see some subcommands as covered — the disconnect is confusing. There's no clear indication of which specific subcommands are STILL uncovered.

## Root Cause Analysis

### Bug in `coveredSubcommands()` (`server/services/rules_service.go`)

The current filter:
```go
isBashTool := strings.EqualFold(spec.ToolName, "Bash")
isBashCat := strings.EqualFold(spec.ToolCategory, "bash")
if !isBashTool && !isBashCat && spec.ToolName != "" {
    continue
}
```

This only skips rules when `ToolName != ""` AND it's not Bash. Rules with `ToolPattern = "Read|Glob|Grep"` (e.g., seed-allow-read-only-operations) have `ToolName = ""` and `ToolPattern != ""`. They pass the filter and enter the coverage logic. When such a rule has `CommandPattern = ""`, it sets `covered[""] = true` — marking bare program calls (no subcommand) as covered even for Bash programs.

### Additional issue: `CommandPattern == ""` only marks `covered[""]`

When a rule has no `CommandPattern` (covers all Bash commands), the current code:
```go
if spec.CommandPattern == "" {
    covered[""] = true  // ← only empty-subcommand key, NOT specific subcommands
    continue
}
```

A rule covering all Bash should mark ALL known subcommands as covered, not just `""`.

## Requirements

### R1: Fix `coveredSubcommands` ToolPattern filtering (BUG FIX)

**Acceptance criteria:**
- R1.1: A rule with `ToolPattern` that does NOT match "Bash" (e.g., `Read|Glob|Grep`) MUST be skipped in `coveredSubcommands`.
- R1.2: A rule with `ToolPattern` that DOES match "Bash" (e.g., `Bash`, `.*`) MUST be included in coverage analysis.
- R1.3: A rule with `ToolName = ""` AND `ToolPattern = ""` (matches all tools) MUST be included (current behavior preserved).
- R1.4: Unit tests cover all four cases above.

### R2: Fix `CommandPattern == ""` to mark all subcommands covered

**Acceptance criteria:**
- R2.1: When a rule has `CommandPattern == ""` and no `CriteriaPrograms`, ALL known subcommands in `knownSubcmds` are marked covered, not just `""`.
- R2.2: Seed rules with `ToolName = "Bash"` and no `CommandPattern` (e.g., a hypothetical "allow all bash" rule) correctly mark every subcommand as covered.
- R2.3: Unit test verifies this behavior.

### R3: Improve ReclassifyGaps robustness

**Acceptance criteria:**
- R3.1: `ReclassifyGaps` passes `CommandProgram` from stored analytics entries into the classifier payload (via a new `ToolInput["program"]` key or `Cwd` — whichever the classifier uses for criteria matching). Actually, criteria-based matching uses `ExtractAllCommands(cmd)` so the command string itself is key, not a separate field. The issue is truncation.
- R3.2: Analytics entries store the full command or at least enough for criteria matching. The `CommandPreview` truncation limit (200 chars) should be reviewed — for criteria-based matching, only the program name + first subcommand are needed, which typically fits in 200 chars. Verify via test.
- R3.3: A unit test confirms that `ReclassifyGaps` correctly reclassifies entries for criteria-based rules when the command is under 200 chars.

### R4: UX — Show "why uncovered" in drill-down

**Acceptance criteria:**
- R4.1: In `ProgramDetailPanel`, subcommands that are still showing as gaps (not covered, AND had escalated events) are highlighted differently from subcommands that are covered but happened to be in the same program.
- R4.2: The "Uncovered Programs" row in `ApprovalAnalyticsPanel` shows the count of STILL-uncovered subcommands (not just total unmatched events).
- R4.3: The coverage column in `ProgramDetailPanel` distinguishes:
  - "✓ covered" — rule exists AND no recent gaps
  - "⚠ partial" — rule exists but still has some escalated events
  - "✗ gap" — no rule and has escalated events

### R5: Tests

**Acceptance criteria:**
- R5.1: Go unit tests in `server/services/rules_service_test.go` cover:
  - `coveredSubcommands` with `ToolPattern = "Read|Glob"` (should be skipped)
  - `coveredSubcommands` with `ToolPattern = "Bash"` (should be included)
  - `coveredSubcommands` with all-tool rule (`ToolName = ""`, `ToolPattern = ""`) → still included
  - `coveredSubcommands` with `CommandPattern = ""` + known subcommands → all marked covered
- R5.2: Jest unit tests for `ProgramDetailPanel` verify the coverage status display logic (covered/partial/gap).

## Out of Scope

- Changes to the `ApprovalRulesPanel` UI (rule creation form).
- Changes to how analytics data is stored (no schema migration).
- Changes to the classifier itself (only `coveredSubcommands` and `ReclassifyGaps` are in scope).
- AI rule generation changes.
