# Package split roadmap: open-closed leaf packages (2026-11-10)

Produced by the `package-split-design` workflow (159 agents: one analyst per package, four review lenses per proposal, one synthesizer) over `session`, `server/services`, `server/services/backlogsvc`, `session/backlog`, `server/mcp` and `server`. Builds on ADR-010 (`project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md`) and its `scoping-ledger.md`. Owner principle: end with leaf packages that follow the open-closed principle (a new variant is a new file or package plus one registration at the composition root, with no edit to the stable core).

## Assessment

Applied strictly, the accept rule passes very little. Almost every reviewed package-move proposal drew a payoff reject, and most drew a feasibility or dependency-direction blocker. The reviews repeatedly found the same structure. The real open-closed violations (fixed migration list, concrete plugin constructors, kind switches over the backend enum, the 26-call detector list, NewCore's tool list, knownFeatureFlags) can be fixed in place with a small consumer-defined seam and a composition-root list. The proposed physical splits mostly leave the core importing the new package, so the core is not stable and the new package is not a leaf. The owner's goal is OCP leaves rather than Bazel gain alone, so I used judgment beyond the literal rule: each item first fixes the OCP seam in place and only then proposes the physical leaf move. Only SRV-reviewqueue passed with no reject, and it is the one pure move with a verified build. FLAGS-1, DET-0 and HIST-slice are the narrowed forms of proposals that also drew no reject, and ITEMSRC-move and BL-shadowledger come after their seams land. SC4/SC6 are unmeasured for every moved package, so each physical move must pass scoping replay (or Q1 must be answered 'OCP is enough') before merge. Waves 1 and 2 contain no cross-package moves except reviewqueue and are safe to run in parallel. The deferred list holds the large splits (entstore, records, backlogsvc extractions, mcp leaves, server/startup). They stay deferred until the seams exist and a replay or owner decision justifies them."

## Wave 1

Cheapest items that either have no lens reject (reviewqueue) or establish the open-closed seam in place before any physical split. All three are independent and touch different packages. No new interface is added without a consumer-side need.

### SRV-reviewqueue: server -> server/reviewqueue

- Cost: S (single PR, one agent wave, no wall-clock blocker)
- Role: application-service; leaf imported only by server/dependencies.go and tests
- Payoff: Only server proposal with no lens reject. Verified in a throwaway worktree: go build and go vet pass. Edits to dependencies.go and server.go (136 and 140 commits in 180d) stop rebuilding and re-testing the 1014-LOC queue manager and its tests (16 commits in 180d). SC4/SC6 numbers are NOT measured; run scoping replay before merge.
- Files moved:
  - server/review_queue_manager.go
  - server/review_queue_manager_test.go
  - server/slack_gate_test.go
- Ports:
  - Keep the existing consumer-defined seams unchanged: OneShotPRCreator, CallbackDispatchWiring, SlackNotifierWiring. No new port. Drop the proposed ItemSink and the handleEvent-to-map rewrite (fails the 2+ implementations test; the map would still be edited per event).
- Exports:
  - ReactiveQueueManager
  - NewReactiveQueueManager
  - WatchReviewQueueFilters
  - OneShotPRCreator
  - CallbackDispatchWiring
  - SlackNotifierWiring
  - SlackNotifier() accessor (new, replaces the unexported slackNotifier read in dependencies_test.go:80-82)
- Extension points:
  - New queue side-effect today = new Set* wiring port implemented in a new file and injected in dependencies.go (unchanged behaviour, so only the existing OneShotPRCreator/CallbackDispatch/Slack seams)
  - A new notification channel stays closed to the manager only if it is added as a new injected port; the allowQueueItem(Channel) gate stays in the package
- Composition-root wiring: server/dependencies.go imports server/reviewqueue instead of defining it; construction and Set* calls unchanged; reviewQueueLookupAdapter stays in server/dependencies.go
- Tests to move:
  - review_queue_manager_test.go
  - slack_gate_test.go (needs newReactiveQueueTestSetup, setTestSlackConfig, fakeSlackNotifierWiring from the manager test)
- Guard changes:
  - Do NOT move review_queue_lookup_adapter_test.go (it tests an adapter in dependencies.go)
  - Fix dependencies_test.go:80-82 to use SlackNotifier()
  - Add a budget.txt row only if scoping criteria says the package qualifies; check slack_interactive_handler_integration_test.go (comment-only reference)
  - Check git worktree list for branches editing server/review_queue*
- Verify:
  - go list -deps ./server/reviewqueue | grep stapler-squad/server$  (must print nothing)
  - go build ./... && go vet ./server/... ./server/reviewqueue/...
  - go test ./server/reviewqueue/ ./server/ -run 'Queue|Slack|Dependencies'
  - go run ./tools/bzl/scoping replay and criteria on the file set
  - Gazelle diff clean; make lint-custom

### HIST-slice: session -> session (in place, step 1 of 2; physical session/historyadapters move deferred)

- Cost: S-M (single PR; characterization test first)
- Role: application-service
- Payoff: Removes a real OCP violation: resolveHistoryAdapter is a 5-way if-chain (claude, agy, opencode, gemini, pi) with 7 call sites in 3 files, plus dst.Name()==agy/claude/opencode branches in history_transfer.go (lines 58, 89, 140). No Bazel payoff claimed (ledger SC6 would fail).
- Ports:
  - HistoryAdapter (already exists in session/history_adapter.go, consumer-defined, 5 implementations): add one optional hook interface PostExporter {AfterExport(...)} so per-target post-export logic lives in the adapter, not in history_transfer.go
  - Do NOT add HistorySession until adapters physically move
- Extension points:
  - New agent = new <agent>_adapter.go implementing HistoryAdapter (+ optional PostExporter), one entry in the ordered slice built by the composition root
  - Ordering is explicit data (ordered slice with a documented priority), pinned by a table test over program strings (claude, agy, opencode, gemini, pi, aider, empty, antigravity-claude)
  - Stays closed: history_transfer.go, instance_program.go, instance_checkpoint.go (no agent-name branches)
- Composition-root wiring: server/dependencies.go (or session.DefaultHistoryAdapters() called there) builds the ordered []HistoryAdapter and injects it into Instance/HistoryPorter; replace the process-global SetHistoryAdapters; an empty registry fails loudly, not as a silent nil
- Guard changes:
  - Add a characterization table test BEFORE the refactor (pins CanHandle order byte for byte)
  - Test helper in session builds the default slice so bare-Instance tests keep working
- Verify:
  - go test ./session -run 'HistoryAdapter|history_transfer|InstanceProgram|Checkpoint'
  - go test -race ./session
  - grep: no dst.Name()=="agy"/"claude"/"opencode" remains in session/history_transfer.go

### DET-0: session/backlog -> session/backlog (in place, 'wave 0' only; no detector subpackages)

- Cost: S-M (one PR; 26 call sites)
- Role: application-service
- Payoff: Removes the main OCP violation in the hub: the 26 hard-coded runStuckDetector calls in ReconcileStuck (backlog_lifecycle.go:1419-1639). Physical per-family packages are NOT committed (43 hard split_sim blockers; SC4/SC6 expected to fail).
- Ports:
  - Detector {Name() string; Run(ctx, er) } defined in session/backlog (the consumer, 26 implementations). Env is NOT introduced yet: detectors stay methods/closures over the hub in this step
- Extension points:
  - New stuck condition = new detector file returning a Detector, registered in the ordered slice
  - Explicit ordering field/phase so invariants are tested: work_commit_refresh before pr_ready+merge_detection (BUG-047), transition_denied_redrive first
  - Stays closed: the panic-isolating runner and ReconcileStuck loop
- Composition-root wiring: Hub constructor owns a default ordered []Detector; server/dependencies.go may append extra detectors through one option (WithDetectors); remove the corresponding Set* setters only as each detector gets its own constructor-injected ports
- Guard changes:
  - Add an ordering test that fails if the two first-position invariants move
  - wake_polling_guard-style allow-lists: none expected; check make actor-field-guard and noarchivedrevival still pass on moved call sites
- Verify:
  - go test ./session/backlog -run 'Lifecycle|Stuck|Reconcile'
  - go test -race ./session/backlog
  - grep -c runStuckDetector session/backlog/backlog_lifecycle.go shows only the runner definition and the slice loop
  - make lint-custom

## Wave 2

Remaining in-place OCP fixes on the highest-churn registries. Each removes a named violation (fixed migration list, concrete plugin constructors, kind switches over the backend enum) and is the prerequisite that makes any later physical leaf move non-circular. Independent of each other.

### MIG-option: session -> session (in place; session/startupmig package NOT created)

- Cost: S (one PR)
- Role: application-service
- Payoff: Fixes the OCP violation: NewEntRepository loops a fixed startupMigrations list. The Host port from the proposal is dropped (cannot compile: EntRepository has an exported Client field; and dropObsoleteIndexes needs the unexported driver).
- Ports:
  - Existing Migration interface in ent_repository_migrations.go (already 8 implementations) stays the only abstraction; no Host port
- Exports:
  - WithMigrations(...Migration) RepositoryOption
  - DefaultStartupMigrations() []Migration (ordered; golden test pins the eight names in order)
- Extension points:
  - New startup migration = one new file declaring a Migration + one line in DefaultStartupMigrations or one WithMigrations argument at the composition root
  - Stays closed: NewEntRepository body; the workflow_enabled pre-Schema.Create exception stays in the constructor with a comment (no second pre-schema migration exists)
- Composition-root wiring: server/dependencies.go passes WithMigrations(session.DefaultStartupMigrations()...) (or relies on the default for tests); remove the package-level var startupMigrations
- Guard changes:
  - Add golden Names() order test
  - 86 NewEntRepository call sites unchanged because the default is applied when the option is absent
- Verify:
  - go test ./session -run 'Migration|NewEntRepository'
  - go build ./... && go vet ./session/...
  - go test ./server/... -run Dependencies

### ITEMSRC-registry: session + server/mcp + server/services -> session (in place); consumer-side ports in server/mcp and server/services

- Cost: S-M (one PR)
- Role: adapter wiring
- Payoff: Removes the concrete-plugin coupling that would stop session/itemsources from being a leaf: NewDefaultRegistry hard-codes four Register calls (single caller server/dependencies.go:1608); server/mcp/tools_backlog_import_tracker.go:80,115 builds NewLinearPlugin/NewJiraPlugin directly; services forward/progress sync builds &GitHubIssuesPlugin{}.
- Ports:
  - IssueFetcher {FetchIssue(ctx, PluginConfig, id)} defined in server/mcp (consumer), resolved from the PluginRegistry by tracker ID; implemented by the Linear/Jira/GitHub plugins (3 implementations)
  - Reuse existing server/services ports (issueProgressCommenter, externalIssueStateUpdater) and resolve through registry capability probes, not &GitHubIssuesPlugin{}
- Exports:
  - session.NewPluginRegistry(plugins ...ItemSourcePlugin)
- Extension points:
  - New tracker = new plugin file implementing ItemSourcePlugin (+ optional PaginatedFetcher/IssueFetcher/externalIssueStateUpdater capabilities), one entry in the plugin list at the composition root
  - Stays closed: PluginRegistry, SyncLoop, server/mcp import tool, forward/progress sync subscribers
- Composition-root wiring: server/dependencies.go builds the []ItemSourcePlugin list and passes it to the registry; delete NewDefaultRegistry from session; mcp and services get the registry injected
- Tests to move:
  - TestNewDefaultRegistry_should_IncludeLinearAndJira moves to the composition-root test
- Guard changes:
  - Keep mapExternalToBacklogItem and maxTrackerResponseBytes unexported (do not widen session API)
  - Add a grep/ratchet test: no non-root package calls NewLinearPlugin/NewJiraPlugin/NewGitHubIssuesPlugin directly
- Verify:
  - go test ./session -run Plugin ./server/mcp ./server/services -run 'ImportTracker|ForwardSync|ProgressSync'
  - go build ./... && go vet ./server/... ./session/...
  - go list -f '{{.Imports}}' ./server/mcp | grep -v plugin constructors (no direct concrete constructor use: rg 'NewLinearPlugin|NewJiraPlugin' server/mcp returns nothing)

### BACKEND-registry: session -> session (in place, step 1; session/procbackend NOT created)

- Cost: S-M (two small commits in one PR)
- Role: adapter
- Payoff: Fixes the kind-enum switches that make a fourth transport edit ~5 core sites: NewProcessManager switch, processManagerBackendLabel type switch (backend_observability.go), pinResolvedBackend switch (backend_resolution.go:56-63), validateBackendOverride, backend_info_metric observes. No package split (SC1/SC6 fail; 36 in-package tests, bare &Instance{} pm() default makes a leaf impossible without init()).
- Ports:
  - Optional capability method on ProcessManager returning its existing ProcessManagerBackend key (replaces Label() factory and the type switch); backends already implement ProcessManager (3 implementations)
- Exports:
  - BackendFactory registry map[ProcessManagerBackend]func(ProcessManagerOptions)(ProcessManager,error) with Known(key) and optional flag gate
- Extension points:
  - New transport = new <x>_backend.go implementing ProcessManager (+ key method) and one registry entry
  - Closed: pm() tmux default stays in core behind the registry, so bare &Instance{} tests keep a non-nil manager; no init() self-registration
- Composition-root wiring: Default registry built in session (tmux, native, tymux); server/dependencies.go may pass additional factories via ProcessManagerOptions; unregistered key returns ErrUnrecognizedBackend
- Guard changes:
  - Update backend_assertion_guard_test.go for the removed type switch
  - Keep backend_resolution and backend_factory tests green unchanged
- Verify:
  - go test ./session -run 'Backend|ProcessManager|Resolution'
  - GOOS=windows go vet ./session/... ; GOOS=darwin go build ./session/...
  - rg 'processManagerBackendLabel' session returns nothing

## Wave 3

Registry and module-slice seams in the stable cores that every new variant currently edits (feature flags, MCP tool families, composition-root route/step lists). Each is partly gated on an owner decision or on wave 1-2.

### FLAGS-1: server/services -> server/services/featureflags (stage 1 only: Spec, Default lookup, FeatureController, FlagAuditPolicy, flag-name consts)

- Cost: M (one PR; service, mutation and audit files stay in server/services)
- Role: domain (stable core, small, abstract)
- Payoff: Removes the verified OCP violation: knownFeatureFlags is a 72-entry central slice scanned linearly by FeatureFlagDefault; every new flag edits the core (51 commits). Package imports config only. Bazel payoff unmeasured; run scoping replay.
- Files moved:
  - Spec/default table pieces and FlagAuditPolicy from feature_flag_service.go
  - FeatureController interface (declared in session_service.go:82) with a type alias left in services so the 22 uses compile
  - flag-name consts
- Ports:
  - FeatureController (existing, many implementations, moved not new)
  - No Register() global: an immutable Registry built from []Spec at the composition root
- Exports:
  - Spec (incl. Scopes)
  - Registry
  - NewRegistry([]Spec)
  - Registry.Default(name)
  - FeatureController
  - FlagAuditPolicy
- Extension points:
  - New flag = the owning leaf declares its Spec beside its flag-name const; composition root adds one entry to the []Spec slice
  - Stays closed: FeatureFlagService, its RPC handler, FeatureFlagDefault callers (receiver-less callers get the registry through a package-level accessor set once at startup, documented), the audit sink
- Composition-root wiring: server/dependencies.go assembles []Spec from core specs plus each leaf's Flags(); notification-actions and multi-PR audit policies keep injecting through MergeAuditPolicies
- Tests to move:
  - Pure Spec/Default tests only; config_surface_proof_* tests stay in services and the 8 tests ranging over knownFeatureFlags are rewritten against the registry; add a test that fails if any known flag is unregistered
- Guard changes:
  - Leave workflow-engine mode sources and WorkspacePeersBlockFor in services (not flag code)
  - depguard: featureflags may import config only
  - Update svc path keys only if sink_guard-style maps reference moved symbols
- Prerequisites:
  - Owner decision Q3 (is a global accessor acceptable for receiver-less FeatureFlagDefault callers)
- Verify:
  - go list -deps ./server/services/featureflags | grep stapler-squad  (config and itself only)
  - go test ./server/services -run 'FeatureFlag|ConfigSurface' ./server/...
  - go build ./... && go vet ./...
  - scoping replay on the file set

### MCP-modules: server/mcp -> server/mcp (in place; no leaf subpackages yet)

- Cost: M (one PR)
- Role: adapter
- Payoff: Fixes NewCore's hard-coded registerXTools list gated by if-chains (svc, storage, backlogEnabled, prCache); every tool family currently edits server.go. ADR-010 rejected splitting server/mcp for Bazel (SC2/SC4), so this is justified by OCP alone and claims no cache payoff.
- Ports:
  - ToolModule {Register(*mcpserver.MCPServer)} defined in server/mcp (consumer) with a CoreOption WithToolModules; implementations: wake, peer-help, owner-action, terminal, lifecycle, vcs, github, workflow, rules, notifications, history, flow, backlog, guidance, stuck-report (10+)
- Exports:
  - WithToolModules(...ToolModule)
- Extension points:
  - New tool family = new file with a module constructor returning a ToolModule, one line at the composition root; nil-svc gating is inside the module constructor (never returns a typed-nil interface)
  - Closed: NewCore, callerIdentityGate map and withCallerIdentityGate (allowed roles passed at registration instead of a central identityGatedTools map for new tools)
  - Registration order is explicit; shared *backlogHandlers across backlog, stuck-report and guidance modules is built once and passed to all three
- Composition-root wiring: server/mcp NewCore iterates the module slice; server/dependencies.go (and wakes_module.go, backlog_flow_wiring.go) append modules
- Guard changes:
  - Update caller_identity_gate_test key-set assertion and server_test.go source scans only if a file path changes (none expected; nothing physically moves)
- Verify:
  - go test ./server/mcp/... ./server/...
  - go list -deps ./server/mcp unchanged importer set (main.go, server, inttest)
  - go vet ./server/...

### SRV-wiring: server -> server (in place: same-package functions and tables; no server/startup, server/actuator, server/httpx, wakes/wiring or prwatch/wiring packages)

- Cost: M-L (2-3 PRs; sequenced behind backend-di-refactor Story 1.3)
- Role: composition root
- Payoff: Reduces the two hub files' OCP violations without creating cycles: registerActuatorRoutes becomes an endpoint table; BuildRuntimeDeps (~1470 LOC, ~86 locals) is split into same-file-set functions with typed inputs/outputs; installWakeTools/buildAndStartWakes list shrink. Root server is already a leaf (only main and inttest import it), so no Bazel payoff is claimed.
- Ports:
  - No new interfaces. Plain struct {Path string; Handler http.Handler} for actuator endpoints; plain func parameters (Go func(func()), OnShutdown func(func())) instead of a Router/BackgroundRunner/Mux interface
- Extension points:
  - New actuator endpoint = new handler file + one table row
  - New startup subsystem = new step function file called from BuildRuntimeDeps; only introduce a Step/Env pipeline if a step is shown to need no new Env field (otherwise it is cosmetic)
- Composition-root wiring: server/dependencies.go remains the single place that lists endpoints and steps; db_maintenance.go and escape_events_migration.go stay *Server methods
- Guard changes:
  - Preserve shutdown-clear semantics of the PR-watch status provider (dependencies_test.go:652-674) while converting to an injected func
- Prerequisites:
  - Owner decision Q4 (backend-di-refactor Story 1.3 ordering)
- Verify:
  - go build ./... && go test ./server/ -run 'Actuator|Dependencies|Wakes|PRWatch'
  - go vet ./server/...
  - make ready

## Wave 4

Physical leaf moves that become honest only after waves 1-3 removed the concrete back-edges. Each must first pass scoping criteria/replay (SC1-SC6) on a throwaway worktree; if SC6 fails the item is recorded as skipped in scoping-ledger.md and the OCP seam from earlier waves stands alone.

### ITEMSRC-move: session -> session/itemsources

- Cost: S (one PR; verified feasible in a prior throwaway: go vet and go test of the new package passed, no cycle)
- Role: adapter
- Payoff: Makes tracker plugins a true leaf: after ITEMSRC-registry only server/dependencies.go (plus tests) imports it. Cache payoff weak (ledger tierA-reeval backlogplugin: lib 0.00, test -0.024 pp); justification is OCP, so gate on owner decision Q1.
- Files moved:
  - session/backlog_plugin_github.go and the Jira, Linear, forward_sync impl files (6 impl files) plus 5 test files, as listed by scoping move --dry-run
  - mapExternalToBacklogItem and maxTrackerResponseBytes move UNEXPORTED into itemsources (call exported session.BoundImportedText)
- Ports:
  - ItemSourcePlugin, PaginatedFetcher, PluginRegistry stay in session (consumer-defined, 4 implementations)
- Exports:
  - Default() []session.ItemSourcePlugin
  - NewGitHubIssuesPlugin
  - NewGitHubPRsPlugin
  - NewLinearPlugin
  - NewJiraPlugin
  - ParseLinearIssueRef
  - ParseJiraIssueRef
- Extension points:
  - New tracker = new file in session/itemsources (or a new sibling leaf) + one Default() entry
  - Closed: session core, registry, SyncLoop, server/mcp and services (they use ports only)
- Composition-root wiring: server/dependencies.go calls itemsources.Default() and hands the slice to the registry
- Tests to move:
  - github_test, jira/linear forward_sync tests (in-package unexported helpers move with them)
  - registry default test moved in wave 2
- Guard changes:
  - scoping budget row for session/itemsources; lower session cap by the moved files/LOC
  - depguard: only server/dependencies.go and tests may import session/itemsources
- Prerequisites:
  - ITEMSRC-registry
  - Owner decision Q1
- Verify:
  - go list -deps ./session | grep itemsources  (nothing)
  - go list -f '{{.ImportPath}}' -deps ./server/... and check importers of itemsources are root + tests only
  - go build ./... && go vet ./... && go test ./session/itemsources ./session ./server/...
  - go run ./tools/bzl/scoping budget, boundaries; Gazelle diff clean

### BL-shadowledger: session/backlog -> session/backlog/shadowledger

- Cost: S (one PR)
- Role: infrastructure (observability recorder)
- Payoff: True leaf: sole production importer is server/dependencies_workflow_engine.go; implements the existing consumer-defined session.ShadowRecorder/ClassedShadowRecorder seam. Moves ~half of the 930 LOC proposal (graph_validator is NOT merged in; it stays or becomes its own stable library later). Payoff unmeasured (2 commits in 180d); replay required.
- Files moved:
  - session/backlog/workflow_engine_shadow.go
  - session/backlog/workflow_engine_shadow_test.go
- Ports:
  - session.ShadowRecorder and session.ClassedShadowRecorder (existing, injected via session.WithShadowRecorder/WithConfigLabeler)
- Exports:
  - ShadowLedgerRecorder
  - NewShadowLedgerRecorder
  - ShadowLedgerDir
  - ShadowLedgerPath
  - ReadShadowLedgerFile
  - NewConfigLabeler
- Extension points:
  - Another shadow sink = new implementation of ShadowRecorder in its own leaf, passed through session.WithShadowRecorder; the engine and selector stay closed
- Composition-root wiring: server/dependencies_workflow_engine.go imports shadowledger and passes the recorder via the existing options (:48-49)
- Tests to move:
  - workflow_engine_shadow_test.go; twin (copy, do not export) selectorHarness, fixedMode, classModes, addGateToSeededEdge, newSeededConfiguredWorkflowEngine, passingReviewItem, setEdgeEnabled into session/backlog/shadowledger/fixtures_test.go
- Guard changes:
  - Update stale header comments in session/gate_config.go and session/stage_config_repository.go
  - server/actuator_test.go import rewrite (5 refs)
  - budget row for the new package; lower session/backlog cap
- Prerequisites:
  - scoping replay shows SC6 pass or owner accepts OCP-only justification (Q1)
- Verify:
  - go build ./session/backlog/shadowledger/ && go test ./session/backlog/... ./server/ -run 'Shadow|Actuator'
  - go list -deps ./session/backlog/shadowledger | grep -c 'session/backlog$'  (0)
  - go vet ./... ; scoping budget; Gazelle clean

## Deferred or rejected

- **session P1-records**: Payoff reject (scoping touch gain 0.00 pp); breaks on alias rules for methods in staying files (InstanceData, PRRef, PRView); pr_set.go drags github and its tmux/git closure; ErrNotFound and ShellRepository leak ent. Revisit only bundled with a consumer split that proves a replay gain.
- **session P2-startupmig (package move)**: Host port cannot compile (Client field vs method) and misses er.driver; back-edges (ResolveMainRepoRoot, GitHubRef, 61 callers) unmet. Replaced by MIG-option in wave 2.
- **session P3-itemsources (as proposed)**: Folded into ITEMSRC-registry (wave 2) and ITEMSRC-move (wave 4) with corrections: drop depends_on P1, keep helpers unexported, fix server/mcp and services direct constructors.
- **session P4-procbackend (package move)**: pm() lazily builds a tmux default for bare &Instance{}; 36 in-package tests cannot import a leaf that imports session; SC6 fails (ledger lib 0.00, universal -0.41). In-place registry (BACKEND-registry) delivers the OCP win.
- **session P5-entstore**: XL; port 'second implementation' is NewTestEntRepository (real ent); 9 staying files declare EntRepository methods; GetEntClient has 37 callers; depends on P1/P2 that do not exist; 149 of 170 ent_* commits also touch core. Only wake+webhook and guidance sub-leaves could be re-proposed after replay.
- **session P6-historyadapters (physical move)**: Step 2 of HIST-slice. Needs canonical/records lowering, HistorySession port defined in a lower package, Instance setters via the actor path, Pi event reader relocation; SC6 near zero.
- **server/services P1-svckit**: Grab-bag; outcome/applied claims false; opens no extension point. At most relocate DerivePriority and RedactErrorDetail inside a consuming PR if backlogsvc back-edge removal is wanted.
- **server/services P2-backlogsync**: Subscriber port speculative (plugin dispatch already open); SC4 fails; deliverygate sink_guard keys break. Do only an injected subscriber-start list at the composition root if wanted.
- **server/services P3-webhooks**: Not a leaf: cycles via CrossHostClaimDedupFlagName, validateTargetDirectory, SSRF validator; Source port speculative; SC6 fail (ledger). Defer until a second webhook provider exists.
- **server/services P4-capacity**: SessionService reads monitor internals (mu, current, clients, tokenStore); real provider switch is token-usage reading; FeatureController/DerivePriority cycle; NewSessionService has 38 callers.
- **server/services P5-autonomous**: OCP reject: SessionService holds concrete AutonomousSvc, calls unexported driverCtx/registerDriver/stopAndDeregisterDriver; StallStrategy invented.
- **backlogsvc P1-workflowdefs, P2-attention, P3-sourcesync, P4-steer, P5-admission, P6-headlessrun**: All payoff reject: receiver extraction of a 58-field hub; core embeds the new package (wrong dependency direction); SC4/SC6 fail; P3 mis-scoped (mostly attach/import, not sync); steer Strategy and Gate interfaces do not fit the real shapes. In-place alternatives (single CacheInvalidator, Gate slice with Prepare/Admit/Outcome, headless methods into own files, role table replacing UUID-prefix switch) are candidates for a later wave once owner answers Q2.
- **session/backlog P1-workflow (combined), P2-stuckpolicy, P3-pipeline, P4-liveness, P5-review**: Combined workflow package is a grab-bag (shadow ledger is split out as BL-shadowledger; graphcheck stays a stable library). stuckpolicy imports the 82k-LOC session; pipeline/review have prompt-builder cycles and 117-caller rewrites; liveness needs a DefaultTable injection with no owner. All fail SC4/SC6. Revisit after DET-0 and a replay.
- **session/backlog P6 waves 1-3 (detector family packages)**: Only wave 0 is accepted (DET-0). livework/archive are guards the hub and PR path call, not detectors; Env under-specified; depends on liveness/stuckpolicy packages that do not exist.
- **server/mcp P1-mcpkit and leaves P2 waketools, P3 terminaltools, P4 sessiontools, P5 svctools, P6 flowtools, P7 backlogtools**: ADR-010 already rejected splitting server/mcp (SC2/SC4). mcpkit would import server/wakes and server/services; ToolModule port belongs to the consumer (now MCP-modules); leaves still import services so no invalidation gain; test helper straddling is large. Re-open after MCP-modules lands and a replay shows gain.
- **server/mcp P8-mcptest**: Conditional on a leaf split that is deferred; real size is ~164 lines plus per-leaf fakes. Do only if a leaf split is approved, as internal/mcptest with testonly=True.
- **server P2 actuator, P3 httpx, P4 wakes/wiring, P5 prwatch/wiring, P6 startup**: Root server is already a leaf; no Bazel payoff; db_maintenance.go is not an actuator; httpx is a grab-bag mixing TLS, metrics, themes and changelog; wakes OCP claims false (installWakeTools is one struct); prwatch/wiring violates server/prwatch/deps_test.go and needs shutdownHooks and ServerDependencies; server/startup creates an import cycle through ServerDependencies/RuntimeDeps. In-place tables and functions are in SRV-wiring.

## Owner decisions

- **2026-11-10, Q1 answered by the owner: "open close shape alone justifies".** A physical move is justified by leaf-ness and an open-closed extension point alone; it does not need to pass ADR-010 SC4/SC6 or show a replayed Bazel gain. Wave 4 items (`ITEMSRC-move`, `BL-shadowledger`) and the `FLAGS-1` stage-1 package are therefore unblocked once their seams exist. SC1 (no new import cycle, at most 10 relocated declarations) and SC3 still apply because they protect feasibility, not payoff. Speculative abstractions are still rejected (consumer-side only, 2+ implementations or a real test seam).
- **2026-11-10, ADR-010 open-PR cap lifted for file-disjoint split PRs (owner).** Waves 2 and 4 were opened together with wave 1 (PRs #1562-#1566) because they touch disjoint areas; the stacked `ITEMSRC-move` PR (#1566) is based on `ITEMSRC-registry` (#1563) and is retargeted to `main` after #1563 merges.

## Owner questions (open)

(Q1 is answered above.)

- Q1: For packages that fail ADR-010 SC6 (ITEMSRC-move, BL-shadowledger, FLAGS-1), is OCP/leaf shape alone sufficient justification to move them, or must scoping replay show a measured gain before each physical move?
- Q2: Do you want the in-place OCP refactors inside backlogsvc (admission Gate slice with Prepare/Admit/Outcome, one CacheInvalidator interface, headless methods into their own files, session-role table replacing the Headless*UUIDPrefix HasPrefix switches in server/mcp and backlog_service_query.go) scheduled as wave 5, given they were rejected as package splits?
- Q3: FeatureFlagDefault has receiver-less callers (wsconn, tests). Is a once-set package-level registry accessor acceptable, or should all callers receive the Registry by injection (larger caller rewrite)?
- Q4: Should SRV-wiring wait for backend-di-refactor Story 1.3 (warren wrapping of Set* calls), or proceed in parallel with a risk of two wiring mechanisms?
- Q5: Is server/mcp acceptable as a second composition root (it assembles tool modules from deps), which the MCP-modules design assumes, or must every module be registered only from server/dependencies.go?
- Q6: Is the history-adapter step 2 (physical session/historyadapters with a lowered canonical/records package and a HistorySession port) wanted at all, given it needs Instance setters via the actor path and has near-zero Bazel payoff?

## Rules for executing

- One split per PR, at most 3 open at once (ADR-010 Decision 4). Waves 1 and 2 are in-place OCP seams plus one pure move.
- Every physical move must still pass `go run ./tools/bzl/scoping criteria` SC1 (boundary budget, no new cycle) and SC3 (isolation). SC4/SC6 (measured payoff) are NOT required: see Owner decisions.
- A new interface is allowed only in the consumer package, with 2+ implementations or a real test seam (`interface-pollution-checklist`).
