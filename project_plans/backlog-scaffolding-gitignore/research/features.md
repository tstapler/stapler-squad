# Research: Features & Edge Cases — backlog-scaffolding-gitignore

Agent 2 (Features). Confirmed root cause from requirements.md not re-derived here.

## 1. Existing `--git-common-dir` resolution: `session/git/util.go:148-161`

`findMainRepoPathForWorktree` (`session/git/util.go:150-161`) already does exactly the
right thing and should be the pattern `addWorktreeExcludes` copies:

```go
cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
cmd.Dir = worktreePath
out, err := cmd.Output()
commonDir := strings.TrimSpace(string(out))
return filepath.Dir(commonDir), nil
```

Its doc comment (lines 133-149) is explicit that this resolves "the shared .git
directory a worktree and its main repo both point to" — i.e. it already treats
`--git-dir` and `--git-common-dir` as *not* equivalent for worktrees, which matches
the confirmed root cause. **Correction to the research-question's framing**: this
function does NOT claim `--git-dir`/`--git-common-dir` equivalence for plain repos —
it doesn't mention `--git-dir` at all. It was written specifically to avoid a
different, `--git-dir`-adjacent bug (`findGitRepoRoot`'s directory-walk logic
misfiring on a worktree's gitlink file, corrupting the worktree — see the full
comment for that incident). There is no existing code comment anywhere in the repo
that asserts `--git-dir == --git-common-dir` for plain (non-worktree) repos; that
equivalence is simply a property of git itself (a plain repo's own `.git` is both its
git-dir and its common-dir), which is why the existing `addWorktreeExcludes` tests
(see §5) pass today despite exercising the buggy code path.

**Implication for the fix**: `addWorktreeExcludes` should switch from
`git rev-parse --git-dir` to `git rev-parse --path-format=absolute --git-common-dir`,
mirroring `findMainRepoPathForWorktree` exactly (including the `--path-format=absolute`
flag, which sidesteps the existing manual `filepath.IsAbs`/`filepath.Join` fallback
logic at `backlog_commands.go:251-254` — that fallback becomes dead code once
`--path-format=absolute` is used and can be deleted).

## 2. `GetWorktreeDirtyPaths` edge cases — reuse, don't reimplement

The repo already has a **correct, tested** porcelain parser that must be reused
rather than reimplemented: `session/vc/git_provider.go`.

- `GitProvider.GetChangedFiles()` (`git_provider.go:163-192`) runs
  `git status --porcelain=v2 -z --untracked-files=all` and returns `[]FileChange`
  (`session/vc/types.go:64-71`: `Path`, `Status`, `IsStaged`, `OldPath` for
  renames/copies, plus numstat `Additions`/`Deletions`).
- `parsePorcelainV2Z` (`git_provider.go:213+`) is the parser. Its doc comment
  (lines 194-212) documents the exact record layout being asked about:
  - Ordinary changed entry: `1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>`
  - Rename/copy entry: `2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>` —
    **followed by one extra NUL-delimited token holding `<origPath>`**, consumed
    explicitly by the loop rather than mistaken for the next record.
  - Untracked: `? <path>`; ignored: `! <path>`; unmerged/conflict: `u <xy> ...`.
  - This is **porcelain v2**, not v1 — differs from the porcelain v1 -z format the
    research question describes (`XY <path>` with `" -> "` for renames in non-`-z`
    mode, or `path NUL origPath NUL` in v1 `-z` mode per git's own docs). The repo's
    chosen format is already the more robust v2 variant; no need to hand-roll v1
    parsing.
  - Records are NUL-separated (`-z`), so paths with spaces/special characters parse
    correctly — confirmed by the doc comment at `git_provider.go:164-170` and by a
    dedicated regression test: `session/vc/git_provider_test.go:1136` ("the
    regression case for the porcelain=v2 (non -z) bug" — i.e. there is already a
    named regression test guarding exactly this).
  - Rename handling is independently tested: `git_provider_test.go:433` (`git mv`
    staging) and `:972` (NUL-after-every-token assertion matching
    `porcelain=v2 -z`).

**Recommendation**: implement `GetWorktreeDirtyPaths` as a thin wrapper —
`vc.NewGitProvider(worktreePath)` (constructor at `git_provider.go:28`) →
`.GetChangedFiles()` → map `[]FileChange` to path strings (using `OldPath -> Path`
formatting for `FileRenamed`/`FileCopied` entries) — rather than writing a new
parser against `IsWorktreeDirty`'s plain (non-`-z`, v1) `git status --porcelain`
command. This directly satisfies acceptance criterion 2's "existing rename-parsing
correctness must be preserved/reused, not reimplemented naively."

One caveat: `GetChangedFiles`/`GetNumstat` calls `git diff --numstat` twice per
invocation in addition to `status`, which is more subprocess overhead than
`IsWorktreeDirty`'s single call. For a message-formatting path (only invoked on the
rejection branch of `request_review`, not hot-path polling) this is acceptable, but
worth flagging in the plan — the numstat calls could be skipped by not reusing
`GetChangedFiles()` wholesale, at the cost of duplicating slightly more of
`GetChangedFiles`'s orchestration.

## 3. Already-committed scaffolding + self-heal sufficiency

`UntrackScaffolding` (`session/git/scaffolding.go:49-85`) is called by
`selfHealWorktreeScaffolding` at the start of **every** scaffolding write
(`backlog_commands.go:290-298`, itself invoked from `WriteBacklogContextFile` and
`WriteSlashCommands`), and:
- Removes matching index entries via go-git (`git rm --cached` semantics — working
  tree file untouched).
- Serializes per-worktree-path via `untrackMu` (`scaffolding.go:11-24`) against
  concurrent callers within this process.
- Is directly tested for the exact "already committed before this fix shipped"
  scenario: `session/backlog_commands_test.go:336`
  (`TestWriteBacklogContextFile_UntracksPreviouslyCommittedContextFile`) and `:389`
  (`TestWriteSlashCommands_UntracksPreviouslyCommittedSlashCommandFiles`) both commit
  a stale scaffolding file first, then call the write function and assert it's no
  longer tracked afterward.

**Conclusion**: self-heal already runs unconditionally before `addWorktreeExcludes`
in the same call path, and by the time `request_review` executes (later in the
session, after scaffolding writes have already happened), any pre-existing committed
scaffolding has already been untracked. `GetWorktreeDirtyPaths` does **not** need any
separate handling for "already committed scaffolding" — it only needs to correctly
report whatever is *currently* dirty (untracked/modified/staged) at the time
`request_review` calls it, and untracked-but-not-yet-`git rm --cached`'d scaffolding
would already have been caught by `selfHealWorktreeScaffolding` earlier in the
session lifecycle. The one gap `GetWorktreeDirtyPaths` genuinely exists to cover
(per requirements.md's scope) is defense-in-depth for cases where a scaffolding file
became dirty *after* the self-heal ran but *before* the exclude took effect (or a
completely unrelated real dirty file) — not re-covering already-committed history.

## 4. Non-worktree / directory-mode / non-git graceful no-op requirement

Confirmed pattern to replicate: `UntrackScaffolding` returns `(nil, nil)` — not an
error — when `git.PlainOpenWithOptions(worktreePath, ...)` fails, i.e. the path isn't
a git repo at all (`scaffolding.go:54-57`, doc comment at `:46-48`). This is tested by
`TestUntrackTrackedScaffolding_NoErrorOnNonGitDirectory`
(`backlog_commands_test.go:484-493`, using a bare `t.TempDir()` with no `git init`).

`addWorktreeExcludes` already has an analogous best-effort pattern: it currently logs
a warning and returns early on `git rev-parse --git-dir` failure
(`backlog_commands.go:246-250`) — this must be preserved unchanged when the command
is swapped to `--git-common-dir` (same failure mode: non-git dir → `rev-parse`
non-zero exit → `cmd.Output()` returns an error → log + return, no panic, no error
propagated to the caller since `addWorktreeExcludes` has no return value at all).

For the new `GetWorktreeDirtyPaths`, the same contract applies for consistency with
its sibling functions and with acceptance criterion 4 (no behavior change for
directory-mode sessions): if `vc.NewGitProvider(worktreePath)` fails to open
(non-git dir), return `(nil, nil)` rather than an error, so a caller in
`request_review`'s rejection path doesn't itself need special-casing for non-git
sessions — mirroring how `IsWorktreeDirty` today is only called when
`wt.WorktreePath != ""` (`tools_backlog.go:384`) and treats a `dirtyErr` as "don't
reject" rather than surfacing the error, i.e. the caller already tolerates
best-effort failure silently.

Verified: `vc.NewGitProvider(path)` (`git_provider.go:28-37`) calls
`FindVCSRoot(path, VCSGit)` and returns an **error** — `"not a git repository: %w"`
— for a non-git path; it does not return `(nil, nil)` the way `UntrackScaffolding`
does. So `GetWorktreeDirtyPaths` must explicitly catch that specific error and
return `(nil, nil)` itself (don't propagate it) to match the no-op contract required
by acceptance criterion 4 and to keep `request_review`'s caller free of extra
non-git special-casing.

## 5. Existing `addWorktreeExcludes` test coverage — does NOT cover the real bug

`session/backlog_commands_test.go` has no test named `TestAddWorktreeExcludes*`
directly, but `addWorktreeExcludes` is exercised transitively through
`selfHealWorktreeScaffolding` inside every `WriteBacklogContextFile`/
`WriteSlashCommands` call in that file — including the two self-heal tests above.
**All of them use `setupTestGitRepo`** (`backlog_commands_test.go:20-45`), which is a
**plain repo** (`git init` + one commit on `main`), never a linked worktree
(`git worktree add`). In a plain repo, `--git-dir` and `--git-common-dir` resolve to
the identical path (the repo's own `.git`), so the existing tests exercise
`addWorktreeExcludes` in a topology where the bug is *invisible* — they would pass
identically before and after the fix. This directly confirms requirements.md's
claim and satisfies research question 5: **no existing test would have caught the
`--git-dir` bug**; a new regression test against a real linked worktree is required
(acceptance criterion 1).

The codebase already has the right fixture pattern to build that test on:
`git.NewGitWorktree(repoDir, branchName)` + `.Setup()` + `.GetWorktreePath()`, used
today in `session/git/worktree_creation_test.go` (e.g. `:421`/`:435` for
`TestNewGitWorktree...IsDirty...` tests) and in
`session/review_queue_uncommitted_changes_test.go:264`
(`TestReviewQueue_UncommittedChanges_Integration`, which builds a real repo +
`git.NewGitWorktree` + `worktree.Setup()`/`defer worktree.Cleanup()`). The new
regression test for acceptance criterion 1 should follow this exact pattern instead
of `setupTestGitRepo`'s plain-repo helper: create a base repo, call
`git.NewGitWorktree`, write `.backlog-context.md` into the *worktree* path, run
`addWorktreeExcludes(worktreePath)`, then assert `git status --porcelain` (or the new
`GetWorktreeDirtyPaths`) reports clean — this is the fixture that fails today (before
the fix) and passes after.

## Summary of implications for the plan phase

- Swap `addWorktreeExcludes`'s `git rev-parse --git-dir` for
  `git rev-parse --path-format=absolute --git-common-dir`, matching
  `findMainRepoPathForWorktree` (`session/git/util.go:150-161`) exactly; the manual
  absolute-path fallback (`backlog_commands.go:251-254`) becomes dead code and can be
  removed.
- Implement `GetWorktreeDirtyPaths` as a wrapper around
  `vc.NewGitProvider(worktreePath).GetChangedFiles()` (`session/vc/git_provider.go`),
  not a new porcelain parser — reuses already-tested rename/space-handling logic.
  `vc.NewGitProvider` returns an error (not `(nil, nil)`) for a non-git path
  (`git_provider.go:28-37`), so `GetWorktreeDirtyPaths` must catch that error
  explicitly and convert it to a no-op `(nil, nil)` return.
- No special-case needed for pre-existing committed scaffolding in the new function —
  `selfHealWorktreeScaffolding`'s `UntrackScaffolding` call already runs earlier in
  every scaffolding-write path and is independently tested for that scenario.
- The new regression test (acceptance criterion 1) must use the
  `git.NewGitWorktree`/`.Setup()` linked-worktree fixture pattern already established
  in `session/git/worktree_creation_test.go` and
  `session/review_queue_uncommitted_changes_test.go`, not the plain-repo
  `setupTestGitRepo` helper in `backlog_commands_test.go` — the latter cannot
  distinguish `--git-dir` from `--git-common-dir` and would not have caught this bug.
