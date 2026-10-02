// @feature terminal-scroll-settings
import React, { createRef } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ScrollingPanel, shouldRenderPanelAsOverlay, type ScrollingPanelProps } from "../ScrollingPanel";
import { createScrollSettings } from "@/lib/terminal/scrollOverride";
import { decideScrollTarget } from "@/lib/terminal/scrollRouting";

function props(over: Partial<ScrollingPanelProps> = {}): ScrollingPanelProps {
  return {
    variant: "full",
    override: "auto",
    onOverrideChange: jest.fn(),
    effectiveTarget: "tui-pgkeys",
    gestureScrollEnabled: true,
    onGestureScrollChange: jest.fn(),
    onClose: jest.fn(),
    onOpenFull: jest.fn(),
    renderAsOverlay: false,
    ...over,
  };
}

describe("ScrollingPanel", () => {
  it("panel_should_UseNativeRadiosInLabelledFieldsetWithDescriptions_And_AnnounceChange", async () => {
    const user = userEvent.setup();
    const p = props();
    const { rerender } = render(<ScrollingPanel {...p} />);
    const group = screen.getByRole("group", { name: "How dragging scrolls" });
    expect(group.tagName).toBe("FIELDSET");
    const radios = within(group).getAllByRole("radio");
    expect(radios).toHaveLength(3);
    for (const r of radios) {
      expect(r.tagName).toBe("INPUT");
      expect(r).toHaveAttribute("type", "radio");
      expect(document.getElementById(r.getAttribute("aria-describedby")!)?.textContent).toBeTruthy();
    }
    expect(screen.getByRole("radio", { name: /^Page keys/ })).toHaveAccessibleDescription(
      "Dragging sends Page Up/Down to the app, one page per half-screen. Use for Claude Code or tmux history.",
    );
    expect(screen.getByRole("switch")).toBeInTheDocument();
    expect(screen.getByText(/Turn off if you use a screen reader \(TalkBack\)/)).toBeInTheDocument();
    expect(screen.getByText(/PgDn until you reach the bottom/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^(esc|q)$/i })).toBeNull();

    const status = screen.getByTestId("scrolling-announcer");
    expect(status).toHaveAttribute("role", "status");
    expect(status.textContent).toBe("");
    await user.click(screen.getByRole("radio", { name: /^Page keys/ }));
    expect(p.onOverrideChange).toHaveBeenCalledTimes(1);
    expect(p.onOverrideChange).toHaveBeenCalledWith("tui");
    expect(status.textContent).toBe("Scroll mode: Page keys");
    rerender(<ScrollingPanel {...p} override="tui" />);
    expect(status.textContent).toBe("Scroll mode: Page keys");
    await user.click(screen.getByRole("radio", { name: /^Terminal history/ }));
    expect(status.textContent).toBe("Scroll mode: Terminal history");
  });

  it("toggle_should_ChangeSelectionOnArrowKeys", async () => {
    const user = userEvent.setup();
    const store = createScrollSettings({ getStorage: () => null, onOverrideChange: () => {} });
    const Host = () => {
      const [o, setO] = React.useState(store.getOverride());
      return (
        <ScrollingPanel
          {...props({ override: o, onOverrideChange: (v) => { store.setOverride(v); setO(v); } })}
        />
      );
    };
    render(<Host />);
    screen.getByRole("radio", { name: /Auto/ }).focus();
    await user.keyboard("{ArrowDown}");
    expect(store.getOverride()).toBe("local");
    expect(screen.getByRole("radio", { name: /^Terminal history/ })).toBeChecked();
    await user.keyboard("{ArrowDown}");
    expect(store.getOverride()).toBe("tui");
  });

  it("toggle_should_RouteNextDragToPgKeys_When_TuiSelected", () => {
    const store = createScrollSettings({ getStorage: () => null, onOverrideChange: () => {} });
    const mode = { bufferType: "normal", mouseTrackingMode: "none" } as const;
    expect(decideScrollTarget(mode, undefined, undefined, store.getOverride())).toBe("xterm-local");
    store.setOverride("tui");
    expect(decideScrollTarget(mode, undefined, undefined, store.getOverride())).toBe("tui-pgkeys");
  });

  it("picker_should_CloseAndReturnFocusToChip_When_RouteSelected", async () => {
    const user = userEvent.setup();
    const opener = document.createElement("button");
    document.body.appendChild(opener);
    const openerRef = { current: opener } as React.RefObject<HTMLElement>;
    const p = props({ variant: "picker", openerRef });
    render(<ScrollingPanel {...p} />);
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.getAllByRole("radio")).toHaveLength(3);
    await user.click(screen.getByRole("radio", { name: /^Terminal history/ }));
    expect(p.onOverrideChange).toHaveBeenCalledWith("local");
    expect(p.onClose).toHaveBeenCalledTimes(1);
    expect((p.onOverrideChange as jest.Mock).mock.invocationCallOrder[0]).toBeLessThan(
      (p.onClose as jest.Mock).mock.invocationCallOrder[0],
    );
    expect(opener).toHaveFocus();
    opener.remove();
  });

  it("picker_should_OpenFullPanel_When_MoreSettingsPressed", async () => {
    const user = userEvent.setup();
    const p = props({ variant: "picker", openerRef: createRef() });
    render(<ScrollingPanel {...p} />);
    await user.click(screen.getByRole("button", { name: "More scrolling settings" }));
    expect(p.onOpenFull).toHaveBeenCalledTimes(1);
    expect(p.onClose).not.toHaveBeenCalled();
  });

  it("toggle_should_ShowEffectiveModeUnderAuto", () => {
    const { rerender } = render(<ScrollingPanel {...props({ effectiveTarget: "tui-pgkeys" })} />);
    expect(screen.getByText("now: Page keys")).toBeInTheDocument();
    rerender(<ScrollingPanel {...props({ effectiveTarget: "xterm-local" })} />);
    expect(screen.getByText("now: Terminal history")).toBeInTheDocument();
  });

  it("full panel gesture switch announces once and reports the new value", async () => {
    const user = userEvent.setup();
    const p = props();
    render(<ScrollingPanel {...p} />);
    await user.click(screen.getByRole("switch"));
    expect(p.onGestureScrollChange).toHaveBeenCalledWith(false);
    expect(screen.getByTestId("scrolling-announcer").textContent).toBe("Gesture scrolling: off");
  });

  it("overlay applies maxHeight and every control declares a 44px target", () => {
    render(<ScrollingPanel {...props({ renderAsOverlay: true, maxHeight: 200 })} />);
    expect(screen.getByTestId("scrolling-full")).toHaveStyle({ maxHeight: "200px" });
    const src = require("fs").readFileSync(require("path").join(__dirname, "../ScrollingPanel.css.ts"), "utf8");
    expect((src.match(/minHeight:\s*"44px"/g) ?? []).length).toBeGreaterThanOrEqual(4);
    expect(src).not.toMatch(/animation|transition/);
  });

  it("panel_should_RenderAsOverlay_When_InlineWouldLeaveFewerThanFiveRows", () => {
    expect(shouldRenderPanelAsOverlay(4)).toBe(true);
    expect(shouldRenderPanelAsOverlay(5)).toBe(false);
    expect(shouldRenderPanelAsOverlay(0)).toBe(true);
  });
});
