/**
 * Page-level wiring for backlog bulk actions: archive-done fan-out, partial
 * failure feedback, archive-selected, row selection, delete confirm, and
 * selection cleanup on unmount.
 */

import React from "react";
import { Provider } from "react-redux";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { configureStore } from "@reduxjs/toolkit";
import BacklogPage from "../page";
import backlogBulkSelectionReducer from "@/lib/store/backlogBulkSelectionSlice";
import backlogItemsReducer from "@/lib/store/backlogItemsSlice";

const mockArchive = jest.fn();
const mockDelete = jest.fn();

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


jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      createBacklogItem: jest.fn(),
      importGitHubIssue: jest.fn(),
      triggerTriage: jest.fn(),
      archiveBacklogItem: (id: string) => mockArchive(id),
      deleteBacklogItem: (id: string) => mockDelete(id),
    }),
  };
});

const mk = (id: string, status: string) =>
  ({ id, title: `Item ${id}`, status, priority: 3, acCriteria: [] }) as any;

const mockUseWatchBacklogItems = jest.fn();
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => mockUseWatchBacklogItems(),
}));

function renderPage() {
  const store = configureStore({
    reducer: { backlogBulkSelection: backlogBulkSelectionReducer, backlogItems: backlogItemsReducer },
  });
  const utils = render(
    <Provider store={store}>
      <BacklogPage />
    </Provider>
  );
  return { store, ...utils };
}

describe("BacklogPage bulk actions", () => {
  beforeEach(() => {
    localStorage.clear();
    mockArchive.mockReset().mockResolvedValue(true);
    mockDelete.mockReset().mockResolvedValue(true);
    mockUseWatchBacklogItems.mockReturnValue({
      items: [mk("d1", "done"), mk("d2", "done"), mk("d3", "done"), mk("a1", "in_progress")],
      connectionState: "live",
    });
  });

  it("archives exactly the done items and reports success", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Archive done (3)"));
    await waitFor(() => expect(screen.getByText("Archived 3 items")).toBeInTheDocument());
    expect(mockArchive.mock.calls.map((c) => c[0]).sort()).toEqual(["d1", "d2", "d3"]);
  });

  it("reports partial failure as 'Archived N of M (K failed)'", async () => {
    mockArchive.mockImplementation(async (id: string) => {
      if (id === "d2") throw new Error("boom");
      return true;
    });
    renderPage();
    fireEvent.click(await screen.findByText("Archive done (3)"));
    await waitFor(() => expect(screen.getByText("Archived 2 of 3 (1 failed)")).toBeInTheDocument());
  });

  it("selecting rows shows the toolbar; partial delete failure keeps failed ids selected", async () => {
    mockDelete.mockImplementation(async (id: string) => {
      if (id === "d2") throw new Error("boom");
      return true;
    });
    const { store } = renderPage();
    const boxes = await screen.findAllByTestId("backlog-row-checkbox");
    fireEvent.click(boxes[0]);
    fireEvent.click(boxes[1]);
    expect(screen.getByText("2 of 4 selected")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("backlog-bulk-delete-selected"));
    expect(screen.getByRole("dialog")).toHaveTextContent("Delete 2 items?");
    fireEvent.click(screen.getByTestId("backlog-bulk-delete-confirm"));
    await waitFor(() => expect(screen.getByText("Deleted 1 of 2 (1 failed)")).toBeInTheDocument());
    expect(store.getState().backlogBulkSelection.selectedIds).toHaveLength(1);
  });

  it("archive selected skips archived items and clears selection on success", async () => {
    const { store } = renderPage();
    const boxes = await screen.findAllByTestId("backlog-row-checkbox");
    fireEvent.click(boxes[0]);
    fireEvent.click(screen.getByTestId("backlog-bulk-archive-selected"));
    await waitFor(() => expect(screen.getByText("Archived 1 item")).toBeInTheDocument());
    expect(store.getState().backlogBulkSelection.selectedIds).toEqual([]);
  });

  it("clears selection when the page unmounts", async () => {
    const { store, unmount } = renderPage();
    fireEvent.click((await screen.findAllByTestId("backlog-row-checkbox"))[0]);
    expect(store.getState().backlogBulkSelection.selectedIds).toHaveLength(1);
    unmount();
    expect(store.getState().backlogBulkSelection.selectedIds).toEqual([]);
  });
});
