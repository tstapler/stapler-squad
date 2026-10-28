# Requirements: e2e-per-worker-server

**Date**: 2026-10-27
**Type**: CI/test-infrastructure optimization (continuation of a deferred item)
**Complexity**: 3 — system design

## Problem Statement

`project_plans/e2e-test-parallelization`'s Phase 3 (duration-neutral `fullyParallel: true` rebalancing + chromium-only shard installs) shipped and was measured at Story 3.3.1's checkpoint: `percent_improvement = -43.3%` against the pre-Phase-3 baseline (35.73 min → 51.22 min, slowest-shard-start to `web-build-gate` completion) — a regression, not an improvement. Full numbers: [`project_plans/e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md`](../e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md).

[ADR-003](../e2e-test-parallelization/decisions/ADR-003-go-on-per-worker-server-rearchitecture.md) records the go/no-go decision this project acts on: **Go**, sequenced behind a cheaper duration-aware shard balancer that was recommended by the original research (`architecture.md` §6) but never actually implemented — Story 3.1.1 shipped Playwright's native `fullyParallel: true` (a test-*count* split) instead of a true duration-*aware* split, and the checkpoint shows that bet didn't pay off (shard-duration spread got worse, not better: 2.0x/2.16x baseline → 2.35x/2.48x).

This project does not re-run Phase 1-2 research from scratch — it inherits `project_plans/e2e-test-parallelization/research/{architecture,build-vs-buy,pitfalls}.md` and `docs/explanation/e2e-per-worker-server-seam.md`'s design sketch wholesale, with one load-bearing correction: the seam design's claim that only two spec files need to change is wrong. Verified by direct grep + spot-read (see ADR-003): 140 of 185 spec files plus all 17 `tests/e2e/pages/*.ts` helpers declare their own module-level `const BASE_URL = process.env.TEST_SERVER_URL || '...'` and never read Playwright's injected `baseURL` fixture — so the real migration surface is ~157 files, not 2. It is still mechanical (one of two near-identical textual patterns throughout) and codemod-able, not an open-ended rewrite.

## Baseline

- Pre-Phase-3: 35.73 min slowest-shard-start-to-verdict (run `40184788547`), 2.0x/2.16x shard spread.
- Post-Phase-3 (current `main`, both levers merged): ~44.9 min (run `40392784895`), 2.68x/2.01x spread — still a ~26% regression against the pre-Phase-3 baseline, though improved relative to PR #1197's own isolated validating run (51.22 min).
- 10-way job-level sharding (`#1166`) is unchanged and stays unchanged — this project adds an opt-in in-process parallelism mode on top of it, not a replacement.
- The suite: 185 top-level spec files + `tests/e2e/pages/` (17 files) + `tests/e2e/terminal-stress/`, `tests/e2e/tools/` subdirs.

## Users / Consumers

Same as the parent project: every contributor whose PR waits on `web-build-gate`'s `e2e-next`/`e2e-bun` jobs; the repo owner, who already felt Phase 3 make this worse, not better.

## Success Metrics

- Epic 0 (duration-aware balancer) alone: shard-duration spread (fastest/slowest within one `e2e-next-shard`/`e2e-bun-shard` matrix) measurably below the 2.0x-2.2x baseline, on at least one real CI run, using the same `gh api .../actions/jobs/<id>` step-timestamp method the parent project's checkpoints used.
- Epics 1-4 (N-server rearchitecture), if Epic 0 doesn't close enough of the gap: a `playwright.multiworker.config.ts` run with `workers: N` (N≥2) on one CI runner completes with wall-clock below the single-worker-per-job baseline, with zero failure clustering attributable to port/resource/isolation contention across ≥3 repeated runs (Epic 3's acceptance criterion).
- No regression in comparator semantics or route coverage, same invariant the parent project held throughout.

**Wall-clock blockers**: real CI cycles for every validation step (same as the parent project — budget for iterative, multi-cycle validation, not a single clean pass).

## Constraints

- `playwright.config.ts`'s `fullyParallel`/`workers` settings and `global-setup.ts`'s single-server lifecycle stay unchanged for the default `npm test`/existing 20-shard-job path — the new mode is additive (`playwright.multiworker.config.ts` + a new fixture module), per ADR-001's and ADR-003's risk-containment strategy.
- The 157-file call-site migration must leave the 62 already-clean spec files untouched (verify via the same grep, post-migration, finding zero remaining module-level `BASE_URL`/`TEST_SERVER_TESTDIR` reads outside the new shared helper).
- Must not regress `web-build-gate`'s comparator/route-coverage semantics (same constraint as the parent project).
- The N-server mode stays out of `build.yml`'s gating path (`needs: web-build-gate`) until Epic 3's empirical validation passes.

## Scope

### In Scope

- Epic 0: a custom, repo-owned duration-aware shard balancer (binpacks spec files into N buckets by last-run duration from the blob-report JSON `e2e-next`/`e2e-bun` already produce), as a cheaper, lower-risk alternative/complement to the N-server rearchitecture — tried and measured before committing further to Epics 1-4's larger investment.
- Epic 1: the worker-scoped `TestServer` fixture module + opt-in `playwright.multiworker.config.ts`, per the seam doc's design (refined — see plan.md Story 1.1 for why an auto worker-scoped fixture + lazy accessor beats overriding the `baseURL` fixture given the corrected 157-file reality).
- Epic 2: the corrected 157-file call-site migration (mechanical, ast-grep/gritql-assisted).
- Epic 3: empirical validation of the three named risks (port exhaustion, resource contention, isolation completeness) via repeated CI runs on an isolated (non-gating) workflow path.
- Epic 4: go/no-go on wiring the validated multiworker config into `build.yml`'s gating path, staged as its own PR.

### Out of Scope

- Re-opening the `bun test`-as-Jest-replacement question (still closed, same as the parent project).
- Replacing Playwright or the comparator/baseline-classification machinery.
- Anything touching Story 8.1/8.2's `WEB_BUILD` cutover-gate track.

## Rabbit Holes

- **Epic 0 becomes its own rabbit hole if scope-crept into a general-purpose balancer library.** Keep it to "read the existing blob-report JSON this repo already merges, bucket files by last duration, write a shard-assignment file `--shard`-compatible jobs can consume" — not a new dependency, not a generalized tool.
- **Epic 2's codemod risk is "mechanical but large."** A single mis-converted call site (e.g. a page-helper method used both inside and outside a worker-scoped-fixture context) fails silently as a `page.goto('http://localhost:undefined/...')`-shaped runtime error, not a compile error, if the accessor returns `undefined` before the worker's server has started. Batch the migration and run `make lint`/`tsc --noEmit` + a smoke subset after every batch, not once at the end.

## Feasibility Risks

Unchanged from the parent project's pitfalls.md §4 (port exhaustion, resource contention, isolation completeness) — reproduced in [`implementation/plan.md`](implementation/plan.md) Epic 3, not restated here.

## Risk Control

- Epic 0 and Epics 1-4 ship as separate PRs, same discipline as Phase 3.
- The multiworker config stays non-gating until Epic 3 passes.
- Rollback is trivial by construction for every epic here (revert the config/fixture-module commit; Epic 2's codemod reverts as a unit per batch).
