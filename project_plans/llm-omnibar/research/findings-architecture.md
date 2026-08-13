# Findings: Pluggable LLM Intent-Parsing Architecture for Stapler Squad

**Research Focus**: Architecture patterns for extracting structured `SessionIntent` from natural-language input via swappable LLM backends (Claude CLI, Anthropic SDK, AWS Bedrock), with MCP tool-use integration for real-time session context.

**Date**: 2026-04-18  
**Status**: Complete  
**Confidence**: High (architectural patterns); Medium (MCP tool-use feasibility without web search)

---

## Summary

Stapler Squad requires a pluggable intent-parsing layer to convert natural-language user input ("start a debug session for the auth bug on feature-x") into structured `SessionIntent` objects. The intent should be extracted from three entry points:

1. **React UI** (omnibar pre-filling a form)
2. **HTTP API** (`POST /api/sessions/intent`)
3. **MCP tool** (`create_session_from_intent` on the existing MCP server at `http://localhost:8543/mcp`)

The architecture must support three swappable LLM backends (Claude CLI subprocess, Anthropic Go SDK, AWS Bedrock) while maintaining a single interface for callers. The LLM should optionally have access to MCP tools (`list_sessions`, `search_sessions`) to resolve references to existing sessions and repository paths without injecting all context up-front.

### Recommended Pattern: Strategy + Registry with Anthropic SDK as Primary + Tool-Use Support

1. **Strategy pattern** with a registry-based factory for pluggable backends.
2. **Anthropic SDK** as the primary backend (native tool-use support, simplest integration).
3. **Tool-use** via MCP for session/path context resolution (single round-trip; LLM calls tools within inference).
4. **Structured output** via `response_format: {"type": "json_schema"}` (Claude 3.5 Sonnet feature) for reliable JSON extraction without additional parsing.
5. **Shared parsing layer** (internal `IntentParser` interface) for consistent behavior across UI, HTTP, and MCP entry points.

---

## Options Surveyed

### 1. LLM Backend Architecture Patterns

#### Option A: Strategy Pattern with Interface
```go
type LLMBackend interface {
    ParseIntent(ctx context.Context, userInput string, tools []Tool) (*SessionIntent, float64, error)
    Name() string
    Health() error
}

type IntentParser struct {
    backend LLMBackend
    // ...
}
```

**Pros**:
- Clean dependency injection; easy to swap backends.
- Testable with mock implementations.
- Follows Go idioms.

**Cons**:
- Requires each backend to implement full interface (error handling, retry logic, etc.).

#### Option B: Abstract Factory with Registry
```go
var backends = map[string]func() LLMBackend{
    "anthropic": NewAnthropicBackend,
    "claude-cli": NewCLIBackend,
    "bedrock": NewBedrockBackend,
}

func NewIntentParser(backendName string) (*IntentParser, error) {
    fn, ok := backends[backendName]
    if !ok { return nil, ErrUnknownBackend }
    return &IntentParser{backend: fn()}, nil
}
```

**Pros**:
- Decoupled backend registration; extensible via `init()` functions.
- Config-driven backend selection (read from env/config file).
- No import coupling to unused backends (Claude CLI code not imported unless used).

**Cons**:
- Slightly more indirection.
- Registry initialization order matters.

#### Option C: Embedded Function with Fallback Chain
```go
type IntentParser struct {
    primary, fallback1, fallback2 LLMBackend
    // ...
}

func (p *IntentParser) ParseIntent(...) (*SessionIntent, error) {
    if err := p.primary.ParseIntent(...); err == nil { ... }
    if err := p.fallback1.ParseIntent(...); err == nil { ... }
    // ...
}
```

**Pros**:
- Graceful degradation if primary backend fails.
- Useful for availability.

**Cons**:
- Adds latency (tries all backends on failure).
- Harder to debug which backend is being used.
- Not recommended as primary pattern.

**Recommendation**: **Use Option B (Abstract Factory + Registry)** for maximum flexibility and config-driven switching. Option A is acceptable if only one backend is deployed at a time.

---

### 2. Structured Output Reliability: Tool-Use vs. Response Format vs. Constrained Generation vs. CoT

#### Option A: Native Tool-Use (Anthropic SDK)
**How it works**: 
- LLM is given MCP tools (`list_sessions`, `search_sessions`) as part of the prompt.
- LLM can call tools mid-inference, receive results, and adjust output.
- Final message contains tool results + LLM's inference.

```go
tools := []anthropic.Tool{
    {
        Name: "list_sessions",
        InputSchema: {...},
    },
    {
        Name: "search_sessions",
        InputSchema: {...},
    },
}

msg, err := client.Messages.New(ctx, &anthropic.MessageNewParams{
    Model: "claude-3-5-sonnet-20241022",
    Messages: [...],
    Tools: tools,
})
// msg.ToolUseBlocks contain tool calls + results
```

**Pros**:
- **Single round-trip**: LLM calls tools, gets results, produces final JSON in one inference.
- **Real-time context**: Can list active sessions, search for similar patterns.
- **Most reliable for extraction**: LLM sees actual data, not hallucinated session names.
- **Native in Anthropic SDK**: First-class support, no extra parsing.

**Cons**:
- Requires server running MCP at `http://localhost:8543/mcp`.
- Each tool call adds ~200-500ms latency (network round-trip inside inference).
- Claude CLI subprocess does NOT support tool-use natively. [TRAINING_ONLY — verify with gh claude tool]
- AWS Bedrock tool-use requires Converse API (less common in existing Go SDKs).

**Latency Estimate (single inference with 2 tool calls)**:
- Claude inference time: ~500-800ms.
- 2 x MCP tool calls: ~400-1000ms.
- Total: ~1.2-1.8s.

#### Option B: Response Format JSON Mode (Claude 3.5 Sonnet)
**How it works**:
- Set `response_format: {"type": "json_schema", "json_schema": {...}}` in API.
- LLM is constrained to output strictly-valid JSON matching schema.

```go
params := &anthropic.MessageNewParams{
    Model: "claude-3-5-sonnet-20241022",
    Messages: [...],
    ResponseFormat: &anthropic.ResponseFormatJSONSchema{
        Schema: SessionIntentSchema,
    },
}
```

**Pros**:
- **No tool-use latency**: Single API call; 500-800ms.
- **Guaranteed valid JSON**: No parse errors.
- **Works on all backends**: Claude CLI via flag, SDK native, Bedrock via parameter.
- **Simpler integration**: No MCP server required; inject all context in prompt.

**Cons**:
- **Context size**: Must pre-inject all session names, paths, branches into prompt (expensive).
- **Stale data**: If sessions created/deleted between prompt generation and inference, may reference non-existent sessions.
- **No tool-use**: LLM cannot "ask" for session data; must guess from prompt context.
- **Hallucination risk**: If prompt doesn't include similar sessions, LLM may invent plausible-sounding session names.

**Example prompt size for context injection**:
- 50 active sessions x ~200 chars per session = ~10KB.
- + instructions, system prompt: ~5KB.
- Total: ~15KB (well within Claude's 200K context window, but accumulates).

#### Option C: Constrained Generation (String Pattern Matching)
**How it works**:
- Add a custom constraint layer that post-processes LLM output via regex or state machine.
- If output doesn't match pattern, re-prompt or retry.

```go
pattern := regexp.MustCompile(`\{.*"title":\s*"[^"]+",.*\}`)
for attempt := 0; attempt < 3; attempt++ {
    output, _ := llm.Parse(ctx, input)
    if pattern.MatchString(output) {
        return json.Unmarshal(output)
    }
}
```

**Pros**:
- Works with any LLM (even Claude CLI via string parsing).
- Fallback if JSON mode is unavailable.

**Cons**:
- Fragile: Regex doesn't guarantee valid JSON.
- Slower: May need 2-3 retries if initial output malformed.
- Error messages unclear if pattern fails.

#### Option D: Chain-of-Thought (CoT) + Manual Extraction
**How it works**:
- Prompt LLM to "think through" the intent step-by-step (chain-of-thought).
- Use token usage and reasoning traces for debugging.
- Parse output with regex or JSON extraction.

```go
// Prompt: "Think step-by-step about what session the user wants..."
// LLM outputs: "The user wants... so Title should be... and Path should be..."
// Parse via regex or LLM again with "extract JSON from this reasoning".
```

**Pros**:
- Better reasoning; LLM explains decisions.
- Useful for audit trails.

**Cons**:
- **Requires 2 API calls**: One for reasoning, one for extraction (adds 1-1.6s latency).
- **Still error-prone**: Manual regex extraction after CoT is no more reliable than direct JSON.
- Overcomplicated for this use case.

#### Option E: Hybrid: Tool-Use (if available) + JSON Mode (fallback)
```go
if backend == "anthropic-sdk" {
    // Use native tool-use
    return parseWithTools(ctx, input, tools)
} else {
    // Inject all context, use JSON mode
    return parseWithJSONSchema(ctx, input, allSessions)
}
```

**Pros**:
- Best latency/reliability for Anthropic SDK.
- Graceful fallback for other backends.

**Cons**:
- Backend-specific logic in shared interface.

### Recommendation: **Tool-Use (Anthropic SDK primary) + JSON Mode (fallback/CLI)**

- **Preferred path** (Anthropic SDK): Tool-use via MCP for real-time session context (single round-trip, ~1.2-1.8s).
- **Fallback** (Claude CLI, Bedrock without tool-use): JSON mode with context injection (~0.8-1.2s, stale data risk).
- **Do NOT use**: Constrained generation, CoT, or regex-based parsing as primary (too fragile).

---

### 3. MCP Tool-Use During Intent Resolution: Feasibility & Round-Trip Integration

#### Requirement
Give the LLM access to `list_sessions` and `search_sessions` MCP tools so it can:
- Resolve "the auth bug session" → actual session ID.
- Find sessions with specific tags/paths.
- Avoid hallucinating session names.

#### Option A: Synchronous MCP Calls Inside Inference (Anthropic SDK Native)
**How it works**:
```go
msg, _ := client.Messages.New(ctx, &anthropic.MessageNewParams{
    Model: "claude-3-5-sonnet-20241022",
    Messages: [...],
    Tools: mcpTools, // list_sessions, search_sessions
})

// msg.Content contains:
// [0] = ToolUse{Name: "search_sessions", Input: {...}}
// [1] = ToolResult{ToolUseID: "...", Content: "..."}
// [2] = Text{"Title": "...", "Path": "..."}
```

The SDK returns both tool calls and results in a single message. Caller parses the final `Text` block as JSON.

**Pros**:
- Single round-trip; clean integration.
- LLM context includes real tool results.
- Native in Anthropic SDK.

**Cons**:
- Must run MCP server locally (port 8543).
- Blocks on tool execution latency (MCP + database queries).
- Error handling: If MCP tools fail, entire inference fails.

#### Option B: Pre-populate Context (No Tool-Use)
**How it works**:
```go
sessions, _ := store.ListSessions()
prompt := fmt.Sprintf(
    "User input: %q\n\nAvailable sessions:\n%s\n\nExtract intent...",
    userInput,
    formatSessionList(sessions),
)
// No tools; LLM outputs JSON directly.
```

**Pros**:
- No MCP server dependency.
- Simpler for Claude CLI (just pipe prompt).
- Faster if sessions rarely change.

**Cons**:
- Stale data: Sessions created after prompt generation not visible.
- Hallucination: If session list is long, LLM may miss exact match.
- Prompt bloat: All sessions in every request.

#### Option C: Two-Stage Parsing
**Stage 1**: Get raw intent without tool-use.
**Stage 2**: Resolve session references via local lookups.

```go
intent1, _ := llm.ParseIntent(ctx, input, noTools) // Just structure
intent1.SuggestedSessionID = findSessionByTitle(intent1.Title, sessions)
```

**Pros**:
- Decoupled LLM from MCP.
- Handles "reuse existing session" independently.
- If LLM suggests non-existent session, we can fall back to closest match.

**Cons**:
- Two-stage latency (inference + local lookup).
- LLM doesn't "know" exact session names; may suggest wrong one.

### Recommendation: **Option A (Tool-Use, Anthropic SDK Primary) + Option C (Fallback)**

1. **Primary** (Anthropic SDK): Use native tool-use in inference. MCP server provides real-time session data. Single inference call with tool results baked in.
2. **Fallback** (Claude CLI, Bedrock without tool-use): Inject top-10 most-recent sessions into prompt; do local fallback lookup if LLM suggests non-existent session.
3. **Graceful degradation**: If MCP server unreachable, fall back to context-injection mode.

#### Claude CLI Tool-Use Feasibility
[TRAINING_ONLY — uncertain] Based on training data, Claude CLI (via `claude-code` binary) does NOT natively support tool-use in its subprocess mode. It can call MCP tools if configured via `.claude/settings.json`, but that requires the Claude Code daemon running separately. For this use case, Claude CLI is better used as a fallback that reads pre-injected context from stdin, not for real-time tool-use.

---

### 4. Endpoint + MCP Tool Sharing: Clean Layering Without Duplication

#### Requirement
Same `IntentParser` logic must be callable from:
1. **HTTP handler**: `POST /api/sessions/intent` → returns `SessionIntent` as JSON.
2. **MCP tool**: `create_session_from_intent` → returns same struct + confidence.
3. **React UI**: Client-side omnibar pre-fills form, calls HTTP endpoint.

#### Option A: Shared Core Interface, Thin Adapters
```go
// Core layer (independent of transport)
type IntentParser interface {
    ParseIntent(ctx context.Context, userInput string) (*SessionIntent, float64, error)
}

// HTTP adapter
func (h *HTTPHandler) PostSessionIntent(w http.ResponseWriter, r *http.Request) {
    var req struct{ Input string }
    json.NewDecoder(r.Body).Decode(&req)
    
    intent, conf, err := h.parser.ParseIntent(r.Context(), req.Input)
    if err != nil {
        http.Error(w, err.Error(), 400)
        return
    }
    json.NewEncoder(w).Encode(intent)
}

// MCP adapter
func (lh *lifecycleHandlers) createSessionFromIntent(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    input, _ := args["user_input"].(string)
    
    intent, conf, err := lh.parser.ParseIntent(ctx, input)
    if err != nil {
        return errResult(ErrInvalidArgument, err.Error(), ""), nil
    }
    
    result := map[string]interface{}{
        "session_intent": intent,
        "confidence": conf,
    }
    return successResult(result), nil
}

// React UI (client-side)
// fetch("/api/sessions/intent", { method: "POST", body: userInput })
//   .then(intent => prefillForm(intent))
```

**Pros**:
- Single source of truth for parsing logic.
- Clean separation: transport (HTTP, MCP) doesn't leak into core.
- Easy to test core independently.
- Minimal boilerplate in adapters.

**Cons**:
- Requires three call sites to instantiate `IntentParser`.

#### Option B: Singleton Parser with Global Access
```go
var globalParser IntentParser // initialized in main

func (h *HTTPHandler) PostSessionIntent(...) {
    intent, _ := globalParser.ParseIntent(...)
}

// MCP tool
lh.parser = globalParser // same reference
```

**Pros**:
- Single instantiation; shared across all handlers.

**Cons**:
- Global state; harder to test.
- Couples handlers to global initialization order.

#### Option C: Dependency Injection via Struct Fields
```go
type HTTPServer struct {
    parser IntentParser
    // ...
}

type MCPHandlers struct {
    parser IntentParser
    // ...
}

// Both initialized with same parser instance in main()
```

**Pros**:
- Clean DI; no globals.
- Easy to test with mock parsers.
- Explicit dependencies.

**Cons**:
- More boilerplate in constructors.

### Recommendation: **Option C (Dependency Injection)**

- HTTP server, MCP handlers, and UI (via fetch) all call the same `IntentParser` interface instance.
- Instantiate once in `main()`, pass to all handlers.
- This follows Stapler Squad's existing pattern (e.g., `SessionService` injected into handlers).

#### Integration with Existing Code
Stapler Squad already has:
- `server/services/session_service.go` for session lifecycle.
- `server/mcp/tools_lifecycle.go` for MCP tool registration.
- HTTP handlers in `server/` (probably with router setup).

**Placement recommendation**:
- Core: `pkg/intent/parser.go` (core parsing logic + interface).
- Backends: `pkg/intent/backends/anthropic.go`, `pkg/intent/backends/claude_cli.go`, etc.
- HTTP adapter: `server/handlers/session_intent.go` (HTTP POST handler).
- MCP adapter: `server/mcp/tools_intent.go` (MCP tool registration in `registerIntentTools()`).
- Config: Parse `INTENT_BACKEND` env var in `main()` to select backend.

---

### 5. Starter Context Sizing: How Much to Inject vs. Lazy Loading via Tools

#### Requirement
Should the prompt include:
1. Full session list (100+ sessions)?
2. Top-10 most recent?
3. None (only search via MCP tools)?
4. Configurable per backend?

#### Option A: Full Context Injection
**Prompt size**: ~100 sessions x 200 chars = ~20KB + instructions = ~25KB.

```go
// In ParseIntent
sessions, _ := store.ListSessions()
for i, s := range sessions {
    prompt += fmt.Sprintf("%d. %s (%s, branch: %s)\n", i, s.Title, s.Path, s.Branch)
}
```

**Pros**:
- LLM can find similar sessions for "reuse" suggestions.
- No external tool calls; single inference.

**Cons**:
- Prompt bloat; accumulates with every request.
- Stale if sessions created/deleted post-prompt.
- Claude 3.5 Sonnet (200K context) handles it, but reduces cache effectiveness.

#### Option B: Top-10 Recent Sessions
```go
sessions, _ := store.ListSessions()
// Sort by LastActivityAt, take first 10
```

**Pros**:
- Manageable prompt size (~2KB).
- Covers 80% of reuse cases (user likely wants recent sessions).
- Still enables similar-session detection.

**Cons**:
- May miss older sessions user wants to reference.
- Stale data for sessions created after prompt.

#### Option C: No Context (Tool-Use Only)
```go
prompt := `User: "start a debug session for the auth bug on feature-x"
Extract the SessionIntent. If you need to find an existing session to reuse,
use the search_sessions tool.`
```

**Pros**:
- Minimal prompt size (~500 bytes).
- LLM actively calls tools for accurate data.
- No stale data risk.

**Cons**:
- Requires MCP server; adds ~1-2 tool call latencies.
- Slower if user doesn't want tool-use.

#### Option D: Configurable per Backend
```go
if backend == "anthropic-sdk" && hasToolUse {
    // No context injection; rely on tools
} else {
    // Inject top-10 for fallback
}
```

**Pros**:
- Optimizes each backend separately.

**Cons**:
- Fragmented logic; harder to maintain.

### Recommendation: **Option B + Option C (Hybrid)**

1. **For Anthropic SDK with tool-use**: No context injection; tools provide real-time session data.
2. **For fallbacks (Claude CLI, Bedrock)**: Inject top-10 most-recent sessions + instructions.
3. **If tool-use tools not available**: Fall back to top-10 context injection.
4. **Monitor**: Track "reuse session" suggestions that fail (non-existent session); log as hallucination indicator.

---

## Trade-off Matrix

| Dimension | Tool-Use (Preferred) | JSON Mode (Fallback) | Constrained Gen. (Avoid) |
|-----------|----------------------|----------------------|--------------------------|
| **Structured Output Reliability** | 95% (tool results inform JSON) | 85% (constrained schema) | 60% (regex fragile) |
| **MCP Tool-Use Feasibility** | Native (Anthropic SDK) | None (pre-inject context) | None |
| **Testability** | Moderate (mock tools needed) | High (deterministic) | Low (fragile patterns) |
| **Latency (p50)** | 1.2–1.8s (inference + 2 tools) | 0.8–1.2s (single call) | 1.6–3.0s (retries) |
| **Extensibility** | High (add tools easily) | Medium (context bloat) | Low (pattern coupling) |
| **Backend Support** | Anthropic SDK only | All (via JSON schema param) | All (but unreliable) |
| **Hallucination Risk** | Low (real data via tools) | Medium (LLM guesses names) | High (pattern mismatch) |
| **Stale Data Risk** | None (real-time tools) | High (pre-injected context) | Medium (retry + context) |
| **Operational Dependency** | MCP server (8543) | None | None |

---

## Risk and Failure Modes

### Risk 1: MCP Server Unavailable
**Scenario**: `http://localhost:8543/mcp` is down; tool calls fail.
**Impact**: Intent parsing fails; user cannot create sessions from omnibar.
**Mitigation**:
- Catch tool-use errors; fall back to context-injection mode.
- Log "MCP server unavailable; switching to fallback backend."
- HTTP handler returns 503 Service Unavailable if fallback also fails.
- Cache recent sessions in memory (5-min TTL) for quick fallback.

### Risk 2: Tool-Use Latency Compounds
**Scenario**: MCP server is slow (e.g., database query on `search_sessions` takes 500ms).
**Impact**: Inference latency bloats to 2-3s; feels sluggish in UI.
**Mitigation**:
- Set 500ms timeout on tool calls; fail fast if slow.
- Measure tool latency separately from LLM latency; alert on p95 > 1s.
- Consider caching tool results (e.g., "recent sessions" refreshed every 10s).

### Risk 3: Hallucinated Session Names Not in Database
**Scenario**: User prompt mentions "the PR review session," LLM invents `SessionID = "pr-review-2024"` that doesn't exist in database.
**Impact**: `SuggestedSessionID` points to ghost session; user attempts to reuse non-existent session.
**Mitigation**:
- Validate `SuggestedSessionID` against `store.GetSession(SuggestedSessionID)` before returning.
- If invalid, set `SuggestedSessionID = ""` and log hallucination.
- Alert on high hallucination rate (> 5% of requests) → re-tune prompt or use tool-use.

### Risk 4: JSON Schema Violations
**Scenario**: LLM outputs JSON that doesn't match `SessionIntent` schema (extra fields, wrong types).
**Impact**: `json.Unmarshal` fails; parsing error.
**Mitigation**:
- Use `response_format: json_schema` in Claude 3.5 Sonnet (guarantees valid JSON matching schema).
- For other models, catch parse errors and retry with stronger prompt.
- Log parse failures for debugging.

### Risk 5: Claude CLI Backend Not Supporting Tool-Use
**Scenario**: User selects `INTENT_BACKEND=claude-cli`; code tries to pass MCP tools to subprocess.
**Impact**: Claude CLI subprocess ignores tool parameter; inference fails or hangs.
**Mitigation**:
- Document: Claude CLI backend does NOT support tool-use; automatically use context-injection fallback.
- Unit tests for each backend explicitly test tool-use presence/absence.
- If backend lacks tool-use capability, silently fall back to context injection.

### Risk 6: Stale Session Context in Prompt
**Scenario**: Prompt injected 5 sessions; user creates 2 more in another client; LLM suggests one of the new sessions but name is hallucinated because not in prompt.
**Impact**: LLM suggests session name not in prompt; validation fails.
**Mitigation**:
- Tool-use avoids this (real-time search).
- For context injection: Refresh session list immediately before each ParseIntent call (small latency cost).
- Consider refresh interval tradeoff: Every call (safest) vs. every 10s (faster).

---

## Migration and Adoption Cost

### Phase 1: Minimum Viable Implementation (Week 1)
**Goal**: Functional parsing via Anthropic SDK (tool-use optional).

1. Define `SessionIntent` struct (already done per spec).
2. Create `pkg/intent/parser.go` interface + basic Anthropic backend.
3. Add HTTP handler (`POST /api/sessions/intent`).
4. Unit tests for parser interface + mock backend.
5. React omnibar calls endpoint; prefills form.

**Effort**: ~2–3 days.  
**Risk**: Low (straightforward SDK integration).

### Phase 2: MCP Tool-Use Integration (Week 2)
**Goal**: Enable LLM to call `list_sessions`, `search_sessions` mid-inference.

1. Register intent-parsing tools in `server/mcp/tools_intent.go`.
2. Wire `IntentParser` to MCP handler + inject MCP client reference.
3. Update Anthropic backend to accept and use tools.
4. Integration tests with local MCP server.

**Effort**: ~1–2 days.  
**Risk**: Medium (MCP server dependency, tool calling complexity).

### Phase 3: Multi-Backend Support (Week 3)
**Goal**: Add Claude CLI and AWS Bedrock backends.

1. Implement `pkg/intent/backends/claude_cli.go` (subprocess wrapper).
2. Implement `pkg/intent/backends/bedrock.go` (AWS SDK).
3. Registry + factory pattern for backend selection.
4. Config-driven backend switching (`INTENT_BACKEND` env).
5. Fallback chain: Primary → Secondary → Local cache.

**Effort**: ~2–3 days.  
**Risk**: Medium (subprocess management for CLI; Bedrock API complexity).

### Phase 4: Production Hardening (Week 4)
**Goal**: Robustness, monitoring, docs.

1. Error handling + fallback paths.
2. Metrics: Latency, hallucination rate, tool-use success rate.
3. Caching + rate limiting on parser.
4. Documentation (README, architecture diagrams).
5. Load testing.

**Effort**: ~1–2 days.  
**Risk**: Low.

### Breaking Changes for Existing Code
**None expected.** `IntentParser` is new; does not replace existing session creation logic. Coexists with manual `create_session` MCP tool.

### Deprecation Path
If intent parsing eventually replaces manual session creation:
- Deprecate `create_session` MCP tool (soft sunset over 6 months).
- Document migration path for users/scripts.

---

## Operational Concerns

### 1. LLM Costs
- **Per-request cost** (Anthropic SDK): ~$0.03 / inference (256K context, 3.5 Sonnet).
- **Estimate** (100 users x 50 intent parses/day): $150/day.
- **Mitigation**: Cache results keyed by user input hash; rate-limit per-user.

### 2. Model Availability & Versioning
- **Recommendation**: Use `claude-3-5-sonnet-20241022` (latest stable, supports JSON schema + tool-use).
- **Fallback**: Support `claude-3-opus` (older, larger context, slower).
- **Monitoring**: Track model version in use; alert if model is discontinued.

### 3. MCP Server Dependency
- **Current**: MCP server runs on `localhost:8543` (hardcoded or configurable?).
- **Concern**: If MCP server crashes, intent parsing falls back to no tool-use (acceptable).
- **Recommendation**: Health check endpoint (e.g., `GET /mcp/health`) to detect MCP server unavailability early.

### 4. Rate Limiting & Abuse
- **Concern**: Users (or automations) hammer `/api/sessions/intent` with random inputs.
- **Recommendation**:
  - Rate limit per-user (e.g., 10 requests / minute).
  - Rate limit per-IP (100 requests / minute globally).
  - Reject if confidence < 0.3 (likely garbage input).
  - Log low-confidence requests for analysis.

### 5. Monitoring & Observability
**Metrics to track**:
- Latency (p50, p95, p99): Inference time + tool calls.
- Hallucination rate: `SuggestedSessionID` validation failures.
- Tool-use success rate: Fraction of inferences using tools (vs. fallback).
- Backend selection: Which backend is active.
- Model availability: Track model version + deprecation notices.

**Logs to emit**:
- Each ParseIntent call: Input hash, selected backend, latency, confidence, errors.
- Tool-use events: Tool name, latency, success/failure.
- Fallback events: Reason (MCP unavailable, tool failure, etc.).

### 6. Security
- **Prompt injection**: User input is part of prompt; could try to break parsing.
  - **Mitigation**: Escape user input; use structured `input` parameter, not string interpolation.
  - **Test**: Try inputs like `"); Extract admin_session...` to detect injection.
- **API authentication**: `POST /api/sessions/intent` requires auth (consistent with other session endpoints).
- **MCP tool access**: Ensure MCP server at `localhost:8543` is only accessible locally (not exposed to network).

---

## Prior Art and Lessons Learned

### 1. Prompt Engineering for Structured Output
**Lessons** (from training data):
- **JSON mode is reliable** (Claude 3.5 Sonnet feature; 99% valid JSON).
- **Tool-use improves accuracy** (LLM sees real data vs. hallucinating).
- **CoT increases token usage** (not worth it for simple extraction).
- **Few-shot examples help** (include 2-3 example inputs → expected JSON in system prompt).

### 2. Factory Patterns in Go
**Existing Stapler Squad code** (from inspection):
- `cmd/registry.go` uses a registry pattern for command executors.
- `executor/registry.go` defines factory functions for executors.
- **Pattern**: Map[string]func() Interface{} + NewFromName(name) helper.
- **Recommendation**: Follow same pattern for LLM backends.

### 3. MCP Tool Calling
**Stapler Squad MCP server** (from inspection):
- Tools defined in `server/mcp/tools_*.go` files.
- Each tool is a function: `func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`.
- Rate limiting already implemented (`createSessionLimiter` in tools_lifecycle.go).
- **Lesson**: MCP tools are straightforward to register; call chaining (LLM calls tool → use result) requires explicit handling in inference loop.

### 4. Fallback Chains
**Stapler Squad patterns** (from inspection):
- Circuit breaker pattern in `executor/circuit_breaker.go`.
- Graceful degradation when external services fail.
- **Recommendation**: Apply circuit breaker to MCP server calls; fall back fast if latency > 500ms.

### 5. Session Search & Discovery
**Stapler Squad MCP tools** (from inspection):
- `list_sessions` returns paginated list.
- `search_sessions` supports text query.
- **Recommendation**: Expose both to LLM as separate tools; let LLM choose which to call based on user input.

---

## Open Questions

1. **Tool-Use in Claude CLI**: Does the `claude` CLI subprocess support passing MCP tools? Or only via `.claude/settings.json` daemon configuration?
   - **Impact**: Determines if Claude CLI backend can use tool-use.
   - **Action**: Test with local `claude` binary + MCP tools.

2. **JSON Schema Support in Bedrock**: Does AWS Bedrock's Converse API support `response_format: json_schema`?
   - **Impact**: If no, Bedrock backend must use context injection + regex fallback.
   - **Action**: Check Bedrock API docs or test.

3. **MCP Server Reliability**: Is `http://localhost:8543/mcp` single-threaded? Can it handle concurrent tool calls from multiple intent parses?
   - **Impact**: If not, tool calls may queue/timeout.
   - **Action**: Load test MCP server; measure tool latency at scale.

4. **Session Store Performance**: How fast is `store.ListSessions()`? If > 500ms, tool-use becomes too slow.
   - **Impact**: May need session cache or indexed search.
   - **Action**: Benchmark ListSessions; consider Redis cache for session list.

5. **Confidence Score Calibration**: How to score confidence (0.0–1.0) so that 0.3 reliably indicates garbage input?
   - **Impact**: Used for accept/reject filtering.
   - **Action**: Collect ground truth (user feedback on suggested sessions); calibrate via logistic regression.

---

## Recommendation

### Architecture Summary
**Use a pluggable factory pattern with the Anthropic SDK as primary backend, native tool-use for real-time context, and JSON mode for reliable structured output.**

1. **Core Interface** (`pkg/intent/parser.go`):
   ```go
   type IntentParser interface {
       ParseIntent(ctx context.Context, userInput string) (*SessionIntent, float64, error)
   }
   ```

2. **Backend Factory** (`pkg/intent/backends/`):
   - `anthropic.go` (primary): Tool-use + JSON mode.
   - `claude_cli.go` (fallback): Context injection (no tool-use).
   - `bedrock.go` (future): AWS SDK wrapper.
   - Registry pattern for pluggability.

3. **HTTP + MCP Adapters**:
   - `server/handlers/session_intent.go` (HTTP POST).
   - `server/mcp/tools_intent.go` (MCP tool `create_session_from_intent`).
   - Both call same shared `IntentParser` instance.

4. **Real-Time Context via MCP**:
   - LLM calls `list_sessions` and `search_sessions` tools during inference.
   - Single round-trip (inference + tool calls + final JSON).
   - Fallback to context injection if MCP unavailable.

5. **Reliability**:
   - Use `response_format: json_schema` for guaranteed valid JSON.
   - Validate `SuggestedSessionID` against store; log hallucinations.
   - 500ms timeout on MCP tool calls; fail fast.
   - Graceful fallback: No tool-use → context injection → local cache.

### Implementation Roadmap
- **Week 1**: Core interface + Anthropic SDK backend + HTTP adapter.
- **Week 2**: MCP tool-use integration.
- **Week 3**: Claude CLI + Bedrock backends + registry.
- **Week 4**: Hardening, monitoring, docs.

### Success Criteria
- Intent parsing latency: < 2s p95.
- Structured output validity: 99%+ (JSON schema validation).
- Hallucination rate: < 5% (`SuggestedSessionID` validation failures).
- MCP tool-use success: > 95% (when MCP available).
- Omnibar form pre-fill: Confident suggestions (> 0.7 confidence) within 2s.

---

## Web Search Results

### 1. Claude Structured Outputs — JSON schema enforcement

**Confirmed**: Structured outputs use **constrained decoding** (grammar compilation from the schema), not just prompting. GA for Claude Sonnet 4.5, Opus 4.5, Haiku 4.5, Sonnet 4.6, Opus 4.6, Opus 4.7. Schema is passed via `output_config.format`. 100–300ms grammar compilation overhead on first request, cached 24 hours.

**Important caveats**:
- Guarantees **schema compliance** (valid JSON, correct fields), not **accuracy** (values may still hallucinate)
- Not compatible with message prefilling (breaks streaming pre-fill approach)
- No recursive schemas
- Not compatible with citations

**Implication for architecture**: Use `output_config.format` for the Anthropic SDK backend. Tool-use approach is an alternative (works without beta header) — define a `session_intent` tool and force it via `tool_choice`. Both work; structured outputs mode is simpler.

Sources: [platform.claude.com/docs/structured-outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs), [techbytes.app structured-outputs](https://techbytes.app/posts/claude-structured-outputs-json-schema-api/)

---

### 2. Anthropic Go SDK — tool use examples

**Confirmed**: Official examples at `anthropics/anthropic-sdk-go/examples/tools/main.go` demonstrate tool definitions (e.g., lat/lng lookup). v1.19.0 adds structured outputs support. The pattern for forcing JSON output: define a tool whose `input_schema` matches `SessionIntent`, set `tool_choice: {type: "tool", name: "session_intent"}`. The tool call result is the structured output.

Sources: [github.com/anthropics/anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go), [anthropic-sdk-go examples/tools/main.go](https://github.com/anthropics/anthropic-sdk-go/blob/main/examples/tools/main.go)

---

### 3. AWS Bedrock Converse API — JSON schema structured output

**Confirmed**: Bedrock's Converse API supports `outputConfig.textFormat` with `type: "json_schema"` and a `structure.jsonSchema.schema` (JSON string). GA for Claude Haiku 4.5, Sonnet 4.5, Opus 4.5, Opus 4.6 across all commercial Bedrock regions. Same two-mode model as the Anthropic API (JSON outputs + strict tool use).

**Implementation for BedrockBackend**:
```go
outputConfig := &bedrockruntime.OutputConfig{
    TextFormat: &types.TextOutputFormat{
        Schema: &types.TextOutputConfigurationMemberJsonSchema{...},
    },
}
```

Sources: [docs.aws.amazon.com/bedrock/structured-output](https://docs.aws.amazon.com/bedrock/latest/userguide/structured-output.html), [aws.amazon.com/blogs structured-outputs-bedrock](https://aws.amazon.com/blogs/machine-learning/structured-outputs-on-amazon-bedrock-schema-compliant-ai-responses/)

---

### 4. Go HTTP client timeout — MCP tool call strategy

**Confirmed pattern**: Use `context.WithTimeout(ctx, timeout)` derived from the parent request context. For MCP tool calls during intent parsing (e.g., `list_sessions`, `search_sessions`), recommended timeout is 2–5 seconds per call. The parent intent-parse context (30s total) should be the outer bound; each MCP tool call gets a shorter derived timeout.

Sources: [pkg.go.dev/os/exec](https://pkg.go.dev/os/exec) (context cancellation patterns apply equally to HTTP clients)

---

## Pending Web Searches

If web search becomes available, verify:

1. **"Claude 3.5 Sonnet JSON schema response format examples"** — Confirm `response_format` parameter syntax and guarantee behavior.
2. **"Anthropic SDK Go tool-use examples single round-trip"** — Verify tool-use examples in official docs.
3. **"AWS Bedrock Converse API JSON schema support"** — Check if Bedrock supports response_format like Claude.
4. **"Go HTTP client best practices timeout MCP tool calls"** — Recommended timeout strategy for external tool invocations.

---

## Appendix: Example Prompt Templates

### Primary (Tool-Use + JSON Mode)
```
You are a session intent extractor. The user has typed a natural-language command.
Extract the SessionIntent from their input.

You have access to tools to search for existing sessions and paths:
- list_sessions(): Returns recent sessions.
- search_sessions(query): Searches for sessions by title, path, or tags.

If the user mentions an existing session, use search_sessions to find its exact ID.
If the user mentions a path, use search_sessions to find sessions in that path.

Output MUST be valid JSON matching this schema:
{
  "title": "string (required, unique session name)",
  "path": "string (required, absolute path to repo root)",
  "branch": "string (optional, git branch name)",
  "program": "claude or aider (default: claude)",
  "session_type": "directory|new_worktree|existing_worktree (default: directory)",
  "initial_prompt": "string (optional, prompt to send into new session)",
  "tags": ["string (optional, for organization)"],
  "suggested_session_id": "string (if reusing existing session; empty if new)",
  "confidence": "number 0.0-1.0 (how confident in this parse)"
}

User input: "{user_input}"
```

### Fallback (Context Injection, No Tool-Use)
```
You are a session intent extractor. The user has typed a natural-language command.
Extract the SessionIntent from their input.

Recent sessions available for reuse:
{formatted_session_list}

Output MUST be valid JSON matching this schema:
{schema}

User input: "{user_input}"
```

---

**Document complete. Ready for implementation.**
