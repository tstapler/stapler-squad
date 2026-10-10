# Gates: outcomes of Phase 0 (spikes S0 to S7)

Date: 2026-10-08. Branch `chore/sdd-go-git-fork-planning`. Every row cites a file under `spikes/`; numbers are copied from those files, not re-run here. Companion documents: `fork-necessity.md` (Story 0.2.7), `recalibration.md` (Story 0.2.9). `plan.md` is under a planning freeze and is **not** edited by this pass; implied edits are listed in `fork-necessity.md` section 4 for Tyler's approval.

## Summary table

| Gate | Spike | Verdict | One-line reason |
|---|---|---|---|
| G0 | S0 audit and baseline | **DONE, with gaps** | Call-site counts match the plan; 1,006 PATH-resolved git spawns and 61 to 72 s for `./session`; `vc` and `vcs` are both live |
| G1 | S1 fork and `replace` | **GO** (2 criteria open) | Unpatched fork builds, vets, and tests identically; one go-git copy in the graph; rebase of 3 probe patches had 0 conflicts |
| G2 | S2 v5 vs v6 | **ADR-001 CONFIRMED (stay on v5)** | v6 has no stable release; 19+ compile errors in 7 files as a floor; `x/plumbing/worktree` would not delete native code |
| G3 | S3 performance | **PARTIAL: per-op split** | Flip candidates: open, `rev-parse HEAD`, `ResolveRevision`, `merge-base` (marginal). Fail: `status` (240x), `diff` (3.6x to 9x), `Log`. Writes, edge probes and index v3/v4 not measured |
| G4 | S4 locking | **PASS on the go/no-go core; no F1** | Scoped lock layer built on public API; stock lost update and partial-read segfault reproduced; read-your-writes matrix equals CLI for 9 ops; `Checkout`/`Reset(Hard)` diverge |
| G5 | S5 credentials and transport | **GO on mechanism; live checks manual** | No fork patch; 7 new findings (F1 to F7) change test and preflight text; real github.com/GHE not run |
| G6 | S6 repack | **PASS: wrapper works, F2 stays in-repo** | Fresh open plus retry gave 0 residual errors; max 3 attempts observed, so retry must be at least 3 (recommend 5) |
| G8 | S7 `refs` linked-worktree HEAD | **Outcome B** | Production symptom not reproduced; class 1 root-caused (already fixed); class 2 reproduced only as a dangling ref the CLI also returns |
| G7 | User checkpoint | **OPEN (Tyler)** | O-2, O-10, O-12, O-14, O-15 (see `recalibration.md` section 5) |
| G7a | User checkpoint | **OPEN (Tyler)** | Confirm the empty-fork end state with the evidence below |
| GS, GL, T3 | schedule gates | **Re-dated in `recalibration.md` section 4** | Week numbers in plan section 0.3.1 are stale |

Key to evidence labels: VERIFIED means the spike ran a command or test and recorded output; INFERRED means it was read or reasoned, not run.

---

## G0: baseline and audit (Stories 0.1.1 to 0.1.3)

**Verdict: DONE.** Evidence: `spikes/S0-audit.md`, `spikes/S0-baseline.md`; scripts in `spikes/audit/`.

- Plan counts reproduced by script at HEAD `a7f0809ef`: 62 product `"git",` sites in 29 files, 223 test sites (121 under `session/`), 36 exec/safeexec constructor sites, 30 non-test go-git importers, 30 `LookPath("git")`, 20 `Setenv("PATH"` sites. One discrepancy: the plan's "10 test `exec.Command("git"` sites" is 10 true calls but the script's regex matches 11 because of `session/unfinished/gogit_vcs_reader_shellout_bench_test.go:38` (a `safeexec` bench). The 62 includes about 4 non-spawn matches (`"reason"` 2, `"npm"` 1, `"hg"` 1), so it is a grep count, not a spawn count.
- Baseline: **1,006 git spawns** in `go test ./session` via a PATH shim, identical in 3 of 3 runs (rev-parse 447, config 135, fetch 79, commit 53, add 51, diff 40, init 33, checkout 32). Wall time 61.2 to 72.1 s (median 61.9 s) under load 12 to 32; one pre-existing failing test (`TestSessionRestartWithConversationContinuity`). Mutex delay 51.62 s summed (single run, fraction=1, perturbed).
- Destructive call sites D1 to D9 listed (Task 0.1.1c). Key finding: `session/instance.go:2278,2300-2303` (pause) and `:2467,2485` (stop) continue to `Remove()` when `IsDirty()` **errors**. D8 (worktree removal) has no dirty check of its own and relies on gates D1 to D7.
- `session/vc` vs `session/vcs`: both live, disjoint capabilities, 9 combined spawn sites; `vcs` has no tests. Recommendation: merge into `vc` after the Epic 2 facade exists; do not delete (O-11 still Tyler's).

**Open conditions**: (1) 1,006 is a floor (absolute-path git and `Setenv("PATH")` tests are invisible to the shim); (2) no live create-work-pause-cleanup spawn count; (3) the shim lives in `/tmp/ssq-shim`, not at the plan's `scripts/git-spawn-shim/git`; (4) wall times are noisy (load average 12 to 32).

**Changes in the plan**: Story 0.1.1 gate regex should exclude the safeexec bench; Story 1.1.2b marks both `vc` and `vcs` live; Story 2.1.2 must not call `vcs.GitClient` dead; Story 2.2.2c gains a fail-closed requirement (see `fork-necessity.md` section 3, item 9); Story 0.1.2 baseline numbers replace the plan's 39 to 52 ms spawn assumption (measured `/usr/bin/git` `rev-parse HEAD` p50 is 10.5 to 18.1 ms; the dotfiles wrapper at `~/.local/bin/git` adds about 12 ms per spawn, S3).

## G1: fork bootstrap and `replace` (Story 0.2.1)

**Verdict: GO.** Evidence: `spikes/S1-fork-bootstrap.md`.

- Local stand-in fork (`/tmp/s1/go-git`, branch `ssq/v5` from `v5.19.2`, tag `v5.19.2-ssq.0`). With `replace` to a directory and to a tagged module via a file GOPROXY: `go mod tidy`, `go mod verify`, `go build ./...`, `go vet ./...` all exit 0; `go list -m` shows one go-git copy `v5.19.2 => ...`.
- `go test ./session/git/...`: the same 3 failures before and after (two macOS `/var` vs `/private/var` path compares, one test needing `../../.gitignore` that the scratch copy omitted). Identical failure set is the criterion that held; "all exit 0" was not achievable in this environment.
- No third-party package imports go-git (empty filtered `go list -deps` output), so type identity is not at risk today. `tools/lint/go.mod` has 0 go-git references: no replace needed there.
- **Correction to ADR-002**: Go does not require the replacement's `module` line to match; both an unchanged upstream line and a `tstapler` line built. Recommendation: keep the upstream `module` line (no conflict on line 1 at rebase). Rename remains rejected.
- Rebase v5.19.2 to v5.19.3: 22 files, 1,937 insertions, 90 deletions upstream (10 non-test files, 465 / 36); 3 additive probe patches rebased with **0 conflicts** (feeds O-7, caveat: only additive patches probed). The bump pulls `go-billy/v5 v5.9.0 to v5.9.2`, `sha1cd v0.6.0 to v0.7.0`, `x/crypto v0.55.0 to v0.56.0`.

**Open conditions** (G1 sign-off): Task 0.2.1a (real fork, needs Tyler's owner/name per Story 1.2.0), Task 0.2.1c (public-fork CI access, INFERRED until a real tag exists), whole-repo tests not run (scoped to `./session/git/...`). **Cleanup debt**: fake `v5.19.2-ssq.0` and `v5.19.3-ssq.0` zips were extracted into the real `GOMODCACHE` (`github.com/tstapler/go-git/` under the module dir and under `cache/download/`); remove them before the real tags are published or `go` raises a SECURITY ERROR on hash mismatch.

**Unblocks**: Story 1.2.0 to 1.2.3 (after G7a), Story 6.3.1. **Changes**: ADR-002 text (module line), Story 1.2.1 hygiene (do not edit the fork's `go.mod`), Story 6.3.2 (first rebase number: 0 conflicts for 3 additive patches).

## G2: v5 versus v6 (Story 0.2.2)

**Verdict: ADR-001 CONFIRMED.** Evidence: `spikes/S2-v5-vs-v6.md`.

- Latest v6 is `v6.0.0-beta.1` (2026-10-04); no `v6.0.0`. v5.19.3 shipped the same day (13 PRs, all fixes, including `transport/http` credentials across redirects #2358, cyclic delta chains #2340, two `x/crypto` security bumps).
- Mechanical v6 rewrite of 80 files (30 non-test): `go build -gcflags=-e` gives 19 errors in 7 files in only the first two package layers, with downstream packages and test files not yet visible (a floor). API-semantic changes: `billy.Filesystem.ReadDir` returns `[]fs.DirEntry`, `Worktree.Filesystem` is a function, `PlainOpenOptions.EnableDotGitCommonDir` removed, `plumbing.Hash` is a struct, several constructors take an extra argument, `idxfile.Index` needs `Close`, `NewPackfileWithCache` removed. The `session/unfinished/gogitstore` mmap store (13 files) is the heaviest. `tools/lint/norawgitopen/analyzer.go:97` hardcodes the v5 path.
- `x/plumbing/worktree` has Add/Remove(metadata only)/List(names only)/Open/Init; no prune, lock, or move; replaces at most 2 of 8 native worktree functions.

**Open conditions**: v6 migration hours unmeasured; v6 `Add` race claim not re-run; v5 EOL not searched. Re-check at 2027-01-02 (90 days from v5.19.3) if v6.0.0 has shipped.

**Changes**: take v5.19.3 as the first real fork tag base (credential-redirect and cyclic-delta fixes); note v6 would also require migrating `norawgitopen` and replacing `EnableDotGitCommonDir` behaviour. No story is unblocked or removed.

## G3: performance versus CLI (Story 0.2.3)

**Verdict: PARTIAL. Per-op split, rule "flip only if go-git p50 <= CLI p50".** Evidence: `spikes/S3-performance.md` (read-only, `/usr/bin/git`, 30 iterations, go-git open included per iteration; load average 5 to 31).

| Op (large repo, 27 GB `.git`) | CLI p50 | go-git p50 | Verdict |
|---|---|---|---|
| open only | 18.1 ms | 0.48 ms | flip |
| `rev-parse HEAD` / `Head()` | 17.4 ms | 0.54 ms | flip |
| `rev-parse HEAD~5` / `ResolveRevision` | 18.6 ms | 8.0 ms | flip |
| `merge-base HEAD HEAD~50` | 20.4 ms | 14.9 ms | flip, thin margin (medium repo: 14.1 vs 15.5 ms fails by 10% against `/usr/bin/git`, passes against the PATH wrapper) |
| `rev-list --count` / `Log` | 13.7 ms | 24.2 ms | stay CLI; also not semantically equal (CLI 278 vs go-git 200; 591 vs 200 on the medium repo) |
| `diff --shortstat HEAD~1 HEAD` / `Patch.Stats` | 16.6 ms | 102.7 ms | stay CLI (6x); medium repo 4.3x |
| `diff --shortstat HEAD~200 HEAD` | 233.5 ms | 852.4 ms | stay CLI (3.6x); medium repo 9x |
| `status --porcelain` / `Worktree.Status` | 238.2 ms (67 ms on a quiet second sample) | **57,207 ms** (n=5, 56.5 to 70.9 s) | stay CLI (240x); medium repo 227 vs 57 ms (4x) |

- **Correctness divergence**: go-git `Status` on the large repo returned **896 entries** where the CLI printed 2 porcelain lines (ignore rules not honoured; cause not investigated).
- The plan's assumption that `status` is 182 ms and `rev-parse HEAD` 39 to 52 ms is superseded by these measurements (and the PATH wrapper adds about 12 ms per spawn: 23.1 vs 10.8 ms).

**Open conditions**: not measured: worktree add (existing branch), commit and any write (needs a scratch clone), index v3/v4 probes, split-index `ErrUnknownExtension` mapping, rename/CRLF/LFS/shallow edge probes (Task 0.2.3b not run), a like-for-like rev-list count (`--first-parent`). Single machine, single day. The medium repo is `~/ws/tn-nftitus` (whether it is stapler-squad-managed is unverified).

**Unblocks / changes**: Epic 2.1 may flip `Head`, `ResolveRef`, `ResolveRevision`, `MergeBase` per-op. Epic 2.2: `IsDirty` stays on `worktree_dirty_fast.go`; status lists, diff text and numstat **stay CLI unless Tyler chooses a patch or an in-repo implementation** (this is a direct conflict with the "no git" end state: see `fork-necessity.md` section 3, item 1 and decision O-14). Story 4.2.3 (F4) is the only candidate fork patch and it is **not proven** by S3, because a wrapper (own status walker, as `worktree_dirty_fast.go` already is) has not been shown unable to do the job. Epics 2.3 and 4.1 "conditional on G3" cannot be cleared for commit and checkout until the write benchmarks exist.

## G4: CLI-compatible locking (Story 0.2.4, go/no-go core only)

**Verdict: PASS on the core; F1 is out of the fork; the lock layer lives in-repo (Story 2.3.1).** Evidence: `spikes/S4-locking.md`, source in `spikes/S4-src/` (`lock.go` 250 lines, 8 test files); go1.26.6, Apple Git 2.50.1.

- Public-API design works: embed `*filesystem.Storage`, override `Index`, `SetIndex`, `SetReference`, `CheckAndSetReference`, `Reference`, then `git.Open(wrapper, fs)`. `WithIndexLock` takes `<gitdir>/index.lock` (`O_EXCL`) before `fn`, `SetIndex` rewrites the held lock, later `Index()` reads the lock file (read-your-writes), refs buffered behind `<ref>.lock`, commit phase index first then refs.
- Reproduced: stock lost update (T1: 6 of 20 iterations lost an entry); stock in-place truncation (T3b: 23 anomalies in 416 CLI invocations, each a `signal: segmentation fault` of Apple Git 2.50.1 on the half-written index); stale-read hazard (T8: `Commit(All)` tree differs from CLI without read-your-writes).
- Passing: scoped partial-read (T3a: 220 invocations, 0 anomalies), CLI blocked while scope holds (T13), no leak on error/panic (T4), foreign lock never deleted (T5), no deadlock under `-race` (T6), abort consistency and ref-lock window (T9a to T9d), census = 23 sites equal to the plan's list (T10).
- Read-your-writes vs CLI twin: equal for `Commit(All)`, `Add`+`Commit(Amend)`, `Add`+`Commit`, `Restore(Staged)`, `Reset(Mixed)`, `Reset(Soft)`, `AddWithOptions(All)`; probe-only equal for `Remove`, `RemoveGlob`, `Move`.
- **Divergences (stock go-git semantics, independent of the lock layer, T12)**: `Reset(Hard)` deletes untracked files (CLI keeps them); `Checkout(Branch|Hash, Keep)` leaves a new file staged as `A` (CLI removes it); `Commit(All+Amend)` refused by go-git (`all and amend cannot be used together`).
- T2 had 0 collisions (CLI start dominates timing); T13 and T6 are the standing regression tests, not T2.

**Open conditions (do not read PASS as covering these)**: linked worktrees (`<CommonDir>/worktrees/<name>`) untested; ref delete, `packed-refs.lock`, `HEAD.lock`, `config.lock` and the 60 s `pack-refs`/`branch -d` stress not implemented; no reflog lines written (go-git writes none); journal, token/xattr, SIGKILL recovery, 60 s age rule, instance flock not implemented; worktree-write fault injection not run; index v3/v4 round trip and racy-git mtime not tested; census test compares only the count, the list is below.

**Census list (23 call sites, VERIFIED T10; commit this to the repo when Task 0.2.4c lands)**: `status.go:122` (preloadStatus); `submodule.go:61,259,394`; `worktree.go:375,436,452,484`; `worktree_commit.go:62,109,125`; `worktree_status.go:134,250,354,390,412,441,586,603,682,708,723,741`.

**Unblocks / changes**: Story 2.3.1 proceeds in-repo; Story 4.2.1 (F1) is cancelled; Epic 4.1 "INFERRED: admin-file writes share the lock protocol" is still unproven (linked worktrees untested). Operations that stay on the CLI as `unsafe_worktree_write` are confirmed and now have measured reasons: `Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)` (worktree written before `SetIndex`, and the first two diverge from CLI semantics), plus `Remove`/`RemoveGlob`/`Move` (index equal on the happy path, abort case not fault-injected). `CommitOptions.Validate` mutates its options (it sets `Parents` to HEAD), so a wrapper must build options per call. A grep of product code (`ResetOptions{`, `HardReset`, `"reset", "--hard"`, excluding tests) finds no go-git `Reset(Hard)` use today (VERIFIED by the grep run for this document, not by the spike).

## G5: credentials and transport (Story 0.2.5)

**Verdict: GO on mechanism; live GitHub/GHE checks are manual (validation.md G-4).** Evidence: `spikes/S5-credentials-transport.md`, `spikes/S5-src/` (24 tests, `go test ./credential -race` passed in 44.9 s).

- No fork patch: every hook used is public (`transport/http.AuthMethod`, `client.InstallProtocol`, `http.ClientOptions`, `ssh.AuthMethod`).
- Verified with zero `git` spawns (spawn shim): helper protocol client round trip, `git-credential-osxkeychain` and `gh auth git-credential` exec directly (dummy hosts), `PlainClone`/`Push` through a custom client, host-scoped `TokenSource`, process-group kill on helper timeout.
- Fallback: one HTTPS auth failure then one SSH attempt with an agent `AuthMethod` (no userinfo, no token); counts asserted; `insteadOf` resolver matches `git remote get-url` for 12 of 12 cases.
- Findings: **F1** same-hostname different-port redirect forwards `Authorization` on stock go-git/net/http (`A.withAuth=1 B.withAuth=1`); a `CheckRedirect` that strips on `host:port` change fixes it. **F2** with the transport cache on (default), a non-`*http.Transport` `RoundTripper` plus any TLS option panics at `plumbing/transport/http/common.go:323`. **F3** one `Match` block makes kevinburke/ssh_config silently return `""` for every key (drops Hostname/Port aliases for the whole file). **F4** `git-credential-osxkeychain` is not on PATH on macOS (git finds it via `git --exec-path`), so resolve through known libexec dirs. **F5** `client.Protocols` is an unlocked map: install once at startup. **F6** resolver-based `insteadOf` is required (go-git reads only repo-local config, ignores `[include]`, and cannot hold multi-valued `insteadOf`). **F7** `file://` and local-path remotes spawn `git-upload-pack` (or `git --exec-path`): "zero git spawn" is false there; an in-process `server.NewClient(server.NewFilesystemLoader(osfs.New("/")))` registered for `file` spawned nothing for clone and push-back. **F8** user-less `ssh://` URLs authenticate as the OS user, not `git`.
- ssh_config: go-git ignores `User`, `StrictHostKeyChecking`, `IdentityFile`, `ProxyCommand` (probe reports each); honours `Hostname`/`Port`; known_hosts checked against the resolved host:port; hashed known_hosts accepted.

**Open conditions**: real github.com/GHE fetch and push; real keychain item; real OpenSSH server, real ssh-agent/1Password agent, `IdentityAgent`; the real injection through `session/gitwiring` of the Go-native keychain `TokenSource` (the spike used a stub closure); GitHub's actual status for a bad token on a private repo (fallback trigger set may need `repository not found` for github.com); whether F1 is exactly CVE-2026-41506; in-process `file` server unverified for hooks, concurrency with a CLI holding `index.lock`, shallow/protocol-v2 parity, large repos; wire-level proof of no token to the SSH host (argued by construction only). Note v5.19.3 (#2358) already contains an upstream fix for credentials across redirects; whether it covers the same-host-other-port case was not tested.

**Unblocks / changes**: Epics 3.1 and 3.2 are feasible without the fork; Story 3.1.2 test list gains F1 and F2; Story 3.1.1 gains F4; Story 3.1.3 gains the F8 explicit user and the trigger-set caveat; ADR-006 preflight list gains `Match` anywhere in the file, `IdentityAgent`, `ProxyJump`, `includeIf`; plan text "zero git spawn" for local transport (Story 3.2.1 and the Revision 4 tripwire) must be reworded or the in-process `file` server adopted after its own gate (decision O-14 item b).

## G6: concurrent repack (Story 0.2.6)

**Verdict: PASS. A fresh-open-plus-retry wrapper works; F2 stays in-repo.** Evidence: `spikes/S6-concurrent-repack.md`, `spikes/S6-src/results.txt`; 50-commit repo, 8 readers, 1 maintenance goroutine.

- Stock go-git reproduces `plumbing.ErrObjectNotFound` under `repack -ad` (333 in 5 s; 37, 0, 934 in three 8 s runs; 0 in one 60 s run: non-deterministic), `gc --prune=now --aggressive` (265 in 20 s), `pack-refs` (16 in 20 s). Once a shared handle is stale the errors repeat.
- `fresh` without retry still failed 15 times; **`fresh-retry` gave 0 residual errors** under repack (16 retried reads, max 3 attempts, p50 208 ms, p99 1.05 s, max 1.34 s) and gc (max 3 attempts, p99 115 ms); `pack-refs` needed no retries.
- `Reindex()` on a shared handle without a lock **panics** (nil map in `loadIdxFile`); behind a mutex it showed 0 failures but also 0 retries, so it was not shown to fix a stale handle.
- **Cost**: fresh opens cut read throughput about 15x (6.6k vs 97k reads per 60 s) under this load.

**Open conditions**: repo had 50 commits (larger repos slow opens and retries); only `CommitObject` + `Tree` exercised; the in-repo `repackspike_test.go` was not written; single APFS machine.

**Changes**: retry bound is at least 3 attempts (recommend 5, 10 to 50 ms backoff), not "retry once" as Story 0.2.6 and the handle-lifetime rule say; `Reindex()` is banned on shared handles; reads still failing after the bound fall back to CLI by reason `object_not_found`. Story 4.2.2 (F2) is cancelled. The 15x throughput cost matters for the soak ceiling and SLO checks in Story 5.2.1: it applies to the fresh-handle rule the plan already adopted (adversarial C5), and bulk loops must open once per batch with retry around the batch, not per object.

## G8: `refs` linked-worktree HEAD (Story 0.2.8)

**Verdict: Outcome B (production symptom not reproducible within budget), with two reproduced adjacent findings.** Evidence: `spikes/S7-refs-head.md`, `spikes/S7-src/`.

- Why the mitigation exists: commit `f4d7d5e1c` (2026-07-12); `EnableDotGitCommonDir` was added later (2026-09-02), so the production symptom was seen with the flag off. The commit never recorded what `git rev-parse HEAD` returned at that moment, so "go-git differs from CLI" was never established.
- **Class 1 (reproduced, root-caused, already fixed)**: without `EnableDotGitCommonDir`, `Head()` on a linked worktree returns `ErrReferenceNotFound` 100% of the time (30,587 of 30,587; 27,842 of 27,842 with no racers); with it, 0 errors in every run. `defaultPlainOpenOptions` already sets it (`session/git/util.go:36-39`).
- **Class 2 (reproduced only as a dangling ref)**: with a `commit-tree` + `update-ref` writer racing `gc --prune=now`, go-git returned a SHA with no object in 182, 428 and 246 reads across 3 runs of 120 s; in every case the CLI `rev-parse HEAD` returned the **same** SHA (0 go-git-only phantoms) and `git fsck` reported `invalid sha1 pointer`. Cause: gc deleted the unreachable commit before `update-ref` published it. Realistic writers (`git commit`) over the 10-minute run (125,892 reads, 286 branch advances) gave 0 class-2 hits.
- **Class 3 (stale-but-existing SHA)**: 0 occurrences; the predicate cannot detect it, so shadow stays mandatory.
- Transient: `CommitObject` failed while the CLI could `cat-file` the object 155 times in the 10-minute run (about 0.12% of reads) from pack removal between listing and open; recoverable by the 3 x 20 ms retry.

**Adopted (outcome B)**: object-exists predicate, permanent allow-listed `torn_read`/`object_missing` CLI route on `ResolveRef`, `CurrentBranch`, `RefExists`, `ListRefs` only, soak ceiling **0.25%** of `(torn_read + object_missing) / refs-cohort calls` (derived from a hostile stress rate, an upper bound; revisit after the 7-day shadow window), shadow mandatory, `getHeadCommitSHA` fallback not deleted in Story 6.2.1.

**New requirement from S7**: on `object_missing` the CLI answer must itself be object-checked (`git cat-file -e <sha>^{commit}`); if the CLI agrees the SHA is dangling, return a typed `ErrRefDangling`, never a SHA. Today's `getHeadCommitSHAViaCLI` (`util.go:394`) has no such check. The predicate also must retry before classifying: CLI sees the object means `torn_read`, not `object_missing`.

**Open conditions**: single machine; low commit rate (about 25 per minute under contention); the spike used 17 minutes of racing; class 3 only probed with a 30-commit long-lived handle.

**Unblocks**: `refs` may leave `shadow` after the 7-day window (Story 2.1.3) with no waiver. **Changes**: Stories 2.1.1 and 2.1.3 (retry, `ErrRefDangling`, linked-worktree conformance test `Open_should_SetCommonDir_When_PathIsLinkedWorktree`), soak ceiling value in Story 5.2.1.

## G7 and G7a (user checkpoints)

Both are **OPEN**. Neither can be satisfied by this document. See `recalibration.md` section 5 for the decisions Tyler must make and the defaults the plan will use if he is silent. G7a is confirm-only: the evidence supports an empty pinned fork F0 (see `fork-necessity.md`), with one unresolved tension (status and diff in-process, below) that Tyler should see before confirming.

## Backlog list (planning freeze: findings that do not edit plan.md)

1. Remove the fake `ssq` go-git zips from `GOMODCACHE` (G1 cleanup debt).
2. The spawn shim should live at `scripts/git-spawn-shim/git` as the plan says (currently `/tmp/ssq-shim/git`).
3. Add characterization tests for `vcs.GitClient.SwitchTo` (including `Abandon`) before touching it; it has no tests.
4. Write the census list to a committed fixture when Task 0.2.4c lands.
5. File upstream: unchecked type assertion at `plumbing/transport/http/common.go:323` (S5 F2), `ssh_config` `Match` silent failure (S5 F3, upstream is kevinburke/ssh_config), `Worktree.Status` ignore rules and speed (S3).
6. Re-run S3 with the PATH wrapper as the CLI baseline in a second column, and a quiet machine.
