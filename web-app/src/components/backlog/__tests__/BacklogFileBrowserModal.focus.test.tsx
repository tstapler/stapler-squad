/**
 * Focus-restoration regression test for BacklogFileBrowserModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike BacklogFileBrowserModal.test.tsx)
 * so the real trap-and-restore behavior runs end to end. Captures the trigger
 * from the click event's `currentTarget` (mirroring BacklogItemDetail.tsx's
 * real onBrowseFiles handler) rather than a ref attached directly to the
 * button — this exercises the same capture path as production, which also
 * works on Safari (no `<button>` auto-focus-on-click there, so a
 * `document.activeElement` read would silently fail).
 */

import React, { useRef } from "react";
import type { MouseEvent } from "react";
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
  const triggerRef = useRef<HTMLElement | null>(null);

  return (
    <>
      <button
        data-testid="open-browser"
        onClick={(event: MouseEvent<HTMLButtonElement>) => {
          triggerRef.current = event.currentTarget;
          setOpen(true);
        }}
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
