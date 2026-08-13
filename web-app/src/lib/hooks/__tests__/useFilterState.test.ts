import { renderHook, act } from "@testing-library/react";
import { useFilterState } from "../useFilterState";

jest.mock("next/navigation", () => ({
  useSearchParams: jest.fn(),
  useRouter: jest.fn(),
}));

import { useSearchParams, useRouter } from "next/navigation";

describe("useFilterState", () => {
  let mockReplace: jest.Mock;

  beforeEach(() => {
    mockReplace = jest.fn();
    (useRouter as jest.Mock).mockReturnValue({ replace: mockReplace });
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams(""));
  });

  it("returns undefined for unknown key initially", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams(""));
    const { result } = renderHook(() => useFilterState(["status", "tag"] as const));
    expect(result.current.filterState.status).toBeUndefined();
    expect(result.current.filterState.tag).toBeUndefined();
  });

  it("returns existing param value when present", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("status=running"));
    const { result } = renderHook(() => useFilterState(["status"] as const));
    expect(result.current.filterState.status).toBe("running");
  });

  it("setFilter(key, value) calls router.replace with the correct URL", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams(""));
    const { result } = renderHook(() => useFilterState(["status"] as const));

    act(() => {
      result.current.setFilter("status", "running");
    });

    expect(mockReplace).toHaveBeenCalledWith("?status=running", { scroll: false });
  });

  it("setFilter(key, value) preserves existing params", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("tag=frontend"));
    const { result } = renderHook(() => useFilterState(["status", "tag"] as const));

    act(() => {
      result.current.setFilter("status", "running");
    });

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const calledUrl = mockReplace.mock.calls[0][0] as string;
    const params = new URLSearchParams(calledUrl.replace(/^\?/, ""));
    expect(params.get("status")).toBe("running");
    expect(params.get("tag")).toBe("frontend");
  });

  it("clearFilters() removes the specified keys from URL", () => {
    (useSearchParams as jest.Mock).mockReturnValue(
      new URLSearchParams("status=running&tag=frontend")
    );
    const { result } = renderHook(() => useFilterState(["status", "tag"] as const));

    act(() => {
      result.current.clearFilters();
    });

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const calledUrl = mockReplace.mock.calls[0][0] as string;
    const params = new URLSearchParams(calledUrl.replace(/^\?/, ""));
    expect(params.get("status")).toBeNull();
    expect(params.get("tag")).toBeNull();
  });

  it("clearFilters() preserves unrelated params", () => {
    (useSearchParams as jest.Mock).mockReturnValue(
      new URLSearchParams("status=running&page=2")
    );
    const { result } = renderHook(() => useFilterState(["status"] as const));

    act(() => {
      result.current.clearFilters();
    });

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const calledUrl = mockReplace.mock.calls[0][0] as string;
    const params = new URLSearchParams(calledUrl.replace(/^\?/, ""));
    expect(params.get("status")).toBeNull();
    expect(params.get("page")).toBe("2");
  });

  it("setFilter with undefined value removes the key", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("status=running"));
    const { result } = renderHook(() => useFilterState(["status"] as const));

    act(() => {
      result.current.setFilter("status", undefined);
    });

    const calledUrl = mockReplace.mock.calls[0][0] as string;
    const params = new URLSearchParams(calledUrl.replace(/^\?/, ""));
    expect(params.get("status")).toBeNull();
  });
});
