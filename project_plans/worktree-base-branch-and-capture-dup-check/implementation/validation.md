# Validation Plan: worktree-base-branch-and-capture-dup-check

**Date**: 2026-09-24

## Happy Path Scenario

Given the pre-existing-fix finding that PR #833/#837 already shipped the `new_worktree` base-branch resolution and terminal-output-capture fix to `main`, when this triage's verification tasks (Story 1.1.1's test re-runs plus the live MCP repro) run, then AC1-AC4 are all confirmed still true and AC5's duplicate-closure call routes the item toward closure — either directly via `report_duplicate` or, for a triage-role session, via a `submit_triage_result` hand-off — with no re-implementation needed.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: `new_worktree` branches from the resolved default branch's tip, not ambient HEAD | `session/git/ops_test.go`; `session/instance_worktree_test.go` | `TestResolveWorktreeBaseCommit_UsesOriginDefaultBranch` (Task 1.1.1a); `TestSetupFirstTimeWorktree_NewWorktree_BranchesFromOriginDefault_NotAmbientHEAD` (Task 1.1.1b) | Existing Unit + Live MCP Repro (Task 1.1.1d, if live tool access is available) | Given a source repo checked out on a branch diverged from `origin/main`, when a new worktree is set up via `newWorktreeFromResolvedBase`/`ResolveWorktreeBaseCommit`, then the new worktree's HEAD matches `origin/main`'s resolved tip, not the diverged ambient branch; the live repro additionally confirms this via a real `create_session(session_type=new_worktree)` call and `git log` comparison. |
| AC2: Ambient-HEAD divergence from the resolved base is surfaced to the caller, not silent | `session/git/ops_test.go`; `session/instance_worktree_test.go` | `TestAmbientHEADDivergesFromBase_True_When_CheckedOutBranchDiffersFromBase` (Task 1.1.1a); `TestSetupFirstTimeWorktree_NewWorktree_NoWarning_When_AmbientHEADMatchesBase` (Task 1.1.1b) | Existing Unit | Given the same diverged-repo scenario as AC1, when the worktree is created, then `AmbientHEADDivergesFromBase` returns `true` and `Instance.CreationWarning`/`SessionDetail.creation_warning` is populated (the companion "no warning when HEAD matches base" test confirms the negative case doesn't false-positive). |
| AC3: `run_command`/`read_session_output` return real output for a trivial command on a ready session | `server/mcp/tools_terminal_test.go` | `TestReadOutputSucceeds_When_ReadyWithNoNewBytes`; `TestReadOutputLineCap` (Task 1.1.1c) | Existing Unit + Live MCP Repro (Task 1.1.1d, if live tool access is available) | Given a ready session whose scrollback has advanced, when `read_session_output` is called after `run_command echo alive-check`, then the response contains real output because `forwardOneControlModeFrame`/`forwardCapturePaneOutput` call `ScrollbackManager.AppendOutput` on every live frame; the live repro additionally confirms the echoed `alive-check` line comes back through a real MCP round-trip, not just a test double. |
| AC4: A not-yet-ready session is distinguishable from one that ran and produced no output | `server/mcp/tools_terminal_test.go` | `TestReadOutputSessionNotReady`; `TestSessionNotReadyResult` (Task 1.1.1c) | Existing Unit | Given a session whose scrollback sequence hasn't advanced since creation, when `read_session_output` is called, then the response is the `SESSION_NOT_READY` error code from `sessionNotReadyResult`, not an empty-but-"successful" result. |
| AC5: Backlog item `4d856751-...` is confirmed a duplicate of `c7466f05-...` and routed to closure, not re-implementation | N/A — no code test; MCP tool call only | `report_duplicate` (direct or dispatched to `reportDuplicateUnclaimed`) or `submit_triage_result` naming the duplicate, per the role/link branch determined in Task 1.1.2a, followed by a `get_backlog_item` read-back (Task 1.1.2c) | Process Verification | Given this session's link/role to item `4d856751-...` as determined by `get_backlog_item`, when the routed call (`report_duplicate` for a linked work-role or unclaimed item, `submit_triage_result` for a linked non-work/triage-role session) is made citing PR #837, then the item's state change (status transition to `review`/`archived`, or the triage summary/suggestions) is confirmed by re-reading the item — not trusted from the tool's response text alone. |

## UX Acceptance Tests

N/A — no user-facing surface; MCP tool calls only.

## Test Stack

- **Existing Go tests**: go test + testify (per this repo's `golang-testing`/`golang-stretchr-testify` conventions), run via `go test` directly for the targeted `-run` filters this plan specifies (not `make test`/gotestsum, since these are single targeted re-runs, not the full suite)
- **Live MCP repro**: this session's own MCP tool calls (`create_session`, `run_command`, `read_session_output`), per plan.md Task 1.1.1d
- **Process verification (AC5)**: MCP tool call (`report_duplicate` / `reportDuplicateUnclaimed` / `submit_triage_result` per plan.md Story 1.1.2) + read-back via `get_backlog_item`, not a code test

## Coverage Targets and How to Measure

No new production code is written by this plan, so line/branch coverage targets don't apply in the usual sense — there is no diff for `make test-coverage` to measure against. The relevant "coverage" metric here is requirements coverage: all 5 ACs each map to a concrete, already-existing verification step above (5/5). The repo-wide coverage gates (`make test-coverage`, the `dupl`/`jscpd` duplication gates in `make ready`) are unaffected, since this item's scope is re-verification and a backlog-state mutation, not a source change. The two coverage *gaps* this triage did surface — `runCommand`/`AppendOutput` real-call-site test coverage, and the hardcoded-default-branch-candidates edge case — are intentionally deferred, per plan.md's Story 1.1.3, to a future backlog item rather than fixed here.
