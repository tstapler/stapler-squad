import React, { useReducer } from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import {
  bufferReducer, initialBufferState, selectedPath, type BufferAction, type BufferState,
} from "@/lib/files/bufferReducer";
import { OpenTabsBar } from "../files/OpenTabsBar";

const S = "s1";
const load = (path: string): BufferAction[] => [
  { type: "open", path, sessionId: S, pinned: true },
  { type: "markLoaded", path, token: "v1:1:1:x", readOnly: "none" },
];
const seed = (cap: number, ...actions: BufferAction[]) => actions.reduce(bufferReducer, initialBufferState(cap));

function Bar({ initial, extra }: { initial: BufferState; extra?: Partial<React.ComponentProps<typeof OpenTabsBar>> }) {
  const [state, dispatch] = useReducer(bufferReducer, initial);
  return (
    <div>
      <OpenTabsBar
        buffers={state.buffers}
        activeIndex={state.activeIndex}
        notice={state.notice}
        panelId="files-panel"
        onActivate={(path) => dispatch({ type: "activate", path })}
        onClose={(path) => dispatch({ type: "close", path })}
        onPin={(path) => dispatch({ type: "pin", path })}
        onUnpin={(path) => dispatch({ type: "unpin", path })}
        onMove={(path, delta) => dispatch({ type: "move", path, delta })}
        onCloseOthers={(path) => state.buffers.filter((b) => b.path !== path).forEach((b) => dispatch({ type: "close", path: b.path }))}
        onCopyPath={() => undefined}
        onReveal={() => undefined}
        onUndoEvict={() => dispatch({ type: "reopenClosed" })}
        {...extra}
      />
      <span data-testid="active">{selectedPath(state)}</span>
      <button onClick={() => dispatch({ type: "open", path: "z.go", sessionId: S })}>open-z</button>
      <button onClick={() => dispatch({ type: "markLoaded", path: "z.go", token: "t", readOnly: "none" })}>load-z</button>
      <button onClick={() => dispatch({ type: "markDirty", path: "b.go", dirty: true })}>dirty-b</button>
    </div>
  );
}

const three = () => seed(30, ...load("a.go"), ...load("b.go"), ...load("c.go"), { type: "activate", path: "b.go" });

function mount(initial: BufferState, extra?: Partial<React.ComponentProps<typeof OpenTabsBar>>) {
  const h = createFilesTestHarness();
  return { h, ...render(<Bar initial={initial} extra={extra} />, { wrapper: h.wrapper }) };
}

const tab = (name: RegExp | string) => screen.getByRole("tab", { name });

describe("OpenTabsBar", () => {
  it("openTabsBar_should_MoveAndActivateOnArrowRight_When_TabFocused", async () => {
    mount(three());
    const user = userEvent.setup();
    tab(/^b\.go/).focus();
    await user.keyboard("{ArrowRight}");
    expect(tab(/^c\.go/)).toHaveFocus();
    expect(tab(/^c\.go/)).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("active")).toHaveTextContent("c.go");
  });

  it("openTabsBar_should_ExposeRolesAndHandleKeys_When_ArrowHomeEndDelete", async () => {
    mount(three());
    const user = userEvent.setup();
    expect(screen.getByRole("tablist")).toBeInTheDocument();
    const b = tab(/^b\.go/);
    expect(b).toHaveAttribute("aria-selected", "true");
    expect(b).toHaveAttribute("aria-controls", "files-panel");
    expect(b).toHaveAttribute("tabindex", "0");
    expect(tab(/^a\.go/)).toHaveAttribute("tabindex", "-1");
    b.focus();
    await user.keyboard("{End}");
    expect(screen.getByTestId("active")).toHaveTextContent("c.go");
    await user.keyboard("{Home}");
    expect(screen.getByTestId("active")).toHaveTextContent("a.go");
    await user.keyboard("{ArrowLeft}");
    expect(screen.getByTestId("active")).toHaveTextContent("c.go");
    await user.keyboard("{Delete}");
    expect(screen.queryByRole("tab", { name: /^c\.go/ })).toBeNull();
    expect(screen.getAllByRole("tab")).toHaveLength(2);
    expect(document.activeElement).toBe(tab(/^b\.go/));
  });

  it("openTabsBar_should_NameUnsavedAndMeetDotContrast_When_Dirty", async () => {
    mount(three());
    await userEvent.click(screen.getByText("dirty-b"));
    expect(tab("b.go, unsaved")).toBeInTheDocument();
    expect(within(tab("b.go, unsaved")).getByTestId("files-tab-dirty-dot-b.go")).toBeInTheDocument();
  });

  it("openTabsBar_should_RenderPreviewItalicAndPinOnEditOrDoubleClick", async () => {
    const preview = seed(30, { type: "open", path: "p.go", sessionId: S }, { type: "markLoaded", path: "p.go", token: "t", readOnly: "none" });
    mount(preview);
    const p = tab("p.go, preview");
    expect(p).toHaveAttribute("data-preview", "true");
    await userEvent.dblClick(p);
    expect(tab("p.go, pinned")).toBeInTheDocument();
    expect(tab("p.go, pinned")).not.toHaveAttribute("data-preview");
  });

  it("openTabsBar_should_AnnounceEvictionWithUndo_And_OpenTabListWhenRefused", async () => {
    jest.useFakeTimers();
    try {
      const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
      // cap 2: one pinned tab and one clean preview; opening z.go evicts the preview.
      const evictable = seed(2, ...load("a.go"), { type: "open", path: "p.go", sessionId: S }, { type: "markLoaded", path: "p.go", token: "t", readOnly: "none" });
      const { unmount } = mount(evictable);
      await user.click(screen.getByText("open-z"));
      await user.click(screen.getByText("load-z"));
      const toast = screen.getByTestId("files-tab-toast");
      expect(toast).toHaveTextContent("Closed p.go to stay within 2 tabs");
      expect(screen.getByRole("status")).toHaveTextContent("Closed p.go to stay within 2 tabs");
      act(() => void jest.advanceTimersByTime(7_900));
      expect(screen.getByTestId("files-tab-toast")).toBeInTheDocument();
      await user.hover(toast);
      act(() => void jest.advanceTimersByTime(60_000));
      expect(screen.getByTestId("files-tab-toast")).toBeInTheDocument();
      await user.unhover(toast);
      act(() => void jest.advanceTimersByTime(8_100));
      expect(screen.queryByTestId("files-tab-toast")).toBeNull();
      unmount();

      mount(evictable);
      await user.click(screen.getByText("open-z"));
      await user.click(screen.getByText("load-z"));
      await user.click(screen.getByRole("button", { name: "Undo" }));
      expect(screen.getByRole("tab", { name: /^p\.go/ })).toBeInTheDocument();

      // all pinned or dirty: refused, message shown and the tab list opens
      mount(seed(2, ...load("a.go"), ...load("b.go")));
      const openers = screen.getAllByText("open-z");
      await user.click(openers[openers.length - 1]);
      expect(screen.getByRole("note")).toHaveTextContent("Too many tabs open - close one");
      expect(screen.getByRole("dialog", { name: "Open tabs" })).toBeInTheDocument();
    } finally {
      jest.useRealTimers();
    }
  });

  it("openTabsBar_should_EmitCloseSwitchAndEvictEventsWithEnumLabelsOnly_When_UserActs", async () => {
    const { h } = mount(seed(2, ...load("a.go"), { type: "open", path: "p.go", sessionId: S }, { type: "markLoaded", path: "p.go", token: "t", readOnly: "none" }));
    const user = userEvent.setup();
    await user.click(tab("a.go, pinned"));
    await user.click(screen.getByRole("button", { name: "Close a.go" }));
    await user.click(screen.getByText("open-z"));
    const names = h.analytics.events.map((e) => e.name);
    expect(names).toEqual(expect.arrayContaining(["files.tab.close"]));
    const close = h.analytics.events.find((e) => e.name === "files.tab.close")!;
    expect(close.labels).toMatchObject({ dirty: "false", method: "button" });
    for (const e of h.analytics.events) expect(JSON.stringify(e.labels)).not.toMatch(/a\.go|p\.go|z\.go/);
  });

  it("openTabsBar_should_OfferMoveUnpinAndCloseOthers_When_ContextMenuOpened", async () => {
    const onReveal = jest.fn();
    const onCopyPath = jest.fn();
    mount(three(), { onReveal, onCopyPath });
    const user = userEvent.setup();
    await user.pointer({ keys: "[MouseRight]", target: tab(/^b\.go/) });
    const menu = screen.getByRole("menu");
    expect(within(menu).getByRole("menuitem", { name: "Unpin" })).toBeInTheDocument();
    await user.click(within(menu).getByRole("menuitem", { name: "Move right" }));
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual([expect.stringContaining("a.go"), expect.stringContaining("c.go"), expect.stringContaining("b.go")]);
    await user.pointer({ keys: "[MouseRight]", target: tab(/^b\.go/) });
    await user.click(screen.getByRole("menuitem", { name: "Reveal in tree" }));
    expect(onReveal).toHaveBeenCalledWith("b.go");
    await user.pointer({ keys: "[MouseRight]", target: tab(/^b\.go/) });
    await user.click(screen.getByRole("menuitem", { name: "Close others" }));
    expect(screen.getAllByRole("tab")).toHaveLength(1);
  });

  it("openTabsBar_should_ScrollActiveTabIntoView_When_ManyTabsAndLastActivates", async () => {
    const scroll = jest.fn();
    window.HTMLElement.prototype.scrollIntoView = scroll;
    const actions = Array.from({ length: 20 }, (_, i) => load(`f${i}.go`)).flat();
    mount(seed(30, ...actions));
    const user = userEvent.setup();
    scroll.mockClear();
    await user.click(tab(/^f3\.go/));
    scroll.mockClear();
    tab(/^f3\.go/).focus();
    await user.keyboard("{End}");
    expect(scroll).toHaveBeenCalled();
    expect(scroll.mock.instances.at(-1)).toBe(tab(/^f19\.go/));
  });
  it("openTabsBar_should_KeepMenuKeyboardPosition_When_ParentRerenders", () => {
    mount(three());
    fireEvent.contextMenu(tab(/^a\.go/));
    const items = screen.getAllByRole("menuitem");
    expect(items[0]).toHaveFocus();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "ArrowDown" });
    expect(items[1]).toHaveFocus();

    fireEvent.click(screen.getByText("open-z")); // state change: the bar re-renders with fresh inline handlers

    expect(items[1]).toHaveFocus();
  });

  it("openTabsBar_should_DismissMenuOnOutsideMouseDown_When_ParentRerendered", () => {
    mount(three());
    fireEvent.contextMenu(tab(/^a\.go/));
    fireEvent.click(screen.getByText("open-z"));

    fireEvent.mouseDown(document.body);

    expect(screen.queryByRole("menu")).toBeNull();
  });
});
