# ADR-010: Behaviour-preserving package scoping refactors, chosen by measured payoff and sequenced after the Gazelle graph exists

**Date**: 2026-11-02
**Status**: Proposed (owner request 2026-11-02: "include refactors to make these packages smaller and more scoped"; becomes Accepted when the owner confirms the "ADR-010" Unresolved Questions in `implementation/plan.md`). Amends `requirements.md` Out of Scope ("rewriting application code beyond what hermeticity requires") for behaviour-preserving moves only. Adds improvement target T9. Related: ADR-009 (risk 1: `session/`, `server/` and `config/` may sit in nearly every spec slice), ADR-001 (package-level targets from Gazelle), `docs/architecture-audit-session-package-split-2026-09-07.md` (not contradicted, see "Existing decisions").
**Context**: under Bazel a Go package is the unit of dependency and of cache invalidation; under ADR-009 it is also the unit of an e2e slice. Two packages hold 186k of the module's non-generated LOC (`server/services` 93,118, `session` 92,248; the next is 11,116) and 187 of the last 500 `main` commits touched a `.go` file in one of them (126 and 119 individually).

## Evidence

Full tables, method and reproduction scripts: `research/package-scoping.md`, `research/package-scoping-clusters.json`, `research/scoping-tools/`. All numbers recomputed in this pass on `14b02fd65`.

- **The baseline RA1 number mixes test edges into library edges.** Reproduced: median 0.2587 over 30 PRs, 0.2657 over the 17 Go-touching PRs. 62 of 143 packages have exactly 37 reverse dependencies, because `session`'s external tests import `server/services`, `server/auth` and `pkg/events`, each of which imports `session`. With tests modelled as Bazel models them (leaf targets), the same PRs affect a median 0.053 of all targets (14 of 264), so the count-based T1 bound (0.30) is not in danger. The cost-weighted footprint is: the median PR affects 32% of library LOC and 62% of test LOC, because the packages that change are the large ones.
- **15 test files (3,583 LOC) in 6 packages invert the layering** (12 package pairs). They are external test packages and reference no unexported symbol. Neither depguard (`no_server_in_core` exempts `_test.go`) nor `deps_guard_test.go` (bans two external modules only) covers them. Moving them lowers the mean affected test-LOC share from 55.4% to 44.9% over the last 500 commits, and 92.6% of Go-touching commits then affect less test LOC (replay, `research/package-scoping.md` section 5).
- **Package size is a weaker lever than the request assumes.** The cores of `session` (173 of 315 files, 64,331 of 92,248 LOC) and `server/services` (142 of 319 files, 55,019 of 93,118 LOC) are one strongly connected component because one receiver type binds dozens of files (`Instance` 38 files, `SessionService` 39, `BacklogService` 25, `EntRepository` 15). Purely mechanical extraction (above/independent clusters of at least 3 files) covers 11,212 LOC of `session` and 10,507 LOC of `server/services`, and five of the eight `session` clusters had zero commits in the last 500. Replayed together with the test fix, the mean affected library-LOC share moves only from 40.5% to 40.1%.
- **Near-mechanical extraction (Tier B) is where size actually drops.** Groups whose only inbound references from the rest of the core are at most 10 declarations (usually interfaces or callback types) become dependents after those declarations move. Peeled: `session` -26,102 LOC (backlog/workflow/liveness engine 32 files 14,681 LOC; agent adapters and review queue 21 files 6,903 LOC), `server/services` -27,734 LOC (`BacklogService` 28 files 15,651 LOC for only 10 relocated declarations, websocket transport 6 files 5,385 LOC). Replayed with Tiers A and the test fix: largest packages 54,934 and 54,877 LOC, mean affected library-LOC share 40.5% to 38.6%, 15.1% of changes affect less, mean test-LOC share 55.4% to 43.0%, and the share of Go commits touching a package assumed to be in every e2e slice drops from 70% to 57% (last 500).
- **The criteria keep almost all of the benefit.** Tier 0 plus the four Tier A and five Tier B clusters that pass SC1 to SC6 (replay): mean affected library-LOC share 40.5% to 38.5% (all clusters: 38.6%), test-LOC share 55.4% to 43.5%, 14.8% of changes affect less, `server/services` 93,118 to 67,445 LOC and `session` 92,248 to 68,883 LOC, universal-slice touch share 71% to 61% (last 100) and 70% to 58% (last 500). Per-cluster marginal gains are small (0.0 to 0.6 percentage points of lib LOC); the sum is what matters.
- **Count-based fan-out gets worse, by design.** Upper packages add dependents to the core: the target-model share rises from 0.053 to 0.072 (17 PRs; 0.156 if every cluster is split) and the RA1 merged proxy from 0.266 to 0.297 (0.450 if every cluster is split). A target that counts targets would punish exactly the change that reduces cost, so T9 is cost-weighted and T1 stays on the target model.
- Packages measured and rejected: `config` (16 of its 17 last-500 commits touch `config.go`, the hub file with 24 importers), `server`, `session/tmux`, `server/mcp`, `session/unfinished`, `github`, `session/git`, `session/tymux`, `server/prwatch` (section 6 of the research file).

## Decision

1. **Refactors are in scope, as Epic 13, behaviour-preserving only.** Allowed: move files between packages; capitalise a symbol that crosses a new boundary; relocate a declaration (interface, callback type, constant) to break a back-edge; move test files to break a layering inversion. Not allowed without a new ADR: renaming or reshaping a public API, changing a signature, changing runtime behaviour, adding an abstraction that has one implementation and no consumer-side need (`interface-pollution-checklist`), introducing a new dependency, or moving `Instance` fields.
2. **Selection criteria.** A cluster is split only if all hold, evaluated by the harness of Task 13.1c on the then-current tree:
   - SC1 boundary budget: at most 10 declarations to relocate from the rest of its package into the core, and no new import cycle (the harness reports cycles before any file moves). **Lower packages** (packages the core `session` imports; amended 2026-11-05): SC1 counts the declarations relocated into the core, not the core's uses of the cluster, because a lower package's "uses" are just its callers.
   - SC2 size: at least 3 files and 400 LOC, and the new package does not exceed 25% of the original's LOC (the largest Tier B component, 15,651 LOC, is 17% of `server/services`).
   - SC3 isolation: at most 5 importing packages outside the original. **Lower packages** (amended 2026-11-05): a lower package's natural callers (importers that already imported the original package) are not counted; only new coupling is. Implemented by `scoping criteria --lower` (`tools/bzl/scoping/criteria.go`).
   - SC4 invalidation relevance: either its files appear in at least 3 of the last 500 `main` commits **and** some of those commits touch only its files, or it has no import edge to the rest of its package (an independent cluster stops being invalidated by every change to the core, which is where the benefit comes from; this is why `sessionbundle` and `devtools` pass although they almost never change). Once the manifest exists, SC5 may substitute for SC4.
   - SC5 slice dilution (once `tests/e2e/slice-manifest.json` exists, Story 12.1): the cluster's packages are absent from at least 50% of spec slices, or the covered share of its files in the median spec is at most 30%.
   - SC6 materiality, predicted by the harness for that cluster alone, at least one of: mean affected library-LOC share lower by at least 0.10 percentage points; mean affected test-LOC share lower by at least 0.30 points; at least 3% of the replayed changes (last 100 or last 500) affect less library LOC; universal-slice touch share (section 5 of the research file) lower by at least 1.0 point over the last 500; and always a target-model share below 0.30.
   Test back-edge moves (Story 13.2) are exempt from SC2 to SC6: they are unconditional because each move is test-only.
   Applied to today's tree (`research/package-scoping.md` section 5, per-cluster table): of 15 Tier A clusters four pass (`server/services/wsbridge`, `sessionbundle`, `devtools`, `session/external`), and of the Tier B components five pass (`session` p0, p1, p2; `server/services` p0, p2). The other eleven Tier A clusters would not change a measured number and stay where they are.
3. **Tiers.** Tier 0 = test back-edges (Story 13.2). Tier A = purely mechanical clusters (Story 13.3). Tier B = near-mechanical peeled components (Stories 13.4 for `server/services` and 13.5 for `session`; each starts with a design confirmation task). Tier C = seam work for the remaining core (`Instance`, `EntRepository`, `Storage`): **not planned here**; the design lives in `docs/architecture-audit-session-package-split-2026-09-07.md` items 3 and 4 and is recorded as a follow-up, because it needs interface extraction that this ADR forbids doing mechanically.
4. **Constraints on every split.**
   - One cluster per PR and per Task; at most 3 open split PRs at once.
   - `git mv` plus a codemod (Task 13.1d), never a hand rewrite; `go build ./...`, `go vet ./...` and `go test` of the moved and importing packages pass before the PR is opened; `bazel run //:gazelle` leaves no diff once Gazelle is wired (Story 1.1); the Story 4.8a test-set diff shows no missing test.
   - Conflict control with the live worktrees (120 registered in this checkout, VERIFIED `git worktree list | wc -l`): Task 13.1e publishes the move list to the backlog and a freeze notice before the PR is opened, runs `git log --since=24h --name-only -- <moved files>` and `gh pr list --search "<moved file> in:files"` to find open touches, and the move lands within one working session. Git follows renames in `merge`, and `rerere.enabled=true` is already set; agents rebasing an old branch onto a moved file get a rename-aware merge. Open-PR files in the cluster make the Task wait or coordinate, not skip.
   - Dependency guards move with the code: `.golangci.yml` `no_server_in_core` globs and any `depguard` rule naming a moved path are updated in the same PR, and Task 13.2c adds the test-inversion check so Tier 0 cannot regress.
   - `instance-lock-free-reads.md`: a moved file reads `Instance` state only through `Snapshot()`, `Workspace()` or an exported accessor; the compile error from the new package boundary is the enforcement.
   - No generated code is committed; the ent and proto surfaces are not moved.
5. **Sequencing.** Epic 13 starts after Story 1.1 (Gazelle BUILD files exist, so a split is `git mv` plus `bazel run //:gazelle`) and Story 1.2. Story 13.1 (harness, baseline) and Story 13.2 (Tier 0) can start then. Story 13.3 (Tier A) waits for Story 13.1 and for a `main` with no open `core-domain-decomposition` or `backlog-service-refactor` PR touching the cluster's files. Stories 13.4 and 13.5 (Tier B) wait for the Story 0.14 S14 verdict, the Story 12.1 manifest (SC5 data) and the owner's confirmation of the Unresolved Questions. Story 13.7 closes the epic by recomputing T9.
6. **What is gated and what is not.** Epic 13 does **not** gate the CI cutover (Story 7.6), the `BUILD_SYSTEM` flip (Story 8.6), `make` removal (Epic 10) or Epic 12: the end state works without it, with worse selectivity. T9 is reported in the parity report (Task 13.1f) and enforced only at Story 13.7. Epic 13 depends on nothing in Epic 8 to 10 and can run beside them. Story 12.4's replay uses whatever package layout exists when it runs; it is re-run at Story 13.7 to measure the change in selected share.
7. **Alternative recorded (no refactor).** ADR-009 builds slices from observed coverage at package granularity. A file-level slice (observed source files of a package, plus the package's `init`-bearing files and its non-test dependencies) would give some of the selectivity without moving code, at the cost of soundness for same-package edits that change compilation of unobserved files. It is not adopted here; the S14 verdict can adopt it independently, and SC5 is computed against whichever granularity the manifest uses.

## Improvement target T9 (proposal; values fixed at Task 13.1f from the then-current baseline, owner-confirmed)

All components are measured by the replay harness (`tools/bzl/scoping/`) over the last 500 and last 100 `main` commits at the commit that creates the baseline, then re-measured at each story:

| Component | Baseline (this tree) | Proposed target | Predicted by the harness (selected set) |
|---|---|---|---|
| T9a mean affected test-LOC share, last 500 | 55.4% | at most 46.0% after Story 13.2; at most 45.0% at close | 44.9% after 13.2; 43.5% at close |
| T9b share of Go production commits touching a universal-slice package, last 100 / last 500 | 71% / 70% | at most 64% / 61% at close (report-only before) | 61% / 58% |
| T9c mean affected library-LOC share, last 500, and share of changes affecting less than baseline | 40.5%, 0% | at most 39.5% and at least 12% at close | 38.5% and 14.8% |
| T9d size of the two largest packages and test inversions | `server/services` 93,118, `session` 92,248 LOC; 12 inverted pairs | each at most 72,000 LOC at close (90,000 if Tier B is not executed); inversions 0 | 67,445 and 68,883 LOC |
| T9e guard: T1 on the target model | 0.053 (17 PRs) | at most 0.30, every story | 0.072 |
| T9f median e2e slice size per spec (LOC of the slice packages), once Story 12.1 exists | measured at Task 13.1e | at most baseline minus 0.8 x the harness-predicted reduction for the executed set | n/a until S14 |

The proposed values keep a tolerance of about 1 percentage point on T9a and 3 on T9b around the prediction, and 3,000 LOC on T9d. If a story's measured value misses its prediction by more than that, Task 13.1c's model is recorded as wrong for that cluster and the next story is re-evaluated before it starts; this is a measurement control, not a downscale.

## Existing decisions this builds on, and does not contradict

- `docs/architecture-audit-session-package-split-2026-09-07.md` recommends `session/backlog` first, `session/review` second, and defers `Instance` and persistence to composed sub-objects and repository ports. Tier B is the file-level measurement of items 1 and 2 (`session` components p0 and p1); Tier C is items 3 and 4 and stays out of this epic. `docs/architecture-audit-session-backlog-extraction-design-2026-09-07.md` is the design Task 13.5a adopts.
- `project_plans/core-domain-decomposition` (ADR-001 service-per-cluster) works inside `server/services` at type level; Story 13.1's conflict check keeps the two from editing the same files at once.
- ADR-001 (this project): one `go_library` per package from Gazelle. Splits change the package list only; no hand BUILD files are added (the Starlark surface cap of Story 11.7 is unaffected).
- ADR-009: SC5 and T9f use its manifest; Epic 12 is not blocked and its replay is re-run at close.

## Consequences

- New Epic 13 (7 stories, 28 tasks) and a new decision gate for the owner (which tiers, freeze windows). Epics 0 to 12 are not renumbered and their task ids are unchanged.
- The honest expectation for the cost-weighted payoff is negligible for Tier A on library LOC (40.5% to 40.2% for the four clusters that pass), meaningful for Tier 0 (test-LOC share -10.5 percentage points) and for Tier B (the two largest packages -27% to -26%, library-LOC share 40.2% to 38.5%); the harness is the arbiter, and a cluster that fails SC6 at execution time is recorded as `skipped by criterion`.
- Count-based metrics on the merged graph (the RA1 proxy) get worse after upper splits; the plan's T1 stays defined on the Bazel target model, and the RA1 measurement is annotated, not rewritten (plan.md "Sampling, proxies and risky assumptions").
- Risks: merge conflicts with live worktrees (controls in Decision 4), a split that turns out to need an interface (the SC1 budget and the cycle report catch it before files move), test files that straddle a boundary (10 to 21 per scenario, handled case by case), churn-weighted metrics that look at the past (the replay uses the last 500 commits and is re-run before each story). Rollback of any split is a single revert, because each is a pure move plus renames.

## Measured outcome (2026-11-04, branch `sdd/bazel-epic13-tierb`)

Status is left at Proposed: acceptance is the owner's confirmation of the Unresolved Questions, not an agent's.

- Executed: Tier 0 (12 test inversions moved), `sessionbundle` and `devtools`, `session/prpoller`; plus the tooling (`scoping move|relocate|deps|budget|boundaries`), the budget ratchet and the depguard rule. `scoping t9` on the branch: T9a, T9c and T9e PASS; T9b (universal touch 70.97%, bound 61%) and T9d (`server/services` 91,713 and `session` 90,866 LOC, bound 72,000) FAIL.
- Skipped by criterion, with numbers, in `implementation/scoping-ledger.md`: `session/external`, `wsbridge` and ten of the eleven Tier A re-evaluations (SC6; several also SC1), `server/services` p1, p3, p4, p5 and `session` p3 and p4.
- Passing SC6 but not executed, each blocked on something other than the criteria: `server/services` p0 (cluster move needs a test-support package and 32 test files to move; relocation of the 15 non-test declarations is done), `session` p0 and p1 (519 and 216 core uses, 70 and 60 declarations to relocate), `session` p2 (`NewEntRepository` calls the cluster; needs an injected or registered migration list, which is not behaviour-preserving across 86 call sites).
- Not planned (Tier C): `Instance` 38 files, `EntRepository` 15, `Storage` 11, with 321 and 225 inbound symbols (research section 4).
- Follow-ups worth a backlog item each: a `svctest` test-support package for `server/services`; the relocation commits for `session` p0 and p1; an injected-migrations design for `NewEntRepository`; `server/services` p2 (websocket transport, 27 declarations).

### Update (2026-11-05, branch `sdd/bazel-epic13-tierb2`): `server/services` p0 executed

- `server/services` p0 (the `BacklogService` group, `github_user_*` kept in core) moved to `server/services/backlogsvc`. Method: shared test helpers and the hermetic `TestMain` body extracted to the testonly `server/services/internal/svctest`, the declarations of staying tests that use `BacklogService` split into `*_bsvc_test.go` files that move with the group, a few shared pieces peeled out of the cluster, then `scoping move`. 0 staying declarations use the group (SC1); `scoping t9` before and after: T9a test share 0.4434 to 0.4391, T9c lib share 0.3896 to 0.3852 (-0.44 pp), T9b universal touch 0.7097 to 0.6895 (still FAIL, max 0.61), T9d `server/services` 91,713 to 77,709 LOC (still FAIL, max 72,000), `session` unchanged at 90,866, inversions 0.
- Not done: `session` p0/p1 relocations, `server/services` p2, the `importer` and `session` p4 re-measures. `session` p2 stays rejected (`NewEntRepository` loops over `startupMigrations`; an injected list would change which of its 86 call sites run migrations).
- The depguard rule `scoped_packages_off_ent` is not applied to `backlogsvc`: it imports `session/ent` in two non-test files.

### Update (2026-11-05, branch `sdd/bazel-epic13-session`): `session` p0 executed, p1, p4, importer and p2 not

- `session` p0 moved to `session/backlog` (27 files: the backlog lifecycle listener and its reconcilers, liveness and pipeline engines, review gate, chain firer, stuck decisions). Method: `BacklogController` now holds a `BacklogListener` interface and `EntRepository` a `ChainDispatcher` interface (`WireChainFirer` is a package function over `Storage.Repository()`), 28 declarations the core shares moved to two core files, 44 test files were split into `*_bk_test.go` and moved, free test helpers and the hermetic `TestMain` body went to `session/internal/sessiontest`, bound ones were copied. 0 staying declarations use the cluster (SC1). `scoping t9` before and after: T9a test share 0.4391 to 0.4333, T9c lib share 0.3852 to 0.3802 (-0.50 pp), T9b universal touch 0.6895 to 0.6573 (still FAIL, max 0.61), T9d `session` 90,866 to 77,473 LOC and `server/services` 77,709 to 77,715 (still FAIL, max 72,000). Test function names unchanged apart from three new tests.
- Not executed, with numbers in `implementation/scoping-ledger.md`: `session` p1 (12 core files construct or hold its collaborators; the SC1-compliant 3-file remainder fails SC6 at lib -0.08 pp), `importer` (SC6: lib +0.01 pp), `session` p4 (SC1: the core factory constructs the backends; SC6: lib 0.00 pp), `session` p2 (stays rejected). `scoping move` still loads linux only; p4's windows-only file was moved by hand for the measurement.
- Still Proposed; `targets_confirmed` and the owner attestations are untouched.

### Update (2026-11-05, branch `sdd/bazel-epic13-next`): `wsconn`, `importer` and `ptyio` executed; T9b passes

- `server/services/wsconn` (websocket transport, 18 files, 8.5k LOC), `session/importer` (import commit/correlate/kill/reconcile plus the history linker and watcher, 13 files) and `session/ptyio` (PTY buffer and response stream, a lower package the core imports). `scoping t9` before and after: T9b universal touch 0.6573 to 0.5806 (PASS, max 0.61), T9a 0.4330 to 0.4213, T9c 0.3785 to 0.3695, T9d `server/services` 77,715 to 69,543 (PASS) and `session` 77,473 to 74,306 (still FAIL, max 72,000). Per-move numbers and the criteria each passed are in `implementation/scoping-ledger.md` ("continuation 4").
- Two findings that change how to pick the next cluster. A cluster nothing in the package uses (a leaf) still depends on the core, so a core change rebuilds it and the lib share barely moves (every `session` leaf cluster tried fails SC6); a lower cluster the core imports leaves the core's invalidation set and does move it (`ptyio`, -0.12 pp; `hostclaim`, -0.43 pp). T9b counts replayed changes whose only universal-package files are in the cluster, which `scoping touch` computes.
- `session/hostclaim` was implemented and tested, then reverted: 7 non-test importers outside `session` against SC3's 5. The lower `command_*` package (1.4k LOC) passes SC6 but has 13 staying uses against SC1's 10. Together they would take `session` to about 71.2k and close T9d; for a lower package SC1 and SC3 count the core's callers rather than relocations or rewrite blast radius, which is an owner decision on how to read them. Still Proposed; `targets_confirmed` and the owner attestations are untouched.

### Update (2026-11-05, branch `sdd/bazel-owner-decisions`): owner decision on SC1 and SC3 for lower packages

- Changelog, 2026-11-05: owner decision via AskUserQuestion, recorded in `implementation/owner-attestations.yaml` as `adr_010_lower_package_criteria`: "SC1 counts declarations relocated into the core (not callers' uses); SC3 does not count a lower package's natural callers as importers" for lower packages (packages the core `session` imports). This reads the "uses" and "importers" of a lower package as its callers rather than coupling; upper clusters keep the original SC1 (staying uses) and SC3 (all importers). The ADR stays Proposed; `targets_confirmed` is untouched.
- Applied the same day: `session/hostclaim` and the lower `session/cmdexec` (`command_*`) were moved. `scoping t9` after: T9a 0.4158, T9b 0.5806, T9c 0.3637, T9d PASS (`server/services` 69,546, `session` 71,252 against 72,000, inversions 0), T9e PASS, T9f N/A (ADR-009 "Full e2e run"). Rows in `implementation/scoping-ledger.md`.

## Reconciliation: predicted versus measured T9a-e (Tasks 13.7a/13.7b, 2026-11-06)

Measured by `go run ./tools/bzl/scoping t9` on `sdd/bazel-epic0` at `43933064f`, compared with `tools/bzl/scoping/baseline.json` (tree `145bce76b`, 378 Go-touching changes, 17 PRs). Output, verbatim:

```
T9a: PASS (test_loc_share=0.4158 max=0.45)
T9b: PASS (universal_touch_500=0.5806 max=0.61)
T9c: PASS (lib_loc_share=0.3637 max=0.395 lower=0.553 min=0.12)
T9d: PASS (services=69546 session=71252 inversions=0 max=72000)
T9e: PASS (target_share=0.0501 max=0.30)
T9f: N/A (owner decision 2026-11-05: Full e2e run, ADR-009; no e2e selection, so no slice size)
```

| Component | Baseline (`baseline.json`) | Predicted at close (table above) | Measured | Bound | Result |
|---|---|---|---|---|---|
| T9a test-LOC share, last 500 | 55.4% | 43.5% | 41.58% | at most 45.0% | better than predicted by 1.9 pp |
| T9b universal-slice touch, last 500 | 70.97% | 58% | 58.06% | at most 61% | matches |
| T9c mean lib-LOC share | 39.45% | 38.5% | 36.37% | at most 39.5% | better than predicted by 2.1 pp |
| T9c changes with lower lib LOC | 0% | 14.8% | 55.3% | at least 12% | far above prediction (see note 2) |
| T9d `server/services` / `session` LOC | 93,019 / 92,060 | 67,445 / 68,883 | 69,546 / 71,252 | each at most 72,000 | 2.1k and 2.4k LOC above prediction, inside the 3,000 LOC tolerance |
| T9d test inversions | 12 | 0 | 0 | 0 | matches |
| T9e target-model share | 4.43% | 7.2% | 5.01% | at most 30% | below prediction |
| T9f | n/a | n/a | N/A | n/a | ADR-009 "Full e2e run" (`owner-attestations.yaml` `adr_009_e2e`) |

Notes (each cites its source):

1. The baseline values here differ from the ADR's Evidence section (T9c 40.5%, T9e 0.053) because the harness baseline was recomputed on tree `145bce76b`; the Evidence section used `14b02fd65`. T9a and T9b baselines agree.
2. The "changes with lower lib LOC" figure moved from 12.7-14.8% in `implementation/scoping-ledger.md` (continuation rows through `session` p0) to 55.3% at the final tree. The ledger does not record the intermediate values after continuation 4, and this pass did not re-derive the cause; treat the jump as UNVERIFIED. It clears the 12% bound either way.
3. Bounds were met only after the 2026-11-05 owner decision on SC1 and SC3 for lower packages (`adr_010_lower_package_criteria`), which let `session/hostclaim` and `session/cmdexec` move (`scoping-ledger.md` "Epic 13 owner decisions"). T9d `session` was 74,306 LOC (FAIL) before it and 71,252 after.
4. Per-cluster predicted versus measured, and every cluster skipped by criterion with its numbers, are in `implementation/scoping-ledger.md` ("Predicted versus measured (Task 13.7a)" and continuations 1-4). Predictions that missed: `session/external` and `wsbridge` (-0.18 / -0.15 pp predicted, -0.04 / -0.05 pp measured, both skipped by SC6); `session/prpoller` was not predicted to pass and measured -0.22 pp.

### Tier C follow-up (Task 13.7b)

Tier C is recorded here as a follow-up, not planned in this epic. Remaining core, from `scoping-ledger.md` and Decision 3: `Instance` (38 files), `EntRepository` (15), `Storage` (11), with 321 and 225 inbound symbols. `session` `storage*` (8 files, 4.0k LOC) and `ent_*` (22 files, 9.6k LOC) pass SC6 (`ent_*`: lib -0.134 pp, test -0.188 pp, universal -1.2 pp) but fail SC1 with 14 staying uses each; `session` p2 (startup backfills) stays rejected because `NewEntRepository` itself loops over `startupMigrations`, which is not behaviour-preserving across its 86 call sites. The design lives in `docs/architecture-audit-session-package-split-2026-09-07.md` items 3 and 4 and needs interface extraction, which this ADR forbids doing mechanically. Other follow-ups already listed above: `session` p1 (needs collaborators injected), `server/services` p2 (websocket transport, 27 declarations; `wsconn` since moved, see the 2026-11-05 update).

### Status

Stays **Proposed**. `owner-attestations.yaml` `adr_010_policy` records an execution policy (all tiers may be executed by agents, agents merge when CI passes, characterization tests required before Tier B/C moves, no freeze windows), not acceptance of the Unresolved Questions; `targets` there is `recorded` with `targets_confirmed` unset. Acceptance remains the owner's confirmation.
