# Pitfalls: AC 9 and AC 11 (open criteria)

Builds on `project_plans/fix-worktreeconfig-extension-compat/research/pitfalls.md`. Evidence labels: VERIFIED = command run or file read this session; INFERRED = not run.

## AC 9: `make test` exits 0

Fix commit b6820123c touches `session/vc/main_test.go` (new) and `session/tymux/session_test.go`.

**session/vc fix: sound for the stated cause, with gaps.**
- VERIFIED: `isJujutsuRepo`/`isGitRepo` (`session/vc/detector.go:42,61`) walk up to `/` looking for `.jj`/`.git`. `~/.jj` exists on this host (`ls ~/.jj` shows `repo/ working_copy/`), so any `t.TempDir()` under `$HOME` looks like a jj repo.
- `TestMain` relocates `TMPDIR` to `/tmp` only if `os.TempDir()` itself is inside a jj/git repo. It uses the same detectors as the code under test, so it cannot fail open on a different definition of "repo".
- Gap 1: if `/tmp` has a `.git` or `.jj` ancestor (e.g. `/.git`, or a stray `/tmp/.git`), the relocated dir is also contaminated and the guard does not re-check it. Cheap hardening: re-run the detectors on the new dir and fail loudly.
- Gap 2: the fix covers only `session/vc`. Any other package whose tests use `t.TempDir()` and walk up for VCS markers is still host-dependent. Not enumerated this session (my grep for `UserHomeDir` in tests errored on a zsh glob). Before declaring AC 9 closed, run `make test` from a clean `TMPDIR` under `$HOME` and from default `/tmp`, and compare.
- Gap 3: `os.Setenv("TMPDIR")` in `TestMain` affects only the vc test binary; `make test` runs each package separately, so this is fine, but packages that shell out to jj/git from other packages inherit the host environment.
- Gap 4: `/tmp` is a RAM tmpfs on this host (see memory note `instinct_tmp_is_tmpfs_ram_oom_restarts_service`). Small per-test dirs are fine.

**session/tymux fix: assertion moved, not a root-cause fix.**
- VERIFIED diff: `assert.True(t, sess.HasMeaningfulContent(content))` now runs only when `updated` is true. Previously it ran for pollers that correctly got no update (empty content).
- Risk: the test can now pass vacuously if no poller ever sees `updated`. Check that the test still asserts `updates.Load() >= 1` (or equivalent) after the polls; otherwise the weakened assertion lets a regression through. I did not read the rest of the test (INFERRED).
- It is a concurrency test: a single green run is weak evidence for "flake fixed". Run with `-race -count=20` on that test.

**Misread risks for AC 9**
- "exit 0" from `make test` can be satisfied by a stale `.cache` sentinel or a subset target (`test-affected`). Record the exact target and the output tail.
- Memory note: BUG-087..090 are known flakes elsewhere in the repo. A red unrelated test is not proof the fix failed; check `main` first. A green single run is not proof the flake is gone.
- Per CLAUDE.md, `make test` needs `make build` first (proto/ent generation in fresh worktrees).

## AC 11: no `does not support extension: worktreeconfig` in `~/.stapler-squad/logs/staplersquad.log`

**The criterion as written is probably vacuous. VERIFIED facts:**
- `~/.stapler-squad/logs/staplersquad.log` was last modified **Sep 5** (`ls -la`), 5.8 MB. The live service is running (started Wed 2026-11-04 12:27:48 PST per `systemctl --user show`), so it is **not writing to this file**. The service logs to an instance or workspace directory (`log.GetConfigDir()` follows `STAPLER_SQUAD_INSTANCE` / workspace mode; see CLAUDE.md "Application Data" and `docs/reference/state-isolation.md`), or the file rotated.
- `grep -c "does not support extension"` on that file returns **0**. This says nothing about the fix: the file predates the bug and the deploy.
- The working tree path in this session is a `workspaces/<hash>/` worktree, which suggests workspace mode; the real log is likely under `~/.stapler-squad/workspaces/<hash>/logs/staplersquad.log`. Resolve the path first (startup log or `/actuator`), then grep that.

**Stale binary risk.**
- VERIFIED: the service `ExecStart` is `/home/tstapler/Programming/stapler-squad/stapler-squad` (a different checkout than this worktree), started 12:27:48. The fix commit afbc063dd is dated 12:26:40, only 68 seconds earlier. A binary built from that checkout includes the fix only if that checkout had afbc063dd merged and `go build` run before 12:27:48. Not verified. Prove it with `go version -m <binary>` (module line for `go-git ... v5.19.3-ssq.3`) or `strings`/`go version -m | grep ssq.3`, plus the binary mtime.
- `make install-service` builds and restarts; a plain `git pull` leaves the old binary running.

**Zero matches from no traffic.** The error only occurs on async session start in a repo whose `.git/config` has `extensions.worktreeconfig=true` (default-branch resolution, `ops.go`). Zero matches proves nothing unless such a session start was exercised after the restart. Positive control: start a session in a repo with a linked worktree (config shows `extensions.worktreeconfig=true`) and confirm a success log line, then grep. Also grep the old pattern against the pre-fix log window to confirm the grep itself can match (negative-control the grep).

**Log rotation / retention.** If logs rotate, grep only the current file and miss archived lines (`staplersquad.log*`); and a rotation after deploy can drop the evidence window. Bound the grep by timestamp (JSON-lines, `time` field >= restart time 2026-11-04T12:27:48-08:00).

**Other extensions go-git still rejects (VERIFIED from `repository_extensions.go` in `~/go/pkg/mod/github.com/tstapler/go-git/v5@v5.19.3-ssq.3`).**
- Allow-list (`builtinExtensions`): `noop`, `noop-v1`, `worktreeconfig`, `partialclone`, `preciousobjects`. V0 allow-list adds the same minus `noop-v1`. Keys are lowercase (case bug from the earlier pitfalls doc fixed).
- Everything else fails with `unknown extension: <name>` (V1 repos) or `core.repositoryformatversion does not support extension: <name>` (V0 repos). Notably still rejected: `objectformat` (SHA-256 repos), `compatobjectformat`, `refstorage` (reftable), `relativeworktrees`, `submodulepathconfig`, `worktreeconfig`-adjacent `extensions.*` added by newer git, and any custom extension.
- Consequence for AC 11: the grep string `does not support extension` now matches *other* extensions on V0 repos, and the string `worktreeconfig` is the discriminator. A log line `does not support extension: relativeworktrees` is a different bug, not an AC 11 failure; conversely, grepping only `worktreeconfig` hides these. Grep for both `does not support extension` and `unknown extension` and triage by extension name.
- Misread risk: git >= 2.48-era features (`relativeworktrees` via `git worktree --relative-paths`, `refstorage`) make this class likely to recur on repos created by newer git; the host git version was not checked.
- Storage-level extension checks (`ExtensionChecker` on storers) were not inspected (INFERRED not an issue for the filesystem storer).

## Recommended verification for closing both ACs
1. AC 9: `make build && make test`, record exit code and tail; run vc and tymux packages with `-race -count=20`; run once with default TMPDIR.
2. AC 11: confirm binary contains `go-git ... v5.19.3-ssq.3` and was started after install; resolve the real log path; exercise a session start in a worktreeconfig repo; grep the post-restart window for `worktreeconfig` plus `unknown extension`/`does not support extension`.
