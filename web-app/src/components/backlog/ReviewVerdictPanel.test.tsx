import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReviewVerdictPanel } from "./ReviewVerdictPanel";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: jest.fn() }));
const mockFlag = useFeatureFlag as jest.Mock;

const full: NonNullable<LinkedSession["reviewVerdict"]> = {
  overallOutcome: "PARTIAL",
  summary: "One criterion missing",
  perCriterion: [
    { criterionIndex: 0, outcome: "PASS", evidence: "tests green" },
    { criterionIndex: 1, outcome: "FAIL", evidence: "no mobile check" },
  ],
};

beforeEach(() => mockFlag.mockReturnValue(true));

describe("ReviewVerdictPanel", () => {
  it("renders overall badge and per-criterion rows with evidence", () => {
    render(<ReviewVerdictPanel verdict={full} />);
    expect(screen.getByTestId("review-overall")).toHaveTextContent("PARTIAL");
    expect(screen.getByText("Criterion 1")).toBeInTheDocument();
    expect(screen.getByText("tests green")).toBeInTheDocument();
    expect(screen.getByText("no mobile check")).toBeInTheDocument();
  });

  it("flags criteria the owner must decide on, not ones rework can fix", () => {
    const verdict = {
      overallOutcome: "FAIL" as const,
      perCriterion: [
        { criterionIndex: 0, outcome: "FAIL", evidence: "e", failureClass: "NOT_DONE" },
        { criterionIndex: 1, outcome: "FAIL", evidence: "e", failureClass: "UNATTAINABLE" },
        { criterionIndex: 2, outcome: "PARTIAL", evidence: "e", proposedWaiver: true },
      ],
    };
    render(<ReviewVerdictPanel verdict={verdict} />);
    expect(screen.queryByTestId("review-owner-decision-0")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-owner-decision-1")).toHaveTextContent("Needs your decision: unattainable");
    expect(screen.getByTestId("review-owner-decision-2")).toHaveTextContent("Reviewer proposes waiving this");
  });

  it("renders nothing when the flag is off", () => {
    mockFlag.mockReturnValue(false);
    const { container } = render(<ReviewVerdictPanel verdict={full} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("is hidden when there is no verdict", () => {
    const { container } = render(<ReviewVerdictPanel verdict={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows a skeleton while loading", () => {
    render(<ReviewVerdictPanel verdict={undefined} loading />);
    expect(screen.getByTestId("review-verdict-skeleton")).toHaveAttribute("aria-busy", "true");
  });

  it("toggles the raw JSON disclosure", () => {
    render(<ReviewVerdictPanel verdict={full} />);
    const details = screen.getByTestId("review-raw-json") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    fireEvent.click(screen.getByText("View raw JSON"));
    expect(details.open).toBe(true);
    fireEvent.click(screen.getByText("View raw JSON"));
    expect(details.open).toBe(false);
  });

  it("falls back to a parse-error message for a malformed verdict", () => {
    render(<ReviewVerdictPanel verdict={{ perCriterion: "x" } as unknown as LinkedSession["reviewVerdict"]} />);
    expect(screen.getByText("Review result could not be displayed.")).toBeInTheDocument();
    expect(screen.getByTestId("review-raw-json")).toBeInTheDocument();
  });
});
