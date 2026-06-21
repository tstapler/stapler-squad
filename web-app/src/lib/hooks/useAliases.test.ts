import { renderHook, waitFor, act } from "@testing-library/react";
import { useAliases } from "./useAliases";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@/gen/session/v1/session_pb", () => ({ SessionService: {} }));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

const mockListAliases = jest.fn();

(createClient as jest.Mock).mockReturnValue({ listAliases: mockListAliases });

beforeEach(() => {
  jest.clearAllMocks();
  mockListAliases.mockResolvedValue({
    aliases: [
      {
        name: "myproj",
        group: "work",
        path: "~/code",
        description: "",
        profile: "",
        program: "claude",
        autoYes: false,
        tags: [],
      },
    ],
  });
});

describe("useAliases", () => {
  describe("return shape", () => {
    it("useAliases_should_expose_refetch_as_function", async () => {
      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(typeof result.current.refetch).toBe("function");
    });

    it("useAliases_should_return_aliases_array_after_load", async () => {
      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.aliases).toHaveLength(1);
      expect(result.current.aliases[0].name).toBe("myproj");
    });

    it("useAliases_should_return_null_error_on_success", async () => {
      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.error).toBeNull();
    });
  });

  describe("refetch", () => {
    it("useAliases_should_call_listAliases_twice_when_refetch_called", async () => {
      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(mockListAliases).toHaveBeenCalledTimes(1);

      act(() => {
        result.current.refetch();
      });

      await waitFor(() => expect(mockListAliases).toHaveBeenCalledTimes(2));
    });

    it("useAliases_should_update_aliases_after_refetch_returns_new_data", async () => {
      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.aliases).toHaveLength(1);

      mockListAliases.mockResolvedValue({
        aliases: [
          {
            name: "myproj",
            group: "work",
            path: "~/code",
            description: "",
            profile: "",
            program: "claude",
            autoYes: false,
            tags: [],
          },
          {
            name: "newproj",
            group: "",
            path: "~/new",
            description: "",
            profile: "",
            program: "aider",
            autoYes: false,
            tags: [],
          },
        ],
      });

      act(() => {
        result.current.refetch();
      });

      await waitFor(() => expect(result.current.aliases).toHaveLength(2));
      expect(result.current.aliases[1].name).toBe("newproj");
    });

    it("useAliases_should_set_error_when_listAliases_rejects", async () => {
      mockListAliases.mockRejectedValue(new Error("network failure"));

      const { result } = renderHook(() => useAliases());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(result.current.error).toBeInstanceOf(Error);
      expect(result.current.error?.message).toBe("network failure");
    });
  });
});
