# Notification Chimes System - Cross-Process Event Broadcasting

## Epic Overview

**Goal**: Enable Claude Code instances running in isolated tmux sessions to send notification chimes with metadata to the stapler-squad server, which broadcasts these notifications to all connected clients (web UI and TUI) with audio alerts and visual banners.

**Value Proposition**:
- **Real-time Awareness**: Users instantly know when sessions need attention without polling
- **Cross-Process Communication**: Isolated tmux sessions can communicate with the server and UI
- **Multi-Channel Notifications**: Audio chimes + visual banners in web UI and TUI
- **Contextual Information**: Notifications include session name, event type, and relevant metadata
- **Reduced Context Switching**: Users don't need to constantly check sessions manually
- **Extensibility**: Foundation for future inter-session communication features

**Success Metrics**:
- <500ms notification latency from tmux session to UI display
- 100% notification delivery to all connected clients
- Audio chime plays within 200ms of notification receipt
- Visual banner appears within 300ms of notification receipt
- Zero performance impact on tmux session processes
- Support for 10+ notification types (approval needed, error, task complete, etc.)
- Graceful degradation when server is unavailable

**Business Impact**: Improves developer productivity by enabling async work across multiple sessions with immediate feedback, reduces time spent checking session status manually.

---

## Architecture Context

### Current State Analysis

**Existing Infrastructure**:
- ✅ Event bus system (`server/events/bus.go`) with pub/sub pattern
- ✅ Protobuf event definitions (`proto/session/v1/events.proto`)
- ✅ WatchSessions streaming RPC for real-time event broadcasting
- ✅ Web UI notification system (`web-app/src/lib/utils/notifications.ts`)
- ✅ NotificationToast and NotificationPanel components
- ✅ Session instances run in isolated tmux sessions
- ✅ HTTP/gRPC server with ConnectRPC endpoints
- ❌ No mechanism for tmux processes to send events to server
- ❌ No notification chime event types defined
- ❌ No TUI notification banner rendering

**Gaps Identified**:
1. **Communication Channel**: Tmux sessions cannot send data back to server
2. **Event Types**: No notification-specific event types in protobuf schema
3. **TUI Display**: No notification banner rendering in BubbleTea app
4. **Audio Integration**: No audio chime support in TUI (terminal-only)
5. **Process Discovery**: Server needs to identify notification-capable sessions

### System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Stapler Squad Ecosystem                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────────┐         ┌──────────────┐                      │
│  │ Tmux Session │         │ Tmux Session │                      │
│  │  (Claude 1)  │         │  (Claude 2)  │                      │
│  │              │         │              │                      │
│  │  [Agent]     │         │  [Agent]     │                      │
│  │     │        │         │     │        │                      │
│  │     ▼        │         │     ▼        │                      │
│  │  [Notifier]  │         │  [Notifier]  │                      │
│  └──────┬───────┘         └──────┬───────┘                      │
│         │                        │                              │
│         │ HTTP POST              │ HTTP POST                    │
│         │ /api/notify            │ /api/notify                  │
│         │                        │                              │
│         ▼                        ▼                              │
│  ┌─────────────────────────────────────┐                       │
│  │      Stapler Squad Server            │                       │
│  │                                     │                       │
│  │  ┌──────────────────────────────┐  │                       │
│  │  │ NotificationService          │  │                       │
│  │  │ - ReceiveNotification RPC    │  │                       │
│  │  │ - Validate & Authenticate    │  │                       │
│  │  │ - Enrich with metadata       │  │                       │
│  │  └─────────┬────────────────────┘  │                       │
│  │            │                        │                       │
│  │            ▼                        │                       │
│  │  ┌──────────────────────────────┐  │                       │
│  │  │ EventBus                      │  │                       │
│  │  │ - Broadcast notification      │  │                       │
│  │  │ - Pub/Sub pattern             │  │                       │
│  │  └─────────┬────────────────────┘  │                       │
│  └────────────┼────────────────────────┘                       │
│               │                                                 │
│               ├─────────────────┬───────────────────┐          │
│               │                 │                   │          │
│               ▼                 ▼                   ▼          │
│        ┌──────────┐      ┌──────────┐      ┌──────────┐      │
│        │ Web UI 1 │      │ Web UI 2 │      │   TUI    │      │
│        │          │      │          │      │          │      │
│        │ [Toast]  │      │ [Toast]  │      │ [Banner] │      │
│        │ [Panel]  │      │ [Panel]  │      │          │      │
│        │ [Audio]  │      │ [Audio]  │      │          │      │
│        └──────────┘      └──────────┘      └──────────┘      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Decoupled Architecture**: Tmux sessions communicate via HTTP, no tight coupling
2. **Event-Driven**: Use existing event bus for notification broadcasting
3. **Graceful Degradation**: Sessions continue working if server is unavailable
4. **Authentication**: Verify notifications come from legitimate sessions
5. **Extensibility**: Support multiple notification types and custom metadata
6. **Performance**: Non-blocking, async notification delivery
7. **Observability**: Log all notification events for debugging and audit

---

## Architecture Decision Records (ADRs)

### ADR 008: HTTP-Based Notification API for Tmux Sessions

**Status**: Proposed

**Context**:
Claude Code instances run in isolated tmux sessions without direct access to the Go server's internal APIs. We need a communication channel for these processes to send notifications to the server.

**Options Considered**:

1. **HTTP REST API** ✅ (Selected)
   - Pros: Universal, language-agnostic, simple to implement in shell scripts
   - Pros: Works from any process (Python, Node, shell, Go)
   - Pros: Built-in authentication via session tokens
   - Pros: Easy to test with curl/httpie
   - Cons: Requires HTTP client in notifier script
   - Cons: Slightly higher latency than Unix sockets (~5-10ms)

2. **Unix Domain Sockets**
   - Pros: Lowest latency, no network overhead
   - Pros: File-based permissions
   - Cons: Platform-specific (harder on Windows)
   - Cons: Requires socket file management
   - Cons: More complex client implementation
   - Rejected: Additional complexity not justified by latency benefit

3. **Named Pipes (FIFO)**
   - Pros: Simple file-based communication
   - Pros: No HTTP overhead
   - Cons: Platform-specific
   - Cons: Requires polling or inotify on server side
   - Cons: No built-in authentication
   - Rejected: Lack of authentication and cross-platform issues

4. **Direct gRPC/ConnectRPC**
   - Pros: Type-safe, efficient binary protocol
   - Cons: Requires gRPC client libraries (not available in shell)
   - Cons: Complex setup for simple notification use case
   - Rejected: Too heavyweight for tmux session scripts

**Decision**: HTTP REST API endpoint `/api/session.v1.SessionService/SendNotification`

**Rationale**:
- Universally accessible from shell scripts, Python, Node.js, Go
- Simple authentication via session ID in request body or header
- Easy to test and debug with standard HTTP tools
- Consistent with existing ConnectRPC HTTP-based API
- Negligible latency difference (5-10ms) is acceptable for notifications

**Implementation**:
```bash
# Example shell script usage from tmux session
curl -X POST http://localhost:3000/api/session.v1.SessionService/SendNotification \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "my-session",
    "notification_type": "APPROVAL_NEEDED",
    "title": "User approval required",
    "message": "Claude needs your permission to continue",
    "priority": "HIGH",
    "metadata": {"command": "git push origin main"}
  }'
```

**Consequences**:
- ✅ Simple client implementation (curl, httpie, or HTTP libraries)
- ✅ Cross-platform compatibility
- ✅ Easy to extend with new notification types
- ⚠️ Requires HTTP server address configuration (defaults to localhost:3000)
- ⚠️ Network dependency (mitigated by localhost + graceful degradation)

---

### ADR 009: Notification Event Types and Priority Levels

**Status**: Proposed

**Context**:
Different notification types require different UI treatments (audio, visual urgency, auto-dismiss behavior). We need a clear taxonomy of notification types with well-defined semantics.

**Decision**: Structured notification type hierarchy with priority levels

**Notification Type Taxonomy**:

```protobuf
enum NotificationType {
  NOTIFICATION_TYPE_UNSPECIFIED = 0;
  
  // User Action Required (High Priority)
  NOTIFICATION_TYPE_APPROVAL_NEEDED = 1;    // User approval dialog waiting
  NOTIFICATION_TYPE_INPUT_REQUIRED = 2;     // Waiting for user input
  NOTIFICATION_TYPE_CONFIRMATION_NEEDED = 3; // Confirmation prompt waiting
  
  // Status Updates (Medium Priority)
  NOTIFICATION_TYPE_TASK_COMPLETE = 4;      // Task finished successfully
  NOTIFICATION_TYPE_PROCESS_STARTED = 5;    // Long-running process started
  NOTIFICATION_TYPE_PROCESS_FINISHED = 6;   // Long-running process finished
  
  // Errors and Warnings (High/Urgent Priority)
  NOTIFICATION_TYPE_ERROR = 7;              // Error occurred
  NOTIFICATION_TYPE_WARNING = 8;            // Warning condition
  NOTIFICATION_TYPE_FAILURE = 9;            // Operation failed
  
  // Informational (Low Priority)
  NOTIFICATION_TYPE_INFO = 10;              // General information
  NOTIFICATION_TYPE_DEBUG = 11;             // Debug information
  NOTIFICATION_TYPE_STATUS_CHANGE = 12;     // Session status changed
  
  // Custom (Medium Priority)
  NOTIFICATION_TYPE_CUSTOM = 100;           // Custom notification type
}

enum NotificationPriority {
  NOTIFICATION_PRIORITY_UNSPECIFIED = 0;
  NOTIFICATION_PRIORITY_LOW = 1;       // Info, can be dismissed
  NOTIFICATION_PRIORITY_MEDIUM = 2;    // Normal, auto-dismiss after delay
  NOTIFICATION_PRIORITY_HIGH = 3;      // Important, requires acknowledgment
  NOTIFICATION_PRIORITY_URGENT = 4;    // Critical, blocking action required
}
```

**UI Treatment by Priority**:

| Priority | Audio | Web Toast | TUI Banner | Auto-Dismiss |
|----------|-------|-----------|------------|--------------|
| Low      | None  | 3s toast  | 5s banner  | Yes (5s)     |
| Medium   | Ding  | 8s toast  | 10s banner | Yes (10s)    |
| High     | Chime | Persistent| Persistent | No (user)    |
| Urgent   | Alert | Persistent| Persistent | No (user)    |

**Rationale**:
- Clear semantic meaning for each notification type
- Priority determines UI behavior automatically
- Extensible with custom types
- Maps to existing review queue priority system

---

### ADR 010: Session Authentication for Notification Requests

**Status**: Proposed

**Context**:
We need to verify that notification requests come from legitimate stapler-squad sessions, not arbitrary external processes or malicious actors.

**Options Considered**:

1. **Session ID-Based Verification** ✅ (Selected)
   - Pros: Simple, session ID already exists
   - Pros: Server can lookup session to verify it exists
   - Pros: Sufficient for localhost communication
   - Cons: Session ID is not cryptographically secure
   - Mitigation: Only accept connections from localhost

2. **Shared Secret Token**
   - Pros: Cryptographically secure
   - Pros: Standard authentication pattern
   - Cons: Requires token distribution to tmux sessions
   - Cons: Token management complexity
   - Rejected: Overkill for localhost-only communication

3. **Process ID Verification**
   - Pros: OS-level process identity
   - Pros: Can verify process is tmux
   - Cons: Platform-specific
   - Cons: Requires process inspection
   - Rejected: Complex and fragile

**Decision**: Session ID-based verification with localhost-only restriction

**Security Model**:
```go
func (s *SessionService) SendNotification(
    ctx context.Context,
    req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
    // 1. Verify request comes from localhost
    if !isLocalhost(req.Peer().Addr) {
        return nil, connect.NewError(connect.CodePermissionDenied, 
            errors.New("notifications only accepted from localhost"))
    }
    
    // 2. Verify session exists
    session, err := s.storage.GetInstance(req.Msg.SessionId)
    if err != nil {
        return nil, connect.NewError(connect.CodeNotFound, 
            fmt.Errorf("session not found: %w", err))
    }
    
    // 3. Verify session is active (not paused/deleted)
    if session.Status == session.Paused || session.Status == session.Stopped {
        return nil, connect.NewError(connect.CodeFailedPrecondition, 
            errors.New("session is not active"))
    }
    
    // Accept notification...
}
```

**Consequences**:
- ✅ Simple authentication sufficient for localhost
- ✅ No token management complexity
- ✅ Prevents external notification injection
- ⚠️ Not suitable for remote notification sources (future consideration)

---

## Known Issues and Bug Prevention

### Potential Bug 1: Notification Loss During Server Restart [SEVERITY: High]

**Description**: If the stapler-squad server restarts while tmux sessions are running, notifications sent during downtime will be lost (HTTP request fails).

**Mitigation Strategy**:
1. **Client-side retry logic** with exponential backoff:
   ```bash
   # In notification client script
   MAX_RETRIES=3
   RETRY_DELAY=1
   for i in $(seq 1 $MAX_RETRIES); do
       if curl -f -X POST ...; then
           break
       fi
       sleep $((RETRY_DELAY * i))
   done
   ```

2. **Best-effort delivery** - Accept notification loss as acceptable trade-off:
   - Notifications are advisory, not critical for system operation
   - Session state remains correct even if notification is lost
   - User can still see session status via manual navigation

3. **Server availability check** before sending:
   ```bash
   # Quick health check (100ms timeout)
   if curl -f --max-time 0.1 http://localhost:3000/health 2>/dev/null; then
       send_notification
   fi
   ```

**Prevention Strategy**:
- Document retry behavior in notification client library
- Add `--no-retry` flag for performance-critical contexts
- Log notification send failures for debugging
- Monitor notification success rate via metrics

**Files Likely Affected**:
- `scripts/notify-server.sh` (notification client script)
- `server/services/notification_service.go` (RPC handler)

---

### Potential Bug 2: Race Condition in Event Broadcasting [SEVERITY: Medium]

**Description**: Multiple simultaneous notifications from different sessions could cause event bus subscriber buffer overflow, dropping events.

**Root Cause Analysis**:
- EventBus has fixed buffer size per subscriber (100 events)
- Non-blocking send drops events if buffer is full
- High notification rate (10+ sessions sending simultaneously) could overflow

**Mitigation Strategy**:
1. **Increase event bus buffer size** for notification subscribers:
   ```go
   // In server initialization
   notificationEventBus := events.NewEventBus(500) // Larger buffer
   ```

2. **Rate limiting on notification endpoint**:
   ```go
   // Per-session rate limiter
   type RateLimiter struct {
       limiter *rate.Limiter  // 10 notifications/second per session
   }
   ```

3. **Priority queue for high-priority notifications**:
   ```go
   // Separate channel for urgent notifications
   type PriorityEventBus struct {
       urgentCh   chan *Event  // Higher priority
       normalCh   chan *Event  // Normal priority
   }
   ```

**Prevention Strategy**:
- Load test with 50+ simultaneous notifications
- Monitor event bus drop rate via metrics
- Alert on buffer overflow conditions
- Document notification rate limits

**Files Likely Affected**:
- `server/events/bus.go` (event bus implementation)
- `server/services/notification_service.go` (rate limiting)

---

### Potential Bug 3: TUI Layout Corruption from Banner Rendering [SEVERITY: High]

**Description**: Notification banner overlay could corrupt TUI layout if terminal dimensions change during rendering or if banner height calculation is incorrect.

**Root Cause Analysis**:
- BubbleTea layout calculations assume static dimensions
- Banner rendering reduces available height for main list
- Race between layout calculation and banner render
- Terminal resize during banner display could cause overlap

**Mitigation Strategy**:
1. **Defensive height calculations**:
   ```go
   func (m *Model) View() string {
       availableHeight := m.height
       
       // Reserve space for banner (defensive: always reserve 3 lines)
       if m.notificationBanner != nil {
           bannerHeight := m.notificationBanner.Height()
           if bannerHeight <= 0 {
               bannerHeight = 3  // Fallback minimum
           }
           availableHeight -= bannerHeight
       }
       
       // Ensure we never go negative
       if availableHeight < 1 {
           availableHeight = 1
       }
       
       // Render main content with reduced height
       return m.renderContent(availableHeight)
   }
   ```

2. **Atomic layout updates**:
   ```go
   // Lock during layout calculation
   m.layoutMutex.Lock()
   defer m.layoutMutex.Unlock()
   ```

3. **Banner dismissal on terminal resize**:
   ```go
   func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
       switch msg := msg.(type) {
       case tea.WindowSizeMsg:
           // Dismiss banner on resize to force recalculation
           m.notificationBanner = nil
       }
   }
   ```

**Prevention Strategy**:
- Unit tests for banner height calculation edge cases
- Integration tests with terminal resize events
- Visual regression testing with various terminal sizes
- Fuzzing with random resize sequences

**Testing Approach**:
```go
func TestNotificationBannerResize(t *testing.T) {
    model := NewModel()
    
    // Show banner
    model, _ = model.Update(NotificationMsg{...})
    
    // Resize terminal (simulate race condition)
    model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
    
    // Verify layout remains valid
    view := model.View()
    lines := strings.Split(view, "\n")
    assert.LessOrEqual(t, len(lines), 24)  // No overflow
}
```

**Files Likely Affected**:
- `app/app.go` (main TUI model and layout)
- `ui/notification_banner.go` (new file - banner rendering)
- `app/layout.go` (layout calculation logic)

---

### Potential Bug 4: Memory Leak from Persistent Notification History [SEVERITY: Medium]

**Description**: Notification history in web UI could grow unbounded, causing memory leaks in long-running browser tabs.

**Root Cause Analysis**:
- NotificationContext stores all notifications in state
- No automatic cleanup of old notifications
- Long-running sessions accumulate thousands of notifications
- Each notification includes metadata (session info, timestamps)

**Mitigation Strategy**:
1. **Automatic history pruning**:
   ```typescript
   // In NotificationContext
   const MAX_HISTORY_SIZE = 100;  // Keep last 100 notifications
   
   const addNotification = (notification: NotificationData) => {
       setNotificationHistory(prev => {
           const updated = [notification, ...prev];
           return updated.slice(0, MAX_HISTORY_SIZE);  // Prune oldest
       });
   };
   ```

2. **Age-based cleanup**:
   ```typescript
   // Prune notifications older than 24 hours
   useEffect(() => {
       const interval = setInterval(() => {
           const cutoff = Date.now() - 24 * 60 * 60 * 1000;
           setNotificationHistory(prev => 
               prev.filter(n => n.timestamp > cutoff)
           );
       }, 60 * 60 * 1000);  // Check hourly
       
       return () => clearInterval(interval);
   }, []);
   ```

3. **User-configurable history size**:
   ```typescript
   // Allow users to set history limit
   const historyLimit = parseInt(
       localStorage.getItem('notification-history-limit') || '100'
   );
   ```

**Prevention Strategy**:
- Monitor notification history size in dev tools
- Add memory profiling for long-running sessions
- Document history limits in user settings
- Provide "Clear History" button

**Files Likely Affected**:
- `web-app/src/lib/contexts/NotificationContext.tsx` (history management)
- `web-app/src/components/ui/NotificationPanel.tsx` (cleanup UI)

---

### Potential Bug 5: Audio Chime Playback Failure on Firefox [SEVERITY: Low]

**Description**: Web Audio API usage for notification chimes may fail on Firefox due to autoplay policies or AudioContext restrictions.

**Root Cause Analysis**:
- Firefox requires user interaction before allowing AudioContext
- Autoplay policies block audio without user gesture
- `notifications.ts` creates AudioContext on every chime
- No fallback for audio failure

**Mitigation Strategy**:
1. **Lazy AudioContext initialization with user interaction**:
   ```typescript
   let audioContext: AudioContext | null = null;
   
   // Initialize on first user interaction
   export function initAudio() {
       if (!audioContext) {
           audioContext = new (window.AudioContext || 
               (window as any).webkitAudioContext)();
       }
       return audioContext;
   }
   
   // Call from button click handler
   document.addEventListener('click', initAudio, { once: true });
   ```

2. **Fallback to HTML5 Audio element**:
   ```typescript
   export function playNotificationSound(soundType: NotificationSound) {
       try {
           // Try Web Audio API first
           playWithWebAudio(soundType);
       } catch (error) {
           // Fallback to HTML5 Audio
           const audio = new Audio(`/sounds/${soundType}.mp3`);
           audio.play().catch(console.warn);
       }
   }
   ```

3. **Silent notification option**:
   ```typescript
   // Allow users to disable audio
   if (getNotificationPreference().audioEnabled) {
       playNotificationSound(NotificationSound.CHIME);
   }
   ```

**Prevention Strategy**:
- Test on Firefox with strict autoplay policies enabled
- Add user preference for audio on/off
- Log audio failures for monitoring
- Document browser compatibility requirements

**Files Likely Affected**:
- `web-app/src/lib/utils/notifications.ts` (audio playback)
- `web-app/src/lib/contexts/NotificationContext.tsx` (audio init)

---

## Story Breakdown

### Story 1: Backend Notification Infrastructure (Week 1: 3 days)

**Objective**: Implement server-side notification reception, validation, and event broadcasting infrastructure.

**Value**: Establishes the core communication channel between tmux sessions and the server, enabling all downstream notification features.

**Dependencies**: None (foundational)

**Acceptance Criteria**:
- [ ] SendNotification RPC endpoint accepts HTTP POST requests
- [ ] Session ID validation prevents unauthorized notifications
- [ ] Notification events broadcast to all WatchSessions subscribers
- [ ] Rate limiting prevents notification flooding (10/sec per session)
- [ ] Localhost-only restriction enforced
- [ ] Error responses return actionable messages
- [ ] Unit tests cover validation edge cases
- [ ] Integration tests verify end-to-end notification flow

---

### Story 2: Notification Client Library and Tooling (Week 1: 2 days)

**Objective**: Create shell script and library for tmux sessions to send notifications to the server.

**Value**: Enables Claude Code instances to send notifications with minimal integration effort.

**Dependencies**: Story 1 (requires server endpoint)

**Acceptance Criteria**:
- [ ] Shell script `notify-server` accepts notification parameters
- [ ] Retry logic with exponential backoff (3 attempts)
- [ ] Health check before sending (optional)
- [ ] Error handling with meaningful exit codes
- [ ] Documentation with usage examples
- [ ] Integration with Claude Code example scripts
- [ ] Cross-platform support (macOS, Linux)

---

### Story 3: Web UI Notification Integration (Week 2: 3 days)

**Objective**: Integrate notification events into web UI with toast alerts, audio chimes, and notification panel.

**Value**: Provides visual and audio feedback to web users when sessions need attention.

**Dependencies**: Story 1 (requires notification events)

**Acceptance Criteria**:
- [ ] WatchSessions subscription includes notification events
- [ ] Toast notifications display for new notifications
- [ ] Audio chimes play with configurable sound type
- [ ] Notification panel shows history with filters
- [ ] Priority-based UI treatment (colors, persistence)
- [ ] User preferences for audio on/off
- [ ] Notification history pruning (100 item limit)
- [ ] Audit logging for notification interactions

---

### Story 4: TUI Notification Banner (Week 2: 2 days)

**Objective**: Implement notification banner overlay in BubbleTea TUI with auto-dismiss and keyboard controls.

**Value**: Brings notification awareness to TUI users (primary use case).

**Dependencies**: Story 1 (requires notification events)

**Acceptance Criteria**:
- [ ] Notification banner renders at top of screen
- [ ] Banner auto-dismisses after priority-based timeout
- [ ] Keyboard shortcut ('d') to dismiss manually
- [ ] Layout calculations account for banner height
- [ ] Terminal resize dismisses banner gracefully
- [ ] Priority-based styling (colors via ANSI codes)
- [ ] No layout corruption during banner display
- [ ] Multiple notifications queue properly

---

### Story 5: Protobuf Schema and Event Types (Week 1: 1 day)

**Objective**: Define notification-specific protobuf messages and event types.

**Value**: Establishes type-safe contract between client and server for notifications.

**Dependencies**: None (can be done in parallel with Story 1)

**Acceptance Criteria**:
- [ ] NotificationType enum with 12+ types
- [ ] NotificationPriority enum with 4 levels
- [ ] SendNotificationRequest message with metadata
- [ ] SendNotificationResponse message
- [ ] NotificationEvent extends SessionEvent
- [ ] Protobuf compilation generates Go and TypeScript
- [ ] Documentation of event type semantics

---

## Atomic Task Breakdown

### Story 1: Backend Notification Infrastructure

#### Task 1.1: Define Protobuf Notification Messages (2h) - Small

**Scope**: Add notification-specific message types to protobuf schema.

**Files**:
- `proto/session/v1/types.proto` (modify) - Add NotificationType and NotificationPriority enums
- `proto/session/v1/session.proto` (modify) - Add SendNotification RPC definition
- `proto/session/v1/events.proto` (modify) - Add NotificationEvent message

**Implementation**:
```protobuf
// proto/session/v1/types.proto
enum NotificationType {
  NOTIFICATION_TYPE_UNSPECIFIED = 0;
  NOTIFICATION_TYPE_APPROVAL_NEEDED = 1;
  NOTIFICATION_TYPE_INPUT_REQUIRED = 2;
  NOTIFICATION_TYPE_TASK_COMPLETE = 4;
  NOTIFICATION_TYPE_ERROR = 7;
  NOTIFICATION_TYPE_INFO = 10;
  // ... (12+ types)
}

enum NotificationPriority {
  NOTIFICATION_PRIORITY_UNSPECIFIED = 0;
  NOTIFICATION_PRIORITY_LOW = 1;
  NOTIFICATION_PRIORITY_MEDIUM = 2;
  NOTIFICATION_PRIORITY_HIGH = 3;
  NOTIFICATION_PRIORITY_URGENT = 4;
}

// proto/session/v1/session.proto
service SessionService {
  // ... existing RPCs ...
  
  // SendNotification allows tmux sessions to send notifications to the server
  rpc SendNotification(SendNotificationRequest) returns (SendNotificationResponse) {}
}

message SendNotificationRequest {
  // Session identifier sending the notification
  string session_id = 1;
  
  // Type of notification
  NotificationType notification_type = 2;
  
  // Priority level
  NotificationPriority priority = 3;
  
  // Human-readable title
  string title = 4;
  
  // Detailed message
  string message = 5;
  
  // Optional metadata (JSON-serialized context)
  map<string, string> metadata = 6;
}

message SendNotificationResponse {
  // Whether notification was accepted
  bool success = 1;
  
  // Human-readable response message
  string message = 2;
  
  // Notification ID (for tracking)
  string notification_id = 3;
}

// proto/session/v1/events.proto
message NotificationEvent {
  // Session that sent the notification
  string session_id = 1;
  
  // Session name (for display)
  string session_name = 2;
  
  // Notification details
  NotificationType notification_type = 3;
  NotificationPriority priority = 4;
  string title = 5;
  string message = 6;
  map<string, string> metadata = 7;
  
  // When notification was sent
  google.protobuf.Timestamp timestamp = 8;
}

// Extend SessionEvent with notification
message SessionEvent {
  google.protobuf.Timestamp timestamp = 1;
  
  oneof event {
    // ... existing event types ...
    NotificationEvent notification = 9;
  }
}
```

**Success Criteria**:
- [ ] Protobuf compiles without errors
- [ ] Go code generated in `gen/proto/go/session/v1/`
- [ ] TypeScript code generated in `web-app/src/gen/session/v1/`
- [ ] All 12+ notification types defined
- [ ] Priority levels map to UI treatment

**Testing**:
- Manual: `make protobuf` compiles successfully
- Automated: CI validates protobuf compilation

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 1.2: Implement SendNotification RPC Handler (4h) - Medium

**Scope**: Create server-side handler for notification requests with validation and event broadcasting.

**Files**:
- `server/services/session_service.go` (modify) - Add SendNotification RPC handler
- `server/events/types.go` (modify) - Add NewNotificationEvent constructor

**Context**:
- SessionService already has eventBus for broadcasting
- Need to validate session exists and is active
- Must enforce localhost-only restriction
- Rate limiting via per-session token bucket

**Implementation**:
```go
// server/services/session_service.go

// SendNotification receives notifications from tmux sessions and broadcasts them.
func (s *SessionService) SendNotification(
    ctx context.Context,
    req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
    // 1. Validate request comes from localhost
    peerAddr := req.Peer().Addr
    if !isLocalhostAddr(peerAddr) {
        log.WarningLog.Printf("Rejected notification from non-localhost: %s", peerAddr)
        return nil, connect.NewError(connect.CodePermissionDenied, 
            errors.New("notifications only accepted from localhost"))
    }
    
    // 2. Validate session exists
    session, err := s.storage.GetInstance(req.Msg.SessionId)
    if err != nil {
        return nil, connect.NewError(connect.CodeNotFound, 
            fmt.Errorf("session not found: %w", err))
    }
    
    // 3. Validate session is active
    if session.Status == session.Paused {
        return nil, connect.NewError(connect.CodeFailedPrecondition, 
            errors.New("session is paused"))
    }
    
    // 4. Rate limiting (10 notifications/sec per session)
    if !s.rateLimiter.Allow(req.Msg.SessionId) {
        log.WarningLog.Printf("Rate limit exceeded for session: %s", req.Msg.SessionId)
        return nil, connect.NewError(connect.CodeResourceExhausted, 
            errors.New("notification rate limit exceeded"))
    }
    
    // 5. Generate notification ID
    notificationID := generateNotificationID()
    
    // 6. Broadcast notification event
    event := events.NewNotificationEvent(
        session,
        req.Msg.NotificationType,
        req.Msg.Priority,
        req.Msg.Title,
        req.Msg.Message,
        req.Msg.Metadata,
    )
    s.eventBus.Publish(event)
    
    log.InfoLog.Printf("Notification from session %s: %s", 
        req.Msg.SessionId, req.Msg.Title)
    
    return connect.NewResponse(&sessionv1.SendNotificationResponse{
        Success: true,
        Message: "Notification sent successfully",
        NotificationId: notificationID,
    }), nil
}

// isLocalhostAddr checks if address is localhost
func isLocalhostAddr(addr string) bool {
    if addr == "" {
        return false
    }
    // Check for localhost patterns
    return strings.HasPrefix(addr, "127.0.0.1") || 
           strings.HasPrefix(addr, "::1") ||
           strings.Contains(addr, "localhost")
}

// generateNotificationID creates unique notification identifier
func generateNotificationID() string {
    return fmt.Sprintf("notif_%d_%s", 
        time.Now().UnixNano(), 
        uuid.New().String()[:8])
}
```

```go
// server/events/types.go

const (
    // ... existing event types ...
    EventNotification EventType = "session.notification"
)

// NewNotificationEvent creates a notification event.
func NewNotificationEvent(
    sess *session.Instance,
    notifType sessionv1.NotificationType,
    priority sessionv1.NotificationPriority,
    title string,
    message string,
    metadata map[string]string,
) *Event {
    return &Event{
        Type:      EventNotification,
        Timestamp: time.Now(),
        Session:   sess,
        SessionID: sess.Title,
        Context:   title,
        Metadata: map[string]interface{}{
            "notification_type": notifType,
            "priority":          priority,
            "title":             title,
            "message":           message,
            "metadata":          metadata,
        },
    }
}
```

**Success Criteria**:
- [ ] SendNotification RPC handler registered
- [ ] Localhost validation enforces security
- [ ] Session existence check prevents invalid notifications
- [ ] Rate limiting prevents flooding
- [ ] Event broadcast to all subscribers
- [ ] Proper error responses for all failure modes
- [ ] Logging includes session ID and notification details

**Testing**:
```go
func TestSendNotification(t *testing.T) {
    service := setupTestService(t)
    
    // Create test session
    session := createTestSession(service, "test-session")
    
    // Send notification
    resp, err := service.SendNotification(context.Background(), 
        connect.NewRequest(&sessionv1.SendNotificationRequest{
            SessionId:        "test-session",
            NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_INFO,
            Priority:         sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM,
            Title:            "Test Notification",
            Message:          "This is a test",
        }))
    
    assert.NoError(t, err)
    assert.True(t, resp.Msg.Success)
    assert.NotEmpty(t, resp.Msg.NotificationId)
}

func TestSendNotification_NonLocalhost(t *testing.T) {
    service := setupTestService(t)
    
    // Simulate non-localhost request (mock peer address)
    req := connect.NewRequest(&sessionv1.SendNotificationRequest{
        SessionId: "test-session",
    })
    // Mock peer address to simulate non-localhost
    
    resp, err := service.SendNotification(context.Background(), req)
    
    assert.Error(t, err)
    assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestSendNotification_RateLimit(t *testing.T) {
    service := setupTestService(t)
    session := createTestSession(service, "test-session")
    
    // Send 11 notifications rapidly (rate limit is 10/sec)
    for i := 0; i < 11; i++ {
        _, err := service.SendNotification(context.Background(), 
            connect.NewRequest(&sessionv1.SendNotificationRequest{
                SessionId: "test-session",
                NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_INFO,
                Title: fmt.Sprintf("Notification %d", i),
            }))
        
        if i < 10 {
            assert.NoError(t, err)
        } else {
            // 11th should be rate limited
            assert.Error(t, err)
            assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
        }
    }
}
```

**Dependencies**: Task 1.1 (protobuf definitions)

**Status**: ⏳ Pending

---

#### Task 1.3: Add Rate Limiter for Notifications (2h) - Small

**Scope**: Implement token bucket rate limiter to prevent notification flooding.

**Files**:
- `server/services/rate_limiter.go` (new) - Token bucket rate limiter
- `server/services/session_service.go` (modify) - Integrate rate limiter

**Implementation**:
```go
// server/services/rate_limiter.go

package services

import (
    "sync"
    "time"
    "golang.org/x/time/rate"
)

// NotificationRateLimiter provides per-session rate limiting.
type NotificationRateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

// NewNotificationRateLimiter creates a rate limiter.
// rate: notifications per second (e.g., 10)
// burst: max burst size (e.g., 20)
func NewNotificationRateLimiter(r float64, b int) *NotificationRateLimiter {
    return &NotificationRateLimiter{
        limiters: make(map[string]*rate.Limiter),
        rate:     rate.Limit(r),
        burst:    b,
    }
}

// Allow checks if notification is allowed for session.
func (rl *NotificationRateLimiter) Allow(sessionID string) bool {
    rl.mu.Lock()
    limiter, exists := rl.limiters[sessionID]
    if !exists {
        limiter = rate.NewLimiter(rl.rate, rl.burst)
        rl.limiters[sessionID] = limiter
    }
    rl.mu.Unlock()
    
    return limiter.Allow()
}

// Cleanup removes rate limiters for inactive sessions.
func (rl *NotificationRateLimiter) Cleanup(activeSessions []string) {
    active := make(map[string]bool)
    for _, id := range activeSessions {
        active[id] = true
    }
    
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    for sessionID := range rl.limiters {
        if !active[sessionID] {
            delete(rl.limiters, sessionID)
        }
    }
}
```

**Success Criteria**:
- [ ] Rate limiter allows 10 notifications/sec per session
- [ ] Burst allows up to 20 rapid notifications
- [ ] Cleanup removes limiters for inactive sessions
- [ ] Thread-safe concurrent access

**Testing**:
```go
func TestRateLimiter(t *testing.T) {
    rl := NewNotificationRateLimiter(10, 20)
    
    // First 20 should succeed (burst)
    for i := 0; i < 20; i++ {
        assert.True(t, rl.Allow("session-1"))
    }
    
    // 21st should fail (burst exhausted)
    assert.False(t, rl.Allow("session-1"))
    
    // Wait for token replenishment
    time.Sleep(200 * time.Millisecond)
    
    // Should allow 2 more (10/sec * 0.2sec = 2)
    assert.True(t, rl.Allow("session-1"))
    assert.True(t, rl.Allow("session-1"))
}
```

**Dependencies**: None

**Status**: ⏳ Pending

---

### Story 2: Notification Client Library

#### Task 2.1: Create Shell Notification Script (3h) - Small

**Scope**: Shell script for tmux sessions to send notifications via HTTP.

**Files**:
- `scripts/notify-server` (new) - Shell notification client
- `scripts/notify-server.1` (new) - Man page documentation

**Implementation**:
```bash
#!/usr/bin/env bash
# notify-server - Send notification to stapler-squad server
# Usage: notify-server --session SESSION_ID --type TYPE --title TITLE [options]

set -euo pipefail

# Default configuration
STAPLER_SQUAD_URL="${STAPLER_SQUAD_URL:-http://localhost:3000}"
MAX_RETRIES=3
RETRY_DELAY=1
HEALTH_CHECK=true

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --session|-s)
            SESSION_ID="$2"
            shift 2
            ;;
        --type|-t)
            NOTIFICATION_TYPE="$2"
            shift 2
            ;;
        --title)
            TITLE="$2"
            shift 2
            ;;
        --message|-m)
            MESSAGE="$2"
            shift 2
            ;;
        --priority|-p)
            PRIORITY="$2"
            shift 2
            ;;
        --no-retry)
            MAX_RETRIES=1
            shift
            ;;
        --no-health-check)
            HEALTH_CHECK=false
            shift
            ;;
        --help|-h)
            show_usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "${SESSION_ID:-}" ]]; then
    echo "Error: --session is required" >&2
    exit 1
fi

if [[ -z "${NOTIFICATION_TYPE:-}" ]]; then
    echo "Error: --type is required" >&2
    exit 1
fi

if [[ -z "${TITLE:-}" ]]; then
    echo "Error: --title is required" >&2
    exit 1
fi

# Default values
MESSAGE="${MESSAGE:-}"
PRIORITY="${PRIORITY:-MEDIUM}"

# Health check (optional)
if [[ "$HEALTH_CHECK" == "true" ]]; then
    if ! curl -f --max-time 0.5 "${STAPLER_SQUAD_URL}/health" &>/dev/null; then
        echo "Warning: Server health check failed, notification may not be delivered" >&2
    fi
fi

# Build JSON payload
JSON_PAYLOAD=$(jq -n \
    --arg session_id "$SESSION_ID" \
    --arg notification_type "$NOTIFICATION_TYPE" \
    --arg priority "$PRIORITY" \
    --arg title "$TITLE" \
    --arg message "$MESSAGE" \
    '{
        session_id: $session_id,
        notification_type: $notification_type,
        priority: $priority,
        title: $title,
        message: $message
    }')

# Send notification with retry
for attempt in $(seq 1 "$MAX_RETRIES"); do
    if curl -f -X POST \
        -H "Content-Type: application/json" \
        -d "$JSON_PAYLOAD" \
        "${STAPLER_SQUAD_URL}/api/session.v1.SessionService/SendNotification" \
        &>/dev/null; then
        exit 0
    fi
    
    if [[ $attempt -lt $MAX_RETRIES ]]; then
        sleep $((RETRY_DELAY * attempt))
    fi
done

echo "Error: Failed to send notification after $MAX_RETRIES attempts" >&2
exit 1
```

**Success Criteria**:
- [ ] Script sends HTTP POST with JSON payload
- [ ] Retry logic with exponential backoff
- [ ] Health check before sending (optional)
- [ ] Proper error handling and exit codes
- [ ] Man page documentation
- [ ] Supports all notification types and priorities

**Testing**:
```bash
# Manual testing
./scripts/notify-server \
    --session test-session \
    --type APPROVAL_NEEDED \
    --title "Approval Required" \
    --message "Claude needs your permission" \
    --priority HIGH

# Unit testing (bats)
@test "notify-server sends notification" {
    run ./scripts/notify-server \
        --session test-session \
        --type INFO \
        --title "Test"
    
    [ "$status" -eq 0 ]
}

@test "notify-server fails without required args" {
    run ./scripts/notify-server --title "Test"
    [ "$status" -ne 0 ]
}
```

**Dependencies**: Task 1.2 (server endpoint)

**Status**: ⏳ Pending

---

#### Task 2.2: Create Python Notification Library (2h) - Small

**Scope**: Python library for notification sending (optional, for advanced use cases).

**Files**:
- `scripts/notify.py` (new) - Python notification client

**Implementation**:
```python
#!/usr/bin/env python3
"""
notify.py - Python library for sending notifications to stapler-squad server
"""

import os
import sys
import json
import time
import requests
from typing import Optional, Dict

class NotificationClient:
    def __init__(
        self,
        base_url: str = None,
        max_retries: int = 3,
        retry_delay: float = 1.0,
    ):
        self.base_url = base_url or os.getenv(
            "STAPLER_SQUAD_URL", "http://localhost:3000"
        )
        self.max_retries = max_retries
        self.retry_delay = retry_delay
    
    def send(
        self,
        session_id: str,
        notification_type: str,
        title: str,
        message: str = "",
        priority: str = "MEDIUM",
        metadata: Optional[Dict[str, str]] = None,
    ) -> bool:
        """Send notification to server with retry logic."""
        payload = {
            "session_id": session_id,
            "notification_type": notification_type,
            "priority": priority,
            "title": title,
            "message": message,
            "metadata": metadata or {},
        }
        
        for attempt in range(1, self.max_retries + 1):
            try:
                response = requests.post(
                    f"{self.base_url}/api/session.v1.SessionService/SendNotification",
                    json=payload,
                    timeout=5,
                )
                response.raise_for_status()
                return True
            except requests.RequestException as e:
                if attempt < self.max_retries:
                    time.sleep(self.retry_delay * attempt)
                else:
                    print(f"Error: Failed to send notification: {e}", file=sys.stderr)
                    return False
        
        return False

def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="Send notification to stapler-squad")
    parser.add_argument("--session", "-s", required=True, help="Session ID")
    parser.add_argument("--type", "-t", required=True, help="Notification type")
    parser.add_argument("--title", required=True, help="Notification title")
    parser.add_argument("--message", "-m", default="", help="Notification message")
    parser.add_argument("--priority", "-p", default="MEDIUM", help="Priority level")
    
    args = parser.parse_args()
    
    client = NotificationClient()
    success = client.send(
        session_id=args.session,
        notification_type=args.type,
        title=args.title,
        message=args.message,
        priority=args.priority,
    )
    
    sys.exit(0 if success else 1)

if __name__ == "__main__":
    main()
```

**Success Criteria**:
- [ ] Python 3.7+ compatibility
- [ ] Retry logic with exponential backoff
- [ ] Type hints for IDE support
- [ ] Command-line interface
- [ ] Importable as library

**Testing**:
```python
def test_notification_send():
    client = NotificationClient()
    result = client.send(
        session_id="test-session",
        notification_type="INFO",
        title="Test Notification",
    )
    assert result == True
```

**Dependencies**: Task 1.2 (server endpoint)

**Status**: ⏳ Pending

---

### Story 3: Web UI Notification Integration

#### Task 3.1: Subscribe to Notification Events (3h) - Medium

**Scope**: Extend WatchSessions subscription to receive notification events.

**Files**:
- `web-app/src/lib/hooks/useSessions.ts` (modify) - Add notification event handling
- `web-app/src/lib/contexts/NotificationContext.tsx` (modify) - Integrate notifications

**Implementation**:
```typescript
// web-app/src/lib/hooks/useSessions.ts

import { NotificationEvent } from "@/gen/session/v1/events_pb";
import { useNotifications } from "@/lib/contexts/NotificationContext";

export function useSessions() {
  const { addNotification } = useNotifications();
  
  useEffect(() => {
    const stream = client.watchSessions({});
    
    for await (const event of stream) {
      switch (event.event.case) {
        case "notification":
          handleNotificationEvent(event.event.value);
          break;
        // ... other event types
      }
    }
  }, []);
  
  const handleNotificationEvent = (notification: NotificationEvent) => {
    addNotification({
      id: generateNotificationId(),
      sessionId: notification.sessionId,
      sessionName: notification.sessionName,
      message: notification.message,
      title: notification.title,
      priority: mapPriority(notification.priority),
      timestamp: Date.now(),
      metadata: notification.metadata,
    });
    
    // Play audio chime based on priority
    const soundType = getSoundTypeForPriority(notification.priority);
    playNotificationSound(soundType);
  };
  
  const mapPriority = (priority: NotificationPriority): string => {
    switch (priority) {
      case NotificationPriority.NOTIFICATION_PRIORITY_URGENT:
        return "urgent";
      case NotificationPriority.NOTIFICATION_PRIORITY_HIGH:
        return "high";
      case NotificationPriority.NOTIFICATION_PRIORITY_MEDIUM:
        return "medium";
      case NotificationPriority.NOTIFICATION_PRIORITY_LOW:
        return "low";
      default:
        return "medium";
    }
  };
  
  const getSoundTypeForPriority = (priority: NotificationPriority): NotificationSound => {
    switch (priority) {
      case NotificationPriority.NOTIFICATION_PRIORITY_URGENT:
        return NotificationSound.ALERT;
      case NotificationPriority.NOTIFICATION_PRIORITY_HIGH:
        return NotificationSound.CHIME;
      default:
        return NotificationSound.DING;
    }
  };
}
```

**Success Criteria**:
- [ ] WatchSessions subscription receives notification events
- [ ] Notification events added to NotificationContext
- [ ] Audio chime plays based on priority
- [ ] Toast appears with notification details
- [ ] Event handling doesn't block UI thread

**Testing**:
```typescript
describe("Notification Event Handling", () => {
  it("should add notification on event", async () => {
    const { result } = renderHook(() => useNotifications());
    
    // Simulate notification event
    const event = new NotificationEvent({
      sessionId: "test-session",
      sessionName: "Test Session",
      title: "Test Notification",
      message: "This is a test",
      priority: NotificationPriority.NOTIFICATION_PRIORITY_MEDIUM,
    });
    
    // Trigger event handler
    handleNotificationEvent(event);
    
    await waitFor(() => {
      expect(result.current.notificationHistory).toHaveLength(1);
      expect(result.current.notificationHistory[0].title).toBe("Test Notification");
    });
  });
});
```

**Dependencies**: Task 1.2 (notification events), Story 1 complete

**Status**: ⏳ Pending

---

#### Task 3.2: Add Notification History Pruning (2h) - Small

**Scope**: Implement automatic cleanup of old notifications to prevent memory leaks.

**Files**:
- `web-app/src/lib/contexts/NotificationContext.tsx` (modify) - Add pruning logic

**Implementation**:
```typescript
// web-app/src/lib/contexts/NotificationContext.tsx

const MAX_HISTORY_SIZE = 100;
const MAX_AGE_MS = 24 * 60 * 60 * 1000; // 24 hours

export function NotificationContextProvider({ children }: { children: React.ReactNode }) {
  const [notificationHistory, setNotificationHistory] = useState<NotificationData[]>([]);
  
  // Prune old notifications periodically
  useEffect(() => {
    const interval = setInterval(() => {
      setNotificationHistory(prev => {
        const cutoff = Date.now() - MAX_AGE_MS;
        return prev
          .filter(n => n.timestamp > cutoff)
          .slice(0, MAX_HISTORY_SIZE);
      });
    }, 60 * 60 * 1000); // Check hourly
    
    return () => clearInterval(interval);
  }, []);
  
  const addNotification = (notification: NotificationData) => {
    setNotificationHistory(prev => {
      const updated = [notification, ...prev];
      // Immediate pruning on add
      return updated.slice(0, MAX_HISTORY_SIZE);
    });
  };
  
  // ... rest of context
}
```

**Success Criteria**:
- [ ] History limited to 100 most recent notifications
- [ ] Notifications older than 24 hours removed
- [ ] Pruning runs hourly in background
- [ ] Immediate pruning on add to prevent spikes

**Testing**:
```typescript
describe("Notification Pruning", () => {
  it("should limit history to 100 items", () => {
    const { result } = renderHook(() => useNotifications());
    
    // Add 150 notifications
    for (let i = 0; i < 150; i++) {
      act(() => {
        result.current.addNotification({
          id: `notif-${i}`,
          sessionId: "test",
          sessionName: "Test",
          message: `Notification ${i}`,
          timestamp: Date.now(),
        });
      });
    }
    
    expect(result.current.notificationHistory).toHaveLength(100);
  });
  
  it("should remove notifications older than 24 hours", async () => {
    const { result } = renderHook(() => useNotifications());
    
    // Add old notification
    act(() => {
      result.current.addNotification({
        id: "old-notif",
        sessionId: "test",
        sessionName: "Test",
        message: "Old notification",
        timestamp: Date.now() - 25 * 60 * 60 * 1000, // 25 hours ago
      });
    });
    
    // Trigger pruning
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 100));
    });
    
    expect(result.current.notificationHistory).toHaveLength(0);
  });
});
```

**Dependencies**: Task 3.1

**Status**: ⏳ Pending

---

### Story 4: TUI Notification Banner

#### Task 4.1: Create Notification Banner Component (4h) - Medium

**Scope**: BubbleTea component for rendering notification banner in TUI.

**Files**:
- `ui/notification_banner.go` (new) - Banner component
- `ui/notification_banner_test.go` (new) - Unit tests

**Implementation**:
```go
// ui/notification_banner.go

package ui

import (
    "fmt"
    "strings"
    "time"
    "github.com/charmbracelet/lipgloss"
)

// NotificationBanner renders a notification at the top of the screen.
type NotificationBanner struct {
    sessionID   string
    sessionName string
    title       string
    message     string
    priority    NotificationPriority
    timestamp   time.Time
    dismissed   bool
    width       int
}

type NotificationPriority int

const (
    PriorityLow NotificationPriority = iota
    PriorityMedium
    PriorityHigh
    PriorityUrgent
)

// NewNotificationBanner creates a notification banner.
func NewNotificationBanner(
    sessionID, sessionName, title, message string,
    priority NotificationPriority,
) *NotificationBanner {
    return &NotificationBanner{
        sessionID:   sessionID,
        sessionName: sessionName,
        title:       title,
        message:     message,
        priority:    priority,
        timestamp:   time.Now(),
        dismissed:   false,
    }
}

// View renders the notification banner.
func (nb *NotificationBanner) View() string {
    if nb.dismissed {
        return ""
    }
    
    // Priority-based styling
    style := nb.getStyle()
    
    // Icon based on priority
    icon := nb.getIcon()
    
    // Format banner content
    header := fmt.Sprintf("%s %s", icon, nb.sessionName)
    content := fmt.Sprintf("%s: %s", nb.title, nb.message)
    footer := fmt.Sprintf("[Press 'd' to dismiss]  %s", nb.formatTimestamp())
    
    // Truncate content to fit width
    if len(content) > nb.width-4 {
        content = content[:nb.width-7] + "..."
    }
    
    // Render with lipgloss styling
    banner := style.Render(
        header + "\n" +
        content + "\n" +
        footer,
    )
    
    return banner
}

// Height returns the banner height in lines.
func (nb *NotificationBanner) Height() int {
    if nb.dismissed {
        return 0
    }
    return 3 // Header + content + footer
}

// SetWidth updates banner width for responsive layout.
func (nb *NotificationBanner) SetWidth(width int) {
    nb.width = width
}

// Dismiss marks banner as dismissed.
func (nb *NotificationBanner) Dismiss() {
    nb.dismissed = true
}

// ShouldAutoDismiss checks if banner should auto-dismiss based on priority.
func (nb *NotificationBanner) ShouldAutoDismiss() bool {
    switch nb.priority {
    case PriorityLow:
        return time.Since(nb.timestamp) > 5*time.Second
    case PriorityMedium:
        return time.Since(nb.timestamp) > 10*time.Second
    case PriorityHigh, PriorityUrgent:
        return false // Manual dismissal only
    default:
        return false
    }
}

// getStyle returns lipgloss style based on priority.
func (nb *NotificationBanner) getStyle() lipgloss.Style {
    baseStyle := lipgloss.NewStyle().
        Padding(0, 1).
        Width(nb.width - 2)
    
    switch nb.priority {
    case PriorityUrgent:
        return baseStyle.
            Foreground(lipgloss.Color("#FFFFFF")).
            Background(lipgloss.Color("#FF0000")).
            Bold(true)
    case PriorityHigh:
        return baseStyle.
            Foreground(lipgloss.Color("#000000")).
            Background(lipgloss.Color("#FFA500"))
    case PriorityMedium:
        return baseStyle.
            Foreground(lipgloss.Color("#000000")).
            Background(lipgloss.Color("#87CEEB"))
    case PriorityLow:
        return baseStyle.
            Foreground(lipgloss.Color("#000000")).
            Background(lipgloss.Color("#90EE90"))
    default:
        return baseStyle
    }
}

// getIcon returns emoji icon based on priority.
func (nb *NotificationBanner) getIcon() string {
    switch nb.priority {
    case PriorityUrgent:
        return "🚨"
    case PriorityHigh:
        return "⚠️ "
    case PriorityMedium:
        return "ℹ️ "
    case PriorityLow:
        return "✓"
    default:
        return "🔔"
    }
}

// formatTimestamp returns human-readable time since notification.
func (nb *NotificationBanner) formatTimestamp() string {
    duration := time.Since(nb.timestamp)
    
    if duration < time.Minute {
        return "just now"
    } else if duration < time.Hour {
        mins := int(duration.Minutes())
        return fmt.Sprintf("%dm ago", mins)
    } else {
        hours := int(duration.Hours())
        return fmt.Sprintf("%dh ago", hours)
    }
}
```

**Success Criteria**:
- [ ] Banner renders with priority-based styling
- [ ] Icon and colors indicate priority level
- [ ] Auto-dismiss based on priority timeout
- [ ] Manual dismiss with 'd' key
- [ ] Responsive width adjustment
- [ ] Height calculation accurate

**Testing**:
```go
func TestNotificationBanner(t *testing.T) {
    banner := NewNotificationBanner(
        "test-session",
        "Test Session",
        "Approval Needed",
        "Claude needs your permission",
        PriorityHigh,
    )
    banner.SetWidth(80)
    
    // Test rendering
    view := banner.View()
    assert.Contains(t, view, "Test Session")
    assert.Contains(t, view, "Approval Needed")
    assert.NotEmpty(t, view)
    
    // Test height
    assert.Equal(t, 3, banner.Height())
    
    // Test dismiss
    banner.Dismiss()
    assert.Equal(t, 0, banner.Height())
    assert.Empty(t, banner.View())
}

func TestNotificationBanner_AutoDismiss(t *testing.T) {
    // Low priority should auto-dismiss after 5 seconds
    banner := NewNotificationBanner(
        "test", "Test", "Info", "Message", PriorityLow,
    )
    
    assert.False(t, banner.ShouldAutoDismiss())
    
    // Simulate time passing
    time.Sleep(6 * time.Second)
    
    assert.True(t, banner.ShouldAutoDismiss())
}
```

**Dependencies**: None (can be developed in parallel)

**Status**: ⏳ Pending

---

#### Task 4.2: Integrate Banner into Main TUI App (3h) - Medium

**Scope**: Add notification banner to app model with event handling and layout adjustment.

**Files**:
- `app/app.go` (modify) - Add banner to model and layout
- `app/notification_handler.go` (new) - Notification event handling

**Implementation**:
```go
// app/app.go

type Model struct {
    // ... existing fields ...
    
    // Notification banner (nil when no active notification)
    notificationBanner *ui.NotificationBanner
    
    // Notification queue for multiple notifications
    notificationQueue []*ui.NotificationBanner
}

// View renders the TUI with notification banner overlay.
func (m Model) View() string {
    // Calculate available height (reserve space for banner)
    availableHeight := m.height
    
    if m.notificationBanner != nil {
        bannerHeight := m.notificationBanner.Height()
        if bannerHeight <= 0 {
            bannerHeight = 3 // Defensive fallback
        }
        availableHeight -= bannerHeight
        
        // Ensure we never go negative
        if availableHeight < 1 {
            availableHeight = 1
        }
    }
    
    // Render main content with adjusted height
    mainContent := m.renderMainContent(availableHeight)
    
    // Overlay notification banner at top
    if m.notificationBanner != nil {
        m.notificationBanner.SetWidth(m.width)
        bannerView := m.notificationBanner.View()
        
        return bannerView + "\n" + mainContent
    }
    
    return mainContent
}

// Update handles messages including notification events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case NotificationMsg:
        return m.handleNotification(msg)
    
    case tea.KeyMsg:
        // Handle notification dismissal
        if msg.String() == "d" && m.notificationBanner != nil {
            m.notificationBanner.Dismiss()
            m.notificationBanner = nil
            
            // Show next notification from queue
            if len(m.notificationQueue) > 0 {
                m.notificationBanner = m.notificationQueue[0]
                m.notificationQueue = m.notificationQueue[1:]
            }
            
            return m, nil
        }
    
    case tea.WindowSizeMsg:
        // Dismiss banner on resize to prevent layout corruption
        m.notificationBanner = nil
        m.notificationQueue = nil
        m.width = msg.Width
        m.height = msg.Height
        return m, nil
    
    case AutoDismissMsg:
        // Check if current banner should auto-dismiss
        if m.notificationBanner != nil && m.notificationBanner.ShouldAutoDismiss() {
            m.notificationBanner = nil
            
            // Show next notification
            if len(m.notificationQueue) > 0 {
                m.notificationBanner = m.notificationQueue[0]
                m.notificationQueue = m.notificationQueue[1:]
            }
        }
        
        // Continue checking every second
        return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
            return AutoDismissMsg{}
        })
    }
    
    // ... existing update logic ...
}

// handleNotification processes notification event.
func (m Model) handleNotification(msg NotificationMsg) (Model, tea.Cmd) {
    banner := ui.NewNotificationBanner(
        msg.SessionID,
        msg.SessionName,
        msg.Title,
        msg.Message,
        mapPriority(msg.Priority),
    )
    
    // If no active banner, show immediately
    if m.notificationBanner == nil {
        m.notificationBanner = banner
    } else {
        // Queue notification for later
        m.notificationQueue = append(m.notificationQueue, banner)
    }
    
    return m, nil
}

// NotificationMsg carries notification event data.
type NotificationMsg struct {
    SessionID   string
    SessionName string
    Title       string
    Message     string
    Priority    string
}

// AutoDismissMsg triggers auto-dismiss check.
type AutoDismissMsg struct{}

func mapPriority(priority string) ui.NotificationPriority {
    switch priority {
    case "urgent":
        return ui.PriorityUrgent
    case "high":
        return ui.PriorityHigh
    case "medium":
        return ui.PriorityMedium
    case "low":
        return ui.PriorityLow
    default:
        return ui.PriorityMedium
    }
}
```

```go
// app/notification_handler.go

package app

import (
    "stapler-squad/server/events"
    tea "github.com/charmbracelet/bubbletea"
)

// SubscribeToNotifications creates a command that listens for notification events.
func SubscribeToNotifications(eventBus *events.EventBus) tea.Cmd {
    return func() tea.Msg {
        // Subscribe to event bus
        ctx := context.Background()
        eventCh, _ := eventBus.Subscribe(ctx)
        
        // Listen for notification events
        for event := range eventCh {
            if event.Type == events.EventNotification {
                // Convert to NotificationMsg
                return NotificationMsg{
                    SessionID:   event.SessionID,
                    SessionName: event.Session.Title,
                    Title:       event.Context,
                    Message:     fmt.Sprintf("%v", event.Metadata["message"]),
                    Priority:    fmt.Sprintf("%v", event.Metadata["priority"]),
                }
            }
        }
        
        return nil
    }
}
```

**Success Criteria**:
- [ ] Notification banner overlays at top of screen
- [ ] Layout recalculates available height correctly
- [ ] 'd' key dismisses current banner
- [ ] Auto-dismiss works based on priority
- [ ] Terminal resize dismisses banner gracefully
- [ ] Multiple notifications queue properly
- [ ] No layout corruption during banner display

**Testing**:
```go
func TestNotificationBannerIntegration(t *testing.T) {
    model := NewModel()
    model.width = 80
    model.height = 24
    
    // Send notification
    msg := NotificationMsg{
        SessionID:   "test-session",
        SessionName: "Test Session",
        Title:       "Test Notification",
        Message:     "This is a test",
        Priority:    "high",
    }
    
    model, _ = model.Update(msg)
    
    // Verify banner is active
    assert.NotNil(t, model.notificationBanner)
    
    // Verify layout adjustment
    view := model.View()
    lines := strings.Split(view, "\n")
    assert.LessOrEqual(t, len(lines), 24)
    
    // Verify banner content appears
    assert.Contains(t, view, "Test Session")
    assert.Contains(t, view, "Test Notification")
}

func TestNotificationBannerResize(t *testing.T) {
    model := NewModel()
    
    // Show banner
    model, _ = model.Update(NotificationMsg{
        SessionID:   "test",
        SessionName: "Test",
        Title:       "Notification",
        Message:     "Message",
        Priority:    "medium",
    })
    
    assert.NotNil(t, model.notificationBanner)
    
    // Resize terminal
    model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
    
    // Verify banner dismissed
    assert.Nil(t, model.notificationBanner)
}
```

**Dependencies**: Task 4.1 (banner component)

**Status**: ⏳ Pending

---

## Implementation Timeline

### Week 1: Backend and Client Tooling (5 days)

**Day 1-2**: Backend Notification Infrastructure (Story 1)
- Task 1.1: Define protobuf messages (2h)
- Task 1.2: Implement SendNotification RPC (4h)
- Task 1.3: Add rate limiter (2h)
- **Deliverable**: Working notification endpoint with validation

**Day 3-4**: Notification Client Library (Story 2)
- Task 2.1: Create shell script (3h)
- Task 2.2: Create Python library (2h)
- Documentation and examples (2h)
- **Deliverable**: Notification client tools ready for use

**Day 5**: Testing and Integration
- End-to-end testing of notification flow
- Performance testing with load simulation
- Bug fixes and refinements

### Week 2: UI Integration (5 days)

**Day 6-7**: Web UI Notification Integration (Story 3)
- Task 3.1: Subscribe to notification events (3h)
- Task 3.2: Add history pruning (2h)
- Testing and bug fixes (3h)
- **Deliverable**: Web UI receives and displays notifications

**Day 8-9**: TUI Notification Banner (Story 4)
- Task 4.1: Create banner component (4h)
- Task 4.2: Integrate into main app (3h)
- Testing with various scenarios (3h)
- **Deliverable**: TUI displays notification banners

**Day 10**: Final Integration and Documentation
- End-to-end testing across all components
- Performance validation
- Documentation updates
- User guide creation

---

## Testing Strategy

### Unit Tests

**Backend**:
- [ ] SendNotification RPC handler validation logic
- [ ] Rate limiter token bucket behavior
- [ ] Event broadcasting to subscribers
- [ ] Localhost restriction enforcement

**Frontend (Web)**:
- [ ] Notification event handling in useSessions
- [ ] History pruning logic
- [ ] Priority mapping correctness
- [ ] Audio playback triggers

**Frontend (TUI)**:
- [ ] Banner height calculation
- [ ] Auto-dismiss timing
- [ ] Layout adjustment with banner
- [ ] Priority-based styling

### Integration Tests

**End-to-End Notification Flow**:
```go
func TestNotificationE2E(t *testing.T) {
    // 1. Start server with event bus
    server := setupTestServer(t)
    defer server.Shutdown()
    
    // 2. Create test session
    session := createTestSession(server, "test-session")
    
    // 3. Subscribe to events (simulate web UI)
    eventCh, _ := server.EventBus().Subscribe(context.Background())
    
    // 4. Send notification via HTTP
    resp := sendNotificationHTTP(t, "test-session", NotificationTypeInfo, "Test")
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // 5. Verify event received
    select {
    case event := <-eventCh:
        assert.Equal(t, events.EventNotification, event.Type)
        assert.Equal(t, "test-session", event.SessionID)
    case <-time.After(1 * time.Second):
        t.Fatal("Notification event not received")
    }
}
```

**Cross-Component Tests**:
- Shell script → Server → Web UI notification display
- Python library → Server → TUI banner display
- Multiple simultaneous notifications → Queue behavior
- Server restart → Graceful reconnection

### Performance Tests

**Load Testing**:
```go
func BenchmarkNotificationThroughput(b *testing.B) {
    server := setupBenchServer(b)
    defer server.Shutdown()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            sendNotificationHTTP(b, "session-1", NotificationTypeInfo, "Test")
        }
    })
    // Target: >1000 notifications/sec
}
```

**Latency Testing**:
- Notification send to UI display: Target <500ms
- Audio chime playback: Target <200ms
- Banner rendering: Target <50ms

### Security Tests

- [ ] Non-localhost notification rejection
- [ ] Invalid session ID rejection
- [ ] Rate limit enforcement
- [ ] Paused session rejection
- [ ] Malformed JSON rejection

---

## Monitoring and Observability

### Metrics to Track

**Server-Side**:
- `notifications_received_total` (counter) - Total notifications received
- `notifications_sent_total` (counter) - Total notifications broadcast
- `notifications_rejected_total` (counter, by reason) - Rejected notifications
- `notification_latency_ms` (histogram) - Time from receive to broadcast
- `event_bus_subscribers` (gauge) - Active event bus subscribers
- `rate_limit_hits_total` (counter) - Rate limit rejections

**Client-Side (Web)**:
- `notification_displayed_total` (counter) - Notifications shown to user
- `notification_dismissed_total` (counter) - User dismissals
- `notification_audio_played_total` (counter) - Audio chimes played
- `notification_audio_failed_total` (counter) - Audio playback failures
- `notification_history_size` (gauge) - Current history size

### Logging Strategy

**Server Logs**:
```go
log.InfoLog.Printf("Notification received: session=%s type=%s priority=%s",
    sessionID, notificationType, priority)

log.WarningLog.Printf("Notification rejected: session=%s reason=%s",
    sessionID, reason)

log.ErrorLog.Printf("Failed to broadcast notification: session=%s error=%v",
    sessionID, err)
```

**Client Logs** (Web Console):
```typescript
console.log("[Notification] Received:", {
    sessionId: notification.sessionId,
    type: notification.type,
    priority: notification.priority,
});

console.warn("[Notification] Audio playback failed:", error);
```

### Alerting Rules

- **High notification rate**: >100 notifications/min from single session → Alert
- **Event bus subscriber drop**: Subscribers drop to 0 → Critical alert
- **Rate limit excessive**: >50% of notifications rate-limited → Warning
- **Audio failure rate**: >10% audio playback failures → Warning

---

## Rollout Plan

### Phase 1: Internal Testing (Week 1)
- Deploy to development environment
- Manual testing with shell script
- Verify event broadcasting
- Monitor for issues

### Phase 2: Limited Rollout (Week 2)
- Deploy to staging environment
- Enable for 10% of sessions
- Monitor metrics and error rates
- Gather user feedback

### Phase 3: Full Rollout (Week 3)
- Deploy to production
- Enable for all sessions
- Monitor performance at scale
- Document best practices

### Rollback Plan

If critical issues arise:
1. Disable notification endpoint via feature flag
2. Fall back to existing review queue polling
3. Fix issues in development
4. Re-deploy with fixes

---

## Documentation Requirements

### User Documentation

**Notification Client Guide** (`docs/notification-client.md`):
- How to send notifications from tmux sessions
- Shell script usage examples
- Python library API reference
- Notification type guidelines
- Best practices for notification frequency

**Web UI Guide** (`docs/web-ui-notifications.md`):
- How to enable/disable audio notifications
- Notification history management
- Priority level meanings
- Keyboard shortcuts

**TUI Guide** (`docs/tui-notifications.md`):
- Notification banner behavior
- Dismissal shortcuts
- Auto-dismiss timeouts
- Multiple notification handling

### Developer Documentation

**Architecture Overview** (`docs/architecture/notifications.md`):
- System design and data flow
- Component responsibilities
- Event bus integration
- Security model

**API Reference** (`docs/api/send-notification.md`):
- SendNotification RPC specification
- Request/response schemas
- Error codes and handling
- Rate limiting details

**Integration Guide** (`docs/integration/notification-client.md`):
- How to integrate notification client
- Custom notification types
- Metadata usage patterns
- Error handling strategies

---

## Success Criteria Summary

### Functional Requirements
- ✅ Tmux sessions can send notifications via HTTP API
- ✅ Server validates and broadcasts notification events
- ✅ Web UI displays toast notifications with audio
- ✅ TUI displays banner notifications
- ✅ Priority-based UI treatment (colors, persistence, audio)
- ✅ Rate limiting prevents notification flooding
- ✅ Localhost-only restriction enforced

### Non-Functional Requirements
- ✅ <500ms notification latency (send to display)
- ✅ <200ms audio chime playback
- ✅ <50ms banner rendering
- ✅ Support 10+ notification types
- ✅ Support 100+ simultaneous subscribers
- ✅ Zero layout corruption in TUI
- ✅ Graceful degradation on server unavailable

### Quality Requirements
- ✅ >85% test coverage for new code
- ✅ Zero critical bugs in production
- ✅ Zero performance regression
- ✅ Complete documentation
- ✅ Comprehensive error handling
- ✅ Observable via metrics and logs

---

## Future Enhancements

### Phase 2 Features (Post-MVP)
1. **Rich Notification Metadata**:
   - Attachments (diffs, logs)
   - Action buttons (Approve/Deny)
   - Custom styling per type

2. **Notification Filtering**:
   - User preferences for notification types
   - Session-based filtering
   - Priority-based filtering

3. **Cross-Session Communication**:
   - Session-to-session messaging
   - Broadcast to all sessions
   - Notification groups

4. **Advanced TUI Features**:
   - Notification history view
   - Inline notification actions
   - Banner queuing UI

5. **Remote Notification Support**:
   - Secure token authentication
   - Webhook integration
   - External service notifications

---

## Conclusion

This feature plan establishes a comprehensive notification system that bridges the gap between isolated tmux sessions and the stapler-squad server, enabling real-time awareness and improved developer productivity. The architecture follows best practices for event-driven systems, prioritizes performance and reliability, and includes extensive bug prevention measures and testing strategies.

The implementation is broken down into manageable atomic tasks with clear dependencies, success criteria, and testing requirements. The phased rollout approach ensures stability while gathering feedback, and the extensive documentation ensures long-term maintainability.

**Next Steps**:
1. Review and approve feature plan
2. Set up project tracking in issue tracker
3. Begin implementation with Story 1 (Backend Infrastructure)
4. Schedule weekly sync meetings to track progress

---

**Document Metadata**:
- **Author**: Claude Code (Architecture Planning Specialist)
- **Created**: 2025-12-05
- **Status**: Proposed
- **Version**: 1.0
- **Related ADRs**: ADR-008, ADR-009, ADR-010
