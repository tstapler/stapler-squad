# Workflow-engine cluster: feasibility study (read-only)

Measured on main `efcbe4db7` (2026-11-14), a fresh worktree after `make sync`. No production code was changed. Owner decision Q1 applies: OCP/leaf shape alone justifies a move, so SC6 (materiality) is waived. SC1 (no new cycle, at most 10 relocated declarations) and SC3 (at most 5 new importers) must still pass.

**Verdict: GO.** The cluster is already a leaf at the symbol level. The "needs a `prompts` leaf and a `StageRepoFactory` seam" work in `session-hub-redesign-plan.md` (D3, W0b/W0c) is stale: `session/instance` is already extracted and the shadow ledger already moved, and neither seam is needed now. LOC: the owner's 4,371 / 14 files, Lens 1's 5,823 / 21 and Lens 3's 4,491 / 20 do not reproduce. The measured set is 17 files, 4,149 lines (`wc -l`, non-test).

## 1. File set (all in `session/`)

| Group | Files | wc -l |
|---|---|---|
| Engine core | `workflow_engine.go` (140), `workflow_engine_types.go` (111), `workflow_engine_enforce.go` (219), `workflow_engine_bypass.go` (63), `workflow_engine_selector.go` (774), `workflow_engine_soak.go` (546), `workflow_engine_replay.go` (414) | 2,267 |
| Configured engine | `configured_workflow_engine.go` (575), `stage_config_cache.go` (204), `workflow_system_edges.go` (97), `workflow_slug.go` (27), `workflow_stage_seed.go` (423), `gate_latch.go` (160) | 1,486 |
| Repository ports (consumer-side interfaces and DTOs the engine names) | `stage_config_repository.go` (191), `gate_satisfaction_repository.go` (93), `pipeline_mode_repository.go` (64), `pipeline_mode_executor.go` (48) | 396 |
| **Total (17 files)** | | **4,149** (3,989 for the first 16, plus `gate_latch.go` 160) |

Stay behind: `transition_gate.go` (371; it is `Storage` methods and has 15 back-edges to `Storage`/`EntRepository`), `gate_approval.go`, `ent_stage_config_repository.go`, `ent_gate_satisfaction_repository.go`, `ent_pipeline_mode_repository.go`, `ent_workflow_repository.go`, `workflow_repository.go` (workflow-definition CRUD, covered by the `workflowrepo` row in the hub plan), `workflow_enabled_field_migration.go`. Also outside scope: the `server/workflows` and `server/services/workflow_*` files (workflow *definitions and triggers*, a different feature from the transition-gate engine).

## 2. Evidence: back-edges and importers

Method (reproducible): a go/types scan of `./session` (scratch program, not committed) listing, for the file set, every use of a package-level `session` declaration made from a file outside the set, and vice versa.

- Imports of the cluster (all already below `session`): `config`, `log`, `telemetry`, `internal/sqlitedsn`, `session/domain`, `session/domain/backlogdto`, `session/ent`, `session/ent/backlogitem`, `session/ent/backlogstage`, `session/hostclaim`, `session/instance`. The only `session/instance` use is `instance.ValidTransitions` at `session/workflow_engine.go:19-20`. `go list -deps ./session/instance` shows no cluster package and no `session`, so there is no cycle.
- Back-edges, 12 core files only: 8 symbols, all in three port files (`StageConfigRepository` and its methods, `GateSatisfactionRepository`/`GateSatisfactionData`, `PipelineModeRepository`). Adding those three files leaves 2 (`PipelineStageExecutor`, `StageRole` from `pipeline_mode_executor.go`). Adding that 48-line file leaves **0**. Adding `gate_latch.go` (it defines methods on `ConfiguredWorkflowEngine`, so it cannot stay) still leaves **0**.
- Forward edges (staying non-test files into the cluster): 44 distinct symbols, from `transition_gate.go`, `storage.go`, `backlog_sync.go`, `backlog_remediation.go`, `ent_*` repositories, `gate_approval.go`, `pipeline_mode_seed.go`, `pipeline_mode_validation.go`. All exported except 3 (below).
- Importers after the move: `scoping move --dry-run` rewrites non-test files in `server`, `server/services`, `server/services/backlogsvc`, `session/backlog`, `session/backlog/chains`, `session/backlog/shadowledger` (6 packages; test files add `server/mcp`, `session/backlog/reviewgate`). All six already import `session`.
- The hub plan's edges "WE to `backlog` 20 / `driver` 18 / store 47" do not appear in the current code: the cluster imports no `session/backlog*`, `driver` or store type.

## 3. The cut and what must be inverted

Nothing needs inversion. The cut is a whole-file move of the 17 files to `session/workflowengine`; the ports are already consumer-defined interfaces, and the ent implementations stay in `session` and satisfy them structurally (`NewEntStageConfigRepository`, `NewEntGateSatisfactionRepository`, `NewEntPipelineModeRepository` are constructed at the composition root, so there is no `StageRepoFactory`).

Declaration-level residue (SC1 inputs):
- Unexported names that staying code or tests use: `edgeKey`, `systemRequiredCore`, `systemRequiredOffGraph` (all used by `session/transition_callers_test.go:126-132`) and `evaluateStructuralOrLatched` (only if `gate_latch.go` is left behind, which it must not be). Count: 4 at most.
- White-box staying tests: `configured_workflow_engine_test.go`, `workflow_engine_{equivalence,quiet,replay,selector,selector_bench}_test.go`, `workflow_stage_seed_test.go` and the `ent_*`/`gate_satisfaction` tests use `NewTestEntRepository`, `EntRepository`, `Storage`, `twins_core_test.go` helpers. They stay in `session` (importing `workflowengine`) or move to an external `_test` package; they only touch exported cluster symbols except the three above. Tests with no back-edge move with the code: `workflow_engine_soak_test.go`, `workflow_engine_test.go`, `workflow_slug_test.go`, `workflow_system_edges_test.go`, `workflow_system_edges_archive_test.go`, `pipeline_mode_executor_test.go`.

## 4. SC1 / SC3 results

```
go run ./tools/bzl/scoping criteria --lower --relocated 4 --importers <6 pkgs> --natural <same 6>
SC1: PASS (lower package, count=4 max=10)
SC3: PASS (lower package, importers=0 max=5 [])
```

SC1 also needs "no new cycle": the back-edge scan shows 0 `session` symbols used by the cluster, and `session/instance` does not import `session`. SC6 was not run (waived by Q1). `scoping touch --files` reports gain 0.00 pp on the universal-touch share (`prod_changes=248`), consistent with a leaf-shape-only payoff. `kibitzer split_sim` was not run: it requires a partition covering every file of `session` exactly once.

## 5. Guard and CI traps

- `server/services/workflow_bypass_strict_test.go:217`: key `"session/workflow_engine_replay.go replayAnswer"` becomes `session/workflowengine/workflow_engine_replay.go replayAnswer`. `workflow_bypass_inventory_test.go` keys on `session/storage.go`, `session/transition_gate.go`, `session/backlog_remediation.go` (lines 116-126, 165), which do not move.
- `tools/bzl/scoping/budget.txt` and `boundaries.txt` need rows for the new package (`go run ./tools/bzl/scoping budget --init session/workflowengine`; `make ready-scoping-budget` and `ready-scoping-boundaries`). `session`'s own cap may only shrink.
- `make gazelle` after the move; Bazel `BUILD.bazel` is generated.
- Metric names are string literals and unchanged: `workflow_engine_bypass_total` is registered in `workflow_engine_bypass.go:28` and read at `server/actuator.go:296` via `session.EngineBypassSnapshot()` (becomes `workflowengine.EngineBypassSnapshot()`); `actuator_test.go:115,157` read the JSON key only.
- `move` rewrites imports but leaves staying `session` files that use moved symbols unqualified (it prints `WARNING ... uses moved symbol`); they need a hand edit (about 8 files: `transition_gate.go`, `gate_latch.go` if left, `storage.go`, `backlog_sync.go`, `backlog_remediation.go`, `ent_*`). A scratch build fails on exactly those.
- `.claude/rules/noarchivedrevival.md` and `norawghrequest.md` file lists do not include these files. `.golangci.yml` depguard has no rule for the new package; add one matching `session/instance` if wanted.
- `go.sum` gets dirtied by `make sync`; do not commit it.
- Disk: ~30 GB Go-cache regrowth per verification wave. Use a private `GOCACHE` and clean it.

## 6. Go / no-go and PR sequence

**GO.** Risk is mechanical (about 8 hand edits plus about 60 import rewrites); no behavior or seam change.

| PR | Agent wave | Content | Verify |
|---|---|---|---|
| WE-1 | 1 worker | `git mv` of the 17 files and their back-edge-free tests via `scoping move`, hand-qualify staying users, export the 3 test-used names (or move `transition_callers_test.go`), fix `workflow_bypass_strict_test.go:217`, `make gazelle`, budget/boundaries rows | `go build ./... && go vet ./session/... ./server/...`; `go test ./session/... ./server/services/... -run 'Workflow|Gate|Engine|Stage|Latch|Bypass'`; `make ready-scoping-budget ready-scoping-boundaries gazelle-check`; `go run ./tools/bzl/scoping criteria --lower ...` |
| WE-2 (optional) | 1 worker | Move the white-box tests to an external test package, and add the depguard rule | `make lint`; `make test` |

Cost class: one small-to-medium agent wave (one worker plus one review), blocked only on CI. Rebase risk: `transition_gate.go`, `storage.go` and the `ent_*` files are hot; land WE-1 quickly after rebasing on main.

## 7. Other clusters marked "held for owner decision"

- **History adapters** and **`ent_*`**: covered by other studies, skipped.
- **FLAGS-1** (`server/services` to `server/services/featureflags`, `roadmap.md:166-172`): stage 1 already exists (`server/services/featureflags/` holds `registry.go`, `feature_flag_controller.go`, `feature_flag_names.go`, `feature_flag_audit_policy.go`); it removes 0 LOC from `session` and was not re-measured here. Status: UNVERIFIED beyond the directory listing.
