# S0 audit: Stories 0.1.1 and 0.1.3

Run 2026-10-08 at HEAD `a7f0809ef` on branch `chore/sdd-go-git-fork-planning` (plan cites `test/go-git-fixtures` @ `85bd89b5c`; counts below are from the current checkout). Scripts: `spikes/audit/git-cli-sites.sh`, `spikes/audit/vc-vcs-callers.sh`. All numbers are VERIFIED by running those scripts. No product code changed.

## Story 0.1.1 call-site counts (claimed vs actual)

| Metric | Claimed | Actual | Match |
|---|---|---|---|
| Product `"git",` sites | 62 | 62 | yes |
| Product `"git",` files | 29 | 29 | yes |
| Test `"git",` sites | 223 | 223 | yes |
| Test `"git",` sites under `session/` | 121 | 121 | yes |
| Product exec/safeexec constructor sites | 36 | 36 | yes |
| ... files | 19 | 19 | yes |
| Non-test go-git importers | 30 | 30 | yes |
| Product runner sites (`.Run(..."git"`, `commandRunner().Run`, `runGitCommand(`) | 25 | 25 | yes |
| Test direct `exec.Command("git"` sites | 10 | **11** with the script's regex; 10 true `exec.Command` | see below |
| `LookPath("git")` total | 30 | 30 | yes |
| `LookPath("git")` test sites / files | 29 / 10 | 29 / 10 | yes |
| `Setenv("PATH"` test sites / files | 20 / 12 | 20 / 12 | yes |

**Discrepancy (only one).** The regex `exec\.Command(Context)?\([^)]*"git"` also matches `safeexec.CommandContext(ctx, "git", ...)` at `session/unfinished/gogit_vcs_reader_shellout_bench_test.go:38`, giving 11. The 10 claimed are the true `exec.Command("git"` calls: `server/services/backlog_service_test.go` (3: 2001, 4138, 4143), `backlog_service_triage_test.go` (6: 2676, 2694, 3104, 3106, 3108, 3161), `session/backlog_lifecycle_test.go` (1: 4402). Total 10 in 3 files, as the plan says; the 11th is a benchmark that shells out via safeexec on purpose. The plan's "3 files" wording holds only for true `exec.Command`. Recommend the plan's gate regex be `(^|[^a-z])exec\.Command` to exclude the bench, or list the bench explicitly.

Product subcommand histogram (first arg after `"git",`): rev-parse 12, push 4, worktree 3, fetch 3, status 2, clone 2, symbolic-ref 1, rev-list 1, merge-base 1, for-each-ref 1, diff 1, config 1. Noise entries from the loose grep: `"reason"` 2, `"npm"` 1, `"hg"` 1 (not git subcommands; counted in the 62). So the 62 includes about 4 non-spawn matches; the plan's number is a grep count, not a spawn count.

Story 0.1.1b items: `session/git/util.go:36` `EnableDotGitCommonDir: true` is taken from the plan, not re-read here (out of this task's scope). `GitClient` subcommands are enumerated in the 0.1.3 section below.

## Task 0.1.1c: destructive-sites (Intent=Destructive candidates)

Method: grep for `IsDirty`, `IsDirtyUncached`, `HasUncommitted`, `GetWorktreeDirtyPaths`, `HasCommitsAheadOfMain`, `GetGitDiff*`, then read each caller. Line numbers at HEAD `a7f0809ef`. For the raw-field/answer to irreversible decision mapping, "answer" is what the git-status/diff call returns.

| # | Site (file:line) | Answer consumed | Irreversible decision it feeds | Failure mode if answer is wrong |
|---|---|---|---|---|
| D1 | `server/services/session_retention_sweeper.go:150` (`wt.IsDirty()` in `baseSafeToDelete`) | dirty / clean / error | Sweeper calls `DeleteSession` (`:103-111`), which tears down the session and worktree. Only dirty or error returns "not safe" | A false "clean" deletes uncommitted work. Errors fail closed (`:152`). |
| D2 | `session/instance.go:2278` (`pauseLocked`, `i.gitManager.IsDirty()`), then `:2300-2303` `Remove()` + `Prune()` | dirty / clean / error | If dirty: auto-commit (`CommitChanges`), then **removes the worktree directory**. | False "clean" skips the commit then deletes the worktree: data loss. **IsDirty error does not stop removal**: it is appended to `errs` and execution continues to `Remove()` at `:2303`. Only a commit failure returns early (`:2285`). |
| D3 | `session/instance.go:2467` (stop path, same shape), then `:2485` `Remove()` + `Prune()` | dirty / clean / error | Same as D2 for stop. | Same as D2; error on the dirty check does not block `Remove()`. |
| D4 | `session/git/worktree_git.go:175` (`PushChanges`) and `:222` (`CommitChanges`): `IsDirty()` then `stageAndCommit` | dirty / clean | Skips the commit when "clean". Called by D2/D3 pre-removal, so a false "clean" here means unsaved work is never committed before D2/D3 removal. | Same data-loss path as D2/D3. |
| D5 | `session/git/worktree_git.go:254-262,362-372` (`stageAndCommit`, `HasStagedChanges`) "nothing to commit" gate | staged-vs-HEAD diff present or not | Skips the commit when index equals HEAD. | False "nothing staged" drops the final commit before D2/D3/D8 removal. |
| D6 | `server/mcp/tools_backlog.go:1373` (`session.GetWorktreeDirtyPaths`, via `session/backlog_review.go:683` `vc.GitProvider.GetChangedFiles`) review-gate dirty rejection | list of dirty paths | A non-empty list rejects `request_review`; empty list lets the item proceed into `review` and onward to PR/done. **Fails open on error** (`:1374-1376`). | Wrong "clean" or error lets the reviewer see only the committed diff while real work is uncommitted; later cleanup (D8) can delete it. This is the one site that reads through `session/vc`, not `session/git`. |
| D7 | `session/backlog_lifecycle_pr.go:591` (`HasCommitsAheadOfMain`) | has-commits-ahead bool (fails open to true) | `!hasCommits` calls `fallbackToDone` ("nothing to ship"), marking the item done without a PR (`:592-596`). | False "no commits ahead" marks the item done and later cleanup removes the worktree: unpushed commits become unreachable except through the branch ref (branch is kept, per `Cleanup` doc at `session/git/worktree_ops.go:432-439`). |
| D8 | Worktree removal callers: `server/services/backlog_service.go:1327` (`cleanupItemWorktrees`, `g.Cleanup()` on terminal state), `server/services/backlog_service_trigger_triage.go:117`, `session/instance.go:1798,2066,2380,2410` (start-failure cleanup), `session/instance_worktree.go:537` (`CleanupWorktree`), `server/services/session_service_create.go:563` (`RemoteWorktreeOps.RemoveWorktree`, runs `git worktree remove --force`), `session/git/worktree_ops.go:443-463` (`Remove` -> `nativeRemoveWorktree`) | none directly: these callers do **not** check dirty before removing | The removal itself. | `Cleanup`/`Remove` is unconditional; safety depends entirely on upstream gates D1 to D7 having run. A fork change to `IsDirty` does not protect these sites. Listed so Story 2.1.2 does not assume a dirty check exists here. |
| D9 | `session/vcs/git.go:203` (`HasUncommittedChanges`) feeding `Abandon` -> `AbandonChanges` (`:228`, `:305`); `session/instance_workspace.go:252` (`hasChanges, _ := ...`, error discarded) | dirty / clean | `SwitchTo` with `ChangeStrategy=Abandon` discards uncommitted changes (`session/vcs/git.go:225-229`). User-selected strategy, but the dirty answer decides whether the discard runs and what `ChangesHandled` reports. | Error on the dirty check is ignored at `instance_workspace.go:252`, only the reported text changes; the discard is driven by the in-`SwitchTo` check (`vcs/git.go:203`). |

**Non-destructive (Display) consumers confirmed, listed so they are not mislabelled:** `session/review_queue_determiner.go:96` (review queue priority), `session/git_worktree_manager.go:224` (`IsDirtyUncached` in change detector, UI refresh), `session/unfinished/scanner.go:722` (`HasUncommitted`, unfinished-work badge), `server/services/pr_creation_service.go:187` (PR preview body; ignores error), `server/services/backlog_service_triage.go:3587` and `session/backlog_lifecycle_pr.go:602` (`GetGitDiff` for LLM prompts / PR text).

**Items the plan's Task 0.1.1c named that were not found:** "`server/services/unfinished_work_service.go`" has no destructive consumer (it only maps scanner results to protos, `:151-179,652`). There is no remaining `IsWorktreeDirty` function (only comments at `session/backlog_review.go:672` and `server/mcp/tools_backlog.go:1367`). Backlog terminal-state cleanup (D8, `backlog_service.go:1327`) has no dirty check of its own.

**Findings for Tyler (G7 review):** (1) D2/D3 proceed to `Remove()` when the dirty check *errors*; if the fork makes `IsDirty` return errors more often (e.g. during fallback), this turns into deletion. Recommend `Intent=Destructive` plus fail-closed in Story 2.2.2c. (2) D6 fails open by design; keep as is but classify Destructive because its "clean" answer unlocks D8. (3) D8 is the real irreversible step and has no gate of its own.

## Story 0.1.3: `session/vc` vs `session/vcs`

Evidence from `spikes/audit/vc-vcs-callers.sh` (go list incl. tests, plus grep):

| | `session/vc` (git_provider.go, jj_provider.go, detector.go, types.go, provider.go) | `session/vcs` (git.go, jj.go, detect.go, vcs.go) |
|---|---|---|
| Interface | `vc.VCSProvider` (status, branch, changed files, stage/unstage/commit/amend/push/pull/fetch, diffs, interactive command) | `vcs.VCS` (status, uncommitted check, current revision/bookmark, `SwitchTo`, create bookmark, `AbandonChanges`, list bookmarks/revisions, create/list worktrees) |
| Concrete git type | `vc.GitProvider` | `vcs.GitClient` (methods: `Type, RepoPath, GetStatus, HasUncommittedChanges, GetCurrentRevision, GetCurrentBookmark, SwitchTo, CreateBookmark, DescribeWIP, AbandonChanges, ListBookmarks, ListRecentRevisions, CreateWorktree, ListWorktrees`; spawns `git branch`/`stash`/`checkout`/`worktree` etc. via `run()` at `git.go:41`) |
| go-list importers | `server/services`, `session`, `session/workspace` | `server/services`, `session` |
| Non-test import sites | `search_service.go:19`, `workspace_service.go:21`, `session/backlog_review.go:20`, `session/workspace/types.go:9`, `session/workspace/vcs_batch.go:8` | `workspace_service.go:22`, `session/instance_workspace.go:12` |
| Tests | yes (git_provider_test 42.8K, jj_provider_test, detector_test, types_test) | **none** (`ls session/vcs` shows no `_test.go`) |
| Exported symbols (approx., grep) | 65 | 48 |
| Product `"git",` spawn sites | combined 9 across both packages (grep over `session/vc session/vcs`, non-test) | |

Both layers are **live**; neither is dead. They are duplicate abstractions over the same two CLIs with disjoint capabilities:
- `vc` owns the read/stage/commit/push/diff surface used by the workspace panel, search enrichment and the review-gate dirty check (D6 above).
- `vcs` owns branch/revision switching, bookmark and worktree operations used only by `Instance.SwitchWorkspace` (`instance_workspace.go:98-155,244-278,433-494`) and the `ChangeStrategy`/`VCSType` enums in `workspace_service.go`.
- Duplicated: VCS type enums (`vc.VCSGit` vs `vcs.VCSTypeGit`), repo detection (`vc.DetectVCS` vs `vcs.Detect`/`GetVCSInfo`/`gitAvailable`'s own `exec.LookPath("git")` at `detect.go:123`, the only product `LookPath("git")`), status models (`vc.VCSStatus` vs `vcs.WorkingCopyStatus`), a git runner (`vc.runGit` `git_provider.go:52` vs `vcs.run` `git.go:41`), and a JJ runner each.
- Both are used in the same file `server/services/workspace_service.go`.

**Recommendation (O-11 stays Tyler's; no code deleted or moved):** **Merge, do not delete.** Keep `session/vc` as the surviving package because it is the larger surface, has tests, and carries the destructive-gate caller (D6). Fold the `vcs` capabilities (switch, bookmarks, worktree list/create, abandon) into it, and route both through the single git operation owner introduced in Epic 2 (`vc.GitProvider` and `vcs.GitClient` become thin callers). Do the merge **after** the Epic 2 git facade exists, so each of the 9 spawn sites is migrated once rather than twice. Consequences for the plan:
- Story 1.1.2b "moves code only from layers marked live": mark **both live**.
- Story 2.1.2 must not say "delete `vcs.GitClient`" as dead code: it is live in `instance_workspace.go` and `workspace_service.go`.
- Cheapest first step if the merge is deferred: leave both and add the 9 sites to the Epic 2 migration list; the main regression risk is `vcs` having no tests, so add characterization tests for `vcs.GitClient.SwitchTo` (including `Abandon`, D9) before touching it.
- Alternative (delete `vcs`, re-implement switching inside `vc`) is a larger rewrite of untested code for no extra spawn reduction; not recommended.
- Confidence: caller lists VERIFIED by `go list`; the claim that the capabilities are disjoint is from reading both interface definitions and `git.go`/`git_provider.go` method lists, not from a full behavioral diff (INFERRED).
