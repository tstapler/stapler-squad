import { renderHook } from "@testing-library/react";
import { useWindowsController } from "../useWindowsController";

jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({ useFeatureFlagValue: jest.fn(() => false) }));

describe("useWindowsController", () => {
  it("controller_should_BindDispatchToItsOwnWindow_And_KeepIdentityStable", () => {
    const dispatchPane = jest.fn();
    const { result, rerender } = renderHook((p) => useWindowsController(p), {
      initialProps: { currentWindowId: "w1", dispatchPane },
    });
    const d1 = result.current.paneDispatchFor("w1");
    const d2 = result.current.paneDispatchFor("w2");
    const action = { type: "FOCUS_PANE", paneId: "p" } as never;
    d2(action);
    d1(action);
    expect(dispatchPane).toHaveBeenNthCalledWith(1, "w2", action);
    expect(dispatchPane).toHaveBeenNthCalledWith(2, "w1", action);
    rerender({ currentWindowId: "w2", dispatchPane });
    expect(result.current.paneDispatchFor("w1")).toBe(d1);
  });

  it("controller_should_PointActiveDetailRefAtCurrentWindowContainer_When_CurrentChanges", () => {
    const dispatchPane = jest.fn();
    const { result, rerender } = renderHook((p) => useWindowsController(p), {
      initialProps: { currentWindowId: "w1", dispatchPane },
    });
    const el1 = document.createElement("div");
    const el2 = document.createElement("div");
    result.current.detailRefFor("w1").current = el1;
    result.current.detailRefFor("w2").current = el2;
    const active = result.current.activeDetailRef;
    expect(active.current).toBe(el1);
    rerender({ currentWindowId: "w2", dispatchPane });
    expect(result.current.activeDetailRef).toBe(active);
    expect(active.current).toBe(el2);
  });

  it("controller_should_ExposeFlagValue_As_KeepAlive", () => {
    const { result } = renderHook(() => useWindowsController({ currentWindowId: "w1", dispatchPane: jest.fn() }));
    expect(result.current.keepAlive).toBe(false);
  });
});
