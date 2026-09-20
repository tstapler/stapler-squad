# Requirements: backlog-stage-execution-costs

**Date**: 2026-09-17
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
Backlog items run through pipeline stages (triage, review, work/implementation, and others defined by a `PipelineMode`), but every stage of an item is forced to run with the same program and model. There is no way to deliberately run a cheap/fast stage (e.g. triage) on a cheaper model or a different program than an expensive stage (e.g. implementation). Separately, spend on triage and review stages is not visible as its own slice of cost — the existing Insights dashboard breaks total spend down by model family and by day, but not by pipeline stage/role or by backlog item, so there is no way to see "how much did triage cost this week" or "which backlog item is expensive."

**Known, accepted limitation (not a gap to keep re-flagging):** there is no quantified cost-pain baseline (e.g. "triage currently costs $X/week") backing this Problem Statement — the existing dashboard's lack of a stage/role breakdown (described above) is exactly why no such number can be produced today. Capturing one is intentionally deferred to the operator's own manual "before" snapshot at first-adoption time (see plan.md's Unresolved Questions), not to a business-case step that could run before this feature ships. This is inherent to a pre-launch business case for a single-user tool with no separate stakeholder to demand the number in advance, not an oversight in this document.

## Baseline
Today, `Instance.Program` (a free-form string like `"claude"`, `"claude --model sonnet"`, `"aider"`) is set once per session/instance, and headless triage/review calls (`session/headless`) always use the pool's single hardcoded `DefaultModel` — there is no per-stage override. All per-session costs (`ItemSession.Cost`, populated from `headless.CallResult.CostUSD` or transcript-derived pricing) already roll up into a global total and into `/insights` charts (`DailySpendChart`, `ModelBreakdownChart`), but those charts group only by day or by model family — not by stage/role or by backlog item — so a user wanting to see triage/review's share of spend, or a given item's total cost, has no dedicated view today.

## Users / Consumers
The operator running stapler-squad (single-user tool) — the person configuring pipeline modes and watching spend in `/insights` and `/settings/pipeline-modes`.

## Success Metrics
- A user can configure, per pipeline stage (triage/review/work/etc.), which program and model runs that stage, and see it take effect on the next run of that stage — verified not just by inspecting the saved config but by comparing that item's triage-stage cost (via the new per-stage cost view) from before the override to after it, on a real pipeline mode actually in use.
- Triage-stage cost per item measurably drops after configuring a cheap model on a real pipeline mode, verified via the new `StageCostChart`/per-item cost breakdown against a captured pre-change baseline (see plan.md's Unresolved Questions for how that baseline is captured) — not merely "the chart renders a number."
- The Insights dashboard (or a new view within it) shows a bar chart of cost broken down by stage/role, filterable/drillable by backlog item name, with totals that sum to the existing global cost total — so a user trusts the drilldown numbers enough to act on them (e.g. reconfigure a stage's model) without manually cross-checking against the total, not merely "a chart renders."
- A soft budget warning fires (visibly, not silently) when a configured spend threshold is crossed for a stage or item — so an operator notices and can decide whether to intervene without proactively checking Insights or grepping logs for it.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- Single-user local tool — no multi-tenant concerns.
- Must not break existing `PipelineMode`s that don't set a per-stage program/model override (default behavior must be unchanged for items using today's default mode).
- `server/services` may not import `session/ent` directly (existing depguard rule `no_ent_in_services`) — any new PipelineMode/cost fields need a DTO on the repository boundary, matching the existing pattern.

## Non-functional Requirements
- **Performance SLO**: not specified — this is config/reporting, not a hot path.
- **Scalability**: not applicable (single-user, bounded backlog item/session volume).
- **Security classification**: internal.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Extend `PipelineMode` so each stage template (triage, review, work/initial, and others it already defines) can carry its own program + model override, alongside its existing prompt/command template.
- Thread that per-stage program/model override through to actual execution:
  - Headless calls (triage, review) via `headless.CallOptions.Model` (already exists; just unwired) plus a model/program-appropriate equivalent for non-Claude programs.
  - Interactive work-stage sessions via the existing `Instance.Program`/`SwitchProgram` mechanism, driven by the pipeline mode's configured value instead of only a manual per-session switch.
- Investigate (in research phase) which currently-supported programs (`claude`, `aider`, `gemini`, others detected via `config.GetAvailablePrograms()`) can realistically run headless with a parseable cost, since only `claude -p` has a headless path today. Build headless adapters for the ones that are feasible within the appetite; for ones that aren't (e.g. no non-interactive/scriptable mode), fall back to Claude headless and record that as a known limitation rather than silently no-op.
- Extend the `/settings/pipeline-modes` UI to configure program + model per stage template.
- Extend cost tracking/aggregation so `SessionRole` (triage/review/work) is a first-class breakdown dimension, matching the existing `ActivityCostBreakdown`-by-`ActivityType` pattern.
- New/extended Insights view: a bar chart with stage/role as the primary grouping, with the ability to filter or drill down to a single backlog item's cost breakdown by stage. Numbers here must be consistent with (a subset that sums correctly into) the existing global total shown elsewhere in Insights.
- Soft budget warning: a configurable spend threshold (per stage and/or per item — exact granularity is a planning decision) that surfaces a visible warning (not a hard block) when crossed.

### Out of Scope
- Hard budget enforcement (blocking a stage from running because it would exceed a budget) — explicitly deferred; this project is visibility + soft warnings only.
- Multi-tenant or per-user cost tracking/permissions.
- Building headless support for a program that has no scriptable non-interactive mode at all (document as a limitation instead of forcing it).
- Changing how the interactive `work` stage's tmux session behaves beyond selecting which program/model starts it.

## Rabbit Holes
- **Headless adapters for non-Claude programs.** `claude -p --output-format stream-json` gives structured output and a parsed `total_cost_usd`; it's unconfirmed whether Aider, Gemini CLI, etc. have an equivalent non-interactive, machine-parseable mode with cost reporting. This is the single biggest unknown driving whether "any installed CLI per stage" is achievable within appetite — resolve early in research, not late in implementation.
- **Cost normalization across programs/providers.** The existing `session/tokens/pricing.go` table is Claude-model-specific. A different program (or a free/local model) may report tokens/cost differently or not at all — decide how "free" or unpriced runs are represented in the cost breakdown (e.g. the existing `unpricedModels`/`pricingUnavailable` pattern may need to extend to non-Claude programs).
- **PipelineMode schema growth.** PipelineMode already has 9 template fields; adding a program+model pair per stage roughly doubles its field count. Watch for this tipping into primitive obsession — a per-stage struct/sub-message may be cleaner than N more flat fields (a design call for Phase 3 planning, not decided here).

## Alternatives Considered
- A separate, orthogonal "stage defaults" settings page (independent of `PipelineMode`) was considered and rejected by the user in favor of extending `PipelineMode` directly, keeping stage prompt and stage execution config in one place.
- Hard budget enforcement was considered and deferred as out of scope in favor of soft warnings, to avoid this project also having to design safe-failure behavior for a blocked pipeline stage.

## Feasibility Risks
- Non-Claude headless execution may not be feasible for all currently-supported programs within the Large appetite (see Rabbit Holes) — the plan should include a go/no-go checkpoint after research, with Claude-model-only per-stage override as the fallback scope if cross-program headless proves infeasible.
- `server/services`'s `no_ent_in_services` depguard rule means any new PipelineMode fields need corresponding DTO/repository changes in multiple places (schema → repository → service → proto) — straightforward but easy to under-scope in planning.

## Observability Requirements
- Log (structured, per existing `slog` conventions) when a stage runs with a non-default program/model override, and when a headless call falls back from a configured non-Claude program to Claude (mirrors the existing "unresolved PipelineMode slug falls back to default, logged as Warning" pattern in `session/pipeline_engine.go`).
- Log/emit when a soft budget warning threshold is crossed, including which item/stage and the threshold vs. actual spend.
- No new oncall alerting — this is a single-user local tool; visibility via logs and the Insights UI is sufficient.

## Risk Control
- Backward compatible by construction: pipeline modes with no per-stage override configured keep today's behavior (single default model/program for all stages) — no migration of existing data required beyond additive schema fields.
- New non-Claude headless code paths should fail closed: if a configured program/model can't execute headlessly, fall back to the existing Claude headless path and log a warning, rather than failing the stage outright.
- No feature flag needed — this is an additive, opt-in configuration surface; existing items are unaffected until a user explicitly sets a per-stage override.

## Open Questions
- Exact granularity of the soft budget warning (per-stage threshold, per-item total threshold, or both) — resolve in Phase 3 planning.
- Whether the per-stage config on `PipelineMode` should be modeled as N flat fields (matching today's style) or a repeated sub-message per stage — resolve in Phase 3 planning (flagged under Rabbit Holes).
- Which non-Claude programs are worth building headless adapters for, based on Phase 2 research into their non-interactive/scriptable capabilities.
