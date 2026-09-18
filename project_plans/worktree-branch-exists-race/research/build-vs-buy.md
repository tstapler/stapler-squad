# Build vs. buy: "does this branch exist" check

Bug: `GitWorktree.Setup()` (`session/git/worktree_ops.go:43`) and `setupNewWorktree()`
(`session/git/worktree_ops.go:235`) both do:

```go
branchRef := plumbing.NewBranchReferenceName(g.branchName)
if _, err := repo.Reference(branchRef, false); err == nil {
    branchExists = true // or: treated as "exists"
}
```

Any non-nil error (not just "ref not found") is silently treated as "branch does not
exist," which can misclassify a transient/other error as absence and send the caller
down the "create new branch" path, later failing with `git worktree add -b <branch>`:
"branch already exists."

## 1. Does go-git expose a higher-level "branch exists" helper?

Checked `github.com/go-git/go-git/v5 v5.14.0` (per `go.mod:17`), package cache at
`~/go/pkg/mod/github.com/go-git/go-git/v5@v5.14.0`.

- **`(*Repository).Reference(name, resolved bool)`** (`repository.go:1502`) is a thin
  passthrough: `return r.Storer.Reference(name)`. This is what the buggy code already
  calls — there's no more-direct variant.
- **`(*Repository).Branch(name string) (*config.Branch, error)`** (`repository.go:682`)
  looks like the obvious candidate but is a **false friend**: it reads `.git/config`'s
  `[branch "name"]` section (tracking/merge config), not the ref itself, and returns
  `ErrBranchNotFound` — a different sentinel. Most branches (especially freshly created
  local ones with no upstream) have no config entry at all, so this would report
  "doesn't exist" for branches that plainly do. Not usable for this check.
- **`(*Repository).Branches() (storer.ReferenceIter, error)`** (`repository.go:1382`)
  returns an iterator over all local branch refs; no `Exists`/lookup convenience is
  layered on top. Using it here would mean iterating and comparing names — strictly
  more code than the existing `Reference()` call, no error-handling improvement.
- Tracing the call down: `Storer.Reference()` → filesystem storer's
  `ReferenceStorage.Reference()` (`storage/filesystem/reference.go:21`) → `dotgit.Ref()`
  (`storage/filesystem/dotgit/dotgit.go:739`), which tries
  `readReferenceFile(".", name)` first and falls back to `packedRef(name)` only on
  *any* error from the file read — not specifically not-exist. `packedRef` itself
  returns the sentinel `plumbing.ErrReferenceNotFound` (`dotgit.go:32`,
  `plumbing/reference.go:32`) only when the name genuinely isn't in the packed-refs
  scan; other failures (permission errors, a read racing a concurrent writer) surface
  as their own distinct errors.

**Conclusion:** go-git has no built-in "branch exists, correctly distinguishing
not-found from other errors" helper. `errors.Is(err, plumbing.ErrReferenceNotFound)`
on the existing `Reference()` call is the correct and only supported way to ask this
question at this API level.

## 2. Is shelling out to `git show-ref`/`git rev-parse --verify` a viable alternative?

Viable mechanically — `git show-ref --verify --quiet refs/heads/<branch>` (exit 0 =
exists, exit 1 = doesn't, anything else = a real error) does cleanly separate
"not found" from "broken" via exit code, and would sidestep the go-git error-typing
question entirely.

Weighed against `.claude/rules/prefer-go-git-over-subshells.md`: that rule's bar for a
subshell exception is a **named, documented failure mode** that go-git specifically
can't handle — the precedent it cites is `getHeadCommitSHA`'s fallback to
`getHeadCommitSHAViaCLI` for "a torn-read race on ref files that the git CLI's atomic
rename ref updates don't hit" (`session/git/util.go`). That precedent is directly
relevant here: `dotgit.Ref()`'s `readReferenceFile` read (above) is exactly this class
of race — a concurrent `git worktree add -b` in another goroutine/process can be
mid-write to the loose ref file while this check reads it, which is plausibly the
actual root cause of the reported race, not just a missing `errors.Is`.

However: this bug is not "go-git categorically cannot answer this question" — it can,
correctly, via `errors.Is(err, plumbing.ErrReferenceNotFound)`. The CLI would also
have its own race exposure (reading `refs/heads/<branch>` while another process writes
it — git's atomic rename helps but `show-ref` can still transiently see a torn packed-refs
scan under concurrent `pack-refs`, though loose-ref renames are atomic). Introducing a
subshell here would fix the symptom but isn't justified by the rule's bar: the fix is a
2-line change to existing go-git code, not a case where go-git structurally can't do
the job. Recommend **staying with go-git** and using `errors.Is`; if the race persists
after that fix (i.e. the torn-read hypothesis proves correct in practice), that would
be the point to name the specific failure and consider the CLI fallback pattern
`getHeadCommitSHA` already establishes — not preemptively.

## 3. Is a 2-line `errors.Is` fix clearly correct vs. any library/dependency change?

Yes — confirmed by checking for an existing internal helper this code should have
called instead of hand-rolling the check.

Grep across `session/git/*.go` and `session/*.go` for `ErrReferenceNotFound` and
branch-existence patterns:

```
session/git/worktree_branch.go:15:  if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
session/git/worktree_branch.go:24:  if err := repo.Storer.RemoveReference(worktreeRef.Name()); err != nil && err != plumbing.ErrReferenceNotFound {
session/git/worktree_ops.go:43:     if _, err := repo.Reference(branchRef, false); err == nil {   <- bug site 1
session/git/worktree_ops.go:235:    if _, err := repo.Reference(branchRef, false); err == nil {  <- bug site 2
session/git/ops.go:76,109,113,166:  repo.Reference(..., true) — different call sites, error checked as plain err != nil failure (no exists/not-exists branching)
session/git/util.go:236:            repo.Reference(plumbing.HEAD, false) — same, straight failure path
```

Findings:
- No existing "branch exists" helper function anywhere in `session/git/` — nothing to
  call instead of hand-rolling.
- `worktree_branch.go`'s `cleanupExistingBranch` already compares directly against
  `plumbing.ErrReferenceNotFound` (with `!=`, not `errors.Is`) at two sites for the
  identical sentinel — i.e. the codebase already treats this sentinel as the
  not-found signal elsewhere, just via direct equality rather than `errors.Is`.
  Since `plumbing.ErrReferenceNotFound` is never wrapped on this path (confirmed
  above — `dotgit.Ref`/`packedRef` return it directly, not via `fmt.Errorf("%w", ...)`),
  direct `==` and `errors.Is` are behaviorally equivalent here today; `errors.Is` is
  still the right choice going forward per `golang-error-handling` conventions (safe
  if the error ever gets wrapped upstream) and for consistency with the fix being
  proposed. This is also a decent signal to flag as a possible small follow-up:
  `worktree_branch.go:15,24` could be switched to `errors.Is` too for consistency,
  though that's out of scope for the 2-line fix requested here.
- No third-party dependency exposes this more directly than go-git already does (see
  §1) — go-git is already the dependency in use; nothing new is needed.

**Conclusion:** the 2-line `errors.Is(err, plumbing.ErrReferenceNotFound)` fix at both
`worktree_ops.go:43` and `:235` is correct and sufficient. No new dependency, no
subshell, no existing internal helper was overlooked.
