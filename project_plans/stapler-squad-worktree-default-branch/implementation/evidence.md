# Evidence: worktreeconfig default-branch item (9c33e913)

Recorded 2026-11-04 from HEAD of `backlog/stapler-squad-fix-worktreeconfig-extension-compat`. No source code was changed; this note is the only new deliverable. Labels: VERIFIED = command run, output seen.

## Root cause (VERIFIED)

`git config --get-regexp '^extensions\.|repositoryformatversion'` in the repo: `core.repositoryformatversion 0`, `extensions.worktreeconfig true` (from `.git/config`). Fixed by PR #1430 (MERGED 2026-11-04T20:26:40Z, merge commit `afbc063dd`, ancestor of HEAD).

## AC 9: `make test` exits 0

Run from this worktree with `TMPDIR=/home/tstapler/.cache/tmp` (under `~/.jj`, the contaminated case), `make build` then `make test`, logs in `~/.cache/tmp/ssq-ac9/`.

| Run | Result |
|---|---|
| 1, with `STAPLER_SQUAD_TMUX_SOCKET=ssq-verify-1` exported (plan's T-AC9-1) | `exit=2`, 10 failures: `session/tmux` TestResolveSocket, TestPrependSocket (+subtests), `session/mux` and `session` `*UsesIsolatedSocket` tests |
| 2, env var not set | `build_exit=0`, `exit=0`, `grep -c '^--- FAIL\|^FAIL\|^=== FAIL'` over the full log = 0, `DONE 5433 tests, 59 skipped in 50.692s` and `DONE 14638 tests, 39 skipped in 154.836s` |

Root cause of run 1 (VERIFIED): `STAPLER_SQUAD_TMUX_SOCKET=ssq-verify-1 go test ./session/tmux -run 'TestPrependSocket|TestResolveSocket' -count=1` fails; the same command with the variable unset passes. Those tests assert the unset-env test-mode default. Test mode already isolates the tmux socket, so the export was unnecessary; it was a defect in the plan (T-AC9-1, now corrected), not in the code. Run 1's logs are kept under `~/.cache/tmp/ssq-ac9/run1-with-env/`.

Live service untouched (VERIFIED, `systemctl --user show stapler-squad`): before and after both `ActiveEnterTimestamp=Wed 2026-11-04 12:27:48 PST`, `MainPID=2175107`. Monitor log: 0 ABORT, peak load1 about 3 on 24 cores, 42 GiB available.

Hermeticity checks (VERIFIED), run sequentially after `make test`:
- `go test ./session/tymux -run TestTymuxGRPCSession_ActivityConcurrentPolls -race -count=20 -timeout=10m -v`: exit 0, 20 `--- PASS` lines; `session/tymux/session_test.go:722` still has `assert.EqualValues(t, 1, updates.Load(), ...)`, so the test cannot pass with zero updates.
- `TMPDIR=/home/tstapler/.cache/tmp go test ./session/vc -count=1`: ok. `TMPDIR=/tmp go test ./session/vc -count=1`: ok. `ls -d /.git /.jj /tmp/.git /tmp/.jj`: all "No such file".

An earlier accidental `make build`/`make test` ran in `~` because of a script error (empty `cd` path). `~/Makefile` has no `build` or `test` target, so make errored with `No rule to make target`. `~` is not a git repository, so `git status`/`git diff` cannot confirm it; `~/Makefile` mtime is 2025-11-25. INFERRED that nothing changed; not provable by git.

## AC 11: no real worktreeconfig errors after deploy

Interpretation: the requirement names `~/.stapler-squad/logs/staplersquad.log`, which is stale on this host. The live log is `~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log`. Only real errors (the `err`/`error` JSON field) count; the bare word `worktreeconfig` appears in branch and session names.

- Binary (VERIFIED): `/home/tstapler/Programming/stapler-squad/stapler-squad` embeds `github.com/tstapler/go-git/v5 v5.19.3-ssq.3`, `vcs.revision=afbc063dd`, `vcs.modified=true` (caveat). `git merge-base --is-ancestor afbc063dd <vcs.revision>`: yes. Commit time 12:26:40-08:00 < binary mtime 12:27:36 < service start 12:27:48.
- Pin (VERIFIED): `go.mod:232` replaces go-git with `github.com/tstapler/go-git/v5 v5.19.3-ssq.3`; `git ls-remote --tags` resolves it to `a328acc1f747b208f5398abd4458e3b5c2a1b627`.
- Negative control (VERIFIED): the filter over `staplersquad-2026-11-04T13-51-53.139.log.gz` (sha256 `1f07a38e7ff092337453ec7f8c58e95a19fc7262752bf8e2602f5f10ca7adbb1`, recorded in `evidence/SHA256SUMS`) matches real pre-fix errors from `2026-11-04T10:32:03` to `12:24:23`. The phrase sits in the `err`, `error` and `program` fields.
- Window search (VERIFIED): `.time >= 2026-11-04T12:27:48-08:00`, filter `(.err // .error) | test("does not support extension|unknown extension")`. Live log: 33877 records, 0 hits. Rotated 13-51 file: 24758 records, 0 hits. The two files are contiguous (13:51:53.1375 then 13:51:53.1397). Window: 12:27:48 to 15:49:56, 3h22m, 58635 records.
- Echo noise (VERIFIED): the raw phrase appears in 6 live-log lines, all this item's description inside the `program` field (`creating session` x3, `tmux session created successfully` x3); 0 elsewhere.
- Positive control (VERIFIED): `go test ./session -run TestCreateBacklogWorktree_Succeeds_When_RepoHasWorktreeConfigExtension -count=1 -v` shows `--- PASS` (not SKIP). `go test ./session/git -run 'Succeeds_When_RepoHasWorktreeConfigExtension' -count=1 -v` shows `--- PASS` for `TestResolveLocalBranchSHA_...` and `TestResolveOriginBranchSHA_...`. The 15:25:17 `set base commit SHA` record is supplementary only (it comes from `initBaseCommitSHA`/`MergeBase`). No fresh session start was done (optional, deliberate).
- Direct check (VERIFIED, earlier this session): `ResolveLocalBranchSHA` and `ResolveOriginBranchSHA` against the real repo return SHAs with nil error.

Limits: no new session was started on a worktreeConfig repo in the window, so the clean count rests on the named tests plus the filter, not on a live session start.

## Follow-up (not filed)

The fork still rejects `objectformat`, `compatobjectformat`, `refstorage`, `relativeworktrees` and `submodulepathconfig` (allow-list in `repository_extensions.go`). This repo has only `extensions.worktreeconfig` (`git 2.53.0`), so the plan's trigger for filing a follow-up item was not met. A repo using one of those would fail the same way with a different extension name.

## Review round 1 additions (verdict PARTIAL)

Reviewer gaps: fork tests live outside the repo, `make lint` not re-run, AC 11 log check not valid evidence. Addressed:

- **AC 10, `make lint`** (VERIFIED, from HEAD): `make lint` exit 0, ends `custom lint: ok`, `shellcheck: ok`, `0 issues.`
- **AC 1, 2, 3, 6, fork tests** (VERIFIED): the pinned fork source was copied from the module cache (`$(go env GOMODCACHE)/github.com/tstapler/go-git/v5@v5.19.3-ssq.3`) to a scratch dir and run: `go test ./ -run 'TestVerifyExtensions|TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension' -count=1 -v` -> `--- PASS: TestVerifyExtensions` (subtests include `repositoryformatversion=0: allows worktreeConfig` [AC 1], `repositoryformatversion=1: allows worktreeConfig` [AC 2], `repositoryformatversion=0: allows partialClone and preciousObjects` [AC 3], `worktreeConfig does not mask an invalid extension`) and `--- PASS: TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension` [AC 6].
- **AC 8** (VERIFIED): `go test ./session/git/backend/gogit -run TestReadEnrichment_RejectsIncludesAndUnsupported -count=1 -v` result recorded in the review request.
- **AC 11 is not checkable at the path the AC names.** `~/.stapler-squad/logs/staplersquad.log` was last written Sep 5, so a `grep -c` there returns 0 no matter which binary runs; that count is vacuous. The service that handles session creation logs to `~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log`. Reproduce the real check:

```
D=~/.stapler-squad/workspaces/d685c4b1a423cca3/logs; R=2026-11-04T12:27:48-08:00
F='fromjson? | select(.time >= $r) | select(((.err // .error // "")|tostring)|test("does not support extension|unknown extension";"i")) | {time,msg,err}'
jq -Rc --arg r "$R" "$F" $D/staplersquad.log | wc -l                                   # 0
zcat $D/staplersquad-2026-11-04T13-51-53.139.log.gz | jq -Rc --arg r "$R" "$F" | wc -l   # 0
zcat $D/staplersquad-2026-11-04T13-51-53.139.log.gz | jq -Rr 'fromjson? | select(((.err // .error // "")|tostring)|test("does not support extension";"i")) | .time' | sed -n '1p;$p'   # negative control: 10:32:03 ... 12:24:23
```

## Verbatim log tails (unedited)

Both commands ran to completion in this session. Command lines and exit codes are quoted from the shell; log lines are unedited `tail` output (no filtering, no truncation).

### make test (run 2): `bash -c 'make test > ~/.cache/tmp/ssq-ac9/make-test.log 2>&1; echo "exit=$?" >> ...'`, TMPDIR=/home/tstapler/.cache/tmp, STAPLER_SQUAD_TMUX_SOCKET unset

```

=== SKIP: session/unfinished/gogitstore TestMmapIndex_MatchesCopyBasedLoader_ByteForByte (0.00s)
    mmapindex_test.go:52: skipped under -short: see BenchmarkMmapIndex_LoadVsCopyBased for on-demand coverage of this path

=== SKIP: session/unfinished/gogitstore TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash (0.00s)
    mmapindex_test.go:334: skipped under -short: crash-detection subprocess can't share the parent's fixture cache, so this always pays a full from-scratch build

=== SKIP: session/unfinished/gogitstore TestRegistry_ConcurrentOpenClose_NeverEvictsInUseStore (0.00s)
    registry_eviction_test.go:360: skipped under -short: too slow for make test/quick-check

=== SKIP: tests/demo TestRecordDemo (0.00s)
    demo_test.go:25: Set RECORD_DEMO=1 to record demo video

DONE 14638 tests, 39 skipped in 154.836s
exit=0
```
The `exit=0` line above was appended by the run script from `$?` after `make test` finished. Full-log failure count: `grep -c '^--- FAIL\|^FAIL\|^=== FAIL'` = 0.

### make lint: `make lint > ~/.cache/tmp/ssq-ac9/make-lint.log 2>&1; echo "make lint EXIT=$?"` printed `make lint EXIT=0` (shell output; the log itself has no exit line)

```
✅ Proto files unchanged, skipping generation
✅ ent schema unchanged, skipping generation
Building web UI with Bun (WEB_MINIFY=0)...
web-build: ok (unminified, 2839 ms) -> out
Copying built files to server/web/dist...
✅ Web UI built and copied successfully
Running custom lint...
ok  	github.com/tstapler/stapler-squad/tools/lint/norawgitcli	0.788s
custom lint: ok
Running shellcheck on 67 first-party shell script(s)...
shellcheck: ok
0 issues.
```

Run 1 of `make test` (with the socket variable exported) failed with exit=2 and 10 failures; its log is at `~/.cache/tmp/ssq-ac9/run1-with-env/make-test.log` on this host only.
