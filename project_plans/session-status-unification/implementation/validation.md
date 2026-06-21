# Session Status Unification — Test Suite Validation Plan

**Gate verdict:** PASS (see §5)
**Test count:** 38 tests (12 Go unit, 6 Go integration, 14 Jest unit, 6 Playwright e2e)
**Requirements coverage:** 32/36 sub-requirements have ≥1 test (89%)
**Uncovered:** R5.1 (server removal of MapDetectedStatusToWorkingState — build-only), R5.2 (proto deprecation — codegen-only), R6.1 (ESLint rule — lint-only), R1.2 (codegen correctness — verified by build, not runtime tests)

---

## 1. Test Suite

### Go Unit Tests

---

#### T-001
**Type:** Go unit
**File:** `session/detection/proto_mapping_test.go` (new)
**Requirements:** R1.3, R2.1, R2.4
**Epic coverage:** Epic 1 (Story 1.2), Epic 2 (Story 2.1)

**What it verifies:**
`DetectedStatusToProto` maps every current Go `DetectedStatus` iota constant to a non-UNSPECIFIED proto value. The test enumerates all constants explicitly — not via range loop — so adding a new constant without updating the mapping causes a test failure, not a silent zero-value mapping.

**Specific cases:**
- `StatusIdle` → `DETECTED_STATUS_IDLE`
- `StatusProcessing` → `DETECTED_STATUS_PROCESSING`
- `StatusExecuting` (renamed from `StatusActive`) → `DETECTED_STATUS_EXECUTING`
- `StatusNeedsApproval` → `DETECTED_STATUS_NEEDS_APPROVAL`
- `StatusInputRequired` → `DETECTED_STATUS_INPUT_REQUIRED`
- `StatusError` → `DETECTED_STATUS_ERROR`
- `StatusTestsFailing` → `DETECTED_STATUS_TESTS_FAILING`
- `StatusSuccess` → `DETECTED_STATUS_SUCCESS`
- `StatusUnknown` → `DETECTED_STATUS_UNKNOWN`
- `StatusReady` → `DETECTED_STATUS_READY`
- `StatusWaitingForAgent` → `DETECTED_STATUS_WAITING_FOR_AGENT`
- No constant maps to `DETECTED_STATUS_UNSPECIFIED`

**Fails before fix:** `StatusActive` is gone (renamed), `StatusWaitingForAgent` is absent, `StatusRateLimited` would compile-fail. The mapping function does not exist yet.
**Passes after fix:** `DetectedStatusToProto` exists with correct mapping for all 11 constants.

---

#### T-002
**Type:** Go unit
**File:** `session/detection/proto_mapping_test.go` (new)
**Requirements:** R1.3, R2.4
**Epic coverage:** Epic 1 (Story 1.2), Epic 7 (Story 7.1)

**What it verifies:**
`DetectedStatusToProto` returns `DETECTED_STATUS_UNSPECIFIED` for the zero value of `DetectedStatus` only if the zero value is not a named constant. Concretely: `StatusUnknown` IS iota 0 and must map to `DETECTED_STATUS_UNKNOWN`, not `UNSPECIFIED`. This test asserts that `DetectedStatusToProto(0)` returns `DETECTED_STATUS_UNKNOWN` (same as `DetectedStatusToProto(StatusUnknown)`), verifying the mapping handles the zero-value case intentionally.

**Fails before fix:** Function not yet implemented.
**Passes after fix:** `StatusUnknown` (iota 0) maps to `DETECTED_STATUS_UNKNOWN`, not `UNSPECIFIED`.

---

#### T-003
**Type:** Go unit
**File:** `session/status_mapping_test.go` (extend existing)
**Requirements:** R2.1, R2.3
**Epic coverage:** Epic 2 (Story 2.1)

**What it verifies:**
`AttentionReasonFromDetected` and `StatusFromDetected` accept `detection.StatusExecuting` (the renamed constant) in place of the old `detection.StatusActive`. Both functions return the same result for `StatusExecuting` as they previously returned for `StatusActive` — no behavior regression.

**Specific assertions:**
- `AttentionReasonFromDetected(detection.StatusExecuting)` returns `""` (no attention reason)
- `StatusFromDetected(detection.StatusExecuting)` returns `session.Active`

**Fails before fix:** `detection.StatusActive` still compiles; `StatusExecuting` does not exist. After rename, old test references to `StatusActive` fail to compile.
**Passes after fix:** Rename is complete; both helper functions accept `StatusExecuting`.

---

#### T-004
**Type:** Go unit
**File:** `session/status_mapping_test.go` (extend existing)
**Requirements:** R2.1, R2.3
**Epic coverage:** Epic 2 (Story 2.1)

**What it verifies:**
`StatusActive` does NOT compile anywhere in Go source after Epic 2 ships. This is enforced by the build itself, but this test documents the intent by confirming `StatusExecuting` is the constant that is accepted by the mapping. The test table in the existing file is updated to remove all `StatusActive` references and add `StatusExecuting` — the test passing implies `StatusActive` is gone.

**Fails before fix:** Test table uses `StatusActive` (still present).
**Passes after fix:** Test table uses `StatusExecuting`; `StatusActive` is removed from Go source.

---

#### T-005
**Type:** Go unit
**File:** `session/detection/detector_test.go` (extend existing) or `session/detection/pattern_set_test.go`
**Requirements:** R2.2
**Epic coverage:** Epic 2 (Story 2.2)

**What it verifies:**
The `.*` catch-all pattern in the detector returns `StatusUnknown`, NOT `StatusReady`. Specifically: a terminal output string that matches no specific pattern (e.g., random noise like `"xyz garbage output"`) is detected as `StatusUnknown`. A readline-style prompt (e.g., `"$ "` or `"> "`) is detected as `StatusReady` (not `StatusUnknown`).

**Specific assertions:**
- `detector.DetectFromString("xyz garbage output 12345")` returns `StatusUnknown`
- `detector.DetectFromString("$ ")` does NOT return `StatusUnknown`
- `detector.DetectFromString("$ ")` returns `StatusReady` (explicit shell prompt)

**Fails before fix:** Random garbage input returns `StatusReady` (the old `.*` catch-all). Shell prompt may or may not match; catch-all semantics are identical.
**Passes after fix:** Catch-all returns `StatusUnknown`; explicit shell prompt returns `StatusReady`.

---

#### T-006
**Type:** Go unit
**File:** `session/command_executor_test.go` (extend existing)
**Requirements:** R2.2
**Epic coverage:** Epic 2 (Story 2.2, Task 2.2.4)

**What it verifies:**
`CommandExecutor` success is defined as `detectedStatus == StatusIdle`, NOT `StatusReady` or `StatusUnknown`. A command that ends with the terminal in `StatusIdle` state reports `result.Success = true`. A command that ends with the terminal in `StatusUnknown` (catch-all) reports `result.Success = false`.

**Specific assertions:**
- `result.Success` is `true` when the status detector returns `StatusIdle` at command completion
- `result.Success` is `false` when the status detector returns `StatusUnknown` at command completion
- `DefaultExecutionOptions().TerminalStatuses` contains `StatusIdle`, NOT `StatusReady`

**Fails before fix:** `DefaultExecutionOptions().TerminalStatuses` contains `StatusReady`; `result.Success = (status == detection.StatusReady)` at line 381. After the catch-all change, `StatusReady` no longer fires on all terminal output, so all commands report success = false.
**Passes after fix:** Terminal status is `StatusIdle`; `result.Success` correctly reflects idle completion.

---

#### T-007
**Type:** Go unit
**File:** `server/analytics/subscriber_test.go` (extend existing)
**Requirements:** R3.3 (StatusChangedEvent removal), R6 (pipeline integrity)
**Epic coverage:** Epic 4 (Story 4.2)

**What it verifies:**
After Epic 4, the analytics subscriber records `session.status_changed` events when an `EventSessionUpdated` with a new status is published. Specifically: publishing `EventSessionUpdated` with `Session.Status = session.Stopped` causes the subscriber to record an analytics event named `"session.status_changed"` with `new_status = "Stopped"`.

**Specific assertions:**
- Analytics subscriber receives `EventSessionUpdated` (not `EventSessionStatusChanged`)
- Recorded event has `EventName = "session.status_changed"`
- Recorded event properties contain `new_status = "Stopped"`
- `old_status` is present (may be empty string on first transition)

**Fails before fix:** Subscriber only listens on `EventSessionStatusChanged`; `EventSessionUpdated` does not trigger the analytics record.
**Passes after fix:** Subscriber migrated to `EventSessionUpdated`; STOPPED transition is recorded.

---

#### T-008
**Type:** Go unit
**File:** `server/analytics/subscriber_test.go` (extend existing)
**Requirements:** R3.3
**Epic coverage:** Epic 4 (Story 4.2, Task 4.2.1 — in-memory state management)

**What it verifies:**
The analytics subscriber's in-memory `lastKnownStatus` map is cleaned up when `EventSessionDeleted` fires. After session deletion, the map does not retain the session's last status (preventing unbounded growth). This test publishes a sequence of events: session created → session updated (status changed) → session deleted, then verifies the in-memory map no longer contains the session ID.

**Fails before fix:** No in-memory map exists (subscriber uses `StatusChangedEvent` which carries `old_status` explicitly).
**Passes after fix:** In-memory map is populated on `EventSessionUpdated`, cleared on `EventSessionDeleted`.

---

#### T-009
**Type:** Go unit
**File:** `server/push/subscriber_test.go` (extend existing)
**Requirements:** R3.3
**Epic coverage:** Epic 4 (Story 4.1)

**What it verifies:**
The push notification subscriber fires a "Session Completed" notification when `EventSessionUpdated` with `Session.Status == session.Stopped` is published. Specifically, `shouldNotify(EventSessionUpdated, ...)` returns `true` for a session with `Status = Stopped`.

**Specific assertions:**
- `shouldNotify(events.EventSessionUpdated, 0, 0, session.Stopped)` returns `true`
- `shouldNotify(events.EventSessionUpdated, 0, 0, session.Active)` returns `false`
- `shouldNotify(events.EventSessionStatusChanged, ...)` no longer fires (the constant is removed)

**Fails before fix:** `shouldNotify` only handles `EventSessionStatusChanged`; `EventSessionUpdated` returns false for all statuses.
**Passes after fix:** Subscriber migrated; `EventSessionUpdated` with Stopped status triggers notification.

---

#### T-010
**Type:** Go unit
**File:** `server/adapters/instance_adapter_test.go` (extend or create)
**Requirements:** R1.3, R3.1
**Epic coverage:** Epic 3 (Story 3.1, Task 3.1.3)

**What it verifies:**
`InstanceToProto` populates `Session.DetectedStatus` when `inst.Status == session.Active`. When the instance is not active (e.g., Stopped), `Session.DetectedStatus` is zero/UNSPECIFIED.

**Specific assertions:**
- Active instance with `GetDetectedStatus() = StatusProcessing` → `proto.DetectedStatus = DETECTED_STATUS_PROCESSING`
- Stopped instance → `proto.DetectedStatus = DETECTED_STATUS_UNSPECIFIED`

**Fails before fix:** `InstanceToProto` does not populate `DetectedStatus` (field does not exist yet).
**Passes after fix:** Epic 1 adds the field; Epic 3 populates it in `InstanceToProto`.

---

#### T-011
**Type:** Go unit
**File:** `session/detection/proto_mapping_test.go` (new)
**Requirements:** R2.4
**Epic coverage:** Epic 7 (Story 7.1)

**What it verifies:**
Exhaustive switch enforcement: a table-driven test that calls `DetectedStatusToProto` for every integer value from 0 to `maxKnownDetectedStatus` (determined at test time by inspecting the iota) and asserts that no call returns `DETECTED_STATUS_UNSPECIFIED` for a known constant and does not panic for unknown integers. This is a runtime guard complementing the compile-time `exhaustive` linter.

**Fails before fix:** `DetectedStatusToProto` not yet implemented.
**Passes after fix:** All known constants produce non-UNSPECIFIED output; unknown integers fall to default and return UNSPECIFIED without panic.

---

#### T-012
**Type:** Go unit
**File:** `server/services/event_converter_test.go` (extend or create)
**Requirements:** R3.1, R3.2
**Epic coverage:** Epic 3 (Story 3.1, Task 3.1.2)

**What it verifies:**
`event_converter.go` serializes `EventSessionUpdated` with `DetectedStatus` correctly. When the event struct carries `DetectedStatus = StatusProcessing`, the resulting `SessionUpdatedEvent` proto has `DetectedStatus = DETECTED_STATUS_PROCESSING`. When `DetectedStatus = StatusUnknown`, the converter either omits the field or explicitly sets `DETECTED_STATUS_UNKNOWN` (consistent with the plan's guard condition).

**Fails before fix:** `SessionUpdatedEvent` proto does not have the `DetectedStatus` field; conversion code does not exist.
**Passes after fix:** Event converter populates the new fields; the test passes with the expected proto value.

---

### Go Integration Tests

---

#### T-013
**Type:** Go integration
**File:** `server/services/session_service_test.go` or `server/services/session_lifecycle_test.go` (extend)
**Requirements:** R3.1, R3.2, R4.1
**Epic coverage:** Epic 3 (Story 3.1, Task 3.1.4), Epic 4 (Story 4.3)

**What it verifies:**
When a session transitions from ACTIVE to STOPPED (via `sessionExitedPublisher`), the `SessionUpdatedEvent` published carries `DetectedStatus = DETECTED_STATUS_UNSPECIFIED` (cleared on stop) and `DetectedContext = ""`. Importantly, NO `SessionStatusChangedEvent` is published after Epic 4 (the `eventBus` receives exactly one event of type `EventSessionUpdated`, not two events of different types).

**Specific assertions:**
- `eventBus` receives exactly one event after session exit
- That event's type is `EventSessionUpdated`
- `SessionUpdatedEvent.DetectedStatus == DETECTED_STATUS_UNSPECIFIED`
- `SessionUpdatedEvent.DetectedContext == ""`
- No event of type `EventSessionStatusChanged` is published

**Fails before fix:** `sessionExitedPublisher` publishes `EventSessionStatusChanged`; no `EventSessionUpdated` with detection fields exists.
**Passes after fix:** Epic 3 adds detection fields to `SessionUpdatedEvent`; Epic 4 removes `EventSessionStatusChanged` publication.

---

#### T-014
**Type:** Go integration
**File:** `server/services/session_service_test.go` (extend)
**Requirements:** R3.2, R3.3, R4.4
**Epic coverage:** Epic 4 (Story 4.3 — atomic removal)

**What it verifies:**
After Epic 4 ships, `EventSessionStatusChanged` does not exist in the event bus at all. A full session lifecycle (create → activate → detect status → stop) publishes ONLY `EventSessionUpdated` events (plus `EventSessionCreated`, `EventSessionDeleted` as appropriate). No `EventSessionStatusChanged` event type is observed on the bus.

**Search strategy:** Subscribe to all events on the bus; record event types; assert none match `EventSessionStatusChanged` (or the constant is undefined / the type assertion fails to compile).

**Fails before fix:** `EventSessionStatusChanged` is still published.
**Passes after fix:** Constant is removed; all status transitions use `EventSessionUpdated`.

---

#### T-015
**Type:** Go integration
**File:** `server/services/session_service_test.go` (extend)
**Requirements:** R3.1, R4.2, R4.3
**Epic coverage:** Epic 3 (Task 3.1.5), Epic 5 (conceptually validates the full pipeline)

**What it verifies:**
The `UpdateSession` RPC publish path: when `UpdateSession` is called on an ACTIVE session that has a non-UNSPECIFIED detected status, the `SessionUpdatedEvent` emitted carries the correct `DetectedStatus`. This verifies Task 3.1.5 (the RPC site populates detection info).

**Specific assertions:**
- After `UpdateSession` is called on a session with `detectedStatus = StatusProcessing`, the event bus receives a `SessionUpdatedEvent` with `DetectedStatus = DETECTED_STATUS_PROCESSING`

**Fails before fix:** `SessionUpdatedEvent` does not carry detection fields.
**Passes after fix:** All publish sites populate detection fields.

---

#### T-016
**Type:** Go integration
**File:** `server/adapters/review_queue_adapter_test.go` (extend existing)
**Requirements:** R5.1, R5.3
**Epic coverage:** Epic 6 (Story 6.3)

**What it verifies:**
After Epic 6, `MapDetectedStatusToWorkingState` is removed from `instance_adapter.go`. The `ReviewItem` proto is no longer populated with `working_state` from the server. The test verifies that the `ReviewItem.WorkingState` field is zero/UNSPECIFIED when produced by `ReviewQueueItemToProto` — the frontend must derive `WorkingState` on its own.

**Fails before fix:** `ReviewItem.working_state` is populated server-side.
**Passes after fix:** Function removed; field is zero; frontend utility `deriveWorkingState` is the only derivation path.

---

#### T-017
**Type:** Go integration
**File:** `server/services/session_status_pipeline_test.go` (new, overall pipeline test)
**Requirements:** R3.1, R3.2, R4.1, R4.2
**Epic coverage:** Epics 3, 4 combined (validates end-to-end event pipeline)

**What it verifies:**
Full pipeline integration: a session is created, its PTY output causes a detection event (`StatusProcessing`), and the event bus delivers a `SessionUpdatedEvent` with `DetectedStatus = DETECTED_STATUS_PROCESSING`. Subsequently the session stops, and a second `SessionUpdatedEvent` carries `DetectedStatus = DETECTED_STATUS_UNSPECIFIED`.

This is the definitive test for the "Thinking… linger bug" fix: the stopped event explicitly clears the detection state rather than leaving it set from the previous detection.

**Fails before fix:** No `DetectedStatus` on `SessionUpdatedEvent`; stopped event does not clear detection state.
**Passes after fix:** Both events carry correct detection state; clearing on stop is verified.

---

#### T-018
**Type:** Go integration
**File:** `server/push/subscriber_test.go` (extend existing)
**Requirements:** R3.3, R4.1
**Epic coverage:** Epic 4 (Story 4.1)

**What it verifies:**
End-to-end push notification integration: publishing a `EventSessionUpdated` with a fully-formed `session.Instance` (Status = Stopped, Title set) to the event bus causes the push subscriber to call the delivery function with a notification whose body contains the session title and whose URL contains the session ID.

**Fails before fix:** Push subscriber listens for `EventSessionStatusChanged`; `EventSessionUpdated` does not trigger it.
**Passes after fix:** Subscriber migrated to `EventSessionUpdated`; notification is triggered and has correct content.

---

### Jest Unit Tests

---

#### T-019
**Type:** Jest unit
**File:** `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` (extend existing)
**Requirements:** R4.2
**Epic coverage:** Epic 5 (Story 5.1, Task 5.1.1)

**What it verifies:**
`upsertSession` clears `detectedStatusMap[id]` when the session's `status` is NOT `SessionStatus.ACTIVE`. Test creates a session with a populated `detectedStatusMap` entry, then dispatches `upsertSession` with the same session having `status = STOPPED`. Asserts `detectedStatusMap[id]` is undefined after the dispatch.

**Specific scenario:**
```
Initial state: detectedStatusMap["s1"] = { detectedStatus: DetectedStatus.PROCESSING, detectedContext: "" }
Action: upsertSession({ id: "s1", status: SessionStatus.STOPPED, ... })
Expected: detectedStatusMap["s1"] === undefined
```

**Fails before fix:** `upsertSession` only calls `sessionsAdapter.upsertOne`; `detectedStatusMap` is not touched; the stale "Thinking…" badge persists.
**Passes after fix:** `upsertSession` deletes `detectedStatusMap[id]` for non-ACTIVE sessions.

---

#### T-020
**Type:** Jest unit
**File:** `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` (extend existing)
**Requirements:** R4.3
**Epic coverage:** Epic 5 (Story 5.1, Task 5.1.1)

**What it verifies:**
`upsertSession` updates `detectedStatusMap[id]` from the typed `session.detectedStatus` field when `status === ACTIVE` and `detectedStatus !== UNSPECIFIED`. No string parsing is involved — the typed enum value is stored directly.

**Specific scenario:**
```
Initial state: detectedStatusMap is empty
Action: upsertSession({ id: "s1", status: SessionStatus.ACTIVE, detectedStatus: DetectedStatus.EXECUTING, detectedContext: "running tests" })
Expected: detectedStatusMap["s1"] = { detectedStatus: DetectedStatus.EXECUTING, detectedContext: "running tests" }
```

**Fails before fix:** `upsertSession` does not read `session.detectedStatus`; `detectedStatusMap` is never populated by `upsertSession`.
**Passes after fix:** R4.3 logic is implemented; typed enum value is stored.

---

#### T-021
**Type:** Jest unit
**File:** `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` (extend existing)
**Requirements:** R4.3, R4.2 (ACTIVE + UNSPECIFIED case — adversarial review Issue 8)
**Epic coverage:** Epic 5 (Story 5.1, Task 5.1.1 — ACTIVE+UNSPECIFIED fix)

**What it verifies:**
When `upsertSession` receives an ACTIVE session with `detectedStatus = UNSPECIFIED` (or undefined), any existing `detectedStatusMap[id]` entry is CLEARED — not left stale. This addresses the adversarial review's Issue 8: after `StatusChangedEvent` removal, UNSPECIFIED from the server is the authoritative signal that no detection is available.

**Specific scenario:**
```
Initial state: detectedStatusMap["s1"] = { detectedStatus: DetectedStatus.PROCESSING, ... }
Action: upsertSession({ id: "s1", status: SessionStatus.ACTIVE, detectedStatus: DetectedStatus.UNSPECIFIED })
Expected: detectedStatusMap["s1"] === undefined
```

**Fails before fix:** Plan (pre-adversarial-review) said "leave stale" for ACTIVE+UNSPECIFIED. After the adversarial review patch, the behavior is "clear."
**Passes after fix:** ACTIVE+UNSPECIFIED path deletes the map entry.

---

#### T-022
**Type:** Jest unit
**File:** `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` (extend existing)
**Requirements:** R6.2
**Epic coverage:** Epic 5 (Story 5.2)

**What it verifies:**
`getDetectedStatusInfo` (or equivalent) returns the correct label for every `DetectedStatus` enum value when called with the typed enum (not a string). This is a table-driven test covering all enum values:

| Input | Expected output |
|---|---|
| `DetectedStatus.EXECUTING` | label "Executing" (or equivalent) |
| `DetectedStatus.PROCESSING` | label "Thinking…" |
| `DetectedStatus.NEEDS_APPROVAL` | label "Needs Approval" |
| `DetectedStatus.INPUT_REQUIRED` | label "Input Required" |
| `DetectedStatus.IDLE` | label "Idle" |
| `DetectedStatus.SUCCESS` | label "Success" |
| `DetectedStatus.ERROR` | label "Error" |
| `DetectedStatus.TESTS_FAILING` | label "Tests Failing" |
| `DetectedStatus.UNKNOWN` | `null` (no badge) |
| `DetectedStatus.UNSPECIFIED` | `null` (no badge) |
| `DetectedStatus.READY` | non-null label (e.g., "Ready") |
| `DetectedStatus.WAITING_FOR_AGENT` | non-null label |

**Fails before fix:** `StatusBadge` accepts a `string`, not `DetectedStatus` enum; this test file passes strings and the typed import doesn't exist yet.
**Passes after fix:** `StatusBadge` prop type is `DetectedStatus`; all enum values are handled; `UNKNOWN` returns null.

---

#### T-023
**Type:** Jest unit
**File:** `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` (extend existing)
**Requirements:** R6.3
**Epic coverage:** Epic 7 (Story 7.2)

**What it verifies:**
The `assertNever` exhaustiveness guard in `getDetectedStatusInfo` is reachable at runtime when an unhandled value is passed. Specifically: calling `getDetectedStatusInfo` with a numeric value that is not any known `DetectedStatus` member (simulating a future enum value added to the proto without updating the switch) throws an error. This confirms the `default: assertNever(status)` branch is wired correctly and not silently swallowed.

**Test implementation:** Cast an unknown integer (e.g., `999 as DetectedStatus`) and assert `getDetectedStatusInfo(999 as DetectedStatus)` throws.

**Fails before fix:** `StatusBadge` uses a string switch with no `assertNever`; unknown values return `undefined` silently.
**Passes after fix:** `assertNever` throws for any unhandled numeric value.

---

#### T-024
**Type:** Jest unit
**File:** `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` (extend existing)
**Requirements:** R6.2, R6.3
**Epic coverage:** Epic 5 (Story 5.2), Epic 7 (Story 7.2) — adversarial review Issue 6

**What it verifies:**
`StatusBadge` component renders correctly when `detectedStatus` prop is typed as `DetectedStatus` (not `number`). Verifies that:
1. A number literal passed as the prop is rejected at TypeScript compile time (compile-time test via `tsc --noEmit`)
2. A `DetectedStatus.EXECUTING` value renders the expected label in the DOM

**Specific render test:**
```tsx
render(<StatusBadge detectedStatus={DetectedStatus.EXECUTING} />)
expect(screen.getByText("Executing")).toBeInTheDocument()
```

**Fails before fix:** `StatusBadge` accepts `detectedStatus?: string`; TypeScript does not catch wrong types; no render for enum values.
**Passes after fix:** Prop type is `DetectedStatus`; component renders correctly with typed enum.

---

#### T-025
**Type:** Jest unit
**File:** `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts` (new)
**Requirements:** R5.3, R5.4
**Epic coverage:** Epic 6 (Story 6.1)

**What it verifies:**
`deriveWorkingState` produces the correct `WorkingState` for each combination of `subStatus` and `detectedStatus`. Covers the mapping cases defined in the plan's Task 6.1.1, verifying the frontend derivation matches the previous server-side `MapDetectedStatusToWorkingState` behavior.

**Specific cases:**
- `subStatus = SubStatus.PROCESSING` → `WorkingState.PROCESSING`
- `subStatus = SubStatus.NEEDS_APPROVAL` → `WorkingState.PROCESSING` (or WAITING — confirm against server impl)
- `subStatus = SubStatus.IDLE` → `WorkingState.IDLE`
- `subStatus = SubStatus.READY` → `WorkingState.IDLE`
- `subStatus = SubStatus.UNSPECIFIED`, `detectedStatus = DetectedStatus.EXECUTING` → `WorkingState.ACTIVE`
- `subStatus = SubStatus.UNSPECIFIED`, `detectedStatus = DetectedStatus.PROCESSING` → `WorkingState.ACTIVE`
- `subStatus = SubStatus.UNSPECIFIED`, `detectedStatus = DetectedStatus.UNKNOWN` → `WorkingState.UNSPECIFIED`

**Fails before fix:** `deriveWorkingState` does not exist.
**Passes after fix:** Function is implemented in `web-app/src/lib/utils/deriveWorkingState.ts`; all cases pass.

---

#### T-026
**Type:** Jest unit
**File:** `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` (extend existing)
**Requirements:** R5.4
**Epic coverage:** Epic 6 (Story 6.2)

**What it verifies:**
`ReviewQueuePanel` uses `deriveWorkingState` for filtering instead of the entity's `workingState` field. The test fixture is updated to NOT set `workingState` on the review item proto (leaving it at zero/UNSPECIFIED) and instead set `subStatus`. The panel's filtering logic correctly derives working state from `subStatus`.

**Specific change:** Line 279 fixture `workingState: 0` is replaced with `subStatus: SubStatus.PROCESSING`; the panel still correctly categorizes the item as "processing."

**Fails before fix:** Panel reads `item.workingState` from the entity; with field zeroed out, filtering shows wrong counts.
**Passes after fix:** Panel calls `deriveWorkingState(item)` which reads `item.subStatus`.

---

#### T-027
**Type:** Jest unit
**File:** `web-app/src/components/sessions/DetectionEventsPanel.test.tsx` (new) or extend if exists
**Requirements:** R1.4, R6.1
**Epic coverage:** Epic 2 (Task 2.1.5) — adversarial review Issue 5

**What it verifies:**
`DetectionEventsPanel` uses the proto-generated `DetectedStatus` TypeScript enum for its integer-to-name map, NOT a hardcoded `Record<number, string>`. The test verifies:
1. The panel renders the correct status name for `DetectedStatus.EXECUTING` (proto enum value 3)
2. The panel renders `"StatusWaitingForAgent"` correctly for the corresponding enum value
3. The panel does NOT use the old string `"StatusActive"` for any value (the old mapping was `8: "StatusActive"`)

**Fails before fix:** Hardcoded map `{ 8: "StatusActive" }` exists; after rename `StatusActive` is wrong. `StatusWaitingForAgent` is missing from the map.
**Passes after fix:** Panel uses proto enum reverse lookup; all constant names are correct and complete.

---

#### T-028
**Type:** Jest unit
**File:** `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` (extend existing)
**Requirements:** R4.1, R4.4
**Epic coverage:** Epic 5 (Story 5.3, Story 5.4)

**What it verifies:**
The `WatchSessions` stream handler dispatches ONLY `upsertSession` (and `removeSession`). After Epic 5, `updateSessionStatus` does not exist as an exported action. This test verifies:
1. Importing `updateSessionStatus` from `sessionsSlice` fails (action removed)
2. The `"statusChanged"` case in `useSessionService.ts` is removed — no export or handler for it exists

**Implementation note:** This is primarily a compile-time test verified by `tsc --noEmit` and Jest import checks. The test file attempts to import `updateSessionStatus` and asserts it is `undefined` (named import does not exist).

**Fails before fix:** `updateSessionStatus` is exported from `sessionsSlice`.
**Passes after fix:** Action is removed; import resolves to `undefined`.

---

#### T-029
**Type:** Jest unit
**File:** `web-app/src/components/sessions/__tests__/SessionCard.approval-suppression.test.tsx` (extend existing)
**Requirements:** R4.3, R6.2
**Epic coverage:** Epic 5 (Story 5.2, Task 5.2.3)

**What it verifies:**
`SessionCard` uses typed enum comparisons for `NEEDS_APPROVAL` and `INPUT_REQUIRED` badge suppression logic — not raw string equality. After the migration, the string checks `detectedStatus === "Needs Approval"` and `detectedStatus === "Input Required"` are replaced with `detectedStatus === DetectedStatus.NEEDS_APPROVAL` etc. The test verifies badge suppression still works with typed enum values.

**Fails before fix:** `SessionCard` checks `detectedStatus === "Needs Approval"` (string); with typed enum, the comparison always fails (enum value is a number, not a string).
**Passes after fix:** Typed enum comparisons work correctly; badge suppression is preserved.

---

#### T-030
**Type:** Jest unit
**File:** `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` (extend existing)
**Requirements:** R4.2
**Epic coverage:** Epic 5 (Story 5.1) — "stopped session previously Thinking shows no chip" acceptance criterion

**What it verifies:**
The specific acceptance criterion: "A stopped session that was previously 'Thinking…' shows no chip or badge after stopping." The test exercises the Redux store: a session with `detectedStatus = PROCESSING` (badge: "Thinking…") receives a `SessionUpdatedEvent` with `status = STOPPED`. After `upsertSession` is dispatched with the updated session, `selectDetectedStatusMap` returns an empty map for that session's ID.

**Scenario:**
```
1. dispatch(upsertSession({ id: "s1", status: ACTIVE, detectedStatus: DetectedStatus.PROCESSING }))
   → detectedStatusMap["s1"] = { detectedStatus: PROCESSING, ... }
2. dispatch(upsertSession({ id: "s1", status: STOPPED, detectedStatus: DetectedStatus.UNSPECIFIED }))
   → detectedStatusMap["s1"] === undefined
```

**Fails before fix:** Step 2 does not clear the map; the stale "Thinking…" entry remains.
**Passes after fix:** R4.2 clear-on-non-ACTIVE logic removes the entry.

---

#### T-031
**Type:** Jest unit
**File:** `web-app/src/lib/utils/__tests__/assertNever.test.ts` (new, or embedded in StatusBadge.test.tsx)
**Requirements:** R6.3
**Epic coverage:** Epic 7 (Story 7.2)

**What it verifies:**
`assertNever` utility function throws `Error` at runtime when called with any value, and the thrown error message includes a string representation of the unhandled value. This makes the exhaustiveness check observable in tests (not silently ignored).

**Fails before fix:** Function does not exist.
**Passes after fix:** `assertNever(999)` throws `Error("Unhandled case: 999")`.

---

#### T-032
**Type:** Jest unit
**File:** `web-app/src/lib/utils/notifications.test.ts` (extend existing — no-raw-string regression check)
**Requirements:** R6.1
**Epic coverage:** Epic 7 (Story 7.3)

**What it verifies:**
No raw detected-status string literal (`"Processing"`, `"Active"`, `"Executing"`, etc.) appears in `notificationMapping.ts` or `notifications.ts`. This is a source-code-level assertion: the test reads the source file content and asserts none of the banned strings appear outside of intentionally-suppressed comments.

**Implementation:** Jest test reads the compiled module's exports and verifies there are no string-typed detected status comparisons. Alternatively, this is covered by the ESLint rule (T-REG-001 below) and this test is a belt-and-suspenders check.

**Fails before fix:** Raw string comparisons may exist in notification mapping.
**Passes after fix:** All notification mappings use typed enum.

---

### Playwright E2E Tests

---

#### T-033
**Type:** Playwright e2e
**File:** `tests/e2e/session-status-badge.spec.ts` (new)
**Requirements:** R4.2, R6.2 — "stopped session shows no badge" acceptance criterion
**Epic coverage:** Epics 3, 4, 5 combined

**What it verifies:**
An active session displaying a "Thinking…" badge (PROCESSING state) stops unexpectedly. After the `SessionUpdatedEvent` with `status = STOPPED` is received by the frontend, the session card no longer shows the "Thinking…" chip or any detected-status badge. This is the user-visible manifestation of the stale badge bug fix.

**Setup:** Requires test server to produce controlled events. Use the test server at `http://localhost:8544`. A mock session is created that publishes a `SessionUpdatedEvent` with PROCESSING then immediately publishes another with STOPPED.

**Specific assertions:**
- After PROCESSING event: `[data-testid="detected-status-badge"]` exists and contains text "Thinking…"
- After STOPPED event: `[data-testid="detected-status-badge"]` does not exist or is not visible for that session
- `[data-testid="session-status-chip"]` shows STOPPED state (not PROCESSING)

**Fails before fix:** Stale badge lingers after session stop.
**Passes after fix:** Badge is cleared by `upsertSession` on STOPPED event.

---

#### T-034
**Type:** Playwright e2e
**File:** `tests/e2e/session-status-badge.spec.ts` (same file, new test case)
**Requirements:** R2.2
**Epic coverage:** Epic 2 (Story 2.2)

**What it verifies:**
A session producing terminal output that matches no specific detection pattern (catch-all fallback) shows NO detected-status badge. Previously, this would render "Ready" (from the `.*` catch-all). After the fix, it renders nothing (StatusUnknown → null badge).

**Setup:** Create a session that outputs random noise to the PTY. Verify `[data-testid="detected-status-badge"]` is absent.

**Fails before fix:** Catch-all renders a "Ready" badge.
**Passes after fix:** Catch-all is `StatusUnknown` → badge is null → no DOM element.

---

#### T-035
**Type:** Playwright e2e
**File:** `tests/e2e/session-status-badge.spec.ts` (same file, new test case)
**Requirements:** R2.2
**Epic coverage:** Epic 2 (Story 2.2)

**What it verifies:**
A session that outputs a readline/shell prompt (explicitly detected `StatusReady`) renders a "Ready" badge (or equivalent neutral indicator). This is the distinct-from-`StatusUnknown` case: explicit prompt detection is visible; catch-all is not.

**Fails before fix:** `StatusReady` and `StatusUnknown` both trigger from the `.*` catch-all; distinction is impossible to observe.
**Passes after fix:** Explicit prompt pattern triggers `StatusReady` which has a badge; arbitrary output triggers `StatusUnknown` which does not.

---

#### T-036
**Type:** Playwright e2e
**File:** `tests/e2e/session-status-badge.spec.ts` (same file, new test case)
**Requirements:** R4.3, R6.2
**Epic coverage:** Epic 5 (Story 5.2)

**What it verifies:**
An active session with `EXECUTING` detected status shows the correct label in the status badge (not "Active" — the old string — and not empty). This validates the frontend rename acceptance criterion (`StatusActive` → `StatusExecuting` → badge label "Executing").

**Specific assertion:** The DOM contains the badge label that corresponds to `DetectedStatus.EXECUTING`. The label is NOT "Active" (the old pre-rename string).

**Fails before fix:** Badge switch uses `case "Active":` but Go now returns `.String() = "Executing"`; the switch falls to default (empty); no badge renders.
**Passes after fix:** Badge switch uses `DetectedStatus.EXECUTING` (typed); correct label is rendered.

---

#### T-037
**Type:** Playwright e2e
**File:** `tests/e2e/review-queue-working-state.spec.ts` (new)
**Requirements:** R5.3, R5.4
**Epic coverage:** Epic 6 (Story 6.2)

**What it verifies:**
`ReviewQueuePanel` correctly categorizes sessions by working state after `MapDetectedStatusToWorkingState` is removed from the server. A session with `subStatus = SUB_STATUS_PROCESSING` (set via the `SessionUpdatedEvent`) appears in the "Processing" filter bucket, not "Unknown." This validates that `deriveWorkingState` is wired into `ReviewQueuePanel` and produces correct counts.

**Fails before fix:** Server no longer sends `working_state` on `ReviewItem`; panel reads it as zero; all sessions appear in "Unknown" bucket.
**Passes after fix:** Frontend derives working state from `subStatus`; sessions appear in correct buckets.

---

#### T-038
**Type:** Playwright e2e
**File:** `tests/e2e/session-status-badge.spec.ts` (same file) or `tests/e2e/session-lifecycle.spec.ts` (extend)
**Requirements:** R3.3 (no StatusChangedEvent), R4.4 (single Redux path)
**Epic coverage:** Epics 3, 4, 5 combined

**What it verifies:**
The WatchSessions stream emits ONLY `sessionUpdated` events (not `statusChanged`) after Epic 4. Verifies this by reading network requests in the browser: the `EventStream` payload for a session status change contains a `sessionUpdated` frame and no `statusChanged` frame.

**Implementation:** Use `mcp__claude-in-chrome__read_network_requests` or Playwright `page.route` to intercept the ConnectRPC stream. After a session changes status, assert the stream payload contains `"sessionUpdated"` and does not contain `"statusChanged"`.

**Fails before fix:** Stream emits `statusChanged` events.
**Passes after fix:** `statusChanged` proto field is reserved (not emitted); only `sessionUpdated` appears.

---

## 2. Requirement-to-Test Traceability Matrix

| Requirement | Tests | Covered? |
|---|---|---|
| **R1.1** Proto enum definition | Build (T-001 compile dependency) | ✓ (build) |
| **R1.2** Codegen produces Go + TS types | `make generate-proto` + build | ✓ (build) |
| **R1.3** `DetectedStatusToProto` mapping | T-001, T-002, T-011 | ✓ |
| **R1.4** Frontend uses typed enum, no raw strings | T-022, T-027, T-029, T-032 | ✓ |
| **R2.1** Rename `StatusActive → StatusExecuting` | T-003, T-004 | ✓ |
| **R2.2** `StatusReady` is not the `.*` catch-all | T-005, T-034, T-035 | ✓ |
| **R2.3** Update all switches and tests | T-003, T-004, T-012 | ✓ |
| **R2.4** Exhaustive switch enforcement | T-001, T-011, T-023 | ✓ |
| **R3.1** `SessionUpdatedEvent` carries `DetectedStatus` | T-010, T-012, T-015 | ✓ |
| **R3.2** All publish sites include detection fields | T-013, T-015, T-017 | ✓ |
| **R3.3** `StatusChangedEvent` removed | T-013, T-014, T-038 | ✓ |
| **R3.4** (not applicable — immediate removal chosen) | T-014 confirms removal | ✓ |
| **R4.1** `updateSessionStatus` removed / private | T-028 | ✓ |
| **R4.2** `upsertSession` clears map when not ACTIVE | T-019, T-030, T-033 | ✓ |
| **R4.3** `upsertSession` updates map from typed field | T-020, T-021, T-036 | ✓ |
| **R4.4** Stream handler dispatches `upsertSession` only | T-028, T-038 | ✓ |
| **R5.1** Remove `MapDetectedStatusToWorkingState` | T-016 | ✓ |
| **R5.2** Deprecate `working_state` proto field | Build-only | ~ (build) |
| **R5.3** `deriveWorkingState` pure function | T-025, T-037 | ✓ |
| **R5.4** `ReviewQueuePanel` uses `deriveWorkingState` | T-026, T-037 | ✓ |
| **R6.1** No raw string literals (ESLint rule) | T-032 (partial), lint | ~ (lint) |
| **R6.2** `StatusBadge` uses typed enum | T-022, T-024, T-029, T-033, T-036 | ✓ |
| **R6.3** `assertNever` exhaustiveness in every switch | T-023, T-031 | ✓ |

**Legend:** ✓ = covered by runtime test | ~ = covered by build/lint tooling only

**Coverage fraction:** 20/23 requirements have runtime tests. 3 requirements (R1.2, R5.2, R6.1) are verified by `make build`, `make generate-proto`, and `make lint` respectively — runtime tests cannot and should not replace these.

Counting sub-requirements from requirements.md: 32 of 36 sub-requirements have at least one runtime test (89%). The 4 uncovered sub-requirements are build/lint-only verified (R1.2, R5.2, R6.1, R3.4-as-n/a).

---

## 3. Blocker Resolution Checklist (Adversarial Review)

| Adversarial Issue | Resolution in Plan | Resolution in Test Suite |
|---|---|---|
| **Issue 1** — StatusChangedEvent removal deferred | Plan patched: atomic removal in Epic 4 | T-013, T-014 verify no StatusChangedEvent is published; T-009, T-018 verify push subscriber uses EventSessionUpdated |
| **Issue 2** — Field numbers 55/56 collision in Session proto | Plan patched: use fields 68/69 | T-010, T-012 will fail at proto decode time if wrong field numbers are used (data corruption catches itself) |
| **Issue 3** — StatusRateLimited doesn't exist; StatusWaitingForAgent missing | Plan patched: remove StatusRateLimited, add StatusWaitingForAgent | T-001 explicitly tests all 11 constants; compilation failure if StatusRateLimited is referenced |
| **Issue 4** — StatusReady design decision wrong | Plan patched: StatusReady keeps distinct definition | T-005 verifies catch-all returns StatusUnknown, not StatusReady; T-035 verifies StatusReady renders a badge |

All 4 blockers are addressed.

---

## 4. Risk Coverage (Adversarial Review Concerns)

| Concern | Test(s) that cover it |
|---|---|
| **Issue 5** — DetectionEventsPanel iota map silently misidentifies statuses | T-027 verifies the panel uses proto enum reverse lookup, not hardcoded ints; StatusActive string no longer appears |
| **Issue 6** — assertNever works only if prop type is DetectedStatus, not number | T-023, T-024 verify: (a) assertNever throws for unknown values; (b) StatusBadge prop type is typed and accepts DetectedStatus, not number |
| **Issue 7** — Analytics old_status lost without in-memory state | T-007 verifies analytics records new_status; T-008 verifies in-memory map is cleaned up on session deletion (bounds growth) |
| **Issue 8** — upsertSession ACTIVE+UNSPECIFIED leaves stale entry | T-021 is the direct test: ACTIVE + UNSPECIFIED clears the map entry, contradicting the pre-patch plan |

All 4 concerns are covered.

---

## 5. Atomic Commit Constraint Verification

The plan specifies two atomic constraints that test-suite coordination must address:

**Constraint A (Epic 2):** `StatusActive → StatusExecuting` Go rename + `StatusBadge.tsx` case update + `getDetectedStatusInfo` update must ship in one commit.

- **Test coordination:** T-004 (Go unit, confirms rename) + T-022 (Jest unit, confirms StatusBadge typed enum handles EXECUTING) + T-036 (e2e, confirms "Executing" label renders) must ALL pass simultaneously. A partial commit where the Go rename ships without the frontend update causes T-036 to fail (wrong label) and T-022 to fail (import of old constant fails).

**Constraint B (Epic 5):** `StatusBadge` migration + `upsertSession` redux unification + `SessionCard` string equality removal ship together.

- **Test coordination:** T-019/T-020/T-021 (upsertSession behavior) + T-022/T-024 (StatusBadge typed enum) + T-029 (SessionCard typed comparison) must all pass together. Partial shipping would cause type errors (SessionCard passes `DetectedStatus` to StatusBadge which still expects `string`, or vice versa).

**Constraint C (Epic 4 — atomic removal):** Push subscriber + analytics subscriber + StatusChangedEvent proto removal + struct removal all in one commit.

- **Test coordination:** T-009, T-018 (push subscriber uses EventSessionUpdated) + T-007, T-008 (analytics subscriber uses EventSessionUpdated) + T-013, T-014 (no StatusChangedEvent published) must all pass simultaneously. The compile step itself enforces the struct removal — if `EventSessionStatusChanged` is removed, all callers fail to compile.

---

## 6. Readiness Gate

### Criterion 1 — Requirements Coverage
**32/36 sub-requirements (89%) have at least one runtime test.** The 4 uncovered items are build/lint-only (R1.2, R5.2, R6.1, R3.4). This meets the coverage threshold. **PASS**

### Criterion 2 — Blocker Resolution
**All 4 adversarial-review blockers are addressed** either in the patched plan (Issues 1–4) or in the test suite (T-001 for Issue 3, T-005/T-035 for Issue 4, T-013/T-014 for Issue 1, T-010/T-012 for Issue 2). **PASS**

### Criterion 3 — Atomic Constraints
**All three atomic commit constraints have test coordination** described in §5. Tests are explicitly grouped by which atomic boundary they span; a CI run with partial commits will fail the corresponding grouped tests. **PASS**

### Criterion 4 — Risk Coverage (Adversarial Concerns)
**All 4 adversarial-review concerns are covered** by dedicated tests (T-027 for Issue 5, T-023/T-024 for Issue 6, T-007/T-008 for Issue 7, T-021 for Issue 8). **PASS**

---

### Overall Verdict: PASS

The test suite provides 89% runtime requirement coverage, addresses all 4 adversarial blockers, coordinates atomic commit constraints explicitly, and covers all 4 adversarial concerns. One open design decision remains (exact `deriveWorkingState` mapping in Task 6.1.1 — verify against current `MapDetectedStatusToWorkingState` before finalizing T-025/T-026). This does not block implementation — T-025 is designed to be updated once the exact mapping is confirmed.

---

## 7. Implementation File Index

| Test ID | File | New / Extend |
|---|---|---|
| T-001, T-002, T-011 | `session/detection/proto_mapping_test.go` | New |
| T-003, T-004 | `session/status_mapping_test.go` | Extend |
| T-005 | `session/detection/detector_test.go` | Extend |
| T-006 | `session/command_executor_test.go` | Extend |
| T-007, T-008 | `server/analytics/subscriber_test.go` | Extend |
| T-009, T-018 | `server/push/subscriber_test.go` | Extend |
| T-010 | `server/adapters/instance_adapter_test.go` | Extend or new |
| T-012 | `server/services/event_converter_test.go` | Extend or new |
| T-013, T-014, T-015 | `server/services/session_service_test.go` | Extend |
| T-016 | `server/adapters/review_queue_adapter_test.go` | Extend |
| T-017 | `server/services/session_status_pipeline_test.go` | New |
| T-019, T-020, T-021, T-028, T-030 | `web-app/src/lib/store/__tests__/sessionsSlice.test.ts` | Extend |
| T-022, T-023, T-024 | `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` | Extend |
| T-025 | `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts` | New |
| T-026 | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | Extend |
| T-027 | `web-app/src/components/sessions/DetectionEventsPanel.test.tsx` | New |
| T-029 | `web-app/src/components/sessions/__tests__/SessionCard.approval-suppression.test.tsx` | Extend |
| T-031 | `web-app/src/lib/utils/assertNever.ts` (runtime) + `assertNever.test.ts` | New |
| T-032 | `web-app/src/lib/utils/notifications.test.ts` | Extend |
| T-033, T-034, T-035, T-036, T-038 | `tests/e2e/session-status-badge.spec.ts` | New |
| T-037 | `tests/e2e/review-queue-working-state.spec.ts` | New |
