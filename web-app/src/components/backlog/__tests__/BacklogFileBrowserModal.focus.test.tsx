/**
 * Focus-restoration regression test for BacklogFileBrowserModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike BacklogFileBrowserModal.test.tsx)
 * so the real trap-and-restore behavior runs end to end.
 */

import React, { useRef } from "react";
import { render, fireEvent, waitFor } from "@testing-library/react";
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

function Harness() {
  const [open, setOpen] = React.useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  return (
    <>
      <button
        ref={triggerRef}
        data-testid="open-browser"
        onClick={() => setOpen(true)}
      >
        Browse Files
      </button>
      {open && (
        <BacklogFileBrowserModal
          sessionId="s1"
          onClose={() => setOpen(false)}
          triggerRef={triggerRef}
        />
      )}
    </>
  );
}

describe("BacklogFileBrowserModal focus restoration", () => {
  it("BacklogFileBrowserModal_should_restoreFocusToOpener_When_closedViaButton", async () => {
    const { getByTestId, getByRole, queryByRole } = render(<Harness />);

    fireEvent.click(getByTestId("open-browser"));
    await waitFor(() => expect(queryByRole("dialog")).not.toBeNull());

    fireEvent.click(getByRole("button", { name: "Close file browser" }));

    await waitFor(() => expect(document.activeElement).toBe(getByTestId("open-browser")));
  });

  it("BacklogFileBrowserModal_should_restoreFocusToOpener_When_closedViaEscape", async () => {
    const { getByTestId, queryByRole } = render(<Harness />);

    fireEvent.click(getByTestId("open-browser"));
    await waitFor(() => expect(queryByRole("dialog")).not.toBeNull());

    fireEvent.keyDown(window, { key: "Escape" });

    await waitFor(() => expect(document.activeElement).toBe(getByTestId("open-browser")));
  });
});
