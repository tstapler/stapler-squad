/**
 * Tests for the backlog list page (Epic 5.4, Tasks 5.4.1c, 5.4.1d).
 *
 * Covers:
 *  - AC10: distinct table-row status chip class per status (including
 *    `duplicate`) for page.tsx's independently-maintained STATUS_CSS map.
 *  - Filter-chip default visibility: `archived` and `duplicate` are excluded
 *    from the default status filter chips, while `duplicate` remains a valid,
 *    sortable status (proxy: status-column sort ordering).
 */

import React from "react";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import BacklogPage from "./page";
import type { BacklogItem, KnownBacklogStatus } from "@/lib/hooks/useBacklogService";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockListBacklogItems = jest.fn();
const mockCreateBacklogItem = jest.fn();
const mockTriggerTriage = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    listBacklogItems: mockListBacklogItems,
    createBacklogItem: mockCreateBacklogItem,
    triggerTriage: mockTriggerTriage,
  }),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/backlog",
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
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

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({
    href,
    children,
    ...rest
  }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Test item",
    status: "idea",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    ...overrides,
  };
}

const ALL_STATUSES: KnownBacklogStatus[] = [
  "idea",
  "refining",
  "ready",
  "in_progress",
  "review",
  "done",
  "archived",
  "duplicate",
];

beforeEach(() => {
  jest.clearAllMocks();
  mockCreateBacklogItem.mockResolvedValue(null);
  mockTriggerTriage.mockResolvedValue(null);
});

afterEach(cleanup);

// ---------------------------------------------------------------------------
// Task 5.4.1c: table chip — all-statuses distinct class test
// ---------------------------------------------------------------------------

describe("BacklogTable_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate", () => {
  it("renders a distinct CSS class per status in the table row status chip", async () => {
    const items = ALL_STATUSES.map((status) =>
      makeItem({ id: `item-${status}`, status, title: `${status} item` })
    );
    mockListBacklogItems.mockResolvedValue(items);

    render(<BacklogPage />);

    const rows = await screen.findAllByTestId("backlog-table-row");
    expect(rows).toHaveLength(ALL_STATUSES.length);

    const classByStatus: Record<string, string> = {};
    for (const status of ALL_STATUSES) {
      const row = screen.getByText(`${status} item`).closest("tr") as HTMLElement;
      const chip = row.querySelector('[aria-label^="Status:"]') as HTMLElement;
      classByStatus[status] = chip.className;
    }

    const uniqueClasses = new Set(Object.values(classByStatus));
    expect(uniqueClasses.size).toBe(ALL_STATUSES.length);
    expect(classByStatus.duplicate).not.toBe(classByStatus.archived);
  });
});

// ---------------------------------------------------------------------------
// Task 5.4.1d: filter-chip default-visibility test
// ---------------------------------------------------------------------------

describe("StatusFilterChips default visibility", () => {
  it("excludes both 'archived' and 'duplicate' from the default status filter chips", async () => {
    mockListBacklogItems.mockResolvedValue([makeItem()]);

    render(<BacklogPage />);

    // Wait for the page to finish its initial load.
    await screen.findAllByTestId("backlog-table-row");

    expect(screen.queryByTestId("backlog-filter-status-archived")).not.toBeInTheDocument();
    expect(screen.queryByTestId("backlog-filter-status-duplicate")).not.toBeInTheDocument();

    // The other 6 statuses remain visible as filter chips.
    for (const status of ALL_STATUSES.filter((s) => s !== "archived" && s !== "duplicate")) {
      expect(screen.getByTestId(`backlog-filter-status-${status}`)).toBeInTheDocument();
    }
  });

  it("still treats 'duplicate' as a valid sortable status positioned after 'archived'", async () => {
    // Proxy for "duplicate remains in ALL_STATUSES": the status-column sort
    // uses ALL_STATUSES.indexOf() to order rows. If duplicate had silently
    // fallen out of that list, indexOf would return -1 and the item would
    // sort to the front regardless of click direction.
    const items = [
      makeItem({ id: "item-idea", status: "idea", title: "idea item" }),
      makeItem({ id: "item-done", status: "done", title: "done item" }),
      makeItem({ id: "item-archived", status: "archived", title: "archived item" }),
      makeItem({ id: "item-duplicate", status: "duplicate", title: "duplicate item" }),
    ];
    mockListBacklogItems.mockResolvedValue(items);

    render(<BacklogPage />);
    await screen.findAllByTestId("backlog-table-row");

    // Single click on the Status header: sortCol was "updatedAt", so this
    // switches to sortCol="status" with sortAsc=false (descending) —
    // highest ALL_STATUSES index first.
    fireEvent.click(screen.getByText(/^Status/));

    await waitFor(() => {
      const rows = screen.getAllByTestId("backlog-table-row");
      const orderedIds = rows.map((r) => r.getAttribute("data-item-id"));
      expect(orderedIds).toEqual([
        "item-duplicate",
        "item-archived",
        "item-done",
        "item-idea",
      ]);
    });
  });
});
