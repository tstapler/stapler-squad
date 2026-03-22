# Notification De-Duplication and Aggregation

**Status**: Draft
**Priority**: P1 -- User-facing annoyance; 58 notifications with visible duplicates
**Epic ID**: EPIC-NOTIF-DEDUP-001
**Stories**: 3 (each 1-2 weeks)
**Updated**: 2026-03-19

---

## Problem Statement

The notification panel displays duplicate cards for the same `(sessionId, eventType)` combination. In observed usage a panel shows 58 notifications with obvious duplicates:

- "engineering-score-cards" APPROVAL_NEEDED appears 2+ times with identical file paths
- "rule-packs" APPROVAL_NEEDED shows the same branch path duplicated
- Every time the approval polling loop fires or a new hook POST arrives for the same session, a brand-new notification record is appended to the store

**Root cause**: `NotificationHistoryStore.Append()` (in `server/notifications/store.go`) deduplicates only by the `ID` field. Because each call to `SendNotification` generates a fresh `uuid.New()` notification ID, two approval events for the same session produce two distinct records even when they describe the same logical event.

The frontend (`NotificationContext.tsx`) also deduplicates only by `id`, so every record from the backend renders as its own card.

---

## Success Metrics

1. **Notification badge count** = number of distinct unread `(sessionId, notificationType)` groups, not raw record count
2. **Zero duplicate cards** for the same pending approval on the same session
3. **Aggregated card shows "xN" badge** when N > 1 occurrences exist in the group
4. **Mark-all-read** clears the aggregated unread count to 0
5. **`go test ./server/...`** passes after implementation
6. **`go test ./server/notifications/...`** has tests covering dedup and aggregation logic

---

## Architecture Overview

### Current Data Flow

```
ApprovalHandler.broadcastApprovalNotification()
    |
    v
EventBus.Publish(NotificationEvent)
    |
    +---> SSE to web clients (real-time toasts via useSessionNotifications)
    |
    +---> notifications.StartSubscriber -> store.Append(record)
                                                |
                                                v
                                    NotificationHistoryStore (JSON file)
                                                |
                                                v
                                    GetNotificationHistory RPC
                                                |
                                                v
                                    useNotificationHistory hook -> NotificationContext -> NotificationPanel
```

### Proposed Change Points

Deduplication happens at **two layers** for defense-in-depth:

1. **Server-side (primary)**: `NotificationHistoryStore` gains a dedup index keyed by `(sessionId, notificationType)`. When a new record arrives with a key that already exists in the store, the existing record is updated (timestamp refreshed, occurrence count incremented) instead of inserting a new row.

2. **Client-side (rendering)**: `NotificationPanel` groups records by `(sessionId, notificationType)` before rendering. Even if the server returns non-deduplicated records (e.g., from old persisted data before migration), the panel collapses them into one card with a count badge.

This "both" approach was chosen because:
- Server-side prevents the JSON file from ballooning (500 records with duplicates vs. 500 distinct records)
- Client-side provides immediate visual dedup even against stale data
- Neither layer alone is sufficient -- server-side alone misses already-persisted duplicates; client-side alone does not prevent store bloat

---

## ADR-001: Deduplication Key and Time Window

**Context**: We need to define what constitutes "the same notification" to determine when to aggregate rather than insert.

**Decision**: The deduplication key is `(sessionId, notificationType)`. Two records with the same session ID and notification type are considered duplicates. There is no time-window constraint -- all unread occurrences of the same key collapse into one group regardless of when they arrived.

**Rationale**:
- Approval notifications are the primary offender. A session can only have one active "approval needed" state at a time from the user's perspective.
- Adding a time window (e.g., "within 5 minutes") complicates the logic and would still show duplicates when approvals re-fire on the same session minutes apart.
- For non-approval notification types (info, task_complete, error), grouping by `(sessionId, type)` still makes sense -- the user wants to know "session X had 3 errors," not see 3 identical error cards.
- Read records are excluded from aggregation: once the user marks a group as read, a new occurrence of the same key creates a fresh unread group.

**Consequences**:
- Two genuinely different approval requests on the same session (e.g., different tools) will collapse into one card with a count badge. This is acceptable because the card links to the session where the user resolves approvals individually.
- If we later need per-tool distinction, we can refine the key to `(sessionId, notificationType, toolName)` by adding a `dedup_key` field to the proto message.

---

## ADR-002: Proto Backward Compatibility Strategy

**Context**: The `NotificationHistoryRecord` proto message needs to carry aggregation metadata (occurrence count) to the frontend. We need to decide whether to add fields to the existing message or create a new one.

**Decision**: Add two optional fields to the existing `NotificationHistoryRecord` message:

```protobuf
message NotificationHistoryRecord {
  // ... existing fields 1-11 ...

  // Number of deduplicated occurrences this record represents.
  // Default 0 means "1 occurrence" (backward-compatible with old clients).
  int32 occurrence_count = 12;

  // Timestamp of the most recent occurrence (may differ from created_at
  // which tracks the first occurrence).
  optional google.protobuf.Timestamp last_occurred_at = 13;
}
```

**Rationale**:
- Adding optional fields to an existing message is fully backward-compatible in protobuf. Old clients ignore unknown fields; old servers send 0/nil which new clients interpret as "1 occurrence."
- A separate `AggregatedNotificationRecord` message would require a new RPC or a `oneof` wrapper, adding unnecessary complexity for two fields.
- The `occurrence_count` uses 0 as the default (meaning single occurrence) so old persisted records with no count field render correctly.

**Consequences**:
- The `NotificationHistoryStore` JSON file gains `occurrence_count` and `last_occurred_at` fields. Old JSON files without these fields load correctly (Go zero values).
- No new RPC endpoints needed.

---

## ADR-003: Mark-Read Semantics for Aggregated Groups

**Context**: When a user marks an aggregated group (showing "x3") as read, should it clear all underlying occurrences or just the latest?

**Decision**: Marking an aggregated record as read marks the single consolidated record as read. Because server-side deduplication collapses N occurrences into one record (with `occurrence_count = N`), there is only one record ID to mark. If a new occurrence of the same `(sessionId, notificationType)` arrives after the record was marked read, it creates a **new unread record** rather than incrementing the read one.

**Rationale**:
- From the user's perspective, "mark as read" means "I have seen this notification group." A new event arriving later is genuinely new information and should re-notify.
- This matches how email clients handle conversations: reading a thread does not prevent new messages from appearing as unread.
- Implementation is simpler: `MarkRead` operates on a single record ID, same as today.

**Consequences**:
- The dedup logic in `Append()` must check `IsRead` on the existing record. If the existing record is read, insert a new unread record instead of updating the read one.
- The `GetUnreadCount` method correctly reflects distinct actionable groups.

---

## Story 1: Server-Side Dedup in NotificationHistoryStore

**Goal**: Eliminate duplicate records at the persistence layer so the JSON file and API responses contain at most one record per `(sessionId, notificationType)` unread group.

**Duration**: 1 week
**Files touched**: 5

### Task 1.1: Add Dedup Fields to NotificationRecord and Proto

**Scope**: Extend the data model to support occurrence tracking.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/store.go` -- add `OccurrenceCount int` and `LastOccurredAt *time.Time` to `NotificationRecord`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/session.proto` -- add `occurrence_count` (field 12) and `last_occurred_at` (field 13) to `NotificationHistoryRecord`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/notification_service.go` -- update `recordToProto()` to map new fields

**Acceptance Criteria**:
- `NotificationRecord` struct has `OccurrenceCount int` and `LastOccurredAt *time.Time` with JSON tags
- Proto regenerated (`make proto-gen`) with new fields
- `recordToProto()` populates `occurrence_count` and `last_occurred_at` from the Go struct
- Old JSON files without these fields load without error (zero-value defaults)
- `go build ./...` passes

### Task 1.2: Implement Dedup Logic in Store.Append

**Scope**: When a new record arrives, check if an unread record with the same `(sessionId, notificationType)` already exists. If so, update it in place rather than inserting.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/store.go` -- modify `Append()` and add `findUnreadDuplicate()` helper

**Implementation Details**:
```go
func (s *NotificationHistoryStore) Append(record *NotificationRecord) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Existing ID dedup check (unchanged)

    // NEW: Check for unread duplicate by (sessionId, notificationType)
    if existing := s.findUnreadDuplicate(record.SessionID, record.NotificationType); existing != nil {
        // Update existing record
        existing.OccurrenceCount++
        existing.LastOccurredAt = &record.CreatedAt
        existing.Message = record.Message         // Use latest message content
        existing.Metadata = record.Metadata       // Use latest metadata (e.g., approval_id)
        existing.Title = record.Title             // Use latest title
        // Move to front of slice (newest-first ordering)
        s.moveToFront(existing)
        return s.saveToDisk()
    }

    // No duplicate found -- insert as new record with count=1
    record.OccurrenceCount = 1
    now := record.CreatedAt
    record.LastOccurredAt = &now
    s.records = append([]*NotificationRecord{record}, s.records...)
    s.enforceRetention()
    return s.saveToDisk()
}
```

**Acceptance Criteria**:
- Calling `Append()` twice with same `(sessionId, notificationType)` results in one record with `OccurrenceCount=2`
- The consolidated record has the latest `Message`, `Metadata`, `Title`, and `LastOccurredAt`
- The consolidated record is moved to the front (newest-first)
- If the existing record is already read (`IsRead=true`), a new unread record is created instead
- `go test ./server/notifications/...` passes

### Task 1.3: Update GetUnreadCount for Deduped Records

**Scope**: `GetUnreadCount()` should return the number of distinct unread records (which now represents distinct groups after dedup), not a raw count that would double-count.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/store.go` -- no functional change needed (the existing implementation already counts unread records, and with dedup there is one record per group)

**Acceptance Criteria**:
- Verify via test that `GetUnreadCount()` returns the number of deduplicated unread groups
- With 3 approval events for session A and 2 for session B (all unread), `GetUnreadCount()` returns 2

### Task 1.4: Unit Tests for Dedup Behavior

**Scope**: Comprehensive test coverage for the new dedup logic.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/store_test.go` -- create or extend

**Test Cases**:
1. `TestAppendDedup_SameSessionAndType` -- two appends collapse into one record
2. `TestAppendDedup_DifferentSessions` -- two different sessions create two records
3. `TestAppendDedup_DifferentTypes` -- same session, different types create two records
4. `TestAppendDedup_ReadThenNew` -- read record is not updated; new unread record created
5. `TestAppendDedup_MetadataUpdated` -- latest metadata (approval_id) replaces old
6. `TestAppendDedup_OccurrenceCountIncrements` -- count goes from 1 to 2 to 3
7. `TestAppendDedup_MoveToFront` -- updated record is at index 0
8. `TestAppendDedup_BackwardCompatibility` -- old records with OccurrenceCount=0 load and display as 1
9. `TestGetUnreadCount_WithDedup` -- unread count reflects deduplicated groups

**Acceptance Criteria**:
- All 9 tests pass
- `go test -cover ./server/notifications/...` shows >= 80% coverage on store.go
- `go test ./server/...` passes (no regressions)

### Task 1.5: Migration of Existing Persisted Data

**Scope**: On startup, deduplicate any existing records in the JSON file that have the same `(sessionId, notificationType)` and are unread.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/store.go` -- add `deduplicateExisting()` called from `NewNotificationHistoryStore()` after `loadFromDisk()`

**Implementation Details**:
- After loading records, scan for unread duplicates by `(sessionId, notificationType)`
- Merge duplicates into the newest record, summing occurrence counts
- Save the deduplicated set back to disk

**Acceptance Criteria**:
- A JSON file with 5 duplicate unread records for session "foo" type APPROVAL_NEEDED is reduced to 1 record with `OccurrenceCount=5` after `NewNotificationHistoryStore()` returns
- Read records are not affected by migration
- `go test ./server/notifications/...` includes a migration test

---

## Story 2: Frontend Aggregation and Count Badge

**Goal**: The NotificationPanel renders one card per `(sessionId, notificationType)` group with an "xN" badge when N > 1. The header badge count reflects distinct groups.

**Duration**: 1 week
**Files touched**: 4

### Task 2.1: Add Client-Side Grouping Logic

**Scope**: Create a utility function that groups `NotificationHistoryItem[]` by `(sessionId, notificationType)` and returns one representative item per group with a count.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/utils/notificationGrouping.ts` -- new file

**Implementation Details**:
```typescript
export interface GroupedNotification {
  /** The representative notification (most recent in the group) */
  notification: NotificationHistoryItem;
  /** Number of occurrences in this group (from server occurrence_count, or client-side count) */
  count: number;
  /** IDs of all notifications in this group (for batch mark-read) */
  allIds: string[];
}

/**
 * Groups notifications by (sessionId, notificationType).
 * Uses server-provided occurrence_count when available,
 * falls back to client-side grouping for backward compatibility.
 */
export function groupNotifications(
  notifications: NotificationHistoryItem[]
): GroupedNotification[];
```

**Acceptance Criteria**:
- 3 notifications for session "foo" type "approval_needed" produce 1 group with count=3
- Groups are ordered by most recent `timestamp` of any member
- The representative notification is the most recent one (has latest metadata/approval_id)
- Notifications with different `notificationType` are separate groups

### Task 2.2: Render Aggregated Cards with Count Badge

**Scope**: Update `NotificationPanel.tsx` to use grouped notifications and show "xN" badges.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationPanel.tsx` -- use `groupNotifications()` on `notificationHistory` before mapping to JSX
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationPanel.module.css` -- add `.countBadge` style

**Implementation Details**:
- Import and call `groupNotifications(notificationHistory)` in the render
- For each `GroupedNotification`, render the existing card layout using `group.notification`
- When `group.count > 1`, show a badge like "x3" next to the type label
- The "Approve" / "Deny" buttons use the representative notification's `metadata.approval_id` (the latest one, which is most likely to still be live)

**CSS for count badge**:
```css
.countBadge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.25rem;
  height: 1.25rem;
  padding: 0 0.375rem;
  background-color: var(--text-secondary);
  color: var(--background);
  border-radius: 10px;
  font-size: 0.6875rem;
  font-weight: 600;
  margin-left: 0.25rem;
}
```

**Acceptance Criteria**:
- Panel renders one card per group, not one card per raw record
- Badge "x3" is visible when 3 occurrences exist
- Badge is not shown when count is 1
- Approve/Deny buttons use the latest approval_id from the group
- Existing mark-as-read, remove, and clear functionality still works

### Task 2.3: Update Unread Count in Header Badge

**Scope**: The header notification bell badge should reflect the number of distinct unread groups, not raw unread records.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx` -- update `getUnreadCount()` to count by group

**Implementation Details**:
- Import `groupNotifications` in the context
- `getUnreadCount()` should return `groupNotifications(notificationHistory.filter(n => !n.isRead)).length`
- Alternatively, if the server already deduplicates, the raw unread count from the server response is correct and no client change is needed. In that case, prefer using `history.unreadCount` from the `useNotificationHistory` hook.

**Acceptance Criteria**:
- Badge count = number of distinct unread `(sessionId, notificationType)` groups
- With 5 raw unread records that collapse into 2 groups, badge shows "2"
- Mark-all-read sets badge to 0

### Task 2.4: Handle Mark-Read for Grouped Notifications

**Scope**: When the user clicks a grouped notification or marks it read, mark all underlying record IDs as read.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/components/ui/NotificationPanel.tsx` -- update click handler to pass `group.allIds`
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx` -- ensure `markAsRead()` handles array of IDs

**Implementation Details**:
- When the user interacts with a grouped card (clicks "View Session", "Approve", or explicitly marks read), pass all `group.allIds` to `markAsRead()`
- The existing `MarkNotificationReadRequest` already accepts `repeated string notification_ids`, so no proto change is needed
- Note: After Story 1 dedup, there will typically be only one record per group, so `allIds` will usually have length 1. But the client-side grouping may still see multiple IDs from old pre-dedup data.

**Acceptance Criteria**:
- Clicking "View Session" on a group with 3 underlying IDs marks all 3 as read
- The group disappears from the unread list
- `go test ./server/...` passes (server already supports batch mark-read)

---

## Story 3: Toast Dedup and Real-Time Event Coalescing

**Goal**: Prevent duplicate toast notifications from firing when the same event type arrives for the same session in rapid succession, and coalesce real-time SSE events.

**Duration**: 0.5-1 week
**Files touched**: 3

### Task 3.1: Debounce Duplicate Toasts in useSessionNotifications

**Scope**: When a `NotificationEvent` arrives via SSE that matches a recently-shown toast for the same `(sessionId, notificationType)`, suppress the duplicate toast.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useSessionNotifications.ts` -- add a dedup cache (Map) tracking recently-shown `(sessionId, type)` keys with a 10-second TTL

**Implementation Details**:
```typescript
// Inside useSessionNotifications:
const recentToastKeys = useRef<Map<string, number>>(new Map());

const handleNotification = useCallback((event: NotificationEvent) => {
  const dedupKey = `${event.sessionId}:${event.notificationType}`;
  const now = Date.now();
  const lastShown = recentToastKeys.current.get(dedupKey);

  if (lastShown && now - lastShown < 10_000) {
    // Suppress duplicate toast, but still update the history
    // (the server-side dedup in Story 1 handles the store)
    return;
  }
  recentToastKeys.current.set(dedupKey, now);

  // ... existing toast creation logic ...
}, [addNotification]);
```

**Acceptance Criteria**:
- Two approval events for the same session within 10 seconds produce only one toast
- The second event still reaches the store (server handles dedup)
- After 10 seconds, a new event for the same key produces a new toast
- Different sessions or different event types are not affected

### Task 3.2: Server-Side Event Coalescing in Subscriber

**Scope**: The `notifications.StartSubscriber` goroutine should coalesce rapid-fire events for the same `(sessionId, notificationType)` before calling `store.Append()`, reducing disk I/O.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/server/notifications/subscriber.go` -- add a coalescing buffer with 500ms flush interval

**Implementation Details**:
- Maintain a `map[string]*NotificationRecord` keyed by `sessionId:notificationType`
- When an event arrives, upsert into the buffer
- Every 500ms, flush all buffered records to `store.Append()`
- On context cancellation, flush remaining records immediately

**Acceptance Criteria**:
- 10 events for the same `(sessionId, type)` arriving within 500ms result in one `store.Append()` call
- Events for different keys are flushed independently
- The subscriber still processes events in order (no reordering)
- `go test ./server/notifications/...` includes coalescing tests

### Task 3.3: Reconcile NotificationContext Real-Time + History

**Scope**: The `NotificationContext` currently merges real-time notifications (from `addNotification`) with backend history (from `useNotificationHistory`). After dedup, ensure that a real-time event and the corresponding backend record do not both appear.

**Files**:
- `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx` -- update the `useEffect` that hydrates from backend to deduplicate against real-time items by `(sessionId, notificationType)` key, not just by `id`

**Implementation Details**:
- In the merge `useEffect`, build a set of `${sessionId}:${notificationType}` keys from existing real-time items
- Skip backend items whose key already exists in the real-time set (the real-time one is more current)
- This prevents the same logical notification from appearing twice during the window between real-time arrival and the next history fetch

**Acceptance Criteria**:
- A real-time approval notification and its persisted counterpart (fetched on next history load) result in one card, not two
- The panel count is accurate after a history refresh
- No flicker or reordering when history data arrives

---

## Known Issues

### Bug Risk: Race Condition in Store.Append Dedup Index (SEVERITY: Medium)

**Description**: The `findUnreadDuplicate()` scan iterates `s.records` under the write lock, which is safe. However, if two goroutines call `Append()` concurrently for the same `(sessionId, type)`, the mutex serializes them correctly. The risk is minimal because the subscriber is a single goroutine, but the `SendNotification` RPC could theoretically be called concurrently for the same session.

**Mitigation**:
- The existing `sync.RWMutex` in `NotificationHistoryStore` serializes all writes. No additional locking needed.
- Add a unit test with `sync.WaitGroup` that calls `Append()` concurrently and verifies exactly one record exists.

**Files Likely Affected**:
- `server/notifications/store.go`

**Prevention Strategy**:
- Rely on the existing mutex (already held during `Append`)
- Add concurrent append test in Task 1.4

---

### Bug Risk: Stale approval_id After Dedup Merge (SEVERITY: High)

**Description**: When the dedup logic updates an existing record's metadata with the latest event's metadata, the `approval_id` in metadata changes. If the frontend had previously rendered a card with the old `approval_id` and the user clicks "Approve," the RPC call will fail because the old approval has already expired or been replaced.

**Mitigation**:
- Task 1.2 specifies that the merged record gets the **latest** metadata (including `approval_id`). The frontend renders from the store, so after the next history fetch, it sees the current `approval_id`.
- For the window between real-time toast (old ID) and store update (new ID), the `ResolveApproval` RPC will return an error. The existing error handling in `NotificationPanel.tsx` (line 53) already shows an alert: "Could not resolve approval -- it may have already timed out."
- No additional code change needed, but add a comment documenting this expected behavior.

**Files Likely Affected**:
- `web-app/src/components/ui/NotificationPanel.tsx`
- `server/services/notification_service.go`

**Prevention Strategy**:
- Always use latest metadata in dedup merge (Task 1.2)
- Document the expected "stale approval_id" error path
- Consider showing a softer error message: "This approval may have been superseded. Check the session for the latest request."

---

### Bug Risk: Subscriber Coalescing Buffer Memory Leak (SEVERITY: Low)

**Description**: The coalescing buffer in Task 3.2 holds records in memory. If the flush timer is not properly stopped on context cancellation, or if events arrive for sessions that are deleted, the buffer could grow.

**Mitigation**:
- The buffer is a bounded map (one entry per unique key). With typical usage (< 50 active sessions x ~10 notification types), the map stays under 500 entries.
- The flush goroutine must `defer flush()` on context cancellation.
- The existing `enforceRetention()` in the store caps total records at 500.

**Files Likely Affected**:
- `server/notifications/subscriber.go`

**Prevention Strategy**:
- Use `defer` for final flush
- Add a max buffer size check (e.g., 1000 entries triggers immediate flush)
- Add test for context cancellation flushing

---

### Bug Risk: Client-Side Grouping Mismatch with Server Count (SEVERITY: Low)

**Description**: If the server sends `occurrence_count = 3` but the client's `groupNotifications()` also finds 3 separate records (from stale pre-dedup data), the displayed count could be wrong (showing "x3" from client grouping when the server says "x3" on a single record, or "x6" if both are combined).

**Mitigation**:
- After Story 1 migration (Task 1.5), old duplicates are consolidated on disk. The client will receive deduplicated records from the API.
- The client-side grouping function should prefer the server's `occurrence_count` when a record has one, and only fall back to client-side counting when multiple records exist for the same key (backward compatibility).
- Add a comment in `notificationGrouping.ts` explaining this precedence.

**Files Likely Affected**:
- `web-app/src/lib/utils/notificationGrouping.ts`
- `web-app/src/components/ui/NotificationPanel.tsx`

**Prevention Strategy**:
- Run migration on startup (Task 1.5) to eliminate old duplicates
- Client grouping uses `max(server_count, client_group_size)` as the display count

---

### Bug Risk: JSON File Size Regression During Migration (SEVERITY: Low)

**Description**: Task 1.5 runs deduplication and rewrites the JSON file on startup. If the store has 500 records with many duplicates, the rewrite is safe (file gets smaller). However, if `saveToDisk()` fails during migration (disk full, permissions), the old file with duplicates persists.

**Mitigation**:
- `saveToDisk()` already uses atomic write (temp file + rename), so a failure leaves the old file intact.
- On next startup, migration runs again against the same data -- idempotent.
- Log a warning if migration fails so operators are aware.

**Files Likely Affected**:
- `server/notifications/store.go`

**Prevention Strategy**:
- Atomic write pattern already in place
- Idempotent migration
- Warning log on failure

---

## Implementation Sequence

```
Story 1 (Server-Side Dedup)
  Task 1.1 -> Task 1.2 -> Task 1.3 -> Task 1.4 -> Task 1.5
                                         |
Story 2 (Frontend Aggregation)           |
  Task 2.1 -> Task 2.2 -> Task 2.3 -> Task 2.4
                                         |
Story 3 (Toast Dedup + Coalescing)       |
  Task 3.1 (independent)                 |
  Task 3.2 (depends on Story 1)          |
  Task 3.3 (depends on Story 2)   -------+
```

**Critical path**: Task 1.1 -> 1.2 -> 1.4 -> 2.1 -> 2.2 (server proto + dedup logic must exist before frontend can use `occurrence_count`)

**Parallelizable**:
- Task 3.1 (toast dedup) can start immediately -- it is a client-only change
- Task 2.1 (client grouping utility) can start as soon as proto types are generated (after Task 1.1)
- Task 1.4 (tests) and Task 1.5 (migration) can run in parallel

---

## Testing Strategy

### Unit Tests (Story 1)
- `server/notifications/store_test.go`: 9 test cases covering dedup, read/unread transitions, concurrent access, backward compatibility, migration

### Unit Tests (Story 2)
- `web-app/src/lib/utils/notificationGrouping.test.ts`: Grouping logic with various input combinations

### Integration Tests (Story 3)
- `server/notifications/subscriber_test.go`: Coalescing buffer flush behavior, context cancellation
- Manual E2E test: trigger 5 approval events for the same session, verify panel shows 1 card with "x5"

### Regression Tests
- `go test ./server/...` -- full backend test suite
- `make restart-web` -- verify panel renders correctly with deduped data
