/**
 * Regression tests for usePushNotifications.
 *
 * Key regression: push API fetch URLs must not double the /api prefix.
 * getApiBaseUrl() returns origin+'/api', so fetch paths must be /push/...
 * not /api/push/... (which would produce origin/api/api/push/...).
 */

import { renderHook, act } from "@testing-library/react";
import { usePushNotifications } from "../usePushNotifications";

describe("usePushNotifications – fetch URL construction", () => {
  let mockFetch: jest.Mock;

  beforeEach(() => {
    mockFetch = jest.fn().mockResolvedValue({
      text: () => Promise.resolve("validVapidKey_abcdefghijklmnopqrstuvwxyz01234567890"),
      json: () => Promise.resolve({ subscriptionId: "sub-123" }),
      ok: true,
    } as Response);
    global.fetch = mockFetch;

    // jsdom doesn't provide these Push/Notification APIs — stub them
    (window as unknown as Record<string, unknown>).PushManager = {};
    Object.defineProperty(window, "Notification", {
      writable: true,
      configurable: true,
      value: {
        permission: "granted",
        requestPermission: jest.fn().mockResolvedValue("granted"),
      },
    });
    Object.defineProperty(navigator, "permissions", {
      writable: true,
      configurable: true,
      value: {
        query: jest.fn().mockResolvedValue({
          addEventListener: jest.fn(),
          removeEventListener: jest.fn(),
        }),
      },
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
    delete (window as unknown as Record<string, unknown>).PushManager;
    delete (global as Record<string, unknown>).fetch;
  });

  function makeRegistration() {
    const mockSubscription = {
      endpoint: "https://push.example.com/sub1",
      toJSON: () => ({
        endpoint: "https://push.example.com/sub1",
        keys: { p256dh: "key1", auth: "auth1" },
      }),
      unsubscribe: jest.fn().mockResolvedValue(true),
    };

    const pushManager = {
      getSubscription: jest.fn().mockResolvedValue(null),
      subscribe: jest.fn().mockResolvedValue(mockSubscription),
    };

    const registration = {
      pushManager,
      addEventListener: jest.fn(),
      installing: null,
    };

    return { registration, pushManager, mockSubscription };
  }

  it("subscribe uses /push/vapid-key and /push/subscribe (no double /api/api/)", async () => {
    const { registration, mockSubscription } = makeRegistration();

    Object.defineProperty(navigator, "serviceWorker", {
      writable: true,
      configurable: true,
      value: {
        register: jest.fn().mockResolvedValue(registration),
        ready: Promise.resolve(registration),
        controller: {},
      },
    });

    registration.pushManager.getSubscription.mockResolvedValue(null);
    registration.pushManager.subscribe.mockResolvedValue(mockSubscription);

    const { result } = renderHook(() => usePushNotifications());

    // Wait for the initial effect (setupServiceWorker)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    mockFetch.mockClear();

    await act(async () => {
      await result.current.subscribe("https://host:8543/api");
    });

    const calledUrls = mockFetch.mock.calls.map(([url]) => url as string);

    expect(calledUrls.length).toBeGreaterThan(0);
    expect(calledUrls.some((u) => u.includes("/api/api/"))).toBe(false);
    expect(calledUrls.some((u) => u.endsWith("/push/vapid-key"))).toBe(true);
    expect(calledUrls.some((u) => u.endsWith("/push/subscribe"))).toBe(true);
  });

  it("unsubscribe uses /push/unsubscribe (no double /api/api/)", async () => {
    const { registration, mockSubscription } = makeRegistration();

    registration.pushManager.getSubscription.mockResolvedValue(mockSubscription);

    Object.defineProperty(navigator, "serviceWorker", {
      writable: true,
      configurable: true,
      value: {
        register: jest.fn().mockResolvedValue(registration),
        ready: Promise.resolve(registration),
        controller: {},
      },
    });

    const { result } = renderHook(() => usePushNotifications());

    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    mockFetch.mockClear();

    await act(async () => {
      await result.current.unsubscribe("https://host:8543/api");
    });

    const calledUrls = mockFetch.mock.calls.map(([url]) => url as string);

    expect(calledUrls.length).toBeGreaterThan(0);
    expect(calledUrls.some((u) => u.includes("/api/api/"))).toBe(false);
    expect(calledUrls.some((u) => u.endsWith("/push/unsubscribe"))).toBe(true);
  });
});
