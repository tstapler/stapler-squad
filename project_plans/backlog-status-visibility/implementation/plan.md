# Implementation Plan: backlog-status-visibility

**Date**: 2026-08-01
**Status**: Adopt `project_plans/backlog-session-lifecycle-ux/implementation/plan.md` verbatim — see that file for the full architecture, pattern decisions, migration plan, and per-task acceptance criteria. Not re-derived here (see `requirements.md`'s duplicate-work finding).

## Phase Summary (condensed from the adopted plan)

| Phase | Scope | Depends on |
|---|---|---|
| 0 | Pre-flight: re-verify `end_reason` prerequisite commits (`0ac676001`, `f7ab0c9ad`) are merged to both `origin/main` and `upstream-fanatics/main` before starting Phase 1 on a fresh branch/worktree — **confirmed still unmerged as of 2026-08-01, hard gate, not optional** | none |
| 1 | `end_reason`: proto field → `itemSessionToProto` population → frontend type/mapper → `formatEndReason` → chip in `SessionsSection.tsx` | Phase 0 |
| 2 | `pause_reason`: promote `SessionCard.tsx` from tooltip-only to an always-visible badge (keep tooltip/aria-label too) | none |
| 3 | `BlockerChip` `×N` remediation-count suffix + next-retry countdown + `context` in tooltip, on board card and `LifecycleSummary.tsx` | none |
| 4 | New `RespawnEvent` ent schema → repository write (`CreateRespawnEvent`, **with dedupe guard**, pre-mortem #1) → eager-load in `GetBacklogItem` (capped, 50 most recent) → proto + conversion → instrument the 4 respawn call sites → frontend `RespawnHistorySection.tsx` (progressive disclosure, **with two-counter reconciliation caption**, pre-mortem #2) | 4.1→4.2→4.3→4.4→4.5 strictly sequential |
| 5 | Feature registry entries (`docs/registry/features/`) + `make registry-generate` + `make ci` | 1–4 complete |

Phases 1, 2, 3 are independent and parallelizable. Phase 4's five epics are strictly sequential. Phase 5 is last.

## Carried-forward pre-mortem/adversarial findings (do not drop during implementation)

1. **P1 — idempotency**: `CreateRespawnEvent` must dedupe on `(item_id, triggering_session_uuid, reason)` within a short window (~10s) before insert. The 4 call sites it instruments were patched weeks ago (`0ac676001`) for exactly this double-fire class from overlapping reconciliation sweeps.
2. **P1 — two counters diverge by design**: `BlockerChip`'s `×N` (episode-scoped) and `RespawnHistorySection`'s count (all-time) will show different numbers for any item that has cycled stuck→resolved→stuck more than once. Ship the explanatory caption (Story 4.5.2's 3rd AC), don't treat the divergence as a bug to "fix" by making them match.
3. **P2 — `formatEndReason` fail-open, not fail-silent**: an unrecognized future `end_reason` value must render a distinct "Unrecognized end reason" warning chip, not the same "no chip" treatment as genuine success — the adversarial review flags the naive default as indistinguishable from clean success, inverting the feature's purpose for the case that matters most.
4. **P2 — queued vs. failed empty state**: `resultingSessionUuid === ""` currently collapses "hit concurrency cap, will retry" and "spawn attempt failed" into one string; the `SpawnSessionFromItemResponse.queued` boolean already exists to disambiguate — use it.
5. **Unresolved (adversarial review)**: `RespawnHistorySection`'s "always render with explicit empty state" (features.md) vs. ux.md §4's general "omit empty sections" guidance are in direct tension; the plan picked always-render (matching `WorkflowHistorySection`'s precedent) — implementer should not silently flip this without re-reading both docs.

## Migration / Observability / Risk Control

See the adopted plan — additive-only ent auto-migration (one new table, no altered/dropped columns), no feature flag needed (observability-only, no behavior change), standard `log.ErrorLog` for write failures (no new metrics/alerting infra — single-user tool).
