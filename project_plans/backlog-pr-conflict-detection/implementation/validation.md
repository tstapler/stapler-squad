# Validation Plan: backlog-pr-conflict-detection

**Date**: 2026-07-12

## Happy Path Scenario

Given a `pr_pending` backlog item whose PR has developed a real merge conflict against its base branch (GitHub reports `mergeStateStatus == "DIRTY"` or `mergeable == "CONFLICTING"`) while CI is still green and no reviewer has requested changes, when `ReconcilePRPending` polls the item and calls `GetPRStatus`, then `PRStatus.HasConflicts` is `true` and the reconciler spawns a fix session via `AutoReopenForPRFix` — passing `fixCtx` containing the `## Merge conflict` section with rebase/force-with-lease guidance — the same autonomous path already used for CI failures and blocking reviews, bounded by the existing `maxAutoReworkIterations` cap.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Epic 1.1 (Tasks 1.1.1a–1.1.1d): `HasConflicts` computed via `mss == "DIRTY" \|\| mg == "CONFLICTING"` OR condition | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_ConflictDetection` (Task 1.3.1a) | Unit | Happy path — conflict-true subtests: `CONFLICTING`/`DIRTY`, `CONFLICTING`/`BLOCKED`, and the stale-`mergeable` case `MERGEABLE`/`DIRTY` (cli/cli#9583) |
| Epic 1.1 (Tasks 1.1.1a–1.1.1d): near-miss states must NOT be treated as conflicts | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_ConflictDetection` (Task 1.3.1a) | Unit | Error/edge path — conflict-false subtests: `MERGEABLE`/`CLEAN`, transient `UNKNOWN`/`UNKNOWN`, `MERGEABLE`/`BLOCKED` (review/check gate, not a conflict), `MERGEABLE`/`BEHIND` |
| Epic 1.2 (Task 1.2.1a): conflict-specific fix guidance (`--force-with-lease`, `.gitignore` suspicion, leave-markers-and-stop) rendered into `FeedbackText` | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_ConflictGuidanceText` (Task 1.3.1b) | Unit | Happy path — all 3 guidance substrings present for both `CONFLICTING`/`DIRTY` and the stale-`mergeable` `MERGEABLE`/`DIRTY` case, proving guidance is keyed off `status.conflict != nil`, not the specific field that tripped |
| Epic 1.3 Story 1.3.2: `CIFailing` regression coverage (pre-existing, previously untested) | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_CIFailing` (Task 1.3.2a) | Unit | Happy path — terminal `FAILURE` conclusion sets `CIFailing=true`, `FeedbackText` contains `"## Failing CI checks"` + check name |
| Epic 1.3 Story 1.3.2: `CIFailing` regression coverage (pre-existing, previously untested) | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_CIFailing` (Task 1.3.2a) | Unit | Error/negative path — non-terminal `IN_PROGRESS` leaves `CIFailing=false`, no `## Failing CI checks` section |
| Epic 1.3 Story 1.3.2: `HasBlockingReviews` regression coverage (pre-existing, previously untested) | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_HasBlockingReviews` (Task 1.3.2b) | Unit | Happy path — `CHANGES_REQUESTED` review sets `HasBlockingReviews=true`, `FeedbackText` contains author + body |
| Epic 1.3 Story 1.3.2: `HasBlockingReviews` regression coverage (pre-existing, previously untested) | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_HasBlockingReviews` (Task 1.3.2b) | Unit | Error/negative path — `APPROVED`-only reviews leave `HasBlockingReviews=false`, no `## Review:` section |
| Epic 1.3 Story 1.3.3: `FeedbackText` section ordering — conflict rendered before CI/review sections via `render()`'s fixed order | `session/git/worktree_git_test.go` | `TestParsePRStatusPayload_ConflictSectionOrderedFirst` (Task 1.3.3a) | Unit | Happy path — combined `CIFailing=true` + `HasConflicts=true` payload asserts `strings.Index("## Merge conflict") < strings.Index("## Failing CI checks")` |
| Epic 2.1 Story 2.1.1: `prPendingChecker` testability seam + test doubles enabling gate tests | `session/backlog_lifecycle_test.go` | `fakePRPendingChecker` / `fakePRFixSpawner` test doubles (Task 2.2.1a) | Integration (fixture) | Setup — doubles satisfy `prPendingChecker`/`PRFixSpawner`, letting the gate be exercised without a live, authenticated `gh` CLI or real GitHub state |
| Epic 2.1/2.2: 3-way gate — `HasConflicts` alone (CI/reviews both false) is sufficient to spawn a fix session | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_SpawnsFixSession_WhenHasConflictsTrue_Alone` (Task 2.2.2a) | Integration | Happy path — `fakePRPendingChecker` returns `HasConflicts=true` only; `fakeSpawner.spawnCalled==true` and `lastFixContext` contains `"## Merge conflict"` |
| Observability Requirement: log line records which signal(s) triggered the spawn (`conflict=%v`) | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_LogsConflictTrue_WhenConflictTriggersSpawn` (Task 2.2.2b) | Integration | Happy path — with `log.InfoLog` redirected to a `log.NewDummyLogger`, captured output contains `"conflict=true"` |
| Epic 2.2 Story 2.2.3: `CIFailing`-triggered spawn regression coverage (pre-existing pathway, previously untested) + log correctly reports `conflict=false` | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_SpawnsFixSession_WhenCIFailingTrue` (Task 2.2.3a) | Integration | Happy path for the CI trigger / negative-path assertion for the conflict signal — spawn occurs, log contains `"CI=true"` and `"conflict=false"` |
| Epic 2.2 Story 2.2.3: `HasBlockingReviews`-triggered spawn regression coverage (pre-existing pathway, previously untested) + log correctly reports `conflict=false` | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_SpawnsFixSession_WhenHasBlockingReviewsTrue` (Task 2.2.3b) | Integration | Happy path for the review trigger / negative-path assertion for the conflict signal — spawn occurs, log contains `"reviews=true"` and `"conflict=false"` |
| Epic 2.2 Story 2.2.4: gate stays closed when all three signals are false (no over-triggering regression) | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_NoSpawn_WhenAllSignalsFalse` (Task 2.2.4a) | Integration | Error/negative path — `PRStatus{}` (all false, matching live-verified PR #151 data); `fakeSpawner.spawnCalled==false`, item status unchanged |

## UX Acceptance Tests

N/A — backend-only feature, no `design/ux.md`. This project has no UI surface; `PRStatusPoller`/`WorktreePRPoller` UI-badge behavior is explicitly Out of Scope (requirements.md) and untouched.

## Test Stack

- **Unit**: Go `testing` package, table-driven tests (`t.Run` subtests) against the pure function `parsePRStatusPayload(raw []byte) (*PRStatus, error)` — no live `gh` CLI or network dependency, per the architecture decision in plan.md (Pattern Decisions: "Mergeability interpretation").
- **Integration**: Go `testing` package with fake test doubles (`fakePRPendingChecker` satisfying the consumer-defined `prPendingChecker` seam, `fakePRFixSpawner` satisfying `PRFixSpawner`) driving the real `ReconcilePRPending` method against a real `storage.CreateBacklogItem`-created `pr_pending` item — exercises the actual gate/spawn/log code path without a live git worktree or authenticated GitHub session.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./session/git/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% branch coverage on the new `HasConflicts` OR condition and the extended 3-way gate in `ReconcilePRPending` |

- All public/consumer-facing methods touched by this project (`GetPRStatus`, `parsePRStatusPayload`, `render`, `ReconcilePRPending`) have both happy-path and error/negative-path coverage.
- The one external integration in scope (`gh pr view` via `GetPRStatus`) is not directly unit-tested — by design, per plan.md's Pattern Decisions ("`GetPRStatus` requires a live, authenticated `gh` CLI... therefore untestable in CI without network access"). Coverage is achieved by testing the extracted pure function (`parsePRStatusPayload`) that contains all JSON-parsing/evaluation logic; `GetPRStatus` itself is reduced to a thin I/O wrapper with no branching logic left to test.
- `ReconcilePRPending`'s dependency on git/gh (`prPendingChecker`) is unit/integration-tested via the `newPRPendingChecker` seam and fakes — no live git worktree or GitHub session required, per Epic 2.1's explicit testability-seam rationale.
- Zero-regression requirement (requirements.md Success Metrics) is satisfied by Tasks 1.3.2a/1.3.2b (parsing-level regression for `CIFailing`/`HasBlockingReviews`, first-ever coverage) and Tasks 2.2.3a/2.2.3b (gate-level regression for the same two signals, also first-ever coverage) — both existing paths are now tested for the first time as part of this project, per requirements.md's explicit "hardening the same reconciliation loop being extended" scope note.
