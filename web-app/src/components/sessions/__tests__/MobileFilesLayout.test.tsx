import React, { useReducer } from "react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { bufferReducer, initialBufferState, type BufferAction, type BufferState } from "@/lib/files/bufferReducer";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { useMobilePane } from "@/lib/hooks/useMobilePane";
import { MobileFilesLayout } from "../files/MobileFilesLayout";

const load = (path: string): BufferAction[] => [
  { type: "open", path, sessionId: "s1", pinned: true },
  { type: "markLoaded", path, token: "v1:1:1:x", readOnly: "none" },
];
const seed = (...actions: BufferAction[]) => actions.reduce(bufferReducer, initialBufferState(10));
const THREE = () => seed(...load("a.go"), ...load("b.go"), ...load("c.go"), { type: "activate", path: "b.go" });

interface Spies { back: jest.Mock; forward: jest.Mock; setOverlay: jest.Mock }

function Harness({ initial, width = 390, spies }: { initial: BufferState; width?: number; spies: Spies }) {
  const [state, dispatch] = useReducer(bufferReducer, initial);
  const nav = useMobilePane(true);
  const v2 = {
    buffers: {
      state,
      active: state.buffers[state.activeIndex],
      close: (path: string) => dispatch({ type: "close", path }),
      pin: (path: string) => dispatch({ type: "pin", path }),
      unpin: (path: string) => dispatch({ type: "unpin", path }),
      reopenClosed: () => dispatch({ type: "reopenClosed" }),
    },
    bridge: { activate: (path: string) => dispatch({ type: "activate", path }), back: spies.back, forward: spies.forward, setOverlay: spies.setOverlay },
    feedback: { announceSuccess: jest.fn() },
  } as unknown as FilesV2;
  return (
    <MobileFilesLayout
      active
      v2={v2}
      pane={nav.pane}
      origin={nav.origin}
      containerWidth={width}
      onPane={nav.go}
      onBack={nav.back}
      onQuickOpen={jest.fn()}
      onSearch={() => nav.go("search", "bottom_bar")}
    >
      <div data-testid="panes" data-pane={nav.pane}>
        <button onClick={() => nav.go("editor", "tap_hit")}>tap-file</button>
      </div>
    </MobileFilesLayout>
  );
}

function mount(initial: BufferState = THREE(), width = 390) {
  const h = createFilesTestHarness();
  const spies = { back: jest.fn(), forward: jest.fn(), setOverlay: jest.fn() };
  const view = render(<Harness initial={initial} width={width} spies={spies} />, { wrapper: h.wrapper });
  return { h, spies, ...view };
}

const pane = () => screen.getByTestId("panes").getAttribute("data-pane");

afterEach(() => window.history.replaceState(null, "", "/"));

describe("MobileFilesLayout", () => {
  it("mobileFilesLayout_should_PushEditorPaneAndPopOnBack_When_FileTapped", async () => {
    const { h } = mount();
    const push = jest.spyOn(window.history, "pushState");
    const url = window.location.href;
    const user = userEvent.setup();
    expect(pane()).toBe("files");

    await user.click(screen.getByText("tap-file"));
    expect(pane()).toBe("editor");
    expect(push).toHaveBeenCalledTimes(1);
    expect(h.analytics.events.find((e) => e.name === "files.mobile.pane_switch")?.labels).toMatchObject({ from: "files", to: "editor", via: "tap_hit" });

    await user.click(screen.getByRole("button", { name: "Back to Files" }));
    await waitFor(() => expect(pane()).toBe("files"));
    expect(window.location.href).toBe(url);

    await user.click(screen.getByText("tap-file"));
    act(() => window.history.back());
    await waitFor(() => expect(pane()).toBe("files"));
    expect(window.location.href).toBe(url);
    const vias = h.analytics.events.filter((e) => e.name === "files.mobile.pane_switch").map((e) => String(e.labels?.via));
    expect(vias).toEqual(["tap_hit", "back_arrow", "tap_hit", "system_back"]);
    push.mockRestore();
  });

  it("mobileFilesLayout_should_FallBackToInMemoryPanes_When_PushStateThrows", async () => {
    mount();
    const push = jest.spyOn(window.history, "pushState").mockImplementation(() => { throw new Error("router race"); });
    const user = userEvent.setup();
    await user.click(screen.getByText("tap-file"));
    expect(pane()).toBe("editor");
    await user.click(screen.getByRole("button", { name: "Back to Files" }));
    expect(pane()).toBe("files");
    push.mockRestore();
  });

  it("tabSwitcherSheet_should_ListThreeRowsAndTrapFocus_When_CountButtonTapped", async () => {
    mount();
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "Open files, 3" });
    await user.click(trigger);
    const dialog = screen.getByRole("dialog", { name: "Open files" });
    const rows = within(dialog).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    rows.forEach((r) => expect(r).toHaveStyle({ minHeight: "44px" }));
    expect(within(dialog).getByRole("heading", { name: "Open files (3)" })).toBeInTheDocument();

    // Focus starts inside and Tab never leaves the sheet.
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    for (let i = 0; i < 20; i++) {
      await user.tab();
      expect(dialog).toContainElement(document.activeElement as HTMLElement);
    }

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Open files" })).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("sheets_should_TrapFocusDismissAndDisableAnimation_When_ReducedMotion", async () => {
    mount();
    const user = userEvent.setup();
    const open = () => user.click(screen.getByRole("button", { name: "Open files, 3" }));

    await open();
    await user.click(screen.getByRole("button", { name: "Close tab list" }));
    expect(screen.queryByRole("dialog", { name: "Open files" })).not.toBeInTheDocument();

    // System Back dismisses through the sheet's own history entry and keeps the pane.
    await open();
    act(() => window.history.back());
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Open files" })).not.toBeInTheDocument());
    expect(pane()).toBe("files");
  });

  it("tabSwitcherSheet_should_ShowEmptyStateWithGoToFiles_When_NoBuffers", async () => {
    mount(initialBufferState(10));
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Open files, 0" }));
    expect(screen.getByText("No open files - open one from Files")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Go to Files" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("mobileBottomBar_should_ShowFiveGroupsAt360_And_CollapseToMoreBelow", async () => {
    const wide = mount(THREE(), 360);
    const names = (nav: HTMLElement) => within(nav).getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(names(screen.getByRole("navigation", { name: "Files actions" }))).toEqual(
      ["Files", "Search", "Quick open", "Go to line", "Previous location", "Next location"],
    );
    wide.unmount();

    mount(THREE(), 320);
    const nav = screen.getByRole("navigation", { name: "Files actions" });
    expect(names(nav)).toEqual(["Files", "Search", "Quick open", "Previous location", "More"]);
    const user = userEvent.setup();
    await user.click(within(nav).getByRole("button", { name: "More" }));
    const menu = screen.getByRole("menu", { name: "More" });
    expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Go to line", "Next location"]);
  });

  it("backModel_should_ReturnToResultsOnPaneBack_And_PreviousLocationOnJumplistBack", async () => {
    const { spies } = mount();
    const user = userEvent.setup();
    // Search, open a hit (the editor is pushed over the search pane).
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(pane()).toBe("search");
    await user.click(screen.getByText("tap-file"));
    expect(pane()).toBe("editor");

    // The bottom-bar Back is the jumplist: it moves the location and never the pane.
    await user.click(screen.getByRole("button", { name: "Previous location" }).closest("button") as HTMLElement);
    expect(spies.back).toHaveBeenCalledTimes(1);
    expect(spies.back).toHaveBeenCalledWith("button");
    expect(pane()).toBe("editor");

    // The top-bar arrow and system Back both return to the results.
    expect(screen.getByRole("button", { name: "Back to results" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Back to results" }));
    await waitFor(() => expect(pane()).toBe("search"));

    await user.click(screen.getByText("tap-file"));
    act(() => window.history.back());
    await waitFor(() => expect(pane()).toBe("search"));
  });

  it("androidBack_should_NotChangePaneOrModeOrHistory_When_KeyboardDismissedWithoutPopstate", async () => {
    const vv = Object.assign(new EventTarget(), { height: 844, offsetTop: 0 });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: vv });
    try {
      mount();
      const user = userEvent.setup();
      await user.click(screen.getByText("tap-file"));
      const push = jest.spyOn(window.history, "pushState");
      const back = jest.spyOn(window.history, "back");
      const depth = window.history.length;

      act(() => { vv.height = 500; vv.dispatchEvent(new Event("resize")); });
      act(() => { vv.height = 844; vv.dispatchEvent(new Event("resize")); });

      expect(pane()).toBe("editor");
      expect(push).not.toHaveBeenCalled();
      expect(back).not.toHaveBeenCalled();
      expect(window.history.length).toBe(depth);

      act(() => window.history.back());
      await waitFor(() => expect(pane()).toBe("files"));
      push.mockRestore();
      back.mockRestore();
    } finally {
      delete (window as { visualViewport?: unknown }).visualViewport;
    }
  });

  it("overlays_should_RestoreFocusToInvoker_When_SheetClosedByXOrEscape", async () => {
    mount();
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "Open files, 3" });
    for (const close of [() => user.click(screen.getByRole("button", { name: "Close tab list" })), () => user.keyboard("{Escape}")]) {
      trigger.focus();
      await user.click(trigger);
      await close();
      await waitFor(() => expect(trigger).toHaveFocus());
    }
  });
});
