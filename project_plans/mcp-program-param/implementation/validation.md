# Validation Plan: mcp-program-param

**Date**: 2026-09-24

## Happy Path Scenario

Given a running server whose `*services.SessionService` has a custom `ProgramConfig{ID: "claude-250k-proxy"}` registered via `UpsertProgramConfig`, when an agent calls the `create_session` MCP tool with `program="claude-250k-proxy"`, then the tool schema does not reject the value client-side, `CreateSessionRequest.Program == "claude-250k-proxy"` reaches the backend unchanged, and the tool result reports `success == true`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: `create_session` accepts custom program | `server/mcp/tools_lifecycle_program_test.go` | `TestCreateSession_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig` | Integration (real `SessionService` + config-file store, envtest-isolated) | Happy path |
| AC1: schema no longer hardcodes 2-value enum | `server/mcp/tools_lifecycle_program_test.go` | `TestRegisterLifecycleTools_should_IncludeAllBuiltinsInProgramEnum_When_SvcIsWired` | Integration (real `svc` wired at registration) | Schema inspection, previously-unreachable built-ins (`bash`, `opencode`) |
| AC1: previously-unreachable built-in also unblocked | `server/mcp/tools_lifecycle_program_test.go` | `TestCreateSession_should_AcceptPreviouslyUnreachableBuiltin_When_ProgramIsBash` | Integration | Happy path variant |
| AC1: shared helper correctness (both tools depend on this) | `server/mcp/tools_program_common_test.go` | `TestProgramSchemaOptions_should_IncludeAllBuiltinsAndCustom_When_SvcHasCustomProgram` | Integration (real `svc`) | Happy path (helper unit under test) |
| AC1/AC2: helper nil-safety (`svc == nil`, the documented `githubHandlers.svc`-may-be-nil case) | `server/mcp/tools_program_common_test.go` | `TestProgramSchemaOptions_should_OmitEnum_When_SvcIsNil` | Unit (pure function, no I/O) | Error/edge path |
| AC2: `create_session_for_pr` accepts custom program, parity | `server/mcp/tools_github_test.go` | `TestCreateSessionForPR_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig` | Integration | Happy path |
| AC3: `claude`/`aider` unaffected — explicit `aider` via `create_session` | `server/mcp/tools_lifecycle_program_test.go` | `TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` | Integration | Regression |
| AC3: `claude`/`aider` unaffected — explicit `aider` via `create_session_for_pr` | `server/mcp/tools_github_test.go` | `TestCreateSessionForPR_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` | Integration | Regression, parity |
| AC3: omitted `program` still defaults to `claude` (`create_session`) | `server/mcp/tools_lifecycle_program_test.go` | **MISSING — recommend add** `TestCreateSession_should_DefaultToClaudeProgram_When_ProgramOmitted` | Integration | Regression (gap — see Findings) |
| AC3: omitted `program` still defaults to `claude` (`create_session_for_pr`) | `server/mcp/tools_github_test.go` | **MISSING — recommend add** `TestCreateSessionForPR_should_DefaultToClaudeProgram_When_ProgramOmitted` | Integration | Regression, parity (gap — see Findings) |
| AC4: discoverability via schema description + dynamic enum | `server/mcp/tools_lifecycle_program_test.go` | `TestRegisterLifecycleTools_should_IncludeAllBuiltinsInProgramEnum_When_SvcIsWired` | Integration | Happy path (same test as AC1's schema row) |
| AC4: non-fatal warning on unrecognized `program` (`create_session`) | `server/mcp/tools_lifecycle_program_test.go` | **MISSING — recommend add** `TestCreateSession_should_SetProgramWarning_When_ProgramNotInKnownList` | Integration | Error/edge path (gap — see Findings) |
| AC4: warning is empty for a recognized-but-previously-unreachable program | `server/mcp/tools_lifecycle_program_test.go` | **MISSING — recommend fold into** `TestCreateSession_should_AcceptPreviouslyUnreachableBuiltin_When_ProgramIsBash` **as an added assertion** (`ProgramWarning == ""`) | Integration | Happy path (gap — see Findings) |
| AC4: non-fatal warning on unrecognized `program` (`create_session_for_pr`, parity) | `server/mcp/tools_github_test.go` | **MISSING — recommend add** `TestCreateSessionForPR_should_SetProgramWarning_When_ProgramNotInKnownList` | Integration | Error/edge path, parity (gap — see Findings) |
| AC4: live discovery beyond the startup-time enum snapshot (`list_programs`) | `server/mcp/tools_programs_test.go` | `TestListPrograms_should_IncludeProgramAddedAfterRegistration_When_CalledLiveAtRuntime` | Integration | Liveness (proves no reliance on Phase 1's registration-time snapshot) |
| AC5: existing suites keep passing + new tests exist | N/A (whole-suite gate) | `go test ./server/mcp/...` (full package) | Integration (suite run) | Regression gate |
| AC5: test infrastructure itself — rate-limiter isolation | `server/mcp/tools_lifecycle_program_test.go` | **MISSING — recommend add** `TestResetCreateSessionLimiterForTest_should_RestoreCapacity_When_BucketExhausted` | Unit (exercises `tokenBucket` directly, no service/config I/O) | Infra self-check (gap — see Findings) |
| AC6: no proto/backend change, MCP-schema-layer scope only | N/A (static check, not a Go test) | `git diff --stat` shows no changes under `proto/` or `config/` | — | Scope gate, run at Phase 6 verify, not a unit/integration test |

## Findings (gaps in the plan's Epic 1.4 task list) — RESOLVED 2026-09-24

All "MISSING — recommend add" rows and gaps below were patched directly into `plan.md`'s Tasks 1.4.1a, 1.4.1b, and 1.4.2a during the sdd:4-validate repair pass: `TestCreateSession_should_SetProgramWarning_When_ProgramNotInKnownList`, `TestCreateSessionForPR_should_SetProgramWarning_When_ProgramNotInKnownList`, `TestCreateSession_should_DefaultToClaudeProgram_When_ProgramOmitted`, `TestCreateSessionForPR_should_DefaultToClaudeProgram_When_ProgramOmitted`, the `ProgramWarning == ""` assertion folded into the existing `bash` test, and `TestResetCreateSessionLimiterForTest_should_RestoreCapacity_When_BucketExhausted`. The mapping table above still shows the original findings for traceability; treat every "MISSING" annotation as closed.



Cross-checking plan.md's Story-level Given/When/Then acceptance criteria (Story 1.2.1, Story 1.3.1) against its actual Epic 1.4 test tasks (1.4.1a/b, 1.4.2a) surfaces two real gaps — the plan's Verification Checklist claims coverage that its own task list doesn't deliver:

1. **`ProgramWarning` has zero test coverage.** Story 1.2.1's AC explicitly specifies two scenarios — an unrecognized `program="clade"` producing a non-empty `ProgramWarning`, and a recognized-but-previously-unreachable `program="bash"` producing an empty one — and Story 1.3.1 specifies the same pair for `create_session_for_pr`. But Task 1.4.1b's three tests (bash/custom/aider) and Task 1.4.2a's two tests (custom/aider) never assert on `ProgramWarning` at all, and no task names a `program="clade"`-style test. The plan's own Verification Checklist row for AC4 cites "Task 1.2.1a/1.3.1a's non-fatal unrecognized-program warning" as satisfying AC4 — but those are implementation tasks, not test tasks; nothing in Epic 1.4 exercises the behavior. This is the single most important gap: the new non-fatal-warning mechanism (the plan's answer to the architecture-review CONCERN about silent typo failures) ships unverified. Recommend adding the three tests listed above before implementation.
2. **Omitted-`program`-defaults-to-`claude` has no dedicated test**, for either tool. Both Story ACs name this scenario explicitly (mapped to AC3 in the Verification Checklist), but no existing test in `server/mcp/*_test.go` exercises `create_session`/`create_session_for_pr` with `program` omitted through the handler (confirmed via repo search — existing fixtures set `Program: "claude"` on pre-built `session.Instance` structs, never on a tool-call request with the field left out). Lower risk than gap 1 (the fallback logic is a 1:1 rename of an existing literal to `defaultProgramID`, not new behavior), but still an explicit AC with no regression pin. Recommend the two tests listed above.

Verified not to be gaps: the `svc == nil` schema case (`TestProgramSchemaOptions_should_OmitEnum_When_SvcIsNil`) and the `list_programs` liveness case (`TestListPrograms_should_IncludeProgramAddedAfterRegistration_When_CalledLiveAtRuntime`) are both concretely named in the plan.

The rate-limiter/config-isolation test infrastructure itself (`resetCreateSessionLimiterForTest`, the `envtest.NewIsolatedStateDir(t)` addition to `newWorktreeGuardHandlers`) has no dedicated test proving the reset helper does what it claims — confirmed via search: neither `server/mcp/` nor `envtest/` has any existing test file for a token bucket or `NewIsolatedStateDir`. The plan's other 6+ new rate-limited tests implicitly depend on this reset working correctly (an incorrect reset would surface as a flaky `ErrRateLimitExceeded` failure in an unrelated test, not a clear signal pointing at the helper) — recommend the one infra self-test listed above.

## UX Acceptance Tests

N/A — no user-facing surface (MCP tool consumed by agents, not humans; no `design/ux.md` exists).

## Test Stack

- **Unit**: Go's `testing` package + `testify/assert`/`require` for assertions. Confirmed via repo search: no mocking framework (`gomock`, `mockery`, `testify/mock`) anywhere in `server/mcp/`. The package convention is real `*services.SessionService` fixtures (`newWorktreeGuardHandlers`, `server/mcp/tools_lifecycle_worktree_guard_test.go:44`) backed by `envtest.NewIsolatedStateDir(t)`-isolated config/session storage — most "unit" tests in this package are therefore integration-shaded (real config-file I/O to a temp dir), with only pure-function tests (e.g. `programSchemaOptions(nil)`) being true, I/O-free units.
- **Integration**: All tests marked Integration above run through the real `services.SessionService` (config-file-backed program registry, session storage) rather than a stub. They are ordinary `go test` targets — no `integration` build tag, no real tmux — and run under both `make test` and `make test-integration` (the latter's non-tmux package sweep at Makefile:694 includes `server/mcp` with `-tags integration`, but since these test files declare no build constraint they compile and run under both invocations identically).
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./server/mcp/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of `programSchemaOptions`, `programIDs`, and the `ProgramWarning` branch in `createSessionWithAwaitTimeout`/`createSessionForPRWithAwaitTimeout` — these are the entire surface of this feature |

- All public service methods: happy path + error paths covered (see Findings above for the two gaps to close before this is true for `ProgramWarning` and the omitted-default case).
- All external integrations: `svc.ListProgramsConfig`/`svc.UpsertProgramConfig` are exercised directly via real `SessionService` fixtures in every Integration-typed test above — no separate mock-vs-integration split needed given this package's no-mocking convention.
- Rate limiter and config-dir test isolation (the infra Epic 1.4 introduces) should itself be covered per the Findings section, since every other new test's reliability depends on it.
