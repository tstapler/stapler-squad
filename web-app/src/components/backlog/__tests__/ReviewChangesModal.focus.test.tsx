/**
 * Focus-restoration regression test for ReviewChangesModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike ReviewChangesModal.test.tsx)
 * so the real trap-and-restore behavior runs end to end. Also covers the case
 * that motivated capturing document.activeElement at click-time rather than a
 * single shared ref: this modal has two independent openers ("View Changes"
 * and "View Diff" in BacklogItemDetail.tsx), and each must restore focus to
 * its own trigger, not the other's.
 */

import React, { useRef } from "react";
import { render, fireEvent, waitFor } from "@testing-library/react";
import { ReviewChangesModal } from "../ReviewChangesModal";

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getBacklogItemDiff: () => Promise.resolve({ diff: "", added: 0, removed: 0 }),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

function Harness() {
  const [open, setOpen] = React.useState(false);
  const viewChangesRef = useRef<HTMLButtonElement | null>(null);
  const viewDiffRef = useRef<HTMLButtonElement | null>(null);
  const activeTriggerRef = useRef<HTMLElement | null>(null);

  return (
    <>
      <button
        ref={viewChangesRef}
        data-testid="view-changes"
        onClick={() => {
          activeTriggerRef.current = viewChangesRef.current;
          setOpen(true);
        }}
      >
        View Changes
      </button>
      <button
        ref={viewDiffRef}
        data-testid="view-diff"
        onClick={() => {
          activeTriggerRef.current = viewDiffRef.current;
          setOpen(true);
        }}
      >
        View Diff
      </button>
      {open && (
        <ReviewChangesModal
          itemId="item-1"
          onClose={() => setOpen(false)}
          triggerRef={activeTriggerRef}
        />
      )}
    </>
  );
}

describe("ReviewChangesModal focus restoration", () => {
  it("ReviewChangesModal_should_restoreFocusToViewChangesButton_When_openedFromThere", async () => {
    const { getByTestId, getByRole, queryByRole } = render(<Harness />);

    fireEvent.click(getByTestId("view-changes"));
    await waitFor(() => expect(queryByRole("dialog")).not.toBeNull());

    fireEvent.click(getByRole("button", { name: "Close changes viewer" }));

    await waitFor(() => expect(document.activeElement).toBe(getByTestId("view-changes")));
  });

  it("ReviewChangesModal_should_restoreFocusToViewDiffButton_When_openedFromThere", async () => {
    const { getByTestId, getByRole, queryByRole } = render(<Harness />);

    fireEvent.click(getByTestId("view-diff"));
    await waitFor(() => expect(queryByRole("dialog")).not.toBeNull());

    fireEvent.click(getByRole("button", { name: "Close changes viewer" }));

    await waitFor(() => expect(document.activeElement).toBe(getByTestId("view-diff")));
    expect(document.activeElement).not.toBe(getByTestId("view-changes"));
  });

  it("ReviewChangesModal_should_restoreFocus_When_closedViaEscape", async () => {
    const { getByTestId, queryByRole } = render(<Harness />);

    fireEvent.click(getByTestId("view-changes"));
    await waitFor(() => expect(queryByRole("dialog")).not.toBeNull());

    fireEvent.keyDown(window, { key: "Escape" });

    await waitFor(() => expect(document.activeElement).toBe(getByTestId("view-changes")));
  });
});
