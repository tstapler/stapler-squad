# Adversarial review: Phase 2 (ops.go ResolveRef port) and the 2026-11-04 amendment

Reviewed: `implementation/plan.md` Phase 2 (lines 229-307), `research/ops-resolve-sha-gap.md`.
Reviewer date: 2026-11-04. Read-only on the repo: the port was applied and run in a throwaway
git worktree (removed afterwards); `git status` in the main checkout is unchanged.

## Verdict: CONCERNS (no BLOCKERs)

The port is sound. Applying exactly the plan's snippets compiled, and the whole
`./session/git` package passed (`ok ... 5.769s`). Under stock go-git v5.19.3 (fork `replace`
removed), the two worktreeConfig tests fail on the current code and pass after the port.
The concerns below are spec defects in the amendment's test instructions and stale plan text.

## BLOCKERs

None.

## Verified claims (evidence)

1. **Fully-qualified refs work in both backends.**
   - CLI: `session/git/backend/cli/refs.go:94-105` runs `rev-parse --verify <ref>^{commit}`.
     `commitRef` (`cli/refs.go:30-41`) only rejects `-` prefixes, `:` and non-commit peel suffixes.
     In a run through the CLI route, `refs/heads/feature/x` and `refs/heads/nonexistent` resolved
     or failed as git does.
   - go-git: `gogit/refs.go:79-102` -> `revision.go:55-82` (`dwim`) has an explicit `%s` rule
     for names starting with `refs/` (`revision.go:63-67`). A direct backend test on a fresh repo
     (with a decoy local branch named `origin/main`) gave: `refs/remotes/origin/main` -> correct
     SHA; `refs/heads/main` -> correct SHA; `refs/heads/nope`, `refs/remotes/origin/nope`,
     `refs/heads/feature/x` -> `ErrRefNotFound`. The fully-qualified form is correct: the
     decoy `origin/main` branch did not shadow the lookup.
2. **Missing ref returns `ErrRefNotFound` in both.**
   - go-git: `gogit/refs.go:15` (`notFound` wraps the sentinel), returned at `refs.go:101`.
   - CLI: `cli/cli.go:262` maps "unknown revision" / "needed a single revision" / "bad revision"
     to `ErrRefNotFound`. The run showed `errors.Is(err, backend.ErrRefNotFound) == true` for
     `ResolveLocalBranchSHA(work, "nonexistent")`, with and without worktreeConfig.
3. **worktreeConfig repos route to the CLI, as designed.** `gogit/layout.go:335-337` returns
   fallback-eligible for `extensions.*` in a `repositoryformatversion` 0 repo, and
   `layout.go:379-384` does the same for a truthy `worktreeconfig` in v1 (both observed
   directly). The Router then runs the CLI (`routed.go:331-334`). The default cohort mode is
   `BackendCLI` (`cohort.go:64-69`), so today every call goes straight to the CLI anyway.
4. **Existing tests hold.** `ops_test.go:317` (fetched tip), `:339` (fetch fails), `:366` and
   `:377` (worktreeConfig), plus all `TestResolveExplicit*`, `TestResolveDefault*`,
   `TestResolveWorktreeBaseCommit_*` pass with the port. No test asserts the old
   `"failed to open git repo"` text for these functions (grep: the only hits for that string
   are `util.go`, `worktree_git.go` and `worktree_git_test.go:116`, none for these functions).
   `ops_test.go:366/377` are real regression tests: with stock go-git and the unported
   code they fail with `core.repositoryformatversion does not support extension: worktreeconfig`.
5. **No caller depends on go-git error types.** Callers of the two functions are
   `ops.go:67`, `:121`, `:259-261`. They only collect or `%w`-wrap errors
   (`ResolveWorktreeBaseCommit` at `ops.go:142-154`, `instance_worktree.go:284`). The
   `plumbing.ErrReferenceNotFound` uses (`ops.go:280`, `worktree_ops.go:49`) belong to
   `IsUnbornRepo` and `branchRefExists`, which are not touched by this port.
6. **No staleness after the CLI fetch.** `gogit/repoview.go:39-56` builds a fresh view per
   call, so a go-git-served `ResolveRef` after `FetchBranch` sees the fetched loose ref.
7. **Line anchors in the plan are right.** Lines 87-95 and 104-112 of `ops.go` are the exact
   `OpenRepo` ... `return ref.Hash().String(), nil` spans.
8. **Imports.** `plumbing` is still used 25 times in `ops.go`, `context` and `time` are
   already imported (`ops.go:4,10`), and `backend`/`gitprovider` too (`ops.go:21,23`). The
   package compiled with no import edits.

## Concerns

C1 (HIGH, spec defect). The amendment (plan.md:307) says to add "a missing-branch test asserting
`errors.Is(err, backend.ErrRefNotFound)` for both functions". For `ResolveOriginBranchSHA`, an
ordinary missing branch fails at the **fetch** step, before `ResolveRef` runs. Observed error:
`failed to fetch nonexistent: failed to fetch branch: git Fetch failed: fatal: couldn't find remote ref nonexistent`,
and `errors.Is(err, ErrRefNotFound)` was **false**. An implementer who follows the amendment
literally will write a test that fails. Story 2.1.1's third AC describes the achievable
scenario (fetch succeeds, no tracking ref). I reproduced it: clone, then create `feature-x` on
origin, `git config --unset-all remote.origin.fetch` on the clone, and call
`ResolveOriginBranchSHA(work, "feature-x")`. Result: `failed to resolve origin/feature-x: git backend: ref not found: ...`
with `errors.Is(..., ErrRefNotFound) == true`. Fix: reword the amendment so the origin test
uses this no-refspec setup, and the local test uses a plain missing branch.

C2 (MEDIUM). Even the plan's go-git claim is only exercised on the CLI route by the test suite,
because the default cohort mode is CLI (`cohort.go:64-69`) and the tests don't configure
`GitBackendCohorts`. The worktreeConfig tests therefore prove "CLI path works", not "go-git
path falls back". I verified the go-git-to-CLI fallback only with a direct backend test
(see claim 3). Optionally add a Router-level test with the `refs` cohort set to `gogit`.

C3 (LOW). `if sha == ""` guard in Task 2.1.1a is effectively dead: both backends return
`parseSHA`/`Hash.String()` values or an error, and `ResolveLocalBranchSHA` omits the same
guard. Either drop it or add it to both for symmetry. (The research doc cites `ops.go:213`
as precedent; that guard exists because that caller collects errors in a loop.)

C4 (LOW). Plan text still says `ResolveRef("origin/"+branch)` DWIM risk (plan.md:76) and
describes `resolveRemoteWorktreeBaseCommit` as the pattern (plan.md:25). The port deliberately
uses `refs/remotes/origin/` instead, which removes that risk row. Update or delete the risk row
so the plan does not contradict Task 2.1.1a.

C5 (LOW). New 10s timeout is safe but is new behavior: neither original function had one.
Both calls are local ref reads (CLI `rev-parse`, ~10 ms in tests; the full set ran in 0.14 s).
Callers loop at most 4 candidates (`ops.go:64-73`), each already bounded by `FetchBranch`'s 30 s.
On a loaded machine (see OOM notes in MEMORY.md) a timeout surfaces as an error wrapped in the
same prefix; `fallbackReasonFor` (`routed.go:346`) deliberately does not retry on a done
context. No caller special-cases it. Acceptable; consider reusing one named constant for the
two timeouts.

C6 (INFO, out of scope but affects the "immune" claim in the Phase 2 goal). Ported
functions are immune, but `IsUnbornRepo` (`ops.go:296`, via `OpenRepo`) is still on the
fallback path of `ResolveWorktreeBaseCommit` (`ops.go:150`). On a worktreeConfig repo with stock
go-git it returns false instead of detecting an unborn repo. The plan's merge gate (keep the
fork bump) already covers this; the Phase 2 goal wording should say "these two functions",
not "session creation".

## Not done

`make lint` / `make test` were not run (the amendment gates the `go.mod` commit on them).
Only `go test ./session/git/` and `go vet ./session/git/` were run on the ported copy.
There is no `norawgitopen` ratchet count tied to `ops.go` (grep of `tools/lint`, `Makefile`,
`.golangci.yml`), so removing two `OpenRepo` calls needs no count update.
