// @feature ui:fork-pressure-status-banner
/**
 * Backlog item cfda07b7-73fb-42e1-a21b-7fdf8a052a14, AC3/AC4: the persistent
 * status banner must reflect the CURRENT state of the two host-wide
 * system-health monitors (Fork Pressure, Memory usage) from
 * NotificationContext's already-flowing notificationHistory, hidden when both
 * are normal, and cleared once a monitor's episode actually ends.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { ForkPressureStatusBanner } from "./ForkPressureStatusBanner";
import type { NotificationHistoryItem } from "@/lib/types/notification";

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

function makeItem(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: overrides.id ?? `notif-${Math.random()}`,
    sessionId: "fork-pressure",
    sessionName: "System",
    message: "",
    timestamp: Date.now(),
    isRead: false,
    notificationType: "warning",
    ...overrides,
  };
}

let mockHistory: NotificationHistoryItem[] = [];
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ notificationHistory: mockHistory }),
}));

describe("ForkPressureStatusBanner", () => {
  beforeEach(() => {
    mockHistory = [];
  });

  it("renders nothing when both monitors are normal (no history at all)", () => {
    const { container } = render(<ForkPressureStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when both monitors' latest record is a cleared (ok) state", () => {
    mockHistory = [
      makeItem({
        sessionId: "fork-pressure",
        message: "Fork pressure has returned to normal.",
        metadata: { fork_pressure_level: "ok" },
        timestamp: 100,
      }),
      makeItem({
        sessionId: "system",
        message: "Process memory has dropped back below 75% of its configured limit.",
        metadata: { reason: "memory_pressure", memory_pressure_level: "ok" },
        timestamp: 100,
      }),
    ];
    const { container } = render(<ForkPressureStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the fork-pressure banner when it is elevated", () => {
    mockHistory = [
      makeItem({
        sessionId: "fork-pressure",
        message: "Subprocess failures: 12/30s | Spawns: 5/30s | Zombies: 0 | Level: critical",
        metadata: { fork_pressure_level: "critical" },
        timestamp: 200,
      }),
    ];
    render(<ForkPressureStatusBanner />);
    expect(screen.getByTestId("fork-pressure-status")).toBeInTheDocument();
    expect(screen.getByText(/Subprocess failures/)).toBeInTheDocument();
    expect(screen.queryByTestId("memory-pressure-status")).not.toBeInTheDocument();
  });

  it("shows the memory banner when it is elevated", () => {
    mockHistory = [
      makeItem({
        sessionId: "system",
        message: "Process memory is at 95% of its configured limit.",
        metadata: { reason: "memory_pressure", memory_pressure_level: "warning" },
        timestamp: 300,
      }),
    ];
    render(<ForkPressureStatusBanner />);
    expect(screen.getByTestId("memory-pressure-status")).toBeInTheDocument();
    expect(screen.queryByTestId("fork-pressure-status")).not.toBeInTheDocument();
  });

  it("shows both banners when both monitors are elevated", () => {
    mockHistory = [
      makeItem({
        sessionId: "fork-pressure",
        message: "elevated fork pressure",
        metadata: { fork_pressure_level: "warning" },
        timestamp: 100,
      }),
      makeItem({
        sessionId: "system",
        message: "elevated memory",
        metadata: { reason: "memory_pressure", memory_pressure_level: "warning" },
        timestamp: 100,
      }),
    ];
    render(<ForkPressureStatusBanner />);
    expect(screen.getByTestId("fork-pressure-status")).toBeInTheDocument();
    expect(screen.getByTestId("memory-pressure-status")).toBeInTheDocument();
  });

  it("reflects the cleared state once the fork-pressure record's latest occurrence is 'ok', even though an earlier elevated occurrence exists", () => {
    // A stable-ID record collapses in place, so only the latest occurrence's
    // timestamp/metadata should win -- this pins that "current state" means
    // "latest", not "ever seen elevated".
    mockHistory = [
      makeItem({
        sessionId: "fork-pressure",
        message: "elevated",
        metadata: { fork_pressure_level: "critical" },
        timestamp: 100,
      }),
      makeItem({
        sessionId: "fork-pressure",
        message: "cleared",
        metadata: { fork_pressure_level: "ok" },
        timestamp: 200,
      }),
    ];
    render(<ForkPressureStatusBanner />);
    expect(screen.queryByTestId("fork-pressure-status")).not.toBeInTheDocument();
  });
});
