# Stack facts for the open criteria (AC9, AC11)

Builds on `project_plans/fix-worktreeconfig-extension-compat/research/stack.md` (whose
`ssq.1` and "lowercase keys" findings are now superseded by the merged fix). All items
below were checked on branch `backlog/stapler-squad-fix-worktreeconfig-extension-compat`
at `a80d6614c`.

## Dependency pin (VERIFIED)
- `go.mod:232`: `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.3`.
- `git ls-remote --tags https://github.com/tstapler/go-git v5.19.3-ssq.3` returns
  `a328acc1f747b208f5398abd4458e3b5c2a1b627 refs/tags/v5.19.3-ssq.3`. The tag resolves.
- `go.sum:366-367` has both hashes for ssq.3, and the module is in the local cache
  (`~/go/pkg/mod/github.com/tstapler/go-git/v5@v5.19.3-ssq.3`).
- The fork's `repository_extensions.go` now carries lowercase `"worktreeconfig"` in both
  `builtinExtensions` (V1) and `extensionsValidForV0` (V0), with the comment "Keys must be
  lowercase to match extensions()". This fixes both gates the old research flagged.

## Remaining go-git OpenRepo callers (VERIFIED by grep)
- Every non-test `session/` path opens through one chokepoint:
  `session/git/util.go:34 OpenRepo` -> `session/git/native/open.go:28 OpenRepo` ->
  `git.PlainOpenWithOptions`. This chokepoint is the one `norawgitopen` lint allows.
  Callers outside `session/git` also go through it: `github/client.go:862`,
  `session/repo_path.go:287,346`. All get the ssq.3 fix with no per-caller change.
- Other direct go-git importers (non-test) that open repos on their own:
  `session/git/backend/gogit/repoview.go`, `session/unfinished/gogitstore/open.go`,
  `session/unfinished/gogit_vcs_reader*.go`, `session/unfinished/aheadbehind.go`,
  `tools/scanner-workload/*.go`. These still depend on the fork's extension check, so they
  are covered by the same pin and not by the ResolveRef port.
- The ResolveRef port (PR #1430) covers only `ResolveOriginBranchSHA` and
  `ResolveLocalBranchSHA`. All other go-git consumers rely on the fork fix alone.
  A go.mod regression to ssq.1 or ssq.2 would silently reintroduce the error everywhere
  except those two functions.

## AC9 (`make test` exits 0)
- `Makefile:658` `test` depends on `ensure-tools proto-gen $(BIN_TMUX) $(GOTESTSUM_BIN)`.
  It runs `session`, `session/mux` and `session/tmux` serially (`-p 1`) because of tmux
  contention, then the rest in parallel. It needs a real tmux and generated protos/ent.
- Gitignored generated code (`gen/`, `session/ent/*`) means a fresh worktree needs the
  `ensure-tools proto-gen` prerequisites to run; `make test` handles this itself.
- HEAD `b6820123c` already changed `session/vc` and `tymux` tests for hermeticity.
  AC9 is a run-and-show-output criterion: only an actual `make test` exit code proves it.
  Known flakes (BUG-087..090, per memory) should be checked against `main` before fixing.
- Per the user's `CLAUDE.md` rule, do not claim AC9 without pasting the output.

## AC11 (no worktreeconfig log lines after deploying the fix)
- Deploy: `make install-service` (`Makefile:504`) runs preflight (requires the deployed unit
  to have `--tmux-keep-server`), `backup-binary`, `build-embedded-tymux`, `install-hooks`,
  then `STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh`.
  Note it uses `$(CURDIR)/stapler-squad`, so it deploys whatever is at the repo root, built
  from this worktree. Confirm the binary was built after the ssq.3 commit.
- On Linux it writes `~/.config/systemd/user/stapler-squad.service` with
  `ExecStart=<bin> --remote-access --tmux-keep-server`, then `systemctl --user restart
  stapler-squad` (`install-service.sh:343,374`). Tmux sessions survive the restart.
- Logs, in `~/.stapler-squad/logs/` (the default instance; other instances and workspace
  modes use `instances/<name>/` or `workspaces/<id>/logs`):
  - `staplersquad.log`: JSON lines, one slog record per line. This is the main grep target.
  - `service.log`: raw systemd stdout/stderr (`install-service.sh:356-357`), which also
    catches early panics.
- Grep the error text case-insensitively, since go-git emits the lowercase extension name:
  `grep -ic 'does not support extension' ~/.stapler-squad/logs/{staplersquad,service}.log`
  Use `grep -i 'worktreeconfig'` as a second pattern. Neither string exists in this repo's
  Go source (it comes from go-git), so a grep of the code cannot find the emitter.
- Baseline today (pre-deploy, this machine): `grep -ic` of `worktreeconfig` returns 0 in both
  logs, and `staplersquad.log` was last written Sep 5 (the file is 5.8 MB).
  A zero count after deploy is therefore weak evidence unless the log is current.
  To get a valid check:
  1. Record the log's byte size or mtime before deploy.
  2. Deploy.
  3. Trigger the old failure path (create a session or backlog worktree on a repo that has
     `extensions.worktreeConfig`).
  4. Grep only the lines added after the recorded offset (`tail -c +<size>`).
- A stronger, deterministic proof is a Go test that opens a temp repo with
  `extensions.worktreeConfig=true` through `OpenRepo`. Check whether #1430 already added one
  before writing another.
