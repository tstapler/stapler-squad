import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { SessionInactiveNotice } from "../SessionInactiveNotice";

describe("SessionInactiveNotice", () => {
  it("shows the reason and a Retry action for a permanently failed session", () => {
    const onRetryNow = jest.fn();
    render(
      <SessionInactiveNotice status={SessionStatus.PERMANENTLY_FAILED} archived={false} terminalReadOnly onRetryNow={onRetryNow} />,
    );
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Read-only: session is permanently failed.");
    fireEvent.click(screen.getByTestId("session-inactive-notice-retry"));
    expect(onRetryNow).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when the server says input still works, even for a stopped session", () => {
    render(<SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} terminalReadOnly={false} />);
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });

  it("clears once the server stops reporting the terminal read-only", () => {
    const { rerender } = render(<SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} terminalReadOnly />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("session is stopped");
    rerender(<SessionInactiveNotice status={SessionStatus.ACTIVE} archived={false} terminalReadOnly={false} />);
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });

  it("says Restart, with no Retry button, for a stopped session", () => {
    render(<SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} terminalReadOnly onRetryNow={jest.fn()} />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Restart the session to resume input.");
    expect(screen.queryByTestId("session-inactive-notice-retry")).toBeNull();
  });

  it("says Unarchive and offers no Retry for an archived session", () => {
    render(<SessionInactiveNotice status={SessionStatus.PERMANENTLY_FAILED} archived terminalReadOnly onRetryNow={jest.fn()} />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Read-only: session is archived.");
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Unarchive");
    expect(screen.queryByTestId("session-inactive-notice-retry")).toBeNull();
  });

  it("hides its Retry while the terminal's own hard-failed banner shows one", () => {
    render(
      <SessionInactiveNotice status={SessionStatus.PERMANENTLY_FAILED} archived={false} terminalReadOnly onRetryNow={jest.fn()} terminalShowsRetry />,
    );
    expect(screen.queryByTestId("session-inactive-notice-retry")).toBeNull();
  });

  it("shows a retry error inline", () => {
    render(
      <SessionInactiveNotice status={SessionStatus.PERMANENTLY_FAILED} archived={false} terminalReadOnly onRetryNow={jest.fn()} retryError="Failed to retry session." />,
    );
    expect(screen.getByTestId("session-inactive-notice-error")).toHaveTextContent("Failed to retry session.");
  });

  it("announces the reason through the announcer", () => {
    const announce = jest.fn();
    render(
      <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
        <SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} terminalReadOnly />
      </AnnouncerContext.Provider>,
    );
    expect(announce).toHaveBeenCalledWith("Read-only: session is stopped.", "polite", "session-inactive-notice");
  });
});
