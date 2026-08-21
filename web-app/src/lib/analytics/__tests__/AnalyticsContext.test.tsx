import React from "react";
import { renderHook, act } from "@testing-library/react";
import { useAnalytics, AnalyticsContextProvider } from "@/lib/contexts/AnalyticsContext";
import type { AnalyticsProvider, AnalyticsEvent } from "@/lib/analytics/types";

function makeProvider(overrides: Partial<AnalyticsProvider> = {}): AnalyticsProvider {
  return {
    metadata: { name: "MockProvider" },
    track: jest.fn(),
    initialize: jest.fn().mockResolvedValue(undefined),
    onClose: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("useAnalytics", () => {
  it("useAnalytics_throws_when_outside_provider", () => {
    const { result } = renderHook(() => {
      try {
        return useAnalytics();
      } catch (e) {
        return e as Error;
      }
    });
    expect(result.current).toBeInstanceOf(Error);
    expect((result.current as Error).message).toMatch(/AnalyticsContextProvider/);
  });

  it("useAnalytics_returns_provider_track", () => {
    const trackMock = jest.fn();
    const provider = makeProvider({ track: trackMock });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AnalyticsContextProvider provider={provider}>
        {children}
      </AnalyticsContextProvider>
    );

    const { result } = renderHook(() => useAnalytics(), { wrapper });

    const event: AnalyticsEvent = { name: "test", category: "user_action" };
    act(() => {
      result.current.track(event);
    });

    expect(trackMock).toHaveBeenCalledWith(event);
  });
});

describe("AnalyticsContextProvider", () => {
  it("provider_initialize_called_on_mount", async () => {
    const provider = makeProvider();

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AnalyticsContextProvider provider={provider}>
        {children}
      </AnalyticsContextProvider>
    );

    renderHook(() => useAnalytics(), { wrapper });

    // initialize is called in a useEffect; wait for it to settle
    await act(async () => {
      await Promise.resolve();
    });

    expect(provider.initialize).toHaveBeenCalledTimes(1);
  });

  it("provider_onClose_called_on_unmount", async () => {
    const provider = makeProvider();

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AnalyticsContextProvider provider={provider}>
        {children}
      </AnalyticsContextProvider>
    );

    const { unmount } = renderHook(() => useAnalytics(), { wrapper });

    await act(async () => {
      unmount();
      await Promise.resolve();
    });

    expect(provider.onClose).toHaveBeenCalledTimes(1);
  });
});
