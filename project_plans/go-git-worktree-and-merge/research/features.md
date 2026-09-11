# Research: Features — go-git-worktree-and-merge

Scope per requirements.md: replace `session/git/worktree_ops.go`'s subprocess
`git worktree add/remove/list/prune/unlock` and `ops.go`'s `MergeMainIntoWorktree`
(fetch + `merge --no-edit` + conflict detection + `merge abort`) with pure-Go
implementations on `go-git` primitives.

## 1. Industry precedent — what similar reimplementations had to handle

### JGit (Java) — worktree
JGit only gained **read** support for worktrees in JGit 7.0 (2024) — it can
recognize and open a linked worktree, but has no `worktree add`-equivalent
write path at all. That confirms this project's premise: even a decade-mature,
well-funded pure-language git reimplementation didn't attempt worktree
*creation* until very recently, and still doesn't fully own it. The part JGit
did implement is instructive for the biggest risk in this project (on-disk
interop):
- A linked worktree's `$GIT_DIR` is a private subdirectory under the main
  repo's `$GIT_DIR/worktrees/<name>`, and `$GIT_COMMON_DIR` points back to the
  main repo — JGit had to model *two* directory roots per repository instance
  (common vs. per-worktree), not one, which is a structural change to how a
  "repository" is represented, not just a new method.
- Per-worktree config (`extensions.worktreeConfig`, `config.worktree`) is a
  second config-resolution edge case: whether `core.bare`/`core.worktree`
  apply per-worktree or repo-wide depends on this extension flag, and gitdir
  layout differs between the main worktree (`.git/config.worktree`) and a
  linked one (`.git/worktrees/<name>/config.worktree`).
- [JGit worktree read support (7.0 release notes)](https://github.com/eclipse-egit/egit/issues/154), [jgit-dev mailing list: "Git worktree support - help needed"](https://www.eclipse.org/lists/jgit-dev/msg03648.html)

**Applies to this project**: `worktree_ops.go`'s callers don't need per-worktree
config (out of scope per requirements.md), but the two-directory-root
structural point is real: any new Go worktree layer needs an explicit
`commondir` vs `gitdir` distinction in its own types, not just "the repo path"
and "the worktree path" as two strings — that's exactly what the admin-file
constraint (Constraints section) is pointing at.

### libgit2 (C) — `worktree.c`
Fetched and read `src/libgit2/worktree.c` directly (main branch). Concrete
things it checks that a naive implementation would miss:
- `is_worktree_dir()` validates a directory is a real worktree admin dir by
  requiring **three** specific files to exist together: `commondir`, `gitdir`,
  `HEAD` — not just presence of the directory.
- `git_worktree_validate()` separately validates parent dir, common dir, *and*
  worktree dir all still exist, with distinct error messages per failure —
  i.e., three independent existence checks, not one.
- Adding a worktree explicitly rejects a branch that's already checked out
  elsewhere (`git_branch_is_checked_out()` → `"reference %s is already checked
  out"`) — the actual git behavior this repo's `worktreeAlreadyRegisteredForBranch`
  reimplements in the current subprocess code, so it's already handled
  correctly today, but is a real invariant a reimplementation could silently
  drop.
- All directory creation uses `GIT_MKDIR_EXCL` (fails if it already exists) —
  the C library treats "directory already there" as failure-worthy, not
  silently overwritable; the Go replacement needs the equivalent explicit
  exclusivity, not a bare `os.MkdirAll`.
- `git_worktree__read_link()` handles **both relative and absolute** paths
  read out of the `gitdir`/`commondir` files depending on how git wrote them —
  a real-git-created worktree isn't guaranteed to use one form consistently
  across git versions/configs.
- `git_worktree_lock()` refuses to lock an already-locked worktree
  (`GIT_ELOCKED`), and locked status is orthogonal to prunability
  (`GIT_WORKTREE_PRUNE_LOCKED` flag must be explicitly passed to prune through
  a lock).
- [libgit2 worktree.c source](https://raw.githubusercontent.com/libgit2/libgit2/main/src/libgit2/worktree.c), [git_worktree_is_prunable docs](https://libgit2.org/docs/reference/main/worktree/git_worktree_is_prunable.html)

### Real git's own `worktree.c` — administrative file formats (the actual interop target)
Fetched `git/git`'s `worktree.c` directly, since this project's constraint is
byte-compatibility with **real git**, not libgit2's independent implementation:
- `gitdir` file: single line, `"<path>/.git"`, git strips both `\n` and `\r`
  on read (`write_worktree_linking_files()`/read side) — a reimplementation
  that doesn't trim `\r` will misbehave on any file touched from a
  Windows-adjacent tool, and one that doesn't write a trailing newline the way
  git does risks another tool's naive parser choking (not git's own reader,
  which is tolerant, but `gh`/scripts might not be).
- `commondir`: path back to the *main* repo's real `.git`, used to resolve
  shared refs/objects — get this wrong and every ref looked up from inside the
  linked worktree resolves against the wrong object database.
- `locked`: file's mere *existence* means locked; its *content* (if any) is
  just a human-readable reason string, read by `worktree_lock_reason()`.
  Locked worktrees are unconditionally excluded from `should_prune_worktree()`
  regardless of the index-mtime expiry check below.
- `index`: not just the worktree's staging area — `should_prune_worktree()`
  also treats this file's **mtime** as a grace-period signal: a worktree with
  a missing `gitdir` target is only actually prunable once `index`'s mtime is
  older than the expire timestamp (default grace period from `git worktree
  prune`'s docs). A reimplementation that prunes on first sight of a missing
  directory (no grace period) is *stricter* than real git and could delete a
  worktree real git would still consider borderline-recoverable — a behavior
  divergence "other tools that inspect it" (this project's own success
  metric) would notice.
- HEAD is resolved into the admin dir via `add_head_info()` /
  `refs_resolve_ref_unsafe()` at creation time — not written once and
  forgotten; this is the same file this codebase's `getHeadCommitSHA` already
  has a documented torn-read workaround for (see Constraints in
  requirements.md) — new code must keep using that hardened path.
- [git/git worktree.c source](https://raw.githubusercontent.com/git/git/master/worktree.c)

**Applies to this project**: the grace-period-before-prunable behavior is a
gap risk if not explicitly designed for — the current subprocess code
delegates this decision entirely to real `git worktree prune`, so today's
codebase gets this correctly "for free." A pure-Go prune implementation must
reimplement `should_prune_worktree()`'s two-part check (missing target AND
past expiry), not just "directory doesn't exist ⇒ prune."

### gitoxide (Rust)
`gix-worktree` crate exists but as of the current release only covers
checkout/switch/restore/reset (working-tree *state* management), not
`worktree add`/multi-worktree administration — gitoxide's crate-status doc
lists worktree creation as still open. Merge is further behind: `gix-merge`
has been refactored toward matching git's modern `merge-ort` algorithm more
closely, but full merge workflows (as opposed to low-level tree-merge
primitives) are still described as in-progress, not complete, as of this
research. [gitoxide crate-status.md](https://github.com/GitoxideLabs/gitoxide/blob/main/crate-status.md)

**Takeaway across all three**: no pure-language git reimplementation
(JGit, gitoxide) has shipped *both* multi-worktree management and 3-way merge
to the level real git users depend on — JGit still lacks worktree write
support entirely, and gitoxide's merge is pre-1.0/still-converging on
`merge-ort` parity years into the project. This validates requirements.md's
"Large (3-6 weeks)" appetite and its "Rabbit Holes" framing (scope
deliberately narrower than full git parity) rather than suggesting either
project's approach can be adopted wholesale — libgit2 (a decade-old, widely
deployed C library) is the only one of the three with genuinely mature
worktree+merge, and its C source is exactly what's cited above as the
concrete edge-case reference, alongside real git's own `worktree.c` for the
on-disk format itself.

### libgit2's three-way merge (`merge.c`) — algorithm shape for stage-1/2/3 conflicts
Fetched and read `src/libgit2/merge.c` (main). Directly relevant to the Open
Question "does go-git's index-writing API support representing a real 3-way
conflict faithfully":
- Conflicts are represented as index entries at **stage 1 (ancestor), stage 2
  (ours), stage 3 (theirs)** — `git_index_conflict_add()` populates all three
  simultaneously per conflicting path; a partially-populated conflict (e.g.
  only stages 1+2, no stage 3, for an add/add conflict) is a distinct, valid
  case, not a bug.
- Detection walks three tree iterators together (`queue_difference()`),
  classifying each difference via `merge_diff_detect_type()` into named
  categories (both-added, both-modified, modified/deleted, etc.) before
  deciding conflict-vs-trivial-resolution — "trivial resolution" (only one
  side changed, or both sides changed identically) auto-resolves without
  ever touching the conflict stages.
- Rename detection is a real two-pass system (exact-hash match, then
  similarity-threshold inexact match, default 50%) with an explicit
  candidate-count cap (default 200) past which inexact detection is skipped
  entirely for performance — i.e., real git/libgit2 do not attempt full
  O(n²) rename matching on large diffs; they degrade to "these are separate
  add+delete, not a rename" once it would get too expensive.
- Directory/file (D/F) conflicts and gitlink (submodule, `S_ISGITLINK`) and
  symlink/regular-file mode-type mismatches are explicitly rejected from
  automatic content merging — they always become conflicts, never
  auto-resolved.
- [libgit2 merge.c source](https://raw.githubusercontent.com/libgit2/libgit2/main/src/libgit2/merge.c), [libgit2 rename-detection PR #4202](https://github.com/libgit2/libgit2/pull/4202)

**Applies to this project**: requirements.md's Scope explicitly excludes
octopus merges, `.gitattributes` merge drivers, and submodule-aware merging —
consistent with libgit2's own D/F-conflict/gitlink handling being "always
conflict, never merge," so this project can adopt the same blanket rule
(gitlink or mode-type mismatch ⇒ immediate conflict) rather than needing to
special-case it further. Rename handling is explicitly called out in
requirements.md's Rabbit Holes as an open decision — libgit2's answer (treat
as a first-class feature with a real similarity algorithm, not an
afterthought) is the higher-effort path; given this project's actual call
pattern is CI-bot-style `origin/main` → session-branch merges (not long-lived
feature branches with heavy renames), treating a rename+modify collision as a
conflict rather than building rename detection is a defensible scope cut,
but should be an explicit planning decision, not a silent gap.

## 2. Current codebase edge cases a reimplementation could regress

Read `session/git/worktree_ops.go` in full (699 lines) and `session/git/ops.go`'s
`MergeMainIntoWorktree` (lines 789-877) plus their direct dependencies
(`worktree.go`, `worktree_lock.go`, `util.go`).

### Worktree lifecycle (`worktree_ops.go`)
- **Cross-process + intra-process locking is two-layered by design**
  (`worktree_lock.go`): a `sync.Mutex` for intra-process exclusion *and* a
  `flock.Flock`-backed file lock (outside the repo, keyed by SHA-256 of the
  repo's absolute path) for cross-process exclusion. `Setup`, `Remove`, and
  `Prune` all serialize through `WithRepoWorktreeLock`. A reimplementation
  that only takes an in-process mutex (easy to do if go-git's own repo object
  gives a false sense of "it's just an in-memory operation now") would
  reintroduce the exact cross-OS-process race this locking exists to prevent
  — this repo runs the server as one process but backlog automation and a
  human's manual `git` commands are separate processes touching the same
  `.git/worktrees/`.
- **Ground-Truth Re-Query (ADR-001), not error-text matching.** Every
  self-heal path (`branchExistsAfterAddFailure`, `findLiveWorktreeForBranch`)
  responds to a failed `worktree add` by re-querying real git state (open a
  fresh repo, check `worktree list --porcelain` / branch refs) rather than
  parsing the failed command's stderr — because stderr wording varies by git
  version/locale and doesn't cover a timeout-killed subprocess ("signal:
  killed"). The go-git reimplementation removes the subprocess (so "stderr
  wording" stops being the relevant axis), but the *underlying race* — two
  concurrent sessions computing the same deterministic branch name
  (`backlogWorkBranchSlug`) and both attempting to create it — is unchanged
  and still needs a retry-and-recheck loop, just against go-git's own error
  types instead of exec exit codes.
- **A worktree can be git-registered but not present on disk** (a stale
  `worktree list` entry for a directory an external `rm -rf` deleted).
  `worktreeAlreadyRegisteredForBranch` and `findLiveWorktreeForBranch` both
  explicitly `os.Stat` the path before trusting git's registration — this is
  the codebase's own handling of "worktree deleted out from under it"
  (see Q4 below); it must carry over verbatim or the reimplementation will
  silently hand back a path that doesn't exist.
- **A worktree can be `locked` mid-checkout by an interrupted `worktree add`**
  (killed by `runGitCommand`'s 30s timeout under load), leaving a
  half-populated directory. `worktreeAlreadyRegisteredForBranch` explicitly
  excludes locked worktrees from reuse (`isWorktreeLocked`), forcing an
  unlock+remove+recreate cycle instead of silently handing back the broken
  checkout — covered by `TestWorktreeSetup_RecreatesLockedInterruptedWorktree`.
  This is git's own `locked` file semantics (see §1's git/git findings) and
  must be reimplemented, not dropped, since a Go worktree-add that gets
  interrupted (process killed, OOM) mid-checkout leaves the exact same kind
  of half-populated state.
- **Branch deletion is deliberately never automatic.** `Cleanup()`'s doc
  comment cites a real prior bug (`docs/tasks/backlog-feature-improvement.md`:
  "stop_session silently deletes the git branch") — removal only removes the
  *worktree*, never the branch ref, because a branch can hold commits that
  exist nowhere else. A reimplementation must not "helpfully" add branch
  cleanup via go-git's `RemoveReference` as part of worktree removal.
  `CleanupWorktrees()` (the bulk cleanup path) has the identical rule for the
  same reason.
- **Removal degrades gracefully through git-remove → manual `os.RemoveAll` →
  admin-file-only cleanup**, distinguishing "worktree not a working tree" /
  "not a git repository" / "worktree not found" errors (treated as expected,
  not failures) from other errors. A reimplementation should preserve this
  degrade-gracefully shape: worktree removal on this codebase's paths is a
  best-effort, self-healing operation (called during session teardown, where
  failing loudly blocks the user from finishing an unrelated action), not a
  strict "removal must fully succeed or return an error" contract.
- **`worktreeAlreadyRegisteredForBranch`'s reuse-in-place behavior exists
  for a specific, real product requirement**: backlog rework/reopen cycles
  reuse the same `backlog/<item>` branch and worktree path across every
  revision, and force-recreating it on every reopen previously discarded
  in-progress uncommitted state. Any reimplementation of "does a worktree
  already exist for this branch" must preserve reuse-in-place as the default,
  not "always remove and recreate" — that was an actual regression this
  code already fixed once.
- **Path canonicalization matters for identity comparisons.**
  `getWorktreeDirectory()`'s `filepath.EvalSymlinks` and
  `CanonicalizeWorktreePath` exist because git itself resolves symlinks when
  recording a worktree's path in `gitdir`, so a freshly-computed path and
  git's later-reported path for the *same* directory can differ as raw
  strings (macOS `/tmp` → `/private/tmp` is the concrete example in the
  comment). Any new path-identity check in the Go implementation needs the
  same normalization or worktree-reuse detection silently breaks on macOS.
- **Collision avoidance today is timestamp-suffix-based, not lock-based**:
  `NewGitWorktreeWithBranchAndExecutor` appends `_<hex(UnixNano)>` to every
  freshly computed worktree path — two concurrent sessions get distinct paths
  even without coordination; the `WithRepoWorktreeLock` locking instead
  protects git's own shared `.git/worktrees/` administrative metadata during
  the add/remove itself, not path uniqueness. A reimplementation must keep
  both mechanisms, not conflate them.

### Merge (`ops.go`'s `MergeMainIntoWorktree`)
- **Up-to-date detection is SHA-comparison-based, not merge-output-text
  parsing**, specifically because "Already up to date." vs "Already
  up-to-date." varies across git versions/locales — the exact same class of
  fragility the worktree self-heal logic avoids by re-querying state. Any
  reimplementation already does this "correctly" for free (it never has
  human-readable merge output to parse), but should keep the
  before/after-SHA-comparison *shape* (compute once before, once after) since
  that's also how `MergeMainResult.UpToDate` vs `.Merged` gets decided.
- **The merge is *always* aborted on conflict — the function never leaves a
  worktree in a real conflicted state.** This is the single most important
  fact for scoping the reimplementation's fidelity requirements (see §3
  below): despite requirements.md's Rabbit Holes flagging conflict-marker
  byte-fidelity as a concern "needed for interop... backlog automation's
  fix-agent prompts currently show real conflict markers to an LLM," tracing
  every real call site (below) shows `MergeMainIntoWorktree`'s own callers
  never read conflict-marker content at all — only `MergeMainResult.ConflictedFiles`
  (a `[]string` of paths). The worktree is always left clean either way.
- Reads HEAD via the codebase's own hardened `getHeadCommitSHA`, not a naive
  go-git `repo.Head()` call — per the Constraints section, this must not
  regress.

## 3. Unstated need — what the real consumer actually does with a conflict

Traced every non-test call site of `MergeMainIntoWorktree` and the
`branchReconciler` it's installed as (`session/backlog_lifecycle.go:672`):

1. **`server/services/backlog_service_triage.go`'s `syncPRBranchWithMain`**
   (proactive main→PR-branch sync before a fix session starts): on
   `result.Conflicted`, formats `result.ConflictedFiles` into a **prose
   sentence** (`"produced conflicts in:\n- %s\n\n..."`) that gets prepended
   to the fix session's context. It never re-runs a real `git merge`/`rebase`
   to produce actual conflict markers for the LLM to see.
2. **`session/git/drift.go`'s `EnsureBranchSyncedWithMain`** (proactive
   drift-correction before review, BUG-044's fix): identical pattern — joins
   `ConflictedFiles` into a message, blocks review with that summary as
   `blockedSummary`. Never surfaces raw conflict markers.
3. **`session/backlog_lifecycle_pr.go`'s `attemptPushRemediation`** (push-failed
   remediation, reconciling a non-fast-forward push rejection): same pattern
   — logs and notifies with `strings.Join(result.ConflictedFiles, ", ")` in an
   operator notification, then gives up (no auto-resolve attempted).

**Conclusion**: every real automated consumer of `MergeMainIntoWorktree`
treats `ConflictedFiles` as **a list of paths to name in a message**, not as
a signal to inspect actual conflict-marker content. The requirements doc's
own Rabbit Holes note about "real conflict markers shown to an LLM" refers to
a *different* code path: `session/git/worktree_git.go:588`'s PR-fix-prompt
template instructs a fix *agent* (running inside its own tmux session, using
real subprocess `git rebase`/`git merge` under its own control, not this
library) to "leave the conflict markers in place" if it can't resolve them —
that's the agent's own git invocation on top of the worktree this project
creates, entirely outside `MergeMainIntoWorktree`'s code path.

**Implication for scoping**: the reimplementation's fidelity bar for
`MergeMainIntoWorktree`/3-way-merge conflict detection is "produce an
accurate list of conflicting file paths, then leave the worktree exactly as
clean as the abort left it before" — not "produce byte-identical
`<<<<<<<`/`=======`/`>>>>>>>` marker text in the working tree," since nothing
in the current codebase reads that content through this function. Where
byte-exact real-git conflict-marker fidelity *does* matter is indirectly: the
fix agent's own later `git merge`/`git rebase` runs against a worktree this
project's *worktree-management* half creates — so the on-disk-interop
constraint (worktrees must be indistinguishable from real `git worktree add`
output) is what actually protects that downstream flow, not anything the
merge reimplementation itself needs to fake. This should be confirmed
explicitly in planning (a "Should ship as two independent flags" open
question already flags them as separable) since it meaningfully lowers the
merge implementation's required fidelity versus what the Rabbit Holes
section implies.

## 4. Failure modes — directory deleted out from under a worktree, colliding paths

### Directory deleted out from under a worktree (today's behavior)
Multiple independent guards already exist for this, all preserved via
explicit `os.Stat` checks rather than trusting git's/go-git's registration
alone:
- `worktreeAlreadyRegisteredForBranch` (`worktree_ops.go:267-284`): `os.Stat`s
  the worktree path first; if missing, returns `false` unconditionally
  (regardless of what `worktree list --porcelain` says), forcing a fresh
  remove+recreate rather than reusing a phantom entry.
- `findLiveWorktreeForBranch` (`worktree_ops.go:351-376`): explicitly
  documents that `git worktree list --porcelain` "still reports prunable
  entries for directories deleted out from under git" and treats a
  registered-but-missing path as "not found," continuing its retry loop
  rather than adopting the stale entry.
- `removeLocked` (`worktree_ops.go:488-550`): checks `os.Stat` before even
  attempting `git worktree remove`; if the directory is already gone, it
  skips straight to admin-file cleanup rather than erroring.
- `GitWorktree.IsDirtyErrorCacheTTL`'s doc comment (`worktree.go:56-61`)
  independently documents the same failure mode surfacing through the
  *dirty-check* path (a missing worktree directory makes `git status` fail),
  with a 60s backoff so a broken worktree isn't re-checked every poller tick.
- `TestNewGitWorktreeFromStorage_MissingDirectory_NonFatal` pins the
  contract for a fourth angle: rehydrating a `GitWorktree` from storage
  (session reconnection after restart) for a path that no longer exists on
  disk must not fail construction — canonicalization is skipped gracefully,
  the raw stored path is kept as-is, and any actual failure surfaces later
  when an operation is attempted against it.

**Required for the reimplementation**: preserve this "always verify
liveness via a real filesystem stat, never trust an admin-metadata read
alone" pattern. This maps directly onto real git's own `should_prune_worktree()`
two-part check found in §1 (missing target AND past an mtime-based grace
period) — the reimplementation needs an equivalent explicit liveness check,
not just "if the Go worktree registry has an entry, it must be valid."

### Two sessions racing to create a worktree with a colliding path
Today, this isn't actually a *path*-collision risk — `NewGitWorktreeWithBranchAndExecutor`
(`worktree.go:289-295`) suffixes every freshly computed path with
`_<hex(time.Now().UnixNano())>`, so two concurrent creates get distinct
directories without any coordination. The real race is at the **branch**
level: two concurrent spawns for the same backlog item compute the identical
*deterministic* branch name (`backlogWorkBranchSlug`), and both can pass the
"does this branch exist" check before either has created it. This is the
race `branchExistsAfterAddFailure`/`findLiveWorktreeForBranch`'s Ground-Truth
Re-Query loop (ADR-001) exists to resolve — poll real git state up to 6 times
with 300ms backoff (deliberately larger than the unrelated 20ms precedent
used for the HEAD torn-read retry, since this is waiting out a subprocess,
not an in-process read) rather than trusting either side's error text.
`TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo`
and `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`
pin this behavior today.

**Required for the reimplementation**: the same two-part defense — (1) the
`WithRepoWorktreeLock` cross-process serialization around the actual
admin-metadata write (go-git doesn't know about OS-level flock, so this
layer is unchanged regardless of what's underneath it), plus (2) a
Ground-Truth Re-Query retry loop against go-git's own state-reading calls
after a failed create, rather than assuming go-git's error types are a
reliable enough signal on their own (go-git may raise a different error
shape than the CLI for the identical underlying race — e.g. a raw ref-lock
error vs. git's own retry-tolerant wrapper — so the retry-and-recheck
strategy matters even more here, not less).

## Summary of the biggest regression risks going into planning

1. **On-disk format subtleties beyond "create the files"**: real git's
   prunability decision is a two-part mtime-based grace period, not a simple
   directory-exists check (§1); `gitdir`/`commondir` content must tolerate
   both relative/absolute forms and `\r` stripping; the `locked` file's
   existence (not content) is the semantic signal.
2. **Two-layer locking must survive the library swap.** `WithRepoWorktreeLock`
   protects against other *OS processes* (human `git`, `gh` CLI) as well as
   goroutines — go-git's in-process object model doesn't remove the
   cross-process race this exists for.
3. **The Ground-Truth Re-Query pattern (ADR-001) needs to be re-targeted at
   go-git's error/state surface, not deleted** — the underlying concurrent-
   branch-name race it defends against is unchanged by removing the
   subprocess.
4. **Conflict-marker byte-fidelity is not required by any current
   `MergeMainIntoWorktree` consumer** — all three treat conflicts as a
   `[]string` of paths for prose messages. This meaningfully de-risks the
   merge half of this project relative to what requirements.md's Rabbit
   Holes section implies, and should be confirmed/narrowed explicitly in
   planning.
5. **Never regress the "don't delete branches" and "reuse worktree in place
   on rework/reopen" behaviors** — both are fixes for real, previously-shipped
   bugs, not incidental implementation choices.
