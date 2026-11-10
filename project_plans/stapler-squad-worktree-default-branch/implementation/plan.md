# Implementation Plan: stapler-squad-worktree-default-branch

**Feature**: Close the two open acceptance criteria of backlog item 9c33e913 (AC9 `make test` exits 0; AC11 no real `does not support extension: worktreeconfig` errors after deploy). The code fix is already merged (#1430, `afbc063dd`); this is a verification-only plan. Code changes happen only if a verification step finds a defect.
**Date**: 2026-11-04
**Status**: Ready for implementation
**ADRs**: None (no new decision; the fork-vs-upstream decision stands per `research/build-vs-buy.md`; see `project_plans/fix-worktreeconfig-extension-compat/decisions/ADR-001-fork-lowercase-normalization.md`)

Evidence labels: VERIFIED = command run or file read while planning (2026-11-04); INFERRED = not run.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `worktreeConfig repo` | A repo whose `.git/config` has `extensions.worktreeconfig=true` with `core.repositoryformatversion=0` | This repo is one (VERIFIED: `git config --get-regexp 'extensions\|repositoryformat'`); every linked worktree shares it |
| `ssq.3` | Fork tag `github.com/tstapler/go-git/v5 v5.19.3-ssq.3` (`go.mod:232`), lowercase `worktreeconfig` in both extension maps | The only tag that carries the fix across all go-git callers |
| `ResolveRef port` | `ops.go` `ResolveLocalBranchSHA` / `ResolveOriginBranchSHA` routed via `backend.ResolveRef` | Covers two functions only; other go-git openers rely on `ssq.3` |
| `deployed binary` | `/home/tstapler/Programming/stapler-squad/stapler-squad`, the `ExecStart` of the user service | A different checkout than this worktree; `make install-service` here would not rebuild it |
| `restart instant` | `2026-11-04T12:27:48-08:00`, service `ActiveEnterTimestamp` | Lower bound of the AC11 evidence window; compared lexically against the log's `time` field |
| `live log` | `~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log` plus rotated `staplersquad-*.log.gz` | NOT `~/.stapler-squad/logs/` (stale since Sep 5) |
| `err field` | The `err` / `error` key of a JSON log record | The only place the real failure appears; `program` holds prompt echoes |
| `prompt echo` | A log record (msg `creating session`, `tmux session created successfully`) whose `program` field embeds the backlog description text | Contains the error phrase verbatim, so a raw grep false-positives |
| `negative control` | Proof the AC11 filter CAN match: pre-fix records in rotated logs (err field, last hit `2026-11-04T12:24:23`, first `10:32:03` in `staplersquad-2026-11-04T13-51-53.139.log.gz`) | VERIFIED |
| `positive control` | Proof the guarded code path succeeds on a worktreeConfig repo | The two named tests: `TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension` (`session/repo_path_test.go:598`, must not be skipped) and the `session/git` `Succeeds_When_RepoHasWorktreeConfigExtension` tests. The `2026-11-04T15:25:17` "set base commit SHA ... merge-base" record is NOT sufficient (it comes from initBaseCommitSHA/MergeBase, `worktree_ops.go:271`, not the default-branch resolve path); it may be cited only as supplementary |
| `contaminated TMPDIR` | A temp dir with a `.jj`/`.git` ancestor (here `TMPDIR=/home/tstapler/.cache/tmp`, under `~/.jj`) | Triggers the `session/vc` host-dependence; running `make test` under it is the worst case |
| `EVIDENCE_DIR` | `project_plans/stapler-squad-worktree-default-branch/implementation/evidence/` | Holds the local copy of the rotated log and `SHA256SUMS`; only the manifest is committed |
| `foreign extension` | Any `extensions.*` other than the allow-list (`objectformat`, `refstorage`, `relativeworktrees`, ...) | Still rejected by the fork; a different bug, not an AC11 failure |

---

## Pattern Decisions

Step 0.5 alternatives (strength / weakness):
1. **Run-and-show verification, fix only on defect** (chosen): strength: matches "run it, don't read it" and the merged fix; weakness: AC11 evidence depends on the live log being representative.
2. **Add a new end-to-end Go test for AC11** (rejected): strength: deterministic; weakness: `session/git/ops_test.go`, `session/git/native/gitconfig/*_test.go` and `session/repo_path_test.go` already mention worktreeconfig (VERIFIED by grep), so it duplicates #1430 coverage and proves nothing about the deployed binary.
3. **Redeploy with `make install-service` then re-observe** (rejected): strength: fresh restart instant; weakness: the deployed binary already embeds `ssq.3` (VERIFIED `go version -m`), the unit runs from another checkout, and a restart ships an in-progress worktree build (see `CLAUDE.md` manual-testing note).

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| AC9 proof | Transaction Script: one recorded `make test` run with exit code and tail | PoEAA (Fowler) | New test harness | The criterion is a command result; nothing to model |
| AC11 evidence | Controlled experiment: negative control + positive control + filtered window | Test design | Bare `grep -c` | Zero matches against a window with no traffic, or a grep that cannot match, proves nothing |
| Log filter | Match on structured `err`/`error` fields via `jq`, not raw text | Type-driven design (parse, don't grep) | Raw `grep worktreeconfig` | The session/branch name itself contains `worktreeconfig`: raw `tostring` matching returned 9492 false hits (VERIFIED) |
| Evidence record | Append-only results note in this plan dir | n/a | PR-body-only | Survives the session; PR body gets a 3-line summary |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|-------------|---------------|
| `session/vc/main_test.go` | Relocation to `/tmp` is not re-checked for its own VCS ancestor (pitfalls Gap 1) | Extend as-is | Latent only; fix (Task T-AC9-6) is conditional on a failing run. No new violation added |
| `go.mod:232` fork `replace` | Hides upstream version; must track v5 releases | Extend as-is | `research/build-vs-buy.md`: keep fork; revisit at v6 GA or a v5.19.4+ carrying lowercase keys |
| Other go-git `OpenRepo` consumers (`session/unfinished/*`, `repoview.go`, `tools/scanner-workload`) | Rely on the fork pin alone, not the ResolveRef port | Extend as-is | Covered by `ssq.3`; Task T-AC11-3 adds a pin-regression check rather than porting them |

---

## Observability Plan
- **Logs**: no new log lines. Existing records are the evidence source (`err` field).
- **Metrics**: none added.
- **Alerts**: no new alerts required. Recurrence signal: `jq` filter from Task T-AC11-5 run on demand.

## Risk Control
- **Feature flag**: not gated (no product code change planned).
- **Rollback procedure**: standard revert via PR close + revert commit for any test change; fork rollback is `docs/how-to/roll-back-go-git-fork.md`.
- **Staged rollout**: full rollout on merge. Do NOT run `make install-service` from this worktree: the live service runs a different checkout and backlog work depends on it staying up.

## Unresolved Questions
None. (The log path, binary provenance, restart instant and positive control were all resolved while planning; Tasks T-AC11-1 to T-AC11-5 re-verify them at execution time because the log keeps growing.)

## Dependency Visualization

```
Wave 1:  T-AC9-1 prereqs --> T-AC9-2 make test (+ 60s load/mem monitor)
         in parallel, read-only only: T-AC11-1 provenance, T-AC11-3 pin check, T-AC11-4 negative control
                                   |
                                   v   (make test finished; end-capture ActiveEnterTimestamp + MainPID)
Wave 2:  load re-check (uptime/free -g) --> T-AC9-3 tymux -race x20 --> T-AC9-4 vc both TMPDIRs
         --> T-AC11-6 positive control (go test) --> T-AC11-2 restart instant + T-AC11-1 re-run --> T-AC11-5 window --> T-AC11-7
                                   |
                                   v   (red? T-AC9-5 triage --> T-AC9-6 fix = conditional Wave 4)
Wave 3:  T-CLOSE-1 evidence note --> T-CLOSE-2 report_progress
```
Nothing that runs `go test` or `-race` overlaps `make test` on the live host: T-AC9-3, T-AC9-4 and T-AC11-6 start only after T-AC9-2 exits and a fresh load/memory re-check passes the T-AC9-1 thresholds. Wave 1's AC11 tasks only read files, `systemctl` and the log.

---

## Phase 1: Verify and close

*ID note*: Epic and Story IDs are numeric (`1.1`, `1.1.1`); task IDs are `T-AC9-n` (AC9 tasks), `T-AC11-n` (AC11 tasks) and `T-CLOSE-n` (close-out), so no ID can be read as another level. The diagram uses task IDs.

### Epic 1.1: AC9, `make test` exits 0
**Goal**: A recorded `make test` run (exit code plus output tail) from HEAD, with the two hermeticity fixes from `b6820123c` shown to hold.

#### Story 1.1.1: Full-suite run
**As a** maintainer, **I want** `make test` to exit 0 from a clean build at HEAD, **so that** AC9 is closed with evidence rather than inference.
**Acceptance Criteria**:
- `make test` exits 0 with `TMPDIR` set to a contaminated dir.
  - *Given* a repo checkout at HEAD with `TMPDIR=/home/tstapler/.cache/tmp` (under `~/.jj`), *When* `bash -o pipefail -c 'make test > ~/.cache/tmp/ssq-ac9/make-test.log 2>&1'; echo "exit=$?"` runs, *Then* pass evidence is exit code `0` AND `grep -c '^--- FAIL\|^FAIL' <full log>` equals `0` over the whole log (not `tail -n 200`, which can hide an early failure).
  - Prior art: `make test` already ran once at `EXIT=0` (5433 + 14638 tests) with `ActiveEnterTimestamp` unchanged (`2026-11-04 12:27:48 PST`); that run is context, not a substitute for the recorded run here.
- The run is not satisfied by a stale sentinel or subset target.
  - *Given* the Makefile target `test` (`Makefile:658`), *When* the run is recorded, *Then* the note names the literal command `make test` (not `test-affected`) and shows gotestsum's final `DONE <N> tests` summary line.
**Files**: none changed. Evidence only.

##### Task T-AC9-1: Prerequisites and load check (XS)
- Live-service guards (this host runs the live service and backlog work):
  - `list_backlog_items`: in_progress count must be within the WIP cap of 2; otherwise wait.
  - Numeric abort thresholds (pre-start): `uptime` load1 > `nproc` -> abort and wait; `free -g` available < 8 GiB -> abort and wait; `/tmp` tmpfs near full -> abort (memory notes record OOM restarts from heavy scratch).
  - Do NOT export `STAPLER_SQUAD_TMUX_SOCKET` for `make test`: VERIFIED defect in the original plan, exporting it makes 10 `*_UsesIsolatedSocket` / `TestResolveSocket` / `TestPrependSocket` tests fail (they assert the unset-env test-mode default). Test mode already isolates the socket; keep scratch on disk (`~/.cache/tmp/ssq-ac9/`), not tmpfs `/tmp`.
  - Capture BEFORE: `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p MainPID` (expected `2026-11-04 12:27:48 PST`, i.e. restart instant `2026-11-04T12:27:48-08:00`; record the `ExecStart` MainPID). The PID catches a restart that leaves the timestamp reading the same.
  - Socket isolation check (must pass before `make test` starts): `grep -rn 'EnvSocketVar\|STAPLER_SQUAD_TMUX_SOCKET' session/tmux/tmux.go` (const at `session/tmux/tmux.go:653`), then grep the tmux-using test packages (`session`, `session/tmux`, `session/mux`) for `ResolveSocket`/`EnvSocketVar` to confirm they honor `STAPLER_SQUAD_TMUX_SOCKET`. If any tmux-using package does not, record it and do not run `make test` against the live tmux server. Use the single socket name `ssq-verify-1` everywhere.
- `unset npm_config_user_agent` (web-app Bun guard); `echo $TMPDIR` and `ls ~/.jj` to record that the contaminated case is in effect.
- `make build` (generates `gen/` and `session/ent/*`, gitignored; also proves lint passes). If `make build` fails on lint for a non-test reason, report it as a separate finding and do NOT mask it (no skipping lint, no editing around it); AC9 proceeds only via `make test`'s own prerequisites, and the finding is recorded in `evidence.md`.
- Files: none.

##### Task T-AC9-2: Run `make test` and record result (S)
- Run in the background with a log on disk (not `/tmp`): `mkdir -p ~/.cache/tmp/ssq-ac9 && bash -o pipefail -c 'make test > ~/.cache/tmp/ssq-ac9/make-test.log 2>&1'; echo "exit=$?" >> ~/.cache/tmp/ssq-ac9/make-test.log`. Two serialized gotestsum phases, each with `-timeout=20m` (`Makefile:673-674`). Expected wall clock: the prior run took ~50 s + ~155 s of test time with part of it cached; budget 5 to 15 minutes cold.
- Mid-run abort rule: start a background monitor with the run (`MT=<make pid>`): every 60 s append `uptime` load1 and `free -g` available to `~/.cache/tmp/ssq-ac9/monitor.log`; if load1 > `nproc*1.5` or available < 4 GiB, log `ABORT`, `kill $MT` and its children (`kill -- -$(ps -o pgid= -p $MT | tr -d " ") (process-group kill; start make under setsid so the group is its own)`, scoped to that PID only), wait for load to settle, re-baseline T-AC9-1, and restart. An aborted run is never reported as pass.
- Record: `tail -n 40`, the `exit=` line, and `grep -c '^--- FAIL\|^FAIL' <log>` over the full log (must be 0; pass = exit 0 AND count 0).
- Capture AFTER: `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p MainPID`. If either differs from the BEFORE value, the service restarted mid-run: re-baseline the AC11 restart instant (Task T-AC11-2) and re-run T-AC11-1 to T-AC11-7 against it.
- Recovery if a live session drops during the run: `resume_session` for each affected session (check `list_sessions`), and `tmux -L <socket> list-sessions` against the live socket to confirm sessions survive; record the incident in `evidence.md`. Do not run `make install-service`.
- If exit is 0: go to Story 1.1.2 (T-AC9-3 after a load re-check). If non-zero: Task T-AC9-5.
- Files: none (log in `~/.cache/tmp/ssq-ac9/`).

#### Story 1.1.2: Hermeticity fixes are not vacuous
**As a** maintainer, **I want** the `session/vc` and `session/tymux` fixes shown to work under repetition and under both temp-dir environments, **so that** one lucky green run is not mistaken for a fix.
**Acceptance Criteria**:
- tymux concurrent-poll test is stable and not vacuous.
  - *Given* `TestTymuxGRPCSession_ActivityConcurrentPolls` (8 pollers, one shared `PaneSnapshot`), *When* `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m` runs, *Then* it exits 0 and each iteration enforces `assert.EqualValues(t, 1, updates.Load())` (VERIFIED present at `session/tymux/session_test.go:722`, so the moved `HasMeaningfulContent` assertion cannot pass with zero updates).
- vc tests pass under both a contaminated and a clean temp dir.
  - *Given* `session/vc/main_test.go` `TestMain`, *When* `go test ./session/vc -count=1` runs once with `TMPDIR=/home/tstapler/.cache/tmp` and once with `TMPDIR=/tmp`, *Then* both exit 0.
**Files**: none changed (read `session/tymux/session_test.go`, `session/vc/main_test.go`).

##### Task T-AC9-3: tymux repetition run (XS)
- Runs only after T-AC9-2 has finished, and only after a fresh `uptime`/`free -g` re-check meeting the T-AC9-1 thresholds (load1 <= `nproc`, available >= 8 GiB). Never concurrent with `make test` on the live host.
- `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m -v 2>&1 | tail -n 15`; record exit code.
- Re-read the test body once to confirm the `updates.Load()` assertion is still after `wg.Wait()`.
- Files: none.

##### Task T-AC9-4: vc under both environments (XS)
- `TMPDIR=/home/tstapler/.cache/tmp go test ./session/vc -count=1`; then `TMPDIR=/tmp go test ./session/vc -count=1`. Record both exits.
- Check pitfalls Gap 1 cheaply: `ls -d /.git /.jj /tmp/.git /tmp/.jj 2>&1` (expect all "No such file").
- Files: none.

#### Story 1.1.3: Defect handling (conditional)
**As a** maintainer, **I want** any red result triaged against `main` before a code change, **so that** unrelated known flakes (BUG-087..090) do not become scope creep and real regressions get a root-cause fix.
**Acceptance Criteria**:
- A failing test is classified before anything is edited.
  - *Given* a `make test` run where `session/mux` `TestFoo` fails (example), *When* triage runs (`go test <pkg> -run TestFoo -count=3 -race`, and `gh run list --repo tstapler/stapler-squad --branch main --limit 5`), *Then* the note records: failing in isolation yes/no, failing on `main` CI yes/no, and a root-cause sentence of the form "fails because X". A fix is made only for failures that reproduce in isolation or touch `b6820123c` files.
- If no failure occurred, this story is closed with "not triggered".
**Files**: only if triggered: the failing test's file, plus `session/vc/main_test.go` for Task T-AC9-6.

##### Task T-AC9-5: Triage a red run (S, conditional)
- For each failing test: rerun in isolation, check `main` CI, state root cause, label VERIFIED/INFERRED. Per memory (`fix-flaky-tests-dont-defer`), fix flakes found rather than re-deferring; ship as its own commit.
- Files: failing test file (variable).

##### Task T-AC9-6: Harden `session/vc` TestMain (XS, only if T-AC9-4 or T-AC9-2 shows the `/tmp` relocation itself contaminated)
- In `session/vc/main_test.go`, after `os.MkdirTemp("/tmp", ...)`, re-run `isJujutsuRepo(dir) || isGitRepo(dir)` and `os.Exit(1)` with a message if true. Then `go test ./session/vc -count=1` under both TMPDIRs.
- Files: `session/vc/main_test.go`.

### Epic 1.2: AC11, no real worktreeconfig errors after deploy
**Goal**: Show the binary serving traffic since the restart instant embeds `ssq.3`, that the log filter can detect the old failure (negative control), that the guarded path ran successfully on a worktreeConfig repo post-restart (positive control), and that the window contains zero real errors.

#### Story 1.2.1: Binary and window provenance
**As a** maintainer, **I want** proof the running process is built from the fix, **so that** a clean log is attributable to the fix and not to an old binary.
**Acceptance Criteria**:
- Deployed binary carries `ssq.3`.
  - *Given* `deployed binary` at `/home/tstapler/Programming/stapler-squad/stapler-squad`, *When* `go version -m <path> | grep go-git`, *Then* output includes `github.com/tstapler/go-git/v5 v5.19.3-ssq.3` (VERIFIED at planning time).
- Deployed binary's `vcs.revision` descends from the fix commit.
  - *Given* `go version -m <path>` (VERIFIED: `vcs.revision=afbc063dd`, `vcs.modified=true`), *When* `git merge-base --is-ancestor afbc063dd <vcs.revision>` runs (trivially true when equal), *Then* exit 0; the note records `vcs.modified=true` (uncommitted changes in the build tree) as a caveat, with `ssq.3` in the embedded module list as the load-bearing proof.
- Binary started after it was built and after the fix commit.
  - *Given* the service `ActiveEnterTimestamp`, *When* compared with `stat -c %y <path>` and `git log -1 --format=%cI afbc063dd`, *Then* build mtime (12:27:36) is later than the commit time (`2026-11-04T12:26:40-08:00`) and earlier than the restart instant (12:27:48).
- The pin cannot silently regress.
  - *Given* `go.mod` at HEAD, *When* `grep -n 'tstapler/go-git/v5' go.mod`, *Then* line 232 shows `v5.19.3-ssq.3` (not `ssq.1` / `ssq.2` / upstream), since a regression reintroduces the error everywhere except the two ResolveRef-ported functions.
**Files**: none changed.

##### Task T-AC11-1: Binary provenance (XS)
- `go version -m /home/tstapler/Programming/stapler-squad/stapler-squad | grep -E 'go-git|vcs.revision|vcs.time|vcs.modified'`; `stat -c '%y' /home/tstapler/Programming/stapler-squad/stapler-squad`.
- Assert the `vcs.revision` (VERIFIED `afbc063dd`) descends from `afbc063dd`: `git merge-base --is-ancestor afbc063dd <vcs.revision>`; record `vcs.modified=true`.
- Files: none.

##### Task T-AC11-2: Restart instant and live log path (XS)
- `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p ExecStart`; confirm the live log by `ls -la ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log` (mtime within the last minutes) and its first record's `time`.
- If the service was restarted again since 12:27:48, use the newest restart instant and re-run T-AC11-1 on the new binary.
- Files: none.

##### Task T-AC11-3: Pin check (XS)
- `grep -n 'tstapler/go-git/v5' go.mod` and `git ls-remote --tags https://github.com/tstapler/go-git v5.19.3-ssq.3` (expect `a328acc1...`).
- Files: none.

#### Story 1.2.2: Controlled log search
**As a** maintainer, **I want** a filter that ignores prompt echoes and matches only real errors, validated against known-bad history, **so that** zero means zero.
**Acceptance Criteria**:
- Filter detects the old failure (negative control).
  - *Given* rotated log `staplersquad-2026-11-04T13-51-53.139.log.gz` (pre-fix records), *When* `zcat <file> | jq -Rc 'fromjson? | select(((.err // .error // "")|tostring)|test("does not support extension|unknown extension";"i")) | .time' | sed -n '1p;$p'`, *Then* it prints a first/last pair (VERIFIED: `2026-11-04T10:32:03` and `2026-11-04T12:24:23`), proving the filter can match.
- Filter ignores prompt echoes.
  - *Given* the post-restart `creating session` / `tmux session created successfully` records whose `program` field embeds the backlog text (VERIFIED: 3+3 in the current file), *When* the same filter runs on `staplersquad.log` with `.time >= "2026-11-04T12:27:48-08:00"`, *Then* it returns 0 lines.
- Window has no real errors, in any extension.
  - *Given* the live log plus rotated files overlapping the window, *When* the filter runs over `zcat staplersquad-2026-11-04T13-51-53.139.log.gz` and `staplersquad.log` restricted to `.time >= restart instant`, *Then* 0 records have a matching `err`/`error`; any hit for a `foreign extension` is reported separately as a different bug.
- Window is long enough to matter.
  - *Given* pre-fix rate of the same error (VERIFIED: 9802 `failed to check git status` hits in `staplersquad-2026-11-02T12-52-06.943.log.gz`), *When* the window length is computed (`restart instant` to last log record), *Then* the note states the window length and that at the pre-fix rate a regression would have produced many hits.
**Files**: none changed.

##### Task T-AC11-4: Negative control, evidence copy and time-format check (XS)
- Preserve the rotated log: `cp ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad-2026-11-04T13-51-53.139.log.gz $EVIDENCE_DIR/ && (cd $EVIDENCE_DIR && sha256sum *.gz > SHA256SUMS)`, where `EVIDENCE_DIR=project_plans/stapler-squad-worktree-default-branch/implementation/evidence/`; record the hash in `evidence.md`. Commit only `SHA256SUMS`, not the `.gz` (large; the log is a local runtime artifact) (rotation or cleanup could otherwise destroy the negative-control source).
- Confirm the log `.time` field format and offset before any lexical comparison: print one record's `.time` (expected `-08:00` offset) and check it matches the restart instant's zone (`ActiveEnterTimestamp` 12:27:48 PST = `2026-11-04T12:27:48-08:00`). If the offsets differ, normalize before comparing.
- Run the `zcat ... | jq` command above on `staplersquad-2026-11-04T13-51-53.139.log.gz`; also `zcat ... | jq -Rc 'fromjson? | select(tostring|test("does not support extension";"i")) | to_entries | map(select(.value|tostring|test("does not support extension";"i"))|.key)|join(",")' | sort | uniq -c` to confirm which keys hold the phrase (VERIFIED: `err`, `error`, `program`).
- Files: none.

##### Task T-AC11-5: Window search (XS)
- Sequencing: run this AFTER the `make test` end-capture of `ActiveEnterTimestamp` (Task T-AC9-2 AFTER), or re-run it then, so the restart instant used is the final one.
- Current file: `jq -Rc 'fromjson? | select(.time >= "2026-11-04T12:27:48-08:00") | select(((.err // .error // "")|tostring)|test("does not support extension|unknown extension";"i")) | {time,msg,err}' ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log`.
- Same filter over `zcat ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad-2026-11-04T13-51-53.139.log.gz` (and any newer `.gz`). Do NOT rely on the bare word `worktreeconfig` (branch/session names contain it).
- Also record `grep -c` of the raw phrase restricted to msg types `creating session` / `tmux session created successfully` to show the echo noise is explained (VERIFIED: 6).
- Files: none.

#### Story 1.2.3: Positive control on a worktreeConfig repo
**As a** maintainer, **I want** evidence that the default-branch/worktree path succeeds on a worktreeConfig repo, **so that** the zero count reflects exercised code.
**Acceptance Criteria**:
- Named test runs pass and are not skipped.
  - *Given* HEAD, *When* `go test ./session -run TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension -count=1 -v` runs (`session/repo_path_test.go:598`), *Then* exit 0 and the output shows `--- PASS` (not `--- SKIP`).
  - *Given* HEAD, *When* `go test ./session/git -run 'Succeeds_When_RepoHasWorktreeConfigExtension' -count=1 -v` runs, *Then* exit 0 and `--- PASS` for each matched test.
- The window filter finds nothing real.
  - *Given* the jq filter of Story 1.2.2, *When* run with `.time >= ActiveEnterTimestamp`, *Then* 0 real hits.
- The weak record is supplementary only.
  - *Given* the `2026-11-04T15:25:17` `set base commit SHA for branch to merge-base` record (`sha":"5d3b16ed"`, `with_branch":"main"`), *Then* it is NOT sufficient on its own: it comes from initBaseCommitSHA/MergeBase, not the default-branch resolve path.
- The repo really has the extension.
  - *Given* the repo common dir (`git rev-parse --git-common-dir`), *When* `git config --get-regexp '^extensions\.|repositoryformatversion'`, *Then* output includes `extensions.worktreeconfig true` and `core.repositoryformatversion 0`.
- Optional real-session control (a deliberate deviation from the pre-mortem text, which called for a fresh session start on the repo).
  - The two named tests above are the mandatory control, so the fresh new-worktree session start is optional. Reason: it consumes one of the 2 WIP slots and runs real session-start work on the live-service host, a risk the deterministic tests avoid. Do it only when a slot is free.
  - *Given* the in_progress count is under the 2-session WIP cap, *When* a session is created on this repo via the normal path, *Then* a new `set base commit SHA`/`connected to existing worktree` record appears and the filter still returns 0. Otherwise the two named test runs suffice.
**Files**: none changed.

##### Task T-AC11-6: Positive control (S)
- Run both named test commands above; record exit codes and `--- PASS`/`--- SKIP` lines.
- Supplementary: `jq -Rc 'fromjson? | select(.time >= "2026-11-04T12:27:48-08:00") | select(.msg|test("set base commit SHA|found existing worktree for branch|connected to existing worktree")) | del(.program)' <live log>` (and the 13-51 `.gz`); `git config --get-regexp 'extensions|repositoryformatversion'` from this worktree.
- Gate: AC11 is reportable only if both named tests passed (not skipped) and the jq filter returned 0 real hits.
- Files: none.

##### Task T-AC11-7: Extension survey for the discriminator note (XS)
- `git --version` (VERIFIED 2.53.0); `git config --get-regexp '^extensions\.'` in the repo; list any extension other than `worktreeconfig`. If `objectformat`/`refstorage`/`relativeworktrees`/others appear, record them as a follow-up backlog item (separate bug) and do not widen this one.
- Files: none.

### Epic 1.3: Close-out
**Goal**: Evidence is durable and the two criteria are marked in the backlog item.

#### Story 1.3.1: Record and report
**As a** reviewer, **I want** the commands and outputs in one note and the backlog ACs updated, **so that** closure is checkable without rerunning anything.
**Acceptance Criteria**:
- Evidence note exists with exact commands and outputs.
  - *Given* Tasks T-AC9-2 to T-AC9-4, T-AC11-1 to T-AC11-7 complete, *When* `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md` is written, *Then* it holds one section per AC with each command, its exit code or count, and a VERIFIED/INFERRED label.
- Criteria reported only when proven.
  - *Given* `make test` exit `0` and window filter count `0` with controls satisfied, *When* `report_progress` runs for item `9c33e913-0aa0-4eeb-b6a0-ee33f15ad8a8` with `criteria_index=8` (AC9) and `criteria_index=10` (AC11, 0-indexed per the `backlog:done-N` skills), *Then* both read back as passing via `get_backlog_item` (mutation read back, not assumed). If a control is unmet, that AC is reported as `fail` with the reason.
- AC11 `report_progress` message content.
  - *Given* the AC11 report, *Then* the message states (a) the log-path reinterpretation: the live log is `~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/`, not `~/.stapler-squad/logs/` named in the requirement; (b) the window length (restart instant to last record); (c) the evidence: both named tests passed, filter 0 hits, negative control pair.
  - *And* `evidence.md` is committed and pushed BEFORE `report_progress` is called, so the message can cite a retrievable path.
**Files**: `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md` (new, justified: durable evidence trail requested by the AC wording "after deploying").

##### Task T-CLOSE-1: Write the evidence note (S)
- Assemble outputs from Epic 1.1 and 1.2 into `evidence.md`; re-read adversarially once (every number recounted from a command, every path exists).
- Files: `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md`.

##### Task T-CLOSE-2: Report to backlog (XS)
- Order: (1) commit and push the plan artifacts and `evidence.md` first (stage only these files, including `evidence/SHA256SUMS` but never `evidence/*.gz`; no `git add -A`; `git status` first: `research/` is currently untracked); (2) then call `report_progress` for AC9 (index 8) and AC11 (index 10); (3) `get_backlog_item` to read back status.
- AC11 gate: only if Task T-AC11-6's named tests passed (not skipped) and the filter returned 0 real hits since the final `ActiveEnterTimestamp`. The message must state the log-path reinterpretation, the window length, and that evidence (see Story 1.3.1).
- If Task T-AC11-1/T-AC11-2 are re-run (P2 re-run after any restart or after `make test`), do it before this task.
- Files: `project_plans/stapler-squad-worktree-default-branch/` (add).

---

## Effort estimate

Weights per `ESTIMATION.md`: CU = in + 1.25·cache_write + 0.1·cache_read + 5·out. Assumed output share ~5-10% (read/run-heavy, little code). Costs are dominated by waiting on `make test`, not tokens.

| Story | Tasks | Raw tokens (est.) | Assumed output share | CU (est.) |
|---|---|---|---|---|
| 1.1.1 full-suite run | T-AC9-1 XS, T-AC9-2 S | ~60k | 5% (log tails only) | ~75k |
| 1.1.2 hermeticity | T-AC9-3 XS, T-AC9-4 XS | ~40k | 5% | ~50k |
| 1.1.3 defect handling (conditional) | T-AC9-5 S, T-AC9-6 XS | ~0 / up to ~100k | 10% | 0 / ~150k |
| 1.2.1 provenance | T-AC11-1 to T-AC11-3 XS | ~40k | 5% | ~50k |
| 1.2.2 controlled search | T-AC11-4, T-AC11-5 XS | ~50k | 5% | ~60k |
| 1.2.3 positive control | T-AC11-6, T-AC11-7 XS | ~40k | 5% | ~50k |
| 1.3.1 close-out | T-CLOSE-1 S, T-CLOSE-2 XS | ~70k | 15% (writes the note) | ~100k |
| amendment work | extra `go test` runs (repo_path_test, session/git), `systemctl` before/after captures, WIP/load/free pre-checks, rotated-log copy + sha256, P2 re-run of T-AC11-1/T-AC11-2 before T-CLOSE-2 | ~45k | 5% | ~55k |
| **Subtotal** | 15 tasks (2 conditional: T-AC9-5, T-AC9-6) + amendment work | ~345k (+100k if triggered) | | ~440k (+150k) |
| Verification multiplier | x1.5 for review of evidence note | | | ~660k |

- **Size band**: low Medium (~0.66M CU with amendments, ~0.8M if the conditional defect path triggers).
- **Critical path (agent waves)**: 3 waves, plus a conditional 4th. Wave 1: T-AC9-1 and T-AC9-2 (`make test` with monitor) in parallel with the read-only AC11 tasks (T-AC11-1, T-AC11-3, T-AC11-4). Wave 2, after `make test` ends and a load re-check passes: T-AC9-3, T-AC9-4, T-AC11-6, then the final restart-instant tasks T-AC11-2, T-AC11-1 re-run, T-AC11-5, T-AC11-7. Wave 3: T-CLOSE-1, T-CLOSE-2 (commit and push `evidence.md` and `SHA256SUMS`, then `report_progress`). Conditional Wave 4 only if T-AC9-5/T-AC9-6 trigger.
- **Overrun checkpoint**: if spend exceeds the table value by > 25% for completed stories, pause and report the delta with its cause (informational).
- **Wall-clock blockers**: (1) `make test` duration: ~50 s + ~155 s of test time in the prior run (partly cached); allow 5 to 15 minutes cold. Two serialized gotestsum phases with 20m timeouts each (owner: none, machine time; avoid running concurrently with other tmux-heavy work or Bazel JVMs on this tmpfs-/tmp host). (2) AC11 needs organic or triggered session traffic on a worktreeConfig repo after the restart instant; the named tests in Story 1.2.3 supply the positive control deterministically; the optional real-session control needs a free backlog WIP slot (owner: the operator).

## Flagged choices
1. Filter AC11 on structured `err`/`error` fields, not the word `worktreeconfig`: the branch and session names contain it (9492 false hits VERIFIED).
2. AC11 requirement text names `~/.stapler-squad/logs/staplersquad.log`; the real log is the workspace-scoped path above. Evidence uses the real path and states the discrepancy.
3. One `make test` run under the contaminated `TMPDIR` (the harness default here) instead of two full runs; the clean-`/tmp` comparison is done at package level (Task T-AC9-4) to avoid doubling a long, tmux-heavy run.
4. No redeploy: the service already runs an `ssq.3` binary started after the fix.
5. No code change planned. The tymux assertion is already non-vacuous (`EqualValues(1, updates.Load())`); the vc re-check hardening is conditional.

## Pre-mortem P1 amendments (phase 4)

Folded into the task and story bodies above; this section is a pointer only. P1 #1 (AC11 positive controls) -> Story 1.2.3, Task T-AC11-6, Task T-CLOSE-2. P1 #2 (`make test` on the live-service host) -> Task T-AC9-1, Task T-AC9-2, Story 1.1.1.
