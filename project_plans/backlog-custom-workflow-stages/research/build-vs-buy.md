# Research: Build vs. Buy — backlog-custom-workflow-stages

**Question**: should the custom/configurable workflow state machine — per-stage liveness/timeout
detection plus a multi-type transition-gate system — be built from scratch in Go, or sourced from
an existing solution?

**Bottom line up front**: build in-house, extending `session/workflow_engine.go` and
`session/pipeline_engine.go`'s already-established patterns. This mirrors the sibling
`backlog-configurable-pipeline` project's own build-vs-buy verdict
(`project_plans/backlog-configurable-pipeline/research/build-vs-buy.md`), and for the same root
reason: the hard part of this feature — liveness/staleness semantics and typed transition gates —
is not a solved problem any of the candidate libraries or services model, so adopting one would add
a dependency without removing the work this project actually needs to do.

---

## 1. Existing OSS Go library/framework for embeddable workflow/state-machine engines

Surveyed: `qmuntal/stateless` (generics-based Go FSM library), Temporal Go SDK as an embedded
library, and `go.mod` for any DAG/graph library already present (none — no `dominikbraun/graph`,
no `looplab/fsm`, no `qmuntal/stateless`, no workflow-engine client of any kind).

**`qmuntal/stateless`** (github.com/qmuntal/stateless):
- Pros: actively maintained — latest release Feb 2026 per [pkg.go.dev](https://pkg.go.dev/github.com/qmuntal/stateless), BSD-2-Clause license (permissive, no copyleft concerns), generics-based (any comparable type for states/triggers), supports hierarchical states, guard clauses, entry/exit callbacks, DOT graph export for visualization, thread-safe.
- Cons — the critical finding: **it has no concept of liveness/staleness of a state, and no concept of a typed, multi-actor transition gate.** Its "guard clauses" are synchronous boolean predicates evaluated at transition time (`func() bool`) — structurally similar to today's `TransitionGuard`, but with no notion of "how long has this state been active," "what counts as progress while in this state," or "which of N gate types (human/automated-review/structural/custom) is outstanding and who can satisfy it." Every requirement this project's Problem Statement and Scope sections describe as new work — the liveness/timeout envelope, the gate-type taxonomy, the BUG-055 sweep-vs-budget invariant — would need to be hand-built on top of `stateless` exactly as much as on top of today's hand-rolled `validTransitions` map. The library would only replace the ~40-line `CanTransition`/`AllowedTransitions` mechanics in `workflow_engine.go`, which are not the part of the system that is hard or currently broken.
- Additional cons: introducing it would create a second transition-modeling mechanism alongside the existing `DefaultWorkflowEngine`/`validTransitions` machinery for the built-in 9-state case (the Risk Control section requires built-in behavior stay bit-for-bit identical) — either migrating the built-in states onto `stateless` too (churn with no functional benefit, re-testing surface for zero-regression-risk code) or running two engines side by side (the "two competing transition-modeling mechanisms" anti-pattern the sibling project's own build-vs-buy research already flagged for this same library).
- Verdict: **Not recommended.** Legitimate library, wrong problem. It would answer a question ("is this transition structurally legal") this codebase already answers correctly, while leaving every actually-new requirement (liveness, gates) fully unaddressed.

**Temporal Go SDK (embedded, not full server)**:
- Pros: mature durable-execution model, retries, `workflow.Sleep`/timer primitives that resemble "liveness/timeout" conceptually.
- Cons: [Temporal's own documentation](https://docs.temporal.io/self-hosted-guide/embedded-server) states embedding its server via `temporal.NewServer()` with SQLite is explicitly for **testing and development only, not recommended for production** — this repo's backlog engine is a live, in-production (if single-operator) system. Even setting that aside, Temporal solves distributed-durable-execution-across-process-restarts — a problem the sibling `backlog-configurable-pipeline` project's own research already ruled out for this codebase ("NOT a distributed workflow-orchestration problem... no need for durable execution, retries-across-process-restarts, or a DAG scheduler"), and nothing in this project's requirements changes that verdict. Adopting Temporal (even embedded) means re-platforming `AutonomousDriver`/the tmux-session/headless-pool machinery onto a new execution substrate — a multi-week infrastructure migration dwarfing this project's own Large (3–6 week) appetite for the entire feature.
- Verdict: **Not recommended.** Disproportionate, and documented as unfit for the production posture this tool actually has.

**Any DAG/graph library already in `go.mod`**: none exists. No graph, workflow, or FSM library appears in the `require` block (verified by reading `go.mod` directly — see below).

---

## 2. SaaS/managed workflow service (Temporal Cloud, AWS Step Functions, Camunda Cloud)

- Pros: offloads durability, scaling, and visualization to a vendor; Step Functions/Camunda both have first-class "human approval task" and "wait for external signal" primitives that superficially resemble this project's gate model.
- Cons, decisive: this is a **single-operator, locally-run personal tool** (per requirements.md's Non-functional Requirements: "Scalability: not applicable... same posture as the sibling `PipelineEngine` project," and per this repo's broader threat model of a local Go binary + local SQLite). A SaaS dependency for the backlog engine's core state machine would:
  - Introduce a hard network dependency for every status transition on a tool designed to run entirely offline/locally today (`~/.stapler-squad/`, local SQLite via `session/ent`).
  - Add recurring cost (Temporal Cloud, Step Functions per-transition pricing, Camunda Cloud subscription) for a workload of one user's backlog items — pricing models built for org-scale usage.
  - Add a second operational surface (auth/API keys, service outages, version/API drift) to a codebase whose entire architectural point (per this repo's own CLAUDE.md) is a single Go binary + embedded state.
  - Data residency: backlog item content (titles, descriptions, code context) would leave the local machine for a third party with no stated need — a regression from the current all-local posture, not a neutral trade.
  - None of these services address the actually-hard requirements (BUG-055's sweep-vs-budget-with-margin invariant, the stage-×-pipeline-mode liveness axis, the custom/pluggable gate sandboxing question) any better than in-house code — they'd need the same design work layered on top via their own extension mechanisms (Step Functions Lambda tasks, Camunda external tasks), just now over a network boundary.
- Verdict: **Not recommended — does not fit a single-operator local tool at all.** This is the same "no" the sibling `PipelineEngine` project already gave for equivalent reasons (see its build-vs-buy Temporal/Cadence/Argo section), except stronger here: those are self-hostable OSS; a managed SaaS additionally fails on cost and data-residency grounds that don't even apply to the self-hosted OSS case.

---

## 3. LLM-generated bespoke graph code vs. a small, well-tested graph library — for cycle detection / topological validation specifically

This is the one sub-piece where "buy a small library" is genuinely worth weighing on its own,
separate from the whole-engine question above, because it isolates the correctness-critical part:
validating a user-authored transition graph (detecting cycles, unreachable states, etc.) is
classic graph-algorithm territory where off-by-one/visited-set bugs are a known LLM failure mode.

- **Bespoke Go implementation** (a hand-written DFS-based cycle detector / topo-sort over
  `map[Stage][]Stage`):
  - Pros: zero new dependency; trivial to keep inside the same package as `WorkflowEngine`;
    the graph here is small (custom stages are expected to number in the dozens at most, defined by
    one operator through a UI, not machine-generated) so algorithmic sophistication isn't the risk —
    correctness of a ~50-line DFS is.
  - Cons: cycle detection and topological sort are exactly the class of "looks right, has a subtle
    bug on a specific graph shape (self-loop, disconnected component, diamond)" code this repo's own
    `code-root-cause-analysis`/`quality:reflect-and-fix` philosophy warns about for LLM-authored code.
    A hand-rolled version needs real test coverage (self-loops, multi-cycle graphs, disconnected
    stages) to trust it, which is exactly the coverage a library would have already paid for.
- **Small, well-tested graph library** (e.g. `dominikbraun/graph`'s `graph.TopologicalSort` /
  cycle-detection helpers, or even stdlib-adjacent approaches):
  - Pros: correctness for cycle detection is a solved, narrowly-scoped problem; a well-tested library
    removes exactly the risk class above for a small, easily-isolated piece of the system. This is a
    genuinely different shape of decision from the whole-engine question in §1 — it's "outsource one
    ten-line algorithm with high correctness stakes and low interface-surface risk," not "outsource
    the whole liveness/gate model."
  - Cons: one new `go.mod` dependency for a small amount of functionality; needs evaluation for
    maintenance/license fit (not deeply surveyed here since the bespoke option is judged sufficient —
    see verdict).
- Verdict: **Viable either way, lean bespoke.** Unlike the whole-engine question, this is a real
  build-vs-buy trade-off worth Phase 3 planning revisiting concretely — but the graph size here
  (operator-authored, small, rarely-changing) and the fact this repo's own precedent
  (`interface-pollution-checklist`'s "a little copying over a little dependency" guidance, already
  invoked verbatim in the sibling project's build-vs-buy doc) both favor a bespoke implementation
  **with mandatory table-driven tests covering self-loops, multi-node cycles, and disconnected
  stages** before it's trusted — i.e., pay the cost with tests, not with a dependency. Phase 3
  planning should treat "does the bespoke cycle-detector have adversarial test coverage" as a
  concrete gate, not assume correctness from code review alone.

---

## 4. Fork or adapt existing in-repo code

This is where nearly all of the real leverage is. Both closest candidates were read in full.

**`session/workflow_engine.go`'s `WorkflowEngine`/`DefaultWorkflowEngine` (76 lines)**:
- Directly reusable: the **interface shape** (`CanTransition`, `ValidateGates`, `AllowedTransitions`
  — a query/validate/enumerate triad) and the **construction discipline** (deep-copy the backing map
  at `New*` time so no caller can mutate shared state; `NewDefaultWorkflowEngine()` is a zero-arg,
  infallible, pure in-memory constructor). Both are explicitly named in requirements.md's Constraints
  section as the pattern `ConfiguredWorkflowEngine` must reuse, and both transplant mechanically: a
  `ConfiguredWorkflowEngine` backed by a DB-loaded map, with the same three methods, satisfies the
  existing interface with zero call-site changes at `GuardedTransitionAllowed` and everywhere else
  that already programs against `WorkflowEngine` rather than the concrete `DefaultWorkflowEngine`
  type. `DefaultWorkflowEngine` itself is also the literal Milestone-1/Risk-Control regression
  baseline — "the default/built-in stage set... must be behaviorally identical to today's hardcoded
  behavior" is testable by asserting `ConfiguredWorkflowEngine`'s built-in-stage-set output matches
  `DefaultWorkflowEngine`'s, transition for transition.
- What does NOT transplant: the interface has no liveness or gate-taxonomy concept at all — the
  Feasibility Risks section already correctly flags this ("designed for status-transition legality
  only... extending it needs its own design pass, not a mechanical 4th method"). This is genuinely
  new design, not adaptable from the existing 76 lines.

**`session.WorkflowRepository`**: closest existing DB-persisted/slug-addressed precedent per the
Feasibility Risks section, but explicitly flagged as having **no caching layer** — the NFR section
already directs Phase 3 to reuse `backlog-configurable-pipeline`'s caching design
(`pipelineModeCache` in `session/pipeline_engine.go`: an `atomic.Pointer[map[...]]` for lock-free
reads plus a `sync.Mutex`-serialized `refresh` for writer-side consistency) rather than building a
second cache mechanism. Read in full: this is directly reusable as a structural template —
`stageConfigCache`/`livenessConfigCache` can be near-verbatim copies of `pipelineModeCache`'s
`ptr atomic.Pointer[...]` / `writeMu sync.Mutex` / `Load`/`Invalidate`/`refresh`/`Get` shape, same
"hold `writeMu` across the DB read, not just the Store" discipline that prevents the lost-update race
`pipelineModeCache`'s own doc comment calls out.

**`project_plans/backlog-configurable-pipeline/`'s shipped `PipelineEngine`/`CachingPipelineEngine`
(session/pipeline_engine.go, 481 lines)**:
- Directly reusable patterns (beyond the cache, above):
  - The **fail-closed-with-Warn-log fallback shape**: every `CachingPipelineEngine` method resolves a
    slug via `e.cache.Get`, and on a miss logs exactly one `[PipelineEngine]`-prefixed Warn line
    naming the item and the unresolved value, then falls back to default behavior — this is verbatim
    the behavior requirements.md's Risk Control section mandates for an unresolvable stage/liveness
    config ("fail closed... never a silent no-op or crash"). `ConfiguredWorkflowEngine` and any new
    liveness-resolution code should copy this exact log-then-fallback idiom, including the
    `[ConfiguredWorkflowEngine]`/`[Liveness]`-style prefix convention.
  - The **`PipelineModeDefault` sentinel-value pattern**: an empty-string sentinel that is guaranteed,
    by construction, to never touch the cache or DB — this is the concrete mechanism that keeps the
    NFR's "no uncached DB read on the hot path for the common case" true. The same sentinel shape
    (e.g. a `StageSetDefault`/no-custom-config-configured value) should gate `ConfiguredWorkflowEngine`
    resolution the same way.
  - The **deep-copy-at-cache-load-time discipline**: `resolvedPipelineMode` is copied field-by-field
    from `*ent.PipelineMode` so concurrent readers never see a partially-updated ent object — the same
    discipline applies directly to a `resolvedStage`/`resolvedLivenessConfig` type.
- What is a sibling relationship, not a fork target: `PipelineEngine` governs *which prompt/skill
  content* runs within a stage; this project governs *which stages exist and how long work inside
  them may run*. Per requirements.md's own Users/Consumers section and the existing package-doc
  comment on `session/pipeline_engine.go` (`PipelineEngine is a SIBLING of WorkflowEngine... coupling
  them... would pull unrelated concerns together for no benefit`), `ConfiguredWorkflowEngine` should
  be constructed and held the same way — a new, independent field on the same callers
  (`BacklogService`, `BacklogLifecycleListener`), not a wrapper around or extension of
  `PipelineEngine`. This mirrors the already-settled `WorkflowEngine`/`PipelineEngine` boundary and
  answers one of requirements.md's Open Questions (whether liveness/gates should be a new method on
  `WorkflowEngine` or a sibling interface) by precedent: **sibling interface**, consistent with how
  `PipelineEngine` was kept separate from `WorkflowEngine` rather than merged into it.

**`session/review_gate.go`'s `ReviewGateRunner`** (567 lines; read for structure, not in full detail
here since Rabbit Holes already flags it for deeper Phase 2 survey): its PASS/FAIL/UNVERIFIABLE
verdict flow (`ReviewOutcome*` constants already aliased in `session/backlog.go`) is the existing,
working shape for one of the four gate actor types (automated reviewer). It is a genuine
generalization target, not a fork-and-duplicate target — requirements.md's Rabbit Holes section
already correctly directs "generalizing it... should extend/parameterize that existing mechanism,
not build a parallel one."

**Estimated reuse fraction**: the interface *shape* (query/validate/enumerate + deep-copy-on-construct),
the *caching layer* (near-verbatim from `pipelineModeCache`), and the *fail-closed fallback idiom*
(near-verbatim from `CachingPipelineEngine`) are all directly portable — call it the mechanical
"plumbing" layer. The genuinely new design work is: (1) the liveness data model itself (expected
duration / staleness threshold / stage-vs-pipeline-mode-vs-finer-grain axis — an open question in
requirements.md, not decidable by copying existing code), (2) the BUG-055-safe
derive-don't-duplicate relationship between a work budget and its sweep threshold, (3) the
transition-gate taxonomy and its UI affordances, and (4) whatever `ReviewGateRunner` generalization
custom-transition automated-review gates require. None of these four are answered by any existing
in-repo code — they are the actual Large-appetite content of this project.

---

## Summary Table

| Option | Verdict |
|---|---|
| `qmuntal/stateless` (OSS Go FSM library) | Not recommended — actively maintained, permissive license, but models none of this project's actually-hard requirements (liveness, typed gates); would only replace the already-correct, already-tested 40 lines of transition mechanics |
| Temporal Go SDK (embedded) | Not recommended — vendor docs say embedded-server mode is dev/test only, not production; disproportionate infra migration for a single-operator tool |
| SaaS workflow service (Temporal Cloud / Step Functions / Camunda Cloud) | Not recommended — does not fit a single-operator local tool: hard network dependency, recurring cost, data leaves the machine, doesn't solve the actually-hard requirements any better |
| Bespoke cycle-detection/topo-validation vs. a small graph library | Viable either way; lean bespoke with mandatory adversarial test coverage (self-loops, multi-cycle, disconnected stages) given the small, operator-authored graph size |
| Fork/extend `WorkflowEngine` interface shape + construction discipline | **Recommended** — direct reuse, explicitly mandated by requirements.md's Constraints |
| Fork/extend `pipelineModeCache`'s caching design | **Recommended** — near-verbatim template, satisfies the NFR's no-uncached-hot-path-read requirement |
| Fork/extend `CachingPipelineEngine`'s fail-closed-with-Warn-log idiom | **Recommended** — directly satisfies Risk Control's fail-closed mandate |
| Generalize `ReviewGateRunner` for the automated-review gate type | **Recommended** (per requirements.md's own Rabbit Holes direction) — extend, don't parallel-build |
| `ConfiguredWorkflowEngine`/liveness/gates as a method of `WorkflowEngine` rather than a sibling interface | Not recommended — breaks the settled `WorkflowEngine`/`PipelineEngine` separation precedent |

## Final Recommendation

**Build in-house.** For a single-operator internal Go/ent/ConnectRPC monorepo that already contains
two hand-rolled, working precedents for exactly this shape of problem (`WorkflowEngine`'s
transition-legality engine, `PipelineEngine`'s DB-persisted/cached/fail-closed content-selection
engine), no external library or service earns its dependency cost:

1. No OSS state-machine or workflow-engine library — embeddable (`qmuntal/stateless`) or
   durable-execution (Temporal) — models the two concepts that are actually new here: per-stage
   liveness/staleness and typed, multi-actor transition gates. Adopting one swaps out the
   already-correct 40 lines of transition mechanics while leaving 100% of the real design work
   (liveness model, BUG-055 invariant, gate taxonomy, `ReviewGateRunner` generalization) exactly as
   unsolved as it is today.
2. A managed SaaS workflow service is categorically wrong for this tool's single-operator,
   local-first, no-oncall posture — new cost, new network dependency, new data-residency exposure,
   for a workload of one user's backlog items, solving none of the hard parts any better.
3. For the one narrowly-scoped sub-problem where a small library is a genuine option — graph
   cycle-detection/topological validation — lean bespoke, but treat adversarial test coverage
   (self-loops, cycles, disconnected components) as a non-negotiable gate given this is exactly the
   LLM-authored-code failure class this repo's own quality skills warn about.
4. The real leverage is Option 4: **`ConfiguredWorkflowEngine` should be a near-mechanical extension
   of `WorkflowEngine`'s interface/construction pattern, backed by a cache copied from
   `pipelineModeCache`'s design, with fallback behavior copied from `CachingPipelineEngine`'s
   fail-closed idiom, held as a sibling field alongside (not a method of) `PipelineEngine` and
   `WorkflowEngine`.** This satisfies requirements.md's Constraints section verbatim and lets Phase 3
   planning spend its design budget on the genuinely new content — the liveness data model's grain
   (stage / stage×mode / finer), the BUG-055-safe sweep-threshold derivation, and the transition-gate
   taxonomy — rather than re-litigating plumbing this codebase has already solved twice.
