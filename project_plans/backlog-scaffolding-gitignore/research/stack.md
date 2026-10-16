# Research: Stack/Tooling — backlog-scaffolding-gitignore

Agent 1 (Stack). Confirms/extends the coordinator's already-verified root cause
(`addWorktreeExcludes` resolves via `--git-dir` instead of `--git-common-dir`).

## Module versions (verified: `grep` in `go.mod`)

```
go 1.26.3
github.com/go-git/go-git/v5 v5.14.0
github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
github.com/go-git/go-billy/v5 v5.6.2
```

## Question 1: subshell (`safeexec`) vs go-git for the common-dir resolution

**Recommendation: keep the subshell, change only the flag `--git-dir` → `--git-common-dir`.**
Do not force this specific call onto go-git. Two independent lines of evidence:

### A. go-git *does* have an equivalent, but it's not cleanly reusable for a bare path lookup

- `git.PlainOpenOptions.EnableDotGitCommonDir` (`repository.go:781`) exists and, when
  set, makes `PlainOpenWithOptions` call the unexported `dotGitCommonDirectory(dot)`
  (`repository.go:421-445`) to resolve the common dir the same way `git
  rev-parse --git-common-dir` does (reads the `commondir` file inside the
  worktree-private gitdir, resolves it relative to that gitdir if not absolute).
- That resolution logic is **unexported** — there's no public
  `repo.CommonDir() string` accessor. The only way to reach it through the public
  API is to open a full `*git.Repository` with `EnableDotGitCommonDir: true`, then
  type-assert `repo.Storer.(*filesystem.Storage)` and call `.Filesystem()`
  (`storage/filesystem/storage.go:74`) to get back a `*dotgit.RepositoryFilesystem`.
  That filesystem's `mapToRepositoryFsByPath`
  (`storage/filesystem/dotgit/repository_filesystem.go:26-51`) auto-routes writes
  under `info/` (among `objects/`, `refs/`, `config`, etc.) to the **commondir**
  filesystem, not the worktree-private one — `info/exclude` would land in the right
  place automatically if written through that `billy.Filesystem`.
- This repo has **already solved this exact problem** by hand, without going through
  `*git.Repository` at all: `session/unfinished/gogitstore/open.go`'s
  `resolveGitFilesystems` (lines 60-106) reproduces the identical two-case logic
  (main worktree: `.git` is a dir, no commondir; linked worktree: `.git` is a
  `gitdir: <path>` pointer file, and the pointed-to dir has a `commondir` file) using
  only `os`, `path/filepath`, `strings`, and `go-billy`'s `osfs` — explicitly because,
  per its own doc comment, go-git's version "is unexported, so it cannot be called
  directly; it is reproduced here rather than reinvented." That function already
  returns `commonDirAbs string` — literally the value needed here.

### B. Why not reuse/extract that logic anyway

Doing so is *possible* but not proportionate:
- It would mean either (a) importing `session/unfinished/gogitstore` from
  `session/backlog_commands.go` — a layering smell (`gogitstore` is
  scanner-feature-flag-gated infrastructure, not a general git utility package,
  per `server/services/feature_flag_service.go:40`'s mmap-loader flag comment) — or
  (b) copy-pasting `resolveGitFilesystems`'s ~45 lines into `session/git` to get a
  bare path string, which duplicates logic that already has one canonical home.
- The operation being performed — "resolve one filesystem path, write a small text
  file to it" — is exactly the shape `.claude/rules/prefer-go-git-over-subshells.md`
  calls out as fine to leave as a subshell: *"go-git genuinely can't do the job"* in
  a *reusable, low-friction* way here (its own public API doesn't expose the path,
  only a full Repository+Storer object that then has to be laundered through a type
  assertion to reach a `billy.Filesystem` just to open one file). The rule's
  worked exception (`getHeadCommitSHAViaCLI` in `session/git/util.go`) is precisely
  this shape: go-git has a *documented, specific* limitation (there, a torn-read
  race; here, no public path accessor), so a narrow, single-purpose subshell stands
  in rather than contorting the public API or duplicating unexported internals.
- `addWorktreeExcludes` already uses `safeexec.CommandContext(ctx, "git",
  "rev-parse", "--git-dir")` today — this is the established pattern for "ask git
  for a filesystem path" in this exact function; `--git-common-dir` is a drop-in
  flag swap with no other code-shape change. `git rev-parse --git-common-dir`
  also correctly no-ops to the same value as `--git-dir` for a plain (non-worktree)
  repo, which is exactly Acceptance Criterion 4 (no behavior change for plain
  repos) — verified by design (git's own doc: for a normal repo, `--git-common-dir`
  and `--git-dir` both print `.git`; there's no separate commondir file to diverge).

**Conclusion:** change line ~244 in `session/backlog_commands.go` from
`"rev-parse", "--git-dir"` to `"rev-parse", "--git-common-dir"`. No import changes,
no new dependency, same `safeexec.CommandContext` call shape. This is the "keep the
subshell" carve-out, cited specifically: go-git's common-dir resolution is real but
unexported/not path-accessible without extra machinery this call doesn't otherwise
need, mirroring the `getHeadCommitSHAViaCLI` precedent.

## Question 2: parsing `git status --porcelain -z` for `GetWorktreeDirtyPaths`

**There is already a complete, tested, NUL-aware, rename-correct parser in this
codebase — reuse it, don't reimplement.**

- `session/vc/git_provider.go`:
  - `GetChangedFiles()` (line ~160) runs
    `g.runGit("status", "--porcelain=v2", "-z", "--untracked-files=all")` via
    `safeexec` (same package the rest of this codebase's git-status paths use).
  - `parsePorcelainV2Z(output string) []FileChange` (line ~195 onward) parses the
    NUL-delimited porcelain v2 output into `[]FileChange{Path, OldPath, Status,
    IsStaged, ...}`, correctly handling:
    - `?` untracked, `!` ignored, `1` ordinary changed, `2` rename/copy (consumes
      the extra NUL-delimited `<origPath>` token that follows a `2 ` record — the
      exact "rename-parsing" logic the requirements doc's lost prior plan referred
      to), and `u` unmerged/conflict records.
    - Paths with embedded spaces — this is a **regression-tested fix**:
      `TestGitProviderGetChangedFiles_PathWithSpace` in
      `session/vc/git_provider_test.go` (~line 1129) exists specifically because an
      earlier version used non-`-z` `--porcelain=v2` and `strings.Fields()`, which
      silently truncated paths containing literal spaces. The `-z` + fixed-field
      `SplitN` approach in the current `parsePorcelainV2Z` is the fix.
  - `FileChange`/`FileStatus` types live in `session/vc/types.go` (`FileUntracked`,
    `FileModified`, `FileRenamed`, `FileConflict`, etc., `IsStaged bool`,
    `Path`/`OldPath string`).

- By contrast, `IsWorktreeDirty` (`session/backlog_review.go:626-634`) uses the
  older, simpler `git status --porcelain` (no `-z`, no `=v2`) and only checks
  `len(strings.TrimSpace(output)) > 0` — it was never meant to enumerate paths, just
  answer a boolean, so it's not a parsing bug, just insufficient for the new
  requirement.

**Recommendation for `GetWorktreeDirtyPaths`:** implement it in `session/backlog_review.go`
(next to `IsWorktreeDirty`, same package/file, same `safeexec` pattern already used
there) by running `git status --porcelain=v2 -z --untracked-files=all` and calling
`vc.parsePorcelainV2Z` — but note `parsePorcelainV2Z` is **unexported** (lowercase,
package `vc`). Two options, in order of preference:
1. Export it (`ParsePorcelainV2Z`) from `session/vc` and import `session/vc` from
   `session/backlog_review.go` (check for import-cycle risk first — `session/vc` is
   a leaf VCS-provider package with no apparent import back into `session` proper
   from the files read here; confirm during planning).
2. If a cycle exists, add a thin duplicate in `session/git` — but only as a last
   resort, since it would fork logic that already has one regression-tested home
   and the `-z` rename/space-handling correctness is exactly what's easy to get
   subtly wrong a second time (per the "second time it's been diagnosed" framing in
   the requirements doc).

Either way, the underlying git invocation and parser are proven; no new Go stdlib
or third-party dependency is needed beyond what's already imported
(`safeexec`, `strings`, `strconv` — all already used in `session/vc/git_provider.go`).

## Summary of exact code touchpoints for the plan phase

| Concern | File | Change |
|---|---|---|
| `--git-dir` → `--git-common-dir` | `session/backlog_commands.go:244` | flag string swap only |
| New `GetWorktreeDirtyPaths` | `session/backlog_review.go` (near `IsWorktreeDirty`, line ~626) | new func using `git status --porcelain=v2 -z --untracked-files=all` + reused/exported `parsePorcelainV2Z` from `session/vc` |
| Rejection message using dirty paths | `server/mcp/tools_backlog.go:381-392` | call `GetWorktreeDirtyPaths`, format path list instead of blanket `git add -A` instruction |
| Regression test fixture | new `*_test.go` near `session/backlog_commands_test.go` or `session/git/*_test.go` | real `git worktree add` fixture (pattern already exists: `session/git/drift_test.go:146`, `session/review_gate_test.go:1034` use `runGit(t, work, "status", "--porcelain")` helpers against real worktree fixtures — reuse that helper style) |
