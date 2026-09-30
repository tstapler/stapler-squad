import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { CommitPushModal } from "./CommitPushModal";

const quickCommitPush = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    quickCommitPush: (...args: unknown[]) => quickCommitPush(...args),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

describe("CommitPushModal", () => {
  beforeEach(() => {
    quickCommitPush.mockReset();
  });

  it("CommitPushModal_should_wrapFocusToTextarea_When_TabPressedOnCommitButton", () => {
    render(<CommitPushModal repoPath="/repo" branch="main" onClose={jest.fn()} />);

    const textarea = screen.getByLabelText("Commit message");
    const commitButton = screen.getByRole("button", { name: /stage all, commit, and push/i });

    // Commit & Push starts disabled (empty message) — type to enable it, making
    // it the last focusable element in the trap.
    fireEvent.change(textarea, { target: { value: "fix: something" } });
    expect(commitButton).toBeEnabled();

    commitButton.focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(textarea);

    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(commitButton);
  });

  it("CommitPushModal_should_submitOnCtrlEnter_And_closeOnEscape_When_ExistingHandlersInvoked", async () => {
    quickCommitPush.mockResolvedValue({ success: true });
    const onClose = jest.fn();

    render(<CommitPushModal repoPath="/repo" branch="main" onClose={onClose} />);

    const textarea = screen.getByLabelText("Commit message");
    fireEvent.change(textarea, { target: { value: "fix: something" } });

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(quickCommitPush).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
