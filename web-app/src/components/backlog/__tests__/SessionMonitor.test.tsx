/**
 * Tests for SessionMonitor's fetch-failure handling.
 *
 * getTerminalSnapshot/getConversationMessages (useSessionService) now reject
 * on a genuine RPC failure instead of resolving to "" / [] — SessionMonitor
 * must render a distinct, retryable error state rather than the ambiguous
 * "No output yet…"/"No conversation history yet…" empty state, which looks
 * identical to real emptiness (docs/tasks/backlog-feature-improvement.md,
 * Manual Gates section).
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SessionMonitor } from "../SessionMonitor";

const getTerminalSnapshot = jest.fn();
const getConversationMessages = jest.fn();
const writeToSession = jest.fn();

jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({
    getTerminalSnapshot,
    getConversationMessages,
    writeToSession,
  }),
}));

describe("SessionMonitor", () => {
  beforeEach(() => {
    getTerminalSnapshot.mockReset();
    getConversationMessages.mockReset();
    writeToSession.mockReset();
  });

  it("SessionMonitor_should_renderRetryableError_When_getTerminalSnapshotRejects", async () => {
    getTerminalSnapshot.mockRejectedValue(new Error("network down"));
    getConversationMessages.mockResolvedValue([]);

    render(<SessionMonitor sessionId="s1" isRunning={true} />);

    await waitFor(() =>
      expect(screen.getByTestId("session-monitor-terminal-error")).toBeInTheDocument()
    );
    expect(screen.getByText(/network down/)).toBeInTheDocument();
    expect(screen.queryByText("No output yet…")).toBeNull();
  });

  it("SessionMonitor_should_retryFetch_When_terminalRetryClicked", async () => {
    getTerminalSnapshot.mockRejectedValueOnce(new Error("network down"));
    getTerminalSnapshot.mockResolvedValueOnce("some output");
    getConversationMessages.mockResolvedValue([]);

    render(<SessionMonitor sessionId="s1" isRunning={true} />);

    await waitFor(() =>
      expect(screen.getByTestId("session-monitor-retry-terminal")).toBeInTheDocument()
    );
    fireEvent.click(screen.getByTestId("session-monitor-retry-terminal"));

    await waitFor(() => expect(screen.getByText("some output")).toBeInTheDocument());
    expect(screen.queryByTestId("session-monitor-terminal-error")).toBeNull();
  });

  it("SessionMonitor_should_renderRetryableError_When_getConversationMessagesRejects", async () => {
    getTerminalSnapshot.mockResolvedValue("");
    getConversationMessages.mockRejectedValue(new Error("timed out"));

    render(<SessionMonitor sessionId="s1" isRunning={true} />);

    fireEvent.click(screen.getByRole("button", { name: /history/i }));

    await waitFor(() =>
      expect(screen.getByTestId("session-monitor-conversation-error")).toBeInTheDocument()
    );
    expect(screen.getByText(/timed out/)).toBeInTheDocument();
    expect(screen.queryByText("No conversation history yet…")).toBeNull();
  });

  it("SessionMonitor_should_renderGenuineEmptyState_When_fetchesSucceedWithNoData", async () => {
    getTerminalSnapshot.mockResolvedValue("");
    getConversationMessages.mockResolvedValue([]);

    render(<SessionMonitor sessionId="s1" isRunning={true} />);

    await waitFor(() => expect(getTerminalSnapshot).toHaveBeenCalled());
    expect(screen.getByText("No output yet…")).toBeInTheDocument();
    expect(screen.queryByTestId("session-monitor-terminal-error")).toBeNull();
  });
});
