# ADR-001: Isolate via seam — defer the per-worker N-server rearchitecture

**Date**: 2026-10-25
**Status**: Accepted

## Context

`tests/e2e/playwright.config.ts` pins `fullyParallel: false`/`workers: 1` because `global-setup.ts` spawns exactly one shared `stapler-squad` backend (`TestServer`) per CI job, and every Playwright worker inherits its URL/test-dir via a single, unindexed `process.env` write. Lifting this — one isolated `TestServer` per worker, via a worker-scoped fixture — would unlock true in-process parallelism on top of the already-shipped (#1166) 10-way job-level sharding.

Phase 2 research (`architecture.md`, `build-vs-buy.md`, `pitfalls.md`) found:
- `TestServer`'s internals (port, tmux socket, test-dir, bin-dir) are already per-instance-safe — the blocker is purely `global-setup.ts`'s lifecycle/discovery layer (architecture.md §1-2).
- Real CI timing (architecture.md §6, run `40184788547`) shows the dominant cost in the current 10-way split is shard-to-shard duration **imbalance** (2.0x-2.2x spread), not fixed per-job overhead (~3-3.5 min, ~10-20% of the slowest shard) — and a much cheaper, already-in-scope fix (`fullyParallel: true`, this plan's Story 3.1.1) attacks that imbalance directly.
- The correctness risk (per-spec cross-test state assumptions) is only substantially, not fully, de-risked by the already-running 20-job matrix — architecture.md §5 is a bounded grep (~1-4% of 193 files flagged), not a full audit.
- The resource-contention risk (N server+tmux stacks sharing one CI runner's fixed CPU/RAM instead of N dedicated runners) is genuinely unverified and would need the same kind of iterative, multi-CI-cycle validation the #1165→#1166 saga already cost this session once this quarter.
- The blast radius of touching `playwright.config.ts` for this is larger than a precedent case (`isolated-dev-stacks`) that already rejected branching the single-server config in-place: flipping `fullyParallel`/`workers` applies to all 10 `projects` entries, not a scoped subset.

## Decision

**Isolate via seam.** Design the extension point now — a worker-scoped Playwright fixture overriding the `baseURL` fixture, reusing `TestServer` unchanged, in a separate opt-in config file (not a branch inside the existing `playwright.config.ts`/`global-setup.ts`) — and document it (this plan's Phase 5, `docs/explanation/e2e-per-worker-server-seam.md`). Do not implement N-server orchestration in this round.

Not **Refactor-first**: the evidence above shows the problem this would solve (shard-duration imbalance, fixed overhead) is smaller and cheaper to fix than advertised, via Story 3.1.1/3.2.1 instead.

Not **Extend as-is** either: a concrete, cheap-to-write extension point exists and is worth recording now (two known call sites, a known config-file shape) rather than treated as untouchable.

## Consequences

- This round's implementation scope (Phase 3) captures the actual dominant costs (imbalance, Firefox/WebKit install overhead) at a fraction of the risk.
- A future implementer starts from a bounded design (Phase 5's doc) instead of re-deriving `architecture.md` §1-4 and `build-vs-buy.md` §3-4 from scratch.
- If CI timing after Phase 3 ships still shows a wall-clock floor the owner wants to push past, the N-server rearchitecture remains available as a future, separately-staged project — its go/no-go is unchanged by this ADR, only its implementation is deferred.

## References

- `project_plans/e2e-test-parallelization/research/architecture.md` §1-§7
- `project_plans/e2e-test-parallelization/research/build-vs-buy.md` §3-§4
- `project_plans/e2e-test-parallelization/research/pitfalls.md` §4
- `project_plans/e2e-test-parallelization/implementation/plan.md` Phase 5
