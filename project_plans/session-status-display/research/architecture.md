# Session Status Display — Architecture Research

## Full Data Flow

1. **tmux scrollback capture**
   - `session/claude_controller.go:518` — `cc.instance.Preview()` fetches terminal content via tmux `capture-pane`
   - `session/claude_controller.go:528` — tail-sliced to last `statusDetectionTailBytes` (4096) bytes; FNV-64a hash cache avoids re-detection if output hasn't changed

2. **Status detection: `StatusDetector.DetectWithContextFromLines()`**
   - `session/detection/detector.go:841` — `DetectWithContextFromLines(lines []string)` iterates lines in reverse (most recent first), skipping blank lines; first non-Ready non-Unknown match wins; StatusReady is kept as low-confidence fallback
   - `session/detection/detector.go:239` — inner `Detect(output []byte)` applies `collapseCarriageReturns` then `stripANSI`, then tests regex patterns in priority order: Error > TestsFailing > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready
   - Returns `DetectedStatus` (int iota): `StatusUnknown=0, StatusReady, StatusProcessing, StatusNeedsApproval, StatusInputRequired, StatusError, StatusTestsFailing, StatusIdle, StatusActive, StatusSuccess`
   - Called from `session/claude_controller.go:547`: `status, desc := cc.statusDetector.DetectWithContextFromLines(lines)`

3. **Status cached in ClaudeController**
   - `session/claude_controller.go:550` — result stored in `cc.statusCache` (`statusCacheEntry{tailHash, status, desc}`)
   - `session/claude_controller.go:905` — `runStatusChangeLoop()` polls `statusCheckCh`; calls `GetCurrentStatus()` each time; if status differs from `cc.lastEmittedStatus`, calls `cc.statusChangeListener(newStatus, sessionName)`

4. **Status-change callback → ReactiveQueueManager**
   - `server/services/session_service.go:3217` — `wireStatusChangeCallback()` sets callback: `inst.SetStatusChangeCallback(func(newStatus detection.DetectedStatus, _ string) { mgr.OnControllerStatusChange(inst, newStatus) })`
   - `server/review_queue_manager.go:173` — `OnControllerStatusChange()` calls `rqm.signalActivity()` and spawns goroutine calling `rqm.poller.CheckSession(inst)`

5. **Instance.GetDetectedStatus() — on-demand read**
   - `session/instance_state.go:152` — `GetDetectedStatus()` calls `mgr.GetStatus(i)` → `InstanceStatusManager.GetStatus()` → `controller.GetCurrentStatus()` (cached)
   - Returns `detection.StatusUnknown` when no controller is active

6. **toProtoSubStatus() — mapping DetectedStatus → proto SubStatus**
   - `server/adapters/instance_adapter.go:191` — called inside `InstanceToProto()` at line 134
   - Logic (lines 191–213):
     - `inst.Status != session.Active` → `SUB_STATUS_UNSPECIFIED`
     - `ratelimit.StateWaiting` → `SUB_STATUS_RATE_LIMITED` (takes precedence)
     - `StatusProcessing | StatusActive` → `SUB_STATUS_PROCESSING`
     - `StatusNeedsApproval | StatusInputRequired` → `SUB_STATUS_NEEDS_APPROVAL`
     - `StatusError` → `SUB_STATUS_ERROR`
     - `StatusTestsFailing` → `SUB_STATUS_TESTS_FAILING`
     - `StatusReady | StatusIdle` → `SUB_STATUS_IDLE`
     - all others (Unknown, Success) → `SUB_STATUS_UNSPECIFIED`

7. **Proto Session message — `sub_status` field**
   - `proto/session/v1/types.proto:178` — `SubStatus sub_status = 54;`
   - `proto/session/v1/types.proto:332–346` — `enum SubStatus { SUB_STATUS_UNSPECIFIED=0, SUB_STATUS_IDLE=1, SUB_STATUS_PROCESSING=2, SUB_STATUS_NEEDS_APPROVAL=3, SUB_STATUS_ERROR=4, SUB_STATUS_TESTS_FAILING=5, SUB_STATUS_RATE_LIMITED=6 }`
   - Derived at read time; never stored in the database (comment at line 331)

8. **WatchSessions stream → frontend**
   - `server/services/session_service.go:1423` — `WatchSessions()` sends initial snapshot via `createInitialSnapshotEvent(inst)` (which calls `InstanceToProto(inst)`), then streams events from `eventBus`
   - `server/services/session_service.go:1498` — real-time events converted via `convertEventToProto(event)` and streamed

9. **useSessionService hook — dispatch to Redux**
   - `web-app/src/lib/hooks/useSessionService.ts:180` — initial list: `dispatch(setSessions(response.sessions))`
   - `web-app/src/lib/hooks/useSessionService.ts:235` — watch events: `dispatch(upsertSession(response.session))`
   - Sessions (including `subStatus` field) stored in Redux via `sessionsSlice` entity adapter

10. **SessionList → SessionRow render**
    - `web-app/src/components/sessions/SessionList.tsx:387` — optional filter: `if (filterNeedsApproval && session.subStatus !== SubStatus.NEEDS_APPROVAL)`
    - `web-app/src/components/sessions/SessionRow.tsx:103` — receives `session: Session` prop directly from Redux store via `selectAllSessions`
    - SessionRow reads `session.subStatus` from the proto object

11. **SessionRow guard condition**
    - `web-app/src/components/sessions/SessionRow.tsx:172–177`
    ```tsx
    {session.status === SessionStatus.ACTIVE &&
      session.subStatus !== SubStatus.UNSPECIFIED &&
      session.subStatus !== SubStatus.IDLE &&
      !(suppressApprovalSubStatus && session.subStatus === SubStatus.NEEDS_APPROVAL) && (
        <SubStatusChip subStatus={session.subStatus} />
      )}
    ```
    - Three guard conditions before render: (a) session must be ACTIVE lifecycle status, (b) subStatus must not be UNSPECIFIED, (c) subStatus must not be IDLE
    - Optional fourth condition: if `suppressApprovalSubStatus=true` prop is set, also suppresses NEEDS_APPROVAL chip (used for optimistic UI during approval clear)

12. **SubStatusChip render**
    - `web-app/src/components/sessions/SubStatusChip.tsx:22`

---

## SubStatusChip Prop Interface

```typescript
// web-app/src/components/sessions/SubStatusChip.tsx:13–15
interface SubStatusChipProps {
  subStatus: SubStatus;
}
```

Single prop: `subStatus: SubStatus` (the proto enum imported from `@/gen/session/v1/types_pb`).

---

## SubStatusChip Render Cases

File: `web-app/src/components/sessions/SubStatusChip.tsx`

| Case | Line | Returns |
|---|---|---|
| `SubStatus.PROCESSING` | 24 | `<span className={chipProcessing}>` with CSS spinner + "Thinking…" text; `role="status"`, `aria-label="Session is processing"` |
| `SubStatus.NEEDS_APPROVAL` | 37 | `<span className={chipNeedsApproval}>` with "🔔 Needs Approval" text |
| `SubStatus.ERROR` | 49 | `<span className={chipError}>` with "✖ Error" text |
| `SubStatus.TESTS_FAILING` | 61 | `<span className={chipTestsFailing}>` with "⚠ Tests Failing" text |
| `SubStatus.RATE_LIMITED` | 73 | `<span className={chipRateLimited}>` with "⏱ Rate Limited" text |
| `SubStatus.IDLE` | 85 | `null` — no chip rendered |
| `SubStatus.UNSPECIFIED` | 86 | `null` — no chip rendered |
| `default` | 87 | `null` — no chip rendered |

---

## SessionRow Guard Condition (exact code + line number)

File: `web-app/src/components/sessions/SessionRow.tsx`, lines 172–177:

```tsx
{session.status === SessionStatus.ACTIVE &&
  session.subStatus !== SubStatus.UNSPECIFIED &&
  session.subStatus !== SubStatus.IDLE &&
  !(suppressApprovalSubStatus && session.subStatus === SubStatus.NEEDS_APPROVAL) && (
    <SubStatusChip subStatus={session.subStatus} />
  )}
```

The guard that would need to change for a feature adding a new visible chip state: the `!== SubStatus.UNSPECIFIED` and `!== SubStatus.IDLE` checks. Adding a new `SubStatus` enum value that should render (e.g. `SUB_STATUS_SUCCESS`) requires no guard change here — the `default: return null` in SubStatusChip means new values are silently hidden until a case is added. The proto enum, `toProtoSubStatus()` switch in `instance_adapter.go:191`, and `SubStatusChip` switch at `SubStatusChip.tsx:23` are the three co-located registration points.

---

## Key Files Summary

| File | Role |
|---|---|
| `session/detection/detector.go:239` | `Detect()` — regex pattern matching on PTY bytes |
| `session/detection/detector.go:841` | `DetectWithContextFromLines()` — line-by-line reverse scan |
| `session/detection/idle.go:126` | `DetectStateFromContent()` — maps DetectedStatus → IdleState (alternative path for idle detection) |
| `session/claude_controller.go:510` | `GetCurrentStatus()` — calls detector, caches result |
| `session/claude_controller.go:896` | `runStatusChangeLoop()` — goroutine that emits status-change callbacks |
| `session/instance_state.go:152` | `Instance.GetDetectedStatus()` — on-demand read via InstanceStatusManager |
| `server/adapters/instance_adapter.go:191` | `toProtoSubStatus()` — DetectedStatus → proto SubStatus enum |
| `server/adapters/instance_adapter.go:14` | `InstanceToProto()` — full Session proto construction including SubStatus at line 134 |
| `server/review_queue_manager.go:173` | `OnControllerStatusChange()` — triggers CheckSession on status change |
| `server/services/session_service.go:1423` | `WatchSessions()` — streams Session protos to frontend |
| `server/services/session_service.go:3217` | `wireStatusChangeCallback()` — wires controller callback to ReactiveQueueManager |
| `proto/session/v1/types.proto:178,332` | `sub_status` field + `SubStatus` enum definition |
| `web-app/src/lib/hooks/useSessionService.ts:180,235` | Dispatches sessions into Redux store |
| `web-app/src/components/sessions/SessionRow.tsx:172` | Guard condition before `<SubStatusChip>` render |
| `web-app/src/components/sessions/SubStatusChip.tsx:22` | Chip render switch |
