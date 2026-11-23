# Package layout: domain-driven design and Patterns of Enterprise Application Architecture

How new code is placed in this repository, and why. The short rules live in `CLAUDE.md` ("Package layout"); this page has the mapping, the recipes and the current status. The vocabulary is Evans' *Domain-Driven Design* and Fowler's *Patterns of Enterprise Application Architecture* (PoEAA).

## Status: what is in force and what is still a direction

In force today (enforced by `.golangci.yml` depguard rules and `tools/bzl/scoping`):

- Dependencies point downward: `server/` -> `session/` -> `config/` and `pkg/`. `session` never imports `server` (`no_server_in_core`); `pkg/` never imports `session` except `pkg/events` (`pkg_no_session`).
- Generated ent code is imported only by the persistence mapper and a short allow-list (`no_ent_in_services`, `scoped_packages_off_ent`).
- External-system adapters are isolated (`git_*` rules, `isolate_*` rules), and `scoping boundaries` (`tools/bzl/scoping/boundaries.txt`) forbids transitive imports for the packages listed there.
- Per-package size, file and import caps ratchet down (`tools/bzl/scoping/budget.txt`).

Still a direction (see `project_plans/package-split-ocp/`): `session` (about 14.5k top-level non-test lines: `wc -l` over `session/*.go` minus `_test.go`) and `server/services` (about 65.8k) remain hubs because `Storage`, `EntRepository` and `SessionService` bind most of their files. The persistence DTO lowering (ADR-011, `dto-lowering-roadmap.md`) is the staged unlock for the rest of `EntRepository`; the instance DTO lowering (`instance-dto-lowering-plan.md`) is finished, so `ent_repository.go` is only the constructor and its embedded stores. Do not invent a new layout; extend the one below and use the roadmap's stages for the hubs.

### Leaves that exist today

Checked with `ls -d` against this PR's base, `origin/main` (2026-11-20). "Embedded" means `session.EntRepository` embeds the store and `initStores()` builds it.

| Leaf | Holds | Notes |
|---|---|---|
| `session/webhookstore`, `guidancestore`, `wakestore`, `markerstore`, `shellstore`, `projectstore`, `rulestore`, `analyticsstore`, `instancestore` | one persistence store each (`instancestore.Instances`: instance create, read, update, delete) | embedded |
| `session/sessionpr` | pull-request rows (`PRStore`) | embedded; its depguard row allows `github` and `session/instance` |
| `session/backlogstore` | backlog item, source, activity and status-event persistence (30 non-test files) | embedded; owns the status writers (ADR-007 D1) |
| `session/flowstore` | flow reader | not embedded by design |
| `session/stagecfg`, `session/workflowengine` | stage, gate, workflow and pipeline-mode repositories; the workflow engine | consumed through narrow interfaces |
| `session/domain` | persistence DTOs and value types such as `InstanceData`, `LoadOptions`, `UpdateSkip` | imported by the stores and `session` |
| `session/instance` | the `Instance` family | interim budget exception, to be decomposed |
| `session/historyadapters`, `session/itemsources`, `session/migrations` | history adapters, tracker plugins, ent-only startup migrations | registry-style leaves |
| `session/driver` | the session driver: startup-dialog answering, initial prompt, stall detection, retry and tail handling (an upper leaf: imports `session`, never imported by it) | upper leaf |
| `session/external` | external-session approval, discovery and tmux streaming (an upper leaf: imports `session`, never imported by it) | upper leaf |
| `session/sweeper` | hibernation sweeper, orphaned-tmux sweeper and orphan reconcile (an upper leaf: imports `session`, never imported by it) | upper leaf |
| `session/autonomous` | the autonomous driver, session health checker (dead-pane recovery), nudge gate and dedup, idle-status gate and review transcript reader (an upper leaf: imports `session`, never imported by it) | upper leaf |
| `session/diagnose` | pure Diagnose & Nudge context-bundle assembly and token budget (a bottom leaf: imports only `session/domain` and `session/domain/backlogdto`; consumed by `server/services`' diagnostic service) | bottom leaf |
| `session/tagpoller` | LLM session-tag classification poller: content-hash-gated tagging of sessions against the tagging engine vocabulary (a standalone leaf: imports `session/instance`, `session/headless` and `pkg/classifier`, never `session`) | standalone leaf |
| `session/paneactivity` | driver pane-activity gate: one shared `list-panes -a` snapshot and a per-driver capture-skip gate behind the `driver_pane_activity_gate` flag (a standalone leaf: imports `session/instance`, `session/tmux`, `config`; `session/driver` and `server/services` import it; the capture-cache TTL arrives as an argument) | `session/driver` |
| `session/registry` | Session ID to live-actor registry: refcounted `Acquire`/`Register`/`Retain`/`ForceRelease` over `instance.LiveInstance` (a standalone leaf: imports `session/instance` and `session/domain`, never `session`; reads persistence through the consumer-side `registry.Store` port that `*session.Storage` satisfies) | standalone leaf |
| `session/pipelinemode` | pipeline-mode seeding (`EnsureDefaultSDDPipelineMode`), save-time content validation (`ValidatePipelineModeContent`) and the shared work-prompt sections (`Sec*`, `ForSeed`, `ForDefault`) (an upper leaf: imports `session/domain`, `session/stagecfg` and `session/tokens`, never `session`; imported by `server`, `server/services/backlogsvc` and `session/backlog/pipeline`) | upper leaf |
| `session/claudehistory` | Claude `history.jsonl` reader (`ClaudeSessionHistory`: load, search, per-project lookup) and the conversation-file finder `FindConversationFilePath` (a standalone leaf: imports only `session/instance`, never `session`; `server/services`, `session/driver`, `session/search` and `session/summaries` import it) | standalone leaf |
| `session/nudgeoutcomes` | durable `nudge_outcome_total{kind,outcome}` counter: `NudgeOutcomeRecorder`, the process-wide recorder (`InitNudgeOutcomes`, `RecordNudgeOutcome`) and the `/actuator/nudge-outcomes` report (a standalone leaf: imports only `config`, `log` and `telemetry`, never `session`; `server`, `server/mcp` and `server/services` import it) | standalone leaf |
| `session/slowops` | slow terminal-op instrumentation: `Recorder` (still-running goroutine dump, `terminal_slow_op_total`), `ReportIfSlow`/`ReportCall` (`terminal_slow_call_total` plus Tempo span) and the `Watch` hang watchdog (a standalone leaf: imports only `log`, `telemetry` and `internal/termlat`, never `session`; `session`'s backend timers and `NewEntRepository` import it, and the backend label crosses as a plain string) | standalone leaf |
| `server/services/featureflags` | flag `Spec`, immutable `Registry` | stage 2 injects the registry: `flags-stage2-plan.md` |
| `server/services/webhooktrigger` | inbound webhook trigger handlers: GitHub (PR-fix routing, provenance, reopened) and generic HMAC webhooks, shared trigger claim-and-fire (an upper leaf: never imports `server/services`; its production code imports `server/workflows` and `session/ent`, allowed by the `no_ent_in_services` exclusions in `.golangci.yml`) | wired by `server/server.go`; the registration service and callback validators stay in `server/services` |
| `server/services/sessionretention` | archived-session retention sweeper: PR-evidence holds, closed-PR reverify, per-item hold tracker (an upper leaf: never imports `server/services`; `*SessionService` reaches it through the consumer-side `SessionDeleter` port) | wired by `server/server.go`; `SweepOnce` lets `server/services` tests drive the real `DeleteSession` |
| `server/services/unfinishedwork` | `UnfinishedWorkService`: ConnectRPC handlers for the Unfinished tab (scan, dismiss/snooze, AI summary, quick commit-push, watch-dir config) over `session/unfinished` (an upper leaf: never imports `server/services`) | wired by `server/dependencies_analytics.go` and `server/server.go` |
| `server/services/importsvc` | `ImportService`: ConnectRPC handlers for import-external-session (preview, commit, confirm-kill, cancel-pending-kill) over `session/importer` (an upper leaf: never imports `server/services`) | wired by `server/server.go` |
| `server/services/federation` | cross-host session federation: `FederatedSessions` peer mirror and SSRF-guarded dial, `FederatedWatchAggregator`, federated terminal stream, deep-link resolver (an upper leaf: never imports `server/services`; `SessionService` holds a `*federation.FederatedSessions`; never starts a session, see `noarchivedrevival`) | wired by `server/server.go` and `main.go` |
| `server/reviewqueue` | review queue manager | imported by the composition root (`server/dependencies*.go`) and an integration test |

The instance store is done (I0-I6 in `instance-dto-lowering-plan.md`); `ent_repository.go` keeps only the constructor, `initStores`, `GetEntClient`, `MaintenanceDriver` and `Close`.

## How to add a leaf

Pick the shape first. A persistence store becomes its own struct built by `New(client, ...)` and embedded by `EntRepository`; a variant (tracker, migration, history adapter) becomes a file plus one registration at the composition root. Then follow the checklist in `.claude/rules/leaf-packages.md`: constructor paths, consumer-side `Ports{...}`, the `isolate_<leaf>` depguard row and `boundaries.txt` row, the budget row, the `go_sources` filegroup, and the pre-push verification list. Move with `git mv` plus `go run ./tools/bzl/scoping move --from D --to D --files ...`, then fix receivers and build-tagged files by hand.

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
| Points of variation | Plugin, Separated Interface, Registry | `ItemSourcePlugin` + `PluginRegistry` (tracker plugins), the process-backend registry (`session/backend_registry.go`), the history-adapter registry (`session/historyadapters`, a new agent is one adapter file plus one line in `DefaultHistoryAdapters`), `DefaultStartupMigrations`, the ordered stuck-detector list (`session/backlog/stuck_detectors.go`), the history adapter list (`session/history_adapter.go`) |
| Object graph assembly | Composition root | `server/dependencies.go` (`BuildDependencies`) is the one place that names concrete implementations |
| Test seams | Gateway fakes | `NewTestEntRepository`, `session/internal/sessiontest`, `server/services/internal/svctest` |

## Rules for new code

1. **Smallest cohesive package.** Add a new package under the owning layer (`session/<concept>`, `server/<concept>`) instead of another file in `session/` or `server/services/`. A package should have one reason to change and a name that states its role.
2. **A leaf is imported by its owning hub (for embedded stores, `session.EntRepository`), the composition root and tests; never by a sibling leaf except where its `isolate_` row lists it.** Keep the core small, abstract and stable, and put volatile detail in leaves (stable dependencies, stable abstractions).
3. **Define an interface where it is consumed**, not where it is implemented (Go's separated interface). Add one only for a real consumer and either two implementations (for example the ent adapter and an in-memory fake) or a genuine test seam. See the `interface-pollution-checklist` skill.
4. **Open-closed.** A new variant (tracker, migration, stuck detector, process backend, history adapter, delivery channel) is a new file plus one registration at the composition root. If adding a variant means editing a `switch` on a kind enum in several files, add a registry instead of another case.
5. **Persistence stays behind the mapper.** Return domain DTOs, wrap `ent.IsNotFound` as `session.ErrNotFound`, and import `session/ent` only where the depguard allow-list says.
6. **No lasting type aliases** to keep an old import path alive; an alias keeps `session` as the hub. A temporary alias is allowed only when the same series removes it.
7. **One composition root.** `server/dependencies.go` is the only place that builds dependencies and names concrete types (owner decision Q5, 2026-11-20). `server/mcp` receives its deps from there and is not a second root, enforced by `server/mcp/single_root_guard_test.go` (no `New*` of a concrete type) and the `isolate_server_mcp` depguard row; its measured gap is in `project_plans/package-split-ocp/mcp-single-root-plan.md`.
8. **Behaviour-preserving moves.** `git mv` plus a codemod, one cluster per PR, `go build`, `go vet` and the moved and importing packages' tests green, `make gazelle` leaving no diff, and a `scoping-budget-raise:` line in the commit message if a cap must rise.

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
