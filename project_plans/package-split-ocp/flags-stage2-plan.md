# FLAGS-1 stage 2: inject the flag Registry everywhere, delete the global

Owner decision (roadmap Q3, 2026-11-19): inject `*featureflags.Registry`; no once-set package-level accessor. Stage 1 left
exactly one such accessor, which this plan removes.

## Measured state (origin/main 0b183b1d3)

The global is `featureflags.current` (`atomic.Pointer`) behind `SetCurrent` / `Current()` in
`server/services/featureflags/registry.go`. Two reads reach it: `featureflags.Current()` directly and
`services.FeatureFlagDefault(name)` (`feature_flag_service.go`), which is `Current().Default(name)`.

| Reader | Where | Has a carrier? |
|---|---|---|
| `FeatureFlagService` methods (list, update, persistScoped, persistAndApply, statusDetailFor) | `feature_flag_service.go`, `feature_flag_mutation.go` | yes, it is the struct (slice 1 done) |
| `parseFlagOp`, `clearTurnsKindOn`, `autonomyFlagStatusDetail` | same | called only from the service methods (slice 1 threads `reg`) |
| `globalOffStatusDetail(cfg)` | `feature_flag_mutation.go` | no, passed as a func value to `AddConfigStatusDetailSource` |
| `wireNotificationActionFlags` | `feature_flags_notification_actions.go` | yes, takes `*FeatureFlagService` |
| `scoped`, `dispatcherRunning`, `resolveAnswerQuestion` (+ `effectiveFlag`) | `feature_flags_notification_actions.go` | no, free functions over `cfg` |
| `ResolveAttentionFlags(cfg)` | `feature_flags_attention.go`; one prod caller, `LoadAttentionFlags` (line ~46), used by `server/attention_options.go` | no |
| `WorkflowEngineModeFromConfig(cfg)` | `feature_flag_service.go` | no (one prod caller in the same file) |
| `ChangelogEnabled(cfg)`, `UpdateCheckFlagEnabled(cfg)` | `update_check_service.go` | no; prod caller `server/changelog_handler.go` |
| `SddDefaultPipelineEnabled()` | `backlog_shared.go` | no; prod caller `backlogsvc/backlog_service_lifecycle.go` |
| `GitHubIssueProgressCommentsEnabled()`, `ProgramCLIFlagProbeEnabled()` | `backlog_github_progress_sync.go`, `feature_flag_service.go` | no |
| `filesFlagEnabled(name)` via `FilesEditingEnabled`, `FilesViewerV2Enabled`, `FilesMergePhoneEnabled`, `FilesRangeReadEnabled`, `FilesDeltaWriteEnabled`, `FilesEditLargeEnabled` (`feature_flag_service.go:134-154`) | same file | no; the six `Files*Enabled(ctx)` wrappers are the prod entry points |
| `currentResyncOptions()` | `server/services/wsconn/connectrpc_websocket.go:2857` (`services.FeatureFlagDefault`) | no free-function carrier; wsconn already imports `services` |

Installers of the global: `BuildCoreDepsWithOptions` in `server/dependencies.go` (production); test mains in `server/services`,
`server/services/backlogsvc`, `server/services/wsconn`, `session/inttest`. Test readers of `featureflags.Current()`: 8 existing files in `server/services` plus this PR's new
`feature_flag_injected_registry_test.go` (4 uses; measured with `git grep -c 'featureflags.Current()' -- '*_test.go'`). Non-test uses: `BuildCoreDepsWithOptions` (`SetCurrent`), `FeatureFlagDefault` and `FeatureFlagService.reg()`'s fallback. `go list -deps ./server/services/featureflags` imports `config` only.

## Slices (each one PR, no behaviour change)

1. **Seam beside the global (this PR).** `Registry.Enabled(cfg, name)` and `Registry.EnabledScoped(cfg, name, scope)`;
   `FeatureFlagService` gets an injected registry (`NewFeatureFlagServiceWithRegistry`, nil falls back to `Current()`) and
   every flag read inside the service and its mutation helpers uses it. Files: `featureflags/registry.go`,
   `feature_flag_service.go`, `feature_flag_mutation.go`, tests. Risk: low, constructor default unchanged. Proof: services
   test-name diff empty plus new tests; mutation checks on the seam. Rollback: revert.
2. **Composition root owns the registry.** `BuildCoreDepsWithOptions` builds one `*Registry`, passes it to `SessionService`
   so `NewFeatureFlagServiceWithRegistry` is used in production, and exposes it on `CoreDeps`. Still also calls `SetCurrent`.
   Files: `server/dependencies.go`, `session_service.go` (constructor field), tests. Risk: low. Proof: `/api` GetFeatureFlags
   golden equal before/after. **Done.** `BuildCoreDepsWithOptions` builds one `services.FlagRegistry()`, passes the same
   instance to `SetCurrent`, to `NewSessionServiceFromConfig`/`NewSessionServiceWithEntClient` (new `flags` argument,
   forwarded by `newGatedSessionService` to `NewFeatureFlagServiceWithRegistry`) and to `CoreDeps.FlagRegistry`;
   `SessionService.FeatureFlagRegistry()` exposes the service's registry. Proof: `server/dependencies_flag_registry_test.go`
   (deps registry == `Current()` == service registry, and the service keeps the injected registry after the global is replaced).
   The three test call sites of `newGatedSessionService` (`feature_flag_gate_test.go` twice, `session_service_events_test.go` once) pass `featureflags.Current()` (unchanged behaviour). Fragility: `NewSessionService` builds a default flag service and `newGatedSessionService` swaps it, so a capture added inside `NewSessionServiceWithSearchEngine` would bind to the dropped object; slice 7 should pass the registry into the constructor and delete the swap. Test-side `NewFeatureFlagService()` has 47 occurrences in 12 files (`git grep -c 'NewFeatureFlagService()' -- '*_test.go'`).
3. **Notification-actions and attention readers.** Convert the free functions to a small struct (`flagReader{reg}`) or
   methods taking `reg`; `wireNotificationActionFlags` passes the service's registry. Files: `feature_flags_notification_actions*.go`,
   `feature_flags_attention*.go`, and `server/attention_options.go` (`liveAttentionFlags` loads via `services.LoadAttentionFlags`,
   so the cache takes the loader as a constructor argument). Risk: medium (hot poller paths, config-surface proofs). Proof: `config_surface_proof_*` tests
   green (only the helper call syntax in `config_surface_proof_flags_test.go` changed, 4 lines).
   **Done.** `flagReader{reg}` (nil falls back to `Current()`) owns `effectiveFlag`, `effectiveAnswerQuestion`, `resolveEffectiveFlag`, `resolveStage`, `scoped`, `dispatcherRunning`, `resolveAnswerQuestion`; `wireNotificationActionFlags` seeds from `ff.reg()`. `ResolveAttentionFlags`/`LoadAttentionFlags`, `NewDescriptorBuilder`, `NewGuidanceHookMirror` and `NewNotificationActionDispatcher` take a trailing `*featureflags.Registry`; `configActionFlags` carries a `flagReader`. `server.newAttentionOptionsSource(core.FlagRegistry)` replaces the `liveAttentionFlags` global and is shared via `ServiceDeps.AttentionOptions`. Known gap: `newGatedSessionService` still calls `wireNotificationActionFlags`, which resolves `ff.reg()` eagerly (falls back to `Current()` when no registry was injected); main did the same through `FeatureFlagDefault`, and slice 7 removes the fallback. Proof: `feature_flags_reader_injected_registry_test.go`, `feature_flags_reader_threading_test.go`, `server/flag_registry_wiring_test.go`.
4. **Per-feature readers.** `ChangelogEnabled`/`UpdateCheckFlagEnabled` (changelog handler gets the registry),
   `SddDefaultPipelineEnabled` (backlogsvc service field), `GitHubIssueProgressCommentsEnabled`,
   `ProgramCLIFlagProbeEnabled`, `filesFlagEnabled`, `WorkflowEngineModeFromConfig`, `globalOffStatusDetail`.
   One package per sub-PR if the diff passes ~400 lines. Risk: medium. Proof: existing reader tests re-pointed at an
   injected registry; `config_surface_proof_flags*` green.
5. **wsconn.** `currentResyncOptions` becomes a method on the type that owns the resync call (or takes a `func(string) bool`
   from `connectrpc_websocket.go`'s constructor). Drops wsconn's last `services.FeatureFlagDefault` use. Risk: low-medium
   (terminal hot path; keep the config read per call). Proof: wsconn tests with an injected registry.
6. **Tests.** Replace `SetCurrent` in the four test mains and the 8 existing `featureflags.Current()` test files (the new injected-registry test keeps its deliberate `Current()` comparison until slice 7 rewrites it) with a shared test
   registry built by `services.FlagRegistry()`; mechanical.
7. **Delete the global and ratchet.** Remove `current`, `SetCurrent`, `Current`, `services.FeatureFlagDefault`, and the `FeatureFlagService.reg()` fallback to `Current()` (a nil registry then becomes a constructor error or panic). Add a
   test in `featureflags` that parses the package and fails on any package-level `var` of type `*Registry` /
   `atomic.Pointer[Registry]`, and a depguard-style grep test in `server/services` that fails on `FeatureFlagDefault(`.
   Verify: `git grep -nE 'FeatureFlagDefault|featureflags\.Current|SetCurrent' -- '*.go'` is empty. Rollback: revert (the
   injected paths stay valid).

Order: 1 -> 2 -> {3, 4, 5 in any order} -> 6 -> 7. Slices 3-5 can run in parallel worktrees since they touch disjoint files.

## Mixed-state caveat (slices 1-5)

Until every reader migrates, an injected registry that differs from the global leaves the unmigrated readers
(`globalOffStatusDetail`, `scoped()`/`dispatcherRunning`, attention, update-check, `backlog_shared`, wsconn) reading the
global. That is acceptable only because production installs the same registry through `SetCurrent`; slice 2 must therefore
build the injected registry and the `SetCurrent` registry from ONE `FlagRegistry()` call, never two.
