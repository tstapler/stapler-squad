# Architecture Research: backlog-triage-autonomous

**Date**: 2026-06-22  
**Codebase**: ssq-backlog_18bb73808fbc722d

---

## 1. Current Data Flow (with break points annotated)

### TriggerTriage → session spawn

**File**: `server/services/backlog_service.go:1074–1206`

```
TriggerTriage(req)
  → buildTriagePrompt(item, artifactAbsPath, slug)   // produces 700+ char prompt
  → useAutonomous = s.autonomousStarter != nil
  → CreateDirectorySession(ctx, title, path, triagePrompt,
        tags=["backlog:triage"],
        oneShot=!useAutonomous,   // oneShot=false when autonomous
        hidden=true)
  → if useAutonomous:
      s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
```

**Key insight — `triagePrompt` is passed as the `prompt` parameter of `CreateDirectorySession`.**

### CreateDirectorySession → InstanceOptions

**File**: `server/services/session_service.go:579–626`

```go
opts := session.InstanceOptions{
    Title:        title,
    Path:         path,
    Prompt:       prompt,          // ← triagePrompt goes here
    InitialPrompt: "",             // ← NOT SET (zero value)
    Tags:         tags,
    OneShot:      oneShot,
    ...
}
```

### InstanceOptions → tmux launch command

**File**: `session/instance_tmux.go:68–70`

```go
// Prompt is appended as a CLI argument to the claude binary
if i.Prompt != "" && (claudeSessionID == "" || i.OneShot) && strings.Contains(program, "claude") {
    program = fmt.Sprintf("%s %q", program, i.Prompt)
}
```

**So `Prompt` becomes a CLI positional argument**: `claude "You are a senior software architect..."`

In **oneShot** mode (`-p`): Claude reads the prompt directly and exits — this works.

In **autonomous** mode (not oneShot): `claude` is launched interactively. A positional argument after `claude` in interactive mode is **silently ignored** by Claude Code. The prompt never reaches Claude.

### session_driver.go: InitialPrompt injection

**File**: `session/session_driver.go:92–102`

```go
func runSessionDriver(inst *Instance, allowedPath string) {
    var initialPrompt string
    if inst.InitialPrompt != "" {         // ← checks InitialPrompt (not Prompt)
        sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
        if sanitized != "" {
            initialPrompt = sanitized
        }
    }
    runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, &retried)
}
```

When `initialPrompt == ""`, `sentInitial` is set to `true` immediately (line 127):

```go
sentInitial := initialPrompt == ""   // true → driver skips send step entirely
```

The driver handles startup dialogs and auto-approval, but **never types anything into the session**.

### AutonomousDriver: goal from inst.Prompt

**File**: `server/services/session_service.go:734`

```go
driver := session.NewAutonomousDriver(inst, s.headlessPool, inst.Prompt, 0, session.WithStartupTimeout(startupTimeout))
```

`inst.Prompt` is used as the `goal` for the orchestration LLM. The AutonomousDriver:
1. Waits for `statusCh` to receive `StatusIdle` within 5 minutes (startup timeout)
2. Sends `inst.Prompt` to the orchestrator LLM (`buildOrchestrationPrompt`)
3. LLM replies `NEXT_MESSAGE: <text>` → injected into tmux via `SendCommandImmediate`
4. Waits for idle again, then loops

**The critical question**: does Claude ever reach an idle state? Since Claude was launched without a prompt (Prompt is silently dropped in interactive mode), Claude starts up, shows the `>` prompt, and IS idle. So the AutonomousDriver should work — but only if `headlessPool != nil`.

---

## 2. Root Cause Analysis — Where Does It Break?

### Bug 1 (CONFIRMED): `triagePrompt` stored as `inst.Prompt`, not `inst.InitialPrompt`

`CreateDirectorySession` maps the `prompt` parameter to `InstanceOptions.Prompt`.  
`InstanceOptions.InitialPrompt` is always empty for backlog sessions.  
`session_driver.go` only injects `inst.InitialPrompt`.  
Result: **session_driver sends nothing**. Claude sees no task.

However, the AutonomousDriver uses `inst.Prompt` as its `goal`, which IS set. So the AutonomousDriver CAN call the orchestration LLM and inject a first message — IF the driver starts.

### Bug 2 (LIKELY): `headlessPool` nil → `StartAutonomousDriverWithTimeout` is a no-op

**File**: `session_service.go:729–732`

```go
func (s *SessionService) StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration) {
    if s.headlessPool == nil {
        log.Warn("[SessionService] StartAutonomousDriverWithTimeout: headlessPool is nil", "session", inst.Title)
        return   // ← silent early return
    }
```

If `headlessPool` is not wired at server startup, `useAutonomous = s.autonomousStarter != nil` is `true` (the interface is set), but the actual driver never starts. The session spawns as non-oneShot (interactive), Claude receives no prompt, and sits idle forever.

### Bug 3 (DESIGN FLAW): `inst.Prompt` is not injected into interactive Claude

In interactive mode (not `-p`), `claude "some prompt"` does NOT inject the prompt as an initial message. The positional argument is only meaningful in `-p` (one-shot) mode. So even if the AutonomousDriver does start, the triage prompt is sent via `NEXT_MESSAGE` from the orchestrator LLM which then parrots the triage instructions back — an indirect double-hop that:
- Adds latency (extra LLM call)
- Risks prompt truncation (orchestrator → `NEXT_MESSAGE` → tmux 4096-char limit)
- Is semantically wrong (orchestrator designed for steering, not kickoff)

### Bug 4 (IDLE DETECTION RACE): AutonomousDriver waits for idle on a fresh session

`waitForIdle` has a fast-path check: `if cc.IsIdle() { return true }`. A freshly started session may already be idle when the driver starts (Claude at `>` prompt). If the controller hasn't attached yet (status channel has no events), `waitForIdle` could block until the 5-minute startup timeout fires — then `fireCompletion(Stuck: true)` and the session is marked stuck without ever injecting the triage prompt.

---

## 3. Architecture Alternative Comparison

### A) Current: `oneShot=false` + AutonomousDriver injects via orchestration LLM

**Status**: Broken by Bug 2 (headlessPool nil) and Bug 3 (wrong CLI arg semantics).  
**Pros**: Driver can steer multi-step tasks, detect stuck sessions.  
**Cons**: Requires headlessPool; orchestrator LLM is a single-instruction model, not designed for complex multi-step triage.

### B) Hybrid: inject triage prompt via `InitialPrompt`, AutonomousDriver monitors for completion

**How it works**:
- Pass `triagePrompt` as `InitialPrompt` (not `Prompt`) → `session_driver` types it into the tmux pane at `>` prompt
- Pass `"Call submit_triage_result when done"` (or similar) as `goal` to AutonomousDriver
- AutonomousDriver waits for `submit_triage_result` to fire (or DONE signal from session output), not for orchestrator LLM output

**Pros**: Session receives the full prompt reliably within 30s. Simple to implement (change one parameter).  
**Cons**: `InitialPrompt` is sanitized to one line and limited to 4096 chars — the 700+ char triage prompt WILL be truncated at newlines (`\n → " "`). The prompt becomes a wall of text but should be functional.

**Best fix for immediate unblocking**: In `CreateDirectorySession`, when called in autonomous mode (or always for backlog sessions), also set `InitialPrompt = prompt` so session_driver injects it.

### C) Headless-only: entire triage as headless LLM call chain, no tmux

**How it works**: Replace the session spawn with `headlessPool.CallBlockingWithOptions` directly, writing artifact files via tool calls.

**Pros**: No idle detection, no tmux, no startup race, deterministic.  
**Cons**: Headless Claude `-p` has no MCP tool access — `submit_triage_result` would be unreachable. Requires a separate mechanism to record the result. Heavy engineering lift.

### D) `oneShot=true` + BacklogLifecycleListener on exit

**How it works**: Keep `oneShot=true` (current fallback path), pass `triagePrompt` as `Prompt` (already done), do NOT use AutonomousDriver. When session exits, `BacklogLifecycleListener.onSessionExited` fires.

**Problem**: `onSessionExited` only handles `SessionRoleWork` transitions. For triage, the only transition path is:
- `submit_triage_result` MCP tool → fired by triage session before exit → transitions `idea → ready` via `onAutonomousDriverComplete` (but AutonomousDriver is not started in oneShot mode)

Actually: `submit_triage_result` transitions status **directly** in the MCP handler (`tools_backlog.go`) by publishing a notification. The item status itself is transitioned by `onAutonomousDriverComplete` only in autonomous mode. In oneShot mode, the triage session exits, `BacklogLifecycleListener` records the end time (but ignores `SessionRoleTriage`), and nothing advances the item status.

Wait — re-read `tools_backlog.go:submitTriageResult`: it saves the triage result to the ItemSession but does NOT transition the item status to `ready`. That transition is in `onAutonomousDriverComplete` (autonomous mode only). So oneShot triage never advances the item to `ready` automatically.

**Verdict for D**: oneShot works for running triage but leaves the item stuck at `idea`. Requires separate plumbing to advance status.

---

## 4. Integration Points

### MCP: `submit_triage_result`

**File**: `server/mcp/tools_backlog.go:436–569`

- Validates caller via `STAPLER_SESSION_UUID` (injected via `--mcp-config` header `X-Stapler-Session-UUID`)
- Verifies `itemSession.SessionRole == "triage"` — if role mismatches, returns permission denied
- Saves triage result JSON to `ItemSession.TriageResult`
- Updates `BacklogItem.PlanArtifactsPath` if provided
- Publishes `NotificationEvent` (type `INPUT_REQUIRED`, priority `MEDIUM`) via EventBus
- **Does NOT transition item status** — that is left to `onAutonomousDriverComplete`

### BacklogLifecycleListener

**File**: `session/backlog_lifecycle.go`

- Listens for `EventStarted` → records `ItemSession.StartedAt`
- Listens for `EventExited` → records `ItemSession.EndedAt`
- Only drives status transitions for `SessionRoleWork` (not triage/review)
- For triage exits: **no status transition fires**

### onAutonomousDriverComplete (autonomous mode only)

**File**: `server/services/session_service.go:3548–3644`

For `SessionRoleTriage`:
- If `outcome.Done == true`: transitions `idea → ready` (with precondition)
- If `outcome.Done == false` (stuck): fires "Triage stuck" notification, leaves item at `idea`

The `DONE` signal from AutonomousDriver comes from the orchestrator LLM parsing session output. The orchestrator LLM must return `DONE: <reason>` based on seeing `submit_triage_result` output in the terminal tail.

---

## 5. EventStorming Table

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| BacklogItemCreated | auto-triage enabled | TriggerTriage | BacklogService (auto) |
| TriageSessionSpawned | TriggerTriage called | CreateDirectorySession | BacklogService |
| SessionStarted | instance.Start() completes | StartSessionDriver | SessionService |
| SessionStarted | instance.Start() completes | StartAutonomousDriverWithTimeout | SessionService (if headlessPool≠nil) |
| SessionDriverStarted | driverRunning CAS | poll for StatusIdle | SessionDriver goroutine |
| StatusIdle detected | InitialPrompt≠"" | SendKeys(initialPrompt+"\r") | SessionDriver → tmux |
| *(BUG: InitialPrompt="" → skip)* | sentInitial=true | (nothing sent) | SessionDriver |
| AutonomousDriverStarted | headlessPool≠nil | waitForIdle (5min timeout) | AutonomousDriver goroutine |
| SessionIdle detected | turn loop | CallBlockingWithOptions(goal) | AutonomousDriver → HeadlessPool |
| OrchestrationLLMResponded | parse NEXT_MESSAGE | SendCommandImmediate(msg) | AutonomousDriver → ClaudeController |
| TriagePromptInjected | Claude at ">" | (Claude begins triage work) | Claude Code (interactive) |
| SubagentResearchDone | agent completes | write research/*.md | Claude subagent |
| PlanWritten | research done | write plan.md, validation.md | Claude agent |
| submit_triage_result called | plan artifacts written | UpdateItemSessionTriageResult | MCP handler |
| submit_triage_result called | EventBus | Publish(NotificationEvent) | MCP handler |
| SessionOutputContainsResult | turn loop | CallBlockingWithOptions (check output) | AutonomousDriver |
| OrchestrationLLMReturnsDONE | parse DONE | fireCompletion(Done=true) | AutonomousDriver |
| AutonomousDriverCompleted | onAutonomousDriverComplete | TransitionBacklogItemStatus(idea→ready) | SessionService |
| BacklogItemReady | operator reviews triage | (manual review or spawn work session) | Operator |
| MaxTurnsReached | turn count >= maxTurns | fireCompletion(Stuck=true) | AutonomousDriver |
| AutonomousDriverStuck | onAutonomousDriverComplete | Publish(NotificationEvent "Triage stuck") | SessionService |
| SessionExited | EventExited | UpdateItemSessionEnded | BacklogLifecycleListener |

---

## 6. Recommended Fix (Minimum Viable)

### Fix A — `InitialPrompt` population (unblocks prompt injection)

In `CreateDirectorySession` (`session_service.go:579–594`), set both `Prompt` and `InitialPrompt`:

```go
opts := session.InstanceOptions{
    ...
    Prompt:        prompt,
    InitialPrompt: prompt,   // ← ADD THIS: session_driver will inject it into the pane
    ...
}
```

**Caveat**: `sanitizeInitialPromptForTmux` collapses `\n → " "` and truncates at 4096 chars. The triage prompt is ~700 chars with newlines → it arrives as one line. This should still work because Claude parses markdown formatting from whitespace-separated text, but the `### Step N` headers lose their semantic weight.

**Better approach**: Use `AppendSystemPrompt` instead of `InitialPrompt` for multi-line structured prompts. `AppendSystemPrompt` is passed via `--append-system-prompt` at CLI launch time, so Claude receives it properly formatted before the interactive session begins. Then `InitialPrompt` can be a short trigger like `"Please proceed with the triage task."`.

Revised approach:
```go
opts := session.InstanceOptions{
    ...
    Prompt:             "",            // not used in interactive mode
    AppendSystemPrompt: prompt,        // full triage prompt in system context
    InitialPrompt:      "Please proceed with the triage task described in your system instructions.",
    ...
}
```

### Fix B — headlessPool nil guard surface

The current nil-check in `StartAutonomousDriverWithTimeout` silently falls back to nothing. When `headlessPool` is nil, the session should fall back to `oneShot=true` mode OR the `autonomousStarter` interface should not be set. Consider:

```go
useAutonomous := s.autonomousStarter != nil && s.headlessPool != nil
```

...but `BacklogService` does not have direct access to `headlessPool`. A better approach: add `IsReady() bool` to `AutonomousDriverStarter` that BacklogService can query.

### Fix C — oneShot status transition gap

If oneShot mode is used as fallback, wire `BacklogLifecycleListener.onSessionExited` to handle `SessionRoleTriage` exits: check if `submit_triage_result` was already called (TriageResult non-empty on ItemSession), and if so, transition `idea → ready`.

---

## 7. Summary of Dual-Path Behavior

| Parameter | oneShot=true (fallback) | oneShot=false + autonomous |
|---|---|---|
| `inst.Prompt` | → `claude -p "prompt"` CLI arg | → goal string for orchestrator LLM |
| `inst.InitialPrompt` | irrelevant (not read by driver for oneShot) | → session_driver types into pane |
| `inst.AppendSystemPrompt` | → `--append-system-prompt` flag | → `--append-system-prompt` flag |
| Prompt delivery mechanism | Claude CLI `-p` positional arg | `SendKeys` from session_driver OR `SendCommandImmediate` from AutonomousDriver |
| Session exits | When Claude finishes (-p) | When max turns reached or DONE |
| Status transition | NOT wired (BacklogLifecycleListener ignores triage) | `onAutonomousDriverComplete` → `idea→ready` |
