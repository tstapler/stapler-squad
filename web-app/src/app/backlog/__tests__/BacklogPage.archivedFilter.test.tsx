/**
 * Regression test for the backlog default-view archived-item leak: the page
 * used to fetch server-side with `includeTerminal: true` unconditionally,
 * which (before the ExcludeDone/ExcludeArchived split in
 * session.BacklogItemFilter) also pulled in "archived" items by default with
 * no way to hide them. This verifies the frontend side of the fix — archived
 * items are hidden by default and the "Show Archived" toggle explicitly opts
 * back in.
 *
 * Updated for Epic 5.1 (backlog-event-driven-updates, project_plans/
 * backlog-event-driven-updates): the page no longer fetches via
 * useBacklogService().listBacklogItems on mount — it reads live from
 * useWatchBacklogItems and applies the same archived/status/priority/search
 * filtering client-side instead of via server-side request params (see
 * design/ux.md Surface 1's "subscribes ... unfiltered ... then filters
 * client-side same as today"). This test now asserts the client-side
 * filtering behavior directly rather than the request params sent to a
 * one-time REST fetch.
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

// The page's own raw-client hydration path (used only after item creation)
// — never exercised in this test, but its module-level client construction
// still runs on mount, so stub the transport layer to keep it inert.
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

// useWatchBacklogItems (mocked below) now returns domain-shaped BacklogItem
// objects directly (Epic 5.2 fix — mapping happens inside the hook), so
// these fixtures mirror useBacklogService.ts's mapped BacklogItem shape, not
// the raw proto message.
const activeItem = {
  id: "item-active",
  title: "Active item",
  status: "in_progress",
  priority: 3,
  acCriteria: [],
} as any;

const archivedItem = {
  id: "item-archived",
  title: "Archived item",
  status: "archived",
  priority: 3,
  acCriteria: [],
} as any;

const mockUseWatchBacklogItems = jest.fn();
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => mockUseWatchBacklogItems(),
}));

describe("BacklogPage archived-item default filtering", () => {
  beforeEach(() => {
    mockUseWatchBacklogItems.mockReturnValue({
      items: [activeItem, archivedItem],
      connectionState: "live",
    });
  });

  it("BacklogPage_should_HideArchivedItems_When_LoadedByDefault", async () => {
    render(<BacklogPage />);

    await waitFor(() => expect(screen.getByText("Active item")).toBeInTheDocument());
    expect(screen.queryByText("Archived item")).not.toBeInTheDocument();
  });

  it("BacklogPage_should_ShowArchivedItems_When_ShowArchivedToggled", async () => {
    render(<BacklogPage />);

    await waitFor(() => expect(screen.getByText("Active item")).toBeInTheDocument());

    const toggle = screen.getByTestId("backlog-show-archived-toggle");
    fireEvent.click(toggle);

    await waitFor(() => expect(screen.getByText("Archived item")).toBeInTheDocument());
    expect(screen.getByText("Active item")).toBeInTheDocument();
  });

  it("BacklogPage_should_RenderShowArchivedToggle_UncheckedByDefault", async () => {
    render(<BacklogPage />);

    const toggle = screen.getByTestId("backlog-show-archived-toggle") as HTMLInputElement;
    expect(toggle.checked).toBe(false);
  });
});
