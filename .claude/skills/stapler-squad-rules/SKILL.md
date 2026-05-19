---
name: stapler-squad-rules
description: Use when working in the stapler-squad repo to manage Claude Code approval rules — reviewing analytics for coverage gaps, adding or retiring rules, debugging why a session generates excessive manual review requests, or editing seed rules in pkg/classifier/classifier.go.
---

# Stapler Squad Rules Engine

Guides you through reading the approval analytics, identifying coverage gaps, and adding or tuning rules in the stapler-squad rules engine.

## When to Use This Skill

- Reviewing analytics to understand what Claude is doing
- Identifying gaps (commands not covered by any rule)
- Deciding whether to add, modify, or retire an auto-approval rule
- Investigating why a session is generating lots of manual review requests

## Accessing the Analytics

### Option A: UI (quick overview)

Navigate to **http://localhost:8543/rules** (run `make restart-web` if the server is not running).

Two panels:
1. **Approval Rules Panel** (top) — active rules with trigger counts
2. **Approval Analytics Panel** (bottom) — time-series data, coverage gaps, program breakdowns

Use the **7 / 14 / 30 / 90 day** window selector. **Note:** if the panel shows "Loading analytics…" with 0 decisions but you know data exists, use Option B instead — the UI reads from the SQLite DB while the JSONL migration may still be in progress for recent data.

### Option B: REST API (authoritative, 90-day window)

```bash
curl -s -X POST "http://localhost:8543/api/session.v1.SessionService/GetApprovalAnalytics" \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -d '{"windowDays": 90}' -o /tmp/approval_analytics_api.json

# Parse key metrics
python3 - <<'EOF'
import json, collections
with open("/tmp/approval_analytics_api.json") as f:
    s = json.load(f)["summary"]
total = s["totalDecisions"]
counts = s.get("decisionCounts", {})
print(f"Total: {total}")
for d, c in sorted(counts.items(), key=lambda x: -x[1]):
    print(f"  {d}: {c} ({100*c/total:.1f}%)")
print(f"\nGap count: {s.get('coverageGapCount')} ({s.get('coverageGapRate',0)*100:.1f}%)")
print("\nTop uncovered programs:")
for p in s.get("topUncoveredPrograms", []):
    print(f"  {p['programName']} [{p['category']}]: {p['count']}")
print("\nTop uncovered tools:")
for t in s.get("topUncoveredTools", []):
    print(f"  {t['toolName']}: {t['count']}")
EOF
```

### Option C: Direct SQLite queries (deepest drill-down)

All classification decisions are written to `~/.stapler-squad/sessions.db`:

```bash
DB=~/.stapler-squad/sessions.db

# Decision breakdown (90 days)
sqlite3 $DB "
SELECT decision, COUNT(*) as c
FROM classification_analytics
WHERE created_at >= datetime('now', '-90 days')
GROUP BY decision ORDER BY c DESC;"

# Top programs with unmatched escalations (the coverage gap list)
sqlite3 $DB "
SELECT command_program, COUNT(*) as c
FROM classification_analytics
WHERE created_at >= datetime('now', '-90 days')
  AND decision = 'escalate'
  AND (rule_id IS NULL OR rule_id = '')
GROUP BY command_program
ORDER BY c DESC LIMIT 20;"

# Sample commands for a specific unmatched program (replace 'strings' with target)
sqlite3 $DB "
SELECT command_subcategory, substr(command_preview,1,200)
FROM classification_analytics
WHERE created_at >= datetime('now', '-90 days')
  AND decision = 'escalate' AND (rule_id IS NULL OR rule_id = '')
  AND command_program = 'strings'
ORDER BY created_at DESC LIMIT 10;"

# Top triggered rules
sqlite3 $DB "
SELECT rule_name, COUNT(*) as c
FROM classification_analytics
WHERE created_at >= datetime('now', '-90 days')
  AND rule_name IS NOT NULL
GROUP BY rule_name ORDER BY c DESC LIMIT 15;"

# Compound command failures: programs that have rules but still escalate in pipelines
sqlite3 $DB "
SELECT command_program, COUNT(*) as c
FROM classification_analytics
WHERE created_at >= datetime('now', '-90 days')
  AND decision = 'escalate' AND (rule_id IS NULL OR rule_id = '')
  AND command_program IN ('cat','ls','echo','cd','ps','grep','find','git','gh')
GROUP BY command_program ORDER BY c DESC;"
```

**Key insight:** When a program has an AutoAllow rule but still shows up in the gap list, it is always the first program in a **compound command** (`cmd1 | cmd2 && cmd3`) where a later sub-command has no matching rule. The compound classifier requires ALL top-level sub-commands to have AutoAllow — if `python3 -c "..."` in a pipeline fails to classify (e.g., due to escaped quotes inside the inline script), the whole compound escalates attributed to the first program.

## Reading the Analytics

### Rule Coverage Gaps (most important)

Appears at the bottom of the analytics panel when decisions went unmatched.

| Metric | Meaning |
|--------|---------|
| **Gap rate** | % of decisions with no matching rule. >30% = high; <10% = good |
| **Uncovered Tools** | Tool types (Bash, Write, Edit) escalating without a rule |
| **Uncovered Bash Programs** | Executables whose commands frequently escape all rules |

For each row → click "Add rule →" to open the rules editor.

### Other Sections

- **Top Triggered Rules** — verify rules are active, find candidates for sub-rules
- **Top Tools** — if Bash is dominant with high gap rate, existing Bash rules are too narrow
- **Top Bash Programs** — appears in both "top" and "uncovered" → needs a rule
- **Top Python Imports** — `requests`/`urllib`/`httpx` = Claude making HTTP calls from Python

## Known Gaps from 90-Day Analysis (as of 2026-05-18)

Current state: **6.8% gap rate** (1,943 / 28,397 decisions), 89.8% auto-allow.

### Tier 1 — Missing rules (program has no rule at all)

| Program | Count | Recommended action |
|---------|-------|--------------------|
| `strings` | 234 | **Auto-allow** — read-only Unix binary inspector (`strings /path/to/bin`) |
| `adb` | 230 | **Allow** read-only subcommands (`devices`, `shell`, `logcat`, `dumpsys`, `pm list`); escalate write ops (`install`, `push`, `shell rm`) |
| `nix` | 139 | **Allow** `--version`, `develop`, `build`, `eval`, `flake`; escalate `install`, `profile add` |
| `r2` / radare2 | 56 | **Auto-allow** — analysis-only invocations (`-A`, `-q`, `-c "?"`); used in RE skill |
| `which` | 44 | **Auto-allow** — read-only shell utility, already conceptually covered by `ls/pwd/echo` |
| `[` | 36 | **Auto-allow** — POSIX `test` builtin, appears in shell scripts, never dangerous alone |
| `sleeper` | 45 | **Auto-allow** — internal test/mock process in this project |
| `java` | 23 | Check against existing `java -jar` / `java -version` rule; add missing subcommands |
| `proextract` | 87 | **Auto-allow** — this is the `proextract` Rust binary from `raystudio-linux`, read-only analysis tool |

### Tier 2 — Compound command failures (program has a rule, but pipeline partner doesn't)

Root cause: `classifyCompound` requires ALL top-level pipeline sub-commands to have AutoAllow. The most common failure is `X | python3 -c "import sys,json; ..."` where the inline python3 script has escaped quotes that confuse the bash tokenizer, preventing the stdlib-import rule from matching.

| First program | Count | What fails |
|--------------|-------|-----------|
| `gh` | 276 | `gh run list ... \| python3 -c "import json,sys; ..."` — complex inline python |
| `cd` | 206 | `cd /path && complex_loop_or_script` — shell loops/heredocs in the chain |
| `cat` | 79 | `cat file \| python3 -c "import sys,json; ..."` — same pattern |
| `ps` | 73 | `ps aux \| awk/python3/grep` — some awk patterns or inline python |
| `ls` | 61 | Compound commands with unrecognized pipeline partners |
| `python3` | 232 | Multi-line `-c` scripts with backslash escapes or heredoc-style quoting |

**Best fix:** investigate `Allow python -c with stdlib-only imports` rule — it should handle `import json,sys` and `import sys, json` but may be failing on `import json,sys` (no space after comma) or on commands where the bash tokenizer sees unbalanced quotes.

### Tier 3 — MCP tools (no rules)

The following MCP tools are unmatched. Add to `Allow read-only MCP tools` or create targeted rules:

- `Monitor` (35) — Claude Code built-in; should already be in "Allow Claude Code agent and planning tools"
- `mcp__stapler-squad__*` (10–7 each) — stapler-squad MCP server tools
- `mcp__claude_ai_Google_Calendar__list_events` (9) — read-only calendar
- `mcp__claude_ai_Gmail__*` (7–4 each) — Gmail read operations

## Deciding What Rules to Add

```
□ What tool is unmatched? (Bash, Write, Edit, Read, etc.)
□ If Bash: what program? (git, npm, curl, docker, …)
□ Is this program safe in this workflow? (vcs=usually safe, network=review)
□ What subcommands are most common? (git commit vs git push differ in risk)
□ Is there a pattern in the command text? (regex on command field)
□ Should this be auto-allow, auto-deny, or explicit escalate?
```

**Safe to auto-allow:** read-only VCS (`git status`, `git log`), package-manager queries (`npm list`), local build/test commands (`go build`, `go test`, `pytest ./...`)

**Auto-deny:** writes to `.env`/credential files, `rm -rf` on non-tmp paths, `curl`/`wget` piped to `sh`

**Escalate:** `git push`, `npm publish`, `kubectl apply`, `terraform apply`

## Rules Engine: Criteria vs. CommandPattern

**Always prefer Criteria** (AST-based). Use `CommandPattern` regex only when Criteria cannot express the match.

### Criteria fields (AND semantics when combined)

| Field | Purpose | Example |
|-------|---------|---------|
| `Programs` | Exact program names | `["git", "jj"]` |
| `Subcommands` | Allowed first positional args | `["status", "log"]` |
| `BlockedSubcommands` | Subcommands that prevent a match | `["push"]` |
| `RequiredFlags` | At least one flag must be present | `["--hard"]` |
| `ForbiddenFlags` | Any of these → rule does not match | `["--force"]` |
| `PythonModes` | Python invocation mode | `["script", "module"]` |

Criteria correctly handles: `git -C /some/path status` → subcommand is `status`; `rtk git push` → unwraps to `git push`; `sudo npm test` → unwraps to `npm test`.

### CommandPattern (last resort)

Use only when matching a flag value, redirection target, or inline argument that Criteria cannot express:

```go
CommandPattern: regexp.MustCompile(`\bcurl\b.*\s(-[a-zA-Z]*[oO]|--(output|remote-name))\b`),
```

## Creating a Rule

### Code change (seed rules — permanent)

1. Open `pkg/classifier/classifier.go` → add to `SeedRules()`:

```go
{
    ID:       "seed-allow-bash-mytool",
    Name:     "Allow mytool read-only subcommands",
    ToolName: "Bash",
    Criteria: &CommandCriteria{
        Programs:    []string{"mytool"},
        Subcommands: []string{"list", "show", "status", "info"},
    },
    Decision:  AutoAllow,
    RiskLevel: RiskLow,
    Reason:    "Read-only mytool operations pose no risk.",
    Priority:  100,
    Enabled:   true,
    Source:    "seed",
},
```

2. Run `go test ./pkg/classifier/...` — all tests must pass.
3. New rule loads on next server restart.

### Runtime addition (no code change)

1. Go to **http://localhost:8543/rules** → **Add Custom Rule** form
2. Fill in Name, Decision, Tool Name, Command Pattern, Reason, Priority
3. Click **Save Rule** — takes effect immediately without restart.

## Keeping Rules Evergreen

Review weekly or after major workflow changes:

1. Check top uncovered programs — new tools needing rules
2. Check stale rules — haven't triggered recently, may be too specific
3. Check manual review rate trend — rising = new patterns to cover
4. After adding a new Claude skill — Claude may start using new programs; check analytics a day later

## Backend Files

| File | Purpose |
|------|---------|
| `pkg/classifier/classifier.go` | `SeedRules()`, `CommandCriteria`, `AuditCommand` |
| `pkg/classifier/command_parser.go` | Bash AST parser, `ExtractAllCommands`, Python import extractor |
| `server/services/rules_service.go` | RPC handlers + proto mapping |
| `server/services/analytics_store.go` | JSONL analytics storage + aggregation |
| `server/services/approval_handler.go` | HTTP hook handler + secret scanner + domain checker |
| `server/services/secret_scanner.go` | Regex patterns for plaintext secret detection |
| `server/services/domain_checker.go` | RDAP-based new-domain escalation |
| `proto/session/v1/types.proto` | Proto definitions (run `make proto-gen` after changes) |
| `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | Analytics UI |
| `web-app/src/components/sessions/ApprovalRulesPanel.tsx` | Rules management UI |
