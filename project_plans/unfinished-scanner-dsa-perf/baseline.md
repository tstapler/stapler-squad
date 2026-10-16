# Baseline: unfinished-scanner-dsa-perf

Measurements that later stories compare against. Appended to by Stories 0.0.3 and 0.1.1.

## Revisions

- Live baseline (deployed binary): `39fa33a11`, old stack-based `findMergeBase`/`countCommitsTo`.
- Lockstep `findMergeBase` + `parentMemo`: landed in commit `f0a75ac10` (after `39fa33a11`).

## Lockstep-only row (Story 0.0.1)

Cold, `W=1`, 2,500-commit linear go-git history (fork of 3 commits, 5 behind), in-memory storer wrapped by a counting storer; `findMergeBase` + two `countCommitsTo` per run. Test: `TestFindMergeBase_ShouldDecodeFewerThanLegacyStack_WhenCold2500Linear` (`session/unfinished/mergebase_decodecount_test.go`).

| Walk | DecodeCount | allocs/op |
|---|---|---|
| old committed walk (`legacyStack*`) | 2013 | 50,354 |
| lockstep | 11 | 305 |

This gain belongs to the lockstep commit, not to this project; the 24 h profile (decode = 51.9% of `alloc_space`) was taken on the old walk.

## Differential: old committed walk vs lockstep (Story 0.0.1)

- Identical on 600 seeded DAGs (300 linear forks, 300 merge-free trees): enforced by test.
- Divergence exists: yes. Golden file `session/unfinished/testdata/mergebase_lockstep_divergences.golden` (1213 cases, 134 diverge): single-merge 5 of 400 pairs; general merge-rich DAGs 88 of 400 (caps 2000 and 4); shallow-truncated variants 41 of 400; named criss-cross, octopus, shallow and cap-linear fixtures 0. Details in ADR-003.
- Vs `object.Commit.MergeBase` best set (synthetic, default cap): single-merge legacy 393/400, lockstep 392/400; general legacy 157/200, lockstep 148/200.

## Real-worktree differential (reporting only)

Opt-in: `MERGEBASE_REAL_REPOS=1 go test ./session/unfinished/ -run RealWorktree -v`. Current repo plus `~/code/github.com/tstapler/*` (excluding `stapler-squad-*` sibling clones), 6 refs per repo, read-only via go-git. Run on the machine at commit time: 13 repos, 55 pairs; 44 identical, 8 divergent or error-mismatched (3 pairs had an error on at least one side); vs `object.Commit.MergeBase` over 52 pairs: legacy-in-best 48, lockstep-in-best 46. Not repeatable bit-for-bit (refs change).

## Native-git inventory under `session/unfinished/**` (Task 0.0.3a, re-derived by grep)

`grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'` listed 15 files, all `_test.go`, matching the plan: top-level `gogit_vcs_reader_gitignore_test.go`, `gogit_vcs_reader_limits_test.go`, `gogit_vcs_reader_shellout_bench_test.go` (deleted by this task), `state_test.go`, `vcsreader_bench_test.go`, `vcsreader_test.go`; `gogitstore/` `crash_subprocess_test.go` (re-execs `os.Args[0]`, the one allow-listed file), `gogitstore_test.go`, `mmap_adversarial_test.go`, `mmap_realrepo_test.go`, `mmap_stage2_test.go`, `mmap_truncation_test.go`, `mmapindex_test.go`, `registry_eviction_test.go`, `soak_test.go`. `repostate/` does not exist yet.

`gitRunInternal` counts (`grep -rc`, non-zero only): `gogit_vcs_reader_limits_test.go` 20, `gogit_vcs_reader_objectcache_test.go` 1 (a call, no own import), `gogitstore/gogitstore_test.go` 1 (a comment reference at `:33`, not a call).

Custom linters and `_test.go`: `norawgitcli` skips `_test.go` explicitly (`tools/lint/norawgitcli/analyzer.go:93`); `norawexec` only flags raw `os/exec.Command*` calls (not `safeexec`), so every `safeexec.CommandContext(..., "git", ...)` test site passes it; `nocommandpattern` checks `Rule.CommandPattern` literals and is unrelated to git. None enforces the go-git-only policy for tests, so `TestNoNativeGitInPackage` (`session/unfinished/no_native_git_test.go`) is the enforcement.

`TestNoNativeGitInPackage` is red at this commit by design (14 offending files listed in its output) and turns green at Task 0.0.3f. `BenchmarkDiffShortstat_GoGitVsNative` is gone; `go test -list BenchmarkDiffShortstat ./session/unfinished` now lists exactly `BenchmarkDiffShortstat` and `BenchmarkDiffShortstatCached`.

## Story 0.1.5 workload corpus

Measured 2026-10-15 with `go run ./tools/scanner-workload cadence -days 90` (read-only go-git walk of `HEAD`; commits/day = commits with committer time in the last 90 days / 90). Sizes are `.git` plus working tree, minus `.git/worktrees`, `.git/lfs`, `.claude/worktrees` and `node_modules`.

| Repo | Total commits | Commits (90 d) | Commits/day | Tracked files | Size (MB) | In default corpus |
|---|---|---|---|---|---|---|
| stapler-squad | 4356 | 1543 | 17.14 | 8832 | 19455 | no: above `-max-repo-mb 4096` |
| consolette | 229 | 229 | 2.54 | 604 | 23617 | no: above guard |
| docspan | 125 | 101 | 1.12 | 271 | 279 | yes |
| gh-signal | 102 | 102 | 1.13 | 266 | 54590 | no: above guard |
| go-git | 2566 | 37 | 0.41 | 506 | 15 | yes |
| kibitzer | 135 | 135 | 1.50 | 216 | 1965 | yes |
| magic-compact | 45 | 10 | 0.11 | 61 | 0 | yes |
| pi-extensions | 2 | 2 | 0.02 | 17 | 0 | yes |
| rtk | 1450 | 98 | 1.09 | 410 | 333 | yes |
| stapler-mcp | 8 | 5 | 0.06 | 95 | 1457 | no: `-max-repos 6` cap reached |

- Default build (`corpus -config-dir <dir>`): 6 repos copied, W=3 linked worktrees each (18 linked + 6 main = 24 scan targets), ~27 s wall. A second instance built the same way pinned an identical list (`verify`, paths compared relative to each corpus root) and the op-log hash for seed 1 / 30 min logical was equal on both (`8e5512ed...909c`, 65 ops, 4 of them logged `skip:` on first-delete-before-create).
- **stapler-squad is not in the default corpus**: its `.git` alone is ~15 GB of packs (plus an 11 GB `.git/lfs` cache, skipped), so a byte copy per instance is ~19 GB each. Pass `-max-repo-mb 20000` (and `-max-repos`) to include it; the owner decides whether 2 x 19 GB is acceptable. Its stale commit-graph is carried by the byte copy.
- Corpus copies of sibling instances are byte-identical except per-instance linked-worktree admin files (`.git/worktrees/<name>/{gitdir,index}` embed instance paths and mtimes).
- The driver's cadence input is `commits_per_day` above (stapler-squad 17.1/day is the ceiling; a 30 min run at speedup 1 yields about 0 to 1 commit events per repo). The commit-walk bucket is not tuned to any profile.
