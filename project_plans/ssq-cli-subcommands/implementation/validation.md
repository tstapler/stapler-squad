# Validation Plan: ssq-cli-subcommands

**Date**: 2026-10-18
**Status**: Pre-implementation (tests designed before code)

---

## Happy Path Scenario

A CI script calls `ssq create-session --title "fix-auth" --path ~/code/myrepo --wait --json`, receives a JSON object with `"status": "SESSION_STATUS_ACTIVE"` on stdout, zero exit code, and then pipes `ssq get-diff <session-id>` output to a patch tool.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldPrintSessionID_WhenServerRespondsSuccess` | Unit | Happy: RPC succeeds, stdout has `session-id:` line |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldExitNonZero_WhenServerUnavailable` | Unit | Error: server returns `CodeUnavailable` |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldPopulateAllFlags_WhenAllFlagsProvided` | Unit | Happy: verifies branch/program/category/prompt/auto-yes/one-shot forwarded to RPC |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldErrorOnMissingTitle_WhenTitleFlagOmitted` | Unit | Error: cobra required-flag validation |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldErrorOnMissingPath_WhenPathFlagOmitted` | Unit | Error: cobra required-flag validation |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldEmitValidJSON_WhenJSONFlagSet` | Unit | Happy: stdout is valid JSON with string enum `SESSION_STATUS_CREATING` |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldExitZeroAfterPolling_WhenWaitFlagAndSessionBecomesActive` | Integration | `--wait`: mock server returns CREATING then ACTIVE; assert exit 0 |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldExitNonZero_WhenWaitFlagAndSessionFails` | Integration | `--wait`: mock server returns FAILED terminal state |
| AC-1 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldExitNonZero_WhenWaitFlagAndTimeoutExpires` | Integration | `--wait --wait-timeout 50ms`: session stays CREATING; assert timeout error on stderr |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldPrintTabTable_WhenSessionsExist` | Unit | Happy: two sessions printed in tabwriter columns |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldPrintNoSessionsFound_WhenListIsEmpty` | Unit | Happy (empty): stdout is `"No sessions found."` |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldForwardStatusFilter_WhenStatusFlagSet` | Unit | Happy: `--status active` sets `SESSION_STATUS_ACTIVE` on the RPC request |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldEmitJSONArray_WhenJSONFlagSet` | Unit | Happy: stdout is valid JSON array; enum names are strings not integers |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldExitNonZero_WhenServerUnavailable` | Unit | Error: server returns `CodeUnavailable` |
| AC-2 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldForwardSearchQuery_WhenSearchFlagSet` | Integration | `--search "auth"` is forwarded verbatim in `ListSessionsRequest.search_query` |
| AC-3 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldCallUpdateSession_WhenSessionIDAndMessageProvided` | Unit | Happy: RPC called with correct `steer_message`, stdout prints `"ok"` |
| AC-3 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldExitNonZero_WhenServerReturnsFailedPrecondition` | Unit | Error: `CodeFailedPrecondition` verbatim message on stderr |
| AC-3 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldErrorOnMissingMessage_WhenMessageFlagOmitted` | Unit | Error: cobra required-flag validation |
| AC-3 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldErrorOnMissingSessionID_WhenPositionalArgOmitted` | Unit | Error: cobra ExactArgs(1) validation |
| AC-3 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldEmitValidJSON_WhenJSONFlagSet` | Integration | `--json`: stdout is valid JSON `UpdateSessionResponse`; enum names are strings |
| AC-4 | `cmd/commands/get_diff_test.go` | `TestNewGetDiffCmd_ShouldPrintRawDiff_WhenSessionHasChanges` | Unit | Happy: stdout is raw unified diff string |
| AC-4 | `cmd/commands/get_diff_test.go` | `TestNewGetDiffCmd_ShouldPrintNothing_WhenDiffContentIsEmpty` | Unit | Happy (empty diff): stdout empty, exit 0 |
| AC-4 | `cmd/commands/get_diff_test.go` | `TestNewGetDiffCmd_ShouldExitNonZero_WhenSessionNotFound` | Unit | Error: `CodeNotFound` error on stderr |
| AC-4 | `cmd/commands/get_diff_test.go` | `TestNewGetDiffCmd_ShouldEmitValidJSONWithDiffStats_WhenJSONFlagSet` | Integration | `--json`: stdout is valid JSON with `diff_stats.added`, `removed`, `content` |
| AC-5 | `cmd/commands/client_test.go` | `TestResolveServerURL_ShouldReturnFlagValue_WhenFlagIsSet` | Unit | Happy: flag value wins over env and default |
| AC-5 | `cmd/commands/client_test.go` | `TestResolveServerURL_ShouldReturnEnvVar_WhenFlagEmptyAndEnvSet` | Unit | Happy: SSQ_SERVER env wins over default |
| AC-5 | `cmd/commands/client_test.go` | `TestResolveServerURL_ShouldReturnDefault_WhenFlagAndEnvBothEmpty` | Unit | Happy: default is `http://localhost:8543/api` |
| AC-5 | `cmd/commands/client_test.go` | `TestResolveServerURL_ShouldReturnDefault_WhenEnvVarIsEmptyString` | Unit | Edge: empty string env var treated as unset |
| AC-5 | `cmd/commands/get_session_test.go` | `TestNewGetSessionCmd_ShouldUseDefaultPort_WhenNoServerFlagOrEnv` | Integration | Fix for hard-coded 8080: mock on 8543 must succeed without `--server` flag |
| AC-5 | `cmd/commands/get_session_test.go` | `TestNewGetSessionCmd_ShouldUseSSQServerEnv_WhenEnvVarSet` | Integration | `SSQ_SERVER=http://localhost:9999/api`: mock on 9999 must succeed |
| AC-6 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldPrintKeyValuePerLine_WhenNotJSONMode` | Unit | Default output format: each field on its own `key: value` line |
| AC-6 | `cmd/commands/list_sessions_test.go` | `TestNewListSessionsCmd_ShouldWriteErrorsToStderrOnly_WhenRPCFails` | Unit | Errors go to stderr; stdout is empty on failure |
| AC-6 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldWriteErrorToStderrOnly_WhenRPCFails` | Unit | Errors go to stderr; stdout is empty on failure |
| AC-7 | `cmd/commands/create_session_test.go` | `TestNewCreateSessionCmd_ShouldListRequiredFlags_WhenHelpRequested` | Unit | `--help` output contains `--title`, `--path`, `--server` |
| AC-7 | `cmd/commands/steer_test.go` | `TestNewSteerCmd_ShouldDocumentAutonomousMode_WhenHelpRequested` | Unit | Long description contains "autonomous mode" |

---

## Test Stack

- **Unit**: Go `testing` package + `github.com/stretchr/testify/assert` + `github.com/stretchr/testify/require`. Mock the RPC layer with an `httptest.NewServer` serving a hand-written ConnectRPC-compatible JSON handler (no real process needed). Capture stdout/stderr via `bytes.Buffer` passed to `cmd.SetOut` / `cmd.SetErr`.
- **Integration**: Same framework, but the mock server simulates multi-round-trip sequences (e.g. CREATING → ACTIVE polling for `--wait`, multi-call `--search` forwarding). Tests in `*_test.go` files alongside their command files; use `t.Setenv` for `SSQ_SERVER` isolation so parallel tests don't share env state.
- **E2E / UX**: N/A — pure CLI, no browser surface.

### Mock server pattern

Use `net/http/httptest.NewServer` with a `http.HandlerFunc` that speaks the ConnectRPC unary protocol (POST + `Content-Type: application/json`). Return canned proto-marshalled responses. This avoids spawning a real `stapler-squad` process in unit/integration tests and keeps them fast and deterministic.

```go
// Example skeleton (in test file)
func newMockSessionServer(t *testing.T, handler http.HandlerFunc) string {
    t.Helper()
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return srv.URL
}
```

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (`cmd/commands/`) | `go test ./cmd/commands/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥ 80% line coverage |
| Go (`cmd/commands/`) statement coverage report | `go tool cover -html=coverage.out` | Visual audit — all branches of `ResolveServerURL`, `FormatConnectError`, `--wait` polling loop |

Run coverage after `make build` (required to regenerate protos and ent before tests).

---

## Test File Index

```
cmd/commands/
  client_test.go           # ResolveServerURL, FormatConnectError, MarshalProtoJSON
  get_session_test.go      # NewGetSessionCmd — port-8543 fix, SSQ_SERVER env
  create_session_test.go   # NewCreateSessionCmd — all AC-1 + AC-6 output contract
  list_sessions_test.go    # NewListSessionsCmd — all AC-2 + AC-6 output contract
  steer_test.go            # NewSteerCmd — all AC-3 + AC-6 output contract
  get_diff_test.go         # NewGetDiffCmd — all AC-4
```

---

## Key Test Design Notes

### `ResolveServerURL` — environment isolation

All `ResolveServerURL` tests must use `t.Setenv("SSQ_SERVER", ...)` (or `os.Unsetenv` within `t.Cleanup`) to avoid leaking env state across parallel tests. Never call `os.Setenv` directly in tests without cleanup.

### `--wait` polling — time control

`--wait` tests pass a very short `--wait-timeout` (e.g., `50ms`) and configure the mock server to sequence through status values using an atomic counter. This keeps tests fast without `time.Sleep` (per the `deterministic-fast-tests` project skill).

### `--json` output — enum string names

All JSON output tests assert that enum fields appear as string names (e.g. `"SESSION_STATUS_ACTIVE"`) not integers, confirming `MarshalProtoJSON` (via `protojson`) is used rather than `encoding/json`.

### Error path — stderr vs stdout separation

Error path tests use separate `bytes.Buffer` instances for `cmd.SetOut` and `cmd.SetErr`. Assert stdout is empty and stderr contains the expected message. This validates the AC-6 contract that errors never pollute stdout.

### Help text tests

Help output tests execute `cmd.Execute()` with `args = []string{"--help"}` and assert on the output string. No mock server needed — cobra handles `--help` before `RunE` is called.

---

## Test Count Summary

| Type | Count |
|---|---|
| Unit | 28 |
| Integration | 7 |
| E2E / UX | 0 |
| **Total** | **35** |

## Requirements Coverage

| Requirement | Tests designed | Coverage |
|---|---|---|
| AC-1 `create-session` | 9 | Full |
| AC-2 `list-sessions` | 6 | Full |
| AC-3 `steer` | 5 | Full |
| AC-4 `get-diff` | 4 | Full |
| AC-5 `--server` / `SSQ_SERVER` | 6 | Full |
| AC-6 Script-friendly output | 4 (shared across command tests) | Full |
| AC-7 `--help` documentation | 2 | Partial (spot-checks; full coverage via human review of README) |

**Requirements coverage: 7 / 7 (100%)**
