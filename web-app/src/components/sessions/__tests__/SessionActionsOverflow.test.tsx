/**
 * Tests for SessionActionsOverflow component.
 *
 * Covers:
 *  - Renders without crashing with minimal props
 *  - ··· button present and toggles menu open/closed
 *  - Conditional menu items: only shown when prop is provided
 *  - Delete: confirmation dialog shown before calling onDelete
 *  - Restart: confirmation dialog shown before calling onRestart
 *  - onClearConversationState called when Clear Conversation clicked
 *  - Primary action button shown when showPrimaryAction=true and status is PAUSED
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { SessionActionsOverflow } from "../SessionActionsOverflow";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus, InstanceType } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => undefined,
}));

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    draftPullRequest: jest.fn(),
    createPullRequest: jest.fn(),
  }),
}));

// CreatePullRequestModal has its own dedicated test suite (CreatePullRequestModal.test.tsx) —
// stub it here so these tests verify wiring (trigger -> open/close) without duplicating that
// coverage, matching the same pattern used in ReviewQueuePanel.test.tsx.
jest.mock("../CreatePullRequestModal", () => ({
  CreatePullRequestModal: ({
    session,
    isOpen,
    onClose,
  }: {
    session: { id: string };
    isOpen: boolean;
    onClose: () => void;
  }) =>
    isOpen ? (
      <div data-testid="create-pr-modal" data-session-id={session.id}>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(overrides: Partial<Record<string, unknown>> = {}): Session {
  return {
    id: "session-1",
    title: "Test Session",
    tags: [] as string[],
    status: SessionStatus.RUNNING,
    instanceType: InstanceType.MANAGED,
    rateLimitEnabled: false,
    ...overrides,
  } as unknown as Session;
}

function openMenu() {
  const toggle = screen.getByRole("button", { name: /more session actions/i });
  fireEvent.click(toggle);
}

// ---------------------------------------------------------------------------
// Render helper
// ---------------------------------------------------------------------------

function renderOverflow(props: Partial<React.ComponentProps<typeof SessionActionsOverflow>> = {}) {
  const session = props.session ?? makeSession();
  return render(<SessionActionsOverflow session={session} {...props} />);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SessionActionsOverflow", () => {
  describe("rendering", () => {
    it("renders ··· toggle button", () => {
      renderOverflow();
      expect(screen.getByRole("button", { name: /more session actions/i })).toBeInTheDocument();
    });

    it("does not show menu before toggle is clicked", () => {
      renderOverflow({ onClone: jest.fn() });
      expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    });

    it("shows menu after toggle clicked", () => {
      renderOverflow();
      openMenu();
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    it("renders menu via portal into document.body, not inside the component container", () => {
      // Regression guard: menu must escape the card's stacking context so it
      // isn't hidden behind sibling cards lower in the DOM.
      const { container } = renderOverflow({ onClone: jest.fn() });
      openMenu();
      const menu = screen.getByRole("menu");
      expect(document.body).toContainElement(menu);
      expect(container).not.toContainElement(menu);
    });

    it("closes menu on second toggle click", () => {
      renderOverflow();
      openMenu();
      fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
      expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    });
  });

  describe("conditional menu items", () => {
    it("shows Clone when onClone provided", () => {
      renderOverflow({ onClone: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /clone/i })).toBeInTheDocument();
    });

    it("omits Clone when onClone not provided", () => {
      renderOverflow();
      openMenu();
      expect(screen.queryByRole("menuitem", { name: /clone/i })).not.toBeInTheDocument();
    });

    it("shows Clear Conversation when onClearConversationState provided", () => {
      renderOverflow({ onClearConversationState: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /clear conversation/i })).toBeInTheDocument();
    });

    it("omits Clear Conversation when prop not provided", () => {
      renderOverflow();
      openMenu();
      expect(screen.queryByRole("menuitem", { name: /clear conversation/i })).not.toBeInTheDocument();
    });

    it("shows Rename when onRenameRequest provided", () => {
      renderOverflow({ onRenameRequest: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /rename/i })).toBeInTheDocument();
    });
  });

  describe("delete flow", () => {
    it("shows delete confirmation dialog when Delete clicked", () => {
      renderOverflow({ onDelete: jest.fn() });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));
      expect(screen.getByRole("dialog", { name: /delete session/i })).toBeInTheDocument();
    });

    it("calls onDelete when confirmed in dialog", async () => {
      const onDelete = jest.fn().mockResolvedValue(undefined);
      renderOverflow({ onDelete });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));
      fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
      await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1));
    });

    it("does not call onDelete when dialog cancelled", () => {
      const onDelete = jest.fn();
      renderOverflow({ onDelete });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));
      fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
      expect(onDelete).not.toHaveBeenCalled();
    });
  });

  describe("restart flow", () => {
    it("shows restart confirmation dialog when Restart clicked", () => {
      renderOverflow({ onRestart: jest.fn() });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /restart/i }));
      expect(screen.getByRole("dialog", { name: /restart session/i })).toBeInTheDocument();
    });

    it("calls onRestart when confirmed", async () => {
      const onRestart = jest.fn().mockResolvedValue(true);
      renderOverflow({ onRestart });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /restart/i }));
      fireEvent.click(screen.getByRole("button", { name: /^restart$/i }));
      await waitFor(() => expect(onRestart).toHaveBeenCalledWith("session-1"));
    });
  });

  describe("retry now flow (AC6)", () => {
    it("omits Retry now menu item for a running session with no pending retry", () => {
      renderOverflow({ onRetryNow: jest.fn() });
      openMenu();
      expect(screen.queryByRole("menuitem", { name: /retry.*now/i })).not.toBeInTheDocument();
    });

    it("shows Retry now menu item when session is PERMANENTLY_FAILED", () => {
      const session = makeSession({ status: SessionStatus.PERMANENTLY_FAILED });
      renderOverflow({ session, onRetryNow: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /retry.*now/i })).toBeInTheDocument();
    });

    it("shows Retry now menu item mid-backoff-wait (nextRetryAt set, not yet permanently failed)", () => {
      const session = makeSession({ nextRetryAt: { seconds: 1n, nanos: 0 } });
      renderOverflow({ session, onRetryNow: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /retry.*now/i })).toBeInTheDocument();
    });

    it("shows primary Retry now button when showPrimaryAction=true and session is PERMANENTLY_FAILED", () => {
      const session = makeSession({ status: SessionStatus.PERMANENTLY_FAILED });
      renderOverflow({ session, showPrimaryAction: true, onRetryNow: jest.fn() });
      expect(screen.getByRole("button", { name: /retry permanently-failed session/i })).toBeInTheDocument();
    });

    it("shows retry confirmation dialog when Retry now clicked", () => {
      const session = makeSession({ status: SessionStatus.PERMANENTLY_FAILED });
      renderOverflow({ session, onRetryNow: jest.fn() });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /retry.*now/i }));
      expect(screen.getByRole("dialog", { name: /retry session/i })).toBeInTheDocument();
    });

    it("calls onRetryNow with session id when confirmed, including from PERMANENTLY_FAILED", async () => {
      const session = makeSession({ status: SessionStatus.PERMANENTLY_FAILED });
      const onRetryNow = jest.fn().mockResolvedValue(true);
      renderOverflow({ session, onRetryNow });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /retry.*now/i }));
      fireEvent.click(screen.getByRole("button", { name: /^retry now$/i }));
      await waitFor(() => expect(onRetryNow).toHaveBeenCalledWith("session-1"));
    });

    it("surfaces an error and keeps the dialog open when onRetryNow resolves false", async () => {
      const session = makeSession({ status: SessionStatus.PERMANENTLY_FAILED });
      const onRetryNow = jest.fn().mockResolvedValue(false);
      renderOverflow({ session, onRetryNow });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /retry.*now/i }));
      fireEvent.click(screen.getByRole("button", { name: /^retry now$/i }));
      await waitFor(() => expect(screen.getByText(/failed to retry session/i)).toBeInTheDocument());
      expect(screen.getByRole("dialog", { name: /retry session/i })).toBeInTheDocument();
    });
  });

  describe("clear conversation", () => {
    it("calls onClearConversationState with session id when clicked", () => {
      const onClear = jest.fn().mockResolvedValue(true);
      renderOverflow({ onClearConversationState: onClear });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /clear conversation/i }));
      expect(onClear).toHaveBeenCalledWith("session-1");
    });
  });

  describe("primary action button", () => {
    it("shows Resume button when showPrimaryAction=true and session is PAUSED", () => {
      const session = makeSession({ status: SessionStatus.PAUSED });
      renderOverflow({ session, showPrimaryAction: true, onResume: jest.fn() });
      expect(screen.getByRole("button", { name: /resume session/i })).toBeInTheDocument();
    });

    it("shows Pause button when showPrimaryAction=true and session is RUNNING", () => {
      const session = makeSession({ status: SessionStatus.RUNNING });
      renderOverflow({ session, showPrimaryAction: true, onPause: jest.fn() });
      expect(screen.getByRole("button", { name: /pause session/i })).toBeInTheDocument();
    });
  });

  describe("program picker", () => {
    function openProgramPicker(overrides: Partial<React.ComponentProps<typeof SessionActionsOverflow>> = {}) {
      const onChangeProgram = overrides.onChangeProgram ?? jest.fn().mockResolvedValue(undefined);
      const utils = renderOverflow({ onChangeProgram, ...overrides });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /change program/i }));
      return { onChangeProgram, ...utils };
    }

    it("shows the Change Program menu item when onChangeProgram is provided", () => {
      renderOverflow({ onChangeProgram: jest.fn() });
      openMenu();
      expect(screen.getByRole("menuitem", { name: /change program/i })).toBeInTheDocument();
    });

    it("omits the Change Program menu item when onChangeProgram is not provided", () => {
      renderOverflow();
      openMenu();
      expect(screen.queryByRole("menuitem", { name: /change program/i })).not.toBeInTheDocument();
    });

    it("pre-fills the picker with the session's current program", () => {
      const session = makeSession({ program: "aider" });
      openProgramPicker({ session });
      expect(screen.getByRole("dialog", { name: /change program/i })).toBeInTheDocument();
      expect(screen.getByRole("combobox")).toHaveValue("aider");
    });

    it("calls onChangeProgram with the session id and picked value when saved (non-Active session)", async () => {
      const session = makeSession({ status: SessionStatus.PAUSED, program: "claude" });
      const { onChangeProgram } = openProgramPicker({ session });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

      await waitFor(() => expect(onChangeProgram).toHaveBeenCalledWith("session-1", "aider"));
    });

    it("sends an empty string when System default is selected", async () => {
      const session = makeSession({ status: SessionStatus.PAUSED, program: "claude" });
      const { onChangeProgram } = openProgramPicker({ session });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

      await waitFor(() => expect(onChangeProgram).toHaveBeenCalledWith("session-1", ""));
    });

    it("shows the restart hint only when the session is Active", () => {
      const activeSession = makeSession({ status: SessionStatus.ACTIVE });
      openProgramPicker({ session: activeSession });
      expect(screen.getByText(/the session will restart/i)).toBeInTheDocument();
    });

    it("omits the restart hint when the session is not Active", () => {
      const pausedSession = makeSession({ status: SessionStatus.PAUSED });
      openProgramPicker({ session: pausedSession });
      expect(screen.queryByText(/the session will restart/i)).not.toBeInTheDocument();
    });

    it("shows a confirmation dialog instead of saving immediately on an Active session", () => {
      const session = makeSession({ status: SessionStatus.ACTIVE });
      const { onChangeProgram } = openProgramPicker({ session });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

      expect(screen.getByRole("dialog", { name: /change program/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /change & restart/i })).toBeInTheDocument();
      expect(onChangeProgram).not.toHaveBeenCalled();
    });

    it("does not call onChangeProgram when the confirmation is cancelled", () => {
      const session = makeSession({ status: SessionStatus.ACTIVE });
      const { onChangeProgram } = openProgramPicker({ session });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
      fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

      expect(onChangeProgram).not.toHaveBeenCalled();
    });

    it("calls onChangeProgram once the restart confirmation is accepted", async () => {
      const session = makeSession({ status: SessionStatus.ACTIVE });
      const { onChangeProgram } = openProgramPicker({ session });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
      fireEvent.click(screen.getByRole("button", { name: /change & restart/i }));

      await waitFor(() => expect(onChangeProgram).toHaveBeenCalledWith("session-1", "aider"));
    });

    it("keeps the dialog open and shows an inline error when the save fails (non-Active session)", async () => {
      const session = makeSession({ status: SessionStatus.PAUSED, program: "claude" });
      const onChangeProgram = jest.fn().mockRejectedValue(new Error("network down"));
      openProgramPicker({ session, onChangeProgram });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

      await waitFor(() => expect(screen.getByText("network down")).toBeInTheDocument());
      expect(screen.getByRole("dialog", { name: /change program/i })).toBeInTheDocument();
    });

    it("keeps the restart-confirm dialog open and shows an inline error when the save fails (Active session)", async () => {
      const session = makeSession({ status: SessionStatus.ACTIVE });
      const onChangeProgram = jest.fn().mockRejectedValue(new Error("network down"));
      openProgramPicker({ session, onChangeProgram });

      fireEvent.change(screen.getByRole("combobox"), { target: { value: "aider" } });
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
      fireEvent.click(screen.getByRole("button", { name: /change & restart/i }));

      await waitFor(() => expect(screen.getByText("network down")).toBeInTheDocument());
      expect(screen.getByRole("button", { name: /change & restart/i })).toBeInTheDocument();
    });

    it("re-syncs the picker value when the session's program changes externally while open", () => {
      const session = makeSession({ status: SessionStatus.PAUSED, program: "claude" });
      const { rerender } = openProgramPicker({ session });

      expect(screen.getByRole("combobox")).toHaveValue("claude");

      const updatedSession = makeSession({ status: SessionStatus.PAUSED, program: "agy" });
      rerender(<SessionActionsOverflow session={updatedSession} onChangeProgram={jest.fn()} />);

      expect(screen.getByRole("combobox")).toHaveValue("agy");
    });
  });

  describe("Create PR trigger", () => {
    it("disables the Create PR trigger (with tooltip) when the session has no commits ahead (State B)", () => {
      const session = makeSession({ hasCommitsAhead: false, githubPrUrl: "" });
      renderOverflow({ session });
      openMenu();

      const trigger = screen.getByTestId(`create-pr-trigger-${session.id}`);
      expect(trigger).toBeDisabled();
      expect(trigger).toHaveAttribute("title", "No commits ahead of main yet");
    });

    it("enables the Create PR trigger when the session has commits ahead (State A)", () => {
      const session = makeSession({ hasCommitsAhead: true, githubPrUrl: "" });
      renderOverflow({ session });
      openMenu();

      const trigger = screen.getByTestId(`create-pr-trigger-${session.id}`);
      expect(trigger).not.toBeDisabled();
    });

    it("shows a View PR link instead of the trigger when the session already has a githubPrUrl (State C)", () => {
      const session = makeSession({
        githubPrUrl: "https://github.com/org/repo/pull/99",
        githubPrNumber: 99,
      });
      renderOverflow({ session });
      openMenu();

      expect(screen.queryByTestId(`create-pr-trigger-${session.id}`)).not.toBeInTheDocument();
      const link = screen.getByTestId("github-pr-link");
      expect(link).toHaveAttribute("href", "https://github.com/org/repo/pull/99");
      expect(link).toHaveTextContent("#99");
    });

    it("opens the shared CreatePullRequestModal for the session when the enabled trigger is clicked", () => {
      const session = makeSession({ hasCommitsAhead: true, githubPrUrl: "" });
      renderOverflow({ session });
      openMenu();

      fireEvent.click(screen.getByTestId(`create-pr-trigger-${session.id}`));

      const modal = screen.getByTestId("create-pr-modal");
      expect(modal).toBeInTheDocument();
      expect(modal).toHaveAttribute("data-session-id", session.id);
    });

    it("closes the modal when the modal's onClose fires", () => {
      const session = makeSession({ hasCommitsAhead: true, githubPrUrl: "" });
      renderOverflow({ session });
      openMenu();

      fireEvent.click(screen.getByTestId(`create-pr-trigger-${session.id}`));
      fireEvent.click(screen.getByRole("button", { name: /close/i }));

      expect(screen.queryByTestId("create-pr-modal")).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // Give Direction / steer dialog (pr-fix-steering Story 1.1.3): the dialog
  // must check the steer RPC's result instead of unconditionally closing —
  // see plan.md's Task 1.1.3c.
  // -------------------------------------------------------------------------
  describe("give direction (steer) dialog", () => {
    function openSteerDialog(onSteerAutonomousSession: jest.Mock) {
      const session = makeSession({ autonomousMode: true });
      renderOverflow({ session, onSteerAutonomousSession });
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: /give direction/i }));
      const input = screen.getByPlaceholderText(/focus on the ui tests first/i);
      fireEvent.change(input, { target: { value: "fix the bug" } });
      return input;
    }

    it("keeps the dialog open and preserves the message when the steer call resolves false", async () => {
      const onSteerAutonomousSession = jest.fn().mockResolvedValue(false);
      const input = openSteerDialog(onSteerAutonomousSession);

      fireEvent.keyDown(input, { key: "Enter" });
      await waitFor(() => expect(onSteerAutonomousSession).toHaveBeenCalledWith("session-1", "fix the bug"));

      expect(screen.getByRole("dialog", { name: /give direction/i })).toBeInTheDocument();
      expect(screen.getByDisplayValue("fix the bug")).toBeInTheDocument();
    });

    it("closes the dialog and clears the message when the steer call resolves true", async () => {
      const onSteerAutonomousSession = jest.fn().mockResolvedValue(true);
      const input = openSteerDialog(onSteerAutonomousSession);

      fireEvent.keyDown(input, { key: "Enter" });
      await waitFor(() =>
        expect(screen.queryByRole("dialog", { name: /give direction/i })).not.toBeInTheDocument()
      );
    });

    it("disables the Send button and input while the steer call is pending, and ignores a second click", async () => {
      let resolveSteer: (ok: boolean) => void = () => {};
      const pending = new Promise<boolean>((resolve) => {
        resolveSteer = resolve;
      });
      const onSteerAutonomousSession = jest.fn().mockReturnValue(pending);
      openSteerDialog(onSteerAutonomousSession);

      const sendButton = screen.getByRole("button", { name: /send/i });
      fireEvent.click(sendButton);

      await waitFor(() => expect(sendButton).toBeDisabled());
      expect(screen.getByPlaceholderText(/focus on the ui tests first/i)).toBeDisabled();

      // A second click while the RPC is in flight must not fire a duplicate call.
      fireEvent.click(sendButton);
      expect(onSteerAutonomousSession).toHaveBeenCalledTimes(1);

      await act(async () => {
        resolveSteer(true);
        await pending;
      });

      await waitFor(() =>
        expect(screen.queryByRole("dialog", { name: /give direction/i })).not.toBeInTheDocument()
      );
    });

    it("keeps Cancel enabled and Escape working while the steer call is pending", async () => {
      let resolveSteer: (ok: boolean) => void = () => {};
      const pending = new Promise<boolean>((resolve) => {
        resolveSteer = resolve;
      });
      const onSteerAutonomousSession = jest.fn().mockReturnValue(pending);
      const input = openSteerDialog(onSteerAutonomousSession);

      fireEvent.keyDown(input, { key: "Enter" });
      await waitFor(() => expect(onSteerAutonomousSession).toHaveBeenCalledTimes(1));

      const cancelButton = screen.getByRole("button", { name: /^cancel$/i });
      expect(cancelButton).not.toBeDisabled();

      const dialog = screen.getByRole("dialog", { name: /give direction/i });
      fireEvent.keyDown(dialog, { key: "Escape" });
      expect(screen.queryByRole("dialog", { name: /give direction/i })).not.toBeInTheDocument();

      // Avoid an unhandled-rejection/act warning from the now-orphaned promise.
      resolveSteer(true);
      await act(async () => {
        await pending;
      });
    });
  });
});
