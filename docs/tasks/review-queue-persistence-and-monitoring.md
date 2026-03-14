# Review Queue Persistence and Background Monitoring

## Epic Overview

### Problem Statement

When the Claude Squad server restarts, sessions that were already waiting for approval (or
in other attention-requiring states) are not immediately detected. The review queue starts
empty and only populates after the background poller detects idle/attention states, which
relies on the session having valid timestamps and the poller's 2-second poll cycle completing.

More critically, the **HTTP hook-based approval system** (`ApprovalStore`) is entirely
in-memory. If a Claude Code session sends a `PermissionRequest` hook and then the
claude-squad server restarts before the user responds, the pending approval is **lost
forever**. The HTTP connection breaks, Claude Code receives no response, and the approval
prompt may remain stuck in the terminal with no way to resolve it from the web UI.

The user's specific experience was: a session was waiting for a prompt approval, but they
only received a notification **after** they manually connected to the session. This indicates
that the review queue's terminal-content-based detection is reactive to user connection
rather than proactive.

### Root Cause Analysis

1. **ApprovalStore is in-memory only** (`server/services/approval_store.go`): The `pending`
   map and `decisionCh` channels are not persisted to disk. Server restart = all pending
   approvals lost.

2. **No startup scan for pre-existing approval prompts**: When `BuildDependencies()`
   (`server/dependencies.go`) loads instances, it starts controllers and the poller, but
   there is no explicit "scan all running sessions for existing approval prompts" step.
   The poller's `checkSessions()` does run immediately at startup (line 112 of
   `review_queue_poller.go`), but this depends on:
   - The session having a non-zero `LastMeaningfulOutput` (otherwise it's skipped)
   - The controller being started and actively reading terminal content
   - The `detection.StatusDetector` successfully parsing the terminal buffer

3. **Terminal detection is content-based, not event-based**: The poller's `checkSession()`
   reads terminal content via `inst.Preview()` and runs pattern matching. If the terminal
   buffer has scrolled past the approval prompt, or the prompt uses a format not matched
   by `detection.StatusDetector`, the approval is invisible to the review queue.

4. **HTTP hook approval is a separate system from terminal detection**: The `ApprovalHandler`
   (`server/services/approval_handler.go`) creates `PendingApproval` records in the
   in-memory store when Claude Code calls the HTTP hook. The `ReviewQueuePoller` detects
   approvals through terminal content analysis. These two systems are not synchronized.
   A hook-based approval exists in `ApprovalStore` but may not be reflected in the review
   queue until the poller's next cycle detects the terminal prompt.

5. **Poller initial scan timing**: The `Start()` method of `ReviewQueuePoller` calls
   `checkSessions()` immediately (line 112), but this runs before controllers may have
   fully initialized. The controller startup happens in `BuildDependencies()` step 7,
   while the poller starts inside `ReactiveQueueManager.Start()` which runs concurrently
   via `go deps.ReactiveQueueMgr.Start(context.Background())` in `server.go` line 66.
   There is a race condition where the initial scan may run before controllers are ready.

### Success Metrics

- **SM-1**: On server restart, sessions with pending approval prompts appear in the review
  queue within 5 seconds (not dependent on web UI connection).
- **SM-2**: Pending HTTP hook approvals survive server restarts (the HTTP connection will
  still break, but the approval record is preserved for display and re-detection).
- **SM-3**: The review queue's startup scan detects approval prompts in terminal content
  even without active controllers.
- **SM-4**: Zero false negatives for approval detection: if a session is displaying an
  approval prompt, it must appear in the review queue within one poll cycle (2 seconds).

---

## Architecture Decision Records

### ADR-1: Startup Terminal Scan for Pre-Existing Approval Prompts

**Status**: Proposed

**Context**: When the server starts, sessions may already be in states requiring attention
(approval prompts, error states, input prompts). The current system relies on the
`ReviewQueuePoller`'s poll loop to detect these, but the initial poll may fire before
controllers are ready, and the no-controller fallback path requires terminal content
to be readable and pattern-matchable.

**Decision**: Add a dedicated **startup scan phase** in `BuildDependencies()` after
controller startup (step 7) but before `ReactiveQueueManager.Start()`. This scan will:

1. Iterate all started, non-paused instances
2. Read terminal content via `inst.Preview()`
3. Run `detection.StatusDetector.DetectWithContext()` on the content
4. For any session detected as `StatusNeedsApproval`, `StatusInputRequired`, or
   `StatusError`, immediately add a `ReviewItem` to the queue
5. Log the results for debugging

**Rationale**: This is the simplest approach that addresses the core problem. It reuses
existing detection infrastructure without introducing new persistence mechanisms. The
scan happens at a well-defined point in the startup sequence when all controllers
are initialized.

**Consequences**:
- (+) Immediate review queue population on startup
- (+) No new persistence layer needed for this specific fix
- (+) Reuses existing `StatusDetector` patterns
- (-) Still depends on terminal content being readable (tmux capture-pane)
- (-) Cannot detect approvals that have scrolled out of the terminal buffer

**Files Affected**:
- `server/dependencies.go` (add startup scan after step 7)

---

### ADR-2: ApprovalStore Disk Persistence for Restart Survival

**Status**: Proposed

**Context**: The `ApprovalStore` (`server/services/approval_store.go`) stores pending
approvals entirely in memory. When the server restarts, all pending approvals are lost.
Even though the HTTP connection breaks (so the original hook response cannot be sent),
the approval record is valuable for:

1. Displaying in the review queue that a session has a pending approval
2. Showing the tool name, input, and context in the web UI
3. Allowing the user to navigate to the terminal and manually respond

The `decisionCh` channel cannot be persisted (it is a Go channel), so persisted approvals
are "display-only" after restart -- they indicate that the session needs attention but
cannot directly resolve the hook.

**Decision**: Add file-based persistence to `ApprovalStore` using a JSON file in the
config directory (`~/.claude-squad/pending_approvals.json`). The store will:

1. Write to disk on every `Create()` call (append-style with file lock)
2. Remove from disk on `Resolve()`, `Remove()`, and `CleanupExpired()`
3. Load from disk on startup in `NewApprovalStore()` (or a new `LoadPersistedApprovals()`)
4. Mark loaded approvals as "orphaned" (no `decisionCh`, cannot resolve via hook)
5. Expose orphaned approvals in `ListPendingApprovals` with a flag indicating they
   need terminal-based resolution

**Rationale**: File-based persistence is consistent with the existing config/state storage
pattern in claude-squad. The Ent/SQLite database would be overkill for what is typically
0-5 records. JSON is human-readable for debugging. The "orphaned" concept cleanly separates
live hook approvals from historical records.

**Consequences**:
- (+) Pending approvals survive server restarts
- (+) Web UI shows approval context even after restart
- (+) Simple implementation using existing patterns
- (-) Orphaned approvals require terminal-based resolution (user must go to terminal)
- (-) Small disk I/O on each approval create/resolve
- (-) Need cleanup logic for stale orphaned approvals

**Files Affected**:
- `server/services/approval_store.go` (add persistence methods)
- `server/services/approval_handler.go` (mark orphaned approvals)
- `server/dependencies.go` (load persisted approvals on startup)

---

### ADR-3: Cross-System Approval Synchronization

**Status**: Proposed

**Context**: There are currently two independent systems that detect approval-needing
sessions:

1. **HTTP Hook System** (`ApprovalHandler` + `ApprovalStore`): Triggered by Claude Code's
   `PermissionRequest` HTTP hook. Creates `PendingApproval` records. Broadcasts
   `NOTIFICATION_TYPE_APPROVAL_NEEDED` events.

2. **Terminal Detection System** (`ReviewQueuePoller` + `StatusDetector`): Polls terminal
   content every 2 seconds. Detects `StatusNeedsApproval` from pattern matching. Adds
   `ReviewItem` to the queue.

These systems operate independently and may produce conflicting or duplicate signals.
The review queue service (`GetReviewQueue`) already enriches `APPROVAL_PENDING` items
with `pending_approval_id` metadata from the `ApprovalStore`, but this is a read-time
join, not a synchronized system.

**Decision**: Introduce a lightweight synchronization mechanism:

1. When `ApprovalHandler` creates a new approval, it also calls
   `reviewQueuePoller.CheckSession()` for immediate queue update (instead of waiting
   for the next poll cycle).

2. When the startup scan detects an approval prompt in a terminal, check
   `ApprovalStore` for a matching session. If a persisted (orphaned) approval exists,
   enrich the `ReviewItem` with the approval's metadata (tool name, input, etc.).

3. Add a `SyncApprovalState()` method to the poller that reconciles the two systems:
   - If ApprovalStore has an approval for a session but the queue does not, add it
   - If the queue has an approval item but the terminal no longer shows an approval
     prompt and the ApprovalStore has no record, remove it

**Rationale**: Full unification of the two systems would require significant refactoring
(see `docs/tasks/architecture-refactor.md`). A lightweight sync layer bridges the gap
without disrupting either system's existing behavior.

**Consequences**:
- (+) Consistent review queue state regardless of which system detects the approval
- (+) Richer context in review items (tool name, input from hook data)
- (+) Faster detection when hook fires (immediate vs 2-second poll)
- (-) Additional complexity in the poller
- (-) Potential for brief inconsistencies during sync windows

**Files Affected**:
- `server/services/approval_handler.go` (trigger immediate queue check)
- `session/review_queue_poller.go` (add `SyncApprovalState()`)
- `server/dependencies.go` (wire sync on startup)

---

## Story Breakdown

### Story 1: Startup Terminal Scan for Pre-Existing States

**As a** user who restarts the claude-squad server,
**I want** sessions with pending approval prompts to appear in the review queue immediately,
**So that** I do not have to manually check each session to find ones needing attention.

**INVEST Criteria**:
- **Independent**: Can be implemented without stories 2 or 3
- **Negotiable**: Scan timing and detected states are configurable
- **Valuable**: Directly addresses the reported user issue
- **Estimable**: Well-scoped to one function in one file
- **Small**: Single function addition with clear inputs/outputs
- **Testable**: Mock instances with known terminal content, verify queue population

**Acceptance Criteria**:
- Given a session displaying "Allow" / "Deny" approval prompt in its terminal
- When the server starts and completes initialization
- Then the session appears in the review queue with reason `approval_pending` within 5 seconds

- Given a session displaying an error message in its terminal
- When the server starts
- Then the session appears in the review queue with reason `error_state`

- Given a session that is actively processing (no prompt visible)
- When the server starts
- Then the session does NOT appear in the review queue

#### Task 1.1: Implement `scanSessionsOnStartup()` in `server/dependencies.go`

**Scope**: Add a function that runs after controller startup (step 7) and before
the reactive queue manager starts.

**Implementation Details**:
```go
func scanSessionsOnStartup(
    instances []*session.Instance,
    queue *session.ReviewQueue,
    statusManager *session.InstanceStatusManager,
) {
    detector := detection.NewStatusDetector()
    scanned, added := 0, 0
    for _, inst := range instances {
        if !inst.Started() || inst.Paused() { continue }
        scanned++
        
        // Try controller-based detection first
        statusInfo := statusManager.GetStatus(inst)
        if statusInfo.IsControllerActive {
            if statusInfo.ClaudeStatus == detection.StatusNeedsApproval ||
                statusInfo.ClaudeStatus == detection.StatusInputRequired ||
                statusInfo.ClaudeStatus == detection.StatusError {
                // Add to queue with appropriate reason/priority
                added++
            }
            continue
        }
        
        // Fallback: terminal content detection
        content, err := inst.Preview()
        if err != nil || content == "" { continue }
        status, context := detector.DetectWithContext([]byte(content))
        if status == detection.StatusNeedsApproval ||
            status == detection.StatusInputRequired ||
            status == detection.StatusError {
            // Map to ReviewItem and add to queue
            added++
        }
    }
    log.InfoLog.Printf("[StartupScan] Scanned %d sessions, added %d to review queue", scanned, added)
}
```

**Files**: `server/dependencies.go`

#### Task 1.2: Add integration test for startup scan

**Scope**: Test that `scanSessionsOnStartup` correctly populates the review queue
from terminal content containing approval prompts.

**Files**: `server/dependencies_test.go` (new file)

---

### Story 2: ApprovalStore Disk Persistence

**As a** user whose server restarts while a session has a pending approval,
**I want** the pending approval information to be preserved,
**So that** I can see which sessions need attention even after a restart.

**INVEST Criteria**:
- **Independent**: Can be implemented without stories 1 or 3
- **Negotiable**: Storage format and cleanup policy are flexible
- **Valuable**: Prevents loss of approval context across restarts
- **Estimable**: Clear data model and I/O patterns
- **Small**: Changes confined to approval_store.go + dependencies.go
- **Testable**: Create approvals, simulate restart, verify loaded state

**Acceptance Criteria**:
- Given a pending approval in the store
- When the server process is killed and restarted
- Then `ListPendingApprovals` returns the previously pending approval with an `orphaned` flag

- Given an orphaned approval for session "foo"
- When the user resolves the approval via the terminal
- Then the orphaned approval is removed from the store on next cleanup cycle

- Given 10 orphaned approvals older than 4 hours
- When the cleanup runs
- Then all 10 are removed (terminal has long since moved past the prompt)

#### Task 2.1: Add persistence methods to `ApprovalStore`

**Scope**: Add `persistToDisk()`, `loadFromDisk()`, `removeFromDisk()` methods.
Use a JSON file at `~/.claude-squad/pending_approvals.json`.

**Data Model for persisted approval**:
```go
type PersistedApproval struct {
    ID              string                 `json:"id"`
    SessionID       string                 `json:"session_id"`
    ClaudeSessionID string                 `json:"claude_session_id"`
    ToolName        string                 `json:"tool_name"`
    ToolInput       map[string]interface{} `json:"tool_input"`
    Cwd             string                 `json:"cwd"`
    PermissionMode  string                 `json:"permission_mode"`
    CreatedAt       time.Time              `json:"created_at"`
    ExpiresAt       time.Time              `json:"expires_at"`
    Orphaned        bool                   `json:"orphaned"`
}
```

**Files**: `server/services/approval_store.go`

#### Task 2.2: Load persisted approvals on startup

**Scope**: In `NewApprovalStore()` or `BuildDependencies()`, load persisted approvals,
mark them as orphaned (no `decisionCh`), and expose them in `ListAll()`.

**Files**: `server/services/approval_store.go`, `server/dependencies.go`

#### Task 2.3: Cleanup orphaned approvals

**Scope**: Extend `CleanupExpired()` (or add `CleanupOrphaned()`) to remove orphaned
approvals older than a configurable threshold (default: 4 hours). Also remove orphaned
approvals when the corresponding session's terminal no longer shows an approval prompt.

**Files**: `server/services/approval_store.go`, `server/services/approval_handler.go`

#### Task 2.4: Add unit tests for persistence lifecycle

**Scope**: Test create-persist-load-cleanup cycle, concurrent access, and edge cases
(corrupt JSON, missing file, permissions).

**Files**: `server/services/approval_store_test.go`

---

### Story 3: Cross-System Approval Synchronization

**As a** user viewing the review queue,
**I want** approval items to show rich context (tool name, command) from the HTTP hook,
**And** I want the queue to update immediately when Claude Code sends a hook,
**So that** I can quickly understand and act on pending approvals.

**INVEST Criteria**:
- **Independent**: Builds on story 2 but can be partially implemented alone
- **Negotiable**: Sync frequency and enrichment depth are adjustable
- **Valuable**: Improves approval UX with richer context and faster detection
- **Estimable**: Clear interface between hook handler and poller
- **Small**: Wire-up changes in 3 files
- **Testable**: Mock hook event, verify queue item enrichment

**Acceptance Criteria**:
- Given Claude Code sends a PermissionRequest hook for session "foo"
- When the hook handler creates the approval record
- Then the review queue is immediately updated (within 100ms, not waiting for next poll)

- Given an orphaned approval exists for session "foo" with tool_name="Bash"
- When the startup scan detects "foo" needs approval
- Then the review queue item includes metadata: tool_name="Bash"

- Given the terminal for session "foo" no longer shows an approval prompt
- And the ApprovalStore has no record for "foo"
- When the poller checks "foo"
- Then "foo" is removed from the review queue

#### Task 3.1: Trigger immediate queue check on hook approval creation

**Scope**: In `ApprovalHandler.HandlePermissionRequest()`, after creating the approval
and broadcasting the notification, trigger an immediate `reviewQueuePoller.CheckSession()`
call. This requires passing the poller reference to the handler.

**Files**: `server/services/approval_handler.go`, `server/dependencies.go`

#### Task 3.2: Enrich review queue items with ApprovalStore metadata

**Scope**: In `ReviewQueuePoller.checkSession()`, when adding an item with reason
`ReasonApprovalPending`, check the `ApprovalStore` for matching session approvals
and populate the `ReviewItem.Metadata` with tool name, command, and file path.

**Files**: `session/review_queue_poller.go`, `server/dependencies.go` (wire store to poller)

#### Task 3.3: Add startup sync of persisted approvals to review queue

**Scope**: In `BuildDependencies()`, after loading persisted (orphaned) approvals,
iterate them and add corresponding `ReviewItem` entries to the queue. This ensures
the queue reflects known approvals even before the first poll cycle completes.

**Files**: `server/dependencies.go`

---

## Known Issues

### Potential Bug: Race Condition in Startup Scan vs Controller Initialization [SEVERITY: Medium]

**Description**: The startup scan in `BuildDependencies()` may execute before all
controllers have fully initialized their terminal readers. The `inst.Preview()` call
in the scan requires the tmux session to be attached and the capture-pane command to
succeed. If a controller is still starting, the terminal buffer may be empty or
incomplete.

**Mitigation**:
- Add a short delay (500ms) between controller startup loop and the scan
- Or use a retry with backoff for sessions where `Preview()` returns empty content
- Log sessions where the scan produced empty content for debugging

**Files Likely Affected**:
- `server/dependencies.go`

**Prevention Strategy**:
- Run the scan after a brief settling period
- Make the scan idempotent (the poller will catch anything missed within 2 seconds)
- Log clearly which sessions were scanned and their detected states

---

### Potential Bug: Orphaned Approval Cleanup Removes Valid Approvals [SEVERITY: High]

**Description**: The cleanup logic for orphaned approvals uses a time threshold (e.g.,
4 hours). However, a user might legitimately leave a session in an approval-pending
state for hours (e.g., overnight). Cleaning up the orphaned approval record would remove
it from the web UI, making the user unaware that the session needs attention.

**Mitigation**:
- Use a generous default timeout (e.g., 4 hours instead of 30 minutes)
- Only remove orphaned approvals when the terminal content no longer shows an approval
  prompt (requires active terminal check, not just time-based)
- Keep a "recently cleaned" log for debugging

**Files Likely Affected**:
- `server/services/approval_store.go`
- `server/services/approval_handler.go`

**Prevention Strategy**:
- Require both conditions: time threshold exceeded AND terminal no longer shows prompt
- Make the threshold configurable
- Add a "keep" flag that users can set via the web UI to prevent cleanup

---

### Potential Bug: Duplicate Review Queue Items from Hook + Poller [SEVERITY: Medium]

**Description**: When a Claude Code hook fires, two systems may detect the approval:
1. The `ApprovalHandler` broadcasts an event, causing the `ReactiveQueueManager` to
   potentially trigger a queue update
2. The `ReviewQueuePoller` detects `StatusNeedsApproval` in the terminal content

If both fire simultaneously, the queue could briefly contain duplicate items or the
item could be added, removed, and re-added in rapid succession, causing UI flickering.

**Mitigation**:
- The `ReviewQueue.Add()` method already deduplicates by `SessionID` (uses a map)
- Add a debounce period after hook-triggered queue updates before the poller can modify
  the same session's queue entry
- The existing `minReAddInterval` (2 minutes) in the poller prevents rapid re-adds

**Files Likely Affected**:
- `session/review_queue_poller.go`
- `server/review_queue_manager.go`

**Prevention Strategy**:
- Verify that `ReviewQueue.Add()` is truly idempotent (check `queue.go`)
- Add integration test with concurrent hook and poller firing for same session
- Consider a "source" field on `ReviewItem` to track which system added it

---

### Potential Bug: JSON File Corruption from Concurrent Writes [SEVERITY: Medium]

**Description**: If multiple goroutines call `ApprovalStore.Create()` concurrently
(multiple Claude sessions sending hooks simultaneously), the file-based persistence
could experience write conflicts. The Go `os.WriteFile` is not atomic on all filesystems.

**Mitigation**:
- The `ApprovalStore` already has a `sync.RWMutex` that protects the in-memory map
- Extend the mutex to cover disk writes (persistence happens inside the lock)
- Use atomic write pattern: write to temp file, then rename (atomic on POSIX)

**Files Likely Affected**:
- `server/services/approval_store.go`

**Prevention Strategy**:
- Always persist inside the existing `mu.Lock()` block
- Use `os.CreateTemp` + `os.Rename` pattern for atomic writes
- Add file integrity check on load (JSON parse validation)

---

### Potential Bug: Stale Terminal Content After Server Restart [SEVERITY: Low]

**Description**: When claude-squad restarts, the tmux sessions continue running
independently. The terminal buffer captured by `inst.Preview()` may contain content
from before the restart mixed with new content. If the approval prompt was displayed
before the restart and the session has since moved on, the startup scan might detect
a false positive.

**Mitigation**:
- The startup scan should check for recency signals (cursor position, timestamp patterns)
- Cross-reference with `ApprovalStore` persisted data for confirmation
- The poller's subsequent checks will correct any false positives within 2 seconds

**Files Likely Affected**:
- `server/dependencies.go` (startup scan)
- `session/detection/detector.go` (pattern matching context)

**Prevention Strategy**:
- Focus detection on the last N lines of terminal output (e.g., last 50 lines)
- Weight detection confidence based on proximity to end of buffer
- Log false positive rate for monitoring

---

### Potential Bug: Memory Leak from Persisted Approval Channel References [SEVERITY: Low]

**Description**: Orphaned approvals loaded from disk do not have a `decisionCh` channel.
If code paths assume all `PendingApproval` instances have a non-nil `decisionCh` and
attempt to send on it, this would panic. Conversely, if orphaned approvals are mixed
into `GetBySession()` results, callers might attempt to resolve them, which would fail
because there is no HTTP handler waiting on the channel.

**Mitigation**:
- Add an `Orphaned bool` field to `PendingApproval`
- All resolution code paths must check `Orphaned` before sending on `decisionCh`
- The `Resolve()` method returns a clear error for orphaned approvals

**Files Likely Affected**:
- `server/services/approval_store.go`
- `server/services/approval_handler.go`

**Prevention Strategy**:
- Never create a `decisionCh` for orphaned approvals
- Add explicit nil-check for `decisionCh` in `Resolve()`
- Unit test: attempt to resolve an orphaned approval, expect graceful error

---

## Dependency Visualization

```
Story 1 (Startup Scan)                Story 2 (Persistence)
         |                                     |
         |  independent                        |  independent
         |                                     |
         +------------------+------------------+
                            |
                     Story 3 (Sync)
                            |
              depends on Story 1 + Story 2
              for full functionality,
              but Task 3.1 (immediate check)
              can be done independently
```

**Recommended Implementation Order**:
1. **Story 1** (startup scan) -- immediate value, lowest risk
2. **Story 2** (persistence) -- enables restart survival
3. **Story 3** (sync) -- ties everything together

Stories 1 and 2 can be developed in parallel by different engineers.

---

## Integration Checkpoints

### Checkpoint 1: After Story 1

**Verification**:
1. Start claude-squad with a session that has an approval prompt visible
2. Kill the server process (`pkill -f claude-squad`)
3. Restart the server (`make restart-web`)
4. Open the web UI -- the session should appear in the review queue within 5 seconds
5. Check logs for `[StartupScan]` entries showing detected states

**Regression Checks**:
- Existing review queue behavior unchanged for running server
- No double-entries in queue after startup scan + first poll cycle
- Paused sessions not incorrectly added to queue

### Checkpoint 2: After Story 2

**Verification**:
1. Create a session and trigger a hook approval (use `curl` to POST to
   `/api/hooks/permission-request`)
2. Verify the approval appears in `~/.claude-squad/pending_approvals.json`
3. Kill and restart the server
4. Call `ListPendingApprovals` -- the orphaned approval should appear
5. Verify the orphaned approval is cleaned up after the configured timeout

**Regression Checks**:
- Live hook approvals still work (approve/deny from web UI)
- Expiration cleanup still runs for live approvals
- No file permission errors on first run (file does not exist yet)

### Checkpoint 3: After Story 3

**Verification**:
1. Send a hook approval via `curl`
2. Verify the review queue item appears within 100ms (check logs for timing)
3. Verify the review queue item contains `tool_name` and `tool_input` metadata
4. Restart the server with a persisted orphaned approval
5. Verify the queue item appears immediately (from persisted data, before poll)

**Regression Checks**:
- No duplicate queue entries from hook + poller concurrent detection
- UI does not flicker when both systems update the same item
- `minReAddInterval` spam prevention still works
- Acknowledge/snooze behavior unchanged

---

## Testing Strategy

### Unit Tests

| Test | File | Coverage |
|------|------|----------|
| Startup scan detects approval prompt | `server/dependencies_test.go` | Story 1 |
| Startup scan ignores active sessions | `server/dependencies_test.go` | Story 1 |
| Startup scan handles empty terminals | `server/dependencies_test.go` | Story 1 |
| Persist approval to disk | `server/services/approval_store_test.go` | Story 2 |
| Load persisted approvals on init | `server/services/approval_store_test.go` | Story 2 |
| Cleanup orphaned approvals by age | `server/services/approval_store_test.go` | Story 2 |
| Resolve orphaned approval returns error | `server/services/approval_store_test.go` | Story 2 |
| Atomic write prevents corruption | `server/services/approval_store_test.go` | Story 2 |
| Immediate queue check on hook | `server/services/approval_handler_test.go` | Story 3 |
| Queue item enrichment from store | `session/review_queue_poller_test.go` | Story 3 |

### Integration Tests

| Test | Scope | Coverage |
|------|-------|----------|
| Full startup cycle with pre-existing approval | E2E | Stories 1+2+3 |
| Concurrent hook + poller detection | Race condition | Story 3 |
| Server restart with pending approvals | Persistence | Story 2 |

---

## Non-Functional Requirements

### Performance

- Startup scan must complete in <2 seconds for 20 sessions (terminal capture is the bottleneck)
- Approval persistence adds <5ms per create/resolve operation
- No measurable increase in steady-state CPU from sync logic

### Reliability

- Corrupted `pending_approvals.json` must not crash the server (log warning, start fresh)
- Missing file on first boot must not produce errors (create on first write)
- All persistence operations use atomic file writes

### Observability

- Log `[StartupScan]` with per-session results: `detected_status`, `added_to_queue`
- Log `[ApprovalPersistence]` on every persist/load/cleanup operation
- Log `[ApprovalSync]` when cross-system sync occurs
- Structured log fields for monitoring and alerting

### Security

- `pending_approvals.json` written with `0600` permissions (owner-only)
- File path constrained to config directory (no path traversal)
- No sensitive data in persisted approvals (tool input may contain file paths but not credentials)
