# Requirements: e2e-test-parallelization

**Date**: 2026-10-25
**Type**: feature addition (CI/test-infrastructure optimization, with a possible test-content-rework component)
**Complexity**: 3 — system design

## Problem Statement

The `tests/e2e/` Playwright suite (193 spec files, ~881 `test()` cases, ~30.7k lines) runs against a **single shared backend server process per CI job** (`tests/e2e/global-setup.ts` spawns one `stapler-squad` binary for the whole run; its port/testdir are exported via `process.env` and inherited by every Playwright worker). That constraint is why `playwright.config.ts` pins `workers: 1` / `fullyParallel: false` — concurrent in-process workers would race against the one server (one documented collision already exists: a TOFU-dialog-skip workaround in `global-setup.ts` for `chromium-dom`'s shared-server dependency).

Earlier this session (PR #1159, #1166), the *job-level* dimension of this was already addressed: `e2e-next`/`e2e-bun` in `build.yml` now shard across 10 independent GitHub Actions jobs, each with its own fully isolated server/port, cutting a ~3h20m serial run to roughly 25-40 minutes per shard. That fix is merged and live on `main`.

What's unexamined: whether job-level sharding is the *only* lever available, whether the current shard count/shape is actually optimal, whether Bun (the repo's package manager and JS runtime) offers additional speed levers beyond what's already used, and whether the suite's *content* — 193 full-stack browser specs — is testing at the right altitude, or whether some of it could move to lighter, faster test types without losing real coverage.

## Baseline

- `e2e-next`/`e2e-bun`: 10-way job-level shards, each ~25-40 min, run in parallel (wall-clock ≈ slowest shard, not the sum).
- Each of those 20 shard jobs independently repeats: Go setup, `bun install` (tests/e2e), Playwright browser install, and (for `e2e-bun`) a full `make stapler-squad` web-UI+binary build — fixed per-job overhead that scales with shard count and is not shared/cached across shards today.
- Within a single shard, execution is still **sequential** (`workers: 1`) — no in-process parallelism at all, by design, due to the single-server constraint.
- A *separate*, smaller Playwright tree (`web-app/tests/e2e/`, 19 specs) exists for `benchmark.yml` and is not part of the main gate; out of scope here unless research finds it meaningfully informs the main suite's approach.
- `bun test` (Bun's own native test runner) was already evaluated as a Jest replacement for `web-app`'s ~763 unit-test files and **segfaulted after ~90s with 125 failures** on the currently-installed Bun version; the owner already decided (2026-10-19, recorded in `docs/explanation/bun-migration-feasibility.md` and `project_plans/web-app-bun-build/requirements.md`) to keep Jest and revisit Rstest/Vitest separately. **This decision stands — do not re-propose `bun test` as a Jest replacement.** (Bun's test runner is also not the right tool for *browser* e2e specs regardless; Playwright owns that role.)

## Users / Consumers

- Every contributor opening a PR that touches `web-app/`, `session/`, `server/`, or `tests/e2e/` — they wait on `e2e-next`/`e2e-bun` as part of `web-build-gate`.
- The repo owner, who personally felt the pain of the pre-fix ~3h20m serial runtime and the chronic-red-gate bug earlier this session, and who merged the first fix (#1166) directly.

## Success Metrics

- Measured wall-clock for `e2e-next`/`e2e-bun` to reach a final verdict (slowest shard + merge/comparator jobs) drops further from the current post-#1166 baseline (~25-40 min/shard), with the specific delta to be established once research quantifies which levers are worth pulling.
- No regression in what the suite actually verifies: the comparator's pass/flaky/fail baseline classification semantics (Story 7.10/7.11, already shipped) must keep working unchanged, and route coverage (route-coverage-manifest.ts) must not drop.
- If test-content rework is recommended and implemented: the specs that move to a lighter test type keep equivalent assertions/coverage, verified by the existing baseline-classification tooling not flagging new unexplained failures.

**Wall-clock blockers**: none identified yet — this is pre-plan. If the plan phase recommends rearchitecting `global-setup.ts`'s single-shared-server model (to unlock true in-process `fullyParallel`/multi-worker execution on top of the existing job-level sharding), that specific sub-effort may need a soak/validation window on CI similar to what Stories 7.10/7.11 and the #1165/#1166 saga just required (multiple full-suite CI cycles to catch sharding-specific bugs) — flag as a likely blocker once scoped, not assumed now.

## Constraints

- Must not regress the just-shipped (#1166) baseline-classification/comparator machinery (Stories 7.10/7.11) or `web-build-gate`'s pass/fail semantics.
- Must not re-open the closed `bun test`-as-Jest-replacement question (see Baseline).
- Changes to `.github/workflows/build.yml` must pass `actionlint` and the existing `web-build-gate.test.mjs` test suite (or update it deliberately, with reasoning, same as this session's #1165/#1166 work did).
- Public GitHub repo → generous Actions concurrency, but not unlimited; any shard-count recommendation should stay within what the plan phase can confirm is comfortably supported (the session's own prior work found this not to be a binding constraint at 8-10 shards).

## Non-functional Requirements

- **Performance SLO**: not formally specified; directionally, "noticeably faster than ~25-40 min/shard" is the target research should quantify.
- **Scalability**: N/A (CI job count, not user-facing load).
- **Security classification**: internal (CI infrastructure for a personal/public repo; no sensitive data in scope).
- **Data residency**: not applicable.

## Scope

### In Scope

- Research: whether the single-shared-backend-server constraint in `global-setup.ts`/`helpers/test-server.ts` can be safely lifted (e.g., spinning up N isolated server instances, one per Playwright worker, inside a single job) to add true in-process parallelism on top of the existing job-level sharding — and what that would cost/risk.
- Research: whether shard count (currently 10) is well-tuned, or whether per-shard fixed overhead (repeated Go/Bun/Playwright-browser setup, repeated `make stapler-squad` build) is now a bigger lever than further sharding — including whether build artifacts can be shared/cached across shards more aggressively than today.
- Research: any other Bun-specific speed levers not yet used for the *Playwright* e2e path specifically (install/cache strategy, `bunx` vs `npx` overhead, Bun's own HTTP server or process-spawning primitives if relevant to `global-setup.ts`'s server lifecycle) — explicitly excluding re-litigating `bun test` as a Jest replacement.
- Research + triage: whether any meaningful subset of the 193 e2e specs are testing something that doesn't actually need a real backend + real browser (e.g., pure client-state assertions already heavily mocked) and could move to a faster test tier without losing the coverage intent — and a concrete, reviewed list of candidates if so (not a blind mass-conversion).
- Implementation of whichever findings the plan phase (with adversarial review) judges safe and valuable to land now; staging riskier items (e.g., the shared-server rearchitecture) as explicit follow-up stories if the adversarial review flags them as too risky for this round, rather than silently dropping them.

### Out of Scope

- Replacing Jest with `bun test` for `web-app`'s unit-test suite (closed decision, see Baseline).
- Replacing Playwright with a different e2e framework.
- The `web-app/tests/e2e/` benchmark-only tree, unless research finds it directly informs the main suite's approach.
- Anything touching Story 8.1/8.2's cutover-gate or `WEB_BUILD` default-flip decisions (separate, owner-sign-off-gated track).

## Rabbit Holes

- **Shared-server rearchitecture**: the highest-potential-value, highest-risk item. `global-setup.ts` currently exports one dynamic port/testdir via `process.env` for all workers to share; supporting N servers would mean either worker-indexed env vars Playwright's fixture system can read per-worker, or a different test-server lifecycle entirely. This session's own #1165/#1166 saga (job-level sharding alone) took multiple full CI cycles (each 25min-3h+) to get right, including one real missing-dependency bug found only in CI. A deeper architecture change here should expect a similarly iterative, CI-cycle-expensive validation process — budget for it explicitly in the plan rather than assuming a single clean pass.
- **Test-content retriage**: 193 specs is a lot to manually re-judge "does this really need a full browser+backend." A shallow pass risks either (a) rubber-stamping everything as "needs full e2e" (no actual finding) or (b) recommending conversions that quietly drop real coverage. The plan phase should scope this as a bounded sample/methodology, not "triage all 193."
- **Fixed per-shard overhead**: tempting to assume "just cache more," but the existing `prepare` job already shares a Next-mode build artifact across jobs; `e2e-bun` specifically rebuilds for Bun mode via `make stapler-squad`'s `WEB_BUILD_STAMP` seam, which may have real reasons for not being shareable across shards (same-build-different-seed concerns, concurrent-write hazards). Research should read why before proposing a sharing scheme, not just note that rebuild-in-every-shard looks wasteful.

## Alternatives Considered

- Leave the suite as-is post-#1166 (25-40 min/shard, 10-way). This is the do-nothing baseline the research needs to beat to justify further work.
- `bun test` as the e2e runner — rejected outright: it's a unit-test runner, not a browser-automation tool, and the adjacent Jest-replacement attempt already segfaulted on this stack.

## Feasibility Risks

- The shared-server constraint may turn out to be load-bearing for reasons beyond what a surface read of `global-setup.ts` reveals (e.g., some specs may rely on cross-test state continuity within a single server lifetime that isn't obviously visible from the fixture code) — research must verify, not assume, that per-worker servers are safe.
- GitHub Actions concurrency ceilings (even though not currently binding) could become a real constraint if shard count is pushed much higher than 10 — should be checked empirically, not assumed infinite.
- Any CI workflow change here risks repeating this session's own chronic-red-gate incident (a hardcoded default-member list breaking on every future PR) if gate-membership semantics are touched again without equally careful validation.

## Observability Requirements

Standard CI signal (job pass/fail, timing visible in the Actions UI and `gh run view`) is sufficient — no new metrics/alerting needed. If shard count or server-lifecycle changes land, the plan should specify what before/after timing comparison proves the change worked (mirroring how #1166's own PR body cited exact run/job IDs and timestamps as evidence).

## Risk Control

- Any workflow-file (`build.yml`) change must be validated with `actionlint` plus the existing `web-build-gate.test.mjs` suite before pushing, and should expect at least one full CI cycle to prove out before being considered done (per this session's own experience).
- If the shared-server rearchitecture is attempted, stage it behind its own PR, separate from any smaller/safer wins (overhead caching, shard-count tuning), so a problem in the risky change doesn't block or get conflated with the safe ones — same lesson this session's #1165 (superseded by the simpler, already-shipped #1166) illustrates directly.
- Rollback for any CI change is trivial by construction (revert the workflow-file commit) — no feature flag needed.

## Open Questions

- Is the current 10-way shard count actually near-optimal, or does per-shard fixed overhead now dominate enough that fewer, larger shards (or a different sharding strategy) would win? (For research.)
- Can per-shard setup cost be reduced (shared build artifacts, better caching) without reintroducing the cross-job race conditions that motivated per-job isolation in the first place? (For research.)
- Does lifting the single-shared-server constraint (N servers per job, true in-process `fullyParallel`) pencil out as worth the implementation/validation cost, or does job-level sharding alone already capture most of the available win? (For research, with a plan-phase go/no-go recommendation.)
- Is there a principled, boundable way to identify e2e specs that don't need a real backend+browser, or does the suite's content genuinely require full-stack testing throughout? *(Unresolved after Phase 2 research — no agent produced a bounded methodology. Owner decision 2026-10-25: scope a bounded sample triage as an explicit story in this round's plan, not deferred to a future project, and not an exhaustive 193-spec retriage.)*
