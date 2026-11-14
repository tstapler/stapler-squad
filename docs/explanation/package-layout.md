# Package layout: domain-driven design and Patterns of Enterprise Application Architecture

How new code is placed in this repository, and why. The short rules live in `CLAUDE.md` ("Package layout"); this page has the mapping, the recipes and the current status. The vocabulary is Evans' *Domain-Driven Design* and Fowler's *Patterns of Enterprise Application Architecture* (PoEAA).

## Status: what is in force and what is still a direction

In force today (enforced by `.golangci.yml` depguard rules and `tools/bzl/scoping`):

- Dependencies point downward: `server/` -> `session/` -> `config/` and `pkg/`. `session` never imports `server` (`no_server_in_core`); `pkg/` never imports `session` except `pkg/events` (`pkg_no_session`).
- Generated ent code is imported only by the persistence mapper and a short allow-list (`no_ent_in_services`, `scoped_packages_off_ent`).
- External-system adapters are isolated (`git_*` rules, `isolate_*` rules), and `scoping boundaries` (`tools/bzl/scoping/boundaries.txt`) forbids transitive imports for the packages listed there.
- Per-package size, file and import caps ratchet down (`tools/bzl/scoping/budget.txt`).

Still a direction (see `project_plans/package-split-ocp/`): `session` (about 81k non-test lines) and `server/services` (about 74k) remain hubs because `Instance`, `Storage`, `EntRepository` and `SessionService` bind most of their files, and some storage methods still return `*ent.*` types. Lowering the persistence DTOs (ADR-011, `dto-lowering-roadmap.md`) is the unlock for extracting more leaves. Do not invent a new layout; extend the one below and use the roadmap's stages for the hubs.

## The mapping

| Concern | DDD / PoEAA name | Where it lives here |
|---|---|---|
| Value types, status enums, error sentinels | Domain Model, Value Object | `session/domain` (pure leaf types, no infrastructure imports); the DTO lowering moves more here |
| The session aggregate | Domain Model / Entity | `instance.Instance` in `session/instance` (read it through `Snapshot()`; see `.claude/rules/instance-lock-free-reads.md`) |
| Loading and saving aggregates | Repository + Data Mapper | `session.Storage` over `EntRepository`; narrow per-aggregate interfaces such as `StageConfigRepository` and `PipelineModeRepository`; ent rows convert to DTOs only in `ent_repository_*.go` |
| Transactions | Unit of Work (ent transactions) | `EntRepository` methods run their writes in an `*ent.Tx`; transaction handling stays inside the repository |
| Use cases and orchestration | Service Layer | `server/services` handlers and the use-case logic in `session/backlog`, `session/prpoller`, `server/reviewqueue` |
| The wire API | Remote Facade + DTO | ConnectRPC services in `proto/session/v1`; proto messages are the DTOs |
| External systems (git, tmux, GitHub, push, shells) | Gateway / Adapter behind a port | `session/git/backend` (cli, gogit, native), `session/tmux`, `session/tymux`, `github`, `server/push` |
| Points of variation | Plugin, Separated Interface, Registry | `ItemSourcePlugin` + `PluginRegistry` (tracker plugins), the process-backend registry (`session/backend_registry.go`), `DefaultStartupMigrations`, the ordered stuck-detector list (`session/backlog/stuck_detectors.go`), the history adapter list (`session/history_adapter.go`) |
| Object graph assembly | Composition root | `server/dependencies.go` (`BuildDependencies`) is the one place that names concrete implementations |
| Test seams | Gateway fakes | `NewTestEntRepository`, `session/internal/sessiontest`, `server/services/internal/svctest` |

## Rules for new code

1. **Smallest cohesive package.** Add a new package under the owning layer (`session/<concept>`, `server/<concept>`) instead of another file in `session/` or `server/services/`. A package should have one reason to change and a name that states its role.
2. **Leaves are imported only by the composition root and tests.** A package that other domain packages import is part of the core, not a leaf; keep the core small, abstract and stable, and put volatile detail in leaves (stable dependencies, stable abstractions).
3. **Define an interface where it is consumed**, not where it is implemented (Go's separated interface). Add one only for a real consumer and either two implementations (for example the ent adapter and an in-memory fake) or a genuine test seam. See the `interface-pollution-checklist` skill.
4. **Open-closed.** A new variant (tracker, migration, stuck detector, process backend, history adapter, delivery channel) is a new file plus one registration at the composition root. If adding a variant means editing a `switch` on a kind enum in several files, add a registry instead of another case.
5. **Persistence stays behind the mapper.** Return domain DTOs, wrap `ent.IsNotFound` as `session.ErrNotFound`, and import `session/ent` only where the depguard allow-list says.
6. **No lasting type aliases** to keep an old import path alive; an alias keeps `session` as the hub. A temporary alias is allowed only when the same series removes it.
7. **Behaviour-preserving moves.** `git mv` plus a codemod, one cluster per PR, `go build`, `go vet` and the moved and importing packages' tests green, `make gazelle` leaving no diff, and a `scoping-budget-raise:` line in the commit message if a cap must rise.

## Recipes

- **New tracker plugin:** implement `ItemSourcePlugin` in a new file and register it where the plugin registry is built; do not add a constructor call to the core.
- **New startup migration:** add a `Migration` and one entry in `DefaultStartupMigrations` (tests can override the list with `WithMigrations`).
- **New stuck detector:** add a detector to `defaultStuckDetectors()` in `session/backlog/stuck_detectors.go`; the ordering test pins the load-bearing order.
- **New process backend:** implement the backend, give it a `BackendKey()`, and add it to the registry in `session/backend_registry.go`.
- **New persistence DTO:** put the type (and any error sentinel) in `session/domain`, convert it in the `ent_repository_*.go` mapper, and expose it through a narrow consumer-defined interface.
- **New RPC:** follow "New API Endpoints" in `CLAUDE.md`; the handler stays thin and calls a use-case service, and `terminal_write_rpcs_test.go` must classify it.

## Sources in this repository

- `project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md` and `implementation/scoping-ledger.md`: the tiers, the criteria and what was measured.
- `project_plans/backlog-service-refactor/decisions/ADR-010-session-domain-sub-package.md` and `ADR-011-storage-interface-domain-dtos.md`.
- `project_plans/package-split-ocp/roadmap.md`, `tierc-roadmap.md`, `dto-lowering-roadmap.md`: what has been split, what was rejected and why, and the staged plan for the hubs.
- `docs/architecture-audit-session-package-split-2026-09-07.md`: the hotspot analysis of the `session` package.
