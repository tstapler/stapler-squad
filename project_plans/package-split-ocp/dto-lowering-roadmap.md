# DTO lowering roadmap (ADR-011), 2026-11-11

Output of the `dto-lowering-design` workflow (90 agents: five analysts over backlog DTOs, instance persistence, small stores, consumers/ports and the ent boundary; four review lenses per proposal; one synthesizer). Started at the owner's direction after `tierc-roadmap.md` concluded that further leaf extraction from `session` and `server/services` is blocked on lowering the `Storage`/`EntRepository` DTOs. Every count is the reviewers' and is INFERRED until the per-stage spikes confirm it.

## Assessment

All numbers come from the reviewed data. I ran nothing myself (read-only worktree); every count is the reviewers' and is INFERRED until the per-stage spikes confirm it.

Verdict: the DTO lowering has no standalone Bazel or cache payoff. The payoff lens said so for every proposal. The reasons are structural. session/domain is imported by session and all consumers, so lowering DTOs alone drops no import edge. Churn is concentrated in the same hot files. The ledger's SC6 skips are unchanged. I therefore scheduled the backlog DTO series as a strangler prerequisite (BL-A to BL-F) with explicit gates. It uses a separate backlogdto sub-package for stable-dependencies reasons. It is bridged by one temporary alias file with a sunset ratchet test and rewritten in chunked mechanical PRs. The series leaves main green at every step and is closed by BL-E.

Items that win on measurable or structural grounds are front-loaded:
- MIG-1: the only prior SC6 pass, to be re-measured.
- PLUG-1 and STUCK-1: real OCP seams. They remove the constructor that imports every plugin and the hard-coded detector list, and they need no DTO prerequisite.
- WAKE-1/WAKE-2 plus DIALOG-1: the only concrete path to a leaf. server/wakes can become session-free only if pkg/events is lowered, so a recorded spike gates that path.

Ten proposals were rejected outright (BL-3, DTO-2-sessionstore, DTO-3-prstore, DTO-S3, DTO-S4, DTO-3-ent, DTO-4, DTO-5, ENT-1, ENT-3). Their useful slices were folded into NF-1, STUCK-1 and WAKE-2.

Open-closed status after the roadmap:
- New migration, tracker plugin and stuck detector become new-file-plus-one-registration changes at the composition root or the migrations package.
- Wake-kind switches and the stage/pipeline-mode/gate families remain open violations.
- No wave leaves the stable core edited for every new variant. MIG-1 keeps a core-to-detail import edge, which is flagged in the owner questions.

## Wave 1

Three independent in-place or leaf-creating stages with no wide codemod. Each opens a real extension point or has a ledger-measured SC6 pass, so they are the best payoff per cost and need no DTO lowering. Run `go run ./tools/bzl/scoping replay` at the merge base before MIG-1 (the ledger numbers are stale). They touch disjoint directories (session migrations, server wiring plus services, session/backlog), so they can land in parallel.

### MIG-1: session -> session/migrations

- Cost: M (one agent wave, about 600 LOC moved, 7 straddling test files to re-point)
- Payoff: The only item with a prior SC6 pass on record. Ledger tierB session p2: lib -0.34 pp, test -0.31 pp (SC6 needs lib >= 0.10 pp or test >= 0.30 pp). The test margin is thin and the numbers are stale, so re-run `scoping criteria` and `scoping replay` first and stop if SC6 fails. Moves about 606 of 82.7k session LOC and the migration tests out of the session test binary. Honest limit: session still imports migrations, so a migrations edit still rebuilds session. The gain is a smaller session test binary and the migration code being cached on its own. This also fixes the OCP gap that adding a migration means editing session.
- Prerequisites:
  - Record `scoping criteria` and `replay` at the merge base. If SC6 fails, drop the item.
  - Decouple status_corruption_repair.go from the Status enum with local frozen constants (maxValidPersistedStatus=8, minShiftEligibleStatus=5) plus a session-side pin test asserting equality with int(Failed) and int(Restoring). This removes the former DTO-1 dependency.
  - Re-grep path-keyed guards for ent_repository_migrations paths.
- Files moved:
  - The 7 ent-only migration implementation files listed in DefaultStartupMigrations (4 backlog_item_*_migration/backfill files, status_corruption_repair.go, workflow_updated_at_utc_migration.go, github_pr_url_backfill.go; confirm exact names with `grep -n` in ent_repository_migrations.go), about 600 LOC
  - session/ent_repository_migrations.go keeps only the thin adapter (funcMigration wrapper, WithMigrations, the call into migrations.Default)
  - BackfillBacklogItemPublicIDs stays in session because it needs hub helpers (resolveBacklogItemLookup, BacklogItemToData). It is injected into migrations.Default at its current pinned slot.
- Exports:
  - migrations.Migration{Name string; Run func(ctx, Env) error}
  - migrations.Env{Client *ent.Client; Driver dialect.Driver /* may be nil */}
  - migrations.Hooks{PublicIDBackfill Migration; PRURL func(path, owner, repo string, n int) (string, bool)}
  - migrations.Default(Hooks) []Migration
  - exported run functions or constructors used by the in-session tests
- Ports:
  - Migration (already exists in session as the consumer-defined interface; keep it, with a migrations.Migration to session.Migration adapter in session). There are 2+ implementations: the production migrations plus test fakes, already used via WithMigrations. No new interface is introduced.
- Extension points:
  - Adding a startup migration = one new file in session/migrations plus one entry in migrations.Default. NewEntRepository and session are not edited.
  - Composition roots and tests can still override the list with WithMigrations.
- Tests to move:
  - Pure migration tests that need only an ent client move to session/migrations (add at least one Env-level test so the verification is not vacuous).
  - The 7 straddling session tests that use createTestStorage-style helpers stay in session and call the exported run functions with Env{repo.Client, repo.MaintenanceDriver()}.
- Guard changes:
  - Add a .golangci.yml depguard rule isolate_migrations: allow $gostd, session/ent/**, entgo.io/ent/dialect, and log; deny session, github, server.
  - Add a boundaries.txt line and a new budget.txt row for session/migrations; lower the session cap by the measured delta.
  - Update session/BUILD.bazel srcs; add session/migrations/BUILD.bazel (Gazelle diff clean).
  - Add a norawgitopen/norawghrequest check only if the moved files use them (expected: none).
- Verify:
  - go build ./... && go vet ./session/... && go test ./session/... ./session/migrations/...
  - go list -f '{{join .Imports "\n"}}' ./session/migrations | grep -E 'stapler-squad/(session|github)$' (must print nothing)
  - go run ./tools/bzl/scoping boundaries; go run ./tools/bzl/scoping budget; go run ./tools/bzl/scoping criteria; replay before and after
  - make lint-custom; gazelle diff clean
  - Rollback: revert the single PR. The Migration order test and the golden name/order test guard behaviour.
- Composition-root wiring: session.NewEntRepository calls migrations.Default(Hooks{PublicIDBackfill: backfillMigration, PRURL: sessionPRURL}) and passes Env{repo.Client, repo.driver}. Constructor signatures are unchanged, so the 86 NewEntRepository call sites are untouched. The order test (OrderPinned) must still pass; add a golden test of names and order. Known remaining edge: session imports the migrations list (core to detail). This is an owner question.

### PLUG-1: session -> session (in place); wiring in server/dependencies.go

- Cost: S to M (about 12 files, mostly tests)
- Payoff: OCP only, with no Bazel payoff. This is the cheap part of BL-2: it removes the constructor that imports every concrete tracker plugin and makes the services and mcp call sites stop building concrete plugins. The physical move into itemsource leaves is deferred, because the ledger skipped backlogplugin on SC6 (lib 0.00, test -0.024 pp) and the plugins need BacklogItemData in domain first.
- Prerequisites:
  - Run `grep -rE 'NewDefaultRegistry|session\.(New)?(GitHubIssues|GitHubPRs|Jira|Linear)Plugin|ParseJiraIssueRef|ParseLinearIssueRef|JiraIssueRef'` over all Go files and record the full call-site list in the PR. Known: server/dependencies.go, server/mcp/tools_backlog.go, tools_backlog_import_tracker.go (+test), server/services/backlog_github_forward_sync.go:139 (+test), backlog_github_progress_sync.go:263 (+test), backlog_linear_forward_sync_test.go:107-108, backlogsvc service, sync and sources_preview tests, and in-package backlog_sync.go and feature_controller.go.
- Files moved:
  - No files move. Delete session.NewDefaultRegistry and relocate TestNewDefaultRegistry_should_IncludeLinearAndJira to the server composition-root test.
  - Add a BuildPluginRegistry(...) function in server (dependencies area) that registers github_issues, github_prs, linear and jira.
- Exports:
  - None new in session. The existing PluginRegistry, ItemSourcePlugin and PluginConfig are unchanged.
- Ports:
  - None new. Reuse the existing consumer-defined capability interface externalIssueStateUpdater in server/services. The two services sites that build `&session.GitHubIssuesPlugin{}` resolve the plugin via registry.Get("github_issues") plus a capability type assertion. mcp import-tracker resolves fetchers via the registry instead of calling NewLinearPlugin and NewJiraPlugin. ParseLinearIssueRef and ParseJiraIssueRef remain session helpers until the physical move.
- Extension points:
  - New tracker = a new plugin file implementing ItemSourcePlugin plus one Register line in BuildPluginRegistry. The session registry code and the sync loop are unchanged.
- Tests to move:
  - Registry-composition test moves to server. The services and mcp tests that assert `_ externalIssueStateUpdater = (*session.LinearPlugin)(nil)` stay but resolve through the registry or keep the type assertion.
- Guard changes:
  - No depguard change. Add a go test or analyzer rule: no non-test file outside session and the composition root references session.New*Plugin. If a path-keyed guard names backlog_plugin_*.go, re-grep it (the earlier 'none found' was unverified).
- Verify:
  - go build ./... && go vet ./server/... ./session/... && go test ./server/... ./session/...
  - grep -rn 'NewDefaultRegistry' (zero hits)
  - Behaviour: the same plugin names are registered in the same order; keep the registry golden test.
  - Rollback: revert the single PR.
- Composition-root wiring: server/dependencies.go calls BuildPluginRegistry once and passes the registry into SyncLoop, the backlog services, and mcp tool deps. session no longer lists concrete plugins in a constructor.

### STUCK-1: session/backlog -> session/backlog (in place; port in session/backlog/ports.go, fake in session/backlog/backlogtest)

- Cost: M (one wave, depends on a per-detector method matrix)
- Payoff: OCP plus a genuine test seam. A new stuck detector no longer edits stuck_detectors.go or the listener. Detector unit tests stop needing sqlite. No Bazel payoff (session/backlog still imports session and session/ent). The stated end state for session/backlog is NOT a leaf; this stage does not claim it.
- Prerequisites:
  - Produce the per-detector `er.*` method matrix (go/types, non-test). Known counts: FindOpenStuckStates ~22 non-test, MarkStuck 14, MarkStuckNotified 14, ResolveStuck. Part (b) is conditional on every detector using only methods that exist on both Storage and EntRepository.
  - Do not touch ChainFirer. NewChainFirer must keep the exact *EntRepository instance because of the shared callbackDispatcher and itemChangePublisher wiring.
- Files moved:
  - session/backlog/stuck_detectors.go (defaultStuckDetectors becomes an exported DefaultStuckDetectors() constructor with the pinned order, plus an injectable option on the listener)
  - session/backlog/ports.go (new)
  - session/backlog/backlogtest/fake_stuck_store.go (new in-memory fake) plus a contract test run against both the sqlite-backed EntRepository and the fake
- Exports:
  - WithStuckDetectors(...StuckDetector) listener option
  - DefaultStuckDetectors() []StuckDetector
- Ports:
  - StuckStore, defined in session/backlog (consumer): FindOpenStuckStates, MarkStuck, MarkStuckNotified, ResolveStuck, UnresolvedBlockerItemIDs (the 5 methods that exist on both Storage and EntRepository). It is used only if the matrix shows all detectors fit. RemediationDue, RemediationBlocked and TransitionDenialHolds exist only on *Storage and stay out. If any detector needs more (for example the six Find*Items that return []*ent.BacklogItem), ship only part (a) and keep Run(ctx, *session.EntRepository).
- Extension points:
  - Detectors: a new file implementing StuckDetector, registered by the composition root in the injected slice. The ReconcileStuck loop is closed for modification.
  - Test seam: the in-memory StuckStore fake, validated by the shared contract suite.
- Tests to move:
  - No tests move. Add detector tests driven by the fake and keep the sqlite-backed tests for CAS and cascade semantics.
- Guard changes:
  - Check the session/backlog budget.txt row (files=29 loc=13802 imports=15): ports.go must not break it. If the ratchet is exceeded, lower elsewhere rather than raising it.
  - interface-pollution check: the port stays only if the fake and the contract test land in the same PR.
- Verify:
  - go build ./session/... ./server/... && go test ./session/backlog/... -race
  - Order test: the injected default list preserves the pinned detector order.
  - Contract suite passes against both implementations.
  - Rollback: revert the single PR.
- Composition-root wiring: server/dependencies.go passes backlog.DefaultStuckDetectors() plus any extra detectors into the listener options (explicit slice instead of a hard-coded in-method list).

## Wave 2

SENT-1 is the one wide mechanical codemod (about 110 files) and is the prerequisite for the domain/DTO series. It lands alone-first in a quiet window after the wave 1 PRs have merged, and the PR is atomic (no alias ever on main). DIALOG-1 is a small stdlib-only leaf that touches different files and can run in parallel. Regenerate the codemod rather than hand-merging if main moves.

### SENT-1: session -> session/domain

- Cost: M (mechanical codemod; the wide diff and merge-conflict handling dominate cost)
- Payoff: No cache payoff and no leaf on its own (said plainly; the reviewers agreed). It removes the ErrNotFound identity coupling that blocks every later stage: the wakes leaf (operator_api.go:36), backlogsvc, backlogdto. It also gives future store packages a lower place to wrap their errors. Measure with `go list -f` that the files using only the sentinels no longer need session for errors.
- Prerequisites:
  - Wave 1 merged. Check open PRs (gh pr list) and live worktrees that touch repository.go before starting.
  - Run the go build spike on a throwaway worktree first and record the result in the PR.
- Files moved:
  - session/repository.go lines 13-19: ErrPreconditionFailed, ErrNotFound, ErrConflict, which become session/domain/errors.go
  - Rewrite about 110 files: 52 non-test files outside session (10 packages: server 5, mcp 8, reviewqueue 1, services 14, backlogsvc 17, sessionbundle 1, wakes 1, session/backlog 4, prpoller 1), about 189 bare in-package lines in session, about 118 test lines. Exclude the four unrelated ErrNotFound declarations in other packages. Recount with `rg` before starting.
- Exports:
  - domain.ErrNotFound
  - domain.ErrConflict
  - domain.ErrPreconditionFailed (same variable identity and the same message text)
- Extension points:
  - None added, and none needed. Convention for later stores: store-specific errors live in the store package and wrap domain.ErrNotFound or domain.ErrConflict, so consumers use errors.Is on one class.
- Tests to move:
  - session tests that assert identity (about 79 lines) are rewritten in the same PR.
- Guard changes:
  - Add session/domain/purity_test.go: `go list -deps ./session/domain` must show no module package other than itself (and, from BL-A, session/domain/*).
  - Update session/domain/BUILD.bazel srcs and deps; update the BUILD deps of every touched package (Gazelle diff clean).
  - budget.txt: lower the session import and LOC caps only if measured lower. Check boundaries.
- Verify:
  - go build ./... && go vet ./...
  - rg 'session\.Err(NotFound|Conflict|PreconditionFailed)' returns nothing; no `Err... = domain.` var remains in session/repository.go
  - go test ./session/... ./server/... ./pkg/...
  - make lint (depguard); go run ./tools/bzl/scoping boundaries and budget
  - Codemod command committed in the PR description (gofmt -r or ast-grep, then goimports) so conflicts are regenerated, not hand-merged.
  - Rollback: revert the single atomic PR.
- Composition-root wiring: None.

### DIALOG-1: session -> session/dialogpresence

- Cost: S
- Payoff: Removes one of the five session symbols server/wakes needs (PaneShowsDialog) and gives guidance_ack_detector and guidance_dialog_gone_sampler a session-free pure helper. Prerequisite for the wakes library leaf. No standalone Bazel gain (about 330 LOC and low churn). The destination is a new stdlib-only package, not session/detection, because detection drags config, proto and executor deps.
- Prerequisites:
  - Grep all users of the moved helpers (ansiEscape, dialogGutter and dialogFooterPrefix are shared with pane_reply.go and question_tabbed.go). Export what other session files still need from dialogpresence; do not copy it (dupl gate).
  - Confirm `go list -deps` of the new package is stdlib only.
- Files moved:
  - session/dialog_presence.go and dialog_presence_test.go. Declarations: PaneShowsDialog, DialogPresence, DialogPresenceFromCapture, DialogMatch (if it lives there), and the unexported helpers presenceLines, isDialogMarker, isRuleLine, isComposerPrompt, composerAtBottom. Add any shared exports needed by pane_reply.go and question_tabbed.go.
  - Callers rewritten: server/wakes/pane.go:63; server/services/guidance_ack_detector.go:168-175; guidance_dialog_gone_sampler.go:46,65; in-session users. No alias.
- Exports:
  - dialogpresence.PaneShowsDialog
  - dialogpresence.DialogPresence and DialogPresenceFromCapture
  - (plus the shared constants and helpers confirmed in the prerequisite)
- Extension points:
  - None; pure functions. No registry is needed.
- Tests to move:
  - dialog_presence_test.go (no other session test uses its helpers). Move the testdata/reply_dialogs fixtures only if they are used solely by it.
- Guard changes:
  - Re-point the path-keyed map in session/wake_polling_guard_test.go and the BUILD.bazel srcs lists.
  - Add a depguard rule allowing only $gostd for session/dialogpresence, a boundaries.txt line, and a budget row; lower the session caps.
- Verify:
  - go build ./... && go vet ./session/... ./server/... && go test ./session/... ./server/wakes/... ./server/services/... -run 'Dialog|Guidance|Pane'
  - go list -deps ./session/dialogpresence | grep stapler-squad (only itself)
  - Rollback: revert the single PR.
- Composition-root wiring: None.

## Wave 3

Starts the owner-chosen DTO lowering. BL-A is the leaf-most backlog DTO group (it must precede BL-B because ItemSessionSummary and BacklogItemData embed these types). NF-1 is the cheap not-found cleanup. WAKE-1 is gated on the events spike. All three touch disjoint files (repository.go and storage_backlog.go; backlogsvc; wake files).

### BL-A: session -> session/domain/backlogdto

- Cost: L (multi-PR series start: bridge PR first, then rewrites in later waves)
- Payoff: Milestone gating, no standalone Bazel win. The stable-dependencies fix: 73 commits/180d on repository.go and 60 on storage_backlog.go must not invalidate the stable domain consumers (wake_trigger, autonomous_stop), so the DTOs go in a sub-package. The series pays off only when ports and hub splits consume it. Re-run `scoping replay` and `criteria` on the spike and reject the series if it is purely negative.
- Prerequisites:
  - SENT-1 merged.
  - Spike gate for each stage: `go build ./... && go vet ./...` on a throwaway branch (kibitzer cannot model the existing aliases, so its cycle report is not a go/no-go signal). Record the result.
  - Check `go list -deps ./session/domain` has no module imports; the new package imports only stdlib and session/domain.
- Files moved:
  - From session/repository.go and session/storage_backlog.go: ReviewVerdictData, ReviewVerdictSummary, ProgressNoteData, ActivityNoteData, SourceSyncEventData, BacklogStatusEventData, OpenStuckStateData, StuckStateHistoryData, ItemSourceData, ItemSourceUpdate, BacklogItemDependencyEdge, BacklogItemDependencyRef, DeletedItemSessionCostEntry, DiagnoseResultData (verify its closure first).
  - Pure helpers (BuildAcChecklist, ParsePerCriterionVerdicts, ReviewVerdictSummariesMostRecentFirst, AcAuditEventData) move only if go/types shows they use only DTO types. Otherwise they stay and are listed as not moved.
  - The three storage_backlog.go structs that use session-local aliases (ItemSessionData, ReviewVerdictData, OpenStuckStateData) are rewritten to domain.X as they move.
  - Temporary bridge file session/backlog_dto_bridge.go: one `type X = backlogdto.X` per moved type. Sunset: removed by BL-E, enforced by the ratchet test below.
- Exports:
  - backlogdto.* for the types above
- Extension points:
  - None added. State plainly: DTO lowering opens no extension seam. Adding a DTO field is an edit to a stable-by-intent leaf data package, not an OCP seam.
- Tests to move:
  - Tests of pure DTO helpers move with them. Tests that use unexported items stay and are switched to exported names (backlog_remediation_test.go and diagnose_nudge_test.go are NOT affected here; they are tied to remediation and are untouched).
- Guard changes:
  - New depguard rule for session/domain/backlogdto: allow $gostd and session/domain only (ulid arrives in BL-B). Add a boundaries.txt line and a budget row.
  - Extend session/domain purity test to cover session/domain/*.
  - Add session/alias_sunset_test.go: the allowed alias names list can only shrink, and the test fails if a new `= backlogdto.` alias appears.
  - Update session/domain/backlogdto/BUILD.bazel and session/BUILD.bazel srcs.
- Verify:
  - go build ./... && go vet ./... && go test ./session/... ./server/...
  - go list -deps ./session/domain/backlogdto | grep stapler-squad (only session/domain and itself)
  - go run ./tools/bzl/scoping boundaries, budget, criteria, replay; record the SC4/SC6 deltas
  - Rollback: revert this PR. Later stages must be reverted first, newest first.
- Composition-root wiring: None.

### NF-1: server/services/backlogsvc -> server/services/backlogsvc (in place)

- Cost: S
- Payoff: Direct session/ent imports and no_ent_in_services exclusion lines drop in the backlogsvc files that no longer use ent. Transitive ent stays through session, so there is no cache or test-set effect; that is stated in the PR. It also centralizes the not-found class on domain.ErrNotFound.
- Prerequisites:
  - SENT-1 merged (uses domain.ErrNotFound).
  - Pin tests FIRST, in a separate earlier commit: for every storage method that feeds a guarded site (GetBacklogItem, DeleteBacklogItem, DeleteItemSource, UpdateItemSource for every sourceBackend impl, UpdateBacklogItem, GatedTransition, GatedArchive, GatedUnarchive, GatedSetPRAndTransition) and malformed ids, assert errors.Is(err, domain.ErrNotFound) and !ent.IsNotFound(err). Without a wrap on every path, do not drop the ent check.
- Files moved:
  - No move. Edit the 14 sites of the form `ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound)` in backlogsvc: lifecycle, query, triage, trigger_triage, ship, ship_status and sync (replace with errors.Is(err, domain.ErrNotFound)). Recount with `rg` and put the per-site table in the PR.
  - DO NOT touch the 4 bare sites (backlog_service_lifecycle.go :698, :1165, :1229, :1355). Today the wrapped error makes them dead branches that fall through to CodeInternal; changing them is a wire behaviour change. See the owner question.
- Extension points:
  - None.
- Tests to move:
  - Add the pin tests above (characterization per RPC path).
- Guard changes:
  - Delete no_ent_in_services exclusion lines only for backlogsvc files that no longer import session/ent. backlog_service_pipeline_mode.go still imports ent (*ent.PipelineMode), so its line stays. Probe the glob with a deliberately violating file. Lower the backlogsvc imports row in budget.txt if the count drops.
- Verify:
  - go build ./server/services/backlogsvc/... && go test ./server/services/backlogsvc ./session -run 'NotFound|Backlog'
  - make lint-custom (no_ent_in_services)
  - go list -f '{{join .Imports "\n"}}' ./server/services/backlogsvc | grep session/ent$ shows the remaining files (direct imports, not -deps)
  - Rollback: revert the PR.
- Composition-root wiring: None.

### WAKE-1: session -> session/domain

- Cost: M (33-file mechanical rewrite; the throwaway-worktree spike by a reviewer built clean)
- Payoff: Conditional on WAKE-2. By itself it only removes the DTO edge from server/wakes (WakeTriggerData: 12 files, 72 non-test refs). Ship it only if the WAKE-2 spike shows pkg/events can shed session; otherwise defer it, since a DTO move that leaves wakes importing session has no payoff.
- Prerequisites:
  - SENT-1 and DIALOG-1 merged.
  - WAKE-2 spike result is positive (see WAKE-2).
  - Rewrite the callers in the same PR; no alias.
- Files moved:
  - Declarations to session/domain/wake_store.go: WakeTriggerData, CreateWakeTriggerInput, DefaultWakePendingPerOwnerCap, ErrWakeCapExceeded, ErrWakeTransitionRejected. Fields use only stdlib and domain types; no methods.
  - Rewrite 33 files: server/wakes (13 non-test files), server/wakes_source_adapters.go, server/mcp/wake_tool_deps.go, server/services/wake_trigger_service.go, session adapters ent_repository_wake*.go, storage_wake.go, ent_repository_operator_wake.go, storage_operator_wake.go, and their tests.
- Exports:
  - domain.WakeTriggerData
  - domain.CreateWakeTriggerInput
  - domain.ErrWakeCapExceeded
  - domain.ErrWakeTransitionRejected
  - domain.DefaultWakePendingPerOwnerCap
- Ports:
  - wakes.Store and services.WakeTriggerStore already exist consumer-side; unchanged. No new interface.
- Extension points:
  - wakes already has Store/Steerer/Admission. No change; this is a dependency lowering step, not an OCP step.
- Tests to move:
  - No test moves. fakes_test.go and the integration tests in server/wakes switch to domain types and keep session for the repo (see WAKE-2).
- Guard changes:
  - BUILD.bazel srcs and deps for session/domain and touched packages. Budget lines lowered as measured.
- Verify:
  - go build ./... && go vet ./server/... ./session/... && go test ./server/wakes/... ./server/mcp/... ./server/services/... -race
  - grep proves zero `session.WakeTriggerData|CreateWakeTriggerInput` references remain
  - Rollback: revert the single PR.
- Composition-root wiring: None. server/wakes_module.go keeps passing *session.EntRepository as the wakes.Store implementation.

## Wave 4

BL-B moves the core backlog DTOs (it needs BL-A because ItemSessionSummary and BacklogItemData embed the group A types). WAKE-2 completes the wakes library leaf and is independent of the backlog files.

### BL-B: session -> session/domain/backlogdto

- Cost: L (bridge PR; rewrites follow in waves 5 and 6)
- Payoff: Milestone only. The core DTOs get a lower home; session-hub aliases remain until BL-E. No consumer is expected to drop its session import yet (backlogsvc still uses Storage with 60 refs, SessionRole*, TriggeredBy*); the plan does not claim that. Measure with `go list -f` per package before and after.
- Prerequisites:
  - BL-A merged.
  - Spike build recorded.
  - Confirm no open PR or live worktree edits repository.go or storage_backlog.go (174 worktrees exist). Land fast.
- Files moved:
  - session/backlog_item_id.go: BacklogItemID, ParseBacklogItemID, String/IsValid, the ulid usage, and the (*BacklogItemData).PublicID method moves together with BacklogItemData (the only method on any moved type).
  - BacklogItemData, BacklogItemSummary, BacklogItemUpdate, BacklogItemFilter, BacklogItemPrecondition, ItemSessionSummary, ItemSessionData, ItemSessionBacklogEntry, PRReassignmentGuard
  - Backlog sentinels: ErrDependencyCycle, ErrPRReassignmentNotAllowed, ErrPRDetached, ErrAcCriterionWaived, ErrAcCriterionOutOfRange, ErrWaiverReasonRequired, ErrAcCriterionAlreadyDone, ErrAcCriterionAlreadyWaived, ErrAcWaiverViaUpdate (the bridge keeps `var ErrX = backlogdto.ErrX` so errors.Is identity is preserved)
  - Do NOT move ErrTransitionNotAllowed (already `= domain.ErrTransitionNotAllowed`; its re-export var is deleted by BL-F) or any Storage method or process global such as ServerStartTime.
  - Extend session/backlog_dto_bridge.go with the new aliases and vars.
- Exports:
  - backlogdto.BacklogItemID and ParseBacklogItemID
  - backlogdto.BacklogItemData and its siblings
  - backlogdto.Err* backlog sentinels
- Extension points:
  - None added (state plainly).
- Tests to move:
  - backlog_item_id_test.go moves with the code.
- Guard changes:
  - backlogdto depguard allow-list adds github.com/oklog/ulid/v2. Update session/domain/backlogdto/BUILD.bazel deps (ulid).
  - Alias-sunset ratchet test: update the allowed list (it may only shrink after this PR).
  - Budget row for backlogdto; lower session caps as measured.
- Verify:
  - go build ./... && go vet ./... && go test ./session/... ./server/...
  - go list -deps ./session/domain/backlogdto | grep stapler-squad (only session/domain and itself)
  - scoping boundaries, budget, criteria, replay recorded; kibitzer is not trusted for cycles
  - Rollback: revert this PR (revert BL-C and BL-D first if they have landed).
- Composition-root wiring: None.

### WAKE-2: pkg/events -> pkg/events (ent-free and session-free payloads); lock-in guard in server/wakes

- Cost: L, uncertain. A spike decides it; split into PRs per payload family if the spike is large.
- Payoff: The only stage in this roadmap with a directly measurable leaf result. `go list -deps ./server/wakes | grep -c 'stapler-squad/session$'` must go from 1 to 0 for the non-test library (today the closure is 131 packages against 129 for session, so wakes is not a leaf by closure). The root cause is pkg/events/types.go importing session, session/ent, session/detection and session/sshremote, and wakes imports pkg/events (kinds.go, escalator.go, metrics_session.go). The wakes tests keep session (11 files use EntRepository), so the go_test target still depends on session. Only the library gets independent.
- Prerequisites:
  - SPIKE (a recorded gate, not a PR): go/types inventory of every session.* symbol that pkg/events non-test code uses, and `go list -deps` for detection and sshremote to confirm neither imports the root session package.
  - If the inventory is more than DTO-sized symbols (for example session.Instance or Status in payloads), stop: WAKE-1 and this stage are deferred and the wakes leaf claim is dropped. See the owner question.
  - WAKE-1 merged.
- Files moved:
  - pkg/events/types.go payload fields changed from session-typed or ent-typed to lowered domain/DTO types. Known: WorkflowEventPayload.Workflow *ent.Workflow (types.go:143) becomes a domain view; 7 constructors (webhook_lifecycle_service.go:157,160; webhook_registration_service.go:250; workflow_service.go:329,488,521,592) and the subscriber entWorkflowToProto (workflow_service_events.go:192,198), server/events/forward.go, and the two webhook handler tests must change in the same PR. Golden-payload test: serialized bytes unchanged.
  - server/wakes/deps_test.go:28 (`require.Contains(deps, ...session)`) is inverted into a forbidden-import assertion.
- Exports:
  - domain-level event payload view types (decided by the spike)
- Ports:
  - No new interface. Existing wakes.Store stays, with the ent adapter (*session.EntRepository) and the fakes already used in tests.
- Extension points:
  - Events: new payload = a new type in pkg/events. Wake kinds (describe.go and kinds.go) remain switch-based; this stage does NOT claim to fix that. Making them table-driven is a separate OCP candidate, not scheduled.
- Tests to move:
  - Rewrite the test sites that assert on payload.Workflow (about 14 references) and keep an ent-backed contract test for the transition-race semantics. Do not replace wakes tests with a fake Store in this stage.
- Guard changes:
  - Add depguard wakes_no_session on server/wakes non-test files only (tests excluded explicitly), a boundaries.txt line, and a budget row for server/wakes.
  - Probe the glob by adding a deliberately violating file.
- Verify:
  - go list -deps ./server/wakes | grep -c 'stapler-squad/session$' must print 0 (non-test); record the closure size
  - go build ./... && go vet ./server/... ./pkg/... && go test ./server/wakes/... ./server/services/... ./server/events/... ./pkg/events/... -race
  - scoping replay before and after, counting the tests that no longer re-run on a session-only change
  - Rollback: revert the PR(s); the golden-payload test guards the bytes.
- Composition-root wiring: server/wakes_module.go keeps constructing wakes with the concrete session repository. Only the wakes library import graph changes.

## Wave 5

Rewrite the external consumers of the backlog DTOs, three chunk PRs by directory (independent files, the 3-open-PR cap). Each chunk is a mechanical codemod (command in the PR description) over session.X to backlogdto.X and leaves main green because the bridge still exists. This is the strangler that lets the DTO series ship without one 245-file PR.

### BL-C: session (consumers outside session) -> session/domain/backlogdto (consumers retargeted)

- Cost: L total (3 chunks, each M)
- Payoff: Per-chunk measured direct-import deltas (`go list -f '{{.Imports}}'`). Honest expectation: no package drops its `session` import in this wave. The edges to backlogdto are added and the session-DTO references go to zero outside session. This is the precondition for BL-E, so the lasting alias can be deleted.
- Prerequisites:
  - BL-B merged.
  - Check open PRs and worktrees touching these packages right before each chunk; land each chunk quickly.
- Files moved:
  - Chunk C1: server/services/backlogsvc (12 non-test files use BacklogItemData) and server/services/internal/svctest (2).
  - Chunk C2: session/backlog (14), server/mcp (5), server (1), pkg/events (1), reviewqueue, prpoller, wakes, any other non-test referrer.
  - Chunk C3: server/services (16 non-test files). Tests in each package are rewritten with their chunk.
  - Symbols per chunk: BacklogItemData (52 non-test files outside session), ItemSessionSummary (37), ErrNotFound already done, ItemSessionData (9), BacklogItemUpdate (10), BacklogItemFilter (13), BacklogItemPrecondition (9) plus the group A DTOs. Recount with go/types before each chunk.
- Extension points:
  - None added.
- Tests to move:
  - Rewrite in place in each chunk.
- Guard changes:
  - Alias-sunset ratchet test: record which aliases have zero external users after each chunk.
  - Lower the per-package imports rows in budget.txt only if measured lower.
- Verify:
  - Per chunk: go build ./... && go vet ./... && go test <chunk packages> -race
  - rg 'session\.(BacklogItemData|ItemSessionSummary|ItemSessionData|BacklogItemUpdate|BacklogItemFilter|BacklogItemPrecondition)' in the chunk's packages returns nothing
  - Rollback: revert the chunk (the bridge keeps main compiling).
- Composition-root wiring: None.

## Wave 6

Rewrite the bare in-session references (about 98 non-test and 161 total session files). It collides with parallel backlog work, so schedule it in a quiet window and split by file glob into at most 3 PRs. It is independent of wave 5 in code, but the PR cap serializes them.

### BL-D: session -> session/domain/backlogdto (in-session references retargeted)

- Cost: L (3 chunks; the S6(e) rename flagged by the reviewers as the riskiest)
- Payoff: Prerequisite for deleting the bridge. No standalone payoff.
- Prerequisites:
  - BL-B merged (BL-C is not required, but both must finish before BL-E).
  - Quiet window: check `gh pr list --search 'session/storage_backlog.go in:files'` and `git worktree list`.
- Files moved:
  - D1: storage*.go, ent_repository_backlog*.go, storage_backlog*.go
  - D2: backlog_*.go, ac_audit.go, diagnose_result.go, transition_gate.go, backlog_remediation.go
  - D3: all remaining session/*.go and their tests
  - Use gopls rename or `gofmt -r` limited to package session, then goimports. Command committed in each PR.
- Extension points:
  - None.
- Tests to move:
  - Rewritten in place.
- Guard changes:
  - Alias-sunset ratchet: the bridge should have zero in-package users at the end.
- Verify:
  - Per chunk: go build ./... && go vet ./session/... && go test ./session/... -race (use -p 1 per the Makefile for contention-prone packages)
  - rg for bare `\bBacklogItemData\b` without a package qualifier in session returns nothing at the end
  - Rollback: revert the chunk.
- Composition-root wiring: None.

## Wave 7

Close the series. BL-E deletes the temporary bridge so no new lasting alias remains. BL-F retires the pre-existing domain re-exports in session/backlog.go (about 42-50 `= domain.X` lines used about 2572-3450 times); the owner must confirm it (question 2).

### BL-E: session -> session/domain/backlogdto

- Cost: S
- Payoff: The series is closed with zero new aliases. Record the final SC4/SC6 `scoping replay` numbers and the per-consumer direct-import table in the PR. If SC6 is negative, record the series in scoping-ledger.md as a prerequisite-only investment and gate further DTO families (stage, pipeline-mode, gate) on a consumer that actually sheds `session`.
- Prerequisites:
  - BL-C and BL-D merged.
- Files moved:
  - Delete session/backlog_dto_bridge.go and session/alias_sunset_test.go. Replace them with a test or lint that fails on `type X = backlogdto.` in package session.
- Extension points:
  - None.
- Guard changes:
  - Lower the session caps in budget.txt to the measured values.
  - Add the depguard rule: non-session packages may import backlogdto, not session, for DTO-only needs (informational ratchet).
- Verify:
  - go build ./... && go vet ./... && go test ./session/... ./server/...
  - rg 'type \w+ = backlogdto\.' returns nothing
  - Rollback: revert (the bridge returns).
- Composition-root wiring: None.

### BL-F: session -> session/domain (existing aliases retired)

- Cost: L (mechanical but very wide: 2572-3450 sites)
- Payoff: Removes the existing lasting hub aliases in session/backlog.go (BacklogStatus, StuckReason, AcStatus, AcCriteriaJSON, AcCriterion, ReviewOutcome, CriterionVerdict, BacklogCategory, BacklogItemTransitionInput, TransitionGuard) and the Err* re-export vars (ErrACRequired, ErrPlanRequired, ErrVerdictRequired, ErrCodeNotOnMain, ErrUnresolvedBlockers, ErrTransitionNotAllowed). Hygiene and rule compliance, with no cache payoff on its own. Gate on the owner's decision.
- Prerequisites:
  - Owner confirms question 2.
  - BL-E merged, so the churn is not stacked.
- Files moved:
  - Delete the alias lines in session/backlog.go (recount; the reviewers disagreed, 42 against 50) and the 10 other session files with `= domain.` lines that are in scope.
  - Rewrite consumers: external first (one PR), then in-session (one PR).
- Extension points:
  - None.
- Tests to move:
  - Rewritten in place.
- Guard changes:
  - A lint or test: no `= domain.` alias in non-test session files.
- Verify:
  - go build ./... && go vet ./... && go test ./session/... ./server/...
  - rg '^(type|var) \w+ = domain\.' session/*.go returns nothing
  - Rollback: revert the PR.
- Composition-root wiring: None.

## Deferred or rejected

- **BL-3-backlog-reconcile-read-port**: REJECTED (dependency-direction lens: reject/major). ReconcileQueries (6 Find* methods) cannot replace the *session.EntRepository parameter: reconcilers call about 20-24 distinct er.* methods (FindOpenStuckStates, MarkStuck, MarkStuckNotified, RecordRemediationAttempt, ItemPRsByID and others), and er.Client 13 times. Three of the Find* queries return eager-loaded ent edges (Edges.ItemSessions, Edges.ReviewVerdict) that BacklogItemData does not carry, so the change is a data-model change, not a move. It removes no import edge, so the Bazel payoff is zero. Only the stuck-state slice survives, as STUCK-1.
- **BL-2-itemsource-plugin-leaves (physical packages)**: Deferred. Only the registry inversion is scheduled (PLUG-1). Physical moves into session/itemsource/{githubissues,githubprs,linear,jira} are blocked on BacklogItemData being in backlogdto, and the ledger's SC6 skip (lib 0.00, test -0.024 pp) is unchanged. The leaf claim was wrong anyway: server/services and server/mcp import the concrete plugins. Only jira and linear imports leave session (github stays through 10 other files). Revisit after BL-E with `scoping criteria`, and as one itemsource package, not four.
- **DTO-1-sessiondata (InstanceData, Status, PR vocabulary)**: Deferred. Preconditions are not met: the PR types import github (NormalizeHost, which pulls session/git, tmux, config, executor), PRRef.RepoRef is a method in a staying file (9 call sites), ProcessManagerBackend needs its method and unexported map, config.SessionType weight is undecided, and 724+128 Status references make a mass rewrite. Kibitzer showed InstanceData introduces a cycle through WorkspaceKey. Payoff is unproven: of five named consumers only session/workspace could drop the session import. Gate: NormalizeHost moved to a stdlib-only leaf; then a first stage of Status+ProcessManagerBackend measured by replay for session/workspace.
- **DTO-2-sessionstore**: REJECTED (payoff lens: reject/major). The claim of zero hub references is false: Delete calls stampItemSessionConversationUUID, deleteOwnerPRRows and SessionPROwner; the mapper needs ProcessManagerBackend, SessionType and listLoadOptions; embedding a Store breaks the three tests that reassign repo.Client and the literal constructors (ent_repository.go:115 and :302). About 1.7% of session LOC, below the SC6 threshold, and no test binary shrinks.
- **DTO-3-prstore**: REJECTED (payoff lens: reject/major). A mechanical move leaves 94 session imports in the moved files (30 EntRepository receivers) and creates a cycle; the guard file mixes item-session queries; the 30 methods are on *EntRepository so a Store rewrite is required; about 1.5% of LOC with 4 commits/180d. Only the PR DTO lowering inside DTO-1 is worth reconsidering.
- **DTO-S3-guidance**: REJECTED (payoff lens: reject/major). CreateGuidanceRequestInput has an unexported mirror field and source() method; ErrPendingCapExceeded is not in the move set; 22 files plus 17 test files change for no dropped session edge; the 8-method guidanceStore port has no second implementation. Revisit only when a consumer group (ack, reaper, mirror) drops session entirely.
- **DTO-S4-webhook-credential**: REJECTED (open-closed and payoff lenses: reject). Three consumer files, 1-3 commits each; WebhookRegistrationStore has no fake (tests deliberately use the real repo); stage (a) is really an ent-free WorkflowEventPayload change across 7 publishers. The WorkflowEventPayload piece is folded into WAKE-2 if the spike supports it.
- **DTO-3-ent-containment**: REJECTED (payoff lens: reject/major). The headline check is unsound: `go list -deps ./server/services | grep session/ent$` stays at 1 after every stage because session imports ent. There are at least 31 non-test importers, not 22. The ports return *ent.Workflow and *ent.PipelineMode; StageConfigIntrospector, WorkflowSessionPruner and ErrorEventSink have no second implementation; and 65 test files call GetEntClient(). It is hygiene with no cache benefit.
- **DTO-4-backlog-consumer-ports**: REJECTED (payoff lens: reject/major). The StuckStore claim of 2 implementations is false for 3 of 8 methods; the port leaves 91 *EntRepository uses; no import edge is removed; ItemStore and ItemSessionStore (backlogsvc, mcp) have one implementation and tests already use real sqlite. The narrow StuckStore slice survives as STUCK-1.
- **DTO-5-gate-guidance-pr-dtos**: REJECTED (payoff lens: reject/major). Moving PRRef into session/domain makes the lowest package import github. The 'redeclared' repository ports return *ent.Workflow, *ent.PipelineMode and []*ent.TriggerFireEvent, so consumers still import ent. TransitionRequest needs CallerClass/EngineMode lowering. pr_set.go has 1 commit/180d (INST-prset was already rejected for low payoff).
- **ENT-1 (standalone)**: REJECTED as a standalone stage (payoff reject/minor). The 14 no-op sites are folded into NF-1; the 4 bare sites are a behaviour change held for an owner decision.
- **ENT-2-row-dto-lowering**: Deferred. Only the pkg/events Workflow payload piece is retained, inside WAKE-2 and conditional on the spike. The rest has a wide file list (34 test files and 24 non-test files use the ent rows), the DTO needs stage slugs and gates (Edges.FromStage/ToStage/Gates), and no consumer drops session. Re-propose per family only with a leaf that benefits.
- **ENT-3-client-handle-removal**: REJECTED (payoff lens: reject/major). Leak inventory incomplete (33 GetEntClient sites incl. workflow_service.go:708/759, workspace_peers.go, svctest), 65 test files depend on the accessor, port definitions do not match the real signatures, and ErrorEventSink and WorkflowSessionPruner have no real second implementation.
- **ENT-4-backlog-repo-ports**: Mostly deferred. Only StuckStateStore is retained, as STUCK-1. WebhookReconciler is dropped (no fake exists; the dedup tests deliberately use real sqlite). ReviewQueries and PRQueries return []*ent.BacklogItem and wait for a DTO that carries sessions and verdict edges.
- **ENT-5-entstore-adapter-leaf**: Deferred. Depends on ENT-1..4, most of which are rejected; the 209-method StorageBackend would be an interface-pollution mega-port with one implementation; the leaf claim is false (wakes, workflows, mcp, webhook_management, tests/demo, e2e seed import the concrete repo); only about 15% of adapter commits are adapter-only; and session/migrate.go and testing.go construct the adapter. Revisit only after consumers are behind narrow ports with real seams.
- **session/backlogstore, backlog_sync leaf, backlog_remediation package, backlog_crypto/intent**: Rejected clusters carried over from the proposals: about 25 real core back-edges (writeTx, invalidate, publishItemChanged, rawTransition*), the sync loop and remediation depend on the workflow engine and transition gate that stay in session, and the files are below any measurable payoff (SC6 skips stand).
- **Guidance/handoff/rules/projects/escape-events/notifications small-store packages**: Not session-resident or SC1/SC6-skipped in the ledger (wake/guidance helpers: 29 core uses, lib 0.00 pp; webhooks: 24 core uses). No new package proposed.
- **wake kinds table-driven (describe.go/kinds.go switch)**: Known OCP violation (kind switches), but out of scope here and unmeasured. Candidate follow-up after WAKE-2.
- **4 bare ent.IsNotFound sites in backlog_service_lifecycle.go**: Held back from NF-1: they change an RPC result from Internal to NotFound (a wire behaviour change). Needs the owner decision (question 1).

## Owner questions

- Accept the Internal to NotFound behaviour fix at the four bare ent.IsNotFound sites (backlog_service_lifecycle.go :698, :1165, :1229, :1355) as a separate, explicitly labelled PR with RPC-level tests? NF-1 leaves them untouched.
- The pre-existing `= domain.X` re-exports in session/backlog.go (about 42-50 lines, 2572-3450 uses): retire them (BL-F, very wide mechanical rewrite) or grant an explicit exemption from the no-lasting-alias rule? The new bridge in the DTO series is deleted either way (BL-E).
- MIG-1 leaves session importing migrations.Default (a core-to-detail edge). Adding a migration no longer edits session. Is that enough for OCP, or do you want NewEntRepository to require the list from the composition root, which touches the 86 call sites?
- Is a separate session/domain/backlogdto sub-package acceptable, to keep the high-churn DTOs (73 and 60 commits/180d) away from the stable session/domain consumers? Or should everything live in session/domain?
- If the WAKE-2 spike shows pkg/events needs session.Instance/Status-level types, do you abandon the wakes library-leaf goal and defer WAKE-1, or fund an events lowering project first?
- The backlog DTO series (BL-A to BL-F, about 8 PRs across several sessions) has no standalone Bazel payoff, as every lens agreed. Do you accept it as a prerequisite investment with replay checkpoints at BL-B and BL-E, with SC6 recorded and the later DTO families (stage, pipeline-mode, gate) gated on a consumer that sheds `session`?
- Quiet windows: the wide codemods (SENT-1, BL-D, BL-F) collide with 174 worktrees and the backlog WIP cap of 2. Is a coordinated freeze on repository.go and storage_backlog.go acceptable for those PRs?
- Do you accept single-implementation ports only with a written in-memory fake and a shared contract suite in the same PR (STUCK-1 applies this rule)?