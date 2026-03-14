# Feature Plan: Notification History Persistence

**Date**: 2026-03-14
**Status**: Draft
**Scope**: Persist notification history to disk with rolling retention, provide a backend API, and surface history in the web UI across restarts

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Notification System](#research-findings-current-notification-system)
- [ADR-016: Notification History Storage Format](#adr-016-notification-history-storage-format)
- [ADR-017: Notification Capture Strategy](#adr-017-notification-capture-strategy)
- [Architecture Overview](#architecture-overview)
- [Data Model](#data-model)
- [Story 1: Backend Notification Store](#story-1-backend-notification-store)
- [Story 2: EventBus Integration and Capture](#story-2-eventbus-integration-and-capture)
- [Story 3: ConnectRPC API Endpoints](#story-3-connectrpc-api-endpoints)
- [Story 4: Frontend Notification History](#story-4-frontend-notification-history)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

Currently, notifications in Claude Squad are **ephemeral**. They exist in two disconnected layers:

1. **Backend**: Events are published to the `EventBus` and streamed to connected clients via `WatchSessions`. Once delivered, they are gone. If no client is connected, the event is lost.
2. **Frontend**: The `NotificationContext` accumulates a `notificationHistory` array in React state, but this is purely in-memory. A browser refresh or server restart wipes all history. The `notificationStorage.ts` utility in localStorage only tracks deduplication metadata (session IDs and timestamps for grace periods), not the notification content itself.

This means:
- Users cannot review past notifications after dismissing a toast or refreshing the page.
- If the web UI was not open when a notification fired, it is permanently lost.
- There is no audit trail of what happened and when across sessions.
- The existing `NotificationPanel` component shows "No notifications yet" after every page reload.

**User Goals:**
1. See a history of all notifications that have occurred, even after page refresh or server restart.
2. Browse and filter notification history by type, priority, and session.
3. Mark notifications as read and clear old ones.
4. Have old notifications automatically pruned (rolling retention: 500 entries or 7 days).

---

## Research Findings: Current Notification System

### Notification Sources (Backend)

There are **three** distinct notification-producing paths in the backend:

**Source 1: Approval Requests** (`server/services/approval_handler.go`)
- When Claude Code fires a `PermissionRequest` HTTP hook, `ApprovalHandler.HandlePermissionRequest()` creates a `PendingApproval` and calls `broadcastApprovalNotification()`.
- This constructs an `events.Event` of type `EventNotification` with `NotificationType = APPROVAL_NEEDED` and `NotificationPriority = URGENT`.
- Published to the `EventBus` via `h.eventBus.Publish(event)`.

**Source 2: External Notifications** (`server/services/session_service.go`, line 1545)
- The `SendNotification` RPC allows tmux sessions or external tools to push arbitrary notifications.
- It validates localhost origin, applies rate limiting (10/sec per session), generates a UUID notification ID, and publishes an `EventNotification` event to the `EventBus`.
- Supports all `NotificationType` and `NotificationPriority` values.

**Source 3: Review Queue Events** (`server/review_queue_manager.go`)
- `ReactiveQueueManager.OnItemAdded()` fires `ReviewQueueItemAddedEvent` through the dedicated `WatchReviewQueue` stream.
- These are NOT routed through the `EventBus` notification path -- they go through a separate `reviewQueueStreamClient` channel system.
- The frontend `useReviewQueueNotifications` hook generates client-side notifications from these events.

### Event Flow Architecture

```
[Approval Handler] ---> EventBus ---> WatchSessions stream ---> useSessionService hook
                                                             |
                                                             v
[SendNotification RPC] ---> EventBus -+              useSessionNotifications hook
                                       |                     |
                                       v                     v
                              (other subscribers)    NotificationContext (in-memory)
                                                             |
                                                             v
                                                     NotificationPanel (UI)

[Review Queue Poller] ---> ReviewQueue ---> WatchReviewQueue stream
                                                     |
                                                     v
                                           useReviewQueue hook
                                                     |
                                                     v
                                         useReviewQueueNotifications hook
                                                     |
                                                     v
                                           NotificationContext (in-memory)
```

### Frontend Notification Infrastructure

**`NotificationContext.tsx`**: Central React context that manages:
- `notifications: NotificationData[]` -- active toast notifications
- `notificationHistory: NotificationHistoryItem[]` -- in-memory history (lost on refresh)
- `isPanelOpen: boolean` -- slide-out panel visibility
- Methods: `addNotification`, `markAsRead`, `markAllAsRead`, `removeFromHistory`, `clearHistory`, `getUnreadCount`

**`NotificationPanel.tsx`**: Slide-out panel with notification history list. Currently renders from the in-memory `notificationHistory` array. Shows unread badges, priority colors, type icons, "View Session" links, and timestamps.

**`notificationStorage.ts`**: localStorage utility for **deduplication only** (not content storage). Tracks `NotificationRecord` entries with `sessionId`, `notifiedAt`, `acknowledgedAt`. Uses 1-hour TTL and 5-minute grace period to prevent duplicate alerts on WebSocket reconnection.

**`NotificationToast.tsx`**: Toast component with approve/deny buttons for approval-type notifications, "View Session" and "Dismiss" actions, auto-close after 8 seconds.

### Existing Storage Patterns

The project uses **JSON file storage** with file locking for session state (`config/state.go`):
- `~/.claude-squad/state.json` for UI state
- `~/.claude-squad/instances.json` for session instances
- File locking via `github.com/gofrs/flock`
- Atomic writes with write-then-rename pattern
- Instance-level isolation via workspace hashing

The SQLite migration was explicitly **DEFERRED** (see `docs/tasks/repository-pattern-sqlite-migration.md`) with the assessment that JSON storage is production-ready for current scale.

---

## ADR-016: Notification History Storage Format

### Status: Proposed

### Context

We need to persist notification history to disk. The options are:

1. **JSON file** (append-only with periodic compaction)
2. **SQLite database**
3. **Embedded key-value store** (bbolt/badger)

### Decision

**Use a JSON file** (`~/.claude-squad/notifications.json`) with the following rationale:

**Consistency**: The rest of the application uses JSON files for persistence (`state.json`, `instances.json`). Introducing a different storage backend for one feature creates tooling and maintenance burden.

**Scale**: The maximum size is 500 entries with 7-day TTL. At approximately 500 bytes per notification record, the maximum file size is ~250KB. This is trivially small for JSON read/write.

**Simplicity**: No additional dependencies. The project already has JSON serialization, file locking, and atomic write patterns that can be reused.

**SQLite deferred**: The team has explicitly deferred SQLite migration (`docs/tasks/repository-pattern-sqlite-migration.md`). Adding it for a single feature contradicts that decision.

**Operational simplicity**: JSON files are human-readable, easily inspectable with `cat`/`jq`, and trivially backed up. This aligns with the project's debugging philosophy (debug snapshots are also JSON).

### Alternatives Rejected

- **SQLite**: Adds a C dependency (CGo) or requires pure-Go SQLite (modernc.org/sqlite). Over-engineered for 500 records. Contradicts the deferred migration decision.
- **bbolt/badger**: Adds binary dependency, opaque storage format. Not inspectable. Over-engineered for this scale.
- **localStorage only**: Already exists for deduplication. Not accessible from backend, lost when clearing browser data, no server-side durability.

### Consequences

- JSON read/write for the entire file on each save operation (acceptable at 250KB max).
- Must implement file locking to prevent corruption from concurrent access.
- Rolling retention must be applied on write (trim to 500 entries, prune entries older than 7 days).
- File format is forward-compatible: adding fields to notification records is non-breaking.

---

## ADR-017: Notification Capture Strategy

### Status: Proposed

### Context

Notifications come from two distinct backend paths (EventBus notifications and ReviewQueue stream events) and one client-side path (review queue notifications generated by `useReviewQueueNotifications`). We must decide where to intercept and persist them.

### Decision

**Capture at the EventBus level in the backend** with the following design:

1. Create a `NotificationHistoryStore` that subscribes to the `EventBus`.
2. Filter for `EventNotification` events and persist them.
3. For review queue events: add a new `EventNotification` emission in `ReactiveQueueManager.OnItemAdded()` so that review queue additions are also captured by the EventBus subscriber. This unifies the notification path.
4. The `NotificationHistoryStore` writes to `~/.claude-squad/notifications.json`.

This means **all notifications go through the EventBus**, which becomes the single capture point. The store persists them regardless of whether any web client is connected.

### Alternatives Rejected

- **Client-side persistence** (expand `notificationStorage.ts`): Would not capture notifications when the browser is closed. Tied to a single browser instance.
- **Dual capture** (backend for EventBus + frontend for review queue): Complex, requires merge logic, risk of duplicates and ordering issues.
- **Intercept at the service layer** (modify `SendNotification`, `HandlePermissionRequest`, etc.): Scattered capture points, easy to miss new notification sources, violates single responsibility.

### Consequences

- `ReactiveQueueManager.OnItemAdded()` must emit an `EventNotification` in addition to the `ReviewQueueItemAddedEvent` it already publishes. This adds one new `EventBus.Publish()` call.
- The frontend can fetch persisted history from the backend API on page load, replacing the current in-memory-only `notificationHistory`.
- The `NotificationPanel` and `NotificationContext` will be updated to hydrate from the backend on mount.

---

## Architecture Overview

```
                         Backend (Go)
  +--------------------------------------------------------------+
  |                                                              |
  |  [ApprovalHandler] ---> EventBus <--- [SendNotification RPC] |
  |                           |                                  |
  |                           |  EventNotification               |
  |                           v                                  |
  |            +---NotificationHistoryStore---+                  |
  |            |  - Subscribe to EventBus     |                  |
  |            |  - Filter EventNotification  |                  |
  |            |  - Append to history         |                  |
  |            |  - Enforce retention limits  |                  |
  |            |  - Persist to JSON file      |                  |
  |            +-------|--------|------+------+                  |
  |                    |        |      |                         |
  |            Read    |  Write |   Mark Read                   |
  |                    v        v      v                         |
  |           ~/.claude-squad/notifications.json                |
  |                                                              |
  |  [ReactiveQueueManager] ---> EventBus (new Publish call)    |
  |                                                              |
  |  New ConnectRPC RPCs:                                        |
  |    - GetNotificationHistory                                  |
  |    - MarkNotificationRead                                    |
  |    - ClearNotificationHistory                                |
  +--------------------------------------------------------------+

                         Frontend (React)
  +--------------------------------------------------------------+
  |                                                              |
  |  NotificationContext (updated)                               |
  |    - On mount: fetch history from GetNotificationHistory     |
  |    - On new event: append to state + backend already stored  |
  |    - markAsRead: call MarkNotificationRead RPC               |
  |    - clearHistory: call ClearNotificationHistory RPC         |
  |                                                              |
  |  NotificationPanel (updated)                                 |
  |    - Renders persisted history (survives refresh)            |
  |    - Filter by type/priority                                 |
  |    - "Load more" for pagination                             |
  |                                                              |
  +--------------------------------------------------------------+
```

---

## Data Model

### Notification Record (Go struct)

```go
// NotificationRecord is the persisted representation of a notification event.
type NotificationRecord struct {
    ID               string            `json:"id"`
    SessionID        string            `json:"session_id"`
    SessionName      string            `json:"session_name"`
    NotificationType int32             `json:"notification_type"` // Maps to sessionv1.NotificationType
    Priority         int32             `json:"priority"`          // Maps to sessionv1.NotificationPriority
    Title            string            `json:"title"`
    Message          string            `json:"message"`
    Metadata         map[string]string `json:"metadata,omitempty"`
    CreatedAt        time.Time         `json:"created_at"`
    IsRead           bool              `json:"is_read"`
    ReadAt           *time.Time        `json:"read_at,omitempty"`
}
```

### JSON File Format

```json
{
  "version": 1,
  "updated_at": "2026-03-14T10:30:00Z",
  "notifications": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "session_id": "my-feature-branch",
      "session_name": "my-feature-branch",
      "notification_type": 1,
      "priority": 4,
      "title": "Permission Required: Bash",
      "message": "npm test",
      "metadata": {"approval_id": "abc-123", "tool_name": "Bash"},
      "created_at": "2026-03-14T10:30:00Z",
      "is_read": false
    }
  ]
}
```

### Protobuf Messages (new)

```protobuf
// NotificationHistoryRecord represents a persisted notification.
message NotificationHistoryRecord {
  string id = 1;
  string session_id = 2;
  string session_name = 3;
  NotificationType notification_type = 4;
  NotificationPriority priority = 5;
  string title = 6;
  string message = 7;
  map<string, string> metadata = 8;
  google.protobuf.Timestamp created_at = 9;
  bool is_read = 10;
  google.protobuf.Timestamp read_at = 11;
}

message GetNotificationHistoryRequest {
  optional int32 limit = 1;           // Default 50, max 500
  optional int32 offset = 2;          // For pagination
  optional NotificationType type_filter = 3;
  optional NotificationPriority priority_filter = 4;
  optional string session_id = 5;     // Filter by session
  optional bool unread_only = 6;      // Only unread notifications
}

message GetNotificationHistoryResponse {
  repeated NotificationHistoryRecord notifications = 1;
  int32 total_count = 2;
  int32 unread_count = 3;
  bool has_more = 4;
}

message MarkNotificationReadRequest {
  repeated string notification_ids = 1;  // Empty = mark all as read
}

message MarkNotificationReadResponse {
  bool success = 1;
  int32 marked_count = 2;
}

message ClearNotificationHistoryRequest {
  optional string before_timestamp = 1;  // Clear notifications older than this (RFC3339)
}

message ClearNotificationHistoryResponse {
  bool success = 1;
  int32 cleared_count = 2;
}
```

---

## Story 1: Backend Notification Store

**As** the Claude Squad server, **I want** to persist notification events to a JSON file with rolling retention, **so that** notification history survives server restarts and is available to all clients.

### Acceptance Criteria (Given-When-Then)

- **Given** the server is running, **when** a notification event is published to the EventBus, **then** it is appended to `~/.claude-squad/notifications.json` within 1 second.
- **Given** the notifications file has 500 entries, **when** a new notification arrives, **then** the oldest entry is removed before the new one is appended.
- **Given** the notifications file has entries older than 7 days, **when** a new notification arrives, **then** expired entries are pruned.
- **Given** the notifications file does not exist, **when** the first notification arrives, **then** the file is created with proper permissions (0644).
- **Given** two server processes write concurrently, **when** both try to persist, **then** file locking prevents corruption.

### Task 1.1: NotificationHistoryStore Implementation

**Files**: `server/notifications/store.go`, `server/notifications/store_test.go`

Create the `NotificationHistoryStore` struct with:
- `Append(record *NotificationRecord) error` -- add a notification, enforce retention
- `List(opts ListOptions) ([]*NotificationRecord, int, error)` -- paginated read with filters
- `MarkRead(ids []string) (int, error)` -- mark specific notifications as read
- `MarkAllRead() (int, error)` -- mark all as read
- `Clear(before *time.Time) (int, error)` -- remove notifications, optionally before a timestamp
- `GetUnreadCount() int` -- fast unread count

Implementation details:
- Read entire file into memory on startup (max 250KB)
- Keep an in-memory copy for fast reads
- Write to disk on mutation with file locking (`gofrs/flock`)
- Enforce 500-entry limit and 7-day TTL on every write
- Use `sync.RWMutex` for thread-safe access
- Apply workspace-based isolation (use same `GetConfigDir()` pattern)

### Task 1.2: EventBus Subscriber

**Files**: `server/notifications/subscriber.go`, `server/notifications/subscriber_test.go`

Create the subscriber goroutine that:
- Subscribes to `EventBus` on startup
- Filters for `EventNotification` type events
- Converts `events.Event` to `NotificationRecord`
- Calls `store.Append()` to persist
- Uses debounced writes (batch writes every 500ms to reduce disk I/O when many notifications arrive simultaneously)
- Stops cleanly on context cancellation

---

## Story 2: EventBus Integration and Capture

**As** the notification system, **I want** all notification sources to emit events through the EventBus, **so that** the history store captures every notification regardless of source.

### Acceptance Criteria

- **Given** a review queue item is added (not from initial snapshot), **when** `OnItemAdded` fires with `isSnapshot=false`, **then** an `EventNotification` event is also published to the EventBus.
- **Given** an approval request arrives, **when** `broadcastApprovalNotification` fires, **then** the notification is captured by the history store (already works via EventBus).
- **Given** a `SendNotification` RPC is called, **when** the notification event is published, **then** it is captured by the history store (already works via EventBus).

### Task 2.1: Emit EventNotification from ReviewQueue Additions

**Files**: `server/review_queue_manager.go`

In `ReactiveQueueManager.OnItemAdded()`, when `isSnapshot` is false (real-time event, not initial snapshot), publish an additional `EventNotification` to the EventBus with:
- `NotificationType` mapped from the review item's `AttentionReason` (e.g., `APPROVAL_PENDING` -> `NOTIFICATION_TYPE_APPROVAL_NEEDED`, `TASK_COMPLETE` -> `NOTIFICATION_TYPE_TASK_COMPLETE`, etc.)
- `NotificationPriority` mapped from the review item's `Priority`
- `SessionID` and `SessionName` from the review item
- `Title` and `Message` derived from the review item's reason and context
- `NotificationID` as a deterministic string: `fmt.Sprintf("review-%s-%d", item.SessionID, item.DetectedAt.UnixMilli())`

This requires no new dependencies -- `ReactiveQueueManager` already has `rqm.eventBus`.

### Task 2.2: Wire NotificationHistoryStore into Server

**Files**: `server/server.go`

Initialize `NotificationHistoryStore` during server startup:
1. Create the store with the config directory path
2. Start the EventBus subscriber goroutine with the server context
3. Pass the store reference to the service that handles the new RPCs
4. Ensure the subscriber is cleaned up on graceful shutdown (context cancellation)

---

## Story 3: ConnectRPC API Endpoints

**As** a web UI client, **I want** API endpoints to fetch, mark-read, and clear notification history, **so that** the frontend can display persisted notifications.

### Acceptance Criteria

- **Given** the server has persisted notifications, **when** `GetNotificationHistory` is called with no filters, **then** the 50 most recent notifications are returned in reverse chronological order.
- **Given** there are 200 notifications, **when** `GetNotificationHistory` is called with `limit=20, offset=20`, **then** notifications 21-40 are returned with `has_more=true`.
- **Given** there are unread notifications, **when** `MarkNotificationRead` is called with specific IDs, **then** those notifications are marked as read and the updated count is returned.
- **Given** there are old notifications, **when** `ClearNotificationHistory` is called with a timestamp, **then** only notifications before that timestamp are removed.

### Task 3.1: Protobuf Definitions

**Files**: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`

Add the new message types and RPC methods to the proto definitions:
- `NotificationHistoryRecord` message in `types.proto`
- `GetNotificationHistory`, `MarkNotificationRead`, `ClearNotificationHistory` RPCs and their request/response messages in `session.proto`
- Run `make proto-gen` to regenerate Go and TypeScript code

### Task 3.2: RPC Handler Implementation

**Files**: `server/services/notification_history_service.go`

Implement the three RPC handlers as methods that can be added to `SessionService` (keeping the existing service pattern) or as a standalone struct:
- `GetNotificationHistory` -- delegate to `store.List()` with filter mapping from proto to internal types
- `MarkNotificationRead` -- delegate to `store.MarkRead()` or `store.MarkAllRead()` based on whether IDs are provided
- `ClearNotificationHistory` -- delegate to `store.Clear()` with optional timestamp parsing

Register handlers in `server/server.go`.

---

## Story 4: Frontend Notification History

**As** a user of the web UI, **I want** the notification panel to show persisted notification history that survives page refreshes, **so that** I can review past notifications at any time.

### Acceptance Criteria

- **Given** the user opens the web UI, **when** the page loads, **then** the notification panel shows the most recent 50 notifications from the server.
- **Given** the user has unread notifications, **when** they open the notification panel, **then** the unread count badge in the header reflects the server-side count.
- **Given** the user clicks "Mark all read", **when** the action completes, **then** the server persists the read state and the badge count resets to 0.
- **Given** the user clicks "Clear all", **when** the action completes, **then** the server removes all notifications and the panel shows empty state.
- **Given** new notifications arrive via the WatchSessions stream, **when** they appear as toasts, **then** they are also immediately visible in the notification panel (added to the existing server-fetched list).

### Task 4.1: useNotificationHistory Hook

**Files**: `web-app/src/lib/hooks/useNotificationHistory.ts`

Create a new React hook that:
- Fetches notification history from `GetNotificationHistory` on mount
- Exposes `markAsRead(ids: string[])` that calls `MarkNotificationRead` RPC
- Exposes `markAllAsRead()` that calls `MarkNotificationRead` with empty IDs
- Exposes `clearHistory()` that calls `ClearNotificationHistory` RPC
- Exposes `loadMore()` for pagination (increment offset, append results)
- Returns `{ notifications, unreadCount, loading, error, hasMore, markAsRead, markAllAsRead, clearHistory, loadMore, refresh }`

### Task 4.2: Update NotificationContext

**Files**: `web-app/src/lib/contexts/NotificationContext.tsx`

Modify `NotificationProvider` to:
- Use `useNotificationHistory` internally for the persistent history
- On mount, populate `notificationHistory` from the backend instead of starting empty
- When `addNotification` is called (from real-time events), prepend to the existing list with deduplication by notification ID (the backend already captured it; the frontend just needs to show it immediately without waiting for a re-fetch)
- When `markAsRead` / `clearHistory` is called, delegate to the hook's RPC methods so the backend persists the change
- Maintain backward compatibility: the same `useNotifications()` API is exposed to consumers

### Task 4.3: Update NotificationPanel for Pagination and Filters

**Files**: `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/NotificationPanel.module.css`

Update the panel to:
- Show a "Load more" button at the bottom if `hasMore` is true
- Show a loading spinner during initial fetch
- Handle error states gracefully (show cached data if available, error banner if not)
- Optional stretch: add filter dropdowns for notification type and priority

---

## Known Issues and Bug Risks

### Bug Risk 1: Duplicate Notifications from Review Queue Path [SEVERITY: Medium]

**Description**: When `OnItemAdded` is modified (Task 2.1) to emit an `EventNotification`, the frontend will receive the notification through two paths simultaneously: (a) the `WatchReviewQueue` stream (existing), processed by `useReviewQueueNotifications`, and (b) the `WatchSessions` stream (via EventBus), processed by `useSessionNotifications`. This could produce duplicate toast notifications and duplicate entries in the notification panel.

**Mitigation**:
- Use the notification ID as a deduplication key in `NotificationContext.addNotification()`. Before appending, check if a notification with the same ID already exists.
- In the `OnItemAdded` EventBus emission, use a predictable notification ID derived from the session ID and timestamp (e.g., `review-queue-{sessionID}-{timestamp}`) rather than a random UUID, so both paths can be reconciled.
- The existing `notificationStorage.ts` already prevents duplicate sounds and browser notifications via `shouldNotify()`. The dedup in `NotificationContext` handles duplicate panel entries.

**Files Likely Affected**:
- `web-app/src/lib/contexts/NotificationContext.tsx`
- `server/review_queue_manager.go`

**Prevention Strategy**:
- Add deduplication by notification ID in `addNotification()`
- Use deterministic IDs for review queue notifications
- Add integration test that verifies no duplicate entries appear

### Bug Risk 2: File Corruption on Crash During Write [SEVERITY: Medium]

**Description**: If the server process is killed (SIGKILL, OOM, power loss) during a write to `notifications.json`, the file could be left in a partial/corrupt state, causing all history to be lost on next startup.

**Mitigation**:
- Use atomic write pattern: write to a temporary file (`notifications.json.tmp`), then rename over the target. The rename is atomic on most filesystems.
- On startup, if the main file fails to parse, attempt to read from a backup file (`notifications.json.bak`) that was saved before the last write.
- This pattern is already used in the project's state management (`config/state.go`).

**Files Likely Affected**:
- `server/notifications/store.go`

**Prevention Strategy**:
- Write to temp file, sync, rename (atomic write)
- Keep one backup copy before each write
- On load, try main file first, then backup, then start empty
- Add test that simulates partial write and verifies recovery

### Bug Risk 3: Race Condition Between Read and Write in Store [SEVERITY: Medium]

**Description**: If a `GetNotificationHistory` RPC reads the in-memory notification list while the EventBus subscriber goroutine is appending a new notification, a data race could occur.

**Mitigation**:
- The `NotificationHistoryStore` uses `sync.RWMutex` -- reads acquire `RLock`, writes acquire full `Lock`.
- All public methods on the store must acquire the appropriate lock before accessing the internal slice.

**Files Likely Affected**:
- `server/notifications/store.go`

**Prevention Strategy**:
- Run tests with `-race` flag
- Review all public methods for proper lock discipline
- Use `go vet -race ./server/notifications/...` in CI

### Bug Risk 4: Memory Growth from Unbounded In-Memory History [SEVERITY: Low]

**Description**: If the retention enforcement fails or the limit is misconfigured, the in-memory notification list could grow unbounded.

**Mitigation**:
- Hard-code a maximum capacity constant (`MaxNotifications = 500`).
- Enforce the limit in the `Append` method before writing to disk.
- Add a startup validation that truncates the list if the loaded file exceeds the limit.

**Files Likely Affected**:
- `server/notifications/store.go`

**Prevention Strategy**:
- Defense-in-depth: enforce limit in both `Append()` and `loadFromDisk()`
- Add a benchmark test that appends 10,000 notifications and verifies memory stays bounded

### Bug Risk 5: Frontend State Divergence After Network Error [SEVERITY: Low]

**Description**: If the `MarkNotificationRead` RPC fails (network error), the frontend will show the notification as read but the backend still considers it unread. On next page load, the notification will appear unread again.

**Mitigation**:
- Use optimistic update with rollback on failure (same pattern used by `useApprovals` and `useReviewQueue`).
- Show a brief error toast if the RPC fails.
- On next page load, the server state is authoritative and the UI will re-sync.

**Files Likely Affected**:
- `web-app/src/lib/hooks/useNotificationHistory.ts`
- `web-app/src/lib/contexts/NotificationContext.tsx`

**Prevention Strategy**:
- Follow the existing optimistic update + rollback pattern
- The server state is always authoritative; page refresh re-syncs

### Bug Risk 6: Workspace Isolation Mismatch [SEVERITY: Low]

**Description**: If the user runs multiple Claude Squad instances in different workspaces, each instance has its own state directory (`~/.claude-squad/workspaces/{hash}/`). Notifications from one workspace would not appear in another workspace's notification history.

**Mitigation**:
- This is actually correct behavior -- workspace isolation is by design. Each workspace has its own sessions, so notifications should be scoped to that workspace.
- Document this behavior in the notifications panel (e.g., "Showing notifications for this workspace").

**Prevention Strategy**:
- Use the same `GetConfigDir()` / workspace isolation path used by other state files
- No special handling needed; this matches existing behavior

---

## Testing Strategy

### Unit Tests

**`server/notifications/store_test.go`**:
- Test `Append` with empty store, verify file created
- Test `Append` at capacity (500), verify oldest removed
- Test `Append` with expired entries, verify pruning
- Test `List` with no filters, with type filter, with priority filter, with session filter, with unread-only filter
- Test `List` with pagination (limit/offset)
- Test `MarkRead` with specific IDs
- Test `MarkAllRead`
- Test `Clear` with and without timestamp
- Test `GetUnreadCount`
- Test concurrent read/write (with `-race`)
- Test file corruption recovery (corrupt main file, valid backup)
- Test workspace isolation (different config dirs produce independent stores)

**`server/notifications/subscriber_test.go`**:
- Test that `EventNotification` events are captured
- Test that non-notification events are ignored
- Test that debounced writes batch correctly
- Test graceful shutdown (context cancellation)

### Integration Tests

- Test full flow: publish event to EventBus -> subscriber captures -> RPC returns it
- Test `MarkNotificationRead` RPC updates backend state and subsequent `GetNotificationHistory` reflects it
- Test review queue notification emission (Task 2.1) produces a capturable EventNotification

### Frontend Tests

- Test `useNotificationHistory` hook fetches on mount
- Test `NotificationContext` deduplication (same ID added twice, only one entry)
- Test `NotificationPanel` renders persisted notifications
- Test "Mark all read" calls RPC and updates UI
- Test "Clear all" calls RPC and shows empty state

---

## Dependency Graph

```
Task 1.1: NotificationHistoryStore
  |
  v
Task 1.2: EventBus Subscriber  ----+
  |                                 |
  v                                 v
Task 2.1: Review Queue EventBus    Task 2.2: Wire Store into Server
  emission                           |
                                     v
                              Task 3.1: Protobuf Definitions
                                     |
                                     v
                              Task 3.2: RPC Handler Implementation
                                     |
                                     v
                              Task 4.1: useNotificationHistory Hook
                                     |
                                     v
                              Task 4.2: Update NotificationContext
                                     |
                                     v
                              Task 4.3: Update NotificationPanel
```

### Execution Order

**Phase 1 -- Backend Foundation** (Tasks 1.1, 1.2, 2.1, 2.2):
- Can be developed and tested independently of the frontend.
- Task 1.1 is the foundation; Task 1.2 depends on it.
- Task 2.1 is independent of Task 1.x but should be done before Task 2.2.
- Task 2.2 wires everything together.

**Phase 2 -- API Layer** (Tasks 3.1, 3.2):
- Depends on Phase 1 being complete.
- Task 3.1 (proto definitions) must be done before 3.2.
- Run `make proto-gen` between 3.1 and 3.2 to generate code.

**Phase 3 -- Frontend Integration** (Tasks 4.1, 4.2, 4.3):
- Depends on Phase 2 being complete (needs generated TypeScript client code).
- Task 4.1 is the hook; 4.2 updates the context to use it; 4.3 updates the UI.
- Sequential dependency: 4.1 -> 4.2 -> 4.3.

### Integration Checkpoints

**Checkpoint 1** (after Phase 1): Verify notifications are being written to `~/.claude-squad/notifications.json`. Trigger an approval request or SendNotification and confirm the file is populated. Inspect with `cat ~/.claude-squad/notifications.json | jq .`.

**Checkpoint 2** (after Phase 2): Use `grpcurl` or the ConnectRPC web UI to call `GetNotificationHistory` and verify it returns persisted records. Mark one as read and re-fetch to confirm.

**Checkpoint 3** (after Phase 3): Run `make restart-web`. Open the web UI, trigger a notification, refresh the page, and confirm the notification panel still shows the notification. Test "Mark all read" and "Clear all" buttons.

---

## Success Metrics

1. **Persistence**: Notifications survive server restarts and page refreshes.
2. **Completeness**: All three notification sources (approvals, SendNotification RPC, review queue) are captured.
3. **Retention**: File size stays bounded (max 500 entries, max 7-day age).
4. **Latency**: `GetNotificationHistory` responds in under 10ms (in-memory read).
5. **Correctness**: No duplicate notifications in the panel after WebSocket reconnection.
6. **Disk Safety**: No data corruption on unclean shutdown (atomic writes + backup).
