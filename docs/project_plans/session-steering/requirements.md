# Session Steering — Requirements

## Problem Statement

Automated sessions (backlog triage, work, review, MCP-created) currently start with no supervision. They block on Claude Code's trust-folder startup dialog, sit idle at the `>` prompt until a human sends the first message, and if they crash or hang silently there is no recovery mechanism. The result is invisible failures: sessions that look "Running" but are doing nothing.

## Target Users

- The stapler-squad system itself (automated pipeline driving backlog items through triage → work → review)
- Any MCP client creating sessions via `create_session`

## Goals

1. **Universal driver coverage** — every automated session gets a `SessionDriver` goroutine at creation time, regardless of which code path created it
2. **Startup dialog handling** — driver answers the Claude Code trust-folder safety check and directory-access dialogs without human intervention
3. **Initial prompt dispatch** — driver sends the task prompt once the session is at the `>` ready state (or after a 30s timeout fallback)
4. **Dead/stuck detection** — driver (or a watchdog) detects two failure modes:
   - Session process exits unexpectedly (status → Stopped before task completion)
   - Session is alive but produces no terminal output change for 10 minutes (inactivity timeout)
5. **Auto-retry with JSONL continuation** — on first failure, restart the session and inject a "here's where you left off" prompt built from the session's JSONL conversation history; if retry also fails, mark status as `NeedsAttention`
6. **Passive Go watchdog coordinator** — a single goroutine per process that monitors all steered sessions; no persistent Claude coordinator session required

## Out of Scope

- Coordinator agent implemented as a long-lived Claude session
- Retry count configurable beyond once (always retry exactly once)
- UI changes for displaying steering status beyond the existing `NeedsAttention` status
- Steering non-automated (user-created) sessions

## Functional Requirements

### FR-1: Universal driver wiring
- `SessionDriver` must be started for sessions created via:
  - Backlog triage (`backlog_service.go` → `createTriageSession`)
  - Backlog work (`backlog_service.go` → `createWorkSession` or equivalent)
  - Backlog review (`backlog_service.go` → `createReviewSession` or equivalent)
  - MCP `create_session` tool (`tools_lifecycle.go`)
- `CreateDirectorySession` in `session_service.go` already wires the driver; the remaining four paths must be updated
- The `allowedPath` passed to the driver must match the session's effective working directory

### FR-2: Startup dialog handling (existing, needs verification)
- `isStartupDialog()` detects the Claude Code trust-folder safety check
- Driver sends `"1\n"` to select "Yes, I trust this folder"
- Driver checks on every poll tick before checking `sentInitial`

### FR-3: Initial prompt dispatch (existing, needs verification)
- Driver sends `driverInitialPrompt` ("Please proceed with the task described in your instructions.") when `inst.Status == Ready`
- Falls back to sending after `driverReadyTimeout` (30s) even if status never reaches `Ready`

### FR-4: Inactivity detection
- Driver tracks last-seen terminal output (via `inst.Preview()`) and its timestamp
- If output has not changed for `driverInactivityTimeout` (10 minutes) while status is `Running`, the session is considered stuck
- Stuck detection only applies after `sentInitial == true` (don't fire during startup)

### FR-5: Exit detection
- Driver detects when `inst.Status == Stopped` after `sentInitial == true`
- Distinguishes expected completion from unexpected exit by checking whether a completion signal was observed (TBD: use a sentinel string in output, or rely on BacklogLifecycleListener for backlog sessions)

### FR-6: JSONL continuation prompt
- On failure trigger (stuck or unexpected exit), locate the session's JSONL conversation log at `~/.claude/projects/<hashed-path>/<uuid>.jsonl`
- Read the last N assistant + user messages (N = 10 or until 2000 tokens)
- Produce a concise summary: last task attempted, last tool used, last error or output, what remains
- Use this as the body of the continuation prompt: `"Here's where you left off: <summary>. Please continue."`

### FR-7: Auto-retry (once)
- On first failure: stop the existing session instance, create a new instance on the same path/branch, start `SessionDriver` on it, send the continuation prompt instead of `driverInitialPrompt`
- Update the instance in storage with the new session reference
- Set a `retried` flag on the driver so it does not retry a second time
- On second failure (if retry also fails): update `inst.Status` to `NeedsAttention` and stop the driver

### FR-8: Watchdog coordinator
- A `SessionStewardship` goroutine (or equivalent) holds references to all active steered sessions
- Runs alongside the existing pollers in `server/dependencies.go`
- Provides a `RegisterSteered(inst *Instance, allowedPath string)` entry point called by all session creation paths
- The watchdog does not itself do polling; each session's own `SessionDriver` goroutine does the polling and calls back to the watchdog on failure

## Non-Functional Requirements

- **No persistent Claude session for coordination** — all steering logic runs in Go goroutines
- **Minimal API calls** — JSONL reading is local disk I/O; no Anthropic API calls for continuation summary (the continuation prompt is constructed by Go code, not a separate LLM call)
- **Crash isolation** — a panic in one session's driver goroutine must not affect other sessions; recover with a log
- **Idempotent wiring** — calling `StartSessionDriver` twice on the same instance must be a no-op (guard with a sync.Once or status check)

## Acceptance Criteria

| ID | Criterion |
|----|-----------|
| AC-1 | A new backlog triage session automatically answers the trust-folder dialog and sends the initial prompt without human input |
| AC-2 | A new backlog work session does the same |
| AC-3 | A new backlog review session does the same |
| AC-4 | A new MCP-created session does the same |
| AC-5 | A session that exits unexpectedly (kill -9 on its tmux process) is restarted exactly once with a continuation prompt derived from its JSONL history |
| AC-6 | A session that produces no output for 10 minutes is restarted exactly once with a continuation prompt |
| AC-7 | A session that fails twice has its status set to NeedsAttention and no further restarts are attempted |
| AC-8 | All 4 session creation paths call `StartSessionDriver` (or equivalent registration) |
| AC-9 | Two `StartSessionDriver` calls on the same instance do not spawn two driver goroutines |
| AC-10 | A panic inside a driver goroutine is recovered and logged without crashing the server |

## Existing Partial Implementation

The following already exists and must be preserved/extended:

- `session/session_driver.go` — `StartSessionDriver`, `runSessionDriver`, `isStartupDialog`, `shouldApprovePrompt`
- `session/session_driver_test.go` — unit tests for dialog detection
- `server/services/session_service.go:474` — `session.StartSessionDriver(instance, path)` call in `CreateDirectorySession`

The driver is **not** yet wired to backlog sessions or MCP sessions. The inactivity detection, JSONL reading, and retry logic do not yet exist.
