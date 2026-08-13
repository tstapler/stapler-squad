# Research Synthesis: LLM Omnibar

## Decision Required

Design a pluggable LLM intent-parsing system that accepts a natural-language session description and returns structured `SessionIntent` JSON, with three interchangeable backends (Claude CLI subprocess, Anthropic Go SDK, AWS Bedrock), a React UX for editable form pre-fill, and a headless API + MCP tool surface.

## Context

Creating a stapler-squad session requires knowing repo path, branch, and how to phrase a prompt. The omnibar feature lets a developer type `> fix the login bug in auth service` and have the New Session form pre-filled within 3 seconds. The backend is Go; MCP server (`/mcp`) is already running; `NewSessionModal` already exists. There is no mandatory API key — the Claude CLI subscription path must work out of the box.

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| Claude CLI subprocess | `exec.CommandContext` → parse `--output-format json` wrapper | Zero API key needed; cold start 3–6s; no structured output guarantee; JSON parse must handle wrapper |
| Anthropic Go SDK | Direct HTTP to `api.anthropic.com`; native structured outputs via `output_config.format` | Requires `ANTHROPIC_API_KEY`; 2–4s latency; guaranteed schema compliance; cleanest code |
| AWS Bedrock Go SDK | `bedrockruntime.Converse` with `outputConfig.textFormat`; native structured outputs | Requires AWS credentials + model ARN; similar latency to SDK; most operationally complex; enables cost control via AWS billing |

## Dominant Trade-off

**Reliability of structured JSON output vs. zero-config accessibility.**

The CLI subprocess backend is the only path with no required API key, but it cannot use the API's constrained decoding — its JSON parse must handle a wrapper struct and potentially malformed inner content. The SDK and Bedrock backends get guaranteed schema compliance via constrained decoding (100–300ms overhead, cached 24h), but require credentials. This is the central tension the `IntentParser` interface resolves: users without credentials fall back to CLI; users with credentials get the more reliable path.

## Recommendation

**Choose**: Implement all three backends behind the `IntentParser` interface, with **Anthropic Go SDK as the preferred backend** when `ANTHROPIC_API_KEY` is set, **Claude CLI as the default fallback** when no key exists.

**Because**:
1. The `IntentParser` interface isolates backend differences completely — switching is a config change. No architectural reason to choose only one.
2. The SDK backend's native `output_config.format` structured outputs (GA, constrained decoding) eliminates the JSON parse reliability risk that dominates the pitfalls list. For a 3-second latency budget, removing retries is worth the API key requirement.
3. The CLI subprocess path satisfies the "zero API key" requirement from the spec and serves as a universal fallback. Its JSON reliability risk is mitigated by: (a) extracting the `result` field from the wrapper, (b) a `json.Unmarshal` with fallback to a repair pass, and (c) a 10s timeout (not 3s — cold start data shows 3–6s is the norm, with tail latency up to 12s).
4. Bedrock is the lowest priority of the three — it adds credential complexity without user-facing benefit vs. the SDK backend. Implement it third.

**Accept these costs**:
- CLI subprocess has 3–6s cold start (mitigate: pre-warm on server start, show spinner immediately on `>` prefix)
- CLI backend cannot guarantee JSON schema compliance (mitigate: JSON repair + validation layer)
- Bedrock implementation adds AWS credential management complexity (mitigate: document clearly; scope to optional)

**Reject these alternatives**:
- *CLI only*: rejected because no structured output guarantee; 12s tail latency under network degradation; blocks users with API keys from getting a better experience
- *SDK only*: rejected because the spec explicitly requires a zero-API-key path (Claude CLI subscription)
- *Iterative dialog / multi-turn clarification*: rejected per requirements spec (out of scope for v1; adds latency that kills adoption)

## UX Decision: Editable Form Pre-fill

The Linear AI-style **editable form pre-fill** (spinner → open `NewSessionModal` with fields populated → user edits → submit) beats both "immediate creation" (unrecoverable wrong path) and "preview card" (extra interaction step). This is confirmed by VS Code's `showInputBox` pre-fill pattern and Claude API's `input_json_delta` streaming capability (field-by-field progressive fill is technically feasible via partial JSON streaming — a v1.1 enhancement).

**UX decisions locked in**:
1. `>` prefix detection in omnibar → immediate spinner; no submit until parse completes
2. On parse complete → open `NewSessionModal` pre-filled
3. Fields with `confidence < 0.7` highlighted amber (show but not block)
4. `SuggestedSessionID` non-empty → show "Use existing session: [title]" banner
5. Confidence badge pattern (Raycast-style) is **unverified** — implement amber highlight as a simpler alternative

## Architecture Decisions

### A. `IntentParser` Interface + Factory

```go
type IntentParser interface {
    ParseIntent(ctx context.Context, description string, ctx StarterContext) (*SessionIntent, error)
}
```

Factory reads from config: `claude_cli` | `anthropic_sdk` | `bedrock`. Default: `claude_cli`.

### B. Structured Output Strategy by Backend

| Backend | Method | Guarantee |
|---------|--------|-----------|
| Claude CLI | Prompt-based JSON + wrapper extraction + repair | Best-effort |
| Anthropic SDK | `output_config.format` (constrained decoding) | Schema-guaranteed |
| Bedrock | `outputConfig.textFormat.type: json_schema` | Schema-guaranteed |

### C. MCP Tool-Use During Intent Resolution

The LLM can call `list_sessions` and `search_sessions` via the already-running `/mcp` HTTP endpoint. Wire the `MCPServerURL` into `StarterContext`. Tool calls add ~1–2s; cap at 2 tool calls max per parse request. For CLI backend: inject MCP server URL into the system prompt and instruct the model to call it via HTTP (or skip tool-use and inject context directly as `StarterContext` text).

### D. Headless API + MCP Tool

`POST /api/sessions/intent` accepts `{description, execute}`. Same endpoint drives UI pre-fill (via ConnectRPC) and MCP tool `create_session_from_intent`. No separate code paths.

## Open Questions Before Committing

- [ ] **CLI subprocess: does `claude -p --output-format json` work without a project context?** — blocks CLI backend implementation; test with a minimal prompt and no `.claude/` directory present
- [ ] **Streaming JSON partial parse library for Go**: does `buger/jsonparser` or a custom accumulator support progressive `SessionIntent` field population? — blocks v1.1 progressive pre-fill
- [ ] **Claude CLI pre-warm latency**: does calling `claude --version` on server start actually reduce first-invocation latency by pre-loading the Node.js runtime? — blocks decision on whether to add pre-warm
- [ ] **`confidence` field in structured output**: the `output_config.format` schema can include a `confidence` float; does Claude produce calibrated scores or does it always return 1.0? — informs whether amber highlighting is useful

If the open questions about CLI subprocess behavior cannot be answered from docs, a 1-day spike is recommended before writing the ADR.

## Sources

- `project_plans/llm-omnibar/research/findings-stack.md` — backend comparison, Go SDK maturity, subprocess patterns
- `project_plans/llm-omnibar/research/findings-features.md` — UX pattern survey, editable form pre-fill recommendation
- `project_plans/llm-omnibar/research/findings-architecture.md` — `IntentParser` interface design, prompt engineering, MCP tool-use during parsing
- `project_plans/llm-omnibar/research/findings-pitfalls.md` — 21 failure modes across CLI, JSON output, Bedrock, context window, and UX
- Web searches: Anthropic structured outputs docs, Claude CLI startup issues, Bedrock regional availability, VS Code pre-fill UX pattern, Claude API streaming JSON
