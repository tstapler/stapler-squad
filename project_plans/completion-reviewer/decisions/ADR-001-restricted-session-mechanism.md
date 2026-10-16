# ADR-001: Restricted-Session Mechanism — Direct Anthropic Messages API via the Existing `AnthropicAIClient`

**Status**: Accepted
**Date**: 2026-08-22

## Context

The requirements' hardest AC is: "Spawned review session has no tools except memory-write,
enforced at the session-builder level (allowed-tool list / capability gate in code) — not by
prompt instruction alone." Two of the four research documents disagreed on the mechanism:

- `research/architecture.md` and `research/build-vs-buy.md` recommended a **direct Anthropic
  Messages API call from Go**, with a single hard-coded tool (or no tool at all) in the request
  body — reasoning that `--allowedTools`/`--disallowedTools` passed to the `claude` CLI have
  already been empirically proven, in this exact codebase, to provide **no real technical
  enforcement for Bash** under `--permission-mode bypassPermissions`
  (`TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`,
  `session/headless/integration_test.go:167`; ADR referenced from
  `session/backlog_review.go:405-424`).
- `research/stack.md` and `research/features.md` recommended extending
  `session/headless.Pool`/`CallOptions` with a new `--mcp-config` exposing exactly one
  memory-write MCP tool, `AllowedTools` set to only that tool's name, plus a new empirical
  self-check test in the style of `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`
  before trusting it.

Both sides agree on the deeper principle: never trust an allow-list flag alone as a security
boundary — enforcement must come from either (a) the process literally never having the tool
implemented/exposed, or (b) an empirical proof test.

While grounding the plan against the actual codebase (not just the research docs), a third,
better-fitting option was found that neither research pass had located:
**`server/services/anthropic_client.go`'s `AnthropicAIClient`** already exists in production,
implementing the existing consumer-defined `AIClient` interface
(`server/services/ai_interfaces.go:45-47`, `Complete(ctx, systemPrompt, userPrompt string)
(string, error)`) for AI-assisted rule generation (`server/services/rules_service.go`). It makes
a raw HTTP POST to `https://api.anthropic.com/v1/messages` with a request body defined by:

```go
// server/services/anthropic_client.go:69-74
type anthropicRequest struct {
    Model     string             `json:"model"`
    MaxTokens int                `json:"max_tokens"`
    System    string             `json:"system"`
    Messages  []anthropicMessage `json:"messages"`
}
```

**This struct has no `Tools` field at all.** There is no code path in `AnthropicAIClient.Complete`
that can attach a tool definition to the outbound request — the guarantee is structural (a Go
type has no such field), not behavioral (a flag that a CLI version might or might not honor).

Credential resolution is already solved too: `server/services/credentials.go`'s `CredentialChain`
(`NewDefaultChain(cfg)`) resolves an Anthropic credential from, in priority order, an environment
variable, config file entry, or **the same `~/.claude/.credentials.json` OAuth file the `claude`
CLI itself uses** (`ClaudeOAuthCredentialSource`) — so no new secret/billing relationship is
required to use this path.

One adjacent trap was found and must be avoided: `server/services/cli_ai_client.go`'s
`NewBestAvailableAIClient` — an existing helper used by `rules_service.go` — **prefers CLI
backends** (`claude --print`, `gemini`, `agy --print`, `opencode run`) **over the Anthropic HTTP
API**, and none of those CLI invocations pass `--allowedTools`, `--permission-mode`, or
`--mcp-config` at all (`cli_ai_client.go:29-64`) — i.e. they are *more* permissive than even the
disproven `--allowedTools` gating, not less. Completion-reviewer must not call
`NewBestAvailableAIClient`; it must construct `AnthropicAIClient` directly and exclusively.

## Decision

The completion reviewer's restricted call is a direct `AnthropicAIClient.Complete(ctx,
completionReviewSystemPrompt, userPrompt)` call — no `claude` CLI subprocess, no tmux session, no
`session/headless.Pool`, no new MCP server or `--mcp-config` plumbing.

- **Session shape**: none — this is a stateless one-shot HTTPS POST, not a spawned session.
  Resolves the requirements doc's Open Question directly (real tmux session vs. a lighter-weight
  direct API call → the latter, more directly than either research doc anticipated).
- **Credential**: `services.NewDefaultChain(cfg).Resolve(ctx, "anthropic")` →
  `services.NewAnthropicAIClient(cred)`. If resolution fails (no credential configured anywhere
  in the chain), the reviewer no-ops with a logged warning — same shape as
  `session_service.go`'s existing "AI rule generation unavailable" degrade path — and must
  **never** fall back to `NewBestAvailableAIClient`.
- **Enforcement**: the absence of a `Tools` field on `anthropicRequest` is the enforcement
  boundary — a property of the Go type, verified once at compile time and pinned by a marshal-level
  regression test (`TestAnthropicRequestBody_NeverIncludesToolsField` in
  `server/services/completion_reviewer_prompt_test.go`) asserting the JSON body sent for a
  completion-review call never contains a `"tools"` key, for any input. This is the AC's required
  empirical proof test — stronger than an integration test against a real `claude` binary, because
  it does not depend on a CLI version's runtime behavior at all.

## Alternatives Rejected

| Alternative | Rejected because |
|---|---|
| `session/headless.Pool` + new `--mcp-config` exposing one memory-write MCP tool, `AllowedTools` scoped to it, new empirical self-check test | Requires building brand-new infrastructure that doesn't exist today (`--mcp-config` plumbing in `session/headless.ProcessRunner`, a new minimal MCP server/tool, a new self-check) for a weaker guarantee than the zero-new-code alternative below — the enforcement would still ultimately rest on the `claude` CLI's own tool-gating behavior, the exact class of mechanism already disproven for one tool (Bash) in this codebase and never proven for MCP-scoped tools. |
| A brand-new raw Anthropic Messages API HTTP client, built from scratch, per architecture.md/build-vs-buy.md's literal suggestion | Unnecessary — `AnthropicAIClient` already exists, is already tested (`anthropic_client_test.go`), and already solves credential resolution including OAuth reuse. Building a second one would duplicate this and needlessly expand the diff. |
| `session.Instance` (tmux-backed) with `AllowedTools`/`PermissionMode` set to a memory-only allowlist | **Already proven unsafe** in this exact codebase for at least one tool class (Bash) under `bypassPermissions` (`TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`) — never proven safe for MCP tool scoping either. Also inherits tmux orphan-on-restart risk (`.claude/rules/service-restart-orphan-process.md`) for a call that has no need for a persistent pane, resumability, or a UI-visible session. |
| `NewBestAvailableAIClient` (existing helper, prefers CLI backends) | CLI backends it selects (`claude --print`, `gemini`, `agy --print`, `opencode run`) pass **no** tool-restriction flags at all — strictly more permissive than the disproven `--allowedTools` mechanism, not a fallback that degrades safely. |

## Consequences

- Zero new infrastructure: no new MCP server, no new headless-pool plumbing, no new HTTP client,
  no new credential/secret.
- The reviewer's tool-restriction guarantee is provable by a pure unit test (JSON marshal
  assertion), runnable in CI with no real API key and no `claude` binary — stronger CI coverage
  than an integration-tagged test requiring a real `claude` CLI invocation.
- The reviewer depends on `AnthropicAIClient`/`CredentialChain` continuing to have no `Tools`
  field / no tool-use support. If a future feature adds real tool-use to `AnthropicAIClient` (a
  `Tools []anthropicTool` field on `anthropicRequest`), that change would need to either add a
  `Tools: nil` explicit override for this call site or split a new tool-free variant — the pinning
  regression test above is designed to fail loudly if that ever happens silently.
- Costs real Anthropic API usage (subject to whatever rate limits the resolved credential has) —
  no different in kind from the existing AI-rule-generation feature that already uses this same
  client.
