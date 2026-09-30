import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { TymuxRolloutPanel } from "./TymuxRolloutPanel";

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockClient = {
  getTymuxRolloutStatus: jest.fn(),
  listSessions: jest.fn(),
  completeTymuxRollbackRehearsal: jest.fn(),
  setTymuxSessionOverride: jest.fn(),
  setTymuxGlobalOverride: jest.fn(),
};

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => mockClient),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

function emptyStatus() {
  return {
    globalEnvVarSet: false,
    rollbackRehearsalCompletedAt: undefined,
    sessionOverrides: [],
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockClient.listSessions.mockResolvedValue({ sessions: [] });
});

describe("TymuxRolloutPanel", () => {
  it("renders the default global-override status and no-overrides state", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    render(<TymuxRolloutPanel />);

    await waitFor(() => expect(screen.getByTestId("tymux-rollout-panel")).toBeInTheDocument());
    expect(screen.getByText("Not set (default: off)")).toBeInTheDocument();
    expect(screen.getByText("No sessions are currently overridden.")).toBeInTheDocument();
    expect(screen.getByTestId("tymux-complete-rehearsal")).toBeInTheDocument();
  });

  it("forces the global override on and reflects the new status", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setTymuxGlobalOverride.mockResolvedValue({
      ...emptyStatus(),
      globalOverride: true,
    });
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-global-override-on"));
    fireEvent.click(screen.getByTestId("tymux-global-override-on"));

    await waitFor(() => expect(screen.getByText("Forced on")).toBeInTheDocument());
    expect(mockClient.setTymuxGlobalOverride).toHaveBeenCalledWith({ forceTymux: true });
  });

  it("clears the global override", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue({
      ...emptyStatus(),
      globalOverride: false,
    });
    mockClient.setTymuxGlobalOverride.mockResolvedValue(emptyStatus());
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-global-override-clear"));
    fireEvent.click(screen.getByTestId("tymux-global-override-clear"));

    await waitFor(() => expect(screen.getByText("Not set (default: off)")).toBeInTheDocument());
    expect(mockClient.setTymuxGlobalOverride).toHaveBeenCalledWith({ forceTymux: undefined });
  });

  it("marks the rehearsal complete and reflects the new status", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.completeTymuxRollbackRehearsal.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: { seconds: BigInt(1700000000), nanos: 0 },
      sessionOverrides: [],
    });
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-complete-rehearsal"));
    fireEvent.click(screen.getByTestId("tymux-complete-rehearsal"));

    await waitFor(() => expect(screen.getByText(/Completed/)).toBeInTheDocument());
    expect(mockClient.completeTymuxRollbackRehearsal).toHaveBeenCalledWith({});
  });

  it("sanitizes the typed title into the tmux session name before sending", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setTymuxSessionOverride.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: undefined,
      sessionOverrides: [{ sessionName: "staplersquad_MyCanary", forceTymux: true }],
    });
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-override-input"));
    fireEvent.change(screen.getByTestId("tymux-override-input"), { target: { value: "My Canary" } });
    expect(screen.getByText("staplersquad_MyCanary")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("tymux-add-override"));

    await waitFor(() => expect(screen.getByTestId("tymux-override-row")).toBeInTheDocument());
    expect(mockClient.setTymuxSessionOverride).toHaveBeenCalledWith({
      sessionName: "staplersquad_MyCanary",
      forceTymux: true,
    });
  });

  it("sanitizes punctuation and leading/trailing whitespace, not just internal spaces", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-override-input"));
    fireEvent.change(screen.getByTestId("tymux-override-input"), { target: { value: "  A:B.C  " } });

    expect(screen.getByText("staplersquad_A_B_C")).toBeInTheDocument();
  });

  it("removes a session override", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: undefined,
      sessionOverrides: [{ sessionName: "staplersquad_canary", forceTymux: true }],
    });
    mockClient.setTymuxSessionOverride.mockResolvedValue(emptyStatus());
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-remove-override"));
    fireEvent.click(screen.getByTestId("tymux-remove-override"));

    await waitFor(() => expect(screen.queryByTestId("tymux-override-row")).not.toBeInTheDocument());
    expect(mockClient.setTymuxSessionOverride).toHaveBeenCalledWith({ sessionName: "staplersquad_canary" });
  });
});
