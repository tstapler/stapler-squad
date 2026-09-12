/**
 * Tests for ForkPressureStatusBanner — the persistent status banner for
 * ongoing system-health monitors (Fork Pressure, memory usage), distinct
 * from the discrete notification feed/toast stack. Covers:
 *  - AC2: an active Fork Pressure episode renders via SystemBanner.
 *  - AC3: a cleared episode (metadata.state === "cleared") renders nothing,
 *    with no stale lingering alert.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { ForkPressureStatusBanner } from "./ForkPressureStatusBanner";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";
import { useSystemMemory } from "@/lib/contexts/SystemMemoryContext";
import type { NotificationHistoryRecord } from "@/gen/session/v1/session_pb";

jest.mock("@/lib/hooks/useNotificationHistory", () => ({
  useNotificationHistory: jest.fn(),
}));

jest.mock("@/lib/contexts/SystemMemoryContext", () => ({
  useSystemMemory: jest.fn(),
}));

// SystemBanner calls useAnalytics() directly for action/dismiss tracking —
// mock it rather than standing up a real AnalyticsContextProvider, matching
// PatternsView.test.tsx's convention for the same situation.
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockUseNotificationHistory = useNotificationHistory as jest.MockedFunction<typeof useNotificationHistory>;
const mockUseSystemMemory = useSystemMemory as jest.MockedFunction<typeof useSystemMemory>;

function makeRecord(overrides: Partial<NotificationHistoryRecord> = {}): NotificationHistoryRecord {
  return {
    id: "episode-1",
    sessionId: "fork-pressure",
    sessionName: "System",
    notificationType: 1,
    priority: 2,
    title: "Fork Pressure: warning",
    message: "Subprocess failures: 0/60s | Spawns: 120/60s | Zombies: 0 | Level: warning",
    metadata: { reason: "fork_pressure", level: "warning", state: "active" },
    createdAt: { seconds: 1n, nanos: 0 },
    isRead: false,
    occurrenceCount: 0,
    $typeName: "session.v1.NotificationHistoryRecord",
    ...overrides,
  } as NotificationHistoryRecord;
}

function mockHistory(notifications: NotificationHistoryRecord[]) {
  mockUseNotificationHistory.mockReturnValue({
    notifications,
    unreadCount: 0,
    loading: false,
    error: null,
    hasMore: false,
    lastUpdatedAt: null,
    markAsRead: jest.fn(),
    clearHistory: jest.fn(),
    loadMore: jest.fn(),
    refresh: jest.fn(),
  });
}

describe("ForkPressureStatusBanner", () => {
  beforeEach(() => {
    mockUseSystemMemory.mockReturnValue({ systemMemoryPct: 10, isUnderPressure: false });
  });

  it("renders an active Fork Pressure warning via SystemBanner", () => {
    mockHistory([makeRecord()]);
    render(<ForkPressureStatusBanner />);
    expect(screen.getByTestId("fork-pressure-status-banner")).toBeInTheDocument();
    expect(screen.getByText(/Level: warning/)).toBeInTheDocument();
  });

  it("renders nothing once the episode's cleared signal lands", () => {
    mockHistory([makeRecord({ metadata: { reason: "fork_pressure", level: "warning", state: "cleared" } })]);
    render(<ForkPressureStatusBanner />);
    expect(screen.queryByTestId("fork-pressure-status-banner")).not.toBeInTheDocument();
  });

  it("ignores notifications from other sessions", () => {
    mockHistory([makeRecord({ sessionId: "some-other-session" })]);
    render(<ForkPressureStatusBanner />);
    expect(screen.queryByTestId("fork-pressure-status-banner")).not.toBeInTheDocument();
  });

  it("renders a memory pressure banner independently of Fork Pressure", () => {
    mockHistory([]);
    mockUseSystemMemory.mockReturnValue({ systemMemoryPct: 92, isUnderPressure: true });
    render(<ForkPressureStatusBanner />);
    expect(screen.getByTestId("memory-pressure-status-banner")).toBeInTheDocument();
    expect(screen.queryByTestId("fork-pressure-status-banner")).not.toBeInTheDocument();
  });
});
