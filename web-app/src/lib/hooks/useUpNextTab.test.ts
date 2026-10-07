import { act, renderHook } from "@testing-library/react";
import { useRouter, useSearchParams } from "next/navigation";
import { useUpNextTab } from "./useUpNextTab";
import { UP_NEXT_TAB_STORAGE_KEY } from "@/lib/unfinished/upNextTab";

jest.mock("next/navigation", () => ({
  useRouter: jest.fn(),
  useSearchParams: jest.fn(),
}));

const replace = jest.fn();
const setParams = (qs: string) => (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams(qs));
const fixedNow = () => 1000;

/** Renders the hook and records every render pass (RTL has no `result.all`). */
function renderRecorded() {
  const passes: string[] = [];
  const utils = renderHook(() => {
    const value = useUpNextTab(fixedNow);
    passes.push(value.tab);
    return value;
  });
  return { ...utils, passes };
}

beforeEach(() => {
  window.localStorage.clear();
  replace.mockClear();
  (useRouter as jest.Mock).mockReturnValue({ replace });
  setParams("");
});

describe("useUpNextTab", () => {
  it("useUpNextTab_should_RenderPRsFirstThenStoredTab_When_StoredIsQueue", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "queue");
    const { result, passes } = renderRecorded();
    expect(passes[0]).toBe("prs");
    expect(result.current.tab).toBe("queue");
  });

  it("useUpNextTab_should_ApplyStoredTabInLayoutEffectBeforePaint_When_NoParamsAndStoredQueue", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "queue");
    const { result, passes } = renderRecorded();
    // Layout effect re-renders synchronously within the same commit: no async tick needed.
    expect(result.current.tab).toBe("queue");
    expect(passes).toEqual(["prs", "queue"]);
  });

  it("useUpNextTab_should_WriteStorageAndReplaceUrlDroppingItem_When_UserClicksTab", () => {
    setParams("item=abc&foo=1");
    const { result } = renderHook(() => useUpNextTab(fixedNow));
    act(() => result.current.setTab("queue"));
    expect(window.localStorage.getItem(UP_NEXT_TAB_STORAGE_KEY)).toBe("queue");
    expect(replace).toHaveBeenCalledWith("?foo=1&tab=queue", { scroll: false });
  });

  it("useUpNextTab_should_NotWriteStorage_When_ResolvedFromItemDeepLink", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "worktrees");
    setParams("item=abc");
    const { result } = renderHook(() => useUpNextTab(fixedNow));
    expect(result.current.tab).toBe("stuck");
    expect(window.localStorage.getItem(UP_NEXT_TAB_STORAGE_KEY)).toBe("worktrees");
  });

  it("useUpNextTab_should_RestoreClickedTab_When_RemountedWithRealLocalStorage", () => {
    const first = renderHook(() => useUpNextTab(fixedNow));
    act(() => first.result.current.setTab("worktrees"));
    first.unmount();
    setParams("");
    const second = renderHook(() => useUpNextTab(fixedNow));
    expect(second.result.current.tab).toBe("worktrees");
  });

  it("useUpNextTab_should_ReturnStuckOnVeryFirstRender_When_ItemParamPresent", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "worktrees");
    setParams("item=abc");
    const getItem = jest.spyOn(Storage.prototype, "getItem");
    const { passes } = renderRecorded();
    expect(passes[0]).toBe("stuck");
    expect(getItem).not.toHaveBeenCalledWith(UP_NEXT_TAB_STORAGE_KEY);
    getItem.mockRestore();
  });

  it("useUpNextTab_should_ReturnQueueOnVeryFirstRender_When_TabQueueParamPresent", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "worktrees");
    setParams("tab=queue");
    const { passes } = renderRecorded();
    expect(passes).toEqual(["queue"]);
  });

  it("useUpNextTab_should_FollowUrlWithoutWritingStorage_When_BackForwardChangesSearchParams", () => {
    setParams("tab=queue");
    const { result, rerender } = renderHook(() => useUpNextTab(fixedNow));
    expect(result.current.tab).toBe("queue");
    const setItem = jest.spyOn(Storage.prototype, "setItem");
    setParams("tab=worktrees");
    rerender();
    expect(result.current.tab).toBe("worktrees");
    expect(setItem).not.toHaveBeenCalledWith(UP_NEXT_TAB_STORAGE_KEY, expect.anything());
    setItem.mockRestore();
  });
});
