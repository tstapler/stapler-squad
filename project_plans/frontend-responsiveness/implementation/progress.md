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
| 2.1.0 SessionDetailView seam | done | 0941d4052 | |
| 2.1.4a flagStore | done | bbc53c8d5 | |
| 2.1.5a programs cache | done | 83c7e3b00 | |
| 5.3.2a reader queue watermark | done | 20ea716f5 | |
| 2.1.4b unify flag fetchers | done | babf93c51 | 14 -> 7 (1 per doc) until cross-document coalescing |
| 2.1.1a-d migrate transport callers | done | 4e3a2bd2d, 2d7ea52eb, 96c0a2351, 7f04f4677 | |
| 2.1.3 log batching | done | 62c79ca35 | |
| 2.1.1e + 7.1.1a ESLint ban | done | d1c04d0bf | |
| 2.1.2a/b coalescing | done | b2b77d4f5, e99bb4ee1 | |
| 2.1.6a-d per-pane dedupe | done (programs-fallback abort moved to 2.1.9 worker) | 65a581b1c..3f938e5db | |

## Deviations
- Tag `perf-baseline-code` marks the pre-Epic-2 code state (HEAD at 7478e4944+docs). baseline.json could not be captured (machine load 45-97); owner/quiet machine must run it against that tag, then tag `perf-baseline`.

## Waiting on owner
(none yet)
- HAR re-capture with DevTools closed (1.1.1b/1.1.4b); confirm baseline was 7 documents.
- baseline.json: quiet machine, >=5 kept runs: `PERF_WRITE_BASELINE=1 PW_PERF=1 bunx playwright test --project=perf` against checkout of `perf-baseline-code`; commit and tag `perf-baseline`.
- 1.1.3: route-change scenario debug; CI seeding + page.clock spikes on a runner.

## Phase 5 end state (2026-10-26)

Done (code + tests, local commits, not pushed): 1.1.1a/b(code part), 1.1.2, 1.1.3 (harness), 1.1.6, 2.1.0-2.1.6, 2.1.9, 3.1.1, 3.1.2, 4.1.1-4.1.5, 5.1.1, 5.2.1 (a-g, plus /actuator/ws-unary e6d145059), 5.3.1, 5.3.2, 5.4.1a/b, 7.1.1-7.1.2, 7.1.3a. Gap-repair commits: cef5c6c26, a8e1cbc8f, 7712e411b, 33ae80ffb, bdadcaa6a, 04c12edf3, 355142824, e6d145059.

Blocked (owner / quiet machine / gate): 1.1.3 baseline.json + `perf-baseline` tag; 1.1.4 slot holders; 1.1.5 backlog attribution; 2.1.7 numeric exit; 2.1.8 (conditional on 1.1.4); 5.1.2 decision gate; 5.4.1c; 5.5.1/5.5.2 three-arm; 4.1.6 default flip; 6.1.1-6.1.3 (need 1.1.5 note); 7.1.3b blocking flip; 7.1.4 final recapture; headed VoiceOver pass; HAR recapture with DevTools closed.
Known unmet tests: statusText contrast / newControls keyboard-reachable (UI from Epic 6); keystroke +10% echo e2e; page.clock spike unrun in CI.
