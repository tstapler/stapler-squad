import { renderHook } from "@testing-library/react";
import { usePaneShortcuts } from "../usePaneShortcuts";
import type { PaneState } from "../paneTypes";

const state: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const cockpit = document.createElement("div");
cockpit.setAttribute("data-context", "cockpit");
document.body.appendChild(cockpit);

function pressSplit() {
  cockpit.dispatchEvent(new KeyboardEvent("keydown", { key: "\\", ctrlKey: true, bubbles: true }));
}

describe("usePaneShortcuts windowActive gating (Task 4.1.3d)", () => {
  it("paneShortcuts_should_RunOnlyActiveContainerHandler_When_TwoContainersMounted", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const activeDispatch = jest.fn();
    const hiddenDispatch = jest.fn();
    const ref = { current: null };
    cockpit.tabIndex = 0;
    cockpit.focus();
    const hidden = renderHook(() => usePaneShortcuts(state, hiddenDispatch, ref, false));
    const active = renderHook(() => usePaneShortcuts(state, activeDispatch, ref, true));
    pressSplit();
    expect(activeDispatch).toHaveBeenCalledTimes(1);
    expect(hiddenDispatch).not.toHaveBeenCalled();
    hidden.unmount(); // hidden unmount must not delete the active registration
    pressSplit();
    expect(activeDispatch).toHaveBeenCalledTimes(2);
    active.unmount();
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
