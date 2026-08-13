# Research: Build vs. Buy — backlog-scaffolding-gitignore

Agent 6. Question: for (1) parsing `git status` output into a dirty-path list, (2)
resolving the worktree's common git dir, and (3) reusing internal helpers — is there
an existing library or internal helper this fix should reuse instead of hand-rolling
new code? Bias: smallest correct change over a rewrite, per requirements.md scope.

## 1. Porcelain status parsing (for `GetWorktreeDirtyPaths`)

**`go.mod` has no dedicated git-porcelain-parsing library.** The only git-related
dependency is `github.com/go-git/go-git/v5` (plus its `go-billy` filesystem
abstraction) — no `go-gitparse`, no vendored porcelain parser as a separate module.

**But the codebase already has a hand-rolled, rename-aware, NUL-safe parser — reuse
it rather than writing a new one.** `session/vc/git_provider.go`:

- `GetChangedFiles()` (`git_provider.go:163`, exported) runs
  `git status --porcelain=v2 -z --untracked-files=all` and returns `[]FileChange`
  (fields include `Path`, `Status` — `FileUntracked`/`FileModified`/`FileConflict`
  etc. — and `IsStaged`).
- `parsePorcelainV2Z()` (`git_provider.go:213`, package-private) is the actual parser.
  Its doc comment explains *why* `-z` matters: without it, git only quotes "unusual"
  paths, so a plain-ASCII filename containing a literal space parses ambiguously;
  with `-z`, records are NUL-separated and rename `origPath` tokens are consumed
  explicitly as a continuation of the preceding `"2 "` record rather than
  misread as a new record.
- This parser already has regression coverage for exactly the correctness class the
  requirements doc calls out: `session/vc/git_provider_test.go:433` covers `git mv`
  staged-rename output (porcelain `"2 "` record), and
  `git_provider_test.go:1136` is a named regression test for "the porcelain=v2
  (non -z) bug."
- `NewGitProvider(path string)` (`git_provider.go:28`) takes an arbitrary path — not
  tied to a singleton — so `vc.NewGitProvider(worktreePath).GetChangedFiles()` is a
  direct, arbitrary-worktree-path call. `session/vc` is already imported from
  `server/services/` (search_service.go, workspace_service.go) and from
  `session/workspace/`, and does not import `session` — no import-cycle risk from
  `session/backlog_review.go` (package `session`) depending on it.

Compare against `IsWorktreeDirty` (`session/backlog_review.go:626`): it shells out to
plain `git status --porcelain` (no `-z`, no `=v2`) and only checks for non-empty
output — it was never meant to enumerate paths, just answer a boolean, so it has no
rename-awareness and isn't a base to extend.

### Options for `GetWorktreeDirtyPaths`

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Reuse `vc.GetChangedFiles()` and map `[]FileChange` → path list** | Rename-safe, NUL-safe, already tested against exactly the bug classes (spaces in filenames, staged renames) that a naive reimplementation would risk reintroducing; zero new parsing code; `vc` already an approved cross-package dependency | Pulls in `numstat` insertion/deletion counting the caller doesn't need (`GetChangedFiles` calls `getNumstat` twice) — extra subprocess calls beyond the minimum; `FileChange`'s `Status` enum is git-specific (fine here, worktrees in this project are always git) | **Recommended** |
| **Hand-roll a new porcelain parser in `session/`** | None over the existing one | Directly reimplements logic already written, tested, and hardened against real bugs (the `-z` regression, rename `origPath` continuation) in `session/vc`; violates AC #2's explicit "not reimplemented naively"; duplicate maintenance burden | **Not recommended** |
| **`IsWorktreeDirty`'s subshell approach, extended to capture paths** | Minimal diff to a file already touched by this fix | `--porcelain` (no `-z`, no `=v2`) has the exact quoting ambiguity `parsePorcelainV2Z`'s doc comment warns about — extending it means either accepting that ambiguity or re-deriving the same `-z` fix `vc` already has | **Not recommended** (superseded by reuse option) |

If `GetChangedFiles()`'s numstat overhead is judged unnecessary for this call site, a
lighter option is calling the unexported `parsePorcelainV2Z` by promoting it to
exported (`ParsePorcelainV2Z`) and having `GetWorktreeDirtyPaths` run its own
`git status --porcelain=v2 -z --untracked-files=all` + call the exported parser
directly, skipping numstat. Either way, the **parsing logic itself** should be
reused from `session/vc`, not rewritten.

## 2. go-git's `Worktree.Status()` vs. subshell — for the boolean and path-list cases

**Do not use go-git's native `Worktree.Status()` here.** This codebase has direct,
documented, empirical evidence against it for exactly this kind of check:

`session/unfinished/gogit_vcs_reader.go:861` (`HasUncommitted`'s doc comment):

> "This avoids the 1.85 GB allocation caused by `wt.Status()`, which hashes every
> modified file in full."

That function (`HasUncommitted`, `gogit_vcs_reader.go:864`) exists specifically
*because* `go-git`'s `Worktree.Status()` was measured to allocate ~1.85 GB by hashing
full file contents for every modified file, rather than using the stat/mtime
short-circuit the git CLI (and this custom 3-phase index/stat/untracked-walk
implementation) uses. It also documents a second limitation at line 932: the
mtime-stat approach "doesn't read `.gitignore` files (callers that need full
`.gitignore` support should use `wt.Status()")` — i.e. even this codebase's own
go-git-avoidance code acknowledges `wt.Status()` is the *more correct* but
*prohibitively expensive* option.

This directly explains — and validates — the requirements doc's own "out of scope"
call: *"Changing `IsWorktreeDirty`'s underlying mechanism away from shelling out to
`git status --porcelain` (go-git's `Worktree.Status()` is a plausible alternative but
out of scope here — no evidence it's needed to fix this bug)."* The `session/unfinished`
package is real, actively-used code (imported by `server/services/backlog_service.go`,
`server/services/unfinished_work_service.go`, `server/actuator.go`, etc. — not dead
code), so this finding is load-bearing precedent, not speculative.

`.claude/rules/prefer-go-git-over-subshells.md`'s own carve-out applies here almost
verbatim: it's fine to shell out "when go-git genuinely can't do the job," and its
example hybrid pattern (`getHeadCommitSHA`, `session/git/util.go`) is "try go-git
first, fall back to CLI only for a specific, documented failure mode." Here the
"specific, documented failure mode" is the 1.85 GB allocation already measured and
written down in this repo for the exact operation (`Worktree.Status()`) under
discussion — so keeping the subshell is following that rule, not violating it.

### Options

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Subshell (`git status --porcelain=v2 -z`, via `vc.GetChangedFiles()`)** | Matches documented, measured behavior of this exact codebase (`gogit_vcs_reader.go:861`); matches existing `IsWorktreeDirty`'s approach — consistent within `backlog_review.go`; requirements doc explicitly keeps `IsWorktreeDirty`'s mechanism out of scope, so a *new* function using a *different* mechanism for the same worktree in the same review-gate flow would be an inconsistency, not a simplification | One more subprocess per `request_review` rejection path (rejections are not hot-path/high-frequency, so this is not a performance concern in practice) | **Recommended** |
| **go-git `Worktree.Status()`** | Native Go API, in-process, "prefer go-git" rule's general default | Documented 1.85 GB allocation risk from hashing every file's full content, measured in this exact codebase for this exact kind of check; explicitly out of scope per requirements.md; would also diverge from `IsWorktreeDirty`'s mechanism for the same worktree, so a genuinely-dirty worktree could disagree between the boolean check and the path-list check if their underlying mechanisms differ (e.g. `.gitignore` handling differences between go-git's `Status()` and the CLI, per `gogit_vcs_reader.go:932`) | **Not recommended** |

## 3. Existing internal helpers to adapt for `--git-common-dir` resolution

**Yes — the exact resolution this fix needs already exists, correctly, in three
places in this codebase.** This overlaps with Agent 1/Stack's likely finding;
noting briefly rather than duplicating:

- **`session/git/util.go:150-161`**, `findMainRepoPathForWorktree` (package-private):
  runs `git rev-parse --path-format=absolute --git-common-dir` with `cmd.Dir =
  worktreePath`, trims the output, and returns `filepath.Dir(commonDir)`. Its doc
  comment (`util.go:133-149`) is itself a warning about *why* worktree common-dir
  resolution needs care — it explicitly documents a production bug where a naive
  go-git-based approach misresolved a worktree's `.git` redirect file and corrupted
  the worktree. This is strong precedent for preferring the subshell here too.
- **`session/repo_path.go:396-419`**, `GetMainRepoPath`: same
  `git rev-parse --git-common-dir` command (via `safeexec.CommandContext`, `-C
  path` instead of `cmd.Dir`), with its own absolute/relative path handling.
- **`session/vcs/detect.go:130`**: uses plain `--git-dir` (not `--git-common-dir`) for
  a different purpose (repo-root detection, not exclude-file resolution) — not a
  fit to adapt, but confirms the codebase already distinguishes the two flags
  deliberately elsewhere.

**go-git equivalent check:** go-git's public API has no direct
`rev-parse --git-common-dir`-equivalent that returns a plain path string. It does
support `git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true,
EnableDotGitCommonDir: true})`, which correctly *opens* a worktree against its
shared common-dir storage internally — but the filesystem-resolution logic that
computes the common-dir path
(`dotGitToOSFilesystems`/`dotGitCommonDirectory` in go-git's own `repository.go`) is
unexported. This codebase already had to reproduce that unexported logic once, for a
different, heavier feature: `session/unfinished/gogitstore/open.go`'s
`resolveGitFilesystems` (see its doc comment, `open.go:21-26`, which says so
explicitly: "that logic ... is unexported, so it cannot be called directly; it is
reproduced here rather than reinvented"). That reproduction exists to share an
object-store cache across worktrees for a scanning feature — a much larger surface
than "get one directory path to write one file into."

### Options

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Adapt the subshell pattern already in `session/git/util.go`'s `findMainRepoPathForWorktree` (or `repo_path.go`'s `GetMainRepoPath`)** | Exact command already proven correct in this codebase for this exact worktree topology; smallest change — same `git rev-parse --path-format=absolute --git-common-dir` invocation `addWorktreeExcludes` already almost has (it already shells out to `rev-parse`, just with the wrong flag) | `findMainRepoPathForWorktree` is package-private to `session/git` and returns the *main repo root* (one `filepath.Dir` past the common dir), not the common dir itself — `addWorktreeExcludes` needs the common dir directly (to join `info/exclude` onto it), so this is "adapt the pattern," not "call the function unchanged" | **Recommended** |
| **Reproduce go-git's unexported common-dir resolution (à la `gogitstore/open.go`)** | Stays in-process, no subshell | Significant complexity for this fix's scope — `resolveGitFilesystems` exists to solve a harder problem (shared object-store filesystems across worktrees); using it (or copying it) just to get a directory path is a large, unjustified increase in surface area for a "one flag was wrong" bug fix | **Not recommended** |
| **Write a brand-new `--git-common-dir` subshell call from scratch in `backlog_commands.go`** | Still small | No reason to — the pattern already exists twice in this codebase (`util.go`, `repo_path.go`); copying an untested-in-this-context new invocation when a proven one exists nearby adds risk for no benefit | **Viable, but reuse is strictly better** |

## Summary recommendation

For the smallest correct change, given this is a small, contained, low-risk infra fix:

1. **`addWorktreeExcludes`**: change the `git rev-parse --git-dir` call to
   `git rev-parse --path-format=absolute --git-common-dir`, mirroring the existing,
   proven invocation in `session/git/util.go:153` / `session/repo_path.go:401` — no
   new abstraction needed, just the corrected flag plus (optionally) the
   `--path-format=absolute` flag those two call sites already use to sidestep the
   relative/absolute path handling `addWorktreeExcludes` currently does by hand.
2. **`GetWorktreeDirtyPaths`**: implement it on top of `session/vc`'s existing
   `GetChangedFiles()` / `parsePorcelainV2Z` machinery rather than writing a new
   parser — this satisfies AC #2's "not reimplemented naively" requirement directly,
   for free.
3. **Keep the subshell approach** (no go-git `Worktree.Status()`) for both the
   existing `IsWorktreeDirty` and the new `GetWorktreeDirtyPaths` — this codebase has
   already measured and documented why `wt.Status()` is the wrong tool for this job
   (`session/unfinished/gogit_vcs_reader.go:861`), and the requirements doc already
   scopes that change out.

No new third-party dependency is warranted for any of the three sub-questions.
