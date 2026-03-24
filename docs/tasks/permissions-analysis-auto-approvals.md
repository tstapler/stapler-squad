# Permissions Analysis & Automatic Approvals

**Date**: 2026-03-23 (planned), 2026-03-24 (implemented)
**Status**: Implemented
**Scope**: Intelligent auto-approval layer that classifies tool use risk, auto-handles low-risk operations, auto-denies high-risk operations with alternatives, and escalates ambiguous requests to the manual review queue.

---

## Table of Contents

- [Overview](#overview)
- [Architecture Decisions](#architecture-decisions)
  - [ADR-020: Rule-Based Classification over ML](#adr-020-rule-based-classification-over-ml)
  - [ADR-021: Append-Only JSONL Analytics](#adr-021-append-only-jsonl-analytics)
  - [ADR-022: Claude Settings Integration via Read-Only Parse](#adr-022-claude-settings-integration-via-read-only-parse)
  - [ADR-023: Three Decision Outcomes](#adr-023-three-decision-outcomes)
  - [ADR-024: Priority Tier Conventions](#adr-024-priority-tier-conventions)
  - [ADR-025: Optional Injection via Setter Methods](#adr-025-optional-injection-via-setter-methods)
- [Component Overview](#component-overview)
- [Data Flow](#data-flow)
- [Story Breakdown (Retrospective)](#story-breakdown-retrospective)
  - [Story 1: Risk Classifier Engine](#story-1-risk-classifier-engine)
  - [Story 2: Rules Engine and Config Integration](#story-2-rules-engine-and-config-integration)
  - [Story 3: Analytics Store and Decision Log](#story-3-analytics-store-and-decision-log)
  - [Story 4: Handler Integration](#story-4-handler-integration)
  - [Story 5: Proto/API Extensions](#story-5-protoapi-extensions)
  - [Story 6: Web UI -- Rules and Analytics](#story-6-web-ui----rules-and-analytics)
- [Seed Rules Reference](#seed-rules-reference)
- [Testing Strategy](#testing-strategy)
- [Known Gaps and Recommended Follow-Up](#known-gaps-and-recommended-follow-up)
- [Rollout Considerations](#rollout-considerations)
- [File Inventory](#file-inventory)

---

## Overview

Before this feature, every Claude Code tool use request arriving via the HTTP hook at `/api/hooks/permission-request` blocked on the manual review queue. Users had to approve even trivially safe operations such as `ls`, `git status`, or file reads. This caused constant interruptions and slowed down autonomous agents.

The auto-approval layer sits at the top of `HandlePermissionRequest`, before the existing manual review queue. It evaluates each incoming tool use request against a priority-ordered list of regex-based rules and makes one of three decisions:

1. **AutoAllow** -- immediately return HTTP 200 with `behavior=allow`. No PendingApproval created, no UI notification, no user interruption.
2. **AutoDeny** -- immediately return HTTP 200 with `behavior=deny` and a message combining the rule's reason and a suggested alternative. Claude receives the denial and can adjust.
3. **Escalate** -- fall through to the existing manual review queue. The request blocks until a user approves or denies via the web UI, or until the 4-minute timeout expires.

Every decision (including escalated ones resolved manually) is recorded asynchronously to an append-only JSONL analytics log. Users manage rules and view analytics through a dedicated `/rules` page in the web UI.

---

## Architecture Decisions

### ADR-020: Rule-Based Classification over ML

**Context**: Classifying Bash commands and file paths for risk requires pattern recognition. ML inference was considered.

**Decision**: Pure regex/glob pattern matching with a tiered priority system.

**Rationale**:
- ML inference would add 50-500ms latency; unacceptable in the HTTP hook path where the 4-minute timeout is a hard constraint
- Rules are deterministic, auditable, and user-editable
- 90% of cases are structurally simple: tool name + command prefix + path pattern
- Users can inspect and tune rules directly; no black-box behavior

**Consequences**: Cannot handle novel adversarial inputs without explicit rules. The seed rule set covers the most common tools and commands, and users can add custom rules for their specific workflows.

**Patterns Applied**: Strategy (classifier interface with pluggable implementations), Chain of Responsibility (priority-ordered rule evaluation).

**Implementation**: `RuleBasedClassifier` in `server/services/classifier.go` with `Classify()` iterating `[]Rule` sorted by `Priority` descending, returning the first match.

---

### ADR-021: Append-Only JSONL Analytics

**Context**: Every classification decision needs to be recorded for rule tuning, without blocking the hot path.

**Decision**: Asynchronous buffered writes to `~/.stapler-squad/approval_analytics.jsonl`.

**Rationale**:
- Zero-blocking: `Record()` sends to a buffered channel (capacity 1000); a background goroutine flushes to disk every 5 seconds or 10 entries
- Human-readable JSONL format enables offline analysis with `jq`
- No schema migration risk
- SQLite migration can happen later (see `docs/tasks/repository-pattern-sqlite-migration.md`)

**Consequences**: No indexed queries; the analytics API performs a linear scan filtered by timestamp. Acceptable for the expected volume of fewer than 100K entries per 30-day window.

**Implementation**: `AnalyticsStore` in `server/services/analytics_store.go` with a `chan AnalyticsEntry` buffer. Dropped entries are counted atomically. `ComputeSummary()` is a pure function with no I/O.

---

### ADR-022: Claude Settings Integration via Read-Only Parse

**Context**: Claude Code already has `~/.claude/settings.json` with a `permissions.allow` list. Stapler-squad should respect these to avoid double-prompting.

**Decision**: Read Claude settings files at startup (4 paths: global, global-local, project, project-local) and convert `permissions.allow` entries to AutoAllow rules with `Source="claude-settings"`.

**Rationale**: Respects the user's existing configuration as ground truth. If Claude Code already allows `Bash(git log*)`, stapler-squad should not prompt for it.

**Implementation**: `LoadClaudeSettingsRules()` in `server/services/claude_settings_parser.go` reads from 4 paths at priorities 150-180. `globToRegex()` converts Claude's glob patterns (e.g., `Bash(git log*)`) to anchored regex (`^git log.*`).

**Consequences**: Settings are read at startup only (no fsnotify watcher for Claude settings files). If a user changes their Claude settings, they need to restart the server to pick up the new rules. The rules store has its own fsnotify watcher for hot-reload of user rules.

---

### ADR-023: Three Decision Outcomes

**Context**: The Claude Code hook expects a response with `behavior` set to `allow` or `deny`. There is no explicit `ask` -- the existing manual review queue blocks the HTTP connection open until the user decides.

**Decision**: The classifier returns one of three decisions:
- `AutoAllow` -- return immediately with `allow`
- `AutoDeny` -- return immediately with `deny` + reason message
- `Escalate` -- fall through to manual review queue (existing behavior)

**Rationale**: `Escalate` preserves the entire existing manual review flow unchanged. The classifier only short-circuits the flow for clear-cut cases.

**Implementation**: The `ClassificationDecision` enum in `classifier.go` has three values. The switch statement in `HandlePermissionRequest` handles `AutoAllow` and `AutoDeny` inline and falls through for `Escalate`.

---

### ADR-024: Priority Tier Conventions

**Context**: Rules from three sources (seed, user, claude-settings) must coexist without collisions, and users need the ability to override seed rules.

**Decision**: Priority tiers:
- P1000: Seed AutoDeny rules (critical safety guardrails like `.env` writes, `rm -rf /`)
- P150-180: Claude settings rules (read from settings files)
- P100: Seed AutoAllow and Escalate rules
- P50: Seed Escalate rules (lower priority)
- P1-999: User rules (custom)

Higher priority is evaluated first. This means seed safety guardrails at P1000 always run before anything else, but users can add rules at any priority in the 1-999 range.

**Implementation**: `SeedRules()` in `classifier.go` returns rules at priorities 1000, 100, and 50. User rules default to priority 10 in the UI form. Claude settings rules are loaded at 150-180 depending on scope (global vs project).

---

### ADR-025: Optional Injection via Setter Methods

**Context**: The `ApprovalHandler` predates the classifier and analytics store. Adding them as constructor parameters would break the existing initialization sequence in `server.go`.

**Decision**: Use setter methods (`SetClassifier`, `SetAnalyticsStore`) rather than constructor injection.

**Rationale**: The classifier and analytics store are created by `SessionService` (which manages rules, analytics, and the ConnectRPC handlers). The `ApprovalHandler` is created separately as an HTTP handler. Setter injection allows `server.go` to wire them together after both are constructed, without a circular dependency.

**Implementation**: `approval_handler.go` has `SetClassifier(c *RuleBasedClassifier)` and `SetAnalyticsStore(a *AnalyticsStore)`. Both fields are nil-safe: if the classifier is nil, all requests fall through to manual review (safe default).

---

## Component Overview

```
                           +-----------------------+
    Claude Code            |   ApprovalHandler     |
    HTTP Hook  ----------->|  HandlePermission-    |
    (POST)                 |  Request()            |
                           +----------+------------+
                                      |
                           1. Parse payload
                           2. Resolve session ID
                                      |
                           +----------v------------+
                           | RuleBasedClassifier    |
                           |  .Classify(payload,    |
                           |   context)             |
                           +----------+------------+
                                      |
                    +-----------------+------------------+
                    |                 |                   |
              AutoAllow          AutoDeny            Escalate
                    |                 |                   |
              Return allow     Return deny         Create PendingApproval
              + analytics      + reason/alt        + broadcast notification
              entry            + analytics         + block on decisionCh
                               entry               + analytics entry
                                                   (when resolved)
```

### Backend Components

| Component | File | Lines | Purpose |
|-----------|------|-------|---------|
| RuleBasedClassifier | `server/services/classifier.go` | 336 | Core classification engine: Rule struct, Classify(), BuildContext(), SeedRules() |
| RulesStore | `server/services/rules_store.go` | 297 | User rule persistence (JSON), Upsert/Delete, WatchAndReload (fsnotify) |
| AnalyticsStore | `server/services/analytics_store.go` | 373 | Async JSONL writer, LoadWindow(), ComputeSummary() |
| RulesService | `server/services/rules_service.go` | 287 | ConnectRPC handlers: List/Upsert/Delete rules, GetAnalytics |
| ClaudeSettingsParser | `server/services/claude_settings_parser.go` | 140 | Reads Claude settings.json permissions.allow, converts to rules |
| ApprovalHandler | `server/services/approval_handler.go` | 461 | HTTP handler (modified): classification at top of HandlePermissionRequest |
| SessionService | `server/services/session_service.go` | (modified) | Creates RulesStore, AnalyticsStore, Classifier; delegates 4 RPCs |
| Server | `server/server.go` | (modified) | Wires SetClassifier/SetAnalyticsStore into ApprovalHandler |

### Frontend Components

| Component | File | Lines | Purpose |
|-----------|------|-------|---------|
| useApprovalRules | `web-app/src/lib/hooks/useApprovalRules.ts` | 118 | Hook: list/upsert/delete rules via ConnectRPC |
| useApprovalAnalytics | `web-app/src/lib/hooks/useApprovalAnalytics.ts` | 70 | Hook: load AnalyticsSummary for configurable window |
| ApprovalRulesPanel | `web-app/src/components/sessions/ApprovalRulesPanel.tsx` | 420 | Full rules management UI with analytics summary bar |
| Rules page | `web-app/src/app/rules/page.tsx` | 14 | Next.js page at /rules route |
| Routes | `web-app/src/lib/routes.ts` | (modified) | Added `rules: "/rules"` |
| Header | `web-app/src/components/layout/Header.tsx` | (modified) | Added "Rules" nav link |

### Proto Definitions

| Message/Enum | File | Purpose |
|-------------|------|---------|
| AutoDecision | `proto/session/v1/types.proto` | ALLOW / DENY / ESCALATE enum |
| ApprovalRuleProto | `proto/session/v1/types.proto` | Wire format for rules (14 fields) |
| AnalyticsSummaryProto | `proto/session/v1/types.proto` | Aggregated analytics (9 fields) |
| ToolStatProto | `proto/session/v1/types.proto` | Tool name + count |
| CommandStatProto | `proto/session/v1/types.proto` | Command preview + tool + count |
| RuleStatProto | `proto/session/v1/types.proto` | Rule ID + name + count |
| 4 RPCs | `proto/session/v1/session.proto` | ListApprovalRules, UpsertApprovalRule, DeleteApprovalRule, GetApprovalAnalytics |

---

## Data Flow

### 1. Incoming Hook Request

```
Claude Code ──POST──> /api/hooks/permission-request
                       │
                       ├─ Header: X-CS-Session-ID: "my-session"
                       └─ Body: { tool_name: "Bash", tool_input: { command: "git status" }, cwd: "/repo" }
```

### 2. Classification (in HandlePermissionRequest)

```
1. Parse PermissionRequestPayload from JSON body
2. Resolve session ID (X-CS-Session-ID header, then cwd prefix match)
3. IF classifier != nil:
   a. BuildContext(cwd) -- detect git repo, worktree
   b. Classify(payload, context)
   c. RecordFromResult() -- async analytics entry
   d. Switch on result.Decision:
      - AutoAllow -> writeDecision(w, "allow", "") -> RETURN
      - AutoDeny  -> writeDecision(w, "deny", reason + alternative) -> RETURN
      - Escalate  -> fall through
4. Create PendingApproval, broadcast notification, block on decisionCh
```

### 3. Rule Evaluation (in RuleBasedClassifier.Classify)

```
for each rule in rules (sorted by Priority DESC):
    if !rule.Enabled: skip
    if rule.ToolName != "" and != payload.ToolName: skip
    else if rule.ToolPattern != nil and !match: skip
    if rule.CommandPattern != nil and !match(command): skip
    if rule.FilePattern != nil and !match(file_path): skip
    if rule.ID == "seed-bash-find-name" and dangerousFindFlags.match(command): skip
    -> MATCH: return ClassificationResult
default: return Escalate
```

### 4. Analytics Recording

```
AnalyticsStore.RecordFromResult()
    │
    └─> channel (buffered, 1000)
         │
         └─> flush goroutine (every 5s or 10 entries)
              │
              └─> append to ~/.stapler-squad/approval_analytics.jsonl
```

### 5. Rule Management (Web UI)

```
/rules page
    │
    ├─ useApprovalRules hook
    │   ├─ listApprovalRules() -> all rules (user + seed + claude-settings)
    │   ├─ upsertApprovalRule() -> save user rule, rebuild classifier
    │   └─ deleteApprovalRule() -> remove user rule, rebuild classifier
    │
    └─ useApprovalAnalytics hook
        └─ getApprovalAnalytics(windowDays=7) -> AnalyticsSummary
```

### 6. Classifier Rebuild on Rule Change

```
RulesService.rebuildClassifier()
    │
    ├─ Get all rules from classifier
    ├─ Filter out source="user" rules
    ├─ Load fresh user rules from RulesStore.ToRules()
    └─ ReplaceRules(nonUser + freshUser) -- atomic swap, re-sorted by priority
```

---

## Story Breakdown (Retrospective)

### Story 1: Risk Classifier Engine

**What was delivered**: The core classification engine in `server/services/classifier.go` (336 lines) with 15 unit tests in `classifier_test.go` (280 lines).

**Key design decisions**:
- `Classifier` is defined as an interface with `Classify()` and `BuildContext()` methods, allowing future alternative implementations
- `Rule` struct uses compiled `*regexp.Regexp` fields for zero-allocation matching at classification time
- `RuleBasedClassifier` uses `sync.RWMutex` for concurrent safety: `Classify()` holds a read lock, `ReplaceRules()`/`AddRules()` hold a write lock
- `BuildContext()` shells out to `git rev-parse` to detect repo root and worktree status; no caching was implemented (the syscall is fast enough for the hook path)
- `matchesRule()` uses a conjunction model: all non-nil criteria must match. This means a rule with `ToolName="Bash"` and `CommandPattern=^git log` only matches Bash commands starting with `git log`
- Special case: `seed-bash-find-name` rule matches `find` commands but rejects them if `dangerousFindFlags` regex detects `-exec`, `-delete`, pipes, or semicolons

**Seed rules delivered** (10 rules across 3 tiers): see [Seed Rules Reference](#seed-rules-reference).

**Deviation from plan**: The plan called for a `RiskLevel.ReadOnly` enum value. The implementation uses `RiskLow`, `RiskMedium`, `RiskHigh`, `RiskCritical` (4 levels, no ReadOnly). This simplification was adequate because the decision enum (`AutoAllow`/`AutoDeny`/`Escalate`) already captures the behavioral intent.

---

### Story 2: Rules Engine and Config Integration

**What was delivered**:
- `RulesStore` in `server/services/rules_store.go` (297 lines): JSON-backed persistence with `RuleSpec` (string-based, serializable version of `Rule`), atomic writes via tmp-file-then-rename, fsnotify hot-reload with 100ms debounce
- `ClaudeSettingsParser` in `server/services/claude_settings_parser.go` (140 lines): reads 4 settings paths at priorities 150-180, parses `permissions.allow` patterns like `Bash(git log*)`, converts glob `*` to anchored regex

**Key design decisions**:
- `RuleSpec` is a separate struct from `Rule` because `Rule` contains compiled `*regexp.Regexp` fields that cannot be serialized to JSON. `specsToRules()` compiles the string patterns into regex at load time, skipping invalid patterns with a warning log
- `RulesStore.Upsert()` validates all regex patterns before persisting, preventing corrupt rules from being saved
- `RulesStore.Delete()` refuses to delete non-user rules (`Source != "user"`), protecting seed and claude-settings rules
- `WatchAndReload()` watches the parent directory (not the file itself) to catch atomic rename operations. Uses a debounce timer to coalesce rapid filesystem events
- Claude settings are read at server startup only (when `LoadClaudeSettingsRules()` is called from `NewSessionService()`). There is no fsnotify watcher for Claude settings files -- this is a known gap

**File format**: `auto_approve_rules.json` with versioned envelope:
```json
{
  "version": 1,
  "rules": [
    {
      "id": "user-1711234567890",
      "name": "Allow npm test",
      "tool_name": "Bash",
      "command_pattern": "^npm test",
      "decision": "auto_allow",
      "risk_level": "low",
      "priority": 10,
      "enabled": true,
      "source": "user",
      "created_at": "2026-03-24T10:00:00Z"
    }
  ]
}
```

---

### Story 3: Analytics Store and Decision Log

**What was delivered**: `AnalyticsStore` in `server/services/analytics_store.go` (373 lines) with async JSONL recording, windowed loading, and pure-function aggregation.

**Key design decisions**:
- Buffered channel with capacity 1000. `Record()` is non-blocking: if the buffer is full, the entry is dropped and an atomic counter (`dropped`) is incremented. This prevents the analytics subsystem from ever blocking the HTTP hook handler
- Background flush goroutine drains the channel and appends to JSONL. Flush triggers: every 5 seconds (ticker) or when 10 entries accumulate (batch threshold). `fsync` is called only when flushing 10+ entries to balance durability with performance
- `LoadWindow(since time.Time)` performs a linear scan of the JSONL file, filtering by timestamp. Uses a 1MB line buffer to handle long command previews. Malformed lines are skipped with a warning log, not an error
- `ComputeSummary()` is a pure function (no I/O, no side effects) that takes `[]AnalyticsEntry` and returns `AnalyticsSummary` with decision counts, top-10 tools, top-10 denied commands, top-10 triggered rules, auto-approve rate, and manual review rate
- `RecordFromResult()` is a convenience method that extracts the command or file path from `ToolInput`, truncates to 200 chars for the preview, and builds an `AnalyticsEntry`
- `RecordManualDecision()` records `manual_allow` or `manual_deny` decisions from the existing approval flow

**Deviation from plan**: The plan called for a per-day `sync.Map` cache for loaded entries. The implementation does not cache loaded entries -- it re-reads the file on each `LoadWindow()` call. This is simpler and adequate for the expected volume, though it means analytics queries scan the full file each time.

---

### Story 4: Handler Integration

**What was delivered**: Classification logic wired into `HandlePermissionRequest` in `server/services/approval_handler.go` (461 lines total after modification).

**Key design decisions**:
- Classification runs at the top of `HandlePermissionRequest`, after payload parsing and session ID resolution, but before creating a `PendingApproval`
- The classifier and analytics store are optional (nil-safe). If `h.classifier == nil`, all requests fall through to manual review -- this is the backward-compatible default
- Classification latency is measured with `time.Since(start)` and recorded in the analytics entry
- For `AutoDeny`, the `message` field in the hook response combines `result.Reason` and `result.Alternative` (space-separated). Claude Code receives this as feedback explaining why the tool use was denied
- The `systemMessage` field was planned (Task 4.2 in the original plan) but was not implemented as a separate JSON field. Instead, the combined reason+alternative is passed via the existing `message` field in the `hookDecision` struct. This is sufficient because Claude Code displays the denial message to the user

**Wiring in server.go**:
```go
approvalHandler.SetClassifier(deps.SessionService.GetClassifier())
approvalHandler.SetAnalyticsStore(deps.SessionService.GetAnalyticsStore())
```

This happens after both `SessionService` (which owns the classifier) and `ApprovalHandler` (which uses it) are constructed.

---

### Story 5: Proto/API Extensions

**What was delivered**: 4 new RPCs and supporting message types in `proto/session/v1/session.proto` and `proto/session/v1/types.proto`.

**RPCs**:
- `ListApprovalRules(ListApprovalRulesRequest) returns (ListApprovalRulesResponse)` -- optional `source_filter` parameter
- `UpsertApprovalRule(UpsertApprovalRuleRequest) returns (UpsertApprovalRuleResponse)` -- returns rule + created flag
- `DeleteApprovalRule(DeleteApprovalRuleRequest) returns (DeleteApprovalRuleResponse)` -- returns success + message
- `GetApprovalAnalytics(GetApprovalAnalyticsRequest) returns (GetApprovalAnalyticsResponse)` -- optional `window_days` (default 7, max 90)

**Key design decisions**:
- All 4 RPCs are registered on the existing `SessionService` (delegated to `RulesService` internally) rather than creating a new service. This avoids adding a new ConnectRPC handler path and keeps the client transport configuration simple
- `ApprovalRuleProto` has 14 fields covering all rule attributes including `tool_pattern` (for regex-based tool matching, separate from exact `tool_name` match)
- `RiskLevel` was kept as a string field in the proto (not an enum) to allow for future extensibility without proto-level changes. The `AutoDecision` enum was defined in proto because it has a fixed set of values
- `AnalyticsSummaryProto` uses `map<string, int32>` for `decision_counts` to accommodate the 5 decision types without a fixed enum

**Deviation from plan**: The plan proposed a separate `RiskLevel` enum in proto. The implementation uses `string risk_level` in `ApprovalRuleProto`, with the Go code mapping to/from the internal `RiskLevel` int type. This gives more flexibility but less type safety on the wire.

---

### Story 6: Web UI -- Rules and Analytics

**What was delivered**: Rules management panel with analytics summary bar, accessible at `/rules` via a nav link in the header.

**Components**:
- `ApprovalRulesPanel` (420 lines): full CRUD interface for rules with source filter tabs, analytics summary bar, rule table, and add-rule form
- `useApprovalRules` hook (118 lines): ConnectRPC client for list/upsert/delete with optimistic delete
- `useApprovalAnalytics` hook (70 lines): ConnectRPC client for analytics summary with configurable window

**Key design decisions**:
- Source filter tabs (All / Custom / Built-in / Claude Settings) let users focus on specific rule types
- Built-in and claude-settings rules are shown read-only: the enable/disable toggle and delete button are disabled for non-user rules
- The analytics summary bar shows 7-day aggregates inline (total decisions, auto-allow rate, manual review rate, top tool) rather than a full analytics dashboard. This was a scope reduction from the plan, which called for charts and a separate analytics tab
- The add-rule form generates IDs as `user-{Date.now()}` for uniqueness
- Optimistic delete: the rule is removed from the local state immediately, without waiting for the server response. The upsert path does not use optimistic updates (waits for `refresh()` after save)
- CSS modules are used for styling (`ApprovalRulesPanel.module.css`), consistent with the project's existing pattern

**Deviation from plan**: The plan proposed separate Rules and Analytics tabs with a pie chart, bar charts, and a recent denials table. The implementation provides a simpler inline analytics summary bar and no charting library. This was a pragmatic scope reduction -- the summary bar gives the user the key metrics at a glance, and detailed analytics can be added later.

---

## Seed Rules Reference

| ID | Name | Tool | Pattern | Decision | Priority | Risk |
|----|------|------|---------|----------|----------|------|
| seed-deny-env-write | Block writes to .env files | Write/Edit/MultiEdit | `file_path ~ (^|/)\.env(\.|$)` | AutoDeny | 1000 | Critical |
| seed-deny-git-internals-write | Block writes to .git internals | Write/Edit/MultiEdit | `file_path ~ (^|/)\.git/` | AutoDeny | 1000 | Critical |
| seed-deny-rm-rf-root | Block rm -rf on root/home | Bash | `command ~ rm\s+(-rf|-fr)\s+(/|~|\$HOME)` | AutoDeny | 1000 | Critical |
| seed-allow-read-tools | Allow read-only tools | Read/Glob/Grep/WebFetch/WebSearch/MCP | (any input) | AutoAllow | 100 | Low |
| seed-allow-bash-ls-pwd | Allow inspection commands | Bash | `command ~ ^\s*(ls|pwd|echo|printenv|...)` | AutoAllow | 100 | Low |
| seed-bash-find-name | Allow find (no exec/delete) | Bash | `command ~ ^\s*find\s+` (rejects `-exec`, `-delete`, pipes) | AutoAllow | 100 | Low |
| seed-allow-bash-cat-read | Allow file read commands | Bash | `command ~ ^\s*(cat|head|tail|wc|file|...)` | AutoAllow | 100 | Low |
| seed-allow-git-read | Allow read-only git commands | Bash | `command ~ ^\s*git\s+(status|log|diff|show|...)` | AutoAllow | 100 | Low |
| seed-escalate-git-push | Escalate git push | Bash | `command ~ ^\s*git\s+push` | Escalate | 50 | High |
| seed-escalate-network-write | Escalate curl/wget with output | Bash | `command ~ ^\s*(curl|wget)\s+.*(-o|-O|--output)` | Escalate | 50 | High |

---

## Testing Strategy

### Unit Tests (classifier_test.go -- 15 tests)

| Test | What it validates |
|------|-------------------|
| TestClassify_ReadTools_AutoAllow | Read, Glob, Grep, WebFetch, WebSearch all auto-allowed |
| TestClassify_BashInspection_AutoAllow | ls, ls -la, pwd, echo, which, date, whoami auto-allowed |
| TestClassify_FindName_AutoAllow | `find . -name '*.go'` auto-allowed |
| TestClassify_FindExec_NotAutoAllow | find with -exec, -delete, pipes, semicolons NOT auto-allowed |
| TestClassify_EnvFileWrite_AutoDeny | Write/Edit/MultiEdit to .env, .env.local, .env.production auto-denied |
| TestClassify_GitInternalsWrite_AutoDeny | Write to .git/hooks/pre-commit auto-denied |
| TestClassify_RmRfRoot_AutoDeny | rm -rf /, rm -rf ~/, rm -rf $HOME, rm -fr / auto-denied |
| TestClassify_GitPush_Escalate | git push origin main escalated |
| TestClassify_GitReadOnly_AutoAllow | git status, log, diff, branch, remote auto-allowed |
| TestClassify_CatHead_AutoAllow | cat, head, tail, wc, diff auto-allowed |
| TestClassify_UnknownTool_Escalate | SomeFutureTool escalated (safe default) |
| TestClassify_DisabledRule_Skipped | All rules disabled -> Escalate |
| TestClassify_ReplaceRules_Atomic | After ReplaceRules, only new rules apply |
| TestClassify_AddRules_HighPriorityFirst | High-priority added rule overrides seed rule |
| TestSeedRules_SortedByPriority | SeedRules() returns rules sorted by priority descending |

### Integration Tests (approval_handler_integration_test.go -- 6 tests)

| Test | What it validates |
|------|-------------------|
| TestApprovalFlow_Allow | Resolve with "allow" unblocks HTTP handler |
| TestApprovalFlow_Deny | Resolve with "deny" returns deny decision |
| TestApprovalFlow_Timeout | Short timeout -> auto-deny with timeout message |
| TestApprovalFlow_ParseError | Malformed JSON body -> auto-allow (never block Claude on server errors) |
| TestApprovalFlow_MethodNotAllowed | GET request -> 405 |
| TestApprovalFlow_SessionIDFromHeader | X-CS-Session-ID header correctly maps to session |

### What is NOT tested

- RulesStore persistence (no unit tests for JSON roundtrip)
- AnalyticsStore flush and LoadWindow (no unit tests)
- ClaudeSettingsParser (no unit tests for glob-to-regex conversion)
- RulesService RPC handlers (no unit tests for the 4 ConnectRPC handlers)
- Web UI components (no React tests)
- End-to-end classification through the HTTP handler with a real classifier wired in (the integration tests use nil classifier)
- Race condition tests (`go test -race` coverage)

---

## Known Gaps and Recommended Follow-Up

### Gap 1: No Global On/Off Toggle

There is no way to disable auto-classification entirely. If a user experiences false positives, they must either delete/disable individual rules or stop the server. A global toggle (environment variable or config flag) would provide an escape hatch.

**Recommendation**: Add `STAPLER_SQUAD_AUTO_APPROVE=false` environment variable that causes `SetClassifier(nil)` to be called, reverting to manual-only mode.

### Gap 2: No Per-Session Opt-Out

All sessions use the same classifier. Some sessions may need stricter review (e.g., production deployments) while others can be more permissive (local development).

**Recommendation**: Add a per-session `auto_approve_mode` field (on/off/custom-rule-set) that can be set at session creation or updated via the web UI.

### Gap 3: Existing Sessions Do Not Get the Hook

`InjectHookConfig()` is called in `CreateSession()`. Sessions created before the feature was deployed do not have the hook in their `.claude/settings.local.json`. They continue to use Claude Code's built-in approval flow.

**Recommendation**: Run `InjectHookConfig()` at server startup for all active sessions, or provide a "re-inject hooks" button in the web UI.

### Gap 4: Individual Auto-Allow/Deny Events Not Visible in UI

The web UI shows aggregate analytics (auto-allow rate, top tools) but does not show a live feed of individual auto-allowed or auto-denied events. Users cannot see in real-time what is being auto-approved.

**Recommendation**: Add a "Recent Decisions" table to the rules page showing the last N auto-allow/deny events with tool name, command preview, and matched rule.

### Gap 5: Claude Settings Not Hot-Reloaded

`LoadClaudeSettingsRules()` is called once at startup. If the user modifies `~/.claude/settings.json` while the server is running, the changes are not picked up.

**Recommendation**: Add fsnotify watchers for the 4 Claude settings paths, similar to `RulesStore.WatchAndReload()`.

### Gap 6: .env Pattern May False-Positive on .environment Files

The seed rule `seed-deny-env-write` uses pattern `(^|/)\.env(\.|$)` which matches `.env`, `.env.local`, `.env.production` -- but also matches `.env.example` and `.environment`. The plan identified this as Bug 003.

**Recommendation**: Add explicit AutoAllow rules for `.env.example`, `.env.template`, `.env.sample` at a higher priority than the deny rule, or tighten the deny pattern.

### Gap 7: No Test Coverage for RulesStore, AnalyticsStore, or RulesService

The persistence layer (JSON roundtrip, JSONL flush, LoadWindow scan) and the RPC handlers have no unit tests. Edge cases like corrupt files, concurrent writes, or large JSONL files are untested.

**Recommendation**: Add unit tests for `RulesStore.Upsert()`/`Delete()`/`reload()`, `AnalyticsStore.LoadWindow()`/`ComputeSummary()`, and all 4 `RulesService` RPC handlers.

### Gap 8: Analytics Has No Rotation or Size Limit

The JSONL file grows unbounded. Over months of heavy usage, it could become large enough to slow down `LoadWindow()` scans.

**Recommendation**: Implement log rotation (e.g., one file per month) or a maximum file size with truncation of old entries.

---

## Rollout Considerations

### Hook Injection

The auto-approval layer is always-on for any request reaching the `/api/hooks/permission-request` endpoint. However, this endpoint is only called by sessions that have the hook injected via `InjectHookConfig()`.

- **New sessions**: Automatically get the hook injected during `CreateSession()`
- **Existing sessions**: Do NOT get the hook retroactively. They continue to use Claude Code's built-in approval flow. Users must create new sessions or manually add the hook to `.claude/settings.local.json`
- **External (mux) sessions**: Do not have the hook injected at all. They bypass the entire approval system

### Backward Compatibility

- The classifier is nil-safe: if not wired up, all requests fall through to manual review
- The analytics store is nil-safe: if not wired up, no analytics are recorded
- The existing manual review queue is completely unchanged for escalated requests
- The 4 new ConnectRPC RPCs are additive; no existing RPCs are modified
- The `/rules` page is a new route; no existing routes are affected

### Performance Impact

- Classification is in-memory regex matching against a small rule set (typically 10-20 rules). Expected latency: sub-millisecond
- Analytics recording is non-blocking (channel send). Expected impact: zero
- BuildContext() calls `git rev-parse` via subprocess. Expected latency: 5-20ms. This runs on every hook request but is acceptable given the 4-minute timeout budget

### Security Considerations

- Seed deny rules (`.env` writes, `.git` writes, `rm -rf /`) are at priority 1000 and cannot be overridden by user rules at lower priorities
- However, user rules CAN be added at priority 1000+ to override seed deny rules. There is no enforcement preventing this. A malicious or careless user rule could auto-allow destructive operations
- The rules file (`~/.stapler-squad/auto_approve_rules.json`) has standard file permissions. Anyone with filesystem access can modify it
- The web UI allows creating arbitrary rules without confirmation. There is no "are you sure?" dialog for creating an AutoAllow rule for `rm -rf`

---

## File Inventory

### New Files (Backend)

| Path | Lines | Description |
|------|-------|-------------|
| `server/services/classifier.go` | 336 | Core classifier: Rule, RuleBasedClassifier, SeedRules, Classify, BuildContext |
| `server/services/classifier_test.go` | 280 | 15 unit tests for all seed rules and classifier behavior |
| `server/services/rules_store.go` | 297 | RulesStore: JSON persistence, Upsert, Delete, WatchAndReload |
| `server/services/analytics_store.go` | 373 | AnalyticsStore: async JSONL writer, LoadWindow, ComputeSummary |
| `server/services/rules_service.go` | 287 | RulesService: 4 ConnectRPC handlers + mapping helpers |
| `server/services/claude_settings_parser.go` | 140 | Parse Claude settings.json permissions.allow into rules |

### New Files (Frontend)

| Path | Lines | Description |
|------|-------|-------------|
| `web-app/src/lib/hooks/useApprovalRules.ts` | 118 | Hook for rule CRUD via ConnectRPC |
| `web-app/src/lib/hooks/useApprovalAnalytics.ts` | 70 | Hook for analytics summary via ConnectRPC |
| `web-app/src/components/sessions/ApprovalRulesPanel.tsx` | 420 | Rules management panel component |
| `web-app/src/components/sessions/ApprovalRulesPanel.module.css` | -- | CSS modules for rules panel |
| `web-app/src/app/rules/page.tsx` | 14 | Next.js page for /rules route |
| `web-app/src/app/rules/page.module.css` | -- | CSS modules for rules page |

### Modified Files

| Path | Change |
|------|--------|
| `server/services/approval_handler.go` | Added classifier/analyticsStore fields, SetClassifier/SetAnalyticsStore setters, classification logic at top of HandlePermissionRequest |
| `server/services/session_service.go` | Creates RulesStore, AnalyticsStore, Classifier, RulesService; GetClassifier/GetAnalyticsStore accessors; delegates 4 RPCs |
| `server/server.go` | Wires SetClassifier/SetAnalyticsStore into ApprovalHandler |
| `proto/session/v1/types.proto` | AutoDecision enum, ApprovalRuleProto, AnalyticsSummaryProto, ToolStatProto, CommandStatProto, RuleStatProto |
| `proto/session/v1/session.proto` | 4 new RPCs + request/response messages |
| `web-app/src/lib/routes.ts` | Added `rules: "/rules"` |
| `web-app/src/components/layout/Header.tsx` | Added "Rules" nav link |

### Data Files (Runtime)

| Path | Format | Description |
|------|--------|-------------|
| `~/.stapler-squad/auto_approve_rules.json` | JSON | User-defined rules |
| `~/.stapler-squad/approval_analytics.jsonl` | JSONL | Append-only decision log |
