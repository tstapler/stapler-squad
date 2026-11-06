# Pre-mortem: hook-classification-fast-path
**Date**: 2026-09-23

## Failure Modes

| # | Failure | First Symptom | Prevention | Severity |
|---|---|---|---|---|
| 1 | Extracting `HookClassifier` subtly changes standalone allow/deny/defer semantics, making the fast path quick but wrong. | Differential tests show a decision, reason, rule ID, or stdout-format mismatch; correctness counters rise after cutover. | **Mitigated before implementation:** the Risk Control section now makes a sanitized golden-corpus differential test a release-blocking gate; validation pins the corpus test. | P1 |
| 2 | Endpoint derivation or injected metadata routes a named/manual/test hook to the production listener. | A fingerprint mismatch is rejected, or any response fingerprint differs from the caller's expected identity. | **Mitigated before implementation:** the Risk Control section now requires an endpoint ownership self-probe before cutover; protocol boundary rejection and a production/manual-a/manual-b same-cwd load test are release gates. | P1 |
| 3 | The real short-lived CLI still misses the latency SLO even though in-process socket benchmarks pass. | Socket benchmark is fast while real `ssq-hooks check` subprocess p99 exceeds 50 ms. | Benchmark both the socket boundary and spawned real CLI, profile process/JSON/context costs, and block replacement unless both pass. | P2 |
| 4 | Analytics queue saturation silently loses more data than the accepted bounded window during SQLite contention or shutdown. | Queue depth/oldest age and drop counters climb; shutdown reports unflushed events. | Use non-blocking bounded enqueue, size/deadline batches, explicit drop accounting, shutdown deadline, alerts, and a locked-WAL saturation test. | P2 |
| 5 | Direct installer replacement leaves duplicate hooks, partial settings, or an incompatible client/server pair. | Invocation count remains doubled, settings fail JSON parsing, or fallback exceeds 1% immediately after install. | Atomic normalize-with-backup transaction, compatibility check before mutation, interruption fault injection, exact rollback command, and duplicate-count verification. | P2 |

## P1 Items (address before implementation)

- [x] Failure #1 — semantic compatibility is now an explicit release-blocking plan gate and differential validation case.
- [x] Failure #2 — endpoint ownership self-probe and cross-instance isolation are now explicit release-blocking plan gates.

## Severity Summary

- **P1**: 2, both mitigated/closed in the plan before implementation.
- **P2**: 3, each has prevention and validation coverage.
- **P3**: 0.

The top failure mode is a fast classifier whose extracted semantics no longer match the current standalone hook, because it could silently make incorrect permission decisions.
