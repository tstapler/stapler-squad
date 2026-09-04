/**
 * Tests for useBrowserLogStream hook.
 *
 * Uses fake timers and mocked ConnectRPC client to verify:
 * - Console interception and restoration
 * - Buffer capping and immediate flush
 * - Timer-based flush
 * - Error handling (silent swallowing)
 * - Re-entrancy guard
 * - Unload beacon
 * - Session ID propagation
 */

import { renderHook, act } from "@testing-library/react";
import { useBrowserLogStream } from "../useBrowserLogStream";

// Mock the ConnectRPC dependencies so we don't need an actual server
const mockLogClientEvents = jest.fn().mockResolvedValue({});

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    logClientEvents: mockLogClientEvents,
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543/api"),
}));

describe("useBrowserLogStream", () => {
  const origLog = console.log;
  const origWarn = console.warn;
  const origError = console.error;
  const origDebug = console.debug;
  const origOnError = window.onerror;

  let mockSendBeacon: jest.Mock;

  beforeEach(() => {
    jest.useFakeTimers();
    mockLogClientEvents.mockClear();
    mockLogClientEvents.mockResolvedValue({});

    // Stub navigator.sendBeacon
    mockSendBeacon = jest.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "sendBeacon", {
      writable: true,
      configurable: true,
      value: mockSendBeacon,
    });
  });

  afterEach(() => {
    // Safety net: restore console methods in case hook cleanup didn't run
    console.log = origLog;
    console.warn = origWarn;
    console.error = origError;
    console.debug = origDebug;
    window.onerror = origOnError;
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  // UT-F-01
  it("disabled_should_still_patch_console_but_gate_only_debug_forwarding", async () => {
    const origLogRef = console.log;
    const origWarnRef = console.warn;
    const origErrorRef = console.error;
    const origDebugRef = console.debug;

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: false, sessionId: "sess-1" })
    );

    // Interceptors always install — log/warn/error forward unconditionally
    // (info-and-up, matching the server's own log-level default); only
    // console.debug is gated behind `enabled`.
    expect(console.log).not.toBe(origLogRef);
    expect(console.warn).not.toBe(origWarnRef);
    expect(console.error).not.toBe(origErrorRef);
    expect(console.debug).not.toBe(origDebugRef);

    console.log("log should forward while disabled");
    console.warn("warn should forward while disabled");
    console.debug("debug should not forward while disabled");
    console.error("error should forward while disabled");

    act(() => {
      jest.advanceTimersByTime(5000);
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entries = mockLogClientEvents.mock.calls[0][0].entries as Array<{ message: string }>;
    expect(entries.some((e) => e.message.includes("log should forward"))).toBe(true);
    expect(entries.some((e) => e.message.includes("warn should forward"))).toBe(true);
    expect(entries.some((e) => e.message.includes("debug should not forward"))).toBe(false);
    expect(entries.some((e) => e.message.includes("error should forward"))).toBe(true);

    unmount();
  });

  // UT-F-02
  it("enabled_should_patch_all_four_console_methods", () => {
    const origLogRef = console.log;
    const origWarnRef = console.warn;
    const origErrorRef = console.error;
    const origDebugRef = console.debug;

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    expect(console.log).not.toBe(origLogRef);
    expect(console.warn).not.toBe(origWarnRef);
    expect(console.error).not.toBe(origErrorRef);
    expect(console.debug).not.toBe(origDebugRef);

    unmount();
  });

  // UT-F-03
  it("enabled_should_call_through_to_originals", () => {
    const origLogMock = jest.fn();
    console.log = origLogMock;

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    console.log("test message");

    expect(origLogMock).toHaveBeenCalledWith("test message");

    unmount();
  });

  // UT-F-04
  it("enqueue_should_truncate_message_at_200_chars", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    const longMsg = "a".repeat(300);
    console.log(longMsg);

    // Trigger flush via timer
    act(() => {
      jest.advanceTimersByTime(5000);
    });

    // Wait for the async client call
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const callArg = mockLogClientEvents.mock.calls[0][0];
    const msg = callArg.entries[0].message as string;
    // 200 chars + ellipsis
    expect(msg.endsWith("…")).toBe(true);
    expect(msg.length).toBeLessThanOrEqual(201); // 200 chars + 1 ellipsis char

    unmount();
  });

  // UT-F-05
  it("enqueue_should_include_level_url_userAgent_timestamp", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-xyz" })
    );

    console.warn("check fields");

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entry = mockLogClientEvents.mock.calls[0][0].entries[0];
    expect(entry.level).toBe("warn");
    expect(entry.message).toContain("check fields");
    expect(entry.timestamp).toBeTruthy();
    expect(entry.url).toBeTruthy();
    expect(entry.userAgent).toBeTruthy();
    expect(entry.sessionId).toBe("sess-xyz");

    unmount();
  });

  // UT-F-06
  it("buffer_at_cap_should_trigger_immediate_flush", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // Push 50 entries to trigger immediate flush
    for (let i = 0; i < 50; i++) {
      console.log(`entry-${i}`);
    }

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entries = mockLogClientEvents.mock.calls[0][0].entries;
    expect(entries).toHaveLength(50);

    unmount();
  });

  // UT-F-07
  it("flush_timer_should_fire_after_5000ms", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    console.log("timer test");

    // Not flushed yet
    expect(mockLogClientEvents).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);

    unmount();
  });

  // UT-F-08
  it("overflow_beyond_cap_should_not_send_extra_calls_in_window", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // Push 60 entries — first 50 flush immediately; last 10 sit in buffer
    for (let i = 0; i < 60; i++) {
      console.log(`entry-${i}`);
    }

    await act(async () => {
      await Promise.resolve();
    });

    // Only 1 flush for the 50-entry cap; the remaining 10 are in buffer
    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);
    const firstCallEntries = mockLogClientEvents.mock.calls[0][0].entries;
    expect(firstCallEntries).toHaveLength(50);

    unmount();
  });

  // UT-F-09
  it("fetch_failure_should_be_silently_swallowed", async () => {
    mockLogClientEvents.mockRejectedValue(new Error("network error"));

    const consoleErrorSpy = jest.spyOn(console, "error");

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // Push enough to trigger immediate flush
    for (let i = 0; i < 50; i++) {
      console.log(`entry-${i}`);
    }

    await act(async () => {
      await Promise.resolve();
    });

    // Should not throw and should not call console.error from the flush itself
    // (The spy was installed after the hook patched console.error, so any recursive
    //  calls within the hook's enqueue would call the patched version)
    expect(consoleErrorSpy).not.toHaveBeenCalledWith(
      expect.stringContaining("network error")
    );

    unmount();
    consoleErrorSpy.mockRestore();
  });

  // UT-F-10
  it("toggle_off_should_restore_original_console_methods", () => {
    // Track invocations via a spy installed before the hook
    const logSpy = jest.fn();
    const origConsoleLog = console.log;
    console.log = logSpy;

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // After hook installs, console.log is patched (a different function)
    expect(console.log).not.toBe(logSpy);

    unmount();

    // After unmount, the hook restores the function it captured at install time.
    // That captured function was logSpy (bound), so console.log should be
    // callable and invoke logSpy.
    console.log("after-restore");
    // logSpy should have been called (through the restored reference)
    expect(logSpy).toHaveBeenCalledWith("after-restore");

    // Restore for afterEach safety
    console.log = origConsoleLog;
  });

  // UT-F-11
  it("window_onerror_should_be_intercepted", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    expect(window.onerror).not.toBe(origOnError);

    // Simulate a window error
    window.onerror?.("test error", "test.js", 10, 5, new Error("boom"));

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entries = mockLogClientEvents.mock.calls[0][0].entries;
    const errorEntry = entries.find(
      (e: { level: string }) => e.level === "error"
    );
    expect(errorEntry).toBeTruthy();

    unmount();
  });

  // UT-F-12
  it("unhandledrejection_should_be_intercepted", async () => {
    const addEventListenerSpy = jest.spyOn(window, "addEventListener");

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    expect(
      addEventListenerSpy.mock.calls.some(([type]) => type === "unhandledrejection")
    ).toBe(true);

    // jsdom does not support PromiseRejectionEvent constructor; dispatch a
    // plain Event and manually invoke the listener to simulate the rejection.
    // The hook's onUnhandled listener will be called by dispatchEvent since
    // jsdom supports the event type name lookup even without the constructor.
    // Simulate by finding the registered handler and calling it directly.
    const unhandledCall = addEventListenerSpy.mock.calls.find(
      ([type]) => type === "unhandledrejection"
    );
    expect(unhandledCall).toBeTruthy();
    const handler = unhandledCall![1] as EventListener;

    // Create a minimal mock PromiseRejectionEvent
    const fakeEvent = {
      type: "unhandledrejection",
      reason: new Error("unhandled rejection"),
    } as unknown as PromiseRejectionEvent;
    handler(fakeEvent as unknown as Event);

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entries = mockLogClientEvents.mock.calls[0][0].entries;
    const rejEntry = entries.find((e: { message: string }) =>
      e.message.includes("UnhandledRejection")
    );
    expect(rejEntry).toBeTruthy();

    unmount();
  });

  // UT-F-13
  it("reentrancy_guard_prevents_infinite_loop", async () => {
    // When the patched console.error is called inside flush (e.g. from a
    // rejected promise handler), it should not enqueue a new entry.
    let callCount = 0;
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // Simulate reentrancy by making the client call console.error recursively
    mockLogClientEvents.mockImplementation(() => {
      callCount++;
      if (callCount === 1) {
        // This should be caught by the reentrancy guard
        console.error("recursive call inside flush");
      }
      return Promise.resolve({});
    });

    // Push 50 to force flush
    for (let i = 0; i < 50; i++) {
      console.log(`entry-${i}`);
    }

    await act(async () => {
      await Promise.resolve();
    });

    // Only one flush, not an infinite loop
    expect(callCount).toBe(1);

    unmount();
  });

  // UT-F-14
  it("toggle_off_should_cancel_pending_flush_timer", () => {
    const clearTimeoutSpy = jest.spyOn(global, "clearTimeout");

    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    console.log("pending entry");

    unmount();

    // clearTimeout should have been called for the pending timer
    expect(clearTimeoutSpy).toHaveBeenCalled();
  });

  // UT-F-15
  it("toggle_off_should_clear_buffer", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    console.log("buffered entry");

    // Unmount before timer fires
    unmount();

    // Advance timer — flush should not fire since buffer was cleared
    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).not.toHaveBeenCalled();
  });

  // UT-F-16
  it("beforeunload_should_trigger_sendBeacon", () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    console.log("unload entry");

    // Trigger beforeunload
    window.dispatchEvent(new Event("beforeunload"));

    expect(mockSendBeacon).toHaveBeenCalled();

    unmount();
  });

  // UT-F-17
  it("session_id_included_in_posted_entry", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "test-session-id" })
    );

    console.log("session id test");

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const entry = mockLogClientEvents.mock.calls[0][0].entries[0];
    expect(entry.sessionId).toBe("test-session-id");

    unmount();
  });

  // UT-F-18
  it("circular_reference_arg_falls_back_to_string", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // Create a circular reference that JSON.stringify would throw on
    const circular: Record<string, unknown> = {};
    circular.self = circular;

    // Should not throw
    expect(() => {
      console.log(circular);
    }).not.toThrow();

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    // An entry was recorded (may be "[object Object]" or similar fallback)
    expect(mockLogClientEvents).toHaveBeenCalled();

    unmount();
  });

  // UT-F-19
  it("args_limited_to_5_per_entry", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: true, sessionId: "sess-1" })
    );

    // 10 distinct args — only first 5 should appear in message
    console.log("a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10");

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).toHaveBeenCalled();
    const msg = mockLogClientEvents.mock.calls[0][0].entries[0].message as string;
    expect(msg).toContain("a5");
    expect(msg).not.toContain("a6");

    unmount();
  });

  // UT-F-20
  it("disabled_should_not_forward_debug_level_calls", async () => {
    const { unmount } = renderHook(() =>
      useBrowserLogStream({ enabled: false, sessionId: "sess-1" })
    );

    console.debug("debug should not forward while disabled");

    act(() => {
      jest.advanceTimersByTime(10000);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(mockLogClientEvents).not.toHaveBeenCalled();

    unmount();
  });
});
