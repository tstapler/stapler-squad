# Scoping ledger (Epic 13, ADR-010)

Measured with `tools/bzl/scoping` (`bazel run //tools/bzl/scoping -- t9`), baseline `tools/bzl/scoping/baseline.json`
(tree 145bce76b, history `33f46bb21`, 378 Go-touching changes, 17 PRs). Lib share = mean affected library-LOC share.

| Step | Decision | T9a test share | T9c lib share | Changes with lower lib LOC | `server/services` / `session` LOC | Inversions |
|---|---|---|---|---|---|---|
| baseline | | 0.5536 | 0.3945 | 0 | 93,019 / 92,060 | 12 |
| Tier 0 (inttest moves + `session/internal/peeraddr`) | executed | 0.4464 (predicted 0.449) | 0.3945 | 0 | 93,019 / 92,053 | 0 |
| Tier A `sessionbundle` + `devtools` | executed (passes SC6 via "fewer") | 0.4461 | 0.3940 | 12.7% (predicted 11.6%) | 91,697 / 92,053 | 0 |
| Tier A `session/external` | skipped by criterion SC6 | 0.4460 (-0.01 pp) | 0.3936 (-0.04 pp, needs 0.10) | unchanged | 91,703 / 90,844 | 0 |
| Tier A `server/services/wsbridge` | skipped by criterion SC6 | 0.4459 (-0.02 pp, needs 0.30) | 0.3935 (-0.05 pp, needs 0.10) | unchanged | 88,382 / 92,053 | 0 |
| Tier B `server/services` p0 (`BacklogService` group) | not executed: SC1 | | | | | |

Notes:
- `session/external` and `wsbridge` compile and pass their tests when moved with `scoping move` (one boundary field
  setter, `Instance.SetProcessManager`, was needed for `external`; `wsbridge` needed a local metric-reader helper and
  fixture path and data fixes). Both were reverted because the measured payoff is below SC6, not because of a defect.
- Tier B `server/services` p0 with the plan's file globs (66 files with tests): 15 distinct non-test declarations used
  by the staying core (`SessionStopper`, `AutonomousDriverStarter`, `SessionSteerer`, `WithTransitionCaller`,
  `redactErrorDetail`, `fromProtoStuckReason`, `crossHostClaimDedupFlagName`, ...) and 33 straddling test files,
  against an SC1 budget of 10. A relocation commit plus test-helper extraction must come first.
- Tier B `session` p0/p1/p2: not attempted (same shape, 70/60/10 declarations in the research).
- The research prediction for the marginal effect of `wsbridge` (-0.148 pp lib, -0.31 pp test) and `external`
  (-0.18 pp lib) is not reproduced by the Go replay; the Go tool follows renames through `git diff -M` against the
  baseline commit, the research tool mapped moved files to the package's core node.
