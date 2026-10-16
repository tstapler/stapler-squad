# Requirements: unfinished-scanner-dsa-perf

**Date**: 2026-10-15
**Type**: feature addition (performance re-architecture of an existing package)
**Complexity**: 3 — system design (multiple epics, fork-patch dependency)

## Problem Statement
`session/unfinished` (the background scanner that finds worktrees with unfinished work) is the largest CPU and allocation consumer of the long-running `stapler-squad` process. Its cost scales as `worktrees × event rate × per-worktree cost`, and several per-worktree costs are recomputed for data that worktrees of one repo share (commits, reachable sets, HEAD tree hashes, gitignore matchers). The owner's machine runs at load 110–160 and the process's allocation churn drives GC work that is billed to the scanner goroutines, so every extra millisecond and allocation here slows every concurrent AI session.

## Baseline
All figures measured 2026-10-15 on the owner's machine (profiles taken from the live process, up ~24h). VERIFIED in Phase 2 (research): the live binary is built from `39fa33a11` (vcs.modified=true), restarted 2026-10-15 13:55, and lacks the fsnotify-filter commits `c46c69faf`/`150168a4c` and the lockstep walk; the profiles below predate that restart.
- CPU (Pyroscope, 30-min merge): scanner `Scanner.worker` 39.3% cum; `HasUncommitted` 21.3%; `diffShortstatUncached` 12.4%; GC mark/clear (`memclrNoHeapPointers`, `tryDeferToSpanScan`, span init) ≈ 28% flat; `syscall.rawsyscalln` 32% flat.
- Allocations (`-alloc_space`, 24h): 13.4 TB total; `object.DecodeCommit` 51.9%; of its callers `findMergeBase` 60% and `countCommitsTo` 40%. Inside decode: `bytes.growSlice` 30.8%, `MemoryObject.Write` 10.8%, `bufio.ReadBytes` 10.3%, `continuationCont` 9.2%, `Buffer.String` 8.0%.
- On this branch: lockstep `findMergeBase` + `parentMemo` landed in commit `f0a75ac10` (after `39fa33a11`) (2500-commit history: 114,780 → 657 allocs/op, 7.8 MB → 54 KB/op; the live binary still has the old stack-based walk from `39fa33a11`); `.git` fsnotify filter and debounce (`c46c69faf`, `150168a4c`) are committed on this unmerged branch and not in the live binary; untracked-walker allocation cuts and dir-fd `lstat` in `HasUncommitted` (#1038/#1039) are in the live binary.
- **The baseline the project ships on is S1** (HEAD after the lockstep commit, flags off); the figures above are **S0** (live `39fa33a11`, old stack-based walk) and are kept for attribution only. Project targets are stated and measured relative to S1, taken on a manual control instance under a scripted workload (plan Stories 0.1.5 and 0.1.1).
- Known structural duplication: `reachableSetCache`, `aheadBehindCache`, `commitMessagesCache` and the HEAD-tree-hash and matcher caches are keyed by worktree path, although worktrees of one repo share commits and objects.

## Users / Consumers
The stapler-squad server (scanner → Unfinished/Up Next UI, review queue), and every AI session on the machine as an indirect consumer of CPU/memory headroom.

## Success Metrics
Measured as before/after on the same workload (benchmark suite plus Pyroscope 30-min windows), tied to the S1 baseline above. **Numeric targets below are proposed, S1-relative, owner confirmation required before Wave B (plan Unresolved Question Q1)**; adjustable by the owner:
- Scanner CPU under an identical scripted workload: S2 (flags on) / S1 (flags off) CPU-seconds ≤ rho, where rho = 15 / s1 and s1 is the S1 scanner share of live process CPU (measured at an S1 live deploy if the owner approves, else estimated from the lockstep-only gain and labelled INFERRED). The absolute live scanner share ≤ 15% (S0 was 39.3%) is judged at the live deploy (plan Story 5.2.2), not in the manual A/B.
- Scanner `alloc_space` S2/S1 ≤ an S1-derived target set at plan Task 0.1.1d from the S1 profile. With d1 the S1 `DecodeCommit` share of `alloc_space`, the decode-path-only ceiling is 1/(1-d1) and a 2× process-wide cut needs a decode-bucket cut of 0.5/d1 (reachable only if d1 ≥ 0.5; otherwise the target is carried by the non-decode buckets). (S0's `DecodeCommit` share of 51.9% capped the gain at ≈2.08×, adversarial-review B2; S0 is attribution only.) Each remaining allocator ≥ 3% of S1 `alloc_space` needs an owning story or an explicit "accepted, not worth it" note.
- Reachable-set/commit-parent work for W worktrees of one repo: O(R) per (repo, base), not O(W·R); asserted by a test that counts decodes across ≥ 3 worktrees of one repo. (Delivered only if the commit-walk stories pass their S1 scheduling gate, plan Task 0.1.1d row SG2; if the lockstep already removed the hotspot they are deferred and this requirement is reported "unmet, deferred by gate", with an ungated characterization test recording the W-fold baseline.)
- Every changed hot path ships a `-benchmem` benchmark comparing old vs new, and a regression test that fails on the old behaviour.
- No functional regression: existing `session/unfinished` tests and result parity (ahead/behind, dirty, shortstat) verified against the old implementation on a corpus.

### Outcome metric (DIAGNOSTIC, not a gate)
The owner's pain is a slow machine, and scanner CPU share is only a proxy for it. So the before/after record (plan Tasks 0.1.1b, 5.2.1, 5.2.2) also carries, per instance and per live window, all labelled **DIAGNOSTIC**:
- (a) scanner CPU-seconds per hour (Pyroscope window or `--profile` pprof window);
- (b) host load average (`/proc/loadavg` on Linux, `sysctl -n vm.loadavg` on macOS; neither is native git) sampled at a fixed interval (every 60 s) across the whole A/B window, reported as min/median/max next to the control window, plus the stapler-squad process's own CPU-seconds so the reader can see how much of the host load it could even account for. Git-hook or tool latency is deliberately **not** measured: it would need native git, which the go-git-only policy forbids; the loadavg samples are the available proxy.
- These never pass or fail a story. Machine load is dominated by other causes (see "Adjacent load causes"), so a flat loadavg across an otherwise successful A/B is an expected outcome, reported as such, not a miss.

### Best-case ceiling on total machine load
The scanner was 39.3% of **one** process (S0, `Scanner.worker` cumulative). The best case this project can reach is to take that process's scanner share to the 15% target, i.e. at most about 24 percentage points of one process's CPU, and the GC share billed to the scanner follows the allocation cut. That is a ceiling on one process, not on host load 110-160. The stapler-squad process's share of total host CPU is UNVERIFIED (the diagnostic above measures it); nothing in this project reduces the other contributors, so the owner should not expect host load to fall by a proportional amount.

### Project-level success definition
The project is **successful** when all of these hold, judged by the Wave H report:
1. Each Success Metric above is reported individually as `met`, `missed` or `unmet, deferred by gate` with its measured number (no row is reported as "mapped" in place of a result).
2. The headline CPU target is `met` at the live deploy (plan Story 5.2.2), **or** the early-stop clause below closed the later waves as `accepted`.
3. Parity holds: `parity_mismatch_total` is 0 over the soaks and the existing and oracle tests pass.
4. A gated metric that closes `unmet, deferred by gate` (for example the O(R)-per-repo metric when the S1 profile shows the commit-walk buckets under their scheduling threshold) counts toward success **only** when the recorded S1 share for that row is under its threshold, which proves the hotspot is not there, and the ungated characterization test keeps the W-fold figure visible in `baseline.md`. If a deferred metric coincides with a missed headline target, the project status is `partial` and the owner decides the next step; it is never reported as success.

### Early-stop criterion (evaluated once, at plan Task 0.1.1d)
Let X = **20%** (proposed; adjustable by the owner in the Q1 confirmation). If the S1 re-profile shows **both** (i) the S1 scanner share of live process CPU `s1` is at most `15 x (1 + X)`, i.e. 18% at X = 20%, and (ii) the further scanner `alloc_space` cut that the Task 0.1.1d bucket table projects as needed to reach the S1-derived alloc target is at most X% of S1 scanner `alloc_space`, then the optimization waves (plan Waves B-F, Wave G's fork adoption, Waves H-I) are closed as `accepted` with the recorded S1 numbers instead of being built. Wave 0 and Wave A deliverables (landed lockstep, split, native-git retirement, harness, `baseline.md`) stay. The owner may override and continue (full scope remains the default posture); the plan states the stop as an evidence-based closure, not a downscale. If `s1` is only estimated (INFERRED, no S1 live deploy), clause (i) cannot close the waves alone: the owner must confirm in writing.

## Size
Large (3–15M CU) — informational estimate in price-weighted cost units for all agents incl. review/verify (bands INFERRED, see ESTIMATION.md). Full scope is the default; this is not a gate. After the plan's x1.75 verification multiplier the estimate is ≈17.1M CU (45 stories, 106 tasks; plan "Effort estimate"), above the top of the Large band; informational only.

**Wall-clock blockers**: owner decision on proposed targets (owner: Tyler); fork tag cut and `replace` bump per fork patch (CI `ssq-ci` run on `tstapler/go-git`); a soak window after deploy to compare Pyroscope 30-min windows (deploy needs `make install-service`, which restarts the live service — owner go-ahead, and only after a scripted preflight confirms the installed unit passes `--tmux-keep-server`, plan Task 5.2.2a); per-unit A/B soak windows on the manual instances. (The live-binary-vs-branch check is done: see Baseline.)

## Constraints
- **go-git only. Never native git** — no `exec`/shell-out, and none suggested as a fix or fallback. If go-git lacks a primitive, extend the `tstapler/go-git` fork (`ssq/v5`, consumed via the root `go.mod` `replace`; see `docs/how-to/roll-back-go-git-fork.md` and ADR-002 of the go-git-fork project, `project_plans/go-git-fork-full-git-replacement/decisions/ADR-002-consume-fork-via-replace-directive.md`; not this project's ADR-002). Recorded in `session/unfinished/AGENTS.md`.
- **Native-git retirement covers the whole `session/unfinished/**` test tree**, including `gogitstore`, with one exact-path allow-list entry for a test that re-execs its own binary. Ordinary developer VCS workflow commands (read-only inspection and commit workflow: `git status`/`diff`/`log`/`show`, staging, committing, opening a PR) are not product, test, benchmark, fixture, CI or acceptance-check use of native git.
- Fork patches stay small and additive (new functions, not changed behaviour); one tag `vX.Y.Z-ssq.N` per patch with fork CI green before the `replace` bump.
- Lock discipline: hold `entry.mu` only around go-git object access; follow `.claude/rules/instance-lock-free-reads.md` for `*Instance` reads.
- Cache hits must stay allocation-free; follow `deterministic-fast-tests` (no sleeps).
- Must pass `make ready` including the `dupl` and complexity gates.

## Non-functional Requirements
- **Performance SLO**: the Success Metrics above; no scan may hold a repo-wide lock across a stat walk.
- **Scalability**: tens of worktrees across several repos, histories up to ≥ 50k commits (the existing `maxReachableSetCommits`), indexes up to tens of thousands of tracked files.
- **Security classification**: internal (local developer tool).
- **Data residency**: no special requirements.

## Scope
### In Scope
- Per-repo shared caches keyed by (git common dir, hash): reachable set, commit-parent memo, ahead/behind inputs, HEAD tree hashes, gitignore matcher where valid across worktrees.
- Commit-graph usage and a header-only commit/parents reader (fork patch) to avoid full commit decode.
- Dirty detection that scales with changed paths (watcher-driven) instead of tracked-file count where correct.
- Better diff algorithm for shortstat (e.g. Myers) and allocation-lean data structures in the walkers.
- Scanner scheduling/coalescing across worktrees of one repo (batching shared work once per event burst).
- Memory-bound the shared caches (budgeted eviction), and measurement/regression gates (benchmarks in CI Tier 1, pprof before/after).
- Lock-free read and fill paths for the shared repo state (chunked pointer-free arena, per-row atomic state, generation pointer; plan ADR-009 and Epic 2.3, the latter conditional on the S1 mutex profile).
- The pure-move split of `gogit_vcs_reader.go` and of `scanner.go`'s watcher code into per-concern files (plan Story 0.0.2), so parallel stories do not collide.
- New supporting packages: `tools/scanner-workload/` (go-git-only corpus builder and scripted event driver, plan Story 0.1.5) and `session/unfinished/internal/testrepo/` (shared go-git-only test fixtures, plan Story 0.0.3).

### Out of Scope
- Any native-git path, including as a benchmark baseline for new work.
- Non-scanner packages (session lifecycle, tmux, UI) except where a call site must change.
- The system-level load causes (Chrome memory, parallel builds, CrowdStrike, tmux-gate mutex contention) — see "Adjacent load causes" below; not yet tracked, owner to open a backlog item.
- Upstreaming fork patches to go-git/go-git.

### Adjacent load causes (out of scope; untouched by this project)
- Chrome memory and CPU, parallel builds, and CrowdStrike scanning: system-level, outside the repo.
- `session/tmux` mutex contention: the relayed live mutex profile (INFERRED, ADR-009 context) shows `runGatedWith`, `listSessionsRaw`, `DoesSessionExist` and `GetPanePID` as the top own-package mutex delay, with no `session/unfinished` frame in the top 14. This project does not change that code; Task 0.1.1e re-measures on S1.
- Tracking: **not yet tracked — owner to open a backlog item** for each; no link exists today and none is invented here.

## Rabbit Holes
- Correctness of cross-worktree sharing: worktrees share objects and refs but have their own index, HEAD, and `info/exclude`; the shared-key boundary must be exactly what is truly shared.
- Concurrency and invalidation of a repo-wide cache under many scanner workers; eviction under the memory budget.
- Whether repos in use actually carry a commit-graph file (UNVERIFIED); if not, the fork must build or ignore it.
- Watcher-driven dirty tracking must not miss changes (fsnotify event loss, overflow) — needs a safe fallback to a full walk.
- Result parity of a new diff algorithm with the current LCS-based counts (insertions/deletions semantics).
- Fork rebase burden grows with each patch.

## Alternatives Considered
- Native `git` subprocesses — rejected by constraint (see above).
- Per-call micro-optimizations only (pools, preallocation) — already partly done; does not remove the W× duplication or the full-commit-decode cost.
- Raising the scan interval / lowering worker count — reduces cost but also freshness; kept as a tuning knob, not the fix.
- Replacing go-git with another pure-Go library — unproven, higher risk than extending the owned fork.

## Feasibility Risks
- Live binary may not include recent scanner fixes, making baseline numbers stale.
- Benchmarks are noisy at machine load 100+; gates need CPU-time/alloc metrics rather than wall-clock.
- Shared caches raise peak memory if unbounded.

## Observability Requirements
Expose shared-cache hit/miss/eviction counters and cache bytes **per cache** (not per repo: a repo label is unbounded cardinality; per-repo detail goes to a debug log line and a `repostates` gauge) via the existing `metrics.go` and actuator endpoints, plus a sampled shadow-verify parity counter (`parity_mismatch_total`); log scan-cycle duration and decoded-commit counts at debug; keep Pyroscope (`process_cpu`, `memory`) as the before/after comparison source. No new alerts.

## Risk Control
Feature flags at **rollback-unit granularity: six flags** (`shared_commit_state`, `diff_counter`, `scan_skip`, `watch_worktree_gitdirs`, `dirty_watch_tierb`, `best_merge_base`; plan ADR-008), because caches that share interned ids cannot revert independently; each default flips on after its own A/B, off-switch via config (restart-only) so each change can be reverted without a deploy, and every flag carries a `RemoveBy` expiry; each fork patch is revertable by rolling the `replace` back one tag; staged rollout: benchmarks → manual-instance soak on `:62871` (never the live `:8543`) → live deploy with owner go-ahead.

## Open Questions
Resolved by Phase 2 research (2026-10-15):
- Commit-graph presence: 1 of 10 repos under `~/code/github.com/tstapler/` has one (stapler-squad, v1, written Oct 8); go-git never loads it. Design must not depend on its presence (research/stack.md).
- Live binary: built from `39fa33a11` (vcs.modified=true), service restarted 2026-10-15 13:55. It contains #1038/#1039 but NOT `c46c69faf`/`150168a4c` (the fsnotify filter commits on this unmerged branch). The profile in Baseline was taken before that restart, so Phase 5 must start with a fresh baseline from Pyroscope for the post-13:55 process.
- Shareable keys: commit-derived data (reachable set, parents memo, merge base, HEAD tree hashes) is safe by (canonical common dir, hash); the gitignore matcher is per-worktree except the `info/exclude` + global layer (research/features.md, architecture.md).

Unresolved after Phase 2 research (the plan tracks these and later ones as Q1-Q14 in its Unresolved Questions; the first two below are Q1 and Q2, the third is settled by a Phase 5 gate):
- Are the proposed numeric targets acceptable, or should any be tighter, and is the early-stop threshold X = 20% acceptable? (owner: Tyler; Q1)
- Tier B (recursive working-tree watching, risky on macOS kqueue) as an opt-in, or Tier A only (watch linked-worktree gitdirs)? Plan defaults to Tier A. (owner: Tyler)
- Does an in-process index (no commit-graph dependency) meet the CPU target on its own? Settled by the Phase 5 benchmark gate, not by argument.
