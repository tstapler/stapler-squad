import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { BacklogQueueSection } from "./BacklogQueueSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

const listBacklogItems = jest.fn();
const importGitHubIssue = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    listBacklogItems,
    importGitHubIssue,
  }),
}));

// GitHubIssuePicker pulls in useGitHubIssuePicker (Redux session selector); stub it
// so this test stays focused on BacklogQueueSection's own open/close + load behavior.
const pickerRender = jest.fn();
jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: (props: unknown) => {
    pickerRender(props);
    return <div data-testid="mock-github-issue-picker" />;
  },
}));

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Fix flaky test",
    status: "ready",
    priority: 2,
    skipPlanning: false,
    skipReviewGate: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("BacklogQueueSection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    listBacklogItems.mockResolvedValue([makeItem()]);
  });

  it("loads and renders queued backlog items on mount", async () => {
    render(<BacklogQueueSection />);

    expect(listBacklogItems).toHaveBeenCalledWith({ statuses: ["idea", "refining", "ready"] });
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());
  });

  it("does not mount the GitHub issue picker until the import affordance is opened", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    expect(pickerRender).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("import-github-issue-button"));

    expect(pickerRender).toHaveBeenCalled();
    expect(screen.getByTestId("mock-github-issue-picker")).toBeInTheDocument();
  });

  it("shows an empty state when there are no queued items", async () => {
    listBacklogItems.mockResolvedValue([]);
    render(<BacklogQueueSection />);

    await waitFor(() =>
      expect(screen.getByText(/No queued backlog items/i)).toBeInTheDocument()
    );
  });
});
