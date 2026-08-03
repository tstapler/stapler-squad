import React from "react";
import { render, screen } from "@testing-library/react";
import { RespawnHistorySection } from "./RespawnHistorySection";
import type { BacklogItem, RespawnEvent } from "@/lib/hooks/useBacklogService";

beforeEach(() => {
  localStorage.clear();
});

function makeRespawnEvent(overrides: Partial<RespawnEvent> = {}): RespawnEvent {
  return {
    id: `re-${Math.random()}`,
    reason: "stale_work_remediation",
    queued: false,
    ...overrides,
  };
}

function makeItem(respawnEvents: RespawnEvent[]): BacklogItem {
  return {
    id: "itm_df0d5872",
    title: "Chronically stuck item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    respawnEvents,
    totalEstimatedCostUsd: 0,
  };
}

describe("RespawnHistorySection", () => {
  it("RespawnHistorySection_should_RenderEmptyStateText_When_NoRespawnEventsExist", () => {
    render(<RespawnHistorySection item={makeItem([])} defaultExpanded={true} />);
    expect(screen.getByTestId("respawn-history-empty")).toHaveTextContent(
      "No automated respawns recorded for this item."
    );
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("RespawnHistorySection_should_RenderEventRowWithFormattedReasonAndSessionRefs_When_EventHasBothUuids", () => {
    const events = [
      makeRespawnEvent({
        reason: "review_respawn",
        triggeringSessionUuid: "trigger-uuid-1",
        resultingSessionUuid: "result-uuid-1",
        createdAt: "2026-08-01T12:00:00.000Z",
      }),
    ];
    render(<RespawnHistorySection item={makeItem(events)} defaultExpanded={true} />);

    expect(screen.getByText("Abandoned review respawn")).toBeInTheDocument();
    expect(screen.getByText(/Triggered by session trigger-uuid-1/)).toBeInTheDocument();
    expect(screen.getByText(/Spawned session result-uuid-1/)).toBeInTheDocument();
  });

  it("RespawnHistorySection_should_RenderQueuedText_When_QueuedIsTrueAndNoResultingSession", () => {
    const events = [makeRespawnEvent({ queued: true, resultingSessionUuid: undefined })];
    render(<RespawnHistorySection item={makeItem(events)} defaultExpanded={true} />);
    expect(screen.getByText(/Queued — waiting for a concurrency slot\./)).toBeInTheDocument();
  });

  it("RespawnHistorySection_should_RenderFailedText_When_NotQueuedAndNoResultingSession", () => {
    const events = [makeRespawnEvent({ queued: false, resultingSessionUuid: undefined })];
    render(<RespawnHistorySection item={makeItem(events)} defaultExpanded={true} />);
    expect(screen.getByText(/Spawn attempt failed\./)).toBeInTheDocument();
  });

  it("RespawnHistorySection_should_RenderReconciliationCaption_When_EventsExist", () => {
    render(<RespawnHistorySection item={makeItem([makeRespawnEvent()])} defaultExpanded={true} />);
    expect(screen.getByText(/All-time respawn history/)).toBeInTheDocument();
  });
});
