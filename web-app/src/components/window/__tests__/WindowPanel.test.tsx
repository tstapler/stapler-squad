import { render, screen } from "@testing-library/react";
import { WindowPanel } from "../WindowPanel";
import { WindowTabStrip } from "../WindowTabStrip";
import type { NamedWindow } from "@/lib/window/windowTypes";
import { initialPaneState } from "@/lib/pane/paneReducer";

const makeWindow = (id: string, name: string): NamedWindow => ({ id, name, paneState: initialPaneState() });
const handlers = { onSwitch: jest.fn(), onCreate: jest.fn(), onClose: jest.fn(), onRename: jest.fn() };

describe("WindowPanel", () => {
  it("tab_should_ControlTabpanelWithBackReference_When_FlagOffAndOn", () => {
    const windows = [makeWindow("w1", "Alpha"), makeWindow("w2", "Beta")];
    render(
      <>
        <WindowTabStrip windows={windows} currentWindowId="w2" {...handlers} />
        <WindowPanel windowId="w2">content</WindowPanel>
      </>,
    );
    const tab = screen.getByRole("tab", { name: "Beta" });
    const panelId = tab.getAttribute("aria-controls");
    expect(panelId).toBeTruthy();
    const panel = document.getElementById(panelId as string);
    expect(panel).toBe(screen.getByRole("tabpanel"));
    expect(panel?.getAttribute("aria-labelledby")).toBe(tab.id);
    expect(document.getElementById(panel?.getAttribute("aria-labelledby") as string)).toBe(tab);
  });

  it("windowPanel_should_BeFocusableButNotInTabOrder_When_Rendered", () => {
    render(<WindowPanel windowId="w1">content</WindowPanel>);
    expect(screen.getByRole("tabpanel")).toHaveAttribute("tabindex", "-1");
  });

  it("windowPanel_should_KeepOneElementAcrossSwitches_When_WindowIdChanges", () => {
    const { rerender } = render(<WindowPanel windowId="w1">content</WindowPanel>);
    const before = screen.getByRole("tabpanel");
    rerender(<WindowPanel windowId="w2">content</WindowPanel>);
    expect(screen.getByRole("tabpanel")).toBe(before);
    expect(before.id).toBe("window-panel-w2");
  });
});
