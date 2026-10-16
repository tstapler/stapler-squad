# Research: Features/Edge-Cases — worktree-selfheal-test-flake

Agent 2 (Features/Edge-cases). Scope: enumerate git-stderr edge cases the self-heal
fallback's two `strings.Contains` checks don't catch, check prior-fix history for overlap,
and check for existing/duplicate test coverage.

## 1. VERIFIED: a context-timeout kill produces an error string neither check matches

`runGitCommand` (`session/git/worktree_git.go:37-47`) wraps every git subprocess in a fixed
30s `context.WithTimeout`, then calls `g.commandRunner().Run(ctx, ...)`, which for the
production default (`tmux.LocalRunner`, `session/tmux/command_runner.go:81-85`) is
`safeexec.CommandContext(ctx, name, args...).CombinedOutput()` — a bare wrapper around
`os/exec.CommandContext` with `WaitDelay` pre-set (`executor/safeexec/safeexec.go:30-34`).

I reproduced this directly (not inferred) with a throwaway Go program mirroring the same
call shape (`context.WithTimeout` + `exec.CommandContext(...).CombinedOutput()` +
`WaitDelay`):

```
out=""
err=signal: killed
err type=*exec.ExitError
```

So when the 30s deadline fires, `Run()` does **not** return `context.DeadlineExceeded` or any
text containing "context deadline exceeded" — it returns a plain `*exec.ExitError` whose
`.Error()` is exactly `"signal: killed"`, with empty captured output (git is SIGKILL'd, no
graceful stderr flush). `runGitCommand` then wraps this as:

```
git command failed:  (signal: killed)
```

This string contains neither `"already exists"` (the check at `worktree_ops.go:336`) nor
`"already checked out"` / `"already used by worktree"` (the check at `worktree_ops.go:136`).
A `git worktree add -b ...` that times out under load — not a git-level "already exists"
race at all — falls straight through both fallback layers to the hard error at
`worktree_ops.go:340` (`"failed to create worktree from commit %s: %w"`), or the equivalent
at `worktree_ops.go:163` if the timeout instead hits `setupFromExistingBranch`'s own
`worktree add`. This confirms the requirements doc's stated hypothesis (not just
"plausible" — verified against the real error text `os/exec` produces) and is the
single most likely root cause of the observed CI-load flake.

## 2. Other stderr shapes plausible under CI load that also aren't caught

None of these were reproduced against a real contended repo (out of scope/time for this
agent), but each is a documented git failure mode for concurrent `.git/worktrees/` access,
and none matches either `strings.Contains` check:

- **`.lock` file contention** (a second concurrent git process — not necessarily this
  package's own, since `WithRepoWorktreeLock` only serializes goroutines that go through
  `Setup()`/`Remove()`/`Prune()`, not the raw `git` CLI a human or another tool might invoke
  on the same repo path) — git's message is `"fatal: Unable to create
  '.../index.lock': File exists."` or `"fatal: cannot lock ref 'refs/heads/<branch>':
  Unable to create '.../<branch>.lock': File exists."`. Git's own wording is **"File
  exists"**, not **"already exists"** — a near-miss that a naive reviewer might assume is
  covered by the existing check; it is not (different phrase, `strings.Contains` is exact
  substring only).
- **`packed-refs` lock contention** during a concurrent `git pack-refs`/`gc`:
  `"fatal: Unable to create '.../packed-refs.lock': File exists."` — same near-miss.
  (This overlaps conceptually with `project_plans/worktree-branch-exists-race/`'s
  `packed-refs` corruption scenario, but that prior fix was about `repo.Reference()`
  misclassifying an error as "branch missing" via go-git, not about this package's
  `strings.Contains`-based stderr matching on the `worktree add` subprocess — no overlap in
  the actual code path, see §4.)
- **Disk/OS transient errors**: `"fatal: Unable to create directory ..."`,
  `"fatal: cannot mkdir ...: No space left on device"` — not caught, correctly falls to hard
  error (arguably *should* hard-fail rather than self-heal, since it's not a "someone else
  already created it" condition).
- **Directory already exists (non-git) collision**: if `setupFromExistingBranch`'s cleanup
  (`worktree remove -f`) itself raced and left a stale directory, `worktree add <path>
  <branch>` can fail with `"fatal: '<path>' already exists"` — this string **does** contain
  `"already exists"` and so is (accidentally) caught by the `setupNewWorktree` check at line
  336, but that check only fires from `setupNewWorktree`'s own `worktree add -b` call, not
  from `setupFromExistingBranch`'s `worktree add` (no `-b`) — that path only checks the
  "already checked out"/"already used by worktree" pair (line 136), so this particular
  message reaching `setupFromExistingBranch` would fall through uncaught there.

## 3. Test-only vs. production-reachable (AC-3) — strong evidence for test-only

`setupNewWorktree()` and `setupFromExistingBranch()` are unexported and, per
`grep -rn "setupNewWorktree\|setupFromExistingBranch"` across `session/`, are only ever
invoked by (a) each other, (b) `setupLocked()` (`worktree_ops.go:100,102`), and (c) the
test file directly. There is no other internal or external caller.

`setupLocked()` is only reachable via `Setup()` (`worktree_ops.go:39-41`) or `SetupLocked()`
(`worktree_ops.go:53-55`), and **every** production call site of either
(`server/services/backlog_service_triage.go`, `session/git_worktree_manager.go`,
`session/instance.go`, `session/instance_worktree.go`, `session/git/remote_worktree.go`)
goes through `WithRepoWorktreeLock` (`session/git/worktree_lock.go:86-107`) — a lock keyed
by `absRepoPath` (SHA-256'd path, `worktree_lock.go:70-71`) that is **both** an
intra-process `sync.Mutex` **and** a cross-process `flock.Flock`, held for the full
duration of `fn()` (i.e. the entire `setupLocked` call, including the `git worktree add`
subprocess) with a 30s acquire timeout. Two concurrent `Setup()` calls for the *same*
repoPath (the scenario this test exercises — both `wt1`/`wt2` share `repoDir`) cannot
overlap in production: the second blocks until the first's `fn()` — including its
`git worktree add -b` call — has fully returned and released the lock.

Timeline confirms this was closed *after* the self-heal fallback was added:
- `492b0d6df` "fix(worktree): close backlog worktree branch-collision race" (2026-08-13)
  introduced the two-layer self-heal fallback specifically to tolerate the
  concurrent-spawn-same-branch race, *before* any cross-process locking existed.
- `de174658f` "fix(git): serialize worktree add/remove/prune across goroutines and
  processes" (later) added `WithRepoWorktreeLock`, which fully serializes `Setup()` for a
  given repoPath — closing the *exact* race the self-heal fallback was built to tolerate,
  at the source, for every real caller.
- `33cfab20f` "fix(worktree): close CreateBacklogWorktree repair/setup race" extended the
  same lock to cover repo-repair too.

`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` calls the unlocked
`setupNewWorktree()` directly (bypassing `WithRepoWorktreeLock` entirely) specifically to
still exercise this now-otherwise-unreachable-in-production code path. The adjacent
`TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo`
(added by `de174658f`) is the one that proves the *lock* now prevents the race for real
callers — its own doc comment says as much ("verify the lock actually prevents the metadata
race... rather than merely tolerating single-branch collisions like [the SelfHeals test]").

**Conclusion for AC-3**: the specific concurrent-same-branch race this flaky test recreates
is a test-only artifact today — real production callers cannot hit it because
`WithRepoWorktreeLock` fully serializes them. The self-heal fallback code itself is not
dead/pointless, though: it's still a legitimate defense for a branch that exists for
*other* reasons (e.g. a human `git branch`'d it manually, or a previous partially-failed
run left the branch but not the worktree) — those aren't concurrency-dependent and can still
occur one-caller-at-a-time. The *test's* value going forward is arguably weaker (it's
proving a fallback that no in-process caller can trigger via the race it simulates), but the
underlying fallback code is still worth keeping and worth hardening against the timeout gap
in §1.

## 4. No overlap with `project_plans/worktree-branch-exists-race/`

That prior fix (commit context: fixes `Setup()`'s and `setupNewWorktree()`'s
`repo.Reference()` calls to distinguish `plumbing.ErrReferenceNotFound` from any other
go-git error via the now-existing `branchRefExists` helper, `worktree_ops.go:18-32`) is a
different code path entirely: it's about **go-git's in-process ref read**
(`repo.Reference(...)`), not about **parsing `git` CLI subprocess stderr** (the
`strings.Contains` checks this investigation is about). `branchRefExists` is already fixed
and unrelated to the timeout gap — no duplicate work needed, and no risk of the new fix
re-touching that helper.

## 5. Existing regression-test infrastructure fits a deterministic fix directly

`session/git/worktree_git_test.go:730-761` already defines `gitSpyCommandRunner`, a
`tmux.CommandRunner` test double injectable via `GitWorktree`'s `WithCommandRunner` option,
supporting either a canned `(runOut, runErr)` pair or a `runFunc` hook invoked at the moment
each `Run` call would exec. This is the exact seam needed to write a **deterministic**
regression test for the gap in §1 — inject a spy that returns `("", errors.New("signal:
killed"))` (or wraps `context.DeadlineExceeded`) for the `worktree add -b` call specifically,
and assert on the resulting behavior — without depending on real CI-load timing at all. No
existing test in `worktree_ops_test.go` or `worktree_git_test.go` already covers this
timeout-during-self-heal scenario (grepped both files; the only two `setupNewWorktree`-
related tests besides the race test are `TestSetupNewWorktree_SurfacesError_When_
BranchRefIsMalformed` and `TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists`,
neither of which touches the `strings.Contains` fallback or `runGitCommand`'s timeout path)
— a new test would not be a duplicate.

## Files referenced

- `session/git/worktree_ops.go:18-32` (`branchRefExists`), `:39-55` (`Setup`/`SetupLocked`),
  `:100-170` (`setupFromExistingBranch`), `:270-344` (`setupNewWorktree`)
- `session/git/worktree_git.go:37-47` (`runGitCommand`)
- `session/git/worktree_lock.go` (`WithRepoWorktreeLock`, full file)
- `session/tmux/command_runner.go:52-113` (`CommandRunner`, `LocalRunner`)
- `executor/safeexec/safeexec.go` (`CommandContext`, `WaitDelay`)
- `session/git/worktree_ops_test.go:318-419` (the two tests under investigation)
- `session/git/worktree_git_test.go:730-813` (`gitSpyCommandRunner` + injection pattern)
- `project_plans/worktree-branch-exists-race/requirements.md` (prior, unrelated fix)
