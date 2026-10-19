import { render, screen, fireEvent } from "@testing-library/react";
import { OpenTabsBar, type OpenTabsBarProps } from "./OpenTabsBar";
import type { Buffer } from "@/lib/files/bufferReducer";

const emit = jest.fn();
jest.mock("@/lib/files/analytics/filesAnalytics", () => ({ useFilesAnalytics: () => ({ emit }) }));

const buf = (path: string): Buffer => ({
  path, sessionId: "s", pinned: false, viewState: { line: 1, col: 0, scrollTop: 0 }, usedAt: 0, status: "loading",
});

const props = (over: Partial<OpenTabsBarProps> = {}): OpenTabsBarProps => ({
  buffers: [buf("a.ts"), buf("b.ts")], activeIndex: 0, notice: null, panelId: "panel",
  onActivate: jest.fn(), onClose: jest.fn(), onPin: jest.fn(), onUnpin: jest.fn(), onMove: jest.fn(),
  onCloseOthers: jest.fn(), onCopyPath: jest.fn(), onReveal: jest.fn(), onUndoEvict: jest.fn(), ...over,
});

describe("OpenTabsBar TabMenu", () => {
  beforeEach(() => emit.mockClear());

  it("tabMenu_should_KeepKeyboardPosition_When_ParentRerenders", () => {
    const { rerender } = render(<OpenTabsBar {...props()} />);
    fireEvent.contextMenu(screen.getByTestId("files-tab-a.ts"));
    const items = screen.getAllByRole("menuitem");
    expect(document.activeElement).toBe(items[0]);
    fireEvent.keyDown(screen.getByRole("menu"), { key: "ArrowDown" });
    expect(document.activeElement).toBe(items[1]);

    rerender(<OpenTabsBar {...props()} />); // fresh handlers, as on any parent render
    expect(document.activeElement).toBe(items[1]);
  });

  it("tabMenu_should_DismissOnOutsideMouseDown_When_ParentRerendered", () => {
    const { rerender } = render(<OpenTabsBar {...props()} />);
    fireEvent.contextMenu(screen.getByTestId("files-tab-a.ts"));
    rerender(<OpenTabsBar {...props()} />);
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
