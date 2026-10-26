# Phase 5 progress log: frontend-responsiveness

Started 2026-10-26. Coordinator: top-level Phase 5 thread. Workers: fresh general-purpose agents, one story/wave at a time.

| Story/Task | Status | Commit | Notes |
|---|---|---|---|
| (owner session) notification parse perf | done | 078c165b9 | pre-existing owner edits committed first |
| 1.1.6 flags registered | done | 5c740f8ae | |
| 3.1.2c Go zero-size resize guards | done | 8b2198d63 | |
| 1.1.1a traceCall + 1.1.2 gauge/marks | done | 20b35acc6, e12563d88 | window-switch end mark = 2 rAF, does not yet wait for terminal fit/focus |
| 1.1.1b GetFeatureFlags trace | done | see git log (docs(perf): trace GetFeatureFlags) | 14 = 7 documents x 2 uncoupled consumers |
| 1.1.3 perf spec | partial | 7478e4944 | a,b,d done; c/e partial; baseline.json NOT captured |

## Deviations
- Tag `perf-baseline-code` marks the pre-Epic-2 code state (HEAD at 7478e4944+docs). baseline.json could not be captured (machine load 45-97); owner/quiet machine must run it against that tag, then tag `perf-baseline`.

## Waiting on owner
(none yet)
- HAR re-capture with DevTools closed (1.1.1b/1.1.4b); confirm baseline was 7 documents.
- baseline.json: quiet machine, >=5 kept runs: `PERF_WRITE_BASELINE=1 PW_PERF=1 bunx playwright test --project=perf` against checkout of `perf-baseline-code`; commit and tag `perf-baseline`.
- 1.1.3: route-change scenario debug; CI seeding + page.clock spikes on a runner.
