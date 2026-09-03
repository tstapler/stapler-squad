# ADR-001: Program-gating uses exact-match comparison, not substring

**Status**: Accepted
**Date**: 2026-08-26
**Context project**: pr-fix-steering

## Context

The steer message built by `steerActiveSessionForPRFix` must only append
`\n\n Run /github:pr-ship to address this.` when the active session's program
is Claude Code — a `/github:pr-ship` slash command sent to Aider or another
non-Claude agent is garbage input (requirements.md's Constraints section).

Research left two contradictory recommendations for how to test that:

- stack.md: `strings.Contains(strings.ToLower(program), "claude")`, mirroring
  `ClaudeAdapter.CanHandle` ([`session/claude_adapter.go:27-29`](../../../session/claude_adapter.go#L27-L29)).
- architecture.md: an exact literal comparison, citing
  [`session/instance_program_test.go:19,76`](../../../session/instance_program_test.go#L19).

Neither citation is actually a real precedent for *this* decision once
checked directly:

- `ClaudeAdapter.CanHandle` (`Contains`) parses an already-produced session
  **transcript** to pick a message-format adapter — a different problem
  (which parser understands this JSONL?) from "should this live `Instance`
  get a Claude-Code-specific instruction?".
- `instance_program_test.go:19` (`inst.Program = "claude"`) is test *fixture*
  data, not a comparison the production code performs — it doesn't settle
  anything either way.

The actual closest precedent in the codebase for "does this `Program`-typed
field mean Claude Code, for the purpose of appending a Claude-specific
flag/instruction" is
[`server/workflows/scheduler.go:385`](../../../server/workflows/scheduler.go#L385):

```go
isClaudeProgram := program == "" || program == "claude"
```

This is an exact, case-sensitive literal match against `wf.AgentType` (the
same kind of raw, user/config-supplied program string as `Instance.Program`),
deciding whether to append a Claude-specific `--model` flag — structurally
identical to this project's "append a Claude-specific slash-command
instruction" decision.

## Decision

`SessionProgram`'s returned `program` is compared with an exact,
case-sensitive match against the literal `"claude"`, with the same empty-string
fallback as `scheduler.go:385`:

```go
isClaudeCode := program == "" || program == "claude"
```

Empty string is included because a session created before
`config/defaults.go`'s program-resolution logic ran (or one whose Program was
never explicitly set) defaults to Claude Code today — `scheduler.go` already
encodes this exact same assumption for its own Claude-specific gating.

`strings.Contains`/`CanHandle`-style substring matching is rejected for this
call site: it would also match `"proxy-claude"` (a distinct configured
program value present in this codebase's own test fixtures, e.g.
`config/config_test.go:88`) and any future agent whose name happens to
contain the substring "claude" without actually being able to run
`/github:pr-ship` as a slash command.

## Consequences

- `buildSteerMessage` (Task 3.1.1) uses this exact comparison, not `Contains`.
- A hypothetical `"proxy-claude"` session gets the plain-English fixContext
  body only, no slash-command suffix — consistent with `scheduler.go`'s
  existing narrower treatment of that same value, not a new inconsistency.
- If a future agent needs `/github:pr-ship`-equivalent support, that agent's
  literal program string should be added to this comparison explicitly (or
  the comparison promoted to a small allowlist), not loosened to a substring
  match.
