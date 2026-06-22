# Features Research: backlog-triage-autonomous

**Date**: 2026-06-22
**Scope**: How the existing autonomous infrastructure works, what the triage path does differently, and what edge cases the fix must handle.

---

## 1. How the Review Gate Headless Path Differs from Triage

### Review gate (headless path)
- Defined in `session/backlog_lifecycle.go` (`spawnReviewGate`)
- Triggered on `EventExited` from a work session (`SessionRoleWork`)
- Uses `headless.Pool.CallBlocking` — calls the LLM **directly**, without spawning a tmux session
- The LLM receives a single JSON-output prompt (`BuildHeadlessReviewPrompt`), produces a verdict, and returns in one shot
- Creates a synthetic `ItemSession` with `SessionRole=review` and a `ReviewVerdict` in one atomic write via `CreateItemSessionWithVerdict`
- No autonomous driver involved — the "session" never runs in tmux at all

### Triage autonomous path
- Defined in `server/services/backlog_service.go` (`TriggerTriage`)
- Spawns a **real tmux session** via `CreateDirectorySession` with `oneShot=false` (when autonomous starter is wired) and `hidden=true`
- Then calls `StartAutonomousDriverWithTimeout(inst, 5*time.Minute)` on the spawned instance
- The autonomous driver is supposed to inject orchestration prompts via `headlessPool.CallBlockingWithOptions` and forward them to the running claude process via `SendCommandImmediate`
- Triage must write several files (`research/*.md`, `plan.md`, `validation.md`) and call `submit_triage_result` MCP tool — this requires real tool access that headless `-p` mode cannot provide

**Key difference**: Review gate uses a stateless headless LLM call; triage requires a live claude session with full tool access. This is why triage cannot use the headless path directly.

---

## 2. How Work Sessions Use StartAutonomousDriverForInstance

Work sessions (spawned via `SpawnSessionFromItem`) use `StartAutonomousDriverForInstance` (no timeout override):

```go
// backlog_service.go:941
if req.Msg.Autonomous && s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

`StartAutonomousDriverForInstance` (`session_service.go:711`) creates an `AutonomousDriver` with `inst.Prompt` as the goal and a default 60-second startup timeout.

**Critical observation**: `CreateDirectorySession` sets `opts.Prompt = prompt` on the instance (line 588), which flows to `Instance.Prompt`. The autonomous driver uses `inst.Prompt` as the `goal` argument to `buildOrchestrationPrompt`. The driver's LLM then decides what message to inject into the tmux session on each turn.

Work sessions are not obviously broken — they work because:
1. The session driver (`session_driver.go`) injects the `InitialPrompt` if set, or falls back to `"Please proceed with the task described in your instructions."` (line 32)
2. The triage prompt is in `inst.Prompt` but NOT in `inst.InitialPrompt`
3. Work sessions receive a system prompt via CLAUDE.md and `.claude/backlog-context.md` that provides context, so the fallback "Please proceed..." actually works

**Triage sessions are broken because**: The triage prompt (the full 500-word instruction block) is stored in `inst.Prompt` but `CreateDirectorySession` does not set `InitialPrompt`. The session driver has no prompt to inject, so claude sees only "Please proceed with the task described in your instructions." — which has no content about triage tasks, artifact paths, or which MCP tool to call.

---

## 3. What the oneShotFallback Path Does

When `autonomousStarter` is nil (e.g., in test environments or if headlessPool failed to initialize), `TriggerTriage` falls back to `oneShot=true`:

```go
// backlog_service.go:1181-1187
useAutonomous := s.autonomousStarter != nil
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, !useAutonomous /*oneShot*/, true /*hidden*/)
if useAutonomous {
    s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
}
```

**OneShot mode** (`-p` flag in claude CLI):
- Session calls `claude -p <prompt>` — the full triage prompt is passed as a CLI argument
- Claude executes the task and exits; the session transitions to `Stopped`
- `BacklogLifecycleListener.onSessionExited` fires, but since `SessionRole=triage` (not `work`), it only records the end time and returns early (line 179: `if is.SessionRole != SessionRoleWork { return }`)
- No status transition happens on completion

The oneShot fallback was the original design before autonomous mode was added. It is not "deprecated" per se — it is the graceful degradation path — but it also does not drive the `idea→ready` transition because `onSessionExited` skips non-work roles. This means even the oneShot path is broken for status transitions (the item would stay at `idea` indefinitely).

**The oneShot path does correctly deliver the prompt to claude**: `session/instance.go` passes `Prompt` to the tmux session launch command when `OneShot=true` (via the `-p` flag in `initTmuxSession`). So the agent actually receives the triage instructions in oneShot mode.

---

## 4. Edge Cases the Design Must Handle

### 4a. Re-triage after failure
- `TriggerTriage` has an orphan-aware guard (lines 1112-1132) that checks for open triage ItemSessions
- An orphaned session is detected by: `started_at == NULL` (never confirmed running), `!IsSessionLive(uuid)`, or `status != idea`
- Orphaned sessions are tombstoned (end time set) and the previous tmux session is killed via `KillTmuxSessionByTitle`
- The stale tmux kill (line 1156) targets the deterministic name `"triage:" + slug` to prevent session reuse
- Re-triage on a `ready` item moves it back to `idea` first (line 1136)

### 4b. Orphaned sessions after server restart
- After a restart, in-memory instance tracking is lost
- `IsSessionLive` returns false for any session UUID not in the live poller
- Since `neverStarted = is.StartedAt == nil`, an unstarted triage session from a prior run is always tombstoned correctly
- Started-but-now-dead sessions are also tombstoned because `sessionStopper.IsSessionLive` returns false

### 4c. Triage on items without AC
- `buildTriagePrompt` includes AC section only when `item.AcceptanceCriteria != ""` (line 1219)
- No guard in `TriggerTriage` requires AC — items without AC can be triaged
- The triage prompt instructs the agent to add clarifying questions as suggestions with `rationale: "question"` if AC is missing

### 4d. Triage on items at wrong status
- Status guard at line 1094: only `idea` or `ready` items can be triaged
- `in_progress`, `review`, `done`, `archived` items are rejected with `CodeFailedPrecondition`

### 4e. Session startup race: autonomous driver starts before idle
- The driver waits for the first idle signal with a 5-minute startup timeout (passed via `WithStartupTimeout`)
- Triage sessions spawn parallel subagents; these may keep the session busy for extended periods
- The 5-minute startup timeout is appropriate but the 5-minute-per-turn timeout (`5*time.Minute` in `waitForIdle`) may be too short for subagent fan-out

### 4f. headlessPool nil at autonomous driver start
- Both `StartAutonomousDriverForInstance` and `StartAutonomousDriverWithTimeout` check `headlessPool == nil` and log a warning, returning early
- No error propagates to the caller; the session is created but no driver runs
- This means the triage session will be stuck at the `>` prompt indefinitely (the current bug symptom)

### 4g. Controller nil at driver start
- `NewAutonomousDriver` stores `inst.GetController()` at construction time
- In `Start()`, it re-fetches: `d.controller = d.inst.GetController()`
- If the controller is nil (session not yet started, or not wired to status manager), `Start()` returns an error

---

## 5. What onAutonomousDriverComplete Does for the Triage Role

`onAutonomousDriverComplete` (`session_service.go:3548`) is the completion callback registered on every autonomous driver.

For triage sessions specifically (lines 3586-3604):

```go
case session.SessionRoleTriage:
    if !outcome.Done {
        // Triage was interrupted/stuck — notify operator, leave item at 'idea'
        s.eventBus.Publish(events.NewNotificationEvent(..., "Triage did not complete", ...))
        return
    }
    toStatus = session.BacklogStatusReady
    expectedStatus = string(session.BacklogStatusIdea)
```

When `outcome.Done = true` (the LLM orchestrator returned `DONE: <reason>`), the callback transitions the item from `idea` to `ready` using an optimistic-lock precondition check.

When `outcome.Done = false` (stuck/max turns), the callback fires a failure notification and leaves the item at `idea`.

**Important**: The `DONE` signal is generated by the orchestrator LLM (in `headlessPool`) when it observes `submit_triage_result` has been called in the session output. The orchestrator looks at the session tail and decides whether the goal is complete. This means:
1. The session must successfully call `submit_triage_result`
2. The orchestrator LLM must detect this in the terminal tail
3. Only then does `onAutonomousDriverComplete` fire with `Done=true`

`submit_triage_result` itself does NOT transition the item — it only persists the triage result JSON and fires a notification. The `idea→ready` transition is entirely driven by `onAutonomousDriverComplete`.

---

## 6. The submit_triage_result MCP Tool

**Location**: `server/mcp/tools_backlog.go:436`

**What it does**:
1. Validates caller has `STAPLER_SESSION_UUID` set in environment
2. Verifies the caller session is linked to the item with `SessionRole=triage`
3. Parses `summary`, `suggestions` (optional), `tasks` (optional, max 12), `plan_artifact_path` (optional)
4. If `plan_artifact_path` is set, updates `BacklogItem.PlanArtifactsPath` via `UpdateBacklogItem`
5. Persists triage result JSON on the ItemSession via `UpdateItemSessionTriageResult`
6. Publishes a `NOTIFICATION_TYPE_INPUT_REQUIRED` event via the event bus (if wired)
7. Returns a success text to the caller

**What it does NOT do**:
- Does not transition the item status (no `TransitionBacklogItemStatus` call)
- Does not stop the AutonomousDriver (no `reviewStopper` equivalent for triage)
- The item stays at `idea` until `onAutonomousDriverComplete` fires with `Done=true`

**Who calls it**: The claude agent running in the triage session. For autonomous triage, the agent must call this as its last step. The autonomous driver then sees this in the terminal tail, the orchestrator LLM returns `DONE:`, and `onAutonomousDriverComplete` transitions the item.

**Missing: TriageCompletionSignaler**. Unlike review sessions (which call `reviewStopper.StopDriverForSession` from `submit_review_verdict`), triage has no equivalent hook to signal the driver when `submit_triage_result` is called. The driver must infer completion from the terminal tail on its next orchestration turn. This adds one extra LLM round-trip after completion.

---

## Root Cause Summary

The primary bug is in `CreateDirectorySession` (`session_service.go:579`): it sets `Prompt` but NOT `InitialPrompt`. The session driver (`session_driver.go`) only injects `InitialPrompt` into the tmux pane. Since `InitialPrompt` is empty for autonomous triage sessions, the driver injects the fallback `"Please proceed with the task described in your instructions."` — which has no triage context, no artifact paths, and no mention of `submit_triage_result`.

The `AutonomousDriver` receives `inst.Prompt` as the `goal` argument and uses it to construct the orchestrator LLM prompt. On turn 1, the orchestrator sees an idle session with `"Please proceed..."` as the only output and should in theory inject the triage prompt — but this requires the orchestrator to extract the full triage goal from `<goal>` and reformulate a concrete next message. This might work intermittently but is fragile.

The clean fix is to set `InitialPrompt = triagePrompt` when creating the triage session (in `TriggerTriage`) so the session driver injects the actual triage task directly on startup, before the autonomous driver's first turn.

Additionally, `submit_triage_result` should call `StopDriverForSession` (like `submit_review_verdict` calls `reviewStopper`) so the driver exits immediately after the agent completes, rather than waiting for the next orchestration turn to detect `DONE`.
