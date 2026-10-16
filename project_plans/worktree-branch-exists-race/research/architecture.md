# Architecture: worktree-branch-exists-race

No prior `code-hotspot-analysis`/`quality:architecture-review` run exists for `session/git`
(checked via `find project_plans -path "*/research/architecture.md"` and
`grep -rl "worktree_ops"/"session/git" project_plans/` — neither turned up a prior
architecture doc for this package; the hits are all this project's own other artifacts, or
unrelated projects that merely reference `session/git` in passing). This doc is written
fresh, but it builds directly on this project's own `research/stack.md`, `research/pitfalls.md`,
and `research/build-vs-buy.md` (already unusually thorough — sentinel-error propagation
path, existing in-repo precedent, a verified test fixture) and on `implementation/plan.md`,
which already exists in this project directory and has made (and justified) the same shape
decisions this doc would otherwise derive from scratch. Where that's true, this doc cites and
confirms rather than re-deriving.

## 1. Current structure: `Setup()`'s two-goroutine pattern vs. `setupNewWorktree()`'s
   sequential check

Read in full: `session/git/worktree_ops.go` (492 lines).

**`Setup()` (lines 18–60)** runs directory creation and the branch-existence check
concurrently via a buffered `errChan := make(chan error, 2)`:

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
    errChan <- nil                                                   // ALWAYS nil today
}()

for i := 0; i < 2; i++ {
    if err := <-errChan; err != nil {
        return err
    }
}
```

Goroutine B is the bug: `err` from `repo.Reference()` is read only to decide the `== nil`
branch; any non-nil `err` (including a genuine I/O failure) is discarded, and `errChan`
unconditionally receives `nil`. So a real ref-read error is indistinguishable from "branch
absent" — `Setup()` proceeds to `setupNewWorktree()` either way.

The channel is buffered at capacity 2 and each goroutine sends **exactly once**, so
`Setup()`'s "return on first error read" pattern is race-safe today and stays race-safe after
the fix, as long as that one-send-per-goroutine invariant is preserved (confirmed in
`research/pitfalls.md` §1 — do not switch to unbuffered or add a second send). `branchExists`
is written once by goroutine B before its send and read only after the wait loop, which is a
valid happens-before via channel receive — no data race, none introduced by the fix.

**`setupNewWorktree()` (lines 217–267)** does the identical `repo.Reference()` check
sequentially, not in a goroutine, at lines 233–239:

```go
branchRef := plumbing.NewBranchReferenceName(g.branchName)
if _, err := repo.Reference(branchRef, false); err == nil {
    // Branch exists - use setupFromExistingBranch instead
    return g.setupFromExistingBranch()
}
```

Same defect, simpler control flow: no channel, so the fix here is a direct early return rather
than a channel send. This function is also reached in two ways: as `Setup()`'s continuation
when goroutine B correctly determined the branch is absent, and it re-derives `branchRef` and
re-checks `repo.Reference()` itself rather than trusting `Setup()`'s `branchExists` result —
i.e. the check is duplicated, not shared, between the two call sites today. (This
re-derivation is itself a second, narrower TOCTOU window — see §2.)

**Third site, already correct**: `session/git/worktree_branch.go`'s `cleanupExistingBranch`
(lines 15, 24), called from `setupNewWorktree()` immediately after the branch-absent branch is
taken, already distinguishes the sentinel correctly:
```go
if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
    return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
}
```
This is the in-repo reference implementation for the classification shape (direct `!=` rather
than `errors.Is`, per `research/stack.md` §2–3 — behaviorally identical today since the
sentinel is never wrapped on this path, but `errors.Is` is the more defensive/idiomatic choice
requirements.md calls for).

## 2. All callers of `GitWorktree.Setup()` — single-goroutine-per-spawn, or can two
   concurrent calls race for the same branch?

`grep -rn "\.Setup()" --include="*.go" . | grep -v _test.go` (equivalent to the requested
`sg --pattern '$X.Setup()' --lang go`) finds six non-test call sites, all going through
`*GitWorktree.Setup()` or wrapping it one level (`GitWorktreeManager.Setup()` at
`session/git_worktree_manager.go:126` is a thin forwarding call to the same method):

| Call site | Context |
|---|---|
| `session/instance_worktree.go:123` | `CreateBacklogWorktree(repoPath, branchSuffix)` — the bug-report path |
| `server/services/backlog_service_triage.go:2417` | `TriggerTriage`'s isolated triage worktree (`triage-<itemID>` branch, not `backlog/<slug>`) |
| `session/git_worktree_manager.go:126` | Forwarding wrapper, `GitWorktreeManager.Setup()` |
| `session/instance.go:966,1180,1431,1571` | Four call sites inside `Instance` lifecycle methods (session start/resume paths) |

**Each individual `Setup()` invocation runs on its own call stack** — there is no single
shared goroutine pool issuing `Setup()` calls; whichever goroutine calls `CreateBacklogWorktree`
(or an `Instance` lifecycle method) owns that one `Setup()` call's two child goroutines. So
*within* one `Setup()` call, the two-goroutine race is bounded and already reasoned about
above. The open question is *across* calls: can two independent `Setup()` invocations, each
for the *same* branch name, run concurrently?

**Yes — a residual TOCTOU exists, independently of the classification bug, and it's already
named as an explicit Unresolved Question in this project's `implementation/plan.md`** (§
"Unresolved Questions" #2, and the matching "Known Gaps" entry). Summary, confirmed by reading
`session/instance_worktree.go`'s `CreateBacklogWorktree` and its caller
`server/services/backlog_service_triage.go:1427`'s `resolveSessionPath`:

- `CreateBacklogWorktree`'s branch name is `BacklogBranchPrefix + branchSuffix` —
  deterministic given `(repoPath, branchSuffix)`, not randomized per attempt (see
  `session/instance_worktree.go`'s `BacklogBranchPrefix` doc comment, which explicitly says
  this determinism is intentional — reopen/rework spawns must land on the same branch).
- Two concurrent or back-to-back-retried `Setup()` calls for the same deterministic branch
  name (e.g. a stuck backlog item respawned twice in close succession, or a legitimate
  concurrent retry) can **both** correctly see `plumbing.ErrReferenceNotFound` — a correct
  classification, not the bug this fix addresses — and both proceed into
  `setupNewWorktree()`. Whichever's `git worktree add -b <branch>` runs second then hits git's
  own genuine "branch already exists" error, which is fatal and looks, from the caller's
  perspective, identical to the misclassification bug this fix targets.
- `setupNewWorktree()` itself widens this window further: it re-derives `branchRef` and
  re-checks `repo.Reference()` a second time (line 235) after `Setup()`'s goroutine B already
  checked it once — i.e. there are two check-then-act gaps per call (`Setup()`'s check →
  `setupNewWorktree()`'s re-check → `worktree add -b`), not one, each an independent window
  for another process's `worktree add -b` to land in between.

**This is out of scope for the current fix** (requirements.md's Non-Goals explicitly exclude
"Adding a resolveSessionPath fallback" and any retry/backoff redesign), and `plan.md` already
recommends filing it as its own follow-up rather than silently dropping it — flagging the same
recommendation here per this research task's explicit ask to note it. A per-branch-name
mutex/lock or a "retry once on `git worktree add -b` already-exists" strategy would close it,
but neither belongs in this bug-fix's diff.

## 3. Minimal-diff shared-helper shape

`implementation/plan.md`'s Pattern Decisions table has already settled this (Story 1.1.1,
Task 1.1.1a) — confirmed sound against `.claude/rules/interface-pollution-checklist.md`:

```go
// branchRefExists reports whether branchRef exists in repo, distinguishing a genuine
// "no such branch" (plumbing.ErrReferenceNotFound) from any other ref-read error.
func (g *GitWorktree) branchRefExists(repo *git.Repository, branchRef plumbing.ReferenceName) (bool, error) {
    _, err := repo.Reference(branchRef, false)
    switch {
    case err == nil:
        return true, nil
    case errors.Is(err, plumbing.ErrReferenceNotFound):
        return false, nil
    default:
        return false, fmt.Errorf("failed to check branch reference %s: %w", branchRef, err)
    }
}
```

This is a legitimate extraction, not premature abstraction, under the interface-pollution
checklist's own framing: **two call sites, identical classification logic** (the requirements
doc names both `Setup()` line ~43 and `setupNewWorktree()` line ~235 as doing the exact same
three-way `switch` inline) is the checklist's own "rung 4" case for extract-a-function, not a
speculative interface — it's a concrete method on the existing `*GitWorktree` receiver (no new
interface, no generic, no premature parameterization), matching the file's existing style of
attaching helpers to `*GitWorktree` (`worktreeAlreadyRegisteredForBranch`, `isWorktreeLocked`,
`findWorktreeForBranch` are all the same shape: private methods/functions colocated in this
file, not interfaces).

What the two call sites do **not** share, and shouldn't be forced to: *propagation*.
`Setup()`'s goroutine must send the error on `errChan` (and, per `research/pitfalls.md` §1,
`log.Error` at the detection site so the error is visible even if goroutine A's `MkdirAll`
error wins the race for the channel read that `Setup()`'s wait loop returns first); a
`log.Error` there is not paid for by `plan.md`'s alternative). `setupNewWorktree()` can
`return err` directly since it's not inside a goroutine. The helper unifies only the
classification (the part that was actually duplicated and buggy), leaving control flow at each
site — correctly, per the interface-pollution checklist's guidance against collapsing things
that legitimately differ.

## 4. Data flow: error surfacing to the UI

Traced the wrapping chain from `Setup()` outward for the bug-report path
(`CreateBacklogWorktree`, the path named in the backlog item):

```
GitWorktree.setupNewWorktree() / Setup()'s goroutine
    -> (after fix) branchRefExists returns (false, fmt.Errorf("failed to check branch reference %s: %w", branchRef, err))
session.CreateBacklogWorktree (session/instance_worktree.go:123-125)
    -> fmt.Errorf("CreateBacklogWorktree setup: %w", err)
server/services/backlog_service_triage.go:1427-1436, resolveSessionPath
    -> log.ErrorLog.Printf("[SpawnSessionFromItem] worktree creation failed for git-managed repo %s (%v)", resolvedRepo, wtErr)
    -> connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create git worktree: %w", wtErr))
```

Composed end-to-end, the new message for a genuine ref-read failure reads as:

```
failed to create git worktree: CreateBacklogWorktree setup: failed to check branch reference
refs/heads/backlog/<slug>: <underlying error, e.g. "malformed packed-ref">
```

**This is already a reasonable, readable message at the UI layer** — no separate wrapping is
needed for the new "propagated ref-lookup error" case specifically. It follows the exact same
three-hop wrapping chain (`Setup`/`setupNewWorktree` → `CreateBacklogWorktree` →
`resolveSessionPath`) that today's "branch already exists" fatal from `git worktree add -b`
already goes through and that the bug report confirms renders sensibly as an ERROR toast on
the backlog item detail page — the only change is *which* underlying error text appears at the
innermost `%w`, not the wrapping structure around it. No new wrap point needs to be added in
`resolveSessionPath` or anywhere else in this chain for AC1/AC2 to read sensibly.

The second `Setup()` caller relevant to backlog work, `backlog_service_triage.go:2417`'s
`TriggerTriage` isolated-worktree path, deliberately does **not** propagate `Setup()` failures
as fatal — it logs a warning and falls back to running triage directly in `itemRepoPath` (see
the surrounding comment block: "falls back to itemRepoPath directly if worktree creation
fails"). A propagated ref-lookup error from this fix will follow that same graceful-degradation
path, unchanged — it becomes a louder, more accurate warning log line, not a new user-facing
failure. Worth noting only because it's a second, differently-behaved consumer of the same
`Setup()` error surface; no code change is implied here.

## Summary of architectural shape for the fix

- Extract `(g *GitWorktree) branchRefExists(repo *git.Repository, branchRef plumbing.ReferenceName) (bool, error)` — classification only, shared by both call sites; propagation (channel send vs. direct return) stays separate at each site.
- `Setup()`'s goroutine: `log.Error` at detection + send the real error on `errChan` instead of the unconditional `nil`.
- `setupNewWorktree()`: `return err` directly on a non-`ErrReferenceNotFound` result.
- No new error-wrapping needed anywhere in the `CreateBacklogWorktree → resolveSessionPath` chain — the existing wrap chain already produces a sensible UI-facing message once the inner error is real.
- Residual risk, explicitly out of scope: a TOCTOU race between concurrent/retried `Setup()` calls on the same deterministic backlog branch name (two independent check-then-act windows: `Setup()`'s check and `setupNewWorktree()`'s re-check, each racing another process's `worktree add -b`). Already tracked in `implementation/plan.md`'s Unresolved Questions/Known Gaps; not this fix's job to close.
