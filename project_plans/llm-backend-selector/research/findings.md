# Research findings (code survey, 2026-10-05)

Method: direct code survey (grep/read); no external research agents, none needed.

## Seams
- `headless.PoolClient.CallBlocking` is already the de-facto provider interface; `Pool` (claude) and `GeminiCaller` satisfy it and approval_handler/gate_custom_check declare identical local interfaces. Evolve it, don't replace it.
- `FeatureKey` constants (session/headless/features.go) already identify each feature -> natural per-feature override key.
- `CallOptions` carries WorkDir, Model, AllowedTools, PermissionMode -> maps to capability flags.
- `GeminiCaller.Available()` + `availabilityChecker` + `resolveHeadlessCaller` fallback-to-claude-with-reason pattern is reusable.
- `CLIAgentSpec` already encodes agy (`--print` positional) and opencode (`run` positional) invocation; reuse in a `CLIBackend`.
- Consolette: Anthropic-compatible; claude CLI with `ANTHROPIC_BASE_URL` set (proxy-claude alias) is the lowest-risk implementation — a `ClaudeCLI` backend with env override, rather than a new HTTP client. HTTP `AnthropicAIClient` base URL constant (anthropic_client.go:13) becomes configurable.

## Risks
- Hard-coded sites need `--resume` (instance_claude.go) -> capability `Resume`; only claude(+consolette via claude CLI) supports it. Do not reroute those silently.
- Tool-using calls (WorkDir + AllowedTools) are unsafe on backends lacking tool restriction -> fail closed to claude.
- Free-tier proxy quality/rate limits: per-feature opt-in; default stays claude.
- Settings live: need config store + settings RPC; follow `rollout flags: live-settable` convention (feature-flag RPC/panel).
- Cost: gemini has computeCost; others may be unpriced.
- Test seams: use narrow fakes (FakeRunner exists).

## Open questions
- Consolette model alias map: consolette not reachable this session (MCP failed to connect); verify supported model names against the router.
- Should backlog stage `program` field and selector merge (stage program overrides feature default)? Proposed: stage program wins.
