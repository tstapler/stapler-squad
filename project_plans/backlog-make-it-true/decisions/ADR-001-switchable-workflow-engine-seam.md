# ADR-001: Isolate the workflow-engine swap behind a SwitchableWorkflowEngine (guards stay code)

**Status**: Accepted (planning; spike results pending). Amended by [ADR-007](ADR-007-engine-gated-transition-choke-point.md): modes are per caller class (operator, agent, system), and the handler IS edited, minimally, in Story 1.5.5a. Owner pre-decided "Isolate via seam"; this ADR records the shape and the sub-decisions research forced.
**Date**: 2026-10-17
**Project**: backlog-make-it-true, workstream A

## Context

`BacklogService` is constructed with `session.NewDefaultWorkflowEngine()` (`server/dependencies.go:678`, passed at `:1477`). The `ConfiguredWorkflowEngine` (`:773`) is used there only as a cache invalidator (`backlog_service.go:417-461`); the lifecycle listener also holds it (`dependencies.go:1544`) for automated-review gate context. Research proved a one-line swap would be a behavior regression (VERIFIED, `research/architecture.md` Findings 1, 2, 5; `research/pitfalls.md` Top 5):

1. The seed writes zero `TransitionGate` rows (`session/workflow_stage_seed.go:72-135`), so `AC required`, `plan/blockers/artifacts`, `verdict-clear`, `PASS verdict + shipped` (all in `TransitionGuard`, `session/domain/backlog.go:599-700`) would vanish.
2. `override_reason` short-circuits live inside `TransitionGuard`; the configured engine never reads it.
3. A failed cache load leaves an empty cache, so `CanTransition` is false for everything (`configured_workflow_engine.go:78-85`). There is no fallback to the default graph.
4. Three engines coexist: `BacklogService` (default), the lifecycle listener (configured), and `SyncLoop` (`session/backlog_sync.go:57,69`, a private `NewDefaultWorkflowEngine()`).

## Decision

Add `session.SwitchableWorkflowEngine` implementing `WorkflowEngine`, constructed in `server/dependencies.go` after `stageConfigEngine` exists, and passed at `:1477`. Through Stories 1.1.x the `TransitionBacklogItemStatus` handler is not edited; ADR-007 later routes it (with every other status writer) through `Storage.GatedTransition`, which consumes this selector.

Modes, read live (TTL-cached about 1s) from feature flags `backlog:workflow-engine-shadow` (default on, read-only) and the enforce flag of the caller class (`backlog:workflow-engine-enforce` for class `operator`, `-enforce-agent`, `-enforce-system`; all default off, ADR-007 Decision 8); mode for a class = enforce if its enforce flag, else shadow if shadow flag, else off. Stories 1.1.x implement the operator/engine-routed class; Story 1.5.2 adds the class dimension. The table below is per class:

| Mode | Behavior |
|---|---|
| off | Default engine only. |
| shadow | Default engine answers and is returned. `enforceAnswer` (below) is computed on identical input; divergence of Default versus `enforceAnswer` is counted by class. Never alters the return value. Stateful gate kinds are not evaluated in shadow (counted `not_compared{reason=stateful_gate}`), so shadow performs no `GateSatisfactionRepository` I/O. |
| enforce | Returns `enforceAnswer`. |

`enforceAnswer` is implemented once, privately, and shared by `enforce` and `shadow`: graph = Configured graph UNION the system-required edge set (ADR-002, as amended by ADR-007 D4: fixed-edge `agent`/`system` callers and the archive edges only; the generic MCP delegate is exempt); guards = Default `TransitionGuard` AND Configured operator gates; a non-empty `OverrideReason` skips both guard halves (never the graph). If Configured is nil or its cache is empty/unloaded: guards fall back to Default and the graph falls back to Default UNION `SystemRequiredEdges()` (never plain Default, which lacks the `in_progress|review|pr_pending->archived` and `archived->archived` edges and would make Enforce deny archive on the fail-open path; Repair round 5, ADV-N1), count `fallback_total{reason}`, WARN once per minute. `SystemRequiredEdges()` does not exist in the tree today; Task 1.1.2b creates it.

Sub-decisions:

- **Guards are invariants, not config (architecture option A).** `TransitionGuard` stays code and always runs on built-in `(from,to)` pairs in every mode. Operators add/remove stages, edges and extra gates; they cannot delete "plan approved before work". Rejected option B (seed ~5 new structural check IDs plus a per-edge versioned migration, because the seed no-ops once rows exist).
- **`override_reason` (decided, not spiked)**: the handler always calls `ValidateGates` with `OverrideReason` set (`server/services/backlog_service_lifecycle.go:929-933`), only Default `TransitionGuard` honors it, and `ConfiguredWorkflowEngine.ValidateGates` never reads it. Left alone, an `Enforce` operator gate (or the synthetic `stage-config-unresolvable` gate for a deleted stage, whose own ActionHint promises "Manual Override") would block even with an override, removing the documented escape hatch. Decision: a non-empty `override_reason` bypasses Default guards AND Configured operator gates, including the synthetic deleted-stage gate, but NEVER the graph check (`CanTransition`). Implemented inside `enforceAnswer` using the `OverrideReason` already present on the forwarded `BacklogItemTransitionInput`; the handler is not edited. Acceptance criteria are in Story 1.1.2 and Story 1.2.4; Task 0.1.1a characterizes Default-engine behavior only.
- **Fallback direction differs by concern**: graph falls back to Default UNION `SystemRequiredEdges()` on cache-empty; an unresolvable operator gate blocks (fail-closed, existing behavior, `configured_workflow_engine.go:242-345`). One generic "fall back" rule would get one of them wrong (`research/pitfalls.md` A5).
- **Mixed-mode within one request** (flag flips between `CanTransition` at `:892` and `ValidateGates` at `:933`): accepted. Default guards run in both modes, so a mixed request cannot drop a built-in guard; worst case is one request judged by the graph of one engine and the extra gates of the other.
- **Divergence classes** compared at the engine boundary, Default versus `enforceAnswer`: `graph`, `gate_missing` (Default blocks, `enforceAnswer` passes: a composition defect, impossible from an ordinary guard denial because Default guards are part of the composition), `gate_extra` (`enforceAnswer` blocks, Default passes: an operator gate), `error_kind` (via `errors.Is` on `Err*` sentinels, never `Description` text), `cache_empty`; labeled `config=seeded|edited` using `SeedFingerprint` over edges, enabled stages, per-edge gate count and liveness, and `path=transition|read`. Earlier drafts compared Default to the raw `ConfiguredWorkflowEngine`, which has no built-in guards, so every legitimate guard denial would have counted as `gate_missing` and the soak criterion `gate_missing=0` was unmeetable (adversarial review B1, architecture review blocker 1). The raw comparison now exists only in the offline harness (Story 1.1.1) as the informational `gate_builtin_absent_in_config` report.

## Alternatives rejected

- **Direct swap at `dependencies.go:1477`**: drops guards, denies all on load failure. Rejected by research.
- **Seed built-in guards as `TransitionGate` rows (option B)**: larger, needs new check IDs and a migration, and lets operators delete safety rails. UNVERIFIED value.
- **Refactor `TransitionBacklogItemStatus` first**: disproportionate; the handler consumes the engine through two narrow calls, so a seam below them leaves the hotspot untouched and outside the new-code complexity gate.
- **`looplab/fsm` / `qmuntal/stateless` / Temporal**: see `research/build-vs-buy.md` A2-A3.

## Consequences

- No new `ent` import in `server/services` (`no_ent_in_services` untouched).
- Rollback is a live flag flip (enforce off); code-level rollback is one line at `dependencies.go:1477`.
- Shadow data against production rows cannot be produced on a manual instance (own DB). The shadow runs on the live service behind the flag (read-only), per `research/pitfalls.md` A6.
- A UI that shows gates for a built-in edge renders the union (`GetPendingGates` already does).
- Correction to earlier research text: Configured evaluation is NOT purely in-process. `evaluateHumanApprovalGate` and `evaluateRecordedGate` call `gateSatisfactionRepo.GetByItemAndGate(context.Background(), ...)` (`session/configured_workflow_engine.go:284,374`), a DB read with an uncancellable context. Decision: shadow skips stateful gate kinds (`not_compared` counter) so it adds no I/O; `Enforce` keeps evaluating them (existing behavior in the lifecycle listener) but Task 1.1.2e bounds each read with a 2s timeout that fails closed. The seeded config has no stateful gates, so the hot path is unchanged until an operator adds one.

## Spike results (filled by Task 0.1.1b)

- LoadConfig() cost on the hot path: _pending_
- Scope-key validation for non-gate flags: _pending_
- Characterization of Default-engine `override_reason` behavior at `backlog_service_lifecycle.go:933-996`: _pending (Task 0.1.1a)_ (the Enforce semantics are decided above, not spiked)
