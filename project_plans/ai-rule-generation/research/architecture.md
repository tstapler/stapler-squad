# Architecture Research: GenerateSuggestedRule

## 1. How RulesService Is Structured

`RulesService` (`server/services/rules_service.go`) is a focused domain service with three fields:

```go
type RulesService struct {
    rulesStore     *RulesStore
    analyticsStore *AnalyticsStore
    classifier     *classifier.RuleBasedClassifier
}
```

It already holds all three data sources the AI agent needs:
- `rulesStore.All()` — all user rules as `[]RuleSpec`
- `analyticsStore.LoadWindow(since)` — raw `[]AnalyticsEntry` for any time window
- `classifier.SeedRules()` (package-level function) — built-in seed rules as `[]Rule`

`RulesService` is **not** a ConnectRPC handler directly. It is an internal service instantiated inside `NewSessionService` (line 197) and stored as `SessionService.rulesSvc`. `SessionService` exposes each rules method as a thin pass-through that satisfies the `SessionServiceHandler` interface generated from the proto:

```go
func (s *SessionService) ListApprovalRules(ctx, req) (resp, error) {
    return s.rulesSvc.ListApprovalRules(ctx, req)
}
```

This means **all new rules RPCs must be added in two places**: the method on `RulesService` (the implementation) and the pass-through on `SessionService` (the interface satisfaction). There is no separate `RulesServiceHandler`; the existing `NewSessionServiceHandler` at `server/server.go:297` handles everything via the single `sessionv1connect.SessionService` interface.

## 2. How New RPCs Are Registered

The registration chain is:

1. Add `rpc GenerateSuggestedRule(...)` to `proto/session/v1/session.proto`
2. Run `make generate-proto` — regenerates `gen/proto/go/session/v1/sessionv1connect/session.connect.go` which adds `GenerateSuggestedRule` to the `SessionServiceHandler` interface
3. Add the implementation method to `RulesService` in `server/services/rules_service.go`
4. Add the pass-through to `SessionService` in `server/services/session_service.go`
5. No changes to `server/server.go` — the existing `sessionv1connect.NewSessionServiceHandler` call at line 297 automatically picks up any new methods the interface gains

This pattern is used by every existing rules RPC (`ListApprovalRules`, `UpsertApprovalRule`, `DeleteApprovalRule`, `GetApprovalAnalytics`) and is the only correct path.

**Important**: The server's `WriteTimeout` is set to `0` (no write timeout), so long-running unary RPCs do not time out at the transport layer. Only the client-supplied context deadline matters.

## 3. Proto Definition for ApprovalRuleProto

`ApprovalRuleProto` is defined in `proto/session/v1/types.proto` (line 887):

```protobuf
message ApprovalRuleProto {
  string id = 1;
  string name = 2;
  string tool_name = 3;
  string tool_pattern = 4;
  string command_pattern = 5;
  string file_pattern = 6;
  AutoDecision decision = 7;
  string risk_level = 8;
  string reason = 9;
  string alternative = 10;
  int32 priority = 11;
  bool enabled = 12;
  string source = 13;
  google.protobuf.Timestamp created_at = 14;
}
```

The new `SuggestedRuleProto` should embed or mirror `ApprovalRuleProto` and add:
- `float confidence = 1` (0–1 agent confidence in the pattern)
- `string explanation = 2` (why these fields were chosen)
- `repeated string source_commands = 3` (sample commands that informed the pattern)

`SuggestedRuleProto` should live in `types.proto` alongside `ApprovalRuleProto`.

## 4. Classifier Seed Rules

`classifier.SeedRules()` (defined at line 738 of `pkg/classifier/classifier.go`) returns ~40+ `classifier.Rule` structs sorted by descending priority. Each rule has:

- `ID`, `Name`, `Source` (`"seed"`)
- `ToolName` (exact), `ToolPattern` (`*regexp.Regexp`), `CommandPattern` (`*regexp.Regexp`), `FilePattern` (`*regexp.Regexp`)
- `Criteria *CommandCriteria` — structured matching (programs, subcommands, flags)
- `Decision` (`AutoAllow` / `AutoDeny` / `Escalate`)
- `RiskLevel`, `Reason`, `Alternative`, `Priority`

For the AI prompt, seed rules should be serialized as JSON or YAML examples. The `ruleToSpec()` helper in `rules_service.go` already converts `classifier.Rule → RuleSpec`, converting `*regexp.Regexp` fields to their `.String()` form. Using `allRuleSpecs()` returns user + seed + claude-settings rules all as `RuleSpec`, which is the simplest context assembly path.

## 5. AnalyticsStore: Available Data

`analyticsStore.LoadWindow(since time.Time)` returns `[]AnalyticsEntry` where each entry has:

| Field | Type | Notes |
|---|---|---|
| `ToolName` | string | e.g., `"Bash"`, `"Write"`, MCP name |
| `CommandPreview` | string | First 200 chars of the command |
| `Decision` | string | `"auto_allow"` / `"auto_deny"` / `"escalate"` / `"manual_allow"` / `"manual_deny"` |
| `RuleID` | string | Empty when no rule matched (coverage gap) |
| `CommandProgram` | string | AST-derived primary program (e.g., `"git"`) |
| `CommandSubcategory` | string | First subcommand (e.g., `"commit"`) |

**Coverage gaps** — entries where `Decision == "escalate"` and `RuleID == ""` — are the primary signal for analytics-driven suggestions. `ComputeSummary` already aggregates these into `TopUncoveredTools` and `TopUncoveredPrograms`. For the AI handler, pass raw gap entries (not just summary stats) so the model sees representative command samples.

After loading, `ReclassifyGaps(entries, rs.classifier)` should be called first (as `GetApprovalAnalytics` does) so that the agent only proposes rules for commands that are *still* unmatched today.

## 6. Latency Handling Recommendation

### Option A: Unary RPC with extended timeout (recommended for V1)

A standard unary `GenerateSuggestedRule` RPC with a 60-second client timeout is the simplest approach and fits the existing architecture without adding new transport primitives.

Rationale:
- The server's `WriteTimeout: 0` means no server-side write deadline; the only limit is the client context.
- All existing rules RPCs are unary. Reusing this pattern avoids adding a new streaming handler registration, new WebSocket bridge path, and new frontend streaming client.
- 5–30 second responses are well within a 60-second deadline; the UI just shows a spinner.
- ConnectRPC unary calls are cancellable from the frontend (`AbortController`) if the user abandons the request.

The RPC definition:
```protobuf
// GenerateSuggestedRule asks an AI agent to propose a new auto-approval rule.
// May take 5–30 seconds; callers should set a 60-second deadline.
rpc GenerateSuggestedRule(GenerateSuggestedRuleRequest) returns (GenerateSuggestedRuleResponse) {}
```

### Option B: Server-streaming RPC (future enhancement)

A `returns (stream GenerateSuggestedRuleEvent)` pattern could stream partial results (e.g., a "thinking" event, then the rule fields as they arrive). This is architecturally sound — `WatchSessions` and `WatchReviewQueue` already use server-streaming with the `StreamingWSBridge` — but adds significant complexity for V1:
- Requires a new handler path registration in `server.go`
- Frontend needs a streaming client (not the standard `createClient` path)
- Partial proto messages for streaming intermediate results need design

**Verdict: Use unary for V1. Add streaming only if users hit the 60-second limit in practice.**

### Option C: Background job with polling (do not use)

Overkill for a user-initiated, on-demand operation. Adds a job queue, polling RPC, and persistent state the requirements explicitly exclude ("Cost control: on-demand only").

## 7. Where to Add GenerateSuggestedRule

### Implementation location: `server/services/rules_service.go`

Add as a new method on `RulesService`. The handler should:

1. **Validate source** — reject unknown `source` enum values immediately.
2. **Assemble context** — call `rs.allRuleSpecs()` (user + seed + claude-settings rules), `rs.analyticsStore.LoadWindow(since)`, and `classifier.SeedRules()` for annotated examples.
3. **Filter to gaps** — for `analytics_gaps` source, call `ReclassifyGaps` then filter to entries with `Decision == "escalate"` and `RuleID == ""`.
4. **Build prompt** — serialize rules as JSON, include top-N gap entries as command samples, include seed rule examples (with their `Reason`/`Alternative` text as style guidance).
5. **Call AI provider** — use a configurable provider interface (see §8 below). Pass the assembled context as the system prompt and the focal command/gap as the user turn.
6. **Validate patterns** — after parsing the AI response, compile `commandPattern`, `toolPattern`, and `filePattern` as regexes. Return `connect.CodeInvalidArgument` if any fail to compile.
7. **Return `SuggestedRuleProto`** — never write to `rulesStore`. Client calls `UpsertApprovalRule` after user confirmation.

### Pass-through in `SessionService`

```go
func (s *SessionService) GenerateSuggestedRule(
    ctx context.Context,
    req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
    return s.rulesSvc.GenerateSuggestedRule(ctx, req)
}
```

## 8. AI Provider Interface

There is no existing AI SDK in this codebase (`go.mod` has no `anthropic`, `openai`, or similar dependency). Introduce a minimal interface in the `services` package:

```go
// RuleAIProvider generates rule suggestions from natural-language context.
// This interface keeps RulesService decoupled from any specific AI SDK.
type RuleAIProvider interface {
    // GenerateRule sends context to the AI and returns a raw JSON string
    // matching the SuggestedRuleProto schema.
    GenerateRule(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
```

Wire via `RulesService.SetAIProvider(p RuleAIProvider)` or as a constructor parameter. When `nil`, `GenerateSuggestedRule` returns `connect.CodeUnimplemented` with a message directing the user to configure an API key. A concrete `AnthropicRuleAIProvider` implementation using `claude-3-5-haiku` (fast, low-cost) should live in `server/services/ai_provider.go`.

## 9. Proto Request/Response Messages

New messages to add to `proto/session/v1/session.proto`:

```protobuf
enum SuggestionSource {
  SUGGESTION_SOURCE_UNSPECIFIED = 0;
  SUGGESTION_SOURCE_ANALYTICS_GAPS = 1;
  SUGGESTION_SOURCE_REVIEW_QUEUE_ITEM = 2;
  SUGGESTION_SOURCE_COMMAND_SAMPLE = 3;
}

message GenerateSuggestedRuleRequest {
  SuggestionSource source = 1;
  // For ANALYTICS_GAPS: time window (default 7, max 90).
  optional int32 window_days = 2;
  // For COMMAND_SAMPLE: the raw command string to analyze.
  string command_sample = 3;
  // For REVIEW_QUEUE_ITEM: the analytics entry ID.
  string analytics_item_id = 4;
}

message GenerateSuggestedRuleResponse {
  SuggestedRuleProto suggestion = 1;
}
```

And in `proto/session/v1/types.proto`:

```protobuf
message SuggestedRuleProto {
  // Pre-filled rule fields — same shape as ApprovalRuleProto.
  string id = 1;          // client-generated before saving; agent returns empty
  string name = 2;
  string tool_name = 3;
  string tool_pattern = 4;
  string command_pattern = 5;
  string file_pattern = 6;
  AutoDecision decision = 7;
  string risk_level = 8;
  string reason = 9;
  string alternative = 10;
  int32 priority = 11;
  // Agent metadata.
  float confidence = 12;
  string explanation = 13;
  repeated string source_commands = 14;
}
```

## 10. Prompt Structure

The system prompt should contain three sections in order:

1. **Schema description** — JSON schema for the response (all fields with types and valid values), plus the instruction to return *only* valid JSON with no prose.
2. **Existing rules** — serialized `allRuleSpecs()` output as a JSON array. Keeps the agent from proposing duplicates and teaches the naming/pattern style.
3. **Seed rule examples** — 3–5 high-quality seed rules (one AutoAllow, one AutoDeny, one Escalate) with their full `Reason`/`Alternative` text as style exemplars.

The user prompt contains:
- For `analytics_gaps`: the top-20 unmatched command previews grouped by tool+program, with counts.
- For `review_queue_item`: the single `AnalyticsEntry` as JSON.
- For `command_sample`: the raw command string plus its tool context.

## 11. Summary of Key Architectural Constraints

| Constraint | Value |
|---|---|
| Handler location | Method on `RulesService`, pass-through on `SessionService` |
| Proto registration | Extend existing `SessionService` proto; no new service |
| Transport | Unary RPC (V1); no new route registration needed |
| Client timeout | 60 seconds (set by frontend `AbortController`) |
| Server write timeout | Unlimited (`WriteTimeout: 0`) |
| No auto-save | Handler must never call `rulesStore.Upsert`; read-only |
| Pattern validation | Compile all regex fields; reject before returning |
| AI SDK | New `RuleAIProvider` interface; no existing SDK in codebase |
| Analytics data access | `analyticsStore.LoadWindow` → `ReclassifyGaps` → filter gaps |
| Seed rules | `classifier.SeedRules()` (package-level) + `allRuleSpecs()` |
