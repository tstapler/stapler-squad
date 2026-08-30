import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDiagnosticPanel } from "./SessionDiagnosticPanel";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

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

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "entity-1",
    sessionId: "headless-triage-7f2a9c1d",
    role: "triage",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

describe("SessionDiagnosticPanel_should_RenderTriageReviewPanelReadOnly_When_SessionKindHeadlessDiagnosticWithTriageResultPopulated", () => {
  it("routes to TriageReviewPanel readOnly, not GateVerdictBox, when triageResult is populated", () => {
    const session = makeSession({
      sessionId: "headless-triage-7f2a9c1d",
      role: "triage",
      triageResult: {
        summary: "Refined 3 acceptance criteria for clarity.",
        suggestions: [{ text: "Split AC #2", rationale: "clarity" }],
        clarifyingQuestions: [],
      },
    });

    render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(screen.getByTestId("triage-review-panel")).toBeInTheDocument();
    expect(screen.queryByText("Gate Verdict")).not.toBeInTheDocument();
    expect(screen.getByText("Triage completed — 1 suggestion")).toBeInTheDocument();
  });
});

describe("SessionDiagnosticPanel_should_RenderGateVerdictBoxReadOnly_When_SessionKindHeadlessDiagnosticWithReviewVerdictPopulated", () => {
  it("routes to GateVerdictBox readOnly when reviewVerdict is populated and triageResult is absent", () => {
    const session = makeSession({
      sessionId: "headless-re-review-4b8e2f",
      role: "review",
      reviewVerdict: {
        overallOutcome: "FAIL",
        summary: "2 of 5 criteria still not met after rework.",
        perCriterion: [
          { criterionIndex: 1, outcome: "FAIL", evidence: "error not surfaced to user" },
          { criterionIndex: 2, outcome: "PASS", evidence: "" },
        ],
      },
    });

    render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(screen.queryByTestId("triage-review-panel")).not.toBeInTheDocument();
    expect(screen.getByText("FAILED")).toBeInTheDocument();
    expect(screen.getByText("2 of 5 criteria still not met after rework.")).toBeInTheDocument();
    // No action buttons — readOnly.
    expect(screen.queryByRole("button", { name: /Reopen for Revision/i })).not.toBeInTheDocument();
  });
});

describe("SessionDiagnosticPanel_should_DispatchToBlockedNotice_When_SessionKindIsManualReviewMarker", () => {
  it("routes a manual-review-* session to BlockedNotice with the identical treatment as blocked_guardrail", () => {
    const session = makeSession({
      sessionId: "manual-review-a1b2c3d4-1721577600000000000",
      role: "review",
      reviewVerdict: { overallOutcome: "PASS", summary: "Manual review: verified fix locally" },
    });

    render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(screen.getByTestId("blocked-notice")).toBeInTheDocument();
    expect(screen.getByText("Manual review: verified fix locally")).toBeInTheDocument();
    expect(screen.queryByTestId("triage-review-panel")).not.toBeInTheDocument();
  });

  it("routes a review-blocked-* session to BlockedNotice too", () => {
    const session = makeSession({
      sessionId: "review-blocked-9c1d4a",
      role: "review",
      reviewVerdict: {
        overallOutcome: "FAIL",
        summary: "Review blocked by security check: secret pattern detected: github_pat. Override required to proceed.",
      },
    });

    render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(screen.getByTestId("blocked-notice")).toBeInTheDocument();
    expect(screen.getByText(/Override required to proceed/)).toBeInTheDocument();
  });
});

describe("SessionDiagnosticPanel_should_UseRoleStatusNotRoleLog_When_RenderingOneLineStateSummary", () => {
  it("uses role=status for the one-line summary and never role=log", () => {
    const session = makeSession({
      sessionId: "headless-triage-7f2a9c1d",
      role: "triage",
      triageResult: { summary: "x", suggestions: [], clarifyingQuestions: [] },
    });

    render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(screen.getAllByRole("status").length).toBeGreaterThan(0);
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
  });
});

describe("SessionDiagnosticPanel_should_FallBackToBlockedNotice_When_HeadlessDiagnosticHasNeitherTriageResultNorReviewVerdict", () => {
  it("never renders nothing for malformed/partial headless_diagnostic data", () => {
    const session = makeSession({
      sessionId: "headless-triage-orphan",
      role: "triage",
      // Neither triageResult nor reviewVerdict populated.
    });

    const { container } = render(<SessionDiagnosticPanel session={session} item={makeItem()} />);

    expect(container).not.toBeEmptyDOMElement();
    expect(screen.getByTestId("blocked-notice")).toBeInTheDocument();
    expect(screen.getByText("No diagnostic data recorded.")).toBeInTheDocument();
  });
});
