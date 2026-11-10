# Package scoping: measurements behind ADR-010

**Date**: 2026-11-02. **Tree**: `sdd/bazel-conversion-plan` at `14b02fd65` (code identical to `main` at `33f46bb21`). Everything here was computed from `go list`, `go/packages` type information and `git log`; no repo code was changed. Reproduction scripts and the two Go extractors are in `research/scoping-tools/` (hard-wired to `SCOPE_DIR` and `REPO_ROOT`; Task 13.1 ports them to `tools/bzl/scoping/`). Machine-readable cluster lists: `research/package-scoping-clusters.json`.

Labels: VERIFIED = a command was run and its output read in this pass; INFERRED = reasoned from verified numbers.

## 0. Method in five commands

```
go list -e -json ./... > golist.json                      # 197 packages: 143 non-generated + 54 generated (gen/*, session/ent/*)
git log main -500 --name-only --pretty='@@%H' > gitlog500.txt    # churn source (first 100 = "last 100")
astdump  <repo>  > files.json                             # per file: LOC, imports, top-level decls (go/parser)
typed    <repo>  > edges.json                             # per use: (using file) -> (declaring file), exported?, kind (go/packages + go/types, 166,893 distinct edges, 36 s)
python3 sim2.py                                           # rebuilds the package graph from file-level edges; identity rebuild == go list graph (0 differences of 143 packages)
```

The file-level graph is built from type-checked uses, not name matching. It was cross-checked: rebuilding the package graph from it with no split reproduces the `go list` Imports/TestImports graph exactly (VERIFIED: `identity rebuild diffs 0 of 143`).

## 1. The RA1 number needs a correction before it is used to judge refactors

RA1 (`tools/bzl/baseline/risky-assumptions.sh`) merges `Imports`, `TestImports` and `XTestImports` into one edge set per package. Reproduced here on the same 30-PR sample (`tools/bzl/baseline/pr-sample.json`): median over all 30 = **0.2587**, over the 17 Go-touching PRs = **0.2657** (VERIFIED, equals `measured.json`).

In Bazel a test is a separate target that nothing depends on, so a test-only import edge does not propagate to the importing package's dependents. The merged graph lets a test edge act as a library edge, and that is what produces the 37-package floor:

- 62 of 143 packages have exactly 37 reverse dependencies (histogram top bucket) and 113 have 37 or more. 37 is the cone of `session` (29 libraries by library edges alone plus 8 more through test edges).
- `session`'s external test packages import `server/services`, `server/auth` and `pkg/events`, all of which import `session`. Through those test edges, anything that depends on `server/services` (for example `server/deliverygate`) lands in `session`'s cone.
- Library-only (Bazel target model: libraries propagate along `Imports`; a test target is affected when its own package or any library it imports is affected): the same 17 Go-touching PRs have median **0.053** of all targets (14 of 264), and the median PR affects 32% of library LOC (LOC-weighted proxy for compile cost) and 62% of test LOC (proxy for test run cost).

Consequence: the count-based fan-out is already narrow under the real graph (0.053, far below the T1 bound 0.30). What is wide is the **cost-weighted** footprint, because the packages that change most are the biggest ones. ADR-010 therefore defines T9 on cost-weighted metrics and keeps the count only as a non-regression bound.

## 2. Ranked packages (143 non-generated, root module)

Columns: files = non-test (test) Go files; exported = exported top-level declarations in non-test files; fan-out / fan-in = distinct internal packages imported by / importing it over non-test imports; rdeps (merged) = transitive reverse dependencies in the RA1 graph; rdeps (lib) = transitive reverse dependencies over library edges only; tests hit = test targets affected by a change to the package under the target model; churn = commits among the last 100 / 500 on `main` that touched a `.go` file directly in the package directory. Rank key = rdeps x churn(500).

| # | Package | Files (test) | LOC (test) | Exported | Fan-out | Fan-in | rdeps merged | rdeps lib | Tests hit | Churn 100/500 |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | `session` | 315 (362) | 92,248 (110,899) | 3,335 | 93 | 21 | 37 | 29 | 29 | 21 / 126 |
| 2 | `server/services` | 319 (408) | 93,118 (125,911) | 2,042 | 61 | 5 | 37 | 5 | 8 | 18 / 119 |
| 3 | `server` | 49 (61) | 9,919 (10,681) | 125 | 54 | 1 | 37 | 1 | 3 | 6 / 50 |
| 4 | `config` | 17 (17) | 5,106 (4,712) | 310 | 5 | 25 | 56 | 48 | 49 | 3 / 17 |
| 5 | `session/tymux` | 21 (29) | 4,581 (7,541) | 89 | 7 | 4 | 37 | 30 | 30 | 4 / 24 |
| 6 | `session/git/backend` | 17 (11) | 2,444 (2,272) | 320 | 3 | 13 | 59 | 45 | 44 | 0 / 15 |
| 7 | `session/tmux` | 37 (46) | 10,209 (13,106) | 234 | 8 | 13 | 51 | 40 | 42 | 7 / 16 |
| 8 | `session/git/backend/cli` | 6 (7) | 1,490 (1,864) | 49 | 3 | 5 | 59 | 42 | 42 | 2 / 13 |
| 9 | `session/git` | 14 (23) | 5,279 (7,200) | 138 | 8 | 7 | 40 | 33 | 33 | 1 / 17 |
| 10 | `server/mcp` | 34 (49) | 11,116 (15,973) | 108 | 12 | 2 | 37 | 2 | 4 | 3 / 16 |

By size (LOC, non-test): `server/services` 93,118; `session` 92,248; `server/mcp` 11,116; `session/tmux` 10,209; `session/unfinished` 10,131; `server` 9,919; `github` 7,381; `session/git` 5,279; `config` 5,106; `server/prwatch` 5,075. Only two packages exceed 12k LOC, and together they are 186k of the module's non-generated LOC; the next package is 8 times smaller.

Where the hubs are (library edges, VERIFIED): `session` has a dependency closure of 106 of 143 packages and 29 library dependents; `server/services` has closure 135 and only 5 library dependents (`.`, `cmd/ssq-hooks`, `server`, `server/mcp`, `server/push`); `config` has 25 direct importers and 48 library dependents.

## 3. Layering inversions in tests (the largest single finding)

12 packages' tests import a package that depends on the package under test (VERIFIED, `test inversions`): `session` -> {`pkg/events`, `server/auth`, `server/services`}; `server/services` -> {`server`, `server/mcp`}; `server/deliverygate` -> {`server/push`, `server/services`}; `session/git/backend` -> {`.../cli`, `.../gogit`, `testutil/gitoracle`}; `session/gitprovider` -> `testutil/gitfake`; `session/scanstate` -> `session`. They live in **15 test files, 3,583 LOC**, all external test packages (`*_test`), none referencing an unexported symbol of its own package (VERIFIED, typed edges): `session` 5 files (1,901 LOC), `server/deliverygate` 3, `session/git/backend` 3, `server/services` 2, `session/gitprovider` 1, `session/scanstate` 1.

`.golangci.yml`'s `no_server_in_core` depguard rule exempts `_test.go` files (`"!**/*_test.go"`), and `deps_guard_test.go` only bans two external modules (`go-m1cpu`, `gopsutil/v3`), so nothing guards these edges today (VERIFIED by reading both).

Effect under Bazel (INFERRED from the model, magnitude VERIFIED by replay): `session`'s `go_test` (362 test files, 110,899 LOC) is re-run whenever anything in `server/services` changes, and `server/services` has the highest churn in the module after `session` (119 of the last 500 commits).

## 4. Where `session` and `server/services` are stuck

File-level reference graph, with all methods of one receiver type bound together (Go forbids splitting a type's methods across packages) and strongly connected components computed on the result:

| Package | Files / LOC | Giant SCC (files / LOC) | Outside the SCC | Receiver types that bind files |
|---|---|---|---|---|
| `session` | 315 / 92,248 | 173 / 64,331 | 142 files, 27,917 LOC | `Instance` methods span 38 files (17,579 LOC); `EntRepository` 15; `Storage` 11; `BacklogLifecycleListener` 9 |
| `server/services` | 319 / 93,118 | 142 / 55,019 | 177 files, 38,099 LOC | `SessionService` 39 files (13,638 LOC); `BacklogService` 25 |
| `config` | 17 / 5,106 | 6 / 2,799 | | `Config` 4 files |
| `session/tmux` | 37 files, 10 in the SCC (6,701 of 10,209 LOC); only 14 commits touched the directory in the last 500, and the files outside the SCC are mixed above/below (11 files, 2,318 LOC) | No split now (fails SC2 and SC4) |
| `server` | 49 / 9,919 | 21 / 6,662 | | `Server` 12 files |

Classification of the files outside the SCC relative to the SCC: **above** (reference the core, nothing in the core references them; extractable as a dependent package with no seam), **below** (the core references them; extractable as a lower package, but this does **not** shrink anyone's reverse-dependency set, because the core still imports them), **independent**. `session`: 61 above / 61 below / 20 independent files; `server/services`: 23 / 86 / 68.

**Tier A (purely mechanical)**: above + independent clusters of at least 3 files. Session: 8 clusters, 49 files, 11,212 LOC; services: 7 clusters, 42 files, 10,507 LOC. Boundary cost is tiny: at most 6 unexported symbols per cluster need capitalising (`wsbridge` 6, the rest 0 to 3). Per-cluster numbers, including files, exported declarations, importer files to rewrite and last-500 churn, are in `package-scoping-clusters.json`.

| Cluster (proposed name) | Files | LOC | Test files | Importer files to rewrite | Unexported -> exported | Churn 100/500 |
|---|---|---|---|---|---|---|
| `server/services/wsbridge` | 12 | 3,315 | 10 | 7 | 6 | 2 / 9 |
| `server/services/insights` (insights + webhook helpers) | 10 | 3,557 | 11 | 4 | 1 | 1 / 7 |
| `server/services/gatestats` | 5 | 457 | 1 | 1 | 2 | 1 / 3 |
| `server/services/devtools` | 3 | 486 | 3 | 1 | 0 | 0 / 2 |
| `server/services/sessionbundle`, `hookctx`, `webhooks` | 3, 3, 6 | 836, 400, 1,456 | 1, 3, 5 | 2, 1, 2 | 0, 0, 1 | 0 to 1 / 0 to 1 |
| `session/backlogplugin` | 11 | 2,749 | 8 | 29 | 3 | 0 / 3 |
| `session/paneprompt` | 7 | 1,433 | 6 | 18 | 3 | 1 / 3 |
| `session/prpoller` | 3 | 1,187 | 8 | 6 | 1 | 0 / 3 |
| `session/importer`, `handoff`, `pipelinemode`, `sweeper`, `external` | 11, 7, 4, 3, 3 | 1,493, 1,476, 1,008, 651, 1,215 | 9, 6, 5, 2, 1 | 14, 10, 14, 5, 14 | 2, 0, 1, 1, 2 | 0 to 1 / 0 to 1 |

Churn alone is not the discriminator, which an earlier draft assumed: five of the eight session clusters were not touched once in the last 500 commits and are correctly left alone, but the two independent services clusters (`sessionbundle`, `devtools`) change rarely and still gain, because core changes (119 commits) stop invalidating them. Section 5.1 gives the per-cluster effect; ADR-010 SC4 and SC6 encode this.

**Tier B (near-mechanical, "peel with a relocation budget")**: a group of files in the core whose only inbound references from the rest of the core are K symbols (almost always an interface, a constant, or a callback type declared next to its implementation) becomes an *above* cluster after those K declarations move into the core. Peeling greedily with K <= 10 per group (VERIFIED, `peel.py 10 session server/services`) finds, after merging weakly connected groups:

| Peeled component | Files | LOC | Last-500 churn | Content |
|---|---|---|---|---|
| `session` p0 | 32 | 14,681 | 23 | backlog lifecycle/commands/review/triage, workflow and liveness engines, review gate (the `session/backlog` target of `docs/architecture-audit-session-package-split-2026-09-07.md`) |
| `session` p1 | 21 | 6,903 | 16 (5 in the last 100) | agent adapters (`claude`, `gemini`, `agy`, `opencode`, `pi`), history linker/transfer, review-queue poller and determiner |
| `session` p3, p4 | 7, 5 | 1,442, 1,249 | 3, 7 | autonomous driver and scroll adapters; process backends |
| `server/services` p0 | 28 | 15,651 | 27 | `BacklogService` (25 files) and `github_user_*` |
| `server/services` p2 | 6 | 5,385 | 15 | ConnectRPC websocket transport and stream-hub rollout |

Together: session -26,102 LOC, services -27,734 LOC of core. Relocation and rename cost per component (VERIFIED, sums over the groups merged into each component; a core symbol used by several groups is counted once per group, so the used-symbol counts are upper bounds): `session` p0 70 declarations to relocate, 543 core-symbol uses of which 42 unexported; `session` p1 60 / 280 / 45; `server/services` p0 (`BacklogService`) only 10 / 39 / 4; `server/services` p2 27 / 44 / 18. The unexported ones must be capitalised, and each relocated declaration is a few lines (usually an interface or callback type). Not purely mechanical (declaration relocation, then rename), but most groups need no new abstraction.

**Tier C (needs a seam; design work)**: the remaining core. Cross-group symbol counts inside `session`'s core (rows use columns' symbols): `Instance` group <-> `EntRepository` group 105 / 195; `Instance` <-> `BacklogLifecycleListener` 90 / 3; `EntRepository` -> `Instance` 223 symbols. These are the persistence port and the god type that `docs/architecture-audit-session-package-split-2026-09-07.md` section "Recommended split" items 3 and 4 already say need composed sub-objects and repository interfaces first. Churn ceiling: of the 112 `session` commits that touched production Go in the last 500, 15 touched only `backlog*` files and 15 only `instance*` files.

## 5. Predicted effect (replay on the Go import graph; no code changes)

Method: assign each file to a new package, rebuild the graph from file-level edges (importers are re-pointed through the declaring file of each symbol they use), optionally delete core -> extracted edges for Tier B (modelling the relocated declarations), then replay the last 500 / 100 `main` commits and the 30 sampled PRs. Affected set per replayed change: libraries by reverse library edges plus test targets (target model of section 1). "Mean LOC share" = mean over replayed Go-touching changes of affected library LOC / total library LOC; "fewer" = share of those changes whose affected library LOC is lower than at baseline. n = 378 (500 commits), 59 (100 commits), 17 (30 PRs). Test-LOC columns use test file LOC as a run-cost proxy.

| Scenario | Packages (lib / with tests) | Mean lib LOC share, 500 / 100 / PRs | Mean test LOC share, 500 / 100 / PRs | Changes with lower lib LOC, 500 / 100 | Changes with lower test LOC, 500 |
|---|---|---|---|---|---|
| S0 baseline | 143 / 121 | 40.5 / 43.0 / 32.9 | 55.4 / 56.9 / 49.8 | | |
| S1 move the 15 inverted test files | 149 / 126 | 40.5 / 43.0 / 32.9 | **44.9 / 48.9 / 39.9** | 0% / 0% | **92.6%** |
| S2 session Tier A (8 clusters) | 151 / 129 | 40.3 / 43.0 / 32.9 | 54.9 / 56.4 / 49.2 | 0.8% / 0.0% | 15.3% |
| S3 services Tier A (7 clusters) | 150 / 128 | 40.4 / 42.4 / 32.8 | 54.6 / 55.9 / 49.1 | **11.6% / 13.6%** | 39.9% |
| S1+S2+S3 | 164 / 141 | 40.1 / 42.4 / 32.8 | 44.4 / 48.5 / 39.5 | 12.4% / 13.6% | 92.6% |
| S1+S2+S3 + Tier B (K<=10, both packages) | 202 / 170 | **38.6 / 41.5 / 32.8** | **43.0** / n.a. / **38.1** | **15.1% / 15.3%** | 92.6% |

Largest package after S1+S2+S3+Tier B: `session` 54,934 LOC, `server/services` 54,877 LOC (from 92,248 and 93,118); the largest new package is 15,651 LOC.

Count-based fan-out **rises** with every upper split, because the core now has more dependents:

| Scenario | RA1 merged median (17 Go-touching PRs) | Target model: share of all targets, 17 PRs / last-100 commits |
|---|---|---|
| S0 | 0.266 | 0.053 / 0.223 |
| S1 | 0.255 | 0.055 / 0.225 |
| S2 only | 0.305 | 0.050 / 0.268 |
| S3 only | 0.300 | 0.086 / 0.263 |
| S1+S2+S3 | 0.323 | 0.082 / 0.302 |
| S1+S2+S3 + Tier B | 0.450 | 0.156 / 0.427 |

So after Tier B the RA1 merged median breaches the 0.30 bound that T1 uses as a proxy, while the target-model share stays at 0.156 (still half the bound). This is why ADR-010 gates T1 on the target model, not on the merged proxy.

### 5.1 Marginal effect of each cluster alone (what ADR-010's SC4 and SC6 are computed from)

Replay of that one cluster against baseline, last 500 commits unless stated; "lib" = mean affected library-LOC share change in percentage points, "test" = same for test LOC, "fewer" = share of replayed changes with lower affected library LOC (500 / 100). Does the cluster import the rest of its package? `yes` = an upper cluster (only its own changes stop invalidating the core's consumers); `no` = independent (stops being invalidated by every core change).

| Cluster | Imports core | lib pp | test pp | fewer 500 / 100 | Passes SC6 |
|---|---|---|---|---|---|
| `server/services/wsbridge` | yes | -0.148 | -0.31 | 0.5% / 1.7% | yes (lib, test) |
| `server/services/sessionbundle` | no | -0.026 | -0.04 | 11.6% / 13.6% | yes (fewer) |
| `server/services/devtools` | no | -0.015 | -0.03 | 11.6% / 13.6% | yes (fewer) |
| `session/external` | yes | -0.18 | -0.09 | 0.5% / 0.0% | yes (lib) |
| `server/services/insights`, `gatestats`, `webhooks`, `hookctx` | yes | 0.0 | -0.27, -0.03, -0.10, -0.02 | 0% / 0% | no |
| `session/backlogplugin`, `importer`, `handoff`, `paneprompt`, `pipelinemode`, `sweeper`, `prpoller` | yes | 0.0 | -0.10, -0.04, -0.13, -0.04, -0.03, -0.02, 0.0 | 0% / 0% | no |
| Tier B `session` p0 (32 files, 14,681 LOC) | yes (after relocation) | -0.61 | -0.67 | 1.3% / 0.0% | yes |
| Tier B `session` p1 (21 files, 6,903 LOC) | yes | -0.26 | -0.32 | 0.8% / 1.7% | yes |
| Tier B `session` p2 (8 files, 594 LOC; startup backfill/migration files) | yes | -0.35 | -0.33 | 0.5% / 0.0% | yes |
| Tier B `session` p3, p4 | yes | 0.0 | -0.07, -0.06 | 0% / 0% | no |
| Tier B `server/services` p0 (`BacklogService`, 28 files) | yes | -0.17 | -1.88 | 0.8% / 0.0% | yes (test) |
| Tier B `server/services` p2 (websocket transport, 6 files) | yes | -0.29 | -0.76 | 1.1% / 1.7% | yes |
| Tier B `server/services` p1, p3, p4, p5 | yes | 0.0 to -0.08 | -0.06 to -0.15 | 0% to 0.3% | no |

Selected set (everything that passes SC6, plus the Story 13.2 test moves):

| Scenario | Packages | lib 500 / test 500 | Changes with lower lib LOC, 500 / 100 | Universal-slice touch, last 100 / 500 | `server/services` / `session` LOC | RA1 merged median | Target-model share, 17 PRs / last 100 |
|---|---|---|---|---|---|---|---|
| S0 | 143 | 40.5 / 55.4 | 0 / 0 | 71% / 70% | 93,118 / 92,248 | 0.266 | 0.053 / 0.223 |
| Tier 0 only | 149 | 40.5 / 44.9 | 0 / 0 | 71% / 70% | same | 0.255 | 0.055 / 0.225 |
| Tier 0 + Tier A (4 passing) | 153 | 40.2 / 44.6 | 12.2% / 13.6% | 66% / 68% | 88,481 / 91,061 | 0.275 | 0.060 / 0.247 |
| Tier 0 + A + Tier B (5 passing) | 158 | **38.5 / 43.5** | **14.8% / 15.3%** | **61% / 58%** | **67,445 / 68,883** | 0.297 | 0.072 / 0.273 |

A cluster's gain depends on the others (for example the core's remaining dependents), so the sum of marginal gains is not the combined gain; the combined row is what T9 targets use.

Universal-slice proxy for ADR-009 risk 1 (INFERRED until S14): share of Go production commits that touch a file in `session`, `server/services`, `server`, `config` or the root package (packages assumed to sit in every spec slice), last 100 / last 500: baseline **71% (29/41) / 70% (176/250)**; after S3: 66% / 68%; after S2+S3: 66% / 68%; after Tier B (both): 66% / 60%; after S2+S3+Tier B: **61% (25/41) / 57% (143/250)**. (ADR-009's 89% counts the directory prefix `server/`, which includes small packages that are not in every slice; this is the exact-directory form.)

## 6. Packages measured and not recommended

| Package | Evidence | Verdict |
|---|---|---|
| `config` | `config.go` (2,207 LOC) has 24 importers and is touched by 16 of the 17 last-500 commits; the other files have at most 6 importers and 0 to 3 commits. A satellite split covers 1,122 LOC that nobody changes. | No split (criterion SC4: churn is inside the hub file) |
| `server` | 49 files, 1 importer (`.`), library rdeps 1; 54 direct dependencies; 4% mechanical; 5% of commits avoid its core | No split (compile unit only) |
| `session/tmux` | 37 files, 10 in the SCC, 36% of last-500 commits avoid the core but touch `control_mode.go`/`tmux.go`/`exec_gate.go` (4/4/3 commits); the cluster that avoids the core has `exec_gate` plus metrics, 479 LOC | No split now (fails SC2, size) |
| `server/mcp`, `session/unfinished`, `github`, `session/git`, `session/tymux`, `server/wakes`, `session/git/backend` | 2 to 8% of LOC outside the SCC (`github` 22%, `server/mcp` 25%), 0 to 15% of commits confined to those files; `session/tymux` has `session.go` in 16 of 20 commits | No split (SC2/SC4) |
| `server/prwatch` | 97% outside the SCC but 2 commits in 500 | No split (SC4) |

## 7. Existing plans and audits this builds on (not contradicted)

- `docs/architecture-audit-session-package-split-2026-09-07.md`: recommends `session/backlog` (first), `session/review` (low risk), defers `Instance` (needs composed sub-objects) and persistence (becomes adapters of domain ports). The Tier B `session` p0 and p1 components are the measured, file-level form of items 1 and 2; Tier C is items 3 and 4. ADR-010 adopts that audit's design and sequencing for those and adds only the Bazel-facing measures.
- `docs/architecture-audit-session-backlog-extraction-design-2026-09-07.md`: the design for the `session/backlog` extraction (repository interface, import-cycle avoidance, sub-object triage, PR sequence). Story 13.5 references it rather than redoing it. Not implemented in the tree (VERIFIED: no `session/backlog` directory).
- `project_plans/core-domain-decomposition/` (ADR-001 "service per cluster", ADR-002): decomposes `session_service.go` into services in the same package. Compatible: those are type and file moves inside `server/services`; Tier A/B moves whole files, so Story 13.3 must not run while that project has an open PR touching the same files (the Story 13.1 conflict check).
- `project_plans/backlog-service-refactor/`, `project_plans/backend-di-refactor/`: overlap on `BacklogService` and the staged `server/dependencies.go` builder. Tier B `server/services` p0 is `BacklogService`; its Story must land after any in-flight backlog-service-refactor PR (checked by the Story 13.1 conflict check, not assumed).
- `docs/archive/tasks/completed/instance-god-object-decomposition.md`: earlier decomposition inside the package (file splitting); the July audit found it did not decouple the type, consistent with the 38-file `Instance` binding measured above.
- `.claude/rules/instance-lock-free-reads.md`: Tier B moves of files that read `Instance` fields must keep reads going through `Snapshot()`/`Workspace()`; a moved file may not start reading raw fields because it is now "outside" the package. Moving a file out of `session` also removes its access to unexported `Instance` fields, which enforces that rule structurally (any raw read becomes a compile error and is fixed by calling an exported accessor).
- Skills consulted: `golang-project-layout`, `golang-refactoring`, `code-hotspot-analysis`, `interface-pollution-checklist` (new interfaces for Tier C must be consumer-defined and have at least two implementations or a test double, not one-implementation interfaces for the sake of the cut), `primitive-obsession-checklist` (relocated declarations should keep existing named types; no new same-typed parameter piles).

## 8. Limits of these numbers

- The replay is on HEAD's graph, not each commit's base; files renamed since a commit map to the package's core node (conservative: understates the benefit of moves).
- Test files are assigned to a new package by `x_test.go` -> `x.go` name, else by majority of references; 10 to 21 test files per scenario reference both sides of a boundary (reported as `strad` in the harness) and need case-by-case handling.
- Affected LOC is a proxy for compile cost, and test LOC for test run cost; neither is measured wall time. The Bazel graph, not these proxies, is the authority once Story 1.1 exists (Task 13.1c re-computes from `bazel query`).
- Whether a package is in every e2e slice is INFERRED until spike S14 produces `tests/e2e/slice-manifest.json`.
- Tier B relocation counts treat a relocated declaration as free (its own dependencies on the group are not traced); the real count is therefore a lower bound for files touched.
