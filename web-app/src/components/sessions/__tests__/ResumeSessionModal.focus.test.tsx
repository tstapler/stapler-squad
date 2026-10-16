/**
 * Focus-restoration regression test for ResumeSessionModal (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike ResumeSessionModal.test.tsx)
 * so the real trap-and-restore behavior runs end to end. The real caller
 * (src/app/page.tsx's handleResumeRequest) is a single shared funnel handler
 * invoked from multiple session cards' "Resume" buttons, and captures the
 * trigger via `document.activeElement` synchronously inside that handler —
 * not via the click event's `currentTarget`. This harness mirrors that exact
 * wiring (a shared handler reading `document.activeElement`) rather than the
 * `currentTarget` pattern used by ReviewChangesModal, so it proves the actual
 * production shape restores focus correctly with two independent openers.
 */

import React, { useRef } from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import { ResumeSessionModal } from "../ResumeSessionModal";
import type { Session } from "@/gen/session/v1/types_pb";

function makeSession(overrides: Partial<Record<string, unknown>> = {}): Session {
  return {
    id: "session-1",
    title: "My Session",
    tags: [] as string[],
    branch: "",
    program: "",
    path: "",
    ...overrides,
  } as unknown as Session;
}

function Harness() {
  const [open, setOpen] = React.useState(false);
  const triggerRef = useRef<HTMLElement | null>(null);
  const session = makeSession({ id: "s1", title: "Session One" });

  const handleResumeRequest = () => {
    triggerRef.current = document.activeElement as HTMLElement;
    setOpen(true);
  };

  return (
    <>
      <button data-testid="resume-card-1" onClick={handleResumeRequest}>
        Resume Session One
      </button>
      <button data-testid="resume-card-2" onClick={handleResumeRequest}>
        Resume Session Two
      </button>
      {open && (
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={() => setOpen(false)}
          onCancel={() => setOpen(false)}
          triggerRef={triggerRef}
        />
      )}
    </>
  );
}

describe("ResumeSessionModal focus restoration", () => {
  it("ResumeSessionModal_should_restoreFocusToFirstOpener_When_openedFromCard1", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("resume-card-1");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("ResumeSessionModal_should_restoreFocusToSecondOpener_When_openedFromCard2", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("resume-card-2");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("resume-card-1"));
  });

  it("ResumeSessionModal_should_restoreFocus_When_closedViaEscape", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("resume-card-1");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());
    fireEvent.keyDown(screen.getByLabelText("Session Title"), { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
