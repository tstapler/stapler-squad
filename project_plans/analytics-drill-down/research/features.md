# Agent 2 — Feature Completeness Research

## Current Frontend State

### ApprovalAnalyticsPanel.tsx
File: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` (626 lines)

Sections currently rendered:
1. Window selector (7/14/30/90 days)
2. Summary cards (total, auto-allow %, auto-deny %, manual %, avg/day)
3. Daily breakdown table (date, total, allow, deny, manual, volume bar)
4. Top Tools table
5. Top Triggered Rules table
6. Top Bash Programs table (program, category, calls, share bar)
7. Top Python Imports table
8. Command Distribution table (program + subcommand filter, add-rule link)
9. Rule Coverage Gaps section (uncovered tools + programs with "Suggest Rule" inline)

**Missing (drill-down goals G2–G6):**
- No program detail panel / drawer
- No subcommand breakdown per program (which subcommands of "git" are escalated?)
- No example commands panel (raw `command_preview` strings)
- No per-subcommand coverage (does a rule cover "git push" but not "git rebase"?)
- No sparkline / trend-over-time per subcommand
- "Add rule →" link in Command Distribution table already pre-fills program+subcommand
  (`/rules?program=X&subcommand=Y`) — good skeleton for AC-13

### CommandDistributionTable (inline component, lines 533–599)
Uses `SubcommandStatProto[]` from `AnalyticsSummaryProto.command_subcommand_stats`.
Has filter input but no click-to-drill-down. This is where AC-8 expansion row/panel hooks.

### useApprovalAnalytics hook
File: `web-app/src/lib/hooks/useApprovalAnalytics.ts`

- Calls `SessionService.getApprovalAnalytics({ windowDays })`
- Returns `AnalyticsSummaryProto | null` and `DailyBucketProto[]`
- No `GetProgramAnalytics` call yet

**New hook needed**: `useProgramAnalytics(program, windowDays)` that calls
`GetProgramAnalytics` RPC and returns subcommand breakdown, examples, trend data.

## Proto Types (existing — relevant to drill-down)

From `proto/session/v1/types.proto`:

```protobuf
message AnalyticsSummaryProto {
  repeated SubcommandStatProto command_subcommand_stats = 16; // already populated
}

message SubcommandStatProto {
  string program_name = 1;
  string subcommand = 2;
  string category = 3;
  int32 count = 4;
  // MISSING: per-decision breakdown (allow/deny/escalate counts per subcommand)
}

message ProgramStatProto {
  string program_name = 1;
  string category = 2;
  int32 count = 3;
  // MISSING: per-decision breakdown
}
```

## New Proto Types Required

### New request message
```protobuf
message GetProgramAnalyticsRequest {
  string program = 1;
  optional int32 window_days = 2;  // default 7
}
```

### New response message
```protobuf
message GetProgramAnalyticsResponse {
  string program = 1;
  string category = 2;
  repeated SubcommandBreakdownProto subcommands = 3;
  repeated string recent_examples = 4;          // raw command_preview strings
  repeated DailyBucketProto trend = 5;          // per-day totals for this program
}

message SubcommandBreakdownProto {
  string subcommand = 1;
  int32 total = 2;
  int32 auto_allow = 3;
  int32 auto_deny = 4;
  int32 escalate = 5;
  bool has_rule_coverage = 6;    // true if any rule covers (program, subcommand)
  string suggested_rule_hint = 7; // optional pre-fill hint
}
```

(Note: `DailyBucketProto` already exists in types.proto — reuse it for trend.)

## New RPC

```protobuf
// GetProgramAnalytics returns drill-down analytics for a single command program.
rpc GetProgramAnalytics(GetProgramAnalyticsRequest) returns (GetProgramAnalyticsResponse);
```

Lives alongside `GetApprovalAnalytics` in `SessionService`.

## UI Component Plan

New component: `ProgramDetailPanel.tsx` (or expandable row in `CommandDistributionTable`)

Triggered by clicking a program row in the Command Distribution table or Uncovered Bash
Programs table. Renders:
- AC-8: Subcommand table (`SubcommandBreakdownProto[]`) with allow/deny/escalate columns
- AC-9: Examples section (collapsible list of `recent_examples`)
- AC-10: Coverage column in subcommand table (green tick / red X via `has_rule_coverage`)
- AC-11: Sparklines (per-day trend bars using `trend` field from response)
- AC-12: Inline "Suggest Rule" button (reuse existing `useGenerateRule` hook)
- AC-13: "Add rule manually →" link pre-filled with program+subcommand (already in
  `CommandDistributionTable`'s add-rule link pattern)

## RulesService handler location

`GetApprovalAnalytics` is delegated:
- `session_service.go` line 2075: `SessionService.GetApprovalAnalytics` → `s.rulesSvc.GetApprovalAnalytics`
- `rules_service.go` line 138: actual implementation

New `GetProgramAnalytics` follows the same delegation pattern:
1. Add to `session.proto` service block
2. Implement in `rules_service.go`
3. Delegate in `session_service.go`

## Data flow gap summary

| Requirement | Frontend need | Backend need |
|---|---|---|
| AC-7 | `useProgramAnalytics` hook | `GetProgramAnalytics` RPC |
| AC-8 | Subcommand table in detail panel | `SubcommandBreakdownProto` in response |
| AC-9 | Examples list | `recent_examples []string` in response |
| AC-10 | Coverage column | `has_rule_coverage` bool in `SubcommandBreakdownProto` |
| AC-11 | Sparkline bars | `trend []DailyBucketProto` in response |
| AC-12 | Suggest Rule button (reuse existing) | No new backend needed |
| AC-13 | Pre-fill link | No new backend (pattern already in place) |
