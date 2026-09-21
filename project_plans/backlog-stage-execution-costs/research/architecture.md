# Architecture Research: Per-Stage Executor (Program+Model) and Cost Breakdown

Research Agent 3 (Architecture) — SDD Phase 2, `backlog-stage-execution-costs`.

Builds directly on `project_plans/backlog-configurable-pipeline/research/architecture.md` and
`implementation/plan.md` (the shipped `PipelineMode`/`PipelineEngine`/`pipelineModeCache` seam —
Phases 1-3 shipped per that plan's header), `project_plans/insights-cost-intelligence/research/architecture.md`
(findings/aggregation-loop precedent), and `ADR-029` (batched-snapshot pattern for joining
`SessionRole` into `insights_service.go`). All three are treated as settled; this doc only covers
where this project's scope extends them.

**Everything below is verified against the current shipped source**, not re-derived from the prior
docs' plans — `PipelineMode`'s 9 content-template fields, `CachingPipelineEngine`'s 5-method
interface, `pipelineModeCache`, and `ItemSession.pipeline_mode_snapshot(_hash)` are all confirmed
live in `session/pipeline_engine.go`, `session/ent/schema/pipeline_mode.go`, and
`session/ent/schema/item_session.go` — the prior plan's Phase 1-3 scope is done, not aspirational.

## 0. A pre-existing precedent this project should copy, not reinvent: `Workflow.Model`/`AgentType`

Before designing anything new, note that **program+model-per-entity, resolved into a
`--model`-flagged program string, already exists** for scheduled `Workflow`s — confirmed by direct
read, not inferred:

- `session/workflow_repository.go:46` (`WorkflowCreateInput.Model string`) and `:79`
  (`WorkflowUpdateInput.Model *string`) — a workflow already carries its own model choice,
  independent of any global default.
- `server/workflows/model_families.go:18-30` (`DefaultModelFamilies()`) defines a small
  `"opus"|"sonnet"|"haiku"` → concrete-model-ID alias table, overridable via a JSON file
  (`LoadModelFamilyOverride`, mirroring `session/tokens/pricing.go`'s override-file pattern);
  `:66-90` (`ResolveModel(families, model string) (string, error)`) resolves a stored
  `"family:sonnet"`-prefixed value to a concrete ID, fails closed on an unknown alias, and passes a
  bare (non-`family:`) value through unchanged so pre-existing literal model IDs never break.
- `server/workflows/scheduler.go:371-388` (`FireNow`) is the consumption site: resolves
  `wf.Model` via `ResolveModel`, then `program := wf.AgentType; if resolvedModel != "" &&
  (program == "" || program == "claude") { program = "claude --model " + resolvedModel }` — the
  resolved model is concatenated into the `CreateSessionRequest.Program` string, **not** passed as
  a separate structured field. `wf.AgentType` is the workflow's own program-choice field (its
  `Program`-equivalent, confirmed named `AgentType` not `Program` on this entity).
- `server/workflows/model_families.go:52-58`'s comment on `LoadModelFamilyOverride` confirms the
  validation reason this matters for this project directly: `ValidateModel` guards against
  shell-metacharacter injection because the resolved value is concatenated into a program string
  that eventually reaches a subprocess argv — **any new PipelineMode-level model field must run
  through the same `ValidateModel`/family-alias resolution, not a fresh ad hoc validator**, both to
  avoid re-deriving the injection defense and so a mode's model field and a workflow's model field
  share one alias vocabulary (a mode author typing `family:sonnet` should mean the same thing a
  workflow author typing it means).

This is the load-bearing precedent for §1c below (work-stage program+model → `Instance.Program`):
the mechanism to reuse is "resolve family alias → concrete ID → concatenate into a program string
consumed by `CreateSessionRequest.Program`/`SwitchProgram`", not a new parallel structured
model-selection pipeline.

## 1. Threading a per-stage executor override through the three layers

### 1a. `PipelineMode` schema/repository/proto — extend the existing per-stage sub-message, don't flatten

Confirmed current shape (`session/ent/schema/pipeline_mode.go:16-44`): 9 flat `string` fields, one
per rendered-content artifact (`status_command_template` … `initial_prompt_template`), plus
`slug`/`name`/`description`/`enabled`. There is **no existing per-stage grouping** — every field is
sibling-flat on the entity.

Recommendation (ties directly into §4's primitive-obsession disposition): introduce one new
sub-message/struct — `PipelineStageExecutor{ Program string; Model string }` — used **per stage**,
not one more pair of flat fields per stage tacked onto the existing 9:

- **ent schema**: ent has no native embedded-struct field type, so the sub-message becomes a
  `field.JSON("stage_executors", map[string]PipelineStageExecutor{})` column (ent supports
  `field.JSON` with a Go struct/map value out of the box — same mechanism already used for
  `AcCriteriaJSON` on `BacklogItemData`/`session/repository.go`, confirmed via
  `field.String("acceptance_criteria")`-plus-JSON-marshal convention... actually acceptance
  criteria is stored as a JSON-in-string column (`AcCriteriaJSON`), not `field.JSON` — **use that
  same string-column-holding-marshaled-JSON convention** for consistency with `ac_snapshot`
  (`session/ent/schema/item_session.go:41-43`) rather than introducing ent's separate `field.JSON`
  column type as a second JSON-storage convention in the same schema file. One new column:
  `field.String("stage_executors_json").Optional().Default("{}")`, keyed by stage role
  (`"triage"`, `"review"`, `"work"` — matching `ItemSession.session_role`'s existing 3-value
  vocabulary, `session/ent/schema/item_session.go:26`) → `{program, model}`.
- **repository**: `PipelineModeCreateInput`/`PipelineModeUpdateInput`
  (`session/pipeline_mode_repository.go`) gain `StageExecutors map[string]PipelineStageExecutor`
  (create) / `*map[string]PipelineStageExecutor` (update, partial-update nil-means-untouched,
  mirroring every other field's `*T` convention already established for Update inputs).
  `EntPipelineModeRepository` marshals/unmarshals the JSON string at the repo boundary — the same
  place `ParseAcCriteria`/`AcCriteriaJSON` marshaling already happens for acceptance criteria, so
  this doesn't introduce a new marshaling location convention, just a new field using the existing
  one.
- **proto**: `sessionv1.PipelineMode` (backing `pipelineModeToProto`,
  `server/services/backlog_service_pipeline_mode.go:46-76`) gains
  `map<string, PipelineStageExecutor> stage_executors = N;` (a real proto `map<string, Message>`,
  not JSON-string-on-the-wire — the JSON-string representation is an ent/DB storage detail, not a
  wire contract) with `message PipelineStageExecutor { string program = 1; string model = 2; }`.
  `CreatePipelineModeRequest`/`UpdatePipelineModeRequest` gain the same map field.
- **`no_ent_in_services` boundary** (requirements' Constraint): `backlog_service_pipeline_mode.go`
  is confirmed **already on the depguard exclusion list**
  (`.golangci.yml:118`, `- "!**/server/services/backlog_service_pipeline_mode.go"`) — a
  "temporarily excluded, removed-in: refactor/storage-interface-cleanup (P6)" grandfathered
  exception, not a sanctioned precedent for new code (confirmed by the file's own comment block
  at `.golangci.yml:96` and the surrounding files' identical annotation). **This project must not
  read that exclusion as license to keep piling ent-typed logic into that file.** The new
  `StageExecutors` marshal/unmarshal step belongs in `session/pipeline_mode_repository.go` /
  `session/ent_pipeline_mode_repository.go` (both already in the `session` package, ent-import-safe
  by design) so `pipelineModeToProto` in the excluded services file only ever touches the
  already-unmarshaled `map[string]PipelineStageExecutor` Go value handed back from the repository
  — no *new* ent-typed surface is added to the exemption, even though the file itself remains on
  it for pre-existing reasons.

### 1b. Headless call sites (`server/services/backlog_service_triage.go`,
`backlog_service_trigger_triage.go`) — thread through `CallOptions.Model`, confirmed currently unused

Confirmed by direct grep of every production `headlessPool.CallBlocking`/`CallWithOptions` call
site (`backlog_service_intent.go:47`, `approval_handler.go:534`,
`backlog_service_trigger_triage.go:470`, `backlog_service_triage.go:2875` (`TriggerReReview`'s
review call), `session_service.go:5439`, `session/autonomous_driver.go:330`,
`session/backlog_review.go:440,448`, `session/gate_custom_check.go:166`): **every one passes
`headless.CallOptions{}` or `{WorkDir: ...}` — `CallOptions.Model` (`session/headless/caller.go:30`)
is a field that exists on the struct but is set at exactly zero production call sites today.**
This means threading a per-stage model override through triage/review headless calls is a
**pure additive change to already-dead capacity**, not a new mechanism:

- `TriggerTriage`/`TriggerRetriage` (`server/services/backlog_service_trigger_triage.go:453-470`)
  and `TriggerReReview` (`backlog_service_triage.go:2875`) both already resolve `item` and (via
  Epic 1.5 of the prior project) call `s.pipelineEngine.TriagePromptFor(item, ...)` /
  `s.pipelineEngine.ReviewPromptFor(item, ...)` for prompt content. The natural extension: add a
  6th `PipelineEngine` method, `ExecutorFor(item *BacklogItemData, role string) (program, model
  string)`, resolved the same way the existing 5 methods resolve (`PipelineModeDefault` short-
  circuits to `("", "")` meaning "use the pool's existing hardcoded defaults" — zero regression by
  construction, same discipline as every other method), then at the call site:
  `program, model := s.pipelineEngine.ExecutorFor(item, "triage")` and pass
  `headless.CallOptions{WorkDir: triageWorkDir, Model: resolveModelFamily(model)}` — reusing §0's
  `ResolveModel`/family-alias resolution, not a new validator.
- **`headless.Pool` has no `Program` concept at all** — confirmed: `Pool`/`CallOptions`
  (`session/headless/caller.go`, `session/headless/pool.go`) always shells out to the `claude`
  binary (`findClaudeBinary`, `claudeFallbackDirs`); there is no `program` parameter anywhere in
  the package. **A per-stage `Program` override for triage/review headless calls therefore cannot
  be threaded through `CallOptions` today** — it requires the adapter work in §1's scope item
  ("investigate which programs can run headless with parseable cost") before it's meaningful. Model
  override is threadable now (`CallOptions.Model` exists, unused); Program override for headless
  calls is blocked on new adapter code (see §2 for the concrete gap).

### 1c. Interactive work-stage session creation — `Instance.Program`/`SwitchProgram`, following §0's pattern exactly

Work-stage sessions are created via `SpawnSessionFromItem` → `CreateWorktreeSession(...)` (per the
prior project's architecture.md §2/§3 trace, still accurate — `inst.Prompt` is built by
`PipelineEngine.InitialPromptFor`). `Instance.Program` (`session/types.go:362`) is a plain
`string`, set at creation and mutable later only via `SwitchProgram`
(`session/instance_program.go:59-103`, restart-triggering, history-porting-aware).

Recommended threading, mirroring §0's `FireNow` mechanism exactly rather than inventing a second
one:

- `PipelineEngine.ExecutorFor(item, "work")` returns the mode's configured `(program, model)` for
  the work stage.
- At `SpawnSessionFromItem`'s session-creation call (where `CreateWorktreeSession`'s `program`
  argument is assembled — today always a fixed default, since no per-item program choice exists
  pre-this-project), apply the identical `program := resolvedProgram; if resolvedModel != "" &&
  (program == "" || program == "claude") { program = "claude --model " + resolvedModel }`
  transform §0 already established for `Workflow.AgentType`/`Model` → `CreateSessionRequest.Program`.
  This is a straight copy of `scheduler.go:381-388`'s logic, not a new design — the two code paths
  (`FireNow` for scheduled workflows, `SpawnSessionFromItem` for backlog work-stage sessions) both
  ultimately construct a `program` string consumed the same way downstream (`Instance.Program`),
  so a shared helper (`session.ResolveExecutorProgram(agentType, modelAlias string, families
  map[string]string) (string, error)`, extracted from `scheduler.go`'s inline logic and called from
  both sites) is the concrete recommendation — avoids the two call sites drifting on shell-escaping
  or family-alias behavior independently.
- If the work stage is later reassigned mid-flight (not a v1 requirement per the requirements doc's
  scope, but worth naming since `SwitchProgram` already exists for it): `SwitchProgram`'s existing
  restart/history-port logic requires no change — it already accepts an arbitrary `rawProgram`
  string, so a pipeline-mode-driven program value is indistinguishable from a manually-chosen one at
  that layer.

## 2. Integration points with existing systems

| System | Current state (confirmed) | Integration point for this project |
|---|---|---|
| `session/pipeline_engine.go`'s resolution/fallback logic | `CachingPipelineEngine`'s 5 methods each: check `mode == PipelineModeDefault` → delegate to the pre-existing hardcoded function; else `cache.Get(slug)` → on miss, Warn-log and fall back to the same hardcoded function; else render. Zero exceptions to this shape across all 5 methods (`session/pipeline_engine.go:336-486`). | A 6th method, `ExecutorFor(item, role) (program, model string)`, must follow the **identical** three-branch shape: `PipelineModeDefault` → `("", "")` (pool/session-creation defaults apply, unchanged); unresolved slug → Warn-log + `("", "")` (never partially-resolved); resolved slug with no executor configured for that specific `role` (e.g. mode defines `work` but not `triage`) → also `("", "")` for that role specifically — a mode can override just one stage's executor without being forced to specify all three, matching the requirements' "deliberately run a cheap stage on a cheaper model" framing (opt-in per stage, not all-or-nothing per mode). |
| `headless.Pool`/`CallOptions` API | `CallOptions{WorkDir, Model, TimeoutSecs, AllowedTools, PermissionMode, DisallowedTools}` (`session/headless/caller.go:19-48`). `Model` exists, unused in production (confirmed §1b). No `Program` field; `Pool` is structurally single-program (`claude` binary only — `findClaudeBinary`). | `Model` is a same-struct, zero-new-field integration — just start populating it from `ExecutorFor`. `Program` is a **new-field-plus-new-adapter** integration, gated on §2's adapter feasibility spike (see below) — do not add a `CallOptions.Program` field until at least one non-Claude adapter is confirmed feasible, or the field becomes speculative surface with no real consumer (the exact `interface-pollution-checklist` smell the prior project's own Pattern Decisions table explicitly avoided when it kept `PipelineEngine` to 5, not 7, methods). |
| `Instance.Program`/`SwitchProgram` | Already a live, multi-program-aware mechanism — `session/instance_program.go` handles Claude↔Antigravity history porting and Claude-family-exit history clearing for *any* `rawProgram` string, and `Workflow.AgentType`/`Model` already drives it today via `FireNow` (§0). | No new mechanism needed for the *work* stage (interactive session) — see §1c. The gap is specifically **headless* (triage/review) execution on a non-Claude program, which has no `Instance`/session at all (headless calls are direct subprocess invocations via `headless.Pool`, not `Instance`-backed) — this is a structurally different code path from `SwitchProgram`, not a variant of it. |
| `server/services/insights_service.go`'s aggregation loop + `roleMap`/`SessionRole` | `roleMap := s.sessionRolesForSessions(ctx)` (`:240`) builds `map[sessionUUID]role` from one unfiltered `GetAllItemSessionsWithBacklogInfo` scan (ADR-029's batched-snapshot pattern), passed into `buildSessionSummary(...)` which sets `summary.SessionRole` (`:180`). The aggregation loop already keys `activityMap` by `ActivityType` and builds `ActivityCostBreakdown` (`:227`, `:315-321`) as a **structurally identical** pattern to what a role-cost breakdown needs. `sessionRolesForSessions` currently **discards** `ItemID`/`ItemTitle` from `ItemSessionBacklogEntry` (`session/repository.go:498-504`) even though the underlying query already returns them — confirmed by `:97-98`'s loop only assigning `roles[e.SessionUUID] = e.SessionRole`. | (a) Add a `roleMap`-sibling `activityMap`-style breakdown: `roleMap2 := map[SessionRole]*RoleCostBreakdown{}`, accumulated inside the same per-session loop (`:303-443`) right next to the existing `ab := activityMap[activityType]` block — **same loop, same request-scoped scan, no second pass**, directly answering §3's consistency requirement. (b) Extend `sessionRolesForSessions` (or add a sibling, `sessionMetaForSessions`) to also keep `ItemID`/`ItemTitle` per session UUID — this is the "drillable by backlog item name" requirement's data source, and the underlying query already has the data; today's function is just throwing 2 of its 5 struct fields away. |
| Non-Claude headless adapters (Aider/Gemini CLI) | **Zero existing code** — confirmed via repo-wide grep, no `aider`/`gemini` reference anywhere in `session/headless/` or the triage/review call sites. This is genuinely new territory, matching the requirements doc's own Rabbit Hole flag. | See below — this is the one integration point requiring real feasibility research, not just wiring. |

### Non-Claude headless adapter feasibility — architectural framing, not a verdict

The requirements doc correctly flags this as unconfirmed. Architecturally, the right seam is an
interface **narrower than `headless.Pool` itself**, so a feasible adapter doesn't have to
reimplement session-reuse/concurrency-semaphore/idle-timeout logic that's genuinely Claude-CLI-
specific (`session/headless/pool.go`'s session caching assumes `claude --resume <uuid>` semantics
that may not exist for Aider/Gemini at all):

```go
// A minimal seam a non-Claude adapter would need to satisfy — NOT proposed as a literal
// addition without the feasibility spike confirming at least one adapter can implement it.
type HeadlessCaller interface {
    CallBlocking(ctx context.Context, systemPrompt, userPrompt string, opts CallOptions) (result string, costUSD float64, priced bool, err error)
}
```

`priced bool` matters because of the requirements' own flagged rabbit hole (cost normalization —
`session/tokens/pricing.go` is Claude-specific): an adapter that cannot parse a cost from its CLI's
output must say so explicitly (`priced=false`), the same "abstain rather than guess" precedent
`insights-cost-intelligence/research/architecture.md` §5 already established for unpriced model
families — **do not silently record `$0.00`**, which would be indistinguishable from a genuinely
free/cached call and would silently corrupt the new per-stage cost breakdown (§3) with false zeros.
`CachingPipelineEngine.ExecutorFor`'s resolved `program` value is what selects which
`HeadlessCaller` implementation a call site uses; `PipelineEngine` itself does not need to know
about adapters, keeping the "5 (now 6) narrow methods, one resolve-and-render mechanism" shape the
prior project deliberately chose intact. **Recommendation: the plan phase should timebox a literal
spike — try to get `aider --message "..." --yes` (or Gemini CLI's documented non-interactive flag,
if one exists) to (a) run non-interactively to completion and (b) emit a machine-parseable cost or
token count on exit — before committing to build either adapter.** If neither is feasible within a
short spike, requirements' own fallback ("fall back to Claude headless otherwise, logged") is the
correct v1 scope, and `ExecutorFor`'s `program` return for a stage should simply be validated
against a small allow-list of adapters that actually exist (initially just `"claude"`), with any
other value logged as unsupported and silently coerced to `""` (default/Claude) rather than
attempted — same fail-closed discipline as an unresolved `pipeline_mode` slug.

## 3. Data flow and consistency — per-stage breakdown vs. existing totals

`GetInsightsSummary`'s loop (`server/services/insights_service.go:253-443`) is a **single pass**
over `s.store.GetAll()` that already computes, per session, `costUSD` once
(`summary.EstimatedCostUsd`, from `buildSessionSummary` at `:303`) and folds it into
`totalCostUSD` (`:347`), `dailyMap[bucketDay]` (`:357-388`), `modelMap[family]` (`:400-432`), and
`activityMap[activityType]` (`:315-321`) — four independent accumulators reading the **same**
per-session `costUSD` value. This is the concrete mechanism that already guarantees internal
consistency today: every accumulator sums the identical number, so `sum(daily[].cost) ==
sum(models[].cost) == sum(activityBreakdown[].cost) == totalCostUsd` **by construction**, not by a
separate reconciliation step.

A new `roleBreakdown` (or `stageBreakdown`) accumulator must be **the fifth accumulator in this
same loop, reading the same `costUSD` variable**, not a separately-computed value:

```go
rb := roleMap2[sessionv1.SessionRole(summary.SessionRole)] // or role string directly, matching existing SessionRole field type
if rb == nil {
    rb = &sessionv1.RoleCostBreakdown{SessionRole: summary.SessionRole}
    roleMap2[summary.SessionRole] = rb
}
rb.EstimatedCostUsd += costUSD
rb.SessionCount++
```

placed directly adjacent to the existing `activityMap` block (`:315-321`) in the loop body. This
guarantees `sum(roleBreakdown[].cost) == totalCostUsd` for exactly the same reason the existing
four accumulators already agree — same source value, same loop iteration, no second scan, no risk
of a role-breakdown RPC racing a totals RPC against different store snapshots (a real risk if role
breakdown were instead computed by a *second* call to `GetInsightsSummary` or a separate endpoint
hitting `s.store.GetAll()` at a slightly different instant).

**One real inconsistency risk, inherited from the existing design, not introduced by this
project**: `SessionRole` for a session with no matching `ItemSession` row (an ad hoc/non-backlog
session) is `""` (`roleMap` lookup miss, `buildSessionSummary`'s `roleMap[sessionID]` default
zero-value). A `stageBreakdown` keyed by role will therefore have a `""` ("no role") bucket
alongside `"triage"`/`"review"`/`"work"` — this must be surfaced in the UI as an explicit "not
backlog-attributed" bucket (mirroring how `activityMap` already has an analogous
`ActivityType`-unknown case) rather than silently dropped, or `sum(roleBreakdown[].cost)` would
under-count `totalCostUsd` by exactly the non-backlog sessions' cost, breaking the by-construction
consistency guarantee this design otherwise gets for free.

**Drilldown by backlog item name** (requirements' Scope item, "New Insights bar chart... drillable
by backlog item name"): per §2's table, `sessionRolesForSessions` must be extended to retain
`ItemID`/`ItemTitle` (already present on `ItemSessionBacklogEntry`, currently discarded). The
per-session loop already has `summary` in scope at the point the role-breakdown accumulator runs
(§3's code sketch above) — attach `ItemID`/`ItemTitle` to a nested map
(`map[role]map[itemID]*ItemCostEntry`) or a flat `repeated ItemRoleCost` list on the response,
whichever the plan phase's proto design prefers; either shape reads from data the loop already
computes in this single pass, so drilldown adds zero new store scans regardless of the exact
message shape chosen.

## 4. Tech debt disposition: `PipelineMode`'s 9-flat-field structure

**Refactor-first — but scoped narrowly: introduce the per-stage sub-message for the *new* fields
this project adds, do not migrate the 9 pre-existing content-template fields in the same change.**

Rationale: the prior project's own Pattern Decisions table explicitly named "9 separate typed
`string` columns" a deliberate choice (rejecting a JSON blob) *for content templates specifically*,
reasoned about *at that time* against the alternative of a single unstructured blob — it did not
anticipate a second, structurally different axis (per-stage *executor selection*, not per-stage
*content*) being added later. Flattening this project's 3 new stages × 2 fields (program, model) =
6 more flat columns onto the same entity would be the primitive-obsession failure mode the
requirements doc's own Rabbit Hole flag calls out: the entity would then have 9 content fields +
6 executor fields = 15 flat fields, with two semantically distinct field families
(content-template vs. executor-config) indistinguishable by name pattern alone at the Go-struct
call site. A `map[string]PipelineStageExecutor` sub-structure (§1a) keeps the *new* axis
self-describing (`stageExecutors["triage"].Model`) without touching the *existing*, already-shipped
and already-tested 9-field content-template shape — a smaller, lower-risk diff than a full
re-migration of both axes into one unified per-stage struct would be, and defers "should the 9
content fields also become per-stage-keyed" as a separate, independently-justifiable future
decision rather than bundling two refactors into one PR.

## 5. Event-Command-Policy Table (EventStorming grammar)

| Domain Event | Policy (Whenever…Then) | Command | Actor/System |
|---|---|---|---|
| A backlog item enters a pipeline stage (triage triggered, work session spawned, review gate runs) | Whenever a stage begins, then resolve that stage's configured executor from the item's `PipelineMode` before building the prompt/spawning the session | `PipelineEngine.ExecutorFor(item, role)` | `BacklogService`/`ReviewGateRunner` (system, synchronous, in-request) |
| `ExecutorFor` resolves a non-default `program` value for a **headless** stage (triage/review) that names an unsupported/unimplemented adapter (e.g. `"aider"` before an adapter exists, or ever if the feasibility spike concludes negatively) | Whenever a headless call's resolved program has no working `HeadlessCaller` implementation, then fall back to the Claude headless caller and log a Warn naming the item, stage, and the unsupported program | `HeadlessCaller` dispatch fallback (new, §2) | System (fail-closed, mirrors `CachingPipelineEngine`'s existing unresolved-slug fallback discipline) |
| A non-Claude headless adapter call completes but cannot report a parseable cost | Whenever a headless call returns with `priced=false`, then record the session's cost contribution as explicitly unpriced (not `$0.00`) for that stage | `RecordUnpricedHeadlessCall` (new, mirrors `ModelBreakdown.pricing_unavailable`/`SessionTokenSummary.unpriced_models`'s existing "abstain rather than guess" convention) | System |
| A backlog item's or a stage's accumulated cost crosses its configured soft-budget threshold | Whenever `GetInsightsSummary`'s (or a narrower per-item cost RPC's) computed cost for an item/stage exceeds its threshold, then surface a non-blocking warning — no stage/session is halted or refused | `EvaluateBudgetThreshold(item, role, thresholdConfig)` (new — **not** a re-use of `ProjectedCostCard`/`useProjectedCost`, which are confirmed global-monthly-only client-side computations with no per-item/per-stage scoping at all; see below) | UI (Insights dashboard / item detail panel) — soft warning is presentational, not gate-enforced, matching "soft" in the requirements |
| An operator edits a `PipelineMode`'s stage-executor config via `/settings/pipeline-modes` | Whenever a `PipelineMode` is Created/Updated, then invalidate `pipelineModeCache` synchronously so the next resolution sees the new executor config | `UpdatePipelineMode` → `invalidatePipelineCache` (existing mechanism, `server/services/backlog_service_pipeline_mode.go:98-106` — no new invalidation path needed, the new `StageExecutors` field rides the same cache-refresh `resolvedPipelineMode` snapshot) | Operator (via UI) → System |

**Note on the budget-warning row**: `ProjectedCostCard.tsx`/`useProjectedCost.ts` are confirmed
(direct read) to be a **global**, **monthly**, **client-side-only** projection computed from
`GetInsightsSummaryResponse.daily` buckets, with the threshold itself stored via a
hydration-guarded local input (`isHydrated` pattern, consistent with `localStorage`-only
persistence — no server RPC writes the threshold). This card has **no per-item or per-stage
awareness of any kind** — it cannot be parameterized into a per-stage/per-item warning by passing
it different props; it needs a genuinely separate component fed by genuinely different data (a
per-item or per-stage cost figure, sourced from §3's new breakdown, compared against a
server-persisted-or-configurable threshold since the whole point is per-item/per-stage granularity,
which a single global localStorage number cannot express). Recommend a new
`StageBudgetWarning`/`ItemBudgetWarning` component consuming the new role/item breakdown from §3,
not an extension of `ProjectedCostCard`.
