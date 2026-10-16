import { renderHook, waitFor } from "@testing-library/react";

const mockGetSessionDefaults = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getSessionDefaults: (...args: unknown[]) => mockGetSessionDefaults(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

import { useStaleSessionConfig } from "@/lib/hooks/useStaleSessionConfig";

describe("useStaleSessionConfig", () => {
  beforeEach(() => {
    mockGetSessionDefaults.mockReset();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("useStaleSessionConfig_should_ReturnSafeDefault_When_FetchHasNotResolvedYet", () => {
    // Never resolves within this test — asserts the synchronous state right after render.
    mockGetSessionDefaults.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useStaleSessionConfig());

    expect(result.current).toEqual({ thresholdMinutes: 30, notifyEnabled: true });
  });

  it("useStaleSessionConfig_should_ReturnFetchedConfig_When_FetchResolves", async () => {
    mockGetSessionDefaults.mockResolvedValue({
      defaults: {
        staleSessionThresholdMinutes: 45,
        staleSessionNotifyEnabled: true,
      },
    });

    const { result } = renderHook(() => useStaleSessionConfig());

    await waitFor(() =>
      expect(result.current).toEqual({ thresholdMinutes: 45, notifyEnabled: true })
    );

    expect(mockGetSessionDefaults).toHaveBeenCalledTimes(1);
  });

  it("useStaleSessionConfig_should_KeepSafeDefault_When_FetchRejects", async () => {
    mockGetSessionDefaults.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useStaleSessionConfig());

    await waitFor(() => expect(mockGetSessionDefaults).toHaveBeenCalledTimes(1));

    expect(result.current).toEqual({ thresholdMinutes: 30, notifyEnabled: true });
  });
});
