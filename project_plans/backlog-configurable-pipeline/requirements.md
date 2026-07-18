# Requirements: backlog-configurable-pipeline

**Date**: 2026-07-15
**Type**: feature addition (system design — touches data model, proto, backend orchestration, UI)
**Complexity**: 4 — bumped from 3 after the runtime-configurability decision below added a new
DB-persisted domain object with no existing caching precedent in this codebase to copy from.

Source: bucket [3] of `docs/tasks/backlog-feature-improvement.md` (audit run 2026-07-14 by the
`backlog-feature-improvement` skill). Per that skill's Phase 5 routing table, this requirements
doc is hand-written directly from the audit findings — the `sdd:1-ideate` interview is skipped
because the audit already answered problem/why. Each requirement below is traceable back to a
specific finding; see the "Source" line under each.

## Problem Statement

The backlog feature's stated end goal (`.claude/skills/backlog-feature-improvement/SKILL.md`,
opening line) is: an item goes idea → shipped PR with minimal human intervention, and the
pipeline stages themselves (triage → plan → implement → review → merge) are **configurable
per item** — a user should be able to say "use `/sdd:full` for this item" or "skip the
planning stage for this one, it's trivial."

Today the pipeline is fixed and hardcoded. Bucket [2] of the same audit (now closed) proved
that most *manual-click* friction was already solvable with narrow per-item bool flags
(`SkipReviewGate`, `SkipPlanning`, `AutoSpawnSession`). Bucket [3] is different in kind: no
existing flag lets a user choose *which skills or commands run* for an item, or *how many
stages* a pipeline has. That is a data-model and orchestration gap, not a UI-click gap.

## Baseline

Today, every backlog item gets the identical fixed slash-command set
(`session/backlog_commands.go:20-100`, `WriteSlashCommands`) and the identical fixed sequence
of stages (triage → review, hardcoded in `session/backlog_lifecycle.go` and
`session/review_gate.go`). The only per-item variation is three boolean short-circuits
(`SkipReviewGate`, `SkipPlanning`, `AutoSpawnSession`) that skip a stage entirely — none let a
user *substitute* a different stage, skill, or command. A user who wants "this item should go
through my `/sdd:full` workflow" or "this item only needs a quick fix, use `/sdd:quick`" has no
way to express that; they'd have to manually drive a session themselves, defeating the
automation. `session/repository.go:330-357` (`BacklogItemData`) has no field that could even
hold such a choice today.

## Users / Consumers

- The backlog UI operator (currently just the repo owner) configuring items via
  `BacklogItemForm.tsx` / `BacklogItemDetail.tsx`.
- The backend orchestration code that currently hardcodes stage sequencing:
  `session/backlog_lifecycle.go` (`BacklogLifecycleListener`), `session/review_gate.go`
  (`ReviewGateRunner`), `session/autonomous_driver.go`, `server/services/backlog_service_triage.go`.
- Downstream: `web-app/src/components/backlog/BacklogBoard.tsx` (`COLUMNS`), which will need to
  reflect whatever stage set an item is actually configured to run, not a fixed 6-status list.

## Success Metrics

- A user can set, per backlog item, which skill/command set drives triage and implementation
  (e.g. "default", "sdd:quick", "sdd:full") and the pipeline actually invokes that choice —
  verified by inspecting the slash-commands written into the item's worktree
  (`WriteSlashCommands` output) and/or the prompt handed to the headless/triage call.
- **A user can define a new pipeline mode (name, description, and the slash-command/prompt
  content it maps to) through the UI, without a code change or redeploy, and immediately use it
  on a backlog item.** This is the decisive success metric distinguishing this project from a
  static code registry — see the Runtime Configurability Decision below.
- The UI surfaces, for any item, *what pipeline/skills ran or will run* — closing the UX-review
  finding that no current screen shows this (bucket [3] audit, UX pass).
- Adding a new pipeline mode requires *no engineering involvement* (a DB write via the UI), not
  editing `WriteSlashCommands`'s body and not a code deploy — this supersedes the audit's
  original, narrower "requires extending a registry" framing now that runtime configurability is
  in scope.
- Existing items with no explicit pipeline choice behave exactly as today (default pipeline) —
  zero regression for the current single-pipeline behavior.
- Pipeline-mode resolution on the hot paths (`TriggerTriage`, `WriteSlashCommands`,
  `ReviewGateRunner.Run`) does not add an uncached synchronous DB read per call — see the
  Non-functional Requirements caching mandate below.

## Runtime Configurability Decision (resolved 2026-07-15)

**Decision: pipeline modes are DB-persisted and user-definable at runtime**, not a closed
Go-code registry. This was an explicit build vs. buy / architecture trade-off surfaced during
Phase 2 research and Phase 3 planning kickoff — recorded here because it reverses that research's
initial recommendation and every downstream document must treat this as settled, not open.

**Why the research initially recommended the opposite** (a small code-defined registry,
`map[string]PipelineEngine`): two concerns, both real —
1. *Security*: DB-editable mode content flowing into LLM prompts / `WriteSlashCommands`'s
   file-writing is a larger injection surface than a closed, PR-reviewed set of Go functions.
2. *Precedent*: ADR-013 (`WorkflowEngine`) explicitly considered and rejected "load config from
   DB on every call" (its "Alt B") for exactly this reason — no caching strategy, O(n) DB reads on
   a hot path — and shipped only a static `DefaultWorkflowEngine`, deferring DB-persisted custom
   config to a never-built "Phase 2."

**Why the decision is DB-persisted anyway:**
- This is a **single-operator internal tool** (confirmed throughout this doc's NFR/Scope
  sections) — the person defining a pipeline mode through the UI is the same person who could
  otherwise edit the Go source directly and redeploy. The generic multi-tenant injection threat
  model the research applied doesn't fully hold here: there is no untrusted third party the
  operator needs protecting from. The residual risk is *self-inflicted breakage* (a malformed
  mode definition producing a broken prompt or invalid slash-command file), not adversarial
  injection — a correctness/robustness concern, not a security boundary.
- Real, already-live precedent in this exact codebase: `session.Workflow` /
  `WorkflowRepository` (`session/workflow_repository.go`, `session/ent_workflow_repository.go`)
  is already a DB-persisted, slug-addressed, user-creatable named preset, driving the omnibar's
  `@workflow-slug` detector today. "Runtime-configurable named presets" is proven, shipped
  infrastructure in a sibling subsystem, not a novel bet.
- The stated end goal (Problem Statement above) is explicitly about reducing how much engineering
  involvement is needed to add a new pipeline behavior — a code-registry still requires a PR and
  a deploy for every new mode, which only partially satisfies "customizable... a user can say 'use
  X for this item'" if "user" doesn't mean "engineer who can ship a PR."

**What this decision does NOT reopen:**
- `WorkflowEngine`/`ConfiguredWorkflowEngine` (status-transition legality) stays out of scope,
  unchanged — this decision is scoped entirely to `PipelineEngine` (which skills/prompts run
  within a status), a different seam. Do not conflate the two; see Constraints below.
- This does not mean "arbitrary free-text appended to every prompt" (the first Alternative
  Considered below, still rejected) — mode definitions are structured DB rows (name, description,
  and a small number of typed fields mapping to concrete prompt/command content), not an
  unstructured text box. Structure is what keeps this closer to "configuration" than "arbitrary
  code injection," even under the relaxed single-operator threat model above.

**What this decision newly requires** (flows into Constraints/NFR/Scope below):
1. An ent schema + migration for a `PipelineMode`-equivalent table (mirroring `session.Workflow`'s
   shape: slug, name, description, enabled, plus whatever structured content fields
   `WriteSlashCommands`/prompt-building need — Phase 3 planning must define these concretely).
2. **A caching strategy that `WorkflowRepository` does not already provide.** Confirmed by direct
   inspection of `session/ent_workflow_repository.go`: every method (`GetBySlug`, `ListEnabled`,
   etc.) does a direct, uncached ent query. That's acceptable for the omnibar (human keystrokes,
   low frequency) but NOT acceptable for `PipelineEngine`'s consumers — `TriggerTriage` and
   `WriteSlashCommands` are the two highest-complexity functions in the backlog subsystem (per the
   originating audit's hotspot scan) and already sit on a hot path. Phase 3 planning must design
   an explicit cache (in-memory, invalidated on write, or short-TTL refresh) rather than copying
   `WorkflowRepository`'s query-every-call pattern verbatim.
3. A management UI surface for creating/editing/enabling/disabling pipeline modes (beyond just
   *selecting* one on a backlog item) — this is new scope relative to the original audit findings,
   which only anticipated a selector, not a CRUD surface. See Scope below.

## Appetite

Large (3–6 weeks), with the caveat that the Runtime Configurability Decision above adds real
scope (migration, caching, a CRUD management surface) beyond the original audit's estimate. This
is explicitly the "core software-factory gap" per the audit's `is-it-ready` verdict (Architecture
🔴, Goal Compliance 🔴) — not a quick patch. Scope must still be cut (see Out of Scope) to fit;
do not extend the timeline instead. If Phase 3 planning finds the DB-persisted CRUD surface alone
doesn't fit inside the remaining appetite, cut the management UI to a minimal/CLI-first form
before cutting the caching design or the security/structure guarantees above.

## Constraints

- Must reuse the narrow-interface + deep-copy-on-construct pattern already established by
  `session/workflow_engine.go` (`WorkflowEngine`) rather than inventing a new abstraction
  style — flagged in the audit as "positive pattern to reuse," and required by
  `.claude/rules/interface-pollution-checklist.md` (no speculative interfaces, no
  Java/Spring-shaped layering).
- Any new per-item field follows the existing `BacklogItemData`/`BacklogItemUpdate` pattern in
  `session/repository.go` (plain struct fields, `*T` for optional partial-update semantics) —
  see the bucket-[2] finding that non-optional proto3 bools already caused a silent
  flag-clobbering bug; a new field must not repeat that mistake.
- Any new session-creation-adjacent surface (a "which pipeline" selector) must satisfy
  `.claude/rules/session-creation-registry.md`'s 7-touchpoint checklist if it behaves like a
  session-creation mode; must satisfy `.claude/rules/feature-registry.md` regardless.
- ADR-013 (`docs/adr/013-workflow-engine-replaces-valid-transitions.md`) is **Proposed**, not
  fully implemented — its Phase 2 (`ConfiguredWorkflowEngine`, custom states) was never built and
  remains explicitly out of scope here (see Runtime Configurability Decision above — this
  project's DB-persistence choice applies to `PipelineEngine` only, not `WorkflowEngine`). This
  project's `PipelineEngine` is a sibling concept (which skills/commands run) not the same as
  `WorkflowEngine` (which status transitions are legal) — do not conflate the two seams.
  `ConfiguredWorkflowEngine`'s never-built design is still useful precedent for schema/interface
  *shape*, but `PipelineEngine`'s DB-backing must NOT reuse `WorkflowRepository`'s query-per-call
  pattern verbatim — it needs its own caching layer (see Non-functional Requirements below).
- `session.Workflow`/`WorkflowRepository` (`session/workflow_repository.go`,
  `session/ent_workflow_repository.go`) is the direct precedent for the new DB-persisted pipeline-
  mode table's *shape* (slug, name, description, enabled) and CRUD surface — reuse its pattern for
  schema/repository design, but do NOT reuse its caching behavior (it has none) without adding one,
  per the Runtime Configurability Decision above.

## Non-functional Requirements

- **Performance SLO**: pipeline-mode resolution must not add an *uncached* synchronous DB round-
  trip to every triage/review/spawn call. Because pipeline modes are now DB-persisted (see Runtime
  Configurability Decision), this requires an explicit in-process cache in front of the mode
  table — invalidated on write (create/update/delete/enable/disable a mode) or refreshed on a
  short TTL, Phase 3's choice — not a direct query per call the way `WorkflowRepository` does
  today. `BacklogItemData` itself (already loaded on every relevant path) only needs to carry the
  chosen mode's *identifier* (slug/string); resolving that identifier to concrete behavior is what
  must be cached.
- **Scalability**: not applicable — single-operator tool, no multi-tenant concerns. This is also
  the load-bearing assumption behind relaxing the injection-risk posture below — re-evaluate if
  this tool ever becomes multi-tenant.
- **Security classification**: internal, single-operator. The threat model is **not** an untrusted
  third party — the operator defining a pipeline mode through the UI is the same person who could
  otherwise edit Go source and redeploy, so this is not a privilege boundary. The real requirement
  is **structural integrity, not access control**: mode definitions must be structured (typed
  fields: name, description, and a small number of content fields), not a single unstructured
  free-text blob, so a malformed definition fails predictably (validation error) rather than
  producing a broken prompt, a corrupted slash-command file, or unexpectedly executing something
  the operator didn't intend. Never string-interpolate a mode's raw content field directly into a
  shell/command-line context (as opposed to a prompt/markdown-file context, which is the intended
  use) — that boundary stays a hard rule regardless of the relaxed trust model.
- **Data residency**: not applicable.

## Scope

### In Scope

- A new DB-persisted pipeline-mode table (ent schema + migration), shaped after
  `session.Workflow`'s slug/name/description/enabled pattern, holding the structured content each
  mode maps to (Phase 3 planning defines the exact field set — at minimum enough to drive
  `WriteSlashCommands`'s file set and the triage/review prompt content per mode).
- A new per-item field (or small set of fields) on `BacklogItemData` holding the *identifier*
  (slug/string) of the chosen pipeline mode, following the existing optional-field pattern — the
  item stores a reference, not the mode's content.
- Proto changes to expose the new field(s) through `CreateBacklogItemRequest`,
  `UpdateBacklogItemRequest`, and `BacklogItem` (mirroring how `AutoSpawnSession` was added), plus
  new RPCs for CRUD on pipeline-mode definitions themselves (mirroring `WorkflowRepository`'s
  Create/Update/Delete/GetBySlug/ListAll/ListEnabled surface).
- A `PipelineEngine`-shaped seam (interface defined in the consuming package, narrow,
  ~~1-3 methods~~ **RESOLVED — 5 methods (`SlashCommandSet`, `TriagePromptFor`, `ReviewPromptFor`,
  `InitialPromptFor`, `ContentHashFor`), see plan.md Pattern Decisions** — 1-3 was the
  planning-time guess; Phase 3 planning found `InitialPromptFor` (autonomous-mode prompt content)
  and `ContentHashFor` (content-drift protection for session snapshots) both necessary to avoid the
  seam being cosmetic or losing history-integrity, not optional extensions) that `WriteSlashCommands`, the triage prompt builder, and the review-gate runner
  consult instead of hardcoding behavior — start with a `DefaultPipelineEngine`/default mode row
  that reproduces today's fixed behavior exactly (zero regression), backed by the new DB-persisted
  registry with an explicit cache (see NFR above), then let a user define one or two real
  alternative modes through the UI to prove the seam actually varies behavior end-to-end.
- UI: (a) a selector in `BacklogItemForm.tsx` for choosing an existing pipeline mode on a backlog
  item (same interaction pattern as the `autoSpawnSession` checkbox / the `SESSION_TYPES` radio
  group), (b) a management surface for creating/editing/enabling/disabling pipeline-mode
  definitions themselves (new scope — Phase 3 planning decides where this lives, e.g. a
  `/settings/pipeline-modes` page mirroring `/settings/backlog-sources`), and (c) a read-only "what
  ran" surface in `BacklogItemDetail.tsx` (addresses the UX-review finding).
- `docs/adr/` — a new ADR (or an update to ADR-013 marking its scope as WorkflowEngine-only, with
  a fresh ADR for PipelineEngine) recording the design decision before implementation, per this
  project's own SDD conventions. The ADR must explicitly record the Runtime Configurability
  Decision and its rationale (see above), since it reverses Phase 2 research's initial
  recommendation and future readers need that context.

### Out of Scope

- `docs/adr/013`'s Phase 2 `ConfiguredWorkflowEngine` (custom **states**, not custom
  **skills/stages**) — a related but separate initiative; do not implement it as part of this
  project even though the two would eventually compose.
- Reworking `BacklogBoard.tsx`'s hardcoded `COLUMNS` to support fully custom status sets — only
  the *skill/command* configurability is in scope; the status state machine itself stays as-is.
- `session/autonomous_driver.go:336-341`'s hardcoded orchestration prompt/signals — flagged in
  the audit as a related hardcoding instance, but reworking the `AutonomousDriver` itself is a
  larger, separate concern from picking *which* skill/command set an item uses. Defer to a
  follow-up project once `PipelineEngine` exists and this becomes a natural extension point.
- Multi-tenant / multi-user pipeline permissions (who is allowed to choose which pipeline) — this
  remains a single-operator tool.
- `backlog_service_triage.go:72-97`'s global tuning constants (`maxAutoReworkIterations`,
  `maxConcurrentBacklogWorkItems`, `defaultTriageCleanupTimeout`) — noted in the audit as
  hardcoded "operational tuning knobs," but they are process-level concurrency/safety limits,
  not pipeline/skill selection; out of scope here.

## Rabbit Holes

- **`WriteSlashCommands` currently writes files into the item's git worktree
  (`session/backlog_commands.go`)** — a `PipelineEngine` that varies *which* commands get
  written touches on-disk file generation, not just an in-memory decision. Scope creep risk:
  don't rebuild the slash-command templating system, just parameterize which fixed set gets
  selected.
- **Prompt construction for headless triage/review (`BuildHeadlessTriagePrompt`,
  `BuildHeadlessReviewPrompt`, `BuildReviewPrompt`) is currently free-text string building.**
  Making these pluggable per pipeline mode could balloon into a full templating engine. Resist
  that; start with 2-3 concrete prompt variants selected by mode, not a generic template DSL.
- **The relationship between `PipelineEngine` and the existing `WorkflowEngine`
  (ADR-013) is genuinely ambiguous** — a pipeline mode might legitimately want to add a status
  (e.g. a "quick-fix" mode that skips `review` entirely, not just via `SkipReviewGate`). Phase 3
  planning must explicitly decide whether `PipelineEngine` calls into `WorkflowEngine` or stays
  fully separate; don't let this get decided implicitly by whatever's easiest to code first.
- **`AutonomousDriver`'s hardcoded signals** (out of scope above) are still the actual execution
  engine most pipeline modes would drive through — there's a real risk that "add a
  `PipelineEngine` seam" ends up being cosmetic (selects a mode name) without actually changing
  runtime behavior, because the thing that would need to vary (`autonomous_driver.go`) is
  explicitly out of scope. Phase 3 planning must resolve this tension: either pull one concrete
  `AutonomousDriver` behavior change into scope as the "proof it's not cosmetic," or explicitly
  document that this phase only wires the seam and defers behavior-varying to the follow-up.

## Alternatives Considered

- **Free-text per-item "custom instructions" field appended to every prompt** — simplest
  possible implementation, no new schema/registry. **Still rejected even under the relaxed
  single-operator threat model** (see Runtime Configurability Decision): the objection was never
  purely about adversarial injection — it's that free text gives the UI nothing structured to
  display ("what ran") and fails predictably-on-error less well than typed fields. Structured,
  DB-persisted mode definitions (the chosen approach) get the runtime-editability this option
  offered without losing structure. May still be worth a *secondary*, clearly-labeled free-text
  "notes" field layered on top of a structured mode choice — Phase 3 planning can decide.
- **Let the user pick an arbitrary existing `/sdd:*` or other slash command by name, stored as a
  raw string** — similar rejection: no validation surface, easy to typo into a silent no-op,
  and doesn't compose with `WriteSlashCommands`'s existing fixed-set generation without either
  a registry lookup (which is basically `PipelineEngine` anyway) or raw string interpolation
  (security risk).
- **Skip the seam, hardcode 2-3 more `Skip*`-style booleans** (mirroring bucket [2]'s
  successful pattern) — rejected as the long-term approach because it doesn't scale: bucket [2]
  worked because there were ~6 known gates; bucket 3's ask is open-ended ("use my SDD skills for
  this item type"), which needs a real extensibility seam, not N more booleans.

## Feasibility Risks

- `session/workflow_engine.go`'s pattern was designed for status-transition legality, not
  skill/stage selection — reusing its *style* (narrow interface, deep-copy-on-construct) is
  low-risk, but the *shape* of `PipelineEngine`'s interface is unproven and needs its own
  design pass in Phase 3, not a mechanical copy.
- Risk that this becomes primarily a UI feature (a dropdown that doesn't change runtime
  behavior) if `autonomous_driver.go` changes are deferred out of scope — see the rabbit hole
  above. Phase 3 planning must explicitly resolve this before implementation starts.
- `ent` schema changes require the `--feature sql/upsert` regeneration flag
  (`.claude/rules/ent-schema-generation.md`) — low risk, well-documented, but a known footgun if
  skipped (silently breaks `UpsertRule`-style methods).
- **New risk from the Runtime Configurability Decision**: `WorkflowRepository` — the direct schema/
  CRUD precedent — has no caching layer at all (confirmed by direct code inspection). Phase 3
  planning is inventing a caching strategy for this codebase's *first* hot-path-consulted,
  DB-persisted, user-editable config table — there is no existing pattern to mechanically copy for
  the caching layer specifically (only for the schema/CRUD shape). Budget real design time for
  this; do not treat it as a solved problem just because the schema shape is precedented.
- **New risk**: a management CRUD UI for pipeline-mode definitions is new scope not anticipated by
  the original audit (which only expected a selector). If this doesn't fit the appetite, cutting it
  to a minimal/admin-only form (or even a one-time seed script plus direct-DB editing) is an
  acceptable descope per the Appetite section above — but the *selector* and the *caching-backed
  engine* must not be cut, since those are the actual behavioral seam.

## Observability Requirements

*(complexity ≥ 3)* Log which pipeline mode was resolved for a given item at each stage
transition (triage start, review start) at Info level, following the existing
`[BacklogLifecycle]`/`[TriggerTriage]` log-prefix convention — needed to debug "why did this
item run the wrong skill set" without new tooling. Additionally log cache misses/refreshes for the
new pipeline-mode cache (see NFR) at Debug level, and log at Warn (not silent fallback) whenever a
mode identifier fails to resolve — mirrors the "fail closed and loud on unrecognized value" lesson
from the prior audit's bug #4 (a `switch` that silently no-op'd on an unrecognized value). No new
metrics/alerts required; this is a single-operator tool with no oncall rotation.

## Risk Control

*(complexity ≥ 3)* The `DefaultPipelineEngine`/default mode row (in-scope, first milestone) must
be behaviorally identical to today's hardcoded path, so shipping it is a no-op by construction —
land it as its own reviewable commit before adding any second mode or exposing the CRUD UI, so a
regression in the seam itself (not a specific new mode) is caught in isolation. Additionally, since
mode definitions are now user-editable at runtime: an unresolvable/malformed mode on a live item
must fail closed to `DefaultPipelineEngine` behavior with a loud Warn log (per Observability
above), never a silent no-op or a crash — this is the concrete risk-control mechanism for the
self-inflicted-breakage risk accepted in the Non-functional Requirements' security discussion.

## Open Questions

- ~~Does `PipelineEngine` need to be DB-persisted/configurable at runtime...?~~ **RESOLVED
  2026-07-15 — yes, DB-persisted.** See "Runtime Configurability Decision" above. This reverses
  Phase 2 research's initial lean toward a code-defined registry; that research (particularly
  `research/pitfalls.md` §2-3 and `research/build-vs-buy.md`) still applies to the *structural
  integrity* validation and *interface shape* guidance, just not to the persistence-layer
  conclusion — Phase 3 planning must read those files with this resolution in mind, not take
  their persistence recommendation at face value.
- Should pipeline-mode selection happen once at item creation (immutable) or be changeable
  per-transition (e.g. escalate from "quick" to "full" mid-flight after a failed review)?
  `research/pitfalls.md` §4 recommends immutable-after-first-triage-session as the lower-risk
  choice for this phase (extending the existing `ItemSessionData.AcSnapshot` precedent if later
  phases need true mid-flight changes) — still open pending Phase 3's explicit decision, now with
  an added wrinkle: since modes are DB-mutable, an item's stored mode-slug could point at a mode
  whose *content* changed after the item started (not just whether the item's slug changed) —
  Phase 3 must decide whether to snapshot resolved mode content at triage-session start (mirroring
  `AcSnapshot`) or always resolve live.
- Exactly how does a chosen pipeline mode map to `WriteSlashCommands`'s output — a different
  fixed command set per mode, or a different *set of skills* invoked within the same command
  set? **Substantively answered by `research/features.md` §2**: `/sdd:quick` vs `/sdd:full` differ
  structurally (different control flow entirely), not by subsetting one shared template — supports
  each DB-persisted mode row owning its own self-contained content rather than one parameterized
  template with conditional sections.
