# Notification System Improvements Feature Plan

## Executive Summary

This feature plan addresses three critical issues in Stapler Squad's notification handling system:
1. **Duplicate notifications on backend restart** - Frontend cannot distinguish initial snapshots from real-time events
2. **Poor idle timeout status messages** - Generic "Timed out after X" messages imply failure rather than providing actionable context
3. **No frontend acknowledgment grace period** - Users may be re-notified immediately after dismissing notifications

---

## Architecture Decision Records (ADRs)

### ADR-001: Snapshot vs Real-time Event Discrimination

**Status**: Proposed

**Context**: When the WebSocket connection is established or re-established, the backend sends all existing queue items as `ItemAdded` events with `trigger: "initial_snapshot"`. The frontend hook `useReviewQueueNotifications` tracks previous items using a `useRef`, which resets on component remount (e.g., after navigation or hot reload). This causes duplicate notifications because the frontend treats snapshot items as new additions.

**Decision**: Implement a multi-layer deduplication strategy:
1. **Frontend**: Use `localStorage` to persist seen notification IDs with expiration timestamps
2. **Protocol**: Add explicit `isSnapshot` boolean field to `ReviewQueueItemAddedEvent` (clearer than string matching)
3. **Backend**: Send snapshot events with the new boolean flag for unambiguous detection

**Consequences**:
- (+) Reliable deduplication across reconnections, page refreshes, and browser restarts
- (+) Explicit protocol semantics instead of string matching on `trigger` field
- (-) Requires protobuf schema change and regeneration
- (-) localStorage adds browser storage dependency

**Alternatives Considered**:
1. **Session storage only**: Rejected because it clears on tab close
2. **In-memory Set with reconnection reset**: Current approach, causes duplicates
3. **Backend deduplication**: Would require maintaining per-client state, complexity

---

### ADR-002: Semantic Status Context for Attention Reasons

**Status**: Proposed

**Context**: The current `ReasonIdleTimeout` with context "Timed out after X of inactivity" is confusing because:
- It implies a failure condition when the session is simply idle
- It doesn't explain *why* attention is needed or *what* action to take
- The `DetectedStatus` enum has richer status types that aren't being fully leveraged

**Decision**: Introduce new `AttentionReason` types and context generation that leverage `DetectedStatus`:
1. Add new reasons: `ReasonIdle`, `ReasonStale`, `ReasonWaitingForUser`
2. Generate human-readable context messages that explain the situation and suggest actions
3. Use pattern-based detection context from `StatusDetector` for richer messages

**Consequences**:
- (+) Clear, actionable notifications that explain what happened and what to do
- (+) Better user experience with distinct idle vs. stale vs. waiting states
- (-) Breaking change to `AttentionReason` enum (protobuf + frontend mapping)
- (-) Requires updating backend logic in `review_queue_poller.go`

**Alternatives Considered**:
1. **Context-only changes**: Rejected because reason enum affects filtering/grouping
2. **Keep single IdleTimeout**: Would require long context strings with embedded semantics

---

### ADR-003: Frontend Acknowledgment State Persistence

**Status**: Proposed

**Context**: The backend has a 5-minute grace period after acknowledgment before re-adding sessions to the queue. However, the frontend doesn't track acknowledged items across sessions. If a user dismisses a toast notification, they may see it again after:
- Page refresh
- WebSocket reconnection
- Component remount

**Decision**: Implement frontend acknowledgment tracking:
1. Store acknowledged session IDs with timestamps in `localStorage`
2. Apply grace period filtering in `useReviewQueueNotifications` hook
3. Sync with backend acknowledgment state on reconnection

**Consequences**:
- (+) Consistent notification experience across reconnections
- (+) Matches backend grace period behavior in frontend
- (-) Requires localStorage management and cleanup
- (-) May have edge cases with clock skew between frontend and backend

---

## Epic Overview

### Epic: Notification System Reliability & Clarity

**Epic ID**: NOTIF-001
**Priority**: High
**Estimated Effort**: 3-4 sprints (6-8 weeks)

**Problem Statement**: Users experience notification fatigue due to duplicate alerts on reconnection and confusing "timeout" messages for idle sessions. The lack of frontend acknowledgment persistence causes repeated notifications for dismissed items.

**Success Metrics**:
| Metric | Current | Target | Measurement Method |
|--------|---------|--------|-------------------|
| Duplicate notifications per reconnection | ~N (queue size) | 0 | Frontend telemetry |
| User confusion reports for "timeout" | Unknown | -80% | User feedback |
| Acknowledged items re-notified | ~100% on refresh | 0 within grace period | Frontend telemetry |
| Notification click-through rate | Baseline TBD | +20% | Frontend telemetry |

**Dependencies**:
- Protobuf schema changes require `buf generate` regeneration
- Backend changes need coordination with frontend deployment
- localStorage polyfill may be needed for SSR environments

---

## Story Breakdown

### Story 1: Snapshot Event Discrimination

**Story ID**: NOTIF-001-S01
**Points**: 5
**Sprint**: 1

**User Story**: As a user, I want to only receive notifications for genuinely new queue items so that I'm not overwhelmed with duplicate alerts when the page reloads or the connection drops.

**Acceptance Criteria**:
```gherkin
Given I have 5 items in my review queue
When my browser reconnects to the WebSocket
Then I should receive 0 new toast notifications
And the review queue panel should show all 5 items

Given I have 3 items in my review queue
And a new session enters the "Needs Approval" state
When the backend sends the ItemAdded event
Then I should receive exactly 1 toast notification
And hear exactly 1 notification sound

Given I dismissed a notification 2 minutes ago
When I refresh the page
Then I should not see a notification for that item
And the item should still appear in the review queue panel
```

**Technical Notes**:
- Add `is_snapshot` boolean to `ReviewQueueItemAddedEvent` in `proto/session/v1/events.proto`
- Update `sendInitialSnapshot()` in `server/review_queue_manager.go` to set `is_snapshot: true`
- Modify `useReviewQueueNotifications.ts` to check for snapshot flag and skip notifications
- Persist seen notification IDs in localStorage with 1-hour TTL

---

### Story 2: Semantic Status Messages

**Story ID**: NOTIF-001-S02
**Points**: 8
**Sprint**: 1-2

**User Story**: As a user, I want notification messages to explain why my attention is needed and what action to take so that I can quickly prioritize and respond to sessions.

**Acceptance Criteria**:
```gherkin
Given a session has been idle for 5 minutes with no terminal activity
When it's added to the review queue
Then the context message should say "Session idle - ready for next task"
And the reason should be "ATTENTION_REASON_IDLE"

Given a session shows an approval prompt in the terminal
When it's added to the review queue
Then the context should include the approval type (e.g., "File write permission requested")
And the reason should be "ATTENTION_REASON_APPROVAL_PENDING"

Given a session has task completion indicators in output
When it's added to the review queue
Then the context should say "Task completed - review results"
And the reason should be "ATTENTION_REASON_TASK_COMPLETE"

Given a session has been stale (no output) for 10+ minutes
When it's added to the review queue
Then the context should say "No activity for 10m - may be stuck or waiting"
And the reason should be "ATTENTION_REASON_STALE"
```

**Technical Notes**:
- Add new enum values to `AttentionReason` in `proto/session/v1/types.proto`
- Update `reasonToProto()` mapping in `server/review_queue_manager.go`
- Modify context generation in `session/review_queue_poller.go:checkSession()`
- Leverage `StatusDetector.DetectWithContext()` for pattern-based context

---

### Story 3: Frontend Acknowledgment Persistence

**Story ID**: NOTIF-001-S03
**Points**: 5
**Sprint**: 2

**User Story**: As a user, I want my notification dismissals to persist across page refreshes so that I'm not repeatedly notified about the same items I've already acknowledged.

**Acceptance Criteria**:
```gherkin
Given I acknowledged a session notification
When I refresh the page within 5 minutes
Then I should not receive a notification for that session

Given I acknowledged a session notification
When 5 minutes have passed and the session has new activity
Then I should receive a notification for that session

Given I have acknowledged items in localStorage
When those items have expired (>1 hour old entries)
Then the expired entries should be automatically cleaned up
```

**Technical Notes**:
- Create `lib/utils/notificationStorage.ts` for localStorage management
- Add `getAcknowledgedSessions()`, `setAcknowledged()`, `cleanupExpired()` functions
- Integrate with `useReviewQueueNotifications` hook
- Consider IndexedDB for larger scale (if many sessions)

---

### Story 4: Notification History Enhancement

**Story ID**: NOTIF-001-S04
**Points**: 3
**Sprint**: 2

**User Story**: As a user, I want to view my notification history and acknowledge items directly from the notification panel so that I can manage my attention queue efficiently.

**Acceptance Criteria**:
```gherkin
Given I have received 10 notifications
When I open the notification history panel
Then I should see all 10 notifications with timestamps
And unread notifications should be visually distinct

Given I click "Acknowledge" on a notification in the history
When the action completes
Then the item should be removed from the review queue
And the notification should be marked as read
And I should not be re-notified for 5 minutes
```

**Technical Notes**:
- Extend `NotificationContext.tsx` to support acknowledgment from history
- Add `acknowledgeFromNotification(sessionId)` method
- Wire to existing `useReviewQueue.acknowledgeSession()` API
- Update notification history item UI with acknowledge button

---

### Story 5: WebSocket Reconnection State Sync

**Story ID**: NOTIF-001-S05
**Points**: 5
**Sprint**: 3

**User Story**: As a user, I want my notification state to stay synchronized when my connection drops and reconnects so that I have a consistent experience.

**Acceptance Criteria**:
```gherkin
Given I'm connected to the WebSocket
When the connection drops and reconnects
Then I should see a reconnection indicator
And my notification preferences should be preserved
And duplicate notifications should not fire

Given I acknowledged items while offline (queued actions)
When the connection is restored
Then the acknowledgments should be synced to the backend
And the queue should reflect my actions
```

**Technical Notes**:
- Add connection state tracking to `useReviewQueue.ts`
- Implement optimistic action queue for offline acknowledgments
- Send batch acknowledgment on reconnection
- Add reconnection indicator UI component

---

## Atomic Task Decomposition

### Story 1 Tasks: Snapshot Event Discrimination

#### Task 1.1: Add is_snapshot Field to Protobuf Schema
**Estimated Time**: 1-2 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/events.proto`
- Regenerated: `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/gen/session/v1/events_pb.ts`

**Implementation Details**:
```protobuf
// In ReviewQueueItemAddedEvent message
message ReviewQueueItemAddedEvent {
  ReviewItem item = 1;
  string trigger = 2;
  bool is_snapshot = 3; // NEW: true for initial snapshot items, false for real-time
}
```

**Verification**:
```bash
cd proto && buf generate
# Verify events_pb.ts contains is_snapshot field
grep -n "isSnapshot\|is_snapshot" web-app/src/gen/session/v1/events_pb.ts
```

---

#### Task 1.2: Update Backend to Set is_snapshot Flag
**Estimated Time**: 1-2 hours
**Files** (1):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/review_queue_manager.go`

**Implementation Details**:
```go
// In sendInitialSnapshot()
func (rqm *ReactiveQueueManager) sendInitialSnapshot(client *reviewQueueStreamClient) {
    items := rqm.queue.List()
    for _, item := range items {
        event := &sessionv1.ReviewQueueEvent{
            Timestamp: timestamppb.Now(),
            Event: &sessionv1.ReviewQueueEvent_ItemAdded{
                ItemAdded: &sessionv1.ReviewQueueItemAddedEvent{
                    Item:       rqm.reviewItemToProto(item),
                    Trigger:    "initial_snapshot",
                    IsSnapshot: true, // NEW
                },
            },
        }
        // ... send event
    }
}

// In OnItemAdded()
func (rqm *ReactiveQueueManager) OnItemAdded(item *session.ReviewItem) {
    event := &sessionv1.ReviewQueueEvent{
        // ...
        Event: &sessionv1.ReviewQueueEvent_ItemAdded{
            ItemAdded: &sessionv1.ReviewQueueItemAddedEvent{
                Item:       rqm.reviewItemToProto(item),
                Trigger:    "reactive_manager",
                IsSnapshot: false, // Explicit for real-time events
            },
        },
    }
    // ...
}
```

**Verification**:
- Unit test: Verify snapshot events have `IsSnapshot: true`
- Unit test: Verify real-time events have `IsSnapshot: false`

---

#### Task 1.3: Create Notification Storage Utility
**Estimated Time**: 2-3 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationStorage.ts` (new)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationStorage.test.ts` (new)

**Implementation Details**:
```typescript
// notificationStorage.ts
interface NotificationRecord {
  sessionId: string;
  notifiedAt: number;  // Unix timestamp
  acknowledgedAt?: number;
}

const STORAGE_KEY = 'stapler-squad-notifications';
const NOTIFICATION_TTL_MS = 60 * 60 * 1000; // 1 hour
const GRACE_PERIOD_MS = 5 * 60 * 1000; // 5 minutes

export function getNotifiedSessions(): Map<string, NotificationRecord> {
  try {
    const data = localStorage.getItem(STORAGE_KEY);
    if (!data) return new Map();
    const records: NotificationRecord[] = JSON.parse(data);
    return new Map(records.map(r => [r.sessionId, r]));
  } catch {
    return new Map();
  }
}

export function markNotified(sessionId: string): void {
  const records = getNotifiedSessions();
  records.set(sessionId, {
    sessionId,
    notifiedAt: Date.now(),
  });
  saveRecords(records);
}

export function markAcknowledged(sessionId: string): void {
  const records = getNotifiedSessions();
  const existing = records.get(sessionId);
  records.set(sessionId, {
    sessionId,
    notifiedAt: existing?.notifiedAt ?? Date.now(),
    acknowledgedAt: Date.now(),
  });
  saveRecords(records);
}

export function shouldNotify(sessionId: string): boolean {
  const records = getNotifiedSessions();
  const record = records.get(sessionId);
  
  if (!record) return true; // Never notified
  
  // Within grace period after acknowledgment?
  if (record.acknowledgedAt) {
    const timeSinceAck = Date.now() - record.acknowledgedAt;
    if (timeSinceAck < GRACE_PERIOD_MS) {
      return false; // Still in grace period
    }
  }
  
  return true;
}

export function cleanupExpired(): void {
  const records = getNotifiedSessions();
  const now = Date.now();
  
  for (const [sessionId, record] of records) {
    if (now - record.notifiedAt > NOTIFICATION_TTL_MS) {
      records.delete(sessionId);
    }
  }
  
  saveRecords(records);
}

function saveRecords(records: Map<string, NotificationRecord>): void {
  const data = Array.from(records.values());
  localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
}
```

**Verification**:
- Unit tests for all functions
- Test TTL expiration logic
- Test grace period logic

---

#### Task 1.4: Update useReviewQueueNotifications Hook
**Estimated Time**: 2-3 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueueNotifications.ts`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueueNotifications.test.ts` (new or update)

**Implementation Details**:
```typescript
// Add imports
import {
  markNotified,
  shouldNotify,
  cleanupExpired,
} from "@/lib/utils/notificationStorage";

// In the hook, update the effect
useEffect(() => {
  if (!enabled) return;

  // Periodic cleanup of expired records
  cleanupExpired();

  const currentItemIds = new Set(items.map((item) => item.sessionId));

  // Skip on initial load
  if (isInitialLoadRef.current) {
    isInitialLoadRef.current = false;
    // Mark all current items as notified (snapshot)
    currentItemIds.forEach(id => markNotified(id));
    previousItemsRef.current = currentItemIds;
    return;
  }

  // Find truly new items
  const newItemIds = Array.from(currentItemIds).filter((id) => {
    // Not in previous set AND should notify (respects grace period)
    return !previousItemsRef.current.has(id) && shouldNotify(id);
  });

  if (newItemIds.length > 0) {
    const newItems = items.filter((item) =>
      newItemIds.includes(item.sessionId)
    );

    // Mark as notified
    newItemIds.forEach(id => markNotified(id));

    // Play sound, show toasts, etc.
    playNotificationSound(soundType);
    // ... rest of notification logic
  }

  previousItemsRef.current = currentItemIds;
}, [items, enabled, /* other deps */]);
```

**Verification**:
- Integration test: Simulate reconnection and verify no duplicate notifications
- Test grace period behavior after acknowledgment

---

#### Task 1.5: Update useReviewQueue to Detect Snapshot Events
**Estimated Time**: 1-2 hours
**Files** (1):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueue.ts`

**Implementation Details**:
```typescript
// In handleReviewQueueEventRef.current
case "itemAdded": {
  const addedEvent = event.event.value;
  const item = addedEvent.item;
  const isSnapshot = addedEvent.isSnapshot; // NEW field
  
  if (!item) break;

  // Add item to queue
  setReviewQueue((prev) => {
    if (!prev) return prev;
    const newItems = [...(prev.items ?? []), item];
    return new ReviewQueue({
      ...prev,
      items: newItems,
      totalItems: prev.totalItems + 1,
    });
  });

  // Only trigger notification-related callbacks for non-snapshot events
  if (!isSnapshot) {
    // Could add a callback or state flag here for notification hook
    // For now, the notification hook filters via shouldNotify()
  }
  break;
}
```

**Verification**:
- Unit test verifying snapshot events are processed but flagged
- Integration test with notification hook

---

### Story 2 Tasks: Semantic Status Messages

#### Task 2.1: Add New AttentionReason Enum Values
**Estimated Time**: 1-2 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/types.proto`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/review_queue.go`

**Implementation Details**:
```protobuf
// In types.proto
enum AttentionReason {
  ATTENTION_REASON_UNSPECIFIED = 0;
  ATTENTION_REASON_APPROVAL_PENDING = 1;
  ATTENTION_REASON_INPUT_REQUIRED = 2;
  ATTENTION_REASON_ERROR_STATE = 3;
  ATTENTION_REASON_IDLE_TIMEOUT = 4;  // Deprecated - use IDLE or STALE
  ATTENTION_REASON_TASK_COMPLETE = 5;
  ATTENTION_REASON_UNCOMMITTED_CHANGES = 6;
  // New semantic reasons
  ATTENTION_REASON_IDLE = 7;           // Session idle, ready for next task
  ATTENTION_REASON_STALE = 8;          // No output for extended period, may be stuck
  ATTENTION_REASON_WAITING_FOR_USER = 9; // Explicitly waiting for user input
  ATTENTION_REASON_TESTS_FAILING = 10;   // Tests are failing
}
```

```go
// In review_queue.go
const (
    ReasonApprovalPending    AttentionReason = "approval_pending"
    ReasonInputRequired      AttentionReason = "input_required"
    ReasonErrorState         AttentionReason = "error_state"
    ReasonIdleTimeout        AttentionReason = "idle_timeout"  // Deprecated
    ReasonTaskComplete       AttentionReason = "task_complete"
    ReasonUncommittedChanges AttentionReason = "uncommitted_changes"
    // New reasons
    ReasonIdle               AttentionReason = "idle"
    ReasonStale              AttentionReason = "stale"
    ReasonWaitingForUser     AttentionReason = "waiting_for_user"
    ReasonTestsFailing       AttentionReason = "tests_failing"
)
```

**Verification**:
```bash
cd proto && buf generate
# Verify TypeScript enum values
grep -A 20 "AttentionReason" web-app/src/gen/session/v1/types_pb.ts
```

---

#### Task 2.2: Update Reason-to-Proto Mapping
**Estimated Time**: 1-2 hours
**Files** (1):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/review_queue_manager.go`

**Implementation Details**:
```go
func (rqm *ReactiveQueueManager) reasonToProto(r session.AttentionReason) sessionv1.AttentionReason {
    switch r {
    case session.ReasonApprovalPending:
        return sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING
    case session.ReasonInputRequired:
        return sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED
    case session.ReasonErrorState:
        return sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE
    case session.ReasonIdleTimeout:
        // Map legacy to new reason based on context (if available)
        return sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT
    case session.ReasonTaskComplete:
        return sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE
    case session.ReasonUncommittedChanges:
        return sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES
    // New reasons
    case session.ReasonIdle:
        return sessionv1.AttentionReason_ATTENTION_REASON_IDLE
    case session.ReasonStale:
        return sessionv1.AttentionReason_ATTENTION_REASON_STALE
    case session.ReasonWaitingForUser:
        return sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER
    case session.ReasonTestsFailing:
        return sessionv1.AttentionReason_ATTENTION_REASON_TESTS_FAILING
    default:
        return sessionv1.AttentionReason_ATTENTION_REASON_UNSPECIFIED
    }
}
```

**Verification**:
- Unit tests for all reason mappings

---

#### Task 2.3: Implement Semantic Context Generation
**Estimated Time**: 3-4 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/review_queue_poller.go`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/context_generator.go` (new)

**Implementation Details**:
```go
// context_generator.go - NEW FILE
package session

import (
    "fmt"
    "time"
)

// ContextGenerator creates human-readable context messages for review items.
type ContextGenerator struct {
    statusDetector *StatusDetector
}

func NewContextGenerator() *ContextGenerator {
    return &ContextGenerator{
        statusDetector: NewStatusDetector(),
    }
}

// GenerateContext creates a context message based on the detected status and reason.
func (cg *ContextGenerator) GenerateContext(
    detectedStatus DetectedStatus,
    reason AttentionReason,
    idleDuration time.Duration,
    statusContext string,
) string {
    switch reason {
    case ReasonIdle:
        return "Session idle - ready for next task"
    
    case ReasonStale:
        durationStr := formatDuration(idleDuration)
        return fmt.Sprintf("No activity for %s - session may be stuck or waiting", durationStr)
    
    case ReasonApprovalPending:
        if statusContext != "" {
            return statusContext // Use pattern-detected context
        }
        return "Waiting for approval to proceed"
    
    case ReasonInputRequired:
        if statusContext != "" {
            return statusContext
        }
        return "Waiting for your input"
    
    case ReasonTaskComplete:
        if statusContext != "" {
            return statusContext
        }
        return "Task completed - review results"
    
    case ReasonErrorState:
        if statusContext != "" {
            return statusContext
        }
        return "Error occurred - investigation needed"
    
    case ReasonTestsFailing:
        if statusContext != "" {
            return statusContext
        }
        return "Tests are failing - review required"
    
    case ReasonUncommittedChanges:
        return "Uncommitted changes ready to commit"
    
    case ReasonWaitingForUser:
        if statusContext != "" {
            return statusContext
        }
        return "Claude is waiting for your response"
    
    default:
        if statusContext != "" {
            return statusContext
        }
        return "Session needs attention"
    }
}

func formatDuration(d time.Duration) string {
    if d < time.Minute {
        return fmt.Sprintf("%ds", int(d.Seconds()))
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm", int(d.Minutes()))
    }
    return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
```

**Verification**:
- Unit tests for all context message cases
- Test duration formatting

---

#### Task 2.4: Update Poller to Use New Reasons
**Estimated Time**: 2-3 hours
**Files** (1):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/review_queue_poller.go`

**Implementation Details**:
```go
// In checkSession(), replace IdleTimeout logic:

// Replace this pattern:
// reason = ReasonIdleTimeout
// context = fmt.Sprintf("Timed out after %s of inactivity", formatDuration(idleDuration))

// With semantic reasons based on context:
switch idleState {
case IdleStateWaiting:
    // Short idle, just waiting
    reason = ReasonIdle
    context = cg.GenerateContext(statusInfo.ClaudeStatus, reason, idleDuration, statusInfo.StatusContext)

case IdleStateTimeout:
    // Long idle with no output - might be stuck
    if timeSinceOutput > rqp.config.StalenessThreshold {
        reason = ReasonStale
    } else {
        reason = ReasonIdle
    }
    context = cg.GenerateContext(statusInfo.ClaudeStatus, reason, idleDuration, statusInfo.StatusContext)
}

// For explicit user input detection (detected by StatusDetector)
if statusInfo.ClaudeStatus == StatusInputRequired {
    reason = ReasonWaitingForUser
    context = cg.GenerateContext(statusInfo.ClaudeStatus, reason, 0, statusInfo.StatusContext)
}
```

**Verification**:
- Integration test: Verify new reasons are assigned correctly
- Test context messages are clear and actionable

---

#### Task 2.5: Update Frontend Reason Labels
**Estimated Time**: 1-2 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/sessions/ReviewQueueBadge.tsx`

**Implementation Details**:
```typescript
// In ReviewQueuePanel.tsx
const getReasonLabel = (reason: AttentionReason): string => {
  switch (reason) {
    case AttentionReason.APPROVAL_PENDING:
      return "Approval Needed";
    case AttentionReason.INPUT_REQUIRED:
      return "Input Needed";
    case AttentionReason.ERROR_STATE:
      return "Error";
    case AttentionReason.IDLE_TIMEOUT:
      return "Idle"; // Legacy fallback
    case AttentionReason.TASK_COMPLETE:
      return "Complete";
    case AttentionReason.UNCOMMITTED_CHANGES:
      return "Uncommitted";
    // New reasons
    case AttentionReason.IDLE:
      return "Idle";
    case AttentionReason.STALE:
      return "No Activity";
    case AttentionReason.WAITING_FOR_USER:
      return "Waiting";
    case AttentionReason.TESTS_FAILING:
      return "Tests Failing";
    default:
      return "Attention";
  }
};
```

**Verification**:
- Visual test: Verify all new badges render correctly
- Verify filter buttons work with new reasons

---

### Story 3 Tasks: Frontend Acknowledgment Persistence

#### Task 3.1: Extend Notification Storage for Acknowledgments
**Estimated Time**: 1-2 hours
**Files** (1):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationStorage.ts`

**Implementation Details**:
(Already covered in Task 1.3 - `markAcknowledged()` function)

Add additional methods:
```typescript
export function getAcknowledgedSessions(): Set<string> {
  const records = getNotifiedSessions();
  const acknowledged = new Set<string>();
  const now = Date.now();
  
  for (const [sessionId, record] of records) {
    if (record.acknowledgedAt) {
      const timeSinceAck = now - record.acknowledgedAt;
      if (timeSinceAck < GRACE_PERIOD_MS) {
        acknowledged.add(sessionId);
      }
    }
  }
  
  return acknowledged;
}

export function isInGracePeriod(sessionId: string): boolean {
  const records = getNotifiedSessions();
  const record = records.get(sessionId);
  
  if (!record?.acknowledgedAt) return false;
  
  const timeSinceAck = Date.now() - record.acknowledgedAt;
  return timeSinceAck < GRACE_PERIOD_MS;
}
```

**Verification**:
- Unit tests for grace period logic

---

#### Task 3.2: Integrate Acknowledgment with Notification Context
**Estimated Time**: 2-3 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueue.ts`

**Implementation Details**:
```typescript
// In NotificationContext.tsx
import { markAcknowledged } from "@/lib/utils/notificationStorage";

// Update showSessionNotification to accept acknowledgment callback
const showSessionNotification = useCallback(
  (item: ReviewItem, onView?: () => void, onAcknowledge?: () => void) => {
    addNotification({
      sessionId: item.sessionId,
      sessionName: item.sessionName || "Unnamed Session",
      message: item.context || "This session is waiting for your input",
      priority,
      onView,
      onAcknowledge: () => {
        // Mark in localStorage
        markAcknowledged(item.sessionId);
        // Call provided callback
        onAcknowledge?.();
      },
    });
  },
  [addNotification]
);
```

```typescript
// In useReviewQueue.ts - update acknowledgeSession
const acknowledgeSession = useCallback(
  async (sessionId: string) => {
    // Import and call markAcknowledged
    markAcknowledged(sessionId);
    
    // Optimistic update
    setReviewQueue((prev) => { /* ... */ });

    try {
      // API call
      await clientRef.current.acknowledgeSession(request);
    } catch (err) {
      // Rollback
    }
  },
  [refresh]
);
```

**Verification**:
- Integration test: Acknowledge, refresh, verify no re-notification
- Test grace period expiration

---

#### Task 3.3: Add Acknowledge Button to Notification Toast
**Estimated Time**: 2 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationToast.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationToast.module.css`

**Implementation Details**:
```typescript
// In NotificationToast.tsx
export interface NotificationData {
  id: string;
  sessionId: string;
  sessionName: string;
  message: string;
  priority: "urgent" | "high" | "medium" | "low";
  timestamp: number;
  onView?: () => void;
  onAcknowledge?: () => void; // NEW
}

// In the component
<div className={styles.actions}>
  {notification.onView && (
    <button onClick={notification.onView} className={styles.viewButton}>
      View
    </button>
  )}
  {notification.onAcknowledge && (
    <button 
      onClick={() => {
        notification.onAcknowledge?.();
        onClose();
      }} 
      className={styles.acknowledgeButton}
    >
      Dismiss
    </button>
  )}
</div>
```

**Verification**:
- Visual test: Verify Dismiss button appears and works
- Test notification closes after dismiss

---

### Story 4 Tasks: Notification History Enhancement

#### Task 4.1: Add Acknowledge Action to History Panel
**Estimated Time**: 2 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/notifications/NotificationHistoryPanel.tsx`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx`

**Implementation Details**:
```typescript
// Add acknowledgeFromHistory to NotificationContext
const acknowledgeFromHistory = useCallback(
  async (sessionId: string) => {
    // Mark acknowledged in localStorage
    markAcknowledged(sessionId);
    
    // Mark as read in history
    const notification = notificationHistory.find(n => n.sessionId === sessionId);
    if (notification) {
      markAsRead(notification.id);
    }
    
    // Log audit event
    auditLog.logSessionAcknowledgedFromHistory(sessionId);
    
    // TODO: Need access to acknowledgeSession from useReviewQueue
    // This may require lifting state or passing callback
  },
  [notificationHistory, markAsRead, auditLog]
);
```

**Verification**:
- Integration test: Acknowledge from history, verify queue updated
- Audit log verification

---

### Story 5 Tasks: WebSocket Reconnection State Sync

#### Task 5.1: Add Connection State Tracking
**Estimated Time**: 2-3 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueue.ts`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useConnectionState.ts` (new)

**Implementation Details**:
```typescript
// useConnectionState.ts - NEW FILE
export type ConnectionState = 'connected' | 'disconnected' | 'reconnecting';

export function useConnectionState() {
  const [state, setState] = useState<ConnectionState>('disconnected');
  const [lastConnected, setLastConnected] = useState<number | null>(null);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);

  const setConnected = useCallback(() => {
    setState('connected');
    setLastConnected(Date.now());
    setReconnectAttempts(0);
  }, []);

  const setDisconnected = useCallback(() => {
    setState('disconnected');
  }, []);

  const setReconnecting = useCallback(() => {
    setState('reconnecting');
    setReconnectAttempts(prev => prev + 1);
  }, []);

  return {
    state,
    lastConnected,
    reconnectAttempts,
    setConnected,
    setDisconnected,
    setReconnecting,
  };
}
```

**Verification**:
- Unit test state transitions
- Test reconnect attempt counting

---

#### Task 5.2: Add Reconnection Indicator UI
**Estimated Time**: 2 hours
**Files** (3):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/ConnectionIndicator.tsx` (new)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/ConnectionIndicator.module.css` (new)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/layout/Header.tsx` (update)

**Implementation Details**:
```typescript
// ConnectionIndicator.tsx
interface ConnectionIndicatorProps {
  state: ConnectionState;
  reconnectAttempts?: number;
}

export function ConnectionIndicator({ state, reconnectAttempts }: ConnectionIndicatorProps) {
  if (state === 'connected') {
    return <div className={styles.connected} title="Connected" />;
  }
  
  if (state === 'reconnecting') {
    return (
      <div className={styles.reconnecting}>
        <span className={styles.spinner} />
        {reconnectAttempts && reconnectAttempts > 1 && (
          <span>Attempt {reconnectAttempts}</span>
        )}
      </div>
    );
  }
  
  return <div className={styles.disconnected} title="Disconnected" />;
}
```

**Verification**:
- Visual test: All three states render correctly
- Animation test: Spinner animates during reconnecting

---

#### Task 5.3: Implement Offline Action Queue
**Estimated Time**: 3-4 hours
**Files** (2):
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/offlineQueue.ts` (new)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueue.ts`

**Implementation Details**:
```typescript
// offlineQueue.ts - NEW FILE
interface QueuedAction {
  type: 'acknowledge';
  sessionId: string;
  timestamp: number;
}

const QUEUE_KEY = 'stapler-squad-offline-queue';

export function queueAction(action: QueuedAction): void {
  const queue = getQueue();
  queue.push(action);
  localStorage.setItem(QUEUE_KEY, JSON.stringify(queue));
}

export function getQueue(): QueuedAction[] {
  try {
    const data = localStorage.getItem(QUEUE_KEY);
    return data ? JSON.parse(data) : [];
  } catch {
    return [];
  }
}

export function clearQueue(): void {
  localStorage.removeItem(QUEUE_KEY);
}

export async function flushQueue(
  acknowledgeSession: (sessionId: string) => Promise<void>
): Promise<{ success: number; failed: number }> {
  const queue = getQueue();
  let success = 0;
  let failed = 0;

  for (const action of queue) {
    if (action.type === 'acknowledge') {
      try {
        await acknowledgeSession(action.sessionId);
        success++;
      } catch {
        failed++;
      }
    }
  }

  clearQueue();
  return { success, failed };
}
```

**Verification**:
- Unit tests for queue operations
- Integration test: Queue while offline, flush on reconnect

---

## Known Issues / Bugs Identified During Planning

### Bug 1: Race Condition in Snapshot Processing
**Severity**: Medium
**Component**: `useReviewQueue.ts` + `useReviewQueueNotifications.ts`

**Description**: When the WebSocket reconnects, snapshot events may arrive before the `previousItemsRef` is updated, causing a brief window where items might be incorrectly identified as new.

**Mitigation**:
- Add `isProcessingSnapshot` flag to prevent notification processing during snapshot ingestion
- Use `useRef` to track snapshot completion timestamp

**Files Affected**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueue.ts`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useReviewQueueNotifications.ts`

---

### Bug 2: localStorage Quota Exceeded
**Severity**: Low
**Component**: `notificationStorage.ts`

**Description**: If cleanup is not run frequently, and users have many sessions over time, localStorage may exceed quota (typically 5MB).

**Mitigation**:
- Implement aggressive cleanup on every write
- Add try/catch around localStorage operations
- Consider LRU eviction for oldest records

**Files Affected**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationStorage.ts`

---

### Bug 3: Clock Skew Between Frontend and Backend
**Severity**: Low
**Component**: Grace period logic

**Description**: If client clock differs significantly from server, grace period calculations may be incorrect.

**Mitigation**:
- Use relative timestamps from server when possible
- Add 30-second buffer to grace period calculations
- Document clock synchronization requirement

**Files Affected**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationStorage.ts`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/review_queue_poller.go`

---

### Bug 4: Context Message Truncation
**Severity**: Low
**Component**: `StatusDetector.DetectWithContext()`

**Description**: Some pattern descriptions may be too long for toast notifications, causing UI overflow.

**Mitigation**:
- Add max length (100 chars) to context messages
- Implement smart truncation with "..." suffix
- Show full context on hover/click

**Files Affected**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/context_generator.go`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationToast.tsx`

---

## Dependency Visualization

```
                    ┌──────────────────────────────────────────────────────────────┐
                    │                    STORY 1: Snapshot Discrimination           │
                    │                                                               │
                    │  ┌─────────────┐    ┌─────────────┐    ┌──────────────────┐  │
                    │  │ Task 1.1    │───▶│ Task 1.2    │    │ Task 1.3         │  │
                    │  │ Proto Schema│    │ Backend Flag│    │ Notification     │  │
                    │  └─────────────┘    └─────────────┘    │ Storage          │  │
                    │        │                   │           └────────┬─────────┘  │
                    │        │                   │                    │            │
                    │        ▼                   ▼                    ▼            │
                    │  ┌─────────────────────────────────────────────────────────┐ │
                    │  │                    Task 1.4                              │ │
                    │  │              useReviewQueueNotifications                 │ │
                    │  └─────────────────────────────────────────────────────────┘ │
                    │                              │                               │
                    │                              ▼                               │
                    │  ┌─────────────────────────────────────────────────────────┐ │
                    │  │                    Task 1.5                              │ │
                    │  │                 useReviewQueue                           │ │
                    │  └─────────────────────────────────────────────────────────┘ │
                    └──────────────────────────────────────────────────────────────┘
                                                   │
                                                   │ Shares: Proto changes, Storage utility
                                                   ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                     STORY 2: Semantic Status Messages                                 │
│                                                                                                       │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │ Task 2.1    │───▶│ Task 2.2    │    │ Task 2.3    │───▶│ Task 2.4    │───▶│ Task 2.5            │ │
│  │ Enum Values │    │ Reason Map  │    │ Context Gen │    │ Poller      │    │ Frontend Labels     │ │
│  └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                                   │
                                                   │ Independent - can run in parallel
                                                   ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                     STORY 3: Acknowledgment Persistence                               │
│                                                                                                       │
│  ┌─────────────┐    ┌─────────────────────┐    ┌─────────────────────────────────────────────────┐   │
│  │ Task 3.1    │───▶│ Task 3.2            │───▶│ Task 3.3                                        │   │
│  │ Storage Ext │    │ Context Integration │    │ Toast Button                                    │   │
│  └─────────────┘    └─────────────────────┘    └─────────────────────────────────────────────────┘   │
│       ▲                                                                                               │
│       │ Extends Task 1.3                                                                              │
└───────┼──────────────────────────────────────────────────────────────────────────────────────────────┘
        │
        │
┌───────┼──────────────────────────────────────────────────────────────────────────────────────────────┐
│       │                              STORY 4: Notification History                                    │
│       │                                                                                               │
│       │  ┌─────────────────────────────────────────────────────────────────────────────────────────┐ │
│       └─▶│ Task 4.1: History Panel Acknowledge                                                     │ │
│          │ (Depends on: Task 3.1, Task 3.2)                                                        │ │
│          └─────────────────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                                   │
                                                   │ Independent - can run in parallel
                                                   ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                     STORY 5: WebSocket Reconnection                                   │
│                                                                                                       │
│  ┌─────────────┐    ┌─────────────────────┐    ┌─────────────────────────────────────────────────┐   │
│  │ Task 5.1    │    │ Task 5.2            │    │ Task 5.3                                        │   │
│  │ Connection  │    │ Indicator UI        │───▶│ Offline Queue                                   │   │
│  │ State       │    │                     │    │                                                 │   │
│  └─────────────┘    └─────────────────────┘    └─────────────────────────────────────────────────┘   │
│        │                    ▲                                                                         │
│        └────────────────────┘                                                                         │
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

**Critical Path**: Story 1 (Task 1.1) -> Story 2 (Task 2.1) must complete first as they modify protobuf schemas.

**Parallelization Opportunities**:
- Story 2 and Story 3 can run in parallel after Story 1 completes
- Story 4 and Story 5 can run in parallel
- Within stories, tasks with no dependencies can parallelize (e.g., Task 1.3 parallel to Task 1.1+1.2)

---

## Integration Checkpoints

### Checkpoint 1: Proto Changes (End of Sprint 1, Week 1)
**Gate Criteria**:
- [ ] `is_snapshot` field added to `ReviewQueueItemAddedEvent`
- [ ] New `AttentionReason` enum values added
- [ ] `buf generate` succeeds without errors
- [ ] TypeScript types regenerated and compile
- [ ] No breaking changes to existing API consumers

**Verification Commands**:
```bash
cd proto && buf lint && buf generate
cd web-app && npm run build
go build ./...
go test ./server/... -v
```

### Checkpoint 2: Backend Logic (End of Sprint 1, Week 2)
**Gate Criteria**:
- [ ] `sendInitialSnapshot()` sets `is_snapshot: true`
- [ ] `OnItemAdded()` sets `is_snapshot: false`
- [ ] New reasons mapped in `reasonToProto()`
- [ ] `ContextGenerator` creates human-readable messages
- [ ] Poller uses new reasons appropriately

**Verification Commands**:
```bash
go test ./session/... -v
go test ./server/... -v
# Manual: Start backend, connect WebSocket, verify event payloads
```

### Checkpoint 3: Frontend Notification Dedup (End of Sprint 2, Week 1)
**Gate Criteria**:
- [ ] `notificationStorage.ts` implements all functions
- [ ] `useReviewQueueNotifications` checks `isSnapshot` flag
- [ ] `useReviewQueueNotifications` uses `shouldNotify()` filtering
- [ ] No duplicate notifications on page refresh

**Verification Commands**:
```bash
cd web-app && npm test -- --testPathPattern=notificationStorage
cd web-app && npm test -- --testPathPattern=useReviewQueueNotifications
# Manual: Refresh page, verify no duplicate toasts
```

### Checkpoint 4: Frontend Acknowledgment (End of Sprint 2, Week 2)
**Gate Criteria**:
- [ ] Toast includes "Dismiss" button
- [ ] Acknowledgment persists to localStorage
- [ ] Grace period prevents re-notification
- [ ] History panel shows acknowledge button

**Verification Commands**:
```bash
cd web-app && npm test -- --testPathPattern=NotificationToast
cd web-app && npm test -- --testPathPattern=NotificationContext
# Manual: Dismiss notification, refresh, verify no re-notification
```

### Checkpoint 5: Full Integration (End of Sprint 3)
**Gate Criteria**:
- [ ] All stories complete and tested
- [ ] End-to-end test: disconnect/reconnect without duplicate notifications
- [ ] End-to-end test: new session appears with semantic context
- [ ] End-to-end test: acknowledge persists across browser sessions
- [ ] Performance: < 100ms for notification decision

**Verification Commands**:
```bash
make test
cd web-app && npm run test:e2e
# Manual: Full workflow test with network disconnection
```

---

## Appendix: File Reference

### Backend Files (Go)
| File | Stories | Description |
|------|---------|-------------|
| `proto/session/v1/events.proto` | S1, S2 | Protobuf schema for events |
| `proto/session/v1/types.proto` | S2 | AttentionReason enum |
| `session/review_queue.go` | S2 | Core queue + reason types |
| `session/review_queue_poller.go` | S2 | Session monitoring + reason assignment |
| `session/context_generator.go` | S2 | NEW: Context message generation |
| `session/status_detector.go` | S2 | Pattern-based status detection |
| `server/review_queue_manager.go` | S1, S2 | WebSocket streaming + proto mapping |

### Frontend Files (TypeScript)
| File | Stories | Description |
|------|---------|-------------|
| `lib/hooks/useReviewQueue.ts` | S1, S3, S5 | Queue data + WebSocket |
| `lib/hooks/useReviewQueueNotifications.ts` | S1, S3 | Notification triggering |
| `lib/hooks/useConnectionState.ts` | S5 | NEW: Connection state |
| `lib/utils/notificationStorage.ts` | S1, S3 | NEW: localStorage management |
| `lib/utils/offlineQueue.ts` | S5 | NEW: Offline action queue |
| `lib/contexts/NotificationContext.tsx` | S3, S4 | Global notification state |
| `components/ui/NotificationToast.tsx` | S3 | Toast component |
| `components/ui/ConnectionIndicator.tsx` | S5 | NEW: Connection indicator |
| `components/sessions/ReviewQueuePanel.tsx` | S2 | Queue panel UI |
| `components/sessions/ReviewQueueBadge.tsx` | S2 | Priority/reason badges |
| `gen/session/v1/events_pb.ts` | S1 | Generated protobuf types |
| `gen/session/v1/types_pb.ts` | S2 | Generated enum types |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2024-XX-XX | Claude | Initial feature plan |
