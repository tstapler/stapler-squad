# Pitfalls & Risks: Session Status Unification

## 1. `StatusActive` Rename Blast Radius

**Go source files (non-test, non-generated):** 16 direct references across 8 files.

| File | Line(s) | Context |
|---|---|---|
| `session/detection/detector.go` | 27, 636, 684, 768, 789, 793, 795, 801 | Enum declaration + switch cases + scan logic |
| `session/detection/events.go` | 67 | Switch case in event emission |
| `session/detection/idle.go` | 182 | Switch case |
| `session/detection/pattern_set.go` | 125, 136 | Pattern match return values |
| `session/claude_controller.go` | 662, 672 | Fallback assignment for spinner detection |
| `session/instance_status.go` | 165 | Switch case for proto mapping |
| `session/review_queue_determiner.go` | 230 | Case in working-state derivation |
| `session/status_mapping.go` | 32 | Case group in lifecycle mapping |
| `server/adapters/instance_adapter.go` | 215, 393 | `toProtoSubStatus` and `MapDetectedStatusToWorkingState` |

**Test files:** 5 additional references in `claude_controller_test.go`, `review_queue_reactive_test.go`, `status_mapping_test.go`.

**Critical risk:** The rename is purely mechanical, but `StatusBadge.tsx` switches on the raw string value `"Active"` (line 54). The Go enum's `.String()` method returns `"Active"` today. If `StatusExecuting` returns `"Executing"`, the badge case `"Active"` becomes a dead branch that silently falls to the `default` (renders the raw enum string with a plain dot icon). There is no TypeScript compile error, no test failure — the badge just goes dark. The `detectedStatus` prop in `SessionCard.tsx` is typed as `string`, so the compiler cannot catch the mismatch.

**Mitigation required:** Change the `StatusBadge` case from `"Active"` to `"Executing"` (or to the new proto enum integer) atomically with the Go rename. Without that change the badge breaks on the first deploy.

---

## 2. `StatusReady` Usage: Semantic vs. Catch-All

`detection.StatusReady` appears in 30+ locations across 12 files (non-test, non-generated). These fall into two distinct semantic categories that must not be conflated during the rename:

### Category A: Catch-all fallback (the problem; rename to `StatusUnknown`)
- `session/detection/detector.go` — the `.*` pattern returns `StatusReady`; scan logic uses `StatusReady` as a low-confidence marker
- `session/detection/pattern_set.go:147` — `return StatusReady, ...` when the Ready patterns match (these are the `.*` catch-all patterns in JSON config)
- `session/detection/events.go:83` — switch case for event emission
- `session/detection/idle.go:197` — grouped with `StatusIdle` for idle detection

### Category B: Semantic uses that must be re-evaluated (they mean "prompt visible" or "ready to accept input")
- `session/command_executor.go:52` — `DefaultExecutionOptions` lists `StatusReady` as a terminal status, meaning "command completed, prompt returned." After rename to `StatusUnknown`, this would mean "unknown status is terminal" — **this is wrong**. Must be updated to `StatusIdle` or explicitly handled.
- `session/command_executor.go:381` — `result.Success = (status == detection.StatusReady)`. Success is currently defined as reaching the `.*` catch-all. After the rename, this logic breaks silently.
- `session/autonomous_driver.go:323` — `isIdleOrComplete` treats `StatusReady` as a resting state alongside `StatusIdle`. After rename this needs `StatusUnknown` added if that meaning is preserved, or removed.
- `session/claude_controller.go:670,678` — spinner detection guards that upgrade `StatusReady` → `StatusActive` when spinner verbs are present.
- `session/instance_status.go:123,161` — switch cases for proto sub-status mapping: `StatusReady` maps to a specific `SubStatus`.

**Critical risk:** `command_executor.go`'s success check (`result.Success = (status == detection.StatusReady)`) is currently relying on the catch-all being the "normal completion" signal. If `StatusReady` is renamed to `StatusUnknown` and command execution waits for `StatusUnknown` as its terminal condition, commands will report `Success = false` even when they complete normally. The fix is to change `DefaultExecutionOptions` to `StatusIdle` and update the success check — but this changes runtime behavior of `CommandExecutor` and needs careful testing.

**Collateral `StatusReady` constants** (42 occurrences, not in `detection` package, safe to ignore):
- `session.BacklogStatusReady` — backlog item lifecycle, unrelated to PTY detection
- `session/vnc.VNCStatusReady` — VNC readiness, unrelated
- `session/detection/ratelimit.SessionStatusReady` — rate-limit manager internal state, unrelated

---

## 3. `WorkingState` Removal Checklist

The `WorkingState` enum and its derivation functions touch 5 layers. Complete removal requires:

### Proto layer
- `proto/session/v1/types.proto:163` — `WorkingState working_state = 50` on `Session` message
- `proto/session/v1/types.proto:557` — `WorkingState working_state = 20` on `ReviewItem` message
- `proto/session/v1/events.proto:66` — `WorkingState working_state = 6` on `SessionStatusChangedEvent`
- `proto/session/v1/types.proto` — `WorkingState` enum definition itself
- Run `make generate-proto` to regenerate Go + TypeScript bindings

### Go server layer
- `server/adapters/instance_adapter.go:372-384` — `MapIdleStateToWorkingState` function
- `server/adapters/instance_adapter.go:388-420` — `MapDetectedStatusToWorkingState` function
- All call sites that populate `working_state` on Session/ReviewItem protos in the adapter

### Frontend TypeScript layer
- `web-app/src/components/sessions/ReviewQueuePanel.tsx:12,182,183,190,200,201,409` — imports `WorkingState` from generated types and uses it for filtering/sorting
- `web-app/src/lib/store/reviewQueueSlice.ts:3,89,90,94-106` — imports `WorkingState`, uses it in `selectNeedsAttentionItems` and `selectQueueCountsByWorkingState`
- `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx:279` — test fixture uses `workingState: 0`
- All these must be replaced by a new `deriveWorkingState(session)` utility per R5.3

**Critical risk:** `ReviewQueuePanel` and `reviewQueueSlice` both import `WorkingState` directly from the generated TypeScript proto bindings. Removing the proto field or enum causes a TypeScript compile error that blocks `make build`. The frontend replacement function (`deriveWorkingState`) must be written and wired before the proto field can be deleted. Deprecating-then-removing (two PRs) is safer than a single-PR deletion.

---

## 4. `StatusChangedEvent` Consumers: Full Removal Checklist

`StatusChangedEvent` (`EventSessionStatusChanged`) is both published and consumed in multiple places. If deprecated per R3.3, all consumers must be migrated.

### Publishers (emit the event)
| File | Line | Context |
|---|---|---|
| `server/services/session_service.go:1457` | Only publisher in non-test code | Lifecycle transition (Running→Stopped etc.) via `UpdateSession` RPC |
| `pkg/events/types.go:98-102` | Event factory | `NewSessionStatusChangedEvent` constructor |

Note: The detection-driven `StatusChangedEvent` emission (for PTY pattern changes) is NOT present in production Go source — only the lifecycle one. The `detected_status` fields are populated by `event_converter.go` at stream-conversion time by reading from `InstanceStatusManager`.

### Consumers (handle the event)
| File | Lines | What it does |
|---|---|---|
| `server/services/event_converter.go:43-54` | WatchSessions stream converter | Converts Go event → proto `SessionEvent_StatusChanged` for the gRPC stream. **This is what the frontend receives.** |
| `server/push/subscriber.go:92,111,195` | Push notification gating | `shouldNotify`: fires web-push notification only when `newStatus == Stopped`. `buildDeliveryNotification`: builds "Session Completed" push. `buildNotificationForSession`: constructs approval notification. |
| `server/analytics/subscriber.go:73-87` | Analytics event recording | Records `session.status_changed` event with old/new status labels |

**Critical risk:** `server/push/subscriber.go` handles push notifications specifically via `EventSessionStatusChanged`. If this event is removed, session completion push notifications (sent when a session stops) will stop working silently — no compile error, no test failure. The push notification path must be migrated to listen to `EventSessionUpdated` instead before `EventSessionStatusChanged` is removed. The analytics subscriber (`server/analytics/subscriber.go`) similarly would stop recording lifecycle transitions.

**Frontend consumer:**
- `web-app/src/lib/hooks/useSessionService.ts:730-740` — `case "statusChanged":` dispatches `updateSessionStatus` Redux action with `sessionId`, `newStatus`, `detectedStatus`, `detectedContext`
- `web-app/src/lib/store/sessionsSlice.ts:61-98` — `updateSessionStatus` reducer manages `detectedStatusMap`
Per R4, this case must be replaced by dispatching `upsertSession` with a partial session proto.

---

## 5. E2E Tests: Status Text Assertions

No E2E tests assert on `"Thinking"`, `"Active ⚡"`, or sub-status chip content for session cards.

The E2E tests that reference status badges are exclusively in the **backlog** feature and assert on the **backlog item** status (a separate domain: `"Ready"` means a backlog item is ready to spawn, not a PTY-detected status):
- `tests/e2e/backlog.spec.ts:404` — `toHaveAttribute('aria-label', 'Status: Ready')` — this is `BacklogStatusReady`, not `detection.StatusReady`
- `tests/e2e/pages/BacklogPage.ts:214` — `toContainText('Ready')` — same

The `enter-detection.spec.ts` test asserts on `"Needs Approval"` badge text (line 62), which maps to `AttentionReason.APPROVAL_PENDING` in `StatusBadge.tsx` — this path does not go through `getDetectedStatusInfo` and is unaffected by the rename.

**Safe to proceed:** The rename of `StatusActive` → `StatusExecuting` (and resulting `"Active"` → `"Executing"` in badge text) will NOT break any existing E2E tests. However, if future tests are added asserting `"Active ⚡"`, they would break when the badge text changes.

---

## 6. Proto Wire Compatibility: Adding `detectedStatus` Field to `Session`

Proto3 wire compatibility when adding a new `detected_status` field to the `Session` message:

**Old client receiving new server response:**
- The new `detected_status` field (typed enum) is unknown to old clients (old browser tabs, old mobile app at `https://onyx.staplerhome.internal:8444`)
- Proto3 unknown fields are silently ignored; old clients see zero/default value for the field (equivalent to `DETECTED_STATUS_UNSPECIFIED`)
- The frontend currently uses `detectedStatusMap[session.id]` (a separate Redux entry populated by `StatusChangedEvent`) rather than a `session.detectedStatus` field — so old clients reading the session entity would not regress: they would just not see the new field

**New client receiving old server response:**
- If a new browser tab connects to an old server binary (e.g., during a rolling deploy), the new `detectedStatus` field will read as `DETECTED_STATUS_UNSPECIFIED` (0)
- This is the same as the current "no detected status" state — the badge simply shows nothing
- No crash, no error; gracefully degrades

**Mobile app concern:**
- The mobile app connects via private CA cert to the LAN server. If the app binary is old but the server is new, the session proto response includes the new field but the old app ignores it.
- If the new server stops sending `StatusChangedEvent` (R3.3 deprecation) before the old mobile app is updated, the mobile app's `detectedStatusMap` will stop updating, leaving stale badges.
- **Mitigation:** The requirements correctly require R3.4: keep dual-emission during the transition period so old clients' `statusChanged` stream handlers continue to fire.

**Field numbering risk:**
- `proto/session/v1/session.proto` already uses field number `2393: int32 result_status = 5` inside a nested message. The `Session` message uses up to field 54+ (for `sub_status`). A new `detected_status` field on `Session` needs a fresh field number above existing ones (≥55 or in an unused range) to avoid collision.
- Accidentally reusing a deleted field number (if `working_state = 50` is removed before `detected_status` is added with a new number) is safe in proto3 since field numbers are not recycled by the toolchain — but code review must verify no number collision.

---

## Summary

**Most important pitfalls:**

1. **StatusBadge string coupling is a silent breakage vector.** Renaming `StatusActive` to `StatusExecuting` changes `.String()` output from `"Active"` to `"Executing"`. The `StatusBadge.tsx` switch case `"Active"` silently becomes unreachable — the badge renders a plain dot with `variant: "unknown"` instead of `"⚡ Active"`. No compile error, no existing test failure. This must be fixed atomically with the Go rename.

2. **`CommandExecutor` success logic depends on `StatusReady` being the normal completion signal.** `result.Success = (status == detection.StatusReady)` at `command_executor.go:381` means commands succeed when the `.*` catch-all fires. Renaming `StatusReady` to `StatusUnknown` without updating this check makes all `CommandExecutor` runs report `Success = false`. The `DefaultExecutionOptions` terminal status list at line 52 likewise must move from `StatusReady` to `StatusIdle`.

3. **`StatusChangedEvent` removal breaks push notifications silently.** `server/push/subscriber.go` handles web-push "Session Completed" notifications exclusively through `EventSessionStatusChanged`. Removing this event type (R3.3) without migrating `push/subscriber.go` to `EventSessionUpdated` means users stop receiving completion push notifications — no compile error, no test failure detects this.
