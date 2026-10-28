import React, { forwardRef, useImperativeHandle } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import "@/lib/files/__tests__/fixtures/mockCodeEditor";
import { FilesTab } from "../FilesTab";

jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: () => ({ status: null, diff: null, statusLoading: false, refreshStatus: jest.fn(), ensureDiff: jest.fn() }),
}));
jest.mock("../FileTree", () => ({
  FileTree: forwardRef(function FileTreeStub({ onFileSelect }: { onFileSelect: (p: string) => void }, ref) {
    useImperativeHandle(ref, () => ({ revealPath: jest.fn(), collapseAll: jest.fn() }));
    return <button data-testid="pick" onClick={() => onFileSelect("pick.go")}>pick</button>;
  }),
}));

type Device = { width: number; screen: [number, number]; coarse: boolean };
const PHONE: Device = { width: 390, screen: [390, 844], coarse: true };
const TABLET: Device = { width: 820, screen: [820, 1180], coarse: true };
const DESKTOP: Device = { width: 1280, screen: [1920, 1080], coarse: false };

const observers: { el: Element; cb: (entries: { contentRect: { width: number } }[]) => void }[] = [];
const resizeTo = (el: Element, width: number) => act(() => observers.filter((o) => o.el === el).forEach((o) => o.cb([{ contentRect: { width } }])));
const originalRO = globalThis.ResizeObserver;
const originalMM = window.matchMedia;

function applyDevice(d: Device) {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: d.width });
  Object.defineProperty(window.screen, "width", { configurable: true, value: d.screen[0] });
  Object.defineProperty(window.screen, "height", { configurable: true, value: d.screen[1] });
  window.matchMedia = jest.fn().mockImplementation((q: string) => ({
    matches: d.coarse && q === "(pointer: coarse)", media: q, addEventListener: jest.fn(), removeEventListener: jest.fn(),
  })) as never;
}

beforeEach(() => {
  window.localStorage.clear();
  observers.length = 0;
  globalThis.ResizeObserver = class {
    cb: never;
    constructor(cb: never) { this.cb = cb; }
    observe(el: Element) { observers.push({ el, cb: this.cb }); }
    disconnect() {}
    unobserve() {}
  } as never;
});
afterEach(() => {
  globalThis.ResizeObserver = originalRO;
  window.matchMedia = originalMM;
  window.history.replaceState(null, "", "/");
});

function mount(device: Device) {
  applyDevice(device);
  const h = createFilesTestHarness({
    flags: { "files:viewer-v2": true },
    handlers: {
      getFileContent: () => ({ content: "hello", isBinary: false, isTruncated: false, size: 5n, versionToken: "v1:1:1:x" }),
      statFile: () => ({ versionToken: "v1:1:1:x", size: 5n }),
    },
  });
  render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" />, { wrapper: h.wrapper });
  return h;
}
const root = async () => (await screen.findByTestId("files-tree-pane")).parentElement as HTMLElement;

describe("FilesTab layout", () => {
  it("desktopToolbar_should_ShowQuickOpenGoToLineSearchButtonsAt44pxOnTouchProfile_And_24pxOnHardware", async () => {
    mount(TABLET);
    expect((await root()).getAttribute("data-layout")).toBe("desktop");
    await userEvent.click(await screen.findByTestId("pick"));
    for (const name of ["Quick open", "Go to line", "Search"]) {
      expect(await screen.findByRole("button", { name })).toHaveStyle({ minHeight: "44px", minWidth: "44px" });
    }
    // No phone chrome: no bottom bar, no tab sheet button, no FAB.
    expect(screen.queryByRole("navigation", { name: "Files actions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Open files, / })).not.toBeInTheDocument();
  });

  it("desktopToolbar_should_Use24pxTargets_When_HardwareKeyboardDesktop", async () => {
    mount(DESKTOP);
    await userEvent.click(await screen.findByTestId("pick"));
    for (const name of ["Quick open", "Go to line", "Search"]) {
      expect(await screen.findByRole("button", { name })).toHaveStyle({ minHeight: "24px" });
    }
  });

  it("desktopToolbar_should_OpenTheSearchPaneAndQuickOpen_When_Tapped", async () => {
    mount(TABLET);
    await userEvent.click(await screen.findByTestId("pick"));
    await userEvent.click(await screen.findByRole("button", { name: "Search" }));
    expect(screen.getByRole("tab", { name: "Search" })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(screen.getByRole("button", { name: "Quick open" }));
    expect(await screen.findByRole("combobox", { name: "Quick open file" })).toBeInTheDocument();
  });

  it("filesTab_should_UseMobileLayoutWithBottomBar_When_PhoneAndOpenEditorPane", async () => {
    mount(PHONE);
    const tab = await root();
    expect(tab.getAttribute("data-layout")).toBe("mobile");
    expect(tab.getAttribute("data-pane")).toBe("files");
    expect(screen.getByRole("navigation", { name: "Files actions" })).toBeInTheDocument();
    await userEvent.click(await screen.findByTestId("pick"));
    expect(tab.getAttribute("data-pane")).toBe("editor");
    expect(await screen.findByTestId("fake-code-editor")).toHaveAttribute("data-path", "pick.go");
    // The tab strip and nav row are replaced by the sheet and bottom bar.
    expect(screen.queryByTestId("files-tab-bar")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open files, 1" })).toBeInTheDocument();
  });

  it("filesTab_should_KeepBuffersAndNotRemountTheEditor_When_LayoutChangesAcrossBreakpoint", async () => {
    mount({ ...PHONE, coarse: false, screen: [1280, 800] });
    const tab = await root();
    expect(tab.getAttribute("data-layout")).toBe("mobile");
    await userEvent.click(await screen.findByTestId("pick"));
    const editor = await screen.findByTestId("fake-code-editor");

    resizeTo(tab, 1000);
    await waitFor(() => expect(tab.getAttribute("data-layout")).toBe("desktop"));
    expect(screen.getByTestId("fake-code-editor")).toBe(editor);
    expect(await screen.findByRole("tab", { name: /^pick\.go/ })).toBeInTheDocument();

    resizeTo(tab, 500);
    await waitFor(() => expect(tab.getAttribute("data-layout")).toBe("mobile"));
    expect(screen.getByTestId("fake-code-editor")).toBe(editor);
  });

  it("filesTab_should_StayMobileInLandscapePhone", async () => {
    mount({ width: 844, screen: [844, 390], coarse: true });
    expect((await root()).getAttribute("data-layout")).toBe("mobile");
  });
});
