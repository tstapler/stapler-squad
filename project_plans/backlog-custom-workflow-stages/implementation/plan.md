# Implementation Plan: backlog-custom-workflow-stages

**Feature**: DB-persisted custom workflow stages/transitions with a typed transition-gate model and
per-stage(×pipeline-mode) liveness/staleness definitions, replacing the hardcoded `BacklogStatus`
state machine and per-`StuckReason` timing constants (ADR-013 Phase 2).
**Date**: 2026-09-03
**Status**: Ready for implementation
**ADRs**:
- `project_plans/backlog-custom-workflow-stages/decisions/ADR-001-liveness-engine-sibling-interface.md`
- `project_plans/backlog-custom-workflow-stages/decisions/ADR-002-configured-workflow-engine-and-gates.md`
- `project_plans/backlog-custom-workflow-stages/decisions/ADR-003-custom-gate-check-execution-bound.md`

**Task-sizing convention**: every task below carries an approximate LLM-agent-session sizing unit
(e.g. "~5 min"), not a wall-clock human-engineer estimate — per `subagent-driven-development`'s
per-task-dispatch model, the figure reflects roughly how much work fits in one fresh-subagent
dispatch, not calendar time. Relative implementation risk is signaled by the review docs
(architecture-review.md, adversarial-review.md, pre-mortem.md); the seven tasks those reviews
originally flagged as 10-40 min outliers (2.4.3b, 2.4.4b, 2.6.1b, 2.6.1g, 2.7.2g, 2.7.2h, 2.8.2c) were
decomposed at round-2 plan-repair (2026-09-03) into genuinely atomic sub-tasks each — see those
stories directly. All but one sub-task landed at ~5 min; Task 2.6.1g2 (the cross-transaction assertion
harness) is ~10 min because splitting it further would separate the harness from the single invariant
it verifies. No task-level range wider than ~10 min remains anywhere below; risk on the decomposed
areas is signaled by the parent story's narrative text and the review docs, not a wide time range.

---

## Milestone Structure (read this first)

This plan is split into two **independently mergeable and deployable** phases, per requirements.md's
explicit sequencing decision:

- **Phase 1 = Milestone 1** — `LivenessEngine` (a new sibling interface, not an extension of
  `WorkflowEngine`), a DB-backed per-stage(×pipeline-mode) liveness model, and migrating the
  `orphaned_triage`/`idea` sdd-vs-default timeout bug onto it. **Touches zero custom-stage or
  transition-gate code.** `BacklogStatus`, `validTransitions`, `WorkflowEngine`, and
  `TransitionGuard` are completely untouched by Phase 1 — only the liveness/staleness *threshold
  resolution* inside the existing stuck-detection sweeps changes, from a Go constant read to a
  `LivenessEngine.LivenessFor(...)` call that falls back to the identical constant when no config is
  set. This is what makes Phase 1 shippable and deployable on its own: it adds a new engine and a new
  ent table, wires the four surveyed call sites (`orphaned_triage`/`idea`'s Shape-A pair,
  `stale_work`'s Shape-B sweep, and `bouncing`'s Shape-C sweep) to consult it, and ships — no
  proto/UI/state-machine change is a precondition, and no Phase 2 code is a precondition of Phase 1.
- **Phase 2 = Milestone 2+** — `ConfiguredWorkflowEngine` (custom stages/transitions), the
  transition-gate model (`PendingGates`), the management UI, and re-routing the two
  `WorkflowEngine`-bypass call sites. Phase 2 **consumes** Phase 1's `LivenessEngine` (for a custom
  transition's liveness and for bounding custom/pluggable gate checks) but Phase 1 has zero
  dependency on Phase 2 — the arrow points one way.

```
Phase 1 (Milestone 1, independently shippable)          Phase 2 (Milestone 2+)
┌─────────────────────────────────────┐                 ┌─────────────────────────────────────┐
│ Epic 1.1 Liveness domain + ent       │                 │ Epic 2.1 Repo-wide bypass/whitelist audit│
│ Epic 1.2 DefaultLivenessEngine +     │                 │ Epic 2.2 Stage/transition/gate ent  │
│           characterization tests     │                 │ Epic 2.3 ConfiguredWorkflowEngine   │
│ Epic 1.3 Repo + cache + CRUD RPCs    │                 │ Epic 2.4 Gate evaluation + custom-  │
│ Epic 1.4 Wire into triage, stale-work│──consumed by──▶ │           check execution (uses     │
│  & bouncing sweeps (THE BUG FIX)     │                 │           Phase 1 LivenessEngine)   │
│ Epic 1.5 BUG-083 recovery regression │                 │ Epic 2.5 Snapshot-at-entry audit    │
│ Epic 1.6 Observability + ship gate   │                 │ Epic 2.6 Graph validation           │
└─────────────────────────────────────┘                 │ Epic 2.7 Proto/RPC CRUD surface     │
                                                          │ Epic 2.8 Stages settings UI          │
                                                          │ Epic 2.9 Board/detail dynamic stages │
                                                          │ Epic 2.10 Gate-checklist UI           │
                                                          │ Epic 2.11 ADR-013 resolution          │
                                                          └─────────────────────────────────────┘
```

---

## Domain Glossary

*(Ubiquitous language — every domain term appearing as a type, method, or variable name. Names here
must be used identically in code, tests, comments, and proto.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `StageSlug` | **Not a distinct Go type. Decision (round-2 plan-repair, 2026-09-03): `BacklogStatus` itself is formally widened to be the open type** — "one of 9 built-in values" becomes "one of 9 built-in values, or any operator-configured custom stage slug." "StageSlug" is an informal/aspirational label for that widened role, never a second type engineers convert between. | **Resolves architecture-review Concern 3 for real** (round 1's "same type, not distinct" wording restated the conflation without deciding anything; this is the actual decision). Chose widen-`BacklogStatus` (option b) over a distinct-`StageSlug`-with-conversion-boundary (option a) on verified evidence, not preference: (1) every current switch/case site over `BacklogStatus` already carries a `default:` branch with fail-safe behavior for an unrecognized value — `session/backlog_sync.go:533-540` (`default: return "", false`), `server/services/backlog_service_lifecycle.go:38-50` (`default: return`), `server/services/backlog_service_triage.go:2329-2342` (`default: return nil`), `server/mcp/tools_backlog.go:1391-1397` (`default:` rejects), `session/domain/backlog.go:541-614`'s `TransitionGuard` (`default:` at :611 — "no additional guards"), `server/services/autonomous_orchestration_service.go:460-473` (switch-true with its own default) — none assume/require 9-value closure; (2) `.golangci.yml:181-194` scopes the `exhaustive` linter to `session/detection/`'s `DetectedStatus` only, with an explicit comment "All other packages use iota types with intentional default: clauses" — no compiler/lint mechanism anywhere treats `BacklogStatus` as closed today; (3) Epic 2.3.1's own AC already passes a custom value (`"design-review"`) directly as a `BacklogStatus` argument with zero conversion — the epics as written already assume option (b), so option (a) would contradict already-written stories, not just be extra work. `BacklogStatus`'s 9 named constants remain the built-in subset; `LivenessEngine.LivenessFor`/`WorkflowEngine.CanTransition`/`AllowedTransitions`/`PendingGates` keep their existing, already-`Accepted` `BacklogStatus`-typed signatures unchanged (ADR-001/ADR-002) — option (b) requires no signature change to either. See Epic 2.1's new "Decision: `BacklogStatus` becomes the open stage-slug type" subsection and Story 2.1.3's extended scope for the follow-through audit. (`BacklogStatus`/`PipelineMode` remain defined types, not `type X = string` aliases, per `session/domain/backlog.go:13`/`session/pipeline_engine.go:36` — that part of round 1's wording was already correct.) |
| `LivenessEngine` | New sibling interface (to `WorkflowEngine`) resolving the `LivenessDefinition` for a `(BacklogStatus, PipelineMode)` pair — see `StageSlug`'s row above for why this is `BacklogStatus`, not a separate `StageSlug`-typed parameter. | Consumed by the `reconcile*` sweeps and `TriggerTriage`'s call-budget selection — never by `TransitionBacklogItemStatus`. |
| `DefaultLivenessEngine` | In-memory `LivenessEngine` reproducing every hardcoded `StuckReason` threshold surveyed in `research/architecture.md` §1, verbatim. | The zero-regression baseline; Phase 1's characterization tests assert against this. |
| `CachingLivenessEngine` | DB-backed `LivenessEngine` implementation: cache-first read, falls back to `DefaultLivenessEngine` on any miss/error with a Warn log. | Mirrors `CachingPipelineEngine` exactly. |
| `LivenessDefinition` | Tagged-union value type: a `LivenessKind` discriminator plus kind-specific fields (never one flat schema). | The single most important new type in Phase 1 — see Pattern Decisions. |
| `LivenessKind` | Enum: `LivenessKindDurationBudget` \| `LivenessKindHeartbeat` \| `LivenessKindCycleFrequency`. | Sum type; every consumer switch must be exhaustive (compiler-enforced via a lint or `default: panic` sentinel in tests). |
| `ExpectedDuration` | `time.Duration` field on a Shape-A (`LivenessKindDurationBudget`) `LivenessDefinition` — the bounded call's own timeout (e.g. what `triageCallBudget` is today). | |
| `StalenessMargin` | `time.Duration` field on a Shape-A `LivenessDefinition` — the buffer added to `ExpectedDuration`. | Never independently editable as an absolute threshold — see `StalenessThreshold`. |
| `StalenessThreshold` | **Method**, not a field: `ExpectedDuration + StalenessMargin` for Shape A. | This is the BUG-055 structural fix — a UI/RPC never accepts a raw threshold, only the two inputs it's derived from. |
| `MaxNoProgressDuration` | `time.Duration` field on a Shape-B (`LivenessKindHeartbeat`) `LivenessDefinition` — the no-progress ceiling (e.g. `maxWorkSessionStaleness`). | |
| `CycleThreshold` / `CycleLookback` | `int` / `time.Duration` fields on a Shape-C (`LivenessKindCycleFrequency`) `LivenessDefinition` — max transition-cycle count within a lookback window (e.g. `bounceThreshold`/`bounceLookback`). | |
| `LivenessRepository` | Narrow persistence interface for `LivenessDefinition` rows: `Create`, `Update`, `Delete`, `GetByStageAndMode`, `ListAll`. | Mirrors `WorkflowRepository`/`PipelineModeRepository`'s shape exactly. |
| `livenessCache` | Copy-on-write cache (`atomic.Pointer[map[string]resolvedLivenessDefinition]` + `sync.Mutex writeMu`) for resolved liveness rows. | Near-verbatim copy of `session/pipeline_engine.go`'s `pipelineModeCache`. |
| `resolvedLivenessDefinition` | Deep-copied-at-load-time snapshot of one `LivenessDefinition` row, held inside `livenessCache`. | Mirrors `resolvedPipelineMode`. |
| `GateStatus` | Struct describing one gate's current satisfaction state for a pending transition attempt: `GateID`, `Kind`, `Satisfied`, `Description`, `ActionHint`. | Returned by `WorkflowEngine.PendingGates`; drives the item-detail "what's blocking this" UI. |
| `GateKind` | Enum: `GateKindHumanApproval` \| `GateKindAutomatedReview` \| `GateKindStructural` \| `GateKindCustom`. | Sum type — matches the four "Actors for Transition Gates" in requirements.md exactly, by name. |
| `PendingGates` | New `WorkflowEngine` method: `PendingGates(item BacklogItemTransitionInput, to BacklogStatus) ([]GateStatus, error)`. | `ValidateGates` becomes `len(unsatisfied PendingGates(...)) == 0`. |
| `GateDefinition` | Persisted config for one gate attached to a `TransitionDefinition`: kind, kind-specific config, and whether it's re-checkable or one-shot. | |
| `TransitionDefinition` | Persisted config for one legal `(fromStage, toStage)` edge plus its ordered `[]GateDefinition`. | |
| `StageDefinition` | Persisted config for one stage (built-in or custom): slug, name, description, `IsEntry`, `IsTerminal`. | |
| `ConfiguredWorkflowEngine` | DB-backed `WorkflowEngine` implementation loading `StageDefinition`/`TransitionDefinition`/`GateDefinition` rows, sibling to `DefaultWorkflowEngine`. | Phase 2 only. |
| `GateSatisfactionRecord` | Persisted one-shot gate outcome (e.g. a recorded human approval, or a terminal review verdict) keyed by `(itemID, gateID)`. | Distinguishes stateful/one-shot gates from stateless/re-checkable ones (pitfalls.md §7). |
| `InvokeCustomGateCheck` | Orchestration function: spawns a named skill/slash-command bounded by a `LivenessDefinition` (Shape A) and records its `ReviewOutcome`-shaped verdict. | Phase 2; reuses Phase 1's `LivenessEngine`, not a new timeout mechanism. |
| `StageConfigSnapshot` | A frozen copy of a stage's `LivenessDefinition` + allowed transitions, captured on an item the moment it enters that stage. | Mirrors `AcSnapshot`'s "history is immune to later edits" discipline. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `LivenessEngine` | Sibling interface, not a `WorkflowEngine` method | This repo's `PipelineEngine`/`WorkflowEngine` precedent (PoEAA Service Layer boundary) | Fat `WorkflowEngine` with a 4th liveness method | Disjoint consumers (transition-time synchronous path vs. periodic-sweep path) and disjoint reasons to change — folding them violates interface segregation and `.claude/rules`-adjacent `interface-pollution-checklist` guidance |
| `LivenessEngine` | Sibling interface, not a facade wrapping `WorkflowEngine`+`PipelineEngine`+itself | — | A `PolicyEngine` facade over all three | Reintroduces a god-object the two existing sibling interfaces deliberately avoided; no independent consumer needs the facade, only individual callers of each interface |
| `WorkflowEngine` gate support | Extend with exactly one new method (`PendingGates`); `ValidateGates` becomes a thin wrapper | GoF Strategy (pluggable rule source) | New sibling `GateEngine` interface | Gates and `CanTransition`/`ValidateGates` are the *same question* (same call sites, same clock: transition-attempt time) — a sibling here would be interface pollution, not segregation |
| `LivenessDefinition` | Tagged union (`LivenessKind` discriminator + kind-specific fields) | type-driven-design | Flat `{expectedDuration, stalenessThreshold}` single schema | Category error — 3 of 4 surveyed liveness shapes (heartbeat, cycle-frequency, by-design-indefinite) have no "duration budget" concept at all (`research/architecture.md` §1) |
| `LivenessDefinition` storage | One table, kind-specific nullable columns | PoEAA (single-table inheritance-adjacent) | Separate polymorphic table per `LivenessKind` | Over-normalization for a table expected to hold dozens of rows total, defined by one operator; 3x the CRUD/RPC surface for no correctness gain at this scale |
| `StalenessThreshold` | Derived method (`ExpectedDuration + StalenessMargin`), never an independently stored/editable field | type-driven-design (illegal states unrepresentable) | Two independently-editable absolute thresholds | This is BUG-055's exact race, structurally prevented rather than convention-enforced |
| `LivenessRepository` | Repository (PoEAA) | Fowler | Active Record calls directly on ent models from `session`/`server/services` callers | Matches existing `WorkflowRepository`/`PipelineModeRepository` precedent; keeps ent details out of engine/service code |
| `livenessCache` | Copy-on-write cache (`atomic.Pointer[map[...]]` + `sync.Mutex` held across the full DB-read-then-store sequence) | This repo's `pipelineModeCache` | `sync.RWMutex`-guarded map | A reader is never blocked behind a writer; already-proven pattern avoids inventing a second concurrency design for the same problem shape |
| `ConfiguredWorkflowEngine`/`CachingLivenessEngine` construction | Deep-copy-on-construct/on-cache-load | This repo's `DefaultWorkflowEngine`/`resolvedPipelineMode` precedent (GoF Prototype-adjacent) | Return live references into cached ent objects | Prevents concurrent readers from observing a partially-updated config object mid cache-swap |
| Gate taxonomy | Sum type / sealed set (`GateKind`: 4 named values) | type-driven-design | Open string enum for gate type | Compiler-enforced exhaustive handling; requirements.md explicitly bounds the custom-check surface rather than leaving the type set open |
| Config CRUD surface (liveness, stages, transitions, gates) | Service Layer (PoEAA): a 5-RPC quintet per entity (`Create/Update/Delete/Get/List`) | Fowler; mirrors `PipelineMode`'s existing RPC quintet | Transaction Script — ad hoc per-need RPCs | Matches the established convention already used twice (`WorkflowRepository`, `PipelineModeRepository`); a third bespoke shape would be inconsistent for no benefit |
| Custom-stage transition/gate editor UI | List-based CRUD form (stage list → nested transitions sub-list → gate checkbox-group), plus a **read-only** generated graph diagram | GitHub branch-protection's flat-checklist precedent + this repo's `PipelineModeForm.tsx` | Drag-and-drop node/edge canvas (`@xyflow/react`) | WCAG 2.5.1/2.5.7 dragging-gesture burden requiring a full separate keyboard path; unreadable past ~8-10 nodes (built-in set is already 9); disproportionate to a single-operator, dozens-of-stages-at-most scale |
| Custom-stage transition/gate editor UI | (same as above) | (same as above) | Monaco-editor free-text YAML/JSON graph config | No structural validation feedback inline — exactly the "validation only at save time" failure mode `research/ux.md` names as the most common workflow-editor complaint |
| Custom/pluggable gate check execution | Named skill/slash-command invocation, bounded by a `LivenessDefinition` (Shape A), verdict via a `report_progress`-style call | GoF Command (a bounded, named, pre-registered invocation) | Arbitrary shell command / script path execution | Explicitly rejected by requirements.md as the single largest scope-blowout/security risk in the whole project |
| Custom/pluggable gate check execution | (same as above) | (same as above) | Outbound webhook callback to an operator-hosted service | Reintroduces a network dependency, its own auth/retry/timeout semantics from scratch, and doesn't reuse `LivenessEngine` cleanly — a new integration surface this single-operator local tool doesn't need |
| Graph validation (cycle/reachability) | Bespoke DFS-based validator, with mandatory adversarial test coverage (self-loops, multi-cycle, disconnected components) | `research/build-vs-buy.md` §3 verdict | Adopt `dominikbraun/graph` | Graph is small and operator-authored (dozens of nodes, rarely changing) — this repo's own "a little copying over a little dependency" precedent applies once correctness is test-covered |
| Audit-trail immunity to later config edits | Snapshot-at-entry (`StageConfigSnapshot`), captured the moment an item enters a stage/gate evaluation | This repo's `AcSnapshot` precedent | Foreign-key reference resolved live at render time | History must render correctly even after the referenced stage/gate/transition is later edited or deleted (pitfalls.md §5) |
| Fail-closed direction | Two distinct rules: a liveness threshold falls back to `DefaultLivenessEngine`'s safe default duration; an unresolvable gate falls back to **blocking** the transition | This repo's fail-closed-and-loud house style + pitfalls.md §4's asymmetry finding | One generic "fall back to default" rule applied to both | A gate has no safe universal default ("pass") for an arbitrary custom transition; only a liveness threshold has one |
| `StageID`/`GateID` references on `BacklogItemUpdate` | Optional pointer field (`*string`), presence not truthiness | type-driven-design / requirements.md Constraints | Non-optional plain `string` field | Repeats the exact proto3-bool-clobbering bug class already documented for `SkipReviewGate` |

---

## Migration Plan

### Phase 1 migration: `stage_liveness_definitions`

- **Migration**: new ent schema `session/ent/schema/stage_liveness_definition.go` →
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per this
  repo's `CLAUDE.md` — the flag is mandatory). Generated `session/ent/*.go` output is **not
  committed** (gitignored); only the schema file is.
- **Table shape**: `id (uuid)`, `stage_slug (string, not empty)`, `pipeline_mode (string, nullable —
  NULL means "applies to all modes")`, `kind (string enum)`, `expected_duration_ms (int64,
  nullable)`, `staleness_margin_ms (int64, nullable)`, `max_no_progress_duration_ms (int64,
  nullable)`, `cycle_threshold (int32, nullable)`, `cycle_lookback_ms (int64, nullable)`, `enabled
  (bool, default true)`, `created_at`/`updated_at` (UTC-normalized `time.Now`, per `stack.md`'s
  finding that `workflow.go`'s UTC form — not `pipeline_mode.go`'s local form — is required
  whenever an RPC exposes an `expected_updated_at` CAS field). `UNIQUE(stage_slug, pipeline_mode)`.
  Indexes on `stage_slug`, `enabled`.
- **Reversibility**: additive-only migration (new table, no column changes to any existing table) —
  trivially reversible by dropping the table; no existing data is touched.
- **Zero-downtime strategy**: pure additive `CREATE TABLE`; no backfill needed since
  `CachingLivenessEngine` falls back to `DefaultLivenessEngine` on an empty table (identical to
  today's hardcoded behavior — see Risk Control).
- **Rollback procedure**: drop the table (or leave it — an empty/unreferenced table is harmless);
  revert the four call-site changes (Epic 1.4) to read the Go constants directly again. Since
  `CachingLivenessEngine`'s cache-miss behavior already equals the pre-migration constant, a
  mid-rollout revert produces zero behavior change for any item not explicitly configured.

### Phase 2 migration: `backlog_stages`, `stage_transitions`, `transition_gates`, `gate_satisfaction_records`

- **Migration**: four new ent schemas under `session/ent/schema/`, generated the same way.
- **Table shapes** (summary — full field lists in Epic 2.2):
  - `backlog_stages`: `id`, `slug (unique)`, `name`, `description`, `is_entry (bool)`, `is_terminal
    (bool)`, `enabled (bool)`, timestamps.
  - `stage_transitions`: `id`, `from_stage_id (FK)`, `to_stage_id (FK)`, `enabled`, timestamps.
    `UNIQUE(from_stage_id, to_stage_id)`.
  - `transition_gates`: `id`, `transition_id (FK)`, `kind (string enum)`, `config (JSON — kind-
    specific fields)`, `stateful (bool — one-shot vs. re-checkable)`, `order_index (int)`, timestamps.
  - `gate_satisfaction_records`: `id`, `item_id`, `gate_id (FK)`, `satisfied (bool)`, `satisfied_by
    (string — actor identity)`, `satisfied_at`, `outcome_detail (JSON, nullable — e.g. a
    `ReviewOutcome` payload)`. `UNIQUE(item_id, gate_id)`.
  - Per requirements.md's Constraints and `architecture.md` §6: **`BacklogStuckState` is not
    touched at all** — its `reason` column stays a plain unvalidated string, no FK added.
- **Reversibility**: additive-only; the built-in 9-stage/edge set ships as **seeded rows**, not
  hardcoded fallback behavior for `ConfiguredWorkflowEngine` specifically (unlike `DefaultLivenessEngine`,
  which stays purely in-memory) — see Epic 2.2's seed-migration task. Reversible by dropping all four
  tables and reverting `server/dependencies.go`'s wiring back to `NewDefaultWorkflowEngine()`.
- **Zero-downtime strategy**: additive `CREATE TABLE` + a one-time seed migration inserting the 9
  built-in stages/edges; `ConfiguredWorkflowEngine` is not wired into `server/dependencies.go` until
  Epic 2.3 is merged, so the tables can exist unused for as long as needed during rollout.
- **Rollback procedure**: revert `server/dependencies.go`'s engine construction to
  `NewDefaultWorkflowEngine()` (one line); the four new tables become inert. No data loss for
  existing items since `BacklogStatus` values are unchanged strings.

---

## Observability Plan

- **Logs** (per requirements.md's Observability Requirements, both phases):
  - Info: `[LivenessEngine] resolved liveness for stage=%s mode=%s kind=%s` at every stuck-detection
    sweep decision and triage-call-budget selection (Phase 1); `[ConfiguredWorkflowEngine] resolved
    transition %s->%s, N gate(s) pending` at every transition attempt and item-detail gate-status
    fetch (Phase 2).
  - Debug: `[LivenessEngine] cache refreshed: %d liveness rows` / `[ConfiguredWorkflowEngine] cache
    refreshed: %d stages, %d transitions, %d gates` on every cache load/invalidate (mirrors
    `pipelineModeCache`'s existing Debug line verbatim).
  - Warn: `[LivenessEngine] stage=%s mode=%s liveness config unresolved, falling back to default
    (%s)` on any cache miss/malformed row (Phase 1); `[ConfiguredWorkflowEngine] gate %s on
    transition %s->%s unresolved, blocking transition` on any gate-resolution failure (Phase 2) —
    note the two Warn lines encode **opposite fallback directions** per the fail-closed asymmetry in
    Pattern Decisions.
- **Metrics**: none required — single-operator tool, no oncall rotation (matches requirements.md
  verbatim).
- **Alerts**: no new alerts required.

## Risk Control

- **Feature flag**: not a separate flag — governed by config-row presence, the same
  `PipelineModeDefault`-sentinel-style mechanism `PipelineEngine` already uses. With zero
  `stage_liveness_definitions` rows (Phase 1) or zero `backlog_stages`/`stage_transitions` rows
  (Phase 2, before the seed migration), both new engines reproduce today's hardcoded behavior
  exactly. Emergency kill switch: revert `server/dependencies.go`'s engine construction to `nil`
  (`LivenessEngine`) / `NewDefaultWorkflowEngine()` (`WorkflowEngine`) — one-line change, matches
  `pipelineEngine`'s existing non-fatal-nil-on-construction-failure pattern.
- **Rollback procedure**: standard revert via PR close + revert commit for both phases; Phase 1 and
  Phase 2 can be reverted independently of each other since Phase 2 depends on Phase 1's interface
  but Phase 1 has zero Phase-2 dependency.
- **Staged rollout**: full rollout on merge for both phases (single-operator tool, no cohorts) — but
  Phase 2 must not merge until Phase 1's characterization-test gate (Epic 1.2) and the two
  bypass-call-site fixes (Epic 2.1) are both green, per the Risk Control section's explicit ordering
  requirement.

## Unresolved Questions

- [x] ~~Liveness on `WorkflowEngine` or a sibling interface?~~ Resolved: sibling `LivenessEngine`
  (architecture.md §3a, adopted verbatim).
- [x] ~~Gates on `WorkflowEngine` or a sibling?~~ Resolved: extend `WorkflowEngine` with
  `PendingGates` (architecture.md §3b, adopted verbatim).
- [x] ~~Liveness granularity — (stage), (stage×mode), or finer?~~ Resolved: (stage) with a sparse
  (stage×mode) nullable-mode override, `UNIQUE(stage_slug, pipeline_mode)` (architecture.md §2,
  adopted verbatim).
- [x] ~~Should the existing `StuckReason` enum be replaced by stage-derived reasons, or kept
  separate?~~ **RESOLVED** at Epic 2.4's new "Decision: `StuckReasonGateTimeout`" subsection (start of
  Epic 2.4): `StuckReason` stays a closed enum with exactly one new generic value,
  `StuckReasonGateTimeout`, for all custom-transition/custom-gate liveness timeouts. Decided up front,
  not left to whoever implements Task 2.4.4c.
- [ ] **Per-item liveness "snooze" override** (features.md §6) — a real, named gap but explicitly
  **deferred past this project's scope**: no story below implements it. Flagged here so it isn't
  silently forgotten; owner: reconsider as a follow-up project once Phase 2 ships and operator
  feedback confirms the need in practice.
- [ ] **Known, deliberately-deferred (pre-mortem P2 #3)**: `GateSatisfactionRecord` carries no
  config-version stamp, so a recorded gate satisfaction could later be read against an edited gate
  config — deferred because Epic 2.5's `StageConfigSnapshot` already gives audit-trail-*rendering*
  immunity, and gate-evaluation-*input* immunity is additional scope this Large project doesn't need
  until real gate-config edit frequency is observed in practice.
- [ ] **Known, deliberately-deferred (pre-mortem P3 #4)**: the three independently-invalidating caches
  (`pipelineModeCache`/`livenessCache`/`stageConfigCache`) have no cross-cache torn-read test for a
  multi-entity edit (e.g. a stage + its transitions + a gate edited in one Epic 2.8.2 form submit) —
  deferred because the single-operator threat model makes the race window practically negligible, not
  because the risk is unreal.
- [ ] **Known, deliberately-deferred (pre-mortem P2 #5)**: `ConfiguredWorkflowEngine`'s cutover has no
  shadow-mode validation against real production `backlog_items` before the one-line
  `server/dependencies.go` swap — deferred because the existing one-line revert kill switch already
  bounds a bad cutover to "revert and redeploy," not data loss, and a shadow-mode comparison harness is
  scope this plan doesn't otherwise need.

## Dependency Visualization

```
Phase 1 (Milestone 1)
──────────────────────
Epic 1.1 (ent schema, LivenessDefinition type)
   │
   ├──▶ Epic 1.2 (DefaultLivenessEngine + characterization tests)
   │        │
   │        ▼
   ├──▶ Epic 1.3 (LivenessRepository + CachingLivenessEngine + cache + CRUD RPCs)
   │        │
   │        ▼
   └──▶ Epic 1.4 (wire into reconcileOrphanedTriageItems + TriggerTriage + reconcileStaleWorkSessions
            │    + reconcileBouncingItems — THE FIX, all 4 surveyed liveness shapes)
            ▼
        Epic 1.5 (BUG-083 recovery regression test + set sdd-mode override)
            │
            ▼
        Epic 1.6 (observability + Milestone 1 ship gate) ──▶ SHIP / DEPLOY (independent)


Phase 2 (Milestone 2+) — depends on Phase 1's LivenessEngine (Epic 2.4 only), not on Phase 1 shipping
────────────────────────────────────────────────────────────────────────────────────────────────────
Epic 2.1 (repo-wide BacklogStatus-literal audit — BLOCKING, do first, no other Epic 2.x dependency)
   │
   ▼
Epic 2.2 (stage/transition/gate ent schema + seed migration)
   │
   ▼
Epic 2.3 (ConfiguredWorkflowEngine: CanTransition/AllowedTransitions/PendingGates + cache
          + zero-regression test vs. DefaultWorkflowEngine)
   │
   ├──▶ Epic 2.4 (gate evaluation: structural/human-approval/automated-review + custom-check,
   │              consumes Phase 1's LivenessEngine)
   │        │
   │        ▼
   ├──▶ Epic 2.5 (StageConfigSnapshot — audit-trail immunity)
   │
   ├──▶ Epic 2.6 (graph validation — cycle/reachability, bespoke DFS + adversarial tests)
   │
   ▼
Epic 2.7 (proto/RPC CRUD surface for stages/transitions/gates)
   │
   ├──▶ Epic 2.8 (Stages settings UI: list + form + transitions sub-list + gate checklist)
   ├──▶ Epic 2.9 (BacklogBoard/StageTracker dynamic stage rendering + unresolved-stage fallback)
   └──▶ Epic 2.10 (item-detail gate-checklist UI, generalizes GateVerdictBox)
            │
            ▼
        Epic 2.11 (ADR-013 resolution: mark Accepted/Implemented) ──▶ SHIP / DEPLOY
```

---

# Phase 1: Milestone 1 — Liveness Engine (independently shippable)

## Epic 1.1: Liveness domain model + ent schema

**Goal**: Define `LivenessDefinition`/`LivenessKind` as a Go tagged union and persist it in a new
ent table, with zero wiring into any consumer yet.

### Story 1.1.1: `LivenessKind` tagged union and `LivenessDefinition` type
**As a** backend engineer, **I want** a `LivenessDefinition` type that cannot represent an invalid
combination of kind and fields, **so that** a Shape-B (heartbeat) row can never be misread as a
Shape-A (duration-budget) row.
**Acceptance Criteria**:
- `LivenessDefinition.StalenessThreshold()` returns `ExpectedDuration + StalenessMargin` for
  `LivenessKindDurationBudget` and panics (or returns an error, per the chosen calling convention) for
  any other `LivenessKind`.
  - *Given* a `LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration: 30*time.Minute,
    StalenessMargin: 5*time.Minute}`, *When* `.StalenessThreshold()` is called, *Then* it returns
    `35*time.Minute` — never a value read from a separately-stored field.
- A `LivenessDefinition` constructed with `Kind: LivenessKindHeartbeat` and a non-zero
  `ExpectedDuration` is rejected by a validating constructor (`NewLivenessDefinition(...)`), not
  silently accepted.
  - *Given* `NewLivenessDefinition(LivenessKindHeartbeat, WithExpectedDuration(30*time.Minute))`,
    *When* constructed, *Then* it returns a non-nil error naming `ExpectedDuration` as invalid for
    `LivenessKindHeartbeat`.
**Files**: `session/liveness_definition.go` (new), `session/liveness_definition_test.go` (new)

##### Task 1.1.1a: Define `LivenessKind` and the `LivenessDefinition` struct (~5 min)
- Add `LivenessKind` string enum (3 values) and `LivenessDefinition` struct with all Shape A/B/C
  fields, doc comments citing the exact hardcoded constant each field replaces
  (`triageCallBudget`, `maxWorkSessionStaleness`, `bounceThreshold`/`bounceLookback`).
- Files: `session/liveness_definition.go`

##### Task 1.1.1b: Implement `StalenessThreshold()` and `NewLivenessDefinition` validating constructor (~5 min)
- `StalenessThreshold()` method; `NewLivenessDefinition` with functional-options-style field setters,
  returning an error for any kind/field mismatch.
- Files: `session/liveness_definition.go`

##### Task 1.1.1c: Table-driven tests for all 3 kinds + invalid combinations (~5 min)
- Cover: correct field access per kind, `StalenessThreshold()` derivation, rejected invalid
  combinations (each of the 3 kinds paired with each wrong kind's fields).
- Files: `session/liveness_definition_test.go`

### Story 1.1.2: `stage_liveness_definitions` ent schema
**As a** backend engineer, **I want** a DB table for `LivenessDefinition` rows keyed by
`(stage_slug, pipeline_mode)`, **so that** an operator's override survives a restart and is cheap to
query.
**Acceptance Criteria**:
- The ent schema enforces `UNIQUE(stage_slug, pipeline_mode)` at the DB layer.
  - *Given* an existing row `{stage_slug: "idea", pipeline_mode: "sdd", kind: "duration_budget", ...}`,
    *When* a second `Create` is attempted with the identical `(stage_slug, pipeline_mode)` pair,
    *Then* ent returns a constraint-violation error, not a silent duplicate row.
- `pipeline_mode` is nullable and a `NULL` row is distinguishable from an empty-string row.
  - *Given* a row with `pipeline_mode: nil`, *When* read back via `GetByStageAndMode("idea", "sdd")`
    and no `("idea","sdd")` row exists, *Then* `GetByStageAndMode` returns not-found — it is a dumb
    exact-match query only. The `(stage, mode) → (stage, nil)` mode-less fallback is owned exclusively
    by `livenessCache.Get` (Story 1.3.1); it is not re-implemented here. (Resolves architecture-review
    Concern 1: the fallback was previously specified at both this layer and the cache layer with no
    stated owner — the cache layer is now the single owner, matching `pipelineModeCache`'s precedent
    of doing resolution at the cache/engine layer.)
**Files**: `session/ent/schema/stage_liveness_definition.go` (new)

##### Task 1.1.2a: Write the ent schema file with all fields, indexes, unique constraint (~5 min)
- Fields per Migration Plan's table shape; UTC-normalized timestamps per `stack.md`'s finding.
- Files: `session/ent/schema/stage_liveness_definition.go`

##### Task 1.1.2b: Regenerate ent and confirm `go build ./...` passes (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; do
  not commit generated output (`.gitignore` already excludes `session/ent/*.go`/`session/ent/*/`).
- Files: none committed beyond the schema file already staged in 1.1.2a

---

## Epic 1.2: `DefaultLivenessEngine` + characterization tests (zero-regression baseline)

**Goal**: An in-memory `LivenessEngine` that reproduces every surveyed hardcoded constant exactly,
proven bit-for-bit identical via a characterization-test gate — this is the safety net the rest of
Phase 1 builds on.

### Story 1.2.1: `LivenessEngine` interface + `DefaultLivenessEngine`
**As a** backend engineer, **I want** a `LivenessEngine` interface with one in-memory implementation
matching today's constants, **so that** wiring it into a call site (Epic 1.4) is provably a no-op
until an operator configures an override.
**Acceptance Criteria**:
- `DefaultLivenessEngine.LivenessFor(BacklogStatusIdea, "")` (no pipeline mode) returns a
  `LivenessDefinition` whose `StalenessThreshold()` equals `35*time.Minute` (today's
  `maxHeadlessTriageSessionStaleness`).
  - *Given* a freshly-constructed `session.NewDefaultLivenessEngine()`, *When*
    `LivenessFor(session.BacklogStatusIdea, session.PipelineModeDefault)` is called, *Then* the
    returned `LivenessDefinition.Kind == LivenessKindDurationBudget` and
    `.StalenessThreshold() == 35*time.Minute`.
- `DefaultLivenessEngine.LivenessFor(BacklogStatusInProgress, "")` returns a Shape-B definition
  matching `maxWorkSessionStaleness` (2h).
  - *Given* the same engine, *When* `LivenessFor(session.BacklogStatusInProgress,
    session.PipelineModeDefault)` is called, *Then* `.Kind == LivenessKindHeartbeat` and
    `.MaxNoProgressDuration == 2*time.Hour`.
**Files**: `session/liveness_engine.go` (new)

##### Task 1.2.1a: Define `LivenessEngine` interface (~3 min)
- One method: `LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error)`.
- Files: `session/liveness_engine.go`

##### Task 1.2.1b: Implement `DefaultLivenessEngine` with a hardcoded table for every surveyed `StuckReason` (~5 min)
- One entry per built-in stage covering `stale_work`, `orphaned_triage`, `rework_blocked_stale`,
  `bouncing` thresholds (values from `architecture.md` §1); stages with no timeout concept
  (`plan_not_approved`, `blocked_by_dependency`) return a sentinel "no timeout" definition, not an
  error.
- Files: `session/liveness_engine.go`

##### Task 1.2.1c: Unit tests asserting every returned value against the literal current constant (~5 min)
- One test per stage/reason pair surveyed in `architecture.md` §1.
- Files: `session/liveness_engine_test.go` (new)

### Story 1.2.2: Characterization test — sweep decisions unchanged pre/post
**As a** backend engineer, **I want** a fixed corpus of item-state fixtures whose stuck/not-stuck
verdict is captured before and after any `reconcile*` sweep is wired to `LivenessEngine`, **so that**
Milestone 1's Risk Control guarantee ("bit-for-bit unchanged") is a test, not an assumption.
**Acceptance Criteria**:
- A fixture corpus with at least one item per liveness shape (A, B, C) produces an identical
  stuck/not-stuck decision and identical `reasonDetail` string (including the interpolated threshold
  value) whether the sweep reads the Go constant directly or resolves via `DefaultLivenessEngine`.
  - *Given* a fixture item in `in_progress` with `latestProgressAt` 2h1m in the past, *When*
    `reconcileStaleWorkSessions` runs against both the pre-migration code path and the
    `DefaultLivenessEngine`-backed path, *Then* both mark the item `StuckReasonStaleWork` with an
    identical `reasonDetail` string.
**Files**: `session/backlog_lifecycle_stuck_test.go` (extend), `session/liveness_characterization_test.go` (new)

##### Task 1.2.2a: Build the fixture corpus (one item per liveness shape) (~5 min)
- Reuse existing test-fixture helpers from `session/backlog_lifecycle_stuck_test.go`.
- Files: `session/liveness_characterization_test.go`

##### Task 1.2.2b: Write the before/after equality assertion harness (~5 min)
- Captures decision + `reasonDetail` from the current hardcoded path as the golden value; the same
  test is re-run after Epic 1.4's wiring lands to confirm it still passes (this task authors the
  golden values now, against pre-Epic-1.4 code).
- Files: `session/liveness_characterization_test.go`

---

## Epic 1.3: `LivenessRepository` + `CachingLivenessEngine` + CRUD RPCs

**Goal**: Make `LivenessDefinition` DB-persisted, cached, and settable by an operator (via RPC — no
UI in Phase 1, per the Milestone Structure note above).

### Story 1.3.1: `LivenessRepository` + `livenessCache`
**As a** backend engineer, **I want** a cached, fail-closed repository for `LivenessDefinition` rows,
**so that** resolving a stage's liveness never adds an uncached DB read to a hot path.
**`livenessCache.Get` is the single owner of the `(stage, mode) → (stage, nil)` sparse-override
fallback in this plan** — `EntLivenessRepository.GetByStageAndMode` (this story's own repository,
Task 1.3.1b) performs only an exact-match lookup, per Story 1.1.2's corrected AC. The fallback logic
exists in exactly one place, not two.
**Acceptance Criteria**:
- `livenessCache.Get("idea", "sdd")` is lock-free (never touches `writeMu`) and returns a miss when
  no `("idea","sdd")` row and no `("idea", nil)` row exist.
  - *Given* an empty cache, *When* `Get("idea", "sdd")` is called, *Then* it returns
    `(resolvedLivenessDefinition{}, false)` without blocking on any writer.
- `CachingLivenessEngine.LivenessFor` falls back to `("idea", nil)` when `("idea","sdd")` is absent,
  and to `DefaultLivenessEngine` when neither row exists, logging exactly one Warn line on the final
  fallback.
  - *Given* a cache containing only `("idea", nil)` with `ExpectedDuration=40m`, *When*
    `LivenessFor(BacklogStatusIdea, "sdd")` is called, *Then* it returns the `("idea", nil)` row's
    definition (40m), not `DefaultLivenessEngine`'s 30m default, and logs no Warn (a mode-less
    fallback is not a failure).
**Files**: `session/liveness_cache.go` (new), `session/liveness_engine.go` (extend with
`CachingLivenessEngine`)

##### Task 1.3.1a: Define `LivenessRepository` interface + `*CreateInput`/`*UpdateInput` structs (all-pointer, partial-update) (~5 min)
- Mirrors `WorkflowRepository`'s shape exactly.
- Files: `session/liveness_repository.go` (new)

##### Task 1.3.1b: Implement `EntLivenessRepository` (~5 min)
- `Create`, `Update`, `Delete`, `GetByStageAndMode`, `ListAll` against the ent schema from 1.1.2.
- Files: `session/ent_liveness_repository.go` (new)

##### Task 1.3.1c: Implement `livenessCache` (copy `pipelineModeCache`'s structure) (~5 min)
- `atomic.Pointer[map[string]resolvedLivenessDefinition]` keyed by `stage_slug + "\x00" + mode`;
  `writeMu`-guarded `refresh` shared by `Load`/`Invalidate`.
- Files: `session/liveness_cache.go`

##### Task 1.3.1d: Implement `CachingLivenessEngine.LivenessFor` with the two-level fallback + Warn log (~5 min)
- `(stage,mode)` → `(stage,nil)` → `DefaultLivenessEngine`, Warn only on the final fallback.
- Files: `session/liveness_engine.go`

##### Task 1.3.1e: Unit tests: cache hit, mode-less fallback (no Warn), full fallback (Warn logged) (~5 min)
- Files: `session/liveness_cache_test.go` (new)

### Story 1.3.2: `LivenessDefinition` CRUD RPCs (no UI — operator-callable only)
**As the** single backlog operator, **I want** to create/update/delete a `LivenessDefinition` row via
RPC, **so that** I can set the sdd-mode `idea`-stage override without a code change or redeploy.
**Acceptance Criteria**:
- `CreateLivenessDefinition` rejects a request whose `(stage_slug, pipeline_mode)` pair already has an
  enabled row.
  - *Given* an existing enabled row `("idea", "sdd")`, *When* `CreateLivenessDefinition` is called
    again with the same pair, *Then* it returns `connect.CodeAlreadyExists`.
- `UpdateLivenessDefinition` invalidates `livenessCache` on success.
  - *Given* a cached `("idea","sdd")` row with `ExpectedDuration=45m`, *When*
    `UpdateLivenessDefinition` changes it to `60m` and returns success, *Then* the next
    `LivenessFor(BacklogStatusIdea, "sdd")` call (no server restart) returns `60m`, not the stale
    cached `45m`.
**Files**: `proto/session/v1/backlog.proto` (extend), `server/services/backlog_service_liveness.go`
(new)

##### Task 1.3.2a: Add `LivenessDefinition` message + 5 RPCs to backlog.proto (~5 min)
- Mirror `PipelineMode`'s message/RPC shape (`proto/session/v1/backlog.proto:247`, `:995-1007`).
- Files: `proto/session/v1/backlog.proto`

##### Task 1.3.2b: `make proto-gen` and confirm generated code compiles (~3 min)
- Files: none (generated, gitignored per repo convention where applicable — confirm via `make build`)

##### Task 1.3.2c: Implement the 5 RPC handlers in `BacklogService`, each invalidating `livenessCache` on mutation (~5 min)
- Files: `server/services/backlog_service_liveness.go`

##### Task 1.3.2d: Wire `LivenessRepository`/`CachingLivenessEngine` construction into `server/dependencies.go` (~5 min)
- Same non-fatal-fallback-to-nil-then-Default pattern as `pipelineEngine`'s construction.
- Files: `server/dependencies.go`

##### Task 1.3.2e: RPC handler tests: duplicate-pair rejection, cache invalidation on update (~5 min)
- Files: `server/services/backlog_service_liveness_test.go` (new)

---

## Epic 1.4: Wire `LivenessEngine` into all surveyed stuck-detection sweeps (the actual fix)

**Goal**: Replace all four flat constants this project's Scope names —
`maxHeadlessTriageSessionStaleness`/`triageCallBudget` (Shape A, `orphaned_triage`/`idea`) and
`maxWorkSessionStaleness` (Shape B, `stale_work`)/`bounceThreshold`+`bounceLookback` (Shape C,
`bouncing`) — with `LivenessEngine.LivenessFor(...)` calls, so an operator override actually changes
behavior for every surveyed liveness shape, not only the motivating sdd-triage pair.

### Story 1.4.1: `reconcileOrphanedTriageItems` consults `LivenessEngine`
**As a** backend engineer, **I want** the stuck-detection sweep to resolve its staleness threshold
from `LivenessEngine` instead of the `maxHeadlessTriageSessionStaleness` constant, **so that** an
sdd-mode item gets the sdd-mode-specific threshold.
**Acceptance Criteria**:
- With no `("idea","sdd")` override configured, `reconcileOrphanedTriageItems`'s stuck decision for an
  sdd-mode item is unchanged from today (35m).
  - *Given* an sdd-mode item's open headless-triage session `CreatedAt` 34m ago and no liveness
    override row exists, *When* `reconcileOrphanedTriageItems` runs, *Then* the item is **not**
    marked stuck (unchanged from today's 35m constant).
- With a `("idea","sdd")` override of `ExpectedDuration=45m, StalenessMargin=10m` configured, the same
  item is not marked stuck at 34m (today it would still not be stuck either, since 34m < 35m — use a
  time past the *old* 35m threshold to show the fix).
  - *Given* the same override and the session `CreatedAt` 40m ago (past the old 35m constant, under
    the new 55m derived threshold), *When* `reconcileOrphanedTriageItems` runs, *Then* the item is
    **not** marked stuck — this is the concrete fix for the 12 parked items.
**Files**: `session/backlog_lifecycle_triage.go`

##### Task 1.4.1a: Add a `livenessEngine LivenessEngine` field to `BacklogLifecycleListener`, wired via constructor (~5 min)
- Same pattern as the existing `pipelineEngine` field.
- Files: `session/backlog_lifecycle.go`

##### Task 1.4.1b: Replace the `maxHeadlessTriageSessionStaleness` read with `l.livenessEngine.LivenessFor(...).StalenessThreshold()` (~5 min)
- Preserve the exact `reasonDetail` string interpolation, now using the resolved threshold value.
- Files: `session/backlog_lifecycle_triage.go`

##### Task 1.4.1c: Update Story 1.2.2's characterization test to confirm the pre/post decisions still match with zero rows configured (~3 min)
- Files: `session/liveness_characterization_test.go`

##### Task 1.4.1d: New regression test for the sdd-mode override actually taking effect (~5 min)
- The two Given-When-Then scenarios above as table-driven test cases.
- Files: `session/backlog_lifecycle_stuck_test.go`

### Story 1.4.2: `TriggerTriage`'s call budget consults `LivenessEngine`
**As a** backend engineer, **I want** the headless triage call's own timeout to come from the same
`LivenessDefinition` the sweep uses, **so that** `ExpectedDuration` and `StalenessThreshold` can never
drift apart for the same item (BUG-055's actual invariant).
**Acceptance Criteria**:
- `TriggerTriage`'s `context.WithTimeout` uses `LivenessFor(item.Status, item.PipelineMode).ExpectedDuration`,
  not the flat `triageCallBudget` constant, when a `LivenessEngine` is wired.
  - *Given* an sdd-mode item with a configured override (`ExpectedDuration=45m`), *When*
    `TriggerTriage` starts the headless call, *Then* the call's context timeout is `45m`, not the
    flat `30m` constant.
- With `LivenessEngine` nil (construction failed) or no override configured, behavior is byte-for-byte
  identical to today's `30m` constant.
  - *Given* `s.livenessEngine == nil`, *When* `TriggerTriage` runs, *Then* it falls back to the
    literal `triageCallBudget` constant, unchanged.
**Files**: `server/services/backlog_service_triage.go`

##### Task 1.4.2a: Add `livenessEngine session.LivenessEngine` field to `BacklogService`, wired via `NewBacklogService` (~5 min)
- Files: `server/services/backlog_service.go`

##### Task 1.4.2b: Replace `triageCallBudget` read at the `context.WithTimeout` call site with `LivenessFor(...).ExpectedDuration`, nil-guarded (~5 min)
- Files: `server/services/backlog_service_triage.go`

##### Task 1.4.2c: Regression test: sdd-mode override changes the call's actual timeout; nil engine falls back exactly (~5 min)
- Files: `server/services/backlog_service_triage_test.go`

##### Task 1.4.2d: Update the BUG-055 invariant test to assert the new derived-threshold relationship instead of the two old constants (~5 min)
- Port `TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin`'s *intent*:
  assert `LivenessFor(BacklogStatusIdea, mode).StalenessThreshold() >
  LivenessFor(BacklogStatusIdea, mode).ExpectedDuration` for every configured row, structurally (not
  just for the two hardcoded constants).
- Files: `session/liveness_definition_test.go`

### Story 1.4.3: `reconcileStaleWorkSessions` and `reconcileBouncingItems` consult `LivenessEngine`
**As a** backend engineer, **I want** the `stale_work` (Shape B) and `bouncing` (Shape C)
stuck-detection sweeps to resolve their thresholds from `LivenessEngine` instead of the
`maxWorkSessionStaleness`/`bounceThreshold`/`bounceLookback` package constants, **so that** all four
liveness shapes requirements.md's Scope names are actually migrated onto the new model — not just the
`orphaned_triage`/`idea` pair Stories 1.4.1–1.4.2 cover — closing the sync-drift risk class (BUG-055)
for the remaining two sweeps instead of leaving them as dead-on-arrival config
(`DefaultLivenessEngine`'s Shape B/C entries from Epic 1.2 already compute correct values; nothing
calls them for these two sweeps until this story). Reuses the `livenessEngine LivenessEngine` field
Task 1.4.1a already added to `BacklogLifecycleListener` — no new field.
**Acceptance Criteria**:
- With no `("in_progress", <mode>)` liveness override configured, `reconcileStaleWorkSessions`'s
  stuck decision and notify-message text are unchanged from today (2h, verbatim `maxWorkSessionStaleness`
  wording).
  - *Given* an item's active work session with `lastProgressAt` 1h59m in the past and no liveness
    override row exists, *When* `reconcileStaleWorkSessions` runs, *Then* the item is **not** marked
    `StuckReasonStaleWork` (unchanged from today's 2h constant).
- With an `("in_progress", "sdd")` override of `MaxNoProgressDuration=3h` configured, an sdd-mode
  item at 2h1m (past the old 2h constant, under the new 3h override) is not marked stale — the
  same before/after shape Story 1.4.1 established for `orphaned_triage`.
  - *Given* the override above and an sdd-mode item's active session `lastProgressAt` 2h1m ago,
    *When* `reconcileStaleWorkSessions` runs, *Then* the item is **not** marked
    `StuckReasonStaleWork`, and the `[BacklogLifecycle] item %s work session %s stale` notify body
    interpolates the resolved `3h`, not the bare `maxWorkSessionStaleness` constant, when the item
    does eventually cross it.
- With no `("in_progress", <mode>)` Shape-C override configured, `reconcileBouncingItems`'s
  `since` window and bounce-count decision are unchanged from today (`bounceThreshold=3`,
  `bounceLookback=24h`).
  - *Given* an item with 3 in_progress→review cycles in the last 24h and no PASS verdict, and no
    liveness override row exists, *When* `reconcileBouncingItems` runs, *Then* the item is marked
    `StuckReasonBouncing` (unchanged from today's `bounceThreshold=3`/`bounceLookback=24h`).
- With an `("in_progress", "sdd")` Shape-C override of `CycleThreshold=5, CycleLookback=48h`
  configured, an sdd-mode item with 3 cycles in the last 24h is **not** flagged (today's constants
  would also not flag it at 3 cycles, so use a cycle count past the *old* threshold to show the fix
  applies per-item, not globally).
  - *Given* the override above and an sdd-mode item with 4 in_progress→review cycles in the last
    30h (past the old `bounceThreshold=3` but under the new `CycleThreshold=5`) and no PASS verdict,
    *When* `reconcileBouncingItems` runs, *Then* the item is **not** marked `StuckReasonBouncing`,
    while a default-mode item with the identical 4-cycles-in-30h shape *is* still marked (proving the
    resolution is per-item, not a single package-level override).
**Files**: `session/backlog_lifecycle_stale.go`, `session/backlog_lifecycle.go`,
`session/stuck_decisions.go`

##### Task 1.4.3a: Change `staleWork`'s signature to take a resolved threshold instead of reading `maxWorkSessionStaleness` (~5 min)
- `staleWork(lastProgress, now time.Time) bool` (`session/stuck_decisions.go:82-84`) becomes
  `staleWork(lastProgress, now time.Time, maxNoProgress time.Duration) bool { return
  now.Sub(lastProgress) > maxNoProgress }`. Leave the `maxWorkSessionStaleness` constant
  (`session/backlog_lifecycle_stale.go:62`) in place as `DefaultLivenessEngine`'s own source value
  (per Epic 1.2's doc-comment convention pointing back at it) — only its *call site* changes, not its
  definition.
- Files: `session/stuck_decisions.go`

##### Task 1.4.3b: Resolve `MaxNoProgressDuration` via `LivenessEngine` at `reconcileStaleWorkSessions`'s call site and in the notify string (~5 min)
- At `session/backlog_lifecycle_stale.go:95`, resolve
  `l.livenessEngine.LivenessFor(BacklogStatusInProgress, PipelineMode(item.PipelineMode))` once per
  item (matching Epic 1.2's `LivenessFor(BacklogStatusInProgress, "")` mapping for the `stale_work`
  shape) before calling `staleWork(lastProgress, time.Now(), resolved.MaxNoProgressDuration)`; nil-guard
  `l.livenessEngine` the same way Task 1.4.2b nil-guards `BacklogService`'s field, falling back to the
  literal `maxWorkSessionStaleness` constant. Replace the bare `maxWorkSessionStaleness` interpolation
  at line 138's notify `fmt.Sprintf` with the resolved duration.
- Files: `session/backlog_lifecycle_stale.go`

##### Task 1.4.3c: Change `isBouncing` to take a resolved cycle threshold, and move the `since` window computation inside the per-item loop (~5 min)
- `isBouncing(cycleCount int, hasPass bool) bool` (`session/stuck_decisions.go:89-91`) becomes
  `isBouncing(cycleCount, cycleThreshold int, hasPass bool) bool { return cycleCount >= cycleThreshold
  && !hasPass }`. In `reconcileBouncingItems`, `since := time.Now().Add(-bounceLookback)`
  (`session/backlog_lifecycle.go:1562`) currently computed once before the loop must move inside the
  per-item loop (`session/backlog_lifecycle.go:1563-1629`), resolved per item via
  `l.livenessEngine.LivenessFor(BacklogStatusInProgress, PipelineMode(item.PipelineMode))` (same stage
  key as `stale_work`, since both sweeps scan the item's `in_progress` cycle-origin status), since a
  per-mode override means `CycleLookback` — and therefore the window passed to
  `CountReviewCyclesSince` at line 1631 — can now legitimately differ between items in the same tick.
  Nil-guard identically to Task 1.4.3b, falling back to the literal `bounceThreshold`/`bounceLookback`
  constants. Update the `isBouncing(count, hasPass)` call at line 1658 to pass the resolved
  `CycleThreshold`, and the `reasonDetail`/notify-body interpolations of `bounceLookback` at lines 1662
  and 1688 to use the resolved `CycleLookback` instead of the bare constant.
- Files: `session/backlog_lifecycle.go`

##### Task 1.4.3d: Extend Story 1.2.2's characterization corpus with `stale_work` and `bouncing` fixtures, confirm pre/post decisions match with zero rows configured (~5 min)
- Mirrors Task 1.4.1c for the two newly-wired sweeps.
- Files: `session/liveness_characterization_test.go`

##### Task 1.4.3e: New regression tests: per-mode override changes each sweep's actual decision; a sibling default-mode item on the identical fixture is unaffected (~5 min)
- The four Given-When-Then scenarios above as table-driven test cases, including the
  moved-inside-the-loop `since`/lookback computation with two items of different pipeline modes in
  the same `reconcileBouncingItems` tick.
- Files: `session/backlog_lifecycle_stuck_test.go`

---

## Epic 1.5: BUG-083 recovery-path regression + operator config for the 12 parked items

**Goal**: Prove the specific claim in Success Metrics — a row parked under the *old* hardcoded
thresholds recovers via the existing `RemediationDue` cold-retry heartbeat once Milestone 1 ships and
the sdd-mode override is set, with **no code change to `RemediationDue`/`evaluateRemediation`
required**.

### Story 1.5.1: Parked-row recovery regression test
**As a** backend engineer, **I want** an explicit test proving a `BacklogStuckState` row parked before
Milestone 1 ships becomes remediation-due again after the sdd-mode override is configured, **so that**
this isn't the 7th instance of BUG-083's "write-side fixed, recovery-side untested" pattern.
**Acceptance Criteria**:
- A `BacklogStuckState` row with `reason="orphaned_triage"`, `remediation_attempts=5` (parked, per
  `MaxRemediationAttempts`), and `next_remediation_at` in the past becomes `RemediationDue() == true`
  once the sdd-mode `LivenessDefinition` override is created — with zero code change to
  `backlog_remediation.go`.
  - *Given* a parked `BacklogStuckState{reason: "orphaned_triage", remediation_attempts: 5,
    next_remediation_at: <7 days ago>}` row (BUG-083's cold-retry heartbeat already due) and a newly
    created `("idea","sdd")` `LivenessDefinition` override, *When* the next `reconcile*` tick runs,
    *Then* `RemediationDue(itemID, "orphaned_triage")` returns `true` and the item re-attempts triage
    using the new, larger `ExpectedDuration` — with `session/backlog_remediation.go` untouched by this
    project.
**Files**: `session/backlog_remediation_test.go` (extend)

##### Task 1.5.1a: Write the parked-row fixture + assertion (~5 min)
- Files: `session/backlog_remediation_test.go`

##### Task 1.5.1b: Confirm via a live-code read (not just the test) that `backlog_remediation.go` needs zero edits for this scenario, and record that confirmation in the PR description (~3 min)
- Files: none (verification step)

### Story 1.5.2: Configure the sdd-mode override and verify against real parked items
**As the** single backlog operator, **I want** to set the actual `("idea","sdd")` override value once
Milestone 1 deploys, **so that** the 12 currently-parked items recover without manual per-item
intervention.
**Acceptance Criteria**:
- After deploying Milestone 1 and calling `CreateLivenessDefinition` for `("idea", "sdd")` with a
  duration sized to the observed sdd-triage call lengths (informed by `docs/tasks/backlog-feature-
  improvement.md`'s 2026-09-03 update), the 12 parked items (`306bbc57` and its 11 siblings) each
  transition off `STUCK_REASON_ORPHANED_TRIAGE` within one cold-retry cycle, with no manual
  per-item RPC call.
  - *Given* the 12 parked items and the newly-created override, *When* the next scheduled
    `reconcile*` tick after deploy runs, *Then* `list_backlog_items` (filtered to
    `stuck_reason=orphaned_triage`) returns fewer than 12 items within one retry window, with zero
    manual `ResetStuckRemediation` calls made.
**Files**: none (operational verification task, not a code change)

##### Task 1.5.2a: After Milestone 1 deploys, call `CreateLivenessDefinition("idea", "sdd", ...)` with the sized override (~5 min)
- Files: none (RPC call, documented in the PR/deploy notes)

##### Task 1.5.2b: Verify the 12 parked items' `stuck_reason` count drops post-deploy; record the before/after count in the deploy notes (~5 min)
- Files: none (operational verification)

---

## Epic 1.6: Observability + Milestone 1 ship gate

**Goal**: Add the required log lines and confirm the full regression suite (characterization +
BUG-083 recovery + BUG-055 invariant) is green before Milestone 1 is considered shippable.

### Story 1.6.1: `[LivenessEngine]`-prefixed log lines
**As an** on-call-free single operator, **I want** Info/Debug/Warn log lines for every liveness
resolution, cache refresh, and fallback, **so that** I can debug "why didn't my override apply"
without new tooling.
**Acceptance Criteria**:
- Every `LivenessFor` call that falls all the way back to `DefaultLivenessEngine` logs exactly one
  `[LivenessEngine]`-prefixed Warn line naming the stage and mode.
  - *Given* a request for `("custom-typo-stage", "sdd")` with no matching row, *When*
    `LivenessFor` is called, *Then* one Warn line reading `[LivenessEngine] stage=custom-typo-stage
    mode=sdd liveness config unresolved, falling back to default (...)` is emitted, and the returned
    value is `DefaultLivenessEngine`'s value for that stage (or its own fail-closed default if the
    stage itself isn't in `DefaultLivenessEngine`'s table).
**Files**: `session/liveness_engine.go`, `session/liveness_cache.go`

##### Task 1.6.1a: Add the Info/Debug/Warn log lines per the Observability Plan (~5 min)
- Files: `session/liveness_engine.go`, `session/liveness_cache.go`

##### Task 1.6.1b: Test that the Warn line fires exactly once per unresolved call, not per internal retry (~3 min)
- Files: `session/liveness_engine_test.go`

### Story 1.6.2: Milestone 1 ship gate
**As a** backend engineer, **I want** one CI-verified gate confirming all Milestone 1 regression
tests pass together, **so that** Milestone 1 can be merged and deployed independently of Phase 2.
**Acceptance Criteria**:
- `go test ./session/... ./server/services/... -run 'Liveness|OrphanedTriage|BUG055|BUG083'` passes
  with zero failures, and `make quick-check` passes on the Milestone 1 branch with no Phase 2 code
  present.
  - *Given* the Milestone 1 branch (Epics 1.1–1.5 merged, no `ConfiguredWorkflowEngine`/gate code),
    *When* `make quick-check` runs, *Then* it exits 0.
**Files**: none (verification task)

##### Task 1.6.2a: Run the full Milestone 1 test matrix and attach output to the PR (~5 min)
- Files: none

##### Task 1.6.2b: Run `make quick-check` and confirm lint/build/test all pass (~5 min)
- Files: none

---

# Phase 2: Milestone 2+ — Custom Stages, Transition Gates, Management UI

## Epic 2.1: Repo-wide `BacklogStatus`-literal audit (BLOCKING — do before any `ConfiguredWorkflowEngine` code)

**Goal**: Close the two confirmed `session.CanTransitionBacklog`-bypass call sites
(`server/services/backlog_service_lifecycle.go:1105`, `server/services/backlog_service_sync.go:151`)
before `ConfiguredWorkflowEngine` ships, per pitfalls.md's explicit instruction — otherwise a custom
transition's gates are silently bypassable through these two paths on day one. **Rescoped per
pre-mortem P1 #1**: those two call sites are `CanTransitionBacklog`-specific, but they are not the
only place the codebase hardcodes assumptions about the closed 9-`BacklogStatus` set. This epic now
also runs a repo-wide sweep for every literal `BacklogStatus` comparison/switch/whitelist outside the
transition table itself (`session/backlog.go`'s `validTransitions`/`TransitionGuard`,
`session/domain/backlog.go`'s `CanTransitionBacklog`) — explicitly including
`server/mcp/tools_backlog.go`, named in requirements.md's own Open Questions but absent from this
epic's original task list — with each finding becoming its own blocking re-routing task, not an
unscoped "audit it later."

**Verification performed at plan-repair time (2026-09-03)**, via `grep -rn "BacklogStatus" ...` and
targeted reads across `session/`, `server/services/`, and `server/mcp/` (excluding generated
`session/ent/*` and the domain enum definition itself): confirms (a) `CanTransitionBacklog` has
exactly the two known direct callers outside `session/backlog.go`/`domain/backlog.go` — no new
`CanTransitionBacklog`-bypass site exists beyond Stories 2.1.1/2.1.2 — and (b) two distinct classes of
literal-`BacklogStatus` logic exist outside any `CanTransitionBacklog` call at all, seeded as Stories
2.1.3 and 2.1.4 below. This is a snapshot, not a substitute for the sweep itself — line numbers will
have moved by the time Epic 2.1 is implemented (see the adversarial review's Minor #2 on exactly this
staleness risk), so Task 2.1.3a re-runs the sweep against then-current source rather than trusting
this list verbatim.

### Decision: `BacklogStatus` becomes the open stage-slug type (resolved here, not left as a glossary footnote)

**Decision**: `BacklogStatus` is formally widened, going forward, from "one of 9 built-in values" to
"one of 9 built-in values, or any operator-configured custom stage slug" — no distinct `StageSlug` Go
type is introduced, and no conversion boundary is added between "built-in" and "custom" call sites.
See the Domain Glossary's `StageSlug` entry for the full rationale; in summary: every current
switch/case site over `BacklogStatus` already fails safe via a `default:` branch (verified at
plan-repair time, 2026-09-03 — see the glossary entry for the six file:line citations), the
`exhaustive` linter is deliberately scoped away from `BacklogStatus` (`.golangci.yml:181-194`), and
Epic 2.3.1's own AC already passes a custom value as a bare `BacklogStatus` argument. This was
previously left as reworded-but-unresolved conflation in the glossary (round-2 triad review's
Engineering-lens finding); it is decided here, before Epic 2.1's audit work starts, so Story 2.1.3
below has a settled target to audit against rather than each finding being triaged against an
undecided question.

**Consequence for this epic**: Story 2.1.3's sweep task now also confirms each already-`default:`-
guarded switch site's fail-safe behavior is the *semantically correct* one for a custom stage (not
merely non-crashing), and updates `BacklogStatus`'s doc comment to state the type is open. See Task
2.1.3e below.

### Story 2.1.1: Re-route `OverrideVerdict`'s direct `CanTransitionBacklog` call through the injected engine
**As a** backend engineer, **I want** `OverrideVerdict`'s transition check to go through
`s.engine.CanTransition`, **so that** a `ConfiguredWorkflowEngine`'s custom-transition rules aren't
silently skipped by this one RPC.
**Acceptance Criteria**:
- `OverrideVerdict` refuses a transition `ConfiguredWorkflowEngine.CanTransition` would refuse, even
  though `domain.CanTransitionBacklog` (the static map) would allow it.
  - *Given* a `ConfiguredWorkflowEngine` with a custom transition graph where `review -> done` has
    been disabled (edge removed), *When* `OverrideVerdict` is called with `to_status="done"` on a
    `review`-status item, *Then* it returns `connect.CodeInvalidArgument`, matching what
    `s.engine.CanTransition` (not the static map) says.
**Files**: `server/services/backlog_service_lifecycle.go:1105`

##### Task 2.1.1a: Replace `session.CanTransitionBacklog(from, toStatus)` with `s.engine.CanTransition(from, toStatus)` at line 1105 (~3 min)
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 2.1.1b: Regression test: `OverrideVerdict` respects a `ConfiguredWorkflowEngine`-disabled edge (~5 min)
- Files: `server/services/backlog_service_lifecycle_test.go`

### Story 2.1.2: Re-route `AttachSessionToItem`'s direct `CanTransitionBacklog` call
**As a** backend engineer, **I want** `AttachSessionToItem`'s in_progress transition check to go
through `s.engine.CanTransition`, **so that** attaching a session to an item can't silently skip a
configured transition rule.
**Acceptance Criteria**:
- Same shape as Story 2.1.1, applied to line 151's call site.
  - *Given* the same disabled-edge `ConfiguredWorkflowEngine` scenario but for
    `ready -> in_progress`, *When* `AttachSessionToItem` runs, *Then* it does not call
    `TransitionBacklogItemStatus` and logs the same "transition failed" path it already has for a
    legitimately-rejected transition today.
**Files**: `server/services/backlog_service_sync.go:151`

##### Task 2.1.2a: Replace `session.CanTransitionBacklog(...)` with `s.engine.CanTransition(...)` at line 151 (~3 min)
- Files: `server/services/backlog_service_sync.go`

##### Task 2.1.2b: Regression test mirroring 2.1.1b for this call site (~5 min)
- Files: `server/services/backlog_service_sync_test.go`

### Story 2.1.3: Repo-wide sweep + terminal-status literal-comparison consolidation
**As a** backend engineer, **I want** a documented, repo-wide sweep for every literal
`BacklogStatus` comparison/switch/whitelist outside the transition table, **so that** a custom stage
doesn't silently break or get mis-bucketed at a site nobody thought to check.
**Acceptance Criteria**:
- The sweep (`grep -rn "BacklogStatus[A-Z][a-zA-Z]*" --include="*.go" session server/services
  server/mcp`, excluding `session/ent/*`, `*_test.go`, `session/domain/backlog.go`'s enum
  definition, and `session/backlog.go`'s `CanTransitionBacklog`/`validTransitions` re-export) is
  re-run against current source, and every finding is triaged into exactly one of: (1) a blocking
  re-routing task added to this epic, (2) a documented "reviewed, benign" note inline in this story
  (e.g. constructing a literal status value when creating a new item, which is not affected by a
  custom stage set), or (3) a documented, explicit scope-boundary note citing requirements.md's
  "does not automatically inherit" resolution (e.g. MCP self-resolve target-status hardcoding — see
  Story 2.1.4's boundary note). No finding is silently left untriaged.
  - *Given* the sweep's output, *When* every line is triaged, *Then* the PR description lists the
    full triage table (site → category), not just the sites that got a code change.
- The **terminal-status check** (`status == BacklogStatusDone || status == BacklogStatusArchived`,
  confirmed today at `session/backlog_lifecycle.go:1781`, `session/backlog_lifecycle_pr.go:834`,
  `session/ent_repository_backlog.go:579,606,628-631`, `server/services/backlog_service_lifecycle.go:43,791`,
  `server/services/deep_link_resolver.go:276`, and `server/mcp/tools_backlog.go:630,663,678`) is
  consolidated behind one `IsTerminalStatus`-shaped check that a custom terminal stage satisfies too,
  not a literal two-value OR repeated at every call site.
  - *Given* a custom stage `"legal-review"` marked `IsTerminal: true`, *When* an item reaches that
    stage and `wait_for_backlog_event`'s `buildMatchedWaitResult`/`currentStateWaitResult`
    (`server/mcp/tools_backlog.go:630`/`:663`) evaluate it, *Then* `IsTerminal` in the returned result
    is `true` — not `false`, which is what the literal `Done`/`Archived` OR would (incorrectly)
    return today for a custom terminal stage.
**Files**: `session/graph_validator.go` or a new `session/terminal_status.go` — Task 2.1.3a's triage
table decides which, based on whether `ConfiguredWorkflowEngine` or a narrower helper is the right
owner — plus every call site listed above.

##### Task 2.1.3a: Re-run the sweep against current source and produce the full triage table (~5 min)
- Files: none (produces the PR-description table the AC above requires)

##### Task 2.1.3b: Define a single `IsTerminalStatus`-equivalent check (built-in constant fallback when no `ConfiguredWorkflowEngine`/`StageDefinition` is wired, `StageDefinition.IsTerminal` lookup when it is) (~5 min)
- Files: `session/graph_validator.go` (or `session/terminal_status.go`, per 2.1.3a's finding)

##### Task 2.1.3c: Re-route every confirmed terminal-check call site (session/backlog_lifecycle.go, session/backlog_lifecycle_pr.go, session/ent_repository_backlog.go, server/services/backlog_service_lifecycle.go, server/services/deep_link_resolver.go, server/mcp/tools_backlog.go) to the new check, one call site at a time (~5 min each, ~8 call sites confirmed today — re-verify count via 2.1.3a)
- Files: as listed above

##### Task 2.1.3d: Regression tests: built-in Done/Archived behavior is unchanged; a custom `IsTerminal` stage is recognized at every re-routed call site (~5 min)
- Files: `server/mcp/tools_backlog_test.go`, plus a test alongside each re-routed call site

##### Task 2.1.3e: Per this epic's new "Decision: `BacklogStatus` becomes the open stage-slug type" subsection — audit the six already-`default:`-guarded `BacklogStatus` switch sites confirmed at plan-repair time (`session/backlog_sync.go:533`, `server/services/backlog_service_lifecycle.go:38`, `server/services/backlog_service_triage.go:2329`, `server/mcp/tools_backlog.go:1391`, `session/domain/backlog.go:541` `TransitionGuard`, `server/services/autonomous_orchestration_service.go:460`), confirming each `default:` branch's behavior is the intentional, correct one for a custom stage value (not merely non-crashing) and adding a one-line comment at each citing this decision; update `BacklogStatus`'s doc comment (`session/domain/backlog.go:12`) to state the 9 constants are the built-in subset of an otherwise-open type (~5 min)
- Files: `session/backlog_sync.go`, `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_triage.go`, `server/mcp/tools_backlog.go`, `session/domain/backlog.go`, `server/services/autonomous_orchestration_service.go`

### Story 2.1.4: `server/mcp/tools_backlog.go`'s status whitelists — re-route or document as an explicit scope boundary
**As a** backend engineer, **I want** each of `tools_backlog.go`'s four hardcoded `BacklogStatus`
whitelist maps/slices resolved one by one, **so that** the ones that should recognize custom stages do,
and the ones that intentionally don't are a documented decision, not a silent gap.
**Acceptance Criteria**:
- `validBacklogStatuses` (`server/mcp/tools_backlog.go:147`, backing `list_backlog_items`'s status
  filter validation) is sourced from `ConfiguredWorkflowEngine`'s live stage list, not a fixed
  9-entry slice — a valid custom stage slug must be an accepted filter value.
  - *Given* a configured custom stage `"legal-review"`, *When* `list_backlog_items` is called with
    `status_filter=["legal-review"]`, *Then* it does not reject the filter as invalid (today's fixed
    `validBacklogStatuses` slice would reject it).
- `allowedSelfResolveSourceStatuses` (`:210`), `unclaimedDuplicateSourceStatuses` (`:225`), and
  `reportPRCreatedAllowedSourceStatuses` (`:237`), plus `validateSelfResolveSource`'s hardcoded switch
  (`:1391-1392`) and `request_review`/`report_blocked`'s hardcoded target statuses (`:1261-1263`,
  `:1401-1403`), are each explicitly documented — in a code comment at the declaration site and in
  this story's own text — as an intentional scope boundary: per requirements.md's resolved Open
  Question, a custom stage is a full transition/liveness graph citizen but does **not**
  automatically inherit MCP self-resolve eligibility (`report_progress`/`request_review`/
  `report_blocked`/`report_duplicate`/`report_pr_created`'s built-in-status-keyed source/target
  logic) — that requires an explicit gate-model attachment, which is out of this project's scope. No
  code change is required for these five sites; the requirement is a docstring making the boundary
  explicit at the exact place a future reader would otherwise assume it was an oversight (mirroring
  this same plan's own new note on `useBacklogService.ts` needing no change, near Epic 2.9).
  - *Given* the four maps/switch/hardcoded-target sites above, *When* reviewed, *Then* each carries a
    comment citing requirements.md's "does not automatically inherit" resolution and this story's ID.
**Files**: `server/mcp/tools_backlog.go`

##### Task 2.1.4a: Re-route `validBacklogStatuses` to source from `ConfiguredWorkflowEngine`'s stage list (falling back to the current fixed 9-entry list when no engine is wired, matching this project's fail-closed-to-built-in convention) (~5 min)
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.4b: Add the scope-boundary doc comment to `allowedSelfResolveSourceStatuses`, `unclaimedDuplicateSourceStatuses`, `reportPRCreatedAllowedSourceStatuses`, `validateSelfResolveSource`, and the `request_review`/`report_blocked` hardcoded-target-status sites (~5 min)
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.4c: Test: `list_backlog_items` accepts a configured custom stage as a status filter value; existing 9 built-in filter values are unaffected (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

---

## Epic 2.2: Stage/transition/gate ent schema + seed migration

**Goal**: Persist `StageDefinition`/`TransitionDefinition`/`GateDefinition`/`GateSatisfactionRecord`,
seeded with the existing 9-stage/edge built-in graph so `ConfiguredWorkflowEngine` has real rows to
read from day one.

### Story 2.2.1: Four new ent schemas
**As a** backend engineer, **I want** ent schemas for stages, transitions, gates, and gate-
satisfaction records, **so that** `ConfiguredWorkflowEngine` has a DB-backed graph to load.
**Acceptance Criteria**:
- `stage_transitions` enforces `UNIQUE(from_stage_id, to_stage_id)`; `gate_satisfaction_records`
  enforces `UNIQUE(item_id, gate_id)`.
  - *Given* an existing `stage_transitions` row `(idea_id, ready_id)`, *When* a duplicate `Create` is
    attempted, *Then* ent returns a constraint-violation error.
**Files**: `session/ent/schema/backlog_stage.go`, `stage_transition.go`, `transition_gate.go`,
`gate_satisfaction_record.go` (all new)

##### Task 2.2.1a: Write `backlog_stage.go` schema (~5 min)
- Files: `session/ent/schema/backlog_stage.go`

##### Task 2.2.1b: Write `stage_transition.go` schema with FK edges to `backlog_stage` (~5 min)
- Files: `session/ent/schema/stage_transition.go`

##### Task 2.2.1c: Write `transition_gate.go` schema (JSON `config` field, `stateful` bool) (~5 min)
- Files: `session/ent/schema/transition_gate.go`

##### Task 2.2.1d: Write `gate_satisfaction_record.go` schema (~5 min)
- Files: `session/ent/schema/gate_satisfaction_record.go`

##### Task 2.2.1e: Regenerate ent, confirm `go build ./...` (~3 min)
- Files: none committed beyond schema files

### Story 2.2.2: Seed migration for the built-in 9-stage graph
**As a** backend engineer, **I want** the existing 9 `BacklogStatus` values and their
`validTransitions` edges inserted as seed rows, **so that** `ConfiguredWorkflowEngine` can load a
real, complete graph immediately, matching `DefaultWorkflowEngine`'s behavior.
**Acceptance Criteria**:
- After the seed migration runs, `ConfiguredWorkflowEngine`'s loaded graph has exactly the same edges
  as `domain.ValidTransitions()`.
  - *Given* a freshly-seeded database, *When* `ConfiguredWorkflowEngine.AllowedTransitions("idea")`
    is called, *Then* it returns the identical sorted slice `DefaultWorkflowEngine.AllowedTransitions("idea")`
    returns.
**Files**: seed migration script (location per this repo's existing ent-migration convention —
confirm via `session/ent/migrate/` or an init-time seed function, whichever this repo already uses
for one-time data seeding; verify precedent before choosing)

##### Task 2.2.2a: Survey this repo's existing seed-data precedent (if any) and pick the matching mechanism (~5 min)
- Files: none (research task)

##### Task 2.2.2b: Write the seed migration inserting 9 `backlog_stage` rows + all `validTransitions` edges (~5 min)
- Files: per 2.2.2a's finding

##### Task 2.2.2c: Test asserting seeded-graph output equals `DefaultWorkflowEngine`'s output for every stage (~5 min)
- Files: `session/configured_workflow_engine_test.go` (new — shared with Epic 2.3)

---

## Epic 2.3: `ConfiguredWorkflowEngine`

**Goal**: A `WorkflowEngine` implementation backed by the Epic 2.2 tables, satisfying the interface
with zero call-site changes elsewhere, plus the new `PendingGates` method.

### Story 2.3.1: `CanTransition`/`AllowedTransitions` against the DB-loaded graph
**As a** backend engineer, **I want** `ConfiguredWorkflowEngine` to answer transition-legality
questions from a cached, DB-loaded graph, **so that** an operator-added custom transition is legal
immediately without a redeploy.
**Acceptance Criteria**:
- `ConfiguredWorkflowEngine`'s built-in-stage output is byte-for-byte identical to
  `DefaultWorkflowEngine`'s (the Risk Control regression gate).
  - *Given* the seeded database from Epic 2.2.2 with no custom stages added, *When*
    `ConfiguredWorkflowEngine.CanTransition(BacklogStatusReview, BacklogStatusPRPending)` and the
    same call against `DefaultWorkflowEngine` are both made, *Then* both return `true`, and this
    holds for every `(from,to)` pair in `domain.ValidTransitions()`.
- A newly-added custom transition (e.g. `idea -> "design-review"`) becomes legal immediately after
  the RPC creates it, with no cache-invalidation gap longer than one `Invalidate` call.
  - *Given* a fresh `CreateStageTransition` RPC call adding `idea -> "design-review"`, *When*
    `ConfiguredWorkflowEngine.CanTransition(BacklogStatusIdea, "design-review")` is called
    immediately after, *Then* it returns `true`.
**Files**: `session/configured_workflow_engine.go` (new)

##### Task 2.3.1a: Define `StageConfigRepository` interface + implementation (mirrors `WorkflowRepository`) (~5 min)
- Files: `session/stage_config_repository.go`, `session/ent_stage_config_repository.go` (new)

##### Task 2.3.1b: Implement `stageConfigCache` (copy `pipelineModeCache`'s structure again) (~5 min)
- Files: `session/stage_config_cache.go` (new)

##### Task 2.3.1c: Implement `ConfiguredWorkflowEngine.CanTransition`/`AllowedTransitions` (~5 min)
- Files: `session/configured_workflow_engine.go`

##### Task 2.3.1d: Zero-regression test: every `domain.ValidTransitions()` edge matches between the two engines (~5 min)
- Files: `session/configured_workflow_engine_test.go`

### Story 2.3.2: `PendingGates` + `ValidateGates` thin wrapper
**As a** backend engineer, **I want** `PendingGates` to return structured per-gate status, **so that**
`ValidateGates` becomes a one-line wrapper and the item-detail UI has real data to render.
**Acceptance Criteria**:
- `ValidateGates` returns `nil` exactly when `PendingGates` returns zero unsatisfied entries.
  - *Given* a transition with two gates, one satisfied and one not, *When* `ValidateGates` is called,
    *Then* it returns a non-nil error, and `PendingGates` for the same call returns a 2-element slice
    with one `Satisfied: true` and one `Satisfied: false`.
- A structural-check gate re-evaluates fresh on every `PendingGates` call (never cached as
  "previously satisfied").
  - *Given* a structural gate "all AC done" that was satisfied on the previous call, and an AC is
    since unchecked, *When* `PendingGates` is called again, *Then* the gate now reports
    `Satisfied: false` — proving no stale cached "satisfied" result was reused.
**Files**: `session/configured_workflow_engine.go`, `session/gate_status.go` (new)

##### Task 2.3.2a: Define `GateStatus`/`GateKind` types (~3 min)
- Files: `session/gate_status.go`

##### Task 2.3.2b: Implement `PendingGates` dispatching per `GateKind` to a stateless-recompute-vs-stateful-lookup path (~5 min)
- Files: `session/configured_workflow_engine.go`

##### Task 2.3.2c: Implement `ValidateGates` as `len(unsatisfied) == 0` (~3 min)
- Files: `session/configured_workflow_engine.go`

##### Task 2.3.2d: Tests: multi-gate partial satisfaction, structural-gate fresh-recompute (~5 min)
- Files: `session/configured_workflow_engine_test.go`

---

## Epic 2.4: Gate evaluation — generalize `ReviewGateRunner`, add structural + custom-check gates

**Goal**: Make each of the four `GateKind`s actually evaluable, reusing existing machinery per
Rabbit Holes' explicit instruction (extend, don't parallel-build).

### Decision: `StuckReasonGateTimeout` (resolved here, not deferred to Task 2.4.4c)

**Decision**: keep `StuckReason` a closed enum and add exactly one new generic value,
`StuckReasonGateTimeout`, covering every custom-transition/custom-gate liveness timeout — not a new
value per gate kind or per individual gate. **Rationale**: `research/architecture.md` already treats
liveness classification as a layer sibling to remediation (see this doc's own Pattern Decisions), and
a closed enum keeps `RemediationDue`'s per-reason backoff-schedule switch exhaustive — a
compiler-enforced property an open/string-typed reason would give up for no correctness benefit at
this scale (dozens of gates, one operator). This was previously left implicit inside Task 2.4.4c's
~5-minute coding task (flagged as a planning gap by the adversarial review); it is decided here
instead, before any Epic 2.4 code is written, so `reconcileCustomGateChecks` (Task 2.4.4c) and
`RemediationDue`'s backoff switch are both written against a settled value from the start, not
whatever the task's implementer happens to pick.

### Story 2.4.1: Human-approval gate (`GateSatisfactionRecord`, one-shot)
**As the** single operator, **I want** a human-approval gate satisfied by one explicit action,
**so that** a custom transition can require my sign-off the same way "Approve Plan" works today.
**Acceptance Criteria**:
- `RecordGateApproval(item, gateID)` writes a `GateSatisfactionRecord{Satisfied: true}` and a
  subsequent `PendingGates` call reflects it without re-asking.
  - *Given* a pending human-approval gate with no record, *When* `RecordGateApproval(itemID, gateID)`
    is called, *Then* the next `PendingGates` call for that transition reports that gate
    `Satisfied: true`, and it stays `true` even if unrelated item fields change afterward (one-shot,
    not re-checked).
**Files**: `session/gate_approval.go` (new), `server/services/backlog_service_gates.go` (new)

##### Task 2.4.1a: Implement `GateSatisfactionRepository` (Create/GetByItemAndGate) (~5 min)
- Files: `session/gate_satisfaction_repository.go`, `session/ent_gate_satisfaction_repository.go`

##### Task 2.4.1b: Implement `RecordGateApproval` + wire into `PendingGates`'s human-approval branch (~5 min)
- Files: `session/gate_approval.go`, `session/configured_workflow_engine.go`

##### Task 2.4.1c: `RecordGateApproval` RPC handler (~5 min)
- Files: `server/services/backlog_service_gates.go`

##### Task 2.4.1d: Test: recorded approval persists and is not re-asked (~5 min)
- Files: `session/gate_approval_test.go` (new)

### Story 2.4.2: Structural/mechanical check gate
**As a** backend engineer, **I want** a structural gate to evaluate a named precondition (AC-
complete, PR-green, no open BLOCKERs) directly against item state, **so that** it needs no session
spawn and no persisted record.
**Acceptance Criteria**:
- A structural gate config naming `"ac_complete"` evaluates `len(ParseAcCriteria(item.AcCriteria))>0
  && all criteria done` fresh on every call.
  - *Given* an item with 2 of 3 AC criteria checked, *When* `PendingGates` evaluates an
    `"ac_complete"` structural gate, *Then* it reports `Satisfied: false` with `Description: "1 of 3
    acceptance criteria incomplete"`.
**Files**: `session/gate_structural.go` (new)

##### Task 2.4.2a: Define the closed set of structural-check identifiers (`ac_complete`, `pr_green`, `no_open_blockers`) and their evaluators (~5 min)
- Files: `session/gate_structural.go`

##### Task 2.4.2b: Wire into `PendingGates`'s structural branch (~5 min)
- Files: `session/configured_workflow_engine.go`

##### Task 2.4.2c: Tests for each of the 3 structural checks, pass and fail cases (~5 min)
- Files: `session/gate_structural_test.go` (new)

### Story 2.4.3: Automated-review-verdict gate — generalize `ReviewGateRunner`
**As a** backend engineer, **I want** a custom transition's automated-review gate to reuse
`ReviewGateRunner`'s verdict machinery without inheriting its `review`→`pr_pending`-specific diff/PR
pre-checks, **so that** e.g. `idea→ready` can require a feasibility-review PASS with no diff involved.
**Acceptance Criteria**:
- `ReviewGateRunner.Run` accepts a `gateContext` parameter (which gate/transition this run is for)
  and skips the diff/worktree/branch-drift pre-checks when the gate context says no diff is expected.
  - *Given* a custom transition `idea -> ready` with an automated-review gate configured with
    `RequiresDiff: false`, *When* the gate fires, *Then* `Run` does not attempt
    `GetGitHeadSHA`/branch-drift checks and proceeds straight to the review-prompt call.
- The generalized call site (replacing `backlog_lifecycle.go`'s hardcoded `toStatus ==
  BacklogStatusReview` check) fires a review gate for **any** transition with an
  automated-review `GateDefinition` attached, not only the built-in `review` status.
  - *Given* the same custom transition with its review gate, *When* an item attempts `idea -> ready`,
    *Then* a review session is spawned exactly as it would be for `review -> pr_pending` today,
    driven by the transition's own gate config, not a literal status comparison.
**Files**: `session/review_gate.go`, `session/backlog_lifecycle.go` (the line-798-equivalent call site
in the current file — re-verify exact line number before editing)

##### Task 2.4.3a: Add `gateContext` parameter (`GateID`, `TargetTransition`, `RequiresDiff`) to `ReviewGateRunner.Run` (~5 min)
- Files: `session/review_gate.go`

##### Task 2.4.3b1: Extract `Run`'s existing worktree-identity/branch-drift/diff-computation block (`session/review_gate.go:136`-`~260`: fetch worktree → identity-mismatch check → branch-drift sync → dirty check → diff compute) into a new private helper `runDiffPreChecks(ctx, item, is) (diff string, truncated bool, uncommittedWarning string, blocked bool)`, preserving every existing early-return/terminal-verdict branch verbatim — pure Extract Method, no new conditional logic (~5 min)
- Files: `session/review_gate.go`

##### Task 2.4.3b2: Call `runDiffPreChecks` from `Run` only when `gateContext.RequiresDiff` is true; when false, skip it entirely and leave `diff`/`truncated`/`uncommittedWarning` at their zero values with no worktree/git calls made (~5 min)
- Files: `session/review_gate.go`

##### Task 2.4.3b3: Adjust `reviewPromptFor`'s diff-section rendering so a `RequiresDiff: false` run produces an explicit "no diff expected for this gate" prompt section instead of an empty-diff rendering that reads as a bug (~5 min)
- Files: `session/review_gate.go`

##### Task 2.4.3c: Replace the hardcoded `toStatus == BacklogStatusReview` spawn condition with "does this transition have an automated-review gate attached" (~5 min)
- Files: `session/backlog_lifecycle.go`

##### Task 2.4.3d: Regression test: existing `review -> pr_pending` review-gate behavior is unchanged (~5 min)
- Files: `session/backlog_lifecycle_review_test.go`

##### Task 2.4.3e: New test: a no-diff custom transition's review gate fires and records a verdict without the diff pre-checks (~5 min)
- Files: `session/review_gate_test.go`

### Story 2.4.4: Custom/pluggable check gate via `InvokeCustomGateCheck`
**As a** backend engineer, **I want** a custom-check gate to invoke a named, pre-registered skill/
slash-command bounded by a `LivenessDefinition`, **so that** it can never run arbitrary code or hang
without being caught by the existing liveness sweep. Its config arrives already parsed and validated
by Task 2.7.2g's `ParseGateConfig` at save time — `InvokeCustomGateCheck` consumes a typed
`CustomCheckConfig`, never raw JSON.
**Acceptance Criteria**:
- `InvokeCustomGateCheck` rejects a gate config naming a skill/slash-command not in the pre-registered
  allowlist.
  - *Given* a `GateDefinition{Kind: GateKindCustom, Config: {"skill": "not-a-real-skill"}}`, *When*
    the gate fires, *Then* it returns an error and the transition is blocked (fail-closed for gates,
    per Pattern Decisions), never silently passing.
- `InvokeCustomGateCheck` blocks the transition and logs Warn when the skill/slash-command invocation
  errors synchronously (crash, missing runtime dependency, malformed environment) rather than timing
  out — a distinct, immediate failure path from the sweep-driven timeout path below.
  - *Given* a `GateKindCustom` invocation whose spawn returns a synchronous error (e.g. the process
    exits non-zero immediately, or a required runtime dependency is missing) rather than hanging,
    *When* `InvokeCustomGateCheck` observes the error, *Then* it blocks the transition, logs one Warn
    line naming the spawn failure, and does **not** wait for or rely on the liveness sweep to detect
    it (per the plan's fail-closed-for-gates rule).
- A custom check bounded by `LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration:
  10m, StalenessMargin: 5m}` that exceeds 15m is picked up by the **same** stuck-detection sweep
  used for `orphaned_triage`, not a new detector.
  - *Given* a custom-check invocation still open after 16m, *When* the periodic `reconcile*` sweep
    runs, *Then* it marks the item stuck using `LivenessEngine.LivenessFor` resolution for that gate's
    liveness config, going through the identical code path Epic 1.4 built.
**Files**: `session/gate_custom_check.go` (new), `session/backlog_lifecycle_gates.go` (new)

##### Task 2.4.4a: Define the pre-registered skill/slash-command allowlist mechanism (~5 min)
- Files: `session/gate_custom_check.go`

##### Task 2.4.4b1: Implement the spawn call against Task 2.4.4a's allowlist, returning a blocking error immediately (not a bare timeout) when the spawn itself fails synchronously (non-zero exit, missing runtime dependency, malformed environment) — the synchronous-failure path, logged as Warn (~5 min)
- Files: `session/gate_custom_check.go`

##### Task 2.4.4b2: Bind a successful spawn to the gate's `LivenessDefinition` (Shape A) — thread `ExpectedDuration`/`StalenessMargin` through the invocation the same way `TriggerTriage`'s call budget does (Epic 1.4's pattern), without yet implementing sweep-side stuck detection (that's Task 2.4.4c) (~5 min)
- Files: `session/gate_custom_check.go`

##### Task 2.4.4b3: On successful invocation completion, parse and record the `ReviewOutcome`-shaped verdict against the gate's `GateSatisfactionRecord`, mirroring `recordTerminalReviewVerdict`'s shape (~5 min)
- Files: `session/gate_custom_check.go`

##### Task 2.4.4b4: Assemble `InvokeCustomGateCheck` from Tasks 2.4.4b1-b3's pieces in the correct order (allowlist-checked spawn → liveness-bound tracking → verdict recording) and confirm `go build ./...` passes (~5 min)
- Files: `session/gate_custom_check.go`

##### Task 2.4.4c: Wire the sweep's stuck-marking for an overdue custom check through `LivenessEngine`, using the `StuckReasonGateTimeout` value decided in this epic's Decision subsection above (~5 min)
- Files: `session/backlog_lifecycle_gates.go` (new — houses the new `reconcileCustomGateChecks`
  function; per architecture-review/adversarial-review's file-growth concern, deliberately not added
  to the already-large `session/backlog_lifecycle.go`)

##### Task 2.4.4d: Tests: allowlist rejection, liveness-bounded timeout detection reusing the sweep (~5 min)
- Files: `session/gate_custom_check_test.go` (new)

##### Task 2.4.4e: Test: a synchronous (non-timeout) spawn failure blocks the transition and logs Warn, distinct from the timeout-detection path covered by Task 2.4.4d (~5 min)
- Files: `session/gate_custom_check_test.go`

---

## Epic 2.5: `StageConfigSnapshot` — audit-trail immunity to later config edits

**Goal**: An item's history renders correctly even after the stage/transition/gate it references is
edited or deleted, mirroring `AcSnapshot`'s discipline.

### Story 2.5.1: Snapshot stage config at entry
**As a** backend engineer, **I want** the item's `BacklogStatusEvent` row to store a human-readable
stage name at the time of transition, **so that** deleting a custom stage later doesn't blank out
history.
**Acceptance Criteria**:
- A `BacklogStatusEvent` row created for a transition into a since-deleted custom stage still renders
  the stage's original name in the item-detail history.
  - *Given* an item transitioned into custom stage "design-review" (captured as
    `StageConfigSnapshot{Name: "Design Review"}` on the event row), and the stage is later deleted,
    *When* the item-detail history renders that event, *Then* it shows "Design Review", not a blank
    or crashed lookup.
**Files**: `session/backlog_status_event.go` (extend), `session/ent/schema/backlog_status_event.go`
(extend — confirm exact schema file name before editing)

##### Task 2.5.1a: Add a `stage_name_snapshot` column to the status-event schema (~5 min)
- Files: `session/ent/schema/<status event schema file>.go`

##### Task 2.5.1b: Populate it at transition-write time from the resolved `StageDefinition.Name` (~5 min)
- Files: `session/backlog_lifecycle.go` or `EntRepository.TransitionBacklogItemStatus` (confirm exact
  call site)

##### Task 2.5.1c: Test: history renders correctly after the referenced stage is deleted (~5 min)
- Files: relevant `*_test.go` for the transition-write path

### Story 2.5.2: Frozen-snapshot behavior for an in-flight item on a deleted stage
**As a** backend engineer, **I want** an item currently sitting in a since-deleted stage to keep
functioning under a frozen snapshot of that stage's definition, **so that** `AllowedTransitions`/
`CanTransition` have a defined answer instead of erroring on an unknown `from`.
**Acceptance Criteria**:
- `ConfiguredWorkflowEngine.AllowedTransitions("design-review")` for a since-deleted stage returns the
  transitions that were legal at the time the item entered it (from the item's own snapshot), not an
  empty slice or a panic.
  - *Given* an item with a `StageConfigSnapshot` captured on entry to "design-review", and the stage
    row is since deleted, *When* `AllowedTransitions` is queried for that item's current stage,
    *Then* it returns the snapshotted transitions, with a Warn log noting the stage config is no
    longer live.
**Files**: `session/configured_workflow_engine.go`

##### Task 2.5.2a: Extend `AllowedTransitions`/`CanTransition` to accept an optional per-item snapshot fallback (~5 min)
- Files: `session/configured_workflow_engine.go`

##### Task 2.5.2b: Test: deleted-stage in-flight item still resolves transitions correctly (~5 min)
- Files: `session/configured_workflow_engine_test.go`

---

## Epic 2.6: Graph validation (cycle/reachability) at definition time

**Goal**: Reject (or loudly warn on) a saved stage/transition graph that has an unreachable stage, a
trap with no outgoing edge, or a cycle with no escaping gate — per pitfalls.md §6 and
build-vs-buy.md §3's bespoke-with-adversarial-tests verdict.

### Story 2.6.1: Bespoke DFS reachability + trap validator
**As the** single operator, **I want** the stage/transition save RPC to reject a graph with an
unreachable stage or a non-terminal dead end, **so that** I can't accidentally create a stage nothing
can ever enter or leave.
**Acceptance Criteria**:
- Saving a new stage with zero outgoing transitions (and not marked `IsTerminal`) is rejected.
  - *Given* a `CreateStage` request for a non-terminal stage with no transitions defined yet,
    *When* the graph-wide validator runs (triggered by the subsequent `CreateStageTransition` save,
    or an explicit "validate graph" step — confirm exact trigger point in Task 2.6.1a), *Then* it
    returns a validation error naming the stage as an unreachable/dead-end candidate before the save
    commits.
- Saving a stage with no incoming transition from any reachable-from-an-entry-stage node is rejected.
  - *Given* a graph where every existing entry stage's transitively-reachable set excludes the new
    stage, *When* validation runs, *Then* it returns an error naming the stage unreachable.
- **Disabling an existing transition edge is rejected when doing so leaves any live item's current
  stage with zero enabled outgoing transitions** — not only checked at stage/transition *creation*
  time as originally written. This closes pre-mortem Failure #2: the static graph shape can stay
  valid while a *dynamic*, item-distribution-dependent edge removal strands live work.
  - *Given* a custom stage `"design-review"` whose only enabled outgoing transition is
    `design-review -> ready`, and one live item currently at status `"design-review"`, *When*
    `UpdateStageTransition` is called to set `design-review -> ready`'s `enabled` to `false`, *Then*
    the graph validator rejects the call with an error naming both the stage and the affected live-item
    count (e.g. `"Disabling 'design-review -> ready' would leave 1 live item on 'design-review' with
    no legal outgoing transition"`), and the edge remains enabled in the persisted graph.
  - *Given* the same `"design-review"` stage additionally has a second enabled outgoing transition,
    `design-review -> legal-review`, *When* `design-review -> ready` is disabled, *Then* the validator
    allows it, since the stage still has ≥1 enabled outgoing transition available to the live item.
**Files**: `session/graph_validator.go` (new)

##### Task 2.6.1a: Decide and document the validation trigger point (on every mutating RPC vs. an explicit "validate" RPC) (~5 min)
- Files: `session/graph_validator.go` (doc comment)

##### Task 2.6.1b1: Define the adjacency-building helper `buildAdjacency(stages []StageDefinition, transitions []TransitionDefinition) map[string][]string` — plain data-shaping, no traversal logic yet (~5 min)
- Files: `session/graph_validator.go`

##### Task 2.6.1b2: Implement the DFS traversal `reachableFrom(entryStages []string, adjacency map[string][]string) map[string]bool`, marking every stage reachable from any `IsEntry` stage (~5 min)
- Files: `session/graph_validator.go`

##### Task 2.6.1b3: Wire the reachability result into the validator's "unreachable stage" rejection path, covering both this story's ACs (zero-outgoing non-terminal stage, and zero-incoming-from-an-entry-stage) (~5 min)
- Files: `session/graph_validator.go`

##### Task 2.6.1c: Implement the "non-terminal stage has ≥1 outgoing transition" check (~3 min)
- Files: `session/graph_validator.go`

##### Task 2.6.1d: Adversarial tests: self-loop, multi-node cycle, disconnected component, valid graph passes (~5 min)
- Files: `session/graph_validator_test.go` (new)

##### Task 2.6.1g1: Build the two-concurrent-caller fixture — a pre-mutation graph state plus two candidate `CreateStageTransition` requests that are each individually valid but would jointly produce an invalid graph, synchronized via a barrier (e.g. a `WaitGroup`/channel) so both calls' validation reads happen before either call's write commits (~5 min)
- Files: `server/services/backlog_service_transitions_test.go`

##### Task 2.6.1g2: Write the assertion harness confirming the persisted graph never reaches the jointly-invalid state — exactly one call's transition is committed, the other serializes and re-validates against the post-commit graph under Task 2.7.2h's transaction wrapper (~10 min)
- Closes architecture-review Concern 4: a TOCTOU race between graph validation and the persist write
  that would otherwise defeat this epic's whole guarantee.
- Files: `server/services/backlog_service_transitions_test.go`

##### Task 2.6.1e: Implement the live-item-aware disable check — given a candidate `(fromStage, toStage, enabled=false)` update and a live-item-count-per-stage query, reject if `fromStage`'s remaining-enabled-outgoing-edge count would drop to zero while its live-item count is >0 (~5 min)
- This is a distinct check from Task 2.6.1c's static "non-terminal stage has ≥1 outgoing transition"
  rule: a stage can validly have zero outgoing edges at *creation* time (nothing depends on it yet)
  but must not be allowed to reach zero once items are actually sitting on it.
- Files: `session/graph_validator.go`

##### Task 2.6.1f: Tests: disabling the last enabled outgoing edge for a stage with ≥1 live item is rejected; disabling one of several enabled edges remains allowed; disabling any edge for a stage with zero live items is always allowed (~5 min)
- Files: `session/graph_validator_test.go`

### Story 2.6.2: Cycle-with-no-escape lint warning
**As the** single operator, **I want** a warning (not necessarily a hard block) when a cycle has no
gate anywhere in it that can ever be satisfied, **so that** I don't create a configuration an item can
loop in forever.
**Acceptance Criteria**:
- A detected cycle where every edge has zero gates attached produces a validation warning surfaced in
  the save RPC's response, not a hard rejection (cycles are legitimate — e.g. `review` loops back to
  `in_progress`).
  - *Given* a 3-stage cycle with no gates on any edge, *When* the graph is saved, *Then* the RPC
    response includes a `warnings` field naming the cycle, and the save still succeeds.
**Files**: `session/graph_validator.go`

##### Task 2.6.2a: Implement cycle detection (reuse the DFS traversal from Task 2.6.1b2) + "any edge in cycle has a gate" check (~5 min)
- Files: `session/graph_validator.go`

##### Task 2.6.2b: Add a `warnings []string` field to the save RPC response proto (~3 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 2.6.2c: Test: gate-free cycle produces a warning, gated cycle does not (~5 min)
- Files: `session/graph_validator_test.go`

---

## Epic 2.7: Proto/RPC CRUD surface for stages/transitions/gates

**Goal**: A 5-RPC-quintet-per-entity CRUD surface mirroring `PipelineMode`'s, for `StageDefinition`,
`TransitionDefinition`, and `GateDefinition`.

### Story 2.7.1: Stage CRUD RPCs
**As the** single operator, **I want** `CreateStage`/`UpdateStage`/`DeleteStage`/`GetStage`/
`ListStages` RPCs, **so that** the settings UI (Epic 2.8) has something to call.
**Acceptance Criteria**:
- `DeleteStage` on a stage with ≥1 live item is rejected (or requires an explicit force flag),
  matching the UI's "disable, don't delete" affordance from `research/ux.md` §4.
  - *Given* a stage with 3 items currently on it, *When* `DeleteStage` is called without a force
    flag, *Then* it returns `connect.CodeFailedPrecondition` naming the item count.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stages.go` (new)

##### Task 2.7.1a: Add `BacklogStage` message + 5 RPCs to backlog.proto (~5 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 2.7.1b: `make proto-gen`, confirm build (~3 min)
- Files: none (generated)

##### Task 2.7.1c: Implement handlers, including the live-item-count delete guard (~5 min)
- Files: `server/services/backlog_service_stages.go`

##### Task 2.7.1d: Handler tests: delete guard, cache invalidation on mutation (~5 min)
- Files: `server/services/backlog_service_stages_test.go` (new)

### Story 2.7.2: Transition + gate CRUD RPCs
**As the** single operator, **I want** transition and gate CRUD nested under a stage's edit flow,
**so that** the UI's "Outgoing transitions" sub-list (Epic 2.8) has a backing RPC.
**Acceptance Criteria**:
- `CreateStageTransition` invokes the Epic 2.6 graph validator before committing.
  - *Given* a `CreateStageTransition` request that would create an unreachable stage, *When* called,
    *Then* it returns a validation error and the transition is not persisted.
- `UpdateStageTransition(enabled=false)` invokes Task 2.6.1e's live-item-aware disable check, not only
  `CreateStageTransition`'s static reachability check.
  - *Given* the `"design-review"`-with-one-enabled-edge scenario from Story 2.6.1's new AC, with 1 live
    item currently on `"design-review"`, *When* `UpdateStageTransition` is called to disable
    `design-review -> ready`, *Then* it returns `connect.CodeFailedPrecondition` naming the stage and
    the live-item count, and a follow-up `ListStages`/transition-list call confirms the edge is still
    enabled (no partial write).
- Graph validation and the transition-persist write happen inside one DB transaction, so a concurrent
  edit can't defeat the validator's guarantee (architecture-review Concern 4).
  - *Given* two concurrent `CreateStageTransition` calls that would each individually pass validation
    but jointly produce an invalid graph, *When* both attempt to commit, *Then* they serialize under
    Task 2.7.2h's transaction wrapper — the second call's validation re-runs against the first call's
    already-committed state — and the graph never reaches a state that violates Epic 2.6's guarantee.
- A `GateKindCustom` config payload naming any key other than the allowlisted skill identifier, or
  naming a skill not in Story 2.4.4's pre-registered allowlist, is rejected by the gate-save RPC
  itself, not merely at invocation time (architecture-review Concern 2 / adversarial-review Concern 1).
  - *Given* a gate-attach request with `TransitionGate{Kind: GateKindCustom, Config: {"skill":
    "review-feasibility", "extra_flag": "..."}}`, *When* the RPC is called, *Then* it returns
    `connect.CodeInvalidArgument` naming `extra_flag` as an unrecognized key, and no row is persisted.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_transitions.go` (new),
`session/gate_config.go` (new)

##### Task 2.7.2a: Add `StageTransition`/`TransitionGate` messages + RPCs (~5 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 2.7.2b: `make proto-gen`, confirm build (~3 min)
- Files: none (generated)

##### Task 2.7.2c: Implement handlers, invoking `graph_validator.go` on every mutating call (~5 min)
- Files: `server/services/backlog_service_transitions.go`

##### Task 2.7.2d: Handler tests (~5 min)
- Files: `server/services/backlog_service_transitions_test.go` (new)

##### Task 2.7.2e: Wire `UpdateStageTransition`'s `enabled=false` path to query live-item-count-per-stage (via the storage layer) and invoke Task 2.6.1e's check before persisting (~5 min)
- Files: `server/services/backlog_service_transitions.go`

##### Task 2.7.2f: Handler test: disabling the last enabled edge for a stage with live items returns `CodeFailedPrecondition` and persists nothing (~5 min)
- Files: `server/services/backlog_service_transitions_test.go`

##### Task 2.7.2g1: Define the `GateConfig` sum type — `HumanApprovalConfig`, `AutomatedReviewConfig{PipelineMode, RequiresDiff}`, `StructuralConfig{CheckID}`, `CustomCheckConfig{SkillID}` — as the four kind-specific structs plus a `GateConfig` marker interface (~5 min)
- Files: `session/gate_config.go` (new)

##### Task 2.7.2g2: Implement `ParseGateConfig(kind GateKind, raw json.RawMessage) (GateConfig, error)`, rejecting any JSON key outside the kind's allowlisted field set and validating `CustomCheckConfig.SkillID` against Story 2.4.4's pre-registered allowlist (~5 min)
- Files: `session/gate_config.go`

##### Task 2.7.2g3: Call `ParseGateConfig` in the gate-save RPC handler before persisting, returning `connect.CodeInvalidArgument` naming the offending key/skill on failure — not only at `InvokeCustomGateCheck`'s invocation time (~5 min)
- Closes architecture-review Concern 2 and adversarial-review's Concern 1: config validation moves
  from invocation-time-only to save-time, structurally, per both reviews' own stated recommendation.
  `PendingGates`'s dispatch and `InvokeCustomGateCheck` (Story 2.4.4) consume this already-typed value,
  never raw JSON.
- Files: `server/services/backlog_service_transitions.go`

##### Task 2.7.2g4: Tests: unknown-key rejection per gate kind, unregistered-skill rejection, valid configs for all four kinds round-trip correctly (~5 min)
- Files: `session/gate_config_test.go` (new)

##### Task 2.7.2h1: Wrap Task 2.7.2c's `CreateStageTransition` validate-then-persist sequence in a single ent transaction (`WithTx`), re-running the Epic 2.6 validator against the transaction's own read view rather than a pre-transaction snapshot (~5 min)
- Files: `server/services/backlog_service_transitions.go`

##### Task 2.7.2h2: Apply the identical `WithTx` wrapping to Task 2.7.2e's `UpdateStageTransition` validate-then-persist sequence, including the live-item-aware disable check (~5 min)
- Files: `server/services/backlog_service_transitions.go`

##### Task 2.7.2h3: Confirm a failed re-validation inside either transaction rolls back with no partial write; run the existing handler tests plus Task 2.6.1g's concurrency test against the wrapped handlers (~5 min)
- Closes architecture-review Concern 4's TOCTOU race between Epic 2.6's validator and the persist
  write. Task 2.6.1g's tests exercise this.
- Files: `server/services/backlog_service_transitions.go`, `server/services/backlog_service_transitions_test.go`

---

## Epic 2.8: "Backlog Stages" settings UI

**Goal**: A list-based CRUD UI mirroring `PipelineModeForm.tsx`, per the naming-collision finding
("Stages"/"Pipeline Stages," never "Workflow(s)") and the list-not-canvas UX recommendation.

**Scope decision**: this UI covers stages, transitions, and gates only — it does **not** include a
liveness-parameter editor, per requirements.md's Scope section (updated to make this explicit) and
`design/ux.md`'s "Scope note on liveness UI." Liveness values stay RPC-only (Epic 1.3.2), consistent
with Phase 1 having zero UI of its own; a liveness editor is deferred, not an oversight.

### Story 2.8.1: Stages list page
**As the** single operator, **I want** a `/settings/backlog-stages` page listing all stages with an
enabled toggle, **so that** I have the same CRUD-list experience as `/settings/pipeline-modes`.
**Acceptance Criteria**:
- The page title and nav label read "Backlog Stages" (or "Pipeline Stages"), never "Workflow(s)".
  - *Given* the rendered page, *When* inspected, *Then* no visible text reads "Workflow" as the
    stage-management concept's name (grep the rendered DOM/test snapshot for the string).
- Disabling a stage (toggle) does not delete it or its transitions.
  - *Given* an enabled stage with 2 transitions, *When* the enabled toggle is switched off, *Then*
    the stage row shows disabled state and its transitions are unaffected/still queryable.
**Files**: `web-app/src/app/settings/backlog-stages/page.tsx` (new)

##### Task 2.8.1a: Scaffold the page mirroring `pipeline-modes/page.tsx`'s list structure (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/page.tsx`

##### Task 2.8.1b: Wire `listStages`/`updateStage` (enabled toggle) to the RPCs from Epic 2.7 (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/page.tsx`, `web-app/src/lib/hooks/useBacklogStages.ts` (new)

##### Task 2.8.1c: Test: page renders no "Workflow" label; toggle calls `UpdateStage` with only `enabled` set (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/page.test.tsx` (new)

### Story 2.8.2: Stage form with nested transitions + gate checklist
**As the** single operator, **I want** a stage's edit form to include an "Outgoing transitions"
sub-list, each row with a target-stage `<select>` and a gate checkbox-group, **so that** I never need
a separate canvas to define the graph.
**Acceptance Criteria**:
- Checking the "Automated review" gate checkbox on a transition row reveals a review-prompt/pipeline-
  mode `<select>` inline, progressive-disclosure style.
  - *Given* an unchecked "Automated review" checkbox, *When* the operator checks it, *Then* a
    `<select>` for pipeline mode appears in the same row, and unchecking it hides and clears that
    field.
- The read-only graph visualization renders an `sr-only` text-equivalent table alongside the SVG/
  mermaid diagram.
  - *Given* the stage list has ≥2 stages with transitions, *When* the visualization section renders,
    *Then* an `sr-only` (in the accessibility tree, not `display:none`) table lists every "From → To
    (gates: N)" row matching the diagram's edges.
**Files**: `web-app/src/app/settings/backlog-stages/StageForm.tsx` (new),
`web-app/src/components/backlog/StageGraphDiagram.tsx` (new)

##### Task 2.8.2a: Scaffold `StageForm.tsx` mirroring `PipelineModeForm.tsx`'s overlay/slug-immutable/two-step-delete pattern (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2b: Build the "Outgoing transitions" sub-list (add/remove rows, target-stage `<select>`) (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2c1: Build the base checkbox-group markup — one checkbox per `GateKind` (human approval, automated review, structural, custom) attached to a transition row, with no conditional fields yet (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2c2: Add the automated-review kind's progressive-disclosure fields (pipeline-mode `<select>`, `RequiresDiff` toggle), appearing only when its checkbox is checked and clearing/hiding on uncheck (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2c3: Add the structural and custom-check kinds' progressive-disclosure fields (structural check-ID `<select>` from Task 2.4.2a's closed set; custom-check skill-ID `<select>` from Story 2.4.4's allowlist) (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2c4: Wire keyboard operability for the progressive-disclosure interaction (tab order, checkbox/field association via `<label>`/`aria-describedby`), feeding Task 2.8.2e's accessibility test (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`

##### Task 2.8.2d: Build `StageGraphDiagram.tsx` (SVG or mermaid) + `sr-only` text-equivalent table (~5 min)
- Files: `web-app/src/components/backlog/StageGraphDiagram.tsx`

##### Task 2.8.2e: Accessibility test: gate checkbox progressive disclosure is keyboard-operable; `sr-only` table present in a11y tree (~5 min)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.test.tsx` (new)

---

## Epic 2.9: Dynamic stage rendering on `BacklogBoard`/`StageTracker` + unresolved-stage fallback

**Goal**: `BacklogBoard.tsx`'s `COLUMNS` and `StageTracker.tsx`'s `deriveStageDisplay` render whatever
stage set is actually configured, including an "Unrecognized stage" fallback for a stage ID present
on a live item but absent from the fetched config (never silently drop the item, per BUG-037).

**`useBacklogService.ts` needs no changes**: requirements.md names this hook's "frontend transition
table" as needing updates, but source inspection (`web-app/src/lib/hooks/useBacklogService.ts:195-201`)
confirms it already exposes a server-authoritative `allowedTransitions?: string[]` field per item
rather than a hardcoded table — verified independently in this project's adversarial review
(Minor #1) and again during plan repair (2026-09-03): no hardcoded transition table exists anywhere
in the file. It's absent from the epics below because it's already correct, not because it was
missed.

### Story 2.9.1: `BacklogBoard.tsx` fetches live stage config
**As the** single operator, **I want** the board's columns to reflect the actual configured stage set,
**so that** a custom stage appears as its own column instead of being invisible.
**Acceptance Criteria**:
- A custom stage with 1 live item renders as its own board column.
  - *Given* a configured custom stage "design-review" with 1 item currently on it, *When*
    `BacklogBoard` renders, *Then* a "Design Review" column appears with that 1 item's card.
- An item on a stage ID absent from the fetched config renders in an "Unrecognized stage" overflow
  column, never silently dropped.
  - *Given* an item with `status="some-deleted-stage"` and no matching entry in the fetched stage
    list, *When* the board renders, *Then* the item appears in an "Unrecognized stage" column, not
    nowhere.
**Files**: `web-app/src/components/backlog/BacklogBoard.tsx`,
`web-app/src/components/backlog/detail/StageTracker.tsx`

##### Task 2.9.1a: Replace the hardcoded `COLUMNS` array with a `useBacklogStages()`-fetched, cached list (~5 min)
- Files: `web-app/src/components/backlog/BacklogBoard.tsx`

##### Task 2.9.1b: Add the "Unrecognized stage" fallback column for any item whose status matches no fetched stage (~5 min)
- Files: `web-app/src/components/backlog/BacklogBoard.tsx`

##### Task 2.9.1c: Update `StageTracker.tsx`'s `deriveStageDisplay` to consult the fetched stage list instead of a hardcoded switch, keeping its existing `default:` fallback shape (~5 min)
- Files: `web-app/src/components/backlog/detail/StageTracker.tsx`

##### Task 2.9.1d: Tests: custom stage renders as its own column; unrecognized stage item never disappears (regression test naming BUG-037 explicitly) (~5 min)
- Files: `web-app/src/components/backlog/BacklogBoard.test.tsx`

---

## Epic 2.10: Item-detail "what's blocking this" gate-checklist UI

**Goal**: Generalize `GateVerdictBox.tsx`'s single-verdict card into a multi-gate checklist, each row
showing status and who/what can satisfy it — the concrete answer to the Success Metric.

### Story 2.10.1: Multi-gate checklist component
**As the** single operator, **I want** to see every pending gate for an item's next transition, with
who/what can satisfy each, **so that** I never have to guess why an item hasn't moved.
**Acceptance Criteria**:
- A transition blocked by both a pending human-approval gate and an unmet structural gate shows two
  independent rows, each with its own status and an "Approve" affordance only on the human-approval
  row.
  - *Given* `PendingGates` returning `[{Kind: human_approval, Satisfied: false}, {Kind: structural,
    Satisfied: false, Description: "2 of 5 AC incomplete"}]`, *When* the item-detail page renders,
    *Then* two `role="status"` rows appear, one with an "Approve"/"Reject" button pair and one
    read-only with the AC-incomplete description.
- A gate referencing a since-deleted config (e.g. a deleted pipeline mode) renders a "Configuration
  error" row, never a silent pass or a blank/crashed section.
  - *Given* a gate whose `automated_review` config points at a deleted pipeline mode, *When*
    `PendingGates` returns an unresolved-config error for that gate, *Then* the UI row reads
    "Configuration error — this gate can't be evaluated (referenced pipeline mode not found)".
**Files**: `web-app/src/components/backlog/GateChecklist.tsx` (new, generalizes
`GateVerdictBox.tsx`)

##### Task 2.10.1a: Extract `GateVerdictBox.tsx`'s `VERDICT_CONFIG` map pattern into a per-`GateKind` config map (~5 min)
- Files: `web-app/src/components/backlog/GateChecklist.tsx`

##### Task 2.10.1b: Build the multi-row checklist, each row `role="status" aria-live="polite"` (~5 min)
- Files: `web-app/src/components/backlog/GateChecklist.tsx`

##### Task 2.10.1c: Add the human-approval Approve/Reject affordance calling `RecordGateApproval` (~5 min)
- Files: `web-app/src/components/backlog/GateChecklist.tsx`

##### Task 2.10.1d: Add the "Configuration error" row rendering for an unresolved gate (~3 min)
- Files: `web-app/src/components/backlog/GateChecklist.tsx`

##### Task 2.10.1e: Tests: multi-gate row rendering, Approve action, config-error rendering (~5 min)
- Files: `web-app/src/components/backlog/GateChecklist.test.tsx` (new)

---

## Epic 2.11: ADR-013 resolution

**Goal**: Close the loop on ADR-013's Phase 2 proposal per requirements.md's Scope section — mark it
Accepted/Implemented, with explicit notes on where this project's design diverges (the new
`LivenessEngine` sibling interface and `PendingGates` method were not part of ADR-013's original
proposal at all).

### Story 2.11.1: Update ADR-013 and cross-link the new ADRs
**As a** future maintainer, **I want** ADR-013's status updated and cross-referenced to the ADRs this
project adds, **so that** the design history is traceable.
**Acceptance Criteria**:
- `docs/adr/013-workflow-engine-replaces-valid-transitions.md`'s `Status` field reads `Accepted` (or
  `Superseded`, if Phase 3 planning judges the divergence significant enough) with a note pointing at
  `ADR-001`/`ADR-002`/`ADR-003` in this project's `decisions/` directory.
  - *Given* ADR-013's current `Status: Proposed`, *When* this story merges, *Then* the file's status
    line reads `Accepted` and a new "Phase 2 Implementation Note" section links the three new ADRs.
**Files**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md`

##### Task 2.11.1a: Edit ADR-013's status field and add the implementation-note section (~5 min)
- Files: `docs/adr/013-workflow-engine-replaces-valid-transitions.md`

##### Task 2.11.1b: Cross-link from each of the three new ADRs back to ADR-013 (~3 min)
- Files: the three ADR files in `project_plans/backlog-custom-workflow-stages/decisions/`
