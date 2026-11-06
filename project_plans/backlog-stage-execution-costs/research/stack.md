# Stack Research: backlog-stage-execution-costs

## 1. What already exists (build on this, don't re-invent)

The codebase is closer to this feature than the requirements doc implies. Three pieces are already in place:

- **`SessionRole` is already a first-class dimension.** `session/ent/schema/item_session.go:25` has
  `field.String("session_role")` ("work, triage, review"), and `estimated_cost_usd` (`item_session.go:80-83`)
  is already populated for headless sessions. `server/services/insights_service.go:86-104`
  (`sessionRolesForSessions`) already joins `ItemSession.session_role` onto `SessionTokenSummary.session_role`
  (proto field 22, `proto/session/v1/insights.proto:112-116`, added under ADR-029). **The new stage/role cost
  chart is a `GROUP BY session_role` aggregation over data that's already flowing through the pipeline** — it
  does not require new instrumentation to capture the role, only a new aggregation + proto message + chart.
- **A breakdown-by-category proto/chart pattern to clone already exists twice.** `ModelBreakdown`
  (`insights.proto:154-165`) → `ModelBreakdownChart.tsx` and `ActivityCostBreakdown`
  (`insights.proto:169-176`, built in `insights_service.go:227-478`) → follow this shape exactly for a new
  `StageRoleCostBreakdown` message (role, estimated_cost_usd, session_count, item breakdown) and
  `StageCostChart.tsx`. `ModelBreakdownChart.tsx` is the concrete template: `recharts` `BarChart` +
  `ResponsiveContainer` + `Cell`-per-bar palette + a legend row, styled via `ModelBreakdownChart.css.ts`
  (vanilla-extract). Drill-by-item-name is new (neither existing chart filters by backlog item), but the data
  needed for it (backlog item name) is already reachable — `insightsBacklogReader.GetAllItemSessionsWithBacklogInfo`
  joins `ItemSession` to its `backlog_item` edge.
- **`headless.CallOptions.Model` already threads a per-call model override** (`session/headless/caller.go:29-30`,
  wired through `CallWithOptions`/`CallBlocking`, `caller.go:653-749`). Per-stage model override for
  triage/review is **already fully plumbed at the pool layer** — the only gap is that
  `server/services/backlog_service_triage.go` and the review call site don't yet read a per-stage override
  out of `PipelineMode` and pass it into `opts.Model`.

## 2. Schema changes needed

### `PipelineMode` (`session/ent/schema/pipeline_mode.go`)

Add per-stage program+model override fields. Given the existing `*_prompt_template`/`*_command_template`
naming convention (one field per stage concept, all `field.String(...).Optional()`), the natural extension is:

```go
field.String("triage_program").Optional().Comment("Program override for headless triage under this pipeline mode; empty = pool default."),
field.String("triage_model").Optional().Comment("Model override for headless triage under this pipeline mode; empty = pool default."),
field.String("review_program").Optional().Comment("Program override for headless review under this pipeline mode; empty = pool default."),
field.String("review_model").Optional().Comment("Model override for headless review under this pipeline mode; empty = pool default."),
field.String("work_program").Optional().Comment("Program override for the interactive work stage under this pipeline mode; empty = Instance.Program's own default."),
field.String("work_model").Optional().Comment("Model override for the interactive work stage under this pipeline mode (passed as a CLI flag on Instance.Program, e.g. \"claude --model sonnet\"); empty = program default."),
```

Six new columns (2 per stage × 3 stages), all optional strings — consistent with how every other
per-stage template field in this schema is modeled (no enum, no nested message; ent has no first-class
`repeated`/nested-struct field type without a JSON serde field, and 3 stages × 2 knobs is small enough
that flat columns beat that complexity). Regenerate per the project's ent rule:
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (verified against
`session/ent/generate.go` — **do not** use the plain `ent generate` form, and do not commit generated output;
`session/ent/*.go` outside `schema/` is gitignored).

Mirror the 6 new fields into `PipelineMode` proto (`proto/session/v1/backlog.proto:274-297`, fields 18-23),
and into `content_hash`'s derivation if program/model overrides should participate in "what ran" drift
detection (`pipeline_mode.go:47-49`'s existing hash only covers the 9 content-template fields — a decision
for the plan phase, not this research: does a program/model change count as "content drift" for a
running session's snapshot?).

### `ItemSession` — no new fields needed for cost tracking

`session_role` and `estimated_cost_usd` already exist. If per-item/per-stage **budget thresholds**
(soft warning) need persistent config, that's a new concept — likely a small new field on `BacklogItem`
(per-item threshold) and a new global/per-role config value (`config/config.go`'s JSON config, alongside
existing rollout-flag-style settings) rather than an ent schema change, since thresholds are configuration,
not execution history.

## 3. Backend: threading the override through to execution

- **Headless (triage/review):** `server/services/backlog_service_triage.go` (triage) and the review call
  site build `headless.CallOptions{...}` today with no `Model` set (falls back to `PoolConfig.DefaultModel`).
  Resolve `PipelineMode.triage_model`/`review_model` (falling back to `""` = pool default) and set
  `opts.Model` before calling `CallWithOptions`/`CallBlocking` — this is a straight parameter thread, no new
  library. `opts.WorkDir` already forces a fresh one-shot pool per call (`caller.go:663-693`) which is where
  a **program override** (not just model) would need to plug in if triage/review ever run a non-Claude
  program — today `NewPool`/`CallWithOptions` are hardcoded to the `claude` binary
  (`findClaudeBinary`/`ProcessRunner{claudeBin: bin}`, `caller.go:112-124`), so a per-stage *program* override
  for headless calls is a bigger change than a per-stage *model* override: it needs a
  `headless.Pool`-per-program abstraction (or a new `ClaudeRunner`-like interface implemented per adapter —
  see §4) before `PipelineMode.triage_program` can mean anything beyond "claude".
- **Interactive work stage:** `Instance.SwitchProgram(ctx, rawProgram, persist)` (`session/instance_program.go:59`)
  already exists and is the correct integration point — resolve `PipelineMode.work_program`/`work_model` into
  a single `rawProgram` string (e.g. `"claude --model sonnet"`, matching the free-form `Instance.Program`
  convention noted in the requirements baseline) and call `SwitchProgram` when spawning/transitioning a
  session into the work stage under a mode that specifies an override.

## 4. Feasibility: headless adapters for Aider and Gemini CLI

Both were checked against their official docs (2026-09-17); **neither offers a stdout JSON contract as
clean as `claude -p --output-format stream-json`, but both have a machine-parseable path**, at different
cost/effort:

### Gemini CLI — feasible, tokens yes, cost computation is DIY

Per [gemini-cli headless mode docs](https://google-gemini.github.io/gemini-cli/docs/cli/headless.html):
`gemini -p "<prompt>" --output-format json` returns a single JSON object:

```json
{
  "response": "...",
  "stats": {
    "models": {
      "gemini-2.5-pro": {
        "api": { "totalRequests": 2, "totalErrors": 0, "totalLatencyMs": 5053 },
        "tokens": { "prompt": 24939, "candidates": 20, "total": 25113, "cached": 21263, "thoughts": 154, "tool": 0 }
      }
    },
    "tools": { "totalCalls": 1, "totalSuccess": 1, "totalFail": 0, ... },
    "files": { "totalLinesAdded": 0, "totalLinesRemoved": 0 }
  },
  "error": { "type": "string", "message": "string", "code": "number" }
}
```

This is a single blocking JSON object (not stream-json line-by-line like Claude's first call), which
actually simplifies the adapter — no line-scanner/idle-timeout machinery needed, just
`exec.Command` + `json.Unmarshal` on the whole stdout. **Gap: there is no `cost_usd`/`total_cost_usd`
field at all** — only token counts per model (`prompt`, `candidates`, `cached`, `thoughts`, `tool`,
`total`). `session/tokens/pricing.go` is Claude-only today (`DefaultPricingTable()`'s map is all
`claude-*` keys). A Gemini adapter needs its **own pricing table entries** (Gemini 2.5 Pro/Flash per-MTok
rates from ai.google.dev's pricing page) and a `NormalizeModelFamily`-equivalent for Gemini model IDs
before `EstimateCost`-style arithmetic can produce a dollar figure — this is new work, not a gap in the
existing `tokens` package's design (it's already family-keyed and pluggable via `LoadPricingOverride`).

### Aider — feasible only via a side-channel log file, not stdout

Per [aider scripting docs](https://aider.chat/docs/scripting.html): `aider --message "<prompt>" <files>`
runs one instruction non-interactively and exits — no `--output-format json` flag exists for the main
response at all (confirmed: the scripting page's flag list has no JSON option, only `--message`,
`--message-file`, `--yes`, `--auto-commits`, `--dry-run`). Cost/token data instead comes from a **separate
opt-in analytics stream**: [`aider --analytics-log filename.jsonl --no-analytics`](https://aider.chat/docs/more/analytics.html)
writes a local JSONL file (the `--no-analytics` flag suppresses PostHog reporting/the opt-in prompt while
still writing the local log) whose `message_send` events carry `prompt_tokens`, `completion_tokens`,
`total_tokens`, `cost`, and `total_cost` per LLM call (verified against
[aider's own sample-analytics.jsonl](https://github.com/aider-ai/aider/blob/main/aider/website/assets/sample-analytics.jsonl)
schema). An Aider adapter therefore looks structurally different from the Claude/Gemini adapters: run the
subprocess with a **per-call temp file** passed to `--analytics-log`, wait for exit, then read+parse that
file for the `message_send` event(s) rather than parsing stdout — a "tail a sidecar file" pattern, not a
"parse stdout" pattern. This is buildable but is meaningfully more adapter-specific plumbing than Claude/Gemini
(temp file lifecycle, event-type filtering, no forced 1:1 between "one aider invocation" and "one JSONL line"
since a single message can trigger multiple LLM round-trips/`message_send` events that should be summed).

### Recommendation for scope

Both are technically feasible for a headless adapter with a parseable cost, matching the requirements doc's
"investigate which programs can realistically run headless with a parseable cost" ask. Gemini is the
lower-effort of the two (single JSON blob, no sidecar file) but needs a new pricing table. Aider is feasible
but its cost data requires a sidecar-file protocol distinct from every existing adapter's stdout-parsing
shape — budget it as the larger of the two adapters, or explicitly descope it to "fall back to Claude
headless" per the requirements doc's stated out-of-scope allowance ("Building headless support for programs
with no scriptable non-interactive mode" is out of scope, but Aider *does* have one — the descope decision
is about effort/priority, not technical infeasibility).

## 5. Program/model detection — no new library needed

`config.GetAvailablePrograms()` (`config/config.go:1275-1307`) already shells out to `which` for
`proxy-claude`, `claude`, `claude-code`, `gemini`, `agy` — **`aider` is missing from this candidate list**
and should be added alongside the pipeline-mode UI work so the settings page's program dropdown can offer
it. `session/detection/binaries/aider.go` and `gemini.go` already exist as `dtypes.BinaryDetector`
implementations for tmux-output pattern matching (status detection for the interactive TUI case) — these
are unrelated to headless execution and need no changes for this feature; they're evidence the programs are
already integrated as first-class `Instance.Program` values, just not as headless-callable ones.

## 6. Frontend stack

No new dependencies. Confirmed current pinned versions in `web-app/package.json`:

| Package | Version |
|---|---|
| `recharts` | `^3.8.1` |
| `@vanilla-extract/css` | `^1.20.1` |
| `@vanilla-extract/recipes` | `^0.5.7` |
| `next` | `15.3.2` |
| `react` | `^19.0.0` |

The new stage/role cost chart and the pipeline-mode program/model form fields are both straightforward
extensions of existing patterns (`ModelBreakdownChart.tsx`'s `BarChart`, `PipelineModeForm.tsx`'s existing
form-field styling) — no new charting, form, or state-management library is warranted for this feature's
scope.

## 7. Backend/protocol versions (for context, unchanged by this feature)

| Module | Version |
|---|---|
| Go | 1.26.6 (`go.mod`) |
| `entgo.io/ent` | v0.14.5 |
| `connectrpc.com/connect` | v1.20.0 |
| `google.golang.org/protobuf` | v1.36.12 |

No version bumps are needed for this feature — it's additive schema fields, additive proto messages/fields,
and additive RPC surface on the existing `InsightsService`/pipeline-mode CRUD services.

## 8. Open questions for the planning phase

- Does a per-stage program/model override participate in `PipelineMode.content_hash` drift detection, or is
  it tracked separately (since it changes *execution*, not *prompt content*)?
- Where does the soft-budget-threshold config live — global config (`config/config.go`), a new
  `BacklogItem` field, or a new small ent entity keyed by (scope, role)? No existing precedent in this repo
  for a "threshold + warning" concept to mirror.
- Should the Aider headless adapter be built in this appetite window given its sidecar-file cost-parsing
  cost, or explicitly deferred (ship Gemini + Claude adapters, fall back to Claude for Aider-configured
  stages) — a scope call, not a technical blocker.
