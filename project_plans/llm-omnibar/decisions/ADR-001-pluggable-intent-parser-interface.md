# ADR-001: Pluggable `IntentParser` Interface with Strategy + Factory Pattern

**Status**: Proposed
**Date**: 2026-04-18

## Context

The LLM Omnibar feature requires invoking an LLM to convert natural-language session descriptions into structured `SessionIntent` values. Three backends must be supported:

1. **Claude CLI subprocess** — invokes `claude -p` as an `exec.CommandContext` call; uses the user's existing Claude Code subscription; no API key required
2. **Anthropic SDK** — direct HTTP to `api.anthropic.com`; requires `ANTHROPIC_API_KEY`; supports native constrained-decoding structured outputs
3. **AWS Bedrock** — Claude via `bedrockruntime.Converse`; requires AWS credentials and a model ARN; supports `outputConfig.textFormat` structured outputs

Each backend has different auth requirements, latency profiles, and structured-output mechanisms. If the choice is baked into session-creation code directly, swapping backends (for users with or without API keys) becomes impossible without recompilation. Additionally, the same parsing logic must be callable from three entry points: the React UI (via a ConnectRPC endpoint), the HTTP API (`POST /api/sessions/intent`), and the MCP tool `create_session_from_intent`. Duplicating the logic across those entry points would cause drift.

## Decision

Define a single Go interface in the `server/intent/` package:

```go
type IntentParser interface {
    ParseIntent(ctx context.Context, description string, sc StarterContext) (*SessionIntent, error)
}
```

Implement three concrete types behind it:

- `CLIBackend` (`server/intent/cli_backend.go`)
- `AnthropicSDKBackend` (`server/intent/sdk_backend.go`)
- `BedrockBackend` (`server/intent/bedrock_backend.go`)

A `NewFactory` function in `server/intent/factory.go` reads the active backend from `config.IntentBackend` (which maps to the `intent_backend` key in `config.json`, defaulting to `"claude_cli"`) and returns the appropriate implementation. The factory is called once at server startup; the resulting `IntentParser` is injected into both the `SessionService` and the MCP handlers via constructor arguments — matching the existing dependency-injection pattern used throughout the codebase (e.g., `SessionService` receiving `mcpServerURL` and `storage`).

The `StarterContext` carries lightweight ambient data: recent repo paths, a short list of existing session summaries, and the `MCPServerURL` so backends that support tool-use can call back into the running MCP server.

The `SessionIntent` struct mirrors the requirements spec exactly:

```go
type SessionIntent struct {
    Title              string
    Path               string
    Branch             string
    Program            string   // "claude" | "aider"
    SessionType        string   // "directory" | "new_worktree" | "existing_worktree"
    InitialPrompt      string
    Tags               []string
    SuggestedSessionID string
    Confidence         float64  // 0.0–1.0
}
```

## Consequences

**Positive**:
- Backend selection is a config change; no code change or recompile needed
- All three entry points (UI, HTTP, MCP) call the same `IntentParser` instance, ensuring identical behavior
- Each backend can be unit-tested independently via mock `IntentParser` implementations
- Adding a fourth backend (e.g., Ollama for local inference) requires only a new struct and a factory entry, with zero changes to callers

**Negative / accepted costs**:
- The factory pattern adds one level of indirection vs. direct instantiation
- `StarterContext` must be populated before each call, requiring a brief query to the session store; this adds ~10ms but is unavoidable regardless of architecture
- The `CLIBackend` cannot use native structured outputs (the CLI wrapper returns an outer JSON envelope, not schema-constrained output), so its JSON parsing must include an extraction + repair layer — this complexity is fully contained within `cli_backend.go` and not visible to callers

**Rejected alternatives**:
- *Single backend, config at compile time*: Fails the "zero API key" requirement — users without `ANTHROPIC_API_KEY` would have no working path
- *Global singleton*: Violates the existing DI pattern in the codebase and makes testing harder
- *Inline switch statement in handlers*: Logic duplication across three call sites; cannot be tested independently
