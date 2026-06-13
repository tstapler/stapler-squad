# Validation Plan: smart-notification-dedup

**Date**: 2026-05-29

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| FR-1: Condition-change gating | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_StableCount_NoRepeatAlert` | Unit | Stable zombie count → alert fires once, not twice within cooldown window |
| FR-1: Condition-change gating | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_StableCount_SuppressedForEntireCooldown` | Unit | Unchanged pressure at warning level for 3× cooldown duration → exactly 1 alert |
| FR-1: Condition-change gating | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_WorsenedCount_BypassesCooldown` | Unit | Count rises mid-cooldown → second alert fires immediately |
| FR-2: Count baseline after notification | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_BaselineRecorded_AfterFiring` | Unit | After alert fires, `lastAlertZombieCount` and `lastAlertFailureCount` match `stats` values |
| FR-2: Count baseline after notification | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_BaselineNotUpdated_WhenSuppressed` | Unit | Suppressed (unchanged) call does not mutate baseline fields |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_closeHighPriority_When_30sElapsed` | Unit | High-priority notification: `close()` called after 30 000 ms via fake timers |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_closeMediumPriority_When_15sElapsed` | Unit | Medium-priority notification (default): `close()` called after 15 000 ms |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_closeActionable_When_5minElapsed` | Unit | `requireInteraction: true` notification: `close()` called after 300 000 ms |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_clearMapEntry_When_timerFires` | Unit | After TTL timer fires, Map entry for that tag is removed |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/notification-policy.test.ts` | `nativeAutoCloseMs_should_return30000_When_priorityIsUrgent` | Unit | `URGENT` priority → 30 000 ms |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/notification-policy.test.ts` | `nativeAutoCloseMs_should_return30000_When_priorityIsHigh` | Unit | `HIGH` priority → 30 000 ms |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/notification-policy.test.ts` | `nativeAutoCloseMs_should_return15000_When_priorityIsMedium` | Unit | `MEDIUM` priority → 15 000 ms |
| FR-3: Native notification auto-dismiss | `web-app/src/lib/notification-policy.test.ts` | `nativeAutoCloseMs_should_return15000_When_priorityIsUnspecified` | Unit | `UNSPECIFIED` default branch → 15 000 ms |
| FR-4: Close-before-open dedup | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_closeFirst_When_sameTagCalledTwice` | Unit | Second call with same tag closes first notification before creating new one |
| FR-4: Close-before-open dedup | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_notCloseOther_When_differentTagUsed` | Unit | Second call with different tag does NOT close first notification |
| FR-4: Close-before-open dedup | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_dedupUntagged_When_noTagProvided` | Unit | Two calls with no `tag` both share `__untagged__` slot; first is closed on second call |
| FR-5: Re-arm on clear | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_ClearAndRearm` | Unit | Zombies → alert fires; count drops to 0 (ring expires) → fresh zombies → second alert fires |
| FR-5: Re-arm on clear | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_BaselineResetOnClear` | Unit | After level returns to OK, `lastAlertZombieCount == 0` and `lastAlertLevel == ForkPressureOK` |
| FR-5: Re-arm on clear | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_NoAlertOnClear` | Unit | Transition from critical → OK does NOT fire an alert callback |
| FR-6: Consistent toast + native | `web-app/src/lib/hooks/useSessionNotifications.test.ts` | `handleNotification_should_callShowBrowserNotification_When_nonHistoryEvent` | Unit | Non-history event triggers both `addNotification` and `showBrowserNotification` |
| FR-6: Consistent toast + native | `web-app/src/lib/hooks/useSessionNotifications.test.ts` | `handleNotification_should_notCallShowBrowserNotification_When_historyOnlyEvent` | Unit | History-only event does NOT trigger `showBrowserNotification` |
| FR-6: Consistent toast + native | `web-app/src/lib/hooks/useSessionNotifications.test.ts` | `handleNotification_should_useApprovalTag_When_approvalIdPresent` | Unit | Approval event uses `approval:<id>` tag so each approval gets a distinct native notification |
| NFR-1: Approvals never suppressed | `session/tmux/fork_metrics_test.go` | `TestCheckPressure_ApprovalsUnaffected` | Unit | `APPROVAL_NEEDED` events bypass `checkPressure` entirely (callback registered through server.go, not forkMonitor) — test that forkMonitor alert callbacks do not receive approval-type payloads |
| NFR-1: Approvals never suppressed | `web-app/src/lib/hooks/useSessionNotifications.test.ts` | `handleNotification_should_neverSuppressApproval_When_dedupWindowActive` | Unit | Approval event fires native notification even when same (sessionId, type) was recently shown |
| NFR-1: Approvals never suppressed | `web-app/src/lib/utils/notifications.test.ts` | `showBrowserNotification_should_useActionableTTL_When_requireInteractionTrue` | Unit | `requireInteraction: true` → TTL is `NATIVE_ACTIONABLE_TTL_MS` (5 min), not 15 s default |

---

## Detailed Test Specifications

### Go Unit Tests — `session/tmux/fork_metrics_test.go`

All tests are package-internal (`package tmux`) to access unexported fields. Use `go test -race ./session/tmux/...` to catch data races.

#### `TestCheckPressure_StableCount_NoRepeatAlert`
```
Setup:   alertCount := 0; register alertFn incrementing alertCount
         Inject zombies to reach Warning level (lastAlertZombieCount = N)
         fire at t=0 → alertCount == 1
Action:  Advance clock by 1 min (inside 2-min cooldown); inject same N zombies
         call checkPressure(t+60s)
Assert:  alertCount still == 1
```

#### `TestCheckPressure_StableCount_SuppressedForEntireCooldown`
```
Setup:   3 cooldown windows (6 min). Zombie count remains constant at N.
Action:  Call checkPressure every 30 s for 6 min.
Assert:  alertCount == 1 (fired once only; subsequent calls suppressed each window)
```

#### `TestCheckPressure_WorsenedCount_BypassesCooldown`
```
Setup:   Inject N zombies → checkPressure at t=0 → alertCount == 1
         (baseline: lastAlertZombieCount = N)
Action:  Advance 30 s (inside cooldown); inject 5 more zombies (total N+5)
         checkPressure(t+30s)
Assert:  alertCount == 2 (worsened count bypassed cooldown)
```

#### `TestCheckPressure_LevelEscalation_BypassesCooldown`
```
Setup:   Spawn failures at Warning level → alert fires; lastAlertLevel = Warning
Action:  Advance 30 s; ramp failures to Critical level
         checkPressure(t+30s)
Assert:  alertCount == 2 (level escalation bypasses cooldown)
```

#### `TestCheckPressure_BaselineRecorded_AfterFiring`
```
Setup:   Inject Z zombies, F failures → checkPressure fires
Action:  Read forkMonitor.lastAlertZombieCount, lastAlertFailureCount, lastAlertLevel
Assert:  lastAlertZombieCount == Z, lastAlertFailureCount == F, lastAlertLevel == stats.Level
```

#### `TestCheckPressure_BaselineNotUpdated_WhenSuppressed`
```
Setup:   Alert fires with baseline (N, 0, Warning)
Action:  Unchanged counts → suppressed call
Assert:  forkMonitor.lastAlertZombieCount still == N (baseline not overwritten)
```

#### `TestCheckPressure_ClearAndRearm`
```
Setup:   N zombies → alert at t=0 (baseline N)
         Advance 35 s (ring entries age out); no new events → level returns to OK
         checkPressure called → re-arm (baseline reset to 0)
Action:  Inject 5 new zombies → checkPressure
Assert:  alertCount == 2 (fresh alert fires after re-arm)
```

#### `TestCheckPressure_BaselineResetOnClear`
```
Setup:   Alert fires (lastAlertZombieCount = N, lastAlertLevel = Warning)
Action:  Simulate OK level → checkPressure
Assert:  lastAlertZombieCount == 0, lastAlertFailureCount == 0, lastAlertLevel == ForkPressureOK
```

#### `TestCheckPressure_NoAlertOnClear`
```
Setup:   Register alertFn; alert fires at Warning
Action:  Simulate OK level → checkPressure
Assert:  alertCount still == 1 (clear does not fire alert callbacks)
```

#### `TestCheckPressure_ApprovalsUnaffected`
```
Assert:  forkMonitor.alertFns contains only fork-pressure callbacks.
         Approval events are routed through server.go callback, not forkMonitor.
         This is a structural test: enumerate registered alertFns and verify none
         has an ApprovalNeeded payload shape. (Confirm the contract is not broken
         by the new gating logic.)
Note:    If checkPressure only receives ForkPressureStats, approval paths are
         architecturally impossible to reach here — test documents this invariant.
```

---

### Jest Unit Tests — `web-app/src/lib/utils/notifications.test.ts`

All tests use `jest.useFakeTimers()` and mock `window.Notification` via `Object.defineProperty` with `configurable: true`.

#### Mock setup (shared `beforeEach`)
```typescript
let mockClose: jest.Mock;
let notifInstances: Array<{ close: jest.Mock; onclose: (() => void) | null }>;

beforeEach(() => {
  jest.useFakeTimers();
  mockClose = jest.fn();
  notifInstances = [];
  const MockNotif = jest.fn().mockImplementation((_title, _opts) => {
    const inst = { close: jest.fn(), onclose: null as (() => void) | null };
    notifInstances.push(inst);
    return inst;
  });
  Object.defineProperty(window, 'Notification', {
    value: Object.assign(MockNotif, { permission: 'granted' }),
    writable: true, configurable: true,
  });
  // Reset module-level Map between tests by re-importing (or expose a reset helper)
});
```

#### `showBrowserNotification_should_closeFirst_When_sameTagCalledTwice`
```
Action:  call showBrowserNotification("A", { tag: "sess:fork" })
         call showBrowserNotification("B", { tag: "sess:fork" })
Assert:  notifInstances[0].close called once (before second Notification constructed)
         notifInstances.length == 2
```

#### `showBrowserNotification_should_notCloseOther_When_differentTagUsed`
```
Action:  call showBrowserNotification("A", { tag: "sess:fork" })
         call showBrowserNotification("B", { tag: "sess:tmux" })
Assert:  notifInstances[0].close NOT called
         notifInstances.length == 2
```

#### `showBrowserNotification_should_dedupUntagged_When_noTagProvided`
```
Action:  call showBrowserNotification("A")  // no tag
         call showBrowserNotification("B")  // no tag
Assert:  notifInstances[0].close called once (shared __untagged__ slot)
```

#### `showBrowserNotification_should_closeHighPriority_When_30sElapsed`
```
Action:  call showBrowserNotification("X", { tag: "t1", autoCloseMs: 30_000 })
         jest.advanceTimersByTime(29_999)
Assert:  notifInstances[0].close NOT called
Action:  jest.advanceTimersByTime(1)
Assert:  notifInstances[0].close called once
```

#### `showBrowserNotification_should_closeMediumPriority_When_15sElapsed`
```
Action:  call showBrowserNotification("X", { tag: "t2" })  // defaults to NATIVE_MEDIUM_TTL_MS
         jest.advanceTimersByTime(14_999)
Assert:  close NOT called
         jest.advanceTimersByTime(1) → close called once
```

#### `showBrowserNotification_should_closeActionable_When_5minElapsed`
```
Action:  call showBrowserNotification("X", { tag: "t3", requireInteraction: true })
         jest.advanceTimersByTime(299_999)
Assert:  close NOT called
         jest.advanceTimersByTime(1) → close called once
```

#### `showBrowserNotification_should_clearMapEntry_When_timerFires`
```
Action:  call showBrowserNotification("X", { tag: "t4", autoCloseMs: 15_000 })
         jest.advanceTimersByTime(15_000)
         call showBrowserNotification("Y", { tag: "t4" })
Assert:  notifInstances[0].close called once (from timer)
         notifInstances[1].close NOT called by the second showBrowserNotification
         (map entry was already cleared by timer, so no double-close)
```

#### `showBrowserNotification_should_useActionableTTL_When_requireInteractionTrue`
```
Action:  call showBrowserNotification("X", { tag: "t5", requireInteraction: true })
         jest.advanceTimersByTime(NATIVE_ACTIONABLE_TTL_MS - 1)
Assert:  notifInstances[0].close NOT called
         jest.advanceTimersByTime(1) → close called
```

---

### Jest Unit Tests — `web-app/src/lib/notification-policy.test.ts`

#### `nativeAutoCloseMs_should_return30000_When_priorityIsUrgent`
```
Assert:  nativeAutoCloseMs(NotificationPriority.URGENT) === 30_000
```

#### `nativeAutoCloseMs_should_return30000_When_priorityIsHigh`
```
Assert:  nativeAutoCloseMs(NotificationPriority.HIGH) === 30_000
```

#### `nativeAutoCloseMs_should_return15000_When_priorityIsMedium`
```
Assert:  nativeAutoCloseMs(NotificationPriority.MEDIUM) === 15_000
```

#### `nativeAutoCloseMs_should_return15000_When_priorityIsUnspecified`
```
Assert:  nativeAutoCloseMs(NotificationPriority.UNSPECIFIED) === 15_000
```

---

### Jest Unit Tests — `web-app/src/lib/hooks/useSessionNotifications.test.ts`

Mock `showBrowserNotification` as `jest.fn()` via `jest.mock("@/lib/utils/notifications")`. Mock `addNotification` from the store. Use `renderHook` from `@testing-library/react`.

#### `handleNotification_should_callShowBrowserNotification_When_nonHistoryEvent`
```
Setup:   Notification.permission = "granted"
Action:  Fire a FORK_PRESSURE event (non-history type) through handleNotification
Assert:  showBrowserNotification mock called once
         addNotification mock called once
         Both calls happen (toast + native in sync, FR-6)
```

#### `handleNotification_should_notCallShowBrowserNotification_When_historyOnlyEvent`
```
Action:  Fire a HISTORY_ONLY type event
Assert:  showBrowserNotification NOT called
         addNotification NOT called
```

#### `handleNotification_should_useApprovalTag_When_approvalIdPresent`
```
Action:  Fire APPROVAL_NEEDED event with metadata.approval_id = "abc123"
Assert:  showBrowserNotification called with tag "approval:abc123"
         (not the generic sessionId:type tag)
```

#### `handleNotification_should_neverSuppressApproval_When_dedupWindowActive`
```
Setup:   Fire first APPROVAL_NEEDED event → handled
Action:  Fire second APPROVAL_NEEDED event with same sessionId immediately after
Assert:  showBrowserNotification called twice (approval events bypass dedup)
```

---

### Integration / E2E Stubs (Playwright)

These are stub specifications for `tests/e2e/smart-notification-dedup.spec.ts`. Full implementation is deferred to Story 3 of a follow-up phase; the stubs define the test contract.

#### `e2e_ForkPressure_NoRepeatNotification_WhenStableCount`
```
// @feature session:fork-pressure-dedup
Scenario:  Simulate stable zombie count in test server.
           Wait 3× the fork alert cooldown.
Assert:    In-app notification badge count increments by exactly 1 (not 3).
           No duplicate toast appears in the DOM.
Playwright locators: [data-testid="notification-count"], [data-testid="toast-container"]
```

#### `e2e_ForkPressure_NewNotification_WhenCountIncreases`
```
Scenario:  Stable zombie count for 1 cooldown window → increase count by 5.
Assert:    A second toast appears within the next monitoring tick.
Playwright: Wait for second [data-testid="toast"] to appear after count injection.
```

#### `e2e_NativeNotification_AutoDismiss_WhenTTLExpires`
```
Scenario:  Grant notification permission in browser context.
           Trigger a FORK_PRESSURE notification.
Assert:    window.Notification constructor called.
           After 30 s (fake timer or accelerated clock), notification.close() was called.
Note:      Playwright cannot observe OS Notification Center; verify via spy on
           Notification.prototype.close injected via page.addInitScript.
```

#### `e2e_NativeNotification_CloseBeforeOpen_WhenSameTagFires`
```
Scenario:  Trigger two fork-pressure notifications within 5 s (count increase).
Assert:    First Notification object has .close() called before the second is constructed.
Playwright: Spy injected via page.addInitScript tracking Notification instances.
```

#### `e2e_ApprovalNotification_NeverSuppressed`
```
Scenario:  Fire two consecutive APPROVAL_NEEDED events for the same session.
Assert:    Both appear as distinct toasts; neither is suppressed.
           Both produce distinct native notifications (different approval tags).
```

---

## Test Stack

- **Go Unit Tests**: `go test -race` with package-internal access (`package tmux`); no external test framework beyond stdlib `testing`. Table-driven tests preferred.
- **Jest Unit Tests**: Jest 29 + `@testing-library/react` (hooks); `jest.useFakeTimers()` for timer control; `Object.defineProperty` for `window.Notification` mock.
- **Integration / E2E**: Playwright + the existing `tests/e2e/` harness; test server at `http://localhost:8544`; Allure for reporting.

---

## Coverage Targets

- **Go unit**: ≥80% line coverage on `session/tmux/fork_metrics.go` (new gating block)
- **TypeScript unit**: ≥80% branch coverage on `showBrowserNotification` (all tag / TTL / map paths)
- **All public service methods**: happy path + at least one error/suppression path
- **All external integrations**: `window.Notification` API mocked in unit tests; at least one E2E stub verifying the real browser path

---

## Coverage Map: FR → Tests

| Requirement | Test count | Test names (summary) |
|---|---|---|
| FR-1: Condition-change gating | 3 Go unit | `StableCount_NoRepeatAlert`, `StableCount_SuppressedForEntireCooldown`, `LevelEscalation_BypassesCooldown` |
| FR-2: Count baseline after notification | 2 Go unit | `BaselineRecorded_AfterFiring`, `BaselineNotUpdated_WhenSuppressed` |
| FR-3: Native auto-dismiss | 8 mixed unit | `close{High,Medium,Actionable}_When_*Elapsed`, `clearMapEntry_When_timerFires`, `useActionableTTL_When_requireInteraction`, `nativeAutoCloseMs` × 4 |
| FR-4: Close-before-open dedup | 3 Jest unit | `closeFirst_When_sameTag`, `notCloseOther_When_differentTag`, `dedupUntagged` |
| FR-5: Re-arm on clear | 3 Go unit | `ClearAndRearm`, `BaselineResetOnClear`, `NoAlertOnClear` |
| FR-6: Consistent toast + native | 4 Jest unit (hook) | `callShowBrowserNotification_nonHistory`, `notCall_historyOnly`, `useApprovalTag`, `neverSuppressApproval` |
| NFR-1: Approvals never suppressed | 2 (1 Go + 1 Jest) | `ApprovalsUnaffected`, `neverSuppressApproval_When_dedupWindowActive` |
| NFR-2: No new persistence layer | Architectural — verified by code review; no test needed |
| NFR-3: Backward compatible | `cd web-app && npx tsc --noEmit` + `make build` as gate |

**Total**: 6/6 functional requirements fully covered by named tests. NFR-2 verified by review; NFR-3 by build gate.
