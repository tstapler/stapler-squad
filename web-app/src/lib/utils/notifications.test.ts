/**
 * Tests for showBrowserNotification — native notification lifecycle (FR-3, FR-4).
 *
 * Covers:
 *  FR-4: close-before-open dedup (same tag closes previous notification)
 *  FR-3: auto-close via setTimeout for high/medium/actionable TTLs
 *  FR-3: Map cleanup after timer fires and after onclose callback
 *  NFR-1: requireInteraction → NATIVE_ACTIONABLE_TTL_MS (not suppressed)
 */

// Mock @/lib/notification-policy BEFORE any imports that transitively load it.
// We use real constant values so the TTL assertions are meaningful.
jest.mock("@/lib/notification-policy", () => ({
  NATIVE_MEDIUM_TTL_MS: 15_000,
  NATIVE_ACTIONABLE_TTL_MS: 300_000,
  NATIVE_HIGH_TTL_MS: 30_000,
  nativeAutoCloseMs: jest.fn(),
}));

// ── Notification mock setup ──────────────────────────────────────────────────
// We use a factory so each test gets its own fresh instance array.

interface MockNotifInstance {
  close: jest.Mock;
  onclose: (() => void) | null;
}

let notifInstances: MockNotifInstance[];
let MockNotification: jest.Mock;

function setupNotificationMock() {
  notifInstances = [];
  MockNotification = jest.fn().mockImplementation((_title: string, _opts?: NotificationOptions) => {
    const inst: MockNotifInstance = { close: jest.fn(), onclose: null };
    notifInstances.push(inst);
    return inst;
  });
  Object.defineProperty(window, "Notification", {
    value: Object.assign(MockNotification, { permission: "granted" }),
    writable: true,
    configurable: true,
  });
}

// ── Import under test ────────────────────────────────────────────────────────
// Use jest.isolateModules inside each test to get a fresh module-level Map.
// This is necessary because activeNativeNotifications is module-level state.

async function importShowBrowserNotification() {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const mod = await import("@/lib/utils/notifications");
  return mod.showBrowserNotification;
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe("showBrowserNotification", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.resetModules();
    setupNotificationMock();
    // Ensure localStorage doesn't suppress notifications
    localStorage.removeItem("notifications-enabled");
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  // ── FR-4: Close-before-open dedup ─────────────────────────────────────────

  it("showBrowserNotification_should_closeFirst_When_sameTagCalledTwice", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("A", { tag: "sess:fork" });
    expect(notifInstances).toHaveLength(1);

    await showBrowserNotification!("B", { tag: "sess:fork" });
    expect(notifInstances).toHaveLength(2);
    // First notification should have been closed before second was created
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  it("showBrowserNotification_should_notCloseOther_When_differentTagUsed", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("A", { tag: "sess:fork" });
    await showBrowserNotification!("B", { tag: "sess:tmux" });

    expect(notifInstances).toHaveLength(2);
    // Different tags — first notification should NOT be closed
    expect(notifInstances[0].close).not.toHaveBeenCalled();
  });

  it("showBrowserNotification_should_dedupUntagged_When_noTagProvided", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("A"); // no tag → __untagged__
    await showBrowserNotification!("B"); // no tag → same __untagged__ slot

    expect(notifInstances).toHaveLength(2);
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  // ── FR-3: Auto-close via setTimeout ───────────────────────────────────────

  it("showBrowserNotification_should_closeHighPriority_When_30sElapsed", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("X", { tag: "t1", autoCloseMs: 30_000 });
    expect(notifInstances).toHaveLength(1);

    jest.advanceTimersByTime(29_999);
    expect(notifInstances[0].close).not.toHaveBeenCalled();

    jest.advanceTimersByTime(1);
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  it("showBrowserNotification_should_closeMediumPriority_When_15sElapsed", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    // No autoCloseMs → defaults to NATIVE_MEDIUM_TTL_MS (15_000)
    await showBrowserNotification!("X", { tag: "t2" });

    jest.advanceTimersByTime(14_999);
    expect(notifInstances[0].close).not.toHaveBeenCalled();

    jest.advanceTimersByTime(1);
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  it("showBrowserNotification_should_closeActionable_When_5minElapsed", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    // requireInteraction: true → defaults to NATIVE_ACTIONABLE_TTL_MS (300_000)
    await showBrowserNotification!("X", { tag: "t3", requireInteraction: true });

    jest.advanceTimersByTime(299_999);
    expect(notifInstances[0].close).not.toHaveBeenCalled();

    jest.advanceTimersByTime(1);
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  it("showBrowserNotification_should_clearMapEntry_When_timerFires", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    // First notification with explicit autoCloseMs
    await showBrowserNotification!("X", { tag: "t4", autoCloseMs: 15_000 });
    expect(notifInstances).toHaveLength(1);

    // Fire the timer — Map entry should be removed
    jest.advanceTimersByTime(15_000);
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);

    // Second notification with same tag — since Map was cleared, should NOT close first again
    await showBrowserNotification!("Y", { tag: "t4" });
    expect(notifInstances).toHaveLength(2);
    // notifInstances[0].close should still be 1 (called once by timer, not by second show)
    expect(notifInstances[0].close).toHaveBeenCalledTimes(1);
  });

  // ── onclose callback cleans up the Map ───────────────────────────────────

  it("should_clearMapEntry_When_oscloseFires", async () => {
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("X", { tag: "t6" });
    expect(notifInstances).toHaveLength(1);

    // Simulate OS closing the notification via onclose.
    // Assert onclose is registered — if null, production code failed to wire it.
    expect(notifInstances[0].onclose).not.toBeNull();
    (notifInstances[0].onclose as () => void)();

    // Next call with the same tag should NOT close notifInstances[0] again
    // (the Map entry was already cleared by onclose)
    await showBrowserNotification!("Y", { tag: "t6" });
    expect(notifInstances).toHaveLength(2);
    // close on notifInstances[0] should not have been called (Map was empty)
    expect(notifInstances[0].close).not.toHaveBeenCalled();
  });

  // ── localStorage disabled ─────────────────────────────────────────────────

  it("should_notShowNotification_When_notificationsDisabled", async () => {
    localStorage.setItem("notifications-enabled", "false");
    let showBrowserNotification: Awaited<ReturnType<typeof importShowBrowserNotification>>;
    await jest.isolateModulesAsync(async () => {
      showBrowserNotification = await importShowBrowserNotification();
    });

    await showBrowserNotification!("X", { tag: "disabled" });
    expect(notifInstances).toHaveLength(0);
  });
});
