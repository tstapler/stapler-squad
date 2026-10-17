import React, { forwardRef, useImperativeHandle } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import "@/lib/files/__tests__/fixtures/mockCodeEditor";
import { FilesTab } from "../FilesTab";

jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: () => ({ status: null, diff: null, statusLoading: false, refreshStatus: jest.fn() }),
}));
jest.mock("../FileTree", () => ({
  FileTree: forwardRef(function FileTreeStub({ onFileSelect }: { onFileSelect: (p: string) => void }, ref) {
    useImperativeHandle(ref, () => ({ revealPath: jest.fn(), collapseAll: jest.fn() }));
    return <button data-testid="pick" onClick={() => onFileSelect("pick.go")}>pick</button>;
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
  beforeEach(() => window.localStorage.clear());

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
});
