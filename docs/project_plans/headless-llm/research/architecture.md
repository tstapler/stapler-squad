# Architecture Research: Package Structure & Wiring

## 1. session/namegen — Canonical Sub-Package Pattern

**Location**: `session/namegen/namegen.go`

`namegen` is a standalone sub-package under `session/` with:
- Single responsibility (generate unique directory names)
- No import of parent `session` package — avoids circular deps
- Exported pure functions (`Generate()`, `GenerateAndCreate()`, `GenerateUnique()`)
- Testable with a custom generator function (`GenerateAndCreateWithFn`)
- Zero dependencies on server, storage, or config

The `session/headless` package should follow this exact model:
- `session/headless/pool.go` — Pool type with FeatureKey → active session mapping
- `session/headless/caller.go` — Call() / CallBlocking() wrapping subprocess
- `session/headless/runner.go` — ClaudeRunner interface + real implementation
- `session/headless/fake_runner.go` (or `_test.go`) — FakeRunner for tests
- No import of `session` package internals; depends only on `executor/safeexec`

## 2. server/dependencies.go — Service Wiring Pattern

**Location**: `server/dependencies.go`

`ServerDependencies` is a flat struct holding all wired components. `BuildDependencies()` constructs them in the correct order. Services are wired during startup (not at request time).

The pattern for adding a new service:
1. Add field to `ServerDependencies` struct (e.g., `HeadlessPool *headless.Pool`)
2. Construct in `BuildDependencies()` — inject dependencies (config, executor path)
3. Wire into existing services that need it (e.g., pass to `BacklogService` constructor)
4. Pass `deps.HeadlessPool` to the new `HeadlessService` constructor

For replacing `ReviewGateSpawner`:
- Create a `HeadlessReviewSpawner` adapter struct (in `server/services/` or `session/headless/`) that implements `session.ReviewGateSpawner`
- Wraps a `*headless.Pool`, converts `SpawnReviewSession(ctx, item, itemSessionID, prompt)` into `pool.CallBlocking(ctx, "review", systemPrompt, buildPrompt(item, diff, ac))`
- Pass this adapter to `NewBacklogLifecycleListenerWithSpawner`

## 3. server/server.go — RPC Registration Pattern

**Location**: `server/server.go`

For a new `HeadlessService`:

```go
// Register HeadlessService handler.
if deps.HeadlessPool != nil {
    hlSvc := services.NewHeadlessService(deps.HeadlessPool)
    hlPath, hlHandler := sessionv1connect.NewHeadlessServiceHandler(hlSvc, ConnectOptions(deps.ErrorRegistry)...)
    hlAPIPath := "/api" + hlPath
    srv.RegisterConnectHandler(hlAPIPath, http.StripPrefix("/api", hlHandler))
    log.Info("Registered HeadlessService handler", "path", hlAPIPath)
}
```

For `RunHeadlessCall` server-streaming, also register a `StreamingWSBridge` entry so browsers can receive SSE:
```go
runHeadlessCallPath := "/api" + sessionv1connect.HeadlessServiceRunHeadlessCallProcedure
srv.mux.Handle(runHeadlessCallPath, wsBridge.Handler("/api"))
```

Pattern precedents:
- `InsightsService` — registered identically, has a server-streaming RPC (`WatchInsights`)
- `UnfinishedWorkService` — registered with nil guard
- `BacklogService` — registered with nil guard

## 4. ConnectRPC Server-Streaming Implementation

The canonical implementation (from `WatchInsights` in `server/services/insights_service.go`):

```go
func (s *InsightsService) WatchInsights(
    ctx context.Context,
    _ *connect.Request[sessionv1.WatchInsightsRequest],
    stream *connect.ServerStream[sessionv1.InsightsEvent],
) error {
    // 1. Send initial state
    if err := stream.Send(initialEvent); err != nil {
        return fmt.Errorf("send initial event: %w", err)
    }
    // 2. Subscribe to change channel
    ch := s.store.Subscribe()
    defer s.store.Unsubscribe(ch)
    // 3. Forward until ctx cancelled
    for {
        select {
        case <-ctx.Done():
            return nil
        case _, ok := <-ch:
            if !ok { return nil }
            if err := stream.Send(evt); err != nil {
                return fmt.Errorf("send update event: %w", err)
            }
        }
    }
}
```

For `RunHeadlessCall`:
```go
func (s *HeadlessService) RunHeadlessCall(
    ctx context.Context,
    req *connect.Request[sessionv1.RunHeadlessCallRequest],
    stream *connect.ServerStream[sessionv1.RunHeadlessCallResponse],
) error {
    ch, err := s.pool.Call(ctx, req.Msg.FeatureKey, req.Msg.SystemPrompt, req.Msg.UserPrompt)
    if err != nil {
        return connect.NewError(connect.CodeInternal, err)
    }
    for {
        select {
        case <-ctx.Done():
            return nil
        case chunk, ok := <-ch:
            if !ok { return nil }
            if err := stream.Send(&sessionv1.RunHeadlessCallResponse{Text: chunk.Text, Done: chunk.Done}); err != nil {
                return err
            }
        }
    }
}
```

## 5. Interface-Based Testability Patterns

**`session.ReviewGateSpawner`** (in `session/backlog_lifecycle.go`):
```go
type ReviewGateSpawner interface {
    SpawnReviewSession(ctx context.Context, item *ent.BacklogItem, itemSessionID string, prompt string) (*Instance, error)
}
```
Injected via `NewBacklogLifecycleListenerWithSpawner`. This allows tests to use a fake spawner without spawning real tmux sessions. The headless `Pool` should also implement this interface (or an adapter wrapper should).

**`session.LifecycleListener`** (in `session/instance.go`):
```go
type LifecycleListener interface {
    OnLifecycleEvent(event LifecycleEvent, detail string)
}
```
Multiple listener implementations (e.g., `instanceBacklogListener`) registered on `Instance`. The `WireToInstance` pattern (creating a per-instance shim) is used to avoid shared mutable state.

**`AIClient`** (in `server/services/ai_interfaces.go`):
```go
type AIClient interface {
    Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
```
Used by `RulesService`. `CLIAIClient` implements it. The headless `Pool` should also implement this interface for drop-in replacement in `RulesService`.

**`ClaudeRunner` (proposed)**:
The requirements call for:
```go
type ClaudeRunner interface {
    Run(ctx context.Context, args []string) (stdout io.Reader, err error)
}
```
Real implementation uses `ManagedProcess` or `ShortLivedCmd`. `FakeRunner` returns a predefined `io.Reader` for deterministic tests without spawning `claude`.

## 6. Compile-Time Interface Compliance Checks

A convention used in this codebase (e.g., `insights_service.go`):
```go
var _ sessionv1connect.InsightsServiceHandler = (*InsightsService)(nil)
```

Add analogous checks to `headless/pool.go`:
```go
var _ AIClient = (*Pool)(nil)  // Pool implements AIClient
```
And to the adapter:
```go
var _ session.ReviewGateSpawner = (*HeadlessReviewSpawner)(nil)
```

## 7. Proto Service Pattern

New service proto should live in `proto/session/v1/headless.proto` or be added to `session.proto`:

```protobuf
service HeadlessService {
    rpc RunHeadlessCall(RunHeadlessCallRequest) returns (stream RunHeadlessCallResponse) {}
}

message RunHeadlessCallRequest {
    string feature_key = 1;
    string system_prompt = 2;
    string user_prompt = 3;
}

message RunHeadlessCallResponse {
    string text = 1;
    bool done = 2;
    string error = 3;
}
```

After adding: run `make generate-proto` to regenerate Go and TypeScript bindings. The generated connect handler goes to `gen/proto/go/session/v1/sessionv1connect/`.
