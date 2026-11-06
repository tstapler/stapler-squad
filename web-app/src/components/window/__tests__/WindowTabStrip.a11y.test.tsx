import { fireEvent, render, screen } from "@testing-library/react";
import { WindowTabStrip } from "../WindowTabStrip";
import type { NamedWindow } from "@/lib/window/windowTypes";
import { initialPaneState } from "@/lib/pane/paneReducer";

const makeWindow = (id: string, name: string): NamedWindow => ({ id, name, paneState: initialPaneState() });
const windows = [makeWindow("w1", "Alpha"), makeWindow("w2", "Beta"), makeWindow("w3", "Gamma")];

function setup(currentWindowId = "w1") {
  const onSwitch = jest.fn();
  render(
    <WindowTabStrip windows={windows} currentWindowId={currentWindowId} onSwitch={onSwitch} onCreate={jest.fn()} onClose={jest.fn()} onRename={jest.fn()} />,
  );
  return onSwitch;
}

describe("WindowTabStrip a11y", () => {
  it("activeTab_should_ExposeAriaSelectedAndNoAriaCurrent_When_WindowSelected", () => {
    setup("w2");
    expect(document.querySelectorAll('[role=tab][aria-selected="true"]')).toHaveLength(1);
    expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("aria-selected", "true");
    expect(document.querySelectorAll("[aria-current]")).toHaveLength(0);
  });

  it("tabs_should_HaveIds_And_ControlPanelOnlyOnSelectedTab", () => {
    setup("w2");
    expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("id", "window-tab-w1");
    expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("aria-controls", "window-panel-w2");
    expect(screen.getByRole("tab", { name: "Alpha" })).not.toHaveAttribute("aria-controls");
  });

  it("tablistKeys_should_SwitchWithTabKeyTriggerAndFocusTarget_When_ArrowHomeEndPressed", () => {
    const onSwitch = setup("w2");
    const tab = screen.getByRole("tab", { name: "Beta" });
    tab.focus();
    fireEvent.keyDown(tab, { key: "ArrowRight" });
    expect(onSwitch).toHaveBeenLastCalledWith("w3", "tab-key");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Gamma" }));
    fireEvent.keyDown(tab, { key: "Home" });
    expect(onSwitch).toHaveBeenLastCalledWith("w1", "tab-key");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Alpha" }));
  });

  it.each(["Enter", " "])("tablistKeys_should_KeepFocusOnTab_When_%j_Pressed", (key) => {
    const onSwitch = setup("w1");
    const tab = screen.getByRole("tab", { name: "Beta" });
    tab.focus();
    fireEvent.keyDown(tab, { key });
    expect(onSwitch).toHaveBeenCalledWith("w2", "tab-key");
    expect(document.activeElement).toBe(tab);
  });

  it("click_should_ReportTriggerFromPointerTypeAndDetail", () => {
    const onSwitch = setup("w1");
    const tab = screen.getByRole("tab", { name: "Beta" });
    fireEvent.click(tab, { detail: 1 });
    expect(onSwitch).toHaveBeenLastCalledWith("w2", "mouse");
    fireEvent.click(tab, { detail: 0 });
    expect(onSwitch).toHaveBeenLastCalledWith("w2", "tab-key");
    const touch = new MouseEvent("click", { bubbles: true, detail: 1 });
    Object.defineProperty(touch, "pointerType", { value: "touch" });
    tab.dispatchEvent(touch);
    expect(onSwitch).toHaveBeenLastCalledWith("w2", "touch");
  });
});
