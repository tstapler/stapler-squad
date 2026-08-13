# Findings: LLM Backend Stack for Omnibar Intent Resolution

## Summary

The LLM-powered omnibar (natural language → structured session parameters) requires supporting three backend LLM providers behind a unified Go interface. Each backend has distinct trade-offs in latency, auth complexity, SDK maturity, and reliability.

**Key Decision**: Recommend **tiered implementation** with Claude CLI as default (zero auth, fastest), Anthropic SDK as secondary (robust fallback), and Bedrock as advanced/enterprise option. Implement via common `LLMClient` interface to enable runtime provider selection without code changes.

**Latency Constraint**: ~3 seconds hard limit (UI is blocking) favors subprocess invocation (Claude CLI), but requires careful timeout management and JSON output validation.

---

## Options Surveyed

### Option 1: Claude CLI Subprocess (`claude -p <prompt>`)

**Overview**: Invoke Claude CLI command-line tool as an `os/exec` subprocess with natural language prompt, parse JSON-formatted response.

#### Pros
- **Zero authentication overhead** — uses user's Claude subscription from `~/.config/claude/claude.json` or environment
- **Fastest path** — no API roundtrip, CLI invocation locally
- **No SDK dependency** — one shell command, minimal import footprint
- **Respects user configuration** — reuses existing Claude CLI config (model selection, session management)
- **Offline-friendly** — if CLI is already authenticated, no network roundtrip required (only applies to initial auth; actual inference still requires internet)
- **Default behavior** — user expects `claude` to "just work"

#### Cons
- **Process spawning overhead** — fork/exec, stdio serialization, shell quoting
- **Output parsing fragility** — must handle both JSON and error-text responses, malformed output
- **No streaming support** — subprocess returns complete response, can't stream tokens
- **Version coupling** — depends on specific Claude CLI output format, subject to breaking changes
- **Platform dependency** — requires `claude` binary in `$PATH`, Windows compatibility unclear [TRAINING_ONLY — verify]
- **JSON output mode variability** — older CLI versions may not support `--output-format json` flag reliably
- **Error debugging harder** — subprocess stderr is harder to integrate into Go error handling
- **No token counting** — can't predict latency for very long prompts without running them

#### Technical Implementation
```go
type CliBackend struct {
    execCmd func(ctx context.Context, name string, args ...string) (*exec.Cmd, error)
}

func (c *CliBackend) Parse(ctx context.Context, userText string) (*SessionParams, error) {
    cmd := exec.CommandContext(ctx, "claude", 
        "--output-format", "json",
        "-p", systemPrompt + "\n\nUser request: " + userText)
    
    var out bytes.Buffer
    cmd.Stdout = &out
    // Handle stderr separately for error messages
    
    if err := cmd.Run(); err != nil {
        // timeout, command not found, non-zero exit
    }
    
    var result SessionParams
    if err := json.Unmarshal(out.Bytes(), &result); err != nil {
        // fallback: attempt recovery parsing
    }
    return &result, nil
}
```

#### Latency Profile
- `exec.CommandContext` overhead: ~10-50ms (process fork)
- CLI startup: ~100-300ms (depending on system load, cached binaries)
- Claude inference: ~1-2 seconds (typical for quick classification)
- JSON parsing: ~1-5ms
- **Total typical**: 1.1-2.4s (well under 3s limit)
- **Worst case (cold start)**: ~2.5s + variance in inference

#### Failure Modes
1. `claude` binary not found → return error, allow fallback to SDK
2. JSON output unparseable → attempt fallback JSON recovery, log malformed output
3. Subprocess timeout (>3s) → context timeout kills process, return error
4. CLI config not found → check $PATH, suggest installation to user

---

### Option 2: Anthropic Go SDK Direct API (`github.com/anthropics/anthropic-sdk-go`)

**Overview**: Direct HTTP API calls to Anthropic using native Go SDK, use tool-use or JSON output mode for structured response.

#### Pros
- **Mature SDK** — maintained by Anthropic, frequently updated, wide Go adoption
- **Tool-use support** — native structured output via Claude's tool-use feature
- **JSON mode** — native `model: "claude-3-5-sonnet-20241022", output_type: "json"` for deterministic JSON
- **Streaming support** — if needed for token visualization (future enhancement)
- **Full API control** — access to all Claude capabilities (temperature, max_tokens, system prompts, caching)
- **Better error handling** — structured API errors, known failure modes
- **Token counting** — `CountTokens` API for precise latency prediction
- **Transparent** — full control over API calls, easier debugging

#### Cons
- **Requires ANTHROPIC_API_KEY** — environment variable or secrets manager
- **API roundtrip latency** — ~200-500ms network latency + inference time
- **Higher operational overhead** — keys rotation, rate limit handling, quota management
- **Vendor lock-in** — tightly coupled to Anthropic API schema (less risky than local CLI version coupling, more risky than subprocess)
- **Cost** — API usage is metered per token (free tier users must use Claude CLI instead)
- **SDK breaking changes** — must track Anthropic SDK updates (currently stable, but possible)
- **Debugging requires API monitoring** — harder to trace issues without access to Anthropic dashboard

#### Technical Implementation (Tool-Use)
```go
type AnthropicBackend struct {
    client *anthropic.Client
    model  string // e.g., "claude-3-5-sonnet-20241022"
}

func (a *AnthropicBackend) Parse(ctx context.Context, userText string) (*SessionParams, error) {
    // Define tool schema for structured output
    tools := []anthropic.Tool{
        {
            Name: "create_session",
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "title": {"type": "string", "description": "Session title"},
                    "path": {"type": "string", "description": "Git repository path"},
                    "branch": {"type": "string", "description": "Branch name"},
                    "prompt": {"type": "string", "description": "Initial prompt"},
                    "tags": {"type": "array", "items": map[string]string{"type": "string"}},
                },
                "required": []string{"title", "path"},
            },
        },
    }
    
    resp, err := a.client.Messages.New(ctx, &anthropic.MessageParam{
        Model: anthropic.String(a.model),
        MaxTokens: anthropic.Int(1024),
        Tools: tools,
        Messages: []anthropic.MessageParam{
            {
                Role: anthropic.RoleUser,
                Content: anthropic.String(systemPrompt + "\n\nUser request: " + userText),
            },
        },
    })
    
    // Extract tool use block
    for _, block := range resp.Content {
        if toolUse, ok := block.(*anthropic.ToolUseBlock); ok && toolUse.Name == "create_session" {
            var result SessionParams
            json.Unmarshal(toolUse.Input, &result)
            return &result, nil
        }
    }
    return nil, errors.New("no tool use in response")
}
```

#### Technical Implementation (JSON Mode)
```go
// Simpler: use native JSON output mode (Claude 3.5+)
resp, err := a.client.Messages.New(ctx, &anthropic.MessageParam{
    Model: anthropic.String(a.model),
    MaxTokens: anthropic.Int(1024),
    Thinking: anthropic.BudgetTokens(5000),
    Messages: []anthropic.MessageParam{
        {
            Role: anthropic.RoleUser,
            Content: anthropic.String(`Return ONLY valid JSON: {
"title": "...", "path": "...", "branch": "...", "prompt": "...", "tags": [...]
}

User request: ` + userText),
        },
    },
})

// Extract text, parse JSON
```

[TRAINING_ONLY — verify whether `output_type: "json"` is available in Go SDK]

#### Latency Profile
- Network latency: ~150-300ms (Anthropic US data centers)
- Inference time: ~1-2 seconds (typical)
- Response parsing: ~1-5ms
- **Total typical**: 1.2-2.4s (acceptable, close to CLI)
- **Worst case (high load)**: ~3-4s (may exceed budget)

#### Failure Modes
1. Missing ANTHROPIC_API_KEY → error at startup (validation before use)
2. Invalid API key → HTTP 401, clear error message
3. Rate limiting (429) → backoff retry or fallback to CLI
4. Network timeout → context timeout, fallback
5. Malformed response → return structured error, log response for debugging

---

### Option 3: AWS Bedrock (Claude via `github.com/aws/aws-sdk-go-v2/service/bedrockruntime`)

**Overview**: Invoke Claude model via AWS Bedrock service using AWS SDK v2, requires AWS credentials and model ARN.

#### Pros
- **Multi-model support** — can swap Claude versions, access other AWS foundation models
- **Enterprise features** — CloudTrail logging, VPC endpoints, cost allocation tags
- **Managed service** — AWS handles scaling, no quota management (within account limits)
- **Cross-region failover** — can configure multi-region for HA (advanced)
- **AWS ecosystem integration** — if user is already AWS customer, no new auth scheme
- **Custom model endpoints** — can fine-tune or use private models

#### Cons
- **Complex authentication** — requires AWS credentials (IAM user, role, or STS tokens)
- **Model ARN required** — must specify full model identifier per region (fragile)
- **Higher latency potential** — added AWS service overhead + regional routing
- **Cost** — pay per inference unit, no free tier (unlike CLI subscription)
- **SDK maturity** — AWS SDK v2 is solid, but Bedrock service is newer (launched 2023)
- **Credential management** — .aws/credentials, environment variables, STS, or local IAM
- **Region coupling** — model ARNs are region-specific, must handle regional failover
- **Overkill for most users** — adds architectural complexity for marginal benefit

#### Technical Implementation
```go
type BedrockBackend struct {
    client *bedrockruntime.Client
    modelID string // e.g., "anthropic.claude-3-5-sonnet-20241022-v2:0"
}

func (b *BedrockBackend) Parse(ctx context.Context, userText string) (*SessionParams, error) {
    payload := map[string]interface{}{
        "anthropic_version": "bedrock-2023-06-01",
        "max_tokens": 1024,
        "messages": []map[string]string{
            {
                "role": "user",
                "content": systemPrompt + "\n\nUser request: " + userText,
            },
        },
    }
    
    payloadBytes, _ := json.Marshal(payload)
    
    resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
        ModelId:     aws.String(b.modelID),
        ContentType: aws.String("application/json"),
        Body:        payloadBytes,
    })
    
    if err != nil {
        return nil, fmt.Errorf("bedrock invoke failed: %w", err)
    }
    
    var result map[string]interface{}
    json.Unmarshal(resp.Body, &result)
    
    // Extract content.text, parse JSON from Claude response
    if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
        if text, ok := content[0].(map[string]interface{})["text"].(string); ok {
            var params SessionParams
            json.Unmarshal([]byte(text), &params)
            return &params, nil
        }
    }
    return nil, errors.New("malformed bedrock response")
}
```

#### Latency Profile
- AWS credential resolution: ~10-50ms (if using instance role)
- Service roundtrip: ~300-500ms (inter-region latency potential)
- Inference time: ~1-2 seconds (same Claude model)
- Response parsing: ~1-5ms
- **Total typical**: 1.3-2.6s (acceptable, but slower than CLI/SDK)
- **Worst case (region failover)**: ~4-5s (may exceed budget)

#### Failure Modes
1. Missing AWS credentials → boto3-style error, suggest configuration
2. Invalid model ARN → HTTP 400 (model not available in region)
3. Permission error (AccessDenied) → IAM policy issue, clear error
4. Rate limiting (429) → AWS quota exceeded, use exponential backoff
5. Network timeout → inter-region latency, context timeout

---

## Trade-off Matrix

| Criterion | Claude CLI | Anthropic SDK | Bedrock |
|---|---|---|---|
| **Latency** | 1.1–2.4s (optimal) | 1.2–2.4s (similar) | 1.3–2.6s (slower) |
| **Auth Complexity** | Zero (subscription exists) | 1/5 (env var) | 4/5 (AWS creds, IAM) |
| **Structured Output** | JSON text parsing | Tool-use or JSON mode | Bedrock JSON payload |
| **Error Handling** | Fragile (text parsing) | Robust (SDK validation) | Robust (API errors) |
| **SDK Maturity** | High (CLI stable) | Very High (Anthropic-maintained) | Medium-High (AWS, newer) |
| **Cost** | Included (subscription) | Per-token (API) | Per-token (AWS units) |
| **Requires Setup** | No (if installed) | ANTHROPIC_API_KEY only | AWS credentials + ARN lookup |
| **Streaming** | No | Yes | Yes |
| **Token Counting** | No | Yes (CountTokens API) | No |
| **Vendor Lock-in Risk** | Medium (CLI version coupling) | Low (stable API) | Low (AWS standard) |
| **Typical Failover** | → SDK (if key available) | → CLI (fallback) | → SDK |
| **Best For** | Default path, user convenience | Fallback, full control, advanced features | Enterprise/AWS-first orgs |

---

## Risk and Failure Modes

### High-Risk Scenarios

#### 1. **Latency Timeout (3s budget exceeded)**
- **Scenario**: Inference is slow (heavy load, cold model), causes UI hang
- **Triggers**: High inference latency + network latency + processing
- **Mitigations**:
  - Pre-request validation (max 1000 characters)
  - Request batching (omnibar validates incrementally, not per keystroke)
  - Timeout escalation (500ms → try another backend)
  - Cached responses (for same input strings)

#### 2. **JSON Output Unparseable**
- **Scenario**: LLM hallucinates invalid JSON, omnibar can't parse params
- **Triggers**: Prompt ambiguity, LLM confusion, API error response
- **Mitigations**:
  - Strict JSON schema in tool-use (SDK option)
  - JSON validation with clear error messages
  - Fallback to user-provided text input
  - Log unparseable responses for debugging

#### 3. **Backend Unavailable (cascading fallback)**
- **Scenario**: Primary backend fails (CLI not installed, API key expired, AWS region down)
- **Triggers**: Configuration error, credential expiration, network outage
- **Mitigations**:
  - Health check at startup (verify backend availability)
  - Cascading fallback chain (CLI → SDK → Bedrock → manual input)
  - Clear error messages per failure type
  - Fallback to non-LLM session creation (user can type params manually)

#### 4. **Authentication Drift**
- **Scenario**: API keys expire, AWS credentials rotate, user logs out
- **Triggers**: Scheduled credential refresh, session timeout
- **Mitigations**:
  - Periodically refresh/validate credentials (background task)
  - Clear error when auth fails (e.g., "ANTHROPIC_API_KEY expired")
  - Fallback to another backend
  - Log auth failures for debugging (scrub keys)

### Medium-Risk Scenarios

#### 5. **Malformed User Input → LLM Confusion**
- **Scenario**: User types ambiguous query, LLM misinterprets (e.g., `~/work` → is this a path or a label?)
- **Mitigation**: System prompt includes examples of intent (see system prompt design below)
- **Recovery**: Show predicted params to user for confirmation before session creation

#### 6. **Rate Limit Exhaustion**
- **Scenario**: User rapidly types and submits queries, hits API rate limit
- **Triggers**: SDK/Bedrock rate limits, GitHub API limits (if omnibar also queries GitHub)
- **Mitigations**:
  - Debounce LLM calls (300ms delay)
  - Client-side rate limiting (max 10 req/sec)
  - Exponential backoff on 429
  - Fallback to manual input

#### 7. **Cold Start Latency Spikes**
- **Scenario**: CLI subprocess takes 300ms to start, inference takes 2s, exceeds budget
- **Triggers**: First invocation after boot, process cache miss
- **Mitigations**:
  - Warm up subprocess in background (lazy init)
  - Parallel backend invocation (start CLI + SDK simultaneously, use first response)
  - Accept 3-4s on first call (cold start), cache model for subsequent calls

---

## Migration and Adoption Cost

### Development Cost

| Phase | Effort | Notes |
|---|---|---|
| **Week 1** | Define common interface + CLI impl | 3-4 days (straightforward subprocess wrapping) |
| **Week 1** | Anthropic SDK implementation | 2-3 days (tool-use integration) |
| **Week 2** | Bedrock implementation | 2-3 days (AWS SDK boilerplate, model ARN resolution) |
| **Week 2** | Integration tests + fallback chain | 3-4 days (error scenarios, cascading fallback) |
| **Week 3** | System prompt tuning + validation | 2-3 days (LLM output reliability) |
| **Total** | ~2-3 weeks (1 developer, sequential) | ~1 week (3+ developers, parallel) |

### User Adoption Cost

**Tier 1: Claude CLI Users (Default)**
- **Cost**: Zero (works out of the box)
- **Setup**: None required
- **Adoption**: 100% (no barrier)

**Tier 2: API-First Developers**
- **Cost**: One env var (`ANTHROPIC_API_KEY`)
- **Setup**: `export ANTHROPIC_API_KEY=sk-ant-...`
- **Adoption**: Medium (requires API key, but simple)

**Tier 3: Enterprise (AWS-First)**
- **Cost**: AWS credential configuration (if not already set up)
- **Setup**: IAM policy + model ARN lookup (1-2 hours)
- **Adoption**: Low (architectural change, but optional)

---

## Operational Concerns

### Monitoring & Observability

**Metrics to track** (OpenTelemetry):
- `llm.inference_latency_ms` — per backend, histogram
- `llm.json_parse_errors` — counter (malformed responses)
- `llm.backend_availability` — per backend, gauge (health check)
- `llm.fallback_count` — how often fallback chain is used
- `llm.tokens_used` — per backend, if available

**Logging**:
- Log each backend invocation with: `{ backend, duration_ms, status, error_type }`
- Scrub API keys and user-provided text from logs
- Example: `backend=cli duration_ms=1245 status=success`

**Health Checks**:
```go
// At startup and periodically (30s interval)
func HealthCheck() map[string]bool {
    return map[string]bool{
        "cli": fileExists("/usr/local/bin/claude"),
        "sdk": hasEnv("ANTHROPIC_API_KEY") && validate(key),
        "bedrock": hasAWSCreds() && canResolveModelARN(),
    }
}
```

### Caching Strategy

**Query Cache** (avoid redundant LLM calls):
- Cache key: `hash(userText)` (MD5)
- TTL: 5 minutes
- Size limit: 100 entries (LRU)
- Invalidation: Manual or time-based

```go
type QueryCache struct {
    mu    sync.RWMutex
    cache map[string]*CacheEntry
}

type CacheEntry struct {
    Result    *SessionParams
    ExpiresAt time.Time
}
```

**Model Output Consistency**:
- Use `temperature=0` for deterministic JSON parsing
- Add `max_tokens=1024` to bound response size
- Request retry-on-error (auto-retry with exponential backoff)

### Quota Management

**Per-Backend Quotas**:
- **Claude CLI**: Unlimited (user's subscription)
- **SDK**: Track token usage, warn if approaching monthly limit (if applicable)
- **Bedrock**: AWS metered, no client-side limit (AWS enforces)

**Alerts**:
- If API error rate > 10% over 1 minute → log warning
- If latency > 2s consistently → suggest fallback backend
- If backend unavailable → notify user, switch fallback

---

## Prior Art and Lessons Learned

### Similar Architectures in Open Source

**1. Aider.ai (Python)**
- Supports multiple LLM backends (Claude, GPT-4, Ollama)
- Uses provider abstraction pattern (base class + implementations)
- Reads from `~/.config/aider/` for API keys
- Lesson: Clear API contract + easy key rotation

**2. LLaMA.cpp CLI Tools**
- Local subprocess invocation for speed
- JSON mode parsing for structured output
- Fallback to text parsing if JSON unavailable
- Lesson: Always have a text fallback

**3. Anthropic Libraries (various languages)**
- Tool-use is the most reliable structured output method
- JSON mode exists but not universally supported
- Always version-pin SDK to avoid breaking changes
- Lesson: Tool-use is safer than prompt-based JSON

### Common Pitfalls

**Pitfall 1: Assuming JSON output**
- **Issue**: LLM may return markdown, code blocks, or partial JSON
- **Solution**: Always validate with `json.Unmarshal` + try recovery parsing (remove ```json markers, etc.)

**Pitfall 2: Ignoring timeout**
- **Issue**: LLM request hangs indefinitely, UI freezes
- **Solution**: Always use `context.WithTimeout` with hard limit (3s)

**Pitfall 3: No fallback chain**
- **Issue**: Single backend failure = entire feature breaks
- **Solution**: Design for graceful degradation (CLI → SDK → Bedrock → manual)

**Pitfall 4: Coupling to specific prompt format**
- **Issue**: LLM output format changes with model version updates
- **Solution**: Schema-based validation (tool-use), not prompt-based parsing

---

## Open Questions

1. **Should omnibar LLM calls be user-facing or silent?**
   - Current plan: Silent (no UI indication), but should we show "parsing..." spinner?
   - May need UX decision from product

2. **How to handle user rejection of LLM-generated params?**
   - Currently assumed: user accepts predicted params, clicks "Create"
   - Alternative: Show predicted → allow override → then create
   - May need UI mockup clarification

3. **Should we cache system prompt in binary or fetch from server?**
   - Current: Hardcoded system prompt in Go code
   - Alternative: Fetch from `/api/v1/llm-config` (server-controlled, allows A/B testing)
   - Trade-off: Flexibility vs. latency

4. **What if LLM is confident but wrong?**
   - E.g., LLM generates nonexistent branch name, user clicks "Create", git clone fails
   - Mitigation: Show predicted params to user before confirmation?
   - Or: Accept failure, let user fix in created session?

5. **Should we implement token counting for latency prediction?**
   - SDK supports `CountTokens` API (10ms call)
   - Pro: Can predict if response will exceed 3s budget
   - Con: Adds latency to every request (2-step: count → invoke)

6. **How to handle API key rotation gracefully?**
   - Should we periodically validate keys (background check)?
   - Or only check on first use and cache result?

---

## Recommendation

### Proposed Implementation Strategy

**Tier-based backend selection** with cascading fallback:

1. **Primary (Default)**: Claude CLI
   - Zero setup, user's existing subscription
   - Target users: Anyone with Claude CLI installed
   
2. **Secondary (Fallback)**: Anthropic SDK
   - Requires `ANTHROPIC_API_KEY`
   - Target users: API-first developers, free tier users
   
3. **Tertiary (Enterprise)**: AWS Bedrock
   - Requires AWS credentials + model ARN
   - Target users: Enterprise, AWS customers, on-premise deployments

**Implementation Order**:
1. **Week 1**: Define `LLMClient` interface + CLI backend
2. **Week 1**: Add SDK backend (use tool-use for structured output)
3. **Week 2**: Add Bedrock backend (optional for MVP)
4. **Week 2**: Implement cascading fallback + health checks
5. **Week 3**: System prompt tuning + integration tests

**System Prompt Template**:
```
You are a session creation assistant for Stapler Squad (tmux+git session manager).
Parse user natural-language requests into structured session parameters.

Example inputs and expected outputs:
- "api refactor on main" → { "title": "api refactor", "path": ".", "branch": "main" }
- "~/projects/repo@feature" → { "path": "/users/user/projects/repo", "branch": "feature" }
- "clone owner/repo, branch dev" → { "path": "owner/repo", "branch": "dev" }

Always return valid JSON object with keys: title, path, branch, prompt, tags (all optional except path/branch combo).

User request: {USER_INPUT}
```

**Fallback Chain (Priority Order)**:
```go
backends := []LLMClient{
    NewCliBackend(),           // Try CLI first (zero auth)
    NewSDKBackend(key),        // Try SDK if key available
    NewBedrockBackend(cfg),    // Try Bedrock if AWS creds available
}

for _, backend := range backends {
    if available, err := backend.HealthCheck(ctx); available {
        return backend.Parse(ctx, userText)
    }
    // Log failure reason, continue to next
}

// All backends failed, return error + suggest manual input
```

**Common Interface**:
```go
type LLMClient interface {
    Parse(ctx context.Context, userText string) (*SessionParams, error)
    HealthCheck(ctx context.Context) (bool, error)
    String() string // e.g., "Claude CLI", "Anthropic SDK"
}

type SessionParams struct {
    Title  string   `json:"title,omitempty"`
    Path   string   `json:"path"`
    Branch string   `json:"branch,omitempty"`
    Prompt string   `json:"prompt,omitempty"`
    Tags   []string `json:"tags,omitempty"`
}
```

**Error Handling Pattern**:
```go
func (o *OmnibarOverlay) OnSubmit(userText string) {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    
    params, err := o.llm.Parse(ctx, userText)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            o.showError("LLM request timed out, please try again or enter parameters manually")
        } else if errors.Is(err, context.Canceled) {
            o.showError("LLM request was cancelled")
        } else {
            o.showError(fmt.Sprintf("Failed to parse: %v", err))
        }
        o.fallbackToManualInput()
        return
    }
    
    // Validate params
    if err := validateParams(params); err != nil {
        o.showWarning("LLM output validation failed: " + err.Error())
        o.showParamsForConfirmation(params) // Allow user to edit
        return
    }
    
    // Success: create session with params
    o.createSession(params)
}
```

---

## Web Search Results

### 1. Anthropic Go SDK — Structured Output (JSON mode vs tool use)

**Confirmed**: Native structured outputs landed in `anthropic-sdk-go` v1.18.1+ (beta header fix) and v1.19.0. Two modes:
- **`output_config.format`** (JSON outputs mode): pass a JSON schema; response lands in `content[0].text` as guaranteed-valid JSON. The Go SDK passes raw JSON schemas via `output_config`.
- **`strict: true` on tools** (strict tool use): guarantees tool input matches schema.

Structured outputs use **constrained decoding** (grammar compilation), not prompting. 100–300ms overhead on first request per schema; cached 24 hours. GA for Claude Sonnet 4.5, Opus 4.5, Haiku 4.5, Sonnet 4.6, Opus 4.6, Opus 4.7, and Mythos Preview.

Limitations: No recursive schemas, not compatible with message prefilling, not compatible with citations.

Sources: [platform.claude.com/docs/structured-outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs), [anthropics/anthropic-sdk-go releases](https://github.com/anthropics/anthropic-sdk-go/releases)

---

### 2. Claude CLI `--output-format json`

**Confirmed with caveat**: `--output-format json` (with `--print`) returns Claude Code's **wrapper** structure (metadata + content nested inside). It does **not** guarantee the nested response matches a user-specified JSON schema — that's not the same as the API's native structured outputs. For the `IntentParser` use case, the subprocess approach must parse the nested `result` field and handle malformed JSON.

The Claude Code CLI's `--output-format json` is a convenience for scripting, not a schema-enforcement mechanism.

Sources: [claudelog.com/faqs/what-is-output-format](https://claudelog.com/faqs/what-is-output-format-in-claude-code/)

---

### 3. AWS Bedrock Claude model ARNs — Region Availability (2026)

**Confirmed**: Both `us-east-1` and `us-west-2` have 92 models from 17 publishers. Cross-region inference profiles available for global/geo routing. Claude Opus 4.7 launched April 2026 in `us-east-1`, AP Tokyo, EU Ireland, EU Stockholm. Structured outputs (native `outputConfig.textFormat`) GA for Claude 4.5 models in all commercial regions.

Sources: [docs.aws.amazon.com/bedrock/models-regions](https://docs.aws.amazon.com/bedrock/latest/userguide/models-regions.html), [aws.amazon.com claude-opus-4-7](https://aws.amazon.com/about-aws/whats-new/2026/04/claude-opus-4.7-amazon-bedrock/)

---

### 4. Go `os/exec` subprocess JSON parsing — best practices

**Confirmed pattern**:
- Always use `exec.CommandContext(ctx, ...)` for cancellation/timeout
- Start goroutines to drain `stdout` and `stderr` **before** calling `cmd.Start()` — deadlock risk if buffers fill
- JSON decode from a `bytes.Buffer` populated via `cmd.Output()` or a pipe goroutine
- `json.SyntaxError` type-assertion to extract parse location on failure
- Never concatenate user input into the command string — pass args separately

Sources: [pkg.go.dev/os/exec](https://pkg.go.dev/os/exec), [medium.com/@caring_smitten_gerbil_914](https://medium.com/@caring_smitten_gerbil_914/running-external-programs-in-go-the-right-way-38b11d272cd1)

---

## Pending Web Searches

To verify and deepen understanding, the following searches should be performed:

1. **"anthropic-sdk-go json mode vs tool use structured output 2025"**
   - Confirms whether JSON mode is available in current SDK version
   - Compares reliability of tool-use vs. JSON mode
   - Provides examples of each approach

2. **"Claude CLI --output-format json support version requirements"**
   - Verifies minimum Claude CLI version for JSON output mode
   - Documents Windows compatibility status
   - Shows breaking changes in CLI output format

3. **"AWS Bedrock Claude model ARNs region availability 2026"**
   - Lists available model ARNs per region
   - Documents price per inference unit
   - Shows multi-region failover patterns

4. **"Go os/exec subprocess JSON parsing error handling best practices"**
   - Best practices for robust subprocess output parsing
   - Error recovery techniques (malformed JSON, timeout)
   - Platform-specific considerations (Windows vs. Unix)

---

## Implementation Checklist

### Phase 1: Core (Week 1-2)
- [ ] Define `LLMClient` interface + `SessionParams` struct
- [ ] Implement `CliBackend` with JSON parsing + error recovery
- [ ] Implement `SDKBackend` with tool-use structured output
- [ ] Add `HealthCheck()` for each backend
- [ ] Create cascading fallback orchestrator

### Phase 2: Bedrock (Week 2, Optional for MVP)
- [ ] Implement `BedrockBackend`
- [ ] Add AWS credential resolution
- [ ] Model ARN mapping per region

### Phase 3: Integration (Week 2-3)
- [ ] Write comprehensive integration tests (all backends, error scenarios)
- [ ] System prompt tuning (validation, examples)
- [ ] Cache layer (query deduplication, 5-min TTL)
- [ ] OpenTelemetry metrics (latency, errors, fallback usage)
- [ ] Log all backend invocations (scrubbed of keys)

### Phase 4: Polish (Week 3)
- [ ] Documentation (API contract, setup guide per backend)
- [ ] Error message clarity (user-facing, actionable)
- [ ] Performance benchmarks (latency distribution)
- [ ] User acceptance testing (end-to-end flows)

---

**Document Version**: 1.0  
**Date**: 2026-04-18  
**Author**: Claude (LLM Stack Research)  
**Status**: Ready for Implementation Review

