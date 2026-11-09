# Adversarial Review: stapler-squad-worktree-default-branch

**Date**: 2026-11-04
**Verdict**: CONCERNS

Independent checks run while reviewing (live log `/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/logs/`):

- Window filter (`.time >= "2026-11-04T12:27:48"`, `err`/`error` match on `does not support extension|unknown extension`) over `staplersquad.log` returned 0 records. The same filter over `staplersquad-2026-11-04T13-51-53.139.log.gz` for the window also returned 0. A whole-record scan (minus `program`) for the phrase returned 0. Service `ActiveEnterTimestamp` is `12:27:48 PST`. The .gz covers 12:27:48 to 13:51:53 and the live file starts 13:51:53, so there is no gap. VERIFIED.
- The rotated log exists and the filter matches pre-fix records, so the negative control works. BUT the first/last times are `10:32:03` and `12:24:23`, not the plan's `10:34:39` / `12:11:05` (see Concern 2). VERIFIED.
- `session/tymux/session_test.go:722` still has `assert.EqualValues(t, 1, updates.Load(), ...)` after `wg.Wait()`. The plan says `~726`. VERIFIED.
- `git ls-remote` returns `a328acc1...` for `v5.19.3-ssq.3`, and `go.mod:232` pins it. VERIFIED.
- The 15:25:17 record is real (`sha":"5d3b16ed"`, `with_branch":"main"`), but it does NOT come from the default-branch resolution path (see Concern 1).

## Blockers
None.

## Concerns
- [ ] **Positive control does not exercise the code that failed.** The "set base commit SHA for branch to merge-base" message is emitted only by `initBaseCommitSHA` (`session/git/worktree_ops.go:271`). That function calls `headMergeBase`, which goes through `gitBackend().MergeBase` (`session/git/worktree_refs.go:40-48`). The original failure ("resolve default branch: failed to open git repo", `session/instance_worktree.go:286` via `git.ResolveWorktreeBaseCommit`) is a different path. The record was also written on an existing-worktree/reuse path for this very session. So it shows a worktreeConfig repo was handled without error, not that `OpenRepo`/`ResolveWorktreeBaseCommit` succeeded on the deployed binary. Post-restart there are only 5 `creating session` records and 2 `connected to existing worktree`, none shown as new-worktree starts on this repo. Recommendation: (a) reword the AC and glossary so the 15:25:17 record is a weak control, not proof of the resolve-default-branch path; (b) add a stronger control, either a new-worktree session start on a worktreeConfig repo with its follow-on success record, or (preferably, deterministic) run `go test ./session -run <the repo_path_test.go worktreeconfig test> -count=1` (`session/repo_path_test.go:595` covers the resolve-default-branch -> create-worktree path) and record it; (c) keep the Story 1.2.3 fallback as the required path whenever (b) is not available.
- [ ] **Hardcoded negative-control values are wrong.** The Story 1.2.2 AC says the filter prints first/last `10:34:39` / `12:11:05`. Running the filter as written gives `10:32:03.065...` / `12:24:23.140...`. An implementer who diffs against the AC will see a spurious failure, or worse, trust numbers nobody reproduced. Recommendation: replace with the observed values or state "non-empty, last hit before 12:27:48". Also note the last pre-fix hit was only about 3 minutes before the restart, which is good evidence the fix took effect at restart but should be stated as such.
- [ ] **`make test` runs tmux-heavy tests on the host that runs the live service and in-flight backlog sessions.** Plan Task 1.1b only mentions load/tmpfs. Project notes say private tmux sockets are not automatic and the leaked-control-mode sweeper can kill live control clients. Recommendation: confirm the test packages use `TMUX_BIN`/private sockets (or set `STAPLER_SQUAD_TMUX_SOCKET`), check `uptime`/WIP count first, and say what to do if a live session drops.
- [ ] **AC11 text vs window.** Requirement says "after deploying a binary built from the fix" and names the stale `~/.stapler-squad/logs/` path. The plan handles the path (Flagged choice 2) but should say the AC wording in the backlog item is being reinterpreted, and record the owner-visible reason. Otherwise `report_progress` will claim a different criterion than the one written.

## Minors
- Task 1.1b / Story 1.1.1: `tail -n 200` can cut off `FAIL` lines; the AC should rely on the full-log `grep -c`, not the tail.
- The plan cites `session_test.go:~726`; actual line is 722.
- Task 1.1a runs `make build` first, which also runs lint; an unrelated lint failure would block AC9 evidence. Run `make test` independently if `make build` fails for a non-test reason.
- Task IDs (1.1a, 2.1a, 3.1a) do not match story numbers (1.1.x, 1.2.x, 1.3.x); the note explains it but it remains confusing.
- Evidence trail relies on `evidence.md` being committed; `research/` is currently untracked, so the commit step must add it deliberately.
- Plan is verification-only with conditional code changes; no scope drift found.
