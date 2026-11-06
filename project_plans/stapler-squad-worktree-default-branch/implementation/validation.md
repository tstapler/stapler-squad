# Validation Plan: stapler-squad-worktree-default-branch

**Date**: 2026-11-04
**Complexity**: 2 (verification-only; no product code change planned, see `plan.md`)

Evidence labels: VERIFIED = test name/path confirmed by grep or command run while writing this file; INFERRED = not run.

## Happy Path Scenario

Given a repo whose `.git/config` has `core.repositoryformatversion=0` and `extensions.worktreeconfig=true` (this repo, and every linked worktree of it), when a backlog session starts and resolves its default branch and creates a worktree, then no `does not support extension: worktreeconfig` error occurs, and `make test` exits 0 with the deployed `ssq.3` binary logging zero such errors since its restart.

## Requirements

Source: `requirements.md` (AC9, AC11 open) plus backlog ACs 1-8 and 10 (already passing; kept as regression guards, not re-proven by new work).

| ID | Requirement |
|----|-------------|
| REQ-1 | Default-branch / branch-SHA resolution succeeds on a `worktreeConfig repo` (ACs 1-8, 10, regression guard) |
| REQ-2 | Full session-start path (resolve default branch, create worktree) succeeds on a `worktreeConfig repo` (ACs 1-8, 10, regression guard) |
| REQ-3 | The fork pin `v5.19.3-ssq.3` is in `go.mod` and is what the deployed binary embeds (covers the go-git callers not routed through `ResolveRef port`) |
| REQ-4 | AC9: `make test` exits 0 at HEAD |
| REQ-5 | AC9 part: `session/vc` is hermetic against a VCS-managed `TMPDIR` |
| REQ-6 | AC9 part: `session/tymux` concurrent-poll test is stable and non-vacuous |
| REQ-7 | AC11: zero real `does not support extension` records after the deployed restart, shown with a negative control and a positive control |

## Requirement → Test Mapping

All test names below were confirmed present by grep at the file and line given (VERIFIED). Naming deviates from `methodName_should_X_When_Y` because the repo's existing Go convention is `TestX_Verb_When_Cond`; existing names are reused rather than renamed.

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1 | `session/git/ops_test.go:367` | `TestResolveOriginBranchSHA_Succeeds_When_RepoHasWorktreeConfigExtension` | Unit | Happy path (origin tip resolves on worktreeConfig clone) |
| REQ-1 | `session/git/ops_test.go:378` | `TestResolveLocalBranchSHA_Succeeds_When_RepoHasWorktreeConfigExtension` | Unit | Happy path (local branch tip) |
| REQ-1 | `session/git/ops_test.go:389` | `TestResolveLocalBranchSHA_ReturnsErrRefNotFound_When_BranchMissing` | Unit | Error path: typed not-found, not an extension error |
| REQ-1 | `session/git/ops_test.go:401` | `TestResolveOriginBranchSHA_ReturnsErrRefNotFound_When_NoRemoteTrackingRef` | Unit | Error path |
| REQ-1 | `session/git/ops_test.go:340` | `TestResolveOriginBranchSHA_ReturnsError_When_FetchFails` | Unit | Error path: fetch failure surfaces |
| REQ-1 | `session/git/ops_test.go:318` | `TestResolveOriginBranchSHA_ReturnsFetchedTip_When_OriginHasAdvanced` | Integration | Real git origin + clone, fetched tip returned |
| REQ-2 | `session/repo_path_test.go:598` | `TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension` | Integration | Full path: sets `extensions.worktreeConfig=true` on a real clone, `CreateBacklogWorktree` succeeds and the worktree dir exists |
| REQ-2 | `session/repo_path_test.go` | `TestCreateBacklogWorktree_AnchorsAtMainRepo_When_RepoPathIsAWorktree` | Integration | Regression: repoPath is a linked worktree |
| REQ-2 | `session/git/native/gitconfig/resolver_test.go:205` | `TestWorktreeConfigIsReadOnlyWhenTheRepositoryEnablesIt` | Unit | Error/edge path: `config.worktree` honoured only when extension is on |
| REQ-3 | `go.mod:232` (no Go test; command check) | n/a | Config check | Happy path: `replace` points at `ssq.3` (see Runnable Checks C-3a) |
| REQ-3 | deployed binary (no Go test; command check) | n/a | Integration | Binary embeds `ssq.3` and started after the fix commit (C-3b, C-3c) |
| REQ-3 | `go.mod` (command check) | n/a | Config check | Error path: a regression to `ssq.1`/`ssq.2`/upstream makes C-3a print a different version; check is red |
| REQ-4 | `Makefile:658` target `test` | `make test` (whole suite, 2 gotestsum phases) | Integration | Happy path: exit 0 under contaminated `TMPDIR` (C-4a) |
| REQ-4 | `Makefile:658` | `make test` | Integration | Error path: triage procedure (C-4b) if red; a red run is never reported as pass |
| REQ-5 | `session/vc/main_test.go:12` | `TestMain` (relocates `TMPDIR` to `/tmp/ssq-vc-test-*` when `isJujutsuRepo`/`isGitRepo` of `os.TempDir()`) | Unit | Happy path: `go test ./session/vc` exits 0 with `TMPDIR` under `~/.jj` (C-5a) |
| REQ-5 | `session/vc/main_test.go:12` | `TestMain` | Unit | Control path: also exits 0 with `TMPDIR=/tmp` (C-5b), so the fix is not specific to one env |
| REQ-5 | `session/vc/main_test.go` | `TestMain` re-check of the relocated dir | Unit | Error path (latent, plan Task T-AC9-6): ancestor probe C-5c expects no VCS dir at `/`, `/tmp` |
| REQ-6 | `session/tymux/session_test.go:702` | `TestTymuxGRPCSession_ActivityConcurrentPolls` | Unit | Happy path: 8 pollers, exit 0 under `-race -count=20` (C-6a) |
| REQ-6 | `session/tymux/session_test.go:722` | same test, `assert.EqualValues(t, 1, updates.Load(), ...)` | Unit | Error path / non-vacuity: with zero updates the test fails (C-6b) |
| REQ-7 | live log (command check) | n/a | Integration | Happy path: window filter returns 0 (C-7c) |
| REQ-7 | rotated `.gz` (command check) | n/a | Integration | Negative control: same filter returns a non-empty first/last pair on pre-fix log (C-7b) |
| REQ-7 | `session/repo_path_test.go:598` and `session/git` `Succeeds_When_RepoHasWorktreeConfigExtension` | named tests | Integration | Positive control: both pass and are not skipped (C-7e1, C-7e2) |

## Runnable Checks (exact command, expected output)

Run from the worktree root unless noted. `LOGDIR=~/.stapler-squad/workspaces/d685c4b1a423cca3/logs` (NOT `~/.stapler-squad/logs/`, stale since Sep 5). `RESTART=2026-11-04T12:27:48-08:00` (the one restart-instant literal; service `ActiveEnterTimestamp`, which `systemctl` prints as `2026-11-04 12:27:48 PST`; recompute with C-7a if the service restarted again). `EVIDENCE_DIR=project_plans/stapler-squad-worktree-default-branch/implementation/evidence/`.

### REQ-1, REQ-2 (regression guards)

| ID | Command | Expected |
|----|---------|----------|
| C-1 | `go test ./session/git -run 'Resolve(Origin\|Local)BranchSHA' -count=1 -v 2>&1 \| tail -n 30` | Exit 0; `--- PASS` for all 7 names in the table above; no `FAIL` |
| C-2 | `go test ./session -run 'TestCreateBacklogWorktree_' -count=1 -v 2>&1 \| tail -n 30` | Exit 0; `--- PASS: TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension` |
| C-2b | `go test ./session/git/native/gitconfig -run 'TestWorktreeConfig' -count=1` | Exit 0 |
| C-2c | `git config --get-regexp '^extensions\.\|core.repositoryformatversion'` (in this worktree) | Includes `extensions.worktreeconfig true` and `core.repositoryformatversion 0` (proves the live repo really is the failing shape) |

### REQ-3 (fork pin)

| ID | Command | Expected |
|----|---------|----------|
| C-3a | `grep -n 'tstapler/go-git/v5' go.mod` | Line 232: `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.3` |
| C-3b | `go version -m /home/tstapler/Programming/stapler-squad/stapler-squad \| grep -E 'go-git\|vcs.revision\|vcs.time\|vcs.modified'` | Contains `github.com/tstapler/go-git/v5 v5.19.3-ssq.3`; VERIFIED `vcs.revision=afbc063dd`, `vcs.modified=true` |
| C-3b2 | `git merge-base --is-ancestor afbc063dd <vcs.revision from C-3b>; echo $?` | `0` (deployed revision descends from, or equals, the fix commit) |
| C-3c | `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p ExecStart` and `stat -c '%y' /home/tstapler/Programming/stapler-squad/stapler-squad` and `git log -1 --format=%cI afbc063dd` | Fix commit time (`2026-11-04T12:26:40-08:00`) < binary mtime (~12:27:36) < `ActiveEnterTimestamp` (~12:27:48); `ExecStart` path equals the inspected binary |
| C-3d | `git ls-remote --tags https://github.com/tstapler/go-git v5.19.3-ssq.3` | One line, SHA starts `a328acc1` (VERIFIED (adversarial review) from plan; re-confirm on run) |

### REQ-4 (AC9)

| ID | Command | Expected |
|----|---------|----------|
| C-4pre | `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p MainPID`; `uptime`; `free -g`; `export STAPLER_SQUAD_TMUX_SOCKET=ssq-verify-1`; `grep -rn 'STAPLER_SQUAD_TMUX_SOCKET\|EnvSocketVar' session/tmux/tmux.go` (const at `session/tmux/tmux.go:653`) plus a grep of the tmux-using test packages (`session`, `session/tmux`, `session/mux`) showing they resolve the socket through `ResolveSocket`/`EnvSocketVar` | BEFORE capture of `ActiveEnterTimestamp` (expected `2026-11-04 12:27:48 PST`) and `MainPID` recorded; load1 <= `nproc`; available >= 8 GiB, else abort; grep shows tmux-using packages honor the env var, else the `make test` run is not isolated from the live tmux server and must not start |
| C-4mon | Background monitor started with C-4a (see plan T-AC9-2): every 60 s sample `uptime` load1 and `free -g` available | No sample with load1 > `nproc*1.5` or available < 4 GiB; on breach the run is aborted and C-4pre re-baselined, never reported as pass |
| C-4a | `unset npm_config_user_agent; mkdir -p ~/.cache/tmp/ssq-ac9 && bash -o pipefail -c 'make test > ~/.cache/tmp/ssq-ac9/make-test.log 2>&1'; echo "exit=$?"` with `TMPDIR=/home/tstapler/.cache/tmp` | `exit=0`; `tail -n 5` of the log shows gotestsum `DONE <N> tests` with 0 failures (user-reported prior run: 5433 + 14638 tests, 0 fail, EXIT=0, VERIFIED by caller, not re-run here) |
| C-4a2 | `grep -c '^--- FAIL\|^FAIL' ~/.cache/tmp/ssq-ac9/make-test.log` (full log, not a tail) | `0` (grep exits 1 with count 0; that is the pass condition) |
| C-4post | `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p MainPID` | AFTER capture of both fields equals the BEFORE capture (a changed `MainPID` means a restart even if the timestamp is somehow equal); if it differs, the restart instant moved: re-baseline and re-run C-3b/C-3c and C-7a..C-7e. A prior `make test` run already gave EXIT=0 (5433 + 14638 tests) with the timestamp unchanged |
| C-4b | Only if C-4a is non-zero: for each failing test `go test <pkg> -run <Test> -count=3 -race`, then `gh run list --repo tstapler/stapler-squad --branch main --limit 5` | Note records fails-in-isolation yes/no, fails-on-main yes/no, and a "fails because X" sentence; AC9 stays unreported until green |

### REQ-5 (vc hermeticity)

| ID | Command | Expected |
|----|---------|----------|
| C-5a | `TMPDIR=/home/tstapler/.cache/tmp go test ./session/vc -count=1` | `ok  github.com/.../session/vc` (exit 0) |
| C-5b | `TMPDIR=/tmp go test ./session/vc -count=1` | exit 0 |
| C-5c | `ls -d /.git /.jj /tmp/.git /tmp/.jj 2>&1` | Four "No such file or directory" lines (the `/tmp` relocation target has no VCS ancestor) |

### REQ-6 (tymux)

| ID | Command | Expected |
|----|---------|----------|
| C-6a | Run only AFTER C-4a finishes, and after a fresh `uptime`/`free -g` re-check meeting the C-4pre thresholds (never concurrent with `make test` on the live host): `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m -v 2>&1 \| tail -n 15` | Exit 0; 20 PASS iterations, no `DATA RACE` |
| C-6b | `sed -n 700,724p session/tymux/session_test.go` (or Read) | `assert.EqualValues(t, 1, updates.Load(), ...)` still follows `wg.Wait()` (VERIFIED at line 722 when written) |

### REQ-7 (AC11)

| ID | Command | Expected |
|----|---------|----------|
| C-7a | Run after C-4post (final restart instant): `systemctl --user show stapler-squad -p ActiveEnterTimestamp`; `ls -la $LOGDIR/staplersquad.log` | Restart instant noted (use the newest one if it moved); log mtime within minutes, so it is the live file |
| C-7b (negative control) | `zcat $LOGDIR/staplersquad-2026-11-04T13-51-53.139.log.gz \| jq -Rc 'fromjson? \| select(((.err // .error // "")\|tostring)\|test("does not support extension\|unknown extension";"i")) \| .time' \| sed -n '1p;$p'` | Prints a first/last pair, both before the restart instant. VERIFIED while writing: `2026-11-04T10:32:03...` and `2026-11-04T12:24:23...`. `plan.md` now quotes the same pair; the check is "non-empty pair before restart" |
| C-7c (window) | `jq -Rc 'fromjson? \| select(.time >= "2026-11-04T12:27:48-08:00") \| select(((.err // .error // "")\|tostring)\|test("does not support extension\|unknown extension";"i")) \| .time' $LOGDIR/staplersquad.log \| wc -l` | `0`. VERIFIED while writing: `0`. Repeat over any `.gz` newer than the restart; each must print 0 lines |
| C-7d (window length) | `jq -Rr 'fromjson? \| .time' $LOGDIR/staplersquad.log \| tail -n 1` | Last record time minus restart instant is the window length; state it. Pre-fix rate was 9802 `failed to check git status` hits in one rotated file (plan, VERIFIED at planning), so a regression would be loud |
| C-7e1 (positive control) | `go test ./session -run TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension -count=1 -v` | Exit 0, `--- PASS`, no `--- SKIP` (`session/repo_path_test.go:598`) |
| C-7e2 (positive control) | `go test ./session/git -run 'Succeeds_When_RepoHasWorktreeConfigExtension' -count=1 -v` | Exit 0, `--- PASS` for every matched test, no `--- SKIP` |
| C-7e3 (supplementary only) | `jq -Rc 'fromjson? \| select(.time >= "2026-11-04T12:27:48-08:00") \| select(.msg\|test("set base commit SHA\|found existing worktree for branch\|connected to existing worktree")) \| del(.program) \| {time,msg}' $LOGDIR/staplersquad.log \| head` | The `2026-11-04T15:25:17` record is WEAK (initBaseCommitSHA/merge-base, not the default-branch resolve path) and never satisfies the positive control by itself |
| C-7h (evidence copy) | `cp $LOGDIR/staplersquad-2026-11-04T13-51-53.139.log.gz $EVIDENCE_DIR/ && (cd $EVIDENCE_DIR && sha256sum *.gz > SHA256SUMS)`; compare with `sha256sum` of the source | Hash recorded in `evidence.md`; copy hash equals source hash. Commit only `SHA256SUMS` (and `evidence.md`), never the `.gz` |
| C-7i (time format) | `jq -Rr 'fromjson? \| .time' $LOGDIR/staplersquad.log \| head -n 1` | Offset is `-08:00` and matches the restart instant's zone before the lexical comparison in C-7c is trusted |
| C-7f (noise explained) | `jq -Rc 'fromjson? \| select(.time >= "2026-11-04T12:27:48-08:00") \| select(.msg=="creating session" or .msg=="tmux session created successfully") \| select(.program\|tostring\|test("does not support extension")) \| .msg' $LOGDIR/staplersquad.log \| wc -l` | Non-zero is fine (plan: 6): prompt echoes in `program`. Shows why a raw `grep worktreeconfig` is wrong, and the C-7c filter excludes them |
| C-7g (foreign extension) | `git config --get-regexp '^extensions\.'`; `git --version` | Lists only `extensions.worktreeconfig` (git 2.53.0 per plan). Any other extension is a separate bug to file, not an AC11 failure |

## UX Acceptance Tests

Not applicable: no user-facing surface and no `design/ux.md` (backend/verification-only).

## Migration Test

N/A (no Migration Plan in `plan.md`).

## Test Stack
- **Unit**: Go `testing` + testify (`assert`/`require`), existing tests only; no new tests planned.
- **Integration**: real git repos built in `t.TempDir()` (`runGitOrFail`, `cloneWithOrigin`); `make test` via gotestsum; deployed-binary and live-log checks via `go version -m`, `systemctl --user`, `jq`, `zcat`.
- **E2E / UX**: none.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (scoped) | `go test ./session/git ./session -run 'WorktreeConfig\|Resolve(Origin\|Local)BranchSHA' -coverprofile=cover.out && go tool cover -func=cover.out \| grep -E 'ops.go.*Resolve(Origin\|Local)BranchSHA'` | Both functions > 0%, happy and error branches hit (no new code, so repo-wide 80% is not re-gated here) |

- Every requirement has at least one runnable check above (REQ-1..REQ-7: 7/7).
- AC9 is accepted only if C-4a exits 0, C-4a2 = 0 over the full log, and C-4pre/C-4post show an unchanged `ActiveEnterTimestamp` and `MainPID`, and C-4mon shows no breach.
- AC11 is accepted only if C-7b is non-empty, C-7c = 0, and BOTH C-7e1 and C-7e2 pass (not skipped); C-7e3 alone never suffices. Zero matches without these controls is reported as unproven. Evidence is committed and pushed before `report_progress`.
- If a check is red, report the AC as `fail` with the reason; do not edit product code except per plan Tasks T-AC9-5/T-AC9-6.
- Not covered by a deterministic check: AC11 depends on organic post-restart traffic; if no qualifying positive control exists after a later restart, use the plan's Story 1.2.3 fallback (needs a free backlog WIP slot).
