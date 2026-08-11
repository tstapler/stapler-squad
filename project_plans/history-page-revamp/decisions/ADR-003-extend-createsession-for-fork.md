# ADR-003: Extend CreateSession for History Fork (No New RPC)

**Status**: Accepted
**Date**: 2026-04-12
**Project**: history-page-revamp

## Context

The fork/resume workflow requires creating a new session that:
1. Copies a Claude conversation JSONL file to a new path (the "fork")
2. Optionally truncates the conversation at a specific message index
3. Then starts the new session via `claude --resume <new-id>`

Two API shapes were considered:
- **Option A**: Add a dedicated `ForkHistorySession` RPC
- **Option B**: Extend the existing `CreateSession` request with two new optional fields

## Decision

Extend `CreateSession` with `fork_source_id` and `fork_at_message` fields. Do not add a new RPC.

## Rationale

The fork action in the UI always immediately starts a session. A dedicated `ForkHistorySession` RPC that only creates the forked JSONL without starting a session has no current use case. Merging the operations saves a round trip and keeps the client logic in one place.

`ForkClaudeConversation` already exists in `session/history_fork.go` with the correct signature. The only change needed is dispatch logic inside the existing `CreateSession` handler.

```protobuf
message CreateSessionRequest {
  // ... existing fields 1–11 unchanged ...

  // Optional: ID of a ClaudeHistoryEntry to fork from.
  // When set, the backend calls ForkClaudeConversation(src, fork_at_message, dst)
  // and sets the resulting UUID as the effective resume_id.
  // Takes precedence over resume_id if both are set.
  string fork_source_id = 12;

  // Optional: Truncate the forked conversation at this message index (1-based).
  // 0 or absent = copy all messages.
  uint64 fork_at_message = 13;
}
```

Handler logic (in `server/services/session_service.go`):
```go
if req.Msg.ForkSourceId != "" {
    newUUID, err := session.ForkClaudeConversation(srcPath, forkAtMessage, dstDir)
    if err != nil { return nil, err }
    req.Msg.ResumeId = newUUID
}
// continue normal CreateSession flow
```

## Consequences

**Positive:**
- Single round trip: fork + start session in one call
- No new proto service definition or generated client code beyond two fields
- `ForkClaudeConversation` is already unit-tested

**Negative:**
- `CreateSession` grows in responsibility (fork dispatch + session start)
- A future "fork-only without starting" use case would need either a separate RPC or a new `start: bool` flag

## Future Consideration

If a "fork preview" or "fork without starting" use case emerges, add a thin `ForkHistorySession(ForkHistorySessionRequest) returns (ForkHistorySessionResponse)` wrapper that delegates to the same `session.ForkClaudeConversation` function without the session-start step. This can be done without touching the existing `CreateSession` path.

## Patterns Applied

- **Command aggregation** — combine fork + start into one transactional command
- **Extension point** — optional fields allow existing callers to ignore new fields safely

## Related

- `proto/session/v1/session.proto` — CreateSessionRequest
- `server/services/session_service.go` — CreateSession handler
- `session/history_fork.go` — ForkClaudeConversation (already exists)
- research/architecture.md §3 — fork API design discussion
