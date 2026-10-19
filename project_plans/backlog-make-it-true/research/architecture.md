# Architecture Research: backlog-make-it-true

Scope: wire `ConfiguredWorkflowEngine` into `BacklogService` via shadow mode then enforce; place quota/soft-budget settings and `allow_auto_merge` in the layering. Builds on (cited, not re-derived): `project_plans/backlog-custom-workflow-stages/research/architecture.md` (call it CWS-arch), its `implementation/plan.md`, `docs/adr/013-workflow-engine-replaces-valid-transitions.md`, `docs/architecture-audit-session-backlog-extraction-design-2026-09-07.md`, `project_plans/quota-aware-backlog-gating/research/architecture.md` (QAG-arch). Evidence labels: VERIFIED = file read or command run this session; UNVERIFIED = inferred.

## 0. Findings that change the plan (read first)

1. **The seed is graph-only; the Default engine's gates are not in it. VERIFIED.** `EnsureBuiltInWorkflowStages` creates 9 `BacklogStage` rows and one `StageTransition` per `domain.ValidTransitions()` edge, and nothing else (`session/workflow_stage_seed.go:72-135`). The only non-test `TransitionGate.Create` is the CRUD path (`session/ent_stage_config_repository.go:429`). Meanwhile `DefaultWorkflowEngine` runs `TransitionGuard`, which returns `ErrACRequired`, `ErrPlanRequired`, `ErrPlanArtifactsRequired/Missing`, `ErrUnresolvedBlockers`, `ErrVerdictClearRequiredForReady`, plus `ErrVerdictRequired`/`ErrCodeNotOnMain` for review/pr_pending to done (`session/domain/backlog.go:599-606`, switch at ~:640-700). `ConfiguredWorkflowEngine.PendingGates` returns `nil, nil` for an edge with zero gates (`session/configured_workflow_engine.go:165-180`). **A literal engine swap would silently drop every built-in guard.** Requirement "seed config byte-for-byte equivalent to validTransitions" is necessary but not sufficient; the equivalence test must cover gates too.
2. **The structural-check vocabulary cannot express the built-in guards. VERIFIED.** The sealed set is `ac_complete`, `pr_green` (fail-closed, not wired), `no_open_blockers` (`session/gate_structural.go:14-45`). There is no check for plan approval, plan-artifacts path, PASS verdict, code-on-main, or verdict-clear-for-ready. Seeding gates would mean adding ~5 new check IDs *and* feeding them inputs `BacklogItemTransitionInput` already carries (`OverallOutcome`, `HasUnshippedCode`, `HasUnresolvedBlockers`, `PlanArtifactsPathMissing` are set by the service, `server/services/backlog_service_lifecycle.go:925-932`).
3. **Production already half-uses the configured engine. VERIFIED.** `backlogLifecycleListener.SetWorkflowEngine(stageConfigEngine)` (`server/dependencies.go:1544`) feeds `transitionHasAutomatedReviewGate` / custom-check resolution (`session/backlog_lifecycle.go:1092,1116`). So the listener reads configured gates while `BacklogService` enforces default gates: the two disagree today if an operator edits gates. Requirements.md's "used only for InvalidateCache" is stale on this point (that is the `BacklogService` side only, `dependencies.go:1477-1487`).
4. **Enforcement is not centralized. VERIFIED.** Engine consultation sites: `TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:892,933`), a second `CanTransition` at `:1318`, triage `:683` (`backlog_service_triage.go`), sync `backlog_service_sync.go:157`, `GetPendingGates` (`backlog_service_gates.go:128`), proto allowed-transitions (`backlog_service.go:1048,1148`). **Bypass:** MCP tools call `storage.TransitionBacklogItemStatus` directly (`server/mcp/tools_backlog.go:1507,1515,1682,1821`), as does the lifecycle listener (`session/backlog_lifecycle.go:1852-1855` comment confirms the raw storage layer "has no knowledge of WorkflowEngine/TransitionGuard"). The storage method is a pure CAS write with no graph check (`session/ent_repository_backlog.go:1727+`). Only `tools_backlog_dependencies.go:163` goes through the service. These agents-driven paths are exactly the autonomous transitions, so **graph config is partly fiction for them regardless of engine choice** unless a shared guard seam is added (answers requirements Open Question 1, partially: MCP and lifecycle bypass; UNVERIFIED whether each MCP call site is intentionally exempt, e.g. `request_review`/`report_pr_created` are allowlisted by literal status at `tools_backlog.go:210-239` per CWS-arch §4).
5. **Config-load failure behavior (Open Question 2). VERIFIED.** `NewConfiguredWorkflowEngine` never fails on a bad `cache.Load`: it logs Warn and continues with an **empty cache** (`configured_workflow_engine.go:78-85`). Empty cache means `CanTransition` is false for every edge (no stage, no fallback) and `PendingGates` is `nil, nil`: fail-**closed on graph, fail-open on gates**. It does **not** fall back to the default graph. The wiring site also leaves `stageConfigEngine` nil if construction errors (`dependencies.go:773-778`). The fallback requirement in requirements.md scope A is therefore new work, not existing behavior.
6. **The seed only runs when `backlog_stages` is empty, and `ListAll` ordering/enabled flags matter. VERIFIED (seed); UNVERIFIED (disabled-stage semantics).** Once any row exists the seed is a no-op (`workflow_stage_seed.go:80-85`), so a seed fix (adding gates) does not reach existing installs; it needs a versioned, create-if-missing-per-edge migration, or the guards stay outside the config (see §2, recommended).

## 1. Disposition: Isolate via a seam (with one small refactor-first prep)

**Recommendation: Isolate via seam.** Do not refactor `TransitionBacklogItemStatus` first, and do not "extend as-is" by passing `stageConfigEngine` at `dependencies.go:1477`.

Reasoning:

- **Extend as-is is unsafe** because of Findings 1, 2 and 5: the one-line swap changes behavior for every item (drops guards; empty cache blocks all edges).
- **Refactor-first is disproportionate.** `TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:870`, in a 1,465-line file; complexity ~41 per requirements, UNVERIFIED by me: no gocyclo run) consumes the engine through exactly two narrow calls, `CanTransition` (`:892`) and `ValidateGates` (`:933`). A seam *below* those calls leaves the handler untouched, so the hotspot's complexity does not grow. The new-code gates (`gocyclo/gocognit/funlen` via `--new-from-rev`, `.golangci.yml:25-40`) only flag lines in the diff, so not touching the handler also keeps this out of the gate's blast radius.
- **The seam already exists.** ADR-013 made `WorkflowEngine` the injection point and `NewBacklogService` takes it as a parameter (`backlog_service.go:582`, defaulting to `NewDefaultWorkflowEngine()` at `:584`). The change is a new `WorkflowEngine` implementation, constructed in `dependencies.go`, not a change to `BacklogService`.
- **Small prep refactor (optional, own commit):** extract the guard-input assembly block (`backlog_service_lifecycle.go:897-932`: verdict load, `hasUnshippedCode`, `hasUnresolvedBlockers`, `PlanArtifactsPathMissing`) into a helper. `GetPendingGates` already has a near-duplicate, `buildPendingGatesGuardInput` (`backlog_service_gates.go:148-170`). Merging them removes duplication that the `dupl` gate would otherwise catch once the shadow comparison needs the same input twice, and is the only place the handler's complexity can fall. Mark as a follow-up unless the plan wants the hotspot score down; it is not a precondition.
- **`no_ent_in_services` is orthogonal.** 16 non-test `server/services` files import `session/ent` (VERIFIED: `grep -ln 'session/ent"' server/services/*.go`, excluding tests), of which `backlog_service_lifecycle.go`, `_query.go`, `_triage.go` are on the exclusion list (`.golangci.yml:228-241`). The seam lives in `session/` and `server/dependencies.go`, so it adds **no** new ent import to `server/services`. Constraint for the plan: do not add `ent` usage to any touched services file; if the handler is edited for the extraction above, do not remove its exclusion in the same PR (out of scope per requirements).

### Proposed shape

```
session/workflow_engine_selector.go   (new; package session)
  type SwitchableWorkflowEngine struct {
      def        *DefaultWorkflowEngine
      cfg        *ConfiguredWorkflowEngine      // may be nil -> permanently Default
      mode       func() WorkflowEngineMode      // live read: off | shadow | enforce
      shadow     ShadowRecorder                 // counters + repro sink
  }
  // implements WorkflowEngine (4 methods, fallback param passed through)
```

Behavior per method:

| Mode | `CanTransition` / `PendingGates` / `ValidateGates` / `AllowedTransitions` |
|---|---|
| `off` (default until shadow evidence) | Default only. Configured engine never consulted. |
| `shadow` | Default answers and is returned. Configured is evaluated on the same inputs; difference is counted and logged. Never alters return value or error (same invariant as `docs/how-to/flip-git-backend-cohort.md` intro). |
| `enforce` | Graph from Configured; **guards = Default's `TransitionGuard` AND Configured's gates** (see §2). If Configured is nil or its cache is empty/unloaded, fall back to Default and count `fallback_total{reason}`. |

Construction: build `SwitchableWorkflowEngine{def: workflowEngine, cfg: stageConfigEngine, ...}` after `stageConfigEngine` exists (`dependencies.go:~773-780`) and pass it at `:1477` in place of `workflowEngine`. `cfg` nil is handled inside the type, so no typed-nil boxing hazard at the call site (the guard at `:1480-1487` and `:1543-1545` exists because of exactly that pitfall).

## 2. Where the built-in guards live (decision needed in planning)

Two options; recommend **A**.

- **A. Guards are invariants, not config (recommended).** In `enforce`, the selector composes: `CanTransition` from Configured graph; `PendingGates` = Default's structural `TransitionGuard` gate (for built-in `(from,to)` pairs) **plus** Configured's operator gates. `TransitionGuard` stays code. Rationale: operators editing the stages UI should be able to add/remove stages, edges and *extra* gates, not delete "plan must be approved before work" or "PASS verdict before done"; those protect against the incidents in BUG-026 and the plan-gate work. Cheapest, no migration, and `DefaultWorkflowEngine.PendingGates` already returns them as `GateStatus` (`workflow_engine.go:~77+`). Cost: a UI that shows "Gates" for a built-in edge must render the union (the `GetPendingGates` RPC already does, because it calls `engine.PendingGates`).
- **B. Seed built-in guards as `TransitionGate` rows.** Needs ~5 new `StructuralCheckID`s, a per-edge versioned migration (the seed no-ops once rows exist, Finding 6), and still keeps non-DB inputs (verdict/PR) flowing through `BacklogItemTransitionInput`. Gives full operator control including footguns. Larger and riskier; UNVERIFIED value.

Either way the equivalence test (Success Metric 1 / Risk Control) must assert: for every `(from,to)` in the 9x9 matrix and a fixed corpus of item fixtures, `Default.{CanTransition,AllowedTransitions}` == `Configured.*` on the seeded config, and `PendingGates` unsatisfied-set equal under option A's composition.

## 3. Shadow-mode design (mirrors `docs/how-to/flip-git-backend-cohort.md`)

| Element | Design |
|---|---|
| **Comparison point** | Inside `SwitchableWorkflowEngine`, at the engine boundary. Pure, in-process, no I/O: the Configured side reads an atomic cache snapshot (`stageConfigCache`), so shadow adds no DB round trip (meets the NFR "no per-call DB round trip"). Compare `CanTransition` bool; `AllowedTransitions` set; and for gates the sorted set of unsatisfied `GateStatus.GateID/Kind` plus the first error identity (`errors.Is` against the `Err*` sentinels). Do **not** compare `Description` strings. |
| **What is compared per call** | Same `(from, to, item, fallback)` handed to both. `fallback` is the item's `StageConfigSnapshot` (`BuildStageConfigSnapshotFallback`), so deleted-stage behavior is compared too. |
| **Divergence classes** | `graph` (CanTransition/AllowedTransitions differ), `gate_missing` (Default blocks, Configured passes: the dangerous one), `gate_extra` (Configured blocks, Default passes: expected once an operator adds gates), `error_kind`, `cache_empty`. Only `gate_missing` and `graph` on a default-seeded config count toward the alert condition in requirements ("any divergence >0 on default-seeded config is a bug"). To know "default-seeded", compare the live config to the seed fingerprint (hash of the seeded edge set) and label `config=seeded|edited`. Divergences under `edited` are expected and reported, not alarmed. |
| **Counters** | `workflow_engine_shadow_calls_total{method}` (denominator), `workflow_engine_shadow_divergence_total{method,class,config}`, `workflow_engine_fallback_total{reason}` (cfg nil / cache empty / load error), `workflow_engine_enforce_denied_total{source=config_graph|gate|default_guard}` (satisfies the requirements' "config-denied vs gate-failed" log/count split). In-memory plus a once-a-minute ledger file like `git-backend-soak.json` so a restart does not reset the window (pattern: `session/gitwiring/soak.go:207` `SoakReport`). |
| **Exposure** | `GET /actuator/workflow-engine[?format=text]`, registered next to `/actuator/git-backend` (`server/actuator.go:31`), plus a `make workflow-engine-soak-status` target modeled on `make git-soak-status` (exit 0 ready / 1 not ready / 2 unreachable). |
| **Repro records** | On `graph`/`gate_missing` divergence write a redacted record (item id hash, from, to, both answers) to `<config dir>/workflow-shadow-mismatches/`, capped (200 per run) as the git soak does. Item content is not stored. |
| **Flip criteria** (for the how-to doc; all must hold) | window >= 7 days (INFERRED: copy the git soak; an owner decision for this lower-traffic path, tens of items) **and** >= N shadowed `TransitionBacklogItemStatus` calls on every `(from,to)` edge that is actually used (edges never exercised hold the window open, as in the git doc); `gate_missing`=0 and `graph`=0 under `config=seeded`; `cache_empty` fallbacks = 0 in steady state; equivalence test green in CI. Verdict never changes configuration. |
| **Flag scoping** | `knownFeatureFlags` (`server/services/feature_flag_service.go:172`) already supports `defaultValue` and `scopes` (today only `gateFlagScopes()` uses scopes, `:299`; what a "scope" key means for a non-gate flag is UNVERIFIED, see `config.FeatureFlagScopes`, `config/config.go:375`). Two bool flags fit the existing bool-only model: `backlog:workflow-engine-shadow` (default **on**, since it is read-only and the evidence source) and `backlog:workflow-engine-enforce` (default **off**). Mode = `enforce` if enforce flag, else `shadow` if shadow flag, else `off`. Read live per call via `config.LoadConfig()` like `defaultPipelineModeForNewItem` (`backlog_service_lifecycle.go:150`) so no restart. Per-scope override: the natural scope is the **from-stage** (`stage:<slug>`), allowing enforcement on one edge group (e.g. only `idea`/`refining`) first; this needs a small extension to the scope vocabulary (UNVERIFIED how scopes are validated). If scopes are too heavy, ship global-only first; requirements say "global + per-scope" so keep it as a stated plan item. Never an env var (memory: rollout flags live-settable). |
| **Rollback** | Flip `enforce` off (live). Revert one line at `dependencies.go:1477` is the code-level fallback. No data migration to undo; the stage tables stay inert. |
| **Cost control** | `LoadConfig()` re-reads from disk each call (as the existing flag reads do; the quota gate does the same, `dependencies.go:1429-1436`). On the transition hot path cache the resolved mode for ~1s (atomic) to avoid a disk read per transition and per `AllowedTransitions` call made for every item in `ListBacklogItems` (`backlog_service.go:1048`). UNVERIFIED that LoadConfig is cheap; measure before choosing the TTL. |

## 4. Closing the bypass paths (Finding 4)

Two tiers, chosen in planning:

- **Tier 1 (in scope, cheap):** route every *service-level* check through the selector (already true: all `s.engine.*` sites, listed in Finding 4). `GetPendingGates`/`AllowedTransitions` automatically follow the mode.
- **Tier 2 (decision):** MCP tools and lifecycle auto-transitions call `storage.TransitionBacklogItemStatus` directly. Two ways to cover them: (a) leave them as deliberate "system" transitions exempt from operator graph edits and document that exemption on the settings page ("stage edits govern UI/API transitions; automated lifecycle edges are fixed"); or (b) add a `session.TransitionPolicy` check (a narrow interface satisfied by the selector) called from the MCP handlers before `storage.TransitionBacklogItemStatus`. (b) touches `server/mcp/tools_backlog.go` (large; ent usage there UNVERIFIED) and the `silenttransition` linter's expectations (`tools/lint/silenttransition`). Recommend (a) for this project plus a shadow-only (b) counter (`workflow_engine_bypass_total{caller}`) so the exemption is measured rather than assumed. The requirement "config changes runtime" (Metric 1) is then honestly scoped to service-level transitions.

## 5. Construction order and circular-wiring check (open rabbit hole)

**Result: no circularity today; the proposed seam adds none. VERIFIED by reading `server/dependencies.go`.**

Order (line numbers from this checkout):

1. `workflowEngine := session.NewDefaultWorkflowEngine()` (`:678`).
2. `pipelineModeRepo` via `NewEntPipelineModeRepository(entClient)` (`:711`), after `EnsureBuiltInWorkflowStages` seed (`:708`) and the sdd pipeline seed (`:720`).
3. `livenessEngine` (`:745-755`).
4. `stageConfigEngine` = `NewConfiguredWorkflowEngine(entStageRepo, entGateSatisfactionRepo, pipelineModeRepo)` (`:773`). It depends only on repos created at `:766-772` plus `pipelineModeRepo` from step 2. It depends on **neither** `BacklogService` nor the lifecycle listener.
5. `backlogLifecycleListener` built (`:875`), later `SetWorkflowEngine(stageConfigEngine)` (`:1544`).
6. `backlogSvc := NewBacklogService(..., workflowEngine, ...)` (`:1477`), then `SetStageConfigEngine(stageConfigEngine)` (`:1486`).

Dependency edges: `ConfiguredWorkflowEngine -> {StageConfigRepository, GateSatisfactionRepository, PipelineModeRepository}`; `BacklogService -> engine, pipelineModeRepo`; neither repo depends on the service. The wiring comment says the engine "takes `pipelineModeRepo`" and the service "takes the engine and the repo" (requirements Rabbit Holes); that is a diamond, not a cycle. A `SwitchableWorkflowEngine` constructed between steps 4 and 6 depends on `{workflowEngine, stageConfigEngine, config reader, shadow recorder}`, all available at `:1477`. The shadow recorder must be created before step 6 and registered with the actuator in `server/actuator.go`; that is the only new ordering constraint (actuator wiring location UNVERIFIED; read before planning).

Risk to flag: **the seed runs at `:708` before the engine's first `cache.Load` at `:773`** (good), but `NewConfiguredWorkflowEngine` swallows load errors (Finding 5). The selector must treat `cfg != nil && cache has zero stages` as `cache_empty` fallback, which needs a tiny accessor (`HasAnyStage()`/`Loaded()`); there is no such method today (UNVERIFIED: only `HasStage(slug)` seen at `configured_workflow_engine.go:~104`).

Dangling-status behavior for in-flight items (requirements Rabbit Hole 1) is already handled by the `StageConfigSnapshot` fallback (ADR-004 as implemented, `configured_workflow_engine.go:96-135`); the selector passes it through unchanged.

## 6. Quota and soft-budget settings: placement in the layering

Current state (VERIFIED): `config.QuotaConfig` (`config/types.go:743-770`) with `QuotaConfigOrDefault()`; `NewQuotaGate` takes a **config closure that re-reads `config.LoadConfig().Quota` each call** (`dependencies.go:1429-1436`), so the gate is already live-reconfigurable by editing `config.json`. `Enabled` defaults false; `AssumedWindowTokenBudget` 0 disables the soft signal (`types.go:759-763,792`). Soft budget: `session.EvaluateBudgetThreshold(itemID, stage, *thresholdUSD, spent)` is pure and advisory (`session/budget_warning.go:3-14`); the threshold is a **per-item** `*float64` passed in by the caller, not a global setting (UNVERIFIED where it is stored on the item; grep `BudgetThreshold`/`thresholdUSD` before planning). Requirements' "soft-budget threshold" therefore has two meanings: a per-item USD threshold (already exists per item) vs. a global default for items that have none. Decide which is meant.

Placement, following the layering already in use:

| Concern | Layer | Why |
|---|---|---|
| Value types, defaults, validation | `config/` (`QuotaConfig`, plus a new `SoftBudgetDefaultUSD`-style field if a global default is wanted) | Existing home; `QuotaConfigOrDefault()` is the single defaulting site. Keep "0 = disabled" legible by adding a derived read-only `Mode` (`rate_limit_only` / `proactive`) computed from `AssumedWindowTokenBudget` rather than a new stored field (QAG-arch §4: in-memory/derived, no schema). |
| Live read | `server/services/quota_gate.go` via the existing closure | No change; it already satisfies "live-settable, no restart". Do not introduce env vars. |
| Settable via UI | A **settings RPC**, not the `bool`-only feature-flag RPC. `knownFeatureFlags` entries are bool (`feature_flag_service.go:172-180`), so `Enabled` can be a flag but thresholds/budget cannot. Put a `Get/UpdateQuotaConfig` pair in the existing defaults/settings service (`DefaultsService` already shares the live cfg with `BacklogService`, `dependencies.go:1544+` comment; exact service to extend is UNVERIFIED) using the repo's `config.LoadConfig`/`SaveConfig` round trip. |
| Soft-budget evaluation | Stays in `session/` (pure) and the existing `checkBudget` callback passed into `snapshotEventForItem` (`backlog_service_events.go:244`); the service supplies the threshold. | Pure function, nothing to move. |
| "Every config surface proves runtime effect" (Metric 5) | A table-driven test file listing each settable field with the observable that must change, plus a lint-style test that enumerates `QuotaConfig` json tags and the stage-config RPC fields and fails if a field lacks an entry in the table. | Reflection over struct tags is deterministic and fast; avoids a new analyzer. A `tools/lint/` analyzer is the heavier alternative, matching `noarchivedrevival`/`norawghrequest`; UNVERIFIED which is proportionate. |

The quota gate is a *gate on dispatch*, not on the state machine; it must stay out of `WorkflowEngine`. It already composes at `SetSyncFeatureEnabledCheck` (`dependencies.go:~1505-1510`). Do not route it through the new selector.

## 7. `allow_auto_merge`: placement

Current state (VERIFIED): proto field exists and the mapper explicitly leaves it unset pending "Phase 4 (Story 4.1.4) ... per-repo, TTL-cached value" (`server/services/backlog_service_stuck.go:140-143`); lifecycle code comments that auto-merge is disabled at repo settings (`session/backlog_lifecycle.go:1925`); `EnablePRAutoMerge` is attempted per PR (`session/backlog_lifecycle_pr.go:718` per requirements, not re-read).

Placement:

- **Fetch in `github/`** (the only package allowed GitHub HTTP, subject to `norawghrequest`): a `RepoAutoMergeSetting(ctx, host, owner, repo)` built on `NewConditionalRequest`/`newGHRequestForHostWithToken` against `GET /repos/{o}/{r}`. A conditional ETag request returns 304 at zero rate-limit cost (the lint rule's stated purpose), which makes a short TTL cheap.
- **Cache in `github/` or `session/` (not the proto mapper):** an in-memory TTL map keyed `host/owner/repo` with a tri-state result `{Known(true|false), Unknown(reason)}`. Persisting on the item (requirements alternative) trades staleness for rate-limit safety; the ETag path removes the rate-limit argument, so prefer **not** persisting (no ent schema change, consistent with QAG-arch's in-memory stance).
- **Populate in the service layer** at `stuckBacklogItemToProto`'s caller (`backlog_service_stuck.go`), not inside the pure mapper: the mapper is data-only. The caller resolves repo from the item, calls the cached lookup, and sets `AllowAutoMerge` only when Known. `optional bool` already distinguishes unset from false (proto `:2144`), so "unknown" stays honest; add a reason string field only if the UI should say *why* (proto change -> `make proto-gen`, registry update).
- **"Known" semantics (rabbit hole):** repo `allow_auto_merge` true is a necessary, not sufficient, condition (branch protection / required checks also gate enabling). Define the field as "repo-level setting", label it so in UI, and let the existing `EnablePRAutoMerge` failure path remain the authority for effective status. Pre-PR warning (scope C): warn when repo setting is false; do not claim success when true.
- Counter `github_allow_auto_merge_lookup_total{outcome=success|failure|cached|not_modified}` per requirements, via the existing counter/actuator pattern.

## 8. Event-Command-Policy table: transition and gate flow (with shadow/enforce)

Extends CWS-arch §7 with the engine-mode policy this project introduces. Grammar: Domain Event | Policy (Whenever...Then) | Command | Actor/System.

| Domain Event | Policy | Command | Actor/System |
|---|---|---|---|
| `TransitionRequested{item, to}` (UI RPC or MCP via service) | Whenever a transition is requested through `BacklogService`, then load the item, build the fallback snapshot, ask the engine whether the edge exists | `engine.CanTransition(from,to,fallback)` | `BacklogService.TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:892`) |
| `EngineConsulted` (mode=`shadow`) | Whenever the selector is in shadow, then answer from Default, evaluate Configured on identical input, and count any difference by class; never change the answer | `ShadowRecorder.Record(method, defaultResult, cfgResult)` | `SwitchableWorkflowEngine` |
| `EngineConsulted` (mode=`enforce`, Configured loaded) | Whenever enforce is on and Configured has a non-empty cache, then take the graph from Configured and the guard set as Default guards plus Configured gates | `Configured.CanTransition` + (`Default guards` UNION `Configured.PendingGates`) | `SwitchableWorkflowEngine` |
| `EngineConsulted` (mode=`enforce`, Configured nil / cache empty / load error) | Whenever Configured cannot answer, then fall back to Default for graph and guards and count the fallback so it is visible | `Default.*`, `fallback_total{reason}++` | `SwitchableWorkflowEngine` |
| `GuardsFailed{sentinel}` | Whenever an unsatisfied gate or guard blocks the transition, then return `FailedPrecondition` for known sentinels, `InvalidArgument` otherwise, and log `from,to,reason=config_graph|gate|default_guard` | `enforce_denied_total{source}++`; error response | `BacklogService` (`isFailedPreconditionGuardError`, `:848-865`) |
| `OperatorOverride{override_reason}` | Whenever an override reason is supplied, then bypass `TransitionGuard` (existing semantics), write a progress note and notify | `AppendProgressNote`, `notifyManualOverride` | `BacklogService` (existing, unchanged) |
| `StageConfigEdited` (CRUD RPC success) | Whenever a stage/transition/gate write succeeds, then refresh the Configured cache so the next transition sees it without restart | `stageConfigEngine.InvalidateCache(ctx)` (`backlog_service.go:417-461`) | `BacklogService` stage CRUD handlers |
| `ShadowDivergence{class=gate_missing|graph, config=seeded}` | Whenever a divergence occurs on an unedited seed, then treat as a bug: write a redacted repro, WARN-log, and hold the soak window open | repro record + log | `ShadowRecorder` |
| `ShadowWindowElapsed` + criteria met | Whenever the soak criteria hold, then surface `READY` in `workflow-engine-soak-status`; the owner (not code) flips the enforce flag | none (human decision) | Owner via settings flag panel |
| `AutomatedTransition{caller=MCP|lifecycle}` | Whenever an agent/lifecycle path calls `storage.TransitionBacklogItemStatus` directly, then it is outside the engine (Tier 2 exemption) and counted as bypass in shadow | `workflow_engine_bypass_total{caller}++` (planned) | MCP handler / `BacklogLifecycleListener` |
| `QuotaCritical` / `QuotaRecovered` | Whenever the quota gate trips on the 60s ticker, then disable/enable `BacklogController`; this is dispatch gating, not a transition-graph concern, so it does not pass through the engine | `BacklogController.Disable/Enable` | `QuotaGate` (QAG-arch §4) |
| `PRCreated{repo}` | Whenever a PR is created for an item, then resolve the cached repo `allow_auto_merge` (ETag-conditional); on false, warn before relying on auto-merge; on lookup failure record Unknown with reason | `RepoAutoMergeSetting` lookup; optional warn notification | `github/` + `BacklogLifecycleListener` PR path |

## 9. Plan implications and open items

Proposed work order (each independently shippable; sizes are cost classes, not durations):

1. **Equivalence harness (M):** characterization test comparing Default vs. seeded-Configured over the 9x9 edge matrix and gate fixtures. Expect it to *fail* on gates today (Finding 1); that failing test is the argument for §2 option A.
2. **`SwitchableWorkflowEngine` + recorder + actuator + flags, default mode shadow, no enforce (M-L):** wire at `dependencies.go:1477`; no handler changes.
3. **Soak (wall-clock blocker):** owner-gated; per the git-cohort how-to; write `docs/how-to/flip-workflow-engine-enforcement.md` modeled on it.
4. **Flip enforce flag (owner decision), then optionally Tier 2 bypass counter -> policy.**
5. Quota settings RPC + UI (M) and `allow_auto_merge` lookup (S-M) are independent of 1-4 and can run in parallel under the WIP cap of 2.
6. Metric 5 enforcement test (S-M), added with each surface.

UNVERIFIED / to confirm before planning:
- Where per-item soft budget threshold is stored and whether a global default is wanted.
- Which settings service should host `Get/UpdateQuotaConfig`; how `FeatureFlagScopes` validates keys for a non-gate flag.
- Whether any MCP/lifecycle transition is *intentionally* exempt from the graph (CWS-arch §4 suggests literal-status allowlists make them stage-specific).
- Actual cyclomatic complexity of `TransitionBacklogItemStatus` (requirements say ~41; not measured here).
- `LoadConfig()` cost on the hot path; shadow-mode TTL cache is a precaution.
- Whether `HasAnyStage()`-style accessor is needed or `cache.HasStage` over built-in slugs suffices.
