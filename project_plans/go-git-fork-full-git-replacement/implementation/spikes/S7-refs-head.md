# S7: `refs` linked-worktree HEAD failure root cause (Story 0.2.8, gate G8)

Date: 2026-10-08. Environment: macOS arm64, go1.26.6, git 2.50.1 (Apple Git-155), go-git v5.19.2 (same as `go.mod`). Source and raw outputs: `S7-src/` (own module; run from a copy, e.g. `cp -R S7-src /tmp/s7 && cd /tmp/s7`). No product code changed.

## G8 outcome: **B** (not reproducible within budget for the production symptom), with two reproduced findings that tighten the design

The documented production symptom (`util.go:338-347`: go-git `Head()` returns a valid SHA with no object **while the git CLI disagrees**) was **not reproduced** (0 "go-git-only phantom" results). Two adjacent classes were reproduced and root-caused (below). Under outcome B the plan adopts, unchanged: the object-exists predicate (`object_missing`), the permanent allow-listed CLI route for `torn_read`/`object_missing`, the soak ceiling, mandatory shadow for `refs`, and no Epic 6.2 deletion of the `getHeadCommitSHA` mitigation.

## Why the mitigation exists (VERIFIED by `git log`)

- `git log -S'did not correspond to any real object' -- session/git/util.go` yields commit `f4d7d5e1c` "fix(git): shell out to git CLI for HEAD SHA instead of go-git (#151)", 2026-07-12. Its message: go-git `Head()` returned a 40-hex SHA absent from `cat-file -t`, `rev-list --all`, `reflog show --all` and `fsck --unreachable`; it poisoned `Worktree.base_commit_sha` and made a review report "no diff available". The author *inferred* a race with a concurrent `git worktree add`; the message does not record what `git rev-parse HEAD` returned for the same worktree at that moment (gap: so "go-git differs from CLI" was never established, only "SHA has no object").
- The retry + `CommitObject` check shape came in the same PR's follow-up commit. `EnableDotGitCommonDir` was added later (`git log -S EnableDotGitCommonDir`: `f31079de6`, `d04c18469`; the doc comment dates it 2026-09-02), i.e. **after** the production observation of 2026-07-12. So the production symptom was seen with the flag **off**.

## Class 1: `Head()` errors on a linked worktree (reproduced, root-caused)

Command: `cd /tmp/s7 && go run ./determ` (single-threaded; CLI `git worktree add -b wt`, then a CLI commit on `wt`):

```
common false Head err: reference not found
common=true Head=11bf1558... name=refs/heads/wt commitObjErr=<nil>
```

Stress confirmation (`out/cfg1_nocommon_all.txt`): `EnableDotGitCommonDir=false`, 60 s, all racers: 30587 of 30587 reads returned `ErrReferenceNotFound` (100%, not a race). `out/cfg7_...`: same with no racers at all: 27842 of 27842.

**Root cause (fails because X):** without `EnableDotGitCommonDir`, go-git treats `.git/worktrees/<name>/` as the whole gitdir. That directory holds `HEAD` (`ref: refs/heads/wt`) but not `refs/heads/wt` or `packed-refs`, which live in the common dir, so resolution fails. Deterministic, no concurrency. **Already fixed in-repo** by `defaultPlainOpenOptions` (`session/git/util.go:36-39`). The fork is not needed for this; the closed `torn_read` error set must NOT include this case (it would be `ErrReferenceNotFound` for a ref whose file does not exist on a second read, so the rule "loose file or packed entry exists on a second read" already excludes it, correctly). Backend rule: opening a linked worktree without the flag is a bug; the gogit backend must always set it (conformance test: `Open_should_SetCommonDir_When_PathIsLinkedWorktree`).

With the flag ON, 0 `Head()` errors in every run below (class 1 does not recur under concurrency).

## Class 2: valid SHA with no object (reproduced only as a dangling ref, not as a go-git-only phantom)

Harness: `S7-src/stress/main.go`. Writers (CLI): `commit --allow-empty` or `commit-tree`+`update-ref` (`-fast`) on branch `wt` in the linked worktree; `commit` on `main`; `pack-refs --all --prune` loop; alternating `repack -adq` / `gc -q --prune=now`; `worktree add` + `remove --force` + `branch -D` churn. Readers (4 to 6 goroutines): open fresh with `EnableDotGitCommonDir`, `Reference(HEAD,false)`, `Head()`, `CommitObject()`. Each no-object result is cross-checked against `git rev-parse HEAD` in the worktree.

Result (`out/cfg10_fast_all_120s_run{1,2,3}.txt`, `-common=true -fast -dur=120s -readers=6`, all racers):

| run | reads | class 2 hits | CLI `rev-parse HEAD` equal to go-git SHA | CLI differs (go-git-only phantom) |
|---|---|---|---|---|
| 1 | 10337 | 182 | 182 | **0** |
| 2 | 1946 | 428 | 428 | **0** |
| 3 | 8718 | 246 | 246 | **0** |

Each such run ends with `git fsck` reporting `refs/heads/wt: invalid sha1 pointer <sha>` and `HEAD: invalid sha1 pointer`. (Earlier run `out/cfg6_common_fast_all.txt`, before the CLI cross-check was added: 73 hits, same signature.)

**Root cause (fails because X):** the ref file itself is dangling. My `-fast` writer creates an unreachable commit with `commit-tree`, and the racing `git gc --prune=now` deletes it before `update-ref` publishes it (git's documented gc/new-object race, only reachable with `--prune=now`; default `gc.pruneExpire` of 2 weeks protects real workflows). go-git faithfully reports a ref that the CLI reports too. This is **not** a go-git read bug, and it is the case where the predicate works as designed (detects `object_missing`) but **the CLI fallback returns the same SHA**, so routing to the CLI does not repair it. Consequence for the plan: on `object_missing`, the CLI result must also be object-checked (`git cat-file -e <sha>^{commit}`), and if the CLI agrees the SHA is dangling, return a typed `ErrRefDangling` (not a SHA) instead of treating the CLI answer as truth. Today's `getHeadCommitSHAViaCLI` has no such check (`util.go:394`).

Not reproduced: any run where go-git returned a SHA that differed from the CLI's and had no object. All remaining stress runs with realistic writers (`git commit`, no `--prune=now` interleaving with `commit-tree`): 0 class-2 hits:

| config (`-dur`) | command (from `/tmp/s7`) | reads | wt commits | class1 | class2 | class3 |
|---|---|---|---|---|---|---|
| 1. no common dir, all racers, 60 s | `./stressbin -common=false -dur=60s` | 30587 | 23 | 30587 | 0 | 0 |
| 2. common dir, all racers, 60 s | `./stressbin -common=true -dur=60s` | 9605 | 23 | 0 | 0 | 0 |
| 3. common dir, pack-refs only, 60 s | `./stressbin -common=true -dur=60s -gc=false -addrm=false` | 14329 | 25 | 0 | 0 | 0 |
| 4. common dir, all racers, **10 min** | `./stressbin -common=true -dur=600s` | 125892 | 286 | 0 | 0 | 0 |
| 5. fast writer + pack-refs, 60 s | `./stressbin2 -common=true -fast -dur=60s -gc=false -addrm=false` | 11709 | 16 | 0 | 0 | 0 |
| 8, 9. three more 60 s all-racer runs (CLI cross-check on) | `./stressbin3 ...` | 6954 to 7806 | 10 to 20 | 0 | 0 | 0 |

(`stressbin`, `stressbin2`, `stressbin3` are successive builds of `./stress` as flags/cross-checks were added; `go build -o stressbin ./stress`.) Budget spent: 3 x 60 s configurations plus the 10-minute run, plus 8 extra runs (about 17 min of total racing). Caveat: commit rate under this contention is low (about 25/min; the racing CLI processes starve each other on `packed-refs.lock`/gc), so the 10-minute run saw only 286 branch advances; a quieter, higher-rate writer was tried (`-fast`) and is the one that exposes the dangling-ref artifact above.

Also observed (not a class): `CommitObject` failed while the CLI could still `cat-file -t` the object, 155 times in the 10-minute run (about 0.12% of reads), 8 to 50 per 60-120 s run. Cause: go-git's object store opens a pack that a concurrent `repack -ad`/`gc` removes between listing and open. These are **transient and recoverable**: the existing 3 x 20 ms re-open retry handles them and the predicate must retry before declaring `object_missing`. Predicate needs: on `CommitObject` failure, re-open and retry (3 x 20 ms), then confirm via CLI `cat-file -e` before classifying; if CLI sees the object, reason is `torn_read` (transient), not `object_missing`.

## Class 3: stale-but-existing SHA

0 occurrences: 0 in every stress run above (window check: hash read must lie in the confirmed-tip index range of the history observed just before/after the read) and `go run ./longlived` (long-lived `*git.Repository`, 30 CLI commits interleaved with `pack-refs --all [--prune]`): `long-lived repo stale/mismatch reads: 0 of 30`. The one real class-3 case found is the pre-fix one recorded in `util.go:357-361` (HEAD resolved to a stale SHA with the flag off); I did not re-derive it because the flag-off configuration now fails class 1 instead on my fixtures. Not reproducible does not mean absent: the predicate cannot detect class 3, so shadow stays mandatory.

## Predicate and routing to adopt (outcome B)

1. After any `ResolveRef`/`Head` read: retry open+read up to 3 x 20 ms on the closed typed `torn_read` set; then `HasEncodedObject`/`CommitObject`; on miss, retry once, then CLI `cat-file -e <sha>^{commit}`. CLI sees object: `torn_read`, return the CLI-resolved SHA. CLI also lacks it: return `ErrRefDangling` (new typed error, reason `object_missing`), never a SHA.
2. Always open linked worktrees with `EnableDotGitCommonDir` (class 1 root cause).
3. Permanent allow-listed CLI route for `torn_read`/`object_missing` on `ResolveRef`, `CurrentBranch`, `RefExists`, `ListRefs` only. Do not delete `getHeadCommitSHA`'s fallback (Story 6.2.1: G8 recorded no fix at the root of the *production* symptom).
4. Soak ceiling for `(torn_read + object_missing) / refs-cohort calls`: observed steady state in these races is about 0.12% for transient repack misses under deliberately hostile gc/repack churn (155 / 125892); production churn is far lower. Per the plan formula "2x steady state, minimum 0.1%", record **0.25%** as the ceiling (derived from a stress rate, an upper bound, not a production measurement; revisit after the 7-day shadow window gives a real rate).
5. Shadow remains mandatory for `refs` (class 3 undetectable by the predicate).

## Reproduce

```
cp -R S7-src /tmp/s7 && cd /tmp/s7
go run ./determ                       # class 1, deterministic
go build -o stressbin ./stress
./stressbin -common=true -dur=60s     # realistic writers
./stressbin -common=true -fast -dur=120s -readers=6   # dangling-ref artifact (class 2)
go run ./longlived                    # class 3 probe, long-lived repo
```

Raw outputs: `S7-src/out/*.txt` (first 400 chars per line).
