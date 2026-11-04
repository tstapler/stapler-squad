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
| `positive control` | Proof the guarded code path ran post-restart on a worktreeConfig repo without error | WEAK candidate only (adversarial review: that record comes from initBaseCommitSHA/MergeBase, worktree_ops.go:271, not the default-branch resolve path); strengthen with the worktreeconfig test in session/repo_path_test.go or a real new-worktree session start. Weak record: `2026-11-04T15:25:17` "set base commit SHA for branch to merge-base" for this branch |
| `contaminated TMPDIR` | A temp dir with a `.jj`/`.git` ancestor (here `TMPDIR=/home/tstapler/.cache/tmp`, under `~/.jj`) | Triggers the `session/vc` host-dependence; running `make test` under it is the worst case |
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
| `session/vc/main_test.go` | Relocation to `/tmp` is not re-checked for its own VCS ancestor (pitfalls Gap 1) | Extend as-is | Latent only; fix (Task 1.3b) is conditional on a failing run. No new violation added |
| `go.mod:232` fork `replace` | Hides upstream version; must track v5 releases | Extend as-is | `research/build-vs-buy.md`: keep fork; revisit at v6 GA or a v5.19.4+ carrying lowercase keys |
| Other go-git `OpenRepo` consumers (`session/unfinished/*`, `repoview.go`, `tools/scanner-workload`) | Rely on the fork pin alone, not the ResolveRef port | Extend as-is | Covered by `ssq.3`; Task 2.1c adds a pin-regression check rather than porting them |

---

## Observability Plan
- **Logs**: no new log lines. Existing records are the evidence source (`err` field).
- **Metrics**: none added.
- **Alerts**: no new alerts required. Recurrence signal: `jq` filter from Task 2.2b run on demand.

## Risk Control
- **Feature flag**: not gated (no product code change planned).
- **Rollback procedure**: standard revert via PR close + revert commit for any test change; fork rollback is `docs/how-to/roll-back-go-git-fork.md`.
- **Staged rollout**: full rollout on merge. Do NOT run `make install-service` from this worktree: the live service runs a different checkout and backlog work depends on it staying up.

## Unresolved Questions
None. (The log path, binary provenance, restart instant and positive control were all resolved while planning; Task 2.1/2.2 re-verify them at execution time because the log keeps growing.)

## Dependency Visualization

```
Epic 1 (AC9)                         Epic 2 (AC11)                    Epic 3
1.1a prereqs --> 1.1b make test      2.1a provenance --+
                    |                2.1b restart       +--> 2.2a window --> 2.2b filter run
                    v                2.1c pin check ---+        |               |
1.2a tymux -race x20 (parallel)                                 v               v
1.2b vc under both TMPDIRs                              2.3a positive ctrl --> 3.1a evidence note
        |                                                                          |
        +--> (red? 1.3a triage --> 1.3b fix) --------------------------------------+--> 3.1b report_progress
```
Epics 1 and 2 are independent and can run as parallel agents (one agent runs `make test`, which is CPU/tmux heavy; the other only reads logs).

---

## Phase 1: Verify and close

*Task ID note*: task IDs are `<group><n><letter>` where group 1.x = AC9 tasks, 2.x = AC11 tasks, 3.x = close-out; they do not track the story numbers, and the dependency diagram uses the task IDs.

### Epic 1.1: AC9, `make test` exits 0
**Goal**: A recorded `make test` run (exit code plus output tail) from HEAD, with the two hermeticity fixes from `b6820123c` shown to hold.

#### Story 1.1.1: Full-suite run
**As a** maintainer, **I want** `make test` to exit 0 from a clean build at HEAD, **so that** AC9 is closed with evidence rather than inference.
**Acceptance Criteria**:
- `make test` exits 0 with `TMPDIR` set to a contaminated dir.
  - *Given* a repo checkout at `a80d6614c` with `TMPDIR=/home/tstapler/.cache/tmp` (under `~/.jj`), *When* `bash -o pipefail -c 'make test 2>&1 | tail -n 200 > $SCRATCH/make-test.tail'` runs, *Then* exit status is `0` and the tail shows no `FAIL` lines from `session/vc` or `session/tymux`.
- The run is not satisfied by a stale sentinel or subset target.
  - *Given* the Makefile target `test` (`Makefile:658`), *When* the run is recorded, *Then* the note names the literal command `make test` (not `test-affected`) and shows gotestsum's final `DONE <N> tests` summary line.
**Files**: none changed. Evidence only.

##### Task 1.1a: Prerequisites and load check (XS)
- `unset npm_config_user_agent` (web-app Bun guard); `uptime`, `free -g` (abort and wait if load is high or `/tmp` tmpfs is near full: memory notes record OOM restarts from heavy scratch); `echo $TMPDIR` and `ls ~/.jj` to record that the contaminated case is in effect.
- `make build` (generates `gen/` and `session/ent/*`, gitignored; `make test` has `proto-gen` as a prerequisite but `make build` also proves lint passes).
- Files: none.

##### Task 1.1b: Run `make test` and record result (S)
- Run in the background with a log on disk (not `/tmp`): `mkdir -p ~/.cache/tmp/ssq-ac9 && bash -o pipefail -c 'make test > ~/.cache/tmp/ssq-ac9/make-test.log 2>&1'; echo "exit=$?" >> ~/.cache/tmp/ssq-ac9/make-test.log`. Two serialized gotestsum phases, each with `-timeout=20m` (`Makefile:673-674`).
- Record: `tail -n 40`, the `exit=` line, and `grep -c '^--- FAIL\|^FAIL' <log>`.
- If exit is 0: go to Story 1.2.1. If non-zero: Task 1.3a.
- Files: none (log in `~/.cache/tmp/ssq-ac9/`).

#### Story 1.1.2: Hermeticity fixes are not vacuous
**As a** maintainer, **I want** the `session/vc` and `session/tymux` fixes shown to work under repetition and under both temp-dir environments, **so that** one lucky green run is not mistaken for a fix.
**Acceptance Criteria**:
- tymux concurrent-poll test is stable and not vacuous.
  - *Given* `TestTymuxGRPCSession_ActivityConcurrentPolls` (8 pollers, one shared `PaneSnapshot`), *When* `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m` runs, *Then* it exits 0 and each iteration enforces `assert.EqualValues(t, 1, updates.Load())` (VERIFIED present at `session/tymux/session_test.go:~726`, so the moved `HasMeaningfulContent` assertion cannot pass with zero updates).
- vc tests pass under both a contaminated and a clean temp dir.
  - *Given* `session/vc/main_test.go` `TestMain`, *When* `go test ./session/vc -count=1` runs once with `TMPDIR=/home/tstapler/.cache/tmp` and once with `TMPDIR=/tmp`, *Then* both exit 0.
**Files**: none changed (read `session/tymux/session_test.go`, `session/vc/main_test.go`).

##### Task 1.2a: tymux repetition run (XS)
- `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m -v 2>&1 | tail -n 15`; record exit code.
- Re-read the test body once to confirm the `updates.Load()` assertion is still after `wg.Wait()`.
- Files: none.

##### Task 1.2b: vc under both environments (XS)
- `TMPDIR=/home/tstapler/.cache/tmp go test ./session/vc -count=1`; then `TMPDIR=/tmp go test ./session/vc -count=1`. Record both exits.
- Check pitfalls Gap 1 cheaply: `ls -d /.git /.jj /tmp/.git /tmp/.jj 2>&1` (expect all "No such file").
- Files: none.

#### Story 1.1.3: Defect handling (conditional)
**As a** maintainer, **I want** any red result triaged against `main` before a code change, **so that** unrelated known flakes (BUG-087..090) do not become scope creep and real regressions get a root-cause fix.
**Acceptance Criteria**:
- A failing test is classified before anything is edited.
  - *Given* a `make test` run where `session/mux` `TestFoo` fails (example), *When* triage runs (`go test <pkg> -run TestFoo -count=3 -race`, and `gh run list --repo tstapler/stapler-squad --branch main --limit 5`), *Then* the note records: failing in isolation yes/no, failing on `main` CI yes/no, and a root-cause sentence of the form "fails because X". A fix is made only for failures that reproduce in isolation or touch `b6820123c` files.
- If no failure occurred, this story is closed with "not triggered".
**Files**: only if triggered: the failing test's file, plus `session/vc/main_test.go` for Task 1.3b.

##### Task 1.3a: Triage a red run (S, conditional)
- For each failing test: rerun in isolation, check `main` CI, state root cause, label VERIFIED/INFERRED. Per memory (`fix-flaky-tests-dont-defer`), fix flakes found rather than re-deferring; ship as its own commit.
- Files: failing test file (variable).

##### Task 1.3b: Harden `session/vc` TestMain (XS, only if 1.2b or 1.1b shows the `/tmp` relocation itself contaminated)
- In `session/vc/main_test.go`, after `os.MkdirTemp("/tmp", ...)`, re-run `isJujutsuRepo(dir) || isGitRepo(dir)` and `os.Exit(1)` with a message if true. Then `go test ./session/vc -count=1` under both TMPDIRs.
- Files: `session/vc/main_test.go`.

### Epic 1.2: AC11, no real worktreeconfig errors after deploy
**Goal**: Show the binary serving traffic since the restart instant embeds `ssq.3`, that the log filter can detect the old failure (negative control), that the guarded path ran successfully on a worktreeConfig repo post-restart (positive control), and that the window contains zero real errors.

#### Story 1.2.1: Binary and window provenance
**As a** maintainer, **I want** proof the running process is built from the fix, **so that** a clean log is attributable to the fix and not to an old binary.
**Acceptance Criteria**:
- Deployed binary carries `ssq.3`.
  - *Given* `deployed binary` at `/home/tstapler/Programming/stapler-squad/stapler-squad`, *When* `go version -m <path> | grep go-git`, *Then* output includes `github.com/tstapler/go-git/v5 v5.19.3-ssq.3` (VERIFIED at planning time).
- Binary started after it was built and after the fix commit.
  - *Given* the service `ActiveEnterTimestamp`, *When* compared with `stat -c %y <path>` and `git log -1 --format=%cI afbc063dd`, *Then* build mtime (12:27:36) is later than the commit time (`2026-11-04T12:26:40-08:00`) and earlier than the restart instant (12:27:48).
- The pin cannot silently regress.
  - *Given* `go.mod` at HEAD, *When* `grep -n 'tstapler/go-git/v5' go.mod`, *Then* line 232 shows `v5.19.3-ssq.3` (not `ssq.1` / `ssq.2` / upstream), since a regression reintroduces the error everywhere except the two ResolveRef-ported functions.
**Files**: none changed.

##### Task 2.1a: Binary provenance (XS)
- `go version -m /home/tstapler/Programming/stapler-squad/stapler-squad | grep -E 'go-git|vcs.revision|vcs.time'`; `stat -c '%y' /home/tstapler/Programming/stapler-squad/stapler-squad`.
- Files: none.

##### Task 2.1b: Restart instant and live log path (XS)
- `systemctl --user show stapler-squad -p ActiveEnterTimestamp -p ExecStart`; confirm the live log by `ls -la ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log` (mtime within the last minutes) and its first record's `time`.
- If the service was restarted again since 12:27:48, use the newest restart instant and re-run 2.1a on the new binary.
- Files: none.

##### Task 2.1c: Pin check (XS)
- `grep -n 'tstapler/go-git/v5' go.mod` and `git ls-remote --tags https://github.com/tstapler/go-git v5.19.3-ssq.3` (expect `a328acc1...`).
- Files: none.

#### Story 1.2.2: Controlled log search
**As a** maintainer, **I want** a filter that ignores prompt echoes and matches only real errors, validated against known-bad history, **so that** zero means zero.
**Acceptance Criteria**:
- Filter detects the old failure (negative control).
  - *Given* rotated log `staplersquad-2026-11-04T13-51-53.139.log.gz` (pre-fix records), *When* `zcat <file> | jq -Rc 'fromjson? | select(((.err // .error // "")|tostring)|test("does not support extension|unknown extension";"i")) | .time' | sed -n '1p;$p'`, *Then* it prints a first/last pair (VERIFIED: `2026-11-04T10:34:39` and `12:11:05`), proving the filter can match.
- Filter ignores prompt echoes.
  - *Given* the post-restart `creating session` / `tmux session created successfully` records whose `program` field embeds the backlog text (VERIFIED: 3+3 in the current file), *When* the same filter runs on `staplersquad.log` with `.time >= "2026-11-04T12:27:48"`, *Then* it returns 0 lines.
- Window has no real errors, in any extension.
  - *Given* the live log plus rotated files overlapping the window, *When* the filter runs over `zcat staplersquad-2026-11-04T13-51-53.139.log.gz` and `staplersquad.log` restricted to `.time >= restart instant`, *Then* 0 records have a matching `err`/`error`; any hit for a `foreign extension` is reported separately as a different bug.
- Window is long enough to matter.
  - *Given* pre-fix rate of the same error (VERIFIED: 9802 `failed to check git status` hits in `staplersquad-2026-11-02T12-52-06.943.log.gz`), *When* the window length is computed (`restart instant` to last log record), *Then* the note states the window length and that at the pre-fix rate a regression would have produced many hits.
**Files**: none changed.

##### Task 2.2a: Negative control (XS)
- Run the `zcat ... | jq` command above on `staplersquad-2026-11-04T13-51-53.139.log.gz`; also `zcat ... | jq -Rc 'fromjson? | select(tostring|test("does not support extension";"i")) | to_entries | map(select(.value|tostring|test("does not support extension";"i"))|.key)|join(",")' | sort | uniq -c` to confirm which keys hold the phrase (VERIFIED: `err`, `error`, `program`).
- Files: none.

##### Task 2.2b: Window search (XS)
- Current file: `jq -Rc 'fromjson? | select(.time >= "2026-11-04T12:27:48") | select(((.err // .error // "")|tostring)|test("does not support extension|unknown extension";"i")) | {time,msg,err}' ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log`.
- Same filter over `zcat ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad-2026-11-04T13-51-53.139.log.gz` (and any newer `.gz`). Do NOT rely on the bare word `worktreeconfig` (branch/session names contain it).
- Also record `grep -c` of the raw phrase restricted to msg types `creating session` / `tmux session created successfully` to show the echo noise is explained (VERIFIED: 6).
- Files: none.

#### Story 1.2.3: Positive control on a worktreeConfig repo
**As a** maintainer, **I want** evidence that a session on a worktreeConfig repo started and resolved its base branch under the deployed binary, **so that** the zero count reflects exercised code.
**Acceptance Criteria**:
- A post-restart success record exists for a worktreeConfig repo.
  - *Given* branch `backlog/stapler-squad-fix-worktreeconfig-extension-compat` in a `worktreeConfig repo`, *When* the log is searched for `msg == "set base commit SHA for branch to merge-base"` with `.time >= restart instant`, *Then* at least one record exists (VERIFIED: `2026-11-04T15:25:17`, `sha":"5d3b16ed"`, `with_branch":"main"`), and no `err`-field match for the same branch within the window.
- The repo really has the extension.
  - *Given* the repo common dir (`git rev-parse --git-common-dir`), *When* `git config --get-regexp '^extensions\.|repositoryformatversion'`, *Then* output includes `extensions.worktreeconfig true` and `core.repositoryformatversion 0`.
- Fallback when no qualifying record exists.
  - *Given* no post-restart record from a worktreeConfig repo (e.g. after a later restart), *When* a session is created on this repo through the normal backlog/UI path (respecting the 2-session WIP cap), *Then* a new `set base commit SHA`/`connected to existing worktree` record appears and the window filter still returns 0.
**Files**: none changed.

##### Task 2.3a: Positive control (XS)
- `jq -Rc 'fromjson? | select(.time >= "2026-11-04T12:27:48") | select(.msg|test("set base commit SHA|found existing worktree for branch|connected to existing worktree")) | del(.program)' <live log>` (and the 13-51 `.gz`); `git config --get-regexp 'extensions|repositoryformatversion'` from this worktree.
- Files: none.

##### Task 2.3b: Extension survey for the discriminator note (XS)
- `git --version` (VERIFIED 2.53.0); `git config --get-regexp '^extensions\.'` in the repo; list any extension other than `worktreeconfig`. If `objectformat`/`refstorage`/`relativeworktrees`/others appear, record them as a follow-up backlog item (separate bug) and do not widen this one.
- Files: none.

### Epic 1.3: Close-out
**Goal**: Evidence is durable and the two criteria are marked in the backlog item.

#### Story 1.3.1: Record and report
**As a** reviewer, **I want** the commands and outputs in one note and the backlog ACs updated, **so that** closure is checkable without rerunning anything.
**Acceptance Criteria**:
- Evidence note exists with exact commands and outputs.
  - *Given* Tasks 1.1b, 1.2a/b, 2.1a-c, 2.2a/b, 2.3a/b complete, *When* `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md` is written, *Then* it holds one section per AC with each command, its exit code or count, and a VERIFIED/INFERRED label.
- Criteria reported only when proven.
  - *Given* `make test` exit `0` and window filter count `0` with controls satisfied, *When* `report_progress` runs for item `9c33e913-0aa0-4eeb-b6a0-ee33f15ad8a8` with `criteria_index=8` (AC9) and `criteria_index=10` (AC11, 0-indexed per the `backlog:done-N` skills), *Then* both read back as passing via `get_backlog_item` (mutation read back, not assumed). If a control is unmet, that AC is reported as `fail` with the reason.
**Files**: `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md` (new, justified: durable evidence trail requested by the AC wording "after deploying").

##### Task 3.1a: Write the evidence note (S)
- Assemble outputs from Epic 1.1 and 1.2 into `evidence.md`; re-read adversarially once (every number recounted from a command, every path exists).
- Files: `project_plans/stapler-squad-worktree-default-branch/implementation/evidence.md`.

##### Task 3.1b: Report to backlog (XS)
- Call `report_progress` for AC9 (index 8) and AC11 (index 10) with the evidence summary; then `get_backlog_item` to read back status. Commit the plan artifacts and `evidence.md` (stage only these files; no `git add -A`; `git status` first: `research/` is currently untracked).
- Files: `project_plans/stapler-squad-worktree-default-branch/` (add).

---

## Effort estimate

Weights per `ESTIMATION.md`: CU = in + 1.25·cache_write + 0.1·cache_read + 5·out. Assumed output share ~5-10% (read/run-heavy, little code). Costs are dominated by waiting on `make test`, not tokens.

| Story | Tasks | Raw tokens (est.) | Assumed output share | CU (est.) |
|---|---|---|---|---|
| 1.1.1 full-suite run | 1.1a XS, 1.1b S | ~60k | 5% (log tails only) | ~75k |
| 1.1.2 hermeticity | 1.2a XS, 1.2b XS | ~40k | 5% | ~50k |
| 1.1.3 defect handling (conditional) | 1.3a S, 1.3b XS | ~0 / up to ~100k | 10% | 0 / ~150k |
| 1.2.1 provenance | 2.1a-c XS | ~40k | 5% | ~50k |
| 1.2.2 controlled search | 2.2a-b XS | ~50k | 5% | ~60k |
| 1.2.3 positive control | 2.3a-b XS | ~40k | 5% | ~50k |
| 1.3.1 close-out | 3.1a S, 3.1b XS | ~70k | 15% (writes the note) | ~100k |
| **Subtotal** | 17 tasks (2 conditional) | ~300k (+100k if triggered) | | ~385k (+150k) |
| Verification multiplier | x1.5 for review of evidence note | | | ~580k |

- **Size band**: Small (<= 0.5M) to low Medium if the conditional defect path triggers.
- **Critical path (agent waves)**: 2 waves. Wave 1: Epic 1.1 (one agent: `make test`) in parallel with Epic 1.2 (one agent: log work). Wave 2: Epic 1.3 close-out. A third wave only if Task 1.3a/b triggers.
- **Overrun checkpoint**: if spend exceeds the table value by > 25% for completed stories, pause and report the delta with its cause (informational).
- **Wall-clock blockers**: (1) `make test` duration, two serialized gotestsum phases with 20m timeouts each (owner: none, machine time; avoid running concurrently with other tmux-heavy work or Bazel JVMs on this tmpfs-/tmp host). (2) AC11 needs organic or triggered session traffic on a worktreeConfig repo after the restart instant; already satisfied (15:25:17 record) but the fallback in Story 1.2.3 needs a free backlog WIP slot (owner: the operator).

## Flagged choices
1. Filter AC11 on structured `err`/`error` fields, not the word `worktreeconfig`: the branch and session names contain it (9492 false hits VERIFIED).
2. AC11 requirement text names `~/.stapler-squad/logs/staplersquad.log`; the real log is the workspace-scoped path above. Evidence uses the real path and states the discrepancy.
3. One `make test` run under the contaminated `TMPDIR` (the harness default here) instead of two full runs; the clean-`/tmp` comparison is done at package level (Task 1.2b) to avoid doubling a long, tmux-heavy run.
4. No redeploy: the service already runs an `ssq.3` binary started after the fix.
5. No code change planned. The tymux assertion is already non-vacuous (`EqualValues(1, updates.Load())`); the vc re-check hardening is conditional.

## Pre-mortem P1 amendments (phase 4)

Binding on Task 1.1a, Story 1.2.3 and Task 3.1b; they override anything above that conflicts.

- **P1 #1 (AC11 zero count proves nothing).** Required positive controls before reporting AC11: (a) `go test ./session -run TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension -count=1 -v` exits 0 and the test is not skipped (`session/repo_path_test.go:598`); (b) `go test ./session/git -run 'Succeeds_When_RepoHasWorktreeConfigExtension' -count=1 -v` exits 0; (c) a real new-worktree session start on a worktreeConfig repo, only if under the 2-session WIP cap, else (a)+(b) suffice. The `15:25:17` "set base commit SHA" record is WEAK (it comes from `initBaseCommitSHA`/`MergeBase`, not the default-branch resolve path). Task 3.1b calls `report_progress` for AC11 only after (a)+(b) pass and the err-field jq filter returns 0 real hits since `ActiveEnterTimestamp`.
- **P1 #2 (`make test` on the live-service host).** Task 1.1a pre-checks: `list_backlog_items` in_progress count within the WIP cap, `uptime`, `free -g`; scratch on disk (not tmpfs `/tmp`); export a unique `STAPLER_SQUAD_TMUX_SOCKET` for the run; capture `systemctl --user show stapler-squad -p ActiveEnterTimestamp` before and after and abort and re-baseline AC11's window if it changed. Already observed once: `make test` ran at EXIT=0 and `ActiveEnterTimestamp` stayed `2026-11-04 12:27:48 PST`.
