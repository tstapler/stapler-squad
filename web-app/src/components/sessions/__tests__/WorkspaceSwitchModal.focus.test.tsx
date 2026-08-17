/**
 * Focus-restoration regression test for WorkspaceSwitchModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap so the real trap-and-restore
 * behavior runs end to end. SessionDetailView.tsx has two independent
 * openers for this modal: a direct "Switch workspace" header button
 * (captures via the click event's `currentTarget`) and a "Switch Workspace"
 * item inside the "more actions" action sheet (captures via the persistent
 * `moreActionsButtonRef`, NOT the transient action-sheet item, since the
 * action sheet closes/unmounts in the same click handler that opens this
 * modal). This harness mirrors both real wiring patterns and proves each
 * restores focus to its own correct element.
 */

import React, { useRef } from "react";
import type { MouseEvent } from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import { WorkspaceSwitchModal } from "../WorkspaceSwitchModal";

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getWorkspaceInfo: () => Promise.resolve({ vcsInfo: undefined }),
    listWorkspaceTargets: () =>
      Promise.resolve({ targets: { bookmarks: [], recentRevisions: [], worktrees: [] } }),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

function Harness() {
  const [open, setOpen] = React.useState(false);
  const triggerRef = useRef<HTMLElement | null>(null);
  const moreActionsButtonRef = useRef<HTMLElement | null>(null);

  const openDirect = (event: MouseEvent<HTMLButtonElement>) => {
    triggerRef.current = event.currentTarget;
    setOpen(true);
  };

  const openFromActionSheet = () => {
    // Mirrors production: the action sheet closes in this same handler, so
    // the persistent "more actions" button is captured, not the transient item.
    triggerRef.current = moreActionsButtonRef.current;
    setOpen(true);
  };

  return (
    <>
      <button data-testid="switch-workspace-direct" onClick={openDirect}>
        Switch workspace
      </button>
      <button
        data-testid="more-actions"
        ref={moreActionsButtonRef as React.RefObject<HTMLButtonElement>}
        onClick={openFromActionSheet}
      >
        More actions
      </button>
      {open && (
        <WorkspaceSwitchModal
          sessionId="session-1"
          sessionName="My Session"
          baseUrl="http://localhost:8543"
          onClose={() => setOpen(false)}
          triggerRef={triggerRef}
        />
      )}
    </>
  );
}

describe("WorkspaceSwitchModal focus restoration", () => {
  it("WorkspaceSwitchModal_should_restoreFocusToDirectButton_When_openedFromHeaderButton", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("switch-workspace-direct");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("WorkspaceSwitchModal_should_restoreFocusToMoreActionsButton_When_openedFromActionSheet", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("more-actions");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(
      screen.getByTestId("switch-workspace-direct")
    );
  });
});
