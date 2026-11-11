import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DiagnoseSessionPanel } from "./DiagnoseSessionPanel";
import { WorkflowSuggestionsSection } from "./WorkflowSuggestionsSection";
import { DiagnoseResultCard } from "../DiagnoseResultCard";
import { classifySessionKind } from "@/lib/backlog/sessionKind";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: jest.fn() }));
const mockFlag = useFeatureFlag as jest.Mock;

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-001",
    title: "Test item",
    status: "review",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

function makeDiagnose(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "e1",
    sessionId: "diag-uuid-1",
    role: "diagnose",
    estimatedCostUsd: 0.1234,
    startedAt: "2026-11-11T10:00:00Z",
    endedAt: "2026-11-11T10:05:00Z",
    ...overrides,
  };
}

beforeEach(() => mockFlag.mockReturnValue(true));

describe("classifySessionKind for diagnose", () => {
  it("returns diagnose for role diagnose regardless of sessionId", () => {
    expect(classifySessionKind({ role: "diagnose", sessionId: "diag-uuid-1" })).toBe("diagnose");
  });
});

describe("DiagnoseSessionPanel", () => {
  it("renders outcome badge, summary, cost and a transcript link from the structured result", () => {
    const session = makeDiagnose({ diagnoseResult: { outcome: "NOTE_ONLY", summary: "Stalled on CI." } });
    render(<DiagnoseSessionPanel session={session} item={makeItem()} />);
    expect(screen.getByTestId("diagnose-outcome-badge")).toHaveTextContent("NOTE_ONLY");
    expect(screen.getByText("Stalled on CI.")).toBeInTheDocument();
    expect(screen.getByText("$0.1234")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /View transcript/ })).toHaveAttribute("href", "/?session=diag-uuid-1");
  });

  it("links the filed bug when BUG_FILED", () => {
    const session = makeDiagnose({ diagnoseResult: { outcome: "BUG_FILED", summary: "s", filedItemId: "bug-9" } });
    render(<DiagnoseSessionPanel session={session} item={makeItem()} />);
    expect(screen.getByRole("link", { name: /View filed bug/ })).toHaveAttribute("href", expect.stringContaining("bug-9"));
  });

  it("falls back to the matching activity note when diagnoseResult is empty (old session)", () => {
    const item = makeItem({
      activityNotes: [
        { id: "n1", message: "Diagnosis result: NUDGED\n\nSent a redirect.", authorSessionUuid: "diag-uuid-1", authorSessionTitle: "t" },
      ],
    });
    render(<DiagnoseSessionPanel session={makeDiagnose()} item={item} />);
    expect(screen.getByTestId("diagnose-outcome-badge")).toHaveTextContent("NUDGED");
    expect(screen.getByText("Sent a redirect.")).toBeInTheDocument();
  });

  it("shows a plain status line when there is neither a result nor a note", () => {
    render(<DiagnoseSessionPanel session={makeDiagnose()} item={makeItem()} />);
    expect(screen.queryByTestId("diagnose-outcome-badge")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("No diagnosis result was recorded.");
    expect(screen.getByRole("link", { name: /View transcript/ })).toBeInTheDocument();
  });
});

describe("WorkflowSuggestionsSection", () => {
  const suggestion = (id: string, dismissed = false) => ({
    suggestionId: id,
    actionType: "transition_status",
    recommendation: `Move back to ready ${id}`,
    sessionUuid: "diag-uuid-1",
    dismissed,
  });

  it("calls onDismiss and removes the card", async () => {
    const onDismiss = jest.fn().mockResolvedValue(true);
    render(<WorkflowSuggestionsSection item={makeItem({ workflowSuggestions: [suggestion("s1")] })} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(screen.queryByTestId("workflow-suggestion-card")).not.toBeInTheDocument());
    expect(onDismiss).toHaveBeenCalledWith("item-001", "s1");
  });

  it("keeps the card and shows an error when dismiss fails", async () => {
    const onDismiss = jest.fn().mockRejectedValue(new Error("boom"));
    render(<WorkflowSuggestionsSection item={makeItem({ workflowSuggestions: [suggestion("s1")] })} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByTestId("workflow-suggestion-card")).toBeInTheDocument();
  });

  it("hides already-dismissed suggestions and renders nothing when none are active", () => {
    const { container } = render(
      <WorkflowSuggestionsSection item={makeItem({ workflowSuggestions: [suggestion("s1", true)] })} onDismiss={jest.fn()} />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the diagnose-session-panel flag is off", () => {
    mockFlag.mockReturnValue(false);
    const { container } = render(
      <WorkflowSuggestionsSection item={makeItem({ workflowSuggestions: [suggestion("s1")] })} onDismiss={jest.fn()} />
    );
    expect(container).toBeEmptyDOMElement();
  });
});

describe("DiagnoseResultCard", () => {
  it("links to the authoring session when a uuid is given", () => {
    render(<DiagnoseResultCard message={"Diagnosis result: NO_ACTION\n\nAll fine."} sessionUuid="diag-uuid-1" />);
    expect(screen.getByRole("link", { name: /View session/ })).toHaveAttribute("href", "/?session=diag-uuid-1");
  });

  it("omits the link without a uuid", () => {
    render(<DiagnoseResultCard message={"Diagnosis result: NO_ACTION\n\nAll fine."} />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
