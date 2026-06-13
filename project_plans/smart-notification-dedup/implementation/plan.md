# Implementation Plan: smart-notification-dedup

**Feature**: Suppress repeated health alerts when conditions are unchanged; auto-close and deduplicate native browser notifications.
**Date**: 2026-05-29
**Status**: Ready for implementation
**ADRs**: None — all changes use established in-repo patterns; no new dependencies introduced.

---

## Dependency Visualization

```
Epic 1.1 (Go baseline fields)
  └─ Story 1.1.1 (add fields to forkMonitor)
       └─ Story 1.1.2 (modify checkPressure)
            └─ Story 1.1.3 (re-arm on clear)
                 └─ Story 1.1.4 (Go unit tests)

Epic 2.1 (TS policy constants)  ← independent, can start in parallel with Epic 1.1
  └─ Story 2.1.1 (add NATIVE_*_TTL_MS to notification-policy.ts)

Epic 2.2 (notifications.ts rewrite)  ← depends on Epic 2.1
  └─ Story 2.2.1 (module-level Map + auto-close + close-before-open)
       └─ Story 2.2.2 (wire fork-pressure path in useSessionNotifications)
            └─ Story 2.2.3 (Jest unit tests)
```

---

## Phase 1: Backend — Condition-Change Gating

### Epic 1.1: Extend `forkMonitor` with alert-baseline state
**Goal**: Record the metric snapshot at the time of each alert emission so that subsequent `checkPressure` calls can determine whether conditions have actually worsened.

#### Story 1.1.1: Add baseline fields to `forkMonitor`
**As a** backend service, **I want** the fork pressure monitor to remember what metrics were true at the last alert, **so that** it can compare current state before firing again.
**Acceptance Criteria**:
- `forkMonitor` struct has three new fields: `lastAlertZombieCount int64`, `lastAlertFailureCount int64`, `lastAlertLevel ForkPressureLevel`
- All three fields are read and written only under `alertMu`
- `go build ./...` passes after the change
**Files**:
- `session/tmux/fork_metrics.go`

##### Task 1.1.1a: Add three fields to the `forkMonitor` anonymous struct literal (~3 min)
- Open `session/tmux/fork_metrics.go`
- Locate the `var forkMonitor = struct { ... }{ ... }` block (lines 135–149)
- Add to the struct type declaration (after `lastAlertAt time.Time`):
  ```go
  lastAlertZombieCount  int64
  lastAlertFailureCount int64
  lastAlertLevel        ForkPressureLevel
  ```
- The initializer literal needs no changes (zero values are correct initial state)
- Files: `session/tmux/fork_metrics.go`

---

#### Story 1.1.2: Modify `checkPressure` to gate on condition change
**As a** backend service, **I want** `checkPressure` to fire alert callbacks only when zombie count or failure count has strictly increased OR pressure level escalated, **so that** stable-but-elevated conditions do not spam users every two minutes.
**Acceptance Criteria**:
- If `stats.Level == lastAlertLevel && stats.ZombiesInWindow <= lastAlertZombieCount && stats.FailuresInWindow <= lastAlertFailureCount`, the call returns without firing
- If conditions have worsened (any metric strictly higher or level escalated), the alert fires REGARDLESS of the 2-minute cooldown (cooldown only applies when conditions are unchanged)
- After firing, `lastAlertZombieCount`, `lastAlertFailureCount`, and `lastAlertLevel` are updated under `alertMu`
**Files**:
- `session/tmux/fork_metrics.go`

##### Task 1.1.2a: Rewrite the locking and gating block in `checkPressure` (~5 min)
- Open `session/tmux/fork_metrics.go`, locate `func checkPressure(now time.Time)` (line 249)
- Replace the function body with:

```go
func checkPressure(now time.Time) {
	stats := snapshotAt(now)
	if stats.Level == ForkPressureOK {
		// Re-arm handled in Story 1.1.3 / Task 1.1.3a — see below.
		return
	}

	forkMonitor.alertMu.Lock()
	// Condition-change check: worsened means strictly higher counts OR level escalated.
	// We use strict > (not >=) intentionally: if the ring-buffer count drops due to
	// entry expiry and then rises again, we only re-alert when it exceeds the baseline
	// set at the last alert — not just when it equals it. This prevents re-alerts on
	// oscillation around the threshold boundary.
	worsened := stats.Level > forkMonitor.lastAlertLevel ||
		stats.ZombiesInWindow > forkMonitor.lastAlertZombieCount ||
		stats.FailuresInWindow > forkMonitor.lastAlertFailureCount

	if !worsened {
		// Suppress if within the cooldown window (unchanged stable condition).
		if !forkMonitor.lastAlertAt.IsZero() && now.Sub(forkMonitor.lastAlertAt) < forkAlertCooldown {
			forkMonitor.alertMu.Unlock()
			return
		}
	}
	// Either worsened (bypass cooldown) or cooldown expired (allow unchanged re-alert).
	forkMonitor.lastAlertAt = now
	forkMonitor.lastAlertZombieCount = stats.ZombiesInWindow
	forkMonitor.lastAlertFailureCount = stats.FailuresInWindow
	forkMonitor.lastAlertLevel = stats.Level
	fns := forkMonitor.alertFns
	forkMonitor.alertMu.Unlock()

	go func() {
		for _, fn := range fns {
			fn(stats.Level, stats)
		}
	}()
}
```
- Files: `session/tmux/fork_metrics.go`

---

#### Story 1.1.3: Re-arm baseline when condition clears (FR-5)
**As a** backend service, **I want** the baseline to reset to zero when pressure returns to OK, **so that** a future re-occurrence triggers a fresh alert rather than being suppressed as "same count".
**Acceptance Criteria**:
- When `checkPressure` is called and `stats.Level == ForkPressureOK`, if `lastAlertLevel != ForkPressureOK`, reset `lastAlertZombieCount = 0`, `lastAlertFailureCount = 0`, `lastAlertLevel = ForkPressureOK` under `alertMu`
- The function still returns immediately without firing alert callbacks
- After a clear followed by a new zombie surge, the next `checkPressure` call fires
**Files**:
- `session/tmux/fork_metrics.go`

##### Task 1.1.3a: Add reset-on-clear branch in `checkPressure` (~2 min)
- Replace the early-return branch in the rewritten `checkPressure` (from Task 1.1.2a):
  ```go
  if stats.Level == ForkPressureOK {
      // Only pay the mutex cost when we actually need to reset the baseline.
      // Read lastAlertLevel under the lock to avoid data races.
      forkMonitor.alertMu.Lock()
      if forkMonitor.lastAlertLevel != ForkPressureOK {
          // Condition cleared — reset baseline so next re-occurrence fires a fresh alert (FR-5).
          forkMonitor.lastAlertZombieCount = 0
          forkMonitor.lastAlertFailureCount = 0
          forkMonitor.lastAlertLevel = ForkPressureOK
      }
      forkMonitor.alertMu.Unlock()
      return
  }
  ```
- Note: the mutex is acquired in the OK branch only when `lastAlertLevel != OK` (i.e., during a transition from elevated back to OK). Under steady healthy operation, `lastAlertLevel` is already `OK` so the lock is taken briefly but the reset block is skipped. This is acceptable cost; the hot path when no pressure has ever been seen takes the lock only once per event.
- Files: `session/tmux/fork_metrics.go`

---

#### Story 1.1.4: Go unit tests for condition-change gating
**As a** developer, **I want** unit tests that verify the new gating logic, **so that** regressions in the cooldown-bypass and re-arm paths are caught in CI.
**Acceptance Criteria**:
- Test: stable count → no second alert within cooldown (existing behavior preserved)
- Test: worsened count → alert fires immediately regardless of cooldown
- Test: level escalation (warning → critical) → alert fires immediately
- Test: count drops to zero then re-rises → fresh alert fires
- Test: `APPROVAL_NEEDED`-path is unaffected (goes through server.go callback, not `checkPressure` — existing behavior)
**Files**:
- `session/tmux/fork_metrics_test.go` (create if absent, or extend existing)

##### Task 1.1.4a: Write table-driven tests covering the four gating scenarios (~5 min)
- Check for existing `fork_metrics_test.go`:
  ```bash
  ls /home/tstapler/Programming/stapler-squad/session/tmux/fork_metrics*test*
  ```
- If absent, create `session/tmux/fork_metrics_test.go` with package `tmux`
- Run `gofmt -w session/tmux/fork_metrics_test.go` after writing
- Write four test functions (or one table-driven test):
  1. `TestCheckPressure_StableCount_NoRepeatAlert` — record 10 zombies, fire once, advance clock by 1 min (inside cooldown), record 10 more zombies (same ring count), verify alert count == 1
  2. `TestCheckPressure_WorsenedCount_BypassesCooldown` — record 10 zombies, fire at t=0; advance 30s (still inside cooldown); record 5 more zombies (count now 15 > 10); verify second alert fires
  3. `TestCheckPressure_LevelEscalation_BypassesCooldown` — spike spawns to warning level, alert fires; advance 30s; spike failures to critical level; verify second alert fires immediately
  4. `TestCheckPressure_ClearAndRearm` — 10 zombies → alert; ring expires (advance 35s, no new events); inject 5 new zombies → verify fresh alert fires (baseline was reset on clear)
- Use direct field manipulation on `forkMonitor` for setup (package-internal test)
- Files: `session/tmux/fork_metrics_test.go`

---

## Phase 2: Frontend — Native Notification Lifecycle

### Epic 2.1: Add native notification TTL constants to policy
**Goal**: Centralize auto-close timing for native (OS) notifications in the same file as toast timing, so all notification lifecycle rules are in one place.

#### Story 2.1.1: Add `NATIVE_*_TTL_MS` constants to `notification-policy.ts`
**As a** frontend module, **I want** named constants for native notification lifetimes, **so that** `showBrowserNotification` callers use policy-level values rather than magic numbers.
**Acceptance Criteria**:
- Three new exports: `NATIVE_HIGH_TTL_MS = 30_000`, `NATIVE_MEDIUM_TTL_MS = 15_000`, `NATIVE_ACTIONABLE_TTL_MS = 5 * 60 * 1000`
- `nativeAutoCloseMs(priority)` helper exported, accepting `NotificationPriority` (from generated proto types), returning the appropriate TTL
- Existing exports unchanged
**Files**:
- `web-app/src/lib/notification-policy.ts`

##### Task 2.1.1a: Add constants and helper to `notification-policy.ts` (~3 min)
- Open `web-app/src/lib/notification-policy.ts`
- Add after existing imports:
  ```typescript
  import { NotificationPriority } from "@/gen/session/v1/types_pb";
  ```
- Add after existing constants:
  ```typescript
  /** Auto-close delay for high/urgent native (OS) notifications (FR-3). */
  export const NATIVE_HIGH_TTL_MS = 30_000;
  /** Auto-close delay for medium/low native (OS) notifications (FR-3). */
  export const NATIVE_MEDIUM_TTL_MS = 15_000;
  /** Auto-close delay for actionable native (OS) notifications (FR-3). */
  export const NATIVE_ACTIONABLE_TTL_MS = 5 * 60 * 1000;

  /** Maps a NotificationPriority to the native notification auto-close TTL. */
  export function nativeAutoCloseMs(priority: NotificationPriority): number {
    switch (priority) {
      case NotificationPriority.URGENT:
      case NotificationPriority.HIGH:
        return NATIVE_HIGH_TTL_MS;
      case NotificationPriority.MEDIUM:
      case NotificationPriority.LOW:
      case NotificationPriority.UNSPECIFIED:
      default:
        return NATIVE_MEDIUM_TTL_MS;
    }
  }
  ```
- Files: `web-app/src/lib/notification-policy.ts`

---

### Epic 2.2: Rewrite `showBrowserNotification` for dedup and auto-close
**Goal**: Maintain a module-level registry of open `Notification` handles so that (a) each tag slot closes the previous notification before opening a new one, and (b) all notifications close automatically via `setTimeout`.

#### Story 2.2.1: Add handle map, close-before-open, and auto-close to `notifications.ts`
**As a** frontend module, **I want** `showBrowserNotification` to track open Notification handles by tag, close any previous handle for the same tag, and schedule auto-close, **so that** the OS notification tray does not accumulate stale notifications.
**Acceptance Criteria**:
- `activeNativeNotifications: Map<string, Notification>` declared at module level (outside the function)
- Before creating a new `Notification`, `activeNativeNotifications.get(tag)?.close()` is called
- After creation, `setTimeout(() => { try { n.close(); } catch {} if (activeNativeNotifications.get(tag) === n) { activeNativeNotifications.delete(tag); } }, autoCloseMs)` is scheduled
- `notif.onclose = () => { if (activeNativeNotifications.get(tag) === n) activeNativeNotifications.delete(tag); }` is set
- `showBrowserNotification` accepts an optional `autoCloseMs?: number` in `options` (extended type, not `NotificationOptions` directly)
- Default `autoCloseMs` is `NATIVE_MEDIUM_TTL_MS`
- All `close()` calls are wrapped in try/catch to guard against GC edge cases
- The function signature remains `async showBrowserNotification(title: string, options?: ...): Promise<void>` — no breaking API change
- `requireInteraction: true` notifications (Tier 1 review queue) use `NATIVE_ACTIONABLE_TTL_MS`
- A code comment in `notifications.ts` documents the macOS Notification Center limitation: `notification.close()` dismisses the banner but cannot remove NC entries that the user has already swiped into the NC. This is a known browser/OS limitation and no workaround is attempted.
**Files**:
- `web-app/src/lib/utils/notifications.ts`
- `web-app/src/lib/notification-policy.ts` (imports)

##### Task 2.2.1a: Extend options type and add module-level Map; verify existing callers compile (~4 min)
- Open `web-app/src/lib/utils/notifications.ts`
- Add import at top:
  ```typescript
  import { NATIVE_MEDIUM_TTL_MS, NATIVE_ACTIONABLE_TTL_MS } from "@/lib/notification-policy";
  ```
- Add a local extended options interface before the function:
  ```typescript
  interface BrowserNotificationOptions extends NotificationOptions {
    /** Override auto-close delay in ms. Defaults to NATIVE_MEDIUM_TTL_MS. */
    autoCloseMs?: number;
  }
  ```
- Add module-level Map before the function:
  ```typescript
  /** Tracks open native Notification handles by tag for dedup and auto-close (FR-3, FR-4). */
  const activeNativeNotifications = new Map<string, Notification>();
  ```
- Update function signature:
  ```typescript
  export async function showBrowserNotification(
    title: string,
    options?: BrowserNotificationOptions
  ): Promise<void>
  ```
- `BrowserNotificationOptions extends NotificationOptions` is a structural superset, so the two existing call sites in `useReviewQueueNotifications.ts` (passing `{ body, tag, requireInteraction }` and `{ body, tag, silent }`) remain valid TypeScript with no changes. Verify this compiles with `cd web-app && npx tsc --noEmit`.
- Files: `web-app/src/lib/utils/notifications.ts`

##### Task 2.2.1b: Replace fire-and-forget `new Notification(...)` with lifecycle-aware creation (~3 min)
- In `showBrowserNotification`, replace:
  ```typescript
  new Notification(title, {
    icon: "/favicon.ico",
    badge: "/favicon.ico",
    ...options,
  });
  ```
  with:
  ```typescript
  const tag = options?.tag ?? "__untagged__";
  // Close previous notification for this tag (FR-4)
  try { activeNativeNotifications.get(tag)?.close(); } catch {}

  const { autoCloseMs: _autoCloseMs, ...notifOptions } = options ?? {};
  const autoCloseMs = _autoCloseMs
    ?? (options?.requireInteraction ? NATIVE_ACTIONABLE_TTL_MS : NATIVE_MEDIUM_TTL_MS);

  const notif = new Notification(title, {
    icon: "/favicon.ico",
    badge: "/favicon.ico",
    ...notifOptions,
  });
  activeNativeNotifications.set(tag, notif);

  // Auto-close (FR-3)
  const timerId = setTimeout(() => {
    try { notif.close(); } catch {}
    if (activeNativeNotifications.get(tag) === notif) {
      activeNativeNotifications.delete(tag);
    }
  }, autoCloseMs);

  // Clean up map entry when the OS closes the notification
  notif.onclose = () => {
    clearTimeout(timerId);
    if (activeNativeNotifications.get(tag) === notif) {
      activeNativeNotifications.delete(tag);
    }
  };
  ```
- Files: `web-app/src/lib/utils/notifications.ts`

---

#### Story 2.2.2: Wire fork-pressure path through `showBrowserNotification` (FR-6)
**As a** user, **I want** fork pressure alerts to produce both an in-app toast AND a native browser notification with the same lifecycle rules, **so that** the app does not silently differ between surfaces.
**Acceptance Criteria**:
- `useSessionNotifications.handleNotification` calls `showBrowserNotification` after `addNotification()` for non-history-only, non-approval event types when `Notification.permission === "granted"`
- Tag format: `${event.sessionId}:${mapNotificationType(event.notificationType)}` (matches existing `dedupKey` convention)
- `autoCloseMs` is derived from `nativeAutoCloseMs(event.priority)` imported from `notification-policy.ts`
- Approval notifications (`isApproval === true`) use `NATIVE_ACTIONABLE_TTL_MS` — NOT suppressed by dedup (each approval gets its own tag via approval_id in metadata when available, falling back to the type-based tag)
- History-only types (`HISTORY_ONLY_TYPES` set) do NOT get a native notification
**Files**:
- `web-app/src/lib/hooks/useSessionNotifications.ts`
- `web-app/src/lib/notification-policy.ts` (imports)

##### Task 2.2.2a: Import `showBrowserNotification` and `nativeAutoCloseMs` in `useSessionNotifications.ts` (~2 min)
- Open `web-app/src/lib/hooks/useSessionNotifications.ts`
- Add imports:
  ```typescript
  import { showBrowserNotification } from "@/lib/utils/notifications";
  import { nativeAutoCloseMs, NATIVE_ACTIONABLE_TTL_MS } from "@/lib/notification-policy";
  ```
- Files: `web-app/src/lib/hooks/useSessionNotifications.ts`

##### Task 2.2.2b: Add `showBrowserNotification` call after `addNotification()` in `handleNotification` (~3 min)
- In `handleNotification`, after the `addNotification(notificationData)` call (line 286), add the native notification block.
- **Important**: the insertion point is specifically AFTER `addNotification` so that all existing guards (HISTORY_ONLY_TYPES early-return, dedup window check) already executed. Do NOT insert before any of those guards.
- The `isApproval` variable declared earlier in the function is in scope here:
  ```typescript
  // Native notification (FR-6): fire alongside toast for non-history types.
  // Each (sessionId, notificationType) gets its own tag so dedup is consistent.
  if (typeof window !== "undefined" && "Notification" in window && Notification.permission === "granted") {
    const nativeTag = isApproval && event.metadata?.["approval_id"]
      ? `approval:${event.metadata["approval_id"]}`
      : `${event.sessionId}:${mapNotificationType(event.notificationType)}`;
    void showBrowserNotification(event.title, {
      body: event.message ?? undefined,
      tag: nativeTag,
      autoCloseMs: isApproval ? NATIVE_ACTIONABLE_TTL_MS : nativeAutoCloseMs(event.priority),
      requireInteraction: isApproval,
    });
  }
  ```
- Files: `web-app/src/lib/hooks/useSessionNotifications.ts`

---

#### Story 2.2.3: Jest unit tests for native notification lifecycle
**As a** developer, **I want** Jest tests covering the native notification map, close-before-open, and auto-close behavior, **so that** regressions are caught without needing a browser.
**Acceptance Criteria**:
- Test: calling `showBrowserNotification` twice with the same tag calls `close()` on the first `Notification` before creating the second
- Test: after `autoCloseMs` elapses, `close()` is called on the notification and the Map entry is removed
- Test: `notification.onclose` callback removes the Map entry and cancels the timer
- Test: `requireInteraction: true` defaults to `NATIVE_ACTIONABLE_TTL_MS` not `NATIVE_MEDIUM_TTL_MS`
- Test: untagged calls use `"__untagged__"` slot and still close-before-open each other
- All tests mock `window.Notification` (constructor + `close` method)
**Files**:
- `web-app/src/lib/utils/notifications.test.ts` (create)

##### Task 2.2.3a: Write Jest tests for `showBrowserNotification` lifecycle (~5 min)
- Create `web-app/src/lib/utils/notifications.test.ts`
- Mock `window.Notification` at the top of the file. Use `beforeAll`/`beforeEach` to avoid property conflict with JSDOM's pre-existing `Notification` descriptor:
  ```typescript
  let mockClose: jest.Mock;
  let MockNotification: jest.Mock;

  beforeEach(() => {
    mockClose = jest.fn();
    MockNotification = jest.fn().mockImplementation(() => ({ close: mockClose, onclose: null }));
    // Use defineProperty with configurable:true so it can be redefined between tests
    Object.defineProperty(window, 'Notification', {
      value: Object.assign(MockNotification, { permission: 'granted' }),
      writable: true,
      configurable: true,
    });
  });
  ```
- Mock `localStorage.getItem` to return `null` (notifications enabled)
- Use `jest.useFakeTimers()` to control `setTimeout`
- Write the five test cases listed in the acceptance criteria
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="notifications.test"` to verify
- Files: `web-app/src/lib/utils/notifications.test.ts`

##### Task 2.2.3b: Write Jest tests for `nativeAutoCloseMs` helper (~2 min)
- In `web-app/src/lib/notification-policy.test.ts` (extend if it exists, else create)
- Add test: `nativeAutoCloseMs(NotificationPriority.URGENT)` returns `NATIVE_HIGH_TTL_MS`
- Add test: `nativeAutoCloseMs(NotificationPriority.MEDIUM)` returns `NATIVE_MEDIUM_TTL_MS`
- Add test: `nativeAutoCloseMs(NotificationPriority.UNSPECIFIED)` returns `NATIVE_MEDIUM_TTL_MS` (default)
- Files: `web-app/src/lib/notification-policy.test.ts`

---

## Phase 3: Validation

### Epic 3.1: Build and CI validation
**Goal**: Confirm the changes compile, pass linting, and pass the test suite before PR.

#### Story 3.1.1: Run full build + test + lint
**As a** developer, **I want** to run `make ci` and see it pass, **so that** the PR is in a mergeable state.
**Acceptance Criteria**:
- `make build` passes (Go compiles, protos regenerated if needed)
- `make test` passes (all Go unit tests including new `fork_metrics_test.go` cases)
- `make lint` passes (no new lint warnings)
- `cd web-app && npx jest --no-coverage` passes (all Jest tests including new notification lifecycle tests)
**Files**: N/A (validation step)

##### Task 3.1.1a: Run `make ci` and fix any failures (~5 min)
- Run: `make ci` (covers Go build, Go tests, Go lint)
- Note: Jest tests are NOT part of `make ci`. Run separately: `cd web-app && npx jest --no-coverage`
- Also run TypeScript check: `cd web-app && npx tsc --noEmit`
- If linting fails on the new Go code, run `gofmt -w session/tmux/fork_metrics.go` and `gofmt -w session/tmux/fork_metrics_test.go`
- If Jest fails, check mock setup for `window.Notification` (see Task 2.2.3a)
- Files: varies by failure

---

## Acceptance Criteria Traceability

| Requirement | Covered by |
|---|---|
| FR-1: Condition-change gating | Story 1.1.2 (checkPressure rewrite) |
| FR-2: Count baseline after notification | Story 1.1.1 (fields) + Story 1.1.2 (update on fire) |
| FR-3: Native notification auto-dismiss | Story 2.2.1 (setTimeout + close) |
| FR-4: Close-before-open dedup | Story 2.2.1 (Map + close on reuse) |
| FR-5: Re-arm on clear | Story 1.1.3 (reset in OK branch) |
| FR-6: Consistent toast + native behavior | Story 2.2.2 (wire fork-pressure path) |
| NFR-1: Approvals never suppressed | Story 2.2.2 (approval tag uses approval_id) + existing isApproval guard untouched |
| NFR-2: No new persistence layer | All Go state in existing forkMonitor struct fields |
| NFR-3: Backward compatible | No proto changes, no exported API changes |

---

## File Change Summary

| File | Change |
|---|---|
| `session/tmux/fork_metrics.go` | Add 3 fields to `forkMonitor`; rewrite `checkPressure` |
| `session/tmux/fork_metrics_test.go` | Create: 4 table-driven test cases |
| `web-app/src/lib/notification-policy.ts` | Add 3 TTL constants + `nativeAutoCloseMs()` helper |
| `web-app/src/lib/utils/notifications.ts` | Add module Map; extend options type; rewrite Notification creation block |
| `web-app/src/lib/hooks/useSessionNotifications.ts` | Import showBrowserNotification; add native notif call after addNotification |
| `web-app/src/lib/utils/notifications.test.ts` | Create: 5 Jest tests for lifecycle |
| `web-app/src/lib/notification-policy.test.ts` | Extend/create: 3 Jest tests for nativeAutoCloseMs |

**Total**: 7 files, all existing or clearly scoped new test files.
