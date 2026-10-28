import React, { forwardRef, useImperativeHandle } from "react";
import { render, screen, within } from "@testing-library/react";
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
    return <button data-testid="pick" onClick={() => onFileSelect("pkg/pick.go")}>pick</button>;
  }),
}));

const content = Array.from({ length: 1200 }, (_, i) => `row ${i + 1}`).join("\n");
const originalMM = window.matchMedia;

beforeEach(() => {
  window.localStorage.clear();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1280 });
  window.matchMedia = jest.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: jest.fn(), removeEventListener: jest.fn(),
  })) as never;
});
afterEach(() => {
  window.matchMedia = originalMM;
  window.history.replaceState(null, "", "/");
});

async function open(waitForEditor = true) {
  const h = createFilesTestHarness({
    flags: { "files:viewer-v2": true },
    handlers: {
      getFileContent: () => ({ content, isBinary: false, isTruncated: false, size: BigInt(content.length), versionToken: "v1:1:1:x" }),
      statFile: () => ({ versionToken: "v1:1:1:x", size: BigInt(content.length) }),
    },
  });
  const view = render(<FilesTab sessionId={h.sessionId} baseUrl="http://x" />, { wrapper: h.wrapper });
  await userEvent.click(await screen.findByTestId("pick"));
  if (waitForEditor) await screen.findByTestId("fake-code-editor");
  return { h, ...view };
}

const events = (h: Awaited<ReturnType<typeof open>>["h"]) => h.analytics.events.filter((e) => e.name === "files.a11y.accessible_view");

describe("FilesTab Accessible view toggle", () => {
  it("accessibleView_should_BeSwitchedOnFromSkipLinkAndOffFromToolbar_And_EmitOnlyToViaSize", async () => {
    const { h } = await open();
    const panel = screen.getByRole("tabpanel");
    const first = panel.querySelector<HTMLElement>("button, a, input, [tabindex='0']");
    expect(first).toBe(within(panel).getByRole("button", { name: "Switch to accessible view" }));

    await userEvent.click(first as HTMLElement);
    expect(await screen.findByRole("region", { name: "pick.go, accessible view" })).toBeInTheDocument();
    expect(screen.queryByTestId("fake-code-editor")).not.toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Switch to code editor" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Accessible view" }));
    expect(await screen.findByTestId("fake-code-editor")).toBeInTheDocument();

    const sent = events(h);
    expect(sent.map((e) => [e.labels?.to, e.labels?.via, e.labels?.size])).toEqual([
      ["on", "skip_link", "lt10k"],
      ["off", "toolbar", "lt10k"],
    ]);
    expect(Object.keys(sent[0].labels ?? {}).filter((k) => !["form", "input", "editing", "v2"].includes(k)).sort()).toEqual(["size", "to", "via"]);
  });

  it("accessibleView_should_PersistPerDevice_And_RouteGoToLineIntoPageAndLineFocus", async () => {
    window.localStorage.setItem("files.accessibleView", "1");
    await open(false);
    const region = await screen.findByRole("region", { name: "pick.go, accessible view" });
    expect(within(region).getByTestId("accessible-view-label").textContent).toBe("Lines 1-500 of 1200");

    await userEvent.click(screen.getByRole("button", { name: "Go to line" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Go to line" }), "1034{enter}");
    expect(within(region).getByTestId("accessible-view-label").textContent).toBe("Lines 1001-1200 of 1200");
    expect(document.activeElement).toBe(document.querySelector('[data-line="1034"]'));
    expect(within(region).getByTestId("accessible-view-announce").textContent).toBe("Line 1034");
  });

  it("accessibleView_should_ToggleFromPaletteCommand", async () => {
    const { h } = await open();
    await userEvent.click(screen.getByRole("button", { name: "Quick open" }));
    await userEvent.type(await screen.findByPlaceholderText(/Go to file/), ">accessible");
    await userEvent.click(await screen.findByRole("option", { name: /Switch to accessible view/ }));
    expect(await screen.findByRole("region", { name: "pick.go, accessible view" })).toBeInTheDocument();
    expect(events(h).at(-1)?.labels).toMatchObject({ to: "on", via: "palette" });
  });
});
