# ADR-004: Headless `ParseIntent` RPC Shared Between UI and MCP Tool

**Status**: Proposed
**Date**: 2026-04-18

## Context

The LLM intent-parsing feature must be reachable from two surfaces:

1. **React UI**: The omnibar sends the user's natural-language description to the backend and receives a structured `SessionIntent` to pre-fill the form.
2. **MCP tool**: An external agent (e.g., Claude itself, an automation script) can call `create_session_from_intent` on the stapler-squad MCP server to trigger intent-based session creation headlessly.

A naive implementation would add separate code paths for each: a raw HTTP handler for the UI and a bespoke MCP tool handler. This creates two maintenance surfaces and risks behavioral divergence — the UI path and the MCP tool path could produce different results for the same description.

The existing codebase already uses ConnectRPC (`connectrpc.com/connect`) for all session operations. The `SessionService` is the single handler for all session RPCs (>30 methods). The MCP server's lifecycle handlers (`server/mcp/tools_lifecycle.go`) already call into `SessionService` for operations like `CreateSession`. This established pattern separates the transport layer (ConnectRPC or MCP) from the domain logic (SessionService).

## Decision

Add a single new ConnectRPC method `ParseIntent` to the `SessionService`:

```protobuf
// proto/session/v1/session.proto
rpc ParseIntent(ParseIntentRequest) returns (ParseIntentResponse) {}

message ParseIntentRequest {
  string description = 1;         // natural-language session description
  bool execute       = 2;         // if true, create the session immediately
}

message ParseIntentResponse {
  SessionIntent intent     = 1;   // structured parameters
  string        session_id = 2;   // non-empty only when execute=true and creation succeeded
  string        error      = 3;   // non-empty on parse failure
}

message SessionIntent {
  string   title               = 1;
  string   path                = 2;
  string   branch              = 3;
  string   program             = 4;
  string   session_type        = 5;
  string   initial_prompt      = 6;
  repeated string tags         = 7;
  string   suggested_session_id = 8;
  double   confidence          = 9;
}
```

The handler in `server/services/session_service.go` calls `s.intentParser.ParseIntent(ctx, req.Msg.Description, starterCtx)`, where `intentParser` is an `intent.IntentParser` injected at construction time (see ADR-001).

The MCP tool `create_session_from_intent` in `server/mcp/tools_intent.go` constructs a `ParseIntentRequest` and calls the same `SessionService.ParseIntent` handler via its injected `svc` reference — identical to how `create_session` calls `svc.CreateSession`. When `execute=true` is set, the handler calls `CreateSession` internally after a successful parse.

The React UI calls the ConnectRPC endpoint at `POST /api/session.v1.SessionService/ParseIntent` via the existing generated client (`gen/proto/go/session/v1/sessionv1connect`).

**`StarterContext` population** (done in the `ParseIntent` handler before calling the backend):
- `RecentPaths`: last 10 unique paths from sessions sorted by `LastActivityAt`
- `Sessions`: title + status of up to 20 most-recent sessions
- `MCPServerURL`: the server's own MCP URL (already stored in `s.mcpServerURL`)

## Consequences

**Positive**:
- Single implementation of intent-parsing logic; UI and MCP tool are thin adapters
- Proto-first API definition generates both the Go handler interface and the TypeScript client bindings via the existing `make generate-proto` workflow
- The `execute: bool` field enables the MCP tool to create sessions in one call (headless, no form review), while the UI always uses `execute: false` and shows the pre-fill form
- Adding new fields to `SessionIntent` (e.g., `auto_yes: bool`) requires only a proto change and a backend update — both transports pick it up automatically

**Negative / accepted costs**:
- Adding a proto method requires running `make generate-proto` and committing generated code; this is an established overhead in the codebase
- The `execute: true` path in the MCP tool bypasses the user review step intentionally — this is the headless contract; callers must accept that the created session may not exactly match their intent
- `StarterContext` population adds a session-store query (~10ms) on every `ParseIntent` call; this is unavoidable and acceptable

**Rejected alternatives**:
- *Raw HTTP handler (no ConnectRPC)*: Rejected. All session operations use ConnectRPC; a raw handler would be an inconsistency and would not generate TypeScript client bindings
- *MCP tool calling the HTTP endpoint via `http.Client`*: Rejected. Self-calling via HTTP from within the same process adds unnecessary latency, error-handling complexity, and couples the MCP tool to the server's address
- *Separate `IntentService`*: Rejected. The feature scope is small enough that adding it to `SessionService` (as the existing 30+ RPCs pattern establishes) avoids introducing another service abstraction prematurely
