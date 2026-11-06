import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { SessionUnavailableCard } from "../SessionUnavailableCard";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";

function setup(props: Partial<React.ComponentProps<typeof SessionUnavailableCard>> = {}) {
  const announce = jest.fn();
  const onOpenNotifications = jest.fn();
  const onGoToSessions = jest.fn();
  const onRetry = jest.fn();
  render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <SessionUnavailableCard
        variant="unavailable"
        sessionLabel="ee1b4be0"
        onOpenNotifications={onOpenNotifications}
        onGoToSessions={onGoToSessions}
        onRetry={onRetry}
        {...props}
      />
    </AnnouncerContext.Provider>,
  );
  return { announce, onOpenNotifications, onGoToSessions, onRetry };
}

describe("SessionUnavailableCard (RO-3, RO-7, RO-10)", () => {
  it("ro10_should_show_the_captured_title_and_message_when_the_record_is_known", () => {
    setup({ notification: { title: "Task failed", message: "tests failing", timestampMs: Date.now() - 4 * 60_000 } });
    expect(screen.getByText("Session no longer available")).toBeInTheDocument();
    expect(screen.getByTestId("session-unavailable-notification")).toHaveTextContent("Task failed");
    expect(screen.getByTestId("session-unavailable-notification")).toHaveTextContent("tests failing");
    expect(screen.getByText(/4m ago/)).toBeInTheDocument();
  });

  it("ro10_should_show_only_the_removed_line_when_the_record_is_pruned", () => {
    setup({ notification: null });
    expect(screen.getByText("ee1b4be0 was removed before you opened it")).toBeInTheDocument();
    expect(screen.queryByTestId("session-unavailable-notification")).toBeNull();
  });

  it("ro3_should_offer_open_notifications_and_go_to_sessions_and_announce_once_without_a_live_role", () => {
    const { announce, onOpenNotifications, onGoToSessions } = setup();
    fireEvent.click(screen.getByTestId("session-unavailable-open-notifications"));
    fireEvent.click(screen.getByTestId("session-unavailable-go-to-sessions"));
    expect(onOpenNotifications).toHaveBeenCalledTimes(1);
    expect(onGoToSessions).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("session-unavailable-card").querySelector("[role=status],[role=alert]")).toBeNull();
  });

  it("ro7_should_show_could_not_load_with_retry_for_a_failed_lookup", () => {
    const { onRetry } = setup({ variant: "failed" });
    expect(screen.getByText("Could not load session.")).toBeInTheDocument();
    expect(screen.queryByTestId("session-unavailable-open-notifications")).toBeNull();
    fireEvent.click(screen.getByTestId("session-unavailable-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
