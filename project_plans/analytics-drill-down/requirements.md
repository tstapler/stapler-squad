# Analytics Drill-Down: Requirements

## Project
`analytics-drill-down` — Improve the approval-rules analytics page with proper DB-backed queries and a subcommand detail panel so operators can see exactly which sub-operations are causing escalations before writing a rule.

## Context & Current State
**Data IS already in SQLite** via the `ClassificationAnalytics` ent entity. The pain is in the *query layer* and the *UI*, not data storage per se:

| Layer | Current state | Problem |
|---|---|---|
| DB schema | `ClassificationAnalytics` with `command_program`, `command_subcategory` | No compound index on (program, created_at); no index on subcategory |
| Repository | `ListAnalytics(ctx, limit)` — full table scan, ordered by created_at | No time-windowed filter at DB level; loads ALL rows into Go memory |
| `LoadWindow` | Calls `ListAnalytics(ctx, 0)` then filters in Go | O(n) full load per analytics request |
| UI — uncovered programs | Shows program + count, but no subcommand breakdown | User can't tell which subcommands of `git` or `gh` are causing escalations |
| UI — suggestion cards | "Suggest Rule" generates a rule for the whole program | Rule is too broad because subcommand distribution isn't visible |

## Goals

### G1 — Efficient time-windowed queries
Replace the in-memory filter with proper DB queries. `LoadWindow(since)` must execute `WHERE created_at >= ?` at the SQL level with a covering index.

### G2 — Subcommand breakdown query
Add a DB aggregation query: given a program name and a time window, return each distinct (subcommand, decision) pair with its count. This powers both the detail panel and the existing command-distribution table.

### G3 — Example commands query
For a given program + optional subcommand, return the most recent N raw `command_preview` strings so operators can inspect actual flags and arguments.

### G4 — Rule coverage per subcommand
For a given program + time window, compute which fraction of commands matched an existing rule vs went to manual review, broken down by subcommand.

### G5 — Trend-over-time query
For a given (program, subcommand), return per-day counts for the selected window (7/14/30/90 days).

### G6 — Program detail panel in the UI
Clicking any program row in the "Uncovered Bash Programs" section opens a side-panel (slide-in or inline-expand) showing:
- **Subcommand frequency table**: subcommand, count, % of total, decision breakdown (allow/manual/deny)
- **Example raw commands**: last 5–10 actual command strings
- **Rule coverage summary**: % matched by existing rules vs manual review, with rule names
- **Trend sparklines**: per-subcommand call volume over the selected time window

The panel must also show a "Add rule for this subcommand →" link that pre-populates the rule form with the specific subcommand pattern.

### G7 — Historical JSONL migration (if applicable)
At startup, if JSONL analytics files exist from a legacy period, import them into `ClassificationAnalytics`. *Note: investigation may reveal this is already handled or N/A.*

## Non-Goals
- Aggregation/roll-up tables for performance (data volume is small; raw storage is fine)
- Analytics outside the rules page
- Changing how new decisions are recorded (the write path is already correct)

## Acceptance Criteria

| ID | Criterion |
|---|---|
| AC-1 | `LoadWindow(since)` issues a SQL `WHERE created_at >= ?` query; does NOT load rows outside the window |
| AC-2 | DB schema has index on `(created_at)` and compound index on `(command_program, created_at)` |
| AC-3 | New repository method `ListAnalyticsByProgramSince(ctx, program, since, limit)` exists and is tested |
| AC-4 | New repository method `GetSubcommandBreakdown(ctx, program, since)` exists and returns per-(subcommand, decision) aggregates |
| AC-5 | New repository method `ListRecentCommandsByProgram(ctx, program, subcommand, since, n)` returns last N command previews |
| AC-6 | New `GetSubcommandTrend(ctx, program, subcommand, since)` returns per-day counts |
| AC-7 | New proto RPC `GetProgramAnalytics(program, window_days)` returns SubcommandBreakdown, ExampleCommands, RuleCoverage, DailyTrend |
| AC-8 | Clicking a program row in the "Uncovered Bash Programs" table opens the detail panel |
| AC-9 | Detail panel shows subcommand table with count, %, and decision breakdown columns |
| AC-10 | Detail panel shows last 5 raw command examples |
| AC-11 | Detail panel shows rule coverage (which rules matched vs manual review) |
| AC-12 | Detail panel shows a trend sparkline per subcommand for the selected window |
| AC-13 | "Add rule →" link in the detail panel pre-populates the rule form with the subcommand pattern |
| AC-14 | All existing analytics tests continue to pass |
| AC-15 | `make quick-check` passes with no lint errors |

## Technical Constraints
- Use the existing ent schema migration workflow (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`)
- RPC changes require updating `proto/session/v1/session.proto` and running `make generate-proto`
- UI components must use vanilla-extract CSS (`.css.ts` files)
- CSS tokens must come from `vars` (theme contract), no hardcoded values

## Out of Scope
- Changing the write path / recording side
- Notifications or alerting on gap rate changes
- Multi-instance analytics federation
