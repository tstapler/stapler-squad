# Implementation Plan: AI-Assisted Rule Generation

> Source: requirements.md + research/{stack,features,architecture,pitfalls}.md
> Date: 2026-05-18

---

## Epic 1: Proto + Backend Foundation

### Story 1.1: Proto definition for GenerateSuggestedRule

**Goal:** Extend the proto schema with the new enum, request/response messages, and the RPC declaration. Running `make generate-proto` after this story produces all Go + TypeScript bindings; no hand-written stubs needed.

#### Task 1.1.1 — Add `SuggestionSource` enum and `SuggestedRuleProto` to `types.proto`

- **File:** `proto/session/v1/types.proto`
- **What to add:** After the last existing `message` in the file, insert:

```protobuf
// SuggestionSource identifies what data was used to generate a rule suggestion.
enum SuggestionSource {
  SUGGESTION_SOURCE_UNSPECIFIED = 0;
  SUGGESTION_SOURCE_ANALYTICS_GAPS = 1;
  SUGGESTION_SOURCE_REVIEW_QUEUE_ITEM = 2;
  SUGGESTION_SOURCE_COMMAND_SAMPLE = 3;
}

// SuggestedRuleProto carries a pre-filled rule proposal plus AI metadata.
// It mirrors ApprovalRuleProto fields 1–11 (same field numbers) so the UI
// can reuse ApprovalRuleProto rendering helpers with a simple copy.
message SuggestedRuleProto {
  string name           = 1;
  string tool_name      = 2;
  string tool_pattern   = 3;
  string command_pattern = 4;
  string file_pattern   = 5;
  AutoDecision decision = 6;
  string risk_level     = 7;
  string reason         = 8;
  string alternative    = 9;
  int32  priority       = 10;
  // AI metadata.
  float  confidence        = 11;  // 0.0–1.0; agent's certainty in the pattern
  string explanation       = 12;  // why these fields were chosen
  repeated string source_commands = 13; // up to 20 commands that informed the pattern
  // Conflict detection results (computed server-side).
  repeated string shadowed_by_rule_ids  = 14; // IDs of higher-priority rules that would fire first
  repeated string shadows_rule_ids      = 15; // IDs of lower-priority rules this would suppress
}
```

- **Acceptance criteria:** `make generate-proto` succeeds; `SuggestedRuleProto` and `SuggestionSource` appear in `gen/proto/go/session/v1/types.pb.go` and `web-app/src/gen/session/v1/types_pb.ts`.

#### Task 1.1.2 — Add `GenerateSuggestedRuleRequest/Response` messages and RPC to `session.proto`

- **File:** `proto/session/v1/session.proto`
- **What to add (messages):** Immediately before the closing brace of the file, add:

```protobuf
message GenerateSuggestedRuleRequest {
  SuggestionSource source = 1;
  // For ANALYTICS_GAPS: number of days of history to analyze (1–90, default 7).
  optional int32 window_days = 2;
  // For COMMAND_SAMPLE: the raw command string the user pasted.
  string command_sample = 3;
  // For REVIEW_QUEUE_ITEM: the analytics entry ID of the review item.
  string analytics_item_id = 4;
  // For ANALYTICS_GAPS scoped to a single tool/program: optional filter.
  string tool_name_filter = 5;
  string program_name_filter = 6;
}

message GenerateSuggestedRuleResponse {
  // Multiple suggestions are returned so the user can review a batch at once.
  // Analytics-gaps calls return up to 5 suggestions (one per top gap cluster).
  // Command-sample and review-queue-item calls return exactly 1.
  repeated SuggestedRuleProto suggestions = 1;
}
```

- **What to add (RPC):** In the `SessionService` service block, after the `GetApprovalAnalytics` line:

```protobuf
// GenerateSuggestedRule asks an AI agent to propose a new auto-approval rule.
// Analyzes existing rules, seed examples, and analytics data to produce a
// pre-filled SuggestedRuleProto. May take 5–30 seconds; callers must set a
// 60-second deadline via AbortController.
rpc GenerateSuggestedRule(GenerateSuggestedRuleRequest) returns (GenerateSuggestedRuleResponse) {}
```

- **Run:** `make generate-proto`
- **Acceptance criteria:** `sessionv1connect.SessionServiceHandler` interface gains `GenerateSuggestedRule`; the existing `SessionService` Go struct fails to compile until the pass-through is added (Story 1.3, Task 1.3.3), confirming the interface guard works.

---

### Story 1.2: Separated RulePromptBuilder + AIClient interfaces (ADR-001 Option B)

**Goal:** Implement ADR-001's Option B — a `RulePromptBuilder` interface for pure context assembly and a separate `AIClient` interface for transport. This keeps prompt construction testable without needing an HTTP mock and keeps the AI transport swappable independently. Both interfaces are injected into `RulesService`.

#### Task 1.2.1 — Create `server/services/ai_interfaces.go` with `RulePromptBuilder` and `AIClient`

- **File:** `server/services/ai_interfaces.go` (new file)
- **What to add:**

```go
package services

import "context"

// RulePromptContext carries all domain data needed to build a suggestion prompt.
// Assembled by RulesService before passing to a RulePromptBuilder.
type RulePromptContext struct {
    ExistingRules  []RuleSpec      // user + seed + claude-settings rules
    SeedExamples   []RuleSpec      // hand-picked seed examples for style
    AnalyticsGaps  []AnalyticsGap  // unmatched commands grouped by tool/program
    CommandSample  string          // for COMMAND_SAMPLE source
    ToolNameFilter string          // optional single-tool scope
    ProgramFilter  string          // optional single-program scope
    WindowDays     int
}

// AnalyticsGap groups escalated, rule-less analytics entries by (ToolName, Program).
type AnalyticsGap struct {
    ToolName         string
    Program          string
    Count            int
    RepresentativeCmds []string // up to 5 truncated command previews
}

// RulePromptBuilder assembles system and user prompt strings from domain context.
// Implementations are pure functions — no I/O, no external calls. Fully testable.
type RulePromptBuilder interface {
    BuildSystemPrompt(ctx RulePromptContext) string
    BuildUserPrompt(ctx RulePromptContext) string
}

// AIClient sends assembled prompts to an AI backend and returns the raw response.
// ctx cancellation must abort the outbound request.
type AIClient interface {
    Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
```

- **Acceptance criteria:** File compiles with no external dependencies; both interfaces are in package `services`.

#### Task 1.2.2 — Implement `DefaultRulePromptBuilder` in `server/services/rule_prompt_builder.go`

- **File:** `server/services/rule_prompt_builder.go` (new file)
- **What to add:** A struct `DefaultRulePromptBuilder` implementing `RulePromptBuilder`. All logic from the original plan's `buildAISystemPrompt`/`buildAIUserPrompt` private helpers now lives here:
  - `BuildSystemPrompt`: JSON schema block + existing rules JSON + 5 seed examples (one per decision type) + pattern priority instructions + priority tier legend + "return only JSON, no prose" instruction.
  - `BuildUserPrompt`: for analytics-gaps source, formats top-5 gap clusters (not top-20 — AI output quality degrades with too much data) sorted by count descending, with tool name, program, count, and up to 5 command previews per cluster. For command-sample, includes the raw command. Ends with: "Propose up to 5 rules. Return a JSON array. Do not duplicate any existing rule pattern."
  - Second-pass secret redaction: before including any `CommandPreview` string in the user prompt, run `classifier.ScanForSecrets(preview)` and replace positives with `[REDACTED]`. This is the defense-in-depth pass described in FLAG-1.

- **Acceptance criteria:** Unit tests in `rule_prompt_builder_test.go` covering: (a) system prompt contains "JSON schema", "existing rules", "priority tiers"; (b) user prompt contains tool name and count from fixture gap; (c) a command preview containing `ghp_` is replaced with `[REDACTED]` in the user prompt.

#### Task 1.2.3 — Implement `AnthropicAIClient` in `server/services/anthropic_client.go`

- **File:** `server/services/anthropic_client.go` (new file)
- **What to add:** A struct `AnthropicAIClient` implementing `AIClient`. Holds an `*http.Client` (30-second timeout), the API key, and the model name. `Complete` constructs the Anthropic Messages API request, POSTs to `https://api.anthropic.com/v1/messages`, reads the response, and extracts `content[0].text`. Use `http.NewRequestWithContext` so `ctx.Done()` cancels the in-flight call.

  Key implementation details:
  - Request body: `{"model":"claude-haiku-4-5-20251001","max_tokens":2048,"system":"<systemPrompt>","messages":[{"role":"user","content":"<userPrompt>"}]}`
  - Model: `claude-haiku-4-5-20251001` (latest Haiku — fast, low cost, sufficient for structured JSON output)
  - Headers: `x-api-key: <apiKey>`, `anthropic-version: 2023-06-01`, `Content-Type: application/json`
  - Response parsing: decode into `struct{ Content []struct{ Text string } }` and return `content[0].text`; propagate non-200 as a formatted error.
  - Constructor: `NewAnthropicAIClient(apiKey string) *AnthropicAIClient` — returns error if key is empty.

- **Acceptance criteria:** Unit test `TestAnthropicAIClient_Complete_CancelsOnCtxDone` passes using `httptest.Server` that blocks until context cancels.

#### Task 1.2.4 — Add `AnthropicAPIKey` field to `config/config.go`

- **File:** `config/config.go`
- **What to change:** Add `AnthropicAPIKey string \`json:"anthropicApiKey,omitempty"\`` to the `Config` struct. Add an env-var override in the config loading function: `if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" { cfg.AnthropicAPIKey = v }`. Do not log the key value.
- **Acceptance criteria:** `ANTHROPIC_API_KEY=sk-ant-test ./stapler-squad` starts without panic; `config.AnthropicAPIKey` is non-empty when checked in tests.

#### Task 1.2.5 — Wire `RulePromptBuilder` + `AIClient` into `RulesService`

- **File:** `server/services/rules_service.go`
- **What to change:**
  1. Add `promptBuilder RulePromptBuilder` and `aiClient AIClient` fields to `RulesService`.
  2. Change `NewRulesService` signature: `NewRulesService(rulesStore, analyticsStore, classifier, promptBuilder RulePromptBuilder, aiClient AIClient)`. Either may be `nil` (nil means AI generation unavailable).
  3. Update the call site in `server/server.go` (or wherever `NewRulesService` is called) to construct `NewAnthropicAIClient(cfg.AnthropicAPIKey)` and `&DefaultRulePromptBuilder{}` when `cfg.AnthropicAPIKey != ""`, else pass `nil, nil`.

- **Acceptance criteria:** `NewRulesService(store, analyticsStore, classifier, nil, nil)` compiles and existing tests still pass.

---

### Story 1.3: GenerateSuggestedRule handler in RulesService

**Goal:** Implement the full handler logic — context assembly, AI call, pattern validation, conflict detection — as a read-only method on `RulesService` that never touches `rulesStore.Upsert`.

#### Task 1.3.1 — Add `buildPromptContext` helper to `rules_service.go`

- **File:** `server/services/rules_service.go`
- **What to add:** A private method `(rs *RulesService) buildPromptContext(req *sessionv1.GenerateSuggestedRuleRequest, days int) RulePromptContext` that assembles the `RulePromptContext` struct by calling `rs.allRuleSpecs()`, loading analytics via `rs.analyticsStore.LoadWindow(since)`, grouping escalated rule-less entries into `[]AnalyticsGap` (grouped by ToolName+Program, sorted by count desc, capped at 10 gaps), and applying `ToolNameFilter`/`ProgramNameFilter` when set. This helper contains all domain data assembly — the `RulePromptBuilder` receives only the assembled struct, not the raw stores.

- **Acceptance criteria:** Unit test with a fixture analytics store returns a `RulePromptContext` with non-empty `AnalyticsGaps` when escalated entries exist.

#### Task 1.3.2 — Implement `GenerateSuggestedRule` method on `RulesService`

- **File:** `server/services/rules_service.go`
- **What to add:**

```go
func (rs *RulesService) GenerateSuggestedRule(
    ctx context.Context,
    req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
    // Guard: both interfaces must be configured.
    if rs.promptBuilder == nil || rs.aiClient == nil {
        return nil, connect.NewError(connect.CodeUnimplemented,
            fmt.Errorf("AI rule generation requires ANTHROPIC_API_KEY to be set"))
    }
    if req.Msg.Source == sessionv1.SuggestionSource_SUGGESTION_SOURCE_UNSPECIFIED {
        return nil, connect.NewError(connect.CodeInvalidArgument,
            fmt.Errorf("source is required"))
    }
    days := 7
    if req.Msg.WindowDays != nil && *req.Msg.WindowDays > 0 && *req.Msg.WindowDays <= 90 {
        days = int(*req.Msg.WindowDays)
    }
    promptCtx := rs.buildPromptContext(req.Msg, days)
    systemPrompt := rs.promptBuilder.BuildSystemPrompt(promptCtx)
    userPrompt   := rs.promptBuilder.BuildUserPrompt(promptCtx)
    rawJSON, err := rs.aiClient.Complete(ctx, systemPrompt, userPrompt)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("AI client error: %w", err))
    }
    suggestions, err := rs.parseSuggestions(rawJSON)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("AI returned invalid response: %w", err))
    }
    for _, s := range suggestions {
        rs.attachConflictInfo(s)
    }
    return connect.NewResponse(&sessionv1.GenerateSuggestedRuleResponse{
        Suggestions: suggestions,
    }), nil
}
```

- **Acceptance criteria:** Compiles. Integration test with a mock `AIClient` returning a 2-element JSON array produces a response with `len(Suggestions) == 2`, each with `Confidence > 0`.

#### Task 1.3.3 — Rename `parseSuggestion` → `parseSuggestions` (array response)

- **File:** `server/services/rules_service.go`
- **What to change:** The existing `parseSuggestion(rawJSON string) (*sessionv1.SuggestedRuleProto, error)` plan must handle a JSON array (`[{...}, {...}]`) since the prompt asks for up to 5 rules. Rename to `parseSuggestions` returning `([]*sessionv1.SuggestedRuleProto, error)`. Apply all validation (regex compile, confidence clamp, priority clamp, source_commands cap) per element. Cap the array at 5 items before returning.

#### Task 1.3.4 — Implement `parseSuggestion` helper (pattern validation)

- **File:** `server/services/rules_service.go`
- **What to add:** A private method `(rs *RulesService) parseSuggestion(rawJSON string) (*sessionv1.SuggestedRuleProto, error)` that:
  1. `json.Unmarshal`s the raw JSON into a local struct with snake_case tags matching `SuggestedRuleProto`.
  2. Maps the struct to `*sessionv1.SuggestedRuleProto`.
  3. Validates each pattern field with `regexp.Compile`: if `CommandPattern`, `ToolPattern`, or `FilePattern` is non-empty and fails to compile, return an error naming the field and the invalid pattern.
  4. Clamps `Confidence` to [0.0, 1.0].
  5. Validates `Decision` is one of the known `AutoDecision` enum values.
  6. Validates `Priority` is in [1, 999]; default to 100 if the model returned 0 or out-of-range.
  7. Caps `SourceCommands` at 20 items.

- **Acceptance criteria:** Unit test table covering: (a) valid JSON → proto returned; (b) invalid `commandPattern` regex → error returned; (c) `confidence: 1.5` → clamped to 1.0; (d) `priority: 0` → defaulted to 100.

#### Task 1.3.5 — Implement `attachConflictInfo` helper (shadow detection)

- **File:** `server/services/rules_service.go`
- **What to add:** A private method `(rs *RulesService) attachConflictInfo(s *sessionv1.SuggestedRuleProto)` that:
  1. Iterates `allRuleSpecs()` sorted by priority descending.
  2. For each existing rule with `Priority > s.Priority`: check whether the existing rule's `ToolName`/`ToolPattern` and `CommandPattern` both have non-empty overlap with the suggestion (conservative check: if both patterns are non-empty and share the same `ToolName`, flag it). Append matching rule IDs to `ShadowedByRuleIds`.
  3. For each existing rule with `Priority < s.Priority`: same check in reverse; append to `ShadowsRuleIds`.
  4. Limit each list to 10 IDs to keep the response bounded.

  Note: this is a heuristic (not full semantic overlap), sufficient to surface obvious conflicts without implementing a full regex intersection solver.

- **Acceptance criteria:** Unit test with a fixture rule set where a seed rule at priority 500 overlaps the suggestion at priority 100 → `ShadowedByRuleIds` contains the seed rule ID.

#### Task 1.3.6 — Add `GenerateSuggestedRule` pass-through to `SessionService`

- **File:** `server/services/session_service.go`
- **What to add:** After the existing `GetApprovalAnalytics` pass-through:

```go
func (s *SessionService) GenerateSuggestedRule(
    ctx context.Context,
    req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
    return s.rulesSvc.GenerateSuggestedRule(ctx, req)
}
```

- **Acceptance criteria:** `make build` succeeds (interface fully satisfied); `make test` passes existing suite.

---

### Story 1.4: Secret redaction fix in analytics recording

**Goal:** Fix the critical privacy bug (pitfalls §5a) where secrets detected in commands are still persisted verbatim to analytics before being redacted. This must ship in the same PR as the AI feature because the AI handler reads analytics data and would send secrets to the Anthropic API.

#### Task 1.4.1 — Redact command before analytics recording in `approval_handler.go`

- **File:** `server/services/approval_handler.go` (exact line range: ~150–168)
- **What to change:** In the `ScanForSecrets` branch, before calling `analyticsStore.RecordFromResult`, replace the `payload.Command` field with `"[REDACTED: secret detected]"`:

```go
// Before recording to analytics, clear the command so the secret is not persisted.
sanitizedPayload := *payload // shallow copy
sanitizedPayload.Command = "[REDACTED: secret detected]"
rs.analyticsStore.RecordFromResult(&sanitizedPayload, result, sessionID)
```

  Ensure the original `payload` pointer is not mutated (the handler may use it after this branch for the response). The shallow copy (`*payload`) is safe here because `Command` is a string value, not a pointer.

- **Acceptance criteria:** Unit test `TestApprovalHandler_SecretNotPersistedToAnalytics` — inject a mock `analyticsStore`, fire an approval with a command containing a `ghp_` token, and assert that `RecordFromResult` was called with a payload whose `Command` is `"[REDACTED: secret detected]"`.

#### Task 1.4.2 — Add integration test for redaction + analytics query

- **File:** `server/services/approval_handler_test.go`
- **What to add:** A test that calls the full approval handler with a command containing `ANTHROPIC_API_KEY=sk-ant-test123 curl ...`, then calls `LoadWindow` on the analytics store, and asserts that no entry's `CommandPreview` contains `sk-ant-`.
- **Acceptance criteria:** Test passes; adding a deliberate `sk-ant-` string to the preview without the redaction fix causes the test to fail (verified by temporarily reverting the fix).

---

## Epic 2: Frontend — useGenerateRule hook + SuggestedRuleCard component

### Story 2.1: useGenerateRule hook

**Goal:** Encapsulate all state management for the `GenerateSuggestedRule` RPC in a single reusable hook. All four surfaces (Epics 3–6) import this hook.

#### Task 2.1.1 — Create `web-app/src/lib/hooks/useGenerateRule.ts`

- **File:** `web-app/src/lib/hooks/useGenerateRule.ts` (new file)
- **What to add:**

```typescript
import { useCallback, useRef, useState } from "react";
import {
  GenerateSuggestedRuleRequest,
  SuggestionSource,
} from "@/gen/session/v1/session_pb";
import { SuggestedRuleProto } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";

export interface UseGenerateRuleResult {
  suggestions: SuggestedRuleProto[];  // empty until loaded; up to 5 items
  loading: boolean;
  error: Error | null;
  generate: (req: Partial<GenerateSuggestedRuleRequest>) => Promise<void>;
  cancel: () => void;
  clear: () => void;
}

export function useGenerateRule(): UseGenerateRuleResult {
  const [suggestions, setSuggestions] = useState<SuggestedRuleProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const clientRef = useRef(
    createClient(SessionService, getConnectTransport())
  );

  const generate = useCallback(
    async (req: Partial<GenerateSuggestedRuleRequest>) => {
      abortRef.current?.abort();
      abortRef.current = new AbortController();
      setLoading(true);
      setError(null);
      setSuggestion(null);
      try {
        const resp = await clientRef.current.generateSuggestedRule(
          new GenerateSuggestedRuleRequest(req),
          { signal: abortRef.current.signal, timeoutMs: 60_000 }
        );
        setSuggestions(resp.suggestions ?? []);
      } catch (err) {
        if ((err as Error).name !== "AbortError") {
          setError(err as Error);
        }
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    setLoading(false);
  }, []);

  const clear = useCallback(() => {
    setSuggestions([]);
    setError(null);
  }, []);

  return { suggestions, loading, error, generate, cancel, clear };
}
```

- **Acceptance criteria:** Jest test `useGenerateRule_should_setLoading_During_Fetch` mocks the ConnectRPC client, calls `generate`, and asserts `loading === true` during the call and `false` after.

#### Task 2.1.2 — Add Jest tests for `useGenerateRule`

- **File:** `web-app/src/lib/hooks/useGenerateRule.test.ts` (new file)
- **Tests to add (all using `renderHook` + `@testing-library/react`):**
  - `useGenerateRule_should_setSuggestion_When_RPCSucceeds`
  - `useGenerateRule_should_setError_When_RPCFails`
  - `useGenerateRule_should_clearError_On_SecondGenerate`
  - `useGenerateRule_should_notSetError_When_Cancelled`
  - `useGenerateRule_should_resetLoading_After_Cancel`
- **Acceptance criteria:** All 5 tests pass with `cd web-app && npx jest --no-coverage --testPathPatterns="useGenerateRule"`.

---

### Story 2.2: SuggestedRuleCard component

**Goal:** A self-contained card that displays a proposed rule with all fields editable inline. The user can accept (upserts the rule via `useApprovalRules`) or discard (calls `onDiscard`). This component is used by all four surfaces.

#### Task 2.2.1 — Create `web-app/src/components/sessions/SuggestedRuleCard.tsx`

- **File:** `web-app/src/components/sessions/SuggestedRuleCard.tsx` (new file)
- **What to add:** A React component with props:

```typescript
interface SuggestedRuleCardProps {
  suggestion: SuggestedRuleProto;
  onAccept: (savedRule: ApprovalRuleProto) => void; // called after successful upsert
  onDiscard: () => void;
  /** If true, show the card in a loading skeleton state */
  loading?: boolean;
}
```

  Internal state: a `RuleFormState` initialized from `suggestion` fields. Renders:
  - Confidence badge: colored pill (green ≥0.8, yellow ≥0.5, red <0.5) showing `Math.round(suggestion.confidence * 100)%`
  - Explanation text block (read-only)
  - Source commands collapsible section (read-only, shows up to 5 commands)
  - Conflict warning banner (if `shadowedByRuleIds.length > 0`): "This rule would be shadowed by N higher-priority rule(s): [ids]"
  - Shadow warning banner (if `shadowsRuleIds.length > 0`): "This rule would suppress N lower-priority rule(s): [ids]"
  - Editable form fields: name, toolName, toolPattern, commandPattern, filePattern, decision radio, riskLevel select, priority number, reason textarea, alternative textarea
  - "Accept & Save" button: calls `upsertRule` from `useApprovalRules` hook; disabled while upserting
  - "Discard" button: calls `onDiscard`

  Import `upsertRule` from `useApprovalRules` (passed via props or via hook inside the component — use hook inside to keep the interface simple).

- **Acceptance criteria:** Component renders without TypeScript errors; Storybook story `SuggestedRuleCard/WithHighConfidence` shows the card with confidence=0.9 and no conflict warnings; `SuggestedRuleCard/WithConflict` shows the shadowed-by banner.

#### Task 2.2.2 — Create `SuggestedRuleCard.css.ts` (vanilla-extract styles)

- **File:** `web-app/src/components/sessions/SuggestedRuleCard.css.ts` (new file)
- **What to add:** vanilla-extract `style()` and `recipe()` rules (per CSS architecture ADR-009):
  - `confidenceBadge` recipe with variants `{ level: { high, medium, low } }` using `vars.color.*` tokens
  - `conflictBanner` style using `vars.color.statusWarning` / `vars.color.statusDanger` backgrounds
  - `sourceCommandsBlock` style for the collapsible code block (monospace, muted background)
  - `cardContainer` style with border, padding, `vars.radii.md`
  - Do not hardcode any hex values; do not use CSS modules.
- **Acceptance criteria:** `make lint` passes with no CSS variable errors; `cd web-app && npx tsc --noEmit` passes.

#### Task 2.2.3 — Add Jest/RTL tests for `SuggestedRuleCard`

- **File:** `web-app/src/components/sessions/SuggestedRuleCard.test.tsx` (new file)
- **Tests to add:**
  - `SuggestedRuleCard_should_showConfidenceBadge_When_Rendered`
  - `SuggestedRuleCard_should_showConflictWarning_When_ShadowedByRulesPresent`
  - `SuggestedRuleCard_should_callUpsertRule_When_AcceptClicked`
  - `SuggestedRuleCard_should_callOnDiscard_When_DiscardClicked`
  - `SuggestedRuleCard_should_allowEditingCommandPattern_Before_Accept`
- **Acceptance criteria:** All 5 tests pass with `cd web-app && npx jest --no-coverage --testPathPatterns="SuggestedRuleCard"`.

---

## Epic 3: Surface 1 — Rules Page "Generate Suggestions" button

**Goal:** Add a "Generate Suggestions" button to `ApprovalRulesPanel` that calls `GenerateSuggestedRule` with `source: ANALYTICS_GAPS` and shows a list of `SuggestedRuleCard` instances.

### Story 3.1: Add generate button and suggestions list to `ApprovalRulesPanel`

#### Task 3.1.1 — Add `useGenerateRule` hook and state to `ApprovalRulesPanel.tsx`

- **File:** `web-app/src/components/sessions/ApprovalRulesPanel.tsx`
- **What to change:**
  1. Import `useGenerateRule` and `SuggestedRuleCard`.
  2. Call `const { suggestion, loading, error, generate, cancel, clear } = useGenerateRule()` inside the component.
  3. Add a "Generate Suggestions" button adjacent to the existing analytics mini-bar at the top of the panel. Button label: "Generate Suggestions" when idle; "Generating..." (disabled) when `loading`; "Cancel" (calls `cancel()`) when loading.
  4. When `suggestion` is non-null, render a `<SuggestedRuleCard>` above the rules list with `onAccept={() => { clear(); refresh(); }}` and `onDiscard={clear}`.
  5. When `error` is non-null, render the error message in a dismissible error banner below the button.
  6. Pass `{ source: SuggestionSource.ANALYTICS_GAPS, windowDays: selectedWindowDays }` to `generate` (use the existing `windowDays` state from `useApprovalAnalytics` if available, else default 7).

- **Acceptance criteria:** Playwright e2e test `rules-generate-suggestions.spec.ts` — click "Generate Suggestions", verify loading state appears, mock RPC returns a fixture suggestion, verify `SuggestedRuleCard` appears with the fixture name.

#### Task 3.1.2 — Create e2e test `tests/e2e/rules-generate-suggestions.spec.ts`

- **File:** `tests/e2e/rules-generate-suggestions.spec.ts` (new file)
- **What to add:**
  ```
  // @feature rules:generate-suggestions
  ```
  Test cases:
  - `rules-generate-suggestions > should show loading state when Generate Suggestions clicked`
  - `rules-generate-suggestions > should show SuggestedRuleCard when suggestion returned`
  - `rules-generate-suggestions > should hide card when Discard clicked`
  - `rules-generate-suggestions > should add rule to list when Accept clicked`
- **Acceptance criteria:** Tests pass against the test server (`http://localhost:8544`); no `waitForTimeout` calls; locators use `data-testid` or ARIA roles only.

---

## Epic 4: Surface 2 — Review Queue "Create Rule from This"

**Goal:** Add a "Create Rule from This" button to each approval-pending review queue item in `ReviewQueuePanel`. The button opens a modal that displays a `SuggestedRuleCard` pre-filled by `GenerateSuggestedRule`.

### Story 4.1: Add "Create Rule from This" button and modal to `ReviewQueuePanel`

#### Task 4.1.1 — Add button to the `itemActions` div in `ReviewQueuePanel.tsx`

- **File:** `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- **What to change:**
  1. In the `itemActions` div (rendered per review item, ~line 630), add a third button "Create Rule from This" rendered only when `metadata?.["pending_approval_id"]` is set.
  2. On click, open a modal (follow the `prModal` pattern at lines 731–845) and call `generate` with:
     ```
     { source: SuggestionSource.REVIEW_QUEUE_ITEM,
       commandSample: metadata["tool_input_command"],
       toolNameFilter: item.toolName }
     ```
     (Use `analytics_item_id` if the analytics entry ID is available in metadata; fall back to `command_sample` source if not.)
  3. The modal contains: a loading spinner while `loading`, an error message if `error`, or a `SuggestedRuleCard` when `suggestion` is non-null.
  4. `onAccept` closes the modal and shows an inline success toast ("Rule saved").
  5. `onDiscard` closes the modal without saving.

- **Note:** Do not share the `useGenerateRule` hook instance between items — each item that is open has its own modal, so one hook instance per modal render is correct behavior (the hook is stateless between calls).

- **Acceptance criteria:** Clicking "Create Rule from This" on an approval-pending item opens the modal; the loading state is shown; a successful mock response renders `SuggestedRuleCard` in the modal.

#### Task 4.1.2 — Create e2e test `tests/e2e/review-queue-create-rule.spec.ts`

- **File:** `tests/e2e/review-queue-create-rule.spec.ts` (new file)
- **What to add:**
  ```
  // @feature rules:create-from-review-queue
  ```
  Test cases:
  - `review-queue-create-rule > should show Create Rule button only on pending-approval items`
  - `review-queue-create-rule > should open modal with loading state when button clicked`
  - `review-queue-create-rule > should close modal when Discard clicked`
- **Acceptance criteria:** Same conventions as Story 3.1 Task 3.1.2.

---

## Epic 5: Surface 3 — Analytics Gap "Suggest Rule"

**Goal:** Replace the plain "Add rule →" links in `ApprovalAnalyticsPanel`'s coverage-gap tables with "Suggest Rule" icon buttons that trigger `GenerateSuggestedRule` scoped to the specific tool/program row. Show `SuggestedRuleCard` inline below the row.

### Story 5.1: Add "Suggest Rule" button to coverage-gap rows in `ApprovalAnalyticsPanel`

#### Task 5.1.1 — Replace "Add rule →" with "Suggest Rule" in uncovered-tools table

- **File:** `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`
- **What to change (uncovered tools table, ~line 364):**
  1. Replace `<a href="/rules">Add rule →</a>` with a button element `data-testid="suggest-rule-tool-{toolName}"` labeled "Suggest Rule".
  2. On click, call `generate({ source: SuggestionSource.ANALYTICS_GAPS, toolNameFilter: toolName, windowDays })`.
  3. Track which row is loading via `activeSuggestToolName: string | null` local state.
  4. Below the row (or as an expansion panel), render a `SuggestedRuleCard` when `suggestion` is non-null and the active tool name matches the row.
  5. Keep the existing navigation behavior as a fallback: add a small "or Add manually →" link alongside the button.

#### Task 5.1.2 — Replace "Add rule →" with "Suggest Rule" in uncovered-programs table

- **File:** `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`
- **What to change (~line 399):** Same pattern as Task 5.1.1 but scoped by `programName`, using `generate({ source: SuggestionSource.ANALYTICS_GAPS, programNameFilter: programName, windowDays })`.

- **Note on state management:** Both tables share the same component. Use a single `useGenerateRule` hook instance at the panel level with a `activeRowKey: string | null` discriminator to track which row's suggestion is shown. Calling `generate` for a different row auto-cancels the previous in-flight request (the hook's `generate` already calls `abortRef.current?.abort()` at the start).

- **Acceptance criteria:** Clicking "Suggest Rule" on a tool row shows the loading state in that row; the "Suggest Rule" button in other rows remains enabled; `SuggestedRuleCard` appears below the clicked row after the mock response.

#### Task 5.1.3 — Create e2e test `tests/e2e/analytics-suggest-rule.spec.ts`

- **File:** `tests/e2e/analytics-suggest-rule.spec.ts` (new file)
- **What to add:**
  ```
  // @feature rules:suggest-from-analytics
  ```
  Test cases:
  - `analytics-suggest-rule > should show Suggest Rule button when coverage gaps exist`
  - `analytics-suggest-rule > should show SuggestedRuleCard inline when suggestion returned`
  - `analytics-suggest-rule > should cancel previous suggestion when different row clicked`

---

## Epic 6: Surface 4 — Command Sample input in rule creation form

**Goal:** Add an optional "Generate from command" text input in the manual rule creation form (`ApprovalRulesPanel`). Pasting a command calls `GenerateSuggestedRule` with `source: COMMAND_SAMPLE` and pre-fills the form fields.

### Story 6.1: Add command-sample input to rule creation form

#### Task 6.1.1 — Add "Generate from command" textarea to the rule form in `ApprovalRulesPanel.tsx`

- **File:** `web-app/src/components/sessions/ApprovalRulesPanel.tsx`
- **What to change:**
  1. In the rule creation form section (where `RuleFormState` fields are rendered), add a collapsible "Generate from command" expansion section above the Name field.
  2. Inside, add a `<textarea>` with placeholder "Paste a raw command (e.g. git push origin main)" and a "Generate" button alongside it.
  3. On "Generate" button click (or on `onBlur` if the textarea is non-empty), call `generate({ source: SuggestionSource.COMMAND_SAMPLE, commandSample: value })`.
  4. When `suggestion` returns, call a `prefillForm(suggestion)` helper that sets each `RuleFormState` field from the corresponding `SuggestedRuleProto` field. Fields already edited by the user (non-default) are not overwritten — track which fields have been touched via a `touchedFields: Set<keyof RuleFormState>` ref.
  5. Show a small "AI-generated — review before saving" notice badge above the form after prefill.
  6. The existing "Save" / "Cancel" buttons are unchanged; `useGenerateRule` is only used to pre-fill, not to save.

- **Acceptance criteria:** Typing `git push --force origin main` and clicking "Generate" pre-fills `commandPattern` with a pattern matching force-push (per the fixture mock response); the form's "Save" button remains inactive until `name` is filled.

#### Task 6.1.2 — Add Jest test for prefill logic

- **File:** `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx`
- **What to add:** A test `ApprovalRulesPanel_should_prefillFormFields_When_CommandSampleSuggestionReturns` that:
  1. Renders `ApprovalRulesPanel` with a mock `useGenerateRule` returning a fixture `SuggestedRuleProto`.
  2. Types into the command-sample textarea and clicks "Generate".
  3. Asserts that the `commandPattern` input now contains the fixture value.
  4. Asserts that a "AI-generated" notice badge is visible.
- **Acceptance criteria:** Test passes with `cd web-app && npx jest --no-coverage --testPathPatterns="ApprovalRulesPanel"`.

#### Task 6.1.3 — Create e2e test `tests/e2e/rule-from-command-sample.spec.ts`

- **File:** `tests/e2e/rule-from-command-sample.spec.ts` (new file)
- **What to add:**
  ```
  // @feature rules:generate-from-command-sample
  ```
  Test cases:
  - `rule-from-command-sample > should prefill form fields when Generate clicked with command`
  - `rule-from-command-sample > should show AI-generated badge after prefill`
  - `rule-from-command-sample > should not overwrite user-edited fields after prefill`

---

## Cross-Cutting: Feature Registry Updates

### Task X.1 — Update `docs/registry/backend-features.json`

- **File:** `docs/registry/backend-features.json`
- **What to add:** New entry:

```json
{
  "id": "rules:generate-suggested",
  "type": "backend",
  "description": "AI-generated auto-approval rule suggestion",
  "rpc": "GenerateSuggestedRule",
  "markerFound": true,
  "tested": false,
  "testIds": [],
  "lastModified": "2026-05-18T00:00:00Z"
}
```

Update `tested: true` and populate `testIds` once Story 1.3 integration test is written.

### Task X.2 — Update `docs/registry/frontend-features.json`

- **File:** `docs/registry/frontend-features.json`
- **What to add:** Four entries (one per surface):

```json
{ "id": "rules:generate-suggestions-panel", "type": "frontend", "component": "ApprovalRulesPanel", "filePath": "web-app/src/components/sessions/ApprovalRulesPanel.tsx", "tested": false, "testIds": [] },
{ "id": "rules:create-from-review-queue", "type": "frontend", "component": "ReviewQueuePanel", "filePath": "web-app/src/components/sessions/ReviewQueuePanel.tsx", "tested": false, "testIds": [] },
{ "id": "rules:suggest-from-analytics", "type": "frontend", "component": "ApprovalAnalyticsPanel", "filePath": "web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx", "tested": false, "testIds": [] },
{ "id": "rules:generate-from-command-sample", "type": "frontend", "component": "ApprovalRulesPanel", "filePath": "web-app/src/components/sessions/ApprovalRulesPanel.tsx", "tested": false, "testIds": [] }
```

Run `make registry-generate` after adding entries.

---

## Implementation Order and Dependencies

```
Story 1.1 → Story 1.2 → Story 1.3 → Story 1.4   (Epic 1 — must be sequential)
Story 1.1 + 1.2 → Story 2.1 → Story 2.2           (Epic 2 — needs generated bindings)
Story 2.1 + 2.2 → Stories 3.1, 4.1, 5.1, 6.1     (Epics 3–6 — parallel after Epic 2)
X.1 + X.2 alongside Epic 1 stories                (registry updates can be batched)
```

The critical path is: **1.1 → 1.2 → 1.3 → 2.1 → 2.2 → 3.1** (minimum viable P0 path; Surfaces 2–4 are parallel after Epic 2).

---

## Flagged Design Choices Requiring Attention

### FLAG-1: Secret redaction scope (Story 1.4)

The pitfalls research identifies that secrets can appear in `CommandPreview` **within the first 200 bytes** — the most common case (e.g., `curl -H "Authorization: Bearer ghp_xxx"`). The proposed fix (Task 1.4.1) redacts the entire command when `ScanForSecrets` fires a positive. This is correct and safe, but there is a second, less obvious path: a command that does NOT trigger `ScanForSecrets` (because the secret is a non-standard format not in the scanner's regex set) but still contains sensitive content. The `buildAIUserPrompt` helper (Task 1.3.2) should run a **second pass** of `ScanForSecrets` on each `CommandPreview` before including it in the prompt, replacing any positives with `[REDACTED]`. This defense-in-depth is not currently in the plan and should be decided before implementation.

### FLAG-2: Conflict detection heuristic vs. semantic match (Story 1.3, Task 1.3.5)

The `attachConflictInfo` implementation uses a conservative heuristic: if both rules share the same `ToolName` and have non-empty `CommandPattern`, flag as a potential conflict. This will produce false positives (two `Bash` rules with completely non-overlapping command patterns are flagged as conflicting). True semantic overlap detection requires regex intersection, which is not implementable with Go's RE2 library. The team must decide: (a) keep the heuristic with a UI caveat ("may overlap"), (b) implement a simple string-containment check (does one pattern's literal substring appear in the other), or (c) omit conflict detection in V1 and add it as a V2 enhancement. The plan currently implements option (a), but the UI copy for the conflict banner needs to reflect the heuristic nature.

### FLAG-3: Single suggestion vs. multiple suggestions per analytics gap (FR-1 vs. US-1)

The requirements specify `GenerateSuggestedRule` returns **one** `SuggestedRuleProto` (singular). But US-1 says the user "reviews each proposal and accepts or discards it individually," implying a **list** of proposals. The current plan models the RPC as single-suggestion and adds a single `SuggestedRuleCard` to the UI. If the team wants multiple suggestions in one call (e.g., cover the top 5 gap clusters in one AI call), the `GenerateSuggestedRuleResponse` must change to `repeated SuggestedRuleProto suggestions = 1` and the frontend hook/component must handle a list. This architectural decision must be made before Story 1.1 (proto definition), as changing the response shape later requires a regenerate cycle and breaking frontend changes. The plan as written takes the single-suggestion interpretation of FR-1; update before cutting Story 1.1 if the multi-suggestion interpretation is correct.
