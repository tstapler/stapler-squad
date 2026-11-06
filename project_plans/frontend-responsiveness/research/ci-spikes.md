# CI feasibility spikes (Task 1.1.3e)

Status: partially answered locally; both spikes still need a CI-runner run. Nothing below is a CI result.

## Spike 1: seed the 20-session `perfFixtureShape` with real tmux

- LOCAL (macOS, load average 45-97, binary built from this branch): `seedPerfFixture` (`tests/e2e/perf/fixture.ts`) created all 20 `bash` sessions plus 200 backlog items through RPC against an isolated instance on a private tmux socket (`ssq-manual-1`), and `ListSessions` returned 20 (`Sessions (20)` in the failure screenshot of the first `load / 4x` run). So the RPC seeding path works; the cost is dominated by tmux session creation.
- UNVERIFIED in CI: no GitHub runner was available to this worker. Recommended first CI step: run `--project=perf -g "load / 1x"` with `PERF_RUNS=1` once and read `metadata.fixture.seedMs` from `test-results/perf-results.json`.
- Fallback already wired: `PERF_SESSIONS` / `PERF_BACKLOG` shrink the fixture; the pane count stays fixed (sessions are reused round-robin) and `metadata.fixture.actualSessions` / `actualBacklogItems` are written to `baseline.json`, so thresholds stay on one fixture across arms.
- Finding: each persisted window layout needs a `session-list` leaf or `useWindowManager.repairWindow` resets the window to the default split (`web-app/src/lib/window/useWindowManager.ts:53-58`); the fixture adds one list pane per window beside the 8 / 4 / 4 / 4 session panes.
- Finding: `tests/e2e/global-setup.ts` waits 90 s for server start (`helpers/test-server.ts:184`); under load average > 45 the binary needed ~80 s and one attempt timed out. The live runs here used a hand-started instance with a throwaway config that nulls `globalSetup`.

## Spike 2: `page.clock` against the Next and Bun routers (embedded production binary)

- NOT RUN. The machine was too loaded to get stable timer behaviour, and the Bun-router build was not built. Per plan the fallback stands: UX timing specs use a Jest fake-timer test plus a Playwright check with real elapsed time bounded by `expect.poll`.
