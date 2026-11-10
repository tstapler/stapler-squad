import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DiagnoseButton } from "./DiagnoseButton";
import { ActionsSection } from "./ActionsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { noManualReview } from "./actionsSectionTestFixtures";

const mockTrack = jest.fn();
jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: mockTrack }) }));
jest.mock("@/lib/nav", () => {
  const actual = jest.requireActual("@/lib/nav/impl.memory");
  return { ...actual };
});

describe("DiagnoseButton", () => {
  it("dispatches once and links to the diagnostic session", async () => {
    const onDiagnose = jest.fn().mockResolvedValue("diag-session-1");
    render(<DiagnoseButton onDiagnose={onDiagnose} reasonLabel="none" />);

    fireEvent.click(screen.getByTestId("backlog-action-diagnose"));
    fireEvent.click(screen.getByTestId("backlog-action-diagnose"));

    const link = await screen.findByTestId("backlog-action-diagnose-session-link");
    expect(link.getAttribute("href")).toContain("diag-session-1");
    expect(onDiagnose).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("backlog-action-diagnose")).toHaveTextContent("Diagnostic session dispatched");
  });

  it("shows the dispatch failure and allows a retry", async () => {
    const onDiagnose = jest.fn().mockRejectedValueOnce(new Error("diagnostic dispatch is not configured")).mockResolvedValueOnce("diag-2");
    render(<DiagnoseButton onDiagnose={onDiagnose} reasonLabel="none" />);

    fireEvent.click(screen.getByTestId("backlog-action-diagnose"));
    expect(await screen.findByRole("alert")).toHaveTextContent("diagnostic dispatch is not configured");

    fireEvent.click(screen.getByTestId("backlog-action-diagnose"));
    await waitFor(() => expect(screen.getByTestId("backlog-action-diagnose-session-link")).toBeInTheDocument());
    expect(onDiagnose).toHaveBeenCalledTimes(2);
  });
});

function item(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1", title: "t", status: "in_progress", priority: 2, skipPlanning: false, skipReviewGate: false,
    autoSpawnSession: false, autoCreatePR: false, autoApprovePlan: false, planApproved: true, acCriteria: [],
    linkedSessions: [], statusEvents: [], progressNotes: [], activityNotes: [], totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

function renderActions(props: { item: BacklogItem; terminalState?: "archived" | "removed" | null; withDiagnose?: boolean; onAction?: (a: string) => void }) {
  const noop = () => {};
  render(
    <ActionsSection
      item={props.item} actionLoading={null} latestWorkSession={undefined} manualReview={noManualReview} onAction={props.onAction ?? noop}
      terminalState={props.terminalState ?? null} activeWorkSessionCount={0} onSendBackWithFeedback={async () => {}}
      diagnose={props.withDiagnose === false ? undefined : { run: async () => "s", reasonLabel: "none" }}
    />
  );
}

describe("ActionsSection — Diagnose button placement", () => {
  it("shows Diagnose on a live item", () => {
    renderActions({ item: item() });
    expect(screen.getByTestId("backlog-action-diagnose")).toBeInTheDocument();
  });
  it.each([
    ["a done item", { item: item({ status: "done" }) }],
    ["an item archived elsewhere", { item: item(), terminalState: "archived" as const }],
    ["a caller that does not wire diagnose", { item: item(), withDiagnose: false }],
  ])("hides Diagnose for %s", (_name, props) => {
    renderActions(props);
    expect(screen.queryByTestId("backlog-action-diagnose")).not.toBeInTheDocument();
  });
});

describe("ActionsSection — item action tracking", () => {
  beforeEach(() => mockTrack.mockClear());

  it("tracks the action once and still forwards it to the parent", () => {
    const onAction = jest.fn();
    renderActions({ item: item({ status: "review" }), onAction });

    fireEvent.click(screen.getByTestId("backlog-action-delete"));

    expect(onAction).toHaveBeenCalledWith("delete");
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({ name: "backlog_item_action", labels: { action: "delete" } }));
  });
});
