# Pitfalls: worktree-branch-exists-race

Research for `sdd:fix-bug` on backlog item `200b6896-3d63-421b-9e96-41cf06d289fa`.
Target file: `session/git/worktree_ops.go` (491 lines, read in full).

## 1. Concurrency risk in `Setup()`'s goroutine (line ~34-47)

`Setup()` runs two goroutines against a `errChan := make(chan error, 2)` (buffered,
capacity 2) and a shared `var branchExists bool` written only by the branch-check
goroutine:

```go
go func() { errChan <- os.MkdirAll(worktreesDir, 0755) }()          // goroutine A

go func() {                                                          // goroutine B
    repo, err := git.PlainOpen(g.repoPath)
    if err != nil {
        errChan <- fmt.Errorf("failed to open repository: %w", err)
        return
    }
    branchRef := plumbing.NewBranchReferenceName(g.branchName)
    if _, err := repo.Reference(branchRef, false); err == nil {
        branchExists = true
    }
    errChan <- nil                                                   // always nil today
}()

for i := 0; i < 2; i++ {
    if err := <-errChan; err != nil {
        return err                                                    // returns on FIRST error
    }
}
```

Key facts that bound how safe a fix is:

- The channel is **buffered at capacity 2**, and each goroutine sends **exactly once**
  no matter what. So the "return on first error, don't drain the rest" pattern is safe
  today, and stays safe after the fix, as long as **each goroutine still sends exactly
  one value** — the buffer absorbs the second send even if the caller already returned.
  Do not change this to an unbuffered channel or add a second send from goroutine B
  (e.g. accidentally sending both a "reference error" and the trailing `errChan <- nil`)
  or it will either deadlock (unbuffered, no one left reading) or silently double-count.
- **A naive fix that changes `errChan <- nil` to `errChan <- err` (propagating any
  non-nil `Reference()` error)** is *mostly* correct for goroutine B in isolation, but:
  - It must still distinguish `plumbing.ErrReferenceNotFound` (expected, not an error)
    from a genuine failure — sending `ErrReferenceNotFound` up `errChan` unconditionally
    would make `Setup()` return an error on the **common, everyday case** ("branch does
    not exist yet, please create it") and break every new-worktree spawn. This is the
    central classification bug to fix, not a side effect to avoid.
  - Because the loop returns on the *first* error read from `errChan`, and goroutine A
    (`MkdirAll`) and goroutine B race in nondeterministic order, if goroutine A also
    happens to have an error (rare, but e.g. permission denied on `worktreesDir`), only
    one of the two errors is ever returned/logged — the other is silently dropped after
    being absorbed into the buffer. This is **pre-existing** behavior (not introduced by
    this fix) but worth naming: if the fix wants "surface a genuine non-NotFound error
    loudly" per the requirements, prefer `log.Error(...)` *inside* goroutine B before
    sending on `errChan`, so the error is visible in logs even if goroutine A's error
    wins the race for the returned value. Don't rely solely on the return value to
    guarantee visibility.
  - `branchExists` is written by goroutine B with no lock, but it's only *read* by the
    main goroutine after both channel receives complete (a `MkdirAll` receive and a
    goroutine-B receive), which is a valid happens-before relationship (channel receive
    synchronizes-with the corresponding send) — no data race today, and none introduced
    by this fix as long as `branchExists` is still written by exactly one goroutine
    before its `errChan` send and read only after the wait loop completes.

## 2. Over-fixing vs under-fixing

- **Over-fixing**: treating *every* non-nil, non-NotFound error from `repo.Reference()`
  as immediately fatal for `Setup()` risks turning a previously-tolerated transient
  blip (e.g. a `.git/packed-refs` read racing a concurrent `git gc` or another
  worktree's `git branch` call) into a hard failure that used to self-resolve by luck
  on the *next* retry. The requirements doc explicitly frames this as "swallowed
  ref-lookup errors" causing *false* "branch already exists" failures — the fix's job
  is to stop **misrouting** into branch-creation, not to make `Setup()` newly brittle
  under real transient I/O errors. A single hard failure that surfaces the real error
  (instead of a confusing "branch already exists" fatal from git) is still strictly
  better than today even if it's not retried automatically — `resolveSessionPath`'s
  BUG-057 policy already treats worktree setup failures as non-recoverable, so this
  fix does not need to invent its own retry/backoff (explicitly out of scope).
- **Under-fixing**: logging the non-NotFound error but *still* falling through to
  "assume branch absent" reproduces the exact bug being fixed — it would still route
  into `setupNewWorktree()`'s `git worktree add -b <branch> ...`, which is the call
  that produces the non-recoverable `fatal: a branch named '...' already exists` in
  the first place. A log line alone does not change control flow; the fix must
  actually branch differently (propagate the error, or otherwise avoid the
  branch-creation path) for a confirmed non-NotFound error.
- **Net guidance**: `errors.Is(err, plumbing.ErrReferenceNotFound)` → treat as absent
  (existing behavior, preserved). Any other non-nil error → do NOT set
  `branchExists = false` and fall through silently; propagate it as a real error from
  `Setup()`/`setupNewWorktree()` (log loudly at minimum, per requirements' "or is
  logged loudly instead").

## 3. Third occurrence check — NO third instance of the same bug, but a THIRD site
   already shows the *correct* pattern

Read `session/git/worktree_ops.go` in full (`Setup()`, `setupFromExistingBranch()`,
`initBaseCommitSHA()`, `worktreeAlreadyRegisteredForBranch()`, `isWorktreeLocked()`,
`findWorktreeForBranch()`, `setupNewWorktree()`, `Cleanup()`, `Remove()`,
`forceCleanupWorktree()`, `Prune()`, `CleanupWorktrees()`) plus the sibling file
`session/git/worktree_branch.go` (`cleanupExistingBranch`, called from
`setupNewWorktree()` at line 242):

- Confirmed: exactly the two call sites named in the requirements doc collapse "any
  error" into "does not exist" — `Setup()` line 43 and `setupNewWorktree()` line 235.
  Both are `plumbing.NewBranchReferenceName` + `repo.Reference(branchRef, false)` +
  `if ... err == nil { branchExists = true }`.
- `cleanupExistingBranch` (`session/git/worktree_branch.go:15,24`) is a **third
  ref-error-handling site in the same call chain** (it's invoked from
  `setupNewWorktree()` right after the branch-absent branch is taken) but it already
  does the distinction correctly:
  ```go
  if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
      return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
  }
  ```
  Two things worth carrying into the fix:
  1. This is the **existing in-repo precedent** for how to compare against
     `plumbing.ErrReferenceNotFound` — use it as the reference implementation/style
     match rather than inventing new phrasing.
  2. It uses **direct `!=` comparison**, not `errors.Is`. `plumbing.ErrReferenceNotFound`
     is a package-level `var` sentinel (`errors.New(...)`-style) returned unwrapped by
     go-git's `Reference()`/`RemoveReference()` in current versions, so `==`/`!=` and
     `errors.Is` are behaviorally equivalent *here*. The requirements doc explicitly
     asks for `errors.Is` — that's the more defensive/idiomatic choice (survives future
     wrapping) and doesn't conflict with this precedent; just note the stylistic
     mismatch so the fix doesn't need to "match" the `!=` form if `errors.Is` is
     preferred repo-wide per `golang-error-handling`.
- No other `repo.Reference(...)` or `Storer.RemoveReference(...)` call sites exist
  elsewhere in `session/git/*.go` outside these three (confirmed via
  `grep -rn "ErrReferenceNotFound\|errors.Is(err" session/git/*.go` and a full read of
  `worktree_ops.go`).

## 4. Repo conventions from `.claude/rules/`

- **`go-double-checked-locking.md`** (return the locally-computed value, not the cache
  slot): not directly applicable here — there's no read-lock/write-lock/re-read-slot
  pattern in `Setup()`. The closest analogy is `branchExists`, but it's written once by
  a single goroutine and read once after synchronization, not re-read from shared state
  after a race — no violation risk, no change needed to comply with this rule.
- **`primitive-obsession-checklist.md`** (avoid same-typed parameter piles): not
  triggered by this fix — no new function signature is being added with 2+ same-typed
  primitive parameters. `Setup()`/`setupNewWorktree()` already operate on `g` (the
  `GitWorktree` receiver) rather than loose parameters. If the fix introduces a new
  helper (e.g. a shared `branchExists(repo, branchRef) (bool, error)` to de-duplicate
  the two call sites), keep it a method on `*GitWorktree` or take the already-typed
  `plumbing.ReferenceName` rather than a bare `string`, consistent with existing style.
- **`prefer-go-git-over-subshells.md`**: both call sites already use `go-git`
  (`repo.Reference`) rather than shelling out — no regression risk from this rule, but
  worth confirming the fix doesn't introduce a `git show-ref`/`git rev-parse` subshell
  fallback for "double check" purposes; stick with go-git idioms
  (`errors.Is(err, plumbing.ErrReferenceNotFound)`) per this rule's spirit.
- **`fix-flaky-tests-dont-defer.md`**: not directly about this bug, but relevant to
  #5 below — since this bug is itself about a **race** producing an intermittent
  failure, the new regression test should be deterministic (simulate the error
  directly, not rely on timing a real race) so it doesn't become a new flaky test.

## 5. Existing test infra for constructing a broken/errored `Reference()` call

`session/git/worktree_creation_test.go` and `session/git/ops_test.go` are the two
existing test files exercising this code path (also `worktree_test.go`,
`worktree_git_test.go`, `worktree_git_stage_test.go`, `scaffolding_test.go`,
`drift_test.go`, `diff_test.go`, `util_test.go` — no dedicated `worktree_ops_test.go`
exists yet; the relevant tests currently live in `worktree_creation_test.go`).

Key existing helpers (real filesystem git repos, no mocking of go-git):

- `setupTestRepo(t *testing.T) string` (`worktree_creation_test.go:17-41`) — creates a
  real temp-dir git repo via `safeexec` + real `git init`/`config`/`commit`, renames
  default branch to `main`. This is the standard fixture used by every test in the
  file.
- `NewGitWorktree(repoDir, sessionName)` / `NewGitWorktreeWithBranch(repoDir,
  sessionName, branch)` — construct a `*GitWorktree` against that real repo.
- Existing precedent for **simulating an abnormal git state on disk** to hit a
  specific code branch: `TestWorktreeSetup_RecreatesLockedInterruptedWorktree`
  (`worktree_creation_test.go:379-403`) calls `wt1.runGitCommand(repoDir, "worktree",
  "lock", "--reason", "initializing", path)` and then `os.Remove(trackedFile)` to
  fabricate an "interrupted checkout" state, then asserts the second `Setup()` call
  rebuilds rather than reuses it. This is the closest existing pattern to what's
  needed for a non-NotFound `Reference()` error test.

**Feasibility for testing the non-NotFound branch specifically**: this is doable
without shelling out to git or a full repo corruption, using go-git APIs directly
inside the test (no need to fake a corrupted repo on disk):

- **Cleanest option**: don't test through `Setup()`'s goroutine at all — extract (or
  keep) the classification logic as a small testable unit, e.g. a helper
  `branchRefExists(repo *git.Repository, ref plumbing.ReferenceName) (bool, error)`
  that returns `(false, nil)` on `ErrReferenceNotFound` and `(false, err)` on any other
  error. Unit-test that helper directly against:
  - A `git.PlainOpen` of a `setupTestRepo(t)` fixture, checking a ref name that
    genuinely doesn't exist → expect `(false, nil)`.
  - A **fabricated non-NotFound error** — go-git's `Reference()` ultimately delegates
    to the underlying `storer.Storer` (`repo.Storer`), which for `git.PlainOpen` is a
    `filesystem.Storage`. The simplest reliable way to force a non-NotFound error
    without corrupting a real repo is to pass a **malformed reference name** or query
    against a repo whose `.git` directory has had its `packed-refs` file replaced with
    unreadable/invalid content (e.g. `os.Chmod(filepath.Join(repoDir, ".git",
    "packed-refs"), 0000)` if `packed-refs` exists, or write garbage bytes into a loose
    ref file under `.git/refs/heads/<branch>` so it fails to parse as a valid SHA) —
    both are filesystem-level manipulations on the *real* temp-dir fixture already used
    elsewhere in this file, consistent with the existing "manipulate real git state on
    disk" testing style (see #5's `TestWorktreeSetup_RecreatesLockedInterruptedWorktree`
    precedent) rather than introducing a mock `storer.Storer`.
  - **CORRECTION (2026-08-11, post-adversarial-review — the claim below was never executed
    against the real library and is FALSE for go-git v5.14.0, the version pinned in this
    repo's `go.mod`; do not re-trust it)**: the paragraph originally here claimed that
    writing invalid ref content directly, e.g. `os.WriteFile(filepath.Join(repoDir, ".git",
    "refs", "heads", branchName), []byte("not-a-valid-sha\n"), 0644)`, causes go-git's
    loose-ref parser to reject the malformed content with a non-`ErrReferenceNotFound`
    error. **This is false.** `plumbing.NewHash()` (`plumbing/hash.go:26-33`) does
    `b, _ := hex.DecodeString(s)` — it silently discards the hex-decode failure and returns
    a zero `Hash` with a **nil** error. Verified directly with a throwaway `go run` program
    against the pinned `github.com/go-git/go-git/v5@v5.14.0`: this fixture produced
    `err: <nil>`, `ref: 0000000000000000000000000000000000000000 refs/heads/<branch>` —
    i.e. `repo.Reference()` reports the branch as *existing* (with a zero hash), not
    erroring. Use the **packed-refs-corruption** option from the bullet above instead
    (write malformed bytes, e.g. `"not a valid packed-refs file\nrandom garbage"`, directly
    into `.git/packed-refs` via `os.WriteFile` — no chmod needed — while the target branch
    has no loose ref of its own, so the lookup falls through to the packed-refs parser).
    That fixture was verified in the same run: `err: "malformed packed-ref"`,
    `errors.Is(err, plumbing.ErrReferenceNotFound): false` — a genuine, distinguishable
    error. This is the fixture used by `project_plans/worktree-branch-exists-race/
    implementation/plan.md`'s Stories 1.1.1/1.1.2 and Task 1.2.1a.
- Testing all the way through `Setup()`'s goroutine (rather than a unit-tested helper)
  is also possible using the same fabricated packed-refs-corruption trick (see the
  CORRECTION above — not the disproven loose-ref-content trick) against a real
  `setupTestRepo(t)` fixture, then asserting `Setup()` returns a non-nil error (instead
  of silently proceeding to `setupNewWorktree()` and hitting `fatal: a branch named
  '...' already exists` from the underlying `git worktree add -b` — the actual bug
  manifestation). This is preferable if the fix keeps the classification logic inline
  rather than extracting a helper, since it exercises the real regression end-to-end.
