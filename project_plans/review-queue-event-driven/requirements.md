# Review Queue Event-Driven Architecture

## Problem Statement

The review queue currently relies on a 2-second polling loop (`ReviewQueuePoller`) to discover that sessions need attention. This creates:
- Up to 2s lag before an approval prompt surfaces in the queue
- A polling loop that scales linearly with session count
- A design mismatch: the `ClaudeController` already observes PTY output in real time (via `RecordActivity()` on every PTY write), but nothing connects that real-time signal to the queue

A partial fix has already been applied (branch `stapler-squad-queue-not-detecting`): when the poller fetches fresh terminal content via `getContent()`, it now calls `UpdateTerminalTimestamps()` so `LastMeaningfulOutput` is updated and the acknowledgment snooze works correctly. This PR extends that fix with a proper event-driven architecture.

## Current Architecture (as-is)

```
PTY output → ClaudeController.RecordActivity() → idleDetector.lastActivity
                                                      ↕  (cache invalidation)
                                              ReviewQueuePoller (every 2s)
                                              → checkSession() for ALL sessions
                                              → getContent() → Preview()
                                              → status detection
                                              → add/remove from ReviewQueue
                                              → ReviewQueueObserver.OnItemAdded()
                                              → ReactiveQueueManager streams to frontend
```

The `ReactiveQueueManager` already handles some events immediately (approval responses, user interactions → `CheckSession()`). The missing link: **PTY status changes are not events — the poller must discover them by polling**.

## Target Architecture (to-be)

```
PTY output → ClaudeController.OnOutput callback
           → RecordActivity() [existing]
           → status detection (hash-cached, ~O(1) on cache hit)
           → if status changed: fire StatusChangeListener(newStatus, context)
                                    ↓
                         InstanceStatusManager
                                    ↓
                         ReactiveQueueManager.handleStatusChange()
                         → CheckSession(inst) immediately
                         → add/remove from ReviewQueue
                         → stream to frontend

Idle timer per controller:
  RecordActivity() → reset per-session idle timer
  timer fires → fire IdleListener(sessionID)
              → ReactiveQueueManager → CheckSession()

Poller retained as safety net (30s interval):
  - Staleness detection (2-min threshold: needs time-based check)
  - Uncommitted changes (needs git status I/O)
  - External/non-controller sessions
  - Reconciliation against tmux reality (already 30s)
```

## Requirements

### R1 — Controller Status Change Events (highest priority)
- R1.1: `ClaudeController` MUST call a registered `StatusChangeListener` when the detected terminal status changes (e.g., Active → NeedsApproval, Waiting → Success)
- R1.2: Status detection in the `OnOutput` callback MUST use the existing hash cache (`statusCache`) to avoid re-running detection when PTY content is unchanged
- R1.3: The listener MUST be called only when status actually changes (i.e., `newStatus != lastEmittedStatus`); spurious identical-status calls MUST be suppressed
- R1.4: The `ClaudeController` MUST NOT import `server/` packages — the listener is a plain Go function set via setter method (avoids circular imports)
- R1.5: `InstanceStatusManager` wires the controller's `StatusChangeListener` to the `ReactiveQueueManager` at session creation time

### R2 — Idle Timeout Events (high priority)
- R2.1: When a session's idle detector transitions to `IdleStateTimeout`, an event MUST be delivered to the `ReactiveQueueManager` without waiting for the next poll cycle
- R2.2: The idle timer MUST be reset on each `RecordActivity()` call so active sessions are never falsely flagged
- R2.3: Implementation MUST reuse the existing `IdleDetector` infrastructure (not duplicate it)
- R2.4: The debounce interval (`minActivityInterval = 500ms`) prevents timer thrashing on rapid PTY output

### R3 — Poller Retained as Safety Net
- R3.1: The `ReviewQueuePoller` MUST be retained but its `PollInterval` SHOULD be reduced from 2s → safety-net only when event-driven path is active
- R3.2: For sessions WITH an active `ClaudeController`, the poller MAY skip the fast-path check (rely on events) while still running the 30s reconciliation and staleness checks
- R3.3: For sessions WITHOUT a `ClaudeController` (external/attached sessions), the poller MUST continue its normal 2s scan
- R3.4: `SlowPollInterval` behavior (backing off when queue is empty) MUST be preserved

### R4 — No Circular Imports
- R4.1: `session` package MUST NOT import `server/`, `pkg/events`, or any package that imports `session`
- R4.2: The listener/callback pattern (plain Go `func` type) is the approved inter-package boundary
- R4.3: All event publishing to the `EventBus` happens in `server/` code (ReactiveQueueManager), not in `session/`

### R5 — Correctness and Regression Safety
- R5.1: The existing `IsAcknowledgedAfterOutput()` snooze behavior MUST be preserved
- R5.2: The existing content-signature dedup (`LastOutputSignature`) MUST continue to prevent spurious re-queuing
- R5.3: Approval events (`ReasonApprovalPending`) MUST surface within 1 second of Claude producing the approval prompt in the PTY
- R5.4: All existing `TestReviewQueue*` and `TestReviewQueuePoller*` tests MUST continue to pass
- R5.5: The `ReactiveQueueManager.handleEvent()` path for `EventApprovalResponse` and `EventUserInteraction` MUST be unchanged

## Out of Scope
- Removing the poller entirely (staleness and external sessions still need it)
- Uncommitted-changes detection via inotify (separate feature, high complexity)
- Frontend polling interval changes (frontend 30s fallback poll is fine)
- Changes to the `WatchReviewQueue` WebSocket stream protocol

## Success Criteria
1. A session with a fresh approval prompt appears in the review queue within 1 second (down from up to 2s)
2. All existing tests pass: `make test`
3. No new goroutine leaks (verify with `go test -race`)
4. Controller-managed sessions trigger queue checks purely via events; the poller's role is safety net only for these sessions
