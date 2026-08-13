# Architecture Research: Triage Autonomous Migration

## Overview

Two bugs to fix:

1. `WatchSessions` sends hidden sessions (`inst.Hidden==true`) in initial snapshots without filtering
2. `TriggerTriage` and `TriggerReReview` use `oneShot: true` but should use `AutonomousDriver` for multi-turn orchestration

---

## Bug 1: WatchSessions Hidden Session Leak

### Current State

**`ListSessions`** (`server/services/session_service.go` lines 797-800) correctly filters:
```go
// Exclude hidden (system/background) sessions unless explicitly requested
if inst.Hidden && !req.Msg.IncludeHidden {
    continue
}
```

`ListSessionsRequest` (`proto/session/v1/session.proto` lines 416-419) has the field:
```proto
bool include_hidden = 6;
```

**`WatchSessions`** (`server/services/session_service.go` lines 1623-1703) has NO hidden filtering.

In the initial snapshot loop (lines 1655-1669), it only filters by `CategoryFilter` and `StatusFilter`:
```go
for _, inst := range instances {
    if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
        if inst.Category != *req.Msg.CategoryFilter {
            continue
        }
    }
    if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
        if adapters.StatusToProto(inst.Status) != *req.Msg.StatusFilter {
            continue
        }
    }
    if err := stream.Send(createInitialSnapshotEvent(inst)); err != nil { ... }
}
```

In the real-time event loop (lines 1682-1701), the same two filters are applied to `event.Session`, but there is also no hidden check.

**`WatchSessionsRequest`** (`proto/session/v1/session.proto` lines 595-606) currently has NO `include_hidden` field:
```proto
message WatchSessionsRequest {
  optional string category_filter = 1;
  optional SessionStatus status_filter = 2;
  uint64 after_seq = 3;
}
```

### Root Cause

`WatchSessions` was written before the `Hidden` field was introduced to sessions. `TriggerTriage` and `TriggerReReview` create sessions with `hidden: true` (via `CreateDirectorySession(... true /*hidden*/)`) but the watch stream does not filter them, so UI clients see these background sessions.

### Proposed Fix

**Option A (minimal, no proto change):** Add a hidden filter to `WatchSessions` that always excludes hidden sessions. This mirrors the default behavior of `ListSessions` (where `include_hidden` defaults to false).

**Option B (proto change):** Add `bool include_hidden = 4` to `WatchSessionsRequest`, then filter accordingly.

Option A is correct for the immediate bug: no callers of `WatchSessions` want to see hidden sessions. A proto change can be deferred until there is an actual use case.

#### Changes Required (Option A)

**File: `server/services/session_service.go`**

Initial snapshot loop — add after the existing status filter check (after line 1664):
```go
// Exclude hidden (system/background) sessions from the watch stream.
// Hidden sessions (triage, review gates) are system sessions not meant
// for the UI session list. Mirrors the default behavior of ListSessions.
if inst.Hidden {
    continue
}
```

Real-time event loop — add inside the event loop (after line 1694, before the convert call):
```go
// Exclude hidden sessions from real-time events too.
if event.Session != nil && event.Session.Hidden {
    continue
}
```

Note: `EventSessionDeleted` events carry `SessionID` only (not `Session`), so hidden-delete events will still be sent. This is acceptable: the client ignores sessions it never saw in its local state.

If Option B (proto field) is chosen, the steps are:
1. Add `bool include_hidden = 4;` to `WatchSessionsRequest` in `proto/session/v1/session.proto`
2. Run `make generate-proto`
3. In the initial snapshot loop: `if inst.Hidden && !req.Msg.IncludeHidden { continue }`
4. In the real-time event loop: `if event.Session != nil && event.Session.Hidden && !req.Msg.IncludeHidden { continue }`

---

## Bug 2: TriggerTriage / TriggerReReview Should Use AutonomousDriver

### Current State

**`TriggerTriage`** (`server/services/backlog_service.go` line 1179):
```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, true /*oneShot*/, true /*hidden*/)
```

**`TriggerReReview`** (`server/services/backlog_service.go` line 1533):
```go
inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
    []string{"backlog:review"}, true /*oneShot*/, true /*hidden*/)
```

Both pass `oneShot: true`. The `CreateDirectorySession` method (`session_service.go` line 577) propagates this to `InstanceOptions.OneShot`.

In `session/instance_tmux.go` line 65-66, `OneShot` causes:
```go
if i.OneShot && strings.Contains(program, "claude") {
    program = program + " -p --output-format json"
```

So `claude -p` is run: a single-shot non-interactive call that reads stdin, runs once, and exits. The session has **no tool access** (confirmed by `backlog_lifecycle.go` line 278: "Use JSON-output prompts because headless claude -p has no tool access").

This means `submit_triage_result` and `submit_review_verdict` MCP tools **cannot be called** from a oneShot session. The current triage prompt (lines 1226-1272) instructs Claude to call the MCP tool, but this contradicts the `-p` flag behavior.

The `BacklogService` already has `autonomousStarter AutonomousDriverStarter` wired at `server/dependencies.go` line 757:
```go
backlogSvc.SetAutonomousDriverStarter(sessionService)
```

And `SpawnSessionFromItem` already uses it correctly (line 940-942):
```go
if req.Msg.Autonomous && s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

### What AutonomousDriver Provides

`session/autonomous_driver.go` implements an external LLM orchestrator that:
1. Waits for the session to become idle
2. Calls `headlessPool.CallBlockingWithOptions` with a system prompt + session tail
3. Parses `NEXT_MESSAGE: <text>` or `DONE: <reason>` responses
4. Injects next messages via `SendCommandImmediate`
5. Loops up to `maxTurns` (default 20)
6. On `DONE`, extracts PR URL, fires `CompletionCallback`

This is the correct model for triage sessions: Claude needs tool access to write files and call MCP tools, which requires an interactive tmux session (not `-p` mode).

### Proposed Fix

Remove `oneShot: true` from `TriggerTriage` and `TriggerReReview` calls. Add `AutonomousDriver` startup after the session is spawned.

#### `TriggerTriage` Changes (`backlog_service.go`)

Replace lines 1178-1183:
```go
// Before:
title := "triage:" + slug
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, true, true)
if err != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn triage session: %w", err))
}
```

With:
```go
// After:
title := "triage:" + slug
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, false /*oneShot=false: needs tool access*/, true /*hidden*/)
if err != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn triage session: %w", err))
}

// Start AutonomousDriver for multi-turn orchestration.
// Degrades gracefully when headlessPool is nil (no pool = no orchestration, session runs unguided).
if s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

#### `TriggerReReview` Changes (`backlog_service.go`)

Replace lines 1532-1535:
```go
// Before:
slug := slugify(item.Title)
title := "re-review:" + slug
inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
    []string{"backlog:review"}, true, true)
if spawnErr != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
}
```

With:
```go
// After:
slug := slugify(item.Title)
title := "re-review:" + slug
inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
    []string{"backlog:review"}, false /*oneShot=false: needs tool access*/, true /*hidden*/)
if spawnErr != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
}

// Start AutonomousDriver for multi-turn orchestration.
if s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

### Graceful Degradation When headlessPool is nil

`StartAutonomousDriverForInstance` (`session_service.go` lines 673-676) already guards:
```go
func (s *SessionService) StartAutonomousDriverForInstance(inst *session.Instance) {
    if s.headlessPool == nil {
        log.Warn("[SessionService] StartAutonomousDriverForInstance: headlessPool is nil", "session", inst.Title)
        return
    }
```

When `headlessPool` is nil (claude binary not found at startup), the triage session will still spawn and run as a normal interactive session. Claude will receive the prompt on startup, perform triage autonomously using its MCP tool access, and call `submit_triage_result` when done. The AutonomousDriver only provides orchestration insurance (re-injection if Claude stalls); it is not required for the happy path.

This matches the degradation contract in `NewBacklogService` and `SpawnSessionFromItem`.

---

## Completion Signal Design

### How AutonomousDriver Currently Detects Completion

`AutonomousDriver.run` (`autonomous_driver.go` lines 136-233):
1. Reads session tail via `d.inst.Preview()`
2. Calls orchestrator LLM: "Turn N/M. Reply with NEXT_MESSAGE or DONE"
3. `parseOrchestrationResponse` looks for the literal prefix `DONE:` or `NEXT_MESSAGE:`
4. On `DONE`, calls `fireCompletion` with `AutonomousDriverOutcome{Done: true, Reason: ..., PRUrl: ...}`

The LLM decides when to emit `DONE` by reading the session tail and inferring that the goal is achieved.

### How submit_triage_result Signals Completion

`submit_triage_result` (`server/mcp/tools_backlog.go` lines 402-535) currently:
1. Persists `triage_result` JSON to the `ItemSession` record
2. Updates `plan_artifacts_path` on the `BacklogItem`
3. Publishes a `NotificationEvent` to the EventBus

It does NOT send any signal to the AutonomousDriver. However, when triage completes, Claude calls `submit_triage_result` as its final action (per the prompt). After this call, Claude will naturally become idle. The orchestrator LLM in the AutonomousDriver will read the session tail, see the successful `submit_triage_result` call, and emit `DONE: triage complete`.

No explicit callback hook is required — the LLM-based detection is sufficient. However, there is an optional improvement:

### Optional: Explicit DONE Signal from MCP Tool

If faster/more reliable completion detection is needed, `submit_triage_result` could write `DONE: triage complete` to the session's terminal. This would let the AutonomousDriver detect completion via the session tail pattern rather than waiting for the next LLM orchestration turn.

Implementation sketch (in `tools_backlog.go`, after the notification publish):
```go
// Optional: inject DONE signal into session terminal for AutonomousDriver to detect.
// This is belt-and-suspenders — the orchestrator LLM will also detect completion
// by reading the successful submit_triage_result call in the session tail.
```

The current design without this is fine. The AutonomousDriver sees the `submit_triage_result` MCP call in the terminal output during its next polling cycle, the orchestrator LLM will read it and return `DONE:`, and `fireCompletion` will be called.

### Completion Callback Registration

`StartAutonomousDriverForInstance` (`session_service.go` lines 679-682) already registers:
```go
driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)
```

`onAutonomousDriverComplete` handles session cleanup, notification, and lifecycle transitions. No new hook is needed.

---

## OneShot vs AutonomousDriver Decision

### OneShot (`claude -p --output-format json`)

**How it works:**
- Sets `InstanceOptions.OneShot = true`
- In `instance_tmux.go` line 65-66: appends `-p --output-format json` to the claude command
- Claude reads the initial prompt from stdin, runs once in print mode, outputs JSON to stdout, and exits
- **No MCP server connection** — Claude `-p` does not establish an MCP transport
- Session exits automatically; `BacklogLifecycleListener` fires on exit

**Use cases only OneShot serves:**
- Batch/pipeline scenarios where structured JSON output is needed from the session process itself
- Contexts where no tool access is required and simple text generation suffices
- `RunOneShot` RPC (`session_service.go`) which explicitly reads JSON from `--output-format json`
- The headless pool itself uses `claude -p` via `headless.Pool.CallBlockingWithOptions` (internal to pool, not exposed as a session)

**Limitation:** No MCP tool access, so Claude cannot call `submit_triage_result` or `submit_review_verdict`.

### AutonomousDriver

**How it works:**
- Session runs with `OneShot = false` (normal interactive Claude with full tool access)
- External orchestrator loop polls session state and injects prompts via `SendCommandImmediate`
- Detects completion via LLM-based response parsing (`DONE:` prefix)
- Runs up to `maxTurns` (default 20) before giving up
- Requires `headlessPool` to be non-nil

**Use cases only AutonomousDriver serves:**
- Multi-turn workflows where Claude needs to call MCP tools (triage, review)
- Any session where Claude needs to persist artifacts, call RPCs, or make structured decisions via tools
- Completion detection based on session output (not just process exit)

### Verdict: Retain Both, Fix the Triage/Review Usage

They are **complementary, not duplicative**:

| Dimension | OneShot (`-p`) | AutonomousDriver |
|---|---|---|
| MCP tool access | No | Yes |
| Structured JSON output from process | Yes | No |
| Multi-turn | No | Yes |
| Requires headlessPool | No | Yes |
| Session visible in UI | Configurable | Configurable |
| Completion detection | Process exit | LLM + turn limit |

**Correct assignments:**
- Triage sessions: `oneShot=false` + `AutonomousDriver` (needs `submit_triage_result` MCP call)
- Review sessions: `oneShot=false` + `AutonomousDriver` (needs `submit_review_verdict` MCP call)
- `RunOneShot` RPC: remains `oneShot=true` (explicit user request for single-turn JSON output)
- `headless.Pool` internal calls: remain `claude -p` (pool manages its own subprocess, not exposed as sessions)

---

## Summary of All Required Changes

### File: `proto/session/v1/session.proto`

No change required for the minimal fix (Option A for Bug 1).

If adding `include_hidden` to `WatchSessionsRequest` (Option B), add after field 3:
```proto
// When true, include hidden (system/background) sessions in the watch stream.
// Defaults to false — hidden sessions are excluded unless explicitly requested.
bool include_hidden = 4;
```
Then run `make generate-proto`.

### File: `server/services/session_service.go`

**Bug 1 fix — WatchSessions initial snapshot (after line 1664):**
```go
// Exclude hidden (system/background) sessions.
if inst.Hidden {
    continue
}
```

**Bug 1 fix — WatchSessions real-time events (after line 1694):**
```go
// Exclude hidden session events.
if event.Session != nil && event.Session.Hidden {
    continue
}
```

### File: `server/services/backlog_service.go`

**Bug 2 fix — TriggerTriage (line 1179): change `true` oneShot to `false`:**
```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, false /*oneShot*/, true /*hidden*/)
```

Add after the error check:
```go
if s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

**Bug 2 fix — TriggerReReview (line 1533): change `true` oneShot to `false`:**
```go
inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
    []string{"backlog:review"}, false /*oneShot*/, true /*hidden*/)
```

Add after the error check:
```go
if s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

---

## Key File Paths

- `server/services/session_service.go` — WatchSessions (line 1623), CreateDirectorySession (line 577), StartAutonomousDriverForInstance (line 673)
- `server/services/backlog_service.go` — TriggerTriage (line 1075), TriggerReReview (line 1396), SpawnSessionFromItem (line 858, canonical autonomous wiring example)
- `session/autonomous_driver.go` — full AutonomousDriver implementation
- `session/instance_tmux.go` — line 65: OneShot → `-p --output-format json` flag application
- `session/session_driver.go` — OneShot session exit handling
- `server/mcp/tools_backlog.go` — submitTriageResult (line 402), submitReviewVerdict (line 276)
- `proto/session/v1/session.proto` — WatchSessionsRequest (line 595), ListSessionsRequest.include_hidden (line 416)
- `server/dependencies.go` — headlessPool construction (line 435), backlogSvc wiring (line 755-757)
- `pkg/events/types.go` — Event struct, Session field carries `*session.Instance` with Hidden bool
