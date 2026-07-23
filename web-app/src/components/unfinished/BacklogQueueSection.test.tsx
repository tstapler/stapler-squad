import React from "react";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { BacklogQueueSection } from "./BacklogQueueSection";
import type { BacklogItem, GitHubIssue } from "@/lib/hooks/useBacklogService";

const listBacklogItems = jest.fn();
const importGitHubIssue = jest.fn();
let mockLastError: Error | null = null;

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    listBacklogItems,
    importGitHubIssue,
    lastError: mockLastError,
  }),
}));

// GitHubIssuePicker pulls in useGitHubIssuePicker (Redux session selector); stub it
// so this test stays focused on BacklogQueueSection's own open/close + load behavior.
// The mock captures the onSelect/onCancel props so tests can drive the picker's
// callbacks directly without needing the real picker's own selection UI.
const pickerRender = jest.fn();
let capturedOnSelect: ((owner: string, repo: string, issue: GitHubIssue) => void) | null = null;
let capturedOnCancel: (() => void) | null = null;
jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: (props: { onSelect: (owner: string, repo: string, issue: GitHubIssue) => void; onCancel: () => void }) => {
    pickerRender(props);
    capturedOnSelect = props.onSelect;
    capturedOnCancel = props.onCancel;
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

function makeIssue(overrides: Partial<GitHubIssue> = {}): GitHubIssue {
  return {
    number: 42,
    title: "Some issue",
    state: "open",
    url: "",
    labels: [],
    ...overrides,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockLastError = null;
  capturedOnSelect = null;
  capturedOnCancel = null;
  listBacklogItems.mockResolvedValue([makeItem()]);
});

describe("BacklogQueueSection — loads and renders queued backlog items on mount", () => {
  it("loads and renders queued backlog items on mount", async () => {
    render(<BacklogQueueSection />);

    expect(listBacklogItems).toHaveBeenCalledWith({ statuses: ["idea", "refining", "ready"] });
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());
  });
});

describe("BacklogQueueSection — does not mount the GitHub issue picker until the import affordance is opened", () => {
  it("does not mount the GitHub issue picker until the import affordance is opened", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    expect(pickerRender).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("import-github-issue-button"));

    expect(pickerRender).toHaveBeenCalled();
    expect(screen.getByTestId("mock-github-issue-picker")).toBeInTheDocument();
  });
});

describe("BacklogQueueSection — shows an empty state when there are no queued items", () => {
  it("shows an empty state when there are no queued items", async () => {
    listBacklogItems.mockResolvedValue([]);
    render(<BacklogQueueSection />);

    await waitFor(() =>
      expect(screen.getByText(/No queued backlog items/i)).toBeInTheDocument()
    );
  });
});

describe("BacklogQueueSection — imports a selected GitHub issue and reloads the list", () => {
  it("builds the fallback URL, imports, closes the modal, and reloads on success", async () => {
    importGitHubIssue.mockResolvedValue({ item: makeItem(), triageTriggered: false });

    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-github-issue-button"));
    expect(screen.getByTestId("backlog-queue-import-modal")).toBeInTheDocument();

    expect(capturedOnSelect).not.toBeNull();
    expect(listBacklogItems).toHaveBeenCalledTimes(1);

    const issue = makeIssue({ number: 7, url: "" });
    await act(async () => {
      capturedOnSelect!("octocat", "hello-world", issue);
    });

    await waitFor(() =>
      expect(importGitHubIssue).toHaveBeenCalledWith("https://github.com/octocat/hello-world/issues/7")
    );

    await waitFor(() =>
      expect(screen.queryByTestId("backlog-queue-import-modal")).not.toBeInTheDocument()
    );

    await waitFor(() => expect(listBacklogItems).toHaveBeenCalledTimes(2));
  });

  it("uses issue.url directly when present instead of the constructed fallback", async () => {
    importGitHubIssue.mockResolvedValue({ item: makeItem(), triageTriggered: false });

    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-github-issue-button"));
    const issue = makeIssue({ number: 9, url: "https://github.com/octocat/hello-world/issues/9" });
    await act(async () => {
      capturedOnSelect!("octocat", "hello-world", issue);
    });

    await waitFor(() =>
      expect(importGitHubIssue).toHaveBeenCalledWith("https://github.com/octocat/hello-world/issues/9")
    );
  });
});

describe("BacklogQueueSection — surfaces an error when GitHub issue import fails", () => {
  it("shows an error message and keeps the modal closed when importGitHubIssue resolves falsy", async () => {
    importGitHubIssue.mockResolvedValue(null);
    mockLastError = new Error("rate limited");

    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-github-issue-button"));
    const issue = makeIssue({ number: 3, url: "" });
    await act(async () => {
      capturedOnSelect!("octocat", "hello-world", issue);
    });

    await waitFor(() =>
      expect(screen.queryByTestId("backlog-queue-import-modal")).not.toBeInTheDocument()
    );

    await waitFor(() => expect(screen.getByText(/Failed to import GitHub issue\.|rate limited/)).toBeInTheDocument());

    // Reload should not have been triggered on failure (only the initial mount load).
    expect(listBacklogItems).toHaveBeenCalledTimes(1);
  });
});

describe("BacklogQueueSection — surfaces an error when the initial backlog load fails", () => {
  it("shows a failure message when listBacklogItems rejects", async () => {
    listBacklogItems.mockRejectedValue(new Error("network down"));

    render(<BacklogQueueSection />);

    await waitFor(() =>
      expect(screen.getByText("Failed to load queued backlog items.")).toBeInTheDocument()
    );
  });
});

describe("BacklogQueueSection — toggles the collapsible section via click and keyboard", () => {
  it("toggles aria-expanded and hides the list content when the header is clicked", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    const header = screen.getByRole("button", { name: /Up Next/i });
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Fix flaky test")).toBeInTheDocument();

    fireEvent.click(header);

    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Fix flaky test")).not.toBeInTheDocument();
  });

  it("toggles aria-expanded when Enter is pressed on the header", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    const header = screen.getByRole("button", { name: /Up Next/i });
    expect(header).toHaveAttribute("aria-expanded", "true");

    fireEvent.keyDown(header, { key: "Enter" });

    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Fix flaky test")).not.toBeInTheDocument();
  });
});

describe("BacklogQueueSection — import button keyboard activation does not toggle the section", () => {
  it("does not bubble a keyDown from the import button to the section header", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    const header = screen.getByRole("button", { name: /Up Next/i });
    expect(header).toHaveAttribute("aria-expanded", "true");

    fireEvent.keyDown(screen.getByTestId("import-github-issue-button"), { key: "Enter" });

    // The section must remain expanded — the keydown must not have reached the
    // ancestor header's handleKeyDown (which would collapse the section).
    expect(header).toHaveAttribute("aria-expanded", "true");
  });
});
