## Existing Event Infrastructure

### ConnectRPC streaming
- `WatchSessions` (streaming): emits `SessionEvent` oneofs — `SessionCreatedEvent`, `SessionUpdatedEvent`, `SessionStatusChangedEvent`, etc. `SessionStatusChangedEvent` already carries `detected_status: optional string` and `detected_context: optional string` (proto fields 4 and 5 of `events.proto`).
- `WatchReviewQueue` (streaming): emits `ReviewQueueEvent` oneofs — `itemAdded`, `itemRemoved`, `itemUpdated`, `statistics`. This is the direct channel the review queue panel subscribes to.
- Both streams are registered in `server/server.go` and use WebSocket transport via `connectrpc_websocket.go`.

### Observer pattern (backend)
`ReviewQueue` has an observer interface:
```go
type ReviewQueueObserver interface {
    OnItemAdded(item *ReviewItem)
    OnItemRemoved(sessionID string)
    OnQueueUpdated(items []*ReviewItem)
}
```
`ReactiveQueueManager` subscribes to this and forwards events to WebSocket streams. The poller calls `queue.Add()` / `queue.Remove()` which triggers observers synchronously (but notifies after lock release).

### Session watcher goroutine
`ReviewQueuePoller.pollLoop()` drives all detection on a 2s/8s tick. `ClaudeController.responseStream.SetOnOutput()` calls `idleDetector.RecordActivity()` immediately on each PTY chunk (no polling for activity timestamping). The two paths are therefore: (a) event-driven activity recording via PTY bytes, and (b) polled state assessment every 2s.

### Configuration
`ReviewQueuePollerConfig` (in `review_queue_poller.go`) holds all thresholds:
- `PollInterval: 2s`, `SlowPollInterval: 8s`
- `IdleThreshold: 5s`, `InputWaitDuration: 3s`, `StalenessThreshold: 2m`
- `ReconcileInterval: 30s`

These are **not exposed in `config.json`** — they are hardcoded as Go defaults with no config.json override path. The `DaemonPollInterval` in `config.go` controls something different (legacy daemon polling).

## Proposed State Model

### Option A: Add `is_working` bool to proto `Session` and `ReviewItem`
Minimal change. Add to `types.proto`:
```protobuf
// True when the session is actively generating output (working state detected).
bool is_working = 50;
```
And to the `ReviewItem` message (already has a `Status string` field):
```protobuf
bool is_working = 20;
```
The frontend can then filter working sessions out of the displayed queue (or show a "working" badge instead of surfacing them as needing attention).

**Pro**: Backward-compatible, minimal proto surface, easy to backfill.
**Con**: Loses the nuance between `StatusActive`, `StatusProcessing`, and `IdleStateActive`.

### Option B: Add `working_state` enum to proto (recommended)
```protobuf
enum WorkingState {
  WORKING_STATE_UNSPECIFIED = 0;
  WORKING_STATE_ACTIVE = 1;      // "esc to interrupt" visible
  WORKING_STATE_PROCESSING = 2;  // "Thinking..." / tool_use patterns
  WORKING_STATE_IDLE = 3;        // idle prompt visible
  WORKING_STATE_WAITING = 4;     // waiting for user but not a blocked prompt
}
```
Add `working_state WorkingState = 50` to `Session` in `types.proto`.
Add `working_state WorkingState = 20` to `ReviewItem` in `session.proto` (currently in the GetReviewQueue response section).

**Pro**: Richer signal, frontend can show appropriate status indicator.
**Con**: More proto surface, requires `make generate-proto`.

### Option C: Derive from existing `detected_status` in `SessionStatusChangedEvent`
The field already exists on `SessionStatusChangedEvent` (proto field 4). The review queue could subscribe to this stream (backend pub/sub) and remove sessions that emit `detected_status = "Active"` or `"Processing"`. No new proto fields needed.

**Pro**: Zero new proto fields.
**Con**: Requires the review queue to subscribe to a second event stream; adds coupling between `SessionStatusChangedEvent` and `ReviewQueue`.

## Recommended Architecture

### Phase 1: Filter working sessions from queue (immediate fix)
The core bug is that `detectProcessing()` and the `StatusActive` / `IdleStateActive` short-circuit in `checkSession()` rely on the poller running at the right time. The fix is to make the `ClaudeController` push a working-state change event immediately on `RecordActivity()` or when the `IdleDetector` transitions from `IdleStateWaiting` → `IdleStateActive`.

**Concrete change**:
1. In `ClaudeController.responseStream.SetOnOutput()`, when `idleDetector.RecordActivity()` returns AND the current idle state is `IdleStateActive`, fire an event to the `ReviewQueue` to remove this session. Use the existing `ReviewQueueObserver` / `ReviewQueue.Remove()` directly from the controller via a registered callback.
2. This is already nearly true — `RecordActivity()` updates `lastActivity`; the poller reads this on next tick. The 2s polling delay is the only gap. Making removal push-based (on PTY output) closes this gap.

**Files to change**:
- `session/claude_controller.go`: Add `reviewQueue *ReviewQueue` field + `SetReviewQueue(rq)` setter; in `SetOnOutput` callback call `rq.Remove(sessionName)` when idle state transitions to Active.
- `session/review_queue_poller.go`: The existing `idleState == IdleStateActive → queue.Remove()` path remains as a reconcile fallback.

### Phase 2: Structured working-state events via ConnectRPC
Add `working_state WorkingState` to proto `Session` and populate it in:
- `server/adapters/instance_adapter.go` — map `IdleState.State` → proto `WorkingState`
- `server/services/review_queue_service.go` — populate `ReviewItem.working_state`
- `SessionStatusChangedEvent` — optionally carry `working_state` in addition to `detected_status`

Frontend (`useReviewQueue.ts`): filter items where `working_state == WORKING_STATE_ACTIVE` or `WORKING_STATE_PROCESSING` from the displayed queue. Or show a "working" label instead of raising attention.

### Phase 3: Configurable thresholds
Expose `ReviewQueuePollerConfig` fields via `config.json`:
```json
{
  "review_queue": {
    "poll_interval_ms": 2000,
    "staleness_threshold_ms": 120000,
    "idle_threshold_ms": 5000
  }
}
```
Wire through `config/config.go` → `server/server.go` → `ReviewQueuePollerConfig`.

## Integration Points

| Component | Change Needed |
|---|---|
| `session/detection/detector.go` | Add cost summary pattern, Ebbing pattern, `> ` prompt pattern |
| `session/detection/idle.go` | Consider moving `RecordActivity` to trigger `Remove()` immediately |
| `session/claude_controller.go` | Optionally wire review queue for push-based removal |
| `session/review_queue_poller.go` | No structural changes; `detectProcessing()` remains as fallback |
| `proto/session/v1/types.proto` | Add `WorkingState` enum + `working_state` field to `Session` |
| `proto/session/v1/session.proto` | Add `working_state` to `ReviewItem` message |
| `server/adapters/instance_adapter.go` | Populate `working_state` from `IdleState` |
| `server/services/review_queue_service.go` | Pass through `working_state` in `ReviewItem` |
| `web-app/src/lib/hooks/useReviewQueue.ts` | Filter/label items by `working_state` |
| `config/config.go` | Add `ReviewQueueConfig` struct with threshold fields |
