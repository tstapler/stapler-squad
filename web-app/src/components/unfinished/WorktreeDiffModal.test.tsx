import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { WorktreeDiffModal } from "./WorktreeDiffModal";

const getWorktreeDiff = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getWorktreeDiff: (...args: unknown[]) => getWorktreeDiff(...args),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

describe("WorktreeDiffModal", () => {
  beforeEach(() => {
    getWorktreeDiff.mockReset();
  });

  it("WorktreeDiffModal_should_wrapFocusToCloseButton_When_TabPressedOnLastStaticElement", () => {
    getWorktreeDiff.mockReturnValue(new Promise(() => {})); // stays loading

    render(<WorktreeDiffModal repoPath="/repo" branch="main" repoName="my-repo" onClose={jest.fn()} />);

    const closeButton = screen.getByRole("button", { name: /close diff/i });

    closeButton.focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(closeButton);

    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(closeButton);
  });

  it("WorktreeDiffModal_should_includeAsyncRenderedControl_When_TabPressedAfterDiffFetchResolves", async () => {
    const diffContent = [
      "diff --git a/foo.txt b/foo.txt",
      "+++ b/foo.txt",
      "@@ -1,1 +1,2 @@",
      " context line",
      "+added line",
    ].join("\n");
    getWorktreeDiff.mockResolvedValue({
      diffStats: { content: diffContent, added: 1, removed: 0 },
      error: "",
    });

    render(<WorktreeDiffModal repoPath="/repo" branch="main" repoName="my-repo" onClose={jest.fn()} />);

    const closeButton = screen.getByRole("button", { name: /close diff/i });
    // "Unified" is the last focusable element rendered by DiffRenderer once the
    // diff resolves — "Split" is disabled and excluded from the trap.
    const unifiedButton = await screen.findByRole("button", { name: "Unified" });

    closeButton.focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(unifiedButton);
  });

  it("WorktreeDiffModal_should_callOnClose_When_EscapePressed", () => {
    getWorktreeDiff.mockReturnValue(new Promise(() => {}));
    const onClose = jest.fn();

    render(<WorktreeDiffModal repoPath="/repo" branch="main" repoName="my-repo" onClose={onClose} />);

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
