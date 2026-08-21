# Claude-Mux Hooks Injection and External Session Review Queue

**Epic ID**: CMUX-001
**Created**: 2025-12-15
**Status**: Planning

## Executive Summary

This feature plan addresses two interrelated issues:

1. **Claude-Mux Hooks Injection**: Enable claude-mux to inject Claude Code hooks configuration that triggers notifications with session context, enabling deep linking back to the source terminal/IDE.

2. **Review Queue for External Sessions**: Fix the review queue functionality for external sessions (created via claude-mux) which currently fails to detect approval requests and status changes properly.

Both features share a common root cause: external sessions lack the ClaudeController infrastructure that managed sessions use for status detection and review queue integration.

---

## Problem Analysis

### Current Architecture

```
MANAGED SESSIONS (stapler-squad created):
  Instance -> TmuxSession -> ClaudeController -> StatusDetector -> ReviewQueuePoller
                                    |
                                    v
                            IdleDetector -> Status Detection -> Review Queue

EXTERNAL SESSIONS (claude-mux created):
  Instance -> TmuxSession (attached) -> No ClaudeController
                    |
                    v
            ExternalApprovalMonitor -> Polling-based Detection (limited)
            ExternalTmuxStreamer -> Terminal Output Only
```

### Issue 1: Hooks Integration Gap

The existing hooks system (`scripts/ssq-hook-handler`) works for Claude sessions but has limitations:

1. **Session ID Detection**: The hook handler tries to detect session ID from:
   - `CS_SESSION_ID` environment variable
   - tmux session name
   - Project directory name
   
2. **Missing Mux Integration**: When Claude runs via claude-mux, hooks need to:
   - Know the mux socket path for correlation
   - Include mux-specific metadata for deep linking
   - Route notifications through the correct session

3. **No Automatic Injection**: Users must manually configure hooks in `~/.claude/settings.json`

### Issue 2: Review Queue Detection Gap

External sessions rely on `ReviewQueuePoller.checkSession()` which has different code paths:

```go
// For sessions WITH controller (managed):
if statusInfo.IsControllerActive {
    controller, exists := rqp.statusManager.GetController(inst.Title)
    // Uses ClaudeController's idle detection and status monitoring
    idleState, lastActivity := controller.GetIdleState()
    // Full status detection from controller
}

// For sessions WITHOUT controller (external):
else {
    // Falls back to terminal content detection
    content, err := inst.Preview()
    detectedStatus, statusContext := rqp.statusDetector.DetectWithContext([]byte(content))
    // Limited detection - only catches terminal content patterns
    // PROBLEM: Relies on Preview() which may have stale content
    // PROBLEM: No idle state tracking
    // PROBLEM: No integration with ExternalApprovalMonitor
}
```

**Root Cause**: External sessions don't have ClaudeController, so:
- No `IdleDetector` for tracking activity states
- No continuous output monitoring
- `Preview()` returns tmux capture-pane which may miss transient prompts
- `ExternalApprovalMonitor` detects approvals but doesn't integrate with `ReviewQueue`

---

## Solution Architecture

### ADR-001: Hooks Injection via Environment Variable

**Context**: Claude Code hooks need to be automatically configured when running via claude-mux without requiring manual settings.json modification.

**Decision**: Inject hooks configuration via `CLAUDE_CODE_HOOKS_PATH` environment variable pointing to a dynamically generated hooks file.

**Consequences**:
- (+) No modification to user's settings.json
- (+) Hooks are session-specific with correct context
- (+) Can include mux socket path for correlation
- (-) Requires claude-mux to generate hooks file on startup
- (-) Needs cleanup on exit

**Implementation**:
```
claude-mux starts:
  1. Generate temporary hooks file with mux context
  2. Set CLAUDE_CODE_HOOKS_PATH environment variable
  3. Start claude with hooks enabled
  4. On exit, cleanup hooks file
```

### ADR-002: Unified Status Detection for External Sessions

**Context**: External sessions need the same status detection quality as managed sessions.

**Decision**: Create a lightweight `ExternalSessionController` that provides ClaudeController-like functionality without full lifecycle management.

**Consequences**:
- (+) Consistent review queue behavior for all session types
- (+) Reuses existing StatusDetector patterns
- (+) Can integrate with ExternalApprovalMonitor
- (-) Additional complexity in session management
- (-) Need to handle controller lifecycle for discovered sessions

**Implementation**:
```
ExternalSessionController:
  - Wraps tmux session (already attached in handleNewSession)
  - Runs continuous terminal monitoring
  - Maintains idle state tracking
  - Integrates with StatusDetector
  - Reports to InstanceStatusManager
```

### ADR-003: Hook-to-Notification Bridge for Deep Linking

**Context**: Notifications from hooks need to enable navigation back to the source terminal.

**Decision**: Extend hook handler to include comprehensive source context and implement focus window API for IDE deep linking.

**Consequences**:
- (+) Notifications include all context needed for deep linking
- (+) Web UI can focus source IDE/terminal
- (+) TUI can display source information
- (-) Platform-specific focus implementation
- (-) IDE detection is heuristic-based

---

## Epic: External Session Integration Enhancement

### User Value

1. **Developers using IDEs**: Get notifications when Claude needs approval in IntelliJ/VSCode, with one-click return to IDE
2. **Multi-terminal users**: Review queue shows all sessions needing attention regardless of where they started
3. **Workflow continuity**: Start Claude anywhere, manage it from stapler-squad

### Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| External session review queue detection rate | 95% | Automated test with approval prompts |
| Hook notification delivery latency | <1s | Time from Claude prompt to notification |
| Deep link success rate | 80% | Users successfully returning to source IDE |
| False positive review queue items | <5% | Items incorrectly flagged as needing attention |

---

## Story Breakdown

### Story 1: Hooks File Generation in claude-mux

**As a** developer running Claude via claude-mux
**I want** hooks to be automatically configured
**So that** I receive notifications without manual setup

**Acceptance Criteria**:
- [ ] claude-mux generates hooks configuration file on startup
- [ ] Hooks file includes session-specific metadata (mux socket, PID, cwd)
- [ ] `CLAUDE_CODE_HOOKS_PATH` environment variable is set before spawning Claude
- [ ] Hooks file is cleaned up on mux exit
- [ ] Existing user hooks in settings.json are not overwritten

**Technical Notes**:
- Location: `/tmp/claude-mux-hooks-{pid}.json`
- Must merge with existing hooks if present
- Include `CS_MUX_SOCKET_PATH` in hook environment

### Story 2: Enhanced Hook Context for Deep Linking

**As a** user viewing notifications in the web UI
**I want** to click "Open in IDE" and return to my terminal
**So that** I can respond to Claude's prompts quickly

**Acceptance Criteria**:
- [ ] Hook handler includes IDE detection metadata
- [ ] Notification includes bundle ID for macOS focus
- [ ] Web UI shows "Open in IDE" button for external sessions
- [ ] Clicking button focuses the source application
- [ ] Works for IntelliJ, VSCode, Terminal, iTerm

**Technical Notes**:
- Extend `ssq-hook-handler` source detection
- Add `FocusWindow` API endpoint (already exists, needs IDE support)
- Test with common IDE environments

### Story 3: ExternalSessionController for Status Detection

**As a** developer with external Claude sessions
**I want** them to appear in the review queue when needing attention
**So that** I don't miss approval prompts

**Acceptance Criteria**:
- [ ] External sessions have continuous terminal monitoring
- [ ] Approval prompts are detected within 5 seconds
- [ ] Input required prompts are detected
- [ ] Error states are detected
- [ ] Idle timeout is tracked correctly
- [ ] Sessions appear in review queue with correct reason

**Technical Notes**:
- Create `ExternalSessionController` struct
- Integrate with `ExternalSessionDiscovery.handleNewSession()`
- Register with `InstanceStatusManager`
- Reuse `StatusDetector` patterns

### Story 4: Review Queue Poller Integration with External Sessions

**As a** developer monitoring multiple sessions
**I want** external sessions to be processed by the review queue
**So that** they have feature parity with managed sessions

**Acceptance Criteria**:
- [ ] External sessions are added to ReviewQueuePoller's instance list
- [ ] Poller detects status changes from ExternalSessionController
- [ ] Grace periods and spam prevention work correctly
- [ ] Acknowledging external sessions works properly
- [ ] Session removal cleans up controller

**Technical Notes**:
- Wire `ExternalSessionDiscovery.OnSessionAdded` to `ReviewQueuePoller.AddInstance`
- Wire `OnSessionRemoved` to `RemoveInstance`
- Ensure controller is started before adding to poller

### Story 5: Approval Monitor to Review Queue Bridge

**As a** developer with pending approvals in external sessions
**I want** them to immediately appear as high priority in the review queue
**So that** I can respond quickly

**Acceptance Criteria**:
- [ ] ExternalApprovalMonitor detects approval prompts
- [ ] Detected approvals create/update ReviewQueue items
- [ ] Priority is set to High for approvals
- [ ] Reason is set to "approval_pending"
- [ ] Context includes approval details

**Technical Notes**:
- Add callback from ExternalApprovalMonitor to ReviewQueue
- Bypass normal poller flow for immediate queue updates
- Handle duplicate detection (same approval shouldn't create multiple items)

---

## Atomic Tasks

### Task 1.1: Hooks File Template Generation
**Files**: `session/mux/hooks.go` (new)
**Duration**: 2 hours
**Dependencies**: None

Create hooks configuration generator:
```go
// GenerateHooksFile creates a temporary hooks configuration for claude-mux
func GenerateHooksFile(socketPath string, pid int, cwd string) (string, error)

// CleanupHooksFile removes the generated hooks file
func CleanupHooksFile(path string) error
```

### Task 1.2: claude-mux Hooks Environment Setup
**Files**: `session/mux/multiplexer.go`, `cmd/claude-mux/main.go`
**Duration**: 2 hours
**Dependencies**: Task 1.1

Modify multiplexer to:
- Generate hooks file before starting tmux session
- Set `CLAUDE_CODE_HOOKS_PATH` in tmux environment
- Add cleanup to `Shutdown()` method

### Task 1.3: Hook Handler Mux Context Enhancement
**Files**: `scripts/ssq-hook-handler`
**Duration**: 2 hours
**Dependencies**: None

Extend hook handler to:
- Read mux socket path from environment
- Include mux metadata in notifications
- Add session correlation via socket path

### Task 2.1: ExternalSessionController Struct
**Files**: `session/external_controller.go` (new)
**Duration**: 3 hours
**Dependencies**: None

Create lightweight controller:
```go
type ExternalSessionController struct {
    instance       *Instance
    statusDetector *StatusDetector
    idleState      IdleState
    lastActivity   time.Time
    outputBuffer   *CircularBuffer
    ctx            context.Context
    cancel         context.CancelFunc
}

func NewExternalSessionController(inst *Instance, detector *StatusDetector) *ExternalSessionController
func (c *ExternalSessionController) Start() error
func (c *ExternalSessionController) Stop()
func (c *ExternalSessionController) GetIdleState() (IdleState, time.Time)
func (c *ExternalSessionController) GetStatus() (DetectedStatus, string)
```

### Task 2.2: Terminal Monitoring Loop
**Files**: `session/external_controller.go`
**Duration**: 2 hours
**Dependencies**: Task 2.1

Implement monitoring:
- Poll tmux capture-pane every 500ms
- Compare with previous content for changes
- Update idle state based on content changes
- Run status detection on content

### Task 2.3: Wire Controller to External Discovery
**Files**: `session/external_discovery.go`, `server/server.go`
**Duration**: 2 hours
**Dependencies**: Task 2.1, Task 2.2

In `handleNewSession()`:
- Create ExternalSessionController after tmux attachment
- Register controller with InstanceStatusManager
- Store controller reference in Instance

### Task 3.1: Wire External Sessions to Review Queue Poller
**Files**: `server/server.go`
**Duration**: 1 hour
**Dependencies**: Task 2.3

Add callbacks:
```go
externalDiscovery.OnSessionAdded(func(instance *session.Instance) {
    // Existing storage add...
    reviewQueuePoller.AddInstance(instance)
})

externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
    // Existing storage remove...
    reviewQueuePoller.RemoveInstance(instance.Title)
})
```

### Task 3.2: Review Queue Poller Controller Integration
**Files**: `session/review_queue_poller.go`
**Duration**: 2 hours
**Dependencies**: Task 2.3, Task 3.1

Modify `checkSession()` to:
- Check for ExternalSessionController if no ClaudeController
- Use controller's status detection
- Apply same logic as managed sessions

### Task 3.3: Approval Monitor to Review Queue Bridge
**Files**: `session/external_approval.go`, `server/server.go`
**Duration**: 2 hours
**Dependencies**: Task 3.1

Add review queue integration:
```go
externalApprovalMonitor.OnApproval(func(event *ExternalApprovalEvent) {
    item := &ReviewItem{
        SessionID:  event.SessionTitle,
        Reason:     ReasonApprovalPending,
        Priority:   PriorityHigh,
        Context:    event.Request.DetectedText,
        // ...
    }
    reviewQueue.Add(item)
})
```

### Task 4.1: Integration Tests for External Review Queue
**Files**: `session/external_review_queue_test.go` (new)
**Duration**: 3 hours
**Dependencies**: Tasks 3.1-3.3

Test scenarios:
- External session discovery triggers controller creation
- Status changes propagate to review queue
- Approval detection creates queue items
- Session removal cleans up properly

### Task 4.2: E2E Test for Hook Notifications
**Files**: `tests/e2e/external-session-hooks.spec.ts` (new)
**Duration**: 2 hours
**Dependencies**: Tasks 1.1-1.3

Test scenarios:
- claude-mux starts with hooks enabled
- Approval prompt triggers notification
- Notification includes source context
- Deep link focuses source app (manual verification)

---

## Known Issues and Potential Bugs

### Bug Risk 1: Race Condition in Controller Startup

**Description**: ExternalSessionController may start polling before tmux session is fully attached.

**Mitigation**:
- Add readiness check in controller Start()
- Verify tmux session exists before starting monitor loop
- Add retry with backoff for initial capture

**Files Affected**: `session/external_controller.go`

### Bug Risk 2: Duplicate Approval Notifications

**Description**: Both ExternalApprovalMonitor and ExternalSessionController may detect the same approval, creating duplicate review queue items.

**Mitigation**:
- Use approval fingerprint (hash of detected text) for deduplication
- ExternalApprovalMonitor takes precedence (faster detection)
- Controller detection is fallback for missed approvals

**Files Affected**: `session/external_controller.go`, `session/external_approval.go`

### Bug Risk 3: Hooks File Left Behind on Crash

**Description**: If claude-mux crashes, hooks file may not be cleaned up.

**Mitigation**:
- Use unique filename with PID
- Add cleanup on startup for stale files
- Consider using signal handlers for cleanup

**Files Affected**: `session/mux/hooks.go`, `session/mux/multiplexer.go`

### Bug Risk 4: Review Queue Spam from Idle Detection

**Description**: External sessions may trigger idle timeout repeatedly if content doesn't change but session is actually active.

**Mitigation**:
- Track "meaningful" content changes (not just any difference)
- Use same grace period and spam prevention as managed sessions
- Consider terminal cursor position as activity indicator

**Files Affected**: `session/external_controller.go`, `session/review_queue_poller.go`

### Bug Risk 5: Memory Leak in Output Buffer

**Description**: Circular buffer in ExternalSessionController may grow unbounded if not properly managed.

**Mitigation**:
- Use fixed-size circular buffer (4KB max)
- Clear buffer after status detection
- Ensure cleanup on controller stop

**Files Affected**: `session/external_controller.go`

---

## Dependency Visualization

```
Task 1.1 (Hooks Template) ─────────────┐
                                       │
Task 1.2 (Mux Environment) ◀───────────┤
                                       │
Task 1.3 (Hook Handler) ◀──────────────┘

Task 2.1 (Controller Struct) ─────────────┐
                                          │
Task 2.2 (Monitor Loop) ◀─────────────────┤
                                          │
Task 2.3 (Discovery Wiring) ◀─────────────┘
         │
         │
         ▼
Task 3.1 (Poller Wiring) ────────────────┐
                                         │
Task 3.2 (Poller Integration) ◀──────────┤
                                         │
Task 3.3 (Approval Bridge) ◀─────────────┘
         │
         │
         ▼
Task 4.1 (Integration Tests)
         │
         │
         ▼
Task 4.2 (E2E Tests)
```

---

## Context Preparation Guides

### Before Starting Task 1.x (Hooks)

Read these files:
- `scripts/ssq-hook-handler` - Current hook handling
- `scripts/ssq-hooks-install` - Hook installation
- `session/mux/multiplexer.go` - Mux startup flow
- `docs/claude-hooks-integration.md` - Hook documentation

Key concepts:
- Claude Code hooks configuration format
- Environment variable injection in tmux
- Session ID detection hierarchy

### Before Starting Task 2.x (Controller)

Read these files:
- `session/claude_controller.go` - Reference implementation
- `session/idle_detector.go` - Idle state tracking
- `session/status_detector.go` - Status pattern matching
- `session/external_discovery.go` - External session lifecycle

Key concepts:
- IdleState enum and transitions
- StatusDetector pattern matching
- tmux capture-pane output format

### Before Starting Task 3.x (Review Queue)

Read these files:
- `session/review_queue_poller.go` - Polling logic
- `session/review_queue.go` - Queue data structures
- `session/external_approval.go` - Approval detection
- `server/server.go` - Wiring setup

Key concepts:
- ReviewItem structure and priorities
- AttentionReason types
- Grace periods and spam prevention
- Observer pattern for queue updates

### Before Starting Task 4.x (Testing)

Read these files:
- `session/review_queue_test.go` - Existing tests
- `session/external_discovery_test.go` - If exists
- `tests/e2e/smoke.spec.ts` - E2E test patterns

Key concepts:
- Test isolation requirements
- Mock tmux sessions
- Integration test patterns

---

## Implementation Order Recommendation

**Phase 1 (Foundation)**: Tasks 2.1, 2.2 - Controller infrastructure
**Phase 2 (Integration)**: Tasks 2.3, 3.1, 3.2 - Wire into existing systems
**Phase 3 (Hooks)**: Tasks 1.1, 1.2, 1.3 - Notification enhancement
**Phase 4 (Bridge)**: Task 3.3 - Approval to queue bridge
**Phase 5 (Verification)**: Tasks 4.1, 4.2 - Testing

This order prioritizes fixing the review queue issue first (most impactful), then adding hooks integration (enhancement), and finally comprehensive testing.

---

## Rollback Plan

If issues arise in production:

1. **Disable ExternalSessionController**: Set flag to skip controller creation in `handleNewSession()`
2. **Disable Hooks Injection**: Set `CLAUDE_MUX_DISABLE_HOOKS=1` environment variable
3. **Disable Approval Bridge**: Remove OnApproval callback registration

All changes are additive and can be feature-flagged without affecting core functionality.

---

## References

- [Claude Code Hooks Documentation](https://code.claude.com/docs/en/hooks)
- [ADR-002: External Session Streaming Mode](../architecture/decisions/002-external-session-streaming-mode.md)
- [ADR-003: Session Resolution Strategy](../architecture/decisions/003-session-resolution-strategy.md)
- [Review Queue Feature Plan](./review-queue-feature.md)
- [Notification Chimes Feature](./notification-chimes.md)
