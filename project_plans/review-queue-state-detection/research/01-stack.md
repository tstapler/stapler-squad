## Current Architecture

The review queue is a polling-based system with the following layers:

1. **Backend state machine**: `session/detection/` — `StatusDetector` + `IdleDetector` + `IdleState` classify each session's terminal output into discrete states.
2. **Poller**: `session/review_queue_poller.go` — `ReviewQueuePoller` runs on a 2s fast / 8s slow tick, calls `checkSession()` for each live instance, and calls `queue.Add()` / `queue.Remove()` based on classification.
3. **Queue storage**: `session/queue/queue.go` — in-memory `ReviewQueue` (map + observer pattern) with `ReviewItem` structs.
4. **Service layer**: `server/services/review_queue_service.go` — `ReviewQueueService` exposes `GetReviewQueue` (unary) and `WatchReviewQueue` (streaming SSE) via ConnectRPC.
5. **Frontend**: `useReviewQueue.ts` + Redux `reviewQueueSlice.ts` — WebSocket stream (`WatchReviewQueue`) with a 30s fallback poll; `ReviewQueuePanel.tsx` renders items.

## State Detection Today

The three-tier detection model is already implemented:

| Type | Location | Values |
|---|---|---|
| `DetectedStatus` | `session/detection/detector.go` | StatusUnknown, StatusReady, StatusProcessing, StatusNeedsApproval, StatusInputRequired, StatusError, StatusTestsFailing, StatusIdle, StatusActive, StatusSuccess |
| `IdleState` | `session/detection/idle.go` | IdleStateUnknown, IdleStateActive, IdleStateWaiting, IdleStateTimeout |
| `AttentionReason` | `session/queue/queue.go` | ReasonApprovalPending, ReasonInputRequired, ReasonErrorState, ReasonIdle, ReasonStale, ReasonTaskComplete, ReasonUncommittedChanges |

**How state is routed to the queue** (`checkSession`, lines 602–1215 of `review_queue_poller.go`):
- `StatusActive` / `StatusProcessing` → `queue.Remove()` immediately (session is working)
- `IdleStateActive` → `queue.Remove()` (controller signals activity)
- `IdleStateTimeout` → `queue.Add(ReasonIdle)`
- `timeSinceOutput > StalenessThreshold (2m)` → `queue.Add(ReasonStale)`
- `StatusNeedsApproval` / `StatusInputRequired` / `StatusError` → `queue.Add(...)` with appropriate reason

**`detectProcessing()` helper** (lines 477–521): looks at 4 signals before removing from queue after user responded — `StatusActive`/`StatusProcessing`, `IdleStateActive`, `LastMeaningfulOutput < 2s`, or string-match for "Thinking...", "esc to interrupt", etc. in last 50 lines.

**Source of content for detection**:
- Sessions with active `ClaudeController`: PTY circular buffer (in-memory, no subprocess), last 4096 bytes
- Sessions without controller: tmux `capture-pane` subprocess, cached by `pane_last_activity` timestamp

## Key Files

| File | Role |
|---|---|
| `session/detection/detector.go` | `StatusDetector` — regex-based, ANSI-stripped, priority-ordered matching |
| `session/detection/idle.go` | `IdleDetector` — maps `DetectedStatus` → `IdleState`, debounces, tracks `lastActivity` |
| `session/review_queue_poller.go` | Central orchestration: `checkSession()` decision tree, `getContent()` cache, `detectProcessing()` |
| `session/review_state.go` | `ReviewState` struct embedded in `Instance` — timestamps, signatures, prompt tracking |
| `session/queue/queue.go` | `ReviewQueue` in-memory store + observer notifications |
| `session/status_mapping.go` | Explicit mapping table: `DetectedStatus → AttentionReason`, `DetectedStatus → Status` |
| `session/claude_status_patterns.yaml` | YAML override for detection patterns (minimal: only approval, error, active) |
| `session/claude_controller.go` | `ClaudeController.Start()` — wires idle detector to PTY stream via `SetOnOutput` |
| `server/services/review_queue_service.go` | ConnectRPC handlers: `GetReviewQueue`, `WatchReviewQueue`, `AcknowledgeSession` |
| `web-app/src/lib/hooks/useReviewQueue.ts` | React hook: WatchReviewQueue stream + 30s fallback poll |
| `web-app/src/components/sessions/ReviewQueuePanel.tsx` | UI panel with filters, priority badges, approve/deny/skip actions |
| `web-app/src/lib/store/reviewQueueSlice.ts` | Redux slice: setReviewQueue, removeItem, stats |

## Gaps Found

1. **"Working" state not surfaced via proto**: `DetectedStatus.StatusActive` and `IdleStateActive` exist in Go but there is no `is_working: bool` or `working_state` field on the proto `Session` or `ReviewItem`. The review queue simply removes items when working; the UI has no way to show "this session is actively working right now."

2. **Stale + working = false positive**: The staleness check (`timeSinceOutput > 2m`) runs AFTER the `StatusActive` / `IdleStateActive` short-circuit return — but only for sessions with an active controller. For sessions without a controller, the `StatusActive` path does `queue.Remove()` and returns early, but if detection fails (ANSI noise, pattern change), the session falls through to the staleness check and gets flagged stale despite actively working.

3. **`detectProcessing()` is string-based, not regex**: The 8 hardcoded strings in `detectProcessing()` (e.g. "Thinking...", "Working...") duplicate the pattern logic in `StatusDetector` and don't benefit from ANSI stripping.

4. **No "working" field propagated to frontend**: `ReviewItem.Status` is the lifecycle `session.Status` string (Running/Ready/etc.), not the fine-grained detected status. The UI cannot distinguish "idle" from "actively working" without polling separately.

5. **Recency detection relies on `LastMeaningfulOutput` timestamp**, but this is updated only by WebSocket streaming and `HasUpdated()` — not by the PTY buffer directly for controller-managed sessions. There's a dual-path inconsistency: controller sessions use `IdleDetector.lastActivity` (driven by PTY bytes), while no-controller sessions use `pane_last_activity` from tmux.

6. **`claude_status_patterns.yaml` is a stub**: The file deliberately suppresses most patterns ("Disable everything else — if none of the above match, it's idle/done"). The richer `getDefaultPatterns()` in `detector.go` is what's actually used unless the YAML file is explicitly loaded.
