# Review Queue Feature - AIC Task Breakdown

## Epic: Session Review Queue System

**Goal**: Implement a review queue system that helps users identify and prioritize sessions requiring their attention (pending approval dialogs, idle sessions, error states, completion signals).

**Value Proposition**: Users can quickly identify which Stapler Squad sessions need immediate attention, reducing context switching overhead and preventing important tasks from being overlooked.

**Success Metrics**:
- Users can identify all sessions needing attention within 2 seconds
- Sessions are prioritized by urgency/importance
- Integrates seamlessly with existing filtering/navigation
- Zero false positives in attention detection

**Technical Scope**:
- Extend existing status detection infrastructure
- Add new UI components for queue view
- Integrate with session monitoring system
- Implement priority scoring algorithm

---

## Story 1: Status Detection Enhancement (1-2 weeks)

**Objective**: Enhance the existing status detection system to identify sessions requiring user attention with high accuracy.

**Value**: Provides the foundation for identifying which sessions need review.

### Task 1.1: Extend DetectedStatus Enum (2h)

**Scope**: Add new status types for attention states

**Files**:
- session/status_detector.go (modify)
- session/status_detector_test.go (modify)

**Context**:
- Current statuses: Unknown, Ready, Processing, NeedsApproval, Error
- Need to add: NeedsResponse, Idle, Completed, Stalled

**Implementation**:
```go
const (
    StatusUnknown DetectedStatus = iota
    StatusReady
    StatusProcessing
    StatusNeedsApproval
    StatusError
    StatusNeedsResponse    // NEW: Waiting for user input
    StatusIdle            // NEW: No activity for threshold
    StatusCompleted       // NEW: Task finished successfully
    StatusStalled         // NEW: Processing stuck/timeout
)
```

**Success Criteria**:
- New status enums defined
- String() method updated for display
- All switch statements handle new cases

**Testing**: Unit tests for status string conversion

**Dependencies**: None

**Status**: ⏳ Pending

### Task 1.2: Create Attention Pattern Definitions (3h)

**Scope**: Define regex patterns for detecting attention-needed states

**Files**:
- session/status_patterns.yaml (modify)
- session/attention_patterns.yaml (create)
- session/status_detector.go (modify)

**Context**:
- Leverage existing YAML pattern loading system
- Need patterns for: approval dialogs, completion messages, error states, idle detection
- Consider Claude-specific output patterns

**Implementation**:
```yaml
needs_response:
  - name: "approval_dialog"
    pattern: "Would you like to (proceed|continue|approve)"
    priority: 10
  - name: "confirmation_prompt"
    pattern: "Please confirm|Are you sure"
    priority: 9

completed:
  - name: "task_complete"
    pattern: "Task completed successfully|Done\\.|Finished"
    priority: 5
```

**Success Criteria**:
- Comprehensive pattern coverage for Claude outputs
- Patterns tested against real session outputs
- Priority ordering established

**Testing**: Pattern matching tests with sample outputs

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

### Task 1.3: Implement Idle Detection Logic (3h)

**Scope**: Detect when sessions have been idle beyond threshold

**Files**:
- session/idle_detector.go (create)
- session/idle_detector_test.go (create)
- session/instance.go (modify)

**Context**:
- Track last activity timestamp
- Configurable idle threshold (default 5 minutes)
- Consider both PTY output and user input

**Implementation**:
```go
type IdleDetector struct {
    threshold     time.Duration
    lastActivity  time.Time
    activityChan  chan time.Time
}

func (d *IdleDetector) IsIdle() bool {
    return time.Since(d.lastActivity) > d.threshold
}
```

**Success Criteria**:
- Accurate idle time tracking
- Configurable thresholds
- Integration with Instance struct

**Testing**: Time-based unit tests with mock clock

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

### Task 1.4: Priority Scoring Algorithm (4h)

**Scope**: Calculate priority scores for attention-needed sessions

**Files**:
- session/priority_scorer.go (create)
- session/priority_scorer_test.go (create)
- session/instance.go (modify)

**Context**:
- Score based on: status type, idle time, age, user-defined priority
- Higher scores = more urgent
- Configurable weight factors

**Implementation**:
```go
type PriorityScorer struct {
    statusWeights map[DetectedStatus]int
    idleWeight    float64
    ageWeight     float64
}

func (s *PriorityScorer) CalculateScore(instance *Instance) int {
    score := s.statusWeights[instance.CurrentStatus]
    score += int(instance.IdleTime().Minutes() * s.idleWeight)
    return score
}
```

**Success Criteria**:
- Consistent scoring across sessions
- Configurable weights
- Real-time score updates

**Testing**: Unit tests with various session states

**Dependencies**: Tasks 1.1, 1.3

**Status**: ⏳ Pending

---

## Story 2: Review Queue Data Model (1 week)

**Objective**: Create the core data structures and business logic for managing the review queue.

**Value**: Provides the foundation for queue operations and state management.

### Task 2.1: Define Queue Entry Model (2h)

**Scope**: Create data structure for queue entries

**Files**:
- session/review_queue.go (create)
- session/review_queue_test.go (create)

**Context**:
- Each entry represents a session needing attention
- Include metadata for display and sorting
- Immutable design for thread safety

**Implementation**:
```go
type ReviewQueueEntry struct {
    SessionID      string
    Title          string
    Status         DetectedStatus
    Priority       int
    IdleTime       time.Duration
    LastActivity   time.Time
    AttentionReason string
    Context        string // Last few lines of output
}
```

**Success Criteria**:
- Complete data model defined
- JSON serialization support
- Comparable for sorting

**Testing**: Unit tests for creation and serialization

**Dependencies**: Story 1 completion

**Status**: ⏳ Pending

### Task 2.2: Implement Queue Manager (4h)

**Scope**: Central manager for review queue operations

**Files**:
- session/queue_manager.go (create)
- session/queue_manager_test.go (create)

**Context**:
- Singleton pattern for global queue access
- Thread-safe operations
- Observable for UI updates

**Implementation**:
```go
type QueueManager struct {
    mu         sync.RWMutex
    entries    []ReviewQueueEntry
    observers  []QueueObserver
    scorer     *PriorityScorer
}

func (m *QueueManager) GetQueue() []ReviewQueueEntry
func (m *QueueManager) UpdateEntry(sessionID string, status DetectedStatus)
func (m *QueueManager) Subscribe(observer QueueObserver)
```

**Success Criteria**:
- Thread-safe queue operations
- Automatic sorting by priority
- Observer pattern for updates

**Testing**: Concurrent operation tests

**Dependencies**: Task 2.1

**Status**: ⏳ Pending

### Task 2.3: Session Monitor Integration (3h)

**Scope**: Integrate queue updates with session monitoring

**Files**:
- session/instance.go (modify)
- session/session_monitor.go (create)
- app/app.go (modify)

**Context**:
- Monitor all active sessions for status changes
- Update queue when attention needed
- Periodic status checks

**Implementation**:
```go
type SessionMonitor struct {
    instances    []*Instance
    queueManager *QueueManager
    ticker       *time.Ticker
}

func (m *SessionMonitor) Start()
func (m *SessionMonitor) checkSession(instance *Instance)
```

**Success Criteria**:
- Real-time status monitoring
- Queue updates within 2 seconds
- Minimal performance impact

**Testing**: Integration tests with mock sessions

**Dependencies**: Task 2.2

**Status**: ⏳ Pending

---

## Story 3: Review Queue UI Components (1-2 weeks)

**Objective**: Implement the user interface for the review queue, including dedicated view and navigation.

**Value**: Users can visualize and interact with the review queue efficiently.

### Task 3.1: Create Queue View Mode (3h)

**Scope**: Add new view mode for displaying review queue

**Files**:
- ui/queue_view.go (create)
- ui/queue_view_test.go (create)
- app/app.go (modify)

**Context**:
- Similar to existing list view but queue-focused
- Toggle between normal and queue view
- Preserve navigation state

**Implementation**:
```go
type QueueView struct {
    entries      []ReviewQueueEntry
    selected     int
    scrollOffset int
    width, height int
}

func (v *QueueView) View() string
func (v *QueueView) HandleKey(key tea.KeyMsg)
```

**Success Criteria**:
- Clean queue display
- Smooth navigation
- Visual priority indicators

**Testing**: UI rendering tests

**Dependencies**: Story 2 completion

**Status**: ⏳ Pending

### Task 3.2: Add Queue Toggle Key Binding (2h)

**Scope**: Add keyboard shortcut to toggle queue view

**Files**:
- app/app.go (modify)
- ui/menu.go (modify)

**Context**:
- Use 'r' key for "Review queue"
- Update help system
- Add to bottom menu

**Implementation**:
```go
case "r":
    m.toggleReviewQueue()
    m.menu.SetContext("Review Queue")
```

**Success Criteria**:
- Instant toggle response
- Menu reflects current mode
- Help system updated

**Testing**: Key handler tests

**Dependencies**: Task 3.1

**Status**: ⏳ Pending

### Task 3.3: Queue Entry Rendering (3h)

**Scope**: Create rich display for queue entries

**Files**:
- ui/queue_renderer.go (create)
- ui/queue_renderer_test.go (create)

**Context**:
- Show: title, status, idle time, reason
- Color coding by priority
- Truncate long context appropriately

**Implementation**:
```go
func RenderQueueEntry(entry ReviewQueueEntry, width int, selected bool) string {
    // Priority-based styling
    style := getPriorityStyle(entry.Priority)

    // Format entry with status icon, title, time
    line := fmt.Sprintf("%s %s (%s)",
        getStatusIcon(entry.Status),
        truncate(entry.Title, width-20),
        formatIdleTime(entry.IdleTime))

    return style.Render(line)
}
```

**Success Criteria**:
- Clear visual hierarchy
- Responsive to terminal width
- Accessible color scheme

**Testing**: Rendering output tests

**Dependencies**: Task 3.1

**Status**: ⏳ Pending

### Task 3.4: Queue Status Indicator (2h)

**Scope**: Add queue count indicator to main view

**Files**:
- ui/list.go (modify)
- ui/status_bar.go (create)

**Context**:
- Show count of sessions needing attention
- Update in real-time
- Non-intrusive placement

**Implementation**:
```go
func (l *List) renderStatusBar() string {
    queueCount := l.queueManager.GetQueueCount()
    if queueCount > 0 {
        return fmt.Sprintf("📥 %d sessions need attention", queueCount)
    }
    return ""
}
```

**Success Criteria**:
- Always visible indicator
- Real-time updates
- Clear but not distracting

**Testing**: Status bar rendering tests

**Dependencies**: Task 3.1

**Status**: ⏳ Pending

---

## Story 4: Queue Filtering and Actions (1 week)

**Objective**: Implement advanced queue management features including filtering, sorting, and bulk actions.

**Value**: Power users can efficiently manage multiple sessions requiring attention.

### Task 4.1: Queue Filtering Options (3h)

**Scope**: Add filters for queue view

**Files**:
- ui/queue_filter.go (create)
- ui/queue_view.go (modify)

**Context**:
- Filter by: status type, priority threshold, age
- Combine multiple filters
- Persistent filter preferences

**Implementation**:
```go
type QueueFilter struct {
    StatusTypes  []DetectedStatus
    MinPriority  int
    MaxAge       time.Duration
}

func (f *QueueFilter) Apply(entries []ReviewQueueEntry) []ReviewQueueEntry
```

**Success Criteria**:
- Intuitive filter controls
- Instant filter application
- Filter state persistence

**Testing**: Filter logic tests

**Dependencies**: Story 3 completion

**Status**: ⏳ Pending

### Task 4.2: Quick Actions Menu (3h)

**Scope**: Add context menu for queue entries

**Files**:
- ui/queue_actions.go (create)
- app/app.go (modify)

**Context**:
- Actions: Jump to session, dismiss, snooze
- Keyboard shortcuts for common actions
- Confirmation for destructive actions

**Implementation**:
```go
type QueueAction int

const (
    ActionJumpTo QueueAction = iota
    ActionDismiss
    ActionSnooze
    ActionMarkResolved
)

func (v *QueueView) ExecuteAction(action QueueAction)
```

**Success Criteria**:
- Fast action execution
- Undo capability
- Clear action feedback

**Testing**: Action execution tests

**Dependencies**: Task 4.1

**Status**: ⏳ Pending

### Task 4.3: Batch Operations (2h)

**Scope**: Enable operations on multiple queue entries

**Files**:
- ui/queue_view.go (modify)
- ui/queue_selection.go (create)

**Context**:
- Multi-select with shift+arrow
- Batch dismiss/snooze
- Select all/none shortcuts

**Implementation**:
```go
type QueueSelection struct {
    selected map[string]bool
    anchor   int
}

func (s *QueueSelection) Toggle(entryID string)
func (s *QueueSelection) ExecuteBatch(action QueueAction)
```

**Success Criteria**:
- Intuitive selection model
- Visual selection indicators
- Batch operation confirmation

**Testing**: Selection state tests

**Dependencies**: Task 4.2

**Status**: ⏳ Pending

---

## Story 5: Performance and Integration (1 week)

**Objective**: Optimize performance and ensure seamless integration with existing features.

**Value**: Review queue operates smoothly even with hundreds of sessions.

### Task 5.1: Queue Caching Strategy (3h)

**Scope**: Implement efficient caching for queue operations

**Files**:
- session/queue_cache.go (create)
- session/queue_manager.go (modify)

**Context**:
- Cache computed priorities
- Invalidate on status changes
- Memory-efficient storage

**Implementation**:
```go
type QueueCache struct {
    entries     []ReviewQueueEntry
    lastUpdate  time.Time
    ttl         time.Duration
}

func (c *QueueCache) Get() ([]ReviewQueueEntry, bool)
func (c *QueueCache) Invalidate()
```

**Success Criteria**:
- Sub-100ms queue retrieval
- Accurate cache invalidation
- Bounded memory usage

**Testing**: Cache performance benchmarks

**Dependencies**: Stories 1-4 complete

**Status**: ⏳ Pending

### Task 5.2: Background Status Monitoring (4h)

**Scope**: Implement efficient background monitoring

**Files**:
- session/background_monitor.go (create)
- app/app.go (modify)

**Context**:
- Goroutine pool for monitoring
- Rate limiting for PTY reads
- Graceful shutdown

**Implementation**:
```go
type BackgroundMonitor struct {
    workers  int
    workChan chan *Instance
    stopChan chan struct{}
}

func (m *BackgroundMonitor) Start()
func (m *BackgroundMonitor) MonitorInstance(instance *Instance)
```

**Success Criteria**:
- < 1% CPU usage idle
- No goroutine leaks
- Responsive to status changes

**Testing**: Load tests with many sessions

**Dependencies**: Task 5.1

**Status**: ⏳ Pending

### Task 5.3: Integration Tests (3h)

**Scope**: Comprehensive integration testing

**Files**:
- integration/queue_test.go (create)
- testdata/queue_scenarios.yaml (create)

**Context**:
- Test with real tmux sessions
- Various status scenarios
- Performance benchmarks

**Implementation**:
```go
func TestQueueIntegration(t *testing.T) {
    // Create test sessions
    // Trigger various states
    // Verify queue updates
    // Test user interactions
}
```

**Success Criteria**:
- 100% scenario coverage
- Performance benchmarks pass
- No race conditions

**Testing**: Full integration test suite

**Dependencies**: All stories complete

**Status**: ⏳ Pending

---

## Dependency Visualization

```
Story 1 (Status Detection)
├─ Task 1.1: Extend Status Enum ──────────┐
├─ Task 1.2: Pattern Definitions ─────────┼─┐
├─ Task 1.3: Idle Detection ──────────────┘ │
└─ Task 1.4: Priority Scoring ──────────────┘
                │
                ▼
Story 2 (Data Model) [depends on Story 1]
├─ Task 2.1: Queue Entry Model
├─ Task 2.2: Queue Manager ───────────────┐
└─ Task 2.3: Session Monitor ─────────────┘
                │
                ▼
Story 3 (UI Components) [depends on Story 2]
├─ Task 3.1: Queue View Mode ─────────────┐
├─ Task 3.2: Toggle Key Binding ──────────┤
├─ Task 3.3: Entry Rendering ─────────────┤
└─ Task 3.4: Status Indicator ────────────┘
                │
                ▼
Story 4 (Filtering/Actions) [depends on Story 3]
├─ Task 4.1: Filtering Options
├─ Task 4.2: Quick Actions ───────────────┐
└─ Task 4.3: Batch Operations ────────────┘
                │
                ▼
Story 5 (Performance) [depends on Stories 1-4]
├─ Task 5.1: Caching Strategy
├─ Task 5.2: Background Monitoring ───────┐
└─ Task 5.3: Integration Tests ───────────┘
```

---

## Context Preparation

**Required Understanding Before Starting**:

1. **Stapler Squad Architecture**:
   - BubbleTea event-driven model
   - Session/Instance lifecycle
   - tmux integration patterns

2. **Existing Status Detection**:
   - `session/status_detector.go` implementation
   - YAML pattern loading system
   - PTY output monitoring

3. **UI Component Patterns**:
   - List view implementation (`ui/list.go`)
   - Filtering mechanisms
   - Key binding system

4. **Testing Approach**:
   - Unit tests for business logic
   - Integration tests with tmux
   - Performance benchmarks

---

## Implementation Order

**Recommended Sequence**:

1. **Phase 1 - Foundation** (Week 1):
   - Story 1: Status Detection Enhancement
   - Start with Tasks 1.1-1.2 (parallel)
   - Then Tasks 1.3-1.4 (sequential)

2. **Phase 2 - Core Logic** (Week 2):
   - Story 2: Data Model
   - Tasks 2.1-2.3 (sequential)
   - Begin Story 3 Task 3.1 in parallel

3. **Phase 3 - User Interface** (Week 3):
   - Complete Story 3: UI Components
   - Tasks 3.2-3.4 can be parallel

4. **Phase 4 - Advanced Features** (Week 4):
   - Story 4: Filtering and Actions
   - Tasks can be developed in parallel

5. **Phase 5 - Polish** (Week 5):
   - Story 5: Performance optimization
   - Integration testing
   - Documentation

---

## Acceptance Criteria

**Epic-Level Success Criteria**:

- ✅ Users can view all sessions needing attention in one place
- ✅ Queue updates in real-time (< 2 second latency)
- ✅ Priority-based sorting helps focus on important items
- ✅ Seamless integration with existing navigation
- ✅ Performance scales to 100+ sessions
- ✅ Zero false positives in attention detection
- ✅ Comprehensive test coverage (> 80%)
- ✅ Documentation for users and developers

---

## Risk Mitigation

**Identified Risks**:

1. **Pattern Matching Accuracy**:
   - Risk: False positives/negatives in status detection
   - Mitigation: Extensive pattern testing with real outputs

2. **Performance with Many Sessions**:
   - Risk: UI lag with 50+ sessions
   - Mitigation: Caching, pagination, background processing

3. **tmux Integration Complexity**:
   - Risk: PTY reading overhead
   - Mitigation: Rate limiting, selective monitoring

4. **User Adoption**:
   - Risk: Feature discovery
   - Mitigation: Clear UI indicators, documentation

---

## Future Enhancements

**Post-MVP Features**:

1. **Notification System**:
   - Desktop notifications for high-priority items
   - Sound alerts for critical states

2. **Custom Priority Rules**:
   - User-defined priority formulas
   - Per-session priority overrides

3. **Queue Analytics**:
   - Session attention patterns
   - Response time metrics

4. **Smart Suggestions**:
   - ML-based priority prediction
   - Suggested actions based on context

5. **External Integrations**:
   - Slack/Discord notifications
   - REST API for queue status

---

## Technical Debt Considerations

**Areas to Address**:

1. **Existing Status System**:
   - Refactor to support extended statuses
   - Ensure backward compatibility

2. **Testing Infrastructure**:
   - Need better tmux mocking
   - Performance test harness

3. **Configuration Management**:
   - Centralize queue settings
   - User preferences persistence

---

## Documentation Requirements

**Required Documentation**:

1. **User Guide**:
   - Queue navigation shortcuts
   - Understanding priority indicators
   - Customization options

2. **Developer Documentation**:
   - Architecture overview
   - Adding new status patterns
   - Extending priority algorithm

3. **API Documentation**:
   - Queue manager interface
   - Observer pattern usage
   - Integration points

---

## Success Metrics

**Measurable Outcomes**:

1. **Performance Metrics**:
   - Queue update latency < 2s
   - UI render time < 100ms
   - Memory overhead < 50MB

2. **Quality Metrics**:
   - Test coverage > 80%
   - Zero critical bugs in first month
   - < 5% false positive rate

3. **User Experience Metrics**:
   - Time to identify attention items reduced by 70%
   - User satisfaction score > 4.5/5
   - Feature adoption rate > 60%

---

## Notes for Developers

**Key Implementation Considerations**:

1. **Thread Safety**: Queue manager must handle concurrent updates
2. **Memory Management**: Limit context storage per entry
3. **Error Handling**: Graceful degradation if monitoring fails
4. **Backward Compatibility**: Existing sessions continue to work
5. **Configuration**: All thresholds should be configurable
6. **Observability**: Add logging for queue operations
7. **Testing**: Mock time for deterministic tests

**Code Quality Standards**:

- Follow existing patterns in codebase
- Comprehensive unit test coverage
- Performance benchmarks for critical paths
- Clear documentation for public APIs
- Error messages with actionable context