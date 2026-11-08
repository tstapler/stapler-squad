# session/git — agent guidance

Git operations for sessions and worktrees (diff stats, status, worktree setup, merge-base, commits),
implemented on go-git rather than the `git` CLI.

## Policy: complete git parity

- **Match git's observable behaviour exactly**: defaults, config keys (`diff.renameLimit`,
  `merge.renameLimit`, ...), rename/copy detection semantics, status and ignore rules, and
  porcelain/numstat output semantics. A number shown in the UI must be the number `git` would print.
- **Fix the `tstapler/go-git` fork instead of working around it here.** If go-git diverges from git,
  patch the fork (`ssq/v5`) so this package stays thin. Do not re-implement a git rule in this package
  when the fork can own it (the rename limit is the worked example: the rule lives in the fork's
  `plumbing/object/rename.go`; this package only reads `diff.renameLimit` and passes it down).
- **No ad-hoc caps or heuristics that diverge from git.** A divergence needs a documented, measured
  justification in the code comment, and a test that pins the divergence so it cannot grow silently.
  A bare constant like "100 files" is not acceptable: it silently changes persisted results
  (`ShippedFileStats`, backlog ship status) for large moves.
- Never shell out to `git` as a fix or fallback (see `session/unfinished/AGENTS.md`). The CLI is allowed
  **in tests only**, as the oracle.

## Verifying parity

Compare against the CLI on a fixture and assert equal, as
`TestFileStatsBetween_ShouldMatchGitCLI_AroundRenameLimit` (`ops_rename_limit_test.go`) does with
`git diff --numstat -M` and `--name-status -M`:

- Read stdout only. git prints warnings such as "exhaustive rename detection was skipped due to too many
  files" on stderr, which `runGit` merges into its result.
- Pick boundary cases, not typical ones (for the rename limit: product exactly at `limit^2`, both sides
  above the limit, one side within the limit with the product above, exact renames above the limit).
- Known divergence, not mirrored by the fork: git's basename-guess pre-pass
  (`diffcore-rename.c`, `find_basename_matches`). Use fixtures whose basenames differ.
- Known gap (measured 2026-11-08, git rule at the default limit of 1000): go-git's similarity matrix is
  far slower than git's for the same input. 120 moved-and-edited files took about 1.3 s and 400 about
  10 s through `DiffStatBetween`; 1000 x 1000 exceeded its 30 s timeout. Cause: `buildSimilarityMatrix`
  rebuilds every destination's similarity index once per source (O(n^2) index builds). Fix it in the fork
  (cache destination indexes), not with a cap here.

## Shipping a fork change

Fork checkout: `~/code/github.com/tstapler/go-git` (`origin` = fork, `upstream` = go-git/go-git; never PR
upstream).

1. Branch off `ssq/v5`, patch with a fork-side test, keep it small, and update `README-SSQ.md`.
2. Merge to `ssq/v5`, wait for `ssq-ci`, then tag `vX.Y.Z-ssq.N` (tag only after its tests pass).
3. Bump the `replace` line in the root `go.mod` to that tag, then `go mod tidy` and `make ci`.
4. Update `docs/how-to/roll-back-go-git-fork.md` if rolling back below the new tag changes behaviour.

Do not call a fork-only exported symbol from this package without a fallback: a tag that does that
fails the rollback described in the how-to. Pass plain values (as `gitRenameLimit` does) instead.
