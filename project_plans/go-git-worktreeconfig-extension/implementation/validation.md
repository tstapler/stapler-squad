# Validation (complexity 1, condensed)
| AC | Evidence |
|---|---|
| 1,2,3,6 | fork `TestVerifyExtensions` + `TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension`; `go test .` green in fork at v5.19.3-ssq.2 |
| 4,5,7 | `TestResolve{Origin,Local}BranchSHA_Succeeds_When_RepoHasWorktreeConfigExtension` (file:// remote); red on ssq.1 with the production error, green on ssq.2 |
| 8 | `go test ./session/git/...` incl. gogit backend green, test file unchanged |
| 9,10 | `make test` / `make lint` logs |
| 11 | post-deploy log grep |
Readiness gate: PASS (every AC mapped to a test or command).

## Results (2026-11-04)
- Fork `tstapler/go-git` @ v5.19.3-ssq.2 (a328acc1): `go test ./...` green except `TestNewSSHAgentAuth` (needs 1Password SSH agent socket; unrelated).
- New ops tests red on ssq.1 with `core.repositoryformatversion does not support extension: worktreeconfig`; green on ssq.2.
- `make lint`: exit 0 (after fixing pre-existing `prealloc` finding in `server/services/backlog_service_pr_pending_restart_test.go`, separate commit).
- `make test`: exit 0, 14634 tests / 0 failures. First run had one `session/mux` failure (unix socket path too long under `~/.cache/tmp`); passes with a short `TMPDIR`, unrelated to this change.
- AC11 (log grep) is post-deploy and not yet verifiable.
