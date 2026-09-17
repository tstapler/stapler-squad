# Implementation Plan: backlog-stage-execution-costs

**Feature**: Per-pipeline-stage program/model overrides (triage/review/work) plus a stage- and item-level cost breakdown and soft budget warning in Insights.
**Date**: 2026-09-17
**Status**: Ready for implementation
**ADRs**:
- `decisions/ADR-001-pipeline-mode-stage-executor-storage.md` — JSON-string column reconciling stack.md vs architecture.md
- `decisions/ADR-002-aider-excluded-from-headless-cost-accounting.md` — Gemini adapter shipped, Aider headless excluded
- `decisions/ADR-003-soft-budget-warning-inline-not-batched.md` — inline threshold check, not ADR-029's batched snapshot, not a `CapacityMonitor` fork

---

## Step 0.5 — Creative Pass (Alternatives Explored)

**Approach A — Flat fields (stack.md's original proposal).** 6 new flat
`string` columns on `PipelineMode` (`triage_program`, `triage_model`, etc.).
*Strength*: zero new marshaling code, most mechanically similar to the 9
existing content-template fields. *Weakness*: doubles down on the exact
primitive-obsession pattern the requirements doc's own Rabbit Hole flags —
15 total flat fields with two semantically distinct families
indistinguishable by name pattern alone, and every future stage/knob
addition means another schema migration plus another proto field. Rejected.

**Approach B — Full per-stage refactor (re-model all 9 content fields +
new executor fields as one unified `map[StageRole]PipelineStage{...}`
struct).** *Strength*: cleanest possible end-state, one stage concept for
both content and execution config. *Weakness*: touches already-shipped,
already-tested code (`ComputeContentHash`, `pipelineModeCache`, the 9-field
UI form) for no functional gain this project needs, bundling an unrelated
refactor into a feature PR and multiplying review surface and regression
risk for zero new capability. Rejected — deferred as an independently
justifiable future decision (see Tech Debt Disposition).

**Approach C — JSON-string sub-structure for new fields only (chosen).**
One new `stage_executors_json` string column, storing
`map[StageRole]PipelineStageExecutor` at the domain-model level, following
the exact `ac_snapshot`/`triage_result` JSON-in-string-column convention
already used twice in this schema family (confirmed by direct read of
`session/ent/schema/item_session.go`). *Strength*: additive-only (existing 9
fields untouched, zero risk to shipped behavior), self-describing at the Go
level (`stageExecutors[StageRoleTriage].Model`), and resolves the stack.md
vs. architecture.md disagreement without picking a side — it satisfies both
constraints simultaneously (see ADR-001). *Weakness*: a JSON-in-string
column is not queryable via SQL `WHERE` clauses the way a flat column would
be — acceptable here since nothing needs to filter/index on a stage's
program/model value (this repo already accepts this tradeoff for
`ac_snapshot`/`triage_result`, both also never queried directly).

**Decision: Approach C.** Recorded in the Pattern Decisions table below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `PipelineMode` | Existing DB-persisted entity holding per-mode content templates (9 fields) and, after this project, per-stage executor overrides, for a backlog item's triage/review/work pipeline. | `session/ent/schema/pipeline_mode.go` |
| `StageRole` | New Go `type StageRole string` newtype with 3 exported consts (`StageRoleTriage`, `StageRoleReview`, `StageRoleWork`) — the closed vocabulary a stage/role can take. Mirrors `ItemSession.session_role`'s existing 3-value convention as a type, not a bare string. | New, `session/pipeline_mode_executor.go` |
| `PipelineStageExecutor` | New domain struct `{Program string; Model string}` — one stage's program+model override. Empty `Program`/`Model` means "inherit the pool/session default." | New, `session/pipeline_mode_executor.go` |
| `StageExecutors` | A `map[StageRole]PipelineStageExecutor` — the full per-mode override set, JSON-serialized into `PipelineMode.stage_executors_json`. | New |
| `ExecutorFor` | New 6th method on `PipelineEngine`: `ExecutorFor(item *BacklogItemData, role StageRole) (program, model string)` — resolves a stage's configured executor, following the identical 3-branch fallback shape (`PipelineModeDefault` → defaults; unresolved slug → Warn + defaults; resolved but role absent → defaults) every existing `CachingPipelineEngine` method already uses. | New, `session/pipeline_engine.go` |
| `session.ResolveExecutorProgram` | New shared helper extracted from `server/workflows/scheduler.go`'s `FireNow` inline logic — resolves a model/family-alias string plus a base program into a concrete `Instance.Program`-consumable string (e.g. `"claude --model sonnet"`). Used by both `FireNow` (workflows) and the new work-stage spawn path (backlog items) so the two can't drift. | New, `session/executor_program.go` |
| `headless.PoolClient` | Existing narrow interface (`CallBlocking(ctx, key, systemPrompt, userPrompt, opts, sink) (string, error)`), already satisfied by `*headless.Pool` (Claude). The seam this project uses for per-program headless dispatch via the Strategy pattern. | `session/headless/client.go:7-9` |
| `GeminiCaller` | New `PoolClient` implementation shelling out to `gemini -p "<prompt>" --output-format json`, parsing `stats.models[*].tokens`, and computing cost via a new pricing-table entry. No session/history reuse in v1 (fresh process per call). Bounds its own concurrent subprocess count via a dedicated semaphore (sized to match `Pool`'s existing `MaxConcurrentSessions: 5`, `server/dependencies.go:741`) — it is not built on `headless.Pool`, so it inherits none of that pool's semaphore. | New, `session/headless/gemini_caller.go` |
| `headlessCallerRegistry` | New `map[string]headless.PoolClient` (keyed by resolved program string: `"claude"`, `"gemini"`) wired in `server/dependencies.go`, consulted by `resolveHeadlessCaller` to pick which `PoolClient` a given stage's headless call uses. An unrecognized program (including `"aider"`, per ADR-002), or a resolved program whose binary is no longer detected at call time (Story 2.3.1's re-probe), falls back to the `"claude"` entry with a Warn log **and** a persisted, UI-visible fallback marker on the resulting `ItemSession` row (`configured_program`/`executor_fallback_reason`, Story 2.1.2) — never only a log line. | New |
| `priced` | Bool signal (mirrors `unpriced`/`pricingUnavailable`'s existing "abstain rather than guess" convention), threaded end-to-end through `headless.PoolClient.CallBlocking`'s `CostSink` (Epic 2.5) → `ItemSession.cost_priced` → `RoleCostBreakdown`/`ItemRoleCost`'s per-bucket unpriced counts (Epic 4.1) → `StageCostChart`'s unpriced indicator (Epic 5.2) — an adapter that can't produce a trustworthy dollar cost is visible as "unpriced" at every layer that reads cost, never a misleading `$0.00`. | New; originates in `GeminiCaller` (Epic 3.1), carried by `CostSink` (Epic 2.5) |
| `executor_snapshot_hash` | New `ItemSession` field, SHA-256 (hex, truncated 16 chars) of the resolved `(program, model)` pair for the stage this session ran, captured once at session start — independent of `pipeline_mode_snapshot_hash` (which covers only the 9 content-template fields). Lets a "what ran" UI detect a `PipelineMode`'s executor config having changed since a given session started. | New, `session/ent/schema/item_session.go` |
| `RoleCostBreakdown` | New proto message aggregating `estimated_cost_usd`/`session_count` per `SessionRole`, structurally identical to the existing `ActivityCostBreakdown`. | New, `proto/session/v1/insights.proto` |
| `ItemRoleCost` | New proto message: one backlog item's cost for one role, used for the "drillable by item name" requirement. | New, `proto/session/v1/insights.proto` |
| `sessionMetaForSessions` | Renamed/extended sibling of the existing `sessionRolesForSessions` (`server/services/insights_service.go:86`) that also retains `ItemID`/`ItemTitle` from `ItemSessionBacklogEntry` (currently discarded) instead of only `SessionRole`. | Modified |
| `CostBudgetThresholdUsd` | New optional per-item field (`*float64`) on `BacklogItemData`/`BacklogItemUpdateInput`, mirroring `ReworkCapOverride`'s existing single-pointer-presence convention. Nil = no threshold configured for this item (no warning ever fires). | New |
| `EvaluateBudgetThreshold` | New pure function comparing an item's accumulated cost against `CostBudgetThresholdUsd`, invoked inline at cost-recording time (see ADR-003) — advisory only, never blocks. | New, `session/budget_warning.go` |
| `StageCostChart` | New frontend `recharts` `BarChart` component, sibling to `ModelBreakdownChart.tsx`, grouping cost by `SessionRole` with bars clickable to cross-filter `SessionsTable`. | New, `web-app/src/app/insights/StageCostChart.tsx` |
| `ItemBudgetWarning` | New frontend component, visually consistent with (not a fork of) `ProjectedCostCard.tsx`, showing a per-item/per-stage soft-budget warning. | New, `web-app/src/app/insights/ItemBudgetWarning.tsx` |

18 glossary terms.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `PipelineMode` stage-executor storage | JSON-serialized `map[StageRole]PipelineStageExecutor` in a single `string` column (`stage_executors_json`), matching the `ac_snapshot`/`triage_result` convention | PoEAA: Active Record field on an existing entity; ADR-001 | (a) 6 new flat `string` columns (stack.md); (b) ent's native `field.JSON` column type (architecture.md's initial phrasing) | (a) reintroduces the primitive-obsession pattern the requirements doc's Rabbit Hole flags; (b) would be a second, inconsistent JSON-storage mechanism in a schema that already has a working one (`ac_snapshot`, `triage_result`) — see ADR-001 |
| Non-Claude headless execution | Strategy — `headless.PoolClient` interface, one implementation per program, dispatched via a small program→implementation registry | GoF Strategy | (a) add a `Program` field to the shared `CallOptions`/`Pool`, forcing every adapter through Claude's session-reuse/idle-timeout machinery; (b) adopt an external multi-CLI orchestration library | (a) `Pool`'s session caching assumes `claude --resume <uuid>` semantics that don't generalize (confirmed, `session/headless/pool.go`); (b) build-vs-buy research found no mature Go-native option — "extend by hand" is the actual cheapest path |
| Work-stage program/model resolution | Reuse `server/workflows/model_families.go`'s `ResolveModel`, extracted into a shared `session.ResolveExecutorProgram` helper called from both `scheduler.go`'s `FireNow` and the new work-stage spawn path | Shared-function reuse (avoid duplicated business logic) | A second, PipelineMode-specific model-resolution function | Explicit architecture-research recommendation; prevents the two entities' (`Workflow`, `PipelineMode`) model-alias vocabularies from independently drifting |
| Cost aggregation by stage/role and by item | Extend `insights_service.go`'s existing single-pass accumulator loop (Transaction Script) with a 5th accumulator, reading the same per-session `costUSD` value every other accumulator already sums | Transaction Script (PoEAA) | A new, separately-computed `CostAggregationService` or a second query/scan | Consistency-by-construction: every existing accumulator (`dailyMap`, `modelMap`, `activityMap`) sums the identical per-session value in the same loop iteration; a second pass risks racing a different `s.store.GetAll()` snapshot and breaking the "subtotals sum to the total" guarantee (pitfalls research §3c) |
| Soft-budget-warning evaluation | New, separate, inline `EvaluateBudgetThreshold` function, invoked at cost-recording time, reading the canonical `session/tokens/pricing.go` table | New narrow pure function (not GoF, just isolated responsibility) | (a) reuse ADR-029's batched Insights snapshot; (b) extend `CapacityMonitor.checkThresholds` directly | (a) staleness tolerance appropriate for read-only display is wrong for a warning whose value is firing promptly (pitfalls §3b); (b) conflates advisory-only visibility with `CapacityMonitor`'s enforcement responsibility and risks BUG-050's drift class one layer up (pitfalls §3d) — see ADR-003 |
| `PipelineMode.stage_executors` repository boundary | Marshal/unmarshal lives in `session/pipeline_mode_repository.go` / `session/ent_pipeline_mode_repository.go` (already-`session`-package, ent-import-safe) | Repository pattern (PoEAA) | Extending `backlog_service_pipeline_mode.go`'s ent-typed surface (it's already on the `no_ent_in_services` grandfathered exclusion list) | Explicit requirement constraint: new code must not lean on the grandfathered depguard exemption |
| Stage-role map keys | `type StageRole string` newtype with 3 exported consts, used as `map[StageRole]PipelineStageExecutor`'s key type | Type-driven design — illegal-state prevention (newtype) | Bare `string` map keys | A typo'd key (`"triaage"`) silently creates a dead, never-consulted map entry with a bare string; `ExecutorFor(item, role StageRole)` makes an illegal role a compile error at every call site |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `PipelineMode`'s 9-flat-content-field structure | Adding this project's 6 new program/model knobs as more flat columns would grow the entity to 15 flat fields spanning two semantically distinct concerns (content vs. execution config), indistinguishable by name pattern alone | **Refactor-first, narrowly scoped** — introduce the `map[StageRole]PipelineStageExecutor` sub-structure for the *new* executor fields only (ADR-001); leave the 9 pre-existing content-template fields exactly as they are | Adopts architecture.md's recommendation as-is. The 9-field shape was a deliberate, already-reasoned-about choice made *for content templates specifically* by the prior `backlog-configurable-pipeline` project; it did not anticipate a second, structurally different axis (executor selection) being added later. Bundling a full re-migration of both axes into one struct into this project would be a second, unrelated refactor riding on a feature PR — smaller, lower-risk diff to keep them separate and revisit "should content also become per-stage-keyed" independently later if ever justified |
| `server/services/insights_service.go` (~322 lines before this project; `GetInsightsSummary`'s single-pass loop, verified lines 198-520) | This project adds a 5th accumulator (`roleMap`/`RoleCostBreakdown`, Epic 4.1's Story 4.1.3) plus a second, transcript-less accumulation pass over `ItemSession` rows (Story 4.1.4) directly into this file | **Extend as-is** — factor the new role/item accumulation into a small per-iteration helper (e.g. `accumulateRoleCost(roleMap, meta SessionMeta, costUSD float64, unpriced bool)`) so the loop body itself doesn't grow by another 15-20 inline lines, but keep it in this file/function rather than splitting into a second service | The single-pass design is correct and load-bearing (every accumulator must read the same per-session `costUSD` in the same iteration to guarantee "subtotals sum to the total" — see the Pattern Decisions row above); a second, separately-computed aggregation service would risk racing a different `s.store.GetAll()` snapshot. Story 4.1.4's transcript-less pass is necessarily a second loop (it iterates `ItemSession` rows, not `results`), but stays additive and narrowly scoped to `total_cost_usd`/`role_breakdown` only — it does not touch `dailyMap`/`modelMap`/`activityMap`, keeping the growth bounded |
| `server/services/backlog_service_triage.go` (3,426 lines) and `server/services/backlog_service_trigger_triage.go` (791 lines) | Epics 2.3/2.4/4.2 add several new stories' worth of code (executor resolution, cost-sink priced threading, fallback-reason persistence, budget-threshold evaluation) to these already-large files | **Extend as-is** — the new code is additive and localized to specific existing functions (`TriggerTriage`, `TriggerReReview`, `SpawnSessionFromItem`, the triage/review `CostSink` closures); no new file split is warranted by this project's scope | Per `code-architecture-best-practices`' "Extending Bad Architecture" guidance, touching an already-oversized file requires stating which of refactor-first / isolate-via-seam / extend-as-is was chosen and why, rather than leaving a future reviewer to reverse-engineer it. A full split of either file is an independently-scoped refactor this project's appetite does not budget for |

---

## Migration Plan
- **Migration file**: None — this repo applies ent schema changes via
  `client.Schema.Create(ctx)` at process startup (`session/ent_repository.go:187`),
  not versioned SQL migration files (confirmed: `session/ent_repository_migrations.go`'s
  own header distinguishes ent's DDL step from separate one-off data
  migrations, none of which this project needs). Schema changes: run
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  after editing `pipeline_mode.go`, `item_session.go`, and `backlog_item.go` —
  never the plain `ent generate` form (breaks `UpsertRule`, per this repo's
  own generate.go convention). Do not commit generated output under
  `session/ent/` outside `schema/`.
- **Reversibility**: Every new column is `.Optional()` with a zero-value
  default (`""`, `"{}"`, or `Nillable()` for the one `*float64`) — a rollback
  to a pre-project binary reads the same rows fine, simply ignoring the new
  columns.
- **Zero-downtime strategy**: Purely additive columns; no backfill, no
  NOT NULL constraint, no column rename or drop. Existing `PipelineMode`
  rows get `stage_executors_json = "{}"` (parsed as an empty map, meaning
  "no override configured, every stage inherits today's default behavior")
  — satisfies the requirements' hard constraint that existing pipeline modes
  must be unaffected.
- **Rollback procedure**: Revert the binary; the new columns remain in the
  DB (harmless, ent tolerates extra columns it doesn't reference) until a
  later cleanup, per this repo's existing additive-migration convention
  (no destructive rollback needed or performed).

## Observability Plan
- **Logs**: `[PipelineEngine]`-prefixed Warn on (a) an unresolved
  `pipeline_mode` slug (existing, unchanged), (b) a stage's resolved
  `program` value with no working `headless.PoolClient` implementation
  (falls back to Claude, names item/stage/rejected-program), (c) a stage's
  resolved `model` failing `ResolveModel`'s family-alias resolution (aborts
  that headless call rather than passing a broken string to the CLI, per
  BUG-062/§1b precedent). `[BudgetWarning]`-prefixed Warn when
  `EvaluateBudgetThreshold` reports a crossed threshold, naming item ID,
  stage role, threshold, and actual spend.
- **Metrics**: None new — this is a single-user local tool; existing
  Insights dashboard aggregates (now including the role/item breakdown) are
  the metrics surface, per the requirements' explicit "no new oncall
  alerting" Observability Requirements.
- **Alerts**: None — visibility via logs and the Insights UI only, matching
  the requirements doc verbatim.

## Risk Control
- **Feature flag**: None needed — additive, opt-in configuration surface;
  existing items and existing `PipelineMode` rows are unaffected until a
  user explicitly sets a per-stage override or a per-item budget threshold
  (matches the requirements' explicit "No feature flag needed" Risk Control
  section).
- **Rollback procedure**: Standard revert-the-binary; see Migration Plan.
- **Staged rollout**: N/A (single-user local tool, no fleet/percentage
  rollout mechanism exists or is needed).
- **Staged-ship off-ramp**: Phases 1-2 (`PipelineMode` schema/proto/validation
  + `ExecutorFor`/resolution + headless & work-stage threading for Claude
  models only) are independently shippable and deliver the feature's core
  value — per-stage model choice, e.g. running triage on `claude-haiku-4-5`
  while work runs on a larger model — using `headless.CallOptions.Model`,
  which already exists and only needed wiring. Phases 3-5 (Gemini adapter,
  cost aggregation/breakdown, Insights chart, soft budget warning) are each
  additive on top and can slip, be descoped, or be cut without leaving
  Phases 1-2's shipped behavior broken or half-finished — there is no
  Phase-3-through-5 change that Phases 1-2 depend on. For a single-user tool
  where "ship what's done" beats "wait for the full Large-appetite scope,"
  this means a real fallback plan exists if the appetite runs out partway
  through, not just a phase diagram that happens to be ordered.

## Unresolved Questions
- [ ] Should `GeminiCaller` support session/history reuse (if Gemini CLI has
  an equivalent to `claude --resume <uuid>`) to get prefix-cache-style
  savings across repeated calls, matching `Pool`'s existing session
  rotation? Not needed for v1 correctness (each call is self-contained and
  correctly priced without it) — blocks a future performance/cost-efficiency
  follow-up, not any Story in this plan. Owner: whoever picks up a
  Gemini-adapter performance follow-up project.
- [ ] Should the soft-budget threshold ever grow a **per-stage** granularity
  in addition to the per-item granularity this plan ships (Story 4.2.1)? Not
  blocking — per-item matches the existing `ReworkCapOverride` per-item
  override precedent and is sufficient for the requirements' stated success
  metric. Owner: revisit only if user feedback specifically asks for
  per-stage threshold granularity.
- [ ] No quantified baseline for current per-stage spend exists yet, so the
  requirements' "triage cost measurably drops" Success Metric has nothing to
  compare against until one is captured. Not blocking — no new instrumentation
  is needed, since the data already exists today. Before configuring a
  per-stage override on a real pipeline mode for the first time, the operator
  should manually note that mode's current triage/review-stage cost from the
  existing Insights dashboard (post-Phase-5, the new `StageCostChart`/
  `ItemStageCostTable`; pre-Phase-5, the existing `ModelBreakdownChart`/
  `SessionsTable` breakdown) as the "before" number. Owner: the operator —
  this is a single-user tool with no separate PM/QA function, so "whoever
  first configures a per-stage override" and "the tool's one user" are the
  same person by definition; this is stated explicitly here rather than left
  worded as if a team needed to assign the step to someone.

## Dependency Visualization

```
Phase 1: PipelineMode Schema/Repository/Proto/Validation
  Epic 1.1 (ent schema + domain types) ─┐
  Epic 1.2 (proto + service wiring) ────┼──> Epic 1.3 (save-time validation)
                                        │
                                        v
Phase 2: Executor Resolution + Execution Threading
  Epic 2.1 (ExecutorFor + snapshot hash) ──> Epic 2.2 (shared resolve helper)
        │                                          │
        v                                          v
  Epic 2.3 (headless call sites: triage/review) Epic 2.4 (work-stage spawn,
        │                                        resolves BEFORE Start(), no
        │                                        post-hoc SwitchProgram)
        v
  Epic 2.5 (CostSink priced-signal foundation — MUST land before Epic 3.1
            and Epic 4.1; independent of 2.1-2.4, can run in parallel)
        │
        v
Phase 3: Gemini Adapter + Pricing (needs Epic 2.5's priced CostSink signature;
         parallel with Phase 2's Epic 2.3 once Epic 2.1's ExecutorFor exists
         — Epic 2.3's registry needs Epic 3.1)
  Epic 3.1 (GeminiCaller, own concurrency bound, call-time availability) ──> Epic 3.2 (pricing entries)
  Epic 3.3 (Aider save-time rejection, piggybacks on Epic 1.3)
        │
        v
Phase 4: Cost Aggregation + Soft Budget Warning (backend, needs Epic 2.5)
  Epic 4.1 (role/item cost breakdown, incl. unpriced counts) ──> Epic 4.2 (per-item threshold + inline eval)
        │
        v
Phase 5: Frontend
  Epic 5.1 (stage-executor form UI, needs Phase 1; incl. save-error surfacing + force_unknown_model)
  Epic 5.2 (StageCostChart + cross-filter + unpriced indicator + item drilldown, needs Epic 4.1;
            Story 5.2.4's provenance/drift rendering additionally needs Story 2.1.2
            (defines the fields) and Story 2.3.1 (resolveHeadlessCaller — what
            actually populates executor_fallback_reason at runtime; Story 2.1.2
            alone only defines the empty-by-default field). Epic 2.3 is already
            sequenced after Epic 2.1 above, so this is a citation fix, not a new
            scheduling edge.)
  Epic 5.3 (ItemBudgetWarning, needs Epic 4.2)
  Epic 5.4 (accessibility pass on 5.2's new chart)
```

---

## Phase 1: PipelineMode Stage Executors — Schema, Repository, Proto, Validation

### Epic 1.1: Ent schema and domain types
**Goal**: Add the storage column and the Go domain types that give it shape, with zero behavior change for existing rows.

#### Story 1.1.1: `StageRole`/`PipelineStageExecutor` domain types and JSON marshal helpers
**As a** backend developer, **I want** a typed, closed vocabulary for stage roles and a small struct for one stage's executor override, **so that** `ExecutorFor` and the repository layer can't be handed a typo'd role string.
**Acceptance Criteria**:
- A new `StageRole` type with exactly 3 exported consts exists and round-trips through JSON as its underlying string.
  - *Given* `StageExecutors{StageRoleTriage: {Model: "claude-haiku-4-5"}}`, *When* `SerializeStageExecutors` marshals it, *Then* the resulting JSON string is `{"triage":{"program":"","model":"claude-haiku-4-5"}}`.
- `ParseStageExecutors("")` and `ParseStageExecutors("{}")` both return an empty, non-nil map with no error.
  - *Given* a `PipelineMode` row with `stage_executors_json = ""` (a pre-existing row from before this migration), *When* `ParseStageExecutors` is called on it, *Then* it returns `map[StageRole]PipelineStageExecutor{}` and `err == nil`.
**Files**: `session/pipeline_mode_executor.go` (new)

##### Task 1.1.1a: Define `StageRole`, consts, and `PipelineStageExecutor` (~3 min)
- Add `type StageRole string`, `const (StageRoleTriage StageRole = "triage"; StageRoleReview StageRole = "review"; StageRoleWork StageRole = "work")`, and `type PipelineStageExecutor struct { Program string `json:"program"`; Model string `json:"model"` }`.
- Files: `session/pipeline_mode_executor.go`

##### Task 1.1.1b: Add `SerializeStageExecutors`/`ParseStageExecutors`, mirroring `SerializeAcCriteria`/`ParseAcCriteria` (~4 min)
- `func SerializeStageExecutors(m map[StageRole]PipelineStageExecutor) (string, error)` — `json.Marshal`; `func ParseStageExecutors(raw string) (map[StageRole]PipelineStageExecutor, error)` — treats `""` and `"{}"` identically, returning an empty non-nil map.
- Files: `session/pipeline_mode_executor.go`

##### Task 1.1.1c: Unit tests for round-trip and empty-string handling (~4 min)
- Table-driven test covering: empty map, one stage, all 3 stages, malformed JSON (expect error), `""` input (expect empty map, no error).
- Files: `session/pipeline_mode_executor_test.go` (new)

#### Story 1.1.2: Ent schema field + regenerate
**As a** backend developer, **I want** `PipelineMode` to persist `stage_executors_json`, **so that** the override survives a restart.
**Acceptance Criteria**:
- `PipelineMode.stage_executors_json` exists, `Optional()`, `Default("{}")`.
  - *Given* an existing `PipelineMode` row created before this migration, *When* the server restarts with the new schema, *Then* `client.Schema.Create(ctx)` adds the column with default `"{}"` and the row's `ParseStageExecutors` call returns an empty map (no error, no data loss).
**Files**: `session/ent/schema/pipeline_mode.go`

##### Task 1.1.2a: Add the ent field (~2 min)
- Add `field.String("stage_executors_json").Optional().Default("{}").Comment("JSON-serialized map[StageRole]PipelineStageExecutor — see session.SerializeStageExecutors/ParseStageExecutors. Empty/\"{}\" means no stage override configured.")` after `initial_prompt_template` in `session/ent/schema/pipeline_mode.go`.
- Files: `session/ent/schema/pipeline_mode.go`

##### Task 1.1.2b: Regenerate ent code (~2 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; confirm `go build ./...` compiles. Do not commit generated output.
- Files: (generated, not committed)

### Epic 1.2: Proto and service wiring
**Goal**: Expose `StageExecutors` over the RPC surface Create/Update/Get/List already use for `PipelineMode`.

#### Story 1.2.1: Proto messages for `PipelineStageExecutor` and the stage-executors map
**As a** frontend developer, **I want** a typed proto shape for stage executors, **so that** the settings UI can read/write them without hand-parsing JSON.
**Acceptance Criteria**:
- `sessionv1.PipelineMode` includes `map<string, PipelineStageExecutor> stage_executors = 18;`, and `CreatePipelineModeRequest`/`UpdatePipelineModeRequest` include a way to set it.
  - *Given* a `CreatePipelineModeRequest` with `stage_executors: {"triage": {model: "claude-haiku-4-5"}}`, *When* `CreatePipelineMode` succeeds, *Then* the returned `PipelineMode.stage_executors["triage"].model == "claude-haiku-4-5"` and `.program == ""`.
**Files**: `proto/session/v1/backlog.proto`

##### Task 1.2.1a: Add `message PipelineStageExecutor` and the map field on `PipelineMode` (~3 min)
- After `message PipelineMode { ... string content_hash = 17; }`, add:
  ```proto
  message PipelineStageExecutor {
    string program = 1;
    string model = 2;
  }
  ```
  and inside `PipelineMode`: `map<string, PipelineStageExecutor> stage_executors = 18;`
- Files: `proto/session/v1/backlog.proto`

##### Task 1.2.1b: Add the map field to `CreatePipelineModeRequest` (field 14) (~2 min)
- `map<string, PipelineStageExecutor> stage_executors = 14;` after `initial_prompt_template = 13;`.
- Files: `proto/session/v1/backlog.proto`

##### Task 1.2.1c: Add an optional-presence wrapper for `UpdatePipelineModeRequest` (field 14) (~3 min)
- Proto3 has no field-presence for `map`/`repeated` fields directly, so wrap it: `message StageExecutorsUpdate { map<string, PipelineStageExecutor> values = 1; }` then `optional StageExecutorsUpdate stage_executors = 14;` on `UpdatePipelineModeRequest` — `nil` means "leave stage executors untouched," present-but-empty means "clear all overrides," matching every other field's pointer-means-untouched convention on this message.
- Files: `proto/session/v1/backlog.proto`

##### Task 1.2.1d: Regenerate proto Go/TS bindings (~2 min)
- Run `make proto-gen`; confirm `go build ./...` and `cd web-app && npx tsc --noEmit` both succeed.
- Files: (generated, not committed per repo convention for `gen/`)

#### Story 1.2.2: Repository + service layer thread `StageExecutors`
**As a** backend developer, **I want** `PipelineModeCreateInput`/`UpdateInput` and `pipelineModeToProto` to carry `StageExecutors`, **so that** a save round-trips correctly.
**Acceptance Criteria**:
- `EntPipelineModeRepository.Create` with `StageExecutors: map[StageRole]PipelineStageExecutor{StageRoleTriage: {Model: "claude-haiku-4-5"}}` persists a row whose `stage_executors_json` parses back to the same map.
  - *Given* a `PipelineModeCreateInput{Slug: "cheap-triage", StageExecutors: map[StageRole]PipelineStageExecutor{StageRoleTriage: {Model: "claude-haiku-4-5"}}}`, *When* `EntPipelineModeRepository.Create` is called, *Then* `ParseStageExecutors(row.StageExecutorsJSON)[StageRoleTriage].Model == "claude-haiku-4-5"`.
**Files**: `session/pipeline_mode_repository.go`, `session/ent_pipeline_mode_repository.go`, `server/services/backlog_service_pipeline_mode.go`

##### Task 1.2.2a: Add `StageExecutors`/`*map[StageRole]PipelineStageExecutor` to the create/update input structs (~3 min)
- `PipelineModeCreateInput.StageExecutors map[StageRole]PipelineStageExecutor`; `PipelineModeUpdateInput.StageExecutors *map[StageRole]PipelineStageExecutor` (nil = untouched).
- Files: `session/pipeline_mode_repository.go`

##### Task 1.2.2b: Marshal in `EntPipelineModeRepository.Create`/`Update` (~4 min)
- Call `SerializeStageExecutors` before `SetStageExecutorsJSON(...)`; on `Update`, only touch the field when `m.StageExecutors != nil`.
- Files: `session/ent_pipeline_mode_repository.go`

##### Task 1.2.2c: Unmarshal in `pipelineModeToProto` (~4 min)
- Call `ParseStageExecutors(pm.StageExecutorsJSON)`, log-and-treat-as-empty on parse error (never fail the whole response for one malformed row), convert to `map[string]*sessionv1.PipelineStageExecutor`.
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 1.2.2d: Wire `req.Msg.StageExecutors` through `CreatePipelineMode`/`UpdatePipelineMode` (~4 min)
- Convert the proto map to `map[StageRole]PipelineStageExecutor` (validated in Epic 1.3 first), pass into `PipelineModeCreateInput`/`PipelineModeUpdateInput`, call `s.invalidatePipelineCache` after a successful write (existing call, already present at both sites — no new invalidation call needed since it invalidates unconditionally after every write).
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 1.2.2e: Repository + service tests (~5 min)
- `EntPipelineModeRepository` round-trip test; `CreatePipelineMode`/`UpdatePipelineMode` RPC-level test asserting the returned proto's `stage_executors` map matches the request.
- Files: `session/ent_pipeline_mode_repository_test.go`, `server/services/backlog_service_pipeline_mode_test.go`

### Epic 1.3: Save-time validation
**Goal**: A typo'd model/program string is rejected at config-save time with a clear error, per BUG-062/pitfalls §1b — never left to surface as a confusing subprocess failure later.

#### Story 1.3.1: Validate stage program/model strings in `ValidatePipelineModeContent`
**As an** operator, **I want** an invalid program or model name rejected when I save a pipeline mode, **so that** I never discover the mistake via a failed autonomous triage run hours later.

**Design note on model validation (resolves architecture-review Blocker #2):** verified by direct read of `server/workflows/model_families.go` that `ValidateModel` (lines 108-116) is a character-class regex check only (no model-ID registry), and `ResolveModel` (lines 80-95) passes any non-`family:`-prefixed string through unchanged by design — reusing that logic verbatim, as originally specified, would NOT reject `"claude-opus-9000"`, contradicting this story's own acceptance test. Chosen fix is **option (a)** from the review (cross-check against the pricing table, with an explicit override) rather than (b) (weaken the AC to character-class-only): the whole point of this story is catching a BUG-062-style typo at save time, which is exactly the failure (a syntactically-valid-but-nonexistent model ID) a character-class check cannot catch. A pricing-table cross-check is cheap (the table already exists, Epic 3.2 extends it with Gemini entries) and the override flag accepts the real risk named in the review ("new models ship before pricing tables are updated") without silently re-permitting typos.
**Acceptance Criteria**:
- Saving a mode with `stage_executors["triage"].program = "aider"` is rejected with `CodeInvalidArgument`.
  - *Given* a `CreatePipelineModeRequest{Slug: "bad-mode", StageExecutors: {"triage": {program: "aider"}}}`, *When* `CreatePipelineMode` is called, *Then* it returns a `connect.CodeInvalidArgument` error containing `"aider"` and `"triage"`, and no row is persisted.
- Saving a mode with `stage_executors["work"].program = "aider"` succeeds (work stage has no headless-only allow-list).
  - *Given* the same request but with `stage_executors: {"work": {program: "aider"}}`, *When* `CreatePipelineMode` is called, *Then* it succeeds and the returned `PipelineMode.stage_executors["work"].program == "aider"`.
- Saving a mode with `stage_executors["review"].model = "claude-opus-9000"` (unrecognized model ID, not a `family:` alias, not present in `session/tokens/pricing.go`'s table) is rejected **unless** the request sets `force_unknown_model: true`.
  - *Given* `stage_executors: {"review": {model: "claude-opus-9000"}}` and `force_unknown_model` unset/false, *When* `CreatePipelineMode` is called, *Then* it returns `CodeInvalidArgument` naming the unrecognized model and mentioning `force_unknown_model` as the override.
  - *Given* the same request but with `force_unknown_model: true`, *When* `CreatePipelineMode` is called, *Then* it succeeds and the returned mode's `stage_executors["review"].model == "claude-opus-9000"`.
  - *Given* `stage_executors: {"review": {model: "family:opus"}}` (a resolvable family alias, not a literal ID), *When* `CreatePipelineMode` is called, *Then* it succeeds with no pricing-table cross-check performed (family aliases are validated by `ResolveModel`'s existing unknown-alias check, unaffected by this story).
- A `stage_executors` map with a key outside `{"triage","review","work"}` is rejected (architecture-review Concern: typo'd keys were previously silently accepted).
  - *Given* `stage_executors: {"wrok": {model: "claude-haiku-4-5"}}`, *When* `CreatePipelineMode` is called, *Then* it returns `CodeInvalidArgument` naming the invalid key `"wrok"` and the 3 valid role names.
**Files**: `session/pipeline_mode_validation.go`, `proto/session/v1/backlog.proto`

##### Task 1.3.1a: Add `StageExecutors map[StageRole]PipelineStageExecutor` and `ForceUnknownModel bool` to `PipelineModeContentFields` (~2 min)
- Extend the struct so `ValidatePipelineModeContent` receives the stage executors to check plus the override flag.
- Files: `session/pipeline_mode_validation.go`

##### Task 1.3.1b: Validate headless-role programs against an allow-list (~4 min)
- For `StageRoleTriage`/`StageRoleReview` entries only: reject any non-empty `Program` not in `{"claude", "gemini"}` with a clear error naming the role and rejected program (ADR-002). `StageRoleWork` entries are not checked against this allow-list.
- Files: `session/pipeline_mode_validation.go`

##### Task 1.3.1c: Validate model strings via `workflows.ValidateModel`-equivalent shared logic (~5 min)
- Reuse the same character-class/shell-metacharacter guard `server/workflows/model_families.go`'s `ValidateModel` already applies (extract to a shared location if not already importable from `session`, avoiding import-cycle — `session` cannot import `server/workflows`, so move the pure regex-based `ValidateModel` function to a location both can import, e.g. `session/executor_program.go`, and have `server/workflows` call the shared one instead of keeping its own copy). This step catches shell-metacharacter/whitespace injection only, not unknown-but-syntactically-valid IDs — that's Task 1.3.1c2.
- Files: `session/executor_program.go` (new, shared with Story 2.2.1), `server/workflows/model_families.go`

##### Task 1.3.1c2: Cross-check literal (non-`family:`-prefixed) model IDs against the pricing table (~5 min)
- After Task 1.3.1c's character-class check passes: if the model string is not `family:`-prefixed, call `tokens.DefaultPricingTable().LookupByModel(model)`; if not found (`ok == false`) and `ForceUnknownModel` is not set, return `CodeInvalidArgument` naming the model and mentioning the `force_unknown_model` override. A `family:`-prefixed alias skips this check entirely — it's already validated by `ResolveModel`'s existing unknown-alias error, which this task does not duplicate or change.
- Files: `session/pipeline_mode_validation.go`

##### Task 1.3.1d: Wire the new validation into `CreatePipelineMode`/`UpdatePipelineMode` call sites (~3 min)
- Pass `StageExecutors` and `ForceUnknownModel` into the `PipelineModeContentFields` literal already built at both RPC handlers. Add `bool force_unknown_model = 15;` to `CreatePipelineModeRequest` and `optional bool force_unknown_model = 15;` to `UpdatePipelineModeRequest` (next free field numbers after Task 1.2.1b/c's `stage_executors = 14`).
- Files: `server/services/backlog_service_pipeline_mode.go`, `proto/session/v1/backlog.proto`

##### Task 1.3.1e: Validate `stage_executors` map keys against `{"triage","review","work"}` (~3 min)
- Iterate the map's keys (not just values) and reject any key outside the 3 known `StageRole` consts — closes the gap where `ParseStageExecutors`'s `map[StageRole]PipelineStageExecutor` unmarshal (a newtype, not a validated sum type) would otherwise let a typo'd key round-trip through JSON/proto and silently never be consulted by `ExecutorFor`.
- Files: `session/pipeline_mode_validation.go`

##### Task 1.3.1f: Validation unit tests (~6 min)
- Cases: valid claude/gemini triage program, invalid aider triage program, valid aider work program, valid `family:sonnet` model, invalid raw model string (shell metacharacters), unrecognized-but-syntactically-valid model ID rejected, same rejected model accepted with `force_unknown_model: true`, invalid map key rejected, empty program/model (always valid — means inherit default).
- Files: `session/pipeline_mode_validation_test.go`

#### Story 1.3.2: Add `"aider"` to `config.GetAvailablePrograms()`'s detection candidates
**As an** operator, **I want** the settings UI's program dropdown to detect Aider if it's installed, **so that** I can select it for the work stage without it being invisible to program-detection.
**Acceptance Criteria**:
- `GetAvailablePrograms()`'s candidate list includes `"aider"`.
  - *Given* `aider` is on `$PATH`, *When* `Config.GetAvailablePrograms()` runs, *Then* the returned slice includes `"aider"`.
**Files**: `config/config.go`

##### Task 1.3.2a: Add `"aider"` to the `candidates` slice (~2 min)
- `candidates := []string{"proxy-claude", "claude", "claude-code", "gemini", "agy", "aider"}` at `config/config.go:1285`.
- Files: `config/config.go`

##### Task 1.3.2b: Test coverage for the updated candidate list (~2 min)
- Extend/confirm existing `GetAvailablePrograms` test covers the new candidate (skips gracefully if `aider` isn't installed in CI, matching the existing pattern for other candidates).
- Files: `config/config_test.go`

---

## Phase 2: Executor Resolution + Execution Threading

### Epic 2.1: `PipelineEngine.ExecutorFor` and executor snapshotting
**Goal**: A single, narrow resolution method that every headless and work-stage call site consults, following the identical fallback shape every existing `PipelineEngine` method uses.

#### Story 2.1.1: Add `ExecutorFor` to the `PipelineEngine` interface and `CachingPipelineEngine`
**As a** backend developer, **I want** one method that resolves a stage's `(program, model)`, **so that** every call site (headless triage/review, work-stage spawn) shares one resolution/fallback implementation.
**Acceptance Criteria**:
- `ExecutorFor` returns `("", "")` for `PipelineModeDefault`, for an unresolved slug (with a Warn log), and for a resolved mode with no override configured for that specific role.
  - *Given* an item with `PipelineMode: "cheap-triage"` whose `stage_executors` map has only a `"triage"` entry (`{model: "claude-haiku-4-5"}`), *When* `ExecutorFor(item, StageRoleReview)` is called, *Then* it returns `("", "")` — the mode overrides triage only, review inherits the default.
  - *Given* the same item, *When* `ExecutorFor(item, StageRoleTriage)` is called, *Then* it returns `("", "claude-haiku-4-5")`.
  - *Given* an item with `PipelineMode: "does-not-exist"`, *When* `ExecutorFor(item, StageRoleTriage)` is called, *Then* it returns `("", "")` and logs `[PipelineEngine] unresolved pipeline_mode="does-not-exist" item=<id> — falling back to default`.
**Files**: `session/pipeline_engine.go`

##### Task 2.1.1a: Add `ExecutorFor` to the `PipelineEngine` interface (~2 min)
- `ExecutorFor(item *BacklogItemData, role StageRole) (program, model string)` added to the interface block, doc comment cross-referencing the other 5 methods' identical 3-branch shape.
- Files: `session/pipeline_engine.go`

##### Task 2.1.1b: Implement `CachingPipelineEngine.ExecutorFor` (~4 min)
- Mirror `TriagePromptFor`'s exact structure: `PipelineModeDefault` → `("", "")`; cache miss → Warn log + `("", "")`; resolved mode → look up `role` in the mode's (already-parsed, cached) `StageExecutors` map, missing key → `("", "")`, present → `(entry.Program, entry.Model)`.
- Files: `session/pipeline_engine.go`

##### Task 2.1.1c: Extend `pipelineModeCache`'s cached row shape to hold parsed `StageExecutors` (~4 min)
- The cache currently holds the 9 raw template strings per slug; add the parsed `map[StageRole]PipelineStageExecutor` (parsed once on cache load/refresh, not per-call) so `ExecutorFor` never re-parses JSON on the hot path.
- Files: `session/pipeline_engine.go` (the `pipelineModeCache` type and its `Load`/`refresh` methods)

##### Task 2.1.1d: Unit tests for `ExecutorFor`'s 3 branches (~5 min)
- Default mode, unresolved slug (assert Warn log via existing test log-capture convention), resolved mode with role present, resolved mode with role absent.
- Files: `session/pipeline_engine_test.go`

#### Story 2.1.2: `executor_snapshot_hash` on `ItemSession`
**As a** debugging operator, **I want** to see what program/model actually ran for a given session, not just what the mode currently says, **so that** editing a mode later doesn't retroactively misrepresent an old session's cost driver.
**Acceptance Criteria**:
- A newly-spawned session's `ItemSession.executor_snapshot_hash` reflects the resolved `(program, model)` at spawn time, and does not change if the mode is edited afterward.
  - *Given* a triage call resolves to `(program="", model="claude-haiku-4-5")` via `ExecutorFor`, *When* the resulting `ItemSession` row is created, *Then* `resolved_program == ""`, `resolved_model == "claude-haiku-4-5"`, and `executor_snapshot_hash == sha256("|claude-haiku-4-5")[:16]` (fixed `program|model` order, matching `ComputeContentHash`'s fixed-order convention).
- A stage configured for a program that falls back to Claude at call time (adversarial-review Blocker #3: Gemini installed-then-uninstalled) records both what was *configured* and what actually *ran*, not just the latter.
  - *Given* an item's triage stage is configured with `program: "gemini"` but `resolveHeadlessCaller` (Story 2.3.1) detects Gemini unavailable at call time and falls back to Claude, *When* the resulting `ItemSession` row is created, *Then* `configured_program == "gemini"`, `resolved_program == ""` (Claude default), and `executor_fallback_reason == "gemini_unavailable"` — visible on the same "what ran" surface as `resolved_program`/`resolved_model`, not only in a log line.
**Files**: `session/ent/schema/item_session.go`, `session/pipeline_engine.go`

##### Task 2.1.2a: Add 5 ent fields to `ItemSession` (~4 min)
- `field.String("resolved_program").Optional().Default("")`, `field.String("resolved_model").Optional().Default("")`, `field.String("executor_snapshot_hash").Optional().Default("")`, each with a comment cross-referencing `pipeline_mode_snapshot_hash`'s doc comment and noting this hash is independent (execution config, not content). Plus, for adversarial-review Blocker #3's loud-fallback fix: `field.String("configured_program").Optional().Default("")` (the program the stage was actually configured for, before any availability fallback) and `field.String("executor_fallback_reason").Optional().Default("")` (empty unless a fallback occurred, e.g. `"gemini_unavailable"`, `"unsupported_program"`) — populated by Story 2.3.1's `resolveHeadlessCaller` fallback path.
- Files: `session/ent/schema/item_session.go`

##### Task 2.1.2b: Add `ComputeExecutorHash(program, model string) string` (~3 min)
- SHA-256 hex truncated to 16 chars of `program + "|" + model`, mirroring `ComputeContentHash`'s existing truncation convention. Every call site must pass the raw, pre-`ResolveModel` `(program, model)` pair — the value stored on `PipelineMode.stage_executors`/returned directly by `ExecutorFor`, never a `family:`-alias-resolved concrete model ID — so the mode-side hash (Task 5.2.4a) and every session-side hash (Tasks 2.3.2c/2.3.3b/2.4.1d) stay comparable for `family:`-aliased stages; this function performs no resolution itself.
- Files: `session/pipeline_engine.go`

##### Task 2.1.2c: Regenerate ent code (~2 min)
- `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; `go build ./...`.
- Files: (generated, not committed)

##### Task 2.1.2d: Expose the 5 new fields on the `ItemSession` proto message (fields 21-25) (~4 min)
- `string resolved_program = 21; string resolved_model = 22; string executor_snapshot_hash = 23; string configured_program = 24; string executor_fallback_reason = 25;` — extends the same "what ran" read-only surface `pipeline_mode_snapshot`/`pipeline_mode_snapshot_hash` (fields 16-17) already provide, per UX research's unstated-need finding that program/model provenance is the same class of fact and belongs on the same surface. `configured_program`/`executor_fallback_reason` are empty unless `resolveHeadlessCaller` (Story 2.3.1) actually fell back. Wire into `itemSessionToProto` alongside the existing 2 fields.
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service.go` (`itemSessionToProto`, `backlog_service.go:710`)

### Epic 2.2: Shared model/program resolution helper
**Goal**: One resolution mechanism for both `Workflow.Model`/`AgentType` and `PipelineMode`'s new stage executors — no second, parallel implementation.

#### Story 2.2.1: Extract `session.ResolveExecutorProgram` from `scheduler.go`'s `FireNow`
**As a** backend developer, **I want** the `family:sonnet` → concrete-model-ID → `"claude --model <id>"` transform in one shared place, **so that** `FireNow` and the new work-stage spawn path can't independently drift on alias resolution or shell-escaping.
**Acceptance Criteria**:
- `session.ResolveExecutorProgram("", "family:sonnet", families)` returns `("claude --model claude-sonnet-4-6", nil)`.
  - *Given* `families := map[string]string{"sonnet": "claude-sonnet-4-6"}`, *When* `ResolveExecutorProgram("", "family:sonnet", families)` is called, *Then* it returns `("claude --model claude-sonnet-4-6", nil)`.
- `session.ResolveExecutorProgram("aider", "family:sonnet", families)` returns `("aider", nil)` unchanged — the `--model` concatenation only applies when the base program is empty or `"claude"`, matching `FireNow`'s existing `isClaudeProgram` guard.
  - *Given* the same `families`, *When* `ResolveExecutorProgram("aider", "family:sonnet", families)` is called, *Then* it returns `("aider", nil)` — the resolved model is silently not appended for a non-Claude base program (documented limitation: per-stage model override for a non-Claude work-stage program isn't expressible via this string-concatenation mechanism in v1).
- `scheduler.go`'s `FireNow` produces byte-identical `program` values before and after the extraction.
  - *Given* the existing `TestFireTrigger_NeverSetsAutoApproveFlag`-style test fixtures, *When* `FireNow` is run against the extracted helper instead of its old inline logic, *Then* all pre-existing `scheduler_test.go` assertions on the resulting `program` string still pass unmodified.
**Files**: `session/executor_program.go` (new), `server/workflows/scheduler.go`

##### Task 2.2.1a: Move `ResolveModel` and `ValidateModel` to `session/executor_program.go` (~5 min)
- Relocate the pure, dependency-free logic from `server/workflows/model_families.go` (both functions have no `server/workflows`-specific dependency) into a new shared file; keep `DefaultModelFamilies`/`LoadModelFamilyOverride` in `server/workflows` (they're workflow-specific config loading), calling the relocated `session.ResolveModel`/`session.ValidateModel`.
- Files: `session/executor_program.go` (new), `server/workflows/model_families.go`

##### Task 2.2.1b: Add `ResolveExecutorProgram(baseProgram, modelValue string, families map[string]string) (string, error)` (~4 min)
- Extract the `program := wf.AgentType; if resolvedModel != "" && (program == "" || program == "claude") { program = "claude --model " + resolvedModel }` logic from `scheduler.go:381-388` verbatim into this function, calling `session.ResolveModel` internally.
- Files: `session/executor_program.go`

##### Task 2.2.1c: Update `scheduler.go`'s `FireNow` to call the shared helper (~4 min)
- Replace the inline resolution block with a call to `session.ResolveExecutorProgram(wf.AgentType, wf.Model, families)`.
- Files: `server/workflows/scheduler.go`

##### Task 2.2.1d: Update all existing `model_families_test.go`/`scheduler_test.go` imports (~4 min)
- Fix any test that directly called `workflows.ResolveModel`/`workflows.ValidateModel` to call the relocated `session.ResolveModel`/`session.ValidateModel` instead; add a thin re-export or update call sites — whichever keeps the diff smaller once the actual test file is inspected.
- Files: `server/workflows/model_families_test.go`, `server/workflows/scheduler_test.go`

##### Task 2.2.1e: Unit tests for `ResolveExecutorProgram` (~4 min)
- Cases: empty base program + family alias, `"claude"` base + concrete model ID (no `family:` prefix), non-claude base + any model (model silently dropped), unknown family alias (error).
- Files: `session/executor_program_test.go` (new)

### Epic 2.3: Headless call sites (triage/review) wire per-stage model + program
**Goal**: `TriggerTriage`/`TriggerRetriage`/`TriggerReReview` resolve and apply the item's configured triage/review executor, with fail-closed fallback to Claude for an unsupported program.

#### Story 2.3.1: Headless caller registry and `resolveHeadlessCaller`
**As a** backend developer, **I want** a single lookup that picks the right `headless.PoolClient` for a resolved program, **so that** an unsupported/typo'd/no-longer-installed program name fails closed to Claude with a loud, visible signal, never a silent no-op, a crash, or a startup-time-only check that goes stale.
**Acceptance Criteria**:
- `resolveHeadlessCaller("gemini")` returns the wired `GeminiCaller` once Epic 3.1 lands; before that, it returns the Claude pool with a Warn log.
  - *Given* `s.headlessCallers = map[string]headless.PoolClient{"claude": s.headlessPool}` (Gemini not yet wired), *When* `s.resolveHeadlessCaller(ctx, "gemini", itemID, "triage")` is called, *Then* it returns `(s.headlessPool, "gemini", "unsupported_program")` and logs `[PipelineEngine] unsupported headless program="gemini" item=<id> stage="triage" — falling back to claude`.
  - *Given* `s.headlessCallers` includes `"gemini": geminiCaller` (Epic 3.1 landed) and Gemini is currently detected available, *When* `resolveHeadlessCaller(ctx, "gemini", ...)` is called, *Then* it returns `(geminiCaller, "", "")` with no log line.
- **Availability is re-checked at call time, not only at server-startup wiring** (resolves adversarial-review Blocker #3: a program installed when a `PipelineMode` was saved but later uninstalled would otherwise silently downgrade to paid Claude forever, discoverable only by reading logs).
  - *Given* `gemini` was on `$PATH` when `server/dependencies.go` wired `s.headlessCallers["gemini"]` at startup, but is removed from `$PATH` before a later call, *When* `resolveHeadlessCaller(ctx, "gemini", itemID, "triage")` is called, *Then* it re-probes availability (via `GeminiCaller.Available()`, Task 3.1.1g), detects it missing, returns `(s.headlessPool, "gemini", "gemini_unavailable")`, and logs a Warn — the caller (Story 2.3.2/2.3.3) persists the returned `configuredProgram`/`fallbackReason` onto the resulting `ItemSession` row (Story 2.1.2's `configured_program`/`executor_fallback_reason` fields) so the substitution is visible on the session's own "what ran" surface, not only in a log line nobody may be watching.
**Files**: `server/services/backlog_service.go`, `server/dependencies.go`, `session/headless/gemini_caller.go`

##### Task 2.3.1a: Add `headlessCallers map[string]headless.PoolClient` field to `BacklogService` (~2 min)
- Alongside the existing `headlessPool headless.PoolClient` field.
- Files: `server/services/backlog_service.go`

##### Task 2.3.1b: Add `resolveHeadlessCaller(program, itemID, stage string) (caller headless.PoolClient, configuredProgram, fallbackReason string)` method (~5 min)
- Empty `program` or `"claude"` → `(s.headlessPool, "", "")` directly (no map lookup, no availability probe, no log — this is the overwhelmingly common case). Non-empty, non-`"claude"` → map lookup; miss → `(s.headlessPool, program, "unsupported_program")` + Warn log; hit → call the caller's `Available() bool` (Task 3.1.1g) if it implements an `availabilityChecker` interface — unavailable → `(s.headlessPool, program, program+"_unavailable")` + Warn log; available → `(caller, "", "")`.
- Files: `server/services/backlog_service.go`

##### Task 2.3.1c: Wire `s.headlessCallers` in `server/dependencies.go` (~3 min)
- Populate with `{"claude": headlessPool}` for now (Story 3.1.2 adds `"gemini"`).
- Files: `server/dependencies.go`

##### Task 2.3.1d: Unit test for `resolveHeadlessCaller`'s branches (~4 min)
- Empty program, known non-claude program (available), unknown program (assert fallback + Warn log), known program that reports unavailable at call time (assert fallback + `fallbackReason` + Warn log, distinct from the unknown-program case).
- Files: `server/services/backlog_service_test.go`

#### Story 2.3.2: `TriggerTriage`/`TriggerRetriage` resolve and apply the triage executor
**As an** operator, **I want** triage to actually run on the cheap model I configured, **so that** my per-stage cost-control configuration takes effect.
**Acceptance Criteria**:
- With `stage_executors["triage"] = {model: "claude-haiku-4-5"}` on the item's mode, the triage `CallOptions.Model` is `"claude-haiku-4-5"`.
  - *Given* item `bl_abc123` with `PipelineMode: "cheap-triage"` (as configured in Story 1.2.2's example), *When* `TriggerTriage` runs for `bl_abc123`, *Then* the `headless.CallOptions{}` passed to `CallBlocking` has `Model: "claude-haiku-4-5"`.
- With no override configured, behavior is byte-identical to today (`Model: ""`).
  - *Given* an item on `PipelineModeDefault`, *When* `TriggerTriage` runs, *Then* `CallOptions.Model == ""`, unchanged from current behavior.
**Files**: `server/services/backlog_service_trigger_triage.go`

##### Task 2.3.2a: Resolve `(program, model)` via `ExecutorFor(item, StageRoleTriage)` before the `CallBlocking` call (~4 min)
- Insert right before the existing `s.headlessPool.CallBlocking(triageCtx, ...)` call at `backlog_service_trigger_triage.go:452`; run `model` through `session.ResolveModel` (family-alias resolution) before use.
- Files: `server/services/backlog_service_trigger_triage.go`

##### Task 2.3.2b: Set `opts.Model` and select the caller via `resolveHeadlessCaller` (~3 min)
- `caller, configuredProgram, fallbackReason := s.resolveHeadlessCaller(program, itemID, "triage")`; call `caller.CallBlocking(...)` instead of `s.headlessPool.CallBlocking(...)` directly; `headless.CallOptions{WorkDir: triageWorkDir, Model: model}`.
- Files: `server/services/backlog_service_trigger_triage.go`

##### Task 2.3.2c: Persist `resolved_program`/`resolved_model`/`executor_snapshot_hash`/`configured_program`/`executor_fallback_reason`/`cost_priced` on the created `ItemSession` (~5 min)
- Compute `ComputeExecutorHash(program, model)` and set all 6 fields (the last 3 per Story 2.1.2's Blocker-#3 fix and Epic 2.5's Blocker-#1 fix) when the triage `ItemSession` row is created; `configured_program`/`executor_fallback_reason` come straight from Task 2.3.2b's `resolveHeadlessCaller` return values. **Hash the raw, pre-`ResolveModel` `model` value that Task 2.3.2a's `ExecutorFor` call returned — not the `ResolveModel`-resolved concrete ID used for `CallOptions.Model`/the persisted `resolved_model` field.** Otherwise a `family:`-aliased stage (this project's own canonical `family:opus` example, Story 2.3.3) would hash a different value here than Task 5.2.4a's mode-side hash — which is also computed from the raw, unresolved alias — permanently false-flagging drift on a config that was never edited.
- Files: `server/services/backlog_service_trigger_triage.go`

##### Task 2.3.2d: Test: triage with a per-stage model override actually sets `CallOptions.Model` (~5 min)
- Extend the existing `TriggerTriage` test suite with a case using a `PipelineMode` fixture carrying a triage override.
- Files: `server/services/backlog_service_triage_test.go`

#### Story 2.3.3: `TriggerReReview` resolves and applies the review executor
**As an** operator, **I want** review to run on the model I configured for that stage, **so that** a deliberately stronger review model is actually used.
**Acceptance Criteria**:
- With `stage_executors["review"] = {model: "family:opus"}`, the review call's `CallOptions.Model` resolves to the concrete opus model ID.
  - *Given* item with `stage_executors["review"] = {model: "family:opus"}` and `DefaultModelFamilies()["opus"] == "claude-opus-4-8"`, *When* `TriggerReReview` runs, *Then* `CallOptions.Model == "claude-opus-4-8"`.
**Files**: `server/services/backlog_service_triage.go`

##### Task 2.3.3a: Resolve and apply the review executor at `TriggerReReview`'s `CallBlocking` call site (~4 min)
- Mirror Task 2.3.2a/b exactly, at `backlog_service_triage.go:2875`'s review call.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.3.3b: Persist executor snapshot + fallback + priced fields on the review `ItemSession` row (~4 min)
- Mirror Task 2.3.2c for the review-side `ItemSession` creation, including `configured_program`/`executor_fallback_reason` (Story 2.1.2) and `cost_priced` (Epic 2.5, via the review `CostSink` closure's `priced` value). **As in Task 2.3.2c: `ComputeExecutorHash` must be computed from the raw, pre-`ResolveModel` `model` value, never the resolved concrete ID used for `CallOptions.Model`** — this story's own `family:opus` acceptance test (line 557) is exactly the case that would otherwise permanently false-flag drift against Task 5.2.4a's mode-side (also raw) hash.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.3.3c: Test: review with a `family:opus` override resolves to the concrete model ID (~5 min)
- Files: `server/services/backlog_service_triage_test.go`

### Epic 2.4: Work-stage program threading
**Goal**: `SpawnSessionFromItem` applies the mode's configured work-stage program/model by resolving it **before** the session is created and passing it into `CreateWorktreeSession`/`CreateDirectorySession`'s spawn options — never via a post-hoc `SwitchProgram`/`Restart` call on an already-`Start()`ed instance.

**Design note (resolves adversarial-review Blocker #1):** verified by direct read that `CreateWorktreeSession`/`CreateDirectorySession` (`server/services/session_service.go:1660`, `:1695`+) already call `instance.Start(true)` before returning `inst`, and that `InitialPrompt` is typed into the tmux pane asynchronously once the session reaches Ready state (`session/instance.go:910-913`) — an event armed by that same `Start()` call. The original design (resolve the executor *after* `inst` exists, then call `inst.SwitchProgram`) would unconditionally `Restart()` (kill + relaunch) an already-Active instance for every work-stage spawn carrying an override, racing or dropping the just-armed initial-prompt delivery, with no test covering session/prompt behavior (only the final `inst.Program` string). The fix resolves the executor first and threads it into `InstanceOptions.Program` — the same field `config.ResolveDefaults` already populates before `Start()` — so the session starts on the right program the first time; `SwitchProgram` is reserved for genuine post-spawn/mid-item switches, which this project does not add.

#### Story 2.4.1: Thread a per-spawn program override into `SessionCreator.CreateWorktreeSession`/`CreateDirectorySession`
**As a** backend developer, **I want** `SpawnSessionFromItem` to resolve the work-stage executor before creating the session, **so that** the session starts on the configured program on its first launch, with no kill-and-relaunch and no risk to the initial prompt.
**Acceptance Criteria**:
- `SessionCreator.CreateWorktreeSession`/`CreateDirectorySession` accept a `programOverride string` parameter; a non-empty value is set as `InstanceOptions.Program` in place of `resolved.Program`, **before** `session.NewInstance`/`instance.Start(true)` is called.
  - *Given* `programOverride == "claude --model claude-sonnet-4-6"`, *When* `CreateWorktreeSession` runs, *Then* `NewInstance` is called with `InstanceOptions.Program == "claude --model claude-sonnet-4-6"` and `SwitchProgram`/`Restart` are never invoked during this call.
  - *Given* `programOverride == ""`, *When* either method runs, *Then* behavior is byte-identical to today (`InstanceOptions.Program = resolved.Program`).
- With `stage_executors["work"] = {model: "family:sonnet"}` and no `program` set, the spawned session's `Instance.Program == "claude --model claude-sonnet-4-6"`.
  - *Given* item `bl_def456` with `stage_executors["work"] = {model: "family:sonnet"}`, *When* `SpawnSessionFromItem` completes, *Then* `inst.Program == "claude --model claude-sonnet-4-6"`.
- **The item's kickoff prompt is not dropped or raced when a per-stage override is set** (resolves the adversarial-review blocker directly — no prior AC tested this).
  - *Given* item `bl_def456` with `stage_executors["work"] = {model: "family:sonnet"}` and a non-empty kickoff prompt, *When* `SpawnSessionFromItem` completes and the session reaches Ready state, *Then* the kickoff prompt is delivered to the pane exactly once (asserted via a fake/spy `SessionCreator` or `Instance` capturing calls — confirm the narrowest existing test seam at implementation time), and no `Restart`/`KillSession` call occurs during the spawn.
- With no work-stage override, `Instance.Program` is unchanged from today's behavior.
  - *Given* an item on `PipelineModeDefault`, *When* `SpawnSessionFromItem` completes, *Then* `inst.Program` equals exactly what it would have been before this project.
**Files**: `server/services/backlog_service.go`, `server/services/session_service.go`, `server/services/backlog_service_triage.go`

##### Task 2.4.1a: Resolve the work-stage executor **before** spawning, not after (~4 min)
- Insert immediately before "11. Spawn session" (`backlog_service_triage.go:983`, right after `writeSessionFilesLocked`): call `program, model := s.pipelineEngine.ExecutorFor(item, session.StageRoleWork)`; if both empty, `programOverride := ""` (the overwhelmingly common case — zero overhead, identical behavior). Otherwise resolve `programOverride, resolveErr := session.ResolveExecutorProgram(program, model, families)`; on error, log Warn and use `programOverride = ""` (fail closed to whatever the session creator's own default resolves to) rather than failing the whole spawn.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.4.1b: Add `programOverride string` to `SessionCreator.CreateWorktreeSession`/`CreateDirectorySession` and their `*SessionService` implementations (~6 min)
- Extend the interface (`server/services/backlog_service.go:31,35`) and both `SessionService` methods (`server/services/session_service.go:1632`, `:1695`) with a trailing `programOverride string` parameter; inside each, set `opts.Program = resolved.Program` as today when `programOverride == ""`, else `opts.Program = programOverride` — set on `InstanceOptions` before `session.NewInstance`/`instance.Start(true)`, never afterward. Update the 2 `SpawnSessionFromItem` call sites (`backlog_service_triage.go:995,998`) to pass the value from Task 2.4.1a. Update the 2 other call sites unaffected by this story — `SessionService.SpawnReviewSession` (`session_service.go:1620`) and `TriggerReReview`'s spawn (`backlog_service_triage.go:3042`) — to pass `""` (no behavior change; review-stage program threading is out of this story's scope).
- Files: `server/services/backlog_service.go`, `server/services/session_service.go`, `server/services/backlog_service_triage.go`

##### Task 2.4.1c: Wire `families` (the model-family alias map) into `BacklogService` (~3 min)
- `BacklogService` needs access to the same `families map[string]string` `WorkflowScheduler` already holds — thread it through the constructor/dependency wiring from `server/dependencies.go`, reusing `workflows.LoadModelFamilyOverride`'s already-loaded map rather than loading it a second time. Must be wired before Task 2.4.1a's resolution call.
- Files: `server/services/backlog_service.go`, `server/dependencies.go`

##### Task 2.4.1d: Persist executor snapshot fields on the work-stage `ItemSession`/session row (~3 min)
- Mirror Task 2.3.2c's non-fallback fields: record `resolved_program`, `resolved_model`, `executor_snapshot_hash` for the work-stage spawn too (the work stage has no headless-caller-registry fallback concept — `configured_program`/`executor_fallback_reason`/`cost_priced` are N/A here and left at their zero-value defaults). As with 2.3.2c, `executor_snapshot_hash` must be computed from the raw, pre-`ResolveModel` `(program, model)` pair (per Task 2.1.2b's contract) — moot for a literal model string, but load-bearing the moment a `family:` alias is used here too, so hash before resolution, not after.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.4.1e: Test: work-stage spawn with a `family:sonnet` override sets `Instance.Program` correctly via `InstanceOptions`, not `SwitchProgram`; no-override case is unchanged (~5 min)
- Assert (via a fake `SessionCreator` capturing its `programOverride` argument, or by inspecting `InstanceOptions.Program` if `NewInstance` is reachable in the test) that the override reaches instance creation, not a subsequent method call.
- Files: `server/services/backlog_service_triage_test.go`

##### Task 2.4.1f: Test: the initial prompt survives a work-stage spawn with an override set (~5 min)
- The AC's dedicated Given-When-Then above — confirms the fix actually closes the race, not just that the final `Program` string looks right.
- Files: `server/services/backlog_service_triage_test.go`, `server/services/session_service_test.go` (whichever already has the relevant `InitialPrompt`-delivery test seam — confirm at implementation time)

### Epic 2.5: Priced-cost signal foundation
**Goal**: Extend `headless.PoolClient.CallBlocking`'s `CostSink` to carry an explicit `priced bool` alongside the dollar amount, and thread that signal through to `ItemSession` persistence, so a headless adapter that cannot produce a trustworthy cost (Epic 3.1's `GeminiCaller`) is recorded as unpriced — never a corrupting `$0.00`. **Must land before Epic 3.1 (`GeminiCaller`) and Epic 4.1 (cost breakdown) begin**, since both depend on a trustworthy priced signal reaching `ItemSession`; independent of Epics 2.1-2.4 otherwise and may be implemented in parallel with them.

**Design note (resolves architecture-review Blocker #1):** verified `headless.PoolClient.CallBlocking(ctx, key, systemPrompt, userPrompt, opts, sink CostSink) (string, error)` (`session/headless/client.go:8`) and `type CostSink func(usd float64)` (`session/headless/caller.go:727`) carry only a bare float, with no channel to signal "this call was unpriced." Every existing call site initializes a zero-value float and only overwrites it if `sink` fires — when `sink` isn't invoked (Story 3.1.1's own AC: an unpriced response "does not call `sink(0)`... or not at all if nothing is priced"), the caller's cost variable stays at Go's zero value, indistinguishable from a genuinely free call. This undermines two of requirements.md's Success Metrics (per-stage cost view, budget warning) and directly contradicts ADR-002's own "silent-zero is worse than loud rejection" claim. The fix mirrors the `unpriced`/`pricingUnavailable` convention `session/tokens/pricing.go`'s `EstimateCost`/`ModelFamilyCost` and `ModelBreakdownChart.tsx`'s `pricingUnavailable` prop already use elsewhere in this codebase — abstain rather than guess, at every layer that touches a dollar figure.

#### Story 2.5.1: `CostSink` carries a `priced bool`
**As a** backend developer, **I want** every `CostSink` invocation to say whether its dollar amount is trustworthy, **so that** an adapter that can't price a call has a real channel to say so instead of the caller's cost variable silently staying at its Go zero value.
**Acceptance Criteria**:
- `headless.CostSink`'s signature is `func(usd float64, priced bool)`; `DiscardCost` becomes `func(float64, bool) {}`.
  - *Given* `Pool.CallBlocking` completes successfully, *When* its sink fires, *Then* it is called as `sink(cost, true)` — Claude's `total_cost_usd` is always authoritative when the CLI call succeeds (`session/headless/caller.go:58`), so this is a behavior-identical, mechanically-added `true` at every existing call site.
- All 14 existing `CallBlocking` call sites (verified via `grep -rn '\.CallBlocking('`: `server/services/approval_handler.go:529`, `backlog_service_trigger_triage.go:453`, `session_service.go:5439`, `backlog_service_triage.go:2875`, `backlog_service_intent.go:43`, `session/gate_custom_check.go:166`, `session/autonomous_driver.go:330`, `session/headless/capability_check.go:203`, and 6 sites in `session/headless/features.go`) compile against the new signature with no behavior change.
**Files**: `session/headless/client.go`, `session/headless/caller.go`, plus the 14 call sites above.

##### Task 2.5.1a: Change `CostSink`'s type and `DiscardCost` (~2 min)
- `type CostSink func(usd float64, priced bool)`; `func DiscardCost(float64, bool) {}`.
- Files: `session/headless/caller.go`

##### Task 2.5.1b: Update `Pool.CallBlocking`'s own `sink(cost)` call to `sink(cost, true)` (~2 min)
- Claude's cost always comes from the CLI's own `total_cost_usd` field (`session/headless/caller.go:58`) — there is no unpriced case at this layer for Claude.
- Files: `session/headless/caller.go`

##### Task 2.5.1c: Mechanically update all 14 existing call sites to the new signature (~10 min)
- Inline closures (`func(usd float64) { x = usd }`) become `func(usd float64, _ bool) { x = usd }`; bare `DiscardCost` references need no change (the signature change is transparent there).
- Files: the 14 files listed in the story's acceptance criteria.

##### Task 2.5.1d: Compile + existing test suite green (~3 min)
- `go build ./...` and the existing `session/headless`/`server/services` test suites pass unmodified (this task introduces no behavior change, only a signature).
- Files: n/a (verification task)

#### Story 2.5.2: `ItemSession.cost_priced` persisted field
**As a** backend developer, **I want** the persisted cost to carry its own priced/unpriced flag, **so that** Epic 4.1's role/item breakdown and Epic 4.2's budget threshold can distinguish "$0, unpriced" from "$0, genuinely free."
**Acceptance Criteria**:
- `ItemSession.cost_priced` defaults `true` for all existing and newly-created rows unless a call site explicitly records `priced=false`.
  - *Given* an existing `ItemSession` row from before this migration, *When* the server restarts with the new schema, *Then* `cost_priced == true` (existing Claude-only cost data is retroactively assumed priced — it always was, since Claude is the only program that has ever populated this field).
  - *Given* a `GeminiCaller` call whose `sink` fires with `priced=false`, *When* `UpdateItemSessionCost` is called with that result, *Then* the row's `cost_priced` becomes `false` and `estimated_cost_usd` is left unchanged (not incremented by an untrustworthy `0`).
**Files**: `session/ent/schema/item_session.go`, `session/storage_backlog.go`, `session/storage.go`, `proto/session/v1/backlog.proto`, `server/services/backlog_service.go`

##### Task 2.5.2a: Add the ent field (~2 min)
- `field.Bool("cost_priced").Default(true).Comment("False when the most recent cost-contributing headless call could not produce a trustworthy dollar figure (e.g. an unpriced Gemini model family) — see headless.CostSink's priced signal. Default true so pre-existing Claude-only rows read as priced, matching their actual (always-priced) history.")`.
- Files: `session/ent/schema/item_session.go`

##### Task 2.5.2b: Extend `UpdateItemSessionCost` to take a `priced bool` (~4 min)
- `func (r *EntRepository) UpdateItemSessionCost(ctx context.Context, id string, usd float64, priced bool) error` — when `priced` is `false`, skip the additive `usd` update entirely (it should already be `0` per Story 3.1.1's AC) and set `cost_priced = false` on the row (sticky: once any contributing call for a session is unpriced, the row's total is known-incomplete and must say so, even if a later call on the same row is priced). Update all existing call sites (`backlog_service_trigger_triage.go:488`, `session/backlog_lifecycle_pr.go:607` — both pass `true`, Claude-only today) plus the new review call site (Story 2.3.3).
- Files: `session/storage_backlog.go`, `session/storage.go`

##### Task 2.5.2c: Wire the triage/review `CostSink` closures to pass `priced` through to `UpdateItemSessionCost` (~4 min)
- `func(usd float64, priced bool) { triageCostUSD = usd; triageCostPriced = priced }`, then `s.storage.UpdateItemSessionCost(cleanupCtx, isID, triageCostUSD, triageCostPriced)` — this task extends Story 2.3.2's Task 2.3.2c and Story 2.3.3's Task 2.3.3b (both already touch these exact closures) to also thread `priced`.
- Files: `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_triage.go`

##### Task 2.5.2d: Expose `cost_priced` on the `ItemSession` proto message (field 26) (~2 min)
- `bool cost_priced = 26;` alongside `resolved_program`/`resolved_model`/`executor_snapshot_hash`/`configured_program`/`executor_fallback_reason` (fields 21-25, Story 2.1.2). Wire into `itemSessionToProto`.
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service.go`

##### Task 2.5.2e: Repository + unit tests (~4 min)
- Cases: priced call increments cost and leaves `cost_priced=true`; unpriced call leaves cost unchanged and sets `cost_priced=false`; a priced call after an unpriced one does not reset `cost_priced` back to `true`.
- Files: `session/ent_repository_backlog_test.go` (or `storage_backlog_test.go` — confirm the exact existing test file at implementation time)

---

## Phase 3: Gemini Headless Adapter + Pricing

### Epic 3.1: `GeminiCaller` adapter
**Goal**: A `headless.PoolClient` implementation for `gemini -p ... --output-format json`, with an explicit `priced`/unpriced signal (via Epic 2.5's extended `CostSink`) — never a silent `$0.00` — its own bounded subprocess concurrency (resolves adversarial-review Blocker #2), and a live availability check consulted by `resolveHeadlessCaller` (resolves adversarial-review Blocker #3, Story 2.3.1).

#### Story 3.1.1: `GeminiCaller` implements `headless.PoolClient`
**As a** backend developer, **I want** a Gemini CLI adapter satisfying the same interface Claude's pool does, **so that** call sites can swap between them via `resolveHeadlessCaller` with no special-casing.
**Acceptance Criteria**:
- `GeminiCaller.CallBlocking` shells out `gemini -p "<systemPrompt>\n\n<userPrompt>" --output-format json`, parses the response, and invokes `sink(costUSD, true)` with a cost computed from `stats.models[*].tokens` via the pricing table.
  - *Given* a stubbed `gemini` binary that prints `{"response":"ok","stats":{"models":{"gemini-2.5-pro":{"tokens":{"prompt":1000,"candidates":500,"total":1500}}}}}` on stdout, *When* `CallBlocking` runs against it, *Then* it returns `("ok", nil)` and `sink` is invoked with `(cost, true)` where `cost` is computed from `1000` input / `500` output tokens against the `gemini-2.5-pro` pricing entry.
- A `gemini` response containing an `"error"` object returns a non-nil `error` from `CallBlocking`, never a silently-successful empty result.
  - *Given* a stubbed response `{"error":{"type":"...","message":"quota exceeded","code":429}}`, *When* `CallBlocking` runs, *Then* it returns `("", err)` where `err.Error()` contains `"quota exceeded"`.
- A response with token usage but an unrecognized model family calls `sink` with `priced=false` — **never** `sink(0, true)`, which Epic 2.5's extended `CostSink` now makes structurally distinguishable from a genuinely free call.
  - *Given* a response naming model `"gemini-3.0-ultra"` (not yet in the pricing table) and no other priced models in the same response, *When* `CallBlocking` runs, *Then* the caller logs a Warn naming the unpriced family (mirroring `warnNewUnpricedFamilies`), calls `sink(0, false)` exactly once (Epic 2.5's `UpdateItemSessionCost` treats `priced=false` as "leave `estimated_cost_usd` unchanged, set `cost_priced=false`" — so the `0` argument here is a required placeholder, not a reported cost), and does not report a fabricated `$0.00`-as-priced result.
  - *Given* a response with **both** a priced model (`gemini-2.5-pro`) and an unpriced one (`gemini-3.0-ultra`) in the same call, *When* `CallBlocking` runs, *Then* `sink` is called once with `(costOfPricedPortionOnly, false)` — a mixed response is still marked unpriced overall, since the reported total would otherwise understate the true cost.
- `GeminiCaller` receives a non-empty Claude-CLI-specific `CallOptions` field it cannot honor (`AllowedTools`/`PermissionMode`/`DisallowedTools`) — resolves architecture-review Concern (Liskov/ISP mismatch): these are documented as claude-CLI-flag-shaped and meaningless to `gemini -p`.
  - *Given* `opts.PermissionMode != ""`, *When* `CallBlocking` runs, *Then* it logs a Warn naming the ignored field(s) and proceeds using only `opts.WorkDir`/`opts.Model` — it never silently drops the field with no signal, and never errors (the call still has a valid, if less-restricted, shape without it).
- `GeminiCaller` bounds its own concurrent subprocess count independently of Claude's `Pool` (resolves adversarial-review Blocker #2: no other bound exists anywhere in this call path).
  - *Given* `GeminiCaller` constructed with `maxConcurrent: 2` and 2 calls already holding its semaphore, *When* a 3rd `CallBlocking` call is made and a short queue-wait window elapses with no slot freed, *Then* it returns an error classified as pool-saturation (mirroring `Pool`'s own `ErrPoolSaturated`/`classifyHeadlessCallError`, BUG-093 precedent) rather than spawning an unbounded 3rd subprocess or hanging indefinitely.
**Files**: `session/headless/gemini_caller.go` (new)

##### Task 3.1.1a: Define `GeminiCaller` struct and constructor (~4 min)
- `type GeminiCaller struct { binPath string; pricing *tokens.PricingTable; concurrencySem chan struct{} }`; `func NewGeminiCaller(binPath string, pricing *tokens.PricingTable, maxConcurrent int) *GeminiCaller` — `maxConcurrent <= 0` defaults to `5`, mirroring `headless.defaultMaxConcurrent` and the literal `5` already hardcoded for Claude's `Pool` at `server/dependencies.go:741`, so both pools share one operator-legible concurrency expectation even though they're separate semaphores.
- Files: `session/headless/gemini_caller.go`

##### Task 3.1.1b: Implement subprocess invocation with its own concurrency bound (~6 min)
- Acquire `g.concurrencySem` before `exec.CommandContext(ctx, g.binPath, "-p", combinedPrompt, "--output-format", "json")`, release on return; use the same short-wait-then-fail-fast discipline `Pool.call()`'s `maxQueueWait`/`ErrPoolSaturated` already applies (BUG-093 precedent) rather than blocking indefinitely for a slot. Honor `opts.WorkDir` as `cmd.Dir` with the same eager `filepath.IsAbs`+`os.Stat` validation `TriggerTriage` already does (BUG-062 precedent — apply the same guard here, not rediscover it). If any of `opts.AllowedTools`/`opts.PermissionMode`/`opts.DisallowedTools` is non-empty, log a Warn naming it before proceeding (Claude-only fields, meaningless here).
- Files: `session/headless/gemini_caller.go`

##### Task 3.1.1c: Parse the JSON response into a typed struct matching the documented schema (~4 min)
- `type geminiResult struct { Response string; Stats struct{ Models map[string]struct{ Tokens struct{ Prompt, Candidates, Total, Cached int64 } } }; Error *struct{ Type, Message string; Code int } }`.
- Files: `session/headless/gemini_caller.go`

##### Task 3.1.1d: Compute cost per named model via the pricing table, calling `sink` once with the summed cost and an overall `priced` flag (~6 min)
- For each `stats.models[family]`, look up pricing via `pt.LookupByModel(family)`; sum priced portions; log a Warn (deduped, mirroring `warnNewUnpricedFamilies`'s per-process-lifetime dedup) for any unpriced family. `priced := len(unpricedFamilies) == 0` — a mixed response (some priced, some not) reports `priced=false` overall, since a partial total would understate true cost. Call `sink(total, priced)` exactly once.
- Files: `session/headless/gemini_caller.go`

##### Task 3.1.1e: Map `error` object presence to a returned Go error (~3 min)
- Files: `session/headless/gemini_caller.go`

##### Task 3.1.1f: Unit tests using a fake/stubbed subprocess (mirroring `session/headless/*_test.go`'s existing `ClaudeRunner` fake pattern) (~6 min)
- Cases: success with priced tokens (`sink(cost, true)`), success with unpriced model family (`sink(0, false)`), mixed priced/unpriced response (`sink(partial, false)`), `error` object present, malformed JSON (non-`error`-shaped garbage on stdout), a Claude-only `CallOptions` field set (assert Warn log, call still succeeds).
- Files: `session/headless/gemini_caller_test.go` (new)

##### Task 3.1.1g: Add `Available() bool` for call-time re-probe (~4 min)
- `func (g *GeminiCaller) Available() bool` — a short-TTL-cached (e.g. 30s) `exec.LookPath(g.binPath)` check, not a syscall on every call but bounded fresh enough to catch an uninstall within a bounded window (resolves adversarial-review Blocker #3, consumed by Story 2.3.1's `resolveHeadlessCaller`). Define a small local `availabilityChecker interface { Available() bool }` in `server/services` (or `session/headless`, whichever avoids an import cycle) so `resolveHeadlessCaller` can type-assert against it without every `PoolClient` needing the method — Claude's `*Pool` doesn't implement it and isn't expected to (it has no equivalent "goes missing mid-run" failure mode; the binary is a hard runtime dependency already assumed present).
- Files: `session/headless/gemini_caller.go`, `server/services/backlog_service.go`

##### Task 3.1.1h: Unit tests for `Available()`'s cache/TTL behavior (~3 min)
- Cases: binary present, binary absent, cached result reused within TTL, re-probed after TTL elapses.
- Files: `session/headless/gemini_caller_test.go`

#### Story 3.1.2: Wire `GeminiCaller` into the headless caller registry
**As an** operator, **I want** selecting `"gemini"` for a triage/review stage to actually route to the Gemini adapter, **so that** the per-stage program override is not silently ignored.
**Acceptance Criteria**:
- After this story, `resolveHeadlessCaller("gemini", ...)` returns the real `GeminiCaller`, not the Claude fallback.
  - *Given* `gemini` is on `$PATH` (per `config.GetAvailablePrograms()`) and detected available, *When* the server starts, *Then* `s.headlessCallers["gemini"]` is a non-nil `*headless.GeminiCaller`, and `resolveHeadlessCaller("gemini", ...)` returns `(geminiCaller, "", "")` with no fallback Warn log.
**Files**: `server/dependencies.go`

##### Task 3.1.2a: Construct `GeminiCaller` in `server/dependencies.go` and add it to `headlessCallers` (~4 min)
- Only wire it when a `gemini` binary is actually found (reuse whatever detection `config.GetAvailablePrograms()` already does, or a direct `exec.LookPath("gemini")` — pick whichever avoids a second detection mechanism, confirmed at implementation time by checking if `GetAvailablePrograms`'s result is available at this wiring point). Pass `maxConcurrent: 5` — the same literal already used for Claude's `Pool{MaxConcurrentSessions: 5}` at this file's line 741, so both concurrency bounds are visible together at the wiring site even though they're separate semaphores. Startup-time absence just means the map entry isn't populated at all; Story 2.3.1's `resolveHeadlessCaller` handles that as `"unsupported_program"`, and `Available()` (Task 3.1.1g) handles the entry going stale *after* being wired.
- Files: `server/dependencies.go`

##### Task 3.1.2b: Integration-style test confirming the registry entry is present when `gemini` is detected (~3 min)
- Files: `server/dependencies_test.go`

### Epic 3.2: Gemini pricing table entries
**Goal**: `session/tokens/pricing.go` can price Gemini token usage, following the existing dated/sourced entry convention — no second pricing table.

#### Story 3.2.1: Add Gemini model family pricing entries
**As a** backend developer, **I want** `gemini-2.5-pro`/`gemini-2.5-flash` priced in the canonical table, **so that** `GeminiCaller`'s cost computation and the Insights dashboard agree on one number.
**Acceptance Criteria**:
- `pt.LookupByModel("gemini-2.5-pro")` returns a populated `ModelPricing` with a source-dated comment.
  - *Given* the updated `DefaultPricingTable()`, *When* `LookupByModel("gemini-2.5-pro")` is called, *Then* `ok == true` and `InputPricePerMTok > 0`.
**Files**: `session/tokens/pricing.go`

##### Task 3.2.1a: Add `gemini-2.5-pro` and `gemini-2.5-flash` entries to `DefaultPricingTable()` (~4 min)
- Follow the exact existing comment convention: dated, sourced against `ai.google.dev`'s pricing page (verify the actual current rate at implementation time — do not copy stale numbers from this plan).
- Files: `session/tokens/pricing.go`

##### Task 3.2.1b: Extend `NormalizeModelFamily` to recognize Gemini model ID shapes (~4 min)
- `gemini-2.5-pro`/`gemini-2.5-flash` (and dated variants, if Gemini uses date suffixes) need to normalize the same way `claude-*` variants already do, so `ModelBreakdown`/the new `RoleCostBreakdown` group Gemini calls under one family, not one row per exact model-ID-with-date-suffix.
- Files: `session/tokens/pricing.go`

##### Task 3.2.1c: Pricing table unit tests for the new entries and normalization (~3 min)
- Files: `session/tokens/pricing_test.go`

### Epic 3.3: Aider save-time rejection (ADR-002)
**Goal**: Configuring `"aider"` for a headless (triage/review) stage is rejected at save time, not silently accepted and later falling back mid-run. (Implemented as part of Story 1.3.1's allow-list — this epic is a checkpoint confirming ADR-002's decision is actually enforced end-to-end, not a new code change.)

#### Story 3.3.1: End-to-end confirmation that Aider is rejected for headless roles, accepted for work
**As a** reviewer, **I want** a test proving ADR-002's decision is enforced, **so that** the exclusion isn't just documentation.
**Acceptance Criteria**:
- Already covered by Story 1.3.1's acceptance criteria and Task 1.3.1e's test cases — this story adds one more explicit end-to-end RPC-level test.
  - *Given* a live `BacklogService` test harness, *When* `CreatePipelineMode` is called with `stage_executors["triage"].program = "aider"`, *Then* the RPC returns `CodeInvalidArgument` and no `PipelineMode` row is created (confirmed via a subsequent `ListPipelineModes` call showing the mode absent).
**Files**: `server/services/backlog_service_pipeline_mode_test.go`

##### Task 3.3.1a: Add the end-to-end RPC test (~4 min)
- Files: `server/services/backlog_service_pipeline_mode_test.go`

---

## Phase 4: Cost Aggregation + Soft Budget Warning (Backend)

### Epic 4.1: Role and item cost breakdown
**Goal**: Extend `insights_service.go`'s existing single-pass loop with a role-keyed accumulator and item-drilldown data, guaranteeing sum-consistency with the existing global total by construction; fold in `ItemSession`-sourced costs that have no Claude transcript at all (Story 4.1.4), so a Gemini-priced session is visible in Insights, not silently absent.

#### Story 4.1.1: Proto messages for `RoleCostBreakdown`/`ItemRoleCost`
**As a** frontend developer, **I want** typed proto shapes for the stage/role cost breakdown, **so that** the new chart doesn't need to hand-parse a generic map.
**Acceptance Criteria**:
- `GetInsightsSummaryResponse.role_breakdown` (field 16) is a `repeated RoleCostBreakdown`, each carrying `session_role`, `estimated_cost_usd`, `session_count`, `unpriced_session_count`, and `repeated ItemRoleCost items`.
  - *Given* a response with one `"triage"` session costing `$0.02` for item `bl_abc123` ("Fix login bug"), *When* `GetInsightsSummary` returns, *Then* `role_breakdown` contains one entry with `session_role: "triage"`, `estimated_cost_usd: 0.02`, `session_count: 1`, `unpriced_session_count: 0`, and `items: [{item_id: "bl_abc123", item_title: "Fix login bug", estimated_cost_usd: 0.02, unpriced_session_count: 0}]`.
- `unpriced_session_count` surfaces Epic 2.5's `ItemSession.cost_priced` signal (resolves architecture-review Blocker #1's remaining hop — Insights aggregation must not silently fold an unpriced session's `$0` into the total as if it were free), mirroring `ModelBreakdown.pricing_unavailable`'s existing "abstain rather than guess" convention rather than introducing a new one.
  - *Given* one `"triage"` session with `cost_priced: false` (an unpriced Gemini call) and `estimated_cost_usd: 0`, *When* `GetInsightsSummary` returns, *Then* `role_breakdown`'s `"triage"` entry has `unpriced_session_count: 1`, and that session's `$0` is excluded from `estimated_cost_usd` and from `total_cost_usd` rather than being silently summed in as genuinely free (see Story 4.1.3's sum-consistency AC for how the excluded amount is still accounted for).
**Files**: `proto/session/v1/insights.proto`

##### Task 4.1.1a: Add `message ItemRoleCost` (~3 min)
- `message ItemRoleCost { string item_id = 1; string item_title = 2; double estimated_cost_usd = 3; int32 session_count = 4; int32 unpriced_session_count = 5; }`.
- Files: `proto/session/v1/insights.proto`

##### Task 4.1.1b: Add `message RoleCostBreakdown` (~3 min)
- `message RoleCostBreakdown { string session_role = 1; double estimated_cost_usd = 2; int32 session_count = 3; repeated ItemRoleCost items = 4; int32 unpriced_session_count = 5; }` — `session_role` is a plain string (mirroring `ItemSession.session_role`'s own proto representation), not `ActivityType`'s enum, since `SessionRole` is currently string-typed end-to-end.
- Files: `proto/session/v1/insights.proto`

##### Task 4.1.1c: Add `repeated RoleCostBreakdown role_breakdown = 16;` to `GetInsightsSummaryResponse` (~2 min)
- Files: `proto/session/v1/insights.proto`

##### Task 4.1.1d: Regenerate proto bindings (~2 min)
- `make proto-gen`; `go build ./...`; `cd web-app && npx tsc --noEmit`.
- Files: (generated)

#### Story 4.1.2: `sessionMetaForSessions` retains `ItemID`/`ItemTitle`
**As a** backend developer, **I want** the existing role-lookup map to also carry item identity, **so that** the drilldown-by-item requirement needs zero new store scans.
**Acceptance Criteria**:
- The renamed/extended lookup exposes `ItemID`/`ItemTitle` per session UUID, sourced from data `GetAllItemSessionsWithBacklogInfo` already returns.
  - *Given* `ItemSessionBacklogEntry{SessionUUID: "sess-1", SessionRole: "triage", ItemID: "bl_abc123", ItemTitle: "Fix login bug"}`, *When* `sessionMetaForSessions` builds its map, *Then* `meta["sess-1"] == SessionMeta{Role: "triage", ItemID: "bl_abc123", ItemTitle: "Fix login bug"}`.
**Files**: `server/services/insights_service.go`

##### Task 4.1.2a: Rename `sessionRolesForSessions` → `sessionMetaForSessions`, returning `map[string]SessionMeta` (~4 min)
- `type SessionMeta struct { Role, ItemID, ItemTitle string }` — the loop body (`server/services/insights_service.go:86-99`) already has `e.ItemID`/`e.ItemTitle` available, just needs to stop discarding them.
- Files: `server/services/insights_service.go`

##### Task 4.1.2b: Update all call sites (`GetInsightsSummary`, `ListSessionTokens`, `watchInsights`) (~3 min)
- Replace `roleMap := s.sessionRolesForSessions(ctx)` with `sessionMeta := s.sessionMetaForSessions(ctx)`, and `roleMap[sessionID]` reads with `sessionMeta[sessionID].Role` in `buildSessionSummary`'s existing call.
- Files: `server/services/insights_service.go`

##### Task 4.1.2c: Unit test for the extended map (~3 min)
- Files: `server/services/insights_service_test.go`

#### Story 4.1.3: Add the `roleMap`/`RoleCostBreakdown` accumulator to `GetInsightsSummary`'s existing loop
**As an** operator, **I want** the Insights response to include a cost-by-stage breakdown that sums exactly to the total, **so that** I trust the drilldown numbers.
**Acceptance Criteria**:
- `sum(role_breakdown[].estimated_cost_usd) == total_cost_usd` for any response.
  - *Given* 3 sessions costing `$0.02` (triage, item `bl_abc123`), `$0.15` (review, item `bl_abc123`), and `$1.40` (work, item `bl_xyz789`), *When* `GetInsightsSummary` runs, *Then* `total_cost_usd == 1.57` and `role_breakdown` sums to exactly `1.57` across its 3 entries.
- A session with no matching `ItemSession` row (ad hoc/non-backlog session) is bucketed under an explicit `""` role, not dropped.
  - *Given* an additional ad hoc session costing `$0.05` with no backlog attribution, *When* `GetInsightsSummary` runs, *Then* `role_breakdown` includes an entry with `session_role: ""` and `estimated_cost_usd: 0.05`, and `total_cost_usd == 1.62`.
- A session whose transcript reports usage against an unrecognized model family (`len(summary.UnpricedModels) > 0`, the existing pre-project convention) increments its role/item bucket's `unpriced_session_count` and is excluded from that bucket's (and the grand total's) `estimated_cost_usd` — extending the existing "abstain rather than guess" convention to the new dimension rather than silently folding an unreliable `$0` into a real sum (Epic 4.1's part of architecture-review Blocker #1).
  - *Given* one additional `"review"` session with `summary.UnpricedModels == ["claude-opus-9000"]` and `summary.EstimatedCostUsd == 0`, *When* `GetInsightsSummary` runs, *Then* `role_breakdown`'s `"review"` entry has `unpriced_session_count` incremented by 1, and `total_cost_usd`/that entry's `estimated_cost_usd` are unaffected by it (both the numerator and the total exclude the same flagged subset, so the sum-consistency AC above still holds).
**Files**: `server/services/insights_service.go`

##### Task 4.1.3a: Add `roleMap := make(map[string]*sessionv1.RoleCostBreakdown)` and the per-session accumulation, adjacent to the existing `activityMap` block (~5 min)
- Insert directly after the existing `ab.EstimatedCostUsd += costUSD; ab.SessionCount++` block (`insights_service.go:315-321`), reading `sessionMeta[sessionID].Role` (empty string for unattributed sessions — kept, not skipped). `sessionUnpriced := len(unpriced) > 0` (already computed at line 304 as `summary.UnpricedModels`); when `true`, increment `rb.UnpricedSessionCount` and skip the `EstimatedCostUsd += costUSD`/`totalCostUSD += costUSD` additions for this session (matching how every other accumulator in this loop must now also skip it — see Task 4.1.3e).
- Files: `server/services/insights_service.go`

##### Task 4.1.3b: Accumulate per-item entries within each role bucket (~5 min)
- Nested `map[string]*sessionv1.ItemRoleCost` keyed by `ItemID` within each role's accumulator, using `sessionMeta[sessionID].ItemID`/`ItemTitle`, applying the same unpriced-exclusion rule as Task 4.1.3a.
- Files: `server/services/insights_service.go`

##### Task 4.1.3c: Build the sorted `role_breakdown` slice into the response, sorted by cost descending (matching `activityBreakdown`'s existing sort) (~3 min)
- Files: `server/services/insights_service.go`

##### Task 4.1.3d: Test: sum-consistency across daily/model/activity/role breakdowns for a fixed fixture set, including an unattributed session and an unpriced session (~6 min)
- Files: `server/services/insights_service_test.go`

##### Task 4.1.3e: Audit `totalCostUSD`'s own accumulation site for the same unpriced-exclusion (~3 min)
- `totalCostUSD` is accumulated once, near where `costUSD`/`unpriced` are first read (line 304) — confirm at implementation time whether it already skips unpriced sessions (pre-existing behavior, since `allUnpricedFamilies`/`dailyUnpriced` already track unpriced separately from cost sums) or needs the same guard added; this task exists so the new role/item exclusion added above doesn't silently diverge from whatever the pre-existing total already does.
- Files: `server/services/insights_service.go`

#### Story 4.1.4: Include transcript-less (non-Claude) `ItemSession` costs in the Insights totals
**As an** operator, **I want** a Gemini-priced triage/review session's cost to actually appear in Insights, **so that** the entire point of routing triage to a cheaper/free model is visible, not silently absent.

**Design note (discovered during this repair pass's code verification — not one of the 5 named blockers, but required for Blocker #1's fix to actually deliver requirements.md's stated cost-visibility success metric for Gemini specifically):** verified `GetInsightsSummary`'s entire aggregation pipeline iterates `s.store.GetAll()` (`server/services/insights_service.go:206`), populated exclusively from `.jsonl` Claude-conversation-transcript files under a watched history directory (`session/tokens/store.go:91,233-254`). `GeminiCaller`'s `exec.CommandContext` subprocess call (Story 3.1.1) writes no such file — there is no Claude conversation, no resumable UUID, nothing for the watcher to find. A Gemini-priced triage/review session's cost, though correctly persisted on `ItemSession.estimated_cost_usd`/`cost_priced` via `UpdateItemSessionCost` (Epic 2.5), is therefore invisible to `total_cost_usd`, `dailyMap`, `modelMap`, `activityMap`, **and** the new `role_breakdown` alike — none of them read `ItemSession` directly, all of them only ever see `results`. This is a correctness gap independent of the priced/unpriced signal: even a successfully-*priced* Gemini call disappears from Insights entirely as the plan was originally written, which would silently fail the requirements' "Running triage on a cheaper/free model measurably lowers that item's triage cost... visible in the new per-stage cost view" success metric for the one program this project builds a headless adapter for.
**Acceptance Criteria**:
- An `ItemSession` row with no corresponding transcript in `s.store.GetAll()` (its `session_uuid` matches no `ParseResult`'s associated session/conversation ID) contributes a synthetic entry to `total_cost_usd`, `role_breakdown`, and `role_breakdown[].items`, using its own `estimated_cost_usd`/`cost_priced`/`session_role`/`item_id`.
  - *Given* an `ItemSession` row for a Gemini-executed triage call (`session_role: "triage"`, `estimated_cost_usd: 0.003`, `cost_priced: true`), and `s.store.GetAll()` has no `ParseResult` whose associated session ID matches its `session_uuid`, *When* `GetInsightsSummary` runs, *Then* `total_cost_usd` includes the `0.003`, and `role_breakdown`'s `"triage"` entry includes it in both `estimated_cost_usd` and its `items[]` list.
- A Claude-executed triage/review `ItemSession` row, which does have a matching transcript, is not double-counted.
  - *Given* a Claude triage `ItemSession` whose `session_uuid` matches a `ParseResult` already included via the existing transcript loop (Story 4.1.3), *When* `GetInsightsSummary` runs, *Then* that session's cost is counted exactly once (via the transcript path, as today).
**Files**: `server/services/insights_service.go`, `session/storage_backlog.go`

##### Task 4.1.4a: Add/extend a repository query for `ItemSession` rows with cost data in a time range (~5 min)
- Reuse or extend the existing item-sessions-for-range query (confirm the narrowest existing one at implementation time — likely alongside `GetAllItemSessionsWithBacklogInfo`, Story 4.1.2) to also return `estimated_cost_usd`, `cost_priced`, `session_uuid`.
- Files: `session/storage_backlog.go`

##### Task 4.1.4b: Build the set of transcript-covered session IDs from `results`, then fold in any `ItemSession` row whose `session_uuid` isn't in that set (~6 min)
- After Task 4.1.3's per-`results` loop completes, iterate the `ItemSession` rows from Task 4.1.4a; skip any whose `session_uuid` is present in the transcript-covered set built during that loop (already counted — do not double-count); for the rest, accumulate into `totalCostUSD` and the same `roleMap`/per-item maps using the row's own `estimated_cost_usd`/`cost_priced`, applying Story 4.1.3's identical unpriced-exclusion rule (`!cost_priced` → increment `unpriced_session_count`, exclude from `estimated_cost_usd`/`totalCostUSD`). These synthetic entries do not populate `dailyMap`/`modelMap`/`activityMap` — those remain transcript-only in v1, since the model/activity dimensions for a Gemini call are already handled by Epic 3.2's pricing entries and don't share this loop's transcript-derived shape; scoping the fix to `total_cost_usd`/`role_breakdown` (the two surfaces this project's own success metrics name) keeps the diff bounded.
- Files: `server/services/insights_service.go`

##### Task 4.1.4c: Test: a Gemini-priced triage session with no transcript appears in `total_cost_usd` and `role_breakdown`; a Claude session with a transcript is not double-counted (~5 min)
- Files: `server/services/insights_service_test.go`

### Epic 4.2: Soft budget warning
**Goal**: A per-item, optional spend threshold, checked inline at
cost-recording time for the headless triage/review stages and at
live-cost-recompute time for the work stage (ADR-003) — advisory only,
covering all three pipeline stages, not just the two cheapest.

#### Story 4.2.1: `CostBudgetThresholdUsd` per-item field
**As an** operator, **I want** to set a dollar threshold on a backlog item, **so that** I get warned if that item's total spend crosses it.
**Acceptance Criteria**:
- `UpdateBacklogItem` with `cost_budget_threshold_usd: 5.00` persists and round-trips.
  - *Given* item `bl_abc123`, *When* `UpdateBacklogItem(id: "bl_abc123", cost_budget_threshold_usd: 5.00)` is called, *Then* a subsequent `GetBacklogItem` returns `cost_budget_threshold_usd: 5.00`.
  - *Given* an item that has never had a threshold set, *When* `GetBacklogItem` is called, *Then* `cost_budget_threshold_usd` is unset (proto `optional` absent), not `0.0` (0 is a legitimate configured threshold, distinct from "unset" — same nil-pointer-presence convention as `rework_cap_override`).
**Files**: `session/ent/schema/backlog_item.go`, `session/repository.go`, `session/ent_repository_backlog.go`, `proto/session/v1/backlog.proto`

##### Task 4.2.1a: Add the ent field (~2 min)
- `field.Float("cost_budget_threshold_usd").Optional().Nillable().Comment("Per-item soft-budget-warning threshold in USD. Nil = no threshold configured, no warning ever fires for this item. Mirrors rework_cap_override's single-pointer-presence convention.")`, mirroring `rework_cap_override`'s exact `.Optional().Nillable()` shape.
- Files: `session/ent/schema/backlog_item.go`

##### Task 4.2.1b: Add `CostBudgetThresholdUsd *float64` to `BacklogItemData` and `BacklogItemUpdateInput` (~3 min)
- Mirror `ReworkCapOverride *int`'s exact placement/pattern at `session/repository.go:335` and `:630`.
- Files: `session/repository.go`

##### Task 4.2.1c: Wire through `EntRepository`'s create/update/read paths (~4 min)
- Mirror `ReworkCapOverride`'s 4 call sites (`session/ent_repository_backlog.go:344,464,1114,1261`) exactly.
- Files: `session/ent_repository_backlog.go`

##### Task 4.2.1d: Add `optional double cost_budget_threshold_usd = 39;` to `BacklogItem`, and `= 16`/`= 21` to Create/UpdateBacklogItemRequest respectively (~3 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 4.2.1e: Regenerate ent + proto; repository round-trip test (~4 min)
- Files: (generated), `session/ent_repository_backlog_test.go`

#### Story 4.2.2: Inline threshold evaluation at cost-recording time
**As an** operator, **I want** a Warn log and a visible Insights signal the moment an item's spend crosses its configured threshold, **so that** I notice promptly, not on the next dashboard refresh.
**Acceptance Criteria**:
- A headless call that pushes an item's cumulative cost past its threshold logs a `[BudgetWarning]` line naming the item, stage, threshold, and actual spend, at the moment `CostSink` fires.
  - *Given* item `bl_abc123` with `cost_budget_threshold_usd: 5.00` and cumulative prior spend `$4.90`, *When* a triage call completes with `costUSD: 0.15` (new cumulative: `$5.05`), *Then* `EvaluateBudgetThreshold` returns `warn=true` and a log line `[BudgetWarning] item=bl_abc123 stage=triage threshold=5.00 spent=5.05` is emitted.
  - *Given* the same item with cumulative spend `$3.00` before a `$0.15` call (new cumulative `$3.15`), *When* the same call completes, *Then* `EvaluateBudgetThreshold` returns `warn=false` and no log line is emitted.
- Calling `EvaluateBudgetThreshold` never blocks, retries, or errors the call whose cost triggered it — it is purely observational.
  - *Given* the crossing case above, *When* the triage call's result is returned to its caller, *Then* the call's own success/failure status is unaffected by the threshold having been crossed.
**Files**: `session/budget_warning.go` (new), `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_triage.go`

##### Task 4.2.2a: Implement `EvaluateBudgetThreshold(itemID, stage string, thresholdUSD *float64, cumulativeSpentUSD float64) (warn bool)` (~4 min)
- Pure function: `thresholdUSD == nil` → always `false`; else `cumulativeSpentUSD >= *thresholdUSD`.
- Files: `session/budget_warning.go` (new)

##### Task 4.2.2b: Compute an item's cumulative cost-to-date at cost-recording time (~5 min)
- Sum `ItemSession.estimated_cost_usd` across all of the item's sessions (existing repository query, e.g. via `ListItemSessions`/an existing item-sessions-for-item lookup — confirm the narrowest existing query at implementation time rather than adding a new full scan) plus the just-completed call's cost. Per Epic 2.5's `cost_priced` signal: any session with `cost_priced == false` contributes `$0` to this sum by construction (its `estimated_cost_usd` was never incremented) — the threshold check is therefore a check against *known* spend only, and may under-warn while an item has unpriced sessions. Accepted for v1 (mirrors ADR-003's existing "advisory only, can lag" framing) rather than blocking on a hard number that doesn't exist; if any of the item's sessions have `cost_priced == false`, append `" (excludes N unpriced session(s))"` to the `[BudgetWarning]` log line so the gap is visible in the one place an operator would look, not silently absorbed into "under threshold."
- Files: `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_triage.go`

##### Task 4.2.2c: Call `EvaluateBudgetThreshold` inside the existing `CostSink` callbacks for triage and review, log `[BudgetWarning]` on `warn=true` (~4 min)
- Insert into the `func(usd float64, priced bool) { triageCostUSD = usd; triageCostPriced = priced }`-style sink closures at both call sites (Story 2.3.2/2.3.3's already-modified locations, extended by Epic 2.5).
- Files: `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_triage.go`

##### Task 4.2.2d: Unit tests for `EvaluateBudgetThreshold`'s boundary conditions (~3 min)
- Nil threshold, exactly-at-threshold, just-under, just-over.
- Files: `session/budget_warning_test.go` (new)

##### Task 4.2.2e: Integration test: a triage call crossing the threshold produces the expected log line (~4 min)
- Files: `server/services/backlog_service_triage_test.go`

#### Story 4.2.3: Work-stage soft-budget check on live-cost recompute
**As an** operator, **I want** the soft budget warning to also cover an
actively-running work-stage session, **so that** the stage most likely to run
away in cost (open-ended interactive Claude Code/Aider sessions) isn't the
one stage this feature can't warn about (pre-mortem P1 #2).

**Design note (resolves pre-mortem P1 #2 and the matching cross-artifact
signature inconsistency in ADR-003):** verified there is no periodic ticker
or goroutine anywhere in `session/instance*.go` that writes
`ItemSession.estimated_cost_usd` for an active work-stage session — that
field's own schema comment (`session/ent/schema/item_session.go:80-83`)
states it is "populated for headless sessions" only, and a grep of every
`UpdateItemSessionCost` call site (`server/services/backlog_service_trigger_triage.go:488`,
`session/backlog_lifecycle_pr.go:607`) confirms both are one-shot headless-call
persists, not a recurring work-stage update. A work-stage session's cost is
instead recomputed fresh on every read: `session/history_watcher.go`'s
fsnotify-based `HistoryFileWatcher` fires on every JSONL transcript write,
`session/tokens/store.go`'s `TokenStore.OnHistoryFileChanged`/`GetByUUID`
keeps an in-memory parsed-and-priced cache warm, and
`server/services/backlog_service.go:661`'s `buildCostLookup()` +
`itemSessionToProto` (`:710`, `:811`) recompute `EstimatedCostUsd` from that
cache on every `GetBacklogItem`/`ListBacklogItems`/`WatchBacklogItems`
response — never persisting it back to the DB. This recompute-on-read
cadence, driven by existing polls and event pushes rather than a dedicated
ticker, is the mechanism ADR-003's "periodically re-read" language refers to
(reconciled there to match). The fix hooks `EvaluateBudgetThreshold` into the
smallest set of call sites that already recompute an item's live
`TotalEstimatedCostUsd` on a recurring/subscribed basis — `WatchBacklogItems`'s
snapshot and live-event paths (`server/services/backlog_service_events.go`'s
`snapshotEventForItem`, `convertEventToBacklogItemEvent`) and the direct
`GetBacklogItem` poll (`server/services/backlog_service_query.go:86`) — not
all ~18 other call sites of `backlogItemToProto` (one-shot, action-triggered
lifecycle-mutation responses), which would be a disproportionate diff for
this fix's scope.
**Acceptance Criteria**:
- With `item.CostBudgetThresholdUsd = 5.00` and an active work-stage session
  whose live-computed cost brings the item's `TotalEstimatedCostUsd` to
  `$5.20`, the next read logs a `[BudgetWarning]` line exactly once.
  - *Given* item `bl_abc123` with `CostBudgetThresholdUsd: 5.00` and a live
    work-stage session whose `costFor`-computed cost brings
    `TotalEstimatedCostUsd` to `5.20`, *When* `GetBacklogItem("bl_abc123")` is
    called, *Then* `EvaluateBudgetThreshold("bl_abc123", "work", ptr(5.00),
    5.20)` returns `warn=true` and a `[BudgetWarning] item=bl_abc123
    stage=work threshold=5.00 spent=5.20` line is logged.
  - *Given* the same item polled again immediately after with cost unchanged,
    *When* `GetBacklogItem` is called a second time, *Then* no additional
    `[BudgetWarning]` line is logged (per-item, per-process dedup — mirrors
    the `warnNewUnpricedFamilies` convention already used by Task 3.1.1d) —
    until the item's cost drops back under threshold, at which point a future
    re-crossing warns again.
- An item with no threshold configured never logs, regardless of live cost.
  - *Given* `item.CostBudgetThresholdUsd == nil`, *When* `GetBacklogItem` is
    called with any `TotalEstimatedCostUsd`, *Then* `EvaluateBudgetThreshold`
    returns `warn=false` and nothing is logged.
**Files**: `server/services/backlog_service.go`, `server/services/backlog_service_events.go`, `server/services/backlog_service_query.go`

##### Task 4.2.3a: Add a per-`BacklogService` budget-warning dedup set (~3 min)
- `budgetWarnedItems sync.Map` (key: item ID, value: `struct{}`) field on
  `BacklogService`, cleared only on process restart — same
  per-process-lifetime dedup convention as `warnNewUnpricedFamilies` (Task
  3.1.1d), applied here to avoid re-logging on every poll/push while an item
  stays over threshold.
- Files: `server/services/backlog_service.go`

##### Task 4.2.3b: Add a `checkWorkStageBudget(itemID string, thresholdUSD *float64, totalCostUSD float64)` helper on `BacklogService` (~5 min)
- Calls `session.EvaluateBudgetThreshold(itemID, "work", thresholdUSD,
  totalCostUSD)`; on `warn=true` and `itemID` not already in
  `budgetWarnedItems`, logs `[BudgetWarning] item=<id> stage=work
  threshold=<t> spent=<s>` and stores the ID; on `warn=false`, deletes any
  existing entry for `itemID` so a future re-crossing warns again. No-ops
  when `thresholdUSD == nil`.
- Files: `server/services/backlog_service.go`

##### Task 4.2.3c: Call the helper from `WatchBacklogItems`'s snapshot/live paths and from `GetBacklogItem` (~5 min)
- After `backlogItemToProto`/`backlogItemToProtoOrNil` returns in
  `snapshotEventForItem` (`backlog_service_events.go:233`) and
  `convertEventToBacklogItemEvent` (`:278`), call
  `s.checkWorkStageBudget(itemID, payload.Item.CostBudgetThresholdUsd,
  protoItem.TotalEstimatedCostUsd)`; add the same call directly after
  `backlogItemToProto` in `GetBacklogItem` (`backlog_service_query.go:86`).
  Deliberately left out of the other ~15 `backlogItemToProto` call sites
  (lifecycle mutations, sync) — those are one-shot, action-triggered
  responses, not the recurring read/poll surface this fix targets, and
  adding the check there would be redundant with the two paths already
  covered.
- Files: `server/services/backlog_service_events.go`, `server/services/backlog_service_query.go`

##### Task 4.2.3d: Unit tests for the dedup + threshold-crossing behavior (~5 min)
- Cases: first crossing logs once, a repeated poll while still over threshold
  doesn't re-log, cost dropping back under threshold then re-crossing logs
  again, nil threshold never logs.
- Files: `server/services/backlog_service_test.go`

---

## Phase 5: Frontend

### Epic 5.1: Pipeline mode stage-executor UI
**Goal**: A compact stage × {program, model} table in `PipelineModeForm.tsx`, separate from the existing 9 content-template fields, using the established autocomplete pattern.

#### Story 5.1.1: Stage executor table in `PipelineModeForm.tsx`
**As an** operator, **I want** to set a program/model per stage in the same form where I already edit prompts, **so that** stage execution config lives next to stage content config.
**Acceptance Criteria**:
- The form renders 3 rows (Triage, Review, Work), each with a program autocomplete and a model autocomplete, and submits `stage_executors` alongside the existing 9 fields.
  - *Given* an operator sets the Triage row's Model field to `claude-haiku-4-5` and leaves Program blank, *When* they submit the form for a new mode with slug `cheap-triage`, *Then* `createPipelineMode` is called with `stageExecutors: {triage: {program: "", model: "claude-haiku-4-5"}}`.
- An unset field visually reads as "inherits default," not "no model" (per UX research's explicit gap-to-fix).
  - *Given* the Review row's Model field is empty, *When* the form renders, *Then* the input shows placeholder text `"System default"` (mirroring `PROGRAMS`'s own `{ value: "", label: "System default" }` convention from `programs.ts`), not a bare empty box.
**Files**: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`, `web-app/src/app/settings/pipeline-modes/PipelineModeForm.css.ts`

##### Task 5.1.1a: Add `StageExecutorValues` state (~4 min)
- `type StageExecutorValues = Record<"triage" | "review" | "work", { program: string; model: string }>`, initialized from `mode?.stageExecutors ?? {}`.
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 5.1.1b: Render the stage-executor table using `<table>`/`<th scope="col">` (per UX research's accessibility guidance — not CSS-grid divs) (~5 min)
- 3 rows (Triage/Review/Work), 2 columns (Program, Model), each cell a labeled input (`aria-label="Triage stage program"` etc.) using `PROGRAMS`/`MODEL_AUTOCOMPLETE_OPTIONS` from `web-app/src/lib/constants/programs.ts` as datalist-backed autocomplete, matching `WorkflowForm.tsx`'s existing pattern.
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 5.1.1c: Add the one-line prompt-cache-tradeoff callout near the table (per UX research) (~2 min)
- "Different models per stage means no prompt cache carries over between stages."
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 5.1.1d: Add `templateFieldsGrid`-sibling CSS for the new table section (~3 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.css.ts`

##### Task 5.1.1e: Wire `stageExecutors` into `createPipelineMode`/`updatePipelineMode` calls (~3 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`, `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 5.1.1f: Component tests: submitting sets `stageExecutors` correctly; empty fields submit as `""`/absent-key (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.test.tsx`

##### Task 5.1.1g: Surface `CodeInvalidArgument` save errors inline, near the offending stage row (~5 min)
- Resolves the adversarial-review Concern that Story 1.3.1 fully specifies the backend error shape but no frontend task previously caught it — without this, a rejected save (aider-for-triage, unrecognized model, invalid map key) would present as a generic "failed to save" toast with no indication of which stage/program/model was the problem, undermining the entire point of validating at save time. On a `CodeInvalidArgument` response from `createPipelineMode`/`updatePipelineMode`, parse the error message and render it next to the matching stage row (fall back to a top-of-form banner if the row can't be confidently matched), not just a generic toast.
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 5.1.1h: "Use it anyway" override for an unrecognized-model rejection (~4 min)
- When the surfaced error (Task 5.1.1g) is specifically the unrecognized-model-ID case (Story 1.3.1's `force_unknown_model` check), show a small inline "I know this model isn't in the pricing table yet — save anyway" checkbox/button next to the error; checking it and resubmitting sets `forceUnknownModel: true` on the retried request. New models ship before this codebase's pricing table is updated (Epic 3.2), so a hard, un-overridable rejection would block a legitimate save.
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 5.1.1i: Component tests for error surfacing and the override flow (~4 min)
- Cases: aider-for-triage rejection shows near the Triage row; unrecognized-model rejection shows the override control; checking the override and resubmitting sends `forceUnknownModel: true`.
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.test.tsx`

### Epic 5.2: Stage cost chart
**Goal**: A new `BarChart` sibling to `ModelBreakdownChart.tsx`, bars clickable to cross-filter `SessionsTable`, with an unpriced-session indicator, a per-item stage-cost drilldown (Story 5.2.3), and executor-provenance rendering (Story 5.2.4).

#### Story 5.2.1: `StageCostChart.tsx`
**As an** operator, **I want** to see cost broken down by triage/review/work, **so that** I can tell whether my spend is concentrated in overhead or real implementation work.
**Acceptance Criteria**:
- Given `role_breakdown` with 3 entries (triage `$0.50`, review `$1.20`, work `$14.30`), the chart renders 3 bars sorted by cost descending, each labeled with the role name.
  - *Given* `RoleCostBreakdown[]` = `[{session_role:"triage", estimated_cost_usd:0.50}, {session_role:"review", estimated_cost_usd:1.20}, {session_role:"work", estimated_cost_usd:14.30}]`, *When* `StageCostChart` renders, *Then* the bars appear in order work, review, triage (descending cost), matching `ModelBreakdownChart`'s existing sort convention.
- Zero-session roles are omitted from the chart entirely (never a zero-height bar).
  - *Given* `role_breakdown` has no `"review"` entry for the selected time range, *When* the chart renders, *Then* no "review" bar appears.
- All-empty state shows the same `"No data"` card `ModelBreakdownChart` uses.
  - *Given* `role_breakdown` is empty, *When* `StageCostChart` renders, *Then* it shows a `chartCard` with `"No data"`.
- A role bucket with `unpriced_session_count > 0` shows an unpriced indicator, mirroring `ModelBreakdownChart.tsx`'s existing `pricingUnavailable` prop/badge (UX research's own flagged gap: `RoleCostBreakdown`/`ItemRoleCost` had no unpriced signal at all before this repair pass; resolves architecture-review Blocker #1's frontend hop).
  - *Given* `role_breakdown` includes `{session_role:"triage", estimated_cost_usd:0.50, unpriced_session_count:2}`, *When* `StageCostChart` renders, *Then* the triage bar/legend entry shows the same unpriced badge/asterisk convention `ModelBreakdownChart` already uses for `pricingUnavailable`, with a tooltip naming the count.
**Files**: `web-app/src/app/insights/StageCostChart.tsx` (new), `web-app/src/app/insights/StageCostChart.css.ts` (new)

##### Task 5.2.1a: Scaffold `StageCostChart.tsx` from `ModelBreakdownChart.tsx`'s structure (~5 min)
- Same `PALETTE`, `chartCard`/`chartWrap`/`emptyChart` CSS class reuse (import from `ModelBreakdownChart.css.ts` rather than duplicating, or copy into a new `StageCostChart.css.ts` if the class names need role-specific tweaks — prefer reuse to avoid `jscpd` duplication-gate findings).
- Files: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.2.1b: `toDataPoints` mapping `RoleCostBreakdown[]` → chart data, sorted descending, empty roles omitted (~3 min)
- Files: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.2.1c: Bar `onClick` handler stub (wired to cross-filter in Story 5.2.2) (~2 min)
- Files: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.2.1d: Component tests: sort order, empty-role omission, all-empty state, unpriced badge rendering (~5 min)
- Files: `web-app/src/app/insights/StageCostChart.test.tsx` (new)

#### Story 5.2.2: Bar click cross-filters `SessionsTable`
**As an** operator, **I want** clicking a stage's bar to filter the session list to that stage's sessions, **so that** I don't need a second, disconnected drill-down UI.
**Acceptance Criteria**:
- Clicking the "work" bar sets `SessionsTable`'s search text such that only work-role sessions remain visible.
  - *Given* `StageCostChart` and `SessionsTable` are both rendered in `InsightsDashboard.tsx`, *When* a user clicks the "work" bar, *Then* `SessionsTable`'s filtered result set contains only sessions with `sessionRole === "work"`.
**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.2.2a: Add optional controlled `searchText`/`onSearchTextChange` props to `SessionsTable`, defaulting to its existing internal `useState` when not provided (~5 min)
- Preserves `SessionsTable`'s existing uncontrolled behavior for any other caller; `InsightsDashboard.tsx` becomes the first controlled caller.
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 5.2.2b: Lift `searchText` state into `InsightsDashboard.tsx`, pass to both `SessionsTable` and `StageCostChart` (~4 min)
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 5.2.2c: `StageCostChart`'s bar `onClick` calls `onRoleClick(role)`, which `InsightsDashboard.tsx` maps to a search-text filter matching that role (~4 min)
- Since `SessionsTable`'s Fuse.js search targets session/backlog-title text, not `sessionRole` directly, add a small role-filter mechanism alongside `searchText` (a `roleFilter` prop on `SessionsTable`, applied as an additional array filter before Fuse search) rather than trying to fuzzy-match a role name as free text.
- Files: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 5.2.2d: Component/integration test: clicking a bar filters the table to that role (~5 min)
- Files: `web-app/src/app/insights/InsightsDashboard.test.tsx` (or a new colocated test if none exists for this integration — confirm at implementation time)

##### Task 5.2.2e: Show an "active filter" affordance when `SessionsTable` is role-filtered (~3 min)
- Per `design/ux.md`'s closing "Summary of findings" section: a small "Filtered to: work ×" chip/badge above the table, clearable back to the unfiltered state — avoids the "why is my table filtered and I don't remember why" confusion a bar-click-triggered filter with no visual trace would otherwise cause.
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/app/insights/SessionsTable.tsx`

#### Story 5.2.3: Per-item cost-by-stage breakdown on the backlog item detail view
**As an** operator, **I want** to see one backlog item's cost broken down by stage, **so that** "which backlog item is expensive, and in which stage" (a stated Success Metric) has a real answer, not just a fuzzy text search over the global sessions table.

**Design note (folds in adversarial-review Concern: "drillable by backlog item name" was only partially built):** `ItemRoleCost` (Story 4.1.1) is a message this plan explicitly designs and populates (Epic 4.1's `role_breakdown[].items`), but as originally written had no confirmed frontend consumer anywhere in Phase 5 — Epic 5.2's only drilldown was bar-click → role-filtered `SessionsTable`, and `BacklogItemDetail.tsx`'s free-text session search is not the same as a purpose-built per-item, per-stage cost view the requirements doc names directly ("filterable/drillable by backlog item name... The Insights dashboard... shows a bar chart of cost broken down by stage/role, filterable/drillable by backlog item name").
**Acceptance Criteria**:
- The backlog item detail view shows a small table of this item's cost per stage (triage/review/work), sourced from `GetInsightsSummaryResponse.role_breakdown[].items` filtered to this item's ID — no new RPC needed, since the data already flows through the existing Insights fetch.
  - *Given* item `bl_abc123` has `role_breakdown` entries `{triage: [{item_id:"bl_abc123", estimated_cost_usd:0.02}]}`, `{review: [{item_id:"bl_abc123", estimated_cost_usd:0.15}]}`, *When* `BacklogItemDetail.tsx` renders for `bl_abc123`, *Then* it shows a table with rows `Triage: $0.02` and `Review: $0.15` (no `Work` row, since no `ItemRoleCost` entry exists for that role on this item).
- An item with no cost data anywhere renders nothing (not an empty table).
  - *Given* an item with no matching entries in any `role_breakdown[].items`, *When* the view renders, *Then* the cost-by-stage table is omitted entirely.
- Each stage row also shows its `session_count`, so a cheap-triage-induced
  rework cascade is visible as a rising attempt count next to a shrinking
  cost, not masked by the cost number alone (resolves pre-mortem P1 #1 —
  `ItemRoleCost.session_count` was already computed by Epic 4.1's Task
  4.1.1a but had no confirmed Phase 5 renderer, per adversarial-review's
  Minor finding).
  - *Given* item `bl_abc123`'s triage `ItemRoleCost` is
    `{estimated_cost_usd: 0.02, session_count: 3}` (2 re-triages beyond the
    first attempt), *When* `ItemStageCostTable` renders the Triage row,
    *Then* it shows both the cost and `"3 runs"` (or equivalent), visually
    emphasized (matching `StageCostChart`'s unpriced-badge emphasis
    convention) whenever `session_count > 1`, so a shrinking triage-cost bar
    accompanied by a rising run count reads as a rework signal rather than a
    pure win.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/app/insights/ItemStageCostTable.tsx` (new)

##### Task 5.2.3a: `ItemStageCostTable.tsx` component (~5 min)
- Small presentational component: `{ role: string; costUsd: number; sessionCount: number; unpricedSessionCount: number }[]` in, a table out; reuses `StageCostChart`'s unpriced-badge convention (Task 5.2.1) for consistency rather than inventing a second visual language for the same concept.
- Files: `web-app/src/app/insights/ItemStageCostTable.tsx` (new)

##### Task 5.2.3b: Wire into `BacklogItemDetail.tsx`, filtering the existing Insights fetch to this item's ID (~5 min)
- Placed near where `ItemBudgetWarning` is already being wired (Story 5.3.1c) — both are per-item cost surfaces on the same view. Map each `ItemRoleCost.session_count` (already present on the proto since Task 4.1.1a — no backend change needed) straight into the new `sessionCount` prop.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.2.3c: Component tests: multi-role table, single-role table, empty-state omission (~4 min)
- Files: `web-app/src/app/insights/ItemStageCostTable.test.tsx` (new)

##### Task 5.2.3d: Render `session_count` as a rework/attempt indicator, emphasized when > 1 (~3 min)
- Add a `"N run(s)"` (or equivalent) label to each row from Task 5.2.3a's
  `sessionCount` field; apply the same visual emphasis treatment
  `StageCostChart`'s unpriced badge already uses (Task 5.2.1) when
  `sessionCount > 1`, so multiple triage/review attempts for one item are
  noticeable at a glance, not just present in a tooltip.
- Files: `web-app/src/app/insights/ItemStageCostTable.tsx`

##### Task 5.2.3e: Component test: `session_count > 1` renders the emphasized rework indicator; `session_count == 1` renders plainly (~3 min)
- Files: `web-app/src/app/insights/ItemStageCostTable.test.tsx`

#### Story 5.2.4: Executor provenance — fallback badge and drift indicator on the "what ran" surface
**As an** operator, **I want** to see when a stage's configured program silently fell back to Claude, and when a session's executor config no longer matches what the pipeline mode currently says, **so that** both facts are visible on the page I already check, not only recoverable by reading server logs or querying the API directly — and each fact must be individually recognizable and simultaneously visible, not merged into one lookalike signal or silently suppressed by the other.

**Design note (resolves UX-lens BLOCKER, independently confirmed by the Engineering lens as adversarial-review Concern (d)/pre-mortem Concern (g)):** Story 2.1.2 persists `configured_program`/`executor_fallback_reason`/`executor_snapshot_hash` on `ItemSession` and exposes them on the proto (fields 24, 25, 23) specifically so a program-availability fallback is "a persisted, UI-visible fallback marker... never only a log line" (line 65's Domain Glossary rationale for `headlessCallerRegistry`) — but no task anywhere in the original Phase 5 rendered them. This story closes that gap by extending the exact "what ran" surface that already exists for `pipeline_mode_snapshot`/`pipeline_mode_snapshot_hash` — `resolvePipelineModeDisplay` (`web-app/src/lib/backlog/pipelineModeDisplay.ts`), rendered per-session-row in `SessionsSection.tsx:264` — rather than inventing a second display location or visual language. `executor_fallback_reason` is populated at runtime only by Story 2.3.1's `resolveHeadlessCaller` (Story 2.1.2 alone just defines the field, defaulted empty) — this story's fallback badge is therefore buildable/testable against fixtures the moment Story 2.1.2 lands, but depends on Story 2.3.1 to ever render non-null in practice.

**Design note (resolves Engineering-lens BLOCKER — Finding A, sparse-vs-dense hash mismatch):** `ItemSession.executor_snapshot_hash` (persisted by Tasks 2.3.2c/2.3.3b/2.4.1d, using `ComputeExecutorHash` per Task 2.1.2b) is computed **unconditionally** for every session, including the `("","")` default (AC 2.1.1: `ExecutorFor` returns `("","")` for an unconfigured role, and `ComputeExecutorHash("","")` still produces a concrete non-empty 16-char hash) — it is dense. Task 5.2.4a's `stage_executor_hashes` map must therefore also be dense (an entry for all 3 `StageRole`s on every `PipelineMode`, not just configured ones) so the comparison is meaningful. This mirrors `content_hash`'s own density: verified by direct read of `pipelineModeToProto` (`server/services/backlog_service_pipeline_mode.go:64-74`) that `ComputeContentHash` is called unconditionally over all 9 content fields regardless of whether any given field is empty — there is no "skip if blank" branch. A sparse `stage_executor_hashes` (only configured roles get a key) compared against the dense per-session hash would read every session that ran an unconfigured role's ordinary default as "drifted," which is the common case pre-adoption (Risk Control: overrides are opt-in) and would violate this story's own AC below, which requires no drift indicator for a matching/default-hash session. No change is needed to `ComputeExecutorHash` itself, nor to its existing call sites in Tasks 2.3.2c/2.3.3b/2.4.1d (those already compute a session's hash unconditionally, which is correct and is exactly why the mode side must match) — the fix is scoped entirely to Task 5.2.4a's own new per-role loop in `pipelineModeToProto`.

**Design note (resolves UX-lens BLOCKER 2 — case-priority silently suppresses drift under a simultaneous fallback):** the fallback fact (a spawn-time runtime substitution) and the drift fact (a config-changed-since-spawn fact) are independent and can both be true for the same session (e.g. a session fell back from `gemini` to Claude at spawn time, and the mode's triage executor config was edited again afterward). Task 5.2.4b's `resolveExecutorProvenance` must report both facts when both are true, not pick one via if/else-if priority — see the rewritten task below.

**Acceptance Criteria**:
- A session that fell back from a configured non-Claude program shows a fallback badge naming both the configured and actual program.
  - *Given* a session with `configuredProgram: "gemini"`, `resolvedProgram: ""` (Claude default), `executorFallbackReason: "gemini_unavailable"`, *When* `SessionsSection` renders that session's row, *Then* it shows a badge reading `"Ran on different program (Gemini unavailable, used Claude)"` (or equivalent naming both the configured program and the fallback reason — see the distinct-wording requirement below), positioned next to the existing `pipelineDisplay` badge.
  - *Given* a session with `executorFallbackReason: ""` (no fallback occurred), *When* the row renders, *Then* no fallback badge appears — never an empty badge.
- A session whose `executor_snapshot_hash` no longer matches the pipeline mode's currently-configured executor for that stage/role shows a drift indicator, using its own distinct wording and visual treatment (never the pre-existing content-drift badge's exact text) — see "Distinguishable badge treatments" below.
  - *Given* a session with `executorSnapshotHash: "a1b2c3d4e5f6a1b2"` for its `triage` role, and the item's current `PipelineMode.stageExecutorHashes["triage"] == "f6e5d4c3b2a1f6e5"` (the mode's triage executor was edited after this session ran), *When* the row renders, *Then* it shows an executor-drift indicator reading `"(executor config since changed)"` — distinct from the pre-existing `pipelineDriftBadge`'s `"(content since changed)"` text, which is specifically about content-template drift and would be factually wrong here.
  - *Given* the hashes match, *When* the row renders, *Then* no drift indicator appears — including when the session's role has **no configured override** on the mode (both the session's hash and the mode's per-role hash are `ComputeExecutorHash("","")`, so they match by construction; this is the dense-hash fix's own regression test, not a separate case).
  - *Given* the session predates this feature (`executorSnapshotHash == ""`), *When* the row renders, *Then* no drift indicator appears (an empty snapshot hash is treated as "no signal," matching `resolvePipelineModeDisplay`'s own pre-existing empty-hash handling).
- A session with **both** a non-empty `executorFallbackReason` and a mismatched `executorSnapshotHash` shows **both** the fallback badge and the drift indicator simultaneously — neither suppresses the other.
  - *Given* a session with `executorFallbackReason: "gemini_unavailable"` **and** `executorSnapshotHash` mismatched against `mode.stageExecutorHashes[session.sessionRole]`, *When* the row renders, *Then* both the fallback badge and the executor-drift indicator are shown — a reviewer/screen-reader user sees both facts, not just whichever one a priority order picked.
- The fallback badge, the executor-drift indicator, and the pre-existing content-drift badge are visually distinguishable from one another at a glance (not three lookalike warning-colored text pills) — see "Distinguishable badge treatments" below and ux.md's updated Shared Emphasis Convention.
- Upon seeing either badge, the intended user action is advisory-only — notice and, if desired, investigate manually (e.g. check whether the configured program is still installed, or re-verify the stage's executor config) — mirroring Surface D2's "advisory only" statement; neither badge blocks anything or requires immediate action.
**Files**: `web-app/src/lib/backlog/pipelineModeDisplay.ts`, `web-app/src/components/backlog/detail/SessionsSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.css.ts`, `proto/session/v1/backlog.proto`, `server/services/backlog_service_pipeline_mode.go`

**Distinguishable badge treatments (resolves UX-lens BLOCKER 1):** verified against the real codebase that no icon convention currently exists for warning-style annotations in Insights (`ModelBreakdownChart.tsx`'s `unpricedLabel` and `ProjectedCostCard.tsx`'s `warningText` are both plain colored text, no icon — confirmed by direct read of `ModelBreakdownChart.css.ts:55-58` and `ProjectedCostCard.tsx:44-45`; there is no "asterisk/dagger" glyph anywhere in this codebase). A real icon-prefix convention **does** exist one component over, in this same file: `SessionsSection.tsx`'s `SYNTHETIC_KIND_ICON` map (`SessionsSection.tsx:60-64`) prefixes a diagnostic row's title with an `aria-hidden` emoji icon (`🩺`/`🚫`/`✍️`, falling back to `🔍`). This story introduces a new, analogous icon-prefix convention for the three "what ran" badges rather than falsely claiming reuse of a convention that doesn't exist for warning pills specifically:
- **Fallback badge**: icon `↩` (or an equivalent "substitution" glyph — implementer's choice, confirmed at implementation time), label text "Ran on different program", background/border token distinct from both drift treatments below (new `executorFallbackBadge` style in `BacklogItemDetail.css.ts`, not reusing `pipelineDriftBadge`).
- **Executor-drift indicator**: icon `⚙` (a "config" glyph, distinct from the fallback icon), label text "(executor config since changed)", a new `executorDriftBadge` style — same warning color family as `pipelineDriftBadge` for family resemblance (all three are "something changed" signals) but a distinct border/icon so it's never pixel-identical to the content-drift badge.
- **Pre-existing content-drift badge** (`pipelineDriftBadge`, unchanged): no icon today; this story does **not** retrofit an icon onto it (out of scope — it's shipped, unrelated to this story's two new badges) but its distinctiveness from the two new badges is guaranteed by them each having their own icon, where it has none.
- Each badge's accessible name states the fact in words (e.g. `aria-label="Fell back to Claude: Gemini unavailable"`, `aria-label="Executor config changed since this session ran"`) — never conveyed by icon/color alone (WCAG 1.4.1), matching the "Shared Emphasis Convention" ux.md now extends to cover this surface.

**Narrow-viewport treatment:** per this repo's standing mobile+desktop parity requirement, a session row can now carry up to 3 inline badges (existing `pipelineDisplay`/content-drift badge, new fallback badge, new executor-drift indicator) plus the pre-existing branch/cost/ended badges above it. Below a to-be-confirmed breakpoint (reuse whichever breakpoint `BacklogItemDetail.css.ts`'s existing responsive rules already use, confirmed at implementation time), the pipeline/provenance badge group wraps onto its own line below the session id/role row rather than staying inline and forcing horizontal scroll or truncation — this is a CSS flex-wrap change on the existing `pipelineGroup` container, not a new component.

##### Task 5.2.4a: Add `map<string, string> stage_executor_hashes = 19;` to `PipelineMode` proto, populated densely in `pipelineModeToProto` (~5 min)
- **Dense, not sparse** (per the Engineering-lens design note above): iterate all 3 `StageRole` consts (`StageRoleTriage`, `StageRoleReview`, `StageRoleWork`) unconditionally — for each, look up `stageExecutors[role]` (zero-value `PipelineStageExecutor{}` if the role has no configured override) and set `stage_executor_hashes[string(role)] = session.ComputeExecutorHash(entry.Program, entry.Model)` (Task 2.1.2b's existing function, unmodified). An unconfigured role therefore gets an entry equal to `ComputeExecutorHash("", "")` — the same value a default (unconfigured) session's own `executor_snapshot_hash` computes — so the frontend comparison in Task 5.2.4b is meaningful for every role, configured or not. This is a change to Task 5.2.4a's own new loop only; it does not touch `ComputeExecutorHash`, `ContentHashFor`, or any Phase 2 call site.
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service_pipeline_mode.go`

##### Task 5.2.4b: Add `resolveExecutorProvenance(session, mode)` to `pipelineModeDisplay.ts`, returning independent facts rather than a single-priority `kind` (~6 min)
- Unlike `resolvePipelineModeDisplay`'s single discriminated-union return (appropriate there because "unrecognized"/"resolved"/drift-vs-not are mutually exclusive by construction), fallback and drift here are **independent, co-occurring facts** — return `{ fallback: FallbackInfo | null; drifted: boolean }` (or equivalent — an object/array that can carry both facts at once, not an if/else-if chain that returns early on the first true condition). `fallback` is non-null iff `executorFallbackReason` is non-empty (carrying `configuredProgram`/`resolvedProgram`/`resolvedModel`/`reason`); `drifted` is `true` iff `executorSnapshotHash` is non-empty and does not match `mode.stageExecutorHashes[session.sessionRole]` (per Task 5.2.4a's now-dense map) — computed independently of `fallback`, never short-circuited by it.
- Files: `web-app/src/lib/backlog/pipelineModeDisplay.ts`

##### Task 5.2.4c: Render the fallback badge in `SessionsSection.tsx`, using its own `executorFallbackBadge` treatment (~5 min)
- Renders when `resolveExecutorProvenance(...).fallback` is non-null. Uses the new distinct icon/label/style described above (not `ItemBudgetWarning`'s exact treatment verbatim, and not `pipelineDriftBadge`'s) — same warning color family as the rest of this surface for family resemblance, per AC18's cross-surface consistency requirement, but its own icon and label text per "Distinguishable badge treatments" above.
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.css.ts`

##### Task 5.2.4d: Render the executor-drift indicator using a new `executorDriftBadge` style — distinct icon and distinct label text from the pre-existing content-drift badge (~4 min)
- Renders independently of Task 5.2.4c's fallback badge (both can show at once, per the both-true AC above) when `resolveExecutorProvenance(...).drifted` is `true`. Label text `"(executor config since changed)"` — never the pre-existing `pipelineDriftBadge`'s `"(content since changed)"` string, which names a different fact (prompt/content template, not executor/program/model config).
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.css.ts`

##### Task 5.2.4e: Component tests: fallback-only, drift-only, both-simultaneously, and neither cases; visual distinctness of the 3 badge classNames (~7 min)
- Cases: fallback badge shows/hides correctly and names both programs; drift indicator shows/hides correctly on hash mismatch/match; an unconfigured role's default session never shows drift (regression test for the dense-hash fix); a session with both a fallback reason and a drifted hash renders **both** badges simultaneously (regression test for the case-priority fix) with distinct `className`s/label text from each other and from `pipelineDriftBadge`; a `family:`-aliased stage never shows drift when unedited, even though its resolved concrete model ID differs from the raw stored alias (regression test for the family-alias hash fix) — *Given* a session spawned under `stage_executors["review"] = {model: "family:opus"}`, with `executorSnapshotHash` computed per Task 2.3.3b's fix from the raw `"family:opus"` string and `mode.stageExecutorHashes["review"]` computed per Task 5.2.4a from that same raw string, *When* the row renders with no executor config edit since spawn, *Then* no drift indicator appears — even though `resolved_model`/`CallOptions.Model` resolved to the concrete `"claude-opus-4-8"` ID.
- Files: `web-app/src/lib/backlog/pipelineModeDisplay.test.ts`, `web-app/src/components/backlog/detail/SessionsSection.test.tsx`

### Epic 5.3: Item budget warning UI
**Goal**: A visible, non-blocking per-item warning, visually consistent with `ProjectedCostCard.tsx` but a genuinely separate component (per architecture research — it cannot be parameterized from the existing global-monthly card).

#### Story 5.3.1: `ItemBudgetWarning.tsx`
**As an** operator, **I want** to see, on an item's detail view, whether it has crossed its configured budget, **so that** I notice a runaway-cost item without cross-referencing the global Insights dashboard.
**Acceptance Criteria**:
- An item with `costBudgetThresholdUsd: 5.00` and current total cost `$5.05` shows a warning; one at `$3.00` does not.
  - *Given* `BacklogItem{costBudgetThresholdUsd: 5.00}` and its computed total cost `5.05`, *When* `ItemBudgetWarning` renders, *Then* it shows the warning state (same visual language as `ProjectedCostCard`'s `warningText`/`isWarning` styling).
  - *Given* the same item with total cost `3.00`, *When* it renders, *Then* no warning is shown.
- An item with no threshold configured renders nothing (not an empty warning box).
  - *Given* `costBudgetThresholdUsd` is `undefined`, *When* `ItemBudgetWarning` renders, *Then* the component returns `null`.
**Files**: `web-app/src/app/insights/ItemBudgetWarning.tsx` (new), `web-app/src/app/insights/ItemBudgetWarning.css.ts` (new)

##### Task 5.3.1a: Scaffold `ItemBudgetWarning.tsx`, visually consistent with `ProjectedCostCard.css.ts`'s token/color usage but its own component (~5 min)
- Files: `web-app/src/app/insights/ItemBudgetWarning.tsx`, `web-app/src/app/insights/ItemBudgetWarning.css.ts`

##### Task 5.3.1b: `null`-render when no threshold is configured; warning styling when `totalCost >= threshold` (~3 min)
- Files: `web-app/src/app/insights/ItemBudgetWarning.tsx`

##### Task 5.3.1c: Wire into the backlog item detail view (~4 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.1d: Wire a threshold-setting control into the item's edit form (~4 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx` (or its edit-form sub-component — confirm exact sub-component at implementation time)

##### Task 5.3.1e: Component tests: threshold unset (null render), under threshold, over threshold (~4 min)
- Files: `web-app/src/app/insights/ItemBudgetWarning.test.tsx` (new)

### Epic 5.4: Accessibility
**Goal**: The new chart does not repeat `ModelBreakdownChart.tsx`'s existing accessibility gaps.

#### Story 5.4.1: `role="img"`/`aria-label` and keyboard-interactive legend on `StageCostChart`
**As a** screen-reader user, **I want** the stage cost chart announced with its data, and its clickable legend keyboard-operable, **so that** I'm not excluded from the drilldown interaction.
**Acceptance Criteria**:
- The chart's wrapping `div` has `role="img"` and a descriptive `aria-label` summarizing the data.
  - *Given* the data points from Story 5.2.1's example, *When* the chart renders, *Then* the wrapping `div`'s `aria-label` reads something equivalent to `"Cost by stage: work $14.30, review $1.20, triage $0.50"`.
- Each legend entry is keyboard-operable (`tabIndex={0}`, `role="button"`, Enter/Space triggers the same cross-filter as a click), matching `SessionsTable.tsx`'s existing pattern.
  - *Given* a legend entry for "work", *When* it receives keyboard focus and Enter is pressed, *Then* the same `onRoleClick("work")` handler fires as a mouse click would.
**Files**: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.4.1a: Add `role="img"`/`aria-label` to the chart's `chartWrap` div (~2 min)
- Files: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.4.1b: Add `tabIndex={0}`/`role="button"`/`onKeyDown` (Enter/Space) to each legend entry, mirroring `SessionsTable.tsx:233-238` (~4 min)
- Files: `web-app/src/app/insights/StageCostChart.tsx`

##### Task 5.4.1c: Accessibility-focused component tests (keyboard activation fires the same handler as click) (~3 min)
- Files: `web-app/src/app/insights/StageCostChart.test.tsx`

**Note on `ModelBreakdownChart.tsx`'s own pre-existing accessibility gap** (no `role`/`aria-label`, non-keyboard-interactive legend): per the task brief, fixing it is explicitly **out of scope** for this project — it is not a 1-line addition (it requires the same `onRoleClick`-equivalent wiring `ModelBreakdownChart` doesn't have a cross-filter target for today) and touching it risks an unrelated regression in an already-shipped, already-tested component. Filed as a candidate follow-up, not silently done here.
