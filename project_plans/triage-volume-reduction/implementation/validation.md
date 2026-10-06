# Validation: requirement -> test coverage
| AC | Test |
|---|---|
| AC1 | measurement.md (PR description) |
| AC2 | skip_test: AC present, PlanApproved, unchanged hash (respawn), changed hash runs, guidance resume bypasses, manual RPC runs, note written, queued item not demoted on skip |
| AC3 | concurrency test: cap=1 queues second run; config accessor clamp tests |
| AC4 | resolveTriageModel table test; call-option model asserted via fake headless caller |
Adversarial review: B1-B3 resolved in plan.md (see adversarial-review.md). Readiness: PASS.
