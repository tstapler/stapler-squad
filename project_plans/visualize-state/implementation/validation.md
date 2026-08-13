# Validation Plan: Session State Visibility & Triage UX

Created: 2026-04-14
Phase: 4 — Validation (pre-implementation)
Input: requirements.md, synthesis.md, docs/tasks/visualize-state.md

---

## Requirements Traceability

| Requirement | Test Suite | Coverage |
|---|---|---|
| R1: Accurate session status | Unit: status event serialization; Integration: GetStatus→event emission; E2E: status chip reflects detected state | Full |
| R2: Refined status labels (e.g. "Waiting for input") | Unit: DetectedStatus→label mapping; Integration: SessionStatusChangedEvent carries detected_context; E2E: label visible on session card | Full |
| R3: Terminal snapshot preview | Unit: ansi-to-html conversion, empty/error states; Integration: GetTerminalSnapshot RPC; E2E: preview visible on card without navigating | Full |
| R4: Non-interrupting approvals | Unit: ApprovalDrawer renders without focus steal; Integration: badge count updates; E2E: approve without leaving current session view | Full |
| R5: Stable review queue | Unit: snapshot-on-enter freeze logic; Integration: new items don't inject mid-list; E2E: position doesn't jump during active review | Full |
| P0: Stale status after reconnect | Unit: reconnect triggers ListSessions; Integration: full-sync on WatchSessions reconnect | Full |

---

## Test Pyramid

```
         [E2E - 6 tests]
        Playwright: triage flow, approval drawer,
        queue stability, reconnect, snapshot render

      [Integration - 14 tests]
     Go: status event emission, snapshot RPC,
     approval store + drawer wiring

   [Unit - 28 tests]
  Go: status event serialization, debounce, expiry check
  TS: ansi-to-html conversion, status label mapping,
      snapshot component states, drawer open/close,
      queue snapshot freeze, React key stability
```

---

## Unit Tests

### Group U1: Status event serialization (Go)
**File**: `server/services/session_service_test.go` (extend) or new `session_status_event_test.go`
**Requirement**: R1, R2

| ID | Test | Input | Expected |
|---|---|---|---|
| U1-1 | `detected_status` field in `SessionStatusChangedEvent` | `DetectedStatus = StatusNeedsApproval` | proto field 4 = `"StatusNeedsApproval"` |
| U1-2 | `detected_context` field carries human description | `statusContext = "Waiting for tool approval"` | proto field 5 = `"Waiting for tool approval"` |
| U1-3 | Empty detected_context when controller inactive | `IsControllerActive = false` | fields 4 and 5 absent / empty |
| U1-4 | Debounce: no duplicate events within 200ms | rapid scrollback writes | single event emitted |
| U1-5 | Emit only on state change | same `DetectedStatus` twice | one event, not two |
| U1-6 | Emit when state changes | `StatusReady` → `StatusNeedsApproval` | event emitted with new status |

### Group U2: Status label mapping (TypeScript)
**File**: `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` (new)
**Requirement**: R2, MP2

| ID | Test | Input | Expected |
|---|---|---|---|
| U2-1 | StatusNeedsApproval → "Needs Approval" label + icon | `detectedStatus = "StatusNeedsApproval"` | label text + alert icon |
| U2-2 | StatusInputRequired → "Waiting for input" label | `detectedStatus = "StatusInputRequired"` | label text + input cursor icon |
| U2-3 | StatusError → "Error" label + warning shape | `detectedStatus = "StatusError"` | label text + triangle icon |
| U2-4 | StatusTestsFailing → "Tests failing" label | `detectedStatus = "StatusTestsFailing"` | label text + ✗ icon |
| U2-5 | StatusActive → "Executing" label | `detectedStatus = "StatusActive"` | label text + spinner icon |
| U2-6 | StatusIdle → "Idle" label | `detectedStatus = "StatusIdle"` | label text + clock icon |
| U2-7 | Unknown / empty detectedStatus → no label rendered | `detectedStatus = ""` | null / nothing rendered |
| U2-8 | Label does not rely on color alone (has icon or text shape) | any status | icon or text present alongside color |

### Group U3: Terminal snapshot component (TypeScript)
**File**: `web-app/src/components/sessions/__tests__/SessionSnapshotPreview.test.tsx` (new)
**Requirement**: R3, MP1

| ID | Test | Input | Expected |
|---|---|---|---|
| U3-1 | Renders ANSI output with colors | ANSI string with `\x1b[32m` green | `<span>` with green color class, no raw escape visible |
| U3-2 | Empty output → shows placeholder | empty string / all blank lines | "No recent output" text |
| U3-3 | Fetch error → shows fallback | RPC throws | "Preview unavailable" text, no error thrown |
| U3-4 | Raw escape codes never shown to user | partial escape `\x1b[3` | stripped / rendered safely, not shown literally |
| U3-5 | Fixed max-height applied | long output (50 lines) | container height ≤ 120px |
| U3-6 | Snapshot fetch does not block card render | fetch in-flight | card renders immediately, snapshot loads async |
| U3-7 | Cache: no re-fetch within TTL | two renders within 5s | `GetTerminalSnapshot` RPC called once |
| U3-8 | Cache expired: re-fetches after TTL | render after 5s+ | `GetTerminalSnapshot` RPC called again |

### Group U4: GetTerminalSnapshot RPC (Go)
**File**: `server/services/terminal_snapshot_service_test.go` (new)
**Requirement**: R3

| ID | Test | Input | Expected |
|---|---|---|---|
| U4-1 | Returns last N lines from `inst.Preview()` | session with 50 lines of output, N=20 | 20 lines returned |
| U4-2 | Returns empty string for paused session | paused instance | empty string, no error |
| U4-3 | Returns empty string for stopped session | stopped instance | empty string, no error |
| U4-4 | Session not found → returns Not Found error | unknown session ID | gRPC NOT_FOUND |
| U4-5 | ANSI escapes preserved when requested | `include_escapes = true` | escape codes in response |
| U4-6 | ANSI escapes stripped when not requested | `include_escapes = false` | plain text response |

### Group U5: Approval drawer (TypeScript)
**File**: `web-app/src/components/sessions/__tests__/ApprovalDrawer.test.tsx` (new)
**Requirement**: R4, MP3

| ID | Test | Input | Expected |
|---|---|---|---|
| U5-1 | Drawer opens when nav badge clicked | click badge | drawer visible, no focus steal from active element |
| U5-2 | Drawer does not steal focus on open | focused input, badge clicked | input remains focused |
| U5-3 | Approvals sorted by time-to-expire (soonest first) | 3 approvals with different expiries | sorted ascending by expiry |
| U5-4 | Expired approval transitions to "Expired" state without layout shift | timer reaches 0 | card shows "Expired" + Dismiss, no height change |
| U5-5 | Approving removes item from drawer | click Approve | item removed, badge count decrements |
| U5-6 | Expiry aria-live announcement fires | approval expires off-screen | `aria-live` region announces "Approval expired for [session]" |
| U5-7 | ApprovalPanel removed from terminal tab | render SessionDetail | no ApprovalPanel in terminal tab DOM |
| U5-8 | Invisible Enter-to-approve shortcut absent | keydown Enter in SessionDetail | no approval submitted |

### Group U6: Review queue snapshot-on-enter (TypeScript)
**File**: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` (extend)
**Requirement**: R5

| ID | Test | Input | Expected |
|---|---|---|---|
| U6-1 | Opening panel freezes item order | open panel, then context adds new item | new item at bottom, not injected in list |
| U6-2 | "N new items" notice appears for injected items | 2 new items added while panel open | notice shows "2 new items added" |
| U6-3 | Clicking notice refreshes snapshot | click notice | snapshot updates to include new items |
| U6-4 | Completing item advances queue correctly | approve first item | next item becomes active, order stable |
| U6-5 | Badge count stays live (not frozen) | new item added while panel open | badge count increments even when list frozen |
| U6-6 | Re-opening panel refreshes snapshot | close and reopen panel | full current item list loaded |

### Group U7: WatchSessions reconnect (TypeScript)
**File**: `web-app/src/lib/transport/__tests__/watchSessionsReconnect.test.ts` (new)
**Requirement**: R1, P0 streaming bug

| ID | Test | Input | Expected |
|---|---|---|---|
| U7-1 | Reconnect calls `ListSessions` before re-subscribing | simulate disconnect + reconnect | `ListSessions` called on reconnect |
| U7-2 | Sessions updated during disconnect show correct state post-reconnect | session changes status while disconnected | card shows current status after reconnect |
| U7-3 | Staleness indicator shows after 15s without event | no events for 15s | "status stale" badge visible |
| U7-4 | Staleness indicator clears when events resume | event arrives after stale state | badge hidden |
| U7-5 | `lastEventTime` tracked correctly | events arrive at known times | timestamp accurate |

### Group U8: Session React key stability (TypeScript)
**File**: `web-app/src/components/sessions/__tests__/SessionList.test.tsx` (extend)
**Requirement**: P1 key thrashing pitfall

| ID | Test | Input | Expected |
|---|---|---|---|
| U8-1 | Session key is stable across status updates | session receives status update | same DOM node, no remount |
| U8-2 | Session key is stable across rename (if UUID implemented) | session renamed | same DOM node, no remount |

### Group U9: Approval expiry (Go)
**File**: `server/services/approval_service_test.go` (extend)
**Requirement**: R4, pitfall P1

| ID | Test | Input | Expected |
|---|---|---|---|
| U9-1 | `ResolveApproval` returns error for expired approval | resolve after `ExpiresAt` | gRPC error "Approval request expired" |
| U9-2 | `ResolveApproval` succeeds for valid approval | resolve before `ExpiresAt` | success |
| U9-3 | `seconds_remaining` = 0 for already-expired | fetch expired approval | `seconds_remaining = 0` |

---

## Integration Tests

### Group I1: Detected status flows from backend to WatchSessions stream (Go)
**File**: `server/services/session_service_integration_test.go` (new)
**Requirement**: R1, R2

| ID | Test | Scenario | Expected |
|---|---|---|---|
| I1-1 | Status change event carries detected_status | session's `ClaudeController` reports `StatusNeedsApproval` | `WatchSessions` stream emits `SessionStatusChangedEvent` with `detected_status = "StatusNeedsApproval"` |
| I1-2 | Status change event carries detected_context | controller reports context string "Waiting for tool approval" | event `detected_context = "Waiting for tool approval"` |
| I1-3 | No spurious events when status unchanged | controller returns same status on two consecutive reads | single event, not duplicate |
| I1-4 | Debounce: rapid scrollback does not spam events | 10 scrollback writes within 100ms | ≤1 status event emitted |
| I1-5 | Lifecycle status (Running/Paused) still works | session paused | `SessionStatusChangedEvent` with `new_status = SESSION_STATUS_PAUSED` (existing behavior unchanged) |

### Group I2: GetTerminalSnapshot RPC end-to-end (Go)
**File**: `server/services/transport_e2e_test.go` (extend)
**Requirement**: R3

| ID | Test | Scenario | Expected |
|---|---|---|---|
| I2-1 | RPC returns tmux pane content for active session | call `GetTerminalSnapshot` for running session | non-empty string response |
| I2-2 | RPC returns empty for paused session | paused session | empty string, HTTP 200 |
| I2-3 | RPC is callable without active `StreamTerminal` stream | fresh client, no terminal open | success (does not require open stream) |
| I2-4 | `last_n_lines` parameter respected | N=5 | ≤5 lines returned |

### Group I3: Approval drawer + approval store wiring
**File**: `web-app/src/lib/contexts/__tests__/ApprovalDrawer.integration.test.tsx` (new)
**Requirement**: R4

| ID | Test | Scenario | Expected |
|---|---|---|---|
| I3-1 | Approval arrives → badge count increments | `NotificationEvent` received | badge shows "+1" |
| I3-2 | Approve action calls `ResolveApproval` RPC | click Approve in drawer | RPC called with correct approval ID and ALLOW decision |
| I3-3 | Deny action calls `ResolveApproval` RPC | click Deny in drawer | RPC called with DENY decision |
| I3-4 | Approval removed from drawer after resolution | approve | item disappears from drawer list |
| I3-5 | Multiple simultaneous approvals all listed | 3 pending approvals | all 3 visible in drawer |

### Group I4: ReviewItem data wired to SessionCard
**File**: `web-app/src/components/sessions/__tests__/SessionList.integration.test.tsx` (new)
**Requirement**: R1, M3 (quick win)

| ID | Test | Scenario | Expected |
|---|---|---|---|
| I4-1 | Session in review queue shows ReviewQueueBadge on card | session has matching ReviewItem | badge visible on card in sessions list |
| I4-2 | Session not in queue shows no badge | no matching ReviewItem | no badge rendered |
| I4-3 | Badge shows correct reason | ReviewItem reason = `NEEDS_APPROVAL` | badge text = "Needs Approval" |

### Group I5: WatchSessions full-sync on reconnect
**File**: `web-app/src/lib/transport/__tests__/watchSessionsReconnect.integration.test.ts` (new)
**Requirement**: R1, P0

| ID | Test | Scenario | Expected |
|---|---|---|---|
| I5-1 | Full sync on reconnect reflects missed updates | disconnect, session status changes, reconnect | card shows new status after reconnect |
| I5-2 | No duplicate session entries after reconnect | reconnect | session list length unchanged |

---

## End-to-End Tests (Playwright)

**File base**: `web-app/tests/e2e/visualize-state/`
**Run with**: `npm run test:e2e -- tests/e2e/visualize-state/`

### E1: Triage speed — rich status visible on session cards
**Requirement**: R1, R2, Success Criterion "Triage speed < 30 seconds"
```
1. Start app with 3 sessions: one Running+Active, one Running+NeedsApproval, one Paused
2. Navigate to Sessions page
3. Assert: session card 1 shows lifecycle "Running" AND detected label "Executing"
4. Assert: session card 2 shows lifecycle "Running" AND detected label "Needs Approval" badge
5. Assert: session card 3 shows "Paused"
6. Assert: no session requires clicking into it to determine its state
```
**Pass condition**: All status states visible without any session click, within 3 seconds of page load.

### E2: Terminal snapshot preview visible on session cards
**Requirement**: R3
```
1. Start app with an active session that has known terminal output
2. Navigate to Sessions page
3. Assert: session card shows a preview pane with last lines of terminal output
4. Assert: no raw ANSI escape codes visible (e.g. "^[[32m" must not appear)
5. Assert: preview has fixed height (does not expand to push other cards)
6. Assert: "No recent output" shown for a cleared terminal session
```
**Pass condition**: Preview visible, clean ANSI rendering, fixed height maintained.

### E3: Approve pending hook without leaving current session view
**Requirement**: R4, Success Criterion "No context interruptions"
```
1. Start app with session A open in terminal view
2. Trigger a hook approval on session B (different session)
3. Assert: session A's terminal view is NOT replaced by session B
4. Assert: approval nav badge count increments (badge is visible)
5. Click approval badge → drawer opens
6. Assert: session A's terminal view still visible behind/alongside drawer
7. Click Approve in drawer
8. Assert: approval resolves, drawer item removed, badge decrements
9. Assert: session A's terminal view still the active view
```
**Pass condition**: Never lose session A view throughout the entire flow.

### E4: Review queue does not jump during active review
**Requirement**: R5, Success Criterion "Stable review queue"
```
1. Start app with 5 sessions in review queue
2. Open review queue panel
3. Record position of item at index 2
4. While panel is open, trigger a new session to enter queue (simulating background activity)
5. Assert: item at index 2 is STILL at index 2 (not displaced by new item)
6. Assert: "1 new item added" notice visible at bottom of list
7. Click notice to refresh
8. Assert: new item now visible, order re-sorted
```
**Pass condition**: Queue position stable; new items arrive at bottom with notice.

### E5: Status updates correctly after WebSocket reconnect
**Requirement**: R1, P0 reconnect bug
```
1. Start app, observe session A with status "Running"
2. Session A status changes to "NeedsApproval" on backend
3. Simulate WebSocket disconnect (browser DevTools: Network offline)
4. Wait 2 seconds
5. Restore network
6. Assert: session A card shows "NeedsApproval" (post-reconnect state) within 3 seconds
7. Assert: no "stale" indicator persists after reconnect resolves
```
**Pass condition**: Correct status shown within 3 seconds of reconnect.

### E6: Quick wins — accessibility smoke test
**Requirement**: C3, H5, H6
```
1. Navigate to review queue
2. Press Tab to navigate to first review queue item
3. Press Enter on the item click area
4. Assert: session opens (keyboard navigation works)
5. Open an ApprovalCard
6. Assert: session title (not raw ID) visible in the approval card
7. Open session detail modal
8. Assert: modal height >= 80% of viewport
```
**Pass condition**: All three quick-win assertions pass.

---

## Risk-Based Test Prioritization

Tests ordered by risk × coverage impact:

| Priority | Tests | Risk Addressed |
|---|---|---|
| **P0 — implement first** | U7-1 through U7-4, I5-1, I5-2, E5 | WatchSessions reconnect stale state |
| **P0 — implement first** | U5-7, U5-8, E3 | Approval modal interruption |
| **P1 — before PR merge** | U1-1 through U1-6, I1-1 through I1-5, E1 | Status detection wiring |
| **P1 — before PR merge** | U3-1 through U3-8, I2-1 through I2-4, E2 | Terminal snapshot (incl. ANSI edge cases) |
| **P1 — before PR merge** | U6-1 through U6-6, E4 | Review queue stability |
| **P2 — before release** | U2-1 through U2-8 | Status label taxonomy |
| **P2 — before release** | U5-1 through U5-6, I3-1 through I3-5 | Approval drawer full flow |
| **P2 — before release** | U9-1 through U9-3 | Approval expiry edge cases |
| **Smoke** | E6 | Quick wins (keyboard + title + modal height) |

---

## Existing Tests — Must Not Regress

These existing test files cover functionality that the feature touches. Run and confirm green after each task.

| File | Covers | Tasks that touch it |
|---|---|---|
| `session/detection/detector_test.go` | `DetectWithContext`, pattern matching | TASK-014 |
| `session/detection/approval_test.go` | Approval pattern detection | TASK-014 |
| `server/services/transport_e2e_test.go` | StreamTerminal handshake, CurrentPaneRequest | TASK-018 |
| `web-app/src/lib/transport/websocket-transport.test.ts` | WatchSessions subscription | TASK-016 |
| `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts` | Terminal stream lifecycle | TASK-016 |
| `web-app/src/lib/contexts/__tests__/NotificationContext.test.tsx` | Notification events | TASK-019 |
| `web-app/src/lib/utils/__tests__/notificationStorage.test.ts` | Notification history | TASK-019 |
| `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx` | Terminal rendering | TASK-018 |

**Command to run all existing tests before starting**: `make quick-check`

---

## Test Conventions (match existing patterns)

**Go unit tests**: Table-driven (`tests []struct{ name, input, expected }`), same package as tested code, e.g. `session/detection/detector_test.go`

**TypeScript unit tests**: Jest + `@testing-library/react`, colocated in `__tests__/` dir alongside component. Mock ConnectRPC clients via `jest.mock`.

**E2E**: Playwright, `web-app/tests/e2e/`. Use `page.waitForSelector` not `page.waitForTimeout`. Each test is independent (no shared state).

**New test files** follow naming convention of nearest existing test in the same package.

---

## Definition of Done

The feature is implementation-complete when:
- [ ] All unit tests (U1–U9) pass: `make test`
- [ ] All integration tests (I1–I5) pass: `go test ./server/services/...`
- [ ] All E2E tests (E1–E6) pass: `npm run test:e2e -- tests/e2e/visualize-state/`
- [ ] All pre-existing tests still pass: `make quick-check`
- [ ] No raw ANSI escape codes visible in terminal preview (manual smoke)
- [ ] Approve a hook without leaving active session view (manual smoke, E3 scenario)
- [ ] `make lint` passes with zero new warnings
