# Skill: stapler-squad-rules

Use this skill when working in the claude-squad/stapler-squad repo and you need to:
- Review the approval analytics to understand what Claude is doing
- Identify gaps in the rules engine (commands not covered by any rule)
- Decide whether to add, modify, or retire an auto-approval rule
- Investigate why a session is generating lots of manual review requests

## How to access the analytics

Navigate to **http://localhost:8543/rules** in your browser (the server must be running — `make restart-web` if not).

The page has two panels:
1. **Approval Rules Panel** (top) — shows all active rules with their trigger counts
2. **Approval Analytics Panel** (bottom) — time-series data, coverage gaps, program breakdowns

Use the **7 / 14 / 30 / 90 day** window selector to change the time range.

---

## Reading the analytics to find new rules

### 1. Rule Coverage Gaps section

This is the most important section for finding missing rules. It appears at the bottom of the analytics panel when any decisions went unmatched.

- **Gap rate** — what percentage of decisions had no matching rule and fell through to manual review by default. Above 30% is high; below 10% is good.
- **Uncovered Tools** — Claude Code tool types (Bash, Write, Edit, etc.) that are frequently escalating without a rule match.
- **Uncovered Bash Programs** — specific executables (git, npm, docker, etc.) whose commands frequently escape all rules.

> **Note on compound commands**: `command_program` in the analytics is the *first* program in a compound pipeline, not necessarily the uncovered one. If `cd` or `find` shows as an uncovered program, the real gap is usually a second command in the pipeline (e.g., `cd /tmp && jar xf ...` → gap is `jar`, recorded as `cd`). Query `command_subcommand_stats` or sample `command_preview` to find the actual gap.

**Action**: For each row in these tables, click "Add rule →" to open the rules editor. Then:
- If the program is consistently safe in your workflow → add an **auto-allow** rule
- If the program is consistently risky → add an **auto-deny** rule
- If it depends on the specific subcommand → apply the **read/write split pattern** (see below)

### 2. Top Triggered Rules

Shows which rules fire most often. Useful for:
- Verifying that important rules are actually being used (not stale)
- Finding rules that fire so often they might need sub-rules for finer control
- Identifying if the default escalate path is larger than expected

### 3. Top Tools

Shows which Claude Code tools are most active overall. If a tool like `Bash` is dominant but has a high coverage gap rate, it means the existing Bash rules are too narrow.

### 4. Top Bash Programs

Shows which executables Claude uses most via the `Bash` tool. If a program appears here but NOT in the "Uncovered Programs" list, it means an existing rule is already handling it — good. If it appears in both, it needs a rule.

### 5. Top Python Imports

Shows Python modules imported in inline `-c` invocations. High use of `requests`, `urllib`, or `httpx` suggests Claude is making HTTP calls from Python — worth adding a rule if you want to review those.

---

## Deciding what rules to add

Use this checklist when looking at the coverage gap data:

```
□ What tool is unmatched? (Bash, Write, Edit, Read, etc.)
□ If Bash: what program? (git, npm, curl, docker, …)
□ Is this program category safe in my workflow? (vcs=usually safe, network=review)
□ Does the program have both safe read-only ops AND risky write ops?
  → YES: apply the Read/Write Split Pattern below
  → NO:  decide allow vs. escalate for the whole program
□ What subcommands/flags are most common? (git commit vs git push have different risk)
□ Is there a pattern in the command text I can use? (regex on command field)
□ Should this be auto-allow, auto-deny, or explicit escalate?
```

**Safe patterns to auto-allow:**
- Read-only VCS commands: `git status`, `git log`, `git diff`
- Package-manager info queries: `npm list`, `pip show`
- Build commands that don't touch secrets or deploy: `go build`, `cargo build`
- Test runners that don't require network: `go test`, `pytest ./...`

**Patterns to auto-deny:**
- Commands writing to `.env` or credential files
- `rm -rf` on non-tmp paths
- `curl` / `wget` piped to `sh` or `bash` (supply chain risk)
- Commands containing secrets (handled automatically by the secret scanner)

**Patterns to escalate (explicit):**
- `git push` (changes remote state)
- `npm publish`, `cargo publish` (deploys packages)
- Any cloud CLI deploy commands (`kubectl apply`, `terraform apply`)

---

## Read/Write Split Pattern

Many CLIs have *safe* read-only subcommands that Claude should run autonomously alongside *risky* write operations that need review. The pattern is:

```
Priority 100 (AutoAllow): explicit safe read-only operations
Priority  50 (Escalate):  everything else from this program
```

This ensures Claude can inspect state freely while write operations bubble up for review, and new unrecognized subcommands automatically escalate rather than silently allowing.

### When to use it

Apply the split pattern when a program:
1. Has well-defined read-only subcommands/flags that are safe in any context
2. Also has write operations that modify persistent state (files, network, packages, DB)
3. Shows up in the analytics as both frequently used AND frequently escalating

### Implementation approaches

**Approach A — `BlockedSubcommands` (best for verb-based CLIs)**

Use when the same top-level subcommand (`route`, `addr`) has both read and write verbs. Add the program to `deepSubcommandPrograms` so two-token subcommands are captured, then block known write verbs in the allow rule. Anything not blocked auto-allows; anything blocked falls through to the escalate catch-all.

```go
// command_parser.go: add to deepSubcommandPrograms
"ip": true,

// classifier.go: allow at Priority 100
{
    ID:       "seed-allow-bash-ip-read",
    Criteria: &CommandCriteria{
        Programs: []string{"ip"},
        BlockedSubcommands: []string{
            "route add", "route del", "route flush",
            "addr add", "addr del",
            "link set", "link add",
            // ... full list of write verbs
        },
    },
    Decision: AutoAllow, Priority: 100,
},
// escalate catch-all at Priority 50 catches what wasn't allowed
{
    ID: "seed-escalate-ip-networking",
    Criteria: &CommandCriteria{Programs: []string{"ip"}},
    Decision: Escalate, Priority: 50,
},
```

**Approach B — `CommandPattern` regex (best for flag-based CLIs)**

Use when the program uses mode flags (e.g., `pacman -Q` vs `-S`) rather than positional subcommands. An allow regex matches known-safe flag variants; the escalate catch-all handles everything else.

```go
// classifier.go: allow at Priority 100
{
    ID:             "seed-allow-bash-pacman-query",
    CommandPattern: regexp.MustCompile(`^pacman\s+(-Q[a-zA-Z]*\b|--query\b|-F[a-zA-Z]*\b|--files\b|-[Vh]\b)`),
    Decision:       AutoAllow, Priority: 100,
},
// escalate catch-all at Priority 50
{
    ID: "seed-escalate-pacman",
    Criteria: &CommandCriteria{Programs: []string{"pacman"}},
    Decision: Escalate, Priority: 50,
},
```

**Approach C — `CommandPattern` with argument inspection (best for query-language CLIs)**

Use when the program accepts a query language (SQL, dot commands) rather than subcommands. Match only the known-safe query forms; all others escalate.

```go
// Allow only read-only dot commands for sqlite3
{
    ID:             "seed-allow-bash-sqlite3-read",
    CommandPattern: regexp.MustCompile(
        `\bsqlite3\b\s+\S+\s+["']?\.(tables|databases?|schema(\s+\w+)?|indexes?(\s+\w+)?|pragma\s+\w+)["']?\s*$`,
    ),
    Decision: AutoAllow, Priority: 100,
},
// escalate all other sqlite3 (SQL queries, DML, DDL)
{
    ID: "seed-escalate-sqlite3",
    Criteria: &CommandCriteria{Programs: []string{"sqlite3"}},
    Decision: Escalate, Priority: 50,
},
```

### Currently implemented splits

| Program | Read-only → AutoAllow | Write → Escalate |
|---------|----------------------|-----------------|
| `git` | status, log, diff, show, fetch, config, ls-remote, … | push, filter-repo, filter-branch |
| `gh` | pr view/list, run view/list, issue view/list, … | pr create, issue create, workflow run, secret set, … |
| `curl` | GET requests without output flags | -o/-O (file output), POST/PUT/DELETE/PATCH |
| `docker` | ps, images, logs, inspect, info | exec, run, rm, build, pull, push, compose |
| `systemctl` | status, is-active, list-units, cat | start, stop, restart, enable, daemon-reload |
| `tmux` | list-sessions, list-windows, display-message | new-session, send-keys, run-shell |
| `ip` | route show, addr show, link show, neigh show (bare too) | route add/del/flush, addr add/del, link set |
| `pacman` | -Q (all variants), -F, -V, -h | -S (install), -R (remove), -U, -D |
| `sqlite3` | .tables, .schema, .indexes, .databases, .pragma | SQL queries, DML, bare invocation |
| `tailscale` | status, ip, dns, ping, netcheck, version | (rest escalates) |

---

## Java API Discovery Workflow (fully auto-approved)

The `java-api-discovery` skill uses a compound `jar xf + javap` pipeline to inspect compiled JARs from Gradle caches. All steps in this workflow are auto-approved:

```bash
# Step 1: list classes in a JAR (read-only)
jar tf /path/to/lib.jar | grep "ClassName"

# Step 2: extract a specific class to /tmp
cd /tmp && jar xf /path/to/lib.jar com/example/ClassName.class

# Step 3: disassemble (read-only)
javap -p /tmp/com/example/ClassName.class

# Step 4: find + batch disassemble
find /tmp/com/example -name "*.class" | xargs javap -p

# Step 5: find JARs and inspect
find ~/.gradle -name "*.jar" | xargs jar tf | grep "ClassName"
```

All components are covered: `jar` (allow), `javap` (allow), `xargs jar` (allow), `xargs javap` (allow), `find` (allow), `cd` + compound (allow).

---

## How to create a rule

1. Go to **http://localhost:8543/rules**
2. Scroll to the **Add Custom Rule** form at the bottom of the Rules panel
3. Fill in:
   - **Name** — descriptive, e.g. "Allow go test"
   - **Decision** — auto_allow / auto_deny / escalate
   - **Tool Name** — exact tool name, e.g. `Bash` (case-sensitive)
   - **Command Pattern** — regex on the full command text, e.g. `^go\s+test\b`
   - **Reason** — explain why this rule exists (shown to Claude on deny)
   - **Alternative** — (optional) suggest what Claude should do instead
   - **Priority** — higher number = evaluated first (seed rules are at 1000/500/100/50)
4. Click **Save Rule**

Rules take effect immediately without a restart.

---

## Rule pattern tips

```
# Match a specific program exactly (starts with):
^git\s

# Match git read-only subcommands:
^git\s+(status|log|diff|show|branch|remote|describe)\b

# Match any npm install variant:
^npm\s+(install|i|ci)\b

# Match curl/wget downloads to disk:
^(curl|wget)\s+.*\s+-[oO]

# Match python -c with requests import (network calls):
^python3?\s+-c\s+.*\bimport\s+requests\b

# Deny writes to credential files:
\.(env|pem|key|p12|pfx|credentials)$

# Match pacman query mode only (not install/remove):
^pacman\s+(-Q[a-zA-Z]*|--query)\b

# Match sqlite3 read-only dot commands only:
\bsqlite3\b\s+\S+\s+["']?\.(tables|schema|indexes?)\b
```

---

## Keeping rules evergreen

Review the analytics weekly or after major changes to your workflow:

1. **Check top uncovered programs** — new tools Claude is using that need rules
2. **Check stale rules** — rules in the Rules panel that haven't triggered recently may be too specific or no longer needed
3. **Check manual review rate trend** — if it's rising, you have new patterns to cover; if it's stable and low, rules are healthy
4. **After adding a new Claude skill or tool** — Claude may start using new programs; check the analytics a day later
5. **After adding a read/write split** — verify the write escalate still fires for risky ops via the smoke-test pattern in `classifier_test.go`

---

## Backend files (for code changes)

| File | Purpose |
|------|---------|
| `pkg/classifier/classifier.go` | Rule matching engine + all seed rules |
| `pkg/classifier/command_parser.go` | Bash AST parser, wrapperCommands, deepSubcommandPrograms |
| `server/services/rules_store.go` | RPC handlers + user rule persistence |
| `server/services/analytics_store.go` | SQLite analytics storage + aggregation |
| `server/services/approval_handler.go` | HTTP hook handler + secret scanner + domain checker |
| `server/services/secret_scanner.go` | Regex patterns for plaintext secret detection |
| `server/services/domain_checker.go` | RDAP-based new-domain escalation |
| `proto/session/v1/types.proto` | Proto definitions (run `make generate-proto` after changes) |
| `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | Analytics UI |
| `web-app/src/components/sessions/ApprovalRulesPanel.tsx` | Rules management UI |
