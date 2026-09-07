import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { NativeGitRolloutPanel } from "./NativeGitRolloutPanel";

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockClient = {
  getNativeGitRolloutStatus: jest.fn(),
  listSessions: jest.fn(),
  setNativeWorktreeGlobalOverride: jest.fn(),
  setNativeWorktreeSessionOverride: jest.fn(),
  setNativeMergeGlobalOverride: jest.fn(),
  setNativeMergeWorktreeOverride: jest.fn(),
};

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => mockClient),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

function emptyStatus() {
  return {
    worktreeGlobalOverride: undefined,
    worktreeSessionOverrides: [],
    mergeGlobalOverride: undefined,
    mergeWorktreeOverrides: [],
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockClient.listSessions.mockResolvedValue({ sessions: [] });
});

describe("NativeGitRolloutPanel", () => {
  it("renders the default global-override status and no-overrides state for both sections", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    render(<NativeGitRolloutPanel />);

    await waitFor(() => expect(screen.getByTestId("native-git-rollout-panel")).toBeInTheDocument());
    expect(screen.getByTestId("native-git-worktree-global-badge")).toHaveTextContent("Not set (default: off)");
    expect(screen.getByTestId("native-git-merge-global-badge")).toHaveTextContent("Not set (default: off)");
    expect(screen.getByText("No sessions are currently overridden.")).toBeInTheDocument();
    expect(screen.getByText("No worktree paths are currently overridden.")).toBeInTheDocument();
  });

  // AC1 / validation.md item 1
  it("forces native worktree off globally in a single click", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setNativeWorktreeGlobalOverride.mockResolvedValue({
      ...emptyStatus(),
      worktreeGlobalOverride: false,
    });
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-worktree-global-override-off"));
    fireEvent.click(screen.getByTestId("native-git-worktree-global-override-off"));

    await waitFor(() =>
      expect(screen.getByTestId("native-git-worktree-global-badge")).toHaveTextContent("Forced off"),
    );
    expect(mockClient.setNativeWorktreeGlobalOverride).toHaveBeenCalledWith({ forceNative: false });
  });

  // AC2 / validation.md item 2
  it("adds a session override in two steps: type then click", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setNativeWorktreeSessionOverride.mockResolvedValue({
      ...emptyStatus(),
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-worktree-override-input"));
    fireEvent.change(screen.getByTestId("native-git-worktree-override-input"), {
      target: { value: "backlog-fix-142" },
    });
    fireEvent.click(screen.getByTestId("native-git-worktree-add-override"));

    await waitFor(() => expect(screen.getByTestId("native-git-worktree-override-row")).toBeInTheDocument());
    expect(mockClient.setNativeWorktreeSessionOverride).toHaveBeenCalledWith({
      sessionName: "backlog-fix-142",
      forceNative: true,
    });
    expect(screen.getByTestId("native-git-worktree-override-input")).toHaveValue("");
  });

  // AC3 / validation.md item 3
  it("removes an existing session override in a single click", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({
      ...emptyStatus(),
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    mockClient.setNativeWorktreeSessionOverride.mockResolvedValue(emptyStatus());
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-worktree-remove-override"));
    fireEvent.click(screen.getByTestId("native-git-worktree-remove-override"));

    await waitFor(() => expect(screen.queryByTestId("native-git-worktree-override-row")).not.toBeInTheDocument());
    expect(mockClient.setNativeWorktreeSessionOverride).toHaveBeenCalledWith({ sessionName: "backlog-fix-142" });
  });

  it("clears a global override in a single click", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({ ...emptyStatus(), mergeGlobalOverride: true });
    mockClient.setNativeMergeGlobalOverride.mockResolvedValue(emptyStatus());
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-merge-global-override-clear"));
    fireEvent.click(screen.getByTestId("native-git-merge-global-override-clear"));

    await waitFor(() =>
      expect(screen.getByTestId("native-git-merge-global-badge")).toHaveTextContent("Not set (default: off)"),
    );
    expect(mockClient.setNativeMergeGlobalOverride).toHaveBeenCalledWith({ forceNative: undefined });
  });

  // AC4 / validation.md item 4
  it("shows a specific error message and keeps the control retriable when SetNativeMergeGlobalOverride fails", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    mockClient.setNativeMergeGlobalOverride.mockRejectedValue(new Error("boom"));
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-merge-global-override-on"));
    fireEvent.click(screen.getByTestId("native-git-merge-global-override-on"));

    const error = await screen.findByTestId("native-git-merge-error");
    expect(error).toHaveTextContent("Failed to update the global override — try again");
    expect(error).toHaveAttribute("role", "alert");
    expect(screen.getByTestId("native-git-merge-global-override-on")).not.toBeDisabled();
  });

  // AC5 / validation.md item 5 (Task 4.3.1d)
  it("shows Unknown badge and disables only global toggle buttons when GetNativeGitRolloutStatus fails", async () => {
    mockClient.getNativeGitRolloutStatus.mockRejectedValue(new Error("boom"));
    render(<NativeGitRolloutPanel />);

    await waitFor(() => expect(screen.getByTestId("native-git-load-error-banner")).toBeInTheDocument());
    expect(screen.getByTestId("native-git-worktree-global-badge")).toHaveTextContent("Unknown — reload failed");
    expect(screen.getByTestId("native-git-merge-global-badge")).toHaveTextContent("Unknown — reload failed");

    expect(screen.getByTestId("native-git-worktree-global-override-on")).toBeDisabled();
    expect(screen.getByTestId("native-git-worktree-global-override-off")).toBeDisabled();
    expect(screen.getByTestId("native-git-worktree-global-override-clear")).toBeDisabled();
    expect(screen.getByTestId("native-git-merge-global-override-on")).toBeDisabled();
    expect(screen.getByTestId("native-git-merge-global-override-off")).toBeDisabled();
    expect(screen.getByTestId("native-git-merge-global-override-clear")).toBeDisabled();

    // Override add/remove controls stay enabled — filling the input proves the
    // add button isn't disabled by the load-failure state (only by its own
    // separate empty-input guard).
    fireEvent.change(screen.getByTestId("native-git-worktree-override-input"), {
      target: { value: "backlog-fix-142" },
    });
    expect(screen.getByTestId("native-git-worktree-add-override")).not.toBeDisabled();
  });

  // AC6 / validation.md item 6
  it("recovers from a load failure via the Retry button", async () => {
    mockClient.getNativeGitRolloutStatus.mockRejectedValueOnce(new Error("boom"));
    render(<NativeGitRolloutPanel />);

    await waitFor(() => expect(screen.getByTestId("native-git-load-error-banner")).toBeInTheDocument());

    mockClient.getNativeGitRolloutStatus.mockResolvedValueOnce(emptyStatus());
    fireEvent.click(screen.getByTestId("native-git-retry"));

    await waitFor(() => expect(screen.queryByTestId("native-git-load-error-banner")).not.toBeInTheDocument());
    expect(screen.getByTestId("native-git-worktree-global-badge")).toHaveTextContent("Not set (default: off)");
    expect(screen.getByTestId("native-git-worktree-global-override-on")).not.toBeDisabled();
  });

  // AC7 / validation.md item 7 (Task 4.3.1e)
  it("names the still-forced-on session after a global force-off, with role=status not alert", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({
      ...emptyStatus(),
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    mockClient.setNativeWorktreeGlobalOverride.mockResolvedValue({
      ...emptyStatus(),
      worktreeGlobalOverride: false,
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-worktree-global-override-off"));
    fireEvent.click(screen.getByTestId("native-git-worktree-global-override-off"));

    const note = await screen.findByTestId("native-git-worktree-precedence-note");
    expect(note).toHaveAttribute("role", "status");
    expect(note).toHaveTextContent(
      "1 session override still forces native worktree management on for this session, which takes " +
        "precedence over the global setting above: backlog-fix-142. Remove it below if this is part of the incident.",
    );
  });

  // AC7, merge section symmetric case (keyed by worktreePath)
  it("names the still-forced-off worktree path after a global force-on in the merge section", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({
      ...emptyStatus(),
      mergeWorktreeOverrides: [{ worktreePath: "/repo/worktrees/backlog-fix-142", forceNative: false }],
    });
    mockClient.setNativeMergeGlobalOverride.mockResolvedValue({
      ...emptyStatus(),
      mergeGlobalOverride: true,
      mergeWorktreeOverrides: [{ worktreePath: "/repo/worktrees/backlog-fix-142", forceNative: false }],
    });
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-merge-global-override-on"));
    fireEvent.click(screen.getByTestId("native-git-merge-global-override-on"));

    const note = await screen.findByTestId("native-git-merge-precedence-note");
    expect(note).toHaveAttribute("role", "status");
    expect(note).toHaveTextContent("/repo/worktrees/backlog-fix-142");
    expect(note).toHaveTextContent("native merge off for this worktree path");
  });

  // AC8 / validation.md item 8
  it("reuses StreamHubRolloutPanel.css badge classnames, the established data-testid scheme, and renders no rollback-rehearsal section", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-rollout-panel"));
    expect(screen.getByTestId("native-git-worktree-global-badge").className).toMatch(/badge/);
    expect(screen.getByTestId("native-git-merge-global-badge").className).toMatch(/badge/);
    expect(screen.queryByText(/rollback rehearsal/i)).not.toBeInTheDocument();
  });

  // AC10 / validation.md item 10 (Task 4.3.1f)
  it("gives worktree and merge Force-off buttons distinct aria-labels", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue(emptyStatus());
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-rollout-panel"));
    const worktreeOff = screen.getByRole("button", { name: "Force native worktree management off for all sessions" });
    const mergeOff = screen.getByRole("button", { name: "Force native merge off for all worktree paths" });
    expect(worktreeOff).toBeInTheDocument();
    expect(mergeOff).toBeInTheDocument();
    expect(worktreeOff).not.toBe(mergeOff);
  });

  it("gives worktree and merge per-row Remove buttons distinct aria-labels", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({
      worktreeGlobalOverride: undefined,
      worktreeSessionOverrides: [{ sessionName: "canary-1", forceNative: true }],
      mergeGlobalOverride: undefined,
      mergeWorktreeOverrides: [{ worktreePath: "/repo/worktrees/canary-1", forceNative: true }],
    });
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-rollout-panel"));
    expect(
      screen.getByRole("button", { name: "Remove the native worktree management override for canary-1" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove the native merge override for /repo/worktrees/canary-1" }),
    ).toBeInTheDocument();
  });

  // AC11 / validation.md item 11
  it("uses role=alert for mutation errors and role=status for the precedence note", async () => {
    mockClient.getNativeGitRolloutStatus.mockResolvedValue({
      ...emptyStatus(),
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    mockClient.setNativeWorktreeGlobalOverride.mockResolvedValue({
      ...emptyStatus(),
      worktreeGlobalOverride: false,
      worktreeSessionOverrides: [{ sessionName: "backlog-fix-142", forceNative: true }],
    });
    mockClient.setNativeMergeGlobalOverride.mockRejectedValue(new Error("boom"));
    render(<NativeGitRolloutPanel />);

    await waitFor(() => screen.getByTestId("native-git-worktree-global-override-off"));
    fireEvent.click(screen.getByTestId("native-git-worktree-global-override-off"));
    const note = await screen.findByTestId("native-git-worktree-precedence-note");
    expect(note.getAttribute("role")).toBe("status");

    fireEvent.click(screen.getByTestId("native-git-merge-global-override-on"));
    const error = await screen.findByTestId("native-git-merge-error");
    expect(error.getAttribute("role")).toBe("alert");
  });

  it("marks the load-failure banner with role=alert", async () => {
    mockClient.getNativeGitRolloutStatus.mockRejectedValue(new Error("boom"));
    render(<NativeGitRolloutPanel />);

    const banner = await screen.findByTestId("native-git-load-error-banner");
    expect(banner).toHaveAttribute("role", "alert");
  });
});
