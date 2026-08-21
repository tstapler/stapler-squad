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
});
