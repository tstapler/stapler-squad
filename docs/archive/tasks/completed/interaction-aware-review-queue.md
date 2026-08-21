# Interaction-Aware Review Queue - AIC Task Breakdown

## Epic: Real-Time Event-Driven Review Queue System

**Goal**: Transform the review queue from a polling-based system with 7-32 second latency and flickering issues into a reactive, event-driven system that responds to user interactions within <100ms.

**Value Proposition**:
- Immediate feedback when users interact with sessions (terminal input, acknowledgments, approvals)
- Zero flickering or inconsistent state in UI
- Keyboard navigation for efficient queue management
- Foundation for advanced features (auto-resolution, priority escalation, multi-user coordination)

**Success Metrics**:
- Queue updates appear within 100ms of user interaction (down from 7-32 seconds)
- Zero flickering or inconsistent state during updates
- Keyboard navigation ([ and ] keys) works smoothly
- Network traffic reduced by 90% (events vs polling)
- Foundation for smart features (priority escalation, context enrichment)

**Technical Scope**:
- Replace polling with WebSocket streaming for real-time updates
- Implement event-driven architecture (ReactiveQueueManager)
- Add navigation controls and optimistic UI updates
- Fix architectural issues (duplicate queue creation, missing event publishing)
- Enable advanced features through rich event model

---

## Story 1: Event-Driven Architecture Foundation (1-2 weeks)

**Objective**: Replace polling-based queue updates with an event-driven architecture that immediately responds to user interactions and state changes.

**Value**: Eliminates latency and enables reactive UI updates.

---

### Task 1.1: Define Event Types for Queue Updates (2h)

**Scope**: Create comprehensive event types for all interactions that should trigger queue re-evaluation.

**Files**:
- `proto/session/v1/events.proto` (modify)
- `session/events.go` (modify)

**Context**:
- Currently only SessionStateChanged and TerminalOutput events exist
- Need events for: terminal input, acknowledgments, approvals, session state changes
- Events should include rich context for immediate queue evaluation

**Implementation**:
```protobuf
// New event types for reactive queue management
message UserInteractionEvent {
  string session_id = 1;
  google.protobuf.Timestamp timestamp = 2;

  // Type of interaction
  enum InteractionType {
    TERMINAL_INPUT = 0;
    APPROVAL_RESPONSE = 1;
    SESSION_ACKNOWLEDGED = 2;
    SESSION_ATTACHED = 3;
    SESSION_DETACHED = 4;
  }

  InteractionType type = 3;
  string context = 4;  // Additional context (command text, response, etc)
}

message SessionStateTransitionEvent {
  string session_id = 1;
  string old_state = 2;
  string new_state = 3;
  google.protobuf.Timestamp timestamp = 4;
  string reason = 5;  // Why the transition occurred
}

message QueueStateChangedEvent {
  repeated string session_ids = 1;  // All sessions in queue
  int32 total_items = 2;
  int32 high_priority_count = 3;
  google.protobuf.Timestamp timestamp = 4;
}
```

**Success Criteria**:
- All event types compile and code generates successfully
- Event types include all necessary context for queue evaluation
- Events follow ConnectRPC/Protobuf best practices

**Testing**: Verify protoc code generation succeeds

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 1.2: Implement ReactiveQueueManager (3h)

**Scope**: Create event-driven queue manager that subscribes to interaction events and updates queue state immediately.

**Files**:
- `session/review_queue_reactive.go` (create)
- `session/review_queue.go` (modify)
- `session/review_queue_test.go` (modify)

**Context**:
- Current ReviewQueue is passive (items added/removed by external code)
- ReactiveQueueManager should subscribe to event bus and automatically update queue
- Must be thread-safe for concurrent event processing
- Should deduplicate rapid updates (debouncing)

**Implementation**:
```go
// ReactiveQueueManager listens to events and automatically maintains queue state
type ReactiveQueueManager struct {
    queue         *ReviewQueue
    eventBus      *events.EventBus
    statusChecker *StatusDetector

    // Debouncing
    updateTimer   *time.Timer
    pendingUpdates map[string]bool
    mu            sync.Mutex
}

func NewReactiveQueueManager(queue *ReviewQueue, eventBus *events.EventBus) *ReactiveQueueManager {
    manager := &ReactiveQueueManager{
        queue:          queue,
        eventBus:       eventBus,
        pendingUpdates: make(map[string]bool),
    }

    // Subscribe to relevant events
    eventBus.Subscribe("session.terminal.input", manager.handleTerminalInput)
    eventBus.Subscribe("session.acknowledged", manager.handleAcknowledgement)
    eventBus.Subscribe("session.state.changed", manager.handleStateChange)

    return manager
}

func (m *ReactiveQueueManager) handleTerminalInput(event *UserInteractionEvent) {
    // User typed in terminal - session no longer needs attention
    m.scheduleRemoval(event.SessionID, "user_interaction")
}

func (m *ReactiveQueueManager) handleAcknowledgement(event *UserInteractionEvent) {
    // User acknowledged session - remove from queue
    m.scheduleRemoval(event.SessionID, "acknowledged")
}

func (m *ReactiveQueueManager) scheduleRemoval(sessionID string, reason string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.pendingUpdates[sessionID] = true

    // Debounce: wait 100ms for more updates before processing
    if m.updateTimer != nil {
        m.updateTimer.Stop()
    }

    m.updateTimer = time.AfterFunc(100*time.Millisecond, func() {
        m.processPendingUpdates()
    })
}
```

**Success Criteria**:
- ReactiveQueueManager subscribes to all relevant events
- Queue updates happen within 100ms of events
- No race conditions in concurrent event handling
- Debouncing prevents excessive updates

**Testing**: Unit tests with mock event bus

**Dependencies**: Task 1.1 (Event Types)

**Status**: ⏸️ Pending

---

### Task 1.3: Publish Events in Terminal Handler (2h)

**Scope**: Add event publishing to terminal input handler so user interactions immediately trigger queue updates.

**Files**:
- `session/command_executor.go` (modify)
- `session/pty_access.go` (modify)
- `session/instance.go` (modify)

**Context**:
- Terminal input currently updates terminal state but doesn't publish events
- Need to publish UserInteractionEvent when user types in terminal
- Must distinguish between user input vs. program output
- Event should include command context (last N characters typed)

**Implementation**:
```go
func (i *Instance) handleTerminalInput(data []byte) error {
    // Existing terminal write logic...

    // NEW: Publish event for queue reactivity
    event := &UserInteractionEvent{
        SessionID: i.ID,
        Timestamp: time.Now(),
        Type:      InteractionType_TERMINAL_INPUT,
        Context:   string(data), // Last input for context
    }

    i.eventBus.Publish("session.terminal.input", event)

    return nil
}
```

**Success Criteria**:
- Events published whenever user types in terminal
- Event context includes meaningful input data
- No performance regression from event publishing
- Events only published for user input, not program output

**Testing**: Integration test verifying events are published

**Dependencies**: Task 1.1 (Event Types)

**Status**: ⏸️ Patch

---

### Task 1.4: Add Event Publishing to AcknowledgeSession (1h)

**Scope**: Publish events when sessions are acknowledged so queue immediately reflects the acknowledgment.

**Files**:
- `server/services/session_service.go` (modify)
- `session/instance.go` (modify)

**Context**:
- AcknowledgeSession RPC currently updates session state but doesn't publish events
- Event should trigger immediate removal from queue
- This fixes the "acknowledged sessions linger in queue" issue

**Implementation**:
```go
func (s *SessionService) AcknowledgeSession(ctx context.Context, req *sessionv1.AcknowledgeSessionRequest) (*sessionv1.AcknowledgeSessionResponse, error) {
    // Existing acknowledgment logic...

    // NEW: Publish event for reactive queue
    event := &UserInteractionEvent{
        SessionID: req.SessionId,
        Timestamp: time.Now(),
        Type:      InteractionType_SESSION_ACKNOWLEDGED,
        Context:   fmt.Sprintf("Acknowledged by user at %s", time.Now().Format(time.RFC3339)),
    }

    s.eventBus.Publish("session.acknowledged", event)

    return &sessionv1.AcknowledgeSessionResponse{Success: true}, nil
}
```

**Success Criteria**:
- Event published immediately when session acknowledged
- Event includes relevant context (timestamp, user info if available)
- No errors or crashes when event bus unavailable

**Testing**: Integration test verifying event flow

**Dependencies**: Task 1.1 (Event Types)

**Status**: ⏸️ Pending

---

## Story 2: WebSocket Streaming for Real-Time Updates (1-2 weeks)

**Objective**: Replace HTTP polling with WebSocket streaming so clients receive queue updates pushed from server in real-time.

**Value**: Eliminates network overhead and reduces latency from seconds to milliseconds.

---

### Task 2.1: Define WatchReviewQueue RPC (2h)

**Scope**: Add server-streaming RPC to session service for real-time queue updates.

**Files**:
- `proto/session/v1/session.proto` (modify)
- `proto/session/v1/types.proto` (modify)

**Context**:
- ConnectRPC supports server streaming (similar to gRPC)
- Need to stream ReviewQueueUpdate messages to clients
- Client should receive update whenever queue state changes
- Support filtering (only send updates for specific sessions)

**Implementation**:
```protobuf
service SessionService {
  // Existing RPCs...

  // WatchReviewQueue streams real-time updates when review queue changes.
  // This replaces polling GetReviewQueue with push-based updates.
  rpc WatchReviewQueue(WatchReviewQueueRequest) returns (stream ReviewQueueUpdate);
}

message WatchReviewQueueRequest {
  // Optional: Filter updates to specific sessions
  repeated string session_ids = 1;

  // Optional: Only send updates for certain priorities
  repeated int32 priorities = 2;

  // Whether to send initial snapshot
  bool send_initial_state = 3;
}

message ReviewQueueUpdate {
  // Type of update
  enum UpdateType {
    INITIAL_STATE = 0;      // Complete queue state
    ITEM_ADDED = 1;         // New item added
    ITEM_REMOVED = 2;       // Item removed
    ITEM_UPDATED = 3;       // Item properties changed
    QUEUE_CLEARED = 4;      // All items removed
  }

  UpdateType type = 1;

  // Full queue state (for INITIAL_STATE)
  repeated ReviewItem items = 2;

  // Single item (for ITEM_ADDED/ITEM_UPDATED)
  ReviewItem item = 3;

  // Session ID (for ITEM_REMOVED)
  string session_id = 4;

  google.protobuf.Timestamp timestamp = 5;
}
```

**Success Criteria**:
- RPC definition compiles and generates Go code
- Type definitions support all update scenarios
- Design supports filtering and selective updates

**Testing**: Verify protoc code generation

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 2.2: Implement WatchReviewQueue Server (3h)

**Scope**: Implement server-side streaming handler that watches queue and sends updates to connected clients.

**Files**:
- `server/services/session_service.go` (modify)
- `server/services/review_queue_stream.go` (create)

**Context**:
- Must maintain active connections to multiple clients
- Each client may have different filters
- Should handle client disconnections gracefully
- Need to batch updates to avoid overwhelming clients

**Implementation**:
```go
func (s *SessionService) WatchReviewQueue(
    ctx context.Context,
    req *sessionv1.WatchReviewQueueRequest,
    stream *connect.ServerStream[sessionv1.ReviewQueueUpdate],
) error {
    // Send initial state if requested
    if req.SendInitialState {
        items := s.reviewQueue.GetItems()
        update := &sessionv1.ReviewQueueUpdate{
            Type:  sessionv1.ReviewQueueUpdate_INITIAL_STATE,
            Items: items,
        }
        if err := stream.Send(update); err != nil {
            return err
        }
    }

    // Subscribe to queue changes
    updateChan := make(chan *sessionv1.ReviewQueueUpdate, 10)

    // Register observer for queue updates
    observer := &streamObserver{
        stream:     stream,
        updateChan: updateChan,
        filter:     req,
    }
    s.reviewQueue.AddObserver(observer)
    defer s.reviewQueue.RemoveObserver(observer)

    // Stream updates until client disconnects or context cancels
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case update := <-updateChan:
            if err := stream.Send(update); err != nil {
                return err
            }
        }
    }
}
```

**Success Criteria**:
- Multiple clients can connect simultaneously
- Updates streamed in real-time (<100ms latency)
- Filters applied correctly (only relevant updates sent)
- Clean disconnection handling

**Testing**: Integration test with multiple clients

**Dependencies**: Task 2.1 (RPC Definition)

**Status**: ⏸️ Pending

---

### Task 2.3: Integrate Queue Observer Pattern (3h)

**Scope**: Refactor ReviewQueue to notify observers when state changes so streaming can push updates.

**Files**:
- `session/review_queue.go` (modify)
- `session/review_queue_test.go` (modify)

**Context**:
- ReviewQueue currently has observer pattern but it's not fully utilized
- Need to ensure all state changes trigger observer notifications
- Observers should receive rich context (what changed, why)
- Must be thread-safe for concurrent observers

**Implementation**:
```go
type ReviewQueueObserver interface {
    OnItemAdded(item *ReviewItem)
    OnItemRemoved(sessionID string)
    OnItemUpdated(item *ReviewItem)
    OnQueueCleared()
}

// Ensure all state changes notify observers
func (rq *ReviewQueue) Add(item *ReviewItem) bool {
    rq.mu.Lock()
    defer rq.mu.Unlock()

    isNew := true
    if existing, found := rq.items[item.SessionID]; found {
        isNew = false
        // Notify update
        for _, observer := range rq.observers {
            observer.OnItemUpdated(item)
        }
    } else {
        // Notify addition
        for _, observer := range rq.observers {
            observer.OnItemAdded(item)
        }
    }

    rq.items[item.SessionID] = item
    return isNew
}
```

**Success Criteria**:
- All state changes trigger appropriate observer notifications
- Observers receive complete context for updates
- Thread-safe notification (no deadlocks)
- Observer errors don't crash queue

**Testing**: Unit tests with mock observers

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 2.4: Implement Frontend WebSocket Client (4h)

**Scope**: Replace polling HTTP client with WebSocket streaming client in React frontend.

**Files**:
- `web-app/src/lib/api/review-queue-stream.ts` (create)
- `web-app/src/lib/hooks/useReviewQueueStream.ts` (create)
- `web-app/src/components/ReviewQueue.tsx` (modify)

**Context**:
- Currently using HTTP polling with setInterval
- Need to use ConnectRPC's streaming support for React
- Should handle reconnection on disconnect
- Must apply optimistic updates for smooth UI

**Implementation**:
```typescript
import { createConnectTransport } from "@connectrpc/connect-web";
import { ReviewQueueService } from "@/proto/session_connect";

export class ReviewQueueStream {
  private stream: AsyncIterable<ReviewQueueUpdate> | null = null;
  private onUpdate: (update: ReviewQueueUpdate) => void;

  constructor(onUpdate: (update: ReviewQueueUpdate) => void) {
    this.onUpdate = onUpdate;
  }

  async connect() {
    const transport = createConnectTransport({
      baseUrl: process.env.NEXT_PUBLIC_API_URL,
    });

    const client = createPromiseClient(ReviewQueueService, transport);

    // Start streaming
    this.stream = client.watchReviewQueue({
      sendInitialState: true,
    });

    // Process updates
    for await (const update of this.stream) {
      this.onUpdate(update);
    }
  }

  disconnect() {
    // ConnectRPC handles cleanup when iteration stops
    this.stream = null;
  }
}

// React hook for easy integration
export function useReviewQueueStream() {
  const [items, setItems] = useState<ReviewItem[]>([]);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    const stream = new ReviewQueueStream((update) => {
      // Apply update to local state
      switch (update.type) {
        case ReviewQueueUpdate_Type.INITIAL_STATE:
          setItems(update.items);
          break;
        case ReviewQueueUpdate_Type.ITEM_ADDED:
          setItems(prev => [...prev, update.item]);
          break;
        case ReviewQueueUpdate_Type.ITEM_REMOVED:
          setItems(prev => prev.filter(i => i.sessionId !== update.sessionId));
          break;
        // ... other cases
      }
    });

    stream.connect()
      .then(() => setIsConnected(true))
      .catch(console.error);

    return () => stream.disconnect();
  }, []);

  return { items, isConnected };
}
```

**Success Criteria**:
- WebSocket connection established successfully
- Updates applied to UI within 100ms
- Reconnection works on disconnect
- No memory leaks from unclosed connections

**Testing**: Manual testing + integration tests

**Dependencies**: Task 2.2 (Server Implementation)

**Status**: ⏸️ Pending

---

## Story 3: Queue Navigation and UI Enhancements (1 week)

**Objective**: Add keyboard navigation, optimistic UI updates, and visual improvements to make queue management efficient and delightful.

**Value**: Users can navigate and interact with queue items without touching mouse.

---

### Task 3.1: Add Queue Navigation Keyboard Shortcuts (3h)

**Scope**: Implement [ and ] keys for navigating between queue items in web UI.

**Files**:
- `web-app/src/components/ReviewQueue.tsx` (modify)
- `web-app/src/lib/hooks/useKeyboardNav.ts` (create)

**Context**:
- Current UI has no keyboard navigation for queue
- [ and ] keys should move between items like Vim-style navigation
- Should work when queue panel has focus
- Need visual indication of selected item

**Implementation**:
```typescript
export function useQueueKeyboardNav(items: ReviewItem[]) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      if (e.key === '[') {
        // Previous item
        setSelectedIndex(prev => Math.max(0, prev - 1));
        e.preventDefault();
      } else if (e.key === ']') {
        // Next item
        setSelectedIndex(prev => Math.min(items.length - 1, prev + 1));
        e.preventDefault();
      } else if (e.key === 'Enter') {
        // Attach to selected session
        const item = items[selectedIndex];
        if (item) {
          attachToSession(item.sessionId);
        }
        e.preventDefault();
      }
    };

    window.addEventListener('keydown', handleKeyPress);
    return () => window.removeEventListener('keydown', handleKeyPress);
  }, [items, selectedIndex]);

  return { selectedIndex, selectedItem: items[selectedIndex] };
}
```

**Success Criteria**:
- [ and ] keys navigate through queue items
- Enter key attaches to selected session
- Visual feedback shows selected item
- Works with keyboard focus management

**Testing**: Manual testing + Playwright tests

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 3.2: Implement Optimistic UI Updates (3h)

**Scope**: Add optimistic updates so UI reflects user actions immediately before server confirmation.

**Files**:
- `web-app/src/lib/hooks/useOptimisticQueue.ts` (create)
- `web-app/src/components/ReviewQueue.tsx` (modify)

**Context**:
- Current UI waits for server response before updating
- Optimistic updates make UI feel instant
- Need rollback mechanism if server rejects action
- Should show loading state during server confirmation

**Implementation**:
```typescript
export function useOptimisticQueue() {
  const [items, setItems] = useState<ReviewItem[]>([]);
  const [pendingActions, setPendingActions] = useState<Map<string, PendingAction>>(new Map());

  const acknowledgeSession = async (sessionId: string) => {
    // Optimistic update - remove immediately
    setItems(prev => prev.filter(item => item.sessionId !== sessionId));

    // Track pending action
    setPendingActions(prev => new Map(prev).set(sessionId, {
      type: 'acknowledge',
      timestamp: Date.now(),
    }));

    try {
      // Send to server
      await sessionClient.acknowledgeSession({ sessionId });

      // Success - clear pending
      setPendingActions(prev => {
        const next = new Map(prev);
        next.delete(sessionId);
        return next;
      });
    } catch (error) {
      // Rollback on error
      console.error('Failed to acknowledge:', error);

      // Re-fetch current state to restore consistency
      const current = await sessionClient.getReviewQueue({});
      setItems(current.items);

      setPendingActions(prev => {
        const next = new Map(prev);
        next.delete(sessionId);
        return next;
      });
    }
  };

  return { items, pendingActions, acknowledgeSession };
}
```

**Success Criteria**:
- UI updates immediately on user action
- Actions roll back gracefully on server errors
- Pending state visible to user (e.g., opacity, spinner)
- No flashing between optimistic and confirmed state

**Testing**: Manual testing with network throttling

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 3.3: Add Visual Queue State Indicators (2h)

**Scope**: Enhance UI with visual indicators for queue state (loading, error, empty, disconnected).

**Files**:
- `web-app/src/components/ReviewQueue.tsx` (modify)
- `web-app/src/styles/review-queue.css` (modify)

**Context**:
- Current UI doesn't clearly show connection state
- Need indicators for: WebSocket connected/disconnected, loading, empty queue, errors
- Should match existing Stapler Squad design language

**Implementation**:
```tsx
export function ReviewQueue() {
  const { items, isConnected } = useReviewQueueStream();

  return (
    <div className="review-queue">
      <header className="queue-header">
        <h2>Review Queue</h2>
        <ConnectionIndicator connected={isConnected} />
      </header>

      {!isConnected && (
        <div className="queue-disconnected">
          <Icon name="wifi-off" />
          <p>Reconnecting to server...</p>
        </div>
      )}

      {isConnected && items.length === 0 && (
        <div className="queue-empty">
          <Icon name="check-circle" />
          <p>All caught up! No sessions need attention.</p>
        </div>
      )}

      {isConnected && items.length > 0 && (
        <QueueItemList items={items} />
      )}
    </div>
  );
}
```

**Success Criteria**:
- Connection state visible at all times
- Empty state is friendly and informative
- Error states clearly communicate issue
- All states match design system

**Testing**: Manual testing + visual regression tests

**Dependencies**: None

**Status**: ⏸️ Pending

---

## Story 4: Architecture Fixes and Technical Debt (1 week)

**Objective**: Fix architectural issues discovered during investigation (duplicate queue creation, missing event publishing, etc.).

**Value**: Ensures system is built on solid foundation for future features.

---

### Task 4.1: Remove Duplicate GetReviewQueue Logic (2h)

**Scope**: Fix the code duplication in GetReviewQueue where queue items are constructed twice.

**Files**:
- `server/services/session_service.go` (modify)

**Context**:
- Lines 699-875 in session_service.go contain duplicated queue construction logic
- First construction is for gRPC response, second is for internal state
- This causes inconsistencies and wastes CPU
- Should use single source of truth (ReviewQueue)

**Implementation**:
```go
func (s *SessionService) GetReviewQueue(
    ctx context.Context,
    req *sessionv1.GetReviewQueueRequest,
) (*sessionv1.GetReviewQueueResponse, error) {
    // Get items from ReviewQueue (single source of truth)
    items := s.reviewQueue.GetItems()

    // Apply filters from request
    filtered := s.applyFilters(items, req)

    // Convert to protobuf
    pbItems := make([]*sessionv1.ReviewItem, len(filtered))
    for i, item := range filtered {
        pbItems[i] = s.reviewItemToProto(item)
    }

    return &sessionv1.GetReviewQueueResponse{
        Items:      pbItems,
        TotalCount: int32(len(filtered)),
    }, nil
}

func (s *SessionService) applyFilters(items []*session.ReviewItem, req *sessionv1.GetReviewQueueRequest) []*session.ReviewItem {
    // Apply filtering logic (priority, status, etc)
    // ...
}
```

**Success Criteria**:
- Single queue construction logic
- No code duplication between APIs
- Consistent state between gRPC and internal queue
- No performance regression

**Testing**: Integration tests verifying consistency

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 4.2: Add Event Bus Injection to Session Instances (2h)

**Scope**: Ensure all Session instances have access to event bus for publishing events.

**Files**:
- `session/instance.go` (modify)
- `session/storage.go` (modify)
- `server/services/session_service.go` (modify)

**Context**:
- Some Session instances don't have event bus reference
- Event publishing fails silently in these cases
- Need to inject event bus during instance creation
- Should validate event bus is available

**Implementation**:
```go
type Instance struct {
    // Existing fields...
    eventBus *events.EventBus
}

func NewInstance(config InstanceConfig, eventBus *events.EventBus) (*Instance, error) {
    if eventBus == nil {
        return nil, errors.New("event bus is required")
    }

    return &Instance{
        ID:       config.ID,
        // ... other fields ...
        eventBus: eventBus,
    }, nil
}

// Ensure Storage passes event bus to instances
func (s *Storage) CreateSession(config SessionConfig, eventBus *events.EventBus) (*Instance, error) {
    instance, err := NewInstance(config, eventBus)
    if err != nil {
        return nil, err
    }

    // Store instance...
    return instance, nil
}
```

**Success Criteria**:
- All instances have event bus reference
- Event publishing never silently fails
- Errors logged if event bus unavailable
- Tests validate event bus injection

**Testing**: Unit tests with mock event bus

**Dependencies**: None

**Status**: ⏸️ Pending

---

### Task 4.3: Add Event Publishing to Session State Transitions (2h)

**Scope**: Ensure all session state changes publish events for queue reactivity.

**Files**:
- `session/instance.go` (modify)
- `session/storage.go` (modify)

**Context**:
- Some state changes (pause, resume, stop) don't publish events
- Queue can't react to these changes
- Need consistent event publishing for all state transitions
- Events should include reason for transition

**Implementation**:
```go
func (i *Instance) Pause() error {
    oldState := i.Status

    // Existing pause logic...

    // Publish state transition event
    i.eventBus.Publish("session.state.changed", &SessionStateTransitionEvent{
        SessionID: i.ID,
        OldState:  oldState,
        NewState:  i.Status,
        Timestamp: time.Now(),
        Reason:    "user_pause",
    })

    return nil
}

func (i *Instance) Resume() error {
    oldState := i.Status

    // Existing resume logic...

    i.eventBus.Publish("session.state.changed", &SessionStateTransitionEvent{
        SessionID: i.ID,
        OldState:  oldState,
        NewState:  i.Status,
        Timestamp: time.Now(),
        Reason:    "user_resume",
    })

    return nil
}
```

**Success Criteria**:
- All state transitions publish events
- Events include context (old state, new state, reason)
- Event publishing doesn't block critical path
- Failed event publishing logged but doesn't crash

**Testing**: Integration tests verifying event flow

**Dependencies**: Task 1.1 (Event Types)

**Status**: ⏸️ Pending

---

### Task 4.4: Add Event Subscription Filters (3h)

**Scope**: Implement filtering in event bus so subscribers only receive relevant events.

**Files**:
- `events/bus.go` (modify)
- `events/filters.go` (create)
- `events/bus_test.go` (modify)

**Context**:
- Current event bus sends all events to all subscribers (chatty)
- Subscribers should be able to filter by session ID, event type, etc.
- Reduces network traffic and CPU for filtering
- Enables efficient multi-tenant scenarios

**Implementation**:
```go
type EventFilter interface {
    Matches(event Event) bool
}

type SessionIDFilter struct {
    SessionIDs []string
}

func (f *SessionIDFilter) Matches(event Event) bool {
    sessionID := event.GetSessionID()
    for _, id := range f.SessionIDs {
        if id == sessionID {
            return true
        }
    }
    return false
}

type EventTypeFilter struct {
    Types []string
}

func (f *EventTypeFilter) Matches(event Event) bool {
    eventType := event.GetType()
    for _, t := range f.Types {
        if t == eventType {
            return true
        }
    }
    return false
}

// Subscription with filters
func (b *EventBus) SubscribeWithFilter(topic string, filter EventFilter, handler EventHandler) *Subscription {
    // Subscribe to topic
    sub := b.Subscribe(topic, handler)

    // Store filter on subscription
    sub.filter = filter

    return sub
}

// Event delivery checks filters
func (b *EventBus) Publish(topic string, event Event) {
    subscribers := b.getSubscribers(topic)

    for _, sub := range subscribers {
        // Apply filter if present
        if sub.filter != nil && !sub.filter.Matches(event) {
            continue
        }

        // Deliver event
        sub.handler(event)
    }
}
```

**Success Criteria**:
- Filters work for session ID, event type, custom predicates
- Filtering happens server-side (not client)
- No events sent to client that don't match filters
- Performance improvement measurable (network traffic reduced)

**Testing**: Unit tests with various filter combinations

**Dependencies**: None

**Status**: ⏸️ Pending

---

## Story 5: Advanced Queue Features (Future)

**Objective**: Implement smart features that leverage the reactive architecture (priority escalation, auto-resolution, context enrichment).

**Note**: These tasks are intentionally vague as they're for future work. They'll be expanded into proper AIC tasks when prioritized.

---

### Task 5.1: Implement Priority Escalation (4h)

**Scope**: Automatically increase priority of items that remain unresolved for too long.

**Files**:
- `session/review_queue_reactive.go` (modify)
- `session/priority_escalator.go` (create)

**Context**:
- Items stuck in queue for >30min should escalate to higher priority
- Prevents important items from being forgotten
- Configurable escalation rules

**Status**: ⏸️ Future

---

### Task 5.2: Implement Context Enrichment (4h)

**Scope**: Automatically attach relevant context to queue items (recent commands, error logs, resource usage).

**Files**:
- `session/review_queue_reactive.go` (modify)
- `session/context_enricher.go` (create)

**Context**:
- Items should include recent commands, error snippets, resource usage
- Helps user quickly understand what's wrong
- Requires integration with command history, error detection

**Status**: ⏸️ Future

---

### Task 5.3: Implement Auto-Resolution (4h)

**Scope**: Automatically remove items from queue when conditions resolve (e.g., error clears, command completes).

**Files**:
- `session/review_queue_reactive.go` (modify)
- `session/auto_resolver.go` (create)

**Context**:
- Some conditions resolve automatically (transient errors, etc)
- Should auto-remove from queue to reduce noise
- Configurable resolution rules

**Status**: ⏸️ Future

---

### Task 5.4: Implement Queue Analytics (3h)

**Scope**: Track queue metrics (time to resolution, false positive rate, etc.) for continuous improvement.

**Files**:
- `session/review_queue_analytics.go` (create)
- `server/services/analytics_service.go` (create)

**Context**:
- Track how long items stay in queue
- Measure false positive rate (items that don't need attention)
- Feed data into ML for better detection

**Status**: ⏸️ Future

---

## Dependency Graph

```
Story 1: Event-Driven Architecture (Foundation)
├─ Task 1.1: Event Types (MUST complete first)
├─ Task 1.2: ReactiveQueueManager (depends on 1.1)
├─ Task 1.3: Terminal Events (depends on 1.1)
└─ Task 1.4: Acknowledgment Events (depends on 1.1)

Story 2: WebSocket Streaming (Parallel after 1.2)
├─ Task 2.1: RPC Definition (can start early)
├─ Task 2.2: Server Implementation (depends on 2.1, 1.2)
├─ Task 2.3: Observer Pattern (parallel with 2.2)
└─ Task 2.4: Frontend Client (depends on 2.2, 2.3)

Story 3: UI Enhancements (Parallel with Story 2)
├─ Task 3.1: Keyboard Nav (parallel)
├─ Task 3.2: Optimistic Updates (parallel)
└─ Task 3.3: Visual Indicators (parallel)

Story 4: Architecture Fixes (Can start anytime, should finish before Story 5)
├─ Task 4.1: Remove Duplication (parallel)
├─ Task 4.2: Event Bus Injection (parallel)
├─ Task 4.3: State Event Publishing (depends on 1.1)
└─ Task 4.4: Event Filters (parallel with 2.2)

Story 5: Advanced Features (Future - depends on 1-4 complete)
├─ Task 5.1: Priority Escalation
├─ Task 5.2: Context Enrichment
├─ Task 5.3: Auto-Resolution
└─ Task 5.4: Analytics
```

---

## Development Workflow Recommendations

### Phase 1: Foundation (Week 1)
**Goal**: Get event-driven architecture working

1. Start with Task 1.1 (Event Types) - foundational for everything
2. Implement Task 1.2 (ReactiveQueueManager) - core logic
3. Add Task 1.3 & 1.4 (Event Publishing) - complete the loop
4. Start Task 4.2 (Event Bus Injection) in parallel - fixes infrastructure

**Milestone**: Events flow from user actions to queue updates

### Phase 2: Streaming (Week 2)
**Goal**: Replace polling with WebSocket streaming

1. Task 2.1 (RPC Definition) - sets contract
2. Task 2.3 (Observer Pattern) - enables streaming
3. Task 2.2 (Server Implementation) - implement streaming
4. Task 2.4 (Frontend Client) - consume streaming

**Milestone**: Frontend receives real-time queue updates

### Phase 3: Polish (Week 3)
**Goal**: Make UI delightful and fix technical debt

1. Task 3.1 (Keyboard Nav) - navigation shortcuts
2. Task 3.2 (Optimistic Updates) - instant feedback
3. Task 3.3 (Visual Indicators) - clear state
4. Task 4.1 (Remove Duplication) - clean up code
5. Task 4.3 & 4.4 (Event Improvements) - finishing touches

**Milestone**: Production-ready queue system with great UX

### Phase 4: Advanced Features (Future)
**Goal**: Add smart features that leverage reactive foundation

1. Task 5.1-5.4 (Advanced Features) - expand as needed

**Milestone**: Intelligent queue that anticipates user needs

---

## Testing Strategy

### Unit Tests
- Event types and serialization
- ReactiveQueueManager logic
- Event filtering
- Queue observer notifications

### Integration Tests
- Event flow (user action → event → queue update)
- WebSocket streaming (server → client)
- Multi-client scenarios
- Event bus under load

### End-to-End Tests
- User types in terminal → queue updates
- User acknowledges session → item removed
- Network disconnect → reconnect
- Keyboard navigation works

### Performance Tests
- Event publishing overhead (<1ms)
- Queue update latency (<100ms end-to-end)
- WebSocket throughput (1000+ updates/sec)
- Memory usage with many connections

---

## Metrics and Success Criteria

### Performance Metrics
- **Queue Update Latency**: <100ms from user action to UI update (currently 7-32 seconds)
- **Network Traffic**: 90% reduction vs polling (measure bytes/sec)
- **Event Overhead**: <1ms CPU per event published
- **WebSocket Throughput**: Support 1000+ concurrent connections

### Quality Metrics
- **Zero Flickering**: No visual inconsistencies in queue
- **Zero False Positives**: <1% items in queue that don't need attention
- **High Availability**: WebSocket reconnect <1 second on disconnect
- **Test Coverage**: >80% coverage on new code

### User Experience Metrics
- **Time to Action**: <2 seconds from queue item appearing to user taking action
- **Navigation Speed**: <100ms between [ and ] key presses
- **Perceived Performance**: Optimistic updates make actions feel instant

---

## Risk Management

### Technical Risks

**Risk**: WebSocket connections don't scale to many clients
- **Mitigation**: Load test early, implement connection limits
- **Fallback**: Keep polling as backup mode

**Risk**: Event flooding overwhelms system
- **Mitigation**: Event batching, rate limiting, filters
- **Fallback**: Circuit breaker to disable events temporarily

**Risk**: Frontend state gets out of sync with server
- **Mitigation**: Periodic reconciliation, version numbers on updates
- **Fallback**: Full refresh button for users

### Timeline Risks

**Risk**: Streaming implementation takes longer than expected
- **Mitigation**: Start with polling to ReactiveQueueManager, add streaming later
- **Fallback**: Ship with polling + ReactiveQueueManager (still 10x improvement)

**Risk**: Frontend work blocks backend progress
- **Mitigation**: Implement backend with curl/wscat testing first
- **Fallback**: Ship backend with CLI client, web UI later

---

## Future Enhancements (Not in Scope)

These features are explicitly out of scope for this epic but documented for future reference:

1. **Multi-User Coordination** - Multiple users collaborating on same sessions
2. **Machine Learning** - Learn from user actions to improve detection
3. **Mobile Support** - Native mobile apps for queue management
4. **Voice Notifications** - Speak alerts for critical items
5. **Slack/Discord Integration** - Notify external channels
6. **Queue Analytics Dashboard** - Visualize queue metrics over time
7. **Custom Detection Rules** - User-defined patterns for queue items

---

## Appendix: Key Files Reference

### Backend (Go)
- `session/review_queue.go` - Core queue data structure
- `session/review_queue_reactive.go` - NEW: Event-driven queue manager
- `session/instance.go` - Session lifecycle and events
- `server/services/session_service.go` - gRPC service implementation
- `proto/session/v1/session.proto` - API definitions
- `proto/session/v1/events.proto` - Event type definitions
- `events/bus.go` - Event bus implementation

### Frontend (React/TypeScript)
- `web-app/src/lib/api/review-queue-stream.ts` - NEW: WebSocket client
- `web-app/src/lib/hooks/useReviewQueueStream.ts` - NEW: React hook
- `web-app/src/components/ReviewQueue.tsx` - Queue UI component
- `web-app/src/lib/hooks/useOptimisticQueue.ts` - NEW: Optimistic updates

### Tests
- `session/review_queue_test.go` - Queue unit tests
- `server/services/session_service_test.go` - Service integration tests
- `web-app/src/components/ReviewQueue.test.tsx` - Frontend tests

---

## Appendix: ConnectRPC Streaming Reference

### Server Streaming Example (Go)
```go
func (s *SessionService) WatchReviewQueue(
    ctx context.Context,
    req *sessionv1.WatchReviewQueueRequest,
    stream *connect.ServerStream[sessionv1.ReviewQueueUpdate],
) error {
    // Send initial state
    if req.SendInitialState {
        if err := stream.Send(initialState); err != nil {
            return err
        }
    }

    // Stream updates until client disconnects
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case update := <-updateChan:
            if err := stream.Send(update); err != nil {
                return err
            }
        }
    }
}
```

### Client Streaming Example (TypeScript)
```typescript
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/proto/session_connect";

async function watchQueue() {
  const client = createPromiseClient(SessionService, transport);

  // Start streaming
  const stream = client.watchReviewQueue({
    sendInitialState: true,
  });

  // Process updates
  for await (const update of stream) {
    console.log("Queue update:", update);
  }
}
```

---

## Document Metadata

- **Created**: 2025-01-20
- **Author**: Claude (Project Coordinator)
- **Epic Status**: Planning
- **Estimated Duration**: 3-4 weeks for Stories 1-4
- **Priority**: High (fixes critical UX issues)
- **Dependencies**: None (self-contained)

---

## Change Log

| Date | Change | Reason |
|------|--------|--------|
| 2025-01-20 | Initial document | Epic kickoff |

