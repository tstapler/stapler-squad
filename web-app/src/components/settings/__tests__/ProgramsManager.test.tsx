import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { ProgramsManager } from "../ProgramsManager";
import {
  mockProbeCheck,
  probeStates,
  resetProbeMocks,
  setProbeHookState,
} from "@/lib/hooks/__mocks__/probeProgramMock";
import { NOT_FOUND_TEXT } from "@/components/ui/ProbeStatusBadge";
import type { ProbeUiState } from "@/lib/hooks/useProbeProgram";

jest.mock("@/lib/hooks/useProbeProgram", () => require("@/lib/hooks/__mocks__/probeProgramMock").hookMockFactory());

const mockListProgramsConfig = jest.fn();
const mockUpsertProgramConfig = jest.fn();
const mockDeleteProgramConfig = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listProgramsConfig: mockListProgramsConfig,
    upsertProgramConfig: mockUpsertProgramConfig,
    deleteProgramConfig: mockDeleteProgramConfig,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

describe("ProgramsManager", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    resetProbeMocks();
    mockListProgramsConfig.mockResolvedValue({
      programs: [
        {
          id: "claude",
          label: "Claude Code",
          command: "claude",
          description: "Anthropic CLI",
          isBuiltin: true,
          cliFlags: "",
          env: {},
        },
        {
          id: "custom-script",
          label: "Custom Script",
          command: "/usr/local/bin/script",
          description: "Custom python runner",
          isBuiltin: false,
          cliFlags: "--flag",
          env: { KEY: "VAL" },
        },
      ],
    });
  });

  it("renders list of built-in and custom programs", async () => {
    render(<ProgramsManager />);

    await waitFor(() => {
      expect(screen.getByTestId("program-row-claude")).toBeInTheDocument();
      expect(screen.getByTestId("program-row-custom-script")).toBeInTheDocument();
    });

    expect(screen.getByText("Claude Code")).toBeInTheDocument();
    expect(screen.getByText("Built-in")).toBeInTheDocument();
    expect(screen.getByText("Custom Script")).toBeInTheDocument();
    expect(screen.getByText("Custom")).toBeInTheDocument();
  });

  it("opens create form and submits valid custom program", async () => {
    mockUpsertProgramConfig.mockResolvedValue({});
    render(<ProgramsManager />);

    await waitFor(() => {
      expect(screen.getByTestId("add-program-btn")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId("add-program-btn"));
    expect(screen.getByTestId("program-form")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("prog-id-input"), { target: { value: "new-agent" } });
    fireEvent.change(screen.getByTestId("prog-label-input"), { target: { value: "New Agent" } });
    fireEvent.change(screen.getByTestId("prog-command-input"), { target: { value: "new-agent-bin" } });

    fireEvent.click(screen.getByTestId("save-program-btn"));

    await waitFor(() => {
      expect(mockUpsertProgramConfig).toHaveBeenCalledWith({
        program: {
          id: "new-agent",
          label: "New Agent",
          command: "new-agent-bin",
          cliFlags: "",
          description: "",
          env: {},
          isBuiltin: false,
        },
      });
    });
  });

  it("prevents overriding built-in program ID", async () => {
    render(<ProgramsManager />);

    await waitFor(() => {
      expect(screen.getByTestId("add-program-btn")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId("add-program-btn"));
    fireEvent.change(screen.getByTestId("prog-id-input"), { target: { value: "claude" } });
    fireEvent.change(screen.getByTestId("prog-label-input"), { target: { value: "Override Claude" } });
    fireEvent.change(screen.getByTestId("prog-command-input"), { target: { value: "claude" } });

    fireEvent.click(screen.getByTestId("save-program-btn"));

    await waitFor(() => {
      expect(screen.getByText("Cannot override a built-in program ID.")).toBeInTheDocument();
    });
    expect(mockUpsertProgramConfig).not.toHaveBeenCalled();
  });

  it("deletes custom program after confirmation", async () => {
    mockDeleteProgramConfig.mockResolvedValue({});
    render(<ProgramsManager />);

    await waitFor(() => {
      expect(screen.getByTestId("delete-program-custom-script")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId("delete-program-custom-script"));
    expect(screen.getByTestId("confirm-delete-program-custom-script")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("confirm-delete-program-custom-script"));

    await waitFor(() => {
      expect(mockDeleteProgramConfig).toHaveBeenCalledWith({ id: "custom-script" });
    });
  });
});

describe("ProgramsManager probe badge", () => {
  beforeEach(() => {
    resetProbeMocks();
    mockUpsertProgramConfig.mockReset();
  });

  async function openForm(state: ProbeUiState = probeStates.idle) {
    mockListProgramsConfig.mockResolvedValue({ programs: [] });
    setProbeHookState(state);
    const utils = render(<ProgramsManager />);
    await waitFor(() => expect(screen.getByTestId("add-program-btn")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("add-program-btn"));
    return utils;
  }
  const command = () => screen.getByTestId("prog-command-input");
  const check = () => screen.getByTestId("prog-command-check");

  it("ProgramsManager_should_ShowFoundStatus_When_CommandBlurs", async () => {
    await openForm(probeStates.found);
    fireEvent.blur(command());
    expect(mockProbeCheck).toHaveBeenCalledWith();
    expect(screen.getByTestId("prog-command-status")).toHaveTextContent("Found: /usr/bin/claude");
  });

  it("ProgramsManager_should_RunImplicitProbeAndNotSubmit_When_EnterPressedInCommandField", async () => {
    await openForm(probeStates.needsConfirm);
    fireEvent.keyDown(command(), { key: "Enter" });
    expect(mockProbeCheck).toHaveBeenCalledTimes(1);
    expect(mockProbeCheck).toHaveBeenCalledWith({ immediate: true });
    expect(mockUpsertProgramConfig).not.toHaveBeenCalled();
    expect(screen.getByTestId("prog-command-status")).toHaveTextContent("Not checked for flags yet");
  });

  it.each(["notFound", "transportError"])("ProgramsManager_should_KeepSaveEnabled_When_%s", async (name) => {
    mockUpsertProgramConfig.mockResolvedValue({});
    await openForm(probeStates[name]);
    expect(screen.getByTestId("save-program-btn")).toBeEnabled();
    fireEvent.change(screen.getByTestId("prog-id-input"), { target: { value: "x-agent" } });
    fireEvent.change(screen.getByTestId("prog-label-input"), { target: { value: "X" } });
    fireEvent.change(command(), { target: { value: "claudee" } });
    fireEvent.click(screen.getByTestId("save-program-btn"));
    await waitFor(() => expect(mockUpsertProgramConfig).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("ProgramsManager_should_LinkStatusViaDescribedByAndNeverSetInvalid_When_ProbeSettles", async () => {
    await openForm(probeStates.notFound);
    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveTextContent(NOT_FOUND_TEXT);
    expect(command()).toHaveAttribute("aria-describedby", status.id);
    expect(command()).not.toHaveAttribute("aria-invalid");
  });

  it("ProgramsManager_should_KeepActiveElementAndScroll_When_ProbeSettles", async () => {
    const scroll = jest.fn();
    window.HTMLElement.prototype.scrollIntoView = scroll;
    const { rerender } = await openForm();
    check().focus();
    setProbeHookState(probeStates.checking);
    rerender(<ProgramsManager />);
    setProbeHookState(probeStates.found);
    rerender(<ProgramsManager />);
    expect(document.activeElement).toBe(check());
    expect(scroll).not.toHaveBeenCalled();
  });

  it("ProgramsManager_should_DisableAutoCapAutoCorrectSpellcheck_When_CommandAndFlagsInputs", async () => {
    await openForm();
    for (const id of ["prog-command-input", "prog-flags-input"]) {
      const el = screen.getByTestId(id);
      expect(el).toHaveAttribute("autocapitalize", "off");
      expect(el).toHaveAttribute("autocorrect", "off");
      expect(el).toHaveAttribute("spellcheck", "false");
    }
  });

  it("ProgramsManager_should_SendConfirmExecuteOnlyForCheck_When_BlurThenEnterThenCheck", async () => {
    await openForm();
    fireEvent.blur(command());
    fireEvent.keyDown(command(), { key: "Enter" });
    expect(mockProbeCheck.mock.calls).toEqual([[], [{ immediate: true }]]);
    fireEvent.click(check());
    expect(mockProbeCheck).toHaveBeenLastCalledWith({ explicit: true });
    expect(mockProbeCheck.mock.calls.filter(([o]) => o?.explicit)).toHaveLength(1);
  });

  it("ProgramsManager_should_KeepCheckMountedFocusedAndBusyAndDebounceCheckingAnnouncement_When_CheckRuns", async () => {
    const { rerender } = await openForm();
    const button = check();
    button.focus();
    expect(button).toBeEnabled();
    jest.useFakeTimers();
    try {
      setProbeHookState(probeStates.checking);
      rerender(<ProgramsManager />);
      expect(check()).toBe(button);
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute("aria-busy", "true");
      fireEvent.click(button);
      expect(mockProbeCheck).not.toHaveBeenCalled();
      const live = screen.getByRole("status");
      act(() => jest.advanceTimersByTime(100));
      expect(live.querySelector("[aria-hidden='true']:not(svg):not([data-icon])")).not.toBeNull();
      act(() => jest.advanceTimersByTime(250));
      expect(live).toHaveTextContent("Checking...");
      expect(live.querySelector("span:not([aria-hidden])")).toHaveTextContent("Checking...");
      setProbeHookState(probeStates.found);
      rerender(<ProgramsManager />);
      expect(check()).toBe(button);
      expect(button).toBeEnabled();
      expect(button).toHaveAttribute("aria-busy", "false");
    } finally {
      jest.useRealTimers();
    }
  });

  it("ProgramsManager_should_SuggestFlagsInFlagsField_When_ProbeFound", async () => {
    await openForm({
      ...(probeStates.found as Extract<ProbeUiState, { kind: "found" }>),
      flags: [{ name: "--model", short: "", takesValue: true, description: "Model to use", aliases: [] }] as never,
    });
    const flagsInput = screen.getByTestId("prog-flags-input");
    fireEvent.change(flagsInput, { target: { value: "--mo", selectionStart: 4, selectionEnd: 4 } });
    expect(screen.getByRole("option", { name: "--model, takes a value" })).toBeInTheDocument();
    expect(screen.queryByTestId("prog-flags-hint")).toBeNull();
  });

  it.each(["notFound", "noFlags", "wrapper", "found"])(
    "ProgramsManager_should_KeepFlagsFieldPlain_When_%s",
    async (name) => {
      await openForm(probeStates[name]);
      const flagsInput = screen.getByTestId("prog-flags-input");
      fireEvent.change(flagsInput, { target: { value: "--mo", selectionStart: 4, selectionEnd: 4 } });
      expect(screen.queryByRole("listbox")).toBeNull();
      expect(flagsInput).not.toHaveAttribute("role");
    },
  );

  it.each([
    ["needsConfirm", "Suggestions appear after Check reads the flags."],
    ["idle", "Check the command above to enable flag suggestions"],
  ])("ProgramsManager_should_ExplainMissingSuggestionsViaDescribedBy_When_%s", async (name, text) => {
    await openForm(probeStates[name]);
    const hint = screen.getByTestId("prog-flags-hint");
    expect(hint).toHaveTextContent(text);
    expect(screen.getByTestId("prog-flags-input")).toHaveAttribute("aria-describedby", hint.id);
  });
  it("ProgramsManager_should_RestoreFocusToCheck_When_ExplicitCheckSettlesAndFocusWasDropped", async () => {
    const { rerender } = await openForm(probeStates.needsConfirm);
    fireEvent.click(check());
    setProbeHookState(probeStates.checking);
    rerender(<ProgramsManager />);
    (document.activeElement as HTMLElement | null)?.blur(); // browsers drop focus on a disabled button
    setProbeHookState(probeStates.found);
    rerender(<ProgramsManager />);
    expect(check()).toHaveFocus();
  });

  describe("unknown-flag warnings", () => {
    const FOUND_WITH_MODEL = {
      ...(probeStates.found as Extract<ProbeUiState, { kind: "found" }>),
      flags: [{ name: "--model", short: "", takesValue: true, description: "Model to use", aliases: [] }] as never,
    } as ProbeUiState;
    const flagsInput = () => screen.getByTestId("prog-flags-input");
    const type = (value: string) => {
      fireEvent.focus(flagsInput());
      fireEvent.change(flagsInput(), { target: { value } });
      fireEvent.blur(flagsInput());
    };

    it("ProgramsManager_should_ShowWarningDescribedByAndNotInvalid_When_VerbosTyped", async () => {
      await openForm(FOUND_WITH_MODEL);
      type("--verbos");
      const warning = screen.getByTestId("prog-flags-warning");
      expect(warning).toHaveTextContent(
        "--verbos is not listed in claude --help. It may still work (hidden or subcommand flags).",
      );
      expect(flagsInput().getAttribute("aria-describedby")).toContain(warning.id);
      expect(flagsInput()).not.toHaveAttribute("aria-invalid");
      expect(warning).not.toHaveAttribute("role");
      expect(warning.querySelector("[role='alert']")).toBeNull();
      expect(warning.querySelector("button")).toBeNull();
      expect(warning.textContent).not.toMatch(/invalid|error/i);
    });

    it("ProgramsManager_should_HoldWarningUntilTokenSettles_When_StillTyping", async () => {
      await openForm(FOUND_WITH_MODEL);
      fireEvent.focus(flagsInput());
      fireEvent.change(flagsInput(), { target: { value: "--mod" } });
      expect(screen.queryByTestId("prog-flags-warning")).toBeNull();
      fireEvent.change(flagsInput(), { target: { value: "--mod " } });
      expect(screen.getByTestId("prog-flags-warning")).toBeInTheDocument();
    });

    it("ProgramsManager_should_ShowNoSuggestionsOrWarning_When_NotFoundZeroFlagsOrWrapper", async () => {
      for (const name of ["notFound", "noFlags", "wrapper", "found", "needsConfirm", "checking", "transportError"]) {
        const { unmount } = await openForm(probeStates[name]);
        type("--verbos");
        expect(screen.queryByTestId("prog-flags-warning")).toBeNull();
        expect(screen.queryByTestId("prog-available-flags")).toBeNull();
        unmount();
      }
    });

    it("ProgramsManager_should_ShowOnlySoftWarningAndKeepSaveEnabled_When_KnownValidFlagNotInParsedSet", async () => {
      await openForm(FOUND_WITH_MODEL);
      type("--hidden-but-valid");
      expect(screen.getByTestId("prog-flags-warning")).toBeInTheDocument();
      expect(screen.getByTestId("save-program-btn")).toBeEnabled();
      expect(flagsInput()).not.toHaveAttribute("aria-invalid");
    });

    it("ProgramsManager_should_DropWarningIdFromDescribedBy_When_FlagFixed", async () => {
      await openForm(FOUND_WITH_MODEL);
      type("--verbos");
      type("--model x");
      expect(screen.queryByTestId("prog-flags-warning")).toBeNull();
      expect(flagsInput().getAttribute("aria-describedby") ?? "").not.toContain("prog-flags-warning");
    });

    it("ProgramsManager_should_ShowInfoButtonsInAvailableFlagsDisclosureOutsideOptions_When_Found", async () => {
      await openForm(FOUND_WITH_MODEL);
      expect(screen.queryByTestId("prog-flag-info-button")).toBeNull();
      fireEvent.click(screen.getByTestId("prog-available-flags-toggle"));
      expect(screen.getByTestId("prog-available-flags-toggle")).toHaveTextContent("Available flags (1)");
      fireEvent.change(flagsInput(), { target: { value: "--mo", selectionStart: 4, selectionEnd: 4 } });
      const listbox = screen.getByRole("listbox");
      expect(listbox.querySelector("button")).toBeNull();
      const info = screen.getByTestId("prog-flag-info-button");
      expect(listbox.contains(info)).toBe(false);
      fireEvent.click(info);
      expect(screen.getByText("Model to use", { selector: "[data-testid='prog-flag-info-description']" })).toBeInTheDocument();
      expect((flagsInput() as HTMLInputElement).value).toBe("--mo");
    });
  });
});
