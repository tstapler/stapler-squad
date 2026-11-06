import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { TymuxdVersionDirection } from "@/gen/session/v1/tymux_rollout_pb";
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
  restartSession: jest.fn(),
  replaceTymuxd: jest.fn(),
};

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
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

  it("migrates a live session to tymux by its raw title, not the sanitized tmux name", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.restartSession.mockResolvedValue({});
    render(<TymuxRolloutPanel />);

    // kibitzer:ignore replace-magic-literal -- repeating a data-testid string across
    // getByTestId calls is this file's established test pattern (see the override tests above).
    await waitFor(() => screen.getByTestId("tymux-migrate-input"));
    fireEvent.change(screen.getByTestId("tymux-migrate-input"), { target: { value: "My Canary" } });
    fireEvent.click(screen.getByTestId("tymux-migrate-to-tymux"));

    expect(mockClient.restartSession).toHaveBeenCalledWith({
      id: "My Canary",
      preserveOutput: true,
      backendOverride: "tymux",
    });
    await waitFor(() => expect(screen.getByText(/restarted on tymux/)).toBeInTheDocument());
  });

  it("migrates a live session back to tmux and surfaces a failure", async () => {
    mockClient.getTymuxRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.restartSession.mockRejectedValue(new Error("boom"));
    render(<TymuxRolloutPanel />);

    await waitFor(() => screen.getByTestId("tymux-migrate-input"));
    fireEvent.change(screen.getByTestId("tymux-migrate-input"), { target: { value: "My Canary" } });
    fireEvent.click(screen.getByTestId("tymux-migrate-to-tmux"));

    expect(mockClient.restartSession).toHaveBeenCalledWith({
      id: "My Canary",
      preserveOutput: true,
      backendOverride: "tmux",
    });
    await waitFor(() => expect(screen.getByText(/Failed to migrate "My Canary" to tmux/)).toBeInTheDocument());
  });

  describe("tymuxd version", () => {
    const upgradeAvailable = {
      ...emptyStatus(),
      daemon: { running: true, pid: 42, runningVersion: "1.3.0", installedVersion: "1.4.0", direction: TymuxdVersionDirection.UPGRADE },
    };

    it("shows running vs configured versions and offers replacement when they differ", async () => {
      mockClient.getTymuxRolloutStatus.mockResolvedValue(upgradeAvailable);
      render(<TymuxRolloutPanel />);

      await waitFor(() => expect(screen.getByTestId("tymuxd-version")).toHaveTextContent("1.3.0"));
      expect(screen.getByTestId("tymuxd-version")).toHaveTextContent("1.4.0");
      expect(screen.getByTestId("tymuxd-version")).toHaveTextContent("an upgrade is available");
      expect(screen.getByTestId("tymuxd-replace")).toBeEnabled();
    });

    it("hides the replace button when versions match", async () => {
      mockClient.getTymuxRolloutStatus.mockResolvedValue({
        ...emptyStatus(),
        daemon: { running: true, pid: 42, runningVersion: "1.3.0", installedVersion: "1.3.0", direction: TymuxdVersionDirection.SAME },
      });
      render(<TymuxRolloutPanel />);

      await waitFor(() => expect(screen.getByTestId("tymuxd-version")).toHaveTextContent("up to date"));
      expect(screen.queryByTestId("tymuxd-replace")).not.toBeInTheDocument();
    });

    it("asks for confirmation when live sessions block the replace, then retries with the opt-in", async () => {
      mockClient.getTymuxRolloutStatus.mockResolvedValue(upgradeAvailable);
      mockClient.replaceTymuxd
        .mockRejectedValueOnce(new ConnectError("2 live", Code.FailedPrecondition))
        .mockResolvedValueOnce({
          ...emptyStatus(),
          daemon: { running: true, pid: 43, runningVersion: "1.4.0", installedVersion: "1.4.0", direction: TymuxdVersionDirection.SAME },
        });
      render(<TymuxRolloutPanel />);

      await waitFor(() => screen.getByTestId("tymuxd-replace"));
      fireEvent.click(screen.getByTestId("tymuxd-replace"));
      await waitFor(() => screen.getByTestId("tymuxd-replace-anyway"));
      expect(mockClient.replaceTymuxd).toHaveBeenLastCalledWith({ allowInterruptLiveSessions: false });

      fireEvent.click(screen.getByTestId("tymuxd-replace-anyway"));
      await waitFor(() => expect(screen.getByTestId("tymuxd-version")).toHaveTextContent("up to date"));
      expect(mockClient.replaceTymuxd).toHaveBeenLastCalledWith({ allowInterruptLiveSessions: true });
    });
  });
});
