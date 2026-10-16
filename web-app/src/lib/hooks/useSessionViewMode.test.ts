import { renderHook, act } from "@testing-library/react";
import { useSessionViewMode } from "./useSessionViewMode";

const mockUseDatabases = jest.fn();
jest.mock("./useDatabase", () => ({
  useDatabases: () => mockUseDatabases(),
}));

beforeEach(() => {
  localStorage.clear();
  mockUseDatabases.mockReset();
});

describe("useSessionViewMode", () => {
  it("useSessionViewMode_should_DefaultToList_When_NoStoredValueExistsForWorkspace", () => {
    mockUseDatabases.mockReturnValue({ currentId: "ws-abc123" });
    const { result } = renderHook(() => useSessionViewMode());
    expect(result.current[0]).toBe("list");
  });

  it("useSessionViewMode_should_PersistUnderWorkspaceScopedKey_When_ModeIsSet", () => {
    mockUseDatabases.mockReturnValue({ currentId: "ws-abc123" });
    const { result } = renderHook(() => useSessionViewMode());

    act(() => {
      result.current[1]("board");
    });

    expect(result.current[0]).toBe("board");
    expect(localStorage.getItem("ws-ws-abc123.stapler-squad-session-view-mode")).toBe("board");
  });

  it("useSessionViewMode_should_NotLeakAcrossWorkspaces_When_DifferentWorkspaceIdsAreUsed", () => {
    mockUseDatabases.mockReturnValue({ currentId: "ws-abc123" });
    const first = renderHook(() => useSessionViewMode());
    act(() => {
      first.result.current[1]("board");
    });
    expect(localStorage.getItem("ws-ws-abc123.stapler-squad-session-view-mode")).toBe("board");

    // A different workspace (a fresh mount, mirroring the same-origin reload switchDatabase() triggers)
    mockUseDatabases.mockReturnValue({ currentId: "ws-def456" });
    const second = renderHook(() => useSessionViewMode());

    expect(second.result.current[0]).toBe("list");
    expect(localStorage.getItem("ws-ws-def456.stapler-squad-session-view-mode")).toBeNull();
    // The first workspace's stored value is untouched by mounting the second.
    expect(localStorage.getItem("ws-ws-abc123.stapler-squad-session-view-mode")).toBe("board");
  });

  it("useSessionViewMode_should_FallBackToInMemoryList_When_LocalStorageThrowsOnRead", () => {
    mockUseDatabases.mockReturnValue({ currentId: "ws-abc123" });
    const getItemSpy = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });

    const { result } = renderHook(() => useSessionViewMode());
    expect(result.current[0]).toBe("list");

    getItemSpy.mockRestore();
  });

  it("useSessionViewMode_should_NotThrow_When_LocalStorageThrowsOnWrite", () => {
    mockUseDatabases.mockReturnValue({ currentId: "ws-abc123" });
    const setItemSpy = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });

    const { result } = renderHook(() => useSessionViewMode());
    expect(() => {
      act(() => {
        result.current[1]("board");
      });
    }).not.toThrow();
    // In-memory state still updates even though the write failed.
    expect(result.current[0]).toBe("board");

    setItemSpy.mockRestore();
  });
});
