# ADR-002: Triage sessions use AutonomousDriver instead of OneShot

**Date**: 2026-06-15
**Status**: Accepted

## Context

`TriggerTriage` and `TriggerReReview` currently create sessions with `oneShot: true`. OneShot runs Claude as `claude -p --output-format json` — a single LLM turn with no MCP server connection. This means `submit_triage_result` (an MCP tool) cannot be called from within a triage session. Additionally, if the session stalls mid-triage, there is no re-injection mechanism.

`AutonomousDriver` runs Claude in interactive mode with MCP enabled, injects orchestration prompts between idle periods, and has a configurable completion signal. `SpawnSessionFromItem(autonomous=true)` already uses this pattern.

**Research conclusion (AC-4)**: OneShot and AutonomousDriver are complementary, not duplicative.
- OneShot: structured JSON output, deterministic single turn, no MCP. Appropriate for LLM evaluation harnesses where MCP is not needed.
- AutonomousDriver: multi-turn, MCP-capable, completion-aware. Required for any session that calls MCP tools (triage, review, work).

Triage requires MCP (to call `submit_triage_result`). Therefore triage must use AutonomousDriver. OneShot is retained for non-backlog use cases where MCP is not needed.

## Decision

- `TriggerTriage` and `TriggerReReview` switch from `oneShot: true` to `oneShot: false` and start an `AutonomousDriver` when `autonomousStarter != nil`.
- When `autonomousStarter == nil` (headlessPool unavailable), fall back to `oneShot: true` for graceful degradation.
- Extended startup timeout of 5 minutes (vs. default 60s) is used for triage/re-review to accommodate parallel subagent spawning.
- `isOneShot()` tag-based no-retry logic in `session_driver.go` is unaffected (it checks tags `backlog:triage` / `backlog:review`, not the `OneShot` field).
- Non-backlog OneShot usages (e.g. frontend `CreateSession` with `oneShot: true`) are out of scope and remain unchanged.

## Consequences

- Triage and re-review sessions now require MCP connectivity and an active `headlessPool`. Deployments without headlessPool degrade gracefully to single-shot mode.
- `onAutonomousDriverComplete` now fires for triage sessions, requiring role-aware status transition logic (see ADR-003).
- The `buildTriagePrompt` comment "one-shot triage agent prompt" should be renamed to "triage agent prompt" in a follow-on cleanup.
