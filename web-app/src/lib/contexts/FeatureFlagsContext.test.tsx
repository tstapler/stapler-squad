import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { FlagMutation } from "@/gen/session/v1/session_pb";
import { FeatureFlagsProvider, readbackMismatch, useFeatureFlags } from "./FeatureFlagsContext";

const mockGetFeatureFlags = jest.fn();
const mockUpdateFeatureFlag = jest.fn();

jest.mock("next/navigation", () => ({ useRouter: () => ({ replace: jest.fn() }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getFeatureFlags: (...a: unknown[]) => mockGetFeatureFlags(...a),
    updateFeatureFlag: (...a: unknown[]) => mockUpdateFeatureFlag(...a),
  }),
}));

const GATE = "hidden_session_gate";

function flag(enabled: boolean, scopes: Array<{ scope: string; enabled: boolean }> = []) {
  return { name: GATE, enabled, description: "d", statusDetail: "", scopes };
}

async function setup(initial = flag(false)) {
  mockGetFeatureFlags.mockResolvedValue({ flags: [initial] });
  const hook = renderHook(() => useFeatureFlags(), {
    wrapper: ({ children }) => <FeatureFlagsProvider>{children}</FeatureFlagsProvider>,
  });
  await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
  return hook;
}

beforeEach(() => {
  mockGetFeatureFlags.mockReset();
  mockUpdateFeatureFlag.mockReset();
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => jest.restoreAllMocks());

describe("FeatureFlagsContext scoped changes", () => {
  it("feature_flags_context_should_send_scope_with_mutation_CLEAR_SCOPE_when_inherit_chosen_SET_ENABLED_or_SET_DISABLED_with_scope_global_or_kind_and_RESET_GLOBAL_with_empty_scope_for_reset", async () => {
    const { result } = await setup();
    const responses = [
      flag(true),
      flag(true, [{ scope: "kind:review", enabled: false }]),
      flag(true),
      flag(false),
    ];
    mockUpdateFeatureFlag.mockImplementation(async () => ({ flag: responses.shift() }));

    await act(() => result.current.setFlag(GATE, { mutation: "set", scope: "global", enabled: true }));
    await act(() => result.current.setFlag(GATE, { mutation: "set", scope: "kind:review", enabled: false }));
    await act(() => result.current.setFlag(GATE, { mutation: "clear", scope: "kind:review" }));
    await act(() => result.current.setFlag(GATE, { mutation: "reset" }));

    expect(mockUpdateFeatureFlag.mock.calls.map((c) => c[0])).toEqual([
      { name: GATE, scope: "global", mutation: FlagMutation.SET_ENABLED },
      { name: GATE, scope: "kind:review", mutation: FlagMutation.SET_DISABLED },
      { name: GATE, scope: "kind:review", mutation: FlagMutation.CLEAR_SCOPE },
      { name: GATE, mutation: FlagMutation.RESET_GLOBAL },
    ]);
    expect(result.current.error).toBeNull();
    expect(result.current.flagList[0].enabled).toBe(false);
  });

  it("feature_flags_context_should_keep_legacy_boolean_requests_unscoped", async () => {
    const { result } = await setup();
    mockUpdateFeatureFlag.mockResolvedValue({ flag: flag(true) });
    await act(() => result.current.setFlag(GATE, true));
    expect(mockUpdateFeatureFlag).toHaveBeenCalledWith({ name: GATE, enabled: true });
  });

  it("feature_flags_context_should_show_read_back_scopes_from_the_response", async () => {
    const { result } = await setup();
    mockUpdateFeatureFlag.mockResolvedValue({ flag: flag(false, [{ scope: "kind:diagnose", enabled: true }]) });
    await act(() => result.current.setFlag(GATE, { mutation: "set", scope: "kind:diagnose", enabled: true }));
    expect(result.current.flagList[0].scopes).toEqual({ "kind:diagnose": true });
  });

  // T-FL-19
  it("feature_flags_context_should_treat_a_response_without_the_expected_scopes_entry_as_an_error_when_an_older_server_ignores_scope", async () => {
    const { result } = await setup();
    mockUpdateFeatureFlag.mockResolvedValue({ flag: flag(false, []) }); // success, scope dropped
    await act(() => result.current.setFlag(GATE, { mutation: "set", scope: "kind:review", enabled: true }));
    expect(result.current.error).toBe("Failed to update feature flag");
    expect(result.current.flagList[0].scopes).toEqual({});
  });

  it("feature_flags_context_should_reread_server_state_when_the_update_fails", async () => {
    const { result } = await setup(flag(false, [{ scope: "kind:review", enabled: true }]));
    mockUpdateFeatureFlag.mockRejectedValue(new Error("boom"));
    mockGetFeatureFlags.mockClear();
    await act(() => result.current.setFlag(GATE, { mutation: "set", scope: "kind:review", enabled: false }));
    await waitFor(() => expect(mockGetFeatureFlags).toHaveBeenCalledTimes(1));
    expect(result.current.flagList[0].scopes).toEqual({ "kind:review": true });
    expect(result.current.error).toBe("Failed to update feature flag");
  });
});

describe("readbackMismatch", () => {
  const f = (enabled: boolean, scopes: Array<{ scope: string; enabled: boolean }>) =>
    flag(enabled, scopes) as unknown as Parameters<typeof readbackMismatch>[1];

  it("readbackMismatch_should_CheckEachMutationAgainstTheServerState", () => {
    expect(readbackMismatch({ mutation: "reset" }, f(false, []))).toBeNull();
    expect(readbackMismatch({ mutation: "set", scope: "global", enabled: true }, f(true, []))).toBeNull();
    expect(readbackMismatch({ mutation: "set", scope: "global", enabled: true }, f(false, []))).toBe("global value");
    expect(readbackMismatch({ mutation: "clear", scope: "kind:review" }, f(false, []))).toBeNull();
    expect(
      readbackMismatch({ mutation: "clear", scope: "kind:review" }, f(false, [{ scope: "kind:review", enabled: true }])),
    ).toBe("kind:review");
    expect(
      readbackMismatch({ mutation: "set", scope: "kind:review", enabled: false }, f(false, [{ scope: "kind:review", enabled: true }])),
    ).toBe("kind:review");
  });
});
