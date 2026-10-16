# Validation Plan: hook-classification-fast-path

**Date**: 2026-09-23

## Happy Path Scenario

Given a warm running Stapler Squad instance and a Claude Code `PreToolUse` payload, when `ssq-hooks check` sends one versioned request to that instance, then it emits the policy-equivalent decision within p95 20 ms/p99 50 ms while analytics persistence proceeds asynchronously.

## Requirement → Test Mapping

Requirements below enumerate every bullet in `requirements.md` → **Scope / In Scope**.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: Canonical single hook registration | `internal/claudehooks/claudehooks_test.go` | `InstallRules_should_PreserveUnrelatedHooksAndInstallOneCanonicalGroup_When_DuplicatesExist` | Unit | Happy path |
| REQ-1 | `internal/claudehooks/claudehooks_test.go` | `InstallRules_should_LeaveValidOriginalSettings_When_AtomicWriteFails` | Unit | Error path |
| REQ-1 | `scripts/test-hook-upgrade-rollback.sh` | `install_should_ConvergeToOneInvocation_When_WrappedAndDirectHooksExist` | Integration | Real settings copy, concurrent install, and rollback |
| REQ-2: Versioned local classification transport | `internal/hookipc/protocol_test.go` | `DecodeEnvelope_should_ReturnClassificationEnvelope_When_VersionAndIdentityMatch` | Unit | Happy path |
| REQ-2 | `internal/hookipc/protocol_test.go` | `DecodeEnvelope_should_RejectRequest_When_VersionOrFingerprintMismatches` | Unit | Error path |
| REQ-2 | `internal/hookipc/server_test.go` | `Server_should_ReturnExactPreToolUseReply_When_ClientUsesProtocolV1` | Integration | HTTP/JSON over Unix socket |
| REQ-3: Correct endpoint resolution for every state namespace | `internal/hookipc/endpoint_test.go` | `ResolveEndpoint_should_ReturnDistinctHookEndpoints_When_ConfigDirectoriesDiffer` | Unit | Happy path |
| REQ-3 | `internal/hookipc/endpoint_test.go` | `ResolveEndpoint_should_NotReturnDefaultEndpoint_When_IsolatedEndpointIsMissing` | Unit | Error path |
| REQ-3 | `server/services/hook_injector_test.go` | `ManagedSessions_should_ReachOwningInstance_When_CwdIsShared` | Integration | Default, named, workspace, and test-directory matrix |
| REQ-4: Explicit managed-session identity and mismatch rejection | `server/services/hook_injector_test.go` | `InjectEndpoint_should_SetSocketFingerprintAndProtocol_When_SessionStarts` | Unit | Happy path |
| REQ-4 | `internal/hookipc/server_test.go` | `ServeHTTP_should_NotClassify_When_InstanceFingerprintMismatches` | Unit | Error path |
| REQ-4 | `tests/integration/hook_fast_path_test.go` | `instances_should_NeverCrossRoute_When_ProductionAndManualServersShareCwd` | Integration | Mixed concurrent requests |
| REQ-5: Concurrent immutable classification | `server/services/hook_classifier_test.go` | `Classify_should_UseOneCompleteRuleSnapshot_When_ReloadRaces` | Unit | Happy path under race detector |
| REQ-5 | `server/services/hook_classifier_test.go` | `Classify_should_DeferWithoutPartialDecision_When_SnapshotIsUnavailable` | Unit | Error path |
| REQ-5 | `internal/hookipc/server_test.go` | `Server_should_ProcessRequestsConcurrently_When_OneClassificationIsSlow` | Integration | No head-of-line blocking |
| REQ-6: Context caching/coalescing without decision coalescing | `server/services/hook_context_cache_test.go` | `Get_should_CoalesceRepositoryLookup_When_CwdMissesAreConcurrent` | Unit | Happy path |
| REQ-6 | `server/services/hook_context_cache_test.go` | `Get_should_ReturnStaleContextWithinBudget_When_RefreshIsSlow` | Unit | Error path |
| REQ-6 | `server/services/hook_deduper_test.go` | `Deduper_should_ProcessDistinctToolUseIDs_When_CommandsAreIdentical` | Integration | Context shared; decisions remain distinct |
| REQ-7: Batched bounded-loss analytics actor | `server/services/analytics_store_test.go` | `Enqueue_should_ReturnImmediatelyAndFlushBatch_When_EventsArrive` | Unit | Happy path |
| REQ-7 | `server/services/analytics_store_test.go` | `Enqueue_should_DropAndCount_When_QueueIsFull` | Unit | Error path |
| REQ-7 | `session/ent_repository_analytics_test.go` | `PersistBatch_should_CommitAtomically_When_UsingWALDatabase` | Integration | Real SQLite WAL batch transaction |
| REQ-8: Safe coalescing and stable-ID deduplication | `server/services/hook_deduper_test.go` | `Do_should_ReuseDecisionAndEvent_When_HookRequestIDRepeats` | Unit | Happy path |
| REQ-8 | `server/services/hook_deduper_test.go` | `Do_should_NotDeduplicate_When_ToolUseIDIsMissing` | Unit | Error/safety path |
| REQ-8 | `tests/integration/hook_fast_path_test.go` | `duplicateDelivery_should_WriteOneEvent_When_StableIdentityRepeats` | Integration | Client/server/actor path |
| REQ-9: Research broader actor/database/shard options | `server/services/analytics_storage_benchmark_test.go` | `BenchmarkAnalyticsStorage_should_RecordPrimaryAndDedicatedLayouts_When_DatasetIsRepresentative` | Unit/Benchmark | Happy evidence path |
| REQ-9 | `session/write_contention_benchmark_test.go` | `BenchmarkWriteContention_should_KeepGlobalActorOutOfScope_When_ThresholdIsNotBreached` | Unit/Benchmark | Negative decision path |
| REQ-9 | `server/services/analytics_storage_benchmark_test.go` | `storageGate_should_ProduceExplicitVerdict_When_BenchmarksComplete` | Integration | ADR evidence is complete; no ad-hoc migration |
| REQ-10: Cached-rule fallback then defer | `cmd/ssq-hooks/fallback_test.go` | `Fallback_should_ReturnCachedDecision_When_SnapshotIsValidAndInstanceBound` | Unit | Happy path |
| REQ-10 | `cmd/ssq-hooks/fallback_test.go` | `Fallback_should_EmitEmptyOutput_When_CacheIsExpiredCorruptOrMismatched` | Unit | Error path |
| REQ-10 | `tests/integration/hook_fast_path_test.go` | `client_should_UseCacheThenDefer_When_IntendedServerStops` | Integration | Never opens DB or routes elsewhere |
| REQ-11: Complete safe observability | `instrumentation/hook_metrics_test.go` | `RecordHookRequest_should_ExposeRequiredLowCardinalityDimensions_When_RequestCompletes` | Unit | Happy path |
| REQ-11 | `instrumentation/hook_metrics_test.go` | `RecordHookRequest_should_RedactSensitiveFields_When_ErrorOccurs` | Unit | Error path |
| REQ-11 | `tests/integration/hook_fast_path_test.go` | `telemetry_should_ReportLatencyFallbackQueueLossAndConsistency_When_FaultsAreInjected` | Integration | Metrics and alerts exercise |
| REQ-12: Concurrency, isolation, protocol, failure, benchmark, manual tests | `tests/integration/hook_fast_path_benchmark_test.go` | `BenchmarkFastPath_should_MeetLatencySLO_When_100ClientsSend100RPS` | Unit/Benchmark | Happy load path |
| REQ-12 | `tests/integration/hook_fast_path_test.go` | `fastPath_should_DeferSafely_When_SocketCacheProtocolAndSinkFail` | Unit | Failure matrix |
| REQ-12 | `scripts/test-hook-fast-path.sh` | `cli_should_MeetSLOAndIsolation_When_RealProcessesRunConcurrently` | Integration | Real subprocess end-to-end |
| REQ-13: Transport-neutral protocol boundary | `internal/hookipc/protocol_test.go` | `Protocol_should_NotDependOnUnixSocketTypes_When_EnvelopeRoundTrips` | Unit | Happy path |
| REQ-13 | `internal/hookipc/protocol_test.go` | `Protocol_should_RejectUnknownRequiredFields_When_IncompatiblePeerConnects` | Unit | Error path |
| REQ-13 | `internal/hookipc/client_test.go` | `Client_should_UseTransportAdapter_When_EndpointIsUnixSocket` | Integration | Local adapter without SSH implementation |
| REQ-14: Direct-replacement deployment and rollback | `internal/claudehooks/claudehooks_test.go` | `Replace_should_CreateBackupAndCompleteAtomically_When_PrimaryIsCompatible` | Unit | Happy path |
| REQ-14 | `internal/claudehooks/claudehooks_test.go` | `Replace_should_KeepOldConfiguration_When_HealthCheckFailsOrWriteIsInterrupted` | Unit | Error path |
| REQ-14 | `scripts/test-hook-upgrade-rollback.sh` | `rollback_should_RestoreWorkingPriorHook_When_NewVersionIsRemoved` | Integration | Mixed-version and interruption matrix |

## Cross-cutting Semantic and Performance Gates

| Gate | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Existing allow/deny/defer parity | `server/services/hook_classifier_test.go` | `Classify_should_MatchStandaloneCheck_When_GoldenPayloadCorpusRuns` | Differential | Sanitized Bash, Write, and AskUserQuestion corpus; any mismatch fails release |
| No repository bootstrap on hook path | `cmd/ssq-hooks/main_test.go` | `HandleCheck_should_NotOpenRepository_When_PrimaryOrFallbackRuns` | Unit | Repository constructor panic spy remains untouched |
| Analytics cannot affect response latency | `tests/integration/hook_fast_path_benchmark_test.go` | `fastPath_should_MeetSLO_When_AnalyticsSinkIsBlocked` | Integration | Locked sink under load |
| WAL remains enabled | `session/ent_repository_analytics_test.go` | `analyticsDatabase_should_UseWALMode_When_BatchSinkStarts` | Integration | Assert `PRAGMA journal_mode=wal` on actual connection |
| Safe live-database handling | `server/services/analytics_storage_benchmark_test.go` | `storageResearch_should_NeverCopyLiveMainFileAlone_When_WALHasFrames` | Integration | Only SQLite backup/closed immutable shard techniques allowed |

## UX Acceptance Tests

These are CLI/manual/dashboard behaviors; browser automation is not applicable to the local hook process. Dashboard checks use the existing Grafana test/development environment.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Hook stdout contains only valid decision JSON | `scripts/test-hook-fast-path.sh` | `hook_should_EmitOnlyProtocolJSON_When_AllowedOrDenied` | Shell/manual | Invoke allow/deny payloads; parse stdout as one JSON object; verify stderr separately |
| Warm latency meets SLO | `scripts/test-hook-fast-path.sh` | `hook_should_MeetP95AndP99_When_Warm` | Shell | Run real subprocess workload; assert p95 <20 ms and p99 <50 ms |
| Errors do not contaminate JSON | `cmd/ssq-hooks/main_test.go` | `hook_should_KeepDiagnosticsOffStdout_When_ErrorOccurs` | Go/manual | Inject connection error; assert stdout empty and diagnostics safe |
| AskUserQuestion/defer emits empty stdout | `cmd/ssq-hooks/main_test.go` | `hook_should_EmitEmptyStdout_When_AskUserQuestionOrFinalDefer` | Go | Exercise both paths |
| Doctor reports all health dimensions | `cmd/ssq-hooks/doctor_test.go` | `doctor_should_ReportEndpointProtocolCacheFallbackAndActor_When_Degraded` | Go/manual | Stop manual server; inspect text and JSON modes |
| Doctor redacts sensitive values | `cmd/ssq-hooks/doctor_test.go` | `doctor_should_NotExposeSensitiveFields_When_HealthContainsSecrets` | Go | Seed sentinel command/cwd/token/session/path values; assert absent |
| Doctor status is not color-only | `cmd/ssq-hooks/doctor_test.go` | `doctor_should_UseTextualStatus_When_ColorIsDisabled` | Go/manual | Run with no-color terminal and assert HEALTHY/DEGRADED/INCOMPATIBLE text |
| Doctor gives remediation and exits promptly | `cmd/ssq-hooks/doctor_test.go` | `doctor_should_ExitPromptlyWithAction_When_EndpointFails` | Go | Simulate timeout; assert one action and bounded runtime |
| Doctor JSON supports automation | `cmd/ssq-hooks/doctor_test.go` | `doctor_should_EmitStableJSON_When_JSONFlagIsSet` | Go | Parse output and assert schema/version |
| Installer reports normalization outcome | `scripts/test-hook-upgrade-rollback.sh` | `installer_should_ReportCountsBackupAndRollback_When_DuplicatesAreRemoved` | Shell/manual | Install into fixture and inspect output |
| Installer is interruption-safe | `scripts/test-hook-upgrade-rollback.sh` | `installer_should_LeaveCompleteJSON_When_InterruptedAtWriteBoundaries` | Shell | Fault-inject every boundary and parse resulting settings |
| Installer checks compatibility before cutover | `scripts/test-hook-upgrade-rollback.sh` | `installer_should_KeepOldHook_When_ProtocolIsIncompatible` | Shell | Start incompatible server; verify old config unchanged |
| Installer never prints settings secrets | `internal/claudehooks/claudehooks_test.go` | `installer_should_RedactSettings_When_ReportingFailure` | Go | Place sentinel secret in unrelated setting and inspect output |
| Dashboard keeps instance classes separate | companion dashboard review | `dashboard_should_FilterInstanceClassesWithoutMerging_When_DataExists` | Grafana/manual | Select production/manual/test filters; compare series |
| Dashboard status is textually accessible | companion dashboard review | `dashboard_should_UseNamedPanelsAndTextStatus_When_ViewedWithoutColor` | Grafana/manual | Inspect labels/table and color-independent status |
| Dashboard percentiles/max render | companion dashboard review | `dashboard_should_RenderP95P99AndMax_When_NewMetricsArrive` | Grafana/manual | Load fixture metrics and verify non-empty panels |
| Dashboard preserves baseline annotation | companion dashboard review | `dashboard_should_ShowBaseline_When_TimeRangeIncludesMigration` | Grafana/manual | Verify 4,419/day, 38,661 ms, ~40/min annotation |
| Dashboard exposes rollback thresholds | companion dashboard review | `dashboard_should_ShowCorrectnessAndLatencyThresholds_When_Degraded` | Grafana/manual | Inject mismatch/fallback/p99 fixture and verify visible thresholds |

## Test Stack

- **Unit**: Go `testing`, table tests, fakes/spies, injected clock, `go test -race`.
- **Integration**: Go tests with temporary Unix sockets and real temporary modernc SQLite databases in WAL mode; shell tests for real CLI processes and settings mutation.
- **Benchmark/load**: Go benchmarks plus `scripts/test-hook-fast-path.sh`, including 100 concurrent clients at 100 requests/second for 60 seconds.
- **E2E / UX**: shell/manual CLI checklist and Grafana manual/fixture validation; no browser UI is introduced.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -race -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line in new/changed packages |
| Integration | `go test ./tests/integration/... -count=1` | All isolation/failure cases pass |
| Real CLI load | `scripts/test-hook-fast-path.sh` | p95 <20 ms, p99 <50 ms, zero cross-instance replies |

- All public service methods: happy path and error paths covered.
- All external integrations: unit mocked plus at least one real Unix socket/SQLite integration.
- Every Scope/In Scope requirement: at least one happy, one error/safety, and one integration or evidence test.
- Every UX acceptance criterion: corresponding automated or manual acceptance test.
- **Migration test**: N/A. The mandatory path has no schema migration; any research-gated dedicated database/shard work requires a separate migration plan and `migration_should_be_reversible` test before approval.
