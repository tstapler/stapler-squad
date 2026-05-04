# Implementation Plan: review-queue-state-detection

**Feature**: Review Queue Working-State Detection — filter actively-working sessions from the review queue and emit structured state-change events
**Date**: 2026-05-02
**Status**: Ready for implementation
**ADRs**: ADR-001-working-state-enum-vs-bool.md

---

## Dependency Visualization

```
Epic 1 (Detection Correctness)
  1.1a → 1.1b → 1.1c → 1.1d
  1.2a → 1.2b
  1.3a → 1.3b → 1.3c → 1.3d
         ↓
Epic 2 (Push-Based State Events) [depends on Epic 1 complete]
  2.1a → 2.1b → 2.1c
  2.2a → 2.2b → 2.2c → 2.2d
         ↓
Epic 3 (Review Queue Filtering) [depends on Epic 2 complete]
  3.1a → 3.1b → 3.1c
  3.2a → 3.2b
         ↓
Epic 4 (Config + Corpus Infrastructure) [independent; can parallelize with Epic 2]
  4.1a → 4.1b
  4.2a → 4.2b → 4.2c
```

---

## Phase 1: Detection Correctness Fixes

**Goal**: Fix the low-level bugs that cause misclassification before wiring up the new state model. These are minimal-risk, high-value changes that unblock all downstream work.

### Epic 1.1: Scrollback Tail Limiting

**Goal**: Prevent stale output from poisoning state detection by ensuring only the most recent 4096 bytes are scanned.

#### Story 1.1.1: Use `DetectRecent` instead of `Detect` for controller sessions

**As a** backend engineer, **I want** detection to only scan recent output, **so that** a "esc to interrupt" pattern from 500 lines ago doesn't cause false `StatusActive`.

**Acceptance Criteria**:
- `ReviewQueuePoller.checkSession()` and `detectProcessing()` call `DetectRecent(output, statusDetectionTailBytes)` not `Detect(output)` when handling controller-managed sessions
- The `statusDetectionTailBytes = 4096` constant is the single source of truth
- Detection test `TestDetectRecent` confirms only the tail window is scanned

**Files**:
- `session/review_queue_poller.go`
- `session/detection/detector.go`
- `session/detection/detector_test.go`

##### Task 1.1.1a: Wire `DetectRecent` in `checkSession()` (~3 min)
- In `review_queue_poller.go`, locate the `Detect(content)` call inside `checkSession()`
- Replace with `DetectRecent(content, statusDetectionTailBytes)` where `statusDetectionTailBytes` is imported from `detection` package
- Confirm `DetectRecent` is already implemented (it is — per research); no new function needed
- Files: `session/review_queue_poller.go`

##### Task 1.1.1b: Wire `DetectRecent` in `detectProcessing()` (~3 min)
- In `review_queue_poller.go`, `detectProcessing()` currently passes full `content` to string contains checks
- Slice `content` to last `statusDetectionTailBytes` bytes before the loop (handle case where len < limit)
- Files: `session/review_queue_poller.go`

##### Task 1.1.1c: Add regression test for scrollback poisoning (~4 min)
- In `detector_test.go`, add `TestDetectRecentDoesNotMatchStaleContent`
- Construct a buffer where the first 5000 bytes contain `"esc to interrupt"` and the last 4096 bytes contain only an idle prompt `"? for shortcuts"`
- Assert `DetectRecent(buf, 4096)` returns `StatusIdle`, not `StatusActive`
- Files: `session/detection/detector_test.go`

---

### Epic 1.2: Carriage-Return Handling in ANSI Strip

**Goal**: Fix spinner animation sequences (`\r`-overwritten lines) that survive ANSI stripping and confuse pattern matching.

#### Story 1.2.1: Strip carriage-return overwriting before pattern detection

**As a** backend engineer, **I want** `\r`-overwritten lines collapsed to their final visible form, **so that** spinner sequences like `⠋ Thinking\r⠙ Thinking\r` don't produce spurious multi-line matches.

**Acceptance Criteria**:
- `stripANSI()` (or a new pre-processing step called before it) collapses `\r`-overwritten content so only the last write on each line remains
- Existing snapshot tests still pass
- New test covers the `\r` case

**Files**:
- `session/detection/detector.go`
- `session/detection/detector_test.go`

##### Task 1.2.1a: Add `collapseCarriageReturns()` helper (~3 min)
- In `detector.go`, add:
  ```go
  // collapseCarriageReturns replaces CR-overwritten segments with their final value.
  // "foo\rbar" → "bar"; "foo\r\nbar" is left alone (CR+LF = real newline).
  func collapseCarriageReturns(s string) string
  ```
- Implement: split on `\n`, for each segment split on `\r`, take last element, rejoin with `\n`
- Call this before `stripANSI()` in `Detect()` and `DetectRecent()`
- Files: `session/detection/detector.go`

##### Task 1.2.1b: Add unit tests for CR collapsing (~3 min)
- `TestCollapseCarriageReturns_SpinnerSequence` — spinner multi-line `\r` collapses to last value
- `TestCollapseCarriageReturns_CRLFPreserved` — `\r\n` is not collapsed
- `TestStripANSI_WithCarriageReturn` — full pipeline test: `stripANSI(collapseCarriageReturns(input))`
- Files: `session/detection/detector_test.go`

---

### Epic 1.3: Missing Detection Patterns

**Goal**: Add the three missing patterns that the requirements and research identified as critical signals.

#### Story 1.3.1: Add "Ebbing...", cost summary, and `> ` readline prompt patterns

**As a** backend engineer, **I want** the detector to recognize Claude Code's turn-completion signals, **so that** sessions that have finished a turn re-enter the queue promptly.

**Acceptance Criteria**:
- `StatusSuccess` is returned when a cost summary line `$\d+\.\d+ •` is in the tail
- `StatusIdle` is returned when `^>\s*$` (readline prompt) is in the tail
- `StatusProcessing` is returned when `Ebbing...` is in the tail
- All three patterns are covered by snapshot tests with real fixture files

**Files**:
- `session/detection/detector.go`
- `session/detection/testdata/claude_cost_summary.txt` (new fixture)
- `session/detection/testdata/claude_readline_prompt.txt` (new fixture)
- `session/detection/testdata/claude_ebbing.txt` (new fixture)
- `session/detection/snapshot_test.go`

##### Task 1.3.1a: Add cost summary pattern to `getDefaultPatterns()` (~3 min)
- In `detector.go`, in the `StatusSuccess` pattern group, add:
  ```go
  {Name: "cost_summary_line", Pattern: `(?m)\$\d+\.\d+\s+•`, Priority: 22,
   Description: "Claude cost summary line — turn complete"}
  ```
- Anchor matching to tail (the `DetectRecent` fix in 1.1.1 handles this)
- Files: `session/detection/detector.go`

##### Task 1.3.1b: Add `> ` readline prompt pattern to `StatusIdle` (~2 min)
- In `detector.go`, in the `StatusIdle` pattern group, add:
  ```go
  {Name: "claude_readline_prompt", Pattern: `(?m)^>\s*$`, Priority: 16,
   Description: "Claude Code readline input prompt"}
  ```
- Files: `session/detection/detector.go`

##### Task 1.3.1c: Add Claude thinking-verb pattern to `StatusActive` (~2 min)
- Claude Code displays the thinking state as `* <RANDOM_VERB>… (<time info>)` — the verb rotates through arbitrary words ("Moonwalking", "Ebbing", "Pondering", etc.)
- The invariant is the structure: bullet asterisk + single word + ellipsis/dots, NOT a specific verb
- In `detector.go`, in the `Active` pattern group, add:
  ```go
  {Name: "claude_thinking_verb",
   Pattern: `(?m)^\*\s+\w+[…\.]{1,3}`,
   Priority: 26,
   Description: "Claude thinking state with random verb (Moonwalking…, Ebbing..., etc.)"}
  ```
- This matches `* Moonwalking…`, `* Ebbing...`, `* Pondering.` and any future verbs Claude introduces
- Files: `session/detection/detector.go`

##### Task 1.3.1d: Populate 5 snapshot fixture files (~5 min)
- Create `session/detection/testdata/claude_cost_summary.txt` — realistic 15-line terminal tail ending with `$0.42 • 3 tool uses • 1.2k tokens`
- Create `session/detection/testdata/claude_thinking_verb.txt` — terminal tail containing `* Moonwalking… (4m 18s · ↓ 2.0k tokens · thinking)` (mirroring the existing `claude_active.txt` fixture format)
- Update `session/detection/testdata/claude_idle_ready.txt` — confirm `? for shortcuts` prompt is present (it already exists in the file)
- Update `session/detection/testdata/claude_active.txt` — already populated; use as regression anchor
- Add `TestSnapshotDetection` case that asserts `claude_thinking_verb.txt` → `StatusActive`
- Confirm all snapshot tests pass
- Files: `session/detection/testdata/*.txt`, `session/detection/snapshot_test.go`

---

### Epic 1.4: Unify `detectProcessing()` with ANSI-Stripped Path

**Goal**: Remove the duplicate string-matching logic in `detectProcessing()` and replace it with a call through the proper detection pipeline.

#### Story 1.4.1: Refactor `detectProcessing()` to use `DetectRecent`

**As a** backend engineer, **I want** `detectProcessing()` to use the same ANSI-stripped detector as `checkSession()`, **so that** there is one code path for processing detection that benefits from ANSI stripping and tail limiting.

**Acceptance Criteria**:
- `detectProcessing()` calls `DetectRecent(content, statusDetectionTailBytes)` and checks whether the result is `StatusActive` or `StatusProcessing`
- The 8 hardcoded `processingPatterns` strings are removed
- All existing poller behavior is preserved (same removal/retention logic)

**Files**:
- `session/review_queue_poller.go`

##### Task 1.4.1a: Replace `detectProcessing()` string loop with `DetectRecent` call (~4 min)
- In `review_queue_poller.go`, replace the `processingPatterns` slice and `strings.Contains` loop with:
  ```go
  status := detector.DetectRecent(content, detection.StatusDetectionTailBytes)
  return status == detection.StatusActive || status == detection.StatusProcessing
  ```
- Remove the `processingPatterns` var block
- Ensure `detection` package is imported
- Files: `session/review_queue_poller.go`

---

## Phase 2: Push-Based State Events

**Goal**: Eliminate the 2-second polling window and propagate structured `WorkingState` through the proto stack so the frontend can act on it.

### Epic 2.1: Push-Based Queue Removal on PTY Output

**Goal**: Remove a session from the review queue immediately when the `IdleDetector` transitions to `IdleStateActive`, rather than waiting for the next 2s poll tick.

#### Story 2.1.1: Wire `ClaudeController` to call `queue.Remove()` on activity

**As a** user, **I want** a session to disappear from the review queue the moment it starts responding, **so that** I don't see it as "needing attention" while it's actively working.

**Acceptance Criteria**:
- When `ClaudeController` detects a transition to `IdleStateActive` via PTY output, `ReviewQueue.Remove()` is called within 100ms
- The 2s poller continues to exist as a reconciliation fallback
- No goroutine leak: the callback is cleaned up when the controller stops

**Files**:
- `session/claude_controller.go`
- `session/review_queue_poller.go`
- `session/queue/queue.go`

##### Task 2.1.1a: Add `OnActive` callback field to `ClaudeController` (~3 min)
- In `claude_controller.go`, add:
  ```go
  // OnActive is called when the controller detects a transition to IdleStateActive.
  // It is safe to call from any goroutine.
  OnActive func(sessionName string)
  ```
- In the `SetOnOutput` callback, after `idleDetector.RecordActivity()`, check if `idleDetector.State() == IdleStateActive` and call `ctrl.OnActive(ctrl.sessionName)` if set
- Files: `session/claude_controller.go`

##### Task 2.1.1b: Wire `OnActive` to `queue.Remove()` in `ReviewQueuePoller` (~3 min)
- In `review_queue_poller.go`, when registering a controller for a session (in `startWatching()` or equivalent), set `ctrl.OnActive = func(name string) { rqp.queue.Remove(name) }`
- Files: `session/review_queue_poller.go`

##### Task 2.1.1c: Add test for push-based removal (~4 min)
- In a new `review_queue_poller_push_test.go` or existing test file, add `TestPushBasedRemoval_SessionRemovedOnPTYOutput`
- Simulate PTY output arriving and verify `queue.Remove()` is called before the next poll tick
- Files: `session/review_queue_poller_test.go`

---

### Epic 2.2: `WorkingState` Proto Enum and Propagation

**Goal**: Add `WorkingState` to the proto schema and propagate it through the Go service layer to the frontend, so the review queue can distinguish "idle" from "working" without inference.

#### Story 2.2.1: Add `WorkingState` enum to proto and regenerate bindings

**As a** frontend engineer, **I want** `ReviewItem` to carry a `working_state` field, **so that** I can filter or label working sessions without polling.

**Acceptance Criteria**:
- `WorkingState` enum with values `UNSPECIFIED / ACTIVE / PROCESSING / IDLE / WAITING` is defined in `types.proto`
- `Session.working_state` and `ReviewItem.working_state` are populated
- `make generate-proto` succeeds; generated Go and TypeScript bindings are committed
- `SessionStatusChangedEvent` carries `working_state`

**Files**:
- `proto/session/v1/types.proto`
- `proto/session/v1/session.proto`
- `proto/session/v1/events.proto`
- `session/gen/session/v1/*.go` (generated)
- `web-app/src/gen/session/v1/*_pb.ts` (generated)

##### Task 2.2.1a: Add `WorkingState` enum to `types.proto` (~3 min)
- In `proto/session/v1/types.proto`, add:
  ```protobuf
  enum WorkingState {
    WORKING_STATE_UNSPECIFIED = 0;
    WORKING_STATE_ACTIVE = 1;
    WORKING_STATE_PROCESSING = 2;
    WORKING_STATE_IDLE = 3;
    WORKING_STATE_WAITING = 4;
  }
  ```
- Add `working_state WorkingState = 50;` to the `Session` message
- Files: `proto/session/v1/types.proto`

##### Task 2.2.1b: Add `working_state` to `ReviewItem` and `SessionStatusChangedEvent` (~3 min)
- In `proto/session/v1/session.proto`, add `working_state WorkingState = 20;` to `ReviewItem`
- In `proto/session/v1/events.proto`, add `working_state WorkingState = 6;` to `SessionStatusChangedEvent`
- Files: `proto/session/v1/session.proto`, `proto/session/v1/events.proto`

##### Task 2.2.1c: Run `make generate-proto` and verify (~2 min)
- Run `make generate-proto`
- Verify `session/gen/session/v1/` and `web-app/src/gen/session/v1/` contain the new enum
- Files: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts`

##### Task 2.2.1d: Populate `working_state` in Go service layer (~5 min)
- In `server/adapters/instance_adapter.go`, add a `mapIdleStateToWorkingState(s IdleState) sessionv1.WorkingState` helper that maps `IdleStateActive → WORKING_STATE_ACTIVE`, `IdleStateWaiting → WORKING_STATE_IDLE`, `IdleStateTimeout → WORKING_STATE_WAITING`, `IdleStateUnknown → WORKING_STATE_UNSPECIFIED`
- Call this in the `Session` adapter and set `working_state`
- In `server/services/review_queue_service.go`, propagate `working_state` from the session's `IdleState` into `ReviewItem.working_state`
- Files: `server/adapters/instance_adapter.go`, `server/services/review_queue_service.go`

---

## Phase 3: Review Queue Filtering (Frontend)

**Goal**: Use the new `working_state` field to filter working sessions from the queue display, show counts by state, add a manual override, and trigger transition notifications.

### Epic 3.1: Filter and Counts

**Goal**: Working sessions are excluded from the main queue list; a badge shows counts by state.

#### Story 3.1.1: Filter working sessions from queue display and show state counts

**As a** user, **I want** sessions that are actively working to not appear in my review queue, **so that** I only see sessions that actually need my attention.

**Acceptance Criteria**:
- Sessions with `working_state == ACTIVE` or `PROCESSING` are not shown in the main queue list
- Queue header shows "N waiting · M working · K stuck"
- No page refresh needed — the WatchReviewQueue stream drives updates

**Files**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/lib/store/reviewQueueSlice.ts`
- `web-app/src/lib/hooks/useReviewQueue.ts`

##### Task 3.1.1a: Add `workingState` to Redux slice and selectors (~4 min)
- In `reviewQueueSlice.ts`, update the `ReviewItem` TypeScript type to include `workingState: WorkingState`
- Add selector `selectWaitingItems` (filters out `ACTIVE` / `PROCESSING` states)
- Add selector `selectQueueStats` returning `{ waiting: number, working: number, stuck: number }`
- Files: `web-app/src/lib/store/reviewQueueSlice.ts`

##### Task 3.1.1b: Update `ReviewQueuePanel` to use filtered list and count badge (~4 min)
- In `ReviewQueuePanel.tsx`, use `selectWaitingItems` instead of all items for the rendered list
- Add a header count row: `"{waiting} waiting · {working} working · {stuck} stuck"`
- Optionally render working sessions in a collapsed `<details>` section below the main list
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.1.1c: Update `useReviewQueue.ts` to map `working_state` from proto (~3 min)
- In `useReviewQueue.ts`, map `item.workingState` from the proto enum to the Redux slice's `workingState` field
- Import generated proto enum `WorkingState` from `web-app/src/gen/session/v1/`
- Files: `web-app/src/lib/hooks/useReviewQueue.ts`

---

### Epic 3.2: Manual Override and Transition Notifications

**Goal**: Allow the user to manually force a session into the queue (override working-state detection), and notify when a working session transitions to idle.

#### Story 3.2.1: Manual "mark as waiting" override

**As a** user, **I want** to manually add a session to the review queue even if it's detected as working, **so that** detection failures don't block me.

**Acceptance Criteria**:
- Working sessions in the collapsed section have a "Mark as waiting" button
- Clicking it calls a new `ForceQueueSession` RPC (or reuses `AcknowledgeSession` with a flag)
- The session appears in the main queue list until detection clears it

**Files**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `server/services/review_queue_service.go`
- `session/queue/queue.go`

##### Task 3.2.1a: Add `ForceQueue` method to `ReviewQueue` and expose via RPC (~5 min)
- In `session/queue/queue.go`, add `ForceAdd(sessionID string, item *ReviewItem)` that sets a `forced: true` flag on the item
- In `session/review_queue_poller.go`, `checkSession()` must skip `queue.Remove()` for force-added items unless `working_state` clears (add a check: `if item.forced && currentStatus != Idle { skip remove }`)
- Add `ForceQueueSession(req)` RPC to `review_queue_service.go` and register in `server/server.go`
- Files: `session/queue/queue.go`, `session/review_queue_poller.go`, `server/services/review_queue_service.go`, `server/server.go`

##### Task 3.2.1b: Add "Mark as waiting" button to collapsed working-sessions section (~3 min)
- In `ReviewQueuePanel.tsx`, in the working sessions `<details>` block, add a button per item that calls `ForceQueueSession` via `useReviewQueue` hook
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

#### Story 3.2.2: Transition notification (working → idle)

**As a** user, **I want** a notification when a session I was waiting on finishes its turn, **so that** I know it's ready without constantly checking.

**Acceptance Criteria**:
- When `working_state` transitions from `ACTIVE/PROCESSING` to `IDLE` in the stream, a push notification is fired using the existing notification mechanism
- The notification message includes the session name

**Files**:
- `web-app/src/lib/hooks/useReviewQueue.ts`
- `web-app/src/lib/hooks/useNotifications.ts` (or equivalent existing hook)

##### Task 3.2.2a: Detect working→idle transition in `useReviewQueue` and fire notification (~4 min)
- In `useReviewQueue.ts`, track previous `workingState` per item in a `useRef` map
- On stream update, compare old vs new `workingState`; if changed from `ACTIVE/PROCESSING` to `IDLE`, call the existing notification API (e.g., `notifySessionReady(item.sessionName)`)
- Files: `web-app/src/lib/hooks/useReviewQueue.ts`

---

## Phase 4: Config Exposure + Golden-State Infrastructure

**Goal**: Make detection thresholds configurable without recompilation, and provide developer tooling to capture and validate detection heuristics.

### Epic 4.1: Config Exposure for Detection Thresholds

**Goal**: Expose `ReviewQueuePollerConfig` fields via `config.json` so thresholds can be tuned without rebuilding.

#### Story 4.1.1: Add `review_queue` section to `config.json` schema

**As a** developer/operator, **I want** to tune polling and staleness thresholds in `config.json`, **so that** I can adjust detection sensitivity for a specific environment.

**Acceptance Criteria**:
- `config.json` supports a `review_queue` key with `poll_interval_ms`, `staleness_threshold_ms`, `idle_threshold_ms`, `stuck_threshold_ms`
- Values are loaded at startup and override the `ReviewQueuePollerConfig` Go defaults
- Missing `review_queue` key in config falls back to existing Go defaults

**Files**:
- `config/config.go`
- `server/server.go`
- `session/review_queue_poller.go`

##### Task 4.1.1a: Add `ReviewQueueConfig` struct to `config/config.go` (~3 min)
- In `config/config.go`, add:
  ```go
  type ReviewQueueConfig struct {
    PollIntervalMs       int `json:"poll_interval_ms,omitempty"`
    StalenessThresholdMs int `json:"staleness_threshold_ms,omitempty"`
    IdleThresholdMs      int `json:"idle_threshold_ms,omitempty"`
    StuckThresholdMs     int `json:"stuck_threshold_ms,omitempty"`
  }
  ```
- Add `ReviewQueue ReviewQueueConfig `json:"review_queue,omitempty"`` to the main `Config` struct
- Files: `config/config.go`

##### Task 4.1.1b: Wire `ReviewQueueConfig` into `ReviewQueuePollerConfig` at startup (~3 min)
- In `server/server.go`, after loading config, populate a `ReviewQueuePollerConfig` from `cfg.ReviewQueue`, using Go defaults for zero-value fields
- Pass the config into `NewReviewQueuePoller()`
- Files: `server/server.go`, `session/review_queue_poller.go`

---

### Epic 4.2: Golden-State Capture Infrastructure

**Goal**: Provide a UI button and backend endpoint to capture labeled scrollback snapshots, and a `make corpus-validate` target to measure detection accuracy.

#### Story 4.2.1: Backend snapshot capture endpoint

**As a** developer, **I want** to save a labeled scrollback snapshot from the terminal view, **so that** I can build a corpus of real-world examples to validate and tune detection.

**Acceptance Criteria**:
- `CaptureStateSnapshot(sessionID, label)` RPC saves a JSON file to `~/.stapler-squad/state-corpus/<timestamp>-<sessionID>-<label>.json`
- JSON includes: `timestamp`, `session_id`, `label` (working/idle/stuck), `scrollback_tail` (last 200 lines as string)
- Files are written atomically (write to `.tmp` then rename)

**Files**:
- `proto/session/v1/session.proto`
- `server/services/corpus_service.go` (new)
- `server/server.go`

##### Task 4.2.1a: Add `CaptureStateSnapshot` RPC to proto (~2 min)
- In `proto/session/v1/session.proto`, add:
  ```protobuf
  rpc CaptureStateSnapshot(CaptureStateSnapshotRequest) returns (CaptureStateSnapshotResponse);
  message CaptureStateSnapshotRequest { string session_id = 1; string label = 2; }
  message CaptureStateSnapshotResponse { string file_path = 1; }
  ```
- Run `make generate-proto`
- Files: `proto/session/v1/session.proto`

##### Task 4.2.1b: Implement `corpus_service.go` and register (~5 min)
- Create `server/services/corpus_service.go`
- Implement `CaptureStateSnapshot`: look up session, get last 200 lines of scrollback, marshal JSON, write to `~/.stapler-squad/state-corpus/`
- Register in `server/server.go`
- Files: `server/services/corpus_service.go`, `server/server.go`

##### Task 4.2.1c: Add "Capture State" button to terminal view and `make corpus-validate` target (~5 min)
- In the terminal session view component (locate by `data-testid="terminal-view"` or similar), add a small developer-only button (visible only when a debug flag is set, e.g., `?debug=1` query param) that calls `CaptureStateSnapshot` with a dropdown for `working/idle/stuck`
- In `Makefile`, add:
  ```makefile
  corpus-validate:
      go run ./cmd/corpus-validate/... ~/.stapler-squad/state-corpus/
  ```
- Create `cmd/corpus-validate/main.go` that reads each JSON file, runs `DetectRecent` on the `scrollback_tail`, and compares to the `label`; prints a summary table of TP/FP/FN rates
- Files: `web-app/src/components/sessions/TerminalView.tsx` (or equivalent), `Makefile`, `cmd/corpus-validate/main.go`

---

## Risk Flags and Technology Choices

### ADR-001: `WorkingState` Enum vs. `is_working` Bool

**Decision**: Use `WorkingState` enum (Option B from architecture research) instead of a simple `is_working: bool`.

**Rationale**: The bool loses the distinction between `StatusActive` (Claude generating, interrupt available) and `StatusProcessing` (tool use, no interrupt visible). The frontend needs this distinction to show appropriate UI (spinner vs. tool-use indicator). The enum is backward-compatible: proto default 0 = `UNSPECIFIED` which the frontend treats same as the old "no info" state.

**Risk**: Requires `make generate-proto` and committing generated files. Medium effort; well-understood in this codebase.

### Flagged Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Pattern fragility (P-1): Claude Code UI text changes invalidate patterns | Medium | Golden-state corpus + `make corpus-validate` (Epic 4.2) |
| `\r` CR collapsing may change behavior for edge-case terminals | Low | Covered by unit tests in Task 1.2.1b; additive change. Note: `DetectStateFromContent` in `idle.go` already calls `stripANSI` before passing to `Detect()` — that double-strips harmlessly but `collapseCarriageReturns` needs to run before the first `stripANSI` call |
| `ForceQueue` override (Epic 3.2) creates state that outlives detection fix | Low | Clear forced flag when session transitions to idle naturally |
| `collapseCarriageReturns` adds CPU overhead per detection call | Low | 4096-byte tail means ~4KB per call; negligible |
| No-controller sessions (P-6) still use coarser `capture-pane` detection | Medium | Out of scope for this plan; document as known gap |

---

## Task Summary

| Phase | Epic | Stories | Tasks |
|---|---|---|---|
| Phase 1 | 1.1 Scrollback Tail Limiting | 1 | 3 |
| Phase 1 | 1.2 Carriage-Return Handling | 1 | 2 |
| Phase 1 | 1.3 Missing Detection Patterns | 1 | 4 |
| Phase 1 | 1.4 Unify detectProcessing | 1 | 1 |
| Phase 2 | 2.1 Push-Based Queue Removal | 1 | 3 |
| Phase 2 | 2.2 WorkingState Proto Propagation | 1 | 4 |
| Phase 3 | 3.1 Filter and Counts | 1 | 3 |
| Phase 3 | 3.2 Manual Override + Notifications | 2 | 3 |
| Phase 4 | 4.1 Config Exposure | 1 | 2 |
| Phase 4 | 4.2 Corpus Infrastructure | 1 | 3 |
| **Total** | **10 epics** | **12 stories** | **28 tasks** |
