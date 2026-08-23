/**
 * Focus-restoration regression test for ReviewChangesModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike ReviewChangesModal.test.tsx)
 * so the real trap-and-restore behavior runs end to end. Also covers the case
 * that motivated capturing the trigger from the click event's
 * `currentTarget` rather than a single shared ref keyed off
 * `document.activeElement`: this modal has two independent openers ("View
 * Changes" and "View Diff" in BacklogItemDetail.tsx), and each must restore
 * focus to its own trigger, not the other's. Capturing from `currentTarget`
 * (mirroring BacklogItemDetail.tsx's real onClick handlers) also works on
 * Safari, which doesn't focus `<button>` elements on click — unlike a
 * `document.activeElement` read, which would silently fail there.
 */

import React, { useRef } from "react";
import type { MouseEvent } from "react";
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
  const activeTriggerRef = useRef<HTMLElement | null>(null);

  const handleOpen = (event: MouseEvent<HTMLButtonElement>) => {
    activeTriggerRef.current = event.currentTarget;
    setOpen(true);
  };

  return (
    <>
      <button data-testid="view-changes" onClick={handleOpen}>
        View Changes
      </button>
      <button data-testid="view-diff" onClick={handleOpen}>
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
