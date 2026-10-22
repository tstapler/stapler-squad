import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogResultPanels } from "./BacklogResultPanels";
import { PlanningSection } from "./detail/PlanningSection";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: jest.fn() }));
const mockFlag = useFeatureFlag as jest.Mock;

const item = (over: Partial<BacklogItem>) =>
  ({ status: "review", priority: 2, triageStatus: "completed", linkedSessions: [], ...over }) as unknown as BacklogItem;

beforeEach(() => mockFlag.mockReturnValue(true));

describe("BacklogResultPanels", () => {
  it("shows the review skeleton only while a review session is running", () => {
    const running = item({ linkedSessions: [{ role: "review" }] as BacklogItem["linkedSessions"] });
    const { unmount } = render(<BacklogResultPanels item={running} />);
    expect(screen.getByTestId("review-verdict-skeleton")).toBeInTheDocument();
    unmount();
    render(<BacklogResultPanels item={item({})} />);
    expect(screen.queryByTestId("review-verdict-skeleton")).not.toBeInTheDocument();
  });

  it("hides the legacy Planning section when the triage panel flag is on, shows it when off", () => {
    const withPlan = item({ status: "ready", triageResult: { summary: "plan", tasks: [], suggestions: [] } } as unknown as Partial<BacklogItem>);
    const { unmount } = render(<PlanningSection item={withPlan} />);
    expect(screen.queryByText("Planning")).not.toBeInTheDocument();
    unmount();
    mockFlag.mockReturnValue(false);
    render(<PlanningSection item={withPlan} />);
    expect(screen.getByText("Planning")).toBeInTheDocument();
  });
});
