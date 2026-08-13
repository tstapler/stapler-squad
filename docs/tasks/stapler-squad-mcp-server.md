# Implementation Plan: Stapler Squad MCP Server

**Feature**: MCP Server for LLM-driven session orchestration
**Branch**: `stapler-squad-mcp-server`
**Status**: Planning complete — ready for implementation
**ADRs**: ADR-001, ADR-002, ADR-003, ADR-004, ADR-005, ADR-006 (see `project_plans/stapler-squad-mcp-server/decisions/`)

---

## Epic Overview

LLM agents (Claude, etc.) have no programmatic interface to Stapler Squad. This epic adds a Model Context Protocol (MCP) server embedded in the stapler-squad binary, activated by `./stapler-squad --mcp`. When running, it exposes 15 tools across 4 families (Discovery, Lifecycle, Terminal I/O, VCS) that let an LLM create workspaces, delegate tasks, read/write terminal output, and inspect git state — all as single tool calls with no manual steps.

**Architecture**: Embedded Go binary (`--mcp` flag, stdio transport, `mark3labs/mcp-go` SDK). See ADR-001, ADR-002, ADR-003.

**Tool surface**: 15 tools in 4 families. See ADR-004.

### Success Metrics

- An LLM can create a workspace, delegate a task, and read terminal output in a single prompt with no manual steps
- All 4 tool families (15 tools) pass integration tests against a live Stapler Squad session
- `write_to_session` rate limiting enforced; command injection surface documented in tool description
- `read_session_output` always returns `truncated: bool` and `total_lines: int`; ANSI stripping on by default
- `stop_session` requires `confirm: true` or returns an explicit error
- Log output does not pollute MCP stdio channel in `--mcp` mode

---

## Dependency Graph

```
Story 1: MCP Foundation + Discovery Tools
    └── Story 2: Session Lifecycle Tools
            └── Story 3: Terminal I/O + VCS Tools
```

Story 2 depends on Story 1 because it requires the MCP server scaffolding, the `--mcp` flag wiring, and the storage/service access patterns established in Story 1. Story 3 depends on Story 2 because `write_to_session` and `wait_for_output` require understanding of session state established by the lifecycle tools, and the scrollback read path must be validated against a running session.

---

## Story 1: MCP Foundation and Discovery Tools

**Goal**: A working MCP server skeleton with `mark3labs/mcp-go`, wired to the `--mcp` flag, with 3 read-only discovery tools that an LLM can use to inspect existing sessions.

**Value**: Establishes the full MCP call path — from Claude spawning the binary, through tool dispatch, to session data — without any state-mutation risk. All subsequent stories build on this path.

**Acceptance Criteria**:
- `./stapler-squad --mcp` starts without error and speaks MCP stdio protocol
- Claude Code can list it as an MCP server and call `list_sessions`
- `list_sessions`, `get_session`, and `search_sessions` return correct data for existing sessions
- All log output goes to stderr (not stdout) in `--mcp` mode; MCP protocol is not polluted
- Tool result schema includes `success: bool` and `error: { code, message, remediation }` on all tools

### Task 1.1: Add `mark3labs/mcp-go` dependency and `--mcp` flag wiring

**Files (max 5)**:
- `go.mod` — add `github.com/mark3labs/mcp-go`
- `go.sum` — updated by `go mod tidy`
- `main.go` — add `--mcp` flag; when set, call `mcp.RunServer()` instead of starting HTTP listener
- `server/mcp/server.go` — new file: `RunServer()` function that initializes the mcp-go server, registers tools (stubs for now), and starts stdio transport

**INVEST**:
- Independent: no dependency on session logic changes
- Negotiable: stub tool implementations are acceptable; real logic in Task 1.2
- Valuable: establishes the full binary invocation and MCP handshake path
- Estimable: 2-3 hours
- Small: 4 files, well-bounded scope
- Testable: `echo '{"jsonrpc":"2.0","id":1,"method":"initialize",...}' | ./stapler-squad --mcp` returns valid MCP initialize response

**Notes**:
- All logging in `--mcp` mode must use `log.ErrorLog` / `log.WarningLog` directed to stderr, not the default `log.InfoLog` which may write to stdout. Audit all log calls reachable from MCP path.
- `RunServer()` must accept a `session.InstanceStore` and `*services.SessionService` (or equivalent minimal interfaces) injected from `main.go` — do not reach for global state.

---

### Task 1.2: Implement `list_sessions`, `get_session`, `search_sessions`

**Files (max 5)**:
- `server/mcp/tools_discovery.go` — new file: handler implementations for all 3 discovery tools
- `server/mcp/server.go` — register discovery tools (replace stubs)
- `server/mcp/types.go` — new file: shared response types (`MCPResult`, `MCPError`) used by all tool families

**INVEST**:
- Independent: reads from `session.InstanceStore` only; no mutation
- Negotiable: `search_sessions` can use simple substring match in v1; BM25/fuzzy search is optional
- Valuable: delivers the complete read-only tool surface; LLM can immediately explore existing sessions
- Estimable: 3-4 hours
- Small: 3 files, read-only path through existing storage layer
- Testable: unit tests with a mock `InstanceStore` returning fixture sessions; verify field mapping and error paths

**Tool schemas** (required fields — all others optional):

`list_sessions`:
- Input: `status_filter` (optional string enum: `running|paused|ready|loading|needs_approval`), `limit` (optional int, default 10, max 100), `cursor` (optional string: opaque pagination cursor from previous response)
- Output: `{ sessions: [SessionSummary], total_count: int, next_cursor: string|null }`
- Note: default limit is 10 to avoid filling LLM context. For finding a specific session, prefer `search_sessions`.

`get_session`:
- Input: `session_id` (required string)
- Output: `{ session: SessionDetail }` or error `SESSION_NOT_FOUND`

`search_sessions`:
- Input: `query` (required string), `tag_filter` (optional string array), `limit` (optional int, default 10, max 50)
- Output: `{ sessions: [SessionSummary], total_count: int }`
- Note: **prefer this over `list_sessions` when looking for a specific session** — it is faster and returns less context.

**Notes**:
- `SessionSummary` must include: `id`, `title`, `status`, `tags`, `branch`, `path`, `created_at`, `last_activity_at`
- `SessionDetail` extends `SessionSummary` with: `program`, `session_type`, `working_dir`
- Do NOT include terminal output in `get_session` response — that is `read_session_output`'s job
- Cursor pagination: `next_cursor` is null when no further pages exist. Cursor encodes the last-seen session ID + sort key; implement as base64-encoded JSON internally.

---

### Task 1.3: Log isolation and MCP mode integration test

**Files (max 5)**:
- `server/mcp/logging.go` — new file: `MCPLogger` type that wraps all log output to stderr when in MCP mode; `InitMCPLogging()` function called from `main.go` before `RunServer()`
- `server/mcp/server_test.go` — new file: integration test that spawns `./stapler-squad --mcp` as a subprocess, sends MCP `initialize` and `tools/list`, and validates the response

**INVEST**:
- Independent: logging change is isolated; test uses subprocess invocation
- Negotiable: integration test can be skipped in short test runs with a build tag
- Valuable: prevents the critical defect where log lines corrupt the MCP stdio channel
- Estimable: 2-3 hours
- Small: 2 files
- Testable: the integration test itself is the acceptance criterion

**Notes**:
- The subprocess test must build the binary first (`go build .`) then invoke it. Use `t.Helper()` and `exec.Command` with a timeout context.
- Log isolation must cover both `log.InfoLog` and `log.DebugLog` paths. Any log writer that targets `os.Stdout` must be redirected to `os.Stderr` when `--mcp` is active.

---

## Story 2: Session Lifecycle Tools

**Goal**: Add 5 tools that let an LLM create, pause, resume, stop, and update sessions. Establishes the mutation path through the service layer and the `confirm: true` guard pattern.

**Value**: An LLM can now orchestrate the complete session lifecycle — the core use case from the requirements doc ("create a workspace, delegate a task").

**Acceptance Criteria**:
- `create_session` creates a real tmux session + git worktree and returns session ID
- `stop_session` without `confirm: true` returns error code `CONFIRMATION_REQUIRED` with a message explaining what will be destroyed
- `pause_session` and `resume_session` correctly transition session state
- `update_session` can update `title`, `tags`, and `category` without affecting session state
- All state-changing tools return the new session state in the response
- Rate limiting on `create_session`: max 3 new sessions per minute (prevents runaway creation loop)

### Task 2.1: Implement `create_session` and `update_session`

**Files (max 5)**:
- `server/mcp/tools_lifecycle.go` — new file: `create_session` and `update_session` handlers
- `server/mcp/server.go` — register lifecycle tools
- `server/mcp/rate_limiter.go` — new file: per-tool rate limiter using `server/services/rate_limiter.go` patterns; enforces create_session cap

**INVEST**:
- Independent: `create_session` wraps existing `SessionService.CreateSession` RPC logic
- Negotiable: `create_session` can be synchronous in v1 (wait up to 30s for `Ready` or `Running` status); async with polling is a v2 concern
- Valuable: delivers the primary creation workflow
- Estimable: 3-4 hours
- Small: 3 files
- Testable: integration test creates a session, verifies it appears in `list_sessions`, then stops it

**`create_session` input schema**:
- `title` (required string): must be unique
- `path` (required string): repository root path; validated as existing directory before passing to service layer
- `branch` (optional string): creates if missing
- `program` (optional string enum: `claude|aider`; default `claude`)
- `session_type` (optional string enum: `directory|new_worktree`; default `new_worktree`)
- `tags` (optional string array)
- `inject_mcp` (optional bool, default `true`): when true, writes the Stapler Squad MCP server config into the new session's `.claude/settings.local.json`. Set to `false` to leave settings untouched. See ADR-005.
- `hooks` (optional string array, default `["permission_approval", "stop_notification"]`): built-in hook names to inject. Valid values: `permission_approval` (always injected regardless), `stop_notification`, `pre_tool_logging`, `post_tool_logging`, `prompt_submit`. See ADR-006.

**Notes**:
- Path traversal defense: validate `path` is an absolute path and does not contain `..` components before passing to service layer. Return `INVALID_PATH` error if validation fails.
- `create_session` must enforce a 30-second timeout for session startup. If the session does not reach `Running` or `Ready` status within the timeout, return `SESSION_STARTUP_TIMEOUT` with the partial session ID so the LLM can call `get_session` to check status.
- When `inject_mcp: true`, call `InjectMCPConfig` (see Task 2.3) after the worktree is created but before the tmux session starts. If injection fails, log at WARN and proceed — do not fail session creation.
- Hook injection calls `InjectHooksConfig` (see Task 2.4) with the `hooks` list immediately after MCP injection. `permission_approval` is always included regardless of the `hooks` parameter.
- `update_session` must also support `inject_mcp: bool`, `remove_mcp: bool`, `add_hooks: []string`, and `remove_hooks: []string` to toggle injection on existing sessions without restarting them.

---

### Task 2.2: Implement `pause_session`, `resume_session`, `stop_session`

**Files (max 5)**:
- `server/mcp/tools_lifecycle.go` — add `pause_session`, `resume_session`, `stop_session` handlers
- `server/mcp/server.go` — register new tools

**INVEST**:
- Independent: all three wrap existing session status transitions
- Negotiable: `stop_session` cleanup can be async (fire background goroutine for worktree cleanup); tool returns immediately after initiating stop
- Valuable: completes lifecycle control; LLM can now fully manage session state
- Estimable: 2-3 hours
- Small: 2 files (extending existing lifecycle file)
- Testable: unit tests for each status transition; verify `CONFIRMATION_REQUIRED` error on `stop_session` without `confirm: true`

**`stop_session` schema**:
- `session_id` (required string)
- `confirm` (required bool): must be `true` or tool returns `CONFIRMATION_REQUIRED` error with message: "Stopping a session removes its tmux process and git worktree. Pass confirm=true to proceed."

**Notes**:
- Status transition errors must use machine-readable codes: `SESSION_NOT_FOUND`, `INVALID_STATUS_TRANSITION`, `SESSION_ALREADY_PAUSED`, `SESSION_NOT_RUNNING`.
- Do not expose the internal `Creating` or `Stopped` status as valid transition targets — these are internal states that the MCP surface should not allow external control over.

---

### Task 2.3: Implement `InjectMCPConfig` and per-session MCP injection

**Files (max 5)**:
- `server/services/mcp_injector.go` — new file: `InjectMCPConfig(rootDir, binaryPath string) error` and `RemoveMCPConfig(rootDir string) error`; sibling to `InjectHookConfig` in `approval_handler.go`
- `server/mcp/tools_lifecycle.go` — call `InjectMCPConfig` from `create_session` (when `inject_mcp: true`) and `update_session` (when `inject_mcp`/`remove_mcp` is set)

**INVEST**:
- Independent: `InjectMCPConfig` is a pure file operation; no dependency on MCP server runtime
- Negotiable: `RemoveMCPConfig` can be a no-op in v1 if removal use case is rare; injection is the priority
- Valuable: enables managed Claude sessions to self-discover and use MCP tools; without this, the MCP server exists but the managed agents cannot reach it
- Estimable: 2-3 hours
- Small: 2 files; the core logic mirrors `InjectHookConfig` exactly
- Testable: unit tests covering: file not existing (creates it), file exists without `mcpServers` (merges), file exists with different `mcpServers` entry (merges without overwriting), file exists with our entry already present (idempotent no-op), malformed JSON (attempts repair)

**`InjectMCPConfig` behavior**:
1. Resolve `binaryPath` via `os.Executable()` at call time — always use the absolute path of the running binary
2. Read `<rootDir>/.claude/settings.local.json` (create if missing)
3. Check if `mcpServers.stapler-squad` already points to the same binary — if yes, no-op
4. Merge: add/update `mcpServers.stapler-squad` entry; preserve all other keys including `hooks`
5. Write back atomically (write to temp file, rename)

**Injected JSON** (merged into existing file, not replacing it):
```json
{
  "mcpServers": {
    "stapler-squad": {
      "type": "stdio",
      "command": "<absolute-path-to-stapler-squad-binary>",
      "args": ["--mcp"]
    }
  }
}
```

**`RemoveMCPConfig` behavior**:
1. Read `<rootDir>/.claude/settings.local.json`; if missing, no-op
2. Delete `mcpServers.stapler-squad` key only; preserve all other keys
3. If `mcpServers` is now empty, remove the `mcpServers` key entirely
4. Write back atomically

**Scope boundary — what this task does NOT include**:
- Global install to `~/.claude/settings.json` — this is a future explicit UI action, not part of this epic
- Any modification to existing sessions that were not created with `inject_mcp: true` — zero automatic migration

**Notes**:
- `settings.local.json` is typically listed in `.gitignore` — injection does not pollute the session's git history. Verify before writing; log a warning if the file is tracked by git.
- If injection fails (permission error, disk full, etc.), `create_session` must still succeed. Injection failure is non-fatal: log at WARN, include `mcp_injection_failed: true` in the `create_session` response so the LLM is aware.
- The binary path will break if the binary is moved. Users can re-inject by calling `update_session` with `inject_mcp: true`.

### Task 2.4: Generalized hook injection (`InjectHooksConfig`) and server-side hook receivers

**Files (max 5)**:
- `server/services/hook_injector.go` — new file: `InjectHooksConfig(rootDir, sessionTitle string, hooks []HookName) error`; replaces and supersedes the narrow `InjectHookConfig` in `approval_handler.go`; `HookName` is a typed string enum
- `server/services/hook_receivers.go` — new file: HTTP handlers for `/api/hooks/stop`, `/api/hooks/pre-tool-use`, `/api/hooks/post-tool-use`, `/api/hooks/prompt-submit`
- `server/server.go` — register new hook receiver routes

**INVEST**:
- Independent: hook injection is a pure file operation; receivers are thin HTTP handlers over `EventBus`
- Negotiable: `pre_tool_logging` and `post_tool_logging` receivers can no-op in v1 (log to file only) without full analytics schema; full analytics is a follow-on
- Valuable: `stop_notification` alone enables idle state detection — a high-value capability the existing codebase cannot do without scrollback polling
- Estimable: 4 hours
- Small: 3 files
- Testable: unit tests for `InjectHooksConfig` covering all 5 hook names, combinations, idempotency, and merge-safety; integration test verifying `Stop` receiver updates session `last_idle_at`

**`HookName` enum**:
```go
const (
    HookPermissionApproval HookName = "permission_approval" // maps to Notification event
    HookStopNotification   HookName = "stop_notification"   // maps to Stop event
    HookPreToolLogging     HookName = "pre_tool_logging"    // maps to PreToolUse event
    HookPostToolLogging    HookName = "post_tool_logging"   // maps to PostToolUse event
    HookPromptSubmit       HookName = "prompt_submit"       // maps to UserPromptSubmit event
)
```

**`InjectHooksConfig` behavior**:
1. Always include `permission_approval` regardless of input — it is non-optional
2. For each requested hook, build a `curl` command POSTing to the matching server endpoint with `X-CS-Session-ID` header
3. Merge into existing `hooks` section of `.claude/settings.local.json` using the same read-modify-write-atomically pattern as `InjectHookConfig`
4. Idempotent: if the hook command is already present, skip
5. After writing, deprecate `InjectHookConfig` — update its sole caller (`create_session` in Task 2.1) to use `InjectHooksConfig`

**Server endpoint behavior** (`hook_receivers.go`):

`POST /api/hooks/stop`:
- Parse `X-CS-Session-ID` header
- Update `session.Instance.LastIdleAt = time.Now()` on the matching instance
- Fire `events.SessionIdleEvent` on `EventBus` (new event type; web UI can use this for status indicator)
- Return HTTP 200 with `{"hookSpecificOutput": {"hookEventName": "Stop", "decision": {"behavior": "proceed"}}}`

`POST /api/hooks/pre-tool-use`:
- Parse session ID, tool name, and tool input from request body
- Log to application log at DEBUG level: `[session-id] PreToolUse: <tool-name>`
- In v1: always return proceed (exit 0 semantics). Future: could block specific tools
- Return HTTP 200 with proceed decision

`POST /api/hooks/post-tool-use`:
- Parse session ID, tool name, tool input, tool response from request body
- Log at DEBUG level
- In v1: no-op beyond logging; analytics schema extension is future work
- Return HTTP 200

`POST /api/hooks/prompt-submit`:
- Parse session ID
- Increment `session.Instance.TurnCount` (new field, or log only in v1)
- Return HTTP 200

**Notes**:
- All receivers must respond within the Claude Code hook timeout (300 seconds max; our receiver should respond in <1 second)
- The `stop_notification` event enables a new `last_idle_at` timestamp on sessions, surfaced in `get_session` response — without this, the only way to know Claude finished was to poll scrollback
- `InjectHookConfig` in `approval_handler.go` should be marked `Deprecated` after this task, with a comment pointing to `InjectHooksConfig`. Do not delete it yet — ensure all callers are migrated first.
- Exit code semantics: our HTTP receivers always return HTTP 200 with `behavior: "proceed"` in v1. The `PreToolUse` hook CAN return `behavior: "block"` with a message in the future — this is the evaluation/enforcement path, but blocking requires much more thought about safety.

### Story 2 Acceptance Criteria (updated)
- `create_session` with `inject_mcp: true` writes `mcpServers.stapler-squad` into the new session's `.claude/settings.local.json`
- `create_session` with `inject_mcp: false` leaves `.claude/settings.local.json` untouched
- `create_session` with default `hooks` injects `permission_approval` + `stop_notification` hooks
- `create_session` with `hooks: ["pre_tool_logging", "post_tool_logging"]` injects those plus `permission_approval` (always)
- Existing sessions not created through this flow are never touched
- `update_session` with `inject_mcp: true` injects MCP; `add_hooks: ["stop_notification"]` adds that hook; `remove_hooks: ["pre_tool_logging"]` removes it
- All injection is idempotent — calling twice on the same session produces the same file
- Injection does not overwrite other `mcpServers` entries or user-defined hooks
- `POST /api/hooks/stop` updates session `LastIdleAt` and fires `SessionIdleEvent`
- `get_session` response includes `last_idle_at` field (null if never fired)

---

## Story 3: Terminal I/O and VCS Tools

**Goal**: Add 5 tools for terminal interaction and git inspection. These are the highest-risk tools: `write_to_session` is a command injection surface; `read_session_output` can silently truncate. Both require careful implementation and explicit documentation in tool descriptions.

**Value**: Completes the full LLM workflow — an LLM can now create a session, send a task, poll for completion, and inspect the resulting code changes.

**Acceptance Criteria**:
- `read_session_output` always returns `truncated: bool` and `total_lines: int`; ANSI codes stripped by default
- `read_session_output` never returns more than 200 lines or 10KB, whichever is smaller
- `write_to_session` enforces 1 call/second rate limit per session; returns `RATE_LIMITED` error if exceeded
- `write_to_session` tool description explicitly warns that input reaches the shell/agent unfiltered
- `wait_for_output` respects `timeout_seconds` with hard max of 60; returns `TIMEOUT` error with last-seen output on expiry
- `get_session_diff` and `list_session_branches` return correct git data for worktree-backed sessions
- All 13 tools registered and returning correct responses in the subprocess integration test

### Task 3.1: Implement `read_session_output`

**Files (max 5)**:
- `server/mcp/tools_terminal.go` — new file: `read_session_output` handler
- `server/mcp/ansi.go` — new file: ANSI stripping utility; wraps or reimplements strip logic (validate UTF-8, strip escape sequences, replace invalid bytes with replacement character)
- `server/mcp/server.go` — register terminal tools

**INVEST**:
- Independent: reads from `session.Instance.ScrollbackManager` / tmux capture; no mutation
- Negotiable: ANSI stripping can use a simple regex in v1; a full VT100 parser is optional
- Valuable: first read path to terminal output; required for any monitoring workflow
- Estimable: 3-4 hours
- Small: 3 files
- Testable: unit tests for ANSI stripping edge cases (partial escape sequences, non-UTF8 bytes, empty output, output exactly at line limit, output exceeding line limit)

**`read_session_output` schema**:
- `session_id` (required string)
- `lines` (optional int, default 50, max 200): number of lines from the tail of scrollback
- `strip_ansi` (optional bool, default `true`): strip ANSI escape sequences before returning

**Output**:
- `output` (string): the terminal content
- `truncated` (bool): true if the scrollback has more lines than were returned
- `total_lines` (int): total lines in scrollback buffer at time of read
- `last_sequence` (uint64): scrollback sequence number at time of read (use for change detection)

**Notes**:
- The `CircularBuffer` in `session/scrollback/buffer.go` stores `ScrollbackEntry` items with `Sequence uint64`. Use the sequence number to detect whether output has changed between calls.
- Hard limit: even if `lines=200`, cap total output bytes at 10240 (10KB) before returning. If the byte cap triggers before the line cap, set `truncated: true`.
- ANSI stripping must handle partial escape sequences at buffer boundaries (e.g., `\x1b[` at end of last line without the terminator).

---

### Task 3.2: Implement `run_command`, `write_to_session`, `wait_for_output`, and `send_control`

**Files (max 5)**:
- `server/mcp/tools_terminal.go` — add all four handlers
- `server/mcp/rate_limiter.go` — add per-session rate limiter for `write_to_session` (1 call/second per session_id)

**INVEST**:
- Independent: all four wrap existing PTY/scrollback primitives; no new session state introduced
- Negotiable: `run_command` polling interval can be 1 second in v1; `wait_for_output` shares the same poller
- Valuable: `run_command` covers ~80% of LLM use cases in 1 call vs the previous 3-call sequence; `send_control` is required for interrupt/EOF
- Estimable: 4 hours
- Small: 2 files
- Testable: unit tests for rate limiting; integration test for `run_command` verifying output appears after command completes

**`run_command` schema** (composite — use this for the common case):
- `session_id` (required string)
- `command` (required string): shell command to run; appends `\n` automatically; max 4096 bytes
- `timeout_seconds` (optional int, default 30, max 120): how long to wait for output to stop changing
- `lines` (optional int, default 50, max 200): lines to return from output

**`run_command` output**:
- `output` (string): terminal output captured after the command, ANSI stripped
- `truncated` (bool): true if output exceeded `lines` or 10KB
- `timed_out` (bool): true if `timeout_seconds` elapsed before output stabilized
- `last_sequence` (uint64): scrollback sequence at time of capture

**`run_command` tool description** (exact text — this is prompt engineering):
> Send a shell command to a running session and wait for output. Combines write + wait + read in one call. Waits until output stops changing or timeout_seconds elapses, then returns the captured output. Use this for most command execution; use write_to_session + wait_for_output only when you need finer control. Input reaches the PTY unfiltered.

**Implementation note**: `run_command` is implemented as: `SendKeys(command + "\n")` → poll `ScrollbackManager` until `Sequence` stops changing for 2 consecutive seconds or timeout expires → return last N lines. Do not hold a lock during polling.

---

**`write_to_session` schema** (low-level — use when `run_command` doesn't fit):
- `session_id` (required string)
- `input` (required string): text to send; max 4096 bytes; returns `INPUT_TOO_LONG` if exceeded
- `press_enter` (optional bool, default `true`): append `\n` after input

**`write_to_session` tool description** (exact text — this is prompt engineering):
> Send text input to a running session's terminal. Input is written directly to the session's PTY and reaches the running program (claude, shell, etc.) unfiltered. This tool is fire-and-forget: it returns immediately without waiting for the input to be processed. Use run_command for most cases; use this only when you need to send input without waiting. Rate limited to 1 call per second per session.

---

**`send_control` schema** (for interrupt, EOF, clear, suspend):
- `session_id` (required string)
- `key` (required string enum: `C` | `D` | `Z` | `L`): control character to send
  - `C` → Ctrl+C (`\x03`): interrupt running process
  - `D` → Ctrl+D (`\x04`): EOF / exit shell
  - `Z` → Ctrl+Z (`\x1a`): suspend process to background
  - `L` → Ctrl+L (`\x0c`): clear screen

**`send_control` output**: `{ sent: string }` — confirms which byte was sent (e.g., `"^C"`)

**`send_control` tool description** (exact text):
> Send a control character to a running session. Use key="C" to interrupt (Ctrl+C) a hung or running process, key="D" for EOF/exit, key="Z" to suspend, key="L" to clear screen. Returns immediately. Follow with read_session_output to confirm effect.

---

**`wait_for_output` schema** (for explicit pattern matching):
- `session_id` (required string)
- `pattern` (required string): substring or regex pattern to match in output
- `timeout_seconds` (optional int, default 30, max 60)
- `lines` (optional int, default 50, max 200): lines to return on match/timeout

**Output**:
- `matched` (bool): whether the pattern was found before timeout
- `output` (string): the terminal output at time of match or timeout, ANSI stripped
- `matched_line` (string, optional): the specific line that matched the pattern

**Notes**:
- `wait_for_output` must use a polling loop with 1-second intervals and a `context.WithTimeout`. It must NOT hold a lock on the session or subscribe to the control mode channel (to avoid subscriber channel overflow — see pitfalls research).
- On timeout, return `matched: false`, the last-seen output, and error code `WAIT_TIMEOUT`. Do not return an error-level result — timeout is an expected outcome the LLM should handle gracefully.
- `run_command` is preferred over `write_to_session` + `wait_for_output` for simple command execution.

---

### Task 3.3: Implement `get_session_diff` and `list_session_branches`

**Files (max 5)**:
- `server/mcp/tools_vcs.go` — new file: `get_session_diff` and `list_session_branches` handlers
- `server/mcp/server.go` — register VCS tools; also verify all 15 tools are registered

**INVEST**:
- Independent: both call existing git manager methods on the session's worktree
- Negotiable: `get_session_diff` can be truncated at 50KB in v1; full diff is a v2 concern
- Valuable: completes the VCS inspection surface; LLM can see what code changes a session has produced
- Estimable: 2-3 hours
- Small: 2 files
- Testable: unit tests against a fixture git repo with staged and unstaged changes; verify truncation behavior

**`get_session_diff` schema**:
- `session_id` (required string)
- `max_bytes` (optional int, default 51200, max 102400): cap on diff output bytes

**Output**:
- `diff` (string): unified diff output
- `truncated` (bool): true if diff exceeded `max_bytes`
- `stats` (object): `{ files_changed: int, insertions: int, deletions: int }`

**`list_session_branches` schema**:
- `session_id` (required string)

**Output**:
- `current_branch` (string)
- `branches` (string array): all local branches for the worktree's repository
- `has_upstream` (bool): whether current branch has a remote tracking branch

---

## Known Issues

### Command Injection via `write_to_session` [SEVERITY: CRITICAL]

**Description**: `write_to_session` passes its `input` parameter directly to the session's PTY via `session.Instance.SendKeys()` (which calls `ptmx.Write()`). There is no sanitization. An LLM processing attacker-controlled content (prompt injection, malicious tool descriptions) could be manipulated into sending dangerous shell commands — `rm -rf`, credential exfiltration, etc. Real CVEs exist for this pattern: CVE-2025-6514 (shell injection via OAuth metadata) affected 437k developer environments.

**Mitigation**:
- Tool description explicitly states input reaches the PTY unfiltered (see Task 3.2 exact description text)
- Input length cap at 4096 bytes prevents bulk command injection
- Rate limiting (1 call/second) limits blast radius of runaway injection
- All `write_to_session` calls logged at INFO level with session ID, input length, and first 100 characters (redacted if containing credentials patterns)
- Consider adding an opt-in allowlist mode (`STAPLER_SQUAD_MCP_WRITE_ALLOWLIST`) for high-security environments

**Files affected**:
- `server/mcp/tools_terminal.go`
- `server/mcp/rate_limiter.go`
- `session/tmux/tmux.go` (existing `SendKeys` method — no change needed, but understand the path)

**Prevention**: The tool description is the primary control. It must remain accurate if the implementation changes. Add a test that asserts the tool description contains the word "unfiltered".

---

### Silent Truncation in `read_session_output` [SEVERITY: HIGH]

**Description**: Claude Code silently truncates MCP tool output at 256 lines / 10KB (confirmed issue: github.com/anthropics/claude-code/issues/2638). If `read_session_output` returns more than 10KB, the LLM sees a partial response with no indication that truncation occurred. It then makes decisions on incomplete data — missing error messages, partial diffs, etc.

**Mitigation**:
- Apply an explicit 10KB / 200-line cap before returning; do not rely on the MCP client to truncate
- Always return `truncated: bool` and `total_lines: int` so the LLM can detect partial data
- If `truncated: true`, include a note in the output: `[... N lines omitted. Call read_session_output with lines=200 to see earlier output ...]`
- Use `last_sequence` in the response so the LLM can detect whether output has changed between calls

**Files affected**:
- `server/mcp/tools_terminal.go`
- `server/mcp/ansi.go` (byte cap applied after ANSI stripping)

**Prevention**: Unit test that passes 250 lines to `read_session_output` and asserts `truncated: true` is returned.

---

### Orphaned Session Cleanup on `stop_session` Failure [SEVERITY: MEDIUM]

**Description**: `CleanupWorktree()` in `session/instance.go` can fail if the git worktree has lock files (`.git/index.lock`) or the directory is in use. When this happens from the MCP `stop_session` tool, the session is removed from the database but the worktree directory persists on disk. Subsequent `create_session` calls with the same branch name will collide with the orphaned worktree, returning a git error that the LLM cannot resolve without manual intervention.

**Mitigation**:
- `stop_session` returns a partial success response when cleanup fails: `{ success: true, cleanup_error: "worktree cleanup failed: ...", cleanup_error_code: "WORKTREE_CLEANUP_FAILED" }`. The session is stopped; only the worktree remains.
- Include remediation advice: `"cleanup_remediation": "Run: git worktree remove --force <path> to manually clean up"`
- Log cleanup failures at WARN level so they appear in `~/.stapler-squad/logs/stapler-squad.log`
- Future: background retry task (not in this epic scope)

**Files affected**:
- `server/mcp/tools_lifecycle.go`
- `session/instance.go` (existing `CleanupWorktree` — read but do not modify)

**Prevention**: Unit test for `stop_session` where `CleanupWorktree` returns an error; assert partial success response shape.

---

### PTY Write Deadlock on Blocked `write_to_session` [SEVERITY: MEDIUM]

**Description**: The PTY (`creack/pty`) can deadlock when the read buffer fills while a write is blocked. `session.Instance.SendKeys()` calls `ptmx.Write()` with no timeout. If the tmux pane's stdout buffer is full and the pane is waiting for input, neither read nor write can proceed — classic PTY deadlock. The `write_to_session` MCP tool would hang indefinitely.

**Mitigation**:
- Wrap `SendKeys` call in a goroutine with a 5-second timeout context; if the goroutine does not complete, cancel and return `PTY_WRITE_TIMEOUT` error
- Document in tool description that the tool returns immediately (fire-and-forget) — this is already the design, but the timeout is a safety net against the deadlock scenario

**Files affected**:
- `server/mcp/tools_terminal.go`

**Prevention**: Note that this is a latent bug in existing `SendKeys` — the MCP layer adds the timeout wrapper that the existing code lacks. Test by mocking a blocked PTY write.

---

### Rate Limiting Bypass via Concurrent Tool Calls [SEVERITY: LOW]

**Description**: The per-session rate limiter for `write_to_session` (1 call/second) is implemented at the MCP handler layer. If the LLM issues concurrent tool calls (which MCP clients can do), the rate limiter must be goroutine-safe. A naive `map[string]time.Time` implementation without synchronization will have a race condition.

**Mitigation**:
- Use `sync.Map` or a mutex-protected map in `rate_limiter.go`
- Run tests with `-race` flag to detect data races in the rate limiter

**Files affected**:
- `server/mcp/rate_limiter.go`

**Prevention**: The `server/services/rate_limiter.go` file contains an existing rate limiter implementation — follow its patterns.

---

## Integration Checkpoints

### Checkpoint A: After Story 1 complete
- Run `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./stapler-squad --mcp`
- Expected: valid MCP initialize response on stdout, no log lines mixed in
- Run `make test` — all tests pass

### Checkpoint B: After Story 2 complete
- Add `stapler-squad` to Claude Code's MCP config: `{"mcpServers": {"stapler-squad": {"command": "<path>/stapler-squad", "args": ["--mcp"]}}}`
- Ask Claude: "List my active sessions" — should return real session data
- Ask Claude: "Create a new session called 'test-mcp' in ~/projects/test" — should create and return session ID
- Verify `.claude/settings.local.json` in the new session's worktree contains `mcpServers.stapler-squad` and both `Notification` and `Stop` hook entries
- Ask Claude to create a second session with `inject_mcp: false, hooks: []` — verify `.claude/settings.local.json` only contains the always-on `permission_approval` hook (Notification), nothing else
- Ask Claude to create a third session with `hooks: ["pre_tool_logging", "post_tool_logging"]` — verify `PreToolUse` and `PostToolUse` entries appear in the file
- Manually trigger a `Stop` event (run a task to completion in the session) and verify `GET /api/sessions/<id>` shows a non-null `last_idle_at`
- Verify calling `update_session` on an older existing session with `inject_mcp: true, add_hooks: ["stop_notification"]` injects correctly without touching other settings keys
- Ask Claude to stop it with `stop_session` without `confirm: true` — should get a clear error
- Run `make test` — all tests pass

### Checkpoint C: After Story 3 complete (all 15 tools)
- Full workflow test using `run_command`: "Create a session, run `ls -la` via `run_command`, read the output in one call, then stop the session" — verify this takes 3 tool calls total (create, run_command, stop), not 5
- Verify `truncated: bool` in `read_session_output` and `run_command` responses
- Verify `write_to_session` rate limit fires on rapid repeated calls
- Verify `send_control` with `key="C"` interrupts a running `sleep 60` command
- Verify `list_sessions` with no args returns ≤10 results and includes `next_cursor`
- Verify `get_session_diff` returns diff for a session with uncommitted changes
- Run `make test` and `make lint` — all pass

---

## Context Preparation Guide

Before starting each story, read these files. Do not read files not listed here — context is expensive.

### Story 1 context (MCP Foundation)
- `main.go` — understand existing flag parsing and startup flow; find where to add `--mcp` branch
- `server/server.go` — understand `NewServer()` dependency initialization order (relevant for what NOT to initialize in MCP mode)
- `server/services/session_service.go` — understand `SessionService` struct and what dependencies it needs
- `session/instance.go` — understand `session.Status` constants and `Instance` struct
- `session/storage.go` (if exists) or the `InstanceStore` interface — understand the storage query methods available
- `go.mod` — verify current dependencies before adding `mark3labs/mcp-go`

### Story 2 context (Session Lifecycle + MCP Injection)
- `server/mcp/server.go` — the MCP server created in Story 1
- `server/mcp/tools_discovery.go` — response type patterns established in Story 1
- `server/mcp/types.go` — shared response types
- `server/services/session_service.go` — find `CreateSession`, `PauseSession`, `ResumeSession`, `StopSession` method signatures
- `session/instance.go` — understand status transitions, `CleanupWorktree()`, and `GetEffectiveRootDir()`
- `server/services/approval_handler.go` — read `InjectHookConfig` (lines ~472–600) to understand the merge-and-write pattern; Task 2.3/2.4 generalize this into `InjectMCPConfig` and `InjectHooksConfig`
- `server/services/rate_limiter.go` — understand existing rate limiter patterns before writing a new one

### Story 3 context (Terminal I/O + VCS)
- `server/mcp/tools_lifecycle.go` — patterns from Story 2 for service calls and error handling
- `session/scrollback/buffer.go` — understand `CircularBuffer`, `ScrollbackEntry`, `Sequence` field
- `session/scrollback/manager.go` — understand how to get the scrollback buffer for a session
- `session/tmux/tmux.go` — find `SendKeys` method; understand PTY write path
- `session/git/` (directory) — find diff and branch listing methods
- `server/mcp/rate_limiter.go` — extend for write_to_session per-session limiting

---

## File Layout

New files created by this epic:

```
server/mcp/
  server.go          — MCP server init, tool registration, RunServer()
  server_test.go     — subprocess integration test
  types.go           — shared MCPResult, MCPError, SessionSummary, SessionDetail types
  logging.go         — MCPLogger, InitMCPLogging()
  rate_limiter.go    — per-tool and per-session rate limiting
  ansi.go            — ANSI stripping, UTF-8 validation
  tools_discovery.go — list_sessions, get_session, search_sessions
  tools_lifecycle.go — create_session, pause_session, resume_session, stop_session, update_session
  tools_terminal.go  — run_command, read_session_output, write_to_session, send_control, wait_for_output
  tools_vcs.go       — get_session_diff, list_session_branches

server/services/
  mcp_injector.go    — InjectMCPConfig, RemoveMCPConfig
  hook_injector.go   — InjectHooksConfig, HookName enum; supersedes InjectHookConfig in approval_handler.go
  hook_receivers.go  — HTTP handlers for /api/hooks/stop, /pre-tool-use, /post-tool-use, /prompt-submit
```

Modified files:
```
main.go   — add --mcp flag and RunServer() invocation
go.mod    — add mark3labs/mcp-go dependency
go.sum    — updated automatically
```
