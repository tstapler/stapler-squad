# Validation Plan: worktree-selfheal-test-flake

**Date**: 2026-08-22

## Happy Path Scenario

Given two concurrent `setupNewWorktree()` callers racing to create the same backlog branch
under real subprocess-scheduling load, when one loses the branch-create race and its
`git worktree add -b` (or the follow-on `git worktree add`) fails with *any* error text —
not just the two literal strings recognized today — then the loser re-verifies actual git
state (Ground-Truth Re-Query) and self-heals into the winner's branch/worktree instead of
hard-failing, so both callers return no error.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC-4, AC-5 (setupNewWorktree happy path) | `session/git/worktree_ops_test.go` | `TestSetupNewWorktree_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` (plan Task 2.2.1a) | Unit | `gitSpyCommandRunner.runFunc` creates the branch for real at the moment the `worktree add -b` "subprocess" would run, then returns `errors.New("signal: killed")` — an error string matching neither of the old literals. `setupNewWorktree()` must still return no error, proving the self-heal decision no longer depends on `err.Error()` content. |
| AC-4 (setupNewWorktree error path — new, not yet named in plan.md) | `session/git/worktree_ops_test.go` | `TestSetupNewWorktree_HardFails_When_WorktreeAddErrorsAndBranchStillDoesNotExist` | Unit | `gitSpyCommandRunner.runFunc` returns `errors.New("signal: killed")` for `worktree add -b` **without** creating the branch. After `worktreeAddRetryAttempts` retries of `branchRefExists` all observe `false`, `setupNewWorktree()` must return the original hard error (`"failed to create worktree from commit %s: %w"`), proving Ground-Truth Re-Query does not spuriously mask a genuine failure (disk full, permissions) — the negative case Story 1.1.1's AC requires and the one a "when in doubt, self-heal" implementation bug would silently fail. |
| AC-4, AC-5 (setupFromExistingBranch happy path) | `session/git/worktree_ops_test.go` | `TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` (plan Task 2.2.2a) | Unit | `gitSpyCommandRunner.runFunc` returns `errors.New("signal: killed")` for the `worktree add <path> <branch>` call (no `-b`) and a synthesized `worktree list --porcelain` response reporting the branch checked out at a distinct simulated winner path. `setupFromExistingBranch()` must return no error and adopt the winner's path into `g.worktreePath`. |
| AC-4 (setupFromExistingBranch error path — new, not yet named in plan.md) | `session/git/worktree_ops_test.go` | `TestSetupFromExistingBranch_HardFails_When_WorktreeAddErrorsAndBranchNotFoundAnywhere` | Unit | `gitSpyCommandRunner.runFunc` returns `errors.New("signal: killed")` for `worktree add <path> <branch>`, and the scripted `worktree list --porcelain` response does **not** contain the branch anywhere. `setupFromExistingBranch()` must return the original hard error (`"failed to create worktree from branch %s: %w"`), unchanged from today — proving the unconditional re-query still correctly hard-fails when the branch genuinely isn't registered to any worktree. |
| AC-1 (local reproduction, pre-fix baseline) | shell / `session/git` package (no source change) | Task 2.1.1a stress-repro command: `go test -race -c -o /tmp/worktree_ops.test ./session/git && timeout 90s stress -p $(nproc) /tmp/worktree_ops.test -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate'` | Integration (stress harness, not a `go test` unit test) | Run against pre-fix HEAD; record whether the flake reproduces locally within the 90s budget or exits clean (124), closing AC-1 with an executed command and observed outcome rather than static analysis alone. |
| AC-1 (post-fix confirmation) | shell / `session/git` package (no source change) | Task 2.1.2a — identical stress-repro command re-run against post-fix HEAD | Integration (stress harness) | Same command, run after Phase 1 lands. A clean 90s run is the direct before/after signal that Ground-Truth Re-Query closed the flake (or, if Task 2.1.1a didn't reproduce it either, a consistent-with-fix result — recorded as such, not overclaimed as proof). |
| AC-1, AC-4, AC-5 (real concurrent race, existing coverage retained) | `session/git/worktree_ops_test.go` | `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` (existing, line 338 — kept as-is per plan's Pattern Decisions table) | Integration | Two goroutines call the real unlocked `setupNewWorktree()` against real git subprocesses for the identical branch name. After the fix, this is the only test exercising the *actual* nondeterministic race end-to-end (real subprocess timing, real git error text) rather than a scripted error string — the deterministic spy-based unit tests above pin the specific failure mode this test can only probabilistically hit. |
| AC-3 (production-reachable gap resolved: unlocked race is test-only) | `session/git/worktree_ops_test.go` | `TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo` (existing, line 383 — no change required) | Integration | Already proves the public, lock-wrapped `Setup()` serializes concurrent same-repo worktree creation via `WithRepoWorktreeLock`, so the exact two-unlocked-goroutines race the flaky test constructs cannot occur through any real caller. AC-3's resolution is primarily documentation (Task 1.1.3's doc comment on `setupNewWorktree`/`setupFromExistingBranch`), not a new test; this existing test is the executable evidence backing that doc comment's claim. |
| AC-2(a): test's own timing assumptions | N/A — ruled out by research, no test needed | — | — | `research/pitfalls.md` and `research/features.md` establish the observed failure mode is an unrecognized error string (`"signal: killed"`) reaching the fallback's string match, not a too-short retry/backoff window in the test itself — the test has no retry/backoff of its own to tighten. Documented in Task 1.1.3's doc comment, not re-verified by a dedicated test. |
| AC-2(b): unrecognized git error string reaching fallback | covered above | `TestSetupNewWorktree_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError`, `TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` | Unit | Confirmed root cause — both tests directly reproduce a `"signal: killed"` error (verified in `research/features.md` as git's real message on subprocess timeout-kill) reaching the fallback and prove the pre-fix code would have failed it (matches neither of the two old literals) while the post-fix code self-heals. |
| AC-2(c): fallback logic gap independent of load | covered above | `TestSetupFromExistingBranch_HardFails_When_WorktreeAddErrorsAndBranchNotFoundAnywhere` plus `research/features.md` §2 | Unit + research | `research/features.md` notes the pre-fix two-literal OR-gate at `worktree_ops.go:136` under-covers a real self-healable case (a stale-directory `"already exists"` message reaching this call site matches neither literal) independent of any timing/load — a real, load-independent gap. The unconditional Ground-Truth Re-Query closes it; the error-path unit test above confirms the fix doesn't overcorrect into masking genuine failures. |

## UX Acceptance Tests

N/A — pure infrastructure fix, no user-facing surface (no `design/ux.md` for this project).

## Test Stack

- **Unit**: Go stdlib `testing` + `testify` (`assert`/`require`) — matches `session/git`'s
  existing test style (see `worktree_ops_test.go`, `worktree_git_test.go`).
- **Integration**: `gitSpyCommandRunner` test double (existing seam, `worktree_git_test.go:730-761`)
  for deterministic error-injection at the exact subprocess-call boundary; real concurrent
  goroutines against a real `setupTestRepo(t)` git repo (existing pattern in
  `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` and
  `TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo`) for
  the real-race coverage that a scripted spy can't substitute for.
- **Stress/repro harness**: `golang.org/x/tools/cmd/stress@v0.47.0` (go-stress), the same
  CI-adopted tool as `build.yml:400-438`, for AC-1's local reproduction attempts — not a
  `go test` unit test, a documented shell procedure with a recorded pass/fail outcome.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/git/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of the new/changed branches in `setupNewWorktree` and `setupFromExistingBranch` (both the self-heal-success and still-hard-fails paths) exercised by the four unit tests above |
| Go (race + flake confirmation) | `make quick-check` (build + test + lint), plus the go-stress commands in Task 2.1.1a/2.1.2a | All pass; stress repro records an explicit before/after outcome for AC-1 |

- All public/exported service methods: N/A here — `setupNewWorktree`/`setupFromExistingBranch`
  are unexported; their behavior is reached through the existing exported `Setup()`/`SetupLocked()`
  tests plus the four new/named unit tests calling them directly (same pattern as the existing
  flaky test, which already calls `setupNewWorktree()` unlocked for the same reason).
- All external integrations (real `git` subprocess calls): unit-mocked via `gitSpyCommandRunner`
  for the four deterministic tests above, **and** at least one real-subprocess integration test
  per layer (`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` for layer 1,
  `TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo` for the
  locked/production path) — both already exist and require no changes.

## AC-1: Local Reproduction Results (pre-fix, 2026-08-24)

**Isolated single-test stress** (Task 2.1.1a's exact command,
`go test -race -c -o /tmp/worktree_ops.test ./session/git && timeout 90s stress -p $(nproc)
/tmp/worktree_ops.test -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate'`):
**did not reproduce** — 1897 runs, 0 failures, clean exit 124 (full 90s budget, 24 concurrent
workers on this machine's `nproc`).

**Whole-package stress** (adversarial-review's non-blocking recommendation, running the full
`session/git` package binary instead of a single `-test.run` filter, to better approximate the
"full-suite CI load" the requirements doc cites as the actual trigger): attempted, but the
result is **inconclusive**, not a clean pass or a reproduction of the target flake. The stress
binary was invoked from an unrelated cwd (`/tmp`), and `TestScaffoldingExcludePatterns_MatchGitignore`
(`session/git/scaffolding_test.go:31`) fails deterministically outside `session/git/`'s own
working directory (`open ../../.gitignore: no such file or directory` — a relative-path
dependency on `go test`'s auto-cd into the package directory, unrelated to worktree self-heal).
This alone drove `stress`'s 100%-failure signal (verified via a single non-`stress` run of the
same binary, exit 1, exactly one `--- FAIL:`, naming that test). No `--- FAIL` for
`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` (or any other test)
appeared in either the single run or the ~410-iteration stress log, and no `panic:`, `FATAL`, or
`signal:` text appears anywhere in the captured output — i.e. nothing in this run's evidence
points at the self-heal fallback itself. Per the user's explicit direction mid-session, this was
not further chased (e.g. by re-running with `cwd` fixed) given the isolated run already
documents an executed-and-recorded AC-1 attempt either way, and the retry-budget sizing below is
based on the root-cause hypothesis (`runGitCommand`'s 30s timeout, confirmed independently in
`research/features.md`) rather than empirical contended-completion timing this attempt didn't
capture.

**Retry-budget sizing consequence** (pre-mortem Failure #1): since neither repro captured real
contended `git worktree add` completion timing, `worktreeAddRetryAttempts`/
`worktreeAddRetryDelay` are sized as a materially larger bound than the unrelated
`headSHARetryAttempts`/`headSHARetryDelay` precedent (60ms total) — 6 attempts × 300ms delay
(1.5s total, with the loop able to observe the winner completing well before `runGitCommand`'s
30s ceiling in the common case) — per plan.md Task 1.1.1a's fallback guidance for this exact
scenario, rather than left at the unexamined 60ms default.

**Post-fix confirmation (Story 2.1.2, 2026-08-24)**: identical isolated stress command re-run
against post-fix HEAD (Ground-Truth Re-Query in place) — 1559 runs, 0 failures, clean exit 124
for the full 90s budget. Since the pre-fix isolated run was already clean, this is a
consistent-with-fix result, not a direct before/after flip (per the framing this doc's earlier
note commits to). The six new deterministic regression tests
(`TestSetupNewWorktree_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError`,
`TestSetupNewWorktree_SelfHeals_When_BranchCreatedByDelayedRaceWinner`,
`TestSetupNewWorktree_HardFails_When_WorktreeAddErrorsAndBranchStillDoesNotExist`,
`TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError`,
`TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner`,
`TestSetupFromExistingBranch_HardFails_When_WorktreeAddErrorsAndBranchNotFoundAnywhere`) are the
mechanism that actually pins the specific failure mode (an unrecognized error string reaching
either self-heal layer) — the stress harness alone, isolated or not, can only probabilistically
hit that mode and was never expected to give a hard guarantee either way.
