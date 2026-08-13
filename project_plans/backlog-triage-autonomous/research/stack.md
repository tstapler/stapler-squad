# Stack Research: backlog-triage-autonomous

## 1. Session Lifecycle Packages and Patterns

**Package layout:**
- `session/` — core session management (Instance, Storage, lifecycle)
- `session/tmux/` — tmux process management
- `session/detection/` — Claude status detection (idle, needs-approval, etc.)
- `session/headless/` — headless LLM pool (`claude -p` subprocess)
- `server/services/` — RPC handlers (SessionService, BacklogService)
- `server/mcp/` — MCP server exposing tools to Claude sessions

**Session instance fields (relevant to triage):**
- `Prompt string` — passed to `InstanceOptions` by `CreateDirectorySession`; stored on `inst.Prompt`; used by `AutonomousDriver` as the `goal` argument
- `InitialPrompt string` — the field `session_driver.go` reads for tmux prompt injection; replaces the static `driverInitialPrompt` ("Please proceed with the task described in your instructions.") when non-empty
- `OneShot bool` — controls whether the session uses `claude -p` (one-shot) vs interactive tmux mode; drives the `isOneShot()` guard in the session driver

**Critical gap (root cause):** `CreateDirectorySession` receives the triage prompt as `prompt` and sets `InstanceOptions.Prompt = prompt`. The `Prompt` field flows to `inst.Prompt`. But `session_driver.go` reads `inst.InitialPrompt`, **not** `inst.Prompt`, for tmux injection. When `useAutonomous=true`, the `AutonomousDriver` reads `inst.Prompt` as its `goal`. When `useAutonomous=false` (oneShot path), the session driver sends the `driverInitialPrompt` ("Please proceed…"), not the actual triage prompt, because `inst.InitialPrompt` is empty.

## 2. headless.Pool: Initialization and Nil Conditions

**Source:** `session/headless/caller.go`

```go
func NewPool(cfg PoolConfig) (*Pool, error) {
    bin, err := exec.LookPath("claude")
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrClaudeNotFound, err)
    }
    // ...
}
```

**When pool is nil:** `headlessPool` is only nil if `exec.LookPath("claude")` fails — i.e., the `claude` binary is not in `$PATH`. The log message is: `"headless pool disabled: claude binary not found"`. The success path logs: `"headless LLM pool initialized"`.

**Wiring path** (`server/dependencies.go` lines 435–448):
1. `headless.NewPool(PoolConfig{MaxCallsPerSession: 25, MaxConcurrentSessions: 5})` is called
2. On error: `headlessPool` stays `nil`
3. On success: `headless.SetDefaultPool(p)`, `sessionService.SetHeadlessPool(p)`, both called
4. `backlogLifecycleListener` is then constructed with `pool` — it uses the pool for **review gates**, not triage
5. `backlogSvc.SetAutonomousDriverStarter(sessionService)` at line 757 wires `sessionService` as the `AutonomousDriverStarter` for `BacklogService`

**Guard in StartAutonomousDriverWithTimeout** (`session_service.go` line 731):
```go
if s.headlessPool == nil {
    log.Warn("[SessionService] StartAutonomousDriverWithTimeout: headlessPool is nil", "session", inst.Title)
    return  // silently no-ops; triage session already spawned and sitting idle
}
```

This is the silent-failure path — the triage tmux session is running, but if the headless pool is nil, `StartAutonomousDriverWithTimeout` returns immediately with no error surfaced to the caller.

## 3. session_driver.go: InitialPrompt vs Prompt

**Source:** `session/session_driver.go`

The session driver is started unconditionally by `CreateDirectorySession`:
```go
session.StartSessionDriver(instance, path)
```

The driver reads:
```go
func runSessionDriver(inst *Instance, allowedPath string) {
    var retried atomic.Bool
    var initialPrompt string
    if inst.InitialPrompt != "" {
        sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
        if sanitized != "" {
            initialPrompt = sanitized
        }
    }
    runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, &retried)
}
```

And in `runSessionDriverWithPrompt`:
```go
sentInitial := initialPrompt == ""  // true if no InitialPrompt set → driver treats "already sent"
```

**Dual-path behavior:**
- `oneShot=true` (autonomous unavailable): `inst.InitialPrompt` is empty → `sentInitial=true` → driver never sends a prompt → session sits idle at `>`
  - Actually for `oneShot=true` with `claude -p`, the prompt would be passed as the `-p` argument; but for `CreateDirectorySession` with `prompt` arg, the triage prompt is in `inst.Prompt`, not `inst.InitialPrompt`, so even the driver would not inject it
- `oneShot=false` (autonomous path): `inst.InitialPrompt` is still empty → same result — driver sends the static `driverInitialPrompt` ("Please proceed with the task described in your instructions.") which is useless to Claude without context

**Neither path injects the actual triage prompt into the tmux pane via the session driver.** The autonomous path depends entirely on the `AutonomousDriver` to inject it via `inst.Prompt` (as the `goal`). The `isOneShot` guard in the driver (line 491) checks tags `backlog:triage` and `backlog:review`, treating them as non-retryable exits — this is correct, but still doesn't fix the missing initial prompt.

## 4. AutonomousDriver: Dependencies and Failure Modes

**Source:** `session/autonomous_driver.go`

**Constructor:** `NewAutonomousDriver(inst, pool, goal, maxTurns, opts...)`
- `pool` = `s.headlessPool` (from `SessionService`)
- `goal` = `inst.Prompt` (the full triage prompt string)
- `maxTurns` = 0 → defaults to 20

**Startup sequence:**
1. `Start()` checks `d.headlessPool == nil` → returns error (not silent)
2. Gets `d.controller = inst.GetController()` → if nil, returns error
3. Spawns goroutine that calls `run(ctx)`
4. `run()` waits for first idle via `waitForIdle(startupCtx, statusCh, cc)` with 5-minute startup timeout (for triage, set via `WithStartupTimeout(5*time.Minute)`)
5. On each turn: calls `buildOrchestrationPrompt(goal, tail, turn, maxTurns)` → sends to headless LLM → parses `NEXT_MESSAGE: <text>` or `DONE: <reason>` → injects `<text>` via `d.controller.SendCommandImmediate(nextMsg + "\r")`

**Key failure modes:**
1. **headlessPool nil** (silent): `StartAutonomousDriverWithTimeout` logs a warning and returns; no error propagated to `TriggerTriage` caller; session is idle with no driver
2. **controller nil**: `Start()` returns error; logged at Warn level; no retry
3. **startup timeout (5 min)**: `waitForIdle` times out → driver logs `"timed out waiting for initial idle state"` and fires completion with `{Stuck: true, Reason: "startup timeout"}`
4. **idle detection misfire**: A fresh Claude session shows the `>` prompt which triggers `StatusIdle`; the driver correctly detects this (fast path: `cc.IsIdle()` check in `waitForIdle`)
5. **Turn 1 LLM call**: The orchestrator LLM is called with `goal=<triage prompt>` + `tail=<current terminal output>`; the triage prompt IS passed here as the goal. The driver injects the LLM's `NEXT_MESSAGE` response — meaning **the first injected message is whatever the orchestrator LLM decides to send**, not the triage prompt directly.

**Important nuance on Turn 1:** The `AutonomousDriver` does NOT inject the triage prompt directly. It calls the headless LLM with `goal = inst.Prompt` (the triage prompt) and `tail = preview of session output`, then injects the LLM's `NEXT_MESSAGE`. This is a meta-orchestration step — the orchestrator decides what to tell Claude. If the orchestrator LLM is also unavailable or the first call fails, nothing is injected.

## 5. MCP Tools: submit_triage_result Registration

**Source:** `server/mcp/tools_backlog.go`, `server/mcp/server.go`

**Registration path:**
- `NewCore()` in `server/mcp/server.go` calls `registerBacklogTools(s, &backlogHandlers{...})` when `storage != nil`
- `registerBacklogTools` registers `submit_triage_result` (line 654)
- The MCP server is mounted at the HTTP path (`/mcp`) AND available via stdio

**Session UUID injection for `callerSessionUUID`:**
- HTTP path: The MCP streamable HTTP server is mounted at `s.mcpServerURL` (configured in `MCPServerURL` field on `InstanceOptions`)
- Stdio path: `STAPLER_SESSION_UUID` env var is read at server startup and injected into context via `WithSessionUUID`
- For triage sessions: `inst.MCPServerURL = s.mcpServerURL` is set in `CreateDirectorySession`; the session launches with MCP server URL configured, enabling the `submit_triage_result` call via HTTP MCP

**`callerSessionUUID` resolution** (`tools_backlog.go` line 437):
```go
callerUUID, err := callerSessionUUID(ctx)
```
This reads `STAPLER_SESSION_UUID` from the tmux session environment (set at `instance.go` line 1096: `"STAPLER_SESSION_UUID=" + i.UUID`).

**Role verification:** `submitTriageResult` verifies `itemSession.SessionRole == "triage"` before proceeding. The role is set to `SessionRoleTriage` at `TriggerTriage` line 1191.

## Summary of Root Cause

The triage flow has two paths:

**Autonomous path (useAutonomous=true, i.e. headlessPool available):**
- Session is created with `oneShot=false`, `inst.Prompt=triagePrompt`
- `StartAutonomousDriverWithTimeout` is called
- If headlessPool is nil at call time: silently no-ops → session idle
- If headlessPool is available: driver waits for idle → calls orchestrator LLM → injects orchestrator's decision, not the raw triage prompt
- The session driver also runs (`StartSessionDriver`) but `inst.InitialPrompt` is empty → sends static "Please proceed…" which tells Claude nothing useful (or `sentInitial=true` if the JSONL file already exists)

**oneShot fallback path (useAutonomous=false):**
- Session is created with `oneShot=true`
- `inst.InitialPrompt` is empty → session driver cannot send the triage prompt
- `inst.Prompt` is set (for `claude -p` passthrough) but `CreateDirectorySession` sets `OneShot=true` only; actual `claude -p` invocation with `-p prompt` would need to happen in `instance.Start()` flow

**Critical bug:** In the autonomous path, `StartAutonomousDriverWithTimeout` silently no-ops when `headlessPool == nil`, leaving the triage session running but completely idle. No error is returned to `TriggerTriage` and no fallback is attempted.

**Secondary issue:** The `AutonomousDriver` does not inject the triage prompt directly on Turn 1; it invokes an orchestrator LLM call, adding a dependency on the headless pool working correctly for the orchestration itself (separate from the reviewed session). If this meta-LLM call fails, Turn 1 gets skipped and the loop breaks.

## Key File Paths

- `/session/autonomous_driver.go` — AutonomousDriver, NewAutonomousDriver, run(), waitForIdle
- `/session/session_driver.go` — runSessionDriver(), InitialPrompt vs Prompt distinction
- `/session/instance.go` — Instance struct, Prompt/InitialPrompt fields (lines 122-125)
- `/session/headless/caller.go` — NewPool(), binary lookup
- `/server/services/backlog_service.go` — TriggerTriage (lines 1074-1206), buildTriagePrompt
- `/server/services/session_service.go` — StartAutonomousDriverWithTimeout (line 729), CreateDirectorySession (line 579)
- `/server/dependencies.go` — headlessPool initialization (lines 435-449), wiring (line 757)
- `/server/mcp/tools_backlog.go` — submit_triage_result tool registration (line 654)
- `/server/mcp/server.go` — MCP server, STAPLER_SESSION_UUID injection (line 64)
