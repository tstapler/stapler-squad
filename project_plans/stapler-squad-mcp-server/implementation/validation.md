# Validation Plan: Stapler Squad MCP Server

**Status**: Draft | **Phase**: 4 — Validation
**Created**: 2026-04-18
**Input**: `docs/tasks/stapler-squad-mcp-server.md`

---

## Coverage Map

Every requirement from the feature plan is mapped to at least one test below. Requirements without a test are marked `[UNCOVERED]`.

| Requirement | Test IDs |
|---|---|
| `--mcp` starts and speaks MCP stdio protocol | U-1.1, I-1.1 |
| Log output does not pollute stdout in MCP mode | U-1.2, I-1.2 |
| `list_sessions` returns correct data with cursor pagination | U-1.3, U-1.4, I-1.3 |
| `get_session` returns `SESSION_NOT_FOUND` for unknown ID | U-1.5 |
| `search_sessions` matches by query and tag | U-1.6, U-1.7 |
| `SessionSummary` fields complete | U-1.8 |
| `create_session` creates tmux + worktree, returns session ID | I-2.1 |
| `create_session` enforces 30s startup timeout | U-2.1 |
| `create_session` validates path (absolute, no `..`) | U-2.2 |
| `create_session` rate limited at 3/minute | U-2.3 |
| `stop_session` without `confirm: true` returns `CONFIRMATION_REQUIRED` | U-2.4 |
| `stop_session` cleanup failure returns partial success | U-2.5 |
| `pause_session` / `resume_session` state transitions | U-2.6, I-2.2 |
| `update_session` updates metadata without affecting state | U-2.7 |
| `InjectMCPConfig` idempotent merge into settings.local.json | U-3.1, U-3.2, U-3.3, U-3.4, U-3.5 |
| `inject_mcp: false` leaves settings untouched | U-3.6 |
| `InjectHooksConfig` merges all 5 hook types | U-3.7, U-3.8 |
| `permission_approval` always injected regardless of hooks param | U-3.9 |
| `POST /api/hooks/stop` updates `last_idle_at` | I-3.1 |
| `read_session_output` strips ANSI by default | U-4.1, U-4.2, U-4.3 |
| `read_session_output` enforces 200-line / 10KB cap | U-4.4, U-4.5 |
| `read_session_output` returns `truncated: bool` + `total_lines` | U-4.4, U-4.5 |
| `read_session_output` handles partial escape sequences | U-4.3 |
| `run_command` polls until output stabilises, returns captured output | U-4.6, I-4.1 |
| `run_command` returns `timed_out: true` on timeout | U-4.7 |
| `write_to_session` rate limited 1/sec per session | U-4.8 |
| `write_to_session` enforces 4096-byte input cap | U-4.9 |
| `write_to_session` PTY write wrapped in 5s timeout | U-4.10 |
| `write_to_session` tool description contains "unfiltered" | U-4.11 |
| `send_control` sends correct byte for each key | U-4.12 |
| `wait_for_output` matches pattern before timeout | U-4.13, I-4.2 |
| `wait_for_output` returns `WAIT_TIMEOUT` on expiry | U-4.14 |
| `get_session_diff` returns unified diff + stats + truncation | U-5.1, U-5.2 |
| `list_session_branches` returns current + all branches | U-5.3 |
| All 15 tools registered in MCP tool list | I-1.4 |
| Rate limiter is goroutine-safe | U-2.3, U-4.8 (run with `-race`) |
| All error codes are machine-readable strings | U-1.9 |

---

## Test Pyramid

```
         E2E (3)
        /       \
    Integration (12)
   /               \
        Unit (45+)
```

---

## Unit Tests

All unit tests live in `_test.go` files colocated with the package under test. Run with `go test ./server/mcp/... ./server/services/...`. All must pass with `go test -race`.

### Package: `server/mcp`

#### U-1.1 — MCP `initialize` response shape
**File**: `server/mcp/server_test.go`
**Method**: `TestInitializeResponse`
```
Given: RunServer() called with stub InstanceStore
When:  MCP initialize message sent over stdin pipe
Then:  Response is valid JSON-RPC 2.0 with protocolVersion field
       Response arrives on stdout (not stderr)
```

#### U-1.2 — Log isolation: no stdout writes in MCP mode
**File**: `server/mcp/logging_test.go`
**Method**: `TestMCPLoggerStderrOnly`
```
Given: InitMCPLogging() called
When:  log.InfoLog.Printf, log.DebugLog.Printf, log.WarningLog.Printf called
Then:  Nothing written to captured stdout buffer
       Content written to captured stderr buffer
```

#### U-1.3 — `list_sessions` returns page up to limit
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestListSessionsDefaultLimit`
```
Given: InstanceStore with 25 sessions
When:  list_sessions called with no params
Then:  Response contains ≤10 sessions (default limit)
       next_cursor is non-null
       total_count = 25
```

#### U-1.4 — `list_sessions` cursor pagination is stable
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestListSessionsCursorPagination`
```
Given: InstanceStore with 25 sessions
When:  Page 1: list_sessions limit=10 → cursor C1
       Page 2: list_sessions limit=10 cursor=C1 → cursor C2
       Page 3: list_sessions limit=10 cursor=C2
Then:  All 25 sessions returned across 3 pages with no duplicates
       Page 3 next_cursor is null
```

#### U-1.5 — `get_session` unknown ID returns error
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestGetSessionNotFound`
```
Given: Empty InstanceStore
When:  get_session called with session_id="nonexistent"
Then:  success=false, error.code="SESSION_NOT_FOUND"
       error.remediation is non-empty
```

#### U-1.6 — `search_sessions` matches by title substring
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestSearchSessionsByTitle`
```
Given: Sessions titled ["auth-service", "auth-tests", "payment-api"]
When:  search_sessions query="auth"
Then:  Returns exactly 2 sessions; "payment-api" not in result
```

#### U-1.7 — `search_sessions` filters by tag
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestSearchSessionsByTag`
```
Given: Sessions with tags [["frontend"], ["backend"], ["frontend", "urgent"]]
When:  search_sessions query="" tag_filter=["frontend"]
Then:  Returns 2 sessions; backend session excluded
```

#### U-1.8 — `SessionSummary` contains all required fields
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestSessionSummaryFields`
```
Given: Session with all fields populated
When:  list_sessions called
Then:  Each entry has: id, title, status, tags, branch, path, created_at, last_activity_at
       No terminal output present in summary
```

#### U-1.9 — All error codes are machine-readable strings
**File**: `server/mcp/types_test.go`
**Method**: `TestErrorCodeFormat`
```
Given: All defined MCPError constants
Then:  Each code matches regex ^[A-Z_]+$ (uppercase snake case)
       No spaces or special chars
```

#### U-2.1 — `create_session` startup timeout
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestCreateSessionStartupTimeout`
```
Given: SessionService.CreateSession that never transitions to Running/Ready
When:  create_session called (30s timeout)
Then:  Returns SESSION_STARTUP_TIMEOUT after ≤31 seconds
       Partial session_id present in response for follow-up polling
```

#### U-2.2 — `create_session` path validation
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestCreateSessionPathValidation`
Boundary cases:
```
"/valid/absolute/path"           → passes validation
"relative/path"                  → INVALID_PATH error
"/path/../traversal"             → INVALID_PATH error
""                               → INVALID_PATH error
"/path/does/not/exist"           → INVALID_PATH error (directory must exist)
```

#### U-2.3 — `create_session` rate limit: 3 per minute (goroutine-safe)
**File**: `server/mcp/rate_limiter_test.go`
**Method**: `TestCreateSessionRateLimit`
```
Given: Rate limiter initialized
When:  3 create_session calls succeed within 60s
       4th call attempted within same 60s window
Then:  4th call returns RATE_LIMITED error
Run with: go test -race (verify no data races on concurrent access)
```

#### U-2.4 — `stop_session` without `confirm: true`
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestStopSessionRequiresConfirm`
```
Given: Running session
When:  stop_session called without confirm param
       stop_session called with confirm=false
Then:  Both return CONFIRMATION_REQUIRED
       Error message mentions "tmux process" and "git worktree"
```

#### U-2.5 — `stop_session` cleanup failure returns partial success
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestStopSessionCleanupFailure`
```
Given: SessionService that stops tmux but CleanupWorktree returns error
When:  stop_session called with confirm=true
Then:  success=true (session stopped)
       cleanup_error is non-empty
       cleanup_error_code="WORKTREE_CLEANUP_FAILED"
       cleanup_remediation contains "git worktree remove --force"
```

#### U-2.6 — `pause_session` / `resume_session` state transitions
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestPauseResumeCycle`
```
Running → pause_session → Paused    (success=true, new_status="paused")
Paused  → resume_session → Running  (success=true, new_status="running")
Paused  → pause_session             → SESSION_ALREADY_PAUSED
Running → resume_session            → SESSION_NOT_PAUSED (or INVALID_STATUS_TRANSITION)
```

#### U-2.7 — `update_session` metadata update does not change status
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestUpdateSessionMetadata`
```
Given: Running session
When:  update_session called with title="new-title", tags=["a","b"]
Then:  Session status unchanged (still Running)
       title and tags updated in storage
       Response includes new session state
```

### Package: `server/services` (injection)

#### U-3.1 — `InjectMCPConfig`: file not existing creates it
**File**: `server/services/mcp_injector_test.go`
**Method**: `TestInjectMCPConfigCreatesFile`
```
Given: Temp dir with no .claude/settings.local.json
When:  InjectMCPConfig(dir, "/bin/stapler-squad") called
Then:  File created with mcpServers.stapler-squad entry
       command="/bin/stapler-squad", args=["--mcp"], type="stdio"
```

#### U-3.2 — `InjectMCPConfig`: merges without overwriting existing keys
**File**: `server/services/mcp_injector_test.go`
**Method**: `TestInjectMCPConfigMerges`
```
Given: settings.local.json with {"hooks": {"PermissionRequest": [...]}}
When:  InjectMCPConfig called
Then:  hooks section preserved exactly
       mcpServers.stapler-squad added
```

#### U-3.3 — `InjectMCPConfig`: idempotent when entry already present
**File**: `server/services/mcp_injector_test.go`
**Method**: `TestInjectMCPConfigIdempotent`
```
Given: settings.local.json already has mcpServers.stapler-squad pointing to same binary
When:  InjectMCPConfig called again
Then:  File content unchanged (byte-for-byte or semantically equivalent)
       No error returned
```

#### U-3.4 — `InjectMCPConfig`: updates stale binary path
**File**: `server/services/mcp_injector_test.go`
**Method**: `TestInjectMCPConfigUpdatesPath`
```
Given: settings.local.json has mcpServers.stapler-squad pointing to "/old/path"
When:  InjectMCPConfig called with binaryPath="/new/path"
Then:  command field updated to "/new/path"
```

#### U-3.5 — `InjectMCPConfig`: handles malformed JSON (repair)
**File**: `server/services/mcp_injector_test.go`
**Method**: `TestInjectMCPConfigMalformedJSON`
```
Given: settings.local.json with invalid JSON: `{"hooks": {`  (truncated)
When:  InjectMCPConfig called
Then:  No panic; file repaired or reset to minimal valid config
       mcpServers.stapler-squad present in resulting file
```

#### U-3.6 — `create_session` with `inject_mcp: false` does not write settings
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestCreateSessionNoMCPInjection`
```
Given: Temp worktree dir
When:  create_session called with inject_mcp=false
Then:  .claude/settings.local.json not created (or not modified if pre-existing)
```

#### U-3.7 — `InjectHooksConfig`: injects all 5 hook types correctly
**File**: `server/services/hook_injector_test.go`
**Method**: `TestInjectHooksConfigAllTypes`
```
Given: Empty settings.local.json
When:  InjectHooksConfig(dir, "my-session", AllHookNames) called
Then:  hooks.Notification contains curl command for /api/hooks/permission-request
       hooks.Stop contains curl command for /api/hooks/stop
       hooks.PreToolUse contains curl command for /api/hooks/pre-tool-use
       hooks.PostToolUse contains curl command for /api/hooks/post-tool-use
       hooks.UserPromptSubmit contains curl command for /api/hooks/prompt-submit
       Each command contains X-CS-Session-ID header with "my-session"
```

#### U-3.8 — `InjectHooksConfig`: merges with existing user-defined hooks
**File**: `server/services/hook_injector_test.go`
**Method**: `TestInjectHooksConfigPreservesUserHooks`
```
Given: settings.local.json with user-defined PreToolUse hook for linting
When:  InjectHooksConfig called with HookPreToolLogging
Then:  User's linting hook preserved
       Stapler Squad hook prepended to PreToolUse list
       No duplicate entries
```

#### U-3.9 — `permission_approval` always injected regardless of hooks param
**File**: `server/services/hook_injector_test.go`
**Method**: `TestPermissionApprovalAlwaysInjected`
```
Given: Empty settings.local.json
When:  InjectHooksConfig called with hooks=[] (empty list)
Then:  hooks.Notification (PermissionRequest) entry present
       No other hook types present
```

### Package: `server/mcp` (terminal)

#### U-4.1 — `read_session_output`: ANSI stripped by default
**File**: `server/mcp/ansi_test.go`
**Method**: `TestANSIStripDefault`
```
Given: Scrollback containing "\x1b[32mgreen text\x1b[0m"
When:  read_session_output called (strip_ansi default true)
Then:  output == "green text"
       No escape sequences in result
```

#### U-4.2 — `read_session_output`: raw mode preserves ANSI
**File**: `server/mcp/ansi_test.go`
**Method**: `TestANSIPreservedWhenRaw`
```
Given: Scrollback with ANSI escapes
When:  read_session_output called with strip_ansi=false
Then:  Escape sequences present in output
```

#### U-4.3 — `read_session_output`: partial escape sequence at boundary
**File**: `server/mcp/ansi_test.go`
**Method**: `TestPartialEscapeSequenceAtBoundary`
Input space:
```
"\x1b[" at end of last line (no terminator)   → stripped, no crash
"\x1b" alone                                   → stripped, no crash
Invalid UTF-8 bytes (\xff, \xfe)               → replaced with U+FFFD
Valid UTF-8 mixed with escapes                  → only escapes stripped
```

#### U-4.4 — `read_session_output`: 200-line cap sets `truncated: true`
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestReadOutputLineCap`
```
Given: Scrollback with 250 lines
When:  read_session_output called with lines=200
Then:  output has exactly 200 lines
       truncated=true
       total_lines=250
       Output includes "[... 50 lines omitted...]" marker
```

#### U-4.5 — `read_session_output`: 10KB byte cap sets `truncated: true`
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestReadOutputByteCap`
```
Given: Scrollback with 50 lines each 300 bytes = 15KB total
When:  read_session_output called with lines=50
Then:  output byte length ≤ 10240
       truncated=true
       total_lines=50
```

#### U-4.6 — `run_command`: polls until sequence stabilises
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestRunCommandStabilisation`
```
Given: Mock scrollback that changes sequence 3 times then stabilises
When:  run_command called with command="echo hello"
Then:  output contains content from final stable sequence
       timed_out=false
```

#### U-4.7 — `run_command`: returns `timed_out: true` on timeout
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestRunCommandTimeout`
```
Given: Mock scrollback whose sequence never stabilises
When:  run_command called with timeout_seconds=2
Then:  Returns within ~2s
       timed_out=true
       output contains last-seen content
```

#### U-4.8 — `write_to_session`: rate limit 1/sec per session (goroutine-safe)
**File**: `server/mcp/rate_limiter_test.go`
**Method**: `TestWriteRateLimitPerSession`
```
Given: Two different session IDs (S1, S2)
When:  S1: call 1 at t=0 → success
       S1: call 2 at t=0.5s → RATE_LIMITED
       S2: call 1 at t=0.5s → success (different session, different bucket)
       S1: call 3 at t=1.1s → success (window elapsed)
Run with: go test -race
```

#### U-4.9 — `write_to_session`: 4096-byte input cap
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestWriteInputLengthCap`
```
Given: Running session
When:  write_to_session called with input = string of 4097 bytes
Then:  Returns INPUT_TOO_LONG error
       No bytes written to PTY
```

#### U-4.10 — `write_to_session`: PTY write timeout prevents deadlock
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestWritePTYTimeout`
```
Given: Mock SendKeys that blocks indefinitely (channel read with no writer)
When:  write_to_session called
Then:  Returns PTY_WRITE_TIMEOUT error within ≤6 seconds
       Does not hang
```

#### U-4.11 — `write_to_session` tool description contains "unfiltered"
**File**: `server/mcp/server_test.go`
**Method**: `TestWriteToSessionDescriptionContainsUnfiltered`
```
Given: MCP server with all tools registered
When:  tools/list response parsed
Then:  write_to_session tool description contains the word "unfiltered"
```
*This test acts as a regression guard: if someone rewrites the description and removes the warning, the test fails.*

#### U-4.12 — `send_control`: correct byte per key
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestSendControlBytes`
```
key="C" → \x03 written to PTY, sent="^C" in response
key="D" → \x04 written to PTY, sent="^D" in response
key="Z" → \x1a written to PTY, sent="^Z" in response
key="L" → \x0c written to PTY, sent="^L" in response
key="X" → INVALID_KEY error (not in enum)
```

#### U-4.13 — `wait_for_output`: matches pattern before timeout
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestWaitForOutputMatch`
```
Given: Scrollback that adds "$ " prompt after 2 seconds
When:  wait_for_output called with pattern="\\$ ", timeout_seconds=10
Then:  matched=true
       matched_line contains "$ "
       Returns in ~2 seconds (not 10)
```

#### U-4.14 — `wait_for_output`: returns `WAIT_TIMEOUT` on expiry
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestWaitForOutputTimeout`
```
Given: Scrollback that never contains "DONE"
When:  wait_for_output called with pattern="DONE", timeout_seconds=2
Then:  matched=false
       error.code="WAIT_TIMEOUT" (not a fatal error level)
       output contains last-seen scrollback content
       Returns in ~2 seconds
```

### Package: `server/mcp` (VCS)

#### U-5.1 — `get_session_diff`: returns diff + stats
**File**: `server/mcp/tools_vcs_test.go`
**Method**: `TestGetSessionDiff`
```
Given: Session with worktree containing 1 modified file (10 lines added)
When:  get_session_diff called
Then:  diff is non-empty unified diff string
       stats.files_changed=1, stats.insertions=10, stats.deletions=0
       truncated=false
```

#### U-5.2 — `get_session_diff`: truncation at max_bytes
**File**: `server/mcp/tools_vcs_test.go`
**Method**: `TestGetSessionDiffTruncation`
```
Given: Session worktree with diff > 100 bytes
When:  get_session_diff called with max_bytes=100
Then:  diff length ≤ 100 bytes
       truncated=true
```

#### U-5.3 — `list_session_branches`: lists branches
**File**: `server/mcp/tools_vcs_test.go`
**Method**: `TestListSessionBranches`
```
Given: Session worktree on branch "feature-x" with branches ["main", "feature-x", "old-branch"]
When:  list_session_branches called
Then:  current_branch="feature-x"
       branches contains all 3 names
       has_upstream reflects actual remote tracking state
```

---

## Integration Tests

Run with `go test -tags=integration ./server/mcp/... -timeout=120s`. Require a live stapler-squad binary and tmux available.

#### I-1.1 — MCP stdio handshake via subprocess
**File**: `server/mcp/server_test.go`
**Method**: `TestMCPHandshakeSubprocess`
**Build tag**: `//go:build integration`
```
Steps:
1. go build -o /tmp/test-stapler-squad .
2. Spawn subprocess: /tmp/test-stapler-squad --mcp
3. Write initialize JSON-RPC message to stdin
4. Write tools/list message to stdin
5. Close stdin

Assert:
- initialize response arrives on stdout (valid JSON-RPC 2.0)
- tools/list response lists ≥15 tool names
- Nothing on stdout that is not valid JSON-RPC
- Process exits cleanly after stdin close
Timeout: 10 seconds
```

#### I-1.2 — No log output on stdout during MCP mode
**File**: `server/mcp/server_test.go`
**Method**: `TestMCPNoLogPollution`
**Build tag**: `//go:build integration`
```
Steps:
1. Spawn subprocess with --mcp
2. Capture both stdout and stderr
3. Send initialize + tools/list
4. Read all stdout output until process exits

Assert:
- Every line of stdout parses as valid JSON
- stderr may contain log lines (acceptable)
- No non-JSON lines on stdout
```

#### I-1.3 — `list_sessions` returns real sessions
**File**: `server/mcp/server_test.go`
**Method**: `TestListSessionsLive`
**Build tag**: `//go:build integration`
**Prerequisite**: stapler-squad running with ≥1 session
```
Steps:
1. Spawn --mcp subprocess connected to live stapler-squad state
2. Call list_sessions via stdin
Assert:
- sessions array is non-empty
- Each session has all SessionSummary fields populated
- next_cursor is null (if ≤10 sessions) or string
```

#### I-1.4 — All 15 tools registered
**File**: `server/mcp/server_test.go`
**Method**: `TestAllToolsRegistered`
**Build tag**: `//go:build integration`
```
Steps:
1. Spawn --mcp subprocess
2. Call tools/list
Assert:
- Response contains exactly these 15 tool names:
  list_sessions, get_session, search_sessions,
  create_session, pause_session, resume_session,
  stop_session, update_session,
  run_command, read_session_output, write_to_session,
  send_control, wait_for_output,
  get_session_diff, list_session_branches
```

#### I-2.1 — `create_session` end-to-end lifecycle
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestCreateSessionEndToEnd`
**Build tag**: `//go:build integration`
**Cleanup**: defer stop_session(confirm=true) at test start
```
Steps:
1. create_session(title="mcp-test-XXXX", path=<real git repo>, inject_mcp=false, hooks=[])
2. Assert: success=true, session_id non-empty
3. Call list_sessions, assert new session appears
4. Call get_session(session_id), assert status is Running or Ready
5. stop_session(session_id, confirm=true)
6. Assert: session no longer in list_sessions
Timeout: 60 seconds
```

#### I-2.2 — `pause_session` / `resume_session` end-to-end
**File**: `server/mcp/tools_lifecycle_test.go`
**Method**: `TestPauseResumeEndToEnd`
**Build tag**: `//go:build integration`
```
Steps:
1. Create session (cleanup deferred)
2. pause_session → assert status=paused in get_session response
3. resume_session → assert status=running in get_session response
Timeout: 60 seconds
```

#### I-3.1 — `POST /api/hooks/stop` updates `last_idle_at`
**File**: `server/services/hook_receivers_test.go`
**Method**: `TestStopHookUpdatesIdleAt`
**Build tag**: `//go:build integration`
```
Steps:
1. Create session
2. POST http://localhost:8543/api/hooks/stop with X-CS-Session-ID header
3. GET session via API

Assert:
- last_idle_at is non-null and within last 5 seconds
- HTTP 200 returned with proceed decision JSON
Timeout: 10 seconds
```

#### I-4.1 — `run_command` executes and returns output
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestRunCommandEndToEnd`
**Build tag**: `//go:build integration`
```
Steps:
1. Create session (shell, not claude)
2. run_command(session_id, command="echo hello-mcp-test", timeout_seconds=10)

Assert:
- output contains "hello-mcp-test"
- timed_out=false
- truncated=false
Timeout: 30 seconds
```

#### I-4.2 — `wait_for_output` matches real terminal output
**File**: `server/mcp/tools_terminal_test.go`
**Method**: `TestWaitForOutputEndToEnd`
**Build tag**: `//go:build integration`
```
Steps:
1. Create session
2. write_to_session(command="echo wait-marker")
3. wait_for_output(pattern="wait-marker", timeout_seconds=15)

Assert:
- matched=true
- matched_line contains "wait-marker"
Timeout: 30 seconds
```

---

## End-to-End Tests

Manual verification steps for Checkpoint C. Not automated in v1.

#### E-1 — Three-tool full workflow
```
Prompt to Claude: "Create a new Stapler Squad session called 'e2e-test' in
<a real git repo path>, run 'echo workflow-complete', read the output, then
stop the session."

Assert (observe Claude's tool calls):
1. create_session called with correct path → session_id returned
2. run_command called with "echo workflow-complete" → output contains string
3. stop_session called with confirm=true → success
Total tool calls: 3 (not 5 — verifies run_command composite value)
```

#### E-2 — `send_control` interrupt
```
1. Create session
2. run_command("sleep 60", timeout_seconds=2) → timed_out=true
3. send_control(key="C")
4. read_session_output → confirm "sleep" process no longer in output

Assert: sleep process interrupted, prompt returns
```

#### E-3 — Hook injection visible in settings file
```
1. Create session with default hooks
2. Cat the session's .claude/settings.local.json
Assert:
- mcpServers.stapler-squad present with correct binary path
- hooks.Notification contains /api/hooks/permission-request
- hooks.Stop contains /api/hooks/stop
- No other hook types present (since only defaults requested)
```

---

## Property-Based Tests

#### P-1 — ANSI stripper never crashes on arbitrary input
**File**: `server/mcp/ansi_test.go`
**Method**: `TestANSIStripNeverPanics`
```
Property: for any []byte input, StripANSI(input) never panics
          and result is valid UTF-8
Strategy: rapid.SliceOf(rapid.Byte()) — 1000 iterations
```

#### P-2 — Cursor pagination covers all sessions without overlap
**File**: `server/mcp/tools_discovery_test.go`
**Method**: `TestCursorPaginationComplete`
```
Property: for N sessions (1 ≤ N ≤ 200, limit random 1–20),
          paging through all cursors returns exactly N unique session IDs
Strategy: rapid.Int() for N and limit — 500 iterations
```

#### P-3 — `InjectHooksConfig` never corrupts existing JSON
**File**: `server/services/hook_injector_test.go`
**Method**: `TestInjectHooksNeverCorruptsJSON`
```
Property: for any valid JSON object as starting content,
          InjectHooksConfig produces valid JSON output
          that contains all original top-level keys
Strategy: rapid.Map(rapid.String(), rapid.String()) as base JSON — 500 iterations
```

---

## Mutation Testing Targets

Run with `go-mutesting` focused on these high-risk functions. Target >80% mutation kill rate.

| Function | File | Why high-risk |
|---|---|---|
| `StripANSI` | `server/mcp/ansi.go` | Security-adjacent; wrong stripping = data leak to LLM |
| `applyLineByteCap` | `server/mcp/tools_terminal.go` | Truncation logic; off-by-one → LLM sees too much/too little |
| `InjectHooksConfig` merge logic | `server/services/hook_injector.go` | Wrong merge = lost user hooks or missing safety hooks |
| Rate limiter `Allow()` | `server/mcp/rate_limiter.go` | Wrong logic = injection amplification |
| Path validation in `create_session` | `server/mcp/tools_lifecycle.go` | Wrong = path traversal |

---

## Test Coverage Requirements

| Package | Minimum line coverage |
|---|---|
| `server/mcp/` | 80% |
| `server/services/mcp_injector.go` | 90% (pure file logic, easy to cover) |
| `server/services/hook_injector.go` | 90% |
| `server/services/hook_receivers.go` | 75% |
| Overall new code | 80% |

Run: `go test -coverprofile=coverage.out ./server/mcp/... ./server/services/... && go tool cover -func=coverage.out`

---

## Risk-Based Prioritization

Implement tests in this order — highest risk first:

1. **U-4.11** — `write_to_session` description contains "unfiltered" (CRITICAL; guards injection surface documentation)
2. **U-4.10** — PTY write timeout (CRITICAL; prevents hang)
3. **U-4.4, U-4.5** — output truncation + `truncated` metadata (HIGH; silent data loss)
4. **U-3.1–U-3.5** — `InjectMCPConfig` correctness (HIGH; broken injection = feature doesn't work)
5. **U-3.7–U-3.9** — `InjectHooksConfig` correctness (HIGH; same)
6. **U-1.2** — log isolation (HIGH; corrupted stdio breaks all MCP)
7. **U-2.4** — `stop_session` confirm guard (MEDIUM; accidental destruction)
8. **U-2.3, U-4.8** — rate limiters with `-race` (MEDIUM; race = undefined behaviour)
9. **U-2.2** — path traversal validation (MEDIUM; security boundary)
10. All remaining unit tests
11. Integration tests (require live environment)
12. Property-based tests
13. E2E manual verification
