import React from "react";
import { render } from "@testing-library/react";
import { MobileTopBar } from "../files/MobileBars";

const deckTop = () => document.documentElement.style.getPropertyValue("--mobile-deck-top-offset");

describe("MobileTopBar toast deck anchor", () => {
  afterEach(() => document.documentElement.style.removeProperty("--mobile-deck-top-offset"));

  it("mobileTopBar_should_DockTheToastDeckBelowItself_And_ReleaseItOnUnmount", () => {
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ top: 96, bottom: 144, height: 48, left: 0, right: 390, width: 390, x: 0, y: 96, toJSON: () => ({}) });
    const view = render(<MobileTopBar pane="editor" origin="files" path="a.go" dirty={false} tabCount={1} overflow={[]} onBack={jest.fn()} onTabs={jest.fn()} />);

    expect(deckTop()).toBe("144px"); // a pinned toast must sit under the Back control, not over it

    view.unmount();
    expect(deckTop()).toBe("");
    jest.restoreAllMocks();
  });
});
