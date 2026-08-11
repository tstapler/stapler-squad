# Stack Research: AI-Assisted Rule Generation

## 1. Go Packages for Claude/Anthropic API

**No Anthropic SDK is present in `go.mod`.** The module has no `github.com/anthropics/anthropic-sdk-go` or equivalent dependency. The only Anthropic reference in the codebase is a regex pattern in `server/services/secret_scanner.go` that detects leaked API keys (`sk-ant-...`).

**Options for adding Claude support:**

- **`github.com/anthropics/anthropic-sdk-go`** — The official Go SDK (released 2024). Supports Messages API, streaming, tool use. Would need to be added via `go get github.com/anthropics/anthropic-sdk-go`. Requires `ANTHROPIC_API_KEY` env var.
- **Raw `net/http`** — The codebase already uses `net/http` extensively (see `domain_checker.go` for the canonical `http.Client` construction pattern with timeout). The Messages API is a straightforward `POST https://api.anthropic.com/v1/messages` with JSON body. This avoids adding a dependency but requires manual streaming response parsing.

**Recommendation:** Use `github.com/anthropics/anthropic-sdk-go` for type safety and built-in streaming support. The `domain_checker.go` pattern (`&http.Client{Timeout: N}`) is the model for any custom HTTP client if the raw approach is chosen.

**Configuration:** The API key should be read from `ANTHROPIC_API_KEY` environment variable. The existing `config/config.go` pattern (struct fields + JSON unmarshalling + env var override) is where to add `AnthropicAPIKey string` alongside the existing config fields.

## 2. External HTTP Call Patterns in the Codebase

No existing AI/LLM integrations exist. External HTTP call patterns found:

- **`DomainAgeChecker`** (`server/services/domain_checker.go`) — canonical `http.Client` with 3s timeout, JSON decoding into structs, context-aware via `ctx` parameter threading. This is the best reference for new external API calls.
- **GitHub PR operations** (`server/services/github_service.go`) — shells out to `gh` CLI rather than direct HTTP; not a useful pattern for Anthropic.
- No existing streaming HTTP response consumers (SSE or chunked transfer) in the services layer.

**For a blocking (non-streaming) `GenerateSuggestedRule` RPC** (single-shot V1 per FR-1), the handler can simply call the Anthropic API synchronously. The 5–30s latency is within ConnectRPC's default server handler timeout; the client must set a request timeout via `AbortController` or `connect.Request` deadline. The handler signature follows the standard unary pattern used throughout `session_service.go`:

```go
func (s *SomeService) GenerateSuggestedRule(
    ctx context.Context,
    req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
    // call Anthropic API with ctx for cancellation
}
```

**For a server-streaming variant** (progress updates during the 5–30s call), the pattern is `*connect.ServerStream[T]` + `stream.Send()` loop (see `WatchSessions` at line 1234 of `session_service.go`).

## 3. ConnectRPC Streaming Patterns for Long-Running Operations

**Server-streaming RPC** is the established pattern for long-running operations. From `session.proto`:

```protobuf
rpc WatchSessions(WatchSessionsRequest) returns (stream SessionEvent) {}
rpc WatchReviewQueue(WatchReviewQueueRequest) returns (stream ReviewQueueEvent) {}
rpc StreamTerminal(stream TerminalData) returns (stream TerminalData) {}  // bidi
```

The Go handler signature for server-streaming:

```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
    // stream.Send(event) in a loop; return nil on clean close
}
```

**For `GenerateSuggestedRule`**, two viable approaches:

1. **Unary RPC** — simplest; the frontend shows a spinner for the full 5–30s. The requirements (FR-1, NFR latency) call for a "meaningful loading state" but do not require incremental streaming of partial results. V1 can use unary.
2. **Server-streaming RPC** — enables progress messages ("Analyzing rules...", "Calling AI...", "Validating regex...") before the final `SuggestedRuleProto` payload. Add a `oneof payload { ProgressEvent progress = 1; SuggestedRuleProto result = 2; }` discriminated response message. This adds complexity but satisfies the "meaningful loading state" requirement more richly.

**Cancellation:** Both approaches inherit `ctx.Done()` cancellation. The frontend passes an `AbortController.signal` to the ConnectRPC call; when the user cancels, the server context is cancelled and the Anthropic API call (if passed `ctx`) terminates cleanly.

## 4. Frontend Streaming/Loading Patterns

**For a unary RPC with long latency**, the existing `useApprovalRules.ts` hook pattern is the template:

```typescript
const [loading, setLoading] = useState(false);
const [error, setError] = useState<Error | null>(null);

const generate = useCallback(async () => {
  setLoading(true);
  setError(null);
  try {
    const resp = await clientRef.current.generateSuggestedRule(req, {
      signal: abortControllerRef.current?.signal,
    });
    setSuggestion(resp);
  } catch (err) {
    if (err.name !== "AbortError") setError(err);
  } finally {
    setLoading(false);
  }
}, []);
```

**For a server-streaming RPC** (if server-streaming variant is chosen), the `for await` loop pattern used in `useSessionService.ts` lines 593–596 and `useReviewQueue.ts` line 283 is the standard:

```typescript
const stream = clientRef.current.generateSuggestedRule(req, {
  signal: abortControllerRef.current?.signal,
});

for await (const event of stream) {
  if (event.payload.case === "progress") setProgress(event.payload.value.message);
  else if (event.payload.case === "result") setSuggestion(event.payload.value);
}
```

**Cancellation UI:** `AbortController` is used everywhere (see `useSessionService.ts` line 579). Wire a "Cancel" button to `abortControllerRef.current?.abort()`.

**Transport:** Unary RPCs use the HTTP transport (`getConnectTransport()` from `@/lib/api/transport`). Server-streaming RPCs use the WebSocket-based watch transport (`createWatchTransport` from `@/lib/transport/watch-ws-transport`). A new `useGenerateRule` hook should use `getConnectTransport()` for unary or `createWatchTransport()` for streaming.

## Summary of Decisions Needed

| Question | Options | Notes |
|---|---|---|
| Go Anthropic client | Official SDK vs raw `net/http` | SDK preferred; add `github.com/anthropics/anthropic-sdk-go` |
| RPC shape | Unary vs server-streaming | Unary is simpler for V1; streaming enables richer progress UX |
| API key config | Env var | Follow existing `config.go` pattern; add `AnthropicAPIKey` field |
| Frontend hook | `useState` + `async/await` | Follow `useApprovalRules.ts`; add `AbortController` for cancel |
