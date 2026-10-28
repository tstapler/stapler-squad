# Implementation Plan: e2e-per-worker-server

**Status**: scheduled (go decision recorded in [ADR-003](../../e2e-test-parallelization/decisions/ADR-003-go-on-per-worker-server-rearchitecture.md)), not started.
**Feature**: Lift the single-shared-backend-server constraint in `tests/e2e/global-setup.ts`, sequenced behind a cheaper duration-aware shard balancer that the parent project's research recommended but never shipped.

## Epic 0: Duration-aware shard balancer (cheaper lever, try first)

**Goal**: Attack the proven-dominant cost (`architecture.md` §6: 2.0x-2.2x fastest/slowest shard spread, same 10-way split) with the lever the research actually recommended — binpacking by measured historical duration — which Story 3.1.1 did not implement (it shipped Playwright's native `fullyParallel: true`, a test-*count* split, and the checkpoint measured the spread getting worse, not better).

### Story 0.1: Build a blob-report-driven shard assignment script

`e2e-next-report`/`e2e-bun-report` already merge every shard's blob report into one JSON (confirmed by Story 3.1.2's comparator reading it) — this JSON already contains per-test durations. Write a script (`tests/e2e/scripts/plan-shards.ts` or similar) that:
- Reads the most recent merged blob-report JSON available (checked into the repo as a committed "last known timings" snapshot, refreshed periodically — mirrors the `@flakiness/playwright` `shardBalancing` pattern pitfalls.md §2 describes, but as an in-repo script, not a new dependency, per `build-vs-buy.md` §1's verdict against third-party balancers).
- Sums per-file duration (sum of its tests' last-recorded durations) and bin-packs files into N buckets (greedy longest-processing-time-first, a standard bin-packing heuristic) targeting even total duration per bucket, not even file/test count.
- Emits a `--shard`-compatible artifact: either N `.txt` files of newline-separated spec paths consumable via Playwright's `--grep`/explicit-file-list invocation, or a GH Actions matrix-compatible JSON array `build.yml` can read via `fromJson()`.
- Falls back gracefully (even file-count split, today's behavior) for any file missing from the timings snapshot — same degrade-gracefully property `@flakiness/playwright`'s own stale-hint-file behavior has (pitfalls.md §2), so a new/renamed spec file never breaks the split, just falls back to naive distribution for that one file.

**Acceptance criteria**:
- Given the current merged blob-report JSON from a recent `main` run, the script's computed bucket-duration spread is measurably tighter than the current 2.35x/2.48x (Story 3.3.1's measured post-Phase-3 numbers) when simulated against that same run's actual per-file durations.
- Given a spec file with no entry in the timings snapshot, the script assigns it to the currently-smallest bucket (by running total) rather than erroring.

### Story 0.2: Wire it into `build.yml`, validate on real CI

Replace (or offer alongside, if staged as a feature-flagged step) the current `--shard=N/M` matrix input with the script's computed assignment. Validate with the same before/after wall-clock + comparator-regression method Story 3.1.2/3.3.1 used.

**Rollback trigger**: same as Phase 3's stories — any new comparator `fail`-severity finding, or a wall-clock regression vs. the current post-Phase-3 `main` baseline (44.9 min, run `40392784895`) — revert immediately, same discipline as Stories 3.1.1/3.2.1.

**Decision point**: after Story 0.2's real-CI numbers land, compare against this project's Success Metric. If the spread closes to near the historical baseline (2.0x-2.2x) and wall-clock recovers to at or below the pre-Phase-3 35.73 min baseline, re-evaluate Epics 1-4's priority — they may still be worth doing for the larger structural win, but are no longer urgent. If the gap remains open, proceed to Epic 1 with that context recorded.

---

## Epic 1: Worker-scoped server fixture infrastructure

**Goal**: Build the extension point the seam doc designed, corrected for the verified reality that specs don't read Playwright's `baseURL` fixture.

### Story 1.1: `workerScopedServer` auto fixture + env-var bridge

The seam doc's original sketch (override the `baseURL` fixture) doesn't work given ADR-003's correction: 157 of 202 relevant files read a module-level `const` captured at import time, before any fixture — test-scoped or worker-scoped — can have run. The fix needs to work *with* that reality, not require touching every test's fixture-injection signature (a far larger, riskier change than a call-site rename).

Design: an **auto, worker-scoped** fixture (`{ scope: 'worker', auto: true }`) that runs exactly once per Playwright worker process, before any test in that worker executes, and does what `global-setup.ts` does today — `new TestServer({ port: 0 }); await server.start();` — but writes `process.env.TEST_SERVER_URL`/`TEST_SERVER_TESTDIR` into *that worker's own process environment* (each worker is a separate OS process under Playwright's default `workers: N`, so these writes never collide across workers) instead of the Playwright main process's environment. Because the fixture is `auto: true`, Playwright guarantees it resolves before the first test body in that worker runs — strictly before any call-site read of `process.env.TEST_SERVER_URL`, provided that read happens inside a test/hook body (not at module import time — see Story 2.1 below for why the call sites must also change, not just when they're read).

```ts
// tests/e2e/fixtures/worker-scoped-server.ts (concrete, not a sketch this time)
import { test as base } from '@playwright/test';
import { TestServer } from '../helpers/test-server';

export const test = base.extend<{}, { workerScopedServer: TestServer }>({
  workerScopedServer: [
    async ({}, use) => {
      const server = new TestServer({ port: 0 });
      await server.start();
      process.env.TEST_SERVER_URL = server.getBaseUrl();
      process.env.BASE_URL ??= process.env.TEST_SERVER_URL;
      process.env.TEST_SERVER_TESTDIR = server.getTestDir();
      await use(server);
      await server.stop();
    },
    { scope: 'worker', auto: true },
  ],
});
```

Every spec imports `test`/`expect` from this module instead of `@playwright/test` directly (a one-line import-path change per file, folded into Epic 2's codemod since it touches the same files).

### Story 1.2: `playwright.multiworker.config.ts`

A new, separate config file (not a branch inside `playwright.config.ts`, per ADR-001's and `isolated-dev-stacks`' shared precedent) with `fullyParallel: true`, `workers: N` (configurable, start at 2 for Epic 3's smallest validation step), reusing every existing `projects` entry unchanged. `playwright.config.ts` itself, and therefore every existing CI shard job and local `npx playwright test` run, is untouched.

### Story 1.3: Local smoke validation

Run a tiny subset (`smoke.spec.ts` + 2-3 others already known-independent per `architecture.md` §5's bounded audit) under the new config with `workers: 2` locally. Confirm via logs that two distinct `TestServer` instances start (distinct ports, distinct `ssq-e2e-<port>` tmux sockets, distinct PID-scoped testdirs) and both specs pass.

**Acceptance criteria**: two concurrently-running `TestServer` processes are observed (via `ps`/log timestamps) with non-overlapping port/testdir/tmux-socket values, and the smoke subset passes under `playwright.multiworker.config.ts` with `workers: 2`.

---

## Epic 2: Corrected 157-file call-site migration

**Goal**: Convert every file that captures `process.env.TEST_SERVER_URL`/`BASE_URL`/`TEST_SERVER_TESTDIR` into a module-level constant (evaluated at import time, before Story 1.1's auto fixture can have run) into a call-time read, so the per-worker value Story 1.1 publishes is actually the value each spec sees.

### Story 2.1: Shared lazy accessor helper

`tests/e2e/helpers/base-url.ts`:
```ts
export function baseUrl(): string {
  return process.env.TEST_SERVER_URL || process.env.BASE_URL || 'http://localhost:8544';
}
export function testDir(): string | undefined {
  return process.env.TEST_SERVER_TESTDIR;
}
```
A no-op under the existing single-server path (same env vars, same fallback, same precedence `global-setup.ts` already uses) — this is purely a *when it's read* change, not a *where the value comes from* change, so the single-server/20-shard-job path is provably unaffected (it reads `process.env` either way; only the timing of the read changes, and under the single-server path the env var is already set long before any test body runs, so there is no behavior difference to verify beyond "still reads the same two env vars with the same fallback").

### Story 2.2: Codemod the 17 `tests/e2e/pages/*.ts` helpers first (highest leverage)

These are imported by most of the 185 spec files (including many of the 62 already-clean ones, which depend on page objects like `AppShellPage.ts` for their `.goto()` wrapper) — fixing them first fixes navigation transitively for every spec that uses a page object, before touching spec files directly. Use `ast-grep`/`gritql` (per this repo's own skills) to mechanically replace:
```ts
const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
```
with
```ts
import { baseUrl } from '../helpers/base-url';
```
and every subsequent bare `BASE_URL` reference with `baseUrl()`.

**Acceptance criteria**: `bunx tsc --noEmit` clean, `make lint` clean, and the full `tests/e2e/pages/` directory contains zero remaining `process.env.TEST_SERVER_URL`/`process.env.BASE_URL` reads outside `helpers/base-url.ts` itself (grep-verifiable).

### Story 2.3: Codemod the 140 spec files with their own module-level constant

Same transform, batched (e.g. alphabetically, 20-30 files per batch) to keep each review diff reviewable and to run `tsc --noEmit`/`make lint`/a smoke-subset test after every batch rather than once at the end (per requirements.md's Rabbit Holes note: a bad conversion fails silently at runtime, not at compile time, if a call site ends up reading the accessor before the worker fixture has run).

### Story 2.4: The 7 already-flagged special-case files

- `connection-count-indicator.spec.ts`, `launcher-presets.spec.ts`: convert their `TEST_DIR = process.env.TEST_SERVER_TESTDIR` reads to `testDir()` from Story 2.1's helper.
- `alias.spec.ts`, `alias-settings.spec.ts`, `omnibar-backlog-intent-confirm.spec.ts`, `omnibar-backlog-compose.spec.ts`, `workflow-cron-schedule.spec.ts`: these read `process.env.BASE_URL` (not `TEST_SERVER_URL`) directly, per `global-setup.ts`'s own comment explaining why (`BASE_URL` is set as a fallback for specs that "read it directly instead of `TEST_SERVER_URL`"). Convert to `baseUrl()` the same way — the helper already checks both env vars in the right precedence order.

### Story 2.5: Post-migration verification sweep

Re-run the bounded grep from ADR-003/this project's requirements.md across all of `tests/e2e/`: confirm zero remaining `^const BASE_URL = process\.env` or raw `process\.env\.(TEST_SERVER_URL|BASE_URL|TEST_SERVER_TESTDIR)` matches outside `helpers/base-url.ts`, `helpers/test-server.ts`, `fixtures/worker-scoped-server.ts`, and `global-setup.ts` itself. Confirm the 62 already-clean spec files are byte-identical (git diff empty) — they should need zero changes.

---

## Epic 3: Empirical risk validation

**Goal**: Validate the three failure modes `pitfalls.md` §4 named, each of which "degrades into flaky-looking failures" rather than crashing loudly — so the validation method must specifically look for failure clustering by worker index, not just "did the suite pass once."

### Story 3.1: Port exhaustion / TOCTOU

Run `playwright.multiworker.config.ts` with `workers: 4` and `workers: 8` against the full suite, with each worker's bound port logged (add a one-line log in Story 1.1's fixture, tagged with that worker's `parallelIndex` and the server's bound base URL). Confirm no port-bind failures across ≥3 repeated runs at each worker count.

### Story 3.2: Resource contention on one runner

Run the full suite under `playwright.multiworker.config.ts` with `workers: 2`, then `4`, on an actual GitHub-hosted runner via a temporary `workflow_dispatch`-triggered job (not wired into `web-build-gate`'s `needs:` yet). Compare wall-clock and failure-clustering-by-worker-index across ≥3 repeated runs at each worker count against the single-worker 10-way-job baseline. Per pitfalls.md §4's framing: a failure rate that clusters on specific worker slots (not spec files) across repeated runs is the resource-contention signature to look for, distinct from ordinary flakiness.

### Story 3.3: Isolation completeness

Re-verify `STAPLER_SQUAD_INSTANCE`/`STAPLER_SQUAD_TMUX_SOCKET`/testdir uniqueness empirically across concurrent workers (log and diff each worker's resolved values; confirm no collisions). Specifically re-test `remote-workspaces.spec.ts`'s already-fragile TOFU-dialog-skip workaround (`global-setup.ts`'s own comment flags this as the one known instance of "shared state breaking a cross-cutting assumption") under N-worker mode, since this is the one place the suite has already been bitten by exactly this class of bug once.

**Acceptance criteria (all three stories)**: ≥3 repeated full-suite runs at each tested worker count, zero failures attributable to port/contention/isolation causes (vs. the suite's already-known baseline flakiness rate, which this validation must not conflate with a new regression).

---

## Epic 4: Rollout decision

### Story 4.1: Go/no-go on wiring into the gating path

Based on Epic 0's and Epic 3's real numbers: either (a) stop at Epic 0 if the duration-aware balancer alone closed the gap, (b) wire `playwright.multiworker.config.ts` into `build.yml` as a replacement for (or addition to) the current 10-way job-level matrix if Epic 3 validated cleanly, or (c) record a new finding note and further-deferred decision if Epic 3 surfaced a blocking risk. Staged as its own PR, per the original requirements' Risk Control section — never conflated with Epic 0's or Epic 2's PRs.

---

## Sequencing

```
Epic 0 (cheap, try first) ──> decision point ──┬──> stop here if gap closed
                                                 └──> Epic 1 ──> Epic 2 ──> Epic 3 ──> Epic 4
```

Epic 1 and Epic 2 can run concurrently with Epic 0 if the owner wants to parallelize rather than gate strictly — Epic 0 has no file overlap with Epics 1-2's fixture/call-site work. The strict dependency is only: Epic 3 needs Epics 1+2 both merged (needs the fixture infra and the migrated call sites to run the real suite under multiworker mode), and Epic 4 needs Epic 3's real numbers.
