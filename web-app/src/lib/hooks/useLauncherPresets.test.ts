import { renderHook, waitFor, act } from "@testing-library/react";
import { useLauncherPresets } from "./useLauncherPresets";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@/gen/session/v1/session_pb", () => ({ SessionService: {} }));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

const mockGetLauncherPresets = jest.fn();

(createClient as jest.Mock).mockReturnValue({ getLauncherPresets: mockGetLauncherPresets });

beforeEach(() => {
  jest.clearAllMocks();
  mockGetLauncherPresets.mockResolvedValue({
    presets: [
      { id: "codex-gpt5", label: "Codex GPT-5", argv: ["codex", "--model", "gpt-5"], program: "", defaultPath: "" },
      { id: "remote-claude", label: "Remote Claude", argv: ["ssh", "-t", "host"], program: "", defaultPath: "" },
    ],
    loadError: "",
  });
});

describe("useLauncherPresets", () => {
  describe("successful fetch", () => {
    it("useLauncherPresets_should_PopulatePresets_When_FetchSucceeds", async () => {
      const { result } = renderHook(() => useLauncherPresets());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.presets).toHaveLength(2);
      expect(result.current.presets[0].id).toBe("codex-gpt5");
    });

    it("useLauncherPresets_should_ReturnNullLoadErrorAndNullError_When_ResponseIsClean", async () => {
      const { result } = renderHook(() => useLauncherPresets());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.loadError).toBeNull();
      expect(result.current.error).toBeNull();
    });
  });

  describe("load_error distinct from transport error", () => {
    it("useLauncherPresets_should_SetLoadErrorNotError_When_ResponseHasLoadError", async () => {
      mockGetLauncherPresets.mockResolvedValue({
        presets: [],
        loadError: 'duplicate id "x"',
      });

      const { result } = renderHook(() => useLauncherPresets());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(result.current.loadError).toBe('duplicate id "x"');
      expect(result.current.error).toBeNull();
    });

    it("useLauncherPresets_should_SetErrorNotLoadError_When_RPCThrows", async () => {
      mockGetLauncherPresets.mockRejectedValue(new Error("network failure"));

      const { result } = renderHook(() => useLauncherPresets());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(result.current.error).toBeInstanceOf(Error);
      expect(result.current.error?.message).toBe("network failure");
      expect(result.current.loadError).toBeNull();
    });
  });

  describe("refetch", () => {
    it("useLauncherPresets_should_CallGetLauncherPresetsTwice_When_RefetchCalled", async () => {
      const { result } = renderHook(() => useLauncherPresets());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(mockGetLauncherPresets).toHaveBeenCalledTimes(1);

      act(() => {
        result.current.refetch();
      });

      await waitFor(() => expect(mockGetLauncherPresets).toHaveBeenCalledTimes(2));
    });
  });
});
