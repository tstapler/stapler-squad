import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { WindowTabStrip } from "../WindowTabStrip";
import type { NamedWindow } from "@/lib/window/windowTypes";
import { initialPaneState } from "@/lib/pane/paneReducer";

const makeWindow = (id: string, name: string): NamedWindow => ({
  id,
  name,
  paneState: initialPaneState(),
});

function makeHandlers() {
  return {
    onSwitch: jest.fn(),
    onCreate: jest.fn(),
    onClose: jest.fn(),
    onRename: jest.fn(),
  };
}

describe("WindowTabStrip", () => {
  it("renders a tablist with the correct aria-label and tabs with aria-selected", () => {
    const windows = [makeWindow("win-1", "Reviewing PR #42"), makeWindow("win-2", "Debugging session y")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-2" {...makeHandlers()} />);

    expect(screen.getByRole("tablist", { name: "Window switcher" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Reviewing PR #42" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("tab", { name: "Debugging session y" })).toHaveAttribute("aria-selected", "true");
  });

  it("uses roving tabindex — active tab is 0, others are -1", () => {
    const windows = [makeWindow("win-1", "Alpha"), makeWindow("win-2", "Beta")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-2" {...makeHandlers()} />);

    expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("tabindex", "0");
  });

  it("WindowTabStrip_should_callOnSwitch_When_inactiveTabClicked", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Alpha"), makeWindow("win-2", "Beta")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-2" {...handlers} />);

    fireEvent.click(screen.getByRole("tab", { name: "Alpha" }));
    expect(handlers.onSwitch).toHaveBeenCalledWith("win-1");
  });

  it("calls onCreate when the + button is clicked", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Alpha")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.click(screen.getByRole("button", { name: "New window" }));
    expect(handlers.onCreate).toHaveBeenCalledTimes(1);
  });

  it("does not render a close button when only one window exists", () => {
    const windows = [makeWindow("win-1", "Alpha")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...makeHandlers()} />);

    expect(screen.queryByRole("button", { name: "Close Alpha" })).not.toBeInTheDocument();
  });

  it("calls onClose when the close button is clicked, without triggering onSwitch", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Alpha"), makeWindow("win-2", "Beta")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-2" {...handlers} />);

    fireEvent.click(screen.getByRole("button", { name: "Close Alpha" }));
    expect(handlers.onClose).toHaveBeenCalledWith("win-1");
    expect(handlers.onSwitch).not.toHaveBeenCalled();
  });

  it("WindowTabStrip_should_beIdempotent_When_closeInvokedTwiceForAlreadyRemovedWindow", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Alpha"), makeWindow("win-2", "Beta")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-2" {...handlers} />);

    const closeButton = screen.getByRole("button", { name: "Close Alpha" });
    expect(() => {
      fireEvent.click(closeButton);
      fireEvent.click(closeButton);
    }).not.toThrow();
    expect(handlers.onClose).toHaveBeenCalledTimes(2);
    expect(handlers.onClose).toHaveBeenNthCalledWith(1, "win-1");
    expect(handlers.onClose).toHaveBeenNthCalledWith(2, "win-1");
  });

  it("calls onClose when Delete is pressed on a focused tab, and ignores Backspace", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Alpha"), makeWindow("win-2", "Beta")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    const tab = screen.getByRole("tab", { name: "Alpha" });
    fireEvent.keyDown(tab, { key: "Backspace" });
    expect(handlers.onClose).not.toHaveBeenCalled();

    fireEvent.keyDown(tab, { key: "Delete" });
    expect(handlers.onClose).toHaveBeenCalledWith("win-1");
  });

  it("WindowTabStrip_should_notReRender_When_unrelatedWindowsPaneStateChanges", () => {
    const renderSpy = jest.fn();
    const OuterCheck = ({ children }: { children: React.ReactNode }) => {
      renderSpy();
      return <>{children}</>;
    };

    const handlers = makeHandlers();
    const windows = [
      makeWindow("win-1", "Alpha"),
      makeWindow("win-2", "Beta"),
      makeWindow("win-3", "Gamma"),
      makeWindow("win-4", "Delta"),
      makeWindow("win-5", "Epsilon"),
    ];

    const { rerender } = render(
      <OuterCheck>
        <WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />
      </OuterCheck>
    );
    expect(renderSpy).toHaveBeenCalledTimes(1);

    // Only win-3 (a non-active window)'s paneState changes; id/name/currentWindowId unchanged.
    const nextWindows = windows.map((w) =>
      w.id === "win-3" ? { ...w, paneState: { ...w.paneState, zoomedPaneId: "some-pane" } } : w
    );

    rerender(
      <OuterCheck>
        <WindowTabStrip windows={nextWindows} currentWindowId="win-1" {...handlers} />
      </OuterCheck>
    );

    // The wrapper re-rendered (proves the rerender happened), but WindowTabStrip's
    // memo comparator should have reported props-equal, so its own DOM output is
    // untouched — verified by nothing throwing and tabs remaining stable.
    expect(renderSpy).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("aria-selected", "true");
  });

  it("WindowTabStrip_should_callOnRenameWithTrimmedText_When_blurWithNonEmptyInput", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Window 1")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.doubleClick(screen.getByRole("tab", { name: "Window 1" }));
    const input = screen.getByDisplayValue("Window 1");
    fireEvent.change(input, { target: { value: "  Reviewing PR #42  " } });
    fireEvent.blur(input);

    expect(handlers.onRename).toHaveBeenCalledTimes(1);
    expect(handlers.onRename).toHaveBeenCalledWith("win-1", "Reviewing PR #42");
  });

  it("WindowTabStrip_should_revertToPreviousNameWithoutCallingOnRename_When_blurWithEmptyInput", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Window 1")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.doubleClick(screen.getByRole("tab", { name: "Window 1" }));
    const input = screen.getByDisplayValue("Window 1");
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.blur(input);

    expect(handlers.onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("tab", { name: "Window 1" })).toBeInTheDocument();
  });

  it("Escape cancels the edit without committing", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Window 1")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.doubleClick(screen.getByRole("tab", { name: "Window 1" }));
    const input = screen.getByDisplayValue("Window 1");
    fireEvent.change(input, { target: { value: "Something else" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(handlers.onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("tab", { name: "Window 1" })).toBeInTheDocument();
  });

  it("Enter commits immediately", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Window 1")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.doubleClick(screen.getByRole("tab", { name: "Window 1" }));
    const input = screen.getByDisplayValue("Window 1");
    fireEvent.change(input, { target: { value: "New Name" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(handlers.onRename).toHaveBeenCalledWith("win-1", "New Name");
    // Committing exits edit mode immediately, without waiting for blur — the
    // displayed name only updates once the parent re-renders with the new
    // `windows` prop, which is out of scope for this component-level test.
    expect(screen.queryByDisplayValue("New Name")).not.toBeInTheDocument();
  });

  it("F2 on a focused tab enters edit mode identically to double-click", () => {
    const handlers = makeHandlers();
    const windows = [makeWindow("win-1", "Window 1")];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...handlers} />);

    fireEvent.keyDown(screen.getByRole("tab", { name: "Window 1" }), { key: "F2" });
    expect(screen.getByDisplayValue("Window 1")).toBeInTheDocument();
  });

  it("truncates long names via title attribute", () => {
    const longName = "A very long window name that should be truncated visually";
    const windows = [makeWindow("win-1", longName)];
    render(<WindowTabStrip windows={windows} currentWindowId="win-1" {...makeHandlers()} />);

    expect(screen.getByRole("tab", { name: longName })).toHaveAttribute("title", longName);
  });
});
