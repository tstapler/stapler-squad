/**
 * Focus-return regression coverage for the modal's real (unmocked)
 * useFocusTrap wiring — see BacklogItemDetail.tsx's "Browse Files" opener
 * (VcsWidgetHeader.tsx) and .claude/rules/fix-flaky-tests-dont-defer.md's
 * "root-cause, don't defer" precedent.
 */

import React, { useRef, useState } from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogFileBrowserModal } from "../BacklogFileBrowserModal";

jest.mock("@/components/sessions/FileTree", () => ({
  FileTree: () => <div data-testid="mock-file-tree" />,
}));
jest.mock("@/components/sessions/FileContentViewer", () => ({
  FileContentViewer: () => <div data-testid="mock-file-content-viewer" />,
}));
jest.mock("@/lib/hooks/useSessionVcs", () => ({
  useSessionVcs: () => ({ status: null }),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

function BrowseFilesHarness() {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  return (
    <div>
      <button
        data-testid="browse-files-opener"
        onClick={(e) => {
          triggerRef.current = e.currentTarget;
          setOpen(true);
        }}
      >
        Browse Files
      </button>
      {open && (
        <BacklogFileBrowserModal sessionId="s1" onClose={() => setOpen(false)} triggerRef={triggerRef} />
      )}
    </div>
  );
}

describe("BacklogFileBrowserModal focus return", () => {
  it("BacklogFileBrowserModal_should_RestoreFocusToBrowseFilesButton_When_Closed", async () => {
    render(<BrowseFilesHarness />);

    fireEvent.click(screen.getByTestId("browse-files-opener"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /close file browser/i }));

    expect(document.activeElement).toBe(screen.getByTestId("browse-files-opener"));
  });
});
