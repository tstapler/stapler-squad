/** Characterises the raw History use in useTrayDismissal before it moves behind the nav facade. */
import { act, renderHook } from "@testing-library/react";
import { useTrayDismissal } from "../useTrayDismissal";

describe("useTrayDismissal raw History use", () => {
  let pushState: jest.SpyInstance;
  let back: jest.SpyInstance;
  beforeEach(() => {
    pushState = jest.spyOn(window.history, "pushState");
    back = jest.spyOn(window.history, "back").mockImplementation(() => {});
  });
  afterEach(() => {
    pushState.mockRestore();
    back.mockRestore();
  });

  it("pushes a state carrying ssqTray:true (keeping the existing state) when the sheet opens, and popstate closes it", () => {
    const close = jest.fn();
    const trayRef = { current: document.createElement("div") };
    window.history.replaceState({ existing: 1 }, "");
    renderHook(() => useTrayDismissal({ isOpen: true, isSheet: true, modal: false, trayRef, close }));

    expect(pushState).toHaveBeenCalledTimes(1);
    expect(pushState.mock.calls[0][0]).toEqual({ existing: 1, ssqTray: true });

    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(close).toHaveBeenCalledTimes(1);
    expect(back).not.toHaveBeenCalled();
  });

  it("does not touch History when the tray is not a sheet", () => {
    const trayRef = { current: document.createElement("div") };
    renderHook(() => useTrayDismissal({ isOpen: true, isSheet: false, modal: false, trayRef, close: jest.fn() }));
    expect(pushState).not.toHaveBeenCalled();
  });
});
