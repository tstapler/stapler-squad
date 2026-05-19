# ADR-001: RuleAIProvider Interface Design

## Status
Proposed

## Context

The `GenerateSuggestedRule` RPC requires a call to an external AI provider (Anthropic initially) that may take 5–30 seconds. The existing codebase has no AI SDK dependency. The implementation lives in `RulesService` (`server/services/rules_service.go`), which already holds the three data sources the agent needs: `rulesStore`, `analyticsStore`, and `classifier`.

Three design options were evaluated:

**Option A — Simple synchronous interface**

```go
type RuleAIProvider interface {
    GenerateRule(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
```

`RulesService.GenerateSuggestedRule` assembles context (existing rules, analytics gaps, seed examples), serializes them to strings, builds prompts internally, then calls `aiProvider.GenerateRule`. The interface boundary is transport only: the provider receives fully-formed prompt strings and returns raw JSON.

**Option B — Prompt-builder pattern**

Two separate interfaces: `RulePromptBuilder` (assembles context into a `PromptPayload` struct) and `AIClient` (sends the payload to a provider). `RulesService` holds both. Tests can exercise prompt assembly and transport independently.

**Option C — Multi-turn agent conversation**

Use the Claude Managed Agents API for iterative refinement: an agent turn requests clarification or proposes multiple alternatives in sequence. More expressive but requires managing conversation state across turns.

### Constraints from requirements and architecture research

- V1 is explicitly single-shot: "Multi-step agent conversations or iterative refinement" is out of scope (requirements §Out of Scope).
- Latency budget is 5–30s; a 60-second unary RPC with client `AbortController` cancellation is sufficient (research §6).
- The server's `WriteTimeout: 0` means no server-side deadline; only the client context matters.
- No existing AI SDK in `go.mod`; the interface must not pre-commit to any SDK's types.
- Prompt assembly (what context to include, how to serialize rules and analytics) is the domain logic most likely to change as the feature matures. It should be unit-testable without a live AI endpoint.
- A concrete `AnthropicRuleAIProvider` using `claude-3-5-haiku` is the only provider needed for V1; the interface must not over-engineer for providers that do not yet exist.

### Why Option B over Option A

Option A's interface boundary is at the prompt-string level. This means:

1. **Prompt assembly is buried inside `GenerateSuggestedRule`** and cannot be unit-tested without either calling the AI or setting up a mock that inspects raw strings. Verifying that "the system prompt includes the top-20 gap entries" requires parsing strings in tests.
2. **The single `GenerateRule(ctx, systemPrompt, userPrompt string)` signature** forces callers to pre-serialize all context before the interface boundary. Context assembly (which rules to include, how many gap entries, seed rule selection) is domain logic, not transport logic. Mixing it into the handler makes the handler harder to test and change.
3. **Option A is not wrong**, but it makes prompt assembly opaque and couples it to the handler's implementation. Every change to context strategy (e.g., switching from JSON to YAML rule serialization, adding a new analytics field) requires touching the handler, not a focused builder type.

Option B separates two concerns that change at different rates:
- **Context assembly** (`RulePromptBuilder`): changes when context strategy evolves — which analytics fields to include, how many seed examples, serialization format.
- **Transport** (`AIClient`): changes when the provider or model changes — endpoint URL, auth, retry policy, model name.

This gives each concern a clean mock surface in tests and a focused implementation file.

### Why Option C is deferred

Option C (multi-turn agents) requires managing conversation history, handling partial states, and potentially streaming intermediate results back to the client. The requirements explicitly exclude iterative refinement for V1. Introducing this complexity before validating the single-shot pattern would violate YAGNI and add months of work to an unvalidated UX.

## Decision

**Adopt Option B: Prompt-builder pattern with two interfaces.**

Define both interfaces in `server/services/ai_provider.go`:

```go
// RuleContext holds all data assembled for a rule generation request.
// It is the output of RulePromptBuilder and the input to AIClient.
type RuleContext struct {
    SystemPrompt string
    UserPrompt   string
}

// RulePromptBuilder assembles RulesService data into prompts for the AI.
// Implementations are pure functions of their inputs — no I/O, easily unit-tested.
type RulePromptBuilder interface {
    Build(req *RuleGenerationInput) (RuleContext, error)
}

// AIClient sends a RuleContext to an AI provider and returns raw JSON
// matching the SuggestedRuleProto schema.
// Implementations handle network I/O, retries, and auth.
type AIClient interface {
    Complete(ctx context.Context, rc RuleContext) (string, error)
}

// RuleGenerationInput is the assembled domain data passed to RulePromptBuilder.
type RuleGenerationInput struct {
    AllRules       []RuleSpec
    GapEntries     []AnalyticsEntry   // pre-filtered: Decision==escalate, RuleID==""
    SeedExamples   []classifier.Rule  // 3–5 selected exemplars
    Source         sessionv1.SuggestionSource
    CommandSample  string             // non-empty for COMMAND_SAMPLE source
    AnalyticsItem  *AnalyticsEntry    // non-nil for REVIEW_QUEUE_ITEM source
}
```

`RulesService` gains two fields and a constructor parameter:

```go
type RulesService struct {
    rulesStore     *RulesStore
    analyticsStore *AnalyticsStore
    classifier     *classifier.RuleBasedClassifier
    promptBuilder  RulePromptBuilder  // nil → returns CodeUnimplemented
    aiClient       AIClient           // nil → returns CodeUnimplemented
}
```

When either field is nil, `GenerateSuggestedRule` returns `connect.CodeUnimplemented` with a message directing the user to configure an AI provider API key in settings.

### Concrete implementations

- `DefaultRulePromptBuilder` in `server/services/rule_prompt_builder.go` — pure, no I/O, fully unit-testable.
- `AnthropicAIClient` in `server/services/anthropic_ai_client.go` — wraps the Anthropic Go SDK, targets `claude-3-5-haiku-latest` for speed and cost. Reads the API key from config.

### Wiring

`NewRulesService` gains optional functional options or explicit parameters:

```go
func NewRulesService(
    rulesStore *RulesStore,
    analyticsStore *AnalyticsStore,
    classifier *classifier.RuleBasedClassifier,
    promptBuilder RulePromptBuilder, // may be nil
    aiClient AIClient,               // may be nil
) *RulesService
```

`server.go` constructs `AnthropicAIClient` only when the API key is present in config, then passes it (or nil) to `NewRulesService`. This keeps the no-key degradation path explicit at the wiring site.

### Handler flow in `GenerateSuggestedRule`

1. Validate `source` enum — reject unspecified immediately.
2. Assemble `RuleGenerationInput` from `allRuleSpecs()`, `analyticsStore.LoadWindow` + `ReclassifyGaps`, and `classifier.SeedRules()`.
3. Call `rs.promptBuilder.Build(input)` → `RuleContext`.
4. Call `rs.aiClient.Complete(ctx, rc)` → raw JSON string (5–30s, respects `ctx` cancellation).
5. Unmarshal JSON → `SuggestedRuleProto` fields.
6. Validate all regex fields (`commandPattern`, `toolPattern`, `filePattern`) — return `CodeInvalidArgument` if any fail to compile.
7. Return `SuggestedRuleProto`. Never call `rulesStore.Upsert`.

### Latency handling

The handler runs synchronously in the goroutine serving the ConnectRPC request. No background goroutine or polling is needed because:
- `WriteTimeout: 0` on the HTTP server means no server-side write deadline cancels the long call.
- The client sets a 60-second `AbortController` deadline. If the user navigates away, the request context is cancelled and `AIClient.Complete` returns early via `ctx.Done()`.
- This is consistent with how all existing rules RPCs work.

## Consequences

### Positive

- `DefaultRulePromptBuilder` is fully unit-testable without any AI endpoint — tests assert on the assembled `RuleContext` fields, not on raw strings returned from a mock.
- `AnthropicAIClient` can be swapped for any other provider (OpenAI, Gemini, local Ollama) by implementing the four-line `AIClient` interface, with no changes to `RulesService` or the handler.
- The nil-provider fast-path keeps the binary deployable without an API key; the feature degrades gracefully with a clear error message.
- Prompt strategy changes (e.g., switching to YAML serialization, adding a new analytics field, tuning example selection) are isolated to `DefaultRulePromptBuilder` and its tests.
- The `AIClient` interface is narrow (one method, two parameters) — easy to mock with `gomock` or a hand-written stub.

### Negative

- Two interfaces instead of one adds a small amount of indirection; contributors must know to look in both `rule_prompt_builder.go` and `anthropic_ai_client.go` when debugging end-to-end.
- `RuleGenerationInput` is a new struct that must be kept in sync if `RulesService` gains new data sources for context assembly.
- Option A's single `GenerateRule(ctx, systemPrompt, userPrompt)` interface would have been sufficient for V1 if the prompt assembly never needed independent testing; the added abstraction is a bet on that testing value.

### Neutral

- Option C (multi-turn agents) remains architecturally available: if V2 requires iterative refinement, `AIClient.Complete` can be replaced with a stateful conversation interface without changing `RulePromptBuilder` or the handler's context assembly logic.
- The unary RPC transport decision (research §6) is orthogonal to this ADR and remains unchanged: server-streaming is deferred to V2 if the 60-second timeout proves insufficient in practice.
