/**
 * Tests for useShells hook.
 *
 * Covers:
 *  - spawnShell calls SpawnShell RPC with correct params (sessionId, name, command, workingDir)
 *  - spawnShell adds the returned shell to the shells list (refetch side-effect via state update)
 *  - stopShell calls StopShell RPC with the correct shellId
 *  - deleteShell calls DeleteShell RPC with the correct shellId
 */

import { renderHook, act } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Mock ConnectRPC to prevent real transport creation
jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(),
}));

// Mock the watch transport so no WebSocket is opened
jest.mock("@/lib/transport/watch-ws-transport", () => ({
  createWatchTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => () => ({}),
}));

jest.mock("@/lib/telemetry/rpcTiming", () => ({
  createRpcTimingInterceptor: () => () => ({}),
}));

// Minimal proto stubs — just enough for the hook to import without error
jest.mock("@/gen/session/v1/session_pb", () => ({
  SessionService: {},
  SpawnShellRequest: {},
}));

jest.mock("@/gen/session/v1/types_pb", () => ({
  Shell: {},
  ShellStatus: {
    STOPPED: 1,
    ERROR: 2,
  },
}));

// AnalyticsContext — return null so the hook uses the auth-only interceptor path
jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  AnalyticsContext: { _currentValue: null },
}));

import { createClient } from "@connectrpc/connect";
import { useShells } from "../useShells";

// ---------------------------------------------------------------------------
// Mock client factory
// ---------------------------------------------------------------------------

function makeMockClient() {
  return {
    listShells: jest.fn().mockResolvedValue({ shells: [] }),
    spawnShell: jest.fn().mockResolvedValue({
      shell: {
        id: "shell-1",
        name: "my-shell",
        command: "bash",
        status: 0,
        exitCode: 0,
      },
    }),
    stopShell: jest.fn().mockResolvedValue({ success: true }),
    restartShell: jest.fn().mockResolvedValue({ success: true }),
    deleteShell: jest.fn().mockResolvedValue({ success: true }),
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useShells", () => {
  let mockClient: ReturnType<typeof makeMockClient>;

  beforeEach(() => {
    jest.clearAllMocks();
    mockClient = makeMockClient();
    (createClient as jest.Mock).mockReturnValue(mockClient);
  });

  describe("spawnShell_should_callRpc_With_correctParams", () => {
    it("useShells_should_callSpawnShellRpc_When_spawnShellCalledWithParams", async () => {
      const { result } = renderHook(() => useShells("session-abc"));

      await act(async () => {
        await result.current.spawnShell({
          name: "my-shell",
          command: "bash",
          workingDir: "/home/user",
        });
      });

      expect(mockClient.spawnShell).toHaveBeenCalledTimes(1);
      expect(mockClient.spawnShell).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: "session-abc",
          name: "my-shell",
          command: "bash",
          workingDir: "/home/user",
        })
      );
    });

    it("useShells_should_passEmptyStrings_When_optionalParamsOmitted", async () => {
      const { result } = renderHook(() => useShells("session-xyz"));

      await act(async () => {
        await result.current.spawnShell({});
      });

      expect(mockClient.spawnShell).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: "session-xyz",
          name: "",
          command: "",
          workingDir: "",
        })
      );
    });
  });

  describe("spawnShell_should_refetchShells_After_success", () => {
    it("useShells_should_addShellToList_When_spawnShellSucceeds", async () => {
      const { result } = renderHook(() => useShells("session-abc"));

      expect(result.current.shells).toHaveLength(0);

      await act(async () => {
        await result.current.spawnShell({ name: "my-shell", command: "bash" });
      });

      // The hook appends the returned shell to the local state
      expect(result.current.shells).toHaveLength(1);
      expect(result.current.shells[0].id).toBe("shell-1");
    });
  });

  describe("stopShell_should_callRpc_With_shellId", () => {
    it("useShells_should_callStopShellRpc_When_stopShellCalled", async () => {
      const { result } = renderHook(() => useShells("session-abc"));

      await act(async () => {
        await result.current.stopShell("shell-42");
      });

      expect(mockClient.stopShell).toHaveBeenCalledTimes(1);
      expect(mockClient.stopShell).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: "session-abc",
          shellId: "shell-42",
        })
      );
    });

    it("useShells_should_returnTrue_When_stopShellSucceeds", async () => {
      const { result } = renderHook(() => useShells("session-abc"));

      let returned: boolean | undefined;
      await act(async () => {
        returned = await result.current.stopShell("shell-42");
      });

      expect(returned).toBe(true);
    });
  });

  describe("deleteShell_should_callRpc_With_shellId", () => {
    it("useShells_should_callDeleteShellRpc_When_deleteShellCalled", async () => {
      const { result } = renderHook(() => useShells("session-abc"));

      await act(async () => {
        await result.current.deleteShell("shell-99");
      });

      expect(mockClient.deleteShell).toHaveBeenCalledTimes(1);
      expect(mockClient.deleteShell).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: "session-abc",
          shellId: "shell-99",
        })
      );
    });

    it("useShells_should_removeShellFromList_When_deleteShellSucceeds", async () => {
      // Pre-populate shells state by spawning one first
      const { result } = renderHook(() => useShells("session-abc"));

      await act(async () => {
        await result.current.spawnShell({ name: "to-delete", command: "bash" });
      });
      expect(result.current.shells).toHaveLength(1);
      expect(result.current.shells[0].id).toBe("shell-1");

      // Now delete it
      await act(async () => {
        await result.current.deleteShell("shell-1");
      });

      expect(result.current.shells).toHaveLength(0);
    });
  });
});
