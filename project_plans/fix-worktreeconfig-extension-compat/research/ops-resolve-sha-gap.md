# Research: ops.go `ResolveOriginBranchSHA` / `ResolveLocalBranchSHA` still call `OpenRepo`

Scoped follow-up to the failed review (backlog 9c33e913). Run inline, not as 6 subagents: two functions, one file.

## Hypothesis (stated before any code change)
Both functions open the repo through in-process go-git (`OpenRepo`) and read one ref. They work with `extensions.worktreeConfig=true` only because the fork (`tstapler/go-git` v5.19.3-ssq.3, `replace` in go.mod) relaxes the extension check. Every other ref read in `ops.go` already goes through `gitprovider.Backend().ResolveRef`, which routes to the git CLI when go-git cannot serve the repo. These two are the odd ones out, so a stock go-git bump would reintroduce the bug on exactly the path the item reported.

## Findings
- Precedent for the port is in the same file: `session/git/ops.go:203` (`refs.ResolveRef(ctx, loc, "origin/"+candidate)`) and `:213` (`"refs/heads/"+candidate`); `FetchBranch` (`ops.go:41`) already uses `gitprovider.Backend()` with `backend.Local{Root: ...}`.
- `Backend.ResolveRef` (`session/git/backend/backend.go:29`) returns `CommitSHA`, peels tags, and returns `backend.ErrRefNotFound` for a missing ref.
- Use fully qualified names (`refs/remotes/origin/<b>`, `refs/heads/<b>`) so a local branch literally named `origin/<b>` cannot shadow the lookup.
- Callers outside ops.go: none (grep). Only `ResolveDefaultLocalBranchSHA` and tests call them; error strings are wrapped with `%w`, so callers see errors only through `fmt.Errorf`.
- Existing tests: `ops_test.go:317,339` (origin tip, fetch failure) and `:366,:377` (worktreeConfig regression for both) must pass unchanged.

## Recommendation (build vs. buy)
Port both to `gitprovider.Backend().ResolveRef` with a bounded context; keep the error prefixes (`failed to resolve origin/%s`, `failed to resolve local branch %s`) so callers and tests are unaffected; wrap `ErrRefNotFound` with `%w`. No new dependency.

## Risks
- Behavior change: CLI-backed lookup adds a subprocess when the router picks the CLI. Low cost (called once per session start); rolled out through the existing cohort router.
- Empty-SHA result: guard `sha == ""` as an error (matches `ops.go:213` check).
- Add a test with a missing branch asserting `errors.Is(err, backend.ErrRefNotFound)`.
