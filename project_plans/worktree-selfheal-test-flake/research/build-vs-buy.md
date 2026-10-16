# Build vs. Buy: worktree-selfheal-test-flake

Scope: evaluate whether any dependency/tool change is warranted for fixing
`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`
(`session/git/worktree_ops_test.go:338`), vs. a first-party fix in
`session/git/worktree_ops.go` / `worktree_git.go`.

## 1. go-git v5 for worktree-add race detection / linked-worktree creation

**go.mod**: `github.com/go-git/go-git/v5 v5.14.0` (already a direct dependency).

**Does go-git expose a typed error or state check for this race?** No. go-git v5's
`Repository.Worktree()` returns the *single* worktree bound to that `Repository` value —
it has no concept of git's linked-worktree administrative area
(`.git/worktrees/<name>/`) at all. There is no `AddWorktree`/`LinkWorktree` API, no
typed error for "branch already checked out elsewhere" or "branch already exists",
and no porcelain-equivalent read of `git worktree list`. This is a known, complete
gap in go-git, not a partial one — confirmed by the fact that this repo's own code
already works around it everywhere it needs linked-worktree behavior:
`session/git/worktree_ops.go:131` and `:329` both shell out to `git worktree add`
via `runGitCommand`, and `findWorktreeForBranch`/`isWorktreeLocked`
(`worktree_ops.go:224-268`) hand-parse `git worktree list --porcelain` text because
go-git has nothing equivalent to read.

**Consequence for the strings.Contains pattern**: the two match sites
(`worktree_ops.go:136` — `"already checked out"` / `"already used by worktree"`, and
`worktree_ops.go:336` — `"already exists"`) cannot be replaced with a go-git typed
error or a go-git state query, because go-git has no visibility into linked-worktree
state at all. The only structured alternative already in this file is exactly what
`findWorktreeForBranch` already does: parse `git worktree list --porcelain` (still a
subprocess, but structured output instead of matching stderr prose). If the fix
needs to detect "worktree metadata already reflects the winner's checkout," a
porcelain-list read is more robust than a stderr substring match — but it is still
a first-party parsing routine over CLI output, not a go-git feature. This is the
"already a known 'buy' gap" the requirements doc anticipated: **confirmed, not
newly discovered**.

**Verdict on go-git for this sub-problem**: no library change indicated. go-git stays
scoped to what it's already used for in this file (`branchRefExists` via
`repo.Reference`, `git.PlainOpen`) — reference-level reads, not worktree-add
operations.

## 2. Flake reproduction / stress tooling

`golang.org/x/tools/cmd/stress` (`go-stress`, dvyukov's original "stress" tool) is
**already adopted** in this repo, added in commit `7a695fe45` ("ci: replace serial
PTY race repetitions with go-stress, simplify integration coverage") — the second
commit back in this branch's own recent history. See
`.github/workflows/build.yml:400-401`:

```yaml
- name: Install go-stress
  run: go install golang.org/x/tools/cmd/stress@v0.47.0
```

and the "PTY-triple race regression (parallel amplification via go-stress)" step at
`build.yml:408-433`, which compiles a `-race` test binary (`go test -race -c -o
...`) and runs it under `stress` for a fixed wall-clock budget across all CPUs,
replacing a serial `go test -count=20` loop specifically because it packs more
repetitions into the same budget and fails fast on the first bad run.

The same pattern applies directly to this investigation: compile
`session/git`'s test binary with `-race -c` and run it under `stress` (already
installed via the exact command above) rather than looping
`go test -race -count=N ./session/git -run TestSetupNewWorktree_SelfHeals...`.
`go test -count=N` is not wrong for local reproduction, but `stress` is the
project's already-established idiom for "amplify a rare interleaving under CPU
contention" once reproduction moves into CI or needs to run many iterations in
parallel — no new tool to adopt or evaluate.

`Makefile`'s `install-tools` target (`Makefile:603-613`) does not list `go-stress`
alongside nilaway/staticcheck/golangci-lint/gosec/go-nilcheck/deadcode/benchstat/
checklocks — it's currently only installed ad hoc in the CI workflow step, not as a
standing dev-tool. That's a minor gap worth flagging to the planning phase (whether
to add `go install golang.org/x/tools/cmd/stress@v0.47.0` to `install-tools` so
local repro doesn't require copying the version pin out of `build.yml`), but it does
not change the "already adopted, no new dependency" verdict.

## 3. Existing retry-around-git-subprocess-race pattern to reuse

Yes — `session/git/util.go:283-310`'s `getHeadCommitSHA` retry (backing
`headSHARetryAttempts = 3`, `headSHARetryDelay = 20 * time.Millisecond`) is the
directly analogous, already-in-repo pattern for "a git-adjacent read observed to
fail transiently under concurrent load; retry a small bounded number of times with
a short fixed delay before falling back/failing for real":

```go
// headSHARetryAttempts and headSHARetryDelay bound the go-git torn-read mitigation in
// getHeadCommitSHA — see its doc comment.
const (
	headSHARetryAttempts = 3
	headSHARetryDelay    = 20 * time.Millisecond
)
```

Its doc comment (`util.go:290-309`) documents exactly the shape of problem this
flake is: a go-git/git-CLI race under concurrent worktree operations, where the
first read can come back wrong/erroring but a second read shortly after succeeds —
"both retry briefly, and only after repeated failures do we fall back." This is a
closer structural match than inventing a new retry helper, and `.claude/rules/
prefer-go-git-over-subshells.md` (this repo's own checked-in rule) cites this exact
function as the canonical "hybrid: try go-git first, fall back to CLI only for a
specific, documented failure mode" pattern — i.e. reuse-not-reinvent is already the
codified house style here, not just incidentally available.

Distinct from retry, `WithRepoWorktreeLock` (`session/git/worktree_lock.go:86-107`)
already serializes `worktree add`/`worktree remove` per-repoPath across goroutines
*and* OS processes via a file lock (`Setup()` at `worktree_ops.go:37-39` wraps
`setupLocked` in it). The flaky test deliberately bypasses this by calling the
unlocked `setupNewWorktree()` directly (confirmed at `worktree_ops_test.go:373-386`,
whose own doc comment states real production callers always go through
`Setup()`/the lock). That means the self-heal fallback being tested is a second,
independent layer of protection for a window `WithRepoWorktreeLock` cannot close
(two *separate processes* or the test's intentional in-process race below the
lock), not a redundant one — the fix must live in the fallback logic itself, not in
adding more locking.

No dedicated "retry a git subprocess call" helper exists yet in `session/git/*.go`
(grep for `retry`/`Retry`/`backoff`/`Backoff` across the package turned up only
`getHeadCommitSHA`'s local constants/loop and unrelated comment mentions — nothing
generic to import). If the eventual fix needs retry logic beyond what
`getHeadCommitSHA` already inlines, the precedent set by that function (a small
local `const` + `for` loop, not a generic package) is the style to follow, not a
new shared "retry" abstraction — consistent with
`.claude/rules/interface-pollution-checklist.md`'s "no speculative
generic/abstraction for a single call site" guidance.

## 4. Verdict

**Build — fix in first-party code, no new dependency, no new tool.**

- go-git v5 has no linked-worktree API to migrate to; the existing subprocess +
  stderr/porcelain-parsing approach in `worktree_ops.go` is already the
  furthest go-git can be pushed for this problem (confirmed gap, matches the
  requirements doc's suspicion).
- Stress-testing tooling (`golang.org/x/tools/cmd/stress`) is already installed
  and already the established CI idiom for amplifying this exact class of race
  (`build.yml:400-433`, committed two commits prior on this branch) — reuse it
  for local/CI reproduction rather than adding anything or defaulting to a bare
  `go test -count=N` loop.
- A directly analogous retry-then-fallback pattern already exists
  (`getHeadCommitSHA`, `util.go:283-329`) and is the right template to follow if
  the fix needs bounded retries around the racy `git worktree add`/`list` calls,
  rather than inventing a new retry helper or reaching for an external
  retry/backoff library (e.g. `github.com/cenkalti/backoff`) that this repo does
  not otherwise use anywhere in `session/git`.

**Pros of build**: no new dependency surface, reuses code review-approved patterns
and conventions already in the file/package, keeps the fix scoped to the actual
root cause (fallback logic + its string matching / retry timing) rather than a
tooling swap.

**Cons of build**: the stderr-substring matching at `worktree_ops.go:136` and `:336`
remains inherently version-fragile (already documented as such in the existing
comment about git 2.50.1's message wording differing from older git) — but no
build-vs-buy alternative removes that fragility, since go-git cannot see linked
worktrees at all; the only more-robust option (parsing `worktree list --porcelain`)
is still hand-rolled first-party code, already partially present via
`findWorktreeForBranch`.
