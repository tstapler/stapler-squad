# ADR-001: Per-Fix Feature Flags, Not a Single Umbrella Flag

**Date**: 2026-08-13
**Status**: Accepted
**Project**: terminal-resync-reliability

## Context

`requirements.md`'s Risk Control section requires every one of the five fix categories
(visibility-scoped resync, correlation ID, server capacity fixes 3a/3b, staggering/
prioritization, batching/compression) to be individually gated behind a feature flag,
following the `TmuxExecGateConfig` (`config/types.go:84-103`) config-driven pattern. Its
Open Question #1 states the author is "leaning toward per-fix flags" but leaves the final
call to planning.

The existing flag registry (`server/services/feature_flag_service.go:45-77`'s
`knownFeatureFlags`) already holds 7 independent, narrowly-scoped flags — there is no
precedent in this codebase for one flag gating multiple unrelated behaviors.

## Decision

Register 7 flags, one per fix (3a and 3b split, batching split from compression):
`terminal:resync-visibility-scope`, `terminal:resync-correlation-id`,
`terminal:resync-skip-stale-dimension-slowpath`, `terminal:resync-exec-gate-fast-lane`,
`terminal:resync-stagger`, `terminal:resync-compression`, `terminal:resync-batching`. All
default `false`. Each fix's code checks only its own flag; no fix's code path reads another
fix's flag.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Single umbrella `terminal:resync-fixes` flag | A regression discovered in one fix (e.g. batching corrupting a capture) would force disabling five/six other independently-safe fixes simultaneously, directly contradicting the point of feature-flagging incremental changes |
| Two flags (client-bundle, server-bundle) | Splits along the wrong axis — a single server-side regression (e.g. the exec-gate fast lane) would still force disabling correlation-ID and stale-dimension-skip together, since both are also server-side |

## Consequences

- 7 new registry entries in `knownFeatureFlags`, each with its own operator-facing
  description (Task 1.2.1.1-1.2.1.3 in `implementation/plan.md`).
- Rollout can proceed fix-by-fix in the order recommended by `implementation/plan.md`'s
  Migration Plan, with each step independently reversible.
- More flags to eventually retire once all fixes are proven stable and promoted to
  default-on/removed — an accepted long-term cleanup cost in exchange for blast-radius
  isolation during rollout.
