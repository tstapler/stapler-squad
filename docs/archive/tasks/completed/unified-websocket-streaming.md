# Feature Plan: Unified WebSocket Streaming Architecture

## Executive Summary

This document outlines the comprehensive plan for unifying external session WebSocket streaming with the managed session streaming infrastructure. The goal is to eliminate the dual-track WebSocket implementation by adapting the ConnectRPC WebSocket handler to support both managed and external sessions, providing a single consistent streaming protocol with superior features.

**Current State**: Two separate WebSocket implementations with different capabilities
**Target State**: Single unified ConnectRPC WebSocket handler supporting both session types
**Benefits**: Code reuse, consistent features, improved maintainability, better performance

## Problem Statement

### Current Architecture Limitations

The codebase currently maintains two distinct WebSocket implementations:

1. **Managed Sessions** (`/api/session.v1.SessionService/StreamTerminal`)
   - Uses ConnectRPC protocol with protobuf envelopes
   - Direct PTY access via `ResponseStream`
   - Advanced features: flow control, compression, state sync
   - Real-time incremental byte streaming

2. **External Sessions** (`/api/ws/external`)
   - Simple WebSocket with raw text protocol
   - Polling-based `tmux capture-pane` via `ExternalTmuxStreamer`
   - No advanced features
   - Full snapshot replacement (~150ms intervals)

This dual implementation creates:
- **Maintenance burden**: Duplicate code paths for similar functionality
- **Feature disparity**: External sessions lack compression, flow control, state sync
- **Performance inconsistency**: External sessions use inefficient full snapshots
- **Frontend complexity**: Two separate React hooks with different capabilities

### Technical Debt

- External sessions send full terminal snapshots with `\x1b[2J\x1b[H` (clear screen) prefix
- No incremental updates for external sessions
- Polling interval hardcoded at 150ms
- No compression for high-bandwidth scenarios
- Two different input handling mechanisms (`PTY.Write` vs `tmux send-keys`)

## Proposed Solution

### Architecture Overview

Adapt the existing `ConnectRPCWebSocketHandler` to intelligently route between PTY-based streaming (managed) and tmux-based streaming (external) based on the session's `InstanceType`.

```
Client → useTerminalStream → ConnectRPC WebSocket → Handler
                                                        ↓
                                              Check InstanceType
                                                   ↙        ↘
                                          Managed         External
                                             ↓                ↓
                                      ResponseStream   ExternalTmuxStreamer
                                             ↓                ↓
                                         PTY Read      tmux capture-pane
```

### Key Design Decisions

#### ADR-001: Unified Handler Approach

**Status**: Accepted

**Context**: We need to choose between creating a new unified handler or adapting the existing one.

**Decision**: Adapt the existing `ConnectRPCWebSocketHandler` to support both session types.

**Rationale**:
- Preserves existing tested code for managed sessions
- Minimal changes to working functionality
- Gradual migration path
- Reuses protocol infrastructure

**Consequences**:
- Need conditional logic for session type detection
- Must ensure backward compatibility
- Testing complexity increases slightly

#### ADR-002: Streaming Mode for External Sessions

**Status**: Accepted

**Context**: External sessions use `tmux capture-pane` which provides full snapshots, not incremental bytes.

**Decision**: Use "state" streaming mode for external sessions with snapshot-based updates.

**Rationale**:
- State mode already handles complete terminal states
- Natural fit for `tmux capture-pane` output
- Enables compression and efficient updates
- Client-side `StateApplicator` can handle both modes

**Consequences**:
- External sessions get compression benefits
- Consistent protocol envelope format
- May need to optimize state generation for snapshots

#### ADR-003: Session Resolution Strategy

**Status**: Accepted

**Context**: The handler needs to determine if a session is managed or external.

**Decision**: Use a unified resolution strategy checking multiple sources in order:
1. ReviewQueuePoller (for managed sessions with fresh state)
2. Storage (for managed sessions)
3. ExternalDiscovery (for external sessions)

**Rationale**:
- Maintains existing priority for managed sessions
- Seamless fallback to external sessions
- No breaking changes to session lookup

**Consequences**:
- Need to inject ExternalDiscovery into handler
- Session ID must be unique across both types
- May need session type hint in initial request

## Requirements

### Functional Requirements

#### FR-1: Unified WebSocket Endpoint
- **Description**: Single `/api/session.v1.SessionService/StreamTerminal` endpoint serves both session types
- **Acceptance Criteria**:
  - Managed sessions continue working without changes
  - External sessions accessible via same endpoint
  - Session type automatically detected

#### FR-2: Protocol Compatibility
- **Description**: External sessions use same ConnectRPC/protobuf protocol
- **Acceptance Criteria**:
  - Same envelope format for both types
  - Consistent message types (TerminalData, etc.)
  - Error handling unified

#### FR-3: Feature Parity
- **Description**: External sessions gain advanced features where applicable
- **Acceptance Criteria**:
  - LZMA compression available for external sessions
  - Flow control signals respected
  - Terminal resize properly handled

#### FR-4: Input Handling
- **Description**: Unified input processing with appropriate routing
- **Acceptance Criteria**:
  - Managed sessions: PTY write
  - External sessions: tmux send-keys
  - Same protobuf input format

#### FR-5: Frontend Unification
- **Description**: Single React hook for all terminal streaming
- **Acceptance Criteria**:
  - `useTerminalStream` handles both types
  - `useExternalTerminalStream` deprecated
  - Automatic session type detection

### Non-Functional Requirements

#### NFR-1: Performance
- **Requirement**: No performance degradation for existing managed sessions
- **Metrics**:
  - Latency: <10ms for managed sessions
  - External polling: Maintain 150ms interval
  - CPU usage: <5% increase

#### NFR-2: Backward Compatibility
- **Requirement**: Existing clients continue working during migration
- **Approach**:
  - Keep `/api/ws/external` endpoint temporarily
  - Deprecation warnings in logs
  - Graceful fallback

#### NFR-3: Reliability
- **Requirement**: No increase in connection failures
- **Metrics**:
  - Connection success rate: >99%
  - Reconnection handling maintained
  - Error recovery unchanged

#### NFR-4: Maintainability
- **Requirement**: Reduced code complexity
- **Measures**:
  - LOC reduction: ~30% in WebSocket handling
  - Single point of protocol updates
  - Unified testing approach

## Known Issues and Mitigation

### 🐛 Issue: Session Type Detection Race Condition
**Severity**: Medium

**Description**: If external discovery hasn't completed, external sessions may not be found immediately.

**Mitigation**:
- Pre-warm external discovery on server start
- Add retry logic with exponential backoff
- Cache discovered sessions
- Add session type hint in connection request

**Prevention**:
- Ensure discovery runs before accepting connections
- Add health check for discovery readiness

### 🐛 Issue: Input Buffering for External Sessions
**Severity**: Low

**Description**: Rapid input to external sessions via `tmux send-keys` may be buffered differently than PTY writes.

**Mitigation**:
- Implement input queue for external sessions
- Rate limit if needed
- Monitor tmux buffer status

**Prevention**:
- Test with rapid input scenarios
- Add input acknowledgment mechanism

### 🐛 Issue: Terminal Size Mismatch
**Severity**: Medium

**Description**: External sessions attached to other terminals may have fixed sizes.

**Mitigation**:
- Best-effort resize for external sessions
- Send size hints but don't expect compliance
- Client-side handling of size mismatches

**Prevention**:
- Document resize limitations
- Add size negotiation protocol

### 🐛 Issue: Memory Leak in Streamer Manager
**Severity**: High

**Description**: ExternalTmuxStreamer instances may not be cleaned up when connections close.

**Mitigation**:
- Implement reference counting
- Add periodic cleanup of idle streamers
- Monitor memory usage

**Prevention**:
- Use context cancellation properly
- Add streamer lifecycle tests
- Implement max streamer limit

## Implementation Tasks

### Phase 1: Backend Unification (2-3 days)

#### Task 1.1: Extend ConnectRPCWebSocketHandler [BACKEND]
**Description**: Add session type detection and routing logic
**Acceptance Criteria**:
- [ ] Add `externalDiscovery` field to handler
- [ ] Implement `resolveSession()` method checking all sources
- [ ] Add session type detection in `streamTerminal()`
- [ ] Route to appropriate streaming method

**Files**:
- `server/services/connectrpc_websocket.go`

#### Task 1.2: Implement External Session Streaming [BACKEND]
**Description**: Add tmux-based streaming path in handler
**Acceptance Criteria**:
- [ ] Create `streamExternalTerminal()` method
- [ ] Integrate `ExternalTmuxStreamer` for output
- [ ] Implement `tmux send-keys` for input
- [ ] Handle resize via `tmux resize-window`

**Files**:
- `server/services/connectrpc_websocket.go`
- `session/external_tmux_streamer.go`

#### Task 1.3: Protocol Adaptation [BACKEND]
**Description**: Convert tmux snapshots to appropriate protocol format
**Acceptance Criteria**:
- [ ] Wrap snapshots in TerminalData protobuf
- [ ] Use "state" mode for external sessions
- [ ] Apply compression when configured
- [ ] Handle clear-screen sequences properly

**Files**:
- `server/services/connectrpc_websocket.go`
- `server/terminal/state_generator.go`

#### Task 1.4: Session Service Integration [BACKEND]
**Description**: Wire external discovery to session service
**Acceptance Criteria**:
- [ ] Inject ExternalDiscovery into handler constructor
- [ ] Update server initialization
- [ ] Ensure session uniqueness across types
- [ ] Add logging for debugging

**Files**:
- `server/services/session_service.go`
- `server/server.go`

### Phase 2: Frontend Migration (1-2 days)

#### Task 2.1: Extend useTerminalStream Hook [FRONTEND]
**Description**: Add external session support detection
**Acceptance Criteria**:
- [ ] Add `sessionType` detection logic
- [ ] Handle both incremental and snapshot updates
- [ ] Maintain backward compatibility
- [ ] Update error handling

**Files**:
- `web-app/src/lib/hooks/useTerminalStream.ts`

#### Task 2.2: Update TerminalOutput Component [FRONTEND]
**Description**: Use unified hook for all sessions
**Acceptance Criteria**:
- [ ] Replace `useExternalTerminalStream` usage
- [ ] Pass session type hints if needed
- [ ] Update props interface
- [ ] Test both session types

**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

#### Task 2.3: Deprecate External Hook [FRONTEND]
**Description**: Mark external hook as deprecated
**Acceptance Criteria**:
- [ ] Add deprecation comments
- [ ] Log migration warnings
- [ ] Update documentation
- [ ] Plan removal timeline

**Files**:
- `web-app/src/lib/hooks/useExternalTerminalStream.ts`

### Phase 3: Testing & Validation (2 days)

#### Task 3.1: Unit Tests [TEST]
**Description**: Test unified handler logic
**Acceptance Criteria**:
- [ ] Test session type detection
- [ ] Test routing logic
- [ ] Test protocol conversion
- [ ] Test error scenarios

**Files**:
- `server/services/connectrpc_websocket_test.go` (new)

#### Task 3.2: Integration Tests [TEST]
**Description**: End-to-end streaming tests
**Acceptance Criteria**:
- [ ] Test managed session streaming
- [ ] Test external session streaming
- [ ] Test session transitions
- [ ] Test reconnection

**Files**:
- `tests/integration/websocket_test.go` (new)

#### Task 3.3: Performance Tests [TEST]
**Description**: Benchmark unified implementation
**Acceptance Criteria**:
- [ ] Compare before/after latency
- [ ] Measure CPU usage
- [ ] Test memory consumption
- [ ] Validate compression ratios

**Files**:
- `server/services/websocket_bench_test.go` (new)

#### Task 3.4: Manual Testing Protocol [TEST]
**Description**: Comprehensive manual testing
**Acceptance Criteria**:
- [ ] Test with IntelliJ external sessions
- [ ] Test with VS Code external sessions
- [ ] Test rapid input scenarios
- [ ] Test network interruptions

### Phase 4: Migration & Cleanup (1 day)

#### Task 4.1: Deprecation Notices [DOCS]
**Description**: Document migration path
**Acceptance Criteria**:
- [ ] Add deprecation to old endpoint
- [ ] Update API documentation
- [ ] Create migration guide
- [ ] Set removal timeline

**Files**:
- `docs/API_MIGRATION.md` (new)
- `README.md`

#### Task 4.2: Monitoring & Metrics [OPS]
**Description**: Add observability
**Acceptance Criteria**:
- [ ] Add metrics for session types
- [ ] Monitor connection success rates
- [ ] Track performance metrics
- [ ] Set up alerts

**Files**:
- `server/metrics/websocket_metrics.go` (new)

#### Task 4.3: Feature Flag (Optional) [OPS]
**Description**: Gradual rollout capability
**Acceptance Criteria**:
- [ ] Add feature flag for unified mode
- [ ] Default to enabled
- [ ] Allow per-session override
- [ ] Remove after validation

**Files**:
- `config/features.go`

## Testing Strategy

### Unit Testing
- Mock session types for handler testing
- Test protocol conversion logic
- Validate error handling paths
- Test session resolution order

### Integration Testing
- Real tmux sessions for external testing
- WebSocket connection lifecycle
- Message flow validation
- Concurrent session handling

### Performance Testing
- Benchmark message throughput
- Measure latency for both types
- Memory usage under load
- CPU utilization comparison

### Manual Testing Checklist
- [ ] Create managed session, verify streaming
- [ ] Create external session (IntelliJ), verify streaming
- [ ] Test input for both types
- [ ] Test terminal resize
- [ ] Test compression (large output)
- [ ] Test reconnection
- [ ] Test session discovery
- [ ] Test error scenarios

## Rollout Plan

### Stage 1: Development (Week 1)
- Implement backend unification
- Update frontend hooks
- Internal testing

### Stage 2: Staging (Week 2)
- Deploy to staging environment
- Team testing
- Performance validation

### Stage 3: Production Canary (Week 3)
- 10% traffic rollout
- Monitor metrics
- Gather feedback

### Stage 4: Full Rollout (Week 4)
- 100% traffic
- Deprecate old endpoint
- Documentation updates

### Stage 5: Cleanup (Week 6)
- Remove deprecated code
- Final optimization
- Close out technical debt

## Success Metrics

### Technical Metrics
- **Code Reduction**: 30% fewer lines in WebSocket handling
- **Test Coverage**: >80% for unified handler
- **Performance**: No regression in latency
- **Reliability**: <0.1% connection failure rate

### Business Metrics
- **Maintenance Time**: 50% reduction in WebSocket-related bugs
- **Feature Velocity**: Faster feature additions
- **User Satisfaction**: No increase in complaints

## Risk Assessment

### High Risks
- **Session detection failure**: Mitigated by fallback logic
- **Performance regression**: Mitigated by benchmarking
- **Breaking changes**: Mitigated by backward compatibility

### Medium Risks
- **Input handling differences**: Mitigated by testing
- **Memory leaks**: Mitigated by lifecycle management
- **Protocol incompatibility**: Mitigated by gradual migration

### Low Risks
- **Documentation gaps**: Mitigated by comprehensive docs
- **Team knowledge**: Mitigated by code reviews
- **Edge cases**: Mitigated by extensive testing

## Dependencies

### Technical Dependencies
- ConnectRPC framework
- Protobuf definitions
- tmux 3.0+
- WebSocket library

### Team Dependencies
- Backend team for implementation
- Frontend team for hook updates
- QA team for testing
- DevOps for deployment

## Documentation Requirements

### Code Documentation
- Inline comments for routing logic
- Protocol format documentation
- Session type detection explanation

### API Documentation
- Updated endpoint specifications
- Migration guide for clients
- Deprecation timeline

### User Documentation
- No changes needed (transparent to users)

## Appendix

### A. Current vs. Unified Architecture Comparison

| Aspect | Current (Dual) | Unified |
|--------|---------------|---------|
| Endpoints | 2 (`/api/session.v1.SessionService/StreamTerminal`, `/api/ws/external`) | 1 (`/api/session.v1.SessionService/StreamTerminal`) |
| Protocols | ConnectRPC + Simple WS | ConnectRPC only |
| Compression | Managed only | Both |
| Flow Control | Managed only | Both |
| Frontend Hooks | 2 | 1 |
| Code Complexity | High | Medium |
| Maintenance | Difficult | Easy |

### B. Message Flow Diagrams

**Managed Session Flow (Unchanged)**:
```
Client → TerminalData{Input} → Handler → PTY.Write → Process
Process → PTY.Read → Handler → TerminalData{Output} → Client
```

**External Session Flow (New)**:
```
Client → TerminalData{Input} → Handler → tmux send-keys → Process
Process → tmux capture-pane → Handler → TerminalData{State} → Client
```

### C. File Structure Changes

```
Before:
server/services/
├── connectrpc_websocket.go (managed only)
├── external_websocket.go (external only)

After:
server/services/
├── connectrpc_websocket.go (unified)
├── external_websocket.go (deprecated)
```

### D. Testing Matrix

| Test Case | Managed | External | Priority |
|-----------|---------|----------|----------|
| Connection | ✓ | ✓ | High |
| Input | ✓ | ✓ | High |
| Output | ✓ | ✓ | High |
| Resize | ✓ | ✓ | Medium |
| Compression | ✓ | ✓ | Medium |
| Flow Control | ✓ | ✓ | Low |
| Reconnection | ✓ | ✓ | High |
| Discovery | N/A | ✓ | High |

---

**Document Version**: 1.0
**Author**: Claude (AI Assistant)
**Date**: December 2024
**Status**: Ready for Review
