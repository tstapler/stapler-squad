# ADR-003: Session Resolution Strategy

## Status
Accepted

## Context
The unified WebSocket handler needs to resolve session IDs to `Instance` objects that may come from different sources:
- Managed sessions stored in JSON files
- External sessions discovered via tmux/mux
- Sessions cached in ReviewQueuePoller

We need a consistent strategy for finding the correct session instance and determining its type.

### Current Behavior
- Managed sessions: Loaded from storage or ReviewQueuePoller
- External sessions: Retrieved from ExternalSessionDiscovery
- No unified lookup mechanism

### Challenges
- Session IDs must be unique across both types
- Performance implications of checking multiple sources
- Consistency of instance state across sources
- Race conditions during discovery

## Decision
Implement a **hierarchical resolution strategy** with the following priority order:
1. ReviewQueuePoller (for managed sessions with fresh state)
2. Storage (for managed sessions)
3. ExternalDiscovery (for external sessions)

## Rationale

### Maintains Fresh State
ReviewQueuePoller has the most up-to-date instance state with active controllers and timestamps, making it the preferred source.

### Backward Compatible
Existing code that relies on storage lookups continues to work without modification.

### Clear Precedence
The priority order ensures managed sessions are always preferred when there's a naming conflict.

### Performance Optimized
Most common case (managed sessions in poller) is checked first, minimizing lookup time.

## Consequences

### Positive
- Unified session lookup across all types
- Consistent behavior for session resolution
- No breaking changes to existing code
- Clear debugging path with ordered checks

### Negative
- Multiple lookups may impact performance
- Session ID uniqueness must be enforced
- External discovery must be running for external sessions to work
- Potential for stale data if sources disagree

### Neutral
- Session type determined implicitly by source
- May need explicit type hints in future

## Implementation

### Resolution Method
```go
func (h *ConnectRPCWebSocketHandler) resolveSession(sessionID string) (*session.Instance, error) {
    // 1. Check ReviewQueuePoller first (managed sessions with fresh state)
    if h.sessionService.reviewQueuePoller != nil {
        if instance := h.sessionService.reviewQueuePoller.FindInstance(sessionID); instance != nil {
            log.InfoLog.Printf("Resolved session '%s' from ReviewQueuePoller (managed)", sessionID)
            return instance, nil
        }
    }
    
    // 2. Check storage (managed sessions)
    instances, err := h.sessionService.storage.LoadInstances()
    if err == nil {
        for _, inst := range instances {
            if inst.Title == sessionID {
                log.InfoLog.Printf("Resolved session '%s' from storage (managed)", sessionID)
                return inst, nil
            }
        }
    }
    
    // 3. Check external discovery (external sessions)
    if h.externalDiscovery != nil {
        // Try by tmux session name first (preferred)
        if instance := h.externalDiscovery.GetSessionByTmux(sessionID); instance != nil {
            log.InfoLog.Printf("Resolved session '%s' from external discovery by tmux name", sessionID)
            return instance, nil
        }
        
        // Fallback to title search
        for _, inst := range h.externalDiscovery.GetSessions() {
            if inst.Title == sessionID {
                log.InfoLog.Printf("Resolved session '%s' from external discovery by title", sessionID)
                return inst, nil
            }
        }
    }
    
    return nil, fmt.Errorf("session not found: %s", sessionID)
}
```

### Session Type Detection
```go
func (h *ConnectRPCWebSocketHandler) streamTerminal(stream *connectWebSocketStream) error {
    instance, err := h.resolveSession(sessionID)
    if err != nil {
        return err
    }
    
    // Route based on instance type
    if instance.InstanceType == session.InstanceTypeExternal {
        return h.streamExternalTerminal(stream, instance)
    }
    return h.streamManagedTerminal(stream, instance)
}
```

### Uniqueness Enforcement
- Managed sessions: Enforced at creation time
- External sessions: Use tmux session name as unique ID
- Validation: Check both sources before creating new sessions

## Monitoring

### Metrics
- Resolution source distribution (poller vs storage vs external)
- Resolution time by source
- Resolution failures
- Type mismatches

### Logging
- Log resolution source for debugging
- Warn on slow lookups (>100ms)
- Error on conflicts

## Future Considerations

### Session Registry
Consider implementing a unified session registry that indexes all sessions regardless of type.

### Type Hints
Allow clients to provide session type hints to skip unnecessary lookups.

### Caching Layer
Add an LRU cache for frequently accessed sessions.

### Async Discovery
Make external discovery async with promises for better performance.

## References
- [Instance Type Definition](https://github.com/claude-squad/session/types.go)
- [ReviewQueuePoller](https://github.com/claude-squad/session/review_queue_poller.go)
- [ExternalSessionDiscovery](https://github.com/claude-squad/session/external_discovery.go)
