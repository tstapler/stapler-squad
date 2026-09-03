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
let capturedOnSelect: ((owner: string, repo: string, issues: GitHubIssue[]) => void) | null = null;
let capturedOnCancel: (() => void) | null = null;
// useFocusTrap.ts's own Tab-wrap behavior has its own dedicated suite
// (useFocusTrap.test.tsx) run against the real hook — mocked here so this
// file only asserts what's actually this component's responsibility: that
// it wires the hook onto its import-dialog ref with the trap active.
const useFocusTrapSpy = jest.fn();
jest.mock("@/lib/hooks/useFocusTrap", () => ({ useFocusTrap: (...args: unknown[]) => useFocusTrapSpy(...args) }));

jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: (props: { onSelect: (owner: string, repo: string, issues: GitHubIssue[]) => void; onCancel: () => void }) => {
    pickerRender(props);
    capturedOnSelect = props.onSelect;
    capturedOnCancel = props.onCancel;
    return (
      <div data-testid="mock-github-issue-picker">
        <input aria-label="Repository" />
        <button type="button" onClick={props.onCancel}>
          Cancel
        </button>
      </div>
    );
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
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    autoSpawnSession: false,
    autoCreatePR: false,
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
    isPR: false,
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
      capturedOnSelect!("octocat", "hello-world", [issue]);
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
      capturedOnSelect!("octocat", "hello-world", [issue]);
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
      capturedOnSelect!("octocat", "hello-world", [issue]);
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

describe("BacklogQueueSection — import dialog traps focus via useFocusTrap", () => {
  it("BacklogQueueSection_should_activateFocusTrapOnImportDialogRef_When_ImportOpened", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-github-issue-button"));

    await waitFor(() => expect(useFocusTrapSpy).toHaveBeenCalledWith(expect.anything(), true));
    const lastCall = useFocusTrapSpy.mock.calls[useFocusTrapSpy.mock.calls.length - 1];
    const [refArg, isActiveArg] = lastCall;
    expect(refArg.current).toBeInstanceOf(HTMLElement);
    expect(isActiveArg).toBe(true);
  });

  it("BacklogQueueSection_should_notCloseImportDialog_When_EscapePressed", async () => {
    render(<BacklogQueueSection />);
    await waitFor(() => expect(screen.getByText("Fix flaky test")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-github-issue-button"));
    expect(screen.getByTestId("backlog-queue-import-modal")).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });

    expect(screen.getByTestId("backlog-queue-import-modal")).toBeInTheDocument();
  });
});
