import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { TriageResultPanel } from "./TriageResultPanel";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import type { TriageResult } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: jest.fn() }));
const mockFlag = useFeatureFlag as jest.Mock;

const full: TriageResult = {
  summary: "Add a formatted view",
  suggestions: [
    { text: "Use details element", rationale: "accessible" },
    { text: "Which view?", rationale: "question" },
  ],
  clarifyingQuestions: [],
  tasks: [{ text: "Build panel", estimate: "2h", category: "frontend" }],
};

beforeEach(() => mockFlag.mockReturnValue(true));

describe("TriageResultPanel", () => {
  it("renders priority, tasks with estimates, and suggestions (not questions)", () => {
    render(<TriageResultPanel triageResult={full} priority={2} />);
    expect(screen.getByText("Triage Results")).toBeInTheDocument();
    expect(screen.getByText("P2")).toBeInTheDocument();
    expect(screen.getByText("Build panel")).toBeInTheDocument();
    expect(screen.getByText("2h")).toBeInTheDocument();
    expect(screen.getByText("Use details element")).toBeInTheDocument();
    expect(screen.queryByText("Which view?")).not.toBeInTheDocument();
  });

  it("renders nothing when the flag is off", () => {
    mockFlag.mockReturnValue(false);
    const { container } = render(<TriageResultPanel triageResult={full} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("is hidden when there is no triage result", () => {
    const { container } = render(<TriageResultPanel triageResult={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows a skeleton while loading", () => {
    render(<TriageResultPanel triageResult={undefined} loading />);
    expect(screen.getByTestId("triage-result-skeleton")).toHaveAttribute("aria-busy", "true");
  });

  it("toggles the raw JSON disclosure", () => {
    render(<TriageResultPanel triageResult={full} />);
    const details = screen.getByTestId("triage-raw-json") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    fireEvent.click(screen.getByText("View raw JSON"));
    expect(details.open).toBe(true);
    expect(details.querySelector("pre")?.textContent).toBe(JSON.stringify(full, null, 2));
    fireEvent.click(screen.getByText("View raw JSON"));
    expect(details.open).toBe(false);
  });

  it("falls back to a parse-error message with raw access for a malformed result", () => {
    render(<TriageResultPanel triageResult={{ summary: 7 } as unknown as TriageResult} />);
    expect(screen.getByText("Triage result could not be displayed.")).toBeInTheDocument();
    expect(screen.getByTestId("triage-raw-json")).toBeInTheDocument();
  });

  it("falls back instead of throwing when tasks is not an array", () => {
    render(<TriageResultPanel triageResult={{ summary: "s", suggestions: [], tasks: "oops" } as unknown as TriageResult} />);
    expect(screen.getByText("Triage result could not be displayed.")).toBeInTheDocument();
  });
});
