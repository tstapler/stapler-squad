import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import BacklogPage from "../page";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: () => {},
}));

jest.mock("@/components/backlog/BacklogItemDetail", () => ({
  BacklogItemDetail: () => null,
}));
jest.mock("@/components/backlog/BacklogItemForm", () => ({
  BacklogItemForm: () => null,
}));
jest.mock("@/components/backlog/BacklogEmptyState", () => ({
  BacklogEmptyState: () => null,
  FilterZeroState: () => null,
  FooterNudge: () => null,
}));
jest.mock("@/components/backlog/VaguenessPromptModal", () => ({
  VaguenessPromptModal: () => null,
}));
jest.mock("@/components/backlog/BacklogTourModal", () => ({
  BacklogTourModal: () => null,
}));
jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: () => null,
}));

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getBacklogItem: jest.fn() }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

jest.mock("@/lib/store", () => ({
  useAppDispatch: () => jest.fn(),
}));

jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      createBacklogItem: jest.fn(),
      importGitHubIssue: jest.fn(),
      triggerTriage: jest.fn(),
    }),
  };
});

const mockUseWatchBacklogItems = jest.fn();
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => mockUseWatchBacklogItems(),
}));

function itemFixture(overrides: Record<string, unknown>) {
  return {
    id: "item-1",
    title: "Fix retry loop in triage",
    status: "in_progress",
    priority: 3,
    acCriteria: [],
    liveVersion: 1,
    repoPath: "owner/repo",
    ...overrides,
  } as any;
}

describe("BacklogPage sortable header keyboard accessibility", () => {
  beforeEach(() => {
    mockUseWatchBacklogItems.mockReturnValue({
      items: [itemFixture({})],
      connectionState: "live",
    });
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("BacklogPage_should_ExposeSortableHeadersAsFocusableButtons_When_Rendered", () => {
    render(<BacklogPage />);
    for (const name of ["Title", "Status", "Priority", "Updated", "Repository"]) {
      const header = screen.getByRole("columnheader", { name: new RegExp(`^${name}`) });
      // The operable control is a nested <button>, not the <th> itself — this
      // is what gives screen readers a "button" affordance in addition to
      // the columnheader role (WAI-ARIA APG sortable-table pattern), and
      // native <button> focusability means no explicit tabIndex is needed.
      const button = within(header).getByRole("button", { name: new RegExp(`^${name}`) });
      expect(button.tagName).toBe("BUTTON");
    }
    const acHeader = screen.getByRole("columnheader", { name: "AC" });
    expect(within(acHeader).queryByRole("button")).toBeNull();
  });

  it("BacklogPage_should_SortAndToggleDirection_When_EnterIsPressedOnAHeader", () => {
    render(<BacklogPage />);
    const header = screen.getByRole("columnheader", { name: /^Title/ });
    const button = within(header).getByRole("button");

    expect(header).toHaveAttribute("aria-sort", "none");

    fireEvent.keyDown(button, { key: "Enter" });
    expect(header).toHaveAttribute("aria-sort", "descending");

    fireEvent.keyDown(button, { key: "Enter" });
    expect(header).toHaveAttribute("aria-sort", "ascending");
  });

  it("BacklogPage_should_SortAndPreventScroll_When_SpaceIsPressedOnAHeader", () => {
    render(<BacklogPage />);
    const header = screen.getByRole("columnheader", { name: /^Status/ });
    const button = within(header).getByRole("button");

    expect(header).toHaveAttribute("aria-sort", "none");

    const event = fireEvent.keyDown(button, { key: " " });
    expect(header).toHaveAttribute("aria-sort", "descending");
    // fireEvent.keyDown returns false when preventDefault() was called —
    // confirms Space won't also scroll the page.
    expect(event).toBe(false);
  });

  it("BacklogPage_should_NotTriggerSort_When_AnUnrelatedKeyIsPressedOnAHeader", () => {
    render(<BacklogPage />);
    const header = screen.getByRole("columnheader", { name: /^Priority/ });
    const button = within(header).getByRole("button");

    fireEvent.keyDown(button, { key: "Tab" });
    expect(header).toHaveAttribute("aria-sort", "none");
  });

  it("BacklogPage_should_LeaveNonSortableHeaderNonInteractive_When_Rendered", () => {
    render(<BacklogPage />);
    const acHeader = screen.getByRole("columnheader", { name: "AC" });

    expect(acHeader).not.toHaveAttribute("aria-sort");
    expect(within(acHeader).queryByRole("button")).toBeNull();
    fireEvent.keyDown(acHeader, { key: "Enter" });
    // No sort attribute appears/changes on an unrelated header.
    expect(acHeader).not.toHaveAttribute("aria-sort");
  });
});
