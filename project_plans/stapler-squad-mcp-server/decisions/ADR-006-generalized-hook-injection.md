# ADR-006: Generalized Hook Injection for Session Lifecycle Events

**Status**: Accepted
**Date**: 2026-04-18

## Context

Stapler Squad currently injects exactly one Claude Code hook: `PermissionRequest` (now called `Notification` in the Claude Code hooks spec), which routes approval decisions through the Stapler Squad web UI. This is a narrow, hardcoded function (`InjectHookConfig`).

Claude Code as of 2026 supports 21 hook lifecycle events. The ones immediately useful for session state tracking and evaluation are:

| Event | When it fires | Stapler Squad use |
|---|---|---|
| `Notification` | Permission requests, auth prompts | Existing — approval routing |
| `Stop` | Claude concludes a response (turn end) | Session idle state detection |
| `PreToolUse` | Before Claude executes a tool | Evaluation, blocking, logging |
| `PostToolUse` | After a tool completes | Result logging, analytics |
| `UserPromptSubmit` | User sends a message | Turn start tracking |
| `SubagentStop` | A subagent finishes | Subagent monitoring |

The goal is to let sessions opt into any of these, with Stapler Squad running HTTP receivers for each, so the server can track session state, log tool calls for evaluation, and expose this data through the MCP surface.

The "hookify" pattern: inject multiple hooks into a session's `.claude/settings.local.json` in a single operation, with all hook commands POSTing back to the Stapler Squad HTTP server at known endpoints.

## Decision

**Generalize `InjectHookConfig` → `InjectHooksConfig`** that accepts a `HookSpec` declaring which hook events to activate. Replace the hardcoded `PermissionRequest`-only injection with a composable system.

**Built-in hook names** (enum, not arbitrary strings):
- `permission_approval` — existing `Notification` hook; approval routing (always injected)
- `stop_notification` — `Stop` hook; fires when Claude finishes a turn; server marks session as idle
- `pre_tool_logging` — `PreToolUse` hook; logs tool name + input to Stapler Squad analytics
- `post_tool_logging` — `PostToolUse` hook; logs tool result to Stapler Squad analytics
- `prompt_submit` — `UserPromptSubmit` hook; fires on each user turn start; useful for turn counting

**Server-side endpoints added** (all existing HTTP server, not MCP):
- `POST /api/hooks/stop` — receives `Stop` events; updates session `last_idle_at`, fires UI event
- `POST /api/hooks/pre-tool-use` — receives `PreToolUse` events; logs to analytics store; returns exit 0 (proceed) always in v1
- `POST /api/hooks/post-tool-use` — receives `PostToolUse` events; logs tool + result to analytics
- `POST /api/hooks/prompt-submit` — receives `UserPromptSubmit` events; increments turn counter

**`create_session` hook parameter**:
```
hooks: string[] — names from the built-in enum above
       default: ["permission_approval", "stop_notification"]
```

**`update_session` hook parameters**:
```
add_hooks: string[]    — add these hooks to the session
remove_hooks: string[] — remove these hooks from the session
```

All hook commands use `curl` POSTing to `http://localhost:8543/api/hooks/<event>` with session ID header, identical to the existing `PermissionRequest` pattern.

## Rationale

- Reuses the proven `curl`-based hook command pattern — no new mechanism, just more endpoints
- `stop_notification` enables the server to know when Claude is idle vs. working — currently impossible without polling scrollback
- `pre_tool_logging`/`post_tool_logging` provides the evaluation data layer: what tools did Claude use, on what inputs, with what results
- Enum of built-in names (not arbitrary user-defined hook commands) keeps the attack surface controlled — no arbitrary shell injection via hook config
- Default of `["permission_approval", "stop_notification"]` gives new sessions two high-value hooks without requiring explicit opt-in for each
- `permission_approval` is always injected regardless of the `hooks` parameter — it is a core Stapler Squad capability that should never be silently absent

## Consequences

- `InjectHookConfig` becomes `InjectHooksConfig(rootDir, sessionTitle string, hooks []HookName) error` — existing callers updated to pass `[]HookName{"permission_approval"}`
- Four new HTTP handler functions needed in `server/services/`; can be thin wrappers over existing `EventBus`
- Analytics store needs new schema for tool call events (pre/post tool use data)
- The `stop_notification` hook gives us a new session state: `idle` (Claude finished its turn) vs `running` (actively generating). This feeds a future UX improvement but is useful immediately for `wait_for_output` heuristics.
- Custom (user-defined) hook commands are explicitly out of scope for v1 — only built-in named hooks. Arbitrary hook injection would require sanitization and is a future feature.

## Alternatives Considered

- **Keep `InjectHookConfig` hardcoded and add new functions per hook type**: Rejected — leads to N parallel injection functions with duplicated merge logic. A single composable function is cleaner.
- **Allow arbitrary curl commands as hooks**: Rejected for v1 — creates a shell injection surface and makes the config harder to reason about. Built-in named hooks are safe and auditable.
- **WebSocket-based hook delivery instead of HTTP**: Rejected — Claude Code hooks use HTTP or command (stdio); adding WebSocket would require a new protocol. `curl` is already proven.
