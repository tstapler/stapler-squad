import React from "react";
import { render, screen } from "@testing-library/react";
import { ConnectionIndicator } from "./ConnectionIndicator";

describe("ConnectionIndicator", () => {
  it("ConnectionIndicator_should_renderLiveLabel_When_connectionStateIsLive", () => {
    render(<ConnectionIndicator connectionState="live" />);
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("ConnectionIndicator_should_renderReconnectingLabel_When_connectionStateIsReconnecting", () => {
    render(<ConnectionIndicator connectionState="reconnecting" />);
    expect(screen.getByText("Reconnecting…")).toBeInTheDocument();
  });

  it("ConnectionIndicator_should_renderConnectingLabel_When_connectionStateIsConnecting", () => {
    render(<ConnectionIndicator connectionState="connecting" />);
    expect(screen.getByText("Connecting…")).toBeInTheDocument();
  });

  it("ConnectionIndicator_should_renderDistinctStaleLabel_When_connectionStateIsStale", () => {
    render(<ConnectionIndicator connectionState="stale" />);
    // Distinct from "reconnecting" — see pre-mortem P1 #1 / component doc
    // comment: the idle-staleness backstop must be visible, not folded
    // silently into the same "actively retrying" label.
    expect(screen.getByText("Stale — reconnecting…")).toBeInTheDocument();
    expect(screen.queryByText("Reconnecting…")).not.toBeInTheDocument();
  });

  it("ConnectionIndicator_should_renderPollingLabel_When_connectionStateIsPolling", () => {
    render(<ConnectionIndicator connectionState="polling" />);
    expect(screen.getByText("Polling (every 30s)")).toBeInTheDocument();
  });

  it("ConnectionIndicator_should_exposeStatusRoleAndAriaLive_When_rendered", () => {
    render(<ConnectionIndicator connectionState="live" />);
    const el = screen.getByTestId("connection-indicator");
    expect(el).toHaveAttribute("role", "status");
    expect(el).toHaveAttribute("aria-live", "polite");
    expect(el).toHaveAttribute("aria-atomic", "true");
  });

  it("ConnectionIndicator_should_neverRenderBlankOrAmbiguousText_When_anyStateIsPassed", () => {
    const states: Array<React.ComponentProps<typeof ConnectionIndicator>["connectionState"]> = [
      "connecting",
      "live",
      "reconnecting",
      "stale",
      "polling",
    ];
    for (const connectionState of states) {
      const { unmount } = render(<ConnectionIndicator connectionState={connectionState} />);
      const el = screen.getByTestId("connection-indicator");
      expect(el.textContent?.trim()).not.toBe("");
      unmount();
    }
  });
});
