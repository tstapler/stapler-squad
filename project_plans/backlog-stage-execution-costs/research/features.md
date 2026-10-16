# Research: Feature Landscape — backlog-stage-execution-costs

Agent 2 (Features), SDD Phase 2. Scope: per-stage program/model configuration
patterns in comparable systems; edge cases/failure modes; unstated user needs.

## 0. Codebase grounding (read before the external-pattern section — it changes the answer)

- `PipelineMode` (`session/ent/schema/pipeline_mode.go`) already exists from
  `project_plans/backlog-configurable-pipeline/`. Its 9 fields are all
  **content templates** (`status_command_template`, `triage_prompt_template`,
  etc.) — there is no `program`/`model` field on the row today. Adding
  per-stage program/model is a schema extension to an *already-DB-persisted,
  already-cached* config object, not a new subsystem. Reuse its cache
  invalidation and `ContentHashFor`/fail-closed-to-default machinery
  (`docs/adr/` for this project, referenced in that project's requirements.md)
  rather than inventing a second config path.
- `headless.CallOptions` (`session/headless/caller.go:19-48`) already has a
  `Model string` field, forwarded end-to-end through `CallWithOptions` →
  `acquireSession`'s `effectiveModel := model; if effectiveModel == "" {
  effectiveModel = p.cfg.DefaultModel }` (lines 190-193). **The override
  plumbing this project needs for headless calls already exists** — the gap
  is purely "what sets `CallOptions.Model` for a given call," i.e. resolving
  it from the item's `PipelineMode` at the `TriggerTriage`/review-gate call
  sites, not building new plumbing.
- Headless execution today is **`claude`-only**: `session/headless/client.go`
  shells out to a hardcoded `claude` binary (`findClaudeBinary`,
  `claudeFallbackDirs`); there is no `aider`/`gemini` headless caller anywhere
  in `session/headless/`. `Instance.Program` (interactive work-stage sessions,
  `session/instance.go:208`) is a free-form string already supporting
  `claude`/`aider`/`gemini`/others via `session/detection/binaries/{claude,aider,gemini}.go`
  and `session/agy_adapter.go`, but headless (triage/review) is a separate,
  narrower code path with no program abstraction at all. This confirms the
  requirements doc's scope item "investigate which programs can run headless
  with parseable cost" is real, unsolved work, not a config wiring exercise —
  see §2 below for the concrete unknowns.
- Cost aggregation by a categorical dimension **already has a direct,
  working precedent**: `ActivityCostBreakdown` (`proto/session/v1/insights.proto:166-172`)
  aggregates `estimated_cost_usd`/`session_count` keyed by `ActivityType`,
  computed inline in `InsightsService`'s per-session loop
  (`server/services/insights_service.go:311-321`) from the same
  `summary := buildSessionSummary(...)` call that already resolves
  `summary.SessionRole` via ADR-029's batched `roleMap`. A `StageRoleCostBreakdown`
  (or similar) keyed by `SessionRole` instead of `ActivityType` is a
  ~15-line addition to that same loop, not a new join or a new aggregation
  pattern — `ModelBreakdownChart.tsx`/`ActivityBreakdownTable.tsx` are the two
  existing frontend shapes (bar chart vs. table) to choose between or mirror.
- "Drillable by backlog item name" is *also* close to free: the same batched
  query the role map comes from, `GetAllItemSessionsWithBacklogInfo`
  (`session/ent_repository_backlog.go:2786`, `session/repository.go:497-502`
  `ItemSessionBacklogEntry{SessionUUID, SessionRole, ItemID, ItemTitle}`),
  already carries `ItemID`/`ItemTitle` alongside `SessionRole` in one query.
  Today only `SessionRole` is threaded into `buildSessionSummary`'s `roleMap`;
  extending that map's value type (or adding a sibling map) to also carry
  `ItemID`/`ItemTitle` reuses ADR-029's exact batching decision instead of a
  new live join — the ADR's ADR-029 rationale (avoid N+1 on `GetInsightsSummary`/
  `ListSessionTokens`/`watchInsights`) applies identically here.
- **Soft budget warning already exists, but only at one granularity**:
  `ProjectedCostCard.tsx` (`web-app/src/app/insights/ProjectedCostCard.tsx`)
  implements a single global monthly-projection threshold — `threshold`,
  `warningText`, `budgetInput` props, `isWarning = projection.projectedMonthly
  > threshold`. It has **no per-stage or per-item threshold concept** and no
  concept of "warn now, mid-item" — it only warns against a monthly
  *projection* computed from historical daily spend. This project's "soft
  budget warning for a stage or item" is a materially different feature
  (real-time, scoped to one item/stage, not a monthly trend projection) that
  should probably be a sibling component, not an extension of this card —
  but should visually/behaviorally rhyme with it (same warning-color
  language, same non-blocking soft-warn philosophy) since a user will see
  both on the same dashboard.
- `ModelBreakdown.pricing_unavailable` (`proto/session/v1/insights.proto:161-163`,
  "true when total_input_tokens/total_output_tokens > 0 but no PricingTable
  entry exists for model_family") and `ProjectedCostCard`'s
  `hasUnpricedUsage`/"Projection excludes unpriced usage" caveat are the
  existing UX pattern for "we know spend happened but can't price it" — this
  is exactly the shape needed for a new aider/gemini headless adapter whose
  cost-parsing is missing or wrong (§2). Reuse this caveat pattern rather than
  inventing a new one, and reuse `session/tokens/pricing.go`'s
  `unpriced`/`unpricedSet` return-shape convention (ADR-001-unpriced-signal-return-shape.md).

## 1. Patterns from comparable systems

| System | Unit of work | How executor/model is scoped | Notable mechanism |
|---|---|---|---|
| **GitHub Actions** | job (within a workflow) | `runs-on:` per job, plus a `strategy.matrix` to fan a job out across multiple runners/OS/versions | Job-level scoping is the closest analog to "per-pipeline-stage program": each job is an independent unit that can declare its own runner image/labels, and later jobs `needs:` earlier ones for sequencing — directly comparable to `PipelineMode`'s per-stage template fields wanting a per-stage `program`/`model` pair. |
| **GitLab CI** | job | `image:` (container image) per job, overridable per-job even within one `.gitlab-ci.yml`; `tags:` select which runner (with what hardware/software) picks up the job | Runner *tags* are a closer analog to routing by capability ("this runner has GPU," "this runner has `aider` installed") than to routing by cost/quality — relevant to the "program not installed on this machine" edge case in §2: GitLab's answer is the job simply sits unpicked-up/stalled until a matching runner appears, which is a cautionary "silent hang," not a good failure mode to copy. |
| **LangGraph** | graph node | No first-class per-node model field; achieved by closing over a different model client per node function, or via conditional edges routing to different nodes that each call a different model (paired with LiteLLM for multi-provider routing) | The "per-node" granularity maps directly onto "per-pipeline-stage" here. The lack of a first-class field (it's just "call whatever client this node's closure captured") is a useful negative precedent: LangGraph treats model choice as *code*, not *config* — this project deliberately chooses the opposite (DB-persisted `PipelineMode` fields), consistent with the parent project's Runtime Configurability Decision. |
| **Temporal** | activity | Task-queue-based routing: each activity type/invocation specifies a `TaskQueue`, and only workers polling that queue execute it; "worker affinity" patterns exist for routing a sequence of activities to the *same* worker (e.g. GPU-bound work) | This is the most mature version of "assign a different executor per stage" in the survey — task queues decouple *what work needs doing* from *which worker does it*, and a worker can poll multiple queues. Closest analog for this project would be: each `PipelineMode` stage names a "capability" (queue) rather than a literal program binary, and whichever machine/runner has that program installed is the one that picks it up. That's a heavier design than this project's single-machine, single-operator scope needs, but the queue/capability *separation* is worth keeping in mind if stapler-squad ever runs across multiple hosts. |
| **Dagger** | pipeline function/module call | Each function call can specify its own container image and resource requests, resolved at execution time from the module's code, not a separate scheduling layer | Similar to GitLab's `image:` — closely coupled to containerization, less relevant to stapler-squad's non-containerized tmux/subprocess model, but reinforces that "per-stage image/binary" is a well-worn, unremarkable pattern across CI/CD tools generally. |

**Cross-cutting pattern**: every system above separates *declaring* the
executor choice (a static, versioned/reviewable config: YAML job definition,
graph node closure, task-queue name) from *resolving* it at run time (runner
matching, worker polling, model client construction). `PipelineMode`'s
existing DB-row-plus-cache design already has this separation — the schema
extension in this project is additive to a pattern already proven for
content templates.

Sources: [LangGraph 2026 guide](https://futureagi.com/blog/what-is-langgraph-2026/), [Temporal Task Routing docs](https://docs.temporal.io/task-routing), [Temporal worker affinity blog](https://temporal.io/blog/task-queue-worker-affinity), [Temporal community: separate task queue per activity](https://community.temporal.io/t/separate-task-queue-for-each-activity/6232).

## 2. Edge cases and failure modes

1. **Configured program not installed on this machine.** GitLab CI's answer
   (job sits unpicked-up, silently stalled) is explicitly a bad model to
   copy. stapler-squad already has a fail-closed-and-loud convention for
   exactly this shape of problem: `NewPool`/`findClaudeBinary` returns
   `ErrClaudeNotFound` today when `claude` itself is missing, and
   `session/detection/binaries/{aider,gemini}.go` already do install-probing
   for interactive `Instance.Program` switches. A per-stage program override
   should resolve/validate at the point a session/headless call is about to
   start (not at `PipelineMode` save time, since save time is a different
   machine's editor session) and fail closed to the pipeline's default
   program with a Warn log — mirroring `backlog-configurable-pipeline`'s own
   Risk Control precedent ("an unresolvable/malformed mode on a live item
   must fail closed to `DefaultPipelineEngine` behavior with a loud Warn
   log, never a silent no-op or a crash").
2. **User changes a stage's program/model mid-flight while a session for
   that stage is already running.** `PipelineMode` content is already
   snapshotted at triage-session start via `ContentHashFor` (per that
   project's plan.md Pattern Decisions) specifically to protect against
   "mode content changed after the item started." A program/model field
   added to the same row should be covered by the *same* snapshot/hash
   mechanism, not a new one — an in-flight session keeps the program/model it
   started with; only the *next* stage/session picks up a live-edited value.
3. **A new headless adapter's cost-parsing is wrong or missing (unpriced).**
   The `claude -p --output-format stream-json` path parses `total_cost_usd`
   from a well-known JSON envelope (`session/headless/client.go`'s
   `firstCallJSONResult`). `aider`/`gemini` have no such envelope today, and
   their eventual cost source (if any) is almost certainly a different shape
   (e.g. token counts requiring a client-side pricing-table lookup, matching
   `session/tokens/pricing.go`'s existing `EstimateCost`/`unpriced` pattern,
   rather than a self-reported dollar figure). The existing
   `ModelBreakdown.pricing_unavailable` / `ProjectedCostCard`'s
   "Projection excludes unpriced usage" caveat is the right UX precedent:
   an adapter that can't produce a trustworthy cost must surface as
   *unpriced*, never as *zero* — a silent zero would make a stage look free
   and could make the new soft-budget-warning feature (success metric 3)
   actively misleading by undercounting spend right when it matters most.
4. **Budget threshold crossed exactly on the response that completes the
   stage.** Cost is only known after `CallBlocking`/`drainChannelWithCost`
   returns (`CostSink` is invoked post-hoc with the final `total_cost_usd`)
   — there is no pre-flight cost estimate anywhere in the headless path
   today. This means a threshold check can only ever be a **post-call,
   next-call-blocking (or purely advisory) warning**, never a mid-call
   circuit breaker — the call that crosses the threshold always completes
   and is paid for. This must be stated explicitly in the design (it's
   consistent with "soft" in the requirement's "soft budget warning," and
   with Out-of-Scope's "no hard enforcement") rather than left implicit,
   since a naive reading of "budget warning" could wrongly imply
   mid-response cancellation.
5. **A stage's program override is set but the model override is left
   blank (or vice versa).** `headless.CallOptions.Model == ""` today falls
   back to `p.cfg.DefaultModel` (`acquireSession`'s `effectiveModel`
   fallback) — the per-stage config should follow the identical "empty
   means inherit the pool/global default" semantics rather than requiring
   both fields to be set together, matching the existing optional-field
   convention `backlog-configurable-pipeline`'s Constraints section already
   mandates for `BacklogItemData`.
6. **Interactive work-stage `Instance.Program` override interacts with
   `SwitchProgram`'s existing cross-program guards** — `session/instance_program.go`'s
   `isClaudeAntigravityCrossSwitch` shows there are already known-unsafe
   program transitions (e.g. history-transfer incompatibility, per the
   Antigravity parity-gap memory). A `PipelineMode`-driven program for the
   work stage must go through the *same* `SwitchProgram` validation path a
   manual user switch would, not bypass it via a raw field write — otherwise
   this feature reintroduces a class of bug `SwitchProgram` was built to
   prevent.
7. **Prompt-cache loss on a mid-item program/model switch** (see §3) is not
   strictly a "failure," but the cost dashboard should not silently attribute
   the resulting higher token count to "the model is expensive" without
   context — see unstated need below.
8. **`SessionRole` inheritance for the new cost-by-stage aggregate**: per
   ADR-029, `session_role` is set once at `ItemSession` creation and never
   mutated; a session's role reflects the stage it was *created* for, and
   never reassignment. If a future change routes a resumed/reused session
   across stages (e.g. resuming a triage-flavored headless session for a
   review call), the stage-cost attribution would misattribute cost to the
   wrong stage — worth a design note even if it's not a live problem today
   per ADR-029's finding of "no production path ever calls Update on
   session_role."

## 3. Unstated needs

- **"What actually ran" audit trail, not just "what was configured."**
  `PipelineMode` already tracks *content* provenance via `ContentHashFor`
  for drift protection — the same idea should extend to program/model: when
  a stage session starts, log (and ideally persist, e.g. on
  `ItemSessionData`) the *resolved* program/model actually used, not just
  the mode's current configured value. Config can be edited after the fact
  (edge case 2); a user debugging "why did this item cost $40" needs to see
  what ran *then*, not what the mode says *now*. This is the direct
  extension of the existing "what ran" read-only surface in
  `BacklogItemDetail.tsx` that `backlog-configurable-pipeline` already
  shipped for skill/command content — program/model is the same class of
  fact and belongs on the same surface.
- **"Was this expensive item's cost due to model choice or usage volume?"**
  Today `ModelBreakdownChart` answers "which model family cost the most
  overall" and (with this project) a new stage/role chart would answer
  "which stage cost the most," but neither answers the *cross* question a
  user actually wants when looking at one expensive item: cost-per-stage
  broken down *by model*, for that one item specifically. The existing
  "drillable by item name" success metric gets partway there structurally
  (same `ItemID`/`ItemTitle` join as §0), but the UI should let a user pivot
  a single item's stage-cost bars by model family (stacked bar: stage on
  the x-axis, model family as the stack segments) rather than only offering
  a single dimension at a time — otherwise "expensive model vs. heavy usage"
  still requires manually cross-referencing two separate charts.
- **Prompt-cache economics tradeoff should be surfaced, not just accepted.**
  Switching model/program per stage means Anthropic's prompt cache (and any
  equivalent for other providers) cannot carry forward between stages —
  each stage's headless call already starts a fresh session
  (`acquireSession`'s per-`FeatureKey` session rotation is *already*
  per-stage-scoped in practice, since triage/review are different
  `FeatureKey`s) so this cost is already partially paid today even without
  per-stage model choice. What's new is that a deliberate model *change*
  between two stages that previously shared a model family removes any
  chance of coincidental cache reuse a user might have been getting. This
  should be a one-line callout in the `/settings/pipeline-modes` UI near the
  per-stage model picker (e.g. "different models per stage means no prompt
  cache carries over between stages") rather than a silent behavior change a
  user discovers later from a cost spike.
- **Default/inherit affordance in the UI, not just a free-text model
  field.** Given edge case 5 (empty means inherit) and the existing
  `PipelineModeForm.tsx` pattern of labeled textareas per template field,
  the per-stage program/model UI should visually distinguish "explicitly
  set to X" from "inherits the global default," mirroring how the
  `enabled` toggle and slug-immutable-on-edit affordances already
  communicate state — an unlabeled blank text input reads as "no model,"
  not "default model," which is a likely source of user confusion once
  overrides exist for some stages but not others.
