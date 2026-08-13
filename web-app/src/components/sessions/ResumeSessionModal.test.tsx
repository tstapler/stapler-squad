/**
 * Tests for ResumeSessionModal component.
 *
 * Covers:
 *  - Pre-filled title from session.title (no conflict)
 *  - Conflict detection: title input shows unique name when conflict exists
 *  - Conflict hint rendered only when conflict present
 *  - Submit calls onConfirm with { title, tags }
 *  - Cancel button calls onCancel
 *  - Escape key on title input calls onCancel
 *  - Escape key on tag input calls onCancel
 *  - isSubmitting guard: button disabled / shows "Resuming..." after first click
 *  - Tag management: add and remove tags
 *  - Overlay click calls onCancel
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { ResumeSessionModal } from "./ResumeSessionModal";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Build a minimal Session-shaped object sufficient for the modal.
 * The real Session type is a protobuf Message; cast through unknown for tests.
 */
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

// ---------------------------------------------------------------------------
// Mock: useFocusTrap
// The hook uses DOM focus operations that behave poorly in jsdom.
// ---------------------------------------------------------------------------
jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => undefined,
}));

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ResumeSessionModal", () => {
  describe("title pre-filling", () => {
    it("renders title input pre-filled with session.title when no conflict exists", () => {
      const session = makeSession({ id: "s1", title: "My Session" });
      const otherSession = makeSession({ id: "s2", title: "Other Session" });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session, otherSession]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      const input = screen.getByLabelText("Session Title") as HTMLInputElement;
      expect(input.value).toBe("My Session");
    });

    it("pre-fills title with generated unique name when another session has same title", () => {
      const session = makeSession({ id: "s1", title: "My Session" });
      const conflicting = makeSession({ id: "s2", title: "My Session" });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session, conflicting]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      const input = screen.getByLabelText("Session Title") as HTMLInputElement;
      // generateUniqueName("My Session", ["My Session"]) → "My Session (2)"
      expect(input.value).toBe("My Session (2)");
    });
  });

  describe("conflict hint visibility", () => {
    it("shows conflict hint when another session has the same title", () => {
      const session = makeSession({ id: "s1", title: "My Session" });
      const conflicting = makeSession({ id: "s2", title: "My Session" });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session, conflicting]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByText(/is already in use/i)).toBeInTheDocument();
    });

    it("does not show conflict hint when session title is unique", () => {
      const session = makeSession({ id: "s1", title: "My Session" });
      const other = makeSession({ id: "s2", title: "Other Session" });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session, other]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      expect(screen.queryByText(/is already in use/i)).not.toBeInTheDocument();
    });
  });

  describe("submit behaviour", () => {
    it("calls onConfirm with updated title and existing tags when Resume Session is clicked", () => {
      const onConfirm = jest.fn();
      const session = makeSession({
        id: "s1",
        title: "My Session",
        tags: ["frontend"],
      });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={onConfirm}
          onCancel={jest.fn()}
        />
      );

      // Change the title
      const titleInput = screen.getByLabelText("Session Title");
      fireEvent.change(titleInput, { target: { value: "Renamed Session" } });

      fireEvent.click(screen.getByRole("button", { name: /resume session/i }));

      expect(onConfirm).toHaveBeenCalledTimes(1);
      expect(onConfirm).toHaveBeenCalledWith({
        title: "Renamed Session",
        tags: ["frontend"],
      });
    });

    it("calls onConfirm with a newly added tag", () => {
      const onConfirm = jest.fn();
      const session = makeSession({ id: "s1", title: "My Session", tags: [] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={onConfirm}
          onCancel={jest.fn()}
        />
      );

      // Add a tag via the Add button
      const tagInput = screen.getByPlaceholderText("Add a tag...");
      fireEvent.change(tagInput, { target: { value: "backend" } });
      fireEvent.click(screen.getByRole("button", { name: /^add$/i }));

      fireEvent.click(screen.getByRole("button", { name: /resume session/i }));

      expect(onConfirm).toHaveBeenCalledWith({
        title: "My Session",
        tags: ["backend"],
      });
    });

    it("does not call onConfirm when title is empty", () => {
      const onConfirm = jest.fn();
      const session = makeSession({ id: "s1", title: "My Session" });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={onConfirm}
          onCancel={jest.fn()}
        />
      );

      // Clear the title so it is blank
      const titleInput = screen.getByLabelText("Session Title");
      fireEvent.change(titleInput, { target: { value: "" } });

      const resumeBtn = screen.getByRole("button", { name: /resume session/i });
      expect(resumeBtn).toBeDisabled();

      fireEvent.click(resumeBtn);
      expect(onConfirm).not.toHaveBeenCalled();
    });
  });

  describe("cancel behaviour", () => {
    it("calls onCancel when Cancel button is clicked", () => {
      const onCancel = jest.fn();
      const session = makeSession();

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={onCancel}
        />
      );

      fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));
      expect(onCancel).toHaveBeenCalledTimes(1);
    });

    it("calls onCancel when Escape is pressed in the title input", () => {
      const onCancel = jest.fn();
      const session = makeSession();

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={onCancel}
        />
      );

      const titleInput = screen.getByLabelText("Session Title");
      fireEvent.keyDown(titleInput, { key: "Escape", code: "Escape" });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });

    it("calls onCancel when Escape is pressed in the tag input", () => {
      const onCancel = jest.fn();
      const session = makeSession();

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={onCancel}
        />
      );

      const tagInput = screen.getByPlaceholderText("Add a tag...");
      fireEvent.keyDown(tagInput, { key: "Escape", code: "Escape" });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });

  describe("isSubmitting guard", () => {
    it("shows Resuming... and disables the button after first click", async () => {
      // onConfirm never resolves so isSubmitting stays true
      const onConfirm = jest.fn(() => new Promise<void>(() => {}));
      const session = makeSession();

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={onConfirm}
          onCancel={jest.fn()}
        />
      );

      const resumeBtn = screen.getByRole("button", { name: /resume session/i });

      await act(async () => {
        fireEvent.click(resumeBtn);
      });

      await waitFor(() => {
        const btn = screen.getByRole("button", { name: /resuming\.\.\./i });
        expect(btn).toBeDisabled();
      });
    });

    it("does not call onConfirm a second time when clicked while submitting", async () => {
      const onConfirm = jest.fn(() => new Promise<void>(() => {}));
      const session = makeSession();

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={onConfirm}
          onCancel={jest.fn()}
        />
      );

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: /resume session/i }));
      });

      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: /resuming\.\.\./i })
        ).toBeDisabled()
      );

      // Attempt a second click on the now-disabled button
      fireEvent.click(screen.getByRole("button", { name: /resuming\.\.\./i }));
      expect(onConfirm).toHaveBeenCalledTimes(1);
    });
  });

  describe("tag management", () => {
    it("displays existing tags from session", () => {
      const session = makeSession({ tags: ["alpha", "beta"] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByText("alpha")).toBeInTheDocument();
      expect(screen.getByText("beta")).toBeInTheDocument();
    });

    it("adds a tag when Enter is pressed in the tag input", () => {
      const session = makeSession({ tags: [] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      const tagInput = screen.getByPlaceholderText("Add a tag...");
      fireEvent.change(tagInput, { target: { value: "newTag" } });
      fireEvent.keyDown(tagInput, { key: "Enter", code: "Enter" });

      expect(screen.getByText("newTag")).toBeInTheDocument();
    });

    it("removes a tag when its remove button is clicked", () => {
      const session = makeSession({ tags: ["remove-me"] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByText("remove-me")).toBeInTheDocument();

      fireEvent.click(
        screen.getByRole("button", { name: /remove tag remove-me/i })
      );

      expect(screen.queryByText("remove-me")).not.toBeInTheDocument();
    });

    it("shows error when adding a duplicate tag", () => {
      const session = makeSession({ tags: ["existing"] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      const tagInput = screen.getByPlaceholderText("Add a tag...");
      fireEvent.change(tagInput, { target: { value: "existing" } });
      fireEvent.click(screen.getByRole("button", { name: /^add$/i }));

      expect(screen.getByText(/tag already exists/i)).toBeInTheDocument();
    });

    it("shows No tags when session has no tags", () => {
      const session = makeSession({ tags: [] });

      render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByText(/no tags/i)).toBeInTheDocument();
    });
  });

  describe("overlay click", () => {
    it("calls onCancel when clicking the overlay outside the modal", () => {
      const onCancel = jest.fn();
      const session = makeSession();

      const { container } = render(
        <ResumeSessionModal
          session={session}
          sessions={[session]}
          onConfirm={jest.fn()}
          onCancel={onCancel}
        />
      );

      // The overlay is the outermost div; its onClick calls onCancel directly.
      const overlay = container.firstChild as HTMLElement;
      fireEvent.click(overlay);

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });
});
