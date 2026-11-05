# Build vs Buy (re-check): keep the go-git fork?

Date: 2026-11-04. Builds on `project_plans/fix-worktreeconfig-extension-compat/research/build-vs-buy.md`. Scope: verification only (AC 9, AC 11).

## Facts (VERIFIED via `gh api`, 2026-11-04)

- `go.mod:31` requires `go-git/v5 v5.19.3`; `go.mod:232` replaces it with `tstapler/go-git/v5 v5.19.3-ssq.3`.
- Latest upstream v5 release is `v5.19.3` (2026-10-04). Its `repository_extensions.go` still has the camelCase `extensionsValidForV0` keys (`partialClone`, `preciousObjects`, `worktreeConfig`), so the bug is unfixed in every v5 release.
- Upstream `main` and `v6.0.0-beta.1` (2026-10-04) have the lowercase keys, so the fix exists, but only on v6. Upstream issues/PRs on this (#1939, #2156, #2171, #2190, #2298) are all closed; I did not confirm that any of them landed on the v5 branch, and no v5 release past 5.19.3 exists.
- The fork `v5.19.3-ssq.3` is 2 commits ahead of upstream `v5.19.3`. It touches `repository_extensions.go`, its test, `README-SSQ.md` and one CI workflow.

## Options

### 1. Keep the fork (v5.19.3-ssq.3)
- Pros: already merged (#1430) and working; tiny diff; rollback runbook exists (`docs/how-to/roll-back-go-git-fork.md`); no API churn.
- Cons: fork must be rebased on each upstream v5 release; the `replace` directive hides the real upstream version from tooling.
- Verdict: KEEP. Nothing changed that justifies revisiting.

### 2. Upstream bump
- v5: no release contains the fix, so a bump to stock v5 reintroduces the bug (the roll-back doc warns of this).
- v6.0.0-beta.1 has the fix, but it is a beta with a new major API; migrating all `OpenRepo` and `session/git` callers is a large change for a verification-only task.
- Pros: drops the fork and its maintenance.
- Cons: v5 path impossible today; v6 path is high-risk and out of scope.
- Verdict: NOT NOW. Revisit when v6 reaches GA, or if upstream cuts a v5.19.4+ with the lowercase keys. Trigger check: `gh api repos/go-git/go-git/releases --jq '.[0:3][].tag_name'`, then diff `repository_extensions.go`. When a v5 release carries the fix, remove the `replace` per the roll-back doc.

### 3. Shell out to the git CLI
- Pros: git always accepts the extension; no fork.
- Cons: contradicts the repo policy (`prefer-go-git-over-subshells`); subprocess cost and error-parsing on hot paths; loses the in-process plumbing already built.
- Verdict: REJECT as primary. Keep as an emergency fallback only.

## Conclusion

Build-vs-buy decision stands: keep the fork. No new code is needed to close the remaining criteria; verify AC 9 by running `make test`, and AC 11 by grepping `~/.stapler-squad/logs/staplersquad.log` for `does not support extension: worktreeconfig` after deploying a binary built from this fix (a zero count against the old binary proves nothing).
