/**
 * Focus-restoration regression test for SessionActionsOverflow (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike SessionActionsOverflow.test.tsx)
 * so the real trap-and-restore behavior runs end to end. Every dialog and the
 * overflow menu itself restore focus to the persistent "···" button
 * (overflowButtonRef), never a transient menu-item button that unmounts when
 * the menu closes.
 *
 * The Program Picker case is a regression test: that dialog previously had no
 * useFocusTrap wiring at all, so closing it via Cancel left focus stranded.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SessionActionsOverflow } from "../SessionActionsOverflow";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus, InstanceType } from "@/gen/session/v1/types_pb";

jest.mock("../SessionActionsOverflow.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);
jest.mock("../TagEditor.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

function makeSession(overrides: Partial<Record<string, unknown>> = {}): Session {
  return {
    id: "session-1",
    title: "Test Session",
    tags: [] as string[],
    status: SessionStatus.RUNNING,
    instanceType: InstanceType.MANAGED,
    rateLimitEnabled: false,
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

function getOverflowButton() {
  return screen.getByRole("button", { name: /more session actions/i });
}

function openMenu() {
  const toggle = getOverflowButton();
  fireEvent.click(toggle);
  return toggle;
}

describe("SessionActionsOverflow focus restoration", () => {
  it("SessionActionsOverflow_should_restoreFocusToOverflowButton_When_menuClosedViaSecondClick", async () => {
    render(<SessionActionsOverflow session={makeSession()} onDelete={jest.fn()} />);
    const toggle = openMenu();
    await waitFor(() => expect(screen.getByRole("menu")).not.toBeNull());

    fireEvent.click(toggle);

    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it("SessionActionsOverflow_should_restoreFocusToOverflowButton_When_deleteDialogCancelled", async () => {
    render(<SessionActionsOverflow session={makeSession()} onDelete={jest.fn()} />);
    const toggle = openMenu();
    await waitFor(() => expect(screen.getByRole("menu")).not.toBeNull());

    fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /delete session/i })).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it("SessionActionsOverflow_should_restoreFocusToOverflowButton_When_restartDialogCancelled", async () => {
    render(
      <SessionActionsOverflow session={makeSession()} onRestart={jest.fn().mockResolvedValue(true)} />
    );
    const toggle = openMenu();
    await waitFor(() => expect(screen.getByRole("menu")).not.toBeNull());

    fireEvent.click(screen.getByRole("menuitem", { name: /restart session/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /restart session/i })).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it("SessionActionsOverflow_should_restoreFocusToOverflowButton_When_programPickerCancelled", async () => {
    render(<SessionActionsOverflow session={makeSession()} onChangeProgram={jest.fn()} />);
    const toggle = openMenu();
    await waitFor(() => expect(screen.getByRole("menu")).not.toBeNull());

    fireEvent.click(screen.getByRole("menuitem", { name: /change program/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /change program/i })).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });
});
