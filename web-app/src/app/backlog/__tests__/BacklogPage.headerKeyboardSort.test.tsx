import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import BacklogPage from "../page";
import { itemFixture, mockUseWatchBacklogItems } from "./backlogPageTestFixtures";

jest.mock("next/navigation", () => require("./backlogPageTestFixtures").nextNavigationMock());
jest.mock("@/lib/analytics", () => require("./backlogPageTestFixtures").analyticsMock());
jest.mock("@/lib/analytics/usePageView", () => require("./backlogPageTestFixtures").usePageViewMock());
jest.mock("@/components/backlog/BacklogItemDetail", () => require("./backlogPageTestFixtures").backlogItemDetailMock());
jest.mock("@/components/backlog/BacklogItemForm", () => require("./backlogPageTestFixtures").backlogItemFormMock());
jest.mock("@/components/backlog/BacklogEmptyState", () => require("./backlogPageTestFixtures").backlogEmptyStateMock());
jest.mock("@/components/backlog/VaguenessPromptModal", () => require("./backlogPageTestFixtures").vaguenessPromptModalMock());
jest.mock("@/components/backlog/BacklogTourModal", () => require("./backlogPageTestFixtures").backlogTourModalMock());
jest.mock("@/components/backlog/GitHubIssuePicker", () => require("./backlogPageTestFixtures").gitHubIssuePickerMock());
jest.mock("@connectrpc/connect", () => require("./backlogPageTestFixtures").connectMock());
jest.mock("@connectrpc/connect-web", () => require("./backlogPageTestFixtures").connectWebMock());
jest.mock("@/lib/config", () => require("./backlogPageTestFixtures").configMock());
jest.mock("@/lib/store", () => require("./backlogPageTestFixtures").storeMock());
jest.mock("@/lib/hooks/useBacklogService", () => require("./backlogPageTestFixtures").useBacklogServiceMockFactory());
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogPageTestFixtures").useWatchBacklogItemsMock());

describe("BacklogPage sortable header keyboard accessibility", () => {
  beforeEach(() => {
    // Sort state is persisted to localStorage (see usePersistedViewState /
    // BACKLOG_VIEW_FIELDS in page.tsx) — clear it so each test starts from
    // the same default ("updatedAt", descending) regardless of what a
    // previous test in this file left behind.
    localStorage.clear();
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

  it("BacklogPage_should_SortAndToggleDirection_When_HeaderButtonIsClicked", () => {
    render(<BacklogPage />);
    const header = screen.getByRole("columnheader", { name: /^Title/ });
    const button = within(header).getByRole("button");

    expect(header).toHaveAttribute("aria-sort", "none");

    fireEvent.click(button);
    expect(header).toHaveAttribute("aria-sort", "descending");

    fireEvent.click(button);
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
