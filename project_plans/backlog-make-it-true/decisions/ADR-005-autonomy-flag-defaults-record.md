# ADR-005: Autonomy flags are ON live, OFF in code; keep code defaults off until measurable exit criteria exist

**Status**: Proposed. State and rationale are recorded now; the default flip itself is an owner decision (wall-clock blocker). Promote to `docs/adr/` in Task 4.1.1a.
**Date**: 2026-10-17
**Project**: backlog-make-it-true, workstream D

## Context

| Flag | Code default | Live instance (VERIFIED 2026-10-17) | Declared in |
|---|---|---|---|
| `backlog:sdd-default-pipeline` | false (plain `GetFeatureFlag`, `backlog_service_lifecycle.go:136-150`) | enabled | read at item creation only; registry entry location UNVERIFIED |
| `diagnose_nudge_enabled` | false (`config/config.go:555-556`) | enabled | `config.go` and the `knownFeatureFlags` registry |
| `autonomous_stall_nudge_enabled` | false (`config.go:561,571`, `AutonomousStallNudgeDefault`) | enabled (AND-ed with diagnose) | `config.go` and `feature_flag_service.go:287-292` |

Evidence (`research/build-vs-buy.md` "Soak evidence"): live workspace `config.json` has all three true; no enablement date or actor recorded; the active log covers about 23 minutes; zero in-process stall-nudge outcomes observable; notification history has no nudge type; `DiagnoseNudgeCount` is not exposed over RPC. There is no queryable soak evidence. Known gaps that default-on would widen (`autonomous-stall-causes.md:151-166`): two nudgers per session, lifetime nudge cap never reset, stale-work/health-checker restarts still kill-and-respawn, `detection.StatusReady` is dead code so unattended PTY writes must be gated on `StatusIdle` plus a `StatusContext` allowlist (UNVERIFIED that the in-process nudge does so).

## Decision

1. **Keep code defaults OFF.** A fresh install must not auto-write into LLM panes or multiply headless-pool load (5-slot pool; sdd is long-lived and can exceed the WIP cap of 2 invisibly, `docs/tasks/backlog-feature-improvement.md:1364-1380`).
2. **The live ON state is a recorded operator opt-in**, not an inherited default. Rollback for all three is already a live flag flip. `sdd-default-pipeline` rollback is not retroactive (items created `sdd` stay `sdd`).
3. **Add the missing evidence**: a durable nudge-outcome counter and structured log (`nudge_outcome_total{kind=stall|diagnose, outcome=delivered|uncertain|slot_unavailable|fallback_respawn|skipped_cap|skipped_not_idle}`), a ledger file that survives restarts, an actuator endpoint, and `DiagnoseNudgeCount` surfaced. The enablement date is established from the flag service audit lines or recorded by the owner.
4. **Single-source defaults**: each default is declared once and the registry reads it from `config` (today `config.go` and `knownFeatureFlags` both declare it; the `HiddenSessionGateDefault` comment, `config.go:515-518`, exists for this). `backlog:sdd-default-pipeline` moves to `GetFeatureFlagWithDefault` with a registered `defaultValue` (still false); flipping the constant alone would change nothing otherwise. A test asserts registry default == config default for all three.
5. **Proposed exit criteria for flipping a code default on** (owner to confirm; Unresolved Question Q5): at least N=20 stall nudges delivered over at least 14 days on the live instance; zero `uncertain` outcomes; zero nudges followed by a respawn within 10 minutes (`fallback_respawn` after `delivered`); no increase in `autonomous_stuck` rows versus the 14 days before enablement; for sdd: stuck rate of sdd-created items not worse than flat-pipeline items over the same period (command: `ListBacklogItems` grouped by pipeline mode). Thresholds are INFERRED placeholders until the owner sets them.
6. **UI legibility** (workstream D-legibility): the Features page shows the effective value and its source (default vs explicit), the AND dependency ("On (inactive)" with reason when `diagnose_nudge_enabled` is off), and a Default/Flip-when/Rollback line served from the flag description or `statusDetail`, not hard-coded JSX.

## Alternatives rejected

- Flip all three defaults now: no evidence exists, and the owner's workspace is already on, so a flip would only change fresh installs without information gain.
- Leave undocumented: the current state, which is the problem.

## Consequences

- Q4 (derive quota budget) and Q5 (flip criteria) stay Unresolved Questions in the plan; neither blocks any story except the owner-gated default-flip task, which is deliberately not in the plan's critical path.
