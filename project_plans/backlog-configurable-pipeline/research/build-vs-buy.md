# Research: Build vs. Buy — backlog-configurable-pipeline

**Question**: should the per-item pipeline/skill-set selection mechanism (a `PipelineEngine`-shaped
seam consulted by `WriteSlashCommands`, the triage prompt builder, and the review-gate runner) be
built in-house, or sourced from an existing library/engine?

**Bottom line up front**: build in-house, with no new dependency. This is a "select 1 of N
named Go-code-defined behaviors" problem — a `map[string]PipelineMode` registry, following the
exact pattern already established twice in this codebase (`session/workflow_engine.go` and
`session/detection/binary_detector.go`). No external library clears the bar to justify a
dependency; no workflow engine is remotely appropriate given the constraints. The interface
shape should explicitly mirror `WorkflowEngine`, not be invented independently.

---

## 1. OSS Go libraries for "pluggable strategy/policy selection by name"

Surveyed category: small registry/strategy-pattern helper libraries (e.g. generic
`registry[T]` packages, `mitchellh/mapstructure`-adjacent plugin loaders, DI-flavored
"named implementation" containers).

**Pros of adopting one:**
- Marginal boilerplate reduction (a generic `Registry[K,V]` saves ~15 lines vs. a hand-rolled map).
- Some provide thread-safety and duplicate-registration panics for free.

**Cons:**
- This repo already has two independent, hand-rolled implementations of this exact
  pattern in-tree (`DetectorRegistry` in `session/detection/binary_detector.go`,
  `DefaultWorkflowEngine`'s transition map in `session/workflow_engine.go`). A third
  hand-rolled one is idiom-consistent; a library-backed one is not — it would be the only
  place in the package doing registry lookups differently.
- `.claude/rules/interface-pollution-checklist.md` (smell #5, "Unjustified generic") and
  this repo's own `go-development` skill both explicitly favor "a little copying... over a
  little dependency" for exactly this shape of problem — single call site, ~5 registered
  entries (`DefaultPipelineEngine` + 1-2 alternative modes per the requirements doc), no
  need for runtime plugin loading.
- Any such library would need a `go.mod` dependency; none is currently imported. No general
  DI/registry library appears anywhere in the existing 180-line `require` block.
- A `map[string]PipelineMode` with a package-level `var registry = map[...]{...}` is fewer
  lines than importing, learning, and wiring a generic registry package, and it doesn't
  leak a generic type parameter into a single-call-site consumer (checklist smell #5 again).

**Verdict: Not recommended.** Use `map[string]PipelineMode` (or equivalent named-constant
switch) directly in the `session` package, matching `DetectorRegistry`'s
`Register`/`Lookup`/`Names` shape. No new dependency.

---

## 2. OSS workflow/pipeline engines (Temporal, Cadence, Argo Workflows, `looplab/fsm`,
   `qmuntal/stateless`)

**Temporal / Cadence / Argo Workflows** (durable execution, distributed workflow orchestration):
- Pros: battle-tested retries, durable state across process restarts, rich visualization/observability.
- Cons: every one of these solves problems this project explicitly does not have per the
  requirements doc ("NOT a distributed workflow-orchestration problem... no need for durable
  execution, retries-across-process-restarts, or a DAG scheduler"). Adopting any of them means
  re-platforming the *existing, working* `AutonomousDriver`/headless-pool/tmux-session machinery
  onto a new execution substrate — a multi-week infrastructure migration to add what is scoped as
  a small selection feature. Requires running additional infrastructure (Temporal server, Argo on
  k8s) for a single-operator local tool. None of this repo's `go.mod` currently imports a workflow
  engine client.
- Verdict: **Not recommended.** Wildly disproportionate to the problem; violates the constraint
  section of requirements.md directly.

**`looplab/fsm` / `qmuntal/stateless`** (lightweight embedded Go state-machine libraries):
- Pros: would be a legitimate lighter-weight choice if the problem were "govern legal state
  transitions with guard/callback hooks" — which is coincidentally almost exactly what
  `session/workflow_engine.go` already does today, hand-rolled, for `BacklogStatus` transitions.
- Cons: the *actual* ask here is not a state machine — it's selecting *which named prompt/skill
  template and Go glue code* runs for a stage, not modeling legal transitions between states
  (that's already `WorkflowEngine`'s job and is out of scope for this feature to duplicate or
  replace). Introducing an FSM library would create two competing transition-modeling mechanisms
  in the same package. Also a new dependency where zero justification exists — this repo already
  proved (via `workflow_engine.go`) that hand-rolling this exact shape of problem is both trivial
  and preferred locally.
- Verdict: **Not recommended** for this feature. (Worth noting for the record, not worth adopting:
  if `WorkflowEngine` itself were ever rewritten, `qmuntal/stateless` would be the stronger of the
  two libraries — generics-based, no reflection — but that's a separate, unscoped question.)

---

## 3. Bespoke `PipelineEngine` interface vs. mimicking an established pattern

The question here is about the *shape* of the interface, not whether to use a library.

**Bespoke, invented independently:**
- Pros: none of substance — this is a well-worn shape (name → behavior lookup + validation)
  that the codebase has already solved twice.
- Cons: real risk of subtly reinventing `WorkflowEngine`'s conventions differently — e.g.
  different error-wrapping style, different deep-copy-on-construct discipline, different
  validation-at-construction-vs-validation-at-call-time choice. That inconsistency is exactly
  what an LLM-authored "fresh" interface tends to introduce (see this repo's own
  `.claude/rules/interface-pollution-checklist.md` — bespoke interfaces invented ad hoc are the
  primary failure mode it exists to catch).

**Mimic `WorkflowEngine`'s established shape:**
- `WorkflowEngine` is a 3-method, narrow, package-scoped interface: a query method
  (`CanTransition`), a validation/guard method (`ValidateGates`), and an enumeration method
  (`AllowedTransitions`). `DefaultWorkflowEngine` deep-copies its backing map at construction
  time (explicitly called out in requirements.md as the pattern to reuse) and is constructed via
  `NewDefaultWorkflowEngine()`.
- A `PipelineEngine` interface shaped the same way — e.g. `ResolveStages(mode PipelineMode)
  []Stage`, `ValidateMode(mode PipelineMode) error`, `AvailableModes() []PipelineMode` — gets
  the deep-copy-on-construct discipline, the narrow-interface-in-consumer-package convention, and
  the "hardcoded default map wrapped in a struct" pattern for free, with no new design risk.
- Verdict: **Recommended.** Bespoke code is correct here, but it should explicitly copy
  `WorkflowEngine`'s shape (method count, naming convention, construction pattern) rather than be
  designed independently. This directly satisfies requirements.md's constraint to "reuse
  `session/workflow_engine.go`'s narrow-interface + deep-copy-on-construct Go pattern."

---

## 4. Fork or adapt existing in-repo code

**Can `PipelineEngine` be a method on `WorkflowEngine`, or a value it returns, rather than a
separate type?**

No — `WorkflowEngine` (`session/workflow_engine.go`) governs `BacklogStatus` *transitions*
(triage → planning → implementing, etc., validated against `validTransitions` and
`TransitionGuard`). `PipelineEngine` governs a materially different axis: *which prompt/skill
template and Go glue code runs* for a given item at a given stage (e.g. `/sdd:full` vs. a
trivial-item fast path) — this is orthogonal to whether the transition itself is legal. Item A
and item B can both legally transition `triage → planning`, while running completely different
pipeline modes to get there. Conflating the two would violate single-responsibility: a status
guard and a template/skill selector are different concerns with different consumers
(`WorkflowEngine`'s only consumer today is the backlog status-transition path; `PipelineEngine`'s
consumers per requirements.md are `WriteSlashCommands`, the triage prompt builder, and the
review-gate runner — barely overlapping call sites). **Recommendation: a sibling type in the
`session` package, not a method or return value of `WorkflowEngine`** — but constructed and
shaped identically (see §3).

**Does `session/detection/`'s registry pattern transplant directly?**

Yes, and more directly than `WorkflowEngine` does, for the *storage* mechanism specifically.
`session/detection/binary_detector.go`'s `DetectorRegistry` is precisely a
`map[string]BinaryDetector` with `Register` (panics on duplicate), `Lookup(name) (T, bool)`,
`Names() []string`, and `Len() int`. This maps almost verbatim onto "look up a `PipelineMode` by
name and get back its behavior," and `session/detection/registry.go`'s `DefaultRegistry()`
free function (pre-populating a `*DetectorRegistry` with the built-in set) is a ready-made
template for a `DefaultPipelineRegistry()` free function pre-populating built-in pipeline modes.
Combining the two found patterns: **`PipelineEngine` interface shaped like `WorkflowEngine`
(query/validate/enumerate methods, deep-copy-on-construct), backed internally by a
`DetectorRegistry`-shaped `map[string]PipelineMode` for storage/lookup.** This satisfies both
explicit reuse instructions in requirements.md using code patterns that already exist twice in
this repo, with zero new dependencies.

**Security note (from requirements.md constraints):** both existing patterns already satisfy
"validate against a known registry, never let free text flow into a prompt/command-line
context" — `DetectorRegistry.Lookup` and `DefaultWorkflowEngine.CanTransition` both reject
unknown keys by construction (map miss → `false`/`not found`, never string interpolation). The
same discipline transplants directly: `PipelineMode` should be a typed string/enum validated via
map lookup, never interpolated raw into a prompt template.

---

## Summary Table

| Option | Verdict |
|---|---|
| Generic/library-based named-registry package | Not recommended — no dependency justified; hand-rolled map matches 2 existing in-repo patterns |
| Temporal / Cadence / Argo Workflows | Not recommended — solves a distributed-durable-execution problem this feature explicitly doesn't have |
| `looplab/fsm` / `qmuntal/stateless` | Not recommended for this feature — wrong problem shape (selection, not state-transition modeling); `WorkflowEngine` already owns transitions |
| Bespoke `PipelineEngine` interface, independently designed | Not recommended — real risk of convention drift from `WorkflowEngine` |
| Bespoke `PipelineEngine`, shaped after `WorkflowEngine` + backed by `DetectorRegistry`-style map | **Recommended** |
| `PipelineEngine` as a method/return value of `WorkflowEngine` | Not recommended — different concern, different consumers, would violate single-responsibility |

## Recommended approach

Build a new, small, sibling type in the `session` package:
- Interface `PipelineEngine` with narrow methods mirroring `WorkflowEngine`'s shape (e.g.
  `ResolveStages`/`ValidateMode`/`AvailableModes`), consumed by `WriteSlashCommands`,
  `session/backlog_triage.go`, `server/services/backlog_service_triage.go`, and
  `session/review_gate.go`.
- `DefaultPipelineEngine` implementation backed internally by a `map[PipelineMode]...`
  structure styled after `session/detection/binary_detector.go`'s `DetectorRegistry`
  (`Register`/duplicate-panic, `Lookup` returning `(T, bool)`).
- Deep-copy the backing map at construction time, per `NewDefaultWorkflowEngine`'s pattern.
- Zero new `go.mod` dependencies.
