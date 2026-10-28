import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { FeatureFlagsProvider, useFeatureFlags } from "./FeatureFlagsContext";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";

const mockGetFeatureFlags = jest.fn();
const mockUpdateFeatureFlag = jest.fn();

jest.mock("@/lib/nav", () => ({ useRouter: () => ({ replace: jest.fn() }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getFeatureFlags: (...a: unknown[]) => mockGetFeatureFlags(...a),
    updateFeatureFlag: (...a: unknown[]) => mockUpdateFeatureFlag(...a),
  }),
}));

const flag = (enabled: boolean) => ({ name: "x", enabled, description: "", statusDetail: "", scopes: [] });

beforeEach(() => {
  resetFlagStoreForTest();
  mockGetFeatureFlags.mockReset();
  mockUpdateFeatureFlag.mockReset();
});

describe("FeatureFlagsProvider over flagStore", () => {
  it("provider_should_IssueOneGetFeatureFlags_When_ProviderAnd5HookConsumersMountTogether", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [flag(true)] });
    const hooks = renderHook(
      () => ({ ctx: useFeatureFlags(), v: [1, 2, 3, 4, 5].map(() => useFeatureFlagValue("x")) }),
      { wrapper: ({ children }) => <FeatureFlagsProvider>{children}</FeatureFlagsProvider> },
    );
    await waitFor(() => expect(hooks.result.current.ctx.isLoading).toBe(false));
    await waitFor(() => expect(hooks.result.current.v).toEqual([true, true, true, true, true]));
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(1);
  });

  it("provider_should_UpdateHookConsumersImmediately_When_SetFlagSucceeds", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [flag(false)] });
    const hooks = renderHook(() => ({ ctx: useFeatureFlags(), v: useFeatureFlagValue("x") }), {
      wrapper: ({ children }) => <FeatureFlagsProvider>{children}</FeatureFlagsProvider>,
    });
    await waitFor(() => expect(hooks.result.current.ctx.isLoading).toBe(false));
    expect(hooks.result.current.v).toBe(false);

    mockUpdateFeatureFlag.mockResolvedValue({ flag: flag(true) });
    mockGetFeatureFlags.mockResolvedValue({ flags: [flag(true)] });
    await act(async () => void (await hooks.result.current.ctx.setFlag("x", true)));
    expect(hooks.result.current.v).toBe(true);
  });
});
