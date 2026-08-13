# Pitfalls Research: Smart Notification Dedup

## Pitfall 1: Zombie PID Re-detection vs. New Zombies

**The problem**: `StartZombieWatcher` already deduplicates by PID (`reported map[int]bool`). A stable zombie (reaped but PID persists in ps output) is NOT re-reported to `RecordZombieProcess`. However, the ring buffer (`zombieRing`) is sliding-window based: entries from a 30s window count toward the threshold. If 10 new zombies appeared in the last 30s and no new ones appear, the ring entries age out after 30s — zombie count drops to 0 — and `checkPressure` returns OK. This is correct behavior.

**Actual risk**: If zombie reaping fails (the `reapZombieChildren()` call in `StartZombieWatcher` doesn't reap them), the same zombie PIDs persist across scan cycles. The `reported` map prevents re-recording them, so ring entries age out. After 30s, `ZombiesInWindow` drops to 0, level returns to OK — **but the zombies are still there**. The re-arm logic (FR-5) correctly fires a new alert only when a new zombie appears (new PID not in `reported`).

**Design implication**: The condition-change check must use the `ZombiesInWindow` count (not `TotalZombies`) to avoid suppressing legitimate new-zombie alerts that look like "same count" at the total level.

## Pitfall 2: Cooldown vs. Condition-Change Ordering

**The problem**: The new condition-change check and the existing 2-minute cooldown must interact correctly.

**Risk scenario**: 
- t=0: 10 zombies → alert fires (lastAlertZombieCount=10, lastAlertAt=t0)
- t=1m: 15 zombies → cooldown not expired (1m < 2m) → suppressed even though count worsened

**Design decision**: The condition-change check should SHORT-CIRCUIT the cooldown. If the situation worsened (more zombies OR higher level), fire immediately regardless of cooldown. The cooldown only applies when the situation is unchanged — it prevents repeated identical alerts.

**Proposed ordering in `checkPressure`**:
```
1. snapshot
2. if level == OK → maybe reset baseline → return
3. acquire alertMu
4. condition-change check:
   - if worsened: skip cooldown entirely, proceed to alert
   - if unchanged: apply cooldown check, suppress if within cooldown
5. update baseline + lastAlertAt
6. fire alertFns
```

This way: stable critical state → suppressed after first alert (not re-firing every 2 min); newly-worse state → fires immediately.

## Pitfall 3: Native Notification `close()` vs OS Behavior

**The problem**: `Notification.close()` is not universally honored:
- **Chrome desktop**: `close()` works reliably
- **Firefox**: `close()` works
- **macOS Notification Center**: notifications that have been moved to the NC cannot be closed programmatically — `close()` removes the banner but the NC entry persists
- **iOS Safari**: Notification API not supported at all (returns at permission check)
- **Android Chrome**: `close()` works

**Design implication**: `notification.close()` satisfies FR-3 ("Explicit call to `notification.close()` must happen") per the spec even if the OS NC entry persists. Document this limitation; do not attempt workarounds (service worker approaches are out of scope per requirements).

**Additional risk**: If the `setTimeout` fires but the `Notification` object has been GC'd (shouldn't happen since we hold a reference in the Map), `close()` would throw. Wrap in try/catch.

## Pitfall 4: Approval Notifications Accidentally Suppressed

**The problem**: The new backend condition-change gate operates at the `checkPressure` level — it only affects fork pressure alerts. Approval notifications go through a completely different path (review queue polling → `useReviewQueueNotifications` → `showSessionNotification`). No backend-level gating applies to them.

**Risk**: The frontend `TOAST_DEDUP_WINDOW_MS` check in `useSessionNotifications` has an `isApproval` guard. But if a non-approval notification for the same `sessionId` was recently shown, and an approval arrives within 10s, the dedup key is `${sessionId}:${notificationType}` — different type, different key. No suppression. This is correct.

**Risk with native notifications**: If we add `showBrowserNotification` to `handleNotification` and key the tag as `system-${sessionId}`, an approval notification arriving for the same session would close a previous fork-pressure native notification and open the approval one. This is acceptable and actually desired (FR-4: new notification for same (sessionId, type) closes old). But the tag must include `notificationType` to avoid approval notifications being replaced by non-approval ones for the same session.

**Recommended tag format**: `${sessionId}:${notificationType}` — matches the existing `dedupKey` convention in `useSessionNotifications`.

## Pitfall 5: Multi-Tab Native Notification Duplication

**The problem**: Each browser tab runs its own JavaScript context. The `activeNativeNotifications` Map in `notifications.ts` is per-tab. If two tabs are open, both will create native notifications on the same event (the SSE stream is connected in both).

**Current state**: This problem already exists (fire-and-forget approach). The `tag:` option provides OS-level dedup: Chrome replaces a notification with the same tag regardless of which tab created it.

**Design implication**: Continue to rely on `tag:` for cross-tab dedup. The per-tab Map handles `close()` for the notification created in that tab; the OS handles the cross-tab case. No architectural change needed, but this must be documented as a known limitation.

## Pitfall 6: Re-arm on Clear Race Condition

**The problem**: The re-arm logic resets the baseline when `level == OK`. But the OK check happens in `checkPressure` which is called from `RecordZombieProcess`. If the zombie watcher's 30s scan interval runs at t=0 (10 zombies), and by t=30s those zombies are reaped (ring entries expire), the next `checkPressure` call is triggered only when a new event is recorded. If no new events occur, the baseline is never reset.

**Implication**: The baseline reset must also happen in `ForkPressureSnapshot()` or in a periodic check. However, the requirements say the re-arm fires when "zombie count drops to zero" — the ring buffer already handles this naturally (ring entries expire after 30s, next snapshot shows count=0). The reset must happen eagerly.

**Proposed fix**: Add a periodic check goroutine (using the existing `StartForkPressureLogger` timer or a new one) that calls a `maybeReset()` function when `snapshotAt(now).Level == OK && lastAlertLevel != OK`.

Alternatively: the simplest fix is to reset the baseline inside `checkPressure`'s early-return branch for `level == OK`. This is called on every new fork event. But if the system is healthy and no events occur, `checkPressure` is never called — the baseline stays stale. In practice this is fine: if there are no events, there's nothing to re-alert on. The re-arm is only needed when activity resumes.

## Pitfall 7: `ZombiesInWindow` Count Direction

**The problem**: `ZombiesInWindow` from the snapshot is the count of ring buffer entries in the last 30s. If zombie alerts fire every ~30s (one scan cycle), the ring contains exactly N entries from the last scan. But ring buffer capacity is 64:

```go
zombieRing: newTimestampRing(64)
```

If 64+ zombies appear, the ring wraps and oldest entries are overwritten. `countSince` still counts correctly (by timestamp), so this is fine. But the count seen by the condition-change check may oscillate if the watcher detects new PIDs (ring grows) and reaped PIDs age out (ring count drops). Compare by "worsened" (strictly higher than baseline), not equality, to avoid false suppression.

## Pitfall 8: `lastAlertZombieCount` Shadowing by Failures

**The problem**: The `ForkPressureCritical` level is reached by EITHER `failures >= 10` OR `zombies >= 10`. If alerts are driven only by failures (no zombies), `lastAlertZombieCount` remains 0. A later surge of zombies is correctly detected as worsened. But if failures drop below threshold while zombies remain, the level stays critical due to zombies, yet `lastAlertFailureCount` may be higher than current failures — triggering a false "worsened" detection.

**Fix**: Track baselines separately per metric and use per-metric "worsened" logic:
```
worsened = (zombies > lastAlertZombieCount) 
        || (failures > lastAlertFailureCount) 
        || (level > lastAlertLevel)
```
This correctly handles the mixed case.
