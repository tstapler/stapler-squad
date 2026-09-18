/**
 * Tests for TriggerExecutionHistory (webhook-triggers Epic 7.2).
 *
 * Covers: 5-state outcome badges, the "N received / M matched" counter, and the
 * default collapse of no_match events (research/ux.md §2).
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { TriggerExecutionHistory } from "./TriggerExecutionHistory";
import { TriggerFireEventProto } from "@/gen/session/v1/session_pb";

let mockEvents: Partial<TriggerFireEventProto>[] = [];
let mockLoading = false;
let mockError: Error | null = null;

jest.mock("@/lib/hooks/useTriggerFireEvents", () => ({
  useTriggerFireEvents: () => ({
    events: mockEvents,
    loading: mockLoading,
    error: mockError,
    refresh: jest.fn(),
  }),
}));

function makeEvent(overrides: Partial<TriggerFireEventProto> = {}): TriggerFireEventProto {
  return {
    id: `ev-${Math.random()}`,
    workflowId: "wf-1",
    outcome: "fired_success",
    deliveryId: "d1",
    sessionId: "",
    errorMessage: "",
    ...overrides,
  } as unknown as TriggerFireEventProto;
}

describe("TriggerExecutionHistory", () => {
  beforeEach(() => {
    mockEvents = [];
    mockLoading = false;
    mockError = null;
  });

  it("TriggerExecutionHistory_should_showEmptyState_When_noEventsReceived", () => {
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByTestId("trigger-history-empty")).toBeInTheDocument();
  });

  it("TriggerExecutionHistory_should_showReceivedAndMatchedCounts", () => {
    mockEvents = [
      makeEvent({ id: "e1", outcome: "fired_success" }),
      makeEvent({ id: "e2", outcome: "no_match" }),
      makeEvent({ id: "e3", outcome: "no_match" }),
    ];
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByText(/3 received \/ 1 matched/)).toBeInTheDocument();
  });

  it("TriggerExecutionHistory_should_collapseNoMatchEvents_By_default", () => {
    mockEvents = [
      makeEvent({ id: "e1", outcome: "fired_success" }),
      makeEvent({ id: "e2", outcome: "no_match" }),
    ];
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByTestId("trigger-history-entry-e1")).toBeInTheDocument();
    expect(screen.queryByTestId("trigger-history-entry-e2")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("trigger-history-toggle-no-match"));
    expect(screen.getByTestId("trigger-history-entry-e2")).toBeInTheDocument();
  });

  it("TriggerExecutionHistory_should_renderDistinctBadges_For5States", () => {
    mockEvents = [
      makeEvent({ id: "e1", outcome: "fired_success" }),
      makeEvent({ id: "e2", outcome: "fired_failed", errorMessage: "worktree failure" }),
      makeEvent({ id: "e3", outcome: "rejected", errorMessage: "signature mismatch" }),
    ];
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByTestId("trigger-history-entry-e1")).toHaveTextContent("Fired");
    expect(screen.getByTestId("trigger-history-entry-e2")).toHaveTextContent("Session creation failed");
    expect(screen.getByTestId("trigger-history-entry-e2")).toHaveTextContent("worktree failure");
    expect(screen.getByTestId("trigger-history-entry-e3")).toHaveTextContent("Rejected");
    expect(screen.getByTestId("trigger-history-entry-e3")).toHaveTextContent("signature mismatch");
  });

  it("TriggerExecutionHistory_should_linkToSession_When_sessionIdPresent", () => {
    mockEvents = [makeEvent({ id: "e1", outcome: "fired_success", sessionId: "sess-42" })];
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    const link = screen.getByTestId("trigger-history-session-link-e1");
    expect(link).toHaveAttribute("href", "/?session=sess-42");
  });

  it("TriggerExecutionHistory_should_showLoadingState", () => {
    mockLoading = true;
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByTestId("trigger-history-loading")).toBeInTheDocument();
  });

  it("TriggerExecutionHistory_should_showError_When_fetchFails", () => {
    mockError = new Error("network error");
    render(<TriggerExecutionHistory workflowId="wf-1" />);
    expect(screen.getByRole("alert")).toHaveTextContent("network error");
  });
});
