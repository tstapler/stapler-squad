/**
 * Focus-return regression coverage for the modal's real (unmocked)
 * useFocusTrap wiring — see .claude/rules/fix-flaky-tests-dont-defer.md's
 * "root-cause, don't defer" precedent and the backlog item this fast-follows
 * (PR #508 wired trapping but not focus *return*, WCAG 2.4.3).
 *
 * Reproduces BacklogItemDetail.tsx's real pattern: two distinct opener
 * buttons ("View Changes" in ReviewingSection, "View Diff" in
 * VersionControlSection/VcsWidget) share one `changesModalTriggerRef`,
 * populated via `e.currentTarget` at each click site — proving the ref
 * tracks whichever opener was clicked last, not a fixed per-button ref.
 */

import React, { useRef, useState } from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ReviewChangesModal } from "../ReviewChangesModal";

const getBacklogItemDiff = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getBacklogItemDiff: (...args: unknown[]) => getBacklogItemDiff(...args),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

function TwoOpenerHarness() {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  return (
    <div>
      <button
        data-testid="view-changes-opener"
        onClick={(e) => {
          triggerRef.current = e.currentTarget;
          setOpen(true);
        }}
      >
        View Changes
      </button>
      <button
        data-testid="view-diff-opener"
        onClick={(e) => {
          triggerRef.current = e.currentTarget;
          setOpen(true);
        }}
      >
        View Diff
      </button>
      {open && <ReviewChangesModal itemId="item-1" onClose={() => setOpen(false)} triggerRef={triggerRef} />}
    </div>
  );
}

describe("ReviewChangesModal focus return", () => {
  beforeEach(() => {
    getBacklogItemDiff.mockReset();
    getBacklogItemDiff.mockResolvedValue({ diff: "", added: 0, removed: 0 });
  });

  it("ReviewChangesModal_should_RestoreFocusToViewChangesButton_When_OpenedFromViewChangesAndClosed", async () => {
    render(<TwoOpenerHarness />);

    fireEvent.click(screen.getByTestId("view-changes-opener"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /close changes viewer/i }));

    expect(document.activeElement).toBe(screen.getByTestId("view-changes-opener"));
  });

  it("ReviewChangesModal_should_RestoreFocusToViewDiffButtonNotViewChanges_When_OpenedFromViewDiffAndClosed", async () => {
    render(<TwoOpenerHarness />);

    fireEvent.click(screen.getByTestId("view-diff-opener"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /close changes viewer/i }));

    expect(document.activeElement).toBe(screen.getByTestId("view-diff-opener"));
    expect(document.activeElement).not.toBe(screen.getByTestId("view-changes-opener"));
  });
});
