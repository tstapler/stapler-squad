import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ProgramsManager } from "../ProgramsManager";

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
