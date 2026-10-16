# Validation

| AC | Coverage |
|---|---|
| 1 | accumulator unit test w/ fixture, duplicate ids |
| 2,3 | pool test with fake runner streaming usage lines past ceiling → ErrCostCeilingExceeded, subprocess killed |
| 4 | config default + CallOptions passthrough test in services |
| 5 | classifyHeadlessCallError table test |
| 6 | existing BUG-055 tests (session/backlog_lifecycle_stuck_test.go) |
| 7 | unpriced-model + disabled cases |

Pre-mortem: biggest failure is a wrong usage-shape assumption (task 1 gates everything) and a too-low default aborting valid runs.
