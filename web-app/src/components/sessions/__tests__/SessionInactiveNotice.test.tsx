import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { SessionInactiveNotice } from "../SessionInactiveNotice";

describe("SessionInactiveNotice", () => {
  it("shows the read-only reason and a Retry action for a permanently failed session", () => {
    const onRetryNow = jest.fn();
    render(<SessionInactiveNotice status={SessionStatus.PERMANENTLY_FAILED} archived={false} onRetryNow={onRetryNow} />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Read-only: session is permanently failed.");
    fireEvent.click(screen.getByTestId("session-inactive-notice-retry"));
    expect(onRetryNow).toHaveBeenCalledTimes(1);
  });

  it("clears once the session becomes ACTIVE", () => {
    const { rerender } = render(<SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("session is stopped");
    rerender(<SessionInactiveNotice status={SessionStatus.ACTIVE} archived={false} />);
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });

  it("offers no Retry for a stopped session without onRetryNow", () => {
    render(<SessionInactiveNotice status={SessionStatus.STOPPED} archived={false} />);
    expect(screen.queryByTestId("session-inactive-notice-retry")).toBeNull();
  });

  it("says archived and offers no Retry for an archived session, even when ACTIVE", () => {
    render(<SessionInactiveNotice status={SessionStatus.ACTIVE} archived onRetryNow={jest.fn()} />);
    expect(screen.getByTestId("session-inactive-notice")).toHaveTextContent("Read-only: session is archived.");
    expect(screen.queryByTestId("session-inactive-notice-retry")).toBeNull();
  });

  it("renders nothing for transient statuses", () => {
    render(<SessionInactiveNotice status={SessionStatus.CREATING} archived={false} />);
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });
});
