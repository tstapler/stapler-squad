# Build vs Buy: Backlog Triage Autonomous Execution

**Date**: 2026-06-22
**Author**: Research agent
**Scope**: Evaluate execution strategies for the autonomous triage feature

---

## Context: What Is Broken and Why

`TriggerTriage` (backlog_service.go:1076) spawns a tmux session with `oneShot=false` and calls `StartAutonomousDriverWithTimeout(inst, 5*time.Minute)`. The session contains the triage prompt in `inst.Prompt` — but `session_driver.go` only reads `inst.InitialPrompt`, not `inst.Prompt`. Because the session is created with `oneShot=false` and `InitialPrompt=""`, the session driver skips the injection step entirely (`sentInitial = initialPrompt == ""`). Claude Code starts, displays its banner, and sits idle at `>`. The AutonomousDriver is supposed to rescue this by injecting the prompt on turn 1 via `SendCommandImmediate`, but there are multiple ways this can fail silently:

1. **Startup race**: The driver's idle detection waits on a `statusCh` (capacity 1). If the session startup fires multiple status changes rapidly, the buffered channel drops all but one. The driver may miss the first idle transition and time out after 5 minutes.
2. **headlessPool nil**: If the `claude` binary is not found at startup, `headlessPool` is nil and `StartAutonomousDriverWithTimeout` logs a warning and returns immediately — leaving a `oneShot=false` session with no driver and no initial prompt.
3. **controller nil at Start()**: `d.controller = d.inst.GetController()` is called inside `Start()` after `CompareAndSwap`. If the session's controller has not been wired yet (controller start happens asynchronously in `CreateDirectorySession`), the driver exits with "no controller available".
4. **Orchestrator LLM decides DONE prematurely**: On turn 1, the driver calls the headless LLM with `buildOrchestrationPrompt(d.goal, tail, 1, 20)`. The tail is likely the Claude startup banner. A mediocre orchestrator LLM response could misread the banner as "done" and send `DONE:` instead of `NEXT_MESSAGE:`.

The net result: the triage prompt is never delivered, and there is no recovery path.

---

## Reference: How the Headless Review Gate Works

`spawnReviewGate` (backlog_lifecycle.go:229) is the canonical example of a working autonomous operation:

```go
// Headless path: call LLM directly without spawning a tmux session.
headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated)
reviewResult, callErr := pool.CallBlocking(reviewCtx, headless.FeatureKeyReview,
    headless.HeadlessReviewSystemPrompt(), headlessPrompt)
// Parse JSON verdict from reviewResult
overall, perCriterion, summary := ParseHeadlessVerdictResult(reviewResult)
// Store result atomically
l.storage.CreateItemSessionWithVerdict(ctx, ...)
```

Key properties:
- **No tmux session** — runs `claude -p` in a subprocess via `ProcessRunner.Run`.
- **No AutonomousDriver** — single blocking call, result comes back as a string.
- **JSON output** — the system prompt instructs Claude to output a JSON blob instead of calling MCP tools (`headlessReviewSystemPrompt`).
- **No MCP tool access** — headless `claude -p` does not have tool access in the current implementation.
- **Fully synchronous** from the caller's perspective — `pool.CallBlocking` blocks until the subprocess exits.
- **Session reuse** — the pool caches the `session_id` from the first call's JSON output and passes `--resume session_id` on subsequent calls for the same `FeatureKey`, enabling prefix-cache reuse.

This design is robust because there is no idle-state polling, no prompt injection through a PTY, and no multi-party coordination between a tmux pane, a controller goroutine, and an orchestrator LLM.

---

## Option 1: Fix the Current Custom AutonomousDriver

**What it is**: Keep `TriggerTriage`'s `oneShot=false` + `StartAutonomousDriverWithTimeout` path, but fix the three bugs that prevent prompt delivery.

**What would need to change**:

1. **Fix the Prompt → InitialPrompt disconnect** (root cause #1). Either:
   - Set `InitialPrompt = triagePrompt` in `CreateDirectorySession` so the session driver injects it before the autonomous driver starts, OR
   - Make the autonomous driver inject the `d.goal` prompt on turn 1 unconditionally (bypassing the LLM decision for the first turn), instead of asking the orchestrator LLM what to do when the session hasn't even started working yet.

2. **Fix the startup race** (root cause #2). The `statusCh` is capacity 1; under burst startup the driver can miss the idle signal. Fix: add a fallback poll (`cc.IsIdle()` every 1s) inside `waitForIdle` so a missed channel event is caught.

3. **Guard against headlessPool nil** (root cause #3). If `headlessPool == nil`, fall back to `oneShot=true` with `InitialPrompt = triagePrompt` so Claude at least receives the task. Log a warning that autonomous steering is unavailable.

4. **Prevent premature DONE** (root cause #4). Add a minimum-turns guard: if `turnCount == 0` and the session tail looks like a startup banner (no code execution visible), force `NEXT_MESSAGE: <initial_prompt>` instead of delegating to the LLM.

**Pros**:
- Session is visible in the UI with real terminal output.
- AutonomousDriver already handles rate-limit waits and multi-turn steering — useful if triage gets stuck partway.
- Operator can watch progress in the live terminal view.
- Existing test infrastructure (`autonomous_driver_test.go`) can be extended.
- Consistent with how work-session autonomous mode operates.

**Cons**:
- Four distinct failure modes must be fixed; missing any one leaves triage broken again.
- The orchestrator LLM adds latency and cost to every turn (even when Claude is doing perfectly fine work).
- Idle detection via PTY terminal-content parsing is fragile — it depends on Claude's UI output format, which can change across versions.
- MCP tool accessibility still needs verification (`MCPServerURL` must be set and `submit_triage_result` must be registered).
- The driver interacts with a tmux PTY that spawns parallel subagents — the subagents generate output that the orchestrator LLM reads as the "session tail", which may confuse it.

**Verdict**: Viable but requires careful surgery. The minimal fix is: set `InitialPrompt = triagePrompt` (fixing the root cause) and add a nil-headlessPool fallback for the autonomous mode. The orchestrator LLM layer is a net negative for triage — it adds complexity without value because the triage prompt already gives Claude complete instructions.

---

## Option 2: Claude's Native `-p` Flag + Headless Mode (No tmux)

**What it is**: Replace the tmux session entirely for triage. Run `claude -p "<triage_prompt>" --work-dir <repo_path>` via the existing `headless.Pool` with a `WorkDir` override, exactly as the review gate does.

**How it works in practice**: `pool.CallWithOptions(ctx, featureKey, systemPrompt, triagePrompt, headless.CallOptions{WorkDir: item.RepoPath})` spawns a subprocess and blocks until Claude completes. The headless pool's `CallWithOptions` with a non-empty `WorkDir` creates a one-shot pool (no session reuse), ensuring each triage call starts fresh. Timeout: up to 30 minutes (`headless.MaxCallTimeout = 1800s`).

**Critical constraint**: `headless.Pool`'s `ProcessRunner` calls `claude -p` with `--output-format json` on the first call and `--resume <session_id>` on subsequent ones. The `--output-format json` flag causes Claude to return a plain-text JSON blob at the end — **not** tool calls. The `headlessReviewSystemPrompt` explicitly instructs the model to output JSON instead of calling tools. For triage, `submit_triage_result` is an MCP tool call; with `-p --output-format json`, Claude cannot invoke it.

There are two sub-approaches:
- **2a**: Rewrite the triage prompt (like the headless review prompt) to request JSON output instead of MCP tool invocation. Process the JSON on the server side and call `UpdateItemSessionTriageResult` directly. **This bypasses MCP entirely.**
- **2b**: Run `claude -p` without `--output-format json` (plain streaming mode, no session reuse). Pass `--mcp-server` so Claude can call `submit_triage_result`. This requires modifying `ProcessRunner.Run` and the pool's `acquireSession` path to support MCP-server-enabled headless runs.

**2a - Headless JSON Output Pros**:
- Identical to the review gate pattern — already proven to work.
- No idle detection, no PTY, no orchestrator LLM.
- Simple error handling: one `CallBlocking` call, one JSON parse.
- Inherits pool concurrency limiting and error/retry logic.
- Fast: no startup overhead from tmux session lifecycle.

**2a - Headless JSON Output Cons**:
- Loses visibility: operator cannot watch triage progress in the UI terminal view.
- JSON schema drift: triage result parsing must be kept in sync with `triageResultJSON` struct.
- No file-system writes during triage visible in real-time (though they still happen — Claude writes to `docs/tasks/<slug>/` in the subprocess working directory).
- Triage prompt is complex (spawn subagents, write files, synthesize) — fits Claude's agentic mode better than a constrained JSON-output mode. The subagent spawning behavior is a question mark in headless `-p` mode with `--output-format json`.

**2b - Headless + MCP Pros**:
- Uses the existing `submit_triage_result` MCP tool path — no schema drift.
- Retains agentic file-writing behavior.

**2b - Headless + MCP Cons**:
- Requires modifying `ProcessRunner`/pool to support `--mcp-server` flag — not currently supported.
- MCP server URL must be passed in a way the subprocess can reach it (network + auth).
- More complex than 2a.

**Verdict for 2a**: Recommended for the triage-only path. The review gate proves this pattern. The key change is: replace `buildTriagePrompt` with a headless-variant that requests JSON output, remove the tmux session spawn, and call `pool.CallBlockingWithOptions` in `TriggerTriage` directly (running in a goroutine so it doesn't block the RPC). Store the result when the goroutine completes.

**Verdict for 2b**: Not recommended without MCP-in-headless support being added to the pool.

---

## Option 3: Claude Code SDK / Agents SDK

**What it is**: Use a programmatic Go/Python SDK from Anthropic to drive a Claude Code agent session without tmux.

**Current state (as of 2026)**: The [Claude Code SDK](https://docs.anthropic.com/en/docs/claude-code/sdk) provides language bindings (Python, TypeScript/JS) for running Claude Code in subprocess headless mode (`-p` flag). It is essentially a typed wrapper around the same `claude -p` binary interface that `headless.Pool` already implements directly. The SDK does not provide a network API or daemon mode — it still shells out to the `claude` binary.

The [Anthropic Agents SDK](https://docs.anthropic.com/en/docs/agents) is a higher-level framework for building multi-agent systems using the Claude API directly (HTTP calls to `api.anthropic.com`), not the `claude` CLI binary. It is primarily Python-based and requires direct API key access.

**Pros**:
- The Claude Code SDK's subprocess model validates that the current `headless.Pool` approach is architecturally correct.

**Cons**:
- Adding a Python or TypeScript subprocess dependency into a Go binary is not feasible without adding significant build complexity.
- The Anthropic Agents SDK requires direct API access, which may not be the deployment model here (the service runs `claude` binary which handles auth internally).
- No additional capability beyond what `headless.Pool` already provides.
- Would require adding a foreign-language runtime dependency to a Go project.

**Verdict**: Not recommended. The existing `headless.Pool` is the equivalent of the Claude Code SDK for this codebase. Adopting the upstream SDK would add complexity without benefit.

---

## Option 4: Task Queue / Goroutine Worker Pool

**What it is**: Replace `StartAutonomousDriverWithTimeout` with a simple goroutine that runs the full triage operation as a blocking call and writes results when done. No per-turn orchestrator LLM, no idle polling — just a worker that runs triage end-to-end.

**Concrete implementation**:

```go
// In TriggerTriage, after creating the ItemSession:
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
    defer cancel()
    // Option A: headless (no tmux)
    result, err := pool.CallBlockingWithOptions(ctx, headless.FeatureKeyTriage,
        triageSystemPrompt, triageUserPrompt,
        headless.CallOptions{WorkDir: item.RepoPath})
    // Option B: one-shot tmux + wait for submit_triage_result notification
}()
```

This is essentially Option 2a with explicit goroutine management. The key insight is that the AutonomousDriver's multi-turn loop is not needed for triage: the triage prompt is complete and self-contained, Claude does not need external steering per turn, and the expected outcome is a single `submit_triage_result` call at the end.

A bounded worker pool (like `maxConcurrentReviewGates = 8`) can cap concurrent triage runs using a semaphore channel — identical to how `BacklogLifecycleListener.reviewSem` is used for review gates.

**Pros**:
- Trivially simple: one goroutine, one blocking call, one result write.
- Proven: the review gate already does exactly this.
- No idle detection, no PTY, no orchestrator LLM.
- Rate limit handling is already built into `pool.CallBlocking` via context cancellation.

**Cons**:
- No per-turn UI visibility (same as Option 2a).
- The goroutine must report progress or completion somehow — requires an event bus publish when done.
- If the process crashes mid-triage, there is no recovery path (though the review gate has the same limitation).

**Verdict**: Recommended — this is the correct shape. It is a simplification of Option 2a with explicit goroutine lifecycle management. The worker pool cap already exists in the codebase as a pattern.

---

## Option 5: MCP-Only Triage (Pure Headless LLM Call)

**What it is**: Run triage as a headless LLM call where `submit_triage_result` is called by the LLM as part of its JSON output (not as an MCP tool call), and the server side processes the JSON to store results and advance the item state.

This is identical to Option 2a except with the explicit framing of "no MCP at all" — the triage prompt instructs Claude to output a JSON object with the same fields as `submit_triage_result` (`item_id`, `summary`, `suggestions`, `tasks`, `plan_artifact_path`), and the server parses it directly.

**Concrete implementation**: Analogous to `BuildHeadlessReviewPrompt` / `ParseHeadlessVerdictResult`, add `BuildHeadlessTriagePrompt` and `ParseHeadlessTriageResult`. The system prompt instructs Claude to output structured JSON. Server-side code calls `UpdateItemSessionTriageResult` and `UpdateBacklogItem` directly.

**Pros**:
- Identical to the review gate pattern — proven to work.
- Simplest possible implementation.
- No MCP dependency.
- Can run Claude's full agentic (file-writing) capabilities — the subprocess sets `--work-dir` so Claude writes files to the repo.

**Cons**:
- Claude in `--output-format json` mode may behave differently for complex multi-step agentic tasks (subagent spawning). The research/synthesis/validation steps in `buildTriagePrompt` rely on Claude spawning parallel sub-agents — this may work differently in headless `-p` mode. Needs verification.
- Loses the `STAPLER_SESSION_UUID` env var that lets Claude identify itself to the MCP server. However, since we're not using MCP tools in this path, that's moot.
- Requires adding a `FeatureKeyTriage` constant to `headless/features.go`.

**Verdict**: Recommended as the primary implementation path. It is the cleanest architectural fit, consistent with the review gate, and requires minimal new code.

---

## How the Review Gate Informs Triage

The review gate (`spawnReviewGate`) demonstrates the correct pattern for backlog autonomous operations:

| Property | Review Gate | Triage (current, broken) | Triage (recommended) |
|---|---|---|---|
| Execution method | `pool.CallBlocking` | tmux + AutonomousDriver | `pool.CallBlockingWithOptions` with WorkDir |
| UI visibility | None (headless) | tmux pane | None (headless) |
| Prompt delivery | System prompt + user prompt string | Autonomous driver loop | System prompt + user prompt string |
| Result format | JSON parsed server-side | MCP tool call from within tmux | JSON parsed server-side |
| Failure modes | LLM error, timeout | 4+ distinct failure modes | LLM error, timeout |
| Complexity | ~50 LOC | ~400 LOC across 4 files | ~50–80 LOC |
| MCP dependency | None | `submit_triage_result` tool | None |
| Concurrency cap | `reviewSem` (8) | None currently | Add `triageSem` (8) |

The review gate's headless path was added specifically because the tmux-spawning path (`SpawnReviewSession`) was fragile and hard to test. The same reasoning applies to triage.

---

## Decision Matrix

| Option | Complexity | Reliability | UI Visibility | Effort | Verdict |
|---|---|---|---|---|---|
| 1: Fix AutonomousDriver | High | Medium | Full tmux | Large | Viable |
| 2a: Headless `-p` + JSON | Low | High | None | Small | **Recommended** |
| 2b: Headless `-p` + MCP | Medium | Medium | None | Medium | Not recommended |
| 3: Claude/Anthropic SDK | High | Unknown | None | Large | Not recommended |
| 4: Goroutine worker pool | Low | High | None | Small | **Recommended** |
| 5: MCP-only headless | Low | High | None | Small | **Recommended** |

Options 2a, 4, and 5 converge on the same implementation: a headless `pool.CallBlocking` call in a bounded goroutine, with a JSON-output prompt and server-side result parsing. They are the same option expressed at different levels of abstraction.

---

## Recommended Implementation Plan

**Short path (< 1 day of work)**:

1. Add `FeatureKeyTriage FeatureKey = "triage"` to `session/headless/features.go`.
2. Add `BuildHeadlessTriagePrompt(item, artifactAbsPath, slug string) string` in a new `session/backlog_triage.go` — a variant of `buildTriagePrompt` that appends JSON-output instructions instead of MCP tool instructions.
3. Add `ParseHeadlessTriageResult(text string) (triageResultPayload, error)` to parse the JSON.
4. Modify `TriggerTriage` to: (a) not spawn a tmux session, (b) create the `ItemSession` with `SessionUUID = "headless-triage-" + uuid.New().String()`, (c) launch a bounded goroutine that calls `pool.CallBlockingWithOptions`, parses the result, calls `UpdateItemSessionTriageResult`, updates `plan_artifacts_path`, and transitions the item to `ready`.
5. Wire a `triageSem` semaphore (cap 8) in `BacklogLifecycleListener` or a new `TriageRunner` struct.
6. Publish an event-bus notification when triage completes (same as the review gate).

**If tmux visibility is required later**: The `AutonomousDriver` can be reintroduced on top of this working headless baseline. Fix the `InitialPrompt` disconnect (set `InitialPrompt = triagePrompt` in `CreateDirectorySession` options), remove the orchestrator LLM turn-1 indirection, and use the driver only for monitoring/steering if Claude gets stuck. But do not start with the AutonomousDriver as the primary execution mechanism.

---

## Key Open Questions Resolved by This Research

1. **Is the Prompt→InitialPrompt disconnect the root cause?** Yes. `session_driver.go:94` reads `inst.InitialPrompt`; `TriggerTriage:1181` calls `CreateDirectorySession` which sets `Prompt` (not `InitialPrompt`) via `InstanceOptions`. The session driver skips injection entirely.

2. **Can triage use the same headless pool as the review gate?** Yes. `headless.Pool.CallWithOptions` with `WorkDir = item.RepoPath` creates a one-shot subprocess with the correct working directory. Claude will write files to the repo from within the subprocess.

3. **Does the headless path support file writing?** Yes. The subprocess runs with `WorkDir` set; Claude can `Write` files during its execution. The final artifacts appear on disk when the subprocess exits.

4. **Is there a `FeatureKeyTriage` constant?** No — needs to be added to `session/headless/features.go`.

5. **What is the correct concurrency cap?** 8 (matching `maxConcurrentReviewGates`).
