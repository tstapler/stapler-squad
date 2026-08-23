import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { StreamHubRolloutPanel } from "./StreamHubRolloutPanel";

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockClient = {
  getStreamHubRolloutStatus: jest.fn(),
  listSessions: jest.fn(),
  completeStreamHubRollbackRehearsal: jest.fn(),
  setStreamHubSessionOverride: jest.fn(),
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

describe("StreamHubRolloutPanel", () => {
  it("renders the global env var status and no-overrides state", async () => {
    mockClient.getStreamHubRolloutStatus.mockResolvedValue(emptyStatus());
    render(<StreamHubRolloutPanel />);

    await waitFor(() => expect(screen.getByTestId("stream-hub-rollout-panel")).toBeInTheDocument());
    expect(screen.getByText("Off")).toBeInTheDocument();
    expect(screen.getByText("No sessions are currently overridden.")).toBeInTheDocument();
    expect(screen.getByTestId("stream-hub-complete-rehearsal")).toBeInTheDocument();
  });

  it("marks the rehearsal complete and reflects the new status", async () => {
    mockClient.getStreamHubRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.completeStreamHubRollbackRehearsal.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: { seconds: BigInt(1700000000), nanos: 0 },
      sessionOverrides: [],
    });
    render(<StreamHubRolloutPanel />);

    await waitFor(() => screen.getByTestId("stream-hub-complete-rehearsal"));
    fireEvent.click(screen.getByTestId("stream-hub-complete-rehearsal"));

    await waitFor(() => expect(screen.getByText(/Completed/)).toBeInTheDocument());
    expect(mockClient.completeStreamHubRollbackRehearsal).toHaveBeenCalledWith({});
  });

  it("adds a session override and lists it", async () => {
    mockClient.getStreamHubRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setStreamHubSessionOverride.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: undefined,
      sessionOverrides: [{ sessionName: "canary-session", forceHub: true }],
    });
    render(<StreamHubRolloutPanel />);

    await waitFor(() => screen.getByTestId("stream-hub-override-input"));
    fireEvent.change(screen.getByTestId("stream-hub-override-input"), { target: { value: "canary-session" } });
    fireEvent.click(screen.getByTestId("stream-hub-add-override"));

    await waitFor(() => expect(screen.getByTestId("stream-hub-override-row")).toBeInTheDocument());
    expect(mockClient.setStreamHubSessionOverride).toHaveBeenCalledWith({
      sessionName: "canary-session",
      forceHub: true,
    });
  });

  it("removes a session override", async () => {
    mockClient.getStreamHubRolloutStatus.mockResolvedValue({
      globalEnvVarSet: false,
      rollbackRehearsalCompletedAt: undefined,
      sessionOverrides: [{ sessionName: "canary-session", forceHub: true }],
    });
    mockClient.setStreamHubSessionOverride.mockResolvedValue(emptyStatus());
    render(<StreamHubRolloutPanel />);

    await waitFor(() => screen.getByTestId("stream-hub-remove-override"));
    fireEvent.click(screen.getByTestId("stream-hub-remove-override"));

    await waitFor(() => expect(screen.queryByTestId("stream-hub-override-row")).not.toBeInTheDocument());
    expect(mockClient.setStreamHubSessionOverride).toHaveBeenCalledWith({ sessionName: "canary-session" });
  });
});
