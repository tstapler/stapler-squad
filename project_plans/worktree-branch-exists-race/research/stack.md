# Research: go-git `Reference()` error handling for worktree-branch-exists-race

## 1. Sentinel error type and how it's returned

`plumbing.ErrReferenceNotFound` is a plain sentinel, not a typed/wrapped error:

```go
// $GOMODCACHE/github.com/go-git/go-git/v5@v5.14.0/plumbing/reference.go:32
ErrReferenceNotFound = errors.New("reference not found")
```

`go.mod` pins `github.com/go-git/go-git/v5 v5.14.0` (go.mod:17).

Call chain for `repo.Reference(branchRef, false)` (unresolved lookup, which is what both
call sites in `worktree_ops.go` use):

```
Repository.Reference(name, resolved=false)   // repository.go:1502
  -> r.Storer.Reference(name)                // no wrapping
       -> filesystem.ReferenceStorage.Reference(n)   // storage/filesystem/reference.go:21
            -> d.dir.Ref(n)                          // dotgit.go
                 -> packedRef(name) returns `plumbing.ErrReferenceNotFound` directly
                    (dotgit.go:798) when the ref isn't found in either loose or packed refs
```

At every layer in the filesystem-backed storer (the one `git.PlainOpen` uses, and what
`GitWorktree` uses), the sentinel is returned **unwrapped** — no `fmt.Errorf("...: %w", ...)`
anywhere in this path. So a plain `err == plumbing.ErrReferenceNotFound` comparison and
`errors.Is(err, plumbing.ErrReferenceNotFound)` are behaviorally equivalent for this specific
call path today.

Any *other* non-nil error from this chain (I/O error opening `packed-refs`, permission error,
a lock-contention error surfaced by the OS during a concurrent `git worktree add`) is **not**
`ErrReferenceNotFound` and must not be treated as "branch doesn't exist."

## 2. `errors.Is` vs direct `==` comparison

**Use `errors.Is(err, plumbing.ErrReferenceNotFound)`**, not `==`, even though they're
equivalent for the current filesystem-storer path. Rationale:
- `errors.Is` is unwrap-aware, so it stays correct if any future go-git release, or any
  wrapping this repo's own code does before returning the error, adds a `%w` wrap. `==` breaks
  silently the moment that happens — a correctness regression that wouldn't show up in a diff
  review of the call site itself.
- It's the pattern already used in the newer/more careful call site in this same codebase
  (see #3) and is the golang-error-handling skill's standard guidance for sentinel-error
  checks.

## 3. Existing precedent in this repo — inconsistent, two patterns present

Two different patterns already exist for this exact sentinel:

- `session/git/worktree_branch.go:15,24` (`cleanupExistingBranch`) — **older/looser pattern**,
  direct `!=` comparison:
  ```go
  if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
  ```
- `session/unfinished/gogit_vcs_reader.go:751` (`resolveHeadTreeHashes`) — **preferred
  pattern**, `errors.Is` with a documented sentinel-result comment:
  ```go
  headRef, headErr := repo.Head()
  if headErr != nil {
      if errors.Is(headErr, plumbing.ErrReferenceNotFound) {
          return nil, nil //nolint:nilnil // documented sentinel: no HEAD yet (unborn branch)...
      }
      return nil, fmt.Errorf("head: %w", headErr)
  }
  ```

The bug's two call sites (`session/git/worktree_ops.go:43` in `Setup()` and
`worktree_ops.go:235` in `setupNewWorktree()`) currently do neither — they collapse all errors
to "branch doesn't exist":
```go
branchRef := plumbing.NewBranchReferenceName(g.branchName)
if _, err := repo.Reference(branchRef, false); err == nil {
    branchExists = true   // or: proceed straight into branch-creation path
}
```
Fix should follow the `gogit_vcs_reader.go` shape: `errors.Is` check for the not-found case,
and propagate/log any other error instead of silently falling through to "does not exist."

## 4. Stays within go-git idioms — no subshell needed

`.claude/rules/prefer-go-git-over-subshells.md` directs preferring `go-git` over
`safeexec.CommandContext("git", ...)` whenever go-git can do the job. `Reference()` +
`errors.Is(err, plumbing.ErrReferenceNotFound)` is a pure go-git API check — no git CLI
subshell is needed or justified for this fix. Both call sites already use `git.PlainOpen`
and `repo.Reference`; the fix only changes how the returned `error` is classified.

## 5. go-git version / known issues

`v5.14.0` (go.mod:17), current at time of research. No known open go-git issues specific to
`ErrReferenceNotFound` misdetection in this version — the sentinel and its unwrapped
propagation from the filesystem storer (per #1) is straightforward and long-standing
behavior in go-git's `storage/filesystem` and `plumbing` packages; the bug here is entirely
in this repo's own call-site error handling, not in go-git.

## Recommended fix shape

```go
branchRef := plumbing.NewBranchReferenceName(g.branchName)
_, err = repo.Reference(branchRef, false)
switch {
case err == nil:
    branchExists = true
case errors.Is(err, plumbing.ErrReferenceNotFound):
    // branch genuinely does not exist — fall through to creation path
default:
    // transient/genuine error (lock contention, I/O) — do not silently treat as "no branch"
    return fmt.Errorf("failed to check branch reference %s: %w", g.branchName, err)
    // (Setup()'s goroutine variant: send err on errChan instead of returning directly)
}
```

Apply the same shape at both call sites (`worktree_ops.go:43` inside the `Setup()` goroutine —
send the error on `errChan` rather than swallowing it — and `worktree_ops.go:235` in
`setupNewWorktree()`, which can return the error directly since it's not inside a goroutine).
