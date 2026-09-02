# Requirements: worktree-selfheal-test-flake

## Source

Backlog item `a60ce219-38ac-43c8-81bb-5e5e69704865`: "Flaky test:
TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate". Non-interactive
triage — this doc is derived directly from the item's title/description, not an interview.
Entry point: `sdd` pipeline in triage mode (research → plan → validate only, no
implementation in this session).

## Problem Statement

`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`
([`session/git/worktree_ops_test.go:338`](https://github.com/tstapler/stapler-squad/blob/master/session/git/worktree_ops_test.go#L338))
intermittently failed under full-suite CI load on PR #583's run (2026-08-22, run
32547378297), a PR unrelated to worktree code. The assertion that fired —
`"first concurrent setup must not hard-fail on a lost branch-create race"` — means the
self-heal fallback the test exists to verify (`setupNewWorktree` → `setupFromExistingBranch`
on a lost `git worktree add -b` race, [`worktree_ops.go:329-341`](https://github.com/tstapler/stapler-squad/blob/master/session/git/worktree_ops.go#L329-L341))
itself returned a hard error under real parallel CI load, not just under the test's own
simulated race.

The test calls the **unlocked** `setupNewWorktree()` directly (not the lock-wrapped public
`Setup()`) specifically to exercise the self-heal fallback logic in isolation — confirmed by
reading `Setup()` (`worktree_ops.go:39-41`), which always serializes via
`WithRepoWorktreeLock` (intra-process `sync.Mutex` + cross-process `flock`,
`worktree_lock.go:86-107`) before ever reaching `setupNewWorktree`. **Every real production
caller goes through `Setup()`**, so the exact two-unlocked-goroutines-race this test
constructs cannot occur outside the test itself. This scopes the investigation: the question
is whether the flake reveals (a) a narrow timing assumption in the test/self-heal fallback's
own error-string matching that CI's added scheduling noise exposes, or (b) a real
production-reachable gap — and per the item's own framing, that must be established by
investigation, not assumed.

## Why This Needs Investigation, Not Just a Re-run

Per `.claude/rules/fix-flaky-tests-dont-defer.md`, a flake must be root-caused and fixed (or
explicitly filed) in the session that finds it — "known pre-existing flake, unrelated"
re-excusal is the exact anti-pattern this repo has already paid for once
(`TestRemoveHooksConfig_...`, cited in that rule doc). The self-heal fallback this test
covers has two layered race windows (documented in the test's own doc comment,
`worktree_ops_test.go:318-337`):

1. Both callers' upfront `branchRefExists` checks can observe `false` before either creates
   the branch → the loser's `git worktree add -b <branch>` fails with `"a branch named
   '<branch>' already exists"`, caught by a `strings.Contains` match at
   `worktree_ops.go:336`, falling into `setupFromExistingBranch()`.
2. By the time the loser reaches `setupFromExistingBranch()`, the winner may have already
   checked out the branch into its worktree → the loser's own `git worktree add <path>
   <branch>` (no `-b`) fails with a *second*, version-dependent error string ("already used
   by worktree at '<path>'" on git 2.50.1, "already checked out" on older git), caught by a
   second `strings.Contains` match at `worktree_ops.go:136`.

Any git error message this fallback doesn't recognize (a third message variant, a timeout,
a different git version's wording) falls through to a hard `return fmt.Errorf(...)` at either
layer — exactly the symptom observed. `runGitCommand`'s fixed 30s context timeout
(`worktree_git.go:38`) is a specific, checkable hypothesis: CI's full-suite `-race` load adds
real subprocess scheduling delay, and a `git worktree add` that times out returns a
`context.DeadlineExceeded`-flavored error, not either of the two recognized string patterns.

## Acceptance Criteria

- AC-1: Local reproduction attempted via amplified/stress-style repeated runs
  (`go test -run TestSetupNewWorktree_SelfHeals... -count=N`, `-race`, and/or artificial CPU
  contention) to determine whether the flake reproduces outside CI, or is CI-environment
  specific (documented either way with the exact commands run and their outcome).
- AC-2: A root-cause hypothesis is stated and confirmed or ruled out for each of: (a) the
  test's own timing assumptions (fixed retry/backoff/timeout window too short under load),
  (b) an unrecognized git error string variant reaching the fallback's `strings.Contains`
  checks, (c) the fallback logic itself having a real gap independent of load.
- AC-3: The distinction between "test-only artifact" (unreachable via any real caller,
  since `Setup()` always serializes) and "production-reachable gap" is explicitly resolved
  and documented, since it determines whether the fix belongs in the test, the fallback
  code, or both.
- AC-4: A fix (code, test, or both) is proposed that closes the root cause rather than
  loosening the assertion or increasing a timeout as a blind symptom fix, per
  `.claude/CLAUDE.md`'s "No fix without root cause" rule.
- AC-5: The fix plan includes a regression test/mechanism that would have caught this
  specific failure mode (per `.claude/rules/fix-flaky-tests-dont-defer.md`'s worked example).

## Out of Scope

- Implementing the fix in this session (triage/planning only — research, plan, and
  pre-mortem validation land in `project_plans/worktree-selfheal-test-flake/`; a follow-up
  implementation session executes the plan).
- Re-litigating `worktree-branch-exists-race`'s separate, already-shipped fix
  (`branchRefExists`'s `ErrReferenceNotFound` classification, `project_plans/worktree-branch-exists-race/`) —
  that fix addressed a different call site's error *classification*; this item is about the
  self-heal *fallback's* error-string matching and/or timing under load.
