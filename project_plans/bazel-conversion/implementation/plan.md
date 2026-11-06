# Implementation Plan: bazel-conversion

**Feature**: Make Bazel (bzlmod, 9.x) the build of record for Go, proto, ent, tmux/tymuxd and the web-app; `make` targets become thin wrappers; CI runs Bazel with affected-target selection; agents get a first-class Bazel workflow.
**Date**: 2026-11-01
**Status**: Epic 0 is NOT sealed (Story 0.14 not passed; see `epic0-seal.md`). State at 2026-11-06 on `sdd/bazel-epic0`: `spike-results.yaml` has 16 verdict rows; `python3 tools/bzl/check-spike-results.py` reports `RESULT: FAIL` only because the S6 row (Story 0.10, worktree cost) is missing and pending measurement. S10 and S10-redaction have no verdict and are WAIVED in `owner-attestations.yaml` `spike_waivers` (remote cache deferred; disk cache first; waiver expires 2027-02-04, still `needs-owner-reconfirm`). S13 moved from `Fail` to `FallbackWorkaround` (scratch-repo experiment ran 2026-11-04). S14 is `Fail` and the owner chose "Full e2e run" (2026-11-05), so ADR-009 is Accepted with that outcome, Epic 12 is not released and T9f is N/A. ADR-002, ADR-003 and ADR-007 are Accepted (Task 0.14d); ADR-004 is Accepted for the owner decision while its reachability, read-only enforcement and redaction parts stay Proposed (S10 waived); ADR-010 stays Proposed (measured outcome and predicted-versus-measured table recorded in the ADR; Epic 13 partly executed, `go run ./tools/bzl/scoping t9` prints T9a-T9e PASS). Epics 1-11 remain gated by Story 0.14. Reviews: review iterations 1 and 2, Phase 4 (pre-mortem, consistency review, validation), triad iterations 1 and 2; see the disposition sections at the end.
**Sequencing fact**: written at plan time, when `baselines.md`, `targets.yaml`, `spike-results.md`, `spike-results.yaml`, `owner-attestations.yaml`, `soak-ledger.yaml` and the `tools/bzl/**` helpers did not exist. As of 2026-11-06 most of them exist on `sdd/bazel-epic0` (`batches.json` and parts of Epics 1-12 do not); several are Go programs rather than the shell or Python scripts named in task text, see "Closeout reconciliation (2026-11-06)" at the end for the name mapping. The wrapper file `tools/bazel` and the helper directory `tools/bzl/` are distinct paths on purpose: Bazelisk runs the executable file `tools/bazel`, and every helper, package and label lives under `tools/bzl/` (`//tools/bzl/...`).
**Counts**: 14 epics, 88 stories, 270 tasks; raw 19910k tokens, 21.92M CU before verification, 38.36M CU after the x1.75 verification multiplier (recomputed by script after the ADR-010 additions: +1 epic, +7 stories, +28 tasks, namely Epic 13 with Stories 13.1-13.7; the previous total, after the ADR-009 additions, was 13 epics, 81 stories, 241 tasks, 17645k raw, 19.43M CU, 34.00M CU after verification) [follow-up F6-F11 pass then added +1 task, Task 1.5b, and its raw/CU delta (+55k raw, +0.06M CU, taken from the pass's own recount); recount by script before relying on the last digit]
**ADRs**: ADR-001 (adopt Bazel, supersedes ci-speed ADR-001), ADR-002 (generated code + sync-gen; Accepted), ADR-003 (Bun-owned web build; Accepted), ADR-004 (bazel-remote on onyx; remote-tier parts Proposed, S10 waived), ADR-005 (permanent exceptions, Windows cross-compile only), ADR-006 (pure-Go sqlite, darwin host cc), ADR-007 (embed staging seam; Accepted), ADR-008 (rule-repo fork policy), ADR-009 (Accepted 2026-11-05 with the "Full e2e run" outcome after S14 `Fail`: no e2e selection; the directive applies to the Go, Jest and integration layers; the ADR-005 amendment is not applied), ADR-010 (Proposed: behaviour-preserving package scoping refactors chosen by measured payoff, Epic 13; adds target T9; measured outcome recorded in the ADR) in `project_plans/bazel-conversion/decisions/`.

**Corrections to research applied in this plan**: (1) `research/architecture.md` says Windows "owner decision: dropped"; the actual decision is **cross-compile only** (Epic 6, ADR-005). (2) cgo is resolved: production uses `modernc.org/sqlite`; `mattn/go-sqlite3` is imported only by two test files; real cgo is three darwin files (ADR-006). (3) Scale numbers differ between research files (3,373 tracked vs ~3,743 untracked-inclusive `.go` files); Task 0.1d recounts them. (4) Review repair: every Bazel-owned BUILD file lives in a tracked, non-ignored, non-submodule path (`tools/bzl/tmux`, `tools/bzl/flags`, `tools/bzl/windows`, `tools/bzl/ent`; checked by `tools/bzl/check-build-paths.sh`), never under the `third_party/tmux` gitlink or the gitignored `build/`. (5) Every Bazel behaviour in `architecture.md` is INFERRED (never run); only the go-git `replace` resolution through `go_deps.from_file` is VERIFIED by spike (Bazel 9.3.1, rules_go 0.65.0, gazelle 0.54.0). Epic 0 exists to convert the rest. (6) Flag-name policy (Phase 4): every Bazel flag name the pre-mortem marked UNVERIFIED (remote-cache circuit breaker, `--remote_timeout`/`--remote_retries`, `--remote_download_toplevel`/`minimal`, disk-cache GC, `--incompatible_enable_proto_toolchain_resolution`) is a spike-verified item, not an assumption: Task 0.2b records the `bazel help` line for each on 9.3.1 in `spike-results.md` and no unverified flag may appear in a committed `.bazelrc` (checked by `check-bazelrc.sh`, Task 0.2g, which also rejects every flag absent from `bazel help <command> --long` on the pinned version). Four flags are VERIFIED ABSENT on 9.3.1 by run (`research/bazel-docs-best-practices.md` sections 2.3-2.5) and must never appear: `--local_ram_resources`, `--local_cpu_resources`, `--incompatible_sandbox_hermetic_tmp`, `--experimental_guard_against_concurrent_changes`. Labels from that research are binding: VERIFIED-IN-DOCS and VERIFIED-BY-RUN items are requirements; INFERRED items are spike checks (experiments E1-E11 in Story 0.10 and Task 0.12f), never assumptions. The repository cache and the extracted-repo contents cache are already shared per user by default (VERIFIED by run), so the committed `~/.cache/stapler-squad-bazel/repo` paths are an owner-chosen location, not a requirement for sharing; they must be set together (Task 0.2b). (7) Host fact (VERIFIED 2026-11-01, `hostname` prints `onyx`): the dev box, the live service on `:8543`, all agent worktrees and the future `bazel-remote` are one 24-core, 61 GB host, so containment of Bazel (Task 0.2f) is a prerequisite, not a polish item.

---

## Docs-derived corrections (from `research/bazel-docs-best-practices.md`, 2026-11-01)

Source labels: `[DOC]` quoted from bazel.build, `[RUN]` run on Bazel 9.3.1 on this host, `[INF]` inferred. `[DOC]`/`[RUN]` items below are plan requirements or corrections; `[INF]` items are spike checks. The full per-delta disposition is the table "Docs-derived disposition" at the end of this file.

| Changed task | What changed | Delta |
|---|---|---|
| Task 0.2b | Bug fix: `startup:<config>` lines are ignored, so every startup flag is unconditional (`startup --max_idle_secs=300`, heap, `--shutdown_on_low_sys_mem`, `--io_nice_level`, `--batch_cpu_scheduling`); `--local_resources` flag names; disk-cache GC with `--experimental_disk_cache_gc_idle_delay=10s`; `--repo_contents_cache` set with the repository cache; `--remote_local_fallback` dropped, circuit-breaker strategy flag and compression added to the cache configs; `--remote_download_toplevel` kept as an explicit default; `--config=agent` presentation-only; `spike-verified` list grows to 20 flags plus 4 recorded-absent flags | D1 D2 D3 D4 D6 D7 D8 D10 D11 D14 D20 |
| Task 0.2a | `.bazelignore` and `.gitignore` gain the in-worktree output-base directory `.bazel-out` | D15 |
| Task 0.2f | Wrapper passes `--host_jvm_args=-Xmx2g` and, for agent callers, `--max_idle_secs=30` on the command line (not in a config); absolute `--output_base` fallback mode; startup resource options; queueing rationale (no cross-server action dedup); mixed-caller server-restart check | D1 D12 D13 D14 D15 D23 |
| Task 0.2g | `check-bazelrc.sh` rejects `startup:<non-platform-config>`, any flag absent from `bazel help` on the pinned version, the four known-absent flags, `--digest_function`, `--action_env=PATH`, `--profile=` in a committed rc, and a no-op `--remote_local_fallback`; fixture test E9 | D1 D20 D21 |
| Story 0.2 | New AC: startup flags are observed on the running server's command line | D1 |
| Story 0.9, Task 0.9b, Task 4.1b | `--incompatible_sandbox_hermetic_tmp` does not exist on 9.3.1; the short-private-`/tmp` mechanism becomes a spike check | X1 |
| Story 0.10, Tasks 0.10a, 0.10c | Same-filesystem and reflink precondition, extracted-repo sharing, no-dedup CPU multiple, execution-log tooling | D3 D5 D15 D18 D23 |
| Task 0.10b, Task 0.10d, Task 11.2a | Merge driver starts from Bazel's documented jq-based driver (exact text TO-VERIFY from the docs page) | D19 |
| Tasks 0.10e, 0.10f, 0.10g (new) | Experiments E1-E7, E10, E11 with falsifiable pass criteria | E1-E7 E10 E11 |
| Task 0.12c, Task 0.12f (new), Story 0.12 | Fallback policy is timeout + retries + circuit breaker (the fallback flag is a no-op for a cache-only setup without the incompatible flag); disk + remote layering experiment E8 | D6 D7 E8 |
| ADR-004 | Local tier, failure policy, download policy and non-hermetic list updated | D2 D4 D6 D7 D8 D10 D22 |
| Fallback matrix rows S6 and S10 | PASS iff gains the cross-worktree sharing checks E1-E6 and E8 | E1-E6 E8 |
| Task 1.3b, Task 7.4c | No `--profile=<path>` in default configs; CI uses it per job only (ephemeral runners) | D21 |
| Task 1.6c (checks 3 and 7) | Doctor warns on cache/output-base filesystem mismatch and on `~/.bazelrc` heap or `/etc/bazel.bazelrc` | D3 D12 |
| Task 7.1a, Task 7.4a, Task 7.4c, Task 7.7a | Eviction retries change the invocation id; compression; breaker flags; execution-log tool choice | D6 D7 D8 D9 D10 D18 |
| Task 0.7b, Task 3.3a | Host-cc actions also protected against local disk-cache poisoning across an OS upgrade | D22 |
| Story 11.4, Tasks 11.4a, 11.4c | Orphan procedure kills the server via `server/server.pid.txt` (workspace gone, so no `bazel shutdown`); install-base report; GC settings | D2 D16 D17 |

---

## Alternatives exploration (Step 0.5)

| Approach | Key strength | Key weakness | Verdict |
|---|---|---|---|
| **A. Spike-gated strangler**: Bazel graph grown stage by stage (Go, codegen, embeds, tests, macOS, Windows), non-blocking CI beside make, parity gate P1-P10, wrappers, install-service last | Every stage is independently revertible; unknowns are retired before they can block; matches the owner's end state | Longest calendar chain because of soak and device blockers | **Chosen** |
| B. Big-bang conversion on one branch | Shortest path if everything works | 3,700 files of BUILD churn with no parity evidence; one failed rule (ent, embed, bun) blocks the whole branch; no rollback short of abandoning the branch | Rejected |
| C. Bazel as a cache/orchestrator around existing make and `go build` (genrule per make target) | Cheap, no rule risk | No target graph, so no affected-target selection and no hermetic fresh-clone build; fails two of three drivers | Rejected |

Within approach A, the sequence follows the owner's strangler path: foundations/spikes, Go-only non-blocking CI, codegen, embeds, test-sandbox remediation, macOS, Windows, CI cutover, wrappers, install-service, then make retention and removal.

---

## Domain Glossary
*(Names are used verbatim in file names, Starlark symbols, scripts, `.bazelrc` configs and docs.)*

| Term | Definition | Notes |
|---|---|---|
| `BuildSystemOfRecord` | The system whose output is shipped and whose result is authoritative: `make` before cutover, `bazel` after | Selected by `BUILD_SYSTEM` |
| `BUILD_SYSTEM` | Make variable, values `make` or `bazel`, choosing the recipe body in each wrapper target | Default `make` until Story 8.6; skeleton and validation land early in Story 1.5 |
| `StranglerStage` | One of: Foundation, GoOnly, Codegen, Embeds, TestParity, macOS, WindowsCross, CICutover, Wrappers, Deploy, Removal | Maps to Epics 0-10 |
| `Spike` | Throwaway-branch experiment with a recorded `SpikeVerdict` and a cap on agent runs | Prose in `implementation/spike-results.md`; verdicts in machine-readable `implementation/spike-results.yaml` |
| `SpikeVerdict` | Sum type: `Pass`, `FallbackWorkaround`, `FallbackFork`, `Fail` | `Fail` blocks dependent epics and escalates to owner; validated by `tools/bzl/check-spike-results.py` (Task 0.14a) |
| `FallbackPath` | The decided alternative for a spike item: `Workaround` (repo-owned Starlark/script) or `Fork` (pinned fork of a rules repo, ADR-008) | Costed before choosing |
| `GenSurface` | The three gitignored generated trees: `gen/`, `web-app/src/gen/`, `session/ent/*` (non-schema) | Never committed |
| `SyncGen` | `bazel run //:sync-gen`, wrapped by `make sync`; copies Bazel outputs into the `GenSurface` for `go`/gopls/lint | Bazel never reads its output |
| `GenFreshness` | Hash check that the synced `GenSurface` equals `bazel-bin` output | Human/IDE protection only |
| `GazelleFreshness` | CI/pre-commit gate: `bazel run //:gazelle -- -mode=diff` exits 0 | Also `bazel mod tidy` diff |
| `HandBuildFile` | A BUILD file Gazelle cannot own (proto, ent, web, tmux, embed staging) marked with a header comment and `# gazelle:ignore` | About 15 files, listed in `tools/bzl/hand-build-files.txt` (the 12 above plus `tools/bzl/BUILD.bazel`, `tools/bzl/bun/BUILD.bazel`, `tools/gazelle/BUILD.bazel`); each path must be git-tracked, not gitignored, not under a submodule gitlink and not in `.bazelignore` (`check-build-paths.sh`) |
| `EmbedStaging` | Rule that places an artifact at the relative path a `//go:embed` expects (ADR-007) | `copy_directory`/`copy_file` |
| `TagConfig` | A `.bazelrc` `--config` that sets rules_go tags or race: `embed_tmux`, `embed_tymux`, `sinkguard`, `gitoracle`, `integration`, `race`, `release` | Tags are global per invocation |
| `AffectedSet` | Test/build targets selected from a git diff by `tools/ci/affected-targets.sh` | `rdeps` plus `GlobalInputs` |
| `GlobalInputs` | Files whose change forces `//...`: `MODULE.bazel`, `.bazelrc*`, `.bazelversion`, `tools/bzl/**`, `go.mod`, `go.sum` | Single list, one file |
| `CacheTier` | `Disk` (local shared), `Remote` (bazel-remote on onyx) | PRs reach only a server-enforced read-only endpoint; the write endpoint is reachable only from `main` push jobs (ADR-004) |
| `OutputBase` | Per-workspace Bazel state directory; one per worktree | Orphans cleaned by `worktree-disk-cleanup` |
| `ParityGate` | The set of checks P1-P10 that must pass on one commit before cutover | Story 7.5 |
| `ExclusionsFile` | `tools/bzl/test-exclusions.yaml`: tests that differ between go test and Bazel, each with owner, reason, tracking issue | Ratchet only shrinks |
| `SandboxClass` | Failure class of a test under Bazel: `cwd`, `home`, `tmpdir-socket`, `exec-host-binary`, `network`, `runtime-caller`, `shard-timeout` | Used to batch fixes |
| `reporoot.Find()` | Stdlib-only leaf package `internal/reporoot` returning the repo root under both `go test` and Bazel runfiles; env-default helpers go in the equally stdlib-only `internal/testenv` | Task 4.1a. Not in `testutil`: VERIFIED `go list -deps ./testutil` includes `config`, `session`, `session/tmux`, `executor`, so a `package config` or `package session` test importing `testutil` is an import cycle |
| `AgentConfig` | `--config=agent` in `.bazelrc`: quiet, terse, bounded resources | Auto-selected by wrappers when stdout is not a TTY |
| `FallbackCanary` | Scheduled and path-triggered workflow proving `BUILD_SYSTEM=make` still passes `make ready`, builds, and passes an e2e shard, on a runner with no Bazel installed | Prevents silent rot |
| `PermanentException` | An item that stays outside Bazel by ADR-005 | List is closed |
| `ScopingTier` | Tier 0 = test-only layering inversions moved out (Story 13.2); Tier A = purely mechanical clusters of files that do not depend on the rest of their package or that nothing in it depends on (Story 13.3); Tier B = groups whose only inbound references from the rest of the package are at most 10 declarations to relocate first (Stories 13.4, 13.5); Tier C = seams needing interface extraction (out of plan) | ADR-010 |
| `UniversalSlicePackage` | A package assumed to sit in every e2e spec slice (`session`, `server/services`, `server`, `config`, root) until the Story 12.1 manifest measures it | Used by T9b; INFERRED until S14 |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Migration sequencing | Strangler Fig | Fowler (strangler application) | Big-bang rewrite | Each stage revertible, parity evidence per stage |
| Makefile wrappers | Facade over two implementations with a runtime Strategy (`BUILD_SYSTEM`) | GoF Facade, Strategy | Delete make recipes at once | One-release-cycle rollback; agents keep stable names |
| Generated code | Single producer (Bazel action) + Adapter to the source tree (`sync-gen`) | GoF Adapter | Commit generated code; Bazel reads synced tree | Repo rule; avoids worktree drift (ADR-002) |
| Embed inputs | Staging adapter at the expected path | GoF Adapter | Change `//go:embed` paths | Zero Go source change, one-PR revert (ADR-007) |
| go_test defaults | Template macro attached by Gazelle `map_kind` | Template Method | Per-target hand edits | One place for size/timeout/shard tables |
| Affected selection | Pure function `changed files -> targets` with `GlobalInputs` short-circuit | Functional core | Hand-maintained regex, script and `paths:` filters | One source of truth, unit-testable fixtures |
| Spike outcomes | Sum type `SpikeVerdict` with pre-decided `FallbackPath` | type-driven-design | Free-form "we'll see" | No spike can end undecided |
| Lint gates | Anti-corruption layer: wrapped tools consume `SyncGen` output; `nogo` for pure analyzers | Evans (ACL) | Port everything to Bazel | golangci-lint and diff gates need a normal tree and git history |
| Test environment fixes | Fix once at the source (`reporoot.Find()`, env defaults), not per test | Pragmatic Programmer (DRY) | Tag everything `local`/`no-sandbox` | Repo policy: fix flakes, do not defer |
| Starlark surface | Bounded: macros of at most about 30 lines, repo-owned `.bzl` capped at 750 lines | build-vs-buy section 3 | Custom Gazelle extension / repository rules by default | Custom Gazelle code fails silently |
| Enumerated configs/modes | Sealed set of `TagConfig` names and `BUILD_SYSTEM` values, validated by a test | type-driven-design | Free strings | Prevents unknown-config drift |

---

## Improvement targets, post-cutover tooling status and decision gates (triad repair, iteration 1)

**Improvement targets (required, not informational).** P10 keeps its non-regression bounds, and the improvement rows below are added to P10 (Story 7.5). Every numeric value is a formula over `baselines.md`; the figure shown is a proposal, and the final value is **set at Story 0.1 (Task 0.1g) from baseline, owner-confirmed** and stored in `tools/bzl/baseline/targets.yaml`. Improvement rows cannot be waived; the owner may re-target one with a recorded reason (Task 7.5f).

| Id | Driver | Measured quantity (collector) | Proposal formula | Baseline source |
|---|---|---|---|---|
| T1 | CI speed | `affected_target_count / total_target_count`, median over the last 30 partial-change PRs (no `GlobalInputs` touched), replayed by `affected-targets.sh` (Tasks 7.3c, 7.5g) | at most 0.30 (or 1.5 x the measured median reverse-dependency fan-out of those diffs, whichever is smaller) | Task 0.1f partial-change PR set |
| T2 | CI speed | CI minutes billed and time-to-green p50 on partial-change PRs, Bazel jobs vs the same PR class under make | runner-minutes p50 at most 0.70 x baseline and time-to-green p50 at most 0.90 x baseline for the partial-change class; whole-population check: over all PRs of the same window (partial and `GlobalInputs` PRs together) time-to-green p50 at most 1.00 x baseline and runner-minutes p50 at most 0.90 x baseline, so a gain on easy PRs cannot hide a loss on the rest | `ci-time-to-green.sh` per-class output joined with `ci-billed-minutes.sh` (Task 0.1i) |
| T2b | CI speed | warm remote-cache hit ratio: median over the last 20 warm `main` push runs of `hit_spawns / total_spawns` (hit = `runner` of `remote cache hit` or `disk cache hit`, definition of Task 0.10c), from the same runs as P10's cache-hit floor | at least 0.60 (the P10 floor; absolute, because `make` has no remote cache, so no baseline ratio exists; the target is owner-confirmed with the others) | `bazel-trend.sh` (Task 7.1a); cache-outage runs excluded as under T2 |
| T3 | Worktree cost | second-worktree cold build with the shared cache vs the first-worktree Bazel cold build, and per-worktree disk | second cold build median at most 25% of the first (in addition to the 30%-of-make bound); per-worktree `output_base_disk_kb` at most 1.0 x baseline `disk_kb_worktree` (the 1.25 x ceiling stays as the waivable floor); **improvement clause**: the 5-worktree aggregate (all output bases plus the shared disk and repository caches counted once) at most 0.80 x 5 x baseline `disk_kb_worktree`, measured by Task 0.10a, so sharing must pay back disk and not only match it | `baseline-worktree.json` |
| T4 | Agent experience | edit-build-test latency: edit one non-test `.go` file in `config/`, run `go test ./config` warm, median of 5 | at most max(1.25 x baseline warm one-package `go test`, baseline + 5 s) | Task 0.1f warm one-package latency |
| T5 | Agent experience | first-error legibility: 9 seeded failure fixtures (stale BUILD, missing external dep, missing generated code, OOM, ENOSPC, cache unreachable, flock queue wait, sandbox failure, cache auth rejected) | 100% print a `Remedy:` line within the first 20 lines and at most 60 lines in total (Task 1.6d) | Task 0.1f: same fixtures under make |
| T6 | Agent experience | manual regen-codegen steps (`make proto-gen`, `make ent-gen`, `make sync` run only to unblock a build) per agent session, from transcripts | at most 10% of baseline median per session after the flip (reported at Story 7.6, enforced at Story 10.1) | Task 0.1f transcript scan |
| T7 | Hermeticity | hermeticity before/after (collector `tools/bzl/baseline/hermeticity.sh`, Tasks 0.1j and 7.5g): **H1** distinct host executables outside the workspace and the pinned toolchain dirs, and distinct non-loopback network destinations, seen by `strace -f -e trace=execve,connect` during a fresh-clone build; **H2** fresh-clone build success with `PATH` scrubbed to `env -i PATH=/usr/bin:/bin`; **H3** actions that fail under a strict hermetic sandbox (undeclared inputs), Bazel only; **H4** `h_downgrades`, the number of rows in `tools/bzl/hermetic-downgrades.txt` | H1 host executables at most 0.50 x the make baseline (allow-listed Bazel prerequisites excluded) and 0 network destinations during build actions (fetches only inside listed repository rules); H2 true; H3 equals 0; H4 equals the number of D-HERMETIC downgrades the owner accepted in `owner-attestations.yaml` (so a downgrade is visible, counted against T7 and never silent; an unlisted host-tool action fails T7) | Task 0.1j make-path numbers |
| T8 | CI speed (e2e) | e2e work avoided: e2e runner-minutes (sum of Playwright shard job durations) per partial-change PR, selective Bazel e2e vs the full suite, plus `e2e_escapes` (a spec that fails on `main`/nightly and was not selected on the PR that introduced the failure; collector `tools/bzl/parity/e2e-avoided.sh`, Tasks 12.4d and 7.5g) | at most 0.50 x the baseline median e2e runner-minutes of the 30 sampled partial-change PRs (two sampled PR runs give about 113 runner-minutes, 1030-1949 s per shard, five shards; `ci-billed-minutes.sh` computes the real median) and `e2e_escapes` equals 0 over the last N soak runs; not waivable | Task 0.1i partial-change PR set, restricted to the `e2e (Bun build) shard` jobs; Story 0.15 (`e2e_selected_share_median`) |
| T9 | Package scoping (cost of invalidation) | cost-weighted footprint of the package layout, measured by the replay harness (`tools/bzl/scoping/replay.py`, Tasks 13.1b and 13.1f, collector `tools/bzl/parity/scoping.sh`) over the last 500 and 100 `main` commits and the 30 sampled PRs: **T9a** mean affected test-LOC share, **T9b** share of Go production commits touching a package assumed to be in every e2e slice, **T9c** mean affected library-LOC share and share of changes affecting less than baseline, **T9d** LOC of the two largest packages and the number of test-only layering inversions, **T9e** guard: T1 recomputed on the Bazel target model (library and test edges separate), **T9f** median e2e slice size per spec once the Story 12.1 manifest exists (N/A by owner decision 2026-11-05: e2e stays a full run) | baseline 55.4% / 71% and 70% / 40.5% / 93,118 and 92,248 LOC and 12 inversions / 0.053; proposals: T9a at most 46.0% after Story 13.2 and 45.0% at close; T9b at most 64% (last 100) and 61% (last 500) at close; T9c at most 39.5% and at least 12%; T9d each of the two packages at most 72,000 LOC (90,000 without Tier B) and 0 inversions; T9e at most 0.30 at every story; T9f at most baseline minus 0.8 x the harness-predicted reduction. Values fixed at Task 13.1f, owner-confirmed; **not part of the Story 7.6 flip gate** (enforced at Story 13.7); not waivable once confirmed | Task 13.1b writes `tools/bzl/scoping/baseline.json`; ADR-010 and `research/package-scoping.md` hold the 2026-11-02 values |

**Sampling, proxies and risky assumptions (triad iteration 2).**
- *Sampling of the T1/T2 partial-change set.* Task 0.1i takes the 100 most recent merged PRs, classifies each `partial` (touches no `GlobalInputs` file) or `global`, and uses the 30 most recent `partial` PRs. Representativeness check: the sample's median changed-file count is within 2x of the 100-PR population's and the share of PRs touching each top-level area (`server/`, `session/`, `web-app/`, other) is within 20 percentage points of the population's; on failure the sample is re-drawn stratified by top-level area and the report says `restratified: true`.
- *Cache-outage runs (T2, T2b).* A run during which the cache was unreachable (circuit breaker tripped, or `--remote_local_fallback` path taken, per the build event log) is flagged `cache_outage: true` by `ci-time-to-green.sh` and `bazel-trend.sh` and excluded from the T2 and T2b medians; the report prints how many runs were excluded, and more than 20% excluded makes T2 `UNMEASURED` (a FAIL at the gate), so an outage cannot hide a regression nor be counted as a Bazel slowdown.
- *When baselines and targets are produced.* `requirements.md` says baselines come in SDD Phase 2 and targets in Phase 3; in this plan baselines are produced by Story 0.1 (Tasks 0.1a-0.1j) and targets by Task 0.1g, both in Epic 0, so measured numbers exist before any Epic 1 work. This plan governs where they differ.
- *T1 is a proxy.* The affected-target ratio measures work avoided, not time saved; T2 is the outcome and must be read with it. T2 needs billed or runner minutes joined by PR class (Task 0.1i): the repository is public, so GitHub bills 0 and the join uses job-duration runner-minutes weighted by OS multiplier (Linux 1, macOS 10, Windows 2), labelled as such.
- *Risky assumption RA1: reverse-dependency fan-out is narrow* (most partial-change diffs reach at most 30% of the test targets). *RA2: time-to-green is not dominated by the parts Bazel does not touch* (runner setup, lint, and, before ADR-009, e2e). **Measured 2026-11-02: RA2 = 0.987** (e2e median 1974 s, runner queue/setup 398 s, lint 0 s, other 34 s of a 2601 s median span; `baselines.md`), so it fails the 0.60 threshold; that measurement is the reason for Epic 12 and ADR-009. After ADR-009, RA2 and EG5 apply to the share that remains after e2e selection: the projected critical path is `setup_and_queue + lint + go_or_other + e2e_median x s`, where `s` is the median selected-spec share measured by spike S14 (Story 0.15), and EG5 is judged on that projection (Task 0.14e). Task 0.1f pre-measures both before any Bazel work, from the Go import graph and per-job timings; Task 0.14e trips EG5 if RA1 or RA2 fails.
- *Mid-point economic re-check.* Story 2.7 re-evaluates T1 (replay), T3 and T4 after Epics 1 and 2, before Epic 3 releases, as an extra decision gate (never a downscale).
- *RA1 refinement (ADR-010, 2026-11-02; annotation, the measured 0.2587 and 0.2657 stand).* RA1 merges library and test imports, so `session`'s external tests that import `server/services`, `server/auth` and `pkg/events` pull the whole upper layer into `session`'s cone (62 of 143 packages have exactly 37 reverse dependencies). On the Bazel target model (test targets are leaves) the same 17 Go-touching PRs affect a median 0.053 of all targets, not 0.266, while 32% of library LOC and 62% of test LOC are affected because the packages that change are the large ones (`research/package-scoping.md` section 1). T1's formula and bound are therefore to be evaluated on the target model (Unresolved Question "ADR-010: T1 on the target model"); the merged proxy is kept for continuity and is expected to rise after Epic 13 (0.266 to about 0.30) because splitting adds dependents to the core.

**Post-cutover status of native `go build` / `go test` / gopls (decided; ADR-002 amended).** After the flip, Bazel is the build of record and the only path that produces shipped artifacts, release binaries, parity evidence and the CI pass/fail verdict. Native `go build`, `go vet`, `go test` and gopls stay working in every worktree after `make sync` (sync-gen materialises the `GenSurface`) and are **supported for editor tooling, golangci-lint, quick local iteration and bisecting only**; a native `go test` result is not authoritative (a disagreement is resolved by the Bazel result, and the ExclusionsFile lists known differences). They are not removed in Epic 10. The inverted precondition is announced, not hidden: Bazel needs no generated code, plain `go`/gopls needs `make sync` (printed by `make help` and `make bazel-doctor`, Task 1.6c; staleness caught by the cheap freshness form, Tasks 2.4d and 8.2d).

**Economic abort / re-plan criterion.** Evaluated once at Story 0.14 by `tools/bzl/economic-gate.py` (Task 0.14e) from spike data and `targets.yaml`. It is a decision gate, never a downscale: a trip stops the release of Epics 1-11 until the owner records `continue`, `re-plan sequencing`, `pause at stage` or `revert to make-only` in `owner-attestations.yaml`, and no agent picks the option or removes scope.

**Priority rationale.** RICE scoring with every input labelled UNVERIFIED is in ADR-001 ("Priority rationale") and re-scored with measured inputs at Task 0.1g.

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `server/web/embed.go` (`all:dist`) | Embeds a gitignored tree produced by another toolchain | **Isolate via seam** (Story 3.2) | `EmbedStaging` keeps Go source untouched; fall back to Refactor-first only per ADR-007 |
| `changelog_embed.go` + `pkg/changelog` setter | Root-level embed with setter indirection | **Extend as-is** | Already Bazel-compatible (`embedsrcs = ["CHANGELOG.md"]`) |
| `session/tmux/embed/tmux`, `session/tymux/embed/tymuxd` | Untracked staging paths populated by scripts | **Isolate via seam** (Story 3.3) | Stage at path; tags remain the switch |
| Test files assuming repo-root cwd, `$HOME`, host tmux/git (49 of 154 test packages in `features.md`) | Environment-coupled tests; the six worst: `server/services`, `session`, `session/tmux`, `config`, `session/sshremote`, `server` | **Refactor-first**, incrementally and only where Bazel exposes a failure (Epic 4) | Fix once via `reporoot.Find()` and env defaults; also helps `deterministic-fast-tests` |
| `go.mod` carries `mattn/go-sqlite3` for two tests | Dead cgo dependency for production | **Refactor-first** (Task 1.1b) | Removes Linux cgo (ADR-006) |
| Hand-maintained CI selection logic: one `grep -qE` regex in `build.yml:97`, `tools/ci/detect-feature-changes.sh`, and native `paths:` filters in `mcp-integration.yml` (VERIFIED: `mcp-integration.yml` has no `grep -E`; three different mechanisms, not three regex lists) | Two-sources-of-truth correctness risk | **Refactor-first**: replaced by `AffectedSet` plus a short retained list of non-Bazel path filters (Story 7.3); legacy selection deleted with the legacy jobs in Epic 10 (Task 10.2c) | The core CI-speed driver |
| Makefile stamp-file logic (`.proto-gen.stamp`, `.ent-gen.stamp`, `LDFLAGS`) | Hand-built dependency tracking | **Replace inside wrappers** (Epics 2, 3, 8) | Bazel's action cache is the source of truth |
| `third_party/tmux` submodule (a gitlink: `git ls-tree HEAD third_party/tmux` is mode 160000) with stale in-tree `.o` files; empty in fresh worktrees | Poisons any in-tree build; breaks "fresh clone"; no file can be tracked under a gitlink | **Isolate via seam** (Story 3.3): tmux Bazel targets live at tracked `tools/bzl/tmux/`, sourced from the pinned release tarball via `http_archive`; the submodule is only a version pin and stays in `.bazelignore` and `gazelle:exclude` | Fixes the `b51b60eb1` worktree class of bug |
| `session/ent` gitignored codegen (ent emits `session/ent` plus entity and helper subpackages: VERIFIED `go list ./session/ent/... | wc -l` prints 53 Go packages, only `schema` of which is tracked), `gen/` | Gazelle cannot see it; one rule cannot emit many go_libraries without a package list | **Isolate via seam**: HandBuildFile macro `tools/bzl/ent` instantiating one `go_library` per package from a checked-in `packages.txt` guarded by a freshness test, plus `gazelle:resolve`/`resolve_regexp` (or extension per S11) | ADR-002 |
| `session/ent/generate.go` is documented in `CLAUDE.md` but VERIFIED absent (`git ls-files | grep ent/generate` is empty); the canonical `--feature sql/upsert` command exists only in the `Makefile` (`ent-gen`) | Bazel action and `make ent-gen` could diverge on the flag | **Isolate**: single source `tools/bzl/ent/ent_flags.txt` read by both the Bazel action and `make ent-gen` (Task 0.4a) | `sql/upsert` is required; a missing flag removes the `OnConflict*` upsert builders (46 vs 0, Epic 0 S2; no symbol is named `UpsertRule`) |
| `testutil` is a fat, session-aware package (imports `session`, `session/tmux`, `config`, `executor`) | Any in-repo test of those packages importing it creates an import cycle | **Isolate via seam**: new stdlib-only leaves `internal/reporoot` and `internal/testenv` (Task 4.1a); `testutil` unchanged | Arch review B3 |
| `tools/lint` as a separate module (VERIFIED 8 tracked `go.mod` files: root, `tools/lint`, `tools/scanner`, `tuitest`, 4 stray under `project_plans/**/spikes/*`) | Analyzers unreachable from the main module; `tools/lint` pins its own `golang.org/x/tools` version | **Isolate via seam**: isolated `go_deps` extension usage so the two `x/tools` versions never unify (Task 0.2c checks `bazel mod graph` shows both); `nogo` is a later story (8.4) | Keeps `x/tools` out of the app dependency universe |
| Root-level stray artifacts (`*.test`, `stapler-squad.bak-*`, `err.log`, `lint_errors_v*.log`) | Pollute globs and Gazelle's root package | **Isolate**: `.bazelignore` plus root BUILD never globs; not cleaned here | Out of scope for cleanup |
| `project_plans/**/spikes/*/go.mod` (4 stray modules) and `.claude/worktrees`, `worktrees/` | Gazelle/Bazel would walk copies of the repo | **Isolate**: `gazelle:exclude` and `.bazelignore` (Task 0.2a) | Mandatory or N worktrees leak into one graph |
| otel auto-instrumentation build | Wraps `go build` | **Replaced 2026-11-03:** the one binary is always otelc-woven; Bazel `//:stapler-squad` is the S15b `otel_weave_hermetic` action (FallbackWorkaround: sandboxed and remote-cacheable, networked module-cache fetch and host cgo remain); no longer a PermanentException (ADR-005 amended) | `otel_weave_hermetic` rule |

---

## Observability Plan
- **Logs**: every read-only CI Bazel invocation (PR jobs, no write credential) uploads `--build_event_json_file` and `--profile` as artifacts; the `main` write job uploads only the scrubbed trend JSON produced by `bazel-trend.sh`, never raw BEP or profile (ADR-004); wrappers print the exact `bazel` command they run (discoverability).
- **Metrics** (written by `tools/ci/bazel-trend.sh`, extending `tools/ci/duration-trend.sh`): `ci_time_to_green_seconds` (PR, p50/p90), `bazel_remote_cache_hit_ratio` (from BEP), `bazel_cold_build_seconds_per_worktree`, `bazel_output_base_disk_kb_per_worktree`, `gazelle_freshness_failures_total`, `affected_target_count`, `affected_selection_false_negatives_total` (parity soak). Each metric is an operation over 100ms by nature (a build); computed from BEP, not instrumented in the app.
- **Alerts**: advisory only (workflow annotation, no paging): trend regression greater than 25% on p50 CI time; any `FallbackCanary` failure; any determinism-job mismatch. Existing service metrics and dashboards are untouched (the app binary behaviour is parity item P5/P9).

## Risk Control
- **Feature flag**: `BUILD_SYSTEM` Make variable, default `make` until Story 8.6; Bazel CI job is `continue-on-error` until Story 7.6a flips jobs one at a time. VERIFIED: `main` has no branch protection and no rulesets (`gh api .../branches/main/protection` returns 404, `.../rulesets` returns `[]`), so "blocking" means "the job has no `continue-on-error`" unless the owner adds protection (decision D-PROTECT-MERGE).
- **Rollback procedure**: before Epic 10, revert the single PR that flipped `BUILD_SYSTEM`/CI job (Bazel-only source changes are banned until the retention window ends, so the revert is mechanical). `BUILD_SYSTEM=make make build` stays tested weekly by the `FallbackCanary`.
- **Staged rollout**: per job (build, tests, lint wrappers, e2e, release), per platform (Linux, macOS), install-service last, gated by P1-P10 and owner sign-off. The live service on `:8543` is never touched until P9 passes on an isolated instance (ports 62871/62872, private tmux socket).
- **Never**: commit generated code; use npm/pnpm/yarn; force-push; run `make install-service` or `bazel run //:stapler-squad` to try changes.

## Unresolved Questions
- [ ] Remote-cache reachability from GitHub runners: Tailscale ephemeral-key action (preferred) versus a TLS public path — blocks Story 7.4 (selected by spike 0.12) — owner: Tyler (approves exposure; supplies auth keys)
- [ ] Deploy `bazel-remote` on `onyx` bound to loopback, with the two proxy listeners (read-only and read-write) and Tailscale ACL tags from ADR-004; create the GitHub environment `bazel-cache-write` restricted to `main` with the owner as required reviewer (Task 0.12e) — blocks Task 0.12b and Story 7.4 — owner: Tyler
- [ ] Soak parameters N and K (rule defined in Story 7.6; proposal N=30 PR runs, K=10) — owner fixes both before Epic 7 starts, blocks Task 7.6a — owner: Tyler
- [ ] macOS device check that TCC grants persist for a Bazel-built, post-signed binary on arm64 and amd64 — blocks Task 5.2b and Story 9.1 on macOS — owner: Tyler
- [ ] D-PROTECT-WRITE (cache-write part, needed earlier, Task 0.12e): choose (a) a `main` ruleset or (b) the `bazel-cache-write` environment with deployment branch `main` AND owner required-reviewer only, with direct-push exposure accepted; the environment gate is mandatory either way — blocks Task 0.12b — owner: Tyler
- [ ] D-PROTECT-MERGE: `main` has no branch protection or rulesets (VERIFIED), so there is nothing to rename. Decide at the Story 7.6 flip whether to (a) add a ruleset with required checks (owner action; the plan lists the job names to require) or (b) keep the merge-gate convention that "blocking" means no `continue-on-error` and agents merge only on green `gh pr checks` — blocks Task 7.6a only for the choice, not for the work — owner: Tyler
- [ ] D-HERMETIC (pre-registered before Story 0.14): if S4 (hermetic Bun) or S5 (tmux via rules_foreign_cc) fails, may the plan downgrade hermeticity to a host-tool action tagged `no-remote-cache`? Pre-answering avoids a surprise at the gate — owner: Tyler
- [ ] GitHub environment approval as the integrity gate for owner attestations (`owner-attestations.yaml` is agent-writable; see Task 7.5c) — owner: Tyler configures the `bazel-owner-attest` environment with himself as required reviewer
- [ ] Cross-workflow trigger credential for Story 7.8: pushes made with the default `GITHUB_TOKEN` do not trigger other workflows, so the repair/autofix commits need either a GitHub App installation token or a fine-grained PAT (contents and pull-requests write, this repo only) stored as the secret `BAZEL_REPAIR_TOKEN` in the environment `bazel-repair`, or the `workflow_dispatch` alternative of Task 7.8a (no extra credential; the workflow dispatches `bazel.yml` on the repair branch) — blocks Tasks 7.8a and 7.8b only for the choice — owner: Tyler
- [ ] Run the live-service OOM protection drop-in of Task 0.2i (`OOMScoreAdjust=-900`, or the system-level variant if the user manager rejects negative values) before any Bazel run at scale — blocks Story 0.10 — owner: Tyler
- [ ] Install the daily orphan-output-base timer (Task 11.4b) on onyx — owner: Tyler
- [ ] Docs-derived: the owner's `$HOME/.bazelrc` sets an 8g JVM heap, which overrides the committed 2g (rc order); the wrapper forces 2g on the command line (Task 0.2f) and `make bazel-doctor` warns, but bypassing the wrapper still gets 8g per server. Owner may remove the home setting — does not block — owner: Tyler
- [ ] Docs-derived: S6 may select the in-worktree `--output_base` (`.bazel-out`) fallback; it trades orphan-proofing for scan cost and path length (Task 0.10a). Decided by the S6 verdict, owner informed — owner: Tyler
- [ ] Create `github.com/tstapler/<rules repo>` forks if any spike returns `FallbackFork` — blocks the dependent story — owner: Tyler (Task 0.14b lists which)
- [ ] goreleaser consuming Bazel binaries (`builder: prebuilt`) available? — blocks Task 3.7b — owner: agent (resolved by Task 3.7a), owner confirms exception if not
- [ ] Go SDK 1.26.6 downloadable by `go_sdk.from_file` — blocks Story 0.2 — owner: agent (Task 0.2d), fallback is the nearest published patch plus a `go.mod` `toolchain` line, owner approval required if go.mod changes
- [ ] Definition of "one release cycle" for make-fallback retention. Proposal (event-based, Story 10.1): the flip plus at least one merged release-please Release PR, at least 2 green weekly canary runs after the flip, and one owner-run `make install-service` from the Bazel binary on Linux and one on macOS recorded in `owner-attestations.yaml` — blocks Story 10.1 — owner: Tyler
- [ ] Cache hit-ratio floor and per-worktree disk bound (P10, Task 7.4c): proposal hit ratio median at least 0.60 over the last 20 warm `main` push runs and `output_base_disk_kb` at most 1.25 x baseline `disk_kb_worktree`; owner may waive either in `owner-attestations.yaml` — blocks Task 7.6a — owner: Tyler
- [ ] Storage budget for `bazel-remote` and the shared disk cache on onyx: a separate dataset or quota'd `systemd` `StateDirectory` with `--max_size` at most 100 GB (the root filesystem also holds the live service's SQLite state; VERIFIED one 1.9 TB `/` at 67% used) — blocks Task 0.12b — owner: Tyler
- [ ] Commit or ignore `MODULE.bazel.lock`: decided by spike S13 (Task 0.10d); ignoring it amends ADR-001's consequence line and needs owner approval — blocks Story 0.14 — owner: Tyler
- [ ] Confirm or amend improvement targets T1-T8 and the economic-gate factors EG1-EG5 (proposals in "Improvement targets" and Task 0.14e) after Task 0.1g computes them from baseline — blocks Story 0.14 — owner: Tyler
- [ ] Waiver policy: proposal maximum validity 90 days, reason of at least 20 characters, only the hit-ratio floor and the 1.25 x disk ceiling are waivable (Task 7.5f) — owner may tune — blocks Task 7.6a — owner: Tyler
- [ ] Queue-wait timeout for the `tools/bazel` flock slot: proposal 900 s with exit 75 (Task 1.6a E7) — owner may tune — owner: Tyler
- [ ] ADR-009 (selective e2e): accept the recommended design (observed-dependency slices, out-of-band binary, selection by `rdeps`, nightly full run, shadow before blocking) or choose differently; decided at Story 0.14 from spike S14 — blocks Epic 12 — owner: Tyler
- [ ] ADR-009: accept the residual risk that a PR can merge a regression the nightly full run catches the next cycle (bounded by the escape detector and the shadow phase), versus keeping full e2e on every PR; and set the proposal values: replay median selected share at most 0.50, p90 at most 0.90, T8 at most 0.50 x baseline e2e minutes, zero escapes over N runs — blocks Story 0.15's verdict and Task 12.6a — owner: Tyler
- [ ] ADR-009: the full e2e suite on `main` push stays blocking in effect; confirm whether `main` red from the nightly or push full run should block merging (convention, as with D-PROTECT-MERGE) — blocks Task 12.6a — owner: Tyler
- [ ] ADR-009: spend for Story 0.15 and Task 12.5c (about 30 mutation runs, about 113 runner-minutes each on the public repository's free runners; the harness bounds this with targeted runs) — owner informed, agent proceeds unless the owner objects — owner: Tyler
- [ ] Record the economic-gate decision (`continue`, `re-plan`, `pause`, `revert`) in `owner-attestations.yaml` if Task 0.14e prints `OWNER DECISION` — blocks Epics 1-11 only on a trip — owner: Tyler
- [ ] ADR-010 (package scoping): confirm the tiers and the selection criteria SC1 to SC6 with their materiality thresholds, and decide whether Tier B (Stories 13.4 and 13.5, `server/services` and `session` shrink by about 27% each) is executed, knowing that its measured gain in mean affected library-LOC share is about 2 percentage points (40.5% to 38.5%) and in test-LOC share about 12 (55.4% to 43.5%, of which 10.5 come from Tier 0) — blocks Stories 13.4 and 13.5 — owner: Tyler
- [ ] ADR-010: acceptable freeze windows for the moves. Proposal: one cluster per PR, at most 3 split PRs open, each lands within one working session after the Task 13.1e conflict check; the owner may pause backlog dispatch on a cluster's files for that window (120 worktrees are registered in this checkout) — blocks Story 13.3 — owner: Tyler
- [ ] ADR-010: T1 on the Bazel target model (library and test edges separate) instead of the RA1 merged proxy; the measured baselines are 0.053 versus 0.266. Amends the T1 formula text and the Task 0.1g/2.7a replay harness, not the stored RA1 numbers — blocks Task 13.1f — owner: Tyler
- [x] ADR-009 (2026-11-05, owner): after the S14 `Fail`, chose "Full e2e run": e2e stays a full run in CI, no e2e selection; ADR-009 Accepted with that outcome, T9f N/A (`owner-attestations.yaml` `adr_009_e2e`)
- [x] ADR-010 (2026-11-05, owner): for lower packages (packages `session` imports) SC1 counts declarations relocated into the core and SC3 does not count natural callers (`adr_010_lower_package_criteria`; `scoping criteria --lower`)
- [ ] ADR-010: confirm the T9 proposal values after Task 13.1b computes the baseline (T9a to T9f in "Improvement targets") — blocks Task 13.1f — owner: Tyler
- [ ] ADR-010: Tier C (`Instance`, `EntRepository`, `Storage`: interface extraction, `docs/architecture-audit-session-package-split-2026-09-07.md` items 3 and 4) stays out of this plan; confirm it becomes a separate project or stays deferred — does not block — owner: Tyler

## Dependency Visualization

```
E0 Spikes/Foundation (gate) ─ verdicts + ADR final ─┬──────────────────────────────────────────────────┐
  0.1 baselines ────────────────────────────────────┼──────────────► E7.5 P10 perf comparison          │
  0.2 bootstrap ─► 0.3 proto ─┐                     ▼                                                  │
                   0.4 ent ───┼─► 0.9 sandbox census ──► E4                                            │
                   0.5 embed ─┤                                                                        │
                   0.6 web ───┤        E1 Go-only (non-blocking CI) ─► E2 Codegen ─► E3 Embeds         │
                   0.7 tmux ──┘                    │                      │             │              │
                   0.8 tags, 0.10 worktree, 0.11 gazelle, 0.12 cache, 0.13 nogo           ▼              │
                                                   └──────────────► E4 Test sandbox remediation        │
                                                                          │                             │
                         E11 Agent enablement (11.1 with E1; rest by E8) │                             │
                                                                          ▼                             │
                    E5 macOS ◄── E3         E6 Windows cross ◄── E3     E7 CI cutover (P1-P10, soak) ◄──┘
                         └───────────────────┴─────────────────────────────┤
                                                                           ▼
                                                  E8 Makefile wrappers + BUILD_SYSTEM ─► E9 install-service
                                                                                              │
                                                                                              ▼
                                                                          E10 make retention ─► removal
```

**Cross-epic edges not drawn above** (all are hard dependencies): Task 2.2a <- Task 3.1a (`protoc-gen-es` comes from the Bun repository rule); Task 2.6a <- Task 3.1a (the `//...` build includes `//web-app`); Story 2.7 <- Epic 1, Epic 2 and Tasks 0.1f/0.1g/0.14e; Tasks 4.10f/4.7a/4.7b <- Tasks 4.8a and 4.9a; Task 8.2b <- Task 8.2e; Task 7.8b <- Tasks 0.2h and 0.10b (`make setup-git`); Task 10.2a <- Tasks 10.2d and 10.2c; Task 1.6a E5 hint <- Task 11.4a (script lands with Epic 1); Task 1.6a E1/E2 hints <- Task 1.5a (`make gazelle`, `make sync`, `make setup-git` skeleton targets); **Epic 12 (ADR-009)** <- Stories 3.1 and 3.4 (web dist and root binary), Stories 4.1-4.2 (test env helpers and the `go_test` macro), Story 7.3 (`affected-targets.sh`, `GlobalInputs`), Story 7.4 (read-only PR cache, `main`-only writes) and Story 0.15 (S14 verdict at Story 0.14); Task 12.6a <- Story 7.6 soak and `e2e_escapes = 0`; Task 7.5g (T8) <- Task 12.4d; Task 10.2c must not delete the legacy full e2e job before Task 12.6a; Task 0.14e (EG5) <- Story 0.15. **Epic 13 (ADR-010)** <- Story 1.1 (Gazelle BUILD files) and Story 1.2; Stories 13.4 and 13.5 <- Story 0.14 (S14 verdict), Story 12.1 (manifest, for criterion SC5) and the owner's answers to the "ADR-010" Unresolved Questions; Task 13.1f <- Task 0.1g (`targets.yaml`) and Task 7.5g (`run-all.sh` calls `scoping.sh`); Task 13.2c <- the `make ready` target list; Task 13.1d <- a pinned `gopls`; Task 13.7a <- Story 12.4 (replay, when Epic 12 exists). Epic 13 is **not** a predecessor of Story 7.6, Story 8.6, Epic 10 or Epic 12.

## Effort estimate

Basis: bands and weights INFERRED (ESTIMATION.md); recalibrate after the first implementer run. Cost class midpoints: XS 15k, S 55k, M 130k, L 240k raw tokens (no L tasks were needed; the tasks most likely to move up a class are 3.1a, 0.6d, 4.7a/b and 4.10a-c, which ADR-008 costs at L on the fork path). Tasks 4.10a-e and 4.7a/b execute as batches of at most 5 files (Task 4.10f); their class is the aggregate budget of their batches. Assumed mix per task: 12% output, 44% fresh input, 44% cache read, giving about 1.1 CU per raw token (`in + 0.1*cache_read + 5*out`). Spike tasks and doc tasks use the same mix.

| Story | Tasks | Classes | Raw tokens (k) | Output share | CU (k) | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1 Measure baselines for the three drivers | 10 | 7S, 3XS | 430 | 12% | 473 | owner confirms improvement targets (Task 0.1g) |
| 0.2 Foundation bootstrap (inert, merges to main) | 9 | 2M, 3S, 4XS | 485 | 12% | 534 | owner runs the live-service OOM drop-in (Task 0.2i) |
| 0.3 Spike S1 - proto Go + connect-go + TS through Bazel | 5 | 2M, 3S | 425 | 12% | 468 |  |
| 0.4 Spike S2 - ent generation as a Bazel action | 3 | 2M, 1XS | 275 | 12% | 303 |  |
| 0.5 Spike S3 - embed from a tree artifact | 2 | 2S | 110 | 12% | 121 |  |
| 0.6 Spike S4 - Bun web build under Bazel; fork-versus-workaround costing | 6 | 3M, 3S | 555 | 12% | 611 |  |
| 0.7 Spike S5 - tmux and tymuxd on bzlmod | 3 | 2M, 1S | 315 | 12% | 347 |  |
| 0.8 Spike S7 - rules_go flags and TagConfig composition | 2 | 2S | 110 | 12% | 121 |  |
| 0.9 Spike S8/S9 - sandbox census and unix-socket path limit | 2 | 1M, 1S | 185 | 12% | 204 |  |
| 0.10 Spike S6 - worktree cost, memory and lock-file conflicts | 8 | 2M, 6S | 590 | 12% | 649 |  |
| 0.11 Spike S11 - Gazelle behaviour on this repo | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 0.12 Spike S10 - remote cache reachability, server-enforced read-only, secret redaction | 6 | 5S, 1XS | 290 | 12% | 319 | owner deploys bazel-remote on onyx |
| 0.13 Spike S12 - custom analyzers as nogo | 1 | 1M | 130 | 12% | 143 |  |
| 0.15 Spike S14 - selective e2e through observed-dependency slices | 4 | 3M, 1S | 445 | 12% | 490 | none (about 30 full-suite runs on free runners) |
| 0.14 Spike gate - verdicts, ADR finalisation, owner sign-off | 5 | 2S, 3XS | 155 | 12% | 171 | owner reviews verdicts; creates fork repos if any; owner decision if the economic gate trips (Task 0.14e) |
| 1.1 Gazelle-generated BUILD files and the sqlite test move | 3 | 1M, 1S, 1XS | 200 | 12% | 220 |  |
| 1.2 Build and test the generated-independent subset | 2 | 1M, 1S | 185 | 12% | 204 |  |
| 1.3 Non-blocking Bazel CI job | 2 | 2S | 110 | 12% | 121 |  |
| 1.4 TagConfig set in .bazelrc | 2 | 1S, 1XS | 70 | 12% | 77 |  |
| 1.5 BUILD_SYSTEM skeleton (make stays independent of Bazel) | 2 | 2S | 110 | 12% | 121 |  |
| 1.6 Wrapper error translation, queue messages and `make bazel-doctor` (lands with Epic 1) | 6 | 1M, 5S | 405 | 12% | 446 |  |
| 2.1 Proto to Go and connect-go | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 2.2 Proto to TypeScript for the web build | 1 | 1M | 130 | 12% | 143 |  |
| 2.3 ent generation | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 2.4 SyncGen and GenFreshness | 5 | 1M, 3S, 1XS | 310 | 12% | 341 |  |
| 2.5 buf lint as a Bazel test | 1 | 1S | 55 | 12% | 61 |  |
| 2.6 Full-graph build on a fresh clone | 1 | 1S | 55 | 12% | 61 |  |
| 2.7 Mid-point economic re-check (decision gate after Epic 1 and Stories 2.1, 2.3, 2.4, 2.5) | 1 | 1S | 55 | 12% | 61 | owner decision if the mid-point gate trips (Task 2.7a) |
| 3.1 Web dist as a Bazel target | 4 | 2M, 2S | 370 | 12% | 407 |  |
| 3.2 server/web embed staging (ADR-007) | 1 | 1S | 55 | 12% | 61 |  |
| 3.3 tmux and tymuxd embeds | 4 | 1M, 3S | 295 | 12% | 325 |  |
| 3.4 Root binary, stamping, release config | 4 | 3S, 1XS | 180 | 12% | 198 |  |
| 3.5 Auxiliary binaries | 1 | 1S | 55 | 12% | 61 |  |
| 3.6 otel variant documented as an exception | 1 | 1XS | 15 | 12% | 17 |  |
| 3.7 Release artifacts and goreleaser | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 4.1 Shared test environment helpers and defaults | 2 | 2S | 110 | 12% | 121 |  |
| 4.2 go_test defaults macro, timeouts and sharding | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 4.3 Remediate `server/services` (residual after the Story 4.10 sweeps) | 4 | 2M, 2S | 370 | 12% | 407 |  |
| 4.4 Remediate `session` (residual after the Story 4.10 sweeps) | 2 | 1M, 1S | 185 | 12% | 204 |  |
| 4.5 Remediate `session/tmux` (residual after the Story 4.10 sweeps) | 2 | 1M, 1XS | 145 | 12% | 160 |  |
| 4.6 Remediate `config`, `session/sshremote`, `server` (residual after the Story 4.10 sweeps) | 3 | 3M | 390 | 12% | 429 |  |
| 4.7 Sweep the remaining sandbox-hostile packages | 2 | 2M | 260 | 12% | 286 |  |
| 4.10 Class sweeps by SandboxClass (runs after 4.2, 4.8a and 4.9a, before 4.3-4.7) | 6 | 3M, 3S | 555 | 12% | 611 |  |
| 4.8 Test-set diff tooling and tag-matrix parity (P1, P2) | 2 | 1M, 1S | 185 | 12% | 204 |  |
| 4.9 Fix surfaced flakes and guard the ExclusionsFile | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 5.1 Darwin build under Bazel | 4 | 2M, 2S | 370 | 12% | 407 | macOS runner |
| 5.2 Signing outside Bazel and device verification | 2 | 2S | 110 | 12% | 121 | owner device TCC check |
| 6.1 Cross-compiled Windows binary | 3 | 1M, 2S | 240 | 12% | 264 |  |
| 7.1 Build metrics and trend | 1 | 1M | 130 | 12% | 143 |  |
| 7.2 Freshness gates | 2 | 2S | 110 | 12% | 121 |  |
| 7.3 Affected-target selection replacing the hand-maintained selection logic | 3 | 2M, 1S | 315 | 12% | 347 |  |
| 7.4 Remote cache wiring (server-enforced read-only for PRs) | 3 | 3S | 165 | 12% | 182 | owner credentials/exposure approval |
| 7.5 Parity gate P1-P10 | 7 | 3M, 4S | 610 | 12% | 671 | owner creates the bazel-owner-attest environment; 20 PR runs and 20 main pushes accrue; owner creates the bazel-owner-attest environment; 20 PR runs and 20 main pushes accrue |
| 7.6 Soak and per-job cutover | 4 | 3M, 1XS | 405 | 12% | 446 | soak (N, K) + D-PROTECT-MERGE decision |
| 7.7 Determinism check and non-hermetic action policy | 1 | 1M | 130 | 12% | 143 |  |
| 7.8 Auto-repair of generated files (pre-mortem #2) | 3 | 1M, 1S, 1XS | 200 | 12% | 220 | owner creates BAZEL_REPAIR_TOKEN or accepts the dispatch alternative |
| 8.1 Wrapper framework and discoverability | 2 | 2S | 110 | 12% | 121 |  |
| 8.2 Wrap build and test targets | 5 | 2M, 3S | 425 | 12% | 468 |  |
| 8.3 Lint gates stay wrapped; Bazel gates added | 1 | 1S | 55 | 12% | 61 |  |
| 8.4 Custom analyzers as nogo (conditional on S12) | 2 | 2M | 260 | 12% | 286 |  |
| 8.5 Fallback canary | 2 | 1S, 1XS | 70 | 12% | 77 |  |
| 8.6 Flip the default | 1 | 1S | 55 | 12% | 61 | owner sign-off |
| 8.7 Test summary instead of gotestsum under Bazel | 1 | 1S | 55 | 12% | 61 |  |
| 9.1 Bazel-built install path | 4 | 1M, 1S, 2XS | 215 | 12% | 237 | owner-run live install after P9 |
| 10.1 Retention window | 1 | 1S | 55 | 12% | 61 | one release-please cycle |
| 10.2 Removal | 4 | 1M, 2S, 1XS | 255 | 12% | 281 |  |
| 11.1 Skill, rule and CLAUDE.md section (lands with Epic 1) | 4 | 1M, 3S | 295 | 12% | 325 |  |
| 11.2 Agent config, lockfile conflict handling, gazelle ergonomics | 2 | 1S, 1XS | 70 | 12% | 77 |  |
| 11.3 Diataxis documentation | 4 | 2M, 2S | 370 | 12% | 407 |  |
| 11.4 worktree-disk-cleanup knows Bazel output bases | 3 | 3S | 165 | 12% | 182 | owner installs the orphan timer (Task 11.4b) |
| 11.5 Go and gopls keep working | 1 | 1XS | 15 | 12% | 17 |  |
| 11.6 Reviewability of BUILD changes | 3 | 3S | 165 | 12% | 182 |  |
| 11.7 Starlark surface cap and buildifier | 1 | 1S | 55 | 12% | 61 |  |
| 11.8 Version pinning and upgrade canary | 1 | 1S | 55 | 12% | 61 |  |
| 12.1 Observed-dependency manifest | 4 | 1M, 3S | 295 | 12% | 325 |  |
| 12.2 Slice targets in the Bazel graph | 3 | 2M, 1XS | 275 | 12% | 303 |  |
| 12.3 The out-of-band binary seam | 2 | 1S, 1XS | 70 | 12% | 77 |  |
| 12.4 Selection, shard planning and the shadow job | 4 | 1M, 3S | 295 | 12% | 325 | shadow runs accrue (count-based, same N as Story 7.6) |
| 12.5 Soundness controls | 4 | 1M, 2S, 1XS | 255 | 12% | 281 | nightly runs accrue |
| 12.6 Make selective e2e the PR gate | 1 | 1S | 55 | 12% | 61 | soak and zero escapes; owner records the flip |
| 12.7 The other slow suites | 1 | 1M | 130 | 12% | 143 |  |
| 13.1 Replay harness, T9 baseline and move tooling | 7 | 3M, 4S | 610 | 12% | 671 |  |
| 13.2 Break test-only layering inversions (Tier 0) | 3 | 3S | 165 | 12% | 182 |  |
| 13.3 Tier A clusters that pass SC6 | 4 | 1M, 2S, 1XS | 255 | 12% | 281 | open PRs on the cluster's files (Task 13.1e) |
| 13.4 Tier B in `server/services` | 4 | 2M, 2S | 370 | 12% | 407 | S14 verdict and Story 12.1 manifest; owner answers the ADR-010 questions |
| 13.5 Tier B in `session` | 5 | 4M, 1S | 575 | 12% | 633 | same; open PRs on `backlog_*` files |
| 13.6 Keep packages scoped (budget, boundary rules, documentation) | 3 | 3S | 165 | 12% | 182 |  |
| 13.7 Re-measure, compare with the prediction, close | 2 | 1S, 1XS | 70 | 12% | 77 |  |

- **Raw task total**: 19910k tokens over 270 tasks, 88 stories, 14 epics. **CU before verification**: 21.92M. **Verification multiplier x1.75** (per-story spec and quality review, `6-verify` layers, CI reruns): **38.36M CU**. Size band: **XL (greater than 15M CU)**, consistent with the XL band recorded in requirements.md (informational, not a gate). Epic 13 (ADR-010) adds 7 stories, 28 tasks, 2210k raw and 2.43M CU before verification (previous total: 13 epics, 81 stories, 241 tasks, 17645k raw, 19.43M CU, 34.00M CU after verification).
- Rule-repo fork paths, if chosen, are not in the table; ADR-008's matrix costs them as roughly +0.4M to +1.5M CU each (INFERRED) and they replace, not add to, the matching workaround tasks.

**Critical path in agent waves** (a wave = tasks that can run concurrently in separate worktrees; at most 3 Bazel-heavy runs at once because of host memory and the backlog WIP limit of 2):
1. W1: Stories 0.1 (baselines, serial because timing-sensitive), 0.2 (bootstrap, with the Task 0.2f wrapper first so every later Bazel run is contained) in parallel with 11.1 skill draft.
2. W2: Spikes 0.3 proto (Go half only), 0.4 ent, 0.5 embed (3 concurrent); then W3: 0.6 web (includes the TS-generation half of S1, Task 0.6e, because `protoc-gen-es` comes from the Bun install built in 0.6a), 0.7 tmux, 0.8 tags; then W4: 0.9 census (needs 0.3/0.4), 0.10 worktree cost, 0.11 Gazelle; 0.12 cache and 0.13 nogo run when the owner has deployed onyx.
3. W5: Story 0.14 gate (serial; owner review).
4. W6: Epic 1 (1.1 then 1.2 and 1.3 in parallel, 1.4). W7: Epic 2 (2.1, 2.3 parallel; 2.2 and 2.6 after 3.1a; 2.4 after 2.1 and 2.3; 2.5), then the Story 2.7 mid-point economic re-check before Epic 3 is released. Epic 1 now also lands the Gazelle resolve lines (Task 1.1a) and the `BUILD_SYSTEM` skeleton (Story 1.5), so Epic 2 only adds targets.
5. W8: Epic 3 (3.1, 3.3, 3.7a parallel; 3.2 after 3.1; 3.4 after 3.2 and 3.3; 3.5, 3.6). W9: Epic 4 (4.1 and 4.2 first; then the tooling tasks 4.8a and 4.9a, which the later verifies call; then the class sweeps of Story 4.10 (cwd, HOME, tmux/host binaries, git env, ports); then the residual per-package stories 4.3-4.6 in parallel pairs; 4.7; 4.8; 4.9), with Epics 5 and 6 running beside it (macOS runner, cross-compile).
6. W10: Epic 7 (7.1, 7.2, 7.3, 7.4, 7.8 parallel; 7.5 after 4.8 and 3.4, including Task 7.5e (the P9 manual-instance script, moved from Epic 9 so the parity gate can pass); 7.7), then the soak (wall-clock), then 7.6 serial per job.
7. W11: Epic 8 (8.1 then 8.2; 8.3, 8.4, 8.5, 8.7 parallel), 8.6 flip after sign-off. W12: Epic 9. Then the retention window (wall-clock) and Epic 10. Epic 11 stories 11.2-11.8 fill gaps in W8-W11.
8. W13: Epic 13 (ADR-010) starts when Stories 1.1 and 1.2 are done and runs beside W8-W12: Story 13.1 (harness; 13.1a before 13.1b-13.1g) and Story 13.2 (Tier 0; 13.2c with 13.2a) first, then Story 13.3 one cluster per PR, then Stories 13.4 and 13.5 after the Story 0.14 verdict, the Story 12.1 manifest and the owner's answers (at most 3 split PRs open at once; each Bazel-heavy verify counts against the 3-run limit), Story 13.6 with 13.2c, Story 13.7 last. It never blocks W10-W12.
Task ids are not always in lexical order in the file; file order is execution order (0.1h, 0.1i and 0.1j before 0.1f, 0.2e before 0.2d, 0.3e before 0.3d, 0.6e before 0.6d, 0.12e before 0.12c, 7.5e before 7.5d, 10.2d then 10.2c then 10.2a then 10.2b, 8.2e before 8.2b, 11.1d before 11.1a, Stories 4.8a and 4.9a before 4.10, and Story 4.10 sits between 4.2 and 4.3 in execution though listed after 4.7; Story 11.4's Task 11.4a lands with Epic 1).
Serialising shared files: `Makefile` (8.x, 2.4b, 3.6a, 5.2a, 9.1a), `.bazelrc` (many; one owner per wave), `MODULE.bazel` (Epic 0, 1.1b, 2.5a, 3.3), `.github/workflows/bazel.yml` (1.3, 2.6, 5.1, 6.1, 7.2, 7.4, 11.7).

**Overrun checkpoint**: if cumulative spend on completed stories exceeds the table value by more than 25% (at the end of Epic 0, then at the end of each epic), pause and report the delta with its cause (wrong estimate versus real surprise). Informational, not an invitation to cut. Exit rule (pre-mortem #12): at each epic end the owner chooses continue, flip, or revert to make-only with the strangler artifacts deleted; `bazel-trend.sh` tracks the `ExclusionsFile` size and CI-minutes-per-PR, and the checkpoint fails if either rises two epics in a row.

**Wall-clock blockers** (the only calendar-time items; owner = Tyler unless noted):
- Deploy `bazel-remote`, the read-only and read-write proxy listeners, Tailscale ACL tags and the `bazel-cache-write` environment on onyx/GitHub (Story 0.12, 7.4) - owner.
- Owner review of spike verdicts and creation of any fork repos (Story 0.14) - owner.
- Owner confirmation of improvement targets T1-T8 and economic-gate factors (Task 0.1g, evaluated at Task 0.14e); an owner decision if the economic gate trips - owner.
- Soak of the non-blocking job (Story 7.6; N and K fixed by the owner before Epic 7) and the D-PROTECT-MERGE decision - owner.
- macOS device TCC verification on arm64 and amd64 (Story 5.2, P8) - owner.
- Owner sign-off of the parity gate and the `BUILD_SYSTEM` flip (Story 8.6), and the live `make install-service` (Story 9.1d) - owner.
- One release-please cycle of fallback retention (Story 10.1) - elapsed time, no action.
- Create the GitHub environment `bazel-owner-attest` (required reviewer = the owner) before Task 7.5c can run, and the environment `bazel-repair` plus `BAZEL_REPAIR_TOKEN` (or choose the dispatch alternative) before Story 7.8 - owner.
- Run the live-service OOM drop-in (Task 0.2i) before Story 0.10 and install the orphan timer (Task 11.4b) - owner.
- P10 sample windows: the P10 and T2 rows need at least 20 Bazel PR runs and 20 warm `main` push runs to accrue after the non-blocking job exists (count-based elapsed time, no action) - external.
- Owner answers to the "ADR-010" Unresolved Questions (tiers and criteria, freeze windows, T1 on the target model, T9 values) before Stories 13.1f, 13.3, 13.4 and 13.5 - owner.
- GitHub Actions queue and macOS runner availability for CI runs - external.


---

## Epic 0: Spikes and Foundation (runs first; gates all other epics)

**Goal**: Turn every UNVERIFIED/INFERRED research item into a capped spike with an explicit pass/fail criterion and a pre-decided fallback, measure the three driver baselines, and land the inert foundation files. No application code changes merge from this epic (spike work lives on throwaway branches in separate worktrees; at most 3 spikes run concurrently because of the memory-bounded host and the backlog WIP limit).

**Spike protocol** (applies to Stories 0.3-0.13): each spike records into `project_plans/bazel-conversion/implementation/spike-results.md` a section with: the command(s) run and their output, the `SpikeVerdict` (`Pass`, `FallbackWorkaround`, `FallbackFork`, `Fail`), the measured cost class of each path, and the chosen path under ADR-008. **Cap**: a spike stops after its listed tasks plus one repair run per task; if it has not reached `Pass` or a costed fallback by then, the verdict is `Fail` and the owner is told with the evidence (no silent extension).

**Falsifiable verdict rule** (applies to every spike): each spike below states a `PASS iff` line made of numeric or boolean checks. `Pass` requires every check true. A result that needs an "explained delta" is `FallbackWorkaround` unless every delta is listed in the pre-approved allow-list `tools/bzl/parity/allowed-deltas.txt` (initially only: minifier banner/version string comment). A check that cannot be evaluated (tool missing, owner action pending) is `Fail`, never `Pass`. Verdicts are recorded as rows in `spike-results.yaml` (`{spike, verdict, path, cost}`) and validated by `tools/bzl/check-spike-results.py` (Task 0.14a); spike ids are `S-gomod, S1, S1-ts, S2, S3, S4, S5, S6, S7, S8, S9, S10, S10-redaction, S11, S12, S13`.

**If S4 (hermetic Bun) ends `Fail`** (the repository rule, `rules_bun` and the fork each failed their checks): S1-ts, Story 2.2 and Story 3.1 depend on `protoc-gen-es` from the Bun install, so they fail with it. Pre-decided consequences: (a) `protoc-gen-es` and its two dependencies come from a pinned-tarball repository rule (`http_archive` of the three npm tarballs by sha256, cost M) so proto TS generation stays hermetic; (b) the web `dist` action becomes a host-Bun action tagged `no-remote-cache`, `no-sandbox` and `requires-network` is NOT allowed (the install stays in the repository rule); (c) this hermeticity downgrade is covered by the pre-registered owner decision D-HERMETIC. The same applies to S5: a host-cc tmux genrule is the only permitted downgrade and is single-target.

**Fallback matrix** (what each spike decides; costs are classes, INFERRED):

| Spike | Pass criterion | Workaround path (cost) | Fork path (cost) | Default if Pass fails |
|---|---|---|---|---|
| S-gomod (0.2) | `bazel mod tidy` leaves no diff; `bazel build //tools/lint/...` green on a fresh clone; Go 1.26.6 SDK resolved | Merge `tools/lint` into main `go_deps` universe (S); pinned nearest SDK + go.mod toolchain line (XS, owner approval) | none | Workaround |
| S1 proto Go half (0.3) | PASS iff import paths equal `.../gen/proto/go/session/v1[/sessionv1connect]`, `diff -r` against `make proto-gen` output after gofmt is empty (no allow-list entries), connect-go v1.20.0, built with `--sandbox_default_allow_network=false`, the `buf.yaml` BSR dependency question is resolved (Task 0.3e), and a cold-runner build log has no protoc C++ compile steps because protoc is prebuilt (Task 0.3f) | `buf generate` as `run_binary` with local pinned plugins (M) | Connect compiler + managed-mode import support in rules_go (L) | Workaround |
| S2 ent (0.4) | PASS iff `bazel build //tools/bzl/ent/...` (where the 52 generated libraries live; `//session/ent/...` builds only `schema`) from a clean clone with no network produces every one of the 53 packages from `go list ./session/ent/...` as a `go_library` with its own importpath, `OnConflict*` upsert builders present (the `UpsertRule` symbol does not exist; evidence is 46 `OnConflict` builders vs 0 without the flag), two clean builds hash-equal, AND `bazel build //session/... //server/...` resolves every importer of every ent subpackage (a single-package success does not count) | Custom rule creating `GoSource` from the output tree (M+S) or `entc` program with SDK and local `GOFLAGS=-mod=mod` marked `no-remote-cache` (M) | rules_go directory `srcs` (L) | Custom rule |
| S3 embed (0.5) | `go_library(embedsrcs = staged dist)` compiles and a test reads `index.html` from the embedded FS | Custom tree-expansion rule (M) | rules_go directory `embedsrcs` (M+L) | Workaround; `dist.tar` only via amended ADR-007 |
| S4 web (0.6) | PASS iff `//web-app:dist` file list and sha256 equal `make web-build` output (deltas only from `allowed-deltas.txt`); 20-test Jest subset pass-set equal; zero writes to the source tree and zero reads outside `web-app/` (count of such paths is exactly 0); the sandboxed `dist` action wall time with the real `node_modules` is at most 1.5x `make web-build` wall time, else the verdict stays `Pass` with `path` = `repo-rule+no-sandbox` (a recorded cost: consuming actions tagged `no-sandbox`, not a downgrade) | pinned third-party `rules_bun` via `git_override` (S+M, Task 0.6f), reached when the repository rule fails output parity, hermeticity or determinism | aspect_rules_js reads `bun.lock` (L+L+M+S), reached when `rules_bun` fails the same checks or per-package granularity is needed | `Pass` = Bun repository rule (M+M+S+S, the ADR-003 default); workaround = `rules_bun`; fork = aspect_rules_js; all three failing = `Fail` and the S4-fails consequences below apply |
| S5 tmux (0.7) | PASS iff `//tools/bzl/tmux:tmux_bin` builds from the `http_archive` tarball in a clone where `third_party/tmux` is empty, `tmux -V` prints exactly `tmux 3.8`, `git ls-files third_party/tmux` shows only the gitlink, `git check-ignore tools/bzl/tmux/BUILD.bazel` prints nothing, and the action ran sandboxed | `rules_foreign_cc` 0.17.0 `configure_make` (M); else genrule with host cc, single-target hermeticity exception (M) | Patch rules_foreign_cc for Bazel 9 (M) | Genrule |
| S7 tags (0.8) | PASS iff all `TagConfig` build; `embed_tymux` implies `embed_tmux`; `select()` on `//tools/bzl/flags:embed_tmux` wires the embed file; and `--config=linux --config=race` builds (race needs `pure=false`, see Task 1.1c) | Transition macro for tag lists (S) | none | Workaround |
| S8/S9 sandbox (0.9) | PASS iff every failing test of the 10 census packages is assigned exactly one `SandboxClass` (sum of class counts equals the failing-test count from `test.xml`, computed by script) and the socket test passes with short `TMPDIR` and fails without it | n/a (research) | n/a | Fix at source via helpers |
| S6 worktree cost (0.10) | PASS iff, over 5 runs, the median second-worktree cold Bazel build is at most 30% of the `baseline-worktree.json` make median (the same definition as P10) and 3 concurrent builds peak at most 12 GB combined RSS; in addition 8 queued invocations through the `tools/bazel` wrapper (Task 0.2f) while a synthetic terminal stream runs against a manual instance keep peak combined RSS at most 12 GB and stream p99 latency at most 1.2x idle, and the concurrent shared-disk-cache test (Task 0.10c) finds zero digest mismatches, AND the cross-worktree sharing checks hold: (E1, Task 0.10e) a second worktree at a different path and depth builds `//:stapler-squad` with at least 95% of cacheable spawns as disk-cache hits, every non-hit on a pre-declared allow-list (tmux, Bun consumers); (E2) a third worktree runs zero non-allow-listed `linux-sandbox`/`worker` spawns in at most 30% of the first worktree's wall time; (E3) the `GoCompile` aquery command lines of three Go targets are byte-identical across two worktree paths with no absolute path; (E4) the sorted sha256 list of `bazel-bin` regular files from two cache-disabled builds in two worktrees is identical and the positive-control `pwd` genrule is flagged; (E5, Task 0.10f) with `max_idle_secs=30` the disk cache is trimmed to its GC cap after the idle delay (and is not trimmed under the default 5 m delay); (E6) a fresh worktree builds the Go toolchain repos with network blocked and every non-extension external repo is a symlink into the repo contents cache (extension-created repos are listed as a per-worktree cost, not a failure); the reflink result (E10), the containment and cold-start measurements (E7, E11, Task 0.10g) and the multiplex-worker inventory (Task 0.10h, a within-build memory optimisation, not cross-worktree) are recorded inputs, not gates: without reflink the disk bound is re-measured as copy-based | per-worktree `--output_base` inside the worktree (absolute path from the wrapper plus a `.bazelignore` entry); lower `max_idle_secs`; agent concurrency cap; if E3 fails, evaluate `--experimental_output_paths=strip` | none | Workaround |
| S10 cache (0.12) | PASS iff a real `bazel build` on a GitHub-hosted runner with `--remote_cache=https://<onyx>:9092` (HTTP cache protocol, PR key only) reports remote cache hits greater than 0 on its second run, a PUT through the same endpoint returns 4xx/405 from the server, an unauthenticated PUT to `:9093` returns 401, the main-only write credential PUTs successfully, and none of three unreachable-cache cases (DROP-ed address, `tc netem` 2 s delay with 20% loss, accept-then-never-respond listener; Task 0.12c) adds more than 10 s over the no-remote-cache run, and with `--disk_cache` and `--remote_cache` both set a build from an empty disk cache gets remote hits and the disk cache grows afterwards (E8, Task 0.12f). S10-redaction (Task 0.12d) is a separate verdict: PASS iff a canary secret value, and its base64 form, appear 0 times in `bep.json`, the gunzipped profile, the execution log, `command.log` and stdout/stderr (with HTTP verbose/debug output enabled) of both a successful run and a deliberately wrong-credential run that authenticates through `--credential_helper` | Public TLS+htpasswd path vs Tailscale; else `actions/cache` disk cache (ADR-004) | none | Disk-cache-only (owner decides on BuildBuddy) |
| S11 gazelle (0.11) | PASS iff second Gazelle run has zero diff; zero diff with and without a synced `GenSurface` present; at most 8 `resolve`/`resolve_regexp` lines in total (a `resolve_regexp` mapping all ent packages to `tools/bzl/ent` targets counts as one line if Gazelle substitutes `$1`; if it does not, the extension path applies); and `session/ent/schema` still gets its generated BUILD file | Directives + `map_kind` macro (S) | Custom Gazelle extension with Go unit tests (M-L) | Directives first |
| S13 lockfile (0.10d) | PASS iff, over a 20-PR replay of two-dependency-addition merges, the chosen option (committed `MODULE.bazel.lock` with the regenerating merge driver, versus not committing it) yields zero lock conflicts and zero `--lockfile_mode=error` failures on the replayed `main`, AND a real two-PR merge on GitHub shows whether GitHub-side merges ignore repo merge drivers (INFERRED today) | Do not commit the lock (amends ADR-001, owner approval) | none | Commit lock + merge driver (Tasks 0.10b, 11.2a) |
| S12 nogo (0.13) | One `tools/lint` analyzer runs as `nogo` and flags a seeded violation | Keep `make lint-custom` wrapped (zero) | none | Wrapped; Story 8.4 skipped |
| S14 selective e2e (0.15) | PASS iff 193/193 specs have an observed manifest entry, the replay over the last 100 `main` commits gives median selected share at most 0.50, p90 at most 0.90 and `__ALL__` fallback at most 15%, the 30-mutation backtest has 0 misses, and the Bazel semantic checks S-a to S-e pass | Diff-line coverage selection with an escape budget (M); or a coarser slice (package prefix plus declared registry sources) | none | Epic 12 not released; e2e stays a full run (owner decision, ADR-009 Consequences) |

#### Story 0.1: Measure baselines for the three drivers
**As the** owner, **I want** pre-Bazel numbers for CI time-to-green, cold build time and disk per worktree, and hermetic fresh-clone behaviour, **so that** every later claim is compared with evidence.
**Acceptance Criteria**:
- CI time-to-green baseline is recorded for the last 30 PR head SHAs on which every required-in-practice workflow (`build.yml`, `lint.yml`, `mcp-integration.yml` when triggered, and the e2e shards in `build.yml`) succeeded.
  - *Given* the repo's GitHub Actions history, *When* `tools/bzl/baseline/ci-time-to-green.sh` runs, *Then* it prints JSON with `n: 30`, `p50_s`, `p90_s` and `p95_s` numeric fields where time-to-green is defined as the latest `updated_at` minus the earliest `run_started_at` across those workflows for the same head SHA (skipped and short-path runs included, so the baseline is not biased toward long runs).
- Cold build time and disk per worktree are recorded under the make path.
  - *Given* a fresh `git worktree add` from `origin/main` with no shared caches, *When* `tools/bzl/baseline/worktree-cost.sh` runs five times, *Then* it writes `baseline-worktree.json` with `wall_s`, `disk_kb_worktree`, `disk_kb_node_modules`, `loadavg1` and `mem_used_gb` per run, plus median, min and max. The script refuses to start while the 1-minute load average exceeds 2.0 (the host is shared with agent sessions, so quiet is enforced, not requested).
- The hermetic-clone gap is quantified.
  - *Given* a clone into an empty temp directory, *When* `go build ./...` runs with no generation, *Then* it exits non-zero and the report lists the missing generated packages (`gen/proto/go/...`, `session/ent`).
- The measured pain behind the decision is recorded (why now).
  - *Given* the repo history and agent transcripts, *When* `tools/bzl/baseline/pain-numbers.sh` runs, *Then* `baselines.md` has a "Measured pain" section with CI p50/p95 time-to-green, cold worktree build time, disk per worktree, the count of manual regen-codegen steps per agent session (median and p90 over the last 30 sessions), warm one-package `go test` latency, and the legacy first-error legibility result for the 9 seeded fixtures; `decisions/ADR-001-adopt-bazel-bzlmod-build-of-record.md` links to that section.
- Improvement targets exist before any Bazel work is judged.
  - *Given* those baselines, *When* Task 0.1g completes, *Then* `tools/bzl/baseline/targets.yaml` holds targets T1-T8 (see "Improvement targets" above) with formula, baseline value and target value, and `owner-attestations.yaml` records `targets_confirmed: <date>` by the owner (target set at Story 0.1 from baseline, owner-confirmed).
- The T1/T2 sample is representative and the billed-minutes join exists.
  - *Given* the 100 most recent merged PRs, *When* `bash tools/bzl/baseline/pr-sample.sh` runs, *Then* it prints JSON with 30 `partial` PRs, `population: 100` and either `repr.ok: true` or `restratified: true`, and `ci-billed-minutes.sh` prints runner-minutes and time-to-green p50 for the `partial`, `global` and `all` classes.
- Hermeticity before is recorded.
  - *Given* a fresh clone, *When* `bash tools/bzl/baseline/hermeticity.sh --system make` runs, *Then* it prints H1 (host executables, network destinations) and H2 (build with scrubbed `PATH`), the make-path baseline for target T7.
- The two risky assumptions are pre-measured.
  - *Given* the partial-change sample, *When* `bash tools/bzl/baseline/risky-assumptions.sh` runs, *Then* it prints `ra1_median_fanout` (fraction of Go packages that transitively import the changed packages) and `ra2_non_go_share` (share of the baseline critical path outside Go build and test).
**Files**: `tools/bzl/baseline/*.sh`, `tools/bzl/baseline/fixtures/*.sh`, `tools/bzl/baseline/targets.yaml`, `tools/bzl/baseline/measured.json`, `tools/bzl/baseline/pr-sample.json`, `project_plans/bazel-conversion/implementation/baselines.md`, `project_plans/bazel-conversion/implementation/baseline-test-set.json`

##### Task 0.1a: CI time-to-green script (cost S)
- Write `tools/bzl/baseline/ci-time-to-green.sh` using `gh api repos/tstapler/stapler-squad/actions/runs?event=pull_request` grouped by `head_sha`; keep SHAs where every workflow that ran concluded `success` or `skipped`; compute p50/p90/p95 of (max `updated_at` - min `run_started_at`) per SHA over the 30 most recent. Also output the per-workflow p50 so a later workflow-by-workflow comparison is possible. Record the output, run date and `n` in `baselines.md`.
- Files: `tools/bzl/baseline/ci-time-to-green.sh`, `project_plans/bazel-conversion/implementation/baselines.md`
- Verify: `bash tools/bzl/baseline/ci-time-to-green.sh | jq -e '.n==30 and (.p50_s|type=="number") and (.p95_s|type=="number") and (.per_workflow|length>=3)'` prints `true`.

##### Task 0.1b: Cold build and disk per worktree script (cost S)
- Write `tools/bzl/baseline/worktree-cost.sh`: create a temp worktree outside the repo, run `make build` with `GOCACHE` and the shared node_modules mechanism left as default, record wall seconds and `du -sk` deltas, remove the worktree. Run it five times, each gated on `loadavg1 <= 2.0` (`/proc/loadavg`) and recording `free -g` next to the timing; record median, min and max. Never touches `:8543`.
- Files: `tools/bzl/baseline/worktree-cost.sh`, `baselines.md`
- Verify: `bash tools/bzl/baseline/worktree-cost.sh --runs 5 | jq -e '(.runs|length==5) and (.runs|all(.loadavg1<=2))'` prints `true`; `git worktree list` shows no leftover temp worktree.

##### Task 0.1c: Fresh-clone hermeticity gap (cost XS)
- Clone to `$(mktemp -d)` (short path), run `go build ./... 2>&1 | head -40`, record exit code and missing packages in `baselines.md`.
- Files: `baselines.md`
- Verify: `grep -c 'gen/proto/go' project_plans/bazel-conversion/implementation/baselines.md` is at least 1.

##### Task 0.1d: Recount scale numbers (cost XS)
- Recount tracked `.go` files, `_test.go` files, packages with tests, live `go.mod` files, with the commands in `baselines.md`; reconcile 3,373 (tracked, VERIFIED by `git ls-files '*.go' | wc -l`) vs ~3,743 (research/requirements, which counted untracked files); close with a one-line note.
- Files: `baselines.md`
- Verify: `grep -cE '^(tracked_go_files|test_packages|go_mod_files)[:= ]+[0-9]+' baselines.md` prints `3`; `grep -oE 'tracked_go_files[^0-9]*[0-9]+' baselines.md | grep -oE '[0-9]+$'` prints the value `git ls-files '*.go' | wc -l` printed at the recorded baseline commit (`git rev-parse HEAD` stored beside it in `baselines.md`).

##### Task 0.1e: Record the current Go test pass set (cost S)
- Run `gotestsum --jsonfile /tmp/go-test.json -- -short ./...` (via the existing make recipe flags) and store the sorted unique `Package/Test` names that passed into `baseline-test-set.json`; list pre-existing failures separately (BUG-087..090 flakes). This file is the Epic 0 reference only and goes stale within weeks; P1 compares a live `go test` run with a live Bazel run on the same commit (Task 4.8a) and the flake list is regenerated at the gate.
- Files: `project_plans/bazel-conversion/implementation/baseline-test-set.json`
- Verify: `jq -e '.passed|length>1000' baseline-test-set.json` prints `true`.

##### Task 0.1h: Seeded failure fixtures on the make path (cost S) (runs before Task 0.1f; Task 1.6d replays the same fixtures)
- Author the 9 seeded failure fixtures once, here, so Epic 0 never depends on Task 1.6d: `tools/bzl/baseline/fixtures/<n>-<name>.sh` for stale BUILD, missing external dep, missing generated code, OOM, ENOSPC, cache unreachable, flock queue wait, sandbox failure and cache auth rejected (a local stub HTTP server answering 403). Each script seeds its failure in a throwaway worktree, runs the command an agent would run, and writes `expected.key` (the errors.yaml row E<n> that must fire under Bazel). `run-make-path.sh` runs all 9 through the make path and records per fixture whether the message names a remedy (`true`, `false`, or `N/A-make` for failures that cannot occur without Bazel). Task 1.6d replays the same scripts through the wrapper.
- Files: `tools/bzl/baseline/fixtures/*.sh`, `tools/bzl/baseline/fixtures/run-make-path.sh`, `baselines.md`
- Verify: `bash tools/bzl/baseline/fixtures/run-make-path.sh | jq -e '(.fixtures|length==9) and (.fixtures|all(.names_remedy==true or .names_remedy==false or .names_remedy=="N/A-make"))'` prints `true`; `ls tools/bzl/baseline/fixtures/[0-9]*.sh | wc -l` prints `9`; `git worktree list | grep -c fixture` prints `0` afterwards.

##### Task 0.1i: PR sampling, representativeness and billed-minutes join (cost S) (runs before Task 0.1f)
- `tools/bzl/baseline/pr-sample.sh`: take the last 100 merged PRs (`gh pr list --state merged --limit 100 --json number,mergedAt,headRefOid,files`), classify each `partial` or `global` using the seed list `tools/bzl/baseline/global-inputs.seed.txt` (a static copy of the planned `GlobalInputs`, because `global-inputs.txt` is Task 7.3a), take the 30 most recent `partial`, apply the representativeness rule of "Sampling, proxies and risky assumptions" (restratify on failure) and write `pr-sample.json`. `tools/bzl/baseline/ci-billed-minutes.sh` joins per head SHA the runner-minutes (job `completed_at` minus `started_at` summed over `gh api repos/tstapler/stapler-squad/actions/runs/<id>/jobs`, OS-weighted 1/10/2; the repository is public so the billing API returns 0) and time-to-green with the class, printing p50 per class (`partial`, `global`, `all`).
- Files: `tools/bzl/baseline/pr-sample.sh`, `tools/bzl/baseline/ci-billed-minutes.sh`, `tools/bzl/baseline/global-inputs.seed.txt`, `tools/bzl/baseline/pr-sample.json`
- Verify: `bash tools/bzl/baseline/pr-sample.sh | jq -e '(.partial|length==30) and (.population==100) and (.repr.ok==true or .restratified==true)'` prints `true`; `bash tools/bzl/baseline/ci-billed-minutes.sh | jq -e '(.partial.runner_min_p50|type=="number") and (.global.runner_min_p50|type=="number") and (.all.ttg_p50_s|type=="number")'` prints `true`.

##### Task 0.1j: Hermeticity baseline on the make path (cost S) (runs before Task 0.1f)
- `tools/bzl/baseline/hermeticity.sh --system make|bazel` (the Bazel mode is run by Task 7.5g): H1 counts distinct host executables outside the workspace and the pinned toolchain dirs and distinct non-loopback network destinations from `strace -f -e trace=execve,connect` over a fresh-clone `make build` (`UNMEASURED: strace missing` if unavailable, never estimated); H2 runs the same build under `env -i PATH=/usr/bin:/bin` and records success as a boolean; H3 (Bazel only, flag names verified in Task 0.2b) counts actions that fail under a hermetic sandbox; H4 counts rows in `tools/bzl/hermetic-downgrades.txt`. Output goes to `tools/bzl/baseline/hermeticity-make.json` and the "Measured pain" section.
- Files: `tools/bzl/baseline/hermeticity.sh`, `tools/bzl/baseline/hermeticity-make.json`, `baselines.md`
- Verify: `bash tools/bzl/baseline/hermeticity.sh --system make | jq -e '(.h1_host_executables|type=="number") and (.h1_network_destinations|type=="number") and (.h2_path_scrubbed_build_ok|type=="boolean")'` prints `true`.

##### Task 0.1f: Measured pain numbers (cost S)
- Write `tools/bzl/baseline/pain-numbers.sh` (calls Tasks 0.1a and 0.1b output, so no recomputation) and `tools/bzl/baseline/regen-steps.sh`. The latter scans the last 30 agent-session transcripts for this repo (`~/.claude/projects/-home-tstapler-Programming-stapler-squad*/*.jsonl`) and counts, per session, `make proto-gen`, `make ent-gen` and `make sync` invocations plus build errors matching `gen/.* not found` or `undefined: .*sessionv1`; prints median and p90. Also records: warm one-package latency (`go test ./config` after one warm-up (`make test PKG=` does not exist today; Task 8.2e introduces it), median of 5), the partial-change class stats from Task 0.1i, the fixture results from Task 0.1h, the hermeticity baseline from Task 0.1j, and the pre-measurement of the two risky assumptions: `tools/bzl/baseline/risky-assumptions.sh` computes RA1 (for each of the 30 sampled diffs, the fraction of Go packages that transitively import a changed package, from `go list -f '{{.ImportPath}} {{join .Deps " "}}' ./...`; median reported) and RA2 (the share of each sampled PR's critical-path time spent in e2e, lint and runner-setup jobs, from `gh api repos/tstapler/stapler-squad/actions/runs/<id>/jobs`; median reported). Both are inputs to EG5 of Task 0.14e.
- Everything is written into a "Measured pain" section of `baselines.md` with run date and `n`; numbers that cannot be measured (for example fewer than 30 transcripts) are written as `UNMEASURED: <reason>`, never estimated.
- Files: `tools/bzl/baseline/pain-numbers.sh`, `tools/bzl/baseline/regen-steps.sh`, `tools/bzl/baseline/risky-assumptions.sh`, `tools/bzl/baseline/measured.json`, `baselines.md`
- Verify: `bash tools/bzl/baseline/pain-numbers.sh | tee tools/bzl/baseline/measured.json | jq -e '(.ci_p50_s|type=="number") and (.ci_p95_s|type=="number") and (.cold_worktree_wall_s|type=="number") and (.disk_kb_worktree|type=="number") and (.regen_steps_per_session_median|type=="number") and (.warm_one_pkg_test_s|type=="number") and (.ra1_median_fanout|type=="number") and (.ra2_non_go_share|type=="number")'` prints `true` (the `tee` saves the output as `measured.json` for later doc checks); `grep -c 'Measured pain' project_plans/bazel-conversion/implementation/baselines.md` is at least `1`.

##### Task 0.1g: Improvement targets and ADR-001 evidence link (cost XS)
- Compute T1-T8 and T2b (absolute cache-hit target, no baseline ratio) from `baselines.md` (T8's baseline is the e2e share of the partial-class runner-minutes from the `e2e (Bun build) shard` jobs, taken from the same Task 0.1i sample) using the proposal formulas in "Improvement targets" and write `tools/bzl/baseline/targets.yaml` (`{id, formula, baseline_value, target_value, source}` per row, plus the economic-gate factor rows EG1-EG5, which Task 0.14e defines and later consumes; write them now with proposal values). Re-score the RICE table in ADR-001 with the measured Reach/Impact values (inputs that remain unmeasured stay labelled UNVERIFIED) and add the link from ADR-001 to the "Measured pain" section. Ask the owner to confirm or amend each target; record `targets_confirmed: <date>` and any amendment reason in `owner-attestations.yaml`. Owner confirmation is a wall-clock blocker for Story 0.14, not for Epic 0 measurement work.
- Files: `tools/bzl/baseline/targets.yaml`, `baselines.md`, `decisions/ADR-001-adopt-bazel-bzlmod-build-of-record.md`, `owner-attestations.yaml`
- Verify: `yq -e '(.targets|length)>=7 and (.targets|all(.formula and .baseline_value != null and .target_value != null))' tools/bzl/baseline/targets.yaml` prints `true`; `grep -c 'Measured pain' project_plans/bazel-conversion/decisions/ADR-001-adopt-bazel-bzlmod-build-of-record.md` is at least `1`; `yq -e '.targets_confirmed' owner-attestations.yaml` prints a date.

#### Story 0.2: Foundation bootstrap (inert, merges to main)
**As an** agent, **I want** pinned Bazel, module and ignore files, **so that** every spike and later epic starts from one reproducible base.
**Acceptance Criteria**:
- Bazel resolves with pinned versions.
  - *Given* `.bazelversion` containing `9.3.1`, *When* `bazel version` runs, *Then* it prints `Build label: 9.3.1`.
- The module graph is stable.
  - *Given* `MODULE.bazel` with rules_go 0.65.0, gazelle 0.54.0 and `go_sdk.from_file(go_mod = "//:go.mod")`, *When* `bazel mod tidy` runs twice, *Then* `git diff --exit-code MODULE.bazel MODULE.bazel.lock` is clean after the second run.
- Nested worktrees and copies are invisible.
  - *Given* `.claude/worktrees/agent-x/BUILD.bazel` exists, *When* `bazel query //...` runs, *Then* no label starts with `//.claude/worktrees`.
- The smallest self-contained module builds.
  - *Given* a fresh clone, *When* `bazel test //tools/lint/...` runs, *Then* exit 0.
- Every Bazel-owned BUILD path is loadable on a clean clone.
  - *Given* `tools/bzl/hand-build-files.txt`, *When* `bash tools/bzl/check-build-paths.sh` runs, *Then* it exits 0, meaning each listed path is not matched by `git check-ignore`, is not under a gitlink (mode 160000 per `git ls-files -s`) and is not covered by a `.bazelignore` entry; it exits 1 naming the first violation (the old plan placed targets under the `third_party/tmux` gitlink and the gitignored `build/`).
- Owner-controlled files have a CODEOWNERS entry.
  - *Given* `.github/CODEOWNERS`, *When* `gh api repos/tstapler/stapler-squad/codeowners/errors --jq '.errors|length'` runs, *Then* it prints `0`.
- Bazel is contained on the shared host (pre-mortem #1).
  - *Given* `tools/bazel` (Task 0.2f), *When* 6 invocations start at once, *Then* at most 2 run concurrently under a `systemd-run` scope with `MemoryMax=14G`, and `bash tools/bzl/wrapper_test.sh` exits 0.
- Startup flags really reach the server (docs-derived D1: `startup:<config>` lines are silently ignored).
  - *Given* the committed `.bazelrc`, *When* `bazel build --config=agent //tools/lint/...` runs and `tr '\0' '\n' < /proc/$(bazel info server_pid)/cmdline` is read, *Then* it contains `--max_idle_secs=300` and `-Xmx2g` (the wrapper adds `--max_idle_secs=30` for agent callers on the command line, Task 0.2f), and `bash tools/bzl/check-bazelrc_test.sh` proves a `startup:agent` line is rejected.
- Repo guardrails exist as checks, not conventions (validation G4, G5).
  - *Given* a WORKSPACE file, a `--spawn_strategy=local` line, `flaky = True`, or a force-push in a workflow, *When* `bash tools/bzl/check-bazelrc.sh` or `bash tools/bzl/check-no-force-push.sh` runs, *Then* it exits 1 naming the offender.
**Files**: `.bazelversion`, `MODULE.bazel`, `MODULE.bazel.lock`, `.bazelrc`, `.bazelignore`, `BUILD.bazel`, `tools/bzl/check-build-paths.sh`, `tools/bazel`, `tools/bzl/check-bazelrc.sh`, `tools/bzl/check-no-force-push.sh`, `tools/bzl/check-bazelignore.sh`, `tools/bzl/hand-build-files.txt`, `.github/CODEOWNERS`, `scripts/hooks/pre-push-no-force.sh`, `scripts/check-live-oom-protect.sh`, `docs/how-to/protect-live-service-from-bazel.md`, `Makefile`

##### Task 0.2a: MODULE.bazel, .bazelversion, .bazelignore, root BUILD (cost M)
- Create `.bazelversion` (`9.3.1`), `MODULE.bazel` (rules_go 0.65.0, gazelle 0.54.0, protobuf 36.2, rules_cc 0.2.27, platforms 1.1.0, bazel_skylib 1.9.2, rules_shell 0.8.0, `go_sdk.from_file`, `go_deps.from_file` for the root `go.mod`, a second isolated usage for `tools/lint`; every version pin is checked by `bazel mod tidy` resolving it against the BCR, which fails on an unknown version), `.bazelignore`, root `BUILD.bazel` with `gazelle` target and directives from `architecture.md` section 2.1 (prefix, `go_naming_convention import_alias`, excludes). `.bazelignore` entries are exact root-relative paths, not patterns (`node_modules` ignores only `/node_modules`): list `.claude/worktrees`, `worktrees`, `.bazel-out` (the in-worktree output base of the S6 fallback, Task 0.2f; VERIFIED by run that without this entry `//...` fails with "infinite symlink expansion detected ... external/_main"; the directory is also added to `.gitignore` as the anchored `/.bazel-out`), `third_party/tmux` (submodule, version pin only), `project_plans`, `website`, `gen`, `web-app/src/gen`, `web-app/node_modules`, `web-app/out`, `build` (make output directory; no Bazel file ever lives there) and every nested `node_modules` directory found by `find . -name node_modules -type d -prune -not -path './.claude/worktrees/*'` at implementation time (none exist in a fresh checkout before `bun install`, so `tools/bzl/check-bazelignore.sh` fails if any `node_modules` directory exists that is neither listed nor under a listed path). Gazelle does not read `.bazelignore`, so the root BUILD also carries `# gazelle:exclude **/node_modules`, `# gazelle:exclude third_party/tmux`, `# gazelle:exclude gen` and `# gazelle:exclude web-app/src/gen`. The exclusion for the gitignored `session/ent` entity directories must spare the tracked `session/ent/schema` (VERIFIED: the only tracked files under `session/ent` are `schema/*`); the mechanism is provisional in Epic 0 (`# gazelle:exclude session/ent/[!s]*`, which spares `schema`) because Gazelle is not run repo-wide until Task 1.1a, and is finalised by Task 0.11b; nothing in Tasks 0.2a-0.2e depends on the final form. The `.bazel-out` entry is added now although that directory first exists in Task 0.2f's fallback mode.
- Files: `.bazelversion`, `MODULE.bazel`, `.bazelignore`, `BUILD.bazel`, `tools/bzl/check-bazelignore.sh`
- Verify: `bazel mod tidy && git diff --exit-code MODULE.bazel` exits 0 on the second run; `bash tools/bzl/check-bazelignore.sh` exits 0; `bazel query //tools/lint/... //tools/scanner/... //tuitest/...` and `bazel run //:gazelle -- -mode=diff tools/lint tools/scanner tuitest` (incremental: the whole-repo run belongs to Task 1.1a) touch no path under a `node_modules` (`... | grep -c node_modules` prints `0`).

##### Task 0.2b: .bazelrc (configs, shared caches, agent mode) (cost S)
- Create `.bazelrc` with `common --enable_bzlmod`, `--lockfile_mode=update` (CI overrides to `error`), shared `--disk_cache`/`--repository_cache` under `~/.cache/stapler-squad-bazel/`, `common --enable_platform_specific_config`, and the startup block, all **unconditional** because `startup:<config>` lines are ignored (`[DOC]` https://bazel.build/run/bazelrc, `[RUN]` a `startup:agent --max_idle_secs=777` line did not reach the server while `startup:linux` did, and `build --config=agent` with only a `startup:agent` line fails with "Config value 'agent' is not defined"): `startup --host_jvm_args=-Xmx2g`, `startup --max_idle_secs=300`, `startup --shutdown_on_low_sys_mem` (the docs' multi-workspace memory recommendation), `startup --io_nice_level=7`, `startup --batch_cpu_scheduling` (belt-and-braces for callers that bypass the wrapper; their effect is checked in Task 0.10g, not assumed). Only `startup:linux`/`startup:macos`-style platform configs may exist. Resource cap: `build --local_resources=cpu=HOST_CPUS*.5` and `build --local_resources=memory=HOST_RAM*.2` (never the nonexistent `--local_ram_resources`/`--local_cpu_resources`). Disk-cache GC (built in since Bazel 7.4, names still `experimental_`): `common --experimental_disk_cache_gc_max_size=100G`, `common --experimental_disk_cache_gc_max_age=30d`, `common --experimental_disk_cache_gc_idle_delay=10s`; the delay must be below the shortest `max_idle_secs` in use (agent callers: 30 s) because GC runs inside the idle server and the default 5 m delay would never elapse (`[INF]`, proven or refuted by E5 in Task 0.10f); note the cap is enforced well under the limit in one pass (`[RUN]` 20 MB shrank to 8 KB under a 5 MB cap), so steady state sits below 100 GB. Repository caches: keep `--repository_cache=~/.cache/stapler-squad-bazel/repo` and set `--repo_contents_cache=~/.cache/stapler-squad-bazel/repo/contents` and `--repo_contents_cache_gc_max_age=30d` explicitly beside it, so downloads and extracted repos (Go SDK, toolchains: `external/<repo>` is a symlink into the contents cache, `[RUN]`) land together. `--config=agent` is presentation only (`--color=no --curses=no --noshow_progress --noshow_loading_progress --test_output=errors --test_summary=terse --verbose_failures`), so it never discards the analysis cache for a human and an agent sharing a worktree (`[DOC]` the iteration-speed page lists changed flags as a discard cause; that presentation flags do not discard is `[INF]`, checked in Task 0.2g's allow-list and by the second verify line below); `--config=ci` (`common:ci --lockfile_mode=error`; to disable the disk cache in a config use the empty-string form `--disk_cache=`; `--nodisk_cache` is not used; `[RUN]` the S2/S4/S11 spike builds ran with `--disk_cache=`), `--config=dev`, `try-import %workspace%/user.bazelrc`; `.bazelrc.user.example`; add `user.bazelrc` and the ANCHORED `/bazel-*` (root-level output symlinks only) to `.gitignore`; an unanchored `bazel-*` would silently ignore planned tracked files such as `.github/workflows/bazel-parity.yml` and `scripts/bazel-manual-instance.sh`. Verify each flag name against `bazel help build` on 9.3.1 and record deviations in `spike-results.md`. Flags that Phase 4 flagged UNVERIFIED must each get a `spike-verified` row (the `bazel help` line quoted) before they enter a committed config: the 20 flags `--remote_timeout`, `--remote_retries`, `--experimental_circuit_breaker_strategy`, `--experimental_remote_failure_rate_threshold`, `--experimental_remote_failure_window_interval`, `--remote_download_toplevel`, `--remote_download_minimal`, `--experimental_disk_cache_gc_max_size`, `--experimental_disk_cache_gc_max_age`, `--experimental_disk_cache_gc_idle_delay`, `--incompatible_enable_proto_toolchain_resolution`, `--repo_contents_cache`, `--repo_contents_cache_gc_max_age`, `--shutdown_on_low_sys_mem`, `--io_nice_level`, `--batch_cpu_scheduling`, `--local_resources`, `--incompatible_remote_local_fallback_for_remote_cache`, `--remote_cache_compression` and `--experimental_remote_cache_eviction_retries`, plus a recorded "absent on 9.3.1" line for each of `--local_ram_resources`, `--local_cpu_resources`, `--incompatible_sandbox_hermetic_tmp` and `--experimental_guard_against_concurrent_changes`; a flag that does not exist on 9.3.1 is dropped and the gap recorded, never kept on faith. Cache configs (identical failure-policy lines in `ci-cache-read` and `ci-cache-write`, checked by `check-bazelrc.sh`): `build:ci-cache-read --remote_timeout=3s --remote_retries=1 --experimental_circuit_breaker_strategy=failure --experimental_remote_failure_rate_threshold=10 --experimental_remote_failure_window_interval=60s --remote_cache_compression --remote_download_toplevel`. The circuit breaker is OFF until the strategy flag is set (`[RUN]` default unset). `--remote_local_fallback` is deliberately absent: it defaults to false and does not apply to `--remote_cache` unless `--incompatible_remote_local_fallback_for_remote_cache` is also set, while the docs say a cacheable local build does not fail when it cannot cache, so the failure policy is timeout, retries and breaker (Task 0.12c measures both variants). `--remote_download_toplevel` is already the 9.3.1 default (`toplevel`); it stays for explicitness (`minimal` for test-only jobs). Compression is for the CI/WAN path only (bazel-remote serves zstd over HTTP), not the local disk cache. Cache location and quota: `--disk_cache` and `--repository_cache` stay under `~/.cache/stapler-squad-bazel/` (never under `~/.stapler-squad/`, which holds the live service's SQLite state) with the verified GC size flag capped at 100 GB; both directories must sit on the same filesystem as the Bazel output bases (`stat -f -c %T` of the cache dir equals that of `bazel info output_base`; on this host's btrfs `/home` cache hits are reflinks and cost no extra disk, `[RUN]`; never place the cache on tmpfs `/tmp`); `.gitignore` also gets the anchored `/.bazel-out`; the `bazel-remote` data directory and quota are set in Task 0.12a.
- Files: `.bazelrc`, `.bazelrc.user.example`, `.gitignore`
- Verify: `bazel info --config=agent workspace 2>&1 | tail -1` prints the repo path with no flag errors; `{ grep -E '^startup:' .bazelrc || true; } | grep -vcE '^startup:(linux|macos|windows|freebsd|openbsd)'` prints `0`; after `bazel build //tools/lint/...`, `bazel build --config=agent //tools/lint/... 2>&1 | grep -c 'discarding analysis cache'` prints `0` (INFERRED that presentation flags keep the analysis cache; the result is recorded either way). Also `git check-ignore .github/workflows/bazel-parity.yml scripts/bazel-manual-instance.sh .claude/rules/bazel-build-files.md docs/reference/bazel-forks.md; echo $?` prints `1` (nothing ignored), and `mkdir -p bazel-bin && git check-ignore bazel-bin` prints `bazel-bin`.

##### Task 0.2c: Gazelle for the three small modules (cost M)
- Run `bazel run //:gazelle` limited to `tools/lint`, `tools/scanner`, `tuitest` (separate prefixes via per-directory `# gazelle:prefix`), commit the BUILD files, make `bazel test //tools/lint/...` pass; each failing analysistest fixture (cwd-sensitive `testdata/`) is listed in `tools/bzl/test-exclusions.yaml` with a `SandboxClass`, owner and issue, so the verdict is boolean: PASS iff `bazel test //tools/lint/...` exits 0 with at most the listed exclusions. Also confirm `bazel mod graph` shows `golang.org/x/tools` resolved separately for the isolated `tools/lint` usage and the root module.
- Files: `tools/lint/**/BUILD.bazel`, `tools/scanner/**/BUILD.bazel`, `tuitest/BUILD.bazel`
- Verify: `bazel build //tools/lint/... //tools/scanner/... //tuitest/...` exits 0.

##### Task 0.2e: Tracked-path guard and CODEOWNERS (cost S)
- Create `tools/bzl/hand-build-files.txt` listing every planned HandBuildFile path, one per line as `path  # planned: <Task>` (a path whose file does not exist yet is checked only for the not-ignored, not-gitlinked and not-`.bazelignore`d properties, so Epic 0 needs no later task) (`tools/bzl/tmux/BUILD.bazel`, `tools/bzl/flags/BUILD.bazel`, `tools/bzl/windows/BUILD.bazel`, `tools/bzl/ent/BUILD.bazel`, `tools/bzl/proto/BUILD.bazel`, `proto/session/v1/BUILD.bazel`, `session/ent/BUILD.bazel`, `server/web/BUILD.bazel`, `web-app/BUILD.bazel`, `macos/BUILD.bazel`, `session/tmux/embed/BUILD.bazel`, `session/tymux/embed/BUILD.bazel`, plus three more that are hand-written in later tasks: `tools/bzl/BUILD.bazel  # planned: Task 2.3a`, `tools/bzl/bun/BUILD.bazel  # planned: Task 3.1a`, `tools/gazelle/BUILD.bazel  # planned: Task 2.3b, only if S11 chooses the extension`; 15 paths in all) and `check-build-paths.sh` (runs in `make ready` and `bazel.yml`). Create `.github/CODEOWNERS` now (not in Story 11.6) covering `project_plans/bazel-conversion/implementation/owner-attestations.yaml`, `tools/bzl/parity/**`, `.github/workflows/bazel-parity.yml`, `.github/workflows/bazel.yml`, `MODULE.bazel`, `.bazelrc`, `tools/bzl/**`, any `.bzl`, and the root `/BUILD.bazel` (it holds the `# gazelle:` directives, so it is never collapsed as generated; see Task 11.6a). CODEOWNERS review is enforced only if the owner enables protection (D-PROTECT-MERGE), so the attestation gate does not rely on it (Task 7.5c uses an environment approval).
- Generated-tree coverage: `check-build-paths.sh` has a second mode, `--full` (off in Epic 0, switched on by Task 1.1a, the first task that runs Gazelle repo-wide), for Gazelle-generated BUILD files under ignored trees (`gen/`, `web-app/src/gen/`, `session/ent/*` non-schema). Mechanism: root `BUILD.bazel` carries `# gazelle:exclude gen`, `# gazelle:exclude web-app/src/gen` and an exclude for the ent entity subdirectories (the S11-selected form, Task 0.11), so Gazelle never writes BUILD files there; the check enumerates BUILD files that exist on disk but are ignored (`git ls-files --others --ignored --exclude-standard -- '*BUILD' '*BUILD.bazel'`) and every package Bazel loads (`bazel query --output=package //...`, each `<pkg>/BUILD.bazel` or `<pkg>/BUILD` must be git-tracked via `git ls-files --error-unmatch`), and fails naming the first path that is ignored or untracked. Run it after `bazel run //:gazelle` on a tree with a synced GenSurface so a leaked BUILD file in a generated dir is caught, not just the ~12 hand-written ones.
- Files: `tools/bzl/check-build-paths.sh`, `tools/bzl/hand-build-files.txt`, `.github/CODEOWNERS`
- Verify: `bash tools/bzl/check-build-paths.sh; echo $?` prints `0`; from Task 1.1a (`--full` on), after `make proto-gen ent-gen` and `bazel run //:gazelle`, `bash tools/bzl/check-build-paths.sh --full; echo $?` prints `0` and `git ls-files --others --ignored --exclude-standard -- '*BUILD' '*BUILD.bazel' | wc -l` prints `0` and the script still exits `0`; `git check-ignore $(cat tools/bzl/hand-build-files.txt)` prints nothing; `gh api repos/tstapler/stapler-squad/codeowners/errors --jq '.errors|length'` prints `0`.

##### Task 0.2d: Go SDK 1.26.6 resolution (cost XS)
- Confirm `go_sdk.from_file` downloads 1.26.6; if not, record the fallback in `spike-results.md` and stop for owner approval before touching `go.mod`.
- Files: `spike-results.md`
- Verify: `bazel run @rules_go//go -- version` prints `go version go1.26.6`.

##### Task 0.2f: Host-wide Bazel containment, `tools/bazel` wrapper (cost S)
- Pre-mortem #1 (host exhaustion; `onyx` is the dev box, the live service and every worktree at once). Add `tools/bazel` (Bazelisk runs it automatically and passes the real binary path in `BAZEL_REAL`; `[RUN]` in Epic 0 it cannot be invoked directly: without `BAZEL_REAL` it must print a clear message and exit non-zero, and manual runs use `BAZEL_REAL=$(command -v bazelisk) tools/bazel ...`; executable, shellcheck-clean). Behaviour: acquire one of N=2 `flock` slots under `${XDG_RUNTIME_DIR}/stapler-squad-bazel.lock.{1,2}` (queue; print "waiting for a Bazel slot" once), then `exec systemd-run --user --scope --quiet -p MemoryMax=14G -p CPUWeight=20 -p IOWeight=20 -p OOMScoreAdjust=500 nice -n 10 ionice -c3 "$BAZEL_REAL" "$@"` (`OOMScoreAdjust=500` makes Bazel the OOM killer's first victim; raising the score needs no privilege). Startup flags that differ by caller are passed by the wrapper on the command line, before the command, never through a config (`startup:agent` is ignored, D1): for agent/non-TTY callers `--max_idle_secs=30`; for every caller `--host_jvm_args=-Xmx2g`, because the owner's `$HOME/.bazelrc` sets an 8g heap and, rc files being read in order (system, workspace, home), the home file overrides the committed 2g, which with N servers defeats the 14 GB scope cap (`[DOC]` rc order; the command line has the highest precedence; that two `-Xmx` arguments resolve last-wins is `[INF]`, checked in the verify line). `--max_idle_secs=30` must stay above the disk-cache GC idle delay of Task 0.2b (10 s). Different startup options between two callers in one worktree may restart the server and discard the analysis cache (`[DOC]` iteration-speed page; whether `max_idle_secs` alone triggers it is `[INF]`): the verify line checks that the server pid survives an agent call followed by a human call, and if it does not the wrapper uses 30 s for all wrapper callers. Containment is partly declarative now (`[RUN]` startup help): `.bazelrc` carries `--io_nice_level=7`, `--batch_cpu_scheduling` and `--shutdown_on_low_sys_mem` (Task 0.2b) so a bypass via `bazelisk` is still contained; whether `--experimental_run_in_user_cgroup` can replace the manual `systemd-run --scope` is evaluated in Task 0.10g (E7), not assumed. **S6 fallback mode** (`BAZEL_WORKTREE_OUTPUT_BASE=1`, used only if Story 0.10 selects it): the wrapper passes an absolute `--output_base="$PWD/.bazel-out"`, because `%workspace%` is NOT expanded in startup flags (`[RUN]` it created a literal `%workspace%/` directory and failed), and the directory must be in `.bazelignore` (Task 0.2a); Bazel leaves its outputs read-only, so deleting it needs `chmod -R u+w .bazel-out` first (`[RUN]`). Queue rationale: there is no cross-server in-flight action dedup (`[RUN]` three servers started on one cold cache all executed the same actions), so a cold concurrent start costs CPU roughly equal to the concurrency; serialising through the N=2 slots is therefore also a cache-efficiency measure because later builds hit what the first wrote (measured as E11, Task 0.10g). The 12 GB and 14 GB figures and the slot count N=2 are proposals that Story 0.10 validates, not measurements.
- **Portability and fallbacks (triad iteration 2).** The script is POSIX sh / bash 3.2 compatible (macOS ships bash 3.2) and detects each tool: `flock` present -> fd-lock slots; absent (stock macOS) -> atomic `mkdir ${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/stapler-squad-bazel.slot.{1,2}` locks holding the PID, reclaimed when `kill -0` fails; `ionice` absent -> skipped; `systemd-run --user` is used only when `systemctl --user show-environment` exits 0 and neither `CI` nor `GITHUB_ACTIONS` is set, otherwise plain `nice -n 10` (plus `taskpolicy -b` on macOS when present, INFERRED, verified on the macOS runner in Task 5.1a); when `CI` or `GITHUB_ACTIONS` is set (single-tenant runner) the slot semaphore and containment are skipped and the script only `exec`s `$BAZEL_REAL`. `BAZEL_WRAPPER_NO_FLOCK=1` forces the mkdir branch so it is testable on Linux.
- Files: `tools/bazel`, `tools/bzl/wrapper_test.sh`, `.bazelrc`
- Verify: `bash tools/bzl/wrapper_test.sh` starts 6 invocations, asserts at most 2 run concurrently, and that `systemctl --user show` of the scope reports the `MemoryMax` and `OOMScoreAdjust=500`; it repeats the concurrency assertion with `BAZEL_WRAPPER_NO_FLOCK=1` (mkdir branch) and asserts `CI=true` bypasses the semaphore (the same file runs the mkdir and CI cases on the macOS runner, Task 5.1a); `shellcheck tools/bazel` exits 0 and `bash -n` passes under bash 3.2 syntax (`checkbashisms tools/bazel` prints nothing); with `HOME` pointing at a temp dir whose `.bazelrc` sets `startup --host_jvm_args=-Xmx8g`, the running server's `/proc/<pid>/cmdline` shows `-Xmx2g` as the last `-Xmx`, and `bazel build --config=agent` then `BAZEL_HUMAN=1 bazel build` leaves `bazel info server_pid` unchanged (or the recorded fallback applies); kill-mid-run slot release: `wrapper_test.sh` starts a wrapped synthetic run holding a slot, kills it with `kill -9` (and separately `kill -TERM`), and asserts a new invocation acquires that slot within 3 s in both the flock and the `BAZEL_WRAPPER_NO_FLOCK=1` (mkdir, reclaimed by `kill -0`) branches; with `BAZEL_WORKTREE_OUTPUT_BASE=1` `bazel info output_base` prints `$PWD/.bazel-out` and `bazel query //...` succeeds (fails without the `.bazelignore` entry). The live unit's early protection is Task 0.2i, its final `OOMScoreAdjust=-900` is Task 9.1b, and the orphan-output-base timer is Task 11.4b.

##### Task 0.2g: No-WORKSPACE and `.bazelrc` guard checks (cost XS)
- Validation G5 and pre-mortem #3/#13. `tools/bzl/check-bazelrc.sh` (run from `make ready` and `bazel.yml`) fails on: any `WORKSPACE`, `WORKSPACE.bazel` or `--enable_workspace`/`--enable_bzlmod=false` (`bazel mod graph` must also succeed); `flaky = True` in any BUILD file and `--flaky_test_attempts` greater than 1; `--spawn_strategy=local` in the committed `.bazelrc`; `--remote_executor`/`--remote_exec` (no RBE); `--remote_header` carrying a secret; and any flag that has no `spike-verified` row in `spike-results.md` once Task 0.2b has run. Docs-derived additions (D1, D20, D21, E9): it rejects any `startup:<config>` line whose config is not a platform config (`linux`, `macos`, `windows`, `freebsd`, `openbsd`), because Bazel ignores it silently; any flag in a committed rc file that does not appear in `bazel help <command> --long` for the version in `.bazelversion` (the check runs `bazel help` once per command and caches it in `.cache/`); the four known-absent flags (`--local_ram_resources`, `--local_cpu_resources`, `--incompatible_sandbox_hermetic_tmp`, `--experimental_guard_against_concurrent_changes`); `--digest_function` (a startup option every cache sharer must leave at the default; changing it invalidates the shared cache, `[INF]`); `--action_env=PATH` (`[RUN]` it changes the action key); `--profile=<path>` (disables Bazel's automatic profile GC, `[DOC]`); `--remote_local_fallback` without `--incompatible_remote_local_fallback_for_remote_cache` (a no-op for a cache-only setup); a `ci-cache-read`/`ci-cache-write` pair whose failure-policy lines differ; and any flag in `build:agent`/`test:agent` outside the presentation allow-list (color, curses, progress, `verbose_failures`, `test_output`, `test_summary`).
- Files: `tools/bzl/check-bazelrc.sh`, `tools/bzl/check-bazelrc_test.sh`
- Verify: `bash tools/bzl/check-bazelrc_test.sh` exits 0 (one seeded violation per rule, each must make the script exit 1; experiment E9: a fixture with `startup:agent --max_idle_secs=30` fails and a fixture with `startup:linux --max_idle_secs=30` passes); `test ! -e WORKSPACE && test ! -e WORKSPACE.bazel` exits 0.

##### Task 0.2h: Never-force-push and Conventional Commit guardrails (cost XS)
- Validation G4 (repo rules C7/C8 had only process, no check). `tools/bzl/check-no-force-push.sh` greps `.github`, `scripts`, `tools/bazel`, `tools/bzl` and `tools/ci` for `push (-f|--force|--force-with-lease)` and `+refs` refspecs and is called from `check-workflows.sh` and `make ready`. `scripts/hooks/pre-push-no-force.sh` is a git `pre-push` hook that reads the `<local ref> <local sha> <remote ref> <remote sha>` lines and rejects any update where the remote sha is not an ancestor of the local sha (`git merge-base --is-ancestor`), and any deletion of `main`; it is installed by `make setup-git`, a target this task creates (idempotent, per clone and not versioned: it points `core.hooksPath` at `scripts/hooks`; Task 0.10b registers the merge drivers in the same target and Task 11.2a hardens it), so the hook exists from Epic 0 and the CI grep is the second control. `tools/bzl/check-pr-title.sh` asserts the PR title matches `^(feat|fix|chore|docs|refactor|test)(\(.+\))?!?: ` and runs on `pull_request` events in `bazel.yml`.
- Files: `tools/bzl/check-no-force-push.sh`, `scripts/hooks/pre-push-no-force.sh`, `scripts/hooks/pre-push-no-force_test.sh`, `tools/bzl/check-pr-title.sh`, `Makefile`
- Verify: `bash scripts/hooks/pre-push-no-force_test.sh` exits 0 (temp repos: fast-forward accepted, rewritten history rejected); `bash tools/bzl/check-no-force-push.sh; echo $?` prints `0`; `bash tools/bzl/check-pr-title.sh 'feat(bazel): x'` exits 0 and `'Update stuff'` exits 1; `make -n setup-git | grep -c 'core.hooksPath'` prints at least `1`.

##### Task 0.2i: Early live-service OOM protection drop-in (owner-run) (cost XS)
- Pre-mortem #1 residual: until Task 9.1b the live `:8543` service has no OOM protection through Epics 0-8, while Bazel runs beside it from Story 0.2. Write `docs/how-to/protect-live-service-from-bazel.md` with a systemd user drop-in `~/.config/systemd/user/<unit>.d/oom-protect.conf` (`[Service]` / `OOMScoreAdjust=-900`; unit name per `docs/how-to/manage-systemd-service.md`), `systemctl --user daemon-reload` and the restart that applies it (tmux sessions survive because of `--tmux-keep-server`; the owner picks the moment). A negative `OOMScoreAdjust` may be rejected for an unprivileged user manager (INFERRED); the page gives the system-level fallback (`sudo systemctl edit`), and in every case the Bazel scope already carries `OOMScoreAdjust=500` (Task 0.2f). Owner-run; agents write the page and the check script only.
- Files: `docs/how-to/protect-live-service-from-bazel.md`, `scripts/check-live-oom-protect.sh`
- Verify: `bash scripts/check-live-oom-protect.sh; echo $?` prints `0` when `systemctl --user show <unit> -p OOMScoreAdjust --value` is at most `-500`, and `3` with `OWNER ACTION` before the owner acts; `grep -c 'OOMScoreAdjust=500' tools/bazel` prints at least `1`.

#### Story 0.3: Spike S1 - proto Go + connect-go + TS through Bazel
**As an** implementer, **I want** to know whether `go_proto_library` can reproduce `buf generate` output exactly, **so that** Epic 2 uses the right mechanism. (The TypeScript half is spike S1-ts in Story 0.6, Task 0.6e, because it needs the Bun install.)
**Acceptance Criteria**:
- Go and connect-go output matches.
  - *Given* `proto/session/v1/session.proto` and connect-go v1.20.0 built from `go_deps`, *When* the spike target builds, *Then* the generated files declare import path `github.com/tstapler/stapler-squad/gen/proto/go/session/v1` and `diff` against `make proto-gen` output after `gofmt` is empty.
- No network or BSR dependency.
  - *Given* root `buf.yaml` declares `deps: buf.build/googleapis/googleapis` and no `buf.lock` exists (VERIFIED: `ls buf.lock proto/buf.lock` fails), *When* the proto actions run with `--sandbox_default_allow_network=false`, *Then* they succeed because the dependency was either removed (unused: `git grep googleapis -- proto` is empty) or vendored via `buf.lock` (Task 0.3e).
- The verdict is recorded.
  - *Given* the checks above, *When* the story ends, *Then* `spike-results.yaml` has an `S1` row with one of the four `SpikeVerdict` values.
**Files**: spike branch only; `spike-results.md`

##### Task 0.3a: go_proto_library with a connect-go compiler (cost M)
- On a spike branch add `proto_library`, `go_proto_library` with `compilers` including a `go_proto_compiler` wrapping `protoc-gen-connect-go` from `connectrpc.com/connect` (v1.20.0 pin per `buf.gen.yaml` comment); reproduce managed-mode `go_package_prefix` via `importpath`/`M` options. **Epic 0 correction (S1, verified by run):** load `proto_library` from `@protobuf//bazel:proto_library.bzl` (protobuf 36.2 under bzlmod), not from `@rules_proto`. This route was abandoned: the byte-compare against `buf generate` failed; Epic 1 deletes the attempt (owner decision pending) because it makes Gazelle report "matches multiple rules".
- Files: spike `proto/session/v1/BUILD.bazel`, spike `tools/bzl/proto/BUILD.bazel`
- Verify: `bazel build //proto/session/v1:all` exits 0; output file list printed by `bazel cquery --output=files`.

##### Task 0.3b: Byte-compare against make output (cost S)
- Write `tools/bzl/parity/compare-proto.sh` (kept for P4): run `make proto-gen` into a temp copy, `gofmt` both, `diff -r`.
- Files: `tools/bzl/parity/compare-proto.sh`
- Verify: `bash tools/bzl/parity/compare-proto.sh` exits 0. PASS iff exit 0 with an empty diff; any non-empty diff is `FallbackWorkaround` (Task 0.3d) unless every delta line is in `tools/bzl/parity/allowed-deltas.txt`.

##### Task 0.3e: Resolve the buf.yaml BSR dependency (cost S)
- Run `make proto-gen` with network disabled (`unshare -rn` or equivalent) to observe how the dependency resolves today; if unused (`git grep googleapis -- proto` finds nothing), remove the `deps:` entry from `buf.yaml` in an inert PR that changes no generated output; otherwise vendor with `buf.lock` and `rules_buf` `buf_dependencies`.
- Files: `buf.yaml`, `buf.lock` (only if vendored), `spike-results.md`
- Verify: `make proto-gen` and `buf lint` exit 0 with no network and `git diff --stat -- gen web-app/src/gen` is empty.

##### Task 0.3d: Fallback W (only if 0.3a or 0.3b fails) (cost M)
- Implement `buf generate` as `run_binary` using a `buf.gen.bazel.yaml` with `local:` plugins built by Bazel; re-run 0.3b.
- Files: spike `buf.gen.bazel.yaml`, spike `proto/session/v1/BUILD.bazel`
- Verify: `bash tools/bzl/parity/compare-proto.sh` exits 0.

##### Task 0.3f: Prebuilt protoc (cost S; **resolved by Epic 0: `toolchains_protoc` is not needed**)
- Pre-mortem #4 (cold CI floor): without a prebuilt protoc, a free 2-4 core runner compiles protobuf from source on every cache miss. Add `toolchains_protoc` to the spike `MODULE.bazel` with `--incompatible_enable_proto_toolchain_resolution` (flag verified in Task 0.2b) and point the proto actions at the prebuilt `protoc` for linux amd64/arm64 and darwin. **Epic 0 result (S1, `spike-results.md`):** on Bazel 9.3.1 + protobuf 36.2 the module already resolves a prebuilt protoc (`prebuilt_protoc.linux_x86_64`); a cold build of the proto targets compiled no protobuf C++ (0 `CppCompile`, 0 upb/protobuf/src lines). Do not add `toolchains_protoc`; keep only the check that the cold build has no protobuf compile actions.
- Files: spike `MODULE.bazel`, spike `.bazelrc`, `spike-results.md`
- Verify: on a cold runner (empty remote cache and empty disk cache) `bazel build //proto/... --config=ci 2>&1 | tee log; grep -c 'Compiling .*upb\|protobuf/src' log` prints `0`; if the toolchain or flag does not exist on 9.3.1 the gap is recorded and the cold row of P10 carries the compile cost.

#### Story 0.4: Spike S2 - ent generation as a Bazel action
**As an** implementer, **I want** a working recipe for `ent generate --feature sql/upsert` under Bazel with a variable output file set, **so that** `//session/ent` compiles from a clean clone.
**Acceptance Criteria**:
- Clean-clone build works for every ent package.
  - *Given* a clone with only `session/ent/schema/` tracked (VERIFIED: no `generate.go` is tracked), *When* `bazel build //session/ent/... //tools/bzl/ent/...` runs (the generated libraries are `//tools/bzl/ent:*`; `//session/ent/...` only builds `schema`) with network disabled for actions (`--sandbox_default_allow_network=false`), *Then* exit 0, one `go_library` exists per package in `go list ./session/ent/...` (53 today), the library exports the `sql/upsert` builders (`grep -rc OnConflict bazel-bin/tools/bzl/ent | grep -v :0 | head -1` prints a file; no `UpsertRule` symbol exists), and `bazel build //session/... //server/...` resolves every importer of every ent subpackage.
- Output is deterministic.
  - *Given* two clean output bases, *When* the ent action runs in each, *Then* `sha256sum` of the output trees are equal.
- Gazelle handling is decided.
  - *Given* the importers of `session/ent` and its entity subpackages, *When* `gazelle:resolve` lines are counted, *Then* the S11 threshold (more than 8 lines or any per-entity line means extension) is applied and recorded.
**Files**: spike branch only; `spike-results.md`

##### Task 0.4a: ent CLI as go_binary and generation action (cost M)
- Build `entgo.io/ent/cmd/ent` from `go_deps`; read the flag list from the single source `tools/bzl/ent/ent_flags.txt` (`--feature sql/upsert`, also read by `make ent-gen` so the two cannot diverge) and run `generate $(cat ent_flags.txt) ./session/ent/schema` with the SDK `go` on PATH and module data provided as declared inputs (observe `packages.Load` behaviour; document what it needs). Prefer a small `entc` program importing the schema as a normal dependency.
- Files: spike `session/ent/BUILD.bazel`, spike `tools/bzl/ent/entc/main.go`
- Verify: `bazel build //session/ent:ent_gen` exits 0.

##### Task 0.4b: Tree output to many go_libraries (cost M)
- ent generates 53 Go packages, so one tree-output rule cannot yield one `//session/ent`. Implement a macro in `tools/bzl/ent.bzl` that loads a generated package list (Starlark cannot read files at load time) `tools/bzl/ent/packages.bzl` and instantiates, in the tracked `tools/bzl/ent/BUILD.bazel`, one `go_library` per package (explicit `importpath`, `srcs` from a per-package file group over the generation tree). The list is guarded by a freshness test that builds the tree and compares its directory listing with `packages.bzl` (the generator's `-expect` check) (fails with `Run: bazel run //tools/bzl/ent:update_packages`). Alternative if rules_go rejects directory-derived `srcs`: a custom rule emitting `GoSource` per package (ADR-008 workaround).
- Files: spike `tools/bzl/ent.bzl`, spike `tools/bzl/ent/BUILD.bazel`, spike `tools/bzl/ent/packages.bzl`
- Verify: `bazel build //tools/bzl/ent:all` exits 0 and `bazel query 'kind(go_library, //tools/bzl/ent:all)' | wc -l` equals `go list ./session/ent/... | grep -vc /schema`.

##### Task 0.4c: Determinism and resolve-count check (cost XS)
- Build twice with `--output_base` set to two temp dirs and compare hashes; count the `gazelle:resolve`/`resolve_regexp` lines needed for `session/ent` and its subpackages (target: one `resolve_regexp go ^github.com/tstapler/stapler-squad/session/ent(/(.*))?$ //tools/bzl/ent:$2` style line if Gazelle substitutes capture groups; verified in Task 0.11a); record.
- Files: `spike-results.md`
- Verify: `diff <(cd "$A" && find . -type f | sort | xargs sha256sum) <(cd "$B" && find . -type f | sort | xargs sha256sum); echo $?` prints `0` (`$A` and `$B` are the two `--output_base` generated trees; paths are included so a renamed file fails); `grep -cE '^S2 resolve-lines: [0-9]+$' spike-results.md` prints `1`.

#### Story 0.5: Spike S3 - embed from a tree artifact
**As an** implementer, **I want** `server/web` to embed a `dist/` produced by another package, **so that** the Go source can stay unchanged (ADR-007).
**Acceptance Criteria**:
- Staged tree embeds.
  - *Given* a fixture tree `dist/index.html` and `dist/assets/app.abc123.js` produced by a genrule, *When* `server/web`'s `go_library` takes it via `copy_directory` staging, *Then* a `go_test` reading `Dist.ReadFile("dist/index.html")` returns the fixture bytes.
**Files**: spike branch only; `spike-results.md`

##### Task 0.5a: copy_directory staging plus embedsrcs (cost S)
- Use `bazel_lib` `copy_directory` to stage at `server/web/dist`, pass as `embedsrcs`; run the existing `server/web` tests.
- Files: spike `server/web/BUILD.bazel`
- Verify: `bazel test //server/web:all` exits 0.

##### Task 0.5b: Cost both fallbacks if 0.5a fails (cost S)
- Prototype the tree-expansion custom rule; estimate rules_go directory-`embedsrcs` patch size; record classes per ADR-008.
- Files: `spike-results.md`
- Verify: `grep -cE '^S3 (tree-expansion-cost|embedsrcs-patch-cost): (XS|S|M|L)$' spike-results.md` prints `2` and `grep -cE '^S3 chosen-path: [a-z-]+$' spike-results.md` prints `1` (the task writes these three lines).

#### Story 0.6: Spike S4 - Bun web build under Bazel; fork-versus-workaround costing
**As an** implementer, **I want** the web build as a Bazel action using the single `bun.lock`, **so that** ADR-003 is confirmed or replaced.
**Acceptance Criteria**:
- Output parity.
  - *Given* `web-app/bun.lock` and `bun@1.4.3`, *When* `bazel build //web-app:dist` runs, *Then* `diff <(cd bazel-bin/web-app/dist && find . -type f | sort | xargs sha256sum) <(cd server/web/dist && ...)` from `make web-build` is empty, or every delta line is in `tools/bzl/parity/allowed-deltas.txt` (pre-approved: minifier banner/version comment only); any other delta fails the repository-rule path and moves S4 to its `FallbackWorkaround` path, `rules_bun` (Task 0.6f).
- No hidden coupling.
  - *Given* `web-app/build/build.ts`, *When* the action runs in the sandbox, *Then* it does not read outside `web-app/`, does not write into the source tree, and needs network only in the repository rule (`strace`/`--sandbox_debug` evidence).
- Sandbox cost is bounded.
  - *Given* the real `node_modules` (tens of thousands of files) as inputs, *When* the sandboxed `dist` action runs, *Then* its wall time is at most 1.5x `make web-build` wall time and sandbox setup time is recorded; above that the consuming actions are tagged `no-sandbox` and the verdict stays `Pass` with `path` = `repo-rule+no-sandbox` (a recorded cost, not a downgrade).
- Jest runs.
  - *Given* the same install, *When* `bazel test //web-app:jest` runs on a 20-test subset, *Then* the pass set equals `bunx jest` for those tests.
- Proto TS generation (spike S1-ts) works from the same install.
  - *Given* `protoc-gen-es` from the Bun install built in Task 0.6a, *When* the TS action of Task 0.6e runs, *Then* `diff -r` of its output tree against `web-app/src/gen` from `make proto-gen` is empty (verdict row `S1-ts` in `spike-results.yaml`).
- The fork alternative is costed.
  - *Given* the rules_js `npm_translate_lock` source, *When* a `bun.lock` reader is prototyped for 10 packages, *Then* `spike-results.md` states lines of patch, cost class and per-bump maintenance class.
**Files**: spike branch only; `spike-results.md`

##### Task 0.6a: Bun repository rule and dist action (cost M)
- Prototype `tools/bzl/bun/bun.bzl` repository rule (pinned Bun via `http_archive`, `bun install --frozen-lockfile`) and a `dist` action running `bun build/build.ts`.
- Files: spike `tools/bzl/bun/bun.bzl`, spike `web-app/BUILD.bazel`, spike `MODULE.bazel`
- Verify: `bazel build //web-app:dist` exits 0.

##### Task 0.6b: Probe build.ts and install coupling (cost S)
- Record every path read/written by `build/build.ts` (UNVERIFIED in research) using sandbox debug; list network use.
- Files: `spike-results.md`, `tools/bzl/parity/allowed-deltas.txt`
- Verify: PASS iff `strace`/`--sandbox_debug` evidence yields exactly 0 reads outside `web-app/` and 0 writes into the source tree (`bash tools/bzl/spike/count-escapes.sh` prints `0 0`); any nonzero count fails the repository-rule path (offending paths listed) and moves S4 to Task 0.6f.

##### Task 0.6c: Jest as a test target (cost S)
- Add `sh_test`/`bun_test` target for a 20-test subset using the installed tree as data.
- Files: spike `web-app/BUILD.bazel`
- Verify: `bazel test //web-app:jest_subset` exits 0 and `bazel-testlogs/web-app/jest_subset/test.log` lists 20 passing tests.

##### Task 0.6e: Proto TS generation action (spike S1-ts) (cost S)
- Moved from Story 0.3 because `protoc-gen-es` is built by the Bun repository rule of Task 0.6a. Add a `run_binary` running `protoc` (from the `protobuf` module) with `protoc-gen-es` from the web install into a declared tree.
- Files: spike `web-app/BUILD.bazel`, `spike-results.yaml`
- Verify: `diff -r bazel-bin/web-app/gen web-app/src/gen` exits 0 (PASS iff exit 0, otherwise the "If S4 fails" consequences in the spike protocol apply).

##### Task 0.6d: Fork-versus-workaround costing for bun.lock (cost M)
- Read `aspect_rules_js` `npm_translate_lock` and prototype a `bun.lock` -> internal lock model reader; record patch size, upstream issue link (or open one), and per-bump rebase class; compare with 0.6a measured cost; this costing also feeds the `FallbackFork` branch of Task 0.6f.
- Files: `spike-results.md`
- Verify: `grep -cE '^S4 (workaround-class|fork-class|chosen-path): [A-Za-z-]+$' spike-results.md` prints `3` (the task writes these three lines under the S4 table; `fork-class` and `workaround-class` are ADR-008 classes).

##### Task 0.6f: Fallback W for S4, pinned `rules_bun` (only if Task 0.6a or 0.6b fails the repository rule) (cost M)
- This is the defined, reachable fallback from S4's failure (ADR-003 vocabulary: repository rule = `Pass`; `rules_bun` = `FallbackWorkaround`; aspect_rules_js fork = `FallbackFork`). Add a pinned third-party `rules_bun` via `git_override`, re-run the S4 parity, escape-count and Jest checks of Tasks 0.6a-0.6c against it, and record its bus-factor risk. If it fails the same checks, or per-package granularity is required, the verdict becomes `FallbackFork` using the Task 0.6d costing; if the fork is infeasible within ADR-008's cap the verdict is `Fail` and the "If S4 ends `Fail`" consequences and D-HERMETIC apply.
- Files: spike `MODULE.bazel`, `spike-results.md`
- Verify: `bash tools/bzl/parity/compare-web.sh; echo $?` prints `0` against the `rules_bun` build, or the failing check is recorded and the next path is selected in `spike-results.yaml`.

#### Story 0.7: Spike S5 - tmux and tymuxd on bzlmod
**As an** implementer, **I want** a tmux build that works in a fresh worktree, **so that** the `embed_tmux` config can be hermetic enough and the `b51b60eb1` failure class is gone.
**Acceptance Criteria**:
- Out-of-tree build from a tracked path.
  - *Given* a clean clone where `third_party/tmux` is an empty directory (submodule not initialised), *When* `bazel build //tools/bzl/tmux:tmux_bin` runs, *Then* exit 0 and `bazel-bin/tools/bzl/tmux/tmux -V` prints `tmux 3.8`. The BUILD file is at the tracked path `tools/bzl/tmux/` because `third_party/tmux` is a submodule gitlink (VERIFIED `git ls-tree HEAD third_party/tmux` is mode 160000, so no file can be tracked under it) and is in `.bazelignore`.
- The submodule is a pin only.
  - *Given* the clean clone above, *When* `git ls-files third_party/tmux` runs, *Then* it prints only the gitlink, and `git check-ignore tools/bzl/tmux/BUILD.bazel` prints nothing.
- Pin consistency has a stated rule.
  - *Given* `tools/bzl/tmux/PIN` (tag `3.8`, tarball sha256, commit SHA the tag dereferences to, recorded once with `git ls-remote https://github.com/tmux/tmux.git 3.8^{}`), *When* `tools/ci/check-tymux-pin.sh` and a new pin test run, *Then* they pass iff the gitlink commit equals the `PIN` commit and the `MODULE.bazel` `http_archive` sha256 equals the `PIN` sha256 (the `.gitmodules` `branch = 3.8` line is a branch, not a pin, and is not trusted).
**Files**: spike branch only; `spike-results.md`

##### Task 0.7a: rules_foreign_cc 0.17.0 configure_make with a release tarball (cost M)
- `http_archive` tmux 3.8 release tarball with sha256 (ships generated `configure`; the submodule checkout is never a build input), libevent and ncurses as BCR modules or tarballs, `configure_make(out_binaries=["tmux"])`, static options per `docs/how-to/bundle-tmux.md`.
- Files: spike `MODULE.bazel`, spike `tools/bzl/tmux/BUILD.bazel`
- Verify: `bazel build //tools/bzl/tmux:tmux_bin` exits 0.

##### Task 0.7b: Genrule fallback (cost M)
- Out-of-tree genrule invoking an adapted `scripts/build-tmux.sh` with declared inputs; tag `no-remote-cache` until determinism is shown. Docs-derived (D22): the local disk cache has the same hole across a host toolchain change (`[DOC]` "Bazel does not track tools outside a workspace"; sandboxing does not hide the host), so after `pacman -Syu` a stale tmux built by the old host cc could be served from the local cache. Protect it by either tagging the action `no-cache` or declaring the host cc version as an input (a repository rule that records `cc --version` into a file the genrule lists in `srcs`); the second is `[INF]` and its check is: change the recorded version and the action re-executes.
- Files: spike `tools/bzl/tmux/BUILD.bazel`, spike `scripts/build-tmux-bazel.sh`
- Verify: `bazel build //tools/bzl/tmux:tmux_bin_genrule` exits 0 and `-V` prints `tmux 3.8`.

##### Task 0.7c: Pin reconciliation and tymuxd http_file (cost S)
- Read `scripts/fetch-tymuxd.sh`, `scripts/tymuxd-checksums.txt`, `tools/ci/check-tymux-pin.sh`; map to `http_file` sha256 per OS/arch. Decision (made here, not left open): the release tarball is the build source and `tools/bzl/tmux/PIN` is the single pin file; the submodule remains only as a version pin that the pin test compares against `PIN`. Document in ADR-007's consequences.
- Files: `spike-results.md`, `tools/bzl/tmux/PIN`
- Verify: `spike-results.md` S5 has the pin table and `bash tools/ci/check-tymux-pin.sh` exit code recorded; `awk` check that `PIN` has non-empty `tag`, `sha256` and `commit` fields.

#### Story 0.8: Spike S7 - rules_go flags and TagConfig composition
**As an** implementer, **I want** the exact flag names and composition behaviour on the pinned versions, **so that** `.bazelrc` configs are correct the first time.
**Acceptance Criteria**:
- Configs compose.
  - *Given* `--config=embed_tymux`, *When* `bazel cquery --config=embed_tymux --output=starlark ...` inspects `gotags`, *Then* both `embed_tmux` and `embed_tymux` are present.
- Race composes with pure Go.
  - *Given* `build:linux` sets `pure`, *When* `bazel build --config=linux --config=race //tools/lint/...` runs, *Then* it exits 0 because `build:race` sets `--@rules_go//go/config:pure=false` (INFERRED: rules_go treats race and pure as mutually exclusive; this criterion tests it). Race and non-race configs have different action keys, so the P1 and P2 runs do not share cache.
- Tag-gated files compile.
  - *Given* `--config=sinkguard`, *When* `bazel build --config=sinkguard //session/...` runs on the generated-independent subset, *Then* the 7 sinkguard-tagged files are in `srcs` of the compile action (`aquery`).
**Files**: spike `.bazelrc`; `spike-results.md`

##### Task 0.8a: Validate flag names and composition (cost S)
- For `go/config:tags`, `race`, `pure`, `static`, `msan` n/a, confirm names, that `--config` chains are NOT additive for `tags` (`[RUN]` S7: the last `tags=` wins; `--config=embed_tmux --config=sinkguard --config=gitoracle --config=integration` compiles only `integration`), so each TagConfig must list its full tag set in one `tags=` line, and Gazelle `# gazelle:build_tags integration,gitoracle,sinkguard,harness,live_github` need.
- Files: `spike-results.md`, spike `.bazelrc`
- Verify: `bazel aquery --config=race 'mnemonic("GoCompilePkg", //tools/lint/...)' | grep -c -- '-race'` is at least 1.

##### Task 0.8b: select() on a bool build_setting for embeds (cost S)
- Build `//tools/bzl/flags:embed_tmux` bool flag (package `tools/bzl/flags`, tracked; NOT `build/`, which is gitignored), `config_setting`, and a `select` switching `embedsrcs`.
- Files: spike `tools/bzl/flags/BUILD.bazel`, spike `session/tmux/BUILD.bazel`
- Verify: `bazel build --//tools/bzl/flags:embed_tmux=true //session/tmux` and `=false` both exit 0 on the subset with the tmux fixture.

#### Story 0.9: Spike S8/S9 - sandbox census and unix-socket path limit
**As an** implementer, **I want** a measured failure-class table, **so that** Epic 4 is batched by class and not discovered test by test.
**Acceptance Criteria**:
- Census.
  - *Given* Gazelle BUILD files for `session`, `session/tmux`, `server/services`, `session/vc`, `config`, `github`, `session/mux`, `session/tymux`, `tuitest` and one pure-logic package, and a throwaway `go_library` over `make proto-gen ent-gen` output for the generated imports, *When* `bazel test --sandbox_debug --test_output=errors` runs, *Then* each failing test is classified into a `SandboxClass` in a table with counts.
- Socket paths.
  - *Given* a test binding `os.TempDir()+"/ssq-1234.sock"`, *When* run under linux-sandbox with `--test_env=TMPDIR=/tmp` plus whichever mechanism the spike finds that gives the test a short private `/tmp`, *Then* it passes; without them the path length (`wc -c`) exceeds 108 and it fails. `--incompatible_sandbox_hermetic_tmp` does NOT exist on 9.3.1 (`[RUN]` 0 matches in `bazel help build --long`) and must not be used; candidate mechanisms (`--sandbox_tmpfs_path`, `--sandbox_add_mount_pair`, `--experimental_use_hermetic_linux_sandbox`, the sandbox default) are INFERRED to exist and each is confirmed against `bazel help build --long` before use. **Epic 0 result (S9):** passes with no flag; `--test_env=TMPDIR` is ignored and the private short `/tmp` is Bazel 9.3.1's default (`-M _hermetic_tmp -m /tmp`); the "without" arm is a bind under the long `TEST_TMPDIR` (169 bytes, `bind: invalid argument`).
**Files**: `spike-results.md`

##### Task 0.9a: Ten-package census (cost M)
- Generate BUILD files on the spike branch, seed generated deps from make output, run tests, tabulate `SandboxClass` counts. Also write the producers that Epic 4 depends on (nothing else produces them): `tools/bzl/spike/census.py` runs the given packages' tests, parses every `test.xml` and writes `tools/bzl/spike/census.json` as `{test, package, file, class}` rows; `tools/bzl/spike/census-check.py` reads `census.json` and the class table and takes `--class`, `--package`, `--files` and `--residual`, printing remaining counts and exiting 0 only when every failing test has exactly one class and the per-class sums equal the failure count.
- Files: `spike-results.md`, `tools/bzl/spike/census.py`, `tools/bzl/spike/census-check.py`, `tools/bzl/spike/census-check_test.py`, `tools/bzl/spike/census.json`
- Verify: PASS iff `python3 -m unittest tools/bzl/spike/census-check_test.py` exits 0 and `python3 tools/bzl/spike/census-check.py` exits 0, where it parses every `test.xml` under `bazel-testlogs`, counts failing test cases, and asserts each is assigned exactly one `SandboxClass` in the table and the per-class sum equals the failure count (a row count of 10 is only a format check and is not the criterion).

##### Task 0.9b: Socket path probe (cost S)
- Add a throwaway `go_test` binding a socket; run with and without TMPDIR flags; also run the sshremote/mux/tymux socket tests. Record in `spike-results.md` which `bazel help build --long` line backs each sandbox flag used, and that `--incompatible_sandbox_hermetic_tmp` is absent on 9.3.1.
- Files: spike `tools/bzl/spike/sock_test.go`, `spike-results.md`
- Verify: `bazel test --test_env=TMPDIR=/tmp //tools/bzl/spike:sock_test` exits 0; `bazel help build --long | grep -c incompatible_sandbox_hermetic_tmp` prints `0`.

#### Story 0.10: Spike S6 - worktree cost, memory and lock-file conflicts
**As the** owner, **I want** measured Bazel cost across concurrent worktrees, **so that** the worktree driver is evidence, not hope.
**Acceptance Criteria**:
- Shared caches pay off.
  - *Given* a warm shared `--disk_cache` from worktree 1, *When* worktree 2 builds `//:stapler-squad` (or the largest buildable target at that point) cold in 5 repeated measurements gated on `loadavg1 <= 2.0`, *Then* the median wall time is at most 30% of the `baseline-worktree.json` make median. This is the same definition as P10; the Bazel second-versus-first ratio is recorded as information only.
- Memory is bounded.
  - *Given* 3 worktrees building concurrently with `max_idle_secs=300` and `-Xmx2g`, *When* `ps -o rss` is sampled, *Then* peak combined RSS is at most 12 GB (host has 61 GB with about 34 GB in use).
- Live service is protected under load (pre-mortem #1).
  - *Given* 8 queued invocations through the `tools/bazel` wrapper (Task 0.2f) while a synthetic terminal stream runs against a manual instance (ports 62871/62872, never `:8543`), *Then* peak combined RSS is at most 12 GB and stream p99 latency is at most 1.2x idle; Story 0.14 cannot pass S6 otherwise.
- Shared disk cache is not corrupted by concurrency (validation G6).
  - *Given* 3 worktrees building `//...` into one shared `--disk_cache` simultaneously, twice, *When* a 4th worktree builds from that cache and its `bazel-bin` sha256 list is compared with a no-cache build, *Then* the lists are identical and no `command.log` contains `corrupt` or `digest mismatch` (Task 0.10c).
- Lockfile conflict handling is chosen.
  - *Given* two branches each adding a different Go dependency, *When* merged, *Then* the conflict in `MODULE.bazel.lock` is reproduced and the chosen resolution (merge driver regenerating the lock) is shown to produce a clean tree; spike S13 (Task 0.10d) additionally decides commit-the-lock versus not, from a 20-PR replay.
- Identical sources at different worktree paths share cache entries (docs-derived E1, E2; `[RUN]` shown for genrules, `[INF]` for rules_go).
  - *Given* worktree A at one path and worktree B at a different path and depth, same commit, an empty shared `--disk_cache`, *When* A builds `//:stapler-squad` cold and B (fresh output base) builds it, *Then* at least 95% of B's cacheable spawns in `--execution_log_json_file` are disk-cache hits and every other spawn is on the pre-declared allow-list; a third worktree C runs zero non-allow-listed `linux-sandbox`/`worker` spawns in at most 30% of A's wall time (Task 0.10e).
- Go compile actions are path-independent and outputs are deterministic across worktrees (E3, E4; central `[INF]`).
  - *Given* the same commit in two worktree paths, *When* the `aquery 'mnemonic("GoCompile", ...)'` command lines of three Go targets are compared and every `bazel-bin` regular file of two cache-disabled builds is sha256'd by relative path, *Then* the command lines are byte-identical with no absolute path, and the digest lists are identical with the positive-control `pwd` genrule flagged (Task 0.10e).
- Cache and repo layout preconditions hold (E5, E6, E10).
  - *Given* the shared cache and an output base, *When* Task 0.10f runs, *Then* both are on one filesystem and `filefrag -v` shows a `shared` extent between a restored output and its CAS blob (otherwise the disk term is re-measured as copy-based), GC trims the cache under `max_idle_secs=30` after the 10 s idle delay, and a fresh worktree builds the toolchain repos offline with each non-extension external repo a symlink into the repo contents cache.
- Containment alternatives and the cost of not queuing are measured (E7, E11).
  - *Given* 3 cold servers started at once versus 3 serialised through the wrapper, *When* CPU-seconds are summed from the profiles, *Then* the serialised total is recorded (expected at most 60% of concurrent) and the `--experimental_run_in_user_cgroup` versus `systemd-run --scope` comparison is recorded (Task 0.10g).
- Multiplex workers are inventoried (Task 0.10h).
  - *Given* the build graph, *When* `worker-inventory.sh` runs, *Then* it lists the mnemonics advertising worker support (or reports none found) and, if any, the RSS and wall-time comparison of multiplex versus `--noworker_multiplex`; non-gating.
**Files**: `spike-results.md`, spike `scripts/git-merge-bazel-lock.sh`, `tools/bzl/baseline/worker-inventory.sh`, `tools/bzl/baseline/cross-worktree-cache.sh`, `tools/bzl/baseline/cache-layout-checks.sh`, `tools/bzl/baseline/containment-checks.sh`

##### Task 0.10a: Five-worktree cost and RSS measurement (cost M)
- Script 5 temp worktrees, build one target each with shared caches, sample disk (`du -sk` of output bases; emit `output_base_disk_kb` and compare it with `baseline-worktree.json` `disk_kb_worktree`, bound 1.25x per P10) and RSS; compare per-worktree `--output_base` inside the worktree versus default (the in-worktree variant uses the wrapper's absolute `--output_base="$PWD/.bazel-out"` and the `.bazelignore` entry; its benefit is that `git worktree remove` deletes the output base, so orphans are impossible, and its costs are that the server's execroot sits inside a directory that `rg` and the cleanup skill's `du` scan, and that `$PWD` length enters the unix-socket path limit; both are recorded, `[INF]`). Also record per worktree `du -sL $(bazel info output_base)/external` against `du -sh` of the same path (docs-derived D5: extracted repos are symlinks into the shared repo contents cache, so the difference should be large and the per-worktree new bytes small), and the same-filesystem check `stat -f -c %T` for the disk cache and the output base (D3).
- Files: `tools/bzl/baseline/bazel-worktree-cost.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/bazel-worktree-cost.sh | jq -e '.peak_rss_gb<=12 and .cold_ratio_median<=0.30 and (.cache_fs == .output_base_fs) and (.external_new_kb_per_worktree|type=="number")'` prints `true`; if it prints `false` the verdict is `FallbackWorkaround` (per-worktree `--output_base`, lower `max_idle_secs`, concurrency cap) and the numbers are recorded, not argued away.

##### Task 0.10b: MODULE.bazel.lock conflict simulation (cost S)
- Create two branches adding different deps, merge, observe. Start from Bazel's documented merge driver, not a custom script (`[DOC]` https://bazel.build/external/lockfile#merge-conflicts: "Bazel provides a custom git merge driver to help resolve these conflicts automatically ... adding this line to a `.gitattributes`", registered once per developer, needs `jq` 1.5+; the manual path is restore the lock, resolve `MODULE.bazel`, run `bazel mod deps`). **TO-VERIFY**: the research extractor dropped the page's code blocks, so the exact `.gitattributes` line, `git config` entries and driver name are UNVERIFIED; the first step of this task is to read the page in a browser (or fetch its raw HTML) and paste the snippet verbatim into `spike-results.md` with the page URL and date. The driver name `bazel-lock` used elsewhere in this plan (Tasks 11.2a, 1.6c check 9, 7.8b, `scripts/git-merge-bazel-lock.sh`) is provisional and follows whatever name the docs use; the custom prototype (`.gitattributes` `MODULE.bazel.lock merge=bazel-lock` calling `bazel mod deps --lockfile_mode=update`) is only the fallback if the documented driver fails the two-branch simulation, and CI keeps its `bazel mod tidy` diff either way. Register the driver through `make setup-git` (target created in Task 0.2h) so the setup exists from Epic 0, not only at Task 11.2a; the `git config merge.bazel-lock.driver` entry is per clone (stored in the common `.git/config`, shared by every worktree of that clone but not by a fresh `git clone`), so `make bazel-doctor` check 9 (Task 1.6c) warns when it is unset and the wrapper prints a one-time hint.
- Files: `.gitattributes` (spike), `scripts/git-merge-bazel-lock.sh`, `Makefile`
- Verify: after the merge `bazel mod deps --lockfile_mode=error` exits 0; `grep -c 'bazel.build/external/lockfile' project_plans/bazel-conversion/implementation/spike-results.md` is at least `1` (the quoted docs snippet is recorded); `jq --version` reports 1.5 or newer.

##### Task 0.10c: Concurrent shared disk-cache integrity test (cost S)
- Validation G6. Script `tools/bzl/baseline/disk-cache-concurrency.sh`: 3 temp worktrees run `bazel build //... --disk_cache=<shared>` at the same time, twice, then a 4th builds against the same cache; compare the sorted `sha256sum` of `bazel-bin` outputs with a no-cache build and grep every `command.log` for `corrupt|digest mismatch`. Runs through the `tools/bazel` wrapper so the slot limit applies (the three builds queue to two slots; the script records that and also runs once with the wrapper bypassed under the 12 GB watchdog to exercise true concurrency). Each build writes `--execution_log_json_file` (readable without tools, large; use it on spike-sized builds, and `--execution_log_compact_file` plus the `//src/tools/execlog:parser` built from a pinned Bazel source checkout for `//...`, D18). The script also records the number of times each identical action executed across the concurrent cold start: `[RUN]` shows no cross-server dedup, so the expected CPU multiple equals the concurrency, and that multiple is the measured cost that the wrapper's N=2 queue avoids (D23, feeds Task 0.10g). Add one run where `--experimental_disk_cache_gc_max_size=1G` trims a 3 GB populated cache while a 4th worktree builds (E3 addendum: GC deleting a blob under a reader must give zero `ERROR:` lines and outputs identical to a no-cache build; Bazel treating a missing CAS entry as a miss is `[INF]`).
- Files: `tools/bzl/baseline/disk-cache-concurrency.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/disk-cache-concurrency.sh | jq -e '.hash_lists_equal and (.corruption_lines==0) and (.gc_race_errors==0) and (.concurrency_cpu_multiple|type=="number")'` prints `true`.

##### Task 0.10d: Spike S13, commit or ignore `MODULE.bazel.lock` (cost S)
- Pre-mortem #2(d). Versions are pinned explicitly in `MODULE.bazel` and `go.sum` pins Go dependencies, so the lock may add only conflicts. Replay 20 two-dependency-addition merges under each option (commit lock + `bazel-lock` merge driver; do not commit the lock, with CI using `--lockfile_mode=off` or regenerate-and-ignore) and count lock conflicts and `--lockfile_mode=error` failures on the replayed `main`. Also reproduce one real two-PR merge on GitHub to test the INFERRED claim that GitHub-side merges apply no repo-defined merge drivers. Record verdict in `spike-results.yaml` row `S13`; "do not commit" amends ADR-001's consequence line and needs owner approval.
- Files: `spike-results.md`, `spike-results.yaml`
- Verify: `grep -c 'S13' project_plans/bazel-conversion/implementation/spike-results.yaml` prints `1` and the row's `path` names the chosen option.
- Docs-derived note (D19): the "commit lock + merge driver" option means Bazel's documented jq-based driver (Task 0.10b), and the docs advise committing the lockfile and using Bazelisk with `.bazelversion` (`[DOC]`, "the lockfile is specific to the Bazel version"); the replay measures that advice against this repo rather than assuming it.

##### Task 0.10e: Cross-worktree cache sharing experiments E1-E4 (cost M)
- `tools/bzl/baseline/cross-worktree-cache.sh`, using the harness of Task 0.10c (execution log JSON; hit ratio = spawns with `runner == "disk cache hit"` or `cacheHit == true` over spawns with `cacheable == true`, excluding internal symlink actions). **E1** (cross-path hit ratio, the core proof): worktree A at one path, B at a different path and depth, same commit, empty shared disk cache; A builds `//:stapler-squad` cold, B builds it from a fresh output base. PASS iff at least 95% of cacheable spawns in B are hits and all misses are on the allow-list (tmux, Bun consumers); on FAIL list the first 5 misses with their `aquery` command lines. **E2** (zero-rebuild third worktree): C builds after E1; PASS iff zero non-allow-listed `linux-sandbox`/`worker` spawns in the `INFO: N processes:` line and wall time at most 30% of A. **E3** (Go compile path independence, the rules_go `[INF]`): for three Go library targets of different sizes compare `bazel aquery 'mnemonic("GoCompile", //pkg:lib)' --output=text` between A and B; PASS iff byte-identical with no normalisation and no absolute path; on FAIL evaluate `--experimental_output_paths=strip` (exists on 9.3.1, "highly experimental", Starlark actions must opt in with `supports-path-mapping`) and `-trimpath`-equivalents. **E4** (digest determinism and absolute-path leaks): build everything with `--disk_cache=` (empty, cache off) in A and B, sha256 every `bazel-bin` regular file by relative path; PASS iff the sorted lists are identical, and a positive-control genrule that writes `$(pwd)` to its output is flagged (`[RUN]` a cache hit imports the other worktree's absolute execroot path because the key excludes the cwd while the content does not); every differing real file goes on the `no-remote-cache` list. Also assert that a `stamp = 1` genrule re-executes in the second worktree (`[RUN]` it misses the cache), so only the final binary is stamped (pitfalls R11).
- Files: `tools/bzl/baseline/cross-worktree-cache.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/cross-worktree-cache.sh | jq -e '.e1_hit_ratio>=0.95 and .e2_nonallow_spawns==0 and .e3_gocompile_identical and .e4_digest_lists_equal and .e4_control_flagged'` prints `true`; on `false` the S6 verdict is `FallbackWorkaround` and the numbers are recorded.

##### Task 0.10f: Cache layout checks E5, E6, E10 (cost S)
- `tools/bzl/baseline/cache-layout-checks.sh`. **E10** (reflink precondition): `filefrag -v` of an output restored from a disk-cache hit and of its CAS blob share a physical extent flagged `shared`, with `nlink=1` on both (a clone, not a hardlink; `[RUN]` on this btrfs `/home`, docs describe no reflink behaviour; CI runners on ext4/xfs and macOS will copy); PASS iff `shared` appears, otherwise the Task 0.10a disk term is re-measured as copy-based. **E5** (GC runs under the idle timeout): with `max_idle_secs=30` populate a 300 MB cache, set `--experimental_disk_cache_gc_max_size=100M --experimental_disk_cache_gc_idle_delay=10s`, run one tiny build, wait 20 s; PASS iff `du -sm` is at most 100; then repeat with the default 5 m delay and PASS iff the size is unchanged after the server exits (proves the explicit delay is needed). If GC does not run, the fallback is the standalone tool (`bazel run //src/tools/diskcache:gc -- --disk_cache=... --max_age=30d --max_size=100G`, built from a pinned Bazel source checkout) on the daily timer of Task 11.4b. **E6** (repo contents cache coverage): in A run `bazel build @rules_go//go` and the `go_deps`/Bun fetches; in B (fresh base) repeat with the network blocked (`unshare -rn` or `--repository_disable_download`); PASS iff B succeeds, `ls -l $(bazel info output_base)/external | grep -c -- '-> .*/contents/'` equals the number of repos A fetched through plain repository rules, and new bytes per worktree for those repos are under 50 MB; repos created by module extensions (`go_deps`, the Bun repository rule) that are not symlinked are listed as a per-worktree cost (`[INF]` today). Also record whether the 14-day contents-cache GC can remove a repo a live idle output base still links to (undocumented).
- Files: `tools/bzl/baseline/cache-layout-checks.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/cache-layout-checks.sh | jq -e '(.e10_shared_extent|type=="boolean") and .e5_gc_trimmed_with_idle_delay and (.e6_offline_build_ok) and (.e6_extension_repos_listed|type=="array")'` prints `true`.

##### Task 0.10g: Containment alternatives E7 and cold-start cost E11 (cost S)
- `tools/bzl/baseline/containment-checks.sh`. **E7**: compare the wrapper's `systemd-run --user --scope -p MemoryMax=14G` against `--experimental_run_in_user_cgroup` (+ `--io_nice_level`): PASS iff a deliberately memory-hungry action is limited or killed without the live service's unit being touched and `systemctl --user show` reports the limit; behaviour under this host's user systemd manager (and negative `OOMScoreAdjust`) is `[INF]`, so the result decides whether Task 0.2f keeps the manual scope. Also record whether `--io_nice_level=7` and `--batch_cpu_scheduling` take effect (`ionice -p`, `chrt -p` of the server pid). **E11**: 3 servers start the same cold build at once versus 3 serialised through the wrapper; PASS iff serialised total CPU-seconds (profile `CPU usage`) is at most 60% of concurrent; the result feeds the N=2 decision of Story 0.10.
- Files: `tools/bzl/baseline/containment-checks.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/containment-checks.sh | jq -e '(.e7_limit_reported|type=="boolean") and (.e7_live_unit_untouched) and (.e11_serial_over_concurrent_cpu<=0.60)'` prints `true`; a `false` is recorded as an input to the S6 verdict, not argued away.

##### Task 0.10h: Multiplex-worker inventory and memory check (cost S) (user-requested addition, https://bazel.build/remote/multiplex)
- This is a within-one-build memory optimisation. It is NOT a cross-worktree mechanism: `[DOC]` the worker key derives from environment variables, the execution root and the mnemonic, so each mnemonic has at most one multiplexer per invocation and two worktrees (separate servers, separate execution roots) never share a worker process; it does not deduplicate work across servers. Cross-worktree sharing comes from the shared disk cache, and in-flight dedup across servers would need remote execution (out of scope). Multiplex workers are experimental, opt-in per action through the `supports-multiplex-workers` execution requirement plus a worker strategy for the mnemonic (`--strategy=<mnemonic>=worker`); `--noworker_multiplex` disables them globally; `--worker_max_instances` caps total workers; they are incompatible with dynamic execution unless they implement multiplex sandboxing (`supports-multiplex-sandboxing`, `--experimental_worker_multiplex_sandboxing`).
- Step 1: enumerate every action mnemonic in the build graph that advertises `supports-workers` or `supports-multiplex-workers` (`bazel aquery '//...' --output=jsonproto`, read `executionRequirements`/`execution_requirements`; the profile is the cross-check) and record mnemonic, rule set of origin and which of the two keys it carries. Whether rules_go (`GoCompile` and friends) and aspect_rules_ts advertise either is UNVERIFIED until this query runs. Step 2: if the list is non-empty, rerun the three-concurrent-builds memory test of Story 0.10 with multiplex workers on versus `--noworker_multiplex` and record peak combined RSS and wall time for each. Step 3: record a verdict row. `PASS iff` there is a peak-RSS reduction of at least 10% with no wall-time regression above 10%, in which case the setting is adopted; otherwise `--noworker_multiplex` or the default stays. An explicit "none found" outcome counts as Pass-with-no-action and is written as such, never skipped.
- Files: `tools/bzl/baseline/worker-inventory.sh`, `spike-results.md`
- Verify: `bash tools/bzl/baseline/worker-inventory.sh | jq -e '(.mnemonics|type=="array") and (.outcome=="none-found" or (.rss_reduction_pct|type=="number" and .wall_regression_pct|type=="number")) and (.verdict=="Pass" or .verdict=="NoAction")'` prints `true`.

#### Story 0.11: Spike S11 - Gazelle behaviour on this repo
**As an** implementer, **I want** to know whether directives and one `map_kind` macro suffice, **so that** no custom Gazelle extension is written without evidence.
**Acceptance Criteria**:
- Idempotent.
  - *Given* generated BUILD files, *When* `bazel run //:gazelle` runs twice, *Then* the second run changes nothing.
- Indifferent to the synced tree.
  - *Given* the same commit with and without `make proto-gen ent-gen` output present, *When* Gazelle runs in each, *Then* `git status --porcelain '*.bazel'` is identical.
- Resolve count decided.
  - *Given* the generated packages (`gen/proto/go/...`, `session/ent`, entity subpackages), *When* `gazelle:resolve` lines are counted, *Then* the decision (directives, or a `gazelle_binary` with an extension) is recorded against the 8-line threshold.
**Files**: `spike-results.md`

##### Task 0.11a: Whole-repo Gazelle run and idempotence (cost M)
- Run Gazelle on the spike branch over all packages (generated deps stubbed), re-run, count unresolved imports, check `embedsrcs` handling for gitignored embed inputs (`# keep`/`# gazelle:ignore` need).
- Files: `spike-results.md`
- Verify: `bazel run //:gazelle -- -mode=diff; echo $?` prints `0` on the second run.

##### Task 0.11b: Synced-tree indifference (cost S)
- Run Gazelle in two checkouts (clean, and after `make ent-gen proto-gen`); diff BUILD changes; pick the exclusion mechanism for the 52 gitignored `session/ent` subdirectories that needs no per-entity hand list and spares the tracked `session/ent/schema` (glob-class `gazelle:exclude` patterns such as `session/ent/[!s]*`, a block generated from `tools/bzl/ent/packages.bzl` and guarded by the same freshness test, a `.gitignore`-aware extension, or sync-gen writing into a path Gazelle skips).
- Files: `spike-results.md`
- Verify: `diff <(cd clean && git status --porcelain) <(cd synced && git status --porcelain); echo $?` prints `0`; `git -C synced status --porcelain | wc -l` prints `0` after Gazelle ran there (no BUILD file leaked into a gitignored `session/ent` subdirectory); `grep -cE '^S11 exclusion-mechanism: [a-z-]+$' spike-results.md` prints `1`.

##### Task 0.11c: map_kind macro for go_test defaults (cost S)
- Prototype `tools/bzl/go_test.bzl` (size/timeout/shard table) attached by `# gazelle:map_kind go_test go_test //tools/bzl:go_test.bzl`.
- Files: spike `tools/bzl/go_test.bzl`, spike `BUILD.bazel`
- Verify: `bazel query 'attr(size, enormous, //server/services:all)' | grep -Fxc '//server/services:services_test'` prints `1`.

#### Story 0.12: Spike S10 - remote cache reachability, server-enforced read-only, secret redaction
**As the** owner, **I want** proof that a GitHub-hosted runner can use the onyx cache, that PR jobs cannot write to it, that credentials never reach an uploaded artifact, and that failure is harmless, **so that** ADR-004 is real.
**Acceptance Criteria**:
- Reachable (read path).
  - *Given* `bazel-remote` on onyx bound to loopback behind the two proxy listeners of ADR-004 (read-only on `:9092`, read-write on `:9093`) and the Tailscale tags `tag:ci-pr` (reach `:9092` only) and `tag:ci-main` (reach `:9093`), *When* `bazel-cache-probe.yml` is dispatched from a PR branch, *Then* a real `bazel build --remote_cache=https://<onyx>:9092 --noremote_upload_local_results` of a small target, run twice with the local disk cache cleared between runs, reports remote cache hits greater than 0 on the second run (a blob GET probe alone is not the criterion: gRPC would never pass the GET/HEAD-only listener, and the HTTP protocol is what Bazel is configured to use; bazel-remote documents `--remote_cache=http(s)://...` per its [README](https://github.com/buchgr/bazel-remote/blob/master/README.md)).
  - *Given* `bazel-remote` with `--htpasswd_file` and `--allow_unauthenticated_reads` on loopback (ADR-004), *When* the same build runs through `:9092` with no credential, *Then* reads succeed (no 401), and an unauthenticated PUT through `:9093` returns 401.
- Read-only is enforced by the server, not the client.
  - *Given* the PR-branch probe deliberately omitting `--noremote_upload_local_results`, *When* it attempts a blob PUT through `:9092` and, with the `tag:ci-pr` key, a PUT to `:9093`, *Then* the first returns HTTP 4xx/405 and the second is refused at the network layer (connection reset or timeout), and the blob is absent on a later GET. bazel-remote's htpasswd auth has no read-only role ([buchgr/bazel-remote#628](https://github.com/buchgr/bazel-remote/issues/628), VERIFIED open, last updated 2023-01-13), which is why the split is by listener and ACL.
- Write path works only from `main`.
  - *Given* the GitHub environment `bazel-cache-write` restricted to branch `main`, *When* the probe is dispatched from a PR branch, *Then* the job cannot read the write credential (environment deployment rejected); *When* dispatched from `main`, *Then* a PUT through `:9093` succeeds and a later GET returns the same sha256.
- Secrets stay out of artifacts (spike S10-redaction).
  - *Given* a canary credential value and `--credential_helper=<onyx host>=%workspace%/tools/bzl/credential-helper.sh` reading it from an environment variable, *When* a build runs with `--build_event_json_file`, `--profile` and `--execution_log_compact_file`, *Then* the canary string and its base64 encoding (`printf %s "user:$CANARY" | base64 -w0`, the form in `Authorization: Basic`) occur 0 times in `bep.json`, the gunzipped profile, the execution log, `command.log`, and stdout/stderr (`grep -c` sums to 0); the same grep is repeated for a run with a deliberately wrong credential (failed-auth path, 401 messages) and with `--remote_verbose_failures` and HTTP verbose/debug output enabled, covering the HTTP-protocol logs and, for completeness, a gRPC-configured run's `--remote_grpc_log`. Whether Bazel redacts credential material is UNVERIFIED and is exactly what this criterion tests; on a nonzero count the verdict is `FallbackWorkaround`: the write job uploads no raw BEP, profile or execution log, only the scrubbed JSON from `bazel-trend.sh`.
- Harmless when absent (real failure shapes, pre-mortem #4).
  - *Given* a cache address that drops packets (non-routable `192.0.2.1`, plus an `iptables -j DROP` rule where root is available), a link with `tc netem delay 2s loss 20%`, and a listener that accepts then never responds, *When* `bazel build //tools/lint/...` runs against each, *Then* each exits 0 with a warning and a wall-time increase of at most 10 seconds over the no-remote-cache run. The earlier test against `https://127.0.0.1:1` is an instant connection refusal and proves nothing about a real outage, so it is not the criterion.
- Fork PRs get no credentials.
  - *Given* a fork PR (or `workflow_dispatch` with empty secrets), *When* the workflow runs, *Then* it uses the disk cache only and exits as normal.
- Disk and remote layers compose (E8).
  - *Given* `bazel-remote` on loopback populated by worktree A, *When* worktree B builds with an empty disk cache and both `--disk_cache` and `--remote_cache`, *Then* B's hits are remote hits and its disk cache grows (Task 0.12f).
**Files**: `.github/workflows/bazel-cache-probe.yml`, `tools/bzl/credential-helper.sh`, `docs/how-to/run-bazel-remote-on-onyx.md`, `spike-results.md`, `spike-results.yaml`

##### Task 0.12a: Runbook, proxy config and Tailscale ACL (cost S)
- Write `docs/how-to/run-bazel-remote-on-onyx.md`: bazel-remote bound to `127.0.0.1` over HTTP (flags `--dir`, `--max_size`, `--http_address 127.0.0.1:<port>`, `--grpc_address none`, `--htpasswd_file` for the single write user, `--allow_unauthenticated_reads` so reads through the read-only listener need no credential and writes still do; flag names VERIFIED in the bazel-remote README), TLS terminated at the proxy, a reverse proxy (Caddy or nginx) with listener `:9092` allowing only `GET` and `HEAD` (and `OPTIONS` if needed) on `/cas/` and `/ac/`, plus `GET /status` for the CI pre-flight (Task 7.4b; S10 asserts a `PUT /status` is still 4xx), and rejecting every other method, a `--max_size` of at most 100 GB with the data directory on a separate dataset or a quota'd `systemd` `StateDirectory` (never sharing free space with the live service's SQLite state; add `df` of that path to the alarm in Task 11.4a), listener `:9093` proxying everything to bazel-remote (which enforces the htpasswd write user for PUT), a user systemd unit, credential storage in 1Password, the Tailscale ACL snippet (`tag:ci-pr` -> `onyx:9092`, `tag:ci-main` -> `onyx:9093`, nothing else), and the TLS-public alternative for the owner to choose.
- Files: `docs/how-to/run-bazel-remote-on-onyx.md`
- Verify: `grep -cE 'htpasswd|9092|9093|tag:ci-pr' docs/how-to/run-bazel-remote-on-onyx.md` is at least 4; `curl -s -o /dev/null -w '%{http_code}' -X PUT --data x https://onyx:9092/cas/<hash>` prints a 4xx code and the same PUT to `:9093` without credentials prints `401` (run by the owner after deploy).

##### Task 0.12e: D-PROTECT-WRITE prerequisite - restrict who can reach the write job (cost XS) (prerequisite of Task 0.12b; owner decision D-PROTECT-WRITE)
- `main` is unprotected (VERIFIED), so any push token, including agent tokens, can push to `main` and thereby run the write job or edit `credential-helper.sh`/`bazel.yml` on `main`. Before the `bazel-cache-write` credential is created, the owner picks one of: (a) a ruleset on `main` (PRs required, direct push restricted to the owner) plus the environment settings below; or (b) the environment settings alone, with direct-push exposure recorded in ADR-004 as an accepted risk. The environment settings are mandatory in both: deployment branches limited to `main` AND required reviewer = the owner (approval gate), so a direct push to `main` still cannot use the credential without owner approval. Document the commands in `docs/how-to/run-bazel-remote-on-onyx.md`. This is an owner-gated decision; the agent does not create rulesets.
- Files: `docs/how-to/run-bazel-remote-on-onyx.md`
- Verify: `gh api repos/tstapler/stapler-squad/environments/bazel-cache-write --jq '[.protection_rules[].type]|sort|join(",")'` prints a string containing both `branch_policy` and `required_reviewers`; if (a) was chosen, `gh api repos/tstapler/stapler-squad/rulesets --jq 'length'` prints at least `1`. Task 0.12b does not start until this prints as stated (owner action; unevaluable = Fail).

##### Task 0.12b: Probe workflow (cost S) (blocked on owner deploy and Task 0.12e)
- `bazel-cache-probe.yml` (`workflow_dispatch` only, pinned action SHAs): the PR-capable job uses only the `tag:ci-pr` Tailscale key and the read endpoint (`https://<onyx>:9092`, real `bazel build`); a second job `if: github.ref == 'refs/heads/main'` in `environment: bazel-cache-write` uses the write credential via `credential-helper.sh`. The probe workflow never receives the write credential outside that job.
- Files: `.github/workflows/bazel-cache-probe.yml`, `tools/bzl/credential-helper.sh`
- Verify: `actionlint .github/workflows/bazel-cache-probe.yml` exits 0; dispatched run's second build reports remote cache hits greater than 0.

##### Task 0.12c: Real packet-drop, slow-link and silent-listener fallback tests (cost S)
- Replaces the earlier black-hole test. Run `bazel build //tools/lint/...` with the committed `ci-cache-read` failure policy (`--remote_timeout=3s --remote_retries=1 --experimental_circuit_breaker_strategy=failure --experimental_remote_failure_rate_threshold=10 --experimental_remote_failure_window_interval=60s`; the breaker is off until the strategy flag is set, `[RUN]`) against (a) a DROP-ed address (`192.0.2.1`, RFC 5737, silently unroutable; add an `iptables -j DROP` case on a runner with root), (b) `tc netem delay 2s loss 20%` in a network namespace (`unshare -Urn` rootless support is INFERRED; otherwise run on a runner with sudo), (c) a local listener that accepts then never responds. Record all three deltas against a no-remote-cache run. Repeat each case once with `--remote_local_fallback --incompatible_remote_local_fallback_for_remote_cache` added and record whether it changes the outcome: `--remote_local_fallback` alone is a no-op for `--remote_cache` (`[RUN]` help: the incompatible flag decides "whether --remote_local_fallback applies to --remote_cache"; the docs say a cacheable local build does not fail on cache errors), so the committed policy is timeout, retries and breaker, and the fallback variant is kept only if it measurably helps. Record also whether the breaker trips and stops further remote attempts in case (a) (`[INF]`).
- Files: `spike-results.md`, `tools/bzl/spike/silent_listener.py`
- Verify: for each of (a), (b), (c): `bazel build ...; echo $?` prints `0` and the elapsed-time delta against the no-cache run is at most 10 s, for the committed policy without the fallback flags.

##### Task 0.12d: Secret-redaction spike and read-only enforcement test (cost S)
- Run the S10-redaction criterion above (raw and base64 canary, success and wrong-credential runs, HTTP verbose output) and the enforcement checks (PUT through `:9092`, `tag:ci-pr` reaching `:9093`, unauthenticated PUT to `:9093` returning 401); record both as `S10` and `S10-redaction` rows. Define `tools/bzl/secret-scan-allow.txt` (initially empty; each entry a literal benign `Authorization` string with a reason) so the secret-scan step is not disabled on its first false positive.
- Files: `spike-results.md`, `spike-results.yaml`
- Verify: `grep -c -e CANARY_SECRET_VALUE -e "$CANARY_B64" bep.json profile.txt execlog.txt command.log stderr.txt stderr-wrongcred.txt | awk -F: '{s+=$NF} END{print s}'` prints `0`; the PUT status code is 4xx.

##### Task 0.12f: Disk + remote cache layering, experiment E8 (cost S) (blocked on the owner deploy of Task 0.12a; can run first against `bazel-remote` on loopback)
- With `bazel-remote` on loopback (no proxy needed) and both `--disk_cache` and `--remote_cache` set: worktree A uploads; B, with an empty disk cache, builds with both flags. PASS iff B's hits are remote hits and B's disk cache afterwards contains the blobs (`du` grew; whether a remote hit is written back into the disk cache is `[INF]` today and is exactly what this measures, with the lookup order local outputs, then cache, then execute, then upload per https://bazel.build/remote/caching). Then C builds with the remote killed mid-build and PASS iff it completes with at most 10 s added over a no-remote run (the same bar as Task 0.12c).
- Files: `spike-results.md`, `tools/bzl/baseline/cache-layering.sh`
- Verify: `bash tools/bzl/baseline/cache-layering.sh | jq -e '.b_remote_hits>0 and (.b_disk_cache_kb_after>.b_disk_cache_kb_before) and (.c_delta_s<=10)'` prints `true`.

#### Story 0.13: Spike S12 - custom analyzers as nogo
**As an** implementer, **I want** to know if the 17 analyzers in the separate `tools/lint` module can run as `nogo`, **so that** Story 8.4 is scheduled or skipped on evidence.
**Acceptance Criteria**:
- One analyzer runs.
  - *Given* the `norawgitcli` analyzer from the isolated `tools/lint` extension registered as `nogo`, *When* a seeded violation (`exec.Command("git", ...)`) is added to a throwaway package and `bazel build` runs, *Then* the build fails with the analyzer's message; removing the seed makes it pass.
**Files**: spike `nogo/BUILD.bazel`, `MODULE.bazel`; `spike-results.md`

##### Task 0.13a: nogo with one analyzer from the isolated module (cost M)
- Register `go_sdk`/`nogo` with `//tools/lint/norawgitcli:norawgitcli`; resolve the cross-module dependency via isolated extension or `local_path_override`; record whether `analysistest` fixtures need runfiles `testdata`.
- Files: spike `BUILD.bazel`, spike `MODULE.bazel`
- Verify: `bazel build //tools/bzl/spike-nogo:seed 2>&1 | grep -c norawgitcli` is at least 1 with the seed present (Epic 0 S12 path; `//tools/bzl/spike/nogo_seed` does not exist). **Epic 0 correction (S12):** nogo links one copy of `x/tools`, so each analyzer needs a twin library built against the root `x/tools` (the isolated `tools/lint` module resolves a different version and the link fails); `go_sdk.nogo` registers one nogo for the whole repo, scoped with `exclude_files`. **Story 8.4 applied this** (owner decision `nogo_story_8_4`): twins for all 17 analyzers in `tools/bzl/nogo`, nogo enabled in `MODULE.bazel`, all 17 analysistest targets run under Bazel; the spike dir `tools/bzl/spike-nogo` is gone (its seed is `//tools/bzl/nogo/testdata:seed_norawgitcli`).

#### Story 0.15: Spike S14 - selective e2e through observed-dependency slices (ADR-009)
**As the** owner, **I want** to know before Epic 12 whether a spec's measured dependencies are narrow enough that a typical PR re-runs well under half the e2e suite with no missed failures, **so that** the directive "only changed dependencies re-run their tests" is decided on evidence. (File order: this story is listed before Story 0.14 because the gate reads its verdict; its id is the next free number.)
**Acceptance Criteria**:
- Registry coverage is measured and the gap list is committed.
  - *Given* the 193 specs under `tests/e2e/`, `docs/registry/features/**`, the two aggregate registries and `web-app/src/lib/features/features/*.ts`, *When* `python3 tools/bzl/e2e/registry-coverage.py` runs, *Then* it prints the table of ADR-009 (specs with a feature id, resolvable ids, specs with an existing source file, Go/web split, unresolvable id list) and writes `tests/e2e/feature-gaps.json`; the 2026-11-02 baseline is 193 / 119 (61.7%) / 80 (41.5%) / 102 (52.8%) / 55 Go and 68 web, 106 of 238 ids unresolvable. This is information, not a pass criterion: the registry is the seed and cross-check, not the slice source.
- The observed manifest covers every spec and validates the registry.
  - *Given* Tasks 0.15a-b, *When* `python3 tools/bzl/e2e/check-manifest.py` runs, *Then* 193 of 193 specs have a non-empty `go_pkgs` (or a recorded `no_server` reason) and, for the specs whose declared registry handler package exists, at least 90% of those declared packages appear in the observed set (a lower figure means the registry is unreliable and is recorded as such).
- Selection is narrow enough, measured by replay.
  - *Given* the last 100 first-parent commits on `main`, *When* `bash tools/bzl/e2e/replay-selection.sh` runs the rdeps selection for each commit that touches non-test Go or web source (66 on 2026-11-02, of which 59 touch `session/`, `server/` or `config/`), *Then* the median selected-spec share is at most 0.50, the p90 is at most 0.90 and the share of commits that fall back to `__ALL__` is at most 0.15; today the share is 1.00 for every relevant PR (`build.yml:97`).
- Zero missed failures against a full run.
  - *Given* 30 seeded mutations (a patch each, spread over at least 15 Go packages, 10 web files and 5 RPC handlers, listed in `tools/bzl/e2e/mutations.yaml`), *When* `bash tools/bzl/e2e/mutation-backtest.sh` runs the full suite and the selected set per mutation, *Then* for every mutation that fails at least one spec in the full run the selected set contains at least one failing spec: 0 misses. A spec that fails in the full run and not in the selected set is a miss; on any miss the verdict is `Fail` or `FallbackWorkaround` (coarser slices) and the missing edge is recorded.
- The Bazel semantics ADR-009 relies on are verified on a throwaway target.
  - *Given* a spike target with a declared slice and `--test_env` binary path, *When* the semantic checks of Task 0.15d run, *Then*: (S-a) editing a file inside the slice re-runs the test and editing one outside does not; (S-b) swapping the file behind the constant `--test_env` path does not change the key; (S-c) a failing run is re-executed on the next invocation and is not served from the disk cache; (S-d) `bazel query rdeps` over a source-file label finds the target through a `go_library` dependency; (S-e) per-target Playwright start-up overhead is at most 15% of a 5-spec group's wall time, measured against the same specs run directly. Each check prints `S-x: PASS|FAIL`.
**Verdict mapping**: all criteria hold: `Pass`. Replay median above 0.50 or p90 above 0.90 but zero misses: `FallbackWorkaround` (coarser selection is already the baseline; add diff-line coverage selection, ADR-009 Consequences, cost M, needs a larger escape budget). Any miss, or S-a/S-c/S-d failing: `Fail` for the sound-slice design; the owner decides between diff-line coverage selection with an escape budget and keeping e2e as a full run (the directive then applies to the Go, Jest and integration layers only). The measured median share is written to `spike-results.yaml` as `e2e_selected_share_median` and read by the economic gate (Task 0.14e).
**Files**: `tools/bzl/e2e/registry-coverage.py`, `tools/bzl/e2e/replay-selection.sh`, `tools/bzl/e2e/mutation-backtest.sh`, `tools/bzl/e2e/mutations.yaml`, `tests/e2e/slice-manifest.json` (spike copy), `tests/e2e/feature-gaps.json`, `spike-results.md`

##### Task 0.15a: Cover-instrumented per-spec capture for all specs (cost M)
- Prototype Tasks 12.1a-12.1c on the spike branch: capture each spec with `go build -cover -coverpkg=./...` and `GOCOVERDIR`, `go tool covdata pkglist`, plus the route-visit and JS-coverage capture; run on 20 representative specs first (record the per-spec capture wall time), then all 193 across the five CI shards' worth of runners. Record the specs that cannot be captured (perf, visual-regression, secondary browsers) as `excluded` with a reason.
- Files: `tools/bzl/e2e/capture.sh`, `tools/bzl/e2e/gen-manifest.py`, `tests/e2e/slice-manifest.json`
- Verify: `python3 tools/bzl/e2e/check-manifest.py; echo $?` prints `0`.

##### Task 0.15b: Registry coverage and gap list (cost S)
- Write `tools/bzl/e2e/registry-coverage.py` (spec header parsing as in `tools/coverage/feature-coverage.ts`, including `FEATURE_CATALOG[...]` references, resolution against the three registry sources, existence check of the source files, the cross-check against the observed manifest) and commit `tests/e2e/feature-gaps.json`.
- Files: `tools/bzl/e2e/registry-coverage.py`, `tests/e2e/feature-gaps.json`
- Verify: `python3 tools/bzl/e2e/registry-coverage.py --json | jq -e '.specs==193 and .with_resolvable_id>=1 and (.unresolved_ids|length)>=0'` prints `true`.

##### Task 0.15c: Selection replay on the last 100 commits (cost M)
- `tools/bzl/e2e/replay-selection.sh` runs a prototype of `affected-e2e.sh` (Task 12.4a) per commit from `git log main --first-parent -n 100 --name-only`, treats docs-only and test-only commits as zero prod change, and prints the selected-share distribution, the `__ALL__` fallback rate and a per-top-level-directory breakdown (so a core-package problem shows up by name).
- Files: `tools/bzl/e2e/replay-selection.sh`, `tools/ci/affected-e2e.sh`
- Verify: `bash tools/bzl/e2e/replay-selection.sh --json | jq -e '.commits==100 and .share_median<=0.50 and .share_p90<=0.90 and .all_fallback_rate<=0.15'` prints `true`.

##### Task 0.15d: Mutation backtest and Bazel semantic checks (cost M)
- Write `mutation-backtest.sh` and `mutations.yaml` (30 patches), run per the AC on the five-runner matrix (the cost is about 30 full runs of about 113 runner-minutes each, on a public repository; the harness first runs only the specs the mutation's package appears in plus a 10% random sample of the rest, then confirms any non-selected failure with a full run, to bound the spend) and the semantic checks S-a to S-e on a throwaway target in `tools/bzl/spike/e2e_slice/`.
- Files: `tools/bzl/e2e/mutation-backtest.sh`, `tools/bzl/e2e/mutations.yaml`, `tools/bzl/spike/e2e_slice/BUILD.bazel`, `tools/bzl/e2e/semantic-checks.sh`
- Verify: `bash tools/bzl/e2e/mutation-backtest.sh --summary | grep -c '^MISS '` prints `0`; `bash tools/bzl/e2e/semantic-checks.sh | grep -cE '^S-[a-e]: PASS$'` prints `5`.

#### Story 0.14: Spike gate - verdicts, ADR finalisation, owner sign-off
**As the** owner, **I want** one place that shows every spike verdict and the decided path, **so that** I can release Epics 1-11 or redirect.
**Acceptance Criteria**:
- Complete and machine-checkable.
  - *Given* the spike ids S-gomod, S1, S1-ts, S2, S3, S4, S5, S6, S7, S8, S9, S10, S10-redaction, S11, S12, S13, S14, *When* `python3 tools/bzl/check-spike-results.py` runs on `spike-results.yaml` (rows `{spike, verdict, path, cost}`), *Then* it exits 0 only if every id has exactly one row, every `verdict` is one of `Pass`, `FallbackWorkaround`, `FallbackFork`, `Fail`, and every `FallbackWorkaround`/`FallbackFork` row names a `path` and `cost`; downstream tasks (for example Task 8.4's skip rule and the S4-fail consequences) read the file, not prose.
- Flag names are verified, not assumed.
  - *Given* the flag list of Task 0.2b, *When* `grep -c 'spike-verified' spike-results.md` runs, *Then* it prints at least 24 (one per listed flag: the 20 flags of Task 0.2b plus the 4 recorded-absent flags, or a recorded "absent on 9.3.1" line).
- Validation gaps are closed or waived.
  - *Given* `validation.md` gaps G1-G9, *When* the gate runs, *Then* each maps to a plan task id (see the Phase 4 disposition table) or carries an explicit owner waiver.
- ADRs finalised.
  - *Given* ADR-002, ADR-003, ADR-004 (reachability mechanism), ADR-007 (Proposed), *When* the gate runs, *Then* the `Status:` line of each begins `Accepted` or the ADR is amended with the spike outcome and a changelog line.
- Economic abort criterion evaluated (decision gate, not a downscale).
  - *Given* `spike-results.yaml`, `baselines.md` and `targets.yaml`, *When* `python3 tools/bzl/economic-gate.py` runs, *Then* it prints `ECONOMIC GATE: CLEAR` (exit 0) or `ECONOMIC GATE: OWNER DECISION <criteria tripped>` (exit 3); on exit 3 Epics 1-11 stay unreleased until `owner-attestations.yaml` has `economic_gate: {decision, date, reason}` with `decision` one of `continue`, `re-plan`, `pause`, `revert`; the script never selects a decision and never edits scope.
**Files**: `project_plans/bazel-conversion/implementation/spike-results.md`, `project_plans/bazel-conversion/decisions/ADR-002*.md`, `ADR-003*.md`, `ADR-007*.md`, `docs/reference/bazel-forks.md` (only if a fork is chosen)

##### Task 0.14a: Verdict table and checker (cost S)
- Write `spike-results.yaml` (one row per id), `tools/bzl/check-spike-results.py` (about 15 lines: known ids, known verdict enum, required fields; rejects unknown or missing ids) and a unit test; render a summary table at the top of `spike-results.md` from the YAML; list `FallbackFork` items with the fork repo names the owner must create. (Split from the ADR updates, now Task 0.14d, so each task touches at most 5 files.)
- Files: `spike-results.yaml`, `tools/bzl/check-spike-results.py`, `tools/bzl/check-spike-results_test.py`, `spike-results.md`
- Verify: `python3 tools/bzl/check-spike-results.py project_plans/bazel-conversion/implementation/spike-results.yaml; echo $?` prints `0`; `python3 -m unittest tools/bzl/check-spike-results_test.py` exits 0 including a rejected-unknown-verdict case.

##### Task 0.14d: ADR status updates (cost XS)
- Update the `Status:` lines of ADR-002, ADR-003, ADR-004 (reachability, read-only enforcement, redaction) and ADR-007 from the spike outcomes, or amend each with the outcome and a changelog line.
- Files: `decisions/ADR-002*.md`, `decisions/ADR-003*.md`, `decisions/ADR-004*.md`, `decisions/ADR-007*.md`
- Verify: `grep -hE '^\*\*Status\*\*: Accepted' decisions/ADR-00[2347]*.md | wc -l` equals 4.

##### Task 0.14e: Economic abort / re-plan decision gate (cost S)
- Write `tools/bzl/economic-gate.py` and a unit test. Inputs: `spike-results.yaml`, the S6/S10 measurements in `spike-results.md`, `baselines.md`, and the factors in `targets.yaml` (proposal values; **set at Story 0.1 from baseline, owner-confirmed**). Criteria, any one trips the gate: **EG1** projected PR time-to-green p50 (S10 second-run Bazel job wall time plus the unconverted jobs) above 1.10 x baseline p50; **EG2** projected cold CI (S10 cold run, empty cache) above 1.5 x baseline p50; **EG3** first-worktree Bazel cold build (S6) above 3 x the `baseline-worktree.json` make median, or second-worktree cold median above 2 x the S6 bound (60% of make); **EG4** the plan's CU total re-costed with the chosen fallback paths (Fallback matrix costs) above 1.25 x the verified total in the header; **EG5** (risky assumptions, Task 0.1f) RA1 median reverse-dependency fan-out above 0.60, or RA2 above its limit, because then T1/T2 cannot be reached by Bazel alone. RA2 is evaluated on the post-selection projection (ADR-009): `(setup_and_queue + lint + go_or_other + e2e_median x s) / baseline_span` above 0.90, where `s = e2e_selected_share_median` from spike S14 (Story 0.15); if S14 has no verdict (`s` unknown) `s = 1.0`, which on the measured baseline (0.987) trips EG5, so Story 0.15 must be decided before the gate. The residual non-e2e share `(setup_and_queue + lint + go_or_other) / baseline_span` (0.17 on the measured baseline: 432 s of 2601 s) must stay at or below 0.60 as before. `economic-gate.py --midpoint` re-evaluates T1, T3 and T4 for Story 2.7. Output as in the Story 0.14 AC. The trip list is appended to `spike-results.md` with each measured value next to its limit.
- Options the owner may choose (recorded, never auto-selected): `continue` (with reason), `re-plan` (reorder or re-cost stages, scope unchanged), `pause` (stop after a named stage; later stages stay in the plan), `revert` (make-only, strangler artifacts deleted). A trip never removes a task from this plan.
- Files: `tools/bzl/economic-gate.py`, `tools/bzl/economic-gate_test.py`, `spike-results.md`, `owner-attestations.yaml`
- Verify: `python3 -m unittest tools/bzl/economic-gate_test.py` exits 0 and includes one fixture per criterion that exits 3 plus an all-clear fixture that exits 0; `python3 tools/bzl/economic-gate.py; echo $?` prints `0` or `3`, and on `3` the `economic_gate` key exists in `owner-attestations.yaml` before Task 1.1a starts.

##### Task 0.14b: Fork register (conditional) (cost XS)
- If any fork is chosen, create `docs/reference/bazel-forks.md` with upstream issue/PR link, pin (`git_override`), patch size, removal condition (ADR-008 rule 4).
- Files: `docs/reference/bazel-forks.md`
- Verify: `test ! -e docs/reference/bazel-forks.md || awk -F'|' 'NR>2 && $2 !~ /^ *$/ { if ($3 ~ /^ *$/ || $6 ~ /^ *$/) bad=1 } END { exit bad }' docs/reference/bazel-forks.md; echo $?` prints `0` (columns: `fork | upstream link | pin | patch size | removal condition`; the file is absent when no fork is chosen).

##### Task 0.14c: Mark ci-speed ADR-001 superseded (cost XS)
- Add a header line `**Superseded by**: project_plans/bazel-conversion/decisions/ADR-001-adopt-bazel-bzlmod-build-of-record.md` to the old ADR.
- Files: `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md`
- Verify: `grep -c 'Superseded by' project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md` prints `1`.

---
## Epic 1: Go-only strangler stage (non-blocking CI)

**Goal**: Gazelle owns the BUILD files for every Go package; the part of the repo that does not import generated code builds and tests on Linux under Bazel in a non-blocking CI job beside the make jobs. Gated on Epic 0 verdicts for S-gomod, S7, S11. Pure-Go sqlite change lands here (ADR-006).

#### Story 1.1: Gazelle-generated BUILD files and the sqlite test move
**As an** agent, **I want** BUILD files for all Go packages and no dead cgo dependency, **so that** Bazel can see the code and Linux builds are pure Go.
**Acceptance Criteria**:
- BUILD files are fresh.
  - *Given* the committed BUILD files, *When* `bazel run //:gazelle -- -mode=diff` runs, *Then* exit 0 with empty output, twice in a row.
- mattn is gone.
  - *Given* `session/ent_repository_backlog_test.go` and `session/workflow_enabled_field_migration_test.go` using `modernc.org/sqlite`, *When* `go test ./session -run 'Backlog|WorkflowEnabled' -count=1` runs, *Then* it passes and `grep -c mattn go.mod` prints `0`.
**Files**: `**/BUILD.bazel`, `go.mod`, `go.sum`, `session/ent_repository_backlog_test.go`, `session/workflow_enabled_field_migration_test.go`

##### Task 1.1a: Generate and commit BUILD files (cost M)
- Before the first run, add to the root `BUILD.bazel` the `# gazelle:resolve` / `# gazelle:resolve_regexp` and `# gazelle:exclude` lines for the generated trees, using the S11 verdict and the final target labels (labels may point at targets that do not exist yet; Epic 2 only adds the targets, so Gazelle output is never rewritten wholesale): `gen/proto/go/session/v1` -> `//proto/session/v1:session_go_proto`, `gen/proto/go/session/v1/sessionv1connect` -> `//proto/session/v1:session_connect_go`, and `session/ent` plus its subpackages -> `//tools/bzl/ent:<name>` (one regex line if S11 confirms capture-group substitution). The exclusion for gitignored `session/ent/*` entity directories must spare the tracked `session/ent/schema`. Then run `bazel run //:gazelle`, review that excluded paths (`worktrees`, `.claude`, `third_party/tmux`, `project_plans`, `website`) produced nothing, set `.gitattributes` `**/BUILD.bazel linguist-generated`, commit. HandBuildFile packages get the header comment and `# gazelle:ignore` in later epics. This is the first repo-wide Gazelle run, so it also switches on `check-build-paths.sh --full` (Task 0.2e) and writes `tools/bzl/no-build-packages.txt` (one `<dir>  # <reason>` line per Go package that deliberately has no BUILD file, for example the gitignored `gen/` and `session/ent` entity directories). **Epic 0 correction (S11):** `# gazelle:ignore` does not remove a file's rules from the resolver index, so hand-written libraries can collide with generated ones and Gazelle silently drops the dep (exit 0).
- Guard (S11): the task fails if Gazelle output contains any `matches multiple rules` line (a second `-mode=diff` run being clean is not enough, because the first run exits 0 while dropping deps): `bazel run //:gazelle 2>&1 | grep -c "matches multiple rules"` prints `0`.
- Files: `**/BUILD.bazel`, `.gitattributes`, `tools/bzl/no-build-packages.txt`
- Verify: `grep -c 'gazelle:resolve' BUILD.bazel` is at most `8`; `bazel run //:gazelle -- -mode=diff; echo $?` prints `0` with and without a synced `gen/` and `session/ent`; `comm -23 <(go list -f '{{.Dir}}' ./... | sed "s#^$PWD/##;s#^$PWD\$#.#" | sort) <(git ls-files '*BUILD.bazel' | xargs -n1 dirname | sort -u) | grep -vxFf <(sed 's/ *#.*//' tools/bzl/no-build-packages.txt) | wc -l` prints `0` (every Go package has a BUILD file unless listed with a reason); `bash tools/bzl/check-build-paths.sh --full; echo $?` prints `0`.

##### Task 1.1b: Move the two mattn tests to modernc and drop mattn (cost S)
- Change driver import and DSN usage in the two test files; `go mod tidy`; re-run `bazel mod tidy` so `use_repo` drops `mattn`.
- Files: the two test files, `go.mod`, `go.sum`, `MODULE.bazel`
- Verify: `go test ./session -run 'Backlog|WorkflowEnabled' -count=1` prints `ok`; `grep -c mattn go.mod` prints `0`.

##### Task 1.1c: Pure-Go Linux config (cost XS)
- `.bazelrc`: `build:linux --@rules_go//go/config:pure`; `build:race --@rules_go//go/config:pure=false --@rules_go//go/config:race` so race (which needs cgo and a host C compiler, ADR-006) composes with the pure-Go Linux default instead of conflicting with it (criterion tested in spike S7, Task 0.8a).
- Files: `.bazelrc`
- Verify: `bazel build --config=linux //tools/lint/...` exits 0 with no C compile actions: `bazel aquery --config=linux 'mnemonic("CppCompile", //tools/lint/...)'` prints no actions; `bazel build --config=linux --config=race //tools/lint/...` exits 0.

#### Story 1.2: Build and test the generated-independent subset
**As the** owner, **I want** early signal on everything that does not need codegen, **so that** failures surface before Epic 2 lands.
**Acceptance Criteria**:
- Subset is defined mechanically.
  - *Given* the synced `GenSurface` on a dev machine, *When* `tools/bzl/subset-no-gen.sh` runs, *Then* it writes `tools/bzl/no-gen-packages.txt` listing packages whose `go list -deps` contain neither `gen/proto/go` nor `session/ent`.
- Subset passes or is accounted for.
  - *Given* that list, *When* `bazel test --config=ci $(sed 's#^#//#;s#$#/...#' tools/bzl/no-gen-packages.txt)` runs, *Then* every failure is in `tools/bzl/test-exclusions.yaml` with a `SandboxClass`.
**Files**: `tools/bzl/subset-no-gen.sh`, `tools/bzl/no-gen-packages.txt`, `tools/bzl/test-exclusions.yaml`

##### Task 1.2a: Subset script (cost S)
- Write `subset-no-gen.sh` (temporary: deleted by Task 2.6a). Uses `go list -deps` with the synced tree present.
- Files: `tools/bzl/subset-no-gen.sh`, `tools/bzl/no-gen-packages.txt`
- Verify: `test "$(wc -l < tools/bzl/no-gen-packages.txt)" -ge 1; echo $?` prints `0`; `while read -r p; do go list -deps "./$p" | grep -E 'gen/proto/go|stapler-squad/session/ent' && echo "BAD $p"; done < tools/bzl/no-gen-packages.txt | wc -l` prints `0` (no listed package imports generated code).

##### Task 1.2b: Run the subset and seed the ExclusionsFile (cost M)
- Run, classify each failing test with `SandboxClass`, write entries `{test, class, owner, issue, reason}`; open one tracking issue per class (repo policy: fix, do not defer).
- Files: `tools/bzl/test-exclusions.yaml`
- Verify: `python3 tools/bzl/check-exclusions.py; echo $?` prints `0` once Task 4.9a lands; until then `yq -e '[.[] | select(.issue == null or .class == null or .owner == null or .reason == null)] | length == 0' tools/bzl/test-exclusions.yaml` prints `true`, and `yq -e '[.[].class] | unique | length >= 1' tools/bzl/test-exclusions.yaml` prints `true` (an empty file means the subset was not run).

#### Story 1.3: Non-blocking Bazel CI job
**As the** owner, **I want** a Bazel job on every PR that cannot block merges, **so that** the soak accumulates evidence.
**Acceptance Criteria**:
- Job runs and cannot block.
  - *Given* a PR touching `session/health.go`, *When* CI runs, *Then* a `bazel (non-blocking)` check appears, uses `bazel-contrib/setup-bazel` with disk and repository cache, and its failure leaves the PR mergeable (`continue-on-error: true`, not a required check).
- Artifacts uploaded.
  - *Given* a finished run, *When* its artifacts are listed, *Then* `bep.json` and `profile.gz` exist.
**Files**: `.github/workflows/bazel.yml`

##### Task 1.3a: bazel.yml with gazelle diff and subset build/test (cost S)
- Pin all action SHAs (`gha-pin-actions` skill); `--config=ci`, `--lockfile_mode=error`, `--remote_upload_local_results=false`; steps: `bazel mod tidy` diff, Gazelle diff, subset build+test.
- Files: `.github/workflows/bazel.yml`
- Verify: `actionlint .github/workflows/bazel.yml` exits 0; `test "$(grep -c 'continue-on-error: true' .github/workflows/bazel.yml)" -ge 1` exits 0; after a draft-PR push, `timeout 1800 bash -c 'until [ "$(gh run list --workflow=bazel.yml --branch "$(git branch --show-current)" --limit 1 --json status --jq ".[0].status")" = completed ]; do sleep 30; done'` exits 0 and `gh pr checks --required --json name --jq '[.[]|select(.name|test("bazel";"i"))]|length'` prints `0` (the job is not a required check).

##### Task 1.3b: BEP and profile artifacts (cost S)
- Add `--build_event_json_file=bep.json --profile=profile.gz` on the CI job's command line (never in a committed rc config or `--config=agent`: passing `--profile=<path>` disables Bazel's automatic garbage collection of the 5 retained profiles, `[DOC]` json-trace-profile page, which is harmless on an ephemeral runner but would grow unbounded on a worktree; any local or self-hosted use must prune), upload with `actions/upload-artifact` (pinned). Applies only to jobs that hold no write credential (PR and read-only jobs); the `main` write job uploads only the scrubbed trend JSON (ADR-004).
- Files: `.github/workflows/bazel.yml`
- Verify: `RUN=$(gh run list --workflow=bazel.yml --branch "$(git branch --show-current)" --limit 1 --json databaseId --jq '.[0].databaseId'); gh api repos/tstapler/stapler-squad/actions/runs/$RUN/artifacts --jq '[.artifacts[].name|select(test("bep|profile"))]|length'` prints `2`.

#### Story 1.4: TagConfig set in .bazelrc
**As an** agent, **I want** named configs for the test/build matrix, **so that** `--config=sinkguard` replaces remembering tag lists.
**Acceptance Criteria**:
- Each config builds.
  - *Given* `.bazelrc` configs `embed_tmux`, `embed_tymux`, `sinkguard`, `gitoracle`, `integration`, `race`, `release`, *When* `for c in ...; do bazel build --config=$c //tools/lint/...; done` runs, *Then* all exit 0.
- Config names are a sealed set.
  - *Given* `tools/bzl/check-configs.sh`, *When* `.bazelrc` gains an undocumented `build:*` name, *Then* the script exits 1 naming it.
**Files**: `.bazelrc`, `tools/bzl/configs.txt`, `tools/bzl/check-configs.sh`

##### Task 1.4a: Add the configs per spike S7 (cost S)
- Use the composition verified in Story 0.8 (`embed_tymux` lists both tags); `integration` adds `--test_tag_filters=-slow`; `live_github`, `harness` tests tagged `manual`. Create `tools/bzl/configs.txt` (one name per line) as the machine-readable source of truth for the sealed config set.
- Files: `.bazelrc`, `tools/bzl/configs.txt`
- Verify: loop above exits 0.

##### Task 1.4b: Sealed-set check (cost XS)
- Script (accepts `--rc <file>`, default `.bazelrc`) compares `grep -oE '^(build|test):[a-z_]+' .bazelrc` with `tools/bzl/configs.txt` (exists from Task 1.4a, so Epic 1 has something to compare against). Later, Task 11.3b adds a check that the reference page `docs/reference/bazel-targets-and-configs.md` lists exactly the names in `configs.txt`.
- Files: `tools/bzl/check-configs.sh`
- Verify: `bash tools/bzl/check-configs.sh; echo $?` prints `0`; the script takes `--rc <file>` (default `.bazelrc`) so the negative case is testable: `t=$(mktemp); printf 'build:zzz --x\n' > "$t"; bash tools/bzl/check-configs.sh --rc "$t"; echo $?` prints `1`.

#### Story 1.5: BUILD_SYSTEM skeleton (make stays independent of Bazel)
**As an** agent, **I want** the `BUILD_SYSTEM` switch and its validation to exist before any recipe depends on it, **so that** adding Bazel gates in Epic 2 cannot make the default `make` path require Bazel.
**Acceptance Criteria**:
- Validation exists early.
  - *Given* `make/bazel.mk` included by `Makefile`, *When* `BUILD_SYSTEM=nix make -n help` runs, *Then* it exits 2 with `BUILD_SYSTEM must be make or bazel`; the default is `make`.
- The make path never needs Bazel.
  - *Given* a runner with no `bazel` or `bazelisk` on `PATH` and `BUILD_SYSTEM=make`, *When* `make ready` runs, *Then* it does not invoke Bazel: every Bazel-dependent gate (`gen-freshness`, `gazelle-check`, `buildifier-check`) is skipped with a printed `skipped: BUILD_SYSTEM=make` line. These gates join `ready` only when `BUILD_SYSTEM=bazel`.
**Files**: `make/bazel.mk`, `Makefile`, `.github/workflows/bazel.yml`

##### Task 1.5a: make/bazel.mk skeleton and conditional gates (cost S)
- Create `make/bazel.mk` with `BUILD_SYSTEM ?= make`, the `make|bazel` validation, and a `define wrap` helper stub; Story 8.1 extends it with `BAZEL_FLAGS` and wrappers. Gates added to `ready` by Epic 2/7/8 use `$(if $(filter bazel,$(BUILD_SYSTEM)),<gate>,)`. Also create the skeleton targets that Epic 1 error hints and the skill name, so no hint points at a target that does not exist yet: `gazelle` (runs `bazel run //:gazelle` through the wrapper), `gazelle-check` (`-mode=diff`) and `sync` (under `BUILD_SYSTEM=make` it runs the legacy `proto-gen ent-gen`; the Bazel branch and the stamp arrive in Tasks 2.4a, 2.4b and 2.4d). `setup-git` already exists from Task 0.2h.
- Files: `make/bazel.mk`, `Makefile`
- Verify: `BUILD_SYSTEM=nix make -n help; echo $?` prints the message and `2`; `PATH=$(dirname $(command -v make)):/usr/bin make -n ready | grep -cE '(^|[[:space:]/])(bazel|bazelisk)[[:space:]]'` prints `0` when `BUILD_SYSTEM=make` (the pattern matches a Bazel invocation, not the `check-bazelrc.sh` name that Task 1.5b adds to `ready`); `make -n gazelle; echo $?` and `make -n sync; echo $?` each print `0`.

##### Task 1.5b: Wire the Epic 0 guard scripts into `make ready` and `bazel.yml` (cost S)
- Epic 0 created four guard scripts and described them as running in `make ready` and `bazel.yml`, but no task added the calls. Add a `ready-bazel-guards` target in `make/bazel.mk`, a prerequisite of `ready`, that runs `tools/bzl/check-build-paths.sh` (Task 0.2e), `tools/bzl/check-no-force-push.sh` (Task 0.2h), `tools/bzl/check-bazelrc.sh` (Task 0.2g) and `tools/bzl/check-bazelignore.sh` (Task 0.2a). All four need only a shell and `git`, so they run under both `BUILD_SYSTEM` values, except `check-bazelrc.sh`'s `bazel help` flag validation, which prints `skipped: BUILD_SYSTEM=make` and exits 0 when no `bazel`/`bazelisk` is on `PATH`. In `.github/workflows/bazel.yml` (Task 1.3a) add one early step that runs the same four scripts without `continue-on-error`, and a `pull_request`-only step that runs `bash tools/bzl/check-pr-title.sh "$PR_TITLE"` with the title passed through an `env:` entry, never interpolated into the shell line. Guard failures must not be masked by the job's non-blocking setting (Task 1.3a): the guard step is a separate job `bazel-guards` (it sets up Bazelisk like the build job so `check-bazelrc.sh` validates flags for real), a required-check candidate for Task 7.6.
- Files: `make/bazel.mk`, `Makefile`, `.github/workflows/bazel.yml`
- Verify: `make -n ready | grep -c 'check-build-paths.sh'`, `make -n ready | grep -c 'check-no-force-push.sh'`, `make -n ready | grep -c 'check-bazelrc.sh'` and `make -n ready | grep -c 'check-bazelignore.sh'` each print at least `1`; `yq -e '.jobs."bazel-guards".steps | map(.run // "") | join(" ") | test("check-build-paths.sh") and test("check-bazelrc.sh") and test("check-no-force-push.sh") and test("check-pr-title.sh")' .github/workflows/bazel.yml` prints `true`; `yq -e '.jobs."bazel-guards" | has("continue-on-error") | not' .github/workflows/bazel.yml` prints `true`; `actionlint .github/workflows/bazel.yml` exits 0; with a seeded violation (`touch WORKSPACE`) `make ready-bazel-guards; echo $?` prints a non-zero value and `rm WORKSPACE` restores `0`.

---

#### Story 1.6: Wrapper error translation, queue messages and `make bazel-doctor` (lands with Epic 1)
**As an** agent, **I want** raw Bazel failures, waits and degradations turned into one-line remedies from my very first Bazel command, **so that** a stale BUILD file, a missing generated package, a full disk or a queued build never looks like a hang or a mystery.
**Acceptance Criteria**:
- Translation table is exact.
  - *Given* `tools/bzl/errors.yaml`, *When* a captured Bazel failure matching row E1-E11 is piped through `tools/bzl/translate-errors.sh`, *Then* the output contains the row's `bazel-hint:` line verbatim and the original `ERROR:` line. Message text is a contract (tests compare it verbatim):
    - E1 stale BUILD (`missing strict dependencies`, `no such file` for a deleted source, `undefined: <pkg>` after an import change): `bazel-hint: a BUILD file is out of date with the Go sources. Remedy: run 'make gazelle', then commit the BUILD diff.`
    - E2 missing external dep (`no such package '@@gazelle++go_deps`, `repository ... not found`): `bazel-hint: a Go dependency is not wired into MODULE.bazel. Remedy: run 'go get <module> && go mod tidy && make gazelle && bazel mod tidy'.`
    - E3 plain `go`/gopls missing generated code (`source: go`; `cannot find package`, `package .*/gen/.* is not in std` under `gen/` or `session/ent`; the failure is seen by the `make go-*` shims of Task 2.4e, because raw `go` and gopls never pass through the Bazel wrapper): `bazel-hint: plain go and gopls need generated code; Bazel does not. Remedy: run 'make sync' (or use 'make go-build' / 'make go-test', which sync first).`
    - E10 Bazel generated-code label problem (`source: bazel`; `no such package '//gen`, `no such target '//proto/session/v1:`, `no such target '//tools/bzl/ent:`, `missing input file '//tools/bzl/ent`): `bazel-hint: a BUILD label for generated code is missing or misnamed; 'make sync' will not help because Bazel does not read gen/. Remedy: run 'make gazelle', then check //proto/session/v1 and //tools/bzl/ent (stale ent package list: 'bazel run //tools/bzl/ent:update_packages').`
    - E4 OOM (`Server terminated abruptly`, `java.lang.OutOfMemoryError`, `Killed`): `bazel-hint: the Bazel server ran out of memory or was OOM-killed. Remedy: wait for other Bazel runs to finish, run 'bazel shutdown', then retry; do not raise --jobs.`
    - E5 ENOSPC (`No space left on device`): `bazel-hint: the disk is full. Remedy: run 'bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh' first (dry run: lists orphan output bases, the disk cache size against its GC cap, and stale /tmp test directories, with sizes; removes nothing), review the list, re-run it with --apply, then retry.` The script lands with Epic 1 (Task 11.4a); if it is absent the translator prints instead `bazel-hint: the disk is full. Remedy: free space ('df -h ~/.cache/bazel'), then retry.`
    - E6 cache unreachable (remote-cache connect or timeout warning): `bazel-hint: remote cache unreachable, building locally (results are correct, the build is slower). Remedy: none needed; run 'make bazel-doctor' to check the cache.` Printed once per invocation; the exit code is the build's own.
    - E7 flock queue wait (emitted by the wrapper, not parsed): `bazel: waiting for a build slot (position <p> of <n>, ETA ~<s>s, gives up after <T>s). Another Bazel run is using the host; this is not a hang.` repeated every 15 s as `bazel: still waiting (position <p>, ETA ~<s>s)`; on timeout `bazel: no build slot after <T>s. Remedy: run 'make bazel-doctor' to see the holders, or retry.` with exit code 75. ETA is the median wall time of the last 5 slot holders recorded in `${XDG_STATE_HOME:-~/.local/state}/stapler-squad/bazel-slot-history` (default 120 s when empty); default timeout T is 900 s, overridable by `BAZEL_SLOT_TIMEOUT`.
    - E8 sandbox failure (`permission denied` or `no such file` outside runfiles): `bazel-hint: a test touched something outside its sandbox. Remedy: fix the test to use TEST_TMPDIR or runfiles (see docs/how-to/troubleshoot-bazel.md#state-4); never tag it no-sandbox without a reason comment and an issue.`
    - E9 workspace lock (`Another command (pid=N) is running`): `bazel-hint: another Bazel command holds this worktree's lock; waiting is normal. Remedy: wait, or use a separate worktree.`
    - E11 cache authentication failure (`HTTP 401`, `HTTP 403`, `UNAUTHENTICATED`, `PERMISSION_DENIED`, an expired credential from `credential-helper.sh`; distinct from E6 and louder): printed on every invocation, never once-only, prefixed `!! `, because the build stays correct while the shared cache is silently unused: `!! bazel-hint: the remote cache REJECTED our credentials (HTTP 401/403 or expired), so the shared cache is not being used. Remedy: refresh the credential ('make bazel-doctor' names which one); builds stay correct but slow.` In CI the same match increments `bazel_cache_auth_failures_total` (Task 7.1a) and emits a `::warning` annotation, and `cache-write-health` (Task 7.4b) fails after 3 consecutive runs with it.
    - **Precedence and negative fixtures.** Rows are tried in this order and the first match wins: E9, E5, E4, E11, E6, E2, E3, E10, E8, E1; at most one hint is printed per failure. E4's bare `Killed` counts only when the wrapper saw exit status 137 or a `Server terminated abruptly` / `OutOfMemoryError` line (a test log that merely contains the word `Killed` yields no hint). E8's `no such file` counts only when the line carries a sandbox marker (`linux-sandbox`, `sandbox-exec`, `/sandbox/` or `execroot`); the same words without a marker fall through to E1. Negative fixtures under `tools/bzl/testdata/errors/negative/` (a passing test that prints `Killed`, a passing test asserting on a `no such file` error, a cache warning inside a successful build) must produce no `bazel-hint:` line.
    - **Single source of truth.** The hint texts exist only in `errors.yaml`. The skill (Task 11.1a) and the troubleshooting page (Tasks 1.6e, 11.3b) quote them through `<!-- errors.yaml:E<n> -->` markers, and the guard of Task 11.1d fails if a quoted hint differs from `errors.yaml`, or if a `Remedy:` command is neither in `tools/bzl/verified-commands.txt` nor on a row marked `status: proposal` in `errors.yaml` (rows start as `proposal` and flip to `verified` when their remedy command has been run).
- Raw `bazel` is as quiet as the wrappers.
  - *Given* a non-TTY stdout or `CLAUDECODE=1`, *When* `bazel build //config/...` runs directly (no `make`), *Then* the `tools/bazel` wrapper added `--config=agent` (output at most 40 lines on success, no ANSI); `BAZEL_HUMAN=1` or `--config=human` opts out.
- Long runs do not look hung to an agent with a 2-10 minute Bash timeout.
  - *Given* a build or test running longer than 30 s on a non-TTY, *When* the wrapper is attached, *Then* it prints one heartbeat line (`bazel: still running, <n>s elapsed, log: .bazel-logs/last-build.log`) at 30 s, 60 s, 120 s and every 120 s after, at most 10 heartbeat lines and counted inside the 40-line success cap; `BAZEL_BG=1` (or `make build BG=1`) detaches the run, prints `started in background (pid <p>), log: <path>, poll: bash tools/bzl/bg-status.sh` and returns 0 at once, and `bg-status.sh` prints `running` or `finished exit=<n>` from `.bazel-logs/last-exit`.
- Failure output is capped.
  - *Given* a failing build or test, *When* the wrapper prints, *Then* it prints at most 60 lines (the first `ERROR:` block, the translated hint, then the tail; when output was truncated a line `... <N> more ERROR lines not shown` precedes the log line) and a final line `full log: <worktree>/.bazel-logs/last-build.log` (or `last-test.log`), with `.bazel-logs/` gitignored.
- Doctor and the inverted precondition.
  - *Given* `make bazel-doctor`, *When* it runs, *Then* it reports the nine checks of Task 1.6c and exits 0 only if none failed; `make help` prints the two-line precondition banner.
**Files**: `tools/bzl/errors.yaml`, `tools/bzl/translate-errors.sh`, `tools/bazel` (wrapper from Task 0.2f), `tools/bzl/doctor.sh`, `tools/bzl/bg-status.sh`, `tools/bzl/check-build-fresh-cheap.sh`, `docs/how-to/troubleshoot-bazel.md`

##### Task 1.6a: Error-translation table and translator (cost S)
- Create `tools/bzl/errors.yaml` (rows E1-E11 with `match`, `source` (`bazel` or `go`), `priority` and `status` fields and the verbatim hint text above) and `tools/bzl/translate-errors.sh` (reads stdin (`--source bazel|go` selects the row set, default `bazel`), passes the original text through, appends at most one hint per row, each hint followed by `See docs/how-to/troubleshoot-bazel.md#state-<n>` for states that have a section). Fixture logs are reproduced from real failures during this task (not recalled) and stored under `tools/bzl/testdata/errors/`.
- Files: `tools/bzl/errors.yaml`, `tools/bzl/translate-errors.sh`, `tools/bzl/translate-errors_test.sh`, `tools/bzl/testdata/errors/*.log`, `tools/bzl/testdata/errors/negative/*.log`
- Verify: `bash tools/bzl/translate-errors_test.sh` prints `PASS` x11 (one per row, verbatim compare) and `bash tools/bzl/translate-errors_test.sh --negative` prints `PASS` for every file under `negative/` (no `bazel-hint:` line) and for the E1-versus-E8 pair (exactly one hint each, the right one); `shellcheck tools/bzl/translate-errors.sh` exits 0.

##### Task 1.6b: Wrapper: agent config, output cap, queue-wait messages (cost M)
- Extend the `tools/bazel` wrapper (Task 0.2f): auto-add `--config=agent` when stdout is not a TTY (never for `bazel query`, which rejects a `build:` config, `[RUN]` S11; `cquery`/`aquery`/`info` are unverified) or `CLAUDECODE=1`/`STAPLER_SQUAD_SESSION` is set (opt out via `BAZEL_HUMAN=1`); tee output to `.bazel-logs/last-<build|test>.log`; on failure print the 60-line cap view plus the log path; pipe through the translator; print the E7 messages while waiting for the flock slot (position, ETA, 15 s repeat) and exit 75 on timeout; record each slot holder's wall time to the history file. Also: print the heartbeat lines and the `... <N> more ERROR lines not shown` line of the Story 1.6 ACs; write `.bazel-logs/last-exit` (exit code and finish time) and support `BAZEL_BG=1` with `tools/bzl/bg-status.sh`; print the one-line BUILD-staleness note before `build`/`test` by calling `tools/bzl/check-build-fresh-cheap.sh --warn` (Task 1.6f, later in this story, supplies the script; until it exists the wrapper skips the note). Also: **cold-first-build note** - when the worktree has no server and an empty output base, print once `note: first build in this worktree is cold, about <N> min (later builds are cached)`, where N is the median of the cold samples in the slot-history file, defaulting to the S6 first-worktree cold median (Task 0.10a) until three samples exist. **Auto-detach** - the wrapper always runs Bazel as a child writing to the log, so `BAZEL_AUTO_DETACH_SECS=<n>` (default `0` = off for humans, `600` when `--config=agent` was auto-added) makes it stop waiting after n seconds, print `detached after <n>s; poll with tools/bzl/bg-status.sh`, and exit `76` (wrapper-defined: detached, result pending); the child keeps running and `bg-status.sh` later prints `finished exit=<code>`. Add `.bazel-logs/` to `.gitignore`.
- Files: `tools/bazel`, `tools/bzl/wrapper_test.sh`, `tools/bzl/slot_history.sh`, `tools/bzl/bg-status.sh`, `.gitignore`
- Verify: `bash tools/bzl/wrapper_test.sh` passes these cases: raw `bazel build //config/... 2>&1 | wc -l` is at most `40`; a seeded failure prints at most `60` lines and ends with `full log:`; three holders plus one waiter print the E7 first message within 2 s; `BAZEL_SLOT_TIMEOUT=3` makes the waiter print the timeout line and exit `75`; a 70 s synthetic run prints 3 heartbeat lines and never more than 10; `BAZEL_BG=1` returns within 2 s printing `started in background` and `bash tools/bzl/bg-status.sh` later prints `finished exit=0`; a failure with 120 `ERROR:` lines prints at most 60 lines including `... <N> more ERROR lines not shown`; with an empty output base the first call prints exactly one `first build in this worktree is cold` line and the second call prints none; `BAZEL_AUTO_DETACH_SECS=2` over a 6 s synthetic run returns within 4 s printing `detached after` with exit `76`, and `bg-status.sh` later prints `finished exit=0`; a wrapper killed with `kill -9` while holding a slot frees it for the next waiter within 3 s (same case as Task 0.2f, run against the extended wrapper).

##### Task 1.6c: `make bazel-doctor` and the inverted-precondition banner (cost S)
- `tools/bzl/doctor.sh` runs nine checks and prints one line each: (1) `bazelisk`/Bazel version equals `.bazelversion`; (2) remote cache reachable within 3 s (failure prints the E6 text); (3) free space on the output-base and disk-cache filesystem at least 15% (E5 text if not), plus a warning when the two are on different filesystems (`stat -f -c %T`; on btrfs a same-filesystem cache hit is a reflink, otherwise outputs are copied, D3); (4) current slot holders, their start times and the ETA (the E7 data); (5) where this worktree's state lives: the `bazel-bin`/`bazel-out`/`bazel-testlogs` symlinks in the worktree root (gitignored) and the output base under `~/.cache/bazel/_bazel_$USER/<hash>`; (6) `precondition: Bazel needs no generated code; plain go and gopls need 'make sync' - gen/ is <fresh|STALE: run 'make sync'>` using the cheap check of Task 2.4d (skipped with a note until that task lands); (7) memory and uptime: available memory from `/proc/meminfo` (warn below 8 GB and name the largest RSS holders), the 1-minute load average (warn above 0.8 x core count) and the Bazel server's age and RSS (`bazel info server_pid`, `ps -o etimes,rss`), plus a warning when `$HOME/.bazelrc` sets a `-Xmx` (it overrides the committed 2g, rc files being read in order; the wrapper still forces 2g, D12) or `/etc/bazel.bazelrc` exists (a system rc that whitelists `PATH` can cut cache hits, `[DOC]`); (8) BUILD freshness from the cheap check of Task 1.6f (`BUILD: fresh` or `BUILD: MAYBE STALE for <pkgs> - run 'make gazelle'`); (9) the git merge drivers are configured (`git config --get merge.bazel-lock.driver`, set per clone by `make setup-git`). The report ends with the repro hint `to reproduce a failing test: bazel test <target> --sandbox_debug --test_output=streamed --nocache_test_results`. `make help` prints the same precondition as a two-line header; `tools/bzl/check-make-help.sh` asserts the header.
- Files: `tools/bzl/doctor.sh`, `tools/bzl/doctor_test.sh`, `Makefile`, `make/bazel.mk`, `tools/bzl/check-make-help.sh`
- Verify: `make bazel-doctor; echo $?` prints nine check lines plus the repro hint line, then `0` on a healthy host; `bash tools/bzl/doctor_test.sh` seeds a wrong version, an unreachable cache, a full-disk stub, a cache on a different filesystem from the output base, a `~/.bazelrc` with `-Xmx8g`, a held slot, a stale `gen/`, low memory (stubbed `/proc/meminfo`), a stale BUILD file and an unset merge driver and asserts each prints its hint and exits non-zero; `make help | head -3 | grep -c 'make sync'` prints `1`.

##### Task 1.6d: First-error legibility check (cost S)
- `tools/bzl/check-error-legibility.sh` replays the 9 fixtures of target T5 authored in Task 0.1h (`tools/bzl/baseline/fixtures/*.sh`, not re-authored here) (stale BUILD, missing external dep, missing generated code, OOM, ENOSPC, cache unreachable, flock queue wait, sandbox failure, cache auth rejected) through the wrapper and asserts each prints a `Remedy:` line within its first 20 lines and at most 60 lines in total. It runs in `bazel.yml` and is the T5 collector for P10 (Task 7.5g).
- Files: `tools/bzl/check-error-legibility.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/check-error-legibility.sh | tail -1` prints `LEGIBLE: 9/9`; deleting one fixture's remedy text makes it print `LEGIBLE: 8/9` and exit 1.

##### Task 1.6e: Troubleshooting stub for the first five error states (cost S)
- Pull forward from `research/ux.md` section 5 the states most likely to hit in Epic 1-2: 1 (stale BUILD), 2 (missing external dep), 3 (generated code absent), 4 (sandbox failure) and 10 (remote cache unreachable or credentials missing). Create `docs/how-to/troubleshoot-bazel.md` with five `### State <n>: <title>` sections (anchors `#state-1` ... used by the translator), each with a symptom string copied from a fixture of Task 1.6a and the remedy; Task 11.3b later extends the same file to all 11 states without renaming these headings.
- Files: `docs/how-to/troubleshoot-bazel.md`
- Verify: `grep -c '^### State' docs/how-to/troubleshoot-bazel.md` is at least `5`; each of states 1, 2, 3, 4, 10 appears (`grep -E '^### State (1|2|3|4|10):' docs/how-to/troubleshoot-bazel.md | wc -l` prints `5`).

##### Task 1.6f: Cheap BUILD-staleness warning before every build and test (cost S)
- `tools/bzl/check-build-fresh-cheap.sh` (needs no Bazel, target under 300 ms): takes the `.go` files changed against `HEAD` plus untracked ones (`git status --porcelain -- '*.go'`) and, for every package where a Go file was added, removed, renamed or had an import line changed (`git diff -U0 HEAD -- <file> | grep -E '^[+-][[:space:]]*(import )?"[^"]+"$'`) while that package's `BUILD.bazel` is not itself modified, prints `note: BUILD.bazel may be stale for <pkg> - run 'make gazelle'`; otherwise `BUILD: fresh`. With `--warn` (the wrapper's mode) it exits 0; without it, exit 1 on a stale hit. It is a heuristic with possible false negatives; the authoritative gate stays `gazelle -mode=diff` in CI. The wrapper `tools/bazel` runs it before `build` and `test`, and Task 8.2d (later; not needed for this task's verify) wires it into `make build` and `make test`; `make bazel-doctor` check 8 reuses it.
- Files: `tools/bzl/check-build-fresh-cheap.sh`, `tools/bzl/check-build-fresh-cheap_test.sh`, `tools/bazel`
- Verify: `bash tools/bzl/check-build-fresh-cheap_test.sh` exits 0 (cases: a new `.go` file without a BUILD change warns; a new import warns; the same change with `BUILD.bazel` also modified is silent; a clean tree prints `BUILD: fresh`); `time bash tools/bzl/check-build-fresh-cheap.sh --warn` real time is under 0.5 s.

---

## Epic 2: Codegen as Bazel actions (proto, ent, sync-gen)

**Goal**: `bazel build //...` works on a fresh clone with no `make proto-gen ent-gen` precondition; `make sync` keeps `go`/gopls working (ADR-002). Mechanisms are those chosen by spikes S1, S2, S11 (the tasks below name the default; swap per verdict).

#### Story 2.1: Proto to Go and connect-go
**As an** implementer, **I want** Bazel to produce `gen/proto/go/...` equivalents, **so that** importers resolve without a synced tree.
**Acceptance Criteria**:
- Equivalent output.
  - *Given* `proto/session/v1/*.proto`, *When* `bash tools/bzl/parity/compare-proto.sh` runs, *Then* exit 0 (P4, Go half).
- Importers resolve.
  - *Given* `server/services/*.go` importing `github.com/tstapler/stapler-squad/gen/proto/go/session/v1`, *When* `bazel build //server/services` runs on a fresh clone, *Then* exit 0.
**Files**: `proto/session/v1/BUILD.bazel`, `tools/bzl/proto/BUILD.bazel`, `BUILD.bazel`

##### Task 2.1a: HandBuildFile for proto per S1 verdict (cost M)
- Productionise Task 0.3a (or 0.3d); header comment "managed by hand, not Gazelle"; `# gazelle:ignore`.
- Files: `proto/session/v1/BUILD.bazel`, `tools/bzl/proto/BUILD.bazel`
- Verify: `bazel build //proto/session/v1/...` exits 0.

##### Task 2.1b: Make the pre-landed resolve labels real (cost S)
- The `gazelle:resolve` lines for `gen/proto/go/session/v1` and `.../sessionv1connect` and the `gazelle:exclude gen` already landed in Task 1.1a; here only verify that the labels they name (`//proto/session/v1:session_go_proto`, `:session_connect_go`) exist with those exact names and fix the BUILD target names, not the resolve lines.
- Files: `proto/session/v1/BUILD.bazel`
- Verify: `bazel run //:gazelle -- -mode=diff; echo $?` prints `0` with and without a synced `gen/`.

##### Task 2.1c: Wire P4 compare into tools (cost S)
- Finalise `compare-proto.sh` (from 0.3b) with `gofmt` normalisation and a readable diff; document in the reference page later.
- Files: `tools/bzl/parity/compare-proto.sh`
- Verify: `bash tools/bzl/parity/compare-proto.sh; echo $?` prints `0`.

#### Story 2.2: Proto to TypeScript for the web build
**As an** implementer, **I want** TS generated by Bazel, **so that** the web build no longer depends on `web-app/src/gen/` existing.
**Acceptance Criteria**:
- *Given* proto sources, *When* `bazel build //proto/session/v1:session_ts` runs, *Then* the output tree equals `web-app/src/gen` from `make proto-gen` (`diff -r` exit 0).
**Files**: `proto/session/v1/BUILD.bazel`, `web-app/BUILD.bazel`

##### Task 2.2a: TS action (cost M)
- Productionise Task 0.6e (the S1-ts spike); `protoc-gen-es` comes from the pinned Bun install (ADR-003), so this has a hard dependency on Task 3.1a (Story 3.1's repository rule); land the repository rule first as Task 3.1a (Story 2.2 is listed in wave W7, but this task starts only after Task 3.1a of wave W8 has landed). If S4 failed, use the pinned-tarball `protoc-gen-es` repository rule from the spike protocol.
- Files: `proto/session/v1/BUILD.bazel`
- Verify: `diff -r bazel-bin/proto/session/v1/ts web-app/src/gen` exits 0.

#### Story 2.3: ent generation
**As an** implementer, **I want** `//session/ent` built by Bazel with `--feature sql/upsert`, **so that** no `make ent-gen` precondition exists.
**Acceptance Criteria**:
- Clean-clone build.
  - *Given* a fresh clone, *When* `bazel build //session/... //server/...` runs, *Then* exit 0 and `grep -rl OnConflict bazel-bin/tools/bzl/ent | head -1` prints a file.
- Equivalent output.
  - *Given* `make ent-gen` output in a temp copy, *When* `bash tools/bzl/parity/compare-ent.sh` runs, *Then* exit 0 after `gofmt` (P4, ent half).
- Deterministic.
  - *Given* two clean output bases, *When* the ent target builds in each, *Then* tree hashes match.
**Files**: `session/ent/BUILD.bazel`, `tools/bzl/ent.bzl`, `tools/bzl/ent/BUILD.bazel`, `tools/bzl/parity/compare-ent.sh`, `BUILD.bazel`

##### Task 2.3a: ent macro and HandBuildFile (cost M)
- Productionise Tasks 0.4a/0.4b into `tools/bzl/ent.bzl` (at most 40 lines, with a `bazel_skylib` analysis test), the package `tools/bzl/BUILD.bazel` (created here, a HandBuildFile that holds `ent_bzl_test`; Task 4.2a later adds to it, it does not create it), `tools/bzl/ent/BUILD.bazel` (one `go_library` per package from `packages.txt`, tracked) and `session/ent/BUILD.bazel`; `schema/` stays Gazelle-managed. The package list has a freshness test (fails with `Run: bazel run //tools/bzl/ent:update_packages`) that rebuilds the tree and compares it with `packages.txt`; the `make ent-gen` recipe reads the same `ent_flags.txt`.
- Files: `tools/bzl/ent.bzl`, `tools/bzl/BUILD.bazel`, `tools/bzl/ent/BUILD.bazel`, `tools/bzl/ent/ent_flags.txt`, `session/ent/BUILD.bazel` (`tools/bzl/ent/packages.bzl` is written by `bazel run //tools/bzl/ent:update_packages`, not by hand)
- Verify: `bazel build //tools/bzl/ent:all && bazel test //tools/bzl:ent_bzl_test //tools/bzl/ent:packages_fresh_test` exits 0; `bazel build //session/... //server/...` exits 0.

##### Task 2.3b: Verify ent resolve labels, or Gazelle extension per S11 (cost S)
- The ent resolve lines landed in Task 1.1a; verify every importer of `session/ent` and its subpackages (list from `go list -f '{{.Imports}}'`) resolves to an existing `//tools/bzl/ent:<name>` target; if the S11 threshold said extension, create `tools/gazelle/ent` with a Go unit test instead (cost M, only then).
- Files: `BUILD.bazel` (or `tools/gazelle/ent/*.go`, `tools/gazelle/BUILD.bazel`)
- Verify: `bazel run //:gazelle -- -mode=diff; echo $?` prints `0`.

##### Task 2.3c: compare-ent.sh and determinism check (cost S)
- Script runs `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` in a temp copy and diffs normalised output; add a determinism job target later used in Story 7.7.
- Files: `tools/bzl/parity/compare-ent.sh`
- Verify: `bash tools/bzl/parity/compare-ent.sh; echo $?` prints `0`.

#### Story 2.4: SyncGen and GenFreshness
**As an** agent, **I want** one command that makes plain `go build ./...` work, **so that** gopls, golangci-lint and habits survive.
**Acceptance Criteria**:
- Sync materialises.
  - *Given* a fresh clone, *When* `make sync && go build ./...` runs, *Then* exit 0 and `git status --porcelain` shows no new tracked/untracked-visible files (everything stays gitignored).
- Bazel ignores the copy.
  - *Given* a deliberately corrupted `gen/proto/go/session/v1/session.pb.go`, *When* `bazel build //server/services` runs, *Then* exit 0 (Bazel did not read it).
- Staleness is detected.
  - *Given* a changed `proto/session/v1/session.proto` and an unsynced `gen/`, *When* `make gen-freshness` runs, *Then* it exits 1 and prints `run: make sync`.
**Files**: `BUILD.bazel`, `tools/bzl/sync_gen.sh`, `tools/bzl/check-gen-fresh.sh`, `Makefile`

##### Task 2.4a: //:sync-gen target (cost M)
- `bazel run //:sync-gen` copies proto Go, connect, TS and ent outputs into `gen/`, `web-app/src/gen/`, `session/ent/` (never BUILD files); respects `BUILD_WORKSPACE_DIRECTORY`; refuses to run inside the runfiles sandbox.
- Files: `BUILD.bazel`, `tools/bzl/sync_gen.sh`
- Verify: `bazel run //:sync-gen && go build ./... ; echo $?` prints `0`.

##### Task 2.4b: make sync and GenFreshness check (cost S)
- Extend `sync` (skeleton from Task 1.5a, which already runs the legacy `proto-gen ent-gen` under `make`; honours `BUILD_SYSTEM` from Story 1.5; under `make` it runs the legacy `proto-gen ent-gen`), `gen-freshness` calling `check-gen-fresh.sh` (hash of synced tree vs `bazel-bin`). `gen-freshness` joins `make ready` only when `BUILD_SYSTEM=bazel` (or Bazel is installed and `.bazelversion` exists); under `BUILD_SYSTEM=make` it prints `skipped: BUILD_SYSTEM=make` and exits 0, so the make fallback never depends on Bazel.
- Files: `Makefile`, `tools/bzl/check-gen-fresh.sh`
- Verify: `make sync && make gen-freshness; echo $?` prints `0`; `echo '// x' >> gen/proto/go/session/v1/session.pb.go; make gen-freshness; echo $?` prints `1` with `run: make sync` in the output; `make sync && make gen-freshness; echo $?` prints `0` again; `make help | grep -E '^ *sync'` prints one line.

##### Task 2.4c: Feature-registry generated inputs decision (cost XS)
- Validation G3 (requirements I4: "feature-registry/other generated inputs if applicable"). Decision recorded in ADR-002: `docs/registry/features/*.json` is a committed, scan-derived artifact (not gitignored generated code) produced by `make registry-generate`, so it stays outside Bazel (a PermanentException row in ADR-005). CI keeps running `make registry-generate` and fails on a non-empty `git diff -- docs/registry` under both `BUILD_SYSTEM` values; `affected-targets.sh` maps `docs/registry/**` to that job through `tools/bzl/non-bazel-inputs.txt` (Task 7.3a, later, creates that file and lists `docs/registry/**` in it; until then the job runs unconditionally).
- Files: `decisions/ADR-002-generated-code-bazel-only-with-sync-gen.md`, `decisions/ADR-005-permanent-exceptions-outside-bazel.md`
- Verify: `grep -c 'registry-generate' decisions/ADR-002-generated-code-bazel-only-with-sync-gen.md decisions/ADR-005-permanent-exceptions-outside-bazel.md` prints at least `1` per file; `make registry-generate && git diff --exit-code docs/registry` exits 0.

##### Task 2.4d: Cheap input-hash GenFreshness form (cost S)
- Add a second, cheap form of the freshness check that needs no Bazel: `make sync` (both `BUILD_SYSTEM` values) writes `gen/.sync-inputs.sha256` (gitignored) holding the sha256 over the sorted contents of `proto/**/*.proto`, `buf*.yaml`, `session/ent/schema/**/*.go` and `session/ent/generate.go`; `tools/bzl/check-gen-fresh-cheap.sh` recomputes it (target under 200 ms) and prints `gen: fresh` or `gen: STALE - run 'make sync'` (exit 1, or exit 0 with `--warn`). The expensive `check-gen-fresh.sh` (hash vs `bazel-bin`) stays the CI gate. Because the cheap form needs only the stamp, it works under `BUILD_SYSTEM=make`, which also settles the make-mode variant deferred in the Phase 4 disposition (pre-mortem #7).
- Files: `tools/bzl/check-gen-fresh-cheap.sh`, `tools/bzl/check-gen-fresh-cheap_test.sh`, `tools/bzl/sync_gen.sh`, `Makefile`
- Verify: after `make sync`, `bash tools/bzl/check-gen-fresh-cheap.sh; echo $?` prints `gen: fresh` and `0`; after `touch -a` is not enough, appending a comment to a `.proto` prints `gen: STALE` and `1`; `time bash tools/bzl/check-gen-fresh-cheap.sh` real time is under 0.5 s.

##### Task 2.4e: Native-go shims and the plain-go failure path (cost S)
- Raw `go build` and gopls never pass through the Bazel wrapper, so the plain-go failure gets its own remedy: `make go-build`, `make go-vet` and `make go-test PKG=... RUN=...` run `check-gen-fresh-cheap.sh`, run `make sync` automatically when the stamp is stale, print the banner `[native go - not authoritative; Bazel is the build of record]`, run the native tool and pipe a failure through `translate-errors.sh --source go` (row E3). `docs/how-to/build-with-bazel.md` gets a section "gopls reports 'could not import ...gen/proto'" (run `make sync`, then restart gopls), and `make bazel-doctor` check 6 reports the same staleness. A raw `go build` after a pull is unprotected by design; the shims and the doctor line are the supported path.
- Files: `Makefile`, `make/bazel.mk`, `tools/bzl/translate-errors.sh`, `tools/bzl/check-wrappers.sh`
- Verify: with a stale stamp, `make go-build 2>&1 | head -3 | grep -c 'native go'` prints `1` and `make go-build; echo $?` ends with `0` after the automatic sync; `printf 'cannot find package "x/gen/proto/go/session/v1"' | bash tools/bzl/translate-errors.sh --source go | grep -c "Remedy: run 'make sync'"` prints `1`.

#### Story 2.5: buf lint as a Bazel test
**As an** implementer, **I want** proto lint in the graph, **so that** `proto-lint` needs no wrapper logic.
**Acceptance Criteria**:
- *Given* a seeded lint violation in a copy of the proto, *When* `bazel test //proto/session/v1:buf_lint_test` runs, *Then* it fails with the buf rule name; clean tree passes. (`proto-breaking` stays wrapped: needs git history, ADR-005.)
**Files**: `proto/session/v1/BUILD.bazel`, `MODULE.bazel`

##### Task 2.5a: rules_buf lint test (cost S)
- Add `rules_buf` 0.5.4 and `buf_lint_test`; it must succeed in the no-network sandbox, which holds because Task 0.3e removed or vendored the `buf.build/googleapis/googleapis` dependency.
- Files: `MODULE.bazel`, `proto/session/v1/BUILD.bazel`
- Verify: `bazel test //proto/session/v1:buf_lint_test` exits 0 on main.

#### Story 2.6: Full-graph build on a fresh clone
**As the** owner, **I want** the hermetic fresh-clone driver proven, **so that** metric 2 is met.
**Acceptance Criteria**:
- *Given* `git clone` into an empty short temp directory with no generation step, *When* `bazel build //...` runs, *Then* exit 0 for all non-tag-gated, non-Windows, non-embed-dependent targets, and the subset files are removed (the full `//...` claim of requirements Metric 2 is asserted by P3 after Epic 3).
**Files**: `tools/bzl/subset-no-gen.sh` (deleted), `tools/bzl/no-gen-packages.txt` (deleted), `.github/workflows/bazel.yml`

##### Task 2.6a: Remove the subset and expand CI to //... (cost S)
- Hard dependency: Task 3.1a (the `//...` build includes `//web-app`; wave W8, after 3.1a). Delete the two subset files; CI runs `bazel build --keep_going //...` and `bazel test` on what passes; remaining failures are Epic 3/4 items tracked in the ExclusionsFile.
- Files: `.github/workflows/bazel.yml`, deleted subset files
- Verify: in a temp clone `bazel build //... ; echo $?` prints `0` (embed targets excluded by `target_compatible_with` until Epic 3).

#### Story 2.7: Mid-point economic re-check (decision gate after Epic 1 and Stories 2.1, 2.3, 2.4, 2.5)
**As the** owner, **I want** the improvement targets re-evaluated with real Bazel numbers before the rest of Epic 3 spends effort, **so that** a failed economic assumption (narrow fan-out, cache payoff, edit-build-test latency) is found at the cheapest point.
**Acceptance Criteria**:
- *Given* Epic 1 and Stories 2.1, 2.3, 2.4 and 2.5 merged (Stories 2.2 and 2.6 wait for Task 3.1a and are not inputs), *When* `bash tools/bzl/economic-midpoint.sh` runs, *Then* it prints T1 (replaying the 30 sampled diffs of Task 0.1i with `bazel query 'rdeps(//..., set(<files>))'`), T3 (second-worktree cold-build ratio and the 5-worktree aggregate disk) and T4 (warm `bazel test //config:all` edit-build-test latency, a proxy because the `make` wrapper arrives in Epic 8) against `targets.yaml`, and `python3 tools/bzl/economic-gate.py --midpoint` prints `ECONOMIC GATE (midpoint): CLEAR` (exit 0) or `ECONOMIC GATE (midpoint): OWNER DECISION <criteria>` (exit 3). On exit 3, Task 3.1b onward and Stories 3.2-3.7 stay unreleased until `owner-attestations.yaml` has `economic_midpoint: {decision, date, reason}` with `decision` one of `continue`, `re-plan`, `pause`, `revert`. The script never edits scope.
**Files**: `tools/bzl/economic-midpoint.sh`, `tools/bzl/economic-gate.py`, `tools/bzl/economic-gate_test.py`, `owner-attestations.yaml`

##### Task 2.7a: Mid-point economic re-check (cost S)
- Write `economic-midpoint.sh` and extend `economic-gate.py` with `--midpoint` (unit-tested). It reuses the replay and measurement functions of Tasks 0.1i and 0.10a; Task 7.5g later reuses the same T1 replay.
- Files: `tools/bzl/economic-midpoint.sh`, `tools/bzl/economic-gate.py`, `tools/bzl/economic-gate_test.py`, `owner-attestations.yaml`
- Verify: `python3 -m unittest tools/bzl/economic-gate_test.py` exits 0 including midpoint fixtures (one tripping each of T1, T3 and T4, one clear); `bash tools/bzl/economic-midpoint.sh | grep -cE '^(T1|T3|T4): (PASS|FAIL)$'` prints `3`; `python3 tools/bzl/economic-gate.py --midpoint; echo $?` prints `0` or `3`, and on `3` the `economic_midpoint` key exists in `owner-attestations.yaml` before Task 3.1b starts.

---

## Epic 3: Embeds, stamping, aux binaries, release

**Goal**: `bazel build --config=release //:stapler-squad` produces the embedded binary with the same behaviour as `make build-embedded-tymux` (ADR-007, ADR-003, ADR-005).

#### Story 3.1: Web dist as a Bazel target
**As an** implementer, **I want** `//web-app:dist` and Jest tests, **so that** the Go server can embed a hermetically built UI.
**Acceptance Criteria**:
- Parity.
  - *Given* `bun.lock`, *When* `bash tools/bzl/parity/compare-web.sh` runs, *Then* it exits 0 (P6): file list and sha256 equal to `make web-build` or deltas are listed with a reason.
- Jest parity.
  - *Given* the full Jest suite, *When* run as `bazel test //web-app:jest` versus `cd web-app && bunx jest --no-coverage`, *Then* the pass-set diff is empty.
- No source-tree writes.
  - *Given* a clean tree, *When* `bazel build //web-app:dist` runs, *Then* `git status --porcelain` is empty.
**Files**: `MODULE.bazel`, `tools/bzl/bun/bun.bzl`, `tools/bzl/bun/BUILD.bazel`, `web-app/BUILD.bazel`, `tools/bzl/parity/compare-web.sh`

##### Task 3.1a: Bun repository rule (production) (cost M)
- Productionise Task 0.6a per ADR-003 (or the chosen fallback); pin Bun by sha256 per OS/arch. A repository rule cannot carry an action tag and is never remote-cached, so `no-remote-cache` goes on the consuming web actions (`dist`, Jest, proto TS generation) until Story 7.7 shows the install is byte-reproducible; a non-reproducible `node_modules` costs cache misses, not poisoning. Record sandbox setup time with the real `node_modules` (S4 criterion) and apply `no-sandbox` to the consuming actions only if the 1.5x threshold was exceeded.
- Files: `tools/bzl/bun/bun.bzl`, `MODULE.bazel`
- Verify: `bazel build @web_node_modules//:all` exits 0; second run is a cache hit (`bazel build ... 2>&1 | grep -c 'INFO: 1 process'` equals 0 processes executed).

##### Task 3.1b: //web-app:dist (cost M)
- Action runs `bun build/build.ts` with declared inputs (`glob(src/**)`, config, TS proto gen from Story 2.2); outputs a tree artifact `dist`.
- Files: `web-app/BUILD.bazel`
- Verify: `bazel build //web-app:dist` exits 0 and `ls bazel-bin/web-app/dist/index.html` exists.

##### Task 3.1c: Jest target (cost S)
- Productionise Task 0.6c to the full suite, sharded by file group (`shard_count`), `size = "large"`.
- Files: `web-app/BUILD.bazel`
- Verify: `bazel test //web-app:jest` exits 0.

##### Task 3.1d: Web parity script (cost S)
- `compare-web.sh`: build both ways, compare file lists and hashes, print deltas.
- Files: `tools/bzl/parity/compare-web.sh`
- Verify: `bash tools/bzl/parity/compare-web.sh; echo $?` prints `0`.

#### Story 3.2: server/web embed staging (ADR-007)
**As an** implementer, **I want** `server/web` to embed the Bazel-built dist with no Go change, **so that** revert stays one PR.
**Acceptance Criteria**:
- *Given* `//web-app:dist` staged at `server/web/dist`, *When* `bazel test //server/web:all` runs, *Then* exit 0 and the embedded FS contains `dist/index.html`.
- *Given* `git diff origin/main -- server/web/embed.go`, *When* the story merges, *Then* the diff is empty.
**Files**: `server/web/BUILD.bazel`

##### Task 3.2a: HandBuildFile with staging (cost S)
- Productionise Task 0.5a (or the S3 fallback); `# gazelle:ignore`; `embedsrcs` from staged tree.
- Files: `server/web/BUILD.bazel`
- Verify: `bazel test //server/web:all` exits 0; `git diff --stat origin/main -- server/web/embed.go` is empty.

#### Story 3.3: tmux and tymuxd embeds
**As an** implementer, **I want** the embedded tmux and tymuxd to come from pinned sources, **so that** `embed_tmux`/`embed_tymux` builds are reproducible and fresh worktrees work.
**Acceptance Criteria**:
- Fresh worktree.
  - *Given* a clean clone with an empty `third_party/tmux`, *When* `bazel build --config=embed_tymux //session/tmux //session/tymux` runs, *Then* exit 0, and `git ls-files third_party/tmux` shows only the gitlink (tmux comes from the `http_archive` in `tools/bzl/tmux`, the embed flags from `tools/bzl/flags`; neither lives under the submodule or the gitignored `build/`).
- Pins checked.
  - *Given* `scripts/tymuxd-checksums.txt` and `MODULE.bazel` `http_file` sha256 values, *When* `bash tools/ci/check-tymux-pin.sh` runs, *Then* exit 0 and it also fails if a `MODULE.bazel` sha256 is edited without the checksums file.
**Files**: `tools/bzl/tmux/BUILD.bazel`, `tools/bzl/tmux/PIN`, `tools/bzl/flags/BUILD.bazel`, `MODULE.bazel`, `session/tmux/embed/BUILD.bazel`, `session/tymux/embed/BUILD.bazel`, `tools/ci/check-tymux-pin.sh`

##### Task 3.3a: tmux target per S5 verdict (cost M)
- Productionise Task 0.7a or 0.7b at the tracked path `tools/bzl/tmux/`; `.bazelignore` and `gazelle:exclude` keep the submodule path out of the graph; mark hermeticity exception in a header comment if host cc is used (covered by D-HERMETIC), together with the local-cache protection chosen in Task 0.7b (D22: `no-cache`, or the host cc version declared as an input).
- Files: `tools/bzl/tmux/BUILD.bazel`, `MODULE.bazel`
- Verify: `bazel build //tools/bzl/tmux:tmux_bin && bazel-bin/tools/bzl/tmux/tmux -V` prints `tmux 3.8`; `git check-ignore tools/bzl/tmux/BUILD.bazel` prints nothing.

##### Task 3.3b: tymuxd http_file (cost S)
- One `http_file` per OS/arch with sha256 from `scripts/tymuxd-checksums.txt` (same URL source as `scripts/fetch-tymuxd.sh`).
- Files: `MODULE.bazel`
- Verify: `bazel build @tymuxd_linux_amd64//file` exits 0.

##### Task 3.3c: Embed staging with select (cost S)
- `session/tmux/embed/BUILD.bazel` and `session/tymux/embed/BUILD.bazel` stage `embed/tmux`, `embed/tymuxd`; `session/tmux` and `session/tymux` `go_library` use `select()` on `//tools/bzl/flags:embed_tmux` / `//tools/bzl/flags:embed_tymux` (Story 0.8b).
- Files: `session/tmux/embed/BUILD.bazel`, `session/tymux/embed/BUILD.bazel`, `tools/bzl/flags/BUILD.bazel`
- Verify: `bazel build --config=embed_tymux //session/tmux //session/tymux` exits 0; without the config both also build (empty embeds, tagged files excluded).

##### Task 3.3d: Pin check covers MODULE.bazel (cost S)
- Extend `tools/ci/check-tymux-pin.sh` to compare `MODULE.bazel` sha256/tag against `scripts/tymuxd-checksums.txt` and `tools/bzl/tmux/PIN`, and the submodule gitlink commit (`git ls-tree HEAD third_party/tmux`) against the `PIN` commit.
- Files: `tools/ci/check-tymux-pin.sh`, `tools/ci/check-tymux-pin_test.sh`
- Verify: `bash tools/ci/check-tymux-pin_test.sh` exits 0 including a mutated-pin case that must fail.

#### Story 3.4: Root binary, stamping, release config
**As the** owner, **I want** `//:stapler-squad` with version stamping and the embeds, **so that** the shipped binary equals today's.
**Acceptance Criteria**:
- Behaviour parity (P5).
  - *Given* a Bazel build with `--config=release --stamp` and `make build-embedded-tymux` on the same commit, *When* `bash tools/bzl/parity/compare-binary.sh` runs, *Then* `stapler-squad version` output is equal, `go version -m` shows non-empty version/commit/branch values, and the embedded tmux and tymuxd sha256 equal the pinned values.
- Stamping only relinks.
  - *Given* a new commit touching no Go file, *When* `bazel build --config=release //:stapler-squad` runs, *Then* only `GoLink` and stamping actions execute (`aquery`/`--explain` evidence).
**Files**: `tools/bzl/workspace_status.sh`, `BUILD.bazel`, `.bazelrc`, `tools/bzl/parity/compare-binary.sh`

##### Task 3.4a: workspace_status.sh (cost S)
- Emit `STABLE_GIT_VERSION`, `STABLE_GIT_BRANCH`, `STABLE_GIT_COMMIT`, `STABLE_GIT_WORKTREE` using the same sanitiser as `Makefile:22`; no volatile keys.
- Files: `tools/bzl/workspace_status.sh`
- Verify: `bash tools/bzl/workspace_status.sh | grep -c '^STABLE_'` prints `4`.

##### Task 3.4b: //:stapler-squad go_binary (cost S)
- Gazelle-generated rule plus `x_defs` for `main.version` and the buildinfo package fields, `embedsrcs = ["CHANGELOG.md", ...]`; `client_pages.json` embed; `--config=release` sets `--stamp`, `compilation_mode=opt`, `embed_tymux`.
- Files: `BUILD.bazel`, `.bazelrc`, `server/analytics/BUILD.bazel`
- Verify: `bazel build --config=release //:stapler-squad` exits 0.

##### Task 3.4c: compare-binary.sh (P5) (cost S)
- Script builds both, runs `version` on each, compares `go version -m`, extracts embedded tmux/tymuxd via test hook or `strings`/known-offset check, compares sha256.
- Files: `tools/bzl/parity/compare-binary.sh`
- Verify: `bash tools/bzl/parity/compare-binary.sh; echo $?` prints `0`.

##### Task 3.4d: `godebug netdns=go` preserved in the Bazel binary (cost XS)
- Requirements rabbit hole and validation G2: `go.mod` declares `godebug netdns=go`, and nothing yet shows a rules_go-built binary honours it. Extend `compare-binary.sh` so `go version -m <bazel binary>` and `go version -m <make binary>` report the same `DefaultGODEBUG` and it contains `netdns=go`. If rules_go does not carry the directive, set it through the link-time default (`x_defs` on `runtime.godebugDefault`, INFERRED, verified by this check) and record the mechanism in `docs/reference/bazel-targets-and-configs.md`.
- Files: `tools/bzl/parity/compare-binary.sh`, `BUILD.bazel`
- Verify: `go version -m bazel-bin/stapler-squad_/stapler-squad | grep -c 'DefaultGODEBUG.*netdns=go'` prints `1` and `bash tools/bzl/parity/compare-binary.sh; echo $?` prints `0`.

#### Story 3.5: Auxiliary binaries
**As an** implementer, **I want** every binary the Makefile builds to exist as a Bazel target, **so that** wrappers have a target per make target.
**Acceptance Criteria**:
- *Given* `Makefile` binary targets (`ssq-hooks`, `claude-mux`, `tools/scanner/backend/cmd/scanner`, other `cmd/*`), *When* `bazel build //cmd/... //tools/scanner/...` runs, *Then* exit 0 and each Makefile binary has a target in `docs/reference/bazel-targets-and-configs.md`.
**Files**: `cmd/**/BUILD.bazel`, `docs/reference/bazel-targets-and-configs.md`

##### Task 3.5a: Verify and record aux binary targets (cost S)
- Gazelle already generated them; list the make-target-to-label map; fix any `x_defs` parity.
- Files: `docs/reference/bazel-targets-and-configs.md`
- Verify: `bazel build //cmd/... ; echo $?` prints `0`.

#### Story 3.6: otel variant documented as an exception
**Superseded 2026-11-03 (owner decision, PROVISIONAL, branch `sdd/bazel-otel-default`):** the otel variant is no longer an exception or opt-in. The ACs and tasks below are historical; the replacement is the S15b `otel_weave_hermetic` action behind `//:stapler-squad` (`docs/reference/bazel-targets-and-configs.md`, ADR-005 changelog).

**As an** agent, **I want** the otel build to keep working unchanged, **so that** the exception is a decision, not an accident.
**Acceptance Criteria**:
- *Given* `make sync` has run, *When* `make build-otel-auto` runs, *Then* it still produces `stapler-squad-otel` and `scripts/otel-auto-isolation-guard.sh` exits 0.
**Files**: `docs/reference/bazel-targets-and-configs.md`, `Makefile`

##### Task 3.6a: Guard and doc (cost XS)
- Make `build-otel-auto` depend on `sync`; document in ADR-005 table reference page.
- Files: `Makefile`, `docs/reference/bazel-targets-and-configs.md`
- Verify: `make -n build-otel-auto | grep -c sync` is at least 1.

#### Story 3.7: Release artifacts and goreleaser
**As the** owner, **I want** the release path decided and checked, **so that** release binaries do not silently diverge from dev (they already differ: `CGO_ENABLED=0`).
**Acceptance Criteria**:
- Decision recorded.
  - *Given* `.goreleaser.yaml`, *When* Task 3.7a ends, *Then* ADR-005 states whether goreleaser consumes Bazel binaries (`builder: prebuilt`) or keeps `go build` as a PermanentException.
- Guard still passes.
  - *Given* the chosen path, *When* `goreleaser-check.yml` runs, *Then* the CGO=0 binary check passes.
**Files**: `.goreleaser.yaml`, `.github/workflows/goreleaser-check.yml`, `decisions/ADR-005-permanent-exceptions-outside-bazel.md`

##### Task 3.7a: Prebuilt-builder spike (cost S)
- Check goreleaser docs/version in use for `builder: prebuilt` availability (OSS vs Pro) and cross-compile of pure Go targets via Bazel `go_cross_binary`; record.
- Files: `decisions/ADR-005-permanent-exceptions-outside-bazel.md`
- Verify: `grep -cE '^Decision \(goreleaser\): [a-z-]+' decisions/ADR-005-permanent-exceptions-outside-bazel.md` prints `1` and `grep -c 'TBD' decisions/ADR-005-permanent-exceptions-outside-bazel.md` prints `0`.

##### Task 3.7b: Implement the chosen path (cost M)
- Either wire prebuilt Bazel binaries into goreleaser or leave `go build` and add the exception note; update `goreleaser-check.yml` accordingly.
- Files: `.goreleaser.yaml`, `.github/workflows/goreleaser-check.yml`
- Verify: `goreleaser check` exits 0; `actionlint` exits 0.

##### Task 3.7c: Homebrew `Formula/` still renders and resolves release artifacts (cost S)
- Consistency C6 and validation G1: `Formula/stapler-squad.rb` is written by `.goreleaser.yaml` (`brews.directory: Formula`) and is in requirements scope but had no check. Decision: the formula stays goreleaser-owned (a PermanentException row in ADR-005 while goreleaser keeps `go build`, or unchanged behaviour when it consumes prebuilt Bazel binaries); add a snapshot dry run to `goreleaser-check.yml` that renders the formula and asserts its asset names, URLs and sha256 values equal the release assets of the same snapshot.
- Files: `.github/workflows/goreleaser-check.yml`, `tools/bzl/check-formula.sh`, `decisions/ADR-005-permanent-exceptions-outside-bazel.md`
- Verify: `ruby -c Formula/stapler-squad.rb` exits 0; `goreleaser release --snapshot --skip=publish --clean` then `bash tools/bzl/check-formula.sh dist/` exits 0 (asset names and sha256 match).

---

## Epic 4: Test sandbox remediation

**Goal**: The Go test pass set under `bazel test //...` equals the `go test ./...` pass set (P1) and the tag matrix (P2), with fixes at the source. 49 of 154 test packages are sandbox-hostile (UNVERIFIED count from `features.md`; Task 0.9a re-derives it). Fixes are organised by `SandboxClass` first (Story 4.10: one sweep per class across all packages, using the S8/S9 census) so each shared helper is designed once; the per-package stories 4.3-4.7 are the residual tail that the sweeps did not clear. Order: 4.1, 4.2, then the tooling tasks 4.8a and 4.9a (the verifies of 4.7a/4.7b and 4.10 call `test-set-diff.sh`, `check-exclusions.py` and `census-check.py`), then 4.10f and 4.10, then 4.3-4.7, then 4.8b and 4.9b/c. Policy: fix, do not defer (`fix-flaky-tests-dont-defer`); `ExclusionsFile` entries need an owner and an issue.

#### Story 4.1: Shared test environment helpers and defaults
**As an** implementer, **I want** one helper for repo-root and env defaults, **so that** fixes are made once.
**Acceptance Criteria**:
- Root resolves in both worlds, from a dependency-free package.
  - *Given* `reporoot.Find()` in `internal/reporoot`, *When* called from `go test ./internal/reporoot` and from `bazel test //internal/reporoot:reporoot_test`, *Then* both return a directory containing `go.mod`, and `go list -deps ./internal/reporoot ./internal/testenv | grep stapler-squad | grep -v -e internal/reporoot -e internal/testenv` prints nothing (stdlib-only leaf packages; VERIFIED the reason: `go list -deps ./testutil` includes `config`, `session`, `session/tmux`, `executor`, so `testutil` would be an import cycle for `package config` and `package session` tests).
- Env defaults.
  - *Given* `.bazelrc` `test` section, *When* a test reads `HOME` and `TMPDIR`, *Then* `HOME` is under `TEST_TMPDIR` and `TMPDIR` is a short path (`/tmp`) so unix sockets stay under 104 bytes.
**Files**: `internal/reporoot/reporoot.go`, `internal/reporoot/reporoot_test.go`, `internal/testenv/testenv.go`, `.bazelrc`

##### Task 4.1a: reporoot.Find() and testenv leaf packages (cost S)
- Run `go list -deps ./testutil | grep stapler-squad` first and record the result (it lists `config`, `session`, `executor`, so the helper cannot live in `testutil`). Implement `internal/reporoot` (`Find()` using `TEST_SRCDIR`/`TEST_WORKSPACE` when set, else walking up to `go.mod`) and `internal/testenv` (env-default helpers such as an isolated `HOME`, short `TMPDIR`, unique tmux socket name), both importing only the standard library, so no import cycle is possible from any `_test.go` file including internal-package tests of `config` and `session`, and plain `go build` stays independent of rules_go.
- Files: `internal/reporoot/reporoot.go`, `internal/reporoot/reporoot_test.go`, `internal/testenv/testenv.go`, `internal/testenv/testenv_test.go`
- Verify: `go test ./internal/reporoot ./internal/testenv -count=1` prints `ok`; `bazel test //internal/reporoot:reporoot_test` exits 0; `go list -deps ./internal/reporoot ./internal/testenv | grep stapler-squad | { ! grep -qv -e internal/reporoot -e internal/testenv; }; echo $?` prints `0` (the negated grep's exit status; `grep -vc` exits 1 on zero matches, which reads as failure in a script).

##### Task 4.1b: Test env defaults in .bazelrc (cost S)
- `test --test_env=HOME=<fixture dir containing a .gitconfig>` (a private HOME without one fails every git-committing test with "Author identity unknown", 21 cases, S8). Do NOT add a global `--test_env=TMPDIR` (a no-op: Bazel 9.3.1 already gives tests a private short `/tmp` and ignores the override, S9) or a global `--test_env=STAPLER_SQUAD_TMUX_SOCKET` (breaks 7 tests in `session/mux` and `session/tmux` that assert their own socket names, S8); tmux and gh are supplied through `data`/PATH because Bazel's test PATH is `/usr/bin:/bin` (62 `exec-host-binary` failures, S8). Superseded wording follows: the short-private-`/tmp` mechanism of Task 0.9b (`--incompatible_sandbox_hermetic_tmp` is absent on 9.3.1 and rejected by `check-bazelrc.sh`) strategy (per-test unique socket set in a small wrapper if env interpolation is unavailable), `--sandbox_default_allow_network=false` with loopback allowed.
- Live-state guard (pre-mortem #13): pin `--test_env=HOME`, `STAPLER_SQUAD_INSTANCE=bazel-test` and `STAPLER_SQUAD_TMUX_SOCKET` in the committed `.bazelrc` `test` section (not in agent-writable `user.bazelrc`), and make `internal/testenv` refuse the real `~/.stapler-squad` config dir whenever `TEST_TMPDIR` is set (test-only guard; production behaviour unchanged). `bazel-agent-validate` warns on `--spawn_strategy=local` in `user.bazelrc`. **Epic 0 correction (S8):** the socket guard cannot be a global `test --test_env=STAPLER_SQUAD_TMUX_SOCKET` (it breaks tests asserting their own socket names); pin it per test process in the test wrapper or `internal/testenv` instead.
- Files: `.bazelrc`, `tools/bzl/test_wrapper.sh`
- Verify: `bazel test //tools/bzl/spike:env_test` (a 10-line Go test printing the env) exits 0.

#### Story 4.2: go_test defaults macro, timeouts and sharding
**As an** implementer, **I want** timeouts and shards set in one table, **so that** `server/services` (20m under make) does not hit Bazel's default timeout.
**Acceptance Criteria**:
- *Given* `# gazelle:map_kind go_test go_test //tools/bzl:go_test.bzl` and a size table, *When* `bazel query 'attr(size, enormous, //server/services:all)'` runs, *Then* it lists the services test target, and `bazel test //server/services:services_test --test_timeout=...` does not time out on main.
- *Given* `shard_count = 4` on a slow package, *When* tests run sharded, *Then* the union of passed test names equals the unsharded run.
**Files**: `tools/bzl/go_test.bzl`, `tools/bzl/BUILD.bazel`, `BUILD.bazel`

##### Task 4.2a: Productionise go_test.bzl (cost M)
- From Task 0.11c: table `{package: (size, shard_count)}` in the macro; `bazel_skylib` analysis test; keep under the 30-line macro bound.
- Test-cache false-green control (pre-mortem #3): the macro also holds a per-`SandboxClass` cache policy. `cwd` and `runtime-caller` tests are cacheable once converted to `reporoot.Find()`/`data`; tests of class `exec-host-binary`, `network`, `tmpdir-socket` and `home` get `tags = ["external"]` (always rerun, never cached) until they declare their inputs, tracked as an ExclusionsFile-style ratchet that may only shrink. Bazel caches a passing result keyed on declared inputs only, and about 49 of 154 test packages are environment-coupled, so without this a once-green test keeps "passing" in CI. `flaky = True` and `--flaky_test_attempts` greater than 1 are banned (Task 0.2g).
- Files: `tools/bzl/go_test.bzl`, `tools/bzl/BUILD.bazel`
- Verify: `bazel test //tools/bzl:go_test_bzl_test` exits 0.

##### Task 4.2b: Verify rules_go sharding semantics (cost S)
- Confirm `TEST_SHARD_INDEX` handling by running `//server/services` with `--test_sharding_strategy=explicit` and comparing name union to unsharded; record finding (research marked INFERRED).
- Also record how `--test_filter=TestFoo` reaches the Go test binary (passed as `-test.run`? anchored or not?), because Task 8.2e maps `RUN=` to it and `go test -run` semantics differ.
- Files: `spike-results.md` (append), `tools/bzl/go_test.bzl`, `tools/bzl/parity/shard-union.sh`
- Verify: `bash tools/bzl/parity/shard-union.sh //server/services:services_test; echo $?` prints `0` (the script runs the target sharded with `--test_sharding_strategy=explicit` and unsharded, writes the passing names from both `test.xml` sets to `sharded.names` and `unsharded.names`, and exits 0 only when `diff <(sort sharded.names) <(sort unsharded.names)` is empty, printing `SHARD UNION EQUAL n=<count>`); `bash tools/bzl/parity/shard-union.sh --filter-semantics | grep -cE '^filter: (passthrough|translated) (anchored|unanchored)$'` prints `1`.

##### Task 4.2c: Guard for no-sandbox and local test tags (cost S)
- `tools/bzl/check-test-tags.sh` fails CI when any `go_test`, `go_test_defaults` macro call or `.bazelrc` test config carries `no-sandbox`, `local`, `requires-network`, `external`, `no-cache` or `no-remote-cache` without, on the same or the preceding line, a comment of the form `# tag-reason: <text> (#<issue>)` (this enforces the "never silently tag" rule of the troubleshooting decision tree). A scheduled job (`bazel-upgrade-canary.yml` or the nightly of Task 4.9c) additionally checks with `gh issue view` that each referenced issue exists and is open or has a closing PR; the count of tagged targets joins `bazel-trend.sh`.
- Files: `tools/bzl/check-test-tags.sh`, `tools/bzl/check-test-tags_test.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/check-test-tags_test.sh` prints `PASS` for a tagged target with a valid comment, `FAIL` (exit 1) for one without, and for one whose comment lacks `(#<issue>)`; `shellcheck` exits 0.

#### Story 4.3: Remediate `server/services` (residual after the Story 4.10 sweeps)
**As an** implementer, **I want** `bazel test //server/services/...` green, **so that** the largest package is covered.
**Acceptance Criteria**:
- *Given* the package after remediation, *When* `bazel test --config=ci //server/services/...` runs, *Then* exit 0 and every test name that passes under `go test ./server/services` is in the Bazel passing set (P1 slice), with `ExclusionsFile` entries only for tests that need the live network or host-only resources, each with an issue.
**Files**: `server/services/**/*_test.go`, `server/services/BUILD.bazel`, `tools/bzl/test-exclusions.yaml`

##### Task 4.3a: Census run and classification (cost S)
- Run with `--test_output=errors --sandbox_debug`; `python3 tools/bzl/spike/census.py --package ./server/services --table` writes the counts by `SandboxClass` into the PR description.
- Files: `tools/bzl/test-exclusions.yaml` (working copy)
- Verify: `python3 tools/bzl/spike/census-check.py --package ./server/services; echo $?` prints `0` (exit 0 only when per-class counts sum to the failing-test count).

##### Task 4.3b: Fix residual cwd and relative-path tests via reporoot.Find()/data (cost M)
- Replace `os.Getwd()`+`../` and `runtime.Caller` source discovery with `reporoot.Find()`/`data` deps; add `data = glob(["testdata/**"])` via Gazelle where missing.
- Files: affected `server/services/*_test.go`, `server/services/BUILD.bazel`
- Verify: `bazel test //server/services/... --test_filter='<fixed names>'` exits 0 and `go test ./server/services -run '<fixed names>' -count=1` prints `ok`.

##### Task 4.3c: Fix residual HOME, tmux, git, and fixed-port tests (cost M)
- `t.Setenv("HOME", t.TempDir())` through `internal/testenv` (a leaf package, so `package config`/`package session` tests may import it); tmux via `STAPLER_SQUAD_TMUX_SOCKET` and `TMUX_BIN` from `//tools/bzl/tmux:tmux_bin` (`data`); git via `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM`; port `0` listeners instead of fixed ports.
- Files: affected `server/services/*_test.go`
- Verify: `bazel test //server/services/... ; echo $?` prints `0`.

##### Task 4.3d: Timeout and shard settings (cost S)
- Add the package to the size table with shard count chosen from measured runtime.
- Files: `tools/bzl/go_test.bzl`
- Verify: `bazel test //server/services:services_test` reports `PASSED` within the configured timeout.

#### Story 4.4: Remediate `session` (residual after the Story 4.10 sweeps)
**As an** implementer, **I want** `bazel test //session/...` green, **so that** the second-largest package is covered.
**Acceptance Criteria**:
- *Given* the package after remediation, *When* `bazel test --config=ci //session` runs, *Then* exit 0 and the P1 slice for `session` has an empty diff against `baseline-test-set.json` (excluding listed flakes).
**Files**: `session/*_test.go`, `session/BUILD.bazel`

##### Task 4.4a: Census and classification (cost S)
- As 4.3a for `session`.
- Files: `tools/bzl/test-exclusions.yaml`
- Verify: `python3 tools/bzl/spike/census-check.py --package ./session; echo $?` prints `0`.

##### Task 4.4b: Fix environment-coupled tests (cost M)
- Same techniques as 4.3b/4.3c; respect `.claude/rules/instance-lock-free-reads.md` when touching `session/instance*.go` tests.
- Files: affected `session/*_test.go`
- Verify: `bazel test //session/... ; echo $?` prints `0`.

#### Story 4.5: Remediate `session/tmux` (residual after the Story 4.10 sweeps)
**As an** implementer, **I want** the real-tmux tests to run with a pinned tmux and private sockets, **so that** parallel Bazel shards cannot collide.
**Acceptance Criteria**:
- *Given* `TMUX_BIN` pointing at `//tools/bzl/tmux:tmux_bin` and a unique `STAPLER_SQUAD_TMUX_SOCKET` per test process, *When* `bazel test //session/tmux/... --config=integration` runs, *Then* exit 0, no socket path exceeds 104 bytes (assert in a test), and `exclusive` is applied only to a test that still fails `--runs_per_test=3 --jobs=8` after private sockets are in place (`exclusive` serialises the whole invocation and would hurt P10; the real need behind make's `-p 1` is isolation, which per-process sockets provide). The wall-time effect of any remaining `exclusive` tag is measured in Task 7.5d.
**Files**: `session/tmux/*_test.go`, `session/tmux/BUILD.bazel`

##### Task 4.5a: Census and fixes (cost M)
- Makefile's `-p 1` pair (`./session ./session/tmux`) is first made parallel-safe with unique `STAPLER_SQUAD_TMUX_SOCKET` per test process; only tests that still fail the stress run get `tags = ["exclusive"]` in the size table, each with a comment naming the failure.
- Files: affected `session/tmux/*_test.go`, `tools/bzl/go_test.bzl`
- Verify: `bazel test --config=integration //session/tmux/... ; echo $?` prints `0`.

##### Task 4.5b: Socket-length assertion test (cost XS)
- A test computing the longest socket path used by the harness under `TEST_TMPDIR` and failing above 100 bytes.
- Files: `session/tmux/socket_path_len_test.go`
- Verify: `bazel test //session/tmux:socket_path_len_test` exits 0.

#### Story 4.6: Remediate `config`, `session/sshremote`, `server` (residual after the Story 4.10 sweeps)
**As an** implementer, **I want** the remaining three of the six worst packages fixed, **so that** the top-six list is closed.
**Acceptance Criteria**:
- *Given* each of `config`, `session/sshremote`, `server`, *When* `bazel test --config=ci <pkg>/...` runs, *Then* exit 0 with P1 slice diffs empty.
**Files**: `config/*_test.go`, `session/sshremote/*_test.go`, `server/*_test.go`, their `BUILD.bazel`

##### Task 4.6a: config (cost M)
- `~/.stapler-squad` and `HOME`-based tests to `t.TempDir()` roots through the config-dir resolver; no new env vars.
- Files: `config/*_test.go`
- Verify: `bazel test //config/... ; echo $?` prints `0`.

##### Task 4.6b: session/sshremote (cost M)
- Fake ssh servers on port 0, short socket dirs, no reliance on host `~/.ssh`.
- Files: `session/sshremote/*_test.go`
- Verify: `bazel test //session/sshremote/... ; echo $?` prints `0`.

##### Task 4.6c: server (cost M)
- Same techniques; coordinate with `server/dependencies.go` constraints (`noarchivedrevival` rule untouched).
- Files: `server/*_test.go`
- Verify: `bazel test //server/... --test_tag_filters=-integration ; echo $?` prints `0`.

#### Story 4.7: Sweep the remaining sandbox-hostile packages
**As an** implementer, **I want** whatever the Story 4.10 class sweeps left in the other roughly 43 hostile packages cleared, **so that** P1 reaches an empty diff.
**Acceptance Criteria**:
- *Given* all test packages, *When* `bash tools/bzl/parity/test-set-diff.sh default` runs, *Then* the only delta entries are in the `ExclusionsFile`, each with owner and issue, and the count is at most 10 packages (proposal; owner may tune).
**Files**: many `*_test.go`, `tools/bzl/test-exclusions.yaml`

##### Task 4.7a: Residual batch 1 (cwd, runtime.Caller, relative paths not covered by 4.10a) (cost M)
- First step: `python3 tools/bzl/spike/census.py --all && python3 tools/bzl/spike/census-batches.py --residual` to emit the `4.7a` and `4.7b` groups from the post-sweep census. Executed as batches `4.7a.1 ... 4.7a.N` from `tools/bzl/spike/batches.json` (group `4.7a`, generated by Task 4.10f from the census after the 4.10 sweeps merged); each batch is one worker and one PR touching at most 5 files under the split rule of Task 4.10f. The task is done when every `4.7a` batch is `merged`.
- Files: affected `*_test.go` across `session/git`, `cmd/ssq-hooks`, `server/deliverygate` and the rest by class, as listed per batch in `batches.json`
- Verify (absolute, boolean): `python3 tools/bzl/spike/census-check.py --class cwd,runtime-caller --residual; bazel test $(jq -r '.batches[]|select(.group=="4.7a")|.targets[]' tools/bzl/spike/batches.json) ; echo $?` prints `0` for the census count and `0` for the test exit; `python3 tools/bzl/spike/census-batches.py --check-merged --group 4.7a` exits `0`.

##### Task 4.7b: Residual batch 2 (HOME, host binaries, fixed ports not covered by 4.10b-4.10e) (cost M)
- Same batch mechanism, group `4.7b`. Batches may not start until all `4.7a` batches are merged.
- Files: affected `*_test.go`, as listed per batch in `batches.json`
- Verify (absolute, boolean): all of these hold: `bazel test //... --test_tag_filters=-integration,-manual ; echo $?` prints `0`; `bash tools/bzl/parity/test-set-diff.sh default | grep -c '^missing_in_bazel: 0$'` prints `1`; `python3 tools/bzl/check-exclusions.py; echo $?` prints `0` (at most 10 packages, each with owner and issue); `python3 tools/bzl/spike/census-batches.py --check-merged --group 4.7b` exits `0`.

#### Story 4.10: Class sweeps by SandboxClass (runs after 4.2, 4.8a and 4.9a, before 4.3-4.7)
**As an** implementer, **I want** one sweep per failure class across every package, driven by the S8/S9 census and split into batches of at most 5 files, **so that** each shared fix is designed once, no worker holds more than a reviewable diff, and the six worst packages do not absorb the helper design work.
**Acceptance Criteria**:
- *Given* the census table from Task 0.9a refreshed over all packages by Task 4.10f, *When* each class sweep (Tasks 4.10a-4.10e) merges, *Then* re-running the census shows the count for that class at 0 except for entries in the `ExclusionsFile` (owner and issue each), and the per-class counts are posted in the PR (sum equals the failing-test count, by script).
- *Given* `tools/bzl/spike/batches.json`, *When* `jq -e '[.batches[]|(.files|length)]|max<=5' batches.json` runs, *Then* it prints `true` (no batch touches more than 5 files).
**Files**: `*_test.go` across packages, `internal/testenv/testenv.go`, `tools/bzl/test-exclusions.yaml`, `tools/bzl/spike/batches.json`

##### Task 4.10f: Batch manifest generator and split rule (runs first in Story 4.10) (cost S)
- Write `tools/bzl/spike/census-batches.py` (and a unit test). Input: the refreshed census (`census.json` from Task 0.9a's script, re-run over all packages: per failing test its package, file and `SandboxClass`). Output: `tools/bzl/spike/batches.json` (`{id, group, class, files[], targets[], merged}`) and one `batches/<id>.txt` per batch. `merged` is never edited by hand: `census-batches.py --refresh` sets it true only when (a) `origin/main` holds a commit whose message carries the trailer `Batch: <id>` and (b) the census slice for the batch's files (`census-check.py --files batches/<id>.txt`) prints 0 and `bazel test <its targets>` exited 0 at that commit; `--check-merged --group G` exits 0 only if every batch of group G is true under this rule, so the flag is tied to the census count, not to a claim. **Split rule**: (1) groups are `4.10a` (cwd, runtime-caller), `4.10b` (home), `4.10c` (tmpdir-socket, exec-host-binary), `4.10d` (git env), `4.10e` (network, fixed ports), then `4.7a` and `4.7b` for the residual classes, emitted by a second run `census-batches.py --residual` after the 4.10 sweeps merged (the first run emits only 4.10a-e; the residual run is the first step of Task 4.7a); (2) a file with failures in two classes goes to the earliest group and is dropped from later ones; (3) within a group sort files by package path then file name and fill greedily; (4) a batch holds at most 5 distinct files, counting `*_test.go`, `BUILD.bazel` and helper files together (a BUILD file counts once per batch), and a single `_test.go` file is never split across batches; (5) a change to a shared helper (`internal/testenv`, `internal/reporoot`) is its own batch numbered `.0`, merged before the batches that use it; (6) a batch is done when its census slice prints 0 and `bazel test <its targets>` exits 0. Planning estimate for effort only (UNVERIFIED until this task runs): 4.10a 6 batches, 4.10b 4, 4.10c 5, 4.10d 2, 4.10e 2, 4.7a 4, 4.7b 4; the cost class on Tasks 4.10a-e and 4.7a/b is the aggregate budget of their batches, not per batch.
- Files: `tools/bzl/spike/census-batches.py`, `tools/bzl/spike/census-batches_test.py`, `tools/bzl/spike/batches.json`
- Verify: `python3 -m unittest tools/bzl/spike/census-batches_test.py` exits 0 (includes a fixture where one package has 12 failing files and must split into 3 batches, a two-class file assigned once, and a hand-set `merged: true` with a non-zero census slice that `--refresh` reverts to false); `jq -e '[.batches[]|(.files|length)]|max<=5' tools/bzl/spike/batches.json` prints `true`; `jq '[.batches[].files[]]|length-(unique|length)' tools/bzl/spike/batches.json` prints `0` (no file in two batches).

##### Task 4.10a: `cwd` and `runtime-caller` sweep (cost M)
- Executed as batches of group `4.10a`: replace `os.Getwd()`+`../` and `runtime.Caller` source discovery with `reporoot.Find()` or `data` deps; add `data = glob(["testdata/**"])` where Gazelle missed it.
- Files: affected `*_test.go`, affected `BUILD.bazel`, at most 5 per batch
- Verify: `python3 tools/bzl/spike/census-check.py --class cwd,runtime-caller` prints `0` remaining (excluding `ExclusionsFile` entries) and `python3 tools/bzl/spike/census-batches.py --check-merged --group 4.10a` exits `0`.

##### Task 4.10b: `home` sweep (cost M)
- Executed as batches of group `4.10b`: route `$HOME`/`~/.stapler-squad` tests through `internal/testenv` and the config-dir resolver (`t.TempDir()` roots, no new env vars).
- Files: affected `*_test.go`, at most 5 per batch
- Verify: `python3 tools/bzl/spike/census-check.py --class home` prints `0` and the group's batches are all `merged` (`python3 tools/bzl/spike/census-batches.py --check-merged --group 4.10b` exits `0`).

##### Task 4.10c: `tmpdir-socket` and `exec-host-binary` sweep (cost M)
- Executed as batches of group `4.10c`: short socket directories, unique `STAPLER_SQUAD_TMUX_SOCKET`, `TMUX_BIN` and other binaries supplied as `data`; no reliance on host `PATH` tools.
- Files: affected `*_test.go`, `internal/testenv/testenv.go` (its own batch `.0`), at most 5 per batch
- Verify: `python3 tools/bzl/spike/census-check.py --class tmpdir-socket,exec-host-binary` prints `0` and `python3 tools/bzl/spike/census-batches.py --check-merged --group <this task's group>` exits `0`.

##### Task 4.10d: Git environment sweep (cost S)
- Executed as batches of group `4.10d`: `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM` and a temp-repo helper in `internal/testenv`.
- Files: affected `*_test.go`, at most 5 per batch
- Verify: `bazel test //session/git/... //session/vc/... ; echo $?` prints `0` and `python3 tools/bzl/spike/census-batches.py --check-merged --group <this task's group>` exits `0`.

##### Task 4.10e: `network` and fixed-port sweep (cost S)
- Executed as batches of group `4.10e`: port-0 listeners; loopback-only tests marked; true external-network tests go to `ExclusionsFile` with an issue.
- Files: affected `*_test.go`, at most 5 per batch
- Verify: `python3 tools/bzl/spike/census-check.py --class network` prints `0` remaining beyond `ExclusionsFile` entries and `python3 tools/bzl/spike/census-batches.py --check-merged --group <this task's group>` exits `0`.

#### Story 4.8: Test-set diff tooling and tag-matrix parity (P1, P2)
**As the** owner, **I want** a script that proves test-set parity, **so that** cutover rests on evidence.
**Acceptance Criteria**:
- *Given* `go test` JSON and Bazel `test.xml` for the same commit, *When* `bash tools/bzl/parity/test-set-diff.sh <config>` runs for `default`, `race`, `integration`, `sinkguard`, `gitoracle`, *Then* it prints `missing_in_bazel: 0` (excluding `ExclusionsFile` names) and exits 0.
**Files**: `tools/bzl/parity/test-set-diff.sh`, `tools/bzl/testsummary/main.go`

##### Task 4.8a: Test-set diff script and summariser (cost M) (runs before Story 4.10)
- Go tool `testsummary` parses `bazel-testlogs/**/test.xml`; the script aligns names with `gotestsum --jsonfile` output from a live `go test` run on the same commit (P1 compares two live runs; `baseline-test-set.json` is only the Epic 0 reference, and the flake list is regenerated at the gate).
- Files: `tools/bzl/parity/test-set-diff.sh`, `tools/bzl/testsummary/main.go`, `tools/bzl/testsummary/main_test.go`
- Verify: `go test ./tools/bzl/testsummary -count=1` prints `ok`.

##### Task 4.8b: Tag matrix runs (cost S)
- Run the script for each `TagConfig`; map `-p 1` and 20m/10m timeouts from `Makefile:793-830` to size table entries; `sinkguard` per-directory `-run` filters become `args` in a macro.
- Files: `tools/bzl/go_test.bzl`, `.bazelrc`
- Verify: `for c in default race integration sinkguard gitoracle; do bash tools/bzl/parity/test-set-diff.sh $c || echo FAIL $c; done` prints no `FAIL`.

#### Story 4.9: Fix surfaced flakes and guard the ExclusionsFile
**As the** owner, **I want** newly visible flakes fixed and the exclusion list only able to shrink, **so that** the policy holds.
**Acceptance Criteria**:
- *Given* a PR adding an exclusion without `owner` and `issue`, *When* `python3 tools/bzl/check-exclusions.py` runs in CI, *Then* it exits 1 naming the entry; removing entries is allowed; count may not increase after Story 7.6.
**Files**: `tools/bzl/check-exclusions.py`, `tools/bzl/test-exclusions.yaml`

##### Task 4.9a: check-exclusions.py and ratchet (cost S) (runs before Story 4.10)
- Validate schema; compare entry count against `origin/main`.
- Files: `tools/bzl/check-exclusions.py`, `tools/bzl/check-exclusions_test.py`
- Verify: `python3 -m unittest tools/bzl/check-exclusions_test.py` exits 0.

##### Task 4.9b: Fix BUG-087..090 class flakes that Bazel exposes (cost M)
- Per `fix-flaky-tests-dont-defer` and `quality:prevent-test-flakes`; identify via three repeated runs `--runs_per_test=3`.
- Files: affected tests
- Verify: `bazel test //... --runs_per_test=3 --test_tag_filters=-integration,-manual ; echo $?` prints `0`.

##### Task 4.9c: Nightly uncached flake job (cost S)
- Pre-mortem #3. `.github/workflows/bazel-nightly-flake.yml` runs on `main` on a schedule: `bazel test //... --nocache_test_results --runs_per_test=3 --test_tag_filters=-manual`. Results append to `soak-ledger.yaml` with the attribution classes of Story 7.6 so a flake that PR CI never reported becomes a ledger row. After the Story 8.6 flip, the weekly canary also asserts the make-path pass set equals the Bazel pass set (Task 8.5b).
- Files: `.github/workflows/bazel-nightly-flake.yml`
- Verify: `actionlint .github/workflows/bazel-nightly-flake.yml` exits 0; a dispatched run appends rows to `soak-ledger.yaml`.

---

## Epic 5: macOS (cgo darwin files, Info.plist, codesign outside Bazel)

**Goal**: Native macOS arm64 and amd64 builds under Bazel, equal to the make-built binary, with signing kept outside Bazel (ADR-005, ADR-006). Needs macOS runners and an owner device check.

#### Story 5.1: Darwin build under Bazel
**As an** implementer, **I want** a darwin `--config=macos` build with the Info.plist embedded, **so that** the three cgo files and `-sectcreate` work.
**Acceptance Criteria**:
- Builds and embeds.
  - *Given* a macOS arm64 runner with Xcode CLT, *When* `bazel build --config=macos --config=release //:stapler-squad` runs, *Then* exit 0 and `otool -s __TEXT __info_plist bazel-bin/stapler-squad_/stapler-squad | grep -c CFBundle` is at least 1.
- Both architectures.
  - *Given* the same, *When* built with `--cpu=darwin_x86_64` (or the runner arch matrix), *Then* `file` reports x86_64.
**Files**: `.bazelrc`, `BUILD.bazel`, `.github/workflows/bazel.yml`, `macos/BUILD.bazel`

##### Task 5.1a: macOS job in bazel.yml (cost S)
- The macOS job also runs `bash tools/bzl/wrapper_test.sh` (mkdir-lock and `CI` bypass cases of Task 0.2f; stock macOS has no `flock`, `ionice` or `systemd-run`) so the wrapper is proven on bash 3.2 before any macOS agent depends on it.
- Non-blocking matrix entry on a pinned `macos-14` or `macos-15` image, never `macos-latest`, with Xcode selected by `xcode-select` (pre-mortem #9; free for public repos, INFERRED, check billing) running build and the `darwin`-tagged tests.
- Files: `.github/workflows/bazel.yml`
- Verify: `actionlint` exits 0; run shows the macOS job.

##### Task 5.1b: --config=macos and Info.plist linkopt (cost M)
- `build:macos` sets `--macos_minimum_os`, `-Wl,-sectcreate,__TEXT,__info_plist,$(location //macos:Info.plist)` through `cgo_linkopts` `select` on darwin; add `macos/BUILD.bazel` exporting the plist.
- Files: `.bazelrc`, `BUILD.bazel`, `macos/BUILD.bazel`
- Verify: on macOS, the `otool` command above prints at least `1`.

##### Task 5.1c: Info.plist assertion as a test (cost S)
- `sh_test` (tag `requires-darwin`, `target_compatible_with` macOS) running `otool` on the built binary.
- Files: `macos/BUILD.bazel`, `macos/check_infoplist.sh`
- Verify: on macOS `bazel test //macos:infoplist_test` exits 0; on Linux the target is skipped (`bazel test //macos:infoplist_test` reports `SKIPPED`/incompatible).

##### Task 5.1d: amd64 build (cost M)
- Cross-arch via `clang -arch x86_64` on the arm64 runner, or an Intel runner if cgo cross is rejected; record choice.
- Files: `.bazelrc`, `.github/workflows/bazel.yml`
- Verify: `file -b "$(bazel cquery --config=darwin_amd64 --output=files //:stapler-squad 2>/dev/null)" | grep -c 'x86_64'` prints `1` (the task adds the `darwin_amd64` config to `.bazelrc` and records the choice).

#### Story 5.2: Signing outside Bazel and device verification
**As the** owner, **I want** the Bazel-built binary signed by the existing flow with TCC grants persisting, **so that** macOS users see no new dialogs.
**Acceptance Criteria**:
- Signing flow.
  - *Given* a Bazel-built binary copied to a stable path, *When* `make _codesign-binary BIN=<path>` runs, *Then* `codesign --verify --strict` exits 0 and the designated requirement equals the one from the make build.
- Device check.
  - *Given* a prior TCC grant for the make-built binary, *When* the Bazel-built signed binary replaces it on arm64 and amd64, *Then* the owner confirms no new permission prompt (checklist signed in `docs/how-to/macos-codesigning.md`).
**Files**: `Makefile`, `docs/how-to/macos-codesigning.md`, `scripts/install-service.sh`

##### Task 5.2a: _codesign-binary accepts an explicit path (cost S)
- Parameterise the target (`BIN ?=` default current path), keep behaviour identical by default.
- Files: `Makefile`
- Verify: `make -n _codesign-binary BIN=/tmp/x | grep -c /tmp/x` prints at least `1`.

##### Task 5.2b: Device checklist and script (cost S) (owner runs it)
- Add a section to `docs/how-to/macos-codesigning.md` and `scripts/verify-bazel-codesign.sh` printing designated requirement diff and `tccutil` guidance.
- Files: `docs/how-to/macos-codesigning.md`, `scripts/verify-bazel-codesign.sh`
- Verify: `shellcheck scripts/verify-bazel-codesign.sh` exits 0; owner records result in the PR.

---

## Epic 6: Windows cross-compile (no Windows tests)

**Goal**: `windows_amd64` binaries built by Bazel via platform transition; windows-only packages excluded from default `//...` on other platforms (ADR-005).

#### Story 6.1: Cross-compiled Windows binary
**As the** owner, **I want** the `build/windows_amd64` artifact to keep being produced, **so that** the Windows target is not lost.
**Acceptance Criteria**:
- *Given* a Linux host, *When* `bazel build //tools/bzl/windows:stapler-squad_windows_amd64` runs, *Then* exit 0 and `file -L bazel-bin/tools/bzl/windows/stapler-squad_windows_amd64.exe` contains `PE32+`.
- *Given* `target_compatible_with` on windows-only test targets, *When* `bazel test //...` runs on Linux, *Then* those targets are reported incompatible (skipped), not failed.
**Files**: `tools/bzl/windows/BUILD.bazel`, `.bazelrc`, `.github/workflows/bazel.yml`

##### Task 6.1a: go_cross_binary target (cost M)
- Use rules_go `go_cross_binary` with `@rules_go//go/toolchain:windows_amd64`, `--config=windows_cross` (pure Go, no embeds that need host tools: tmux embed not applicable on Windows).
- Files: `tools/bzl/windows/BUILD.bazel`, `.bazelrc`
- Verify: command in AC prints `PE32+`.

##### Task 6.1b: Compatibility constraints for tagged files (cost S)
- Add the `target_compatible_with` mapping via Gazelle-generated `select` (already for GOOS) and a repo check that `bazel build //... ` on Linux has zero windows-only failures.
- Files: `.bazelrc`, `tools/bzl/check-platform-targets.sh`
- Verify: `bazel build --keep_going //... 2>&1 | grep -c '^ERROR'` prints `0`; `bazel cquery --keep_going --output=starlark --starlark:expr='"incompatible" if "IncompatiblePlatformProvider" in providers(target) else "ok"' '//...' | grep -c incompatible` is at least `1` (windows-only targets are reported incompatible by the platform provider, not by counting the word "windows").

##### Task 6.1c: CI cross-compile and goreleaser alignment (cost S)
- Add the target to `bazel.yml`; confirm the goreleaser windows build excludes (`windows/arm`) match.
- Files: `.github/workflows/bazel.yml`, `.goreleaser.yaml`
- Verify: `actionlint` exits 0; run builds the target.

---

## Epic 7: CI cutover (affected targets, freshness gates, remote cache, parity, soak)

**Goal**: Bazel runs CI; the hand-maintained selection logic is superseded by an `AffectedSet` computed from the graph (deleted with the legacy jobs in Epic 10); remote cache is read-only on PRs by server enforcement (ADR-004); P1-P10 pass; jobs flip from non-blocking to blocking one at a time after the soak.

#### Story 7.1: Build metrics and trend
**As the** owner, **I want** the three driver metrics tracked in CI, **so that** claims are comparable to `baselines.md`.
**Acceptance Criteria**:
- *Given* a completed Bazel CI run, *When* `bash tools/ci/bazel-trend.sh <run-id>` runs, *Then* it prints JSON with `time_to_green_s`, `cache_hit_ratio`, `affected_target_count` and appends to the trend artifact used by `ci-duration-trend.yml`.
**Files**: `tools/ci/bazel-trend.sh`, `.github/workflows/ci-duration-trend.yml`

##### Task 7.1a: BEP parser and trend script (cost M)
- Parse `bep.json` for action cache hit/miss counts, wall time; handle more than one BEP file per CI job, because `--experimental_remote_cache_eviction_retries` (default 5, `[RUN]`) re-runs the build with a new invocation id when a cached artifact was evicted, so "one invocation = one BEP file" is false (D9) and the parser sums the attempts of one job and reports the retry count; extend `ci-duration-trend.yml` with a Bazel section. Also emit `bazel_cache_auth_failures_total` (count of E11 matches: `HTTP 401/403`, `UNAUTHENTICATED`, `PERMISSION_DENIED` in the BEP stderr events and `command.log`) and a `::warning` annotation when it is non-zero, so an expired cache credential is a visible counter and not only a slower build.
- Files: `tools/ci/bazel-trend.sh`, `tools/ci/bazel-trend_test.sh`, `.github/workflows/ci-duration-trend.yml`
- Verify: `bash tools/ci/bazel-trend_test.sh` exits 0 using a recorded BEP fixture, and a second fixture containing a `403` cache response makes it print `bazel_cache_auth_failures_total 1` while the clean fixture prints `0`.

#### Story 7.2: Freshness gates
**As an** agent, **I want** CI and a hook to tell me exactly how to fix BUILD drift, **so that** parallel agent PRs do not break each other.
**Acceptance Criteria**:
- Gazelle drift fails with the fix command.
  - *Given* a PR adding `import "github.com/google/uuid"` to a file without updating BUILD, *When* the gate runs, *Then* it fails with `Run: make gazelle` and the diff.
- `use_repo` drift fails.
  - *Given* a `go.mod` change without `bazel mod tidy`, *When* the gate runs, *Then* it fails with `Run: bazel mod tidy`.
- Hook parity.
  - *Given* the same change, *When* the pre-commit hook runs, *Then* it blocks with the same message.
**Files**: `.github/workflows/bazel.yml`, `scripts/hooks/pre-commit-gazelle.sh`, `Makefile`

##### Task 7.2a: CI gates (cost S)
- Steps: Gazelle diff, `bazel mod tidy` diff, `make gen-freshness`; failure messages carry the remedy. The step logic lives in `tools/ci/bazel-gates.sh` (so it is testable offline); `bazel.yml` only calls it.
- Files: `.github/workflows/bazel.yml`, `tools/ci/bazel-gates.sh`, `tools/ci/bazel-gates_test.sh`
- Verify: `bash tools/ci/bazel-gates_test.sh` exits 0 (seeded: a new import without a BUILD change prints `Run: make gazelle`; a `go.mod` change without tidy prints `Run: bazel mod tidy`; a stale `gen/` prints `run: make sync`); `actionlint` exits 0.

##### Task 7.2b: make gazelle and pre-commit hook (cost S)
- The `gazelle` make target exists from Task 1.5a (so the remedy named by the Epic 1 hints resolves); here add its pre-commit hook through the mechanism `make setup-git` installs (Task 0.2h, `core.hooksPath` = `scripts/hooks`), running only when `*.go` imports or `go.mod` change.
- Files: `Makefile`, `scripts/hooks/pre-commit-gazelle.sh`
- Verify: `make gazelle` exits 0; hook script `bash scripts/hooks/pre-commit-gazelle.sh` exits 0 on a clean tree.

#### Story 7.3: Affected-target selection replacing the hand-maintained selection logic
**As the** owner, **I want** the graph to decide what runs, **so that** the two-sources-of-truth risk disappears.
**Acceptance Criteria**:
- Pure function with fixtures.
  - *Given* a changed-file list `["session/health.go"]`, *When* `tools/ci/affected-targets.sh` runs, *Then* it prints `//session:session_test` and its reverse dependencies' tests and no `//web-app:*`.
- Global inputs force everything.
  - *Given* `["MODULE.bazel"]` or `["go.mod"]`, *When* it runs, *Then* it prints `//...`.
- No false negatives, per selection mechanism.
  - *Given* 30 historical merged PRs, *When* the AffectedSet plus the retained non-Bazel path list `tools/bzl/non-bazel-inputs.txt` is compared with what each of the three actual mechanisms selected (the `grep -qE` regex at `build.yml:97`, `tools/ci/detect-feature-changes.sh`, and the native `paths:` filters of `mcp-integration.yml`), *Then* every job any mechanism would have run is covered by an affected target or a retained non-Bazel filter (misses: 0), reported separately per mechanism.
- Deleted, renamed and non-graph files fail safe.
  - *Given* a PR that deletes `session/old.go`, renames a file, or touches a file under a `.bazelignore`d directory or one absent from the graph, *When* `affected-targets.sh` runs, *Then* it does not error and does not under-select: deleted files select the rdeps of their still-existing package (`//pkg:all`), or `//...` if the package is gone; a path matched by `non-bazel-inputs.txt` selects its mapped jobs; any other unmatched path selects `//...`.
- E2E is selectable.
  - *Given* a change under `web-app/src`, *When* it runs, *Then* the e2e build prerequisite targets (`//:stapler-squad`, `//web-app:dist`) are included.
**Files**: `tools/ci/affected-targets.sh`, `tools/ci/affected-targets_test.sh`, `tools/bzl/global-inputs.txt`

##### Task 7.3a: affected-targets.sh and GlobalInputs (cost M)
- `git diff --name-only --diff-filter=d <base>...HEAD` (existing files only; from the compare API in the checkout-less pre-job, per `build.yml:78-84`) mapped with `bazel query 'rdeps(//..., set(<files>))'` plus `kind(".*_test")`; deleted files (`--diff-filter=D`) and renamed-away paths are mapped to their containing package as in the AC because a removed file is not in the graph and `rdeps` would error. `tools/bzl/global-inputs.txt` is the single list for `//...`; `tools/bzl/non-bazel-inputs.txt` lists the inputs that are not Bazel targets and the jobs they select: `tests/e2e/**`, `docs/registry/**`, `website/**`, `.github/**`, `web-app/.jscpd.json`, the Playwright and Lighthouse configs, `scripts/e2e-baseline-compare.ts`, `server/middleware/testdata/**`, `.bun-version`.
- Files: `tools/ci/affected-targets.sh`, `tools/bzl/global-inputs.txt`, `tools/bzl/non-bazel-inputs.txt`
- Verify: `bash tools/ci/affected-targets_test.sh` exits 0.

##### Task 7.3b: Fixture tests (cost S)
- Tests for the five ACs (including the deleted/renamed/non-graph case) using `bazel query` against the repo.
- Files: `tools/ci/affected-targets_test.sh`
- Verify: same command prints `PASS` five times.

##### Task 7.3c: Historical comparison against the three selection mechanisms (cost M)
- First inventory the real mechanisms (VERIFIED: `build.yml:97` is a `grep -qE` regex over changed paths; `tools/ci/detect-feature-changes.sh` has no `grep -E`; `mcp-integration.yml` uses native `paths:` filters with no `grep`), then for 30 merged PRs replay each mechanism with its own replay logic (regex: run the pattern on the changed-file list; script: run it; `paths:` filters: glob-match the changed files with GitHub's semantics) versus `affected-targets.sh`; report misses per mechanism; switch to bazel-diff only if misses > 0 (`research/architecture.md` 6c).
- Files: `tools/ci/compare-affected-vs-regex.sh`, `project_plans/bazel-conversion/implementation/affected-soak.md`
- Verify: `grep -cE '^misses_(regex|script|paths): 0$' project_plans/bazel-conversion/implementation/affected-soak.md` prints `3`.

#### Story 7.4: Remote cache wiring (server-enforced read-only for PRs)
**As the** owner, **I want** CI to use the onyx cache with a read-only server endpoint on PRs and a write path only from `main`, **so that** PR speed benefits without poisoning risk and without secrets in artifacts.
**Acceptance Criteria**:
- PR cannot write, by construction.
  - *Given* a PR run (same-repo branch, workflow edited to drop `--noremote_upload_local_results`), *When* the build runs, *Then* the cache upload attempt is rejected by the server (the PR job only has the `https://<onyx>:9092` read-only HTTP endpoint and the `tag:ci-pr` Tailscale key; the write credential is unavailable because `bazel-cache-write` is restricted to `main`), and the build still succeeds.
- Main writes.
  - *Given* a push to `main`, *When* it completes, *Then* the next PR build reports remote cache hits greater than 0 (`bazel-trend.sh`).
- No credential in any uploaded artifact.
  - *Given* any finished run, *When* the "secret scan" step greps `bep.json`, the gunzipped profile, the execution log and the uploaded trend files for the credential value, its base64 form and `Authorization` (outside `tools/bzl/secret-scan-allow.txt`), *Then* the step fails the job if anything matches; the `main` write job uploads only the scrubbed trend JSON.
- Write path is monitored.
  - *Given* `ci-cache-write` runs on pushes to `main`, *When* 3 consecutive `main` runs record a PUT 4xx/5xx or the next PR build reports a hit rate of 0, *Then* a `cache-write-health` job fails and notifies the owner (a silently broken credential must not erode cache hits unnoticed).
- Cache-touching workflows cannot be hijacked or over-granted.
  - *Given* every workflow that references `TS_AUTHKEY_PR`, `TS_AUTHKEY_MAIN` or `BAZEL_REMOTE_WRITE_PASSWORD`, *When* `tools/bzl/check-workflows.sh` runs, *Then* it exits non-zero if the workflow uses `pull_request_target`, or `workflow_run` with a checkout of PR code, or an unpinned `tailscale/github-action`; and `tools/bzl/check-tailscale-acl.sh <acl.json>` fails unless the policy is default-deny, has no `autogroup:member` or `*:*` grant reaching `tag:ci-pr`/`tag:ci-main`, `tagOwners` lists only the owner, and `tag:ci-pr` reaches only `:9092`.
- Missing secrets or unreachable cache (fork PR).
  - *Given* the secrets are absent, *When* the workflow runs, *Then* it builds with the disk cache only and exits as normal.
**Files**: `.bazelrc`, `.github/workflows/bazel.yml`, `tools/bzl/credential-helper.sh`, `tools/bzl/check-workflows.sh`, `tools/bzl/check-tailscale-acl.sh`, `tools/bzl/secret-scan-allow.txt`, `docs/how-to/run-bazel-remote-on-onyx.md`

##### Task 7.4a: .bazelrc cache configs and credential helper (cost S)
- `build:ci-cache-read --remote_cache=https://<onyx>:9092 --noremote_upload_local_results --remote_download_toplevel` plus the failure-policy lines of Task 0.2b (`--remote_timeout=3s --remote_retries=1`, `--experimental_circuit_breaker_strategy=failure --experimental_remote_failure_rate_threshold=10 --experimental_remote_failure_window_interval=60s`, `--remote_cache_compression`) and no `--remote_local_fallback` (verified names, Task 0.2b; `toplevel` is already the 9.3.1 default and is kept for explicitness; `minimal` for test-only jobs, so CI does not download every intermediate `.a` and test binary over a residential uplink, pre-mortem #5) and `build:ci-cache-write --remote_cache=https://<onyx>:9093 --credential_helper=<onyx>=%workspace%/tools/bzl/credential-helper.sh` with the same failure-policy lines (`check-bazelrc.sh` asserts the pair matches); the helper prints the JSON headers from an environment variable and is the only place the secret is read. No `--remote_header` carrying a secret anywhere (a header on the command line lands in process arguments and possibly BEP/profile). Hosts and ports come from a non-secret repository variable, not the file, if the owner prefers.
- Files: `.bazelrc`, `tools/bzl/credential-helper.sh`
- Verify: `grep -c 'remote_header' .bazelrc` prints `0`; `grep -c 'remote_local_fallback' .bazelrc` prints `0`; `grep -c 'experimental_circuit_breaker_strategy' .bazelrc` prints `2` (read and write configs); `grep -c 'grpcs\?://' .bazelrc` prints `0` for the cache configs (the HTTP protocol is the only one the read-only listener can carry); `bash tools/bzl/credential-helper.sh <<< '{"uri":"https://onyx"}' | jq -e '.headers.Authorization'` prints a value only when the env var is set and nothing otherwise.

##### Task 7.4b: Workflow wiring and secrets (cost S)
- PR jobs: Tailscale step with the `tag:ci-pr` ephemeral key (secret `TS_AUTHKEY_PR`), `--config=ci-cache-read`, no other credential. Write job: `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`, `environment: bazel-cache-write` (deployment branch rule: `main` only), secrets `TS_AUTHKEY_MAIN` and `BAZEL_REMOTE_WRITE_PASSWORD`, `--config=ci-cache-write`; no `workflow_dispatch` path reaches the write job from a non-`main` ref. Both gated on secret presence. Add the secret-scan step and the `cache-write-health` job (alert after 3 consecutive failed `main` uploads). Add a "cache pre-flight" step right after the Tailscale join: `curl --max-time 3 -fsS https://<onyx>:9092/status || echo "BAZEL_CACHE=off" >> "$GITHUB_ENV"`; when `BAZEL_CACHE=off` the job drops `--config=ci-cache-read` and restores `actions/cache` for `~/.cache/stapler-squad-bazel/disk` keyed on the `MODULE.bazel.lock`/`go.sum` hashes (ADR-004's fallback becomes a standing second tier, pre-mortem #4). A daily scheduled job runs `check-tailscale-acl.sh` against the live ACL and alerts on any new `tag:ci-*` node outside a workflow run (pre-mortem #14). Pin `tailscale/github-action` by SHA and use ephemeral, pre-approved, short-expiry tag-scoped auth keys with a rotation note in the runbook. Add `tools/bzl/check-workflows.sh` (fails on `pull_request_target` or PR-checkout `workflow_run` in any workflow referencing the cache secrets) and `tools/bzl/check-tailscale-acl.sh` (asserts the ACL properties of Story 7.4), both run from `make ready` and `bazel.yml`.
- Files: `.github/workflows/bazel.yml`, `tools/bzl/check-workflows.sh`, `tools/bzl/check-tailscale-acl.sh`
- Verify: `actionlint` exits 0; `bash tools/bzl/check-workflows.sh; echo $?` prints `0`; fork-PR simulation (`workflow_dispatch` with empty secrets) succeeds; a PR-branch run of the write job is rejected by the environment rule.

##### Task 7.4c: Disk-cache GC and hit-rate report (cost S)
- Apply verified GC flag names (0.2b: `--experimental_disk_cache_gc_max_size=100G`, `_max_age=30d`, `_idle_delay=10s`, the delay kept below the shortest `max_idle_secs`), with the disk-cache size capped at 100 GB and `df` of the cache path in the trend alarm (pre-mortem #6). Execution-log tooling (D18): `--execution_log_compact_file` plus the parser built from the Bazel source tree for full builds, `--execution_log_json_file` for spikes; `--execution_log_sort` default true. No default config passes `--profile=<path>` (D21). The trend script reports the eviction-retry count (Task 7.1a). `bazel-trend.sh` reports hit ratio per run and outbound bytes per run (pre-mortem #5). Numeric thresholds (validation G7, G8): the median hit ratio over the last 20 warm `main` push runs is at least 0.60, and `output_base_disk_kb` is at most 1.25 x the baseline `disk_kb_worktree`; both are rows of P10, below-floor values raise a workflow annotation, and the owner may waive either in `owner-attestations.yaml`.
- Files: `.bazelrc`, `tools/ci/bazel-trend.sh`
- Verify: `bazel help build --long | grep -c 'experimental_disk_cache_gc'` is at least 3 (max_size, max_age, idle_delay) and flags in `.bazelrc` match those names; `grep -c -- '--profile' .bazelrc` prints `0`.

#### Story 7.5: Parity gate P1-P10
**As the** owner, **I want** one scripted gate with measurable pass criteria, **so that** cutover is a checklist with evidence.
**Acceptance Criteria**: the gate passes when every row below passes on the same commit; results are posted as a PR comment by `.github/workflows/bazel-parity.yml` and saved as an artifact.

| Id | Check | Measurable pass criterion | Script / source |
|---|---|---|---|
| P1 | Test-set parity (default tags) | `missing_in_bazel: 0` after `ExclusionsFile` (at most 10 packages listed, each with owner+issue) | `test-set-diff.sh default` |
| P2 | Tag-matrix parity | same, for `race`, `integration`, `sinkguard`, `gitoracle` | `test-set-diff.sh <config>` |
| P3 | Gazelle idempotence and fresh clone | Gazelle diff exit 0 twice; `bazel build //...` exit 0 in an empty temp clone with no preconditions | `parity/fresh-clone.sh` |
| P4 | Generated-code equivalence | `compare-proto.sh`, `compare-ent.sh` exit 0 (gofmt-normalised; TS equal) | existing scripts |
| P5 | Binary behaviour | `compare-binary.sh` exit 0 (version fields equal, embed sha256 equal to pins) | `compare-binary.sh` |
| P6 | Web parity | `compare-web.sh` exit 0; Jest pass-set diff empty | `compare-web.sh` |
| P7 | E2E | Playwright suite pass count against the Bazel binary equals the make-binary run; after Epic 12, also the full set of `//tests/e2e:all` targets run with `--nocache_test_results` against the Bazel binary has a comparator verdict equal to the legacy runner's | `parity/e2e.sh` (uses `TEST_SERVER_BINARY`) |
| P8 | macOS | Signed Bazel binary keeps TCC grants on arm64 and amd64; `otool` check passes | owner checklist (Task 5.2b) |
| P9 | Deploy dry run | Isolated manual instance (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`) from the Bazel binary passes the smoke list | `tools/bzl/manual-instance` (Go; Task 7.5e, moved here from Epic 9 so the gate can pass before Epic 8) |
| P10 | Performance | Warm no-op `bazel build //:stapler-squad` at most 10 s (median of 5 runs); PR time-to-green (same definition as Task 0.1a: last required workflow completion minus first start per head SHA) p50 over at least 20 Bazel PR runs is at most 1.10 x the `baselines.md` p50 (10% tolerance for runner variance, with host load recorded beside each sample) and the per-workflow p50 of every required workflow is also within 1.10 x its baseline; second-worktree cold build median of 5 at most 30% of the `baseline-worktree.json` make median (same definition as Story 0.10); per-worktree disk (`output_base_disk_kb`) not more than 1.25 x baseline `disk_kb_worktree` unless the owner waives; cache hit ratio median at least 0.60 over the last 20 warm `main` push runs unless the owner waives; **cold row**: time-to-green with an empty remote cache and an empty `actions/cache` is at most 1.5 x the `baselines.md` p50, and Task 7.6a is blocked until it holds; outbound bytes per run are reported, and if the 1.10 x per-workflow bound fails ADR-004 falls back to `actions/cache` as the primary tier. **Improvement rows (required)**: T1-T8 from "Improvement targets" above, with values from `tools/bzl/baseline/targets.yaml` (set at Story 0.1 from baseline, owner-confirmed): T1 work-avoided ratio, T2 billed minutes and time-to-green on partial-change PRs (cache-outage runs excluded), T2b warm cache hit ratio, T3 second-worktree cold build and disk, T4 edit-build-test latency, T5 first-error legibility, T6 regen steps per session (reported until Story 10.1), T7 hermeticity (H1-H4 against `hermeticity-make.json`; the report prints `h_downgrades`), T8 e2e work avoided and escapes (from `e2e-avoided.sh`, Task 12.4d). Non-regression bounds and improvement rows are both required; improvement rows are never waivable | `bazel-trend.sh`, `bazel-worktree-cost.sh`, `improvement.sh` |

*Given* all ten rows passing, *When* `go run ./tools/bzl/parity/run-all` runs, *Then* it prints `PARITY: PASS` and exits 0; any failing row prints `PARITY: FAIL P<n>` and exits 1. Waivers never pass silently: when any waiver is in force the line is `PARITY: PASS WITH WAIVERS (<row>: <reason> until <date>; ...)`, the same list is the first block of the PR comment and the saved artifact, and an expired or malformed waiver is `PARITY: FAIL WAIVER <row>` (Tasks 7.5c, 7.5f). Only the hit-ratio floor and the 1.25 x disk ceiling rows of P10 are waivable.

**Files**: `tools/bzl/parity/*.sh` (compare scripts) and `tools/bzl/parity/*/main.go` (Go tools), `.github/workflows/bazel-parity.yml`

##### Task 7.5a: run-all orchestrator and fresh-clone check (cost M)
- Orchestrate P1-P7 and P10 scripts; `fresh-clone` implements P3 in a short temp dir.
- Files: `tools/bzl/parity/run-all/main.go`, `tools/bzl/parity/fresh-clone/main.go`
- Verify: `go run ./tools/bzl/parity/run-all --dry-run | grep -cE '^P([1-9]|10): '` prints `10`; `go run ./tools/bzl/parity/fresh-clone; echo $?` prints `0`.

##### Task 7.5b: E2E parity and TEST_SERVER_BINARY (cost M)
- `parity/e2e.sh` builds the Bazel binary and runs `cd tests/e2e && TEST_SERVER_BINARY=<path> bun run test`; compare pass counts with a make-binary run; do not change `tests/e2e/helpers/test-server.ts` semantics beyond honouring the env it already supports. Validation G9: on the P9 isolated instance also diff the sorted metric-name set of `/metrics` from the Bazel-built binary against the make-built binary (`curl -s localhost:62871/metrics | grep -oE '^[a-z_]+' | sort -u`); the sets must be identical.
- Files: `tools/bzl/parity/e2e.sh`
- Verify: `bash tools/bzl/parity/e2e.sh; echo $?` prints `0` (the script exits 0 only when the Bazel-binary and make-binary Playwright pass counts are equal and the sorted `/metrics` name sets are identical, printing `e2e_pass_bazel=<n> e2e_pass_make=<n> metrics_equal=true`).

##### Task 7.5c: Scheduled parity workflow (cost S)
- `bazel-parity.yml`: scheduled and `workflow_dispatch`, posts the table. P8/P9 and the flip/retention rows read `project_plans/bazel-conversion/implementation/owner-attestations.yaml`, which an agent PR can edit, so the attestation is trusted only if (a) the job reading it runs in the GitHub environment `bazel-owner-attest` whose required reviewer is the owner (approval is the signature), and (b) the last commit touching the file has a verified signature or author identity equal to the owner identity in `tools/bzl/owner.txt` (`git log -1 --format='%ae %G?' -- <file>`). `.github/CODEOWNERS` (Task 0.2e) adds review when the owner enables protection, but neither check depends on it. **Waivers** are entries `waivers: [{row, reason, granted, expires}]` in the same file, validated by Task 7.5f: `reason` is non-empty (at least 20 characters), `expires` is at most 90 days after `granted` (proposal; owner may tune), the row must be one of the two waivable P10 rows, and the parity report lists every active waiver (see the AC of Story 7.5).
- Files: `.github/workflows/bazel-parity.yml`, `project_plans/bazel-conversion/implementation/owner-attestations.yaml`, `tools/bzl/owner.txt`
- Verify: `actionlint` exits 0; a seeded attestation edit by a non-owner identity makes the attest step exit 1.

##### Task 7.5e: Manual isolated instance script (P9) (cost S)
- `tools/bzl/manual-instance` (Go; run as `go run ./tools/bzl/manual-instance`) builds with Bazel, copies to `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, checks ports 62871/62872 free (`ss`), starts with `PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 --tmux-keep-server`, prints the smoke list, and a `--stop` mode that kills it and `tmux -L ssq-manual-1 kill-server`. (Moved from Task 9.1c so P9 does not depend on Epic 9.)
- Files: `tools/bzl/manual-instance/main.go`, `tools/bzl/manual-instance/main_test.go`, `tools/bzl/manual-instance/BUILD.bazel`
- Verify: `go vet ./tools/bzl/manual-instance` exits 0; `go run ./tools/bzl/manual-instance --dry-run` prints the exact build, copy, start and stop commands (ports 62871/62872, `ssq-manual-1`) and runs nothing, so it cannot touch `:8543`.

##### Task 7.5d: P10 perf collector (cost S)
- Script measuring the cold row (cache disabled, `actions/cache` empty), warm no-op build time (median of 5) and joining with `baselines.md` numbers using the P10 tolerances; records `loadavg1` per sample and the wall-time effect of every remaining `exclusive`-tagged test (Story 4.5) as a separate line so serialisation cost is visible.
- Files: `tools/bzl/parity/perf.sh`
- Verify: `bash tools/bzl/parity/perf.sh | jq -e '.warm_noop_s<=10'` prints `true`.

##### Task 7.5f: Waiver validator and report block (cost S)
- `tools/bzl/parity/check-waivers.py` validates `waivers:` in `owner-attestations.yaml` (non-empty reason of at least 20 characters, `expires` within 90 days of `granted`, row in the waivable set, not expired today, attested through the same environment-approval and signed-commit checks as Task 7.5c) and prints the waiver block that `run-all.sh` places first in the report and in the `PARITY:` line. A waiver on any other row (including the cold row, the 1.10 x non-regression bounds and T1-T7) is rejected.
- Files: `tools/bzl/parity/check-waivers.py`, `tools/bzl/parity/check-waivers_test.py`, `tools/bzl/parity/run-all.sh`
- Verify: `python3 -m unittest tools/bzl/parity/check-waivers_test.py` exits 0 with cases for an empty reason, an expiry 120 days out, an expired waiver, a waiver on `P5`, and a valid one; `bash tools/bzl/parity/run-all.sh --dry-run --waivers tools/bzl/parity/testdata/waiver-ok.yaml | grep -c 'PARITY: PASS WITH WAIVERS'` prints `1`.

##### Task 7.5g: Improvement-target collector T1-T8 (cost M)
- `tools/bzl/parity/improvement.sh` computes: T1/T2 by replaying `affected-targets.sh` over the 30 sampled partial-change PR diffs of Task 0.1i (reusing the Task 7.3c harness and the T1 replay of Task 2.7a; it prints `affected_target_count`, `total_target_count` and their ratio) and joining runner-minutes (the Task 0.1i definition) and time-to-green from the Bazel PR runs against the baseline for the partial class, the global class and the whole population; T3 from `bazel-worktree-cost.sh` (second vs first worktree cold build, `output_base_disk_kb`); T2b from `bazel-trend.sh` (cache hit ratio over the last 20 warm `main` push runs); T4 via `tools/bzl/parity/edit-build-test.sh` (edit one comment in a non-test file of `config/`, run `go test ./config` warm, median of 5, `loadavg1` recorded); T5 by running `check-error-legibility.sh` (Task 1.6d); T6 by running `regen-steps.sh` over post-flip transcripts (reported as `UNMEASURED` before the flip). T7 by `tools/bzl/baseline/hermeticity.sh --system bazel` (H1-H3 against `hermeticity-make.json`; H4 is the row count of `tools/bzl/hermetic-downgrades.txt`, printed in the parity report, so every D-HERMETIC downgrade is visible and counted). Compares each with `targets.yaml` and prints one `T<n>: PASS|FAIL|UNMEASURED` line each; `UNMEASURED` is a FAIL for T1-T5, T7 and T8 at the gate. T8 is computed by `tools/bzl/parity/e2e-avoided.sh` (Task 12.4d) and is `UNMEASURED` only before the Epic 12 shadow job exists.
- Files: `tools/bzl/parity/improvement.sh`, `tools/bzl/parity/edit-build-test.sh`, `tools/bzl/parity/run-all.sh`
- Verify: `bash tools/bzl/parity/improvement.sh | grep -cE '^T[1-8]: (PASS|FAIL|UNMEASURED)$'` prints `8`; `bash tools/bzl/parity/improvement.sh --fixture tools/bzl/parity/testdata/improvement-fail.json; echo $?` prints `1`.

#### Story 7.6: Soak and per-job cutover
**As the** owner, **I want** each CI job flipped separately after evidence, **so that** any regression is one revert.
**Acceptance Criteria**:
- Soak met, by one rule.
  - *Given* the non-blocking job and the ledger `project_plans/bazel-conversion/implementation/soak-ledger.yaml` (one row per PR run: `run_id`, `result`, and for each failure an attribution `flake` with a BUG id | `bazel-real` | `infra` | `generated-file-race`, the last counted and reported separately and driven to zero by Story 7.8), *When* the last N PR runs (owner fixes N before Epic 7; proposal 30) contain zero unattributed failures, zero `bazel-real` failures in the last K runs (K=10), and `PARITY: PASS` holds, *Then* Task 7.6a may proceed. Failures attributed to known flakes (BUG-087..090) do not reset the count; an attributed `bazel-real` failure does. This replaces the earlier "N consecutive green" rule, which was unmeetable while those flakes exist.
- "Blocking" is defined for a repo with no branch protection.
  - *Given* `gh api repos/tstapler/stapler-squad/branches/main/protection` returns 404 and `rulesets` is `[]` (VERIFIED 2026-11-01), *When* a job is flipped, *Then* flipping means removing `continue-on-error` from that job and the legacy job keeps running; if the owner decides D-PROTECT-MERGE (a), the owner adds a ruleset requiring the listed job names (owner action, command in `docs/how-to/build-with-bazel.md`); there is no required-check rename because no required checks exist.
- Make selection logic is removed last, with the legacy jobs.
  - *Given* the retention window has ended, *When* Task 10.2c merges, *Then* `grep -rn 'detect-feature-changes' .github tools Makefile` prints nothing and the `build.yml:97` regex is gone; the native `paths:` filters in `mcp-integration.yml` may stay.
**Files**: `.github/workflows/build.yml`, `.github/workflows/mcp-integration.yml`, `.github/workflows/_prepare.yml`, `project_plans/bazel-conversion/implementation/soak-ledger.yaml`

##### Task 7.6a: Flip Go build/test job to Bazel (cost M) (blocked on soak and on the P10 cold row)
- Add the Bazel build/test job using `affected-targets.sh` (and `//...` on `main`) without `continue-on-error`. Keep the old `prepare`/`test-affected`/`go-guards` jobs renamed `legacy-*`, running unconditionally (full, no path selection, so the regex is not needed) on every PR during the retention window: they are the PR-time guard that the make path still works (see Story 10.1).
- Files: `.github/workflows/build.yml`, `.github/workflows/_prepare.yml`
- Verify: `actionlint` exits 0; `PR=$(gh pr view --json number --jq .number); gh pr checks $PR --json name,state --jq '[.[]|select(.name|test("^(bazel-|legacy-)"))|.state]|(length>=2) and all(.=="SUCCESS")'` prints `true`.

##### Task 7.6b: Flip e2e jobs to the Bazel binary (cost M)
- `e2e-binary-*` jobs build `//:stapler-squad` via Bazel and set `TEST_SERVER_BINARY`. This keeps the full legacy Playwright runner; selective execution on PRs is Epic 12 (ADR-009), which supersedes the unconditional full PR run at Task 12.6a.
- Files: `.github/workflows/build.yml`
- Verify: `actionlint` exits 0; `PR=$(gh pr view --json number --jq .number); gh pr checks $PR --json name,state --jq '[.[]|select(.name|test("e2e"))|.state]|(length>=1) and all(.=="SUCCESS")'` prints `true`.

##### Task 7.6c: Flip mcp-integration and lint jobs (cost M)
- `mcp-integration.yml` selects via AffectedSet (its native `paths:` filters are replaced or kept per the 7.3c inventory); `lint.yml` runs wrappers after `make sync`.
- Files: `.github/workflows/mcp-integration.yml`, `.github/workflows/lint.yml`
- Verify: `actionlint` exits 0; `PR=$(gh pr view --json number --jq .number); gh pr checks $PR --json state --jq 'all(.state=="SUCCESS" or .state=="SKIPPED")'` prints `true`.

##### Task 7.6d: Record the D-PROTECT-MERGE outcome (cost XS)
- If the owner chose (a), record the ruleset id and required job names in `docs/how-to/build-with-bazel.md`; if (b), record the merge-gate convention. No workflow change.
- Files: `docs/how-to/build-with-bazel.md`
- Verify: `grep -c 'D-PROTECT-MERGE' docs/how-to/build-with-bazel.md` is at least `1`.

#### Story 7.7: Determinism check and non-hermetic action policy
**As the** owner, **I want** determinism proven before non-hermetic actions are cacheable remotely, **so that** every worktree cannot consume a bad artifact. (Repository rules such as the Bun install are never remote-cached and cannot carry `no-remote-cache`; the exposure is a non-byte-reproducible `node_modules` changing the input digest of the consuming actions, which causes cache misses, and the actions that consume it carry the tag until their row passes.)
**Acceptance Criteria**:
- *Given* two clean output bases, *When* the determinism job builds `//:stapler-squad` (release config) in each, *Then* execution-log digests match for every cacheable action; any mismatch lists the action mnemonic and fails the job; actions tagged `no-remote-cache` stay tagged until their row passes.
**Files**: `.github/workflows/bazel-determinism.yml`, `tools/bzl/determinism.sh`

##### Task 7.7a: Determinism job (cost M)
- Use `--execution_log_compact_file` (zstd-compressed `ExecLogEntry` protos; the documented "compact format is recommended") and compare after converting with the `//src/tools/execlog:parser` or `:converter` built from a pinned Bazel source checkout (D18; `--execution_log_json_file` is human-readable but large and is for spike builds only); scheduled and `workflow_dispatch`. The comparison is the same as experiment E4 of Task 0.10e, run on `main`. Pre-mortem #8: the job is a required nightly, its verdict is kept in `owner-attestations.yaml`, and any D-HERMETIC downgrade (host-tool web or tmux action tagged `no-remote-cache`) must list each affected action and the tool-version pin it relies on (`.bun-version`, `tools/bzl/tmux/PIN`) in `docs/reference/bazel-targets-and-configs.md`. Every such downgrade is also a row in `tools/bzl/hermetic-downgrades.txt` (action, tool, pin, reason, owner-accepted date), which feeds T7 H4 and is printed in the parity report, so D-HERMETIC downgrades are counted against the hermeticity target and cannot accumulate silently.
- Files: `.github/workflows/bazel-determinism.yml`, `tools/bzl/determinism.sh`, `tools/bzl/hermetic-downgrades.txt`
- Verify: `bash tools/bzl/determinism.sh //tools/lint/... ; echo $?` prints `0`.

#### Story 7.8: Auto-repair of generated files (pre-mortem #2)
**As an** agent, **I want** generated-file drift repaired for me, **so that** two individually green PRs cannot leave `main` red (about 18 commits a day land from about 70 live worktrees, `main` has no merge queue, and GitHub-side merges apply no repo-defined merge drivers, which is INFERRED and reproduced by spike S13).
**Acceptance Criteria**:
- `main` repairs itself.
  - *Given* a `main` push after which `bazel run //:gazelle -- -mode=diff`, `bazel mod tidy` or `make gen-freshness` is non-empty, *When* `bazel-main-repair.yml` runs, *Then* it opens a PR (or pushes a fast-forward commit to a repair branch) titled `chore(build): regenerate BUILD files` within 10 minutes, and a seeded drift test proves it.
- PRs repair themselves.
  - *Given* a same-repo PR that is red only on the freshness gates, *When* `bazel-autofix.yml` runs, *Then* it pushes the regenerate commit to the PR branch as a normal push (never forced) and the gate re-runs green.
- BUILD conflicts do not need a human.
  - *Given* two branches that conflict only in `**/BUILD.bazel`, *When* merged locally with `.gitattributes` `**/BUILD.bazel merge=bazel-regen`, *Then* the driver takes either side, reruns Gazelle, and the tree is clean. The remedy command is in the backlog dispatcher prompt.
**Files**: `.github/workflows/bazel-main-repair.yml`, `.github/workflows/bazel-autofix.yml`, `.gitattributes`, `scripts/git-merge-bazel-regen.sh`

##### Task 7.8a: `bazel-main-repair` workflow (cost M)
- `on: push` to `main`; runs the three freshness commands; if any diff is non-empty, commits the regenerated files to a repair branch and opens or updates one PR with the label `generated-file-race`; uses no force push (Task 0.2h check applies) and the credentials below. **Trigger problem:** a push made with the default `GITHUB_TOKEN` does not start other workflows, so the repair branch would get no CI. Either (a) check out and push with `secrets.BAZEL_REPAIR_TOKEN` (a GitHub App installation token or a fine-grained PAT limited to this repository with contents and pull-requests write, held in the environment `bazel-repair`; owner action and wall-clock blocker, never echoed, covered by `check-workflows.sh`), or (b) with no extra credential keep `GITHUB_TOKEN` and run `gh workflow run bazel.yml --ref <repair-branch>` (a `workflow_dispatch` created with `GITHUB_TOKEN` is allowed to start a run; `bazel.yml` must declare `workflow_dispatch`). The workflow implements (b) by default and switches to (a) when the secret exists. Seeded-drift test on a scratch branch.
- Files: `.github/workflows/bazel-main-repair.yml`
- Verify: `actionlint` exits 0; after a seeded drift commit, `gh pr list --label generated-file-race --json number --jq length` prints at least `1` within 10 minutes of the push, and `REPAIR=$(gh pr list --label generated-file-race --json headRefName --jq '.[0].headRefName'); gh run list --workflow=bazel.yml --branch "$REPAIR" --json event --jq 'length>=1'` prints `true` (the repair push really started CI).

##### Task 7.8b: PR autofix job and BUILD merge driver (cost S)
- `bazel-autofix.yml` runs only for same-repo PRs (never forks, never `pull_request_target`), pushes the regenerate commit to the PR branch. `scripts/git-merge-bazel-regen.sh` (takes either side, reruns `bazel run //:gazelle`) is registered by `make setup-git` (created in Task 0.2h, hard dependency on Tasks 0.2h and 0.10b) beside the lock driver of Task 0.10b; the autofix push uses the same token choice as Task 7.8a so the re-run of the gate actually starts; if S13 chose not to commit the lock, the lock driver is dropped.
- Files: `.github/workflows/bazel-autofix.yml`, `.gitattributes`, `scripts/git-merge-bazel-regen.sh`
- Verify: `actionlint` exits 0; the two-branch BUILD.bazel conflict simulation leaves `bazel run //:gazelle -- -mode=diff; echo $?` printing `0`.

##### Task 7.8c: Dispatcher prompt line and soak attribution class (cost XS)
- Add the remedy line (`make gazelle && bazel mod tidy && make sync`) to the backlog dispatcher prompt and `.claude/rules/bazel-build-files.md`; `soak-ledger.yaml` gains attribution class `generated-file-race` (Story 7.6), reported separately and expected to reach zero after this story ships.
- Files: `.claude/rules/bazel-build-files.md`, `project_plans/bazel-conversion/implementation/soak-ledger.yaml`
- Verify: `grep -c 'generated-file-race' project_plans/bazel-conversion/implementation/soak-ledger.yaml .claude/rules/bazel-build-files.md` prints at least `1` per file.

---

## Epic 8: Makefile wrappers, BUILD_SYSTEM switch, lint gates

**Goal**: Existing `make` names keep working and delegate to Bazel when `BUILD_SYSTEM=bazel`; the flip is one PR; the make path stays alive and tested until Epic 10.

#### Story 8.1: Wrapper framework and discoverability
**As an** agent, **I want** `make help` to keep listing everything and show the underlying command, **so that** I learn Bazel by copying it.
**Acceptance Criteria**:
- *Given* `BUILD_SYSTEM=bazel make -n build`, *When* it runs, *Then* it prints a `bazel build ...` command and does not invoke `go build`.
- *Given* `BUILD_SYSTEM=make make -n build`, *When* it runs, *Then* it prints the legacy recipe.
- *Given* an unknown value, *When* `BUILD_SYSTEM=nix make build` runs, *Then* it exits 2 with `BUILD_SYSTEM must be make or bazel`.
- *Given* every wrapper target, *When* `tools/bzl/check-make-help.sh` runs, *Then* each has a `##` description containing the Bazel command.
**Files**: `Makefile`, `make/bazel.mk`, `tools/bzl/check-make-help.sh`

##### Task 8.1a: make/bazel.mk with BUILD_SYSTEM validation and agent flags (cost S)
- Extend the `make/bazel.mk` created in Task 1.5a (it already holds `BUILD_SYSTEM` and its validation) with `BAZEL`, `BAZEL_FLAGS` (adds `--config=agent` when `! [ -t 1 ]`), and the `define wrap` helper. Every wrapper recipe's first output line is the banner `[build-system=<bazel|make>] <exact command>` (the `make go-*` shims print `[native go - not authoritative]`), so an agent always sees which system ran and can copy the command.
- Files: `make/bazel.mk`, `Makefile`
- Verify: `BUILD_SYSTEM=nix make -n build; echo $?` prints the message and `2`; `BUILD_SYSTEM=bazel make -n build | grep -c 'build-system=bazel'` and `BUILD_SYSTEM=make make -n build | grep -c 'build-system=make'` each print at least `1`.

##### Task 8.1b: Help consistency check (cost S)
- Script ensures wrapper targets appear in `make help` with the Bazel command text. It derives the target list from the recipes that use the `wrap` helper (Task 8.1a), so Story 8.2's wrapper targets are covered automatically as they land; at this point only the targets Task 1.5a and Task 8.1a created exist.
- Files: `tools/bzl/check-make-help.sh`
- Verify: `bash tools/bzl/check-make-help.sh; echo $?` prints `0`.

#### Story 8.2: Wrap build and test targets
**As an** agent, **I want** `make build`, `test`, `test-race`, `test-integration` to mean the same thing under either system, **so that** nothing I know breaks.
**Acceptance Criteria**:
- *Given* `BUILD_SYSTEM=bazel`, *When* `make build`, `make stapler-squad`, `make build-embedded`, `make build-embedded-tymux`, `make test`, `make test-race`, `make test-integration`, `make test-oracle`, `make test-delivery-guards`, `make proto-gen`/`ent-gen` (alias of `sync`), `make build-tmux`, `make web-build`, `make test-affected` run, *Then* each exits 0 on a clean main and produces the same artifacts at the same paths as the make recipes (`./stapler-squad` is a copy, never a symlink into `bazel-bin`).
- *Given* the mapping table of Task 8.2e, *When* `bash tools/bzl/check-pkg-mapping.sh` runs, *Then* every row's `make -n` output equals the expected Bazel command (for example `make test PKG=./session` maps to `bazel test //session:all`, not the whole subtree, matching `go test ./session`; `PKG=./session/...` maps to `//session/...`).
- *Given* `BUILD_SYSTEM=bazel`, *When* `make build`, `make test` or `make test-race` runs, *Then* the cheap freshness check of Task 2.4d runs first (Task 8.2d).
**Files**: `Makefile`, `make/bazel.mk`

##### Task 8.2a: Build-class wrappers (cost M)
- `build`, `stapler-squad`, `build-embedded`, `build-embedded-tymux`, `build-tmux`, `web-build`, `proto-gen`, `ent-gen`.
- Files: `Makefile`, `make/bazel.mk`
- Verify: `BUILD_SYSTEM=bazel make build && ./stapler-squad version` prints the version line.

##### Task 8.2b: Test-class wrappers (cost M)
- `test`, `test-race`, `test-integration`, `test-oracle`, `test-delivery-guards`, `test-affected` (via `affected-targets.sh`), `PKG=`/`RUN=`/`TAGS=` handling per Task 8.2e; summaries through `testsummary`.
- Files: `Makefile`, `make/bazel.mk`
- Verify: `BUILD_SYSTEM=bazel make test PKG=./config` exits 0.

##### Task 8.2c: Wrapper parity test (cost S)
- A script runs `make -n` under both values for every wrapper and asserts both produce a non-empty plan.
- Files: `tools/bzl/check-wrappers.sh`
- Verify: `bash tools/bzl/check-wrappers.sh; echo $?` prints `0`.

##### Task 8.2d: Cheap gen-freshness inside the build and test wrappers (cost S)
- `build`, `stapler-squad`, `test`, `test-race`, `test-integration`, `lint`, `vet` and `ready` call `tools/bzl/check-gen-fresh-cheap.sh` first (Task 2.4d; about 100 ms), not only `make ready` and CI. Behaviour: for Bazel-run recipes the result is a one-line warning (`note: gen/ is stale for plain go and gopls; Bazel is unaffected. Run 'make sync'.`) and the recipe continues; for recipes that run native Go tools (`lint`, `vet`, and `test` under `BUILD_SYSTEM=make`) a stale tree fails with `gen: STALE - run 'make sync'` exit 1. Applies under both `BUILD_SYSTEM` values and does not require Bazel on `PATH`. The same wrappers call `tools/bzl/check-build-fresh-cheap.sh --warn` (Task 1.6f) and print its one-line `note: BUILD.bazel may be stale for <pkg> - run 'make gazelle'` before the Bazel command (warning only).
- Files: `make/bazel.mk`, `Makefile`, `tools/bzl/check-wrappers.sh`
- Verify: with a stale stamp, `make -n test | head -3 | grep -c 'check-gen-fresh-cheap'` prints `1` under both values; `BUILD_SYSTEM=bazel make test PKG=./config 2>&1 | grep -c "Run 'make sync'"` prints `1` and the exit code is `0`; `BUILD_SYSTEM=make make lint` on a stale tree exits `1`; after adding a new `.go` file under `config/`, `BUILD_SYSTEM=bazel make test PKG=./config 2>&1 | grep -c 'BUILD.bazel may be stale'` prints `1`.

##### Task 8.2e: `PKG=`, `RUN=` and `TAGS=` mapping table and test (cost S)
- Specify and implement (table-driven, one row per case in `tools/bzl/testdata/pkg-mapping.tsv`): `PKG=./session` -> `//session:all`; `PKG=./session/tmux` (nested) -> `//session/tmux:all`; `PKG=./session/...` -> `//session/...`; `PKG="./config ./cmd/ssq-hooks"` -> both labels; `PKG=./tools/lint/...` (separate module) -> `//tools/lint/...`; `RUN=TestFoo` (the old `-run`) -> `--test_filter=TestFoo`; `TAGS=sinkguard` -> `--config=sinkguard`; `TAGS="embed_tmux sinkguard"` -> `--config=embed_tmux --config=sinkguard`; `TAGS=integration` -> `--config=integration`; `make test-race` or `RACE=1` -> `--config=race`; `TEST_TAGS=-integration` -> `--test_tag_filters=-integration`; `FORCE=1` (the old `go test -count=1` rerun) -> `--nocache_test_results`, and a run served from the cache prints `(N test targets cached; rerun with FORCE=1)`; an unknown tag exits 2 with `unknown TAGS value 'x'; valid: embed_tmux embed_tymux sinkguard gitoracle integration race release`; a package with no test targets prints `no tests in ./x (go test would say: no test files)` and exits 0. `make help` shows these variables in the description of `test`, including that `RUN=` is `--test_filter`, not `go test -run` (the semantics recorded by Task 4.2b: how rules_go passes the filter and whether it is anchored), which the skill (Task 11.1a) states too.
- Files: `make/bazel.mk`, `tools/bzl/check-pkg-mapping.sh`, `tools/bzl/testdata/pkg-mapping.tsv`
- Verify: `bash tools/bzl/check-pkg-mapping.sh; echo $?` prints `0` after comparing every row (`grep -vc '^#' tools/bzl/testdata/pkg-mapping.tsv` prints `15`: five `PKG=` cases, `RUN=`, three `TAGS=` cases, `test-race` and `RACE=1`, `TEST_TAGS=`, `FORCE=1`, the unknown tag and the no-tests case); `BUILD_SYSTEM=bazel make -n test TAGS=nope; echo $?` prints the message and `2`.

#### Story 8.3: Lint gates stay wrapped; Bazel gates added
**As an** agent, **I want** `make lint` and `make ready` to include the new Bazel gates, **so that** I get the CI result locally.
**Acceptance Criteria**:
- *Given* a fresh clone, *When* `make ready` runs under `BUILD_SYSTEM=bazel`, *Then* it runs `make sync` first, then golangci-lint, `lint-custom`, dupl and jscpd gates, then Gazelle freshness, `bazel mod tidy` diff, `buildifier` lint and `GenFreshness`, and exits 0 on clean main.
**Files**: `Makefile`

##### Task 8.3a: Add gates to ready (cost S)
- Insert `sync`, `gazelle-check`, `buildifier-check`, `gen-freshness` into `ready` only for `BUILD_SYSTEM=bazel` (they are skipped with a printed line under `make`, per Story 1.5); sentinel-file skip semantics preserved.
- Files: `Makefile`
- Verify: `BUILD_SYSTEM=bazel make -n ready | grep -cE 'gazelle-check|buildifier-check|gen-freshness'` prints at least `3`; `BUILD_SYSTEM=make make -n ready | grep -c 'skipped: BUILD_SYSTEM=make'` prints `3`; `BUILD_SYSTEM=make make -n ready | grep -c 'bazel '` prints `0`.

#### Story 8.4: Custom analyzers as nogo (owner decision: twins for all analyzers)
**As the** owner, **I want** the 17 structural analyzers to run on every build, **so that** the rules in `.claude/rules/*.md` are enforced by the compiler path. `make lint-custom` stays as the parity oracle until the Story 8.6 flip.
**Acceptance Criteria**:
- *Given* a seeded violation for each analyzer (`tools/bzl/nogo/testdata`), *When* `bazel build` compiles it, *Then* it fails with that analyzer's own finding; the twins compile the same `analyzer.go` as `make lint-custom`, so the message and position are the same source (a dual-run of both tools on every seed is not possible: the path-scoped analyzers key on real package paths, which the seeds only stand in for).
- *Given* main, *When* `bazel build //...` runs with nogo enabled, *Then* exit 0.
- *Given* the analyzer tests, *When* `bazel test //tools/lint/...` runs, *Then* all 17 execute (none skips).
**Files**: `MODULE.bazel`, `BUILD.bazel`, `tools/bzl/nogo/**`, `tools/bzl/nogotwins/**`, `tools/bzl/nogoseed/**`, `tools/lint/**/BUILD.bazel`

##### Task 8.4a: Twins and nogo registration (cost M) - done
- Twin library per analyzer plus `internal/nolintcomment` in `tools/bzl/nogo/BUILD.bazel` (sources referenced from `tools/lint`, not copied); `go_sdk.nogo` in `MODULE.bazel`; `nogo_config.json` `_base.exclude_files` for `external/`, `bazel-out/`, `tools/lint/`. No analyzer needs `go/packages` at runtime (all 17 run under nogo).
- Files: `BUILD.bazel`, `tools/bzl/nogo/BUILD.bazel`, `tools/bzl/nogo/nogo_config.json`, `MODULE.bazel`
- Verify: `bazel build //... ; echo $?` prints `0`; `make nogo-twins-check` exits 0 (twin list equals `tools/lint` directories and `cmd/linter`).

##### Task 8.4b: Seeded violations and converted analyzer tests (cost M) - done
- One manual seed target per analyzer (`//tools/bzl/nogo/testdata:seed_<analyzer>`) and `tools/bzl/nogoseed`, which builds them and requires each to fail with its own analyzer; every `analyzer_test.go` calls `gotool.Setup(t)`.
- Files: `tools/bzl/nogo/testdata/**`, `tools/bzl/nogoseed/**`, `tools/lint/*/analyzer_test.go`
- Verify: `make nogo-seed-check; echo $?` prints `0` with 17 `FAILS (finding reported)` rows; `bazel test //tools/lint/...` passes 17 of 17.

#### Story 8.5: Fallback canary
**As the** owner, **I want** the make path exercised on a schedule and on risky PRs, **so that** the escape hatch cannot rot.
**Acceptance Criteria**:
- *Given* a runner with no Bazel installed (the job asserts `! command -v bazel`), *When* `fallback-canary.yml` runs weekly, on `workflow_dispatch`, and on PRs touching `Makefile`, `make/**`, `go.mod`, `go.sum`, `proto/**`, `session/ent/schema/**` or `session/tmux/embed/**`, *Then* `BUILD_SYSTEM=make make ready` passes, `BUILD_SYSTEM=make make build` produces a binary, and one e2e shard passes against that make-built binary; a scheduled failure opens an issue labelled `build-fallback`.
- *Given* the Story 8.6 flip has merged, *When* the weekly run completes its `make-only` job, *Then* a second job `pass-set-equality` (a separate runner with Bazel installed, `needs: make-only`) runs `test-set-diff.sh default`; the `make-only` job never installs Bazel, so the assertion above stays true.
**Files**: `.github/workflows/fallback-canary.yml`

##### Task 8.5a: Scheduled workflow (cost S)
- Cron weekly plus `workflow_dispatch` plus the path-triggered `pull_request` event, pinned actions; steps as in the AC (`make ready`, build, one Playwright shard with `TEST_SERVER_BINARY` pointing at the make-built binary); the workflow has two jobs: `make-only` (a first step asserts `! command -v bazel`, so the make path cannot silently depend on it) and `pass-set-equality` (Bazel installed, `needs: make-only`, enabled only after the flip per `owner-attestations.yaml`).
- Files: `.github/workflows/fallback-canary.yml`
- Verify: `actionlint` exits 0; dispatched run is green.

##### Task 8.5b: Post-cutover pass-set equality (cost XS)
- Pre-mortem #3. After the Story 8.6 flip the `pass-set-equality` job (not the `make-only` job, which must stay Bazel-free) asserts the make-path `go test` pass set equals the Bazel pass set (reuse `test-set-diff.sh`).
- Files: `.github/workflows/fallback-canary.yml`
- Verify: `yq -e '.jobs."make-only".steps|map(.run // "")|any(test("test-set-diff"))|not' .github/workflows/fallback-canary.yml` prints `true` and `yq -e '.jobs."pass-set-equality".steps|map(.run // "")|any(test("test-set-diff"))' .github/workflows/fallback-canary.yml` prints `true`; `bash tools/bzl/parity/test-set-diff.sh default | grep -c 'missing_in_bazel: 0'` prints `1` in the canary run.

#### Story 8.6: Flip the default
**As the** owner, **I want** the default flipped in one revertible PR, **so that** cutover has a single point of rollback.
**Acceptance Criteria**:
- *Given* `PARITY: PASS`, the soak met, and owner sign-off recorded in `owner-attestations.yaml`, *When* the PR changes `BUILD_SYSTEM ?= make` to `bazel`, *Then* `make build`, `make test` and `make ready` pass in CI and `git revert <merge>` restores `make` without conflicts.
**Files**: `Makefile`, `project_plans/bazel-conversion/implementation/owner-attestations.yaml`

##### Task 8.6a: Flip PR (cost S) (blocked on owner sign-off)
- One-line default change plus doc updates.
- Files: `Makefile`, `docs/how-to/build-with-bazel.md`
- Verify: `make -n build | grep -c 'bazel build'` is at least `1`.

#### Story 8.7: Test summary instead of gotestsum under Bazel
**As an** agent, **I want** short readable test results, **so that** output tokens stay bounded.
**Acceptance Criteria**:
- *Given* a failing test under `BUILD_SYSTEM=bazel make test`, *When* the wrapper prints, *Then* output is the failing test names plus `bazel-testlogs/<pkg>/<target>/test.log` paths, at most 60 lines.
**Files**: `tools/bzl/testsummary/main.go`, `make/bazel.mk`

##### Task 8.7a: Wire summariser (cost S)
- Wrapper pipes `test.xml` through `testsummary`; keeps `gotestsum` for `BUILD_SYSTEM=make`. The summary prints `(N test targets cached; rerun with FORCE=1)` when results came from the cache.
- Files: `make/bazel.mk`, `tools/bzl/testsummary/main.go`
- Verify: `go test ./tools/bzl/testsummary -count=1` prints `ok`.

---

## Epic 9: install-service last

**Goal**: `make install-service` builds the release binary with Bazel, copies it to a stable path, signs and installs, without ever risking the live `:8543` service until P9 passes.

#### Story 9.1: Bazel-built install path
**As the** owner, **I want** the install pipeline to consume the Bazel binary safely, **so that** the live service cannot receive a subtly different binary.
**Acceptance Criteria**:
- Stable path.
  - *Given* `BUILD_SYSTEM=bazel make -n install-service`, *When* it prints, *Then* it shows `bazel build --config=release //:stapler-squad`, a `cp` to `./stapler-squad` (not `bazel-bin`), then the existing backup, sign and install steps; `ExecStart` never points into `bazel-*`.
- Post-build assertions.
  - *Given* the copied binary, *When* the pipeline runs, *Then* it asserts `version` output non-empty, embedded tmux/tymuxd checksums equal the pins, and aborts before restart on mismatch.
- Preflight intact.
  - *Given* `scripts/install-service-preflight.test.sh`, *When* run, *Then* exit 0 (the `--tmux-keep-server` guard unchanged).
**Files**: `Makefile`, `scripts/install-service.sh`, `scripts/install-service-preflight.test.sh`

##### Task 9.1a: Makefile install-service under BUILD_SYSTEM (cost M)
- Build, `cp`, then existing `install-service` steps; Linux and macOS branches (macOS signs the copy).
- Files: `Makefile`
- Verify: `out=$(BUILD_SYSTEM=bazel make -n install-service); for p in 'bazel build --config=release //:stapler-squad' 'cp .*\./stapler-squad' 'scripts/install-service.sh'; do printf '%s\n' "$out" | grep -qE "$p" || exit 1; done; ! printf '%s\n' "$out" | grep -qE 'ExecStart.*bazel-'` exits 0.

##### Task 9.1b: Post-build assertions in install-service.sh (cost S)
- Add the asserts before stopping the old unit. Pre-mortem #1: also set `OOMScoreAdjust=-900` in the deployed systemd unit (the preflight test asserts it). Pre-mortem #15 (macOS): assert `codesign -d -r-` of the signed copy equals the designated requirement recorded in `docs/how-to/macos-codesigning.md`, aborting before restart on mismatch.
- Files: `scripts/install-service.sh`
- Verify: `bash scripts/install-service-preflight.test.sh; echo $?` prints `0`; `shellcheck scripts/install-service.sh` exits 0.

##### Task 9.1c: Point docs at `tools/bzl/manual-instance` (the tool itself moved to Task 7.5e so the parity gate can pass before Epic 8) (cost XS)
- Make `docs/how-to/build-with-bazel.md` and the install runbook point at `go run ./tools/bzl/manual-instance`; add no code.
- Files: `docs/how-to/build-with-bazel.md`
- Verify: `test -f tools/bzl/manual-instance/main.go && grep -c manual-instance docs/how-to/build-with-bazel.md` prints at least `1`.

##### Task 9.1d: Owner-run live install (not run by agents) (cost XS)
- Document in `docs/how-to/build-with-bazel.md` the order: P9 pass, owner attestation, then `make install-service`; agents must not run it to test changes.
- Files: `docs/how-to/build-with-bazel.md`
- Verify: `grep -c 'install-service' docs/how-to/build-with-bazel.md` is at least `1`.

---

## Epic 10: make fallback retention, then removal

**Goal**: Keep the escape hatch for one release cycle (canary-tested), then delete the legacy recipes. Bazel-only source layout changes are banned until removal (so revert is mechanical).

#### Story 10.1: Retention window
**As the** owner, **I want** the fallback proven alive through a real release, **so that** rollback is real.
**Acceptance Criteria**:
- *Given* the flip merged, *When* all of these hold, *Then* retention is over and Task 10.2d may start: at least one release-please Release PR merged after the flip, at least 2 green scheduled canary runs since the flip, no `BUILD_SYSTEM=make` use needed, and the owner has run `make install-service` from the Bazel-built binary once on Linux and once on macOS (TCC check included) with both recorded in `owner-attestations.yaml`. (Pre-mortem #11: with about 18 commits a day a Release PR can merge quickly, so the Release PR alone is not enough evidence that the deploy path works.) Task 10.2a requires those attestations, checked by the environment-approved job of Task 7.5c.
**Files**: `project_plans/bazel-conversion/implementation/owner-attestations.yaml`

##### Task 10.1a: Retention guard (cost S)
- No judgment-based label check (a `bazel-only` label cannot be decided mechanically). The guard is the make path itself, run on PRs: the `legacy-*` jobs of Task 7.6a run unconditionally on every PR and `fallback-canary.yml` (Story 8.5) runs on risky-path PRs, both without `continue-on-error` for the whole retention window, so a source change that only works under Bazel layout fails a job. Documented in `.claude/rules/bazel-build-files.md`.
- Files: `.github/workflows/build.yml`, `.github/workflows/fallback-canary.yml`, `.claude/rules/bazel-build-files.md`
- Verify: `actionlint` exits 0; `grep -c 'continue-on-error' .github/workflows/fallback-canary.yml` prints `0`; a seeded Go file that compiles only with Bazel-style layout fails the `legacy-*` job in a draft PR.

#### Story 10.2: Removal
**As the** owner, **I want** the legacy recipes gone after the window, **so that** there is one build system.
**Acceptance Criteria**:
- *Given* retention attestation, *When* the removal PR merges, *Then* `grep -c 'ifeq (\$(BUILD_SYSTEM),make)' Makefile` prints `0`, stamp files `.proto-gen.stamp`/`.ent-gen.stamp` logic is gone, `fallback-canary.yml` and the `legacy-*` jobs are deleted, and `make build test ready` pass in CI.
**Files**: `Makefile`, `make/bazel.mk`, `.github/workflows/fallback-canary.yml`, `.github/workflows/build.yml`, `tools/ci/detect-feature-changes.sh`, `docs/**`, `CLAUDE.md`

##### Task 10.2a: Remove legacy recipes (cost M) (blocked on the Story 10.1 attestations and on Tasks 10.2d and 10.2c)
- Delete make-path bodies, stamp logic, `LDFLAGS` duplication; keep PermanentException targets and every target in `tools/bzl/removal-retained.txt` (the allow-list: `_codesign-binary`, `install-service`, `registry-generate`, `sync`, `gazelle`, `bazel-doctor`, the lint wrappers and the other ADR-005 rows, finalised from ADR-005 when the task starts).
- Order of operations: (1) the last make-path canary pass is the evidence recorded by Task 10.2d (the canary workflow and the `legacy-*` jobs are already deleted by Task 10.2c, so they cannot be re-run here); (2) snapshot `make -qp` target names to `$RUNNER_TEMP/before.targets`; (3) delete; (4) run `tools/bzl/check-removal.py`, which asserts set equality between (before minus after) and `tools/bzl/removal-expected-deleted.txt` (an unexpected deletion or an expected one still present both fail), that every name in `removal-retained.txt` still exists and has a non-empty `make -n <name>` plan, and that no remaining recipe line contains `go build` or `go test` except lines listed in the retained allow-list.
- Files: `Makefile`, `make/bazel.mk`, `tools/bzl/check-removal.py`, `tools/bzl/removal-expected-deleted.txt`, `tools/bzl/removal-retained.txt`
- Verify: `python3 tools/bzl/check-removal.py --before "$RUNNER_TEMP/before.targets"; echo $?` prints `0`; its seeded cases (extra deletion, retained target removed, a leftover `go test` line) each exit `1`; `make -n build test ready | grep -vFf tools/bzl/removal-retained.txt | grep -c 'go build\|go test'` prints `0`; `yq -e '.final_make_canary.run_url' project_plans/bazel-conversion/implementation/owner-attestations.yaml` prints a URL; `make build test ready` pass in CI under the default `BUILD_SYSTEM=bazel`.

##### Task 10.2d: Final make-path canary evidence (cost XS) (first task of Story 10.2)
- Dispatch `fallback-canary.yml` on the pre-removal commit (`BUILD_SYSTEM=make` `make build`, `make ready`, one e2e shard) and record `final_make_canary: {run_url, sha, date}` in `owner-attestations.yaml`. This must run before Task 10.2c deletes the workflow, and Task 10.2c must merge before Task 10.2a, so the removal PR can be green: the `legacy-*` jobs run the make path unconditionally (Task 7.6a) and would turn red the moment Task 10.2a deletes the make recipes.
- Files: `project_plans/bazel-conversion/implementation/owner-attestations.yaml`
- Verify: `gh run view "$(yq -r '.final_make_canary.run_url' project_plans/bazel-conversion/implementation/owner-attestations.yaml | sed 's#.*/##')" --json conclusion --jq .conclusion` prints `success`.
##### Task 10.2c: Retire the legacy selection logic and jobs (cost S)
- Delete `tools/ci/detect-feature-changes.sh`, the `build.yml:97` regex, the `legacy-*` jobs (they only existed as the make-path PR guard during the retention window) and `fallback-canary.yml` (moved here from Task 10.2b). This PR merges before Task 10.2a so that the recipe-removal PR has no job left that runs the make path; the next PR (10.2a) follows without a release in between.
- Files: `tools/ci/detect-feature-changes.sh`, `.github/workflows/build.yml`, `.github/workflows/fallback-canary.yml`
- Verify: `grep -rn 'detect-feature-changes' .github tools Makefile` prints nothing; `test ! -e .github/workflows/fallback-canary.yml` exits 0; `grep -c 'legacy-' .github/workflows/build.yml` prints `0`; `actionlint` exits 0.

##### Task 10.2b: Remove canary and update docs (cost S)
- Update `CLAUDE.md`, how-to pages and the skill after the recipes are gone (the canary workflow was deleted by Task 10.2c).
- Files: `CLAUDE.md`, `docs/how-to/build-with-bazel.md`, `.claude/skills/bazel-stapler-squad/SKILL.md`
- Verify: `grep -rn 'BUILD_SYSTEM=make' CLAUDE.md docs .claude | wc -l` prints `0`.

---

## Epic 11: Agent enablement (cross-cutting)

**Goal**: Agents work fluently in the Bazel world: one skill, one path-scoped rule, a short CLAUDE.md section, Diataxis docs under `docs/` (never `.claude/docs/`), worktree cleanup aware of Bazel output bases, merge-friendly lockfile, and guardrails that fail loudly with the remedy. Story 11.1 lands in the same wave as Story 1.1; Stories 11.2-11.8 land by the end of Epic 8.

#### Story 11.1: Skill, rule and CLAUDE.md section (lands with Epic 1)
**As an** agent, **I want** discoverable instructions before the first BUILD file appears, **so that** I run Gazelle instead of hand-editing.
**Acceptance Criteria**:
- Skill is discoverable.
  - *Given* `.claude/skills/bazel-stapler-squad/SKILL.md` with a description triggering on "bazel", "BUILD.bazel", "gazelle", "MODULE.bazel", "go_deps", *When* the skill list loads, *Then* the entry appears with that one-line description.
- Rule is path-scoped.
  - *Given* `.claude/rules/bazel-build-files.md` with `globs: ["**/BUILD.bazel","**/*.bzl","MODULE.bazel",".bazelrc"]`, *When* an agent edits `session/BUILD.bazel`, *Then* the rule loads and tells it Gazelle owns the file.
- CLAUDE.md stays short.
  - *Given* the new CLAUDE.md section, *When* counted, *Then* it is at most 15 lines and links to the skill and docs; index rows added.
**Files**: `.claude/skills/bazel-stapler-squad/SKILL.md`, `.claude/rules/bazel-build-files.md`, `CLAUDE.md`

##### Task 11.1a: Project skill (cost M)
- Per `stapler-squad-add-skill` conventions: command table (`ux.md` section 2), Gazelle rule, worktree rules, `--config=agent`, the error-state table pointer, links to global skills `bazel-version-upgrades`, `bazel-perf`, `bazel-github-actions-ci`, `bazel-angular`; mark every command as proposal until verified, then replace with verified output from Epics 1-3 before the PR merges. Include a section "Long builds under an agent Bash timeout" (use the harness background facility or `BAZEL_BG=1`, then poll `bash tools/bzl/bg-status.sh`; never re-run a build that is still running), a section "Reruns and filters" (`FORCE=1` is the old `-count=1`; `RUN=` is `--test_filter`, not `go test -run`, with the anchoring semantics recorded by Task 4.2b), and quote every error hint through `<!-- errors.yaml:E<n> -->` markers (Task 1.6a), never retyped.
- Files: `.claude/skills/bazel-stapler-squad/SKILL.md`
- Verify: `grep -c '^description:' .claude/skills/bazel-stapler-squad/SKILL.md` prints `1`; `bash tools/bzl/check-skill-proposals.sh; echo $?` prints `0` (every `bazel`/`make` command is in `tools/bzl/verified-commands.txt` or on a `<!-- proposal -->` line; Task 11.1d runs before this task).

##### Task 11.1b: Path-scoped rule (cost S)
- Rule: do not hand-edit Gazelle-managed rules; HandBuildFile header convention; no `.bazelrc` flag changes without reading the `bazel-perf` memory math; Bazel-only source changes banned until Epic 10.
- Files: `.claude/rules/bazel-build-files.md`
- Verify: `head -5 .claude/rules/bazel-build-files.md | grep -c globs` prints `1`.

##### Task 11.1c: CLAUDE.md section (cost S)
- At most 15 lines under Development Commands, under the heading `### Bazel (build of record)`: build of record statement, wrappers, `make gazelle` after imports, `make sync` for go tooling, never `bazel run //:stapler-squad` against `:8543`; add index rows for each doc page.
- Files: `CLAUDE.md`
- Verify: `test "$(awk '/^### Bazel \(build of record\)/{f=1;next} /^#{1,3} /{f=0} f&&NF' CLAUDE.md | wc -l)" -le 15; echo $?` prints `0`; `grep -c '^### Bazel (build of record)' CLAUDE.md` prints `1`.

##### Task 11.1d: Proposal-marking guard for the skill (cost S)
- `tools/bzl/check-skill-proposals.sh` fails CI when `.claude/skills/bazel-stapler-squad/SKILL.md` contains a `bazel` or `make` command line that is neither listed in `tools/bzl/verified-commands.txt` (appended by the PR that ran it, with the log link) nor on a line marked `<!-- proposal -->`. The dependency-add workflow section (go get, `make gazelle`, `bazel mod tidy`, `use_repo`) must keep the heading suffix `(proposal - not spike-verified)` until `spike-results.yaml` has a non-`Fail` S11 row and each of its commands is in `verified-commands.txt`; the check fails in both directions (marker missing while unverified, marker still present after verification). It also (a) takes `--docs <files>` and applies the same listed-or-marked rule to every fenced `bazel`/`make` command in those docs (Tasks 11.3a, 11.3d), (b) checks that every `<!-- errors.yaml:E<n> -->` quotation in the skill and docs equals the hint text in `errors.yaml`, and (c) fails when an `errors.yaml` `Remedy:` command is neither in `verified-commands.txt` nor on a row marked `status: proposal`, or when a `proposal` row's command has been verified (single source for translator text and the proposal guard).
- Files: `tools/bzl/check-skill-proposals.sh`, `tools/bzl/check-skill-proposals_test.sh`, `tools/bzl/verified-commands.txt`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/check-skill-proposals_test.sh` prints `PASS` for a marked unverified command and `FAIL` for an unmarked one, `FAIL` for an `errors.yaml` quotation that differs from the source and for a docs command that is neither listed nor marked; `bash tools/bzl/check-skill-proposals.sh; echo $?` prints `0` on the merged skill.

#### Story 11.2: Agent config, lockfile conflict handling, gazelle ergonomics
**As an** agent, **I want** quiet output and merge-safe lock handling, **so that** parallel PRs and context cost stay manageable.
**Acceptance Criteria**:
- *Given* `--config=agent` (presentation flags only, so it never forces an analysis-cache discard; its startup flags come from the wrapper, not the config), *When* `bazel test //config/...` runs, *Then* output has no ANSI sequences (`| grep -c $'\x1b'` prints `0`) and at most 40 lines on success.
- *Given* two branches with different dep additions, *When* merged with the `bazel-lock` merge driver, *Then* `bazel mod deps --lockfile_mode=error` exits 0 and the doc `docs/how-to/build-with-bazel.md` explains `git config merge.bazel-lock.driver` setup (also applied by `make setup-git`).
**Files**: `.bazelrc`, `.gitattributes`, `scripts/git-merge-bazel-lock.sh`, `Makefile`

##### Task 11.2a: Productionise merge driver (cost S)
- From Task 0.10b (Bazel's documented jq-based driver, exact text taken from the docs page recorded in `spike-results.md`, D19; `jq` 1.5+ becomes a documented prerequisite and is checked by `make bazel-doctor` check 9); extend the `setup-git` make target created in Task 0.2h (it already installs the pre-push hook and registers the Task 0.10b drivers) with idempotence, the `bazel-regen` driver of Task 7.8b and a `--check` mode used by `make bazel-doctor` check 9; note in `docs/how-to/build-with-bazel.md` that the `git config` entries are per clone and must be re-run after a fresh `git clone`; CI also regenerates the lock and fails on diff.
- Files: `.gitattributes`, `scripts/git-merge-bazel-lock.sh`, `Makefile`
- Verify: the two-branch simulation from Task 0.10b exits `0` for `bazel mod deps --lockfile_mode=error`.

##### Task 11.2b: Agent config validation (cost XS)
- Verify output size and no-ANSI claim; record in the reference page.
- Files: `docs/reference/bazel-targets-and-configs.md`
- Verify: `bazel test --config=agent //config/... 2>&1 | wc -l` is at most `40` when passing.

#### Story 11.3: Diataxis documentation
**As an** agent or the owner, **I want** how-to, reference and explanation pages, **so that** every error state has one place to point to.
**Acceptance Criteria**:
- *Given* the five pages exist, *When* `ls docs/how-to/build-with-bazel.md docs/how-to/set-up-bazel-worktree-caches.md docs/how-to/troubleshoot-bazel.md docs/reference/bazel-targets-and-configs.md docs/explanation/why-bazel.md` runs, *Then* all five are listed and none is under `.claude/docs/`.
- *Given* the troubleshooting page, *When* checked against `ux.md` section 5, *Then* all 11 error states appear with a reproduced symptom string (not recalled) and a remedy.
**Files**: the five pages above, `CLAUDE.md` index rows

##### Task 11.3a: how-to pages (cost M)
- `build-with-bazel.md` (build, test one package, add dependency, regenerate, manual instance, do-not-run-against-live), `set-up-bazel-worktree-caches.md` (shared caches, `user.bazelrc`, idle shutdown, optional per-worktree `--output_base` per Story 0.10).
- Files: `docs/how-to/build-with-bazel.md`, `docs/how-to/set-up-bazel-worktree-caches.md`
- Verify: `bash tools/bzl/check-skill-proposals.sh --docs docs/how-to/build-with-bazel.md docs/how-to/set-up-bazel-worktree-caches.md; echo $?` prints `0` (every fenced `bazel`/`make` command is in `tools/bzl/verified-commands.txt` with its run log link).

##### Task 11.3b: troubleshooting and reference (cost M)
- `troubleshoot-bazel.md` with the 11 error states reproduced during implementation; reference page lists configs, `TagConfig`s, make-to-bazel map, PermanentExceptions, forks register link.
- Extends the Epic 1 stub (Task 1.6e) to all 11 states without renaming its headings or anchors.
- Files: `docs/how-to/troubleshoot-bazel.md`, `docs/reference/bazel-targets-and-configs.md`
- Verify: `grep -c '^### ' docs/how-to/troubleshoot-bazel.md` is at least `11`; `git diff "$(git log --diff-filter=A --format=%H -1 -- docs/how-to/troubleshoot-bazel.md)" -- docs/how-to/troubleshoot-bazel.md | grep -c '^-### State'` prints `0` (the stub's commit is the one that added the file).

##### Task 11.3c: explanation page (cost S)
- `docs/explanation/why-bazel.md`: the three drivers, supersession of the ci-speed ADR (links), PermanentExceptions rationale.
- Files: `docs/explanation/why-bazel.md`
- Verify: `grep -c 'ADR-001' docs/explanation/why-bazel.md` is at least `2`.

##### Task 11.3d: "Fresh worktree, first build" journey doc (cost S)
- `docs/how-to/first-build-in-a-fresh-worktree.md` walks one journey end to end: create the worktree, run `make build`; the expected cold time (taken from `baselines.md` and the Story 0.10 / `bazel-worktree-cost.sh` measurements, with the date and run count, and the T3 target beside it); what the queue looks like when other Bazel runs hold the slots (the E7 messages, ETA, 900 s timeout, exit 75); where state lands (the `bazel-bin`, `bazel-out`, `bazel-testlogs` symlinks in the worktree root, all gitignored; the output base under `~/.cache/bazel/_bazel_$USER/<hash>`; shared disk and repository caches; `.bazel-logs/`); when `make sync` is needed (plain `go`/gopls only); and the first command to run when something looks wrong (`make bazel-doctor`).
- Files: `docs/how-to/first-build-in-a-fresh-worktree.md`
- Verify: `bash tools/bzl/check-skill-proposals.sh --docs docs/how-to/first-build-in-a-fresh-worktree.md; echo $?` prints `0`; `grep -c "$(jq -r '.cold_worktree_wall_s|floor' tools/bzl/baseline/measured.json)" docs/how-to/first-build-in-a-fresh-worktree.md` prints at least `1` (the quoted cold time is the recorded median); `grep -cE 'baselines.md|bazel-worktree-cost' docs/how-to/first-build-in-a-fresh-worktree.md` is at least `1`.

#### Story 11.4: worktree-disk-cleanup knows Bazel output bases
**As the** owner, **I want** removing a worktree to reclaim its Bazel state, **so that** the 148-worktree/178 GB class of incident does not recur with Bazel output bases.
**Acceptance Criteria**:
- *Given* output bases under `~/.cache/bazel/_bazel_$USER/*/` whose `DO_NOT_BUILD_HERE` file names a workspace path that no longer exists, *When* the skill's new script runs in dry-run mode, *Then* it lists those bases with sizes and never lists a base whose workspace exists.
- *Given* `--apply`, *When* run, *Then* orphan bases are removed and the shared disk and repository caches are untouched; `bazel shutdown` cannot be used because the workspace directory is gone, so if `<base>/server/server.pid.txt` names a live process whose `/proc/<pid>/cmdline` contains `--output_base=<base>`, that process is killed first (`[RUN]` both files exist; the kill procedure itself is `[INF]`), and the base is removed only after the process is gone.
- *Given* `_bazel_$USER/install/*` directories, *When* the script runs, *Then* it only reports those older than 30 days (Bazel's own `--experimental_install_base_gc_max_age`, default 30d, collects them; the script must not duplicate that).
- *Given* the owner-run install of Task 11.4b, *When* `systemctl --user is-enabled bazel-orphan-outputbases.timer` runs, *Then* it prints `enabled`; *Given* `du` of `~/.cache/bazel` above 150 GB or `df` of the cache path above 85%, *When* `bash tools/bzl/bazel-disk-alarm.sh` runs, *Then* it exits 1 naming the path.
**Files**: `.claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh`, `.claude/skills/worktree-disk-cleanup/bazel.md` (or `SKILL.md`, see the pre-check), `scripts/systemd/*`, `tools/bzl/bazel-disk-alarm.sh`

##### Task 11.4a: Orphan output base script and skill section (cost S) (lands with Epic 1, before the first E5 hint can fire)
- Script `bazel-orphan-outputbases.sh`. The default mode is a dry run that removes nothing and lists: orphan output bases (`DO_NOT_BUILD_HERE` names a workspace that no longer exists; VERIFIED by run that this file holds exactly the workspace path with no trailing newline, so the orphan test is valid; bases that live inside a worktree as `.bazel-out` vanish with `git worktree remove` and are never orphans), their live servers (`server/server.pid.txt` plus the `--output_base=` match, D16), install bases older than 30 days (report only, D17), the shared disk cache size against its GC cap, and stale test temp directories (`/tmp/ssq-*` and Bazel test tmp dirs older than 24 h with no open handles per `fuser`), each with its size. `--apply` removes exactly those entries, never a base whose workspace exists and never the shared disk or repository cache directories; `--tmp` adds the temp-directory class to `--apply`. The error hint E5 (Task 1.6a) names the dry run first.
- **Owner's uncommitted work (hard pre-check).** At plan time `.claude/skills/worktree-disk-cleanup/SKILL.md` is modified and `worktree-liveness.sh` and `worktree-salvage.sh` are untracked (`git status --short .claude/skills/worktree-disk-cleanup`); they are the owner's. First step of the task: `sha256sum .claude/skills/worktree-disk-cleanup/SKILL.md .claude/skills/worktree-disk-cleanup/worktree-liveness.sh .claude/skills/worktree-disk-cleanup/worktree-salvage.sh > "$TMPDIR/owner-files.sha"`. Never stash, reset, `git add` or overwrite them. If `SKILL.md` still has uncommitted changes, write the Bazel section to the new file `.claude/skills/worktree-disk-cleanup/bazel.md` and add the one-line link to `SKILL.md` only after the owner has committed; otherwise append the section to `SKILL.md`.
- Files: `.claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh`, `.claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases_test.sh`, `.claude/skills/worktree-disk-cleanup/bazel.md` (or `SKILL.md`)
- Verify: `bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases_test.sh` exits 0 (fixtures: an orphan base is listed, a live workspace's base never is, a stale temp directory is listed, `--apply` leaves a live base and both caches untouched, a dry run changes nothing, an orphan base with a live stub server process matching `--output_base=<base>` has that process killed before removal and an unrelated process with the same pid file content is not killed); `shellcheck .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh` exits 0; `sha256sum -c "$TMPDIR/owner-files.sha"` exits 0 (when the `bazel.md` path was used, all three files are checked; when the section was appended to `SKILL.md`, only the two scripts).

##### Task 11.4b: Daily orphan timer and unit (owner-run install) (cost S)
- User systemd units `bazel-orphan-outputbases.service` (runs the script with `--apply --tmp` under `nice -n 10`) and `bazel-orphan-outputbases.timer` (daily, `Persistent=true`) under `scripts/systemd/`, an owner-run `scripts/systemd/install-bazel-timers.sh`, and the install commands in `docs/how-to/set-up-bazel-worktree-caches.md`. Agents write the files; the owner runs `systemctl --user enable --now` (wall-clock blocker).
- Files: `scripts/systemd/bazel-orphan-outputbases.service`, `scripts/systemd/bazel-orphan-outputbases.timer`, `scripts/systemd/install-bazel-timers.sh`, `docs/how-to/set-up-bazel-worktree-caches.md`
- Verify: `systemd-analyze verify --user scripts/systemd/bazel-orphan-outputbases.service scripts/systemd/bazel-orphan-outputbases.timer` exits 0; `shellcheck scripts/systemd/install-bazel-timers.sh` exits 0; after the owner installs it, `systemctl --user is-enabled bazel-orphan-outputbases.timer` prints `enabled` (owner action).

##### Task 11.4c: Disk alarm and trend metrics (cost S)
- `tools/bzl/bazel-disk-alarm.sh` exits 1 and prints `ALARM: <path> <du|df> <value>` when `du -s ~/.cache/bazel` exceeds 150 GB, the shared disk cache exceeds its 100 GB cap (the built-in GC of Task 0.2b enforces the cap only while a server is idle for the 10 s delay and trims to well below it, so an alarm means GC is not running and the standalone `diskcache:gc` fallback of Task 0.10f applies), or `df` of the cache path or of the `bazel-remote` data directory exceeds 85%. The service of Task 11.4b runs it as `ExecStartPost`, and `bazel-trend.sh` adds `bazel_disk_alarm` and the three gauges to its output so `ci-duration-trend.yml` annotates a breach.
- Files: `tools/bzl/bazel-disk-alarm.sh`, `tools/bzl/bazel-disk-alarm_test.sh`, `tools/ci/bazel-trend.sh`, `.github/workflows/ci-duration-trend.yml`, `scripts/systemd/bazel-orphan-outputbases.service`
- Verify: `bash tools/bzl/bazel-disk-alarm_test.sh` exits 0 (stubbed `du`/`df` exit 1 above each threshold and 0 below); `bash tools/ci/bazel-trend_test.sh | grep -c bazel_disk_alarm` prints at least `1`.

#### Story 11.5: Go and gopls keep working
**As an** agent, **I want** `go build`, `go vet` and gopls to work after cutover, **so that** muscle memory is not a failure mode.
**Acceptance Criteria**:
- *Given* a fresh clone after `make sync`, *When* `go build ./... && go vet ./session/...` runs, *Then* both exit 0 (already covered by Story 2.4; this story documents it and the optional `GOPACKAGESDRIVER` setup in the how-to and decides it is unsupported by default).
- *Given* the how-to, *When* read, *Then* it states the post-cutover status verbatim: Bazel is the build of record; native `go build`/`go vet`/`go test` and gopls are supported for editor tooling, golangci-lint, quick local iteration and bisecting after `make sync`, and a native `go test` result is never authoritative (see "Post-cutover status" above and the ADR-002 amendment).
**Files**: `docs/how-to/build-with-bazel.md`

##### Task 11.5a: Document go/gopls workflow and decision (cost XS)
- Note: `make sync` is the supported path; `gopackagesdriver` is optional and unsupported. State the post-cutover status of native Go tooling (supported for gopls and quick loops, Bazel is the build of record, native results not authoritative) and the inverted precondition (Bazel needs no gen; plain go/gopls needs `make sync`).
- Files: `docs/how-to/build-with-bazel.md`, `decisions/ADR-002-generated-code-bazel-only-with-sync-gen.md`
- Verify: `grep -c 'make sync' docs/how-to/build-with-bazel.md` is at least `1`; `grep -c 'build of record' docs/how-to/build-with-bazel.md` is at least `1`; `grep -c 'Post-cutover status' decisions/ADR-002-generated-code-bazel-only-with-sync-gen.md` prints `1`.

#### Story 11.6: Reviewability of BUILD changes
**As the** owner, **I want** generated churn collapsed and hand-written Bazel reviewed carefully, **so that** review stays cheap.
**Acceptance Criteria**:
- *Given* `.gitattributes` and `.github/CODEOWNERS` (created in Task 0.2e, extended here), *When* a PR touches `MODULE.bazel`, `.bazelrc`, `tools/bzl/**` or any `.bzl`, *Then* the owner is requested; Gazelle-managed `**/BUILD.bazel` carry `linguist-generated` **except** the root `/BUILD.bazel` and every path in `tools/bzl/hand-build-files.txt` (they hold `# gazelle:` directives or hand-written rules and must always render in the diff); `.github/CODEOWNERS` covers `/BUILD.bazel`; the PR template has the line "BUILD changes: generated by `make gazelle` (yes/no); hand-written Bazel changed (list)".
- *Given* a PR changing `go.mod`, `MODULE.bazel` or `MODULE.bazel.lock`, *When* `bazel.yml` runs, *Then* a sticky PR comment and a `::notice` annotation list the added, removed and upgraded Go modules and the `bazel_dep` and `use_repo` changes, so an external-dependency change cannot hide inside a collapsed lock diff.
**Files**: `.gitattributes`, `.github/CODEOWNERS`, `.github/pull_request_template.md`, `tools/bzl/annotate-deps.sh`

##### Task 11.6a: Extend CODEOWNERS, add template and attributes (cost S)
- `.gitattributes`: `**/BUILD.bazel linguist-generated=true` followed by negations `/BUILD.bazel -linguist-generated` and one `-linguist-generated` line per `hand-build-files.txt` entry (generated from the list by `make gazelle`'s post-step so the two cannot drift). CODEOWNERS adds `/BUILD.bazel`.
- Files: `.github/CODEOWNERS`, `.github/pull_request_template.md`, `.gitattributes`
- Verify: `gh api repos/tstapler/stapler-squad/codeowners/errors --jq '.errors|length'` prints `0`; `git check-attr linguist-generated BUILD.bazel session/BUILD.bazel tools/bzl/tmux/BUILD.bazel` prints `unset` for the first and third and `set` for the second.

##### Task 11.6b: Gazelle-directive change visibility check (cost S)
- `tools/bzl/check-gazelle-directives.sh` lists every added or removed `# gazelle:` line in a PR diff (any BUILD file) and fails with `gazelle directive changed: <file>:<line>; owner review required` unless the owner is a reviewer or the PR carries the label `gazelle-directive-reviewed`; the list is also posted as a PR annotation so a directive change cannot hide behind a collapsed generated diff.
- Files: `tools/bzl/check-gazelle-directives.sh`, `tools/bzl/check-gazelle-directives_test.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/check-gazelle-directives_test.sh` prints `PASS` for a diff with no directive change and `FAIL` (exit 1) for one adding `# gazelle:resolve`, even when only the root `BUILD.bazel` changed.

##### Task 11.6c: PR annotation for external-dependency and module-file changes (cost S)
- `tools/bzl/annotate-deps.sh <base> <head>` diffs `go.mod`, `MODULE.bazel` and `MODULE.bazel.lock` and prints, per category, `added`, `removed`, `upgraded (old -> new)` for Go modules (`require` lines), `bazel_dep` lines and `use_repo` names, or `no dependency change`. `bazel.yml` posts it as a `::notice` annotation and updates one sticky PR comment on every PR that touches those files (read-only token; the comment step is skipped for fork PRs).
- Files: `tools/bzl/annotate-deps.sh`, `tools/bzl/annotate-deps_test.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/annotate-deps_test.sh` exits 0 (fixtures: one added module, one upgrade, one removed `bazel_dep`, and a lock-only change that prints `no dependency change`); `actionlint .github/workflows/bazel.yml` exits 0.

#### Story 11.7: Starlark surface cap and buildifier
**As the** owner, **I want** repo-owned Starlark bounded, **so that** custom build code stays reviewable (ADR-008).
**Acceptance Criteria**:
- *Given* `tools/bzl/check-starlark-size.sh`, *When* repo-owned `.bzl` and macro lines exceed 500, *Then* CI fails naming the largest files; `buildifier --lint=warn` runs on all Bazel files.
**Files**: `tools/bzl/check-starlark-size.sh`, `.github/workflows/bazel.yml`

##### Task 11.7a: Size check and buildifier step (cost S)
- Files: `tools/bzl/check-starlark-size.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/check-starlark-size.sh; echo $?` prints `0`; `buildifier -lint=warn -r . | wc -l` prints `0`.

#### Story 11.8: Version pinning and upgrade canary
**As the** owner, **I want** Bazel and rules upgrades to arrive as a tested PR, **so that** version churn (ci-speed ADR scar `b51b60eb1`) is detected before it breaks main.
**Acceptance Criteria**:
- *Given* a scheduled workflow, *When* it runs, *Then* it tries the latest Bazel 9.x patch and latest rules_go/gazelle on a branch, runs the subset build, and opens an issue with the first failing step if it fails; the runbook is `docs/how-to/upgrade-bazel.md` pointing at the `bazel-version-upgrades` skill.
**Files**: `.github/workflows/bazel-upgrade-canary.yml`, `docs/how-to/upgrade-bazel.md`

##### Task 11.8a: Canary workflow and runbook (cost S)
- Phase 4 additions: the canary also runs the pinned macOS job (pre-mortem #9); checks that `go.mod`'s `go`/`toolchain` line is listed in `tools/bzl/supported-go.txt` kept beside `global-inputs.txt`, with a path-scoped rule requiring the `bazel-version-upgrades` skill for `go.mod` toolchain edits (pre-mortem #10); and checks every fork's ADR-008 rule 4 removal condition and age on each scheduled run (pre-mortem #8).
- Files: `.github/workflows/bazel-upgrade-canary.yml`, `docs/how-to/upgrade-bazel.md`, `tools/bzl/supported-go.txt`, `tools/bzl/check-go-toolchain.sh`, `tools/bzl/check-fork-age.sh`, `.claude/rules/bazel-go-toolchain.md`, `docs/reference/bazel-forks.md`
- Verify: `actionlint .github/workflows/bazel-upgrade-canary.yml` exits 0; `bash tools/bzl/check-go-toolchain.sh; echo $?` prints `0` (the `go.mod` `go`/`toolchain` line is in `supported-go.txt`) and exits `1` on a seeded unsupported version; `bash tools/bzl/check-fork-age.sh; echo $?` prints `0` with no forks and `1` on a seeded fork row past its removal condition; `head -5 .claude/rules/bazel-go-toolchain.md | grep -c globs` prints `1`.

---

## Epic 12: Expensive tests in the Bazel graph (selective Playwright e2e and the other slow suites; ADR-009)

**Why this epic exists**: the measured RA2 share is 0.987 (`baselines.md` line "RA2 non-Go share of the critical path": e2e median 1974 s of a 2601 s median span), so Epics 1-11 alone cannot reach T1/T2. The owner directive (2026-11-02) is that only the targets whose inputs changed re-run their tests, e2e included. **Dependency position**: Epic 12 starts after Story 3.4 (root binary), Stories 4.1-4.2 (test env helpers, `go_test` macro), Story 7.3 (`affected-targets.sh`, the `GlobalInputs` file) and Story 7.4 (read-only PR cache, `main`-only writes); it needs Story 0.15 (S14) to have passed at Story 0.14. Stories 12.1-12.5 run in shadow beside the full suite; Story 12.6 (blocking) is gated by the Story 7.6 soak and by `e2e_escapes = 0`; Task 10.2c must not delete the legacy full e2e job until Story 12.6 has merged. Task 7.6b (Bazel binary into the legacy runner) stays and is the first step; Epic 12 then replaces the unconditional full run on PRs.

#### Story 12.1: Observed-dependency manifest
**As the** owner, **I want** each spec's real dependencies measured instead of hand-declared, **so that** the slice is complete for what the spec executes and the core packages appear in every slice automatically.
**Acceptance Criteria**:
- Every spec has an entry.
  - *Given* `tests/e2e/slice-manifest.json` generated from a coverage run, *When* `python3 tools/bzl/e2e/check-manifest.py` runs, *Then* it exits 0 only if every `tests/e2e/**/*.spec.ts` appears exactly once with a non-empty `go_pkgs` list (or `"no_server": true` with a reason), `routes`, `web_files`, `rpcs` and a `generated_at` commit, and lists nothing as `unmanaged`.
- Deterministic.
  - *Given* two manifest generations from the same commit, *When* diffed, *Then* the Go package lists are identical (a package is included if any statement in it executed, plus all packages with an `init` that ran) and web/route lists differ only by documented timing-dependent entries, which the generator unions over 2 runs.
**Files**: `tools/bzl/e2e/*`, `tests/e2e/slice-manifest.json`, `tests/e2e/helpers/test-server.ts` (env passthrough only)

##### Task 12.1a: Coverage-instrumented server target and per-spec capture (cost S)
- Add a `manual`-tagged target `//:stapler-squad_cover` built with `-cover -coverpkg=./...` (rules_go `gotags`/`gc_goopts` per spike S7 results; fall back to `go build -cover` on the synced tree if rules_go cannot pass `-coverpkg`, recorded in the spike row). `tools/bzl/e2e/capture.sh <spec>` runs one Playwright invocation with `--workers=1` for that spec file, `TEST_SERVER_BINARY` set to the cover binary and `GOCOVERDIR=<dir>` (one server per invocation means per-spec attribution without a Go source change), then `go tool covdata pkglist -i=<dir>`.
- Files: `tools/bzl/e2e/capture.sh`, `BUILD.bazel`, `tools/bzl/e2e/capture_test.sh`
- Verify: `bash tools/bzl/e2e/capture_test.sh; echo $?` prints `0` (the test captures `alias.spec.ts` and asserts the package list contains `github.com/tstapler/stapler-squad/session` and `.../server/services`).

##### Task 12.1b: Web and RPC capture (cost M)
- Extend the existing route-visit preload (`tests/e2e/baseline/route-visit-preload.js`, the precedent for per-spec observation) with Chromium JS coverage (`page.coverage.startJSCoverage`; Chromium only, INFERRED from the Playwright API docs, checked in Story 0.15) resolved through the bundle source maps to `web-app/src/**` files, plus the RPC method names seen in the server access log. Firefox/WebKit-only specs get `web_files` from the route closure of the bundler metafile instead.
- Files: `tests/e2e/helpers/coverage-preload.js`, `tools/bzl/e2e/resolve-sourcemaps.mjs`, `tools/bzl/e2e/resolve_sourcemaps_test.sh`
- Verify: `bash tools/bzl/e2e/resolve_sourcemaps_test.sh; echo $?` prints `0` (fixture bundle plus source map resolves to the expected three source files).

##### Task 12.1c: Manifest generator, schema and checker (cost S)
- `tools/bzl/e2e/gen-manifest.py` merges captures (union over 2 runs), writes `tests/e2e/slice-manifest.json` sorted; `check-manifest.py` implements the AC; schema in `tools/bzl/e2e/manifest.schema.json`.
- Files: `tools/bzl/e2e/gen-manifest.py`, `tools/bzl/e2e/check-manifest.py`, `tools/bzl/e2e/manifest.schema.json`, `tools/bzl/e2e/gen_manifest_test.py`
- Verify: `python3 -m unittest tools/bzl/e2e/gen_manifest_test.py; echo $?` prints `0` (cases: missing spec, duplicate spec, empty `go_pkgs`, unsorted output all rejected).

##### Task 12.1d: Nightly regeneration and staleness check (cost S)
- Add a job to `nightly.yml` that regenerates the manifest on `main` and opens a PR if it differs; add a PR check that fails when a changed or added spec has no manifest entry or when its `generated_at` is older than the spec's last change (cheap form, no capture).
- Files: `.github/workflows/nightly.yml`, `tools/bzl/e2e/check-staleness.sh`
- Verify: `actionlint .github/workflows/nightly.yml; echo $?` prints `0`; `bash tools/bzl/e2e/check-staleness.sh --fixture tests/e2e/testdata/stale; echo $?` prints `1`.

#### Story 12.2: Slice targets in the Bazel graph
**As the** owner, **I want** each spec group to be a Bazel test whose declared inputs are its measured slice, **so that** `rdeps` and the test cache both operate on real dependencies.
**Acceptance Criteria**:
- Targets exist and exclude the binary.
  - *Given* the generated `tests/e2e/BUILD.bazel`, *When* `bazel query 'deps(//tests/e2e:all)' | grep -cE '^//(:stapler-squad|server/web:dist)'` runs, *Then* it prints `0`, and `bazel query 'kind(e2e_test, //tests/e2e:all)' | wc -l` is at least 20 and at most 60.
- Slice sensitivity.
  - *Given* a manifest-listed Go file of spec group G, *When* `bazel query "rdeps(//tests/e2e:all, //session:instance.go)"` runs (file label), *Then* G's target is in the output; and for a file outside every slice (a `docs/` file) the output is empty.
**Files**: `tests/e2e/BUILD.bazel` (generated), `tools/bzl/e2e/e2e.bzl`, `tools/bzl/e2e/gen-build.py`

##### Task 12.2a: e2e_slice macro and BUILD generator (cost M)
- `tools/bzl/e2e/e2e.bzl` (at most 30 lines: `e2e_slice` = `filegroup` over web files and protos plus `deps` on observed `go_library` labels resolved through Gazelle's `go_library` naming) and `gen-build.py` that groups specs by filename family and emits one `e2e_test` per group from the manifest; marked `HandBuildFile`-style generated header, `# gazelle:ignore`.
- Files: `tools/bzl/e2e/e2e.bzl`, `tools/bzl/e2e/gen-build.py`, `tests/e2e/BUILD.bazel`, `tools/bzl/hand-build-files.txt`
- Verify: `python3 tools/bzl/e2e/gen-build.py --check; echo $?` prints `0` (generated file equals committed); `bazel query //tests/e2e:all 2>&1 | grep -c 'ERROR'` prints `0`.

##### Task 12.2b: e2e_test runner with comparator verdict (cost M)
- `e2e_test` runs `npx playwright test <spec files> --reporter=json` with the Playwright image tag and `tests/e2e/bun.lock` as declared inputs, then applies the per-target comparator (`scripts/e2e-baseline-compare.ts` logic against the baseline and expected-diff lists) so the Bazel verdict equals today's `main` verdict (ADR-009 item 5). Tags: `requires-network` not set (loopback only), `block-network` set, `exclusive` not set; `TEST_TMPDIR` short path for unix sockets (S9 result); `shard_count` unset (grouping, not sharding, controls size). Quarantined specs get `no-cache`.
- Files: `tools/bzl/e2e/e2e.bzl`, `tools/bzl/e2e/run-playwright.sh`, `tools/bzl/e2e/compare-target.ts`, `tools/bzl/e2e/run_playwright_test.sh`
- Verify: `bash tools/bzl/e2e/run_playwright_test.sh; echo $?` prints `0` (fixture report with one baseline-listed failure passes; with one new failure fails).

##### Task 12.2c: GlobalInputs list for e2e (cost XS)
- `tools/bzl/e2e-global-inputs.txt` (go.mod, go.sum, MODULE.bazel*, .bazelrc*, `proto/**`, `server/web/embed.go`, `tests/e2e/{playwright.config.ts,global-setup.ts,helpers/**,fixtures/**,bun.lock}`, `tools/bzl/e2e/**`) becomes `//tests/e2e:global_inputs` (a `filegroup` every target lists) and is appended to the Story 7.3 `GlobalInputs` handling.
- Files: `tools/bzl/e2e-global-inputs.txt`, `tests/e2e/BUILD.bazel`
- Verify: `bash tools/bzl/e2e/check-global-inputs.sh; echo $?` prints `0` (every path in the list exists or matches a glob, and every target lists the group).

#### Story 12.3: The out-of-band binary seam
**As the** owner, **I want** the binary kept out of the declared graph and still provably fresh, **so that** a change outside a slice does not re-run the spec and a stale binary cannot be tested.
**Acceptance Criteria**:
- *Given* a worktree where only `server/services/<unobserved>.go` changed, *When* `bazel test //tests/e2e:all --config=e2e` runs after a previous passing run, *Then* the summary shows only targets whose slice contains the file as re-run, and all others `(cached)`.
- *Given* the binary at `TEST_SERVER_BINARY` was built from a different commit than `HEAD`, *When* `tools/bazel test //tests/e2e:all --config=e2e` starts, *Then* the wrapper fails in under 5 s with `Remedy: rebuild the e2e binary (bazel build //:stapler-squad)`.
**Files**: `.bazelrc`, `tools/bazel`, `tools/bzl/e2e/check-no-binary-dep.sh`

##### Task 12.3a: `--config=e2e` and the revision guard (cost S)
- `.bazelrc` `test:e2e` sets the value-less forms `--test_env=TEST_SERVER_BINARY`, `--test_env=TEST_SEED_DEMO`, `--test_env=TEST_SEED_HIDDEN`, `--test_env=TEST_SSHD_BINARY` (inherit the client environment; constant paths, so they do not change the key) and `--cache_test_results=auto`. The revision guard lives in the `tools/bazel` wrapper, outside the key and outside the sandbox: for `bazel test ... --config=e2e` it runs `bazel build //:stapler-squad` first, exports the four paths from `bazel cquery --output=files`, and compares `"$TEST_SERVER_BINARY" --version` with `git rev-parse HEAD` (the revision can not be a `--test_env` because a per-commit value would change every key). Flag names are checked by `check-bazelrc.sh` (Task 0.2g).
- Files: `.bazelrc`, `tools/bazel`, `tools/bzl/e2e/wrapper_e2e_test.sh`
- Verify: `bash tools/bzl/check-bazelrc.sh; echo $?` prints `0`; `bash tools/bzl/e2e/wrapper_e2e_test.sh; echo $?` prints `0` (the test asserts the wrapper exits non-zero with `Remedy:` when the binary revision differs from `HEAD`, and exports the four variables otherwise).

##### Task 12.3b: Binary-dependency lint (cost XS)
- `tools/bzl/e2e/check-no-binary-dep.sh` fails if `bazel query 'deps(//tests/e2e:all)'` contains `//:stapler-squad`, `//server/web:dist` or an embed-staging target; runs in `bazel.yml`.
- Files: `tools/bzl/e2e/check-no-binary-dep.sh`, `.github/workflows/bazel.yml`
- Verify: `bash tools/bzl/e2e/check-no-binary-dep.sh; echo $?` prints `0`; `bash tools/bzl/e2e/check-no-binary-dep.sh --seed-dep; echo $?` prints `1`.

#### Story 12.4: Selection, shard planning and the shadow job
**As the** owner, **I want** PRs to run only the affected targets spread evenly over the runners, **so that** e2e minutes fall without lengthening the critical path.
**Acceptance Criteria**:
- *Given* a PR diff, *When* `tools/ci/affected-e2e.sh <base> <head>` runs, *Then* it prints the selected target labels one per line, or `__ALL__` if a `GlobalInputs` file changed, plus every `unmanaged` target; a deleted file selects the targets that listed it via the base-side query.
- *Given* the selected set and `tests/e2e/durations.json` (from the last `main` report), *When* `tools/ci/plan-e2e-shards.py --shards 5` runs, *Then* the longest shard is at most 1.25 x the mean shard.
- *Given* the shadow job on a PR, *When* it finishes, *Then* the PR comment lists selected / total targets and E2E_MINUTES_AVOIDED (selected-shard-seconds vs the last full run), and the job is non-blocking.
**Files**: `tools/ci/affected-e2e.sh`, `tools/ci/plan-e2e-shards.py`, `.github/workflows/bazel-e2e.yml`, `tools/bzl/parity/e2e-avoided.sh`

##### Task 12.4a: affected-e2e.sh with fixtures (cost S)
- Reuses the Story 7.3 harness (`--diff-filter=d`, base-side query for deletions); adds the `__ALL__` and `unmanaged` rules; fixtures cover: one core file, one leaf file, a `proto/` change, a deleted file, a spec added without manifest entry, a docs-only change (empty output).
- Files: `tools/ci/affected-e2e.sh`, `tools/ci/affected-e2e_test.sh`
- Verify: `bash tools/ci/affected-e2e_test.sh; echo $?` prints `0`.

##### Task 12.4b: Duration-balanced shard planner (cost S)
- `plan-e2e-shards.py`: longest-processing-time greedy over per-target durations from the merged Playwright report of the last green `main` run; unknown targets get the median; emits the matrix JSON that replaces `E2E_SHARD_TOTAL`/`--shard`.
- Files: `tools/ci/plan-e2e-shards.py`, `tools/ci/plan_e2e_shards_test.py`
- Verify: `python3 -m unittest tools/ci/plan_e2e_shards_test.py; echo $?` prints `0` (property: max shard <= 1.25 x mean on the fixture; every target assigned exactly once).

##### Task 12.4c: Shadow workflow `bazel-e2e.yml` (cost M)
- Jobs: `plan` (affected-e2e + shard plan), `bin` (one `bazel build //:stapler-squad` plus seed tools, uploaded), `shard` matrix (Playwright container image from `e2e-env`, `bazel test <targets> --config=ci --config=e2e`, read-only cache on PRs), `report` (merges reports, runs the comparator, posts the comment). Runs beside the legacy full e2e job with `continue-on-error: true`; on `main` it runs all targets with `--nocache_test_results` and the cache write endpoint.
- Files: `.github/workflows/bazel-e2e.yml`
- Verify: `actionlint .github/workflows/bazel-e2e.yml; echo $?` prints `0`; `grep -c 'continue-on-error: true' .github/workflows/bazel-e2e.yml` is at least `1` until Task 12.6a.

##### Task 12.4d: T8 collector (cost S)
- `tools/bzl/parity/e2e-avoided.sh` computes e2e runner-minutes per partial-change PR (sum of shard job durations from `gh api .../jobs`) for the 30 sampled partial PRs of Task 0.1i, Bazel-selective vs the baseline full run, and `e2e_escapes` from the escape ledger; prints `T8: PASS|FAIL|UNMEASURED` (wired into `improvement.sh`, Task 7.5g).
- Files: `tools/bzl/parity/e2e-avoided.sh`, `tools/bzl/parity/improvement.sh`
- Verify: `bash tools/bzl/parity/e2e-avoided.sh --fixture tools/bzl/parity/testdata/t8-pass.json | grep -c '^T8: PASS$'` prints `1`.

#### Story 12.5: Soundness controls
**As the** owner, **I want** a miss to be detected and fixed within a day, **so that** skipping tests never silently hides a regression.
**Acceptance Criteria**:
- *Given* `main` at any commit, *When* the nightly full run fails a spec, *Then* `tools/ci/e2e-escape.py` finds the PR that last touched the failing spec's slice, reports whether the spec was selected there, and if not appends `{spec, pr, missing_edge}` to `tests/e2e/escapes.json` and files an issue; `e2e_escapes` is the count of unselected-failure rows.
- *Given* `tests/e2e/quarantine.json`, *When* `python3 tools/bzl/e2e/check-quarantine.py` runs, *Then* it exits 0 only if each row has an owner, an issue and an expiry within 30 days of its date, and no `flaky = True` or `--flaky_test_attempts` appears in the e2e targets.
**Files**: `tools/ci/e2e-escape.py`, `tests/e2e/quarantine.json`, `tests/e2e/escapes.json`, `tools/bzl/e2e/*`

##### Task 12.5a: Completeness gate (cost S)
- `tools/bzl/e2e/check-completeness.py`: every spec has a manifest entry; every `@feature` id and `FEATURE_CATALOG` reference resolves or is on `tests/e2e/feature-gaps.json` (the current 106 unresolved ids seed the file and the file may only shrink); every registry feature with a `handlerFile` appears in at least one observed slice or is listed as `untested`. Wired into `registry-generate`'s CI check and `bazel.yml`.
- Files: `tools/bzl/e2e/check-completeness.py`, `tests/e2e/feature-gaps.json`, `tools/bzl/e2e/check_completeness_test.py`
- Verify: `python3 tools/bzl/e2e/check-completeness.py; echo $?` prints `0`; `python3 -m unittest tools/bzl/e2e/check_completeness_test.py` exits 0 (seeded missing-entry and grown-gap fixtures fail).

##### Task 12.5b: Nightly full run and escape detector (cost M)
- `nightly.yml` job `e2e-full-bazel`: `bazel test //tests/e2e:all --nocache_test_results --runs_per_test=3` on `main` (the e2e extension of Task 4.9c); `e2e-escape.py` as in the AC; the job result is the comparator verdict and a failure files one issue per spec.
- Files: `.github/workflows/nightly.yml`, `tools/ci/e2e-escape.py`, `tools/ci/e2e_escape_test.py`
- Verify: `python3 -m unittest tools/ci/e2e_escape_test.py; echo $?` prints `0`; `actionlint .github/workflows/nightly.yml; echo $?` prints `0`.

##### Task 12.5c: Mutation backtest harness as a standing check (cost S)
- Promote the Story 0.15 harness to `tools/bzl/e2e/mutation-backtest.sh` (seeded mutations from `tools/bzl/e2e/mutations.yaml`, each with a patch, run the full suite and the selected set, assert every mutation that fails a spec in the full run fails one in the selected set); scheduled monthly and on any change to `tools/bzl/e2e/**`.
- Files: `tools/bzl/e2e/mutation-backtest.sh`, `tools/bzl/e2e/mutations.yaml`, `.github/workflows/bazel-e2e.yml`
- Verify: `bash tools/bzl/e2e/mutation-backtest.sh --dry-run | grep -c '^MUTATION '` is at least `30`.

##### Task 12.5d: Quarantine policy and cache-poisoning check (cost XS)
- `check-quarantine.py` per the AC; plus a workflow lint that fails if any step with the `bazel-cache-write` credential runs for a `pull_request` event or if `bazel-e2e.yml` passes `--remote_upload_local_results` outside the `main` job (ADR-004 control reused for the new workflow).
- Files: `tools/bzl/e2e/check-quarantine.py`, `tools/bzl/check-workflow-cache-writes.sh`
- Verify: `python3 tools/bzl/e2e/check-quarantine.py; echo $?` prints `0`; `bash tools/bzl/check-workflow-cache-writes.sh; echo $?` prints `0`.

#### Story 12.6: Make selective e2e the PR gate
**As the** owner, **I want** the full PR suite replaced by the selected set once escapes are zero, **so that** the minutes saved are real.
**Acceptance Criteria**:
- *Given* the soak ledger (Story 7.6) and `tests/e2e/escapes.json`, *When* the last N shadow runs (the same N as Story 7.6) show `e2e_escapes = 0` and the T8 collector prints `PASS`, *Then* Task 12.6a may merge; the nightly and `main`-push full runs stay blocking-in-effect (failures open issues and block the next release-please merge by convention).
**Files**: `.github/workflows/build.yml`, `.github/workflows/bazel-e2e.yml`

##### Task 12.6a: Flip PR e2e to selected targets (cost S) (blocked on soak and `e2e_escapes = 0`)
- On `pull_request`, `build.yml`'s `e2e-bun-shard` matrix is replaced by the `bazel-e2e.yml` planner's matrix (selected targets only); `continue-on-error` removed from `bazel-e2e.yml`; the legacy full job runs on `main` push and `workflow_dispatch` only. The owner records the flip in `owner-attestations.yaml`.
- Files: `.github/workflows/build.yml`, `.github/workflows/bazel-e2e.yml`
- Verify: `actionlint .github/workflows/build.yml .github/workflows/bazel-e2e.yml; echo $?` prints `0`; `grep -c 'continue-on-error: true' .github/workflows/bazel-e2e.yml` prints `0`.

#### Story 12.7: The other slow suites
**As the** owner, **I want** the remaining expensive suites to run only when their inputs changed, **so that** the directive holds beyond Playwright.
**Acceptance Criteria**:
- *Given* the inventory `docs/reference/bazel-slow-suites.md` (per-suite p50 from `measured.json`: `build.yml` 2230 s, `lint.yml` 1033 s, `mcp-integration.yml` 590 s, plus the Jest, `ux-analysis.yml` axe/Lighthouse and benchmark jobs), *When* `bash tools/bzl/e2e/check-slow-suites.sh` runs, *Then* each suite is listed with one of: Bazel test selected by `rdeps` (Go integration, Jest), selected by the Epic 12 manifest (axe sweeps that are Playwright specs), or a recorded reason it stays full (lint, benchmarks, visual-regression, RealClaude harness), and the script exits 0 only if no suite over 300 s p50 is unclassified.
**Files**: `docs/reference/bazel-slow-suites.md`, `tools/bzl/e2e/check-slow-suites.sh`, `.github/workflows/mcp-integration.yml`, `.github/workflows/ux-analysis.yml`

##### Task 12.7a: Slow-suite inventory and rdeps wiring (cost M)
- Write the inventory from the measured timings; wire `mcp-integration.yml` and `ux-analysis.yml` to `affected-targets.sh` / the Epic 12 selector instead of their `paths:` filters (the filters stay as a cheap pre-gate where the 7.3c inventory says so); Jest targets are the Story 3.x/4.x Bazel tests, selected by `rdeps` like Go tests.
- Files: `docs/reference/bazel-slow-suites.md`, `tools/bzl/e2e/check-slow-suites.sh`, `.github/workflows/mcp-integration.yml`, `.github/workflows/ux-analysis.yml`
- Verify: `bash tools/bzl/e2e/check-slow-suites.sh; echo $?` prints `0`; `actionlint .github/workflows/mcp-integration.yml .github/workflows/ux-analysis.yml; echo $?` prints `0`.

---

## Epic 13: Package scoping refactors (behaviour-preserving moves; ADR-010)

**Why this epic exists**: the owner asked (2026-11-02) for refactors that make the large packages smaller and more scoped, because under Bazel and the ADR-009 slices a package is the unit of invalidation and `session/`, `server/services` and `config/` may sit in nearly every slice. The measurements (`research/package-scoping.md`, ADR-010) say what such refactors can and cannot buy: moving 15 inverted test files lowers the mean affected test-LOC share from 55.4% to 44.9%; the four Tier A and five Tier B clusters that pass the ADR-010 selection criteria take `server/services` from 93,118 to 67,445 LOC and `session` from 92,248 to 68,883 LOC and lower the mean affected library-LOC share only from 40.5% to 38.5%; the other eleven mechanical clusters move no measured number and are not split. **Dependency position**: starts after Stories 1.1 (Gazelle BUILD files, so a split is `git mv` plus `bazel run //:gazelle`) and 1.2; Stories 13.1 and 13.2 can run beside Epics 2 to 11. Stories 13.4 and 13.5 additionally wait for the Story 0.14 S14 verdict and the Story 12.1 manifest (criterion SC5 data) and for the owner's confirmation of the "ADR-010" Unresolved Questions. **Not gated by this epic**: the CI cutover (Story 7.6), the `BUILD_SYSTEM` flip (Story 8.6), Epic 10 and Epic 12 do not wait for it; T9 is reported from Task 13.1f on and enforced only at Story 13.7. **Tier C** (`Instance`, `EntRepository`, `Storage`, which need interface extraction) is deliberately not planned here (ADR-010 Decision 3). Every split is one cluster per PR, `git mv` plus the Task 13.1d codemod, landed within one working session after the Task 13.1e conflict check.

#### Story 13.1: Replay harness, T9 baseline and move tooling
**As the** owner, **I want** the split decisions made by a reproducible model of the real import graph, **so that** each move is chosen by its predicted payoff and the prediction is checked against the result.
**Acceptance Criteria**:
- The model reproduces the known graph.
  - *Given* the current tree, *When* `python3 tools/bzl/scoping/graph.py --identity-check` runs, *Then* it prints JSON with `"diffs": 0` (the graph rebuilt from file-level type-checked uses equals the `go list` Imports/TestImports/XTestImports graph for all non-generated packages), and `python3 tools/bzl/scoping/replay.py --model merged --pr-sample tools/bzl/baseline/pr-sample.json --json | jq -e '.go_touching_median > 0.25 and .go_touching_median < 0.28'` exits 0 (reproduces the RA1 value 0.2657 recorded in `tools/bzl/baseline/measured.json`).
- The baseline is committed and the target model is the T1 reference.
  - *Given* `tools/bzl/scoping/baseline.json`, *When* `jq -e '.t9a_test_loc_share_500 > 0.5 and .t9e_target_model_share <= 0.30' tools/bzl/scoping/baseline.json` runs, *Then* it exits 0.
- The planner refuses a bad split.
  - *Given* a cluster map that creates an import cycle, *When* `plan-split.py` runs, *Then* it exits 1 and names the two files and the symbol; for a cluster with more than 10 relocated declarations it reports `sc1_ok: false`.
**Files**: `tools/bzl/scoping/*`, `tools/bzl/scoping/clusters/*.json`, `tools/bzl/parity/scoping.sh`, `tools/bzl/baseline/targets.yaml`

##### Task 13.1a: File-level edge extractor and graph model (cost M)
- Port `research/scoping-tools/typed/main.go` to `tools/bzl/scoping/fileedges/main.go` (go/packages with `Tests: true`, one record per use: using file, declaring file, symbol, exported, kind) and `research/scoping-tools/{load,sim,sim2}.py` to `tools/bzl/scoping/graph.py`. The graph keeps library and test edges separate (target model: a test target is a leaf) and offers the merged model only as `--model merged` for continuity with RA1. `--identity-check` rebuilds the package graph from file edges and diffs it against `go list -e -json ./...`. The extractor must not need generated code beyond `make sync` (it runs on a synced tree) and must finish in under 120 s on the CI runner.
- Files: `tools/bzl/scoping/fileedges/main.go`, `tools/bzl/scoping/graph.py`, `tools/bzl/scoping/graph_test.py`, `tools/bzl/scoping/testdata/mini/**` (a 6-package fixture module with one library edge, one test-only back-edge and one method-bound receiver)
- Verify: `python3 -m unittest tools/bzl/scoping/graph_test.py; echo $?` prints `0` (cases: a test-only import does not enlarge the dependents of the tested package in the target model; the merged model does; identity rebuild of the fixture has 0 diffs); `python3 tools/bzl/scoping/graph.py --identity-check | jq -e '.diffs==0'; echo $?` prints `0`.

##### Task 13.1b: Replay metrics and the T9 baseline (cost S)
- `tools/bzl/scoping/replay.py` replays the last N `main` commits (default 500 and 100) and the PR sample: affected libraries by reverse library edges, affected test targets by the target rule of research section 1, and computes T9a to T9e (mean affected test-LOC share, universal-slice touch share over the set `session`, `server/services`, `server`, `config`, root, mean affected library-LOC share and the share of changes with lower affected LOC than a stored baseline, largest-package sizes plus test inversions, T1 on the target model). Writes `tools/bzl/scoping/baseline.json` once, with the commit SHA and N. JSON keys (the names the Verify lines and Story ACs use): `n_changes`, `go_touching_median` (merged model only), `t9a_test_loc_share_500`, `t9b_universal_touch_100`, `t9b_universal_touch_500`, `t9c_mean_lib_loc_share`, `t9c_changes_with_lower_lib_loc`, `t9d_services_loc`, `t9d_session_loc`, `t9d_test_inversions`, `t9e_target_model_share`.
- Files: `tools/bzl/scoping/replay.py`, `tools/bzl/scoping/replay_test.py`, `tools/bzl/scoping/baseline.json`
- Verify: `python3 -m unittest tools/bzl/scoping/replay_test.py; echo $?` prints `0`; `python3 tools/bzl/scoping/replay.py --commits 500 --json | jq -e '.t9a_test_loc_share_500>0 and .t9d_test_inversions>=0 and (.n_changes>0)'; echo $?` prints `0`.

##### Task 13.1c: Split planner and predictor (cost M)
- `tools/bzl/scoping/plan-split.py --pkg <dir> --map <cluster.json> [--json]`: reads a cluster map (file list, new package path), rebuilds the graph with the files reassigned (importers re-pointed through the declaring file of each used symbol; test files assigned by `x_test.go` to `x.go`, else by majority of references), reports cycles, the declarations to relocate (inbound symbols from the rest of the package), unexported symbols to capitalise, importer files to rewrite, straddling test files, the predicted T9 deltas and the verdict on SC1 to SC6 (JSON keys: `cycles`, `relocated_declarations`, `unexported_to_capitalise`, `importer_files`, `straddling_tests`, `new_interfaces`, `predicted`, `sc1_ok` to `sc6_ok`) (SC5 reads Task 13.1g's output when present). Cluster maps for the nine clusters that pass SC6 today are committed from `research/package-scoping-clusters.json` under `tools/bzl/scoping/clusters/`.
- Files: `tools/bzl/scoping/plan-split.py`, `tools/bzl/scoping/plan_split_test.py`, `tools/bzl/scoping/clusters/*.json`
- Verify: `python3 -m unittest tools/bzl/scoping/plan_split_test.py; echo $?` prints `0` (cases: seeded cycle exits 1; 11 relocated declarations gives `sc1_ok` false); `python3 tools/bzl/scoping/plan-split.py --pkg server/services --map tools/bzl/scoping/clusters/services-wsbridge.json --json | jq -e '.sc1_ok and (.cycles|length)==0 and .sc6_ok'; echo $?` prints `0`.

##### Task 13.1d: Move codemod (cost M)
- `tools/bzl/scoping/move.py <cluster.json> [--dry-run]`: `git mv` the cluster's files and their test files into the new directory, rewrite the package clause, qualify references to symbols that stay behind (`oldpkg.Sym`), capitalise the unexported symbols that cross the boundary using `gopls rename` (version pinned in `tools/bzl/scoping/gopls.version`; if `gopls` is not on `PATH` the script prints `Remedy: go install golang.org/x/tools/gopls@<pinned>` and exits 2), rewrite importers' qualifiers and imports with `golang.org/x/tools/go/ast/astutil`, run `gofmt -w` on touched files, and stop before committing. It never edits a file outside the cluster, its tests and its importers.
- Files: `tools/bzl/scoping/move.py`, `tools/bzl/scoping/move_test.sh`
- Verify: `bash tools/bzl/scoping/move_test.sh; echo $?` prints `0` (on the fixture module: after the move `go build ./... && go vet ./... && go test ./...` pass, the moved package compiles on its own, one boundary symbol was capitalised, and `git status --porcelain` shows only renames plus modified importers).

##### Task 13.1e: Conflict check and announcement (cost S)
- `tools/bzl/scoping/conflicts.sh <cluster.json>` lists, for the cluster's files and their importers: open PRs (`gh pr list --state open --json number,files`), branches of every registered worktree that touch them (`git worktree list --porcelain` then `git diff --name-only origin/main...<branch>`), and `main` commits in the last 24 hours. Exit 0 only if nothing is open or the owner passed `--accept-open`. It appends a dated line to `project_plans/bazel-conversion/implementation/scoping-ledger.md` (cluster, files, open touches, predicted and measured T9 deltas) and prints the text of the backlog notice the agent posts (`create_backlog_item` or `post_backlog_update`) before the PR is opened.
- Files: `tools/bzl/scoping/conflicts.sh`, `tools/bzl/scoping/conflicts_test.sh`, `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `bash tools/bzl/scoping/conflicts_test.sh; echo $?` prints `0` (fixture repo with two worktrees: one branch touching a cluster file is reported and exits 1; a clean fixture exits 0).

##### Task 13.1f: T9 targets and collector (cost S)
- Add the T9 rows (formula over `baseline.json`, proposal values from ADR-010) to `tools/bzl/baseline/targets.yaml` at the owner's confirmation; `tools/bzl/parity/scoping.sh` prints `T9a` to `T9f` as `PASS|FAIL|UNMEASURED` (T9f is `UNMEASURED` until the Story 12.1 manifest exists; since the 2026-11-05 owner decision "Full e2e run" on ADR-009, `scoping t9` prints `T9f: N/A` instead, read from `adr_009_e2e` in `owner-attestations.yaml`, because Epic 12 selection is not adopted). It is a separate script, so the `improvement.sh` line count of 8 (Task 7.5g) is unchanged; `run-all.sh` calls both and the parity report prints both. `UNMEASURED` is not a failure before Story 13.7.
- Files: `tools/bzl/parity/scoping.sh`, `tools/bzl/parity/testdata/t9-pass.json`, `tools/bzl/parity/testdata/t9-fail.json`, `tools/bzl/baseline/targets.yaml`
- Verify: `bash tools/bzl/parity/scoping.sh --fixture tools/bzl/parity/testdata/t9-pass.json | grep -cE '^T9[a-f]: PASS$'; true` prints `6`; `bash tools/bzl/parity/scoping.sh --fixture tools/bzl/parity/testdata/t9-fail.json; echo $?` prints `1`.

##### Task 13.1g: Slice-dilution report for SC5 (cost S)
- `tools/bzl/scoping/dilution.py` reads `tests/e2e/slice-manifest.json` (Story 12.1) and, per candidate cluster, prints the share of spec slices that contain its packages and the median covered-file share; prints `SC5: UNAVAILABLE (no manifest)` and exits 0 when the manifest is absent, so SC4 is used instead. Also writes the baseline T9f value (median slice LOC per spec) when the manifest exists.
- Files: `tools/bzl/scoping/dilution.py`, `tools/bzl/scoping/dilution_test.py`
- Verify: `python3 -m unittest tools/bzl/scoping/dilution_test.py; echo $?` prints `0` (fixture manifest of 4 specs gives the expected shares; absent manifest prints the UNAVAILABLE line).

#### Story 13.2: Break test-only layering inversions (Tier 0)
**As the** owner, **I want** the 15 test files that make a lower layer's tests import a higher layer moved out, **so that** a change in `server/services`, `server/auth` or `pkg/events` stops re-running `session`'s 362-file test target and the other lower-layer test targets.
**Acceptance Criteria**:
- Inversions are gone and cannot return.
  - *Given* the tree after the moves, *When* `bash tools/bzl/scoping/check-test-inversions.sh` runs, *Then* it exits 0 (it was 12 package pairs in 6 packages, 15 files), and it exits 1 on a seeded fixture.
- No test is lost.
  - *Given* `go test -list '.*' ./session/... ./server/services/... ./server/deliverygate/... ./session/git/... ./session/gitprovider/... ./session/scanstate/... | grep -c '^Test'` before and after, *Then* the counts are equal, and the Story 4.8a test-set diff reports `missing_in_bazel: 0` once Epic 4 exists.
- The predicted payoff holds.
  - *Given* `replay.py --commits 500`, *When* it runs after Task 13.2b, *Then* `t9a_test_loc_share_500 <= 0.46` (predicted 0.449) and `t9e_target_model_share <= 0.30`.
**Files**: `session/inttest/**`, `server/deliverygate/inttest/**`, `server/services/inttest/**`, `session/git/backend/inttest/**`, `session/gitprovider/inttest/**`, `session/scanstate/inttest/**`, `tools/bzl/scoping/check-test-inversions.sh`, `Makefile`

##### Task 13.2a: Move the `session` and `server/deliverygate` inverted tests (cost S)
- `git mv` the five `session/*_test.go` files that import `server/services`, `server/auth` or `pkg/events` (`claim_gossip_convergence_test.go`, `claim_two_host_integration_test.go`, `ent_repository_backlog_events_test.go`, `host_advertisement_convergence_test.go`, `storage_backlog_events_test.go`) into `session/inttest/` and the three `server/deliverygate/{crash_matrix,matrix_helpers,matrix}_test.go` into `server/deliverygate/inttest/`; change `package session_test` to `package inttest` (a directory holding only `_test.go` files). They are external test packages and reference no unexported symbol (VERIFIED, research section 3); any helper they share with the files left behind is duplicated into the new directory only if `go vet` demands it, else imported from `testutil`.
- Files: `session/inttest/*_test.go`, `server/deliverygate/inttest/*_test.go`
- Verify: `go vet ./session/... ./server/deliverygate/... && go test ./session/inttest/... ./server/deliverygate/inttest/... -count=1; echo $?` prints `0`.

##### Task 13.2b: Move the remaining inverted tests (cost S)
- Same move for `server/services/{guidance_request_ownership_parity,watch_sessions_native_streaming_integration}_test.go`, `session/git/backend/{routed_cli,shadow_gogit_broken_worktree,spawn}_test.go`, `session/gitprovider/gitprovider_test.go` and `session/scanstate/store_test.go`, each into an `inttest/` directory next to its package.
- Files: `server/services/inttest/*_test.go`, `session/git/backend/inttest/*_test.go`, `session/gitprovider/inttest/*_test.go`, `session/scanstate/inttest/*_test.go`
- Verify: `go vet ./server/services/... ./session/git/... ./session/gitprovider/... ./session/scanstate/... && go test ./server/services/inttest/... ./session/git/backend/inttest/... ./session/gitprovider/inttest/... ./session/scanstate/inttest/... -count=1; echo $?` prints `0`; `bash tools/bzl/scoping/check-test-inversions.sh; echo $?` prints `0`.

##### Task 13.2c: Inversion guard in `make ready` (cost S)
- `tools/bzl/scoping/check-test-inversions.sh` (uses `go list -e -f` only, no Python, so it runs in `make ready` on a runner without Bazel) fails when any package's tests import a package that depends on it; add it as `ready-test-inversions` in `make ready` and the `lint` workflow, and tighten the comment in `.golangci.yml` `no_server_in_core` that currently exempts `_test.go` so it points at the new check. Update `deps_guard_test.go`'s header comment to say the layering guard lives in the script.
- Files: `tools/bzl/scoping/check-test-inversions.sh`, `tools/bzl/scoping/check_test_inversions_test.sh`, `Makefile`, `.golangci.yml`, `deps_guard_test.go`
- Verify: `bash tools/bzl/scoping/check_test_inversions_test.sh; echo $?` prints `0` (a seeded fixture inversion exits 1, the clean tree exits 0); `make -n ready | grep -c ready-test-inversions` prints `1`.

#### Story 13.3: Tier A clusters that pass SC6
**As the** owner, **I want** the mechanical clusters whose split changes a measured number moved into their own packages, **so that** a core change stops invalidating the independent ones and a change inside them stops invalidating the core.
**Acceptance Criteria**:
- Behaviour preserved.
  - *Given* each moved cluster, *When* `go build ./... && go vet ./... && go test` for the moved package and every importing package runs, *Then* it exits 0, the Story 4.8a test-set diff shows no missing test, and `bazel run //:gazelle` leaves no diff once Story 1.1 exists.
- Falsifiable payoff (selection criterion SC6 re-run at execution time by Task 13.1c; the numbers are for the four clusters together).
  - *Given* `replay.py --commits 500` after Tasks 13.3a to 13.3c, *Then* `server/services` is at most 89,000 LOC and `session` at most 91,500 LOC (predicted 88,481 and 91,061), at least 11% of replayed changes affect less library LOC than baseline (predicted 12.2%), `t9e_target_model_share <= 0.30`, and the measured values are within 1 percentage point (lib LOC share) of the prediction recorded in `scoping-ledger.md`.
- A cluster that no longer passes SC1 to SC6 at execution time is recorded as `skipped by criterion` in the ledger with its numbers, not moved.
**Files**: `server/services/wsbridge/**`, `server/services/sessionbundle/**`, `server/services/devtools/**`, `session/external/**`, importers under `server/**`, `.golangci.yml`

##### Task 13.3a: Move `server/services/wsbridge` (12 files, 3,315 LOC; cost M)
- Run Tasks 13.1e (conflict check) then 13.1d on `tools/bzl/scoping/clusters/services-wsbridge.json` (`ws_*` files). Six unexported symbols cross the boundary and are capitalised; 4 test files straddle (7 importer files in `server` to rewrite, 10 test files move). Update any depguard glob that names the old path in the same PR.
- Files: `server/services/wsbridge/**`, `server/services/ws_*.go` (moved), `server/*.go` (importers)
- Verify: `go build ./... && go vet ./server/... && go test ./server/services/... ./server/services/wsbridge/... ./server -count=1; echo $?` prints `0`; `python3 tools/bzl/scoping/replay.py --commits 500 --json | jq -e '.t9c_changes_with_lower_lib_loc >= 0.005'; echo $?` prints `0`.

##### Task 13.3b: Move the two independent services clusters `sessionbundle` and `devtools` (cost S)
- Same procedure for `session_bundle_*`/`disk_space_unix.go` (3 files, 836 LOC) and `cdp_stream_handler.go`, `inspect_element_handler.go`, `vnc_proxy_handler.go` (3 files, 486 LOC). Both have no import edge to the rest of the package, so no symbol is capitalised and each importer is a single file in `server`. These pass SC6 through the "no longer invalidated by core changes" clause (11.6% of replayed changes), not through their own churn.
- Files: `server/services/sessionbundle/**`, `server/services/devtools/**`, `server/*.go` (importers)
- Verify: `go build ./... && go vet ./server/... && go test ./server/services/sessionbundle/... ./server/services/devtools/... ./server -count=1; echo $?` prints `0`; `python3 tools/bzl/scoping/replay.py --commits 500 --json | jq -e '.t9c_changes_with_lower_lib_loc >= 0.11'; echo $?` prints `0`.

##### Task 13.3c: Move `session/external` (3 files, 1,215 LOC; cost S)
- `external_approval.go`, `external_discovery.go`, `external_tmux_streamer.go` into `session/external/`; two unexported `session` symbols are capitalised; 14 importer files in `server` and `server/services` are rewritten. Moved code reads `Instance` state only through `Snapshot()` or exported accessors (`.claude/rules/instance-lock-free-reads.md`); the compiler enforces it at the new boundary.
- Files: `session/external/**`, `server/**` (importers)
- Verify: `go build ./... && go vet ./session/... ./server/... && go test ./session/external/... ./session -count=1; echo $?` prints `0`.

##### Task 13.3d: Re-evaluate the other Tier A clusters and record the result (cost XS)
- Run `plan-split.py` for the eleven Tier A clusters that failed SC6 on 2026-11-02 (`insights`, `gatestats`, `webhooks`, `hookctx`, `backlogplugin`, `importer`, `handoff`, `paneprompt`, `pipelinemode`, `sweeper`, `prpoller`) against the then-current tree and, when available, the Task 13.1g SC5 data; append each verdict to `scoping-ledger.md`; open a Task for any that now passes (a new Task id under this Story, same procedure as 13.3a).
- Files: `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `grep -c '^| tierA-reeval |' project_plans/bazel-conversion/implementation/scoping-ledger.md` prints `11`.

#### Story 13.4: Tier B in `server/services` (near-mechanical, relocation budget 10)
**As the** owner, **I want** the `BacklogService` group and the websocket transport group moved out of the 93k-LOC package, **so that** the package that is in nearly every e2e slice is a third smaller and its two most-changed groups stop sharing an invalidation unit with everything else.
**Acceptance Criteria**:
- Boundary computed, not guessed.
  - *Given* `plan-split.py` on the two component maps, *Then* `cycles` is empty and `sc1_ok` is true; the `BacklogService` group needs 10 relocated declarations and 4 capitalised symbols, the websocket group 27 and 18 (VERIFIED 2026-11-02, research section 4).
- Falsifiable payoff.
  - *Given* `replay.py --commits 500` after both moves, *Then* `server/services` is at most 73,000 LOC (93,118 minus 15,651 minus 5,385 is 72,082; Story 13.3 lowers it further), the mean affected test-LOC share is lower than before the story by at least 1.5 percentage points (the two components alone: 1.88 plus 0.76 at their marginal values, 2.6 predicted combined), and `t9e_target_model_share <= 0.30`.
- Design rules held.
  - *Given* the PR diff, *Then* it adds no interface with a single implementation and no consumer (each relocated declaration already existed), the staged `server/dependencies.go` builder keeps its call order, and `grep -c 'github.com/tstapler/stapler-squad/server/services/backlogsvc' session/*.go` prints `0` (the session core never imports an upper package).
**Files**: `server/services/backlogsvc/**`, `server/services/wsconn/**`, `server/services/*.go` (moved and relocated), `server/dependencies.go`, `server/*.go`, `.golangci.yml`

##### Task 13.4a: Confirm the design and open-work check for both components (cost S)
- For the components `server/services` p0 (`backlog_service*.go`, `backlog_sources_preview.go`, `backlog_transition_notify.go`, `github_user_*.go`) and p2 (`connectrpc_websocket*.go`, `guidance_ack_detector.go`, `notification_quality_metrics.go`, `stream_hub_rollout_service.go`, `websocket_transport.go`): write their cluster maps (the exact relocation lists come from `plan-split.py`), check `project_plans/backlog-service-refactor` and `project_plans/core-domain-decomposition` for open PRs on the same files (Task 13.1e), and record in the ledger that each relocated declaration is an existing interface, callback type or constant moved to a new file in the core (`interface-pollution-checklist`: no new interface is introduced).
- Files: `tools/bzl/scoping/clusters/services-backlogsvc.json`, `tools/bzl/scoping/clusters/services-wsconn.json`, `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `python3 tools/bzl/scoping/plan-split.py --pkg server/services --map tools/bzl/scoping/clusters/services-backlogsvc.json --json | jq -e '.sc1_ok and (.cycles|length)==0 and .relocated_declarations<=10'; echo $?` prints `0`; the same for `services-wsconn.json` with `.relocated_declarations<=27`.

##### Task 13.4b: Relocate the 10 declarations, then move `BacklogService` (28 files, 15,651 LOC; cost M)
- PR 1 (no package change): move the 10 declarations the rest of the package uses (for example `AutonomousDriverStarter`, `SessionSteerer`, `WithTransitionCaller`, `redactErrorDetail`) from `backlog_service*.go` into a new `server/services/backlog_ports.go`, and replace the core's use of the concrete `*BacklogService` type by the existing interface. PR 2: move the group to `server/services/backlogsvc/` with Task 13.1d, capitalise the 4 unexported symbols, update `server/dependencies.go` and `server/server.go` wiring (imports only), depguard globs and the feature-registry markers (`make registry-generate`, commit the changed files). Respect `.claude/rules/norawghrequest.md` and `noarchivedrevival.md`: the moved files keep their `//nolint` annotations and the analyzer file lists in those rules are updated if they name a moved path.
- Files: `server/services/backlog_ports.go`, `server/services/backlogsvc/**`, `server/dependencies.go`, `server/server.go`, `docs/registry/features/*.json`
- Verify: `go build ./... && go vet ./server/... && go test ./server/services/... ./server/services/backlogsvc/... ./server -count=1; echo $?` prints `0`; `make registry-generate && git diff --quiet docs/registry; echo $?` prints `0`; `make lint-custom; echo $?` prints `0`.

##### Task 13.4c: Relocate 27 declarations, then move the websocket transport group (6 files, 5,385 LOC; cost M)
- Same two-PR procedure for `connectrpc_websocket*.go`, `websocket_transport.go`, `guidance_ack_detector.go`, `notification_quality_metrics.go`, `stream_hub_rollout_service.go` into `server/services/wsconn/`; 18 unexported symbols are capitalised. The unary-batching allow-list (`ws_unary_allowlist.go`) stays in the core unless the planner shows it in the cluster. Check `docs/reference/unary-request-batching.md` and `docs/reference/terminal-ws-pool.md` for file paths and update them in the same PR.
- Files: `server/services/wsconn/**`, `server/services/ws_ports.go`, `docs/reference/unary-request-batching.md`, `docs/reference/terminal-ws-pool.md`
- Verify: `go build ./... && go vet ./server/... && go test ./server/services/... ./server/services/wsconn/... ./server -count=1; echo $?` prints `0`; `grep -rn 'server/services/connectrpc_websocket' docs | wc -l` prints `0`.

##### Task 13.4d: Re-evaluate the remaining services components (cost S)
- Run the planner on the four other peeled components (p1 guidance/wake/notification helpers 7 files; p3 5 files; p4 2 files; p5 2 files) and the Task 13.1g SC5 data; append verdicts to the ledger; any that passes gets a Task here. Record the measured `server/services` size and T9a to T9d in the ledger.
- Files: `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `grep -c '^| tierB-services-reeval |' project_plans/bazel-conversion/implementation/scoping-ledger.md` prints `4`.

#### Story 13.5: Tier B in `session` (design adopted from the 2026-09-07 audits)
**As the** owner, **I want** the backlog/workflow group and the agent-adapter/review-queue group moved out of `session`, **so that** the package every other package depends on shrinks by a quarter and the hottest file in it (`backlog_lifecycle.go`, 94 revisions) leaves its invalidation unit.
**Acceptance Criteria**:
- The design is the existing one.
  - *Given* `docs/architecture-audit-session-package-split-2026-09-07.md` and `docs/architecture-audit-session-backlog-extraction-design-2026-09-07.md`, *When* Task 13.5a completes, *Then* the ledger lists, for each component, which section of those documents it follows and every place it deviates (none expected: the measured p0 component is the `session/backlog` target, with the workflow and liveness engines the audit left open, and p1 is the `session/review` target plus the agent adapters), and `plan-split.py` reports no cycle.
- Falsifiable payoff.
  - *Given* Stories 13.3 and 13.4 merged and `replay.py --commits 500` after Task 13.5e, *Then* `session` is at most 72,000 LOC (92,248 minus 14,681 minus 6,903 minus 594 is 70,070; 68,883 with Story 13.3), mean affected library-LOC share is at most 39.5% (predicted 38.5%), at least 12% of replayed changes affect less library LOC (predicted 14.8%), the universal-slice touch share over the last 500 is at most 61% (predicted 58%), and `t9e_target_model_share <= 0.30`. Without Story 13.3 and 13.4 merged, the library-LOC bound is 39.8% (predicted 39.3% for the session components alone).
- Instance reads stay safe.
  - *Given* `.claude/rules/instance-lock-free-reads.md`, *Then* no moved file reads a raw `Instance` field (the new package has no access to unexported fields, so `go build` proves it) and no moved file adds a new `i.mu.RLock()`; `make actor-field-guard` and `make lint-custom` pass.
**Files**: `session/backlog/**`, `session/agents/**`, `session/startup/**`, `session/*.go` (moved and relocated), `server/**` (importers), `.golangci.yml`

##### Task 13.5a: Adopt the audited design and produce the per-component relocation lists (cost M)
- For the three passing components (`session` p0: `backlog_*.go` except `backlog_item_id.go`, `chain_firer.go`, `ci_flaky_rerun.go`, `graph_validator.go`, `liveness_*`, `pipeline_engine.go`, `review_gate.go`, `stage_config_repository.go`, `stuck_decisions.go`, `workflow_*`; p1: `*_adapter.go`, `history_*`, `instance_status.go`, `controller_manager.go`, `pane_activity_gate.go`, `pty_discovery.go`, `review_queue_*`; p2: the eight `*_migration.go`/`*_backfill.go` startup files) write cluster maps with the exact files and the relocation lists (70, 60 and 10 declarations), check them against section 1 to 5 of the extraction design, and list every unexported symbol to capitalise (42, 45 and 8). Check each relocated declaration against `interface-pollution-checklist` and `primitive-obsession-checklist`: relocate, do not redesign. If a relocation list exceeds the SC1 budget at execution time, split the component at the group boundary the planner reports instead of raising the budget.
- Files: `tools/bzl/scoping/clusters/session-backlog.json`, `tools/bzl/scoping/clusters/session-agents.json`, `tools/bzl/scoping/clusters/session-startup.json`, `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `for c in session-backlog session-agents session-startup; do python3 tools/bzl/scoping/plan-split.py --pkg session --map tools/bzl/scoping/clusters/$c.json --json | jq -e '(.cycles|length)==0 and .sc6_ok' >/dev/null || echo FAIL; done | grep -c FAIL; true` prints `0`.

##### Task 13.5b: Relocate the p0 declarations inside `session` (cost M)
- One PR, no package change: move the 70 declarations the rest of `session` uses (for example `AutoReopenSpawner`, `ReviewGateSpawner`, `Notifier`, `CallerClass`, `ClassAgent`, the `LivenessDefinition` and `StageConfigRepository` interfaces) out of the p0 files into `session/backlog_ports.go`. Behaviour and signatures do not change; `go build` and the unchanged test suite are the proof. Land it alone so the following move is a pure rename.
- Files: `session/backlog_ports.go`, `session/backlog_lifecycle*.go`, `session/workflow_*.go`, `session/liveness_*.go`
- Verify: `go build ./... && go vet ./session/... && go test ./session -count=1 -timeout=20m; echo $?` prints `0`; `git diff --stat origin/main -- session | tail -1 | grep -c 'files changed'` prints `1` (and the diff contains no changed function body: `git diff -U0 origin/main -- session | grep -E '^[+-]func ' | sort | uniq -u | wc -l` prints `0`).

##### Task 13.5c: Move p0 to `session/backlog` (32 files, 14,681 LOC; cost M)
- Task 13.1e then 13.1d on `session-backlog.json`; capitalise the 42 unexported symbols, update importers in `server`, `server/services`, `server/mcp`, `server/wakes`, `server/workflows` and the root package, depguard (`no_server_in_core` globs and the audit's `domain-is-pure` rule for `session/domain/**`), the `noarchivedrevival` analyzer's file list in `.claude/rules/noarchivedrevival.md` and `tools/lint/noarchivedrevival` (it names `session/session_driver.go` and others, not the moved files, but re-check), and the feature-registry markers. The ent-backed repository stays in the core; the moved package depends on the relocated interfaces only.
- Files: `session/backlog/**`, `server/**` (importers), `.claude/rules/noarchivedrevival.md`, `.golangci.yml`
- Verify: `go build ./... && go vet ./... && go test ./session/... ./server/... -count=1 -timeout=30m; echo $?` prints `0`; `make lint-custom; echo $?` prints `0`; `grep -rn 'stapler-squad/session/ent"' session/backlog | wc -l` prints `0`.

##### Task 13.5d: Relocate and move p1 to `session/agents` (21 files, 6,903 LOC; cost M)
- Same two-PR procedure (60 declarations, 45 unexported symbols) for the agent adapters (`claude`, `gemini`, `agy`, `opencode`, `pi`), history linker/transfer/detector, `instance_status.go`, `controller_manager.go`, `pane_activity_gate.go`, `pty_discovery.go` and the review queue (`review_queue_*`, `review_gate.go` stays with p0 if the planner says so). Moved files read `Instance` through `Snapshot()` or exported accessors only.
- Files: `session/agents/**`, `session/agents_ports.go`, `server/**` (importers)
- Verify: `go build ./... && go vet ./... && go test ./session/... ./server/... -count=1 -timeout=30m; echo $?` prints `0`; `python3 tools/bzl/scoping/replay.py --commits 500 --json | jq -e '.t9d_session_loc <= 80000 and .t9c_mean_lib_loc_share <= 0.40 and .t9e_target_model_share <= 0.30'; echo $?` prints `0`.

##### Task 13.5e: Move p2 (startup backfills) and re-evaluate the other session components (cost S)
- Move the eight startup `*_migration.go`/`*_backfill.go` files (594 LOC) to `session/startup/` if the planner still reports SC1 to SC6 true; run the planner on p3 (autonomous driver and scroll adapters, 7 files) and p4 (process backends, 5 files) and the Task 13.1g SC5 data; append verdicts to the ledger. Record measured `session` size and T9a to T9d.
- Files: `session/startup/**`, `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `go build ./... && go vet ./session/... && go test ./session/startup/... ./session -count=1; echo $?` prints `0`; `grep -c '^| tierB-session-reeval |' project_plans/bazel-conversion/implementation/scoping-ledger.md` prints `2`.

#### Story 13.6: Keep packages scoped (budget, boundary rules, documentation)
**As an** agent working in this repo, **I want** a ratchet and written rules, **so that** the packages do not regrow and the next split is a documented procedure.
**Acceptance Criteria**:
- A ratchet exists.
  - *Given* `tools/bzl/scoping/budget.txt` (per package: maximum non-test files, maximum non-test LOC, maximum direct internal imports; initial values are the post-epic measured values rounded up by 3%), *When* `bash tools/bzl/scoping/check-budget.sh` runs on the tree and on a fixture that adds 5,000 LOC to a budgeted package, *Then* it exits 0 and 1 respectively; a value may only be lowered by a later change (the script rejects a PR that raises one without the label-less justification line `scoping-budget-raise: <reason>` in the commit message).
- Boundary rules are enforced.
  - *Given* the new packages, *When* `make lint` runs, *Then* depguard rules exist that keep `session/backlog` and `session/agents` off `session/ent` and off `server/**`, and `go list -deps ./session/backlog | grep -c 'stapler-squad/server'` prints `0`.
- A how-to exists.
  - *Given* `docs/how-to/split-a-go-package.md`, *Then* it names the harness, the criteria SC1 to SC6, the codemod, the conflict check and the ledger, and `docs/README.md` links it.
**Files**: `tools/bzl/scoping/budget.txt`, `tools/bzl/scoping/check-budget.sh`, `.golangci.yml`, `docs/how-to/split-a-go-package.md`, `docs/explanation/package-size-under-bazel.md`, `docs/README.md`

##### Task 13.6a: Package budget ratchet (cost S)
- `budget.txt` and `check-budget.sh` (uses `go list` and `wc`, no Bazel needed) wired into `make ready`; the PR-title/commit check from Task 0.2h is extended to accept the `scoping-budget-raise:` line.
- Files: `tools/bzl/scoping/budget.txt`, `tools/bzl/scoping/check-budget.sh`, `tools/bzl/scoping/check_budget_test.sh`, `Makefile`
- Verify: `bash tools/bzl/scoping/check_budget_test.sh; echo $?` prints `0`; `make -n ready | grep -c ready-scoping-budget` prints `1`.

##### Task 13.6b: Depguard boundary rules for the new packages (cost S)
- Add depguard rules to `.golangci.yml` for each package created by Stories 13.3 to 13.5 (`session/backlog`, `session/agents` must not import `session/ent` or `server/**`; `server/services/backlogsvc`, `wsconn` must not import `session/ent`); adjust the `no_server_in_core` globs that name moved paths. Use the bare `**/X/**` glob form the file's own comment requires.
- Files: `.golangci.yml`
- Verify: `golangci-lint run ./session/... ./server/... 2>&1 | grep -c 'depguard'` prints `0`; a seeded violation in a scratch file makes the same command print at least `1` (`tools/bzl/scoping/depguard_seed_test.sh; echo $?` prints `0`).

##### Task 13.6c: Documentation (cost S)
- Write `docs/how-to/split-a-go-package.md` (how-to) and `docs/explanation/package-size-under-bazel.md` (why: package as unit of invalidation and slice; the test-edge correction to RA1; why count-based fan-out rises when upper packages are split; the ADR-010 numbers), link both from `docs/README.md`, and add a short paragraph to the Epic 11 agent skill (`.claude/skills/bazel-stapler-squad/SKILL.md`) saying that a new file in `session/` or `server/services/` goes to the smallest package that already owns its imports.
- Files: `docs/how-to/split-a-go-package.md`, `docs/explanation/package-size-under-bazel.md`, `docs/README.md`, `.claude/skills/bazel-stapler-squad/SKILL.md`
- Verify: `grep -c 'split-a-go-package' docs/README.md` prints `1`; `[ "$(grep -c 'plan-split.py' docs/how-to/split-a-go-package.md)" -ge 1 ]; echo $?` prints `0`.

#### Story 13.7: Re-measure, compare with the prediction, close
**As the** owner, **I want** the predicted and measured T9 compared and the open work recorded, **so that** the model is trusted or corrected, and the Tier C follow-up has numbers.
**Acceptance Criteria**:
- *Given* the tree after Stories 13.2 to 13.5, *When* `bash tools/bzl/parity/scoping.sh` runs, *Then* it prints `T9a` to `T9e` each `PASS` (T9f `PASS` or `UNMEASURED` if no manifest exists), the ledger has a predicted-versus-measured row per executed cluster, and every row's lib-LOC-share error is within 1 percentage point or carries a one-line cause.
- *Given* Epic 12's shadow job exists, *When* `bash tools/bzl/e2e/replay-selection.sh --json` is re-run on the final layout, *Then* the ledger records the change in `share_median` against its value at Story 0.15 (report-only, not a gate).
**Files**: `project_plans/bazel-conversion/implementation/scoping-ledger.md`, `project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md`

##### Task 13.7a: Final T9 measurement and ledger reconciliation (cost S)
- Re-run Tasks 13.1b and 13.1f on `main`, fill the predicted-versus-measured table in the ledger, and re-run the Story 12.4 replay if Epic 12 exists. Any cluster whose measured gain differs from the prediction by more than 1 percentage point gets its cause recorded and the model change (if any) committed to `plan-split.py`.
- Files: `project_plans/bazel-conversion/implementation/scoping-ledger.md`
- Verify: `bash tools/bzl/parity/scoping.sh | grep -cE '^T9[a-e]: PASS$'` prints `5`.

##### Task 13.7b: Close the ADR and record the follow-ups (cost XS)
- Set ADR-010 to Accepted (or Amended with the measured outcome); record Tier C as a follow-up project with the numbers in `research/package-scoping.md` section 4 (`Instance` 38 files, `EntRepository` 15, `Storage` 11, inbound symbol counts 321 and 225); open backlog items for any cluster recorded as `skipped by criterion`.
- Files: `project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md`
- Verify: `grep -c '^\*\*Status\*\*: Accepted' project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md` prints `1`.

---

## Review disposition

Source reviews: `implementation/architecture-review.md` (arch B1-B3, C1-C14, nitpicks) and `implementation/adversarial-review.md` (adv B1-B3, C1-C10, minors). Repair iteration 1 of 5. "Fixed" cites the story or task where the plan now changes; verified facts are cited inline there.

| Finding | Disposition |
|---|---|
| arch B1 / adv B1 (tmux targets under submodule gitlink and `.bazelignore`) | fixed at Story 0.7, Tasks 0.7a/0.7c, 0.2a, 0.2e, Story 3.3, ADR-007; targets now `//tools/bzl/tmux:tmux_bin` from an `http_archive`, submodule is a pin only, pin rule in `tools/bzl/tmux/PIN` |
| arch B2 / adv B2 (embed flags and Windows target under gitignored `build/`) | fixed at Tasks 0.8b, 3.3c, 6.1a, 0.2e, Story 1.4, ADR-007: `//tools/bzl/flags`, `//tools/bzl/windows`; `check-build-paths.sh` fails on ignored, gitlinked or `.bazelignore`d BUILD paths |
| arch B3 (`RepoRoot()` import cycles) | fixed at Task 4.1a, Story 4.1, Glossary, Tech Debt row: stdlib-only leaves `internal/reporoot` and `internal/testenv`; `go list -deps ./testutil` verified to include `config`, `session`, `session/tmux`, `executor` |
| adv B3 (client-side read-only, header secret in BEP) | fixed at ADR-004 (rewritten), Story 0.12 (Tasks 0.12a-d), Story 7.4: two server listeners plus Tailscale ACL tags, write credential only in `main`-restricted environment, `--credential_helper`, secret-scan step, no raw BEP from the write job; bazel-remote#628 VERIFIED still open. Redaction is an explicit pass/fail spike (S10-redaction) |
| arch C1 (resolve lines land after BUILD files) | fixed at Task 1.1a (resolve/exclude lines land with the first Gazelle run), Tasks 2.1b/2.3b become verification |
| arch C2 (race vs pure) | fixed at Task 1.1c, Story 0.8, S7 criterion |
| arch C3 (sealed set vs doc that does not exist) | fixed at Tasks 1.4a/1.4b (`tools/bzl/configs.txt`), doc check moved to Task 11.3b |
| arch C4 (`BUILD_SYSTEM` defined late, freshness gate breaks make path) | fixed at Story 1.5, Task 2.4b, Task 8.3a, Story 8.5 (canary asserts no Bazel installed) |
| arch C5 / D-PROTECT-MERGE (no branch protection to rename) | fixed at Story 7.6 and Unresolved Questions: "blocking" = no `continue-on-error`; owner decision D-PROTECT-MERGE (a) add a ruleset or (b) merge-gate convention; rename blocker removed |
| arch C6 (`mcp-integration.yml` uses `paths:` filters) | fixed at Story 7.3, Tasks 7.3a/7.3c, Tech Debt row, Task 7.6c; non-Bazel inputs listed in `tools/bzl/non-bazel-inputs.txt`; deleted/renamed/non-graph files fail safe |
| arch C7 (spike 0.3c needs 0.6) | fixed at Task 0.6e (S1-ts spike), Story 0.3, wave list |
| arch C8 (`no-remote-cache` on a repository rule) | fixed at Task 3.1a, Story 7.7, S4 criterion (sandbox setup measurement, 1.5x threshold) |
| arch C9 (`.bazelignore` exact paths, nested `node_modules`) | fixed at Task 0.2a (exact paths, `check-bazelignore.sh`, `gazelle:exclude **/node_modules`); note: no `node_modules` exists in a fresh checkout so the nested list is produced at implementation time |
| arch C10 (`SpikeVerdict` machine-readable) | fixed at Story 0.14, Task 0.14a: `spike-results.yaml` plus `check-spike-results.py` |
| arch C11 (Epic 4 by package, not class) | fixed at Epic 4 goal, new Story 4.10 (class sweeps first), Stories 4.3-4.7 reduced to residual tails |
| arch C12 (missing Tech Debt rows) | fixed at Tech Debt table (testutil, ent `generate.go` absence with `ent_flags.txt` single source, `tools/lint` `x/tools` isolation and Task 0.2c check) |
| arch C13 (P1 baseline is a stale static file) | fixed at Tasks 0.1e and 4.8a: P1 compares two live runs; the file is the Epic 0 reference |
| arch C14 (who can trigger a write-context run) | fixed at ADR-004 and Tasks 0.12b, 7.4b: separate write user, environment restricted to `main` |
| arch nitpick: ADR numbering vs `docs/adr/` | deferred: only matters if the ADRs are promoted to `docs/adr/`; renumber then |
| arch nitpick: 0.14a `grep -L Proposed` check | fixed at Task 0.14a (explicit `Status: Accepted` grep) |
| arch nitpick: 6.1b counts the word "windows" | fixed at Task 6.1b (error count plus `cquery` incompatibility) |
| arch nitpick: "no L tasks" vs ADR-008 L costs | fixed at Effort estimate (names the tasks most likely to move) |
| adv C1 (P9 needs Task 9.1c before the gate) | fixed at Task 7.5e (script moved to Epic 7), P9 row, wave list; Task 9.1c reduced to docs |
| adv C2 (make fallback rots) | fixed at Task 7.6a (legacy jobs run unconditionally, no regex needed), Story 8.5 (`make ready`, build, e2e shard, path-triggered, no Bazel), Task 10.1a (mechanical guard replaces unmechanizable label check), Task 10.2c (selection logic deleted last) |
| adv C3 (spike criteria that cannot fail) | fixed at the spike protocol (`PASS iff` rule, allow-list, unevaluable = `Fail`) and Tasks 0.2c, 0.3b, 0.6b, 0.9a, 0.10a, Story 0.6 AC |
| adv C4 (fallbacks share the failed dependency) | fixed at spike protocol "If S4 fails" paragraph and pre-registered owner decision D-HERMETIC; S10 disk-cache fallback weakness stated in ADR-004 |
| adv C5 (buf.yaml BSR dependency) | fixed at Task 0.3e, Story 0.3 AC, Task 2.5a |
| adv C6 (ent is many packages) | fixed at Story 0.4, Task 0.4b, S2 criterion (53 packages, importer build), Task 2.3a (`packages.txt` plus freshness test) |
| adv C7 (noisy and narrow baselines) | fixed at Story 0.1 (all workflows, per-SHA definition, 5 runs, load gate), P10 (10% tolerance, n >= 20), Story 0.10 (one 30% definition) |
| adv C8 (`exclusive` serialises CI) | fixed at Story 4.5, Task 4.5a, Task 7.5d |
| adv C9 (ambiguous soak rule) | fixed at Story 7.6 (single ledger-based rule with N and K), Unresolved Questions (owner fixes before Epic 7) |
| adv C10 (agent-writable attestations, no CODEOWNERS) | fixed at Task 0.2e (CODEOWNERS early), Task 7.5c (environment approval plus identity check; does not rely on CODEOWNERS because there is no protection) |
| adv minor: `.bazelignore` exact paths | fixed at Task 0.2a |
| adv minor: Makefile lacks `sync`, `gen-freshness`, `gazelle`, `setup-git` | fixed at Tasks 2.4b, 7.2b, 11.2a (marked as new targets; VERIFIED absent) |
| adv minor: unverified version pins | fixed at Task 0.2a (`bazel mod tidy` fails on unknown versions) |
| adv minor: scope of 8.4, 11.8, 7.7, 8.7 | deferred: individually small or conditional (8.4 on S12); the owner asked for full scope |
| adv minor: Task 0.1d count discrepancy | fixed at Task 0.1d (3,373 tracked, closed with a note) |
| adv N1 (gRPC cannot pass a GET/HEAD-only listener) | fixed at ADR-004 (HTTP cache protocol, `https://`, gRPC disabled), S10 row, Story 0.12 ACs (real `bazel build` with remote cache hits greater than 0), Tasks 0.12a, 7.4a; bazel-remote HTTP/gRPC flags VERIFIED from its [README](https://github.com/buchgr/bazel-remote/blob/master/README.md); Bazel sending the credential-helper header over https to the proxy and runner trust of the certificate are UNVERIFIED and tested by S10 |
| adv N2 (reads vs htpasswd) | fixed at ADR-004, Tasks 0.12a, 0.12d: `--allow_unauthenticated_reads` on loopback bazel-remote behind the two ACL-restricted proxy listeners; S10 asserts an unauthenticated PUT to `:9093` returns 401 |
| adv N3 (redaction coverage) | fixed at S10-redaction row, Story 0.12, Task 0.12d, Story 7.4: raw and base64 canary, wrong-credential run, `command.log`, HTTP verbose and gRPC log, `secret-scan-allow.txt` for benign `Authorization` text |
| adv N4 (unprotected `main` reaches the write job) | fixed at new Task 0.12e (prerequisite of 0.12b), ADR-004, Unresolved Questions: environment deployment-branch rule plus owner required-reviewer gate mandatory; a `main` ruleset remains the owner's decision D-PROTECT-WRITE |
| adv N5 (Tailscale ACL, `pull_request_target`) | fixed at ADR-004, Story 7.4, Task 7.4b: default-deny ACL check, ephemeral short-expiry keys, SHA-pinned action, `check-workflows.sh` and `check-tailscale-acl.sh` |
| adv N6 (write path unmonitored) | fixed at ADR-004, Story 7.4, Task 7.4b: `cache-write-health` job alerts after 3 consecutive failed `main` uploads |
| arch N1 (`grep -vc` exits 1) | fixed at Task 4.1a verify: negated `grep -qv` exit status |
| arch N2 / C1 (generated-tree BUILD files) | fixed at Task 0.2e: Gazelle `exclude` for `gen`, `web-app/src/gen` and ent entity dirs, plus a `check-build-paths.sh` second mode over ignored and untracked BUILD files and every loaded package |

**Counts** (rows above, counted by script; repair iterations 1 and 2): 45 dispositions, 43 fixed, 2 deferred (arch nitpick on ADR numbering; adv minor on scope).

## Phase 4 disposition

Source reviews: `implementation/pre-mortem.md` (15 failure modes), `implementation/consistency-review.md` (1 BLOCKER, 9 CONCERN, 4 NITPICK) and `implementation/validation.md` (gaps G1-G9). "Fixed at" cites the story or task that now carries the change; "deferred" states why.

| Finding | Disposition |
|---|---|
| pre-mortem #1 P1 (host exhaustion on onyx) | fixed at Task 0.2f (`tools/bazel` wrapper: flock semaphore N=2, `systemd-run` caps), Story 0.10 AC and S6 criterion (8 queued invocations, live-latency check, Story 0.14 cannot pass S6 without it), Task 9.1b (`OOMScoreAdjust=-900`), Task 11.4a (daily orphan timer, disk alarm) |
| pre-mortem #2 P1 (generated-file merge races) | fixed at Story 7.8 (Tasks 7.8a-c: `bazel-main-repair`, same-repo PR autofix, `**/BUILD.bazel` merge driver, dispatcher prompt line), Task 0.10d (spike S13: commit or ignore `MODULE.bazel.lock`), Story 7.6 (`generated-file-race` attribution class) |
| pre-mortem #3 P1 (false-green cached tests) | fixed at Task 4.2a (per-`SandboxClass` `external` tag policy, `flaky` ban), Task 0.2g (`check-bazelrc.sh`), Task 4.9c (nightly `--nocache_test_results --runs_per_test=3`), Task 8.5b (post-cutover pass-set equality) |
| pre-mortem #4 P1 (cold floor, unusable onyx) | fixed at Task 0.12c (real DROP, netem and silent-listener tests replace the black-hole test), Task 7.4b (pre-flight and `actions/cache` fallback), Task 0.2b (timeout, retry, circuit-breaker flags), Task 0.3f (prebuilt protoc), P10 cold row at most 1.5 x baseline p50 blocking Task 7.6a |
| pre-mortem UNVERIFIED flag names | handled at the Flag-name policy (header correction 6), Task 0.2b (each flag gets a `spike-verified` row), Task 0.2g (unverified flags rejected), Story 0.14 AC (`grep -c spike-verified` at least 8) |
| pre-mortem #5 P2 (`--remote_download_*`) | fixed at Task 7.4a, Task 0.2b, Task 7.4c (outbound bytes reported; P10 fallback to `actions/cache`) |
| pre-mortem #6 P2 (cache shares the live SQLite filesystem) | fixed at Task 0.2b (paths and 100 GB GC cap), Task 0.12a (`--max_size` at most 100 GB, quota'd dataset or `StateDirectory`), Task 7.4c and Task 11.4a (`df` alarm); owner storage budget in Unresolved Questions |
| pre-mortem #7 P2 (stale SyncGen tree) | deferred: a freshness check under `BUILD_SYSTEM=make` would need either `bazel-bin` (breaking Story 1.5's make-independence) or a new input-hash stamp design; the Bazel-mode gate already lands at Tasks 2.4b and 8.3a, so decide the make-mode variant when Story 8.3 starts |
| pre-mortem #8 P2 (hermeticity downgraded quietly) | fixed at Task 7.7a (required nightly, verdict attested, per-action pin list), Task 11.8a (fork-age check) |
| pre-mortem #9 P2 (macOS toolchain drift) | fixed at Task 5.1a (pinned image and Xcode), Task 11.8a (canary runs the macOS job) |
| pre-mortem #10 P2 (Go toolchain skew) | fixed at Task 11.8a (`supported-go.txt` check and path-scoped rule) |
| pre-mortem #11 P2 (rollback illusion) | fixed at Story 10.1 (event-based retention plus owner-run Linux and macOS installs), Task 10.2a (blocked on attestations), Unresolved Questions |
| pre-mortem #12 P2 (stalled dual-build state) | fixed at Overrun checkpoint (epic-end exit rule, `ExclusionsFile` and CI-minutes trend) |
| pre-mortem #13 P3 (unsandboxed tests corrupt live state) | fixed at Task 4.1b (committed `test` env pins, `testenv` real-config refusal), Task 0.2g (`--spawn_strategy=local` rejected) |
| pre-mortem #14 P3 (Tailscale pivot) | fixed at Task 7.4b (daily ACL-drift job, new-node alert) |
| pre-mortem #15 P3 (macOS signed identity drift) | fixed at Task 9.1b (`codesign -d -r-` assertion) |
| consistency B1 (ADR-003 vs S4 vocabulary) | fixed at ADR-003 (verdict table), Fallback matrix S4 row, Story 0.6 ACs, Task 0.6b, new Task 0.6f (`rules_bun` reachable from S4 failure; fork next; `Fail` last); ADR-008 matrix row aligned |
| consistency C1 (`Task 0.12pre`) | fixed at ADR-004 (now Task 0.12e) |
| consistency C2 (three regex lists) | fixed at requirements.md and ADR-001 (three hand-maintained selection mechanisms) |
| consistency C3 (tmux wording) | fixed at requirements.md In Scope (pinned release tarball; submodule is a version pin only) |
| consistency C4 (stale Open Questions) | fixed at requirements.md (answered items struck through with pointers) |
| consistency C5 (`godebug netdns=go`) | fixed at Task 3.4d |
| consistency C6 (`Formula/`) | fixed at Task 3.7c |
| consistency C7 (two D-PROTECT) | fixed: D-PROTECT-WRITE (Task 0.12e, ADR-004) and D-PROTECT-MERGE (Story 7.6, Task 7.6d) |
| consistency C8 (Gazelle extension stance) | fixed at ADR-008 rule 5 and requirements.md Constraints (sequencing preference inside the owner's allowance) |
| consistency C9 (header status) | fixed at the plan header |
| consistency N1 (truncated titles) | fixed: Effort table regenerated by script with full titles |
| consistency N2 (task id order) | fixed: ordering note before "Serialising shared files" |
| consistency N3 (Task 9.1c tombstone) | fixed at Task 9.1c (retitled) |
| consistency N4 (Story 2.6 AC) | fixed at Story 2.6 AC |
| validation G1 (`Formula/`) | fixed at Task 3.7c |
| validation G2 (`netdns=go`) | fixed at Task 3.4d |
| validation G3 (feature-registry inputs) | fixed at Task 2.4c (ADR-002 and ADR-005 decision) |
| validation G4 (commit and force-push guardrail) | fixed at Task 0.2h (CI grep, `pre-push` hook, PR-title check) |
| validation G5 (no WORKSPACE) | fixed at Task 0.2g |
| validation G6 (shared disk-cache corruption) | fixed at Task 0.10c |
| validation G7 (hit-ratio threshold) | fixed at Task 7.4c and P10 (median at least 0.60, owner may waive) |
| validation G8 (disk per worktree) | fixed at Task 0.10a, Task 7.4c and P10 (at most 1.25 x baseline, owner may waive) |
| validation G9 (service metrics diff) | fixed at Task 7.5b |

**Counts** (rows above, counted by script): 39 dispositions, 38 fixed or handled, 1 deferred.

## Triad disposition

Source: triad review iteration 1 (Product gaps 1-8, UX/DX gaps 9-18, Engineering gaps 19-22). "Fixed at" cites the story or task now carrying the change; nothing was deferred and no scope was cut.

| Gap | Disposition |
|---|---|
| 1 CI-speed improvement target besides non-regression | fixed at Improvement targets T1/T2 (work-avoided ratio, billed minutes and time-to-green on partial-change PRs), Task 0.1g (set from baseline, owner-confirmed), Task 7.5g and the P10 improvement rows; non-regression bounds kept |
| 2 Quantify pain / why-now | fixed at Task 0.1f (CI p50/p95, cold worktree build, disk per worktree, regen steps per agent session, warm one-package latency into `baselines.md`), Story 0.1 AC, Task 0.1g and ADR-001 ('Measured pain') |
| 3 Waivers must not dilute P10 | fixed at Task 7.5c (waiver schema: reason, expiry, signed), Task 7.5f (validator, only two waivable rows, 90-day cap), Story 7.5 AC (`PARITY: PASS WITH WAIVERS` and report block); improvement rows never waivable |
| 4 Worktree disk improvement target | fixed at target T3 (second cold build at most 25% of the first; disk at most 1.0 x baseline; 1.25 x stays as the waivable ceiling), Task 0.1g, Task 7.5g |
| 5 Agent-experience metrics | fixed at targets T4 (edit-build-test latency, warm one-package test) and T5 (first-error legibility with remedy), Task 7.5g, Task 1.6d |
| 6 Post-cutover status of native go build/test | fixed at 'Post-cutover status' section, ADR-002 amendment, Story 11.5 AC and Task 11.5a (supported for gopls, lint, quick loops and bisecting; Bazel is the build of record; native results not authoritative) |
| 7 Economic abort / re-plan criterion at Story 0.14 | fixed at Task 0.14e (EG1-EG4 from spike data, decision gate with owner options, never a downscale), Story 0.14 AC, Unresolved Questions |
| 8 Priority rationale (RICE/ICE, inputs UNVERIFIED) | fixed at ADR-001 'Priority rationale' (also pointed to from requirements.md); re-scored at Task 0.1g |
| 9 Wrapper error translation / `make bazel-doctor` | fixed at Story 1.6, Tasks 1.6a (exact text E1-E9 incl. 'remote cache unreachable, building locally'), 1.6b (queue wait with position, ETA, timeout, exit 75), 1.6c (doctor) |
| 10 gen-freshness in build/test wrappers | fixed at Task 2.4d (cheap input-hash form, no Bazel needed) and Task 8.2d (wired into build/test/lint wrappers); also settles the make-mode variant deferred as Phase 4 pre-mortem #7 |
| 11 `--config=agent` via the `tools/bazel` wrapper | fixed at Task 1.6b and Story 1.6 AC (raw `bazel` gets agent config on non-TTY or `CLAUDECODE=1`; opt-out `BAZEL_HUMAN=1`) |
| 12 Troubleshooting stub for states 1,2,3,4,10 in Epic 1 | fixed at Task 1.6e (five sections with anchors), extended by Task 11.3b without renaming |
| 13 Cap build-failure output, say where the log lands | fixed at Task 1.6b and Story 1.6 AC (60 lines, `full log: <worktree>/.bazel-logs/last-build.log`) |
| 14 Lint for no-sandbox/local tags; proposal marking in skill | fixed at Task 4.2c (comment plus tracking issue, issue existence nightly) and Task 11.1d (proposal markers enforced both directions until spike-verified) |
| 15 Fresh-worktree first-build journey doc | fixed at Task 11.3d |
| 16 `make test PKG=` mapping | fixed at Task 8.2e (nested packages, tagged targets, TAGS-to-config, RUN, unknown tag) and Story 8.2 AC (`PKG=./session` -> `//session:all`, previous AC wording corrected) |
| 17 Announce inverted precondition | fixed at Task 1.6c (`make help` header and doctor line), Task 11.5a, 'Post-cutover status' section |
| 18 Directive change hidden by linguist-generated | fixed at Task 11.6a (root and HandBuildFile paths excluded from `linguist-generated`, CODEOWNERS covers `/BUILD.bazel`, also Task 0.2e) and Task 11.6b (directive-change check) |
| 19 Split 4.10a-c and 4.7a/b into batches of at most 5 files | fixed at Task 4.10f (census-driven manifest, explicit split rule) and batch wording in Tasks 4.10a-e, 4.7a/b |
| 20 Absolute boolean pass for 4.7a/b | fixed at Tasks 4.7a and 4.7b Verify (exit codes, `missing_in_bazel: 0`, exclusions at most 10, all batches merged) |
| 21 Task 0.14a over 5 files | fixed at Tasks 0.14a (4 files) and 0.14d (4 files) |
| 22 Task 10.2a shallow verify | fixed at Task 10.2a (last make-path canary run, `check-removal.py` set equality against expected-deleted and retained allow-lists, seeded negative cases) |

**Counts** (rows above, counted by script): 22 dispositions, 22 fixed, 0 deferred. New tasks: 19 (0.1f, 0.1g, 0.14d, 0.14e, 1.6a-1.6e, 2.4d, 4.2c, 4.10f, 7.5f, 7.5g, 8.2d, 8.2e, 11.1d, 11.3d, 11.6b); new stories: 1 (1.6). Counts and effort totals in the header and the Effort estimate were recomputed by script after these edits.

## Triad disposition (iteration 2)

Source: triad review iteration 2 plus the verified `tools/bazel` file/directory collision. "Fixed at" cites the task now carrying the change; nothing was cut. Open-ended additions that are not defects are under Follow-ups below.

| Gap | Disposition |
|---|---|
| B0 `tools/bazel` used as both the wrapper file and a helper directory | fixed globally: wrapper stays the single file `tools/bazel` (Tasks 0.2f, 1.6b); every helper, package and label moved to `tools/bzl/` (`//tools/bzl/tmux:tmux_bin`, `//tools/bzl/flags:*`, `//tools/bzl/windows:*`, `//tools/bzl:go_test.bzl` ...) across plan, validation, requirements, pre-mortem, research, reviews and all ADRs; header states the distinction |
| E1 Task 0.1f replays Task 1.6d fixtures (forward dependency) | fixed at Task 0.1h (fixtures authored in Epic 0, run on the make path; Task 1.6d replays the same scripts) |
| E2 Tasks 0.2a/0.2e depend on the ent exclusion (0.11b) and whole-repo Gazelle (1.1a) | fixed at Tasks 0.2a, 0.2e (provisional exclusion, `path  # planned: <Task>` lines, incremental Gazelle run) and 1.1a (switches on `check-build-paths.sh --full`) |
| E3 `make gazelle` created in 7.2b but used as the remedy from Epic 1; `make setup-git` in 11.2a | fixed at Task 1.5a (`gazelle`, `gazelle-check`, `sync` skeletons), Task 0.2h (`setup-git`), Tasks 7.2b, 2.4b, 11.2a now extend them |
| E4 E5 hint references `bazel-orphan-outputbases.sh` created in 11.4a | fixed at Task 11.4a (lands with Epic 1, dry run by default) and the E5 text (conditional fallback hint when the script is absent) |
| E5 Tasks 4.7a/b verify use `test-set-diff.sh` (4.8a) and `check-exclusions.py` (4.9a) | fixed at Epic 4 goal and ordering note (4.8a and 4.9a run before Story 4.10), Story 4.10 header |
| E6 4.10f must emit the 4.7 groups; `census.json` and `census-check.py` had no producer | fixed at Task 0.9a (adds `census.py`, `census-check.py`, `census.json`, unit test) and Tasks 4.10f/4.7a (`census-batches.py --residual` emits 4.7a/4.7b after the sweeps) |
| E7 Task 8.2b depends on 8.2e | fixed at the ordering note (8.2e before 8.2b) and the cross-epic edge list |
| E8 Task 10.2a deletes recipes before the `legacy-*` jobs are deleted | fixed at Tasks 10.2d (final canary evidence), 10.2c (deletes `legacy-*` jobs and the canary workflow), 10.2a, 10.2b (reordered; removal PR can be green) |
| E9 Task 8.5b weekly canary needs Bazel while the canary AC says Bazel is absent | fixed at Story 8.5 AC, Tasks 8.5a/8.5b (two jobs: `make-only` Bazel-free, `pass-set-equality` with Bazel, checked by `yq`) |
| E10 Tasks 7.8a/7.8b: pushes with the default `GITHUB_TOKEN` do not trigger workflows | fixed at Tasks 7.8a/7.8b (`BAZEL_REPAIR_TOKEN` from a GitHub App or fine-grained PAT, or the `workflow_dispatch` alternative that needs no extra credential), Unresolved Questions and wall-clock blockers |
| E11 Tasks 7.8b and 0.2h depend on `make setup-git` | fixed at Task 0.2h (creates `setup-git`), Task 0.10b (registers drivers), hard edge in Task 7.8b |
| E12 wrapper needs macOS/CI fallbacks for flock, ionice, systemd-run | fixed at Task 0.2f (capability detection, mkdir lock, CI bypass, bash 3.2) and Task 5.1a (macOS runs `wrapper_test.sh`) |
| E13 `OOMScoreAdjust=-900` only in 9.1b, leaving `:8543` unprotected in Epics 0-8 | fixed at Task 0.2i (owner-run drop-in) and Task 0.2f (Bazel scope `OOMScoreAdjust=500`); 9.1b keeps the final value |
| E14 Task 11.4a under-sized and could overwrite the owner's uncommitted skill edits | fixed at Tasks 11.4a (script, sha256 pre-check, fall back to `bazel.md`), 11.4b (timer and unit), 11.4c (alarm and trend) |
| E15 Tasks 0.1f, 0.9a, 11.8a under-list files | fixed at those tasks' Files lines |
| E16 23 tasks with non-boolean verify text | fixed at Tasks 1.1a, 1.2a, 1.3a, 1.3b, 2.4b, 4.2b, 4.3a, 4.4a, 7.2a, 7.5a, 7.5b, 7.6a, 7.6b, 7.6c, 8.2e, 8.3a, 8.5b, 10.2a, 11.1a, 11.1c, 11.3a, 11.3b, 11.3d (each now a command with an exit code or expected output; `<id>` and `<stub-commit>` placeholders removed) |
| E17 `merged` in `batches.json` self-set | fixed at Task 4.10f (`--refresh` derives it from a `Batch:` commit trailer and a zero census slice; `--check-merged` is the verify) |
| E18 epic dependency graph omits 2.2a<-3.1a and 2.6<-3.1a; bazel-owner-attest setup and P10 sample windows missing from blockers | fixed at 'Cross-epic edges not drawn above', Tasks 2.2a/2.6a, wave list, Wall-clock blockers |
| U1 E3 hint 'run make sync' misdirects on Bazel gen-label problems; no plain-go remedy | fixed at Task 1.6a (E3 `source: go`, new E10 `source: bazel`) and Task 2.4e (`make go-*` shims, gopls guidance) |
| U2 pre-build BUILD-staleness warning | fixed at Task 1.6f and Task 8.2d |
| U3 translator text must be tied to the 11.1d proposal guard | fixed at Task 1.6a (single source `errors.yaml`, `status` field) and Task 11.1d (quote check, status check) |
| U4 translator precedence and negative fixtures | fixed at Story 1.6 AC (ordered precedence, E4/E8 conditions) and Task 1.6a verify (`--negative`) |
| U5 distinct louder row and CI counter for cache auth failure | fixed at row E11 (Task 1.6a), Task 7.1a (`bazel_cache_auth_failures_total`), Task 7.4b (`cache-write-health`) |
| U6 E5 hint dry-run first; cover disk-cache GC and /tmp reclaim | fixed at row E5 and Task 11.4a (dry run default; cache size vs cap; `--tmp`) |
| U7 background-run and heartbeat guidance for agent Bash timeouts | fixed at Story 1.6 AC, Task 1.6b (heartbeats, `BAZEL_BG=1`, `bg-status.sh`) and Task 11.1a |
| U8 FORCE=1 / `--nocache_test_results`; `--test_filter` vs `go -run` | fixed at Task 8.2e (`FORCE=1`, documented difference), Task 4.2b (semantics recorded), Task 8.7a, Task 11.1a |
| U9 'N more errors' count in the 60-line cap | fixed at Story 1.6 AC and Task 1.6b verify |
| U10 wrapper prints which system ran | fixed at Task 8.1a (`[build-system=...]` banner, native-go banner) |
| U11 install lockfile/BUILD merge driver setup earlier; per-clone note | fixed at Tasks 0.2h, 0.10b, 11.2a and doctor check 9 (Task 1.6c) |
| U12 PR annotation for external-dep and module-file changes | fixed at Task 11.6c |
| U13 doctor: memory/uptime, BUILD freshness, repro hint | fixed at Task 1.6c (nine checks plus the `--sandbox_debug --test_output=streamed` hint) |
| P1 hermeticity before/after metric; D-HERMETIC downgrades visible and counted | fixed at target T7, Task 0.1j (make baseline), Task 7.5g (Bazel measurement), Task 7.7a (`hermetic-downgrades.txt`, H4) |
| P2 T3 disk improvement target besides parity | fixed at target T3 (5-worktree aggregate at most 0.80 x 5 x baseline) |
| P3 whole-population CI-speed check; representativeness of the 30-PR set | fixed at target T2 (whole-population clause), 'Sampling' paragraph, Task 0.1i |
| P4 name the risky assumptions; cheap pre-measurement | fixed at RA1/RA2 paragraph, Task 0.1f (`risky-assumptions.sh`), Task 0.14e (EG5) |
| P5 mid-point economic re-check after Epic 1/2 | fixed at Story 2.7 / Task 2.7a |
| P6 T1 is a proxy; T2 needs a billed-minutes join by PR class | fixed at 'Sampling, proxies and risky assumptions' and Task 0.1i (repo is public, so runner-minutes with OS multipliers replace the 0-valued billing API) |
| P7 baselines.md / targets.yaml do not exist yet | left as is by design; stated plainly in the header 'Sequencing fact' |

## Follow-ups (post-triad)

- `make bazel-doctor --json` for tooling: nothing consumes machine-readable doctor output yet; add when a dashboard or hook needs it.
- `make explain-error <log>` to run the translator over an old log: the wrapper already translates live failures and saves `.bazel-logs/`; low incremental value.
- Automatic per-clone git config (for example through `init.templateDir` or the dotfiles repo) so a fresh `git clone` needs no `make setup-git`: it touches the owner's dotfiles, outside this repo's scope; doctor check 9 and the one-time wrapper hint cover it meanwhile.
- Grafana panels for `bazel_cache_auth_failures_total`, `bazel_disk_alarm` and the hermeticity numbers: the plan's observability is advisory annotations; build panels once the metrics exist.
- A Bazel-aware replacement for `ci-time-to-green.sh` that reports the critical-path job instead of only the total: useful for diagnosing RA2 after cutover, not needed to decide it.

---

## Triad disposition (iteration 3, final)

Verdicts: product ready, UX ready, engineering needs-work (1 blocker). Loop cap (3) reached; the blocker was fixed directly and reproduced before and after in a scratch repo (unanchored `bazel-*` ignores `.github/workflows/bazel-parity.yml` and `scripts/bazel-manual-instance.sh`; anchored `/bazel-*` ignores neither and still ignores `bazel-bin`). Not re-reviewed by fresh agents after this one-line fix.

| Finding | Disposition |
|---|---|
| Eng blocker: unanchored `bazel-*` in `.gitignore` (Task 0.2b) | fixed at Task 0.2b (anchored `/bazel-*` plus a `git check-ignore` verify) |
| Eng: Task 2.3a verify uses `//tools/bzl:ent_bzl_test` but `tools/bzl/BUILD.bazel` is first created in Task 4.2a; `hand-build-files.txt` omits `tools/bzl/BUILD.bazel`, `tools/bzl/bun/BUILD.bazel` (3.1a), `tools/gazelle/BUILD.bazel` (2.3b) | follow-up F6: 2.3a creates `tools/bzl/BUILD.bazel`; list the other three |
| Eng: no task wires Epic 0 guard scripts (`check-build-paths.sh`, `check-bazelrc.sh`, `check-pr-title.sh`, `check-no-force-push.sh`) into `make ready` / `bazel.yml` | follow-up F7: add a wiring task in Epic 1 |
| Eng: about 10 verify lines not boolean (0.1d, 0.4c, 0.5b, 0.6d, 0.11b, 0.11c, 0.14b, 3.7a, 5.1d, 9.1a; weak 1.2b, 1.3a, 1.4b) | follow-up F8 |
| Eng: minor forward references (2.4c, 0.1g/EG1-EG5, 1.6b/1.6f, 0.2a, 8.1b, garbled 2.2a wave text); pre-mortem.md P1 boxes unchecked | follow-up F9 |
| Product: T4 non-regression only; cache-hit-rate success metric has no T-row; requirements.md says baselines in Phase 2/targets Phase 3 (plan: Story 0.1/Task 0.1g); T2 should exclude/flag cache-outage runs | follow-up F10 |
| UX: kill-mid-run slot release test; cold-first-build duration note; auto-detach for long runs | follow-up F11 |

### Follow-ups (iteration 3)

- F6 applied (2026-11-03): Task 2.3a creates `tools/bzl/BUILD.bazel`; `hand-build-files.txt` (Task 0.2e) and the HandBuildFile glossary row list `tools/bzl/BUILD.bazel`, `tools/bzl/bun/BUILD.bazel` and `tools/gazelle/BUILD.bazel`.
- F7 applied: new Task 1.5b wires the guard scripts into `make ready` and `bazel.yml` (Story 1.5 now 2 tasks; plan totals 219 tasks); Task 1.5a's verify pattern narrowed to Bazel invocations.
- F8 applied: boolean Verify lines at Tasks 0.1d, 0.4c, 0.5b, 0.6d, 0.11b, 0.11c, 0.14b, 3.7a, 5.1d, 9.1a; strengthened at 1.2b, 1.3a, 1.4b.
- F9 applied: forward references fixed at Tasks 2.4c, 0.1g, 1.6b, 1.6f, 0.2a, 8.1b, 2.2a (pre-mortem.md untouched).
- F10 applied: target T2b (cache hit ratio), cache-outage exclusion for T2/T2b, and the baseline/target timing note under 'Sampling, proxies and risky assumptions'.
- F11 applied: kill-mid-run slot-release test (Tasks 0.2f, 1.6b), cold-first-build note and `BAZEL_AUTO_DETACH_SECS` auto-detach (Task 1.6b).

---

## Docs-derived disposition (`research/bazel-docs-best-practices.md`, 2026-11-01)

Format: delta id -> applied at task | follow-up: reason. "User-requested" rows are additions that are not research deltas.

| Delta | Disposition |
|---|---|
| D1 `startup:<config>` ignored | applied at Task 0.2b, Task 0.2f, Task 0.2g, Story 0.2 AC |
| D2 disk-cache GC + idle delay | applied at Task 0.2b, Task 7.4c, Task 11.4c, Task 0.10f (E5) |
| D3 same filesystem / reflink | applied at Task 0.2b, Task 0.10a, Task 0.10f (E10), Task 1.6c |
| D4 repository + contents cache | applied at Task 0.2b (kept `~/.cache/stapler-squad-bazel/repo`, contents cache explicit), ADR-004 |
| D5 extracted-repo measurement | applied at Task 0.10a, Task 0.10f (E6) |
| D6 `--remote_local_fallback` | applied at Task 0.2b, Task 0.12c, Task 7.4a, ADR-004, validation.md |
| D7 circuit breaker strategy flag | applied at Task 0.2b, Task 0.12c, Task 7.4a, ADR-004 |
| D8 `--remote_download_toplevel` default | applied at Task 0.2b, Task 7.4a, ADR-004 |
| D9 eviction retries / invocation id | applied at Task 7.1a, Task 7.4c |
| D10 `--remote_cache_compression` | applied at Task 0.2b, Task 7.4a, ADR-004 |
| D11 `--local_resources` names | applied at Task 0.2b |
| D12 heap from wrapper vs `~/.bazelrc` | applied at Task 0.2f, Task 1.6c, Unresolved Questions |
| D13 `io_nice_level`, `batch_cpu_scheduling`, cgroup | applied at Task 0.2b, Task 0.2f, Task 0.10g (E7) |
| D14 `--shutdown_on_low_sys_mem` | applied at Task 0.2b, Task 0.2f |
| D15 absolute `--output_base`, `.bazelignore` | applied at Task 0.2a, Task 0.2f, Task 0.10a |
| D16 orphan server kill via pid file | applied at Story 11.4 AC, Task 11.4a |
| D17 install-base report | applied at Story 11.4 AC, Task 11.4a |
| D18 execution-log tooling | applied at Task 0.10c, Task 0.10e, Task 7.4c, Task 7.7a |
| D19 documented lockfile merge driver | applied at Task 0.10b (exact snippet TO-VERIFY), Task 0.10d, Task 11.2a |
| D20 known-absent flags | applied at header correction 6, Task 0.2b, Task 0.2g |
| D21 no `--profile=<path>` in defaults | applied at Task 0.2g, Task 1.3b, Task 7.4c |
| D22 local-cache poisoning by host cc | applied at Task 0.7b, Task 3.3a, ADR-004 |
| D23 no cross-server dedup, queue rationale | applied at Task 0.2f, Task 0.10c, Task 0.10g (E11) |
| E1, E2, E3, E4 | applied at Task 0.10e; S6 PASS iff |
| E5, E6, E10 | applied at Task 0.10f; S6 PASS iff |
| E7, E11 | applied at Task 0.10g (non-gating inputs) |
| E8 | applied at Task 0.12f; S10 PASS iff |
| E9 | applied at Task 0.2g (fixture test) |
| X1 `--incompatible_sandbox_hermetic_tmp` absent (research section 2.5, no delta id) | applied at Story 0.9 AC, Task 0.9b, Task 4.1b |
| Research 2.3 `--experimental_output_paths=strip` | follow-up: only evaluated if E3 fails (Fallback matrix S6) |
| Research 2.2 `bazel vendor` | follow-up: rejected for this repo (a large vendor directory times ~100 worktrees); shared repository cache kept |
| Research 2.5 `--sandbox_base`, `--experimental_inmemory_sandbox_stashes` | follow-up: leave defaults unless profiling shows sandbox setup dominating |
| Research 2.4 `--experimental_remote_repo_contents_cache` | follow-up: not recommended (experimental, no gain on one box) |
| Multiplex-worker inventory (user-requested, https://bazel.build/remote/multiplex) | applied at Task 0.10h (non-gating sub-check of S6), validation.md |

## Epic 0 spike corrections (applied 2026-11-03)

Source: `spike-results.md` and `epic0-seal.md` on branch `sdd/bazel-epic0`. Each row is a plan-text edit; none changes scope.

| Correction | Where applied |
|---|---|
| `@rules_go//`, not `@io_bazel_rules_go//` (bzlmod) | Story 0.8 AC, Task 1.1c |
| `--disk_cache=` (empty), not `--nodisk_cache` | Task 0.2b, Task 0.10e (E4) |
| `proto_library` loads from `@protobuf//bazel:proto_library.bzl` | Task 0.3a |
| `toolchains_protoc` unnecessary (prebuilt protoc already resolved) | Task 0.3f |
| No `UpsertRule` symbol; evidence is `OnConflict*` builders (46 vs 0) | Tech Debt Disposition, S2 gate, Story 0.4 AC, Story 2.3 |
| Package list is generated `packages.bzl`, not `packages.txt` | Task 0.4b |
| Generated ent libraries are `//tools/bzl/ent:*`; `//session/ent/...` builds only `schema` | S2 gate, Story 0.4 AC |
| `make test PKG=` does not exist today; baselines use `go test ./config`; Task 8.2e introduces `PKG` | T4 row, Task 0.1f, Task 7.5g |
| Task 1.1a fails on any Gazelle `matches multiple rules` line | Task 1.1a |
| `bazel query` rejects `--config=agent`; wrapper must not add it for `query` | Task 1.6b |
| Delete `.bazel-out` only after `chmod -R u+w` | Task 0.2f |
| `tools/bazel` needs `BAZEL_REAL`; not directly invocable | Task 0.2f |
| `.bazelignore` also lists `web-app/node_modules`, `web-app/out` | Task 0.2a |
| `--config` chains are not additive for `tags` (last wins) | Task 0.8a |
| Task 4.1b: no global `TMPDIR` or `STAPLER_SQUAD_TMUX_SOCKET` test env; HOME fixture with `.gitconfig`; tmux/gh on test PATH | Task 4.1b, pre-mortem #13 guard |
| Story 0.9: private short `/tmp` is the Bazel 9.3.1 default; `--test_env=TMPDIR` is ignored | Story 0.9 AC |
| Task 0.13a verify path was `//tools/bzl/spike-nogo:seed`; nogo needs per-analyzer twin libraries (built in Story 8.4, spike dir removed) | Task 0.13a |

Follow-ups F6-F11 of the triad disposition are applied (see the follow-ups list there). Open owner decisions (S13, S4, Story 8.4 and others) are in `epic0-seal.md`.

## Selective e2e disposition (ADR-009, owner directive 2026-11-02)

| Item | Disposition |
|---|---|
| Owner directive: only changed dependencies re-run their tests, e2e included | fixed at ADR-009, spike Story 0.15 (S14), Epic 12 (Stories 12.1-12.7) |
| Measured reason (RA2 0.987; e2e 1974 s of 2601 s) | recorded at the RA2 paragraph, Task 0.14e (EG5 re-expressed over the post-selection projection) |
| ADR-005 e2e position | amended by ADR-009 and a pointer in ADR-005; ADR-005 never listed Playwright as a permanent exception, the plan only kept it outside the graph (P7, Task 7.6b) |
| Improvement target for e2e minutes avoided | fixed at T8 (Improvement targets table), P10 row and Task 7.5g, collector Task 12.4d |
| Registry as slice source | rejected as sole source (measured: 52.8% of specs resolve to an existing source file, about 92% of code commits would fall back to run-all); kept as seed and cross-check, Task 12.5a |
| Soundness, flakes, escapes, cache poisoning | fixed at Story 12.5, ADR-009 "Soundness, flake and trust policy", ADR-004 write rules reused (Task 12.5d) |
| Risk that the goal is unattainable | recorded at ADR-009 Consequences and the S14 verdict mapping: if core packages dominate every slice the replay median stays high; the fallbacks are diff-line coverage selection or leaving e2e as a full run |

## Package scoping disposition (ADR-010, owner request 2026-11-02)

| Item | Disposition |
|---|---|
| Owner request: refactors to make the large packages smaller and more scoped | fixed at ADR-010, Epic 13 (Stories 13.1-13.7), improvement target T9; `requirements.md` Out of Scope amended for behaviour-preserving moves only |
| Measured reason | `research/package-scoping.md`: two packages hold 186k of the non-generated LOC; `session` 29 library dependents and 106 dependencies; `server/services` 5 dependents; 15 test files in 6 packages invert the layering |
| RA1 value | annotated, not rewritten (RA1 refinement bullet under "Sampling, proxies and risky assumptions"): the 0.2657 median is partly produced by test edges; target-model value 0.053; T1 evaluated on the target model pending the owner's answer |
| What splitting can buy | stated up front: Tier 0 moves the mean affected test-LOC share 55.4% to 44.9%; Tiers A and B that pass SC1-SC6 move the mean affected library-LOC share 40.5% to 38.5% and shrink the two largest packages by about a quarter; eleven of fifteen Tier A clusters are not split because they change no measured number |
| Count-based fan-out | rises after upper splits (target model 0.053 to 0.072; merged proxy 0.266 to 0.297), by design; T9e keeps T1 below 0.30 |
| Gating | Epic 13 does not gate Story 7.6, Story 8.6, Epic 10 or Epic 12; T9 enforced only at Story 13.7; Stories 13.4 and 13.5 wait for the S14 verdict, the Story 12.1 manifest and the owner |
| Concurrent worktrees | Task 13.1e (conflict check against open PRs and 120 registered worktrees, ledger, backlog notice), one cluster per PR, at most 3 open, `git mv` plus codemod |
| Existing decisions | `docs/architecture-audit-session-package-split-2026-09-07.md` and the backlog-extraction design adopted for Story 13.5 (Task 13.5a records every deviation); `core-domain-decomposition` and `backlog-service-refactor` coordinated through the conflict check; Tier C stays deferred |
| Not done here | interface extraction for `Instance`, `EntRepository`, `Storage`; renaming or reshaping any public API; splitting `config`, `server`, `session/tmux`, `server/mcp`, `session/unfinished`, `github`, `session/git`, `session/tymux` (measured, rejected in the research file section 6) |

## Closeout reconciliation (2026-11-06, WP1 docs closeout)

Every row below was checked against the tree at `43933064f` on `sdd/bazel-epic0` (commands in the right-hand column were run).

### Spike verdicts moved on since the 2026-11-03 corrections

| Spike | Was (`epic0-seal.md` interim) | Now (`spike-results.yaml`) | Evidence |
|---|---|---|---|
| S13 | `Fail` (owner-gated experiment not run) | `FallbackWorkaround`: commit lock + client merge driver + CI auto-regen + required check; GitHub merges ignore the driver (8 of 8 conflicts stayed conflicts), 32 pairs, 32 green with regen | `spike-results.md` S13 "Verdict" |
| S14 | not run | `Fail`; owner chose "Full e2e run" (2026-11-05); ADR-009 Accepted with that outcome; T9f N/A | `owner-attestations.yaml` `adr_009_e2e` |
| S10, S10-redaction | not run | no verdict; WAIVED (`spike_waivers`, granted 2026-11-06, expires 2027-02-04, `needs-owner-reconfirm`) | `python3 tools/bzl/check-spike-results.py` prints `WAIVED S10` and `WAIVED S10-redaction` |
| S6 | not run | row missing; pending measurement (Story 0.10) | same command prints `ERROR: S6: missing row` and `RESULT: FAIL` |
| S15, S15b | not present | `FallbackWorkaround` (S15 superseded by S15b) | `spike-results.yaml` |
| S2 importer check | subset only | closed: `//session/... //server/...` 253 of 253 built, 5 of 5 tests | `spike-results.md` "Importer completion check (S2/S11)" |

Row count: `grep -c '^- spike:' implementation/spike-results.yaml` prints 16 (the seal's "13 rows" is stale). Structural counts above in the header (14 epics, 88 stories, 270 tasks) were recounted from the headings of this file: 14 distinct `## Epic N:` headings, 88 distinct `Story N.M` headings, 270 distinct `Task N.M[a-z]` headings. The raw-token and CU figures were not recomputed in this pass (UNVERIFIED).

### Verify-command names that differ from the tree

The plan names shell or Python scripts that were implemented as Go programs (repo preference: Go for new tooling). No wrapper scripts are added; read the task text through this table. Invocation shown is `go run`; the same package builds under Bazel as `//tools/bzl/parity/<name>`.

| Plan text | In the tree | Checked by |
|---|---|---|
| `scripts/bazel-manual-instance.sh` (P9, Tasks 7.5e, 9.1c) | `tools/bzl/manual-instance` (Go): `--dry-run`, `--smoke`, `--stop`; ports 62871/62872, instance `claude-manual-test`, socket `ssq-manual-1` | `go run ./tools/bzl/manual-instance --dry-run` printed the build, copy, start and stop commands |
| `tools/bzl/parity/run-all.sh` | `tools/bzl/parity/run-all` (Go), flags `--dry-run`, `--attestations`, `--waivers`, `--today`, `--bazel-bin` | `go run ./tools/bzl/parity/run-all --dry-run \| grep -cE '^P([1-9]\|10): '` printed 10 |
| `tools/bzl/parity/fresh-clone.sh` | `tools/bzl/parity/fresh-clone` (Go) | directory listing only; not executed (heavy) |
| `tools/bzl/parity/e2e.sh` | `tools/bzl/parity/e2e-compare` (Go; judges the outputs of the Playwright workflow step) | `ls`; not executed |
| `tools/bzl/parity/perf.sh` | `tools/bzl/parity/perf` (Go): subcommands `warm`, `disk`, `worktree`, `ttg`, `cache`, `check` | header comment of `tools/bzl/parity/perf/main.go` |
| `tools/bzl/parity/improvement.sh`, `edit-build-test.sh` | `tools/bzl/parity/improvement` and `tools/bzl/parity/collect-improvement` (Go; edit-build-test is `MeasureT4` in `collect-improvement/rest.go`) | `ls`, `grep` |
| `tools/bzl/parity/check-waivers.py`, `check-waivers_test.py` | `tools/bzl/parity/check-waivers` (Go) and `tools/bzl/parity/waivers` (library) | `go run ./tools/bzl/parity/check-waivers` exits 0 |
| `tools/bzl/parity/e2e-avoided.sh` (T8) | `tools/bzl/e2eavoided` (Go); T8 is moot while S14 is `Fail` | `head tools/bzl/e2eavoided/main.go` |
| `tools/bzl/parity/scoping.sh` (T9) | `tools/bzl/scoping` (Go): `go run ./tools/bzl/scoping t9` | run; output recorded in ADR-010 |
| `replay.py`, `tools/bzl/scoping/replay.py` (T9, Tasks 13.1b/13.1f) | `tools/bzl/scoping/replay.go` (Go) | `ls tools/bzl/scoping` |
| `testdata/t8-pass.json`, `t9-pass.json`, `t9-fail.json` | not present; `tools/bzl/parity/testdata/` holds `improvement-fail.json`, `waiver-expired.yaml`, `waiver-ok.yaml` | `ls` |

Still shell in the tree and used as named: `tools/bzl/parity/compare-proto.sh`, `compare-ent.sh`, `compare-web.sh`, `compare-binary.sh`, `shard-union.sh`, `test-set-diff.sh`. `tools/bzl/check-spike-results.py` and `tools/bzl/economic-gate.py` exist as Python, as planned. `scripts/verify-manual-instance.sh` (Makefile target `verify-manual-instance`) is a separate, older guard and is not the P9 tool.
