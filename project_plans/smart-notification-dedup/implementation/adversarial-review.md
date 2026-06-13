# Adversarial Review: smart-notification-dedup

**Date**: 2026-05-29
**Verdict**: CONCERNS

## Blockers
_(none)_

## Concerns

- [ ] **`checkPressure` OK-branch acquires `alertMu` on every healthy tick** — The re-arm-on-clear code (Story 1.1.3) holds `alertMu` inside the OK early-return path. `checkPressure` is called by `recordSpawn`, `recordFailure`, and `RecordZombieProcess` on every process event. Under normal (non-elevated) operation this means the mutex is acquired and released on every spawn. The ring-buffer reads in `snapshotAt` already take their own per-ring locks, so this is a second lock acquire on the hot path. Recommendation: guard the OK-branch reset with a cheap atomic read first — check `forkMonitor.lastAlertLevel` (stored as an `int32` atomic or via a quick non-locking peek) before paying the mutex cost. Or simply check `lastAlertLevel` inside the existing lock already taken later. As written, the plan takes the lock in the OK branch and again in the worsened/cooldown branch — these are two separate lock acquisitions per call when level is non-OK. The plan body shows them sequentially, which is correct, but the OK branch lock should be verified in the implementation task steps.

- [ ] **`worsened` comparison uses `>` (strict greater), not `>=`** — The plan's condition-change logic:
  ```go
  worsened := stats.Level > forkMonitor.lastAlertLevel ||
      stats.ZombiesInWindow > forkMonitor.lastAlertZombieCount ||
      stats.FailuresInWindow > forkMonitor.lastAlertFailureCount
  ```
  uses strictly-greater-than. Per Pitfall 8 in the research, this is the intended semantics ("worsened = strictly higher than baseline"). However, a scenario where `ZombiesInWindow` oscillates due to ring-buffer expiry (Pitfall 7) could cause the baseline to be set at count 15, then ring entries age out to count 12, then new zombies bring it to 13 — not higher than 15, so suppressed. This is the intended behavior (count is not higher than peak), but the task description in the plan should explicitly note this so the implementer does not second-guess the `>` vs `>=`. Recommendation: add a comment in Task 1.1.2a explaining the intent.

- [ ] **`isApproval` variable not in scope at the `showBrowserNotification` call site** — Story 2.2.2 / Task 2.2.2b adds a `showBrowserNotification` call after `addNotification(notificationData)`. The `isApproval` variable is declared earlier in `handleNotification` (line 223 in current source) and is in scope at the insertion point. However, the HISTORY_ONLY_TYPES early-return (lines 232–247) exits before `addNotification` is reached, so history-only types correctly never reach the new native-notification code. The plan does not explicitly call this out, but the insertion point (after `addNotification`) naturally handles it. Recommendation: the task description should note that the call goes after `addNotification` specifically to inherit the existing guards (history-only return, dedup check) — confirm the insertion point is AFTER the `addNotification(notificationData)` call at line 286, not at some earlier point.

- [ ] **`useReviewQueueNotifications` callers pass `tag` with no `autoCloseMs`** — After the `notifications.ts` rewrite, `showBrowserNotification` in `useReviewQueueNotifications.ts` still passes `requireInteraction: true` for Tier 1 without passing `autoCloseMs`. The new code defaults to `NATIVE_ACTIONABLE_TTL_MS` when `requireInteraction` is true. This is the correct behavior and the plan documents it. However, the plan does not include a task to verify `useReviewQueueNotifications.ts` still works correctly after the signature change — it relies on the default fallback. Recommendation: add a sub-task under Story 2.2.1 to verify that the two existing `showBrowserNotification` call sites in `useReviewQueueNotifications.ts` compile and behave correctly after the options type change (the extended interface is a structural superset of `NotificationOptions`, so TypeScript should accept it, but this should be explicitly checked).

## Minors

- The plan does not mention `gofmt -w` for the new test file (`fork_metrics_test.go`) — the existing `make lint` step will catch this, but it should be in Task 1.1.4a.
- `window.Notification` mock in Task 2.2.3a uses `Object.defineProperty` which can conflict with JSDOM's pre-defined `Notification` property. Standard Jest pattern is to set it in a `beforeAll` block or use `jest.spyOn(window, 'Notification')`. The task should note this.
- The plan notes that `Notification.close()` does not dismiss macOS Notification Center entries (Pitfall 3). This is a known limitation. The plan correctly defers to documentation rather than attempting a workaround, but there is no task to add a comment in `notifications.ts` explaining this so future maintainers don't try to "fix" it.
- Story 3.1.1 runs `make ci` but the plan earlier notes frontend Jest tests are NOT part of `make ci`. The task should either run `make ci && cd web-app && npx jest --no-coverage` or note that the Jest step must be run separately.
