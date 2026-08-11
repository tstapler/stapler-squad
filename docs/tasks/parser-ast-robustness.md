# Parser AST Robustness

**Date**: 2026-04-13 (planned)
**Status**: Planned
**Scope**: Unify the command parser infrastructure so both the classifier and analytics paths use the `mvdan.cc/sh` AST, add missing seed rules for observed programs, harden `splitCommandParts` as a fallback, and teach the AST walker to unwrap transparent proxy commands (`rtk`, `sudo`, `env`, etc.) so `rtk git clone` is classified as `git clone`.

---

## Table of Contents

- [Overview](#overview)
- [Problem Statement](#problem-statement)
- [Success Metrics](#success-metrics)
- [Architecture Context](#architecture-context)
- [Story Breakdown](#story-breakdown)
  - [Story 1: Rewrite ParseBashCommand to Use AST](#story-1-rewrite-parsebashcommand-to-use-ast)
  - [Story 2: Add Missing Seed Rules](#story-2-add-missing-seed-rules)
  - [Story 3: Harden splitCommandParts Fallback](#story-3-harden-splitcommandparts-fallback)
- [Dependency Visualization](#dependency-visualization)
- [Integration Checkpoints](#integration-checkpoints)
- [Known Issues and Potential Bugs](#known-issues-and-potential-bugs)
- [Backward Compatibility](#backward-compatibility)
- [Files Affected](#files-affected)

---

## Overview

The classifier (`ExtractAllCommands`) and the analytics layer (`ParseBashCommand`) use fundamentally different parsing strategies for the same input. `ExtractAllCommands` walks the `mvdan.cc/sh/v3/syntax` AST and correctly handles subshells, pipelines, compound commands, and env-var prefixes. `ParseBashCommand` delegates to `splitCommandParts`, a naive regex splitter that fails on background operators (`&`), line continuations (`\`), shell flow-control keywords (`for`, `while`, `if`), and function definitions. This divergence causes analytics to record phantom programs (`#`, `for`, `\`) and miss the actual primary program in multi-line or compound commands.

Additionally, three frequently-observed programs lack seed rules: `gofmt` (77 occurrences), `source` (9), and `asdf` (10). Adding rules for these addresses a measurable portion of the 10.9% coverage gap rate.

`rtk` (23 occurrences) is handled differently: it is a **transparent proxy wrapper** that rewrites commands before passing them to the shell (e.g., `rtk git status` → executes `git status` with token-saving wrappers). Adding a blanket `AutoAllow` for `rtk` would be unsafe — `rtk rm -rf /` would bypass the deny rule. Instead, `rtk` is added to `wrapperCommands` and the `ExtractAllCommands` AST walker is updated to unwrap it, so `rtk git clone` is classified identically to `git clone`, inheriting all existing safety rules.

---

## Problem Statement

**Root cause**: Two parsing paths for the same input produce inconsistent results.

| Aspect | ExtractAllCommands (classifier) | ParseBashCommand (analytics) |
|---|---|---|
| Parser | `mvdan.cc/sh` AST | `splitCommandParts` (regex) |
| Handles `$()` / backticks | Yes (recursive AST walk) | No |
| Handles `&` (background) | Yes (BinaryCmd node) | No (not a split point) |
| Handles `\` continuation | Yes (parser joins lines) | No (produces `\` token) |
| Filters shell keywords | Yes (only CallExpr nodes) | No (`for`, `while` become "programs") |
| Filters `#` comments | Implicit (not in AST) | Partial (line-start only) |
| Env-var prefix handling | Yes (Assign nodes separated from CallExpr) | Partial (regex-based) |

**Observed impact** from production analytics (top phantom programs):

| "Program" | Count | Root Cause |
|---|---|---|
| `cd` | 77 | First program in compound command; later sub-commands uncovered |
| `#` | 38 | Shell comments extracted as programs via naive split |
| `for` | 29 | For-loop keyword extracted as program |
| `\` | 9 | Line-continuation character parsed as command |

---

## Success Metrics

1. **ParseBashCommand parity**: For any input where the AST parser succeeds, `ParseBashCommand` returns the same `Program` and `Subcommand` as the first `ParsedCommand` from `ExtractAllCommands`. Verified by a cross-check test.
2. **Phantom program elimination**: After Story 1, the programs `#`, `for`, `while`, `if`, `then`, `do`, `done`, `fi`, `elif`, `else`, `function`, and `\` no longer appear in `AnalyticsEntry.CommandProgram`.
3. **Coverage gap reduction**: After Story 2, commands using `gofmt`, `source`, and `asdf` produce a matching `RuleID` instead of falling through to the no-rule escalation path. Estimated reduction: ~45 historical entries move from gap to covered.
4. **`rtk` wrapper transparency**: After Story 3, `rtk git status` → `{Program: "git"}`, `rtk rm -rf /` → `AutoDeny` (deny rule fires on unwrapped command). The 23 `rtk` gap entries are resolved by the correct underlying classification rather than a new rule.
4. **Fallback resilience**: After Story 3, `splitCommandParts` correctly handles `&`, `\`-continuations, and shell keywords. Verified by targeted unit tests.
5. **Zero regressions**: All existing tests pass: `go test -run "TestClassify|TestExtract|TestDetect|TestMatches|TestCategorize|TestSeed|TestParse" ./server/services/`

---

## Architecture Context

```
BEFORE (current state -- two divergent paths):

+-----------------------------------------------------------+
|              HTTP Hook: /api/hooks/permission-request      |
|                                                           |
|  +------------------+       +---------------------------+ |
|  |   Classify()     |       |  RecordFromResult()       | |
|  |                  |       |                           | |
|  |  ExtractAll      |       |  ParseBashCommand()       | |
|  |  Commands()      |       |  +---------------------+  | |
|  |  +------------+  |       |  | splitCommand        |  | |
|  |  | mvdan.cc/  |  |       |  | Parts()             |  | |
|  |  | sh AST     |<-+- OK --+--| ^ PROBLEM:          |  | |
|  |  +------------+  |       |  | naive regex          |  | |
|  |  +------------+  |       |  +---------------------+  | |
|  |  | splitCmd   |  |       |  +---------------------+  | |
|  |  | Parts()    |<-+- ERR -+--| extractProgram      |  | |
|  |  | (fallback) |  |       |  | AndSubcommand()     |  | |
|  |  +------------+  |       |  +---------------------+  | |
|  +------------------+       +---------------------------+ |
|         ^ GOOD                      ^ DRIFT               |
+-----------------------------------------------------------+


AFTER (unified paths):

+-----------------------------------------------------------+
|              HTTP Hook: /api/hooks/permission-request      |
|                                                           |
|  +------------------+       +---------------------------+ |
|  |   Classify()     |       |  RecordFromResult()       | |
|  |                  |       |                           | |
|  |  ExtractAll      |       |  ParseBashCommand()       | |
|  |  Commands()      |       |  +---------------------+  | |
|  |  +------------+  |       |  | mvdan.cc/sh AST     |  | |
|  |  | mvdan.cc/  |  |       |  | (same as left)      |  | |
|  |  | sh AST     |<-+- OK --+--+                     |  | |
|  |  +------------+  |       |  +---------------------+  | |
|  |  +------------+  |       |  +---------------------+  | |
|  |  | splitCmd   |  |       |  | splitCmd (hardened   |  | |
|  |  | Parts()    |<-+- ERR -+--| fallback)            |  | |
|  |  | (fallback) |  |       |  +---------------------+  | |
|  |  +------------+  |       +---------------------------+ |
|         UNIFIED PATHS                                     |
+-----------------------------------------------------------+
```

---

## Story Breakdown

### Story 1: Rewrite ParseBashCommand to Use AST

**Goal**: `ParseBashCommand` produces the same program/subcommand extraction as `ExtractAllCommands` by using the `mvdan.cc/sh` parser, eliminating all phantom program entries in analytics.

**Acceptance Criteria**:
- `ParseBashCommand` calls `syntax.NewParser().Parse()` as its primary path.
- On parse success, extracts the first `CallExpr` program (path-stripped) as `Program`, uses `extractSubcommand()` for `Subcommand`, and collects all distinct programs across the full AST for `AllPrograms`.
- On parse error, falls back to `splitCommandParts` + `extractProgramAndSubcommand` (existing behavior preserved).
- `categorizeProgram()` still maps `Program` to `Category`.
- Shell keywords (`for`, `while`, `if`, `function`, etc.) never appear as `Program`.
- Comments (`#`-prefixed tokens) never appear as `Program`.
- Line continuations (`\`) never appear as `Program`.
- A cross-check test iterates a table of 20+ commands and asserts `ParseBashCommand(...).Program == ExtractAllCommands(...)[0].Program`.

#### Task 1.1: Implement AST-based ParseBashCommand

**File**: `server/services/command_parser.go`

Rewrite `ParseBashCommand` to:
1. Parse input with `syntax.NewParser().Parse(r, "")`.
2. On success, walk the AST collecting all `CallExpr` nodes (same logic as `ExtractAllCommands`).
3. Use the first `CallExpr` for `Program` and `Subcommand` (via `extractSubcommand`).
4. Collect all distinct programs from all `CallExpr` nodes for `AllPrograms`.
5. On parse error, fall back to existing `splitCommandParts` + `extractProgramAndSubcommand`.
6. Return `CommandInfo{Program, Subcommand, Category, AllPrograms}`.

**Key constraint**: The `extractProgramAndSubcommand` function and `splitCommandParts` must not be removed -- they remain the fallback path and are used by `ExtractAllCommands` fallback too.

**Approach**: The simplest correct implementation is to have `ParseBashCommand` call `ExtractAllCommands` internally, then derive `CommandInfo` from the result:
- `Program` from `cmds[0].Program`
- `Subcommand` from `extractSubcommand(cmds[0].Program, cmds[0].Args)`
- `Category` from `categorizeProgram(cmds[0].Program)`
- `AllPrograms` by deduplicating all `cmds[i].Program` values

This avoids duplicating any AST logic and guarantees the two paths produce identical results by construction.

#### Task 1.2: Add cross-check and regression tests

**File**: `server/services/classifier_test.go` (add new test functions)

Tests to add:

1. **`TestParseBashCommand_ASTConsistency`**: Table-driven test with 20+ commands. For each, assert `ParseBashCommand(cmd).Program == ExtractAllCommands(cmd)[0].Program` when ExtractAllCommands returns at least one result.

   Command table:
   - Simple: `git status`, `ls -la`, `make build`
   - Compound: `cd /tmp && git status`, `go build && go test`
   - Pipeline: `cat file.txt | grep pattern | wc -l`
   - Background: `make restart-web 2>&1 &`
   - Env-var prefix: `CONFLUENCE_BASE_URL="https://example.com" actual-command arg1`
   - Subshell: `echo $(git rev-parse HEAD)`
   - For loop: `for f in *.go; do gofmt "$f"; done`
   - Function def: `reply_to_thread() { local thread_id="$1"; }`
   - Path-qualified: `node_modules/.bin/stylelint 'src/**/*.css'`
   - Redirections: `gofmt -e file.go > /dev/null`
   - Heredoc: ``cat <<'EOF'\nline1\nEOF``

2. **`TestParseBashCommand_NoPhantomPrograms`**: Assert that `ParseBashCommand` never returns `#`, `for`, `while`, `if`, `then`, `do`, `done`, `fi`, `elif`, `else`, `function`, or `\` as `Program`.

   Input table:
   - `# this is a comment`
   - `for f in *.go; do echo "$f"; done`
   - `while true; do sleep 1; done`
   - `if [ -f file ]; then echo yes; fi`
   - `function foo() { echo bar; }`

3. **`TestParseBashCommand_AllPrograms`**: Assert that `AllPrograms` contains all distinct programs from compound commands. Input: `git add . && go test ./... | tee output.log` should produce `AllPrograms` containing `git`, `go`, `tee`.

4. **`TestParseBashCommand_FallbackPath`**: Feed intentionally unparseable input (e.g., bare `}}}`) and verify it returns a result from the fallback splitter without panicking.

#### Task 1.3: Verify analytics integration unchanged

**Scope**: No code changes. Verification only.

Steps:
1. `go test -run "TestClassify|TestExtract|TestDetect|TestMatches|TestCategorize|TestSeed|TestParse" ./server/services/` -- all pass.
2. `go vet ./server/services/` -- no new warnings.
3. Manual smoke test: `make restart-web`, issue commands, check `~/.stapler-squad/approval_analytics.jsonl` for correct `command_program` values.

---

### Story 2: Add Missing Seed Rules

**Goal**: Add seed rules for `gofmt`, `source`/`.`, and `asdf` to cover the top observed programs that currently fall through to the no-rule escalation path.

**Acceptance Criteria**:
- `gofmt` is AutoAllow at Priority 100 (safe code formatter, equivalent to `go fmt`).
- `rtk` is **NOT** a seed rule — it is handled as a wrapper command in Story 3 (Task 3.4) so that `rtk git push` escalates and `rtk rm -rf /` denies.
- `source` and `.` (dot-source builtin) are Escalate at Priority 50, with an Alternative message suggesting `python -m venv` or explicit environment activation.
- `asdf` is Escalate at Priority 50 (installs/activates language runtimes, modifies system state).
- All three new rules have test coverage.
- No existing seed rules are modified or removed.
- `categorizeProgram` is updated to include `gofmt` in the `"go"` category.

#### Task 2.1: Add gofmt AutoAllow rule

**File**: `server/services/classifier.go`

Add to the AutoAllow (Priority 100) section of `SeedRules()`:

```go
{
    ID:       "seed-allow-bash-gofmt",
    Name:     "Allow gofmt (Go code formatter)",
    ToolName: "Bash",
    Criteria: &CommandCriteria{
        Programs: []string{"gofmt"},
    },
    Decision:  AutoAllow,
    RiskLevel: RiskLow,
    Reason:    "gofmt is a read-only code formatter equivalent to go fmt.",
    Priority:  100,
    Enabled:   true,
    Source:    "seed",
},
```

**File**: `server/services/command_parser.go`

Update `categorizeProgram()`:
- Add `"gofmt"` to the `"go"` case.

**Note**: `rtk` is handled as a transparent proxy wrapper in Story 3 (Task 3.4), not as a seed rule. This ensures `rtk rm -rf /` is still AutoDeny and `rtk git push` still Escalates.

#### Task 2.2: Add source/asdf Escalate rules

**File**: `server/services/classifier.go`

Add to the Escalate catch-all (Priority 50) section:

```go
{
    ID:       "seed-escalate-source",
    Name:     "Escalate source/dot-source (shell script sourcing)",
    ToolName: "Bash",
    Criteria: &CommandCriteria{
        Programs: []string{"source", "."},
    },
    Decision:    Escalate,
    RiskLevel:   RiskMedium,
    Reason:      "Sourcing shell scripts executes arbitrary code in the current shell and modifies the environment.",
    Alternative: "For Python virtualenvs, use 'python -m venv .venv && .venv/bin/python' directly instead of sourcing activate scripts.",
    Priority:    50,
    Enabled:     true,
    Source:      "seed",
},
{
    ID:       "seed-escalate-asdf",
    Name:     "Escalate asdf (runtime version manager)",
    ToolName: "Bash",
    Criteria: &CommandCriteria{
        Programs: []string{"asdf"},
    },
    Decision:    Escalate,
    RiskLevel:   RiskMedium,
    Reason:      "asdf installs and activates language runtimes, modifying system-level tool state.",
    Alternative: "Review the asdf command and plugin before proceeding. Consider using project-local .tool-versions.",
    Priority:    50,
    Enabled:     true,
    Source:      "seed",
},
```

**Note on `.` (dot-source)**: The AST parser treats `.` as a program name in `CallExpr` nodes when it appears as a command (e.g., `. ~/.bashrc`). The `Programs: []string{"source", "."}` in Criteria uses exact matching. The `isSubcommandLike(".")` returns `false` (dot is not in the allowed character set), so `Subcommand` will be empty, which is correct.

#### Task 2.3: Add tests for new seed rules

**File**: `server/services/classifier_test.go`

Add test functions:

1. **`TestClassify_Gofmt_AutoAllow`**: Tests `gofmt file.go`, `gofmt -e -l server/`, `gofmt -w file.go`.

2. **`TestClassify_Source_Escalate`**: Tests `source ~/.bashrc`, `source .venv/bin/activate`, `. ~/.profile`. Verify Decision is Escalate and Alternative mentions virtualenv.

3. **`TestClassify_Asdf_Escalate`**: Tests `asdf install python 3.11.0`, `asdf global python 3.11.0`, `asdf list`. Verify Decision is Escalate.

**Note**: `rtk` wrapper transparency tests are in Task 3.4 (`TestClassify_Rtk_WrapperTransparency`), not here, since rtk is handled via wrapperCommands rather than a seed rule.

---

### Story 3: Harden splitCommandParts Fallback

**Goal**: Improve the naive `splitCommandParts` function to handle three edge cases that produce phantom tokens: standalone `&` (background operator), `\`-newline continuations, and shell flow-control keywords.

**Acceptance Criteria**:
- `splitCommandParts` splits on standalone `&` (but not `&&`, not `2>&1`).
- `splitCommandParts` normalizes `\` + newline sequences (joining continuation lines) before splitting.
- `extractProgramAndSubcommand` skips shell flow-control keywords (`for`, `while`, `if`, `then`, `do`, `done`, `fi`, `elif`, `else`, `case`, `esac`, `function`, `select`, `until`, `in`) when looking for the primary program.
- Existing `splitCommandParts` behavior for `|`, `;`, `&&`, `||`, `\n` is unchanged.
- Existing comment filtering (`#` at line start) is unchanged.

#### Task 3.1: Add `&` splitting and `\`-continuation handling

**File**: `server/services/command_parser.go`

Modify `splitCommandParts`:

1. **Continuation normalization**: Before any splitting, replace `\` followed by `\n` (backslash-newline) with a single space. This joins continuation lines before the splitter sees them.

   ```go
   // Normalize line continuations before splitting.
   cmd = strings.ReplaceAll(cmd, "\\\n", " ")
   ```

2. **Background operator splitting**: After replacing `&&` and `||` with sentinels, also replace standalone `&` with the sentinel. Must avoid splitting `2>&1` or similar redirect patterns.

   Approach: after `&&`/`||` sentinel replacement, use a regex to match `&` that is preceded by whitespace (or start-of-string) and not preceded by `>`. Compile once at package level.

   ```go
   // Package-level: matches standalone & (background operator),
   // excluding redirect patterns like 2>&1, >&2, &>file.
   var bgOperatorPattern = regexp.MustCompile(`([^>])\s*&\s*$|([^>])\s*&\s+`)
   ```

   A simpler safe approach: after `&&`/`||` replacement, iterate characters and only replace `&` when the preceding non-whitespace character is not `>`.

#### Task 3.2: Filter shell keywords in extractProgramAndSubcommand

**File**: `server/services/command_parser.go`

Add a package-level set:

```go
var shellKeywords = map[string]bool{
    "for": true, "while": true, "until": true, "if": true,
    "then": true, "else": true, "elif": true, "fi": true,
    "do": true, "done": true, "case": true, "esac": true,
    "in": true, "select": true, "function": true,
}
```

In `extractProgramAndSubcommand`, after stripping env vars and path prefixes, before accepting a token as `prog`:
- If `shellKeywords[bare]`, skip the token and continue scanning.

This ensures the fallback path skips keywords, matching the behavior of the AST path which only yields `CallExpr` nodes.

#### Task 3.3: Add tests for hardened splitCommandParts

**File**: `server/services/classifier_test.go`

Tests to add:

1. **`TestSplitCommandParts_BackgroundOperator`**:
   - `make build &` produces `["make build"]` (the `&` splits, trailing empty part filtered).
   - `make build 2>&1` produces `["make build 2>&1"]` (redirect `&` not split).
   - `make build 2>&1 &` produces `["make build 2>&1"]`.
   - `cmd1 & cmd2` produces `["cmd1", "cmd2"]`.

2. **`TestSplitCommandParts_LineContinuation`**:
   - Input with backslash-newline: the continuations are joined before splitting, producing a single merged part.

3. **`TestExtractProgramAndSubcommand_SkipsKeywords`**:
   - `extractProgramAndSubcommand("for f in *.go")` returns `prog=""`.
   - `extractProgramAndSubcommand("if [ -f x ]")` returns `prog="["` (bracket is a legitimate program, not a keyword).
   - `extractProgramAndSubcommand("while true")` returns `prog="true"` (`while` skipped, `true` is a program).

4. **`TestParseBashCommand_ForLoop`**: `ParseBashCommand("for f in *.go; do gofmt $f; done")` returns `Program: "gofmt"` (from AST path).

#### Task 3.4: Add rtk to wrapperCommands and update AST walker

**File**: `server/services/command_parser.go`

**Goal**: Make `rtk` a transparent proxy wrapper so `rtk git clone` is classified identically to `git clone`, inheriting all existing safety rules. `rtk rm -rf /` must still trigger AutoDeny.

**Step 1 — Add rtk to wrapperCommands**:

```go
var wrapperCommands = map[string]bool{
    "sudo":  true,
    "exec":  true,
    "time":  true,
    "nice":  true,
    "nohup": true,
    "env":   true,
    "watch": true,
    "rtk":   true, // transparent CLI proxy; args are the actual command
}
```

**Step 2 — Update ExtractAllCommands AST walker to unwrap wrappers**:

In the section of `ExtractAllCommands` where it processes a `CallExpr` node and extracts the program from the first token, add a wrapper-unwrapping loop after the initial `prog` is extracted. The loop advances past any wrapper tokens (skipping over env-var prefixes) to find the true underlying program:

```go
// After extracting prog from tokens[0]:
startIdx := 1
for wrapperCommands[strings.ToLower(prog)] && startIdx < len(tokens) {
    // Skip env-var assignments (KEY=VALUE patterns) before the real program.
    for startIdx < len(tokens) && envVarPattern.MatchString(tokens[startIdx]) {
        startIdx++
    }
    if startIdx >= len(tokens) {
        break // wrapper with no following command (e.g., bare "rtk")
    }
    // The next non-env-var token is the wrapped program.
    prog = tokens[startIdx]
    if idx := strings.LastIndex(prog, "/"); idx >= 0 {
        prog = prog[idx+1:] // strip path prefix
    }
    startIdx++
}
args := tokens[startIdx:]
```

This handles chained wrappers (`sudo rtk git status` → `git`), env-var prefixes (`sudo KEY=VAL git status` → `git`), and bare wrappers with no following command (`rtk` alone → returns the wrapper name itself, producing an empty program in classification, which falls through to escalate).

**Key constraint**: The existing `wrapperCommands` usage in `extractProgramAndSubcommand` (naive path) already skips wrappers. This task adds equivalent logic to the AST walker so both paths are consistent.

**File**: `server/services/classifier_test.go`

Add test function **`TestClassify_Rtk_WrapperTransparency`**:

1. `rtk git status` → Decision: AutoAllow, RuleID: `seed-allow-git-read` (inherits git read rule)
2. `rtk rm -rf /` → Decision: AutoDeny, RuleID: `seed-deny-rm-rf-root` (deny rule fires on unwrapped command)
3. `rtk git push origin main` → Decision: Escalate, RuleID: `seed-escalate-git-push` (push still escalates)
4. `sudo rtk git status` → Decision: AutoAllow (chained wrappers both unwrapped)
5. `rtk` (bare, no subcommand) → Decision: Escalate (no matching rule for bare rtk; falls through)
6. `rtk gain` → Decision: Escalate (no rule for `gain` program; rtk wrapper stripped, `gain` has no seed rule)

Also update **`TestClassify_Rtk_AutoAllow`** in Task 2.3 to be removed and replaced by `TestClassify_Rtk_WrapperTransparency` here. The test in Task 2.3 was predicated on a blanket AutoAllow rule that was removed from Story 2.

---

## Dependency Visualization

```
Story 3 (Harden splitCommandParts)
  |
  |  No dependency on Stories 1 or 2.
  |  Can be implemented in parallel.
  |
  v
  splitCommandParts improvements benefit
  the fallback path in both ExtractAllCommands
  and the new ParseBashCommand.

Story 1 (Rewrite ParseBashCommand)
  |
  |  Depends on: nothing (uses existing ExtractAllCommands)
  |  Benefits from: Story 3 (better fallback)
  |  but does not require it.
  |
  v
  ParseBashCommand now uses AST.
  Analytics entries become consistent.

Story 2 (Add Seed Rules)
  |
  |  Fully independent of Stories 1 and 3.
  |  Can be implemented in any order.
  |
  v
  Three new programs covered by rules (gofmt, source, asdf).
  Coverage gap rate reduced.
```

**Recommended implementation order**:

1. **Story 2** first -- smallest scope, independent, immediately reduces coverage gap rate. Quick win.
2. **Story 3** second -- hardens the fallback that both parser paths rely on. Low risk, isolated to `splitCommandParts` and `extractProgramAndSubcommand`.
3. **Story 1** last -- the most impactful change. After Story 3, the fallback path is robust, so the AST rewrite of `ParseBashCommand` has a solid safety net.

Stories 2 and 3 can be done in parallel since they modify different sections of the codebase.

---

## Integration Checkpoints

### After Story 1

- [ ] `go test -run "TestClassify|TestExtract|TestDetect|TestMatches|TestCategorize|TestSeed|TestParse" ./server/services/` -- all pass
- [ ] `go vet ./server/services/` -- clean
- [ ] `go build .` -- compiles
- [ ] New `TestParseBashCommand_ASTConsistency` passes with 20+ command table
- [ ] New `TestParseBashCommand_NoPhantomPrograms` passes
- [ ] Spot-check: `make restart-web`, issue a `for f in *.go; do gofmt "$f"; done` command, verify analytics JSONL shows `command_program: "gofmt"` (not `"for"`)

### After Story 2

- [ ] All existing classifier tests still pass (no regressions from new rules)
- [ ] New `TestClassify_Gofmt_AutoAllow` passes
- [ ] New `TestClassify_Source_Escalate` passes
- [ ] New `TestClassify_Asdf_Escalate` passes
- [ ] `SeedRules()` count increases by exactly 3 (gofmt, source, asdf)

### After Story 3

- [ ] All existing tests pass
- [ ] New `TestSplitCommandParts_BackgroundOperator` passes
- [ ] New `TestSplitCommandParts_LineContinuation` passes
- [ ] New `TestExtractProgramAndSubcommand_SkipsKeywords` passes
- [ ] New `TestClassify_Rtk_WrapperTransparency` passes (all 6 cases)
- [ ] `splitCommandParts("make restart-web 2>&1 &")` returns `["make restart-web 2>&1"]`
- [ ] `splitCommandParts("# this is a comment")` returns `[]` (existing behavior preserved)
- [ ] `rtk git status` → AutoAllow (wrapperCommands unwrapping works end-to-end)
- [ ] `rtk rm -rf /` → AutoDeny (safety rules still apply after unwrapping)

### Final Integration

- [ ] `make quick-check` passes (build + test + lint)
- [ ] `make pre-commit` passes
- [ ] No new `go vet` or `staticcheck` warnings
- [ ] Coverage gap rate measurably lower on historical analytics data (run `ReclassifyGaps` with new classifier)

---

## Known Issues and Potential Bugs

### Bug 1: AST parser may reject valid shell constructs [SEVERITY: Medium]

**Description**: `mvdan.cc/sh/v3` is a POSIX/Bash parser but does not support every shell dialect. Zsh-specific syntax, fish syntax, or highly unusual Bash extensions may fail to parse. When this happens, `ParseBashCommand` falls back to `splitCommandParts` -- but the fallback may produce different results than the AST path would have for a parseable variant.

**Mitigation**:
- The fallback path is hardened in Story 3 to handle the most common failure modes.
- Log a structured warning (not just silent fallback) when the AST parse fails, so patterns can be identified from production logs.
- The cross-check test in Task 1.2 covers the most common real-world command shapes.

**Files Affected**: `server/services/command_parser.go` (ParseBashCommand)

**Prevention Strategy**: Maintain the fallback path. Never remove `splitCommandParts`. Monitor parse-error frequency in production logs.

### Bug 2: extractSubcommand behavior difference between classifier and analytics [SEVERITY: Low]

**Description**: In the classifier path, `ExtractAllCommands` returns `ParsedCommand.Args` which are the tokens after the program as printed by `syntax.Printer`. In the analytics path, if `ParseBashCommand` reuses `ExtractAllCommands` internally, the `Args` will be printer-formatted (e.g., quotes may be stripped). This matches the classifier exactly, which is the desired outcome. However, if someone later adds Args-dependent logic to the analytics layer, the quote-stripping behavior could be surprising.

**Mitigation**: Document that `ParsedCommand.Args` are printer-formatted with outer quotes stripped (via `stripOuterQuotes`). This is established behavior, not new.

**Files Affected**: `server/services/command_parser.go` (extractSubcommand, ParseBashCommand)

### Bug 3: Standalone `&` regex may false-positive on `>&` in obscure redirect forms [SEVERITY: Low]

**Description**: The `splitCommandParts` improvement for `&` needs to avoid splitting on redirect patterns like `>&2`, `2>&1`, `&>file` (bash-specific redirect-all). The regex approach must be tested against these patterns.

**Mitigation**:
- Test table includes: `cmd 2>&1`, `cmd >&2`, `cmd &>file`, `cmd &>/dev/null`.
- Use the pattern: after removing `&&` and `||`, replace `&` with sentinel only when the character before `&` is whitespace or start-of-string (not `>`).
- Compile regex at package level to avoid per-call overhead.

**Files Affected**: `server/services/command_parser.go` (splitCommandParts)

**Prevention Strategy**: Exhaustive test table for redirect patterns. Run `splitCommandParts` against the top 100 real commands from analytics to verify no regressions.

### Bug 4: `.` (dot) as source builtin vs `.` as current directory [SEVERITY: Medium]

**Description**: The seed rule `Programs: []string{"source", "."}` matches the `.` program. However, `.` can also appear in other contexts (e.g., `find .` where `.` is an argument to `find`, not a program). The Criteria matching only checks the `Program` field of the first `ParsedCommand`, so `find .` would have `Program: "find"` and the `.` rule would NOT match. This is correct behavior. But if someone writes a bare `.` command like `. script.sh`, the AST parser extracts `Program: "."` which correctly matches the rule.

**Mitigation**: No code mitigation needed -- the Criteria matching is already correct. Add a comment in the rule explaining the distinction.

**Files Affected**: `server/services/classifier.go` (seed rule for source)

### Bug 5: categorizeProgram may need updates for new programs [SEVERITY: Low]

**Description**: Adding `gofmt` to the `"go"` case in `categorizeProgram` changes the category for commands that were previously `"other"`. Analytics entries recorded before the change will have `category: "other"` while new entries will have `category: "go"`. This is a benign inconsistency in historical data.

**Mitigation**: This is expected behavior. The `ReclassifyGaps` function re-runs the classifier but does not re-derive categories. If consistent historical categories are needed, a backfill script would be required -- but this is not worth the complexity.

**Files Affected**: `server/services/command_parser.go` (categorizeProgram), `server/services/analytics_store.go` (historical data)

### Bug 6: Race condition if SeedRules() is called concurrently with Classify() [SEVERITY: None]

**Description**: `SeedRules()` returns a new slice on each call. The `RuleBasedClassifier` holds its own sorted copy behind a `sync.RWMutex`. There is no shared mutable state between the two paths. No race condition exists.

**Prevention Strategy**: The existing concurrency design is correct. No changes needed.

---

## Backward Compatibility

**Invariants maintained**:
1. No existing seed rules are removed or have their Priority/Decision changed.
2. The `ClassificationResult` struct is unchanged.
3. The `CommandInfo` struct is unchanged (same fields: Program, Subcommand, Category, AllPrograms).
4. The `ParsedCommand` struct is unchanged.
5. `ExtractAllCommands` behavior is unchanged (no modifications to this function).
6. The `splitCommandParts` fallback is still used when AST parsing fails.
7. All test names in the existing test suite are preserved.
8. The `AnalyticsEntry` JSON schema is unchanged (same field names and types).

**Behavioral changes** (intentional improvements, not regressions):
- `ParseBashCommand("for f in *.go; do gofmt $f; done")` previously returned `Program: "for"`, will now return `Program: "gofmt"`.
- `ParseBashCommand("# comment")` previously returned `Program: "#"`, will now return `Program: ""`.
- `ParseBashCommand("cmd1 &")` via `splitCommandParts` fallback previously kept `&` attached, will now split it off.
- `gofmt` commands previously escalated (no rule), will now AutoAllow.
- `rtk git status` previously escalated (no rule for `rtk`), will now AutoAllow (wrapper unwrapped → `git` → git read rule).
- `rtk rm -rf /` previously escalated (no rule), will now AutoDeny (wrapper unwrapped → `rm -rf /` → deny rule).
- `source`/`.` commands previously escalated with generic "no matching rule" reason, will now escalate with a specific reason and alternative suggestion.
- `asdf` commands previously escalated with generic reason, will now escalate with a specific reason.

---

## Files Affected

| File | Stories | Change Type |
|---|---|---|
| `server/services/command_parser.go` | 1, 3 | Modified: `ParseBashCommand` rewrite, `splitCommandParts` hardening, `extractProgramAndSubcommand` keyword filter, `categorizeProgram` additions, `wrapperCommands` rtk entry, `ExtractAllCommands` wrapper-unwrap loop |
| `server/services/classifier.go` | 2 | Modified: 3 new rules in `SeedRules()` (gofmt, source, asdf) |
| `server/services/classifier_test.go` | 1, 2, 3 | Modified: new test functions for ParseBashCommand, seed rules, splitCommandParts, rtk wrapper |
