/**
 * Focus-restoration regression test for TagEditor (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap so the real trap-and-restore
 * behavior runs end to end. TagEditor has four real production openers that
 * use three distinct capture mechanisms:
 *  - SessionCard.tsx's per-card "Edit Tags" button captures via the click
 *    event's `currentTarget`.
 *  - SessionList.tsx's bulk-tag-edit action captures via `document.activeElement`
 *    inside a shared handler.
 *  - SessionDetailView.tsx's action-sheet "Edit Tags" item and
 *    SessionActionsOverflow.tsx's overflow-menu item both reuse a persistent
 *    button ref (`moreActionsButtonRef` / `overflowButtonRef`) instead of the
 *    transient menu item, since the menu closes/unmounts in the same handler
 *    that opens the editor.
 * This harness covers all three mechanisms to prove multi-opener
 * disambiguation, per this backlog item's explicit callout of TagEditor.
 */

import React, { useRef } from "react";
import type { MouseEvent } from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import { TagEditor } from "../TagEditor";

function Harness() {
  const [open, setOpen] = React.useState(false);
  const triggerRef = useRef<HTMLElement | null>(null);
  const moreActionsButtonRef = useRef<HTMLElement | null>(null);

  const openFromCard = (event: MouseEvent<HTMLButtonElement>) => {
    triggerRef.current = event.currentTarget;
    setOpen(true);
  };

  const openFromBulkAction = () => {
    triggerRef.current = document.activeElement as HTMLElement;
    setOpen(true);
  };

  const openFromActionSheet = () => {
    // Mirrors production: the action sheet/overflow menu closes in this same
    // handler, so the persistent "more actions" button is captured instead
    // of the transient menu item.
    triggerRef.current = moreActionsButtonRef.current;
    setOpen(true);
  };

  return (
    <>
      <button data-testid="edit-tags-card" onClick={openFromCard}>
        Edit Tags
      </button>
      <button data-testid="bulk-edit-tags" onClick={openFromBulkAction}>
        Edit Tags (bulk)
      </button>
      <button
        data-testid="more-actions"
        ref={moreActionsButtonRef as React.RefObject<HTMLButtonElement>}
        onClick={openFromActionSheet}
      >
        More actions
      </button>
      {open && (
        <TagEditor
          tags={["alpha"]}
          onSave={() => setOpen(false)}
          onCancel={() => setOpen(false)}
          sessionTitle="My Session"
          triggerRef={triggerRef}
        />
      )}
    </>
  );
}

describe("TagEditor focus restoration", () => {
  it("TagEditor_should_restoreFocusToCardButton_When_openedViaCurrentTarget", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("edit-tags-card");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("TagEditor_should_restoreFocusToBulkButton_When_openedViaActiveElement", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("bulk-edit-tags");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("edit-tags-card"));
  });

  it("TagEditor_should_restoreFocusToMoreActionsButton_When_openedFromActionSheet", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("more-actions");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("edit-tags-card"));
    expect(document.activeElement).not.toBe(screen.getByTestId("bulk-edit-tags"));
  });

  it("TagEditor_should_restoreFocus_When_closedViaEscape", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("edit-tags-card");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.keyDown(screen.getByPlaceholderText("Add a new tag..."), { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
