/**
 * Tests for NewShellDialog component.
 *
 * Covers:
 *  - All form fields (name, command, workingDir) render
 *  - Submit calls onSubmit with correct params when form is filled
 *  - Submit button is disabled while async call is in progress (isSubmitting guard)
 *  - onCancel called when Escape key is pressed
 *  - onCancel called when backdrop (overlay) is clicked
 *  - Error message shown when onSubmit rejects
 *  - Escape key scoping: an Escape that only dismisses the workingDir field's
 *    RepoPathInput dropdown must not also close the whole dialog (Task 1.1.1d)
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { NewShellDialog } from "../NewShellDialog";
import { addRecentShellCommand } from "@/lib/omnibar/recentShellCommands";
import { useSessionRepoPaths } from "@/lib/hooks/useSessionRepoPaths";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// RepoPathInput (used for the workingDir field) uses useSessionRepoPaths (Redux)
// and usePathCompletions (RPC) internally. Stub both hooks — rather than mocking
// RepoPathInput itself — so the dialog tests don't need a Redux store or
// ConnectRPC transport, while still exercising the real component's Escape/dropdown
// behavior for Task 1.1.1d's tests below.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: jest.fn(() => []),
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

const mockUseSessionRepoPaths = useSessionRepoPaths as jest.Mock;

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("NewShellDialog", () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockUseSessionRepoPaths.mockReturnValue([]);
  });

  describe("renders_all_form_fields", () => {
    it("NewShellDialog_should_renderNameCommandWorkingDirFields_When_mounted", () => {
      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/command/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/working directory/i)).toBeInTheDocument();
    });
  });

  describe("submits_with_correct_params_When_formFilled", () => {
    it("NewShellDialog_should_callOnSubmitWithCorrectArgs_When_formFilled", async () => {
      const onSubmit = jest.fn().mockResolvedValue(undefined);

      render(
        <NewShellDialog
          onSubmit={onSubmit}
          onCancel={jest.fn()}
        />
      );

      fireEvent.change(screen.getByLabelText(/name/i), {
        target: { value: "my-shell" },
      });
      fireEvent.change(screen.getByLabelText(/command/i), {
        target: { value: "bash" },
      });

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: /spawn shell/i }));
      });

      expect(onSubmit).toHaveBeenCalledTimes(1);
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "my-shell",
          command: "bash",
        })
      );
    });
  });

  describe("submit_disabled_When_spawning", () => {
    it("NewShellDialog_should_disableSubmitButton_When_submitting", async () => {
      // onSubmit never resolves so isSubmitting stays true
      const onSubmit = jest.fn(() => new Promise<void>(() => {}));

      render(
        <NewShellDialog
          onSubmit={onSubmit}
          onCancel={jest.fn()}
        />
      );

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: /spawn shell/i }));
      });

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /spawning\.\.\./i })).toBeDisabled();
      });
    });
  });

  describe("closes_When_escapePressed", () => {
    it("NewShellDialog_should_callOnCancel_When_escapeKeyPressed", () => {
      const onCancel = jest.fn();

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={onCancel}
        />
      );

      fireEvent.keyDown(document, { key: "Escape", code: "Escape" });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });

  describe("closes_When_backdropClicked", () => {
    it("NewShellDialog_should_callOnCancel_When_overlayClicked", () => {
      const onCancel = jest.fn();

      const { container } = render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={onCancel}
        />
      );

      // The dialog is rendered via createPortal into document.body.
      // Query the overlay element directly by role since container may be empty.
      const overlay = document.querySelector("[role='dialog']") as HTMLElement;
      expect(overlay).not.toBeNull();

      // Simulate click on the overlay backdrop (target === currentTarget path).
      // We use the parent of the inner dialog div — the overlay element itself.
      fireEvent.click(overlay, { target: overlay });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });

  describe("recent_command_suggestions", () => {
    it("NewShellDialog_should_showRecentCommandChips_When_historyExists", () => {
      addRecentShellCommand("npm run dev");
      addRecentShellCommand("go test ./...");

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={jest.fn()}
        />
      );

      expect(screen.getByText("npm run dev")).toBeInTheDocument();
      expect(screen.getByText("go test ./...")).toBeInTheDocument();
    });

    it("NewShellDialog_should_fillCommandField_When_suggestionClicked", () => {
      addRecentShellCommand("npm run dev");

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={jest.fn()}
        />
      );

      fireEvent.click(screen.getByText("npm run dev"));

      expect(screen.getByLabelText(/command/i)).toHaveValue("npm run dev");
    });

    it("NewShellDialog_should_hideSuggestions_When_commandFieldNonEmpty", () => {
      addRecentShellCommand("npm run dev");

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={jest.fn()}
        />
      );

      fireEvent.change(screen.getByLabelText(/command/i), {
        target: { value: "bash" },
      });

      expect(screen.queryByText("npm run dev")).not.toBeInTheDocument();
    });

    it("NewShellDialog_should_persistCommandToHistory_When_submitted", async () => {
      const onSubmit = jest.fn().mockResolvedValue(undefined);

      render(
        <NewShellDialog
          onSubmit={onSubmit}
          onCancel={jest.fn()}
        />
      );

      fireEvent.change(screen.getByLabelText(/command/i), {
        target: { value: "npm run dev" },
      });

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: /spawn shell/i }));
      });

      const { getRecentShellCommands } = jest.requireActual("@/lib/omnibar/recentShellCommands");
      expect(getRecentShellCommands()).toEqual(["npm run dev"]);
    });
  });

  describe("shows_error_When_spawnFails", () => {
    it("NewShellDialog_should_showErrorMessage_When_onSubmitRejects", async () => {
      const onSubmit = jest.fn().mockRejectedValue(new Error("port already in use"));

      render(
        <NewShellDialog
          onSubmit={onSubmit}
          onCancel={jest.fn()}
        />
      );

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: /spawn shell/i }));
      });

      await waitFor(() => {
        expect(screen.getByText(/port already in use/i)).toBeInTheDocument();
      });
    });
  });

  describe("NewShellDialog — Escape key scoping", () => {
    it("NewShellDialog_should_closeOnlyTheWorkingDirDropdown_When_escapePressedWhileDropdownOpen", () => {
      mockUseSessionRepoPaths.mockReturnValue(["/home/user/project-a"]);
      const onCancel = jest.fn();

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={onCancel}
        />
      );

      const workingDirInput = screen.getByLabelText(/working directory/i);
      fireEvent.focus(workingDirInput);
      expect(screen.getByRole("listbox")).toBeInTheDocument();

      fireEvent.keyDown(workingDirInput, { key: "Escape", code: "Escape" });

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
      expect(onCancel).not.toHaveBeenCalled();
    });

    it("NewShellDialog_should_callOnCancel_When_escapePressedWithNoDropdownOpen", () => {
      mockUseSessionRepoPaths.mockReturnValue(["/home/user/project-a"]);
      const onCancel = jest.fn();

      render(
        <NewShellDialog
          onSubmit={jest.fn().mockResolvedValue(undefined)}
          onCancel={onCancel}
        />
      );

      // The workingDir field's dropdown was never opened (never focused).
      fireEvent.keyDown(document, { key: "Escape", code: "Escape" });

      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });
});
