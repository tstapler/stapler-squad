# Plan: split the session/ent_* family out of package session (roadmap item "session P5-entstore")

Date 2026-11-14, measured at `efcbe4db7` (main after #1646). The owner decided to plan this as a multi-PR project. This document replaces the one-line verdict at [roadmap.md:326](roadmap.md) with measurements, a dependency cut, a PR sequence and per-PR go/no-go calls.

Evidence labels: VERIFIED = command run in this worktree (command given); INFERRED = reasoned from verified data. All numbers were taken on a fresh worktree with generated ent code (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`, flags from `tools/bzl/ent/ent_flags.txt`). The helper that produced the identifier-level edge tables was a throwaway `go/packages` + `go/types` program (not committed). Its method: for every identifier use in package `session` (test variant), record the declaring file; classify files into clusters; count distinct symbols crossing a cluster boundary.

## Verdict in six lines

1. The roadmap's blockers are mostly stale. The hard remaining one is real but narrow: Go methods must live with their receiver, so a leaf can only host a store that is its own struct, never `func (r *EntRepository)` bodies.
2. Four clusters (webhook, flow reader, wake, guidance) already import only `session/domain`, `session/ent/*`, `session/flow`, `log` and have 0 to 4 production symbols coming from the rest of `session`. They can leave as small store structs that `EntRepository` embeds, so no call site changes.
3. Nothing needs "EntRepository to become plain functions or a consumer-defined port" for this project. Embedding keeps the method set; ports are added per consumer later, only where a second implementation exists (today none does, VERIFIED below).
4. The measured Bazel/T9 payoff of every sub-leaf is 0.00 pp (`scoping touch`). The payoffs that are real: `server/wakes` stops importing `session` (VERIFIED prerequisite shape), about 3.0k production and 3.3k test lines leave `session`, and new persistence code stops landing in one hub.
5. Backlog (3,445 prod lines, 100 of 180 recent ent_* commits), session-PR (1,184) and the instance core (`ent_repository.go`, 2,826 with siblings) are blocked, with the blockers named and the prerequisite PRs planned in wave C.
6. Recommended first PR: **E3, move the wake DTOs and sentinels to `session/domain`** (S/M). It has no prerequisite, a measurable result (`go list -deps ./server/wakes | grep -c 'stapler-squad/session$'` goes 1 to 0), and unblocks the wake store move. E0 and E1 can run in the same wave.

## 1. Current-state measurements

### 1.1 Size

| Measure | Value | Command / source |
|---|---|---|
| `session/ent_*.go` files | 63 (31 prod, 32 test) | `ls session/ent_*.go` |
| Production lines | 12,008 | `ls session/ent_*.go \| grep -v _test \| xargs cat \| wc -l` |
| Test lines | 9,002 | same, `_test.go` only |
| Package `session` non-test lines | 44,361 (scoping reports 44,251 for T9d) | `session/*.go` minus tests; `go run ./tools/bzl/scoping replay` field `t9d_session_loc` |
| Budget row | `session files=179 loc=45520 imports=85` | [tools/bzl/scoping/budget.txt:4](../../tools/bzl/scoping/budget.txt) |
| Direct in-module imports of `session` | 82 | `go list -f '{{range .Imports}}...' ./session` |

Per cluster (cluster = file-name grouping used throughout this plan):

| Cluster | Prod files / LOC | Test files / LOC | Files |
|---|---|---|---|
| backlog | 2 / 3,445 | 3 / 1,900 | `ent_repository_backlog.go`, `ent_repository_backlog_source.go` |
| entcore | 6 / 2,826 | 8 / 1,443 | `ent_repository.go` (2,436), `ent_client_instrumented.go`, `ent_repository_conversation.go`, `ent_repository_integration_credential.go`, `ent_repository_migrations.go`, `ent_repository_seed.go` |
| flow | 8 / 1,095 | 8 / 1,512 | `ent_repository_flow*.go` (reader 790 + markers 221), `ent_repository_stuck_episode.go` (84) |
| guidance | 4 / 950 | 4 / 1,226 | `ent_repository_guidance*.go` |
| sessionpr | 2 / 1,184 | 2 / 1,047 | `ent_repository_session_pr.go`, `ent_repository_session_pr_guard.go` |
| stagecfg | 4 / 1,261 | 4 / 620 | `ent_stage_config_repository.go`, `ent_workflow_repository.go`, `ent_pipeline_mode_repository.go`, `ent_gate_satisfaction_repository.go` |
| wake | 3 / 694 | 1 / 548 | `ent_repository_wake.go`, `ent_repository_wake_transitions.go`, `ent_repository_operator_wake.go` |
| webhook | 2 / 553 | 2 / 706 | `ent_repository_webhook_registration.go`, `ent_repository_webhook_lifecycle.go` |

The prod column sums to 12,008 and the test column to 9,002 (checked).

### 1.2 Importers and call sites (VERIFIED by grep over `*.go`, excluding the throwaway tool)

| Question | Answer |
|---|---|
| Distinct packages importing `session` (incl. tests) | 43 (`go list -e -test` over `./...`; 33 with non-test imports) |
| `session.EntRepository` referenced outside `session/*.go` | 153 refs in 49 files: `session/backlog` 87, `server/wakes` 13, `server/services` 12, `session/wtconsistency` 9, `backlogsvc` 8, `session/inttest` 5, `server/workflows` 5 |
| `session.NewEntRepository` outside `session` | 40 refs, 27 files (16 non-test) |
| `session.NewTestEntRepository` | 124 refs, 71 files, 5 non-test |
| `GetEntClient()` | 215 sites: 40 lines in 19 non-test files (incl. definitions), 175 sites in 66 test files. The roadmap's "37 callers" matches the non-test count only |
| `Storage` wraps the repo concretely | `Repo *EntRepository` at [session/storage.go:55](../../session/storage.go) |
| Non-ent files declaring `EntRepository` methods | 10, not 9: `storage_backlog.go` 48 methods, `session_pr_backfill.go` 9, `ac_audit.go` 5, `diagnose_result.go` 4, `storage_backlog_jules.go` 3, `backlog_item_prs.go` 2, `backlog_item_pr_mirror.go` 2, `session_pr_gc.go`, `guidance_answer_metrics.go`, `backlog_item_pr_link_write.go` 1 each |
| Tests that reassign `repo.Client` | 3 files: [ent_repository_flow_markers_test.go:199](../../session/ent_repository_flow_markers_test.go), [ent_repository_session_pr_test.go:116](../../session/ent_repository_session_pr_test.go), [ent_repository_stuck_episode_test.go:372](../../session/ent_repository_stuck_episode_test.go). None is in wake, webhook or guidance |
| `&EntRepository{...}` literals | [ent_repository.go:116](../../session/ent_repository.go), [:303](../../session/ent_repository.go), and two in [review_gate_test.go:12-13](../../session/review_gate_test.go) |

### 1.3 Imports of the ent_* production files

In-module imports of every prod file other than `session/ent/*` (VERIFIED, `grep` of each file's import block). The "non-ent session symbols" column is the count of distinct identifiers declared in other, non-`ent_*` files of package `session` that the file uses (production variant, from the `go/types` edge table).

| File | LOC | In-module imports (besides `session/ent/*`) | Non-ent session symbols |
|---|---|---|---|
| `ent_repository_backlog.go` | 3,420 | `log`, `domain`, `domain/backlogdto`, `session/instance` | 20 |
| `ent_repository.go` | 2,436 | `internal/sqlitedsn`, `session/dbpool`, `domain`, `session/instance` | 19 |
| `ent_repository_session_pr.go` | 1,032 | `log`, `domain`, `session/instance` | 2 |
| `ent_repository_guidance.go` | 660 | `log`, `domain`, `domain/backlogdto` | 3 |
| `ent_stage_config_repository.go` | 567 | `domain` | 12 |
| `ent_repository_webhook_registration.go` | 394 | `domain` | 0 |
| `ent_workflow_repository.go` | 325 | `domain` | 2 |
| `ent_repository_wake_transitions.go` | 354 | `domain` | 0 |
| `ent_repository_wake.go` | 252 | `domain` | 0 |
| `ent_repository_flow_markers.go` | 221 | `log`, `session/flow` | 9 |
| `ent_repository_flow_events.go` | 206 | `domain`, `session/flow` | 0 |
| `ent_pipeline_mode_repository.go` | 189 | `domain` | 3 |
| `ent_repository_flow_verdicts.go` | 172 | `domain`, `session/flow` | 0 |
| `ent_gate_satisfaction_repository.go` | 180 | `domain` | 3 |
| `ent_repository_webhook_lifecycle.go` | 159 | `domain` | 0 |
| `ent_repository_guidance_mirror.go` | 155 | `domain` | 1 |
| `ent_repository_session_pr_guard.go` | 152 | `domain` | 4 |
| `ent_repository_flow_items.go` | 154 | `domain`, `session/flow` | 2 |
| `ent_repository_migrations.go` | 125 | `github`, `session/instance`, `session/migrations` | 2 |
| `ent_repository_flow_stuck.go` | 121 | `log`, `session/flow` | 0 |
| `ent_repository_guidance_cache.go` | 105 | none | 0 |
| `ent_repository_integration_credential.go` | 105 | `domain` | 0 |
| `ent_repository_operator_wake.go` | 88 | `domain` | 1 |
| `ent_repository_flow_stream.go` | 88 | `session/flow` | 0 |
| `ent_repository_stuck_episode.go` | 84 | `log` | 0 |
| `ent_repository_seed.go` | 68 | none | 1 |
| `ent_client_instrumented.go` | 58 | `internal/sqlitedsn`, `session/dbobs`, `session/dbsplit` | 0 |
| `ent_repository_flow.go` | 49 | `domain` | 0 |
| `ent_repository_conversation.go` | 34 | none | 0 |
| `ent_repository_guidance_purge.go` | 30 | none | 0 |
| `ent_repository_backlog_source.go` | 25 | none | 0 |

No ent_* file imports `server`, `session/tmux`, `session/git*`, or `session/backlog`. The only module import that could leak a heavy closure is `github` from `ent_repository_migrations.go`, and that file stays (it is the shell's constructor glue). The roadmap's claim that `pr_set.go` drags `github` into the PR store is stale: the PR records now live in `session/domain` (`pr_link.go`, `pr_set.go`) and `ent_repository_session_pr.go` imports only `domain`, `instance` and `log`.

### 1.4 Back-edges and cross-cluster edges

Distinct identifiers crossing a cluster boundary, production files only (VERIFIED, `go/types` edge table). "core" = every non-`ent_*` file of `session`.

| From | To | Symbols | Notes |
|---|---|---|---|
| core | backlog | 56 | `EntRepository` methods defined in `ent_repository_backlog.go`, used by `storage*.go`, `transition_gate.go`, `backlog_sync.go`, `diagnose_*`, plus `BacklogItemToData`, `ItemSessionToSummary` |
| core | entcore | 54 | `EntRepository`, constructors, instance CRUD, `nilIfEmpty` |
| core | sessionpr | 32 | `PROwner`, `SessionPROwner`, `ItemPROwner`, `ItemGuardView`, `withPRTx`, `upsertSessionPRTx`, `linkOwnerPredicates` |
| core | wake | 26 | all from `storage_wake.go` (22) and `storage_operator_wake.go` (forwarders) |
| core | guidance | 15 | `storage_guidance*.go`, `storage_operator_wake.go` forwarders |
| core | flow | 3 | |
| core | webhook, stagecfg | 0 | webhook has no production back-edge at all |
| backlog | core / flow / guidance / entcore / sessionpr | 20 / 3 / 4 / 2 / 1 | hub dependencies: `CallbackDispatcher`, `ItemChangePublisher`, `BacklogItemChange`, `MaxRemediationAttempts`, `itemPRLinkWrite`, `deleteOwnerPRRows` |
| entcore | core | 20 | `AnalyticsData`, `ProjectData`, `RepositoryOption`, `LoadOptions`, `MarkerLimiter`, `watchSlowOp`, startup-migration helpers |
| stagecfg | core | 20 | the four `*Input`/`*Data` DTO files plus `StageDefinition`, `TransitionDefinition`, `SerializeStageExecutors` |
| flow | core / entcore / wake | 11 / 1 / 1 | `FlowMarker*` kinds and data (6), `MarkerLimiter` (4), `BuiltInStageOrder`, `NewDefaultWorkflowEngine`; `utcPtr` from wake |
| guidance | core / backlog / entcore | 4 / 2 / 1 | `recordGuidanceAnswered`, `countGuidanceDelivery`, `BacklogItemChange`, `ChangeItemUpdated`; `BacklogItemToData`, `publishItemChanged` |
| wake | core / guidance / entcore | 1 / 2 / 1 | `recordGuidanceAnswered`; `GuidanceRequestData`, `guidanceRequestsToData` (both from `operator_wake.go`) |
| sessionpr | core | 5 | `ItemHasUnrepresentedLegacyPR`, `itemCoverage`, `containsRef`, `isDeterministicPRImportError`, `itemSessionBatchChunk` |
| webhook | entcore | 1 | `EntRepository` itself (the receiver) |

Receiver fields each cluster touches (VERIFIED, grep of `er.|r.|repo.<field>` in the cluster's prod files): wake `Client` x20 and `wakeChanged` x3; webhook `Client` x4; guidance `Client` x15 and `pendingGuidance` x5; flow markers `Client`, `markerClock`, `markerLimiter`; session-PR `Client` x21, `prDrops` x5, `prVerifyCountersTrusted` x2, `afterPRLinkHook` x2; backlog `Client` x88, `itemChangePublisher` x4, `callbackDispatcher` x4, `ChainFirer` x3. `EntRepository` is declared at [ent_repository.go:53](../../session/ent_repository.go).

### 1.5 What the scoping tools say

| Check | Result |
|---|---|
| `scoping leaves --dir session --min-loc 1` (files nothing else in `session` uses) | ent_* leaves: flow reader files (702 LOC), `ent_stage_config_repository.go`, `ent_workflow_repository.go` + `workflow_repository.go`, `ent_pipeline_mode_repository.go`, `ent_gate_satisfaction_repository.go`, `ent_repository_guidance_purge.go`. Wake, webhook and guidance are not listed because their methods hang off `EntRepository`, which is exactly the receiver blocker |
| `scoping touch --files <webhook files>` | `gain=0 (0.00 pp)` of 248 prod changes |
| `scoping touch` wake incl. storage forwarders | 0.00 pp |
| `scoping touch` guidance incl. forwarders | 0.00 pp |
| `scoping touch` union of webhook + wake + guidance + flow reader + forwarders | 0.00 pp |
| `scoping touch` all 63 ent_* files | gain 4 (1.61 pp); all but backlog: 3 (1.21 pp) |
| `scoping criteria` webhook (upper, relocated 0, uses 0, importers `server/services`) | SC1 PASS, SC3 PASS |
| `scoping criteria --lower` wake (relocated 4, uses 26, 4 importers) | SC1 PASS, SC3 PASS (the numbers are my reading of "relocated" and "uses"; INFERRED) |
| `scoping criteria` guidance (relocated 4, uses 15) | SC1 FAIL (15 > 10) until the 15 forwarder uses fall, which the embedding design in section 2 removes (INFERRED) |

Reading: this project has no T9/SC6 payoff on its own, as [dto-lowering-roadmap.md](dto-lowering-roadmap.md) already concluded for the DTO series. It is justified by OCP shape and by what each PR removes from `session`, not by touch gain.

### 1.6 Churn

`git log --since='180 days ago' --no-merges` over `session/`, grouped by the clusters above (VERIFIED, awk over the file list):

| | Commits touching the cluster | Also touching core | Cluster-only |
|---|---|---|---|
| any ent_* | 180 | 166 | 14 |
| backlog | 100 | 95 | 4 |
| entcore | 91 | 82 | 8 |
| sessionpr | 11 | 11 | 0 |
| guidance | 9 | 8 | 0 |
| flow | 7 | 6 | 1 |
| wake | 4 | 3 | 0 |
| webhook | 3 | 3 | 0 |
| stagecfg | 14 | 14 | 0 |

The roadmap's "149 of 170" is now 166 of 180 (same shape: 92%). The detail matters more: webhook, wake, flow and guidance are cold (3 to 9 commits in 180 days), so moving them is cheap and collision-free; backlog and entcore are hot and are where parallel work collides.

### 1.7 Test seam

The "second implementation" of any store port is still the real ent store on an in-memory SQLite database: [NewTestEntRepository](../../session/testing.go) (`testing.go:81`) copies a migrated template database (template built at [testing.go:33](../../session/testing.go), copied by `newSeededEntRepository`, [ent_repository_seed.go:32](../../session/ent_repository_seed.go)). `server/wakes/fakes_test.go` fakes the pane, lease, steerer, owners, notifier and target, but defines no `Store` fake (VERIFIED: its types are `paneDouble`, `doubleLease`, `fakeSteerer`, `fakeOwners`, `recordingNotifier`, `spyTarget`, `rowOpts`). So the "port needs two implementations" rule in [package-layout.md](../../docs/explanation/package-layout.md) means this plan adds no new interfaces; the existing consumer-defined `wakes.Store` ([server/wakes/ports.go:20](../../server/wakes/ports.go)) stays as it is.

## 2. The dependency cut

### 2.1 The one structural decision

Go forbids methods on a type from another package. The moved clusters hold `func (r *EntRepository) X`. The choice is therefore between (a) turning each into a free function over `*ent.Client`, (b) turning each cluster into its own store struct, or (c) a consumer-defined port per consumer. Evidence picks (b), then (c) only where it pays:

- Each target cluster needs `Client` plus at most one field (wake: `wakeChanged`; guidance: `pendingGuidance` and two hooks). That is a store struct of 1 to 3 fields.
- `EntRepository` embeds the store pointer, for example `*wakestore.Store`. Method promotion keeps the whole method set, so `Storage.Repo.X` (the forwarders in `storage_wake.go`, `storage_guidance*.go`, `storage_operator_wake.go`), `var _ PRSetReader = (*EntRepository)(nil)` ([backlog_item_prs.go:35](../../session/backlog_item_prs.go)) and the 153 external `session.EntRepository` references need no edits. Constructor sites that set `Client` must also build the stores. They are `NewEntRepository` ([ent_repository.go:264](../../session/ent_repository.go)), `NewEntRepositoryFromClient` ([:303](../../session/ent_repository.go)) and the seeded path (`newSeededEntRepository`, [ent_repository_seed.go:32-40](../../session/ent_repository_seed.go)). One helper `repo.initStores()` is called from all three, with a reflect-based test asserting every embedded store is non-nil after each constructor (a missed site would otherwise panic only at runtime).
- Embedding is safe for these clusters because no test reassigns `Client` for them (section 1.2). Flow markers, session-PR and stuck-episode do reassign it, which is one reason they are not in waves A and B.
- Store types move with their DTOs, so each move rewrites that cluster's use sites in one PR (the `TestNoReexportAliasesInSession` guard at [no_reexport_alias_test.go:15](../../session/no_reexport_alias_test.go) forbids bridging aliases; CLAUDE.md "No lasting type aliases").

Not chosen: plain functions (every call site and `Storage` forwarder changes, no gain) and a full port set (no second implementation; ADR-style rule in package-layout.md).

### 2.2 Ordered cuts

Each cut is a precondition of the PRs named in section 3.

| # | Cut | Why | Evidence |
|---|---|---|---|
| C1 | Test harness leaf `session/internal/entdbtest` giving a migrated in-memory `*ent.Client`, plus `EntSchemaCreateMu` lowered from `session` to `session/dbpool` | Leaf tests cannot import `session` (cycle: the embedding shell imports the leaf). Today the template builder calls `NewEntRepository` ([testing.go:33-58](../../session/testing.go)) and the schema mutex is `session.EntSchemaCreateMu` ([ent_repository.go:40-47](../../session/ent_repository.go)), already referenced from 3 non-session files | `grep -rw 'session\.EntSchemaCreateMu'` |
| C2 | Wake DTOs and sentinels to `session/domain` (`WakeTriggerData`, `CreateWakeTriggerInput`, `ErrWakeCapExceeded`, `ErrWakeTransitionRejected`, `DefaultWakePendingPerOwnerCap`) | Types are already pure: fields are `domain.*`, `time`, strings ([ent_repository_wake.go:36-74](../../session/ent_repository_wake.go)). `server/wakes` non-test code uses only these (plus [limits.go:377](../../server/wakes/limits.go)) from `session`; every other direct import of `server/wakes` has 0 `session` in its closure | `go list -deps` of `pkg/events`, `session/tymux`, `session/dialogpresence`, `telemetry`, `log`, `session/domain` each report 0; `server/wakes` reports 1 |
| C3 | Split `ent_repository_operator_wake.go` (88 LOC) in place: peer-guidance methods go to the guidance file, `ListWakeTriggersByStatus` to wake | It is the only wake-to-guidance edge (`GuidanceRequestData`, `guidanceRequestsToData`, `recordGuidanceAnswered`) | edge table, wake to guidance; `scoping relocate` does this without a package change |
| C4 | Guidance DTOs and sentinels to `session/domain` (`GuidanceRequestData`, `CreateGuidanceRequestInput`, `GuidanceRequestStatus*`, `ErrPendingCapExceeded`, `DefaultGuidanceRequestPendingCap`, `GuidanceQuestionsRetention`) | 30 external files, 74 refs for `GuidanceRequestData`; `scoping move --dry-run` lists 54 rewrites for the whole guidance move | dry-run output |
| C5 | Guidance hooks inverted in place: `recordGuidanceAnswered`, `countGuidanceDelivery` (in `guidance_answer_metrics.go`, declared as `EntRepository` methods) and the item-changed publish (`BacklogItemChange`, `ChangeItemUpdated`, `EntRepository.publishItemChanged`) become a `Hooks` struct passed to the store | These are the guidance store's only production edges into core/backlog (4 core + 2 backlog symbols) | edge table, guidance to core and backlog |
| C6 | Split `ent_repository_flow_markers.go`: reader half (`EntFlowReader.Markers/MarkerCapture/firstMarkerAt/CaptureSince`, lines 143-220) moves with the flow reader, writer half stays. `FlowMarkerKind` consts it reads are copied as unexported strings behind a pin test, like MIG-1's frozen constants; `utcPtr` is duplicated (3 lines) | `scoping move --dry-run` for the flow reader warns that the staying markers file uses `EntFlowReader`, `isMissingTableErr`, `flowReadErr`, and that `utcPtr` would need capitalising | dry-run output |
| C7 | Flow reader takes the workflow engine as a required argument instead of calling `NewDefaultWorkflowEngine` / `BuiltInStageOrder` | They are the reader's only production edge into core (edge table, flow to core); the one non-test consumer is [backlogflow/service.go:60](../../server/services/backlogflow/service.go), which today passes no engine | edge table, flow to core; grep |

Items deliberately not cut here: `EntRepository` stays in `session` as the aggregate shell (it owns `Client`, `driver`, migrations, the seed path, `Close`), `Storage.Repo` stays concrete, and `GetEntClient` keeps its 40 non-test and 175 test sites. Changing those is wave C, gated on the hub DTO lowering that [dto-lowering-roadmap.md](dto-lowering-roadmap.md) schedules.

## 3. PR sequence

Conventions for every PR: one agent wave, ready-for-review (not draft, per CLAUDE.md for this repo), merge commit (never squash), no force push, `make gazelle` output committed, no `git add -A`. Fresh worktree needs `make build` once (or `make sync`) so `gen/` and `session/ent/` exist. Check `df -h /home` first; each verification wave regrows the Go cache by tens of GB, so use a private `GOCACHE` and `go clean -cache` afterwards. Create worker worktrees from the feature branch and re-check open PRs touching `session/ent_*` at kickoff.

Guard traps that apply to all or most PRs (VERIFIED locations):

| Trap | Where | What it does to a move |
|---|---|---|
| No re-export aliases | [session/no_reexport_alias_test.go:15](../../session/no_reexport_alias_test.go) | Every type move must rewrite use sites in the same PR; use `go run ./tools/bzl/scoping move --from session --to <dir> --files ... [--dry-run]` |
| Scoping budgets and boundaries | [tools/bzl/scoping/budget.txt:4](../../tools/bzl/scoping/budget.txt), `boundaries.txt` | Add a row per new package, lower the `session` row by the measured delta in a `scoping-budget-raise:`-style commit line only if a cap must rise |
| depguard allow-list for new leaves | [.golangci.yml:124](../../.golangci.yml) (`isolate_migrations` is the template: `$gostd`, `entgo.io/ent/dialect`, `session/ent`, `log`) | Add one `isolate_<store>` rule per leaf; deny `session`, `server`, `github` |
| `nowrongdbentity` | `tools/lint/nowrongdbentity/analyzer.go` header | Covers "package session and its subpackages", so `session/*store` is covered with no change; no moved cluster touches `AnalyticsEvent` or `EscapeEvent` |
| `sinkguard`-tagged guards | [Makefile:871](../../Makefile) `test-delivery-guards-run`, [.github/workflows/build.yml:1193](../../.github/workflows/build.yml); files `server/deliverygate/sink_guard_test.go`, `server/deliverygate/flag_env_guard_test.go`, `server/services/terminal_write_*_test.go` | Keys are function-qualified names (for example `server/services.WatchSessions`) and `PkgPath` suffixes `/session`, `/session/instance` ([terminal_write_callers_test.go:252](../../server/services/terminal_write_callers_test.go)). A codemod that renames no function is safe; a PR that moves a function between packages must update the keys. Run the target for every PR that rewrites `server/wakes`, `server/services` or `server/mcp` |
| Delivery-gate system ids | [server/deliverygate/system_ids_test.go:41](../../server/deliverygate/system_ids_test.go) | Walks the whole module for `NewNotificationEvent(` calls; none of the ent_* or `storage*` files contains one (grep), so new leaf directories add no findings. Run it anyway when a PR touches `server/wakes` |
| Path-keyed allow-lists | [session/legacy_pr_readers_test.go:42](../../session/legacy_pr_readers_test.go), [legacy_item_pr_readers_test.go:31-53](../../session/legacy_item_pr_readers_test.go), [server/services/workflow_bypass_strict_test.go:56,400](../../server/services/workflow_bypass_strict_test.go), [workflow_bypass_inventory_test.go:121](../../server/services/workflow_bypass_inventory_test.go), `Makefile:1330-1335` (`pr-state-write-guard`), [session/pr_state_writer_guard_test.go:117-128](../../session/pr_state_writer_guard_test.go) | Key on `session/ent_repository_flow_markers.go`, `..._session_pr_guard.go`, `..._backlog.go`, `..._session_pr.go`. Waves A and B touch none of those paths except the markers file (E2 edits it in place, path unchanged). Wave C must rewrite them |
| Bazel | `session/BUILD.bazel` is Gazelle-owned ([.claude/rules/bazel-build-files.md](../../.claude/rules/bazel-build-files.md)) | `make gazelle` after every move; the new package gets `go_test` with the in-memory harness (no path data needed); `tools/bazel`, never bare bazel |
| goreleaser / make-only builds | `.goreleaser.yaml` runs `otelc` weave; `make build` is otelc-instrumented, `go build` is not | Run `make build` once on every PR that adds a package (otelc setup has a duplicate-symbol failure mode for multi-package weaves, see the otelc gotchas memory) |
| Windows/integration build tags | the W3 instance move failed in CI only on tagged files | Run `go vet -tags integration ./session/... ./server/...` and `GOOS=windows go vet ./session/...` on every PR that moves symbols |
| `session` TestMain and goleak | [session/integration_test.go:27](../../session/integration_test.go) | Leaf tests with pooled SQLite connections need `entdbtest` to own the connection keeper (like `WarmTestEntRepositoryTemplate`, [testing.go:64](../../session/testing.go)) |

Verify block shared by all PRs (substitute `<leaf>`):

```
make gazelle && git diff --exit-code -- '*.bazel'          # Gazelle clean
go build ./... && go vet ./session/... ./server/...
go list -f '{{join .Imports "\n"}}' ./session/<leaf> | grep -E 'stapler-squad/(session|server|github)$'   # must print nothing
go test ./session/<leaf>/... ./session -run 'TestNoReexportAliasesInSession|TestStateWriterAllowList|Legacy'
go test ./session/... -timeout=20m
go run ./tools/bzl/scoping boundaries && go run ./tools/bzl/scoping budget && go run ./tools/bzl/scoping inversions
make lint-custom && make test-delivery-guards-run
tools/bazel test //session/<leaf>/... //session:go_default_test    # target names per Gazelle output
make build                                                         # otelc
```

Rollback for every PR below: the change has no schema, data or flag effect (ent schema untouched, no migration), so rollback is `git revert -m 1 <merge sha>` of that one PR, then `make gazelle`. Revert in reverse merge order when later PRs build on it. There is no soak period.

### Wave A (no cross-PR prerequisite; run in parallel, disjoint files)

**E0. Test harness and mutex lowering (cut C1). Cost S.**
- Create `session/internal/entdbtest` with `NewClient(t) *ent.Client`: builds the schema once into a shared-cache in-memory template, restores per test (same technique as [ent_repository_seed.go:44-68](../../session/ent_repository_seed.go), but schema only, without startup migrations, so it has no `session` import). Move `EntSchemaCreateMu` to `session/dbpool`; rewrite the 3 outside references and in-session users (codemod via `scoping move`).
- Files: new package (about 120 lines), `session/ent_repository.go` (delete var), 3 external files, `.golangci.yml` rule.
- Verify: shared block plus `go test ./session/internal/entdbtest -race -count=3`. Being under `session/internal`, it is importable only from `session/...`, so `server` tests keep using `NewTestEntRepository`.
- Go/no-go: GO. Evidence: nothing depends on it; it only adds code.

**E1. Webhook store leaf `session/webhookstore` (cluster webhook). Cost M.**
- Move `ent_repository_webhook_registration.go` (394) and `ent_repository_webhook_lifecycle.go` (159) plus their two test files (706) into `session/webhookstore` as `type Store struct{ client *ent.Client }` with `New(client)`; the three methods (`InspectWebhookRegistration`, `ReconcileWebhookRegistration`, `ApplyWebhookLifecycle`) become `Store` methods; `EntRepository` embeds `*webhookstore.Store`. Helpers (`ledgerReplay`, `recordLedger`, `createWebhookRegistration`, ...) are already free functions over `*ent.Tx`.
- Rewrites: `server/services/webhook_registration_service.go:155,167` and `webhook_lifecycle_service.go` (the two production consumers; `grep` finds no others), `session/webhook_management_migration_test.go:71` (stays in session, uses `webhookstore.ErrWebhookSlugUnavailable`, `baseReconcileInput`, `scopeA` which become exported test helpers or are duplicated), `ent_repository_integration_credential.go` (its test `TestIntegrationCredential_...` at `ent_repository_webhook_registration_test.go:403` moves back into an integration-credential test file in session). `scoping move --dry-run` reports exactly these rewrites plus `integrationTokenPrefix`.
- Production back-edges: none. Receiver fields: `Client` only.
- Verify: shared block plus `go test ./server/services -run 'Webhook' ./session/webhookstore/...`.
- Go/no-go: **GO**. Evidence: 0 non-ent production symbols, 0 core-to-webhook production edges, 3 commits in 180 days, `scoping criteria` SC1/SC3 pass. Honest limit: touch gain 0.00 pp and a 553-line move.

**E2. Flow reader leaf `session/flowstore` (cluster flow, reader half). Cost M.**
- Cuts C6 and C7 first (in-place commit inside the PR), then move `ent_repository_flow.go`, `_events`, `_items`, `_stream`, `_stuck`, `_verdicts` and the reader half of `_markers` (about 870 LOC) and tests (`flow_test`, `flow_stream_test`, `flow_perf_test`, `flow_concurrency_test`, `flow_verdicts_test`, `flow_stuck_test`, 834 lines). `EntFlowReader` already takes only `client *ent.Client` ([ent_repository_flow.go:13-25](../../session/ent_repository_flow.go)).
- Rewrites: [server/services/backlogflow/service.go:60](../../server/services/backlogflow/service.go) (the single non-test consumer), 14 files total per `scoping move --dry-run`. `markers` writer half and `ent_repository_flow_markers_test.go` stay in session and keep using `NewEntFlowReader` through `flowstore`.
- Test coupling: the stuck test uses `newSeededConfiguredWorkflowEngine` (twin needed, per dry-run `TWIN`) and `Storage.CreateBacklogItem`; tests that seed through `Storage` stay in session as integration tests, the rest seed through `entdbtest` (E0 is therefore a dependency for the test move only; the production move can land first with tests staying).
- Go/no-go: **GO**, sequenced after E0 only for the test move. Evidence: `scoping leaves` already lists these files as a leaf (702 LOC), importers 1, 7 commits in 180 days.

### Wave B (after the named prerequisite)

**E3. Wake DTO lowering to `session/domain` (cut C2). Cost S/M. Recommended first PR.**
- Move the five wake symbols to `session/domain/wake_trigger.go` (the domain file already holds `WakeStatus`, `WakeSpec`, `WakeAction`). Codemod over 27 external files (15 non-test, 12 test) and 6 in-session files (`ent_repository_wake.go`, `_wake_transitions.go`, `_operator_wake.go`, `_wake_test.go`, `storage_wake.go`, `storage_operator_wake.go`). `session/domain/purity_test.go` stays green because the types use only `time`, strings and `domain` types.
- Result: [server/wakes/ports.go](../../server/wakes/ports.go) and 11 other files drop the `session` import; `go list -deps ./server/wakes | grep -c 'stapler-squad/session$'` goes from 1 to 0 (non-test closure; wakes tests still use the real repository, 11 files). This is the one directly measurable leaf result in the project, and the gate that [dto-lowering-roadmap.md](dto-lowering-roadmap.md) named for WAKE-1 (pkg/events no longer imports `session`: `go list -deps ./pkg/events` reports 0).
- Verify: shared block plus `go list -deps ./server/wakes | grep -c 'stapler-squad/session$'` (expect 0), `go test ./server/wakes/... ./server/mcp/... ./server/services -run 'Wake|Guidance'`, `make test-delivery-guards-run` (the wake delivery path is a terminal sink).
- Go/no-go: **GO**. No prerequisite. Risk: 33 files in one codemod; schedule in a quiet window and regenerate rather than hand-merge if `main` moves (the DTO roadmap's own SENT-1 advice).

**E4. Wake store leaf `session/wakestore` (cluster wake, after E3 and C3). Cost M.**
- Move `ent_repository_wake.go`, `ent_repository_wake_transitions.go` (606 LOC), the `ListWakeTriggersByStatus` method, and `ent_repository_wake_test.go` (548) into `Store{client *ent.Client; wakeChanged atomic.Pointer[func()]}`; `EntRepository` embeds it, so `storage_wake.go` forwarders and `wakes.Store` satisfaction by `*EntRepository` and `*Storage` are unchanged. `SetWakeChangeHook` ([ent_repository_wake.go:79](../../session/ent_repository_wake.go)) moves with the hook field.
- `utcPtr` is used by `ent_repository_flow_stuck.go`; after E2 it is a 3-line duplicate, so there is no residual edge. `wakeStatusStrings` goes with the store.
- Verify: shared block plus `go test ./server/wakes/... -race` (the 32 test call sites in `server/wakes` run against the real repo), `go test ./session/wakestore/...`.
- Go/no-go: **GO after E3**. Evidence: 0 non-ent production symbols besides `recordGuidanceAnswered` (removed by C3), `wakeChanged` is the only extra field, 4 commits in 180 days. Real second implementation: none (tests use real ent), so no new interface.

**E5. Guidance DTO lowering to `session/domain` (cut C4). Cost M; split into two PRs by directory if more than 3 PRs are open.**
- Codemod `GuidanceRequestData`, `CreateGuidanceRequestInput`, status consts, `ErrPendingCapExceeded`, caps. `scoping move --dry-run` shows 54 rewrites (33 non-test): `server/services` (about 25), `server/mcp` (about 6), `server/wakes_source_adapters.go`, `session/backlog/backlog_lifecycle_stuck_test.go`, `session/diagnose_result.go`, `ent_repository_backlog.go`, plus `storage_guidance*.go`, `storage_operator_wake.go`.
- Go/no-go: **GO**. Mechanical, but wide; same quiet-window rule as E3. No payoff on its own beyond unblocking E6 and shrinking `server/services`' `session` surface (30 files import `session.GuidanceRequestData` today).

**E6a. Guidance hooks inversion (cut C5), in place. Cost S.** `recordGuidanceAnswered` and `countGuidanceDelivery` and the item-changed publish become fields of an unexported `guidanceHooks` struct set at repository construction; behavior identical, guarded by the existing guidance tests ([ent_repository_guidance_test.go](../../session/ent_repository_guidance_test.go), `guidance_answer_metrics_test.go`). Go: **GO**; can ship with or before E5.

**E6b. Guidance store leaf `session/guidancestore` (after E5, E6a). Cost M/L (one wave if E5 and E6a are merged; otherwise split).**
- Move 4 prod files (950) and 4 test files (1,226). Store fields: `client`, `pendingGuidance` (plus the write hook installer `watchGuidanceWrites`, [ent_repository_seed.go:36](../../session/ent_repository_seed.go) calls it for seeded databases, so `initStores()` must call it for all three constructor paths). `PurgeTerminalGuidanceQuestions` ([ent_repository_guidance_purge.go](../../session/ent_repository_guidance_purge.go)) moves with it.
- Verify adds: `go test ./server/services -run 'Guidance|HookReply|Question'`, `go test ./server/mcp -run 'Guidance|PeerHelp|OwnerAction'`, `make test-delivery-guards-run`.
- Go/no-go: **GO after E5 and E6a**. Evidence: after hooks inversion the production edges are 0; 9 commits in 180 days. Risk: `ent_repository.go` holds `pendingGuidance pendingGuidanceCache` and calls `watchGuidanceWrites` (edge table, entcore to guidance); the move must update the struct and all three constructors or the cache silently stops invalidating (add a test that answers a question and reads it back through the cache).

**E7. Per-consumer ports and shell slimming (optional, only where a real second implementation exists). Cost S per consumer.** Candidates: `server/services/webhook_registration_service.go` takes `*session.EntRepository` today; once E1 lands it can take a consumer-defined interface of 3 methods implemented by `*webhookstore.Store`, constructed at `server/webhook_management.go:66`. Do this only if a test fake is wanted (none exists today). Go/no-go: **NO-GO as a standing item**; do per consumer on demand, because the rule in [package-layout.md](../../docs/explanation/package-layout.md) (interfaces need two implementations or a genuine seam) is not met by evidence now.

### Wave C (blocked; prerequisites and gates, in dependency order)

| PR | Cluster | What it needs first | Evidence for the block | Go/no-go now |
|---|---|---|---|---|
| E8 | stagecfg: the four free-standing repos (1,261 prod, 620 test) | Stage, transition, gate, workflow and pipeline-mode DTO families lowered to `domain` | Already free structs over `*ent.Client` and listed by `scoping leaves`, but their `*Input`/`*Data` types live in four non-ent files and `StageDefinition`/`TransitionDefinition`/`SerializeStageExecutors` (20 production symbols); ports return `*ent.Workflow` / `*ent.PipelineMode` (so no OCP or ent-containment gain, as [tierc-roadmap.md:175](tierc-roadmap.md) PERS-4 concluded); [dto-lowering-roadmap.md:404](dto-lowering-roadmap.md) gates these families on a consumer that sheds `session` | **NO-GO** until a consumer exists; re-measure with `scoping move --dry-run` first |
| E9 | sessionpr (1,184 + 1,047) | (a) `PROwner`, `SessionPROwner`, `ItemPROwner`, `ItemGuardView`, `PRObservation*` lowered to `domain` (14 and 12 in-session files, 34/20 external refs); (b) `session_pr_backfill.go`, `session_pr_gc.go`, `backlog_item_pr_link_write.go` stop declaring `EntRepository` methods; (c) owner decision on moving the PR-write guard (`Makefile:1330`, `pr_state_writer_guard_test.go:117`) | 32 production and 31 test symbols flow core to sessionpr; 3 tests reassign `repo.Client`; 3 path-keyed guards name `ent_repository_session_pr.go` | **NO-GO** until (a)-(c); then one L wave, likely two PRs |
| E10 | backlog (3,445 + 1,900) | BL-A to BL-F DTO series ([dto-lowering-roadmap.md](dto-lowering-roadmap.md)) closed; the 48 backlog methods in `storage_backlog.go` and the 4 in `ac_audit.go`/`diagnose_result.go`/`storage_backlog_jules.go` relocated; `CallbackDispatcher`, `ItemChangePublisher`, `ChainFirer` made injectable (4+4+3 uses) | 56 production symbols core to backlog and 20 back; 100 of 180 recent ent_* commits (95 also touch core); `RawTransitionBacklogItemStatus` is exempted by path in `workflow_bypass_inventory_test.go:121` and `workflow_bypass_strict_test.go:56` | **NO-GO**; this is the cluster that decides whether the "~12k lines" ever leave, and it is the last |
| E11 | entcore (2,826 + 1,443): instance CRUD, rules, analytics, projects, shells | Instance DTO lowering; rule/project/shell slices (about 250, 100, 70 lines) could be small stores but each is under the SC6 floor | 91 commits touching it, 82 also core; `ent_repository.go` is 2,436 lines | **NO-GO** as a split; shell residue stays by design |
| E12 | markers writer and stuck-episode (`ent_repository_flow_markers.go` writer half 130, `ent_repository_stuck_episode.go` 84) | `MarkerLimiter`, `FlowMarkerData` (core, [flow_marker_limiter.go](../../session/flow_marker_limiter.go) 105 lines, [flow_marker.go](../../session/flow_marker.go) 89) moved with it; the two tests that reassign `Client` made injectable | `repo.Client =` at the two test sites in 1.2 | **NO-GO** now; small enough for a later E2 follow-up |

## 4. What can go first with no prerequisite

| Sub-leaf | PR | Prerequisite | Why it can go first |
|---|---|---|---|
| Webhook | E1 | none (E0 only if its tests should use the shared harness; the move itself can keep tests in `session` for one PR) | 0 production back-edges, 2 production consumers |
| Wake DTOs | E3 | none | pure types, 33 files, measurable `server/wakes` result |
| Flow reader | E2 | none for production; E0 for moving all tests | self-contained struct, 1 consumer |
| Harness | E0 | none | additive |
| Guidance hooks inversion | E6a | none | in-place, behavior-preserving |

Wake store (E4) needs E3 and C3; guidance store (E6b) needs E5 and E6a. The roadmap's "wake+webhook and guidance sub-leaves could be re-proposed after replay" holds; the replay says 0.00 pp, so the justification is OCP and `session` shrinkage, not touch gain.

## 5. Risks and honest go/no-go

| Risk | Evidence | Mitigation |
|---|---|---|
| Embedded store not built on a constructor path, runtime nil panic | 3 construction sites ([ent_repository.go:264,303](../../session/ent_repository.go), seed path); 2 `&EntRepository{...}` literals in `review_gate_test.go` | `initStores()` helper plus a reflect test; the test literals only call `backfillLockFilePath`, which touches no store |
| Method promotion collisions | Embedding several stores promotes all methods; a duplicate name is a compile error at the embedding site | Compile-time; method names are already unique on `EntRepository` today |
| Codemods collide with parallel backlog work | 166 of 180 ent_* commits also touch core; roadmap notes the 3-open-PR cap | E3 and E5 land in quiet windows, regenerate rather than hand-merge |
| Guard keys silently stop matching | path-keyed allow-lists in section 3 | Only waves C touch the keyed paths; run the guard tests in the shared block |
| Test binary and `goleak` behavior of leaves | `session/integration_test.go:27` TestMain; `WarmTestEntRepositoryTemplate` | E0 owns connection keeping; leaf tests call it from their own `TestMain` |
| No Bazel/T9 payoff | `scoping touch` 0.00 pp for every sub-leaf and the union | Stated in every PR body; do not claim a cache gain |
| `scoping criteria` for guidance fails SC1 on forwarder counts | section 1.5 | Embedding removes the forwarders from the count; re-run after E5 |
| Stale-instruction and parallel-work drift | repo memory: check live state at ship | Re-read `git log origin/main -- session/ent_*` before each PR |

Go/no-go summary:

| PR | Call | Basis |
|---|---|---|
| E0 | GO | additive, no dependents |
| E1 | GO | 0 production back-edges, 2 consumers |
| E2 | GO | already a `scoping leaves` leaf |
| E3 | GO | measurable `server/wakes` closure change |
| E4 | GO after E3 | 0 production edges once C3 done |
| E5 | GO | mechanical, wide |
| E6a | GO | in place |
| E6b | GO after E5, E6a | edges 0 after hooks; cache install risk tested |
| E7 | NO-GO as standing item | no second implementation exists |
| E8 | NO-GO | DTO gate unmet, ports return `*ent.*` |
| E9 | NO-GO until prerequisites a-c | 32+31 back-edge symbols, 3 guards |
| E10 | NO-GO | hot, 56 back-edge symbols, BL series open |
| E11 | NO-GO | hot, no leaf shape |
| E12 | NO-GO now | tests reassign `Client` |

What stays in `session` after A and B: about 9.0k of the 12.0k ent_* production lines (moved: webhook 553, flow reader about 870, wake about 620, guidance 950 = about 3.0k, about 6.8% of `session`'s 44.3k; the session budget row should drop by the measured delta). That is a ceiling for this project until the hub DTOs are lowered; claiming more would be unsupported.

## 6. Cost (price-weighted, per ESTIMATION.md; no human time)

Bands are INFERRED, calibrated on the class table in `~/.claude/skills/sdd/skills/ESTIMATION.md` (S 30-80k raw tokens, M 80-180k, L 180-300k per worker run). CU assumes output about 12% of tokens and about 70% of input read from cache: roughly 0.93 CU per raw token.

| PR | Class | Agent runs | Raw tokens (with x1.5-2 verification multiplier) |
|---|---|---|---|
| E0 | S | 1 worker + 1 review | 80-120k |
| E1 | M | 1 worker + 1 review | 200-300k |
| E2 | M | 1 worker + 1 review | 200-300k |
| E3 | S/M (wide codemod, little judgment) | 1 worker + 1 review | 150-220k |
| E4 | M | 1 worker + 1 review | 200-300k |
| E5 | M, 1 or 2 PRs | 1-2 workers | 200-400k |
| E6a | S | 1 | 60-100k |
| E6b | M/L | 1-2 workers | 250-400k |
| Waves A+B total | | about 10-12 runs | 1.3-2.1M raw, about 1.2-2.0M CU (Medium band, 0.5-3M) |
| Wave C prerequisites and moves (E8-E12) | L to XL when planned in full | not estimated per PR here | Large to XL (3M and up), INFERRED from 90-file rewrite scale; re-estimate after E9's dry-run |

Waves and critical path: wave 1 = E0, E1, E3, E6a (disjoint files, parallel); wave 2 = E2, E4, E5; wave 3 = E6b. Serial depth 3.

Wall-clock blockers (the only place calendar time belongs): full CI per PR (including Bazel and the sinkguard job); quiet windows for E3 and E5; the 3-open-PR cap; owner decisions for wave C (whether `EntRepository` stays the long-term aggregate shell, which this plan recommends, and the PR-write guard move in E9); no soak, because there is no behavior or flag change.

Overrun checkpoint: if spend on completed PRs exceeds the table by more than 25%, pause and report the cause (informational).

## 7. Self-review log

Checked before opening the PR:
- Every `[text](path)` link in this file resolves to an existing path (script in the PR checklist: extracted links, `test -e`).
- LOC and file counts recomputed: 63 files, 12,008 + 9,002 lines, cluster columns sum to both.
- Claims corrected during research versus the roadmap: GetEntClient 37 callers (non-test lines 40, tests 175), 9 staying files (10), 149/170 commits (166/180), "pr_set.go drags github" (now in `session/domain`), "depends on P1/P2 that do not exist" (MIG-1 and the Instance move landed; the PR records live in domain).
- Not verified: `scoping criteria` inputs for wake and guidance are my reading of "relocated" and "uses"; Bazel-side test target names; that `go list -deps ./server/wakes` reaches 0 after E3 (inferred from every other direct import having 0).
