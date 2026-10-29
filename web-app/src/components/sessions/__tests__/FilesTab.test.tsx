import React, { forwardRef, useImperativeHandle } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Code, ConnectError } from "@connectrpc/connect";
import { createFilesTestHarness, installVirtualViewport } from "@/lib/files/__tests__/fixtures";
import { fakeCodeEditorProps } from "@/lib/files/__tests__/fixtures/mockCodeEditor";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { FilesTab } from "../FilesTab";
import { storageKey } from "@/lib/hooks/useOpenBuffers";

jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: () => ({ status: null, diff: null, statusLoading: false, refreshStatus: jest.fn(), ensureDiff: jest.fn() }),
}));
jest.mock("../FileTree", () => ({
  FileTree: forwardRef(function FileTreeStub({ onFileSelect }: { onFileSelect: (p: string) => void }, ref) {
    useImperativeHandle(ref, () => ({ revealPath: jest.fn(), collapseAll: jest.fn() }));
    return <button data-testid="pick" onClick={() => onFileSelect("pick.go")}>{/* analytics-exempt */}pick</button>;
  }),
}));

const text = (content: string) => () => ({ content, isBinary: false, isTruncated: false, size: BigInt(content.length), versionToken: "v1:1:1:x" });

function mount(flags: Record<string, boolean>, props: Partial<React.ComponentProps<typeof FilesTab>> = {}) {
  const h = createFilesTestHarness({
    flags,
    handlers: { getFileContent: text("hello"), statFile: () => ({ versionToken: "v1:1:1:x", size: 5n }) },
  });
  const onSelectedPathChange = jest.fn();
  render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" onSelectedPathChange={onSelectedPathChange} {...props} />, { wrapper: h.wrapper });
  return { h, onSelectedPathChange };
}

describe("FilesTab", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetFlagStoreForTest();
  });

  it("filesTab_should_ActivateInitialPathAndNotifyParent_When_ViewerV2On", async () => {
    const { h, onSelectedPathChange } = mount({ "files:viewer-v2": true }, { initialSelectedPath: "src/main.go" });
    expect(screen.getByTestId("files-tab-skeleton")).toBeInTheDocument();
    const editor = await screen.findByTestId("fake-code-editor");
    expect(editor).toHaveAttribute("data-path", "src/main.go");
    expect(onSelectedPathChange).toHaveBeenCalledWith("src/main.go");
    await waitFor(() => expect(h.calls).toContain("statFile"));
  });

  it("filesTab_should_FollowTheBufferSelection_When_UserPicksAFile", async () => {
    const { onSelectedPathChange } = mount({ "files:viewer-v2": true });
    await userEvent.click(await screen.findByTestId("pick"));
    expect(await screen.findByTestId("fake-code-editor")).toHaveAttribute("data-path", "pick.go");
    expect(onSelectedPathChange).toHaveBeenLastCalledWith("pick.go");
  });

  it("filesTab_should_KeepTheLegacySingleSelectionPath_When_ViewerV2Off", async () => {
    const { h, onSelectedPathChange } = mount({ "files:viewer-v2": false });
    await userEvent.click(await screen.findByTestId("pick"));
    expect(await screen.findByTestId("fake-code-editor")).toHaveAttribute("data-path", "pick.go");
    expect(onSelectedPathChange).toHaveBeenCalledWith("pick.go");
    expect(h.calls).not.toContain("statFile");
    expect(Object.keys(window.localStorage).filter((k) => k.startsWith("ssq.files."))).toEqual([]);
  });

  describe("navigation (viewer-v2)", () => {
    const editor = (text: string) => ({ getText: () => text, getCursor: () => ({ line: 1, col: 0 }), gotoLine: jest.fn(), scrollSnapshot: jest.fn(), restoreScroll: jest.fn() });
    const ready = async (handle: ReturnType<typeof editor>) => {
      await screen.findByTestId("fake-code-editor");
      act(() => (fakeCodeEditorProps.last?.onReady as (h: unknown) => void)(handle));
    };

    afterEach(() => window.history.replaceState(null, "", "/"));

    it("filesTab_should_OpenPinnedAtLine_When_QueryDeepLinkOnLoad", async () => {
      window.history.replaceState(null, "", "/session?file=src/main.go&line=42");
      mount({ "files:viewer-v2": true });
      expect(await screen.findByRole("tab", { name: "main.go, pinned" })).toBeInTheDocument();
      const handle = editor("x");
      await ready(handle);
      await waitFor(() => expect(handle.gotoLine).toHaveBeenCalledWith(42));
    });

    it("filesTab_should_ClampCtrlGLine_And_DisableBackAtStart_When_FilesTabVisible", async () => {
      Object.defineProperty(HTMLElement.prototype, "offsetParent", { configurable: true, get() { return this.parentNode; } });
      try {
        mount({ "files:viewer-v2": true });
        await userEvent.click(await screen.findByTestId("pick"));
        const handle = editor(Array.from({ length: 300 }, () => "l").join("\n"));
        await ready(handle);
        await userEvent.keyboard("{Control>}g{/Control}");
        await userEvent.type(await screen.findByRole("textbox", { name: "Go to line" }), "9999{Enter}");
        await waitFor(() => expect(handle.gotoLine).toHaveBeenLastCalledWith(300));
        expect(screen.getByRole("tab", { name: /pick\.go/ })).toHaveAttribute("aria-selected", "true");
        expect(screen.getByRole("button", { name: "Back" })).toHaveAttribute("aria-disabled", "true");
      } finally {
        delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
      }
    });

    it("filesTab_should_ShowStatusBarAndRunGoToLineFromThePalette_When_ViewerV2On", async () => {
      Object.defineProperty(HTMLElement.prototype, "offsetParent", { configurable: true, get() { return this.parentNode; } });
      try {
        mount({ "files:viewer-v2": true });
        await userEvent.click(await screen.findByTestId("pick"));
        const handle = { ...editor("a\nb\r\nc"), getCursor: () => ({ line: 3, col: 4 }) };
        await ready(handle);
        expect(await screen.findByText("Ln 3, Col 5")).toBeInTheDocument();
        expect(screen.getByText("mixed")).toBeInTheDocument();
        expect(screen.getByText("Go")).toBeInTheDocument();
        await userEvent.keyboard("{Control>}p{/Control}");
        await userEvent.type(await screen.findByRole("combobox", { name: "Quick open file" }), ">go to line{Enter}");
        expect(await screen.findByRole("textbox", { name: "Go to line" })).toBeInTheDocument();
      } finally {
        delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
      }
    });
  });

  describe("waits and failures (viewer-v2)", () => {
    function mountWith(statFile: () => unknown) {
      const h = createFilesTestHarness({
        flags: { "files:viewer-v2": true },
        handlers: { getFileContent: text("hello"), statFile: statFile as never },
      });
      render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" />, { wrapper: h.wrapper });
      return h;
    }

    it("filesTab_should_ShowFailureWithRetryAndCloseTab_And_EmitOneOpenFailed_When_StatNotFound", async () => {
      const h = mountWith(() => { throw new ConnectError("gone", Code.NotFound); });
      await userEvent.click(await screen.findByTestId("pick"));
      expect(await screen.findByText("Could not open pick.go: not found")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Reveal in tree" })).toBeInTheDocument();
      const failed = h.analytics.events.filter((e) => e.name === "files.open.failed");
      expect(failed).toHaveLength(1);
      expect(failed[0].labels).toMatchObject({ reason: "not_found" });
      expect(JSON.stringify(failed[0])).not.toContain("pick.go");
      await userEvent.click(screen.getByRole("button", { name: "Close tab" }));
      expect(screen.queryByText(/Could not open/)).toBeNull();
    });

    it("filesTab_should_ShowOpeningPanelAfter150ms_And_RemoveItWhenStatLands", async () => {
      let release: () => void = () => undefined;
      const gate = new Promise<void>((r) => { release = r; });
      mountWith(async () => { await gate; return { versionToken: "v1:1:1:x", size: 5n }; });
      await userEvent.click(await screen.findByTestId("pick"));
      expect(screen.queryByText("Opening pick.go...")).toBeNull();
      const region = await screen.findByText("Opening pick.go...");
      expect(region.closest("[role=status]")).not.toHaveAttribute("aria-busy"); // busy on a live region can hold its announcement
      act(() => release());
      await waitFor(() => expect(screen.queryByText("Opening pick.go...")).toBeNull());
    });

    it("filesTab_should_ShowRestoringTabsAfter150ms_When_SeveralRestoredTabsLoad", async () => {
      let release: () => void = () => undefined;
      const gate = new Promise<void>((r) => { release = r; });
      const h = createFilesTestHarness({
        flags: { "files:viewer-v2": true },
        handlers: { getFileContent: text("hello"), statFile: (async () => { await gate; return { versionToken: "v1:1:1:x", size: 5n }; }) as never },
      });
      const tabs = ["a.go", "b.go", "c.go"].map((path) => ({ path, line: 1, col: 1, scrollTop: 0 }));
      window.localStorage.setItem(storageKey(h.sessionId, "desktop"), JSON.stringify({ v: 1, active: 0, tabs }));
      render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" />, { wrapper: h.wrapper });
      expect(screen.queryByText("Restoring 3 tabs")).toBeNull();
      expect((await screen.findByText("Restoring 3 tabs")).closest("[role=status]")).not.toHaveAttribute("aria-busy");
      act(() => release());
      await waitFor(() => expect(screen.queryByText("Restoring 3 tabs")).toBeNull());
    });

    it("filesTab_should_EmitEnumReasonOnly_When_OpenFailsAsNetworkError", async () => {
      const h = mountWith(() => { throw new ConnectError("secret/path.go down", Code.Unavailable); });
      await userEvent.click(await screen.findByTestId("pick"));
      expect(await screen.findByText("Could not open pick.go: network error")).toBeInTheDocument();
      const failed = h.analytics.events.filter((e) => e.name === "files.open.failed");
      expect(failed).toHaveLength(1);
      expect(failed[0].labels).toMatchObject({ reason: "rpc_error" });
      expect(JSON.stringify(failed)).not.toContain("secret");
    });

    it("filesTab_should_RaiseReloadFailedBannerAndKeepBuffer_When_PaletteReloadStatFails", async () => {
      Object.defineProperty(HTMLElement.prototype, "offsetParent", { configurable: true, get() { return this.parentNode; } });
      try {
        let calls = 0;
        mountWith(() => {
          calls += 1;
          if (calls > 1) throw new ConnectError("down", Code.Unavailable);
          return { versionToken: "v1:1:1:x", size: 5n };
        });
        await userEvent.click(await screen.findByTestId("pick"));
        await screen.findByTestId("fake-code-editor");
        await userEvent.keyboard("{Control>}p{/Control}");
        await userEvent.type(await screen.findByRole("combobox", { name: "Quick open file" }), ">reload{Enter}");
        expect(await screen.findByText("Could not reload: network error")).toBeInTheDocument();
        expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
        expect(screen.getByTestId("fake-code-editor")).toHaveAttribute("data-path", "pick.go");
        expect(screen.getByTestId("files-status-message")).toHaveTextContent("Could not check the file: network error");
      } finally {
        delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
      }
    });
  });

  describe("search (viewer-v2)", () => {
    let restoreViewport: () => void = () => undefined;
    beforeEach(() => {
      restoreViewport = installVirtualViewport();
      Object.defineProperty(HTMLElement.prototype, "offsetParent", { configurable: true, get() { return this.parentNode; } });
    });
    afterEach(() => {
      restoreViewport();
      delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
    });

    const searchable = (matches: { path: string; line: number }[]) => ({
      "files:viewer-v2": true as const,
      handlers: {
        async *searchContent() {
          yield { message: { case: "batch", value: { matches: matches.map((m) => ({ ...m, text: `needle at ${m.line}`, startUtf16: 0, endUtf16: 6, byteOffset: 0n })) } } };
          yield { message: { case: "stats", value: { filesScanned: 1, binarySkipped: 0, truncated: false, durationMs: 1n, largeFilesSkipped: 0 } } };
        },
      },
    });
    function mountSearch(matches: { path: string; line: number }[]) {
      const cfg = searchable(matches);
      const h = createFilesTestHarness({
        flags: { "files:viewer-v2": true },
        handlers: { getFileContent: text("hello"), statFile: () => ({ versionToken: "v1:1:1:x", size: 5n }), ...cfg.handlers } as never,
      });
      render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" sessionName="demo" />, { wrapper: h.wrapper });
      return h;
    }
    const lines = (n: number) => Array.from({ length: n }, () => "l").join("\n");

    it("filesTab_should_ShowSearchPaneAndFocusQuery_When_CtrlShiftF_And_KeepExplorerMounted", async () => {
      mountSearch([]);
      const pick = await screen.findByTestId("pick");
      await userEvent.keyboard("{Control>}{Shift>}f{/Shift}{/Control}");
      const query = await screen.findByRole("searchbox", { name: "Search file contents" });
      expect(query).toHaveFocus();
      expect(screen.getByRole("tab", { name: "Search" })).toHaveAttribute("aria-selected", "true");
      expect(pick.closest("[hidden]")).not.toBeNull();
      await userEvent.click(screen.getByRole("tab", { name: "Explorer" }));
      expect(pick.closest("[hidden]")).toBeNull();
    });

    it("filesTab_should_OpenHitAtLineInAPreviewTab_And_KeepResultsVisible_When_EnterOnAHit", async () => {
      mountSearch([{ path: "src/a.go", line: 42 }]);
      await userEvent.click(await screen.findByRole("tab", { name: "Search" }));
      const query = await screen.findByRole("searchbox", { name: "Search file contents" });
      await userEvent.type(query, "needle");
      expect(await screen.findByText("1 result in 1 file")).toBeInTheDocument();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
      expect(await screen.findByRole("tab", { name: "a.go, preview" })).toBeInTheDocument();
      const handle = { ...({ getText: () => lines(100), getCursor: () => ({ line: 1, col: 0 }), gotoLine: jest.fn(), scrollSnapshot: jest.fn(), restoreScroll: jest.fn(), focus: jest.fn() }) };
      await screen.findByTestId("fake-code-editor");
      act(() => (fakeCodeEditorProps.last?.onReady as (h: unknown) => void)(handle));
      await waitFor(() => expect(handle.gotoLine).toHaveBeenCalledWith(42));
      expect(screen.getByTestId("search-results")).toBeInTheDocument();
      expect(screen.queryByText(/beyond the loaded portion/)).toBeNull();
    });

    it("searchPanel_should_ShowBeyondLoadedBanner_When_HitBeyondTruncatedFile", async () => {
      mountSearch([{ path: "src/big.go", line: 40000 }]);
      await userEvent.click(await screen.findByRole("tab", { name: "Search" }));
      await userEvent.type(await screen.findByRole("searchbox", { name: "Search file contents" }), "needle");
      await screen.findByText("1 result in 1 file");
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
      await screen.findByTestId("fake-code-editor");
      const handle = { getText: () => lines(20000), getCursor: () => ({ line: 1, col: 0 }), gotoLine: jest.fn(), scrollSnapshot: jest.fn(), restoreScroll: jest.fn() };
      act(() => (fakeCodeEditorProps.last?.onReady as (h: unknown) => void)(handle));
      expect(await screen.findByText("Line 40000 is beyond the loaded portion")).toBeInTheDocument();
      await waitFor(() => expect(handle.gotoLine).toHaveBeenCalledWith(40000));
    });

    it("quickfix_should_StepToTheNextHitWithF4_And_ShowTheChip_When_AHitIsOpen", async () => {
      mountSearch([{ path: "src/a.go", line: 3 }, { path: "src/b.go", line: 9 }]);
      await userEvent.click(await screen.findByRole("tab", { name: "Search" }));
      await userEvent.type(await screen.findByRole("searchbox", { name: "Search file contents" }), "needle");
      await screen.findByText("2 results in 2 files");
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
      expect(await screen.findByRole("tab", { name: "a.go, preview" })).toBeInTheDocument();
      expect(screen.getByText("Hit 1/2")).toBeInTheDocument();
      await userEvent.keyboard("{F4}");
      expect(await screen.findByRole("tab", { name: "b.go, preview" })).toBeInTheDocument();
      expect(screen.getByText("Hit 2/2")).toBeInTheDocument();
      await userEvent.keyboard("{F4}");
      expect(await screen.findByText("Wrapped to first hit")).toBeInTheDocument();
      await userEvent.click(screen.getByRole("button", { name: "Dismiss hit navigation" }));
      expect(screen.queryByTestId("quickfix-chip")).toBeNull();
    });
  });
});
