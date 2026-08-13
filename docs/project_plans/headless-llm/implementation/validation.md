# Validation Plan: headless-llm

**Date**: 2026-05-26

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| FR-1: Pool keyed by FeatureKey | `session/headless/pool_test.go` | `TestPool_CallBlocking_FirstCall_CapturesSessionID` | Unit | First call establishes session via JSON path, stores session_id |
| FR-1: Pool keyed by FeatureKey | `session/headless/pool_test.go` | `TestPool_DifferentKeys_RunInParallel` | Unit | Two feature keys produce independent sessions with no cross-contamination |
| FR-1: Pool keyed by FeatureKey | `session/headless/pool_test.go` | `TestPool_RotatesSession_AfterMaxCalls` | Unit | Session ID changes after MaxCallsPerSession (2 in test) calls |
| FR-1: Pool keyed by FeatureKey | `session/headless/pool_test.go` | `TestPool_RotatesSession_AfterConsecutiveErrors` | Unit | Session ID reset after 3 consecutive subprocess errors |
| FR-1: Pool keyed by FeatureKey | `session/headless/integration_test.go` | `TestPool_RealClaude_SessionResumption` | Integration | Second call on same key sends `--resume <session_id>` flag |
| FR-2: Call() / CallBlocking() streaming | `session/headless/pool_test.go` | `TestPool_CallBlocking_ReturnsCollectedText` | Unit | CallBlocking collects all StreamChunks into a single string |
| FR-2: Call() / CallBlocking() streaming | `session/headless/pool_test.go` | `TestPool_Call_ContextCancel_ClosesChannel` | Unit | Context cancellation closes the channel and produces no goroutine leak |
| FR-2: Call() / CallBlocking() streaming | `session/headless/pool_test.go` | `TestPool_Call_MultiLineOutput_StreamsInOrder` | Unit | Resumed call streams each output line as a separate StreamChunk in order |
| FR-2: Call() / CallBlocking() streaming | `session/headless/pool_test.go` | `TestPool_CallBlocking_PropagatesSubprocessError` | Unit | Non-zero exit code from FakeRunner is surfaced as non-nil error |
| FR-2: Call() / CallBlocking() streaming | `session/headless/integration_test.go` | `TestPool_RealClaude_SimplePrompt` | Integration | Real claude binary returns non-empty result for a simple prompt |
| FR-3: Cache optimization flags | `session/headless/pool_test.go` | `TestPool_FirstCall_ArgsContainOutputFormatJSON` | Unit | First call args include `--output-format json` and `--system-prompt` and `--exclude-dynamic-system-prompt-sections` |
| FR-3: Cache optimization flags | `session/headless/pool_test.go` | `TestPool_ResumedCall_ArgsContainResumeAndExclude` | Unit | Second call args include `--resume <session_id>` and `--exclude-dynamic-system-prompt-sections`; do NOT include `--output-format json` |
| FR-3: Cache optimization flags | `session/headless/pool_test.go` | `TestPool_FirstCall_ModelFlagIncluded_WhenNonEmpty` | Unit | Non-empty model value adds `--model <model>` to first-call args |
| FR-3: Cache optimization flags | `session/headless/pool_test.go` | `TestPool_ParsesSessionIDFromFirstCallJSON` | Unit | FakeRunner returns `{"session_id":"abc","result":"hello","cost_usd":0.001}`; asserts sessionID stored as "abc" |
| FR-4: Auth / ErrClaudeNotFound | `session/headless/pool_test.go` | `TestNewPool_ReturnsErrClaudeNotFound_WhenBinaryMissing` | Unit | `NewPool()` with fake PATH containing no claude binary returns `ErrClaudeNotFound` |
| FR-4: Auth / ErrClaudeNotFound | `session/headless/pool_test.go` | `TestNewPoolWithRunner_DoesNotCallLookPath` | Unit | `NewPoolWithRunner()` constructor accepts FakeRunner without PATH check |
| FR-4: Auth / ErrClaudeNotFound | `session/headless/integration_test.go` | `TestPool_RealClaude_SimplePrompt` | Integration | Real pool creation succeeds when claude binary is in PATH; OAuth tokens inherited |
| FR-5: Replace SpawnReviewSession | `session/backlog_lifecycle_test.go` | `TestBacklogLifecycleListener_SpawnReviewGate_UsesHeadlessPool` | Unit | spawnReviewGate calls headlessPool.CallBlocking instead of sessionCreator.SpawnReviewSession |
| FR-5: Replace SpawnReviewSession | `session/backlog_lifecycle_test.go` | `TestBacklogLifecycleListener_SpawnReviewGate_ContextFromLifecycle_NotRPC` | Unit | goroutine uses lifecycle context; cancelling RPC context does not abort the review |
| FR-5: Replace SpawnReviewSession | `session/backlog_lifecycle_test.go` | `TestBacklogLifecycleListener_SpawnReviewGate_HeadlessError_DoesNotPanic` | Unit | headlessPool.CallBlocking error is handled gracefully; no panic |
| FR-5: Replace SpawnReviewSession | `server/services/session_service_test.go` | `TestSessionService_SpawnReviewSession_MethodRemoved` | Regression | `SpawnReviewSession` method no longer exists on SessionService (compile-time check via `var _ = (*SessionService)(nil)`) |
| FR-6: RunOneShot enhancement | `server/services/session_service_test.go` | `TestRunOneShot_UsesHeadlessPool_NotSafeexec` | Unit | RunOneShot handler calls headlessPool.CallBlocking; mock pool returns test output |
| FR-6: RunOneShot enhancement | `server/services/session_service_test.go` | `TestRunOneShot_DefaultTimeout_Is900s` | Unit | WithTimeout default is 900s when req.Msg.TimeoutSeconds == 0 |
| FR-6: RunOneShot enhancement | `server/services/session_service_test.go` | `TestRunOneShot_CustomTimeout_IsRespected` | Unit | req.Msg.TimeoutSeconds = 300 produces 300s timeout |
| FR-6: RunOneShot enhancement | `server/services/session_service_test.go` | `TestRunOneShot_WorkDir_PassedToPool` | Unit | WorkDir from request is threaded through to the pool CallOptions |
| FR-6: RunOneShot enhancement | `server/services/session_service_test.go` | `TestRunOneShot_PRURLExtraction_Preserved` | Regression | Output containing a GitHub PR URL is still extracted from the headless response |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestSummarizeBacklogItem_ReturnsText_WhenFakeRunnerResponds` | Unit | FakeRunner returns valid summary JSON; function parses and returns summary field |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestSummarizeBacklogItem_Error_WhenPoolFails` | Unit | FakeRunner returns error; function surfaces non-nil error |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestGenerateAcceptanceCriteria_ParsesJSONArray` | Unit | FakeRunner returns `["AC1","AC2","AC3"]`; function returns slice of 3 strings |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestGenerateAcceptanceCriteria_Error_WhenJSONInvalid` | Unit | FakeRunner returns malformed JSON; function returns parse error |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestDraftPRDescription_ReturnsText_WhenFakeRunnerResponds` | Unit | FakeRunner returns PR description text; function returns it unchanged |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestDraftPRDescription_TruncatesDiff_WhenOver40000Bytes` | Unit | 50,000-byte diff is truncated to 40,000 bytes before being passed to pool |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestSuggestCommitMessage_ReturnsText_WhenFakeRunnerResponds` | Unit | FakeRunner returns commit message; function returns it |
| FR-7: Background AI features | `session/headless/features_test.go` | `TestSuggestCommitMessage_TruncatesDiff_WhenOver20000Bytes` | Unit | 30,000-byte diff is truncated to 20,000 bytes before being passed to pool |
| FR-8: RunHeadlessCall RPC | `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_StreamsChunks` | Unit | FakeRunner returns multi-line output; all chunks received in order by the test client |
| FR-8: RunHeadlessCall RPC | `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_InvalidFeatureKey_ReturnsInvalidArgument` | Unit | Empty or unknown feature_key returns connect.CodeInvalidArgument |
| FR-8: RunHeadlessCall RPC | `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_ContextCancel_StopsSubprocess` | Unit | Client context cancellation stops the pool goroutine and handler returns nil |
| FR-8: RunHeadlessCall RPC | `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_AllowedFeatureKeys` | Unit | Each of "review", "summarize", "pr-description", "commit-message", "custom" accepted without error |
| FR-8: RunHeadlessCall RPC | `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_DefaultTimeout_Is900s` | Unit | timeout_seconds=0 in request applies 900s context timeout |
| FR-8: RunHeadlessCall RPC | `session/headless/integration_test.go` | `TestRunHeadlessCall_RPC_EndToEnd` | Integration | Full RPC call with real pool returns at least one non-empty chunk and done=true as final message |
| NFR-1: Same-key calls serialized | `session/headless/pool_test.go` | `TestPool_SameKey_ConcurrentCalls_Serialized` | Unit | Two goroutines call the same feature key; FakeRunner records call order; assert no concurrent execution of same-key session |
| NFR-1: Different-key calls parallel | `session/headless/pool_test.go` | `TestPool_DifferentKeys_RunInParallel` | Unit | Two goroutines on different keys run concurrently; both complete within single-call timeout |
| NFR-2: Max 5 concurrent sessions | `session/headless/pool_test.go` | `TestPool_ConcurrencySemaphore_LimitsToMax` | Unit | Pool with MaxConcurrentSessions=2 and 5 simultaneous calls; assert never more than 2 FakeRunner.Run() calls active at once |
| NFR-4: FakeRunner / goleak | `session/headless/pool_test.go` | `TestMain` (goleak.VerifyTestMain) | Unit | TestMain with goleak detects any goroutine not cleaned up after each test |
| NFR-4: FakeRunner / goleak | `session/headless/pool_test.go` | `TestPool_Call_ContextCancel_ClosesChannel` | Unit | Channel closed and goroutine exits within 1s of context cancellation |
| NFR-5: RunOneShot backward compat | `server/services/session_service_test.go` | `TestRunOneShot_RPCSignature_Unchanged` | Regression | `RunOneShot` method signature matches proto-generated interface (compile check) |
| NFR-5: SpawnReviewSession removed | `session/backlog_lifecycle_test.go` | `TestReviewGateSpawner_InterfaceRemoved` | Regression | File `session/backlog_lifecycle.go` no longer exports `ReviewGateSpawner` (verified via `go vet` in CI) |

---

## Test Stack

- **Unit**: Go standard `testing` package + `testify/assert` + `testify/require` + `FakeRunner` (in-package test double, no real claude binary)
- **Integration**: Go standard `testing` package + `//go:build integration` build tag; skipped unless `go test -tags integration` is passed; requires real `claude` binary in PATH
- **Regression**: Compile-time interface checks (`var _ Interface = (*Impl)(nil)`) + targeted behavioral tests for backward-compat paths (`RunOneShot` signature, PR URL extraction, `SpawnReviewSession` removal)
- **Goroutine leak detection**: `go.uber.org/goleak` via `goleak.VerifyTestMain(m)` in `session/headless/pool_test.go` TestMain

---

## Coverage Targets

- Unit test line coverage: ≥80% across `session/headless/` package
- All public Pool methods (`Call`, `CallBlocking`, `NewPool`, `NewPoolWithRunner`): happy path + at least 2 error paths each
- All feature functions (`SummarizeBacklogItem`, `GenerateAcceptanceCriteria`, `DraftPRDescription`, `SuggestCommitMessage`): happy path + error path + truncation behavior
- All external integrations (`ProcessRunner.Run`, `HeadlessService.RunHeadlessCall`): unit-mocked via `FakeRunner` + at least one integration test per integration point
- `ClaudeRunner` interface: `FakeRunner` implementation covers all paths exercised by Pool internals

---

## Adversarial Concern → Test Coverage

Each concern from `adversarial-review.md` has a corresponding test to prevent regression:

| Concern | Guarding Test |
|---|---|
| Mutex deadlock: per-key lock held during subprocess | `TestPool_SameKey_ConcurrentCalls_Serialized` — verifies lock is released before Run(); also `TestPool_RotatesSession_AfterConsecutiveErrors` exercises the rotate-under-lock path |
| DefaultPool global race in tests | `TestPool_DefaultPool_SetAndGet_ThreadSafe` — concurrent readers and one writer; run with `-race` |
| ManagedProcess leak on channel abandon | `TestPool_Call_ContextCancel_ClosesChannel` with goleak — asserts goroutine exits within deadline |
| spawnReviewGate blocks on lifecycle context | `TestBacklogLifecycleListener_SpawnReviewGate_ContextFromLifecycle_NotRPC` |
| RunOneShot loses workDir | `TestRunOneShot_WorkDir_PassedToPool` |
| Non-zero exit not surfaced in StreamChunk | `TestPool_CallBlocking_PropagatesSubprocessError` + `TestPool_Call_ExitCode1_SetsErrLLMError` |
| MaxConcurrentSessions never enforced | `TestPool_ConcurrencySemaphore_LimitsToMax` |
| FakeRunner ignores args for JSON vs plain | `TestPool_FirstCall_ArgsContainOutputFormatJSON` — FakeRunner inspects args; first-call returns JSON |
| Circular import session/headless → session | Build-time check: `go build ./session/headless/...` in CI (no session import allowed) |

---

## Additional Tests (Edge Cases)

| Test File | Test Name | Type | Scenario |
|---|---|---|---|
| `session/headless/pool_test.go` | `TestPool_Call_ExitCode1_SetsErrLLMError` | Unit | FakeRunner returns exit code 1; final StreamChunk.Err wraps ErrLLMError |
| `session/headless/pool_test.go` | `TestPool_Call_ExitCode130_SetsErrInterrupted` | Unit | FakeRunner returns exit code 130; final StreamChunk.Err wraps ErrInterrupted |
| `session/headless/pool_test.go` | `TestPool_DefaultPool_SetAndGet_ThreadSafe` | Unit | 10 goroutines read DefaultPool while 1 writes; run with -race; no data race |
| `session/headless/pool_test.go` | `TestPool_ZeroCallsPerSession_UsesDefault25` | Unit | PoolConfig with MaxCallsPerSession=0 falls back to default of 25 |
| `session/headless/pool_test.go` | `TestFakeRunner_InspectsArgs_ReturnsJSONForFirstCall` | Unit | FakeRunner with args containing "--output-format","json" returns scripted JSON response |
| `session/headless/pool_test.go` | `TestFakeRunner_InspectsArgs_ReturnsPlainForResumedCall` | Unit | FakeRunner with args NOT containing "--output-format" returns plain text response |
| `session/headless/features_test.go` | `TestGenerateAcceptanceCriteria_EmptyResponse_ReturnsError` | Unit | FakeRunner returns empty string; function returns error (cannot parse empty as JSON array) |
| `server/services/headless_service_test.go` | `TestHeadlessService_RunHeadlessCall_PoolNil_ReturnsUnavailable` | Unit | HeadlessService constructed with nil pool returns connect.CodeUnavailable |

---

## Test Count Summary

| Type | Count |
|---|---|
| Unit (happy path) | 29 |
| Unit (error path) | 16 |
| Unit (edge case / concurrency) | 8 |
| Regression (backward compat) | 5 |
| Integration (real claude binary) | 4 |
| **Total** | **62** |

---

## Requirements Coverage

| Requirement | Tests Assigned | Covered? |
|---|---|---|
| FR-1: Session pool with per-key rotation | 5 | Yes |
| FR-2: Call() / CallBlocking() streaming | 5 | Yes |
| FR-3: Cache optimization flags | 4 | Yes |
| FR-4: Auth / ErrClaudeNotFound | 3 | Yes |
| FR-5: Replace SpawnReviewSession | 4 | Yes |
| FR-6: RunOneShot enhancement | 5 | Yes |
| FR-7: New background AI features | 8 | Yes |
| FR-8: RunHeadlessCall RPC | 6 | Yes |
| NFR-1: Concurrency (serialized / parallel) | 2 | Yes |
| NFR-2: Resource limits (concurrencySem) | 1 | Yes |
| NFR-4: FakeRunner / goleak | 2 | Yes |
| NFR-5: Backward compatibility | 2 | Yes |

**Coverage fraction: 8/8 functional requirements (100%), 4/4 relevant NFRs (100%)**
