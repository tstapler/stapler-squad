# Per-Worker E2E Backend Server Seam (Deferred N-Server Design)

**Status: not implemented this round — see [ADR-001: Isolate via seam, defer N-server rearchitecture](../../project_plans/e2e-test-parallelization/decisions/ADR-001-isolate-via-seam-defer-n-server-rearchitecture.md).** This doc exists so a future implementer starts from a bounded design instead of re-deriving it from scratch. No code described below has been written.

## The blocker is `global-setup.ts`, not `TestServer`

`tests/e2e/helpers/test-server.ts`'s `TestServer` class is already per-instance-safe on every axis that matters for running N copies concurrently:

| Field | Current scoping | Per-worker-ready? |
|---|---|---|
| `port` | `findFreePort()` picks a free OS port at `start()` time | Yes, already — each instance gets its own port independent of any other instance in the process. |
| `testDir` | PID-scoped (`` /tmp/stapler-squad-test-${process.pid} ``) | Yes, already — each Playwright worker is a distinct OS process with a distinct PID. |
| `STAPLER_SQUAD_TMUX_SOCKET` | Port-scoped (`` ssq-e2e-${this.config.port} ``) | Yes, already — ports are already unique per instance. |
| `binDir` | PID-scoped copy of the shared repo-root binary | Yes, already — exists specifically so concurrent e2e runs against one checkout can't race a rebuild out from under each other. |

(Table reproduced from [`research/architecture.md` §1](../../project_plans/e2e-test-parallelization/research/architecture.md).)

The actual blocker is lifecycle and discovery, both owned by `tests/e2e/global-setup.ts`:

1. `global-setup.ts` runs once, in Playwright's single main process, before any worker forks. It calls `startGlobalTestServer()` once and writes `process.env.TEST_SERVER_URL` / `process.env.TEST_SERVER_TESTDIR` — a mutation of the *process's own* environment, inherited by every worker spawned afterward. `process.env` is a single flat namespace; there is no mechanism today for worker 2 to see a different port than worker 1.
2. Two specs read that env var directly rather than through a fixture (see below) — they'd break first under per-worker servers, since a module-level `const` captured once at import time can't express "whichever server *this worker's* tests are talking to."

So the seam to design is: replace the single global call + unindexed `process.env` publish with something Playwright's own worker model understands.

## The worker-scoped-fixture shape

Playwright's mechanism for "give each worker something different" is a worker-scoped fixture (`{ scope: 'worker' }`): the fixture function runs once per worker process, lazily, on first use, and its return value is cached for every test that worker runs.

Sketch (not an implementation — field/method names match `TestServer`'s actual API: `new TestServer(config: Partial<TestServerConfig>)`, `start()`, `stop()`, `getBaseUrl()`, `getTestDir()`):

```ts
// tests/e2e/fixtures/worker-scoped-server.ts (sketch)
import { test as base } from '@playwright/test';
import { TestServer } from '../helpers/test-server';

type WorkerFixtures = {
  testServer: TestServer;
  testServerDir: string;
};

export const test = base.extend<{}, WorkerFixtures>({
  testServer: [
    async ({}, use) => {
      // port: 0 → findFreePort() already handles per-instance port allocation;
      // testDir derivation is left to TestServer's existing PID-scoping.
      const server = new TestServer({ port: 0 });
      await server.start();
      await use(server);
      await server.stop();
    },
    { scope: 'worker' },
  ],

  testServerDir: [
    async ({ testServer }, use) => {
      await use(testServer.getTestDir());
    },
    { scope: 'worker' },
  ],

  // Override the BUILT-IN baseURL fixture, not playwright.config.ts's static
  // use.baseURL value — this is the actual discovery mechanism. Every spec's
  // existing page.goto('/')-against-baseURL calls get transparently redirected
  // to this worker's own server with zero per-spec changes.
  baseURL: async ({ testServer }, use) => {
    await use(testServer.getBaseUrl());
  },
});
```

Overriding the built-in `baseURL` fixture (rather than the config's static `use.baseURL`) is the load-bearing choice: Playwright already resolves `baseURL` through the fixture system per test, so every existing spec that calls `page.goto('/')` or a relative path keeps working unchanged. `remote-workspaces.spec.ts`'s existing per-file `sshd` pattern (file-scoped `beforeAll`/`afterAll`) needs no change either — a worker-scoped fixture assigns a whole spec file to one worker, the same granularity that pattern already assumes, unless a file opts into `test.describe.configure({ mode: 'parallel' })` (this suite doesn't).

## The two call sites that bypass fixtures

Two specs read `process.env.TEST_SERVER_TESTDIR` directly instead of going through a fixture, because they write/verify a config file on disk outside the RPC surface:

- [`tests/e2e/connection-count-indicator.spec.ts:39`](../../tests/e2e/connection-count-indicator.spec.ts#L39):
  ```ts
  const TEST_DIR = process.env.TEST_SERVER_TESTDIR;
  ```
- [`tests/e2e/launcher-presets.spec.ts:21`](../../tests/e2e/launcher-presets.spec.ts#L21):
  ```ts
  const TEST_DIR = process.env.TEST_SERVER_TESTDIR;
  ```

Both capture the value once at module import time via a top-level `const`. Under per-worker servers, "this worker's server's testDir" is a different value per worker — a module-level constant captured once cannot express that; it would silently resolve to whichever server happened to set the env var first. Both call sites would need to switch to reading the `testServerDir` worker fixture sketched above (e.g. by moving the read inside a `test()`/`beforeAll()` body that has fixture access) instead of the module-level `process.env` read.

## Why a separate config file, not a branch in the existing one

[`research/build-vs-buy.md` §3-4](../../project_plans/e2e-test-parallelization/research/build-vs-buy.md) and [`research/architecture.md` §3](../../project_plans/e2e-test-parallelization/research/architecture.md) both point at the same precedent: [`project_plans/isolated-dev-stacks/research/architecture.md` §3-4](../../project_plans/isolated-dev-stacks/research/architecture.md), written independently for an unrelated opt-in dev-mode feature (pairing a Go binary with `next dev`), which hit the identical structural fact and rejected branching the existing config:

> "`playwright.config.ts`'s `use.baseURL` and the global-setup's server lifecycle are both single-server-shaped by construction (one `TestServer` instance, one base URL comment says "dynamically assigned by global-setup"); branching all of that on a mode flag inside one file/one global-setup is more invasive than standing up a second small config that imports/reuses the same `TestServer`-style helper extended for two processes, and it keeps the default `npm test` run's behavior and latency completely unchanged..."
> — [`isolated-dev-stacks/research/architecture.md:208-214`](../../project_plans/isolated-dev-stacks/research/architecture.md)

The same fact applies here, more severely. True in-process multi-worker parallelism additionally requires flipping `playwright.config.ts`'s `fullyParallel: false` → `true` and `workers: 1` → `N` ([`tests/e2e/playwright.config.ts:59,65`](../../tests/e2e/playwright.config.ts#L59)) — settings that apply to the **whole config file**, i.e. to all `projects` entries, not just the specs a new mode would cover. Mutating the existing file would change the concurrency model for every spec and every local `npx playwright test` run on day one, with no way to stage it incrementally within one file. A separate config (e.g. `playwright.multiworker.config.ts`, mirroring the dev-stacks precedent's own `playwright.dev-mode.config.ts` naming) reusing the worker-scoped-fixture module above keeps `playwright.config.ts`'s default `workers: 1` behavior — and therefore every existing CI shard job — completely unchanged while the new mode is proven out.

## Open risks — not solved problems

[`research/pitfalls.md` §4](../../project_plans/e2e-test-parallelization/research/pitfalls.md) names three failure modes a future implementer must design against and validate empirically, not assume away. None of them crash the run outright — the common thread is that all three degrade into "flaky-looking" failures that get misattributed to "that spec is just flaky" rather than their real, shared root cause. They are preserved here close to verbatim because the "how it manifests if missed" framing is the actionable part:

**Port exhaustion / collision.** N servers each need a free dynamic port; a race between N near-simultaneous `findFreePort()` calls at worker-startup time (check-then-bind TOCTOU) is more likely at N>1 than at N=1. *How it manifests if missed*: not a crash — a worker's server fails to start, that worker's fixture setup times out, and the resulting test failures look like ordinary app-level flakiness (timeouts waiting for the app to respond) scattered across whatever specs happened to run on that worker, with no obvious common cause pointing back to port allocation. Hard to attribute without specifically logging the bound port per worker and cross-referencing failure clustering by worker index.

**Resource contention on one runner's fixed CPU/memory.** N full `stapler-squad` server processes plus N browser instances plus N tmux subprocesses all competing for the same fixed vCPU count on one GitHub-hosted runner (unlike today's job-level sharding, which gives each shard a dedicated VM) will slow every server down non-uniformly depending on scheduling luck. *How it manifests if missed*: not a crash — tests get slower and timing-sensitive assertions start failing intermittently under load, in a pattern that looks exactly like ordinary test flakiness and gets misdiagnosed as "this spec is flaky" rather than "this runner is oversubscribed," especially since the failure rate varies by which other specs happen to land on the same worker.

**Incomplete test isolation between per-worker servers.** The repo's own state-isolation design (`docs/reference/state-isolation.md`) documents multiple axes that must each be separately isolated — config dir, tmux socket, workspace/session state — and this repo's root `CLAUDE.md` explicitly warns that `STAPLER_SQUAD_INSTANCE` and `--test-mode`/`--test-dir` do **not** isolate tmux; only `STAPLER_SQUAD_TMUX_SOCKET` does. A from-scratch per-worker design that misses even one axis (e.g. isolates port and test-dir but reuses a derived-but-not-unique tmux socket name across workers) reproduces this exact class of bug between worker N and worker M. *How it manifests if missed*: not a crash — two workers' sessions interfere (one worker's leaked-session sweeper kills another worker's active tmux pane, or two workers' resolved testdirs collide under some edge-case env-var precedence), producing failures that look like each spec is independently flaky, aren't reproducible when that spec runs alone, and can only be attributed by correlating failure timing across workers.

Any implementation should budget for an explicit verification step beyond "the suite passed once" — e.g. deliberately running the full per-worker-server suite multiple times and checking whether failures cluster by worker index or by co-scheduled spec.

## Status

Not implemented this round — see [ADR-001 (Isolate via seam, defer N-server rearchitecture)](../../project_plans/e2e-test-parallelization/decisions/ADR-001-isolate-via-seam-defer-n-server-rearchitecture.md). This phase does not decide its own priority: [Story 3.3.1](../../project_plans/e2e-test-parallelization/implementation/plan.md) is the forcing function that compares Phase 3's real CI measurements against the project's Success Metric and records an explicit escalate-or-defer decision as a dated note appended to the top of this doc.
