# Requirements: backlog-triage-autonomous

**Date**: 2026-06-22
**Type**: bug fix + feature improvement
**Complexity**: 3 — system design; multiple interacting components, autonomous session lifecycle, headless LLM pool

## Problem Statement

When a user clicks "Trigger Triage" on a backlog item, a triage session is spawned but sits completely idle — Claude never receives the triage prompt. The session shows the Claude Code startup banner but no task is injected. The autonomous triage flow (intended to run research subagents, synthesize a plan, and call `submit_triage_result`) never executes. This affects every triage trigger attempt.

## Baseline

Current behavior: `TriggerTriage` creates a tmux session with `oneShot=false` and calls `StartAutonomousDriverWithTimeout`. The `AutonomousDriver` is supposed to inject orchestration prompts, but the session remains idle at the Claude Code `>` prompt. The `inst.Prompt` field holds the triage prompt but the session_driver never injects it (it only uses `inst.InitialPrompt`, which is empty for autonomous sessions). The autonomous driver loop must then inject via LLM orchestration on turn 1 — but this may fail silently if `headlessPool` is nil, the controller is unavailable, or idle detection misfires.

Workaround: none. Triage sessions never complete autonomously; operators must manually open the session and paste the prompt.

## Users / Consumers

- Operator (Tyler): triggers triage via UI "Trigger Triage" button, expects the session to run to completion autonomously and surface plan artifacts + triage result.
- BacklogService / AutonomousDriver: automated systems driving the session.

## Success Metrics

1. After "Trigger Triage", the triage session injects the triage prompt within 30 seconds of session startup.
2. The session autonomously runs to completion (calls `submit_triage_result`) without operator intervention.
3. Plan artifacts (stack.md, features.md, architecture.md, pitfalls.md, plan.md, validation.md) are written to `docs/tasks/<slug>/`.
4. The item transitions from `idea` → `ready` automatically when triage completes.
5. All of the above work in the full stack (real Claude binary, real headless pool, real tmux).

## Appetite

Large (3–6 weeks) — redesign the triage flow end-to-end if needed; do not apply band-aids to a broken design.

## Constraints

- Must not break existing non-triage sessions (work sessions, review sessions, manually-created sessions).
- The `AutonomousDriver` architecture should remain intact — it is also used for work sessions; any changes must not regress that path.
- No feature flag needed; the fix is self-contained and low-risk to revert via git.

## Non-functional Requirements

- **Performance SLO**: Triage prompt injection must happen within 30 seconds of Claude reaching idle state.
- **Scalability**: Up to 8 concurrent triage sessions (per `maxConcurrentReviewGates` cap).
- **Security classification**: internal.
- **Data residency**: no special requirements.

## Scope

### In Scope

- Root cause diagnosis: why triage sessions sit idle — confirmed as Prompt vs InitialPrompt disconnect, headlessPool nil, and AutonomousDriver reliability (resolved by replacing the path entirely).
- Fix the prompt injection: replace tmux+AutonomousDriver triage path with a headless pool call (same pattern as the existing review gate); AutonomousDriver is preserved for work sessions.
- Remove autonomous driver path from triage flow; do not attempt to fix the driver for triage.
- Add structured observability: log when headless triage starts, completes, fails, and when the idea→ready status transition fires.
- Triage result submitted via JSON output parsing (no MCP tool call in headless mode); `submit_triage_result` MCP tool preserved for any future tmux-based flows but not called in this path.
- Integration test: `TriggerTriage` RPC drives a headless pool call to completion, persists result, and transitions item to `ready` without manual intervention.
- UI feedback: triage in-progress shown as compact pill in list row; failure state surfaces a retry button in the detail pane.
- Bounded concurrency: up to 8 concurrent headless triage calls (semaphore cap 8); goroutines respect server shutdown via `shutdownCtx`.

### Out of Scope

- Redesigning the backlog review gate (separate flow).
- Changing the headless pool implementation.
- Adding new backlog statuses or workflow states.
- No hard exclusions yet — leave open questions for research.

## Rabbit Holes

- **Prompt vs InitialPrompt disconnect**: `session_driver.go` only injects `inst.InitialPrompt`, not `inst.Prompt`. Autonomous sessions set `OneShot=false` and leave `InitialPrompt` empty, relying entirely on the AutonomousDriver. If the driver fails to start for any reason, the session is stranded with no recovery path.
- **Idle detection timing**: The AutonomousDriver waits for `StatusIdle/Ready/Success` from the controller. Claude Code startup may fire a non-idle status before settling — if the driver's status channel misses the idle transition it will time out.
- **Headless pool nil silently degrades**: If `claude` binary is not found at startup, `headlessPool` is nil and `StartAutonomousDriverWithTimeout` logs a warning and returns without starting the driver. The session was already created with `oneShot=false`, so nothing drives it.
- **MCP tool accessibility**: `submit_triage_result` is registered as an MCP tool. Triage sessions use `MCPServerURL` passed via `--mcp-server`. If the MCP server is not reachable or the tool is not registered, `submit_triage_result` silently fails.
- **Orchestration LLM model**: The autonomous driver uses a simple orchestrator prompt (`NEXT_MESSAGE` / `DONE`). For a multi-step triage that spans dozens of subagent calls, this model may produce poor turn-by-turn decisions — or may DONE prematurely.

## Alternatives Considered

- **oneShot=true for triage**: Inject the triage prompt directly (like review sessions do) instead of using the autonomous driver loop. Simpler, but loses the ability to monitor and steer the session if it gets stuck.
- **Use `--append-system-prompt` only**: Pass the triage prompt as a system prompt injection. This constrains the initial context but doesn't inject a task message.
- **Headless-only triage** (like the review gate): Use `headless.Pool.CallBlocking` directly without spawning a tmux session. Much simpler but loses visibility into what Claude is doing.

## Feasibility Risks

- Root cause is unknown — multiple failure modes are plausible. Phase 2 research must reproduce the failure and identify which path is actually failing.
- The autonomous driver is relatively new and lightly tested in real-world conditions.

## Observability Requirements

- Structured log events at: driver started, idle detected, each turn injected (turn N / maxN + prompt snippet), completion outcome (Done/Stuck/Timeout), and triage result submitted.
- UI should show triage session progress (turn counter, last injected prompt summary).
- Log the headless pool init status at startup (already present: "headless LLM pool initialized" or "headless pool disabled").

## Risk Control

Not needed — low risk, self-contained fix, easy to revert via git.

## Open Questions

1. Is `headlessPool` actually nil in the environment where this was observed? Check logs for "headless pool disabled" vs "headless LLM pool initialized".
2. Does the AutonomousDriver idle detection work correctly for a freshly-started Claude session? What status does the controller report immediately after startup?
3. Is `submit_triage_result` registered as an MCP tool and reachable from a triage session?
4. Should triage use `oneShot=true` (direct prompt injection) + autonomous driver for subsequent turns, rather than relying entirely on the autonomous driver for turn 1?
5. What should happen when autonomous triage gets stuck (max turns reached)? Should it notify the operator and fall back to manual mode?
