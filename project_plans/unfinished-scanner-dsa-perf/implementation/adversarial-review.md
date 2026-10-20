> **Final status (Phase 4 patch)**: this file records re-review pass 1 (BLOCKED, 2 blockers). **Pass 2 found 1 blocker, N3** (the workload-realism circularity of Story 0.1.5), which was **repaired in iteration 3 and not re-reviewed**. The consistency report, the pre-mortem and the validation plan produced after pass 2 have had their findings **applied in the Phase 4 patch** (see the plan's "Phase 4 patch log"); the patch itself has not been adversarially re-reviewed. Treat the verdict below as historical.

# Adversarial Review: unfinished-scanner-dsa-perf (re-review pass 1)

**Date**: 2026-10-15
**Verdict**: BLOCKED (2 open blockers; 8 of the 10 original blockers confirmed resolved)

Re-review history: the first review was BLOCKED with 10 blockers (B1..B10). This pass re-checks only those, plus the repair itself.

Method: read the repaired plan.md in full (including Repair log), ADR-001/004/006 text where cited, requirements.md:13-23, and opened source: `session/unfinished/gogitstore/{store,storer}.go`, `gogit_vcs_reader.go` (AheadBehind 1246-1300, pruneRepoCache), `server/dependencies.go:1250-1290`, `go.mod:231`, `go.sum`, `Makefile`, `benchmark.yml:94,120`, and all `_test.go` imports of `safeexec`. No builds or tests run. VERIFIED = opened or computed this session; INFERRED = reasoning.

## Original blockers: status

| # | Status | Evidence |
|---|---|---|
| B1 lockstep not landed | Resolved | Story 0.0.1 commits it alone, keeps `legacyStack*` oracle, differential test, golden divergence list, "lockstep-only" row. requirements.md:14 already says UNCOMMITTED (VERIFIED). |
| B2 unowned/unreachable metrics | **Partly resolved, see N1** | 2.08x ceiling stated; Stories 0.1.1d/4.2.4/4.4.1 own buckets, but the projection is built on the wrong profile state. |
| B3 no gate can fail | Resolved | G1, G2, G4-G10 compare against retained oracles or numeric ceilings and each names a failure; HOLD rule exists (5.2.1/5.2.2). Residual ambiguity in G2/G3 (C1). |
| B4 confounded A/B | **Partly resolved, see N2** | Same binary, flags off vs on, ports 62871/62873 match the CLAUDE.md port block (VERIFIED), live deploy must carry flags on. Workload and corpus undefined. |
| B5 shallow/negative cache | Resolved | `Truncated` rows (no entry for absent hash), reset on `ShallowStamp` change, stamp in AheadBehind/behind/message/reach keys, deepen-in-place test through the shared graph, Task 0.1.4a settles shallow location. |
| B6 eager O(R) Extend | Resolved | Demand-filled graph, `ErrGraphFull`/`ErrPressure` fall back to legacy per call, G1 and G9 compare cold decodes to oracle. |
| B7 deadlock / key | Resolved | Phase 1 under `entry.mu`, phase 2 without; flight key `(RepoKey, hash)`; 5 s channel-deadline race test. VERIFIED `WorktreeStorer.EncodedObject` delegates to the shared store with a mutex-guarded per-worktree `packHandleCache` (`storer.go:105`, `store.go:655-679`). `pruneRepoCache` already closes stores without `entry.mu` today, so phase 2 without `entry.mu` adds no new close hazard. |
| B8 false clean | Resolved with residual (C2) | Key now includes `.gitignore`/exclude/global stat signatures and tracked-set generation; Tier A never skips the stat pass; model-based oracle. |
| B9 go.sum rollback | Resolved | VERIFIED `go.sum:358-359` has only tstapler entries; plan states `go get` + `go mod tidy` and rehearses with `-modfile`; probe replaces the regex. Probe itself is UNVERIFIED (story is conditional). |
| B10 native git in an AC | Resolved for the cited AC | Replaced by a fork-side go-git `DiffTree` test. Residuals in C3/C4. |

## Open blockers

- [ ] **N1. The CPU/alloc projection gate and the "decode-path 2x" target are defined against the wrong baseline state (S0), not the state the plan ships on (S1).** VERIFIED: requirements.md:14 says lockstep cut a 2,500-commit fixture from 114,780 to 657 allocs/op and from 7.8 MB to 54 KB/op. Story 0.0.1 commits that change, and the plan calls the A/B S1 (committed HEAD, flags off) vs S2. But Task 0.1.1b pulls the profile from the live process (S0, built from `39fa33a11`, old stack walk), and Task 0.1.1d builds the bucket-ownership table and the SHORTFALL/PASS projection from it. Its draft assigns `DecodeCommit` 51.9% of alloc to Stories 1.2.x/2.x. Consequences:
  - The projection can PASS by crediting stories with a decode bucket that Story 0.0.1 has already removed (S0 to S1 gain is not this project's).
  - The 96.3% arithmetic (0.519 x r = 0.5) is S0 arithmetic. Story 5.2.1 and the Verification Matrix apply "decode bucket cut >= 96.3%" to control (S1) vs candidate (S2), where the bucket is a small fraction and 2x process-wide is likely unreachable. The gate is either vacuous or unpassable depending on which baseline is meant, and the plan never says.
  - Wave A's exit guard only requires the S1 binary to have run 30 min; no profile of S1 is required to feed 0.1.1d.
  - Recommendation: take the 0.1.1b profile (CPU, `alloc_space`, `inuse_space`) from the S1 control binary, rebuild the 0.1.1d table and the 24.3-point / alloc arithmetic from it, restate the requirements.md targets relative to S1 (keep S0 only as attribution), and get the owner's confirmation of the restated targets before Wave B. Make the 5.2.1 decode-bucket gate name its baseline explicitly.

- [ ] **N2. The Story 5.2.1 A/B cannot run as specified, so the only measured CPU gate cannot fail meaningfully.** VERIFIED: scanner state lives per config dir (`server/dependencies.go:1255-1260`, `unfinished_state.json` under `config.GetConfigDir()`), and a fresh `STAPLER_SQUAD_INSTANCE` has no sessions or registered repos. The AC says both instances scan "the same >= 5 owner repos" for >= 30 min but gives no mechanism to register the corpus (copy state file, `AddPinnedRepo`, watch dir) and no identical event driver; idle repos produce only the 5-minute tick. Also, "scanner CPU share <= 15%" on a session-less instance is not comparable to the live process's 39.3% (different denominator), yet a miss there means HOLD and blocks Story 5.2.2. Where each instance's flags are set (per-instance `config.json`) is also unstated.
  - Recommendation: add a task that builds the corpus (registers the same repos on both instances) and drives a scripted identical event stream (commits/touches in fixture clones, go-git only), define the A/B gate as the scanner CPU-seconds and allocs ratio S2/S1 under that workload (not absolute share), reserve the share-of-process-CPU <= 15% judgement for the live Story 5.2.2, and state where the flags are set per instance.

## Concerns

- [ ] **C1. G2/G3 fixture wording makes two gates ambiguous or unpassable.** G2 says `DecodeCount(new, W=3) == DecodeCount(new, W=1)` and `<= committed-old W=1`, but Story 2.1.2 uses three distinct tips (decodes = distinct visited commits), which exceeds W=1. State that the G2 fixture has all worktrees on the same head/base, or compare per distinct tip. G3 says "warm `AheadBehind`" `AllocsPerRun==0`, but phase 1 resolves refs and stats `shallow` on every call after the TTL; only the TTL-hit path and the pure walk can be alloc-free. Name which path.
- [ ] **C2. Story 4.2.3's "complete invalidators" is overstated.** INFERRED: a per-directory verdict keyed on the `.gitignore` files "consulted" does not change when a new `.gitignore` is created in an ancestor directory (its own mtime changes, but an unchanged descendant `sub/` keeps its cached verdict, and a `!pattern` in the new file can expose files there). Add negative-stat markers (existence of `.gitignore` at every ancestor level) to the key, or invalidate descendants when a directory's `.gitignore` set changes, and put "create `.gitignore` in an intermediate dir" in the model-based generator. Bound is 5 min, scan_skip is flag-gated, so not a blocker.
- [ ] **C3. Story 0.0.3's native-git inventory is wrong, and a "VERIFIED" claim is false.** The plan says `gitRunInternal` is used by 5 test files (limits, objectcache, gogitstore_eviction, mmap_flag, mergebase). VERIFIED by grep: only `gogit_vcs_reader_limits_test.go` (20 calls) and `gogit_vcs_reader_objectcache_test.go` (1). Meanwhile `safeexec`/native git remains in files the story does not list: `gogit_vcs_reader_gitignore_test.go:21,36` (`git init/config/...` bench fixture), `gogit_vcs_reader_matchercache_test.go`, `vcsreader_test.go`, `state_test.go:12,249-251` (`git add`, `git commit` via `runCmd`). `TestNoNativeGitInPackage` would catch them (so the policy is not violated by design), but the Files list and estimate understate the work and the claim should be corrected from a fresh grep.
- [ ] **C4. Story 0.0.1's AC asserts git commit contents and message.** Plan line 10 carves "ordinary VCS commands" out as workflow, but this AC's pass condition is a VCS outcome. Under the hard owner policy, confirm the carve-out with the owner or reword the AC so nothing is judged by git output (state it as an instruction, not an acceptance check).
- [ ] **C5. Stories 1.2.1, 2.1.1-2.1.3 and 2.2.2 stay unconditional although the lockstep may have removed the hotspot they target.** After Story 0.0.1 the per-call `parentMemo` walk is already 175x leaner on the fixture; the remaining value of the shared graph is the W-fold duplication and warm hits, which only the S1 profile can size. Gate them (scheduling, not a cut) on the S1 `DecodeCommit`/walk share, as Stories 2.2.1/3.1.1 already are. Folded with N1's fix.

## Minors

- Repair log's last bullet says requirements.md:14 "still says Already landed"; VERIFIED it now says UNCOMMITTED. Stale sentence.
- `$(shell go doc ...)` in the Makefile runs on every `make` invocation (load 110-160); cache it in a sentinel file. Probe is UNVERIFIED until Task 1.2.3b.
- Stats claims re-checked: 41 stories, 89 tasks, 8,120k raw, 8,363k CU, x1.75 = 14,635k (all computed, match the plan's ~14.6M); conditional set 1,630k raw / 1,678k CU matches; Wave 0 + A-I = ten stages. The estimate is internally consistent; inputs remain uncalibrated guesses as the plan itself says. CU/raw ratio 1.03 is consistent with the stated mix (1.027).
- `G:` line numbers in plan.md are working-tree orientation only (plan says so); `Makefile` 49 `go build|test|vet` lines, `go.mod:231`, `go.sum:358-359`, `benchmark.yml:94,120` re-verified.

## Newly introduced issues scanned for

- Gates that cannot fail: none found beyond the G2/G3 ambiguity (C1) and the projection gate's baseline (N1). Gates G1, G4-G10, 0.0.1 differential, 4.2.3 model-based, deadlock test all have a stated failing condition.
- Native git/shell-out in product, test, benchmark, fixture or CI path: none proposed. Only `go doc`, `go mod`, `go version -m`, `ss`, and Makefile grep appear, none are git. See C3/C4 for residue.
- Required stories without profile backing: C5 (plus 2.2.1/3.1.1/3.3.1 correctly gated).
- Unverified claims stated as fact: C3 (5 files), the Makefile probe (labelled INFERRED in self-review, acceptable).
