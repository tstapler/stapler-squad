/**
 * Regression test for the backlog default-view archived-item leak: the page
 * used to call listBacklogItems with `includeTerminal: true` unconditionally,
 * which (before the ExcludeDone/ExcludeArchived split in
 * session.BacklogItemFilter) also pulled in "archived" items by default with
 * no way to hide them. This verifies the frontend side of the fix — the
 * default fetch excludes archived items, and the "Show Archived" toggle
 * explicitly opts back in.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
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

// Heavy/irrelevant children — this test is focused on the list-fetch filter
// and the "Show Archived" toggle, not detail/form/picker rendering.
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

const listBacklogItems = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      listBacklogItems,
      createBacklogItem: jest.fn(),
      importGitHubIssue: jest.fn(),
      triggerTriage: jest.fn(),
    }),
  };
});

describe("BacklogPage archived-item default filtering", () => {
  beforeEach(() => {
    listBacklogItems.mockClear();
    listBacklogItems.mockResolvedValue([]);
  });

  it("BacklogPage_should_FetchWithIncludeArchivedFalse_When_LoadedByDefault", async () => {
    render(<BacklogPage />);

    await waitFor(() => expect(listBacklogItems).toHaveBeenCalled());

    expect(listBacklogItems).toHaveBeenCalledWith(
      expect.objectContaining({ includeTerminal: true, includeArchived: false })
    );
  });

  it("BacklogPage_should_RefetchWithIncludeArchivedTrue_When_ShowArchivedToggled", async () => {
    render(<BacklogPage />);

    await waitFor(() => expect(listBacklogItems).toHaveBeenCalled());
    listBacklogItems.mockClear();

    const toggle = screen.getByTestId("backlog-show-archived-toggle");
    fireEvent.click(toggle);

    await waitFor(() =>
      expect(listBacklogItems).toHaveBeenCalledWith(
        expect.objectContaining({ includeArchived: true })
      )
    );
  });

  it("BacklogPage_should_RenderShowArchivedToggle_UncheckedByDefault", async () => {
    render(<BacklogPage />);

    await waitFor(() => expect(listBacklogItems).toHaveBeenCalled());

    const toggle = screen.getByTestId("backlog-show-archived-toggle") as HTMLInputElement;
    expect(toggle.checked).toBe(false);
  });
});
