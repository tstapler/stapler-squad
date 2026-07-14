/**
 * Regression test for the Workflow panel in BacklogItemDetail: it must show a
 * graceful empty state instead of rendering nothing when statusEvents is
 * empty, and must render populated history (including system-triggered
 * events) correctly.
 */

import React from "react";
import { render, screen, act } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));

// BacklogItemDetail calls useAnalytics() directly, which otherwise requires a
// real AnalyticsContextProvider ancestor. Stub it out for this focused test.
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const getBacklogItem = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    lastError: null,
  }),
}));

const baseItem: BacklogItem = {
  id: "item-1",
  title: "Item with history",
  description: "desc",
  status: "in_progress",
  priority: 3,
  repoPath: "/tmp/repo",
  skipPlanning: false,
  skipReviewGate: false,
  planApproved: false,
  triageStatus: "idle",
  acCriteria: [],
  linkedSessions: [],
  notes: "",
  createdAt: "2026-07-01T00:00:00Z",
  updatedAt: "2026-07-01T00:00:00Z",
  statusEvents: [],
  totalEstimatedCostUsd: 0,
};

describe("BacklogItemDetail — Workflow panel status history", () => {
  beforeEach(() => {
    getBacklogItem.mockReset();
  });

  it("shows a graceful empty state when statusEvents is empty", async () => {
    getBacklogItem.mockResolvedValue(baseItem);

    render(<BacklogItemDetail itemId="item-1" />);

    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByText("Workflow")).toBeInTheDocument();
    expect(screen.getByText("No status history recorded")).toBeInTheDocument();
  });

  it("renders a non-RPC (system-triggered) status event without a user marker", async () => {
    getBacklogItem.mockResolvedValue({
      ...baseItem,
      statusEvents: [
        {
          id: "ev-1",
          fromStatus: "in_progress",
          toStatus: "review",
          triggeredBy: "system",
          createdAt: "2026-07-01T01:00:00Z",
        },
      ],
    });

    render(<BacklogItemDetail itemId="item-1" />);

    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.queryByText("No status history recorded")).not.toBeInTheDocument();
    expect(screen.getByText("in progress")).toBeInTheDocument();
    expect(screen.getByText("review")).toBeInTheDocument();
    expect(screen.queryByText((text) => text.includes("· user"))).not.toBeInTheDocument();
  });
});
