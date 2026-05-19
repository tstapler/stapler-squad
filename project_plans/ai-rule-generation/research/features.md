# Feature Research: AI-Assisted Rule Generation

## 1. Manual Rule Creation Form (ApprovalRulesPanel)

**File**: `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

### Form Fields

The existing `RuleFormState` interface captures all fields a rule can have:

| Field | Type | Description |
|---|---|---|
| `name` | string | Human-readable rule label (required) |
| `toolName` | string | Exact tool name match (e.g., `Bash`) |
| `toolPattern` | string | Regex matching tool name (e.g., `Read\|Glob`) |
| `commandPattern` | string | Regex matching full command string (e.g., `^git log`) |
| `filePattern` | string | Regex matching file paths (e.g., `\.md$`) |
| `decision` | AutoDecision enum | `ALLOW`, `DENY`, or `ESCALATE` |
| `reason` | string | Shown to Claude when denied |
| `alternative` | string | Safer command suggestion |
| `priority` | number | 1–999; evaluated descending |
| `enabled` | boolean | Whether the rule is active |

Validation requires: `name` is non-empty AND at least one of `toolName`, `toolPattern`, `commandPattern`, `filePattern` is set.

### Rule Sources

Rules have a `source` field: `"user"` (editable/deletable), `"seed"` (built-in, read-only), `"claude-settings"` (read-only). The form always writes `source: "user"` via `useApprovalRules.upsertRule`.

### UpsertApprovalRule RPC

The existing `upsertApprovalRule` ConnectRPC call (in `useApprovalRules.ts`) is the sole persistence path. The AI rule generation feature MUST route through this same call, satisfying the FR-8 no-auto-save requirement.

### Inline Analytics Summary

`ApprovalRulesPanel` already loads a 7-day `useApprovalAnalytics` summary to show a mini analytics bar at the top. The "Generate Suggestions" button (US-1) can be added adjacent to this bar without requiring new data fetches.

---

## 2. Review Queue UI (ReviewQueuePanel)

**File**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

### Item Structure

Each `ReviewItem` in the queue exposes via `metadata`:
- `pending_approval_id` — identifies this item as an approval-pending item (Approve/Deny buttons shown)
- `tool_input_command` — the raw command string the agent tried to run
- `tool_input_file` — file path for file-based tool calls
- `cwd` — current working directory at time of request
- `orphaned` — whether the approval has expired

### Action Surface for "Create Rule from This"

Approval-pending items already render `✓ Approve` and `✗ Deny` buttons. The "Create Rule from This" action (FR-4) should be added as a third button alongside these two, only visible when `metadata?.["pending_approval_id"]` is present. This button would call `GenerateSuggestedRule` with `source: review_queue_item`, passing the `tool_input_command`, `tool_input_file`, and `tool_name` as context. The resulting `SuggestedRuleCard` should render in a modal (the PR creation modal pattern on lines 731–845 provides a proven template for this overlay pattern).

### PR Creation Modal as a Template

The existing `prModal` pattern in `ReviewQueuePanel` demonstrates exactly the interaction model needed for rule suggestion display:
- User clicks button → modal opens
- Loading state shown while agent runs ("this may take up to 30 seconds")
- Result rendered inline in the modal (editable textarea in the PR case; editable rule fields in the rule case)
- Cancel/Confirm action pair at the bottom

This pattern is the direct prior art for `SuggestedRuleCard` in a modal context.

---

## 3. Analytics Panel Coverage Gap Data (ApprovalAnalyticsPanel)

**File**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

### Coverage Gap Section

The panel already renders a full "Rule Coverage Gaps" section when `summary.coverageGapCount > 0`. It shows:

- **`CoverageGapHeader`**: severity-colored banner (high ≥30%, med ≥10%, low <10%) with gap count, rate, and contextual advice text
- **Uncovered Tools table**: `topUncoveredTools` — tool name, unmatched count, proportional bar, plus an `Add rule →` link (currently a simple `<a href="/rules">`)
- **Uncovered Bash Programs table**: `topUncoveredPrograms` — program name, category badge, unmatched count, bar, plus `Add rule →` link

### Current "Add rule →" Links

Both uncovered tools and uncovered programs rows end with `<a href="/rules" className={addRuleLink}>Add rule →</a>`. These are plain navigation links with no pre-fill context. The FR-5 requirement replaces or augments these with a "Suggest Rule" button that calls `GenerateSuggestedRule` inline.

### Analytics API Fields

`AnalyticsSummaryProto` fields available for AI context assembly (FR-7):

| Field | Description |
|---|---|
| `totalDecisions` | Total classification decisions in window |
| `decisionCounts` | Map of `auto_allow`/`auto_deny`/`escalate`/`manual_allow`/`manual_deny` → count |
| `topTools` | Top tools by request count |
| `topDeniedCommands` | Top denied command previews |
| `topTriggeredRules` | Top rules by trigger count |
| `autoApproveRate` | Float 0–1 |
| `manualReviewRate` | Float 0–1 |
| `coverageGapCount` | Decisions with no rule match |
| `coverageGapRate` | Percentage (0–100) of gaps |
| `topUncoveredTools` | Top tools with no rule match |
| `topUncoveredPrograms` | Top Bash programs with no rule match |
| `commandSubcommandStats` | Full (program, subcommand, category) distribution |
| `topCommandPrograms` | Top Bash programs overall |
| `topPythonImports` | Top Python `-c` import modules |

`windowDays` selector supports 7 / 14 / 30 / 90 day windows.

---

## 4. Analytics & Rules API Hooks

### useApprovalAnalytics (useApprovalAnalytics.ts)

- RPC: `getApprovalAnalytics({ windowDays })`
- Returns: `{ summary: AnalyticsSummaryProto | null, dailyBuckets: DailyBucketProto[], loading, error, refresh }`
- `DailyBucketProto` provides per-day breakdown: `date`, `total`, `autoAllow`, `autoDeny`, `escalate`, `manualAllow`, `manualDeny`

### useApprovalRules (useApprovalRules.ts)

- RPC: `listApprovalRules({ sourceFilter? })`
- Returns: `{ rules: ApprovalRuleProto[], loading, error, upsertRule, deleteRule, refresh }`
- `upsertRule` always writes `source: "user"` — no override possible from the call site
- `deleteRule` does an optimistic client-side removal then calls `deleteApprovalRule`

`ApprovalRuleProto` fields (matches form fields): `id`, `name`, `toolName`, `toolPattern`, `commandPattern`, `filePattern`, `decision`, `riskLevel`, `reason`, `alternative`, `priority`, `enabled`, `source`

---

## 5. Seed Rule Set Structure (pkg/classifier/classifier.go)

**File**: `pkg/classifier/classifier.go` — `SeedRules()` function at line 738

### Priority Tiers

| Priority | Decision | Purpose |
|---|---|---|
| 1000 | AutoDeny | Critical blocks — must fire before all allow rules |
| 500–525 | Escalate/Allow | Targeted overrides that supersede the allow tier |
| 100 | AutoAllow | Standard safe development operations |
| 50 | Escalate | Catch-all escalation for unrecognized patterns |

### Rule Schema (Go `Rule` struct)

Each seed rule populates:
- `ID` — kebab-case string, prefixed `seed-deny-`, `seed-allow-`, or `seed-escalate-`
- `Name` — human-readable description
- `ToolName` — exact tool name (e.g., `"Bash"`) OR
- `ToolPattern` — compiled `*regexp.Regexp` for tool matching
- `CommandPattern` — compiled `*regexp.Regexp` for command matching OR
- `Criteria` — structured `*CommandCriteria` (preferred over regex for simple cases):
  - `Programs []string` — executable names (e.g., `["git"]`)
  - `Subcommands []string` — subcommand names (e.g., `["push"]`)
  - `RequiredFlags []string` — flags that must be present (e.g., `["--force"]`)
- `FilePattern` — compiled `*regexp.Regexp` for file path matching
- `Decision` — `AutoDeny`, `AutoAllow`, or `Escalate`
- `RiskLevel` — `RiskCritical`, `RiskHigh`, `RiskMedium`, `RiskLow`
- `Reason` — explanation shown to Claude (policy rationale)
- `Alternative` — safer command suggestion
- `Priority` — int, evaluated descending
- `Enabled` — bool
- `Source` — always `"seed"`

### CommandCriteria vs CommandPattern

Seed rules use structured `Criteria` where possible (Programs + Subcommands + RequiredFlags), falling back to `CommandPattern` regex only when the match cannot be expressed structurally. `//nolint:commandpattern` lint comments explain when regex is necessary. This preference for structured criteria over regex is directly relevant to the AI agent's pattern generation strategy — it should prefer `Criteria`-based rules and fall back to regex only when needed.

### Representative Rule Examples for AI Context

| Rule ID | Tool | Decision | Criteria/Pattern |
|---|---|---|---|
| `seed-deny-rm-rf-root` | Bash | AutoDeny | regex: `rm\s+(-rf)\s+(/\|~)` |
| `seed-deny-git-reset-hard` | Bash | AutoDeny | Criteria: git reset --hard |
| `seed-deny-git-push-force` | Bash | AutoDeny | Criteria: git push --force/-f |
| `seed-allow-gh-api-rest-jq` | Bash | AutoAllow | regex: `\bgh\s+api\b.*--jq` |
| `seed-escalate-gh-api-explicit-write` | Bash | Escalate | regex: `\bgh\s+api\b.*(-X POST\|PUT\|DELETE\|PATCH)` |
| `seed-deny-env-write` | Write/Edit/MultiEdit | AutoDeny | FilePattern: `(^/)\.env(\.\|$)` |

---

## Key Integration Points for Implementation

1. **SuggestedRuleCard acceptance path**: calls `upsertRule` from `useApprovalRules` with `source: "user"` — no new persistence RPC needed.

2. **Analytics panel gap items**: the "Add rule →" links at lines 364 and 399 in `ApprovalAnalyticsPanel.tsx` are the exact insertion points for FR-5 "Suggest Rule" buttons; each row has `toolName` / `programName` + `count` available as context parameters.

3. **Review queue insertion point**: `itemActions` div (line 630 in `ReviewQueuePanel.tsx`) rendered per item — the "Create Rule from This" button (FR-4) attaches here, conditionally on `metadata?.["pending_approval_id"]` being set.

4. **Agent context availability**: the backend handler for `GenerateSuggestedRule` can assemble all required context (FR-7) from three existing RPCs: `ListApprovalRules`, `GetAnalyticsSummary`, and the embedded `SeedRules()` function — no new data sources needed.
