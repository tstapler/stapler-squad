# Pitfalls Research — Backlog Triage Autonomous Driver

**Date**: 2026-06-22  
**Scope**: Risks and failure modes for the autonomous-driver path in `TriggerTriage` + `StartAutonomousDriverWithTimeout`.

---

## 1. Autonomous Driver Reliability — `waitForIdle` Path

### P1 — Status channel capacity-1 drop (HIGH)

`statusCh` is buffered with capacity 1 (`make(chan detection.DetectedStatus, 1)`).
`AddStatusChangeListener` fires on every status transition. If the controller fires two transitions in rapid succession (e.g. `StatusActive` then `StatusIdle` within a single poll cycle) while `waitForIdle` is blocked in the select, the second write is silently dropped because the channel is full and the send uses a non-blocking select:

```go
select {
case statusCh <- newStatus:
default:  // ← DROP
}
```

If the dropped status was `StatusIdle`, `waitForIdle` will not see it and must wait for the next transition. Since the triage session runs parallel subagents and can bounce through many status values rapidly, this is a realistic scenario that delays or prevents idle detection during the startup window.

**Risk**: Startup timeout fires (5 min default) even though the session became idle; driver reports `stuck`.

### P2 — Idle race: session becomes idle before listener is registered (HIGH)

The listener is registered inside `run()` (line 168 of `autonomous_driver.go`), after `Start()` has launched the goroutine. There is a window between `CreateDirectorySession` returning and `driver.Start()` being called (lines 1181–1187 in `backlog_service.go`). During that window `StartSessionDriver` is already running; if the triage session is already at idle when `run()` registers the listener, `waitForIdle` will miss the idle transition.

The fast-path (`cc.IsIdle()`) at line 307 guards against this for an already-idle controller, **but only if the controller has been wired**. In `CreateDirectorySession` the controller is started at line 604 only when `s.statusManager != nil`. If the status manager is nil (edge case, wiring error), `GetController()` returns nil and `Start()` returns an error before the listener is ever registered.

**Risk**: Session sits permanently in `waitForIdle` if idle fires before listener registration AND the fast-path returns false because the controller is not yet started.

### P3 — Controller nil after `Start()` is called (MEDIUM)

In `NewAutonomousDriver` (line 72), `d.controller = inst.GetController()` is called at construction time. In `Start()` (line 122), it is re-fetched: `d.controller = d.inst.GetController()`. If the controller has not been started yet at construction time, the first fetch returns nil (harmless — it is overwritten). However if the controller fails to start (e.g. `StartController()` errors at line 604 and logs a warning but does not fail `CreateDirectorySession`), both fetches return nil, `Start()` returns an error, and the driver is never registered — sessions sit idle with no driver.

This path logs a warning but silently degrades (`StartAutonomousDriverWithTimeout` just returns early).

---

## 2. Race Conditions

### P4 — Double-driver race: `StartSessionDriver` vs `StartAutonomousDriverWithTimeout` (HIGH)

`CreateDirectorySession` calls `session.StartSessionDriver(instance, path)` unconditionally at line 608, **before** `StartAutonomousDriverWithTimeout` is called at line 1187 (after `CreateDirectorySession` returns). Both goroutines now own the session:

- `SessionDriver` will inject the full triage prompt via tmux `SendKeys` (4096-char truncated) once it detects `StatusIdle`.
- `AutonomousDriver` will then also inject orchestration prompts via `SendCommandImmediate` after each turn.

For triage sessions (`isOneShot(inst)` returns true because tag `backlog:triage` is present), `SessionDriver` does not retry on exit — it exits cleanly. However, the initial prompt injection from `SessionDriver` and the first NEXT_MESSAGE from `AutonomousDriver` can both arrive at nearly the same time (both waiting for idle). This means Claude may receive two messages in rapid sequence: the full triage prompt typed in, then the orchestrator's NEXT_MESSAGE containing the same goal. The triage prompt may be corrupted (4096-char truncation in `sanitizeInitialPromptForTmux`) while the AutonomousDriver sends the full goal via headless orchestrator which then determines what to type.

**The key question**: does the `AutonomousDriver` know it should not also send the triage prompt via tmux? The `SessionDriver` handles the tmux side (using `inst.InitialPrompt` field or falling back to "Please proceed..."). `CreateDirectorySession` sets `Prompt` (not `InitialPrompt`) on the instance (line 588). `SessionDriver` checks `inst.InitialPrompt` (line 95 of `session_driver.go`) — if empty, it falls back to the generic `driverInitialPrompt` (`"Please proceed with the task described in your instructions."`). Since `CreateDirectorySession` sets `Prompt` but not `InitialPrompt`, the session driver will inject the generic prompt, not the triage prompt.

This means **the triage prompt currently reaches Claude only through the AutonomousDriver's NEXT_MESSAGE injection** (the orchestrator LLM deciding what message to type, after seeing `inst.Prompt` as its `goal`). The `SessionDriver` will type a generic "Please proceed..." message first.

**Risk**: Claude sees "Please proceed with the task described in your instructions" before any system prompt or context about triage is present in the terminal (because `AppendSystemPrompt` is not set in `CreateDirectorySession`). Claude may not have any context about what to do, leading it to wait for more instructions or respond with a generic acknowledgment. The AutonomousDriver's first turn message then provides the real context, but only after the first round-trip through the headless LLM.

### P5 — `kill stale tmux session` race (LOW-MEDIUM)

`TriggerTriage` calls `KillTmuxSessionByTitle` before spawning (line 1157). If the kill command races with tmux creating a new session of the same name (from a concurrent re-trigger call that bypassed the orphan guard), the new session could be killed immediately. The orphan guard is under no lock relative to the tmux kill call.

---

## 3. Headless Pool Failure Modes

### P6 — Claude binary not found (HIGH for fresh deployments)

`NewPool` calls `exec.LookPath("claude")`. If the claude CLI is not installed in the server's PATH (common in CI or containerized deployments), `headlessPool` is nil. `StartAutonomousDriverWithTimeout` silently returns early (line 731). `CreateDirectorySession` sets `oneShot=false` (because `useAutonomous` was true), so the session is interactive mode and the session driver waits indefinitely for someone to type commands. Sessions will sit idle forever with no error surfaced to the operator other than a log warn.

### P7 — First-call JSON parse failure rotates session, losing context (MEDIUM)

`caller.go` line 264: if the first `claude -p --output-format json` response is not valid JSON (e.g. claude printed a warning before JSON, or output was truncated by OS pipe buffer), the pool records an error, potentially trips the circuit breaker, and sends `StreamChunk{Err: ...}`. The orchestrator receives an error from `CallBlockingWithOptions`, logs a warning, and **breaks the loop** (line 211 in `autonomous_driver.go`):

```go
if err != nil {
    log.Warn(...)
    break  // ← exits loop, fires stuck completion
}
```

A single transient LLM error immediately terminates the driver. There is no retry within the driver loop for LLM errors.

### P8 — Malformed NEXT_MESSAGE/DONE response — infinite continue loop (MEDIUM)

`parseOrchestrationResponse` returns an error for any response not starting with `NEXT_MESSAGE:` or `DONE:`. On parse error the driver does `continue` (line 218), consuming a turn counter slot but not incrementing `turnCount`. Since `continue` goes back to the start of the loop and `turnCount` IS incremented by the `for` loop, each malformed response consumes one of the 20 turns without making progress. With a max of 20 turns, 20 malformed responses exhaust the turn budget and report `stuck`.

### P9 — Pool concurrency semaphore contention (LOW)

The pool defaults to `MaxConcurrentSessions=5`. During a triage run that spawns 4 parallel subagents (the triage prompt instructs Claude to run 4 subagents), each subagent calls the MCP server which does not use the headless pool. However, the `AutonomousDriver` itself uses a per-session `FeatureKey` (`autonomous_fix-<uuid[:8]>`), so concurrent triage sessions could contend on the semaphore. At 5 concurrent sessions max and multiple triage sessions running simultaneously, new triage drivers may block waiting for a pool slot.

---

## 4. Orchestration LLM Correctness

### P10 — Orchestrator sends a summary of the goal, not the full prompt (HIGH)

The orchestrator system prompt is:
```
You are an orchestrator directing a Claude Code session toward a goal.
Reply with exactly one of:
  NEXT_MESSAGE: <message to inject into the session>
  DONE: <reason the goal is complete>
```

The `goal` passed is `inst.Prompt` — the full multi-page triage prompt including all 5 steps, research directory paths, and the `submit_triage_result` schema. The orchestrator LLM must:
1. Read the full goal
2. On turn 1, decide to inject the full triage prompt verbatim as `NEXT_MESSAGE`

However, the orchestrator may paraphrase, summarize, or omit critical parts of the goal (e.g., the exact paths for research files, the `item_id` value, the `submit_triage_result` schema). If the orchestrator summarizes rather than passes the full prompt, the triage Claude session will not have the exact paths and item_id it needs to call `submit_triage_result` correctly.

**Mitigations needed**: The orchestrator should be instructed to pass the goal verbatim on turn 1, or the triage prompt should be injected via `--append-system-prompt` instead of through the orchestrator.

### P11 — Orchestrator signals DONE prematurely (HIGH)

The orchestrator sees only `tail` (last 80 lines × 120 chars = 9600 bytes max) of the session's terminal output. The triage session involves:
- 4 parallel subagent invocations (each runs a `claude -p` subprocess)
- Writing 6 files (stack.md, features.md, architecture.md, pitfalls.md, plan.md, validation.md)
- Calling `submit_triage_result`

Intermediate output in the terminal may look like the task is complete (e.g., research subagents finish and Claude prints a summary) before `submit_triage_result` has been called. The orchestrator may incorrectly signal DONE based on the terminal tail showing "research complete" messages without having verified the MCP tool call succeeded.

**Risk**: The orchestrator fires `onAutonomousDriverComplete` with `Done=true`, which transitions the item to `BacklogStatusReady`, but no `triage_result` was ever stored in the DB.

### P12 — Turn timeout fires during long subagent execution (MEDIUM)

After injecting a NEXT_MESSAGE, `waitForIdle` has a 5-minute timeout:
```go
turnCtx, turnCancel := context.WithTimeout(ctx, 5*time.Minute)
idleReached := waitForIdle(turnCtx, statusCh, d.controller)
turnCancel()
```

The triage task involves running 4 parallel subagents. Each subagent is a `claude -p` subprocess that may take several minutes. The outer triage session will not be "idle" (at the readline prompt) until all subagents complete. If 4 subagents each take 2 minutes, the total wait is ~8 minutes — exceeding the 5-minute per-turn idle timeout.

When the timeout fires, `waitForIdle` returns false and the driver logs a warning but proceeds to the next turn immediately (`idleReached = false; ... proceed anyway`). The next turn will call the orchestrator which sees a partially-completed terminal. The orchestrator may then inject another message mid-execution, potentially confusing the triage Claude session.

---

## 5. MCP Tool Accessibility

### P13 — `submit_triage_result` requires `STAPLER_SESSION_UUID` in env (HIGH)

The `submitTriageResult` handler calls `callerSessionUUID(ctx)` which reads `STAPLER_SESSION_UUID` from the MCP request context. This env var must be set in the Claude Code subprocess environment. In `CreateDirectorySession`, `MCPServerURL` is passed but `STAPLER_SESSION_UUID` is not explicitly set in the environment.

Checking instance launch: `filteredEnv()` in `runner.go` strips most env vars, only allowing `CLAUDE_*`, `ANTHROPIC_*`, `HOME`, `PATH`, etc. `STAPLER_SESSION_UUID` would only be present if it starts with one of the allowed prefixes — it does not. However, Claude Code itself injects `STAPLER_SESSION_UUID` via the MCP server connection protocol (the MCP server URL carries session context). This needs verification that the env injection path works correctly for sessions spawned with `oneShot=false` and an MCP server URL.

### P14 — MCP server URL not set or wrong port (MEDIUM)

`CreateDirectorySession` uses `s.mcpServerURL` (line 592). If this is not wired (nil or empty string) during server startup, the claude subprocess will not have access to any MCP tools including `submit_triage_result`. The session will run but be unable to submit triage results.

### P15 — Role check in `submitTriageResult` rejects non-triage sessions (LOW for normal flow, HIGH for re-trigger)

Line 465 checks `itemSession.SessionRole != "triage"`. If the same session UUID was previously used in a different role (or if the ItemSession record lookup uses the wrong session UUID), the submission is rejected with `ErrPermissionDenied`. On re-trigger, the orphan guard closes old ItemSessions — this should be correct, but race conditions during re-trigger could leave an `ended_at != nil` old record and the new triage session may not have had its ItemSession created yet when it calls `submit_triage_result`.

---

## 6. Tmux Session Reuse

### P16 — `KillTmuxSessionByTitle` sanitization mismatch (MEDIUM)

`stapleSquadTmuxName` in `session_service.go` replaces `.` and `:` with `_` and prepends `staplersquad_`. The actual tmux session creation in `instance_tmux.go` uses `toStaplerSquadTmuxNameWithPrefix`. If these two sanitization functions diverge (e.g., one strips spaces differently), the kill command targets the wrong session name and the stale session survives, causing the new `claude --append-system-prompt` invocation to re-attach to the existing stale session.

There is a comment in the kill function explicitly noting this must match the logic in `initTmuxSession`. This is a maintenance hazard — any future change to tmux name generation must update both paths.

### P17 — Kill race between orphan guard and re-trigger (MEDIUM)

The orphan guard (lines 1112–1131) and the tmux kill (line 1157) are separated by the artifact directory creation (line 1163). Between the orphan guard and the kill, another concurrent request could observe no open triage session and spawn its own. The kill at line 1157 would then kill the concurrently spawned session.

---

## 7. Goroutine Leak Risks

### P18 — Driver goroutine blocks on headless LLM call after session destroy (MEDIUM)

If the session is destroyed (user calls `DeleteSession`) while the `AutonomousDriver` is blocked in `CallBlockingWithOptions`, the driver will not exit until the LLM call completes or the `lifecycleCtx` is cancelled. `Stop()` cancels the driver's local context (passed to `CallBlockingWithOptions`), which does propagate to the subprocess runner — so cancellation is handled. However:

1. `DeleteSession` does not call `StopDriverForSession`.
2. The `driverRegistry` keyed by session title will retain a reference to the driver.
3. If the session is recreated with the same title (re-trigger), `registerDriver` will overwrite the old entry without calling `Stop()` on it, leaving the old goroutine running against a destroyed session.

Checking `stopAndDeregisterDriver`: it is called only from `onAutonomousDriverComplete` and `StopDriverForSession` (called from the MCP `submit_triage_result`/`submit_review_verdict` belt-and-suspenders path). There is no hook from `DeleteSession` into `StopDriverForSession`.

### P19 — `onAutonomousDriverComplete` references dead instance (LOW)

`FindLiveInstance(instanceName)` is called by `onAutonomousDriverComplete`. If the instance was already removed from the live poller (e.g., DeleteSession fired concurrently), it returns nil and the callback logs a warning and returns. The backlog item status is never transitioned. The item remains at `idea` status even though triage ran to completion.

---

## 8. Triage Prompt Size

### P20 — Triage prompt is NOT subject to the 4096-char tmux limit (GOOD NEWS / VERIFY)

The triage prompt (`buildTriagePrompt`) is stored in `inst.Prompt`. In `CreateDirectorySession`, the session is started with `Prompt: prompt` but **not** `InitialPrompt: prompt`. The `SessionDriver` checks `inst.InitialPrompt` (line 95 of `session_driver.go`) — if empty, it uses the generic fallback `"Please proceed with the task described in your instructions."`. The 4096-char tmux truncation in `sanitizeInitialPromptForTmux` applies only to `InitialPrompt` injections via the session driver.

The actual triage prompt is delivered via the `AutonomousDriver`'s `goal` field and transmitted through the orchestrator LLM's NEXT_MESSAGE, which is then typed into the session via `SendCommandImmediate`. There is no size limit on `SendCommandImmediate`.

However, `buildOrchestrationPrompt` limits the session `tail` to 9600 bytes but places no limit on `goal` size. The `goal` is the full triage prompt (~3–5 KB). The orchestrator LLM receives a large prompt which may cause increased latency but should not fail.

### P21 — The `goal` in NEXT_MESSAGE is injected into tmux as typed text (MEDIUM)

When the orchestrator returns `NEXT_MESSAGE: <full triage prompt>`, the driver calls `controller.SendCommandImmediate(nextMsg + "\r")`. This types the message via tmux `send-keys`. Large messages (thousands of characters) typed via tmux may overflow tmux's internal paste buffer or cause PTY input latency issues. Claude Code may parse the large typed input incorrectly (e.g., treating embedded newlines as multiple commands, though the `sanitizeInitialPromptForTmux` concern about newlines does NOT apply here — `SendCommandImmediate` does not go through that sanitizer).

The orchestrator system prompt instructs it to send "a message to inject", implying it should craft a short natural-language instruction, not paste the entire prompt verbatim. The full triage prompt (with embedded paths, markdown headers, step-by-step instructions) typed as raw text input via tmux would likely not be interpreted correctly by Claude Code's readline — it would appear as user input to be analyzed, not as instructions already in the system context.

**Root cause**: The triage prompt should be injected as a system prompt (`--append-system-prompt`) at session launch, not typed via tmux. The AutonomousDriver should then give a short "begin" instruction, not re-inject the full goal.

---

## Summary of Critical Risks

| Priority | ID | Risk | Likely Impact |
|---|---|---|---|
| CRITICAL | P10 | Orchestrator paraphrases goal instead of passing full prompt | Triage incomplete, wrong paths, missing item_id |
| CRITICAL | P11 | Orchestrator signals DONE before submit_triage_result called | Item transitions to ready with no triage data |
| HIGH | P4 | SessionDriver injects generic prompt before AutonomousDriver runs | Claude starts with no context, wastes turns |
| HIGH | P1 | Status channel drops idle events | Startup timeout fires, driver reports stuck |
| HIGH | P2 | Idle race before listener registered | waitForIdle blocks indefinitely |
| HIGH | P6 | headlessPool nil if claude binary missing | Session sits idle forever, no error surfaced |
| HIGH | P12 | Per-turn 5-min idle timeout fires during subagent execution | Orchestrator injects mid-execution, confuses Claude |
| MEDIUM | P7 | LLM transient error immediately breaks driver loop | Triage abandoned on first error |
| MEDIUM | P13 | STAPLER_SESSION_UUID not in MCP subprocess env | submit_triage_result rejects all calls |
| MEDIUM | P18 | Driver goroutine not stopped on session delete | Registry leak, goroutine against dead instance |
| MEDIUM | P21 | Full goal typed via tmux, not injected as system prompt | Claude interprets prompt as user text, not instructions |
| LOW | P19 | onAutonomousDriverComplete finds dead instance | Item status not transitioned after successful run |

---

## Recommended Fixes

1. **Inject triage prompt via `--append-system-prompt`** (not via orchestrator NEXT_MESSAGE): Set `AppendSystemPrompt: triagePrompt` in `CreateDirectorySession` opts for triage/review sessions. The orchestrator's first message should be a short "begin" directive.

2. **Guard `isOneShot` sessions from `StartSessionDriver` prompt injection**: When `autonomousStarter != nil` and session has `backlog:triage` tag, skip the SessionDriver prompt injection (or use `InitialPrompt=""` so only the generic "proceed" message fires, which is less harmful).

3. **Add retry on LLM error** in the driver loop: Catch transient errors and retry up to 3 times before breaking.

4. **Wire `StopDriverForSession` into `DeleteSession`** to prevent goroutine leaks.

5. **Extend per-turn idle timeout** for triage sessions: 5 min is too short for 4-parallel-subagent runs; use `WithStartupTimeout(10*time.Minute)` and a per-turn timeout of 15 minutes.

6. **Add structured log events** for driver lifecycle: `driver.started`, `driver.turn_injected`, `driver.idle_timeout`, `driver.done`.
