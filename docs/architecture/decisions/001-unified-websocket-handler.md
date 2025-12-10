# ADR-001: Unified WebSocket Handler Approach

## Status
Accepted

## Context
Claude Squad currently maintains two separate WebSocket implementations:
1. **Managed sessions**: ConnectRPC protocol with advanced features (compression, flow control, state sync)
2. **External sessions**: Simple WebSocket with raw text protocol and no advanced features

This duplication creates maintenance burden, feature disparity, and increased complexity. We need to decide how to unify these implementations.

### Options Considered

#### Option A: Create New Unified Handler
- Build a new handler from scratch that supports both session types
- Clean slate design without legacy constraints
- Risk of introducing new bugs
- Longer implementation time

#### Option B: Adapt Existing ConnectRPC Handler
- Extend the existing `ConnectRPCWebSocketHandler` to support both types
- Preserve tested code for managed sessions
- Add conditional logic for external sessions
- Minimal disruption to working functionality

#### Option C: Maintain Separate Handlers with Shared Core
- Extract common logic into shared components
- Keep separate handlers as thin wrappers
- More complex architecture
- Doesn't fully solve the duplication problem

## Decision
We will **adapt the existing ConnectRPC handler** (Option B) to support both managed and external sessions.

## Rationale

### Preserves Working Code
The ConnectRPC handler is battle-tested and handles managed sessions reliably. Starting from this foundation reduces risk.

### Incremental Migration
We can add external session support gradually without breaking existing functionality. The handler can detect session type and route appropriately.

### Protocol Unification
External sessions will benefit from the ConnectRPC protocol features like compression and proper message framing.

### Code Reuse
Maximum reuse of existing protocol handling, error management, and connection lifecycle code.

## Consequences

### Positive
- Minimal risk to existing managed session functionality
- External sessions gain advanced features (compression, flow control)
- Single point of maintenance for WebSocket logic
- Consistent protocol across all session types
- Frontend can use a single hook for all sessions

### Negative
- Handler becomes more complex with conditional logic
- Need careful testing of both paths
- Some external session limitations remain (polling-based updates)
- Slightly increased coupling between session types

### Neutral
- Performance characteristics differ between session types (acceptable)
- Different update patterns (incremental vs snapshot) handled by same code

## Implementation Notes

### Session Type Detection
```go
// In streamTerminal method
instance := s.resolveSession(sessionID)
if instance.InstanceType == session.InstanceTypeExternal {
    return s.streamExternalTerminal(stream, instance)
}
return s.streamManagedTerminal(stream, instance)
```

### Protocol Adaptation
External sessions will use "state" streaming mode to handle snapshot-based updates from `tmux capture-pane`.

### Backward Compatibility
The old `/api/ws/external` endpoint will be maintained temporarily with deprecation warnings.

## References
- [Feature Plan: Unified WebSocket Streaming](../../../tasks/unified-websocket-streaming.md)
- [ConnectRPC Documentation](https://connect.build/)
- [Original External Session Implementation](https://github.com/claude-squad/session/external_websocket.go)
