import { renderHook, act } from "@testing-library/react";
import { useEffectiveScrollMode, useScrollSettings } from "../useEffectiveScrollMode";
import { createScrollSettings } from "@/lib/terminal/scrollOverride";
import type { ScrollMode, ScrollOverride } from "@/lib/terminal/scrollRouting";

const NORMAL: ScrollMode = { bufferType: "normal", mouseTrackingMode: "none" };
const ALTERNATE: ScrollMode = { bufferType: "alternate", mouseTrackingMode: "none" };

function effective(mode: ScrollMode, override: ScrollOverride, gestureOn = true) {
  return renderHook(() => useEffectiveScrollMode(mode, override, gestureOn)).result.current;
}

describe("useEffectiveScrollMode", () => {
  it("useEffectiveScrollMode_should_ReturnOverrideTarget_When_OverrideLocalOrTui", () => {
    expect(effective(ALTERNATE, "local")).toEqual({ target: "xterm-local", source: "override", gesturesOn: true });
    expect(effective(NORMAL, "tui")).toEqual({ target: "tui-pgkeys", source: "override", gesturesOn: true });
  });

  it("useEffectiveScrollMode_should_FollowTable_When_Auto", () => {
    expect(effective(NORMAL, "auto")).toEqual({ target: "xterm-local", source: "auto", gesturesOn: true });
    expect(effective(ALTERNATE, "auto").target).toBe("tui-pgkeys");
    expect(effective({ bufferType: "normal", mouseTrackingMode: "any" }, "auto").target).toBe("tui-pgkeys");
  });

  it("useEffectiveScrollMode_should_KeepRoute_When_GesturesOff", () => {
    expect(effective(ALTERNATE, "auto", false)).toEqual({ target: "tui-pgkeys", source: "auto", gesturesOn: false });
    expect(effective(NORMAL, "auto", false).target).toBe("xterm-local");
  });

  it("useEffectiveScrollMode_should_KeepIdentity_When_InputsUnchanged", () => {
    const { result, rerender } = renderHook(
      ({ mode }) => useEffectiveScrollMode(mode, "auto", true),
      { initialProps: { mode: NORMAL } },
    );
    const first = result.current;
    rerender({ mode: { ...NORMAL } });
    expect(result.current).toBe(first);
  });
});

describe("useScrollSettings", () => {
  it("useScrollSettings_should_ReRender_When_StoreChanges", () => {
    const store = createScrollSettings({ onOverrideChange: () => {} });
    const { result } = renderHook(() => useScrollSettings(store));
    expect(result.current).toEqual({ override: "auto", gestureScrollEnabled: true });
    act(() => store.setOverride("tui"));
    act(() => store.setGestureScroll(false));
    expect(result.current).toEqual({ override: "tui", gestureScrollEnabled: false });
  });
});
